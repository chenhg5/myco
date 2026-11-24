// Draft editor and reusable palette/contrast views. Editing stays local until Save.
import { useEffect, useRef, useState } from "react";
import { Check, Copy, LockKeyhole, UnlockKeyhole, Plus, X, ArrowLeftRight, RefreshCw } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { uid } from "../_lib/appkit";
import { kit, palettes } from "./model";
import { putPalette } from "./edit";
import { hex, hsl, hslHex, rgb, contrast, contrastPass, harmony, ink, validPalette, type Palette, type Color, type Harmony } from "./engine";
export async function copyText(text: string) {
  try { await navigator.clipboard.writeText(text); kit.flash(uiText("已复制")); }
  catch { kit.flash(uiText("复制失败，请长按或选中文字复制")); }
}
export const modes = () => [
  ["analogous", uiText("邻近色")], ["complementary", uiText("互补色")], ["triadic", uiText("三角色")], ["mono", uiText("单色层次")],
] as const;
export function Swatches({ colors, selected, onSelect }: { colors: Color[]; selected?: number; onSelect?: (index: number) => void }) {
  return <div className={`pa-swatches ${colors.length > 6 ? "is-many" : ""}`}>{colors.map((c, i) => <button key={c.id} type="button" className={selected === i ? "is-selected" : ""} style={{ background: hex(c.hex)!, color: ink(c.hex) }} onClick={() => onSelect ? onSelect(i) : void copyText(hex(c.hex)!)} aria-label={uiText("选择颜色 {0}", c.hex)} aria-pressed={onSelect ? selected === i : undefined}><span className="pa-swatch-name">{c.name || String(i + 1).padStart(2, "0")}</span><span className="pa-swatch-hex">{hex(c.hex)} {onSelect ? selected === i && <Check size={14} /> : <Copy size={12} />}</span></button>)}</div>;
}
export function Contrast({ colors }: { colors: Color[] }) {
  const [fg, setFg] = useState(0), [bg, setBg] = useState(colors.length - 1);
  const a = colors[Math.min(fg, colors.length - 1)], b = colors[Math.min(bg, colors.length - 1)], ratio = contrast(a.hex, b.hex), pass = contrastPass(ratio);
  const option = (c: Color) => `${c.name ? c.name + " · " : ""}${hex(c.hex)}`;
  return <section className="pa-contrast">
    <div className="pa-section-title"><h3>{uiText("文字对比度")}</h3><span className="pa-subtle">WCAG 2.x · sRGB</span></div>
    <div className="pa-contrast-selects"><label>{uiText("文字")}<select className="sel" aria-label={uiText("文字颜色")} value={Math.min(fg, colors.length - 1)} onChange={e => setFg(+e.target.value)}>{colors.map((c, i) => <option key={c.id} value={i}>{option(c)}</option>)}</select></label><button className="icon-btn" aria-label={uiText("交换文字与背景")} title={uiText("交换文字与背景")} onClick={() => { setFg(bg); setBg(fg); }}><ArrowLeftRight className="ico" /></button><label>{uiText("背景")}<select className="sel" aria-label={uiText("背景颜色")} value={Math.min(bg, colors.length - 1)} onChange={e => setBg(+e.target.value)}>{colors.map((c, i) => <option key={c.id} value={i}>{option(c)}</option>)}</select></label></div>
    <div className="pa-type-preview" style={{ color: a.hex, background: b.hex }}><span>Aa</span><div><strong>{uiText("让好设计清晰可见")}</strong><p>{uiText("颜色，也是一种表达。")}</p></div></div>
    <div className="pa-ratio"><strong data-testid="contrast-ratio">{ratio.toFixed(2)}<small>:1</small></strong><div><span>{uiText("正文 AA")} <b>{pass.normal ? uiText("通过") : uiText("未通过")}</b></span><span>{uiText("大字 AA")} <b>{pass.large ? uiText("通过") : uiText("未通过")}</b></span><span>{uiText("正文 AAA")} <b>{pass.enhanced ? uiText("通过") : uiText("未通过")}</b></span></div></div>
    <p className="pa-hint">{uiText("正文 AA ≥ 4.5，大字 AA ≥ 3，正文 AAA ≥ 7。大字指 24px，或约 18.7px 粗体。按未舍入值判定；仅检查不透明文字与纯色背景。")}</p>
  </section>;
}
export type Draft = { id: string; version: number; data: Palette };
export function Editor({ initial, onClose, onSaved }: { initial: Draft; onClose: () => void; onSaved: (id: string) => void }) {
  const [draft, setDraft] = useState(() => structuredClone(initial)), [index, setIndex] = useState(0), [mode, setMode] = useState<Harmony>("analogous"), [tagInput, setTagInput] = useState(initial.data.tags.join(", ")), [error, setError] = useState(""), [busy, setBusy] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);
  const color = draft.data.colors[index], [hexInput, setHexInput] = useState(color.hex);
  const [h, s, l] = hsl(color.hex);
  useEffect(() => { const node = dialog.current!; node.showModal(); return () => node.close(); }, []);
  useEffect(() => { setHexInput(color.hex); }, [color.hex, color.id]);
  const patch = (p: Partial<Palette>) => setDraft(d => ({ ...d, data: { ...d.data, ...p } }));
  const changeColor = (p: Partial<Color>) => patch({ colors: draft.data.colors.map((c, i) => i === index ? { ...c, ...p } : c) });
  const reload = () => { const row = palettes.get(draft.id); if (!row) { setError(uiText("此色卡已删除，请另存为新色卡")); return; } if (!validPalette(row.data)) { setError(uiText("请检查名称和色值，色卡需包含 2–12 种颜色")); return; } setDraft({ id: row.id, version: row.version, data: structuredClone(row.data) }); setIndex(0); setTagInput(row.data.tags.join(", ")); setError(""); };
  async function submit(asNew = false) {
    const value = hex(hexInput); if (!value) { setError(uiText("请输入有效的 HEX 色值，例如 #8B5E3C")); return; }
    setBusy(true); setError("");
    const next = { ...draft.data, tags: [...new Set(tagInput.split(/[,，]/).map(t => t.trim()).filter(Boolean))], colors: draft.data.colors.map((c, i) => i === index ? { ...c, hex: value } : c) };
    const id = asNew ? uid("p") : draft.id;
    try { const result = await kit.track(putPalette(id, next, asNew ? 0 : draft.version).catch(e => { setError(e?.code === "conflict" ? uiText("色卡已被修改，请载入最新版本后重试") : e?.message || uiText("保存失败，请重试")); throw e; })); if (result) { kit.flash(uiText("色卡已保存")); onSaved(id); onClose(); } }
    finally { setBusy(false); }
  }
  return <dialog ref={dialog} className="pa-dialog" aria-labelledby="pa-editor-title" onCancel={e => { e.preventDefault(); onClose(); }}><form onSubmit={e => { e.preventDefault(); void submit(); }}>
    <header><div><span className="pa-eyebrow">COLOR STUDIO</span><h2 id="pa-editor-title">{initial.version ? uiText("编辑色卡") : uiText("创建色卡")}</h2></div><button type="button" className="icon-btn" title={uiText("关闭")} aria-label={uiText("关闭")} onClick={onClose}><X className="ico" /></button></header>
    <div className="pa-editor-body"><Swatches colors={draft.data.colors} selected={index} onSelect={setIndex} /><div className="pa-editor-grid">
      <section className="pa-color-editor"><h3>{uiText("调整颜色")}</h3><div className="pa-color-input"><input type="color" aria-label={uiText("取色器")} value={hex(color.hex)!} onChange={e => changeColor({ hex: e.target.value.toUpperCase() })} /><label>HEX<input className="inp" aria-label="HEX" value={hexInput} onChange={e => setHexInput(e.target.value)} onBlur={() => { const v = hex(hexInput); if (v) { changeColor({ hex: v }); setHexInput(v); } }} maxLength={7} /></label><button type="button" className="icon-btn" title={color.locked ? uiText("解锁颜色") : uiText("锁定颜色")} aria-label={color.locked ? uiText("解锁颜色") : uiText("锁定颜色")} aria-pressed={color.locked} onClick={() => changeColor({ locked: !color.locked })}>{color.locked ? <LockKeyhole className="ico" /> : <UnlockKeyhole className="ico" />}</button></div>
      <label className="pa-field">{uiText("颜色名称")}<input className="inp" value={color.name} maxLength={40} onChange={e => changeColor({ name: e.target.value })} placeholder={uiText("例如：主色、背景色")} /></label>
      {[[uiText("色相"), h, 360, "°"], [uiText("饱和度"), s, 100, "%"], [uiText("明度"), l, 100, "%"]].map(([name, value, max, suffix], i) => <label className="pa-slider" key={name}><span>{name}<b>{Math.round(Number(value))}{suffix}</b></span><input type="range" aria-label={String(name)} min="0" max={max} value={value} onChange={e => { const values: [number, number, number] = [h, s, l]; values[i] = +e.target.value; changeColor({ hex: hslHex(values) }); }} /></label>)}
      <p className="pa-hint pa-mono">RGB {rgb(color.hex).join(" / ")}</p><div className="pa-row"><button type="button" className="btn" disabled={draft.data.colors.length >= 12} onClick={() => { const colors = [...draft.data.colors, { id: uid("c"), hex: "#E7DCCF", name: "", locked: false }]; patch({ colors }); setIndex(colors.length - 1); }}><Plus className="ico" />{uiText("添加颜色")}</button><button type="button" className="tool" disabled={draft.data.colors.length <= 2} onClick={() => { patch({ colors: draft.data.colors.filter((_, i) => i !== index) }); setIndex(Math.max(0, index - 1)); }}>{uiText("移除颜色")}</button></div>
      <div className="pa-harmony"><label>{uiText("以选中色为基准")}<select className="sel" value={mode} onChange={e => setMode(e.target.value as Harmony)}>{modes().map(([key, name]) => <option key={key} value={key}>{name}</option>)}</select></label><button type="button" className="btn" onClick={() => { const values = harmony(color.hex, mode, draft.data.colors.length); patch({ colors: draft.data.colors.map((c, i) => c.locked || i === index ? c : { ...c, hex: values[(i - index + values.length) % values.length] }) }); }}><RefreshCw className="ico" />{uiText("生成搭配")}</button><p className="pa-hint">{uiText("保留选中色和锁定色，按 HSL 色轮生成建议。")}</p></div>
      </section><section className="pa-meta"><label className="pa-field">{uiText("色卡名称")}<input className="inp" aria-label={uiText("色卡名称")} value={draft.data.title} required maxLength={80} onChange={e => patch({ title: e.target.value })} /></label><label className="pa-field">{uiText("标签")}<input className="inp" value={tagInput} maxLength={240} onChange={e => setTagInput(e.target.value)} placeholder={uiText("用逗号分隔，例如：品牌，网页")} /></label><label className="pa-field">{uiText("设计笔记")}<textarea className="inp" rows={4} maxLength={2000} value={draft.data.note} onChange={e => patch({ note: e.target.value })} placeholder={uiText("记录灵感、使用场景与颜色分工")} /></label><label className="pa-field">{uiText("灵感来源")}<input className="inp" maxLength={200} value={draft.data.source} onChange={e => patch({ source: e.target.value })} placeholder={uiText("例如：旅行照片、品牌研究")} /></label><Contrast colors={draft.data.colors} /></section>
    </div></div>
    <footer>{error && <div className="pa-error" role="alert">{error}<button type="button" className="link-btn" onClick={reload}>{uiText("载入最新版本")}</button><button type="button" className="link-btn" disabled={busy} onClick={() => void submit(true)}>{uiText("另存为新色卡")}</button></div>}<span className="pa-hint">{uiText("保存后，此页面的成员与 agent 都能看到。")}</span><div className="pa-row"><button type="button" className="btn" onClick={onClose}>{uiText("取消")}</button><button type="submit" className="btn btn-primary" disabled={busy}>{busy ? uiText("正在保存…") : uiText("保存色卡")}</button></div></footer>
  </form></dialog>;
}
