// The SDK for an app that uses this computer (myco.os, docs/app-os.md):
// the bridge's calls with errors as OSError, jobs with progress and an
// AbortSignal, React hooks, and the small helpers such apps need
// (sizes, paths, base64).
//
//   import { os, osState, OSError, useJob, formatBytes } from "../_lib/os";
//   const disks = await os().system.disks();
//   const scan = useJob(() => os().fs.scan({ path: "~", depth: 4 }));
import { useCallback, useEffect, useRef, useState } from "react";
import { myco, type MycoOS, type OSErrorCode, type OSJob } from "./myco";

/** Where this page is: in an App with myco.os ("ready"), in an App from
 *  before it ("old"), or not in the App at all ("outside"). */
export type OSState = "ready" | "old" | "outside";
export const osState: OSState = !myco ? "outside" : myco.os && (myco.features || []).includes("os") ? "ready" : "old";

/** What to tell the person when this page cannot use the computer. */
export function osStateText(s: OSState = osState): string {
  if (s === "outside") return "在 myco App 里打开这个应用，才能用这台电脑";
  if (s === "old") return "这个 myco App 还不能让应用用电脑：更新 App 后再试";
  return "";
}

/** An error from the computer: code says what happened (not_found, permission_denied, cancelled…). */
export class OSError extends Error {
  code: OSErrorCode;
  constructor(code: OSErrorCode, message: string) {
    super(message);
    this.name = "OSError";
    this.code = code;
  }
}

/** Any error as an OSError ("failed" when it has no code). */
export function toOSError(e: unknown): OSError {
  if (e instanceof OSError) return e;
  const x = e as { code?: string; message?: string } | null;
  return new OSError((x?.code as OSErrorCode) || "failed", x?.message || String(e));
}

/** Wraps a job so its done rejects with OSError and an AbortSignal stops it. */
function wrapJob<T>(j: OSJob<T>, signal?: AbortSignal): OSJob<T> {
  const done = j.done.catch((e) => { throw toOSError(e); });
  done.catch(() => undefined); // (handled by whoever awaits it)
  if (signal) {
    if (signal.aborted) void j.cancel();
    else signal.addEventListener("abort", () => void j.cancel(), { once: true });
  }
  return { ...j, on: j.on.bind(j), cancel: j.cancel.bind(j), write: j.write.bind(j), end: j.end.bind(j), done };
}

const isJob = (v: unknown): v is OSJob<unknown> => !!v && typeof v === "object" && "done" in v && typeof (v as OSJob<unknown>).on === "function";

// The bridge's os, with each call's errors turned into OSError and each
// job wrapped (the same shape as MycoOS).
function wrap<T extends object>(o: T): T {
  return new Proxy(o, {
    get(target, key) {
      const v = (target as Record<string | symbol, unknown>)[key];
      if (typeof v === "function") {
        return (...args: unknown[]) => {
          let r: unknown;
          try { r = (v as (...a: unknown[]) => unknown).apply(target, args); } catch (e) { return Promise.reject(toOSError(e)); }
          if (isJob(r)) return wrapJob(r);
          if (r instanceof Promise) return r.catch((e) => { throw toOSError(e); });
          return r;
        };
      }
      if (v && typeof v === "object") return wrap(v as object);
      return v;
    },
  });
}

let wrapped: MycoOS | null = null;
/** This computer, through the App; throws OSError "unsupported" when the page cannot use it (see osState). */
export function os(): MycoOS {
  if (osState !== "ready" || !myco?.os) throw new OSError("unsupported", osStateText());
  return (wrapped ||= wrap(myco.os));
}

let capsP: Promise<Map<string, { available: boolean; missing?: string; about: string }>> | null = null;
/** Every capability and whether it works here (asked once). */
export function capabilities() {
  if (!capsP) {
    capsP = osState !== "ready" ? Promise.resolve(new Map()) : os().list().then((l) => new Map(l.caps.map((c) => [c.name, c])), () => new Map());
  }
  return capsP;
}
/** Can this computer do it? false outside the App and in older Apps. */
export const has = async (name: string) => !!(await capabilities()).get(name)?.available;

/** Starts a job and follows it: onEvent for each event, an AbortSignal to stop it. */
export function follow<T>(job: OSJob<T>, opts: { onEvent?: (type: string, data: any) => void; signal?: AbortSignal } = {}): Promise<T> {
  const j = wrapJob(job, opts.signal);
  if (opts.onEvent) j.on("*", (data, e) => opts.onEvent!(e.type, data));
  return j.done;
}

// --- React ---

/** The computer, in a component: its state, its system info, and has(). */
export function useOS() {
  const [info, setInfo] = useState<Awaited<ReturnType<MycoOS["system"]["info"]>> | null>(null);
  const [caps, setCaps] = useState<Map<string, { available: boolean; missing?: string; about: string }> | null>(null);
  useEffect(() => {
    if (osState !== "ready") return;
    let live = true;
    os().system.info().then((i) => live && setInfo(i), () => undefined);
    capabilities().then((c) => live && setCaps(c));
    return () => { live = false; };
  }, []);
  return { state: osState, info, caps, has: (name: string) => !!caps?.get(name)?.available };
}

export type JobState<T, P> = {
  running: boolean;
  /** The last "progress" event. */
  progress: P | null;
  value: T | null;
  error: OSError | null;
  /** Starts it (stops one still running first). */
  start: () => Promise<T | null>;
  cancel: () => void;
};

/** A job in a component: start() runs make(), progress follows its
 *  "progress" events, value or error is how it ended. Unmounting stops it. */
export function useJob<T, P = any>(make: () => OSJob<T>, onEvent?: (type: string, data: any) => void): JobState<T, P> {
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState<P | null>(null);
  const [value, setValue] = useState<T | null>(null);
  const [error, setError] = useState<OSError | null>(null);
  const cur = useRef<{ job: OSJob<T>; n: number } | null>(null);
  const seq = useRef(0);
  const ev = useRef(onEvent);
  ev.current = onEvent;
  const mk = useRef(make);
  mk.current = make;
  const cancel = useCallback(() => { void cur.current?.job.cancel(); }, []);
  const start = useCallback(async () => {
    void cur.current?.job.cancel();
    const n = ++seq.current;
    setRunning(true); setProgress(null); setError(null);
    let job: OSJob<T>;
    try { job = mk.current(); } catch (e) { setError(toOSError(e)); setRunning(false); return null; }
    cur.current = { job, n };
    job.on("*", (data, e) => {
      if (seq.current !== n) return;
      if (e.type === "progress") setProgress(data as P);
      ev.current?.(e.type, data);
    });
    try {
      const v = await job.done;
      if (seq.current === n) { setValue(v); setRunning(false); }
      return v;
    } catch (e) {
      if (seq.current === n) { setError(toOSError(e)); setRunning(false); }
      return null;
    }
  }, []);
  useEffect(() => () => { seq.current++; void cur.current?.job.cancel(); }, []);
  return { running, progress, value, error, start, cancel };
}

// --- helpers ---

/** Bytes as people read them: 1.2 GB (powers of 1000, as macOS and DaisyDisk count; binary: 1024). */
export function formatBytes(n: number, opts: { binary?: boolean; digits?: number } = {}): string {
  const k = opts.binary ? 1024 : 1000;
  const units = opts.binary ? ["B", "KiB", "MiB", "GiB", "TiB", "PiB"] : ["B", "KB", "MB", "GB", "TB", "PB"];
  if (!Number.isFinite(n) || n < k) return `${Math.max(0, Math.round(n || 0))} B`;
  let i = 0;
  let v = n;
  while (v >= k && i < units.length - 1) { v /= k; i++; }
  const d = opts.digits ?? (v < 10 ? 1 : 0);
  return `${v.toFixed(d)} ${units[i]}`;
}

/** The path's separator (\\ on Windows paths). */
const sepOf = (p: string) => (/^[A-Za-z]:\\/.test(p) || (p.includes("\\") && !p.includes("/")) ? "\\" : "/");
export const basename = (p: string) => p.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || p;
export function dirname(p: string) {
  const t = p.replace(/[\\/]+$/, "");
  const i = Math.max(t.lastIndexOf("/"), t.lastIndexOf("\\"));
  if (i < 0) return ".";
  if (i === 0) return t[0];
  if (i === 2 && /^[A-Za-z]:/.test(t)) return t.slice(0, 3); // C:\
  return t.slice(0, i);
}
export function join(base: string, ...parts: string[]) {
  const s = sepOf(base);
  return [base.replace(/[\\/]+$/, ""), ...parts.map((x) => x.replace(/^[\\/]+|[\\/]+$/g, ""))].filter(Boolean).join(s);
}
export const extname = (p: string) => { const b = basename(p); const i = b.lastIndexOf("."); return i > 0 ? b.slice(i + 1).toLowerCase() : ""; };

export function fromBase64(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}
export function toBase64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

/** A file as text (utf8) or bytes; JSON read and written whole. */
export const readText = async (path: string) => (await os().fs.read({ path })).content;
export async function readBytes(path: string): Promise<Uint8Array> {
  const r = await os().fs.read({ path, encoding: "base64" });
  return fromBase64(r.content);
}
export const writeText = (path: string, text: string) => os().fs.write({ path, content: text });
export const writeBytes = (path: string, bytes: Uint8Array) => os().fs.write({ path, content: toBase64(bytes), encoding: "base64" });
export const readJSON = async <T = unknown>(path: string): Promise<T> => JSON.parse(await readText(path)) as T;
export const writeJSON = (path: string, v: unknown) => writeText(path, JSON.stringify(v, null, 2));

/** An image the computer gave (screen.capture, fs.thumbnail) as a src for <img>. */
export const imageSrc = (img: { type: string; base64: string }) => `data:${img.type};base64,${img.base64}`;
