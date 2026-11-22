// The boat's wake, drawn from above into a small texture that moves with
// the boat: the churned water behind the stern and the two arms of the V
// that spread from the bow. The sea's shader turns it into foam.

import * as THREE from "three";
import { clamp } from "./noise";
import { LENGTH, MAX_SPEED, type BoatState } from "./boat";

export const WAKE_SIZE = 240; // metres across the texture
const RES = 512;
const LIFE = 14; // seconds a bit of wake lasts
type Mark = { x: number; z: number; fx: number; fz: number; v: number; t: number };

export class Wake {
  readonly target = new THREE.WebGLRenderTarget(RES, RES, { type: THREE.UnsignedByteType, depthBuffer: false });
  readonly center = new THREE.Vector2();
  private marks: Mark[] = [];
  private last = -1;
  private scene = new THREE.Scene();
  private cam = new THREE.OrthographicCamera(-WAKE_SIZE / 2, WAKE_SIZE / 2, WAKE_SIZE / 2, -WAKE_SIZE / 2, -10, 10);
  private geo = new THREE.BufferGeometry();
  private pos = new Float32Array(6 * 3 * 2400);
  private val = new Float32Array(6 * 2400);
  private across = new Float32Array(6 * 2400);

  constructor() {
    this.target.texture.minFilter = this.target.texture.magFilter = THREE.LinearFilter;
    this.geo.setAttribute("position", new THREE.BufferAttribute(this.pos, 3).setUsage(THREE.DynamicDrawUsage));
    this.geo.setAttribute("v", new THREE.BufferAttribute(this.val, 1).setUsage(THREE.DynamicDrawUsage));
    this.geo.setAttribute("c", new THREE.BufferAttribute(this.across, 1).setUsage(THREE.DynamicDrawUsage));
    const mat = new THREE.ShaderMaterial({
      vertexShader: "attribute float v; attribute float c; varying float vv; varying float vc; void main(){ vv = v; vc = c; gl_Position = projectionMatrix * viewMatrix * vec4(position, 1.0); }",
      // soft edges: strongest along the middle of each band
      fragmentShader: "varying float vv; varying float vc; void main(){ gl_FragColor = vec4(vv * sqrt(max(0.0, 1.0 - vc * vc)), 0.0, 0.0, 1.0); }",
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

  /** Draws the wake into the texture around the boat. */
  render(r: THREE.WebGLRenderer, b: BoatState, t: number) {
    const px = WAKE_SIZE / RES;
    this.center.set(Math.round(b.x / px) * px, Math.round(b.z / px) * px);
    this.cam.position.set(this.center.x, 5, this.center.y);
    this.cam.lookAt(this.center.x, 0, this.center.y);
    let n = 0;
    const quad = (ax: number, az: number, bx: number, bz: number, wa: number, wb: number, va: number, vb: number, dxa: number, dza: number, dxb: number, dzb: number) => {
      if (n >= 2400 || (va <= 0.002 && vb <= 0.002)) return;
      // perpendicular at each end
      const pa = [-dza * wa, dxa * wa], pb = [-dzb * wb, dxb * wb];
      const p = this.pos, v = this.val, o = n * 18, w = n * 6;
      const A = [ax + pa[0], az + pa[1]], B = [ax - pa[0], az - pa[1]], C = [bx + pb[0], bz + pb[1]], D = [bx - pb[0], bz - pb[1]];
      p.set([A[0], 0, A[1], B[0], 0, B[1], C[0], 0, C[1], C[0], 0, C[1], B[0], 0, B[1], D[0], 0, D[1]], o);
      v.set([va, va, vb, vb, va, vb], w);
      this.across.set([1, -1, 1, 1, -1, -1], w);
      n++;
    };
    const ms = this.marks;
    const cur: Mark = { x: b.x, z: b.z, fx: Math.sin(b.heading), fz: -Math.cos(b.heading), v: Math.max(0, b.speed), t };
    const all = [...ms, cur];
    const stern = (m: Mark) => [m.x - m.fx * LENGTH * 0.5, m.z - m.fz * LENGTH * 0.5];
    const bow = (m: Mark) => [m.x + m.fx * LENGTH * 0.42, m.z + m.fz * LENGTH * 0.42];
    for (let i = 0; i < all.length - 1; i++) {
      const a = all[i], c = all[i + 1];
      const ageA = t - a.t, ageC = t - c.t;
      const sa = clamp(a.v / MAX_SPEED, 0, 1), sc = clamp(c.v / MAX_SPEED, 0, 1);
      const life = (age: number) => Math.pow(clamp(1 - age / LIFE, 0, 1), 1.6);
      // the churned water behind the stern: widening, fading
      const [ax, az] = stern(a), [cx, cz] = stern(c);
      const wA = 1.1 + ageA * (0.45 + sa * 0.6), wC = 1.1 + ageC * (0.45 + sc * 0.6);
      quad(ax, az, cx, cz, wA, wC, Math.min(1, sa * 2.2 + 0.08 * (a.v > 0.5 ? 1 : 0)) * life(ageA), Math.min(1, sc * 2.2) * life(ageC), a.fx, a.fz, c.fx, c.fz);
      // the V from the bow: each arm moves out at about 19.5° to the track
      for (const side of [1, -1]) {
        const [bx, bz] = bow(a), [dx, dz] = bow(c);
        const oa = ageA * a.v * 0.354, oc = ageC * c.v * 0.354;
        const ex = bx + -a.fz * side * oa - a.fx * ageA * 0.2, ez = bz + a.fx * side * oa - a.fz * ageA * 0.2;
        const gx = dx + -c.fz * side * oc - c.fx * ageC * 0.2, gz = dz + c.fx * side * oc - c.fz * ageC * 0.2;
        const vv = (age: number, s: number) => Math.min(0.55, s * 1.6) * Math.pow(clamp(1 - age / (LIFE * 0.6), 0, 1), 2) * clamp(age * 4, 0, 1);
        quad(ex, ez, gx, gz, 0.5 + ageA * 0.12, 0.5 + ageC * 0.12, vv(ageA, sa), vv(ageC, sc), a.fx, a.fz, c.fx, c.fz);
      }
    }
    this.geo.setDrawRange(0, n * 6);
    (this.geo.attributes.position as THREE.BufferAttribute).needsUpdate = true;
    (this.geo.attributes.v as THREE.BufferAttribute).needsUpdate = true;
    (this.geo.attributes.c as THREE.BufferAttribute).needsUpdate = true;
    const prev = r.getRenderTarget(), clear = r.getClearColor(new THREE.Color()), alpha = r.getClearAlpha();
    r.setRenderTarget(this.target);
    r.setClearColor(0x000000, 1);
    r.clear(true, false, false);
    r.render(this.scene, this.cam);
    r.setRenderTarget(prev);
    r.setClearColor(clear, alpha);
  }
  dispose() { this.target.dispose(); this.geo.dispose(); }
}
