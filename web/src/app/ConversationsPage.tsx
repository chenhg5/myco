import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { recall, remember } from "@/lib/memo";
import { Copy, Info, ListFilter, Pin, PinOff, Plus, Search, X } from "lucide-react";
import { IconChat, IconCollapseLeft, IconExchange, IconNewChat, IconPrompt } from "@/components/icons";
import { copyText } from "@/app/copy";
import { useContextMenu, type MenuItem } from "@/app/ContextMenu";
import { appApi, type Conversation, type Hub, type RecentItem } from "@/appapi";
import type { Project } from "@/api";
import type { ProjectRef } from "@/app/ProjectPage";
import { ErrorText } from "@/app/dialogs";
import type { AgentRow } from "@/app/types";
import { groupsOf, isMe, possiblyStuck, tagsOf, nodeName } from "@/app/nodes";
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
type Side = "me" | "others";
interface ListFilters { scope: Scope; state: StateFilter; label: string; side: Side }

const FILTERS_KEY = "myco-recent-filters";
function loadFilters(): ListFilters {
  try {
    const f = JSON.parse(localStorage.getItem(FILTERS_KEY) || "{}");
    return { scope: f.scope === "mine" ? "mine" : "all", state: ["open", "failed", "done"].includes(f.state) ? f.state : "all", label: typeof f.label === "string" ? f.label : "all",
      side: f.side === "others" ? "others" : "me" };
  } catch {
    return { scope: "all", state: "all", label: "all", side: "me" };
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
// The list as last seen, per filter (hub, side, label, search, project):
// back to it, or to a filter used before, shows it at once.
type SavedList = { items: ConversationRow[]; more: Record<string, boolean>; scrollTop: number; selected: string };
const savedKey = (listKey: string) => "convlist\u0000" + listKey;

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
  // Only a task the node acknowledged is work: queued, pulled-but-not-acked
  // (delivered) and held all wait.
  else if (c.last.type === "task") state = c.last.status === "queued" || c.last.status === "delivered" || c.last.status === "held" ? "queued" : "working";
  return { kind: "thread", thread_id: c.thread_id, participants: [c.first.from, c.agent || c.first.to], updated_at: c.updated_at,
    messages: c.messages, first: c.first, last: c.last, task, state };
}

// Shown as a dot on the row's icon (none when done); the label is its tooltip.
function stateTag(s: RecentItem["state"], stuck = false): { label: string } {
  if (stuck) return { label: L("可能卡住", "May be stuck") };
  switch (s) {
    case "queued": return { label: L("排队中", "Queued") };
    case "working": return { label: L("处理中", "Working") };
    case "failed": return { label: L("失败", "Failed") };
    case "done": return { label: L("已完成", "Done") };
  }
  return { label: "" };
}

function matchesState(it: RecentItem, f: StateFilter, stuck = false): boolean {
  if (f === "all") return true;
  // A task that may have lost its result is not in progress any more.
  if (stuck) return false;
  if (f === "open") return it.state === "queued" || it.state === "working";
  return it.state === f;
}

export function ConversationsPage({ hubs, hub, rows, now, onOpen, onNew, onInspect, selectedKey, compact, onCollapse, projects, project, onProject }: {
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
  /** Opens a node's details (from a row's right-click menu). */
  onInspect?: (hubId: string, name: string) => void;
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
  // A project's conversations are mostly between nodes: no split there.
  const side: Side | undefined = project ? undefined : filters.side;
  const listKey = JSON.stringify([hub, filters.scope, filters.label, query, projectKey, side]);
  const restored = recall<SavedList>(savedKey(listKey)) ?? null;
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
      remember<SavedList>(savedKey(listKeyRef.current), { items: itemsRef.current, more: moreRef.current, scrollTop: scroller()?.scrollTop ?? 0, selected: selectedRef.current });
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // Keep the rows for this filter as they change (not only when leaving).
  useEffect(() => {
    if (items.length === 0 && Object.keys(more).length === 0) return;
    remember<SavedList>(savedKey(listKey), { items, more, scrollTop: 0, selected: selectedRef.current });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items, more]);
  // New rows on top while the person reads further down: keep what they see.
  useLayoutEffect(() => {
    const el = scroller();
    if (el && keepView.current !== null) el.scrollTop += el.scrollHeight - keepView.current;
    keepView.current = null;
  }, [items]);

  const fetchPage = async (h: Hub, before?: number) => {
    // One row per pair of parties: all their messages, every task, in one place.
    const r = await appApi.recent(h.id, { scope: side !== "me" && filters.scope === "mine" ? "mine" : undefined, label: filters.label, q: query || undefined, before, project: project?.name, group: "pair", side });
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
      // This filter's rows from before, if any, while it loads.
      const before = recall<SavedList>(savedKey(listKey));
      setItems(before?.items ?? []);
      setMore(before?.more ?? {});
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
    return row ? nodeName(row.st.agent, row.hub) : name;
  };

  const parties = (r: ConversationRow): string => {
    const it = r.item;
    // Under "mine" every row is you and a node, so the row is just the node.
    if (side === "me") {
      if (it.kind === "pair" && it.other?.startsWith(CONSOLE) && it.node) return nameOf(r.hubId, it.node);
      const nodes = it.kind === "pair" ? [] : it.participants.filter((p) => !p.startsWith("@"));
      if (nodes.length) return nodes.map((n) => nameOf(r.hubId, n)).join(L("、", ", "));
    }
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
  const menu = useContextMenu();
  const rowMenu = (row: ConversationRow): MenuItem[] => {
    const it = row.item;
    // The node you talk to in this conversation (as when it is opened).
    const node = it.kind === "pair"
      ? (it.other?.startsWith(CONSOLE) ? it.node : it.other) || ""
      : it.participants.find((p) => !p.startsWith("@")) || "";
    const pinned = pinKeys.has(rowKey(row));
    return [
      { label: L("打开", "Open"), icon: IconChat, onSelect: () => open(row) },
      pinned
        ? { label: L("取消置顶", "Unpin"), icon: PinOff, onSelect: () => togglePin(row) }
        : { label: L("置顶", "Pin to top"), icon: Pin, onSelect: () => togglePin(row) },
      ...(node ? [
        "sep" as const,
        ...(onInspect ? [{ label: L("查看节点详情", "Node details"), icon: Info, onSelect: () => onInspect(row.hubId, node) }] : []),
        { label: L("复制节点名", "Copy node name"), icon: Copy, onSelect: () => void copyText(node) },
      ] : []),
    ];
  };

  // Yours: you (any of your devices) are one side. Hubs before v0.18.32
  // ignore side, so it is checked here too.
  const isMine = (r: ConversationRow) => {
    const h = rows.find((x) => x.hub.id === r.hubId)?.hub;
    const names = r.item.kind === "pair" ? [r.item.other || "", r.item.node || ""] : r.item.participants;
    return names.some((n) => n.startsWith(CONSOLE) && isMe(n, h));
  };
  const onSide = (r: ConversationRow) => !side || (side === "me") === isMine(r);
  // The task the row shows was picked up and has been quiet too long: its
  // result may have been lost, so it is not "in progress" any more.
  const stuckOf = (r: ConversationRow): boolean => {
    // The open task can be older than the newest one (a stuck task behind a
    // newer one that finished). Older hubs send only the newest task.
    const task = r.item.open || (r.item.state === "working" || r.item.state === "queued" ? r.item.task : undefined);
    if (!task) return false;
    const node = rows.find((x) => x.hub.id === r.hubId && x.st.agent.name === task.to);
    return possiblyStuck(task, node
      ? { agent: node.st.agent, receiving: node.st.receiving, offline: node.st.state === "offline" }
      : undefined, now);
  };
  const matching = items.filter((r) => matchesState(r.item, filters.state, stuckOf(r)) && onSide(r));
  const firstLoad = loading && items.length === 0;
  const canLoadMore = Object.values(more).some(Boolean);
  const filtered = (side !== "me" && filters.scope !== "all") || filters.state !== "all" || filters.label !== "all" || !!query || !!project;
  const projectTitle = (hubId: string, name?: string) =>
    name ? projects?.find((x) => x.hubId === hubId && x.p.name === name)?.p.title || name : "";
  const projectOptions = (projects || []).filter((x) => (hub === "all" || x.hubId === hub) && (!x.p.archived || `${x.hubId}/${x.p.name}` === projectKey));
  // Unfiltered: pinned rows first (the loaded copy when there is one), then
  // the rest. Filtered: the matches in their usual order, pins marked.
  const inView = (r: ConversationRow) => (hub === "all" || r.hubId === hub) && hubs.some((h) => h.id === r.hubId);
  const shown = filtered
    ? matching
    : [
        ...pins.filter(inView).filter(onSide).map((p) => items.find((r) => rowKey(r) === rowKey(p)) ?? p).sort(byTime),
        ...matching.filter((r) => !pinKeys.has(rowKey(r))),
      ];
  const set = (p: Partial<ListFilters>) => setFilters((f) => ({ ...f, ...p }));

  // Filters: one menu, and a chip for each one that is on.
  const stateLabel: Record<StateFilter, string> = {
    all: L("全部状态", "All statuses"), open: L("进行中", "In progress"), failed: L("失败", "Failed"), done: L("已完成", "Done"),
  };
  const labelText = (v: string) => v === "group:" ? L("未分组", "No group") : v.startsWith("group:") ? v.slice(6) : v.startsWith("tag:") ? "#" + v.slice(4) : v;
  const pickLabel = (v: string) => { onProject?.(null); set({ label: filters.label === v ? "all" : v }); };
  const filterItems = (): MenuItem[] => [
    ...(side !== "me" ? [
      { header: L("范围", "Scope") },
      { label: L("全部可见", "Everything visible"), checked: filters.scope === "all", onSelect: () => set({ scope: "all" }) },
      { label: L("我的节点", "My nodes"), checked: filters.scope === "mine", onSelect: () => set({ scope: "mine" }) },
    ] : []),
    { header: L("状态", "Status") },
    ...(["all", "open", "failed", "done"] as const).map((st) => ({ label: stateLabel[st], checked: filters.state === st, onSelect: () => set({ state: st }) })),
    ...(projectOptions.length > 0 ? [
      { header: L("项目", "Projects") },
      ...projectOptions.map((x) => {
        const on = !!project && project.hubId === x.hubId && project.name === x.p.name;
        return { label: x.p.title || x.p.name, checked: on, onSelect: () => { set({ label: "all" }); onProject?.(on ? null : { hubId: x.hubId, name: x.p.name }); } };
      }),
    ] : []),
    ...(labelOptions.groups.length > 0 ? [
      { header: L("分组", "Groups") },
      ...labelOptions.groups.map((g) => ({ label: g, checked: filters.label === "group:" + g, onSelect: () => pickLabel("group:" + g) })),
      { label: L("未分组", "No group"), checked: filters.label === "group:", onSelect: () => pickLabel("group:") },
    ] : []),
    ...(labelOptions.tags.length > 0 ? [
      { header: L("标签", "Tags") },
      ...labelOptions.tags.map((t) => ({ label: "#" + t, checked: filters.label === "tag:" + t, onSelect: () => pickLabel("tag:" + t) })),
    ] : []),
  ];
  const chips: { key: string; label: string; clear: () => void }[] = [
    ...(side !== "me" && filters.scope === "mine" ? [{ key: "scope", label: L("我的节点", "My nodes"), clear: () => set({ scope: "all" }) }] : []),
    ...(filters.state !== "all" ? [{ key: "state", label: stateLabel[filters.state], clear: () => set({ state: "all" }) }] : []),
    ...(project ? [{ key: "project", label: projectOptions.find((x) => x.hubId === project.hubId && x.p.name === project.name)?.p.title || project.name, clear: () => onProject?.(null) }] : []),
    ...(filters.label !== "all" ? [{ key: "label", label: labelText(filters.label), clear: () => set({ label: "all" }) }] : []),
  ];

  const open = (r: ConversationRow) => {
    setSelected(rowKey(r));
    selectedRef.current = rowKey(r);
    onOpen(r);
  };

  return (
    <div className={cn("app-page", compact && "conv-list-compact")} ref={listRef}>
      {/* One line: which conversations, then what you can do. The filters
          live behind one button next to the search. */}
      <div className="conv-list-head">
        {project ? (
          <span className="conv-list-count">
            {firstLoad ? <TextSkeleton width={64} /> : L(`${shown.length} 个对话`, `${shown.length} conversation${shown.length === 1 ? "" : "s"}`)}
          </span>
        ) : (
          <div className="seg conv-side" role="tablist" aria-label={L("对话范围", "Which conversations")}>
            <button type="button" role="tab" aria-selected={filters.side === "me"} className={cn("seg-btn", filters.side === "me" && "active")} onClick={() => set({ side: "me" })}>
              {L("我的对话", "Mine")}
            </button>
            <button type="button" role="tab" aria-selected={filters.side === "others"} className={cn("seg-btn", filters.side === "others" && "active")} onClick={() => set({ side: "others" })}
              title={L("节点之间、以及其他人和节点的往来", "Between nodes, and other people with nodes")}>
              {L("其他往来", "Others")}
            </button>
          </div>
        )}
        <span className="conv-list-head-gap" />
        {compact ? (
          <button type="button" className="conv-head-icon is-primary" onClick={onNew}
            aria-label={L("新对话", "New conversation")} title={L("新对话", "New conversation")}>
            <IconNewChat />
          </button>
        ) : (
          <button type="button" className="app-btn app-btn-quiet" onClick={onNew}>
            <Plus />{L("新对话", "New conversation")}
          </button>
        )}
        {compact && onCollapse && (
          <button type="button" className="conv-head-icon" onClick={onCollapse}
            aria-label={L("收起对话列表", "Hide the conversation list")} title={L("收起对话列表", "Hide the conversation list")}>
            <IconCollapseLeft />
          </button>
        )}
      </div>

      <div className="app-toolbar conv-list-search">
        <div className="app-search">
          <Search />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={L("搜索内容或节点", "Search messages or nodes")}
            aria-label={L("搜索对话", "Search conversations")}
          />
        </div>
        <button
          type="button"
          className={cn("conv-head-icon conv-filter-btn", chips.length > 0 && "is-active")}
          aria-haspopup="menu"
          aria-label={L("筛选", "Filter")}
          title={L("筛选", "Filter")}
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => menu.openUnder(e.currentTarget, filterItems())}
        >
          <ListFilter />
        </button>
      </div>
      {chips.length > 0 && (
        <div className="conv-filter-chips">
          {chips.map((c) => (
            <button key={c.key} type="button" className="conv-filter-chip" onClick={c.clear} title={L("去掉这个筛选", "Remove this filter")}>
              {c.label}<X />
            </button>
          ))}
        </div>
      )}

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
          <h2>{filtered ? L("没有匹配的对话", "No matching conversations") : side === "others" ? L("还没有其他往来", "Nothing here yet") : L("还没有对话", "No conversations yet")}</h2>
          <p>
            {filtered
              ? L("换个搜索词或筛选条件试试。", "Try a different search or filter.")
              : side === "others"
                ? L("节点之间互相派活、调用 Shell，以及其他人和节点的往来，会出现在这里。平时不用看，排查问题时来翻。", "Nodes handing each other work, running shell commands, and other people's exchanges with nodes show up here. Look here when tracking something down.")
                : L("给你的 agent 或 Shell 节点发一条消息，你们的对话会出现在这里，每个节点一段。", "Message one of your agents or shell nodes: each conversation shows up here, one per node.")}
          </p>
          {!filtered && side !== "others" && (
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
            const stuck = stuckOf(row);
            const tag = stateTag(it.state, stuck);
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
                onContextMenu={(e) => menu.open(e, rowMenu(row))}
                // Which hub it is on matters only with all hubs in view, and
                // then only now and then: it is in the tooltip, and in the
                // conversation's own header once opened.
                title={[tag.label, hub === "all" ? row.hubName : ""].filter(Boolean).join(" · ") || undefined}
              >
                <span className={cn("conv-row-icon", shell && "is-shell")}>
                  <Icon aria-hidden />
                  {it.state !== "done" && tag.label && <span className={cn("conv-row-state", stuck ? "is-stuck" : `is-${it.state}`)} role="img" aria-label={tag.label} />}
                </span>
                <span className="conv-row-main">
                  <span className="conv-row-top">
                    <span className="conv-row-title">{parties(row)}</span>
                    {it.project && !project && !compact && <span className="conv-row-project">{projectTitle(row.hubId, it.project)}</span>}
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

      {menu.element}
    </div>
  );
}
