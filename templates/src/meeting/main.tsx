// 会议记录: record a meeting held in any app (Zoom, 腾讯会议, 飞书…) as two
// tracks — "me" (the microphone) and "them" (what the computer plays) —
// then transcribe it on this computer and read, play and fix the text.
// The recording stays on the computer that made it; the transcript is the
// page's records, for people and agents alike.
//
// Quiet as the App is: greys, one solid button a screen, colour only for a
// state (the red dot of a recording). Reading a meeting: a timeline of who
// spoke when (click or drag it to go there; search hits are marked on it),
// paragraphs by speaker (a click plays from that sentence, a double click
// edits it in place), a player that skips silence, and keys for all of it.
import "../_lib/sheet.css";
import "./meeting.css";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  AppWindow, ArrowDown, ArrowUp, Check, ChevronsRight, CircleSlash, Download, FastForward, FileAudio, Loader2, Mic, Monitor, MoreHorizontal,
  Pause, Play, Plus, RefreshCw, Rewind, Search, Square, Trash2, X, TextCursor, Repeat, Copy,
} from "lucide-react";
import { Toast, mount, useKit } from "../_lib/appkit";
import { save } from "../_lib/myco";
import type { OSAudioSource } from "../_lib/myco";
import { os, osState, osStateText, useOS, formatBytes } from "../_lib/os";
import { Popover, rectOf } from "../_lib/ui";
import { clock, kit, length, meetingList, meetingRank, meetings, num, segments, segmentsOf, speakerName, when, type Meeting, type Track } from "./model";
import {
  checkSpeech, clearRecordError, installError, installSpeech, installing, isWaiting, lastRecordError, pauseRecording, recording,
  speech, startRecording, stopRecording, stopTranscribing, transcribe, transcribing, useEngine,
} from "./engine";
import { Player } from "./player";

const canUse = osState === "ready";
const TRACKS: Track[] = ["them", "me"];

function App() {
  useKit(kit);
  useEngine();
  const sys = useOS();
  const [sel, setSel] = useState<string>(() => (recording ? recording.meeting : meetingList()[0]?.id || "new"));
  useEffect(() => { if (canUse) void checkSpeech(); }, []);
  // a recording that ended without the page (it was closed while it ran)
  useEffect(() => {
    for (const r of meetings.rows) {
      if (r.data.status === "recording" && recording?.meeting !== r.id && r.data.computer === sys.info?.hostname) {
        void kit.track(meetings.put({ ...r.data, status: "recorded" }, r.id));
      }
    }
  }, [sys.info]); // eslint-disable-line react-hooks/exhaustive-deps
  const list = meetingList();
  const cur = sel !== "new" ? meetings.get(sel) : null;
  const live = recording?.meeting;
  return (
    <div className="app mt-app">
      <aside className="mt-side">
        <div className="mt-side-head">
          <span className="mt-side-title">会议记录</span>
          <button type="button" className="icon-btn" aria-label="新录音" title="新录音" onClick={() => setSel(live || "new")}><Plus className="ico" /></button>
        </div>
        <div className="mt-list" role="listbox" aria-label="会议">
          {!live && (
            <button type="button" role="option" aria-selected={!cur} className={`mt-item is-new${!cur ? " is-on" : ""}`} onClick={() => setSel("new")}>
              <span className="mt-item-ico"><Mic className="ico" /></span><b>新录音</b>
            </button>
          )}
          {list.length > 0 && <div className="mt-list-label">会议</div>}
          {list.map((r) => {
            const m = r.data;
            const isLive = live === r.id;
            const tx = transcribing?.meeting === r.id;
            return (
              <button key={r.id} type="button" role="option" aria-selected={sel === r.id} className={`mt-item${sel === r.id ? " is-on" : ""}`} onClick={() => setSel(r.id)}>
                <span className="mt-item-text">
                  <b>{m.title || "会议"}</b>
                  <span>
                    {isLive ? <><i className="dot is-rec" />录音中 · {clock(recording!.seconds)}</>
                      : tx ? <><i className="dot is-work" />转写中</>
                      : <>{when(m.started_at)}{num(m.seconds) ? ` · ${length(num(m.seconds))}` : ""}{m.status === "recorded" ? " · 没转写" : m.status === "failed" ? " · 转写没成" : ""}</>}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      </aside>
      <main className="mt-main">
        {!canUse && <div className="mt-notice">{osStateText()} 录音和转写要在 myco App 里；这里能看和改文字。</div>}
        {live && (sel === live || !cur) ? <RecordingView id={live} />
          : !cur ? <NewRecording computer={sys.info?.hostname || ""} onStarted={setSel} />
          : <MeetingView key={cur.id} id={cur.id} host={sys.info?.hostname || ""} onDeleted={() => setSel(meetingList()[0]?.id || "new")} />}
      </main>
      <Toast kit={kit} />
    </div>
  );
}

// ---- a new recording: what to record, then one button ----

function NewRecording({ computer, onStarted }: { computer: string; onStarted: (id: string) => void }) {
  useEngine();
  const [sources, setSources] = useState<OSAudioSource[] | null>(null);
  const [why, setWhy] = useState("");
  const [me, setMe] = useState(true);
  const [them, setThem] = useState<string>("system");
  const [busy, setBusy] = useState(false);
  const load = async () => {
    if (!canUse) return;
    try {
      const l = await os().audio.sources();
      setSources(l.sources);
      setWhy("");
      const playing = l.sources.filter((s) => s.kind === "app" && s.playing && meetingRank(s) < 9).sort((a, b) => meetingRank(a) - meetingRank(b));
      if (playing.length) setThem((t) => (t === "system" ? playing[0].id : t));
    } catch (e) { setWhy((e as Error).message); setSources([]); }
  };
  useEffect(() => { void load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const system = sources?.find((s) => s.kind === "system");
  const apps = (sources || []).filter((s) => s.kind === "app").sort((a, b) => Number(!!b.playing) - Number(!!a.playing) || meetingRank(a) - meetingRank(b) || a.name.localeCompare(b.name));
  const themName = them === "none" ? "" : them === "system" ? "电脑的声音" : apps.find((a) => a.id === them)?.name || them;
  const go = async () => {
    setBusy(true);
    clearRecordError();
    try { onStarted(await startRecording({ me, them: them === "none" ? null : them, themName }, computer)); } catch (e) { kit.flash((e as Error).message); }
    setBusy(false);
  };
  return (
    <div className="mt-page">
      <div className="mt-page-in">
        <header className="mt-page-head">
          <h1>新录音</h1>
          <p>开会时录下你说的和别人说的，结束后在这台电脑上转成文字，录音不上传。</p>
        </header>
        {lastRecordError && <div className="mt-alert is-err"><span className="mt-alert-grow"><b>没录上：</b>{lastRecordError}</span><button type="button" className="icon-btn" aria-label="关掉" onClick={clearRecordError}><X className="ico" /></button></div>}

        <section className="mt-group">
          <div className="mt-group-title">我</div>
          <label className="mt-row">
            <span className="mt-row-ico"><Mic className="ico" /></span>
            <span className="mt-row-text"><b>麦克风</b><span>你说的话</span></span>
            <Switch on={me} onChange={setMe} disabled={!canUse} label="录麦克风" />
          </label>
        </section>

        <section className="mt-group">
          <div className="mt-group-title">
            别人
            <button type="button" className="mt-link" onClick={() => void load()} title="会议开始后，会议软件才会出现在这里"><RefreshCw className="ico" />刷新</button>
          </div>
          <div className="mt-choices" role="radiogroup" aria-label="别人的声音">
            {system && <Choice id="system" on={them === "system"} onPick={setThem} icon={<Monitor className="ico" />} name="电脑的全部声音" sub="所有应用放出的声音" />}
            {apps.map((a) => <Choice key={a.id} id={a.id} on={them === a.id} onPick={setThem} icon={<AppWindow className="ico" />} name={a.name} sub={a.playing ? "正在出声" : "只录这个应用"} live={a.playing} />)}
            <Choice id="none" on={them === "none"} onPick={setThem} icon={<CircleSlash className="ico" />} name="不录" sub="只录麦克风" />
          </div>
          {why && <p className="mt-hint">{why}</p>}
          {sources && !system && !why && <p className="mt-hint">这台电脑录不了电脑放出的声音（macOS 14.2 起才行），可以只录麦克风。</p>}
        </section>

        <section className="mt-group">
          <div className="mt-group-title">转写</div>
          <SpeechRow />
        </section>

        <div className="mt-start">
          <button type="button" className="btn btn-primary mt-go" disabled={!canUse || busy || (!me && them === "none")} onClick={() => void go()}>
            {busy ? <Loader2 className="ico spin" /> : <i className="dot is-rec" />}开始录音
          </button>
          <p className="mt-hint">录别人之前，先告诉参会的人。录音时可以收起网页面板；关掉网页会停止录音。</p>
        </div>
      </div>
    </div>
  );
}

function Switch({ on, onChange, disabled, label }: { on: boolean; onChange: (v: boolean) => void; disabled?: boolean; label: string }) {
  return (
    <button type="button" role="switch" aria-checked={on} aria-label={label} className={`mt-switch${on ? " is-on" : ""}`} disabled={disabled}
      onClick={(e) => { e.preventDefault(); onChange(!on); }}><i /></button>
  );
}

function Choice({ id, on, onPick, icon, name, sub, live }: { id: string; on: boolean; onPick: (id: string) => void; icon: ReactNode; name: string; sub: string; live?: boolean }) {
  return (
    <button type="button" role="radio" aria-checked={on} className={`mt-row is-choice${on ? " is-on" : ""}`} onClick={() => onPick(id)} disabled={!canUse}>
      <span className="mt-row-ico">{icon}</span>
      <span className="mt-row-text"><b>{name}</b><span>{live && <i className="dot is-live" />}{sub}</span></span>
      {on && <Check className="ico mt-row-check" />}
    </button>
  );
}

function SpeechRow() {
  useEngine();
  if (!canUse) return <div className="mt-row is-static"><span className="mt-row-text"><span>{osStateText()}</span></span></div>;
  if (installing) {
    const pct = installing.total ? Math.round((installing.received / installing.total) * 100) : 0;
    const what = installing.step === "engine" ? "转写引擎" : installing.step === "vad" ? "断句模型" : "识别模型";
    return (
      <div className="mt-row is-static">
        <span className="mt-row-ico"><Loader2 className="ico spin" /></span>
        <span className="mt-row-text">
          <b>{installing.unpacking ? `正在解开${what}` : `正在下载${what}`}</b>
          <span>{installing.total && !installing.unpacking ? `${formatBytes(installing.received)} / ${formatBytes(installing.total)}` : "请稍候"}</span>
          <i className="mt-progress"><i style={{ width: `${installing.unpacking ? 100 : pct}%` }} /></i>
        </span>
      </div>
    );
  }
  if (!speech) return <div className="mt-row is-static"><span className="mt-row-ico"><Loader2 className="ico spin" /></span><span className="mt-row-text"><span>正在看转写装好了没有</span></span></div>;
  if (speech.ready) {
    return (
      <div className="mt-row is-static">
        <span className="mt-row-ico"><Check className="ico" /></span>
        <span className="mt-row-text"><b>本机转写</b><span>SenseVoice · 中文、英文、粤语、日语、韩语 · 录完自动转</span></span>
      </div>
    );
  }
  const size = speech.engine_size_mb + speech.models.filter((m) => !m.installed).reduce((a, m) => a + m.size_mb, 0);
  return (
    <div className="mt-row is-static">
      <span className="mt-row-ico"><Download className="ico" /></span>
      <span className="mt-row-text">
        <b>先下载转写模型</b>
        <span>{installError ? <em className="is-err">没下成：{installError}</em> : `约 ${size} MB，只下一次，之后都在这台电脑上转`}</span>
      </span>
      <button type="button" className="btn" onClick={() => void installSpeech()}>{installError ? "再试一次" : "下载"}</button>
    </div>
  );
}

// ---- recording ----

function Wave({ peaks }: { peaks: number[] }) {
  const cv = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const c = cv.current;
    if (!c) return;
    const dpr = window.devicePixelRatio || 1;
    const w = c.clientWidth, h = c.clientHeight;
    if (c.width !== Math.round(w * dpr) || c.height !== Math.round(h * dpr)) { c.width = Math.round(w * dpr); c.height = Math.round(h * dpr); }
    const ctx = c.getContext("2d")!;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    ctx.fillStyle = getComputedStyle(c).color;
    const bar = 2, gap = 2, n = Math.floor(w / (bar + gap));
    const list = peaks.slice(-n);
    // (the newest at the right; silence a quiet line, speech grows)
    for (let i = 0; i < n; i++) {
      const p = list[i - (n - list.length)];
      const v = p === undefined ? 0 : Math.min(1, Math.sqrt(p) * 1.3);
      const bh = Math.max(1.5, v * h);
      ctx.globalAlpha = p === undefined ? 0.12 : 0.22 + 0.78 * (i / n);
      ctx.fillRect(i * (bar + gap), (h - bh) / 2, bar, bh);
    }
  });
  return <canvas ref={cv} className="mt-wave" />;
}

function RecordingView({ id }: { id: string }) {
  useEngine();
  const r = recording;
  const m = meetings.get(id)?.data;
  if (!r || !m) return null;
  const tracks = TRACKS.filter((t) => m.sources?.[t]);
  return (
    <div className="mt-rec">
      <span className={`mt-pill${r.paused ? " is-paused" : ""}`}><i className="dot is-rec" />{r.stopping ? "正在结束" : r.paused ? "已暂停" : "录音中"}</span>
      <div className="mt-clock">{clock(r.seconds)}</div>
      <TitleInput id={id} m={m} center />
      <div className="mt-tracks">
        {tracks.map((t) => (
          <div key={t} className="mt-track">
            <span className="mt-track-who"><Face m={m} t={t} />{speakerName(m, t)}<em>{t === "me" ? "麦克风" : m.source_name || "电脑的声音"}</em></span>
            <Wave peaks={r.levels[t] || []} />
            {(r.quiet[t] || 0) > 20 && <span className="mt-track-quiet">{t === "them" ? "一直没有声音：会议在出声吗？" : "一直没有声音：麦克风打开了吗？"}</span>}
          </div>
        ))}
      </div>
      <div className="mt-rec-acts">
        <button type="button" className="btn mt-big" onClick={() => pauseRecording(!r.paused)} disabled={r.stopping}>
          {r.paused ? <><Play className="ico" />继续</> : <><Pause className="ico" />暂停</>}
        </button>
        <button type="button" className="btn btn-primary mt-big" onClick={stopRecording} disabled={r.stopping}>
          {r.stopping ? <Loader2 className="ico spin" /> : <Square className="ico is-fill" />}结束录音
        </button>
      </div>
      <p className="mt-hint">结束后在这台电脑上转成文字{speech?.ready ? "" : "（先在「新录音」里下载转写模型）"}。可以收起网页面板，关掉网页会停止录音。</p>
    </div>
  );
}

function TitleInput({ id, m, center }: { id: string; m: Meeting; center?: boolean }) {
  const [v, setV] = useState<string | null>(null);
  const t = useRef(0);
  return (
    <input className={`mt-title${center ? " is-center" : ""}`} value={v ?? m.title ?? ""} placeholder="会议" aria-label="会议名称" maxLength={120}
      onChange={(e) => {
        const val = e.target.value;
        setV(val);
        window.clearTimeout(t.current);
        t.current = window.setTimeout(() => {
          const cur = meetings.get(id);
          if (cur) void kit.track(meetings.put({ ...cur.data, title: val.trim() || "会议" }, id)).finally(() => setV(null));
        }, 500);
      }}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === "Escape") (e.target as HTMLInputElement).blur(); }} />
  );
}

/** A speaker's face: the first character of the name, in grey (me darker). */
function Face({ m, t, size }: { m: Meeting; t: Track; size?: number }) {
  const name = speakerName(m, t);
  return <i className={`mt-face is-${t}`} style={size ? { width: size, height: size, fontSize: size * 0.5 } : undefined} aria-hidden>{[...name][0] || "?"}</i>;
}

// ---- a meeting ----

type Para = { track: Track; ids: string[]; start: number; end: number };

function paragraphs(list: { id: string; data: { track?: Track; start?: number; end?: number } }[]) {
  const out: Para[] = [];
  for (const s of list) {
    const t = (s.data.track || "them") as Track;
    const last = out[out.length - 1];
    if (last && last.track === t && num(s.data.start) - last.end < 20 && last.ids.length < 12) { last.ids.push(s.id); last.end = num(s.data.end); }
    else out.push({ track: t, ids: [s.id], start: num(s.data.start), end: num(s.data.end) });
  }
  return out;
}

function MeetingView({ id, host, onDeleted }: { id: string; host: string; onDeleted: () => void }) {
  useEngine();
  const m = meetings.get(id)!.data;
  const segs = segmentsOf(id);
  const [gone, setGone] = useState(false);
  const firstFile = Object.values(m.files || {}).find(Boolean);
  const here = canUse && !!firstFile && !gone && (!m.computer || !host || m.computer === host);
  // (the recording may have been moved or deleted since)
  useEffect(() => {
    if (!canUse || !firstFile) return;
    let live = true;
    os().fs.stat({ path: firstFile }).then(() => live && setGone(false), () => live && setGone(true));
    return () => { live = false; };
  }, [firstFile]);
  const player = useMemo(() => (here ? new Player(Object.values(m.files || {}).filter(Boolean) as string[]) : null), [here, JSON.stringify(m.files)]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => player?.destroy(), [player]);
  const [, setTick] = useState(0);
  useEffect(() => player?.subscribe(() => setTick((n) => n + 1)), [player]);
  useEffect(() => { player?.setSpeech(segs.map((s) => [num(s.data.start), num(s.data.end)])); });

  const [only, setOnly] = useState<Track | null>(null);
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [hit, setHit] = useState(0);
  const [follow, setFollow] = useState(true);
  const [editing, setEditing] = useState<string | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; w: number; h: number } | null>(null);
  const [ctx, setCtx] = useState<{ sid: string; x: number; y: number } | null>(null);
  const body = useRef<HTMLDivElement>(null);
  const searchBox = useRef<HTMLInputElement>(null);
  const tx = transcribing?.meeting === id ? transcribing : null;
  const duration = Math.max(num(m.seconds), player?.duration || 0, segs.length ? num(segs[segs.length - 1].data.end) : 0);

  const paras = useMemo(() => paragraphs(only ? segs.filter((s) => s.data.track === only) : segs), [segs, only]);
  const now = player?.time ?? -1;
  const playingId = player && (player.playing || now > 0) ? segs.find((s) => num(s.data.start) - 0.05 <= now && num(s.data.end) + 0.05 >= now)?.id || null : null;

  const q = query.trim().toLowerCase();
  const hits = useMemo(() => (q ? segs.filter((s) => (!only || s.data.track === only) && String(s.data.text || "").toLowerCase().includes(q)).map((s) => s.id) : []), [segs, q, only]);
  useEffect(() => { setHit(0); }, [q]);
  const hitId = hits.length ? hits[Math.min(hit, hits.length - 1)] : null;
  const scrollTo = useCallback((sid: string | null) => {
    if (sid) body.current?.querySelector(`[data-s="${sid}"]`)?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, []);
  useEffect(() => { if (hitId) scrollTo(hitId); }, [hitId, scrollTo]);
  useEffect(() => { if (follow && player?.playing && playingId && !editing) scrollTo(playingId); }, [playingId, follow, player?.playing, editing, scrollTo]);

  const playFrom = (t: number) => { if (!player) return; player.seek(t); player.play(); setFollow(true); };
  const step = (dir: 1 | -1) => {
    if (!player || !segs.length) return;
    const t = player.time;
    let i = segs.findIndex((s) => num(s.data.end) > t);
    if (i < 0) i = segs.length - 1;
    if (dir > 0) i = Math.min(segs.length - 1, num(segs[i].data.start) > t + 0.05 ? i : i + 1);
    else i = Math.max(0, t - num(segs[i].data.start) > 1.5 ? i : i - 1);
    player.seek(num(segs[i].data.start));
  };

  // keys: space, ← →, ⇧← ⇧→, ⌘F
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement;
      const typing = el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable);
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f") { e.preventDefault(); setSearching(true); setTimeout(() => searchBox.current?.select(), 0); return; }
      if (typing || !player || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === " ") { e.preventDefault(); if (player.playing) player.pause(); else player.play(); }
      else if (e.key === "ArrowRight") { e.preventDefault(); if (e.shiftKey) step(1); else player.seek(player.time + 5); }
      else if (e.key === "ArrowLeft") { e.preventDefault(); if (e.shiftKey) step(-1); else player.seek(player.time - 5); }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  });

  const talk = useMemo(() => {
    const out: Partial<Record<Track, number>> = {};
    for (const s of segs) { const t = (s.data.track || "them") as Track; out[t] = (out[t] || 0) + Math.max(0, num(s.data.end) - num(s.data.start)); }
    return out;
  }, [segs]);
  const totalTalk = Object.values(talk).reduce((a, b) => a + (b || 0), 0);
  const speakers = TRACKS.filter((t) => talk[t]);

  const exportAs = async (kind: "txt" | "md" | "srt") => {
    setMenu(null);
    const name = (m.title || "会议").replace(/[\\/:*?"<>|]/g, "_");
    let out = "";
    if (kind === "srt") {
      const t = (s: number) => {
        const ms = Math.round(s * 1000), p = (n: number, w = 2) => String(n).padStart(w, "0");
        return `${p(Math.floor(ms / 3600000))}:${p(Math.floor(ms / 60000) % 60)}:${p(Math.floor(ms / 1000) % 60)},${p(ms % 1000, 3)}`;
      };
      out = segs.map((s, i) => `${i + 1}\n${t(num(s.data.start))} --> ${t(num(s.data.end))}\n${speakerName(m, s.data.track)}：${s.data.text}\n`).join("\n");
    } else {
      const text = (p: Para) => p.ids.map((i) => segments.get(i)?.data.text || "").join("");
      const all = paragraphs(segs);
      out = kind === "md"
        ? `# ${m.title || "会议"}\n\n${when(m.started_at)} · ${length(num(m.seconds))}\n\n` + all.map((p) => `**${speakerName(m, p.track)}** · ${clock(p.start)}\n\n${text(p)}`).join("\n\n") + "\n"
        : `${m.title || "会议"}\n${when(m.started_at)} · ${length(num(m.seconds))}\n\n` + all.map((p) => `${speakerName(m, p.track)}  ${clock(p.start)}\n${text(p)}`).join("\n\n") + "\n";
    }
    const r = await save(`${name}.${kind}`, out, kind === "md" ? "text/markdown" : "text/plain").catch((e: Error) => { kit.flash(e.message); return null; });
    if (r) kit.flash(`已存到 ${r.path}`);
  };
  const remove = async () => {
    setMenu(null);
    if (!window.confirm(`删除「${m.title || "会议"}」和它的文字稿？${here ? "录音会移进废纸篓。" : ""}`)) return;
    stopTranscribing();
    for (const s of segs) await segments.remove(s.id).catch(() => undefined);
    await meetings.remove(id).catch(() => undefined);
    if (here && m.dir) await os().fs.trash({ path: m.dir }).catch(() => undefined);
    kit.emit();
    onDeleted();
  };

  return (
    <div className="mt-meeting">
      <header className="mt-bar">
        <div className="mt-bar-main">
          <TitleInput id={id} m={m} />
          <span className="mt-meta">
            {when(m.started_at)} · {length(num(m.seconds))}{m.source_name ? ` · ${m.source_name}` : ""}{m.computer && m.computer !== host ? ` · 录在 ${m.computer}` : ""}
          </span>
        </div>
        {searching || q ? (
          <div className="mt-find">
            <Search className="ico" />
            <input ref={searchBox} autoFocus value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索文字稿" aria-label="搜索文字稿"
              onKeyDown={(e) => {
                if (e.key === "Enter" && hits.length) { e.preventDefault(); setHit((h) => (e.shiftKey ? (h - 1 + hits.length) % hits.length : (h + 1) % hits.length)); }
                if (e.key === "Escape") { setQuery(""); setSearching(false); }
              }} />
            {q && <span className="mt-find-n">{hits.length ? `${Math.min(hit, hits.length - 1) + 1}/${hits.length}` : "0"}</span>}
            <button type="button" className="icon-btn" aria-label="上一处" disabled={!hits.length} onClick={() => setHit((h) => (h - 1 + hits.length) % hits.length)}><ArrowUp className="ico" /></button>
            <button type="button" className="icon-btn" aria-label="下一处" disabled={!hits.length} onClick={() => setHit((h) => (h + 1) % hits.length)}><ArrowDown className="ico" /></button>
            <button type="button" className="icon-btn" aria-label="关掉搜索" onClick={() => { setQuery(""); setSearching(false); }}><X className="ico" /></button>
          </div>
        ) : (
          <button type="button" className="icon-btn" aria-label="搜索" title="搜索（⌘F）" onClick={() => setSearching(true)}><Search className="ico" /></button>
        )}
        <button type="button" className="icon-btn" aria-label="更多" onClick={(e) => setMenu(rectOf(e.currentTarget))}><MoreHorizontal className="ico" /></button>
        {menu && (
          <Popover at={menu} onClose={() => setMenu(null)} align="right">
            <div className="menu">
              <button type="button" className="menu-item" disabled={!segs.length} onClick={() => void exportAs("txt")}><Download className="ico" />导出文字稿</button>
              <button type="button" className="menu-item" disabled={!segs.length} onClick={() => void exportAs("md")}><Download className="ico" />导出 Markdown</button>
              <button type="button" className="menu-item" disabled={!segs.length} onClick={() => void exportAs("srt")}><Download className="ico" />导出字幕（SRT）</button>
              {here && m.dir && <button type="button" className="menu-item" onClick={() => { setMenu(null); void os().fs.reveal({ path: m.dir! }); }}><FileAudio className="ico" />显示录音文件</button>}
              {here && <button type="button" className="menu-item" disabled={!!tx} onClick={() => { setMenu(null); if (!segs.length || window.confirm("重新转写会换掉现在的文字稿，包括改过的字。")) void transcribe(id); }}><RefreshCw className="ico" />重新转写</button>}
              <div className="menu-sep" />
              <button type="button" className="menu-item is-danger" onClick={() => void remove()}><Trash2 className="ico" />删除</button>
            </div>
          </Popover>
        )}
      </header>

      {segs.length > 0 && (
        <Timeline m={m} segs={segs} duration={duration} speakers={speakers} talk={talk} totalTalk={totalTalk} now={player ? player.time : -1}
          hits={hits} hitId={hitId} only={only} setOnly={setOnly} onSeek={player ? (t) => player.seek(t) : undefined} />
      )}

      <div className="mt-body" ref={body}>
        <div className="mt-doc">
          {tx && <TranscribingRow />}
          {!tx && isWaiting(id) && <div className="mt-alert"><Loader2 className="ico spin" /><span className="mt-alert-grow">排队中：前一场转完就转这场</span></div>}
          {m.status === "failed" && !tx && <div className="mt-alert is-err"><span className="mt-alert-grow"><b>转写没成：</b>{m.error}</span>{here && <button type="button" className="btn" onClick={() => void transcribe(id)}>再试一次</button>}</div>}
          {!tx && !segs.length && <EmptyTranscript id={id} m={m} here={here} />}
          {paras.map((p) => (
            <section key={p.ids[0]} className={`mt-para${p.ids.includes(playingId || "") ? " is-on" : ""}`}>
              <div className="mt-para-head">
                <Face m={m} t={p.track} />
                <SpeakerName id={id} m={m} t={p.track} />
                <button type="button" className="mt-para-time" disabled={!player} onClick={() => playFrom(p.start)} title={player ? "从这里播放" : undefined}>{clock(p.start)}</button>
              </div>
              <p className="mt-para-text">
                {p.ids.map((sid) => {
                  const s = segments.get(sid)?.data;
                  if (!s) return null;
                  return (
                    <Sentence key={sid} sid={sid} text={String(s.text || "")} edited={!!s.edited} q={q}
                      on={sid === playingId} hit={sid === hitId} editing={editing === sid}
                      onPlay={player ? () => playFrom(num(s.start)) : undefined}
                      onEdit={() => setEditing(sid)} onDone={() => setEditing(null)}
                      onMenu={(x, y) => setCtx({ sid, x, y })} />
                  );
                })}
              </p>
            </section>
          ))}
          {segs.length > 0 && (
            <p className="mt-tip"><TextCursor className="ico" />单击一句从那里播放，双击改字，右键有更多。空格播放或暂停，← → 前后 5 秒，⇧← ⇧→ 上一句、下一句，⌘F 搜索。</p>
          )}
        </div>
      </div>

      {ctx && (() => {
        const s = segments.get(ctx.sid)?.data;
        if (!s) return null;
        const other: Track = s.track === "me" ? "them" : "me";
        const close = () => setCtx(null);
        return (
          <Popover at={{ x: ctx.x, y: ctx.y }} onClose={close}>
            <div className="menu">
              {player && <button type="button" className="menu-item" onClick={() => { close(); playFrom(num(s.start)); }}><Play className="ico" />从这里播放</button>}
              <button type="button" className="menu-item" onClick={() => { close(); setEditing(ctx.sid); }}><TextCursor className="ico" />改字</button>
              <button type="button" className="menu-item" onClick={() => { close(); void kit.track(segments.put({ ...s, track: other }, ctx.sid)); kit.emit(); }}><Repeat className="ico" />改成{speakerName(m, other)}说的</button>
              <button type="button" className="menu-item" onClick={() => { close(); void navigator.clipboard?.writeText(String(s.text || "")).then(() => kit.flash("已复制"), () => kit.flash("复制不了")); }}><Copy className="ico" />复制</button>
              <div className="menu-sep" />
              <button type="button" className="menu-item is-danger" onClick={() => { close(); void kit.track(segments.remove(ctx.sid)); kit.emit(); kit.flash("已删掉这句"); }}><Trash2 className="ico" />删除这句</button>
            </div>
          </Popover>
        );
      })()}
      {player ? (
        <PlayerBar player={player} duration={duration} follow={follow} setFollow={setFollow} />
      ) : m.files ? (
        <div className="mt-away">{gone && m.computer === host ? "录音文件不在了（移走或删掉了），这里只能看文字。" : `录音在 ${m.computer || "另一台电脑"} 上，这里只能看文字。`}</div>
      ) : null}
    </div>
  );
}

function Timeline({ m, segs, duration, speakers, talk, totalTalk, now, hits, hitId, only, setOnly, onSeek }: {
  m: Meeting; segs: { id: string; data: { track?: Track; start?: number; end?: number } }[]; duration: number; speakers: Track[];
  talk: Partial<Record<Track, number>>; totalTalk: number; now: number; hits: string[]; hitId: string | null;
  only: Track | null; setOnly: (t: Track | null) => void; onSeek?: (t: number) => void;
}) {
  const lanes = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const at = (x: number) => {
    const r = lanes.current!.getBoundingClientRect();
    return Math.max(0, Math.min(1, (x - r.left) / r.width)) * duration;
  };
  const pct = (t: number) => (Math.max(0, Math.min(duration, t)) / Math.max(1, duration)) * 100;
  const hitSet = new Set(hits);
  return (
    <div className="mt-timeline">
      <div className="mt-tl-names">
        {speakers.map((t) => (
          <button key={t} type="button" className={`mt-tl-name${only === t ? " is-on" : ""}${only && only !== t ? " is-off" : ""}`} onClick={() => setOnly(only === t ? null : t)}
            title={only === t ? "显示所有人" : `只看${speakerName(m, t)}说的`}>
            <Face m={m} t={t} size={18} /><b>{speakerName(m, t)}</b><em>{totalTalk ? Math.round(((talk[t] || 0) / totalTalk) * 100) : 0}%</em>
          </button>
        ))}
      </div>
      <div ref={lanes} className={`mt-tl-lanes${onSeek ? "" : " is-still"}`}
        onPointerDown={(e) => { if (!onSeek) return; (e.currentTarget as Element).setPointerCapture(e.pointerId); onSeek(at(e.clientX)); }}
        onPointerMove={(e) => { setHover(at(e.clientX)); if (onSeek && e.buttons) onSeek(at(e.clientX)); }}
        onPointerLeave={() => setHover(null)}>
        {speakers.map((t) => (
          <div key={t} className={`mt-lane${only && only !== t ? " is-off" : ""}`}>
            {segs.filter((s) => s.data.track === t).map((s) => {
              const a = pct(num(s.data.start)), b = pct(num(s.data.end));
              return <i key={s.id} className={`mt-lane-seg is-${t}`} style={{ left: `${a}%`, width: `max(2px, ${b - a}%)` }} />;
            })}
          </div>
        ))}
        {segs.filter((s) => hitSet.has(s.id)).map((s) => <b key={s.id} className={`mt-tl-hit${s.id === hitId ? " is-on" : ""}`} style={{ left: `${pct(num(s.data.start))}%` }} />)}
        {now >= 0 && <span className="mt-tl-head" style={{ left: `${pct(now)}%` }} />}
        {hover !== null && <span className="mt-tl-tip" style={{ left: `${pct(hover)}%` }}>{clock(hover)}</span>}
      </div>
    </div>
  );
}

function SpeakerName({ id, m, t }: { id: string; m: Meeting; t: Track }) {
  const [edit, setEdit] = useState<string | null>(null);
  const done = (v: string) => {
    setEdit(null);
    const cur = meetings.get(id);
    if (!cur || !v.trim() || v.trim() === speakerName(m, t)) return;
    void kit.track(meetings.put({ ...cur.data, speakers: { ...(cur.data.speakers || {}), [t]: v.trim() } }, id));
  };
  if (edit !== null) {
    return <input className="mt-name-edit" autoFocus onFocus={(e) => e.target.select()} value={edit} onChange={(e) => setEdit(e.target.value)} onBlur={() => done(edit)}
      onKeyDown={(e) => { if (e.key === "Enter") done(edit); if (e.key === "Escape") setEdit(null); }} aria-label="名字" maxLength={40} />;
  }
  return <button type="button" className="mt-name" title="改名字（这场会议里都改）" onClick={() => setEdit(speakerName(m, t))}>{speakerName(m, t)}</button>;
}

function Sentence({ sid, text, edited, q, on, hit, editing, onPlay, onEdit, onDone, onMenu }: {
  sid: string; text: string; edited: boolean; q: string; on: boolean; hit: boolean; editing: boolean;
  onPlay?: () => void; onEdit: () => void; onDone: () => void; onMenu: (x: number, y: number) => void;
}) {
  const ref = useRef<HTMLSpanElement>(null);
  const clickT = useRef(0);
  useEffect(() => {
    if (!editing || !ref.current) return;
    const el = ref.current;
    el.focus();
    const r = document.createRange();
    r.selectNodeContents(el);
    r.collapse(false);
    const sel = window.getSelection();
    sel?.removeAllRanges();
    sel?.addRange(r);
  }, [editing]);
  const commit = () => {
    const v = (ref.current?.innerText || "").replace(/\s*\n\s*/g, " ").trim();
    onDone();
    const cur = segments.get(sid);
    if (!cur || v === (cur.data.text || "")) return;
    if (!v) { void kit.track(segments.remove(sid)); kit.emit(); return; }
    void kit.track(segments.put({ ...cur.data, text: v, edited: true }, sid));
    kit.emit();
  };
  if (editing) {
    return (
      <span ref={ref} className="mt-s is-editing" contentEditable suppressContentEditableWarning data-s={sid} onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") { e.preventDefault(); (e.target as HTMLElement).blur(); }
          if (e.key === "Escape") { e.preventDefault(); if (ref.current) ref.current.innerText = text; (e.target as HTMLElement).blur(); }
        }}>{text}</span>
    );
  }
  return (
    <span className={`mt-s${on ? " is-on" : ""}${hit ? " is-hit" : ""}${edited ? " is-edited" : ""}`} data-s={sid}
      // (a single click plays, after a moment, so a double click can edit instead)
      onClick={() => { window.clearTimeout(clickT.current); if (window.getSelection()?.toString()) return; clickT.current = window.setTimeout(() => onPlay?.(), 220); }}
      onDoubleClick={(e) => { e.preventDefault(); window.clearTimeout(clickT.current); onEdit(); }}
      onContextMenu={(e) => { e.preventDefault(); window.clearTimeout(clickT.current); onMenu(e.clientX, e.clientY); }}
      title={edited ? "改过 · 双击再改" : undefined}>
      {q ? highlight(text, q) : text}
    </span>
  );
}

function highlight(text: string, q: string) {
  const out: ReactNode[] = [];
  const low = text.toLowerCase();
  let at = 0, k = 0;
  for (;;) {
    const i = low.indexOf(q, at);
    if (i < 0) break;
    out.push(text.slice(at, i), <mark key={k++}>{text.slice(i, i + q.length)}</mark>);
    at = i + q.length;
  }
  out.push(text.slice(at));
  return out;
}

function TranscribingRow() {
  useEngine();
  const t = transcribing;
  if (!t) return null;
  const pct = t.total ? Math.min(100, Math.round((t.done / t.total) * 100)) : 0;
  return (
    <div className="mt-alert">
      <Loader2 className="ico spin" />
      <span className="mt-alert-grow">
        正在转写{t.tracks.length > 1 ? ` · ${t.track === "me" ? "我说的" : "别人说的"}（${t.index + 1}/${t.tracks.length}）` : ""} · {pct}%
        <i className="mt-progress"><i style={{ width: `${pct}%` }} /></i>
      </span>
      <button type="button" className="btn" onClick={stopTranscribing}>停止</button>
    </div>
  );
}

function EmptyTranscript({ id, m, here }: { id: string; m: Meeting; here: boolean }) {
  useEngine();
  if (m.status === "failed") return null;
  if (!here) {
    return <div className="mt-empty"><b>还没有文字稿</b><span>{m.computer ? `录音在 ${m.computer} 上，在那台电脑上打开这个网页来转写。` : "录音不在这台电脑上。"}</span></div>;
  }
  if (!speech?.ready) return <div className="mt-empty"><b>还没有文字稿</b><span>转写要先下载模型。</span><div className="mt-empty-row"><SpeechRow /></div></div>;
  return (
    <div className="mt-empty">
      <b>还没有文字稿</b>
      <span>在这台电脑上转，用时大约是录音的十分之一。</span>
      <button type="button" className="btn btn-primary" onClick={() => void transcribe(id)}>转成文字</button>
    </div>
  );
}

const RATES = [1, 1.25, 1.5, 2];

function PlayerBar({ player, duration, follow, setFollow }: { player: Player; duration: number; follow: boolean; setFollow: (v: boolean) => void }) {
  return (
    <div className="mt-player">
      <button type="button" className="icon-btn" aria-label="后退 15 秒" title="后退 15 秒" onClick={() => player.seek(player.time - 15)}><Rewind className="ico" /></button>
      <button type="button" className="mt-play" aria-label={player.playing ? "暂停" : "播放"} title="播放 / 暂停（空格）" onClick={() => (player.playing ? player.pause() : player.play())}>
        {player.playing ? <Pause className="ico is-fill" /> : <Play className="ico is-fill" />}
      </button>
      <button type="button" className="icon-btn" aria-label="前进 15 秒" title="前进 15 秒" onClick={() => player.seek(player.time + 15)}><FastForward className="ico" /></button>
      <span className="mt-ptime">{clock(player.time)}<em> / {clock(duration)}</em></span>
      <span className="mt-gap" />
      <div className="seg mt-rates" role="radiogroup" aria-label="播放速度">
        {RATES.map((r) => <button key={r} type="button" role="radio" aria-checked={player.rate === r} className={player.rate === r ? "is-on" : ""} onClick={() => player.setRate(r)}>{r}×</button>)}
      </div>
      <button type="button" className={`tool${player.skip ? " is-on" : ""}`} aria-pressed={!!player.skip} onClick={() => player.setSkip(!player.skip)} title="跳过没人说话的地方">
        <ChevronsRight className="ico" />跳过静音
      </button>
      <button type="button" className={`tool${follow ? " is-on" : ""}`} aria-pressed={follow} onClick={() => setFollow(!follow)} title="播放时文字跟着走">
        <TextCursor className="ico" />跟随
      </button>
    </div>
  );
}

void mount(kit, App);
