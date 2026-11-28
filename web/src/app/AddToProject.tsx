import { useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, Cloud, FileText, Globe, Info, Laptop, Plus, Upload, Users, X } from "lucide-react";
import { ConnectOver } from "@/app/ConnectOver";
import { ClaudeAuthFix, isClaudeAuthError } from "@/app/ClaudeAuthFix";
import { appApi, type Hub, type JoinLink, type LocalSession, type LocalState, type MachineInfo, type MachineSession, type SpawnCli } from "@/appapi";
import { IconAsterisk, IconCode } from "@/components/icons";
import type { Machine, Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, machineShell, machinesOfHub, nodeName, osInfo, sessionDisplayName, thisComputerId } from "@/app/nodes";
import { Avatar, isWorkspace, whereOf } from "@/app/persona";
import { CopyButton } from "@/app/copy";
import { computerPrompt } from "@/app/agentSetup";
import { LocalShellAccess } from "@/app/LocalShellAccess";
import { Elapsed, Mushroom } from "@/app/Working";
import { ErrorText, Modal } from "@/app/dialogs";
import { LocalAgentLines } from "@/app/LocalAgents";
import { AllowFolder } from "@/app/MachineDirs";
import { EFFORTS, MODELS, effortChoices, effortLabel, modelChoices, modelName } from "@/app/RunSettings";
import { machineMakesFolders, machineTellsInfo } from "@/app/hubVersion";
import { ComputerFix } from "@/app/ComputerFix";
import { askable, cachedMachineInfo, fetchMachineInfo, forgetMachineInfo } from "@/app/machineInfoCache";
import { PublicAccessDialog } from "@/app/SettingsPage";
import { localWorkspaceOf } from "@/app/localHubs";
import type { RuntimeAccess } from "@/api";
import { L, E } from "@/lib/i18n";
import { bytes, cn, timeAgo } from "@/lib/utils";
import { ModelSource } from "@/app/ModelProviders";
import { isSuperagent } from "@/app/superagentMark";

/**
 * Lets a new member use the project's workspaces: each one's "who may run
 * commands" gets the agent added (a workspace always names who may).
 */
export async function shareWorkspaces(hubId: string, project: Project, rows: AgentRow[], agent: string) {
  const members = new Set(project.members.map((m) => m.name));
  const spaces = rows.filter((r) => r.hub.id === hubId && members.has(r.st.agent.name) && isWorkspace(r.st.agent));
  await Promise.all(spaces.map((r) => {
    const now = (r.st.agent.allow_from || []).filter((x) => x !== "*" && !isNobody(x));
    if (now.includes(agent)) return null;
    return appApi.addAllowedSenders(hubId, r.st.agent.name, [agent]);
  }));
}

/**
 * A computer added to a project before any agent is in it is locked to a
 * name no agent has (an empty list would let every agent in); the first
 * agents to join take its place.
 */
export function nobody(): string {
  const a = new Uint8Array(8);
  crypto.getRandomValues(a);
  return `nobody-${[...a].map((b) => (b % 36).toString(36)).join("")}`;
}
export function isNobody(name: string): boolean {
  return /^nobody-[a-z0-9]{8}$/.test(name);
}

/** Puts an agent in a project, and lets it use the project's workspaces. */
export async function addToProject(hubId: string, project: Project | undefined, rows: AgentRow[], agent: string) {
  if (!project) return;
  let p = await appApi.putProjectMember(hubId, project.name, agent);
  // The first agent in a project is its coordinator (there is always one).
  const isAgent = (n: string) => rows.some((r) => r.hub.id === hubId && r.st.agent.name === n && agentKind(r.st.agent) === "agent");
  if (isAgent(agent) && !p.members.some((m) => m.role === "coordinator" && isAgent(m.name))) {
    p = await appApi.putProjectMember(hubId, project.name, agent, { role: "coordinator" }).catch(() => p);
  }
  await shareWorkspaces(hubId, p, rows, agent);
}

// What to tell an agent so it joins. A hub that guides agents itself
// (agent_guide) takes the project in the link, and its script ends with
// the last step: the sentence is just the command. Older hubs need the
// steps spelled out.
export function joinSentence(link: JoinLink, project?: string): string {
  if (link.agent_guide) {
    const command = project ? `curl -fsSL ${link.join_url}/${project} | bash` : link.command;
    return L(E`加入 myco：在终端运行 ${command} ，再照它最后的输出做完。`, E`Join myco: run ${command} in a terminal, then do what its last lines say.`);
  }
  const into = project ? ` --project ${project}` : "";
  return L(
    E`加入 myco：在终端运行 ${link.command} ，然后在你这个会话里运行 myco join${into} --display-name "你的分工" --description "一句话说你负责什么"（把引号里的换成你自己的：分工用 2 到 6 个字，比如 后端工程师、审查员、运维）。之后照 myco join 输出里 receive 的说明接收消息：会话开着时，在后台一直运行 myco inbox wait，收到消息用 myco inbox pull 读取并处理。`,
    E`Join myco: run ${link.command} in a terminal, then in this session run myco join${into} --display-name "your role" --description "one line on what you do" (fill in your own: a short role such as Backend engineer, Reviewer or Ops). Then receive messages as its "receive" output says: while the session is open keep myco inbox wait running in the background, and read what it reports with myco inbox pull.`,
  );
}

const NEW_CLIS: { value: SpawnCli; label: string }[] = [
  { value: "claude", label: "Claude Code" },
  { value: "codex", label: "Codex" },
  { value: "gemini", label: "Gemini CLI" },
  { value: "cursor", label: "Cursor" },
  { value: "opencode", label: "OpenCode" },
  { value: "kiro", label: "Kiro" },
  { value: "pi", label: "Pi" },
  { value: "qoder", label: "Qoder CLI" },
  { value: "qwen", label: "Qwen Code" },
  { value: "kimi", label: "Kimi Code" },
  { value: "goose", label: "goose" },
  { value: "copilot", label: "GitHub Copilot" },
  { value: "auggie", label: "Auggie" },
  { value: "codebuddy", label: "CodeBuddy Code" },
  { value: "kilo", label: "Kilo Code" },
  { value: "cline", label: "Cline" },
  { value: "grok", label: "Grok" },
  { value: "antigravity", label: "Antigravity" },
  { value: "atomcode", label: "AtomCode" },
];

/** The AIs shown at once in a new agent's "用哪个 AI"; the rest wait
 *  behind "更多" unless the computer has them installed (or one is picked). */
const COMMON_CLIS: SpawnCli[] = ["claude", "codex", "gemini", "cursor"];

/** What a computer told about itself, as the dialog keeps it. */
type InfoState = { info?: MachineInfo; error?: unknown; loading?: boolean };

/** Where an agent comes from: one already in the workspace, a computer
 *  ("here", or "m:<machine id>"), a computer to connect, or an AI
 *  elsewhere (claude.ai, any cloud session) told with a sentence. */
type Where = "members" | "here" | "connect" | "elsewhere" | `m:${string}`;

/**
 * Adding an agent starts with where it is: one already in this workspace,
 * this computer, another computer you connected (a VPS, WSL), a computer to
 * connect now, or an AI elsewhere. On a computer, either a session already
 * there (a copy of it joins) or a new one. Whoever joins while this is open
 * shows up at the bottom, and goes into the project.
 */
export function AddAgentDialog({ open, hub, project, projects = [], rows, start, onClose, onAdded }: {
  open: boolean;
  hub?: Hub;
  /** where to begin: "connect" (a computer to connect), or "m:<machine id>" */
  start?: string;
  project?: Project;
  /** the workspace's projects: where the members already are */
  projects?: Project[];
  rows: AgentRow[];
  onClose: () => void;
  onAdded: () => void;
}) {
  const hubId = hub?.id || "";
  const [where, setWhere] = useState<Where>("here");
  const [how, setHow] = useState<"existing" | "new">("existing");
  const touched = useRef(false);
  const [local, setLocal] = useState<LocalState | null>(null);
  const [link, setLink] = useState<JoinLink | null>(null);
  const [linkError, setLinkError] = useState<unknown>(null);
  // The sentence's join link; again from 重试 when the hub did not answer.
  const makeLink = () => {
    if (!hubId) return;
    setLinkError(null);
    appApi.createJoinLink(hubId, { uses: 10, expires_hours: 168 }).then(setLink).catch(setLinkError);
  };
  const [joined, setJoined] = useState<string[]>([]);
  const [sessions, setSessions] = useState<LocalSession[] | null>(null);
  const [sessionsError, setSessionsError] = useState<unknown>(null);
  const [query, setQuery] = useState("");
  const [joining, setJoining] = useState("");
  const [joinError, setJoinError] = useState<unknown>(null);
  const [newState, setNewState] = useState({ ready: false, busy: false });
  const [infos, setInfos] = useState<Record<string, InfoState>>({});
  const ov = rows.find((r) => r.hub.id === hubId)?.hub;
  const machines = ov ? machinesOfHub(ov) : [];
  const hereId = ov ? thisComputerId(ov, local) : undefined;
  const isHere = (m: Machine) => m.id === hereId;
  const hereMachine = machines.find(isHere);
  const elsewhere = machines.filter((m) => m.online && !isHere(m))
    .sort((a, b) => a.name.localeCompare(b.name));
  // Agents of this workspace not in the project yet (in others, or none):
  // online first.
  function others(): AgentRow[] {
    const inIt = new Set((project?.members || []).map((m) => m.name));
    // (not a superagent: it works for you across the workspace, in no project)
    return rows.filter((r) => r.hub.id === hubId && agentKind(r.st.agent) === "agent" && !inIt.has(r.st.agent.name) && !isSuperagent(r.st.agent))
      .sort((a, b) => Number(a.st.state === "offline") - Number(b.st.state === "offline") || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub)));
  }
  // Who was already there when this opened: anyone new has just joined.
  const before = useRef<Set<string> | null>(null);
  // And who was in the project: an agent already in the workspace that
  // joins the project by itself (the sentence's link names it) shows up too.
  const inProject = useRef<Set<string> | null>(null);
  const adding = useRef(new Set<string>());

  useEffect(() => {
    if (!open) return;
    before.current = new Set(rows.filter((r) => r.hub.id === hubId).map((r) => r.st.agent.name));
    inProject.current = new Set((project?.members || []).map((m) => m.name));
    adding.current = new Set();
    touched.current = false;
    // Someone already here who is not in the project: that is the easy way.
    setWhere(start === "connect" || start?.startsWith("m:") ? start as Where : project && others().length > 0 ? "members" : "here");
    setHow("existing");
    setJoined([]);
    setLink(null);
    setLinkError(null);
    setJoinError(null);
    setSessions(null);
    setSessionsError(null);
    setQuery("");
    setInfos({});
    appApi.local().then(setLocal).catch(() => setLocal(null));
    if (!hubId) return;
    makeLink();
    appApi.localSessions().then((r) => {
      setSessions(r.sessions);
      // Nothing on this computer to add: starting one is the way.
      if (!touched.current && !r.sessions.some((s) => !s.joined)) setHow("new");
    }).catch((e) => {
      setSessionsError(e);
      if (!touched.current) setHow("new");
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, hubId]);

  // Opened on this computer by its id: it is "this computer".
  useEffect(() => {
    if (open && hereMachine && where === `m:${hereMachine.id}`) setWhere("here");
  }, [open, hereMachine, where]);
  // What a computer allows and has: asked once it is picked (and online).
  const machineOf = (w: Where): Machine | undefined =>
    w === "here" ? hereMachine : w.startsWith("m:") ? machines.find((m) => m.id === w.slice(2)) : undefined;
  const picked = machineOf(where);
  // The last answer shows at once; a fresh one replaces it when it comes.
  const loadInfo = (m: Machine, fresh = false) => {
    if (fresh) forgetMachineInfo(hubId, m.id);
    const known = cachedMachineInfo(hubId, m.id);
    setInfos((s) => ({ ...s, [m.id]: known?.info ? { info: known.info } : { loading: true } }));
    fetchMachineInfo(hubId, m.id)
      .then((info) => setInfos((s) => ({ ...s, [m.id]: { info } })))
      .catch((error) => setInfos((s) => (s[m.id]?.info ? s : { ...s, [m.id]: { error } })));
  };
  // Every computer that can answer is asked as soon as this opens (they
  // answer from memory), so picking one shows its sessions at once.
  useEffect(() => {
    if (!open) return;
    for (const m of machines) if (askable(m) && !infos[m.id]) loadInfo(m);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, machines.map((m) => `${m.id}:${m.online}`).join(",")]);

  // New agents on this hub since it opened: into the project, and on show.
  useEffect(() => {
    if (!open || !before.current) return;
    for (const r of rows) {
      if (r.hub.id !== hubId || before.current.has(r.st.agent.name) || adding.current.has(r.st.agent.name)) continue;
      if (agentKind(r.st.agent) !== "agent") continue;
      adding.current.add(r.st.agent.name);
      const name = r.st.agent.name;
      void addToProject(hubId, project, rows, name).catch(() => undefined).finally(() => {
        setJoined((j) => (j.includes(name) ? j : [...j, name]));
        onAdded();
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, open]);
  useEffect(() => {
    if (!open || !project || !inProject.current) return;
    for (const m of project.members) {
      if (inProject.current.has(m.name)) continue;
      inProject.current.add(m.name);
      const r = rows.find((x) => x.hub.id === hubId && x.st.agent.name === m.name);
      if (!r || agentKind(r.st.agent) !== "agent") continue;
      adding.current.add(m.name);
      setJoined((j) => (j.includes(m.name) ? j : [...j, m.name]));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project, rows, open]);

  const arrived = async (agent: string) => {
    adding.current.add(agent);
    await addToProject(hubId, project, rows, agent).catch(() => undefined);
    setJoined((j) => (j.includes(agent) ? j : [...j, agent]));
    onAdded();
  };

  const joinLocal = async (s: LocalSession) => {
    setJoining(s.session_id);
    setJoinError(null);
    try {
      const r = await appApi.localJoin(hubId, { cli: s.cli, session_id: s.session_id, workdir: s.workdir, fork: true, display_name: sessionDisplayName(s.title) });
      await arrived(r.agent);
    } catch (e) {
      setJoinError(e);
    } finally {
      setJoining("");
    }
  };
  // A session on another computer: a copy of it starts there and joins.
  const joinRemote = async (m: Machine, s: MachineSession) => {
    setJoining(s.session_id);
    setJoinError(null);
    try {
      const r = await appApi.spawn(hubId, m.id, { cli: s.cli, dir: s.workdir, fork_from: s.session_id, display_name: sessionDisplayName(s.title) });
      await arrived(r.agent);
      loadInfo(m, true);
    } catch (e) {
      setJoinError(e);
    } finally {
      setJoining("");
    }
  };

  const go = (w: Where) => {
    touched.current = true;
    setJoinError(null);
    setWhere(w);
    if (w !== "here") setHow("existing");
  };
  const addMember = async (name: string) => {
    setJoining(name);
    setJoinError(null);
    try {
      await arrived(name);
    } catch (e) {
      setJoinError(e);
    } finally {
      setJoining("");
    }
  };
  const rowOf = (n: string) => rows.find((r) => r.hub.id === hubId && r.st.agent.name === n);
  const place = project ? L(E`「${project.title || project.name}」`, E`“${project.title || project.name}”`) : L("这个工作区", "this workspace");
  const q = query.trim().toLowerCase();
  const list = (sessions || []).filter((s) => !q || [s.title, s.workdir, s.cli].some((x) => (x || "").toLowerCase().includes(q)));
  const free = list.filter((s) => !s.joined);
  const taken = list.filter((s) => s.joined);
  const sentence = link ? joinSentence(link, project?.name) : "";
  const onComputer = where === "here" || where.startsWith("m:");
  const pickedInfo = picked ? infos[picked.id] : undefined;
  const showMembers = !!project && (others().length > 0 || where === "members");

  return (
    <Modal open={open} onOpenChange={(o) => !o && !joining && onClose()} title={L("添加 agent", "Add an agent")}
      description={L(E`给${place}加一个 AI 同事。先选它在哪。`, E`Bring an AI teammate into ${place}. First, where is it?`)}
      className="app-modal-add">
      <div className="app-field">
        <span className="app-field-label">{L("它在哪", "Where it is")}</span>
        <div className="ws-computers" role="tablist" aria-label={L("它在哪", "Where it is")} data-testid="add-where">
          {showMembers && (
            <button type="button" role="tab" aria-selected={where === "members"} className={cn("ws-computer", where === "members" && "active")} onClick={() => go("members")}>
              <Users /><span className="ws-computer-name">{L("工作区里已有的", "Already in the workspace")}</span>
            </button>
          )}
          <button type="button" role="tab" aria-selected={where === "here"} className={cn("ws-computer", where === "here" && "active")} onClick={() => go("here")}>
            <Laptop /><span className="ws-computer-name">{L("这台电脑", "This computer")}</span>
          </button>
          {elsewhere.map((m) => {
            const Icon = osInfo(m.os).icon;
            const w: Where = `m:${m.id}`;
            return (
              <button key={m.id} type="button" role="tab" aria-selected={where === w} className={cn("ws-computer", where === w && "active", !m.online && "is-away")} onClick={() => go(w)}>
                <Icon /><span className="ws-computer-name">{m.name}</span>
                {!m.online && <span className="ws-computer-sub">{L("不在线", "Away")}</span>}
              </button>
            );
          })}
          <button type="button" role="tab" aria-selected={where === "connect"} className={cn("ws-computer is-new", where === "connect" && "active")} onClick={() => go("connect")}>
            <Plus /><span className="ws-computer-name">{L("连一台电脑", "Connect a computer")}</span>
          </button>
          <button type="button" role="tab" aria-selected={where === "elsewhere"} className={cn("ws-computer", where === "elsewhere" && "active")} onClick={() => go("elsewhere")}
            title={L("claude.ai、云端会话，或任何会跑命令的 AI", "claude.ai, cloud sessions, any AI that runs commands")}>
            <Cloud /><span className="ws-computer-name">{L("云端或别处的 AI", "An AI in the cloud or elsewhere")}</span>
          </button>
        </div>
      </div>

      {onComputer && (
        <div className="seg seg-fill add-tabs" role="tablist" aria-label={L("怎么加", "How")}>
          {([["existing", L("用上面已有的会话", "A session already there")], ["new", L("新开一个", "Start a new one")]] as const).map(([v, label]) => (
            <button key={v} type="button" role="tab" aria-selected={how === v} className={cn("seg-btn", how === v && "active")}
              onClick={() => { touched.current = true; setHow(v); }}>{label}</button>
          ))}
        </div>
      )}

      {where === "members" && (
        <div className="add-pane">
          {others().length === 0 && joined.length === 0 ? (
            <div className="app-empty-note">{L("这个工作区里的 agent 都在这个项目里了。", "Every agent in this workspace is in this project already.")}</div>
          ) : (
            <div className="add-sessions">
              {others().map((r) => {
                const a = r.st.agent;
                const inWhich = projects.filter((p) => !p.archived && p.members.some((m) => m.name === a.name)).map((p) => p.title || p.name);
                return (
                  <div key={a.name} className={cn("add-local-row", r.st.state === "offline" && "is-away")}>
                    <Avatar agent={a} size={30} state={r.st.state} />
                    <span className="add-local-main">
                      <span className="add-local-name">{nodeName(a, r.hub)}</span>
                      <span className="add-local-sub">{inWhich.length ? L(E`在「${inWhich.join("」「")}」`, E`In ${inWhich.join(", ")}`) : L("还没进项目", "Not in a project")}{a.description ? ` · ${a.description}` : ""}</span>
                    </span>
                    <button type="button" className="app-btn app-btn-small" disabled={!!joining} onClick={() => void addMember(a.name)}>
                      {joining === a.name ? L("正在加…", "Adding…") : L("加进来", "Add")}
                    </button>
                  </div>
                );
              })}
            </div>
          )}
          <ErrorText error={joinError} />
        </div>
      )}

      {where === "here" && how === "existing" && (
        <div className="add-pane">
          {sessions === null && !sessionsError ? (
            <div className="add-waiting"><Mushroom /><span>{L("正在找这台电脑上的会话…", "Looking for sessions on this computer…")}</span></div>
          ) : sessionsError ? (
            <ErrorText error={sessionsError} />
          ) : (sessions || []).length === 0 ? (
            <div className="app-empty-note">{L("这台电脑上最近没有 Claude Code 或 Codex 会话。可以「新开一个」。", "No recent Claude Code or Codex sessions here. Start a new one.")}</div>
          ) : (
            <>
              {(sessions || []).length > 6 && (
                <input className="app-input add-search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={L("找一个会话：标题、文件夹", "Find a session: title, folder")} aria-label={L("搜索会话", "Search sessions")} />
              )}
              <div className="add-sessions">
                {(() => {
                  const row = (s: LocalSession) => (
                    <SessionRow key={`${s.cli}:${s.session_id}`} cli={s.cli} title={s.title} workdir={s.workdir} at={s.last_active}
                      action={<button type="button" className="app-btn app-btn-small" disabled={!!joining} onClick={() => void joinLocal(s)}>
                        {joining === s.session_id ? L("正在加入…", "Joining…") : L("加入", "Add")}
                      </button>} />
                  );
                  return (
                    <>
                      {free.filter((s) => !s.auto).map(row)}
                      <ByProgram count={free.filter((s) => s.auto).length} open={!!q}>{free.filter((s) => s.auto).map(row)}</ByProgram>
                    </>
                  );
                })()}
                {taken.map((s) => (
                  <SessionRow key={`${s.cli}:${s.session_id}`} cli={s.cli} title={s.title} workdir={s.workdir} taken sub={joinedLine(s)} subTitle={joinedWhere(s).join("\n")} />
                ))}
                {list.length === 0 && <div className="app-empty-note">{L("没找到。", "Nothing matches.")}</div>}
              </div>
              <div className="add-note">{L("会复制一份会话加入，你原来的会话照常用。", "A copy of the session joins; yours carries on as before.")}</div>
              <ErrorText error={joinError} />
            </>
          )}
        </div>
      )}

      {where.startsWith("m:") && how === "existing" && picked && (
        <RemoteSessions hubId={hubId} machine={picked} state={pickedInfo} joining={joining} joinError={joinError}
          onJoin={(s) => void joinRemote(picked, s)} onRetry={() => loadInfo(picked)} />
      )}

      {onComputer && how === "new" && (
        picked ? (
          <NewAgentPane open={open && how === "new"} hubId={hubId} rows={rows} machine={picked} here={where === "here"} state={pickedInfo}
            onStarted={(a) => void arrived(a)} onState={setNewState} onReloadInfo={() => picked && loadInfo(picked, true)} />
        ) : (
          <div className="add-pane"><div className="app-empty-note">{L("这台电脑还没连上这个工作区，开不了新会话。点「连一台电脑」把它连上。", "This computer is not connected to this workspace, so it cannot start sessions. Connect it with Connect a computer.")}</div></div>
        )
      )}

      {where === "connect" && (
        <ConnectComputer open={open && where === "connect"} hub={hub} machines={machines} local={local}
          onConnected={(id) => { setWhere(`m:${id}`); setHow("existing"); }} />
      )}

      {where === "elsewhere" && (
        <div className="add-pane">
          <div className="add-note">{L("把这句话发给你的 agent，它会自己装好 myco 加入。claude.ai、云端的会话，或任何会跑命令的 AI 都行，可以发给好几个。", "Send this to your agent; it installs myco and joins by itself. claude.ai, a cloud session, or any AI that runs commands; send it to as many as you like.")}</div>
          {link && <ReachNote url={link.join_url} hubUrl={hub?.url || ""} local={local} onOpened={() => setLink(null)} onRelink={makeLink} />}
          <div className="add-sentence">
            {link ? (
              <>
                <p className="add-sentence-text">{sentence}</p>
                <div className="add-sentence-actions"><CopyButton value={sentence} /></div>
              </>
            ) : linkError ? <ErrorText error={linkError} onRetry={makeLink} /> : <p className="add-sentence-text is-loading">{L("正在准备…", "Getting it ready…")}</p>}
          </div>
          {joined.length === 0 && (
            <div className="add-waiting"><Mushroom idle /><span>{L("发出去以后，加入的 agent 会出现在下面。", "Once sent, the agents that join show up below.")}</span></div>
          )}
        </div>
      )}

      {/* Who has joined while this was open, whichever way. */}
      {joined.length > 0 && (
        <div className="add-joined" aria-live="polite">
          {joined.map((n) => {
            const r = rowOf(n);
            return (
              <div key={n} className="add-arrived">
                {r && <Avatar agent={r.st.agent} size={28} />}
                <span><b>{r ? nodeName(r.st.agent, r.hub) : n}</b>{L(" 加入了", " joined")}</span>
                <Check className="add-ok" />
              </div>
            );
          })}
        </div>
      )}

      {/* One row of buttons: starting a new one is the main step there. */}
      <div className="app-modal-actions">
        {onComputer && how === "new" && picked ? (
          <>
            <button type="button" className="app-btn" onClick={onClose} disabled={newState.busy}>{joined.length ? L("好了", "Done") : L("取消", "Cancel")}</button>
            <button type="submit" form="add-new-agent" className="app-btn app-btn-primary" disabled={!newState.ready || newState.busy}>
              {newState.busy ? L("正在开…", "Starting…") : L("开一个", "Start it")}
            </button>
          </>
        ) : (
          <button type="button" className="app-btn app-btn-primary" onClick={onClose} disabled={!!joining}>{L("好了", "Done")}</button>
        )}
      </div>
    </Modal>
  );
}

/** One session in a list: its AI, its first words (or folder), where and when. */
function SessionRow({ cli, title, workdir, at, action, taken, sub, subTitle, index }: {
  /** set: the row fades in after the ones above it */
  index?: number;
  cli: string;
  title?: string;
  workdir: string;
  at?: string;
  action?: React.ReactNode;
  taken?: boolean;
  /** instead of "AI · folder · when" */
  sub?: string;
  subTitle?: string;
}) {
  const Icon = cli === "codex" ? IconCode : IconAsterisk;
  return (
    <div className={cn("add-local-row", taken && "is-taken", index !== undefined && "add-row-in")}
      style={index !== undefined ? { animationDelay: `${Math.min(index, 12) * 35}ms` } : undefined}>
      <span className="add-local-icon"><Icon /></span>
      <span className="add-local-main">
        <span className="add-local-name">{title || folderOf(workdir)}</span>
        <span className="add-local-sub" title={subTitle || workdir}>{sub || `${cli === "codex" ? "Codex" : "Claude Code"} · ${folderOf(workdir)}${at ? ` · ${timeAgo(at)}` : ""}`}</span>
      </span>
      {action}
    </div>
  );
}

/** Sessions a program ran (claude -p, codex exec, myco itself), folded
 *  under the ones people started: there are many, and seldom wanted. */
function ByProgram({ count, open, children }: { count: number; open: boolean; children: React.ReactNode }) {
  if (count === 0) return null;
  return (
    <details className="add-auto" open={open || undefined} data-testid="by-program">
      <summary>{L(E`还有 ${count} 个是程序开的会话（myco、脚本、claude -p、codex exec），一般不用加`, E`${count} more were started by a program (myco, scripts, claude -p, codex exec); seldom worth adding`)}<ChevronDown /></summary>
      {children}
    </details>
  );
}

/** The sessions on another computer: a copy of one joins (it starts there). */
function RemoteSessions({ hubId, machine, state, joining, joinError, onJoin, onRetry }: {
  hubId: string;
  machine: Machine;
  state?: InfoState;
  joining: string;
  joinError: unknown;
  onJoin: (s: MachineSession) => void;
  onRetry: () => void;
}) {
  const [query, setQuery] = useState("");
  if (!machine.online) {
    return <div className="add-pane"><div className="app-empty-note">{L(E`「${machine.name}」现在不在线：它开机、myco 在运行时，这里会列出它上面的会话。`, E`“${machine.name}” is away: once it is on and myco runs there, its sessions show here.`)}</div></div>;
  }
  if (!machine.can_spawn || !machineTellsInfo(machine.myco_version)) {
    return <div className="add-pane"><ComputerFix hubId={hubId} machine={machine} here={false} /></div>;
  }
  if (!state || state.loading) {
    return <div className="add-pane"><div className="add-waiting"><Mushroom /><span>{L(E`正在问「${machine.name}」上有哪些会话…`, E`Asking “${machine.name}” for its sessions…`)}</span></div></div>;
  }
  if (state.error || !state.info) {
    return (state.error as { code?: string })?.code === "too_old"
      ? <div className="add-pane"><ComputerFix hubId={hubId} machine={{ ...machine, myco_version: "" }} here={false} /></div>
      : <div className="add-pane"><ErrorText error={state.error} /><button type="button" className="app-btn app-btn-small" onClick={onRetry}>{L("再试一次", "Try again")}</button></div>;
  }
  const info = state.info;
  const q = query.trim().toLowerCase();
  const list = info.sessions.filter((s) => !q || [s.title, s.workdir, s.cli].some((x) => (x || "").toLowerCase().includes(q)));
  const free = list.filter((s) => !s.joined);
  const taken = list.filter((s) => s.joined);
  return (
    <div className="add-pane" data-testid="remote-sessions">
      {info.sessions.length === 0 ? (
        <div className="app-empty-note">{L(E`「${machine.name}」上最近没有 Claude Code 或 Codex 会话。可以「新开一个」。`, E`No recent Claude Code or Codex sessions on “${machine.name}”. Start a new one.`)}</div>
      ) : (
        <>
          {info.sessions.length > 6 && (
            <input className="app-input add-search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={L("找一个会话：标题、文件夹", "Find a session: title, folder")} aria-label={L("搜索会话", "Search sessions")} />
          )}
          <div className="add-sessions">
            {(() => {
              const row = (s: MachineSession, i: number) => {
              const ok = insideDirs(s.workdir, info);
              return (
                <SessionRow key={`${s.cli}:${s.session_id}`} index={i} cli={s.cli} title={s.title} workdir={s.workdir} at={s.last_active}
                  sub={ok ? undefined : L(E`${folderOf(s.workdir)} 不在那台电脑允许的文件夹里`, E`${folderOf(s.workdir)} is outside the folders that computer allows`)}
                  action={<button type="button" className="app-btn app-btn-small" disabled={!!joining || !ok} onClick={() => onJoin(s)}
                    title={ok ? undefined : L(E`允许的文件夹：${info.dirs.join("、")}`, E`Allowed: ${info.dirs.join(", ")}`)}>
                    {joining === s.session_id ? L("正在加入…", "Joining…") : L("加入", "Add")}
                  </button>} />
              );
              };
              const mine = free.filter((s) => !s.auto);
              const auto = free.filter((s) => s.auto);
              return (
                <>
                  {mine.map(row)}
                  <ByProgram count={auto.length} open={!!q}>{auto.map((s, i) => row(s, mine.length + i))}</ByProgram>
                </>
              );
            })()}
            {taken.map((s, i) => (
              <SessionRow key={`${s.cli}:${s.session_id}`} index={free.length + i} cli={s.cli} title={s.title} workdir={s.workdir} taken sub={L(E`已经加入了：${s.joined}`, E`Already joined: ${s.joined}`)} />
            ))}
            {list.length === 0 && <div className="app-empty-note">{L("没找到。", "Nothing matches.")}</div>}
          </div>
          <div className="add-note">{L("会在那台电脑上复制一份会话加入，原来的会话照常用。", "A copy of the session starts on that computer and joins; the original carries on as before.")}</div>
        </>
      )}
      <ErrorText error={joinError} />
    </div>
  );
}

/** Where a join link can be reached from, by its host: this computer only,
 *  its local network, a Tailscale network, or anywhere. */
export function reachOf(url: string): "local" | "lan" | "tailnet" | "public" {
  let host = "";
  try { host = new URL(url).hostname.replace(/^\[|\]$/g, "").toLowerCase(); } catch { return "public"; }
  if (host === "localhost" || host === "::1" || /^127\./.test(host)) return "local";
  const m = host.match(/^(\d+)\.(\d+)\.\d+\.\d+$/);
  if (m) {
    const [a, b] = [Number(m[1]), Number(m[2])];
    if (a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 169 && b === 254)) return "lan";
    if (a === 100 && b >= 64 && b <= 127) return "tailnet";
  }
  if (host.endsWith(".local") || host.endsWith(".lan") || host.endsWith(".home") || !host.includes(".")) return "lan";
  if (host.endsWith(".ts.net") && !/\.funnel\./.test(host)) return "public";
  return "public";
}

/** Before a command goes to another computer: whether it can reach this
 *  workspace at all, and (for a workspace on this computer) opening it to
 *  the internet right here. */
function ReachNote({ url, hubUrl, local, onOpened, onRelink }: {
  url: string;
  hubUrl: string;
  local: LocalState | null;
  /** the workspace was opened to the internet: the link is made again */
  onOpened: () => void;
  onRelink: () => void;
}) {
  const [open, setOpen] = useState(false);
  const reach = reachOf(url);
  if (reach === "public") return null;
  // On this computer (by any of its addresses): opened right here.
  const mine = localWorkspaceOf(local, hubUrl);
  const [title, detail] = reach === "local"
    ? [L("只有这台电脑能连上这个工作区", "Only this computer can reach this workspace"),
      L("别的电脑（比如云服务器）运行这条命令会连不上。", "Another computer (a cloud server, say) can't connect with this command.")]
    : reach === "tailnet"
      ? [L("只有同一个 Tailscale 网络里的电脑能连上", "Only computers on the same Tailscale network can connect"),
        L("这个工作区的地址在 Tailscale 网络里，不在这个网络里的电脑（比如云服务器）连不上。", "Its address is on a Tailscale network: computers outside it (a cloud server, say) can't connect.")]
      : [L("只有同一个局域网里的电脑能连上", "Only computers on the same local network can connect"),
        L("云服务器（VPS）运行这条命令会连不上。", "A cloud server (VPS) can't connect with this command.")];
  return (
    <div className="reach-note" data-testid="reach-note" data-reach={reach}>
      <div className="reach-note-text">
        <b><i aria-hidden />{title}</b>
        <span>{detail}{L("", " ")}{mine
          ? L("开放到公网后，哪里的电脑都能连。", "Open it to the internet and computers anywhere can.")
          : L("它在另一台电脑上，要在那台电脑上开放到公网。", "It runs on another computer: it is opened to the internet there.")}</span>
      </div>
      <button type="button" className="app-btn app-btn-small reach-note-btn" onClick={() => setOpen(true)}><Globe />{L("开放到公网", "Open to the internet")}</button>
      {mine
        ? <PublicAccessDialog open={open} onOpenChange={setOpen} local={local} name={mine.name || undefined}
          onDone={() => { onOpened(); onRelink(); }} />
        : <OpenElsewhere open={open} onClose={() => setOpen(false)} hubUrl={hubUrl} onRelink={onRelink} />}
    </div>
  );
}

/** Opening a workspace that runs on another computer to the internet:
 *  there, in its App, or with one command (or by asking an agent on it).
 *  Done, the command here is made again with the new address. */
function OpenElsewhere({ open, onClose, hubUrl, onRelink }: { open: boolean; onClose: () => void; hubUrl: string; onRelink: () => void }) {
  const [tunnel, setTunnel] = useState<"cloudflare" | "tailscale">("cloudflare");
  let port = "";
  try { port = new URL(hubUrl).port; } catch { /* default */ }
  const cmd = `myco hub up --tunnel ${tunnel}`;
  const ask = L(E`帮我把这台电脑上的 myco 工作区开放到公网：运行 ${cmd}${port && port !== "7780" ? `（它在 ${port} 端口）` : ""}，完成后告诉我它给出的公网地址。`,
    E`Open the myco workspace on this computer to the internet: run ${cmd}${port && port !== "7780" ? ` (it is on port ${port})` : ""}, then tell me the public address it prints.`);
  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={L("把工作区开放到公网", "Open the workspace to the internet")} className="reach-dialog"
      description={L("这个工作区跑在另一台电脑上，开放要在那台电脑上做。开放以后，哪里的电脑都能连上它。", "This workspace runs on another computer, so it is opened there. Then computers anywhere can connect.")}>
      <div className="seg seg-fill reach-tunnel" role="tablist" aria-label={L("用什么开放", "How")}>
        <button type="button" role="tab" aria-selected={tunnel === "cloudflare"} className={cn("seg-btn", tunnel === "cloudflare" && "active")} onClick={() => setTunnel("cloudflare")}>Cloudflare</button>
        <button type="button" role="tab" aria-selected={tunnel === "tailscale"} className={cn("seg-btn", tunnel === "tailscale" && "active")} onClick={() => setTunnel("tailscale")}>Tailscale</button>
      </div>
      <ol className="reach-ways">
        <li><b>{L("在那台电脑的 myco App 里", "In the myco App on that computer")}</b>
          <span>{L("设置 › 工作区 › 这个工作区 ›「开放到公网」，选 ", "Settings › Workspaces › this workspace › Open to the internet, pick ")}{tunnel === "cloudflare" ? "Cloudflare" : "Tailscale"}{L("。", ".")}</span></li>
        <li><b>{L("或者在那台电脑的终端里运行", "Or run this in a terminal on that computer")}</b>
          <div className="app-command"><code>{cmd}</code><CopyButton value={cmd} /></div></li>
        <li><b>{L("或者把这句话发给那台电脑上的 agent", "Or send this to an agent on that computer")}</b>
          <div className="app-command is-long"><code>{ask}</code><CopyButton value={ask} /></div></li>
      </ol>
      <p className="app-small-note">{tunnel === "cloudflare"
        ? L("Cloudflare 免费，给一个 trycloudflare.com 的地址；那台电脑要装 cloudflared（myco 会提示怎么装）。", "Cloudflare is free and gives a trycloudflare.com address; that computer needs cloudflared (myco says how to install it).")
        : L("Tailscale 只让你 Tailscale 网络里的电脑连上；两边都要装 Tailscale 并登录同一个账号。", "Tailscale lets only computers on your tailnet connect; both need Tailscale, signed in to the same account.")}</p>
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={onClose}>{L("关闭", "Close")}</button>
        <button type="button" className="app-btn app-btn-primary" onClick={() => { onRelink(); onClose(); }}>{L("开放好了，重新生成命令", "Done: make the command again")}</button>
      </div>
    </Modal>
  );
}

/** Connecting another computer (a VPS, WSL, a second laptop): one command
 *  there installs myco and connects it; once it shows up it is picked, and
 *  its sessions are listed. */
function ConnectComputer({ open, hub, machines, local, onConnected }: {
  open: boolean;
  hub?: Hub;
  machines: Machine[];
  local: LocalState | null;
  onConnected: (machineId: string) => void;
}) {
  const hubId = hub?.id || "";
  const [os, setOs] = useState<"sh" | "ps">("sh");
  const [link, setLink] = useState<JoinLink | null>(null);
  const [error, setError] = useState<unknown>(null);
  const before = useRef<Set<string> | null>(null);
  // From this App over SSH or into WSL, or a command to run there.
  const [way, setWay] = useState<"ssh" | "wsl" | "command">(local ? "ssh" : "command");
  const [wsl, setWsl] = useState(false);
  useEffect(() => {
    if (!open || !local) return;
    appApi.wslDistros().then((r) => setWsl(r.supported && r.distros.length > 0)).catch(() => setWsl(false));
  }, [open, local]);
  const make = () => {
    if (!hubId) return;
    setError(null);
    appApi.createJoinLink(hubId, { uses: 1, expires_hours: 24 }).then(setLink).catch(setError);
  };
  useEffect(() => {
    if (!open) return;
    before.current = new Set(machines.map((m) => m.id));
    setLink(null);
    if (way === "command") make();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, hubId, way]);
  // Connected over SSH or WSL: pick it once the workspace lists it.
  const [pending, setPending] = useState("");
  useEffect(() => {
    if (pending && machines.some((m) => m.id === pending)) {
      setPending("");
      onConnected(pending);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pending, machines]);
  // It shows up: pick it.
  useEffect(() => {
    if (!open || !before.current) return;
    const fresh = machines.find((m) => !before.current!.has(m.id));
    if (fresh) onConnected(fresh.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, machines]);
  const command = link ? (os === "ps" ? link.ps_command || `irm '${link.join_url}?format=ps1' | iex` : link.command) : "";
  const ways = local ? (
    <div className="seg seg-fill add-tabs" role="tablist" aria-label={L("怎么连", "How")}>
      {([["ssh", L("用 SSH 连", "Over SSH")], ...(wsl ? [["wsl", L("这台 Windows 里的 WSL", "WSL on this Windows")]] : []), ["command", L("复制命令", "Copy a command")]] as const).map(([v, label]) => (
        <button key={v} type="button" role="tab" aria-selected={way === v} className={cn("seg-btn", way === v && "active")} onClick={() => setWay(v as typeof way)}>{label}</button>
      ))}
    </div>
  ) : null;
  if (way !== "command" && local) {
    return (
      <div className="add-pane ws-command" data-testid="connect-computer">
        {ways}
        <ConnectOver hubId={hubId} via={way} onConnected={setPending} />
      </div>
    );
  }
  return (
    <div className="add-pane ws-command" data-testid="connect-computer">
      {ways}
      <div className="add-note">{L("在要连的那台电脑（比如你的云服务器）上打开终端，运行下面这一行。它会装好 myco、连上这个工作区；连上以后，那台电脑上的 Claude Code、Codex 会话就会列在这里，点一下就能加入，也能在上面新开。", "On the computer to connect (your cloud server, say), open a terminal and run the line below. It installs myco and connects it to this workspace; then that computer's Claude Code and Codex sessions are listed here to add with a click, and you can start new ones there.")}</div>
      {link && <ReachNote url={link.join_url} hubUrl={hub?.url || ""} local={local} onOpened={() => setLink(null)} onRelink={make} />}
      <div className="seg ws-command-os" role="tablist" aria-label={L("那台电脑的系统", "That computer's system")}>
        <button type="button" role="tab" aria-selected={os === "sh"} className={cn("seg-btn", os === "sh" && "active")} onClick={() => setOs("sh")}>{L("Mac / Linux / WSL", "Mac / Linux / WSL")}</button>
        <button type="button" role="tab" aria-selected={os === "ps"} className={cn("seg-btn", os === "ps" && "active")} onClick={() => setOs("ps")}>{L("Windows（PowerShell）", "Windows (PowerShell)")}</button>
      </div>
      {link ? (
        <div className="app-command is-long"><code>{command}</code><CopyButton value={command} /></div>
      ) : error ? (
        (error as { code?: string }).code === "permission_denied"
          ? <div className="app-small-note">{L("只有工作区的主人或管理员能连电脑：请他们来连。", "Only the workspace's owner or admins can connect a computer: ask one of them.")}</div>
          : <ErrorText error={error} onRetry={make} />
      ) : (
        <div className="app-command"><span className="app-spinner" /><code>{L("正在生成…", "Making it…")}</code></div>
      )}
      {link && <div className="ws-command-wait"><span className="app-spinner" />{L("等它连上…连上后这里会自动切到那台电脑。命令 24 小时内有效，只能用一次。", "Waiting for it… once it connects, this switches to that computer. The command works once, for 24 hours.")}</div>}
      {link && (
        <div className="add-agent-way">
          <div className="app-field-label">{L("或者把这句话发给那台电脑上已经在跑的 agent，它会帮你连：", "Or send this to an agent already running on that computer; it connects it for you:")}</div>
          <div className="add-sentence">
            <p className="add-sentence-text">{connectPrompt(command, hub?.name || "")}</p>
            <div className="add-sentence-actions"><CopyButton value={connectPrompt(command, hub?.name || "")} /></div>
          </div>
          <div className="app-small-note">{L("命令等同凭据，只发给你自己的 agent。", "The command is a credential: send it only to your own agent.")}</div>
        </div>
      )}
    </div>
  );
}

/** What to tell an agent on another computer so it connects that computer. */
function connectPrompt(command: string, workspace: string): string {
  return L(
    E`帮我把这台电脑连进 myco 工作区「${workspace}」：在终端运行 ${command} ，看到连上了再告诉我（最后提示的 myco join 那一步不用做）。这条命令等同凭据，不要发到别处。`,
    E`Connect this computer to the myco workspace "${workspace}": run ${command} in a terminal and tell me once it is connected (skip the myco join step it suggests at the end). The command is a credential: do not post it anywhere else.`,
  );
}

/** A path as that computer writes it: its home directory as ~, / as the
 *  separator. */
function tildeOf(p: string, home?: string): string {
  let d = p.trim().replace(/\\/g, "/").replace(/\/+$/, "");
  if (home) {
    const h = home.replace(/\\/g, "/").replace(/\/+$/, "");
    if (d === h || d.startsWith(h + "/")) d = "~" + d.slice(h.length);
  }
  return d || "/";
}

/** Whether a folder is inside the folders a computer allows (as far as the
 *  App can tell from their paths; the computer checks again). */
export function insideDirs(dir: string, info: MachineInfo): boolean {
  const d = tildeOf(dir, info.home);
  if (!d.startsWith("~") && !info.home) return true; // cannot tell: it decides
  return info.dirs.some((r) => {
    const root = tildeOf(r, info.home);
    return root === "~" ? d === "~" || d.startsWith("~/") : d === root || d.startsWith(root + "/");
  });
}

const ACCESS_RANK: Record<string, number> = { readonly: 1, workspace: 2, full: 3 };

/** Starts a new agent session on a computer: which AI, which folder. What
 *  the computer said about itself shapes it: its AIs, where folders may be,
 *  the access it allows. */
function NewAgentPane({ open, hubId, rows, machine, here, state, onStarted, onState, onReloadInfo }: {
  open: boolean;
  hubId: string;
  rows: AgentRow[];
  machine: Machine;
  /** this computer (its AIs are checked here, with install and sign-in) */
  here: boolean;
  state?: InfoState;
  onStarted: (agent: string) => void;
  /** whether it can start now, and whether it is starting: the dialog's
   *  footer carries the button */
  onState: (s: { ready: boolean; busy: boolean }) => void;
  /** ask the computer again what it allows (after "Allow" here) */
  onReloadInfo?: () => void;
}) {
  const [cli, setCli] = useState<SpawnCli>("claude");
  const [dir, setDir] = useState("");
  const [role, setRole] = useState("");
  const [model, setModel] = useState("");
  const [effort, setEffort] = useState("");
  const [access, setAccess] = useState<RuntimeAccess>("workspace");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  // Where its model comes from (i115, i119): the computer's own, or one of
  // the member's model providers, which goes to that computer with it.
  const [providerId, setProviderId] = useState("");
  const [allFolders, setAllFolders] = useState(false);
  const [allClis, setAllClis] = useState(false);
  useEffect(() => { if (open) setError(null); }, [open, machine.id]);
  // A model or effort picked for one CLI means nothing to another.
  useEffect(() => { setModel(""); setEffort(""); setProviderId(""); }, [cli]);
  // Folders allowed from here (AllowFolder) show at once.
  const [allowed, setAllowed] = useState<string[] | null>(null);
  useEffect(() => setAllowed(null), [machine.id]);
  const info = state?.info && allowed ? { ...state.info, dirs: allowed } : state?.info;
  const hub = rows.find((r) => r.hub.id === hubId)?.hub;
  const aiOf = (c: string) => info?.ais.find((a) => a.cli === c);
  /** the models its CLI offers on that computer (Codex), when it said */
  const offered = aiOf(cli)?.models;
  const ai = aiOf(cli);
  const hiddenClis = NEW_CLIS.filter((c) => !(COMMON_CLIS.includes(c.value) || c.value === cli || (info?.ais.find((x) => x.cli === c.value)?.state ?? "missing") !== "missing")).length;
  const folders = usedFolders(rows, hubId, machine.id).filter((d) => !info || insideDirs(d, info));
  // Newer computers make a folder when none is given (where they say).
  const autoDir = machineMakesFolders(machine.myco_version);
  const base = info?.new_dir || "~/myco";
  const outside = !!info && !!dir.trim() && !insideDirs(dir.trim(), info);
  const limited = !!info && !(info.dirs.length === 1 && tildeOf(info.dirs[0], info.home) === "~");
  const tooHigh = !!info && (ACCESS_RANK[access] || 2) > (ACCESS_RANK[info.max_access] || 2);
  // A computer that can say where folders may be is waited for, so the
  // folder made for an empty field is one it allows.
  // Not for long, though: after a few seconds without an answer it can be
  // started anyway (the computer checks the folder again when it starts).
  const [waitedLong, setWaitedLong] = useState(false);
  const askingNow = askable(machine) && !info && !state?.error;
  useEffect(() => {
    setWaitedLong(false);
    if (!askingNow) return;
    const t = setTimeout(() => setWaitedLong(true), 8000);
    return () => clearTimeout(t);
  }, [askingNow, machine.id]);
  const asking = askingNow && !waitedLong;
  const ready = machine.online && machine.can_spawn && (!!dir.trim() || autoDir) && !outside && !tooHigh && !asking;
  useEffect(() => { onState({ ready, busy: !!busy }); }, [ready, busy, onState]);

  const start = async (e?: React.FormEvent) => {
    e?.preventDefault();
    if (!ready || busy) return;
    setBusy(new Date().toISOString());
    setError(null);
    try {
      const where = dir.trim() || autoFolder(role, cli, base);
      const r = await appApi.spawn(hubId, machine.id, { cli, dir: where, access, ...(role.trim() ? { display_name: role.trim() } : {}),
        ...(providerId ? { provider: providerId, ...(model ? { model } : {}) } : {}) });
      // Model and effort go in its run settings, as the conversation's own
      // controls set them; an older workspace without them just keeps the defaults.
      if (model || effort) await appApi.setRunSettings(hubId, r.agent, { model, effort }).catch(() => undefined);
      setDir("");
      setRole("");
      onStarted(r.agent);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(/above this machine's limit/.test(msg)
        ? new Error(L("那台电脑不允许这么高的权限。在那台电脑的 ~/.myco/machine.yaml 里设置 spawn.max_access 可以放开，或者选低一档。",
          "That computer does not allow this much access. Raise spawn.max_access in ~/.myco/machine.yaml there, or pick a lower level."))
        : err);
    } finally {
      setBusy(null);
    }
  };

  if (!machine.online) {
    return <div className="add-pane"><div className="app-empty-note">{L(E`「${machine.name}」现在不在线，开不了新会话。`, E`“${machine.name}” is away, so it cannot start a session now.`)}</div></div>;
  }
  if (!machine.can_spawn) {
    return <div className="add-pane"><ComputerFix hubId={hubId} machine={machine} here={here} /></div>;
  }
  return (
    <form id="add-new-agent" className="add-pane" onSubmit={start}>
      <div className="app-field">
        <span className="app-field-label">{L("用哪个 AI", "Which AI")}</span>
        <div className="ws-computers">
          {NEW_CLIS.filter((c) => allClis || COMMON_CLIS.includes(c.value) || c.value === cli || (info?.ais.find((a) => a.cli === c.value)?.state ?? "missing") !== "missing").map((c) => {
            const a = here ? undefined : aiOf(c.value);
            return (
              <button key={c.value} type="button" className={cn("ws-computer", cli === c.value && "active", a?.state === "missing" && "is-away")} onClick={() => setCli(c.value)} disabled={!!busy}>
                <span className="ws-computer-name">{c.label}</span>
                {a?.state === "missing" && <span className="ws-computer-sub">{L("没装", "Not installed")}</span>}
                {a?.state === "signed_out" && <span className="ws-computer-sub">{L("没登录", "Signed out")}</span>}
              </button>
            );
          })}
          {!allClis && hiddenClis > 0 && (
            <button type="button" className="ws-computer ws-computer-more" onClick={() => setAllClis(true)} disabled={!!busy} data-testid="new-agent-more-ais">
              {L(E`更多 · ${hiddenClis}`, E`More · ${hiddenClis}`)}<ChevronDown />
            </button>
          )}
        </div>
      </div>
      {here ? <LocalAgentLines only={[cli]} hideReady /> : ai && (ai.state === "missing" || ai.state === "signed_out") && (
        <div className="app-notice" data-testid="remote-ai-missing">
          <Info />
          <div>
            <div>{ai.state === "missing"
              ? L(E`「${machine.name}」上还没装 ${ai.name}。在那台电脑上运行：`, E`${ai.name} is not installed on “${machine.name}”. Run this there:`)
              : L(E`「${machine.name}」上的 ${ai.name} 还没登录。在那台电脑上运行：`, E`${ai.name} is not signed in on “${machine.name}”. Run this there:`)}</div>
            {(ai.state === "missing" ? ai.install : ai.login) && (
              <div className="app-command"><code>{ai.state === "missing" ? ai.install : ai.login}</code><CopyButton value={(ai.state === "missing" ? ai.install : ai.login) || ""} /></div>
            )}
          </div>
        </div>
      )}
      <ModelSource hubId={hubId} cli={cli} machineName={machine.name} value={providerId} onChange={setProviderId} model={model} onModel={setModel} busy={!!busy} />
      {/* (its name first: it also names the folder made when none is given) */}
      <label className="app-field">
        <span className="app-field-label">{L("它的名字（可以不填）", "Its name (optional)")}</span>
        <input className="app-input" value={role} onChange={(e) => setRole(e.target.value)} placeholder={L("比如 后端小助手", "e.g. Backend helper")} disabled={!!busy} />
      </label>
      <label className="app-field">
        <span className="app-field-label">{autoDir ? L("在哪个文件夹干活（可以不填）", "Which folder it works in (optional)") : L("在哪个文件夹干活", "Which folder it works in")}</span>
        <input className="app-input mono" value={dir} onChange={(e) => setDir(e.target.value)} disabled={!!busy} required={!autoDir}
          placeholder={autoDir ? L(E`不填就新建 ${autoFolder(role, cli, base)}`, E`Empty: a new ${autoFolder(role, cli, base)}`) : L("比如 ~/code/我的项目", "e.g. ~/code/my-project")} />
        {askingNow && waitedLong && (
          <span className="app-field-hint" data-testid="asking-slow">{L(E`「${machine.name}」还没说允许在哪些文件夹开工；可以先开：不填文件夹会建在它允许的地方，开的时候它会再查一遍。`, E`“${machine.name}” has not said which folders it allows yet; you can start anyway: with no folder, one is made where it allows, and it checks again when starting.`)}</span>
        )}
        {asking && (
          <span className="app-field-hint" data-testid="asking-dirs">{L(E`正在问「${machine.name}」允许在哪些文件夹开工…`, E`Asking “${machine.name}” which folders it allows…`)}</span>
        )}
        {info && limited && (
          <span className={cn("app-field-hint", outside && "is-bad")} data-testid="allowed-dirs">{outside
            ? <>{L(E`这个文件夹不在「${machine.name}」允许的范围里。允许的：${info.dirs.join("、")}`, E`That folder is outside what “${machine.name}” allows. Allowed: ${info.dirs.join(", ")}`)}
              {hub && <AllowFolder hubId={hubId} hub={hub} machine={machine} dir={dir.trim()} onAllowed={setAllowed} />}</>
            : <span className="add-dirs-line" title={info.dirs.join("\n")}>{L(E`只允许在这里开工：${info.dirs.join("、")}`, E`Only in: ${info.dirs.join(", ")}`)}</span>}</span>
        )}
      </label>
      {/* Folders used before: four, the rest a click away. */}
      {folders.length > 0 && (
        <div className="app-convpick-folders add-recent">
          <span className="add-recent-label">{L("用过的", "Used before")}</span>
          {(allFolders ? folders : folders.slice(0, 4)).map((d) => <button key={d} type="button" className={cn("app-convpick-folder mono", d === dir.trim() && "active")} onClick={() => setDir(d)} disabled={!!busy} title={d}>{shortDir(d)}</button>)}
          {!allFolders && folders.length > 4 && (
            <button type="button" className="app-convpick-folder add-recent-more" onClick={() => setAllFolders(true)} disabled={!!busy}>{L(E`还有 ${folders.length - 4} 个`, E`${folders.length - 4} more`)}</button>
          )}
        </div>
      )}
      <details className="app-advanced">
        <summary>{L("更多选项：模型、思考强度、权限", "More: model, effort, access")}<ChevronDown /></summary>
        <div className="app-advanced-body">
          {!providerId && (MODELS[cli] || offered?.length) && (
            <label className="app-field">
              <span className="app-field-label">{L("模型", "Model")}</span>
              <input className="app-input mono" list={`models-${cli}`} value={model} onChange={(e) => setModel(e.target.value.trim())} disabled={!!busy}
                placeholder={L("不填就跟随电脑设置", "Empty: the computer's setting")} spellCheck={false} data-testid="new-agent-model" />
              <datalist id={`models-${cli}`}>{modelChoices(cli, offered).map((m) => <option key={m} value={m}>{modelName(m, { models: offered })}</option>)}</datalist>
            </label>
          )}
          {(EFFORTS[cli] || offered?.length) && (
            <label className="app-field">
              <span className="app-field-label">{L("思考强度", "Reasoning effort")}</span>
              <select className="app-select-field" value={effort} onChange={(e) => setEffort(e.target.value)} disabled={!!busy}>
                <option value="">{L("跟随电脑设置", "The computer's setting")}</option>
                {effortChoices(cli, offered, model, effort).map((x) => <option key={x} value={x}>{effortLabel(x)}</option>)}
              </select>
            </label>
          )}
          <div className="app-field">
            <span className="app-field-label">{L("权限", "Access")}</span>
            <div className="seg seg-fill" role="tablist" aria-label={L("权限", "Access")}>
              {([["readonly", L("只读", "Read only")], ["workspace", L("这个文件夹", "This folder")], ["full", L("完全放开", "Full")]] as [RuntimeAccess, string][]).map(([a, label]) => (
                <button key={a} type="button" role="tab" aria-selected={access === a} className={cn("seg-btn", access === a && "active")} onClick={() => setAccess(a)} disabled={!!busy}>{label}</button>
              ))}
            </div>
            <span className={cn("app-field-hint", tooHigh && "is-bad")}>{tooHigh
              ? L(E`「${machine.name}」最多允许「${info?.max_access === "readonly" ? "只读" : "这个文件夹"}」。要放开，在那台电脑的 ~/.myco/machine.yaml 里设置 spawn.max_access。`, E`“${machine.name}” allows at most ${info?.max_access}. To allow more, set spawn.max_access in ~/.myco/machine.yaml there.`)
              : access === "full"
                ? L("没有沙箱：能读写那台电脑上的任何文件、跑任何命令。", "No sandbox: it may read and write any file and run any command there.")
                : access === "readonly"
                  ? L("只看不改：能读文件、搜索，不改文件、不跑有副作用的命令。", "Looks, does not change: reads and searches, no edits, no commands with side effects.")
                  : L("能改这个文件夹里的文件；以后可以在对话里改。", "May change files in this folder; can be changed later in the conversation.")}</span>
            {/* This computer: allow it here, and the computer is asked again,
                so Full can be chosen at once (a new session reads the limit
                when it starts). */}
            {tooHigh && access === "full" && hub && (
              <LocalShellAccess machine={machine} hub={hub} what="full" onlyLocal onChange={async () => { onReloadInfo?.(); }} />
            )}
          </div>
        </div>
      </details>
      {/* Claude could not sign in here: the ways this computer has, then again. */}
      {cli === "claude" && here && isClaudeAuthError(error)
        ? <ClaudeAuthFix onRetry={() => { setError(null); void start(); }} />
        : <ErrorText error={error} />}
      {busy && <div className="app-convpick-starting"><Mushroom /><span>{L("正在开", "Starting it")} · <Elapsed since={busy} /></span></div>}
    </form>
  );
}

/** The folder a new conversation gets when none is given: ~/myco/<its
 *  role>, or ~/myco/<cli>-<month><day>-<hour><minute>. Only letters, digits
 *  and a few marks are kept, so it is a plain folder name. */
export function autoFolder(role: string, cli: string, base = "~/myco"): string {
  const root = base.replace(/[\\/]+$/, "");
  const name = role.trim().replace(/[\\/:*?"<>|\s]+/g, "-").replace(/^[.-]+|-+$/g, "").slice(0, 40);
  if (name) return `${root}/${name}`;
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${root}/${cli}-${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}`;
}

/** Folders already used on a computer, one per label as the chips show
 *  them (/Users/me/code/x and ~/code/x are the same folder). */
export function usedFolders(rows: AgentRow[], hubId: string, machineId: string): string[] {
  const seen = new Map<string, string>();
  for (const r of rows) {
    const d = r.st.agent.binding.workdir;
    if (r.hub.id !== hubId || r.st.agent.machine_id !== machineId || !d) continue;
    if (!seen.has(shortDir(d))) seen.set(shortDir(d), d);
  }
  return [...seen.values()].slice(0, 6);
}

/** A folder as a chip shows it: the home directory as ~, and a deep one
 *  as its last two folders (~/…/acme/demo-service); in full on hover. */
export function shortDir(dir: string): string {
  const d = dir.replace(/^(\/Users\/[^/]+|\/home\/[^/]+|\/root|[A-Za-z]:\\Users\\[^\\]+)(?=$|[\\/])/, "~");
  const sep = d.includes("\\") && !d.includes("/") ? "\\" : "/";
  const parts = d.split(sep);
  return parts.length > 4 ? [parts[0], "…", ...parts.slice(-2)].join(sep) : d;
}

/** Where a local session joined: "<workspace> 的 <agent>", each workspace once. */
function joinedWhere(s: LocalSession): string[] {
  const all = s.joined_all?.length ? s.joined_all : s.joined ? [s.joined] : [];
  return all.map((j) => {
    let ws = j.hub_name || "";
    if (!ws) {
      try { const u = new URL(j.hub); ws = u.host + u.pathname.replace(/\/$/, ""); } catch { ws = j.hub; }
    }
    return L(E`${ws} 的 ${j.agent}`, E`${j.agent} in ${ws}`);
  });
}

function joinedLine(s: LocalSession): string {
  const where = joinedWhere(s);
  return where.length ? L(E`已经加入了：${where.join("、")}`, E`Already joined: ${where.join(", ")}`) : L("已经加入了", "Already joined");
}

function folderOf(dir: string): string {
  return dir.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || dir;
}

/**
 * A workspace: a folder on one of your computers where the project's agents
 * can run commands (a terminal opens on it). Pick the computer, pick the
 * folder; every agent in the project may use it.
 */
export function AddWorkspaceDialog({ open, hub, project, rows, workspace, pick, onClose, onAdded }: {
  open: boolean;
  hub?: Hub;
  /** none: a computer of the workspace, not in a project yet */
  project?: Project;
  rows: AgentRow[];
  /** the workspace as last read: its computers, and what they allow (else from rows) */
  workspace?: AgentRow["hub"];
  /** a computer to pick (one just connected) */
  pick?: string;
  onClose: () => void;
  onAdded: () => void;
  /** (older callers: connecting another computer is the command here now) */
  onNewComputer?: () => void;
}) {
  const hubId = hub?.id || "";
  const ov = workspace || rows.find((r) => r.hub.id === hubId)?.hub;
  const machines = useMemo(() => (ov ? machinesOfHub(ov) : []).filter((m) => m.can_spawn), [ov]);
  // What each computer allows, as it reported.
  const shellOf = (m: Machine) => (ov ? machineShell(ov, m.id) : "unknown");
  const [local, setLocal] = useState<LocalState | null>(null);
  const [machineId, setMachineId] = useState("");
  // Until a computer is picked by hand, this computer (once known) is it.
  const [picked, setPicked] = useState(false);
  const [dir, setDir] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  // One command, run in the folder on that computer (as adding an agent is),
  // or a folder on a computer already connected, picked here.
  const [mode, setMode] = useState<"command" | "pick">("command");
  const [os, setOs] = useState<"sh" | "ps">("sh");
  const [link, setLink] = useState<JoinLink | null>(null);
  const [linkError, setLinkError] = useState<unknown>(null);
  // The workspace's computers when the command was made: any other one that
  // shows up is the folder it was run in.
  const before = useRef<Set<string> | null>(null);
  const [arrived, setArrived] = useState<AgentRow | null>(null);
  useEffect(() => {
    if (!open) return;
    setDir("");
    setError(null);
    setBusy(null);
    setPicked(false);
    setMode(pick || (ov ? machinesOfHub(ov) : []).some((m) => m.can_spawn) ? "pick" : "command");
    setLink(null);
    setLinkError(null);
    setArrived(null);
    before.current = null;
    appApi.local().then(setLocal).catch(() => setLocal(null));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  const hereId = ov ? thisComputerId(ov, local) : undefined;
  const isHere = (m: Machine) => m.id === hereId;
  // This computer first, then the others online.
  const ordered = [...machines].sort((a, b) => Number(isHere(b)) - Number(isHere(a)) || Number(b.online) - Number(a.online) || a.name.localeCompare(b.name));
  useEffect(() => {
    if (open && (!picked || !machines.some((m) => m.id === machineId))) setMachineId(ordered[0]?.id || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, machines.length, local]);
  // A computer just connected is the one to use.
  useEffect(() => {
    if (open && pick && machines.some((m) => m.id === pick)) {
      setMachineId(pick);
      setPicked(true);
    }
  }, [open, pick, machines]);
  const machine = machines.find((m) => m.id === machineId);
  const shell = machine ? shellOf(machine) : "unknown";
  // A computer that said no is not tried (it would only be refused).
  // (This computer is allowed right here, in the notice.)
  const blocked = shell === "no" && !!machine && !isHere(machine);
  // The project's agents may use it; without any yet, every agent of yours here.
  const users = (project ? project.members.map((m) => m.name) : [])
    .filter((n) => rows.some((r) => r.hub.id === hubId && r.st.agent.name === n && agentKind(r.st.agent) === "agent"));
  // The command: a one-off link that makes the folder it is run in a
  // computer for the project's agents (none yet: a placeholder, swapped for
  // them when one joins).
  useEffect(() => {
    if (!open || mode !== "command" || link || linkError || !hubId) return;
    let off = false;
    before.current = new Set(rows.filter((r) => r.hub.id === hubId && isWorkspace(r.st.agent)).map((r) => r.st.agent.name));
    appApi.createJoinLink(hubId, { uses: 1, expires_hours: 24, shell_node: true, allow_from: users.length ? users : [nobody()] })
      .then((l) => { if (!off) setLink(l); }, (e) => { if (!off) setLinkError(e); });
    return () => { off = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, mode, hubId, link, linkError]);
  // It shows up: done (and in the project, when it was added from one).
  useEffect(() => {
    if (!open || !link || arrived || !before.current) return;
    const fresh = rows.find((r) => r.hub.id === hubId && isWorkspace(r.st.agent) && !before.current!.has(r.st.agent.name));
    if (!fresh) return;
    setArrived(fresh);
    if (project) void appApi.putProjectMember(hubId, project.name, fresh.st.agent.name).catch(() => undefined);
    onAdded();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, link, arrived, rows]);
  // Folders already used on that computer, one click away.
  const folders = usedFolders(rows, hubId, machineId);
  // The workspace's computers not in this project yet: one click puts one
  // in, and lets the project's agents use it.
  const inProject = new Set((project?.members || []).map((m) => m.name));
  const existing = project ? rows.filter((r) => r.hub.id === hubId && isWorkspace(r.st.agent) && !inProject.has(r.st.agent.name)) : [];
  const [joining, setJoining] = useState<string | null>(null);
  const addExisting = async (r: AgentRow) => {
    if (!project || joining) return;
    setJoining(r.st.agent.name);
    setError(null);
    try {
      await appApi.putProjectMember(hubId, project.name, r.st.agent.name);
      // Who may send it commands: the project's agents too (unless it
      // already takes them from anyone).
      const now = (r.st.agent.allow_from || []).filter((x) => !isNobody(x));
      if (now.length && !now.includes("*")) {
        const next = [...new Set([...now, ...users])];
        if (next.length !== now.length) await appApi.addAllowedSenders(hubId, r.st.agent.name, users.filter((n) => !now.includes(n)));
      }
      onAdded();
      onClose();
    } catch (err) {
      setError(err);
    } finally {
      setJoining(null);
    }
  };

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!machine || !dir.trim() || busy) return;
    setBusy(new Date().toISOString());
    setError(null);
    try {
      const r = await appApi.spawn(hubId, machine.id, { cli: "exec", dir: dir.trim(), allow_from: users.length ? users : [nobody()] });
      if (project) await appApi.putProjectMember(hubId, project.name, r.agent).catch(() => undefined);
      onAdded();
      onClose();
    } catch (err) {
      // Refused for its own limit: say it as the notice above does.
      const msg = err instanceof Error ? err.message : String(err);
      setError(/spawn\.max_access/.test(msg)
        ? new Error(L("那台电脑还没允许开终端：按上面的说明在那台电脑上允许，再点添加。", "That computer hasn't allowed terminals yet: allow it there as said above, then Add again."))
        : err);
    } finally {
      setBusy(null);
    }
  };

  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()}
      title={project ? L("给项目开一个终端", "Add a terminal to the project") : L("开一个终端", "Add a terminal")}
      description={mode === "command"
        ? L("终端是一台电脑上的一个文件夹，agent 在里面跑命令，你点开就能看着。电脑还没连上的话，在它要用的文件夹里运行这一条命令，电脑也会顺便连上。", "A terminal is a folder on a computer: agents run commands in it, and you can open it to watch. For a computer not connected yet, run this one command in the folder to use; it connects the computer too.")
        : project
          ? L("选你的一台电脑和其中一个文件夹，项目里的 agent 就能在里面跑命令。你也可以打开它的终端在旁边看着。", "Pick one of your computers and a folder on it, and the project's agents can run commands there. You can open its terminal and watch.")
          : L("选一台连上的电脑和其中一个文件夹，它就出现在侧栏的「终端」里，点开就能看着。把它加进项目，项目里的 agent 就能在里面跑命令。", "Pick a connected computer and a folder on it: it shows under Terminals in the sidebar, and opens for you to watch. Put it in a project and the project's agents can run commands there.")}
      className="app-modal-add">
      {mode === "command" && (
        <div className="ws-command" data-testid="computer-command">
          <div className="app-field-label">{L("在要用的那台电脑上打开终端，进到要给 agent 用的文件夹，运行这一行：", "On that computer, open a terminal in the folder agents should use, and run:")}</div>
          <div className="seg ws-command-os" role="tablist" aria-label={L("那台电脑的系统", "That computer's system")}>
            <button type="button" role="tab" aria-selected={os === "sh"} className={cn("seg-btn", os === "sh" && "active")} onClick={() => setOs("sh")}>{L("Mac / Linux / WSL", "Mac / Linux / WSL")}</button>
            <button type="button" role="tab" aria-selected={os === "ps"} className={cn("seg-btn", os === "ps" && "active")} onClick={() => setOs("ps")}>{L("Windows（PowerShell）", "Windows (PowerShell)")}</button>
          </div>
          {link ? (
            <div className="app-command">
              <code>{os === "ps" ? link.ps_command || `irm '${link.join_url}?format=ps1' | iex` : link.command}</code>
              <CopyButton value={os === "ps" ? link.ps_command || `irm '${link.join_url}?format=ps1' | iex` : link.command} />
            </div>
          ) : linkError ? (
            // Only the workspace's owner or admins may make one (it lets agents run commands).
            (linkError as { code?: string }).code === "permission_denied"
              ? <div className="app-small-note" data-testid="computer-command-denied">{L("只有工作区的主人或管理员能用命令加电脑：请他们来加。你也可以用已经连上的电脑选文件夹。", "Only the workspace's owner or admins can add a computer with a command: ask one of them. You can also pick a folder on a connected computer.")}</div>
              : <ErrorText error={linkError} onRetry={() => setLinkError(null)} />
          ) : (
            <div className="app-command"><span className="app-spinner" /><code>{L("正在生成…", "Making it…")}</code></div>
          )}
          {arrived ? (
            <div className="ws-command-done" data-testid="computer-arrived">
              <Check />
              <span>{L(E`加好了：${nodeName(arrived.st.agent, arrived.hub)}`, E`Added: ${nodeName(arrived.st.agent, arrived.hub)}`)}
                {arrived.st.agent.binding.workdir && <span className="mono"> · {arrived.st.agent.binding.workdir}</span>}
                {project ? L("。项目里的 agent 现在就能在这里跑命令。", ". The project's agents can run commands there now.") : L("。在侧栏「终端」里，点开就能看着。", ". It is under Terminals in the sidebar; open it to watch.")}</span>
            </div>
          ) : link && (
            <div className="ws-command-wait"><span className="app-spinner" />{L("等它连上…运行完这里会自己打勾。命令 24 小时内有效，只能用一次。", "Waiting for it… this ticks itself once it has run. The command works once, for 24 hours.")}</div>
          )}
          <div className="ws-who">
            {project
              ? users.length > 0
                ? L(E`项目里的 ${users.length} 个 agent 都能在那里跑命令，之后加入的也能。`, E`The project's ${users.length} agent${users.length > 1 ? "s" : ""} can run commands there, and so can those who join later.`)
                : L("项目里还没有 agent。之后加进来的 agent 都能用它。", "No agents in the project yet; the ones you add later can use it.")
              : L("先不给 agent 用。把它加进项目后，项目里的 agent 就能在那里跑命令。", "No agent may use it yet: put it in a project and the project's agents can.")}
            {" "}{L("运行命令就是那台电脑同意了：agent 能用你的身份在这个文件夹里跑命令。", "Running it is that computer's consent: agents can run commands in that folder as you.")}
          </div>
          {link && !arrived && (
            // The same, done by the agent already on that computer.
            <div className="add-agent-way" data-testid="computer-agent-way">
              <div className="app-field-label">{L("或者把这句话发给那台电脑上的 agent，它会帮你装好：", "Or send this to the agent on that computer; it sets it up for you:")}</div>
              <div className="add-sentence">
                <p className="add-sentence-text">{computerPrompt(os === "ps" ? link.ps_command || `irm '${link.join_url}?format=ps1' | iex` : link.command, hub?.name || "")}</p>
                <div className="add-sentence-actions"><CopyButton value={computerPrompt(os === "ps" ? link.ps_command || `irm '${link.join_url}?format=ps1' | iex` : link.command, hub?.name || "")} /></div>
              </div>
              <div className="app-small-note">{L("句子里的命令等同凭据，只发给你自己的 agent。", "The command in it is a credential: send it only to your own agent.")}</div>
            </div>
          )}
          <div className="app-modal-actions">
            {machines.length > 0 && !arrived && (
              <button type="button" className="app-btn app-btn-quiet ws-switch" onClick={() => setMode("pick")}>{L("用已经连上的电脑选文件夹", "Pick a folder on a connected computer")}</button>
            )}
            <button type="button" className={cn("app-btn", arrived && "app-btn-primary")} onClick={onClose}>{arrived ? L("完成", "Done") : L("关闭", "Close")}</button>
          </div>
        </div>
      )}
      {mode === "pick" && existing.length > 0 && (
        <div className="app-field">
          <span className="app-field-label">{L("已有的电脑", "Computers you already have")}</span>
          <div className="ws-existing">
            {existing.map((r) => (
              <div key={r.st.agent.name} className={cn("ws-existing-row", r.st.state === "offline" && "is-away")}>
                <Avatar agent={r.st.agent} size={28} badge={false} />
                <span className="ws-existing-text">
                  <span className="ws-existing-name">{nodeName(r.st.agent, r.hub)}</span>
                  <span className="ws-existing-sub">{whereOf(r)}</span>
                </span>
                <button type="button" className="app-btn app-btn-small" onClick={() => void addExisting(r)} disabled={!!joining || !!busy}>
                  {joining === r.st.agent.name ? L("正在加…", "Adding…") : L("加进来", "Add")}
                </button>
              </div>
            ))}
          </div>
          <div className="ws-or">{L("或者，用一台电脑上的另一个文件夹", "Or use another folder on a computer")}</div>
        </div>
      )}
      {mode === "pick" && <form onSubmit={add}>
        <div className="app-field">
          <span className="app-field-label">{L("在哪台电脑上", "On which computer")}</span>
          {ordered.length === 0 && <div className="app-small-note">{L("这个工作区还没有连上的电脑：点「还没连上的电脑…」。", "No computer is connected to this workspace yet: use A computer not connected yet….")}</div>}
          <div className="ws-computers">
            {ordered.map((m) => {
              const Icon = osInfo(m.os).icon;
              return (
                <button key={m.id} type="button" className={cn("ws-computer", m.id === machineId && "active", !m.online && "is-away")} onClick={() => { setMachineId(m.id); setPicked(true); }} disabled={!!busy}>
                  <Icon />
                  <span className="ws-computer-name">{isHere(m) ? L("这台电脑", "This computer") : m.name}</span>
                  {!m.online ? <span className="ws-computer-sub">{L("不在线", "Away")}</span>
                    : shellOf(m) === "no" && <span className="ws-computer-sub">{L("还没允许", "Not allowed yet")}</span>}
                </button>
              );
            })}
            <button type="button" className="ws-computer is-new" onClick={() => setMode("command")} disabled={!!busy}>
              <span className="ws-computer-name">{L("还没连上的电脑…", "A computer not connected yet…")}</span>
            </button>
          </div>
        </div>
        <label className="app-field">
          <span className="app-field-label">{L("哪个文件夹", "Which folder")}</span>
          <input className="app-input mono" value={dir} onChange={(e) => setDir(e.target.value)} placeholder={L("比如 ~/code/我的项目", "e.g. ~/code/my-project")} disabled={!!busy} required />
        </label>
        {folders.length > 0 && (
          <div className="app-convpick-folders">
            {folders.map((d) => <button key={d} type="button" className={cn("app-convpick-folder mono", d === dir.trim() && "active")} onClick={() => setDir(d)} disabled={!!busy} title={d}>{shortDir(d)}</button>)}
          </div>
        )}
        <div className="ws-who">
          {!project
            ? L("先不给 agent 用。把它加进项目后，项目里的 agent 就能在这里跑命令。", "No agent may use it yet: put it in a project and the project's agents can.")
            : users.length > 0
              ? L(E`项目里的 ${users.length} 个 agent 都能在这里跑命令，之后加入的也能。`, E`The project's ${users.length} agent${users.length > 1 ? "s" : ""} can run commands here, and so can those who join later.`)
              : L("项目里还没有 agent。之后加进来的 agent 都能用它。", "No agents in the project yet; the ones you add later can use it.")}
        </div>
        {machine && <LocalShellAccess machine={machine} hub={ov} what="shell" shell={shell} />}
        <ErrorText error={error} />
        {busy ? (
          <div className="app-convpick-starting"><Mushroom /><span>{L("正在准备", "Setting it up")} · <Elapsed since={busy} /></span></div>
        ) : (
          <div className="app-modal-actions">
            <button type="button" className="app-btn app-btn-quiet ws-switch" onClick={() => setMode("command")}>{L("在还没连上的电脑上开", "On a computer not connected yet")}</button>
            <button type="button" className="app-btn" onClick={onClose}>{L("取消", "Cancel")}</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={!machine || !dir.trim() || blocked}
              title={blocked ? L("那台电脑还没允许开终端", "That computer hasn't allowed terminals yet") : undefined}>{L("添加", "Add")}</button>
          </div>
        )}
      </form>}
    </Modal>
  );
}

/** Starts a project: its name, what it is for (optional) and its files
 *  (optional, uploaded once it exists). */
export function NewProjectDialog({ open, hubs, defaultHubId, onClose, onCreated }: {
  open: boolean;
  hubs: Hub[];
  defaultHubId?: string;
  onClose: () => void;
  /** the project as the hub made it (when known), to show at once */
  onCreated: (ref: { hubId: string; name: string; project?: Project }) => void;
}) {
  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={L("新建项目", "New project")} className="np-modal"
      description={L("一个项目就是一件事，和一起做这件事的 agent。写清楚目标、放上资料，协调者和成员一开始就知道要做什么。", "A project is one thing to get done, and the agents doing it together. A clear goal and its files tell the coordinator and members what to do from the start.")}>
      <NewProjectForm hubs={hubs} defaultHubId={defaultHubId} onCreated={onCreated} onCancel={onClose} />
    </Modal>
  );
}

const MAX_PROJECT_FILE = 20 << 20;

export function NewProjectForm({ hubs, defaultHubId, onCreated, onCancel, big }: {
  hubs: Hub[];
  defaultHubId?: string;
  /** the project as the hub made it (when known), to show at once */
  onCreated: (ref: { hubId: string; name: string; project?: Project }) => void;
  onCancel?: () => void;
  /** the first project, on its own page */
  big?: boolean;
}) {
  const usable = hubs.filter((h) => h.ok !== false);
  const [hubId, setHubId] = useState(defaultHubId && usable.some((h) => h.id === defaultHubId) ? defaultHubId : usable[0]?.id || "");
  const [title, setTitle] = useState(big ? L("我的项目", "My project") : "");
  const [goal, setGoal] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [dragging, setDragging] = useState(false);
  const [busy, setBusy] = useState<string>("");
  const [error, setError] = useState<unknown>(null);
  // Made, but some files did not go up: in anyway, or try them again.
  const [made, setMade] = useState<{ hubId: string; name: string; failed: File[]; project?: Project } | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const tooBig = files.filter((f) => f.size > MAX_PROJECT_FILE);
  const addFiles = (fs: File[]) => setFiles((have) => [...have, ...fs.filter((f) => !have.some((h) => h.name === f.name && h.size === f.size))]);
  const upload = async (target: { hubId: string; name: string }, list: File[]) => {
    const failed: File[] = [];
    for (let i = 0; i < list.length; i++) {
      setBusy(L(E`正在上传资料 ${i + 1}/${list.length}…`, E`Uploading files ${i + 1}/${list.length}…`));
      try { await appApi.uploadProjectFile(target.hubId, target.name, list[i]); } catch (err) { failed.push(list[i]); setError(err); }
    }
    return failed;
  };
  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!hubId || !title.trim() || busy || tooBig.length) return;
    setBusy(L("正在创建…", "Creating…"));
    setError(null);
    try {
      const slug = title.toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 32);
      const name = `${slug || "p"}-${Math.random().toString(36).slice(2, 6)}`;
      const p = await appApi.createProject(hubId, { name, title: title.trim(), ...(goal.trim() ? { goal: goal.trim() } : {}) });
      const failed = files.length ? await upload({ hubId, name: p.name }, files) : [];
      if (failed.length) { setMade({ hubId, name: p.name, failed, project: p }); return; }
      onCreated({ hubId, name: p.name, project: p });
    } catch (err) {
      setError(err);
    } finally {
      setBusy("");
    }
  };
  const retry = async () => {
    if (!made) return;
    setError(null);
    const failed = await upload(made, made.failed);
    setBusy("");
    if (failed.length) setMade({ ...made, failed });
    else onCreated({ hubId: made.hubId, name: made.name, project: made.project });
  };
  if (made) {
    return (
      <div className="np np-made" data-testid="new-project-partial">
        <p className="np-made-text">{L(E`项目建好了，但有 ${made.failed.length} 个资料没传上去：`, E`The project is made, but ${made.failed.length} file(s) did not go up:`)}</p>
        <ul className="np-files">{made.failed.map((f) => <li key={f.name} className="np-file"><FileText /><span>{f.name}</span><small>{bytes(f.size)}</small></li>)}</ul>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => onCreated({ hubId: made.hubId, name: made.name, project: made.project })} disabled={!!busy}>{L("先进去，稍后再传", "Go in; upload later")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void retry()} disabled={!!busy}>{busy || L("再传一次", "Try again")}</button>
        </div>
      </div>
    );
  }
  return (
    <form onSubmit={create} className={cn("np", big && "is-big")} data-testid="new-project-form">
      <label className="app-field">
        <span className="app-field-label">{L("项目名", "Name")}</span>
        <input className="app-input np-input" value={title} onChange={(e) => setTitle(e.target.value)} autoFocus maxLength={80}
          placeholder={L("比如：官网改版", "e.g. Website redesign")} aria-label={L("项目名", "Project name")} />
      </label>
      <label className="app-field">
        <span className="app-field-label">{L("目标", "Goal")}<small className="np-optional">{L("选填", "optional")}</small></span>
        <textarea className="app-input np-goal" rows={3} value={goal} onChange={(e) => setGoal(e.target.value)} maxLength={2000}
          placeholder={L("想做成什么、什么时候、做到什么程度算完成。比如：11 月底前上线新版官网：首页、文档站和下载页，中英双语。", "What should come out of it, by when, and what counts as done. E.g. ship the new website by the end of November: home, docs and downloads, in Chinese and English.")}
          aria-label={L("目标", "Goal")} />
        <span className="app-small-note">{L("协调者和成员都会看到；以后在「项目资料」里随时能改。", "The coordinator and members all see it; change it any time in Project notes.")}</span>
      </label>
      <div className="app-field">
        <span className="app-field-label">{L("项目资料", "Files")}<small className="np-optional">{L("选填", "optional")}</small></span>
        <div className={cn("np-drop", dragging && "is-over")} role="button" tabIndex={0}
          onClick={() => fileInput.current?.click()} onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); fileInput.current?.click(); } }}
          onDragOver={(e) => { e.preventDefault(); setDragging(true); }} onDragLeave={() => setDragging(false)}
          onDrop={(e) => { e.preventDefault(); setDragging(false); addFiles(Array.from(e.dataTransfer.files)); }}
          data-testid="new-project-drop">
          <Upload />
          <span><b>{L("把文件拖到这里，或点一下选择", "Drop files here, or click to choose")}</b>
            <small>{L("需求文档、设计稿、约定、截图都可以，每个 20 MB 以内；成员 agent 能读到。", "Specs, designs, conventions, screenshots: up to 20 MB each; member agents can read them.")}</small></span>
        </div>
        <input ref={fileInput} type="file" multiple hidden onChange={(e) => { addFiles(Array.from(e.target.files || [])); e.target.value = ""; }} />
        {files.length > 0 && (
          <ul className="np-files" data-testid="new-project-files">
            {files.map((f) => (
              <li key={`${f.name}/${f.size}`} className={cn("np-file", f.size > MAX_PROJECT_FILE && "is-bad")}>
                <FileText />
                <span>{f.name}</span>
                <small>{f.size > MAX_PROJECT_FILE ? L("超过 20 MB", "Over 20 MB") : bytes(f.size)}</small>
                <button type="button" className="np-file-x" onClick={() => setFiles((fs) => fs.filter((x) => x !== f))} aria-label={L(E`去掉 ${f.name}`, E`Remove ${f.name}`)} disabled={!!busy}><X /></button>
              </li>
            ))}
          </ul>
        )}
      </div>
      {usable.length > 1 && (
        <label className="app-field">
          <span className="app-field-label">{L("放在哪个工作区", "In which workspace")}</span>
          <select className="app-select-field" value={hubId} onChange={(e) => setHubId(e.target.value)}>
            {usable.map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
          </select>
        </label>
      )}
      <ErrorText error={error} />
      <div className="app-modal-actions">
        {tooBig.length > 0 && <span className="app-modal-hint">{L("有文件超过 20 MB，去掉它再建", "A file is over 20 MB: remove it first")}</span>}
        {onCancel && <button type="button" className="app-btn" onClick={onCancel} disabled={!!busy}>{big ? L("先不建", "Not now") : L("取消", "Cancel")}</button>}
        <button type="submit" className="app-btn app-btn-primary" disabled={!title.trim() || !!busy || tooBig.length > 0}>
          {busy || (files.length ? L(E`建好，并上传 ${files.length} 个资料`, E`Create, with ${files.length} file(s)`) : L("建好了", "Create"))}
        </button>
      </div>
    </form>
  );
}
