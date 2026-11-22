/// <reference types="vite/client" />
import zh_start from "./zh/start.md?raw";
import en_start from "./en/start.md?raw";
import zh_agents from "./zh/agents.md?raw";
import en_agents from "./en/agents.md?raw";
import zh_chat from "./zh/chat.md?raw";
import en_chat from "./en/chat.md?raw";
import zh_projects from "./zh/projects.md?raw";
import en_projects from "./en/projects.md?raw";
import zh_members from "./zh/members.md?raw";
import en_members from "./en/members.md?raw";
import zh_superagent from "./zh/superagent.md?raw";
import en_superagent from "./en/superagent.md?raw";
import zh_computers from "./zh/computers.md?raw";
import en_computers from "./en/computers.md?raw";
import zh_terminals from "./zh/terminals.md?raw";
import en_terminals from "./en/terminals.md?raw";
import zh_automations from "./zh/automations.md?raw";
import en_automations from "./en/automations.md?raw";
import zh_workflows from "./zh/workflows.md?raw";
import en_workflows from "./en/workflows.md?raw";
import zh_apps from "./zh/apps.md?raw";
import en_apps from "./en/apps.md?raw";
import zh_workspaces from "./zh/workspaces.md?raw";
import en_workspaces from "./en/workspaces.md?raw";
import zh_shortcuts from "./zh/shortcuts.md?raw";
import en_shortcuts from "./en/shortcuts.md?raw";
import zh_faq from "./zh/faq.md?raw";
import en_faq from "./en/faq.md?raw";
import zh_voice from "./zh/voice.md?raw";
import en_voice from "./en/voice.md?raw";
import zh_panes from "./zh/panes.md?raw";
import en_panes from "./en/panes.md?raw";
import zh_public from "./zh/public.md?raw";
import en_public from "./en/public.md?raw";
import zh_proxy from "./zh/proxy.md?raw";
import en_proxy from "./en/proxy.md?raw";
import zh_community from "./zh/community.md?raw";
import en_community from "./en/community.md?raw";
import { L } from "@/lib/i18n";

/*
 * The user guide (使用文档, HelpPage): short pages in Chinese and English,
 * kept as Markdown here (help/zh, help/en) and built into the App, so it
 * reads the same offline and matches this version. A page links another
 * as [text](#help/<id>).
 */

export type HelpId = "start" | "public" | "proxy" | "agents" | "chat" | "projects" | "members" | "superagent" | "computers" | "terminals" | "automations" | "workflows" | "apps" | "workspaces" | "shortcuts" | "faq" | "voice" | "panes" | "community";

type Doc = { id: HelpId; zh: string; en: string };
const DOCS: Doc[] = [
  { id: "start", zh: zh_start, en: en_start },
  { id: "agents", zh: zh_agents, en: en_agents },
  { id: "chat", zh: zh_chat, en: en_chat },
  { id: "projects", zh: zh_projects, en: en_projects },
  { id: "members", zh: zh_members, en: en_members },
  { id: "superagent", zh: zh_superagent, en: en_superagent },
  { id: "computers", zh: zh_computers, en: en_computers },
  { id: "terminals", zh: zh_terminals, en: en_terminals },
  { id: "automations", zh: zh_automations, en: en_automations },
  { id: "workflows", zh: zh_workflows, en: en_workflows },
  { id: "apps", zh: zh_apps, en: en_apps },
  { id: "workspaces", zh: zh_workspaces, en: en_workspaces },
  { id: "shortcuts", zh: zh_shortcuts, en: en_shortcuts },
  { id: "faq", zh: zh_faq, en: en_faq },
  { id: "voice", zh: zh_voice, en: en_voice },
  { id: "panes", zh: zh_panes, en: en_panes },
  { id: "community", zh: zh_community, en: en_community },
  { id: "public", zh: zh_public, en: en_public },
  { id: "proxy", zh: zh_proxy, en: en_proxy },
];

/** The guide's groups, in reading order. */
export const HELP_GROUPS: { title: () => string; ids: HelpId[] }[] = [
  { title: () => L("入门", "Getting started"), ids: ["start", "workspaces", "agents", "chat"] },
  { title: () => L("工作区和网络", "Workspaces and network"), ids: ["public", "proxy", "members"] },
  { title: () => L("一起干活", "Working together"), ids: ["projects", "superagent"] },
  { title: () => L("电脑和终端", "Computers and terminals"), ids: ["computers", "terminals"] },
  { title: () => L("自动化", "Automation"), ids: ["automations", "workflows", "apps"] },
  { title: () => L("用得更顺手", "Getting more out of it"), ids: ["voice", "panes", "shortcuts"] },
  { title: () => L("更多", "More"), ids: ["community", "faq"] },
];

export const HELP_ORDER: HelpId[] = HELP_GROUPS.flatMap((g) => g.ids);

/*
 * Pictures: a page shows a screenshot as ![what it shows](shot:<name>),
 * help/img/<name>.webp (made from the UI test's screenshots by
 * scripts/help-shots.mjs), bundled with the App.
 */
const SHOTS = import.meta.glob("./img/*.webp", { eager: true, query: "?url", import: "default" }) as Record<string, string>;
const shotURL = (name: string) => SHOTS[`./img/${name}.webp`];

/** A page's text in the App's language, its pictures' addresses filled in. */
export function helpText(id: HelpId): string {
  const d = DOCS.find((x) => x.id === id) || DOCS[0];
  return L(d.zh, d.en).replace(/\]\(shot:([\w-]+)\)/g, (all, name: string) => {
    const u = shotURL(name);
    return u ? `](${u})` : all;
  });
}

/** The pictures a page names (for the check that each is there). */
export function helpShots(id: HelpId): { name: string; found: boolean }[] {
  const d = DOCS.find((x) => x.id === id);
  if (!d) return [];
  return [...`${d.zh}\n${d.en}`.matchAll(/\]\(shot:([\w-]+)\)/g)].map((m) => ({ name: m[1], found: !!shotURL(m[1]) }));
}

/** A page's title: its first heading. */
export function helpTitle(id: HelpId): string {
  const m = /^#\s+(.+)$/m.exec(helpText(id));
  return m ? m[1].trim() : id;
}

export function isHelpId(s: unknown): s is HelpId {
  return typeof s === "string" && DOCS.some((d) => d.id === s);
}
