import { useEffect, useSyncExternalStore } from "react";
import { appApi } from "@/appapi";

/*
 * Messages taken out of your own view of a conversation (删除): kept on the
 * workspace, so your other devices agree; on a workspace too old to keep
 * them, on this device. Nothing is deleted: the agents keep their history,
 * and 撤销 shows one again.
 */

const byHub = new Map<string, Set<string>>();
const local = new Set<string>(); // hubs that keep none: this device keeps them
const asked = new Set<string>();
let version = 0;
const listeners = new Set<() => void>();
const emit = () => { version++; listeners.forEach((l) => l()); };
const subscribe = (cb: () => void) => { listeners.add(cb); return () => { listeners.delete(cb); }; };
const EMPTY = new Set<string>();

const localKey = (hubId: string) => `myco-hidden-${hubId}`;
function readLocal(hubId: string): string[] {
  try { return JSON.parse(localStorage.getItem(localKey(hubId)) || "[]") as string[]; } catch { return []; }
}
function writeLocal(hubId: string, ids: Set<string>) {
  try { localStorage.setItem(localKey(hubId), JSON.stringify([...ids].slice(-5000))); } catch { /* not kept */ }
}

/** What you took out of view on this workspace. */
export function useHidden(hubId: string): ReadonlySet<string> {
  useEffect(() => {
    if (!hubId || asked.has(hubId)) return;
    asked.add(hubId);
    appApi.hiddenMessages(hubId).then((r) => { byHub.set(hubId, new Set(r.ids)); emit(); }, (e) => {
      if ((e as { code?: string })?.code === "not_found") {
        local.add(hubId);
        byHub.set(hubId, new Set(readLocal(hubId)));
        emit();
      } else {
        asked.delete(hubId); // asked again next time
      }
    });
  }, [hubId]);
  useSyncExternalStore(subscribe, () => version, () => version);
  return byHub.get(hubId) || EMPTY;
}

/** Takes messages out of view (show: back again). At once here; the
 *  workspace is told after (and the list follows what it says). */
export async function setHidden(hubId: string, ids: string[], hide: boolean) {
  const cur = new Set(byHub.get(hubId) || []);
  ids.forEach((id) => (hide ? cur.add(id) : cur.delete(id)));
  byHub.set(hubId, cur);
  emit();
  if (local.has(hubId)) { writeLocal(hubId, cur); return; }
  try {
    const r = await appApi.updateHidden(hubId, hide ? { hide: ids } : { show: ids });
    byHub.set(hubId, new Set(r.ids));
    emit();
  } catch (e) {
    if ((e as { code?: string })?.code === "not_found") {
      local.add(hubId);
      writeLocal(hubId, cur);
      return;
    }
    // Not taken: as it was.
    const back = new Set(byHub.get(hubId) || []);
    ids.forEach((id) => (hide ? back.delete(id) : back.add(id)));
    byHub.set(hubId, back);
    emit();
    throw e;
  }
}
