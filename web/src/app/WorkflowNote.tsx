import { useState } from "react";
import { ChevronDown, ChevronRight, Workflow as WorkflowIcon } from "lucide-react";
import { appApi } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * The note a workflow sends the agent that started it when the run ends
 * (internal/app/workflows.go, tellStarter):
 *
 *   [工作流「myco-release」完成了]          (or 失败了：why, 取消了)
 *   运行 run_…
 *   - select（wsl-shell）：退出码 0
 *   - …
 *   详情：myco workflow run show --id run_…
 *
 * In a chat it reads as a card of its own: which workflow, how it ended,
 * how many steps, and a way to its run; the steps folded underneath.
 */

export type WorkflowNote = {
  name: string;
  status: "succeeded" | "failed" | "cancelled";
  why: string;
  runId: string;
  steps: { label: string; target: string; line: string }[];
};

const STATUS: Record<string, WorkflowNote["status"]> = { "完成了": "succeeded", "失败了": "failed", "取消了": "cancelled" };

/** The note's parts, or null when it is not one (then it shows as written). */
export function parseWorkflowNote(body: string): WorkflowNote | null {
  const lines = body.replace(/\r/g, "").split("\n");
  const head = /^\[工作流「(.+?)」(完成了|失败了|取消了)(?:：(.*))?\]$/.exec((lines[0] || "").trim());
  const run = /^运行 (\S+)$/.exec((lines[1] || "").trim());
  if (!head || !run) return null;
  const steps: WorkflowNote["steps"] = [];
  for (const raw of lines.slice(2)) {
    const l = raw.trim();
    if (!l || l.startsWith("详情：")) continue;
    // "- label（target）：line" (the label may itself hold brackets; the
    // target is the last （…） before the first "）：")
    const m = /^- (.*)（([^（）]*)）：(.*)$/.exec(l);
    if (m) steps.push({ label: m[1], target: m[2], line: m[3] });
    else if (steps.length) steps[steps.length - 1].line += `\n${l}`;
  }
  return { name: head[1], status: STATUS[head[2]], why: head[3] || "", runId: run[1], steps };
}

const statusWords = (s: WorkflowNote["status"]) =>
  s === "succeeded" ? L("完成了", "Finished") : s === "failed" ? L("失败了", "Failed") : L("取消了", "Cancelled");

/** A step's line that says it did not go well (a failure or a timeout). */
const badLine = (line: string) => /^(超时了|失败|出错|运行结束了|取消)|timed out|failed|error/i.test(line);

export function WorkflowNoteCard({ hubId, note }: { hubId: string; note: WorkflowNote }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const bad = note.steps.filter((s) => badLine(s.line)).length;
  // Its run on the 工作流 page: the run says which workflow it belongs to.
  const openRun = async () => {
    setBusy(true);
    setError(null);
    try {
      const run = await appApi.workflowRun(hubId, note.runId);
      window.dispatchEvent(new CustomEvent("myco-open-notice", { detail: { hubId, target: `workflow:${run.workflow_id}?run=${encodeURIComponent(note.runId)}` } }));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className={cn("wfn", `is-${note.status}`)} data-testid="workflow-note">
      <div className="wfn-head">
        <span className="wfn-icon"><WorkflowIcon /></span>
        <span className="wfn-title">
          <span className="wfn-name">{L(`工作流「${note.name}」`, `Workflow “${note.name}”`)}</span>
          <span className={cn("wfn-state", `is-${note.status}`)}>{statusWords(note.status)}</span>
        </span>
        <button type="button" className="app-btn app-btn-small wfn-open" onClick={() => void openRun()} disabled={busy} data-testid="workflow-note-open">
          {busy ? L("正在打开…", "Opening…") : L("打开这次运行", "Open this run")}<ChevronRight />
        </button>
      </div>
      {note.why && <div className="wfn-why">{note.why}</div>}
      {note.steps.length > 0 && (
        <>
          <button type="button" className="wfn-fold" aria-expanded={open} onClick={() => setOpen((o) => !o)} data-testid="workflow-note-steps">
            <ChevronDown className="wfn-chev" />
            <span>{L(`${note.steps.length} 步`, `${note.steps.length} steps`)}</span>
            {bad > 0 && <span className="wfn-bad">{L(`${bad} 步没顺利完成`, `${bad} did not go well`)}</span>}
          </button>
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
      <ErrorText error={error} />
    </div>
  );
}
