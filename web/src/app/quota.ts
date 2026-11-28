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
  else if (w.name === "monthly") base = L("每月", "Monthly");
  else if (w.name === "weekly") base = L("每周", "Weekly");
  else if (w.name === "daily") base = L("每天", "Daily");
  else if (w.name === "rolling") base = L("滚动窗口", "Rolling window");
  else if (/^limit_\d+$/.test(w.name)) { const n = Number(w.name.slice(6)) + 1; base = L(`额度 ${n}`, `Limit ${n}`); }
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

/** The windows that still describe the account: one whose reset time has
 *  passed is no longer the window those numbers were read for. */
export function quotaLiveWindows(q: Quota, now = Date.now()): QuotaWindow[] {
  return (q.windows ?? []).filter((w) => {
    const t = w.resets_at ? new Date(w.resets_at).getTime() : NaN;
    return !Number.isFinite(t) || t > now;
  });
}

/** A successful read is there: numbers to show. A failure never stands
 *  for a full quota. */
export function quotaHasNumbers(q: Quota, now = Date.now()): boolean {
  const at = new Date(q.at).getTime();
  return quotaLiveWindows(q, now).length > 0 && Number.isFinite(at) && at > Date.UTC(2000, 0, 1);
}

/** When the numbers were observed: a runner's shared snapshot may keep a
 *  weekly window's older time on purpose, so where it has a profile the
 *  collection time is the freshness signal; a session report carries the
 *  time it observed. */
function quotaObservedAt(q: Quota): number {
  return new Date(q.profile_ref ? q.checked_at || q.at : q.at).getTime();
}

/** The status to show: a read nobody refreshed for an hour counts as
 *  unknown, never as ok. */
export function quotaEffectiveStatus(q: Quota, now = Date.now()): string {
  if (q.status && q.status !== "ok") return q.status;
  const at = quotaObservedAt(q);
  if (!Number.isFinite(at) || now - at > 3600e3) return "unknown";
  return q.status || "ok";
}

/** The status says the last read failed (or is too old to trust). */
function quotaFailed(q: Quota, now = Date.now()): boolean {
  const status = quotaEffectiveStatus(q, now);
  return !!status && status !== "ok";
}

/** The person has to do something: sign in again, or add a key. */
function quotaNeedsAction(q: Quota): boolean {
  return q.status === "auth_required" || q.status === "no_credentials";
}

/** Why there are no fresh numbers, in a few words: "需要在 CLI 重新登录". */
export function quotaStatusText(q: Quota, now = Date.now()): string {
  switch (quotaEffectiveStatus(q, now)) {
    case "no_credentials": return L("需要配置 key", "Needs an API key");
    case "auth_required": return L("需要在 CLI 重新登录", "Sign in again in the CLI");
    case "unsupported": return q.provider === "unknown" ? L("这个连接没有能读的额度", "This connection has no usage to read") : L("这个账号不报额度", "This account doesn't report usage");
    case "throttled": return L("查得太勤，稍后再取", "Checked too often; tries again later");
    case "schema_changed": return L("额度格式变了，需要更新 myco", "The usage format changed; update myco");
    case "transient_error": return L("获取失败，稍后再试", "Couldn't fetch; tries again later");
    case "unknown": return L("超过一小时没更新", "Not updated in over an hour");
    default:
      if (q.provider === "unknown") return L("这个连接没有能读的额度", "This connection has no usage to read");
      return q.status && q.status !== "ok" ? L("获取失败", "Couldn't fetch") : "";
  }
}

/** A tag's few words for a login or key to fix. */
function actionShort(q: Quota): string {
  return q.status === "no_credentials" ? L("需配置 key", "Needs a key") : L("需重新登录", "Sign in again");
}

/** Usage older than an hour, or kept from before a failed read: the
 *  numbers have moved on. */
export function quotaStaleNote(quota: Quota, now: number): string | undefined {
  if (!quotaHasNumbers(quota, now)) return undefined;
  const at = new Date(quota.at).getTime();
  if (!quotaFailed(quota, now) && now - at <= 3600e3) return undefined;
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
  const p = q.provider === "unknown" ? "" : q.provider || q.source;
  const who = providerNames[p] || p || "";
  const machine = q.machine_id ? machines.find((m) => m.id === q.machine_id)?.name : undefined;
  // (read in no computer: what its session reported, e.g. a cloud session)
  const where = machine
    || (q.source_kind === "statusline" ? L("状态栏上报", "from its status line")
      : q.source_kind === "cloud_session" ? L("会话上报", "reported by its session") : "");
  return [who, q.plan ? planName(q.plan) : "", where].filter(Boolean).join(" · ");
}

/** The tightest window still describing the account: the one with the
 *  least left (spent first). */
export function quotaWorst(q: Quota, now = Date.now()): QuotaWindow | undefined {
  const windows = quotaLiveWindows(q, now);
  if (!windows.length) return undefined;
  return windows.reduce((x, w) => {
    if (quotaSpent(w) !== quotaSpent(x)) return quotaSpent(w) ? w : x;
    return w.used > x.used ? w : x;
  });
}

export type QuotaLevel = "ok" | "tight" | "spent" | "action" | "failed";

/** One line for a member: the tightest window left, whose usage, when it
 *  resets, and what is wrong. `short` fits a tag; `line` is the whole say. */
export function quotaSummary(q: Quota, machines: Machine[] = [], now = Date.now()): { level: QuotaLevel; short: string; line: string; stale: boolean } {
  const src = quotaSourceText(q, machines);
  const status = quotaStatusText(q, now);
  // Numbers nobody refreshed for an hour are unknown, not ok: do not show
  // them as the account's current use.
  const unknown = quotaEffectiveStatus(q, now) === "unknown";
  const worst = !unknown && quotaHasNumbers(q, now) ? quotaWorst(q, now) : undefined;
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
    stale: stale || quotaFailed(q, now),
  };
}

/** Nothing to read here at all (no supported source, or an account that
 *  reports none): lists say nothing; the details say why. */
export function quotaUnreadable(q: Quota): boolean {
  return (q.provider === "unknown" || q.status === "unsupported") && !quotaHasNumbers(q);
}

/** A node with a window nearly (or fully) used, or a login to fix: the
 *  quiet hint other pages show. The worst window decides. */
export function agentQuotaHint(a: Agent, now = Date.now()): { level: "tight" | "spent"; text: string; title: string } | undefined {
  const quota = a.runtime?.quota;
  if (!quota || quotaUnreadable(quota)) return undefined;
  const s = quotaSummary(quota, [], now);
  if (s.level === "ok" || s.level === "failed") return undefined;
  return { level: s.level === "tight" ? "tight" : "spent", text: s.short, title: s.line };
}

/** A window's name in a few characters for the composer: "5h", "7d", "月". */
export function quotaWindowShort(w: QuotaWindow): string {
  const sec = w.duration_sec || (w.name === "five_hour" ? 5 * HOUR : w.name.startsWith("seven_day") || w.name === "weekly" ? 7 * DAY : w.name === "daily" ? DAY : 0);
  if (w.name === "monthly" || (sec >= 28 * DAY && sec <= 31 * DAY)) return L("月", "mo");
  if (!sec) return w.label || "";
  if (sec < 2 * DAY) return `${Math.round(sec / HOUR)}h`;
  return `${Math.round(sec / DAY)}d`;
}

/** The composer's colour for a window: grey, amber under 20% left, red
 *  under 5% or spent. */
export function quotaPillLevel(w: QuotaWindow): "ok" | "low" | "out" {
  if (quotaSpent(w) || quotaLeft(w) < 5) return "out";
  return quotaLeft(w) < 20 ? "low" : "ok";
}

/** "已用完，3:40 重置": what the composer says before a send while the
 *  account is out (it does not stop the send). */
export function quotaOutNote(q: Quota, now = Date.now()): string | undefined {
  if (!quotaHasNumbers(q, now)) return undefined;
  const w = quotaWorst(q, now);
  if (!w || !quotaSpent(w)) return undefined;
  const at = w.resets_at ? resetLabel(w.resets_at, now) : "";
  return at && at !== "—"
    ? L(`${quotaWindowLabel(w)}额度已用完，${at} 重置；现在发的消息可能要等到那时才会处理。`, `${quotaWindowLabel(w)} quota is used up; resets ${at}. What you send now may wait until then.`)
    : L(`${quotaWindowLabel(w)}额度已用完；现在发的消息可能要等重置后才会处理。`, `${quotaWindowLabel(w)} quota is used up. What you send now may wait until it resets.`);
}

