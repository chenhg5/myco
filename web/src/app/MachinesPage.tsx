import { useMemo, useState } from "react";
import { Monitor, Search } from "lucide-react";
import type { State } from "@/api";
import type { AgentRow } from "@/app/types";
import { AgentRowView, machineOf } from "@/app/NodesPage";
import { cn } from "@/lib/utils";

const STATE_OPTIONS: { value: State | "all"; label: string }[] = [
  { value: "all", label: "全部" },
  { value: "working", label: "工作中" },
  { value: "pending", label: "有待办" },
  { value: "idle", label: "空闲" },
  { value: "offline", label: "离线" },
];

const UNLABELLED = "未标注机器";

/** The same nodes, grouped by their `machine=` label. */
export function MachinesPage({ rows, now, onOpen }: {
  rows: AgentRow[];
  now: number;
  onOpen: (row: AgentRow) => void;
}) {
  const [query, setQuery] = useState("");
  const [state, setState] = useState<State | "all">("all");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return rows.filter((r) =>
      (state === "all" || r.st.state === state) &&
      (!q || [r.st.agent.name, r.st.agent.display_name, r.st.agent.description, machineOf(r), r.hub.name]
        .filter(Boolean).some((x) => String(x).toLowerCase().includes(q))),
    );
  }, [rows, query, state]);

  const groups = useMemo(() => {
    const map = new Map<string, AgentRow[]>();
    filtered.forEach((r) => {
      const key = machineOf(r) || UNLABELLED;
      const list = map.get(key);
      if (list) list.push(r);
      else map.set(key, [r]);
    });
    return [...map.entries()]
      .sort((a, b) => (a[0] === UNLABELLED ? 1 : b[0] === UNLABELLED ? -1 : a[0].localeCompare(b[0])))
      .map(([machine, list]) => ({ machine, rows: list }));
  }, [filtered]);

  const summary = [
    `${groups.length} 台机器`,
    `${filtered.length} 个节点`,
    rows.filter((r) => r.st.state === "working").length
      ? `${rows.filter((r) => r.st.state === "working").length} 工作中`
      : "",
  ].filter(Boolean).join(" · ");

  return (
    <div className="app-page">
      <p className="app-summary">{summary}</p>

      <div className="app-toolbar">
        <div className="app-search">
          <Search />
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索机器或节点" />
        </div>
        <div className="seg seg-sm" role="tablist" aria-label="状态筛选">
          {STATE_OPTIONS.map((o) => (
            <button
              key={o.value}
              type="button"
              role="tab"
              aria-selected={state === o.value}
              className={cn("seg-btn", state === o.value && "active")}
              onClick={() => setState(o.value)}
            >
              {o.label}
            </button>
          ))}
        </div>
      </div>

      {groups.length === 0 ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><Monitor /></div>
          <h2>{rows.length ? "没有匹配的节点" : "还没有机器"}</h2>
          <p>{rows.length ? "换一个搜索或筛选条件试试。" : "节点接入时会带上 machine= 标签，这里就会按机器分组。"}</p>
        </div>
      ) : (
        groups.map((g) => (
          <section className="app-group" key={g.machine}>
            <div className="app-group-title">
              <span className="name">{g.machine}</span>
              <span className="parts">
                <span className="sep">·</span>
                <span className="part">{g.rows.length} 个节点</span>
              </span>
            </div>
            <div className="app-list">
              {g.rows.map((r) => (
                <AgentRowView key={`${r.hub.id}/${r.st.agent.id}`} row={r} now={now} showHub onClick={() => onOpen(r)} />
              ))}
            </div>
          </section>
        ))
      )}
    </div>
  );
}
