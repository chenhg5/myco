import { localPreferences } from "@/lib/localPreferences";
/*
 * Messages put after the running task (⌘/Ctrl+Enter) on a workspace that
 * cannot hold them itself: kept per conversation, like a draft, so they
 * survive a switch or a restart, and sent when the round ends. A workspace
 * from v0.19.2 holds them and this is not used.
 */

export interface Queued {
  id: string;
  body: string;
  /** files uploaded with the message, to send with it */
  attachments?: string[];
  /** the thread it was written into (so it lands where it was typed) */
  threadId?: string;
  at: number;
}

const STORE = "myco-queued";
const MAX = 50;

let queues: Map<string, Queued[]> | null = null;

function all(): Map<string, Queued[]> {
  if (queues) return queues;
  queues = new Map();
  try {
    const raw = localPreferences.getItem(STORE);
    if (raw) {
      for (const [k, v] of JSON.parse(raw) as [string, Queued[]][]) {
        if (!Array.isArray(v)) continue;
        const list = v.filter((x) => x && typeof x.body === "string" && typeof x.id === "string");
        if (list.length) queues.set(k, list);
      }
    }
  } catch { /* nothing kept */ }
  return queues;
}

let saving: number | null = null;
function save() {
  if (saving !== null) return;
  saving = window.setTimeout(() => {
    saving = null;
    try { localPreferences.setItem(STORE, JSON.stringify([...all()])); } catch { /* not kept */ }
  }, 300);
}

export function queueOf(key: string): Queued[] {
  return all().get(key) || [];
}

export function keepQueue(key: string, items: Queued[]) {
  const m = all();
  m.delete(key);
  if (items.length) m.set(key, items);
  while (m.size > MAX) m.delete(m.keys().next().value as string);
  save();
}

/** A conversation's queued messages: one list per agent on that workspace. */
export const chatQueueKey = (hubId: string, agent: string) => `chat\u0000${hubId}\u0000${agent}`;

/** An id for a queued message kept here (not on the hub). */
export function queuedId(): string {
  const c = globalThis.crypto;
  if (c && "randomUUID" in c) return c.randomUUID();
  return `q-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}
