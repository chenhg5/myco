// Page composition: a shared palette library, designer previews and local image import.
import { useEffect, useRef, useState } from "react";
import { Palette as PaletteIcon, Plus, Search, Heart, ImagePlus, Pencil, Copy, Download, Trash2, ArrowUpRight, Link2, ChevronRight, Sparkles } from "lucide-react";
import { mount, Toast, useKit, uid } from "../_lib/appkit";
import { myco, save } from "../_lib/myco";
import { t as uiText } from "../_lib/i18n";
import { publicAppLink, readAppRoute, subscribeAppRoute } from "../_lib/page-route";
import { kit, palettes, rows, isSaved } from "./model";
import { deletePalette, toggleFavorite } from "./edit";
import { cssPalette, svgPalette, ink, hex, hsl, validPalette, type Palette } from "./engine";
import { Editor, Swatches, Contrast, copyText, type Draft } from "./panels";
import { colorsFromFile } from "./image";
import "../_lib/sheet.css";
import "./palette.css";

function makeDraft(colors = ["#244B46", "#628B75", "#D5C7AC", "#C88259", "#F3EEE3"]): Draft {
  const now = new Date().toISOString();
  return { id: uid("p"), version: 0, data: { title: uiText("我的新色卡"), colors: colors.map(color => ({ id: uid("c"), hex: color, name: "", locked: false })), tags: [], note: "", source: "", created_at: now, updated_at: now } };
}
function App() {
  useKit(kit);
  const all = rows(), [selected, setSelected] = useState(all[0]?.id || ""), [tab, setTab] = useState("all"), [query, setQuery] = useState(""), [tag, setTag] = useState(""), [draft, setDraft] = useState<Draft | null>(null), [importing, setImporting] = useState(false), [exporting, setExporting] = useState(false), [format, setFormat] = useState("svg"), [routeMissing, setRouteMissing] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null), requestedRoute = useRef("");
  const filtered = all.filter(r => (tab !== "saved" || isSaved(r.id)) && (!tag || r.data.tags.includes(tag)) && (!query.trim() || [r.data.title, r.data.note, ...r.data.tags, ...r.data.colors.map(c => c.hex)].join(" ").toLowerCase().includes(query.trim().toLowerCase())));
  const row = all.find(r => r.id === selected), palette = row?.data;
  const tags = [...new Set(all.flatMap(r => r.data.tags))].sort();
  useEffect(() => {
    const navigate = () => {
      try { const route = readAppRoute(location.hash); const id = route?.name === "palette" ? route.params.get("id") || "" : ""; requestedRoute.current = id; if (id) { const found = palettes.get(id); setRouteMissing(!found || !validPalette(found.data)); if (found && validPalette(found.data)) setSelected(id); } else setRouteMissing(false); }
      catch { setRouteMissing(true); }
    };
    navigate(); return subscribeAppRoute(navigate);
  }, []);
  useEffect(() => {
    if (requestedRoute.current && palettes.get(requestedRoute.current) && validPalette(palettes.get(requestedRoute.current)!.data)) { setSelected(requestedRoute.current); setRouteMissing(false); requestedRoute.current = ""; }
    else if (!all.some(r => r.id === selected) && !routeMissing) setSelected(all[0]?.id || "");
  }, [kit.version, selected, routeMissing]);
  const choose = (id: string) => { requestedRoute.current = ""; setSelected(id); setRouteMissing(false); };
  async function importImage(file: File) {
    setImporting(true);
    try {
      const values = await colorsFromFile(file);
      if (!values.length) { kit.flash(uiText("图片没有可提取的颜色，请换一张图片")); return; }
      const next = makeDraft(values.length === 1 ? [values[0], "#FFFFFF"] : values);
      next.data.title = file.name.replace(/\.[^.]+$/, "").slice(0, 80) || uiText("图片灵感"); next.data.source = uiText("本地图片提色"); setDraft(next);
    } catch { kit.flash(uiText("无法读取图片，请使用 20 MB 以内的 PNG、JPG、WebP、GIF 或 AVIF")); }
    finally { setImporting(false); if (fileInput.current) fileInput.current.value = ""; }
  }
  async function download() {
    if (!palette) return; setExporting(true);
    try {
      const content = format === "svg" ? svgPalette(palette) : format === "css" ? cssPalette(palette) : JSON.stringify({ title: palette.title, colors: palette.colors, tags: palette.tags, note: palette.note, source: palette.source }, null, 2);
      await save(`palette-${row!.id}.${format}`, content, format === "svg" ? "image/svg+xml" : format === "css" ? "text/css" : "application/json"); kit.flash(uiText("已导出到下载目录"));
    } catch { kit.flash(uiText("导出失败，请重试")); } finally { setExporting(false); }
  }
  async function copyLink() {
    if (!row) return;
    try { const ctx = await myco!.ready; if (!ctx.page.url) throw new Error(); await copyText(publicAppLink(ctx.page.url, `#palette?id=${encodeURIComponent(row.id)}`)); }
    catch { kit.flash(uiText("此客户端暂不支持复制直达链接，请更新 App")); }
  }
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (draft || e.ctrlKey || e.metaKey || e.altKey || /^(INPUT|TEXTAREA|SELECT)$/.test((e.target as HTMLElement).tagName) || (e.target as HTMLElement).isContentEditable) return;
      if (e.key.toLowerCase() === "n") { e.preventDefault(); setDraft(makeDraft()); }
      if (e.key.toLowerCase() === "f" && row) { e.preventDefault(); void toggleFavorite(row.id); }
    };
    window.addEventListener("keydown", key); return () => window.removeEventListener("keydown", key);
  }, [draft, row?.id]);
  return <div className="pa-app">
    <header className="pa-top"><div className="pa-brand"><PaletteIcon size={21} strokeWidth={1.7} /><strong>{uiText("配色助手")}</strong><span>COLOR STUDIO</span></div><div className="pa-top-actions"><span className="pa-save-state" role="status">{kit.pending ? uiText("正在保存…") : kit.failed ? uiText("没保存上") : uiText("已同步")}</span><button className="btn pa-import" disabled={importing} onClick={() => fileInput.current?.click()}><ImagePlus className="ico" />{importing ? uiText("正在提色…") : uiText("图片提色")}</button><button className="btn btn-primary" onClick={() => setDraft(makeDraft())}><Plus className="ico" />{uiText("新建色卡")}</button></div></header>
    <input ref={fileInput} type="file" accept="image/png,image/jpeg,image/webp,image/gif,image/avif" hidden aria-label={uiText("选择提色图片")} onChange={e => { const f = e.target.files?.[0]; if (f) void importImage(f); }} />
    <div className="pa-layout"><aside className="pa-sidebar"><div className="pa-side-intro"><span className="pa-eyebrow">YOUR COLOR LIBRARY</span><h1>{uiText("收集灵感，\n找到你的色彩。")}</h1><p>{uiText("从一抹颜色，到完整的设计语言。")}</p></div><nav className="pa-nav" aria-label={uiText("色卡分类")}><button className={tab === "all" && !tag ? "is-active" : ""} onClick={() => { setTab("all"); setTag(""); }}><PaletteIcon className="ico" />{uiText("全部色卡")}<span>{all.length}</span></button><button className={tab === "saved" ? "is-active" : ""} onClick={() => { setTab("saved"); setTag(""); }}><Heart className="ico" />{uiText("喜欢的配色")}<span>{all.filter(r => isSaved(r.id)).length}</span></button></nav><div className="pa-tag-nav"><h2>{uiText("灵感标签")}</h2><div>{tags.map(t => <button key={t} className={tag === t ? "is-active" : ""} onClick={() => { setTab("all"); setTag(tag === t ? "" : t); }}>{t}</button>)}{!tags.length && <span className="pa-hint">{uiText("保存色卡时添加标签")}</span>}</div></div><div className="pa-side-bottom"><Sparkles className="ico" /><p>{uiText("图片只在本地分析，保存色卡时仅共享颜色与笔记。")}</p></div></aside>
    <main className="pa-main"><div className="pa-main-heading"><div><span className="pa-eyebrow">PALETTE / INSPIRATION</span><h2>{tag || (tab === "saved" ? uiText("喜欢的配色") : uiText("你的配色档案"))}<small>{filtered.length}</small></h2></div><label className="pa-search"><Search className="ico" /><input className="inp" aria-label={uiText("搜索色卡")} value={query} onChange={e => setQuery(e.target.value)} placeholder={uiText("搜索名称、标签或色值")} /></label></div>
    {routeMissing && <div className="pa-route-error" role="status">{uiText("找不到这张色卡，可能已被删除。你仍可浏览其他配色。")}</div>}
    {palette && row && <section className="pa-feature" aria-label={uiText("当前色卡")}><div className="pa-feature-heading"><div><span className="pa-eyebrow">SELECTED PALETTE</span><h2>{palette.title}</h2><p>{palette.note || uiText("为配色记录一点灵感。")}</p></div><div className="pa-row"><button className={`icon-btn ${isSaved(row.id) ? "pa-liked" : ""}`} title={isSaved(row.id) ? uiText("取消喜欢") : uiText("喜欢这组配色")} aria-label={isSaved(row.id) ? uiText("取消喜欢") : uiText("喜欢这组配色")} aria-pressed={isSaved(row.id)} onClick={() => void toggleFavorite(row.id)}><Heart className="ico" fill={isSaved(row.id) ? "currentColor" : "none"} /></button><button className="btn" onClick={() => setDraft({ id: row.id, version: row.version, data: structuredClone(palette) })}><Pencil className="ico" />{uiText("编辑配色")}</button></div></div><Swatches colors={palette.colors} /><div className="pa-feature-bottom"><div className="pa-tags">{palette.tags.map(t => <span key={t}>{t}</span>)}{palette.source && <span className="pa-source">{palette.source}</span>}</div><button className="tool" onClick={() => void copyText(palette.colors.map(c => hex(c.hex)).join(", "))}><Copy className="ico" />{uiText("复制色值")}</button></div>
    <div className="pa-analysis"><div className="pa-preview-wrap"><div className="pa-section-title"><h3>{uiText("看看实际搭配")}</h3><span className="pa-subtle">{uiText("界面与平面设计")}</span></div><DesignPreview key={row.id} palette={palette} /><div className="pa-export-row"><select className="sel" aria-label={uiText("导出格式")} value={format} onChange={e => setFormat(e.target.value)}><option value="svg">SVG {uiText("色卡")}</option><option value="css">CSS {uiText("变量")}</option><option value="json">JSON</option></select><button className="btn" disabled={exporting} onClick={() => void download()}><Download className="ico" />{uiText("导出")}</button><button className="icon-btn" title={uiText("复制直达链接")} aria-label={uiText("复制直达链接")} onClick={() => void copyLink()}><Link2 className="ico" /></button><span className="pa-grow" /><button className="icon-btn" title={uiText("复制为新色卡")} aria-label={uiText("复制为新色卡")} onClick={() => { const d = makeDraft(); d.data = { ...structuredClone(palette), title: (palette.title.slice(0, 65) + " · " + uiText("副本")).slice(0, 80), created_at: d.data.created_at, updated_at: d.data.updated_at }; setDraft(d); }}><Copy className="ico" /></button><button className="icon-btn" title={uiText("删除色卡")} aria-label={uiText("删除色卡")} onClick={() => void deletePalette(row.id)}><Trash2 className="ico" /></button></div></div><Contrast key={row.id} colors={palette.colors} /></div></section>}
    <div className="pa-library-title"><h3>{uiText("灵感色卡")}</h3><span className="pa-subtle">{uiText("点选查看 · 点击色值复制")}</span></div><div className="pa-grid">{filtered.map(r => <article key={r.id} className={`pa-card ${r.id === selected ? "is-selected" : ""}`}><button className="pa-card-open" aria-label={uiText("查看色卡 {0}", r.data.title)} onClick={() => { choose(r.id); document.querySelector(".pa-main")?.scrollTo({ top: 0, behavior: "smooth" }); }}><div className="pa-card-colors">{r.data.colors.map(c => <span key={c.id} style={{ background: c.hex }} />)}</div><div className="pa-card-caption"><strong>{r.data.title}</strong><ChevronRight className="ico" /></div><p>{r.data.tags.join(" / ") || uiText("未分类")}</p></button><button className={`icon-btn pa-card-heart ${isSaved(r.id) ? "pa-liked" : ""}`} aria-label={uiText("收藏色卡 {0}", r.data.title)} title={uiText("收藏色卡 {0}", r.data.title)} aria-pressed={isSaved(r.id)} onClick={() => void toggleFavorite(r.id)}><Heart className="ico" fill={isSaved(r.id) ? "currentColor" : "none"} /></button></article>)}</div>
    {!filtered.length && <div className="pa-empty"><PaletteIcon size={32} strokeWidth={1.2} /><h3>{all.length ? uiText("还没有符合条件的色卡") : uiText("从你的第一组配色开始")}</h3><p>{all.length ? uiText("试试其他关键词，或收藏喜欢的色卡。") : uiText("新建色卡，或从照片中提取灵感颜色。")}</p><button className="btn" onClick={() => { if (all.length) { setQuery(""); setTag(""); setTab("all"); } else setDraft(makeDraft()); }}>{all.length ? uiText("查看全部") : uiText("创建色卡")}</button></div>}
    {all.length < palettes.rows.length && <p className="pa-hint">{uiText("有 {0} 组色卡格式不完整，请让 agent 检查后再显示。", palettes.rows.length - all.length)}</p>}<p className="pa-library-foot">{uiText("色卡与喜欢状态在此页面共享 · N 新建 · F 收藏当前色卡")}</p></main></div>
    {draft && <Editor initial={draft} onClose={() => setDraft(null)} onSaved={choose} />}<Toast kit={kit} />
  </div>;
}
function DesignPreview({ palette }: { palette: Palette }) {
  const [surface, setSurface] = useState(palette.colors.length - 1), [accent, setAccent] = useState(0);
  const bg = palette.colors[Math.min(surface, palette.colors.length - 1)].hex, a = palette.colors[Math.min(accent, palette.colors.length - 1)].hex;
  const secondary = palette.colors[1].hex;
  return <><div className="pa-design-preview" style={{ background: bg, color: ink(bg) }}><div className="pa-preview-copy"><span>STUDIO / 001</span><h4>{uiText("美，源于\n恰好的平衡。")}</h4><p>{uiText("留一点空间，让灵感发生。")}</p><span className="pa-preview-cta" style={{ background: a, color: ink(a) }}>{uiText("开启灵感")}<ArrowUpRight size={14} /></span></div><div className="pa-preview-art" aria-hidden="true"><span style={{ background: a }} /><span style={{ background: secondary }} /><span style={{ background: palette.colors[Math.min(2, palette.colors.length - 1)].hex }} /></div></div><div className="pa-preview-selects"><label>{uiText("画布")}<select className="sel" aria-label={uiText("预览画布颜色")} value={Math.min(surface, palette.colors.length - 1)} onChange={e => setSurface(+e.target.value)}>{palette.colors.map((c, i) => <option key={c.id} value={i}>{hex(c.hex)}</option>)}</select></label><label>{uiText("强调色")}<select className="sel" aria-label={uiText("预览强调色")} value={Math.min(accent, palette.colors.length - 1)} onChange={e => setAccent(+e.target.value)}>{palette.colors.map((c, i) => <option key={c.id} value={i}>{hex(c.hex)}</option>)}</select></label><span className="pa-hint">HSL {hsl(a).map(Math.round).join(" / ")}</span></div></>;
}
mount(kit, App);
