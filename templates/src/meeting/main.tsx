// 会议记录: record a meeting held in any app (Zoom, 腾讯会议, 飞书…) as two
// tracks — "me" (the microphone) and "them" (what the computer plays) —
// then transcribe it on this computer and read, play and fix the text.
// The recording stays on the computer that made it; the transcript is the
// page's records, for people and agents alike.
import "../_lib/sheet.css";
import "./meeting.css";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  AppWindow, AudioLines, Check, ChevronDown, Download, FileAudio, Loader2, Mic, MicOff, Monitor, Pause, Play, Plus, RefreshCw, Search, Square, Trash2, X,
} from "lucide-react";
import { Toast, mount, useKit } from "../_lib/appkit";
import { save } from "../_lib/myco";
import type { OSAudioSource } from "../_lib/myco";
import { os, osState, osStateText, useOS, formatBytes } from "../_lib/os";
import { Popover, rectOf } from "../_lib/ui";
import {
  TRACK_COLOR, clock, kit, length, meetingList, meetingRank, meetings, num, segments, segmentsOf, speakerName, when,
  type Meeting, type Segment, type Track,
} from "./model";
import {
  checkSpeech, clearRecordError, installError, installSpeech, installing, isWaiting, lastRecordError, pauseRecording, recording,
  speech, startRecording, stopRecording, stopTranscribing, transcribe, transcribing, useEngine,
} from "./engine";
import { Player } from "./player";

const canUse = osState === "ready";

function App() {
  useKit(kit);
  useEngine();
  const sys = useOS();
  const [sel, setSel] = useState<string>(() => (recording ? recording.meeting : meetingList()[0]?.id || "new"));
  useEffect(() => { if (canUse) void checkSpeech(); }, []);
  // a recording that ended without the page (closed while it ran): it is what it recorded
  useEffect(() => {
    for (const r of meetings.rows) {
      if (r.data.status === "recording" && recording?.meeting !== r.id && r.data.computer === sys.info?.hostname) {
        void kit.track(meetings.put({ ...r.data, status: "recorded" }, r.id));
      }
    }
  }, [sys.info]); // eslint-disable-line react-hooks/exhaustive-deps
  const list = meetingList();
  const cur = sel !== "new" ? meetings.get(sel) : null;
  const showNew = sel === "new" || !cur;
  return (
    <div className="app mt-app">
      <aside className="mt-side">
        <div className="mt-side-head">
          <h1>会议记录</h1>
          <button type="button" className={`btn btn-primary mt-new${showNew ? " is-on" : ""}`} onClick={() => setSel("new")} disabled={!!recording && sel === recording.meeting}>
            {recording ? <><i className="mt-dot" />录音中</> : <><Plus className="ico" />新录音</>}
          </button>
        </div>
        <div className="mt-list" role="listbox" aria-label="会议">
          {list.map((r) => {
            const m = r.data;
            const live = recording?.meeting === r.id;
            const tx = transcribing?.meeting === r.id;
            return (
              <button key={r.id} type="button" role="option" aria-selected={sel === r.id} className={`mt-item${sel === r.id ? " is-on" : ""}`} onClick={() => setSel(r.id)}>
                <b>{m.title || "会议"}</b>
                <span>
                  {when(m.started_at)}
                  {live ? <em className="is-rec">· 录音中 {clock(recording!.seconds)}</em>
                    : tx ? <em>· 转写中</em>
                    : m.status === "recorded" ? <em>· 没转写</em>
                    : m.status === "failed" ? <em className="is-err">· 转写没成</em>
                    : num(m.seconds) ? <> · {length(num(m.seconds))}</> : null}
                </span>
              </button>
            );
          })}
          {!list.length && <p className="mt-list-empty">录过的会议会在这里。</p>}
        </div>
      </aside>
      <main className="mt-main">
        {!canUse && <div className="mt-banner">{osStateText()} 录音和转写要在 myco App 里；这里能看和改文字。</div>}
        {showNew
          ? (recording ? <RecordingView id={recording.meeting} /> : <NewRecording computer={sys.info?.hostname || ""} onStarted={setSel} />)
          : recording?.meeting === cur!.id ? <RecordingView id={cur!.id} />
          : <MeetingView key={cur!.id} id={cur!.id} host={sys.info?.hostname || ""} onDeleted={() => setSel(meetingList()[0]?.id || "new")} />}
      </main>
      <Toast kit={kit} />
    </div>
  );
}

// ---- a new recording ----

function NewRecording({ computer, onStarted }: { computer: string; onStarted: (id: string) => void }) {
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
      // the meeting app making sound, if there is one
      const apps = l.sources.filter((s) => s.kind === "app" && s.playing && meetingRank(s) < 9);
      if (apps.length && them === "system") setThem(apps.sort((a, b) => meetingRank(a) - meetingRank(b))[0].id);
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
    <div className="mt-new-page">
      <div className="mt-new-in">
        <div className="mt-hero"><AudioLines className="mt-hero-ico" strokeWidth={1.5} /></div>
        <h2>录一场会议</h2>
        <p className="mt-sub">在 Zoom、腾讯会议、飞书里开会时，录下你说的（麦克风）和别人说的（电脑放出的声音），会后在这台电脑上转成文字。</p>
        {lastRecordError && <div className="mt-error"><b>没录上</b>{lastRecordError}<button type="button" className="icon-btn" aria-label="关掉" onClick={clearRecordError}><X className="ico" /></button></div>}
        <section className="mt-pick">
          <h3>录谁</h3>
          <label className={`mt-src${me ? " is-on" : ""}`}>
            <input type="checkbox" checked={me} onChange={(e) => setMe(e.target.checked)} disabled={!canUse} />
            <span className="mt-src-ico is-me">{me ? <Mic className="ico" /> : <MicOff className="ico" />}</span>
            <span className="mt-src-text"><b>我</b><span>麦克风</span></span>
            <i className="mt-check"><Check className="ico" /></i>
          </label>
          <h3>别人<button type="button" className="icon-btn" aria-label="刷新" title="刷新：会议开始后，会议软件才会出现在这里" onClick={() => void load()}><RefreshCw className="ico" /></button></h3>
          {why && <p className="mt-why">{why}</p>}
          <div className="mt-srcs" role="radiogroup">
            {system && <SourceCard id="system" icon={<Monitor className="ico" />} name="电脑的全部声音" sub="所有应用放出的声音" on={them === "system"} onPick={setThem} />}
            {apps.map((a) => (
              <SourceCard key={a.id} id={a.id} icon={<AppWindow className="ico" />} name={a.name} sub={a.playing ? "正在出声" : "只录这个应用"} playing={a.playing} on={them === a.id} onPick={setThem} />
            ))}
            <SourceCard id="none" icon={<X className="ico" />} name="不录" sub="只录我" on={them === "none"} onPick={setThem} />
          </div>
          {sources && !system && !why && <p className="mt-why">这台电脑录不了电脑放出的声音（macOS 14.2 起才行）。可以只录麦克风。</p>}
        </section>
        <SpeechCard />
        <button type="button" className="btn mt-go" disabled={!canUse || busy || (!me && them === "none")} onClick={() => void go()}>
          {busy ? <Loader2 className="ico spin" /> : <i className="mt-dot" />}开始录音
        </button>
        <p className="mt-fine">录别人之前，先告诉参会的人。录音只存在这台电脑上（~/.myco/recordings）。录音时可以收起网页面板，关掉网页会停止录音。</p>
      </div>
    </div>
  );
}

function SourceCard({ id, icon, name, sub, on, playing, onPick }: { id: string; icon: JSX.Element; name: string; sub: string; on: boolean; playing?: boolean; onPick: (id: string) => void }) {
  return (
    <button type="button" role="radio" aria-checked={on} className={`mt-src${on ? " is-on" : ""}`} onClick={() => onPick(id)} disabled={!canUse}>
      <span className="mt-src-ico">{icon}</span>
      <span className="mt-src-text"><b>{name}</b><span className={playing ? "is-live" : ""}>{playing && <i className="mt-live" />}{sub}</span></span>
      <i className="mt-check"><Check className="ico" /></i>
    </button>
  );
}

function SpeechCard() {
  useEngine();
  if (!canUse) return null;
  if (installing) {
    const pct = installing.total ? Math.round((installing.received / installing.total) * 100) : 0;
    const what = installing.step === "engine" ? "转写引擎" : installing.step === "vad" ? "断句模型" : "识别模型";
    return (
      <div className="mt-speech is-busy">
        <Loader2 className="ico spin" />
        <span>{installing.unpacking ? `正在解开${what}…` : `正在下载${what} ${installing.total ? `${formatBytes(installing.received)} / ${formatBytes(installing.total)}` : ""}`}</span>
        <i className="mt-bar"><i style={{ width: `${installing.unpacking ? 100 : pct}%` }} /></i>
      </div>
    );
  }
  if (!speech) return null;
  if (speech.ready) return <div className="mt-speech is-ok"><Check className="ico" />本机转写已就绪：SenseVoice，中英粤日韩，录完自动转成文字</div>;
  const size = speech.engine_size_mb + speech.models.filter((m) => !m.installed).reduce((a, m) => a + m.size_mb, 0);
  return (
    <div className="mt-speech">
      <span>转成文字要先下载转写引擎和模型（约 {size} MB，只下一次，在这台电脑上转，录音不上传）。{installError && <em className="is-err"> 上次没下成：{installError}</em>}</span>
      <button type="button" className="btn" onClick={() => void installSpeech()}><Download className="ico" />下载</button>
    </div>
  );
}

// ---- recording ----

function Meter({ name, color, peaks, quiet }: { name: string; color: string; peaks: number[]; quiet: number }) {
  const cv = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const c = cv.current;
    if (!c) return;
    const dpr = window.devicePixelRatio || 1;
    const w = c.clientWidth, h = c.clientHeight;
    if (c.width !== Math.round(w * dpr)) { c.width = Math.round(w * dpr); c.height = Math.round(h * dpr); }
    const ctx = c.getContext("2d")!;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    const bar = 3, gap = 2, n = Math.floor(w / (bar + gap));
    const list = peaks.slice(-n);
    ctx.fillStyle = color;
    list.forEach((p, i) => {
      // (louder looks louder: a square root, so speech fills the bar)
      const v = Math.max(0.04, Math.min(1, Math.sqrt(p) * 1.25));
      const bh = Math.max(2, v * h);
      const x = w - (list.length - i) * (bar + gap);
      ctx.globalAlpha = 0.35 + 0.65 * (i / Math.max(1, list.length));
      ctx.beginPath();
      ctx.roundRect(x, (h - bh) / 2, bar, bh, 1.5);
      ctx.fill();
    });
  });
  return (
    <div className="mt-meter">
      <span className="mt-meter-name"><i style={{ background: color }} />{name}{quiet > 20 && <em>一直没有声音</em>}</span>
      <canvas ref={cv} className="mt-wave" />
    </div>
  );
}

function RecordingView({ id }: { id: string }) {
  useEngine();
  const r = recording;
  const m = meetings.get(id)?.data;
  if (!r || !m) return null;
  const tracks = (["them", "me"] as Track[]).filter((t) => m.sources?.[t]);
  return (
    <div className="mt-rec">
      <div className="mt-rec-top">
        <span className={`mt-rec-state${r.paused ? " is-paused" : ""}`}><i className="mt-dot" />{r.stopping ? "正在结束…" : r.paused ? "已暂停" : "正在录音"}</span>
        <TitleInput id={id} m={m} />
        <span className="mt-rec-src">{m.source_name ? `别人：${m.source_name}` : "只录麦克风"}</span>
      </div>
      <div className="mt-rec-clock">{clock(r.seconds)}</div>
      <div className="mt-meters">
        {tracks.map((t) => <Meter key={t} name={speakerName(m, t)} color={TRACK_COLOR[t]} peaks={r.levels[t] || []} quiet={r.quiet[t] || 0} />)}
      </div>
      <div className="mt-rec-acts">
        <button type="button" className="btn mt-round" onClick={() => pauseRecording(!r.paused)} disabled={r.stopping}>
          {r.paused ? <><Play className="ico" />继续</> : <><Pause className="ico" />暂停</>}
        </button>
        <button type="button" className="btn mt-round is-stop" onClick={stopRecording} disabled={r.stopping}>
          {r.stopping ? <Loader2 className="ico spin" /> : <Square className="ico" />}结束
        </button>
      </div>
      <p className="mt-fine">结束后会在这台电脑上转成文字{speech?.ready ? "" : "（先在「新录音」里下载转写模型）"}。可以收起网页面板，关掉网页会停止录音。</p>
    </div>
  );
}

function TitleInput({ id, m }: { id: string; m: Meeting }) {
  const [v, setV] = useState<string | null>(null);
  const t = useRef(0);
  return (
    <input className="mt-title" value={v ?? m.title ?? ""} placeholder="会议" aria-label="会议名称" maxLength={120}
      onChange={(e) => {
        const val = e.target.value;
        setV(val);
        window.clearTimeout(t.current);
        t.current = window.setTimeout(() => {
          const cur = meetings.get(id);
          if (cur) void kit.track(meetings.put({ ...cur.data, title: val.trim() || "会议" }, id)).finally(() => setV(null));
        }, 500);
      }}
      onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} />
  );
}

// ---- a meeting ----

function MeetingView({ id, host, onDeleted }: { id: string; host: string; onDeleted: () => void }) {
  useEngine();
  const rec = meetings.get(id)!;
  const m = rec.data;
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
  const [query, setQuery] = useState("");
  const [follow, setFollow] = useState(true);
  const [menu, setMenu] = useState<{ x: number; y: number; w: number; h: number } | null>(null);
  const tx = transcribing?.meeting === id ? transcribing : null;
  const now = player?.time ?? -1;
  const active = player && (player.playing || now > 0) ? segs.findIndex((s) => num(s.data.start) <= now + 0.05 && num(s.data.end) >= now - 0.05) : -1;
  const list = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!follow || !player?.playing || active < 0) return;
    list.current?.querySelector(`[data-i="${active}"]`)?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, [active, follow, player?.playing]);
  const q = query.trim().toLowerCase();
  const hits = q ? segs.filter((s) => String(s.data.text || "").toLowerCase().includes(q)).length : 0;
  const talk = useMemo(() => {
    const out: Partial<Record<Track, number>> = {};
    for (const s of segs) out[(s.data.track || "them") as Track] = (out[(s.data.track || "them") as Track] || 0) + Math.max(0, num(s.data.end) - num(s.data.start));
    return out;
  }, [segs]);
  const totalTalk = Object.values(talk).reduce((a, b) => a + (b || 0), 0);

  const exportAs = async (kind: "txt" | "md" | "srt") => {
    setMenu(null);
    const name = (m.title || "会议").replace(/[\\/:*?"<>|]/g, "_");
    let body = "";
    if (kind === "srt") {
      const t = (s: number) => {
        const ms = Math.round(s * 1000), p = (n: number, w = 2) => String(n).padStart(w, "0");
        return `${p(Math.floor(ms / 3600000))}:${p(Math.floor(ms / 60000) % 60)}:${p(Math.floor(ms / 1000) % 60)},${p(ms % 1000, 3)}`;
      };
      body = segs.map((s, i) => `${i + 1}\n${t(num(s.data.start))} --> ${t(num(s.data.end))}\n${speakerName(m, s.data.track)}：${s.data.text}\n`).join("\n");
    } else {
      const head = kind === "md" ? `# ${m.title || "会议"}\n\n${when(m.started_at)} · ${length(num(m.seconds))}\n\n` : `${m.title || "会议"}\n${when(m.started_at)} · ${length(num(m.seconds))}\n\n`;
      body = head + segs.map((s) => kind === "md" ? `**${speakerName(m, s.data.track)}** \`${clock(num(s.data.start))}\` ${s.data.text}` : `[${clock(num(s.data.start))}] ${speakerName(m, s.data.track)}：${s.data.text}`).join("\n\n") + "\n";
    }
    const r = await save(`${name}.${kind}`, body, kind === "md" ? "text/markdown" : "text/plain").catch((e: Error) => { kit.flash(e.message); return null; });
    if (r) kit.flash(`已存到 ${r.path}`);
  };
  const remove = async () => {
    setMenu(null);
    if (!window.confirm(`删除「${m.title || "会议"}」和它的文字稿？${here ? "录音文件会移进废纸篓。" : ""}`)) return;
    stopTranscribing();
    for (const s of segs) await segments.remove(s.id).catch(() => undefined);
    await meetings.remove(id).catch(() => undefined);
    if (here && m.dir) await os().fs.trash({ path: m.dir }).catch(() => undefined);
    kit.emit();
    onDeleted();
  };

  return (
    <div className="mt-meeting">
      <header className="mt-head">
        <div className="mt-head-main">
          <TitleInput id={id} m={m} />
          <span className="mt-meta">
            {when(m.started_at)} · {length(num(m.seconds))}{m.source_name ? ` · ${m.source_name}` : ""}{m.computer && m.computer !== host ? ` · 录在 ${m.computer}` : ""}
          </span>
        </div>
        <div className="mt-search">
          <Search className="ico" />
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索文字稿" aria-label="搜索文字稿" />
          {q && <span>{hits} 处</span>}
        </div>
        <button type="button" className="icon-btn" aria-label="更多" onClick={(e) => setMenu(rectOf(e.currentTarget))}><ChevronDown className="ico" /></button>
        {menu && (
          <Popover at={menu} onClose={() => setMenu(null)} align="right">
            <div className="menu">
              <button type="button" className="menu-item" disabled={!segs.length} onClick={() => void exportAs("txt")}><Download className="ico" />导出文字稿（.txt）</button>
              <button type="button" className="menu-item" disabled={!segs.length} onClick={() => void exportAs("md")}><Download className="ico" />导出 Markdown</button>
              <button type="button" className="menu-item" disabled={!segs.length} onClick={() => void exportAs("srt")}><Download className="ico" />导出字幕（.srt）</button>
              {here && m.dir && <button type="button" className="menu-item" onClick={() => { setMenu(null); void os().fs.reveal({ path: m.dir! }); }}><FileAudio className="ico" />在访达里显示录音</button>}
              {here && <button type="button" className="menu-item" disabled={!!tx} onClick={() => { setMenu(null); if (!segs.length || window.confirm("重新转写会换掉现在的文字稿（包括改过的字）。")) void transcribe(id); }}><RefreshCw className="ico" />重新转写</button>}
              <div className="menu-sep" />
              <button type="button" className="menu-item is-danger" onClick={() => void remove()}><Trash2 className="ico" />删除这场会议</button>
            </div>
          </Popover>
        )}
      </header>
      {totalTalk > 0 && (
        <div className="mt-who">
          {(["them", "me"] as Track[]).filter((t) => talk[t]).map((t) => (
            <Speaker key={t} id={id} m={m} t={t} share={(talk[t] || 0) / totalTalk} />
          ))}
        </div>
      )}
      <div className="mt-body" ref={list}>
        {tx && <TranscribingBar />}
        {!tx && isWaiting(id) && <div className="mt-state"><Loader2 className="ico spin" />排队中：前一场转完就转这场</div>}
        {!tx && !segs.length && <EmptyTranscript id={id} m={m} here={here} />}
        {m.status === "failed" && !tx && <div className="mt-error"><b>转写没成</b>{m.error}{here && <button type="button" className="btn" onClick={() => void transcribe(id)}>再试一次</button>}</div>}
        <ol className="mt-lines">
          {segs.map((s, i) => {
            const prev = segs[i - 1]?.data;
            const newSpeaker = !prev || prev.track !== s.data.track || num(s.data.start) - num(prev.end) > 30;
            const match = q && String(s.data.text || "").toLowerCase().includes(q);
            if (q && !match) return null;
            return (
              <Line key={s.id} i={i} seg={s.id} s={s.data} m={m} head={newSpeaker || !!q} on={i === active} q={q}
                onSeek={player ? () => { player.seek(num(s.data.start)); player.play(); setFollow(true); } : undefined} />
            );
          })}
        </ol>
      </div>
      {player && <PlayerBar player={player} seconds={num(m.seconds)} follow={follow} setFollow={setFollow} />}
      {!player && m.files && !here && <div className="mt-away">{gone && m.computer === host ? "录音文件不在了（移走或删掉了），这里只能看文字。" : `录音在 ${m.computer || "另一台电脑"} 上，这里只能看文字。`}</div>}
    </div>
  );
}

function Speaker({ id, m, t, share }: { id: string; m: Meeting; t: Track; share: number }) {
  const [edit, setEdit] = useState<string | null>(null);
  const done = (v: string) => {
    setEdit(null);
    const cur = meetings.get(id);
    if (!cur || !v.trim() || v.trim() === speakerName(m, t)) return;
    void kit.track(meetings.put({ ...cur.data, speakers: { ...(cur.data.speakers || {}), [t]: v.trim() } }, id));
  };
  return (
    <span className="mt-speaker">
      <i style={{ background: TRACK_COLOR[t] }} />
      {edit !== null
        ? <input autoFocus onFocus={(e) => e.target.select()} value={edit} onChange={(e) => setEdit(e.target.value)} onBlur={() => done(edit)} onKeyDown={(e) => { if (e.key === "Enter") done(edit); if (e.key === "Escape") setEdit(null); }} aria-label="名字" maxLength={40} />
        : <button type="button" title="改名字" onClick={() => setEdit(speakerName(m, t))}>{speakerName(m, t)}</button>}
      <em>{Math.round(share * 100)}%</em>
    </span>
  );
}

function Line({ i, seg, s, m, head, on, q, onSeek }: { i: number; seg: string; s: Segment; m: Meeting; head: boolean; on: boolean; q: string; onSeek?: () => void }) {
  const [edit, setEdit] = useState<string | null>(null);
  const t = (s.track || "them") as Track;
  const save = () => {
    const v = (edit ?? "").trim();
    setEdit(null);
    const cur = segments.get(seg);
    if (!cur || v === (cur.data.text || "")) return;
    if (!v) { void kit.track(segments.remove(seg)); kit.emit(); return; }
    void kit.track(segments.put({ ...cur.data, text: v, edited: true }, seg));
    kit.emit();
  };
  const swap = () => {
    const cur = segments.get(seg);
    if (cur) { void kit.track(segments.put({ ...cur.data, track: t === "me" ? "them" : "me" }, seg)); kit.emit(); }
  };
  const text = String(s.text || "");
  return (
    <li className={`mt-line${on ? " is-on" : ""}${head ? " is-head" : ""}`} data-i={i}>
      {head && (
        <div className="mt-line-who">
          <i style={{ background: TRACK_COLOR[t] }} />
          <button type="button" className="mt-name" title="改成另一个人说的" onClick={swap}>{speakerName(m, t)}</button>
        </div>
      )}
      <div className="mt-line-row">
        <button type="button" className="mt-time" onClick={onSeek} disabled={!onSeek} title={onSeek ? "从这里播放" : undefined}>{clock(num(s.start))}</button>
        {edit !== null ? (
          <textarea className="mt-edit" autoFocus value={edit} rows={Math.max(1, Math.ceil(edit.length / 48))}
            onChange={(e) => setEdit(e.target.value)} onBlur={save}
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); (e.target as HTMLTextAreaElement).blur(); } if (e.key === "Escape") setEdit(null); }} />
        ) : (
          <p className="mt-text" onClick={() => setEdit(text)} title="点一下改字">
            {q ? highlight(text, q) : text}{s.edited && <em className="mt-edited">改过</em>}
          </p>
        )}
      </div>
    </li>
  );
}

function highlight(text: string, q: string) {
  const out: (string | JSX.Element)[] = [];
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

function TranscribingBar() {
  useEngine();
  const t = transcribing;
  if (!t) return null;
  const pct = t.total ? Math.min(100, Math.round((t.done / t.total) * 100)) : 0;
  return (
    <div className="mt-state is-busy">
      <Loader2 className="ico spin" />
      <span>正在转写{t.tracks.length > 1 ? `（${t.track === "me" ? "我" : "别人"}，${t.index + 1}/${t.tracks.length}）` : ""} {pct}%</span>
      <i className="mt-bar"><i style={{ width: `${pct}%` }} /></i>
      <button type="button" className="link-btn" onClick={stopTranscribing}>停止</button>
    </div>
  );
}

function EmptyTranscript({ id, m, here }: { id: string; m: Meeting; here: boolean }) {
  useEngine();
  if (m.status === "failed") return null;
  if (!here) return <div className="mt-state">还没有文字稿。{m.computer ? `录音在 ${m.computer} 上，在那台电脑上打开这个网页来转写。` : ""}</div>;
  if (!speech?.ready) return <div className="mt-empty"><p>还没有文字稿。转写要先下载模型：</p><SpeechCard /></div>;
  return (
    <div className="mt-empty">
      <p>还没有文字稿。</p>
      <button type="button" className="btn btn-primary" onClick={() => void transcribe(id)}><AudioLines className="ico" />转成文字</button>
    </div>
  );
}

const RATES = [1, 1.25, 1.5, 2];

function PlayerBar({ player, seconds, follow, setFollow }: { player: Player; seconds: number; follow: boolean; setFollow: (v: boolean) => void }) {
  const dur = player.duration || seconds;
  const bar = useRef<HTMLDivElement>(null);
  const seekAt = (x: number) => {
    const r = bar.current!.getBoundingClientRect();
    player.seek(Math.max(0, Math.min(1, (x - r.left) / r.width)) * dur);
  };
  return (
    <div className="mt-player">
      <button type="button" className="mt-play" aria-label={player.playing ? "暂停" : "播放"} onClick={() => (player.playing ? player.pause() : player.play())}>
        {player.playing ? <Pause className="ico" /> : <Play className="ico" />}
      </button>
      <span className="mt-ptime">{clock(player.time)} / {clock(dur)}</span>
      <div ref={bar} className="mt-scrub" role="slider" aria-label="进度" aria-valuemin={0} aria-valuemax={Math.round(dur)} aria-valuenow={Math.round(player.time)} tabIndex={0}
        onPointerDown={(e) => { (e.target as Element).setPointerCapture(e.pointerId); seekAt(e.clientX); }}
        onPointerMove={(e) => { if (e.buttons) seekAt(e.clientX); }}
        onKeyDown={(e) => { if (e.key === "ArrowRight") player.seek(player.time + 5); if (e.key === "ArrowLeft") player.seek(player.time - 5); }}>
        <i style={{ width: `${dur ? (player.time / dur) * 100 : 0}%` }} />
      </div>
      <button type="button" className="mt-rate" onClick={() => player.setRate(RATES[(RATES.indexOf(player.rate) + 1) % RATES.length])} title="播放速度">{player.rate}×</button>
      <label className="mt-follow" title="播放时文字跟着走"><input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} />跟随</label>
    </div>
  );
}

void mount(kit, App);
