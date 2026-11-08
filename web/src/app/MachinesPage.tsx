import { useMemo } from "react";
import { AlertTriangle, Monitor, Plus } from "lucide-react";
import type { Hub, HubOverview } from "@/appapi";
import { machineSubtitle, osInfo } from "@/app/nodes";
import { cn } from "@/lib/utils";
import { L } from "@/lib/i18n";

/** Real machines that run nodes, grouped by hub. Cloud sessions are not machines. */
export function MachinesPage({ hubs, hubsOv, loading, groupByHub, onOpenMachine, onAdd }: {
  hubs: Hub[];
  hubsOv: HubOverview[] | undefined;
  loading: boolean;
  /** Show each hub as a group; false when the switcher already picked one. */
  groupByHub: boolean;
  onOpenMachine: (hubId: string, machineId: string) => void;
  onAdd: () => void;
}) {
  const groups = useMemo(() => hubs.map((h) => {
    const ov = hubsOv?.find((x) => x.id === h.id);
    return { hub: h, ov, machines: ov?.overview?.machines || [] };
  }), [hubs, hubsOv]);

  const all = groups.flatMap((g) => g.machines);
  const online = all.filter((m) => m.online).length;
  const hasUnreachable = groups.some((g) => g.ov && !g.ov.ok);
  const summary = hubsOv
    ? [
      L(`${all.length} 台机器`, `${all.length} machine${all.length === 1 ? "" : "s"}`),
      online ? L(`${online} 台在线`, `${online} online`) : "",
      L(`${hubs.length} 个 Hub`, `${hubs.length} Hub${hubs.length === 1 ? "" : "s"}`),
    ].filter(Boolean).join(" · ")
    : L("正在加载…", "Loading…");

  return (
    <div className="app-page">
      <div className="app-page-head">
        <p className="app-summary">{summary}</p>
        <button type="button" className="app-btn app-btn-primary" onClick={onAdd} disabled={hubs.length === 0}>
          <Plus />{L("添加机器", "Add machine")}
        </button>
      </div>

      {!hubsOv ? (
        loading && <div className="app-empty-note">{L("正在加载…", "Loading…")}</div>
      ) : all.length === 0 && !hasUnreachable ? (
        <div className="app-empty" style={{ margin: "8vh auto 0" }}>
          <div className="app-empty-icon"><Monitor /></div>
          <h2>{L("还没有机器", "No machines yet")}</h2>
          <p>{L("机器是运行节点的地方。添加一台机器后，就能在上面开新会话。", "Machines are where nodes run. Add a machine to start new sessions on it.")}</p>
          <div className="app-empty-actions">
            <button type="button" className="app-btn app-btn-primary" onClick={onAdd} disabled={hubs.length === 0}>
              <Plus />{L("添加机器", "Add machine")}
            </button>
          </div>
        </div>
      ) : (
        groups.map((g) => (
          <section className="app-group" key={g.hub.id}>
            {groupByHub && (
              <div className="app-group-title">
                <span className="name">{g.hub.name}</span>
                <span className="parts">
                  <span className="sep">·</span>
                  <span className="part">{L(`${g.machines.length} 台机器`, `${g.machines.length} machine${g.machines.length === 1 ? "" : "s"}`)}</span>
                </span>
              </div>
            )}
            <div className="app-list">
              {!g.ov ? (
                <div className="app-empty-note">{L("正在读取这个 Hub 的机器…", "Loading this Hub’s machines…")}</div>
              ) : !g.ov.ok ? (
                <div className="app-error-row">
                  <AlertTriangle />
                  <span className="err" title={g.ov.error || undefined}>{g.ov.error || L("App 无法连接这个 Hub。", "The app can’t connect to this Hub.")}</span>
                </div>
              ) : g.machines.length === 0 ? (
                <div className="app-empty-note">{L("这个 Hub 还没有上报机器。", "This Hub hasn’t reported any machines yet.")}</div>
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
                          title={m.online ? L("在线", "Online") : L("离线", "Offline")}
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
