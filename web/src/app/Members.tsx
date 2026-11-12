import { useEffect, useMemo, useState } from "react";
import type { LocalSession, LocalState, RecentItem } from "@/appapi";
import { MessageCircle, Plus, Search, X } from "lucide-react";
import { appApi } from "@/appapi";
import type { Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, cliName, cloudLabel, isCloud, isInbox, isMe, machineOfRow, nodeName } from "@/app/nodes";
import { shortDir } from "@/app/AddToProject";
import { Avatar, isWorkspace, stateWords, whereOf } from "@/app/persona";
import { shareWorkspaces } from "@/app/AddToProject";
import { ErrorText } from "@/app/dialogs";
import { Inspector, OfflineCard } from "@/app/Inspector";
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

        {row.st.state !== "offline" && row.st.session_in_use && row.st.receiving !== "session" && (
          <div className="ms-inuse"><span className="dot pending" />{L("它的会话正开着，新消息先排队，过一会儿再送（详细信息里有办法）。", "Its session is open: new messages wait and are retried (see Details for a fix).")}</div>
        )}
        <WhoItIs row={row} now={now} />
        {!ws && <RecentWork row={row} now={now} />}

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
        {row.st.state === "offline" && <div className="ms-alert"><OfflineCard row={row} now={now} onRefresh={onRefresh} /></div>}
        <ErrorText error={error} />
        <Inspector embedded row={row} now={now} projects={projects} onClose={onClose} onRefresh={onRefresh}
          onThread={() => onChat(row)} onPeer={() => undefined} onMessage={onChat}
          onOpenMachine={onOpenMachine} onOpenProject={onOpenProject} onProjectsChanged={onChanged} />
        {menu.element}
      </aside>
    </>
  );
}

// What this computer is and which sessions are on it, asked once a minute
// at most: opening cards one after another does not rescan.
const soon = <T,>(get: () => Promise<T>, fallback: T) => {
  let at = 0;
  let p: Promise<T> | null = null;
  return () => {
    if (!p || Date.now() - at > 60_000) {
      at = Date.now();
      p = get().catch(() => fallback);
    }
    return p;
  };
};
const localSoon = soon<LocalState | null>(() => appApi.local(), null);
const sessionsSoon = soon<LocalSession[]>(() => appApi.localSessions().then((r) => r.sessions), []);
const recentSoon = new Map<string, () => Promise<RecentItem[]>>();
function recentOf(hubId: string): Promise<RecentItem[]> {
  let f = recentSoon.get(hubId);
  if (!f) {
    f = soon(() => appApi.recent(hubId).then((r) => r.items), [] as RecentItem[]);
    recentSoon.set(hubId, f);
  }
  return f();
}

/**
 * Who this is, for when you have forgotten which session it was: where it
 * runs, which AI, the session it came from (its first request, when it is
 * on this computer), whose it is and since when.
 */
function WhoItIs({ row, now }: { row: AgentRow; now: number }) {
  const a = row.st.agent;
  const [local, setLocal] = useState<LocalState | null>(null);
  const [session, setSession] = useState<LocalSession | null>(null);
  const machine = machineOfRow(row);
  useEffect(() => {
    let off = false;
    void localSoon().then((l) => { if (!off) setLocal(l); });
    setSession(null);
    if (a.binding.session_id && !isCloud(a)) {
      void sessionsSoon().then((list) => { if (!off) setSession(list.find((x) => x.session_id === a.binding.session_id) || null); });
    }
    return () => { off = true; };
  }, [a.name, a.binding.session_id]);
  const here = !!machine && !!local && ((local.machines || []).includes(machine.name) || (!!local.hostname && machine.hostname === local.hostname));
  const ws = isWorkspace(a);
  const where = isCloud(a) ? cloudLabel(a)
    : [machine ? (here ? L("这台电脑", "This computer") : machine.name) : "", a.binding.workdir ? shortDir(a.binding.workdir) : ""].filter(Boolean).join(" · ") || "—";
  const model = a.runtime?.model;
  const mine = !a.owner || row.hub.me?.member?.id === a.owner_id || isMe(a.owner, row.hub);
  const rows: [string, string, string?][] = [
    [L("在哪", "Where"), where, a.binding.workdir || undefined],
    ...(!ws ? [[L("用的", "Runs"), [cliName(a), model].filter(Boolean).join(" · ")] as [string, string]] : []),
    ...(session ? [[L("会话", "Session"), L(`「${session.title || folderOfDir(session.workdir)}」· ${timeAgo(session.last_active, now)}用过`, `“${session.title || folderOfDir(session.workdir)}” · used ${timeAgo(session.last_active, now)}`), session.title] as [string, string, string?]]
      : isInbox(a) ? [[L("会话", "Session"), L("只在它的会话开着时收消息", "Receives only while its session is open")] as [string, string]] : []),
    ...(!mine ? [[L("主人", "Owner"), a.owner || ""] as [string, string]] : []),
    [L("加入", "Joined"), L(`${timeAgo(a.created_at, now)}`, timeAgo(a.created_at, now)) + (row.st.state === "offline" && a.last_seen_at ? L(` · 上次在线 ${timeAgo(a.last_seen_at, now)}`, ` · last online ${timeAgo(a.last_seen_at, now)}`) : "")],
  ];
  return (
    <div className="ms-block">
      <div className="ms-block-title"><span>{ws ? L("这台电脑", "This computer") : L("它是谁", "Who it is")}</span></div>
      <div className="ms-facts">
        {rows.map(([k, v, full]) => (
          <div key={k} className="ms-fact">
            <span className="ms-fact-k">{k}</span>
            <span className="ms-fact-v" title={full || v}>{v}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

const WEEK_MS = 7 * 24 * 3600 * 1000;

/** What it has been doing lately: this week in three numbers, and its
 *  last few tasks in a line each. */
function RecentWork({ row, now }: { row: AgentRow; now: number }) {
  const name = row.st.agent.name;
  const [items, setItems] = useState<RecentItem[] | null>(null);
  useEffect(() => {
    let off = false;
    setItems(null);
    void recentOf(row.hub.id).then((list) => { if (!off) setItems(list); });
    return () => { off = true; };
  }, [row.hub.id, name]);
  if (items === null) return null;
  const mine = items.filter((it) => it.participants.includes(name)).sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  const week = mine.filter((it) => now - new Date(it.updated_at).getTime() < WEEK_MS);
  const count = (st: string[]) => week.filter((it) => st.includes(it.state || "")).length;
  const done = count(["done"]);
  const failed = count(["failed"]);
  const open = Math.max(count(["working", "queued"]), row.st.in_progress + row.st.queued);
  const word = (st?: string) => st === "done" ? L("做完", "Done") : st === "failed" ? L("失败", "Failed") : st === "working" ? L("在做", "Working") : st === "queued" ? L("排队", "Queued") : L("消息", "Note");
  return (
    <div className="ms-block">
      <div className="ms-block-title"><span>{L("最近在做", "Lately")}</span></div>
      <div className="ms-stats">
        <span><b>{done}</b>{L("做完", "done")}</span>
        <span><b>{open}</b>{L("在做", "open")}</span>
        <span className={cn(failed > 0 && "is-bad")}><b>{failed}</b>{L("失败", "failed")}</span>
        <span className="ms-stats-note">{L("最近 7 天", "last 7 days")}</span>
      </div>
      {mine.slice(0, 3).map((it) => {
        const text = (it.open || it.task || it.first)?.body || "";
        return (
          <div key={it.thread_id} className="ms-task">
            <span className={cn("ms-task-state", `is-${it.state || "none"}`)}>{word(it.state)}</span>
            <span className="ms-task-text" title={text}>{text.split("\n")[0]}</span>
            <span className="ms-task-time">{timeAgo(it.updated_at, now)}</span>
          </div>
        );
      })}
      {mine.length === 0 && <p className="ms-note">{L("最近没有派给它的活。", "No tasks for it lately.")}</p>}
    </div>
  );
}

function folderOfDir(dir: string): string {
  return dir.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || dir;
}
