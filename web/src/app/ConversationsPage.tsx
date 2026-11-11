import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, Pin, PinOff, Plus, Search } from "lucide-react";
import { IconChat, IconCollapseLeft, IconExchange, IconNewChat, IconPrompt } from "@/components/icons";
import { appApi, type Conversation, type Hub, type RecentItem } from "@/appapi";
import type { Project } from "@/api";
import type { ProjectRef } from "@/app/ProjectPage";
import { ErrorText } from "@/app/dialogs";
import type { AgentRow } from "@/app/types";
import { groupsOf, isMe, tagsOf } from "@/app/nodes";
import { parseExecResult } from "@/app/Markdown";
import { cn, timeAgo } from "@/lib/utils";
import { summarize } from "@/app/summary";
import { L, locale } from "@/lib/i18n";
import { Busy, RowsSkeleton, TextSkeleton } from "@/app/Skeleton";

/** A row of the recent list, with the hub it comes from. */
export interface ConversationRow {
  hubId: string;
  hubName: string;
  item: RecentItem;
}

const PAGE = 30;
const CONSOLE = "@console:";

type Scope = "all" | "mine";
type StateFilter = "all" | "open" | "failed" | "done";
interface ListFilters { scope: Scope; state: StateFilter; label: string }

const FILTERS_KEY = "myco-recent-filters";
function loadFilters(): ListFilters {
  try {
    const f = JSON.parse(localStorage.getItem(FILTERS_KEY) || "{}");
    return { scope: f.scope === "mine" ? "mine" : "all", state: ["open", "failed", "done"].includes(f.state) ? f.state : "all", label: typeof f.label === "string" ? f.label : "all" };
  } catch {
    return { scope: "all", state: "all", label: "all" };
  }
}

// Pinned rows, newest pin first, kept on this device with the row as last
// seen: a pinned conversation stays on top even when it is older than the
// pages loaded so far.
const PINS_KEY = "myco-conv-pins";
function loadPins(): ConversationRow[] {
  try {
    const v = JSON.parse(localStorage.getItem(PINS_KEY) || "[]");
    return Array.isArray(v) ? v.filter((r) => r && typeof r.hubId === "string" && r.item && typeof r.item.kind === "string") : [];
  } catch {
    return [];
  }
}
function savePins(pins: ConversationRow[]) {
  try { localStorage.setItem(PINS_KEY, JSON.stringify(pins)); } catch { /* not saved */ }
}

/** What the page had when the person opened a row: shown again on return. */
let saved: { key: string; items: ConversationRow[]; more: Record<string, boolean>; scrollTop: number; selected: string } | null = null;

export const rowKey = (r: ConversationRow) =>
  `${r.hubId}\u0000${r.item.kind === "pair" ? pairKey(r.item.other || "", r.item.node || "") : r.item.thread_id}`;
/** The two sides of a pair, in a fixed order. */
export const pairKey = (a: string, b: string) => (a < b ? `${a}\u0000${b}` : `${b}\u0000${a}`);
const timeOf = (r: ConversationRow) => new Date(r.item.updated_at).getTime() || 0;
const byTime = (a: ConversationRow, b: ConversationRow) => timeOf(b) - timeOf(a);

/** A conversation from a hub without the recent list, as a row. */
function fromConversation(c: Conversation): RecentItem {
  const task = c.last.type === "task" ? c.last : undefined;
  let state: RecentItem["state"] = "";
  if (c.last.type === "event" || c.last.status === "failed") state = "failed";
  else if (c.last.type === "reply" || (c.last.type === "task" && c.last.status === "done")) state = "done";
  else if (c.last.type === "task") state = c.last.status === "queued" ? "queued" : "working";
  return { kind: "thread", thread_id: c.thread_id, participants: [c.first.from, c.agent || c.first.to], updated_at: c.updated_at,
    messages: c.messages, first: c.first, last: c.last, task, state };
}

// Shown as a dot on the row's icon (none when done); the label is its tooltip.
function stateTag(s: RecentItem["state"]): { label: string } {
  switch (s) {
    case "queued": return { label: L("排队中", "Queued") };
    case "working": return { label: L("处理中", "Working") };
    case "failed": return { label: L("失败", "Failed") };
    case "done": return { label: L("已完成", "Done") };
  }
  return { label: "" };
}

function matchesState(it: RecentItem, f: StateFilter): boolean {
  if (f === "all") return true;
  if (f === "open") return it.state === "queued" || it.state === "working";
  return it.state === f;
}

export function ConversationsPage({ hubs, hub, rows, now, onOpen, onNew, selectedKey, compact, onCollapse, projects, project, onProject }: {
  hubs: Hub[];
  /** Projects of every hub, for the project filter. */
  projects?: { hubId: string; p: Project }[] | null;
  /** The project filter: null for all conversations. */
  project?: ProjectRef | null;
  onProject?: (p: ProjectRef | null) => void;
  /** The Hub selected in the switcher: "all" or a hub id. */
  hub: string;
  rows: AgentRow[];
  now: number;
  onOpen: (row: ConversationRow) => void;
  onNew: () => void;
  /** Split view: the conversation open on the right (its row key). */
  selectedKey?: string;
  /** Split view: the list is a narrow column. */
  compact?: boolean;
  /** Split view: hides the list (a button in its header). */
  onCollapse?: () => void;
}) {
  const [filters, setFilters] = useState<ListFilters>(loadFilters);
  const [q, setQ] = useState("");
  const [query, setQuery] = useState("");
  const projectKey = project ? `${project.hubId}/${project.name}` : "";
  const listKey = JSON.stringify([hub, filters.scope, filters.label, query, projectKey]);
  const restored = saved && saved.key === listKey ? saved : null;
  const [items, setItems] = useState<ConversationRow[]>(restored?.items ?? []);
  const [selected, setSelected] = useState(restored?.selected ?? "");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [more, setMore] = useState<Record<string, boolean>>(restored?.more ?? {});
  const [moreBusy, setMoreBusy] = useState(false);
  const listRef = useRef<HTMLDivElement>(null);
  const keepView = useRef<number | null>(null);
  const restoreScroll = useRef(restored?.scrollTop ?? 0);
  const itemsRef = useRef(items); itemsRef.current = items;
  const moreRef = useRef(more); moreRef.current = more;
  const selectedRef = useRef(selected); selectedRef.current = selected;
  const keyRef = useRef(listKey);
  const inFlight = useRef(false);
  const listKeyRef = useRef(listKey); listKeyRef.current = listKey;

  const targets = useMemo(
    () => hubs.filter((h) => (hub === "all" || h.id === hub) && h.ok && (!project || h.id === project.hubId)),
    [hubs, hub, project],
  );
  // Hubs in view whatever their state (their rows stay while one is down).
  const inViewIds = hubs.filter((h) => hub === "all" || h.id === hub).map((h) => h.id).join(",");

  useEffect(() => {
    try { localStorage.setItem(FILTERS_KEY, JSON.stringify(filters)); } catch { /* not saved */ }
  }, [filters]);
  useEffect(() => {
    const t = window.setTimeout(() => setQuery(q.trim()), 300);
    return () => window.clearTimeout(t);
  }, [q]);

  const scroller = () => listRef.current?.closest(".app-scroll") as HTMLElement | null;

  // Back from a conversation: where the person was.
  useLayoutEffect(() => {
    const el = scroller();
    if (el && restoreScroll.current) el.scrollTop = restoreScroll.current;
    restoreScroll.current = 0;
    return () => {
      saved = { key: listKeyRef.current, items: itemsRef.current, more: moreRef.current, scrollTop: scroller()?.scrollTop ?? 0, selected: selectedRef.current };
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // New rows on top while the person reads further down: keep what they see.
  useLayoutEffect(() => {
    const el = scroller();
    if (el && keepView.current !== null) el.scrollTop += el.scrollHeight - keepView.current;
    keepView.current = null;
  }, [items]);

  const fetchPage = async (h: Hub, before?: number) => {
    // One row per pair of parties: all their messages, every task, in one place.
    const r = await appApi.recent(h.id, { scope: filters.scope === "mine" ? "mine" : undefined, label: filters.label, q: query || undefined, before, project: project?.name, group: "pair" });
    if (!r.fallback) return { items: r.items, more: r.more };
    if (project) return { items: [], more: false };
    // A hub from before the recent list: its conversations.
    const c = await appApi.conversations(h.id, { q: query || undefined, before });
    return { items: c.conversations.map(fromConversation), more: c.conversations.length >= PAGE };
  };

  // Load, and refresh with the hub list (every few seconds): the first page
  // is merged in; older rows already loaded stay.
  useEffect(() => {
    let cancelled = false;
    const changed = keyRef.current !== listKey;
    // A background refresh while the last one still waits on a slow hub:
    // skip it rather than pile requests up (a new filter always goes).
    if (!changed && inFlight.current) return;
    keyRef.current = listKey;
    if (changed) {
      setItems([]);
      setMore({});
      setSelected("");
      scroller()?.scrollTo({ top: 0 });
    }
    setLoading(true);
    const myKey = listKey;
    const run = async () => {
      inFlight.current = true;
      const results = await Promise.all(targets.map(async (h) => {
        try {
          return { h, ...(await fetchPage(h)), failed: null as unknown };
        } catch (e) {
          return { h, items: [] as RecentItem[], more: false, failed: e };
        }
      }));
      inFlight.current = false;
      // Superseded only by a new filter or hub: a background refresh skipped
      // while this one waited still wants its answer.
      if (cancelled && listKeyRef.current !== myKey) return;
      const fresh: ConversationRow[] = results.flatMap((r) => r.items.map((item) => ({ hubId: r.h.id, hubName: r.h.name, item })));
      setItems((prev) => {
        // Rows of every hub still in view: a hub that is briefly offline, or
        // whose refresh failed this time, keeps its rows until it answers
        // again, instead of the list emptying and filling back.
        const base = changed ? [] : prev.filter((p) => inViewIds.split(",").includes(p.hubId));
        const answered = new Set(results.filter((r) => !r.failed).map((r) => r.h.id));
        const keys = new Set(fresh.map(rowKey));
        const cutoff: Record<string, number> = {};
        for (const r of results) {
          const ts = r.items.map((i) => new Date(i.updated_at).getTime());
          cutoff[r.h.id] = r.more && ts.length ? Math.min(...ts) : -Infinity;
        }
        // Keep older loaded pages; within the first page, the hub's answer wins.
        const kept = base.filter((p) => !answered.has(p.hubId) || (!keys.has(rowKey(p)) && timeOf(p) < (cutoff[p.hubId] ?? -Infinity)));
        const next = [...fresh, ...kept].sort(byTime);
        const el = scroller();
        if (el && el.scrollTop > 0 && prev.length > 0 && next[0] && prev[0] && rowKey(next[0]) !== rowKey(prev[0])) {
          keepView.current = el.scrollHeight;
        }
        return next;
      });
      // After a refresh, older pages already loaded still stand: keep what
      // "more" said for them.
      setMore((prev) => Object.fromEntries(results.map((r) => [r.h.id, r.failed && !changed ? (prev[r.h.id] ?? false) : changed || prev[r.h.id] === undefined ? r.more : prev[r.h.id]])));
      // A refresh that failed while rows are on screen says nothing: the rows
      // stay and the next refresh tries again. Only an empty list explains.
      setError(results.find((r) => r.failed)?.failed ?? null);
      setLoading(false);
    };
    void run();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targets, listKey]);

  const loadMore = async () => {
    const wanted = targets.filter((h) => more[h.id]);
    if (wanted.length === 0) return;
    setMoreBusy(true);
    try {
      const results = await Promise.all(wanted.map(async (h) => {
        const mine = items.filter((i) => i.hubId === h.id);
        const before = mine.length ? Math.min(...mine.map(timeOf)) : undefined;
        return { h, ...(await fetchPage(h, before)) };
      }));
      const fresh = results.flatMap((r) => r.items.map((item) => ({ hubId: r.h.id, hubName: r.h.name, item })));
      setItems((prev) => {
        const seen = new Set(prev.map(rowKey));
        return [...prev, ...fresh.filter((f) => !seen.has(rowKey(f)))].sort(byTime);
      });
      setMore((prev) => ({ ...prev, ...Object.fromEntries(results.map((r) => [r.h.id, r.more])) }));
    } catch (e) {
      setError(e);
    } finally {
      setMoreBusy(false);
    }
  };

  // Groups and tags of the nodes in view (what the node page offers).
  const labelOptions = useMemo(() => {
    const groups = new Set<string>();
    const tags = new Set<string>();
    for (const r of rows) {
      if (hub !== "all" && r.hub.id !== hub) continue;
      groupsOf(r.st.agent).forEach((g) => groups.add(g));
      tagsOf(r.st.agent).forEach((t) => tags.add(t));
    }
    const sort = (x: Set<string>) => [...x].sort((a, b) => a.localeCompare(b, locale()));
    return { groups: sort(groups), tags: sort(tags) };
  }, [rows, hub]);

  const nameOf = (hubId: string, name?: string): string => {
    if (!name) return L("未知节点", "Unknown node");
    if (name.startsWith(CONSOLE)) {
      const h = rows.find((r) => r.hub.id === hubId)?.hub;
      return isMe(name, h) ? L("你", "You") : name.slice(CONSOLE.length);
    }
    const row = rows.find((r) => r.hub.id === hubId && r.st.agent.name === name);
    return row?.st.agent.display_name || name;
  };

  const parties = (r: ConversationRow): string => {
    const it = r.item;
    if (it.kind === "pair") {
      return it.other?.startsWith(CONSOLE)
        ? `${nameOf(r.hubId, it.other)} → ${nameOf(r.hubId, it.node)}`
        : `${nameOf(r.hubId, it.other)} ↔ ${nameOf(r.hubId, it.node)}`;
    }
    const ps = it.participants.length ? it.participants : [it.first.from, it.first.to];
    const head = `${nameOf(r.hubId, ps[0])} → ${nameOf(r.hubId, ps[1])}`;
    return ps.length > 2 ? `${head} +${ps.length - 2}` : head;
  };

  // A person's thread with an exec node reads as a terminal too.
  const isExec = (hubId: string, name: string) =>
    rows.some((r) => r.hub.id === hubId && r.st.agent.name === name && r.st.agent.binding.harness === "exec");
  const shellRow = (r: ConversationRow) =>
    r.item.kind === "pair"
      ? isExec(r.hubId, r.item.node || "") || isExec(r.hubId, r.item.other || "")
      : r.item.participants.some((p) => isExec(r.hubId, p));

  const summaryOf = (row: ConversationRow): string => {
    const it = row.item;
    if (it.kind === "pair" && it.task && shellRow(row)) {
      if (it.task.unreadable) return summarize(it.task);
      const cmd = `$ ${(it.task.body || "").split("\n")[0]}`;
      const res = it.answer && !it.answer.unreadable && it.answer.type === "reply" ? parseExecResult(it.answer.body) : null;
      if (res && typeof res.exit_code === "number") return `${cmd} · exit ${res.exit_code}`;
      if (it.answer?.type === "event") return `${cmd} · ${summarize(it.answer)}`;
      return cmd;
    }
    const last = it.last;
    if (last && !last.unreadable && it.kind !== "pair" && shellRow(row)) {
      if (last.type === "task") return `$ ${(last.body || "").split("\n")[0]}`;
      const res = last.type === "reply" ? parseExecResult(last.body) : null;
      if (res) {
        const out = ((res.stdout || res.stderr || "") as string).trim().split("\n")[0];
        return typeof res.exit_code === "number" ? (out ? `${out} · exit ${res.exit_code}` : `exit ${res.exit_code}`) : out;
      }
    }
    return summarize(it.last);
  };

  const [pins, setPins] = useState<ConversationRow[]>(loadPins);
  const pinKeys = useMemo(() => new Set(pins.map(rowKey)), [pins]);
  const togglePin = (r: ConversationRow) => {
    setPins((ps) => {
      const k = rowKey(r);
      const next = ps.some((p) => rowKey(p) === k) ? ps.filter((p) => rowKey(p) !== k) : [r, ...ps];
      savePins(next);
      return next;
    });
  };
  // Keep the pinned copies up to date with what was loaded.
  useEffect(() => {
    setPins((ps) => {
      let changed = false;
      const next = ps.map((p) => {
        const fresh = items.find((r) => rowKey(r) === rowKey(p));
        if (fresh && (fresh.item.updated_at !== p.item.updated_at || fresh.item.state !== p.item.state)) { changed = true; return fresh; }
        return p;
      });
      if (!changed) return ps;
      savePins(next);
      return next;
    });
  }, [items]);

  // Right-click menu on a row.
  const [menu, setMenu] = useState<{ x: number; y: number; row: ConversationRow } | null>(null);
  useEffect(() => {
    if (!menu) return;
    const close = () => setMenu(null);
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") close(); };
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", onKey);
    window.addEventListener("blur", close);
    window.addEventListener("resize", close);
    document.addEventListener("scroll", close, true);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("blur", close);
      window.removeEventListener("resize", close);
      document.removeEventListener("scroll", close, true);
    };
  }, [menu]);

  const matching = items.filter((r) => matchesState(r.item, filters.state));
  const firstLoad = loading && items.length === 0;
  const canLoadMore = Object.values(more).some(Boolean);
  const filtered = filters.scope !== "all" || filters.state !== "all" || filters.label !== "all" || !!query || !!project;
  const projectTitle = (hubId: string, name?: string) =>
    name ? projects?.find((x) => x.hubId === hubId && x.p.name === name)?.p.title || name : "";
  const projectOptions = (projects || []).filter((x) => (hub === "all" || x.hubId === hub) && (!x.p.archived || `${x.hubId}/${x.p.name}` === projectKey));
  // Unfiltered: pinned rows first (the loaded copy when there is one), then
  // the rest. Filtered: the matches in their usual order, pins marked.
  const inView = (r: ConversationRow) => (hub === "all" || r.hubId === hub) && hubs.some((h) => h.id === r.hubId);
  const shown = filtered
    ? matching
    : [
        ...pins.filter(inView).map((p) => items.find((r) => rowKey(r) === rowKey(p)) ?? p).sort(byTime),
        ...matching.filter((r) => !pinKeys.has(rowKey(r))),
      ];
  const set = (p: Partial<ListFilters>) => setFilters((f) => ({ ...f, ...p }));

  const open = (r: ConversationRow) => {
    setSelected(rowKey(r));
    selectedRef.current = rowKey(r);
    onOpen(r);
  };

  return (
    <div className={cn("app-page", compact && "conv-list-compact")} ref={listRef}>
      {compact ? (
        // The narrow list: a quiet count and two matching icon buttons.
        <div className="conv-list-head">
          <span className="conv-list-count">
            {firstLoad ? <TextSkeleton width={64} /> : L(`${shown.length} 个对话`, `${shown.length} conversation${shown.length === 1 ? "" : "s"}`)}
          </span>
          <button type="button" className="conv-head-icon is-primary" onClick={onNew}
            aria-label={L("新对话", "New conversation")} title={L("新对话", "New conversation")}>
            <IconNewChat />
          </button>
          {onCollapse && (
            <button type="button" className="conv-head-icon" onClick={onCollapse}
              aria-label={L("收起对话列表", "Hide the conversation list")} title={L("收起对话列表", "Hide the conversation list")}>
              <IconCollapseLeft />
            </button>
          )}
        </div>
      ) : (
        <div className="app-page-head">
          <p className="app-summary">
            {firstLoad ? <TextSkeleton width={64} /> : L(`${shown.length} 个对话`, `${shown.length} conversation${shown.length === 1 ? "" : "s"}`)}
          </p>
          <button type="button" className="app-btn app-btn-primary" onClick={onNew}>
            <Plus />{L("新对话", "New conversation")}
          </button>
        </div>
      )}

      <div className="app-toolbar">
        <div className="app-search">
          <Search />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={L("搜索内容或节点", "Search messages or nodes")}
            aria-label={L("搜索对话", "Search conversations")}
          />
        </div>
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.scope !== "all" && "is-set")} value={filters.scope} onChange={(e) => set({ scope: e.target.value as Scope })} aria-label={L("范围", "Scope")}>
            <option value="all">{L("全部可见", "Everything visible")}</option>
            <option value="mine">{L("与我相关", "Involving me")}</option>
          </select>
          <ChevronDown />
        </span>
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.state !== "all" && "is-set")} value={filters.state} onChange={(e) => set({ state: e.target.value as StateFilter })} aria-label={L("按状态筛选", "Filter by status")}>
            <option value="all">{L("全部状态", "All statuses")}</option>
            <option value="open">{L("进行中", "In progress")}</option>
            <option value="failed">{L("失败", "Failed")}</option>
            <option value="done">{L("已完成", "Done")}</option>
          </select>
          <ChevronDown />
        </span>
        <span className="app-select-wrap">
          <select className={cn("app-select", (filters.label !== "all" || project) && "is-set")} value={project ? `project:${projectKey}` : filters.label}
            onChange={(e) => {
              const v = e.target.value;
              if (v.startsWith("project:")) {
                const k = v.slice(8), i = k.indexOf("/");
                set({ label: "all" });
                onProject?.({ hubId: k.slice(0, i), name: k.slice(i + 1) });
                return;
              }
              onProject?.(null);
              set({ label: v });
            }} aria-label={L("按项目、分组和标签筛选", "Filter by project, group and tag")}>
            <option value="all">{projectOptions.length ? L("项目和分组", "Projects and groups") : L("分组和标签", "Groups and tags")}</option>
            {projectOptions.length > 0 && (
              <optgroup label={L("项目", "Projects")}>
                {projectOptions.map((x) => <option key={`${x.hubId}/${x.p.name}`} value={`project:${x.hubId}/${x.p.name}`}>{x.p.title || x.p.name}</option>)}
              </optgroup>
            )}
            {labelOptions.groups.length > 0 && (
              <optgroup label={L("分组", "Groups")}>
                {labelOptions.groups.map((g) => <option key={g} value={"group:" + g}>{g}</option>)}
                <option value="group:">{L("未分组", "No group")}</option>
              </optgroup>
            )}
            {labelOptions.tags.length > 0 && (
              <optgroup label={L("标签", "Tags")}>
                {labelOptions.tags.map((t) => <option key={t} value={"tag:" + t}>#{t}</option>)}
              </optgroup>
            )}
          </select>
          <ChevronDown />
        </span>
      </div>

      {/* A refresh that failed while rows are on screen says nothing: the
          rows stay and the next refresh tries again. */}
      {error !== null && shown.length === 0 && (
        <div style={{ marginBottom: 12 }}><ErrorText error={error} /></div>
      )}

      {firstLoad ? (
        <RowsSkeleton rows={6} className="conv-skel-rows" />
      ) : shown.length === 0 ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><IconChat /></div>
          <h2>{filtered ? L("没有匹配的对话", "No matching conversations") : L("还没有对话", "No conversations yet")}</h2>
          <p>
            {filtered
              ? L("换个搜索词或筛选条件试试。", "Try a different search or filter.")
              : L("给节点发一条任务，或者让节点之间互相派活、调用 Shell，往来都会出现在这里。", "Give a node a task, or let nodes hand work to each other and run shell commands: it all shows up here.")}
          </p>
          {!filtered && (
            <div className="app-empty-actions">
              <button type="button" className="app-btn app-btn-primary" onClick={onNew}>
                <Plus />{L("新对话", "New conversation")}
              </button>
            </div>
          )}
        </div>
      ) : (
        <div className="app-list">
          {shown.map((row) => {
            const it = row.item;
            const tag = stateTag(it.state);
            const shell = shellRow(row);
            const Icon = shell ? IconPrompt : it.participants.some((p) => p.startsWith(CONSOLE)) ? IconChat : IconExchange;
            const key = rowKey(row);
            const pinned = pinKeys.has(key);
            return (
              <button
                key={key}
                type="button"
                className={cn("conv-row", pinned && !filtered && "is-pinned", (selectedKey ?? selected) === key && "is-selected")}
                onClick={() => open(row)}
                onContextMenu={(e) => {
                  e.preventDefault();
                  setMenu({ x: Math.min(e.clientX, window.innerWidth - 168), y: Math.min(e.clientY, window.innerHeight - 52), row });
                }}
                title={tag.label || undefined}
              >
                <span className={cn("conv-row-icon", shell && "is-shell")}>
                  <Icon aria-hidden />
                  {it.state !== "done" && tag.label && <span className={cn("conv-row-state", `is-${it.state}`)} role="img" aria-label={tag.label} />}
                </span>
                <span className="conv-row-main">
                  <span className="conv-row-top">
                    <span className="conv-row-title">{parties(row)}</span>
                    {it.project && !project && !compact && <span className="conv-row-project">{projectTitle(row.hubId, it.project)}</span>}
                    {hub === "all" && !compact && <span className="conv-row-hub">{row.hubName}</span>}
                    {pinned && <Pin className="conv-row-pin" aria-label={L("已置顶", "Pinned")} />}
                    <span className="conv-row-time">{timeAgo(it.updated_at, now)}</span>
                  </span>
                  <span className={cn("conv-row-summary", shell && "is-mono")}>{summaryOf(row)}</span>
                </span>
              </button>
            );
          })}
        </div>
      )}

      {canLoadMore && (
        <div className="app-actions" style={{ justifyContent: "center" }}>
          <button type="button" className="app-btn" onClick={() => void loadMore()} disabled={moreBusy}>
            <Busy busy={moreBusy}>{L("加载更多", "Load more")}</Busy>
          </button>
        </div>
      )}

      {menu && (
        <div
          className="conv-menu"
          role="menu"
          style={{ left: menu.x, top: menu.y }}
          onMouseDown={(e) => e.stopPropagation()}
          onContextMenu={(e) => e.preventDefault()}
        >
          <button type="button" role="menuitem" className="conv-menu-item" autoFocus onClick={() => { togglePin(menu.row); setMenu(null); }}>
            {pinKeys.has(rowKey(menu.row)) ? <><PinOff />{L("取消置顶", "Unpin")}</> : <><Pin />{L("置顶", "Pin to top")}</>}
          </button>
        </div>
      )}
    </div>
  );
}
