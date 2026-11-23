import { t as uiText } from "../_lib/i18n";
// The maps (the small round one and the big one): islands from above, the
// boat, a course. Plain canvas drawing in the page's own colours, so they
// follow light and dark.

import { landMask, Shape, type Isle } from "./islands";

export type Chart = { isle: Isle; img: HTMLCanvasElement; ext: number };
export type Palette = { sea: string; shallow: string; land: string; ink: string; faint: string; accent: string; bg: string };

/** Each island drawn once into a small picture (land and its shallows). */
export function charts(isles: Isle[], pal: Palette): Chart[] {
  return isles.map((isle) => {
    const s = new Shape(isle);
    const m = landMask(s, Math.max(1.5, s.reach / 90));
    const c = document.createElement("canvas");
    c.width = c.height = m.n;
    const g = c.getContext("2d", { willReadFrequently: true })!;
    const img = g.createImageData(m.n, m.n);
    const rgb = (css: string) => { g.fillStyle = css; g.fillRect(0, 0, 1, 1); return g.getImageData(0, 0, 1, 1).data; };
    const land = rgb(pal.land), shallow = rgb(pal.shallow);
    g.clearRect(0, 0, 1, 1);
    for (let i = 0; i < m.cells.length; i++) {
      const v = m.cells[i];
      if (!v) continue;
      const c2 = v === 2 ? land : shallow;
      img.data.set([c2[0], c2[1], c2[2], v === 2 ? 255 : 150], i * 4);
    }
    g.putImageData(img, 0, 0);
    return { isle, img: c, ext: m.ext };
  });
}

/** The page's colours (for canvas). */
export function palette(el: Element): Palette {
  const cs = getComputedStyle(el);
  const v = (n: string, d: string) => cs.getPropertyValue(n).trim() || d;
  return { sea: v("--oc-sea", "#e9ecef"), shallow: v("--oc-shallow", "#cfd6db"), land: v("--oc-land", "#5b5f63"), ink: v("--fg", "#111"), faint: v("--faint", "#999"), accent: v("--fg", "#111"), bg: v("--bg", "#fff") };
}

export type View = { cx: number; cz: number; scale: number }; // metres at the centre, pixels per metre

export type Course = { x: number; z: number; name: string } | null;

/** Draws a map of the sea around (cx, cz). North is up. */
export function drawMap(g: CanvasRenderingContext2D, w: number, h: number, view: View, cs: Chart[], boat: { x: number; z: number; heading: number }, pal: Palette, opts: { names?: boolean; course?: Course; selected?: string; found?: Set<string> } = {}) {
  const sx = (x: number) => w / 2 + (x - view.cx) * view.scale;
  const sy = (z: number) => h / 2 + (z - view.cz) * view.scale;
  g.fillStyle = pal.sea;
  g.fillRect(0, 0, w, h);
  g.imageSmoothingEnabled = true;
  for (const c of cs) {
    const half = c.ext / 2;
    const x0 = sx(c.isle.x - half), y0 = sy(c.isle.z - half), size = c.ext * view.scale;
    if (x0 > w || y0 > h || x0 + size < 0 || y0 + size < 0) continue;
    g.drawImage(c.img, x0, y0, size, size);
  }
  if (opts.course) {
    g.strokeStyle = pal.ink; g.globalAlpha = 0.55; g.setLineDash([4, 4]); g.lineWidth = 1.5;
    g.beginPath(); g.moveTo(sx(boat.x), sy(boat.z)); g.lineTo(sx(opts.course.x), sy(opts.course.z)); g.stroke();
    g.setLineDash([]); g.globalAlpha = 1;
  }
  if (opts.names) {
    g.font = "500 12px -apple-system, 'PingFang SC', 'Microsoft YaHei', 'Segoe UI', sans-serif";
    g.textAlign = "center"; g.textBaseline = "top";
    for (const c of cs) {
      if (!c.isle.name) continue;
      const x = sx(c.isle.x), y = sy(c.isle.z) + Math.max(6, c.isle.size * 0.9 * view.scale) + 4;
      if (x < -60 || x > w + 60 || y < -20 || y > h + 20) continue;
      const sel = opts.selected === c.isle.id;
      g.lineWidth = 3; g.strokeStyle = pal.sea; g.strokeText(c.isle.name, x, y);
      g.fillStyle = sel ? pal.ink : opts.found && !opts.found.has(c.isle.id) ? pal.faint : pal.ink;
      g.fillText(c.isle.name, x, y);
      if (sel) { g.strokeStyle = pal.ink; g.lineWidth = 1.5; g.beginPath(); g.arc(sx(c.isle.x), sy(c.isle.z), Math.max(8, c.isle.size * view.scale * 1.15), 0, Math.PI * 2); g.stroke(); }
    }
  }
  // the boat: an arrow pointing where it heads
  const bx = sx(boat.x), by = sy(boat.z);
  g.save();
  g.translate(bx, by);
  g.rotate(boat.heading);
  g.beginPath(); g.moveTo(0, -8); g.lineTo(5.5, 6); g.lineTo(0, 3); g.lineTo(-5.5, 6); g.closePath();
  g.fillStyle = pal.ink; g.fill();
  g.lineWidth = 1.5; g.strokeStyle = pal.bg; g.stroke();
  g.restore();
}

/** Metres or kilometres, rounded for reading. */
export function distText(m: number) {
  return m >= 1000 ? uiText("{0} 公里", (m / 1000).toFixed(m >= 10000 ? 0 : 1)) : uiText("{0} 米", Math.round(m / 10) * 10);
}
