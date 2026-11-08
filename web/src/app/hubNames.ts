import { useSyncExternalStore } from "react";

const NAMES_KEY = "myco-app-hub-names";

function load(): Record<string, string> {
  try {
    const raw = localStorage.getItem(NAMES_KEY);
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? parsed as Record<string, string> : {};
  } catch {
    return {};
  }
}

// Renames are local to this App until the backend grows a name field.
let names = load();
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

export function renameHub(id: string, name: string) {
  const next = { ...names };
  const trimmed = name.trim();
  if (trimmed) next[id] = trimmed;
  else delete next[id];
  names = next;
  try { localStorage.setItem(NAMES_KEY, JSON.stringify(next)); } catch { /* storage unavailable */ }
  emit();
}

export function useHubNames(): Record<string, string> {
  return useSyncExternalStore(
    (cb) => { listeners.add(cb); return () => listeners.delete(cb); },
    () => names,
    () => names,
  );
}
