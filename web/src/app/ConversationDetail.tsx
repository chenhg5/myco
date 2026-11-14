import { Fragment, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { AlertTriangle, ArrowLeft, ArrowUp, Check, ChevronRight, Copy, Loader2, Lock, Square } from "lucide-react";
import { appApi, isTransient } from "@/appapi";
import type { Agent, Message } from "@/api";
import type { AgentRow } from "@/app/types";
import { canManage, isInbox, isMe, lastActivityAt, machineOfRow, personLabel, possiblyStuck, quietFor, runnerAway, sessionBusy, nodeName } from "@/app/nodes";
import { MessageContent } from "@/app/Markdown";
import { PromptSign, STOPPED, Terminal } from "@/app/Terminal";
import { TraceView } from "@/app/TraceView";
import { WorkingLine } from "@/app/Working";
import { TurnRail, type Turn } from "@/app/TurnRail";
import { summarize } from "@/app/summary";
import { ErrorText } from "@/app/dialogs";
import { recall, remember as rememberMemo } from "@/lib/memo";
import { useCopy } from "@/app/copy";
import { cn, timeAgo } from "@/lib/utils";
import { L, locale } from "@/lib/i18n";
import { Busy } from "@/app/Skeleton";
import { Avatar, whereOf } from "@/app/persona";
import { AttachButton, AttachList, dropFiles, pastedFiles, useAttachments } from "@/app/attach";
import { useAutoGrow } from "@/app/autogrow";
import { MessageAttachments } from "@/app/attachments";
import { VersionsContext, versionsOf } from "@/app/preview";
import { sentTexts, useInputHistory } from "@/app/inputHistory";
import { drafted, Reveal } from "@/app/Reveal";
import { DictateButton } from "@/app/Dictate";

/** Answers already grown in (once each, even if the view is drawn again),
 *  and when this App first had each message. */
const revealed = new Set<string>();
const firstSeen = new Map<string, number>();
import { chatDraftKey, draftOf, keepDraft } from "@/app/drafts";
import { chatQueueKey, keepQueue, queueOf, queuedId, type Queued } from "@/app/queued";
import { hubAtLeast } from "@/app/hubVersion";

export interface ConversationTarget {
  hubId: string;
  hubName: string;
  threadId: string;
  agent: string;
  // With peer: the whole history between agent and peer (all threads),
  // read-only, instead of one thread.
  peer?: string;
  /** A new conversation's first message goes into this project. */
  project?: string;
  /** Open at this message (older pages load until it is there), lit up,
   *  instead of at the bottom. */
  focus?: string;
  /** Your conversation with agent, and also agent's with these (read-only,
   *  the newest page of each): everything it was asked, by whom. */
  also?: string[];
  // Where Back goes: the node panel it was opened from.
  returnTo?: { page: string; node?: { hubId: string; name: string }; project?: { hubId: string; name: string } };
}

const CONSOLE_PREFIX = "@console:";
const POLL_MS = 3000;

function isConsole(from: string): boolean {
  return from.startsWith(CONSOLE_PREFIX);
}

function clockTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit", hour12: false });
}

function dayTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(locale(), { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false });
}

// A pause this long between two messages gets a divider with the time.
const GAP_MS = 30 * 60 * 1000;

// A task open longer than this is not being worked on any more.
const OPEN_FOR_MS = 12 * 3600 * 1000;

/** What a node reported while working on a task, folded to one line ("3 条
 * 进展") that opens to the list. */
function ProgressFold({ items }: { items: Message[] }) {
  const [open, setOpen] = useState(false);
  return (
    <div className={cn("conv-steps", open && "is-open")}>
      <button type="button" className="conv-steps-toggle" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        <ChevronRight className="conv-steps-chev" />
        {L(`${items.length} 条进展`, items.length === 1 ? "1 progress update" : `${items.length} progress updates`)}
      </button>
      {open && (
        <ul className="conv-steps-list">
          {items.map((p) => <li key={p.id} title={clockTime(p.created_at)}>{p.body}</li>)}
        </ul>
      )}
    </div>
  );
}

function isOpen(status: string): boolean {
  return status !== "done" && status !== "failed";
}

/** ⌘ on a Mac, Ctrl elsewhere (the same key the input listens for). */
function modKey(): string {
  const p = typeof navigator === "undefined" ? "" : navigator.platform || "";
  return /Mac|iPhone|iPad/.test(p) ? "⌘" : "Ctrl+";
}

// Conversations opened before: reopening one shows it at once while it
// refreshes (the shared page memory, saved across restarts in the app; see
// lib/memo). The hub stays the source of truth.
type SeenConversation = { messages: Message[]; more: boolean };
const cacheKey = (t: ConversationTarget) => ["conv", t.hubId, t.threadId || "", t.agent, t.peer || ""].join("\u0000");
function remember(key: string, messages: Message[], more: boolean) {
  rememberMemo<SeenConversation>(key, { messages, more });
}

// A full refresh every this many polls, whatever changed.
const FULL_EVERY = 20;

/** The newest change among messages, less 2 s (unix ms; 0 for none). */
function changedUntil(ms: Message[]): number {
  let t = 0;
  for (const m of ms) t = Math.max(t, new Date(m.updated_at || m.created_at).getTime() || 0);
  return t > 0 ? t - 2000 : 0;
}

/** Messages with changed ones put in (by id), in the order they were sent. */
function mergeById(ms: Message[], changed: Message[]): Message[] {
  const by = new Map(ms.map((m) => [m.id, m]));
  for (const m of changed) by.set(m.id, m);
  const at = (m: Message) => new Date(m.created_at).getTime();
  return [...by.values()].sort((a, b) => at(a) - at(b) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

// What an empty chat asks, one picked at random each time: about the
// project when this is its coordinator, else about the agent.
const ASK_PROJECT: [string, string][] = [
  ["你想在「{p}」里做成什么？", "What do you want done in {p}?"],
  ["「{p}」的下一步是什么？", "What's next for {p}?"],
  ["说说「{p}」接下来要做什么，{n} 会分给大家。", "Say what {p} needs next; {n} will hand it out."],
  ["今天想让「{p}」往前走哪一步？", "Which step should {p} take today?"],
];
const ASK_AGENT: [string, string][] = [
  ["想让 {n} 帮你做点什么？", "What should {n} do for you?"],
  ["有什么活，交给 {n} 吧。", "Got something? Hand it to {n}."],
  ["{n} 准备好了，交代第一件事吧。", "{n} is ready. What's the first thing?"],
  ["说说你想做成什么，{n} 来想办法。", "Say what you want done; {n} will work it out."],
  ["今天想和 {n} 一起做点什么？", "What shall you and {n} do today?"],
];

/** An empty chat: the agent's face, large, and a question to start with. */
function EmptyChat({ agent, name, project }: { agent?: Agent; name: string; project?: string }) {
  const [pick] = useState(() => Math.random());
  const list = project ? ASK_PROJECT : ASK_AGENT;
  const [zh, en] = list[Math.floor(pick * list.length) % list.length];
  const fill = (t: string) => t.replace("{n}", name).replace("{p}", project || "");
  return (
    <div className="conv-empty">
      {agent && <Avatar agent={agent} size={64} badge={false} />}
      <h3>{fill(L(zh, en))}</h3>
      <p>{L("一句话就行，它会自己想办法；在下面打字，回车发出去。", "A sentence is enough; it works out the rest. Type below and press Enter.")}</p>
    </div>
  );
}

/** While a conversation loads for the first time: its shape, not a word.
 *  Only after a moment: a load that is quick shows nothing in between,
 *  instead of a blink of placeholders. */
function LoadingSkeleton({ shell }: { shell: boolean }) {
  const [late, setLate] = useState(false);
  useEffect(() => {
    const t = window.setTimeout(() => setLate(true), 220);
    return () => window.clearTimeout(t);
  }, []);
  if (!late) return <div className="conv-skel-list" aria-busy="true" aria-label={L("正在加载", "Loading")} />;
  if (shell) {
    return (
      <div className="term conv-skel-term" aria-busy="true" aria-label={L("正在加载", "Loading")}>
        {[62, 38, 74].map((w, i) => (
          <div key={i} className="conv-skel-cmd">
            <span className="conv-skel conv-skel-line" style={{ width: "22%" }} />
            <span className="conv-skel conv-skel-line" style={{ width: `${w}%` }} />
          </div>
        ))}
      </div>
    );
  }
  return (
    <div className="conv-skel-list" aria-busy="true" aria-label={L("正在加载", "Loading")}>
      {[{ me: true, w: 46, h: 1 }, { me: false, w: 72, h: 3 }, { me: true, w: 34, h: 1 }, { me: false, w: 58, h: 2 }].map((b, i) => (
        <div key={i} className={cn("conv-skel-row", b.me && "is-me")}>
          <span className="conv-skel conv-skel-bubble" style={{ width: `${b.w}%`, height: 18 + b.h * 18 }} />
        </div>
      ))}
    </div>
  );
}

export function ConversationDetail({ target, rows, now, onBack, onNewConversation, onOpen, onRefresh, showBack = true, backLabel, role, inserts, headActions, projectTitle, bare }: {
  target: ConversationTarget;
  /** the project this chat is the home of (its coordinator's): the empty
   *  chat asks about it */
  projectTitle?: string;
  /** buttons at the right end of the header (e.g. full screen, close) */
  headActions?: React.ReactNode;
  /** no header: the page around it already says who this is (the
   *  project's home, with its coordinator) */
  bare?: boolean;
  /** what it is in the project (e.g. coordinator), before its description */
  role?: string;
  /** small things placed in the timeline by time (e.g. a task the agent
   *  handed out), between the messages; shown even before the first */
  inserts?: { id: string; at: string; node: React.ReactNode; waitsFor?: string }[];
  rows: AgentRow[];
  now: number;
  onBack: () => void;
  /** Next to the list (split view) there is nothing to go back to, unless
   *  the conversation was opened from a node. */
  showBack?: boolean;
  /** what the back button says (where it goes) */
  backLabel?: string;
  // Starts a new conversation of yours with the agent (from a read-only thread).
  onNewConversation: (hubId: string, agent: string) => void;
  /** Opens your conversation with another node (a copy just made). */
  onOpen?: (hubId: string, agent: string, peer: string) => void;
  /** The node's work changed outside this conversation (a command on its
   *  computer was stopped): refresh the rows it came from. */
  onRefresh?: () => void;
}) {
  // Seen before in this window: shown from the first frame (no empty
  // flash while the cache is read in an effect).
  const [messages, setMessages] = useState<Message[] | null>(() => recall<SeenConversation>(cacheKey(target))?.messages ?? null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [sendError, setSendError] = useState<unknown>(null);
  // What was typed and not sent stays with the conversation (drafts.ts).
  const draftKey = chatDraftKey(target.hubId, target.agent);
  const [draft, setDraft] = useState(() => draftOf(draftKey));
  const draftKeyRef = useRef(draftKey);
  draftKeyRef.current = draftKey;
  const writeDraft = (v: string) => {
    setDraft(v);
    keepDraft(draftKeyRef.current, v);
  };
  const [sending, setSending] = useState(false);
  // Files going with the next message (pasted, dropped or picked).
  const att = useAttachments(target.hubId);
  // Messages put after the running one, while the workspace cannot hold
  // them (older hubs): kept here, like drafts, and sent when the round ends.
  const queueKey = chatQueueKey(target.hubId, target.agent);
  const queueKeyRef = useRef(queueKey);
  queueKeyRef.current = queueKey;
  const [localQueue, setLocalQueue] = useState<Queued[]>(() => queueOf(queueKey));
  const writeQueue = (items: Queued[]) => {
    setLocalQueue(items);
    keepQueue(queueKeyRef.current, items);
  };
  // Which answers came while you were looking (they grow in, Reveal.tsx):
  // not the ones there when the conversation opened, nor ones loaded in its
  // first moments.
  const arrived = useRef<{ key: string; at: number }>({ key: "", at: 0 });
  /** Queued messages withdrawn here (a stopped one may still be polled back). */
  const [withdrawn, setWithdrawn] = useState<ReadonlySet<string>>(() => new Set());
  const [queueError, setQueueError] = useState<unknown>(null);
  const [queueBusy, setQueueBusy] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  // grow with the text, up to about eight lines, then scroll
  useAutoGrow(inputRef, draft, 200);
  const atBottom = useRef(true);
  // When the box below grows (a line, files), the conversation gets
  // shorter: if it was at the bottom, it stays there instead of the last
  // lines sliding under the box.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    let last = el.clientHeight;
    const ro = new ResizeObserver(() => {
      const h = el.clientHeight;
      if (h !== last && atBottom.current) el.scrollTop = el.scrollHeight;
      last = h;
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const focusDone = useRef(false);
  const focusTries = useRef(0);

  const row = rows.find((r) => r.hub.id === target.hubId && r.st.agent.name === target.agent);
  const agent = row?.st.agent;
  const agentName = agent && row ? nodeName(agent, row.hub) : target.agent;

  const exchange = !!target.peer;
  // Exchange view: older pages loaded so far (oldest first) and whether more exist.
  const [older, setOlder] = useState<Message[]>([]);
  const [more, setMore] = useState(() => recall<SeenConversation>(cacheKey(target))?.more ?? false);
  const [loadingOlder, setLoadingOlder] = useState(false);

  useEffect(() => {
    let cancelled = false;
    // Seen before in this window: show it at once, refresh behind it.
    const key = cacheKey(target);
    const hit = recall<SeenConversation>(key);
    setMessages(hit ? hit.messages : null);
    setLoadError(null);
    setSendError(null);
    setDraft(draftOf(chatDraftKey(target.hubId, target.agent)));
    setLocalQueue(queueOf(chatQueueKey(target.hubId, target.agent)));
    setQueueError(null);
    setFlushStalled(false);
    setWithdrawn(new Set());
    setOlder([]);
    setMore(hit?.more ?? false);
    // Opened at a message: stay there instead of following the bottom.
    atBottom.current = !target.focus;
    focusDone.current = false;
    focusTries.current = 0;
    // Polls ask only for what changed since the newest change they have
    // (hubs from v0.18.35; 2 s back, for changes in the same moment); every
    // FULL_EVERY-th poll, and on older hubs, the whole view again.
    // A new conversation (no thread, no peer yet): nothing to load until the
    // first message, which then opens the whole conversation.
    if (!target.peer && !target.threadId) {
      setMessages([]);
      return () => { cancelled = true; };
    }
    let current: Message[] = hit ? hit.messages : [];
    let more = hit?.more ?? false;
    let since = hit ? changedUntil(hit.messages) : 0;
    let polls = 0;
    let incremental = true;
    // A poll that brings nothing new changes nothing on screen (no new
    // array, no re-render, no scroll).
    const sameAs = (a: Message[], b: Message[]) => a.length === b.length
      && a.every((x, i) => x.id === b[i].id && x.status === b[i].status && x.updated_at === b[i].updated_at && x.body === b[i].body);
    // (Only once something is on screen: the first answer, even an empty
    // one, always replaces the loading state.)
    let onScreen = !!hit;
    const show = (ms: Message[], m: boolean) => {
      const same = onScreen && sameAs(current, ms) && more === m;
      onScreen = true;
      current = ms;
      more = m;
      since = changedUntil(ms);
      setLoadError(null);
      if (same) return;
      setMessages(ms);
      remember(key, ms, m);
    };
    const run = async (silent: boolean) => {
      polls++;
      const delta = silent && incremental && since > 0 && polls % FULL_EVERY !== 0;
      try {
        if (target.peer) {
          // The newest page, polled; older pages stay as loaded. With also,
          // the newest page of each of those is merged in once it comes:
          // yours shows first, the others do not hold it up.
          const extra = target.also?.length
            ? Promise.all(target.also.map((p) => appApi.exchanges(target.hubId, target.agent, p, undefined, delta ? since : undefined)
              .then((x) => x.messages).catch(() => [] as Message[])))
            : null;
          const mergeExtra = () => extra?.then((lists) => {
            const others = lists.flat();
            if (!cancelled && others.length) show(mergeById(current, others), more);
          });
          const r = await appApi.exchanges(target.hubId, target.agent, target.peer, undefined, delta ? since : undefined);
          if (cancelled) return;
          if (delta && r.since) {
            // Many changes at once: the whole page next time.
            if (r.more) polls = FULL_EVERY - 1;
            if (r.messages.length) show(mergeById(current, r.messages), more);
            void mergeExtra();
            return;
          }
          if (delta) incremental = false; // an older hub: whole pages
          // A whole page replaces what was shown, keeping what the others
          // said (merged in again just after).
          const mine = [...r.messages].reverse();
          const keep = target.also?.length ? current.filter((m) => target.also!.includes(m.from) || target.also!.includes(m.to)) : [];
          show(keep.length ? mergeById(mine, keep) : mine, silent ? more : r.more);
          void mergeExtra();
          if (!silent) setMore(r.more);
          return;
        }
        const r = await appApi.thread(target.hubId, target.threadId, delta ? since : undefined);
        if (cancelled) return;
        if (delta && r.since) {
          if (r.messages.length) show(mergeById(current, r.messages), false);
          return;
        }
        if (delta) incremental = false;
        show(r.messages, false);
      } catch (e) {
        if (!cancelled && !silent) setLoadError(e);
      }
    };
    void run(false);
    const poll = window.setInterval(() => {
      if (document.visibilityState === "visible") void run(true);
    }, POLL_MS);
    const onVisible = () => {
      if (document.visibilityState === "visible") void run(true);
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      cancelled = true;
      window.clearInterval(poll);
      document.removeEventListener("visibilitychange", onVisible);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target.hubId, target.threadId, target.agent, target.peer, (target.also || []).join(",")]);

  const loadOlder = async () => {
    const first = older[0] || messages?.[0];
    if (!target.peer || !first || loadingOlder) return;
    setLoadingOlder(true);
    const el = scrollRef.current;
    const fromBottom = el ? el.scrollHeight - el.scrollTop : 0;
    try {
      const r = await appApi.exchanges(target.hubId, target.agent, target.peer, new Date(first.created_at).getTime());
      setOlder((prev) => [...[...r.messages].reverse(), ...prev]);
      setMore(r.more);
      // Keep the view where it was.
      requestAnimationFrame(() => { if (el) el.scrollTop = el.scrollHeight - fromBottom; });
    } catch (e) {
      setLoadError(e);
    } finally {
      setLoadingOlder(false);
    }
  };

  // Before the frame is drawn: opening a chat never shows its top first
  // and then jumps to the newest. Later, only something new (another
  // message, a changed one) brings it down, and only if it was at the
  // bottom: a refresh with nothing new never moves what you are reading.
  const lastSeen = useRef("");
  useLayoutEffect(() => {
    const el = scrollRef.current;
    const last = messages?.[messages.length - 1];
    const sig = `${messages?.length ?? -1}\u0000${last?.id ?? ""}\u0000${last?.updated_at ?? ""}`;
    const changed = sig !== lastSeen.current;
    lastSeen.current = sig;
    if (el && changed && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [messages]);
  // What loads after the messages (a finished task's steps, a picture)
  // makes the conversation taller above the bottom: at the bottom, it
  // stays at the bottom instead of the last lines being pushed down.
  const timelineRef = useRef<HTMLDivElement>(null);
  // Each finished run's time, tokens and cost (from its trace), shown next
  // to its answer's time and copy instead of under the steps.
  const [usageOf, setUsageOf] = useState<Record<string, string>>({});
  const onUsage = (taskId: string, line: string) =>
    setUsageOf((u) => (u[taskId] === line ? u : { ...u, [taskId]: line }));
  // Runs still open that have stopped (their turn over, set aside, silent
  // after they spoke): not at work, so not the loading of the chat.
  const [pausedRuns, setPausedRuns] = useState<ReadonlySet<string>>(() => new Set());
  const onPaused = useCallback((taskId: string, paused: boolean) =>
    setPausedRuns((s) => {
      if (s.has(taskId) === paused) return s;
      const n = new Set(s);
      if (paused) n.add(taskId); else n.delete(taskId);
      return n;
    }), []);
  useEffect(() => {
    const el = timelineRef.current;
    const sc = scrollRef.current;
    if (!el || !sc || typeof ResizeObserver === "undefined") return;
    let h = el.offsetHeight;
    const ro = new ResizeObserver(() => {
      const nh = el.offsetHeight;
      if (nh !== h && atBottom.current) sc.scrollTop = sc.scrollHeight;
      h = nh;
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [messages === null]);

  const onScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  };

  // A message put after a task that is still open waits on the hub: it is
  // not part of the conversation yet (hubs from v0.19.2 hold it; older hubs
  // never see one). It is shown above the box until that task ends.
  const known = new Map((messages || []).map((m) => [m.id, m]));
  const afterOpen = (m: Message): boolean => {
    if (!m.after || !isOpen(m.status)) return false;
    const task = known.get(m.after);
    return !!task && task.type === "task" && isOpen(task.status);
  };
  const hiddenMessage = (m: Message): boolean => withdrawn.has(m.id) || afterOpen(m);
  const hubQueue = (messages || []).filter((m) => afterOpen(m) && !withdrawn.has(m.id));

  // Everything shown: in the exchange view, older pages before the newest.
  const shown: Message[] | null = messages === null ? null : (exchange
    ? [...older.filter((o) => !messages.some((m) => m.id === o.id)), ...messages]
    : messages).filter((m) => !hiddenMessage(m));

  const consoleTasks = (messages || []).filter((m) => m.type === "task" && isMe(m.from, row?.hub) && !hiddenMessage(m));
  const lastTask = consoleTasks[consoleTasks.length - 1];
  // An answer names its task (reply_to); one that does not (older runners)
  // counts when it came after the task in the same thread. Replies to other
  // tasks still open here (say, one just resent) do not close the newest.
  const answered = !!lastTask && (messages || []).some((m) =>
    (m.type === "reply" || m.type === "event") &&
    m.id !== lastTask.id &&
    (m.reply_to
      ? m.reply_to === lastTask.id
      : m.thread_id === lastTask.thread_id && new Date(m.created_at).getTime() >= new Date(lastTask.created_at).getTime()),
  );
  const openTask = lastTask && isOpen(lastTask.status) && !answered ? lastTask : null;

  // Only an event after the newest task says something about this task; an
  // old failure should not paint a later, healthy conversation red.
  const afterTask = lastTask
    ? (messages || []).filter((m) => new Date(m.created_at).getTime() >= new Date(lastTask.created_at).getTime())
    : [];
  const stopped = lastTask?.status === "failed" && lastTask.error === STOPPED;
  const failed = lastTask?.status === "failed" || afterTask.some((m) => m.type === "event");
  const busy = !!row && sessionBusy(row.st);
  const offline = row?.st.state === "offline";
  // A task picked up long ago and quiet since: its result may never come
  // back. Said in the chat, with a way out, instead of a spinner.
  const isStuck = (m: Message): boolean =>
    possiblyStuck(m, { agent: row?.st.agent, receiving: row?.st.receiving, offline: row?.st.state === "offline" }, now);
  const stuckLine = (m: Message): string => {
    const quiet = quietFor(now, lastActivityAt(m));
    return L(
      `${quiet}没有动静，可能卡住了（结果可能没送回来）`,
      `No activity for ${quiet}; it may be stuck (the result may not have made it back)`,
    );
  };
  // Every task of yours still open in this conversation, not only the newest:
  // an older one that went quiet may be stuck too. (One waiting behind the
  // running task is not in the conversation yet.)
  const openTasks = consoleTasks.filter((m) => isOpen(m.status));
  const allStuck = openTasks.length > 0 && openTasks.every(isStuck);
  // The header's 可能卡住 takes you to the (first) stuck task: in a long
  // conversation it can be far above the bottom you are looking at.
  const firstStuck = openTasks.find(isStuck);
  const toStuck = () => {
    const el = firstStuck && scrollRef.current?.querySelector<HTMLElement>(`[data-stuck="${firstStuck.id}"]`);
    if (!el) return;
    // Taken up there on purpose: what grows at the bottom must not pull
    // the view back down.
    atBottom.current = false;
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    el.classList.remove("is-flash");
    void el.offsetWidth; // restart the highlight
    el.classList.add("is-flash");
  };
  const state = busy
    ? { dot: "pending", label: L("会话被占用", "Session in use") }
    : allStuck
    ? { dot: "pending", label: L("可能卡住", "May be stuck") }
    : openTask?.asking
    ? { dot: "pending", label: L("等你决定", "Needs your decision") }
    : openTask && offline
    ? { dot: "idle", label: L("离线 · 等它上线", "Offline · waiting") }
    : openTask && openTask.status !== "acked"
    ? { dot: "pending", label: L("等待中", "Waiting") }
    : openTask
    ? { dot: "working", label: L("处理中", "Working") }
    // At work on something someone else gave it.
    : row?.st.state === "working" && (!exchange || !!target.peer && (isMe(target.peer, row?.hub) || isMe(target.agent, row?.hub)))
    ? { dot: "working", label: target.also?.length ? L("在干活", "Working") : L("在忙别的", "Busy with something else") }
    : stopped
      ? { dot: "idle", label: L("已停止", "Stopped") }
    : failed
      ? { dot: "failed", label: L("失败", "Failed") }
      : lastTask
        ? { dot: "idle", label: L("已完成", "Done") }
        : { dot: "idle", label: L("空闲", "Idle") };
  // An older hub cannot hold a queued message: the App sends it once the
  // round it waits for has ended (all of them, in order).
  const [flushStalled, setFlushStalled] = useState(false);
  const flushing = useRef(false);
  useEffect(() => {
    if (openTask || flushStalled || flushing.current || localQueue.length === 0) return;
    flushing.current = true;
    const items = localQueue;
    // The box may move to another conversation while this runs: its queue is
    // this one's, and its messages belong in this one's flow.
    const flushKey = chatQueueKey(target.hubId, target.agent);
    const here = () => queueKeyRef.current === flushKey;
    (async () => {
      let done = 0;
      try {
        for (; done < items.length; done++) {
          const it = items[done];
          const send = (thread_id?: string) => appApi.sendMessage(target.hubId, {
            to: target.agent, body: it.body, thread_id, attachments: it.attachments,
          });
          let m: Message;
          try {
            m = await send(it.threadId);
          } catch (e) {
            // The thread was closed or renamed under us: the message still goes.
            if (!it.threadId || (e as { code?: string }).code !== "not_found") throw e;
            m = await send(undefined);
          }
          if (here()) setMessages((prev) => (prev ? [...prev, m] : [m]));
        }
        keepQueue(flushKey, []);
        if (here()) setLocalQueue([]);
      } catch (e) {
        keepQueue(flushKey, items.slice(done));
        if (here()) {
          setQueueError(e);
          setFlushStalled(true);
          setLocalQueue(items.slice(done));
        }
      } finally {
        flushing.current = false;
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openTask?.id, localQueue, flushStalled, target.hubId, target.agent]);
  // The runner found the agent's session open elsewhere (a terminal, an
  // editor, the Codex app): it waits and retries rather than write into it.
  // Why a task has not started: said in the chat instead of a spinner.
  const waitingReason = busy
    ? L("对方的会话正开在别处（终端、编辑器或桌面版），空出来后会处理", "The session is open elsewhere (a terminal, an editor or a desktop app); it starts when that is free")
    : offline
      ? agent && isInbox(agent)
        ? L("对方只在会话里接收消息，等它的会话再挂上 inbox wait 就会处理", "The node receives only in its session; it starts when its session runs inbox wait again")
        : L("对方离线，上线后会处理", "The node is offline; it starts when the node is back")
      : row && runnerAway(row.st, row.hub)
        ? L("它那台电脑上的 myco 没在运行（可能在重启），恢复后会接着做", "The node's runner is not running (maybe restarting); it starts when the runner is back")
        : undefined;

  // Busy elsewhere: a copy of the session (a new node on the same machine
  // and folder) can take the work now; the original stays as it is.
  const machine = row ? machineOfRow(row) : undefined;
  // An inbox node has no CLI session to copy and no runner to start it.
  const forkable = !!agent && !isInbox(agent) && (agent.binding.harness === "claude" || agent.binding.harness === "codex") &&
    !!agent.binding.session_id && !!agent.binding.workdir && !!machine?.can_spawn && canManage(row?.hub, machine?.owner_id);
  const [copying, setCopying] = useState(false);
  const [copyError, setCopyError] = useState<unknown>(null);
  const copyAndContinue = async () => {
    if (!agent || !machine || !row) return;
    setCopying(true);
    setCopyError(null);
    try {
      const names = new Set((row.hub.overview?.agents || []).map((x) => x.agent.name));
      let n = 2;
      while (names.has(`${agent.name}-${n}`)) n++;
      const r = await appApi.spawn(target.hubId, machine.id, {
        cli: agent.binding.harness as "claude" | "codex", dir: agent.binding.workdir || "", fork_from: agent.binding.session_id,
        name: `${agent.name}-${n}`, description: agent.description || undefined,
        display_name: `${nodeName(agent, row.hub)} ${L("副本", "copy")}`,
        ...(agent.runtime?.access ? { access: agent.runtime.access } : {}),
      });
      const copy = r.agent || `${agent.name}-${n}`;
      // The waiting task moves to the copy.
      if (openTask) {
        await appApi.stop(target.hubId, openTask.id).catch(() => {});
        await appApi.sendMessage(target.hubId, { to: copy, body: openTask.body });
      }
      const me = row.hub.me?.member ? CONSOLE_PREFIX + row.hub.me.member.name : target.peer || "";
      onOpen?.(target.hubId, copy, target.peer || me);
    } catch (e) {
      setCopyError(e);
    } finally {
      setCopying(false);
    }
  };

  // A thread you take no part in (two nodes working together) is only
  // shown: writing into it would land in their conversation, not yours.
  const observed = exchange
    ? !isMe(target.agent, row?.hub) && !isMe(target.peer || "", row?.hub)
    : !!messages && messages.length > 0 && !messages.some((m) => isMe(m.from, row?.hub) || isMe(m.to, row?.hub));
  const parties = exchange
    ? [target.agent, target.peer!]
    : observed && messages
      ? [...new Set(messages.flatMap((m) => [m.from, m.to]))].filter((n) => n && !n.startsWith("@"))
      : [];

  // With an exec (shell) node on one side, the messages read as a terminal.
  const isExecNode = (n?: string) => !!n && rows.some((x) => x.hub.id === target.hubId && x.st.agent.name === n && x.st.agent.binding.harness === "exec");
  const execNode = [target.agent, target.peer, ...parties].find((n) => isExecNode(n));
  // Your own session with a shell node: you type at a prompt, as in a terminal.
  const shellMode = !observed && isExecNode(target.agent);
  // Enter adds to the running task; ⌘/Ctrl+Enter puts the next one behind
  // it. A hub from v0.19.2 holds both (a steer may carry files, a queued
  // message knows what it waits for); older hubs get the text as a steer,
  // and the queue is kept in the App until the round ends.
  const queueOnHub = hubAtLeast(row?.hub.hub_version, "0.19.2");
  const supplementing = !shellMode && !!openTask;
  const mod = modKey();
  // Either way (typing at it, or reading what other nodes ran on it) the
  // pane is a terminal.
  const termMode = shellMode || !!execNode;

  // Commands sent here, for ↑ / ↓ (newest last); -1 = not browsing.
  const history = consoleTasks.map((m) => m.body).filter((b) => b && !b.includes("\n"));
  const histPos = useRef(-1);
  const histDraft = useRef("");
  // In a chat, ↑ / ↓ from an empty box: what you sent here (tasks and
  // what you added to them).
  const chatHistory = useInputHistory(
    sentTexts((messages || []).filter((m) => (m.type === "task" || m.type === "steer") && isMe(m.from, row?.hub) && !hiddenMessage(m) && !m.unreadable).map((m) => m.body)),
    draft, (v) => writeDraft(v));

  const alsoSet = new Set(target.also || []);
  const nameOf = (from: string): string => {
    if (isConsole(from)) return personLabel(from, row?.hub);
    const r = rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === from);
    return r ? nodeName(r.st.agent, r.hub) : from;
  };

  // The rail at the left: each turn, with how the answer starts. In your own
  // conversation a turn is something you said. Watching two nodes, it is a
  // task either of them gave the other, and the preview says who gave it.
  // A terminal has no rail: it is read from the bottom, like a terminal.
  const turns: Turn[] = [];
  for (const m of shown || []) {
    // A supplement belongs to the task it names, not a turn of its own.
    if (m.type === "progress" || m.type === "event" || m.type === "steer") continue;
    // With others' work merged in, a task one of them gave it starts a
    // turn too (a member's pane is mostly the coordinator's tasks).
    const fromOther = !observed && alsoSet.has(m.from) && m.type === "task" && m.to === target.agent;
    const starts = observed ? m.type === "task" : isMe(m.from, row?.hub) || fromOther;
    if (starts) turns.push({ id: m.id, who: observed || fromOther ? nameOf(m.from) : undefined, question: summarize(m), answer: "" });
    else if (turns.length && !turns[turns.length - 1].answer) turns[turns.length - 1].answer = summarize(m);
  }
  const turnIds = new Set(turns.map((t) => t.id));
  const hasRail = !shellMode && !execNode && turns.length >= 2;

  // Someone else's task that its node is still on (two nodes working
  // together): the foot of the conversation shows it at work. Your own
  // tasks show their run as it goes (TraceView) instead.
  const rowOf = (name: string) => rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === name);

  // What each task got, by the task's id. Answers and progress updates
  // point at their task (reply_to). One without it (older runners) answers
  // the earliest task its sender has not answered in that thread; progress
  // without it belongs to the latest task its sender had there.
  const answerOf = new Map<string, Message>();
  const taskOfAnswer = new Map<string, string>();
  const progressOf = new Map<string, Message[]>();
  const taskOfProgress = new Map<string, string>();
  if (!termMode && shown) {
    const lastTaskTo = new Map<string, string>();
    const unanswered = new Map<string, string[]>();
    for (const m of shown) {
      const key = (to: string) => `${m.thread_id}\u0000${to}`;
      if (m.type === "task") {
        lastTaskTo.set(key(m.to), m.id);
        unanswered.set(key(m.to), [...(unanswered.get(key(m.to)) || []), m.id]);
      }
      if (m.type === "reply" || m.type === "event") {
        const waiting = unanswered.get(key(m.from)) || [];
        const task = m.reply_to || waiting[0];
        if (task && !answerOf.has(task)) {
          answerOf.set(task, m);
          taskOfAnswer.set(m.id, task);
          unanswered.set(key(m.from), waiting.filter((id) => id !== task));
        }
      }
      if (m.type === "progress") {
        const task = m.reply_to || lastTaskTo.get(`${m.thread_id}\u0000${m.from}`);
        if (task) {
          progressOf.set(task, [...(progressOf.get(task) || []), m]);
          taskOfProgress.set(m.id, task);
        }
      }
    }
  }

  // Someone else's tasks that are not answered yet (two nodes working
  // together): the foot of the conversation shows the node at work on the
  // first, and how many wait behind it. Your own tasks show their run as it
  // goes (TraceView) instead. A task left open for half a day, or one for a
  // node that is offline, is not being worked on.
  const working = (() => {
    if (termMode || !shown) return null;
    // (one that has gone quiet may be stuck: the lists say so; no mushroom)
    const open = shown.filter((m) => {
      if (m.type !== "task" || isMe(m.from, row?.hub) || !isOpen(m.status) || answerOf.has(m.id)) return false;
      // A task someone else gave the member shown here has its own line of
      // work under it (with the mushroom): not a second one at the bottom.
      if (!observed && alsoSet.has(m.from) && m.to === target.agent) return false;
      const w = rowOf(m.to);
      return Date.now() - new Date(m.created_at).getTime() < OPEN_FOR_MS && w?.st.state !== "offline"
        && !possiblyStuck(m, { agent: w?.st.agent, receiving: w?.st.receiving, offline: false }, now);
    });
    if (!open.length) return null;
    // Only an acknowledged task is at work: queued, pulled-but-not-acked
    // (delivered) and held all wait.
    const task = open.find((m) => m.status === "acked") || open[0];
    const said = progressOf.get(task.id);
    return {
      task,
      queued: task.status !== "acked",
      behind: open.filter((m) => m !== task && m.to === task.to).length,
      // what it last said about this task, else what it says it is doing
      status: said?.length ? said[said.length - 1].body.split("\n")[0] : rowOf(task.to)?.st.agent.status_text,
    };
  })();

  // One loading in the chat, the newest: of the runs at work (acknowledged,
  // or reporting progress), only the newest one moves; an older one still
  // open says quietly that it is not finished. Someone else's task at the
  // foot counts too.
  const liveRun = (() => {
    if (termMode || !shown) return null;
    let newest: Message | null = null;
    for (const m of shown) {
      if (m.type !== "task" || m.e2e || !isOpen(m.status) || answerOf.has(m.id) || m.asking || pausedRuns.has(m.id)) continue;
      if (m.status !== "acked" && m.status !== "held" && !progressOf.get(m.id)?.length) continue;
      const ours = isMe(m.from, row?.hub) && !isStuck(m);
      const theirs = !observed && alsoSet.has(m.from) && m.to === target.agent;
      if (!ours && !theirs) continue;
      if (!newest || new Date(m.created_at).getTime() >= new Date(newest.created_at).getTime()) newest = m;
    }
    return newest;
  })();
  const footLive = !!working && !working.queued
    && (!liveRun || new Date(working.task.created_at).getTime() > new Date(liveRun.created_at).getTime());
  const liveId = footLive ? "" : liveRun?.id || "";
  // Messages under the run at work (tasks it handed out, your supplements):
  // its loading goes to the foot of the chat, under them, not above.
  const liveIndex = liveId && shown ? shown.findIndex((m) => m.id === liveId) : -1;
  const liveFoot = liveIndex >= 0 && (shown!.slice(liveIndex + 1).some((m) =>
    m.type !== "progress" && !(m.reply_to === liveId && (m.type === "reply" || m.type === "event")))
    || (inserts || []).some((x) => x.at > shown![liveIndex].created_at));
  // What each run of yours waits for: the tasks the node handed out after
  // it began (before your next one) that are still open.
  const waitingOn = new Map<string, string[]>();
  if (!termMode && !observed && messages) {
    // (the whole thread: a task it handed out is not always in this chat)
    let current: Message | null = null;
    for (const m of [...messages].sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())) {
      if (m.type !== "task") continue;
      if (m.to === target.agent && (isMe(m.from, row?.hub) || alsoSet.has(m.from))) { current = m; continue; }
      if (current && m.from === target.agent && isOpen(m.status) && !answerOf.has(m.id)) {
        const who = nameOf(m.to);
        const list = waitingOn.get(current.id) || [];
        if (!list.includes(who)) waitingOn.set(current.id, [...list, who]);
      }
    }
    // Work it handed out shown here as cards (a project's home): the run of
    // yours it came during waits for it too.
    const mineByTime = messages.filter((m) => m.type === "task" && m.to === target.agent && isMe(m.from, row?.hub))
      .sort((a, b) => b.created_at.localeCompare(a.created_at));
    for (const x of inserts || []) {
      if (!x.waitsFor) continue;
      const during = mineByTime.find((m) => m.created_at <= x.at);
      if (!during) continue;
      const list = waitingOn.get(during.id) || [];
      if (!list.includes(x.waitsFor)) waitingOn.set(during.id, [...list, x.waitsFor]);
    }
  }

  // Jump to a message (from a quote of it) and light it up for a moment.
  const jumpTo = (id: string, smooth = true) => {
    const el = scrollRef.current?.querySelector<HTMLElement>(`[data-msg="${CSS.escape(id)}"]`);
    if (!el) return false;
    atBottom.current = false;
    el.scrollIntoView({ block: "center", behavior: smooth ? "smooth" : "auto" });
    el.classList.remove("is-flash");
    void el.offsetWidth;
    el.classList.add("is-flash");
    window.setTimeout(() => el.classList.remove("is-flash"), 1600);
    return true;
  };

  // Opened at a message (a task in the project's group): once it is shown,
  // bring it to the middle; in a long exchange, load older pages until it
  // turns up (a few at most).
  useEffect(() => {
    if (!target.focus || focusDone.current || shown === null) return;
    const id = target.focus;
    const t = requestAnimationFrame(() => {
      if (focusDone.current) return;
      if (jumpTo(id, false)) { focusDone.current = true; return; }
      if (exchange && more && !loadingOlder && focusTries.current < 10) {
        focusTries.current++;
        void loadOlder();
      } else if (!more || focusTries.current >= 10) {
        // Not found: show the newest, as usual.
        focusDone.current = true;
        atBottom.current = true;
        const el = scrollRef.current;
        if (el) el.scrollTop = el.scrollHeight;
      }
    });
    return () => cancelAnimationFrame(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shown?.length, more, loadingOlder, target.focus]);

  // The thread a message continues in the exchange view: the latest one you
  // take part in under your current name (older tasks were signed with the
  // device, and the hub only lets a thread's participants write into it).
  const continuedThread = (): string | undefined => {
    const me = row?.hub.me?.member ? CONSOLE_PREFIX + row.hub.me.member.name : null;
    for (let i = (shown?.length || 0) - 1; i >= 0; i--) {
      const m = shown![i];
      if (!me || m.from === me || m.to === me) return m.thread_id;
    }
    return undefined;
  };

  const projectStarted = useRef(false);
  // "clear" in a shell session is the terminal's own business, not a command
  // for the node (which has no screen to clear): everything so far goes off
  // the screen, and comes back with the line that then heads the terminal.
  // It is remembered on this computer, across restarts of the App, by the
  // time of the last message cleared (the hub's time, as every message
  // carries it, not this clock's).
  const clearKey = `myco-term-clear:${target.hubId}/${target.agent}`;
  const [clearedAfter, setClearedAfter] = useState<string | null>(() => {
    try { return localStorage.getItem(clearKey); } catch { return null; }
  });
  const setCleared = (id: string | null) => {
    setClearedAfter(id);
    try { if (id) localStorage.setItem(clearKey, id); else localStorage.removeItem(clearKey); } catch { /* storage unavailable */ }
  };
  const clearScreen = () => {
    // The latest by time, which is not always the last in the list.
    const latest = (shown || []).reduce<Message | null>((x, m) => (!x || new Date(m.created_at) > new Date(x.created_at) ? m : x), null);
    if (latest) setCleared(latest.created_at);
    writeDraft("");
    histPos.current = -1;
  };
  const clearedTime = clearedAfter ? new Date(clearedAfter).getTime() : NaN;
  const onScreen = shown && !Number.isNaN(clearedTime) ? shown.filter((m) => new Date(m.created_at).getTime() > clearedTime) : shown;
  const cleared = !!shown && !!onScreen && onScreen.length < shown.length;

  const send = async (mode: "now" | "next" = "now") => {
    const typed = draft.trim();
    if (shellMode && (typed === "clear" || typed === "cls")) { clearScreen(); return; }
    if ((!typed && !att.any) || att.busy || sending || busy) return;
    const body = att.body(typed);
    const attachments = att.ids.length ? att.ids : undefined;
    // While this round runs: Enter adds to it, ⌘/Ctrl+Enter puts the next
    // one behind it. With nothing running, both send a message.
    const supplement = mode === "now" && supplementing && !!openTask;
    const queuedNext = mode === "next" && supplementing && !!openTask;
    setSending(true);
    setSendError(null);
    setQueueError(null);
    try {
      if (!target.peer && !target.threadId) {
        // The first message of a new conversation: then the whole one.
        const first = await appApi.sendMessage(target.hubId, { to: target.agent, body, attachments, ...(target.project ? { project: target.project } : {}) });
        writeDraft("");
        att.clear();
        if (onOpen) onOpen(target.hubId, target.agent, first.from);
        else setMessages([first]);
        return;
      }
      // Opened from a project: the first message starts a thread in it.
      const inProject = !!target.project && !projectStarted.current;
      const threadId = inProject ? undefined : exchange ? continuedThread() : target.threadId;
      const sendNew = (thread_id?: string, after?: string) => appApi.sendMessage(target.hubId, {
        to: target.agent, body, thread_id, attachments,
        ...(inProject && !thread_id ? { project: target.project } : {}),
        ...(after ? { after } : {}),
      });
      const sendThread = (thread_id?: string) => sendNew(thread_id).catch((e) => {
        // The hub does not count you in that thread (sent under an older
        // name): start a new one; the exchange view shows every thread.
        if (exchange && thread_id && (e as { code?: string }).code === "not_found") return sendNew(undefined);
        throw e;
      });
      if (inProject) projectStarted.current = true;
      let m: Message;
      if (supplement) {
        // An older hub has nowhere to put files with a correction: they go
        // as a new message, as before.
        // The round may have ended just now: then this is a new message.
        m = attachments && !queueOnHub
          ? await sendThread(threadId)
          : await appApi.steer(target.hubId, openTask!.id, body, attachments).catch((e) => {
            if ((e as { code?: string }).code === "invalid_state") return sendThread(threadId);
            throw e;
          });
      } else if (queuedNext) {
        if (queueOnHub) {
          // Behind the last one already waiting: they go one after another.
          m = await sendNew(threadId, hubQueue[hubQueue.length - 1]?.id || openTask!.id);
        } else {
          // This hub cannot hold it: keep it here and send it when the
          // round ends (queued.ts).
          writeQueue([...localQueue, { id: queuedId(), body, attachments, threadId, at: Date.now() }]);
          setFlushStalled(false);
          writeDraft("");
          att.clear();
          histPos.current = -1;
          chatHistory.reset();
          inputRef.current?.focus();
          return;
        }
      } else {
        m = await sendThread(threadId);
      }
      // The hub signs your messages with your name there: when this view
      // assumed another, follow the one it used.
      if (exchange && m.from && target.peer && m.from !== target.peer && m.type !== "steer" && onOpen) {
        writeDraft("");
        att.clear();
        onOpen(target.hubId, target.agent, m.from);
        return;
      }
      setMessages((prev) => (prev ? [...prev, m] : [m]));
      writeDraft("");
      att.clear();
      histPos.current = -1;
      chatHistory.reset();
      atBottom.current = true;
      inputRef.current?.focus();
    } catch (e) {
      setSendError(e);
    } finally {
      setSending(false);
    }
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (!shellMode && chatHistory.onKeyDown(e)) return;
    if (shellMode && (e.key === "ArrowUp" || e.key === "ArrowDown") && !e.nativeEvent.isComposing && !draft.includes("\n") && history.length > 0) {
      e.preventDefault();
      let pos = histPos.current;
      if (e.key === "ArrowUp") {
        if (pos === -1) { histDraft.current = draft; pos = history.length - 1; } else if (pos > 0) pos -= 1;
      } else {
        if (pos === -1) return;
        pos = pos + 1 < history.length ? pos + 1 : -1;
      }
      histPos.current = pos;
      writeDraft(pos === -1 ? histDraft.current : history[pos]);
      return;
    }
    // Ctrl+L clears the screen, as in a terminal.
    if (shellMode && e.ctrlKey && e.key === "l") {
      e.preventDefault();
      clearScreen();
      return;
    }
    // Ctrl+C with nothing typed stops the running command, as in a terminal.
    if (shellMode && e.ctrlKey && e.key === "c" && !draft && openTask) {
      e.preventDefault();
      void stop(openTask);
      return;
    }
    if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
    e.preventDefault();
    // Enter adds to the running task; ⌘/Ctrl+Enter puts the next one behind
    // it. In a terminal both just send the command, as before.
    if (!shellMode && openTask && (e.metaKey || e.ctrlKey)) void send("next");
    else void send("now");
  };

  // Stop a running task you sent: the hub closes it, its runner ends the run.
  const [stopError, setStopError] = useState<unknown>(null);
  const stop = async (task: Message) => {
    setStopError(null);
    try {
      const m = await appApi.stop(target.hubId, task.id);
      setMessages((prev) => prev && prev.map((x) => (x.id === m.id ? m : x)));
    } catch (e) {
      setStopError(e);
    }
  };

  // Commands other nodes sent to the computer in view. The terminal shows
  // only your own conversation with it, so these have no row there: the
  // overview knows what it is on, and which of it may have gone quiet.
  const shellRow = termMode
    ? rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === (shellMode ? target.agent : execNode))
    : undefined;
  const [stoppedShell, setStoppedShell] = useState<ReadonlySet<string>>(() => new Set());
  const [moreStuck, setMoreStuck] = useState(false);
  const [stuckError, setStuckError] = useState<unknown>(null);
  const [stoppingStuck, setStoppingStuck] = useState(false);
  const shellStuck = (shellRow?.st.current || []).filter((m) =>
    m.type === "task" && m.to === shellRow?.st.agent.name && !isMe(m.from, shellRow?.hub) && !stoppedShell.has(m.id)
    && possiblyStuck(m, { agent: shellRow?.st.agent, receiving: shellRow?.st.receiving, offline: shellRow?.st.state === "offline" }, now));
  const canStopShell = canManage(shellRow?.hub, shellRow?.st.agent.owner_id);
  const shellFrom = [...new Set(shellStuck.map((m) => nameOf(m.from)))];
  const stoppedShellNow = (ids: string[]) => setStoppedShell((prev) => new Set([...prev, ...ids]));
  const stopOnShell = async (m: Message) => {
    setStuckError(null);
    try {
      const stopped = await appApi.stop(target.hubId, m.id);
      stoppedShellNow([m.id]);
      setMessages((prev) => prev && prev.map((x) => (x.id === stopped.id ? stopped : x)));
      onRefresh?.();
    } catch (e) {
      setStuckError(e);
    }
  };
  const stopAllOnShell = async () => {
    if (stoppingStuck) return;
    setStuckError(null);
    setStoppingStuck(true);
    try {
      const all = shellStuck;
      const results = await Promise.allSettled(all.map((m) => appApi.stop(target.hubId, m.id)));
      stoppedShellNow(all.filter((_, i) => results[i].status === "fulfilled").map((m) => m.id));
      // Some may be already closed or out of this app's reach: say why.
      const failed = results.find((r): r is PromiseRejectedResult => r.status === "rejected");
      if (failed) setStuckError(failed.reason);
      onRefresh?.();
    } finally {
      setStoppingStuck(false);
    }
  };

  // A task that may have lost its result: stop it, then send the same message
  // to the same node again in the same conversation, as a fresh task.
  const [resending, setResending] = useState<string | null>(null);
  const resend = async (task: Message) => {
    if (resending) return;
    setStopError(null);
    setResending(task.id);
    try {
      const stopped = await appApi.stop(target.hubId, task.id);
      const fresh = await appApi.sendMessage(target.hubId, { to: task.to, body: task.body, thread_id: task.thread_id });
      setMessages((prev) => {
        const next = prev ? prev.map((x) => (x.id === stopped.id ? stopped : x)) : [stopped];
        return [...next, fresh];
      });
      atBottom.current = true;
    } catch (e) {
      setStopError(e);
    } finally {
      setResending(null);
    }
  };

  // Messages waiting behind this round, in order: on the hub (v0.19.2) or
  // here. Both are one list above the box, with edit and withdraw.
  type QueueItem = { key: string; body: string; server?: Message; local?: Queued };
  const queueRows: QueueItem[] = [
    ...localQueue.map((q) => ({ key: q.id, body: q.body, local: q })),
    ...hubQueue.map((m) => ({ key: m.id, body: m.body, server: m })),
  ];
  const dropLocal = (id: string) => {
    writeQueue(localQueue.filter((x) => x.id !== id));
    setFlushStalled(false);
  };
  // Taking one out of the queue on the hub: the ones waiting behind it go
  // back behind what it waited on, in order (they are stopped last first,
  // so none is let go on the way, then sent again).
  const unqueue = async (gone: Message) => {
    const chain: Message[] = [];
    for (let cur = gone.id; ;) {
      const next = hubQueue.find((x) => x.after === cur);
      if (!next || chain.includes(next)) break;
      chain.push(next);
      cur = next.id;
    }
    for (const x of [...chain].reverse()) await appApi.stop(target.hubId, x.id);
    await appApi.stop(target.hubId, gone.id);
    setWithdrawn((prev) => new Set([...prev, gone.id, ...chain.map((x) => x.id)]));
    let after = gone.after;
    const resent: Message[] = [];
    for (const x of chain) {
      const re = await appApi.sendMessage(target.hubId, {
        to: x.to, body: x.body, thread_id: x.thread_id, ...(after ? { after } : {}),
        ...(x.attachments?.length ? { attachments: x.attachments.map((a) => a.file_id) } : {}),
      });
      resent.push(re);
      after = re.id;
    }
    if (resent.length) setMessages((prev) => (prev ? [...prev, ...resent] : resent));
  };
  const withdrawQueueItem = async (item: QueueItem) => {
    setQueueError(null);
    if (item.local) { dropLocal(item.local.id); return; }
    if (!item.server || queueBusy) return;
    setQueueBusy(item.key);
    try {
      await unqueue(item.server);
    } catch (e) {
      setQueueError(e);
    } finally {
      setQueueBusy(null);
    }
  };
  const editQueueItem = async (item: QueueItem) => {
    if (item.local) {
      dropLocal(item.local.id);
      writeDraft(item.body);
      inputRef.current?.focus();
      return;
    }
    if (!item.server || queueBusy) return;
    setQueueError(null);
    setQueueBusy(item.key);
    try {
      await unqueue(item.server);
      writeDraft(item.body);
      inputRef.current?.focus();
    } catch (e) {
      setQueueError(e);
    } finally {
      setQueueBusy(null);
    }
  };

  // The view stays at the bottom while something there grows (a live trace).
  const followBottom = () => {
    const el = scrollRef.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  };

  // What the timeline lists. Watching two nodes, progress is folded under
  // the answer it led to, or shown by the node at work; only the last
  // progress of a task that ended without an answer here stands in for
  // all of it.
  // A task with its run shown under it (yours, or one another member gave
  // the agent you are looking at): its progress is part of that run.
  const inRun = (task?: Message) => !!task && task.type === "task" && !task.e2e
    && ((isMe(task.from, row?.hub) && !isStuck(task)) || (alsoSet.has(task.from) && task.to === target.agent));
  const byId = new Map((shown || []).map((m) => [m.id, m]));
  // A supplement is drawn inside the task it names, not as a turn of its
  // own (only one loading/status for the task remains).
  const steersOf = new Map<string, Message[]>();
  // What answered a question an agent asked: shown on its card.
  const answerTo = new Map<string, Message>();
  for (const m of shown || []) {
    if (m.type === "steer" && m.answers && byId.get(m.answers)) { answerTo.set(m.answers, m); continue; }
    if (m.type !== "steer" || !m.reply_to || !byId.get(m.reply_to)) continue;
    steersOf.set(m.reply_to, [...(steersOf.get(m.reply_to) || []), m]);
  }
  const answerAsk = (task: Message) => async (text: string) => {
    const m = await appApi.steer(target.hubId, task.id, text, undefined);
    setMessages((prev) => (prev ? [...prev, m] : [m]));
    atBottom.current = true;
  };
  // A message queued behind a task joins the conversation when that task
  // ends (its answer), not when it was written: it goes after that answer.
  const sinceAt = (m: Message): number => {
    const t = new Date(m.created_at).getTime();
    const prev = m.after ? byId.get(m.after) : undefined;
    if (!prev) return t;
    const end = answerOf.get(prev.id);
    return Math.max(t, end ? new Date(end.created_at).getTime() + 1 : !isOpen(prev.status) ? new Date(prev.updated_at).getTime() : t);
  };
  if (arrived.current.key !== cacheKey(target)) arrived.current = { key: cacheKey(target), at: Date.now() };
  const now0 = Date.now();
  for (const m of messages || []) if (!firstSeen.has(m.id)) firstSeen.set(m.id, now0);
  const growsIn = (m: Message) => m.type === "reply" && !isMe(m.from, row?.hub) && !m.unreadable && !revealed.has(m.id)
    && !(m.reply_to && drafted.has(m.reply_to))
    && (firstSeen.get(m.id) ?? 0) - arrived.current.at > 2000;
  const visible = (shown || []).filter((m) => {
    if (m.type === "steer" && m.answers && answerTo.get(m.answers) === m) return false;
    if (m.type === "steer" && m.reply_to && steersOf.has(m.reply_to)) return false;
    if (m.type !== "progress") return true;
    if (!observed) return !inRun(byId.get(taskOfProgress.get(m.id) || ""));
    const task = taskOfProgress.get(m.id) || "";
    if (answerOf.has(task) || task === working?.task.id) return false;
    const all = progressOf.get(task);
    return !all || all[all.length - 1] === m;
  });
  if (visible.some((m) => m.after)) {
    const at = new Map(visible.map((m) => [m.id, sinceAt(m)]));
    // What answers a released message comes after it too (stable sort).
    visible.sort((a, b) => Math.max(at.get(a.id)!, a.reply_to && at.has(a.reply_to) ? at.get(a.reply_to)! : 0)
      - Math.max(at.get(b.id)!, b.reply_to && at.has(b.reply_to) ? at.get(b.reply_to)! : 0));
  }
  // Inserts by time: each goes before the first message newer than it.
  const placed = new Map<string, React.ReactNode[]>();
  const trailing: React.ReactNode[] = [];
  {
    // All of them, from the start (an empty chat shows them too, so none
    // appear all at once when you write); only while older messages are
    // still to load, the ones from before what is loaded wait for them.
    const start = exchange && more && visible.length ? new Date(visible[0].created_at).getTime() : -Infinity;
    const ins = [...(inserts || [])]
      .filter((x) => new Date(x.at).getTime() >= start)
      .sort((a, b) => a.at.localeCompare(b.at));
    let k = 0;
    for (const m of visible) {
      while (k < ins.length && new Date(ins[k].at).getTime() <= new Date(m.created_at).getTime()) {
        placed.set(m.id, [...(placed.get(m.id) || []), <Fragment key={"x" + ins[k].id}>{ins[k].node}</Fragment>]);
        k++;
      }
    }
    for (; k < ins.length; k++) trailing.push(<Fragment key={"x" + ins[k].id}>{ins[k].node}</Fragment>);
  }
  // The message just before each one (what an answer may be answering).
  const prevShown = new Map<string, string>();
  visible.forEach((m, i) => { if (i > 0) prevShown.set(m.id, visible[i - 1].id); });

  // showName: the first of a run of messages from one sender (watching two
  // nodes, names are shown once per run, as a chat groups them).
  const renderMessage = (m: Message, showName = true) => {
      if (m.type === "progress") {
        const line = (
          <div className="conv-progress" key={m.id} title={clockTime(m.created_at)}>
            <span className="conv-progress-dot" aria-hidden />
            <span>{m.body}</span>
          </div>
        );
        if (!observed) return line;
        // Watching two nodes, progress is folded under the answer, or shown
        // by the node at work; what is left (a task that ended without an
        // answer here) is one folded line on its sender's side.
        const items = progressOf.get(taskOfProgress.get(m.id) || "") || [m];
        return (
          <div key={m.id} className={cn("conv-msg is-progress", m.from === target.agent ? "is-me" : "is-agent")}>
            {showName && <div className="conv-msg-name">{nameOf(m.from)}</div>}
            <ProgressFold items={items} />
          </div>
        );
      }
      if (m.type === "event") {
        return (
          <div className="conv-event" key={m.id}>
            <AlertTriangle />
            <span>{m.body}</span>
            <span className="conv-event-time">{clockTime(m.created_at)}</span>
          </div>
        );
      }
      // In an observed thread, the node you opened is on the right. With
      // others merged in, whoever gives it work (you or another member) is
      // on the right, named; its answers to them say whom they answer.
      const fromOther = !observed && alsoSet.has(m.from);
      const toOther = !observed && m.from === target.agent && alsoSet.has(m.to);
      const mine = observed ? m.from === target.agent : isMe(m.from, row?.hub) || fromOther;
      // An answer that does not come right after what it answers (several
      // answered at once, others in between) quotes it, as chat apps do, so
      // it is never an answer to nothing. Watching two nodes, its progress
      // folds under it (in your own chats it is in the run).
      const answers = taskOfAnswer.get(m.id);
      const asked = answers ? shown?.find((x) => x.id === answers) : undefined;
      // (an answer naming a message not loaded here quotes "an earlier one")
      const quote = !!answers && prevShown.get(m.id) !== answers && (!!asked || !!m.reply_to);
      const steps = observed && answers ? progressOf.get(answers) : undefined;
      const usage = answers ? usageOf[answers] : undefined;
      const failedNote = m.status === "failed" && m.error === STOPPED ? L("已停止", "Stopped")
        : m.status === "failed" && m.error ? L(`失败：${m.error}`, `Failed: ${m.error}`) : "";
      return (
        <Fragment key={m.id}>
        <div className={cn("conv-msg", mine ? "is-me" : "is-agent", (fromOther || toOther || (alsoSet.size > 0 && showName && isMe(m.from, row?.hub))) && "is-other")} data-msg={m.id} data-turn={turnIds.has(m.id) ? m.id : undefined}>
          {fromOther ? <div className="conv-msg-name">{nameOf(m.from)}</div>
            : alsoSet.size > 0 && isMe(m.from, row?.hub) ? (showName ? <div className="conv-msg-name">{L("你", "You")}</div> : null)
            : toOther ? <div className="conv-msg-name">{L(`回复 ${nameOf(m.to)}`, `To ${nameOf(m.to)}`)}</div>
            : showName && <div className="conv-msg-name">{nameOf(m.from)}</div>}
          {steps && steps.length > 0 && <ProgressFold items={steps} />}
          <div className="conv-bubble">
            {quote && (asked ? (
              <button type="button" className="conv-quote" onClick={() => jumpTo(asked.id)} title={L("跳到这条消息", "Go to this message")}>
                <span className="conv-quote-name">{nameOf(asked.from)}</span>
                <span className="conv-quote-text">{asked.unreadable ? L("已加密的消息", "An encrypted message") : asked.body.replace(/\s+/g, " ").trim()}</span>
              </button>
            ) : (
              <span className="conv-quote is-gone">{L("回复更早的一条消息", "Replying to an earlier message")}</span>
            ))}
            {m.type === "steer" && <span className="conv-tag">{L("补充", "Added")}</span>}
            {m.type === "note" && <span className="conv-tag neutral">{L("备注", "Note")}</span>}
            {m.unreadable
              ? <span className="conv-unreadable"><Lock />{L("已加密，只有双方的主人能看", "Encrypted: only the owners of both sides can read it")}</span>
              : growsIn(m) ? <Reveal body={m.body} onDone={() => revealed.add(m.id)} />
              : <MessageContent body={m.body} />}
            {m.attachments && m.attachments.length > 0 && <MessageAttachments hubId={target.hubId} items={m.attachments} />}
            {(steersOf.get(m.id) || []).map((s) => (
              <div className="conv-supplement" key={s.id} data-supplement={s.id}>
                <div className="conv-supplement-head">
                  <span className="conv-tag">{L("补充", "Added")}</span>
                  <span className="conv-supplement-time">{clockTime(s.created_at)}</span>
                </div>
                {s.unreadable
                  ? <span className="conv-unreadable"><Lock />{L("已加密，只有双方的主人能看", "Encrypted: only the owners of both sides can read it")}</span>
                  : <MessageContent body={s.body} />}
                {s.attachments && s.attachments.length > 0 && <MessageAttachments hubId={target.hubId} items={s.attachments} />}
              </div>
            ))}
          </div>
          {/* The time and copy stay out of the way until the pointer is on the
              message; a failure or an encryption mark is always shown. */}
          <div className={cn("conv-msg-meta", failedNote && "is-shown", m.status === "failed" && m.error !== STOPPED && "is-failed", (m.e2e || (row?.hub.e2e && !m.from.startsWith("@"))) && "is-shown")}>
            {m.e2e && <Lock className="conv-lock" aria-label={L("端到端加密", "End-to-end encrypted")} />}
            {!m.e2e && row?.hub.e2e && !m.from.startsWith("@") && <span className="conv-plain">{L("未加密", "Unencrypted")}</span>}
            {usage && <span className="conv-usage">{usage}</span>}
            {failedNote || clockTime(m.created_at)}
            {!m.unreadable && <CopyMessage text={m.body} />}
          </div>
        </div>
        {/* What the agent did for a task you gave it (its runner reports it):
            the start of its turn, on its side of the conversation. A task that
            went quiet after it was picked up may have lost its result: say so,
            and offer to stop it and send it again. */}
        {m.type === "task" && !m.e2e && isMe(m.from, row?.hub) && (isStuck(m) ? (
          <div className="conv-stuck" data-stuck={m.id}>
            <span className="dot pending" aria-hidden />
            <span className="conv-stuck-text">{stuckLine(m)}</span>
            <span className="conv-stuck-actions">
              <button type="button" className="conv-steer-btn" disabled={resending === m.id} onClick={() => void stop(m)}>
                {L("停止", "Stop")}
              </button>
              <button type="button" className="conv-steer-btn" disabled={resending === m.id} onClick={() => void resend(m)}>
                {resending === m.id ? L("正在重发…", "Resending…") : L("重发", "Resend")}
              </button>
            </span>
          </div>
        ) : <TraceView hubId={target.hubId} task={m} onGrow={followBottom} waiting={waitingReason} answered={answerOf.has(m.id)} quiet={m.id !== liveId} footed={m.id === liveId && liveFoot} waitingOn={waitingOn.get(m.id)} onPaused={onPaused} said={progressOf.get(m.id)} onUsage={answerOf.has(m.id) ? onUsage : undefined}
          answers={answerTo} canAnswer={isMe(m.from, row?.hub)} onAnswer={answerAsk(m)} />)}
        {/* What it did for a task someone else gave it, the same way. */}
        {m.type === "task" && !m.e2e && fromOther && m.to === target.agent && (
          <TraceView hubId={target.hubId} task={m} onGrow={followBottom} answered={answerOf.has(m.id)} quiet={m.id !== liveId} footed={m.id === liveId && liveFoot} waitingOn={waitingOn.get(m.id)} onPaused={onPaused} said={progressOf.get(m.id)} onUsage={answerOf.has(m.id) ? onUsage : undefined} answers={answerTo} />
        )}
        </Fragment>
      );
  };

  // A file sent again under the same name is its next version.
  const versions = versionsOf(shown || []);

  return (
    <VersionsContext.Provider value={versions}>
    <div className={cn("conv-detail", hasRail && "has-rail")}>
      {!bare && <header className="conv-head">
        {(showBack || target.returnTo) && (
          <button type="button" className="app-btn app-btn-plain" onClick={onBack}
            aria-label={backLabel || L("返回", "Back")} title={backLabel || L("返回", "Back")}>
            <ArrowLeft />
          </button>
        )}
        {/* Who this is with: their faces, their names, and what they do. */}
        <span className="conv-head-faces">
          {(observed ? parties : [target.agent]).map((n) => {
            const r = rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === n);
            return r ? <Avatar key={n} agent={r.st.agent} size={30} badge={false} /> : null;
          })}
        </span>
        <div className="conv-head-titles">
          <h2>{observed ? parties.map(nameOf).join(" 和 ") : agentName}</h2>
          <p>
            {observed
              ? L("它们之间的对话，你只能看", "Their conversation; you can only read it")
              : shellMode ? (agent ? whereOf({ hub: row!.hub, st: row!.st }) : "") : [role, agent?.description].filter(Boolean).join(" · ") || L("和你的一对一", "Just you and it")}
            {row?.hub.stale && <> · <span className="app-stale" title={row.hub.stale.error || undefined}>{L(`连不上 · 显示的是 ${timeAgo(row.hub.stale.at, now)}的内容`, `Can't reach it · showing ${timeAgo(row.hub.stale.at, now)}`)}</span></>}
          </p>
        </div>
        {/* No running state here (已完成, 在干活…): the foot of the chat
            shows that. Only a task that may be stuck is called out, and
            this takes you to it. */}
        {!observed && firstStuck && (
          <button type="button" className="conv-head-state is-link" onClick={toStuck} title={L("跳到可能卡住的任务", "Go to the task that may be stuck")}>
            <span className={cn("dot", state.dot)} />
            {state.label}
          </button>
        )}
        {headActions && <span className="conv-head-actions">{headActions}</span>}
      </header>}

      {/* The terminal shows only your own commands; ones other nodes sent to
          this computer may be stuck with nothing on screen. Say so above it,
          with what they were and a way to stop them. */}
      {termMode && shellStuck.length > 0 && (
        <div className={cn("shell-stuck", moreStuck && "is-open")} data-shell-stuck>
          <div className="shell-stuck-line">
            <span className="dot pending" aria-hidden />
            <span className="shell-stuck-text">
              {L(
                `这台电脑上有 ${shellStuck.length} 条命令可能卡住（来自 ${shellFrom.join("、")}）`,
                `${shellStuck.length} command${shellStuck.length === 1 ? "" : "s"} on this computer may be stuck (from ${shellFrom.join(", ")})`,
              )}
            </span>
            <button type="button" className="shell-stuck-toggle" aria-expanded={moreStuck} onClick={() => setMoreStuck((v) => !v)}>
              <ChevronRight />
              {moreStuck ? L("收起", "Hide") : L("查看", "Show")}
            </button>
            {canStopShell && (
              <button type="button" className="conv-steer-btn" disabled={stoppingStuck} onClick={() => void stopAllOnShell()}>
                {stoppingStuck ? L("正在停止…", "Stopping…") : L("全部停止", "Stop all")}
              </button>
            )}
          </div>
          {moreStuck && (
            <ul className="shell-stuck-list">
              {shellStuck.map((m) => {
                const cmd = (m.body || "").split("\n")[0].trim();
                const quiet = quietFor(now, lastActivityAt(m));
                return (
                  <li key={m.id} className="shell-stuck-row">
                    <span className="shell-stuck-cmd" title={m.body}>{cmd || nameOf(m.from)}</span>
                    <span className="shell-stuck-meta">{nameOf(m.from)} · {L(`${quiet}没有动静`, `quiet for ${quiet}`)}</span>
                    {canStopShell && (
                      <button type="button" className="conv-steer-btn" onClick={() => void stopOnShell(m)}>
                        {L("停止", "Stop")}
                      </button>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
          <ErrorText error={stuckError} />
        </div>
      )}

      <div className={cn("conv-body", hasRail && "has-rail", termMode && "is-shell")}>
      {hasRail && <TurnRail turns={turns} scroller={scrollRef} />}
      <div className="conv-scroll" ref={scrollRef} onScroll={onScroll}>
        <div ref={timelineRef} className={cn("conv-timeline", termMode && "is-shell", observed && "is-observed")}>
          {shown === null ? (
            loadError && !isTransient(loadError) ? (
              <ErrorText error={loadError} />
            ) : loadError ? (
              // The hub is slow or down: keep the placeholders, say so
              // quietly; polling retries and the note goes once it answers.
              <>
                <LoadingSkeleton shell={isExecNode(target.agent) || isExecNode(target.peer)} />
                <div className="conv-retry-note">{L("暂时连不上，正在重试…", "Can't reach this hub right now; retrying…")}</div>
              </>
            ) : (
              <LoadingSkeleton shell={isExecNode(target.agent) || isExecNode(target.peer)} />
            )
          ) : shown.length === 0 && !shellMode && trailing.length === 0 ? (
            observed
              ? <div className="app-empty-note">{L("它们还没说过话。", "They haven't talked yet.")}</div>
              : <EmptyChat key={target.agent} agent={agent} name={agentName} project={projectTitle} />
          ) : (
            <>
            {exchange && more && !(shellMode && cleared) && (
              <div className="conv-older">
                <button type="button" className="app-btn app-btn-small" onClick={() => void loadOlder()} disabled={loadingOlder}>
                  <Busy busy={loadingOlder}>{L("加载更早的消息", "Load earlier messages")}</Busy>
                </button>
              </div>
            )}
            {shellMode ? (
              <>
              <div onMouseUp={() => { if (window.getSelection()?.isCollapsed !== false) inputRef.current?.focus(); }} className="term-wrap">
                {cleared && (
                  <button type="button" className="term-restore" onClick={() => setCleared(null)}>
                    {L("已清屏 · 显示之前的内容", "Cleared · show what was here")}
                  </button>
                )}
                <Terminal messages={onScreen || shown} execNode={target.agent} nameOf={nameOf} hubE2E={!!row?.hub.e2e} live cwd={agent?.binding.workdir}
                  hubId={target.hubId} onGrow={followBottom}>
                  <div className="term-input-line">
                    <span className="term-prompt">{agent?.binding.workdir ? <span className="term-cwd">{agent.binding.workdir}</span> : null}<PromptSign /></span>
                    <textarea
                      ref={inputRef}
                      className="term-input"
                      value={draft}
                      onChange={(e) => { writeDraft(e.target.value); histPos.current = -1; }}
                      onKeyDown={onKeyDown}
                      rows={1}
                      autoFocus
                      spellCheck={false}
                      autoCapitalize="off"
                      autoCorrect="off"
                      placeholder={shown.length === 0 ? L("输入命令，回车执行", "Type a command, Enter to run") : ""}
                      aria-label={L("命令", "Command")}
                    />
                  </div>
                </Terminal>
              </div>
              <div className="term-foot">
                {openTask
                  ? <button type="button" className="term-foot-stop" onClick={() => void stop(openTask)}>{L("正在执行 · Ctrl+C 停止", "Running · Ctrl+C to stop")}</button>
                  : <span>{L(`在 ${agentName} 上执行`, `Runs on ${agentName}`)}</span>}
                <span>{L("回车执行 · Shift+Enter 换行 · ↑↓ 历史命令", "Enter to run · Shift+Enter for a new line · ↑↓ history")}</span>
              </div>
              <ErrorText error={sendError || stopError} />
              </>
            ) : execNode ? <Terminal messages={shown} execNode={execNode} nameOf={nameOf} hubE2E={!!row?.hub.e2e} hubId={target.hubId} onGrow={followBottom} /> : visible.map((m, i) => {
              // Inserts that come before this message (by time).
              const before = placed.get(m.id);
              // A divider with the time only where the conversation picks up
              // again after a while, not between every two threads.
              const divider = i > 0 && new Date(m.created_at).getTime() - new Date(visible[i - 1].created_at).getTime() > GAP_MS
                ? <div className="conv-thread-divider" key={"d" + m.id}><span>{dayTime(m.created_at)}</span></div>
                : null;
              const prev = visible[i - 1];
              const runStart = !prev || !!divider || prev.from !== m.from || prev.type === "event" || m.type === "event";
              const item = renderMessage(m, runStart);
              return [...(before || []), ...(divider ? [divider, item] : [item])];
            })}
            {!shellMode && !execNode && trailing}
            {/* The run at work, when messages sit under it. */}
            {liveFoot && liveRun && (
              <WorkingLine key={liveRun.id} name={nameOf(liveRun.to)} since={liveRun.created_at}
                status={progressOf.get(liveRun.id)?.slice(-1)[0]?.body.split("\n")[0]} side="left" />
            )}
            {working && (
              <WorkingLine key={working.task.id} name={nameOf(working.task.to)} since={working.task.created_at}
                status={working.status} queued={working.queued} quiet={!footLive} behind={working.behind}
                side={observed && working.task.to === target.agent ? "right" : "left"} />
            )}
            </>
          )}
        </div>
      </div>
      </div>

      {shellMode ? null : observed ? (
        <div className="conv-observe">
          <span>{L("这是它们之间的对话，你只能看。", "This is a record of nodes working together. You can only view it.")}</span>
          <button type="button" className="app-btn" onClick={() => onNewConversation(target.hubId, target.agent)}>
            {L(`给 ${agentName} 发任务`, `Give ${agentName} a task`)}
          </button>
        </div>
      ) : (
      <div className="conv-compose">
        {busy && (
          <div className="conv-busy">
            <span className="dot pending" />
            <span className="conv-busy-text">
              {openTask
                ? L("这个会话正开在别处，任务在等它空出来。", "This session is open somewhere else; the task waits until it is free.")
                : L("这个会话正开在别处（终端、编辑器或桌面版），现在发的消息要等它空出来才会处理。", "This session is open somewhere else (a terminal, an editor or a desktop app); messages wait until it is free.")}
            </span>
            {forkable && onOpen && (
              <button type="button" className="conv-steer-btn" disabled={copying} onClick={() => void copyAndContinue()}
                title={L("复制一个一模一样的它（同一台电脑、同一个文件夹），在副本里接着做；原来的不动", "Copy this session into a new node (same machine and folder) and carry on there; the original session is left alone")}>
                {copying ? L("正在复制…", "Copying…") : openTask ? L("复制一份接着做", "Continue in a copy") : L("复制一份来对话", "Talk to a copy")}
              </button>
            )}
            {openTask && (
              <button type="button" className="conv-steer-btn" onClick={() => void stop(openTask)}>{L("停止", "Stop")}</button>
            )}
          </div>
        )}
        {busy && <ErrorText error={copyError} />}
        {!busy && offline && (
          <div className="conv-busy">
            <span className="dot idle" />
            <span className="conv-busy-text">
              {agent && isInbox(agent)
                ? L("对方只在会话里接收消息，现在发的消息等它的会话再挂上 inbox wait 后处理。", "The node receives only in its session; messages sent now are handled when its session runs inbox wait again.")
                : L("对方离线，现在发的消息会在它上线后处理。", "The node is offline; messages sent now are handled when it is back.")}
            </span>
          </div>
        )}
        {queueRows.length > 0 && (
          <div className="conv-queue" data-queue>
            {queueRows.map((item) => (
              <div className="conv-queue-row" data-queue-row={item.key} key={item.key}>
                <span className="conv-queue-text" title={item.body}>
                  <span className="conv-queue-label">{L("排队中：", "Queued: ")}</span>
                  {item.body.split("\n")[0].trim() || L("（附件）", "(files)")}
                </span>
                <button type="button" className="conv-queue-btn" disabled={queueBusy === item.key}
                  onClick={(e) => { e.stopPropagation(); void editQueueItem(item); }}>
                  {L("编辑", "Edit")}
                </button>
                <button type="button" className="conv-queue-btn" disabled={queueBusy === item.key}
                  onClick={(e) => { e.stopPropagation(); void withdrawQueueItem(item); }}>
                  {L("撤回", "Withdraw")}
                </button>
              </div>
            ))}
          </div>
        )}
        {queueRows.length > 0 && <ErrorText error={queueError} />}
        <div className="conv-compose-box" onClick={() => inputRef.current?.focus()} {...dropFiles(att.add)}>
          <AttachList items={att.items} onRemove={att.remove} />
          <textarea
            ref={inputRef}
            className="conv-compose-input"
            value={draft}
            onChange={(e) => { writeDraft(e.target.value); chatHistory.reset(); }}
            onPaste={(e) => {
              const files = pastedFiles(e);
              if (!files.length) return;
              // A file on the clipboard goes as a file (its text, if any, too).
              if (!e.clipboardData.getData("text/plain")) e.preventDefault();
              att.add(files);
            }}
            onKeyDown={onKeyDown}
            disabled={busy}
            placeholder={busy
              ? L("会话正开在别处，暂时不能发；可以「复制一份来对话」", "The session is open elsewhere, so you can't send now; you can talk to a copy")
              : supplementing && openTask?.asking
                ? L(`回答 ${agentName} 的问题…`, `Answer ${agentName}'s question…`)
              : supplementing
                ? L(`补充给 ${agentName}…（${mod}↩ 排到下一条）`, `Add to ${agentName}… (${mod}↩ queues the next one)`)
                : L(`发给 ${agentName}…`, `Message ${agentName}…`)}
            rows={1}
            aria-label={L("消息", "Message")}
          />
          <div className="conv-compose-bar">
            {/* left: what you can do to the running task; room for attachments
                and voice input later */}
            <div className="conv-compose-tools">
              <AttachButton onFiles={att.add} disabled={busy} />
            </div>
            <span className="conv-compose-hint">
              {supplementing
                ? L(`Enter 补充 · ${mod}↩ 排到下一条`, `Enter to add · ${mod}↩ to queue`)
                : L("Enter 发送 · Shift+Enter 换行", "Enter to send · Shift+Enter for a new line")}
            </span>
            {/* Speaking instead of typing, beside Send (as in Codex and Claude). */}
            {!shellMode && <DictateButton input={inputRef} value={draft} onText={(t) => { writeDraft(t); chatHistory.reset(); }} disabled={busy} />}
            {openTask && !draft.trim() && !att.items.length && !sending ? (
              // Nothing to send while it works: the same button stops it.
              <button
                type="button"
                className="conv-send is-stop"
                onClick={(e) => { e.stopPropagation(); void stop(openTask); }}
                title={L("停止这个任务：agent 不再继续做", "Stop this task: the agent stops working on it")}
                aria-label={L("停止", "Stop")}
              >
                <Square size={11} strokeWidth={0} fill="currentColor" />
              </button>
            ) : (
              <button
                type="button"
                className="conv-send"
                onClick={(e) => { e.stopPropagation(); void send(); }}
                disabled={sending || att.busy || (!draft.trim() && !att.any)}
                title={supplementing ? L("补充给这条任务", "Add to this task") : L("发送", "Send")}
                aria-label={supplementing ? L("补充给这条任务", "Add to this task") : L("发送", "Send")}
              >
                {sending ? <Loader2 size={15} className="spin" /> : <ArrowUp size={16} strokeWidth={2.4} />}
              </button>
            )}
          </div>
        </div>
        <ErrorText error={sendError || stopError} />
      </div>
      )}
    </div>
    </VersionsContext.Provider>
  );
}

/** Copies a reply; shown with the time when the pointer is on the message. */
function CopyMessage({ text }: { text: string }) {
  const { copied, copy } = useCopy(text);
  return (
    <button type="button" className="conv-msg-copy" onClick={() => void copy()} title={L("复制", "Copy")} aria-label={L("复制", "Copy")}>
      {copied ? <Check /> : <Copy />}
    </button>
  );
}
