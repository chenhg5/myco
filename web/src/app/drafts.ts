/*
 * What was typed in a box and not sent yet, kept per conversation: switch
 * to another chat and back, or restart the app, and it is still there. A
 * few dozen at most, newest kept; sending clears it.
 */

const STORE = "myco-drafts";
const MAX = 50;

let drafts: Map<string, string> | null = null;

function all(): Map<string, string> {
  if (drafts) return drafts;
  drafts = new Map();
  try {
    const raw = localStorage.getItem(STORE);
    if (raw) for (const [k, v] of JSON.parse(raw) as [string, string][]) if (typeof v === "string") drafts.set(k, v);
  } catch { /* nothing kept */ }
  return drafts;
}

let saving: number | null = null;
function save() {
  if (saving !== null) return;
  // One write per burst of typing, not one per key.
  saving = window.setTimeout(() => {
    saving = null;
    try { localStorage.setItem(STORE, JSON.stringify([...all()])); } catch { /* not kept */ }
  }, 300);
}

export function draftOf(key: string): string {
  return all().get(key) || "";
}

export function keepDraft(key: string, text: string) {
  const m = all();
  m.delete(key);
  if (text) m.set(key, text);
  while (m.size > MAX) m.delete(m.keys().next().value as string);
  save();
}

/** A chat's draft: one per agent you talk to, on that workspace. */
export const chatDraftKey = (hubId: string, agent: string) => `chat\u0000${hubId}\u0000${agent}`;
/** The group box's draft: one per project (or chat) on that workspace. */
export const groupDraftKey = (hubId: string, project?: string) => `group\u0000${hubId}\u0000${project || "*"}`;
