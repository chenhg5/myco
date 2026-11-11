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
 * `$` line with who sent it and when, its output below, and the exit code
 * when it failed. Notes and other messages are shown as `#` comment lines.
 */
export function Terminal({ messages, execNode, nameOf, hubE2E, live, cwd, children, hubId, onGrow, onStop, canStop }: {
  messages: Message[];
  /** the node's working directory, shown in front of each $ of a live session */
  cwd?: string;
  /** show a running command's output as it comes (from the run trace) */
  hubId?: string;
  onGrow?: () => void;
  /** offered on running commands you may stop */
  onStop?: (task: Message) => void;
  canStop?: (task: Message) => boolean;
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
  for (const m of messages) {
    if (used.has(m.id)) continue;
    if (m.type === "task" && m.to === execNode) {
      const a = answers.get(m.id);
      if (a) used.add(a.id);
      items.push(<Command key={m.id} task={m} answer={a} nameOf={nameOf} hubE2E={hubE2E} hubId={hubId} onGrow={onGrow} onStop={onStop && (!canStop || canStop(m)) ? onStop : undefined} plain={live} cwd={cwd} />);
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
  return <div className={cn("term", live && "is-live")}>{items}{children}</div>;
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

function Output({ answer, res, nameOf, plain }: { answer: Message; res: ExecResult; nameOf: (n: string) => string; plain?: boolean }) {
  const took = execDuration(res.duration_ms);
  // Your own session: what a command printed after its line left the screen
  // (cleared, or not loaded) is just more output, as in a terminal.
  if (plain) return <div className="term-cmd"><ResultLines res={res} /></div>;
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

export const STOPPED = "stopped by the sender";

function Command({ task, answer, nameOf, hubE2E, hubId, onGrow, onStop, plain, cwd }: {
  task: Message; answer?: Message; nameOf: (n: string) => string; hubE2E?: boolean;
  hubId?: string; onGrow?: () => void; onStop?: (task: Message) => void;
  /** your own session: as a terminal prints it (prompt, command, output), nothing around it */
  plain?: boolean; cwd?: string;
}) {
  const res = answer?.type === "reply" ? parseExecResult(answer.body) : null;
  const took = execDuration(res?.duration_ms);
  const pending = !answer && task.status !== "done" && task.status !== "failed";
  const stopped = !answer && task.status === "failed" && task.error === STOPPED;
  const failed = answer?.type === "event" || (!answer && task.status === "failed" && !stopped)
    || (typeof res?.exit_code === "number" && res.exit_code !== 0) || (typeof res?.error === "string" && !!res.error);
  if (plain) {
    // When, and how long it took, are a hover away; a terminal shows neither.
    const when = [stamp(task.created_at), took].filter(Boolean).join(" · ");
    return (
      <div className="term-cmd">
        <div className="term-line" title={when}>
          <span className="term-prompt">{cwd ? <span className="term-cwd">{cwd}</span> : null}$</span>{task.body}
        </div>
        {res && <ResultLines res={res} />}
        {answer && !res && <pre className={cn("term-out", answer.type === "event" && "is-err")}>{answer.body}</pre>}
        {(pending || stopped) && hubId && !task.e2e && <LiveOutput hubId={hubId} task={task} live={pending} onGrow={onGrow} />}
        {/* Running: a block cursor after what it has printed so far. Stopping
            is Ctrl+C, or the line under the terminal. */}
        {pending && <div className="term-pending"><span className="term-cursor" role="img" aria-label={L("执行中", "running")} /></div>}
        {stopped && <div className="term-exit is-stopped">^C</div>}
        {!answer && !pending && task.status === "failed" && !stopped && <div className="term-exit">{task.error || L("失败", "failed")}</div>}
      </div>
    );
  }
  return (
    <div className={cn("term-cmd", failed && "is-failed")}>
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
      {(pending || stopped) && hubId && !task.e2e && <LiveOutput hubId={hubId} task={task} live={pending} onGrow={onGrow} />}
      {pending && (
        <div className="term-pending">
          {L("执行中…", "running…")}
          {onStop && <button type="button" className="term-stop" onClick={() => onStop(task)}>{L("停止", "Stop")}</button>}
        </div>
      )}
      {stopped && <div className="term-exit is-stopped">{L("已停止", "stopped")}</div>}
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
  return <>{blocks.map((b, i) => <pre key={i} className={cn("term-out", b.err && "is-err")}>{b.text.replace(/\n$/, "")}</pre>)}</>;
}
