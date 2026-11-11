import { useCallback, useEffect, useRef, useState } from "react";

/**
 * What the App fetched while it runs, by key: a page opened again shows its
 * last data at once and refreshes it in the background, instead of starting
 * empty. Memory only (gone when the App quits); the most recently used
 * entries are kept.
 */
const store = new Map<string, unknown>();
const MAX = 400;

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
  while (store.size > MAX) store.delete(store.keys().next().value as string);
  return value;
}

/** Drops every entry whose key mentions a hub (it was removed). Keys put
 *  the hub id between \u0000 separators, or in a JSON list. */
export function forgetHub(hubId: string) {
  for (const k of [...store.keys()]) {
    if (k.includes(`\u0000${hubId}\u0000`) || k.endsWith(`\u0000${hubId}`) || k.includes(JSON.stringify(hubId))) store.delete(k);
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
