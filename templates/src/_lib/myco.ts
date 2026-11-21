// The myco bridge (internal/console/pagebridge.js), as the App puts it in a
// page: a page's records and the person's Downloads.
export type MycoRecord<T = Record<string, unknown>> = { collection: string; id: string; data: T; version: number; updated_by?: string };
export type MycoChange = { seq: number; collection: string; record: string; op: "put" | "delete"; data?: Record<string, unknown>; version: number; by?: string };
export type PagePreferences = { language: "zh" | "en"; theme: "light" | "dark" };
type Ctx = { preferences?: PagePreferences; page: { id: string; name: string }; me: { name: string } };

interface Bridge {
  readonly preferences?: PagePreferences;
  on?(event: "preferences", callback: (value: PagePreferences) => void): () => void;
  ready: Promise<Ctx>;
  data: {
    list(collection?: string): Promise<{ records: MycoRecord[]; seq: number }>;
    put(collection: string, data: unknown, opts?: { id?: string; ifVersion?: number }): Promise<MycoRecord>;
    remove(collection: string, id: string, opts?: { ifVersion?: number }): Promise<boolean>;
    on(f: (c: MycoChange) => void): () => void;
  };
  download(name: string, content: string | Blob | ArrayBuffer | Uint8Array, type?: string): Promise<{ path: string }>;
  /** What the App's bridge can do (Apps from v0.20.27); missing in older ones.
   *  "os": myco.os is there. */
  features?: string[];
  /** This computer (the App's): files, commands, the screen… Each system
   *  does what it can; what it cannot answers err.code "unsupported". */
  os?: MycoOS;
  /** Network-only bridge on iOS; independent of desktop computer access. */
  net?: MycoOS["net"];
  remote?: {call<T>(action: string, args?: unknown): Promise<T>; onDisconnect?(handler:()=>void):()=>void};
}

/** An error from myco.os: err.code is one of these. */
export type OSErrorCode = "invalid_argument" | "not_found" | "exists" | "permission_denied" | "unsupported" | "timeout" | "cancelled" | "too_large" | "failed";

/** A job: a capability that runs a while (fs.scan, exec.spawn). */
export interface OSJob<T> {
  /** Its events: "progress" (fs.scan), "started" {pid}, "stdout" / "stderr"
   *  (text, or {base64} when not UTF-8) (exec.spawn); "*" for all. */
  on(type: string, f: (data: any, e: { n: number; type: string }) => void): () => void;
  /** Its value when it ends (rejects with err.code "cancelled" when stopped). */
  done: Promise<T>;
  cancel(): Promise<unknown>;
  /** exec.spawn: what the program reads, and the end of it. */
  write(data: string): Promise<unknown>;
  end(): Promise<unknown>;
}

export type OSEntry = { name: string; path: string; type: "file" | "dir" | "link" | "other"; size: number; modified: number; mode: string; hidden?: boolean; target?: string };
/** scan_path: where to scan to see its files when not mount (macOS: the startup disk's Data volume). */
export type OSDisk = { mount: string; device?: string; fs?: string; label?: string; total: number; free: number; available: number; scan_path?: string; removable?: boolean; network?: boolean; read_only?: boolean };
/** A folder or file in a scan; children largest first (by disk), smaller ones folded into more*. */
export type OSScanNode = {
  path: string; name: string; dir: boolean; size: number; disk: number; files: number; modified?: number; children?: OSScanNode[];
  more?: number; more_size?: number; more_disk?: number; more_files?: number; unreadable?: boolean;
};
export type OSScan = {
  roots: OSScanNode[]; largest: { path: string; size: number; disk: number; modified: number }[];
  files: number; dirs: number; bytes: number; disk: number; skipped: number; skipped_paths: string[]; ms: number;
};
export type OSApp = { name: string; id?: string; version?: string; path?: string; publisher?: string; size?: number; modified?: number; system?: boolean };
export type OSSettingsPane = "full_disk" | "screen_recording" | "accessibility" | "notifications" | "privacy" | "storage" | "network" | "display" | "sound" | "bluetooth";
export type OSImage = { type: string; width: number; height: number; base64: string };
export type OSAudioSource = { id: string; kind: "mic" | "system" | "app"; name: string; default?: boolean; playing?: boolean; bundle?: string };
export type OSAudioLevel = { seconds: number; tracks: Record<string, { rms: number; peak: number }> };
export type OSRecording = { dir: string; seconds: number; rate: number; ended: "stopped" | "cancelled" | "limit"; tracks: { id: string; source: string; path: string; seconds: number; bytes?: number; error?: string }[] };
export type OSSpeechStatus = { ready: boolean; engine: boolean; engine_version: string; vad: boolean; dir: string; engine_size_mb: number; diarize?: boolean; models: { id: string; name: string; languages: string[]; size_mb: number; installed: boolean }[] };
export type OSSegment = { start: number; end: number; text: string };
export type OSRun = { code: number; stdout: string; stderr: string; stdout_encoding?: "base64"; stderr_encoding?: "base64"; timed_out?: boolean; truncated?: boolean; ms: number };
type ExecArgs = { argv?: string[]; shell?: string; cwd?: string; env?: Record<string, string>; stdin?: string; stdin_encoding?: "base64"; timeout_ms?: number; max_output?: number };
type Paths = { path?: string; paths?: string[] };
type Batch = { done: string[]; failed: { path: string; error: { code: OSErrorCode; message: string } }[] };

export interface MycoOS {
  /** Every capability, and whether it works here (missing: why not). */
  list(): Promise<{ os: string; arch: string; caps: { name: string; job?: boolean; about: string; available: boolean; missing?: string }[] }>;
  has(name: string): Promise<boolean>;
  /** Any capability by name (a newer App may have more than this list). */
  call<T = unknown>(name: string, args?: object, opts?: { timeout?: number }): Promise<T>;
  start<T = unknown>(name: string, args?: object): OSJob<T>;
  /** An address for <audio> of a recording in ~/.myco/recordings (seeks, any speed). */
  mediaURL(path: string): string;

  system: {
    info(): Promise<{ os: string; name: string; version: string; arch: string; model?: string; hostname: string; user: string; home: string; cpus: number; cpu_model?: string; memory: number; uptime: number }>;
    disks(): Promise<OSDisk[]>;
    processes(): Promise<{ pid: number; ppid: number; name: string; path?: string; user?: string; cpu: number; memory: number }[]>;
    /** CPU (percent of all cores), memory, load; takes a short sample (macOS about a second). */
    usage(): Promise<{ cpu: number; memory_total: number; memory_used: number; memory_available: number; load?: number[] }>;
    /** Installed apps; size: true adds up each one's files (seconds). */
    apps(a?: { size?: boolean }): Promise<OSApp[]>;
    /** Opens a page of the system's settings (to grant 完全磁盘访问, 屏幕录制…). */
    settings(a: { pane: OSSettingsPane }): Promise<true>;
  };
  process: { kill(a: { pid: number; force?: boolean }): Promise<true> };
  fs: {
    /** home, desktop, documents, downloads, pictures, music, videos, temp, cache, config, trash… */
    paths(): Promise<Record<string, string>>;
    /** Paths are absolute, or start with "~". */
    list(a: { path: string; hidden?: boolean; limit?: number }): Promise<{ path: string; entries: OSEntry[]; truncated?: boolean }>;
    stat(a: { path: string }): Promise<OSEntry>;
    read(a: { path: string; encoding?: "utf8" | "base64"; offset?: number; length?: number }): Promise<{ path: string; content: string; encoding: "utf8" | "base64"; size: number; offset: number; length: number; eof: boolean }>;
    write(a: { path: string; content: string; encoding?: "utf8" | "base64"; append?: boolean; mkdirs?: boolean; exclusive?: boolean }): Promise<OSEntry>;
    mkdir(a: { path: string }): Promise<OSEntry>;
    move(a: { from: string; to: string; overwrite?: boolean }): Promise<OSEntry>;
    copy(a: { from: string; to: string; overwrite?: boolean }): Promise<OSEntry>;
    delete(a: Paths & { recursive?: boolean }): Promise<Batch>;
    trash(a: Paths): Promise<Batch>;
    /** By size; min_fraction (0.001 = 0.1%) folds what is smaller into "more", so a whole disk stays small. */
    scan(a: Paths & { depth?: number; top?: number; min_size?: number; min_fraction?: number; per_level?: number; one_filesystem?: boolean }): OSJob<OSScan>;
    /** Events "match" (OSEntry[]) and "progress" {scanned, found}. name: a part of the name, or a pattern (report*.pdf). */
    search(a: Paths & { name?: string; ext?: string[]; type?: "file" | "dir" | "any"; min_size?: number; max_size?: number; modified_after?: number; modified_before?: number; hidden?: boolean; limit?: number }): OSJob<{ matches: OSEntry[]; scanned: number; truncated: boolean }>;
    /** Runs until cancelled; events "ready" and "change" ({type: created | modified | removed, path, dir?, size?}[]). */
    watch(a: { path: string; recursive?: boolean; interval_ms?: number; max_entries?: number }): OSJob<{ stopped: boolean }>;
    thumbnail(a: { path: string; size?: number; format?: "png" | "jpeg" }): Promise<OSImage>;
    /** The system's open dialog; err.code "cancelled" when closed. */
    pick(a?: { title?: string; multiple?: boolean; folders?: boolean; types?: string[]; folder?: string }): Promise<{ paths: string[] }>;
    /** The system's save dialog: where to write (then fs.write it). */
    saveAs(a?: { title?: string; name?: string; folder?: string }): Promise<{ path: string }>;
    reveal(a: { path: string }): Promise<true>;
    open(a: { path: string; app?: string }): Promise<true>;
  };
  exec: {
    /** sh on macOS and Linux, PowerShell on Windows, for a.shell. */
    run(a: ExecArgs): Promise<OSRun>;
    spawn(a: ExecArgs): OSJob<{ code: number; ms: number; pid: number; timed_out?: boolean }>;
  };
  clipboard: { read(): Promise<{ text: string }>; write(a: { text: string }): Promise<true> };
  notify(a: { title?: string; body?: string; subtitle?: string; sound?: boolean }): Promise<true>;
  screen: {
    region(): Promise<{x:number; y:number; w:number; h:number}>;
    /** macOS screen recording. Write "stop" to finalize; originals stay local. */
    record(a?: { region?: {x:number; y:number; w:number; h:number}; interactive?: boolean; microphone?: boolean; clicks?: boolean; max_seconds?: number }): OSJob<{path: string; bytes: number}>;
    capture(a?: { display?: number; region?: { x: number; y: number; width: number; height: number }; format?: "png" | "jpeg"; quality?: number }): Promise<OSImage & { bytes: number }>;
  };
  /** Recording: each source a track, a 16 kHz mono WAV of its own. */
  audio: {
    /** "mic", "system" (what the computer plays), "app:<id>" (one app's sound; macOS 14.2+, Linux). */
    sources(): Promise<{ sources: OSAudioSource[] }>;
    /** Events "started" {dir, files}, "level" (every 100 ms), "paused", "resumed". job.write("pause" | "resume" | "stop"): stop ends it with the files. */
    record(a?: { tracks?: { id: string; source: string }[]; dir?: string; max_minutes?: number }): OSJob<OSRecording>;
  };
  /** Speech to text on this computer (sherpa-onnx; downloaded the first time). */
  speech: {
    status(): Promise<OSSpeechStatus>;
    /** Events "progress" {step: engine | vad | model, received, total, unpacking?}. */
    install(a?: { model?: string }): OSJob<OSSpeechStatus>;
    /** Events "segment" (one sentence) and "progress" {done, total} (seconds). WAV, or other formats via afconvert / ffmpeg. */
    transcribe(a: { path: string; model?: string; language?: string; offset?: number }): OSJob<{ segments: OSSegment[]; seconds: number; ms: number; model: string }>;
    /** Who speaks when: speakers numbered 0, 1… in the order they first speak. speakers: how many, when known. Events "progress" {percent}. */
    diarize(a: { path: string; speakers?: number; threshold?: number }): OSJob<{ turns: { start: number; end: number; speaker: number }[]; speakers: number; ms: number }>;
  };
  net: {
    fetch(a: { url: string; method?: string; headers?: Record<string, string>; body?: string; body_encoding?: "base64"; response?: "auto" | "text" | "base64"; timeout_ms?: number; max_bytes?: number; redirect?: boolean }): Promise<{
      status: number; status_text: string; url: string; headers: Record<string, string>; body: string; encoding: "utf8" | "base64"; truncated?: boolean;
    }>;
  };
}

export const myco = (window as unknown as { myco?: Bridge }).myco;

/** One collection of the page's records, kept here in order (data.order),
 *  with what others change applied as it comes. Writes to one record go
 *  one after another; while some wait, what is here is newer than the
 *  answers and the feed. */
export class Store<T extends Record<string, unknown> = Record<string, unknown>> {
  rows: MycoRecord<T>[] = [];
  private queues = new Map<string, Promise<unknown>>();
  private waiting = new Map<string, number>();
  private listeners: ((c: MycoChange) => void)[] = [];
  constructor(readonly collection: string) {}

  private sort() {
    this.rows.sort((a, b) => {
      const x = a.data.order, y = b.data.order;
      return typeof x === "number" && typeof y === "number" ? x - y : 0;
    });
  }
  get(id: string) { return this.rows.find((r) => r.id === id) || null; }
  async load() {
    const r = await myco!.data.list(this.collection);
    this.rows = r.records as MycoRecord<T>[];
    this.sort();
    return this.rows;
  }
  private upsert(rec: MycoRecord) {
    const cur = this.get(rec.id);
    if (cur) { if (cur.version <= rec.version) { cur.data = rec.data as T; cur.version = rec.version; } }
    else this.rows.push({ collection: this.collection, id: rec.id, data: rec.data as T, version: rec.version });
    this.sort();
  }
  pending(id: string) { return (this.waiting.get(id) || 0) > 0; }
  /** Writes a record (all of it) based on the version here; id "" for a
   *  new one. The record is changed here at once. */
  put(data: T, id = ""): Promise<MycoRecord<T> | null> {
    if (id) {
      const cur = this.get(id);
      if (cur) cur.data = data;
      else { this.rows.push({ collection: this.collection, id, data, version: 0 }); this.sort(); }
    }
    const run = async () => {
      const cur = id ? this.get(id) : null;
      try {
        const rec = await myco!.data.put(this.collection, data, { id, ifVersion: cur ? cur.version : undefined });
        if (id) this.waiting.set(id, (this.waiting.get(id) || 1) - 1);
        const now = this.get(rec.id);
        if (id && this.pending(id) && now) now.version = Math.max(now.version, rec.version);
        else this.upsert(rec);
        return this.get(rec.id);
      } catch (e) {
        if (id) this.waiting.set(id, (this.waiting.get(id) || 1) - 1);
        const err = e as { code?: string; latest?: MycoRecord };
        if (err.code === "conflict" && err.latest) {
          this.upsert(err.latest);
          this.fire({ seq: 0, collection: this.collection, record: err.latest.id, op: "put", data: err.latest.data, version: err.latest.version });
        }
        throw e;
      }
    };
    if (!id) return run();
    this.waiting.set(id, (this.waiting.get(id) || 0) + 1);
    const p = (this.queues.get(id) || Promise.resolve()).then(run, run);
    this.queues.set(id, p.catch(() => undefined));
    return p;
  }
  async remove(id: string) {
    const i = this.rows.findIndex((r) => r.id === id);
    if (i >= 0) this.rows.splice(i, 1);
    await (this.queues.get(id) || Promise.resolve());
    await myco!.data.remove(this.collection, id).catch((e: { code?: string }) => { if (e.code !== "not_found") throw e; });
  }
  on(f: (c: MycoChange) => void) { this.listeners.push(f); }
  private fire(c: MycoChange) { for (const f of this.listeners) f(c); }
  /** A change from the feed; false when this page has it already. */
  apply(c: MycoChange): boolean {
    if (c.collection !== this.collection) return false;
    const cur = this.get(c.record);
    if (c.op === "delete") {
      if (!cur) return false;
      this.rows.splice(this.rows.indexOf(cur), 1);
    } else {
      if (this.pending(c.record)) return false;
      if (cur && cur.version >= c.version) return false;
      this.upsert({ collection: c.collection, id: c.record, data: c.data || {}, version: c.version });
    }
    this.fire(c);
    return true;
  }
}

/** Listens once: each store takes what is its own. */
export function listen(stores: Store<any>[], onAny?: (c: MycoChange) => void) {
  myco!.data.on((c) => {
    let news = false;
    for (const s of stores) if (s.apply(c)) news = true;
    if (news && onAny) onAny(c);
  });
}

/** Saves a file to the person's Downloads. */
export async function save(name: string, content: string | Uint8Array | Blob, type: string) {
  return myco!.download(name, content, type);
}

export function dark() {
  return myco?.features?.includes("preferences") && myco.preferences ? myco.preferences.theme === "dark" : window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
}

/** Local controller values from os.call("proxy.profile" / "proxy.control").
 * Credentials are intentionally absent from return values. */
export type OSProxyProfile = {
  endpoint: string;
  engine: "mihomo" | "sing-box";
  has_secret: boolean;
};
export type OSProxyNode = {
  name: string;
  type: string;
  now?: string;
  all?: string[];
  history?: { delay: number }[];
};
export type OSProxyConnection = {
  id: string;
  metadata: {
    network: string;
    host: string;
    destinationIP: string;
    destinationPort: string;
    process: string;
  };
  upload: number;
  download: number;
  start: string;
  chains: string[];
  rule: string;
};
export type OSProxySnapshot = {
  version: string;
  config: {
    mode: string;
    "mode-list"?: string[];
    "mixed-port": number;
    port: number;
    "socks-port": number;
    "allow-lan"?: boolean;
    "log-level": string;
    tun?: { enable: boolean };
  };
  proxies: Record<string, OSProxyNode> | null;
  traffic: {
    downloadTotal: number;
    uploadTotal: number;
    connections: OSProxyConnection[] | null;
  };
  warnings: string[];
  at: string;
};
