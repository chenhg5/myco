import { useEffect, useRef, useState } from "react";
import { AlertTriangle, ArrowLeft, Paperclip } from "lucide-react";
import { appApi } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow } from "@/app/types";
import { cliName } from "@/app/nodes";
import { MessageContent } from "@/app/Markdown";
import { ErrorText } from "@/app/dialogs";
import { bytes, cn, timeAgo } from "@/lib/utils";

export interface ConversationTarget {
  hubId: string;
  hubName: string;
  threadId: string;
  agent: string;
}

const CONSOLE_PREFIX = "@console:";
const POLL_MS = 3000;

function isConsole(from: string): boolean {
  return from.startsWith(CONSOLE_PREFIX);
}

function clockTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
}

function isOpen(status: string): boolean {
  return status !== "done" && status !== "failed";
}

export function ConversationDetail({ target, rows, now, onBack, onNewConversation }: {
  target: ConversationTarget;
  rows: AgentRow[];
  now: number;
  onBack: () => void;
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
  const atBottom = useRef(true);

  const row = rows.find((r) => r.hub.id === target.hubId && r.st.agent.name === target.agent);
  const agent = row?.st.agent;
  const agentName = agent?.display_name || target.agent;
  const cli = agent ? cliName(agent) : "";
  const model = agent?.runtime?.model;

  useEffect(() => {
    let cancelled = false;
    setMessages(null);
    setLoadError(null);
    setSendError(null);
    setDraft("");
    setSteerMode(false);
    atBottom.current = true;
    const run = async (silent: boolean) => {
      try {
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
  }, [target.hubId, target.threadId]);

  useEffect(() => {
    const el = scrollRef.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [messages]);

  const onScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  };

  const consoleTasks = (messages || []).filter((m) => m.type === "task" && isConsole(m.from));
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
    ? { dot: "working", label: "处理中" }
    : failed
      ? { dot: "failed", label: "失败" }
      : lastTask
        ? { dot: "idle", label: "已完成" }
        : { dot: "idle", label: "空闲" };
  const steerOn = steerMode && !!openTask;

  // A thread you take no part in (two nodes working together) is only
  // shown: writing into it would land in their conversation, not yours.
  const observed = !!messages && messages.length > 0 &&
    !messages.some((m) => isConsole(m.from) || isConsole(m.to));
  const parties = observed
    ? [...new Set(messages.flatMap((m) => [m.from, m.to]))].filter((n) => n && !n.startsWith("@"))
    : [];

  const nameOf = (from: string): string => {
    if (isConsole(from)) return "你";
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

  return (
    <div className="conv-detail">
      <header className="conv-head">
        <button type="button" className="app-btn app-btn-plain" onClick={onBack} aria-label="返回对话列表" title="返回">
          <ArrowLeft />
        </button>
        <div className="conv-head-titles">
          <h2>{observed ? parties.map(nameOf).join(" ↔ ") : agentName}</h2>
          <p>
            {observed && <>其他节点之间的消息 · </>}
            {target.hubName}
            {cli && <> · {cli}</>}
            {model && <> · <span className="mono">{model}</span></>}
          </p>
        </div>
        {!observed && <span className="conv-head-state">
          <span className={cn("dot", state.dot)} />
          {messages === null ? "加载中" : state.label}
        </span>}
      </header>

      <div className="conv-scroll" ref={scrollRef} onScroll={onScroll}>
        <div className="conv-timeline">
          {messages === null ? (
            loadError ? (
              <ErrorText error={loadError} />
            ) : (
              <div className="app-empty-note">正在加载…</div>
            )
          ) : messages.length === 0 ? (
            <div className="app-empty-note">还没有消息。发一条任务开始。</div>
          ) : (
            messages.map((m) => {
              if (m.type === "progress") {
                return (
                  <div className="conv-progress" key={m.id}>
                    <span className="conv-progress-label">进度：</span>
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
              const mine = observed ? m.from === target.agent : isConsole(m.from);
              return (
                <div className={cn("conv-msg", mine ? "is-me" : "is-agent")} key={m.id}>
                  <div className="conv-msg-name">{nameOf(m.from)}</div>
                  <div className="conv-bubble">
                    {m.type === "steer" && <span className="conv-tag">修正</span>}
                    {m.type === "note" && <span className="conv-tag neutral">备注</span>}
                    <MessageContent body={m.body} />
                    {m.attachments && m.attachments.length > 0 && (
                      <div className="conv-attach-list">
                        {m.attachments.map((a) => (
                          <div className="conv-attach" key={a.file_id} title="下载暂不可用">
                            <Paperclip />
                            <span className="conv-attach-name">{a.name}</span>
                            <span className="conv-attach-size">{bytes(a.size)}</span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className={cn("conv-msg-meta", m.status === "failed" && "is-failed")}>
                    {m.status === "failed" && m.error ? `失败：${m.error}` : clockTime(m.created_at)}
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      {observed ? (
        <div className="conv-observe">
          <span>这是节点之间的协作记录，你只能查看。</span>
          <button type="button" className="app-btn" onClick={() => onNewConversation(target.hubId, target.agent)}>
            给 {agentName} 发任务
          </button>
        </div>
      ) : (
      <div className="conv-compose">
        {openTask && (
          <div className="conv-working">
            <span className="dot working" />
            <span>Agent 正在处理…</span>
            <button
              type="button"
              className={cn("conv-steer-btn", steerMode && "active")}
              aria-pressed={steerMode}
              onClick={() => setSteerMode((s) => !s)}
            >
              修正
            </button>
          </div>
        )}
        {steerOn && (
          <div className="conv-steer-hint">修正会打断当前处理，让 agent 按新的要求继续。</div>
        )}
        <div className="conv-compose-box">
          <textarea
            className="app-input"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder={steerOn ? "修改刚才的任务…" : `发给 ${agentName}…`}
            rows={2}
            disabled={sending}
            aria-label="消息"
          />
          <button
            type="button"
            className="app-btn app-btn-primary"
            onClick={() => void send()}
            disabled={sending || !draft.trim()}
          >
            {sending ? "发送中…" : steerOn ? "发送修正" : "发送"}
          </button>
        </div>
        <div className="conv-compose-hint">
          Enter 发送，Shift+Enter 换行
          {messages && messages.length > 0 && (
            <span className="conv-compose-ago">更新于 {timeAgo(messages[messages.length - 1]?.updated_at, now)}</span>
          )}
        </div>
        <ErrorText error={sendError} />
      </div>
      )}
    </div>
  );
}
