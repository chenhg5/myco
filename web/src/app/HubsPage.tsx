import type { Hub, HubOverview } from "@/appapi";
import { Plus } from "lucide-react";
import { agentKind } from "@/app/nodes";
import { cn } from "@/lib/utils";

function host(url: string) {
  try { return new URL(url).host || url; } catch { return url; }
}

export function HubsPage({ hubs, hubsOv, onAdd, onRemove, onOpenHub }: {
  hubs: Hub[];
  hubsOv: HubOverview[] | undefined;
  onAdd: () => void;
  onRemove: (hub: Hub) => void;
  onOpenHub: (hub: Hub) => void;
}) {
  // Machine nodes (`xxx-machine`) are not nodes.
  const agentCount = (id: string) => hubsOv
    ?.find((h) => h.id === id)
    ?.overview?.agents.filter((st) => agentKind(st.agent) !== "machine").length;
  return (
    <div className="app-page">
      <p className="app-summary">已添加 {hubs.length} 个 Hub</p>
      <div className="app-list">
        {hubs.map((h) => {
          const n = agentCount(h.id);
          const online = h.ok;
          return (
            <div
              key={h.id}
              className="app-row"
              role="button"
              tabIndex={0}
              onClick={() => onOpenHub(h)}
              onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onOpenHub(h); } }}
            >
              <span className={cn("dot", online ? "working" : "offline")} style={online ? undefined : { background: "var(--red)" }} />
              <span className="app-row-main">
                <span className="app-row-title">{h.name}</span>
                <span className="app-row-sub">
                  <span className="txt mono">{host(h.url)}</span>
                  {h.hub_version && <><span className="sep">·</span><span className="txt">v{h.hub_version}</span></>}
                  {h.update_available && <span className="app-update-pill">可更新</span>}
                  <span className="sep">·</span>
                  {online
                    ? <span className="txt">{n === undefined ? "已连接" : `${n} 个节点`}</span>
                    : <span className="txt" style={{ color: "var(--red)" }}>连不上</span>}
                </span>
              </span>
              <span className="app-row-trail">
                <button
                  type="button"
                  className="app-link-danger"
                  onClick={(e) => { e.stopPropagation(); onRemove(h); }}
                  onKeyDown={(e) => e.stopPropagation()}
                >
                  移除
                </button>
                <span className="app-chev">›</span>
              </span>
            </div>
          );
        })}
        <button type="button" className="app-row app-row-add" onClick={onAdd}>
          <span className="plus"><Plus size={14} /></span>
          <span className="app-row-main"><span className="app-row-title" style={{ color: "var(--muted)", fontWeight: 400 }}>添加 Hub…</span></span>
        </button>
      </div>
    </div>
  );
}
