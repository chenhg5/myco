import { useCallback, useEffect, useRef, useState } from "react";

/**
 * What the App fetched, by key: a page opened again shows its last data at
 * once and refreshes it in the background, instead of starting empty. The
 * most recently used entries are kept in memory and, in the desktop app,
 * saved by its backend (encrypted, /app/cache) so they are there again after
 * a restart (hydrate).
 */
const store = new Map<string, unknown>();
const MAX = 400;

// Saving: entries changed since the last save, written a moment later.
let persist = false;
const dirty = new Set<string>();
const savedJSON = new Map<string, string>();
let timer: number | undefined;
const SAVE_AFTER_MS = 1500;
const MAX_ENTRY = 4 << 20; // a bigger answer stays in memory only

/** The hub an entry belongs to ("" for this computer or several hubs):
 *  keys are kind\u0000hub\u0000…, or convlist\u0000[hub, …]. */
export function hubOf(key: string): string {
  const parts = key.split("\u0000");
  if (parts.length < 2) return "";
  if (parts[0] === "convlist") {
    try {
      const h = JSON.parse(parts[1])[0];
      return typeof h === "string" && h !== "all" ? h : "";
    } catch {
      return "";
    }
  }
  return parts[1];
}

function markDirty(key: string) {
  if (!persist) return;
  dirty.add(key);
  if (timer === undefined) timer = window.setTimeout(() => void flush(), SAVE_AFTER_MS);
}

/** Saves the changed entries now (keepalive: also when the window closes). */
export async function flush(keepalive = false) {
  if (timer !== undefined) { window.clearTimeout(timer); timer = undefined; }
  if (!persist || dirty.size === 0) return;
  const entries: { key: string; hub: string; data: unknown }[] = [];
  const del: string[] = [];
  for (const key of dirty) {
    if (!store.has(key)) { del.push(key); savedJSON.delete(key); continue; }
    const json = JSON.stringify(store.get(key));
    if (json === undefined || json.length > MAX_ENTRY || savedJSON.get(key) === json) continue;
    savedJSON.set(key, json);
    entries.push({ key, hub: hubOf(key), data: JSON.parse(json) });
  }
  dirty.clear();
  if (entries.length === 0 && del.length === 0) return;
  try {
    await fetch("app/cache", {
      method: "PUT", keepalive, credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ entries, delete: del }),
    });
  } catch {
    // A cache that cannot be saved only means a slower next start.
  }
}

/**
 * Loads what the backend saved last time (desktop app). Waits at most
 * timeoutMs: a slow cache must not hold the App up.
 */
export async function hydrate(timeoutMs = 800) {
  const ctl = new AbortController();
  const t = window.setTimeout(() => ctl.abort(), timeoutMs);
  try {
    const r = await fetch("app/cache", { credentials: "same-origin", signal: ctl.signal, headers: { Accept: "application/json" } });
    if (!r.ok) return;
    const body = await r.json() as { enabled?: boolean; entries?: { key: string; data: unknown }[] };
    persist = !!body.enabled;
    // Most recently used first: insert the oldest first.
    for (const e of [...(body.entries || [])].reverse()) {
      if (store.has(e.key)) continue; // fetched meanwhile: newer
      store.set(e.key, e.data);
      savedJSON.set(e.key, JSON.stringify(e.data));
    }
    while (store.size > MAX) store.delete(store.keys().next().value as string);
  } catch {
    // No cache this time.
  } finally {
    window.clearTimeout(t);
  }
  if (persist && typeof window !== "undefined") {
    const save = () => { if (document.visibilityState === "hidden") void flush(true); };
    document.addEventListener("visibilitychange", save);
    window.addEventListener("pagehide", () => void flush(true));
  }
}

/** Empties the memory and the saved cache (Settings). */
export async function clearAll() {
  store.clear();
  dirty.clear();
  savedJSON.clear();
  await fetch("app/cache", { method: "DELETE", credentials: "same-origin" });
}

export function recall<T>(key: string): T | undefined {
  if (!store.has(key)) return undefined;
  const v = store.get(key);
  store.delete(key); // most recently used last
  store.set(key, v);
  return v as T;
}

export function remember<T>(key: string, value: T): T {
  store.delete(key);
  store.set(key, value);
  markDirty(key);
  while (store.size > MAX) {
    const old = store.keys().next().value as string;
    store.delete(old);
    markDirty(old); // deleted on the next save too
  }
  return value;
}

/** Drops every entry whose key mentions a hub (it was removed). Keys put
 *  the hub id between \u0000 separators, or in a JSON list. */
export function forgetHub(hubId: string) {
  for (const k of [...store.keys()]) {
    if (k.includes(`\u0000${hubId}\u0000`) || k.endsWith(`\u0000${hubId}`) || k.includes(JSON.stringify(hubId)) || hubOf(k) === "") {
      store.delete(k);
      savedJSON.delete(k); // the backend drops the hub's saved entries itself
    }
  }
}

// Requests in flight by key: two views asking for the same thing share one.
const inflight = new Map<string, Promise<unknown>>();

function fetchOnce<T>(key: string, load: () => Promise<T>): Promise<T> {
  const p = inflight.get(key);
  if (p) return p as Promise<T>;
  const q = load().finally(() => inflight.delete(key));
  inflight.set(key, q);
  return q;
}

/**
 * Data for key: what was fetched last time at once (undefined the first
 * time), then the fresh answer. It refetches when key or refreshOn change;
 * a failed refresh keeps the data and reports the error. A null key loads
 * nothing.
 */
export function useCached<T>(key: string | null, load: () => Promise<T>, refreshOn: unknown[] = []) {
  const [data, setData] = useState<T | undefined>(() => (key ? recall<T>(key) : undefined));
  const [error, setError] = useState<unknown>(null);
  const [nonce, setNonce] = useState(0);
  const shown = useRef(key);
  if (shown.current !== key) {
    // Another key: its own last data right away, not the previous key's.
    shown.current = key;
    const v = key ? recall<T>(key) : undefined;
    if (v !== data) setData(v);
    if (error) setError(null);
  }
  const loadRef = useRef(load);
  loadRef.current = load;
  useEffect(() => {
    if (!key) return;
    let off = false;
    fetchOnce(key, () => loadRef.current())
      .then((v) => { if (!off) { setData(remember(key, v)); setError(null); } })
      .catch((e) => { if (!off) setError(e); });
    return () => { off = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, nonce, ...refreshOn]);
  const reload = useCallback(() => setNonce((n) => n + 1), []);
  /** Replaces the data (after a change the person made), here and in memory. */
  const set = useCallback((v: T) => { if (key) remember(key, v); setData(v); }, [key]);
  return { data, error, reload, set };
}
