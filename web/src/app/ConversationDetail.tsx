import { useEffect, useRef, useState } from "react";
import { AlertTriangle, ArrowLeft, ArrowUp, Loader2, Lock, Paperclip } from "lucide-react";
import { appApi } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow } from "@/app/types";
import { cliName, isMe, personLabel } from "@/app/nodes";
import { MessageContent } from "@/app/Markdown";
import { Terminal } from "@/app/Terminal";
import { ErrorText } from "@/app/dialogs";
import { bytes, cn, timeAgo } from "@/lib/utils";
import { L, lang, locale } from "@/lib/i18n";

export interface ConversationTarget {
  hubId: string;
  hubName: string;
  threadId: string;
  agent: string;
  // With peer: the whole history between agent and peer (all threads),
  // read-only, instead of one thread.
  peer?: string;
  // Where Back goes: the node panel it was opened from.
  returnTo?: { page: string; node: { hubId: string; name: string } };
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

export function ConversationDetail({ target, rows, now, onBack, onNewConversation, showBack = true }: {
  target: ConversationTarget;
  rows: AgentRow[];
  now: number;
  onBack: () => void;
  /** Next to the list (split view) there is nothing to go back to, unless
   *  the conversation was opened from a node. */
  showBack?: boolean;
  // Starts a new conversation of yours with the agent (from a read-only thread).
  onNewConversation: (hubId: string, agent: string) => void;
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
  const agentName = agent?.display_name || target.agent;
  const cli = agent ? cliName(agent) : "";
  const model = agent?.runtime?.model;

  const exchange = !!target.peer;
  // Exchange view: older pages loaded so far (oldest first) and whether more exist.
  const [older, setOlder] = useState<Message[]>([]);
  const [more, setMore] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setMessages(null);
    setLoadError(null);
    setSendError(null);
    setDraft("");
    setSteerMode(false);
    setOlder([]);
    setMore(false);
    atBottom.current = true;
    const run = async (silent: boolean) => {
      try {
        if (target.peer) {
          // The newest page, polled; older pages stay as loaded.
          const r = await appApi.exchanges(target.hubId, target.agent, target.peer);
          if (!cancelled) {
            setMessages([...r.messages].reverse());
            if (!silent) setMore(r.more);
            setLoadError(null);
          }
          return;
        }
        const r = await appApi.thread(target.hubId, target.threadId);
        if (!cancelled) {
          setMessages(r.messages);
          setLoadError(null);
        }
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
  const failed = lastTask?.status === "failed" || afterTask.some((m) => m.type === "event");
  const state = openTask
    ? { dot: "working", label: L("处理中", "Working") }
    : failed
      ? { dot: "failed", label: L("失败", "Failed") }
      : lastTask
        ? { dot: "idle", label: L("已完成", "Done") }
        : { dot: "idle", label: L("空闲", "Idle") };
  const steerOn = steerMode && !!openTask;

  // A thread you take no part in (two nodes working together) is only
  // shown: writing into it would land in their conversation, not yours.
  const observed = exchange || (!!messages && messages.length > 0 &&
    !messages.some((m) => isMe(m.from, row?.hub) || isMe(m.to, row?.hub)));
  const parties = exchange
    ? [target.agent, target.peer!]
    : observed && messages
      ? [...new Set(messages.flatMap((m) => [m.from, m.to]))].filter((n) => n && !n.startsWith("@"))
      : [];

  // With an exec (shell) node on one side, the messages read as a terminal.
  const isExecNode = (n?: string) => !!n && rows.some((x) => x.hub.id === target.hubId && x.st.agent.name === n && x.st.agent.binding.harness === "exec");
  const execNode = [target.agent, target.peer, ...parties].find((n) => isExecNode(n));

  const nameOf = (from: string): string => {
    if (isConsole(from)) return personLabel(from, row?.hub);
    const r = rows.find((x) => x.hub.id === target.hubId && x.st.agent.name === from);
    return r?.st.agent.display_name || from;
  };

  const send = async () => {
    const body = draft.trim();
    if (!body || sending) return;
    setSending(true);
    setSendError(null);
    try {
      const m = openTask && steerOn
        ? await appApi.steer(target.hubId, openTask.id, body)
        : await appApi.sendMessage(target.hubId, { to: target.agent, body, thread_id: target.threadId });
      setMessages((prev) => (prev ? [...prev, m] : [m]));
      setDraft("");
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
    if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
    e.preventDefault();
    void send();
  };

  const renderMessage = (m: Message) => {
      if (m.type === "progress") {
        return (
          <div className="conv-progress" key={m.id}>
            <span className="conv-progress-label">{L("进度：", "Progress: ")}</span>
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
      return (
        <div className={cn("conv-msg", mine ? "is-me" : "is-agent")} key={m.id}>
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
          <div className={cn("conv-msg-meta", m.status === "failed" && "is-failed")}>
            {m.e2e && <Lock className="conv-lock" aria-label={L("端到端加密", "End-to-end encrypted")} />}
            {!m.e2e && row?.hub.e2e && !m.from.startsWith("@") && <span className="conv-plain">{L("未加密", "Unencrypted")}</span>}
            {m.status === "failed" && m.error ? L(`失败：${m.error}`, `Failed: ${m.error}`) : clockTime(m.created_at)}
          </div>
        </div>
      );
  };

  return (
    <div className="conv-detail">
      <header className="conv-head">
        {(showBack || target.returnTo) && (
          <button type="button" className="app-btn app-btn-plain" onClick={onBack}
            aria-label={target.returnTo ? L("返回节点", "Back to the node") : L("返回对话列表", "Back to conversations")} title={L("返回", "Back")}>
            <ArrowLeft />
          </button>
        )}
        <div className="conv-head-titles">
          <h2>{observed ? parties.map(nameOf).join(" ↔ ") : agentName}</h2>
          <p>
            {exchange
              ? <>{L("两个节点之间的全部消息", "All messages between the two nodes")} · </>
              : observed && <>{L("其他节点之间的消息", "Messages between other nodes")} · </>}
            {target.hubName}
            {cli && <> · {cli}</>}
            {model && <> · <span className="mono">{model}</span></>}
          </p>
        </div>
        {!observed && <span className="conv-head-state">
          <span className={cn("dot", state.dot)} />
          {messages === null ? L("加载中", "Loading") : state.label}
        </span>}
      </header>

      <div className="conv-scroll" ref={scrollRef} onScroll={onScroll}>
        <div className="conv-timeline">
          {shown === null ? (
            loadError ? (
              <ErrorText error={loadError} />
            ) : (
              <div className="app-empty-note">{L("正在加载…", "Loading…")}</div>
            )
          ) : shown.length === 0 ? (
            <div className="app-empty-note">{exchange ? L("还没有消息。", "No messages yet.") : L("还没有消息。发一条任务开始。", "No messages yet. Give a task to get started.")}</div>
          ) : (
            <>
            {exchange && more && (
              <div className="conv-older">
                <button type="button" className="app-btn app-btn-small" onClick={() => void loadOlder()} disabled={loadingOlder}>
                  {loadingOlder ? L("正在加载…", "Loading…") : L("加载更早的消息", "Load earlier messages")}
                </button>
              </div>
            )}
            {execNode ? <Terminal messages={shown} execNode={execNode} nameOf={nameOf} hubE2E={!!row?.hub.e2e} /> : shown.map((m, i) => {
              // Exchange view: a new thread starts with a divider.
              const divider = exchange && i > 0 && shown[i - 1].thread_id !== m.thread_id
                ? <div className="conv-thread-divider" key={"d" + m.id}><span>{dayTime(m.created_at)}</span></div>
                : null;
              const item = renderMessage(m);
              return divider ? [divider, item] : item;
            })}
            </>
          )}
        </div>
      </div>

      {observed ? (
        <div className="conv-observe">
          <span>{L("这是节点之间的协作记录，你只能查看。", "This is a record of nodes working together. You can only view it.")}</span>
          <button type="button" className="app-btn" onClick={() => onNewConversation(target.hubId, target.agent)}>
            {L(`给 ${agentName} 发任务`, `Give ${agentName} a task`)}
          </button>
        </div>
      ) : (
      <div className="conv-compose">
        {openTask && (
          <div className="conv-working">
            <span className="dot working" />
            <span>{L("Agent 正在处理…", "Agent is working…")}</span>
            <button
              type="button"
              className={cn("conv-steer-btn", steerMode && "active")}
              aria-pressed={steerMode}
              onClick={() => setSteerMode((s) => !s)}
            >
              {L("修正", "Correct")}
            </button>
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
            placeholder={steerOn ? L("修改刚才的任务…", "Change the last task…") : L(`发给 ${agentName}…`, `Message ${agentName}…`)}
            rows={1}
            aria-label={L("消息", "Message")}
          />
          <div className="conv-compose-bar">
            {/* left: room for attachments and voice input later */}
            <div className="conv-compose-tools" />
            <span className="conv-compose-hint">{L("Enter 发送 · Shift+Enter 换行", "Enter to send · Shift+Enter for a new line")}</span>
            {messages && messages.length > 0 && (
              <span className="conv-compose-ago">{lang() === "zh" ? <>更新于 {timeAgo(messages[messages.length - 1]?.updated_at, now)}</> : <>Updated {timeAgo(messages[messages.length - 1]?.updated_at, now)}</>}</span>
            )}
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
          </div>
        </div>
        <ErrorText error={sendError} />
      </div>
      )}
    </div>
  );
}
