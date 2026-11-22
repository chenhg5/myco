// Seeded noise for shaping islands (here, not on the GPU: the boat runs
// aground on the same ground the eye sees).

/** A hash of two integers and a seed, in [0, 1). */
function hash(x: number, y: number, seed: number) {
  let h = (x * 374761393 + y * 668265263 + seed * 2147483647) | 0;
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  h ^= h >>> 16;
  return (h >>> 0) / 4294967296;
}
const fade = (t: number) => t * t * (3 - 2 * t);

/** Smooth value noise in [0, 1). */
export function noise2(x: number, y: number, seed = 0) {
  const xi = Math.floor(x), yi = Math.floor(y);
  const fx = fade(x - xi), fy = fade(y - yi);
  const a = hash(xi, yi, seed), b = hash(xi + 1, yi, seed), c = hash(xi, yi + 1, seed), d = hash(xi + 1, yi + 1, seed);
  return a + (b - a) * fx + (c - a) * fy + (a - b - c + d) * fx * fy;
}

/** Fractal noise in about [0, 1). */
export function fbm(x: number, y: number, seed = 0, octaves = 5) {
  let s = 0, amp = 0.5, f = 1, norm = 0;
  for (let i = 0; i < octaves; i++) { s += amp * noise2(x * f, y * f, seed + i * 17); norm += amp; amp *= 0.5; f *= 2.03; }
  return s / norm;
}

/** Ridged fractal noise (sharp crests, like weathered rock), in [0, 1). */
export function ridged(x: number, y: number, seed = 0, octaves = 5) {
  let s = 0, amp = 0.5, f = 1, norm = 0;
  for (let i = 0; i < octaves; i++) { const n = 1 - Math.abs(noise2(x * f, y * f, seed + i * 31) * 2 - 1); s += amp * n * n; norm += amp; amp *= 0.5; f *= 2.1; }
  return s / norm;
}

/** A small seeded random generator. */
export function rng(seed: number) {
  let s = (seed | 0) || 1;
  return () => { s = Math.imul(s ^ (s >>> 15), 2246822507) + 0x9e3779b9 | 0; s ^= s >>> 13; s = Math.imul(s, 3266489909); s ^= s >>> 16; return (s >>> 0) / 4294967296; };
}

export const smoothstep = (a: number, b: number, x: number) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };
export const clamp = (x: number, a: number, b: number) => Math.min(b, Math.max(a, x));
export const lerp = (a: number, b: number, t: number) => a + (b - a) * t;
