import { localPreferences, PREFERENCES_CHANGED } from "@/lib/localPreferences";
import { useSyncExternalStore } from "react";

/*
 * How a workspace looks in this App (like its name here): a colour of the
 * eight, or a small picture. Kept in this App only.
 */

export type HubLook = { color?: number; pic?: string };

const KEY = "myco-app-hub-looks";

function load(): Record<string, HubLook> {
  try {
    const v: unknown = JSON.parse(localPreferences.getItem(KEY) || "{}");
    return v && typeof v === "object" ? v as Record<string, HubLook> : {};
  } catch {
    return {};
  }
}

let looks = load();
const listeners = new Set<() => void>();

export function setHubLook(id: string, look: HubLook | null) {
  const next = { ...looks };
  if (look && (look.color !== undefined || look.pic)) next[id] = look;
  else delete next[id];
  localPreferences.setItem(KEY, JSON.stringify(next));
  looks = next;
  listeners.forEach((l) => l());
}

export function useHubLooks(): Record<string, HubLook> {
  return useSyncExternalStore(
    (cb) => { listeners.add(cb); return () => listeners.delete(cb); },
    () => looks,
    () => looks,
  );
}

window.addEventListener(PREFERENCES_CHANGED, () => { looks = load(); listeners.forEach((l) => l()); });
