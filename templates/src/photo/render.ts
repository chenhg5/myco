// From the original and its recipe to the picture: turned and cropped, then
// the colours (portrait retouch, adjustments, the filter, sharpening, the
// vignette and grain), then what is drawn over it (mosaic, pens, stickers,
// text), then the frame around it. The editor and the export both go
// through here, so what is shown is what is saved.
import { clamp, filterOf, fontOf, num, type Adjust, type Layer, type Recipe } from "./model";

const RAD = Math.PI / 180;

/** The photo after turning: its size, the crop in it, and the size of the crop. */
export function geom(rc: Recipe, W: number, H: number) {
  const side = rc.rotate % 180 !== 0;
  const FW = side ? H : W, FH = side ? W : H;
  const crop = rc.crop || { x: 0, y: 0, w: 1, h: 1 };
  return { FW, FH, crop, cw: crop.w * FW, ch: crop.h * FH };
}

/** How much a straightened photo grows so that no corner is empty. */
export function coverScale(w: number, h: number, deg: number) {
  const t = Math.abs(deg) * RAD, c = Math.cos(t), s = Math.sin(t);
  return Math.max((w * c + h * s) / w, (w * s + h * c) / h);
}

/** Draws the turned, flipped and straightened photo into (0,0)–(FW,FH) of the current transform. */
function drawTurned(ctx: CanvasRenderingContext2D, src: CanvasImageSource, rc: Recipe, W: number, H: number, FW: number, FH: number) {
  ctx.save();
  ctx.translate(FW / 2, FH / 2);
  if (rc.straighten) ctx.rotate(rc.straighten * RAD);
  const s = rc.straighten ? coverScale(FW, FH, rc.straighten) : 1;
  ctx.scale(s * (rc.flipH ? -1 : 1), s * (rc.flipV ? -1 : 1));
  if (rc.rotate) ctx.rotate(rc.rotate * RAD);
  ctx.imageSmoothingQuality = "high";
  ctx.drawImage(src, -W / 2, -H / 2, W, H);
  ctx.restore();
}

export type RenderOpts = {
  /** The largest side of the result (before the frame), in pixels. */
  max: number;
  /** Only turned and cropped: the original's colours, nothing drawn over it. */
  bare?: boolean;
  /** The whole photo (for cropping): no crop, no frame. */
  whole?: boolean;
  /** Layers not to draw (the one being edited as text, say). */
  hide?: string;
  /** Rounded frames: what is outside the corners (a JPEG has no transparency). */
  fill?: string;
};

export type Rendered = {
  canvas: HTMLCanvasElement;
  /** Pixels per pixel of the original. */
  k: number;
  /** The photo's place inside the frame. */
  padL: number; padT: number; w: number; h: number;
  FW: number; FH: number; crop: { x: number; y: number; w: number; h: number };
};

function canvasOf(w: number, h: number) {
  const c = document.createElement("canvas");
  c.width = Math.max(1, Math.round(w)); c.height = Math.max(1, Math.round(h));
  return c;
}

/** Renders photos, keeping the last colour work (it is the slow part) for
 *  as long as only what is drawn over it changes. */
export class Renderer {
  private base: { key: string; canvas: HTMLCanvasElement } | null = null;
  private veils = new Map<string, HTMLCanvasElement>();

  render(src: CanvasImageSource, srcId: string, rc: Recipe, W: number, H: number, o: RenderOpts): Rendered {
    const g = geom(rc, W, H);
    const crop = o.whole ? { x: 0, y: 0, w: 1, h: 1 } : g.crop;
    const cw = crop.w * g.FW, ch = crop.h * g.FH;
    const k = Math.min(1, o.max / Math.max(cw, ch));
    const bw = Math.max(1, Math.round(cw * k)), bh = Math.max(1, Math.round(ch * k));
    const key = JSON.stringify([srcId, W, H, bw, bh, crop, rc.rotate, rc.flipH, rc.flipV, rc.straighten, o.bare ? 0 : [rc.adjust, rc.filter, rc.filterStrength, rc.beauty]]);
    if (!this.base || this.base.key !== key) {
      const c = canvasOf(bw, bh);
      const ctx = c.getContext("2d", { willReadFrequently: true })!;
      ctx.setTransform(bw / cw, 0, 0, bh / ch, 0, 0);
      ctx.translate(-crop.x * g.FW, -crop.y * g.FH);
      drawTurned(ctx, src, rc, W, H, g.FW, g.FH);
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      if (!o.bare) {
        const img = ctx.getImageData(0, 0, bw, bh);
        colours(img, rc);
        ctx.putImageData(img, 0, 0);
      }
      this.base = { key, canvas: c };
      this.veils.clear();
    }
    const base = this.base.canvas;
    // what is drawn over it
    const layers = o.bare ? [] : rc.layers.filter((l) => l.id !== o.hide);
    let pic = base;
    if (layers.length) {
      pic = canvasOf(bw, bh);
      const ctx = pic.getContext("2d")!;
      ctx.drawImage(base, 0, 0);
      const s = bw / cw; // pixels per original pixel
      const at = (fx: number, fy: number): [number, number] => [(fx * g.FW - crop.x * g.FW) * s, (fy * g.FH - crop.y * g.FH) * s];
      for (const l of layers) drawLayer(ctx, l, at, g.FW * s, (style, block) => this.veil(base, style, block));
    }
    // the frame
    const frame = o.bare || o.whole ? "" : rc.frame;
    const f = frameBox(frame, bw, bh);
    if (!frame) return { canvas: pic, k: bw / cw, padL: 0, padT: 0, w: bw, h: bh, FW: g.FW, FH: g.FH, crop };
    const out = canvasOf(f.W, f.H);
    const ctx = out.getContext("2d")!;
    drawFrame(ctx, frame, rc.frameColor, f, o.fill);
    if (frame === "round") {
      ctx.save();
      roundRect(ctx, f.l, f.t, bw, bh, Math.min(bw, bh) * 0.07);
      ctx.clip();
      ctx.drawImage(pic, f.l, f.t);
      ctx.restore();
    } else ctx.drawImage(pic, f.l, f.t);
    return { canvas: out, k: bw / cw, padL: f.l, padT: f.t, w: bw, h: bh, FW: g.FW, FH: g.FH, crop };
  }

  /** The photo pixelated or blurred (for mosaic strokes), kept for this colour work. */
  private veil(base: HTMLCanvasElement, style: string, block: number) {
    const key = `${style}:${block}`;
    let v = this.veils.get(key);
    if (v) return v;
    v = canvasOf(base.width, base.height);
    const ctx = v.getContext("2d", { willReadFrequently: true })!;
    if (style === "blur") {
      ctx.drawImage(base, 0, 0);
      const img = ctx.getImageData(0, 0, v.width, v.height);
      const r = Math.max(2, Math.round(block * 0.7));
      let d = boxBlur(img.data, v.width, v.height, r);
      d = boxBlur(d, v.width, v.height, r);
      d = boxBlur(d, v.width, v.height, r);
      img.data.set(d);
      ctx.putImageData(img, 0, 0);
    } else {
      const sw = Math.max(1, Math.round(base.width / block)), sh = Math.max(1, Math.round(base.height / block));
      const small = canvasOf(sw, sh);
      const sc = small.getContext("2d")!;
      sc.imageSmoothingQuality = "medium";
      sc.drawImage(base, 0, 0, sw, sh);
      ctx.imageSmoothingEnabled = false;
      ctx.drawImage(small, 0, 0, v.width, v.height);
    }
    this.veils.set(key, v);
    return v;
  }
}

// ---- the colours ----

/** The adjustments with the filter's (scaled by its strength) added. */
export function mixed(rc: Recipe) {
  const f = filterOf(rc.filter), t = rc.filterStrength / 100;
  const a = (k: keyof Adjust) => num(rc.adjust[k], 0) + num(f.adjust?.[k], 0) * t;
  return {
    brightness: a("brightness"), contrast: a("contrast"), saturation: a("saturation"), warmth: a("warmth"), tint: a("tint"),
    highlights: a("highlights"), shadows: a("shadows"), sharpen: a("sharpen"), vignette: a("vignette"), fade: a("fade"), grain: a("grain"),
    mono: (f.mono || 0) * t,
    cast: [f.r, f.g, f.b].map((c) => [1 + ((c?.[0] ?? 1) - 1) * t, (c?.[1] ?? 0) * t]) as [number, number][],
  };
}

function colours(img: ImageData, rc: Recipe) {
  const { data, width: w, height: h } = img;
  const m = mixed(rc);
  const smooth = clamp(num(rc.beauty.smooth, 0), 0, 100) / 100, whiten = clamp(num(rc.beauty.whiten, 0), 0, 100) / 100;
  if (smooth > 0 || whiten > 0) retouch(data, w, h, smooth, whiten);

  // the tone curve (the same for each channel), then the cast (per channel)
  const tone = new Uint8ClampedArray(256);
  const b = m.brightness / 100, c = m.contrast / 100, hi = m.highlights / 100, sh = m.shadows / 100, fd = Math.max(0, m.fade) / 100;
  const gamma = Math.pow(2, -b * 0.85), con = c >= 0 ? 1 + c * 0.85 : 1 + c * 0.65;
  for (let i = 0; i < 256; i++) {
    let x = i / 255;
    if (b) x = Math.pow(x, gamma);
    if (sh) x += sh * 2.2 * x * (1 - x) * (1 - x);
    if (hi) x += hi * 2.2 * x * x * (1 - x);
    if (c) x = 0.5 + (x - 0.5) * con;
    if (fd) x = x * (1 - 0.24 * fd) + 0.1 * fd;
    tone[i] = Math.round(clamp(x, 0, 1) * 255);
  }
  const wm = m.warmth / 100, tn = m.tint / 100;
  const shift = [wm * 22 + tn * 8, wm * 4 - tn * 16, -wm * 22 + tn * 8];
  const cast = [0, 1, 2].map((ch) => {
    const lut = new Uint8ClampedArray(256);
    const [gain, off] = m.cast[ch];
    for (let i = 0; i < 256; i++) lut[i] = Math.round(i * gain + off + shift[ch]);
    return lut;
  });
  const [cr, cg, cb] = cast;
  const sv = m.saturation / 100;
  const sat = (1 + sv * (sv > 0 ? 0.9 : 1)) * (1 - clamp(m.mono, 0, 1));
  const doSat = Math.abs(sat - 1) > 1e-3;
  for (let i = 0; i < data.length; i += 4) {
    let r = tone[data[i]], g = tone[data[i + 1]], bl = tone[data[i + 2]];
    if (doSat) {
      const l = 0.299 * r + 0.587 * g + 0.114 * bl;
      r = l + (r - l) * sat; g = l + (g - l) * sat; bl = l + (bl - l) * sat;
      r = r < 0 ? 0 : r > 255 ? 255 : r | 0; g = g < 0 ? 0 : g > 255 ? 255 : g | 0; bl = bl < 0 ? 0 : bl > 255 ? 255 : bl | 0;
    }
    data[i] = cr[r]; data[i + 1] = cg[g]; data[i + 2] = cb[bl];
  }

  const sharp = Math.max(0, m.sharpen) / 100;
  if (sharp > 0) {
    const r = Math.max(1, Math.round(Math.min(w, h) / 700));
    const bl = boxBlur(data, w, h, r);
    const amt = sharp * 1.6;
    for (let i = 0; i < data.length; i += 4) {
      data[i] = data[i] + (data[i] - bl[i]) * amt;
      data[i + 1] = data[i + 1] + (data[i + 1] - bl[i + 1]) * amt;
      data[i + 2] = data[i + 2] + (data[i + 2] - bl[i + 2]) * amt;
    }
  }

  const vig = m.vignette / 100, grain = Math.max(0, m.grain) / 100;
  if (vig || grain) {
    const cx = w / 2, cy = h / 2, cell = Math.max(1, Math.round(Math.max(w, h) / 1400)), amp = grain * 58;
    for (let y = 0; y < h; y++) {
      const dy = (y - cy) / cy;
      for (let x = 0; x < w; x++) {
        const i = (y * w + x) * 4;
        let r = data[i], g = data[i + 1], bl = data[i + 2];
        if (vig) {
          const dx = (x - cx) / cx;
          const d = Math.sqrt((dx * dx + dy * dy) / 2);
          const f = d <= 0.3 ? 0 : d >= 1 ? 1 : ((d - 0.3) / 0.7) ** 2 * (3 - 2 * ((d - 0.3) / 0.7));
          if (vig > 0) { const mul = 1 - vig * 0.8 * f; r *= mul; g *= mul; bl *= mul; }
          else { const a = -vig * 0.85 * f; r += (255 - r) * a; g += (255 - g) * a; bl += (255 - bl) * a; }
        }
        if (amp) {
          const n = (hash((x / cell) | 0, (y / cell) | 0) - 0.5) * amp;
          r += n; g += n; bl += n;
        }
        data[i] = r; data[i + 1] = g; data[i + 2] = bl;
      }
    }
  }
}

function hash(x: number, y: number) {
  let h = Math.imul(x, 374761393) + Math.imul(y, 668265263);
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  return ((h ^ (h >>> 16)) >>> 0) / 4294967296;
}

/** Smoother, brighter skin: where the colour is a skin colour (soft-edged),
 *  blurred where it is flat and kept where it has edges (eyes, hair). */
function retouch(data: Uint8ClampedArray, w: number, h: number, smooth: number, whiten: number) {
  const n = w * h;
  const mask = new Float32Array(n);
  for (let p = 0, i = 0; p < n; p++, i += 4) {
    const r = data[i], g = data[i + 1], b = data[i + 2];
    const cb = 128 - 0.168736 * r - 0.331264 * g + 0.5 * b;
    const cr = 128 + 0.5 * r - 0.418688 * g - 0.081312 * b;
    const y = 0.299 * r + 0.587 * g + 0.114 * b;
    const inCb = edge(cb, 77, 127, 10), inCr = edge(cr, 133, 173, 10);
    mask[p] = y < 40 ? 0 : inCb * inCr;
  }
  const mr = Math.max(1, Math.round(Math.min(w, h) / 220));
  const soft = blurChannel(blurChannel(mask, w, h, mr), w, h, mr);
  let blurred: Uint8ClampedArray | null = null;
  if (smooth > 0) {
    const r = Math.max(1, Math.round(Math.min(w, h) * 0.008 * (0.5 + smooth)));
    blurred = boxBlur(boxBlur(data, w, h, r), w, h, r);
  }
  const thresh = 18 + 34 * smooth;
  const beta = 1 + 3.5 * whiten, lb = Math.log(beta);
  const curve = new Uint8ClampedArray(256);
  for (let i = 0; i < 256; i++) curve[i] = whiten > 0 ? Math.round((255 * Math.log(1 + (beta - 1) * (i / 255))) / lb) : i;
  for (let p = 0, i = 0; p < n; p++, i += 4) {
    const m = soft[p];
    if (m < 0.01) continue;
    let r = data[i], g = data[i + 1], b = data[i + 2];
    if (blurred) {
      const br = blurred[i], bg = blurred[i + 1], bb = blurred[i + 2];
      const diff = Math.abs(0.299 * (r - br) + 0.587 * (g - bg) + 0.114 * (b - bb));
      const wgt = m * smooth * Math.max(0, 1 - diff / thresh) * 0.92;
      r += (br - r) * wgt; g += (bg - g) * wgt; b += (bb - b) * wgt;
    }
    if (whiten > 0) {
      const a = m * whiten;
      r += (curve[r | 0] - r) * a; g += (curve[g | 0] - g) * a; b += (curve[b | 0] - b) * a;
    }
    data[i] = r; data[i + 1] = g; data[i + 2] = b;
  }
}
function edge(v: number, lo: number, hi: number, soft: number) {
  if (v < lo - soft || v > hi + soft) return 0;
  if (v < lo) return (v - lo + soft) / soft;
  if (v > hi) return (hi + soft - v) / soft;
  return 1;
}

/** A box blur of an RGBA image (alpha kept), radius r, in two passes. */
export function boxBlur(src: Uint8ClampedArray, w: number, h: number, r: number): Uint8ClampedArray {
  const tmp = new Uint8ClampedArray(src.length), out = new Uint8ClampedArray(src.length);
  const win = r * 2 + 1;
  for (let y = 0; y < h; y++) {
    const row = y * w * 4;
    for (let c = 0; c < 3; c++) {
      let sum = 0;
      for (let k = -r; k <= r; k++) sum += src[row + clamp(k, 0, w - 1) * 4 + c];
      for (let x = 0; x < w; x++) {
        tmp[row + x * 4 + c] = sum / win;
        sum += src[row + Math.min(w - 1, x + r + 1) * 4 + c] - src[row + Math.max(0, x - r) * 4 + c];
      }
    }
  }
  for (let x = 0; x < w; x++) {
    for (let c = 0; c < 3; c++) {
      let sum = 0;
      for (let k = -r; k <= r; k++) sum += tmp[clamp(k, 0, h - 1) * w * 4 + x * 4 + c];
      for (let y = 0; y < h; y++) {
        out[(y * w + x) * 4 + c] = sum / win;
        sum += tmp[Math.min(h - 1, y + r + 1) * w * 4 + x * 4 + c] - tmp[Math.max(0, y - r) * w * 4 + x * 4 + c];
      }
    }
  }
  for (let i = 3; i < src.length; i += 4) out[i] = src[i];
  return out;
}

function blurChannel(src: Float32Array, w: number, h: number, r: number) {
  const tmp = new Float32Array(src.length), out = new Float32Array(src.length);
  const win = r * 2 + 1;
  for (let y = 0; y < h; y++) {
    const row = y * w;
    let sum = 0;
    for (let k = -r; k <= r; k++) sum += src[row + clamp(k, 0, w - 1)];
    for (let x = 0; x < w; x++) {
      tmp[row + x] = sum / win;
      sum += src[row + Math.min(w - 1, x + r + 1)] - src[row + Math.max(0, x - r)];
    }
  }
  for (let x = 0; x < w; x++) {
    let sum = 0;
    for (let k = -r; k <= r; k++) sum += tmp[clamp(k, 0, h - 1) * w + x];
    for (let y = 0; y < h; y++) {
      out[y * w + x] = sum / win;
      sum += tmp[Math.min(h - 1, y + r + 1) * w + x] - tmp[Math.max(0, y - r) * w + x];
    }
  }
  return out;
}

// ---- what is drawn over it ----

const EMOJI_FONT = `"Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji", sans-serif`;

/** Is this colour light (so its outline or text on it is dark)? */
export function light(hex: string) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || "");
  if (!m) return true;
  const v = parseInt(m[1], 16);
  return 0.299 * (v >> 16) + 0.587 * ((v >> 8) & 255) + 0.114 * (v & 255) > 150;
}

export function textFont(l: { font?: string; bold?: boolean }, px: number) {
  return `${l.bold === false ? 500 : 700} ${px}px ${fontOf(l.font).css}`;
}

let measurer: CanvasRenderingContext2D | null = null;
/** A text or sticker layer's size (unturned), in the same pixels as `unit` (the photo's width). */
export function layerSize(l: Layer, unit: number): { w: number; h: number } {
  if (l.type === "sticker") return { w: l.size * unit * 1.15, h: l.size * unit * 1.15 };
  if (l.type !== "text") return { w: 0, h: 0 };
  if (!measurer) measurer = document.createElement("canvas").getContext("2d")!;
  const px = Math.max(1, l.size * unit);
  measurer.font = textFont(l, px);
  const lines = (l.text || " ").split("\n");
  const w = Math.max(...lines.map((s) => measurer!.measureText(s || " ").width));
  const pad = l.style === "box" ? px * 0.7 : l.style === "stroke" ? px * 0.2 : 0;
  return { w: w + pad, h: lines.length * px * 1.25 + (l.style === "box" ? px * 0.4 : 0) };
}

function drawLayer(ctx: CanvasRenderingContext2D, l: Layer, at: (x: number, y: number) => [number, number], unit: number,
  veil: (style: string, block: number) => HTMLCanvasElement) {
  ctx.save();
  if (l.type === "draw" || l.type === "mosaic") {
    const pts = l.points || [];
    if (pts.length < 2) { ctx.restore(); return; }
    const lw = Math.max(1, l.width * unit);
    const path = new Path2D();
    const [x0, y0] = at(pts[0], pts[1]);
    path.moveTo(x0, y0);
    if (pts.length === 2) path.lineTo(x0 + 0.01, y0);
    for (let i = 2; i + 1 < pts.length; i += 2) { const [x, y] = at(pts[i], pts[i + 1]); path.lineTo(x, y); }
    ctx.lineCap = "round"; ctx.lineJoin = "round"; ctx.lineWidth = lw;
    if (l.type === "mosaic") {
      const W = ctx.canvas.width, H = ctx.canvas.height;
      const block = Math.max(4, Math.round(Math.min(W, H) * 0.028));
      const v = veil(l.style || "pixel", block);
      const tmp = canvasOf(W, H);
      const t = tmp.getContext("2d")!;
      t.lineCap = "round"; t.lineJoin = "round"; t.lineWidth = lw; t.strokeStyle = "#000";
      t.stroke(path);
      t.globalCompositeOperation = "source-in";
      t.drawImage(v, 0, 0);
      ctx.drawImage(tmp, 0, 0);
    } else if (l.pen === "neon") {
      ctx.strokeStyle = l.color; ctx.shadowColor = l.color; ctx.shadowBlur = lw * 1.6;
      ctx.stroke(path); ctx.stroke(path);
      ctx.shadowBlur = 0; ctx.lineWidth = lw * 0.38; ctx.strokeStyle = "rgba(255,255,255,0.92)";
      ctx.stroke(path);
    } else {
      if (l.pen === "marker") { ctx.globalAlpha = 0.5; ctx.lineCap = "square"; }
      ctx.strokeStyle = l.color;
      ctx.stroke(path);
    }
    ctx.restore();
    return;
  }
  const [x, y] = at(l.x, l.y);
  ctx.translate(x, y);
  if (l.rot) ctx.rotate(l.rot * RAD);
  const px = Math.max(1, l.size * unit);
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  if (l.type === "sticker") {
    ctx.font = `${px}px ${EMOJI_FONT}`;
    ctx.fillText(l.emoji, 0, px * 0.04);
    ctx.restore();
    return;
  }
  const color = l.color || "#ffffff";
  ctx.font = textFont(l, px);
  const lines = (l.text || "").split("\n");
  const lh = px * 1.25, top = -((lines.length - 1) * lh) / 2;
  const style = l.style || "shadow";
  if (style === "box") {
    const { w, h } = layerSize(l, unit);
    ctx.fillStyle = color;
    roundRect(ctx, -w / 2, -h / 2, w, h, px * 0.32);
    ctx.fill();
    ctx.fillStyle = light(color) ? "#1f2329" : "#ffffff";
  } else ctx.fillStyle = color;
  if (style === "shadow") { ctx.shadowColor = "rgba(0,0,0,0.55)"; ctx.shadowBlur = px * 0.22; ctx.shadowOffsetY = px * 0.05; }
  lines.forEach((s, i) => {
    const ly = top + i * lh + px * 0.04;
    if (style === "stroke") {
      ctx.lineJoin = "round"; ctx.lineWidth = px * 0.16; ctx.strokeStyle = light(color) ? "#1f2329" : "#ffffff";
      ctx.strokeText(s, 0, ly);
    }
    ctx.fillText(s, 0, ly);
  });
  ctx.restore();
}

// ---- frames ----

type Box = { W: number; H: number; l: number; t: number; w: number; h: number };
export function frameBox(frame: string, w: number, h: number): Box {
  const m = Math.min(w, h);
  let l = 0, t = 0, r = 0, b = 0;
  if (frame === "white") l = t = r = b = Math.round(m * 0.05);
  else if (frame === "thin") l = t = r = b = Math.max(2, Math.round(m * 0.016));
  else if (frame === "polaroid") { l = r = t = Math.round(w * 0.055); b = Math.round(w * 0.2); }
  else if (frame === "film") { l = r = Math.round(w * 0.02); t = b = Math.round(Math.max(h * 0.1, w * 0.07)); }
  return { W: w + l + r, H: h + t + b, l, t, w, h };
}

function drawFrame(ctx: CanvasRenderingContext2D, frame: string, color: string, f: Box, fill?: string) {
  if (frame === "round") {
    if (fill) { ctx.fillStyle = fill; ctx.fillRect(0, 0, f.W, f.H); }
    return;
  }
  ctx.fillStyle = frame === "film" ? "#141414" : color;
  ctx.fillRect(0, 0, f.W, f.H);
  if (frame === "film") {
    const hole = f.t * 0.32, gap = hole * 1.55;
    ctx.fillStyle = "#efe9dc";
    for (const y of [f.t * 0.34, f.H - f.t * 0.34 - hole]) {
      for (let x = gap * 0.5; x + hole * 0.9 < f.W; x += gap) { roundRect(ctx, x, y, hole * 0.9, hole, hole * 0.18); ctx.fill(); }
    }
  }
}

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  r = Math.min(r, w / 2, h / 2);
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}
