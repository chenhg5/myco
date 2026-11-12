import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Activity, ArrowLeft, ArrowUp, ChevronDown, FileText, Maximize2, Minimize2, Plus, Users, X } from "lucide-react";
import { appApi, type RecentItem } from "@/appapi";
import type { Message, Project } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, isMe, nodeName } from "@/app/nodes";
import { Avatar, AvatarStack, isWorkspace, stateWords, whereOf } from "@/app/persona";
import { ConversationDetail, type ConversationTarget } from "@/app/ConversationDetail";
import { Mushroom, useVerb } from "@/app/Working";
import { MessageContent } from "@/app/Markdown";
import { ErrorText } from "@/app/dialogs";
import { useContextMenu } from "@/app/ContextMenu";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";
import { recall, remember } from "@/lib/memo";

/** A project, or the agents not in any project yet (one per hub). */
export type SpaceRef = { hubId: string; name: string } | { hubId: string; loose: true };

export function isLoose(s: SpaceRef): s is { hubId: string; loose: true } {
  return "loose" in s;
}

/** What the right pane shows: a member's work, or a computer's terminal. */
type Side = { agent: string; peer?: string; focus?: string; also?: string[] };

const SPLIT_KEY = "myco-space-split";
// The members column, and the least each pane needs.
const SIDE_W = 248;
const PANE_MIN = 320;

type View =
  | { kind: "home" }
  | { kind: "activity" }
  | { kind: "chat"; agent: string; peer?: string; focus?: string; also?: string[] }
  | { kind: "thread"; threadId: string; agent: string }
  | { kind: "pair"; agent: string; peer: string; focus?: string };

const POLL_MS = 4000;

/**
 * A project as people see it. You talk to its coordinator (the home view):
 * you say what you want, it hands the work out, and each task it hands out
 * shows up in your chat as a small card that keeps its state. On the side,
 * what each member is doing now; a member opens everything it was asked
 * (by the coordinator, by you) in one timeline, and you can talk to it
 * directly there. Everything between the members is under All activity,
 * for looking back. The agents in no project yet have the group view: a
 * feed of their tasks and a box to give one a task.
 */
export function ProjectSpace({ space, project, looseRows = [], hubName, rows, now, me, openChat, home, onAddAgent, onAddWorkspace, onOpenMember, onAbout, onRefresh }: {
  space: SpaceRef;
  /** the project (none for the agents not in a project) */
  project?: Project;
  /** without a project: the agents and workspaces in none */
  looseRows?: AgentRow[];
  hubName: string;
  rows: AgentRow[];
  now: number;
  /** your name as a sender on this hub (@console:you), when known */
  me?: string;
  /** open the chat with this member (n: each request anew) */
  openChat?: { agent: string; n: number };
  /** bumped when the space is picked again: back to its group */
  home?: number;
  onAddAgent: () => void;
  onAddWorkspace: () => void;
  onOpenMember: (row: AgentRow) => void;
  onAbout?: () => void;
  onRefresh: () => void;
}) {
  const hubId = space.hubId;
  const loose = isLoose(space);
  const hubRows = rows.filter((r) => r.hub.id === hubId);
  const rowOf = (name: string) => hubRows.find((r) => r.st.agent.name === name);
  // Members: the project's, or (loose) every agent and workspace in no project.
  const members = useMemo(() => {
    if (project) return project.members.map((m) => rowOf(m.name)).filter((r): r is AgentRow => !!r);
    return looseRows.filter((r) => r.hub.id === hubId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project, rows, hubId, looseRows]);
  const agents = members.filter((r) => !isWorkspace(r.st.agent) && r.st.agent.kind !== "human" && r.st.agent.kind !== "machine");
  const workspaces = members.filter((r) => isWorkspace(r.st.agent));
  const memberNames = new Set(members.map((r) => r.st.agent.name));

  // The coordinator: the member set as one, else the first agent to join.
  const coordinatorSet = project?.members.find((m) => m.role === "coordinator" && agents.some((r) => r.st.agent.name === m.name))?.name;
  const coordinator = coordinatorSet || (project
    ? [...project.members].filter((m) => agents.some((r) => r.st.agent.name === m.name)).sort((a, b) => a.added_at.localeCompare(b.added_at))[0]?.name
    : undefined);
  const setCoordinator = async (name: string) => {
    if (!project) return;
    for (const m of project.members.filter((x) => x.role === "coordinator" && x.name !== name)) {
      await appApi.putProjectMember(hubId, project.name, m.name, { role: "member" }).catch(() => undefined);
    }
    await appApi.putProjectMember(hubId, project.name, name, { role: "coordinator" }).catch(() => undefined);
    onRefresh();
  };
  const feed = useFeed(hubId, project, memberNames);
  // A project always has a coordinator once it has an agent: the agent
  // learns it from its role, so the default is saved, not only shown.
  const saved = useRef(new Set<string>());
  useEffect(() => {
    if (!project || coordinatorSet || !coordinator) return;
    const k = `${hubId}\u0000${project.name}\u0000${coordinator}`;
    if (saved.current.has(k)) return;
    saved.current.add(k);
    void appApi.putProjectMember(hubId, project.name, coordinator, { role: "coordinator" }).then(onRefresh).catch(() => undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hubId, project?.name, coordinatorSet, coordinator]);

  const [view, setView] = useState<View>({ kind: "home" });
  // Your chat with an agent: the name you have talked to it as (this
  // device's, or your own on the hub), found among your conversations;
  // your own name when there are none yet.
  // Who you are to each agent (kept, so a chat opens at once).
  const pairsKey = `pairs\u0000${hubId}`;
  const loadPairs = async () => {
    const r = await appApi.recent(hubId, { group: "pair", side: "me" });
    return remember(pairsKey, r.items.filter((x) => x.kind === "pair").map((x) => ({ node: x.node || "", other: x.other || "" })));
  };
  useEffect(() => { void loadPairs().catch(() => undefined); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [hubId]);
  const peerIn = (pairs: { node: string; other: string }[] | undefined, agent: string) => {
    const it = (pairs || []).find((x) => x.node === agent && x.other.startsWith("@console:"))
      || (pairs || []).find((x) => x.other === agent && x.node.startsWith("@console:"));
    return it ? (it.node === agent ? it.other : it.node) : undefined;
  };
  const peerFor = async (agent: string): Promise<string | undefined> => {
    const known = peerIn(recall(pairsKey), agent);
    if (known) {
      void loadPairs().catch(() => undefined);
      return known;
    }
    try {
      return peerIn(await loadPairs(), agent) || me;
    } catch {
      return me;
    }
  };
  // Everyone a member has worked with here (from the project's tasks):
  // their conversations with it are shown along with yours.
  const othersOf = (agent: string): string[] => {
    const set = new Set<string>();
    for (const it of feed.items || []) {
      if (it.kind !== "thread" || !it.participants.includes(agent)) continue;
      for (const p of it.participants) if (p !== agent && !p.startsWith("@")) set.add(p);
    }
    if (coordinator && coordinator !== agent) set.add(coordinator);
    return [...set].sort();
  };
  // With a coordinator the project is split: your chat with it on the
  // left, and the member you open (its work, or a computer's terminal) on
  // the right; either can take the whole width.
  const split = !!project && !!coordinator;
  const [side, setSide] = useState<Side | null>(null);
  const [only, setOnly] = useState<"" | "left" | "right">("");
  const chatWith = async (agent: string, focus?: string) => {
    // The coordinator is the home view.
    if (agent === coordinator && !focus) {
      setView({ kind: "home" });
      if (only === "right") setOnly("");
      return;
    }
    if (split) {
      // At once, as whoever you were to it last time (or yourself); if the
      // hub says otherwise, the pane follows.
      const known = peerIn(recall(pairsKey), agent);
      const next = { agent, peer: known || me, focus, also: project ? othersOf(agent) : undefined };
      setView({ kind: "home" });
      setSide(next);
      if (only === "left") setOnly("");
      if (!known) {
        void peerFor(agent).then((p) => {
          if (p && p !== next.peer) setSide((s) => (s && s.agent === agent ? { ...s, peer: p } : s));
        });
      }
      return;
    }
    setView({ kind: "chat", agent, peer: await peerFor(agent), focus, also: project ? othersOf(agent) : undefined });
  };
  // A workspace opens its terminal: your exchange with it.
  const open = (r: AgentRow) => {
    const name = r.st.agent.name;
    if (!isWorkspace(r.st.agent)) { void chatWith(name); return; }
    void peerFor(name).then((peer) => {
      if (!peer) return;
      if (split) { setView({ kind: "home" }); setSide({ agent: name, peer }); if (only === "left") setOnly(""); }
      else setView({ kind: "pair", agent: name, peer });
    });
  };
  const spaceKey = loose ? `${hubId}\u0000*` : `${hubId}\u0000${(space as { name: string }).name}`;
  useEffect(() => { setView({ kind: "home" }); setSide(null); setOnly(""); }, [spaceKey, home]);
  // Who you are to the coordinator (your chat with it is the home view).
  const [homePeer, setHomePeer] = useState<string | undefined>(() => (coordinator ? peerIn(recall(pairsKey), coordinator) : undefined) || me);
  useEffect(() => {
    if (!coordinator) return;
    let off = false;
    void peerFor(coordinator).then((p) => { if (!off) setHomePeer(p || me); });
    return () => { off = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [coordinator, hubId]);
  useEffect(() => {
    if (!openChat) return;
    const r = hubRows.find((x) => x.st.agent.name === openChat.agent);
    if (r) open(r);
    else void chatWith(openChat.agent);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openChat?.n]);

  const nameOf = (n: string) => {
    if (n.startsWith("@console:") || n.startsWith("@")) return isMe(n, hubRows[0]?.hub) ? L("你", "You") : n.replace(/^@console:/, "");
    const r = rowOf(n);
    return r ? nodeName(r.st.agent, r.hub) : n;
  };

  const target = (v: View): ConversationTarget | null => {
    if (v.kind === "chat") return { hubId, hubName, threadId: "", agent: v.agent, peer: v.peer, project: project?.name, focus: v.focus, also: v.also };
    if (v.kind === "thread") return { hubId, hubName, threadId: v.threadId, agent: v.agent };
    if (v.kind === "pair") return { hubId, hubName, threadId: "", agent: v.agent, peer: v.peer, focus: v.focus };
    return null;
  };
  const t = target(view);
  const spRef = useRef<HTMLDivElement>(null);
  const [spWidth, setSpWidth] = useState(0);
  useEffect(() => {
    const el = spRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setSpWidth(el.clientWidth));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  // Narrow windows have no members column: the faces in a row.
  const strip = (
    <div className="sp-strip">
      {[...agents, ...workspaces].map((r) => (
        <button key={r.st.agent.name} type="button" className="sp-strip-face" title={nodeName(r.st.agent, r.hub)}
          onClick={() => open(r)}>
          <Avatar agent={r.st.agent} size={30} state={r.st.state} badge={false} />
        </button>
      ))}
      <button type="button" className="sp-strip-face is-add" onClick={onAddAgent} aria-label={L("添加 agent", "Add an agent")} title={L("添加 agent", "Add an agent")}><Plus /></button>
    </div>
  );

  return (
    // With a member open beside the coordinator, a window too small for two
    // panes and the members column folds the column into the row of faces.
    <div ref={spRef} className={cn("sp", split && side && view.kind === "home" && only !== "left" && spWidth - SIDE_W < 2 * PANE_MIN && "has-side")}>
      <div className="sp-main">
        {t ? (
          <ConversationDetail
            key={JSON.stringify(t)}
            target={t}
            rows={rows}
            now={now}
            showBack
            backLabel={loose ? L("返回", "Back") : L(`返回「${project?.title || project?.name}」`, `Back to ${project?.title || project?.name}`)}
            onBack={() => setView({ kind: "home" })}
            role={view.kind === "chat" && view.agent === coordinator ? L("协调者", "Coordinator") : undefined}
            onNewConversation={(_h, agent) => void chatWith(agent)}
            onOpen={(_h, agent, peer) => (peer ? setView({ kind: "pair", agent, peer }) : void chatWith(agent))}
          />
        ) : project && coordinator && view.kind === "home" ? (
          <SplitPanes
            side={side}
            only={only}
            onOnly={setOnly}
            onCloseSide={() => { setSide(null); setOnly(""); }}
            left={(full) => (
              <HomeView
                key={`${spaceKey}\u0000${coordinator}\u0000${homePeer || ""}`}
                hubId={hubId}
                hubName={hubName}
                project={project}
                coordinator={coordinator}
                peer={homePeer}
                rows={rows}
                now={now}
                items={feed.items || []}
                rowOf={rowOf}
                nameOf={nameOf}
                onOpenTask={(agent, id) => void chatWith(agent, id)}
                onActivity={() => setView({ kind: "activity" })}
                onAbout={onAbout}
                onChat={(agent) => void chatWith(agent)}
                strip={strip}
                fullToggle={side ? full : undefined}
              />
            )}
            right={(actions) => side && (
              <ConversationDetail
                key={JSON.stringify(side)}
                target={{ hubId, hubName, threadId: "", agent: side.agent, peer: side.peer, project: project.name, focus: side.focus, also: side.also }}
                rows={rows}
                now={now}
                showBack={false}
                onBack={() => setSide(null)}
                headActions={actions}
                onNewConversation={(_h, agent) => void chatWith(agent)}
                onOpen={(_h, agent) => void chatWith(agent)}
              />
            )}
          />
        ) : (
          <GroupView
            hubId={hubId}
            project={project}
            loose={loose}
            title={loose ? L("还没进项目的", "Not in a project yet") : project?.title || project?.name || ""}
            agents={agents}
            memberNames={memberNames}
            rowOf={rowOf}
            nameOf={nameOf}
            now={now}
            onOpenThread={(it) => {
              if (it.kind === "pair") {
                const a = it.other?.startsWith("@console:") ? it.node || "" : it.other || "";
                const p = it.other?.startsWith("@console:") ? it.other : it.node;
                setView({ kind: "pair", agent: a, peer: p || "" });
                return;
              }
              // A task opens the whole conversation between its two sides,
              // at that task: yours with the member (where you can go on),
              // or theirs, to read.
              const task = it.open || it.task || it.first;
              const focus = task?.id;
              if (task && task.from && task.to) {
                const [agent, peer] = task.to.startsWith("@") ? [task.from, task.to] : [task.to, task.from];
                if (!agent.startsWith("@")) {
                  if (isMe(peer, hubRows[0]?.hub)) setView({ kind: "chat", agent, peer, focus, also: project ? othersOf(agent) : undefined });
                  else if (project && memberNames.has(agent)) void chatWith(agent, focus);
                  else setView({ kind: "pair", agent, peer, focus });
                  return;
                }
              }
              const agent = it.participants.find((p) => !p.startsWith("@") && memberNames.has(p)) || it.participants.find((p) => !p.startsWith("@")) || "";
              setView({ kind: "thread", threadId: it.thread_id, agent });
            }}
            onAddAgent={onAddAgent}
            onAddWorkspace={onAddWorkspace}
            onAbout={onAbout}
            onSent={onRefresh}
            feed={feed}
            // In a project with a coordinator this is All activity: a record
            // to look back at, with no box to write in.
            activity={!!project && !!coordinator}
            onBack={() => setView({ kind: "home" })}
            strip={strip}
          />
        )}
      </div>

      <MembersPanel
        agents={agents}
        workspaces={workspaces}
        loose={loose}
        selected={side && view.kind === "home" && only !== "left" ? side.agent : view.kind === "chat" ? view.agent : view.kind === "pair" ? view.agent : view.kind === "home" ? coordinator || "" : ""}
        coordinator={coordinator}
        nameOf={nameOf}
        now={now}
        onSetCoordinator={project ? (n) => void setCoordinator(n) : undefined}
        onChat={open}
        onProfile={onOpenMember}
        onAddAgent={onAddAgent}
        onAddWorkspace={onAddWorkspace}
      />
    </div>
  );
}

/** Agents and workspaces in no project of their hub yet. */
export function looseRowsOf(rows: AgentRow[], projects: { hubId: string; p: Project }[]): AgentRow[] {
  const inOne = new Set(projects.filter((x) => !x.p.archived).flatMap((x) => x.p.members.map((m) => `${x.hubId}\u0000${m.name}`)));
  return rows.filter((r) => {
    const k = agentKind(r.st.agent);
    return (k === "agent" || k === "shell") && !inOne.has(`${r.hub.id}\u0000${r.st.agent.name}`);
  });
}

// ---------------------------------------------------------------------------

type Feed = { items: RecentItem[] | null; error: unknown; load: () => Promise<void> };

/** The space's tasks (a project's, or the loose agents'), oldest first:
 *  as last seen at once, then refreshed every few seconds. */
function useFeed(hubId: string, project: Project | undefined, memberNames: Set<string>): Feed {
  const itemsKey = `space\u0000${hubId}\u0000${project?.name || "*"}`;
  const [items, setItemsState] = useState<RecentItem[] | null>(() => recall<RecentItem[]>(itemsKey) ?? null);
  const [error, setError] = useState<unknown>(null);
  const names = [...memberNames].sort().join(",");
  const load = useCallback(async () => {
    try {
      const r = await appApi.recent(hubId, project ? { project: project.name } : {});
      let list = r.items;
      // Not in a project: the threads with these agents that are in none.
      if (!project) list = list.filter((it) => !it.project && [...it.participants, it.other || "", it.node || ""].some((p) => memberNames.has(p)));
      // Oldest first, as a chat reads: by when each task started.
      list = [...list].sort((a, b) => (a.first?.created_at || a.updated_at).localeCompare(b.first?.created_at || b.updated_at));
      setItemsState(remember(itemsKey, list));
      setError(null);
    } catch (e) {
      setError(e);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hubId, project?.name, names]);
  useEffect(() => {
    setItemsState(recall<RecentItem[]>(itemsKey) ?? null);
    void load();
    const t = window.setInterval(() => { if (document.visibilityState === "visible") void load(); }, POLL_MS);
    return () => window.clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load]);
  return { items, error, load };
}

/**
 * The project's home: your conversation with its coordinator. The tasks it
 * hands out to the others appear in it as small cards, at the time they
 * were given, with how each is going; a card opens that member's work at
 * that task.
 */
function HomeView({ hubId, hubName, project, coordinator, peer, rows, now, items, rowOf, nameOf, onOpenTask, onActivity, onAbout, strip, fullToggle }: {
  hubId: string;
  hubName: string;
  project: Project;
  coordinator: string;
  peer?: string;
  rows: AgentRow[];
  now: number;
  items: RecentItem[];
  rowOf: (n: string) => AgentRow | undefined;
  nameOf: (n: string) => string;
  onOpenTask: (agent: string, taskId: string) => void;
  onActivity: () => void;
  onAbout?: () => void;
  onChat: (agent: string) => void;
  strip?: React.ReactNode;
  /** with a member open beside it: take the whole width, or give it back */
  fullToggle?: React.ReactNode;
}) {
  const inserts = items.flatMap((it) => {
    const task = it.task || it.first;
    if (it.kind !== "thread" || !task || task.type !== "task" || task.from !== coordinator || task.to === coordinator || task.to.startsWith("@")) return [];
    const to = task.to;
    const r = rowOf(to);
    const state = it.state || "";
    return [{
      id: task.id,
      at: task.created_at,
      node: (
        <button type="button" className="sp-dispatch" onClick={() => onOpenTask(to, task.id)} title={L("看它怎么做的", "See how it goes")}>
          <span className="sp-dispatch-label">{L("派给", "To")}</span>
          {r && <Avatar agent={r.st.agent} size={18} badge={false} />}
          <span className="sp-dispatch-who">{nameOf(to)}</span>
          <span className="sp-dispatch-text">{task.body.replace(/\s+/g, " ").trim()}</span>
          <span className={cn("sp-dispatch-state", `is-${state || "none"}`)}>{dispatchWord(state)}</span>
          <span className="sp-dispatch-time">{timeAgo(it.updated_at, now)}</span>
        </button>
      ),
    }];
  });
  return (
    <div className="sp-home">
      <header className="sp-head">
        <div className="sp-head-titles">
          <h2>{project.title || project.name}</h2>
          <p>{project.goal || L("跟协调者说你想做什么，它会把活分给大家。", "Tell the coordinator what you want; it hands the work out.")}</p>
        </div>
        <span className="sp-head-tools">
          <button type="button" className="app-btn app-btn-plain" onClick={onActivity} title={L("全部动态：成员之间的所有任务", "All activity: every task between the members")} aria-label={L("全部动态", "All activity")}>
            <Activity />
          </button>
          {onAbout && (
            <button type="button" className="app-btn app-btn-plain" onClick={onAbout} title={L("项目资料：目标、说明、文档", "Project notes: goal, brief, documents")} aria-label={L("项目资料", "Project notes")}>
              <FileText />
            </button>
          )}
          {fullToggle}
        </span>
      </header>
      {strip}
      <ConversationDetail
        target={{ hubId, hubName, threadId: "", agent: coordinator, peer, project: project.name }}
        rows={rows}
        now={now}
        showBack={false}
        role={L("协调者", "Coordinator")}
        inserts={inserts}
        onBack={() => undefined}
        onNewConversation={() => undefined}
      />
    </div>
  );
}

/**
 * Your chat with the coordinator and, beside it, the member you opened.
 * Drag the line between them to share the width; either can take all of
 * it, and the right one closes. Where two do not fit, the member's pane
 * takes the place (closing it brings the coordinator back).
 */
function SplitPanes({ side, only, onOnly, onCloseSide, left, right }: {
  side: Side | null;
  only: "" | "left" | "right";
  onOnly: (o: "" | "left" | "right") => void;
  onCloseSide: () => void;
  left: (fullToggle: React.ReactNode) => React.ReactNode;
  right: (actions: React.ReactNode) => React.ReactNode;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [ratio, setRatio] = useState(() => {
    try { const v = Number(localStorage.getItem(SPLIT_KEY)); return v >= 0.25 && v <= 0.75 ? v : 0.5; } catch { return 0.5; }
  });
  const latest = useRef(ratio);
  const [cramped, setCramped] = useState(false);
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setCramped(el.clientWidth < 2 * PANE_MIN));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  // Dragging the line: from where it is, by how far the pointer moves
  // (grabbing it a few pixels to one side must not make it jump there).
  const dragFrom = useRef<{ x: number; ratio: number; width: number } | null>(null);
  const dragStart = (e: React.PointerEvent<HTMLDivElement>) => {
    const el = box.current;
    if (!el) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    dragFrom.current = { x: e.clientX, ratio: latest.current, width: el.getBoundingClientRect().width };
    document.body.classList.add("is-resizing");
  };
  const dragMove = (e: React.PointerEvent<HTMLDivElement>) => {
    const d = dragFrom.current;
    if (!d) return;
    const r = Math.min(0.75, Math.max(0.25, d.ratio + (e.clientX - d.x) / d.width));
    latest.current = r;
    setRatio(r);
  };
  const dragEnd = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragFrom.current) return;
    dragFrom.current = null;
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
    document.body.classList.remove("is-resizing");
    try { localStorage.setItem(SPLIT_KEY, String(latest.current)); } catch { /* not kept */ }
  };
  const shown = !side ? "left" : cramped ? (only === "left" ? "left" : "right") : only || "both";
  const toggle = (pane: "left" | "right") => (
    <button type="button" className="app-btn app-btn-plain sp-pane-btn" onClick={() => onOnly(only === pane ? "" : pane)}
      title={only === pane ? L("还原成两栏", "Back to two panes") : L("只看这一栏", "Show only this pane")}
      aria-label={only === pane ? L("还原成两栏", "Back to two panes") : L("只看这一栏", "Show only this pane")}>
      {only === pane ? <Minimize2 /> : <Maximize2 />}
    </button>
  );
  return (
    <div ref={box} className={cn("sp-split", `show-${shown}`)} style={{ ["--split" as string]: `${ratio * 100}%` }}>
      <div className="sp-pane sp-pane-left">{left(side && !cramped ? toggle("left") : null)}</div>
      {side && (
        <>
          <div className="sp-divider" role="separator" aria-orientation="vertical" aria-label={L("拖动调整宽度", "Drag to resize")}
            onPointerDown={dragStart} onPointerMove={dragMove} onPointerUp={dragEnd} onPointerCancel={dragEnd} onDoubleClick={() => { setRatio(0.5); latest.current = 0.5; try { localStorage.setItem(SPLIT_KEY, "0.5"); } catch { /* not kept */ } }} />
          <div className="sp-pane sp-pane-right">
            {right(
              <>
                {!cramped && toggle("right")}
                <button type="button" className="app-btn app-btn-plain sp-pane-btn" onClick={onCloseSide} title={L("关掉这一栏", "Close this pane")} aria-label={L("关掉这一栏", "Close this pane")}><X /></button>
              </>,
            )}
          </div>
        </>
      )}
    </div>
  );
}

function dispatchWord(state: string): string {
  return state === "done" ? L("做完了", "Done") : state === "failed" ? L("没做成", "Didn't work")
    : state === "working" ? L("在做", "Working") : state === "queued" ? L("排队中", "Queued") : L("已发出", "Sent");
}

function GroupView({ hubId, project, loose, title, agents, memberNames, rowOf, nameOf, now, onOpenThread, onAddAgent, onAddWorkspace, onAbout, onSent, strip, feed, activity, onBack }: {
  hubId: string;
  project?: Project;
  loose: boolean;
  title: string;
  agents: AgentRow[];
  memberNames: Set<string>;
  rowOf: (n: string) => AgentRow | undefined;
  nameOf: (n: string) => string;
  now: number;
  onOpenThread: (it: RecentItem) => void;
  onAddAgent: () => void;
  onAddWorkspace?: () => void;
  onAbout?: () => void;
  onSent: () => void;
  strip?: React.ReactNode;
  feed: Feed;
  /** All activity: read-only, with a way back */
  activity?: boolean;
  onBack?: () => void;
}) {
  const { items, error, load } = feed;
  const scrollRef = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  void memberNames;

  // Keep to the newest, unless someone scrolled up to read.
  useEffect(() => {
    const el = scrollRef.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [items]);

  const sent = () => {
    atBottom.current = true;
    void load();
    onSent();
  };

  return (
    <div className="sp-group">
      <header className="sp-head">
        {activity && onBack && (
          <button type="button" className="app-btn app-btn-plain" onClick={onBack} aria-label={L("返回", "Back")} title={L("返回", "Back")}><ArrowLeft /></button>
        )}
        <div className="sp-head-titles">
          <h2>{activity ? L("全部动态", "All activity") : title}</h2>
          <p>
            {activity
              ? L(`「${title}」里所有成员之间的任务，留着回头查。`, `Every task between the members of ${title}, to look back at.`)
              : loose
              ? L("这些 agent 还没放进项目。建个项目把它们放进去，它们就能一起干活。", "These agents are not in a project yet. Put them in one so they can work together.")
              : project?.goal || L("给一个成员派活，看它们一起干。", "Give a member a task and watch them work together.")}
          </p>
        </div>
        {!loose && !activity && onAbout && (
          <button type="button" className="app-btn app-btn-quiet" onClick={onAbout}>
            <FileText />{L("项目资料", "Project notes")}
          </button>
        )}
      </header>
      {strip}

      <div className="sp-feed" ref={scrollRef} onScroll={() => {
        const el = scrollRef.current;
        if (el) atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
      }}>
        <div className="sp-feed-col">
          {items === null && !error && <div className="sp-feed-note">{L("正在读取…", "Loading…")}</div>}
          <ErrorText error={error} />
          {items && items.length === 0 && (
            agents.length === 0 ? (
              <EmptyProject loose={loose} onAddAgent={onAddAgent} onAddWorkspace={onAddWorkspace} />
            ) : (
              <div className="sp-feed-empty">
                <AvatarStack agents={agents.map((r) => r.st.agent)} size={40} max={5} />
                <p>{L("人齐了。在下面给其中一个派第一个活吧。", "Everyone's here. Give one of them the first task below.")}</p>
              </div>
            )
          )}
          {items?.map((it) => (
            <TaskCard key={`${it.kind}:${it.thread_id}:${it.node || ""}`} it={it} rowOf={rowOf} nameOf={nameOf} now={now} onOpen={() => onOpenThread(it)} />
          ))}
        </div>
      </div>

      {agents.length > 0 && !activity && <Composer hubId={hubId} project={project} agents={agents} onSent={sent} />}
    </div>
  );
}

function EmptyProject({ loose, onAddAgent, onAddWorkspace }: { loose: boolean; onAddAgent: () => void; onAddWorkspace?: () => void }) {
  return (
    <div className="sp-empty">
      <div className="sp-empty-art"><Mushroom idle /></div>
      <h3>{loose ? L("还没有 agent", "No agents yet") : L("项目建好了，拉几个 agent 进来吧", "Your project is ready. Bring in a few agents")}</h3>
      <p>{L("用这台电脑上的会话，或者发一句话给任何 AI，它就会自己加入。也可以先把一台电脑交给它们用，顺序随你。", "Use a session on this computer, or send one sentence to any AI and it joins by itself. You can also hand them a computer first; any order works.")}</p>
      <div className="sp-empty-actions">
        <button type="button" className="app-btn app-btn-primary" onClick={onAddAgent}><Plus />{L("添加 agent", "Add an agent")}</button>
        {!loose && onAddWorkspace && <button type="button" className="app-btn" onClick={onAddWorkspace}><Plus />{L("加一台电脑", "Add a computer")}</button>}
      </div>
    </div>
  );
}

/** One task in the group: who asked whom, what, and how it is going. */
function TaskCard({ it, rowOf, nameOf, now, onOpen }: {
  it: RecentItem;
  rowOf: (n: string) => AgentRow | undefined;
  nameOf: (n: string) => string;
  now: number;
  onOpen: () => void;
}) {
  const first = it.task || it.first;
  const from = first.from;
  const to = it.kind === "pair" ? (it.node || first.to) : first.to;
  const fromRow = rowOf(from);
  const toRow = rowOf(to);
  const answer = it.answer && it.answer.id !== first.id ? it.answer : it.last && it.last.id !== first.id && it.last.type !== "task" ? it.last : undefined;
  const ansRow = answer ? rowOf(answer.from) : undefined;
  const state = it.state || "";
  const verb = useVerb(state === "working");
  const workspace = !!toRow && isWorkspace(toRow.st.agent);
  return (
    <button type="button" className={cn("sp-card", state && `is-${state}`)} onClick={onOpen}>
      <div className="sp-card-who">
        <Who row={fromRow} name={nameOf(from)} />
        <span className="sp-card-arrow">→</span>
        <Who row={toRow} name={nameOf(to)} />
        <span className="sp-card-when">{timeAgo(first.created_at, now)}</span>
      </div>
      <div className="sp-card-body">
        {workspace ? <code className="sp-card-cmd">{first.body}</code> : <ClampText text={first.body} lines={4} />}
      </div>
      {answer && answer.type !== "progress" && (
        <div className="sp-card-reply">
          {ansRow ? <Avatar agent={ansRow.st.agent} size={18} badge={false} /> : null}
          <span className="sp-card-reply-text">{workspace ? L("命令跑完了", "The command finished") : firstLine(answer.body)}</span>
        </div>
      )}
      <div className="sp-card-foot">
        {state === "working" ? (
          <span className="sp-card-state is-working"><Mushroom />{verb}</span>
        ) : state === "queued" ? (
          <span className="sp-card-state"><Mushroom idle />{L("排队中", "Queued")}</span>
        ) : state === "failed" ? (
          <span className="sp-card-state is-failed">{L("没做成", "Didn't work out")}</span>
        ) : state === "done" ? (
          <span className="sp-card-state is-done">{L("做完了", "Done")}</span>
        ) : null}
        {it.messages > 2 && <span className="sp-card-count">{L(`共 ${it.messages} 条`, `${it.messages} messages`)}</span>}
      </div>
    </button>
  );
}

function Who({ row, name }: { row?: AgentRow; name: string }) {
  return (
    <span className="sp-who">
      {row ? <Avatar agent={row.st.agent} size={20} badge={false} /> : <span className="sp-who-me">{name.slice(0, 1)}</span>}
      <span className="sp-who-name">{name}</span>
    </span>
  );
}

function ClampText({ text, lines }: { text: string; lines: number }) {
  return <div className="sp-clamp" style={{ WebkitLineClamp: lines }}><MessageContent body={text} /></div>;
}

function firstLine(s: string): string {
  return (s || "").replace(/[#*`>_|-]+/g, " ").replace(/\s+/g, " ").trim();
}

// ---------------------------------------------------------------------------

const RECIPIENT_KEY = "myco-space-to";

/** The box at the bottom of the group: a task, and who it goes to. */
// The composer grows to this height, then scrolls.
const COMPOSE_MAX = 220;

function Composer({ hubId, project, agents, onSent }: {
  hubId: string;
  project?: Project;
  agents: AgentRow[];
  onSent: () => void;
}) {
  const key = `${RECIPIENT_KEY}:${hubId}/${project?.name || "*"}`;
  const coordinator = project?.members.find((m) => m.role === "coordinator")?.name;
  const [to, setTo] = useState(() => {
    let saved = "";
    try { saved = localStorage.getItem(key) || ""; } catch { /* none */ }
    return saved;
  });
  const valid = agents.some((r) => r.st.agent.name === to);
  const recipient = valid ? to : coordinator && agents.some((r) => r.st.agent.name === coordinator) ? coordinator : agents[0]?.st.agent.name || "";
  const rec = agents.find((r) => r.st.agent.name === recipient);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [hinted, setHinted] = useState(() => { try { return localStorage.getItem("myco-space-hinted") === "1"; } catch { return true; } });
  const menu = useContextMenu();
  const input = useRef<HTMLTextAreaElement>(null);
  // Grows with what is typed, as chat apps do; past COMPOSE_MAX it scrolls.
  // (field-sizing: content is not in every WebKit the app runs on.)
  useLayoutEffect(() => {
    const el = input.current;
    if (!el) return;
    el.style.height = "auto";
    const h = el.scrollHeight;
    el.style.height = `${Math.min(h, COMPOSE_MAX)}px`;
    el.style.overflowY = h > COMPOSE_MAX ? "auto" : "hidden";
  }, [draft]);
  const pick = (name: string) => {
    setTo(name);
    try { localStorage.setItem(key, name); } catch { /* not kept */ }
  };

  const send = async () => {
    const body = draft.trim();
    if (!body || !recipient || busy) return;
    setBusy(true);
    setError(null);
    try {
      await appApi.sendMessage(hubId, { to: recipient, body, ...(project ? { project: project.name } : {}) });
      setDraft("");
      if (!hinted) {
        setHinted(true);
        try { localStorage.setItem("myco-space-hinted", "1"); } catch { /* not kept */ }
      }
      onSent();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="sp-compose">
      <ErrorText error={error} />
      {/* The text takes the whole width and grows; below it a row of tools:
          who gets it (and later attachments and voice), then Send. */}
      <div className="sp-compose-box" onClick={(e) => { if (e.target === e.currentTarget) input.current?.focus(); }}>
        <textarea
          ref={input}
          className="sp-compose-input"
          value={draft}
          rows={1}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              void send();
            }
          }}
          placeholder={rec ? L(`给 ${nodeName(rec.st.agent, rec.hub)} 派个活…`, `A task for ${nodeName(rec.st.agent, rec.hub)}…`) : ""}
          aria-label={L("任务", "Task")}
        />
        <div className="sp-compose-bar">
          <div className="sp-compose-tools">
            <button type="button" className="sp-to" onClick={(e) => menu.openUnder(e.currentTarget, [
              { header: L("发给谁", "Send to") },
              ...agents.map((r) => ({
                label: nodeName(r.st.agent, r.hub),
                checked: r.st.agent.name === recipient,
                onSelect: () => pick(r.st.agent.name),
              })),
            ])} title={L("换一个收件人", "Change who gets it")}>
              <span className="sp-to-label">{L("发给", "To")}</span>
              {rec && <Avatar agent={rec.st.agent} size={16} badge={false} />}
              <span className="sp-to-name">{rec ? nodeName(rec.st.agent, rec.hub) : ""}</span>
              <ChevronDown />
            </button>
          </div>
          <button type="button" className="sp-send" onClick={() => void send()} disabled={!draft.trim() || busy} aria-label={L("发送", "Send")} title={L("发送", "Send")}>
            <ArrowUp />
          </button>
        </div>
      </div>
      <div className="sp-compose-hint">
        {hinted
          ? L("回车发送 · Shift+回车换行", "Enter to send · Shift+Enter for a new line")
          : L(`只有 ${rec ? nodeName(rec.st.agent, rec.hub) : "它"} 会收到这条，其他成员看得到但不会动手。`, `Only ${rec ? nodeName(rec.st.agent, rec.hub) : "it"} gets this; the others can see it but won't act on it.`)}
      </div>
      {menu.element}
    </div>
  );
}

// ---------------------------------------------------------------------------

/** What a member is doing now, in a line, and since when and for whom. */
function liveLine(r: AgentRow, nameOf: (n: string) => string, now: number): { text: string; meta?: string } {
  const a = r.st.agent;
  if (r.st.state === "offline") return { text: L("不在线", "Away"), meta: a.last_seen_at ? timeAgo(a.last_seen_at, now) : undefined };
  const open = (r.st.current || []).filter((m) => m.to === a.name && m.type === "task");
  const doing = open.find((m) => m.status === "acked" || m.status === "delivered") || (r.st.state === "working" ? open[0] : undefined);
  if (r.st.state === "working" || doing) {
    const text = a.status_text || (doing ? doing.body.replace(/\s+/g, " ").trim() : "") || L("在干活", "Working");
    const meta = doing ? [L(`${nameOf(doing.from)}派的`, `from ${nameOf(doing.from)}`), timeAgo(doing.created_at, now)].join(" · ") : undefined;
    const queued = r.st.queued > 0 ? L(` · 还有 ${r.st.queued} 个排队`, ` · ${r.st.queued} queued`) : "";
    return { text, meta: meta ? meta + queued : queued.replace(/^ · /, "") || undefined };
  }
  if (r.st.queued > 0) return { text: L(`${r.st.queued} 个活在排队`, `${r.st.queued} queued`) };
  return { text: a.description || stateWords(r.st.state) };
}

function MembersPanel({ agents, workspaces, loose, selected, coordinator, nameOf, now, onSetCoordinator, onChat, onProfile, onAddAgent, onAddWorkspace }: {
  agents: AgentRow[];
  workspaces: AgentRow[];
  loose: boolean;
  selected: string;
  coordinator?: string;
  nameOf: (n: string) => string;
  now: number;
  onSetCoordinator?: (name: string) => void;
  onChat: (r: AgentRow) => void;
  onProfile: (r: AgentRow) => void;
  onAddAgent: () => void;
  onAddWorkspace: () => void;
}) {
  const menu = useContextMenu();
  const row = (r: AgentRow) => {
    const a = r.st.agent;
    const ws = isWorkspace(a);
    return (
      <button
        key={a.name}
        type="button"
        className={cn("sp-member", selected === a.name && "active", r.st.state === "offline" && "is-away")}
        onClick={() => onChat(r)}
        onContextMenu={(e) => menu.open(e, [
          { label: ws ? L("打开终端", "Open terminal") : a.name === coordinator ? L("和协调者聊", "Talk to the coordinator") : L("看它在做什么", "See its work"), onSelect: () => onChat(r) },
          ...(!ws && onSetCoordinator && a.name !== coordinator ? [{ label: L("设为协调者", "Make it the coordinator"), onSelect: () => onSetCoordinator(a.name) }] : []),
          { label: L("资料", "Profile"), onSelect: () => onProfile(r) },
        ])}
        title={a.description || nodeName(a, r.hub)}
      >
        <Avatar agent={a} size={32} state={r.st.state} />
        <span className="sp-member-text">
          <span className="sp-member-name">
            <span className="sp-member-name-text">{nodeName(a, r.hub)}</span>
            {a.name === coordinator && <span className="sp-role">{L("协调者", "Coordinator")}</span>}
          </span>
          {ws ? <span className="sp-member-sub">{whereOf(r)}</span> : (() => {
            const line = liveLine(r, nameOf, now);
            return (
              <>
                <span className={cn("sp-member-sub", r.st.state === "working" && "is-working")}>{line.text}</span>
                {line.meta && <span className="sp-member-meta">{line.meta}</span>}
              </>
            );
          })()}
        </span>
      </button>
    );
  };
  return (
    <aside className="sp-side" aria-label={L("成员", "Members")}>
      <div className="sp-side-group">
        <div className="sp-side-title">
          <span><Users />{L(`成员 ${agents.length}`, `Members ${agents.length}`)}</span>
          <button type="button" className="sp-side-add" onClick={onAddAgent} title={L("添加 agent", "Add an agent")} aria-label={L("添加 agent", "Add an agent")}><Plus /></button>
        </div>
        {agents.map(row)}
        {agents.length === 0 && (
          <button type="button" className="sp-member is-add" onClick={onAddAgent}>
            <span className="sp-add-icon"><Plus /></span>
            <span className="sp-member-text"><span className="sp-member-name">{L("添加 agent", "Add an agent")}</span></span>
          </button>
        )}
      </div>
      <div className="sp-side-group">
        <div className="sp-side-title">
          <span>{L(`电脑 ${workspaces.length}`, `Computers ${workspaces.length}`)}</span>
          <button type="button" className="sp-side-add" onClick={onAddWorkspace} title={L("加一台电脑", "Add a computer")} aria-label={L("加一台电脑", "Add a computer")}><Plus /></button>
        </div>
        {workspaces.map(row)}
        {workspaces.length === 0 && (
          <p className="sp-side-note">
            {loose
              ? L("把你的电脑交给 agent 用：它们能在其中一个文件夹里跑命令。", "Let agents use one of your computers: they run commands in a folder on it.")
              : L("把你的一台电脑交给 agent 用：它们能在其中一个文件夹里跑命令。", "Let the agents use one of your computers: they run commands in a folder on it.")}
          </p>
        )}
      </div>
      {menu.element}
    </aside>
  );
}


export type { Message };
