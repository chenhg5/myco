import { useEffect, useMemo, useState } from "react";
import { Gauge } from "lucide-react";
import type { Agent, Project } from "@/api";
import { appApi, isOldHub } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { Avatar } from "@/app/persona";
import { PageHero } from "@/app/PageHero";
import { ErrorText } from "@/app/dialogs";
import { useContextMenu } from "@/app/ContextMenu";
import { L, locale } from "@/lib/i18n";
import { compactUsage, emptyUsage, summarizeUsage, usageDays, usageRange, type UsageAgent, type UsageResponse, type UsageTotals } from "@/lib/usage";
import "@/app/usage.css";

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
function NumberValue({ value }: { value: number }) { return <span title={exact(value)}>{compactUsage(value)}</span>; }
function ShareBar({ value, total }: { value: number; total: number }) {
  return <span className="usage-share"><NumberValue value={value} /><span className="usage-share-track" aria-hidden><span style={{ width: `${total ? Math.min(100, value / total * 100) : 0}%` }} /></span></span>;
}
function avatarAgent(info: UsageAgent): Agent {
  return { id: info.name, name: info.name, display_name: info.display_name, owner: info.owner, kind: "agent", binding: { harness: info.harness }, description: "", capabilities: [], labels: [], delivery: { mode: "pull" }, allow_from: [], online: false, created_at: "" };
}
const costNote = () => L("仅合计 CLI 上报的费用；未上报的费用不计入，不能当作零费用。", "Only CLI-reported costs are included; missing costs do not mean free usage.");

function DailyChart({ from, to, totals }: { from: string; to: string; days: number; totals: Map<string, UsageTotals> }) {
  const [active, setActive] = useState<string>();
  const dates = usageDays(from, to);
  const max = Math.max(1, ...[...totals.values()].map((d) => d.total));
  const selected = active && dates.includes(active) ? active : to;
  const detail = totals.get(selected) || emptyUsage();
  const dayText = (date: string) => new Date(`${date}T00:00:00Z`).toLocaleDateString(locale(), { month: "short", day: "numeric", timeZone: "UTC" });
  const fields = (d: UsageTotals) => [[L("总量", "Total"), d.total], [L("输入", "Input"), d.input], [L("缓存写", "Cache write"), d.cacheWrite], [L("缓存读", "Cache read"), d.cacheRead], [L("输出", "Output"), d.output], [L("调用", "Calls"), d.calls]] as const;
  return <section className="usage-section" aria-labelledby="usage-daily-title">
    <div className="usage-section-head"><h2 id="usage-daily-title">{L("每天的用量", "Daily usage")}</h2><span>{from} – {to} · UTC</span></div>
    <div className="usage-chart" role="group" aria-label={L("每日 token 用量", "Daily token usage")}>
      {dates.map((date, i) => {
        const d = totals.get(date) || emptyUsage();
        const label = `${dayText(date)} · ${fields(d).map(([label, n]) => `${label} ${exact(n)}`).join(" · ")}`;
        return <button key={date} type="button" className="usage-day" title={label} aria-label={label}
          onMouseEnter={() => setActive(date)} onFocus={() => setActive(date)} onClick={() => setActive(date)}>
          <span className="usage-day-column"><span style={{ height: `${d.total / max * 100}%` }} /></span>
          <span className="usage-day-label">{dayLabel(i, dates.length) ? date.slice(5) : ""}</span>
        </button>;
      })}
    </div>
    <div className="usage-chart-detail" aria-live="polite"><strong>{dayText(selected)}</strong>{fields(detail).map(([label, n]) => <span key={label}>{label} {compactUsage(n)}</span>)}</div>
  </section>;
}

// dayLabel: which bars carry their date: the first, the last, and every
// week (every two in a long range) not crowding the last.
function dayLabel(i: number, n: number) {
  const step = n <= 7 ? 1 : n <= 31 ? 7 : 14;
  return i === 0 || i === n - 1 || (i % step === 0 && n - 1 - i >= step / 2);
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
    let off = false;
    appApi.usage(hubId, { ...range, project: project || undefined }).then((data) => {
      if (!off) setResult({ key, data });
    }).catch((error) => {
      if (!off) setResult(isOldHub(error) || (error as { status?: number })?.status === 404 ? { key, old: true } : { key, error });
    });
    return () => { off = true; };
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
  return <div className="app-page usage-page">
    {menu.element}
    <PageHero title={L("用量", "Usage")} subtitle={L("看看工作区的 token 用在哪里，哪些成员用得最多。", "See where workspace tokens go and which members use the most.")} />
    <div className="app-page-head usage-filters">
      <div className="usage-range" role="group" aria-label={L("时间范围", "Date range")}>
        {[7, 30, 90].map((n) => <button key={n} type="button" className={days === n ? "active" : ""} aria-pressed={days === n} onClick={() => {
          setDays(n); try { localStorage.setItem(RANGE_KEY, String(n)); } catch { /* optional */ }
        }}>{L(`${n} 天`, `${n} days`)}</button>)}
      </div>
      <select className="app-select-field" aria-label={L("项目", "Project")} value={project} onChange={(e) => changeProject(e.target.value)}>
        <option value="">{L("全部项目", "All projects")}</option>
        {[...projectNames].map(([name, title]) => <option key={name} value={name}>{title}</option>)}
        <option value="-">{L("不属于项目", "No project")}</option>
      </select>
    </div>
    {current?.old ? <div className="app-empty"><Gauge /><h2>{L("这个工作区的 Hub 还不支持用量统计，升级 Hub 后可用", "This workspace’s Hub does not support usage statistics yet. Upgrade the Hub to use this page.")}</h2></div>
      : current?.error ? <ErrorText error={current.error} />
      : !current ? <div className="usage-loading" role="status">{L("正在读取用量…", "Loading usage…")}</div>
      : <>
        <dl className="usage-metrics" aria-label={L("用量概览", "Usage summary")}>
          {[{ label: L("总 token", "Total tokens"), value: compactUsage(summary.total.total), hint: exact(summary.total.total) },
            { label: L("输出 token", "Output tokens"), value: compactUsage(summary.total.output), hint: exact(summary.total.output) },
            { label: L("调用次数", "Calls"), value: compactUsage(summary.total.calls), hint: exact(summary.total.calls) },
            { label: L("已上报费用", "Reported cost"), value: money(summary.total.cost), hint: costNote() }].map((m) => <div key={m.label} title={m.hint}><dt>{m.label}</dt><dd>{m.value}</dd></div>)}
        </dl>
        {!data?.rows.length && <div className="usage-empty" role="status">{L("这段时间还没有用量记录。成员升级到新版 myco 后，CLI 上报的用量会显示在这里。", "No usage recorded for this period. Usage appears after members upgrade myco and their CLI reports it.")}</div>}
        <DailyChart {...range} days={days} totals={summary.days} />
        <section className="usage-section"><h2>{L("按成员", "By member")}</h2>
          <div className="usage-table-wrap"><table className="usage-table usage-agents">
            <caption className="app-sr-only">{L("按成员统计的用量", "Usage by member")}</caption>
            <thead><tr><th>{L("成员", "Member")}</th><th className="usage-secondary">{L("所有者", "Owner")}</th><th className="usage-secondary">{L("模型", "Models")}</th><th className="usage-secondary usage-numeric">{L("调用", "Calls")}</th><th className="usage-numeric">{L("总 token", "Total tokens")}</th><th className="usage-numeric">{L("输出", "Output")}</th><th className="usage-secondary usage-numeric" title={L("缓存读取占全部输入（含缓存读写）的比例", "Cache reads as a share of all input, including cache reads and writes")}>{L("缓存读占比", "Cache read %")}</th><th className="usage-secondary usage-numeric" title={costNote()}>{L("费用", "Cost")}</th></tr></thead>
            <tbody>{summary.agents.map(([name, s]) => {
              const info = members.get(name) || { name };
              const known = rows.find((r) => r.hub.id === hubId && r.st.agent.name === name)?.st.agent;
              return <tr key={name} onClick={() => onOpenAgent(name)} onContextMenu={(e) => menu.open(e, [{ label: L("打开会话", "Open conversation"), onSelect: () => onOpenAgent(name) }])}>
                <th scope="row"><button type="button" className="usage-member" title={L(`打开 ${info.display_name || name} 的会话`, `Open conversation with ${info.display_name || name}`)} onClick={(e) => { e.stopPropagation(); onOpenAgent(name); }}><Avatar agent={known || avatarAgent(info)} size={24} /><span>{info.display_name || name}</span></button></th>
                <td className="usage-secondary">{info.owner || "—"}</td><td className="usage-secondary usage-models" title={[...s.models].join(", ")}>{[...s.models].join(", ") || "—"}</td>
                <td className="usage-secondary usage-numeric"><NumberValue value={s.calls} /></td><td className="usage-numeric"><ShareBar value={s.total} total={summary.total.total} /></td><td className="usage-numeric"><NumberValue value={s.output} /></td><td className="usage-secondary usage-numeric">{percent(s.cacheRead, s.input + s.cacheWrite + s.cacheRead)}</td><td className="usage-secondary usage-numeric" title={costNote()}>{money(s.cost)}</td>
              </tr>;
            })}</tbody>
          </table></div>
        </section>
        {!project && <section className="usage-section"><h2>{L("按项目", "By project")}</h2>
          <div className="usage-table-wrap"><table className="usage-table usage-projects">
            <caption className="app-sr-only">{L("按项目统计的用量", "Usage by project")}</caption>
            <thead><tr><th>{L("项目", "Project")}</th><th className="usage-secondary usage-numeric">{L("成员数", "Members")}</th><th className="usage-numeric">{L("总 token", "Total tokens")}</th><th className="usage-numeric">{L("输出", "Output")}</th><th className="usage-numeric">{L("占比", "Share")}</th></tr></thead>
            <tbody>{summary.projects.map(([name, s]) => <tr key={name}><th scope="row">{name ? projectNames.get(name) || name : L("不属于项目", "No project")}</th><td className="usage-secondary usage-numeric">{s.agents.size}</td><td className="usage-numeric"><ShareBar value={s.total} total={summary.total.total} /></td><td className="usage-numeric"><NumberValue value={s.output} /></td><td className="usage-numeric">{percent(s.total, summary.total.total)}</td></tr>)}</tbody>
          </table></div>
        </section>}
        <section className="usage-section"><h2>{L("按模型", "By model")}</h2>
          <div className="usage-table-wrap"><table className="usage-table usage-model-table"><caption className="app-sr-only">{L("按模型统计的用量", "Usage by model")}</caption><thead><tr><th>{L("模型", "Model")}</th><th className="usage-numeric">{L("总 token", "Total tokens")}</th><th className="usage-numeric">{L("占比", "Share")}</th></tr></thead><tbody>{summary.models.map(([model, s]) => <tr key={model}><th scope="row">{model || L("未上报", "Not reported")}</th><td className="usage-numeric"><NumberValue value={s.total} /></td><td className="usage-numeric">{percent(s.total, summary.total.total)}</td></tr>)}</tbody></table></div>
        </section>
        <p className="usage-note">{L("数字来自各 agent 的 CLI 自己上报；Claude Code 的费用按 Anthropic 价格算，接其他模型时仅供参考；需要 runner 或会话升级到新版 myco 才会上报。", "Figures are reported by each agent’s CLI. Claude Code costs use Anthropic prices and are estimates when using other models. The runner or session must upgrade myco to report usage.")} {costNote()}</p>
      </>}
  </div>;
}
