// What runs in the background of the page: the recording (one at a time),
// transcribing (one meeting at a time, its tracks one after another), and
// installing the speech engine. Each keeps its state here, outside React,
// so moving between meetings does not stop it; components follow it with
// useEngine().
import { useSyncExternalStore } from "react";
import type { OSAudioLevel, OSJob, OSRecording, OSSegment, OSSpeechStatus } from "../_lib/myco";
import { os, toOSError } from "../_lib/os";
import { myco } from "../_lib/myco";
import { alike, kit, meetings, num, segments, segmentsOf, type Meeting, type Track } from "./model";

type Listener = () => void;
const subs = new Set<Listener>();
let version = 0;
const changed = () => { version++; for (const f of subs) f(); };
export function useEngine() {
  return useSyncExternalStore((f) => { subs.add(f); return () => { subs.delete(f); }; }, () => version);
}

// ---- recording ----

export type Recording = {
  meeting: string; job: OSJob<OSRecording>; started: number; seconds: number; paused: boolean; stopping: boolean;
  /** Recent levels per track (peaks, every 100 ms), newest last. */
  levels: Record<string, number[]>;
  /** Seconds since each track last had sound. */
  quiet: Record<string, number>;
};
export let recording: Recording | null = null;

const patchMeeting = (id: string, p: Partial<Meeting>) => {
  const cur = meetings.get(id);
  if (!cur) return Promise.resolve(undefined);
  return kit.track(meetings.put({ ...cur.data, ...p }, id));
};

function title(d: Date) {
  return `会议 ${d.getMonth() + 1}月${d.getDate()}日 ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

export type RecordPlan = { me: boolean; them: string | null; themName: string };

/** Starts recording a new meeting; its id. Throws what the computer said (no permission…). */
export async function startRecording(plan: RecordPlan, computer: string) {
  if (recording) throw new Error("已经在录了");
  const tracks: { id: Track; source: string }[] = [];
  if (plan.them) tracks.push({ id: "them", source: plan.them });
  if (plan.me) tracks.push({ id: "me", source: "mic" });
  if (!tracks.length) throw new Error("至少录一个声音");
  const now = new Date();
  const data: Meeting = {
    title: title(now), started_at: now.toISOString(), seconds: 0, status: "recording", computer,
    sources: Object.fromEntries(tracks.map((t) => [t.id, t.source])), source_name: plan.them ? plan.themName : "",
    speakers: { me: kit.me || "我", them: "对方" }, by: kit.me,
  };
  const rec = await meetings.put(data as Meeting & Record<string, unknown>);
  if (!rec) throw new Error("没存上");
  const id = rec.id;
  kit.emit();
  const job = os().audio.record({ tracks });
  const r: Recording = { meeting: id, job, started: Date.now(), seconds: 0, paused: false, stopping: false, levels: {}, quiet: {} };
  for (const t of tracks) { r.levels[t.id] = []; r.quiet[t.id] = 0; }
  recording = r;
  changed();
  job.on("started", (d: { dir: string; files: Record<string, string> }) => { void patchMeeting(id, { dir: d.dir, files: d.files }); });
  job.on("level", (d: OSAudioLevel) => {
    r.seconds = d.seconds;
    for (const [k, v] of Object.entries(d.tracks)) {
      const l = (r.levels[k] ||= []);
      l.push(v.peak);
      if (l.length > 600) l.splice(0, l.length - 600);
      r.quiet[k] = v.peak > 0.004 ? 0 : (r.quiet[k] || 0) + 0.1;
    }
    changed();
  });
  job.on("paused", () => { r.paused = true; changed(); });
  job.on("resumed", () => { r.paused = false; changed(); });
  job.done.then(async (v) => {
    recording = null;
    changed();
    const files: Partial<Record<Track, string>> = {};
    for (const t of v.tracks) if (!t.error) files[t.id as Track] = t.path;
    await patchMeeting(id, { status: "recorded", seconds: v.seconds, dir: v.dir, files });
    if (v.seconds >= 1) void autoTranscribe(id);
  }, async (e) => {
    recording = null;
    changed();
    const err = toOSError(e);
    // nothing was recorded: no meeting
    if (!meetings.get(id)?.data.files) { await meetings.remove(id).catch(() => undefined); kit.emit(); }
    else await patchMeeting(id, { status: "recorded" });
    lastRecordError = err.message;
    changed();
  });
  return id;
}
export let lastRecordError = "";
export const clearRecordError = () => { lastRecordError = ""; changed(); };

export function pauseRecording(on: boolean) { if (recording) void recording.job.write(on ? "pause" : "resume"); }
export function stopRecording() {
  if (!recording || recording.stopping) return;
  recording.stopping = true;
  changed();
  void recording.job.write("stop");
}

// ---- the speech engine ----

export let speech: OSSpeechStatus | null = null;
export let installing: { step: string; received: number; total: number; unpacking?: boolean } | null = null;
export let installError = "";

export async function checkSpeech() {
  try { speech = await os().speech.status(); } catch { speech = null; }
  changed();
  return speech;
}

export async function installSpeech() {
  if (installing) return;
  installing = { step: "engine", received: 0, total: 0 };
  installError = "";
  changed();
  const job = os().speech.install({});
  job.on("progress", (p: { step: string; received?: number; total?: number; unpacking?: boolean }) => {
    installing = { step: p.step, received: num(p.received), total: num(p.total), unpacking: !!p.unpacking };
    changed();
  });
  try { speech = await job.done; } catch (e) { installError = toOSError(e).message; }
  installing = null;
  changed();
}

// ---- transcribing ----

export type Transcribing = { meeting: string; track: Track; done: number; total: number; tracks: Track[]; index: number; voices?: boolean };
export let transcribing: Transcribing | null = null;
let transcribeJob: OSJob<unknown> | null = null;
const waiting: string[] = [];

async function autoTranscribe(id: string) {
  const st = speech || (await checkSpeech());
  if (st?.ready) void transcribe(id);
}

/** Transcribes a meeting again from its recording (the sentences there are replaced). */
export async function transcribe(id: string) {
  if (transcribing) { if (transcribing.meeting !== id && !waiting.includes(id)) waiting.push(id); changed(); return; }
  const m = meetings.get(id)?.data;
  if (!m?.files) return;
  const tracks = (["them", "me"] as Track[]).filter((t) => m.files?.[t]);
  if (!tracks.length) return;
  transcribing = { meeting: id, track: tracks[0], done: 0, total: num(m.seconds) * tracks.length, tracks, index: 0 };
  changed();
  await patchMeeting(id, { status: "transcribing", error: "" });
  // the old sentences go first
  for (const s of segmentsOf(id)) await segments.remove(s.id).catch(() => undefined);
  kit.emit();
  let failed = "";
  let model = "";
  const theirs: { start: number; end: number; text: string }[] = [];
  // (one write after another: hundreds of sentences, the hub in order)
  let line: Promise<unknown> = Promise.resolve();
  const add = (track: Track, s: OSSegment) => {
    if (track === "me") {
      // what the microphone heard of the speakers (no headphones): already in "them"
      const echo = theirs.some((t) => t.start < s.end + 1 && t.end > s.start - 1 && alike(t.text, s.text) > 0.5);
      if (echo) return;
    } else theirs.push(s);
    const data = { meeting: id, track, start: s.start, end: s.end, text: s.text, edited: false };
    line = line.then(() => kit.track(segments.put(data))).then(() => kit.emit());
  };
  for (let i = 0; i < tracks.length; i++) {
    const track = tracks[i];
    transcribing = { ...transcribing!, track, index: i, done: num(m.seconds) * i };
    changed();
    const job = os().speech.transcribe({ path: m.files![track]! });
    transcribeJob = job as OSJob<unknown>;
    job.on("segment", (s: OSSegment) => add(track, s));
    job.on("progress", (p: { done: number; total: number }) => {
      if (transcribing) { transcribing = { ...transcribing, done: num(m.seconds) * i + num(p.done) }; changed(); }
    });
    try { const v = await job.done; model = v.model; } catch (e) {
      const err = toOSError(e);
      failed = err.code === "cancelled" ? "已停止" : err.message;
      break;
    }
  }
  await line;
  // then who of the others said what
  if (!failed && m.files.them && speech?.diarize) {
    transcribing = { ...transcribing!, voices: true, done: transcribing!.total };
    changed();
    const err = await diarizeNow(id, 0);
    if (err && err !== "已停止") kit.flash(`没分出说话人：${err}`);
  }
  transcribeJob = null;
  transcribing = null;
  changed();
  await patchMeeting(id, failed ? { status: "failed", error: failed } : { status: "done", model, error: "" });
  const next = waiting.shift();
  if (next) void transcribe(next);
}

export function stopTranscribing() { void transcribeJob?.cancel(); waiting.length = 0; }
export const isWaiting = (id: string) => waiting.includes(id);

// ---- telling the others apart ----

/** Splits "them" into its voices (count: how many, 0 to find out); "" when done, or why not. */
async function diarizeNow(id: string, count: number): Promise<string> {
  const m = meetings.get(id)?.data;
  const path = m?.files?.them;
  if (!m || !path) return "没有别人的录音";
  const job = os().speech.diarize({ path, speakers: count || undefined });
  transcribeJob = job as OSJob<unknown>;
  let v;
  try { v = await job.done; } catch (e) { const err = toOSError(e); return err.code === "cancelled" ? "已停止" : err.message; }
  // each sentence goes to the voice it overlaps most
  const turns = v.turns;
  const many = v.speakers > 1;
  let line: Promise<unknown> = Promise.resolve();
  for (const s of segmentsOf(id)) {
    if (s.data.track !== "them") continue;
    let best = -1, most = 0;
    for (const t of turns) {
      const o = Math.min(num(s.data.end), t.end) - Math.max(num(s.data.start), t.start);
      if (o > most) { most = o; best = t.speaker; }
    }
    if (best < 0) {
      // (no turn overlaps: the nearest one)
      let gap = Infinity;
      for (const t of turns) { const g = Math.min(Math.abs(t.start - num(s.data.end)), Math.abs(num(s.data.start) - t.end)); if (g < gap) { gap = g; best = t.speaker; } }
    }
    const speaker = many && best >= 0 ? `them:${best}` : undefined;
    if ((s.data.speaker || undefined) === speaker) continue;
    const data = { ...s.data, speaker };
    if (!speaker) delete (data as { speaker?: string }).speaker;
    line = line.then(() => kit.track(segments.put(data, s.id))).then(() => kit.emit());
  }
  await line;
  await patchMeeting(id, { voices: many ? v.speakers : 1 });
  return "";
}

export let splitting: string | null = null;
/** Tells the others apart again (count: how many, 0 to find out). */
export async function splitVoices(id: string, count: number) {
  if (transcribing || splitting) return;
  splitting = id;
  changed();
  const err = await diarizeNow(id, count);
  transcribeJob = null;
  splitting = null;
  changed();
  if (err) kit.flash(err === "已停止" ? "已停止" : `没分出说话人：${err}`);
  else kit.flash(`已分成 ${meetings.get(id)?.data.voices || 1} 个人`);
}

// ---- notes, by an agent ----

export const canAsk = () => !!(myco as unknown as { send?: unknown } | undefined)?.send;

/** Asks the page's agent to write the notes of a meeting (as agent.md says). */
export async function askForNotes(id: string) {
  const m = meetings.get(id)?.data;
  if (!m) return;
  const text = [
    `请给「会议记录」网页里的一场会议写纪要。`,
    `网页 ${kit.page}，会议 ${id}「${m.title || "会议"}」（${m.started_at || ""}，${Math.round(num(m.seconds) / 60)} 分钟）。`,
    `按这个网页的 agent.md：读 segments 里 meeting 是 ${id} 的句子，写好后放进这场会议记录的 notes（summary、decisions、todos、chapters，chapters 带开始的秒数）。`,
    `内容多、有方案或流程时，再建一份文档和一张白板，把它们的链接写进 notes.doc、notes.board。写完回我一句。`,
  ].join("\n");
  const sender = myco as unknown as { send: (t: string) => Promise<{ id: string }> };
  await sender.send(text);
  const cur = meetings.get(id)?.data;
  if (cur) await patchMeeting(id, { notes: { ...(cur.notes || {}), asked: new Date().toISOString(), asked_by: kit.me } });
}
