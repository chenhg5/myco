import { useEffect, useMemo, useState } from "react";
import { ArrowLeftRight, ChevronDown, ScrollText, Star } from "lucide-react";
import type { RunComparison, RunStatus, RunStep, StepLog, StepLogEntry, Usage, Workflow, WorkflowRun, WorkflowStats } from "@/api";
import { appApi } from "@/appapi";
import { MessageAttachments } from "@/app/attachments";
import { SectionHead } from "@/app/PageHero";
import { RowsSkeleton } from "@/app/Skeleton";
import { ErrorText, Modal } from "@/app/dialogs";
import { L, E } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/**
 * How a workflow goes, and what each run did (hubs from v0.21.1):
 * - a workflow's figures over its recent runs: how many, how many went
 *   well, how long they take (p50 / p95), each step's time, failures,
 *   retries and send-backs, the latest errors, what it costs, scores;
 * - a run's metrics, its timeline (one bar per step), the files its agents
 *   handed back, its scores, each step's log, and two runs side by side.
 * Older hubs answer not found: the sections are left out.
 */

/** A duration in ms for people: 820 毫秒, 12 秒, 3 分 20 秒, 1 小时 5 分. */
export function msWords(ms?: number): string {
  if (!ms || ms <= 0) return "—";
  if (ms < 1000) return L(E`${Math.round(ms)} 毫秒`, E`${Math.round(ms)}ms`);
  const s = Math.round(ms / 1000);
  if (s < 60) return L(E`${s} 秒`, E`${s}s`);
  const m = Math.floor(s / 60);
  if (m < 60) return s % 60 && m < 10 ? L(E`${m} 分 ${s % 60} 秒`, E`${m}m ${s % 60}s`) : L(E`${m} 分钟`, E`${m} min`);
  const h = Math.floor(m / 60);
  return L(E`${h} 小时${m % 60 ? ` ${m % 60} 分` : ""}`, E`${h}h${m % 60 ? ` ${m % 60}m` : ""}`);
}

const count = (n?: number) => !n ? "0" : n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n);

/** What agents' runs cost: $0.42, or tokens when there is no price. */
export function costWords(u?: Usage): string {
  if (!u) return "—";
  if (u.cost_usd) return `$${u.cost_usd < 0.01 ? u.cost_usd.toFixed(3) : u.cost_usd.toFixed(2)}`;
  const t = (u.input_tokens || 0) + (u.output_tokens || 0);
  return t ? `${count(t)} tokens` : "—";
}
const tokenWords = (u?: Usage) => u && ((u.input_tokens || 0) + (u.output_tokens || 0)) > 0
  ? L(E`输入 ${count(u.input_tokens)} · 输出 ${count(u.output_tokens)} tokens`, E`${count(u.input_tokens)} in · ${count(u.output_tokens)} out tokens`) : "";

const statusWords = (s: RunStatus) => s === "running" ? L("在跑", "running") : s === "waiting" ? L("等人决定", "waiting")
  : s === "succeeded" ? L("完成了", "done") : s === "failed" ? L("失败了", "failed") : L("取消了", "cancelled");

const score = (v: number, unit?: string) => `${Number.isInteger(v) ? v : v.toFixed(2)}${unit || ""}`;

const runTone = (s: RunStatus | string) => s === "failed" ? "is-bad" : s === "succeeded" || s === "done" ? "is-good" : s === "cancelled" ? "is-off"
  : s === "waiting" ? "is-wait" : "is-run";

const stateLegend = () => [
  { tone: "is-good", label: L("完成", "Done") }, { tone: "is-run", label: L("在做", "Working") },
  { tone: "is-wait", label: L("等人决定", "Waiting for someone") }, { tone: "is-bad", label: L("失败", "Failed") },
];

function Legend({ items }: { items: { tone: string; label: string }[] }) {
  return (
    <div className="wfo-legend">
      {items.map((x) => <span key={x.tone}><i className={cn("wfo-swatch", x.tone)} aria-hidden />{x.label}</span>)}
    </div>
  );
}

/* ---------- a workflow's figures ---------- */

type RunsFilter = "" | "failed" | "succeeded" | "active";

export function WorkflowStatsSection({ hubId, wf, stamp, now, onOpenRun }: {
  hubId: string; wf: Workflow; stamp: number; now: number; onOpenRun: (id: string) => void;
}) {
  const [stats, setStats] = useState<WorkflowStats | null>(null);
  const [gone, setGone] = useState(false);
  const [version, setVersion] = useState(0);
  useEffect(() => {
    let off = false;
    const read = () => appApi.workflowStats(hubId, wf.id, { version: version || undefined }).then((s) => { if (!off) setStats(s); },
      () => { if (!off) setGone(true); });
    read();
    const t = setInterval(read, 10_000);
    return () => { off = true; clearInterval(t); };
  }, [hubId, wf.id, wf.version, stamp, version]);
  if (gone && !stats) return null; // an older hub
  if (!stats) {
    return (
      <>
        <SectionHead title={L("运行情况", "How it goes")} />
        <div className="au-stats wfo-stats" aria-hidden>{[0, 1, 2, 3].map((i) => <div key={i} className="au-stat"><span className="skel skel-line is-thin" style={{ width: "40%" }} /><span className="skel skel-line" style={{ width: "60%", height: 18 }} /></div>)}</div>
      </>
    );
  }
  if (stats.total === 0) return null;
  const by = stats.by_status;
  const ended = (by.succeeded || 0) + (by.failed || 0) + (by.cancelled || 0);
  return (
    <>
      <SectionHead title={L("运行情况", "How it goes")}>
        {stats.versions.length > 1 && (
          <div className="seg seg-sm" role="tablist" aria-label={L("哪一版", "Which version")}>
            <button type="button" role="tab" aria-selected={!version} className={cn("seg-btn", !version && "active")} onClick={() => setVersion(0)}>{L("全部", "All")}</button>
            {stats.versions.slice(0, 4).map((v) => (
              <button key={v.version} type="button" role="tab" aria-selected={version === v.version} className={cn("seg-btn", version === v.version && "active")}
                onClick={() => setVersion(v.version)}>{L(E`第 ${v.version} 版`, E`v${v.version}`)}</button>
            ))}
          </div>
        )}
      </SectionHead>
      <div className="au-stats wfo-stats" data-testid="workflow-stats">
        <div className="au-stat"><span>{L("跑了", "Runs")}</span><b>{stats.total}</b>
          <small className="wfo-foot">{L(E`完成 ${by.succeeded || 0} · 失败 ${by.failed || 0}${by.cancelled ? ` · 取消 ${by.cancelled}` : ""}`, E`${by.succeeded || 0} done · ${by.failed || 0} failed${by.cancelled ? ` · ${by.cancelled} cancelled` : ""}`)}</small></div>
        <div className="au-stat"><span>{L("成功率", "Success rate")}</span><b className={cn(!ended && "is-quiet")}>{ended ? `${Math.round(stats.success_rate * 100)}%` : "—"}</b>
          <small className="wfo-foot">{L(E`${ended} 次跑完的里面`, E`of ${ended} ended`)}</small></div>
        <div className="au-stat"><span>{L("用时（中位）", "Time (median)")}</span><b className={cn(!stats.duration_ms.n && "is-quiet")}>{msWords(stats.duration_ms.p50)}</b>
          <small className="wfo-foot">{stats.duration_ms.n > 1 ? L(E`慢的时候（p95）${msWords(stats.duration_ms.p95)}`, E`slow (p95) ${msWords(stats.duration_ms.p95)}`) : " "}</small></div>
        <div className="au-stat"><span>{L("每次花费", "Cost a run")}</span><b className={cn(!stats.usage && "is-quiet")}>{stats.usage ? costWords({ cost_usd: stats.cost_per_run, ...(stats.cost_per_run ? {} : stats.usage) }) : "—"}</b>
          <small className="wfo-foot" title={tokenWords(stats.usage)}>{stats.usage ? L(E`一共 ${costWords(stats.usage)}`, E`${costWords(stats.usage)} in all`) : L("agent 没报用量", "no usage reported")}</small></div>
      </div>
      {stats.waiting_ms.n > 0 && stats.waiting_ms.p50 > 0 && (
        <p className="app-small-note wfo-note">{L(E`其中干活 ${msWords(stats.active_ms.p50)}，等人决定 ${msWords(stats.waiting_ms.p50)}（中位数）。`, E`Of which at work ${msWords(stats.active_ms.p50)}, waiting for decisions ${msWords(stats.waiting_ms.p50)} (median).`)}</p>
      )}

      {stats.recent.length > 1 && <RecentRuns points={stats.recent} now={now} onOpenRun={onOpenRun} />}

      <SectionHead title={L("每一步", "Each step")} count={stats.steps.length} />
      <div className="aud-card wfo-table" data-testid="workflow-step-stats" role="table">
        <div className="wfo-tr wfo-th" role="row">
          <span role="columnheader">{L("步骤", "Step")}</span>
          <span role="columnheader">{L("次数", "Runs")}</span>
          <span role="columnheader">{L("失败", "Failed")}</span>
          <span role="columnheader" title={L("失败后重发、结果不符合退回", "Sent again after failing; sent back for a bad result")}>{L("重发 / 退回", "Retried / sent back")}</span>
          <span role="columnheader">{L("用时中位", "Median")}</span>
          <span role="columnheader">p95</span>
          <span role="columnheader">{L("花费", "Cost")}</span>
        </div>
        {stats.steps.map((s) => (
          <div key={s.step} className="wfo-trow" role="rowgroup">
            <div className="wfo-tr" role="row">
              <span role="cell" className="wfo-step"><b>{s.title || s.step}</b>{s.title && s.title !== s.step && <small className="mono">{s.step}</small>}</span>
              <span role="cell">{s.runs}</span>
              <span role="cell" className={cn(s.failed > 0 && "wfo-bad")}>{s.failed}</span>
              <span role="cell" className={cn(!(s.retries + s.bounces) && "wfo-faint")}>{s.retries} / {s.bounces}</span>
              <span role="cell">{msWords(s.duration_ms.p50)}</span>
              <span role="cell" className={cn(s.duration_ms.n < 2 && "wfo-faint")}>{s.duration_ms.n < 2 ? "—" : msWords(s.duration_ms.p95)}</span>
              <span role="cell" title={tokenWords(s.usage)}>{costWords(s.usage)}</span>
            </div>
            {s.last_error && (
              <button type="button" className="wfo-err" onClick={() => s.last_error_run && onOpenRun(s.last_error_run)}
                title={L("打开那次运行", "Open that run")}>
                <span>{L("最近一次失败：", "Last failure: ")}{s.last_error}</span>
                {s.last_error_at && <small>{timeAgo(s.last_error_at, now)}</small>}
              </button>
            )}
          </div>
        ))}
      </div>

      {stats.evaluations.length > 0 && (
        <>
          <SectionHead title={L("评分", "Scores")} count={stats.evaluations.length} />
          <div className="pc-list wfo-scores" data-testid="workflow-score-stats">
            {stats.evaluations.map((e) => (
              <div key={e.name} className="aud-run wfo-score">
                <span className="wfo-score-name"><Star /><b className="mono">{e.name}</b>{e.direction && <small>{e.direction === "higher" ? L("越高越好", "higher is better") : L("越低越好", "lower is better")}</small>}</span>
                <span className="wfo-score-avg">{L("平均", "avg")} <b>{score(e.values.avg, e.unit)}</b></span>
                <span className="wfo-score-range">{score(e.values.min, e.unit)} – {score(e.values.max, e.unit)}</span>
                <span className="wfo-score-n">{L(E`${e.values.n} 次`, E`${e.values.n} runs`)}</span>
                <button type="button" className="app-btn app-btn-quiet app-btn-small" onClick={() => onOpenRun(e.last_run)}>{L(E`最近 ${score(e.last, e.unit)}`, E`last ${score(e.last, e.unit)}`)}</button>
              </div>
            ))}
          </div>
        </>
      )}
      <p className="app-small-note wfo-note">{stats.version
        ? L(E`用时和每一步按第 ${stats.version} 版最近 ${stats.sample} 次算。`, E`Times and steps: the last ${stats.sample} runs of version ${stats.version}.`)
        : L(E`次数和成功率算全部运行；用时、每一步和花费按最近 ${stats.sample} 次算。`, E`Counts and success rate cover every run; times, steps and cost the last ${stats.sample}.`)}</p>
    </>
  );
}

/** The recent runs as bars: how long each took, coloured by how it ended. */
function RecentRuns({ points, now, onOpenRun }: { points: WorkflowStats["recent"]; now: number; onOpenRun: (id: string) => void }) {
  const list = [...points].reverse(); // oldest on the left
  const max = Math.max(1, ...list.map((p) => p.duration_ms));
  return (
    <div className="aud-card wfo-recent" data-testid="workflow-recent">
      <div className="wfo-recent-head">
        <span>{L(E`最近 ${list.length} 次用时`, E`Time of the last ${list.length} runs`)}</span>
        <span className="wfo-recent-max">{msWords(max)}</span>
      </div>
      <div className="wfo-bars" role="list">
        {list.map((p) => (
          <button key={p.id} type="button" role="listitem" className="wfo-bar-hit" onClick={() => onOpenRun(p.id)}
            title={`${timeAgo(p.created_at, now)} · ${msWords(p.duration_ms)} · ${p.status}${p.failed_step ? ` · ${p.failed_step}` : ""}${p.error ? `\n${p.error}` : ""}`}
            aria-label={`${timeAgo(p.created_at, now)} ${msWords(p.duration_ms)} ${p.status}`}>
            <i className={cn("wfo-bar", runTone(p.status))} style={{ height: `${Math.max(4, (p.duration_ms / max) * 100)}%` }} />
          </button>
        ))}
      </div>
      <Legend items={[{ tone: "is-good", label: L("完成", "Done") }, { tone: "is-bad", label: L("失败", "Failed") }, { tone: "is-off", label: L("取消", "Cancelled") },
        { tone: "is-run", label: L("在跑", "Running") }, { tone: "is-wait", label: L("等人决定", "Waiting for someone") }]
        .filter((x) => x.tone === "is-good" || list.some((p) => runTone(p.status) === x.tone))} />
    </div>
  );
}

/** The runs list's filter. */
export function RunsFilterSeg({ value, onChange }: { value: RunsFilter; onChange: (v: RunsFilter) => void }) {
  const opts: [RunsFilter, string][] = [["", L("全部", "All")], ["failed", L("失败的", "Failed")], ["succeeded", L("完成的", "Done")], ["active", L("在跑的", "Going")]];
  return (
    <div className="seg seg-sm" role="tablist" aria-label={L("看哪些", "Which runs")} data-testid="workflow-runs-filter">
      {opts.map(([v, label]) => (
        <button key={v || "all"} type="button" role="tab" aria-selected={value === v} className={cn("seg-btn", value === v && "active")} onClick={() => onChange(v)}>{label}</button>
      ))}
    </div>
  );
}
export type { RunsFilter };

/* ---------- one run ---------- */

export function RunMetricsStrip({ run }: { run: WorkflowRun }) {
  const m = run.metrics;
  if (!m) return null;
  const extra = [m.retries ? L(E`重发 ${m.retries}`, E`${m.retries} retried`) : "", m.bounces ? L(E`退回 ${m.bounces}`, E`${m.bounces} sent back`) : ""].filter(Boolean).join(" · ");
  return (
    <div className="au-stats wfo-stats wfo-run-stats" data-testid="workflow-run-metrics">
      <div className="au-stat"><span>{L("总用时", "Took")}</span><b>{msWords(m.duration_ms)}</b>
        <small className="wfo-foot">{run.ended_at ? L("从开始到结束", "start to end") : L("到现在，还在跑", "so far")}</small></div>
      <div className="au-stat"><span>{L("干活", "At work")}</span><b>{msWords(m.active_ms)}</b>
        <small className="wfo-foot">{m.waiting_ms ? L(E`等人决定 ${msWords(m.waiting_ms)}`, E`waiting ${msWords(m.waiting_ms)}`) : L("没有等人", "no waiting")}</small></div>
      <div className="au-stat"><span>{L("走了几步", "Steps")}</span><b>{m.steps}</b>
        <small className="wfo-foot">{L(E`发了 ${m.attempts} 个任务`, E`${m.attempts} tasks sent`)}{extra && ` · ${extra}`}</small></div>
      <div className="au-stat"><span>{L("花费", "Cost")}</span><b className={cn(!m.usage && "is-quiet")}>{costWords(m.usage)}</b>
        <small className="wfo-foot" title={tokenWords(m.usage)}>{m.usage ? `${count((m.usage.input_tokens || 0) + (m.usage.output_tokens || 0))} tokens` : L("agent 没报用量", "no usage reported")}</small></div>
    </div>
  );
}

/** One bar per step start, along the run's time. */
export function RunTimeline({ run, now, onPick }: { run: WorkflowRun; now: number; onPick?: (s: RunStep) => void }) {
  const steps = run.steps || [];
  const t0 = new Date(run.created_at).getTime();
  const end = run.ended_at ? new Date(run.ended_at).getTime() : now;
  const total = Math.max(1, end - t0);
  if (steps.length === 0) return null;
  return (
    <div className="aud-card wfo-timeline" data-testid="workflow-timeline">
      {steps.map((s) => {
        const from = new Date(s.started_at).getTime() - t0;
        const len = s.duration_ms ?? Math.max(0, (s.ended_at ? new Date(s.ended_at).getTime() : now) - new Date(s.started_at).getTime());
        const tone = s.status === "waiting" || (s.kind === "human" && s.status === "done") ? "is-wait" : runTone(s.status);
        return (
          <button key={s.id} type="button" className="wfo-tl-row" onClick={() => onPick?.(s)}
            title={`${s.title || s.step}${s.target ? ` · ${s.target}` : ""} · ${msWords(len)}${s.attempts && s.attempts > 1 ? L(E` · 发了 ${s.attempts} 次`, E` · sent ${s.attempts} times`) : ""}`}>
            <span className="wfo-tl-name">{s.title || s.step}{s.attempt > 1 && <small>#{s.attempt}</small>}</span>
            <span className="wfo-tl-track">
              <i className={cn("wfo-tl-bar", tone, s.kind === "human" && "is-human")}
                style={{ left: `${(from / total) * 100}%`, width: `max(4px, ${(len / total) * 100}%)` }} />
            </span>
            <span className="wfo-tl-time">{msWords(len)}</span>
          </button>
        );
      })}
      <div className="wfo-tl-axis"><span>0</span><span>{msWords(total / 2)}</span><span>{msWords(total)}</span></div>
      <Legend items={stateLegend()} />
    </div>
  );
}

/** The files the run's agents handed back, by step; and the last result. */
export function RunArtifacts({ hubId, run }: { hubId: string; run: WorkflowRun }) {
  const files = run.artifacts || [];
  const last = [...(run.steps || [])].reverse().find((s) => s.status === "done" && s.kind !== "human" && s.output);
  const result = last?.output ? Object.fromEntries(Object.entries(last.output).filter(([k]) => k !== "text")) : null;
  if (files.length === 0 && !(result && Object.keys(result).length)) return null;
  const bySteps: { key: string; step: string; items: typeof files }[] = [];
  for (const a of files) {
    const g = bySteps.find((x) => x.key === a.step_run);
    if (g) g.items.push(a); else bySteps.push({ key: a.step_run, step: a.step, items: [a] });
  }
  const title = (id: string) => run.definition?.steps.find((s) => s.id === id)?.title || id;
  return (
    <>
      <SectionHead title={L("产物", "What it produced")} count={files.length || undefined} />
      <div className="aud-card wfo-artifacts" data-testid="workflow-artifacts">
        {bySteps.map((g) => (
          <div key={g.key} className="wfo-art-group">
            <span className="wf-step-label">{title(g.step)}<small>{L(E` · ${g.items[0].from} 交回的`, E` · from ${g.items[0].from}`)}</small></span>
            <MessageAttachments hubId={hubId} items={g.items.map((a) => ({ file_id: a.file_id, name: a.name, size: a.size, sha256: "", content_type: a.content_type }))} />
          </div>
        ))}
        {result && Object.keys(result).length > 0 && last && (
          <div className="wfo-art-group">
            <span className="wf-step-label">{L(E`最后交回的结果 · ${title(last.step)}`, E`Last result · ${title(last.step)}`)}</span>
            <div className="aud-call-body"><pre>{JSON.stringify(result, null, 2)}</pre></div>
          </div>
        )}
      </div>
    </>
  );
}

/** The run's scores, and a way to give one. */
export function RunScores({ hubId, run, onAdded }: { hubId: string; run: WorkflowRun; onAdded: () => void }) {
  const [adding, setAdding] = useState(false);
  const list = run.evaluations;
  if (!list) return null; // an older hub
  return (
    <>
      <SectionHead title={L("评分", "Scores")} count={list.length || undefined}>
        <button type="button" className="app-group-action" onClick={() => setAdding(true)}><Star />{L("打个分", "Score it")}</button>
      </SectionHead>
      {list.length === 0 ? (
        <p className="app-small-note wfo-note">{L("还没有分数。人可以在这里打；测试脚本、负责检查的 agent 用 myco workflow run evaluate 记。打过分的运行，统计里能看平均，两次运行能对比。",
          "No scores yet. Give one here; a test script or a checking agent uses myco workflow run evaluate. Scores add up in the figures and show in comparisons.")}</p>
      ) : (
        <div className="pc-list wfo-scores" data-testid="workflow-run-scores">
          {list.map((e) => (
            <div key={e.id} className="aud-run wfo-score">
              <span className="wfo-score-name"><Star /><b className="mono">{e.name}</b>{e.step && <small>{e.step}</small>}</span>
              <span className="wfo-score-avg"><b>{score(e.value, e.unit)}</b>{e.direction && <small>{e.direction === "higher" ? L(" 越高越好", " higher is better") : L(" 越低越好", " lower is better")}</small>}</span>
              <span className="wfo-score-note">{e.note}</span>
              <span className="wfo-score-n">{e.by}</span>
            </div>
          ))}
        </div>
      )}
      {adding && <ScoreDialog hubId={hubId} run={run} onClose={() => setAdding(false)} onSaved={() => { setAdding(false); onAdded(); }} />}
    </>
  );
}

function ScoreDialog({ hubId, run, onClose, onSaved }: { hubId: string; run: WorkflowRun; onClose: () => void; onSaved: () => void }) {
  const names = useMemo(() => [...new Set((run.evaluations || []).map((e) => e.name))], [run.evaluations]);
  const [name, setName] = useState(names[0] || "quality");
  const [value, setValue] = useState("");
  const [unit, setUnit] = useState("");
  const [direction, setDirection] = useState<"higher" | "lower" | "">("higher");
  const [step, setStep] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const num = Number(value);
  const ok = name.trim() !== "" && value.trim() !== "" && Number.isFinite(num);
  const save = async () => {
    setBusy(true); setError(null);
    try {
      await appApi.evaluateWorkflowRun(hubId, run.id, { name: name.trim(), value: num, unit: unit.trim() || undefined, direction: direction || undefined, note: note.trim() || undefined, step: step || undefined });
      onSaved();
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  return (
    <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={L("给这次运行打分", "Score this run")}
      description={L("记下这次做得怎么样：质量、通过的测试、改动的行数……同一个名字的分数在统计里取平均、在对比里比高低。", "Note how it went: quality, tests passed, lines changed… Scores with the same name average in the figures and compare between runs.")}>
      <form data-testid="workflow-score" onSubmit={(e) => { e.preventDefault(); if (ok) void save(); }}>
        <div className="wfo-score-fields">
          <label className="app-field"><span className="app-field-label">{L("名字", "Name")}</span>
            <input className="app-input mono" value={name} list="wfo-score-names" disabled={busy} onChange={(e) => setName(e.target.value)} placeholder="quality" />
            <datalist id="wfo-score-names">{names.map((n) => <option key={n} value={n} />)}</datalist>
          </label>
          <label className="app-field"><span className="app-field-label">{L("分数", "Value")}</span>
            <input className="app-input" value={value} inputMode="decimal" disabled={busy} autoFocus onChange={(e) => setValue(e.target.value)} placeholder="4" /></label>
          <label className="app-field"><span className="app-field-label">{L("单位（可选）", "Unit (optional)")}</span>
            <input className="app-input" value={unit} disabled={busy} onChange={(e) => setUnit(e.target.value)} placeholder={L("分", "pts")} /></label>
        </div>
        <div className="app-field"><span className="app-field-label">{L("怎么算好", "Which is better")}</span>
          <div className="seg seg-fill" role="tablist">
            {([["higher", L("越高越好", "Higher")], ["lower", L("越低越好", "Lower")], ["", L("不比较", "Neither")]] as const).map(([v, label]) => (
              <button key={v || "none"} type="button" role="tab" aria-selected={direction === v} className={cn("seg-btn", direction === v && "active")} onClick={() => setDirection(v)}>{label}</button>
            ))}
          </div>
        </div>
        <label className="app-field"><span className="app-field-label">{L("给哪一步（可选）", "Which step (optional)")}</span>
          <select className="app-select-field" value={step} disabled={busy} onChange={(e) => setStep(e.target.value)}>
            <option value="">{L("整次运行", "The whole run")}</option>
            {(run.definition?.steps || []).map((s) => <option key={s.id} value={s.id}>{s.title || s.id}</option>)}
          </select>
        </label>
        <label className="app-field"><span className="app-field-label">{L("说明（可选）", "Note (optional)")}</span>
          <input className="app-input" value={note} disabled={busy} onChange={(e) => setNote(e.target.value)} /></label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          {value.trim() !== "" && !Number.isFinite(num) && <span className="app-modal-hint">{L("分数要是数字", "The value must be a number")}</span>}
          <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy || !ok}><Star />{busy ? L("正在记…", "Saving…") : L("记下", "Save")}</button>
        </div>
      </form>
    </Modal>
  );
}

/* ---------- a step's log ---------- */

const kindWords = (e: StepLogEntry): string => {
  switch (e.kind) {
    case "task": return L("交给它", "Sent");
    case "progress": return L("进展", "Progress");
    case "reply": return L("回复", "Reply");
    case "failure": return L("失败", "Failed");
    case "stdout": return "stdout";
    case "stderr": return "stderr";
    case "event": return L("流程", "Run");
    case "trace":
      switch ((e.sub || "").split(":")[0]) {
        case "tool": return L("用工具", "Tool");
        case "tool_result": return L("工具结果", "Result");
        case "tool_error": return L("工具出错", "Tool error");
        case "thinking": return L("思考", "Thinking");
        case "usage": return L("用量", "Usage");
        case "error": return L("出错", "Error");
        case "output": return L("输出", "Output");
        default: return L("说", "Said");
      }
  }
  return e.kind;
};

export function StepLogDialog({ hubId, runId, step, onClose }: { hubId: string; runId: string; step: RunStep; onClose: () => void }) {
  const [log, setLog] = useState<StepLog | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [trace, setTrace] = useState(true);
  useEffect(() => {
    let off = false;
    const read = () => appApi.workflowStepLog(hubId, runId, step.id).then((l) => { if (!off) setLog(l); }, (e) => { if (!off) setError(e); });
    read();
    const t = step.status === "running" ? setInterval(read, 3_000) : undefined;
    return () => { off = true; if (t) clearInterval(t); };
  }, [hubId, runId, step.id, step.status]);
  const entries = (log?.entries || []).filter((e) => trace || e.kind !== "trace");
  const st = log?.step || step;
  return (
    <Modal open onOpenChange={(o) => !o && onClose()} wide className="wfo-log-modal"
      title={L(E`日志 · ${step.title || step.step}${step.attempt > 1 ? ` 第 ${step.attempt} 次` : ""}`, E`Log · ${step.title || step.step}${step.attempt > 1 ? ` #${step.attempt}` : ""}`)}
      description={[st.target, msWords(st.duration_ms), st.attempts && st.attempts > 1 ? L(E`发了 ${st.attempts} 次任务`, E`${st.attempts} tasks sent`) : "", costWords(st.usage) !== "—" ? costWords(st.usage) : ""].filter(Boolean).join(" · ")}>
      <div data-testid="workflow-step-log">
        {step.kind === "agent" && (
          <label className="app-check wfo-log-toggle"><input type="checkbox" checked={trace} onChange={(e) => setTrace(e.target.checked)} />{L("显示它做的每一步（工具、思考）", "Show what it did (tools, thinking)")}</label>
        )}
        <ErrorText error={error} />
        {!log && !error && <RowsSkeleton rows={4} className="pc-list" />}
        {log?.clipped && <p className="app-small-note">{L(E`太长了，只显示最后 ${log.entries.length} 行。`, E`Too long: only the last ${log.entries.length} lines.`)}</p>}
        {log && entries.length === 0 && <div className="app-empty-note">{L("还没有内容。", "Nothing yet.")}</div>}
        <div className="wfo-log">
          {entries.map((e, i) => <LogLine key={i} hubId={hubId} e={e} />)}
        </div>
      </div>
    </Modal>
  );
}

function LogLine({ hubId, e }: { hubId: string; e: StepLogEntry }) {
  const block = ["task", "progress", "reply", "failure", "stdout", "stderr"].includes(e.kind);
  const [open, setOpen] = useState(block && (e.text || "").length < 1200);
  const text = e.text || "";
  const long = text.length > 160 || text.includes("\n");
  return (
    <div className={cn("wfo-log-line", `is-${e.kind}`, e.sub && `is-${e.sub.split(":")[0]}`)}>
      <span className="wf-log-at" title={new Date(e.at).toLocaleString()}>{new Date(e.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false })}</span>
      <span className="wfo-log-kind">{kindWords(e)}{(e.try || 0) > 1 && <small>#{e.try}</small>}</span>
      <div className="wfo-log-body">
        {long && !open ? (
          <button type="button" className="wfo-log-more" onClick={() => setOpen(true)}>
            <span>{text.split("\n")[0].slice(0, 160)}</span><ChevronDown />
          </button>
        ) : (
          <pre className={cn(block && "is-block", (e.kind === "stderr" || e.sub === "tool_error" || e.kind === "failure") && "is-bad")}>{text}</pre>
        )}
        {e.attachments && e.attachments.length > 0 && <MessageAttachments hubId={hubId} items={e.attachments} />}
      </div>
    </div>
  );
}

/** The step row's log button. */
export function LogButton({ onClick }: { onClick: () => void }) {
  return <button type="button" className="app-btn app-btn-quiet app-btn-small" onClick={onClick}><ScrollText />{L("日志", "Log")}</button>;
}

/* ---------- two runs side by side ---------- */

export function CompareButton({ onClick }: { onClick: () => void }) {
  return <button type="button" className="app-btn" onClick={onClick}><ArrowLeftRight />{L("对比", "Compare")}</button>;
}

export function CompareDialog({ hubId, run, onClose, onOpenRun }: {
  hubId: string; run: WorkflowRun; onClose: () => void; onOpenRun: (id: string) => void;
}) {
  const [others, setOthers] = useState<WorkflowRun[] | null>(null);
  const [other, setOther] = useState("");
  const [cmp, setCmp] = useState<RunComparison | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let off = false;
    appApi.workflowRuns(hubId, { workflow: run.workflow_id, limit: 50 }).then((r) => {
      if (off) return;
      const list = (r.runs || []).filter((x) => x.id !== run.id);
      setOthers(list);
      // Default: the run before this one.
      const before = list.find((x) => x.created_at < run.created_at) || list[0];
      if (before) setOther(before.id);
    }, (e) => { if (!off) setError(e); });
    return () => { off = true; };
  }, [hubId, run.id, run.workflow_id, run.created_at]);
  useEffect(() => {
    if (!other) return;
    let off = false;
    setCmp(null);
    appApi.compareWorkflowRuns(hubId, other, run.id).then((c) => { if (!off) setCmp(c); }, (e) => { if (!off) setError(e); });
    return () => { off = true; };
  }, [hubId, other, run.id]);
  const label = (r: WorkflowRun) => `${new Date(r.created_at).toLocaleString([], { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false })} · ${statusWords(r.status)} · ${msWords(r.duration_ms)}${r.version !== run.version ? L(E` · 第 ${r.version} 版`, E` · v${r.version}`) : ""}`;
  const delta = (ms: number) => ms === 0 ? "0" : `${ms > 0 ? "+" : "−"}${msWords(Math.abs(ms))}`;
  return (
    <Modal open onOpenChange={(o) => !o && onClose()} wide className="wfo-compare-modal" title={L("和另一次对比", "Compare with another run")}
      description={L("A 是选的那次，B 是这次。每一步的用时、发了几次任务、花费，和打过的分。", "A is the one you pick, B this one: each step's time, tasks sent and cost, and the scores.")}>
      <div data-testid="workflow-compare">
        <label className="app-field"><span className="app-field-label">{L("和哪一次比（A）", "Compare with (A)")}</span>
          <select className="app-select-field" value={other} onChange={(e) => setOther(e.target.value)} disabled={!others?.length}>
            {(others || []).map((r) => <option key={r.id} value={r.id}>{label(r)}</option>)}
          </select>
        </label>
        {others && others.length === 0 && <div className="app-empty-note">{L("这个工作流只跑过这一次。", "This is its only run.")}</div>}
        <ErrorText error={error} />
        {other && !cmp && !error && <RowsSkeleton rows={3} className="pc-list" />}
        {cmp && (
          <>
            {!cmp.same_version && <p className="app-small-note">{L(E`两次用的不是同一版（第 ${cmp.a.run.version} 版和第 ${cmp.b.run.version} 版）。`, E`Not the same version (v${cmp.a.run.version} and v${cmp.b.run.version}).`)}</p>}
            <div className="wfo-table wfo-cmp" role="table">
              <div className="wfo-tr wfo-th" role="row"><span role="columnheader" /><span role="columnheader">A</span><span role="columnheader">B</span><span role="columnheader">{L("差（B−A）", "B − A")}</span></div>
              <div className="wfo-tr is-total" role="row">
                <span role="cell"><b>{L("总用时", "Took")}</b></span>
                <span role="cell">{msWords(cmp.a.metrics.duration_ms)}</span><span role="cell">{msWords(cmp.b.metrics.duration_ms)}</span>
                <span role="cell" className={cn(cmp.b.metrics.duration_ms < cmp.a.metrics.duration_ms ? "wfo-good" : cmp.b.metrics.duration_ms > cmp.a.metrics.duration_ms && "wfo-bad")}>{delta(cmp.b.metrics.duration_ms - cmp.a.metrics.duration_ms)}</span>
              </div>
              <div className="wfo-tr" role="row">
                <span role="cell">{L("花费", "Cost")}</span>
                <span role="cell">{costWords(cmp.a.metrics.usage)}</span><span role="cell">{costWords(cmp.b.metrics.usage)}</span><span role="cell" />
              </div>
              {cmp.steps.map((s) => (
                <div key={s.step} className="wfo-tr" role="row">
                  <span role="cell" className="wfo-step"><b>{s.title || s.step}</b></span>
                  {[s.a, s.b].map((x, i) => (
                    <span key={i} role="cell" className={cn(!x && "wfo-faint", x?.status === "failed" && "wfo-bad")}>
                      {x ? <>{msWords(x.duration_ms)}{x.attempts > 1 && <small> ×{x.attempts}</small>}{x.status === "failed" && <small> {L("失败", "failed")}</small>}</> : L("没走到", "not reached")}
                    </span>
                  ))}
                  <span role="cell" className={cn(s.faster_in === "b" && "wfo-good", s.faster_in === "a" && "wfo-bad")}>{s.a && s.b ? delta(s.delta_ms) : ""}</span>
                </div>
              ))}
              {cmp.evaluations.map((e) => (
                <div key={e.name} className="wfo-tr" role="row">
                  <span role="cell" className="wfo-step"><Star className="wfo-inline-icon" /><span className="mono">{e.name}</span></span>
                  <span role="cell">{e.a !== undefined ? score(e.a, e.unit) : "—"}</span>
                  <span role="cell">{e.b !== undefined ? score(e.b, e.unit) : "—"}</span>
                  <span role="cell" className={cn(e.better === "b" && "wfo-good", e.better === "a" && "wfo-bad")}>
                    {e.a !== undefined && e.b !== undefined ? `${e.delta > 0 ? "+" : ""}${score(e.delta, e.unit)}` : ""}{e.better && <small> {e.better === "b" ? L("B 更好", "B better") : L("A 更好", "A better")}</small>}
                  </span>
                </div>
              ))}
            </div>
            <div className="app-modal-actions">
              <button type="button" className="app-btn" onClick={() => { onClose(); onOpenRun(cmp.a.run.id); }}>{L("打开 A", "Open A")}</button>
              <button type="button" className="app-btn app-btn-primary" onClick={onClose}>{L("好", "OK")}</button>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}
