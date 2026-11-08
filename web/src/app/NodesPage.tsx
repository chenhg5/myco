import { useEffect, useMemo } from "react";
import { ver } from "./nodes";
import { AlertTriangle, ChevronDown, Plus, Search, Server } from "lucide-react";
import type { Hub, HubOverview } from "@/appapi";
import type { State } from "@/api";
import type { AgentRow, Filters, NodeKind } from "@/app/types";
import {
  CLOUD_FILTER, agentKind, cliName, groupsOf, isCloud, machineFilterValue, machineLabel, matchesLabelFilter, nodeIcon, tagsOf,
  parseMachineFilter,
} from "@/app/nodes";
import { stateMeta } from "@/components/status";
import { cn } from "@/lib/utils";

function host(url: string) {
  try { return new URL(url).host || url; } catch { return url; }
}

const STATE_OPTIONS: { value: State | "all"; label: string }[] = [
  { value: "all", label: "全部" },
  { value: "working", label: "工作中" },
  { value: "pending", label: "有待办" },
  { value: "idle", label: "空闲" },
  { value: "offline", label: "离线" },
];

const KIND_OPTIONS: { value: NodeKind | "all"; label: string }[] = [
  { value: "all", label: "全部" },
  { value: "agent", label: "AI" },
  { value: "shell", label: "shell" },
];

function tasksOf(row: AgentRow): string {
  return [
    row.st.in_progress > 0 ? `处理中 ${row.st.in_progress}` : "",
    row.st.queued > 0 ? `待处理 ${row.st.queued}` : "",
    row.st.held > 0 ? `已挂起 ${row.st.held}` : "",
  ].filter(Boolean).join(" · ");
}

/** One node row: kind icon, name, state · CLI + model · machine. */
export function NodeRowView({ row, highlight, onClick }: {
  row: AgentRow;
  highlight?: boolean;
  onClick: () => void;
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
      title={a.display_name || a.name}
    >
      <span className="app-row-icon"><Icon strokeWidth={1.75} /></span>
      <span className="app-row-main">
        <span className="app-row-titleline">
          <span className="app-row-title">{a.display_name || a.name}</span>
          {groupsOf(a).map((g) => <span key={"g" + g} className="app-label group">{g}</span>)}
          {tagsOf(a).slice(0, 3).map((t) => <span key={"t" + t} className="app-label">#{t}</span>)}
          {tagsOf(a).length > 3 && <span className="app-label">+{tagsOf(a).length - 3}</span>}
        </span>
        <span className="app-row-sub">
          <span className={cn("dot", row.st.state)} />
          <span>{state.label}</span>
          <span className="sep">·</span>
          <span className="txt">{cliName(a)}</span>
          {model && <><span className="sep">·</span><span className="txt mono">{model}</span></>}
          {machine && <><span className="sep">·</span><span className="txt">{machine}</span></>}
        </span>
      </span>
      <span className="app-row-trail">
        {tasks && <span>{tasks}</span>}
        <span className="app-chev">›</span>
      </span>
    </button>
  );
}

export function NodesPage({ rows, hubs, hubMeta, loading, filters, onFilters, highlight, onOpen, onAdd }: {
  rows: AgentRow[];
  hubs: HubOverview[] | undefined;
  hubMeta: Hub[];
  loading: boolean;
  filters: Filters;
  onFilters: (f: Filters) => void;
  highlight: { hubId: string; name: string } | null;
  onOpen: (row: AgentRow) => void;
  onAdd: () => void;
}) {
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
    `${counts.total} 个节点`,
    counts.working ? `${counts.working} 工作中` : "",
    counts.pending ? `${counts.pending} 有待办` : "",
    counts.idle ? `${counts.idle} 空闲` : "",
    counts.offline ? `${counts.offline} 离线` : "",
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
        machineOk &&
        (filters.state === "all" || r.st.state === filters.state) &&
        (!q || [
          a.name, a.display_name, a.description, cliName(a), a.runtime?.model, machineLabel(r),
          ...a.labels,
        ].filter(Boolean).some((x) => String(x).toLowerCase().includes(q)));
    });
  }, [nodes, filters]);

  const set = (patch: Partial<Filters>) => onFilters({ ...filters, ...patch });

  // Groups and tags in use (in the hubs in view), for the label filter.
  const labelOptions = useMemo(() => {
    const groups = new Set<string>();
    const tags = new Set<string>();
    for (const r of nodes) {
      if (filters.hub !== "all" && r.hub.id !== filters.hub) continue;
      groupsOf(r.st.agent).forEach((g) => groups.add(g));
      tagsOf(r.st.agent).forEach((t) => tags.add(t));
    }
    const sort = (x: Set<string>) => [...x].sort((a, b) => a.localeCompare(b, "zh-CN"));
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
        <p className="app-summary">{hubs ? summary : "正在加载…"}</p>
        <button type="button" className="app-btn app-btn-primary" onClick={onAdd}>
          <Plus />添加节点
        </button>
      </div>

      <div className="app-toolbar">
        <div className="app-search">
          <Search />
          <input
            value={filters.query}
            onChange={(e) => set({ query: e.target.value })}
            placeholder="搜索节点"
          />
        </div>
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.kind !== "all" && "is-set")} value={filters.kind} onChange={(e) => set({ kind: e.target.value as Filters["kind"] })} aria-label="按类型筛选">
            {KIND_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.value === "all" ? "全部类型" : o.label}</option>)}
          </select>
          <ChevronDown />
        </span>
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.state !== "all" && "is-set")} value={filters.state} onChange={(e) => set({ state: e.target.value as Filters["state"] })} aria-label="按状态筛选">
            {STATE_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.value === "all" ? "全部状态" : o.label}</option>)}
          </select>
          <ChevronDown />
        </span>
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.label !== "all" && "is-set")} value={filters.label} onChange={(e) => set({ label: e.target.value })} aria-label="按分组和标签筛选">
            <option value="all">分组和标签</option>
            {labelOptions.groups.length > 0 && (
              <optgroup label="分组">
                {labelOptions.groups.map((g) => <option key={g} value={"group:" + g}>{g}</option>)}
                <option value="group:">未分组</option>
              </optgroup>
            )}
            {labelOptions.tags.length > 0 && (
              <optgroup label="标签">
                {labelOptions.tags.map((t) => <option key={t} value={"tag:" + t}>#{t}</option>)}
              </optgroup>
            )}
          </select>
          <ChevronDown />
        </span>
        <span className="app-select-wrap">
          <select className={cn("app-select", filters.machine !== "all" && "is-set")} value={filters.machine} onChange={(e) => set({ machine: e.target.value })} aria-label="按机器筛选">
            <option value="all">全部机器</option>
            <option value={CLOUD_FILTER}>云端</option>
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
        loading && <div className="app-empty-note">正在加载…</div>
      ) : !hasUnreachable && hubScoped.length === 0 ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><Server /></div>
          <h2>还没有节点</h2>
          <p>在 Hub 上接入一个 agent，或者在一台机器上开一个新会话。</p>
          <div className="app-empty-actions">
            <button type="button" className="app-btn app-btn-primary" onClick={onAdd}>
              <Plus />添加节点
            </button>
          </div>
        </div>
      ) : filtered.length === 0 && !hasUnreachable ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><Server /></div>
          <h2>没有匹配的节点</h2>
          <p>换一个搜索或筛选条件试试。</p>
        </div>
      ) : (
        groups.map((g) => (
          <section className="app-group" key={g.hub.id}>
            <div className="app-group-title">
              <span className="name">{g.hub.name}</span>
              <span className="parts">
                {g.meta && <><span className="sep">·</span><span className="part mono">{host(g.meta.url)}</span></>}
                {g.meta?.hub_version && <><span className="sep">·</span><span className="part">{ver(g.meta.hub_version)}</span></>}
                {g.meta?.update_available && <span className="app-update-pill">可更新</span>}
                {!g.hub.ok && <><span className="sep">·</span><span className="part" style={{ color: "var(--red)" }}>连不上</span></>}
              </span>
            </div>
            <div className="app-list">
              {!g.hub.ok ? (
                <div className="app-error-row">
                  <AlertTriangle />
                  <span className="err" title={g.hub.error || undefined}>{g.hub.error || "App 无法连接这个 Hub。"}</span>
                </div>
              ) : g.rows.length === 0 ? (
                <div className="app-empty-note">这个 Hub 上还没有节点。</div>
              ) : (
                g.rows.map((r) => (
                  <NodeRowView
                    key={r.st.agent.id}
                    row={r}
                    highlight={highlight?.hubId === r.hub.id && highlight.name === r.st.agent.name}
                    onClick={() => onOpen(r)}
                  />
                ))
              )}
            </div>
          </section>
        ))
      )}
    </div>
  );
}
