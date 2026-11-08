import { useEffect, useMemo } from "react";
import { AlertTriangle, ChevronDown, Search, Server } from "lucide-react";
import type { Hub, HubOverview } from "@/appapi";
import type { State } from "@/api";
import type { AgentRow } from "@/app/types";
import { harnessInfo, stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";
import type { Filters } from "@/app/types";

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

export function machineOf(row: AgentRow): string | undefined {
  const l = row.st.agent.labels.find((x) => x.startsWith("machine="));
  return l ? l.slice("machine=".length) : undefined;
}

function tasksOf(row: AgentRow): string {
  return [
    row.st.in_progress > 0 ? `处理中 ${row.st.in_progress}` : "",
    row.st.queued > 0 ? `待处理 ${row.st.queued}` : "",
    row.st.held > 0 ? `已挂起 ${row.st.held}` : "",
  ].filter(Boolean).join(" · ");
}

export function AgentRowView({ row, now, onClick, showHub }: {
  row: AgentRow;
  now: number;
  onClick: () => void;
  /** Show the hub name in the subtitle instead of the machine (machines page). */
  showHub?: boolean;
}) {
  const ag = row.st.agent;
  const state = stateMeta[row.st.state];
  const Icon = harnessInfo(ag.binding.harness).icon;
  const where = showHub ? row.hub.name : machineOf(row);
  const tasks = tasksOf(row);
  return (
    <button
      type="button"
      className={cn("app-row", row.st.state === "offline" && "offline")}
      onClick={onClick}
      title={ag.display_name || ag.name}
    >
      <span className="app-row-icon"><Icon strokeWidth={1.75} /></span>
      <span className="app-row-main">
        <span className="app-row-title">{ag.display_name || ag.name}</span>
        <span className="app-row-sub">
          <span className={cn("dot", row.st.state)} />
          <span>{state.label}</span>
          {where && <><span className="sep">·</span><span className="txt">{where}</span></>}
          <span className="sep">·</span>
          <span className="txt">{timeAgo(ag.last_seen_at, now)}</span>
        </span>
      </span>
      <span className="app-row-trail">
        {tasks && <span>{tasks}</span>}
        <span className="app-chev">›</span>
      </span>
    </button>
  );
}

export function NodesPage({ rows, hubs, hubMeta, loading, filters, onFilters, now, onOpen }: {
  rows: AgentRow[];
  hubs: HubOverview[] | undefined;
  hubMeta: Hub[];
  loading: boolean;
  filters: Filters;
  onFilters: (f: Filters) => void;
  now: number;
  onOpen: (row: AgentRow) => void;
}) {
  const machines = useMemo(() => {
    const set = new Set<string>();
    rows.forEach((r) => { const m = machineOf(r); if (m) set.add(m); });
    return [...set].sort();
  }, [rows]);

  // A machine that disappeared from the data must not stay selected.
  useEffect(() => {
    if (filters.machine !== "all" && !machines.includes(filters.machine)) {
      onFilters({ ...filters, machine: "all" });
    }
  }, [machines, filters, onFilters]);

  const counts = useMemo(() => {
    const c = { total: rows.length, working: 0, pending: 0, idle: 0, offline: 0 };
    rows.forEach((r) => { c[r.st.state] += 1; });
    return c;
  }, [rows]);

  const summary = [
    `${counts.total} 个节点`,
    counts.working ? `${counts.working} 工作中` : "",
    counts.pending ? `${counts.pending} 有待办` : "",
    counts.idle ? `${counts.idle} 空闲` : "",
    counts.offline ? `${counts.offline} 离线` : "",
    hubs ? `${hubs.length} 个 Hub` : "",
  ].filter(Boolean).join(" · ");

  const filtered = useMemo(() => {
    const q = filters.query.trim().toLowerCase();
    return rows.filter((r) =>
      (filters.hub === "all" || r.hub.id === filters.hub) &&
      (filters.machine === "all" || machineOf(r) === filters.machine) &&
      (filters.state === "all" || r.st.state === filters.state) &&
      (!q || [r.st.agent.name, r.st.agent.display_name, r.st.agent.description]
        .filter(Boolean).some((x) => String(x).toLowerCase().includes(q))),
    );
  }, [rows, filters]);

  const set = (patch: Partial<Filters>) => onFilters({ ...filters, ...patch });

  const groups = (hubs || [])
    .filter((h) => filters.hub === "all" || h.id === filters.hub)
    .map((h) => ({ hub: h, meta: hubMeta.find((m) => m.id === h.id), rows: filtered.filter((r) => r.hub.id === h.id) }))
    // A hub picked explicitly stays visible even when it has no agents.
    .filter((g) => filters.hub !== "all" || !g.hub.ok || g.rows.length > 0);

  return (
    <div className="app-page">
      <p className="app-summary">{hubs ? summary : "正在加载…"}</p>

      <div className="app-toolbar">
        <div className="app-search">
          <Search />
          <input
            value={filters.query}
            onChange={(e) => set({ query: e.target.value })}
            placeholder="搜索名称、显示名或描述"
          />
        </div>
        <div className="seg seg-sm" role="tablist" aria-label="状态筛选">
          {STATE_OPTIONS.map((o) => (
            <button
              key={o.value}
              type="button"
              role="tab"
              aria-selected={filters.state === o.value}
              className={cn("seg-btn", filters.state === o.value && "active")}
              onClick={() => set({ state: o.value })}
            >
              {o.label}
            </button>
          ))}
        </div>
        <span className="app-select-wrap">
          <select className="app-select" value={filters.hub} onChange={(e) => set({ hub: e.target.value })} aria-label="按 Hub 筛选">
            <option value="all">全部 Hub</option>
            {(hubs || []).map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
          </select>
          <ChevronDown />
        </span>
        <span className="app-select-wrap">
          <select className="app-select" value={filters.machine} onChange={(e) => set({ machine: e.target.value })} aria-label="按机器筛选">
            <option value="all">全部机器</option>
            {machines.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
          <ChevronDown />
        </span>
      </div>

      {!hubs ? (
        loading && <div className="app-empty-note">正在加载…</div>
      ) : groups.length === 0 ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><Server /></div>
          <h2>{rows.length ? "没有匹配的节点" : "还没有节点"}</h2>
          <p>{rows.length ? "换一个搜索或筛选条件试试。" : "在任意 Hub 上接入一个 agent 后，它会出现在这里。"}</p>
        </div>
      ) : (
        groups.map((g) => (
          <section className="app-group" key={g.hub.id}>
            <div className="app-group-title">
              <span className="name">{g.hub.name}</span>
              <span className="parts">
                {g.meta && <><span className="sep">·</span><span className="part mono">{host(g.meta.url)}</span></>}
                {g.meta?.hub_version && <><span className="sep">·</span><span className="part">v{g.meta.hub_version}</span></>}
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
                  <AgentRowView key={r.st.agent.id} row={r} now={now} onClick={() => onOpen(r)} />
                ))
              )}
            </div>
          </section>
        ))
      )}
    </div>
  );
}
