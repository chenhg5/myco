// The videos themselves stay on this computer (the hub keeps only the
// edit): each file is kept in this page's IndexedDB under a key made from
// its size and some of its bytes, so the same file picked again, or on
// another computer, is known by the same key. From a file we take what the
// editor needs: its length and size, a poster for the hub, thumbnails for
// the timeline, and the loudness of its sound for the waveform.
import { ALL_FORMATS, AudioBufferSink, BlobSource, Input } from "mediabunny";

export type Local = {
  key: string; file: Blob; name: string; url: string;
  thumbs: { t: number; url: string }[]; thumbsDone: boolean;
  peaks: Float32Array | null; peaksPerSec: number; peakMax: number;
};

const local = new Map<string, Local>();
const listeners = new Set<() => void>();
export const onLocal = (f: () => void) => { listeners.add(f); return () => { listeners.delete(f); }; };
const changed = () => { for (const f of listeners) f(); };
export const localFor = (key: string | undefined) => (key ? local.get(key) || null : null);

// ---- IndexedDB: key → the file ----
let dbp: Promise<IDBDatabase | null> | null = null;
function db() {
  if (!dbp) dbp = new Promise((resolve) => {
    try {
      const r = indexedDB.open("myco-video", 1);
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

function register(key: string, file: Blob, name: string) {
  const had = local.get(key);
  if (had) return had;
  const l: Local = { key, file, name, url: URL.createObjectURL(file), thumbs: [], thumbsDone: false, peaks: null, peaksPerSec: 50, peakMax: 1 };
  local.set(key, l);
  changed();
  void thumbnails(l);
  void waveform(l);
  return l;
}

/** Keeps a file on this computer; its key. */
export async function keep(file: File) {
  const key = await keyOf(file);
  await idb("readwrite", (s) => s.put({ file, name: file.name, at: Date.now() }, key));
  register(key, file, file.name);
  return key;
}

/** Finds the files of these keys that this computer kept. */
export async function restore(keys: string[]) {
  for (const k of keys) {
    if (!k || local.has(k)) continue;
    const v = await idb<{ file: Blob; name: string }>("readonly", (s) => s.get(k));
    if (v?.file) register(k, v.file, v.name);
  }
}

export async function forget(key: string) {
  const l = local.get(key);
  if (l) { URL.revokeObjectURL(l.url); for (const t of l.thumbs) URL.revokeObjectURL(t.url); local.delete(key); changed(); }
  await idb("readwrite", (s) => s.delete(key));
}

// ---- what a file is ----
export type Probe = { duration: number; width: number; height: number; audio: boolean; poster: string };

function videoEl(url: string) {
  const v = document.createElement("video");
  v.muted = true; v.playsInline = true; v.preload = "auto"; v.crossOrigin = "anonymous"; v.src = url;
  return v;
}
const once = (el: HTMLMediaElement, ev: string, ms = 8000) => new Promise<boolean>((resolve) => {
  const done = (ok: boolean) => { el.removeEventListener(ev, yes); el.removeEventListener("error", no); clearTimeout(t); resolve(ok); };
  const yes = () => done(true), no = () => done(false);
  const t = setTimeout(() => done(false), ms);
  el.addEventListener(ev, yes); el.addEventListener("error", no);
});
async function seek(v: HTMLVideoElement, t: number) {
  if (Math.abs(v.currentTime - t) < 0.01 && v.readyState >= 2) return true;
  v.currentTime = t;
  return once(v, "seeked");
}

export async function probe(file: Blob): Promise<Probe> {
  let duration = 0, width = 0, height = 0, audio = false;
  try {
    const input = new Input({ source: new BlobSource(file), formats: ALL_FORMATS });
    const vt = await input.getPrimaryVideoTrack();
    const at = await input.getPrimaryAudioTrack();
    audio = !!at;
    if (vt) { width = vt.displayWidth; height = vt.displayHeight; }
    duration = await input.computeDuration();
    input.dispose();
  } catch { /* (read below with the browser's own player) */ }
  const url = URL.createObjectURL(file);
  const v = videoEl(url);
  let poster = "";
  try {
    if (await once(v, "loadeddata")) {
      if (!duration || !Number.isFinite(duration)) duration = v.duration;
      if (!width) { width = v.videoWidth; height = v.videoHeight; }
      if (await seek(v, Math.min(duration * 0.1, 1))) poster = frameUrl(v, 192, "image/jpeg", 0.6);
    }
  } finally { v.removeAttribute("src"); v.load(); URL.revokeObjectURL(url); }
  return { duration: Number.isFinite(duration) ? duration : 0, width, height, audio, poster };
}

function frameUrl(v: HTMLVideoElement, w: number, type: string, q?: number) {
  const k = w / Math.max(1, v.videoWidth);
  const c = document.createElement("canvas");
  c.width = Math.max(1, Math.round(v.videoWidth * k)); c.height = Math.max(1, Math.round(v.videoHeight * k));
  try { c.getContext("2d")!.drawImage(v, 0, 0, c.width, c.height); return c.toDataURL(type, q); } catch { return ""; }
}

// ---- thumbnails and waveform, made in the background, one file at a time ----
let line: Promise<unknown> = Promise.resolve();
const queue = (f: () => Promise<unknown>) => { line = line.then(f, f); return line; };

function thumbnails(l: Local) {
  return queue(async () => {
    const v = videoEl(l.url);
    try {
      if (!(await once(v, "loadeddata"))) return;
      const d = v.duration;
      if (!Number.isFinite(d) || d <= 0) return;
      const n = Math.min(120, Math.max(4, Math.ceil(d / 2)));
      for (let i = 0; i < n; i++) {
        const t = Math.min(d - 0.05, (i + 0.5) * (d / n));
        if (!(await seek(v, t))) break;
        const c = document.createElement("canvas");
        const h = 96, w = Math.max(1, Math.round((v.videoWidth / Math.max(1, v.videoHeight)) * h));
        c.width = w; c.height = h;
        c.getContext("2d")!.drawImage(v, 0, 0, w, h);
        const blob = await new Promise<Blob | null>((r) => c.toBlob(r, "image/jpeg", 0.7));
        if (blob) l.thumbs.push({ t, url: URL.createObjectURL(blob) });
        if (i % 4 === 3) changed();
      }
    } finally { l.thumbsDone = true; v.removeAttribute("src"); v.load(); changed(); }
  });
}

function waveform(l: Local) {
  return queue(async () => {
    const per = l.peaksPerSec;
    try {
      const input = new Input({ source: new BlobSource(l.file), formats: ALL_FORMATS });
      const at = await input.getPrimaryAudioTrack();
      if (!at || !(await at.canDecode())) { input.dispose(); return; }
      const d = await input.computeDuration();
      const peaks = new Float32Array(Math.ceil(d * per) + 1);
      for await (const w of new AudioBufferSink(at).buffers()) {
        const b = w.buffer, ch = b.getChannelData(0), rate = b.sampleRate;
        for (let i = 0; i < ch.length; i += 64) {
          const k = Math.floor((w.timestamp + i / rate) * per);
          const a = Math.abs(ch[i]);
          if (k >= 0 && k < peaks.length && a > peaks[k]) peaks[k] = a;
        }
      }
      l.peaks = peaks;
      l.peakMax = Math.max(0.05, ...peaks.slice(0, 200000));
      input.dispose();
    } catch {
      // (the browser's own decoder, for what the library cannot read)
      try {
        if (l.file.size > 300 * 1024 * 1024) return;
        const ctx = new OfflineAudioContext(1, 1, 44100);
        const b = await ctx.decodeAudioData(await l.file.arrayBuffer());
        const ch = b.getChannelData(0), peaks = new Float32Array(Math.ceil(b.duration * per) + 1);
        for (let i = 0; i < ch.length; i += 64) { const k = Math.floor((i / b.sampleRate) * per); const a = Math.abs(ch[i]); if (a > peaks[k]) peaks[k] = a; }
        l.peaks = peaks;
        l.peakMax = Math.max(0.05, ...peaks.slice(0, 200000));
      } catch { /* no sound to show */ }
    }
    changed();
  });
}
