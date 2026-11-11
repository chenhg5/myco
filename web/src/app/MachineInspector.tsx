import { useEffect, useMemo, useState } from "react";
import { ver } from "./nodes";
import { X } from "lucide-react";
import { appApi, type HubOverview } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, canManage, cliName, kindLabel, nodeIcon, osInfo } from "@/app/nodes";
import { NodePicker } from "@/app/NodePicker";
import { ErrorText, Modal } from "@/app/dialogs";
import { stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";
import { L } from "@/lib/i18n";

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
  const [allowOpen, setAllowOpen] = useState(false);
  const [allowFrom, setAllowFrom] = useState<string[]>([]);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    setRenameOpen(false);
    setAllowOpen(false);
    setName("");
    setBusy(false);
    setError(null);
  }, [target?.hubId, target?.machineId]);

  // Escape closes it, like the node inspector (not while renaming).
  useEffect(() => {
    if (!target) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape" && !renameOpen && !allowOpen) onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [target, renameOpen, allowOpen, onClose]);

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

  // The machine's own node: its allow-list says which agents may start new
  // sessions here (agent spawn). Empty or "*" means any agent on the hub.
  const machineNode = (hub.overview?.agents || []).map((x) => x.agent)
    .find((a) => a.machine_id === machine.id && a.binding.harness === "machine");
  const spawners = machineNode?.allow_from || [];
  const anySpawner = spawners.length === 0 || spawners.includes("*");
  const mayManage = !!machineNode && canManage(hub, machineNode.owner_id);
  const nameOf = (n: string) => hub.overview?.agents.find((x) => x.agent.name === n)?.agent.display_name || n;
  const openAllow = () => {
    setAllowFrom(spawners);
    setError(null);
    setAllowOpen(true);
  };
  const saveAllow = async () => {
    if (!machineNode) return;
    // Still open to any node: nothing to change ("*" is never sent by name).
    if (allowFrom.includes("*") || (allowFrom.length === 0 && anySpawner)) {
      setAllowOpen(false);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await appApi.updateAllowFrom(target.hubId, machineNode.name, allowFrom);
      await onRefresh();
      setAllowOpen(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const os = osInfo(machine.os);
  const OsIcon = os.icon;

  return (
    <>
      <div className="app-inspector-backdrop" onClick={onClose} />
      <aside className="app-inspector" role="dialog" aria-label={L(`${machine.name} 详情`, `${machine.name} details`)}>
        <div className="app-inspector-head">
          <span className="app-row-icon"><OsIcon /></span>
          <div className="titles">
            <h2>{machine.name}</h2>
            <div className="sub">{machine.hostname} · {os.label}</div>
          </div>
          <span className={cn("app-pill", machine.online ? "green" : "red")}>
            <span className={cn("dot", machine.online ? "working" : "offline")} />
            {machine.online ? L("在线", "Online") : L("离线", "Offline")}
          </span>
          <button type="button" className="app-btn app-btn-plain" onClick={onClose} aria-label={L("关闭", "Close")}>
            <X />
          </button>
        </div>

        <div className="app-inspector-body">
          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("基本信息", "Details")}</span></div>
            <div className="app-list">
              <div className="app-kv"><span className="app-kv-label">{L("机器 ID", "Machine ID")}</span><span className="app-kv-value mono">{machine.id}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("主机名", "Host name")}</span><span className="app-kv-value mono">{machine.hostname}</span></div>
              {machine.owner && <div className="app-kv"><span className="app-kv-label">{L("主人", "Owner")}</span><span className="app-kv-value">{machine.owner}</span></div>}
              <div className="app-kv">
                <span className="app-kv-label">{L("系统", "System")}</span>
                <span className="app-kv-value">{[os.label, machine.arch].filter(Boolean).join(" · ")}</span>
              </div>
              <div className="app-kv"><span className="app-kv-label">{L("myco 版本", "myco version")}</span><span className="app-kv-value">{ver(machine.myco_version) || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("最近在线", "Last online")}</span><span className="app-kv-value">{machine.online ? L("现在", "Now") : timeAgo(machine.last_seen_at, now)}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("节点数", "Nodes")}</span><span className="app-kv-value">{machine.agents.length}</span></div>
              {machineNode && (
                <div className="app-kv">
                  <span className="app-kv-label">{L("谁能开新会话", "Who can start sessions")}</span>
                  <span className="app-kv-value">
                    {mayManage ? (
                      <button type="button" className="app-kv-link" onClick={openAllow}>
                        {anySpawner ? L("任何节点", "Any node") : spawners.map(nameOf).join("、")}<span className="app-chev">›</span>
                      </button>
                    ) : anySpawner ? L("任何节点", "Any node") : spawners.map(nameOf).join("、")}
                  </span>
                </div>
              )}
            </div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">{L("操作", "Actions")}</span></div>
            <div className="app-list">
              <button type="button" className="app-action-row" onClick={openRename}>
                {L("重命名", "Rename")}<span className="app-chev">›</span>
              </button>
              <button
                type="button"
                className="app-action-row"
                onClick={() => onSpawn(target.hubId, target.machineId)}
                disabled={!machine.can_spawn}
              >
                {L("在这台机器上开新会话", "Start a new session on this machine")}<span className="app-chev">›</span>
              </button>
            </div>
            {!machine.can_spawn && (
              <div className="app-small-note" style={{ marginTop: 8 }}>
                {L("这台机器的 myco 版本太旧，或者它的 runner 没在运行，暂时不能开新会话。", "This machine’s myco is too old, or its runner isn’t running, so it can’t start a new session right now.")}
              </div>
            )}
          </section>

          <section className="app-group">
            <div className="app-group-title">
              <span className="name">{L("节点", "Nodes")}</span>
              <span className="parts"><span className="sep">·</span><span className="part">{L(`${nodes.length} 个`, `${nodes.length}`)}</span></span>
            </div>
            <div className="app-list">
              {nodes.length === 0 ? (
                <div className="app-empty-note">{L("这台机器上还没有节点。", "No nodes on this machine yet.")}</div>
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
                      <span className="app-row-icon"><Icon /></span>
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
        title={L("重命名机器", "Rename machine")}
        description={L("只改这个 App 里显示的名字，不影响那台机器本身。", "This only changes the name shown in this app, not the machine itself.")}
      >
        <label className="app-field">
          <span className="app-field-label">{L("名称", "Name")}</span>
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
            {L("取消", "Cancel")}
          </button>
          <button
            type="button"
            className="app-btn app-btn-primary"
            onClick={() => void rename()}
            disabled={busy || !name.trim()}
          >
            {busy ? L("保存中…", "Saving…") : L("保存", "Save")}
          </button>
        </div>
      </Modal>

      {machineNode && (
        <Modal
          open={allowOpen}
          onOpenChange={(o) => { if (!o && !busy) { setAllowOpen(false); setError(null); } }}
          title={L("谁能在这台机器上开新会话", "Who can start sessions on this machine")}
          description={L(
            `只有选中的节点可以在 ${machine.name} 上开新的 Claude Code、Codex 会话或 Shell 节点（myco agent spawn / 在机器上新建）。一个都不选表示 Hub 上任何节点都可以。你在 App 里操作不受这个限制。`,
            `Only the selected nodes can start new Claude Code or Codex sessions or shell nodes on ${machine.name} (myco agent spawn / New on a machine). Select none to allow any node on the hub. What you do in the app is not limited by this.`,
          )}
        >
          <NodePicker
            nodes={(hub.overview?.agents || []).map((x) => x.agent).filter((a) => agentKind(a) !== "machine" && agentKind(a) !== "human")}
            value={allowFrom.filter((n) => n !== "*")}
            onChange={setAllowFrom}
            disabled={busy}
            emptyPlaceholder={allowFrom.includes("*") || allowFrom.length === 0 ? L("现在任何节点都可以；选中节点后只允许它们", "Any node can now; pick nodes to allow only them") : undefined}
          />
          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={() => { setAllowOpen(false); setError(null); }} disabled={busy}>{L("取消", "Cancel")}</button>
            <button type="button" className="app-btn app-btn-primary" onClick={() => void saveAllow()} disabled={busy}>{busy ? L("保存中…", "Saving…") : L("保存", "Save")}</button>
          </div>
        </Modal>
      )}
    </>
  );
}
