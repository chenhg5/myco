import { useCallback, useEffect, useMemo, useState } from "react";
import { Play, Plus } from "lucide-react";
import type { Automation, AutomationRun } from "@/api";
import { ApiError, appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, nodeName } from "@/app/nodes";
import { CopyButton } from "@/app/copy";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { dayNames, describe, fromCron, localZone, toCron, type Every } from "@/app/schedule";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/**
 * Automations: a task sent for you to one agent, on a schedule or when an
 * outside system calls its webhook. They run on the workspace's hub, so
 * they go on while your computers are off; what an agent answers is in
 * your chat with it, the task marked 自动.
 */

const outcomeWords = (r: AutomationRun) =>
  r.outcome === "sent" ? L("已发送", "Sent")
    : r.outcome === "skipped" ? L(`跳过了${r.error ? `：${skipWhy(r.error)}` : ""}`, `Skipped${r.error ? `: ${r.error}` : ""}`)
      : L(`没发出去${r.error ? `：${r.error}` : ""}`, `Not sent${r.error ? `: ${r.error}` : ""}`);
const skipWhy = (e: string) => (/not finished/.test(e) ? "上一次的任务还没做完" : e);
const triggerWords = (t: AutomationRun["trigger"]) => (t === "schedule" ? L("到点", "On schedule") : t === "webhook" ? L("Webhook", "Webhook") : L("手动", "By hand"));

export function AutomationsPage({ hubId, rows, now }: { hubId: string; rows: AgentRow[]; now: number }) {
  const [list, setList] = useState<Automation[] | null>(null);
  const [unsupported, setUnsupported] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [editing, setEditing] = useState<Automation | "new" | null>(null);
  const [busy, setBusy] = useState("");
  const load = useCallback(() => {
    appApi.automations(hubId).then((r) => { setList(r.automations || []); setUnsupported(false); }, (e) => {
      if (e instanceof ApiError && e.status === 404) setUnsupported(true);
      else setError(e);
      setList([]);
    });
  }, [hubId]);
  useEffect(() => { setList(null); load(); const t = setInterval(load, 20_000); return () => clearInterval(t); }, [load]);
  const agentName = (n: string) => { const r = rows.find((x) => x.hub.id === hubId && x.st.agent.name === n); return r ? nodeName(r.st.agent, r.hub) : n; };

  const toggle = async (a: Automation) => {
    setBusy(a.id);
    setError(null);
    try { await appApi.updateAutomation(hubId, a.id, { enabled: !a.enabled }); load(); } catch (e) { setError(e); } finally { setBusy(""); }
  };
  const runNow = async (a: Automation) => {
    setBusy(a.id);
    setError(null);
    try { await appApi.runAutomation(hubId, a.id); load(); } catch (e) { setError(e); } finally { setBusy(""); }
  };

  return (
    <div className="pc au">
      <div className="pc-head">
        <p className="pc-intro">{L("到点或者被外部系统调用时，替你给某个 agent 发一条消息。跑在工作区的 Hub 上，电脑关着也照常发。", "On a schedule, or when an outside system calls it, a message goes to an agent for you. It runs on the workspace's hub, so it goes on while your computers are off.")}</p>
        {!unsupported && <button type="button" className="app-btn app-btn-small" onClick={() => setEditing("new")}><Plus />{L("新建自动化", "New automation")}</button>}
      </div>
      <ErrorText error={error} />
      {unsupported ? (
        <div className="app-empty-note" data-testid="automations-unsupported">{L("这个工作区的 Hub 还不支持自动化：在服务器上运行 sudo myco upgrade 升级到 v0.19.22 或更新。", "This workspace's hub has no automations yet: upgrade it to v0.19.22 or later (sudo myco upgrade on the server).")}</div>
      ) : list === null ? null : list.length === 0 ? (
        <div className="app-empty-note">{L("还没有自动化。比如：每个工作日早上 9 点让 agent 汇总昨天的 issue；CI 失败时让它看看日志。", "No automations yet. For instance: every weekday at 9, have an agent sum up yesterday's issues; when CI fails, have it look at the log.")}</div>
      ) : (
        <div className="pc-list" data-testid="automations">
          {list.map((a) => {
            const when = [a.schedule ? describe(a.schedule.cron) : "", a.webhook ? "Webhook" : ""].filter(Boolean).join(" · ");
            return (
              <div key={a.id} className={cn("pc-row au-row", !a.enabled && "is-away")}>
                <button type="button" className="pc-row-main" onClick={() => setEditing(a)}>
                  <span className="pc-row-name">{a.name}</span>
                  <span className="pc-row-sub">
                    <span>{L(`发给 ${agentName(a.to)}`, `To ${agentName(a.to)}`)}</span>
                    {when && <span>{when}</span>}
                    {!a.enabled ? <span>{L("已暂停", "Paused")}</span>
                      : a.next_run_at && <span>{L(`下次 ${new Date(a.next_run_at).toLocaleString()}`, `Next ${new Date(a.next_run_at).toLocaleString()}`)}</span>}
                    {a.last_run && <span className={cn(a.last_run.outcome === "failed" && "au-failed")}>{L(`上次 ${timeAgo(a.last_run.at, now)}：`, `Last ${timeAgo(a.last_run.at, now)}: `)}{outcomeWords(a.last_run)}</span>}
                  </span>
                </button>
                <button type="button" className="app-btn app-btn-small" disabled={busy === a.id} onClick={() => void runNow(a)} title={L("现在就发一次", "Send it once now")}><Play />{L("立即运行", "Run now")}</button>
                <button type="button" className="app-btn app-btn-small" disabled={busy === a.id} onClick={() => void toggle(a)}>{a.enabled ? L("暂停", "Pause") : L("启用", "Resume")}</button>
              </div>
            );
          })}
        </div>
      )}
      <AutomationEditor hubId={hubId} rows={rows} editing={editing} now={now} onClose={() => setEditing(null)} onChanged={load} />
    </div>
  );
}

type Mode = "daily" | "weekdays" | "weekly" | "hours" | "cron";

function AutomationEditor({ hubId, rows, editing, now, onClose, onChanged }: {
  hubId: string; rows: AgentRow[]; editing: Automation | "new" | null; now: number; onClose: () => void; onChanged: () => void;
}) {
  const a = editing && editing !== "new" ? editing : null;
  const agents = useMemo(() => rows.filter((r) => r.hub.id === hubId && agentKind(r.st.agent) === "agent")
    .sort((x, y) => nodeName(x.st.agent, x.hub).localeCompare(nodeName(y.st.agent, y.hub))), [rows, hubId]);
  const [name, setName] = useState("");
  const [to, setTo] = useState("");
  const [body, setBody] = useState("");
  const [timed, setTimed] = useState(true);
  const [mode, setMode] = useState<Mode>("weekdays");
  const [time, setTime] = useState("09:00");
  const [day, setDay] = useState(1);
  const [hours, setHours] = useState(2);
  const [cron, setCron] = useState("0 9 * * 1-5");
  const [tz, setTz] = useState(localZone());
  const [hook, setHook] = useState(false);
  const [skip, setSkip] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [secret, setSecret] = useState<{ url: string; secret: string } | null>(null);
  const [runs, setRuns] = useState<AutomationRun[]>([]);
  const [deleting, setDeleting] = useState(false);
  const key = editing === "new" ? "new" : a?.id || "";
  useEffect(() => {
    if (!editing) return;
    setError(null); setSecret(null); setRuns([]); setDeleting(false);
    if (editing === "new") {
      setName(""); setTo(agents[0]?.st.agent.name || ""); setBody(""); setTimed(true); setMode("weekdays"); setTime("09:00");
      setDay(1); setHours(2); setCron("0 9 * * 1-5"); setTz(localZone()); setHook(false); setSkip(true);
      return;
    }
    setName(editing.name); setTo(editing.to); setBody(editing.body); setHook(editing.webhook); setSkip(editing.skip_if_busy);
    setTimed(!!editing.schedule); setTz(editing.schedule?.tz || localZone());
    const e = fromCron(editing.schedule?.cron || "0 9 * * 1-5");
    setCron(editing.schedule?.cron || "0 9 * * 1-5");
    setMode(e.kind);
    if (e.kind !== "cron" && e.kind !== "hours") setTime(`${String(e.hour).padStart(2, "0")}:${String(e.minute).padStart(2, "0")}`);
    if (e.kind === "weekly") setDay(e.day);
    if (e.kind === "hours") setHours(e.every);
    appApi.automation(hubId, editing.id).then((d) => setRuns(d.runs || []), () => undefined);
    // Only when another one is opened (a refresh must not undo edits).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  if (!editing) return null;

  const [hh, mm] = time.split(":").map((x) => +x || 0);
  const every: Every = mode === "daily" ? { kind: "daily", hour: hh, minute: mm }
    : mode === "weekdays" ? { kind: "weekdays", hour: hh, minute: mm }
      : mode === "weekly" ? { kind: "weekly", day, hour: hh, minute: mm }
        : mode === "hours" ? { kind: "hours", every: hours, minute: mm }
          : { kind: "cron", cron };
  const ok = name.trim() && to && body.trim() && (timed || hook) && (!timed || mode !== "cron" || cron.trim().split(/\s+/).length === 5);

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const input = {
        name: name.trim(), to, body, skip_if_busy: skip, webhook: hook,
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
      <Modal open onOpenChange={(o) => !o && onClose()} title={L("Webhook 地址和密钥", "Webhook address and secret")}
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

  return (
    <>
      <Modal open={!deleting} onOpenChange={(o) => !o && !busy && onClose()} title={a ? a.name : L("新建自动化", "New automation")}
        description={L("到点或者被调用时，替你把下面这段话发给 agent，和你自己发的一样。", "When it is time, or when it is called, this goes to the agent for you, just as if you sent it.")} className="au-edit">
        <form data-testid="automation-editor" onSubmit={(e) => { e.preventDefault(); if (ok) void save(); }}>
          <label className="app-field"><span className="app-field-label">{L("名字", "Name")}</span>
            <input className="app-input" value={name} onChange={(e) => setName(e.target.value)} maxLength={80} placeholder={L("比如：每天早上汇总 issue", "e.g. Morning issue summary")} disabled={busy} /></label>
          <label className="app-field"><span className="app-field-label">{L("发给谁", "To whom")}</span>
            <select className="app-select-field" value={to} onChange={(e) => setTo(e.target.value)} disabled={busy} aria-label={L("发给谁", "To whom")}>
              {agents.map((r) => <option key={r.st.agent.name} value={r.st.agent.name}>{nodeName(r.st.agent, r.hub)}</option>)}
            </select></label>
          <label className="app-field"><span className="app-field-label">{L("让它做什么", "What to ask")}</span>
            <textarea className="app-input au-body" rows={4} value={body} onChange={(e) => setBody(e.target.value)} disabled={busy}
              placeholder={L("比如：看一下昨天新开的 issue，挑出要紧的，在这里回我一份清单。", "e.g. Look at yesterday's new issues, pick the urgent ones, and reply with a list.")} />
            {hook && <span className="app-small-note">{L("Webhook 调用时带的内容会放在 {{payload}} 的位置；没写 {{payload}} 就接在最后。", "What a webhook call sends goes where {{payload}} is; without it, at the end.")}</span>}
          </label>

          <div className="app-field">
            <span className="app-field-label">{L("什么时候", "When")}</span>
            <div className="au-triggers">
              <div className="au-trigger">
                <label className="app-check-line"><input type="checkbox" checked={timed} onChange={(e) => setTimed(e.target.checked)} disabled={busy} />
                  <span>{L("定时", "On a schedule")}<small>{L("到点就发，最短 10 分钟一次。", "Sent when it is time; at most every 10 minutes.")}</small></span></label>
                {timed && (
                  <div className="au-trigger-body">
                    <div className="au-when">
                      <select className="app-select-field" value={mode} onChange={(e) => setMode(e.target.value as Mode)} disabled={busy} aria-label={L("多久一次", "How often")}>
                        <option value="daily">{L("每天", "Every day")}</option>
                        <option value="weekdays">{L("工作日（周一到周五）", "Weekdays (Mon–Fri)")}</option>
                        <option value="weekly">{L("每周一次", "Once a week")}</option>
                        <option value="hours">{L("每隔几小时", "Every few hours")}</option>
                        <option value="cron">{L("自定义（cron）", "Custom (cron)")}</option>
                      </select>
                      {mode === "weekly" && (
                        <select className="app-select-field" value={day} onChange={(e) => setDay(+e.target.value)} disabled={busy} aria-label={L("星期几", "Which day")}>
                          {dayNames().map((d, i) => <option key={d} value={i}>{d}</option>)}
                        </select>
                      )}
                      {mode === "hours" && (
                        <select className="app-select-field" value={hours} onChange={(e) => setHours(+e.target.value)} disabled={busy} aria-label={L("隔几小时", "How many hours")}>
                          {[1, 2, 3, 4, 6, 8, 12].map((n) => <option key={n} value={n}>{L(`每 ${n} 小时`, `Every ${n} h`)}</option>)}
                        </select>
                      )}
                      {mode !== "cron" && mode !== "hours" && <input type="time" className="app-input au-time" value={time} onChange={(e) => setTime(e.target.value)} disabled={busy} aria-label={L("几点", "At what time")} />}
                      {mode === "cron" && <input className="app-input mono au-cron" value={cron} onChange={(e) => setCron(e.target.value)} placeholder="0 9 * * 1-5" disabled={busy} aria-label="cron" />}
                    </div>
                    <div className="au-summary">{L(`${describe(toCron(every))}（${tz} 时间）`, `${describe(toCron(every))} (${tz} time)`)}</div>
                  </div>
                )}
              </div>
              <div className="au-trigger">
                <label className="app-check-line"><input type="checkbox" checked={hook} onChange={(e) => setHook(e.target.checked)} disabled={busy} />
                  <span>{L("Webhook", "Webhook")}<small>{L("外部系统（CI、监控、GitHub…）POST 一个地址就触发。保存后给出地址和密钥。", "An outside system (CI, monitoring, GitHub…) POSTs to an address to run it. The address and secret come after saving.")}</small></span></label>
              </div>
            </div>
          </div>
          <div className="au-triggers">
            <div className="au-trigger">
              <label className="app-check-line"><input type="checkbox" checked={skip} onChange={(e) => setSkip(e.target.checked)} disabled={busy} />
                <span>{L("上一次的任务还没做完就跳过这次", "Skip a run while the last task is not finished")}<small>{L("免得 agent 那边越堆越多。", "So that work does not pile up at the agent.")}</small></span></label>
            </div>
          </div>

          {a && runs.length > 0 && (
            <div className="app-field">
              <span className="app-field-label">{L("最近运行", "Recent runs")}</span>
              <div className="au-runs" data-testid="automation-runs">
                {runs.slice(0, 8).map((r) => (
                  <div key={r.id} className={cn("au-run", r.outcome === "failed" && "au-failed")}>
                    <span className="au-run-when">{timeAgo(r.at, now)} · {triggerWords(r.trigger)}</span>
                    <span>{outcomeWords(r)}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="au-note">{L("自动化的内容存在工作区的 Hub 上，不是端到端加密的。", "An automation's text is kept on the workspace's hub, not end-to-end encrypted.")}</div>
          <ErrorText error={error} />
          <div className="app-modal-actions">
            {a && <button type="button" className="app-btn app-btn-quiet au-delete" onClick={() => setDeleting(true)} disabled={busy}>{L("删除", "Delete")}</button>}
            {a?.webhook && <button type="button" className="app-btn app-btn-quiet" onClick={() => void rotate()} disabled={busy}>{L("重新生成密钥", "New secret")}</button>}
            <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{L("取消", "Cancel")}</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={busy || !ok}>{a ? L("保存", "Save") : L("创建", "Create")}</button>
          </div>
        </form>
      </Modal>
      <ConfirmDialog open={deleting} onOpenChange={(o) => !o && setDeleting(false)} title={L("删除这个自动化？", "Delete this automation?")}
        description={L("它不会再发消息了，Webhook 地址也失效。已经发出去的消息和 agent 的回复都保留。", "It sends nothing more, and its webhook address stops working. What it sent and the answers stay.")}
        confirmLabel={L("删除", "Delete")} onConfirm={() => void remove()} busy={busy} />
    </>
  );
}
