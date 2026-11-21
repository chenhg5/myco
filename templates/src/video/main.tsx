import { t as uiText } from "../_lib/i18n";
// 视频剪辑: a video editor people and agents use together. The videos stay
// on the computer that opened them; the edit (clips with their trims and
// speeds, text, blur and mosaic regions, notes) is the page's records, so
// an agent edits it with the CLI and the page follows at once. The mp4 is
// made here in the browser.
import "../_lib/sheet.css";
import "./video.css";
import { useCallback, useEffect, useRef, useState } from "react";
import { Bot, Download, Droplets, Film, Loader2, MessageSquareText, SlidersHorizontal, Type, X, ZoomIn } from "lucide-react";
import { Bar, Toast, mount, titleSaver, uid, useKit } from "../_lib/appkit";
import { myco, save } from "../_lib/myco";
import { Popover, rectOf } from "../_lib/ui";
import { clips, frameSize, kit, media, notes, overlays, placed, project, total, RATIOS, clamp, sizeChoices } from "./model";
import { addClip, change, patch, redo, removeClip, removeOverlay, setProject, split, undo, duplicateClip } from "./edit";
import { keep, keyOf, onLocal, probe, restore } from "./media";
import { Recorder } from "./recorder";
import { FocusPanel } from "./focus";
import { player } from "./player";
import { Timeline, type Sel } from "./timeline";
import { Stage } from "./stage";
import { CensorPanel, Inspector, MediaPanel, NotesPanel, TextPanel, agentText, type Importing } from "./panels";
import { QUALITIES, estimate, render, type ExportOptions, type Progress } from "./export";

const title = titleSaver(kit, project);

type Tab = "media" | "text" | "censor" | "notes" | "props" | "focus";
const TABS: [Tab, string, typeof Film][] = [["media", "素材", Film], ["text", "文字", Type], ["censor", "打码", Droplets], ["notes", "备注", MessageSquareText]];

function useNarrow() {
  const q = window.matchMedia?.("(max-width: 980px)");
  const [n, setN] = useState(!!q?.matches);
  useEffect(() => { if (!q) return; const f = () => setN(q.matches); q.addEventListener("change", f); return () => q.removeEventListener("change", f); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return n;
}

export function VideoApp({ recording = false }: { recording?: boolean }) {
  useKit(kit);
  const [, setLocalVer] = useState(0);
  useEffect(() => onLocal(() => setLocalVer((n) => n + 1)), []);
  const narrow = useNarrow();
  const [recordRequest, setRecordRequest] = useState(0);
  const [tab, setTab] = useState<Tab>("media");
  const [sel, setSelState] = useState<Sel>(null);
  const [pps, setPps] = useState(40);
  const [importing, setImporting] = useState<Importing[]>([]);
  const [noteFocus, setNoteFocus] = useState("");
  const [exporting, setExporting] = useState<Exp | null>(null);
  const [agentAt, setAgentAt] = useState<null | ReturnType<typeof rectOf>>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const relinkFor = useRef("");
  const cancelRef = useRef(false);
  const fitted = useRef(false);

  const setSel = useCallback((s: Sel, reveal = true) => {
    setSelState(s);
    player.selected = s?.id || "";
    if (s && narrow && reveal) setTab("props");
  }, [narrow]);

  // a selection whose record went away (undo, someone else) is dropped
  if (sel && ((sel.kind === "clip" && !clips.get(sel.id)) || (sel.kind === "overlay" && !overlays.get(sel.id)))) setTimeout(() => setSel(null));

  // the timeline starts zoomed to fit
  useEffect(() => {
    if (fitted.current) return;
    const T = total();
    if (T > 0) { fitted.current = true; setPps(clamp((window.innerWidth - 120) / Math.max(T * 1.15, 5), 2, 200)); }
  });

  // ---- files ----
  async function importFiles(files: File[], index?: number) {
    let imported = 0;
    let at = index;
    for (const f of files) {
      const tmpKey = uid("i");
      setImporting((l) => [...l, { name: f.name, key: tmpKey }]);
      try {
        const key = await keep(f);
        let rec = media.rows.find((r) => r.data.key === key);
        if (!rec) {
          const p = await probe(f);
          if (!p.duration) throw new Error(uiText("「{0}」读不出来：换成 mp4（H.264）试试", f.name));
          const id = uid("m");
          change([[media, id, { name: f.name, size: f.size, duration: Math.round(p.duration * 1000) / 1000, width: p.width, height: p.height, audio: p.audio, key, poster: !recording && p.poster.length < 40000 ? p.poster : "" }]]);
          rec = media.get(id)!;
        }
        if (at !== undefined || !clips.rows.length) {
          const id = addClip(rec.id, at ?? clips.rows.length);
          if (at !== undefined) at++;
          setSel({ kind: "clip", id }, false);
        }
        imported++;
      } catch (e) {
        kit.flash((e as Error).message || uiText("「{0}」没导入成功", f.name));
      } finally {
        setImporting((l) => l.filter((x) => x.key !== tmpKey));
      }
    }
    return imported > 0;
  }
  async function relink(file: File, mediaId: string) {
    const m = media.get(mediaId);
    if (!m) return;
    const key = await keyOf(file);
    if (key !== m.data.key) {
      if (!window.confirm(uiText("选的文件和原来的「{0}」不一样（大小或内容不同）。还是用它代替吗？大家看到的都会换成它。", m.data.name))) return;
      const p = await probe(file);
      await keep(file);
      patch(media, mediaId, { key, name: file.name, size: file.size, duration: p.duration || m.data.duration, width: p.width, height: p.height, audio: p.audio });
    } else {
      await keep(file);
      kit.flash(uiText("找到了，接着剪吧"));
    }
  }
  const pickFiles = () => { relinkFor.current = ""; fileInput.current?.click(); };
  const onRelink = (id: string) => { relinkFor.current = id; fileInput.current?.click(); };

  // files others added (or on this computer from before) come back from IndexedDB
  useEffect(() => { void restore(media.rows.map((r) => String(r.data.key || ""))); }, [kit.version]); // eslint-disable-line react-hooks/exhaustive-deps

  // ---- keys ----
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable)) return;
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key.toLowerCase() === "z") { e.preventDefault(); if (e.shiftKey) redo(); else undo(); return; }
      if (mod && e.key.toLowerCase() === "y") { e.preventDefault(); redo(); return; }
      if (mod && e.key.toLowerCase() === "d") { e.preventDefault(); if (sel?.kind === "clip") { const n = duplicateClip(sel.id); if (n) setSel({ kind: "clip", id: n }); } return; }
      if (mod) return;
      if (e.key === " ") { e.preventDefault(); player.toggle(); }
      else if (e.key === "s" || e.key === "S") { const n = split(player.time); if (n) setSel({ kind: "clip", id: n }); }
      else if (e.key === "Delete" || e.key === "Backspace") {
        if (sel?.kind === "clip") removeClip(sel.id); else if (sel?.kind === "overlay") removeOverlay(sel.id);
        if (sel) { e.preventDefault(); setSel(null); }
      }
      else if (e.key === "ArrowLeft") { e.preventDefault(); player.step(e.shiftKey ? -30 : -1); }
      else if (e.key === "ArrowRight") { e.preventDefault(); player.step(e.shiftKey ? 30 : 1); }
      else if (e.key === "Home") player.seek(0);
      else if (e.key === "End") player.seek(total());
      else if (e.key === "Escape") setSel(null);
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [sel, setSel]);

  // files dropped anywhere on the page
  useEffect(() => {
    const over = (e: DragEvent) => { if (e.dataTransfer?.types.includes("Files")) e.preventDefault(); };
    const drop = (e: DragEvent) => {
      if (!e.dataTransfer?.files.length || e.defaultPrevented) return;
      e.preventDefault();
      void importFiles(Array.from(e.dataTransfer.files).filter((f) => f.type.startsWith("video/") || /\.(mp4|mov|m4v|webm|mkv)$/i.test(f.name)));
    };
    window.addEventListener("dragover", over);
    window.addEventListener("drop", drop);
    return () => { window.removeEventListener("dragover", over); window.removeEventListener("drop", drop); };
  });

  // ---- export ----
  /** Saves the mp4 to Downloads through the App; when that fails, the video
   *  is kept here to try again (an older App saves no video). */
  async function keepFile(name: string, blob: Blob, silent: boolean, secs: number, drop: () => void) {
    const OLD_APP = "这个版本的 myco App 还存不了视频：把 App 更新到 v0.20.27 或更新的版本，再导出一次。";
    // (an App whose bridge cannot save video says so at once, rather than
    // after a minute of turning the video into text)
    if (!myco?.features?.includes("download-media")) {
      setExporting({ stage: "unsaved", name, blob, silent, secs, drop, error: OLD_APP, retry: false });
      return;
    }
    try {
      const where = (await save(name, blob, "video/mp4")).path;
      drop();
      setExporting({ stage: "done", where, size: blob.size, silent, secs });
    } catch (e) {
      const err = e as { code?: string; message?: string };
      const old = err.code === "invalid_argument" || /no such call|documents and pictures/.test(err.message || "");
      setExporting({ stage: "unsaved", name, blob, silent, secs, drop, retry: !old,
        error: old ? OLD_APP : err.code === "too_large" ? "视频太大了（App 最多存 4 GB）：换低一点的分辨率或画质再导出。" : `没存上：${err.message || "不知道为什么"}` });
    }
  }
  async function doExport(opts: ExportOptions) {
    player.pause();
    cancelRef.current = false;
    keepOptions(opts);
    const at = performance.now();
    setExporting({ stage: "run", p: { phase: "audio", done: 0 }, at, opts });
    try {
      const { file, silent, drop } = await render(opts, (p) => setExporting((x) => (x && x.stage === "run" ? { ...x, p } : x)), () => cancelRef.current);
      const name = `${(title.get("") || "视频").replace(/[\\/:*?"<>|]+/g, " ").trim() || "视频"}.mp4`;
      await keepFile(name, file, silent, (performance.now() - at) / 1000, drop);
    } catch (e) {
      const msg = (e as Error).message;
      if (msg === "cancelled") setExporting(null);
      else setExporting({ stage: "error", error: msg || "导出失败" });
    }
  }

  const list = placed();
  const ratio = String(project.get("main")?.data.ratio || "source");
  const open = notes.rows.filter((n) => !n.data.done).length;
  const side = narrow ? tab : tab === "props" ? "media" : tab;
  const tabs: [Tab, string, typeof Film][] = narrow ? [...TABS, ["focus", "重点放大", ZoomIn], ["props", "属性", SlidersHorizontal]] : [...TABS, ["focus", "重点放大", ZoomIn]];

  return (
    <div className="app vid">
      <Bar kit={kit} title={title.get("")} placeholder={uiText(recording ? "未命名录屏" : "未命名视频")} onTitle={title.set}>
        {recording && <Recorder request={recordRequest} onRecorded={(f) => importFiles([f], clips.rows.length)} />}
        <select className="sel top-ratio" aria-label={uiText("画面比例")} value={ratio} onChange={(e) => setProject({ ratio: e.target.value })}>
          {RATIOS.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
        </select>
        <button type="button" className="btn" onClick={(e) => setAgentAt(rectOf(e.currentTarget))}><Bot className="ico" />{uiText("交给 agent")}{open ? <span className="badge">{open}</span> : null}</button>
        <button type="button" className="btn btn-primary" disabled={!list.length} onClick={() => setExporting({ stage: "setup" })}><Download className="ico" />{uiText("导出")}</button>
      </Bar>
      <div className="ed">
        <aside className="side">
          <nav className="tabs" role="tablist">
            {tabs.map(([id, name, Icon]) => (
              <button key={id} type="button" role="tab" aria-selected={side === id} className={`tab${side === id ? " is-on" : ""}`} onClick={() => setTab(id)}>
                <Icon className="ico" /><span>{uiText(name)}</span>{id === "notes" && open ? <i className="dot" /> : null}
              </button>
            ))}
          </nav>
          <div className="side-body">
            {side === "media" && <MediaPanel importing={importing} onImport={pickFiles} onRelink={onRelink} setSel={setSel} />}
            {side === "focus" && <FocusPanel setSel={setSel} />}
            {side === "text" && <TextPanel setSel={setSel} />}
            {side === "censor" && <CensorPanel setSel={setSel} />}
            {side === "notes" && <NotesPanel me={kit.me} focus={noteFocus} pageId={kit.page} title={title.get("")} />}
            {side === "props" && <Inspector sel={sel} setSel={setSel} />}
          </div>
        </aside>
        <main className="center">
          <Stage onRecord={recording ? () => setRecordRequest(n => n + 1) : undefined} sel={sel} setSel={setSel} onImport={pickFiles} />
        </main>
        {!narrow && <aside className="inspector"><Inspector sel={sel} setSel={setSel} /></aside>}
      </div>
      <Timeline sel={sel} setSel={setSel} pps={pps} setPps={setPps} onFiles={(f, i) => void importFiles(f, i)}
        onNoteClick={(id) => { setTab("notes"); setNoteFocus(id); }} />
      <input ref={fileInput} type="file" accept="video/*,.mp4,.mov,.m4v,.webm,.mkv" multiple hidden
        onChange={(e) => {
          const files = Array.from(e.target.files || []);
          e.target.value = "";
          if (relinkFor.current && files[0]) void relink(files[0], relinkFor.current);
          else if (files.length) void importFiles(files);
        }} />
      {agentAt && (
        <Popover at={agentAt} onClose={() => setAgentAt(null)} width={340} align="right">
          <div className="panel agent-pop">
            <b>{uiText("让 agent 一起剪")}</b>
            <span className="faint">{uiText("在「备注」里给时间点留话（「这段删掉」「给这个人脸打码」「加一句字幕」），再把这段话发给 agent。它按备注改剪辑方案，改动马上出现在这里。")}</span>
            <textarea className="inp" rows={5} readOnly value={agentText(kit.page, title.get(""))} onFocus={(e) => e.target.select()} />
            <div className="row">
              <button type="button" className="btn" onClick={() => { setTab("notes"); setAgentAt(null); }}><MessageSquareText className="ico" />{uiText("去写备注")}</button>
              <span className="top-gap" />
              <button type="button" className="btn btn-primary" onClick={async () => {
                try { await navigator.clipboard.writeText(agentText(kit.page, title.get(""))); kit.flash(uiText("已复制，发给任意一个 agent 就行")); setAgentAt(null); }
                catch { kit.flash(uiText("没复制上，请手动选中复制")); }
              }}>{uiText("复制")}</button>
            </div>
          </div>
        </Popover>
      )}
      {exporting && <ExportDialog state={exporting} onStart={doExport}
        onRetry={(x) => void keepFile(x.name, x.blob, x.silent, x.secs, x.drop)}
        onClose={() => {
          if (exporting.stage === "run") { cancelRef.current = true; return; }
          if (exporting.stage === "unsaved") exporting.drop();
          setExporting(null);
        }} />}
      <Toast kit={kit} />
    </div>
  );
}

type Exp = { stage: "setup" } | { stage: "run"; p: Progress; at: number; opts: ExportOptions }
  | { stage: "done"; where: string; size: number; silent: boolean; secs: number } | { stage: "error"; error: string }
  | Unsaved;
type Unsaved = { stage: "unsaved"; name: string; blob: Blob; silent: boolean; secs: number; error: string; drop: () => void; retry: boolean };

const SIZE_NAME: Record<number, string> = { 2160: "4K", 1440: "2K", 1080: "1080p", 720: "720p", 480: "480p" };
function savedOptions(): ExportOptions {
  let o: Partial<ExportOptions> = {};
  try { o = JSON.parse(localStorage.getItem("myco-video-export") || "{}"); } catch { /* (none kept) */ }
  const sizes = sizeChoices();
  const short = sizes.includes(Number(o.short)) ? Number(o.short) : sizes.find((s) => s <= 1080) || sizes[sizes.length - 1];
  return { short, fps: [24, 30, 60].includes(Number(o.fps)) ? Number(o.fps) : 30, quality: QUALITIES.some(([q]) => q === o.quality) ? o.quality! : "high" };
}
function keepOptions(o: ExportOptions) { try { localStorage.setItem("myco-video-export", JSON.stringify(o)); } catch { /* (a private window) */ } }
const mmss = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, "0")}`;

function ExportDialog({ state, onStart, onRetry, onClose }: { state: Exp; onStart: (o: ExportOptions) => void; onRetry: (x: Unsaved) => void; onClose: () => void }) {
  const [opts, setOpts] = useState<ExportOptions>(savedOptions);
  const [, tick] = useState(0);
  useEffect(() => { if (state.stage !== "run") return; const t = setInterval(() => tick((n) => n + 1), 1000); return () => clearInterval(t); }, [state.stage]);
  const T = total();
  const head = state.stage === "setup" ? "导出视频" : state.stage === "run" ? "正在导出" : state.stage === "done" ? "导出好了" : state.stage === "unsaved" ? "做好了，还没存上" : "没导出成功";
  let body: JSX.Element;
  if (state.stage === "setup") {
    const est = estimate(opts);
    body = (
      <>
        <div className="field">
          <div className="field-label">{uiText("分辨率")}</div>
          <div className="seg" role="radiogroup" aria-label={uiText("分辨率")} data-testid="export-res">
            {sizeChoices().map((s) => (
              <button key={s} type="button" role="radio" aria-checked={opts.short === s} className={opts.short === s ? "is-on" : ""} onClick={() => setOpts({ ...opts, short: s })}>
                {SIZE_NAME[s] || uiText("原始")}
              </button>
            ))}
          </div>
        </div>
        <div className="field">
          <div className="field-label">{uiText("帧率")}</div>
          <div className="seg" role="radiogroup" aria-label={uiText("帧率")}>
            {[24, 30, 60].map((f) => <button key={f} type="button" role="radio" aria-checked={opts.fps === f} className={opts.fps === f ? "is-on" : ""} onClick={() => setOpts({ ...opts, fps: f })}>{f} {" "}{uiText("帧")}</button>)}
          </div>
        </div>
        <div className="field">
          <div className="field-label">{uiText("画质")}</div>
          <div className="seg" role="radiogroup" aria-label={uiText("画质")}>
            {QUALITIES.map(([q, n]) => <button key={q} type="button" role="radio" aria-checked={opts.quality === q} className={opts.quality === q ? "is-on" : ""} onClick={() => setOpts({ ...opts, quality: q })}>{n}</button>)}
          </div>
        </div>
        <p className="export-sum"><b>{est.w}×{est.h}</b> · {opts.fps} {" "}{uiText("帧 ·")}{" "}{mmss(T)} {" "}{uiText("· 约")}{" "}{est.mb < 10 ? est.mb.toFixed(1) : Math.round(est.mb)} MB</p>
        <p className="faint">{uiText("分辨率和帧率越低导出越快；在这台电脑上完成，不上传。")}</p>
        {myco && !myco.features?.includes("download-media") && (
          <p className="modal-err">{uiText("这个版本的 myco App 还存不了视频：先把 App 更新到 v0.20.27 或更新的版本再导出。")}</p>
        )}
        <div className="modal-foot">
          <span />
          <span className="row">
            <button type="button" className="btn" onClick={onClose}>{uiText("取消")}</button>
            <button type="button" className="btn btn-primary" data-testid="export-start" onClick={() => onStart(opts)}><Download className="ico" />{uiText("开始导出")}</button>
          </span>
        </div>
      </>
    );
  } else if (state.stage === "run") {
    const p = state.p;
    const pct = p.phase === "audio" ? p.done * 5 : p.phase === "video" ? 5 + p.done * 93 : 98;
    const secs = (performance.now() - state.at) / 1000;
    const left = pct > 6 ? (secs / pct) * (100 - pct) : 0;
    const { w, h } = frameSize(placed(), state.opts.short);
    body = (
      <>
        <p className="faint">{w}×{h} · {state.opts.fps} {" "}{uiText("帧 ·")}{" "}{mmss(T)}。{p.phase === "audio" ? uiText("先合成声音…") : p.phase === "video" ? uiText("一帧一帧地画出来。") : uiText("收尾…")}</p>
        <div className="bar"><i style={{ width: `${pct}%` }} /></div>
        <div className="modal-foot">
          <span className="faint"><Loader2 className="ico spin" /> {Math.floor(pct)}{uiText("% · 已用")}{" "}{mmss(secs)}{left ? uiText(" · 还要约 {0}", mmss(left)) : ""}</span>
          <button type="button" className="btn" onClick={onClose}>{uiText("取消")}</button>
        </div>
      </>
    );
  } else if (state.stage === "done") {
    body = (
      <>
        <p>{uiText("已存到")}{" "}<b>{state.where}</b>（{(state.size / 1024 / 1024).toFixed(1)} {" "}{uiText("MB，用了")}{" "}{mmss(state.secs)}）</p>
        {state.silent && <p className="modal-err">{uiText("这个浏览器不能编码声音，导出的视频没有声音。换 Chrome 或 Edge 打开再导出一次。")}</p>}
        <div className="modal-foot"><span /><button type="button" className="btn btn-primary" onClick={onClose}>{uiText("好")}</button></div>
      </>
    );
  } else if (state.stage === "unsaved") {
    const x = state;
    const browser = () => {
      const a = document.createElement("a");
      a.href = URL.createObjectURL(x.blob);
      a.download = x.name; document.body.appendChild(a); a.click(); a.remove();
      setTimeout(() => URL.revokeObjectURL(a.href), 120000);
    };
    body = (
      <>
        <p>{uiText("视频做好了（")}{(x.blob.size / 1024 / 1024).toFixed(1)} {" "}{uiText("MB，用了")}{" "}{mmss(x.secs)}{uiText("），但没存到「下载」：")}</p>
        <p className="modal-err">{x.error}</p>
        {x.retry && <p className="faint">{uiText("关掉这个框之前它一直在，不用重新导出。")}</p>}
        <div className="modal-foot">
          <button type="button" className="btn" onClick={browser}>{uiText("用浏览器下载")}</button>
          <span className="row">
            <button type="button" className="btn" onClick={onClose}>{uiText("不要了")}</button>
            {x.retry && <button type="button" className="btn btn-primary" onClick={() => onRetry(x)}>{uiText("再存一次")}</button>}
          </span>
        </div>
      </>
    );
  } else {
    body = (
      <>
        <p className="modal-err">{state.error}</p>
        <div className="modal-foot"><span /><button type="button" className="btn btn-primary" onClick={onClose}>{uiText("好")}</button></div>
      </>
    );
  }
  return (
    <div className="modal-back" role="dialog" aria-modal="true" aria-label={uiText("导出视频")}>
      <div className="modal">
        <div className="modal-head">
          <b>{head}</b>
          {state.stage !== "run" && <button type="button" className="icon-btn" aria-label={uiText("关闭")} onClick={onClose}><X className="ico" /></button>}
        </div>
        {body}
      </div>
    </div>
  );
}

export function mountVideo(recording = false) { mount(kit, () => <VideoApp recording={recording} />, async () => {
  await restore(media.rows.map((r) => String(r.data.key || "")));
}); }
