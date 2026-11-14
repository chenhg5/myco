import { useSyncExternalStore } from "react";

/*
 * 过程只看进度: a run shows only what the agent reported itself (myco
 * message progress: its words, its plan, its pictures) and its questions;
 * its thinking, commands and the text it writes as it goes are left out.
 * Whether it is at work still shows. An App setting, kept on this computer.
 */

const KEY = "myco-runs-reports-only";

function load(): boolean {
  try { return localStorage.getItem(KEY) === "1"; } catch { return false; }
}

let on = load();
const listeners = new Set<() => void>();

export function setReportsOnly(v: boolean) {
  on = v;
  try { localStorage.setItem(KEY, v ? "1" : "0"); } catch { /* not kept */ }
  listeners.forEach((l) => l());
}

export function useReportsOnly(): boolean {
  return useSyncExternalStore(
    (cb) => { listeners.add(cb); return () => listeners.delete(cb); },
    () => on,
    () => on,
  );
}
