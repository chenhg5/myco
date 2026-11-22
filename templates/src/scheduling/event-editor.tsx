// One editor for new events and entire recurring series. Drafts keep the read
// version so saving cannot overwrite another person's newer edit.
import { useState } from "react";
import { Trash2 } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { addDays } from "./engine";
import { events, kit } from "./model";
import { instant, wallTime, type CalendarEvent } from "./calendar-engine";
import { deleteCalendarEvent, saveCalendarEvent } from "./calendar-edit";
import { ErrorNote, Field, Modal, ZoneSelect } from "./panels";
export type EditDraft = { id?: string; date: string; minute?: number; allDay?: boolean };
export function EventEditor({ draft, zone, close }: { draft: EditDraft; zone: string; close: () => void }) {
  const [old] = useState(() => draft.id && events.get(draft.id) ? structuredClone(events.get(draft.id)) : null), [version] = useState(() => old?.version || 0);
  const [eventZone, setZone] = useState(old?.data.zone || zone), [allDay, setAllDay] = useState(old?.data.all_day ?? draft.allDay ?? false);
  const minute = draft.minute ?? 9 * 60, dateTime = (m: number) => `${draft.date}T${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`;
  const [start, setStart] = useState(old ? old.data.all_day ? old.data.start : wallTime(old.data.start, old.data.zone) : dateTime(minute));
  const [end, setEnd] = useState(old ? old.data.all_day ? addDays(old.data.end, -1) : wallTime(old.data.end, old.data.zone) : minute >= 1380 ? addDays(draft.date, 1) + "T00:" + String(minute % 60).padStart(2, "0") : dateTime(minute + 60));
  const [value, set] = useState({ title: old?.data.title || "", calendar: old?.data.calendar || "work", location: old?.data.location || "", notes: old?.data.notes || "", busy: old?.data.busy ?? true, repeat: old?.data.repeat || "none", until: old?.data.until || "", owner: old?.data.owner || kit.me });
  const [error, setError] = useState(""), [busy, setBusy] = useState(false);
  const run = async (f: () => Promise<void>) => { setBusy(true); setError(""); try { await f(); close(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } };
  return <Modal title={old ? uiText("编辑日程") : uiText("新建日程")} close={() => { if (!busy) close(); }}><form onSubmit={e => { e.preventDefault(); void run(async () => {
    const a = allDay ? start.slice(0, 10) : instant(start, eventZone, old?.data.start), b = allDay ? addDays(end.slice(0, 10), 1) : instant(end, eventZone, old?.data.end);
    if (!a || !b) throw new Error(uiText("这个本地时间不存在，请选择其他时间。"));
    const data: CalendarEvent = { ...value, start: a, end: b, all_day: allDay, zone: eventZone, until: value.repeat === "none" ? "" : value.until };
    await saveCalendarEvent(old?.id || "", data, version);
  }); }}>
    <Field label={uiText("日程标题")}><input className="inp" autoFocus required maxLength={150} value={value.title} placeholder={uiText("添加标题")} onChange={e => set({ ...value, title: e.target.value })} /></Field>
    <label className="sc-check"><input type="checkbox" checked={allDay} onChange={e => { const checked = e.target.checked; setAllDay(checked); setStart(checked ? start.slice(0, 10) : start.slice(0, 10) + "T09:00"); setEnd(checked ? end.slice(0, 10) : end.slice(0, 10) + "T10:00"); }} />{uiText("全天")}</label>
    <div className="sc-form-grid"><Field label={uiText("开始")}><input className="inp" type={allDay ? "date" : "datetime-local"} required value={allDay ? start.slice(0, 10) : start} onChange={e => setStart(e.target.value)} /></Field><Field label={uiText("结束")}><input className="inp" type={allDay ? "date" : "datetime-local"} required value={allDay ? end.slice(0, 10) : end} onChange={e => setEnd(e.target.value)} /></Field></div>
    {!allDay && <Field label={uiText("日程时区")}><ZoneSelect value={eventZone} onChange={z => { setZone(z); }} /></Field>}
    <div className="sc-form-grid"><Field label={uiText("所属日历")}><select className="sel" aria-label={uiText("所属日历")} value={value.calendar} onChange={e => set({ ...value, calendar: e.target.value as CalendarEvent["calendar"] })}><option value="work">{uiText("工作")}</option><option value="personal">{uiText("生活")}</option></select></Field><Field label={uiText("重复")}><select className="sel" aria-label={uiText("重复")} value={value.repeat} onChange={e => set({ ...value, repeat: e.target.value as CalendarEvent["repeat"] })}>{[["none",uiText("不重复")],["daily",uiText("每天")],["weekly",uiText("每周")],["monthly",uiText("每月")]].map(([v,t]) => <option key={v} value={v}>{t}</option>)}</select></Field></div>
    {value.repeat !== "none" && <><Field label={uiText("重复截止日期（可选）")}><input className="inp" type="date" value={value.until} onChange={e => set({ ...value, until: e.target.value })} /></Field><p className="faint">{uiText("修改、移动或删除会应用到整组重复日程。")}</p></>}
    <Field label={uiText("地点")}><input className="inp" maxLength={300} value={value.location} onChange={e => set({ ...value, location: e.target.value })} /></Field>
    <Field label={uiText("备注")}><textarea className="inp" rows={3} maxLength={8000} value={value.notes} onChange={e => set({ ...value, notes: e.target.value })} /></Field>
    <label className="sc-check"><input type="checkbox" checked={value.busy} onChange={e => set({ ...value, busy: e.target.checked })} />{uiText("显示为忙碌，预约时避开")}</label>
    <ErrorNote text={error} /><footer>{old && <button type="button" className="btn sc-danger" disabled={busy} onClick={() => void run(() => deleteCalendarEvent(old.id, version))}><Trash2 className="ico" />{uiText("删除")}</button>}<span className="top-gap" /><button type="button" className="btn" disabled={busy} onClick={close}>{uiText("取消")}</button><button className="btn btn-primary" disabled={busy}>{busy ? uiText("正在保存…") : uiText("保存")}</button></footer>
  </form></Modal>;
}
