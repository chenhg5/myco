import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { appApi, type LocalState } from "@/appapi";
import type { Machine, Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { machinesOfHub, nodeName } from "@/app/nodes";
import { Avatar, isWorkspace } from "@/app/persona";
import { projectsOf } from "@/app/Members";
import { ConfirmDialog, ErrorText } from "@/app/dialogs";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type Projects = { hubId: string; p: Project }[];

/**
 * The computers agents may use, one row each as a project shows them
 * ("Studio Mac · myco"): the folder, the projects that have it, and Open
 * for its terminal. Below, in one line, the computers connected, and a way
 * to connect another.
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
      <p className="pc-intro">{L("agent 能用的电脑。每一台是你某台电脑上的一个文件夹，agent 在里面跑命令，你点「打开」就能看着它的终端。要给项目加一台，在项目右边「电脑」旁点 +。", "The computers agents can use. Each is a folder on one of your computers where agents run commands; Open shows its terminal. To add one to a project, use + next to Computers in the project.")}</p>
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
          <span key={m.id} className={cn("pc-machine", !m.online && "is-away")} title={m.online ? L("在线", "Online") : L("不在线", "Away")}>
            {i > 0 && <span className="pc-sep">·</span>}
            {m.name}{isHere(m) && L("（这台电脑）", " (this computer)")}
          </span>
        ))}
        <button type="button" className="pc-add" onClick={onAddComputer}><Plus />{L("连上另一台电脑", "Connect another computer")}</button>
      </div>

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
