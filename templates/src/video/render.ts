// Draws one frame of the edit: the clip's picture fitted into the frame,
// then the blur and mosaic regions, then the text. The player and the
// export both draw with this, so what you see is what you get.
import { zoomTransform } from "./zoom";
import type { OverlayItem } from "./model";

let tmp: HTMLCanvasElement | null = null;
let tmp2: HTMLCanvasElement | null = null;
let zoomBase: HTMLCanvasElement | null = null;
const scratch = (which: 1 | 2) => {
  if (which === 1) return (tmp ||= document.createElement("canvas"));
  return (tmp2 ||= document.createElement("canvas"));
};

/** A picture to fit into the frame: an image, or something that draws
 *  itself (a decoded video sample). */
export type Picture = { image?: CanvasImageSource; draw?: (ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number) => void; w: number; h: number } | null;

export function fit(sw: number, sh: number, W: number, H: number) {
  const k = Math.min(W / Math.max(1, sw), H / Math.max(1, sh));
  const w = sw * k, h = sh * k;
  return { x: (W - w) / 2, y: (H - h) / 2, w, h };
}

export function drawFrame(ctx: CanvasRenderingContext2D, W: number, H: number, pic: Picture, over: OverlayItem[], t: number,
  opts: { background?: string } = {}) {
  ctx.save();
  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = "high";
  ctx.fillStyle = opts.background || "#000000";
  ctx.fillRect(0, 0, W, H);
  const live = over.filter((o) => t >= o.start - 1e-6 && t < o.end - 1e-6);
  const z = zoomTransform(live.filter((o) => o.type === "zoom").at(-1), t);
  if (pic && z.scale > 1) {
    // Redactions belong to the source picture: flatten them before zooming so
    // a zoom cannot expose a face or address that was covered in the edit.
    const base = zoomBase ||= document.createElement("canvas");
    if (base.width !== W || base.height !== H) { base.width = W; base.height = H; }
    drawFrame(base.getContext("2d")!, W, H, pic, live.filter(o => o.type === "blur" || o.type === "mosaic"), t, opts);
    ctx.save(); ctx.beginPath(); ctx.rect(0, 0, W, H); ctx.clip();
    ctx.translate(W / 2, H / 2); ctx.scale(z.scale, z.scale); ctx.translate(-z.x * W, -z.y * H);
    ctx.drawImage(base, 0, 0); ctx.restore();
  } else {
    if (pic) {
      const r = fit(pic.w, pic.h, W, H);
      try {
        if (pic.draw) pic.draw(ctx, r.x, r.y, r.w, r.h);
        else if (pic.image) ctx.drawImage(pic.image, r.x, r.y, r.w, r.h);
      } catch { /* (not ready yet) */ }
    }
    for (const o of live) if (o.type === "blur" || o.type === "mosaic") censor(ctx, W, H, o);
  }
  for (const o of live) if (o.type === "text") text(ctx, W, H, o);
  ctx.restore();
}

function region(W: number, H: number, o: OverlayItem) {
  const x = Math.max(0, Math.floor(o.x * W)), y = Math.max(0, Math.floor(o.y * H));
  const x2 = Math.min(W, Math.ceil((o.x + o.w) * W)), y2 = Math.min(H, Math.ceil((o.y + o.h) * H));
  return { x, y, w: x2 - x, h: y2 - y };
}

function censor(ctx: CanvasRenderingContext2D, W: number, H: number, o: OverlayItem) {
  const r = region(W, H, o);
  if (r.w < 2 || r.h < 2) return;
  const src = ctx.canvas;
  const a = scratch(1);
  ctx.save();
  if (o.type === "mosaic") {
    const block = Math.max(4, Math.round(Math.max(W, H) * (0.008 + o.strength * 0.042)));
    a.width = Math.max(1, Math.ceil(r.w / block)); a.height = Math.max(1, Math.ceil(r.h / block));
    const ac = a.getContext("2d")!;
    ac.imageSmoothingEnabled = true;
    ac.drawImage(src, r.x, r.y, r.w, r.h, 0, 0, a.width, a.height);
    ctx.imageSmoothingEnabled = false;
    ctx.drawImage(a, 0, 0, a.width, a.height, r.x, r.y, a.width * block, a.height * block);
  } else {
    // a blur: down to a few pixels in two steps, back up smoothly
    const k = 1 / (3 + o.strength * 29);
    const b = scratch(2);
    b.width = Math.max(1, Math.round(r.w * Math.sqrt(k))); b.height = Math.max(1, Math.round(r.h * Math.sqrt(k)));
    a.width = Math.max(1, Math.round(r.w * k)); a.height = Math.max(1, Math.round(r.h * k));
    const bc = b.getContext("2d")!, ac = a.getContext("2d")!;
    bc.imageSmoothingEnabled = ac.imageSmoothingEnabled = true;
    bc.imageSmoothingQuality = ac.imageSmoothingQuality = "high";
    bc.drawImage(src, r.x, r.y, r.w, r.h, 0, 0, b.width, b.height);
    ac.drawImage(b, 0, 0, b.width, b.height, 0, 0, a.width, a.height);
    bc.clearRect(0, 0, b.width, b.height);
    bc.drawImage(a, 0, 0, a.width, a.height, 0, 0, b.width, b.height);
    ctx.beginPath();
    const rad = Math.min(r.w, r.h) * 0.12;
    if (ctx.roundRect) ctx.roundRect(r.x, r.y, r.w, r.h, rad); else ctx.rect(r.x, r.y, r.w, r.h);
    ctx.clip();
    ctx.drawImage(b, 0, 0, b.width, b.height, r.x, r.y, r.w, r.h);
  }
  ctx.restore();
}

const FONT = `-apple-system, BlinkMacSystemFont, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif`;

/** The lines a text breaks into within a width (any character may break). */
export function wrap(ctx: CanvasRenderingContext2D, s: string, width: number) {
  const out: string[] = [];
  for (const para of s.split("\n")) {
    let line = "";
    for (const ch of Array.from(para)) {
      if (line && ctx.measureText(line + ch).width > width) {
        // break at the last space when there is one (latin words)
        const sp = line.lastIndexOf(" ");
        if (sp > 0 && ch !== " ") { out.push(line.slice(0, sp)); line = line.slice(sp + 1) + ch; }
        else { out.push(line); line = ch === " " ? "" : ch; }
      } else line += ch;
    }
    out.push(line);
  }
  return out;
}

function text(ctx: CanvasRenderingContext2D, W: number, H: number, o: OverlayItem) {
  if (!o.text.trim()) return;
  const size = Math.max(8, o.size * H);
  ctx.save();
  ctx.font = `700 ${size}px ${FONT}`;
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  const bw = o.w * W, cx = (o.x + o.w / 2) * W, cy = (o.y + o.h / 2) * H;
  const lines = wrap(ctx, o.text, Math.max(size, bw));
  const lh = size * 1.25, top = cy - ((lines.length - 1) * lh) / 2;
  if (o.style === "box") {
    const pad = size * 0.35;
    const wMax = Math.max(...lines.map((l) => ctx.measureText(l).width));
    ctx.fillStyle = "rgba(0,0,0,0.62)";
    ctx.beginPath();
    const bx = cx - wMax / 2 - pad, by = top - lh / 2 - pad * 0.4, bwid = wMax + pad * 2, bh = lines.length * lh + pad * 0.8;
    if (ctx.roundRect) ctx.roundRect(bx, by, bwid, bh, size * 0.22); else ctx.rect(bx, by, bwid, bh);
    ctx.fill();
  }
  lines.forEach((l, i) => {
    const y = top + i * lh;
    if (o.style === "outline") {
      ctx.lineJoin = "round"; ctx.lineWidth = Math.max(2, size * 0.14); ctx.strokeStyle = "rgba(0,0,0,0.9)";
      ctx.strokeText(l, cx, y);
    } else if (o.style === "shadow") {
      ctx.shadowColor = "rgba(0,0,0,0.65)"; ctx.shadowBlur = size * 0.18; ctx.shadowOffsetY = size * 0.04;
    }
    ctx.fillStyle = o.color;
    ctx.fillText(l, cx, y);
    ctx.shadowColor = "transparent";
  });
  ctx.restore();
}
