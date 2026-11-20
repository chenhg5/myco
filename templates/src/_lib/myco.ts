// The myco bridge (internal/console/pagebridge.js), as the App puts it in a
// page: a page's records and the person's Downloads.
export type MycoRecord<T = Record<string, unknown>> = { collection: string; id: string; data: T; version: number; updated_by?: string };
export type MycoChange = { seq: number; collection: string; record: string; op: "put" | "delete"; data?: Record<string, unknown>; version: number; by?: string };
type Ctx = { page: { id: string; name: string }; me: { name: string } };

interface Bridge {
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
export type OSDisk = { mount: string; device?: string; fs?: string; label?: string; total: number; free: number; available: number; removable?: boolean; network?: boolean; read_only?: boolean };
export type OSScanNode = { path: string; name: string; dir: boolean; size: number; disk: number; files: number; modified?: number; children?: OSScanNode[]; more?: number };
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

  system: {
    info(): Promise<{ os: string; name: string; version: string; arch: string; model?: string; hostname: string; user: string; home: string; cpus: number; cpu_model?: string; memory: number; uptime: number }>;
    disks(): Promise<OSDisk[]>;
    processes(): Promise<{ pid: number; ppid: number; name: string; path?: string; user?: string; cpu: number; memory: number }[]>;
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
    scan(a: Paths & { depth?: number; top?: number; min_size?: number; per_level?: number; one_filesystem?: boolean }): OSJob<{
      roots: OSScanNode[]; largest: { path: string; size: number; modified: number }[]; files: number; dirs: number; bytes: number; skipped: number; skipped_paths: string[]; ms: number;
    }>;
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
    capture(a?: { display?: number; region?: { x: number; y: number; width: number; height: number }; format?: "png" | "jpeg"; quality?: number }): Promise<{ type: string; width: number; height: number; bytes: number; base64: string }>;
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
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
}
