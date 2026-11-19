import { appApi, type MachineInfo } from "@/appapi";
import type { Machine } from "@/api";
import { machineTellsInfo } from "@/app/hubVersion";

// What each computer told about itself (its sessions, AIs, folders),
// kept for this run of the App: asking one takes a round trip through the
// workspace (a second or two), so the App asks ahead, the dialogs show
// the last answer at once and refresh it behind it.

type Entry = { info?: MachineInfo; error?: unknown; at: number; pending?: Promise<MachineInfo> };

const cache = new Map<string, Entry>();
const key = (hubId: string, machineId: string) => `${hubId}\u0000${machineId}`;

/** Whether a computer can be asked (online, and its myco answers). */
export function askable(m: Machine): boolean {
  return m.online && m.can_spawn && machineTellsInfo(m.myco_version);
}

/** The last answer (or error) for a computer, if any. */
export function cachedMachineInfo(hubId: string, machineId: string): { info?: MachineInfo; error?: unknown } | undefined {
  const e = cache.get(key(hubId, machineId));
  return e && (e.info || e.error) ? { info: e.info, error: e.error } : undefined;
}

/** Asks a computer (one ask at a time per computer); an answer younger
 *  than maxAge is used as it is. */
export function fetchMachineInfo(hubId: string, machineId: string, maxAge = 15_000): Promise<MachineInfo> {
  const k = key(hubId, machineId);
  const e = cache.get(k);
  if (e?.pending) return e.pending;
  if (e?.info && Date.now() - e.at < maxAge) return Promise.resolve(e.info);
  const pending = appApi.machineInfo(hubId, machineId).then(
    (info) => { cache.set(k, { info, at: Date.now() }); return info; },
    (error) => {
      // Keep an earlier answer: a failed refresh does not erase what was known.
      const prev = cache.get(k);
      cache.set(k, prev?.info ? { info: prev.info, at: prev.at } : { error, at: Date.now() });
      throw error;
    },
  );
  cache.set(k, { ...(e || { at: 0 }), pending });
  return pending;
}

/** Drops what a computer said (a session just joined there). */
export function forgetMachineInfo(hubId: string, machineId: string) {
  cache.delete(key(hubId, machineId));
}

/** Asks every computer of a workspace that can answer, ahead of need
 *  (at most once a minute each). */
export function prefetchMachineInfos(hubId: string, machines: Machine[]) {
  for (const m of machines) {
    if (askable(m)) fetchMachineInfo(hubId, m.id, 60_000).catch(() => undefined);
  }
}
