/*
 * The chats you opened last (yours with an agent or a computer, in any
 * workspace), newest first, kept on this computer: ⌘K (CommandPalette)
 * lists them first, to go back to one in a keystroke or two.
 */

export type RecentChat = { hubId: string; agent: string; at: number };

const KEY = "myco-recent-chats";
const MAX = 12;

function load(): RecentChat[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) || "[]");
    return Array.isArray(v) ? v.filter((x) => x && typeof x.hubId === "string" && typeof x.agent === "string") : [];
  } catch {
    return [];
  }
}

/** Notes that the chat with agent (on hubId) was opened now. */
export function noteChat(hubId: string, agent: string) {
  if (!hubId || !agent) return;
  const list = [{ hubId, agent, at: Date.now() }, ...load().filter((x) => x.hubId !== hubId || x.agent !== agent)].slice(0, MAX);
  try { localStorage.setItem(KEY, JSON.stringify(list)); } catch { /* not kept */ }
}

/** The chats opened last, newest first. */
export function recentChats(): RecentChat[] {
  return load();
}
