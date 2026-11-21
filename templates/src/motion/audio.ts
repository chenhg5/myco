// The video's sound: music and voice files stay on this computer (this
// page's IndexedDB, by a key made from the file's bytes, as 视频剪辑 keeps
// its videos); the hub keeps only where each track plays. Here: keeping
// and finding the files, their waveform and beats, playing them with the
// preview, and mixing them for the mp4.
import { ALL_FORMATS, AudioBufferSink, BlobSource, Input } from "mediabunny";
import { clamp, num, tracks, type Track } from "./model";

export type Local = { key: string; file: Blob; name: string; url: string; buffer: AudioBuffer | null; peaks: Float32Array | null; peakMax: number };
export const PEAKS_PER_SEC = 50;

const local = new Map<string, Local>();
const listeners = new Set<() => void>();
export const onLocal = (f: () => void) => { listeners.add(f); return () => { listeners.delete(f); }; };
const changed = () => { for (const f of listeners) f(); };
export const localFor = (key: unknown) => (typeof key === "string" && key ? local.get(key) || null : null);

// ---- IndexedDB: key → the file ----
let dbp: Promise<IDBDatabase | null> | null = null;
function db() {
  if (!dbp) dbp = new Promise((resolve) => {
    try {
      const r = indexedDB.open("myco-motion", 1);
      r.onupgradeneeded = () => { r.result.createObjectStore("files"); };
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => resolve(null);
    } catch { resolve(null); }
  });
  return dbp;
}
async function idb<T>(mode: IDBTransactionMode, f: (s: IDBObjectStore) => IDBRequest): Promise<T | null> {
  const d = await db();
  if (!d) return null;
  return new Promise((resolve) => {
    try {
      const req = f(d.transaction("files", mode).objectStore("files"));
      req.onsuccess = () => resolve(req.result as T);
      req.onerror = () => resolve(null);
    } catch { resolve(null); }
  });
}

/** A file's key: its size and a hash of its first and last 256 KB. */
export async function keyOf(file: Blob) {
  const n = 256 * 1024;
  const parts = [file.slice(0, n), file.size > n ? file.slice(Math.max(n, file.size - n)) : new Blob()];
  let h1 = 0x811c9dc5, h2 = 0x01000193 ^ file.size;
  for (const p of parts) {
    const b = new Uint8Array(await p.arrayBuffer());
    for (let i = 0; i < b.length; i++) { h1 = Math.imul(h1 ^ b[i], 16777619); h2 = Math.imul(h2 ^ b[i], 2246822519) ^ (h2 >>> 13); }
  }
  return `${file.size.toString(36)}-${(h1 >>> 0).toString(36)}${(h2 >>> 0).toString(36)}`;
}

let actx: AudioContext | null = null;
const audioContext = () => (actx ||= new AudioContext());

/** Decodes a file's sound (the browser's own decoder: mp3, m4a/aac, wav…, and the sound of a video). */
export async function decode(file: Blob): Promise<AudioBuffer> {
  try {
    const ctx = new OfflineAudioContext(2, 1, 48000);
    return await ctx.decodeAudioData(await file.arrayBuffer());
  } catch (e) {
    // (what the browser's decoder cannot read: mediabunny's, piece by piece)
    const input = new Input({ source: new BlobSource(file), formats: ALL_FORMATS });
    try {
      const at = await input.getPrimaryAudioTrack();
      if (!at || !(await at.canDecode())) throw e;
      const parts: { t: number; b: AudioBuffer }[] = [];
      for await (const w of new AudioBufferSink(at).buffers()) parts.push({ t: w.timestamp, b: w.buffer });
      if (!parts.length) throw e;
      const rate = parts[0].b.sampleRate;
      const end = Math.max(...parts.map((p) => p.t + p.b.duration));
      const out = new AudioBuffer({ length: Math.max(1, Math.ceil(end * rate)), numberOfChannels: 2, sampleRate: rate });
      for (const p of parts) {
        const off = Math.max(0, Math.round(p.t * rate));
        for (let c = 0; c < 2; c++) {
          const src = p.b.getChannelData(Math.min(c, p.b.numberOfChannels - 1));
          out.getChannelData(c).set(src.subarray(0, Math.max(0, Math.min(src.length, out.length - off))), off);
        }
      }
      return out;
    } finally { input.dispose(); }
  }
}

function peaksOf(b: AudioBuffer) {
  const per = PEAKS_PER_SEC, ch = [b.getChannelData(0), b.getChannelData(Math.min(1, b.numberOfChannels - 1))];
  const peaks = new Float32Array(Math.ceil(b.duration * per) + 1);
  const step = b.sampleRate / per;
  for (let k = 0; k < peaks.length; k++) {
    let m = 0;
    const a = Math.floor(k * step), z = Math.min(ch[0].length, Math.floor((k + 1) * step));
    for (let i = a; i < z; i += 8) { const v = Math.abs(ch[0][i]) + Math.abs(ch[1][i]); if (v > m) m = v; }
    peaks[k] = m / 2;
  }
  let max = 0.05;
  for (const v of peaks) if (v > max) max = v;
  return { peaks, max };
}

async function register(key: string, file: Blob, name: string) {
  let l = local.get(key);
  if (l) return l;
  l = { key, file, name, url: URL.createObjectURL(file), buffer: null, peaks: null, peakMax: 1 };
  local.set(key, l);
  changed();
  try {
    l.buffer = await decode(file);
    const p = peaksOf(l.buffer);
    l.peaks = p.peaks; l.peakMax = p.max;
  } catch { /* (it plays without a waveform) */ }
  changed();
  return l;
}

/** Keeps a file on this computer: its key, length, and the decoded sound. */
export async function keep(file: File) {
  const key = await keyOf(file);
  await idb("readwrite", (s) => s.put({ file, name: file.name, at: Date.now() }, key));
  const l = await register(key, file, file.name);
  if (!l.buffer) throw new Error("decode");
  return { key, duration: l.buffer.duration, local: l };
}

/** Finds the files of these keys that this computer kept. */
export async function restore(keys: string[]) {
  for (const k of keys) {
    if (!k || local.has(k)) continue;
    const v = await idb<{ file: Blob; name: string }>("readonly", (s) => s.get(k));
    if (v?.file) await register(k, v.file, v.name);
  }
}

// ---- beats ----

/** The beats of music (seconds of the file) and its tempo: an onset
 *  envelope (rises in loudness, low and high bands), its tempo by
 *  autocorrelation (60–180 BPM, leaning to 120), then the grid placed
 *  where the onsets are strongest and each beat moved to its onset. */
export function findBeats(b: AudioBuffer): { beats: number[]; bpm: number } {
  const rate = b.sampleRate, hop = Math.round(rate / 100); // 10 ms
  const a = b.getChannelData(0), c = b.getChannelData(Math.min(1, b.numberOfChannels - 1));
  const n = Math.floor(a.length / hop);
  if (n < 300) return { beats: [], bpm: 0 };
  // two bands: a one-pole low pass (kick, bass) and what is left (snare, hats)
  const low = new Float32Array(n), high = new Float32Array(n);
  let lp = 0;
  const k = Math.exp((-2 * Math.PI * 150) / rate);
  for (let f = 0; f < n; f++) {
    let el = 0, eh = 0;
    for (let i = f * hop, z = i + hop; i < z; i++) {
      const x = (a[i] + c[i]) * 0.5;
      lp = lp * k + x * (1 - k);
      el += lp * lp; eh += (x - lp) * (x - lp);
    }
    low[f] = Math.log1p(el * 1000); high[f] = Math.log1p(eh * 1000);
  }
  const env = new Float32Array(n);
  for (let f = 1; f < n; f++) env[f] = Math.max(0, low[f] - low[f - 1]) * 1.5 + Math.max(0, high[f] - high[f - 1]);
  // less what is around (a moving mean), so only rises stand out
  const W = 25, mean = new Float32Array(n);
  let sum = 0;
  for (let f = 0; f < n; f++) { sum += env[f]; if (f >= 2 * W) sum -= env[f - 2 * W]; mean[Math.max(0, f - W)] = sum / Math.min(f + 1, 2 * W); }
  for (let f = 0; f < n; f++) env[f] = Math.max(0, env[f] - mean[f]);
  // tempo
  let best = 0, bestLag = 50;
  for (let lag = 33; lag <= 100; lag++) { // 180 … 60 BPM
    let s = 0;
    for (let f = lag; f < n; f++) s += env[f] * env[f - lag];
    s /= n - lag;
    const bpm = 6000 / lag;
    const w = Math.exp(-0.5 * Math.pow(Math.log2(bpm / 120) / 0.9, 2));
    if (s * w > best) { best = s * w; bestLag = lag; }
  }
  // a finer period: the lag's neighbours, weighted
  let p = bestLag;
  {
    const sc = (lag: number) => { let s = 0; for (let f = lag; f < n; f++) s += env[f] * env[f - lag]; return s / (n - lag); };
    const y0 = sc(bestLag - 1), y1 = sc(bestLag), y2 = sc(bestLag + 1);
    const d = y0 - 2 * y1 + y2;
    if (d < 0) p = bestLag + clamp((0.5 * (y0 - y2)) / d, -0.5, 0.5);
  }
  // phase
  let phase = 0, phaseScore = -1;
  for (let o = 0; o < p; o++) {
    let s = 0;
    for (let t = o; t < n; t += p) s += env[Math.round(t)] || 0;
    if (s > phaseScore) { phaseScore = s; phase = o; }
  }
  const beats: number[] = [];
  const tol = Math.max(2, Math.round(p * 0.12));
  for (let t = phase; t < n; t += p) {
    const c0 = Math.round(t);
    let at = c0, m = env[c0] || 0;
    for (let d = -tol; d <= tol; d++) { const v = env[c0 + d] || 0; if (v > m * 1.3) { m = v; at = c0 + d; } }
    beats.push(Math.round(at * hop / rate * 1000) / 1000);
  }
  return { beats, bpm: Math.round((6000 / p) * 10) / 10 };
}

// ---- where tracks play ----

export type PlacedTrack = { id: string; kind: "music" | "voice"; name: string; key: string; start: number; in: number; out: number; end: number; volume: number; fadeIn: number; fadeOut: number; beats: number[] };

export function trackList(): PlacedTrack[] {
  return tracks.rows.map((r) => {
    const d = r.data as Track;
    const full = num(d.duration, 0);
    const a = clamp(num(d.in, 0), 0, Math.max(0, full - 0.1));
    const b = clamp(num(d.out, full), a + 0.1, full || a + 0.1);
    const start = Math.max(0, num(d.start, 0));
    return {
      id: r.id, kind: d.kind === "voice" ? "voice" : "music", name: String(d.name || ""), key: String(d.key || ""),
      start, in: a, out: b, end: start + (b - a), volume: clamp(num(d.volume, d.kind === "voice" ? 1 : 0.8), 0, 2),
      fadeIn: clamp(num(d.fade_in, 0), 0, 10), fadeOut: clamp(num(d.fade_out, d.kind === "voice" ? 0 : 1.5), 0, 10),
      beats: Array.isArray(d.beats) ? (d.beats as number[]).filter((x) => typeof x === "number") : [],
    };
  });
}

/** The music's beats on the video's time (from the music tracks). */
export function beatTimes(T: number) {
  const out: number[] = [];
  for (const t of trackList()) {
    if (t.kind !== "music") continue;
    for (const b of t.beats) if (b >= t.in && b <= t.out) { const at = t.start + b - t.in; if (at > 0 && at < T + 30) out.push(at); }
  }
  return out.sort((x, y) => x - y);
}

/** The gain of a track at video time t (volume and fades). */
function gainAt(t: PlacedTrack, time: number) {
  let g = t.volume;
  if (t.fadeIn > 0 && time < t.start + t.fadeIn) g *= clamp((time - t.start) / t.fadeIn, 0, 1);
  if (t.fadeOut > 0 && time > t.end - t.fadeOut) g *= clamp((t.end - time) / t.fadeOut, 0, 1);
  return g;
}

// ---- playing with the preview ----
// One <audio> for each track, moved to the player's time when it starts,
// seeks, or drifts; volume follows the fades.

const els = new Map<string, HTMLAudioElement>();
let playingNow = false;

function elFor(t: PlacedTrack) {
  const l = localFor(t.key);
  if (!l) return null;
  let el = els.get(t.id);
  if (!el || el.dataset.key !== t.key) {
    el?.pause();
    el = new Audio(l.url);
    el.preload = "auto";
    el.dataset.key = t.key;
    els.set(t.id, el);
  }
  return el;
}

/** Keeps the sound with the player: call it on play, pause, seek and each frame. */
export function syncSound(time: number, playing: boolean, total: number, seeked = false) {
  const list = trackList();
  const ids = new Set(list.map((t) => t.id));
  for (const [id, el] of els) if (!ids.has(id)) { el.pause(); els.delete(id); }
  for (const t of list) {
    const el = elFor(t);
    if (!el) continue;
    const inside = time >= t.start && time < Math.min(t.end, total);
    if (!playing || !inside) { if (!el.paused) el.pause(); continue; }
    const want = t.in + (time - t.start);
    if (el.paused || seeked || !playingNow || Math.abs(el.currentTime - want) > 0.15) el.currentTime = want;
    el.volume = clamp(gainAt(t, time), 0, 1);
    if (el.paused) void el.play().catch(() => undefined);
  }
  playingNow = playing;
}
export function stopSound() { for (const el of els.values()) el.pause(); playingNow = false; }
export function hasSound() { return trackList().some((t) => !!localFor(t.key)); }
export function resumeAudio() { try { void audioContext().resume(); } catch { /* (no audio here) */ } }

// ---- the mix, for the mp4 ----

export const MIX_RATE = 48000;

/** Every track mixed at 48 kHz stereo over the video's length; null when
 *  there is nothing to hear. Tracks whose file is not on this computer are
 *  named in `missing`. */
export async function mix(T: number): Promise<{ buffer: AudioBuffer | null; missing: string[] }> {
  const list = trackList().filter((t) => t.start < T && t.volume > 0);
  const missing: string[] = [];
  if (!list.length || T <= 0) return { buffer: null, missing };
  const ctx = new OfflineAudioContext(2, Math.max(1, Math.ceil(T * MIX_RATE)), MIX_RATE);
  let any = false;
  for (const t of list) {
    const l = localFor(t.key);
    if (!l) { missing.push(t.name || t.key); continue; }
    if (!l.buffer) { try { l.buffer = await decode(l.file); } catch { missing.push(t.name); continue; } }
    const src = ctx.createBufferSource();
    src.buffer = l.buffer;
    const g = ctx.createGain();
    const end = Math.min(t.end, T);
    g.gain.setValueAtTime(t.fadeIn > 0 ? 0 : t.volume, t.start);
    if (t.fadeIn > 0) g.gain.linearRampToValueAtTime(t.volume, Math.min(end, t.start + t.fadeIn));
    if (t.fadeOut > 0 && t.end - t.fadeOut < end) {
      const from = Math.max(t.start + t.fadeIn, t.end - t.fadeOut);
      g.gain.setValueAtTime(t.volume, from);
      g.gain.linearRampToValueAtTime(gainAt(t, end), end);
    }
    src.connect(g).connect(ctx.destination);
    src.start(t.start, t.in, end - t.start);
    any = true;
  }
  return { buffer: any ? await ctx.startRendering() : null, missing };
}

/** Pieces of a buffer, `secs` long each, for the encoder. */
export function* slices(b: AudioBuffer, secs = 1) {
  const n = Math.round(secs * b.sampleRate);
  for (let a = 0; a < b.length; a += n) {
    const len = Math.min(n, b.length - a);
    const out = new AudioBuffer({ length: len, numberOfChannels: b.numberOfChannels, sampleRate: b.sampleRate });
    for (let c = 0; c < b.numberOfChannels; c++) out.getChannelData(c).set(b.getChannelData(c).subarray(a, a + len));
    yield out;
  }
}

// ---- speech: a 16 kHz mono WAV for the computer's transcriber ----

/** A part of a sound as a 16 kHz mono 16-bit WAV. */
export function wav16k(b: AudioBuffer, from = 0, to = b.duration): Uint8Array {
  const rate = 16000, k = b.sampleRate / rate;
  const n = Math.max(0, Math.floor((to - from) * rate));
  const a = b.getChannelData(0), c = b.getChannelData(Math.min(1, b.numberOfChannels - 1));
  const out = new Uint8Array(44 + n * 2);
  const v = new DataView(out.buffer);
  const str = (o: number, s: string) => { for (let i = 0; i < s.length; i++) out[o + i] = s.charCodeAt(i); };
  str(0, "RIFF"); v.setUint32(4, 36 + n * 2, true); str(8, "WAVE"); str(12, "fmt ");
  v.setUint32(16, 16, true); v.setUint16(20, 1, true); v.setUint16(22, 1, true); v.setUint32(24, rate, true);
  v.setUint32(28, rate * 2, true); v.setUint16(32, 2, true); v.setUint16(34, 16, true); str(36, "data"); v.setUint32(40, n * 2, true);
  const off = Math.floor(from * b.sampleRate);
  for (let i = 0; i < n; i++) {
    // (a little averaging over the step: enough of a low pass for speech)
    const p = off + Math.floor(i * k);
    let s = 0, m = 0;
    for (let j = 0; j < Math.max(1, Math.floor(k)) && p + j < a.length; j++) { s += a[p + j] + c[p + j]; m += 2; }
    const x = clamp(m ? s / m : 0, -1, 1);
    v.setInt16(44 + i * 2, x < 0 ? x * 0x8000 : x * 0x7fff, true);
  }
  return out;
}
