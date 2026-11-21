import { t as uiText } from "../_lib/i18n";
// 动效视频: motion videos made of scenes, each an HTML + CSS + animation
// script (written by an agent, or started from a template) with texts and
// pictures people change in a form. The preview and the mp4 come from a
// sandboxed frame (frame.js) that moves every scene to any time; the mp4
// is made here in the page.
import "../_lib/sheet.css";
import "./motion.css";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle, ArrowDown, ArrowUp, Bot, Code2, Copy, Download, Film, ImagePlus, Loader2, MessageSquarePlus, Pause, Play, Plus, Save, SkipBack, Trash2, X,
} from "lucide-react";
import { Bar, Toast, mount, titleSaver, useKit } from "../_lib/appkit";
import { myco, save } from "../_lib/myco";
import { MenuItem, Popover, rectOf } from "../_lib/ui";
import {
  FONTS, PALETTES, RATIOS, TRANSITIONS, MIN_SCENE, assets, captionList, clamp, draft, fieldsOf, fmt, fpsOf, frameDoc, kit, notes, num, placed, project,
  sceneAt, scenes, secs, sizeOf, total, tracks, type Field, type Placed,
} from "./model";
import { addAsset, addScene, change, duplicateScene, moveScene, patch, redo, removeScene, sceneData, setField, setProject, undo } from "./edit";
import { STARTERS, TEMPLATES } from "./templates";
import { SceneFrame } from "./stage";
import { QUALITIES, defaultOptions, estimate, outSize, render, type ExportOptions } from "./export";
import { uid } from "../_lib/appkit";
import { player, usePlayer } from "./player";
import { PEAKS_PER_SEC, beatTimes, localFor, restore, trackList, type PlacedTrack } from "./audio";
import { LibrarySection, NotesPanel, PlanPanel, SoundPanel, canSend, useLocal } from "./panels";
import { addSavedScene, items, saveTemplate, useTemplate } from "./library";

const title = titleSaver(kit, project);
type Rect = ReturnType<typeof rectOf>;

// ---------------------------------------------------------------- thumbnails
// drawn by a hidden frame, once for each look of a scene
const thumbs = new Map<string, { key: string; url: string }>();
let thumbFrame: SceneFrame | null = null;
let thumbBusy = false, thumbAgain = false;
const thumbSubs = new Set<() => void>();
async function drawThumbs() {
  if (!thumbFrame) return;
  if (thumbBusy) { thumbAgain = true; return; }
  thumbBusy = true;
  try {
    const doc = frameDoc("capture");
    const { w, h } = sizeOf();
    const k = 200 / Math.max(w, h);
    const list = placed();
    const look = JSON.stringify([doc.theme, doc.w, doc.h]);
    const todo = list.filter((p) => {
      const s = doc.scenes.find((x) => x.id === p.id);
      return thumbs.get(p.id)?.key !== look + JSON.stringify(s);
    });
    if (!todo.length) return;
    await thumbFrame.load(doc);
    for (const p of todo) {
      const s = doc.scenes.find((x) => x.id === p.id);
      try {
        const bmp = await thumbFrame.capture(p.start + Math.min(p.dur * 0.7, Math.max(0, p.dur - 0.2)), Math.round(w * k), Math.round(h * k));
        const c = document.createElement("canvas");
        c.width = bmp.width; c.height = bmp.height;
        c.getContext("2d")!.drawImage(bmp, 0, 0);
        bmp.close();
        thumbs.set(p.id, { key: look + JSON.stringify(s), url: c.toDataURL("image/jpeg", 0.8) });
        for (const f of thumbSubs) f();
      } catch { /* (no picture for it: the number shows) */ }
    }
  } finally {
    thumbBusy = false;
    if (thumbAgain) { thumbAgain = false; void drawThumbs(); }
  }
}

// ---------------------------------------------------------------- app
type Side = "scene" | "sound" | "notes" | "plan" | "video";
const SIDES: [Side, string][] = [["scene", uiText("场景")], ["sound", uiText("声音")], ["notes", uiText("批注")], ["plan", uiText("分镜")], ["video", uiText("视频")]];

function App() {
  useKit(kit);
  usePlayer();
  const [, setThumbVer] = useState(0);
  useEffect(() => { const f = () => setThumbVer((n) => n + 1); thumbSubs.add(f); return () => { thumbSubs.delete(f); }; }, []);
  const [sel, setSelState] = useState("");
  const [side, setSide] = useState<Side>("scene");
  const [focusField, setFocusField] = useState("");
  const [sideFocus, setSideFocus] = useState("");
  useLocal();
  const openSide = (s: Side, focus = "") => { setSide(s); setSideFocus(focus ? `${focus}:${Date.now()}` : ""); };
  // the music and voice files this computer has
  const trackKeys = tracks.rows.map((r) => String(r.data.key || "")).join(",");
  useEffect(() => { void restore(trackKeys.split(",")); }, [trackKeys]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [agentAt, setAgentAt] = useState<Rect | null>(null);
  const [exporting, setExporting] = useState<Exp | null>(null);
  const [menu, setMenu] = useState<{ at: Rect; id: string } | null>(null);
  const [addAt, setAddAt] = useState<{ at: Rect; index: number } | null>(null);
  const stageRef = useRef<HTMLDivElement>(null);
  const hiddenRef = useRef<HTMLDivElement>(null);
  const cancelRef = useRef(false);

  const list = placed();
  const T = total(list);
  const selected = sel && scenes.get(sel) ? sel : "";
  const setSel = useCallback((id: string, seek = true, reveal = true) => {
    setSelState(id);
    if (id && reveal) setSide("scene");
    const p = placed().find((x) => x.id === id);
    // (a moment when the scene is all there, not its empty first frame)
    if (p && seek && !player.playing) player.seek(p.start + Math.min(p.dur * 0.7, p.dur - 0.05));
  }, []);

  // the preview frame and the hidden one for thumbnails
  useLayoutEffect(() => {
    if (!stageRef.current || player.frame) return;
    const f = new SceneFrame(stageRef.current);
    f.onPick = (scene, field) => {
      if (!scene) return;
      player.pause();
      setSel(scene, false);
      if (field) setFocusField(`${scene}:${field}:${Date.now()}`);
    };
    f.onError = (scene, message) => setErrors((e) => ({ ...e, [scene]: message }));
    player.frame = f;
    thumbFrame = new SceneFrame(hiddenRef.current!, true);
  }, [setSel]);

  // the preview follows the data (and drags), a moment after typing stops
  const docKey = JSON.stringify(frameDoc("preview"));
  useEffect(() => {
    const t = window.setTimeout(async () => {
      if (!player.frame) return;
      const r = await player.frame.load(frameDoc("preview"));
      setErrors(r.errors);
      player.seek(player.time);
    }, 80);
    const t2 = window.setTimeout(() => void drawThumbs(), 700);
    return () => { window.clearTimeout(t); window.clearTimeout(t2); };
  }, [docKey]);

  // the first scene is selected when there is none
  useEffect(() => { if (!selected && list.length) setSel(list[0].id, player.time === 0); }, [selected, list.length]); // eslint-disable-line react-hooks/exhaustive-deps

  // ---- keys ----
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable)) return;
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key.toLowerCase() === "z") { e.preventDefault(); if (e.shiftKey) redo(); else undo(); return; }
      if (mod && e.key.toLowerCase() === "y") { e.preventDefault(); redo(); return; }
      if (mod && e.key.toLowerCase() === "d") { e.preventDefault(); if (selected) { const n = duplicateScene(selected); if (n) setSel(n); } return; }
      if (mod) return;
      const fps = fpsOf();
      if (e.key === " ") { e.preventDefault(); player.toggle(); }
      else if (e.key === "ArrowLeft") { e.preventDefault(); player.pause(); player.seek(player.time - (e.shiftKey ? 1 : 1 / fps)); }
      else if (e.key === "ArrowRight") { e.preventDefault(); player.pause(); player.seek(player.time + (e.shiftKey ? 1 : 1 / fps)); }
      else if (e.key === "ArrowUp" || e.key === "ArrowDown") {
        e.preventDefault();
        const i = list.findIndex((p) => p.id === selected);
        const n = list[clamp(i + (e.key === "ArrowUp" ? -1 : 1), 0, list.length - 1)];
        if (n) { player.pause(); player.seek(n.start); setSel(n.id, false); }
      }
      else if (e.key === "Home") player.seek(0);
      else if (e.key === "n" || e.key === "N") { e.preventDefault(); player.pause(); openSide("notes", "new"); }
      else if (e.key === "End") player.seek(T);
      else if ((e.key === "Delete" || e.key === "Backspace") && selected) { e.preventDefault(); deleteScene(selected); }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  });

  function deleteScene(id: string) {
    const i = list.findIndex((p) => p.id === id);
    removeScene(id);
    const next = list[i + 1] || list[i - 1];
    setSel(next ? next.id : "", false);
    kit.flash(uiText("已删除场景"), undo);
  }

  function startWith(starterId: string) {
    const s = STARTERS.find((x) => x.id === starterId);
    if (!s) return;
    const ops: Parameters<typeof change>[0] = [];
    const cur = project.get("main")?.data || {};
    ops.push([project, "main", { ...cur, ratio: s.ratio }]);
    let first = "";
    s.scenes.forEach((tid, i) => {
      const t = TEMPLATES.find((x) => x.id === tid);
      if (!t) return;
      const id = uid("s");
      if (!first) first = id;
      ops.push([scenes, id, { ...sceneData(t, i + 1), transition: i === 0 ? "cut" : t.transition || "fade" }]);
    });
    change(ops);
    setSel(first, false);
    player.seek(0);
  }

  // ---- export ----
  async function doExport(opts: ExportOptions) {
    player.pause();
    cancelRef.current = false;
    const at = performance.now();
    setExporting({ stage: "run", done: 0, at, opts });
    const host = document.createElement("div");
    host.className = "mo-hidden";
    document.body.appendChild(host);
    const f = new SceneFrame(host, true);
    try {
      const out = await render(f, opts, (done) => setExporting((x) => (x && x.stage === "run" ? { ...x, done } : x)), () => cancelRef.current);
      const blob = out.file;
      const warn = out.silent ? uiText("这个浏览器不能编码声音，视频没有声音。") : out.missing.length ? uiText("这台电脑上没有「{0}」，它没混进去。", out.missing.join("、")) : "";
      const name = `${(title.get("") || uiText("动效视频")).replace(/[\\/:*?"<>|]+/g, " ").trim() || "motion"}.mp4`;
      const took = (performance.now() - at) / 1000;
      try {
        const where = (await save(name, blob, "video/mp4")).path;
        setExporting({ stage: "done", where, size: blob.size, secs: took, warn });
      } catch (e) {
        setExporting({ stage: "unsaved", name, blob, secs: took, error: (e as Error).message || uiText("没存上") });
      }
    } catch (e) {
      const msg = (e as Error).message;
      if (msg === "cancelled") setExporting(null);
      else setExporting({ stage: "error", error: msg || uiText("导出失败") });
    } finally {
      f.remove();
      host.remove();
    }
  }

  const ratio = sizeOf().ratio;
  const cur = list.find((p) => p.id === selected) || null;
  const empty = list.length === 0;
  const openNotes = notes.rows.filter((n) => !n.data.done).length;

  function saveAs(name: string, kind: "video" | "scene", sceneId?: string) {
    const first = kind === "scene" ? sceneId! : list[0]?.id;
    const id = saveTemplate(name || (kind === "scene" ? scenes.get(sceneId!)?.data.name || "" : title.get("")), kind, (first && thumbs.get(first)?.url) || "", kit.me, sceneId);
    if (id) kit.flash(kind === "scene" ? uiText("已存成场景模板，「添加场景」里能找到") : uiText("已存成模板"), undo);
  }

  return (
    <div className="app mo">
      <Bar kit={kit} title={title.get("")} placeholder={uiText("未命名视频")} onTitle={title.set}>
        <div className="seg mo-ratio" role="radiogroup" aria-label={uiText("画面比例")}>
          {RATIOS.slice(0, 3).map((r) => (
            <button key={r.id} type="button" role="radio" aria-checked={ratio === r.id} className={ratio === r.id ? "is-on" : ""} title={r.name}
              onClick={() => setProject({ ratio: r.id })}>{r.id}</button>
          ))}
        </div>
        <button type="button" className="btn" onClick={(e) => setAgentAt(rectOf(e.currentTarget))}><Bot className="ico" />{uiText("让 agent 做")}</button>
        <button type="button" className="btn btn-primary" disabled={empty} onClick={() => setExporting({ stage: "setup" })}><Download className="ico" />{uiText("导出")}</button>
      </Bar>
      <div className="mo-body">
        <aside className="mo-scenes" aria-label={uiText("场景")}>
          <div className="mo-scenes-head"><span>{uiText("场景")}</span><span className="faint">{list.length ? `${list.length} · ${fmt(T)}` : ""}</span></div>
          <div className="mo-scene-list" role="listbox" aria-label={uiText("场景")}>
            {list.map((p) => (
              <SceneCard key={p.id} p={p} on={p.id === selected} playing={player.time >= p.start && player.time < p.end} error={errors[p.id]}
                onClick={() => { player.pause(); setSel(p.id); }}
                onMenu={(at) => setMenu({ at, id: p.id })} />
            ))}
            <button type="button" className="mo-add" onClick={(e) => setAddAt({ at: rectOf(e.currentTarget), index: list.length })}><Plus className="ico" />{uiText("添加场景")}</button>
          </div>
        </aside>
        <main className="mo-center">
          <div className="mo-stage-wrap">
            <div className={`mo-stage${empty ? " is-empty" : ""}`} ref={stageRef} />
            {empty && <Starter onStart={startWith} onBlank={() => { const id = addScene("title", 0); setSel(id); }} onAgent={(r) => setAgentAt(r)}
              onTemplate={(id) => { const first = useTemplate(id); if (first) setSel(first, false); player.seek(0); }} onReference={() => openSide("plan")} />}
          </div>
          {!empty && <Transport list={list} total={T} current={cur} onNote={() => { player.pause(); openSide("notes", "new"); }} />}
          {!empty && <Timeline list={list} total={T} selected={selected} errors={errors} onSelect={(id) => setSel(id, false, false)} onMenu={(at, id) => setMenu({ at, id })}
            onNote={(id) => openSide("notes", id)} onTrack={(id) => openSide("sound", id)} onCaption={() => openSide("sound", "captions")} />}
        </main>
        <aside className="mo-side">
          <nav className="mo-tabs" role="tablist">
            {SIDES.map(([id, name]) => (
              <button key={id} type="button" role="tab" aria-selected={side === id} className={side === id ? "is-on" : ""} onClick={() => openSide(id)}>
                {name}{id === "notes" && openNotes ? <i className="mo-dot" aria-label={uiText("{0} 条没处理", openNotes)} /> : null}
              </button>
            ))}
          </nav>
          <div className="mo-side-body">
            {side === "scene" && (cur ? <SceneProps key={cur.id} p={cur} error={errors[cur.id]} focus={focusField} onDelete={() => deleteScene(cur.id)} onDuplicate={() => { const n = duplicateScene(cur.id); if (n) setSel(n); }} />
              : <div className="mo-none">{uiText("还没有场景。从左边「添加场景」或中间选一个开头。")}</div>)}
            {side === "sound" && <SoundPanel focus={sideFocus} />}
            {side === "notes" && <NotesPanel focus={sideFocus} title={title.get("")} />}
            {side === "plan" && <PlanPanel title={title.get("")} />}
            {side === "video" && <><VideoProps /><LibrarySection onSave={(n) => saveAs(n, "video")} onUsed={(first) => { if (first) setSel(first, false); player.seek(0); }} /></>}
          </div>
        </aside>
      </div>
      <div ref={hiddenRef} className="mo-hidden" />
      {menu && (
        <Popover at={menu.at} onClose={() => setMenu(null)} width={180}>
          <div className="menu">
            <MenuItem icon={<Copy className="ico" />} onClick={() => { const n = duplicateScene(menu.id); if (n) setSel(n); setMenu(null); }}>{uiText("复制")}</MenuItem>
            <MenuItem icon={<ArrowUp className="ico" />} onClick={() => { const i = list.findIndex((p) => p.id === menu.id); moveScene(menu.id, Math.max(0, i - 1)); setMenu(null); }}>{uiText("上移")}</MenuItem>
            <MenuItem icon={<ArrowDown className="ico" />} onClick={() => { const i = list.findIndex((p) => p.id === menu.id); moveScene(menu.id, Math.min(list.length - 1, i + 1)); setMenu(null); }}>{uiText("下移")}</MenuItem>
            <MenuItem icon={<Plus className="ico" />} onClick={() => { const i = list.findIndex((p) => p.id === menu.id); setAddAt({ at: menu.at, index: i + 1 }); setMenu(null); }}>{uiText("在后面添加场景")}</MenuItem>
            <MenuItem icon={<Save className="ico" />} onClick={() => { saveAs("", "scene", menu.id); setMenu(null); }}>{uiText("存成场景模板")}</MenuItem>
            <div className="menu-sep" />
            <MenuItem icon={<Trash2 className="ico" />} onClick={() => { deleteScene(menu.id); setMenu(null); }}>{uiText("删除")}</MenuItem>
          </div>
        </Popover>
      )}
      {addAt && (
        <Popover at={addAt.at} onClose={() => setAddAt(null)} width={360}>
          <TemplatePicker onPick={(tid) => { const id = addScene(tid, addAt.index); setAddAt(null); setSel(id); }}
            onSaved={(item) => { const id = addSavedScene(item, addAt.index); setAddAt(null); if (id) setSel(id); }} />
        </Popover>
      )}
      {agentAt && <AgentPop at={agentAt} selected={cur} onClose={() => setAgentAt(null)} onReference={() => { setAgentAt(null); openSide("plan"); }} />}
      {exporting && <ExportDialog state={exporting} onStart={doExport}
        onClose={() => { if (exporting.stage === "run") { cancelRef.current = true; return; } setExporting(null); }} />}
      <Toast kit={kit} />
    </div>
  );
}

// ---------------------------------------------------------------- scene list
function SceneCard({ p, on, playing, error, onClick, onMenu }: { p: Placed; on: boolean; playing: boolean; error?: string; onClick: () => void; onMenu: (at: Rect) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { if (on) ref.current?.scrollIntoView({ block: "nearest" }); }, [on]);
  const th = thumbs.get(p.id);
  const { w, h } = sizeOf();
  return (
    <div ref={ref} role="option" aria-selected={on} tabIndex={0} className={`mo-card${on ? " is-on" : ""}${playing ? " is-playing" : ""}`}
      onClick={onClick} onKeyDown={(e) => { if (e.key === "Enter") onClick(); }}
      onContextMenu={(e) => { e.preventDefault(); onMenu({ x: e.clientX, y: e.clientY, w: 0, h: 0 }); }}
      draggable onDragStart={(e) => { e.dataTransfer.setData("text/x-scene", p.id); e.dataTransfer.effectAllowed = "move"; }}
      onDragOver={(e) => { if (e.dataTransfer.types.includes("text/x-scene")) { e.preventDefault(); e.currentTarget.classList.add("is-drop"); } }}
      onDragLeave={(e) => e.currentTarget.classList.remove("is-drop")}
      onDrop={(e) => {
        e.currentTarget.classList.remove("is-drop");
        const id = e.dataTransfer.getData("text/x-scene");
        if (id && id !== p.id) { e.preventDefault(); moveScene(id, placed().filter((x) => x.id !== id).findIndex((x) => x.id === p.id)); }
      }}>
      <div className="mo-thumb" style={{ aspectRatio: `${w} / ${h}` }}>
        {th ? <img src={th.url} alt="" draggable={false} /> : null}
        <span className="mo-num">{p.index + 1}</span>
        {error && <span className="mo-err-dot" title={error}><AlertTriangle className="ico" /></span>}
      </div>
      <div className="mo-card-meta"><b>{p.name || uiText("场景 {0}", p.index + 1)}</b><span>{secs(p.dur)}</span></div>
    </div>
  );
}

function TemplatePicker({ onPick, onSaved }: { onPick: (id: string) => void; onSaved: (item: string) => void }) {
  const saved = items("scene");
  return (
    <div className="mo-picker">
      <div className="mo-picker-head">{uiText("选一个场景")}</div>
      <div className="mo-picker-grid">
        {TEMPLATES.map((t) => (
          <button key={t.id} type="button" className="mo-tpl" onClick={() => onPick(t.id)}>
            <b>{t.name}</b><span>{t.hint}</span>
          </button>
        ))}
      </div>
      {saved.length > 0 && (
        <>
          <div className="mo-picker-head is-sub">{uiText("存下的场景")}</div>
          <div className="mo-picker-grid">
            {saved.map((it) => (
              <button key={it.id} type="button" className="mo-tpl is-saved" onClick={() => onSaved(it.id)}>
                {it.data.thumb ? <img src={String(it.data.thumb)} alt="" /> : null}
                <b>{String(it.data.name || "")}</b><span>{secs(num(it.data.duration, 0))}{it.data.by ? ` · ${it.data.by}` : ""}</span>
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function Starter({ onStart, onBlank, onAgent, onTemplate, onReference }: {
  onStart: (id: string) => void; onBlank: () => void; onAgent: (r: Rect) => void; onTemplate: (id: string) => void; onReference: () => void;
}) {
  const saved = items("video");
  return (
    <div className="mo-start">
      <h2>{uiText("做一个动效视频")}</h2>
      <p>{uiText("从一个现成的开头改文字和图片，或者说清楚要什么，让 agent 来做。")}</p>
      {saved.length > 0 && (
        <>
          <div className="mo-start-label">{uiText("存下的模板")}</div>
          <div className="mo-start-grid">
            {saved.slice(0, 6).map((it) => (
              <button key={it.id} type="button" className="mo-start-card is-saved" onClick={() => onTemplate(it.id)}>
                {it.data.thumb ? <img src={String(it.data.thumb)} alt="" /> : null}
                <span className="mo-start-ratio">{String(it.data.ratio || "")}</span><b>{String(it.data.name || "")}</b>
                <span>{uiText("{0} 个场景", num(it.data.scenes, 0))} · {secs(num(it.data.duration, 0))}</span>
              </button>
            ))}
          </div>
          <div className="mo-start-label">{uiText("现成的开头")}</div>
        </>
      )}
      <div className="mo-start-grid">
        {STARTERS.map((s) => (
          <button key={s.id} type="button" className="mo-start-card" onClick={() => onStart(s.id)}>
            <span className="mo-start-ratio">{s.ratio}</span><b>{s.name}</b><span>{s.hint}</span>
          </button>
        ))}
      </div>
      <div className="mo-start-row">
        <button type="button" className="btn" onClick={onBlank}><Plus className="ico" />{uiText("从一个场景开始")}</button>
        <button type="button" className="btn" onClick={onReference}><Film className="ico" />{uiText("照一段视频做")}</button>
        <button type="button" className="btn" onClick={(e) => onAgent(rectOf(e.currentTarget))}><Bot className="ico" />{uiText("让 agent 做")}</button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------- transport + timeline
function Transport({ list, total: T, onNote }: { list: Placed[]; total: number; current: Placed | null; onNote: () => void }) {
  const p = usePlayer();
  const at = sceneAt(p.time, list);
  return (
    <div className="mo-transport">
      <button type="button" className="icon-btn" title={uiText("回到开头")} aria-label={uiText("回到开头")} onClick={() => p.seek(0)}><SkipBack className="ico" /></button>
      <button type="button" className="mo-play" title={p.playing ? uiText("暂停（空格）") : uiText("播放（空格）")} aria-label={p.playing ? uiText("暂停") : uiText("播放")} onClick={() => p.toggle()}>
        {p.playing ? <Pause className="ico" /> : <Play className="ico" />}
      </button>
      <span className="mo-time"><b>{fmt(p.time, true)}</b> / {fmt(T, true)}</span>
      <span className="top-gap" />
      <span className="mo-now">{at ? `${at.index + 1} · ${at.name || uiText("场景 {0}", at.index + 1)}` : ""}</span>
      <button type="button" className="btn mo-note-btn" title={uiText("在这里留批注（N）")} onClick={onNote}><MessageSquarePlus className="ico" />{uiText("批注")}</button>
    </div>
  );
}

/** A track's sound as bars, for the part of the file that plays. */
function Wave({ t, width }: { t: PlacedTrack; width: number }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const l = localFor(t.key);
  useLayoutEffect(() => {
    const c = ref.current;
    if (!c) return;
    const dpr = window.devicePixelRatio || 1, h = 22;
    const w = Math.max(1, Math.min(4000, Math.round(width)));
    c.width = Math.round(w * dpr); c.height = Math.round(h * dpr);
    const g = c.getContext("2d")!;
    g.clearRect(0, 0, c.width, c.height);
    if (!l?.peaks) return;
    g.fillStyle = getComputedStyle(c).color;
    const bar = 2 * dpr, gap = 1 * dpr;
    const n = Math.floor(c.width / (bar + gap));
    for (let i = 0; i < n; i++) {
      const a = t.in + ((i / n) * (t.out - t.in)), b = t.in + (((i + 1) / n) * (t.out - t.in));
      let m = 0;
      for (let k = Math.floor(a * PEAKS_PER_SEC); k <= Math.ceil(b * PEAKS_PER_SEC) && k < l.peaks.length; k++) m = Math.max(m, l.peaks[k]);
      const v = Math.max(1 * dpr, (m / l.peakMax) * c.height * 0.9);
      g.fillRect(i * (bar + gap), (c.height - v) / 2, bar, v);
    }
  }, [l?.peaks, t.in, t.out, width, l]);
  return <canvas ref={ref} className="mo-wave" style={{ width: Math.round(width), height: 22 }} />;
}

function Timeline({ list, total: T, selected, errors, onSelect, onMenu, onNote, onTrack, onCaption }: {
  list: Placed[]; total: number; selected: string; errors: Record<string, string>; onSelect: (id: string) => void; onMenu: (at: Rect, id: string) => void;
  onNote: (id: string) => void; onTrack: (id: string) => void; onCaption: () => void;
}) {
  const p = usePlayer();
  useLocal();
  const ref = useRef<HTMLDivElement>(null);
  const [drag, setDrag] = useState<string>("");
  const timeAt = (x: number) => {
    const r = ref.current!.getBoundingClientRect();
    return clamp((x - r.left) / r.width, 0, 1) * T;
  };
  const beats = beatTimes(T);
  function scrub(e: React.PointerEvent) {
    if ((e.target as HTMLElement).closest(".mo-handle, .mo-pin, .mo-tblock, .mo-capblock")) return;
    p.pause();
    const go = (x: number) => p.seek(timeAt(x));
    go(e.clientX);
    const t = sceneAt(timeAt(e.clientX), list);
    if (t) onSelect(t.id);
    const move = (ev: PointerEvent) => go(ev.clientX);
    const up = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up); };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  }
  function resize(e: React.PointerEvent, s: Placed) {
    e.preventDefault(); e.stopPropagation();
    p.pause();
    const r = ref.current!.getBoundingClientRect();
    const pxPerSec = r.width / T;
    const x0 = e.clientX, d0 = s.dur;
    setDrag(s.id);
    const move = (ev: PointerEvent) => {
      let d = clamp(d0 + (ev.clientX - x0) / pxPerSec, MIN_SCENE, 60);
      // (onto a beat of the music when near one; Alt to move freely)
      const snap = 8 / pxPerSec;
      const near = ev.altKey ? null : beats.reduce<number | null>((m, b) => (Math.abs(b - (s.start + d)) < snap && (m === null || Math.abs(b - (s.start + d)) < Math.abs(m - (s.start + d))) ? b : m), null);
      d = near !== null && near - s.start >= MIN_SCENE ? Math.round((near - s.start) * 1000) / 1000 : Math.round(d * 10) / 10;
      draft.durations.set(s.id, d);
      kit.emit();
    };
    const up = () => {
      window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up);
      const d = draft.durations.get(s.id);
      draft.durations.delete(s.id);
      setDrag("");
      if (d != null && d !== d0) patch(scenes, s.id, { duration: d }); else kit.emit();
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  }
  // a track moves along the video (its start), by tenths of a second
  function moveTrack(e: React.PointerEvent, t: PlacedTrack) {
    if (e.button !== 0) return;
    e.preventDefault(); e.stopPropagation();
    const r = ref.current!.getBoundingClientRect();
    const pxPerSec = r.width / T;
    const x0 = e.clientX, s0 = t.start;
    let moved = false, now = s0;
    const el = e.currentTarget as HTMLElement;
    const move = (ev: PointerEvent) => {
      if (Math.abs(ev.clientX - x0) > 3) moved = true;
      if (!moved) return;
      now = Math.max(0, Math.round((s0 + (ev.clientX - x0) / pxPerSec) * 10) / 10);
      el.style.left = `${(now / T) * 100}%`;
    };
    const up = () => {
      window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up);
      if (moved && now !== s0) patch(tracks, t.id, { start: now });
      else onTrack(t.id);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  }
  // the ruler: a mark every 1, 2, 5, 10… seconds, at least 64 px apart
  const [width, setWidth] = useState(800);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth || 800));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const step = [1, 2, 5, 10, 15, 30, 60].find((s) => (width / Math.max(T, 0.001)) * s >= 64) || 60;
  const marks: number[] = [];
  // (none so near the end that its label would stick out)
  for (let s = 0; s <= T * 0.96; s += step) marks.push(s);
  const tl = trackList();
  const caps = captionList();
  const pct = (t: number) => `${(t / Math.max(T, 0.001)) * 100}%`;
  return (
    <div className="mo-timeline">
      <div className="mo-tl" ref={ref} onPointerDown={scrub}>
        <div className="mo-ruler">
          {marks.map((m) => <span key={m} style={{ left: pct(m) }}>{fmt(m)}</span>)}
          {notes.rows.map((n) => {
            const t = num(n.data.time, 0);
            if (t > T) return null;
            return <button key={n.id} type="button" className={`mo-pin${n.data.done ? " is-done" : ""}`} style={{ left: pct(t) }} title={`${fmt(t, true)} ${String(n.data.text || "")}`}
              aria-label={uiText("批注：{0}", String(n.data.text || ""))} onClick={() => { p.pause(); p.seek(t); onNote(n.id); }} />;
          })}
        </div>
        <div className="mo-track">
          {list.map((s) => {
            const tr = String(scenes.get(s.id)?.data.transition || "cut");
            return (
              <div key={s.id} className={`mo-block${s.id === selected ? " is-on" : ""}${drag === s.id ? " is-drag" : ""}${errors[s.id] ? " is-error" : ""}`}
                style={{ left: pct(s.start), width: pct(s.dur) }}
                onContextMenu={(e) => { e.preventDefault(); onSelect(s.id); onMenu({ x: e.clientX, y: e.clientY, w: 0, h: 0 }, s.id); }}>
                {s.index > 0 && tr !== "cut" && <i className="mo-tr" title={TRANSITIONS.find((x) => x[0] === tr)?.[1] || tr} />}
                <span className="mo-block-name">{s.name || uiText("场景 {0}", s.index + 1)}</span>
                <span className="mo-block-dur">{secs(s.dur)}</span>
                <span className="mo-handle" title={beats.length ? uiText("拖动改时长（会吸到节拍上，按住 Alt 不吸）") : uiText("拖动改时长")} onPointerDown={(e) => resize(e, s)} />
              </div>
            );
          })}
        </div>
        {tl.map((t) => (
          <div key={t.id} className="mo-lane">
            <div className={`mo-tblock is-${t.kind}${localFor(t.key) ? "" : " is-missing"}`} style={{ left: pct(t.start), width: pct(t.end - t.start) }}
              title={localFor(t.key) ? t.name : uiText("这台电脑上没有「{0}」", t.name)} onPointerDown={(e) => moveTrack(e, t)}>
              <Wave t={t} width={((t.end - t.start) / Math.max(T, 0.001)) * width} />
              <span className="mo-tblock-name">{t.name}</span>
            </div>
            {t.kind === "music" && t.beats.length > 0 && (
              <div className="mo-beats" aria-hidden="true">
                {t.beats.map((b) => { const at = t.start + b - t.in; return b >= t.in && b <= t.out && at <= T ? <i key={b} style={{ left: pct(at) }} /> : null; })}
              </div>
            )}
          </div>
        ))}
        {caps.length > 0 && (
          <div className="mo-lane is-caps">
            {caps.map((c) => <div key={c.id} className="mo-capblock" style={{ left: pct(c.start), width: pct(Math.min(c.end, T) - c.start) }} title={c.text}
              onPointerDown={(e) => { e.stopPropagation(); p.pause(); p.seek(c.start + 0.01); onCaption(); }}>{c.text}</div>)}
          </div>
        )}
        <div className="mo-head" style={{ left: pct(p.time) }} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------- inspector
function useDraft(value: string, commit: (v: string) => void, wait = 350) {
  const [v, setV] = useState(value);
  const editing = useRef(false);
  const timer = useRef(0);
  useEffect(() => { if (!editing.current) setV(value); }, [value]);
  const set = (nv: string) => {
    setV(nv); editing.current = true;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => { commit(nv); editing.current = false; }, wait);
  };
  const flush = () => { if (editing.current) { window.clearTimeout(timer.current); commit(v); editing.current = false; } };
  return [v, set, flush] as const;
}

function SceneProps({ p, error, focus, onDelete, onDuplicate }: { p: Placed; error?: string; focus: string; onDelete: () => void; onDuplicate: () => void }) {
  const rec = scenes.get(p.id)!;
  const d = rec.data;
  const fields = fieldsOf(String(d.html || ""));
  const values = (d.fields || {}) as Record<string, string>;
  const [name, setName, flushName] = useDraft(String(d.name || ""), (v) => patch(scenes, p.id, { name: v }, `name:${p.id}`));
  const [code, setCode] = useState(false);
  const tr = String(d.transition || "cut");
  return (
    <div className="mo-props">
      <div className="mo-prop">
        <label className="mo-label" htmlFor="mo-name">{uiText("名字")}</label>
        <input id="mo-name" className="inp" value={name} placeholder={uiText("场景 {0}", p.index + 1)} onChange={(e) => setName(e.target.value)} onBlur={flushName} />
      </div>
      <div className="mo-prop-row">
        <div className="mo-prop">
          <label className="mo-label" htmlFor="mo-dur">{uiText("时长（秒）")}</label>
          <NumberInput id="mo-dur" value={num(d.duration, 3)} min={MIN_SCENE} max={60} step={0.5} onChange={(v) => patch(scenes, p.id, { duration: v }, `dur:${p.id}`)} />
        </div>
        <div className="mo-prop">
          <label className="mo-label" htmlFor="mo-tr">{uiText("进场转场")}</label>
          <select id="mo-tr" className="sel" value={p.index === 0 ? "cut" : tr} disabled={p.index === 0} onChange={(e) => patch(scenes, p.id, { transition: e.target.value })}>
            {TRANSITIONS.map(([id, n]) => <option key={id} value={id}>{n}</option>)}
          </select>
        </div>
      </div>
      {error && <div className="mo-error"><AlertTriangle className="ico" /><span>{uiText("动画代码出错：{0}", error)}</span></div>}
      {fields.length > 0 && <div className="mo-group">{uiText("内容")}</div>}
      {fields.map((f) => <FieldInput key={f.key} sceneId={p.id} f={f} value={values[f.key]} focus={focus} />)}
      {fields.length === 0 && <p className="mo-hint">{uiText("这个场景没有可以在这里改的文字。打开下面的代码改，或者让 agent 改。")}</p>}
      <button type="button" className={`mo-code-toggle${code ? " is-on" : ""}`} aria-expanded={code} onClick={() => setCode(!code)}><Code2 className="ico" />{uiText("代码")}</button>
      {code && <CodeEditor id={p.id} />}
      {code && error && <div className="mo-error"><AlertTriangle className="ico" /><span>{error}</span></div>}
      <div className="mo-actions">
        <button type="button" className="btn" onClick={onDuplicate}><Copy className="ico" />{uiText("复制")}</button>
        <button type="button" className="btn mo-del" onClick={onDelete}><Trash2 className="ico" />{uiText("删除")}</button>
      </div>
    </div>
  );
}

function NumberInput({ id, value, min, max, step, onChange }: { id: string; value: number; min: number; max: number; step: number; onChange: (v: number) => void }) {
  const [v, setV] = useState(String(value));
  useEffect(() => setV(String(value)), [value]);
  const commit = (s: string) => { const n = parseFloat(s); if (Number.isFinite(n)) onChange(Math.round(clamp(n, min, max) * 10) / 10); else setV(String(value)); };
  return <input id={id} className="inp" type="number" inputMode="decimal" min={min} max={max} step={step} value={v}
    onChange={(e) => { setV(e.target.value); const n = parseFloat(e.target.value); if (Number.isFinite(n) && n >= min && n <= max) onChange(Math.round(n * 10) / 10); }}
    onBlur={(e) => commit(e.target.value)} />;
}

function FieldInput({ sceneId, f, value, focus }: { sceneId: string; f: Field; value?: string; focus: string }) {
  const ref = useRef<HTMLTextAreaElement & HTMLInputElement>(null);
  const [v, setV, flush] = useDraft(value ?? f.value, (nv) => setField(sceneId, f.key, nv));
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (focus.startsWith(`${sceneId}:${f.key}:`)) { ref.current?.focus(); ref.current?.select?.(); ref.current?.scrollIntoView({ block: "nearest" }); }
  }, [focus]); // eslint-disable-line react-hooks/exhaustive-deps
  if (f.kind === "image") {
    const src = value ? String(assets.get(value)?.data.data || "") : "";
    return (
      <div className="mo-prop">
        <span className="mo-label">{f.label}</span>
        <div className="mo-image">
          <div className="mo-image-pic">{src ? <img src={src} alt="" /> : <ImagePlus className="ico" />}</div>
          <div className="mo-image-act">
            <button type="button" className="btn" disabled={busy} onClick={() => fileRef.current?.click()}>{busy ? <Loader2 className="ico spin" /> : null}{src ? uiText("换一张") : uiText("选图片")}</button>
            {src && <button type="button" className="link-btn" onClick={() => setField(sceneId, f.key, "")}>{uiText("移除")}</button>}
          </div>
        </div>
        <input ref={fileRef} type="file" accept="image/*" hidden onChange={async (e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (!file) return;
          setBusy(true);
          try { const id = await addAsset(file); setField(sceneId, f.key, id); }
          catch (err) { kit.flash((err as Error).message || uiText("图片没加上")); }
          finally { setBusy(false); }
        }} />
      </div>
    );
  }
  return (
    <div className="mo-prop">
      <label className="mo-label" htmlFor={`f-${f.key}`}>{f.label}</label>
      {f.multiline
        ? <textarea id={`f-${f.key}`} ref={ref} className="inp mo-ta" rows={3} value={v} onChange={(e) => setV(e.target.value)} onBlur={flush} />
        : <input id={`f-${f.key}`} ref={ref} className="inp" value={v} onChange={(e) => setV(e.target.value)} onBlur={flush} />}
    </div>
  );
}

function CodeEditor({ id }: { id: string }) {
  const d = scenes.get(id)!.data;
  const [tab, setTab] = useState<"html" | "css" | "script">("html");
  const [v, setV, flush] = useDraft(String(d[tab] || ""), (nv) => patch(scenes, id, { [tab]: nv }, `code:${id}:${tab}`), 600);
  return (
    <div className="mo-code">
      <div className="seg">
        {(["html", "css", "script"] as const).map((k) => (
          <button key={k} type="button" className={tab === k ? "is-on" : ""} onClick={() => { flush(); setTab(k); }}>{k === "script" ? uiText("动画") : k.toUpperCase()}</button>
        ))}
      </div>
      <textarea key={tab} className="inp mo-code-ta" spellCheck={false} value={v} onChange={(e) => setV(e.target.value)} onBlur={flush}
        onKeyDown={(e) => {
          if (e.key === "Tab") { e.preventDefault(); const t = e.currentTarget, s = t.selectionStart; setV(v.slice(0, s) + "  " + v.slice(t.selectionEnd)); requestAnimationFrame(() => { t.selectionStart = t.selectionEnd = s + 2; }); }
        }} />
    </div>
  );
}

function VideoProps() {
  const d = project.get("main")?.data || {};
  const th = { bg: String(d.bg || PALETTES[0].c[0]), fg: String(d.fg || PALETTES[0].c[1]), accent: String(d.accent || PALETTES[0].c[2]), accent2: String(d.accent2 || PALETTES[0].c[3]) };
  const ratio = sizeOf().ratio;
  const font = String(d.font || "sans");
  const fps = fpsOf();
  const colors: [keyof typeof th, string][] = [["bg", uiText("背景")], ["fg", uiText("文字")], ["accent", uiText("强调色")], ["accent2", uiText("第二强调色")]];
  return (
    <div className="mo-props">
      <div className="mo-prop">
        <span className="mo-label">{uiText("画面比例")}</span>
        <div className="seg" role="radiogroup" aria-label={uiText("画面比例")}>
          {RATIOS.map((r) => <button key={r.id} type="button" role="radio" aria-checked={ratio === r.id} className={ratio === r.id ? "is-on" : ""} title={r.name} onClick={() => setProject({ ratio: r.id })}>{r.id}</button>)}
        </div>
      </div>
      <div className="mo-prop">
        <span className="mo-label">{uiText("配色")}</span>
        <div className="mo-palettes">
          {PALETTES.map((pl) => {
            const on = pl.c[0] === th.bg && pl.c[1] === th.fg && pl.c[2] === th.accent && pl.c[3] === th.accent2;
            return (
              <button key={pl.id} type="button" className={`mo-palette${on ? " is-on" : ""}`} aria-pressed={on} title={pl.id}
                onClick={() => setProject({ bg: pl.c[0], fg: pl.c[1], accent: pl.c[2], accent2: pl.c[3] })}
                style={{ background: pl.c[0] }}>
                <i style={{ background: pl.c[1] }} /><i style={{ background: pl.c[2] }} /><i style={{ background: pl.c[3] }} />
              </button>
            );
          })}
        </div>
        <div className="mo-colors">
          {colors.map(([k, label]) => (
            <label key={k} className="mo-color">
              <input type="color" value={/^#[0-9a-f]{6}$/i.test(th[k]) ? th[k] : "#888888"} onChange={(e) => setProject({ [k]: e.target.value }, `color:${k}`)} />
              <span>{label}</span>
            </label>
          ))}
        </div>
      </div>
      <div className="mo-prop">
        <span className="mo-label">{uiText("字体")}</span>
        <div className="seg">
          {FONTS.map((f) => <button key={f.id} type="button" className={font === f.id ? "is-on" : ""} style={{ fontFamily: f.css }} onClick={() => setProject({ font: f.id })}>{f.name}</button>)}
        </div>
      </div>
      <div className="mo-prop">
        <span className="mo-label">{uiText("帧率")}</span>
        <div className="seg">
          {[24, 30, 60].map((f) => <button key={f} type="button" className={fps === f ? "is-on" : ""} onClick={() => setProject({ fps: f })}>{f}</button>)}
        </div>
      </div>
      <p className="mo-hint">{uiText("配色和字体对所有场景生效。")}</p>
    </div>
  );
}

// ---------------------------------------------------------------- agent
function AgentPop({ at, selected, onClose, onReference }: { at: Rect; selected: Placed | null; onClose: () => void; onReference: () => void }) {
  const [text, setText] = useState("");
  const [only, setOnly] = useState(false);
  const [sending, setSending] = useState(false);
  const sendable = canSend();
  const message = useMemo(() => {
    const lines = [
      uiText("请在「动效视频」网页里做视频。"),
      uiText("网页 {0}「{1}」。", kit.page, title.get("") || uiText("未命名视频")),
      only && selected ? uiText("只改场景 {0}（第 {1} 个，「{2}」）。", selected.id, selected.index + 1, selected.name) : "",
      uiText("要求：{0}", text.trim() || uiText("（写在这里）")),
      uiText("按这个网页的 agent.md：场景写在 scenes（html、css、script、fields、duration、transition），比例和配色在 project。写完回我一句。"),
    ];
    return lines.filter(Boolean).join("\n");
  }, [text, only, selected]);
  async function send() {
    if (!text.trim()) return;
    setSending(true);
    try {
      await (myco as unknown as { send: (t: string) => Promise<unknown> }).send(message);
      kit.flash(uiText("已经发给 agent，做好后这里会自动更新"));
      onClose();
    } catch (e) { kit.flash((e as Error).message || uiText("没发出去")); }
    finally { setSending(false); }
  }
  return (
    <Popover at={at} onClose={onClose} width={380} align="right">
      <div className="panel mo-agent">
        <b>{uiText("让 agent 做")}</b>
        <span className="faint">{uiText("说清楚视频讲什么、给谁看、多长、什么风格。agent 会写好场景放进来，你再改文字和时长。")}</span>
        <textarea className="inp mo-ta" rows={5} autoFocus value={text} placeholder={uiText("比如：给我们的记账 App 做一个 20 秒的竖版宣传，突出「拍照记账」和「每月报表」，活泼一点")}
          onChange={(e) => setText(e.target.value)} onKeyDown={(e) => { if ((e.metaKey || e.ctrlKey) && e.key === "Enter") void send(); }} />
        {selected && (
          <label className="mo-check"><input type="checkbox" checked={only} onChange={(e) => setOnly(e.target.checked)} />{uiText("只改选中的场景「{0}」", selected.name || uiText("场景 {0}", selected.index + 1))}</label>
        )}
        <button type="button" className="link-btn mo-agent-ref" onClick={onReference}><Film className="ico" />{uiText("有想照着做的视频？在「分镜」里拆开交给 agent")}</button>
        <div className="mo-agent-foot">
          {sendable ? <span className="faint">{uiText("发给这个网页的发布者")}</span>
            : <button type="button" className="link-btn" onClick={async () => { try { await navigator.clipboard.writeText(message); kit.flash(uiText("已复制，发给任意一个 agent 就行")); } catch { kit.flash(uiText("没复制上")); } }}>{uiText("复制这段话")}</button>}
          <button type="button" className="btn btn-primary" disabled={!text.trim() || sending || !sendable} onClick={() => void send()}>
            {sending ? <Loader2 className="ico spin" /> : <Bot className="ico" />}{uiText("发给 agent")}
          </button>
        </div>
      </div>
    </Popover>
  );
}

// ---------------------------------------------------------------- export
type Exp = { stage: "setup" } | { stage: "run"; done: number; at: number; opts: ExportOptions }
  | { stage: "done"; where: string; size: number; secs: number; warn: string } | { stage: "error"; error: string }
  | { stage: "unsaved"; name: string; blob: Blob; secs: number; error: string };
const SIZE_NAME: Record<number, string> = { 2160: "4K", 1440: "2K", 1080: "1080p", 720: "720p" };
const mmss = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, "0")}`;

function ExportDialog({ state, onStart, onClose }: { state: Exp; onStart: (o: ExportOptions) => void; onClose: () => void }) {
  const [opts, setOpts] = useState<ExportOptions>(defaultOptions);
  const [, tick] = useState(0);
  useEffect(() => { if (state.stage !== "run") return; const t = setInterval(() => tick((n) => n + 1), 1000); return () => clearInterval(t); }, [state.stage]);
  const T = total();
  const head = state.stage === "setup" ? uiText("导出视频") : state.stage === "run" ? uiText("正在导出") : state.stage === "done" ? uiText("导出好了") : state.stage === "unsaved" ? uiText("做好了，还没存上") : uiText("没导出成功");
  let body: JSX.Element;
  if (state.stage === "setup") {
    const est = estimate(opts);
    body = (
      <>
        <div className="field">
          <div className="field-label">{uiText("分辨率")}</div>
          <div className="seg" role="radiogroup" aria-label={uiText("分辨率")}>
            {[720, 1080, 1440, 2160].map((s) => <button key={s} type="button" role="radio" aria-checked={opts.short === s} className={opts.short === s ? "is-on" : ""} onClick={() => setOpts({ ...opts, short: s })}>{SIZE_NAME[s]}</button>)}
          </div>
        </div>
        <div className="field">
          <div className="field-label">{uiText("帧率")}</div>
          <div className="seg" role="radiogroup" aria-label={uiText("帧率")}>
            {[24, 30, 60].map((f) => <button key={f} type="button" role="radio" aria-checked={opts.fps === f} className={opts.fps === f ? "is-on" : ""} onClick={() => setOpts({ ...opts, fps: f })}>{f}</button>)}
          </div>
        </div>
        <div className="field">
          <div className="field-label">{uiText("画质")}</div>
          <div className="seg" role="radiogroup" aria-label={uiText("画质")}>
            {QUALITIES.map(([q, n]) => <button key={q} type="button" role="radio" aria-checked={opts.quality === q} className={opts.quality === q ? "is-on" : ""} onClick={() => setOpts({ ...opts, quality: q })}>{n}</button>)}
          </div>
        </div>
        <p className="export-sum"><b>{est.w}×{est.h}</b> · {opts.fps} fps · {mmss(T)} · ≈ {est.mb < 10 ? est.mb.toFixed(1) : Math.round(est.mb)} MB</p>
        <p className="faint">{uiText("在这台电脑上一帧一帧画出来，不上传。")}</p>
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
    const pct = state.done * 100;
    const took = (performance.now() - state.at) / 1000;
    const left = pct > 4 ? (took / pct) * (100 - pct) : 0;
    const { w, h } = outSize(state.opts.short);
    body = (
      <>
        <p className="faint">{w}×{h} · {state.opts.fps} fps · {mmss(T)}</p>
        <div className="bar"><i style={{ width: `${pct}%` }} /></div>
        <div className="modal-foot">
          <span className="faint"><Loader2 className="ico spin" /> {Math.floor(pct)}% · {mmss(took)}{left ? uiText(" · 还要约 {0}", mmss(left)) : ""}</span>
          <button type="button" className="btn" onClick={onClose}>{uiText("取消")}</button>
        </div>
      </>
    );
  } else if (state.stage === "done") {
    body = (
      <>
        <p>{uiText("已存到 {0}（{1} MB，用了 {2}）", state.where, (state.size / 1024 / 1024).toFixed(1), mmss(state.secs))}</p>
        {state.warn && <p className="modal-err">{state.warn}</p>}
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
        <p>{uiText("视频做好了，但没存到「下载」：")}</p>
        <p className="modal-err">{x.error}</p>
        <div className="modal-foot">
          <button type="button" className="btn" onClick={browser}>{uiText("用浏览器下载")}</button>
          <button type="button" className="btn" onClick={onClose}>{uiText("关闭")}</button>
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

mount(kit, App);
