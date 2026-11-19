// 视频剪辑: a video editor people and agents use together. The videos stay
// on the computer that opened them; the edit (clips with their trims and
// speeds, text, blur and mosaic regions, notes) is the page's records, so
// an agent edits it with the CLI and the page follows at once. The mp4 is
// made here in the browser.
import "../_lib/sheet.css";
import "./video.css";
import { useCallback, useEffect, useRef, useState } from "react";
import { Bot, Download, Droplets, Film, Loader2, MessageSquareText, SlidersHorizontal, Type, X } from "lucide-react";
import { Bar, Toast, mount, titleSaver, uid, useKit } from "../_lib/appkit";
import { save } from "../_lib/myco";
import { Popover, rectOf } from "../_lib/ui";
import { clips, fmt, frameSize, kit, media, notes, overlays, placed, project, total, RATIOS, clamp } from "./model";
import { addClip, change, patch, redo, removeClip, removeOverlay, setProject, split, undo, duplicateClip } from "./edit";
import { keep, keyOf, onLocal, probe, restore } from "./media";
import { player } from "./player";
import { Timeline, type Sel } from "./timeline";
import { Stage } from "./stage";
import { CensorPanel, Inspector, MediaPanel, NotesPanel, TextPanel, agentText, type Importing } from "./panels";
import { render, type Progress } from "./export";

const title = titleSaver(kit, project);

type Tab = "media" | "text" | "censor" | "notes" | "props";
const TABS: [Tab, string, typeof Film][] = [["media", "素材", Film], ["text", "文字", Type], ["censor", "打码", Droplets], ["notes", "备注", MessageSquareText]];

function useNarrow() {
  const q = window.matchMedia?.("(max-width: 980px)");
  const [n, setN] = useState(!!q?.matches);
  useEffect(() => { if (!q) return; const f = () => setN(q.matches); q.addEventListener("change", f); return () => q.removeEventListener("change", f); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return n;
}

function App() {
  useKit(kit);
  const [, setLocalVer] = useState(0);
  useEffect(() => onLocal(() => setLocalVer((n) => n + 1)), []);
  const narrow = useNarrow();
  const [tab, setTab] = useState<Tab>("media");
  const [sel, setSelState] = useState<Sel>(null);
  const [pps, setPps] = useState(40);
  const [importing, setImporting] = useState<Importing[]>([]);
  const [noteFocus, setNoteFocus] = useState("");
  const [exporting, setExporting] = useState<null | { p: Progress; error?: string; done?: string; size?: number; silent?: boolean }>(null);
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
    let at = index;
    for (const f of files) {
      const tmpKey = uid("i");
      setImporting((l) => [...l, { name: f.name, key: tmpKey }]);
      try {
        const key = await keep(f);
        let rec = media.rows.find((r) => r.data.key === key);
        if (!rec) {
          const p = await probe(f);
          if (!p.duration) throw new Error(`「${f.name}」读不出来：换成 mp4（H.264）试试`);
          const id = uid("m");
          change([[media, id, { name: f.name, size: f.size, duration: Math.round(p.duration * 1000) / 1000, width: p.width, height: p.height, audio: p.audio, key, poster: p.poster.length < 40000 ? p.poster : "" }]]);
          rec = media.get(id)!;
        }
        if (at !== undefined || !clips.rows.length) {
          const id = addClip(rec.id, at ?? clips.rows.length);
          if (at !== undefined) at++;
          setSel({ kind: "clip", id }, false);
        }
      } catch (e) {
        kit.flash((e as Error).message || `「${f.name}」没导入成功`);
      } finally {
        setImporting((l) => l.filter((x) => x.key !== tmpKey));
      }
    }
  }
  async function relink(file: File, mediaId: string) {
    const m = media.get(mediaId);
    if (!m) return;
    const key = await keyOf(file);
    if (key !== m.data.key) {
      if (!window.confirm(`选的文件和原来的「${m.data.name}」不一样（大小或内容不同）。还是用它代替吗？大家看到的都会换成它。`)) return;
      const p = await probe(file);
      await keep(file);
      patch(media, mediaId, { key, name: file.name, size: file.size, duration: p.duration || m.data.duration, width: p.width, height: p.height, audio: p.audio });
    } else {
      await keep(file);
      kit.flash("找到了，接着剪吧");
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
  async function doExport() {
    player.pause();
    cancelRef.current = false;
    setExporting({ p: { phase: "audio", done: 0 } });
    try {
      const { bytes, silent } = await render((p) => setExporting((x) => (x ? { ...x, p } : x)), () => cancelRef.current);
      const name = `${(title.get("") || "视频").replace(/[\\/:*?"<>|]+/g, " ").trim() || "视频"}.mp4`;
      let where = "";
      try { where = (await save(name, bytes, "video/mp4")).path; }
      catch {
        // (an older App: the browser's own download)
        const a = document.createElement("a");
        a.href = URL.createObjectURL(new Blob([bytes as BlobPart], { type: "video/mp4" }));
        a.download = name; a.click();
        setTimeout(() => URL.revokeObjectURL(a.href), 60000);
        where = name;
      }
      setExporting({ p: { phase: "finish", done: 1 }, done: where, size: bytes.length, silent });
    } catch (e) {
      const msg = (e as Error).message;
      if (msg === "cancelled") setExporting(null);
      else setExporting({ p: { phase: "finish", done: 0 }, error: msg || "导出失败" });
    }
  }

  const list = placed();
  const ratio = String(project.get("main")?.data.ratio || "source");
  const open = notes.rows.filter((n) => !n.data.done).length;
  const side = narrow ? tab : tab === "props" ? "media" : tab;
  const tabs: [Tab, string, typeof Film][] = narrow ? [...TABS, ["props", "属性", SlidersHorizontal]] : TABS;

  return (
    <div className="app vid">
      <Bar kit={kit} title={title.get("")} placeholder="未命名视频" onTitle={title.set}>
        <select className="sel top-ratio" aria-label="画面比例" value={ratio} onChange={(e) => setProject({ ratio: e.target.value })}>
          {RATIOS.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
        </select>
        <button type="button" className="btn" onClick={(e) => setAgentAt(rectOf(e.currentTarget))}><Bot className="ico" />交给 agent{open ? <span className="badge">{open}</span> : null}</button>
        <button type="button" className="btn btn-primary" disabled={!list.length} onClick={doExport}><Download className="ico" />导出</button>
      </Bar>
      <div className="ed">
        <aside className="side">
          <nav className="tabs" role="tablist">
            {tabs.map(([id, name, Icon]) => (
              <button key={id} type="button" role="tab" aria-selected={side === id} className={`tab${side === id ? " is-on" : ""}`} onClick={() => setTab(id)}>
                <Icon className="ico" /><span>{name}</span>{id === "notes" && open ? <i className="dot" /> : null}
              </button>
            ))}
          </nav>
          <div className="side-body">
            {side === "media" && <MediaPanel importing={importing} onImport={pickFiles} onRelink={onRelink} setSel={setSel} />}
            {side === "text" && <TextPanel setSel={setSel} />}
            {side === "censor" && <CensorPanel setSel={setSel} />}
            {side === "notes" && <NotesPanel me={kit.me} focus={noteFocus} pageId={kit.page} title={title.get("")} />}
            {side === "props" && <Inspector sel={sel} setSel={setSel} />}
          </div>
        </aside>
        <main className="center">
          <Stage sel={sel} setSel={setSel} onImport={pickFiles} />
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
            <b>让 agent 一起剪</b>
            <span className="faint">在「备注」里给时间点留话（「这段删掉」「给这个人脸打码」「加一句字幕」），再把这段话发给 agent。它按备注改剪辑方案，改动马上出现在这里。</span>
            <textarea className="inp" rows={5} readOnly value={agentText(kit.page, title.get(""))} onFocus={(e) => e.target.select()} />
            <div className="row">
              <button type="button" className="btn" onClick={() => { setTab("notes"); setAgentAt(null); }}><MessageSquareText className="ico" />去写备注</button>
              <span className="top-gap" />
              <button type="button" className="btn btn-primary" onClick={async () => {
                try { await navigator.clipboard.writeText(agentText(kit.page, title.get(""))); kit.flash("已复制，发给任意一个 agent 就行"); setAgentAt(null); }
                catch { kit.flash("没复制上，请手动选中复制"); }
              }}>复制</button>
            </div>
          </div>
        </Popover>
      )}
      {exporting && <ExportDialog state={exporting} onCancel={() => { cancelRef.current = true; if (exporting.done || exporting.error) setExporting(null); }} />}
      <Toast kit={kit} />
    </div>
  );
}

function ExportDialog({ state, onCancel }: { state: { p: Progress; error?: string; done?: string; size?: number; silent?: boolean }; onCancel: () => void }) {
  const { w, h } = frameSize();
  const pct = state.p.phase === "audio" ? state.p.done * 8 : state.p.phase === "video" ? 8 + state.p.done * 90 : state.done ? 100 : 98;
  const over = !!(state.done || state.error);
  return (
    <div className="modal-back" role="dialog" aria-modal="true" aria-label="导出视频">
      <div className="modal">
        <div className="modal-head">
          <b>{state.error ? "没导出成功" : state.done ? "导出好了" : "正在导出"}</b>
          {over && <button type="button" className="icon-btn" aria-label="关闭" onClick={onCancel}><X className="ico" /></button>}
        </div>
        {state.error ? <p className="modal-err">{state.error}</p> : state.done ? (
          <>
            <p>已存到 <b>{state.done}</b>{state.size ? `（${(state.size / 1024 / 1024).toFixed(1)} MB）` : ""}</p>
            {state.silent && <p className="modal-err">这个浏览器不能编码声音，导出的视频没有声音。换 Chrome 或 Edge 打开再导出一次。</p>}
          </>
        ) : (
          <>
            <p className="faint">{w}×{h} · {fmt(total(), true)} · mp4。{state.p.phase === "audio" ? "先合成声音…" : state.p.phase === "video" ? "一帧一帧地画出来，在这台电脑上完成，不上传。" : "收尾…"}</p>
            <div className="bar"><i style={{ width: `${pct}%` }} /></div>
            <div className="modal-foot">
              <span className="faint"><Loader2 className="ico spin" /> {Math.floor(pct)}%</span>
              <button type="button" className="btn" onClick={onCancel}>取消</button>
            </div>
          </>
        )}
        {over && <div className="modal-foot"><span /><button type="button" className="btn btn-primary" onClick={onCancel}>好</button></div>}
      </div>
    </div>
  );
}

mount(kit, App, async () => {
  await restore(media.rows.map((r) => String(r.data.key || "")));
});
