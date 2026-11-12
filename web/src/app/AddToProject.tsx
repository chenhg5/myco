import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Monitor } from "lucide-react";
import { appApi, type Hub, type JoinLink, type LocalSession, type LocalState } from "@/appapi";
import type { Machine, Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, machinesOfHub, nodeName, osInfo } from "@/app/nodes";
import { Avatar, isWorkspace } from "@/app/persona";
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
    const now = (r.st.agent.allow_from || []).filter((x) => x !== "*");
    if (now.includes(agent)) return null;
    return appApi.updateAllowFrom(hubId, r.st.agent.name, [...now, agent]).catch(() => null);
  }));
}

/** Puts an agent in a project, and lets it use the project's workspaces. */
async function addToProject(hubId: string, project: Project | undefined, rows: AgentRow[], agent: string) {
  if (!project) return;
  const p = await appApi.putProjectMember(hubId, project.name, agent);
  await shareWorkspaces(hubId, p, rows, agent);
}

// The one sentence an agent needs: get myco onto its computer, join this
// network from inside its own session, and say who it is.
function joinSentence(command: string, project?: string): string {
  // The project travels with the sentence: myco join --project puts the
  // agent in it as soon as it has joined (it can see its owner's projects).
  const into = project ? ` --project ${project}` : "";
  return L(
    `加入 myco：在终端运行 ${command} ，然后在你这个会话里运行 myco join${into} --display-name "你的分工" --description "一句话说你负责什么"（把引号里的换成你自己的：分工用 2 到 6 个字，比如 后端工程师、审查员、运维）。之后照 myco join 输出里 receive 的说明接收消息：会话开着时，在后台一直运行 myco inbox wait --timeout 55m，收到消息用 myco inbox pull 读取并处理。`,
    `Join myco: run ${command} in a terminal, then in this session run myco join${into} --display-name "your role" --description "one line on what you do" (fill in your own: a short role such as Backend engineer, Reviewer or Ops). Then receive messages as its "receive" output says: while the session is open keep myco inbox wait --timeout 55m running in the background, and read what it reports with myco inbox pull.`,
  );
}

/**
 * Adding agents: one sentence to copy into any AI session, which then joins
 * by itself; or a session on this computer, picked from a list. Whoever
 * joins while this is open shows up here at once, and goes into the project.
 */
export function AddAgentDialog({ open, hub, project, rows, onClose, onAdded }: {
  open: boolean;
  hub?: Hub;
  project?: Project;
  rows: AgentRow[];
  onClose: () => void;
  onAdded: () => void;
}) {
  const hubId = hub?.id || "";
  const [link, setLink] = useState<JoinLink | null>(null);
  const [linkError, setLinkError] = useState<unknown>(null);
  const [joined, setJoined] = useState<string[]>([]);
  const [sessions, setSessions] = useState<LocalSession[] | null>(null);
  const [sessionsError, setSessionsError] = useState<unknown>(null);
  const [joining, setJoining] = useState("");
  const [joinError, setJoinError] = useState<unknown>(null);
  // Who was already there when this opened: anyone new has just joined.
  const before = useRef<Set<string> | null>(null);
  const adding = useRef(new Set<string>());

  useEffect(() => {
    if (!open) return;
    before.current = new Set(rows.filter((r) => r.hub.id === hubId).map((r) => r.st.agent.name));
    adding.current = new Set();
    setJoined([]);
    setLink(null);
    setLinkError(null);
    setJoinError(null);
    setSessions(null);
    setSessionsError(null);
    if (!hubId) return;
    appApi.createJoinLink(hubId, { uses: 10, expires_hours: 168 }).then(setLink).catch(setLinkError);
    appApi.localSessions().then((r) => setSessions(r.sessions)).catch(setSessionsError);
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

  const joinLocal = async (s: LocalSession) => {
    setJoining(s.session_id);
    setJoinError(null);
    try {
      const r = await appApi.localJoin(hubId, { cli: s.cli, session_id: s.session_id, workdir: s.workdir, fork: true });
      adding.current.add(r.agent);
      await addToProject(hubId, project, rows, r.agent).catch(() => undefined);
      setJoined((j) => (j.includes(r.agent) ? j : [...j, r.agent]));
      onAdded();
    } catch (e) {
      setJoinError(e);
    } finally {
      setJoining("");
    }
  };

  const rowOf = (n: string) => rows.find((r) => r.hub.id === hubId && r.st.agent.name === n);
  const where = project ? L(`「${project.title || project.name}」`, `“${project.title || project.name}”`) : L("这里", "here");
  const free = (sessions || []).filter((s) => !s.joined).slice(0, 6);

  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={L("添加 agent", "Add an agent")}
      description={L(`把下面这句话发给你的 agent，它会自己加入${where}。Claude Code、Codex 或任何 AI 会话都可以，可以一口气发给好几个。`, `Send this sentence to your agent and it joins ${where} by itself. Claude Code, Codex or any AI session works; send it to as many as you like.`)}
      className="app-modal-add">
      <div className="add-sentence">
        {link ? (
          <>
            <p className="add-sentence-text">{joinSentence(link.command, project?.name)}</p>
            <div className="add-sentence-actions"><CopyButton value={joinSentence(link.command, project?.name)} /></div>
          </>
        ) : linkError ? <ErrorText error={linkError} /> : <p className="add-sentence-text is-loading">{L("正在准备…", "Getting it ready…")}</p>}
      </div>

      {/* Who has joined while this was open: they show up as they arrive. */}
      <div className="add-joined" aria-live="polite">
        {joined.length === 0 ? (
          <div className="add-waiting"><Mushroom idle /><span>{L("发出去以后，加入的 agent 会出现在这里。", "Once you've sent it, the agents that join show up here.")}</span></div>
        ) : joined.map((n) => {
          const r = rowOf(n);
          return (
            <div key={n} className="add-arrived">
              {r && <Avatar agent={r.st.agent} size={32} />}
              <span><b>{r ? nodeName(r.st.agent, r.hub) : n}</b>{L(" 加入了", " joined")}</span>
              <Check className="add-ok" />
            </div>
          );
        })}
      </div>

      {(free.length > 0 || !!sessionsError) && (
        <div className="add-local">
          <div className="add-local-title">
            <Monitor />{L("或者，直接用这台电脑上最近的会话", "Or use a recent session on this computer")}
          </div>
          {free.map((s) => (
            <div key={`${s.cli}:${s.session_id}`} className="add-local-row">
              <span className="add-local-main">
                <span className="add-local-name">{s.title || folderOf(s.workdir)}</span>
                <span className="add-local-sub">{s.cli === "codex" ? "Codex" : "Claude Code"} · {s.workdir} · {timeAgo(s.last_active)}</span>
              </span>
              <button type="button" className="app-btn app-btn-small" disabled={!!joining} onClick={() => void joinLocal(s)}>
                {joining === s.session_id ? L("正在加入…", "Joining…") : L("加入", "Add")}
              </button>
            </div>
          ))}
          <ErrorText error={joinError} />
        </div>
      )}

      <div className="app-modal-actions">
        <button type="button" className="app-btn app-btn-primary" onClick={onClose}>{L("好了", "Done")}</button>
      </div>
    </Modal>
  );
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
  const folders = [...new Set(rows.filter((r) => r.hub.id === hubId && r.st.agent.machine_id === machineId).map((r) => r.st.agent.binding.workdir).filter((d): d is string => !!d))].slice(0, 6);

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!machine || !dir.trim() || busy || users.length === 0) return;
    setBusy(new Date().toISOString());
    setError(null);
    try {
      const r = await appApi.spawn(hubId, machine.id, { cli: "exec", dir: dir.trim(), allow_from: users });
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
            {folders.map((d) => <button key={d} type="button" className={cn("app-convpick-folder mono", d === dir.trim() && "active")} onClick={() => setDir(d)} disabled={!!busy}>{d}</button>)}
          </div>
        )}
        <div className="ws-who">
          {users.length > 0
            ? L(`项目里的 ${users.length} 个 agent 都能在这里跑命令，之后加入的也能。`, `The project's ${users.length} agent${users.length > 1 ? "s" : ""} can run commands here, and so can those who join later.`)
            : L("先给项目加一个 agent，它才能用这台电脑。", "Add an agent to the project first, so it has someone to use it.")}
        </div>
        {machine && <LocalShellAccess machine={machine} what="shell" />}
        <ErrorText error={error} />
        {busy ? (
          <div className="app-convpick-starting"><Mushroom /><span>{L("正在准备", "Setting it up")} · <Elapsed since={busy} /></span></div>
        ) : (
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={onClose}>{L("取消", "Cancel")}</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={!machine || !dir.trim() || users.length === 0}>{L("添加", "Add")}</button>
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
