// The booking flow uses real availability, then revalidates on commit. A
// chosen date is in the visitor's zone, never the browser's implicit zone.
import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, Check, ChevronLeft, ChevronRight, Clock3, MapPin } from "lucide-react";
import { t as uiText, locale } from "../_lib/i18n";
import { save } from "../_lib/myco";
import { addDays, calendarFile, dateIn, slotsForDay, type Booking, type Slot } from "./engine";
import { occupied, eventData, hostZone, invalidLedger, kit, profile, schedule, types } from "./model";
import { reserve } from "./edit";
import { ErrorNote, Field, ZoneSelect } from "./panels";

export const timeText = (iso: string, zone: string, offset = false) => new Intl.DateTimeFormat(locale(), { timeZone: zone, hour: "2-digit", minute: "2-digit", hourCycle: "h23", ...(offset ? { timeZoneName: "shortOffset" as const } : {}) }).format(new Date(iso));
export const dateText = (iso: string, zone: string) => new Intl.DateTimeFormat(locale(), { timeZone: zone, year: "numeric", month: "long", day: "numeric", weekday: "short" }).format(new Date(iso));
export async function downloadCalendar(entries: Booking[]) {
  const content = calendarFile(entries), type = "text/calendar;charset=utf-8";
  try { const r = await save("appointments.ics", content, type); kit.flash(uiText("已存到 {0}", r.path)); }
  catch {
    const url = URL.createObjectURL(new Blob([content], { type })), a = document.createElement("a");
    a.href = url; a.download = "appointments.ics"; document.body.append(a); a.click(); a.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 30000); kit.flash(uiText("已开始浏览器下载"));
  }
}
function firstDay(type: string, zone: string) {
  const today = dateIn(Date.now(), zone), event = eventData(types.get(type)?.data || {});
  for (let i = 0; i <= Math.min(event.horizon, 14); i++) { const date = addDays(today, i); if (slotsForDay(date, zone, hostZone(), event, schedule(), occupied(date)).length) return date; }
  return today;
}
export function BookingView({ typeId, close }: { typeId: string; close: () => void }) {
  const [zone, setZone] = useState(hostZone), [day, setDay] = useState(() => firstDay(typeId, hostZone())), [month, setMonth] = useState(() => firstDay(typeId, hostZone()).slice(0, 7));
  const [slot, setSlot] = useState<Slot | null>(null), [result, setResult] = useState<Booking | null>(null), [busy, setBusy] = useState(false), [error, setError] = useState("");
  const [guest, setGuest] = useState({ name: "", email: "", notes: "" }), [now, setNow] = useState(Date.now);
  useEffect(() => { const timer = window.setInterval(() => setNow(Date.now()), 30000); return () => window.clearInterval(timer); }, []);
  const rec = types.get(typeId), event = eventData(rec?.data || {}), host = profile.get("main")?.data;
  const broken = invalidLedger();
  const slots = useMemo(() => broken ? [] : slotsForDay(day, zone, hostZone(), event, schedule(), occupied(day), now), [day, zone, kit.version, now, broken]);
  const start = month + "-01", weekday = new Date(start + "T12:00:00Z").getUTCDay(), gridStart = addDays(start, -((weekday + 6) % 7));
  const today = dateIn(now, zone), lastDay = addDays(today, event.horizon + 1);
  function selectDay(date: string) { setDay(date); setMonth(date.slice(0, 7)); setSlot(null); setError(""); }
  return <main className="sc-booking-wrap"><button className="link-btn sc-back" type="button" onClick={close}><ArrowLeft className="ico" />{uiText("返回预约类型")}</button>
    {!rec ? <div className="sc-empty">{uiText("这个预约类型已不存在。")}</div> : result ? <section className="sc-confirmation"><div className="sc-check-circle"><Check size={24} /></div><h1>{uiText("预约已确认")}</h1><p className="faint">{uiText("时间已经为你留好。可下载日历文件，或在「预约记录」里查看与取消。")}</p><div className="sc-confirm-details"><h2>{result.title}</h2><p>{host?.host}</p><p>{dateText(result.start, zone)}</p><p>{timeText(result.start, zone, true)} – {timeText(result.end, zone)} · {zone}</p>{result.location && <p>{result.location}</p>}<p>{result.name} · {result.email}</p></div><p className="faint">{uiText("不会发送确认邮件；请保存日历文件。")}</p><button type="button" className="btn btn-primary" onClick={() => void downloadCalendar([result])}>{uiText("添加到日历")}</button></section> : <div className="sc-booking">
      <section className="sc-book-info"><div className="sc-avatar">{(host?.host || "M").slice(0, 1)}</div><p className="faint">{host?.host}</p><h1>{event.title}</h1><p className="sc-description">{event.description}</p><p className="sc-meta"><Clock3 className="ico" />{uiText("{0} 分钟", event.duration)}</p>{event.location && <p className="sc-meta"><MapPin className="ico" />{event.location}</p>}<p className="faint sc-intro">{host?.bio}</p><Field label={uiText("显示时区")}><ZoneSelect value={zone} onChange={z => { setZone(z); setSlot(null); setError(""); }} /></Field><p className="sc-small faint">{uiText("只有当前页面的预约会占用时间。")}</p></section>
      <section className="sc-calendar" aria-label={uiText("选择日期")}><header><h2>{new Intl.DateTimeFormat(locale(), { year: "numeric", month: "long", timeZone: "UTC" }).format(new Date(start + "T12:00:00Z"))}</h2><div className="sc-inline"><button type="button" className="icon-btn" title={uiText("上个月")} aria-label={uiText("上个月")} disabled={month <= today.slice(0, 7)} onClick={() => setMonth(addDays(start, -1).slice(0, 7))}><ChevronLeft className="ico" /></button><button type="button" className="icon-btn" title={uiText("下个月")} aria-label={uiText("下个月")} disabled={month >= lastDay.slice(0, 7)} onClick={() => setMonth(addDays(addDays(start, 32).slice(0, 7) + "-01", 0).slice(0, 7))}><ChevronRight className="ico" /></button></div></header>
        <div className="sc-calendar-grid">{Array.from({ length: 7 }, (_, i) => <span className="sc-weekday faint" key={i}>{new Intl.DateTimeFormat(locale(), { weekday: "short", timeZone: "UTC" }).format(new Date(Date.UTC(2026, 10, 23 + i)))}</span>)}{Array.from({ length: 42 }, (_, i) => { const date = addDays(gridStart, i); return <button type="button" key={date} className={`sc-date${date === day ? " is-selected" : ""}${date === today ? " is-today" : ""}`} aria-label={date} aria-pressed={date === day} disabled={date < today || date > lastDay || date.slice(0, 7) !== month} onClick={() => selectDay(date)}>{Number(date.slice(8))}</button>; })}</div>
        <div className="sc-calendar-note"><p className="faint">{uiText("按 15 分钟间隔显示可约时段。")}</p><button className="link-btn" type="button" onClick={() => { for (let i = 1; i <= event.horizon + 2; i++) { const date = addDays(day, i); if (slotsForDay(date, zone, hostZone(), event, schedule(), occupied(date)).length) { selectDay(date); return; } } setError(uiText("预约范围内没有更多时段，可调整可用时间。")); }}>{uiText("找下一个可约日期")}</button></div>
      </section>
      <section className="sc-slot-panel"><h2>{new Intl.DateTimeFormat(locale(), { month: "short", day: "numeric", weekday: "short", timeZone: "UTC" }).format(new Date(day + "T12:00:00Z"))}</h2>{slot ? <form onSubmit={async e => { e.preventDefault(); setError(""); setBusy(true); try { setResult(await reserve(typeId, slot, { ...guest, zone })); } catch (e) { setError((e as Error).message); setNow(Date.now()); } finally { setBusy(false); } }}>
        <button type="button" className="sc-selected-slot" onClick={() => setSlot(null)}>{timeText(slot.start, zone, true)} – {timeText(slot.end, zone)} <span className="faint">{uiText("更换")}</span></button>
        <Field label={uiText("你的姓名")}><input autoFocus className="inp" required maxLength={100} value={guest.name} onChange={e => setGuest({ ...guest, name: e.target.value })} /></Field><Field label={uiText("邮箱")}><input className="inp" type="email" required maxLength={200} value={guest.email} onChange={e => setGuest({ ...guest, email: e.target.value })} /></Field><Field label={uiText("备注（可选）")}><textarea className="inp" rows={3} maxLength={1000} value={guest.notes} onChange={e => setGuest({ ...guest, notes: e.target.value })} /></Field><p className="sc-small faint">{uiText("姓名、邮箱和备注对本页面成员可见。")}</p><button className="btn btn-primary sc-full" disabled={busy || !slots.some(s => s.start === slot.start)}>{busy ? uiText("正在保存…") : uiText("确认预约")}</button>
      </form> : <div className="sc-slots">{slots.map(s => <button type="button" className="btn sc-slot" key={s.start} aria-label={timeText(s.start, zone, true)} onClick={() => { setSlot(s); setError(""); }}>{timeText(s.start, zone)}{slots.filter(x => timeText(x.start, zone) === timeText(s.start, zone)).length > 1 && <small>{timeText(s.start, zone, true).replace(timeText(s.start, zone), "")}</small>}</button>)}{!slots.length && <p className="sc-no-slots faint">{event.enabled ? uiText("这一天没有可约时段，试试其他日期。") : uiText("这个类型已暂停预约。")}</p>}</div>}
      <ErrorNote text={broken ? uiText("预约数据有误，请让 agent 检查后再预约。") : error} /></section>
    </div>}
  </main>;
}
