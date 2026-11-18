import { useEffect, useState } from "react";
import { ChevronDown, Info, Plus, Settings2, SquareTerminal, Trash2 } from "lucide-react";
import { appApi, type LocalState, type MachineAI, type MachineInfo } from "@/appapi";
import type { Machine, Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, machineShell, machinesOfHub, nodeName, osInfo, ver } from "@/app/nodes";
import { shortDir } from "@/app/AddToProject";
import { machineTellsInfo } from "@/app/hubVersion";
import { cachedMachineInfo, fetchMachineInfo } from "@/app/machineInfoCache";
import { LocalShellAccess } from "@/app/LocalShellAccess";
import { agentQuotaHint } from "@/app/quota";
import { Avatar, isWorkspace } from "@/app/persona";
import { projectsOf } from "@/app/Members";
import { ErrorText, Modal } from "@/app/dialogs";
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
/**
 * The tightest quota among the agents running on this computer's machine
 * (a shell node has none of its own), named in the title: out of quota
 * first, then the most used.
 */
function machineQuotaHint(r: AgentRow, rows: AgentRow[]) {
  const mid = r.st.agent.machine_id;
  if (!mid) return undefined;
  let best: (NonNullable<ReturnType<typeof agentQuotaHint>> & { who: string }) | undefined;
  let bestRank = -1;
  for (const x of rows) {
    if (x.hub.id !== r.hub.id || x.st.agent.machine_id !== mid || x.st.agent.name === r.st.agent.name) continue;
    const h = agentQuotaHint(x.st.agent);
    if (!h) continue;
    const used = Math.max(...(x.st.agent.runtime?.quota?.windows || []).map((w) => w.used), 0);
    const rank = (h.level === "spent" ? 2 : 0) + Math.min(used, 1.99) / 2;
    if (rank > bestRank) {
      best = { ...h, who: nodeName(x.st.agent, x.hub) };
      bestRank = rank;
    }
  }
  return best && { ...best, title: L(`${best.who}：${best.title}`, `${best.who}: ${best.title}`) };
}

export function ComputersPage({ rows, projects, onOpen, onProfile, onConnect, onAddAgentOn, onAddTerminal, onChanged, onSettings }: {
  rows: AgentRow[];
  projects: Projects;
  /** opens a terminal */
  onOpen: (r: AgentRow) => void;
  onProfile: (r: AgentRow) => void;
  /** connects another computer (a VPS, WSL, a second laptop) */
  onConnect: () => void;
  /** adds an agent on this computer: a session there, or a new one */
  onAddAgentOn: (m: Machine) => void;
  /** opens a terminal in a folder of it (none: on a computer to connect) */
  onAddTerminal: (m?: Machine) => void;
  onChanged: () => void;
  /** who may use a terminal, its projects, removing it (ComputerSettings) */
  onSettings: (r: AgentRow) => void;
}) {
  const [local, setLocal] = useState<LocalState | null>(null);
  const [dropping, setDropping] = useState<Machine | null>(null);
  // The computer whose details (folders, AIs, terminals allowed) are open.
  const [open, setOpen] = useState<string | null>(null);
  useEffect(() => { appApi.local().then(setLocal).catch(() => setLocal(null)); }, []);
  const isHere = (m: Machine) => !!local && ((local.machines || []).includes(m.name) || (!!local.hostname && m.hostname === local.hostname));
  const hubs = [...new Map(rows.map((r) => [r.hub.id, r.hub])).values()];
  const machines = hubs.flatMap((h) => machinesOfHub(h).map((m) => ({ m, hub: h })))
    .sort((a, b) => Number(isHere(b.m)) - Number(isHere(a.m)) || Number(b.m.online) - Number(a.m.online) || a.m.name.localeCompare(b.m.name));
  const onIt = (m: Machine) => rows.filter((r) => r.st.agent.machine_id === m.id);
  const byState = (a: AgentRow, b: AgentRow) => Number(a.st.state === "offline") - Number(b.st.state === "offline") || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub));
  // Terminals whose computer is not known (older runners).
  const known = new Set(machines.map((x) => x.m.id));
  const loose = rows.filter((r) => isWorkspace(r.st.agent) && !known.has(r.st.agent.machine_id || "")).sort(byState);

  const terminalRow = (r: AgentRow) => {
    const a = r.st.agent;
    const ps = projectsOf(r, projects);
    const quota = machineQuotaHint(r, rows);
    return (
      <div key={`${r.hub.id}/${a.name}`} className={cn("pc-row", r.st.state === "offline" && "is-away")}>
        <Avatar agent={a} size={28} state={r.st.state} />
        <button type="button" className="pc-row-main" onClick={() => onProfile(r)}>
          <span className="pc-row-name">{a.binding.workdir ? shortDir(a.binding.workdir) : nodeName(a, r.hub)}</span>
          <span className="pc-row-sub">
            <span>{L("终端", "Terminal")}</span>
            {ps.length > 0 && <span>{L(`在「${ps.map((p) => p.title || p.name).join("」「")}」里`, `In ${ps.map((p) => p.title || p.name).join(", ")}`)}</span>}
            {quota && <span className={cn("pc-quota", quota.level === "spent" && "is-spent")} title={quota.title}>{quota.text}</span>}
            {r.st.state === "offline" && <span>{L("不在线", "Away")}</span>}
          </span>
        </button>
        <button type="button" className="app-btn app-btn-small" onClick={() => onOpen(r)}>{L("打开", "Open")}</button>
        <button type="button" className="app-btn app-btn-small pc-settings" onClick={() => onSettings(r)} title={L("谁能用、项目、移除", "Who can use it, projects, remove")}><Settings2 />{L("设置", "Settings")}</button>
      </div>
    );
  };

  return (
    <div className="pc">
      <div className="pc-head">
        <p className="pc-intro">{L("你的 agent 跑在这些电脑上：笔记本、云服务器、WSL 都各算一台。连上以后，它上面的会话点一下就能加进来，也能在上面新开会话、开终端。", "Your agents run on these computers: a laptop, a cloud server or WSL each counts as one. Once connected, its sessions join with a click, and you can start new ones or open terminals on it.")}</p>
        <button type="button" className="app-btn app-btn-small" onClick={onConnect}><Plus />{L("连一台电脑", "Connect a computer")}</button>
      </div>
      {machines.length === 0 && <div className="app-empty-note">{L("还没有连上的电脑。", "No computers connected yet.")}</div>}
      <div className="pc-cards">
        {machines.map(({ m, hub }) => {
          const all = onIt(m);
          const agents = all.filter((r) => agentKind(r.st.agent) === "agent").sort(byState);
          const terminals = all.filter((r) => isWorkspace(r.st.agent)).sort(byState);
          const Icon = osInfo(m.os).icon;
          const here = isHere(m);
          return (
            <section key={`${hub.id}/${m.id}`} className={cn("pc-list pc-card", !m.online && "is-away")} data-testid="pc-card">
              <div className="pc-card-head">
                <span className="pc-card-icon"><Icon /></span>
                <div className="pc-row-main">
                  <span className="pc-row-name">{m.name}{here && <span className="pc-machine-tag">{L("这台电脑", "This computer")}</span>}</span>
                  <span className="pc-row-sub">
                    <span className={cn("pc-online", m.online && "is-on")}>{m.online ? L("在线", "Online") : L("不在线", "Away")}</span>
                    <span>{osInfo(m.os).label}</span>
                    {m.myco_version && <span>myco {ver(m.myco_version)}</span>}
                  </span>
                </div>
                <button type="button" className="app-btn app-btn-small" onClick={() => onAddAgentOn(m)}><Plus />{L("加 agent", "Add an agent")}</button>
                <button type="button" className="app-btn app-btn-small" onClick={() => onAddTerminal(m)} disabled={!m.can_spawn}><SquareTerminal />{L("开终端", "Terminal")}</button>
                <button type="button" className={cn("app-btn app-btn-small app-btn-quiet pc-card-more", open === m.id && "is-open")} onClick={() => setOpen(open === m.id ? null : m.id)}
                  aria-expanded={open === m.id} title={L("它允许什么、装了哪些 AI、移除", "What it allows, its AIs, remove")}>{L("详情", "Details")}<ChevronDown /></button>
              </div>
              {agents.length > 0 && (
                <div className="pc-card-agents">
                  {agents.map((r) => (
                    <button key={r.st.agent.name} type="button" className={cn("pc-card-agent", r.st.state === "offline" && "is-away")} onClick={() => onProfile(r)} title={r.st.agent.binding.workdir || undefined}>
                      <Avatar agent={r.st.agent} size={20} state={r.st.state} />
                      <span>{nodeName(r.st.agent, r.hub)}</span>
                    </button>
                  ))}
                </div>
              )}
              {terminals.map(terminalRow)}
              {agents.length === 0 && terminals.length === 0 && (
                <div className="pc-card-empty">{L("上面还没有 agent 或终端。", "No agents or terminals on it yet.")}</div>
              )}
              {open === m.id && (
                <ComputerDetails hubId={hub.id} hub={hub} machine={m}
                  onChanged={onChanged} onRemove={() => { setDropping(m); setOpen(null); }} />
              )}
            </section>
          );
        })}
      </div>
      {loose.length > 0 && (
        <>
          <div className="pc-loose-head">{L("其他终端", "Other terminals")}</div>
          <div className="pc-list">{loose.map(terminalRow)}</div>
        </>
      )}
      <RemoveComputer machine={dropping} rows={rows} onClose={() => setDropping(null)} onRemoved={() => { setDropping(null); onChanged(); }} />
    </div>
  );
}

const ACCESS_LABEL: Record<string, () => string> = {
  readonly: () => L("只读", "Read only"),
  workspace: () => L("只能改自己的文件夹", "Its own folder"),
  full: () => L("完全放开", "Full"),
};

/** A computer's details, asked of it: where agents may start, how much
 *  they may do, which AIs it has; whether terminals may be opened there;
 *  removing it. */
function ComputerDetails({ hubId, hub, machine, onChanged, onRemove }: {
  hubId: string;
  hub: AgentRow["hub"];
  machine: Machine;
  onChanged: () => void;
  onRemove: () => void;
}) {
  // The last answer at once (the App asks ahead), a fresh one behind it.
  const [info, setInfo] = useState<MachineInfo | null>(() => cachedMachineInfo(hubId, machine.id)?.info || null);
  const [error, setError] = useState<unknown>(null);
  const asks = machine.online && machine.can_spawn && machineTellsInfo(machine.myco_version);
  useEffect(() => {
    if (!asks) return;
    let off = false;
    fetchMachineInfo(hubId, machine.id).then((i) => { if (!off) setInfo(i); }, (e) => { if (!off) setError(e); });
    return () => { off = true; };
  }, [asks, hubId, machine.id]);
  const aiText = (a: MachineAI) => a.state === "ready" ? L("可用", "Ready") : a.state === "signed_out" ? L("没登录", "Signed out") : a.state === "missing" ? L("没装", "Not installed") : L("已安装", "Installed");
  return (
    <div className="pc-details" data-testid="pc-details">
      {!machine.online ? (
        <div className="app-small-note">{L("它现在不在线：开机、myco 在运行时才看得到它允许什么、装了哪些 AI。", "It is away: what it allows and which AIs it has show once it is on and myco runs there.")}</div>
      ) : !asks ? (
        <div className="app-small-note">{L("它上面的 myco 版本太旧，说不出它允许什么、装了哪些 AI。在那台电脑上运行 myco upgrade。", "Its myco is too old to say what it allows and which AIs it has. Run myco upgrade on it.")}</div>
      ) : error ? <ErrorText error={error} /> : !info ? (
        <div className="app-small-note">{L("正在问它…", "Asking it…")}</div>
      ) : (
        <div>
          <div className="app-kv"><span className="app-kv-label">{L("agent 能在哪里开工", "Where agents may start")}</span><span className="app-kv-value mono">{info.dirs.join("、")}</span></div>
          <div className="app-kv"><span className="app-kv-label">{L("最高权限", "Highest access")}</span><span className="app-kv-value">{(ACCESS_LABEL[info.max_access] || (() => info.max_access))()}</span></div>
          <div className="app-kv"><span className="app-kv-label">AI</span><span className="app-kv-value">{info.ais.map((a) => `${a.name} ${aiText(a)}`).join(" · ")}</span></div>
          <div className="app-small-note">{L("这些由那台电脑自己决定，只能在它上面改（~/.myco/machine.yaml 的 spawn.dirs、spawn.max_access）。", "That computer decides these itself; they are changed on it only (spawn.dirs and spawn.max_access in ~/.myco/machine.yaml).")}</div>
        </div>
      )}
      <LocalShellAccess machine={machine} what="shell" shell={machineShell(hub, machine.id)} onChange={async () => onChanged()} />
      <div className="app-setting-actions">
        <button type="button" className="app-btn app-btn-quiet" onClick={onRemove}><Trash2 />{L("移除这台电脑", "Remove this computer")}</button>
      </div>
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
              ? L(`它上面的 ${n} 个 agent 和终端会一起移除；聊过的内容都保留。`, `The ${n} agent${n === 1 ? "" : "s"} and terminals on it go too; conversations are kept.`)
              : L("它上面没有 agent 或终端；聊过的内容都保留。", "Nothing else is on it; conversations are kept.")}</div>
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
