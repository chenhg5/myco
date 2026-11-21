import { t as uiText } from "../_lib/i18n";
// The side panels: the files of the project (素材), text and censoring to
// add, notes for agents and people, and the properties of what is selected.
import { useEffect, useRef, useState } from "react";
import { Check, Copy, Droplets, FileVideo, Grid3x3, Loader2, MessageSquareText, Plus, RefreshCw, Trash2, Type, Upload, Volume2, VolumeX, Scissors } from "lucide-react";
import { clips, fmt, kit, media, notes, num, overlayList, overlays, placed, project, frameSize, total, RATIOS, SPEEDS, clamp, type OverlayKind } from "./model";
import { addClip, addNote, addOverlay, change, duplicateClip, freeLane, patch, removeClip, removeMedia, removeOverlay, setProject, split } from "./edit";
import { localFor } from "./media";
import { FocusProps } from "./focus";
import { player } from "./player";
import type { Sel } from "./timeline";

export type Importing = { name: string; key: string };

// ---- 素材 ----
export function MediaPanel({ importing, onImport, onRelink, setSel }: {
  importing: Importing[]; onImport: () => void; onRelink: (mediaId: string) => void; setSel: (s: Sel, reveal?: boolean) => void;
}) {
  const used = new Map<string, number>();
  for (const c of clips.rows) used.set(String(c.data.media), (used.get(String(c.data.media)) || 0) + 1);
  return (
    <div className="pane">
      <button type="button" className="import" onClick={onImport}><Upload className="ico" />{uiText("导入视频")}</button>
      <div className="media-grid">
        {importing.map((f) => (
          <div key={f.key} className="media is-busy">
            <div className="media-pic"><Loader2 className="ico spin" /></div>
            <div className="media-name">{f.name}</div>
          </div>
        ))}
        {media.rows.map((r) => {
          const d = r.data, l = localFor(String(d.key || ""));
          const pic = l?.thumbs[0]?.url || String(d.poster || "");
          return (
            <div key={r.id} className={`media${l ? "" : " is-missing"}`} draggable={!!l}
              onDragStart={(e) => { e.dataTransfer.setData("application/x-myco-media", r.id); e.dataTransfer.effectAllowed = "copy"; }}
              title={`${d.name || uiText("视频")}${d.width ? ` · ${d.width}×${d.height}` : ""}`}>
              <div className="media-pic">
                {pic ? <img src={pic} alt="" draggable={false} /> : l ? <FileVideo className="ico" /> : null}
                <span className="media-dur">{fmt(num(d.duration, 0))}</span>
                {used.get(r.id) ? <span className="media-used">{uiText("已用")}{" "}{used.get(r.id)}</span> : null}
                {l ? (
                  <button type="button" className="media-add" title={uiText("放到时间线末尾")} aria-label={uiText("放到时间线末尾")}
                    onClick={() => { const id = addClip(r.id); setSel({ kind: "clip", id }, false); player.seek(total() - 0.001); }}><Plus className="ico" /></button>
                ) : (
                  <button type="button" className="media-relink" onClick={() => onRelink(r.id)}><RefreshCw className="ico" />{uiText("本机没有，选文件")}</button>
                )}
              </div>
              <div className="media-row">
                <span className="media-name">{d.name || uiText("视频")}</span>
                <button type="button" className="icon-btn media-del" title={uiText("从项目里去掉（连同时间线上用到它的片段）")} aria-label={uiText("去掉")}
                  onClick={() => { removeMedia(r.id); kit.flash(uiText("已去掉「{0}」", d.name || "视频")); }}><Trash2 className="ico" /></button>
              </div>
            </div>
          );
        })}
      </div>
      {!media.rows.length && !importing.length && (
        <p className="pane-hint">{uiText("支持 mp4、mov、webm。文件只存在这台电脑的浏览器里，不会上传；换一台电脑打开时，选一下同一个文件就能接着剪。")}</p>
      )}
    </div>
  );
}

// ---- 文字 ----
const TEXTS: { name: string; sample: string; o: { size: number; y: number; h: number; style: string; color: string; w: number; x?: number } }[] = [
  { name: uiText("标题"), sample: uiText("大标题"), o: { size: 0.1, y: 0.36, h: 0.2, style: "shadow", color: "#ffffff", w: 0.8 } },
  { name: uiText("字幕"), sample: uiText("字幕放在底部"), o: { size: 0.05, y: 0.82, h: 0.1, style: "box", color: "#ffffff", w: 0.8 } },
  { name: uiText("描边"), sample: uiText("描边大字"), o: { size: 0.08, y: 0.12, h: 0.16, style: "outline", color: "#ffe14d", w: 0.8 } },
  { name: uiText("角标"), sample: uiText("角标"), o: { size: 0.04, y: 0.05, h: 0.08, style: "box", color: "#ffffff", w: 0.24, x: 0.04 } },
];
export function TextPanel({ setSel }: { setSel: (s: Sel) => void }) {
  const add = (i: number) => {
    const p = TEXTS[i];
    const T = total();
    const start = Math.floor((T > 3 ? Math.min(player.time, T - 3) : 0) * 1000) / 1000, end = start + 3;
    const id = addOverlay({ type: "text", text: p.sample, start, end, x: p.o.x ?? (1 - p.o.w) / 2, y: p.o.y, w: p.o.w, h: p.o.h, size: p.o.size, style: p.o.style, color: p.o.color, lane: freeLane(start, end, 0) });
    setSel({ kind: "overlay", id });
  };
  return (
    <div className="pane">
      <div className="presets">
        {TEXTS.map((p, i) => (
          <button key={p.name} type="button" className={`preset is-${p.o.style}`} onClick={() => add(i)}>
            <span className="preset-pic"><span style={{ color: p.o.color }}>{p.sample}</span></span>
            <span className="preset-name">{p.name}</span>
          </button>
        ))}
      </div>
      <p className="pane-hint">{uiText("点一个样式，在播放头处加 3 秒文字。在画面上拖动它换位置，在时间线上拖两端改时长。")}</p>
    </div>
  );
}

// ---- 打码 ----
export function CensorPanel({ setSel }: { setSel: (s: Sel) => void }) {
  const add = (type: OverlayKind) => {
    const start = Math.floor(player.time * 1000) / 1000;
    const c = placed().find((x) => start >= x.start && start < x.end);
    const end = c ? Math.max(start + 1, c.end) : start + 3;
    const id = addOverlay({ type, start, end: Math.min(end, start + 3600), x: 0.375, y: 0.375, w: 0.25, h: 0.25, strength: 0.5, lane: freeLane(start, end, 1) });
    setSel({ kind: "overlay", id });
  };
  return (
    <div className="pane">
      <div className="presets">
        <button type="button" className="preset" onClick={() => add("blur")}>
          <span className="preset-pic is-blur"><Droplets className="ico" /></span><span className="preset-name">{uiText("模糊")}</span>
        </button>
        <button type="button" className="preset" onClick={() => add("mosaic")}>
          <span className="preset-pic is-mosaic"><Grid3x3 className="ico" /></span><span className="preset-name">{uiText("马赛克")}</span>
        </button>
      </div>
      <p className="pane-hint">{uiText("在播放头处加一块打码，默认盖到这段视频结束。在画面上拖动方框盖住人脸、车牌，拖四角改大小；在时间线上拖两端改出现的时间。")}</p>
    </div>
  );
}

// ---- 备注 ----
export function NotesPanel({ me, focus, pageId, title }: { me: string; focus: string; pageId: string; title: string }) {
  const [text, setText] = useState("");
  const list = [...notes.rows].sort((a, b) => num(a.data.time, 0) - num(b.data.time, 0));
  const open = list.filter((n) => !n.data.done).length;
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { if (focus) ref.current?.querySelector(`[data-id="${focus}"]`)?.scrollIntoView({ block: "nearest" }); }, [focus]);
  const add = () => {
    const s = text.trim();
    if (!s) return;
    addNote(Math.round(player.time * 10) / 10, s, me);
    setText("");
  };
  return (
    <div className="pane" ref={ref}>
      <AgentCard pageId={pageId} title={title} open={open} />
      <div className="note-add">
        <textarea className="inp" rows={2} value={text} placeholder={uiText("在 {0} 留一条备注，比如「这段删掉」「这里给人脸打码」", fmt(player.time, true))}
          onChange={(e) => setText(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); add(); } }} />
        <button type="button" className="btn btn-primary" disabled={!text.trim()} onClick={add}><MessageSquareText className="ico" />{uiText("加备注")}</button>
      </div>
      <div className="notes">
        {list.map((n) => (
          <div key={n.id} data-id={n.id} className={`note-item${n.data.done ? " is-done" : ""}${n.id === focus ? " is-focus" : ""}`}>
            <button type="button" className="note-check" aria-label={n.data.done ? uiText("标成没处理") : uiText("标成已处理")} onClick={() => patch(notes, n.id, { done: !n.data.done })}>
              {n.data.done ? <Check className="ico" /> : null}
            </button>
            <div className="note-body">
              <button type="button" className="note-time" onClick={() => player.seek(num(n.data.time, 0))}>{fmt(num(n.data.time, 0), true)}</button>
              <span className="note-text">{String(n.data.text || "")}</span>
              {n.data.by ? <span className="note-by">{String(n.data.by)}</span> : null}
            </div>
            <button type="button" className="icon-btn" aria-label={uiText("删除备注")} onClick={() => change([[notes, n.id, null]])}><Trash2 className="ico" /></button>
          </div>
        ))}
      </div>
    </div>
  );
}

export function agentText(pageId: string, title: string) {
  return `帮我处理 myco 视频剪辑「${title || "未命名视频"}」（网页 ${pageId}）里没处理的备注。` +
    `先运行 myco template get myco/video 看它的数据怎么读写，再用 myco page data list --page ${pageId} 读剪辑方案和备注，` +
    `按备注改片段、文字和打码，每改完一条就把那条备注的 done 设成 true。`;
}
function AgentCard({ pageId, title, open }: { pageId: string; title: string; open: number }) {
  const copy = async () => {
    try { await navigator.clipboard.writeText(agentText(pageId, title)); kit.flash(uiText("已复制，发给任意一个 agent 就行")); }
    catch { kit.flash(uiText("没复制上，请手动选中复制")); }
  };
  return (
    <div className="agent-card">
      <b>{uiText("让 agent 一起剪")}</b>
      <span>{uiText("在时间点上留备注，然后把下面这段话发给 agent。它会读剪辑方案、按备注改，改动马上出现在这里。")}{open ? uiText("现在有 {0} 条没处理。", open) : ""}</span>
      <button type="button" className="btn" onClick={copy}><Copy className="ico" />{uiText("复制给 agent 的话")}</button>
    </div>
  );
}

// ---- 属性 ----
const COLORS = ["#ffffff", "#ffe14d", "#ff5c5c", "#4c88ff", "#3ddc84", "#111111"];
const BGS = ["#000000", "#ffffff", "#1f2329", "#f5f0e6"];

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="field"><div className="field-label">{label}</div>{children}</div>;
}
function Seconds({ value, onChange, min = 0 }: { value: number; onChange: (v: number) => void; min?: number }) {
  const [s, setS] = useState(value.toFixed(2));
  useEffect(() => setS(value.toFixed(2)), [value]);
  const done = () => { const v = Number(s); if (Number.isFinite(v) && v >= min) onChange(v); else setS(value.toFixed(2)); };
  return <input className="inp sec" value={s} inputMode="decimal" onChange={(e) => setS(e.target.value)} onBlur={done} onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} />;
}

export function Inspector({ sel, setSel }: { sel: Sel; setSel: (s: Sel) => void }) {
  if (sel?.kind === "clip") {
    const c = placed().find((x) => x.id === sel.id);
    if (c) return <ClipProps id={c.id} setSel={setSel} />;
  }
  if (sel?.kind === "overlay") {
    const o = overlayList().find((x) => x.id === sel.id);
    if (o) return <OverlayProps id={o.id} setSel={setSel} />;
  }
  return <ProjectProps />;
}

function ClipProps({ id, setSel }: { id: string; setSel: (s: Sel) => void }) {
  const c = placed().find((x) => x.id === id)!;
  const m = media.get(c.media)?.data;
  const custom = !SPEEDS.includes(c.speed);
  return (
    <div className="pane props">
      <div className="props-head"><FileVideo className="ico" /><b>{m?.name || uiText("视频片段")}</b></div>
      <Field label={uiText("变速")}>
        <div className="chips">
          {SPEEDS.map((s) => (
            <button key={s} type="button" className={`chip${c.speed === s ? " is-on" : ""}`} onClick={() => patch(clips, id, { speed: s })}>{s}x</button>
          ))}
        </div>
        <div className="row">
          <input type="range" min={-2} max={2} step={0.01} aria-label={uiText("速度")} value={Math.log2(c.speed)}
            onChange={(e) => patch(clips, id, { speed: Math.round(Math.pow(2, Number(e.target.value)) * 100) / 100 }, `speed-${id}`)} />
          <span className={`row-v${custom ? " is-on" : ""}`}>{c.speed}x</span>
        </div>
        <div className="field-note">{uiText("变速后时长")}{" "}{fmt(c.end - c.start, true)}{uiText("，声音不变调")}</div>
      </Field>
      <Field label={uiText("音量")}>
        <div className="row">
          <button type="button" className="icon-btn" aria-label={c.muted ? uiText("取消静音") : uiText("静音")} title={c.muted ? uiText("取消静音") : uiText("静音")} onClick={() => patch(clips, id, { muted: !c.muted })}>
            {c.muted ? <VolumeX className="ico" /> : <Volume2 className="ico" />}
          </button>
          <input type="range" min={0} max={2} step={0.01} aria-label={uiText("音量")} value={c.muted ? 0 : c.volume}
            onChange={(e) => patch(clips, id, { volume: Number(e.target.value), muted: false }, `vol-${id}`)} />
          <span className="row-v">{Math.round((c.muted ? 0 : c.volume) * 100)}%</span>
        </div>
      </Field>
      <Field label={uiText("用原片的哪一段")}>
        <div className="row">
          <Seconds value={c.in} onChange={(v) => patch(clips, id, { in: clamp(v, 0, c.out - 0.05) })} />
          <span className="faint">{uiText("到")}</span>
          <Seconds value={c.out} onChange={(v) => patch(clips, id, { out: clamp(v, c.in + 0.05, num(m?.duration, v) || v) })} />
          <span className="faint">{uiText("秒")}</span>
        </div>
        <div className="field-note">{uiText("原片长")}{" "}{fmt(num(m?.duration, 0), true)}{uiText("；在时间线上拖片段两端也能裁剪")}</div>
      </Field>
      <div className="props-actions">
        <button type="button" className="btn" onClick={() => { const n = split(player.time); if (n) setSel({ kind: "clip", id: n }); else kit.flash(uiText("把播放头放到这段中间再分割")); }}><Scissors className="ico" />{uiText("分割")}</button>
        <button type="button" className="btn" onClick={() => { const n = duplicateClip(id); if (n) setSel({ kind: "clip", id: n }); }}><Copy className="ico" />{uiText("复制")}</button>
        <button type="button" className="btn is-danger" onClick={() => { removeClip(id); setSel(null); }}><Trash2 className="ico" />{uiText("删除")}</button>
      </div>
    </div>
  );
}

function OverlayProps({ id, setSel }: { id: string; setSel: (s: Sel) => void }) {
  const o = overlayList().find((x) => x.id === id)!;
  const set = (p: Record<string, unknown>, key?: string) => patch(overlays, id, p, key);
  if (o.type === "zoom") return <FocusProps id={id} />;
  const isText = o.type === "text";
  return (
    <div className="pane props">
      <div className="props-head">{isText ? <Type className="ico" /> : o.type === "blur" ? <Droplets className="ico" /> : <Grid3x3 className="ico" />}<b>{isText ? uiText("文字") : uiText("打码")}</b></div>
      {isText ? (
        <>
          <Field label={uiText("内容")}>
            <textarea className="inp" rows={3} value={o.text} onChange={(e) => set({ text: e.target.value }, `text-${id}`)} placeholder={uiText("写点什么")} />
          </Field>
          <Field label={uiText("样式")}>
            <div className="seg">
              {[["shadow", "阴影"], ["outline", "描边"], ["box", "底色"], ["plain", "无"]].map(([v, n]) => (
                <button key={v} type="button" className={o.style === v ? "is-on" : ""} onClick={() => set({ style: v })}>{uiText(String(n))}</button>
              ))}
            </div>
          </Field>
          <Field label={uiText("颜色")}>
            <div className="swatches">
              {COLORS.map((c) => <button key={c} type="button" aria-label={c} className={`swatch${o.color.toLowerCase() === c ? " is-on" : ""}`} style={{ background: c }} onClick={() => set({ color: c })} />)}
              <input type="color" aria-label={uiText("别的颜色")} value={/^#[0-9a-f]{6}$/i.test(o.color) ? o.color : "#ffffff"} onChange={(e) => set({ color: e.target.value }, `color-${id}`)} />
            </div>
          </Field>
          <Field label={uiText("字号")}>
            <div className="row">
              <input type="range" min={0.02} max={0.2} step={0.002} aria-label={uiText("字号")} value={o.size} onChange={(e) => set({ size: Number(e.target.value) }, `size-${id}`)} />
              <span className="row-v">{Math.round(o.size * 1000) / 10}%</span>
            </div>
          </Field>
          <Field label={uiText("位置")}>
            <div className="seg">
              {[["上", 0.06], ["居中", 0.5 - o.h / 2], ["下", 0.94 - o.h]].map(([n, y]) => (
                <button key={n as string} type="button" onClick={() => set({ y, x: (1 - o.w) / 2 })}>{uiText(n as string)}</button>
              ))}
            </div>
          </Field>
        </>
      ) : (
        <>
          <Field label={uiText("方式")}>
            <div className="seg">
              <button type="button" className={o.type === "blur" ? "is-on" : ""} onClick={() => set({ type: "blur" })}>{uiText("模糊")}</button>
              <button type="button" className={o.type === "mosaic" ? "is-on" : ""} onClick={() => set({ type: "mosaic" })}>{uiText("马赛克")}</button>
            </div>
          </Field>
          <Field label={uiText("强度")}>
            <div className="row">
              <input type="range" min={0} max={1} step={0.01} aria-label={uiText("强度")} value={o.strength} onChange={(e) => set({ strength: Number(e.target.value) }, `str-${id}`)} />
              <span className="row-v">{Math.round(o.strength * 100)}</span>
            </div>
          </Field>
          <Field label={uiText("范围")}>
            <div className="row">
              <button type="button" className="btn" onClick={() => set({ x: 0, y: 0, w: 1, h: 1 })}>{uiText("盖满画面")}</button>
              <button type="button" className="btn" onClick={() => set({ x: 0.375, y: 0.375, w: 0.25, h: 0.25 })}>{uiText("放回中间")}</button>
            </div>
          </Field>
        </>
      )}
      <Field label={uiText("出现时间")}>
        <div className="row">
          <Seconds value={o.start} onChange={(v) => set({ start: v, end: Math.max(v + 0.1, o.end) })} />
          <span className="faint">{uiText("到")}</span>
          <Seconds value={o.end} onChange={(v) => set({ end: Math.max(o.start + 0.1, v) })} />
          <span className="faint">{uiText("秒")}</span>
        </div>
      </Field>
      <div className="props-actions">
        <button type="button" className="btn" onClick={() => { const n = addOverlay({ ...overlays.get(id)!.data, lane: freeLane(o.start, o.end, o.lane) }); setSel({ kind: "overlay", id: n }); }}><Copy className="ico" />{uiText("复制")}</button>
        <button type="button" className="btn is-danger" onClick={() => { removeOverlay(id); setSel(null); }}><Trash2 className="ico" />{uiText("删除")}</button>
      </div>
    </div>
  );
}

function ProjectProps() {
  const p = project.get("main")?.data || {};
  const list = placed();
  const { w, h } = frameSize(list);
  const bg = String(p.background || "#000000");
  return (
    <div className="pane props">
      <div className="props-head"><FileVideo className="ico" /><b>{uiText("作品")}</b></div>
      <Field label={uiText("画面比例")}>
        <select className="sel" value={String(p.ratio || "source")} onChange={(e) => setProject({ ratio: e.target.value })}>
          {RATIOS.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
        </select>
        <div className="field-note">{uiText("导出")}{" "}{w}×{h}{uiText("，30 帧每秒")}</div>
      </Field>
      <Field label={uiText("背景")}>
        <div className="swatches">
          {BGS.map((c) => <button key={c} type="button" aria-label={c} className={`swatch${bg === c ? " is-on" : ""}`} style={{ background: c }} onClick={() => setProject({ background: c })} />)}
          <input type="color" aria-label={uiText("别的颜色")} value={/^#[0-9a-f]{6}$/i.test(bg) ? bg : "#000000"} onChange={(e) => setProject({ background: e.target.value })} />
        </div>
        <div className="field-note">{uiText("比例和视频不一样时，空出来的地方是这个颜色")}</div>
      </Field>
      <div className="stats">
        <div><b>{fmt(total(list), true)}</b><span>{uiText("总时长")}</span></div>
        <div><b>{list.length}</b><span>{uiText("片段")}</span></div>
        <div><b>{overlays.rows.length}</b><span>{uiText("文字和打码")}</span></div>
      </div>
      <div className="keys">
        <div className="field-label">{uiText("快捷键")}</div>
        {[["空格", "播放 / 暂停"], ["S", "在播放头处分割"], ["Delete", "删除选中的"], ["⌘Z / ⇧⌘Z", "撤销 / 重做"], ["← →", "一帧一帧走（加 ⇧ 一秒）"], ["⌘ + 滚轮", "缩放时间线"]].map(([k, v]) => (
          <div key={k} className="key"><kbd>{k}</kbd><span>{v}</span></div>
        ))}
      </div>
    </div>
  );
}
