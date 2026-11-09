import { Lock } from "lucide-react";
import type { Message } from "@/api";
import { execDuration, parseExecResult } from "@/app/Markdown";
import { cn } from "@/lib/utils";
import { L, locale } from "@/lib/i18n";

function stamp(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(locale(), { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false });
}

/**
 * Messages with an exec node shown as a terminal session: each command a
 * `$` line with who sent it and when, its output below, and the exit code
 * when it failed. Notes and other messages are shown as `#` comment lines.
 */
export function Terminal({ messages, execNode, nameOf, hubE2E }: {
  messages: Message[];
  execNode: string;
  nameOf: (name: string) => string;
  /** the hub encrypts: mark commands that were sent unencrypted */
  hubE2E?: boolean;
}) {
  // Answers (reply, or the failure event) by the command they answer.
  // (reply_to, or else the exec node's first answer in the same thread).
  const answers = new Map<string, Message>();
  const byThread = new Map<string, Message>();
  for (const m of messages) {
    if (m.type !== "reply" && m.type !== "event") continue;
    if (m.reply_to) answers.set(m.reply_to, m);
    else if (m.from === execNode && !byThread.has(m.thread_id)) byThread.set(m.thread_id, m);
  }
  for (const m of messages) {
    if (m.type === "task" && m.to === execNode && !answers.has(m.id) && byThread.has(m.thread_id)) {
      answers.set(m.id, byThread.get(m.thread_id)!);
      byThread.delete(m.thread_id);
    }
  }
  const used = new Set<string>();
  const items: React.ReactNode[] = [];
  for (const m of messages) {
    if (used.has(m.id)) continue;
    if (m.type === "task" && m.to === execNode) {
      const a = answers.get(m.id);
      if (a) used.add(a.id);
      items.push(<Command key={m.id} task={m} answer={a} nameOf={nameOf} hubE2E={hubE2E} />);
      continue;
    }
    if (m.type === "progress" || m.type === "steer") continue;
    // A result whose command is further back (not loaded yet): show it as
    // output, not as its raw JSON.
    if (m.from === execNode && m.type === "reply" && !m.unreadable) {
      const res = parseExecResult(m.body);
      if (res) {
        items.push(<Output key={m.id} answer={m} res={res} nameOf={nameOf} />);
        continue;
      }
    }
    if (m.unreadable) {
      items.push(<div className="term-comment" key={m.id}><span className="term-meta">{nameOf(m.from)} · {stamp(m.created_at)}</span><div># {L("已加密，只有双方的主人能看", "encrypted: only the owners of both sides can read it")}</div></div>);
      continue;
    }
    items.push(
      <div className="term-comment" key={m.id}>
        <span className="term-meta">{nameOf(m.from)} · {stamp(m.created_at)}</span>
        {m.body.split("\n").map((l, i) => <div key={i}># {l}</div>)}
      </div>,
    );
  }
  return <div className="term">{items}</div>;
}

type ExecResult = NonNullable<ReturnType<typeof parseExecResult>>;

function ResultLines({ res }: { res: ExecResult }) {
  const code = typeof res.exit_code === "number" ? res.exit_code : undefined;
  return (
    <>
      {res.stdout ? <pre className="term-out">{res.stdout.replace(/\n$/, "")}</pre> : null}
      {res.stderr ? <pre className="term-out is-err">{res.stderr.replace(/\n$/, "")}</pre> : null}
      {typeof res.error === "string" && res.error && <pre className="term-out is-err">{res.error}</pre>}
      {typeof code === "number" && code !== 0 && (
        <div className="term-exit">{L(`退出码 ${code}`, `exit ${code}`)}{res.timed_out ? L("（超时）", " (timed out)") : ""}</div>
      )}
    </>
  );
}

function Output({ answer, res, nameOf }: { answer: Message; res: ExecResult; nameOf: (n: string) => string }) {
  const took = execDuration(res.duration_ms);
  return (
    <div className="term-cmd">
      <div className="term-meta">
        <span>{nameOf(answer.from)} · {stamp(answer.created_at)}</span>
        {took && <span>{took}</span>}
      </div>
      <div className="term-line term-earlier"># {L("命令在更早的消息里", "the command is in earlier messages")}</div>
      <ResultLines res={res} />
    </div>
  );
}

function Command({ task, answer, nameOf, hubE2E }: { task: Message; answer?: Message; nameOf: (n: string) => string; hubE2E?: boolean }) {
  const res = answer?.type === "reply" ? parseExecResult(answer.body) : null;
  const took = execDuration(res?.duration_ms);
  const pending = !answer && task.status !== "done" && task.status !== "failed";
  return (
    <div className="term-cmd">
      <div className="term-meta">
        <span>
          {task.e2e && <Lock className="term-lock" aria-label={L("端到端加密", "End-to-end encrypted")} />}
          {nameOf(task.from)} · {stamp(task.created_at)}
          {!task.e2e && hubE2E && <span className="term-plain">{L("未加密", "unencrypted")}</span>}
        </span>
        {took && <span>{took}</span>}
      </div>
      <div className="term-line"><span className="term-prompt">$</span>{task.body}</div>
      {res && <ResultLines res={res} />}
      {answer && !res && <pre className={cn("term-out", answer.type === "event" && "is-err")}>{answer.body}</pre>}
      {pending && <div className="term-pending">{L("执行中…", "running…")}</div>}
      {!answer && !pending && task.status === "failed" && <div className="term-exit">{task.error || L("失败", "failed")}</div>}
    </div>
  );
}
