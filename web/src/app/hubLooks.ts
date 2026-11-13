import { useSyncExternalStore } from "react";

/*
 * How a workspace looks in this App (like its name here): a colour of the
 * eight, or a small picture. Kept in this App only.
 */

export type HubLook = { color?: number; pic?: string };

const KEY = "myco-app-hub-looks";

function load(): Record<string, HubLook> {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(KEY) || "{}");
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
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    // Too big for this browser's storage: keep the colour, drop the picture.
    if (look?.pic) return setHubLook(id, look.color !== undefined ? { color: look.color } : null);
  }
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
