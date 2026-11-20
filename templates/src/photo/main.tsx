// 修图: quick photo editing — crop, colours, filters, portrait retouch,
// text, stickers, pens, mosaic and frames — in the page, with nothing of
// the computer needed. The photos stay on the computer that opened them;
// each one's recipe is the page's records, so people and agents edit the
// same photo, and nothing is baked in until it is exported.
import "../_lib/sheet.css";
import "./photo.css";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Crop as CropIcon, Download, Eye, Frame, ImagePlus, Loader2, PenLine, Redo2, RotateCcw, SlidersHorizontal, Smile, Sparkles, Sticker, Trash2, Type, Undo2, Grid3x3, Wand2,
} from "lucide-react";
import { Toast, mount, useKit } from "../_lib/appkit";
import { save } from "../_lib/myco";
import { Popover, rectOf } from "../_lib/ui";
import { edited, kit, num, photos, recipeOf, type Crop, type Layer, type Photo } from "./model";
import { addPhoto, canRedo, canUndo, changeLayer, layerToTop, patch, redo, removeLayer, removePhoto, reset, undo, addLayer } from "./edit";
import { IMAGE_TYPES, isImage, keep, localFor, onLocal, posterOf, restore, type Local } from "./local";
import { Renderer } from "./render";
import { Stage, type Brush, type Mode } from "./stage";
import { AdjustPanel, BeautyPanel, CropPanel, DrawPanel, FilterPanel, FramePanel, StickerPanel, TextPanel, Slider } from "./panels";

const TOOLS: [Mode, string, typeof CropIcon][] = [
  ["crop", "裁剪", CropIcon], ["adjust", "调色", SlidersHorizontal], ["filter", "滤镜", Wand2], ["beauty", "人像", Smile],
  ["text", "文字", Type], ["sticker", "贴纸", Sticker], ["draw", "涂鸦", PenLine], ["mosaic", "马赛克", Grid3x3], ["frame", "边框", Frame],
];

function useNarrow() {
  const q = window.matchMedia?.("(max-width: 760px)");
  const [n, setN] = useState(!!q?.matches);
  useEffect(() => { if (!q) return; const f = () => setN(q.matches); q.addEventListener("change", f); return () => q.removeEventListener("change", f); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return n;
}

/** Imports image files: kept here, and a photo for each (or the one there is, found again). */
async function importFiles(files: File[], onPick: (id: string) => void) {
  const images = files.filter(isImage);
  if (!images.length) { kit.flash("这不是图片"); return; }
  let last = "";
  for (const f of images) {
    const l = await keep(f).catch(() => null);
    if (!l) { kit.flash(`读不了 ${f.name}：这台电脑的浏览器不认识这种图片`); continue; }
    const had = photos.rows.find((r) => r.data.key === l.key);
    if (had) { last = had.id; continue; }
    last = addPhoto({ name: f.name || "图片", key: l.key, width: l.width, height: l.height, poster: posterOf(l.view, l.width, l.height), by: kit.me });
  }
  if (last) onPick(last);
}

function App() {
  useKit(kit);
  const [, setLocalVer] = useState(0);
  useEffect(() => onLocal(() => setLocalVer((n) => n + 1)), []);
  const narrow = useNarrow();
  const [cur, setCur] = useState<string>(() => photos.rows[photos.rows.length - 1]?.id || "");
  const [mode, setMode] = useState<Mode>("adjust");
  const [sel, setSel] = useState<string | null>(null);
  const [focusText, setFocusText] = useState(0);
  const [compare, setCompare] = useState(false);
  const [brush, setBrush] = useState<Brush>({ color: "#f54a45", width: 0.012, pen: "pen", mosaic: "pixel", mwidth: 0.06 });
  const [dragOver, setDragOver] = useState(false);
  const [exporting, setExporting] = useState<{ x: number; y: number; w: number; h: number } | null>(null);
  const file = useRef<HTMLInputElement>(null);

  const rec = photos.get(cur) || photos.rows[photos.rows.length - 1] || null;
  const id = rec?.id || "";
  const photo = rec?.data || null;
  const local = localFor(photo?.key);

  useEffect(() => { setSel(null); }, [id]);
  const pick = useCallback((pid: string) => { setCur(pid); }, []);
  const open = () => file.current?.click();
  const onFiles = (list: FileList | File[] | null) => { if (list && list.length) void importFiles(Array.from(list), pick); };

  // paste an image, drop files anywhere
  useEffect(() => {
    const paste = (e: ClipboardEvent) => {
      const t = e.target as HTMLElement;
      if (t && (t.tagName === "TEXTAREA" || t.tagName === "INPUT")) return;
      const files = Array.from(e.clipboardData?.files || []).filter(isImage);
      if (files.length) { e.preventDefault(); void importFiles(files, pick); }
    };
    window.addEventListener("paste", paste);
    return () => window.removeEventListener("paste", paste);
  }, [pick]);

  // keys: undo, redo, delete what is picked, Esc
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      const typing = t && (t.tagName === "TEXTAREA" || (t.tagName === "INPUT" && (t as HTMLInputElement).type !== "range"));
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key.toLowerCase() === "z" && !typing) { e.preventDefault(); if (e.shiftKey) redo(); else undo(); return; }
      if (mod && e.key.toLowerCase() === "y" && !typing) { e.preventDefault(); redo(); return; }
      if (typing) return;
      if ((e.key === "Delete" || e.key === "Backspace") && sel && id) { e.preventDefault(); removeLayer(id, sel); setSel(null); }
      if (e.key === "Escape") setSel(null);
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [sel, id]);

  // others see a small picture: kept up to date with the edit (not an undo step)
  useEffect(() => {
    if (!photo || !local || !id) return;
    const t = window.setTimeout(() => {
      const r = new Renderer().render(local.view, local.key, recipeOf(photo), local.width, local.height, { max: 360, fill: "#ffffff" });
      let url = "";
      try { url = r.canvas.toDataURL("image/jpeg", 0.72); } catch { return; }
      const now = photos.get(id);
      if (now && url && now.data.poster !== url) void kit.track(photos.put({ ...now.data, poster: url }, id));
    }, 1500);
    return () => window.clearTimeout(t);
  }, [photo, local, id]); // eslint-disable-line react-hooks/exhaustive-deps

  const chooseMode = (m: Mode) => {
    setMode(m);
    if (m !== "text" && m !== "sticker") {
      const l = sel ? photo?.layers?.find((x) => x.id === sel) : null;
      if (l && ((m === "draw" || m === "mosaic" || m === "crop"))) setSel(null);
    }
  };
  const onSel = (s: string | null) => {
    setSel(s);
    const l = s ? photo?.layers?.find((x) => x.id === s) : null;
    if (l?.type === "text" && mode !== "text") setMode("text");
    if (l?.type === "sticker" && mode !== "sticker") setMode("sticker");
    if (s && id) layerToTop(id, s);
  };

  const hidden = <input ref={file} type="file" accept={IMAGE_TYPES} multiple hidden onChange={(e) => { onFiles(e.target.files); e.target.value = ""; }} />;
  const dropProps = {
    onDragOver: (e: React.DragEvent) => { if (e.dataTransfer.types.includes("Files")) { e.preventDefault(); setDragOver(true); } },
    onDragLeave: (e: React.DragEvent) => { if (e.currentTarget === e.target) setDragOver(false); },
    onDrop: (e: React.DragEvent) => { e.preventDefault(); setDragOver(false); onFiles(e.dataTransfer.files); },
  };

  if (!rec || !photo) {
    return (
      <div className={`app ph-app${dragOver ? " is-drop" : ""}`} {...dropProps}>
        {hidden}
        <div className="ph-empty">
          <div className="ph-empty-art" aria-hidden><Sparkles className="ph-empty-ico" strokeWidth={1.4} /></div>
          <h1>修图</h1>
          <p>裁剪、调色、滤镜、磨皮美白、文字、贴纸、涂鸦、马赛克、边框，改了随时能退回去。</p>
          <button type="button" className="btn btn-primary ph-big" onClick={open}><ImagePlus className="ico" />选择照片</button>
          <span className="ph-empty-or">也可以把照片拖进来，或者直接粘贴</span>
          <span className="ph-empty-note">照片只存在这台电脑上，hub 只记一张小图和你做了哪些修改。</span>
        </div>
        <Toast kit={kit} />
      </div>
    );
  }

  const panel = !local ? null
    : mode === "crop" ? <CropPanel id={id} photo={photo} />
    : mode === "adjust" ? <AdjustPanel id={id} photo={photo} />
    : mode === "filter" ? <FilterPanel id={id} photo={photo} local={local} />
    : mode === "beauty" ? <BeautyPanel id={id} photo={photo} />
    : mode === "text" ? <TextPanel id={id} sel={sel} onSel={onSel} focusKey={focusText} />
    : mode === "sticker" ? <StickerPanel id={id} sel={sel} onSel={onSel} />
    : mode === "draw" || mode === "mosaic" ? <DrawPanel id={id} mode={mode} brush={brush} setBrush={setBrush} />
    : <FramePanel id={id} photo={photo} />;

  const rail = (
    <nav className="ph-rail" aria-label="工具">
      {TOOLS.map(([m, name, I]) => (
        <button key={m} type="button" className={`ph-tool${mode === m ? " is-on" : ""}`} aria-pressed={mode === m} onClick={() => chooseMode(m)} disabled={!local}>
          <I className="ico" strokeWidth={1.7} /><span>{name}</span>
        </button>
      ))}
    </nav>
  );

  return (
    <div className={`app ph-app${narrow ? " is-narrow" : ""}${dragOver ? " is-drop" : ""}`} {...dropProps}>
      {hidden}
      <header className="top ph-top">
        <span className="ph-name" title={photo.name}>{photo.name || "图片"}</span>
        <span className="ph-dims">{num(photo.width, 0)} × {num(photo.height, 0)}</span>
        <span className={`top-state is-${kit.pending ? "saving" : kit.failed ? "error" : "saved"}`}>{kit.pending ? "正在保存…" : kit.failed ? "没保存上" : edited(photo) ? "已保存" : ""}</span>
        <span className="top-gap" />
        <button type="button" className="icon-btn" aria-label="撤销" title="撤销 (⌘Z)" disabled={!canUndo()} onClick={undo}><Undo2 className="ico" /></button>
        <button type="button" className="icon-btn" aria-label="重做" title="重做 (⇧⌘Z)" disabled={!canRedo()} onClick={redo}><Redo2 className="ico" /></button>
        <button type="button" className={`tool ph-compare${compare ? " is-on" : ""}`} disabled={!local || !edited(photo)} title="按住看原图"
          onPointerDown={() => setCompare(true)} onPointerUp={() => setCompare(false)} onPointerLeave={() => setCompare(false)} onKeyDown={(e) => { if (e.key === " ") setCompare(true); }} onKeyUp={() => setCompare(false)}>
          <Eye className="ico" />{narrow ? "" : "对比原图"}
        </button>
        <button type="button" className="tool" disabled={!edited(photo)} onClick={() => { reset(id); setSel(null); kit.flash("已回到原图", undo); }}><RotateCcw className="ico" />{narrow ? "" : "还原"}</button>
        <button type="button" className="btn btn-primary" disabled={!local} onClick={(e) => setExporting(rectOf(e.currentTarget))}><Download className="ico" />导出</button>
      </header>
      <div className="ph-body">
        {!narrow && rail}
        {!narrow && local && <aside className="ph-side">{panel}</aside>}
        <main className="ph-main">
          {local ? (
            <Stage photo={photo} local={local} mode={mode} compare={compare} brush={brush} sel={sel} onSel={onSel}
              onCrop={(c: Crop | null) => patch(id, { crop: c })}
              onStroke={(l: Layer) => addLayer(id, l)}
              onLayer={(lid, p) => changeLayer(id, lid, p)}
              onRemoveLayer={(lid) => { removeLayer(id, lid); setSel(null); }}
              onEditText={(lid) => { setSel(lid); setMode("text"); setFocusText((n) => n + 1); }} />
          ) : (
            <Missing photo={photo} onOpen={open} />
          )}
          {compare && <span className="ph-badge">原图</span>}
        </main>
        {narrow && local && <div className="ph-under">{panel}</div>}
        {narrow && rail}
      </div>
      <Strip cur={id} onPick={pick} onAdd={open} />
      {exporting && local && <ExportBox at={exporting} photo={photo} local={local} onClose={() => setExporting(null)} />}
      {dragOver && <div className="ph-drop"><ImagePlus className="ph-drop-ico" strokeWidth={1.4} />松开，加进来</div>}
      <Toast kit={kit} />
    </div>
  );
}

function Missing({ photo, onOpen }: { photo: Photo; onOpen: () => void }) {
  return (
    <div className="ph-missing">
      {photo.poster && <img src={photo.poster} alt="" />}
      <div className="ph-missing-box">
        <b>原图不在这台电脑上</b>
        <span>{photo.by ? `它是 ${photo.by} 加的。` : ""}把同一张照片（{photo.name || "图片"}）选进来或拖进来，就能接着修。</span>
        <button type="button" className="btn btn-primary" onClick={onOpen}><ImagePlus className="ico" />选择照片</button>
      </div>
    </div>
  );
}

function Strip({ cur, onPick, onAdd }: { cur: string; onPick: (id: string) => void; onAdd: () => void }) {
  return (
    <div className="ph-strip" role="listbox" aria-label="这一页的照片">
      {photos.rows.map((r) => (
        <div key={r.id} className={`ph-thumb${r.id === cur ? " is-on" : ""}${localFor(r.data.key) ? "" : " is-away"}`} role="option" aria-selected={r.id === cur}
          title={r.data.name} onClick={() => onPick(r.id)}>
          {r.data.poster ? <img src={r.data.poster} alt="" /> : <span />}
          <button type="button" className="ph-thumb-x" aria-label="从这一页移走" title="从这一页移走"
            onClick={(e) => { e.stopPropagation(); removePhoto(r.id); kit.flash(`已移走 ${r.data.name || "图片"}`, undo); }}><Trash2 className="ico" /></button>
        </div>
      ))}
      <button type="button" className="ph-thumb is-add" onClick={onAdd} aria-label="加照片" title="加照片"><ImagePlus className="ico" /></button>
    </div>
  );
}

const SIZES: { id: string; name: string; max: number }[] = [
  { id: "full", name: "原图", max: Infinity },
  { id: "2048", name: "2048", max: 2048 },
  { id: "1080", name: "1080", max: 1080 },
];

function ExportBox({ at, photo, local, onClose }: { at: { x: number; y: number; w: number; h: number }; photo: Photo; local: Local; onClose: () => void }) {
  const [fmt, setFmt] = useState<"jpeg" | "png">("jpeg");
  const [size, setSize] = useState("full");
  const [quality, setQuality] = useState(92);
  const [busy, setBusy] = useState(false);
  const rc = recipeOf(photo);
  const side = rc.rotate % 180 !== 0;
  const FW = side ? local.height : local.width, FH = side ? local.width : local.height;
  const c = rc.crop || { w: 1, h: 1 };
  const cw = Math.round(c.w * FW), ch = Math.round(c.h * FH);
  const max = SIZES.find((s) => s.id === size)!.max;
  const k = Math.min(1, max / Math.max(cw, ch));
  const go = async () => {
    setBusy(true);
    await new Promise((r) => setTimeout(r, 30));
    try {
      const r = new Renderer().render(local.full, local.key + ":full", rc, local.width, local.height, { max: Math.max(cw, ch) * k, fill: fmt === "jpeg" ? "#ffffff" : undefined });
      const blob = await new Promise<Blob | null>((res) => r.canvas.toBlob(res, `image/${fmt}`, quality / 100));
      if (!blob) throw new Error("没导出来");
      const base = (photo.name || "图片").replace(/\.[^.]+$/, "");
      const saved = await save(`${base}-修图.${fmt === "jpeg" ? "jpg" : "png"}`, blob, `image/${fmt}`);
      kit.flash(`已存到 ${saved?.path || "下载"}`);
      onClose();
    } catch (e) {
      kit.flash((e as Error)?.message || "没导出来");
    } finally { setBusy(false); }
  };
  return (
    <Popover at={at} onClose={onClose} align="right" width={272} className="ph-export">
      <div className="ph-export-in">
        <div className="ph-label">格式</div>
        <div className="seg ph-seg">
          <button type="button" className={fmt === "jpeg" ? "is-on" : ""} onClick={() => setFmt("jpeg")}>JPG</button>
          <button type="button" className={fmt === "png" ? "is-on" : ""} onClick={() => setFmt("png")}>PNG</button>
        </div>
        <div className="ph-label">尺寸</div>
        <div className="seg ph-seg">
          {SIZES.map((s) => <button key={s.id} type="button" className={size === s.id ? "is-on" : ""} onClick={() => setSize(s.id)}>{s.name}</button>)}
        </div>
        <span className="ph-export-dims">{Math.round(cw * k)} × {Math.round(ch * k)}{rc.frame ? "（加上边框）" : ""}</span>
        {fmt === "jpeg" && <Slider label="画质" value={quality} min={50} max={100} onChange={setQuality} />}
        <button type="button" className="btn btn-primary ph-export-go" disabled={busy} onClick={go}>
          {busy ? <Loader2 className="ico spin" /> : <Download className="ico" />}{busy ? "正在导出…" : "存到下载"}
        </button>
      </div>
    </Popover>
  );
}

void mount(kit, App, async () => {
  await restore(photos.rows.map((r) => String(r.data.key || "")));
});
