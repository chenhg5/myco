import { t as uiText } from "../_lib/i18n";
// The photo in the middle: rendered to fit, and what you do on it — crop
// (a box with handles over the whole photo), pens and mosaic (strokes
// follow the pointer), text and stickers (pick one, drag it, turn and size
// it from its corner). While something moves, only a draft changes; it is
// saved when the pointer lets go.
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type PointerEvent as RPointerEvent } from "react";
import { RotateCw, X } from "lucide-react";
import { uid } from "../_lib/appkit";
import { clamp, recipeOf, RATIOS, type Crop, type Layer, type PenId, type Photo } from "./model";
import { Renderer, layerSize, type Rendered } from "./render";
import type { Local } from "./local";
import { VIEW_MAX } from "./local";

export type Mode = "crop" | "adjust" | "filter" | "beauty" | "text" | "sticker" | "draw" | "mosaic" | "frame";
/** Pens and mosaic: widths are fractions of the photo's width. */
export type Brush = { color: string; width: number; pen: PenId; mosaic: "pixel" | "blur"; mwidth: number };

type Props = {
  photo: Photo; local: Local; mode: Mode; compare: boolean; brush: Brush;
  sel: string | null; onSel: (id: string | null) => void;
  onCrop: (c: Crop | null) => void;
  onStroke: (l: Layer) => void;
  onLayer: (lid: string, p: Partial<Layer>) => void;
  onRemoveLayer: (lid: string) => void;
  onEditText: (lid: string) => void;
};

type Box = { w: number; h: number };

export function Stage(p: Props) {
  const wrap = useRef<HTMLDivElement>(null);
  const cv = useRef<HTMLCanvasElement>(null);
  const [box, setBox] = useState<Box>({ w: 0, h: 0 });
  const renderer = useMemo(() => new Renderer(), []);
  const [out, setOut] = useState<Rendered | null>(null);
  const [draft, setDraft] = useState<{ layers?: Layer[]; crop?: Crop | null } | null>(null);
  const [cursor, setCursor] = useState<{ x: number; y: number } | null>(null);
  const busy = useRef({ last: "", at: 0, timer: 0 });

  useLayoutEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setBox({ w: el.clientWidth, h: el.clientHeight }));
    ro.observe(el);
    setBox({ w: el.clientWidth, h: el.clientHeight });
    return () => ro.disconnect();
  }, []);

  const rc = useMemo(() => {
    const r = recipeOf(p.photo);
    if (draft?.layers) r.layers = draft.layers;
    if (draft && "crop" in draft) r.crop = draft.crop ?? null;
    return r;
  }, [p.photo, draft]);
  const whole = p.mode === "crop";

  // what it takes to render, and when: the colours are slow, so while they
  // change quickly the photo is rendered smaller, then sharp once they rest
  useEffect(() => {
    if (!box.w || !box.h) return;
    const L = p.local;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const pad = 28;
    const bw = Math.max(40, box.w - pad * 2), bh = Math.max(40, box.h - pad * 2);
    const side = rc.rotate % 180 !== 0;
    const FW = side ? L.height : L.width, FH = side ? L.width : L.height;
    const c = whole || !rc.crop ? { w: 1, h: 1 } : rc.crop;
    const cw = c.w * FW, ch = c.h * FH;
    const fit = Math.min(bw / cw, bh / ch);
    const full = Math.min(VIEW_MAX, Math.ceil(Math.max(cw, ch) * fit * dpr));
    const colourKey = JSON.stringify([rc.adjust, rc.filter, rc.filterStrength, rc.beauty, rc.rotate, rc.straighten, p.compare]);
    const b = busy.current;
    const quick = colourKey !== b.last && Date.now() - b.at < 400;
    if (colourKey !== b.last) { b.last = colourKey; b.at = Date.now(); }
    const draw = (max: number) => {
      const r = renderer.render(L.view, L.key, rc, L.width, L.height, { max, bare: p.compare, whole });
      const el = cv.current;
      if (!el) return;
      if (el.width !== r.canvas.width || el.height !== r.canvas.height) { el.width = r.canvas.width; el.height = r.canvas.height; }
      const ctx = el.getContext("2d")!;
      ctx.clearRect(0, 0, el.width, el.height);
      ctx.drawImage(r.canvas, 0, 0);
      setOut(r);
    };
    const raf = requestAnimationFrame(() => draw(quick ? Math.max(480, Math.round(full * 0.5)) : full));
    window.clearTimeout(b.timer);
    if (quick) b.timer = window.setTimeout(() => draw(full), 220);
    return () => cancelAnimationFrame(raf);
  }, [rc, box, p.local, p.compare, whole, renderer]);

  // the canvas's place on the screen
  const view = useMemo(() => {
    if (!out || !box.w) return null;
    const pad = 28;
    const W = out.canvas.width, H = out.canvas.height;
    // (the rendered size is the crop's; the frame adds to it)
    const fW = (box.w - pad * 2) / W, fH = (box.h - pad * 2) / H;
    const s = Math.min(fW, fH);
    const w = W * s, h = H * s;
    return { s, w, h, left: (box.w - w) / 2, top: (box.h - h) / 2 };
  }, [out, box]);

  // screen ↔ the photo after turning (fractions of its width and height)
  const toFrame = (cx: number, cy: number) => {
    if (!out || !view || !wrap.current) return { x: 0, y: 0 };
    const r = wrap.current.getBoundingClientRect();
    const px = (cx - r.left - view.left) / view.s, py = (cy - r.top - view.top) / view.s;
    const u = (px - out.padL) / out.k + out.crop.x * out.FW, v = (py - out.padT) / out.k + out.crop.y * out.FH;
    return { x: u / out.FW, y: v / out.FH };
  };
  const toScreen = (fx: number, fy: number) => {
    if (!out || !view) return { x: 0, y: 0 };
    return {
      x: view.left + (out.padL + (fx * out.FW - out.crop.x * out.FW) * out.k) * view.s,
      y: view.top + (out.padT + (fy * out.FH - out.crop.y * out.FH) * out.k) * view.s,
    };
  };
  /** Screen pixels per pixel of the photo. */
  const scale = out && view ? out.k * view.s : 1;

  // ---- pens and mosaic ----
  const stroke = useRef<Layer | null>(null);
  const drawing = p.mode === "draw" || p.mode === "mosaic";
  const onDrawDown = (e: RPointerEvent) => {
    if (!out || e.button !== 0) return;
    (e.target as Element).setPointerCapture(e.pointerId);
    const f = toFrame(e.clientX, e.clientY);
    const l: Layer = p.mode === "mosaic"
      ? { id: uid("m"), type: "mosaic", width: p.brush.mwidth, style: p.brush.mosaic, points: [f.x, f.y] }
      : { id: uid("d"), type: "draw", color: p.brush.color, width: p.brush.width, pen: p.brush.pen, points: [f.x, f.y] };
    stroke.current = l;
    setDraft({ layers: [...rc.layers, l] });
  };
  const onDrawMove = (e: RPointerEvent) => {
    const r = wrap.current?.getBoundingClientRect();
    if (r) setCursor({ x: e.clientX - r.left, y: e.clientY - r.top });
    const l = stroke.current;
    if (!l || (l.type !== "draw" && l.type !== "mosaic")) return;
    const f = toFrame(e.clientX, e.clientY);
    const pts = l.points, n = pts.length;
    // (a point only when it moved a little: fewer of them to keep)
    const minGap = 2.5 / Math.max(1e-6, scale * (out?.FW || 1));
    if (Math.hypot(f.x - pts[n - 2], (f.y - pts[n - 1]) * ((out?.FH || 1) / (out?.FW || 1))) < minGap) return;
    const next = { ...l, points: [...pts, +f.x.toFixed(4), +f.y.toFixed(4)] } as Layer;
    stroke.current = next;
    setDraft((d) => ({ layers: [...(d?.layers || rc.layers).filter((x) => x.id !== next.id), next] }));
  };
  const onDrawUp = () => {
    const l = stroke.current;
    stroke.current = null;
    if (l) p.onStroke(l);
    setDraft(null);
  };

  // ---- text and stickers ----
  const drag = useRef<{ lid: string; kind: "move" | "turn"; x0: number; y0: number; l0: Layer; cx: number; cy: number; d0: number; a0: number } | null>(null);
  const hit = (cx: number, cy: number) => {
    if (!out) return null;
    const f = toFrame(cx, cy);
    const X = f.x * out.FW, Y = f.y * out.FH;
    for (let i = rc.layers.length - 1; i >= 0; i--) {
      const l = rc.layers[i];
      if (l.type !== "text" && l.type !== "sticker") continue;
      const { w, h } = layerSize(l, out.FW);
      const dx = X - l.x * out.FW, dy = Y - l.y * out.FH;
      const t = -(l.rot || 0) * (Math.PI / 180);
      const lx = dx * Math.cos(t) - dy * Math.sin(t), ly = dx * Math.sin(t) + dy * Math.cos(t);
      if (Math.abs(lx) <= w / 2 + 6 / scale && Math.abs(ly) <= h / 2 + 6 / scale) return l;
    }
    return null;
  };
  const startDrag = (e: RPointerEvent, l: Layer, kind: "move" | "turn") => {
    if (l.type !== "text" && l.type !== "sticker") return;
    e.stopPropagation();
    (e.currentTarget as Element).setPointerCapture(e.pointerId);
    const c = toScreen(l.x, l.y);
    const r = wrap.current!.getBoundingClientRect();
    const cx = r.left + c.x, cy = r.top + c.y;
    drag.current = { lid: l.id, kind, x0: e.clientX, y0: e.clientY, l0: l, cx, cy, d0: Math.hypot(e.clientX - cx, e.clientY - cy) || 1, a0: Math.atan2(e.clientY - cy, e.clientX - cx) };
  };
  const onLayerDown = (e: RPointerEvent) => {
    if (e.button !== 0 || p.compare) return;
    const l = hit(e.clientX, e.clientY);
    if (!l) { p.onSel(null); return; }
    p.onSel(l.id);
    startDrag(e, l, "move");
  };
  const onLayerMove = (e: RPointerEvent) => {
    const d = drag.current;
    if (!d || !out || (d.l0.type !== "text" && d.l0.type !== "sticker")) return;
    let next: Layer;
    if (d.kind === "move") {
      const dx = (e.clientX - d.x0) / scale / out.FW, dy = (e.clientY - d.y0) / scale / out.FH;
      next = { ...d.l0, x: clamp(d.l0.x + dx, -0.2, 1.2), y: clamp(d.l0.y + dy, -0.2, 1.2) };
    } else {
      const dist = Math.hypot(e.clientX - d.cx, e.clientY - d.cy);
      const ang = Math.atan2(e.clientY - d.cy, e.clientX - d.cx);
      let rot = (d.l0.rot || 0) + ((ang - d.a0) * 180) / Math.PI;
      rot = ((rot + 540) % 360) - 180;
      for (const snap of [-180, -90, 0, 90, 180]) if (Math.abs(rot - snap) < 4) rot = snap;
      next = { ...d.l0, size: clamp(d.l0.size * (dist / d.d0), 0.01, 1.5), rot: +rot.toFixed(1) };
    }
    setDraft({ layers: rc.layers.map((x) => (x.id === d.lid ? next : x)) });
  };
  const onLayerUp = () => {
    const d = drag.current;
    drag.current = null;
    if (!d) return;
    const l = draft?.layers?.find((x) => x.id === d.lid);
    if (l && (l.type === "text" || l.type === "sticker") && (d.l0.type === "text" || d.l0.type === "sticker")) {
      if (l.x !== d.l0.x || l.y !== d.l0.y || l.size !== d.l0.size || l.rot !== d.l0.rot) p.onLayer(d.lid, { x: l.x, y: l.y, size: l.size, rot: l.rot } as Partial<Layer>);
    }
    setDraft(null);
  };

  // ---- crop ----
  const cropNow = rc.crop || { x: 0, y: 0, w: 1, h: 1 };
  const cropDrag = useRef<{ edge: string; x0: number; y0: number; c0: Crop } | null>(null);
  const ratio = RATIOS.find((r) => r.id === (p.photo.ratio || "free")) || RATIOS[0];
  const onCropDown = (e: RPointerEvent, edge: string) => {
    e.stopPropagation();
    (e.currentTarget as Element).setPointerCapture(e.pointerId);
    cropDrag.current = { edge, x0: e.clientX, y0: e.clientY, c0: { ...cropNow } };
  };
  const onCropMove = (e: RPointerEvent) => {
    const d = cropDrag.current;
    if (!d || !out) return;
    const dx = (e.clientX - d.x0) / scale / out.FW, dy = (e.clientY - d.y0) / scale / out.FH;
    const c = { ...d.c0 };
    const min = 0.04;
    if (d.edge === "move") {
      c.x = clamp(d.c0.x + dx, 0, 1 - c.w); c.y = clamp(d.c0.y + dy, 0, 1 - c.h);
    } else {
      let x0 = d.c0.x, y0 = d.c0.y, x1 = d.c0.x + d.c0.w, y1 = d.c0.y + d.c0.h;
      if (d.edge.includes("w")) x0 = clamp(x0 + dx, 0, x1 - min);
      if (d.edge.includes("e")) x1 = clamp(x1 + dx, x0 + min, 1);
      if (d.edge.includes("n")) y0 = clamp(y0 + dy, 0, y1 - min);
      if (d.edge.includes("s")) y1 = clamp(y1 + dy, y0 + min, 1);
      const r = ratio.r === -1 ? out.FW / out.FH : ratio.r;
      if (r > 0) {
        // keep the ratio (in pixels): the side that was not dragged follows
        const fr = (r * out.FH) / out.FW; // w/h in fractions
        let w = x1 - x0, h = y1 - y0;
        if (d.edge === "n" || d.edge === "s") w = h * fr; else h = w / fr;
        const ax = d.edge.includes("w") ? x1 : d.edge === "n" || d.edge === "s" ? d.c0.x + d.c0.w / 2 : x0;
        const ay = d.edge.includes("n") ? y1 : d.edge === "e" || d.edge === "w" ? d.c0.y + d.c0.h / 2 : y0;
        // shrink to stay inside
        const maxW = d.edge.includes("w") ? ax : d.edge === "n" || d.edge === "s" ? 2 * Math.min(ax, 1 - ax) : 1 - ax;
        const maxH = d.edge.includes("n") ? ay : d.edge === "e" || d.edge === "w" ? 2 * Math.min(ay, 1 - ay) : 1 - ay;
        const k = Math.min(1, maxW / w, maxH / h);
        w *= k; h *= k;
        x0 = d.edge.includes("w") ? ax - w : d.edge === "n" || d.edge === "s" ? ax - w / 2 : ax;
        y0 = d.edge.includes("n") ? ay - h : d.edge === "e" || d.edge === "w" ? ay - h / 2 : ay;
        x1 = x0 + w; y1 = y0 + h;
      }
      Object.assign(c, { x: x0, y: y0, w: x1 - x0, h: y1 - y0 });
    }
    setDraft({ crop: c });
  };
  const onCropUp = () => {
    const d = cropDrag.current;
    cropDrag.current = null;
    if (!d) return;
    const c = draft?.crop;
    if (c) p.onCrop(c.x < 1e-3 && c.y < 1e-3 && c.w > 0.999 && c.h > 0.999 ? null : c);
    setDraft(null);
  };

  const selLayer = !drawing && !whole && !p.compare ? rc.layers.find((l) => l.id === p.sel && (l.type === "text" || l.type === "sticker")) : undefined;
  const selBox = (() => {
    if (!selLayer || !out || (selLayer.type !== "text" && selLayer.type !== "sticker")) return null;
    const { w, h } = layerSize(selLayer, out.FW);
    const c = toScreen(selLayer.x, selLayer.y);
    return { x: c.x, y: c.y, w: w * scale + 12, h: h * scale + 12, rot: selLayer.rot || 0 };
  })();

  const cropBox = whole && out && view ? (() => {
    const a = toScreen(cropNow.x, cropNow.y), b = toScreen(cropNow.x + cropNow.w, cropNow.y + cropNow.h);
    return { left: a.x, top: a.y, width: b.x - a.x, height: b.y - a.y };
  })() : null;

  const handlers = drawing
    ? { onPointerDown: onDrawDown, onPointerMove: onDrawMove, onPointerUp: onDrawUp, onPointerCancel: onDrawUp, onPointerLeave: () => setCursor(null) }
    : whole ? {}
    : { onPointerDown: onLayerDown, onPointerMove: onLayerMove, onPointerUp: onLayerUp, onPointerCancel: onLayerUp,
        onDoubleClick: (e: React.MouseEvent) => { const l = hit(e.clientX, e.clientY); if (l?.type === "text") p.onEditText(l.id); } };

  const brushPx = (p.mode === "mosaic" ? p.brush.mwidth : p.brush.width) * (out?.FW || 0) * scale;
  return (
    <div ref={wrap} className={`ph-stage${drawing ? " is-drawing" : ""}${whole ? " is-cropping" : ""}`} {...handlers}>
      <canvas ref={cv} className="ph-canvas" style={view ? { left: view.left, top: view.top, width: view.w, height: view.h } : { visibility: "hidden" }} />
      {cropBox && (
        <div className="ph-crop" style={cropBox} onPointerDown={(e) => onCropDown(e, "move")} onPointerMove={onCropMove} onPointerUp={onCropUp} onPointerCancel={onCropUp}>
          <i className="ph-crop-grid" />
          {["n", "s", "e", "w", "nw", "ne", "sw", "se"].map((h) => (
            <b key={h} className={`ph-crop-h is-${h}`} onPointerDown={(e) => onCropDown(e, h)} onPointerMove={onCropMove} onPointerUp={onCropUp} onPointerCancel={onCropUp} />
          ))}
        </div>
      )}
      {selBox && selLayer && (
        <div className="ph-sel" style={{ left: selBox.x - selBox.w / 2, top: selBox.y - selBox.h / 2, width: selBox.w, height: selBox.h, transform: `rotate(${selBox.rot}deg)` }}
          onPointerDown={(e) => startDrag(e, selLayer, "move")} onPointerMove={onLayerMove} onPointerUp={onLayerUp} onPointerCancel={onLayerUp}
          onDoubleClick={() => { if (selLayer.type === "text") p.onEditText(selLayer.id); }}>
          <button type="button" className="ph-sel-x" aria-label={uiText("删除")} title={uiText("删除")} onPointerDown={(e) => e.stopPropagation()} onClick={() => p.onRemoveLayer(selLayer.id)}><X className="ico" /></button>
          <span className="ph-sel-turn" aria-label={uiText("旋转和缩放")} title={uiText("拖动：旋转、缩放")}
            onPointerDown={(e) => startDrag(e, selLayer, "turn")} onPointerMove={onLayerMove} onPointerUp={onLayerUp} onPointerCancel={onLayerUp}><RotateCw className="ico" /></span>
        </div>
      )}
      {drawing && cursor && brushPx > 0 && (
        <i className="ph-brush" style={{ left: cursor.x - brushPx / 2, top: cursor.y - brushPx / 2, width: brushPx, height: brushPx }} />
      )}
    </div>
  );
}
