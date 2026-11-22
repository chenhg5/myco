// The sea's waves: a sum of Gerstner waves (each a circle of motion of a
// given length, height and direction) shaped by the sea state and the
// wind. The same waves move the water on the GPU (ocean.ts) and here lift
// the boat and the wake, so what floats sits on what you see.

import { rng } from "./noise";

export type Sea = "calm" | "moderate" | "rough";
export type Wave = { dx: number; dz: number; k: number; w: number; a: number; q: number; p: number };

/** How each sea state looks: the longest waves, their height, how sharp
 *  the crests are (0–1) and how readily they break into foam. */
export const SEAS: Record<Sea, { longest: number; height: number; chop: number; foam: number }> = {
  calm: { longest: 34, height: 0.45, chop: 0.45, foam: 0.12 },
  moderate: { longest: 58, height: 1, chop: 0.7, foam: 0.5 },
  rough: { longest: 90, height: 1.9, chop: 0.82, foam: 0.95 },
};

export const GEO_WAVES = 22; // shape the surface (and carry the boat)

const G = 9.81;

/** Waves running away from the wind (from `windDeg`, 0 = north, clockwise),
 *  longest first, then finer and more spread out. Seeded: the same sea
 *  state gives the same sea for everyone. */
export function makeWaves(sea: Sea, windDeg: number, count = GEO_WAVES, shortest = 1.2, longest?: number, seed = 7): Wave[] {
  const s = SEAS[sea] || SEAS.moderate;
  const r = rng(seed);
  const L0 = longest ?? s.longest;
  const ratio = Math.pow(shortest / L0, 1 / Math.max(1, count - 1));
  // north = -z, east = +x; the waves go where the wind goes
  const toward = ((windDeg + 180) * Math.PI) / 180;
  const out: Wave[] = [];
  for (let i = 0; i < count; i++) {
    const L = L0 * Math.pow(ratio, i);
    const spread = 0.35 + 0.9 * (i / count);
    const ang = toward + (r() * 2 - 1) * spread;
    const k = (2 * Math.PI) / L;
    out.push({ dx: Math.sin(ang), dz: -Math.cos(ang), k, w: Math.sqrt(G * k), a: 0.0052 * L * s.height * (0.7 + 0.6 * r()), q: 0, p: r() * Math.PI * 2 });
  }
  // the crests' sharpness, shared out so that they never fold over
  const ka = out.reduce((t, w) => t + w.k * w.a, 0);
  for (const w of out) w.q = s.chop / Math.max(ka, 1e-6);
  return out;
}

/** Where the surface point that started at (x, z) is at time t. */
export function displace(waves: Wave[], x: number, z: number, t: number): [number, number, number] {
  let px = x, py = 0, pz = z;
  for (const w of waves) {
    const th = w.k * (w.dx * x + w.dz * z) - w.w * t + w.p;
    const c = Math.cos(th), s = Math.sin(th);
    px += w.q * w.a * w.dx * c;
    pz += w.q * w.a * w.dz * c;
    py += w.a * s;
  }
  return [px, py, pz];
}

/** The water's height above the point (x, z) at time t (the waves also
 *  move the water sideways, so the point under it is found by steps). */
export function heightAt(waves: Wave[], x: number, z: number, t: number) {
  let sx = x, sz = z;
  for (let i = 0; i < 3; i++) {
    const [px, , pz] = displace(waves, sx, sz, t);
    sx += x - px;
    sz += z - pz;
  }
  return displace(waves, sx, sz, t)[1];
}

/** The waves packed for the shader: per wave (dx, dz, k, ω) and (a, q, phase, 0). */
export function packWaves(waves: Wave[], n: number) {
  const A = new Float32Array(n * 4), B = new Float32Array(n * 4);
  waves.slice(0, n).forEach((w, i) => {
    A.set([w.dx, w.dz, w.k, w.w], i * 4);
    B.set([w.a, w.q, w.p, 0], i * 4);
  });
  return { A, B };
}
