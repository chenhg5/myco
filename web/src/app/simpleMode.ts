import { useSyncExternalStore } from "react";

/*
 * 简单模式: one person using the agents on
 * their own computer. Workspaces, members, other computers and the ways to
 * bring in agents from elsewhere are hidden; conversations and agents stay.
 * On when the App was started from the welcome page (a new install); off
 * for everyone who already had workspaces. Settings › 外观 switches it. An
 * App setting, kept on this computer; it only hides, nothing is removed.
 */

const KEY = "myco-simple-mode";

function load(): boolean {
  try { return localStorage.getItem(KEY) === "1"; } catch { return false; }
}

let on = load();
const listeners = new Set<() => void>();

export function setSimpleMode(v: boolean) {
  on = v;
  try { localStorage.setItem(KEY, v ? "1" : "0"); } catch { /* not kept */ }
  listeners.forEach((l) => l());
}

export function useSimpleMode(): boolean {
  return useSyncExternalStore(
    (cb) => { listeners.add(cb); return () => listeners.delete(cb); },
    () => on,
    () => on,
  );
}
