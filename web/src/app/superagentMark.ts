import { useSyncExternalStore } from "react";
import type { Agent } from "@/api";

// Which agents are superagents (yours, one per workspace), for marks drawn
// anywhere an agent shows (its crown, the sidebar leaving it out). Kept
// apart from Superagent.tsx so the avatar does not pull in its dialogs.
/** The superagent's fixed name (docs/design/chief-of-staff.md): whatever
 *  it called itself when it joined, the App calls it this. */
export const SHROOM = "Shroom";

const byHub = new Map<string, string>();
const subs = new Set<() => void>();
let version = 0;

const keyOf = (a: Pick<Agent, "id" | "name">) => a.id || a.name;

/** Notes a workspace's superagent (none: undefined). */
export function markSuperagent(hubId: string, agent?: Pick<Agent, "id" | "name">) {
  const next = agent ? keyOf(agent) : undefined;
  if (byHub.get(hubId) === next) return;
  if (next) byHub.set(hubId, next); else byHub.delete(hubId);
  version += 1;
  subs.forEach((f) => f());
}

/** Whether agent is a superagent (as far as this App knows: yours). */
export function isSuperagent(agent: Pick<Agent, "id" | "name">): boolean {
  const k = keyOf(agent);
  for (const v of byHub.values()) if (v === k) return true;
  return false;
}

/** isSuperagent, redrawn when it changes. */
export function useIsSuperagent(agent: Pick<Agent, "id" | "name">): boolean {
  useSyncExternalStore((f) => { subs.add(f); return () => { subs.delete(f); }; }, () => version);
  return isSuperagent(agent);
}
