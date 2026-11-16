import { useEffect, useSyncExternalStore } from "react";
import { appApi, type HubOverview } from "@/appapi";
import type { Message } from "@/api";
import { isMe } from "@/app/nodes";

/*
 * What you have not read: per workspace, per conversation (yours with an
 * agent, by its name). A conversation is unread when the agent sent you
 * something to read after the newest message you saw there: its answer,
 * its question, a failure, a note (not progress or steps: there would
 * always be a dot). Seeing it (the chat on screen, the App in front, its
 * bottom in view) marks it read. The marks are kept on the workspace's hub
 * (GET/POST /v1/console/reads), per person, so reading on the phone clears
 * the dot here; an older hub keeps them on this computer.
 *
 * Shown as red dots in the sidebar and ⌘K, and as a number on the Mac
 * app's Dock icon (AppMode).
 */

type Marks = Record<string, string>;

const marks = new Map<string, Marks>(); // hub → agent → read up to
const latest = new Map<string, Marks>(); // hub → agent → newest thing to read
const localOnly = new Set<string>(); // hubs that keep no marks
const pending = new Map<string, Marks>(); // hub → marks not sent yet
let version = 0;
const listeners = new Set<() => void>();
const emit = () => { version++; listeners.forEach((l) => l()); };
const subscribe = (cb: () => void) => { listeners.add(cb); return () => { listeners.delete(cb); }; };
const ms = (iso?: string) => (iso ? new Date(iso).getTime() || 0 : 0);

const localKey = (hubId: string) => `myco-reads:${hubId}`;
function loadLocal(hubId: string): Marks {
  try { return JSON.parse(localStorage.getItem(localKey(hubId)) || "{}") || {}; } catch { return {}; }
}
function keepLocal(hubId: string) {
  try { localStorage.setItem(localKey(hubId), JSON.stringify(marks.get(hubId) || {})); } catch { /* not kept */ }
}

/** Whether something in the conversation with agent was not read yet. */
export function isUnread(hubId: string, agent: string): boolean {
  const l = latest.get(hubId)?.[agent];
  return !!l && ms(l) > ms(marks.get(hubId)?.[agent]);
}

/** How many conversations have something unread (every workspace). */
export function unreadCount(): number {
  let n = 0;
  for (const [hubId, l] of latest) for (const agent of Object.keys(l)) if (isUnread(hubId, agent)) n++;
  return n;
}

/** Redraws when what is unread changes. */
export function useUnread(): number {
  return useSyncExternalStore(subscribe, () => version, () => version);
}

const timers = new Map<string, number>();
/** You saw the conversation with agent up to at (its newest message). */
export function markRead(hubId: string, agent: string, at: string) {
  if (!hubId || !agent || !at) return;
  const m = marks.get(hubId) || {};
  if (ms(at) <= ms(m[agent])) return;
  marks.set(hubId, { ...m, [agent]: at });
  keepLocal(hubId);
  emit();
  if (localOnly.has(hubId)) return;
  pending.set(hubId, { ...(pending.get(hubId) || {}), [agent]: at });
  window.clearTimeout(timers.get(hubId));
  timers.set(hubId, window.setTimeout(() => {
    const send = pending.get(hubId);
    pending.delete(hubId);
    if (send) void appApi.markRead(hubId, send).catch(() => { /* kept here; sent with the next */ });
  }, 800));
}

/** What a message in your conversation with an agent asks you to read. */
function toRead(m: Message | undefined, hub: HubOverview): m is Message {
  if (!m || m.from.startsWith("@") || !isMe(m.to, hub)) return false;
  return m.type === "reply" || m.type === "event" || m.type === "note" || (m.type === "progress" && !!(m as { ask?: unknown }).ask);
}

async function poll(hubs: HubOverview[]) {
  await Promise.all(hubs.filter((h) => h.ok).map(async (hub) => {
    // Your marks: here, and on the hub (another device of yours may have
    // read something since).
    const cur = marks.get(hub.id);
    let remote: Marks = {};
    if (!localOnly.has(hub.id)) {
      try {
        remote = (await appApi.reads(hub.id)).reads || {};
      } catch (e) {
        if ((e as { code?: string })?.code === "not_found") localOnly.add(hub.id);
      }
    }
    const merged: Marks = { ...loadLocal(hub.id) };
    for (const src of [cur || {}, remote]) for (const [k, v] of Object.entries(src)) if (ms(v) > ms(merged[k])) merged[k] = v;
    // (never looked before: no marks anywhere)
    const fresh = !cur && !Object.keys(merged).length;
    marks.set(hub.id, merged);
    // The newest thing to read in each of your conversations.
    let items;
    try { items = (await appApi.recent(hub.id, { group: "pair", side: "me" })).items; } catch { return; }
    const next: Marks = {};
    for (const it of items) {
      for (const m of [it.answer, it.last, it.open, it.task]) {
        if (toRead(m, hub) && ms(m.created_at) > ms(next[m.from])) next[m.from] = m.created_at;
      }
    }
    // The first time this App looks (no marks anywhere yet): what was
    // there before counts as read, so it does not start covered in dots.
    if (fresh) {
      marks.set(hub.id, { ...next });
      keepLocal(hub.id);
      if (!localOnly.has(hub.id) && Object.keys(next).length) void appApi.markRead(hub.id, next).catch(() => undefined);
    }
    latest.set(hub.id, next);
  }));
  emit();
}

/** Keeps what is unread up to date (one in the App: AppMode). */
export function useUnreadWatch(hubs: HubOverview[]) {
  const key = hubs.filter((h) => h.ok).map((h) => h.id).join(",");
  useEffect(() => {
    let off = false;
    const tick = () => { if (!off && document.visibilityState === "visible") void poll(hubs); };
    tick();
    const t = window.setInterval(tick, 12_000);
    const onVis = () => tick();
    document.addEventListener("visibilitychange", onVis);
    return () => { off = true; window.clearInterval(t); document.removeEventListener("visibilitychange", onVis); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
}
