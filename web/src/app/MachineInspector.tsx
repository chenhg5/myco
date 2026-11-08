import { useEffect, useMemo, useState } from "react";
import { ver } from "./nodes";
import { X } from "lucide-react";
import { appApi, type HubOverview } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, cliName, kindLabel, nodeIcon, osInfo } from "@/app/nodes";
import { ErrorText, Modal } from "@/app/dialogs";
import { stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";

export interface MachineRef { hubId: string; machineId: string }

/** Right-hand inspector for one machine: details, rename, its nodes, spawn. */
export function MachineInspector({ target, hubs, rows, now, onClose, onRefresh, onOpenNode, onSpawn }: {
  target: MachineRef | null;
  hubs: HubOverview[] | undefined;
  rows: AgentRow[];
  now: number;
  onClose: () => void;
  onRefresh: () => Promise<void>;
  onOpenNode: (row: AgentRow) => void;
  onSpawn: (hubId: string, machineId: string) => void;
}) {
  const [renameOpen, setRenameOpen] = useState(false);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    setRenameOpen(false);
    setName("");
    setBusy(false);
    setError(null);
  }, [target?.hubId, target?.machineId]);

  // Escape closes it, like the node inspector (not while renaming).
  useEffect(() => {
    if (!target) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape" && !renameOpen) onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [target, renameOpen, onClose]);

  const hub = target ? hubs?.find((h) => h.id === target.hubId) : undefined;
  const machine = target ? hub?.overview?.machines?.find((m) => m.id === target.machineId) : undefined;

  const nodes = useMemo(() => {
    if (!target) return [];
    return rows.filter((r) =>
      r.hub.id === target.hubId &&
      r.st.agent.machine_id === target.machineId &&
      agentKind(r.st.agent) !== "machine",
    );
  }, [rows, target]);

  if (!target) return null;

  const openRename = () => {
    setName(machine?.name || "");
    setError(null);
    setRenameOpen(true);
  };

  const rename = async () => {
    if (!machine) return;
    setBusy(true);
    setError(null);
    try {
      await appApi.renameMachine(target.hubId, target.machineId, name.trim());
      await onRefresh();
      setRenameOpen(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  if (!hub || !machine) return null;

  const os = osInfo(machine.os);
  const OsIcon = os.icon;

  return (
    <>
      <div className="app-inspector-backdrop" onClick={onClose} />
      <aside className="app-inspector" role="dialog" aria-label={`${machine.name} 详情`}>
        <div className="app-inspector-head">
          <span className="app-row-icon"><OsIcon strokeWidth={1.75} /></span>
          <div className="titles">
            <h2>{machine.name}</h2>
            <div className="sub">{machine.hostname} · {os.label}</div>
          </div>
          <span className={cn("app-pill", machine.online ? "green" : "red")}>
            <span className={cn("dot", machine.online ? "working" : "offline")} />
            {machine.online ? "在线" : "离线"}
          </span>
          <button type="button" className="app-btn app-btn-plain" onClick={onClose} aria-label="关闭">
            <X />
          </button>
        </div>

        <div className="app-inspector-body">
          <section className="app-group">
            <div className="app-group-title"><span className="name">基本信息</span></div>
            <div className="app-list">
              <div className="app-kv"><span className="app-kv-label">机器 ID</span><span className="app-kv-value mono">{machine.id}</span></div>
              <div className="app-kv"><span className="app-kv-label">主机名</span><span className="app-kv-value mono">{machine.hostname}</span></div>
              <div className="app-kv">
                <span className="app-kv-label">系统</span>
                <span className="app-kv-value">{[os.label, machine.arch].filter(Boolean).join(" · ")}</span>
              </div>
              <div className="app-kv"><span className="app-kv-label">myco 版本</span><span className="app-kv-value">{ver(machine.myco_version) || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">最近在线</span><span className="app-kv-value">{machine.online ? "现在" : timeAgo(machine.last_seen_at, now)}</span></div>
              <div className="app-kv"><span className="app-kv-label">节点数</span><span className="app-kv-value">{machine.agents.length}</span></div>
            </div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">操作</span></div>
            <div className="app-list">
              <button type="button" className="app-action-row" onClick={openRename}>
                重命名…<span className="app-chev">›</span>
              </button>
              <button
                type="button"
                className="app-action-row"
                onClick={() => onSpawn(target.hubId, target.machineId)}
                disabled={!machine.can_spawn}
              >
                在这台机器上开新会话…<span className="app-chev">›</span>
              </button>
            </div>
            {!machine.can_spawn && (
              <div className="app-small-note" style={{ marginTop: 8 }}>
                这台机器的 myco 版本太旧，或者它的 runner 没在运行，暂时不能开新会话。
              </div>
            )}
          </section>

          <section className="app-group">
            <div className="app-group-title">
              <span className="name">节点</span>
              <span className="parts"><span className="sep">·</span><span className="part">{nodes.length} 个</span></span>
            </div>
            <div className="app-list">
              {nodes.length === 0 ? (
                <div className="app-empty-note">这台机器上还没有节点。</div>
              ) : (
                nodes.map((r) => {
                  const a = r.st.agent;
                  const kind = agentKind(a);
                  const state = stateMeta[r.st.state];
                  const Icon = nodeIcon(a);
                  return (
                    <button
                      key={a.id}
                      type="button"
                      className={cn("app-row", r.st.state === "offline" && "offline")}
                      onClick={() => onOpenNode(r)}
                      title={a.display_name || a.name}
                    >
                      <span className="app-row-icon"><Icon strokeWidth={1.75} /></span>
                      <span className="app-row-main">
                        <span className="app-row-title">{a.display_name || a.name}</span>
                        <span className="app-row-sub">
                          <span className={cn("dot", r.st.state)} />
                          <span>{state.label}</span>
                          <span className="sep">·</span>
                          <span className="txt">{kind === "human" ? kindLabel(kind) : cliName(a)}</span>
                        </span>
                      </span>
                      <span className="app-chev">›</span>
                    </button>
                  );
                })
              )}
            </div>
          </section>
        </div>
      </aside>

      <Modal
        open={renameOpen}
        onOpenChange={(o) => { if (!o && !busy) { setRenameOpen(false); setError(null); } }}
        title="重命名机器"
        description="只改这个 App 里显示的名字，不影响那台机器本身。"
      >
        <label className="app-field">
          <span className="app-field-label">名称</span>
          <input
            className="app-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={machine.hostname}
            autoFocus
          />
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button
            type="button"
            className="app-btn"
            onClick={() => { setRenameOpen(false); setError(null); }}
            disabled={busy}
          >
            取消
          </button>
          <button
            type="button"
            className="app-btn app-btn-primary"
            onClick={() => void rename()}
            disabled={busy || !name.trim()}
          >
            {busy ? "保存中…" : "保存"}
          </button>
        </div>
      </Modal>
    </>
  );
}
