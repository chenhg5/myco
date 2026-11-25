import type { Agent, Machine } from "@/api";

/** A failed restart must expire even when the app is showing cached data. */
export function runnerUpdating(a: Pick<Agent, "runtime" | "last_seen_at" | "updating">, now = Date.now()): boolean {
  if (!a.updating && a.runtime?.away !== "updating") return false;
  const until = a.runtime?.restarting_until
    ? Date.parse(a.runtime.restarting_until)
    : Date.parse(a.last_seen_at || "") + 90_000; // older Hub stripped new fields
  return Number.isFinite(until) && until > now && until <= now + 90_000;
}

export function machineUpdating(m: Machine, agents: Agent[], now = Date.now()): boolean {
  return agents.some((a) => a.machine_id === m.id && runnerUpdating(a, now)) ||
    (!!m.updating && Date.parse(m.last_seen_at || "") + 90_000 > now);
}
