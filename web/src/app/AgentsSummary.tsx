import { useMemo } from "react";
import { AlertTriangle, Search, Users } from "lucide-react";
import type { HubOverview } from "@/appapi";
import type { State } from "@/api";
import type { AgentRow } from "@/app/types";
import { AgentName, Counters, Labels } from "@/components/agents";
import { HarnessBadge, StateBadge } from "@/components/status";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, timeAgo } from "@/lib/utils";

export interface Filters {
  query: string;
  hub: string;
  machine: string;
  state: State | "all";
}

function StatCell({ label, value, dot, active, onClick }: {
  label: string; value: number; dot?: string; active?: boolean; onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn("group relative flex flex-col gap-2 bg-card px-4 py-3.5 text-left transition-colors hover:bg-muted/40 focus-visible:outline-none", active && "bg-muted/50")}
    >
      <span className="flex items-center gap-2 text-xs text-muted-foreground">
        {dot ? <span className={cn("h-1.5 w-1.5 rounded-full", dot)} /> : <span className="h-1.5 w-1.5 rounded-full border border-foreground/40" />}
        {label}
      </span>
      <span className="text-2xl font-semibold leading-none tracking-tight tabular-nums">{value}</span>
      <span className={cn("absolute inset-x-0 bottom-0 h-0.5 bg-foreground transition-opacity", active ? "opacity-100" : "opacity-0")} />
    </button>
  );
}

function Select({ value, onChange, children, className }: {
  value: string; onChange: (v: string) => void; children: React.ReactNode; className?: string;
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={cn("h-9 rounded-md border border-input bg-card px-2.5 text-sm transition-colors hover:border-foreground/20 focus-visible:border-foreground/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20", className)}
    >
      {children}
    </select>
  );
}

function machineOf(row: AgentRow): string | undefined {
  const l = row.st.agent.labels.find((x) => x.startsWith("machine="));
  return l ? l.slice("machine=".length) : undefined;
}

export function AgentsSummary({ rows, hubs, loading, now, filters, onFilters, onOpen }: {
  rows: AgentRow[];
  hubs: HubOverview[] | undefined;
  loading: boolean;
  now: number;
  filters: Filters;
  onFilters: (f: Filters) => void;
  onOpen: (row: AgentRow) => void;
}) {
  const machines = useMemo(() => {
    const set = new Set<string>();
    rows.forEach((r) => { const m = machineOf(r); if (m) set.add(m); });
    return [...set].sort();
  }, [rows]);

  const counts = useMemo(() => {
    const c = { total: rows.length, working: 0, pending: 0, idle: 0, offline: 0 };
    rows.forEach((r) => { c[r.st.state] += 1; });
    return c;
  }, [rows]);

  const visible = useMemo(() => {
    const q = filters.query.trim().toLowerCase();
    return rows.filter((r) =>
      (filters.hub === "all" || r.hub.id === filters.hub) &&
      (filters.machine === "all" || machineOf(r) === filters.machine) &&
      (filters.state === "all" || r.st.state === filters.state) &&
      (!q || [r.st.agent.name, r.st.agent.display_name, r.st.agent.description]
        .filter(Boolean).some((x) => String(x).toLowerCase().includes(q))),
    );
  }, [rows, filters]);

  const down = (hubs || []).filter((h) => !h.ok);
  const up = (hubs || []).filter((h) => h.ok).length;
  const set = (patch: Partial<Filters>) => onFilters({ ...filters, ...patch });

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Agents across your hubs</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {hubs ? `${up} hub${up === 1 ? "" : "s"} connected${down.length ? ` · ${down.length} unreachable` : ""}.` : "Loading hubs…"}
        </p>
      </div>

      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border bg-border shadow-[0_1px_2px_0_rgb(0_0_0/0.03)] md:grid-cols-5">
        <StatCell label="All agents" value={counts.total} active={filters.state === "all"} onClick={() => set({ state: "all" })} />
        <StatCell label="Working" value={counts.working} dot="bg-working" active={filters.state === "working"} onClick={() => set({ state: "working" })} />
        <StatCell label="Pending" value={counts.pending} dot="bg-pending" active={filters.state === "pending"} onClick={() => set({ state: "pending" })} />
        <StatCell label="Idle" value={counts.idle} dot="bg-idle" active={filters.state === "idle"} onClick={() => set({ state: "idle" })} />
        <StatCell label="Offline" value={counts.offline} dot="bg-offline" active={filters.state === "offline"} onClick={() => set({ state: "offline" })} />
      </div>

      {down.map((h) => (
        <Card key={h.id} className="flex items-start gap-3 border-destructive/30 bg-destructive/5 p-4 text-sm">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
          <div className="min-w-0">
            <div className="font-medium text-destructive">{h.name} is unreachable</div>
            <div className="mt-0.5 break-words text-muted-foreground">{h.error || "The app could not reach this hub."} Other hubs keep updating normally.</div>
          </div>
        </Card>
      ))}

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[220px] flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={filters.query} onChange={(e) => set({ query: e.target.value })} placeholder="Search name, display name or description…" className="pl-9" />
        </div>
        <Select value={filters.hub} onChange={(v) => set({ hub: v })}>
          <option value="all">All hubs</option>
          {(hubs || []).map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
        </Select>
        <Select value={filters.machine} onChange={(v) => set({ machine: v })}>
          <option value="all">All machines</option>
          {machines.map((m) => <option key={m} value={m}>{m}</option>)}
        </Select>
        <Select value={filters.state} onChange={(v) => set({ state: v as Filters["state"] })}>
          <option value="all">All states</option>
          <option value="working">Working</option>
          <option value="pending">Pending</option>
          <option value="idle">Idle</option>
          <option value="offline">Offline</option>
        </Select>
      </div>

      {loading && !hubs ? (
        <Skeleton className="h-72 rounded-xl" />
      ) : visible.length === 0 ? (
        <Card className="flex flex-col items-center justify-center gap-2 p-12 text-center">
          <Users className="h-8 w-8 text-muted-foreground/50" />
          <div className="font-medium">No agents match</div>
          <div className="text-sm text-muted-foreground">{rows.length ? "Try a different search or filter." : "Enroll an agent on one of your hubs to see it here."}</div>
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto scrollbar-thin">
            <table className="w-full text-sm">
              <thead className="border-b bg-muted/40 text-left text-xs text-muted-foreground">
                <tr>
                  {["Agent", "Hub", "Status", "Runtime", "Machine", "Labels", "Tasks", "Last seen"].map((h) => (
                    <th key={h} className="whitespace-nowrap px-4 py-2.5 font-medium">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y">
                {visible.map((r) => (
                  <tr key={`${r.hub.id}/${r.st.agent.id}`} onClick={() => onOpen(r)} className={cn("cursor-pointer transition hover:bg-muted/40", r.st.state === "offline" && "opacity-60")}>
                    <td className="max-w-[240px] px-4 py-3">
                      <AgentName name={r.st.agent.name} displayName={r.st.agent.display_name} className="font-medium" />
                      <div className="truncate text-xs text-muted-foreground">{r.st.agent.status_text || r.st.agent.description}</div>
                    </td>
                    <td className="max-w-[160px] px-4 py-3">
                      <span className="inline-flex items-center gap-1.5 truncate text-xs">
                        <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", r.hub.overview ? "bg-working" : "bg-destructive")} />
                        <span className="truncate">{r.hub.name}</span>
                      </span>
                    </td>
                    <td className="px-4 py-3"><StateBadge state={r.st.state} /></td>
                    <td className="px-4 py-3"><HarnessBadge harness={r.st.agent.binding.harness} /></td>
                    <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{machineOf(r) || "—"}</td>
                    <td className="max-w-[220px] px-4 py-3"><div className="flex flex-wrap gap-1"><Labels labels={r.st.agent.labels} max={3} /></div></td>
                    <td className="px-4 py-3"><Counters a={r.st} /></td>
                    <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">{timeAgo(r.st.agent.last_seen_at, now)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}
