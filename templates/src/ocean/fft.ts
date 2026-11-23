// The sea computed as an ocean scientist would (Tessendorf's FFT ocean):
// a wave spectrum for the wind (how much energy each wavelength and
// direction holds), moved on in time, and turned into a patch of sea by
// an inverse FFT on the GPU every frame: height, the sideways push that
// makes crests sharp, the slopes for light, and where the surface is
// squeezed (it breaks: foam). The foam is kept from frame to frame and
// fades, so breaking crests leave streaks. The long waves the boat rides
// are summed here on the CPU from the same spectrum.

import * as THREE from "three";
import { rng } from "./noise";
import type { Sea } from "./waves";

export const FFT_N = 256;
export const TILE = 256; // metres the patch covers (it repeats)

/** Per sea state: wind speed (shapes the spectrum), significant wave height,
 *  how strongly crests are pushed together, and how squeezed the surface
 *  must be to break into foam (measured: about 1% of a calm sea, 5% of a
 *  moderate one and 8% of a rough one gets there). */
export const SPECTRA: Record<Sea, { wind: number; hs: number; chop: number; foam: number }> = {
  calm: { wind: 5, hs: 0.45, chop: 0.75, foam: 0.855 },
  moderate: { wind: 9, hs: 1.3, chop: 1.15, foam: 0.8 },
  rough: { wind: 14, hs: 2.8, chop: 1.3, foam: 0.835 },
};

const G = 9.81;
type Mode = { kx: number; kz: number; k: number; w: number; a: number; b: number; c: number; d: number };

/** The starting amplitudes h0(k) (and conj h0(−k)) for every wave of the patch. */
export function spectrum(sea: Sea, windDeg: number, n = FFT_N, tile = TILE, seed = 4242) {
  const s = SPECTRA[sea] || SPECTRA.moderate;
  const toward = ((windDeg + 180) * Math.PI) / 180;
  const wx = Math.sin(toward), wz = -Math.cos(toward); // north = −z
  const Lw = (s.wind * s.wind) / G, small = Lw * 0.0015;
  const r = rng(seed);
  const gauss = () => { const u = Math.max(1e-9, r()), v = r(); return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v); };
  const amp = new Float32Array(n * n * 2); // h0(k) for each texel, random phase × √P(k)
  const P = (kx: number, kz: number) => {
    const k = Math.hypot(kx, kz);
    if (k < 1e-6) return 0;
    const dot = (kx * wx + kz * wz) / k;
    let p = Math.exp(-1 / (k * Lw) ** 2) / k ** 4 * dot * dot * Math.exp(-(k * k) * small * small);
    if (dot < 0) p *= 0.07; // little runs against the wind
    return p;
  };
  const kOf = (i: number) => (2 * Math.PI * (i < n / 2 ? i : i - n)) / tile;
  for (let j = 0; j < n; j++) for (let i = 0; i < n; i++) {
    const p = Math.sqrt(P(kOf(i), kOf(j)) / 2);
    amp[(j * n + i) * 2] = gauss() * p;
    amp[(j * n + i) * 2 + 1] = gauss() * p;
  }
  // scale to the wanted height: the variance is the sum of |h0(k)|² + |h0(−k)|²
  let v = 0;
  for (let i = 0; i < n * n; i++) v += 2 * (amp[i * 2] ** 2 + amp[i * 2 + 1] ** 2);
  const scale = s.hs / 4 / Math.sqrt(Math.max(v, 1e-12));
  for (let i = 0; i < amp.length; i++) amp[i] *= scale;
  // (h0(k).re, h0(k).im, conj h0(−k).re, conj h0(−k).im)
  const tex = new Float32Array(n * n * 4);
  for (let j = 0; j < n; j++) for (let i = 0; i < n; i++) {
    const o = (j * n + i), mi = (n - i) % n, mj = (n - j) % n, m = mj * n + mi;
    tex.set([amp[o * 2], amp[o * 2 + 1], amp[m * 2], -amp[m * 2 + 1]], o * 4);
  }
  // the long waves, for the boat (and anyone asking for the height here)
  const modes: Mode[] = [];
  const kcut = (2 * Math.PI) / 10; // waves longer than 10 m (all a 7 m boat feels)
  for (let j = 0; j < n; j++) for (let i = 0; i < n; i++) {
    const kx = kOf(i), kz = kOf(j), k = Math.hypot(kx, kz);
    if (k < 1e-6 || k > kcut) continue;
    const t = tex.subarray((j * n + i) * 4, (j * n + i) * 4 + 4);
    modes.push({ kx, kz, k, w: Math.sqrt(G * k), a: t[0], b: t[1], c: t[2], d: t[3] });
  }
  return { tex, modes, chop: s.chop, foam: s.foam };
}

/** h(k, t) for the long waves, then the surface's height at (x, z) (the
 *  crests push water sideways, so the point under (x, z) is found in steps). */
export class LongWaves {
  private hr = new Float32Array(0);
  private hi = new Float32Array(0);
  private at = NaN;
  constructor(private modes: Mode[], private chop: number) {}
  private prepare(t: number) {
    if (t === this.at) return;
    const m = this.modes;
    if (this.hr.length !== m.length) { this.hr = new Float32Array(m.length); this.hi = new Float32Array(m.length); }
    for (let i = 0; i < m.length; i++) {
      const c = Math.cos(m[i].w * t), s = Math.sin(m[i].w * t);
      // h0 e^{iωt} + conj(h0(−k)) e^{−iωt}
      this.hr[i] = m[i].a * c - m[i].b * s + m[i].c * c + m[i].d * s;
      this.hi[i] = m[i].a * s + m[i].b * c - m[i].c * s + m[i].d * c;
    }
    this.at = t;
  }
  /** (height, sideways x, sideways z) of the surface point that started at (x, z). */
  displace(x: number, z: number, t: number): [number, number, number] {
    this.prepare(t);
    const m = this.modes;
    let h = 0, dx = 0, dz = 0;
    for (let i = 0; i < m.length; i++) {
      const ph = m[i].kx * x + m[i].kz * z, c = Math.cos(ph), s = Math.sin(ph);
      const re = this.hr[i] * c - this.hi[i] * s, im = this.hr[i] * s + this.hi[i] * c;
      h += re;
      // −i k̂ h e^{ik·x}: its real part is k̂ · im
      dx += (m[i].kx / m[i].k) * im;
      dz += (m[i].kz / m[i].k) * im;
    }
    return [h, this.chop * dx, this.chop * dz];
  }
  heightAt(x: number, z: number, t: number) {
    let sx = x, sz = z;
    for (let i = 0; i < 2; i++) { const [, dx, dz] = this.displace(sx, sz, t); sx = x - dx; sz = z - dz; }
    return this.displace(sx, sz, t)[0];
  }
}

const QUAD_VS = "void main(){ gl_Position = vec4(position.xy, 0.0, 1.0); }";
const CMUL = "vec2 cmul(vec2 a, vec2 b){ return vec2(a.x * b.x - a.y * b.y, a.x * b.y + a.y * b.x); }\n";

/** The GPU side: spectrum → FFT → the sea's textures (displacement, slopes, foam). */
export class FFTOcean {
  readonly disp: THREE.WebGLRenderTarget;
  readonly slope: THREE.WebGLRenderTarget;
  private foam: THREE.WebGLRenderTarget[];
  private foamAt = 0;
  private h0: THREE.DataTexture;
  private work: THREE.WebGLRenderTarget[];
  private scene = new THREE.Scene();
  private cam = new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1);
  private quad: THREE.Mesh;
  private mats: Record<string, THREE.ShaderMaterial>;
  long = new LongWaves([], 1);
  chop = 1;
  foamRate = 0.815;

  /** Whether this browser can do it (render to float textures). */
  static supported(r: THREE.WebGLRenderer) {
    return r.capabilities.isWebGL2 && r.extensions.has("EXT_color_buffer_float");
  }

  constructor(private r: THREE.WebGLRenderer, readonly n = FFT_N) {
    const float = (filter: THREE.TextureFilter = THREE.NearestFilter) => new THREE.WebGLRenderTarget(n, n, { type: THREE.FloatType, format: THREE.RGBAFormat, minFilter: filter, magFilter: filter as THREE.MagnificationTextureFilter, depthBuffer: false, generateMipmaps: false });
    const half = (mips: boolean) => {
      const t = new THREE.WebGLRenderTarget(n, n, { type: THREE.HalfFloatType, format: THREE.RGBAFormat, minFilter: mips ? THREE.LinearMipmapLinearFilter : THREE.LinearFilter, magFilter: THREE.LinearFilter, depthBuffer: false, generateMipmaps: mips });
      t.texture.wrapS = t.texture.wrapT = THREE.RepeatWrapping;
      return t;
    };
    this.work = [float(), float(), float(), float()];
    this.disp = half(false);
    this.slope = half(true);
    this.foam = [half(true), half(true)];
    this.h0 = new THREE.DataTexture(new Float32Array(n * n * 4), n, n, THREE.RGBAFormat, THREE.FloatType);
    this.h0.needsUpdate = true;
    const mat = (fs: string, uniforms: Record<string, THREE.IUniform>) => new THREE.ShaderMaterial({ vertexShader: QUAD_VS, fragmentShader: fs, uniforms, depthTest: false, depthWrite: false });
    this.mats = {
      // h(k, t) for four fields, two to a texture: (Dx + iH, Dz + i∂H/∂x) and (∂H/∂z + i∂Dx/∂x, ∂Dz/∂z + i∂Dx/∂z)
      spec: mat(CMUL + `
        uniform sampler2D uH0; uniform float uT, uN, uL; uniform int uPart;
        void main() {
          vec2 n = floor(gl_FragCoord.xy);
          vec2 m = vec2(n.x < uN * 0.5 ? n.x : n.x - uN, n.y < uN * 0.5 ? n.y : n.y - uN);
          vec2 k = 6.2831853 * m / uL;
          float kl = length(k);
          if (kl < 1e-6) { gl_FragColor = vec4(0.0); return; }
          float w = sqrt(9.81 * kl);
          vec4 h0 = texture2D(uH0, (n + 0.5) / uN);
          float c = cos(w * uT), s = sin(w * uT);
          vec2 h = cmul(h0.xy, vec2(c, s)) + cmul(h0.zw, vec2(c, -s));
          vec2 ih = vec2(-h.y, h.x);
          vec2 A, B, C, D;
          if (uPart == 0) { A = -(k.x / kl) * ih; B = h; C = -(k.y / kl) * ih; D = k.x * ih; }
          else { A = k.y * ih; B = (k.x * k.x / kl) * h; C = (k.y * k.y / kl) * h; D = (k.x * k.y / kl) * h; }
          // two real fields in one complex number: A + iB
          gl_FragColor = vec4(A.x - B.y, A.y + B.x, C.x - D.y, C.y + D.x);
        }`, { uH0: { value: this.h0 }, uT: { value: 0 }, uN: { value: n }, uL: { value: TILE }, uPart: { value: 0 } }),
      // one radix-2 stage of an inverse FFT (Stockham: the result lands in order)
      fft: mat(CMUL + `
        uniform sampler2D uIn; uniform float uN, uSize; uniform int uHoriz;
        void main() {
          vec2 fc = floor(gl_FragCoord.xy);
          float index = uHoriz == 1 ? fc.x : fc.y;
          float half_ = uSize * 0.5;
          float even = floor(index / uSize) * half_ + mod(index, half_);
          vec2 ce = uHoriz == 1 ? vec2(even, fc.y) : vec2(fc.x, even);
          vec2 co = uHoriz == 1 ? vec2(even + uN * 0.5, fc.y) : vec2(fc.x, even + uN * 0.5);
          vec4 e = texture2D(uIn, (ce + 0.5) / uN), o = texture2D(uIn, (co + 0.5) / uN);
          float a = 6.2831853 * index / uSize;
          vec2 tw = vec2(cos(a), sin(a));
          gl_FragColor = vec4(e.xy + cmul(tw, o.xy), e.zw + cmul(tw, o.zw));
        }`, { uIn: { value: null }, uN: { value: n }, uSize: { value: 2 }, uHoriz: { value: 1 } }),
      // the sea's textures from the four fields
      out: mat(`
        uniform sampler2D uA, uB; uniform float uN, uChop; uniform int uWhich;
        void main() {
          vec2 uv = (floor(gl_FragCoord.xy) + 0.5) / uN;
          vec4 a = texture2D(uA, uv), b = texture2D(uB, uv);
          if (uWhich == 0) { gl_FragColor = vec4(uChop * a.x, a.y, uChop * a.z, 0.0); return; }
          float jxx = 1.0 + uChop * b.y, jzz = 1.0 + uChop * b.z, jxz = uChop * b.w;
          float J = jxx * jzz - jxz * jxz;
          // slopes of the displaced surface, and how squeezed it is
          gl_FragColor = vec4(a.w / max(jxx, 0.1), b.x / max(jzz, 0.1), J, 0.0);
        }`, { uA: { value: null }, uB: { value: null }, uN: { value: n }, uChop: { value: 1 }, uWhich: { value: 0 } }),
      // foam: made where the surface folds, kept, fading
      foam: mat(`
        uniform sampler2D uPrev, uSlope; uniform float uN, uDt, uRate;
        void main() {
          vec2 uv = (floor(gl_FragCoord.xy) + 0.5) / uN;
          float J = texture2D(uSlope, uv).z;
          // (it lasts a few seconds: crests moving on leave streaks behind)
          float kept = texture2D(uPrev, uv).r * exp(-uDt * 0.32);
          float made = smoothstep(uRate, uRate - 0.1, J);
          gl_FragColor = vec4(clamp(max(kept, made), 0.0, 1.0), 0.0, 0.0, 1.0);
        }`, { uPrev: { value: null }, uSlope: { value: null }, uN: { value: n }, uDt: { value: 0 }, uRate: { value: 0.815 } }),
    };
    this.quad = new THREE.Mesh(new THREE.PlaneGeometry(2, 2), this.mats.spec);
    this.quad.frustumCulled = false;
    this.scene.add(this.quad);
  }

  get foamTexture() { return this.foam[this.foamAt].texture; }

  setSea(sea: Sea, windDeg: number) {
    const s = spectrum(sea, windDeg, this.n);
    (this.h0.image.data as unknown as Float32Array).set(s.tex);
    this.h0.needsUpdate = true;
    this.long = new LongWaves(s.modes, s.chop);
    this.chop = s.chop;
    this.foamRate = s.foam;
  }

  private pass(m: THREE.ShaderMaterial, target: THREE.WebGLRenderTarget) {
    this.quad.material = m;
    this.r.setRenderTarget(target);
    this.r.render(this.scene, this.cam);
  }

  /** The sea at time t (dt since the last call, for the foam's fading). */
  update(t: number, dt: number) {
    const prev = this.r.getRenderTarget();
    const auto = this.r.autoClear;
    this.r.autoClear = false;
    const m = this.mats, [a0, a1, b0, b1] = this.work;
    m.spec.uniforms.uT.value = t;
    m.spec.uniforms.uPart.value = 0; this.pass(m.spec, a0);
    m.spec.uniforms.uPart.value = 1; this.pass(m.spec, b0);
    const steps = Math.log2(this.n);
    const run = (src: THREE.WebGLRenderTarget, dst: THREE.WebGLRenderTarget) => {
      let from = src, to = dst;
      for (const horiz of [1, 0]) for (let s = 0; s < steps; s++) {
        m.fft.uniforms.uIn.value = from.texture;
        m.fft.uniforms.uSize.value = 2 ** (s + 1);
        m.fft.uniforms.uHoriz.value = horiz;
        this.pass(m.fft, to);
        [from, to] = [to, from];
      }
      return from; // (an even number of passes: back in src)
    };
    const A = run(a0, a1), B = run(b0, b1);
    m.out.uniforms.uA.value = A.texture; m.out.uniforms.uB.value = B.texture; m.out.uniforms.uChop.value = this.chop;
    m.out.uniforms.uWhich.value = 0; this.pass(m.out, this.disp);
    m.out.uniforms.uWhich.value = 1; this.pass(m.out, this.slope);
    const next = 1 - this.foamAt;
    m.foam.uniforms.uPrev.value = this.foam[this.foamAt].texture;
    m.foam.uniforms.uSlope.value = this.slope.texture;
    m.foam.uniforms.uDt.value = Math.min(dt, 0.25);
    m.foam.uniforms.uRate.value = this.foamRate;
    this.pass(m.foam, this.foam[next]);
    this.foamAt = next;
    this.r.setRenderTarget(prev);
    this.r.autoClear = auto;
  }

  dispose() {
    for (const t of [...this.work, this.disp, this.slope, ...this.foam]) t.dispose();
    this.h0.dispose();
    for (const m of Object.values(this.mats)) m.dispose();
    this.quad.geometry.dispose();
  }
}
