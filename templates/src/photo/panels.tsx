import { t as uiText } from "../_lib/i18n";
// The tools beside the photo: one panel for each of crop, adjust, filters,
// portrait, text, stickers, pens, mosaic and frames.
import { useEffect, useRef, useState } from "react";
import { FlipHorizontal2, FlipVertical2, RotateCcw, RotateCw, Trash2, Undo2, Plus, Bold } from "lucide-react";
import { uid } from "../_lib/appkit";
import {
  ADJUSTS, COLORS, FILTERS, FONTS, FRAMES, FRAME_COLORS, RATIOS, STICKERS, clamp, num, recipeOf,
  photos as store, type Adjust, type Layer, type Photo, type TextStyle,
} from "./model";
import { addLayer, changeLayer, layersOf, patch, removeLayer, setLayers } from "./edit";
import { Renderer } from "./render";
import type { Local } from "./local";
import type { Brush, Mode } from "./stage";

export function Slider({ label, value, min, max, onChange, onReset, step = 1, unit = "" }: {
  label: string; value: number; min: number; max: number; onChange: (v: number) => void; onReset?: () => void; step?: number; unit?: string;
}) {
  const pct = ((value - min) / (max - min)) * 100;
  const zero = min < 0 ? ((0 - min) / (max - min)) * 100 : 0;
  const lo = Math.min(pct, zero), hi = Math.max(pct, zero);
  return (
    <label className="ph-slider" onDoubleClick={onReset} title={onReset ? uiText("双击还原") : undefined}>
      <span className="ph-slider-top"><span>{label}</span><b className={value ? "is-set" : ""}>{value > 0 && min < 0 ? "+" : ""}{Math.round(value * 10) / 10}{unit}</b></span>
      <input type="range" min={min} max={max} step={step} value={value} onChange={(e) => onChange(Number(e.target.value))}
        style={{ ["--lo" as string]: `${lo}%`, ["--hi" as string]: `${hi}%` }} />
    </label>
  );
}

function Swatches({ colors, value, onChange }: { colors: string[]; value: string; onChange: (c: string) => void }) {
  return (
    <div className="ph-swatches" role="radiogroup">
      {colors.map((c) => (
        <button key={c} type="button" role="radio" aria-checked={value.toLowerCase() === c} aria-label={c} className={`ph-swatch${value.toLowerCase() === c ? " is-on" : ""}`}
          style={{ background: c }} onClick={() => onChange(c)} />
      ))}
      <label className="ph-swatch is-pick" title={uiText("别的颜色")} style={{ background: COLORS.includes(value.toLowerCase()) || colors.includes(value.toLowerCase()) ? undefined : value }}>
        <input type="color" value={/^#[0-9a-f]{6}$/i.test(value) ? value : "#ffffff"} onChange={(e) => onChange(e.target.value)} aria-label={uiText("别的颜色")} />
      </label>
    </div>
  );
}

function Seg<T extends string>({ value, options, onChange }: { value: T; options: [T, string][]; onChange: (v: T) => void }) {
  return (
    <div className="seg ph-seg">
      {options.map(([v, name]) => <button key={v} type="button" className={value === v ? "is-on" : ""} onClick={() => onChange(v)}>{name}</button>)}
    </div>
  );
}

// (the latest of a photo, for changes made one after another)
const photos = (id: string) => store.get(id)?.data;

const Head = ({ title, children }: { title: string; children?: React.ReactNode }) => (
  <div className="ph-head"><h2>{title}</h2>{children}</div>
);

// ---- crop ----
export function CropPanel({ id, photo }: { id: string; photo: Photo }) {
  const rc = recipeOf(photo);
  const ratio = photo.ratio || "free";
  const setRatio = (r: string) => {
    const R = RATIOS.find((x) => x.id === r)!;
    const W = num(photo.width, 1), H = num(photo.height, 1);
    const FW = rc.rotate % 180 ? H : W, FH = rc.rotate % 180 ? W : H;
    const want = R.r === -1 ? FW / FH : R.r;
    if (want <= 0) { patch(id, { ratio: r }); return; }
    // the largest box of that ratio, centred on the crop there is
    const c = rc.crop || { x: 0, y: 0, w: 1, h: 1 };
    const cx = c.x + c.w / 2, cy = c.y + c.h / 2;
    let w = 1, h = (FW / want) / FH;
    if (h > 1) { h = 1; w = (FH * want) / FW; }
    const x = clamp(cx - w / 2, 0, 1 - w), y = clamp(cy - h / 2, 0, 1 - h);
    patch(id, { ratio: r, crop: w > 0.999 && h > 0.999 ? null : { x, y, w, h } });
  };
  const turn = (d: number) => {
    // a quarter turn: the crop turns with the photo
    const c = rc.crop;
    let crop = c;
    if (c) crop = d > 0 ? { x: 1 - c.y - c.h, y: c.x, w: c.h, h: c.w } : { x: c.y, y: 1 - c.x - c.w, w: c.h, h: c.w };
    const r = RATIOS.find((x) => x.id === ratio);
    const flipped = r && r.r > 0 && r.id.includes(":") ? r.id.split(":").reverse().join(":") : ratio;
    patch(id, { rotate: (rc.rotate + d + 360) % 360, crop, ratio: RATIOS.some((x) => x.id === flipped) ? flipped : ratio });
  };
  return (
    <div className="ph-pane">
      <Head title={uiText("裁剪和旋转")}>{(rc.crop || rc.rotate || rc.flipH || rc.flipV || rc.straighten) ? <button type="button" className="link-btn" onClick={() => patch(id, { crop: null, rotate: 0, flip_h: false, flip_v: false, straighten: 0, ratio: "free" })}>{uiText("还原")}</button> : null}</Head>
      <div className="ph-ratios">
        {RATIOS.map((r) => (
          <button key={r.id} type="button" className={`ph-ratio${ratio === r.id ? " is-on" : ""}`} onClick={() => setRatio(r.id)}>
            <i style={r.r > 0 ? { aspectRatio: String(r.r) } : undefined} className={r.r <= 0 ? "is-free" : ""} />{r.name}
          </button>
        ))}
      </div>
      <div className="ph-row4">
        <button type="button" className="ph-act" onClick={() => turn(-90)}><RotateCcw className="ico" />{uiText("向左转")}</button>
        <button type="button" className="ph-act" onClick={() => turn(90)}><RotateCw className="ico" />{uiText("向右转")}</button>
        <button type="button" className="ph-act" onClick={() => patch(id, { flip_h: !rc.flipH })}><FlipHorizontal2 className="ico" />{uiText("左右翻")}</button>
        <button type="button" className="ph-act" onClick={() => patch(id, { flip_v: !rc.flipV })}><FlipVertical2 className="ico" />{uiText("上下翻")}</button>
      </div>
      <Slider label={uiText("拉直")} value={rc.straighten} min={-45} max={45} step={0.5} unit="°" onChange={(v) => patch(id, { straighten: v }, "straighten")} onReset={() => patch(id, { straighten: 0 })} />
      <p className="ph-hint">{uiText("拖动框的边和角来裁剪，拖动框内移动它。")}</p>
    </div>
  );
}

// ---- adjust ----
export function AdjustPanel({ id, photo }: { id: string; photo: Photo }) {
  const a = photo.adjust || {};
  const set = (k: keyof Adjust, v: number) => patch(id, { adjust: { ...(photos(id)?.adjust || {}), [k]: v } }, `adjust.${k}`);
  const any = Object.values(a).some((v) => v);
  return (
    <div className="ph-pane">
      <Head title={uiText("调色")}>{any && <button type="button" className="link-btn" onClick={() => patch(id, { adjust: {} })}>{uiText("全部还原")}</button>}</Head>
      {ADJUSTS.map((x) => (
        <Slider key={x.id} label={x.name} value={num(a[x.id], 0)} min={x.min} max={100} onChange={(v) => set(x.id, v)} onReset={() => set(x.id, 0)} />
      ))}
    </div>
  );
}

// ---- filters ----
export function FilterPanel({ id, photo, local }: { id: string; photo: Photo; local: Local }) {
  const rc = recipeOf(photo);
  const geoKey = JSON.stringify([local.key, rc.rotate, rc.flipH, rc.flipV, rc.straighten, rc.crop]);
  const [thumbs, setThumbs] = useState<Record<string, string>>({});
  useEffect(() => {
    let stop = false;
    const base = { ...recipeOf(photo), adjust: {}, beauty: {}, layers: [], frame: "", filterStrength: 100 };
    (async () => {
      const out: Record<string, string> = {};
      for (const f of FILTERS) {
        if (stop) return;
        const r = new Renderer().render(local.view, local.key, { ...base, filter: f.id }, local.width, local.height, { max: 132 });
        out[f.id] = r.canvas.toDataURL("image/jpeg", 0.8);
        await new Promise((res) => setTimeout(res, 0));
      }
      if (!stop) setThumbs(out);
    })();
    return () => { stop = true; };
  }, [geoKey]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="ph-pane">
      <Head title={uiText("滤镜")} />
      <div className="ph-filters">
        {FILTERS.map((f) => (
          <button key={f.id} type="button" className={`ph-filter${rc.filter === f.id ? " is-on" : ""}`} onClick={() => patch(id, { filter: f.id, filter_strength: rc.filter === f.id ? rc.filterStrength : 80 })}>
            <span className="ph-filter-pic">{thumbs[f.id] ? <img src={thumbs[f.id]} alt="" /> : null}</span>
            {f.name}
          </button>
        ))}
      </div>
      {rc.filter && <Slider label={uiText("强度")} value={rc.filterStrength} min={0} max={100} onChange={(v) => patch(id, { filter_strength: v }, "filter_strength")} onReset={() => patch(id, { filter_strength: 80 })} />}
    </div>
  );
}

// ---- portrait ----
export function BeautyPanel({ id, photo }: { id: string; photo: Photo }) {
  const b = photo.beauty || {};
  const set = (k: "smooth" | "whiten", v: number) => patch(id, { beauty: { ...(photos(id)?.beauty || {}), [k]: v } }, `beauty.${k}`);
  return (
    <div className="ph-pane">
      <Head title={uiText("人像")} />
      <Slider label={uiText("磨皮")} value={num(b.smooth, 0)} min={0} max={100} onChange={(v) => set("smooth", v)} onReset={() => set("smooth", 0)} />
      <Slider label={uiText("美白")} value={num(b.whiten, 0)} min={0} max={100} onChange={(v) => set("whiten", v)} onReset={() => set("whiten", 0)} />
      <div className="ph-presets">
        {[["自然", 30, 15], ["清透", 50, 30], ["精致", 70, 40]].map(([n, s, w]) => (
          <button key={n as string} type="button" className="ph-chip" onClick={() => patch(id, { beauty: { smooth: s as number, whiten: w as number } })}>{uiText(String(n))}</button>
        ))}
        <button type="button" className="ph-chip" onClick={() => patch(id, { beauty: {} })}>{uiText("关闭")}</button>
      </div>
      <p className="ph-hint">{uiText("按肤色找到皮肤，只在平滑的地方磨，眼睛、头发和轮廓保持清楚。")}</p>
    </div>
  );
}

// ---- text ----
const TEXT_PRESETS: { name: string; l: Partial<Extract<Layer, { type: "text" }>> }[] = [
  { name: uiText("标题"), l: { size: 0.09, style: "shadow", color: "#ffffff", font: "sans" } },
  { name: uiText("描边"), l: { size: 0.08, style: "stroke", color: "#ffd60a", font: "sans" } },
  { name: uiText("标签"), l: { size: 0.05, style: "box", color: "#f54a45", font: "sans" } },
  { name: uiText("手写"), l: { size: 0.07, style: "plain", color: "#ffffff", font: "kai", bold: false } },
  { name: uiText("印章"), l: { size: 0.055, style: "box", color: "#1f2329", font: "serif" } },
];

export function TextPanel({ id, sel, onSel, focusKey }: { id: string; sel: string | null; onSel: (s: string | null) => void; focusKey: number }) {
  const layers = layersOf(id);
  const l = layers.find((x) => x.id === sel && x.type === "text") as Extract<Layer, { type: "text" }> | undefined;
  const ta = useRef<HTMLTextAreaElement>(null);
  const [draft, setDraft] = useState<string | null>(null);
  useEffect(() => { setDraft(null); }, [sel]);
  useEffect(() => { if (focusKey && ta.current) { ta.current.focus(); ta.current.select(); } }, [focusKey, l?.id]);
  const add = (pre: Partial<Extract<Layer, { type: "text" }>>) => {
    const n = layers.filter((x) => x.type === "text").length;
    const nl = { id: uid("t"), type: "text", x: 0.5, y: clamp(0.5 + (n % 4) * 0.08, 0.1, 0.9), size: 0.08, rot: 0, text: "双击修改文字", color: "#ffffff", style: "shadow", font: "sans", bold: true, ...pre } as Layer;
    addLayer(id, nl);
    onSel(nl.id);
  };
  const set = (p: Partial<Extract<Layer, { type: "text" }>>, key?: string) => l && changeLayer(id, l.id, p, key);
  return (
    <div className="ph-pane">
      <Head title={uiText("文字")} />
      <div className="ph-presets">
        {TEXT_PRESETS.map((t) => <button key={t.name} type="button" className="ph-chip" onClick={() => add(t.l)}><Plus className="ico" />{t.name}</button>)}
      </div>
      {l ? (
        <>
          <textarea ref={ta} className="ph-text" rows={3} value={draft ?? l.text} placeholder={uiText("写点什么")}
            onChange={(e) => { setDraft(e.target.value); set({ text: e.target.value }, `text.${l.id}`); }} />
          <div className="ph-fonts">
            {FONTS.map((f) => <button key={f.id} type="button" className={`ph-chip${(l.font || "sans") === f.id ? " is-on" : ""}`} style={{ fontFamily: f.css }} onClick={() => set({ font: f.id })}>{f.name}</button>)}
            <button type="button" className={`ph-chip${l.bold !== false ? " is-on" : ""}`} aria-label={uiText("加粗")} title={uiText("加粗")} onClick={() => set({ bold: l.bold === false })}><Bold className="ico" /></button>
          </div>
          <Seg<TextStyle> value={l.style || "shadow"} options={[["plain", "无"], ["shadow", "阴影"], ["stroke", "描边"], ["box", "底色"]]} onChange={(s) => set({ style: s })} />
          <Swatches colors={COLORS} value={l.color || "#ffffff"} onChange={(c) => set({ color: c }, `color.${l.id}`)} />
          <Slider label={uiText("大小")} value={Math.round(l.size * 1000) / 10} min={1} max={40} step={0.5} onChange={(v) => set({ size: v / 100 }, `size.${l.id}`)} />
          <Slider label={uiText("角度")} value={Math.round(l.rot || 0)} min={-180} max={180} unit="°" onChange={(v) => set({ rot: v }, `rot.${l.id}`)} onReset={() => set({ rot: 0 })} />
          <button type="button" className="btn is-danger" onClick={() => { removeLayer(id, l.id); onSel(null); }}><Trash2 className="ico" />{uiText("删除这段文字")}</button>
        </>
      ) : (
        <p className="ph-hint">{layers.some((x) => x.type === "text") ? uiText("点图上的文字来改它，拖动移动，拖右下角旋转和缩放。") : uiText("选一种样式加到图上。")}</p>
      )}
    </div>
  );
}

// ---- stickers ----
export function StickerPanel({ id, sel, onSel }: { id: string; sel: string | null; onSel: (s: string | null) => void }) {
  const layers = layersOf(id);
  const l = layers.find((x) => x.id === sel && x.type === "sticker") as Extract<Layer, { type: "sticker" }> | undefined;
  const add = (emoji: string) => {
    const n = layers.filter((x) => x.type === "sticker").length;
    const nl: Layer = { id: uid("s"), type: "sticker", emoji, x: clamp(0.5 + ((n % 5) - 2) * 0.06, 0.1, 0.9), y: clamp(0.45 + ((n % 3) - 1) * 0.06, 0.1, 0.9), size: 0.16, rot: 0 };
    addLayer(id, nl);
    onSel(nl.id);
  };
  return (
    <div className="ph-pane">
      <Head title={uiText("贴纸")} />
      <div className="ph-stickers">
        {STICKERS.map((e) => <button key={e} type="button" className="ph-sticker" onClick={() => add(e)} aria-label={e}>{e}</button>)}
      </div>
      {l ? (
        <>
          <Slider label={uiText("大小")} value={Math.round(l.size * 100)} min={3} max={80} onChange={(v) => changeLayer(id, l.id, { size: v / 100 }, `size.${l.id}`)} />
          <Slider label={uiText("角度")} value={Math.round(l.rot || 0)} min={-180} max={180} unit="°" onChange={(v) => changeLayer(id, l.id, { rot: v }, `rot.${l.id}`)} onReset={() => changeLayer(id, l.id, { rot: 0 })} />
          <button type="button" className="btn is-danger" onClick={() => { removeLayer(id, l.id); onSel(null); }}><Trash2 className="ico" />{uiText("删除这个贴纸")}</button>
        </>
      ) : <p className="ph-hint">{uiText("点一个贴到图上，拖动移动，拖右下角旋转和缩放。")}</p>}
    </div>
  );
}

// ---- pens and mosaic ----
export function DrawPanel({ id, mode, brush, setBrush }: { id: string; mode: Mode; brush: Brush; setBrush: (b: Brush) => void }) {
  const type = mode === "mosaic" ? "mosaic" : "draw";
  const mine = layersOf(id).filter((l) => l.type === type);
  const clearAll = () => setLayers(id, layersOf(id).filter((l) => l.type !== type));
  const undoOne = () => { const last = mine[mine.length - 1]; if (last) removeLayer(id, last.id); };
  return (
    <div className="ph-pane">
      <Head title={type === "mosaic" ? uiText("马赛克") : uiText("涂鸦")} />
      {type === "mosaic"
        ? <Seg value={brush.mosaic} options={[["pixel", "马赛克"], ["blur", "模糊"]]} onChange={(m) => setBrush({ ...brush, mosaic: m })} />
        : <>
            <Seg value={brush.pen} options={[["pen", "画笔"], ["marker", "荧光笔"], ["neon", "霓虹"]]} onChange={(pen) => setBrush({ ...brush, pen })} />
            <Swatches colors={COLORS} value={brush.color} onChange={(color) => setBrush({ ...brush, color })} />
          </>}
      {type === "mosaic"
        ? <Slider label={uiText("笔刷大小")} value={Math.round(brush.mwidth * 1000) / 10} min={1} max={20} step={0.5} onChange={(v) => setBrush({ ...brush, mwidth: v / 100 })} />
        : <Slider label={uiText("笔刷大小")} value={Math.round(brush.width * 1000) / 10} min={0.3} max={10} step={0.1} onChange={(v) => setBrush({ ...brush, width: v / 100 })} />}
      <div className="ph-row2">
        <button type="button" className="btn" disabled={!mine.length} onClick={undoOne}><Undo2 className="ico" />{uiText("撤掉一笔")}</button>
        <button type="button" className="btn" disabled={!mine.length} onClick={clearAll}><Trash2 className="ico" />{uiText("全部擦掉")}</button>
      </div>
      <p className="ph-hint">{type === "mosaic" ? uiText("在图上涂抹要遮住的地方：人脸、车牌、聊天记录里的名字。") : uiText("在图上按住拖动来画。")}</p>
    </div>
  );
}

// ---- frames ----
export function FramePanel({ id, photo }: { id: string; photo: Photo }) {
  const rc = recipeOf(photo);
  const colored = rc.frame === "white" || rc.frame === "thin" || rc.frame === "polaroid";
  return (
    <div className="ph-pane">
      <Head title={uiText("边框")} />
      <div className="ph-frames">
        {FRAMES.map((f) => (
          <button key={f.id} type="button" className={`ph-frame${rc.frame === f.id ? " is-on" : ""}`} onClick={() => patch(id, { frame: f.id })}>
            <span className={`ph-frame-pic is-${f.id || "none"}`}><i /></span>{f.name}
          </button>
        ))}
      </div>
      {colored && <><div className="ph-label">{uiText("边框颜色")}</div><Swatches colors={FRAME_COLORS} value={rc.frameColor} onChange={(c) => patch(id, { frame_color: c }, "frame_color")} /></>}
    </div>
  );
}
