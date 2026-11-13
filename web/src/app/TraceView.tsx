import { useEffect, useRef, useState } from "react";
import { Brain, Check, ChevronRight, FileText, Globe, Loader2, Pencil, Search, SquareTerminal, Wrench, X } from "lucide-react";
import { appApi, isOldHub } from "@/appapi";
import type { Attachment, Message, TraceEvent, TraceStep } from "@/api";
import { cn } from "@/lib/utils";
import { recall, remember } from "@/lib/memo";
import { L, locale } from "@/lib/i18n";
import { Elapsed, Mushroom, useVerb } from "@/app/Working";
import { MessageContent } from "@/app/Markdown";
import { MessageAttachments } from "@/app/attachments";
import { AskCard } from "@/app/AskCard";

const POLL_MS = 1500;
const isOpen = (s: string) => s === "queued" || s === "delivered" || s === "acked" || s === "held";

/** A tool call with its result, or another step. */
type Row =
  | { kind: "tool"; key: string; at?: string; call: TraceStep; result?: TraceStep }
  | { kind: "thinking" | "text" | "error"; key: string; at?: string; step: TraceStep };

function rowsOf(events: TraceEvent[]): { rows: Row[]; usage?: TraceStep } {
  const rows: Row[] = [];
  const calls = new Map<string, Extract<Row, { kind: "tool" }>>();
  let usage: TraceStep | undefined;
  for (const e of events) {
    const s = e.data;
    if (s.kind === "tool") {
      const r = { kind: "tool" as const, key: `t${e.seq}`, at: e.at, call: s };
      rows.push(r);
      if (s.id) calls.set(s.id, r);
    } else if (s.kind === "tool_result") {
      const r = s.id ? calls.get(s.id) : undefined;
      if (r) r.result = s;
    } else if (s.kind === "usage") {
      usage = s;
    } else if (s.kind === "thinking" || s.kind === "text" || s.kind === "error") {
      rows.push({ kind: s.kind, key: `s${e.seq}`, at: e.at, step: s });
    }
  }
  return { rows, usage };
}

const READ = new Set(["Read", "NotebookRead"]);
const EDIT = new Set(["Edit", "Write", "MultiEdit", "NotebookEdit"]);
const SEARCH = new Set(["Grep", "Glob", "LS"]);
const WEB = new Set(["WebFetch", "WebSearch"]);

function toolIcon(tool = "") {
  if (tool === "Bash") return SquareTerminal;
  if (READ.has(tool)) return FileText;
  if (EDIT.has(tool)) return Pencil;
  if (SEARCH.has(tool)) return Search;
  if (WEB.has(tool)) return Globe;
  return Wrench;
}

function toolLabel(tool = ""): string {
  if (tool === "Bash") return L("运行", "Run");
  if (READ.has(tool)) return L("读取", "Read");
  if (EDIT.has(tool)) return L("修改", "Edit");
  if (SEARCH.has(tool)) return L("搜索", "Search");
  if (tool === "WebFetch") return L("打开网页", "Fetch");
  if (tool === "WebSearch") return L("搜索网页", "Web search");
  if (tool === "TodoWrite") return L("计划", "Plan");
  if (tool === "Task" || tool === "Agent") return L("子任务", "Subtask");
  return tool;
}

/** "读了 3 个文件、跑了 2 条命令、改了 1 个文件". */
function summaryOf(rows: Row[]): string {
  let read = 0, run = 0, edit = 0, other = 0, failed = 0;
  const files = new Set<string>();
  for (const r of rows) {
    if (r.kind !== "tool") continue;
    const t = r.call.tool || "";
    if (r.result?.failed) failed++;
    if (READ.has(t)) read++;
    else if (t === "Bash") run++;
    else if (EDIT.has(t)) { edit++; if (r.call.input) files.add(r.call.input); }
    else other++;
  }
  const parts: string[] = [];
  if (read) parts.push(L(`读了 ${read} 个文件`, `read ${read} file${read > 1 ? "s" : ""}`));
  if (run) parts.push(L(`跑了 ${run} 条命令`, `ran ${run} command${run > 1 ? "s" : ""}`));
  if (edit) parts.push(L(`改了 ${files.size || edit} 个文件`, `edited ${files.size || edit} file${(files.size || edit) > 1 ? "s" : ""}`));
  if (other) parts.push(L(`另外 ${other} 次操作`, `${other} other step${other > 1 ? "s" : ""}`));
  if (failed) parts.push(L(`${failed} 次失败`, `${failed} failed`));
  return parts.join(L("、", ", "));
}

/** One item of a plan the agent is working through. */
export type PlanItem = { text: string; state: "done" | "doing" | "todo" };

/** A plan written as a checklist: lines like "[x] done", "[~] doing",
 *  "[ ] to do", or a markdown task list ("- [x] …"). At least two items,
 *  every line one of them; otherwise it is not a plan. */
export function parsePlan(text: string): PlanItem[] | null {
  const lines = text.split("\n").map((l) => l.trim()).filter(Boolean);
  if (lines.length < 2) return null;
  const items: PlanItem[] = [];
  for (const l of lines) {
    const m = /^(?:[-*+]\s+)?\[( |x|X|~|-)\]\s+(.+)$/.exec(l);
    if (!m) return null;
    items.push({ text: m[2].trim(), state: m[1] === " " ? "todo" : m[1] === "~" || m[1] === "-" ? "doing" : "done" });
  }
  return items;
}

function usageLine(u?: TraceStep): string {
  if (!u) return "";
  const parts: string[] = [];
  if (u.duration_ms) {
    const s = Math.round(u.duration_ms / 1000);
    parts.push(s >= 60 ? L(`${Math.floor(s / 60)} 分 ${s % 60} 秒`, `${Math.floor(s / 60)}m ${s % 60}s`) : L(`${s} 秒`, `${s}s`));
  }
  const tokens = (u.input_tokens || 0) + (u.output_tokens || 0);
  if (tokens) parts.push(tokens >= 10000 ? L(`${(tokens / 10000).toFixed(1)} 万 token`, `${(tokens / 1000).toFixed(1)}k tokens`) : `${tokens} token${tokens > 1 ? "s" : ""}`);
  if (u.cost_usd) parts.push(`$${u.cost_usd.toFixed(2)}`);
  return parts.join(" · ");
}

/**
 * What the agent did for a task: what it said along the way, in the open,
 * and between, each run of steps (thinking, tool calls with their results)
 * as one line that opens, as the runner reported them. Live while the task
 * runs: the newest steps say what it is doing now.
 */
export function TraceView({ hubId, task, onGrow, waiting, answered, said, onUsage, answers, canAnswer, onAnswer }: {
  hubId: string; task: Message; onGrow?: () => void;
  /** where the finished run's time, tokens and cost go (next to the
   *  answer's time and copy); without it they end the run */
  onUsage?: (taskId: string, line: string) => void;
  /** what the agent reported on this task (myco message progress): part of
   *  the run, in time order, as it says things between its steps */
  said?: Message[];
  /** the steers that answered its questions, by question id */
  answers?: Map<string, Message>;
  /** you sent the task: you may answer its questions */
  canAnswer?: boolean;
  onAnswer?: (text: string) => Promise<void>;
  /** why it has not started yet, when known (the session is open elsewhere) */
  waiting?: string;
  /** its answer is in the conversation: done, even if its status lags */
  answered?: boolean;
}) {
  const running = isOpen(task.status) && !answered;
  // Waiting on a question to its sender: not at work, no loading.
  const asking = running && !!task.asking;
  // A finished run does not change: kept (and saved with the page data).
  const memoKey = `trace\u0000${hubId}\u0000${task.id}`;
  const [events, setEvents] = useState<TraceEvent[] | null>(() => (running ? null : recall<TraceEvent[]>(memoKey) ?? null));
  const [gone, setGone] = useState(false);
  const last = useRef(events?.length ? events[events.length - 1].seq : 0);
  const finalLoaded = useRef(!running && events !== null);
  const body = useRef<HTMLDivElement>(null);
  // New steps while live: follow them, inside the block and in the chat.
  useEffect(() => {
    if (!running || !events) return;
    const el = body.current;
    if (el) el.scrollTop = el.scrollHeight;
    onGrow?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [events?.length, said?.length, running]);

  // Loaded while it runs (polled) and once when it has finished, folded or
  // not: whether there is anything to show depends on it.
  useEffect(() => {
    if (gone || (!running && finalLoaded.current)) return;
    let off = false;
    const load = async () => {
      try {
        const r = await appApi.trace(hubId, task.id, last.current);
        if (off) return;
        if (!running) finalLoaded.current = true;
        if (r.events.length) {
          last.current = r.events[r.events.length - 1].seq;
          setEvents((prev) => {
            const next = [...(prev || []), ...r.events];
            if (!running) remember(memoKey, next);
            return next;
          });
        } else {
          setEvents((prev) => {
            const next = prev || [];
            if (!running) remember(memoKey, next);
            return next;
          });
        }
      } catch (e) {
        // A hub without traces, or a task you may not see the run of.
        if (!off && (isOldHub(e) || (e as { code?: string }).code === "not_found")) setGone(true);
      }
    };
    void load();
    if (!running) return () => { off = true; };
    const t = window.setInterval(() => void load(), POLL_MS);
    return () => { off = true; window.clearInterval(t); };
  }, [hubId, task.id, running, gone]);
  // While nothing says what it is doing: a playful verb (the pixel mushroom
  // is at work).
  const verb = useVerb(running);
  // A finished run's time, tokens and cost go with its answer.
  useEffect(() => {
    if (!onUsage || running || !events) return;
    const u = rowsOf(events).usage;
    if (u) onUsage(task.id, usageLine(u));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [events, running, task.id]);

// Not started yet (queued or delivered but not acknowledged: the node is
  // offline, its session is busy, or it has only pulled the task): a quiet
  // line saying why, not a spinner. Once it reports progress it is at it.
  const reports = said || [];
  if (running && task.status !== "acked" && !reports.length) {
    return (
      <div className="trace is-waiting">
        <div className="trace-head trace-wait">
          <Mushroom idle />
          <span className="trace-title">{waiting || L("排队中，轮到它时开始", "Queued: it starts when the node gets to it")}</span>
        </div>
      </div>
    );
  }
  // At work with no steps to show yet (none so far, or a hub that does not
  // keep them): the mushroom at work, a playful verb, and the time.
  const atWork = (
    <div className="trace is-live">
      <div className="trace-head trace-wait">
        <Mushroom />
        <span key={verb} className="trace-title is-shimmer is-verb">{verb}</span>
        <Elapsed since={task.created_at} className="trace-usage" />
      </div>
    </div>
  );
  const { rows, usage } = gone ? { rows: [] as Row[], usage: undefined } : rowsOf(events || []);
  const usageText = !running && usage ? usageLine(usage) : "";
  // The process is thinking, tool calls and errors. What the agent says is
  // the reply (shown as the message); text between steps stays as context,
  // text after the last step is the reply itself and is left out.
  const steps = rows.filter((x) => x.kind !== "text").length;
  let end = rows.length;
  while (end > 0 && rows[end - 1].kind === "text") end--;
  const shown = rows.slice(0, end);
  // Nothing but a reply (or no record at all): no process to show.
  if (!reports.length) {
    if (gone) return running ? atWork : null;
    if (!running && (events === null || steps === 0)) return null;
    if (running && steps === 0) return atWork;
  }

  // The plan it is working through, as Claude Code's to-do list and
  // Codex's plan are: the latest one, from its steps (TodoWrite) or what it
  // reported (a checklist in myco message progress), at the top of the run,
  // ticked off as it goes. A report that is only the plan is not repeated
  // in the timeline.
  let plan: { items: PlanItem[]; at: string } | null = null;
  const planReports = new Set<string>();
  for (const r of rows) {
    if (r.kind !== "tool" || r.call.tool !== "TodoWrite") continue;
    const items = parsePlan(r.call.input || "");
    if (items && (!plan || (r.at || "") >= plan.at)) plan = { items, at: r.at || "" };
  }
  for (const p of reports) {
    const items = parsePlan(p.body);
    if (!items) continue;
    planReports.add(p.id);
    if (!plan || p.created_at >= plan.at) plan = { items, at: p.created_at };
  }

  // As Claude Code and Codex show a run: what it says between steps is
  // text, in the open (what it reported with myco message progress too, by
  // time); each run of steps in between folds into one line ("跑了 3 条命令、
  // 改了 1 个文件") that opens to show them. The newest steps, while it runs,
  // say what it is doing now; at the very end, while it works, the mushroom.
  type Item = { at?: string; say?: { key: string; text: string; report?: boolean; when?: string; files?: Attachment[] }; ask?: Message; row?: Row };
  const items: Item[] = shown.map((r) => ({ at: r.at, row: r }));
  for (const p of reports) {
    if (planReports.has(p.id)) continue;
    const it: Item = p.ask ? { at: p.created_at, ask: p }
      : { at: p.created_at, say: { key: p.id, text: p.body, report: true, when: p.created_at, files: p.attachments } };
    // after the last step made before it (steps without a time: at the end)
    let i = items.length;
    if (items.every((x) => x.at)) {
      const t = new Date(p.created_at).getTime();
      i = items.findIndex((x) => new Date(x.at!).getTime() > t);
      if (i < 0) i = items.length;
    }
    items.splice(i, 0, it);
  }
  type Report = { key: string; text: string; when: string; files?: Attachment[] };
  type Segment = { kind: "say"; key: string; text: string }
    | { kind: "ask"; msg: Message }
    | { kind: "think"; key: string; text: string }
    | { kind: "reports"; items: Report[] }
    | { kind: "steps"; rows: Row[] };
  const segments: Segment[] = [];
  for (const it of items) {
    if (it.ask) { segments.push({ kind: "ask", msg: it.ask }); continue; }
    if (it.say?.report) {
      const lastSeg = segments[segments.length - 1];
      const r = { key: it.say.key, text: it.say.text, when: it.say.when || "", files: it.say.files };
      if (lastSeg?.kind === "reports") lastSeg.items.push(r);
      else segments.push({ kind: "reports", items: [r] });
      continue;
    }
    if (it.say) { segments.push({ kind: "say", key: it.say.key, text: it.say.text }); continue; }
    const r = it.row!;
    if (r.kind === "text") {
      if (r.step.text?.trim()) segments.push({ kind: "say", key: r.key, text: r.step.text });
      continue;
    }
    // Some models (DeepSeek, for one) say what they are doing only as
    // thinking: read as faded prose between the steps, not folded away.
    if (r.kind === "thinking") {
      if (r.step.text?.trim()) segments.push({ kind: "think", key: r.key, text: r.step.text });
      continue;
    }
    const lastSeg = segments[segments.length - 1];
    if (lastSeg?.kind === "steps") lastSeg.rows.push(r);
    else segments.push({ kind: "steps", rows: [r] });
  }
  const endsWithSay = segments[segments.length - 1]?.kind !== "steps";

  return (
    <div className={cn("trace is-flow", running && !asking && "is-live")} ref={body}>
      {plan && <PlanCard items={plan.items} live={running} />}
      {segments.map((seg, i) => seg.kind === "say" ? (
        <div key={seg.key} className="trace-say">
          <MessageContent body={seg.text} />
        </div>
      ) : seg.kind === "think" ? (
        <Thought key={seg.key} text={seg.text} />
      ) : seg.kind === "ask" ? (
        <AskCard key={seg.msg.id} hubId={hubId} ask={seg.msg} answer={answers?.get(seg.msg.id)}
          open={asking && task.asking === seg.msg.id} canAnswer={!!canAnswer} onAnswer={onAnswer} />
      ) : seg.kind === "reports" ? (
        // What it reported (myco message progress): a small timeline of its
        // own, quieter than the answer, each with when.
        <ol key={seg.items[0].key} className="trace-reports" aria-label={L("进展", "Progress")}>
          {seg.items.map((r, j) => (
            <li key={r.key} className={cn("trace-report", running && i === segments.length - 1 && j === seg.items.length - 1 && "is-latest")}>
              <span className="trace-report-dot" aria-hidden />
              <div className="trace-report-text">
                <MessageContent body={r.text} />
                {/* What it showed along the way: a screenshot, a draft. */}
                {r.files && r.files.length > 0 && <div className="trace-report-files"><MessageAttachments hubId={hubId} items={r.files} /></div>}
              </div>
              {r.when && <time className="trace-report-time" dateTime={r.when}>{clock(r.when)}</time>}
            </li>
          ))}
        </ol>
      ) : (
        <StepGroup key={seg.rows[0].key} rows={seg.rows} live={running && i === segments.length - 1} since={task.created_at} />
      ))}
      {running && !asking && endsWithSay && (
        <div className="trace-head trace-wait">
          <Mushroom />
          <span key={verb} className="trace-title is-shimmer is-verb">{verb}</span>
          <Elapsed since={task.created_at} className="trace-usage" />
        </div>
      )}
      {usageText && !onUsage && <div className="trace-foot">{usageText}</div>}
    </div>
  );
}

function clock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit", hour12: false });
}

/** A run of steps between two things it said: one line, open to see them.
 *  The newest, while it runs, says what it is doing now. */
/** A thinking step as faded prose: the first lines, all of it on a click. */
function Thought({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  const body = text.trim();
  const long = body.length > 280 || body.split("\n").length > 4;
  return (
    <div className={cn("trace-say trace-think", long && !open && "is-clamped", long && "is-long")}
      title={long && !open ? L("点开看完整的思考", "Show the whole thought") : undefined}
      onClick={long ? () => setOpen((o) => !o) : undefined}>
      {body}
    </div>
  );
}

function StepGroup({ rows, live, since }: { rows: Row[]; live: boolean; since: string }) {
  const [open, setOpen] = useState(false);
  const last = rows[rows.length - 1];
  const doing = !last ? L("正在处理", "Working")
    : last.kind === "tool" ? `${toolLabel(last.call.tool)} ${(last.call.input || "").split("\n")[0]}`.trim()
    : last.kind === "thinking" ? L("正在思考", "Thinking")
    : L("正在处理", "Working");
  const head = live ? doing : summaryOf(rows) || (last?.kind === "thinking" ? L("思考了一下", "Thought it over") : L("过程", "Steps"));
  return (
    <div className={cn("trace-group", open && "is-open", live && "is-live")}>
      <button type="button" className="trace-head" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        {live ? <Mushroom /> : <ChevronRight className="trace-chev" />}
        <span className={cn("trace-title", live && "is-shimmer")}>{head}</span>
        {live && <Elapsed since={since} className="trace-usage" />}
      </button>
      {open && (
        <div className="trace-body">
          {rows.map((r) => <TraceRow key={r.key} row={r} live={live} />)}
        </div>
      )}
    </div>
  );
}

function TraceRow({ row, live }: { row: Row; live: boolean }) {
  const [open, setOpen] = useState(false);
  if (row.kind === "thinking") {
    return (
      <div className={cn("trace-row is-thinking", open && "is-open")}>
        <button type="button" className="trace-line" onClick={() => setOpen((o) => !o)}>
          <Brain /><span className="trace-name">{L("思考", "Thinking")}</span>
          {!open && row.step.text && <span className="trace-arg">{row.step.text.split("\n")[0]}</span>}
        </button>
        {open && row.step.text && <div className="trace-detail is-prose">{row.step.text}</div>}
      </div>
    );
  }
  if (row.kind === "text") {
    return row.step.text ? <div className="trace-row is-text">{row.step.text}</div> : null;
  }
  if (row.kind === "error") {
    return <div className="trace-row is-error"><X />{row.step.text}</div>;
  }
  const tool = row as Extract<Row, { kind: "tool" }>;
  const Icon = toolIcon(tool.call.tool);
  const r = tool.result;
  return (
    <div className={cn("trace-row is-tool", open && "is-open", r?.failed && "is-failed")}>
      <button type="button" className="trace-line" onClick={() => setOpen((o) => !o)} disabled={!r?.output && !tool.call.input}>
        <Icon />
        <span className="trace-name">{toolLabel(tool.call.tool)}</span>
        {tool.call.input && <span className="trace-arg mono">{tool.call.input.split("\n")[0]}</span>}
        <span className="trace-state">
          {r ? (r.failed ? <X className="is-bad" /> : null) : live ? <Loader2 className="trace-spin" /> : null}
        </span>
      </button>
      {open && (
        <div className="trace-detail">
          {tool.call.input && tool.call.input.includes("\n") && <pre className="trace-pre">{tool.call.input}</pre>}
          {r?.output ? <pre className="trace-pre">{r.output}</pre> : <div className="trace-note">{L("没有输出", "No output")}</div>}
        </div>
      )}
    </div>
  );
}

/** The plan: a short list being ticked off. Done items are checked and
 *  struck through; the one in hand is marked while it runs. */
function PlanCard({ items, live }: { items: PlanItem[]; live: boolean }) {
  const done = items.filter((x) => x.state === "done").length;
  return (
    <div className="trace-plan">
      <div className="trace-plan-head">
        <span>{L("计划", "Plan")}</span>
        <span className="trace-plan-count">{L(`${done} / ${items.length} 完成`, `${done} of ${items.length} done`)}</span>
      </div>
      <ul className="trace-plan-list">
        {items.map((x, i) => (
          <li key={i} className={cn("trace-plan-item", `is-${x.state}`, live && x.state === "doing" && "is-live")}>
            <span className="trace-plan-box" aria-hidden>{x.state === "done" && <Check />}</span>
            <span className="trace-plan-text">{x.text}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
