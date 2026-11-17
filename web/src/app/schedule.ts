import { L } from "@/lib/i18n";

/**
 * Schedules as people say them, and the cron the hub keeps. The editor
 * offers the common ones (every day, weekdays, one day a week, every few
 * hours) and a cron of your own; a cron that is one of them reads in words.
 */
export type Every =
  | { kind: "daily"; hour: number; minute: number }
  | { kind: "weekdays"; hour: number; minute: number }
  | { kind: "weekly"; day: number; hour: number; minute: number } // day: 0 = Sunday
  | { kind: "hours"; every: number; minute: number } // every N hours, at :minute
  | { kind: "cron"; cron: string };

export function toCron(e: Every): string {
  switch (e.kind) {
    case "daily": return `${e.minute} ${e.hour} * * *`;
    case "weekdays": return `${e.minute} ${e.hour} * * 1-5`;
    case "weekly": return `${e.minute} ${e.hour} * * ${e.day}`;
    case "hours": return `${e.minute} ${e.every === 1 ? "*" : `*/${e.every}`} * * *`;
    case "cron": return e.cron.trim().split(/\s+/).join(" ");
  }
}

const num = (s: string, lo: number, hi: number) => /^\d+$/.test(s) && +s >= lo && +s <= hi;

/** The preset a cron is, if it is one (else a cron of its own). */
export function fromCron(cron: string): Every {
  const f = cron.trim().split(/\s+/);
  if (f.length === 5 && num(f[0], 0, 59) && f[2] === "*" && f[3] === "*") {
    const minute = +f[0];
    if (num(f[1], 0, 23)) {
      const hour = +f[1];
      if (f[4] === "*") return { kind: "daily", hour, minute };
      if (f[4] === "1-5") return { kind: "weekdays", hour, minute };
      if (num(f[4], 0, 7)) return { kind: "weekly", day: +f[4] % 7, hour, minute };
    }
    if (f[4] === "*" && (f[1] === "*" || /^\*\/\d+$/.test(f[1]))) {
      const every = f[1] === "*" ? 1 : +f[1].slice(2);
      if (every >= 1 && every <= 12) return { kind: "hours", every, minute };
    }
  }
  return { kind: "cron", cron: cron.trim() };
}

const DAYS_ZH = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];
const DAYS_EN = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
const hm = (h: number, m: number) => `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;

/** A schedule in words: "每天 09:00", "工作日 18:30", "每 2 小时（:15）", or the cron. */
export function describe(cron: string): string {
  const e = fromCron(cron);
  switch (e.kind) {
    case "daily": return L(`每天 ${hm(e.hour, e.minute)}`, `Every day at ${hm(e.hour, e.minute)}`);
    case "weekdays": return L(`工作日 ${hm(e.hour, e.minute)}`, `Weekdays at ${hm(e.hour, e.minute)}`);
    case "weekly": return L(`每${DAYS_ZH[e.day]} ${hm(e.hour, e.minute)}`, `Every ${DAYS_EN[e.day]} at ${hm(e.hour, e.minute)}`);
    case "hours": return e.every === 1
      ? L(`每小时（第 ${e.minute} 分）`, `Every hour at :${String(e.minute).padStart(2, "0")}`)
      : L(`每 ${e.every} 小时（第 ${e.minute} 分）`, `Every ${e.every} hours at :${String(e.minute).padStart(2, "0")}`);
    case "cron": return `cron ${e.cron}`;
  }
}

/** This computer's time zone (the default for a new schedule). */
export function localZone(): string {
  try { return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"; } catch { return "UTC"; }
}

export const dayNames = () => (L("zh", "en") === "zh" ? DAYS_ZH : DAYS_EN);

/**
 * When a schedule next sends, on this computer's clock (a schedule in this
 * computer's time zone; else null, as for a cron of its own).
 */
export function nextRun(e: Every, from: Date): Date | null {
  if (e.kind === "cron") return null;
  const t = new Date(from.getTime());
  t.setSeconds(0, 0);
  for (let i = 0; i < 8 * 24 * 60; i += 1) {
    t.setMinutes(t.getMinutes() + 1);
    const h = t.getHours(), m = t.getMinutes(), d = t.getDay();
    switch (e.kind) {
      case "daily": if (h === e.hour && m === e.minute) return t; break;
      case "weekdays": if (d >= 1 && d <= 5 && h === e.hour && m === e.minute) return t; break;
      case "weekly": if (d === e.day && h === e.hour && m === e.minute) return t; break;
      case "hours": if (m === e.minute && h % e.every === 0) return t; break;
    }
  }
  return null;
}

/** A moment soon, as people say it: 今天 16:40, 明天 09:00, 周五 09:00, 12月3日 09:00. */
export function soonLabel(at: Date, now: Date): string {
  const time = hm(at.getHours(), at.getMinutes());
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const days = Math.round((day(at) - day(now)) / 86400e3);
  if (days === 0) return L(`今天 ${time}`, `today ${time}`);
  if (days === 1) return L(`明天 ${time}`, `tomorrow ${time}`);
  if (days === -1) return L(`昨天 ${time}`, `yesterday ${time}`);
  if (days > 1 && days < 7) return L(`${DAYS_ZH[at.getDay()]} ${time}`, `${DAYS_EN[at.getDay()].slice(0, 3)} ${time}`);
  return L(`${at.getMonth() + 1}月${at.getDate()}日 ${time}`, `${at.toLocaleDateString("en", { month: "short", day: "numeric" })} ${time}`);
}

/** The short day names for a row of chips (周一 first, as a week is read here). */
export const weekChips = () => [1, 2, 3, 4, 5, 6, 0].map((d) => ({ day: d, label: L(DAYS_ZH[d].slice(1), DAYS_EN[d].slice(0, 3)) }));
