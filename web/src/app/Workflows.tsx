import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavSub } from "@/app/nav";
import { Bot, Check, ChevronDown, FlaskConical, MessageSquare, Minus, Monitor, MoreHorizontal, Pause, Pencil, Play, Plus, Rocket, RotateCcw, Sparkles, Split, Square, Trash2, UserCheck, Workflow as WorkflowIcon, X } from "lucide-react";
import type { RunStep, WfEdgeDef, WfSchema, WfStepDef, Workflow, WorkflowDecision, WorkflowDef, WorkflowRun } from "@/api";
import { ApiError, appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, nodeName } from "@/app/nodes";
import { Avatar } from "@/app/persona";
import { useContextMenu, type MenuItem } from "@/app/ContextMenu";
import { PageHero, SectionHead, SectionSkeleton } from "@/app/PageHero";
import { RowsSkeleton } from "@/app/Skeleton";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { WorkflowGraph, stateWords, stepKind, stepStates } from "@/app/WorkflowGraph";
import { ContractEditor, ContractsSection } from "@/app/WorkflowContracts";
import { CompareButton, CompareDialog, LogButton, RunArtifacts, RunMetricsStrip, RunScores, RunTimeline, StepLogDialog, WorkflowStatsSection, costWords, msWords, type RunsFilter } from "@/app/WorkflowObserve";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/**
 * 工作流: a fixed way of working the workspace walks for you. Each step goes
 * to an agent as an ordinary task (its chat is where it always is, marked
 * 工作流), to a computer as a command, or to you as a decision. The hub
 * decides what comes next, so it goes on while your computers are off.
 *
 * The page: decisions waiting for you first; then the workflows (how each
 * is going; ▷ starts one, the switch turns it off); then ways to make one:
 * a few common ones where you only pick who does what, or ask an agent to
 * write it. One opened: its steps drawn, then its runs. A run opened: the
 * same drawing with how each step went, then every step with who did it,
 * its summary and its chat.
 */

const seen = new Map<string, Workflow[]>();

const runWords = (s: WorkflowRun["status"]) =>
  s === "running" ? L("在跑", "Running")
    : s === "waiting" ? L("等你决定", "Waiting for you")
      : s === "succeeded" ? L("完成了", "Done")
        : s === "failed" ? L("失败了", "Failed") : L("取消了", "Cancelled");
const runTone = (s: WorkflowRun["status"]) =>
  s === "running" ? "is-ok" : s === "waiting" ? "is-wait" : s === "failed" ? "is-bad" : "";

function duration(from: string, to: string | undefined, now: number): string {
  const s = Math.max(0, Math.round(((to ? new Date(to).getTime() : now) - new Date(from).getTime()) / 1000));
  if (s < 60) return L(`${s} 秒`, `${s}s`);
  const m = Math.round(s / 60);
  if (m < 60) return L(`${m} 分钟`, `${m} min`);
  const h = Math.floor(m / 60);
  return L(`${h} 小时${m % 60 ? ` ${m % 60} 分` : ""}`, `${h}h${m % 60 ? ` ${m % 60}m` : ""}`);
}

/** How long until: 5 分钟, 3 小时, 2 天. */
function untilWords(ms: number): string {
  const m = Math.max(1, Math.round(ms / 60_000));
  if (m < 60) return L(`${m} 分钟`, `${m} min`);
  const h = Math.round(m / 60);
  if (h < 48) return L(`${h} 小时`, `${h}h`);
  return L(`${Math.round(h / 24)} 天`, `${Math.round(h / 24)} days`);
}

/* ---------- ready-made ones: you pick who does what ---------- */

type Role = { key: string; label: string; computer?: boolean; role?: string };
const typed = (s: WfStepDef[]) => s;
type Ready = { id: string; icon: typeof Rocket; name: string; hint: string; roles: Role[]; make: (who: Record<string, string>, name: string) => WorkflowDef };

const readyMade = (): Ready[] => [
  {
    id: "review", icon: WorkflowIcon, name: L("写-审-改", "Write, review, revise"),
    hint: L("一个 agent 做，另一个审；没过就带着意见改，最多 3 轮；最后你点头", "One does it, another reviews; back with the comments if not, up to 3 rounds; you have the last word"),
    roles: [{ key: "writer", label: L("谁来做", "Who does it"), role: L("开发", "developer") }, { key: "reviewer", label: L("谁来审", "Who reviews"), role: L("审查", "review") }],
    make: (w, name) => ({
      name,
      inputs: { required: ["task"], properties: { task: { type: "string", title: L("要做什么", "What to do"), format: "textarea" } } },
      steps: typed([
        { id: "write", title: L("做", "Do it"), agent: w.writer,
          prompt: L("完成这件事：{{inputs.task}}\n{{#if steps.review}}\n上一轮审查的意见，都改掉：{{steps.review.output.comments}}\n{{/if}}\n交结果时 where 写改动在哪（分支、文件或链接）。",
            "Do this: {{inputs.task}}\n{{#if steps.review}}\nThe review's comments, fix them all: {{steps.review.output.comments}}\n{{/if}}\nIn the result, where says where the change is (a branch, files or a link)."),
          output: { required: ["where"], properties: { where: { type: "string", description: L("改动在哪", "where the change is") } } } },
        { id: "review", title: L("审", "Review"), agent: w.reviewer,
          prompt: L("审一下这个改动：{{steps.write.summary}}（在 {{steps.write.output.where}}）。只看对不对、有没有明显的问题。通过就 approved 为 true；不通过在 comments 里一条条写要改什么。",
            "Review this change: {{steps.write.summary}} (at {{steps.write.output.where}}). Only whether it is right and has no obvious problems. If it passes, approved is true; if not, list what to change in comments."),
          output: { required: ["approved"], properties: { approved: { type: "boolean" }, comments: { type: "array", items: { type: "string" } } } } },
        { id: "ok", title: L("你来定", "You decide"), human: "$starter", prompt: L("「{{steps.write.summary}}」审过了。就这样吗？", "\"{{steps.write.summary}}\" passed review. Keep it?"),
          options: [L("就这样", "Keep it"), L("不要了", "Drop it")] },
      ]),
      edges: [
        { from: "write", to: "review" },
        { from: "review", to: "ok", when: "output.approved" },
        { from: "review", to: "write", when: "!output.approved", maxIterations: 3, else: "$fail" },
        { from: "ok", to: "$end", when: `output.choice == '${L("就这样", "Keep it")}'` },
        { from: "ok", to: "$fail", when: `output.choice != '${L("就这样", "Keep it")}'` },
      ],
    }),
  },
  {
    id: "test", icon: FlaskConical, name: L("改完跑测试", "Fix until the tests pass"),
    hint: L("agent 改，电脑上跑测试；没过就把报错交回去再改，最多 3 次", "An agent changes it, a computer runs the tests; on failure the error goes back, up to 3 times"),
    roles: [{ key: "coder", label: L("谁来改", "Who changes it") }, { key: "box", label: L("在哪台电脑上跑测试", "Which computer runs the tests"), computer: true }],
    make: (w, name) => ({
      name,
      inputs: { required: ["task"], properties: {
        task: { type: "string", title: L("要改什么", "What to change"), format: "textarea" },
        command: { type: "string", title: L("测试命令", "Test command"), default: "make test" } } },
      steps: typed([
        { id: "fix", title: L("改", "Change it"), agent: w.coder,
          prompt: L("{{inputs.task}}{{#if steps.test}}\n\n测试没过：\n{{steps.test.error}}\n修好它。{{/if}}", "{{inputs.task}}{{#if steps.test}}\n\nThe tests failed:\n{{steps.test.error}}\nFix it.{{/if}}") },
        { id: "test", title: L("跑测试", "Run the tests"), computer: w.box, run: "{{inputs.command}}" },
      ]),
      edges: [{ from: "fix", to: "test" }, { from: "test", to: "fix", onError: true, maxIterations: 3, else: "$fail" }],
    }),
  },
  {
    id: "fan", icon: Split, name: L("分头做再汇总", "Split, then merge"),
    hint: L("一个 agent 拆成两个方向，两个 agent 同时做，做完再合成一份", "One splits it in two, two agents work at once, then it is merged into one"),
    roles: [{ key: "lead", label: L("谁来拆和汇总", "Who splits and merges") }, { key: "a", label: L("方向一交给", "Direction one") }, { key: "b", label: L("方向二交给", "Direction two") }],
    make: (w, name) => ({
      name,
      inputs: { required: ["task"], properties: { task: { type: "string", title: L("要做什么", "What to do"), format: "textarea" } } },
      steps: typed([
        { id: "plan", title: L("拆", "Split"), agent: w.lead, dispatch: "all",
          prompt: L("把这件事拆成两个能同时做的方向：{{inputs.task}}\n交结果时 a 和 b 各写一句。", "Split this into two directions that can go at once: {{inputs.task}}\nIn the result, a and b one sentence each."),
          output: { required: ["a", "b"], properties: { a: { type: "string" }, b: { type: "string" } } } },
        { id: "one", title: L("方向一", "Direction one"), agent: w.a, prompt: L("按这个方向做：{{steps.plan.output.a}}\n（整件事是：{{inputs.task}}）", "Work on this: {{steps.plan.output.a}}\n(The whole: {{inputs.task}})") },
        { id: "two", title: L("方向二", "Direction two"), agent: w.b, prompt: L("按这个方向做：{{steps.plan.output.b}}\n（整件事是：{{inputs.task}}）", "Work on this: {{steps.plan.output.b}}\n(The whole: {{inputs.task}})") },
        { id: "merge", title: L("汇总", "Merge"), agent: w.lead, join: "all",
          prompt: L("两边都做完了，合成一份给我。\n方向一：{{steps.one.output.text}}\n\n方向二：{{steps.two.output.text}}", "Both are done; merge them into one.\nOne: {{steps.one.output.text}}\n\nTwo: {{steps.two.output.text}}") },
      ]),
      edges: [{ from: "plan", to: "one" }, { from: "plan", to: "two" }, { from: "one", to: "merge" }, { from: "two", to: "merge" }],
    }),
  },
  {
    id: "ship", icon: Rocket, name: L("发布前问你一句", "Ask before shipping"),
    hint: L("agent 列出要发的改动，你点「发布」后才在电脑上跑发布命令；一天没人点就不发", "An agent lists what goes out; the release command runs on a computer only after you say so; no answer in a day, no release"),
    roles: [{ key: "agent", label: L("谁来准备", "Who prepares it") }, { key: "box", label: L("在哪台电脑上发布", "Which computer releases"), computer: true }],
    make: (w, name) => ({
      name,
      inputs: { required: ["what"], properties: {
        what: { type: "string", title: L("发什么", "What goes out") },
        command: { type: "string", title: L("发布命令", "Release command"), default: "make release" } } },
      steps: typed([
        { id: "prepare", title: L("准备", "Prepare"), agent: w.agent, prompt: L("准备发布：{{inputs.what}}。列出要发的改动和风险，第一行一句话总结。", "Get ready to ship: {{inputs.what}}. List what goes out and the risks; the first line sums it up.") },
        { id: "ok", title: L("你来定", "You decide"), human: "$starter", timeout: "1d",
          prompt: L("{{steps.prepare.summary}}\n发布吗？", "{{steps.prepare.summary}}\nShip it?"), options: [L("发布", "Ship"), L("先不发", "Not now")] },
        { id: "release", title: L("发布", "Release"), computer: w.box, run: "{{inputs.command}}" },
      ]),
      edges: [{ from: "prepare", to: "ok" }, { from: "ok", to: "release", when: `output.choice == '${L("发布", "Ship")}'` }, { from: "ok", to: "$end", when: `output.choice != '${L("发布", "Ship")}'` }],
    }),
  },
];

/** A step given "@role:<x>" for its agent goes by role instead. */
function byRole(def: WorkflowDef): WorkflowDef {
  return { ...def, steps: def.steps.map((s) => (s.agent?.startsWith(`${ROLE}:`) ? { ...s, agent: undefined, role: s.agent.slice(ROLE.length + 1) } : s)) };
}

/* ---------- the page ---------- */

export function WorkflowsPage({ hubId, rows, now, open, onOpened, onOpenMessage }: {
  hubId: string; rows: AgentRow[]; now: number;
  /** a workflow (and run) to open, e.g. from an automation's runs */
  open?: { workflow: string; run?: string; at: number };
  /** it was opened (so a later visit starts at the list) */
  onOpened?: () => void;
  /** a step's task, in its conversation */
  onOpenMessage?: (agent: string, messageId: string) => void;
}) {
  const [list, setList] = useState<Workflow[] | null>(() => seen.get(hubId) ?? null);
  const [decisions, setDecisions] = useState<WorkflowDecision[]>([]);
  const [unsupported, setUnsupported] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState("");
  const [openId, setOpenId] = useState<string | null>(null);
  const [runId, setRunId] = useState<string | null>(null);
  const [starting, setStarting] = useState<Workflow | null>(null);
  const [making, setMaking] = useState<Ready | "agent" | null>(null);
  // 新建工作流: the ready-made ones and 让 agent 写一个, in a dialog.
  const [choosing, setChoosing] = useState(false);
  const [removing, setRemoving] = useState<Workflow | null>(null);
  const [editing, setEditing] = useState<Workflow | null>(null);
  const [stamp, setStamp] = useState(0);
  const menu = useContextMenu();
  const load = useCallback(() => {
    appApi.workflows(hubId).then((r) => {
      seen.set(hubId, r.workflows || []); setList(r.workflows || []); setUnsupported(false);
    }, (e) => {
      if (e instanceof ApiError && e.status === 404) setUnsupported(true);
      else setError(e);
      setList((l) => l ?? []);
    });
    appApi.workflowDecisions(hubId).then((r) => {
      setDecisions(r.decisions || []);
      window.dispatchEvent(new CustomEvent("myco-workflow-decisions", { detail: { hubId, count: (r.decisions || []).length } }));
    }, () => undefined);
  }, [hubId]);
  useEffect(() => { setList(seen.get(hubId) ?? null); load(); const t = setInterval(load, 5_000); return () => clearInterval(t); }, [hubId, load]);
  const refresh = () => { load(); setStamp((n) => n + 1); };
  // ← → at the top (nav.ts): what is open here is part of where you are.
  useNavSub("workflows", { open: openId, run: runId, edit: editing?.id ?? null }, { open: null, run: null, edit: null }, (v) => {
    setOpenId(v.open); setRunId(v.run);
    setEditing(v.edit ? (list || []).find((w) => w.id === v.edit) || null : null);
  });
  useEffect(() => { if (open) { setOpenId(open.workflow); setRunId(open.run || null); onOpened?.(); } }, [open?.at]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggle = async (w: Workflow) => {
    setBusy(w.id); setError(null);
    try { await appApi.setWorkflowEnabled(hubId, w.id, !w.enabled); refresh(); } catch (e) { setError(e); } finally { setBusy(""); }
  };
  const remove = async (w: Workflow) => {
    setBusy(w.id); setError(null);
    try { await appApi.deleteWorkflow(hubId, w.id); setRemoving(null); if (openId === w.id) setOpenId(null); refresh(); } catch (e) { setError(e); setRemoving(null); } finally { setBusy(""); }
  };

  const gallery = (picked?: () => void) => (
    <div className="au-templates" data-testid="workflow-templates">
      {readyMade().map((t) => (
        <button key={t.id} type="button" className="au-template" onClick={() => { picked?.(); setMaking(t); }}>
          <span className="tp-icon"><t.icon /></span>
          <span className="au-template-text">
            <span className="au-template-name">{t.name}</span>
            <span className="au-template-hint">{t.hint}</span>
          </span>
        </button>
      ))}
      <button type="button" className="au-template is-blank" onClick={() => { picked?.(); setMaking("agent"); }}>
        <span className="tp-icon"><Sparkles /></span>
        <span className="au-template-text">
          <span className="au-template-name">{L("让 agent 写一个", "Have an agent write one")}</span>
          <span className="au-template-hint">{L("用一句话说清要怎么走，它写好、检查好再存进来", "Say how it should go; it writes, checks and saves it")}</span>
        </span>
      </button>
    </div>
  );

  const dialogs = (
    <>
      <Modal open={choosing} onOpenChange={setChoosing} title={L("新建工作流", "New workflow")} className="wf-new-dlg"
        description={L("挑一个现成的，只选谁来做；或者用一句话说清楚，让 agent 写。", "Pick a ready-made one and choose who does what, or say how it goes and have an agent write it.")}>
        <div data-testid="workflow-new">{gallery(() => setChoosing(false))}</div>
      </Modal>
      <StartDialog hubId={hubId} wf={starting} onClose={() => setStarting(null)} onStarted={(r) => { setStarting(null); refresh(); setOpenId(r.workflow_id); setRunId(r.id); }} />
      <MakeDialog hubId={hubId} rows={rows} making={making} onClose={() => setMaking(null)}
        onMade={(w) => { setMaking(null); refresh(); setOpenId(w.id); }}
        onAsked={(agent, msg) => { setMaking(null); onOpenMessage?.(agent, msg); }} />
      <ConfirmDialog open={!!removing} onOpenChange={(o) => !o && setRemoving(null)} title={L("删除这个工作流？", "Delete this workflow?")}
        description={L("它的运行记录一起删除，还在跑的会先停下。它发出去的任务和 agent 的回复都保留。", "Its runs go too; the ones going are stopped first. The tasks it sent and the answers stay.")}
        confirmLabel={L("删除", "Delete")} onConfirm={() => removing && void remove(removing)} busy={!!removing && busy === removing.id} />
      {menu.element}
    </>
  );

  const opened = openId ? (list || []).find((w) => w.id === openId) : undefined;
  if (editing) {
    return (
      <div className="pc au pcd aud wf">
        <WorkflowEditor hubId={hubId} rows={rows} wf={editing} onClose={() => setEditing(null)}
          onSaved={() => { setOpenId(editing.id); setRunId(null); setEditing(null); refresh(); }} />
        {dialogs}
      </div>
    );
  }
  if (opened && runId) {
    return (
      <div className="pc au pcd aud wf" data-testid="workflow-run">
        <RunDetail hubId={hubId} runId={runId} wf={opened} rows={rows} now={now} stamp={stamp} onBack={() => setRunId(null)}
          onChanged={refresh} onOpenMessage={onOpenMessage} onOpenRun={setRunId} />
        {dialogs}
      </div>
    );
  }
  if (opened) {
    return (
      <div className="pc au pcd aud wf" data-testid="workflow-detail">
        <WorkflowDetail hubId={hubId} wf={opened} rows={rows} now={now} stamp={stamp} busy={busy === opened.id} error={error}
          onBack={() => setOpenId(null)} onRun={() => setStarting(opened)} onToggle={() => void toggle(opened)} onDelete={() => setRemoving(opened)}
          onEdit={() => setEditing(opened)}
          onOpenRun={(id) => setRunId(id)} />
        {dialogs}
      </div>
    );
  }

  return (
    <div className="pc au wf">
      <PageHero title={L("工作流", "Workflows")}
        subtitle={L("把固定的做法写成流程：每一步交给一个 agent、一台电脑，或者等你决定。工作区按顺序往下走，电脑关着也照常。", "A fixed way of working: each step goes to an agent, a computer, or waits for you. The workspace walks it in order, while your computers are off too.")}>
        {!unsupported && <button type="button" className="app-btn app-btn-primary" onClick={() => setChoosing(true)}><Plus />{L("新建工作流", "New workflow")}</button>}
      </PageHero>
      <ErrorText error={error} />
      {unsupported ? (
        <div className="app-empty" data-testid="workflows-unsupported">
          <div className="app-empty-icon"><WorkflowIcon /></div>
          <h2>{L("这个工作区还不能用工作流", "No workflows in this workspace yet")}</h2>
          <p>{L("它的版本太旧。在「设置 › 工作区」里一键升级，或者在服务器上运行 sudo myco upgrade。", "Its version is too old: upgrade it in Settings › Workspace, or run sudo myco upgrade on the server.")}</p>
        </div>
      ) : list === null ? (
        <>
          <SectionSkeleton />
          <RowsSkeleton rows={3} className="pc-list" />
        </>
      ) : (
        <>
          {decisions.length > 0 && (
            <>
              <SectionHead title={L("等你决定", "Waiting for you")} count={decisions.length} />
              <div className="wf-decisions" data-testid="workflow-decisions">
                {decisions.map((d) => (
                  <DecisionCard key={d.step.id} hubId={hubId} d={d} now={now} onDone={refresh}
                    onOpen={() => { setOpenId(d.run.workflow_id); setRunId(d.run.id); }} />
                ))}
              </div>
            </>
          )}
          {list.length === 0 ? (
            <div className="au-none" data-testid="workflows-empty">
              <span className="tp-icon"><WorkflowIcon /></span>
              <div>
                <b>{L("还没有工作流", "No workflows yet")}</b>
                <p>{L("比如：一个 agent 写、另一个审，没过就改，最后你点头。每一步的对话都在原来的地方，标着「工作流」。从下面挑一个开始。", "Say, one agent writes, another reviews, back for changes if needed, and you have the last word. Each step's chat is where it always is, marked 工作流. Start from one below.")}</p>
              </div>
            </div>
          ) : (
            <>
              <SectionHead title={L("我的工作流", "Your workflows")} count={list.length} />
              <div className="pc-list" data-testid="workflows">
                {list.map((w) => {
                  // (right-click, or ⋯: the same menu)
                  const items: MenuItem[] = [
                    { label: L("打开", "Open"), icon: WorkflowIcon, onSelect: () => setOpenId(w.id) },
                    { label: L("编辑", "Edit"), icon: Pencil, onSelect: () => setEditing(w) },
                    { label: w.enabled ? L("停用", "Turn off") : L("启用", "Turn on"), icon: w.enabled ? Pause : Play, onSelect: () => void toggle(w), disabled: busy === w.id },
                    "sep",
                    { label: L("删除", "Delete"), icon: Trash2, danger: true, onSelect: () => setRemoving(w) },
                  ];
                  return (
                  <div key={w.id} className={cn("au-row", !w.enabled && "is-off")} onContextMenu={(e) => { e.preventDefault(); menu.open(e, items); }}>
                    <button type="button" className="au-row-main" onClick={() => setOpenId(w.id)} title={L("打开：流程和每一次运行", "Open: its steps and every run")}>
                      <span className="au-icon"><WorkflowIcon /></span>
                      <span className="au-row-text">
                        <span className="au-row-name">{w.title || w.name}</span>
                        <span className="au-row-what">
                          <span>{L(`${w.steps} 步`, `${w.steps} steps`)}</span>
                          {w.description && <span>{w.description}</span>}
                          {w.project && <span>{L(`项目 ${w.project}`, `Project ${w.project}`)}</span>}
                        </span>
                        <span className="au-row-status wf-row-last">
                          {!w.enabled && <span>{L("已停用", "Off")}</span>}
                          {w.enabled && w.active > 1 && <span className="wf-run-state is-ok">{L(`${w.active} 个在跑`, `${w.active} running`)}</span>}
                          {w.last_run ? (
                            <>
                              <RunMark status={w.last_run.status} />
                              <span className={cn("wf-run-state", runTone(w.last_run.status))}>{runWords(w.last_run.status)}</span>
                              {runSubject(w.last_run) && <span className="wf-row-subject">{runSubject(w.last_run)}</span>}
                              <span>{timeAgo(w.last_run.created_at, now)}</span>
                            </>
                          ) : w.enabled && <span>{L("还没跑过", "Not run yet")}</span>}
                        </span>
                      </span>
                    </button>
                    <span className="wf-row-actions">
                      <button type="button" className="app-btn app-btn-small wf-run-now" disabled={!w.enabled} onClick={() => setStarting(w)}
                        title={L("运行一次", "Run it")}><Play />{L("运行", "Run")}</button>
                      <button type="button" className="app-btn app-btn-small wf-more" aria-label={L(`${w.title || w.name}：更多`, `${w.title || w.name}: more`)}
                        title={L("编辑、停用、删除", "Edit, turn off, delete")} onClick={(e) => menu.openUnder(e.currentTarget, items)}><MoreHorizontal /></button>
                    </span>
                  </div>
                  );
                })}
              </div>
            </>
          )}
          {list.length === 0 && (
            <>
              <SectionHead title={L("从这里开始", "Start here")} />
              {gallery()}
            </>
          )}
        </>
      )}
      {dialogs}
    </div>
  );
}

/* ---------- a decision waiting for you ---------- */

function DecisionCard({ hubId, d, now, onDone, onOpen }: { hubId: string; d: WorkflowDecision; now: number; onDone: () => void; onOpen?: () => void }) {
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<unknown>(null);
  const decide = async (choice: string) => {
    setBusy(choice); setError(null);
    try { await appApi.decideWorkflowRun(hubId, d.run.id, d.step.step, choice, note.trim() || undefined); onDone(); } catch (e) { setError(e); } finally { setBusy(""); }
  };
  const due = d.step.due_at ? new Date(d.step.due_at).getTime() : 0;
  return (
    <div className="wf-decision" data-testid="workflow-decision">
      <div className="wf-decision-head">
        <span className="wf-decision-icon"><UserCheck /></span>
        <span className="wf-decision-where">
          {onOpen ? <button type="button" className="wf-link" onClick={onOpen}>{d.run.name}</button> : <b>{d.run.name}</b>}
          <span> · {d.step.title || d.step.step}</span>
        </span>
        <span className="wf-decision-when" title={due ? new Date(due).toLocaleString() : undefined}>
          {due > now ? L(`${untilWords(due - now)}后过期，过期当作不做`, `Expires in ${untilWords(due - now)}; then it counts as no`) : ""}
        </span>
      </div>
      <div className="wf-decision-text">{d.step.input}</div>
      <input className="app-input wf-decision-note" value={note} onChange={(e) => setNote(e.target.value)} maxLength={300}
        placeholder={L("想说一句就写在这里（可选）", "A word to go with it (optional)")} disabled={!!busy} />
      <div className="wf-decision-options">
        {(d.step.options || []).map((o) => (
          <button key={o} type="button" className="app-btn" disabled={!!busy} onClick={() => void decide(o)}>{busy === o ? L("正在提交…", "Sending…") : o}</button>
        ))}
      </div>
      <ErrorText error={error} />
    </div>
  );
}

/* ---------- start a run: the inputs it takes ---------- */

function StartDialog({ hubId, wf, onClose, onStarted }: { hubId: string; wf: Workflow | null; onClose: () => void; onStarted: (r: WorkflowRun) => void }) {
  const [def, setDef] = useState<WorkflowDef | null>(null);
  const [values, setValues] = useState<Record<string, unknown>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (!wf) return;
    setDef(null); setError(null); setBusy(false);
    let off = false;
    appApi.workflow(hubId, wf.id).then((w) => {
      if (off) return;
      const d = w.definition || { name: w.name, steps: [] };
      setDef(d);
      const v: Record<string, unknown> = {};
      for (const [k, p] of Object.entries(d.inputs?.properties || {})) if (p.default !== undefined) v[k] = p.default;
      setValues(v);
    }, (e) => { if (!off) setError(e); });
    return () => { off = true; };
  }, [hubId, wf]);
  if (!wf) return null;
  const props = Object.entries(def?.inputs?.properties || {});
  const required = def?.inputs?.required || [];
  const missing = required.find((k) => values[k] === undefined || values[k] === "");
  const start = async () => {
    setBusy(true); setError(null);
    try { onStarted(await appApi.startWorkflow(hubId, wf.id, values)); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  return (
    <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={L(`运行「${wf.name}」`, `Run "${wf.name}"`)}
      description={props.length ? L("填好它要的，第一步马上交出去。", "Fill in what it takes; the first step goes out at once.") : L("它不用填什么，第一步马上交出去。", "It takes nothing; the first step goes out at once.")}>
      <form data-testid="workflow-start" onSubmit={(e) => { e.preventDefault(); if (!missing && def) void start(); }}>
        {def === null && !error && <span className="skel skel-line" style={{ width: "60%" }} />}
        {props.map(([k, p]) => (
          <InputField key={k} name={k} schema={p} required={required.includes(k)} value={values[k]} disabled={busy}
            onChange={(v) => setValues((x) => ({ ...x, [k]: v }))} />
        ))}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          {missing && def && <span className="app-modal-hint">{L(`还没填「${def.inputs?.properties?.[missing]?.title || missing}」`, `Fill in "${def.inputs?.properties?.[missing]?.title || missing}"`)}</span>}
          <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy || !!missing || !def}><Play />{busy ? L("正在开始…", "Starting…") : L("开始运行", "Start")}</button>
        </div>
      </form>
    </Modal>
  );
}

function InputField({ name, schema, required, value, disabled, onChange }: {
  name: string; schema: WfSchema; required: boolean; value: unknown; disabled: boolean; onChange: (v: unknown) => void;
}) {
  const label = `${schema.title || name}${required ? "" : L("（可选）", " (optional)")}`;
  if (schema.type === "boolean") {
    return (
      <label className="app-check wf-check"><input type="checkbox" checked={!!value} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />{label}</label>
    );
  }
  return (
    <label className="app-field"><span className="app-field-label">{label}</span>
      {schema.enum ? (
        <select className="app-select-field" value={String(value ?? "")} disabled={disabled} onChange={(e) => onChange(e.target.value)}>
          {!required && <option value="">—</option>}
          {schema.enum.map((o) => <option key={String(o)} value={String(o)}>{String(o)}</option>)}
        </select>
      ) : schema.format === "textarea" ? (
        <textarea className="app-input au-body" rows={3} value={String(value ?? "")} disabled={disabled} placeholder={schema.description}
          onChange={(e) => onChange(e.target.value)} />
      ) : (
        <input className="app-input" value={String(value ?? "")} disabled={disabled} placeholder={schema.description}
          inputMode={schema.type === "number" || schema.type === "integer" ? "decimal" : undefined} onChange={(e) => onChange(e.target.value)} />
      )}
    </label>
  );
}

/* ---------- make one: ready-made (pick who), or ask an agent ---------- */

function MakeDialog({ hubId, rows, making, onClose, onMade, onAsked }: {
  hubId: string; rows: AgentRow[]; making: Ready | "agent" | null; onClose: () => void;
  onMade: (w: Workflow) => void; onAsked: (agent: string, messageId: string) => void;
}) {
  const mine = rows.filter((r) => r.hub.id === hubId);
  const agents = mine.filter((r) => agentKind(r.st.agent) === "agent");
  const computers = mine.filter((r) => r.st.agent.binding.harness === "exec");
  const [name, setName] = useState("");
  const [who, setWho] = useState<Record<string, string>>({});
  const [ask, setAsk] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const key = making === "agent" ? "agent" : making?.id || "";
  useEffect(() => {
    if (!making) return;
    setError(null); setBusy(false); setAsk("");
    if (making === "agent") { setWho({ agent: agents[0]?.st.agent.name || "" }); return; }
    setName(making.name);
    const pick: Record<string, string> = {};
    making.roles.forEach((r, i) => {
      const pool = r.computer ? computers : agents;
      pick[r.key] = pool[Math.min(i, pool.length - 1)]?.st.agent.name || "";
    });
    setWho(pick);
    // only when another one is opened
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  if (!making) return null;

  const picker = (role: Role) => {
    const pool = role.computer ? computers : agents;
    const r = pool.find((x) => x.st.agent.name === who[role.key]);
    return (
      <label key={role.key} className="app-field"><span className="app-field-label">{role.label}</span>
        <span className="au-to">
          {r ? <Avatar agent={r.st.agent} size={20} badge={false} /> : role.computer ? <Monitor className="wf-pick-icon" /> : <Bot className="wf-pick-icon" />}
          <select className="app-select-field" value={who[role.key] || ""} disabled={busy || (!pool.length && !!role.computer)} onChange={(e) => setWho((w) => ({ ...w, [role.key]: e.target.value }))}>
            {!pool.length && <option value="">{role.computer ? L("还没有电脑", "No computers yet") : L("还没有 agent", "No agents yet")}</option>}
            {pool.map((x) => <option key={x.st.agent.name} value={x.st.agent.name}>{nodeName(x.st.agent, x.hub)}</option>)}
            {!role.computer && role.role && <option value={`${ROLE}:${role.role}`}>{L(`按分工「${role.role}」挑空闲的`, `Whoever is free with role "${role.role}"`)}</option>}
          </select>
        </span></label>
    );
  };

  if (making === "agent") {
    const r = agents.find((x) => x.st.agent.name === who.agent);
    const send = async () => {
      if (!r) return;
      setBusy(true); setError(null);
      const names = mine.filter((x) => agentKind(x.st.agent) === "agent" || x.st.agent.binding.harness === "exec")
        .map((x) => `${x.st.agent.name}${x.st.agent.binding.harness === "exec" ? L("（电脑）", " (computer)") : ""}`).join("、");
      const body = L(
        `帮我在 myco 里建一个工作流：${ask.trim()}\n\n格式看 myco workflow validate --help。写好 JSON 后先 myco workflow validate --file <文件> 检查，再 myco workflow apply --file <文件> 存进来。能用的 agent 和电脑：${names}。存好后回我一句它是怎么走的。`,
        `Make me a workflow in myco: ${ask.trim()}\n\nThe format is in myco workflow validate --help. Write the JSON, check it with myco workflow validate --file <file>, then save it with myco workflow apply --file <file>. Agents and computers to use: ${names}. When it is saved, tell me in a sentence how it goes.`);
      try {
        const m = await appApi.sendMessage(hubId, { to: r.st.agent.name, body });
        onAsked(r.st.agent.name, m.id);
      } catch (e) { setError(e); } finally { setBusy(false); }
    };
    return (
      <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={L("让 agent 写一个工作流", "Have an agent write a workflow")} className="au-edit"
        description={L("说清楚要分几步、每步谁来做、什么时候要你点头。它写好、检查好就存进来，出现在这一页。", "Say what the steps are, who does each, and when you want to decide. It writes, checks and saves it; it shows up on this page.")}>
        <form data-testid="workflow-ask" onSubmit={(e) => { e.preventDefault(); if (r && ask.trim()) void send(); }}>
          {picker({ key: "agent", label: L("交给谁写", "Who writes it") })}
          <label className="app-field"><span className="app-field-label">{L("要怎么走", "How it goes")}</span>
            <textarea className="app-input au-body" rows={4} value={ask} onChange={(e) => setAsk(e.target.value)} disabled={busy}
              placeholder={L("比如：每次有新需求，先让 Claude 写方案，Codex 审；审过了 Claude 实现，在 Studio Mac 上跑测试，过了再问我要不要合并。", "e.g. For each new request, Claude writes a plan and Codex reviews it; once it passes Claude builds it, the tests run on Studio Mac, and if they pass I am asked whether to merge.")} /></label>
          <ErrorText error={error} />
          <div className="app-modal-actions au-actions">
            {!agents.length && <span className="app-modal-hint">{L("先加一个 agent", "Add an agent first")}</span>}
            <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={busy || !r || !ask.trim()}><MessageSquare />{busy ? L("正在发…", "Sending…") : L("交给它写", "Ask it")}</button>
          </div>
        </form>
      </Modal>
    );
  }

  const ready = making;
  const lacking = ready.roles.find((r) => !who[r.key]);
  const save = async () => {
    setBusy(true); setError(null);
    try {
      const out = await appApi.applyWorkflow(hubId, byRole(ready.make(who, name.trim() || ready.name)));
      onMade(out.workflow);
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  return (
    <Modal open onOpenChange={(o) => !o && !busy && onClose()} title={L(`新建「${ready.name}」`, `New "${ready.name}"`)} className="au-edit" description={ready.hint}>
      <form data-testid="workflow-make" onSubmit={(e) => { e.preventDefault(); if (!lacking) void save(); }}>
        <label className="app-field"><span className="app-field-label">{L("名字", "Name")}</span>
          <input className="app-input" value={name} onChange={(e) => setName(e.target.value)} maxLength={80} disabled={busy} /></label>
        {ready.roles.map(picker)}
        <div className="wf-make-preview">
          <WorkflowGraph def={byRole(ready.make(who, name || ready.name))} hubId={hubId} rows={rows} />
        </div>
        <ErrorText error={error} />
        <div className="app-modal-actions au-actions">
          {lacking && <span className="app-modal-hint">{lacking.computer ? L("先连一台电脑", "Connect a computer first") : L("先加一个 agent", "Add an agent first")}</span>}
          <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy || !!lacking || !name.trim()}>{busy ? L("正在保存…", "Saving…") : L("保存工作流", "Save workflow")}</button>
        </div>
      </form>
    </Modal>
  );
}

/* ---------- edit one: who does each step, how long, how many rounds ---------- */

/** In the who-does-it list: pick by role (分工) instead of one agent. */
const ROLE = "@role";

const timeouts = (): { v: string; label: string }[] => [
  { v: "", label: L("不限", "None") }, { v: "30m", label: L("30 分钟", "30 min") }, { v: "1h", label: L("1 小时", "1 hour") },
  { v: "2h", label: L("2 小时", "2 hours") }, { v: "4h", label: L("4 小时", "4 hours") }, { v: "1d", label: L("1 天", "1 day") }, { v: "3d", label: L("3 天", "3 days") },
];

/**
 * Editing a workflow, a page of its own: the steps drawn on the left (click
 * one), what is picked on the right — 基本 (its description, what a run is
 * given, how many rounds a loop may go) or one step (who does it, timeout,
 * retries, what it is asked, what it hands back). Saved as a new version;
 * runs going on keep theirs. Steps are added and removed by the agent that
 * wrote it.
 */
function WorkflowEditor({ hubId, rows, wf, onClose, onSaved }: { hubId: string; rows: AgentRow[]; wf: Workflow; onClose: () => void; onSaved: () => void }) {
  const [def, setDef] = useState<WorkflowDef | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [picked, setPickedRaw] = useState<string>(BASIC);
  const panelRef = useRef<HTMLDivElement>(null);
  // One column (narrow): the panel is under the drawing; show it.
  const setPicked = (v: string | ((p: string) => string)) => {
    setPickedRaw(v);
    if (window.matchMedia("(max-width: 980px)").matches) requestAnimationFrame(() => panelRef.current?.scrollIntoView({ block: "start", behavior: "smooth" }));
  };
  const [changed, setChanged] = useState(false);
  const [leaving, setLeaving] = useState(false);
  useEffect(() => {
    setDef(null); setError(null); setBusy(false); setChanged(false); setPickedRaw(BASIC);
    let off = false;
    appApi.workflow(hubId, wf.id).then((w) => { if (!off && w.definition) setDef(structuredClone(w.definition)); }, (e) => { if (!off) setError(e); });
    return () => { off = true; };
  }, [hubId, wf.id]);
  const mine = rows.filter((r) => r.hub.id === hubId);
  const agents = mine.filter((r) => agentKind(r.st.agent) === "agent");
  const computers = mine.filter((r) => r.st.agent.binding.harness === "exec");
  const edit = (f: (d: WorkflowDef) => WorkflowDef) => { setDef((d) => d && f(d)); setChanged(true); };
  const setStep = (i: number, patch: Partial<WfStepDef>) => edit((d) => ({ ...d, steps: d.steps.map((x, j) => (j === i ? { ...x, ...patch } : x)) }));
  const setEdge = (i: number, patch: Partial<WfEdgeDef>) => edit((d) => ({ ...d, edges: (d.edges || []).map((x, j) => (j === i ? { ...x, ...patch } : x)) }));
  const idx = new Map((def?.steps || []).map((x, i) => [x.id, i]));
  const loops = (def?.edges || []).map((e, i) => ({ e, i })).filter(({ e }) => idx.has(e.to) && (idx.get(e.to) ?? 0) <= (idx.get(e.from) ?? 0));
  const noRole = def?.steps.find((s) => s.role !== undefined && !s.role.trim());
  const save = async () => {
    if (!def || noRole) return;
    setBusy(true); setError(null);
    try { await appApi.applyWorkflow(hubId, def); onSaved(); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const close = () => (changed ? setLeaving(true) : onClose());
  const label = (s: WfStepDef) => s.title || s.id;
  const i = def ? def.steps.findIndex((s) => s.id === picked) : -1;
  const s = i >= 0 ? def!.steps[i] : null;

  const basic = def && (
    <div className="wf-ed-panel" data-testid="workflow-edit-basic">
      <h3>{L("基本", "Basics")}</h3>
      <label className="app-field"><span className="app-field-label">{L("说明（可选）", "Description (optional)")}</span>
        <input className="app-input" value={def.description || ""} maxLength={200} disabled={busy}
          onChange={(e) => edit((d) => ({ ...d, description: e.target.value }))} placeholder={L("一句话说这个工作流做什么", "What it does, in a sentence")} /></label>
      <ContractEditor schema={def.inputs} disabled={busy} label={L("运行时要填的", "What a run is given")} testid="workflow-edit-inputs"
        onChange={(inputs) => edit((d) => ({ ...d, inputs }))} />
      {loops.length > 0 && (
        <div className="wf-edit-loops">
          <span className="app-field-label">{L("回去重来最多几次", "How many times a loop may go back")}</span>
          {loops.map(({ e, i: k }) => (
            <label key={k} className="wf-edit-loop">
              <span>{L(`「${label(def.steps[idx.get(e.from)!])}」回到「${label(def.steps[idx.get(e.to)!])}」最多`, `"${label(def.steps[idx.get(e.from)!])}" back to "${label(def.steps[idx.get(e.to)!])}" at most`)}</span>
              <select className="app-select-field" value={e.maxIterations || 1} disabled={busy} onChange={(ev) => setEdge(k, { maxIterations: +ev.target.value })}>
                {[1, 2, 3, 4, 5, 8, 10].map((n) => <option key={n} value={n}>{L(`${n} 次`, `${n}×`)}</option>)}
              </select>
            </label>
          ))}
        </div>
      )}
    </div>
  );

  const step = s && (() => {
    const kind = stepKind(s);
    const pool = kind === "computer" ? computers : agents;
    const who = kind === "computer" ? s.computer : s.agent;
    const n = i + 1;
    return (
      <div className="wf-ed-panel" data-testid="workflow-edit-step" data-step={s.id}>
        <div className="wf-edit-step-head">
          <span className="wf-step-icon">{kind === "computer" ? <Monitor /> : kind === "human" ? <UserCheck /> : <Bot />}</span>
          <input className="app-input wf-edit-title" value={s.title || ""} placeholder={s.id} maxLength={40} disabled={busy}
            aria-label={L(`第 ${n} 步的名字`, `Step ${n} name`)} onChange={(e) => setStep(i, { title: e.target.value || undefined })} />
          <span className="wf-edit-n">{L(`第 ${n} 步`, `Step ${n}`)} · {kind === "computer" ? L("电脑", "computer") : kind === "human" ? L("你来定", "a decision") : "agent"}</span>
        </div>
        <div className="wf-edit-row">
          {kind === "human" ? (
            <span className="wf-edit-who">{s.human === "$starter" ? L("启动它的人来决定", "Whoever starts it decides") : L(`${s.human} 来决定`, `${s.human} decides`)}</span>
          ) : (
            <label className="wf-edit-cell wf-edit-grow"><span>{kind === "computer" ? L("在哪台电脑", "On which computer") : L("交给谁", "Who does it")}</span>
              <span className="wf-edit-pick">
                <select className="app-select-field" value={s.role !== undefined ? ROLE : who || ""} disabled={busy} aria-label={L(`第 ${n} 步交给谁`, `Who does step ${n}`)}
                  onChange={(e) => setStep(i, kind === "computer" ? { computer: e.target.value }
                    : e.target.value === ROLE ? { agent: undefined, role: s.role || "" } : { agent: e.target.value, role: undefined })}>
                  {who && !pool.some((x) => x.st.agent.name === who) && <option value={who}>{who}</option>}
                  {pool.map((x) => <option key={x.st.agent.name} value={x.st.agent.name}>{nodeName(x.st.agent, x.hub)}</option>)}
                  {kind === "agent" && <option value={ROLE}>{L("按分工挑空闲的…", "Whoever is free with a role…")}</option>}
                </select>
                {s.role !== undefined && (
                  <input className="app-input wf-edit-role" value={s.role} maxLength={40} disabled={busy} placeholder={L("分工，比如 审查", "Role, e.g. review")}
                    aria-label={L(`第 ${n} 步的分工`, `Role for step ${n}`)} onChange={(e) => setStep(i, { role: e.target.value })} />
                )}
              </span></label>
          )}
          <label className="wf-edit-cell"><span>{kind === "human" ? L("多久没人决定算不做", "Expires after") : L("超时", "Timeout")}</span>
            <select className="app-select-field" value={s.timeout || ""} disabled={busy} onChange={(e) => setStep(i, { timeout: e.target.value || undefined })}>
              {s.timeout && !timeouts().some((t) => t.v === s.timeout) && <option value={s.timeout}>{s.timeout}</option>}
              {timeouts().filter((t) => kind !== "human" || t.v).map((t) => <option key={t.v} value={t.v}>{t.label}</option>)}
              {kind === "human" && !s.timeout && <option value="">{L("3 天", "3 days")}</option>}
            </select></label>
          {kind !== "human" && (
            <label className="wf-edit-cell"><span>{L("失败了再试", "Retries")}</span>
              <select className="app-select-field" value={s.retry || 0} disabled={busy} onChange={(e) => setStep(i, { retry: +e.target.value || undefined })}>
                {[0, 1, 2, 3].map((k) => <option key={k} value={k}>{k ? L(`${k} 次`, `${k}`) : L("不试", "No")}</option>)}
              </select></label>
          )}
        </div>
        {kind !== "human" ? (
          <label className="app-field"><span className="app-field-label">{kind === "computer" ? L("跑什么命令", "The command") : L("要它做什么", "What it is asked")}</span>
            <textarea className={cn("app-input au-body", kind === "computer" && "mono")} rows={kind === "computer" ? 2 : 5} disabled={busy} value={(kind === "computer" ? s.run : s.prompt) || ""}
              aria-label={kind === "computer" ? L(`第 ${n} 步跑的命令`, `Step ${n} command`) : L(`第 ${n} 步要它做什么`, `What step ${n} is asked`)}
              onChange={(e) => setStep(i, kind === "computer" ? { run: e.target.value } : { prompt: e.target.value })} />
            <span className="app-small-note">{L("{{inputs.task}} 是运行时填的，{{steps.审.summary}} 是前面一步交回的；工作区按这些替换后交出去。", "{{inputs.task}} is what the run was given, {{steps.review.summary}} what a step before handed back; the workspace fills them in.")}</span>
          </label>
        ) : (
          <label className="app-field"><span className="app-field-label">{L("问什么", "The question")}</span>
            <textarea className="app-input au-body" rows={3} disabled={busy} value={s.prompt || ""} onChange={(e) => setStep(i, { prompt: e.target.value })} /></label>
        )}
        {kind === "agent" && (
          <ContractEditor key={`out-${s.id}`} schema={s.output || s.outputSchema} disabled={busy} label={L("交回什么", "What it hands back")} testid="workflow-edit-output"
            onChange={(c) => setStep(i, s.output ? { output: c } : { outputSchema: c })} />
        )}
        {kind !== "human" && (
          <details className="app-advanced wf-edit-more">
            <summary>{L("收到的输入要符合（可选）", "Its input must fit (optional)")}</summary>
            <ContractEditor key={`in-${s.id}`} schema={s.inputSchema} disabled={busy} label={L("收到的输入", "Its input")}
              onChange={(c) => setStep(i, { inputSchema: c })} />
          </details>
        )}
      </div>
    );
  })();

  return (
    <div className="wf-ed" data-testid="workflow-edit">
      <header className="pcd-head wf-ed-head">
        <div className="pcd-titles">
          <h1><span>{L(`编辑「${wf.title || wf.name}」`, `Edit "${wf.title || wf.name}"`)}</span></h1>
          <p className="pcd-sub"><span>{L(`保存成第 ${wf.version + 1} 版；正在跑的照旧用第 ${wf.version} 版。加减步骤让写它的 agent 改。`, `Saved as version ${wf.version + 1}; runs going on keep version ${wf.version}. To add or remove steps, ask the agent that wrote it.`)}</span></p>
        </div>
        <div className="ph-actions wf-actions">
          {noRole && <span className="app-modal-hint">{L(`「${label(noRole)}」还没写分工`, `"${label(noRole)}" needs a role`)}</span>}
          <button type="button" className="app-btn" onClick={close} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void save()} disabled={busy || !def || !!noRole || !changed}>{busy ? L("正在保存…", "Saving…") : L("保存新版本", "Save a new version")}</button>
        </div>
      </header>
      <ErrorText error={error} />
      {!def ? (!error && <RowsSkeleton rows={3} className="pc-list" />) : (
        <div className="wf-ed-body">
          <div className="wf-ed-map">
            <div className="wf-ed-pick" role="tablist" aria-label={L("改哪里", "What to edit")}>
              <button type="button" role="tab" aria-selected={picked === BASIC} className={cn("wf-ed-chip", picked === BASIC && "is-on")} onClick={() => setPicked(BASIC)}>{L("基本", "Basics")}</button>
              {def.steps.map((x, k) => (
                <button key={x.id} type="button" role="tab" aria-selected={picked === x.id} className={cn("wf-ed-chip", picked === x.id && "is-on")} onClick={() => setPicked(x.id)}
                  data-step={x.id}>{k + 1}. {label(x)}</button>
              ))}
            </div>
            <div className="aud-card wf-graph-card">
              <WorkflowGraph def={def} hubId={hubId} rows={rows} selected={picked === BASIC ? undefined : picked} onSelect={(id) => setPicked((p) => (p === id ? BASIC : id))} />
            </div>
            <p className="app-small-note">{L("点图里的一步来改它。", "Click a step in the drawing to edit it.")}</p>
          </div>
          <div ref={panelRef} className="wf-ed-side">{picked === BASIC || !step ? basic : step}</div>
        </div>
      )}
      <ConfirmDialog open={leaving} onOpenChange={setLeaving} title={L("不保存就离开？", "Leave without saving?")}
        description={L("改的这些会丢掉。", "What you changed is lost.")} confirmLabel={L("离开", "Leave")} onConfirm={() => { setLeaving(false); onClose(); }} />
    </div>
  );
}

const BASIC = "$basic";

/* ---------- one workflow: its runs, how it goes, its steps, its contracts ---------- */

/** What a run is about, in a line: what it was given (the first text). */
export function runSubject(r: Pick<WorkflowRun, "inputs">): string {
  const vals = Object.entries(r.inputs || {});
  const pick = vals.find(([k, v]) => typeof v === "string" && v.trim() && /^(task|what|title|goal|request)$/i.test(k))
    || vals.find(([, v]) => typeof v === "string" && v.trim());
  const text = pick ? String(pick[1]).replace(/\s+/g, " ").trim() : "";
  return text.length > 90 ? `${text.slice(0, 89)}…` : text;
}

/** Where a run is now, in words: the steps going (by their names). */
function runWhere(r: WorkflowRun, def?: WorkflowDef | null): string {
  if (!r.current?.length) return "";
  const name = (id: string) => def?.steps.find((s) => s.id === id)?.title || id;
  return r.current.map(name).join("、");
}

type DetailTab = "runs" | "stats" | "flow" | "io";

function WorkflowDetail({ hubId, wf, rows, now, stamp, busy, error, onRun, onToggle, onDelete, onEdit, onOpenRun }: {
  hubId: string; wf: Workflow; rows: AgentRow[]; now: number; stamp: number; busy: boolean; error: unknown;
  onBack: () => void; onRun: () => void; onToggle: () => void; onDelete: () => void; onEdit: () => void; onOpenRun: (id: string) => void;
}) {
  const [def, setDef] = useState<WorkflowDef | null>(null);
  const [runs, setRuns] = useState<WorkflowRun[] | null>(null);
  const [filter, setFilter] = useState<RunsFilter>("");
  // A workflow that has run opens on its runs; a new one on how it goes.
  const [tab, setTab] = useState<DetailTab>(() => (wf.last_run || wf.active ? "runs" : "flow"));
  const more = useContextMenu();
  useEffect(() => {
    let off = false;
    appApi.workflow(hubId, wf.id).then((w) => { if (!off) setDef(w.definition || null); }, () => undefined);
    return () => { off = true; };
  }, [hubId, wf.id, wf.version, stamp]);
  useEffect(() => {
    let off = false;
    setRuns(null);
    const opts = filter === "active" ? { active: true } : filter ? { status: filter } : {};
    const read = () => appApi.workflowRuns(hubId, { workflow: wf.id, limit: 50, ...opts }).then((r) => { if (!off) setRuns(r.runs || []); }, () => { if (!off) setRuns((x) => x ?? []); });
    read();
    const t = setInterval(read, 5_000);
    return () => { off = true; clearInterval(t); };
  }, [hubId, wf.id, wf.version, stamp, filter]);
  const tabs: { key: DetailTab; label: string; n?: number }[] = [
    { key: "runs", label: L("运行记录", "Runs"), n: runs && !filter ? runs.length : undefined },
    { key: "stats", label: L("运行情况", "How it goes") },
    { key: "flow", label: L("流程", "Steps") },
    { key: "io", label: L("输入和输出", "Inputs and outputs") },
  ];
  return (
    <>
      <header className={cn("pcd-head", !wf.enabled && "is-away")}>
        <div className="pcd-titles">
          <h1><span>{wf.title || wf.name}</span></h1>
          <p className="pcd-sub">
            <span className={cn("pc-online", wf.enabled && "is-on")}>{wf.enabled ? L("在用", "On") : L("已停用", "Off")}</span>
            {wf.title && wf.title !== wf.name && <span className="mono">{wf.name}</span>}
            {(wf.tags || []).map((t) => <span key={t} className="app-label">{t}</span>)}
            <span>{L(`${wf.steps} 步`, `${wf.steps} steps`)}</span>
            <span>{L(`第 ${wf.version} 版`, `Version ${wf.version}`)}</span>
            {wf.project && <span>{L(`项目 ${wf.project}`, `Project ${wf.project}`)}</span>}
          </p>
        </div>
        {/* Three buttons of one size: what is done most (运行), what is
            done now and then (编辑), and the rest under ⋯ (停用, 删除). */}
        <div className="ph-actions wf-actions">
          <button type="button" className="app-btn" onClick={onEdit}><Pencil />{L("编辑", "Edit")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={onRun} disabled={!wf.enabled}><Play />{L("运行", "Run")}</button>
          <button type="button" className="app-btn wf-more" aria-label={L("更多", "More")} title={L("更多", "More")} data-testid="workflow-more" disabled={busy}
            onClick={(e) => more.openUnder(e.currentTarget, [
              wf.enabled
                ? { label: L("停用", "Turn off"), icon: Pause, onSelect: onToggle }
                : { label: L("启用", "Turn on"), icon: Play, onSelect: onToggle },
              "sep",
              { label: L("删除这个工作流…", "Delete this workflow…"), icon: Trash2, danger: true, onSelect: onDelete },
            ])}><MoreHorizontal /></button>
        </div>
      </header>
      {wf.description && <p className="wf-desc">{wf.description}</p>}
      <ErrorText error={error} />

      <div className="wf-tabs" role="tablist" aria-label={L("看什么", "View")} data-testid="workflow-tabs">
        {tabs.map((t) => (
          <button key={t.key} type="button" role="tab" aria-selected={tab === t.key} className={cn("wf-tab", tab === t.key && "is-on")} onClick={() => setTab(t.key)}>
            {t.label}{t.n !== undefined && t.n > 0 && <small>{t.n}</small>}
          </button>
        ))}
      </div>

      {tab === "runs" && (
        <>
          {(filter || (runs && runs.length > 0)) && <RunsFilterChips value={filter} onChange={setFilter} />}
          {runs === null ? <RowsSkeleton rows={3} className="pc-list" /> : runs.length === 0 && filter ? (
            <div className="app-empty-note">{L("没有这样的运行。", "No runs like that.")}</div>
          ) : runs.length === 0 ? (
            <div className="pcd-empty" data-testid="workflow-no-runs">
              <span>{L("还没运行过。点「运行」开始一次，每一步交给谁、做得怎么样都会记在这里。", "It has not run yet. Run it once: who each step went to and how it went is kept here.")}</span>
              <button type="button" className="app-btn app-btn-small" onClick={onRun} disabled={!wf.enabled}><Play />{L("运行", "Run")}</button>
            </div>
          ) : (
            <div className="pc-list wf-runs" data-testid="workflow-runs">
              {runs.map((r) => {
                const subject = runSubject(r);
                const where = runWhere(r, def);
                return (
                  <button key={r.id} type="button" className="wf-run-row" onClick={() => onOpenRun(r.id)} data-status={r.status}>
                    <RunMark status={r.status} />
                    <span className="wf-run-text">
                      <span className="wf-run-subject">{subject || L("（没有填内容）", "(nothing given)")}</span>
                      <span className="wf-run-meta">
                        <span className={cn("wf-run-state", runTone(r.status))}>{runWords(r.status)}</span>
                        {where && (r.status === "running" || r.status === "waiting") && <span>{L(`在「${where}」`, `at ${where}`)}</span>}
                        {r.status === "failed" && r.error && <span className="wf-run-why" title={r.error}>{r.error}</span>}
                        {r.by && <span>{L(`${r.by} 启动`, `by ${r.by}`)}</span>}
                      </span>
                    </span>
                    <span className="aud-run-when" title={new Date(r.created_at).toLocaleString()}>{timeAgo(r.created_at, now)}<small>{L(`用了 ${r.duration_ms !== undefined && r.ended_at ? msWords(r.duration_ms) : duration(r.created_at, r.ended_at, now)}`, `took ${r.duration_ms !== undefined && r.ended_at ? msWords(r.duration_ms) : duration(r.created_at, r.ended_at, now)}`)}</small></span>
                  </button>
                );
              })}
            </div>
          )}
        </>
      )}

      {tab === "stats" && (
        <div data-testid="workflow-stats-tab">
          <WorkflowStatsSection hubId={hubId} wf={wf} stamp={stamp} now={now} onOpenRun={onOpenRun} />
          {runs !== null && runs.length === 0 && !filter && <div className="app-empty-note">{L("跑过几次之后，这里会有成功率、用时和花费。", "After a few runs: success rate, timing and cost.")}</div>}
        </div>
      )}

      {tab === "flow" && (
        <>
          <div className="aud-card wf-graph-card">
            {def ? <WorkflowGraph def={def} hubId={hubId} rows={rows} /> : <div className="wf-graph-skel"><span className="skel skel-line" style={{ width: "40%" }} /></div>}
          </div>
          <p className="app-small-note wf-edit-note">{L("「编辑」可以换人、改超时和轮数。", "Edit changes who does what, timeouts and rounds.")}</p>
        </>
      )}

      {tab === "io" && (def ? <ContractsSection def={def} hubId={hubId} rows={rows} /> : <RowsSkeleton rows={2} className="pc-list" />)}
      {more.element}
    </>
  );
}

/** A run's state at a glance: a tick when done, a cross when failed, a
 *  dot when going or waiting. */
function RunMark({ status }: { status: WorkflowRun["status"] | RunStep["status"] }) {
  const t = status === "succeeded" || status === "done" ? "is-done" : status === "failed" ? "is-bad" : status === "running" ? "is-ok" : status === "waiting" ? "is-wait" : "is-off";
  return (
    <span className={cn("wf-mark", t)} aria-hidden>
      {t === "is-done" ? <Check /> : t === "is-bad" ? <X /> : t === "is-off" ? <Minus /> : <i />}
    </span>
  );
}

/* ---------- one run: how each step went ---------- */

function RunDetail({ hubId, runId, wf, rows, now, stamp, onBack, onChanged, onOpenMessage, onOpenRun }: {
  hubId: string; runId: string; wf: Workflow; rows: AgentRow[]; now: number; stamp: number;
  onBack: () => void; onChanged: () => void; onOpenMessage?: (agent: string, messageId: string) => void; onOpenRun: (id: string) => void;
}) {
  const [run, setRun] = useState<WorkflowRun | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState("");
  const [picked, setPicked] = useState<string | undefined>();
  const [open, setOpen] = useState<string | null>(null);
  const [logOpen, setLogOpen] = useState(false);
  const [logOf, setLogOf] = useState<RunStep | null>(null);
  const [comparing, setComparing] = useState(false);
  const [view, setView] = useState<"steps" | "timeline">("steps");
  const [mapOpen, setMapOpen] = useState(true);
  const stepsRef = useRef<HTMLDivElement>(null);
  const [n, setN] = useState(0);
  useEffect(() => {
    let off = false;
    const read = () => appApi.workflowRun(hubId, runId).then((r) => { if (!off) setRun(r); }, (e) => { if (!off) setError(e); });
    read();
    const t = setInterval(read, 3_000);
    return () => { off = true; clearInterval(t); };
  }, [hubId, runId, stamp, n]);
  const states = useMemo(() => stepStates(run?.steps), [run?.steps]);
  const act = async (what: "cancel" | "retry") => {
    setBusy(what); setError(null);
    try {
      const r = what === "cancel" ? await appApi.cancelWorkflowRun(hubId, runId) : await appApi.retryWorkflowRun(hubId, runId);
      setRun(r); onChanged(); setN((x) => x + 1);
    } catch (e) { setError(e); } finally { setBusy(""); }
  };
  const rowOf = (name?: string) => (name ? rows.find((r) => r.hub.id === hubId && r.st.agent.name === name) : undefined);
  const going = run && (run.status === "running" || run.status === "waiting");
  const steps = run?.steps || [];
  const shown = picked ? steps.filter((s) => s.step === picked) : steps;
  const waiting = steps.filter((s) => s.status === "waiting");
  const inputs = Object.entries(run?.inputs || {});
  // The title says what this run is about (what it was given); its state
  // is the first thing under it.
  const subject = run ? runSubject(run) : "";
  return (
    <>
      <header className="pcd-head">
        <div className="pcd-titles">
          <h1><span title={subject || undefined}>{subject || (run ? L(`「${wf.title || wf.name}」的一次运行`, `A run of "${wf.title || wf.name}"`) : L("运行", "Run"))}</span></h1>
          {run && (
            <p className="pcd-sub">
              <button type="button" className="wf-link" onClick={onBack} title={L("打开这个工作流", "Open this workflow")}>{wf.title || wf.name}</button>
              <span className={cn("wf-run-state", runTone(run.status))} data-testid="workflow-run-state">{runWords(run.status)}</span>
              <span title={new Date(run.created_at).toLocaleString()}>{L(`${timeAgo(run.created_at, now)}开始`, `Started ${timeAgo(run.created_at, now)}`)}</span>
              <span>{L(`用了 ${duration(run.created_at, run.ended_at, now)}`, `Took ${duration(run.created_at, run.ended_at, now)}`)}</span>
              {run.by && <span>{run.trigger === "agent" ? L(`${run.by} 启动的`, `Started by ${run.by}`) : L(`${run.by} 启动的`, `Started by ${run.by}`)}</span>}
              <span>{L(`第 ${run.version} 版`, `Version ${run.version}`)}</span>
            </p>
          )}
        </div>
        <div className="ph-actions wf-actions">
          {run?.metrics && <CompareButton onClick={() => setComparing(true)} />}
          {going && <button type="button" className="app-btn" disabled={!!busy} onClick={() => void act("cancel")}><Square />{busy === "cancel" ? L("正在停…", "Stopping…") : L("取消运行", "Cancel run")}</button>}
          {run && (run.status === "failed" || run.status === "cancelled") && (
            <button type="button" className="app-btn app-btn-primary" disabled={!!busy} onClick={() => void act("retry")}
              title={L("从停下的那一步重新开始，前面做完的保留", "Start again at the step where it stopped, keeping what was done before")}><RotateCcw />{busy === "retry" ? L("正在开始…", "Starting…") : L("从这一步重来", "Retry from there")}</button>
          )}
        </div>
      </header>
      <ErrorText error={error} />
      {inputs.length > 1 && (
        <div className="aud-card wf-run-inputs" data-testid="workflow-run-inputs">
          {inputs.map(([k, v]) => (
            <div key={k} className="app-kv"><span className="app-kv-label">{run?.definition?.inputs?.properties?.[k]?.title || k}</span>
              <span className="app-kv-value wf-input-value">{typeof v === "string" ? v : JSON.stringify(v)}</span></div>
          ))}
        </div>
      )}
      {run?.error && <div className={cn("wf-run-error", run.status === "cancelled" && "is-quiet")}>{run.error}</div>}
      {run && <RunMetricsStrip run={run} />}

      {waiting.map((s) => (
        <DecisionCard key={s.id} hubId={hubId} d={{ run: run!, step: s }} now={now} onDone={() => { setN((x) => x + 1); onChanged(); }} />
      ))}

      <div ref={stepsRef} className="wf-steps-anchor" />
      <SectionHead title={picked ? L(`第 ${(run?.definition?.steps.findIndex((s) => s.id === picked) ?? 0) + 1} 步的每一次`, "Each time of this step") : L("每一步", "Each step")} count={shown.length}>
        {picked && <button type="button" className="app-group-action" onClick={() => setPicked(undefined)}>{L("看全部", "Show all")}</button>}
        {!picked && run?.metrics && steps.length > 0 && (
          <div className="seg seg-sm" role="tablist" aria-label={L("怎么看", "View")} data-testid="workflow-steps-view">
            <button type="button" role="tab" aria-selected={view === "steps"} className={cn("seg-btn", view === "steps" && "active")} onClick={() => setView("steps")}>{L("列表", "List")}</button>
            <button type="button" role="tab" aria-selected={view === "timeline"} className={cn("seg-btn", view === "timeline" && "active")} onClick={() => setView("timeline")}>{L("时间线", "Timeline")}</button>
          </div>
        )}
      </SectionHead>
      {run && !picked && view === "timeline" && run.metrics ? <RunTimeline run={run} now={now} onPick={setLogOf} /> : run === null ? <RowsSkeleton rows={3} className="pc-list" /> : (
        <div className="pc-list wf-steps" data-testid="workflow-steps">
          {shown.map((s) => (
            <StepRow key={s.id} s={s} row={rowOf(s.target)} now={now} open={open === s.id} onToggle={() => setOpen((o) => (o === s.id ? null : s.id))}
              contract={(() => { const d = run.definition?.steps.find((x) => x.id === s.step); return !!(d?.output || d?.outputSchema); })()}
              onLog={run.metrics && s.kind !== "human" ? () => setLogOf(s) : undefined}
              onChat={s.message_id && s.target && onOpenMessage ? () => onOpenMessage(s.target!, s.message_id!) : undefined} />
          ))}
          {shown.length === 0 && <div className="app-empty-note">{L("这一步还没走到。", "Not reached yet.")}</div>}
        </div>
      )}

      <SectionHead title={L("流程图", "The steps, drawn")}>
        <button type="button" className="app-group-action" aria-expanded={mapOpen} onClick={() => setMapOpen((o) => !o)}>
          {mapOpen ? L("收起", "Hide") : L("展开", "Show")}<ChevronDown className={cn("aud-chev", mapOpen && "is-open")} />
        </button>
      </SectionHead>
      {mapOpen && (
        <div className="aud-card wf-graph-card">
          {run?.definition ? <WorkflowGraph def={run.definition} hubId={hubId} rows={rows} states={states} selected={picked}
            onSelect={(id) => { setPicked((p) => (p === id ? undefined : id)); requestAnimationFrame(() => stepsRef.current?.scrollIntoView({ block: "start", behavior: "smooth" })); }} />
            : <div className="wf-graph-skel"><span className="skel skel-line" style={{ width: "40%" }} /></div>}
        </div>
      )}
      {mapOpen && <p className="app-small-note wf-edit-note">{L("点图里的一步，上面只看它的每一次。", "Click a step in the drawing to see only its times above.")}</p>}

      {run && <RunArtifacts hubId={hubId} run={run} />}
      {run && <RunScores hubId={hubId} run={run} onAdded={() => setN((x) => x + 1)} />}

      {run?.events && run.events.length > 0 && (
        <>
          <SectionHead title={L("运行日志", "Log")} count={run.events.length}>
            <button type="button" className="app-group-action" aria-expanded={logOpen} onClick={() => setLogOpen((o) => !o)}>
              {logOpen ? L("收起", "Hide") : L("展开", "Show")}<ChevronDown className={cn("aud-chev", logOpen && "is-open")} />
            </button>
          </SectionHead>
          {logOpen && (
            <div className="aud-card wf-log" data-testid="workflow-log">
              {run.events.map((e, i) => (
                <div key={i} className="wf-log-line">
                  <span className="wf-log-at" title={new Date(e.at).toLocaleString()}>{new Date(e.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false })}</span>
                  {e.step && <span className="wf-log-step mono">{e.step}</span>}
                  <span className="wf-log-text">{e.text}</span>
                </div>
              ))}
            </div>
          )}
        </>
      )}
      {logOf && <StepLogDialog hubId={hubId} runId={runId} step={logOf} onClose={() => setLogOf(null)} />}
      {comparing && run && <CompareDialog hubId={hubId} run={run} onClose={() => setComparing(false)} onOpenRun={onOpenRun} />}
    </>
  );
}

function StepRow({ s, row, now, open, onToggle, onChat, onLog, contract }: {
  s: RunStep; row?: AgentRow; now: number; open: boolean; onToggle: () => void; onChat?: () => void; onLog?: () => void;
  /** its result must fit a schema */
  contract?: boolean;
}) {
  const who = s.kind === "human" ? (s.target || L("你", "You")) : row ? nodeName(row.st.agent, row.hub) : s.target || "";
  const tone = s.status === "running" ? "is-ok" : s.status === "waiting" ? "is-wait" : s.status === "failed" ? "is-bad" : "";
  const text = s.status === "failed" ? s.error : s.summary || (s.status === "running" ? L("在做…", "Working…") : s.status === "waiting" ? L("等你决定", "Waiting for you") : s.error);
  const output = s.output ? Object.fromEntries(Object.entries(s.output).filter(([k]) => k !== "text")) : null;
  return (
    <div className={cn("aud-run wf-step", open && "is-open")} data-testid="workflow-step">
      <div className="aud-run-line">
        <RunMark status={s.status} />
        <span className="wf-step-icon">
          {s.kind === "agent" && row ? <Avatar agent={row.st.agent} size={22} badge={false} /> : s.kind === "computer" ? <Monitor /> : s.kind === "human" ? <UserCheck /> : <Bot />}
        </span>
        <span className="wf-step-text">
          <span className="wf-step-name">
            <b>{s.title || s.step}</b>{s.attempt > 1 && <small>{L(` 第 ${s.attempt} 次`, ` #${s.attempt}`)}</small>}
            <span className="wf-step-who">{who}</span>
          </span>
          <span className={cn("wf-step-summary", tone)}>{text || stateWords(s.status)}</span>
        </span>
        <span className="aud-run-when" title={new Date(s.started_at).toLocaleString()}>{timeAgo(s.started_at, now)}
          <small>{s.duration_ms !== undefined && s.ended_at ? msWords(s.duration_ms) : duration(s.started_at, s.ended_at, now)}{(s.attempts || 1) > 1 && L(` · ${s.attempts} 次`, ` · ×${s.attempts}`)}{s.usage && ` · ${costWords(s.usage)}`}</small></span>
        <span className="aud-run-actions wf-step-actions">
          {(s.input || output || s.payload !== undefined) && (
            <button type="button" className="app-btn app-btn-quiet app-btn-small" aria-expanded={open} onClick={onToggle}>
              {L("详情", "Details")}<ChevronDown className={cn("aud-chev", open && "is-open")} />
            </button>
          )}
          {onLog && <LogButton onClick={onLog} />}
          {onChat && <button type="button" className="app-btn app-btn-quiet app-btn-small" onClick={onChat}><MessageSquare />{L("看对话", "Chat")}</button>}
        </span>
      </div>
      {open && (
        <div className="aud-call wf-step-detail">
          {s.payload !== undefined && s.payload !== null && !(typeof s.payload === "object" && Object.keys(s.payload as object).length === 0) && <>
            <span className="wf-step-label">{L("收到的输入", "Its input")}</span>
            <div className="aud-call-body"><pre>{JSON.stringify(s.payload, null, 2)}</pre></div></>}
          {s.input && <><span className="wf-step-label">{s.kind === "computer" ? L("跑的命令", "The command") : s.kind === "human" ? L("问你的", "The question") : L("交给它的任务", "The task it got")}</span>
            <div className="aud-call-body"><pre>{s.input}</pre></div></>}
          {output && Object.keys(output).length > 0 && <><span className="wf-step-label">{L("交回来的结果", "What it handed back")}
            {contract && s.status === "done" && <span className="wfc-ok">{L("符合约定", "Fits its contract")}</span>}</span>
            <div className="aud-call-body"><pre>{JSON.stringify(output, null, 2)}</pre></div></>}
          {(s.bounces || (s.bounced ? 1 : 0)) > 0 && <p className="app-small-note">{L(`交的结果不符合要求，退回去重交过 ${s.bounces || 1} 次。`, `Its result did not fit and was sent back ${s.bounces || 1} time(s).`)}</p>}
        </div>
      )}
    </div>
  );
}


/** Which runs to see: small chips on the left, over the list they filter. */
function RunsFilterChips({ value, onChange }: { value: RunsFilter; onChange: (v: RunsFilter) => void }) {
  const opts: [RunsFilter, string][] = [["", L("全部", "All")], ["active", L("在跑的", "Going")], ["failed", L("失败的", "Failed")], ["succeeded", L("完成的", "Done")]];
  return (
    <div className="wf-chips" role="tablist" aria-label={L("看哪些", "Which runs")} data-testid="workflow-runs-filter">
      {opts.map(([v, label]) => (
        <button key={v || "all"} type="button" role="tab" aria-selected={value === v} className={cn("wf-chip", value === v && "is-on")} onClick={() => onChange(v)}>{label}</button>
      ))}
    </div>
  );
}

