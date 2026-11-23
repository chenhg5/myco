// Human edits never replace the agent's research snapshot. Store writes use CAS.
import { decisions, kit } from "./model";
export async function toggleSaved(comparison: string, offer: string) {
  const before = decisions.get(comparison)?.data || { saved: [], note: "" };
  const saved = Array.isArray(before.saved) ? before.saved.filter((x): x is string => typeof x === "string") : [];
  const next = saved.includes(offer) ? saved.filter(x => x !== offer) : [...saved, offer];
  return kit.track(decisions.put({ ...before, saved: next }, comparison));
}
export async function saveNote(comparison: string, note: string, expectedVersion: number) {
  const current = decisions.get(comparison);
  if ((current?.version || 0) !== expectedVersion) throw new Error("conflict");
  const row = await decisions.put({ ...(current?.data || {}), saved: current?.data.saved || [], note }, comparison);
  kit.emit();
  return row;
}
