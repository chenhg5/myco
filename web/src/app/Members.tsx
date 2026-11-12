import { useEffect, useMemo, useState } from "react";
import { MessageCircle, Plus, Search, X } from "lucide-react";
import { appApi } from "@/appapi";
import type { Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, nodeName } from "@/app/nodes";
import { Avatar, isWorkspace, stateWords, whereOf } from "@/app/persona";
import { shareWorkspaces } from "@/app/AddToProject";
import { ErrorText } from "@/app/dialogs";
import { Inspector } from "@/app/Inspector";
import { useContextMenu } from "@/app/ContextMenu";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

type Projects = { hubId: string; p: Project }[];

// Not seen for this long: resting, folded away at the bottom.
const REST_MS = 3 * 24 * 3600 * 1000;

function resting(r: AgentRow, now: number): boolean {
  if (r.st.state !== "offline") return false;
  const seen = new Date(r.st.agent.last_seen_at || r.st.agent.created_at).getTime();
  return !Number.isFinite(seen) || now - seen > REST_MS;
}

/** The projects an agent or workspace is in. */
export function projectsOf(r: AgentRow, projects: Projects): Project[] {
  return projects.filter((x) => x.hubId === r.hub.id && !x.p.archived && x.p.members.some((m) => m.name === r.st.agent.name)).map((x) => x.p);
}

/**
 * Everyone you work with, like a phone's contacts: each agent and
 * workspace with its face, its name, what it does and where it works.
 * Those not seen for days rest at the bottom.
 */
export function MembersPage({ rows, projects, now, onOpen }: {
  rows: AgentRow[];
  projects: Projects;
  now: number;
  onOpen: (r: AgentRow) => void;
}) {
  const [query, setQuery] = useState("");
  const [showResting, setShowResting] = useState(false);
  const people = rows.filter((r) => {
    const k = agentKind(r.st.agent);
    return k === "agent" || k === "shell";
  });
  const q = query.trim().toLowerCase();
  const match = (r: AgentRow) => !q || [nodeName(r.st.agent, r.hub), r.st.agent.name, r.st.agent.description, r.st.agent.binding.workdir, ...projectsOf(r, projects).map((p) => p.title || p.name)]
    .some((s) => (s || "").toLowerCase().includes(q));
  const order = (a: AgentRow, b: AgentRow) =>
    rank(a) - rank(b) || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub));
  const list = people.filter(match);
  const awake = list.filter((r) => !resting(r, now)).sort(order);
  const asleep = list.filter((r) => resting(r, now)).sort(order);
  const agents = awake.filter((r) => !isWorkspace(r.st.agent));
  const spaces = awake.filter((r) => isWorkspace(r.st.agent));

  return (
    <div className="mb">
      <div className="mb-search">
        <Search aria-hidden />
        <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={L("找一个成员：名字、分工、项目", "Find someone: name, role, project")} aria-label={L("搜索成员", "Search members")} />
      </div>
      {people.length === 0 && <div className="app-empty-note">{L("还没有成员。到一个项目里点「添加 agent」。", "No members yet. Open a project and choose Add an agent.")}</div>}
      {agents.length > 0 && <Section title={L("agent", "Agents")} rows={agents} projects={projects} now={now} onOpen={onOpen} />}
      {spaces.length > 0 && <Section title={L("电脑", "Computers")} rows={spaces} projects={projects} now={now} onOpen={onOpen} />}
      {asleep.length > 0 && (
        <>
          <button type="button" className="mb-rest-toggle" onClick={() => setShowResting((v) => !v)}>
            {showResting ? L(`收起休息中的 ${asleep.length} 个`, `Hide ${asleep.length} resting`) : L(`休息中 ${asleep.length} 个（好几天没上线）`, `${asleep.length} resting (away for days)`)}
          </button>
          {showResting && <Section rows={asleep} projects={projects} now={now} onOpen={onOpen} />}
        </>
      )}
      {list.length === 0 && people.length > 0 && <div className="app-empty-note">{L("没找到。", "Nobody matches.")}</div>}
    </div>
  );
}

function rank(r: AgentRow): number {
  return r.st.state === "working" ? 0 : r.st.state === "pending" ? 1 : r.st.state === "offline" ? 3 : 2;
}

function Section({ title, rows, projects, now, onOpen }: { title?: string; rows: AgentRow[]; projects: Projects; now: number; onOpen: (r: AgentRow) => void }) {
  return (
    <section className="mb-section">
      {title && <h3 className="mb-section-title">{title}</h3>}
      <div className="mb-grid">
        {rows.map((r) => {
          const a = r.st.agent;
          const ps = projectsOf(r, projects);
          return (
            <button key={`${r.hub.id}/${a.name}`} type="button" className={cn("mb-card", r.st.state === "offline" && "is-away")} onClick={() => onOpen(r)}>
              <Avatar agent={a} size={44} state={r.st.state} />
              <span className="mb-card-text">
                <span className="mb-card-name">{nodeName(a, r.hub)}</span>
                <span className="mb-card-desc">{isWorkspace(a) ? whereOf(r) : a.description || stateWords(r.st.state, a.status_text)}</span>
                <span className="mb-card-foot">
                  {ps.length > 0 ? ps.slice(0, 2).map((p) => <span key={p.name} className="mb-chip">{p.title || p.name}</span>) : <span className="mb-chip is-none">{L("还没进项目", "No project yet")}</span>}
                  {ps.length > 2 && <span className="mb-chip">+{ps.length - 2}</span>}
                  {r.st.state === "offline" && <span className="mb-seen">{timeAgo(a.last_seen_at || a.created_at, now)}{L("在线", " online")}</span>}
                </span>
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

/**
 * One member's card, everything in one place: rename it, say what it
 * does, chat with it, put it in projects; below, what it is working on,
 * its skills, the technical details (folded) and managing it.
 */
export function MemberSheet({ row, projects, rows, now, onClose, onChat, onChanged, onRefresh, onOpenProject, onOpenMachine }: {
  row: AgentRow | null;
  projects: Projects;
  rows: AgentRow[];
  now: number;
  onClose: () => void;
  onChat: (r: AgentRow) => void;
  onChanged: () => void;
  onRefresh: () => Promise<void>;
  onOpenProject: (ref: { hubId: string; name: string }) => void;
  onOpenMachine: () => void;
}) {
  const a = row?.st.agent;
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const menu = useContextMenu();
  useEffect(() => {
    if (!a || !row) return;
    setName(nodeName(a, row.hub));
    setDesc(a.description || "");
    setError(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [row?.hub.id, a?.name]);
  useEffect(() => {
    if (!row) return;
    // A dialog opened from the card closes first.
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape" && !document.querySelector(".app-modal")) onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [row, onClose]);
  const mine = useMemo(() => (row ? projectsOf(row, projects) : []), [row, projects]);
  if (!row || !a) return null;
  const ws = isWorkspace(a);
  const others = projects.filter((x) => x.hubId === row.hub.id && !x.p.archived && !mine.some((p) => p.name === x.p.name));
  const dirty = name.trim() !== nodeName(a, row.hub) || desc.trim() !== (a.description || "");

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      onChanged();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const save = () => run(() => appApi.updateProfile(row.hub.id, a.name, { display_name: name.trim(), description: desc.trim() }));
  const join = (p: Project) => run(async () => {
    const np = await appApi.putProjectMember(row.hub.id, p.name, a.name);
    if (!ws) await shareWorkspaces(row.hub.id, np, rows, a.name);
  });
  const leave = (p: Project) => run(() => appApi.removeProjectMember(row.hub.id, p.name, a.name));

  return (
    <>
      <div className="ms-scrim" onClick={onClose} />
      <aside className="ms" role="dialog" aria-label={nodeName(a, row.hub)}>
        <button type="button" className="ms-close app-btn app-btn-plain" onClick={onClose} aria-label={L("关闭", "Close")}><X /></button>
        <div className="ms-face"><Avatar agent={a} size={72} state={row.st.state} /></div>
        <input className="ms-name" value={name} onChange={(e) => setName(e.target.value)} aria-label={L("名字", "Name")} />
        <input className="ms-desc" value={desc} title={desc} onChange={(e) => setDesc(e.target.value)}
          placeholder={ws ? L("这个文件夹是做什么的", "What this folder is for") : L("它负责什么？比如 维护后端接口和数据库", "What does it do? e.g. Looks after the backend and the database")} aria-label={L("分工", "What it does")} />
        <div className="ms-state">{ws ? whereOf(row) : stateWords(row.st.state, a.status_text)}</div>
        {dirty && <div className="ms-save"><button type="button" className="app-btn app-btn-primary app-btn-small" disabled={busy || !name.trim()} onClick={() => void save()}>{L("保存", "Save")}</button></div>}

        <button type="button" className="app-btn ms-chat" onClick={() => onChat(row)}>
          <MessageCircle />{ws ? L("打开终端", "Open terminal") : L("单聊", "Chat")}
        </button>

        <div className="ms-block">
          <div className="ms-block-title">
            <span>{L("所在项目", "Projects")}</span>
            {others.length > 0 && (
              <button type="button" className="sp-side-add" disabled={busy} title={L("加到另一个项目", "Add to another project")} aria-label={L("加到另一个项目", "Add to another project")}
                onClick={(e) => menu.openUnder(e.currentTarget, [{ header: L("加到项目", "Add to project") }, ...others.map((x) => ({ label: x.p.title || x.p.name, onSelect: () => void join(x.p) }))])}>
                <Plus />
              </button>
            )}
          </div>
          {mine.length === 0 && <p className="ms-note">{L("还没进项目。点右上角的 + 放进一个。", "Not in a project yet. Use + to put it in one.")}</p>}
          {mine.map((p) => (
            <div key={p.name} className="ms-project">
              <button type="button" className="ms-project-name" onClick={() => onOpenProject({ hubId: row.hub.id, name: p.name })}>{p.title || p.name}</button>
              <button type="button" className="ms-leave" disabled={busy} onClick={() => void leave(p)}>{L("移出", "Remove")}</button>
            </div>
          ))}
        </div>
        <ErrorText error={error} />
        <Inspector embedded row={row} now={now} projects={projects} onClose={onClose} onRefresh={onRefresh}
          onThread={() => onChat(row)} onPeer={() => undefined} onMessage={onChat}
          onOpenMachine={onOpenMachine} onOpenProject={onOpenProject} onProjectsChanged={onChanged} />
        {menu.element}
      </aside>
    </>
  );
}
