import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavSub } from "@/app/nav";
import { IntroCard, introOf } from "@/app/Onboard";
import { Activity, CalendarDays, ChevronDown, Clock, Coffee, MessageSquare, MoreHorizontal, NotebookPen, Pause, Pencil, Play, Plus, Trash2, Webhook, Workflow as WorkflowIcon } from "lucide-react";
import { useContextMenu, type MenuItem } from "@/app/ContextMenu";
import type { Automation, AutomationRun, WebhookRequest, Workflow, WorkflowDef } from "@/api";
import { ApiError, appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, nodeName } from "@/app/nodes";
import { Avatar } from "@/app/persona";
import { CopyButton } from "@/app/copy";
import { PageHero, SectionHead, SectionSkeleton } from "@/app/PageHero";
import { RowsSkeleton } from "@/app/Skeleton";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { describe, fromCron, localZone, nextRun, soonLabel, toCron, weekChips, type Every } from "@/app/schedule";
import { L } from "@/lib/i18n";
import { bytes, cn, timeAgo } from "@/lib/utils";

/**
 * Automations: a task sent for you to one agent, on a schedule or when an
 * outside system calls its webhook. They run on the workspace's hub, so
 * they go on while your computers are off; what an agent answers is in
 * your chat with it, the task marked 自动.
 *
 * The page is a list of them, each saying what it does in a line: when, to
 * whom, when next, how the last one went; a switch turns one off, ▷ sends it
 * now. With none yet, a few common ones to start from. The editor reads top
 * to bottom as the automation does: to whom, what, when. One opened: what it
 * is set to, then every time it ran, each with how it went, the chat it
 * started and, for a webhook call, what the call carried.
 */

const outcomeWords = (r: AutomationRun) =>
  r.outcome === "sent" ? L("已发送", "Sent")
    : r.outcome === "skipped" ? L(`跳过了${r.error ? `：${skipWhy(r.error)}` : ""}`, `Skipped${r.error ? `: ${r.error}` : ""}`)
      : L(`没发出去${r.error ? `：${r.error}` : ""}`, `Not sent${r.error ? `: ${r.error}` : ""}`);
const skipWhy = (e: string) => (/not finished/.test(e) ? "上一次的任务还没做完" : e);
const triggerWords = (t: AutomationRun["trigger"]) => (t === "schedule" ? L("到点", "On schedule") : t === "webhook" ? L("Webhook", "Webhook") : L("手动", "By hand"));
const outcomeTone = (r: AutomationRun) => (r.outcome === "sent" ? "is-ok" : r.outcome === "failed" ? "is-bad" : "is-skip");

/** Where a new one starts: blank, or one of the common ones. */
type Start = { name: string; body: string; every: Every | null; hook: boolean };
type Editing = Automation | { start: Start };
const blank: Start = { name: "", body: "", every: { kind: "weekdays", hour: 9, minute: 0 }, hook: false };

const templates = (): (Start & { icon: typeof Clock; hint: string })[] => [
  {
    icon: Coffee, name: L("每天早上汇总", "Morning summary"), every: { kind: "weekdays", hour: 9, minute: 0 }, hook: false,
    hint: L("工作日 09:00，把昨天新开的 issue 和 PR 挑重点列给你", "Weekdays at 09:00: yesterday's new issues and PRs, the ones that matter"),
    body: L("看一下昨天新开的 issue 和 PR，挑出要紧的，在这里回我一份清单。", "Look at yesterday's new issues and PRs, pick the ones that matter, and reply with a list."),
  },
  {
    icon: CalendarDays, name: L("每周五写周报", "Friday report"), every: { kind: "weekly", day: 5, hour: 17, minute: 0 }, hook: false,
    hint: L("每周五 17:00，总结这周做了什么、下周打算", "Fridays at 17:00: what got done this week, and what is next"),
    body: L("总结这周做完了什么、还在做什么、下周打算做什么，写成一份简短的周报回我。", "Sum up what got done this week, what is still going, and what is next, as a short report."),
  },
  {
    icon: Activity, name: L("定时巡检", "Regular check"), every: { kind: "hours", every: 4, minute: 0 }, hook: false,
    hint: L("每 4 小时看一眼服务和 CI，有异常才说", "Every 4 hours: services and CI, a word only if something is wrong"),
    body: L("检查一下服务、CI 和依赖有没有异常。一切正常就回一句「正常」，有问题说清楚是什么、影响多大。", "Check the services, CI and dependencies. If all is well, say so in a word; if not, say what is wrong and how much it matters."),
  },
  {
    icon: NotebookPen, name: L("下班前写日报", "End-of-day notes"), every: { kind: "weekdays", hour: 18, minute: 0 }, hook: false,
    hint: L("工作日 18:00，整理今天做完的、卡住的和明天要做的", "Weekdays at 18:00: what got done today, what is stuck, what is next"),
    body: L("整理一下今天做完了什么、哪里卡住了、明天要做什么，写成几行日报回我。", "Write a few lines on what got done today, what is stuck, and what is next."),
  },
  {
    icon: Webhook, name: L("CI 失败时排查", "When CI fails"), every: null, hook: true,
    hint: L("CI 构建失败时调用，让它看日志找原因", "Called when a build fails: it reads the log and finds why"),
    body: L("CI 构建失败了，看一下日志找出原因，能修就修，修不了告诉我卡在哪：\n{{payload}}", "The CI build failed. Read the log and find why; fix it if you can, else tell me where it is stuck:\n{{payload}}"),
  },
];

/** What each hub's page showed last: back on the page, it shows that at
 *  once and refreshes behind it (skeletons only the first time). */
const seen = new Map<string, Automation[]>();

export function AutomationsPage({ hubId, rows, now, onOpenMessage, onOpenRun }: {
  hubId: string; rows: AgentRow[]; now: number;
  /** a task it sent, in its conversation */
  onOpenMessage?: (agent: string, messageId: string) => void;
  /** a workflow run it started, on the 工作流 page */
  onOpenRun?: (workflow: string, run: string) => void;
}) {
  const [list, setList] = useState<Automation[] | null>(() => seen.get(hubId) ?? null);
  const [unsupported, setUnsupported] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [editing, setEditing] = useState<Editing | null>(null);
  const [busy, setBusy] = useState("");
  const [removing, setRemoving] = useState<Automation | null>(null);
  // The one opened (its page), and a count that makes it read its runs again.
  const [openId, setOpenId] = useState<string | null>(null);
  // ← → at the top (nav.ts): the automation open is part of where you are.
  useNavSub("automations", { open: openId }, { open: null }, (v) => setOpenId(v.open));
  const [stamp, setStamp] = useState(0);
  const menu = useContextMenu();
  const load = useCallback(() => {
    appApi.automations(hubId).then((r) => {
      seen.set(hubId, r.automations || []); setList(r.automations || []); setUnsupported(false);
      // (the 上手清单 ticks 设一个自动化 from this, at once)
      window.dispatchEvent(new CustomEvent("myco-automations", { detail: { hubId, count: (r.automations || []).length } }));
    }, (e) => {
      if (e instanceof ApiError && e.status === 404) setUnsupported(true);
      else setError(e);
      setList([]);
    });
  }, [hubId]);
  useEffect(() => { setList(seen.get(hubId) ?? null); load(); const t = setInterval(load, 20_000); return () => clearInterval(t); }, [hubId, load]);
  const rowOf = (n: string) => rows.find((x) => x.hub.id === hubId && x.st.agent.name === n);

  const toggle = async (a: Automation) => {
    setBusy(a.id);
    setError(null);
    try { await appApi.updateAutomation(hubId, a.id, { enabled: !a.enabled }); load(); setStamp((n) => n + 1); } catch (e) { setError(e); } finally { setBusy(""); }
  };
  const runNow = async (a: Automation) => {
    setBusy(a.id);
    setError(null);
    try { await appApi.runAutomation(hubId, a.id); load(); setStamp((n) => n + 1); } catch (e) { setError(e); } finally { setBusy(""); }
  };

  const remove = async (a: Automation) => {
    setBusy(a.id);
    setError(null);
    try { await appApi.deleteAutomation(hubId, a.id); setRemoving(null); if (openId === a.id) setOpenId(null); load(); } catch (e) { setError(e); setRemoving(null); } finally { setBusy(""); }
  };

  const gallery = (
    <div className="au-templates">
      {templates().map((t) => (
        <button key={t.name} type="button" className="au-template" onClick={() => setEditing({ start: t })}>
          <span className="tp-icon"><t.icon /></span>
          <span className="au-template-text">
            <span className="au-template-name">{t.name}</span>
            <span className="au-template-hint">{t.hint}</span>
          </span>
        </button>
      ))}
      <button type="button" className="au-template is-blank" onClick={() => setEditing({ start: blank })}>
        <span className="tp-icon"><Plus /></span>
        <span className="au-template-text">
          <span className="au-template-name">{L("从空白开始", "Start from blank")}</span>
          <span className="au-template-hint">{L("自己写要它做什么、什么时候做", "Write what it does, and when")}</span>
        </span>
      </button>
    </div>
  );
  const opened = openId ? (list || []).find((a) => a.id === openId) : undefined;
  const dialogs = (
    <>
      <AutomationEditor hubId={hubId} rows={rows} editing={editing} now={now} onClose={() => setEditing(null)} onChanged={() => { load(); setStamp((n) => n + 1); }} />
      <ConfirmDialog open={!!removing} onOpenChange={(o) => !o && setRemoving(null)} title={L("删除这个自动化？", "Delete this automation?")}
        description={L("它不会再发消息了，Webhook 地址也失效。已经发出去的消息和 agent 的回复都保留。", "It sends nothing more, and its webhook address stops working. What it sent and the answers stay.")}
        confirmLabel={L("删除", "Delete")} onConfirm={() => removing && void remove(removing)} busy={!!removing && busy === removing.id} />
      {menu.element}
    </>
  );
  if (opened) {
    return (
      <div className="pc au pcd aud" data-testid="automation-detail">
        <AutomationDetail hubId={hubId} a={opened} row={rowOf(opened.to)} now={now} stamp={stamp} busy={busy === opened.id} error={error}
          onBack={() => setOpenId(null)} onEdit={() => setEditing(opened)} onRun={() => void runNow(opened)} onToggle={() => void toggle(opened)}
          onDelete={() => setRemoving(opened)} onOpenMessage={onOpenMessage} onOpenRun={onOpenRun} />
        {dialogs}
      </div>
    );
  }

  const on = (list || []).filter((a) => a.enabled).length;
  const last = (list || []).map((a) => a.last_run).filter(Boolean).sort((x, y) => (y!.at > x!.at ? 1 : -1))[0];

  return (
    <div className="pc au">
      <PageHero title={L("自动化", "Automations")}
        subtitle={L("让 agent 按时、按事件自己干活。跑在工作区的 Hub 上，电脑关着也照常。", "Agents at work on a schedule or when something happens. It runs on the workspace's hub, so it goes on while your computers are off.")}>
        {!unsupported && <button type="button" className="app-btn app-btn-primary" onClick={() => setEditing({ start: blank })}><Plus />{L("新建自动化", "New automation")}</button>}
      </PageHero>
      {!unsupported && !!list?.length && <IntroCard id="automations" {...introOf("automations")} />}
      <ErrorText error={error} />
      {unsupported ? (
        <div className="app-empty" data-testid="automations-unsupported">
          <div className="app-empty-icon"><Clock /></div>
          <h2>{L("这个工作区还不能用自动化", "No automations in this workspace yet")}</h2>
          <p>{L("它的 Hub 版本太旧。在服务器上运行 sudo myco upgrade，升级到 v0.19.22 或更新。", "Its hub is too old: run sudo myco upgrade on the server (v0.19.22 or later).")}</p>
        </div>
      ) : list === null ? (
        <>
          <div className="au-stats" aria-hidden>{[0, 1, 2].map((i) => <div key={i} className="au-stat"><span className="skel skel-line is-thin" style={{ width: "40%" }} /><span className="skel skel-line" style={{ width: "60%", height: 18 }} /></div>)}</div>
          <SectionSkeleton />
          <RowsSkeleton rows={3} className="pc-list" />
        </>
      ) : list.length === 0 ? (
        <div data-testid="automations-empty">
          <div className="au-none">
            <span className="tp-icon"><Clock /></span>
            <div>
              <b>{L("还没有自动化", "No automations yet")}</b>
              <p>{L("比如每天早上汇总 issue、CI 失败时看日志。它的回复在你和它的对话里，标着「自动」。从下面挑一个开始。", "Say, a morning summary of issues, or a look at the log when CI fails. Its answers are in your chat with it, marked 自动. Start from one below.")}</p>
            </div>
          </div>
          <SectionHead title={L("从模板开始", "Start from one of these")} />
          {gallery}
        </div>
      ) : (
        <>
          <div className="au-stats">
            <div className="au-stat"><span>{L("在运行", "Running")}</span><b>{on}</b></div>
            <div className="au-stat"><span>{L("已暂停", "Paused")}</span><b className={on === list.length ? "is-quiet" : ""}>{list.length - on}</b></div>
            <div className="au-stat"><span>{L("最近一次", "Last run")}</span>
              {last ? <b className={cn("au-stat-last", outcomeTone(last))}>{timeAgo(last.at, now)}<small>{outcomeWords(last)}</small></b> : <b className="is-quiet">—</b>}
            </div>
          </div>
          <SectionHead title={L("我的自动化", "Your automations")} count={list.length} />
        <div className="pc-list" data-testid="automations">
          {list.map((a) => {
            const r = rowOf(a.to);
            const rowMenu = (x: Automation): MenuItem[] => [
              { label: L("查看运行记录", "Runs"), icon: Activity, onSelect: () => setOpenId(x.id) },
              { label: L("编辑", "Edit"), icon: Pencil, onSelect: () => setEditing(x) },
              { label: x.enabled ? L("暂停", "Pause") : L("启用", "Resume"), icon: x.enabled ? Pause : Play, onSelect: () => void toggle(x), disabled: busy === x.id },
              "sep",
              { label: L("删除", "Delete"), icon: Trash2, danger: true, onSelect: () => setRemoving(x) },
            ];
            const when = [a.schedule ? describe(a.schedule.cron) : "", a.webhook ? L("Webhook 调用时", "On webhook") : ""].filter(Boolean).join(L("，或", ", or "));
            const Icon = a.schedule ? Clock : Webhook;
            return (
              <div key={a.id} className={cn("au-row", !a.enabled && "is-off")} onContextMenu={(e) => {
                e.preventDefault();
                menu.open(e, rowMenu(a));
              }}>
                <button type="button" className="au-row-main" onClick={() => setOpenId(a.id)} title={L("打开：设置和每一次运行", "Open: its settings and every run")}>
                  <span className={cn("au-icon", !a.schedule && "is-hook")}><Icon /></span>
                  <span className="au-row-text">
                    <span className="au-row-name">{a.name}</span>
                    <span className="au-row-what">
                      <span>{when}</span>
                      {a.workflow ? (
                        <span className="au-row-to">{L("启动工作流", "starts")}<WorkflowIcon className="au-row-wf" /><b>{a.workflow_name || a.workflow}</b></span>
                      ) : (
                        <span className="au-row-to">
                          {L("发给", "to")}
                          {r && <Avatar agent={r.st.agent} size={16} badge={false} />}
                          <b>{r ? nodeName(r.st.agent, r.hub) : a.to}</b>
                        </span>
                      )}
                    </span>
                    <span className="au-row-status">
                      {!a.enabled ? <span>{L("已暂停", "Paused")}</span>
                        : a.next_run_at ? <span>{L(`下次 ${soonLabel(new Date(a.next_run_at), new Date(now))}`, `Next ${soonLabel(new Date(a.next_run_at), new Date(now))}`)}</span>
                          : a.webhook ? <span>{L("等外部调用", "Waiting for a call")}</span> : null}
                      {a.last_run && (
                        <span className={cn("au-last", outcomeTone(a.last_run))}>
                          <i aria-hidden />{L(`上次 ${timeAgo(a.last_run.at, now)} · `, `Last ${timeAgo(a.last_run.at, now)} · `)}{outcomeWords(a.last_run)}
                        </span>
                      )}
                    </span>
                  </span>
                </button>
                {/* 运行 and ⋯ (edit, pause, delete), as a workflow's row */}
                <span className="wf-row-actions">
                  <button type="button" className="app-btn app-btn-small wf-run-now" disabled={busy === a.id} onClick={() => void runNow(a)}
                    title={L("立即运行：现在就发一次", "Run now: send it once now")}><Play />{L("运行", "Run")}</button>
                  <button type="button" className="app-btn app-btn-small wf-more" aria-label={L(`${a.name}：更多`, `${a.name}: more`)}
                    title={L("编辑、暂停、删除", "Edit, pause, delete")} onClick={(e) => menu.openUnder(e.currentTarget, rowMenu(a))}><MoreHorizontal /></button>
                </span>
              </div>
            );
          })}
        </div>
          <SectionHead title={L("从模板开始", "Start from one of these")} />
          {gallery}
        </>
      )}
      {dialogs}
    </div>
  );
}

/** When, as a person says it: 今天 09:00, 昨天 18:30, 11月3日 09:00. */
function whenAt(iso: string, now: number): string {
  const d = new Date(iso);
  const hm = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const back = Math.round((day(new Date(now)) - day(d)) / 86_400_000);
  if (back === 0) return L(`今天 ${hm}`, `Today ${hm}`);
  if (back === 1) return L(`昨天 ${hm}`, `Yesterday ${hm}`);
  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return L(`${sameYear ? "" : `${d.getFullYear()}年`}${d.getMonth() + 1}月${d.getDate()}日 ${hm}`,
    `${d.toLocaleDateString("en", { month: "short", day: "numeric", ...(sameYear ? {} : { year: "numeric" }) })} ${hm}`);
}

type RunFilter = "all" | "sent" | "skipped" | "failed";

/** One automation: what it is set to, then every run (newest first). */
function AutomationDetail({ hubId, a, row, now, stamp, busy, error, onEdit, onRun, onToggle, onDelete, onOpenMessage, onOpenRun }: {
  hubId: string; a: Automation; row?: AgentRow; now: number; stamp: number; busy: boolean; error: unknown;
  onBack: () => void; onEdit: () => void; onRun: () => void; onToggle: () => void; onDelete: () => void;
  onOpenMessage?: (agent: string, messageId: string) => void;
  onOpenRun?: (workflow: string, run: string) => void;
}) {
  const [runs, setRuns] = useState<AutomationRun[] | null>(null);
  const [filter, setFilter] = useState<RunFilter>("all");
  const [open, setOpen] = useState<string | null>(null);
  const more = useContextMenu();
  useEffect(() => {
    let off = false;
    const read = () => appApi.automation(hubId, a.id).then((d) => { if (!off) setRuns(d.runs || []); }, () => { if (!off) setRuns((r) => r ?? []); });
    read();
    const t = setInterval(read, 20_000);
    return () => { off = true; clearInterval(t); };
  }, [hubId, a.id, stamp]);
  const when = [a.schedule ? describe(a.schedule.cron) : "", a.webhook ? L("Webhook 调用时", "On webhook") : ""].filter(Boolean).join(L("，或", ", or "));
  const counts = { sent: 0, skipped: 0, failed: 0 };
  for (const r of runs || []) counts[r.outcome] += 1;
  const shown = (runs || []).filter((r) => filter === "all" || r.outcome === filter);
  const filters: { key: RunFilter; label: string; n: number }[] = [
    { key: "all", label: L("全部", "All"), n: runs?.length || 0 },
    { key: "sent", label: L("已发送", "Sent"), n: counts.sent },
    { key: "skipped", label: L("跳过", "Skipped"), n: counts.skipped },
    { key: "failed", label: L("没发出去", "Not sent"), n: counts.failed },
  ];
  return (
    <>
      <header className={cn("pcd-head", !a.enabled && "is-away")}>
        <div className="pcd-titles">
          <h1><span>{a.name}</span></h1>
          <p className="pcd-sub">
            <span className={cn("pc-online", a.enabled && "is-on")}>{a.enabled ? L("在运行", "Running") : L("已暂停", "Paused")}</span>
            <span>{when}</span>
            {a.enabled && a.next_run_at && <span>{L(`下次 ${soonLabel(new Date(a.next_run_at), new Date(now))}`, `Next ${soonLabel(new Date(a.next_run_at), new Date(now))}`)}</span>}
          </p>
        </div>
        {/* As a workflow's: 编辑, 立即运行 and a square ⋯ (pause, delete). */}
        <div className="ph-actions wf-actions">
          <button type="button" className="app-btn" onClick={onEdit}><Pencil />{L("编辑", "Edit")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={onRun} disabled={busy} title={L("现在就发一次", "Send it once now")}><Play />{L("立即运行", "Run now")}</button>
          <button type="button" className="app-btn wf-more" aria-label={L("更多", "More")} title={L("更多", "More")} data-testid="automation-more" disabled={busy}
            onClick={(e) => more.openUnder(e.currentTarget, [
              a.enabled ? { label: L("暂停", "Pause"), icon: Pause, onSelect: onToggle } : { label: L("启用", "Resume"), icon: Play, onSelect: onToggle },
              "sep",
              { label: L("删除", "Delete"), icon: Trash2, danger: true, onSelect: onDelete },
            ])}><MoreHorizontal /></button>
        </div>
      </header>
      <ErrorText error={error} />

      <SectionHead title={L("设置", "Settings")} />
      <div className="aud-card" data-testid="automation-settings">
        {a.workflow ? (
          <div className="app-kv"><span className="app-kv-label">{L("启动工作流", "Starts")}</span>
            <span className="app-kv-value aud-to"><WorkflowIcon className="au-row-wf" /><b>{a.workflow_name || a.workflow}</b></span></div>
        ) : (
          <div className="app-kv"><span className="app-kv-label">{L("发给", "To")}</span>
            <span className="app-kv-value aud-to">{row && <Avatar agent={row.st.agent} size={18} badge={false} />}<b>{row ? nodeName(row.st.agent, row.hub) : a.to}</b></span></div>
        )}
        <div className="app-kv"><span className="app-kv-label">{L("什么时候", "When")}</span>
          <span className="app-kv-value">{when}{a.schedule && <span className="app-kv-sub">{L(`按 ${a.schedule.tz || "UTC"} 时间`, `${a.schedule.tz || "UTC"} time`)}</span>}</span></div>
        {a.webhook && a.webhook_url && (
          <div className="app-kv"><span className="app-kv-label">{L("Webhook 地址", "Webhook address")}</span>
            <span className="app-kv-value mono aud-url"><span>{a.webhook_url}</span><CopyButton value={a.webhook_url} /></span></div>
        )}
        <div className="app-kv"><span className="app-kv-label">{a.workflow ? L("上一次还没结束", "Last one still going") : L("上一次没做完", "Last one unfinished")}</span>
          <span className="app-kv-value">{a.skip_if_busy ? L("跳过这次", "Skip this run") : L("照样发", "Send anyway")}</span></div>
        {a.workflow ? Object.entries(a.inputs || {}).map(([k, v]) => (
          <div key={k} className="app-kv"><span className="app-kv-label">{L(`输入 ${k}`, `Input ${k}`)}</span><span className="app-kv-value mono">{v}</span></div>
        )) : (
          <div className="app-kv aud-what"><span className="app-kv-label">{L("让它做什么", "What it asks")}</span>
            <div className="aud-body">{a.body}</div></div>
        )}
      </div>

      <SectionHead title={L("运行记录", "Runs")} count={runs?.length} />
      {runs === null ? <RowsSkeleton rows={3} className="pc-list" /> : runs.length === 0 ? (
        <div className="pcd-empty" data-testid="automation-no-runs">
          <span>{L("还没运行过。到点、被调用或者点「立即运行」，每一次都会记在这里：发没发出去、为什么，以及它开始的那段对话。", "It has not run yet. Each run, on time, by a call or by Run now, is listed here: whether it went out, why not, and the chat it started.")}</span>
          <button type="button" className="app-btn app-btn-small" onClick={onRun} disabled={busy}><Play />{L("立即运行", "Run now")}</button>
        </div>
      ) : (
        <>
          {(counts.skipped > 0 || counts.failed > 0) && (
            <div className="wf-chips aud-filters" role="tablist" aria-label={L("只看", "Show")}>
              {filters.filter((f) => f.key === "all" || f.n > 0).map((f) => (
                <button key={f.key} type="button" role="tab" aria-selected={filter === f.key} className={cn("wf-chip", filter === f.key && "is-on")} onClick={() => setFilter(f.key)}>
                  {f.label}<span className="aud-filter-n">{f.n}</span>
                </button>
              ))}
            </div>
          )}
          <div className="pc-list aud-runs" data-testid="automation-runs">
            {shown.map((r) => (
              <RunRow key={r.id} hubId={hubId} r={r} now={now} open={open === r.id} onToggle={() => setOpen((o) => (o === r.id ? null : r.id))}
                onChat={r.message_id && onOpenMessage ? () => onOpenMessage(a.to, r.message_id!) : undefined}
                onRun={r.workflow_run && a.workflow && onOpenRun ? () => onOpenRun(a.workflow!, r.workflow_run!) : undefined} />
            ))}
          </div>
          {runs.length >= 200 && <p className="app-small-note aud-kept">{L("只保留最近 200 次。", "Only the last 200 runs are kept.")}</p>}
        </>
      )}

      {more.element}
    </>
  );
}

/** One run: how it went and when; its chat; a webhook call's content. */
function RunRow({ hubId, r, now, open, onToggle, onChat, onRun }: {
  hubId: string; r: AutomationRun; now: number; open: boolean; onToggle: () => void; onChat?: () => void; onRun?: () => void;
}) {
  const hook = r.trigger === "webhook";
  return (
    <div className={cn("aud-run", open && "is-open")} data-testid="automation-run">
      <div className="aud-run-line">
        <span className={cn("au-last aud-run-what", outcomeTone(r))}><i aria-hidden /><span>{outcomeWords(r)}</span></span>
        <span className="aud-run-trigger">{triggerWords(r.trigger)}</span>
        <span className="aud-run-when" title={new Date(r.at).toLocaleString()}>{whenAt(r.at, now)}<small>{timeAgo(r.at, now)}</small></span>
        <span className="aud-run-actions">
          {hook && (
            <button type="button" className="app-btn app-btn-quiet app-btn-small" aria-expanded={open} onClick={onToggle}>
              {L("调用内容", "The call")}<ChevronDown className={cn("aud-chev", open && "is-open")} />
            </button>
          )}
          {onChat && <button type="button" className="app-btn app-btn-quiet app-btn-small" onClick={onChat}><MessageSquare />{L("看对话", "Chat")}</button>}
          {onRun && <button type="button" className="app-btn app-btn-quiet app-btn-small" onClick={onRun}><WorkflowIcon />{L("看运行", "The run")}</button>}
        </span>
      </div>
      {hook && open && <CallContent hubId={hubId} r={r} />}
    </div>
  );
}

/** What a webhook call carried: read when opened (lists leave bodies out). */
function CallContent({ hubId, r }: { hubId: string; r: AutomationRun }) {
  const [req, setReq] = useState<WebhookRequest | null | "old" | "error">(null);
  useEffect(() => {
    let off = false;
    appApi.automationRun(hubId, r.automation_id, r.id).then((x) => { if (!off) setReq(x.request || "old"); },
      (e) => { if (!off) setReq(e instanceof ApiError && e.status === 404 ? "old" : "error"); });
    return () => { off = true; };
  }, [hubId, r.automation_id, r.id]);
  if (req === null) return <div className="aud-call"><span className="skel skel-line" style={{ width: "50%" }} /></div>;
  if (req === "old") return <div className="aud-call aud-call-none">{L("这次调用没有记下内容：那时工作区的 Hub 版本还太旧（v0.20.18 起才记）。", "Nothing was kept of this call: the workspace's hub was too old then (kept from v0.20.18).")}</div>;
  if (req === "error") return <div className="aud-call aud-call-none">{L("读不到这次调用的内容，稍后再试。", "Could not read this call; try again later.")}</div>;
  let body = req.body || "";
  let json = false;
  try { if (body.trim().startsWith("{") || body.trim().startsWith("[")) { body = JSON.stringify(JSON.parse(body), null, 2); json = true; } } catch { /* as it came */ }
  const headers = Object.entries(req.headers || {});
  return (
    <div className="aud-call" data-testid="automation-call">
      <div className="aud-call-meta">
        {req.content_type && <span className="pcd-chip mono">{req.content_type}</span>}
        <span className="pcd-chip">{bytes(req.size)}</span>
        {json && <span className="pcd-chip">JSON</span>}
      </div>
      {headers.length > 0 && (
        <div className="aud-call-headers">
          {headers.map(([k, v]) => <div key={k} className="app-kv"><span className="app-kv-label mono">{k}</span><span className="app-kv-value mono">{v}</span></div>)}
        </div>
      )}
      {body ? (
        <div className="aud-call-body">
          <CopyButton value={req.body || ""} />
          <pre>{body}</pre>
        </div>
      ) : <div className="aud-call-none">{L("这次调用没有带内容。", "This call carried no body.")}</div>}
      <p className="app-small-note">{L("只记下说明来源的请求头，不记密钥、签名、Cookie 和对方的地址。", "Only headers that say where it came from are kept: never the secret, signatures, cookies or the caller's address.")}</p>
    </div>
  );
}


type Mode = "daily" | "weekdays" | "weekly" | "hours" | "cron";
const modes = (): { mode: Mode; label: string }[] => [
  { mode: "daily", label: L("每天", "Daily") },
  { mode: "weekdays", label: L("工作日", "Weekdays") },
  { mode: "weekly", label: L("每周", "Weekly") },
  { mode: "hours", label: L("每隔几小时", "Hourly") },
  { mode: "cron", label: L("自定义", "Custom") },
];
const pad = (n: number) => String(n).padStart(2, "0");

function AutomationEditor({ hubId, rows, editing, now, onClose, onChanged }: {
  hubId: string; rows: AgentRow[]; editing: Editing | null; now: number; onClose: () => void; onChanged: () => void;
}) {
  const a = editing && !("start" in editing) ? editing : null;
  const agents = useMemo(() => rows.filter((r) => r.hub.id === hubId && agentKind(r.st.agent) === "agent")
    .sort((x, y) => nodeName(x.st.agent, x.hub).localeCompare(nodeName(y.st.agent, y.hub))), [rows, hubId]);
  const [name, setName] = useState("");
  const [to, setTo] = useState("");
  const [body, setBody] = useState("");
  const [timed, setTimed] = useState(true);
  const [mode, setMode] = useState<Mode>("weekdays");
  const [hour, setHour] = useState(9);
  const [minute, setMinute] = useState(0);
  const [day, setDay] = useState(1);
  const [hours, setHours] = useState(2);
  const [cron, setCron] = useState("0 9 * * 1-5");
  const [tz, setTz] = useState(localZone());
  const [hook, setHook] = useState(false);
  const [skip, setSkip] = useState(true);
  // What it does: send a task, or start a workflow (with its inputs).
  const [target, setTarget] = useState<"task" | "workflow">("task");
  const [wfId, setWfId] = useState("");
  const [wfInputs, setWfInputs] = useState<Record<string, string>>({});
  const [wfs, setWfs] = useState<Workflow[] | null>(null);
  const [wfDef, setWfDef] = useState<WorkflowDef | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [secret, setSecret] = useState<{ url: string; secret: string } | null>(null);
  const [deleting, setDeleting] = useState(false);
  const key = !editing ? "" : "start" in editing ? `new:${editing.start.name}` : editing.id;
  const setEvery = (e: Every) => {
    setMode(e.kind);
    if (e.kind === "cron") { setCron(e.cron); return; }
    setMinute(e.minute);
    if (e.kind === "hours") setHours(e.every); else setHour(e.hour);
    if (e.kind === "weekly") setDay(e.day);
    setCron(toCron(e));
  };
  useEffect(() => {
    if (!editing) return;
    setError(null); setSecret(null); setDeleting(false);
    if ("start" in editing) {
      const s = editing.start;
      setName(s.name); setTo(agents[0]?.st.agent.name || ""); setBody(s.body); setTimed(!!s.every); setHook(s.hook); setSkip(true);
      setTarget("task"); setWfId(""); setWfInputs({});
      setTz(localZone()); setHour(9); setMinute(0); setDay(1); setHours(2);
      setEvery(s.every || blank.every!);
      return;
    }
    setName(editing.name); setTo(editing.to || agents[0]?.st.agent.name || ""); setBody(editing.body); setHook(editing.webhook); setSkip(editing.skip_if_busy);
    setTarget(editing.workflow ? "workflow" : "task"); setWfId(editing.workflow || ""); setWfInputs(editing.inputs || {});
    setTimed(!!editing.schedule); setTz(editing.schedule?.tz || localZone());
    setEvery(fromCron(editing.schedule?.cron || "0 9 * * 1-5"));
    // Only when another one is opened (a refresh must not undo edits).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  // The workflows it can start, and the inputs of the one picked.
  useEffect(() => {
    if (!editing) return;
    let off = false;
    appApi.workflows(hubId).then((r) => { if (!off) setWfs(r.workflows || []); }, () => { if (!off) setWfs([]); });
    return () => { off = true; };
  }, [hubId, key]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setWfDef(null);
    if (!wfId) return;
    let off = false;
    appApi.workflow(hubId, wfId).then((w) => { if (!off) setWfDef(w.definition || null); }, () => undefined);
    return () => { off = true; };
  }, [hubId, wfId]);
  if (!editing) return null;

  const every: Every = mode === "daily" ? { kind: "daily", hour, minute }
    : mode === "weekdays" ? { kind: "weekdays", hour, minute }
      : mode === "weekly" ? { kind: "weekly", day, hour, minute }
        : mode === "hours" ? { kind: "hours", every: hours, minute }
          : { kind: "cron", cron };
  const cronOk = cron.trim().split(/\s+/).length === 5;
  const wfProps = Object.entries(wfDef?.inputs?.properties || {});
  const wfMissing = target === "workflow" && !hook ? (wfDef?.inputs?.required || []).find((k) => !(wfInputs[k] || "").trim()) : undefined;
  const missing = !name.trim() ? L("还没写名字", "It needs a name")
    : target === "workflow" ? (!wfId ? L("选一个工作流", "Pick a workflow")
      : wfMissing ? L(`还没填「${wfDef?.inputs?.properties?.[wfMissing]?.title || wfMissing}」`, `Fill in "${wfMissing}"`) : !timed && !hook ? L("选一种触发方式：定时或 Webhook", "Pick when: on a schedule, or by webhook") : "")
    : !to ? L("还没选发给谁", "Pick whom it goes to")
      : !body.trim() ? L("还没写让它做什么", "Say what to ask")
        : !timed && !hook ? L("选一种触发方式：定时或 Webhook", "Pick when: on a schedule, or by webhook")
          : timed && mode === "cron" && !cronOk ? L("cron 要写 5 段", "A cron has 5 fields") : "";
  const next = timed && tz === localZone() ? nextRun(every, new Date(now)) : null;
  const toRow = agents.find((r) => r.st.agent.name === to);

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const what = target === "workflow" ? { workflow: wfId, inputs: wfInputs } : { to, body, ...(a?.workflow ? { workflow: "" } : {}) };
      const input = {
        name: name.trim(), ...what, skip_if_busy: skip, webhook: hook,
        ...(timed ? { schedule: { cron: toCron(every), tz } } : a?.schedule ? { clear_schedule: true } : {}),
      };
      const r = a ? await appApi.updateAutomation(hubId, a.id, input) : await appApi.createAutomation(hubId, { ...input, enabled: true });
      onChanged();
      if (r.webhook_secret) setSecret({ url: r.automation.webhook_url || "", secret: r.webhook_secret });
      else onClose();
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const rotate = async () => {
    if (!a) return;
    setBusy(true);
    setError(null);
    try {
      const r = await appApi.updateAutomation(hubId, a.id, { rotate_secret: true });
      if (r.webhook_secret) setSecret({ url: r.automation.webhook_url || "", secret: r.webhook_secret });
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const remove = async () => {
    if (!a) return;
    setBusy(true);
    try { await appApi.deleteAutomation(hubId, a.id); onChanged(); onClose(); } catch (e) { setError(e); } finally { setBusy(false); }
  };

  if (secret) {
    const curl = `curl -X POST '${secret.url}' -H 'Authorization: Bearer ${secret.secret}' --data '构建失败：…'`;
    return (
      <Modal open onOpenChange={(o) => !o && onClose()} title={L("Webhook 地址和密钥", "Webhook address and secret")} className="au-edit"
        description={L("密钥只显示这一次，请现在复制保存。外部系统带着它 POST 这个地址，就会触发一次；请求内容会替换消息里的 {{payload}}。", "The secret is shown only now: copy it. An outside system POSTs to the address with it to run this once; the request's content takes the place of {{payload}} in the message.")}>
        <div data-testid="automation-secret">
          <div className="app-field"><span className="app-field-label">{L("地址", "Address")}</span>
            <div className="app-command"><code>{secret.url}</code><CopyButton value={secret.url} /></div></div>
          <div className="app-field"><span className="app-field-label">{L("密钥", "Secret")}</span>
            <div className="app-command"><code>{secret.secret}</code><CopyButton value={secret.secret} /></div></div>
          <div className="app-field"><span className="app-field-label">{L("试一下", "Try it")}</span>
            <div className="app-command"><code>{curl}</code><CopyButton value={curl} /></div></div>
          <div className="app-modal-actions"><button type="button" className="app-btn app-btn-primary" onClick={onClose}>{L("好了", "Done")}</button></div>
        </div>
      </Modal>
    );
  }

  const minutes = [...new Set([0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, minute])].sort((x, y) => x - y);
  return (
    <>
      <Modal open={!deleting} onOpenChange={(o) => !o && !busy && onClose()} title={a ? L("编辑自动化", "Edit automation") : L("新建自动化", "New automation")}
        className="au-edit">
        <form data-testid="automation-editor" onSubmit={(e) => { e.preventDefault(); if (!missing) void save(); }}>
          <label className="app-field"><span className="app-field-label">{L("名字", "Name")}</span>
            <input className="app-input" value={name} onChange={(e) => setName(e.target.value)} maxLength={80} placeholder={L("比如：每天早上汇总 issue", "e.g. Morning issue summary")} disabled={busy} /></label>
          <div className="app-field">
            <span className="app-field-label">{L("做什么", "What it does")}</span>
            <div className="seg seg-sm seg-fill" role="radiogroup" aria-label={L("做什么", "What it does")}>
              <button type="button" role="radio" aria-checked={target === "task"} className={cn("seg-btn", target === "task" && "active")}
                onClick={() => setTarget("task")} disabled={busy}>{L("给一个 agent 发任务", "Send an agent a task")}</button>
              <button type="button" role="radio" aria-checked={target === "workflow"} className={cn("seg-btn", target === "workflow" && "active")}
                onClick={() => setTarget("workflow")} disabled={busy}>{L("启动一个工作流", "Start a workflow")}</button>
            </div>
          </div>
          {target === "workflow" ? (
            <>
              <label className="app-field"><span className="app-field-label">{L("哪个工作流", "Which workflow")}</span>
                <select className="app-select-field" value={wfId} onChange={(e) => { setWfId(e.target.value); setWfInputs({}); }} disabled={busy || !wfs?.length} aria-label={L("哪个工作流", "Which workflow")}>
                  <option value="">{wfs === null ? L("正在读…", "Loading…") : wfs.length ? L("选一个", "Pick one") : L("还没有工作流", "No workflows yet")}</option>
                  {(wfs || []).map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
                </select></label>
              {wfProps.map(([k, p]) => (
                <label key={k} className="app-field"><span className="app-field-label">{p.title || k}</span>
                  <input className="app-input" value={wfInputs[k] || ""} disabled={busy} placeholder={p.default !== undefined ? String(p.default) : hook ? `{{payload.${k}}}` : ""}
                    onChange={(e) => setWfInputs((x) => ({ ...x, [k]: e.target.value }))} /></label>
              ))}
              {hook && wfId && (
                <span className="app-small-note au-payload">
                  {L("Webhook 调用时带来的内容：整段用 {{payload}}，JSON 里的字段用 {{payload.字段}}，比如 {{payload.issue.title}}。", "What a webhook call sends: all of it is {{payload}}; a field of its JSON is {{payload.field}}, e.g. {{payload.issue.title}}.")}
                </span>
              )}
            </>
          ) : (
            <>
              <label className="app-field"><span className="app-field-label">{L("发给谁", "To whom")}</span>
                <span className="au-to">
                  {toRow && <Avatar agent={toRow.st.agent} size={20} badge={false} />}
                  <select className="app-select-field" value={to} onChange={(e) => setTo(e.target.value)} disabled={busy} aria-label={L("发给谁", "To whom")}>
                    {agents.map((r) => <option key={r.st.agent.name} value={r.st.agent.name}>{nodeName(r.st.agent, r.hub)}</option>)}
                  </select>
                </span></label>
              <label className="app-field"><span className="app-field-label">{L("让它做什么", "What to ask")}</span>
                <textarea className="app-input au-body" rows={3} value={body} onChange={(e) => setBody(e.target.value)} disabled={busy}
                  placeholder={L("比如：看一下昨天新开的 issue，挑出要紧的，在这里回我一份清单。", "e.g. Look at yesterday's new issues, pick the urgent ones, and reply with a list.")} />
                {hook && (
                  <span className="app-small-note au-payload">
                    {L("外部系统调用时带来的内容，会放在 ", "What a call sends goes where ")}
                    <button type="button" className="au-token" disabled={busy || body.includes("{{payload}}")}
                      onClick={() => setBody((b) => (b.trimEnd() ? `${b.trimEnd()}\n{{payload}}` : "{{payload}}"))}
                      title={L("插入到最后", "Insert at the end")}>{"{{payload}}"}</button>
                    {L(" 的位置；没写就接在最后。", " is; without it, at the end.")}
                  </span>
                )}
              </label>
            </>
          )}

          <div className="app-field">
            <span className="app-field-label">{L("什么时候", "When")}</span>
            <div className="au-kinds">
              <button type="button" className={cn("au-kind", timed && "is-on")} aria-pressed={timed} onClick={() => setTimed((t) => !t)} disabled={busy}>
                <span className="au-icon"><Clock /></span>
                <span><b>{L("定时", "On a schedule")}</b><small>{L("到点就发", "Sent when it is time")}</small></span>
              </button>
              <button type="button" className={cn("au-kind", hook && "is-on")} aria-pressed={hook} onClick={() => setHook((h) => !h)} disabled={busy}>
                <span className="au-icon is-hook"><Webhook /></span>
                <span><b>Webhook</b><small>{L("CI、监控等调用时", "When CI, monitoring… call")}</small></span>
              </button>
            </div>
            {timed && (
              <div className="au-sched">
                <div className="seg seg-sm seg-fill" role="radiogroup" aria-label={L("多久一次", "How often")}>
                  {modes().map((m) => (
                    <button key={m.mode} type="button" role="radio" aria-checked={mode === m.mode} className={cn("seg-btn", mode === m.mode && "active")}
                      onClick={() => setMode(m.mode)} disabled={busy}>{m.label}</button>
                  ))}
                </div>
                {mode === "weekly" && (
                  <div className="au-chips" role="radiogroup" aria-label={L("星期几", "Which day")}>
                    {weekChips().map((d) => (
                      <button key={d.day} type="button" role="radio" aria-checked={day === d.day} className={cn("au-chip", day === d.day && "is-on")}
                        onClick={() => setDay(d.day)} disabled={busy}>{d.label}</button>
                    ))}
                  </div>
                )}
                {mode === "hours" && (
                  <div className="au-chips" role="radiogroup" aria-label={L("隔几小时", "How many hours")}>
                    {[1, 2, 3, 4, 6, 8, 12].map((n) => (
                      <button key={n} type="button" role="radio" aria-checked={hours === n} className={cn("au-chip", hours === n && "is-on")}
                        onClick={() => setHours(n)} disabled={busy}>{L(`${n} 小时`, `${n} h`)}</button>
                    ))}
                  </div>
                )}
                {mode === "cron" ? (
                  <input className="app-input mono au-cron" value={cron} onChange={(e) => setCron(e.target.value)} placeholder="0 9 * * 1-5" disabled={busy} aria-label="cron" />
                ) : (
                  <div className="au-time">
                    <span>{mode === "hours" ? L("在每个整点后第", "At minute") : L("几点", "At")}</span>
                    {mode !== "hours" && (
                      <select className="app-select-field" value={hour} onChange={(e) => setHour(+e.target.value)} disabled={busy} aria-label={L("几点", "Hour")}>
                        {Array.from({ length: 24 }, (_, h) => <option key={h} value={h}>{pad(h)}</option>)}
                      </select>
                    )}
                    {mode !== "hours" && <span>:</span>}
                    <select className="app-select-field" value={minute} onChange={(e) => setMinute(+e.target.value)} disabled={busy} aria-label={L("几分", "Minute")}>
                      {minutes.map((m) => <option key={m} value={m}>{pad(m)}</option>)}
                    </select>
                    {mode === "hours" && <span>{L("分", "")}</span>}
                  </div>
                )}
                <div className="au-summary">
                  {describe(toCron(every))}
                  <span>{L(`${tz} 时间`, `${tz} time`)}</span>
                  {next && <span>{L(`下次 ${soonLabel(next, new Date(now))}`, `next ${soonLabel(next, new Date(now))}`)}</span>}
                </div>
              </div>
            )}
            {hook && !timed && <div className="au-summary au-hook-note">{L("保存后给出调用地址和密钥。", "The address and secret come after saving.")}</div>}
          </div>

          <label className="app-check-line au-skip">
            <input type="checkbox" checked={skip} onChange={(e) => setSkip(e.target.checked)} disabled={busy} />
            <span>{target === "workflow" ? L("上一次的运行还没结束，就跳过这次", "Skip a run while the last one is still going") : L("上一次的任务还没做完，就跳过这次", "Skip a run while the last task is not finished")}<small>{L("免得 agent 那边越堆越多。", "So that work does not pile up at the agent.")}</small></span>
          </label>

          <div className="au-note">{L("自动化的内容存在工作区的 Hub 上，不是端到端加密的。", "An automation's text is kept on the workspace's hub, not end-to-end encrypted.")}</div>
          <ErrorText error={error} />
          <div className="app-modal-actions au-actions">
            {a && <button type="button" className="app-btn app-btn-quiet au-delete" onClick={() => setDeleting(true)} disabled={busy}>{L("删除", "Delete")}</button>}
            {a?.webhook && <button type="button" className="app-btn app-btn-quiet" onClick={() => void rotate()} disabled={busy}>{L("重新生成密钥", "New secret")}</button>}
            {missing && <span className="app-modal-hint">{missing}</span>}
            <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={busy || !!missing}>{a ? L("保存", "Save") : L("创建", "Create")}</button>
          </div>
        </form>
      </Modal>
      <ConfirmDialog open={deleting} onOpenChange={(o) => !o && setDeleting(false)} title={L("删除这个自动化？", "Delete this automation?")}
        description={L("它不会再发消息了，Webhook 地址也失效。已经发出去的消息和 agent 的回复都保留。", "It sends nothing more, and its webhook address stops working. What it sent and the answers stay.")}
        confirmLabel={L("删除", "Delete")} onConfirm={() => void remove()} busy={busy} />
    </>
  );
}
