// Pure sRGB color math, HSL harmony suggestions and deterministic image sampling.
// Contrast follows WCAG 2.x; a ratio alone does not certify a design's accessibility.
export type Color = { id: string; hex: string; name: string; locked: boolean };
export type Palette = Record<string, unknown> & { title: string; colors: Color[]; tags: string[]; note: string; source: string; created_at: string; updated_at: string };
export type RGB = [number, number, number];
export type HSL = [number, number, number];
export type Harmony = "analogous" | "complementary" | "triadic" | "mono";
export function hex(value: string): string | null {
  const v = value.trim().replace(/^#/, "");
  if (/^[0-9a-f]{3}$/i.test(v)) return "#" + [...v].map(c => c + c).join("").toUpperCase();
  return /^[0-9a-f]{6}$/i.test(v) ? "#" + v.toUpperCase() : null;
}
export function rgb(value: string): RGB {
  const h = hex(value);
  if (!h) throw new Error("Invalid sRGB color");
  return [1, 3, 5].map(i => parseInt(h.slice(i, i + 2), 16)) as RGB;
}
const clamp = (x: number, a: number, b: number) => Math.min(b, Math.max(a, x));
export function rgbHex([r, g, b]: RGB): string {
  return "#" + [r, g, b].map(v => clamp(Math.round(v), 0, 255).toString(16).padStart(2, "0")).join("").toUpperCase();
}
export function hsl(value: string): HSL {
  const [r, g, b] = rgb(value).map(x => x / 255), max = Math.max(r, g, b), min = Math.min(r, g, b), d = max - min, l = (max + min) / 2;
  let h = 0;
  if (d) h = max === r ? ((g - b) / d) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
  return [((h * 60) + 360) % 360, d ? d / (1 - Math.abs(2 * l - 1)) * 100 : 0, l * 100];
}
export function hslHex([h, s, l]: HSL): string {
  h = ((h % 360) + 360) % 360; s = clamp(s, 0, 100) / 100; l = clamp(l, 0, 100) / 100;
  const a = s * Math.min(l, 1 - l);
  const f = (n: number) => { const k = (n + h / 30) % 12; return (l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1))) * 255; };
  return rgbHex([f(0), f(8), f(4)]);
}
export function luminance(value: string): number {
  const c = rgb(value).map(v => { const x = v / 255; return x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4; });
  return c[0] * 0.2126 + c[1] * 0.7152 + c[2] * 0.0722;
}
export function contrast(a: string, b: string): number {
  const x = luminance(a), y = luminance(b);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}
export const ink = (bg: string) => contrast(bg, "#FFFFFF") > contrast(bg, "#111111") ? "#FFFFFF" : "#111111";
export const contrastPass = (ratio: number) => ({ normal: ratio >= 4.5, large: ratio >= 3, enhanced: ratio >= 7 });
export function harmony(base: string, mode: Harmony, count = 5): string[] {
  const [h, s, l] = hsl(base); count = clamp(Math.round(count), 2, 12);
  return Array.from({ length: count }, (_, i) => {
    if (!i) return hex(base)!;
    const t = i / (count - 1);
    if (mode === "mono") return hslHex([h, s, 14 + t * 76]);
    const angle = mode === "analogous" ? (i % 2 ? -1 : 1) * Math.ceil(i / 2) * 22 : mode === "triadic" ? (i % 3) * 120 : i % 2 ? 180 : 0;
    return hslHex([h + angle, clamp(s * (1 - t * 0.18), 15, 90), clamp(l + (i % 2 ? 1 : -1) * t * 20, 15, 88)]);
  });
}
export function validPalette(value: unknown): value is Palette {
  if (!value || typeof value !== "object") return false;
  const p = value as Palette;
  return typeof p.title === "string" && !!p.title.trim() && p.title.length <= 80 && typeof p.note === "string" && p.note.length <= 2000 && typeof p.source === "string" && p.source.length <= 200 && Array.isArray(p.tags) && p.tags.length <= 8 && p.tags.every(t => typeof t === "string" && t.length > 0 && t.length <= 30) && Array.isArray(p.colors) && p.colors.length >= 2 && p.colors.length <= 12 && new Set(p.colors.map(c => c?.id)).size === p.colors.length && p.colors.every(c => c && typeof c.id === "string" && /^[a-zA-Z0-9_-]{1,80}$/.test(c.id) && typeof c.hex === "string" && !!hex(c.hex) && typeof c.name === "string" && c.name.length <= 40 && typeof c.locked === "boolean") && [p.created_at, p.updated_at].every(d => typeof d === "string" && Number.isFinite(Date.parse(d)));
}
// Quantize to 5-bit bins, then weighted k-means. Bound the sample to 16K pixels.
// Transparent pixels contribute nothing; partial opacity weights coverage, not color.
export function extractColors(pixels: ArrayLike<number>, count = 5): string[] {
  count = clamp(Math.round(count), 2, 12);
  const bins = new Map<number, { rgb: RGB; weight: number }>();
  const stride = Math.max(1, Math.ceil(pixels.length / 4 / 16384));
  for (let i = 0; i + 3 < pixels.length; i += 4 * stride) {
    const weight = pixels[i + 3] / 255; if (weight < 0.05) continue;
    const r = pixels[i], g = pixels[i + 1], b = pixels[i + 2], key = ((r >> 3) << 10) | ((g >> 3) << 5) | (b >> 3);
    const bin = bins.get(key) || { rgb: [0, 0, 0] as RGB, weight: 0 };
    bin.rgb[0] += r * weight; bin.rgb[1] += g * weight; bin.rgb[2] += b * weight; bin.weight += weight; bins.set(key, bin);
  }
  const points = [...bins.values()].map(p => ({ rgb: p.rgb.map(v => v / p.weight) as RGB, weight: p.weight })).sort((a, b) => b.weight - a.weight);
  if (!points.length) return [];
  const distance = (a: RGB, b: RGB) => a.reduce((sum, x, i) => sum + (x - b[i]) ** 2, 0);
  const centers: RGB[] = [points[0].rgb];
  while (centers.length < Math.min(count, points.length)) {
    const next = points.reduce((best, p) => {
      const score = Math.min(...centers.map(c => distance(c, p.rgb))) * Math.sqrt(p.weight);
      return score > best.score ? { rgb: p.rgb, score } : best;
    }, { rgb: points[0].rgb, score: 0 });
    if (!next.score) break; centers.push(next.rgb);
  }
  let weights: number[] = [];
  for (let round = 0; round < 12; round++) {
    const sums = centers.map(() => [0, 0, 0] as RGB); weights = centers.map(() => 0);
    for (const p of points) {
      let index = 0, best = Infinity;
      centers.forEach((c, i) => { const d = distance(c, p.rgb); if (d < best) { index = i; best = d; } });
      weights[index] += p.weight; p.rgb.forEach((v, j) => sums[index][j] += v * p.weight);
    }
    centers.forEach((c, i) => { if (weights[i]) centers[i] = sums[i].map(v => v / weights[i]) as RGB; });
  }
  return [...new Set(centers.map((c, i) => ({ color: rgbHex(c), weight: weights[i] })).sort((a, b) => b.weight - a.weight).map(p => p.color))];
}
export function cssPalette(p: Palette): string {
  return ":root {\n" + p.colors.map((c, i) => `  --color-${i + 1}: ${hex(c.hex)};`).join("\n") + "\n}\n";
}
const xml = (s: string) => s.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F]/g, "").replace(/[<>&"']/g, c => ({ "<": "&lt;", ">": "&gt;", "&": "&amp;", '"': "&quot;", "'": "&apos;" })[c]!);
export function svgPalette(p: Palette): string {
  const width = 1200, slot = width / p.colors.length;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="700" viewBox="0 0 1200 700"><rect width="1200" height="700" fill="#FFFFFF"/><text x="48" y="68" font-family="sans-serif" font-size="28" fill="#111111">${xml(p.title)}</text>${p.colors.map((c, i) => `<rect x="${i * slot}" y="112" width="${slot + 0.2}" height="470" fill="${hex(c.hex)}"/><text x="${i * slot + 16}" y="620" font-family="monospace" font-size="${p.colors.length > 8 ? 15 : 20}" fill="#111111">${hex(c.hex)}</text><text x="${i * slot + 16}" y="650" font-family="sans-serif" font-size="14" fill="#555555">${xml(c.name.slice(0, 12))}</text>`).join("")}</svg>`;
}
