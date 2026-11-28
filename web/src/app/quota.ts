import type { Agent, Machine, Quota, QuotaWindow } from "@/api";
import { L, E, locale } from "@/lib/i18n";
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
    return L(E`${h} 小时`, E`${h} hours`);
  }
  const d = Math.round(sec / DAY);
  return L(E`${d} 天`, E`${d} days`);
}

/** A usage window's name for people: by its length when the source says
 *  it, else by the CLI's own window names; a model's bucket adds the model. */
export function quotaWindowLabel(w: QuotaWindow): string {
  if (w.balance) return L("余额", "Balance");
  if (w.name === "monthly_mcp") return `${L("每月", "Monthly")} · MCP`;
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
  else if (/^limit_\d+$/.test(w.name)) { const n = Number(w.name.slice(6)) + 1; base = L(E`额度 ${n}`, E`Limit ${n}`); }
  else base = w.label || w.name;
  const scope = w.model_scope || (w.name === "seven_day_opus" ? "opus" : w.name === "seven_day_sonnet" ? "sonnet" : "");
  return scope ? L(E`${base}（${scopeName(scope)}）`, E`${base} (${scopeName(scope)})`) : base;
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
  if (w.balance?.unlimited) return false;
  return (!!w.status && !["allowed", "allowed_warning", "warning"].includes(w.status)) || w.used >= 1;
}

/** Warn at the provider's threshold, 90% used, or when spent. */
export function quotaTight(w: QuotaWindow): boolean {
  return quotaSpent(w) || w.status === "warning" || w.status === "allowed_warning" || w.used >= 0.9;
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
    return d.toLocaleDateString(locale(), { weekday: "short" });
  }
  return d.toLocaleDateString(locale(), { month: "short", day: "numeric" });
}

/** "3 小时后重置": how long until a window resets; "" when the source
 *  does not say. */
export function resetIn(iso: string | undefined, now = Date.now()): string {
  const t = iso ? new Date(iso).getTime() : NaN;
  if (!Number.isFinite(t)) return "";
  const s = (t - now) / 1000;
  if (s <= 60) return L("马上重置", "resets now");
  if (s < HOUR) { const m = Math.ceil(s / 60); return L(E`${m} 分钟后重置`, E`resets in ${m} min`); }
  if (s < 2 * DAY) { const h = Math.round(s / HOUR); return L(E`${h} 小时后重置`, E`resets in ${h} h`); }
  const d = Math.round(s / DAY);
  return L(E`${d} 天后重置`, E`resets in ${d} days`);
}

/** A row's value: "剩 38% · 3 小时后重置", or what a spent window says. */
export function quotaWindowText(w: QuotaWindow, now = Date.now()): string {
  if (w.balance) {
    if (w.balance.unlimited) return L("不限额", "Unlimited");
    return `${usd(w.balance.remaining)} / ${usd(w.balance.limit)}`;
  }
  const reset = resetIn(w.resets_at, now);
  if (quotaSpent(w)) return [L("额度用完", "Out of quota"), reset].filter(Boolean).join(" · ");
  return [L(E`剩 ${quotaLeft(w)}%`, E`${quotaLeft(w)}% left`), quotaTight(w) ? L("接近上限", "Near limit") : "", reset].filter(Boolean).join(" · ");
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

/** When the numbers were observed, as the timestamp the App shows: a
 *  runner's shared snapshot may keep a weekly window's older time on
 *  purpose, so where it has a profile the collection time is the freshness
 *  signal; a session report carries the time it observed. */
export function quotaObservedAt(q: Quota): string {
  return q.profile_ref ? q.checked_at || q.at : q.at;
}

/** The status to show: a read nobody refreshed for an hour counts as
 *  unknown, never as ok. */
export function quotaEffectiveStatus(q: Quota, now = Date.now()): string {
  if (q.status && q.status !== "ok") return q.status;
  const at = new Date(quotaObservedAt(q)).getTime();
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
  return L(E`${timeAgo(quota.at, now)}取到的数据，可能已过时`, E`data from ${timeAgo(quota.at, now)}, may be out of date`);
}

/** Last known numbers whose read is no longer current: when they were
 *  read, and that the account may have moved on since. */
export function quotaOldNote(q: Quota, now = Date.now()): string | undefined {
  if (quotaEffectiveStatus(q, now) !== "unknown" || !quotaHasNumbers(q, now)) return undefined;
  return L(E`上次更新：${timeAgo(quotaObservedAt(q), now)}，可能已过时`, E`last updated ${timeAgo(quotaObservedAt(q), now)}; may be out of date`);
}

const providerNames: Record<string, string> = { glm: "GLM", kimi: "Kimi", openrouter: "OpenRouter", codex: "Codex", claude: "Claude", opencode_go: "OpenCode Go", minimax: "MiniMax" };

function planName(plan: string): string {
  switch (plan) {
    case "chatgpt_subscription": return L("ChatGPT 订阅", "ChatGPT plan");
    case "claude_subscription": return L("Claude 订阅", "Claude plan");
    case "go": return L("Go 套餐", "Go plan");
    case "coding_plan": return "Coding Plan";
    case "api_balance": return L("API 额度", "API allowance");
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

export type QuotaLevel = "ok" | "tight" | "spent" | "action" | "failed" | "old";

/** One line for a member: the tightest window left, whose usage, when it
 *  resets, and what is wrong. `short` fits a tag; `line` is the whole say. */
export function quotaSummary(q: Quota, machines: Machine[] = [], now = Date.now()): { level: QuotaLevel; short: string; line: string; stale: boolean } {
  const src = quotaSourceText(q, machines);
  const status = quotaStatusText(q, now);
  const has = quotaHasNumbers(q, now);
  // Numbers nobody refreshed for an hour are no longer current, but they are
  // still the last thing anyone knows: show them greyed, with their age,
  // never as today's use and never as ok.
  const old = has && quotaEffectiveStatus(q, now) === "unknown";
  const worst = has ? quotaWorst(q, now) : undefined;
  const stale = !!quotaStaleNote(q, now);
  if (!worst) {
    const why = status || L("还没有额度数据", "No usage yet");
    return { level: quotaNeedsAction(q) ? "action" : "failed", short: quotaNeedsAction(q) ? actionShort(q) : L("额度未知", "Usage unknown"), line: L(E`额度：${why}（${src}）`, E`Usage: ${why} (${src})`), stale: false };
  }
  const reset = resetIn(worst.resets_at, now);
  const what = worst.balance ? quotaRemainingText(worst) : quotaSpent(worst) ? L("用完了", "out") : quotaRemainingText(worst);
  const label = quotaWindowLabel(worst);
  let line = L((reset ? E`额度：${label}${what}（${src}）${"，" + reset}` : E`额度：${label}${what}（${src}）`), (reset ? E`Usage: ${label} ${what} (${src})${"; " + reset}` : E`Usage: ${label} ${what} (${src})`));
  if (quotaTight(worst) && !quotaSpent(worst)) line += " · " + L("接近上限", "Near limit");
  if (old) line += L(E`。${quotaOldNote(q, now)}`, E`. ${quotaOldNote(q, now)}`);
  else if (status) line += L(E`。${status}，${quotaStaleNote(q, now)}`, E`. ${status}; ${quotaStaleNote(q, now)}`);
  else if (stale) line += L(E`（${quotaStaleNote(q, now)}）`, E` (${quotaStaleNote(q, now)})`);
  const shared = quotaSharedNote(q, now);
  if (shared) line += ` · ${shared}`;
  const level: QuotaLevel = quotaNeedsAction(q) ? "action" : old ? "old" : quotaSpent(worst) ? "spent" : quotaTight(worst) ? "tight" : "ok";
  const age = old ? timeAgo(quotaObservedAt(q), now) : "";
  return {
    level,
    short: quotaNeedsAction(q) ? actionShort(q) : quotaSpent(worst) ? L(E`额度用完${age ? " · " + age : ""}`, E`Out of quota${age ? " · " + age : ""}`) : quotaRemainingText(worst) + (age ? " · " + age : ""),
    line,
    stale: stale || old || quotaFailed(q, now),
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
  // Only a current warning or something to do (sign in again, add a key):
  // old numbers are not a reason to act.
  if (s.level !== "tight" && s.level !== "spent" && s.level !== "action") return undefined;
  return { level: s.level === "tight" ? "tight" : "spent", text: s.short, title: s.line };
}

/** A window's name in a few characters for the composer: "5h", "7d", "月". */
export function quotaWindowShort(w: QuotaWindow): string {
  if (w.balance) return "";
  const sec = w.duration_sec || (w.name === "five_hour" ? 5 * HOUR : w.name.startsWith("seven_day") || w.name === "weekly" ? 7 * DAY : w.name === "daily" ? DAY : 0);
  if (w.name === "monthly" || w.name === "monthly_mcp" || (sec >= 28 * DAY && sec <= 31 * DAY)) return L("月", "mo");
  if (!sec) return w.label || "";
  if (sec < 2 * DAY) return `${Math.round(sec / HOUR)}h`;
  return `${Math.round(sec / DAY)}d`;
}

/** The composer's colour for a window: grey, amber under 20% left, red
 *  under 5% or spent. */
export function quotaPillLevel(w: QuotaWindow): "ok" | "low" | "out" {
  if (quotaSpent(w) || quotaLeft(w) < 5) return "out";
  return quotaTight(w) || quotaLeft(w) < 20 ? "low" : "ok";
}

/** "已用完，3:40 重置": what the composer says before a send while the
 *  account is out (it does not stop the send). */
export function quotaOutNote(q: Quota, now = Date.now()): string | undefined {
  if (!quotaHasNumbers(q, now)) return undefined;
  const w = quotaWorst(q, now);
  if (!w || !quotaSpent(w)) return undefined;
  if (w.balance) return L("API 额度已用完；请检查 key 的限额或充值。", "API allowance is used up; check the key limit or add credit.");
  const at = w.resets_at ? resetLabel(w.resets_at, now) : "";
  return at && at !== "—"
    ? L(E`${quotaWindowLabel(w)}额度已用完，${at} 重置；现在发的消息可能要等到那时才会处理。`, E`${quotaWindowLabel(w)} quota is used up; resets ${at}. What you send now may wait until then.`)
    : L(E`${quotaWindowLabel(w)}额度已用完；现在发的消息可能要等重置后才会处理。`, E`${quotaWindowLabel(w)} quota is used up. What you send now may wait until it resets.`);
}


function usd(value: number | undefined): string {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(value ?? 0);
}
/** A compact value shared by member tags and the composer. */
export function quotaRemainingText(w: QuotaWindow): string {
  if (w.balance?.unlimited) return L("不限额", "Unlimited");
  if (w.balance) return L(E`剩 ${usd(w.balance.remaining)}`, E`${usd(w.balance.remaining)} left`);
  return L(E`剩 ${quotaLeft(w)}%`, E`${quotaLeft(w)}% left`);
}

export function quotaSharedNote(q: Quota, now = Date.now()): string | undefined {
  if (!q.shared) return undefined;
  const age = timeAgo(q.shared_at || q.at, now);
  return L(E`同账号的另一个 agent ${age}上报`, E`Reported by another agent on the same account ${age}`);
}
