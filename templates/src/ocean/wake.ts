// The boat's wake, drawn from above into a small texture that moves with
// the boat: the churned water behind the stern (white foam, then a trail
// of bubbles), the two arms of the V that spread from the bow (ridges of
// water with a little foam), and the white water along the hull. The
// sea's shader raises the ridges, lights them, and turns the rest into
// foam and paler water.

import * as THREE from "three";
import { clamp } from "./noise";
import { LENGTH, MAX_SPEED, halfWidth, type BoatState } from "./boat";

export const WAKE_SIZE = 240; // metres across the texture
const RES = 512;
const LIFE = 16; // seconds a bit of wake lasts
const MAX_QUADS = 3200;
type Mark = { x: number; z: number; fx: number; fz: number; v: number; t: number };

export class Wake {
  readonly target = new THREE.WebGLRenderTarget(RES, RES, { type: THREE.UnsignedByteType, depthBuffer: false });
  readonly center = new THREE.Vector2();
  private marks: Mark[] = [];
  private last = -1;
  private scene = new THREE.Scene();
  private cam = new THREE.OrthographicCamera(-WAKE_SIZE / 2, WAKE_SIZE / 2, WAKE_SIZE / 2, -WAKE_SIZE / 2, -10, 10);
  private geo = new THREE.BufferGeometry();
  private pos = new Float32Array(6 * 3 * MAX_QUADS);
  private val = new Float32Array(6 * 3 * MAX_QUADS); // foam, bubbles, ridge
  private across = new Float32Array(6 * MAX_QUADS);
  private along = new Float32Array(6 * MAX_QUADS);
  private n = 0;

  constructor() {
    this.target.texture.minFilter = this.target.texture.magFilter = THREE.LinearFilter;
    const dyn = (a: Float32Array, k: number) => new THREE.BufferAttribute(a, k).setUsage(THREE.DynamicDrawUsage);
    this.geo.setAttribute("position", dyn(this.pos, 3));
    this.geo.setAttribute("v", dyn(this.val, 3));
    this.geo.setAttribute("c", dyn(this.across, 1));
    this.geo.setAttribute("a", dyn(this.along, 1));
    const mat = new THREE.ShaderMaterial({
      vertexShader: "attribute vec3 v; attribute float c; attribute float a; varying vec3 vv; varying float vc; varying float va; void main(){ vv = v; vc = c; va = a; gl_Position = projectionMatrix * viewMatrix * vec4(position, 1.0); }",
      // red: foam, soft-edged; green: bubbles under the surface; blue: a
      // ridge of water (0.5 is flat), highest along the middle, in a row of crests
      fragmentShader: `varying vec3 vv; varying float vc; varying float va;
        void main(){
          float edge = sqrt(max(0.0, 1.0 - vc * vc));
          float ridge = cos(clamp(vc, -1.0, 1.0) * 1.5708); ridge *= ridge;
          float crests = 0.55 + 0.45 * sin(va + 0.9 * sin(va * 0.37));
          gl_FragColor = vec4(vv.x * edge, vv.y * edge, 0.5 + 0.5 * vv.z * ridge * crests, 1.0);
        }`,
      blending: THREE.CustomBlending, blendEquation: THREE.MaxEquation, blendSrc: THREE.OneFactor, blendDst: THREE.OneFactor,
      depthTest: false, depthWrite: false, side: THREE.DoubleSide,
    });
    const mesh = new THREE.Mesh(this.geo, mat);
    mesh.frustumCulled = false;
    this.scene.add(mesh);
    this.cam.up.set(0, 0, -1);
  }

  reset() { this.marks = []; this.last = -1; }

  /** Adds where the boat is now (a mark every tenth of a second). */
  add(b: BoatState, t: number) {
    while (this.marks.length && t - this.marks[0].t > LIFE) this.marks.shift();
    if (t - this.last < 0.1) return;
    this.last = t;
    this.marks.push({ x: b.x, z: b.z, fx: Math.sin(b.heading), fz: -Math.cos(b.heading), v: Math.max(0, b.speed), t });
  }

  /** A band from a to b (half widths wa, wb), with foam, bubbles and ridge at each end. */
  private band(ax: number, az: number, bx: number, bz: number, wa: number, wb: number, va: number[], vb: number[], la: number, lb: number) {
    if (this.n >= MAX_QUADS || Math.max(...va, ...vb) <= 0.003) return;
    let dx = bx - ax, dz = bz - az;
    const len = Math.hypot(dx, dz) || 1;
    dx /= len; dz /= len;
    const px = -dz, pz = dx;
    const o = this.n * 18, w = this.n * 6, c3 = this.n * 18;
    const A = [ax + px * wa, az + pz * wa], B = [ax - px * wa, az - pz * wa], C = [bx + px * wb, bz + pz * wb], D = [bx - px * wb, bz - pz * wb];
    this.pos.set([A[0], 0, A[1], B[0], 0, B[1], C[0], 0, C[1], C[0], 0, C[1], B[0], 0, B[1], D[0], 0, D[1]], o);
    this.val.set([...va, ...va, ...vb, ...vb, ...va, ...vb], c3);
    this.across.set([1, -1, 1, 1, -1, -1], w);
    this.along.set([la, la, lb, lb, la, lb], w);
    this.n++;
  }

  /** Draws the wake into the texture around the boat. */
  render(r: THREE.WebGLRenderer, b: BoatState, t: number) {
    const px = WAKE_SIZE / RES;
    this.center.set(Math.round(b.x / px) * px, Math.round(b.z / px) * px);
    this.cam.position.set(this.center.x, 5, this.center.y);
    this.cam.lookAt(this.center.x, 0, this.center.y);
    this.n = 0;
    const cur: Mark = { x: b.x, z: b.z, fx: Math.sin(b.heading), fz: -Math.cos(b.heading), v: Math.max(0, b.speed), t };
    const all = [...this.marks, cur];
    const at = (m: Mark, s: number) => [m.x + m.fx * LENGTH * s, m.z + m.fz * LENGTH * s];
    const sp = (m: Mark) => clamp(m.v / MAX_SPEED, 0, 1);
    let dist = 0;
    for (let i = all.length - 1; i > 0; i--) {
      const c = all[i], a = all[i - 1];
      const ageC = t - c.t, ageA = t - a.t;
      const sc = sp(c), sa = sp(a);
      const step = Math.hypot(c.x - a.x, c.z - a.z);
      const dC = dist, dA = dist + step;
      dist = dA;
      // behind the stern: white churn at first, then a trail of bubbles
      const [cx, cz] = at(c, -0.5), [ax, az] = at(a, -0.5);
      const churn = (age: number, s: number) => [
        Math.min(1, s * 2.6 + (s > 0.04 ? 0.15 : 0)) * Math.pow(clamp(1 - age / (LIFE * 0.3), 0, 1), 1.6),
        Math.min(1, s * 2.2 + (s > 0.04 ? 0.2 : 0)) * Math.pow(clamp(1 - age / LIFE, 0, 1), 1.2),
        0,
      ];
      const wid = (age: number, s: number) => 0.9 + s * 0.7 + age * (0.35 + s * 0.45);
      this.band(cx, cz, ax, az, wid(ageC, sc), wid(ageA, sa), churn(ageC, sc), churn(ageA, sa), dC, dA);
      // a wider, fainter scar of bubbles around the churn
      const scar = (age: number, s: number) => [0, Math.min(0.55, s * 1.4) * Math.pow(clamp(1 - age / LIFE, 0, 1), 1.5), 0];
      this.band(cx, cz, ax, az, wid(ageC, sc) * 1.9 + 1, wid(ageA, sa) * 1.9 + 1, scar(ageC, sc), scar(ageA, sa), dC, dA);
      // the V from the bow: each arm a ridge moving out at about 19.5° to the track
      for (const side of [1, -1]) {
        const [bx, bz] = at(c, 0.42), [dx, dz] = at(a, 0.42);
        const oc = ageC * c.v * 0.354, oa = ageA * a.v * 0.354;
        const ex = bx - c.fz * side * oc, ez = bz + c.fx * side * oc;
        const gx = dx - a.fz * side * oa, gz = dz + a.fx * side * oa;
        const arm = (age: number, s: number) => {
          const life = Math.pow(clamp(1 - age / (LIFE * 0.7), 0, 1), 1.6) * clamp(age * 3, 0, 1);
          return [Math.min(0.85, s * 2) * life * Math.pow(clamp(1 - age / 6, 0, 1), 1.3), Math.min(0.7, s * 1.8) * life, Math.min(1, s * 2.6) * life];
        };
        const aw = (age: number) => 1.5 + age * 0.45;
        // crests along the arm: about one every 3 m
        this.band(ex, ez, gx, gz, aw(ageC), aw(ageA), arm(ageC, sc), arm(ageA, sa), dC * 2.1, dA * 2.1);
      }
    }
    // white water pushed up along the hull, from the bow back
    const s0 = sp(cur);
    if (s0 > 0.02) {
      for (const side of [1, -1]) {
        const pts = [0.9, 0.72, 0.52, 0.32].map((s) => {
          const off = halfWidth(s) + 0.15 + (0.9 - s) * 0.9;
          const [x, z] = at(cur, s - 0.5);
          return [x - cur.fz * side * off, z + cur.fx * side * off, s];
        });
        for (let i = 0; i < pts.length - 1; i++) {
          const f = (s: number) => [Math.min(1, s0 * 2.4) * (0.5 + 0.5 * s), Math.min(1, s0 * 2), Math.min(0.8, s0 * 2.2) * s];
          this.band(pts[i][0], pts[i][1], pts[i + 1][0], pts[i + 1][1], 0.35 + (0.9 - pts[i][2]) * 0.5, 0.35 + (0.9 - pts[i + 1][2]) * 0.5, f(pts[i][2]), f(pts[i + 1][2]), i * 2, i * 2 + 2);
        }
      }
    }
    const n = this.n;
    this.geo.setDrawRange(0, n * 6);
    for (const k of ["position", "v", "c", "a"]) (this.geo.attributes[k] as THREE.BufferAttribute).needsUpdate = true;
    const prev = r.getRenderTarget(), clear = r.getClearColor(new THREE.Color()), alpha = r.getClearAlpha();
    r.setRenderTarget(this.target);
    r.setClearColor(new THREE.Color(0, 0, 0.5), 1);
    r.clear(true, false, false);
    r.render(this.scene, this.cam);
    r.setRenderTarget(prev);
    r.setClearColor(clear, alpha);
  }
  dispose() { this.target.dispose(); this.geo.dispose(); }
}
