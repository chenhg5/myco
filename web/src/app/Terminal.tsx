import { useEffect, useRef, useState } from "react";
import { Lock } from "lucide-react";
import type { Message, TraceStep } from "@/api";
import { appApi, isOldHub } from "@/appapi";
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
 * prompt line with its output below. Your own session (live) shows nothing
 * else and ends with the prompt you type at; a record of other nodes'
 * commands ends each command line with who ran it, when and how it ended.
 * Notes and other messages are `#` comment lines.
 */
export function Terminal({ messages, execNode, nameOf, hubE2E, live, cwd, children, hubId, onGrow }: {
  messages: Message[];
  /** the node's working directory, shown in each prompt of a live session */
  cwd?: string;
  /** show a running command's output as it comes (from the run trace) */
  hubId?: string;
  onGrow?: () => void;
  /** a session you type into: fills the space, ends with the prompt (children) */
  live?: boolean;
  children?: React.ReactNode;
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
  // The prompt after a failed command is red, as shell prompts do it; in a
  // live session the last one colours the input line's prompt.
  let afterFail = false;
  for (const m of messages) {
    if (used.has(m.id)) continue;
    if (m.type === "task" && m.to === execNode) {
      const a = answers.get(m.id);
      if (a) used.add(a.id);
      items.push(<Command key={m.id} task={m} answer={a} nameOf={nameOf} hubE2E={hubE2E} hubId={hubId} onGrow={onGrow} plain={live} cwd={cwd} afterFail={afterFail} />);
      afterFail = didFail(m, a);
      continue;
    }
    if (m.type === "progress" || m.type === "steer") continue;
    // A result whose command is further back (not loaded yet): show it as
    // output, not as its raw JSON.
    if (m.from === execNode && m.type === "reply" && !m.unreadable) {
      const res = parseExecResult(m.body);
      if (res) {
        items.push(<Output key={m.id} answer={m} res={res} nameOf={nameOf} plain={live} />);
        continue;
      }
    }
    if (m.unreadable) {
      items.push(
        <div className="term-comment" key={m.id}>
          <div className="term-line"><span className="term-text"># {L("已加密，只有双方的主人能看", "encrypted: only the owners of both sides can read it")}</span><span className="term-note">{nameOf(m.from)} · {stamp(m.created_at)}</span></div>
        </div>,
      );
      continue;
    }
    items.push(
      <div className="term-comment" key={m.id}>
        {m.body.split("\n").map((l, i) => (
          <div className="term-line" key={i}><span className="term-text"># {l}</span>{i === 0 && <span className="term-note">{nameOf(m.from)} · {stamp(m.created_at)}</span>}</div>
        ))}
      </div>,
    );
  }
  return <div className={cn("term", live && "is-live", live && afterFail && "is-after-fail")}>{items}{children}</div>;
}

/** Whether a command ended badly (a non-zero exit, an error, a failure that
 * was not you stopping it). */
function didFail(task: Message, answer?: Message): boolean {
  const res = answer?.type === "reply" ? parseExecResult(answer.body) : null;
  const stopped = !answer && task.status === "failed" && task.error === STOPPED;
  return answer?.type === "event" || (!answer && task.status === "failed" && !stopped)
    || (typeof res?.exit_code === "number" && res.exit_code !== 0) || (typeof res?.error === "string" && !!res.error);
}

type ExecResult = NonNullable<ReturnType<typeof parseExecResult>>;

/** What a command printed, as a terminal prints it: stderr is not coloured
 * and the exit code is not announced (the next prompt turns red; a record of
 * someone else's commands says it at the end of the command's line). */
function ResultLines({ res }: { res: ExecResult }) {
  return (
    <>
      {res.stdout ? <pre className="term-out">{res.stdout.replace(/\n$/, "")}</pre> : null}
      {res.stderr ? <pre className="term-out">{res.stderr.replace(/\n$/, "")}</pre> : null}
      {typeof res.error === "string" && res.error && <pre className="term-out">{res.error}</pre>}
    </>
  );
}

/** A result whose command line is not on screen (cleared, or further back
 * than what is loaded): in your own session it is just more output; in a
 * record, a dim line says where the command is. */
function Output({ answer, res, nameOf, plain }: { answer: Message; res: ExecResult; nameOf: (n: string) => string; plain?: boolean }) {
  const took = execDuration(res.duration_ms);
  return (
    <div className="term-cmd">
      {!plain && (
        <div className="term-line term-earlier">
          <span className="term-text"># {L("命令在更早的消息里", "the command is in earlier messages")}</span>
          <span className="term-note">{[nameOf(answer.from), stamp(answer.created_at), took].filter(Boolean).join(" · ")}</span>
        </div>
      )}
      <ResultLines res={res} />
    </div>
  );
}

export const STOPPED = "stopped by the sender";

/** The prompt's sign in your own session: a chevron, drawn so that it looks
 * the same in every monospace font (few of them have one), one character
 * wide. Red after a command that failed. */
export function PromptSign({ bad }: { bad?: boolean }) {
  return (
    <svg className={cn("term-sign", bad && "is-bad")} viewBox="0 0 8 14" aria-hidden="true">
      <path d="M1.75 3.25 5.75 7l-4 3.75" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function Command({ task, answer, nameOf, hubE2E, hubId, onGrow, plain, cwd, afterFail }: {
  task: Message; answer?: Message; nameOf: (n: string) => string; hubE2E?: boolean;
  hubId?: string; onGrow?: () => void;
  /** your own session: nothing but the prompt, the command and its output.
   * Otherwise it is a record of someone's commands, and each command line
   * ends with who ran it, when, how long it took and how it ended. */
  plain?: boolean; cwd?: string;
  /** the command before this one failed: its prompt is red */
  afterFail?: boolean;
}) {
  const res = answer?.type === "reply" ? parseExecResult(answer.body) : null;
  const took = execDuration(res?.duration_ms);
  const pending = !answer && task.status !== "done" && task.status !== "failed";
  const stopped = !answer && task.status === "failed" && task.error === STOPPED;
  const code = typeof res?.exit_code === "number" && res.exit_code !== 0
    ? L(`退出码 ${res.exit_code}`, `exit ${res.exit_code}`) + (res.timed_out ? L("（超时）", " (timed out)") : "") : "";
  // In your own session these are a hover away; a terminal shows none of them.
  const when = [stamp(task.created_at), took, code].filter(Boolean).join(" · ");
  return (
    <div className="term-cmd">
      <div className="term-line" title={plain ? when : undefined}>
        <span className="term-text">
          <span className="term-prompt">{cwd ? <span className="term-cwd">{cwd}</span> : null}<PromptSign bad={afterFail} /></span>{task.body}
        </span>
        {!plain && (
          <span className="term-note">
            {task.e2e && <Lock className="term-lock" aria-label={L("端到端加密", "End-to-end encrypted")} />}
            {[nameOf(task.from), stamp(task.created_at), took].filter(Boolean).join(" · ")}
            {!task.e2e && hubE2E && <span className="term-plain">{L("未加密", "unencrypted")}</span>}
            {code && <span className="term-code">{code}</span>}
          </span>
        )}
      </div>
      {res && <ResultLines res={res} />}
      {answer && !res && <pre className="term-out">{answer.body}</pre>}
      {(pending || stopped) && hubId && !task.e2e && <LiveOutput hubId={hubId} task={task} live={pending} onGrow={onGrow} />}
      {/* Running: a block cursor after what it has printed so far. Stopping
          is Ctrl+C, or the line under the terminal. */}
      {pending && <div className="term-pending"><span className="term-cursor" role="img" aria-label={L("执行中", "running")} /></div>}
      {stopped && <div className="term-exit">^C</div>}
      {!answer && !pending && task.status === "failed" && !stopped && <div className="term-exit">{task.error || L("失败", "failed")}</div>}
    </div>
  );
}

const LIVE_POLL_MS = 1000;

/** A running command's output so far (or a stopped one's), from its run trace. */
function LiveOutput({ hubId, task, live, onGrow }: { hubId: string; task: Message; live: boolean; onGrow?: () => void }) {
  const [chunks, setChunks] = useState<TraceStep[]>([]);
  const [gone, setGone] = useState(false);
  const last = useRef(0);
  useEffect(() => {
    if (gone) return;
    let off = false;
    const load = async () => {
      try {
        const r = await appApi.trace(hubId, task.id, last.current);
        if (off || !r.events.length) return;
        last.current = r.events[r.events.length - 1].seq;
        const out = r.events.map((e) => e.data).filter((d) => d.kind === "output" && d.text);
        if (out.length) setChunks((prev) => [...prev, ...out]);
      } catch (e) {
        // A hub or runner without traces: the output comes with the result.
        const code = (e as { code?: string }).code;
        if (!off && (isOldHub(e) || code === "not_found" || code === "permission_denied")) setGone(true);
      }
    };
    void load();
    if (!live) return () => { off = true; };
    const t = window.setInterval(() => void load(), LIVE_POLL_MS);
    return () => { off = true; window.clearInterval(t); };
  }, [hubId, task.id, gone, live]);
  useEffect(() => { if (chunks.length) onGrow?.(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [chunks.length]);
  if (!chunks.length) return null;
  // Consecutive chunks of one stream read as one block.
  const blocks: { err: boolean; text: string }[] = [];
  for (const c of chunks) {
    const err = c.stream === "stderr";
    const b = blocks[blocks.length - 1];
    if (b && b.err === err) b.text += c.text;
    else blocks.push({ err, text: c.text || "" });
  }
  return <>{blocks.map((b, i) => <pre key={i} className="term-out">{b.text.replace(/\n$/, "")}</pre>)}</>;
}
