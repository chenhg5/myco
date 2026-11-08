import { useEffect, useState } from "react";
import { AlertTriangle, Check } from "lucide-react";
import {
  appApi, type Hub, type HubOverview, type SpawnCli, type SpawnResult,
} from "@/appapi";
import type { Agent, RuntimeAccess } from "@/api";
import { ErrorText, Modal } from "@/app/dialogs";
import { cn } from "@/lib/utils";

export interface SpawnTarget { hubId?: string; machineId?: string }

const CLIS: { value: SpawnCli; label: string }[] = [
  { value: "claude", label: "Claude Code" },
  { value: "codex", label: "Codex" },
  { value: "pi", label: "Pi" },
  { value: "kiro", label: "Kiro" },
  { value: "cursor", label: "Cursor" },
  { value: "gemini", label: "Gemini CLI" },
  { value: "opencode", label: "OpenCode" },
];

const ACCESS: { value: RuntimeAccess; label: string }[] = [
  { value: "readonly", label: "只读" },
  { value: "workspace", label: "工作区" },
  { value: "full", label: "完全" },
];

const NO_SPAWN = "这台机器的 myco 版本太旧，或者它的 runner 没在运行。";

/** Opens a new session on a machine, from the nodes page or a machine's detail. */
export function AddNodeDialog({ target, hubs, hubsOv, onClose, onSpawned }: {
  target: SpawnTarget | null;
  hubs: Hub[];
  hubsOv: HubOverview[] | undefined;
  onClose: () => void;
  onSpawned: (hubId: string, agent: Agent) => void;
}) {
  const [hubId, setHubId] = useState("");
  const [machineId, setMachineId] = useState("");
  const [cli, setCli] = useState<SpawnCli>("claude");
  const [dir, setDir] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [model, setModel] = useState("");
  const [access, setAccess] = useState<RuntimeAccess>("workspace");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<SpawnResult | null>(null);

  const machinesFor = (id: string) => hubsOv?.find((h) => h.id === id)?.overview?.machines || [];
  const machines = machinesFor(hubId);
  const machine = machines.find((m) => m.id === machineId);

  // Start from the requested machine, or the first machine that can spawn.
  useEffect(() => {
    if (!target) return;
    const firstHub = hubs.find((h) => h.ok) || hubs[0];
    const initialHub = target.hubId || firstHub?.id || "";
    const ms = machinesFor(initialHub);
    setHubId(initialHub);
    setMachineId(target.machineId || ms.find((m) => m.can_spawn)?.id || "");
    setCli("claude");
    setDir("");
    setName("");
    setDescription("");
    setModel("");
    setAccess("workspace");
    setBusy(false);
    setError(null);
    setResult(null);
    // Only reset when the dialog opens for a new target, not on every poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target]);

  // The overview may arrive after the dialog opened; then pick a machine.
  useEffect(() => {
    if (!target || machineId) return;
    const first = machinesFor(hubId).find((m) => m.can_spawn);
    if (first) setMachineId(first.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, hubId, machineId, hubsOv]);

  const changeHub = (id: string) => {
    setHubId(id);
    setMachineId(machinesFor(id).find((m) => m.can_spawn)?.id || "");
  };

  const close = () => { if (!busy) onClose(); };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!hubId || !machineId || !dir.trim()) return;
    setBusy(true);
    setError(null);
    try {
      const r = await appApi.spawn(hubId, machineId, {
        cli,
        dir: dir.trim(),
        ...(name.trim() ? { name: name.trim() } : {}),
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(model.trim() ? { model: model.trim() } : {}),
        access,
      });
      setResult(r);
      onSpawned(hubId, r.agent);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  const unavailable = machines.filter((m) => !m.can_spawn);

  return (
    <Modal
      open={!!target}
      onOpenChange={(o) => !o && close()}
      title="添加节点"
      description="在机器上启动一个新的会话。"
      wide
    >
      {result ? (
        <>
          <div className="app-success">
            <Check />
            <span>已启动 {result.agent.display_name || result.agent.name}</span>
          </div>
          <div className="app-reply">{result.reply}</div>
          <div className="app-hint">新节点已经加到节点列表里，会用绿色标记一会儿。</div>
          <div className="app-modal-actions">
            <button type="button" className="app-btn app-btn-primary" onClick={onClose}>完成</button>
          </div>
        </>
      ) : (
        <form onSubmit={submit}>
          <label className="app-field">
            <span className="app-field-label">Hub</span>
            <select
              className="app-select-field"
              value={hubId}
              onChange={(e) => changeHub(e.target.value)}
              disabled={busy}
              required
            >
              {hubs.map((h) => (
                <option key={h.id} value={h.id} disabled={!h.ok}>
                  {h.name}{h.ok ? "" : "（连不上）"}
                </option>
              ))}
            </select>
          </label>

          <label className="app-field">
            <span className="app-field-label">机器</span>
            <select
              className="app-select-field"
              value={machineId}
              onChange={(e) => setMachineId(e.target.value)}
              disabled={busy || machines.length === 0}
              required
            >
              {machines.length === 0 && <option value="">没有可用的机器</option>}
              {machines.map((m) => (
                <option key={m.id} value={m.id} disabled={!m.can_spawn}>
                  {m.name}{!m.can_spawn ? "（不可用）" : !m.online ? "（离线）" : ""}
                </option>
              ))}
            </select>
          </label>
          {machines.length === 0 ? (
            <div className="app-small-note" style={{ marginTop: 6 }}>
              这个 Hub 还没有上报机器。先到机器页添加一台。
            </div>
          ) : (
            <>
              {machine && !machine.online && (
                <div className="app-notice">
                  <AlertTriangle />
                  <span>这台机器现在离线，会话可能启动不了。</span>
                </div>
              )}
              {unavailable.map((m) => (
                <div className="app-small-note" key={m.id} style={{ marginTop: 6 }}>
                  {m.name} 不可用：{NO_SPAWN}
                </div>
              ))}
            </>
          )}

          <label className="app-field">
            <span className="app-field-label">CLI</span>
            <select
              className="app-select-field"
              value={cli}
              onChange={(e) => setCli(e.target.value as SpawnCli)}
              disabled={busy}
            >
              {CLIS.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
            </select>
          </label>

          <label className="app-field">
            <span className="app-field-label">目录</span>
            <input
              className="app-input mono"
              value={dir}
              onChange={(e) => setDir(e.target.value)}
              placeholder="~/code/项目名"
              disabled={busy}
              required
            />
          </label>

          <label className="app-field">
            <span className="app-field-label">名称（可选）</span>
            <input
              className="app-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="留空自动命名"
              disabled={busy}
            />
          </label>

          <label className="app-field">
            <span className="app-field-label">描述（可选）</span>
            <input
              className="app-input"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={busy}
            />
          </label>

          <label className="app-field">
            <span className="app-field-label">模型（可选）</span>
            <input
              className="app-input mono"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder="留空用 CLI 默认"
              disabled={busy}
            />
          </label>

          <div className="app-field">
            <span className="app-field-label">权限</span>
            <div className="seg seg-fill" role="tablist" aria-label="权限">
              {ACCESS.map((o) => (
                <button
                  key={o.value}
                  type="button"
                  role="tab"
                  aria-selected={access === o.value}
                  className={cn("seg-btn", access === o.value && "active")}
                  onClick={() => setAccess(o.value)}
                  disabled={busy}
                >
                  {o.label}
                </button>
              ))}
            </div>
          </div>
          {access === "full" && (
            <div className="app-warn">
              <AlertTriangle />
              <span>完全权限可以让它读写那台机器上的任何文件，需要在那台机器上本地允许。</span>
            </div>
          )}

          <ErrorText error={error} />
          {busy && (
            <div className="app-progress">
              <span className="app-spinner" />正在那台机器上启动会话，可能需要一两分钟…
            </div>
          )}
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={close} disabled={busy}>取消</button>
            <button
              type="submit"
              className="app-btn app-btn-primary"
              disabled={busy || !hubId || !machineId || !dir.trim()}
            >
              {busy ? "正在启动…" : "开始会话"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
