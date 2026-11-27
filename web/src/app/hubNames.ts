import { localPreferences, PREFERENCES_CHANGED } from "@/lib/localPreferences";
import { useSyncExternalStore } from "react";

const NAMES_KEY = "myco-app-hub-names";

function load(): Record<string, string> {
  try {
    const raw = localPreferences.getItem(NAMES_KEY);
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? parsed as Record<string, string> : {};
  } catch {
    return {};
  }
}

// Local aliases belong to this App profile, never to the shared workspace.
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
  try { localPreferences.setItem(NAMES_KEY, JSON.stringify(next)); } catch { /* storage unavailable */ }
  emit();
}

export function useHubNames(): Record<string, string> {
  return useSyncExternalStore(
    (cb) => { listeners.add(cb); return () => listeners.delete(cb); },
    () => names,
    () => names,
  );
}

window.addEventListener(PREFERENCES_CHANGED, () => { names = load(); emit(); });
