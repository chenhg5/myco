// Dates as the pages keep them: "2026-11-18", in the viewer's own day.
export const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
export const today = () => ymd(new Date());
export const addDays = (s: string, n: number) => { const d = new Date(`${s}T12:00:00`); d.setDate(d.getDate() + n); return ymd(d); };
export const WEEK = ["日", "一", "二", "三", "四", "五", "六"];
/** 今天, 明天, 昨天, 周三, 11月20日, 2027年1月3日 */
export function dayName(s: string) {
  const t = today();
  if (s === t) return "今天";
  if (s === addDays(t, 1)) return "明天";
  if (s === addDays(t, -1)) return "昨天";
  const d = new Date(`${s}T12:00:00`), n = new Date();
  const diff = Math.round((d.getTime() - new Date(`${t}T12:00:00`).getTime()) / 86400000);
  if (diff > 1 && diff < 7) return `周${WEEK[d.getDay()]}`;
  return d.getFullYear() === n.getFullYear() ? `${d.getMonth() + 1}月${d.getDate()}日` : `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`;
}
