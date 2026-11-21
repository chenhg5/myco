import { t as uiText } from "../_lib/i18n";
// The side panels besides the scene's form: notes on moments of the video
// for people and agents, the shot plan (Motion.md) with a reference video
// to copy, the music, voice and captions, and saved templates.
import { useEffect, useMemo, useRef, useState } from "react";
import {
  AudioLines, Bot, Captions, Check, Download, FileUp, Film, Loader2, MessageSquareText, Music, Pencil, Plus, Save, Trash2, Upload, Wand2, X,
} from "lucide-react";
import { kit, captions, captionList, clamp, fmt, num, notes, placed, project, reference, refFrames, sceneAt, secs, total, tracks, MIN_SCENE, scenes } from "./model";
import { change, patch, setProject, undo } from "./edit";
import { myco, save } from "../_lib/myco";
import { uid } from "../_lib/appkit";
import { player, usePlayer } from "./player";
import { findBeats, keep, localFor, onLocal, trackList, wav16k, type PlacedTrack } from "./audio";
import { installSpeech, speechState, transcribe, type SpeechState } from "./speech";
import { takeApart, type Stage } from "./reference";
import { importTemplate, items, removeTemplate, templateFile, useTemplate } from "./library";

// ---------------------------------------------------------------- agent
export const canSend = () => typeof (myco as unknown as { send?: unknown })?.send === "function";

/** Sends a request to the page's agent, or copies it where the page cannot send. */
export async function toAgent(text: string) {
  if (canSend()) {
    try {
      await (myco as unknown as { send: (t: string) => Promise<unknown> }).send(text);
      kit.flash(uiText("已经发给 agent，做好后这里会自动更新"));
      return true;
    } catch (e) { kit.flash((e as Error).message || uiText("没发出去")); return false; }
  }
  try { await navigator.clipboard.writeText(text); kit.flash(uiText("已复制，发给任意一个 agent 就行")); return true; }
  catch { kit.flash(uiText("没复制上")); return false; }
}
const pageLine = (title: string) => uiText("网页 {0}「{1}」。", kit.page, title || uiText("未命名视频"));

export function useLocal() {
  const [, set] = useState(0);
  useEffect(() => onLocal(() => set((n) => n + 1)), []);
}

// ---------------------------------------------------------------- notes
export function NotesPanel({ focus, title }: { focus: string; title: string }) {
  usePlayer();
  const [text, setText] = useState("");
  const ref = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const list = [...notes.rows].sort((a, b) => num(a.data.time, 0) - num(b.data.time, 0));
  const open = list.filter((n) => !n.data.done);
  useEffect(() => {
    if (focus === "new") inputRef.current?.focus();
    else if (focus) ref.current?.querySelector(`[data-id="${focus.split(":")[0]}"]`)?.scrollIntoView({ block: "nearest" });
  }, [focus]);
  const add = () => {
    const s = text.trim();
    if (!s) return;
    change([[notes, uid("n"), { time: Math.round(player.time * 10) / 10, text: s, done: false, by: kit.me }]]);
    setText("");
  };
  const ask = async () => {
    const all = placed();
    const lines = open.map((n) => {
      const t = num(n.data.time, 0), s = sceneAt(t, all);
      return `- ${fmt(t, true)}${s ? uiText("（场景 {0}「{1}」）", s.id, s.name) : ""}：${String(n.data.text || "").replace(/\s+/g, " ")}`;
    });
    await toAgent([
      uiText("请按批注改「动效视频」网页里的视频。"), pageLine(title),
      uiText("没处理的批注（时间是视频里的时间）："), ...lines,
      uiText("按这个网页的 agent.md 改 scenes（和 project），只改批注说到的地方；改完一条就把那条批注（notes）的 done 设成 true。写完回我一句。"),
    ].join("\n"));
  };
  return (
    <div className="mo-props" ref={ref}>
      <div className="mo-note-add">
        <textarea ref={inputRef} className="inp mo-ta" rows={2} value={text} placeholder={uiText("在 {0} 留一条批注，比如「这里慢一点」「标题换成…」", fmt(player.time, true))}
          onChange={(e) => setText(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); add(); } }} />
        <button type="button" className="btn" disabled={!text.trim()} onClick={add}><MessageSquareText className="ico" />{uiText("加批注")}</button>
      </div>
      {list.length === 0 && <p className="mo-hint">{uiText("把播放头放到要改的地方，写下要怎么改。批注会挂在时间线上，攒几条后一起让 agent 改。")}</p>}
      <div className="mo-notes">
        {list.map((n) => (
          <div key={n.id} data-id={n.id} className={`mo-note${n.data.done ? " is-done" : ""}${focus.startsWith(n.id) ? " is-focus" : ""}`}>
            <button type="button" className="mo-note-check" aria-label={n.data.done ? uiText("标成没处理") : uiText("标成已处理")} onClick={() => patch(notes, n.id, { done: !n.data.done })}>
              {n.data.done ? <Check className="ico" /> : null}
            </button>
            <div className="mo-note-body">
              <button type="button" className="mo-note-time" onClick={() => { player.pause(); player.seek(num(n.data.time, 0)); }}>{fmt(num(n.data.time, 0), true)}</button>
              <span className="mo-note-text">{String(n.data.text || "")}</span>
              {n.data.by ? <span className="mo-note-by">{String(n.data.by)}</span> : null}
            </div>
            <button type="button" className="icon-btn" aria-label={uiText("删除批注")} onClick={() => change([[notes, n.id, null]])}><Trash2 className="ico" /></button>
          </div>
        ))}
      </div>
      {open.length > 0 && (
        <button type="button" className="btn btn-primary mo-wide" onClick={() => void ask()}>
          <Bot className="ico" />{canSend() ? uiText("让 agent 改这 {0} 条", open.length) : uiText("复制给 agent 的话（{0} 条）", open.length)}
        </button>
      )}
    </div>
  );
}

// ---------------------------------------------------------------- shot plan
function inline(s: string) {
  return s.replace(/`([^`]+)`/g, "<code>$1</code>").replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>").replace(/(^|[^*])\*([^*\s][^*]*)\*/g, "$1<i>$2</i>");
}
/** A little Markdown (headings, lists, bold, code, paragraphs) as HTML; the text is escaped first. */
export function markdown(md: string) {
  const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  const out: string[] = [];
  let list: "ul" | "ol" | "" = "", para: string[] = [];
  const flush = () => { if (para.length) { out.push(`<p>${inline(para.join("<br>"))}</p>`); para = []; } };
  const close = () => { if (list) { out.push(`</${list}>`); list = ""; } };
  for (const raw of esc(md).split("\n")) {
    const line = raw.trimEnd();
    let m: RegExpExecArray | null;
    if (!line.trim()) { flush(); close(); continue; }
    if ((m = /^(#{1,4})\s+(.*)$/.exec(line))) { flush(); close(); out.push(`<h${m[1].length + 1}>${inline(m[2])}</h${m[1].length + 1}>`); continue; }
    if ((m = /^\s*[-*]\s+(.*)$/.exec(line)) || (m = /^\s*\d+[.)]\s+(.*)$/.exec(line))) {
      flush();
      const kind = /^\s*\d/.test(line) ? "ol" : "ul";
      if (list !== kind) { close(); out.push(`<${kind}>`); list = kind; }
      out.push(`<li>${inline(m[1])}</li>`);
      continue;
    }
    if (/^\s*(---|\*\*\*)\s*$/.test(line)) { flush(); close(); out.push("<hr>"); continue; }
    close();
    para.push(line);
  }
  flush(); close();
  return out.join("");
}

export function PlanPanel({ title }: { title: string }) {
  const motion = String(project.get("main")?.data.motion || "");
  const [editing, setEditing] = useState(false);
  const [v, setV] = useState(motion);
  useEffect(() => { if (!editing) setV(motion); }, [motion, editing]);
  const html = useMemo(() => markdown(motion), [motion]);
  const ref = reference.get("main")?.data;
  return (
    <div className="mo-props">
      <div className="mo-plan-head">
        <span className="mo-group">{uiText("分镜")}</span>
        {editing
          ? <span className="row"><button type="button" className="link-btn" onClick={() => setEditing(false)}>{uiText("取消")}</button>
            <button type="button" className="btn btn-primary" onClick={() => { setProject({ motion: v }); setEditing(false); }}>{uiText("保存")}</button></span>
          : <button type="button" className="btn" onClick={() => setEditing(true)}><Pencil className="ico" />{motion ? uiText("改") : uiText("写分镜")}</button>}
      </div>
      {editing
        ? <textarea className="inp mo-code-ta mo-plan-ta" spellCheck={false} autoFocus value={v} onChange={(e) => setV(e.target.value)}
          placeholder={uiText("## 1. 开头 0–3 秒\n- 画面：…\n- 文字：…\n- 动效：…")} />
        : motion ? <div className="mo-md" dangerouslySetInnerHTML={{ __html: html }} />
          : <p className="mo-hint">{uiText("分镜写下每个场景讲什么、多长、怎么动，还有哪些要保留（品牌色、节奏）。让 agent 做视频时它会先写好这里；改视频时它也照着这里。")}</p>}
      <div className="mo-sep" />
      <RefSection title={title} has={!!ref} />
    </div>
  );
}

type Taking = { stage: Stage; name: string } | null;
function RefSection({ title, has }: { title: string; has: boolean }) {
  const ref = reference.get("main")?.data;
  const frames = [...refFrames.rows].sort((a, b) => num(a.data.time, 0) - num(b.data.time, 0));
  const [taking, setTaking] = useState<Taking>(null);
  const [want, setWant] = useState("");
  const abort = useRef<AbortController | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  async function take(file: File) {
    const ac = new AbortController();
    abort.current = ac;
    setTaking({ stage: { step: "frames", done: 0 }, name: file.name });
    try {
      const r = await takeApart(file, (stage) => setTaking({ stage, name: file.name }), ac.signal);
      const ops: Parameters<typeof change>[0] = [];
      for (const f of refFrames.rows) ops.push([refFrames, f.id, null]);
      ops.push([reference, "main", { name: r.name, duration: r.duration, width: r.width, height: r.height, transcript: r.transcript, frames: r.frames.length, by: kit.me }]);
      r.frames.forEach((f, i) => ops.push([refFrames, uid("f"), { time: f.time, data: f.data, text: f.text, order: i + 1 }]));
      change(ops);
      kit.flash(r.heard === "yes" ? uiText("拆好了：{0} 个关键帧和台词", r.frames.length) : r.why ? uiText("拆好了 {0} 个关键帧；没转台词：{1}", r.frames.length, r.why) : uiText("拆好了 {0} 个关键帧（没听到说话）", r.frames.length));
    } catch (e) {
      if ((e as Error).message !== "cancelled") kit.flash((e as Error).message || uiText("没拆成"));
    } finally { setTaking(null); abort.current = null; }
  }
  const ask = () => void toAgent([
    uiText("请照参考视频在「动效视频」网页里做一个视频。"), pageLine(title),
    uiText("参考视频「{0}」（{1}）已经拆好：关键帧在 ref_frames（time 是参考视频里的秒数，data 是 JPEG 的 data URL，存成文件再看），台词在 reference 的 transcript。", ref?.name || "", secs(num(ref?.duration, 0))),
    uiText("照它的镜头、节奏和动效重做，内容换成：{0}", want.trim() || uiText("（照原样）")),
    uiText("先把逐镜的分镜写进 project.motion，再按 agent.md 写 scenes。写完回我一句。"),
  ].join("\n"));
  const step = taking ? (taking.stage.step === "frames" ? uiText("找镜头") : taking.stage.step === "sound" ? uiText("读声音") : uiText("转台词")) : "";
  return (
    <>
      <div className="mo-plan-head">
        <span className="mo-group">{uiText("参考视频")}</span>
        {has && !taking && <button type="button" className="link-btn" onClick={() => {
          const ops: Parameters<typeof change>[0] = [[reference, "main", null]];
          for (const f of refFrames.rows) ops.push([refFrames, f.id, null]);
          change(ops);
        }}>{uiText("清除")}</button>}
      </div>
      {!has && !taking && <p className="mo-hint">{uiText("选一段喜欢的视频，在这台电脑上拆出每个镜头的画面和台词，交给 agent 照着做。视频不会上传。")}</p>}
      {taking && (
        <div className="mo-taking">
          <span className="faint"><Loader2 className="ico spin" /> {taking.name} · {step} {Math.round(taking.stage.done * 100)}%</span>
          <div className="bar"><i style={{ width: `${taking.stage.done * 100}%` }} /></div>
          <button type="button" className="link-btn" onClick={() => abort.current?.abort()}>{uiText("停止")}</button>
        </div>
      )}
      {has && ref && !taking && (
        <>
          <p className="mo-ref-meta"><Film className="ico" /><b>{ref.name}</b><span className="faint">{secs(num(ref.duration, 0))} · {uiText("{0} 个镜头", frames.length)}{ref.transcript ? uiText(" · 有台词") : ""}</span></p>
          <div className="mo-ref-frames">
            {frames.map((f) => (
              <figure key={f.id} title={String(f.data.text || "")}>
                <img src={String(f.data.data || "")} alt="" />
                <figcaption>{fmt(num(f.data.time, 0), true)}</figcaption>
              </figure>
            ))}
          </div>
          {ref.transcript ? <details className="mo-ref-text"><summary>{uiText("台词")}</summary><pre>{String(ref.transcript)}</pre></details> : null}
          <textarea className="inp mo-ta" rows={3} value={want} onChange={(e) => setWant(e.target.value)} placeholder={uiText("内容换成什么，比如：换成我们的记账 App，突出拍照记账；保留它的节奏和转场")} />
          <button type="button" className="btn btn-primary mo-wide" onClick={ask}><Bot className="ico" />{canSend() ? uiText("让 agent 照着做") : uiText("复制给 agent 的话")}</button>
        </>
      )}
      {!taking && <button type="button" className="btn" onClick={() => fileRef.current?.click()}><Upload className="ico" />{has ? uiText("换一段参考视频") : uiText("选参考视频")}</button>}
      <input ref={fileRef} type="file" accept="video/*" hidden onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ""; if (f) void take(f); }} />
    </>
  );
}

// ---------------------------------------------------------------- sound
/** Moves the scene cuts to the nearest beats (one undo). */
export function alignToBeats(beats: number[]) {
  if (!beats.length) return 0;
  const list = placed();
  const ops: Parameters<typeof change>[0] = [];
  let t = 0, moved = 0;
  for (const p of list) {
    const end = t + p.dur;
    let best = end, bd = Infinity;
    for (const b of beats) { const d = Math.abs(b - end); if (b - t >= MIN_SCENE && d < bd) { bd = d; best = b; } }
    if (bd > 1.2) best = end;
    const dur = Math.round(Math.max(MIN_SCENE, best - t) * 1000) / 1000;
    if (Math.abs(dur - p.dur) > 0.005) { ops.push([scenes, p.id, { ...scenes.get(p.id)!.data, duration: dur }]); moved++; }
    t += dur;
  }
  if (ops.length) change(ops);
  return moved;
}

export function SoundPanel({ focus }: { focus: string }) {
  useLocal();
  usePlayer();
  const list = trackList();
  const [adding, setAdding] = useState<"" | "music" | "voice">("");
  const fileRef = useRef<HTMLInputElement>(null);
  const kindRef = useRef<"music" | "voice">("music");
  const T = total();
  async function add(file: File, kind: "music" | "voice", replace?: string) {
    setAdding(kind);
    try {
      const k = await keep(file);
      const rec: Record<string, unknown> = { kind, name: file.name, key: k.key, size: file.size, duration: Math.round(k.duration * 1000) / 1000 };
      if (kind === "music" && k.local.buffer) {
        const b = findBeats(k.local.buffer);
        rec.beats = b.beats.slice(0, 1500); rec.bpm = b.bpm;
      }
      if (replace) {
        const cur = tracks.get(replace)!.data;
        change([[tracks, replace, { ...cur, ...rec, kind: cur.kind || kind, out: Math.min(num(cur.out, k.duration), k.duration) }]]);
      } else {
        const voices = list.filter((t) => t.kind === "voice");
        change([[tracks, uid("k"), { ...rec, start: kind === "voice" && voices.length ? Math.round(player.time * 10) / 10 : 0, in: 0, out: rec.duration, volume: kind === "music" ? 0.8 : 1, fade_in: 0, fade_out: kind === "music" ? 1.5 : 0, order: Date.now() }]]);
      }
    } catch (e) {
      kit.flash((e as Error).message === "decode" ? uiText("这个文件读不出声音") : (e as Error).message || uiText("没加上"));
    } finally { setAdding(""); }
  }
  const pickFor = (kind: "music" | "voice") => { kindRef.current = kind; fileRef.current?.click(); };
  return (
    <div className="mo-props">
      <div className="mo-sound-add">
        <button type="button" className="btn" disabled={!!adding} onClick={() => pickFor("music")}>{adding === "music" ? <Loader2 className="ico spin" /> : <Music className="ico" />}{uiText("加音乐")}</button>
        <button type="button" className="btn" disabled={!!adding} onClick={() => pickFor("voice")}>{adding === "voice" ? <Loader2 className="ico spin" /> : <AudioLines className="ico" />}{uiText("加配音")}</button>
      </div>
      <input ref={fileRef} type="file" accept="audio/*,video/*,.mp3,.m4a,.wav,.aac,.ogg,.flac" hidden onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ""; if (f) void add(f, kindRef.current); }} />
      {list.length === 0 && <p className="mo-hint">{uiText("音乐和配音文件只存在这台电脑上，网页里只记它们放在哪。加了音乐会自动找节拍，场景切换可以对上节拍；配音可以在本机转成字幕。")}</p>}
      {list.map((t) => <TrackCard key={t.id} t={t} total={T} on={focus.startsWith(t.id)} onRepick={(f) => void add(f, t.kind, t.id)} />)}
      <div className="mo-sep" />
      <CaptionSection voices={list.filter((t) => t.kind === "voice")} />
    </div>
  );
}

function TrackCard({ t, total: T, on, onRepick }: { t: PlacedTrack; total: number; on: boolean; onRepick: (f: File) => void }) {
  const l = localFor(t.key);
  const rec = tracks.get(t.id)!.data;
  const fileRef = useRef<HTMLInputElement>(null);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { if (on) ref.current?.scrollIntoView({ block: "nearest" }); }, [on]);
  const beats = t.kind === "music" ? t.beats.filter((b) => b >= t.in && b <= t.out).map((b) => t.start + b - t.in).filter((b) => b > 0 && b <= T + 0.5) : [];
  const set = (p: Record<string, unknown>, key?: string) => patch(tracks, t.id, p, key);
  return (
    <div ref={ref} className={`mo-trackcard${on ? " is-on" : ""}`}>
      <div className="mo-trackcard-head">
        {t.kind === "music" ? <Music className="ico" /> : <AudioLines className="ico" />}
        <b title={t.name}>{t.name || (t.kind === "music" ? uiText("音乐") : uiText("配音"))}</b>
        <button type="button" className="icon-btn" aria-label={uiText("删除")} onClick={() => change([[tracks, t.id, null]])}><Trash2 className="ico" /></button>
      </div>
      {!l && (
        <div className="mo-error"><span>{uiText("这台电脑上没有这个文件（只存在加它的那台电脑上）。")}<button type="button" className="link-btn" onClick={() => fileRef.current?.click()}>{uiText("选这个文件")}</button></span></div>
      )}
      <input ref={fileRef} type="file" accept="audio/*,video/*" hidden onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ""; if (f) onRepick(f); }} />
      <div className="mo-prop-row">
        <div className="mo-prop"><span className="mo-label">{uiText("从第几秒开始")}</span><Secs value={t.start} onChange={(v) => set({ start: v })} /></div>
        <div className="mo-prop"><span className="mo-label">{uiText("跳过文件开头")}</span><Secs value={t.in} onChange={(v) => set({ in: clamp(v, 0, t.out - 0.1) })} /></div>
      </div>
      <div className="mo-prop">
        <span className="mo-label">{uiText("音量 {0}%", Math.round(t.volume * 100))}</span>
        <input type="range" min={0} max={1.5} step={0.05} value={t.volume} onChange={(e) => set({ volume: Number(e.target.value) }, `vol:${t.id}`)} />
      </div>
      <div className="mo-prop-row">
        <div className="mo-prop"><span className="mo-label">{uiText("淡入（秒）")}</span><Secs value={t.fadeIn} onChange={(v) => set({ fade_in: clamp(v, 0, 10) })} /></div>
        <div className="mo-prop"><span className="mo-label">{uiText("淡出（秒）")}</span><Secs value={t.fadeOut} onChange={(v) => set({ fade_out: clamp(v, 0, 10) })} /></div>
      </div>
      {t.kind === "music" && (
        <div className="mo-beat">
          <span className="faint">{rec.bpm ? uiText("{0} 拍每分钟 · 视频里 {1} 拍", rec.bpm, beats.length) : uiText("没找到节拍")}</span>
          {beats.length > 1 && <button type="button" className="btn" onClick={() => {
            const n = alignToBeats(beats);
            kit.flash(n ? uiText("{0} 个场景的切换对上了节拍", n) : uiText("场景切换已经在节拍上"), n ? undo : undefined);
          }}><Wand2 className="ico" />{uiText("切换对上节拍")}</button>}
        </div>
      )}
      {t.end > T + 0.05 && t.start < T && <p className="mo-hint">{uiText("视频 {0} 结束，后面的 {1} 不会导出。", fmt(T, true), secs(t.end - T))}</p>}
      {t.start >= T && <p className="mo-hint">{uiText("它在视频结束之后，听不到。")}</p>}
    </div>
  );
}

function Secs({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  const [s, setS] = useState(String(Math.round(value * 10) / 10));
  useEffect(() => setS(String(Math.round(value * 10) / 10)), [value]);
  const done = () => { const v = Number(s); if (Number.isFinite(v) && v >= 0) onChange(Math.round(v * 100) / 100); else setS(String(value)); };
  return <input className="inp" value={s} inputMode="decimal" onChange={(e) => setS(e.target.value)} onBlur={done} onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} />;
}

const CAP_STYLES: [string, string][] = [["bottom", uiText("底部")], ["box", uiText("字幕条")], ["top", uiText("顶部")], ["off", uiText("不显示")]];
const CAP_SIZES: [number, string][] = [[0.8, uiText("小")], [1, uiText("中")], [1.3, uiText("大")]];

function CaptionSection({ voices }: { voices: PlacedTrack[] }) {
  usePlayer();
  const p = project.get("main")?.data || {};
  const style = String(p.captions || "bottom"), size = num(p.caption_size, 1);
  const list = captionList();
  const [sp, setSp] = useState<SpeechState | null>(null);
  const [job, setJob] = useState<{ what: string; done: number } | null>(null);
  const abort = useRef<AbortController | null>(null);
  useEffect(() => { if (voices.length) void speechState().then(setSp); }, [voices.length]);
  async function install() {
    setJob({ what: uiText("下载语音识别"), done: 0 });
    try { await installSpeech((d) => setJob({ what: uiText("下载语音识别"), done: d })); setSp(await speechState()); }
    catch (e) { kit.flash((e as Error).message || uiText("没下成")); }
    finally { setJob(null); }
  }
  async function fromVoice(t: PlacedTrack) {
    const l = localFor(t.key);
    if (!l?.buffer) { kit.flash(uiText("这台电脑上没有这个配音文件")); return; }
    const ac = new AbortController();
    abort.current = ac;
    setJob({ what: uiText("识别「{0}」", t.name), done: 0 });
    try {
      const segs = await transcribe(wav16k(l.buffer, t.in, t.out), (d) => setJob({ what: uiText("识别「{0}」", t.name), done: d }), ac.signal);
      const ops: Parameters<typeof change>[0] = [];
      // the captions of this stretch of the video are replaced
      for (const c of captions.rows) { const s = num(c.data.start, 0); if (s >= t.start - 0.05 && s < t.end) ops.push([captions, c.id, null]); }
      for (const s of segs) {
        const text = s.text.trim();
        if (!text) continue;
        ops.push([captions, uid("c"), { start: Math.round((t.start + s.start) * 100) / 100, end: Math.round((t.start + s.end) * 100) / 100, text }]);
      }
      change(ops);
      kit.flash(segs.length ? uiText("加了 {0} 条字幕", segs.length) : uiText("没听到说话"));
    } catch (e) {
      if ((e as Error).message !== "cancelled" && (e as { code?: string }).code !== "cancelled") kit.flash((e as Error).message || uiText("没识别成"));
    } finally { setJob(null); abort.current = null; }
  }
  return (
    <>
      <div className="mo-plan-head"><span className="mo-group"><Captions className="ico" /> {uiText("字幕")}</span>
        {list.length > 0 && <button type="button" className="link-btn" onClick={() => { change(captions.rows.map((c) => [captions, c.id, null])); }}>{uiText("清空")}</button>}
      </div>
      {voices.length > 0 && (
        sp && !sp.can ? <p className="mo-hint">{sp.why}</p>
          : sp && !sp.ready ? <div className="mo-beat"><span className="faint">{uiText("配音转字幕要先下载语音识别（约 {0} MB，只下一次）", sp.sizeMb)}</span><button type="button" className="btn" disabled={!!job} onClick={() => void install()}><Download className="ico" />{uiText("下载")}</button></div>
            : voices.map((t) => <button key={t.id} type="button" className="btn" disabled={!!job || !sp} onClick={() => void fromVoice(t)}><Wand2 className="ico" />{uiText("从「{0}」生成字幕", t.name)}</button>)
      )}
      {job && (
        <div className="mo-taking">
          <span className="faint"><Loader2 className="ico spin" /> {job.what} {Math.round(job.done * 100)}%</span>
          <div className="bar"><i style={{ width: `${job.done * 100}%` }} /></div>
          {abort.current && <button type="button" className="link-btn" onClick={() => abort.current?.abort()}>{uiText("停止")}</button>}
        </div>
      )}
      <div className="mo-prop">
        <span className="mo-label">{uiText("样式")}</span>
        <div className="seg">{CAP_STYLES.map(([id, n]) => <button key={id} type="button" className={style === id ? "is-on" : ""} onClick={() => setProject({ captions: id })}>{n}</button>)}</div>
      </div>
      <div className="mo-prop">
        <span className="mo-label">{uiText("大小")}</span>
        <div className="seg">{CAP_SIZES.map(([v, n]) => <button key={v} type="button" className={Math.abs(size - v) < 0.01 ? "is-on" : ""} onClick={() => setProject({ caption_size: v })}>{n}</button>)}</div>
      </div>
      <div className="mo-caps">
        {list.map((c) => <CaptionRow key={c.id} id={c.id} start={c.start} end={c.end} text={c.text} on={player.time >= c.start && player.time < c.end} />)}
      </div>
      <button type="button" className="btn" onClick={() => {
        const t = Math.round(player.time * 10) / 10;
        const next = list.find((c) => c.start > t);
        change([[captions, uid("c"), { start: t, end: Math.min(next ? next.start : t + 2.5, t + 2.5), text: uiText("字幕") }]]);
      }}><Plus className="ico" />{uiText("在 {0} 加一条", fmt(player.time, true))}</button>
      {list.length === 0 && !voices.length && <p className="mo-hint">{uiText("加了配音就能自动生成字幕；也可以在这里一条条加。字幕会画进导出的视频里。")}</p>}
    </>
  );
}

function CaptionRow({ id, start, end, text, on }: { id: string; start: number; end: number; text: string; on: boolean }) {
  const [v, setV] = useState(text);
  useEffect(() => setV(text), [text]);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { if (on && !player.playing) ref.current?.scrollIntoView({ block: "nearest" }); }, [on]);
  return (
    <div ref={ref} className={`mo-cap-row${on ? " is-on" : ""}`}>
      <button type="button" className="mo-note-time" onClick={() => { player.pause(); player.seek(start + 0.01); }}>{fmt(start, true)}</button>
      <input className="inp" value={v} aria-label={uiText("字幕 {0}–{1}", fmt(start, true), fmt(end, true))} onChange={(e) => setV(e.target.value)}
        onBlur={() => { if (v !== text) patch(captions, id, { text: v }); }} onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} />
      <button type="button" className="icon-btn" aria-label={uiText("删除")} onClick={() => change([[captions, id, null]])}><X className="ico" /></button>
    </div>
  );
}

// ---------------------------------------------------------------- templates
export function LibrarySection({ onSave, onUsed }: { onSave: (name: string) => void; onUsed: (first: string) => void }) {
  const list = items("video");
  const [name, setName] = useState("");
  const [naming, setNaming] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const empty = scenes.rows.length === 0;
  return (
    <div className="mo-props mo-lib">
      <div className="mo-plan-head"><span className="mo-group">{uiText("模板")}</span>
        <button type="button" className="link-btn" onClick={() => fileRef.current?.click()}><FileUp className="ico" />{uiText("导入")}</button>
      </div>
      <input ref={fileRef} type="file" accept=".json,application/json" hidden onChange={async (e) => {
        const f = e.target.files?.[0]; e.target.value = "";
        if (!f) return;
        try { await importTemplate(f); kit.flash(uiText("模板已导入")); } catch (err) { kit.flash((err as Error).message); }
      }} />
      {naming ? (
        <div className="mo-note-add">
          <input className="inp" autoFocus value={name} placeholder={uiText("模板的名字")} onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") { onSave(name); setNaming(false); setName(""); } if (e.key === "Escape") setNaming(false); }} />
          <span className="row"><button type="button" className="link-btn" onClick={() => setNaming(false)}>{uiText("取消")}</button>
            <button type="button" className="btn btn-primary" onClick={() => { onSave(name); setNaming(false); setName(""); }}>{uiText("存")}</button></span>
        </div>
      ) : <button type="button" className="btn" disabled={empty} onClick={() => { setName(String(project.get("main")?.data.title || "")); setNaming(true); }}><Save className="ico" />{uiText("把这个视频存成模板")}</button>}
      {list.length === 0 && <p className="mo-hint">{uiText("存成模板后，打开这个网页的人都能用它做新视频：场景、配色、分镜都在，改文字就行。也能导出成文件，带到别的网页。")}</p>}
      {list.map((it) => (
        <div key={it.id} className="mo-libitem">
          <div className="mo-libthumb">{it.data.thumb ? <img src={String(it.data.thumb)} alt="" /> : null}</div>
          <div className="mo-libmeta">
            <b>{String(it.data.name || "")}</b>
            <span className="faint">{it.data.ratio} · {uiText("{0} 个场景", num(it.data.scenes, 0))} · {secs(num(it.data.duration, 0))}{it.data.by ? ` · ${it.data.by}` : ""}</span>
            <span className="row">
              <button type="button" className="link-btn" onClick={() => {
                const first = useTemplate(it.id);
                onUsed(first);
                kit.flash(uiText("已换成模板「{0}」", String(it.data.name || "")), undo);
              }}>{uiText("用这个")}</button>
              <button type="button" className="link-btn" onClick={() => { const f = templateFile(it.id); if (f) void save(`${String(it.data.name || "template")}.motion.json`, JSON.stringify(f), "application/json").then((r) => kit.flash(uiText("已存到 {0}", r.path))).catch((e) => kit.flash((e as Error).message)); }}>{uiText("导出")}</button>
              <button type="button" className="link-btn mo-del" onClick={() => { removeTemplate(it.id); kit.flash(uiText("已删除模板"), undo); }}>{uiText("删除")}</button>
            </span>
          </div>
        </div>
      ))}
    </div>
  );
}
