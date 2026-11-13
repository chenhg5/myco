import { useEffect, useState } from "react";
import { Info, Plus, Trash2 } from "lucide-react";
import { appApi, type LocalState } from "@/appapi";
import type { Machine, Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { machinesOfHub, nodeName } from "@/app/nodes";
import { Avatar, isWorkspace } from "@/app/persona";
import { projectsOf } from "@/app/Members";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type Projects = { hubId: string; p: Project }[];

/**
 * The computers agents may use, one row each as a project shows them
 * ("Studio Mac · myco"): the folder, the projects that have it, and Open
 * for its terminal. Connecting another is at the top; below, in one line,
 * the computers connected (each can be removed from there).
 */
export function ComputersPage({ rows, projects, onOpen, onProfile, onAddComputer, onChanged }: {
  rows: AgentRow[];
  projects: Projects;
  /** opens its terminal */
  onOpen: (r: AgentRow) => void;
  onProfile: (r: AgentRow) => void;
  onAddComputer: () => void;
  onChanged: () => void;
}) {
  const [local, setLocal] = useState<LocalState | null>(null);
  const [removing, setRemoving] = useState<AgentRow | null>(null);
  const [dropping, setDropping] = useState<Machine | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => { appApi.local().then(setLocal).catch(() => setLocal(null)); }, []);
  const isHere = (m: Machine) => !!local && ((local.machines || []).includes(m.name) || (!!local.hostname && m.hostname === local.hostname));

  const list = rows.filter((r) => isWorkspace(r.st.agent))
    .sort((a, b) => Number(a.st.state === "offline") - Number(b.st.state === "offline") || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub)));
  const hubs = [...new Map(rows.map((r) => [r.hub.id, r.hub])).values()];
  const machines = hubs.flatMap((h) => machinesOfHub(h))
    .sort((a, b) => Number(isHere(b)) - Number(isHere(a)) || Number(b.online) - Number(a.online) || a.name.localeCompare(b.name));

  const remove = async () => {
    if (!removing) return;
    setError(null);
    try {
      await appApi.removeAgent(removing.hub.id, removing.st.agent.name);
      onChanged();
    } catch (e) {
      setError(e);
    } finally {
      setRemoving(null);
    }
  };

  return (
    <div className="pc">
      <div className="pc-head">
        <p className="pc-intro">{L("agent 在这些电脑上跑命令，点「打开」就能看着。", "Agents run commands on these; Open lets you watch.")}</p>
        <button type="button" className="app-btn app-btn-small" onClick={onAddComputer}><Plus />{L("连上另一台电脑", "Connect another computer")}</button>
      </div>
      <ErrorText error={error} />
      {list.length === 0 ? (
        <div className="app-empty-note">{L("还没有交给 agent 的电脑。", "No computers handed to agents yet.")}</div>
      ) : (
        <div className="pc-list">
          {list.map((r) => {
            const a = r.st.agent;
            const ps = projectsOf(r, projects);
            return (
              <div key={`${r.hub.id}/${a.name}`} className={cn("pc-row", r.st.state === "offline" && "is-away")}>
                <Avatar agent={a} size={34} state={r.st.state} />
                <button type="button" className="pc-row-main" onClick={() => onProfile(r)}>
                  <span className="pc-row-name">{nodeName(a, r.hub)}</span>
                  <span className="pc-row-sub">
                    {a.binding.workdir && <span className="mono">{a.binding.workdir}</span>}
                    {ps.length > 0 && <span>{L(`在「${ps.map((p) => p.title || p.name).join("」「")}」里`, `In ${ps.map((p) => p.title || p.name).join(", ")}`)}</span>}
                    {r.st.state === "offline" && <span>{L("不在线", "Away")}</span>}
                  </span>
                </button>
                <button type="button" className="app-btn app-btn-small" onClick={() => onOpen(r)}>{L("打开", "Open")}</button>
                <button type="button" className="app-btn app-btn-plain pc-remove" onClick={() => setRemoving(r)} aria-label={L("移除", "Remove")} title={L("移除", "Remove")}><Trash2 /></button>
              </div>
            );
          })}
        </div>
      )}

      <div className="pc-connected">
        <span className="pc-connected-label">{L("已连上：", "Connected: ")}</span>
        {machines.map((m, i) => (
          <span key={m.id} className="pc-machine-wrap">
            {i > 0 && <span className="pc-sep">·</span>}
            <button type="button" className={cn("pc-machine", !m.online && "is-away")} onClick={() => setDropping(m)}
              title={L(`${m.online ? "在线" : "不在线"} · 点一下可以移除这台电脑`, `${m.online ? "Online" : "Away"} · click to remove this computer`)}>
              {m.name}{isHere(m) && L("（这台电脑）", " (this computer)")}
            </button>
          </span>
        ))}
      </div>

      <RemoveComputer machine={dropping} rows={rows} onClose={() => setDropping(null)} onRemoved={() => { setDropping(null); onChanged(); }} />
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={L("移除这台电脑？", "Remove this computer?")}
        description={removing ? L(`「${nodeName(removing.st.agent, removing.hub)}」：agent 就不能再在这里跑命令了。文件夹里的东西不会动。`, `“${nodeName(removing.st.agent, removing.hub)}”: agents can no longer run commands here. Nothing in the folder is touched.`) : ""}
        confirmLabel={L("移除", "Remove")}
        onConfirm={() => void remove()}
      />
    </div>
  );
}

/**
 * Removing a whole computer from the workspace: a dry run first says which
 * agents and folders go with it; conversations are kept. Online, its myco
 * is refused and stops, and the command to uninstall it there is given.
 */
function RemoveComputer({ machine, rows, onClose, onRemoved }: {
  machine: Machine | null;
  rows: AgentRow[];
  onClose: () => void;
  onRemoved: () => void;
}) {
  const hubId = machine ? rows.find((r) => (r.hub.overview?.machines || []).some((m) => m.id === machine.id))?.hub.id || "" : "";
  const [gone, setGone] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (!machine || !hubId) return;
    let off = false;
    setGone(null);
    setError(null);
    appApi.removeMachine(hubId, machine.id, true)
      .then((r) => { if (!off) setGone(r.removed); })
      .catch((e) => { if (!off) setError(e); });
    return () => { off = true; };
  }, [machine, hubId]);
  if (!machine) return null;
  const rowOf = (n: string) => rows.find((r) => r.hub.id === hubId && r.st.agent.name === n);
  const remove = async () => {
    setBusy(true);
    setError(null);
    try {
      await appApi.removeMachine(hubId, machine.id);
      onRemoved();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const n = gone?.length || 0;
  return (
    <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={L(`移除「${machine.name}」？`, `Remove “${machine.name}”?`)}
      description={L("这台电脑会从工作区里拿掉，不能撤销。", "This computer leaves the workspace; this can't be undone.")}>
      {gone === null && !error && <div className="app-empty-note">{L("正在看会一起移除什么…", "Looking at what goes with it…")}</div>}
      {gone !== null && n > 0 && (
        <div className="pc-gone">
          {gone.map((name) => {
            const r = rowOf(name);
            return (
              <div key={name} className="pc-gone-row">
                {r && <Avatar agent={r.st.agent} size={24} badge={false} />}
                <span>{r ? nodeName(r.st.agent, r.hub) : name}</span>
              </div>
            );
          })}
        </div>
      )}
      {gone !== null && (
        <div className="app-notice">
          <Info />
          <div>
            <div>{n > 0
              ? L(`它上面的 ${n} 个 agent 和电脑会一起移除；聊过的内容都保留。`, `The ${n} agent${n === 1 ? "" : "s"} and computers on it go too; conversations are kept.`)
              : L("它上面没有 agent 或电脑；聊过的内容都保留。", "Nothing else is on it; conversations are kept.")}</div>
            {machine.online && (
              <>
                <div style={{ marginTop: 6 }}>{L("那台电脑上的 myco 会被拒绝并停下。想彻底卸载，在那台电脑上运行：", "Its myco on that computer is refused and stops. To uninstall it completely, run this on that computer:")}</div>
                <div className="app-command"><code>myco runner service uninstall</code><CopyButton value="myco runner service uninstall" /></div>
              </>
            )}
          </div>
        </div>
      )}
      <ErrorText error={error} />
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
        <button type="button" className="app-btn app-btn-danger" onClick={() => void remove()} disabled={busy || gone === null}>
          {busy ? L("正在移除…", "Removing…") : n > 0 ? L(`移除这台电脑和上面的 ${n} 个`, `Remove it and the ${n} on it`) : L("移除这台电脑", "Remove this computer")}
        </button>
      </div>
    </Modal>
  );
}
