import { useEffect, useMemo } from "react";
import { ver } from "./nodes";
import { AlertTriangle, ChevronDown, Copy, Info, Lock, Plus, Search } from "lucide-react";
import { IconChat, IconNodes } from "@/components/icons";
import { copyText } from "@/app/copy";
import { useContextMenu } from "@/app/ContextMenu";
import type { Hub, HubOverview } from "@/appapi";
import type { State } from "@/api";
import type { AgentRow, Filters, NodeKind } from "@/app/types";
import {
  CLOUD_FILTER, agentKind, cliName, groupsOf, isCloud, machineFilterValue, machineLabel, matchesLabelFilter, nodeIcon, tagsOf,
  parseMachineFilter,
  stateLabel, nodeName } from "@/app/nodes";
import { stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";
import { L, locale } from "@/lib/i18n";
import { RowsSkeleton, TextSkeleton } from "@/app/Skeleton";
import { hubErrorText } from "@/appapi";

function host(url: string) {
  try { return new URL(url).host || url; } catch { return url; }
}

const STATE_OPTIONS: { value: State | "all"; label: string }[] = [
  { value: "all", get label() { return L("全部", "All"); } },
  { value: "working", get label() { return L("工作中", "Working"); } },
  { value: "pending", get label() { return L("有待办", "Pending"); } },
  { value: "idle", get label() { return L("空闲", "Idle"); } },
  { value: "offline", get label() { return L("离线", "Offline"); } },
];

const KIND_OPTIONS: { value: NodeKind | "all"; label: string }[] = [
  { value: "all", get label() { return L("全部", "All"); } },
  { value: "agent", label: "AI" },
  { value: "shell", label: "shell" },
];

function tasksOf(row: AgentRow): string {
  return [
    row.st.in_progress > 0 ? L(`处理中 ${row.st.in_progress}`, `${row.st.in_progress} in progress`) : "",
    row.st.queued > 0 ? L(`待处理 ${row.st.queued}`, `${row.st.queued} queued`) : "",
    row.st.held > 0 ? L(`已挂起 ${row.st.held}`, `${row.st.held} held`) : "",
  ].filter(Boolean).join(" · ");
}

/** One node row: kind icon, name, state · CLI + model · machine. */
export function NodeRowView({ row, highlight, onClick, onContextMenu }: {
  row: AgentRow;
  highlight?: boolean;
  onClick: () => void;
  onContextMenu?: (e: React.MouseEvent) => void;
}) {
  const a = row.st.agent;
  const state = stateMeta[row.st.state];
  const Icon = nodeIcon(a);
  const machine = machineLabel(row);
  const model = a.runtime?.model;
  const tasks = tasksOf(row);
  return (
    <button
      type="button"
      className={cn("app-row", row.st.state === "offline" && "offline", highlight && "is-new")}
      onClick={onClick}
      onContextMenu={onContextMenu}
      title={a.name}
    >
      <span className="app-row-icon"><Icon /></span>
      <span className="app-row-main">
        <span className="app-row-titleline">
          <span className="app-row-title">{nodeName(a, row.hub)}</span>
          {a.visibility === "private" && (
            <span className="app-private" title={L("仅自己：只有主人和主人的 agent 能找到它、给它派活", "Private: only its owner and the owner's agents can find it and give it tasks")}>
              <Lock aria-hidden />{L("仅自己", "Private")}
            </span>
          )}
          {groupsOf(a).map((g) => <span key={"g" + g} className="app-label group">{g}</span>)}
          {tagsOf(a).slice(0, 3).map((t) => <span key={"t" + t} className="app-label">#{t}</span>)}
          {tagsOf(a).length > 3 && <span className="app-label">+{tagsOf(a).length - 3}</span>}
        </span>
        <span className="app-row-sub">
          <span className={cn("dot", stateLabel(row.st, state.label).dot)} />
          <span>{stateLabel(row.st, state.label).label}</span>
          <span className="sep">·</span>
          <span className="txt">{cliName(a)}</span>
          {model && <><span className="sep">·</span><span className="txt mono">{model}</span></>}
          {machine && <><span className="sep">·</span><span className="txt">{machine}</span></>}
          {a.owner && row.hub.me && a.owner_id !== row.hub.me.member?.id && <><span className="sep">·</span><span className="txt">{L(`${a.owner} 的`, `${a.owner}’s`)}</span></>}
        </span>
      </span>
      <span className="app-row-trail">
        {tasks && <span>{tasks}</span>}
        <span className="app-chev">›</span>
      </span>
    </button>
  );
}

export function NodesPage({ rows, hubs, hubMeta, loading, filters, onFilters, highlight, onOpen, onAdd, onCleanup, onMessage }: {
  rows: AgentRow[];
  hubs: HubOverview[] | undefined;
  hubMeta: Hub[];
  loading: boolean;
  filters: Filters;
  onFilters: (f: Filters) => void;
  highlight: { hubId: string; name: string } | null;
  onOpen: (row: AgentRow) => void;
  onAdd: () => void;
  // Opens "clean up offline nodes".
  onCleanup?: () => void;
  /** Starts a conversation with a node (from its right-click menu). */
  onMessage?: (row: AgentRow) => void;
}) {
  const menu = useContextMenu();
  // Machine nodes (`xxx-machine`) and people are not nodes in this App.
  const nodes = useMemo(() => rows.filter((r) => {
    const kind = agentKind(r.st.agent);
    return kind !== "machine" && kind !== "human";
  }), [rows]);

  // Real machines, grouped by hub, for the machine filter.
  const machineGroups = useMemo(() => {
    return (hubs || [])
      .filter((h) => filters.hub === "all" || h.id === filters.hub)
      .map((h) => ({
        hub: h.name,
        machines: (h.overview?.machines || []).map((m) => ({
          value: machineFilterValue(h.id, m.id),
          label: m.name,
        })),
      }))
      .filter((g) => g.machines.length > 0);
  }, [hubs, filters.hub]);
  const machineValues = useMemo(
    () => machineGroups.flatMap((g) => g.machines.map((m) => m.value)),
    [machineGroups],
  );

  // A machine that disappeared from the data must not stay selected.
  useEffect(() => {
    if (filters.machine === "all" || filters.machine === CLOUD_FILTER) return;
    if (!machineValues.includes(filters.machine)) {
      onFilters({ ...filters, machine: "all" });
    }
  }, [machineValues, filters, onFilters]);

  const counts = useMemo(() => {
    const c = { total: 0, working: 0, pending: 0, idle: 0, offline: 0 };
    nodes
      .filter((r) => filters.hub === "all" || r.hub.id === filters.hub)
      .forEach((r) => { c.total += 1; c[r.st.state] += 1; });
    return c;
  }, [nodes, filters.hub]);

  const summary = [
    L(`${counts.total} 个节点`, `${counts.total} node${counts.total === 1 ? "" : "s"}`),
    counts.working ? L(`${counts.working} 工作中`, `${counts.working} working`) : "",
    counts.pending ? L(`${counts.pending} 有待办`, `${counts.pending} pending`) : "",
    counts.idle ? L(`${counts.idle} 空闲`, `${counts.idle} idle`) : "",
    counts.offline ? L(`${counts.offline} 离线`, `${counts.offline} offline`) : "",
  ].filter(Boolean).join(" · ");

  const filtered = useMemo(() => {
    const q = filters.query.trim().toLowerCase();
    const machineFilter = parseMachineFilter(filters.machine);
    return nodes.filter((r) => {
      const a = r.st.agent;
      const machineOk = filters.machine === "all"
        ? true
        : filters.machine === CLOUD_FILTER
          ? isCloud(a)
          : !!machineFilter && r.hub.id === machineFilter.hubId && a.machine_id === machineFilter.machineId;
      return (filters.hub === "all" || r.hub.id === filters.hub) &&
        (filters.kind === "all" || agentKind(a) === filters.kind) &&
        matchesLabelFilter(a, filters.label) &&
        (!filters.owner || filters.owner === "all" ||
          (filters.owner === "mine" ? !!r.hub.me?.member && a.owner_id === r.hub.me.member.id : a.owner === filters.owner)) &&
        machineOk &&
        (filters.state === "all" || r.st.state === filters.state) &&
        (!q || [
          a.name, a.display_name, a.description, cliName(a), a.runtime?.model, machineLabel(r),
          ...a.labels,
        ].filter(Boolean).some((x) => String(x).toLowerCase().includes(q)));
    });
  }, [nodes, filters]);

  const set = (patch: Partial<Filters>) => onFilters({ ...filters, ...patch });

  // People who own nodes in view (hubs with members), for the owner filter.
  const owners = useMemo(() => {
    const s = new Set<string>();
    for (const r of nodes) if (r.hub.me && r.st.agent.owner) s.add(r.st.agent.owner);
    return [...s].sort();
  }, [nodes]);

  // Groups and tags in use (in the hubs in view), for the label filter.
  const labelOptions = useMemo(() => {
    const groups = new Set<string>();
    const tags = new Set<string>();
    for (const r of nodes) {
      if (filters.hub !== "all" && r.hub.id !== filters.hub) continue;
      groupsOf(r.st.agent).forEach((g) => groups.add(g));
      tagsOf(r.st.agent).forEach((t) => tags.add(t));
    }
    const sort = (x: Set<string>) => [...x].sort((a, b) => a.localeCompare(b, locale()));
    return { groups: sort(groups), tags: sort(tags) };
  }, [nodes, filters.hub]);

  const scopedHubs = (hubs || []).filter((h) => filters.hub === "all" || h.id === filters.hub);
  const hasUnreachable = scopedHubs.some((h) => !h.ok);
  const hubScoped = useMemo(
    () => nodes.filter((r) => filters.hub === "all" || r.hub.id === filters.hub),
    [nodes, filters.hub],
  );

  const groups = scopedHubs
    .map((h) => ({ hub: h, meta: hubMeta.find((m) => m.id === h.id), rows: filtered.filter((r) => r.hub.id === h.id) }))
    // A hub picked explicitly stays visible even when it has no agents.
    .filter((g) => filters.hub !== "all" || !g.hub.ok || g.rows.length > 0);

  return (
    <div className="app-page">
      <div className="app-page-head">
        <p className="app-summary">
          {hubs ? summary : <TextSkeleton width={140} />}
          {hubs && counts.offline > 0 && onCleanup && (
            <button type="button" className="app-summary-action" onClick={onCleanup}>{L("清理离线节点", "Clean up offline nodes")}</button>
          )}
        </p>
        <button type="button" className="app-btn app-btn-quiet" onClick={onAdd}>
          <Plus />{L("添加节点", "Add node")}
        </button>
      </div>

      <div className="app-toolbar">
        <div className="app-search">
          <Search />
          <input
            value={filters.query}
            onChange={(e) => set({ query: e.target.value })}
            placeholder={L("搜索节点", "Search nodes")}
          />
        </div>
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.kind !== "all" && "is-set")} value={filters.kind} onChange={(e) => set({ kind: e.target.value as Filters["kind"] })} aria-label={L("按类型筛选", "Filter by type")}>
            {KIND_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.value === "all" ? L("全部类型", "All types") : o.label}</option>)}
          </select>
          <ChevronDown />
        </span>
        {owners.length > 1 && (
          <span className="app-select-wrap">
            <select className={cn("app-select", (filters.owner || "all") !== "all" && "is-set")} value={filters.owner || "all"} onChange={(e) => set({ owner: e.target.value })} aria-label={L("按主人筛选", "Filter by owner")}>
              <option value="all">{L("所有人的", "Everyone’s")}</option>
              <option value="mine">{L("我的", "Mine")}</option>
              {owners.map((o) => <option key={o} value={o}>{L(`${o} 的`, `${o}’s`)}</option>)}
            </select>
            <ChevronDown />
          </span>
        )}
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.state !== "all" && "is-set")} value={filters.state} onChange={(e) => set({ state: e.target.value as Filters["state"] })} aria-label={L("按状态筛选", "Filter by status")}>
            {STATE_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.value === "all" ? L("全部状态", "All statuses") : o.label}</option>)}
          </select>
          <ChevronDown />
        </span>
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.label !== "all" && "is-set")} value={filters.label} onChange={(e) => set({ label: e.target.value })} aria-label={L("按分组和标签筛选", "Filter by group and tag")}>
            <option value="all">{L("分组和标签", "Groups and tags")}</option>
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
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.machine !== "all" && "is-set")} value={filters.machine} onChange={(e) => set({ machine: e.target.value })} aria-label={L("按机器筛选", "Filter by machine")}>
            <option value="all">{L("全部机器", "All machines")}</option>
            <option value={CLOUD_FILTER}>{L("云端", "Cloud")}</option>
            {machineGroups.map((g) => (
              <optgroup key={g.hub} label={g.hub}>
                {g.machines.map((m) => <option key={m.value} value={m.value}>{m.label}</option>)}
              </optgroup>
            ))}
          </select>
          <ChevronDown />
        </span>
      </div>

      {!hubs ? (
        loading && <RowsSkeleton rows={5} />
      ) : !hasUnreachable && hubScoped.length === 0 ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><IconNodes /></div>
          <h2>{L("还没有节点", "No nodes yet")}</h2>
          <p>{L("在 Hub 上接入一个 agent，或者在一台机器上开一个新会话。", "Connect an agent to a Hub, or start a new session on a machine.")}</p>
          <div className="app-empty-actions">
            <button type="button" className="app-btn app-btn-primary" onClick={onAdd}>
              <Plus />{L("添加节点", "Add node")}
            </button>
          </div>
        </div>
      ) : filtered.length === 0 && !hasUnreachable ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><IconNodes /></div>
          <h2>{L("没有匹配的节点", "No matching nodes")}</h2>
          <p>{L("换一个搜索或筛选条件试试。", "Try a different search or filter.")}</p>
        </div>
      ) : (<>
        {/* Nothing matches, and an unreachable hub is still listed below. */}
        {filters.hub === "all" && filtered.length === 0 && hubScoped.length > 0 && (
          <div className="app-empty" style={{ margin: "6vh auto 6vh" }}>
            <div className="app-empty-icon"><IconNodes /></div>
            <h2>{L("没有匹配的节点", "No matching nodes")}</h2>
            <p>{L("换一个搜索或筛选条件试试。", "Try a different search or filter.")}</p>
          </div>
        )}
        {groups.map((g) => (
          <section className="app-group" key={g.hub.id}>
            <div className="app-group-title">
              <span className="name">{g.hub.name}</span>
              <span className="parts">
                {g.meta && <><span className="sep">·</span><span className="part mono">{host(g.meta.url)}</span></>}
                {g.meta?.hub_version && <><span className="sep">·</span><span className="part">{ver(g.meta.hub_version)}</span></>}
                {g.meta?.update_available && <span className="app-update-pill">{L("可更新", "Update available")}</span>}
                {!g.hub.ok && <><span className="sep">·</span><span className="part" style={{ color: "var(--red)" }}>{L("连不上", "Unreachable")}</span></>}
                {g.hub.stale && <><span className="sep">·</span><span className="part app-stale" title={g.hub.stale.error || undefined}>{L(`离线 · 显示的是 ${timeAgo(g.hub.stale.at)}的数据`, `Offline · showing data from ${timeAgo(g.hub.stale.at)}`)}</span></>}
              </span>
            </div>
            <div className="app-list">
              {!g.hub.ok ? (
                <div className="app-error-row">
                  <AlertTriangle />
                  <span className="err" title={g.hub.error || undefined}>{hubErrorText(g.hub.error)}</span>
                </div>
              ) : g.rows.length === 0 ? (
                <div className="app-empty-note">
                  {hubScoped.some((r) => r.hub.id === g.hub.id)
                    ? L("没有匹配的节点，换一个搜索或筛选条件试试。", "No matching nodes. Try a different search or filter.")
                    : L("这个 Hub 上还没有节点。", "No nodes on this Hub yet.")}
                </div>
              ) : (
                g.rows.map((r) => (
                  <NodeRowView
                    key={r.st.agent.id}
                    row={r}
                    highlight={highlight?.hubId === r.hub.id && highlight.name === r.st.agent.name}
                    onClick={() => onOpen(r)}
                    onContextMenu={(e) => menu.open(e, [
                      { label: L("查看详情", "Details"), icon: Info, onSelect: () => onOpen(r) },
                      ...(onMessage ? [{ label: L("发消息", "Message"), icon: IconChat, onSelect: () => onMessage(r) }] : []),
                      "sep",
                      { label: L("复制节点名", "Copy node name"), icon: Copy, onSelect: () => void copyText(r.st.agent.name) },
                    ])}
                  />
                ))
              )}
            </div>
          </section>
        ))}
      </>)}
      {menu.element}
    </div>
  );
}
