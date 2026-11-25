import { useEffect, useMemo, useState } from "react";
import { Gauge } from "lucide-react";
import type { Agent, Project } from "@/api";
import { appApi, isOldHub } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { Avatar } from "@/app/persona";
import { PageHero, SectionHead, SectionSkeleton } from "@/app/PageHero";
import { ErrorText } from "@/app/dialogs";
import { RowsSkeleton } from "@/app/Skeleton";
import { useContextMenu } from "@/app/ContextMenu";
import { L, locale } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { compactUsage, emptyUsage, summarizeUsage, usageDays, usageRange, type UsageAgent, type UsageResponse, type UsageTotals } from "@/lib/usage";
import "@/app/usage.css";

// The usage page (app-ui.md › Usage): where the workspace's tokens go and
// who uses the most. Four figures, one neutral bar a day with a readout,
// then members, projects and models ranked by total tokens. The data and
// its rules live in lib/usage.ts; this file only lays it out.

const RANGE_KEY = "myco-usage-days";
function savedDays(): number {
  try { const n = Number(localStorage.getItem(RANGE_KEY)); return [7, 30, 90].includes(n) ? n : 30; } catch { return 30; }
}
function savedProject(hub: string): string {
  try { return localStorage.getItem(`myco-usage-project:${hub}`) || ""; } catch { return ""; }
}
const percent = (n: number, total: number) => total ? `${(100 * n / total).toFixed(1)}%` : "—";
const money = (n: number) => n > 0 ? new Intl.NumberFormat(locale(), { style: "currency", currency: "USD", maximumFractionDigits: 2 }).format(n) : "—";
const exact = (n: number) => new Intl.NumberFormat(locale(), { maximumFractionDigits: 0 }).format(n);
const dayText = (date: string) => new Date(`${date}T00:00:00Z`).toLocaleDateString(locale(), { month: "short", day: "numeric", timeZone: "UTC" });
function NumberValue({ value }: { value: number }) { return <span title={exact(value)}>{compactUsage(value)}</span>; }

/** Its share of the whole: a bar and the percentage beside it. */
function ShareBar({ value, total }: { value: number; total: number }) {
  const w = total ? Math.min(100, value / total * 100) : 0;
  return <span className="usage-share" title={`${exact(value)} / ${exact(total)}`}>
    <span className="usage-share-track" aria-hidden><span style={{ width: `${Math.max(w, value > 0 ? 1.5 : 0)}%` }} /></span>
    <span className="usage-share-pct">{percent(value, total)}</span>
  </span>;
}
function avatarAgent(info: UsageAgent): Agent {
  return { id: info.name, name: info.name, display_name: info.display_name, owner: info.owner, kind: "agent", binding: { harness: info.harness }, description: "", capabilities: [], labels: [], delivery: { mode: "pull" }, allow_from: [], online: false, created_at: "" };
}
const costNote = () => L("仅合计 CLI 上报的费用；未上报的费用不计入，不能当作零费用。", "Only CLI-reported costs are included; missing costs do not mean free usage.");

/** The top of the axis: the largest day rounded up to 1, 2, 2.5 or 5 × 10ⁿ. */
function niceMax(n: number): number {
  if (n <= 0) return 1;
  const p = 10 ** Math.floor(Math.log10(n));
  for (const m of [1, 2, 2.5, 5, 10]) if (n <= m * p) return m * p;
  return 10 * p;
}

// dayLabel: which bars carry their date: the first, the last, and every
// week (every two in a long range) not crowding the last.
function dayLabel(i: number, n: number) {
  const step = n <= 7 ? 1 : n <= 31 ? 7 : 14;
  return i === 0 || i === n - 1 || (i % step === 0 && n - 1 - i >= step / 2);
}

/** The last seven days against the seven before them, when both are in the
 *  range and the earlier ones had any. */
function weekTrend(dates: string[], totals: Map<string, UsageTotals>): number | null {
  if (dates.length < 14) return null;
  const sum = (ds: string[]) => ds.reduce((s, d) => s + (totals.get(d)?.total || 0), 0);
  const last = sum(dates.slice(-7)), before = sum(dates.slice(-14, -7));
  return before > 0 ? (last - before) / before : null;
}

function DailyChart({ from, to, totals, total }: { from: string; to: string; totals: Map<string, UsageTotals>; total: UsageTotals }) {
  const [active, setActive] = useState<string>();
  const dates = usageDays(from, to);
  const peak = Math.max(0, ...dates.map((d) => totals.get(d)?.total || 0));
  const top = niceMax(peak);
  const avg = dates.length ? total.total / dates.length : 0;
  const peakDay = dates.find((d) => (totals.get(d)?.total || 0) === peak);
  const selected = active && dates.includes(active) ? active : undefined;
  const fields = (d: UsageTotals) => [[L("输入", "Input"), d.input], [L("缓存写", "Cache write"), d.cacheWrite], [L("缓存读", "Cache read"), d.cacheRead], [L("输出", "Output"), d.output], [L("调用", "Calls"), d.calls]] as const;
  const trend = weekTrend(dates, totals);
  const detail = selected ? totals.get(selected) || emptyUsage() : undefined;
  return <section aria-labelledby="usage-daily-title">
    <SectionHead title={L("每天的用量", "Daily usage")}>
      <span className="usage-range-text">{dayText(from)} – {dayText(to)} · UTC</span>
    </SectionHead>
    <div className="usage-card usage-chart-card">
      {/* What the bar under the pointer holds; with none, the range at a glance. */}
      <div className="usage-readout" aria-live="polite">
        {!peak ? null : detail ? <>
          <span className="usage-readout-key">{dayText(selected!)}</span>
          <b>{compactUsage(detail.total)}</b>
          <span className="usage-readout-parts">{fields(detail).map(([label, n]) => <span key={label}>{label} {compactUsage(n)}</span>)}</span>
        </> : <>
          <span className="usage-readout-key"><span className="usage-avg-key" aria-hidden />{L("日均", "Daily average")}</span>
          <b>{compactUsage(avg)}</b>
          <span className="usage-readout-parts">
            {peak > 0 && peakDay && <span>{L(`最多 ${dayText(peakDay)} ${compactUsage(peak)}`, `Peak ${dayText(peakDay)} ${compactUsage(peak)}`)}</span>}
            {trend !== null && <span>{L("最近 7 天比前 7 天", "Last 7 days vs the 7 before")} <em className="usage-trend">{trend >= 0 ? "↑" : "↓"} {Math.abs(trend * 100).toFixed(0)}%</em></span>}
          </span>
        </>}
      </div>
      <div className="usage-plot">
        <div className="usage-axis" aria-hidden>
          {[top, top / 2, 0].map((v, i) => <span key={i}>{!v ? "0" : peak ? compactUsage(v) : ""}</span>)}
        </div>
        <div className="usage-plot-area">
          <div className="usage-grid" aria-hidden>
            <span /><span /><span />
            {avg > 0 && <div className="usage-avg" style={{ bottom: `${avg / top * 100}%` }} />}
          </div>
          <div className="usage-chart" role="group" aria-label={L("每日 token 用量", "Daily token usage")} onMouseLeave={() => setActive(undefined)}>
            {dates.map((date, i) => {
              const d = totals.get(date) || emptyUsage();
              const label = `${dayText(date)} · ${L("总量", "Total")} ${exact(d.total)} · ${fields(d).map(([label, n]) => `${label} ${exact(n)}`).join(" · ")}`;
              return <button key={date} type="button" className={cn("usage-day", selected === date && "is-on")} aria-label={label}
                onMouseEnter={() => setActive(date)} onFocus={() => setActive(date)} onBlur={() => setActive(undefined)} onClick={() => setActive(date)}>
                <span className="usage-day-column"><span style={{ height: d.total ? `max(2px, ${d.total / top * 100}%)` : 0 }} /></span>
                <span className="usage-day-label">{dayLabel(i, dates.length) ? dayText(date) : ""}</span>
              </button>;
            })}
          </div>
        </div>
      </div>
      {!peak && <div className="usage-empty" role="status">
        <b>{L("这段时间还没有用量记录", "No usage recorded for this period")}</b>
        <span>{L("成员升级到新版 myco 后，CLI 上报的用量会显示在这里。", "Usage appears after members upgrade myco and their CLI reports it.")}</span>
      </div>}
    </div>
  </section>;
}

/** The page while its first figures load: their shape, not a word. */
function UsageSkeleton() {
  return <div aria-busy="true" aria-label={L("正在读取用量…", "Loading usage…")}>
    <div className="au-stats usage-metrics-skel">{[0, 1, 2, 3].map((i) => <div key={i} className="au-stat"><span className="skel skel-line is-thin" style={{ width: "40%" }} /><span className="skel skel-line" style={{ width: "55%", height: 18 }} /><span className="skel skel-line is-thin" style={{ width: "35%" }} /></div>)}</div>
    <SectionSkeleton />
    <div className="usage-card usage-chart-skel"><span className="skel" /></div>
    <SectionSkeleton />
    <RowsSkeleton rows={4} className="pc-list" />
  </div>;
}

export function UsagePage({ hubId, projects, rows, onOpenAgent }: {
  hubId: string; projects: Project[]; rows: AgentRow[]; onOpenAgent: (name: string) => void;
}) {
  const menu = useContextMenu();
  const [days, setDays] = useState(savedDays);
  const [project, setProject] = useState(() => savedProject(hubId));
  const [result, setResult] = useState<{ key: string; data?: UsageResponse; error?: unknown; old?: boolean }>();
  const range = usageRange(days);
  const key = `${hubId}:${range.from}:${range.to}:${project}`;
  const current = result?.key === key ? result : undefined;
  const data = current?.data;
  useEffect(() => {
    const request = new AbortController();
    appApi.usage(hubId, { ...range, project: project || undefined }, request.signal).then((data) => {
      if (!request.signal.aborted) setResult({ key, data });
    }).catch((error) => {
      if (!request.signal.aborted) setResult(isOldHub(error) || (error as { status?: number })?.status === 404 ? { key, old: true } : { key, error });
    });
    return () => request.abort();
  }, [key, hubId, range.from, range.to, project]);
  const summary = useMemo(() => summarizeUsage(data?.rows || []), [data]);
  const members = new Map((data?.agents || []).map((a) => [a.name, a]));
  const projectNames = new Map(projects.map((p) => [p.name, p.title || p.name]));
  if (project && project !== "-" && !projectNames.has(project)) projectNames.set(project, project);
  for (const [name] of summary.projects) if (name && !projectNames.has(name)) projectNames.set(name, name);
  const changeProject = (value: string) => {
    setProject(value);
    try { localStorage.setItem(`myco-usage-project:${hubId}`, value); } catch { /* optional */ }
  };
  const t = summary.total;
  const nDays = usageDays(range.from, range.to).length;
  const unreported = summary.agents.filter(([, s]) => s.total > 0 && !s.cost).length;
  const metrics = [
    { label: L("总 token", "Total tokens"), value: compactUsage(t.total), hint: exact(t.total), sub: L(`日均 ${compactUsage(t.total / nDays)}`, `${compactUsage(t.total / nDays)} a day`) },
    { label: L("输出 token", "Output tokens"), value: compactUsage(t.output), hint: exact(t.output), sub: t.total ? L(`占总量 ${percent(t.output, t.total)}`, `${percent(t.output, t.total)} of the total`) : "" },
    { label: L("调用次数", "Calls"), value: compactUsage(t.calls), hint: exact(t.calls), sub: L(`日均 ${compactUsage(t.calls / nDays)}`, `${compactUsage(t.calls / nDays)} a day`) },
    { label: L("已上报费用", "Reported cost"), value: money(t.cost), hint: costNote(), sub: unreported ? L(`${unreported} 个成员没有上报费用`, `${unreported} ${unreported === 1 ? "member" : "members"} reported no cost`) : t.cost ? L("都已上报", "All reported") : "" },
  ];
  const open = (name: string) => onOpenAgent(name);
  return <div className="pc au usage-page">
    {menu.element}
    <PageHero title={L("用量", "Usage")} subtitle={L("看看工作区的 token 用在哪里，哪些成员用得最多。", "See where workspace tokens go and which members use the most.")} />
    <div className="usage-filters">
      <div className="wf-chips usage-range" role="group" aria-label={L("时间范围", "Date range")}>
        {[7, 30, 90].map((n) => <button key={n} type="button" className={cn("wf-chip", days === n && "is-on")} aria-pressed={days === n} onClick={() => {
          setDays(n); try { localStorage.setItem(RANGE_KEY, String(n)); } catch { /* optional */ }
        }}>{L(`${n} 天`, `${n} days`)}</button>)}
      </div>
      <select className="app-select-field" aria-label={L("项目", "Project")} value={project} onChange={(e) => changeProject(e.target.value)}>
        <option value="">{L("全部项目", "All projects")}</option>
        {[...projectNames].map(([name, title]) => <option key={name} value={name}>{title}</option>)}
        <option value="-">{L("不属于项目", "No project")}</option>
      </select>
    </div>
    {current?.old ? <div className="app-empty">
        <div className="app-empty-icon"><Gauge /></div>
        <h2>{L("这个工作区还不能看用量", "No usage for this workspace yet")}</h2>
        <p>{L("它的 Hub 版本太旧，还不统计用量。在服务器上运行 sudo myco upgrade 升级 Hub 后就能看了。", "Its hub is too old to count usage: run sudo myco upgrade on the server, then come back.")}</p>
      </div>
      : current?.error ? <ErrorText error={current.error} />
      : !current ? <UsageSkeleton />
      : <>
        <dl className="au-stats usage-metrics" aria-label={L("用量概览", "Usage summary")}>
          {metrics.map((m) => <div key={m.label} className="au-stat" title={m.hint}>
            <dt>{m.label}</dt><dd className={cn(!t.total && "is-quiet")}>{m.value}</dd><small>{m.sub || " "}</small>
          </div>)}
        </dl>
        <DailyChart {...range} totals={summary.days} total={t} />
        <section aria-labelledby="usage-members-title">
          <div className="ph-section"><h2 id="usage-members-title">{L("按成员", "By member")}{!!summary.agents.length && <span className="ph-count">{summary.agents.length}</span>}</h2></div>
          <div className="usage-card">
            {!summary.agents.length ? <div className="app-empty-note">{L("这段时间没有成员用过。", "No member used any tokens in this period.")}</div>
            : <div className="usage-table-wrap"><table className="usage-table usage-agents">
              <caption className="app-sr-only">{L("按成员统计的用量", "Usage by member")}</caption>
              <thead><tr>
                <th>{L("成员", "Member")}</th>
                <th className="usage-share-col">{L("占比", "Share")}</th>
                <th className="usage-numeric">{L("总 token", "Total")}</th>
                <th className="usage-numeric">{L("输出", "Output")}</th>
                <th className="usage-secondary usage-numeric">{L("调用", "Calls")}</th>
                <th className="usage-secondary usage-numeric" title={L("缓存读取占全部输入（含缓存读写）的比例", "Cache reads as a share of all input, including cache reads and writes")}>{L("缓存读", "Cache read")}</th>
                <th className="usage-secondary usage-numeric" title={costNote()}>{L("费用", "Cost")}</th>
              </tr></thead>
              <tbody>{summary.agents.map(([name, s]) => {
                const info = members.get(name) || { name };
                const known = rows.find((r) => r.hub.id === hubId && r.st.agent.name === name)?.st.agent;
                const models = [...s.models].join(", ");
                const who = [info.owner, models].filter(Boolean).join(" · ");
                return <tr key={name} onClick={() => open(name)} onContextMenu={(e) => menu.open(e, [{ label: L("打开会话", "Open conversation"), onSelect: () => open(name) }])}>
                  <th scope="row">
                    <button type="button" className="usage-member" title={L(`打开 ${info.display_name || name} 的会话`, `Open conversation with ${info.display_name || name}`)} onClick={(e) => { e.stopPropagation(); open(name); }}>
                      <Avatar agent={known || avatarAgent(info)} size={28} />
                      <span className="usage-member-text">
                        <span className="usage-member-name">{info.display_name || name}</span>
                        {who && <span className="usage-member-sub" title={who}>{who}</span>}
                      </span>
                    </button>
                  </th>
                  <td className="usage-share-col"><ShareBar value={s.total} total={t.total} /></td>
                  <td className="usage-numeric usage-strong"><NumberValue value={s.total} /></td>
                  <td className="usage-numeric"><NumberValue value={s.output} /></td>
                  <td className="usage-secondary usage-numeric"><NumberValue value={s.calls} /></td>
                  <td className="usage-secondary usage-numeric">{percent(s.cacheRead, s.input + s.cacheWrite + s.cacheRead)}</td>
                  <td className="usage-secondary usage-numeric" title={costNote()}>{money(s.cost)}</td>
                </tr>;
              })}</tbody>
            </table></div>}
          </div>
        </section>
        <div className={cn("usage-pair", project && "is-one")}>
          {!project && <section aria-labelledby="usage-projects-title">
            <div className="ph-section"><h2 id="usage-projects-title">{L("按项目", "By project")}</h2></div>
            <div className="usage-card">
              {!summary.projects.length ? <div className="app-empty-note">{L("这段时间没有项目用过。", "No project used any tokens in this period.")}</div>
              : <table className="usage-table usage-projects">
                <caption className="app-sr-only">{L("按项目统计的用量", "Usage by project")}</caption>
                <thead><tr><th>{L("项目", "Project")}</th><th className="usage-share-col">{L("占比", "Share")}</th><th className="usage-numeric">{L("总 token", "Total")}</th><th className="usage-secondary usage-numeric">{L("成员", "Members")}</th></tr></thead>
                <tbody>{summary.projects.map(([name, s]) => <tr key={name}>
                  <th scope="row"><span className={cn("usage-name", !name && "is-quiet")}>{name ? projectNames.get(name) || name : L("不属于项目", "No project")}</span></th>
                  <td className="usage-share-col"><ShareBar value={s.total} total={t.total} /></td>
                  <td className="usage-numeric usage-strong"><NumberValue value={s.total} /></td>
                  <td className="usage-secondary usage-numeric">{s.agents.size}</td>
                </tr>)}</tbody>
              </table>}
            </div>
          </section>}
          <section aria-labelledby="usage-models-title">
            <div className="ph-section"><h2 id="usage-models-title">{L("按模型", "By model")}</h2></div>
            <div className="usage-card">
              {!summary.models.length ? <div className="app-empty-note">{L("这段时间没有模型的记录。", "No model was used in this period.")}</div>
              : <table className="usage-table usage-model-table">
                <caption className="app-sr-only">{L("按模型统计的用量", "Usage by model")}</caption>
                <thead><tr><th>{L("模型", "Model")}</th><th className="usage-share-col">{L("占比", "Share")}</th><th className="usage-numeric">{L("总 token", "Total")}</th></tr></thead>
                <tbody>{summary.models.map(([model, s]) => <tr key={model}>
                  <th scope="row"><span className={cn("usage-name", model ? "mono" : "is-quiet")} title={model}>{model || L("未上报", "Not reported")}</span></th>
                  <td className="usage-share-col"><ShareBar value={s.total} total={t.total} /></td>
                  <td className="usage-numeric usage-strong"><NumberValue value={s.total} /></td>
                </tr>)}</tbody>
              </table>}
            </div>
          </section>
        </div>
        <p className="usage-note">{L("数字来自各 agent 的 CLI 自己上报；Claude Code 的费用按 Anthropic 价格算，接其他模型时仅供参考；需要 runner 或会话升级到新版 myco 才会上报。", "Figures are reported by each agent’s CLI. Claude Code costs use Anthropic prices and are estimates when using other models. The runner or session must upgrade myco to report usage.")} {costNote()}</p>
      </>}
  </div>;
}
