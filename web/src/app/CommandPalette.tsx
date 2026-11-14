import { useEffect, useMemo, useRef, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { FolderKanban, MessageSquare, Monitor, Search, Settings, Users } from "lucide-react";
import { appApi, type HubOverview, type RecentItem } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow } from "@/app/types";
import { isMe, nodeName } from "@/app/nodes";
import { Avatar, isWorkspace } from "@/app/persona";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/*
 * Finding anything (⌘K, Ctrl+K on Windows): one box over the App. What you
 * type is matched against members, projects and computers in every
 * workspace this App has, and looked for in the messages of your own
 * conversations (each workspace's hub searches them). ↑/↓ choose, Enter
 * opens, Esc closes. A message opens its conversation at that message,
 * with the words found there marked.
 */

export type PaletteProject = { hubId: string; name: string; title: string; goal?: string };
export type PaletteTarget =
  | { kind: "member"; row: AgentRow }
  | { kind: "project"; hubId: string; name: string }
  | { kind: "message"; row: AgentRow; message: string; find: string }
  | { kind: "page"; page: "settings" | "members" | "computers" };

type Result = { key: string; group: string; title: string; sub?: string; icon: React.ReactNode; target: PaletteTarget };

/** What a conversation search found, waiting for its chat to open: the
 *  chat marks those words (ConversationDetail). */
let pendingFind: { message: string; q: string } | null = null;
export function takePendingFind(message?: string): string {
  if (!message || pendingFind?.message !== message) return "";
  const q = pendingFind.q;
  pendingFind = null;
  return q;
}

const norm = (s: string) => s.toLocaleLowerCase();

function snippet(body: string, q: string): string {
  const flat = body.replace(/\s+/g, " ").trim();
  const i = norm(flat).indexOf(norm(q));
  if (i < 0) return flat.slice(0, 90);
  const start = Math.max(0, i - 30);
  return (start > 0 ? "…" : "") + flat.slice(start, i + q.length + 60);
}

export function CommandPalette({ open, onOpenChange, rows, hubs, projects, now, onPick }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** every member and computer, in every workspace */
  rows: AgentRow[];
  hubs: HubOverview[];
  projects: PaletteProject[];
  now: number;
  onPick: (t: PaletteTarget) => void;
}) {
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const [found, setFound] = useState<{ q: string; items: { hub: HubOverview; item: RecentItem }[]; busy: boolean }>({ q: "", items: [], busy: false });
  const list = useRef<HTMLDivElement>(null);
  useEffect(() => { if (open) { setQ(""); setSel(0); setFound({ q: "", items: [], busy: false }); } }, [open]);

  // Messages: each workspace's hub, a moment after typing stops.
  useEffect(() => {
    const text = q.trim();
    if (!open || text.length < 2) { setFound({ q: text, items: [], busy: false }); return; }
    setFound((f) => ({ ...f, busy: true }));
    let live = true;
    const t = window.setTimeout(() => {
      Promise.all(hubs.filter((h) => h.ok).map((hub) => appApi.recent(hub.id, { q: text, group: "pair", side: "me" })
        .then((r) => r.items.map((item) => ({ hub, item }))).catch(() => [])))
        .then((all) => {
          if (!live) return;
          const items = all.flat().sort((a, b) => b.item.updated_at.localeCompare(a.item.updated_at)).slice(0, 8);
          setFound({ q: text, items, busy: false });
        });
    }, 250);
    return () => { live = false; window.clearTimeout(t); };
  }, [q, open, hubs]);

  const results = useMemo<Result[]>(() => {
    const text = norm(q.trim());
    const hit = (...xs: (string | undefined)[]) => !text || xs.some((x) => x && norm(x).includes(text));
    const many = hubs.filter((h) => h.ok).length > 1;
    const where = (hubId: string) => (many ? hubs.find((h) => h.id === hubId)?.name : undefined);
    const out: Result[] = [];
    const agents = rows.filter((r) => !isWorkspace(r.st.agent) && r.st.agent.kind !== "machine");
    for (const r of agents.filter((x) => hit(nodeName(x.st.agent, x.hub), x.st.agent.name, x.st.agent.description)).slice(0, text ? 6 : 4)) {
      out.push({
        key: `m:${r.hub.id}:${r.st.agent.name}`, group: L("成员", "Members"), title: nodeName(r.st.agent, r.hub),
        sub: [r.st.agent.description, where(r.hub.id)].filter(Boolean).join(" · "),
        icon: <Avatar agent={r.st.agent} size={22} badge={false} />, target: { kind: "member", row: r },
      });
    }
    for (const p of projects.filter((x) => hit(x.title, x.name, x.goal)).slice(0, text ? 6 : 3)) {
      out.push({
        key: `p:${p.hubId}:${p.name}`, group: L("项目", "Projects"), title: p.title || p.name, sub: [p.goal, where(p.hubId)].filter(Boolean).join(" · "),
        icon: <FolderKanban />, target: { kind: "project", hubId: p.hubId, name: p.name },
      });
    }
    for (const r of rows.filter((x) => isWorkspace(x.st.agent) && hit(nodeName(x.st.agent, x.hub), x.st.agent.name)).slice(0, text ? 4 : 2)) {
      out.push({
        key: `c:${r.hub.id}:${r.st.agent.name}`, group: L("电脑", "Computers"), title: nodeName(r.st.agent, r.hub), sub: where(r.hub.id),
        icon: <Monitor />, target: { kind: "member", row: r },
      });
    }
    if (text && found.q === q.trim()) {
      for (const { hub, item } of found.items) {
        const other = item.participants.find((n) => !isMe(n, hub)) || item.other || "";
        const row = rows.find((r) => r.hub.id === hub.id && r.st.agent.name === other);
        if (!row) continue;
        const msgs = [item.answer, item.task, item.last, item.first, item.open].filter(Boolean) as Message[];
        const m = msgs.find((x) => norm(x.body || "").includes(text)) || item.last;
        out.push({
          key: `t:${hub.id}:${item.thread_id}:${other}`, group: L("消息", "Messages"), title: nodeName(row.st.agent, row.hub),
          sub: `${snippet(m.body || "", q.trim())} · ${timeAgo(item.updated_at, now)}`,
          icon: <MessageSquare />, target: { kind: "message", row, message: m.id, find: q.trim() },
        });
      }
    }
    const pages: { page: "settings" | "members" | "computers"; title: string; icon: React.ReactNode; words: string }[] = [
      { page: "settings", title: L("设置", "Settings"), icon: <Settings />, words: "设置 settings 偏好 外观 翻译 preferences" },
      { page: "members", title: L("所有成员", "All members"), icon: <Users />, words: "成员 members agents" },
      { page: "computers", title: L("所有电脑", "All computers"), icon: <Monitor />, words: "电脑 computers workspaces machines" },
    ];
    for (const p of pages.filter((x) => hit(x.title, x.words))) {
      out.push({ key: `g:${p.page}`, group: L("前往", "Go to"), title: p.title, icon: p.icon, target: { kind: "page", page: p.page } });
    }
    return out;
  }, [q, rows, hubs, projects, found, now]);

  useEffect(() => { setSel(0); }, [q]);
  useEffect(() => {
    list.current?.querySelector<HTMLElement>(`[data-i="${sel}"]`)?.scrollIntoView({ block: "nearest" });
  }, [sel]);

  const pick = (r: Result) => {
    if (r.target.kind === "message") pendingFind = { message: r.target.message, q: r.target.find };
    onOpenChange(false);
    onPick(r.target);
  };
  const onKey = (e: React.KeyboardEvent) => {
    if (e.nativeEvent.isComposing) return;
    if (e.key === "ArrowDown") { e.preventDefault(); setSel((s) => Math.min(results.length - 1, s + 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setSel((s) => Math.max(0, s - 1)); }
    else if (e.key === "Enter" && results[sel]) { e.preventDefault(); pick(results[sel]); }
  };

  let lastGroup = "";
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="app-portal cp-scrim" />
        <Dialog.Content className="app-portal cp" aria-describedby={undefined}>
          <Dialog.Title className="app-sr-only">{L("搜索", "Search")}</Dialog.Title>
          <div className="cp-box">
            <Search />
            <input autoFocus value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onKey}
              placeholder={L("找成员、项目、电脑或消息…", "Find a member, project, computer or message…")} aria-label={L("搜索", "Search")} />
            {found.busy && <span className="cp-busy">{L("正在找消息…", "Searching messages…")}</span>}
          </div>
          <div className="cp-list" ref={list} role="listbox">
            {results.length === 0 && <p className="cp-empty">{found.busy ? L("正在找…", "Searching…") : L("没找到", "Nothing found")}</p>}
            {results.map((r, i) => {
              const head = r.group !== lastGroup ? r.group : "";
              lastGroup = r.group;
              return (
                <div key={r.key}>
                  {head && <div className="cp-group">{head}</div>}
                  <button type="button" role="option" aria-selected={i === sel} data-i={i} className={cn("cp-item", i === sel && "is-sel")}
                    onMouseMove={() => setSel(i)} onClick={() => pick(r)}>
                    <span className="cp-icon">{r.icon}</span>
                    <span className="cp-text"><b>{r.title}</b>{r.sub && <span>{r.sub}</span>}</span>
                  </button>
                </div>
              );
            })}
          </div>
          <div className="cp-foot">{L("↑↓ 选择 · ↩ 打开 · Esc 关闭", "↑↓ choose · ↩ open · Esc close")}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
