import type { Agent, Quota, QuotaWindow } from "@/api";
import { L } from "@/lib/i18n";
import { timeAgo } from "@/lib/utils";

/** A usage window's name for people. Five-hour and seven-day are the CLI's
 *  own windows; anything else shows its label (or given name). */
export function quotaWindowLabel(w: QuotaWindow): string {
  if (w.name === "five_hour") return L("5 小时", "5 hours");
  if (w.name === "seven_day") return L("7 天", "7 days");
  if (w.name === "seven_day_opus") return L("7 天（Opus）", "7 days (Opus)");
  if (w.name === "seven_day_sonnet") return L("7 天（Sonnet）", "7 days (Sonnet)");
  if (w.name === "primary") return w.label || L("主要窗口", "Primary window");
  if (w.name === "secondary") return w.label || L("次要窗口", "Secondary window");
  return w.label || w.name;
}

/** Whole percent used, 0..100. */
export function quotaPercent(w: QuotaWindow): number {
  return Math.max(0, Math.min(100, Math.round(w.used * 100)));
}

/** A window with no room left: its status says so, or it is 100% used. */
export function quotaSpent(w: QuotaWindow): boolean {
  return (!!w.status && w.status !== "allowed") || w.used >= 1;
}

/** A window worth a warning: 90% used or more, or spent. */
export function quotaTight(w: QuotaWindow): boolean {
  return quotaSpent(w) || w.used >= 0.9;
}

/** "3:40", "周五" or "11 月 20 日": when a window resets. */
export function resetLabel(iso: string, now = Date.now()): string {
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  const d = new Date(t);
  const diff = t - now;
  if (diff <= 0) return L("马上", "now");
  if (diff < 24 * 3600e3) return `${d.getHours()}:${String(d.getMinutes()).padStart(2, "0")}`;
  if (diff < 7 * 24 * 3600e3) {
    return L(
      ["周日", "周一", "周二", "周三", "周四", "周五", "周六"][d.getDay()],
      ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"][d.getDay()],
    );
  }
  return L(`${d.getMonth() + 1} 月 ${d.getDate()} 日`, d.toLocaleDateString("en-US", { month: "short", day: "numeric" }));
}

/** A row's value: "已用 62% · 3:40 重置", or what a spent window says. */
export function quotaWindowText(w: QuotaWindow, now = Date.now()): string {
  const reset = resetLabel(w.resets_at, now);
  if (quotaSpent(w)) return L(`额度用完，${reset} 重置`, `Out of quota; resets ${reset}`);
  const pct = quotaPercent(w);
  return L(`已用 ${pct}% · ${reset} 重置`, `${pct}% used · resets ${reset}`);
}

/** Usage older than an hour is worth saying: the numbers have moved on. */
export function quotaStaleNote(quota: Quota, now: number): string | undefined {
  const at = new Date(quota.at).getTime();
  if (!Number.isFinite(at) || now - at <= 3600e3) return undefined;
  return L(`${timeAgo(quota.at, now)}的数据`, `data from ${timeAgo(quota.at, now)}`);
}

/** A node with a window nearly (or fully) used: the quiet hint other pages
 *  show. The worst window decides. */
export function agentQuotaHint(a: Agent, now = Date.now()): { level: "tight" | "spent"; text: string; title: string } | undefined {
  const quota = a.runtime?.quota;
  if (!quota?.windows?.length) return undefined;
  const spent = quota.windows.find(quotaSpent);
  if (spent) {
    return {
      level: "spent",
      text: L("额度用完", "Out of quota"),
      title: L(`${quotaWindowLabel(spent)}的额度用完了，${resetLabel(spent.resets_at, now)} 重置`, `${quotaWindowLabel(spent)} is out of quota; resets ${resetLabel(spent.resets_at, now)}`),
    };
  }
  const worst = quota.windows.reduce((x, w) => (w.used > x.used ? w : x));
  if (!quotaTight(worst)) return undefined;
  const pct = quotaPercent(worst);
  return {
    level: "tight",
    text: L(`额度 ${pct}%`, `${pct}% quota`),
    title: L(`${quotaWindowLabel(worst)}已用 ${pct}%，${resetLabel(worst.resets_at, now)} 重置`, `${quotaWindowLabel(worst)} is ${pct}% used; resets ${resetLabel(worst.resets_at, now)}`),
  };
}
