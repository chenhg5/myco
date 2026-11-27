import { localPreferences } from "@/lib/localPreferences";
import { useEffect, useRef, useState } from "react";
import { ChevronDown, ListChecks, X } from "lucide-react";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { PlanCard, type PlanItem } from "@/app/TraceView";

/*
 * The plan of the task at work, pinned at the top of the chat: one line
 * (计划 2/5 · 正在：…, a thin bar of how far), opened to the whole list.
 * It comes by itself from what the agent already writes (Claude Code's
 * to-do list, Codex's plan, a checklist it reports); with nothing at work,
 * the newest plan with things left stays (a coordinator's board of what
 * is left). You can fold it to the line or close it for that plan. Once
 * all is done it says so for a moment and goes.
 */

const OPEN_KEY = "myco-plan-bar-open";
const CLOSED_KEY = "myco-plan-bar-closed";
const DONE_MS = 8000;

function readClosed(): string[] {
  try { return JSON.parse(localPreferences.getItem(CLOSED_KEY) || "[]") as string[]; } catch { return []; }
}

export function PlanBar({ taskId, items, live }: {
  taskId: string;
  items: PlanItem[];
  /** the task is still being worked on */
  live: boolean;
}) {
  const [open, setOpen] = useState(() => { try { return localPreferences.getItem(OPEN_KEY) === "1"; } catch { return false; } });
  const [closed, setClosed] = useState<string[]>(readClosed);
  const done = items.filter((x) => x.state === "done").length;
  const all = items.length > 0 && done === items.length;
  // All done: said for a moment (when it was seen at work here), then
  // gone; one already done when the chat opened is not shown. A plan with
  // things left stays, at work or not, until you close it.
  const [gone, setGone] = useState("");
  const sawLive = useRef(live);
  if (live) sawLive.current = true;
  useEffect(() => {
    if (!all) return;
    const t = window.setTimeout(() => setGone(taskId), DONE_MS);
    return () => window.clearTimeout(t);
  }, [all, taskId]);
  if (!items.length || closed.includes(taskId) || gone === taskId || (all && !sawLive.current)) return null;
  const doing = items.find((x) => x.state === "doing") || items.find((x) => x.state === "todo");
  const toggle = () => {
    setOpen((o) => {
      try { localPreferences.setItem(OPEN_KEY, o ? "0" : "1"); } catch { /* not kept */ }
      return !o;
    });
  };
  const close = () => {
    const next = [...closed.filter((x) => x !== taskId), taskId].slice(-200);
    try { localPreferences.setItem(CLOSED_KEY, JSON.stringify(next)); } catch { /* not kept */ }
    setClosed(next);
  };
  return (
    <div className={cn("plan-bar", open && "is-open", all && "is-done")} data-testid="plan-bar">
      <div className="plan-bar-line">
        <button type="button" className="plan-bar-toggle" onClick={toggle} aria-expanded={open}
          title={open ? L("收起计划", "Fold the plan") : L("展开计划", "Show the whole plan")}>
          <ListChecks className="plan-bar-icon" />
          <span className="plan-bar-count">{L(`计划 ${done}/${items.length}`, `Plan ${done}/${items.length}`)}</span>
          <span className="plan-bar-now">
            {all ? L("全部完成", "All done") : doing ? (live ? L(`正在：${doing.text}`, `Now: ${doing.text}`) : L(`下一步：${doing.text}`, `Next: ${doing.text}`)) : ""}
          </span>
          <span className="plan-bar-track" aria-hidden><span style={{ width: `${Math.round((done / items.length) * 100)}%` }} /></span>
          <ChevronDown className="plan-bar-chev" />
        </button>
        <button type="button" className="plan-bar-close" onClick={close}
          title={L("不再置顶这个计划", "Unpin this plan")} aria-label={L("不再置顶这个计划", "Unpin this plan")}><X /></button>
      </div>
      {open && <div className="plan-bar-body"><PlanCard items={items} live={live} /></div>}
    </div>
  );
}
