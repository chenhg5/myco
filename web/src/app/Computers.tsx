import { useEffect, useState } from "react";
import { ArrowLeft, ChevronRight, Info, Monitor, Pencil, Plus, Settings2, SquareTerminal, Trash2 } from "lucide-react";
import { appApi, type LocalState, type MachineAI, type MachineInfo } from "@/appapi";
import type { Machine, Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, machineShell, machinesOfHub, nodeName, osInfo, thisComputerId, ver } from "@/app/nodes";
import { shortDir } from "@/app/AddToProject";
import { machineTellsInfo } from "@/app/hubVersion";
import { cachedMachineInfo, fetchMachineInfo } from "@/app/machineInfoCache";
import { LocalShellAccess } from "@/app/LocalShellAccess";
import { agentQuotaHint } from "@/app/quota";
import { Avatar, AvatarStack, isWorkspace, stateWords } from "@/app/persona";
import { PageHero, SectionHead } from "@/app/PageHero";
import { projectsOf } from "@/app/Members";
import { ErrorText, Modal } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type Projects = { hubId: string; p: Project }[];

/** The tightest quota among the agents on a computer: out of quota first,
 *  then the most used; named in its title. */
function computerQuotaHint(machineId: string, hubId: string, rows: AgentRow[]) {
  let best: (NonNullable<ReturnType<typeof agentQuotaHint>> & { who: string }) | undefined;
  let bestRank = -1;
  for (const x of rows) {
    if (x.hub.id !== hubId || x.st.agent.machine_id !== machineId) continue;
    const h = agentQuotaHint(x.st.agent);
    if (!h) continue;
    const used = Math.max(...(x.st.agent.runtime?.quota?.windows || []).map((w) => w.used), 0);
    const rank = (h.level === "spent" ? 2 : 0) + Math.min(used, 1.99) / 2;
    if (rank > bestRank) { best = { ...h, who: nodeName(x.st.agent, x.hub) }; bestRank = rank; }
  }
  return best && { ...best, title: L(`${best.who}：${best.title}`, `${best.who}: ${best.title}`) };
}

/*
 * 电脑: the real machines agents run on (a laptop, a VPS, WSL), as the
 * other top pages are: a header with its main action, a few numbers, then
 * a card each (what it is, whether it is on, what runs there). A card
 * opens the computer: its agents, its terminals, what it allows (asked of
 * it), and removing it, each a section of its own. Terminals whose
 * computer is not known (older runners) are listed under the cards.
 */
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
  // The computer opened (coming to the page again shows them all).
  const [openKey, setOpenKey] = useState<string | null>(null);
  const openComputer = (k: string | null) => setOpenKey(k);
  const [renaming, setRenaming] = useState(false);
  useEffect(() => { appApi.local().then(setLocal).catch(() => setLocal(null)); }, []);
  const hubs = [...new Map(rows.map((r) => [r.hub.id, r.hub])).values()];
  const hereIds = new Set(hubs.map((h) => thisComputerId(h, local)).filter(Boolean));
  const isHere = (m: Machine) => hereIds.has(m.id);
  const machines = hubs.flatMap((h) => machinesOfHub(h).map((m) => ({ m, hub: h, key: `${h.id}/${m.id}` })))
    .sort((a, b) => Number(isHere(b.m)) - Number(isHere(a.m)) || Number(b.m.online) - Number(a.m.online) || a.m.name.localeCompare(b.m.name));
  const onIt = (hubId: string, m: Machine) => rows.filter((r) => r.hub.id === hubId && r.st.agent.machine_id === m.id);
  const byState = (a: AgentRow, b: AgentRow) => Number(a.st.state === "offline") - Number(b.st.state === "offline") || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub));
  const split = (hubId: string, m: Machine) => {
    const all = onIt(hubId, m);
    return {
      agents: all.filter((r) => agentKind(r.st.agent) === "agent").sort(byState),
      terminals: all.filter((r) => isWorkspace(r.st.agent)).sort(byState),
    };
  };
  // Terminals whose computer is not known (older runners).
  const known = new Set(machines.map((x) => x.m.id));
  const loose = rows.filter((r) => isWorkspace(r.st.agent) && !known.has(r.st.agent.machine_id || "")).sort(byState);
  const opened = machines.find((x) => x.key === openKey) || null;
  useEffect(() => { if (openKey && !opened && machines.length) openComputer(null); }, [openKey, opened, machines.length]);

  const terminalRow = (r: AgentRow) => {
    const a = r.st.agent;
    const ps = projectsOf(r, projects);
    return (
      <div key={`${r.hub.id}/${a.name}`} className={cn("pc-row pc-term", r.st.state === "offline" && "is-away")}>
        <Avatar agent={a} size={28} state={r.st.state} />
        <button type="button" className="pc-row-main" onClick={() => onProfile(r)}>
          <span className="pc-row-name mono">{a.binding.workdir ? shortDir(a.binding.workdir) : nodeName(a, r.hub)}</span>
          <span className="pc-row-sub">
            {ps.length > 0 ? <span>{L(`在「${ps.map((p) => p.title || p.name).join("」「")}」里`, `In ${ps.map((p) => p.title || p.name).join(", ")}`)}</span> : <span>{L("不在项目里", "In no project")}</span>}
            {r.st.state === "offline" && <span>{L("不在线", "Away")}</span>}
          </span>
        </button>
        <button type="button" className="app-btn app-btn-small" onClick={() => onOpen(r)}>{L("打开", "Open")}</button>
        <button type="button" className="app-btn app-btn-small pc-settings" onClick={() => onSettings(r)} title={L("谁能用、项目、移除", "Who can use it, projects, remove")}><Settings2 />{L("设置", "Settings")}</button>
      </div>
    );
  };
  const agentRow = (r: AgentRow) => {
    const a = r.st.agent;
    const ps = projectsOf(r, projects);
    const quota = agentQuotaHint(a);
    return (
      <button key={`${r.hub.id}/${a.name}`} type="button" className={cn("pc-row pcd-agent", r.st.state === "offline" && "is-away")} onClick={() => onProfile(r)}
        title={a.binding.workdir || undefined}>
        <Avatar agent={a} size={28} state={r.st.state} />
        <span className="pc-row-main">
          <span className="pc-row-name">{nodeName(a, r.hub)}</span>
          <span className="pc-row-sub">
            <span>{stateWords(r.st.state, a.status_text)}</span>
            {a.binding.workdir && <span className="mono">{shortDir(a.binding.workdir)}</span>}
            {ps.length > 0 && <span>{L(`在「${ps.map((p) => p.title || p.name).join("」「")}」里`, `In ${ps.map((p) => p.title || p.name).join(", ")}`)}</span>}
            {quota && <span className={cn("pc-quota", quota.level === "spent" && "is-spent")} title={quota.title}>{quota.text}</span>}
          </span>
        </span>
        <ChevronRight className="pcd-go" />
      </button>
    );
  };

  // ── One computer, opened ──
  if (opened) {
    const { m, hub } = opened;
    const { agents, terminals } = split(hub.id, m);
    const Icon = osInfo(m.os).icon;
    return (
      <div className="pc pcd" data-testid="pc-detail">
        <button type="button" className="pcd-back" onClick={() => { setRenaming(false); openComputer(null); }}><ArrowLeft />{L("电脑", "Computers")}</button>
        <header className={cn("pcd-head", !m.online && "is-away")}>
          <span className="tp-icon pcd-icon"><Icon /></span>
          <div className="pcd-titles">
            {renaming ? (
              <RenameComputer hubId={hub.id} machine={m} taken={machines.filter((x) => x.hub.id === hub.id && x.m.id !== m.id).map((x) => x.m.name)}
                onDone={() => { setRenaming(false); onChanged(); }} />
            ) : (
              <h1>
                <span>{m.name}</span>
                {isHere(m) && <span className="pc-machine-tag">{L("这台电脑", "This computer")}</span>}
                <button type="button" className="app-btn app-btn-plain pc-rename" onClick={() => setRenaming(true)} aria-label={L("改名", "Rename")} title={L("改名", "Rename")}><Pencil /></button>
              </h1>
            )}
            <p className="pcd-sub">
              <span className={cn("pc-online", m.online && "is-on")}>{m.online ? L("在线", "Online") : L("不在线", "Away")}</span>
              <span>{osInfo(m.os).label}{m.arch ? ` · ${m.arch}` : ""}</span>
              {m.hostname && <span className="mono">{m.hostname}</span>}
              {m.myco_version && <span>myco {ver(m.myco_version)}</span>}
            </p>
          </div>
          <div className="ph-actions">
            <button type="button" className="app-btn" onClick={() => onAddTerminal(m)} disabled={!m.can_spawn}
              title={m.can_spawn ? undefined : L("这台电脑还没允许开终端：见下面「它允许什么」", "This computer does not allow terminals yet: see below")}><SquareTerminal />{L("开终端", "Terminal")}</button>
            <button type="button" className="app-btn app-btn-primary" onClick={() => onAddAgentOn(m)}><Plus />{L("加 agent", "Add an agent")}</button>
          </div>
        </header>

        <SectionHead title="Agent" count={agents.length} />
        {agents.length ? <div className="pc-list">{agents.map(agentRow)}</div> : (
          <div className="pcd-empty">
            <span>{L("这台电脑上还没有 agent。可以把它上面已有的会话加进来，或者新开一个。", "No agents on it yet: add a session it already has, or start a new one.")}</span>
            <button type="button" className="app-btn app-btn-small" onClick={() => onAddAgentOn(m)}><Plus />{L("加 agent", "Add an agent")}</button>
          </div>
        )}

        <SectionHead title={L("终端", "Terminals")} count={terminals.length} />
        {terminals.length ? <div className="pc-list">{terminals.map(terminalRow)}</div> : (
          <div className="pcd-empty">
            <span>{L("还没有终端：在它的一个文件夹里开一个，agent 就能在里面跑命令，你也能看着。", "No terminals yet: open one in a folder of it, for agents to run commands in while you watch.")}</span>
            <button type="button" className="app-btn app-btn-small" onClick={() => onAddTerminal(m)} disabled={!m.can_spawn}><SquareTerminal />{L("开终端", "Terminal")}</button>
          </div>
        )}

        <SectionHead title={L("它允许什么", "What it allows")} />
        <ComputerDetails hubId={hub.id} hub={hub} machine={m} onChanged={onChanged} />

        <div className="pcd-danger">
          <div>
            <b>{L("移除这台电脑", "Remove this computer")}</b>
            <span>{L("它和上面的 agent、终端都从工作区里拿掉；聊过的内容保留。", "It leaves the workspace with its agents and terminals; conversations are kept.")}</span>
          </div>
          <button type="button" className="app-btn app-btn-danger-quiet" onClick={() => setDropping(m)}><Trash2 />{L("移除", "Remove")}</button>
        </div>
        <RemoveComputer machine={dropping} rows={rows} onClose={() => setDropping(null)} onRemoved={() => { setDropping(null); openComputer(null); onChanged(); }} />
      </div>
    );
  }

  // ── All of them ──
  const online = machines.filter((x) => x.m.online).length;
  const agentCount = machines.reduce((n, x) => n + split(x.hub.id, x.m).agents.length, 0);
  const terminalCount = machines.reduce((n, x) => n + split(x.hub.id, x.m).terminals.length, 0) + loose.length;
  return (
    <div className="pc pcx">
      <PageHero title={L("电脑", "Computers")}
        subtitle={L("agent 跑在这些电脑上：笔记本、云服务器、WSL 各算一台。点开一台看它上面的 agent 和终端。", "Your agents run on these: a laptop, a cloud server or WSL each counts as one. Open one for its agents and terminals.")}>
        <button type="button" className="app-btn app-btn-primary" onClick={onConnect}><Plus />{L("连一台电脑", "Connect a computer")}</button>
      </PageHero>
      {machines.length > 0 && (
        <div className="au-stats pcx-stats">
          <div className="au-stat"><span>{L("在线", "Online")}</span><b>{online}<small className="pcx-of">/ {machines.length}</small></b></div>
          <div className="au-stat"><span>Agent</span><b className={cn(!agentCount && "is-quiet")}>{agentCount}</b></div>
          <div className="au-stat"><span>{L("终端", "Terminals")}</span><b className={cn(!terminalCount && "is-quiet")}>{terminalCount}</b></div>
        </div>
      )}
      <SectionHead title={L("我的电脑", "Computers")} count={machines.length || undefined} />
      {machines.length === 0 ? (
        <div className="au-none pcx-none" data-testid="pc-empty">
          <span className="tp-icon"><Monitor /></span>
          <div>
            <b>{L("还没有连上的电脑", "No computers yet")}</b>
            <p>{L("连上以后，它上面的 Claude Code、Codex 会话点一下就能加进来，也能在上面新开会话、开终端。", "Once connected, its Claude Code and Codex sessions join with a click, and you can start new ones or open terminals on it.")}</p>
          </div>
        </div>
      ) : (
        <div className="pcx-grid">
          {machines.map(({ m, hub, key }) => {
            const { agents, terminals } = split(hub.id, m);
            const Icon = osInfo(m.os).icon;
            const quota = computerQuotaHint(m.id, hub.id, rows);
            return (
              <button key={key} type="button" className={cn("pc-card", !m.online && "is-away")} data-testid="pc-card" onClick={() => openComputer(key)}>
                <span className="pc-card-top">
                  <span className="tp-icon"><Icon /></span>
                  <span className="pc-card-text">
                    <span className="pc-card-name"><span>{m.name}</span>{isHere(m) && <span className="pc-machine-tag">{L("这台电脑", "This computer")}</span>}</span>
                    <span className="pc-card-sub">
                      <span className={cn("pc-online", m.online && "is-on")}>{m.online ? L("在线", "Online") : L("不在线", "Away")}</span>
                      <span>{osInfo(m.os).label}</span>
                      {m.myco_version && <span>myco {ver(m.myco_version)}</span>}
                    </span>
                  </span>
                  <ChevronRight className="pc-card-go" />
                </span>
                <span className="pc-card-foot">
                  {agents.length > 0 && <AvatarStack agents={agents.map((r) => r.st.agent)} size={20} max={4} />}
                  <span className="pc-card-count">
                    {agents.length || terminals.length
                      ? [agents.length ? L(`${agents.length} 个 agent`, `${agents.length} agent${agents.length === 1 ? "" : "s"}`) : "", terminals.length ? L(`${terminals.length} 个终端`, `${terminals.length} terminal${terminals.length === 1 ? "" : "s"}`) : ""].filter(Boolean).join(" · ")
                      : L("上面还没有 agent", "Nothing on it yet")}
                  </span>
                  {quota && <span className={cn("pc-quota", quota.level === "spent" && "is-spent")} title={quota.title}>{quota.text}</span>}
                </span>
              </button>
            );
          })}
        </div>
      )}
      {loose.length > 0 && (
        <>
          <SectionHead title={L("其他终端", "Other terminals")} count={loose.length} />
          <div className="pc-list">{loose.map(terminalRow)}</div>
        </>
      )}
    </div>
  );
}

/** Changes a computer's name, as everyone in the workspace sees it (it
 *  must not be another computer's). Enter saves, Esc leaves it. */
function RenameComputer({ hubId, machine, taken, onDone }: { hubId: string; machine: Machine; taken: string[]; onDone: () => void }) {
  const [name, setName] = useState(machine.name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const clash = taken.some((t) => t.trim().toLowerCase() === name.trim().toLowerCase());
  const save = async () => {
    const n = name.trim();
    if (!n || n === machine.name) { onDone(); return; }
    if (clash || busy) return;
    setBusy(true);
    setError(null);
    try {
      await appApi.renameMachine(hubId, machine.id, n);
      onDone();
    } catch (e) {
      setError((e as { code?: string })?.code === "conflict" ? new Error(L("已经有一台电脑叫这个名字了。", "Another computer already has this name.")) : e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="pc-rename-form" onSubmit={(e) => { e.preventDefault(); void save(); }}>
      <input className="app-input" value={name} autoFocus onChange={(e) => setName(e.target.value)} disabled={busy} maxLength={64}
        onKeyDown={(e) => { if (e.key === "Escape") { e.preventDefault(); onDone(); } }} aria-label={L("电脑的名字", "Computer name")} />
      <button type="submit" className="app-btn app-btn-small app-btn-primary" disabled={busy || !name.trim() || clash}>{L("保存", "Save")}</button>
      <button type="button" className="app-btn app-btn-small" onClick={onDone} disabled={busy}>{L("取消", "Cancel")}</button>
      {clash && <span className="app-field-hint is-bad">{L("已经有一台电脑叫这个名字了。", "Another computer already has this name.")}</span>}
      <ErrorText error={error} />
    </form>
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
function ComputerDetails({ hubId, hub, machine, onChanged }: {
  hubId: string;
  hub: AgentRow["hub"];
  machine: Machine;
  onChanged: () => void;
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
          <div className="app-kv"><span className="app-kv-label">{L("agent 能在哪里开工", "Where agents may start")}</span>
            <span className="app-kv-value pcd-chips">{info.dirs.map((d) => <span key={d} className="pcd-chip mono">{d}</span>)}</span></div>
          <div className="app-kv"><span className="app-kv-label">{L("最高权限", "Highest access")}</span><span className="app-kv-value">{(ACCESS_LABEL[info.max_access] || (() => info.max_access))()}</span></div>
          <div className="app-kv"><span className="app-kv-label">AI</span>
            <span className="app-kv-value pcd-chips">{info.ais.map((a) => (
              <span key={a.name} className={cn("pcd-chip pcd-ai", `is-${a.state}`)}>{`${a.name} ${aiText(a)}`}</span>
            ))}</span></div>
          <div className="app-small-note">{L("这些由那台电脑自己决定，只能在它上面改（~/.myco/machine.yaml 的 spawn.dirs、spawn.max_access）。", "That computer decides these itself; they are changed on it only (spawn.dirs and spawn.max_access in ~/.myco/machine.yaml).")}</div>
        </div>
      )}
      <LocalShellAccess machine={machine} hub={hub} what="shell" shell={machineShell(hub, machine.id)} onChange={async () => onChanged()} />
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
