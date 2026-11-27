import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { appApi } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { MessageContent } from "@/app/Markdown";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * What a workflow says in a chat (internal/app/workflows.go), read as
 * itself instead of a wall of text. Two kinds, the message's `workflow`
 * holding the run:
 *
 * A step, the task it gives an agent:
 *   [工作流「myco-release」· 第 6 步 finish · 第 2 次]
 *
 *   <what this step is to do>
 *
 *   ---
 *   这是工作流里的一步：…  (the input as JSON, how to hand back a result)
 *
 * The end, the note to whoever started the run:
 *   [工作流「myco-release」完成了]          (or 失败了：why, 取消了)
 *   运行 run_…
 *   - select（wsl-shell）：退出码 0
 *   详情：myco workflow run show --id run_…
 *
 * Both are quiet: a small grey line saying which workflow (and step), a way
 * to the run beside it, then what matters; the rest folded. No icon, no
 * coloured badges: words in the theme's greys.
 */

export type WorkflowNote = {
  name: string;
  status: "succeeded" | "failed" | "cancelled";
  why: string;
  runId: string;
  steps: { label: string; target: string; line: string }[];
};

export type WorkflowStep = {
  name: string;
  n: number;
  label: string;
  attempt: number;
  /** what the step is to do (the workflow's prompt for it) */
  prompt: string;
  /** the workflow's own instructions, its input and the result's shape */
  rest: string;
};

const STATUS: Record<string, WorkflowNote["status"]> = { "完成了": "succeeded", "失败了": "failed", "取消了": "cancelled" };

/** The end note's parts, or null when it is not one (then it shows as written). */
export function parseWorkflowNote(body: string): WorkflowNote | null {
  // (its run on the same line as the head reads the same)
  const lines = body.replace(/\r/g, "").replace(/^(\[工作流「[^\n]*?\])[ \t]+(运行 \S+)/, "$1\n$2").split("\n");
  const head = /^\[工作流「(.+?)」(完成了|失败了|取消了)(?:：(.*))?\]$/.exec((lines[0] || "").trim());
  const run = /^运行 (\S+)$/.exec((lines[1] || "").trim());
  if (!head || !run) return null;
  const steps: WorkflowNote["steps"] = [];
  for (const raw of lines.slice(2)) {
    const l = raw.trim();
    if (!l || l.startsWith("详情：")) continue;
    const m = /^- (.*)（([^（）]*)）：(.*)$/.exec(l);
    if (m) steps.push({ label: m[1], target: m[2], line: m[3] });
    else if (steps.length) steps[steps.length - 1].line += `\n${l}`;
  }
  return { name: head[1], status: STATUS[head[2]], why: head[3] || "", runId: run[1], steps };
}

/** A step task's parts, or null when it is not one. */
export function parseWorkflowStep(body: string): WorkflowStep | null {
  const text = body.replace(/\r/g, "");
  const nl = text.indexOf("\n");
  const head = /^\[工作流「(.+?)」· 第 (\d+) 步 (.+?)(?: · 第 (\d+) 次)?\]$/.exec((nl < 0 ? text : text.slice(0, nl)).trim());
  if (!head) return null;
  const after = nl < 0 ? "" : text.slice(nl + 1);
  const cut = after.search(/\n-{3,}\n这是工作流里的一步/);
  return {
    name: head[1], n: Number(head[2]), label: head[3], attempt: Number(head[4] || 1),
    prompt: (cut < 0 ? after : after.slice(0, cut)).trim(),
    rest: cut < 0 ? "" : after.slice(cut).replace(/^\n-{3,}\n/, "").trim(),
  };
}

const outcome = (s: WorkflowNote["status"]) =>
  s === "succeeded" ? L("完成了", "Finished") : s === "failed" ? L("失败了", "Failed") : L("取消了", "Cancelled");

/** A step's line that says it did not go well (a failure or a timeout). */
const badLine = (line: string) => /^(超时了|失败|出错|运行结束了|取消)|timed out|failed|error/i.test(line);

/** The small grey line on top: which workflow (and step), and its run. */
function Head({ hubId, runId, words }: { hubId: string; runId: string; words: string }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  // Its run on the 工作流 page: the run says which workflow it belongs to.
  const openRun = async () => {
    setBusy(true);
    setError(null);
    try {
      const run = await appApi.workflowRun(hubId, runId);
      window.dispatchEvent(new CustomEvent("myco-open-notice", { detail: { hubId, target: `workflow:${run.workflow_id}?run=${encodeURIComponent(runId)}` } }));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <div className="wfn-head">
        <span className="wfn-where" title={words}>{words}</span>
        <button type="button" className="wfn-open" onClick={() => void openRun()} disabled={busy} data-testid="workflow-note-open">
          {busy ? L("正在打开…", "Opening…") : L("打开这次运行", "Open the run")}<ChevronRight />
        </button>
      </div>
      <ErrorText error={error} />
    </>
  );
}

function Fold({ open, onToggle, children, testId }: { open: boolean; onToggle: () => void; children: React.ReactNode; testId: string }) {
  return (
    <button type="button" className="wfn-fold" aria-expanded={open} onClick={onToggle} data-testid={testId}>
      <ChevronDown className="wfn-chev" />{children}
    </button>
  );
}

/** A run ended. */
export function WorkflowNoteCard({ hubId, note }: { hubId: string; note: WorkflowNote }) {
  const [open, setOpen] = useState(false);
  const bad = note.steps.filter((s) => badLine(s.line)).length;
  return (
    <div className={cn("wfn", `is-${note.status}`)} data-testid="workflow-note">
      <Head hubId={hubId} runId={note.runId} words={L(`工作流 · ${note.name}`, `Workflow · ${note.name}`)} />
      <div className="wfn-outcome">
        {L(`「${note.name}」${outcome(note.status)}`, `“${note.name}” ${outcome(note.status).toLowerCase()}`)}
        {note.why && <span className="wfn-why">{L(`：${note.why}`, `: ${note.why}`)}</span>}
      </div>
      {note.steps.length > 0 && (
        <>
          <Fold open={open} onToggle={() => setOpen((o) => !o)} testId="workflow-note-steps">
            <span>{L(`${note.steps.length} 步`, `${note.steps.length} steps`)}</span>
            {bad > 0 && <span className="wfn-bad">{L(` · ${bad} 步没顺利完成`, ` · ${bad} did not go well`)}</span>}
          </Fold>
          {open && (
            <ol className="wfn-steps">
              {note.steps.map((s, i) => (
                <li key={i} className={cn(badLine(s.line) && "is-bad")}>
                  <span className="wfn-step-label">{s.label}</span>
                  <span className="wfn-step-who">{s.target}</span>
                  <span className="wfn-step-line">{s.line}</span>
                </li>
              ))}
            </ol>
          )}
        </>
      )}
    </div>
  );
}

/** A step given to an agent: what it is to do; the workflow's own
 *  instructions, its input and the result's shape folded. */
export function WorkflowStepCard({ hubId, runId, step }: { hubId: string; runId: string; step: WorkflowStep }) {
  const [open, setOpen] = useState(false);
  const words = [L(`工作流 · ${step.name}`, `Workflow · ${step.name}`), L(`第 ${step.n} 步 ${step.label}`, `step ${step.n}, ${step.label}`),
    step.attempt > 1 ? L(`第 ${step.attempt} 次`, `try ${step.attempt}`) : ""].filter(Boolean).join(" · ");
  return (
    <div className="wfn wfn-step" data-testid="workflow-step">
      <Head hubId={hubId} runId={runId} words={words} />
      {step.prompt && <div className="wfn-prompt"><MessageContent body={step.prompt} /></div>}
      {step.rest && (
        <>
          <Fold open={open} onToggle={() => setOpen((o) => !o)} testId="workflow-step-more">
            <span>{L("这一步的输入和交回格式", "Its input and what to hand back")}</span>
          </Fold>
          {open && <div className="wfn-rest"><MessageContent body={step.rest} /></div>}
        </>
      )}
    </div>
  );
}
