import type { Agent, Machine, Quota, QuotaWindow } from "@/api";
import { L } from "@/lib/i18n";
import { timeAgo } from "@/lib/utils";

const HOUR = 3600;
const DAY = 24 * HOUR;

/** "Opus" for a model bucket's scope. */
function scopeName(scope: string): string {
  return scope ? scope.charAt(0).toUpperCase() + scope.slice(1) : scope;
}

/** A window's length for people: "5 小时", "每周", "每天", "每月". */
function durationLabel(sec: number): string {
  if (sec === 5 * HOUR) return L("5 小时", "5 hours");
  if (sec === DAY) return L("每天", "Daily");
  if (sec === 7 * DAY) return L("每周", "Weekly");
  if (sec >= 28 * DAY && sec <= 31 * DAY) return L("每月", "Monthly");
  if (sec < 2 * DAY) {
    const h = Math.round(sec / HOUR);
    return L(`${h} 小时`, `${h} hours`);
  }
  const d = Math.round(sec / DAY);
  return L(`${d} 天`, `${d} days`);
}

/** A usage window's name for people: by its length when the source says
 *  it, else by the CLI's own window names; a model's bucket adds the model. */
export function quotaWindowLabel(w: QuotaWindow): string {
  let base: string;
  if (w.duration_sec && w.duration_sec > 0) base = durationLabel(w.duration_sec);
  else if (w.name === "five_hour") base = L("5 小时", "5 hours");
  else if (w.name === "seven_day" || w.name === "seven_day_opus" || w.name === "seven_day_sonnet") base = L("每周", "Weekly");
  else if (w.name === "primary") base = w.label || L("主要窗口", "Primary window");
  else if (w.name === "secondary") base = w.label || L("次要窗口", "Secondary window");
  else base = w.label || w.name;
  const scope = w.model_scope || (w.name === "seven_day_opus" ? "opus" : w.name === "seven_day_sonnet" ? "sonnet" : "");
  return scope ? L(`${base}（${scopeName(scope)}）`, `${base} (${scopeName(scope)})`) : base;
}

/** Whole percent used, 0..100. */
export function quotaPercent(w: QuotaWindow): number {
  return Math.max(0, Math.min(100, Math.round(w.used * 100)));
}

/** Whole percent left, 0..100. */
export function quotaLeft(w: QuotaWindow): number {
  return 100 - quotaPercent(w);
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
export function resetLabel(iso: string | undefined, now = Date.now()): string {
  const t = iso ? new Date(iso).getTime() : NaN;
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

/** "3 小时后重置": how long until a window resets; "" when the source
 *  does not say. */
export function resetIn(iso: string | undefined, now = Date.now()): string {
  const t = iso ? new Date(iso).getTime() : NaN;
  if (!Number.isFinite(t)) return "";
  const s = (t - now) / 1000;
  if (s <= 60) return L("马上重置", "resets now");
  if (s < HOUR) { const m = Math.ceil(s / 60); return L(`${m} 分钟后重置`, `resets in ${m} min`); }
  if (s < 2 * DAY) { const h = Math.round(s / HOUR); return L(`${h} 小时后重置`, `resets in ${h} h`); }
  const d = Math.round(s / DAY);
  return L(`${d} 天后重置`, `resets in ${d} days`);
}

/** A row's value: "剩 38% · 3 小时后重置", or what a spent window says. */
export function quotaWindowText(w: QuotaWindow, now = Date.now()): string {
  const reset = resetIn(w.resets_at, now);
  if (quotaSpent(w)) return [L("额度用完", "Out of quota"), reset].filter(Boolean).join(" · ");
  return [L(`剩 ${quotaLeft(w)}%`, `${quotaLeft(w)}% left`), reset].filter(Boolean).join(" · ");
}

/** A successful read is there: numbers to show. A failure never stands
 *  for a full quota. */
export function quotaHasNumbers(q: Quota): boolean {
  const at = new Date(q.at).getTime();
  return !!q.windows?.length && Number.isFinite(at) && at > Date.UTC(2000, 0, 1);
}

/** The status says the last read failed. */
function quotaFailed(q: Quota): boolean {
  return !!q.status && q.status !== "ok";
}

/** The person has to do something: sign in again, or add a key. */
function quotaNeedsAction(q: Quota): boolean {
  return q.status === "auth_required" || q.status === "no_credentials";
}

/** Why there are no fresh numbers, in a few words: "需要在 CLI 重新登录". */
export function quotaStatusText(q: Quota): string {
  switch (q.status) {
    case "no_credentials": return L("需要配置 key", "Needs an API key");
    case "auth_required": return L("需要在 CLI 重新登录", "Sign in again in the CLI");
    case "unsupported": return L("这个账号不报额度", "This account doesn't report usage");
    case "throttled": return L("查得太勤，稍后再取", "Checked too often; tries again later");
    case "schema_changed": return L("额度格式变了，需要更新 myco", "The usage format changed; update myco");
    case "transient_error": return L("获取失败，稍后再试", "Couldn't fetch; tries again later");
    default: return q.status && q.status !== "ok" ? L("获取失败", "Couldn't fetch") : "";
  }
}

/** A tag's few words for a login or key to fix. */
function actionShort(q: Quota): string {
  return q.status === "no_credentials" ? L("需配置 key", "Needs a key") : L("需重新登录", "Sign in again");
}

/** Usage older than an hour, or kept from before a failed read: the
 *  numbers have moved on. */
export function quotaStaleNote(quota: Quota, now: number): string | undefined {
  if (!quotaHasNumbers(quota)) return undefined;
  const at = new Date(quota.at).getTime();
  if (!quotaFailed(quota) && now - at <= 3600e3) return undefined;
  return L(`${timeAgo(quota.at, now)}取到的数据`, `data from ${timeAgo(quota.at, now)}`);
}

const providerNames: Record<string, string> = { codex: "Codex", claude: "Claude", opencode_go: "OpenCode Go", minimax: "MiniMax" };

function planName(plan: string): string {
  switch (plan) {
    case "chatgpt_subscription": return L("ChatGPT 订阅", "ChatGPT plan");
    case "claude_subscription": return L("Claude 订阅", "Claude plan");
    case "go": return L("Go 套餐", "Go plan");
    case "coding_plan": return "Coding Plan";
    default: return plan;
  }
}

/** "Codex · ChatGPT 订阅 · Studio Mac": whose usage, on which plan and
 *  computer. */
export function quotaSourceText(q: Quota, machines: Machine[] = []): string {
  const who = providerNames[q.provider || q.source] || q.provider || q.source || "";
  const machine = q.machine_id ? machines.find((m) => m.id === q.machine_id)?.name : undefined;
  return [who, q.plan ? planName(q.plan) : "", machine || ""].filter(Boolean).join(" · ");
}

/** The tightest window: the one with the least left (spent first). */
export function quotaWorst(q: Quota): QuotaWindow | undefined {
  if (!q.windows?.length) return undefined;
  return q.windows.reduce((x, w) => {
    if (quotaSpent(w) !== quotaSpent(x)) return quotaSpent(w) ? w : x;
    return w.used > x.used ? w : x;
  });
}

export type QuotaLevel = "ok" | "tight" | "spent" | "action" | "failed";

/** One line for a member: the tightest window left, whose usage, when it
 *  resets, and what is wrong. `short` fits a tag; `line` is the whole say. */
export function quotaSummary(q: Quota, machines: Machine[] = [], now = Date.now()): { level: QuotaLevel; short: string; line: string; stale: boolean } {
  const src = quotaSourceText(q, machines);
  const status = quotaStatusText(q);
  const worst = quotaHasNumbers(q) ? quotaWorst(q) : undefined;
  const stale = !!quotaStaleNote(q, now);
  if (!worst) {
    const why = status || L("还没有额度数据", "No usage yet");
    return { level: quotaNeedsAction(q) ? "action" : "failed", short: quotaNeedsAction(q) ? actionShort(q) : L("额度未知", "Usage unknown"), line: L(`额度：${why}（${src}）`, `Usage: ${why} (${src})`), stale: false };
  }
  const reset = resetIn(worst.resets_at, now);
  const what = quotaSpent(worst) ? L("用完了", "out") : L(`剩 ${quotaLeft(worst)}%`, `${quotaLeft(worst)}% left`);
  const label = quotaWindowLabel(worst);
  let line = L(`额度：${label}${what}（${src}）${reset ? "，" + reset : ""}`, `Usage: ${label} ${what} (${src})${reset ? "; " + reset : ""}`);
  if (status) line += L(`。${status}，${quotaStaleNote(q, now)}`, `. ${status}; ${quotaStaleNote(q, now)}`);
  else if (stale) line += L(`（${quotaStaleNote(q, now)}）`, ` (${quotaStaleNote(q, now)})`);
  const level: QuotaLevel = quotaNeedsAction(q) ? "action" : quotaSpent(worst) ? "spent" : quotaTight(worst) ? "tight" : "ok";
  return {
    level,
    short: quotaNeedsAction(q) ? actionShort(q) : quotaSpent(worst) ? L("额度用完", "Out of quota") : L(`剩 ${quotaLeft(worst)}%`, `${quotaLeft(worst)}% left`),
    line,
    stale: stale || quotaFailed(q),
  };
}

/** A node with a window nearly (or fully) used, or a login to fix: the
 *  quiet hint other pages show. The worst window decides. */
export function agentQuotaHint(a: Agent, now = Date.now()): { level: "tight" | "spent"; text: string; title: string } | undefined {
  const quota = a.runtime?.quota;
  if (!quota) return undefined;
  const s = quotaSummary(quota, [], now);
  if (s.level === "ok" || s.level === "failed") return undefined;
  return { level: s.level === "tight" ? "tight" : "spent", text: s.short, title: s.line };
}
