// The world on screen: renderer, sky, sea, islands, the boat and the
// camera that follows it, in one loop. It knows nothing of React or the
// page's records: main.tsx hands it settings and islands, and reads back
// where the boat is.

import * as THREE from "three";
import { applySky, skyDome, skyLook, skyUniforms, type Weather } from "./sky";
import { oceanMaterial, oceanMesh, seabedMaterial, type OceanUniforms } from "./ocean";
import { groundAt, groundMesh, palmGeometry, palmSpots, Shape, type Isle } from "./islands";
import { boatModel, newBoat, placeBoat, stepBoat, type BoatState } from "./boat";
import { GEO_WAVES, SEAS, heightAt, makeWaves, packWaves, type Sea, type Wave } from "./waves";
import { Wake, WAKE_SIZE } from "./wake";
import { Controls } from "./controls";
import { clamp } from "./noise";

export type Settings = { time: number; weather: Weather; sea: Sea; wind: number };
export type Label = { id: string; name: string; x: number; y: number; dist: number };
export type Status = { boat: BoatState; near: { isle: Isle; dist: number } | null; labels: Label[]; fps: number; scale: number };

const SEABED_SIZE = 1600, SEABED_RES = 1024;

export class World {
  readonly renderer: THREE.WebGLRenderer;
  readonly controls = new Controls();
  readonly boat: BoatState = newBoat();
  private scene = new THREE.Scene();
  private camera = new THREE.PerspectiveCamera(55, 1, 0.3, 20000);
  private sky = skyUniforms();
  private dome = skyDome(this.sky);
  private ocean: { mat: THREE.ShaderMaterial; uniforms: OceanUniforms };
  private sun = new THREE.DirectionalLight(0xffffff, 1);
  private pmrem: THREE.PMREMGenerator;
  private env: THREE.WebGLRenderTarget | null = null;
  private model = boatModel();
  private wake = new Wake();
  private waves: Wave[] = [];
  private shapes: Shape[] = [];
  private isles = new THREE.Group();
  private seabed = { scene: new THREE.Scene(), target: new THREE.WebGLRenderTarget(SEABED_RES, SEABED_RES, { depthBuffer: true }), center: new THREE.Vector2(1e9, 1e9), dirty: true, cam: new THREE.OrthographicCamera(-SEABED_SIZE / 2, SEABED_SIZE / 2, SEABED_SIZE / 2, -SEABED_SIZE / 2, -100, 600) };
  private settings: Settings = { time: 15, weather: "cloudy", sea: "moderate", wind: 225 };
  private t = 0;
  private last = 0;
  private raf = 0;
  private frames: number[] = [];
  private scale = 1;
  private size = { w: 1, h: 1 };
  private view = { heading: 0, yaw: 0, pitch: 0.2, dist: 19, touched: -1e9, pointers: new Map<number, { x: number; y: number }>(), pinch: 0 };
  private labelsAt = 0;
  private status: Status = { boat: this.boat, near: null, labels: [], fps: 60, scale: 1 };
  onFrame: (s: Status) => void = () => {};

  constructor(readonly canvas: HTMLCanvasElement) {
    this.renderer = new THREE.WebGLRenderer({ canvas, antialias: true, powerPreference: "high-performance" });
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 0.85;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.pmrem = new THREE.PMREMGenerator(this.renderer);
    this.ocean = oceanMaterial(this.sky);
    this.scene.add(this.dome, oceanMesh(this.ocean.mat), this.isles, this.model, this.sun, this.sun.target);
    this.ocean.uniforms.uSeabed.value = this.seabed.target.texture;
    this.ocean.uniforms.uSeabedSize.value = SEABED_SIZE;
    this.ocean.uniforms.uWake.value = this.wake.target.texture;
    this.ocean.uniforms.uWakeSize.value = WAKE_SIZE;
    this.seabed.cam.up.set(0, 0, -1);
    this.scene.fog = new THREE.FogExp2(0xbcc8d0, 0.0005);
    this.setSettings(this.settings);
    this.bindView();
  }

  setSettings(s: Settings) {
    this.settings = s;
    const look = skyLook(s.time, s.weather);
    applySky(this.sky, look, s.wind);
    this.sun.color.copy(look.sunColor).multiplyScalar(1 / 2.2);
    this.sun.intensity = 2.2;
    (this.scene.fog as THREE.FogExp2).color.copy(look.horizon);
    (this.scene.fog as THREE.FogExp2).density = look.haze;
    this.waves = makeWaves(s.sea, s.wind);
    const g = packWaves(this.waves, GEO_WAVES);
    this.ocean.uniforms.uWA.value = g.A; this.ocean.uniforms.uWB.value = g.B;
    this.ocean.uniforms.uFoamAmt.value = SEAS[s.sea]?.foam ?? 0.5;
    this.ocean.uniforms.uRippleAmt.value = 0.55 + 0.45 * (SEAS[s.sea]?.height ?? 1);
    // the sky's light on the islands and the boat
    const skyScene = new THREE.Scene();
    const dome = skyDome(this.sky);
    skyScene.add(dome);
    const env = this.pmrem.fromScene(skyScene, 0, 1, 2000);
    dome.geometry.dispose(); (dome.material as THREE.Material).dispose();
    this.env?.dispose();
    this.env = env;
    this.scene.environment = env.texture;
    this.scene.environmentIntensity = 0.85;
  }

  setIslands(list: Isle[]) {
    for (const c of [...this.isles.children]) { this.isles.remove(c); disposeTree(c); }
    for (const c of [...this.seabed.scene.children]) this.seabed.scene.remove(c);
    this.shapes = list.map((i) => new Shape(i));
    const ground = new THREE.MeshStandardMaterial({ vertexColors: true, roughness: 0.93, metalness: 0 });
    const palmMat = new THREE.MeshStandardMaterial({ vertexColors: true, roughness: 0.85, side: THREE.DoubleSide });
    const palm = palmGeometry();
    const under = seabedMaterial();
    for (const s of this.shapes) {
      const geo = groundMesh(s);
      this.isles.add(new THREE.Mesh(geo, ground));
      this.seabed.scene.add(new THREE.Mesh(geo, under));
      const spots = palmSpots(s);
      if (spots.length) {
        const inst = new THREE.InstancedMesh(palm, palmMat, spots.length);
        const m = new THREE.Matrix4(), q = new THREE.Quaternion();
        spots.forEach((p, i) => {
          q.setFromEuler(new THREE.Euler(0, p.a, (p.s - 1) * 0.2));
          m.compose(new THREE.Vector3(p.x, p.y, p.z), q, new THREE.Vector3(p.s, p.s, p.s));
          inst.setMatrixAt(i, m);
        });
        inst.computeBoundingSphere();
        this.isles.add(inst);
      }
    }
    this.seabed.dirty = true;
  }

  ground = (x: number, z: number) => groundAt(this.shapes, x, z);
  waterAt = (x: number, z: number) => heightAt(this.waves, x, z, this.t);

  /** Puts the boat somewhere (and the camera behind it). */
  moveBoat(x: number, z: number, heading: number) {
    Object.assign(this.boat, newBoat(x, z, heading));
    this.view.heading = heading;
    this.wake.reset();
  }

  start() {
    const loop = (now: number) => {
      this.raf = requestAnimationFrame(loop);
      const dt = this.last ? Math.min(0.5, (now - this.last) / 1000) : 1 / 60;
      this.last = now;
      this.frame(dt);
    };
    this.raf = requestAnimationFrame(loop);
  }
  stop() { cancelAnimationFrame(this.raf); this.raf = 0; this.last = 0; }

  private resize() {
    const el = this.canvas.parentElement || this.canvas;
    const w = Math.max(1, el.clientWidth), h = Math.max(1, el.clientHeight);
    const dpr = Math.min(window.devicePixelRatio || 1, 2) * this.scale;
    if (w !== this.size.w || h !== this.size.h || Math.abs(this.renderer.getPixelRatio() - dpr) > 0.01) {
      this.size = { w, h };
      this.renderer.setPixelRatio(dpr);
      this.renderer.setSize(w, h, false);
      this.camera.aspect = w / h;
      this.camera.updateProjectionMatrix();
    }
  }

  /** One step of everything, and a picture. */
  frame(dt: number) {
    this.resize();
    this.advance(dt);
    placeBoat(this.model, this.boat);
    this.model.updateMatrixWorld();
    this.ocean.uniforms.uBoatInv.value.copy(this.model.matrixWorld).invert();
    this.follow(dt);
    this.ocean.uniforms.uCenter.value.set(this.camera.position.x, this.camera.position.z);
    this.ocean.uniforms.uPix.value = (2 * Math.tan((this.camera.fov * Math.PI) / 360)) / (this.size.h * this.renderer.getPixelRatio());
    this.dome.position.copy(this.camera.position);
    this.sun.position.set(this.boat.x, 0, this.boat.z).addScaledVector(this.sky.uSun.value, 300);
    this.sun.target.position.set(this.boat.x, 0, this.boat.z);
    // the seabed under and around the boat (again when it has gone far)
    const sb = this.seabed;
    if (sb.dirty || Math.hypot(this.boat.x - sb.center.x, this.boat.z - sb.center.y) > SEABED_SIZE * 0.25) {
      sb.center.set(Math.round(this.boat.x), Math.round(this.boat.z));
      sb.cam.position.set(sb.center.x, 400, sb.center.y);
      sb.cam.lookAt(sb.center.x, 0, sb.center.y);
      const r = this.renderer, prev = r.getRenderTarget();
      r.setRenderTarget(sb.target);
      r.setClearColor(0x000000, 1);
      r.clear();
      r.render(sb.scene, sb.cam);
      r.setRenderTarget(prev);
      this.ocean.uniforms.uSeabedCenter.value.copy(sb.center);
      sb.dirty = false;
    }
    this.wake.render(this.renderer, this.boat, this.t);
    this.ocean.uniforms.uWakeCenter.value.copy(this.wake.center);
    this.renderer.render(this.scene, this.camera);
    this.measure(dt);
    this.report();
  }

  /** Moves time on (the boat in small steps, however slow the frames). */
  advance(dt: number) {
    const input = this.controls.read();
    const n = Math.max(1, Math.ceil(dt / (1 / 30)));
    for (let i = 0; i < n; i++) {
      this.t += dt / n;
      stepBoat(this.boat, input, dt / n, this.waves, this.t, this.ground);
      this.wake.add(this.boat, this.t);
    }
    this.sky.uTime.value = this.t;
  }

  /** The camera behind the boat, a little lazy, kept above the water. */
  private follow(dt: number) {
    const v = this.view, b = this.boat;
    let d = b.heading - v.heading;
    d = Math.atan2(Math.sin(d), Math.cos(d));
    v.heading += d * Math.min(1, dt * 1.6);
    if (performance.now() - v.touched > 4000 && Math.abs(b.speed) > 1) v.yaw *= Math.exp(-dt * 0.8);
    const ang = v.heading + v.yaw;
    const fx = Math.sin(ang), fz = -Math.cos(ang);
    const flat = v.dist * Math.cos(v.pitch);
    const target = new THREE.Vector3(b.x, b.y + 1.8, b.z);
    const want = new THREE.Vector3(b.x - fx * flat, b.y + 2 + v.dist * Math.sin(v.pitch), b.z - fz * flat);
    const floor = Math.max(this.waterAt(want.x, want.z) + 1.2, this.ground(want.x, want.z) + 2);
    if (want.y < floor) want.y = floor;
    this.camera.position.copy(want);
    this.camera.lookAt(target);
  }

  private bindView() {
    const c = this.canvas, v = this.view;
    c.style.touchAction = "none";
    c.addEventListener("pointerdown", (e) => { c.setPointerCapture(e.pointerId); v.pointers.set(e.pointerId, { x: e.clientX, y: e.clientY }); v.touched = performance.now(); });
    const end = (e: PointerEvent) => { v.pointers.delete(e.pointerId); v.pinch = 0; };
    c.addEventListener("pointerup", end);
    c.addEventListener("pointercancel", end);
    c.addEventListener("pointermove", (e) => {
      const p = v.pointers.get(e.pointerId);
      if (!p) return;
      v.touched = performance.now();
      if (v.pointers.size === 2) {
        const [a, b] = [...v.pointers.values()];
        p.x = e.clientX; p.y = e.clientY;
        const d = Math.hypot(a.x - b.x, a.y - b.y);
        if (v.pinch) v.dist = clamp(v.dist * (v.pinch / d), 7, 140);
        v.pinch = d;
        return;
      }
      v.yaw -= (e.clientX - p.x) * 0.006;
      v.pitch = clamp(v.pitch + (e.clientY - p.y) * 0.004, 0.02, 1.35);
      p.x = e.clientX; p.y = e.clientY;
    });
    c.addEventListener("wheel", (e) => { e.preventDefault(); v.touched = performance.now(); v.dist = clamp(v.dist * Math.exp(e.deltaY * 0.0012), 7, 140); }, { passive: false });
    c.addEventListener("dblclick", () => { v.yaw = 0; v.pitch = 0.2; v.dist = 19; });
  }

  /** Lowers the resolution when frames are slow, raises it when there is room. */
  private measure(dt: number) {
    this.frames.push(dt);
    if (this.frames.length < 45) return;
    const avg = this.frames.reduce((a, b) => a + b, 0) / this.frames.length;
    this.frames = [];
    this.status.fps = Math.round(1 / avg);
    if (avg > 1 / 40 && this.scale > 0.5) this.scale = Math.max(0.5, this.scale - 0.1);
    else if (avg < 1 / 57 && this.scale < 1) this.scale = Math.min(1, this.scale + 0.05);
    this.status.scale = this.scale;
  }

  private report() {
    const b = this.boat;
    let near: Status["near"] = null;
    for (const s of this.shapes) {
      const d = Math.hypot(b.x - s.isle.x, b.z - s.isle.z) - s.isle.size;
      if (d < Math.max(140, s.isle.size * 0.9) && (!near || d < near.dist)) near = { isle: s.isle, dist: Math.max(0, d) };
    }
    this.status.near = near;
    const now = performance.now();
    if (now - this.labelsAt > 120) {
      this.labelsAt = now;
      const out: Label[] = [];
      const v = new THREE.Vector3();
      for (const s of this.shapes) {
        if (!s.isle.name) continue;
        const dist = Math.hypot(b.x - s.isle.x, b.z - s.isle.z);
        if (dist > 3200 || dist < s.isle.size + 60) continue;
        v.set(s.isle.x, Math.max(6, s.isle.height) + 8, s.isle.z).project(this.camera);
        if (v.z > 1 || Math.abs(v.x) > 1.05 || Math.abs(v.y) > 1.05) continue;
        out.push({ id: s.isle.id, name: s.isle.name, x: (v.x * 0.5 + 0.5) * this.size.w, y: (-v.y * 0.5 + 0.5) * this.size.h, dist });
      }
      out.sort((a, c) => a.dist - c.dist);
      const kept: Label[] = [];
      for (const l of out) if (kept.every((k) => Math.abs(k.y - l.y) > 26 || Math.abs(k.x - l.x) > 120)) kept.push(l);
      this.status.labels = kept;
    }
    this.onFrame(this.status);
  }

  dispose() {
    this.stop();
    disposeTree(this.scene);
    this.wake.dispose();
    this.seabed.target.dispose();
    this.env?.dispose();
    this.pmrem.dispose();
    this.renderer.dispose();
  }
}

function disposeTree(o: THREE.Object3D) {
  o.traverse((x) => {
    const m = x as THREE.Mesh;
    m.geometry?.dispose?.();
    const mat = m.material as THREE.Material | THREE.Material[] | undefined;
    if (Array.isArray(mat)) mat.forEach((y) => y.dispose()); else mat?.dispose?.();
  });
}

/** Whether this browser can draw 3D at all. */
export function canWebGL() {
  try {
    const c = document.createElement("canvas");
    const g = (c.getContext("webgl2") || c.getContext("webgl")) as WebGLRenderingContext | null;
    // (let the test context go at once: browsers keep only a few)
    g?.getExtension("WEBGL_lose_context")?.loseContext();
    return !!g;
  } catch { return false; }
}
