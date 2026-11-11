import { useEffect, useRef, useState } from "react";
import { Brain, ChevronRight, FileText, Globe, Loader2, Pencil, Search, SquareTerminal, Wrench, X } from "lucide-react";
import { appApi, isOldHub } from "@/appapi";
import type { Message, TraceEvent, TraceStep } from "@/api";
import { cn } from "@/lib/utils";
import { L } from "@/lib/i18n";

const POLL_MS = 1500;
const isOpen = (s: string) => s === "queued" || s === "delivered" || s === "acked" || s === "held";

/** A tool call with its result, or another step. */
type Row =
  | { kind: "tool"; key: string; call: TraceStep; result?: TraceStep }
  | { kind: "thinking" | "text" | "error"; key: string; step: TraceStep };

function rowsOf(events: TraceEvent[]): { rows: Row[]; usage?: TraceStep } {
  const rows: Row[] = [];
  const calls = new Map<string, Extract<Row, { kind: "tool" }>>();
  let usage: TraceStep | undefined;
  for (const e of events) {
    const s = e.data;
    if (s.kind === "tool") {
      const r = { kind: "tool" as const, key: `t${e.seq}`, call: s };
      rows.push(r);
      if (s.id) calls.set(s.id, r);
    } else if (s.kind === "tool_result") {
      const r = s.id ? calls.get(s.id) : undefined;
      if (r) r.result = s;
    } else if (s.kind === "usage") {
      usage = s;
    } else if (s.kind === "thinking" || s.kind === "text" || s.kind === "error") {
      rows.push({ kind: s.kind, key: `s${e.seq}`, step: s });
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
 * What the agent did for a task you gave it: thinking, tool calls with their
 * results, and what it said, as the runner reported them. Live while the task
 * runs; folded into one line afterwards (loaded when opened).
 */
export function TraceView({ hubId, task, onGrow, waiting }: {
  hubId: string; task: Message; onGrow?: () => void;
  /** why it has not started yet, when known (the session is open elsewhere) */
  waiting?: string;
}) {
  const running = isOpen(task.status);
  const [events, setEvents] = useState<TraceEvent[] | null>(null);
  const [open, setOpen] = useState(running);
  const [gone, setGone] = useState(false);
  const last = useRef(0);
  const body = useRef<HTMLDivElement>(null);
  // New steps while live: follow them, inside the block and in the chat.
  useEffect(() => {
    if (!running || !events) return;
    const el = body.current;
    if (el) el.scrollTop = el.scrollHeight;
    onGrow?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [events?.length, running]);

  // Open while it runs; when it finishes, fold to the one-line summary.
  // A beat later, so the last steps and the usage line (fetched once more
  // when it stops) are in before the block folds.
  const wasRunning = useRef(running);
  useEffect(() => {
    const was = wasRunning.current;
    wasRunning.current = running;
    if (running) { setOpen(true); return; }
    if (!was) return;
    const t = window.setTimeout(() => setOpen(false), 1200);
    return () => window.clearTimeout(t);
  }, [running]);

  useEffect(() => {
    if (!open || gone) return;
    let off = false;
    const load = async () => {
      try {
        const r = await appApi.trace(hubId, task.id, last.current);
        if (off) return;
        if (r.events.length) {
          last.current = r.events[r.events.length - 1].seq;
          setEvents((prev) => [...(prev || []), ...r.events]);
        } else {
          setEvents((prev) => prev || []);
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
  }, [hubId, task.id, open, running, gone]);

  // Not picked up yet (queued, the node offline or its session busy): a
  // quiet line saying why, not a spinner.
  if (running && task.status !== "acked" && (!events || events.length === 0)) {
    return (
      <div className="trace is-waiting">
        <div className="trace-head trace-wait">
          <span className="trace-wait-dot" />
          <span className="trace-title">{waiting || L("排队中，轮到它时开始", "Queued: it starts when the node gets to it")}</span>
        </div>
      </div>
    );
  }
  if (gone) return null;
  const { rows, usage } = rowsOf(events || []);
  // Finished without any recorded step: nothing to show (older runner, or a
  // node that handles messages in its own session).
  if (!running && events !== null && rows.length === 0) return null;

  const summary = summaryOf(rows);
  // While it runs, the line says what it is doing right now.
  const lastRow = rows[rows.length - 1];
  const doing = !lastRow ? L("正在处理", "Working")
    : lastRow.kind === "tool" ? `${toolLabel(lastRow.call.tool)} ${(lastRow.call.input || "").split("\n")[0]}`.trim()
    : lastRow.kind === "thinking" ? L("正在思考", "Thinking")
    : L("正在处理", "Working");
  const head = running
    ? doing
    : events === null ? L("查看过程", "Show what it did")
    : summary || L("过程", "What it did");

  return (
    <div className={cn("trace", running && "is-live", open && "is-open")}>
      <button type="button" className="trace-head" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        {running ? <Loader2 className="trace-spin" /> : <ChevronRight className="trace-chev" />}
        <span className={cn("trace-title", running && "is-shimmer")}>{head}</span>
        {!running && usage && <span className="trace-usage">{usageLine(usage)}</span>}
      </button>
      {open && (
        <div className="trace-body" ref={body}>
          {events === null && <div className="trace-note">{L("正在读取…", "Loading…")}</div>}
          {events !== null && rows.length === 0 && running && <div className="trace-note">{L("开始了，等第一步…", "Started; waiting for the first step…")}</div>}
          {rows.map((r) => <TraceRow key={r.key} row={r} live={running} />)}
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
