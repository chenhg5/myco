import { localPreferences } from "@/lib/localPreferences";
import { useEffect, useState } from "react";
import { L } from "@/lib/i18n";

/*
 * 侧栏显示: which entries the sidebar shows. Every one shows unless the
 * person hides it (Settings › Appearance, or right-click an entry); kept on
 * this computer like the theme, applied at once in every window of the App.
 * A hidden page is still reachable (search, links, Settings): only its
 * entry in the sidebar goes.
 */
export type NavEntry = "automations" | "workflows" | "templates" | "computers" | "projects" | "chats" | "terminals";

export const NAV_ENTRIES: NavEntry[] = ["automations", "workflows", "templates", "computers", "projects", "chats", "terminals"];

export function navEntryLabel(e: NavEntry): string {
  switch (e) {
    case "automations": return L("自动化", "Automations");
    case "workflows": return L("工作流", "Workflows");
    case "templates": return L("应用", "Apps");
    case "computers": return L("电脑", "Computers");
    case "projects": return L("项目", "Projects");
    case "chats": return L("会话", "Conversations");
    case "terminals": return L("终端", "Terminals");
  }
}

const KEY = "myco-nav-hidden";
const EVENT = "myco-nav-hidden";

export function storedNavHidden(): Set<NavEntry> {
  try {
    const raw = JSON.parse(localPreferences.getItem(KEY) || "[]");
    return new Set((Array.isArray(raw) ? raw : []).filter((x): x is NavEntry => NAV_ENTRIES.includes(x)));
  } catch {
    return new Set();
  }
}

/** The entries hidden now, and a way to show or hide one. */
export function useNavHidden() {
  const [hidden, setHidden] = useState<Set<NavEntry>>(storedNavHidden);
  useEffect(() => {
    const on = () => setHidden(storedNavHidden());
    window.addEventListener(EVENT, on);
    window.addEventListener("storage", on);
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener("storage", on); };
  }, []);
  const set = (entry: NavEntry, hide: boolean) => {
    const next = storedNavHidden();
    if (hide) next.add(entry); else next.delete(entry);
    try { localPreferences.setItem(KEY, JSON.stringify([...next])); } catch { /* not kept */ }
    setHidden(next);
    window.dispatchEvent(new Event(EVENT));
  };
  return [hidden, set] as const;
}
