import { useMemo } from "react";
import { AlertTriangle, Monitor, Plus } from "lucide-react";
import type { Hub, HubOverview } from "@/appapi";
import { machineSubtitle, osInfo } from "@/app/nodes";
import { cn } from "@/lib/utils";

/** Real machines that run nodes, grouped by hub. Cloud sessions are not machines. */
export function MachinesPage({ hubs, hubsOv, loading, onOpenMachine, onAdd }: {
  hubs: Hub[];
  hubsOv: HubOverview[] | undefined;
  loading: boolean;
  onOpenMachine: (hubId: string, machineId: string) => void;
  onAdd: () => void;
}) {
  const groups = useMemo(() => hubs.map((h) => {
    const ov = hubsOv?.find((x) => x.id === h.id);
    return { hub: h, ov, machines: ov?.overview?.machines || [] };
  }), [hubs, hubsOv]);

  const all = groups.flatMap((g) => g.machines);
  const online = all.filter((m) => m.online).length;
  const summary = hubsOv
    ? [
      `${all.length} 台机器`,
      online ? `${online} 台在线` : "",
      `${hubs.length} 个 Hub`,
    ].filter(Boolean).join(" · ")
    : "正在加载…";

  return (
    <div className="app-page">
      <div className="app-page-head">
        <p className="app-summary">{summary}</p>
        <button type="button" className="app-btn app-btn-primary" onClick={onAdd} disabled={hubs.length === 0}>
          <Plus />添加机器…
        </button>
      </div>

      {!hubsOv ? (
        loading && <div className="app-empty-note">正在加载…</div>
      ) : all.length === 0 ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><Monitor /></div>
          <h2>还没有机器</h2>
          <p>机器是运行节点的地方。添加一台机器后，就能在上面开新会话。</p>
          <div className="app-empty-actions">
            <button type="button" className="app-btn app-btn-primary" onClick={onAdd} disabled={hubs.length === 0}>
              <Plus />添加机器…
            </button>
          </div>
        </div>
      ) : (
        groups.map((g) => (
          <section className="app-group" key={g.hub.id}>
            <div className="app-group-title">
              <span className="name">{g.hub.name}</span>
              <span className="parts">
                <span className="sep">·</span>
                <span className="part">{g.machines.length} 台机器</span>
              </span>
            </div>
            <div className="app-list">
              {!g.ov ? (
                <div className="app-empty-note">正在读取这个 Hub 的机器…</div>
              ) : !g.ov.ok ? (
                <div className="app-error-row">
                  <AlertTriangle />
                  <span className="err" title={g.ov.error || undefined}>{g.ov.error || "App 无法连接这个 Hub。"}</span>
                </div>
              ) : g.machines.length === 0 ? (
                <div className="app-empty-note">这个 Hub 还没有上报机器。</div>
              ) : (
                g.machines.map((m) => {
                  const os = osInfo(m.os);
                  const OsIcon = os.icon;
                  return (
                    <button
                      key={m.id}
                      type="button"
                      className={cn("app-row", !m.online && "offline")}
                      onClick={() => onOpenMachine(g.hub.id, m.id)}
                      title={m.hostname}
                    >
                      <span className="app-row-icon"><OsIcon strokeWidth={1.75} /></span>
                      <span className="app-row-main">
                        <span className="app-row-title">{m.name}</span>
                        <span className="app-row-sub">
                          <span className="txt">{machineSubtitle(m)}</span>
                        </span>
                      </span>
                      <span className="app-row-trail">
                        <span
                          className={cn("dot", m.online ? "working" : "offline")}
                          title={m.online ? "在线" : "离线"}
                        />
                        <span className="app-chev">›</span>
                      </span>
                    </button>
                  );
                })
              )}
            </div>
          </section>
        ))
      )}
    </div>
  );
}
