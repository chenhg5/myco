import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowUp, ChevronDown, FileText, Plus, Users } from "lucide-react";
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

type View =
  | { kind: "group" }
  | { kind: "chat"; agent: string; peer?: string }
  | { kind: "thread"; threadId: string; agent: string }
  | { kind: "pair"; agent: string; peer: string };

const POLL_MS = 4000;

/**
 * A project as people see it: its members talking (a group view of every
 * task between them), and a box to give one of them a task. Every message
 * has one recipient: the group is where you watch and hand out work, not a
 * room where everyone hears everything. A member opens your one-to-one
 * chat with it; a workspace opens its terminal; a task opens its thread.
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

  const [view, setView] = useState<View>({ kind: "group" });
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
  const chatWith = async (agent: string) => {
    setView({ kind: "chat", agent, peer: await peerFor(agent) });
  };
  // A workspace opens its terminal: your exchange with it.
  const open = (r: AgentRow) => {
    const name = r.st.agent.name;
    if (!isWorkspace(r.st.agent)) { void chatWith(name); return; }
    void peerFor(name).then((peer) => { if (peer) setView({ kind: "pair", agent: name, peer }); });
  };
  const spaceKey = loose ? `${hubId}\u0000*` : `${hubId}\u0000${(space as { name: string }).name}`;
  useEffect(() => { setView({ kind: "group" }); }, [spaceKey, home]);
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
    if (v.kind === "chat") return { hubId, hubName, threadId: "", agent: v.agent, peer: v.peer, project: project?.name };
    if (v.kind === "thread") return { hubId, hubName, threadId: v.threadId, agent: v.agent };
    if (v.kind === "pair") return { hubId, hubName, threadId: "", agent: v.agent, peer: v.peer };
    return null;
  };
  const t = target(view);

  return (
    <div className="sp">
      <div className="sp-main">
        {t ? (
          <ConversationDetail
            key={JSON.stringify(t)}
            target={t}
            rows={rows}
            now={now}
            showBack
            backLabel={loose ? L("返回", "Back") : L(`返回「${project?.title || project?.name}」`, `Back to ${project?.title || project?.name}`)}
            onBack={() => setView({ kind: "group" })}
            onNewConversation={(_h, agent) => void chatWith(agent)}
            onOpen={(_h, agent, peer) => (peer ? setView({ kind: "pair", agent, peer }) : void chatWith(agent))}
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
              const agent = it.participants.find((p) => !p.startsWith("@") && memberNames.has(p)) || it.participants.find((p) => !p.startsWith("@")) || "";
              setView({ kind: "thread", threadId: it.thread_id, agent });
            }}
            onAddAgent={onAddAgent}
            onAbout={onAbout}
            onSent={onRefresh}
            strip={(
              // Narrow windows have no members column: the faces in a row.
              <div className="sp-strip">
                {[...agents, ...workspaces].map((r) => (
                  <button key={r.st.agent.name} type="button" className="sp-strip-face" title={nodeName(r.st.agent, r.hub)}
                    onClick={() => open(r)}>
                    <Avatar agent={r.st.agent} size={30} state={r.st.state} badge={false} />
                  </button>
                ))}
                <button type="button" className="sp-strip-face is-add" onClick={onAddAgent} aria-label={L("添加 agent", "Add an agent")} title={L("添加 agent", "Add an agent")}><Plus /></button>
              </div>
            )}
          />
        )}
      </div>

      <MembersPanel
        agents={agents}
        workspaces={workspaces}
        loose={loose}
        selected={view.kind === "chat" ? view.agent : view.kind === "pair" ? view.agent : ""}
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

function GroupView({ hubId, project, loose, title, agents, memberNames, rowOf, nameOf, now, onOpenThread, onAddAgent, onAbout, onSent, strip }: {
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
  onAbout?: () => void;
  onSent: () => void;
  strip?: React.ReactNode;
}) {
  // The space's tasks as last seen: shown at once, then refreshed.
  const itemsKey = `space\u0000${hubId}\u0000${project?.name || "*"}`;
  const [items, setItemsState] = useState<RecentItem[] | null>(() => recall<RecentItem[]>(itemsKey) ?? null);
  const setItems = (list: RecentItem[]) => setItemsState(remember(itemsKey, list));
  const [error, setError] = useState<unknown>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);

  const load = useCallback(async () => {
    try {
      const r = await appApi.recent(hubId, project ? { project: project.name } : {});
      let list = r.items;
      // Not in a project: the threads with these agents that are in none.
      if (!project) list = list.filter((it) => !it.project && [...it.participants, it.other || "", it.node || ""].some((p) => memberNames.has(p)));
      // Oldest first, as a chat reads: by when each task started.
      list = [...list].sort((a, b) => (a.first?.created_at || a.updated_at).localeCompare(b.first?.created_at || b.updated_at));
      setItems(list);
      setError(null);
    } catch (e) {
      setError(e);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hubId, project?.name, [...memberNames].join(",")]);

  useEffect(() => {
    setItemsState(recall<RecentItem[]>(itemsKey) ?? null);
    atBottom.current = true;
    void load();
    const t = window.setInterval(() => { if (document.visibilityState === "visible") void load(); }, POLL_MS);
    return () => window.clearInterval(t);
  }, [load]);

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
        <div className="sp-head-titles">
          <h2>{title}</h2>
          <p>
            {loose
              ? L("这些 agent 还没放进项目。建个项目把它们放进去，它们就能一起干活。", "These agents are not in a project yet. Put them in one so they can work together.")
              : project?.goal || L("给一个成员派活，看它们一起干。", "Give a member a task and watch them work together.")}
          </p>
        </div>
        {!loose && onAbout && (
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
              <EmptyProject loose={loose} onAddAgent={onAddAgent} />
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

      {agents.length > 0 && <Composer hubId={hubId} project={project} agents={agents} onSent={sent} />}
    </div>
  );
}

function EmptyProject({ loose, onAddAgent }: { loose: boolean; onAddAgent: () => void }) {
  return (
    <div className="sp-empty">
      <div className="sp-empty-art"><Mushroom idle /></div>
      <h3>{loose ? L("还没有 agent", "No agents yet") : L("项目建好了，拉几个 agent 进来吧", "Your project is ready. Bring in a few agents")}</h3>
      <p>{L("复制一句话发给你的 Claude Code、Codex 或任何 AI，它就会自己加入。可以一口气加好几个。", "Copy one sentence to your Claude Code, Codex or any AI, and it joins by itself. Add as many as you like.")}</p>
      <button type="button" className="app-btn app-btn-primary" onClick={onAddAgent}><Plus />{L("添加 agent", "Add an agent")}</button>
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
      <div className="sp-compose-box">
        <button type="button" className="sp-to" onClick={(e) => menu.openUnder(e.currentTarget, [
          { header: L("发给谁", "Send to") },
          ...agents.map((r) => ({
            label: nodeName(r.st.agent, r.hub),
            checked: r.st.agent.name === recipient,
            onSelect: () => pick(r.st.agent.name),
          })),
        ])} title={L("换一个收件人", "Change who gets it")}>
          <span className="sp-to-label">{L("发给", "To")}</span>
          {rec && <Avatar agent={rec.st.agent} size={18} badge={false} />}
          <span className="sp-to-name">{rec ? nodeName(rec.st.agent, rec.hub) : ""}</span>
          <ChevronDown />
        </button>
        <textarea
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
        <button type="button" className="sp-send" onClick={() => void send()} disabled={!draft.trim() || busy} aria-label={L("发送", "Send")} title={L("发送", "Send")}>
          <ArrowUp />
        </button>
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

function MembersPanel({ agents, workspaces, loose, selected, onChat, onProfile, onAddAgent, onAddWorkspace }: {
  agents: AgentRow[];
  workspaces: AgentRow[];
  loose: boolean;
  selected: string;
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
          { label: ws ? L("打开终端", "Open terminal") : L("单聊", "Chat"), onSelect: () => onChat(r) },
          { label: L("资料", "Profile"), onSelect: () => onProfile(r) },
        ])}
        title={a.description || nodeName(a, r.hub)}
      >
        <Avatar agent={a} size={32} state={r.st.state} />
        <span className="sp-member-text">
          <span className="sp-member-name">{nodeName(a, r.hub)}</span>
          <span className="sp-member-sub">{ws ? whereOf(r) : r.st.state === "working" ? stateWords("working", a.status_text) : a.description || stateWords(r.st.state)}</span>
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
