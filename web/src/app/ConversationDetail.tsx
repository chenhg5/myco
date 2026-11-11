import { Fragment, useEffect, useRef, useState } from "react";
import { AlertTriangle, ArrowLeft, ArrowUp, Check, Copy, Loader2, Lock, Paperclip, Square } from "lucide-react";
import { appApi, isTransient } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow } from "@/app/types";
import { canManage, cliName, isInbox, isMe, machineOfRow, personLabel, runnerAway, sessionBusy, nodeName } from "@/app/nodes";
import { MessageContent } from "@/app/Markdown";
import { PromptSign, STOPPED, Terminal } from "@/app/Terminal";
import { TraceView } from "@/app/TraceView";
import { WorkingLine } from "@/app/Working";
import { TurnRail, type Turn } from "@/app/TurnRail";
import { summarize } from "@/app/summary";
import { ErrorText } from "@/app/dialogs";
import { recall, remember as rememberMemo } from "@/lib/memo";
import { useCopy } from "@/app/copy";
import { bytes, cn, timeAgo } from "@/lib/utils";
import { L, locale } from "@/lib/i18n";
import { Busy } from "@/app/Skeleton";

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

function isOpen(status: string): boolean {
  return status !== "done" && status !== "failed";
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

/** While a conversation loads for the first time: its shape, not a word. */
function LoadingSkeleton({ shell }: { shell: boolean }) {
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

export function ConversationDetail({ target, rows, now, onBack, onNewConversation, onOpen, showBack = true }: {
  target: ConversationTarget;
  rows: AgentRow[];
  now: number;
  onBack: () => void;
  /** Next to the list (split view) there is nothing to go back to, unless
   *  the conversation was opened from a node. */
  showBack?: boolean;
  // Starts a new conversation of yours with the agent (from a read-only thread).
  onNewConversation: (hubId: string, agent: string) => void;
  /** Opens your conversation with another node (a copy just made). */
  onOpen?: (hubId: string, agent: string, peer: string) => void;
}) {
  const [messages, setMessages] = useState<Message[] | null>(null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [sendError, setSendError] = useState<unknown>(null);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [steerMode, setSteerMode] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  // grow with the text, up to about eight lines, then scroll
  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  }, [draft]);
  const atBottom = useRef(true);

  const row = rows.find((r) => r.hub.id === target.hubId && r.st.agent.name === target.agent);
  const agent = row?.st.agent;
  const agentName = agent && row ? nodeName(agent, row.hub) : target.agent;
  const cli = agent ? cliName(agent) : "";
  const model = agent?.runtime?.model;

  const exchange = !!target.peer;
  // Exchange view: older pages loaded so far (oldest first) and whether more exist.
  const [older, setOlder] = useState<Message[]>([]);
  const [more, setMore] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);

  useEffect(() => {
    let cancelled = false;
    // Seen before in this window: show it at once, refresh behind it.
    const key = cacheKey(target);
    const hit = recall<SeenConversation>(key);
    setMessages(hit ? hit.messages : null);
    setLoadError(null);
    setSendError(null);
    setDraft("");
    setSteerMode(false);
    setOlder([]);
    setMore(hit?.more ?? false);
    atBottom.current = true;
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
    const show = (ms: Message[], m: boolean) => {
      current = ms;
      more = m;
      since = changedUntil(ms);
      setMessages(ms);
      setLoadError(null);
      remember(key, ms, m);
    };
    const run = async (silent: boolean) => {
      polls++;
      const delta = silent && incremental && since > 0 && polls % FULL_EVERY !== 0;
      try {
        if (target.peer) {
          // The newest page, polled; older pages stay as loaded.
          const r = await appApi.exchanges(target.hubId, target.agent, target.peer, undefined, delta ? since : undefined);
          if (cancelled) return;
          if (delta && r.since) {
            // Many changes at once: the whole page next time.
            if (r.more) polls = FULL_EVERY - 1;
            if (r.messages.length) show(mergeById(current, r.messages), more);
            return;
          }
          if (delta) incremental = false; // an older hub: whole pages
          const ms = [...r.messages].reverse();
          show(ms, silent ? more : r.more);
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
  }, [target.hubId, target.threadId, target.agent, target.peer]);

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

  useEffect(() => {
    const el = scrollRef.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [messages]);

  const onScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  };

  // Everything shown: in the exchange view, older pages before the newest.
  const shown: Message[] | null = messages === null ? null : exchange
    ? [...older.filter((o) => !messages.some((m) => m.id === o.id)), ...messages]
    : messages;

  const consoleTasks = (messages || []).filter((m) => m.type === "task" && isMe(m.from, row?.hub));
  const lastTask = consoleTasks[consoleTasks.length - 1];
  const answered = !!lastTask && (messages || []).some((m) =>
    (m.type === "reply" || m.type === "event") &&
    m.id !== lastTask.id &&
    new Date(m.created_at).getTime() >= new Date(lastTask.created_at).getTime(),
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
  const state = busy
    ? { dot: "pending", label: L("会话被占用", "Session in use") }
    : openTask && offline
    ? { dot: "idle", label: L("离线 · 等它上线", "Offline · waiting") }
    : openTask && openTask.status !== "acked"
    ? { dot: "pending", label: L("等待中", "Waiting") }
    : openTask
    ? { dot: "working", label: L("处理中", "Working") }
    : stopped
      ? { dot: "idle", label: L("已停止", "Stopped") }
    : failed
      ? { dot: "failed", label: L("失败", "Failed") }
      : lastTask
        ? { dot: "idle", label: L("已完成", "Done") }
        : { dot: "idle", label: L("空闲", "Idle") };
  const steerOn = steerMode && !!openTask;
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
        ? L("节点的 runner 没在运行（可能在重启），起来后会处理", "The node's runner is not running (maybe restarting); it starts when the runner is back")
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
  // Either way (typing at it, or reading what other nodes ran on it) the
  // pane is a terminal.
  const termMode = shellMode || !!execNode;

  // Commands sent here, for ↑ / ↓ (newest last); -1 = not browsing.
  const history = consoleTasks.map((m) => m.body).filter((b) => b && !b.includes("\n"));
  const histPos = useRef(-1);
  const histDraft = useRef("");

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
    if (m.type === "progress" || m.type === "event") continue;
    const starts = observed ? m.type === "task" : isMe(m.from, row?.hub);
    if (starts) turns.push({ id: m.id, who: observed ? nameOf(m.from) : undefined, question: summarize(m), answer: "" });
    else if (turns.length && !turns[turns.length - 1].answer) turns[turns.length - 1].answer = summarize(m);
  }
  const turnIds = new Set(turns.map((t) => t.id));
  const hasRail = !shellMode && !execNode && turns.length >= 2;

  // Someone else's task that its node is still on (two nodes working
  // together): the foot of the conversation shows it at work. Your own
  // tasks show their run as it goes (TraceView) instead.
  const working = (() => {
    if (termMode || !shown) return null;
    for (let i = shown.length - 1; i >= 0; i--) {
      const m = shown[i];
      if (m.type === "progress" || m.type === "steer") continue;
      if (m.type !== "task" || isMe(m.from, row?.hub) || !isOpen(m.status) || m.status === "queued") return null;
      return m;
    }
    return null;
  })();

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
    setDraft("");
    histPos.current = -1;
  };
  const clearedTime = clearedAfter ? new Date(clearedAfter).getTime() : NaN;
  const onScreen = shown && !Number.isNaN(clearedTime) ? shown.filter((m) => new Date(m.created_at).getTime() > clearedTime) : shown;
  const cleared = !!shown && !!onScreen && onScreen.length < shown.length;

  const send = async () => {
    const body = draft.trim();
    if (shellMode && (body === "clear" || body === "cls")) { clearScreen(); return; }
    if (!body || sending || busy) return;
    setSending(true);
    setSendError(null);
    try {
      if (!target.peer && !target.threadId) {
        // The first message of a new conversation: then the whole one.
        const first = await appApi.sendMessage(target.hubId, { to: target.agent, body, ...(target.project ? { project: target.project } : {}) });
        setDraft("");
        if (onOpen) onOpen(target.hubId, target.agent, first.from);
        else setMessages([first]);
        return;
      }
      // Opened from a project: the first message starts a thread in it.
      const inProject = !!target.project && !projectStarted.current;
      const threadId = inProject ? undefined : exchange ? continuedThread() : target.threadId;
      const sendNew = (thread_id?: string) => appApi.sendMessage(target.hubId, { to: target.agent, body, thread_id, ...(inProject && !thread_id ? { project: target.project } : {}) });
      if (inProject) projectStarted.current = true;
      const m = openTask && steerOn
        ? await appApi.steer(target.hubId, openTask.id, body)
        : await sendNew(threadId).catch((e) => {
          // The hub does not count you in that thread (sent under an older
          // name): start a new one; the exchange view shows every thread.
          if (exchange && threadId && (e as { code?: string }).code === "not_found") return sendNew(undefined);
          throw e;
        });
      // The hub signs your messages with your name there: when this view
      // assumed another, follow the one it used.
      if (exchange && m.from && target.peer && m.from !== target.peer && m.type !== "steer" && onOpen) {
        setDraft("");
        onOpen(target.hubId, target.agent, m.from);
        return;
      }
      setMessages((prev) => (prev ? [...prev, m] : [m]));
      setDraft("");
      histPos.current = -1;
      setSteerMode(false);
      atBottom.current = true;
      inputRef.current?.focus();
    } catch (e) {
      setSendError(e);
    } finally {
      setSending(false);
    }
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
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
      setDraft(pos === -1 ? histDraft.current : history[pos]);
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
    void send();
  };

  // Stop a running task you sent: the hub closes it, its runner ends the run.
  const [stopError, setStopError] = useState<unknown>(null);
  const stop = async (task: Message) => {
    setStopError(null);
    try {
      const m = await appApi.stop(target.hubId, task.id);
      setMessages((prev) => prev && prev.map((x) => (x.id === m.id ? m : x)));
      setSteerMode(false);
    } catch (e) {
      setStopError(e);
    }
  };

  // The view stays at the bottom while something there grows (a live trace).
  const followBottom = () => {
    const el = scrollRef.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  };

  const renderMessage = (m: Message) => {
      if (m.type === "progress") {
        return (
          <div className="conv-progress" key={m.id} title={clockTime(m.created_at)}>
            <span className="conv-progress-dot" aria-hidden />
            <span>{m.body}</span>
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
      // In an observed thread, the node you opened is on the right.
      const mine = observed ? m.from === target.agent : isMe(m.from, row?.hub);
      const failedNote = m.status === "failed" && m.error === STOPPED ? L("已停止", "Stopped")
        : m.status === "failed" && m.error ? L(`失败：${m.error}`, `Failed: ${m.error}`) : "";
      return (
        <Fragment key={m.id}>
        <div className={cn("conv-msg", mine ? "is-me" : "is-agent")} data-turn={turnIds.has(m.id) ? m.id : undefined}>
          <div className="conv-msg-name">{nameOf(m.from)}</div>
          <div className="conv-bubble">
            {m.type === "steer" && <span className="conv-tag">{L("修正", "Correct")}</span>}
            {m.type === "note" && <span className="conv-tag neutral">{L("备注", "Note")}</span>}
            {m.unreadable
              ? <span className="conv-unreadable"><Lock />{L("已加密，只有双方的主人能看", "Encrypted: only the owners of both sides can read it")}</span>
              : <MessageContent body={m.body} />}
            {m.attachments && m.attachments.length > 0 && (
              <div className="conv-attach-list">
                {m.attachments.map((a) => (
                  <div className="conv-attach" key={a.file_id} title={L("下载暂不可用", "Download not available yet")}>
                    <Paperclip />
                    <span className="conv-attach-name">{a.name}</span>
                    <span className="conv-attach-size">{bytes(a.size)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
          {/* The time and copy stay out of the way until the pointer is on the
              message; a failure or an encryption mark is always shown. */}
          <div className={cn("conv-msg-meta", failedNote && "is-shown", m.status === "failed" && m.error !== STOPPED && "is-failed", (m.e2e || (row?.hub.e2e && !m.from.startsWith("@"))) && "is-shown")}>
            {m.e2e && <Lock className="conv-lock" aria-label={L("端到端加密", "End-to-end encrypted")} />}
            {!m.e2e && row?.hub.e2e && !m.from.startsWith("@") && <span className="conv-plain">{L("未加密", "Unencrypted")}</span>}
            {failedNote || clockTime(m.created_at)}
            {!mine && !m.unreadable && <CopyMessage text={m.body} />}
          </div>
        </div>
        {/* What the agent did for a task you gave it (its runner reports it):
            the start of its turn, on its side of the conversation. */}
        {m.type === "task" && !m.e2e && isMe(m.from, row?.hub) && <TraceView hubId={target.hubId} task={m} onGrow={followBottom} waiting={waitingReason} />}
        </Fragment>
      );
  };

  return (
    <div className="conv-detail">
      <header className="conv-head">
        {(showBack || target.returnTo) && (
          <button type="button" className="app-btn app-btn-plain" onClick={onBack}
            aria-label={target.returnTo?.project ? L("返回项目", "Back to the project") : target.returnTo ? L("返回节点", "Back to the node") : L("返回对话列表", "Back to conversations")} title={L("返回", "Back")}>
            <ArrowLeft />
          </button>
        )}
        <div className="conv-head-titles">
          <h2>{observed ? parties.map(nameOf).join(" ↔ ") : agentName}</h2>
          <p>
            {exchange && observed
              ? <>{L("两个节点之间的全部消息", "All messages between the two nodes")} · </>
              : observed && <>{L("其他节点之间的消息", "Messages between other nodes")} · </>}
            {target.hubName}
            {cli && <> · {cli}</>}
            {model && <> · <span className="mono">{model}</span></>}
            {row?.hub.stale && <> · <span className="app-stale" title={row.hub.stale.error || undefined}>{L(`离线 · 显示的是 ${timeAgo(row.hub.stale.at, now)}的数据`, `Offline · showing data from ${timeAgo(row.hub.stale.at, now)}`)}</span></>}
          </p>
        </div>
        {!observed && <span
          className="conv-head-state"
          title={messages && messages.length > 0 ? L(`更新于 ${timeAgo(messages[messages.length - 1]?.updated_at, now)}`, `Updated ${timeAgo(messages[messages.length - 1]?.updated_at, now)}`) : undefined}
        >
          <span className={cn("dot", state.dot)} />
          {messages === null ? L("加载中", "Loading") : state.label}
        </span>}
      </header>

      <div className={cn("conv-body", hasRail && "has-rail", termMode && "is-shell")}>
      {hasRail && <TurnRail turns={turns} scroller={scrollRef} />}
      <div className="conv-scroll" ref={scrollRef} onScroll={onScroll}>
        <div className={cn("conv-timeline", termMode && "is-shell", observed && "is-observed")}>
          {shown === null ? (
            loadError && !isTransient(loadError) ? (
              <ErrorText error={loadError} />
            ) : loadError ? (
              // The hub is slow or down: keep the placeholders, say so
              // quietly; polling retries and the note goes once it answers.
              <>
                <LoadingSkeleton shell={isExecNode(target.agent) || isExecNode(target.peer)} />
                <div className="conv-retry-note">{L("暂时连不上这个 Hub，正在重试…", "Can't reach this hub right now; retrying…")}</div>
              </>
            ) : (
              <LoadingSkeleton shell={isExecNode(target.agent) || isExecNode(target.peer)} />
            )
          ) : shown.length === 0 && !shellMode ? (
            <div className="app-empty-note">{exchange ? L("还没有消息。", "No messages yet.") : L("还没有消息。发一条任务开始。", "No messages yet. Give a task to get started.")}</div>
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
                      onChange={(e) => { setDraft(e.target.value); histPos.current = -1; }}
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
            ) : execNode ? <Terminal messages={shown} execNode={execNode} nameOf={nameOf} hubE2E={!!row?.hub.e2e} hubId={target.hubId} onGrow={followBottom} /> : shown.map((m, i) => {
              // Exchange view: a new thread starts with a divider.
              const divider = exchange && i > 0 && shown[i - 1].thread_id !== m.thread_id
                ? <div className="conv-thread-divider" key={"d" + m.id}><span>{dayTime(m.created_at)}</span></div>
                : null;
              const item = renderMessage(m);
              return divider ? [divider, item] : item;
            })}
            {working && (
              <WorkingLine key={working.id} name={nameOf(working.to)} since={working.created_at}
                status={rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === working.to)?.st.agent.status_text}
                side={observed && working.to === target.agent ? "right" : "left"} />
            )}
            </>
          )}
        </div>
      </div>
      </div>

      {shellMode ? null : observed ? (
        <div className="conv-observe">
          <span>{L("这是节点之间的协作记录，你只能查看。", "This is a record of nodes working together. You can only view it.")}</span>
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
                title={L("复制这个会话成一个新节点（同一台机器、同一个目录），在副本里接着做；原来的会话不动", "Copy this session into a new node (same machine and folder) and carry on there; the original session is left alone")}>
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
        {steerOn && (
          <div className="conv-steer-hint">{L("修正会打断当前处理，让 agent 按新的要求继续。", "Correcting interrupts the current work so the agent continues with your new instructions.")}</div>
        )}
        <div className={cn("conv-compose-box", steerOn && "is-steer")} onClick={() => inputRef.current?.focus()}>
          <textarea
            ref={inputRef}
            className="conv-compose-input"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKeyDown}
            disabled={busy}
            placeholder={busy
              ? L("会话正开在别处，暂时不能发；可以「复制一份来对话」", "The session is open elsewhere, so you can't send now; you can talk to a copy")
              : steerOn ? L("修改刚才的任务…", "Change the last task…") : L(`发给 ${agentName}…`, `Message ${agentName}…`)}
            rows={1}
            aria-label={L("消息", "Message")}
          />
          <div className="conv-compose-bar">
            {/* left: what you can do to the running task; room for attachments
                and voice input later */}
            <div className="conv-compose-tools">
              {openTask && (
                <button
                  type="button"
                  className={cn("conv-steer-btn", steerMode && "active")}
                  aria-pressed={steerMode}
                  onClick={(e) => { e.stopPropagation(); setSteerMode((s) => !s); }}
                  title={L("打断当前处理，让 agent 按你新的要求继续", "Interrupt the current work so the agent continues with your new instructions")}
                >
                  {L("修正", "Correct")}
                </button>
              )}
            </div>
            <span className="conv-compose-hint">{L("Enter 发送 · Shift+Enter 换行", "Enter to send · Shift+Enter for a new line")}</span>
            {openTask && !draft.trim() && !sending ? (
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
                disabled={sending || !draft.trim()}
                title={steerOn ? L("发送修正", "Send correction") : L("发送", "Send")}
                aria-label={steerOn ? L("发送修正", "Send correction") : L("发送", "Send")}
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
