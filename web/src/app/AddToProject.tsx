import { useEffect, useMemo, useRef, useState } from "react";
import { Check } from "lucide-react";
import { appApi, type Hub, type JoinLink, type LocalSession, type LocalState, type SpawnCli } from "@/appapi";
import { IconAsterisk, IconCode } from "@/components/icons";
import type { Machine, Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, machinesOfHub, nodeName, osInfo } from "@/app/nodes";
import { Avatar, isWorkspace, whereOf } from "@/app/persona";
import { CopyButton } from "@/app/copy";
import { LocalShellAccess } from "@/app/LocalShellAccess";
import { Elapsed, Mushroom } from "@/app/Working";
import { ErrorText, Modal } from "@/app/dialogs";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

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
    return appApi.updateAllowFrom(hubId, r.st.agent.name, [...now, agent]).catch(() => null);
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
function joinSentence(link: JoinLink, project?: string): string {
  if (link.agent_guide) {
    const command = project ? `curl -fsSL ${link.join_url}/${project} | bash` : link.command;
    return L(`加入 myco：在终端运行 ${command} ，再照它最后的输出做完。`, `Join myco: run ${command} in a terminal, then do what its last lines say.`);
  }
  const into = project ? ` --project ${project}` : "";
  return L(
    `加入 myco：在终端运行 ${link.command} ，然后在你这个会话里运行 myco join${into} --display-name "你的分工" --description "一句话说你负责什么"（把引号里的换成你自己的：分工用 2 到 6 个字，比如 后端工程师、审查员、运维）。之后照 myco join 输出里 receive 的说明接收消息：会话开着时，在后台一直运行 myco inbox wait，收到消息用 myco inbox pull 读取并处理。`,
    `Join myco: run ${link.command} in a terminal, then in this session run myco join${into} --display-name "your role" --description "one line on what you do" (fill in your own: a short role such as Backend engineer, Reviewer or Ops). Then receive messages as its "receive" output says: while the session is open keep myco inbox wait running in the background, and read what it reports with myco inbox pull.`,
  );
}

type AddTab = "members" | "local" | "link" | "new";

const NEW_CLIS: { value: SpawnCli; label: string }[] = [
  { value: "claude", label: "Claude Code" },
  { value: "codex", label: "Codex" },
  { value: "gemini", label: "Gemini CLI" },
  { value: "cursor", label: "Cursor" },
  { value: "opencode", label: "OpenCode" },
  { value: "kiro", label: "Kiro" },
  { value: "pi", label: "Pi" },
];

/**
 * Adding agents, four ways (tabs): a member already in this workspace (in
 * other projects, or in none), a session already on this computer,
 * one sentence for any AI session anywhere, or a new one started on one of
 * your computers. Whoever joins while this is open shows up at the bottom,
 * and goes into the project.
 */
export function AddAgentDialog({ open, hub, project, projects = [], rows, onClose, onAdded }: {
  open: boolean;
  hub?: Hub;
  project?: Project;
  /** the workspace's projects: where the members already are */
  projects?: Project[];
  rows: AgentRow[];
  onClose: () => void;
  onAdded: () => void;
}) {
  const hubId = hub?.id || "";
  const [tab, setTab] = useState<AddTab>("local");
  const touched = useRef(false);
  const [link, setLink] = useState<JoinLink | null>(null);
  const [linkError, setLinkError] = useState<unknown>(null);
  const [joined, setJoined] = useState<string[]>([]);
  const [sessions, setSessions] = useState<LocalSession[] | null>(null);
  const [sessionsError, setSessionsError] = useState<unknown>(null);
  const [query, setQuery] = useState("");
  const [joining, setJoining] = useState("");
  const [joinError, setJoinError] = useState<unknown>(null);
  const [newState, setNewState] = useState({ ready: false, busy: false });
  // Agents of this workspace not in the project yet (in others, or none):
  // online first.
  function others(): AgentRow[] {
    const inIt = new Set((project?.members || []).map((m) => m.name));
    return rows.filter((r) => r.hub.id === hubId && agentKind(r.st.agent) === "agent" && !inIt.has(r.st.agent.name))
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
    setTab(project && others().length > 0 ? "members" : "local");
    setJoined([]);
    setLink(null);
    setLinkError(null);
    setJoinError(null);
    setSessions(null);
    setSessionsError(null);
    setQuery("");
    if (!hubId) return;
    appApi.createJoinLink(hubId, { uses: 10, expires_hours: 168 }).then(setLink).catch(setLinkError);
    appApi.localSessions().then((r) => {
      setSessions(r.sessions);
      // Nothing on this computer to add: the sentence is the way.
      if (!touched.current && !r.sessions.some((s) => !s.joined)) setTab((t) => (t === "local" ? "link" : t));
    }).catch((e) => {
      setSessionsError(e);
      if (!touched.current) setTab((t) => (t === "local" ? "link" : t));
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, hubId]);

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
      const r = await appApi.localJoin(hubId, { cli: s.cli, session_id: s.session_id, workdir: s.workdir, fork: true });
      await arrived(r.agent);
    } catch (e) {
      setJoinError(e);
    } finally {
      setJoining("");
    }
  };

  const pick = (t: AddTab) => { touched.current = true; setTab(t); };
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
  const where = project ? L(`「${project.title || project.name}」`, `“${project.title || project.name}”`) : L("这个工作区", "this workspace");
  const q = query.trim().toLowerCase();
  const list = (sessions || []).filter((s) => !q || [s.title, s.workdir, s.cli].some((x) => (x || "").toLowerCase().includes(q)));
  const free = list.filter((s) => !s.joined);
  const taken = list.filter((s) => s.joined);
  const sentence = link ? joinSentence(link, project?.name) : "";

  return (
    <Modal open={open} onOpenChange={(o) => !o && !joining && onClose()} title={L("添加 agent", "Add an agent")}
      description={L(`给${where}加一个 AI 同事。`, `Bring an AI teammate into ${where}.`)}
      className="app-modal-add">
      <div className="seg seg-fill add-tabs" role="tablist" aria-label={L("怎么加", "How")}>
        {([
          ...(project ? [["members", L("已有的", "Already here")] as const] : []),
          ["local", L("这台电脑上的", "On this computer")],
          ["link", L("发一句话", "Send a sentence")],
          ["new", L("新开一个", "Start a new one")],
        ] as const).map(([v, label]) => (
          <button key={v} type="button" role="tab" aria-selected={tab === v} className={cn("seg-btn", tab === v && "active")} onClick={() => pick(v)}>{label}</button>
        ))}
      </div>

      {tab === "members" && (
        <div className="add-pane">
          {others().length === 0 && joined.length === 0 ? (
            <div className="app-empty-note">{L("这个工作区里的 agent 都在这个项目里了。", "Every agent in this workspace is in this project already.")}</div>
          ) : (
            <div className="add-sessions">
              {others().map((r) => {
                const a = r.st.agent;
                const where = projects.filter((p) => !p.archived && p.members.some((m) => m.name === a.name)).map((p) => p.title || p.name);
                return (
                  <div key={a.name} className={cn("add-local-row", r.st.state === "offline" && "is-away")}>
                    <Avatar agent={a} size={30} state={r.st.state} />
                    <span className="add-local-main">
                      <span className="add-local-name">{nodeName(a, r.hub)}</span>
                      <span className="add-local-sub">{where.length ? L(`在「${where.join("」「")}」`, `In ${where.join(", ")}`) : L("还没进项目", "Not in a project")}{a.description ? ` · ${a.description}` : ""}</span>
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

      {tab === "local" && (
        <div className="add-pane">
          {sessions === null && !sessionsError ? (
            <div className="add-waiting"><Mushroom /><span>{L("正在找这台电脑上的会话…", "Looking for sessions on this computer…")}</span></div>
          ) : sessionsError ? (
            <ErrorText error={sessionsError} />
          ) : (sessions || []).length === 0 ? (
            <div className="app-empty-note">{L("这台电脑上最近没有 Claude Code 或 Codex 会话。可以「发一句话」给别处的 agent，或者「新开一个」。", "No recent Claude Code or Codex sessions here. Send a sentence to an agent elsewhere, or start a new one.")}</div>
          ) : (
            <>
              {(sessions || []).length > 6 && (
                <input className="app-input add-search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={L("找一个会话：标题、文件夹", "Find a session: title, folder")} aria-label={L("搜索会话", "Search sessions")} />
              )}
              <div className="add-sessions">
                {free.map((s) => {
                  const Icon = s.cli === "codex" ? IconCode : IconAsterisk;
                  return (
                    <div key={`${s.cli}:${s.session_id}`} className="add-local-row">
                      <span className="add-local-icon"><Icon /></span>
                      <span className="add-local-main">
                        <span className="add-local-name">{s.title || folderOf(s.workdir)}</span>
                        <span className="add-local-sub" title={s.workdir}>{s.cli === "codex" ? "Codex" : "Claude Code"} · {folderOf(s.workdir)} · {timeAgo(s.last_active)}</span>
                      </span>
                      <button type="button" className="app-btn app-btn-small" disabled={!!joining} onClick={() => void joinLocal(s)}>
                        {joining === s.session_id ? L("正在加入…", "Joining…") : L("加入", "Add")}
                      </button>
                    </div>
                  );
                })}
                {taken.map((s) => {
                  const Icon = s.cli === "codex" ? IconCode : IconAsterisk;
                  return (
                    <div key={`${s.cli}:${s.session_id}`} className="add-local-row is-taken">
                      <span className="add-local-icon"><Icon /></span>
                      <span className="add-local-main">
                        <span className="add-local-name">{s.title || folderOf(s.workdir)}</span>
                        <span className="add-local-sub">{L("已经加入了", "Already joined")}</span>
                      </span>
                    </div>
                  );
                })}
                {list.length === 0 && <div className="app-empty-note">{L("没找到。", "Nothing matches.")}</div>}
              </div>
              <div className="add-note">{L("会复制一份会话加入，你原来的会话照常用。", "A copy of the session joins; yours carries on as before.")}</div>
              <ErrorText error={joinError} />
            </>
          )}
        </div>
      )}

      {tab === "link" && (
        <div className="add-pane">
          <div className="add-note">{L("把这句话发给你的 agent，它会自己装好 myco 加入。Claude Code、Codex、claude.ai 或任何会跑命令的 AI 都行，可以发给好几个。", "Send this to your agent; it installs myco and joins by itself. Claude Code, Codex, claude.ai or any AI that runs commands; send it to as many as you like.")}</div>
          <div className="add-sentence">
            {link ? (
              <>
                <p className="add-sentence-text">{sentence}</p>
                <div className="add-sentence-actions"><CopyButton value={sentence} /></div>
              </>
            ) : linkError ? <ErrorText error={linkError} /> : <p className="add-sentence-text is-loading">{L("正在准备…", "Getting it ready…")}</p>}
          </div>
          {joined.length === 0 && (
            <div className="add-waiting"><Mushroom idle /><span>{L("发出去以后，加入的 agent 会出现在下面。", "Once sent, the agents that join show up below.")}</span></div>
          )}
        </div>
      )}

      {tab === "new" && <NewAgentPane open={open && tab === "new"} hubId={hubId} rows={rows} onStarted={(a) => void arrived(a)} onState={setNewState} />}

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

      {/* One row of buttons: starting a new one is the main step on its tab. */}
      <div className="app-modal-actions">
        {tab === "new" ? (
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

/** Starts a new agent session on one of your computers: which computer,
 *  which AI, which folder. */
function NewAgentPane({ open, hubId, rows, onStarted, onState }: {
  open: boolean;
  hubId: string;
  rows: AgentRow[];
  onStarted: (agent: string) => void;
  /** whether it can start now, and whether it is starting: the dialog's
   *  footer carries the button */
  onState: (s: { ready: boolean; busy: boolean }) => void;
}) {
  const ov = rows.find((r) => r.hub.id === hubId)?.hub;
  const machines = useMemo(() => (ov ? machinesOfHub(ov) : []).filter((m) => m.can_spawn), [ov]);
  const [local, setLocal] = useState<LocalState | null>(null);
  const [machineId, setMachineId] = useState("");
  const [picked, setPicked] = useState(false);
  const [cli, setCli] = useState<SpawnCli>("claude");
  const [dir, setDir] = useState("");
  const [role, setRole] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (!open) return;
    appApi.local().then(setLocal).catch(() => setLocal(null));
  }, [open]);
  const isHere = (m: Machine) => !!local && ((local.machines || []).includes(m.name) || (!!local.hostname && m.hostname === local.hostname));
  const ordered = [...machines].sort((a, b) => Number(isHere(b)) - Number(isHere(a)) || Number(b.online) - Number(a.online) || a.name.localeCompare(b.name));
  useEffect(() => {
    if (open && (!picked || !machines.some((m) => m.id === machineId))) setMachineId(ordered[0]?.id || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, machines.length, local]);
  const machine = machines.find((m) => m.id === machineId);
  const folders = usedFolders(rows, hubId, machineId);
  const ready = !!machine && !!dir.trim();
  useEffect(() => { onState({ ready, busy: !!busy }); }, [ready, busy, onState]);

  const start = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!machine || !dir.trim() || busy) return;
    setBusy(new Date().toISOString());
    setError(null);
    try {
      const r = await appApi.spawn(hubId, machine.id, { cli, dir: dir.trim(), access: "workspace", ...(role.trim() ? { display_name: role.trim() } : {}) });
      setDir("");
      setRole("");
      onStarted(r.agent);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(null);
    }
  };

  if (machines.length === 0) {
    return <div className="add-pane"><div className="app-empty-note">{L("还没有能新开会话的电脑。先在「电脑」里连上一台。", "No computer can start sessions yet. Connect one under Computers first.")}</div></div>;
  }
  return (
    <form id="add-new-agent" className="add-pane" onSubmit={start}>
      <div className="app-field">
        <span className="app-field-label">{L("在哪台电脑上", "On which computer")}</span>
        <div className="ws-computers">
          {ordered.map((m) => {
            const Icon = osInfo(m.os).icon;
            return (
              <button key={m.id} type="button" className={cn("ws-computer", m.id === machineId && "active", !m.online && "is-away")} onClick={() => { setMachineId(m.id); setPicked(true); }} disabled={!!busy}>
                <Icon />
                <span className="ws-computer-name">{isHere(m) ? L("这台电脑", "This computer") : m.name}</span>
                {!m.online && <span className="ws-computer-sub">{L("不在线", "Away")}</span>}
              </button>
            );
          })}
        </div>
      </div>
      <div className="app-field">
        <span className="app-field-label">{L("用哪个 AI", "Which AI")}</span>
        <div className="ws-computers">
          {NEW_CLIS.map((c) => (
            <button key={c.value} type="button" className={cn("ws-computer", cli === c.value && "active")} onClick={() => setCli(c.value)} disabled={!!busy}>
              <span className="ws-computer-name">{c.label}</span>
            </button>
          ))}
        </div>
      </div>
      <label className="app-field">
        <span className="app-field-label">{L("在哪个文件夹干活", "Which folder it works in")}</span>
        <input className="app-input mono" value={dir} onChange={(e) => setDir(e.target.value)} placeholder={L("比如 ~/code/我的项目", "e.g. ~/code/my-project")} disabled={!!busy} required />
      </label>
      {folders.length > 0 && (
        <div className="app-convpick-folders">
          {folders.map((d) => <button key={d} type="button" className={cn("app-convpick-folder mono", d === dir.trim() && "active")} onClick={() => setDir(d)} disabled={!!busy} title={d}>{shortDir(d)}</button>)}
        </div>
      )}
      <label className="app-field">
        <span className="app-field-label">{L("它的分工（可以不填）", "Its role (optional)")}</span>
        <input className="app-input" value={role} onChange={(e) => setRole(e.target.value)} placeholder={L("比如 后端工程师", "e.g. Backend engineer")} disabled={!!busy} />
      </label>
      <ErrorText error={error} />
      {busy && <div className="app-convpick-starting"><Mushroom /><span>{L("正在开", "Starting it")} · <Elapsed since={busy} /></span></div>}
    </form>
  );
}

/** Folders already used on a computer, one per label as the chips show
 *  them (/Users/me/code/x and ~/code/x are the same folder). */
function usedFolders(rows: AgentRow[], hubId: string, machineId: string): string[] {
  const seen = new Map<string, string>();
  for (const r of rows) {
    const d = r.st.agent.binding.workdir;
    if (r.hub.id !== hubId || r.st.agent.machine_id !== machineId || !d) continue;
    if (!seen.has(shortDir(d))) seen.set(shortDir(d), d);
  }
  return [...seen.values()].slice(0, 6);
}

/** A folder as a chip shows it: the home directory as ~, and a deep one
 *  as its last two folders (~/…/tapnow/loops-harness); in full on hover. */
export function shortDir(dir: string): string {
  const d = dir.replace(/^(\/Users\/[^/]+|\/home\/[^/]+|\/root|[A-Za-z]:\\Users\\[^\\]+)(?=$|[\\/])/, "~");
  const sep = d.includes("\\") && !d.includes("/") ? "\\" : "/";
  const parts = d.split(sep);
  return parts.length > 4 ? [parts[0], "…", ...parts.slice(-2)].join(sep) : d;
}

function folderOf(dir: string): string {
  return dir.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || dir;
}

/**
 * A workspace: a folder on one of your computers where the project's agents
 * can run commands (a terminal opens on it). Pick the computer, pick the
 * folder; every agent in the project may use it.
 */
export function AddWorkspaceDialog({ open, hub, project, rows, onClose, onAdded, onNewComputer }: {
  open: boolean;
  hub?: Hub;
  project?: Project;
  rows: AgentRow[];
  onClose: () => void;
  onAdded: () => void;
  /** connect another computer first */
  onNewComputer: () => void;
}) {
  const hubId = hub?.id || "";
  const ov = rows.find((r) => r.hub.id === hubId)?.hub;
  const machines = useMemo(() => (ov ? machinesOfHub(ov) : []).filter((m) => m.can_spawn), [ov]);
  const [local, setLocal] = useState<LocalState | null>(null);
  const [machineId, setMachineId] = useState("");
  // Until a computer is picked by hand, this computer (once known) is it.
  const [picked, setPicked] = useState(false);
  const [dir, setDir] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (!open) return;
    setDir("");
    setError(null);
    setBusy(null);
    setPicked(false);
    appApi.local().then(setLocal).catch(() => setLocal(null));
  }, [open]);
  const isHere = (m: Machine) => !!local && ((local.machines || []).includes(m.name) || (!!local.hostname && m.hostname === local.hostname));
  // This computer first, then the others online.
  const ordered = [...machines].sort((a, b) => Number(isHere(b)) - Number(isHere(a)) || Number(b.online) - Number(a.online) || a.name.localeCompare(b.name));
  useEffect(() => {
    if (open && (!picked || !machines.some((m) => m.id === machineId))) setMachineId(ordered[0]?.id || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, machines.length, local]);
  const machine = machines.find((m) => m.id === machineId);
  // The project's agents may use it; without any yet, every agent of yours here.
  const users = (project ? project.members.map((m) => m.name) : [])
    .filter((n) => rows.some((r) => r.hub.id === hubId && r.st.agent.name === n && agentKind(r.st.agent) === "agent"));
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
        if (next.length !== now.length) await appApi.updateAllowFrom(hubId, r.st.agent.name, next).catch(() => undefined);
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
      setError(err);
    } finally {
      setBusy(null);
    }
  };

  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={L("给项目加一台电脑", "Add a computer to the project")}
      description={L("选你的一台电脑和其中一个文件夹，项目里的 agent 就能在里面跑命令。你也可以打开它的终端在旁边看着。", "Pick one of your computers and a folder on it, and the project's agents can run commands there. You can open its terminal and watch.")}
      className="app-modal-add">
      {existing.length > 0 && (
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
      <form onSubmit={add}>
        <div className="app-field">
          <span className="app-field-label">{L("在哪台电脑上", "On which computer")}</span>
          <div className="ws-computers">
            {ordered.map((m) => {
              const Icon = osInfo(m.os).icon;
              return (
                <button key={m.id} type="button" className={cn("ws-computer", m.id === machineId && "active", !m.online && "is-away")} onClick={() => { setMachineId(m.id); setPicked(true); }} disabled={!!busy}>
                  <Icon />
                  <span className="ws-computer-name">{isHere(m) ? L("这台电脑", "This computer") : m.name}</span>
                  {!m.online && <span className="ws-computer-sub">{L("不在线", "Away")}</span>}
                </button>
              );
            })}
            <button type="button" className="ws-computer is-new" onClick={onNewComputer} disabled={!!busy}>
              <span className="ws-computer-name">{L("另一台电脑…", "Another computer…")}</span>
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
          {users.length > 0
            ? L(`项目里的 ${users.length} 个 agent 都能在这里跑命令，之后加入的也能。`, `The project's ${users.length} agent${users.length > 1 ? "s" : ""} can run commands here, and so can those who join later.`)
            : L("项目里还没有 agent。之后加进来的 agent 都能用它。", "No agents in the project yet; the ones you add later can use it.")}
        </div>
        {machine && <LocalShellAccess machine={machine} what="shell" />}
        <ErrorText error={error} />
        {busy ? (
          <div className="app-convpick-starting"><Mushroom /><span>{L("正在准备", "Setting it up")} · <Elapsed since={busy} /></span></div>
        ) : (
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={onClose}>{L("取消", "Cancel")}</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={!machine || !dir.trim()}>{L("添加", "Add")}</button>
          </div>
        )}
      </form>
    </Modal>
  );
}

/** Starts a project: just a name. */
export function NewProjectDialog({ open, hubs, defaultHubId, onClose, onCreated }: {
  open: boolean;
  hubs: Hub[];
  defaultHubId?: string;
  onClose: () => void;
  onCreated: (ref: { hubId: string; name: string }) => void;
}) {
  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={L("新建项目", "New project")}
      description={L("一个项目就是一件事，和一起做这件事的 agent。", "A project is one thing to get done, and the agents doing it together.")}>
      <NewProjectForm hubs={hubs} defaultHubId={defaultHubId} onCreated={onCreated} onCancel={onClose} />
    </Modal>
  );
}

export function NewProjectForm({ hubs, defaultHubId, onCreated, onCancel, big }: {
  hubs: Hub[];
  defaultHubId?: string;
  onCreated: (ref: { hubId: string; name: string }) => void;
  onCancel?: () => void;
  /** the first project, on its own page */
  big?: boolean;
}) {
  const usable = hubs.filter((h) => h.ok !== false);
  const [hubId, setHubId] = useState(defaultHubId && usable.some((h) => h.id === defaultHubId) ? defaultHubId : usable[0]?.id || "");
  const [title, setTitle] = useState(big ? L("我的项目", "My project") : "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!hubId || !title.trim() || busy) return;
    setBusy(true);
    setError(null);
    try {
      const slug = title.toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 32);
      const name = `${slug || "p"}-${Math.random().toString(36).slice(2, 6)}`;
      const p = await appApi.createProject(hubId, { name, title: title.trim() });
      onCreated({ hubId, name: p.name });
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form onSubmit={create} className={cn("np", big && "is-big")}>
      <input className="app-input np-input" value={title} onChange={(e) => setTitle(e.target.value)} autoFocus
        placeholder={L("项目叫什么？比如 官网改版", "What's it called? e.g. Website redesign")} aria-label={L("项目名", "Project name")} />
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
        {onCancel && <button type="button" className="app-btn" onClick={onCancel}>{big ? L("先不建", "Not now") : L("取消", "Cancel")}</button>}
        <button type="submit" className="app-btn app-btn-primary" disabled={!title.trim() || busy}>{busy ? L("正在创建…", "Creating…") : L("建好了", "Create")}</button>
      </div>
    </form>
  );
}
