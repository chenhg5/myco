// Calendar edits use exact record versions and keep a local reversible history.
// A repeated event is one record: edits and moves apply to the entire series.
import { t as uiText } from "../_lib/i18n";
import { myco, type MycoRecord } from "../_lib/myco";
import { uid } from "../_lib/appkit";
import { events, kit } from "./model";
import { addDays } from "./engine";
import { dayDiff, instant, validCalendarEvent, wallTime, type CalendarEvent } from "./calendar-engine";
type Step = { id: string; before: CalendarEvent | null; after: CalendarEvent | null; version: number };
const undos: Step[] = [], redos: Step[] = [];
let busy = false;
async function commit(id: string, value: CalendarEvent | null, version: number) {
  if (busy) throw new Error(uiText("请等上一项日程保存完成。"));
  busy = true;
  try {
    const result = await kit.track<MycoRecord | boolean>(value ? myco!.data.put("events", value, { id, ifVersion: version }) : myco!.data.remove("events", id, { ifVersion: version }));
    if (!result) { await events.load().catch(() => {}); kit.emit(); throw new Error(uiText("日程保存未确认，请查看最新内容后再试。")); }
    const current = events.get(id);
    if (!value || typeof result === "object" && (!current || current.version <= result.version)) {
      events.rows = events.rows.filter(r => r.id !== id);
      if (value && typeof result === "object") events.rows.push({ ...result, data: value });
    }
    kit.emit(); return typeof result === "object" ? result.version : 0;
  } finally { busy = false; }
}
export const canUndoEvent = () => undos.length > 0;
export const canRedoEvent = () => redos.length > 0;
export async function saveCalendarEvent(id: string, value: CalendarEvent, version: number) {
  if (!validCalendarEvent(value)) throw new Error(uiText("检查标题和起止时间；单项日程最长 31 天。"));
  const key = id || uid("event"), before = id ? events.get(id)?.data || null : null;
  const next = await commit(key, { ...before, ...value, title: value.title.trim() }, version);
  undos.push({ id: key, before: before ? { ...before } : null, after: { ...value, title: value.title.trim() }, version: next });
  if (undos.length > 60) undos.shift(); redos.length = 0;
  kit.flash(uiText("日程已保存"), () => { void undoEvent().catch(e => kit.flash(e.message)); });
}
export async function deleteCalendarEvent(id: string, version: number) {
  const before = events.get(id)?.data; if (!before) return;
  const next = await commit(id, null, version);
  undos.push({ id, before: { ...before }, after: null, version: next }); redos.length = 0;
  kit.flash(uiText("已删除日程"), () => { void undoEvent().catch(e => kit.flash(e.message)); });
}
export async function undoEvent() { const step = undos.at(-1); if (!step) return; const next = await commit(step.id, step.before, step.version); undos.pop(); redos.push({ ...step, version: next }); kit.flash(uiText("已撤销")); }
export async function redoEvent() { const step = redos.at(-1); if (!step) return; const next = await commit(step.id, step.after, step.version); redos.pop(); undos.push({ ...step, version: next }); kit.flash(uiText("已重做")); }
export async function moveCalendarEvent(id: string, version: number, from: string, to: string, zone: string, minute?: number) {
  const rec = events.get(id); if (!rec || rec.version !== version) throw new Error(uiText("这项日程刚被修改，请重新拖动。"));
  const data = { ...rec.data }, shift = dayDiff(from, to);
  if (data.all_day) { data.start = addDays(data.start, shift); data.end = addDays(data.end, shift); }
  else {
    const local = wallTime(data.start, zone), date = addDays(local.slice(0, 10), shift), time = minute === undefined ? local.slice(11) : `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(minute % 60).padStart(2, "0")}`;
    const start = instant(`${date}T${time}`, zone, data.start);
    if (!start) throw new Error(uiText("这个本地时间不存在，请选择其他时间。"));
    data.end = new Date(Date.parse(start) + Date.parse(data.end) - Date.parse(data.start)).toISOString(); data.start = start;
  }
  if (data.until) data.until = addDays(data.until, shift);
  await saveCalendarEvent(id, data, version);
}
