// Personal calendar dates, recurrence and overlap layout are pure calculations.
// Timed events store UTC instants; all-day events store exclusive date ranges.
import { addDays, dateIn, validDate, validZone, wallInstants, type Booking } from "./engine";
export type CalendarEvent = { title: string; start: string; end: string; all_day: boolean; zone: string; calendar: "work" | "personal"; location: string; notes: string; busy: boolean; repeat: "none" | "daily" | "weekly" | "monthly"; until: string; owner: string };
export type EventRecord = { id: string; data: CalendarEvent; version: number };
export type Occurrence = { key: string; id: string; title: string; start: string; end: string; allDay: boolean; calendar: string; event?: EventRecord; booking?: Booking; };
export const dayDiff = (a: string, b: string) => Math.round((Date.parse(b) - Date.parse(a)) / 86400000);
export function monthShift(date: string, n: number) { const d = new Date(date + "T12:00:00Z"); d.setUTCDate(1); d.setUTCMonth(d.getUTCMonth() + n); return d.toISOString().slice(0, 10); }
export function weekStart(date: string) { const n = new Date(date + "T12:00:00Z").getUTCDay(); return addDays(date, -((n + 6) % 7)); }
export function wallTime(iso: string, zone: string) {
  const p = Object.fromEntries(new Intl.DateTimeFormat("en-CA", { timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).formatToParts(new Date(iso)).map(x => [x.type, x.value]));
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}`;
}
export function instant(local: string, zone: string, preferred?: string) {
  const [date, time] = local.split("T"), [hour, minute] = (time || "").split(":").map(Number);
  if (!validDate(date) || !validZone(zone) || !Number.isInteger(hour) || hour < 0 || hour > 23 || !Number.isInteger(minute) || minute < 0 || minute > 59) return null;
  const values = wallInstants(date, hour * 60 + minute, zone);
  // Preserve an unchanged instant when editing the second occurrence of a
  // repeated DST time. New ambiguous times consistently choose the first.
  const ms = preferred && values.includes(Date.parse(preferred)) ? Date.parse(preferred) : values[0];
  return ms === undefined ? null : new Date(ms).toISOString();
}
export function validCalendarEvent(e: CalendarEvent) {
  if (!e || typeof e.title !== "string" || !e.title.trim() || e.title.length > 150 || typeof e.notes !== "string" || e.notes.length > 8000 || typeof e.location !== "string" || e.location.length > 300 || !validZone(e.zone) || !["work", "personal"].includes(e.calendar) || !["none", "daily", "weekly", "monthly"].includes(e.repeat) || typeof e.busy !== "boolean" || typeof e.all_day !== "boolean" || typeof e.owner !== "string") return false;
  const valid = e.all_day ? validDate(e.start) && validDate(e.end) && e.end > e.start && dayDiff(e.start, e.end) <= 31 : /^\d{4}-\d\d-\d\dT.*Z$/.test(e.start) && /^\d{4}-\d\d-\d\dT.*Z$/.test(e.end) && Number.isFinite(Date.parse(e.start)) && Number.isFinite(Date.parse(e.end)) && Date.parse(e.end) > Date.parse(e.start) && Date.parse(e.end) - Date.parse(e.start) <= 31 * 86400000;
  if (!valid) return false;
  return !e.until || validDate(e.until) && e.until >= (e.all_day ? e.start : dateIn(Date.parse(e.start), e.zone));
}
function occurs(base: string, day: string, repeat: CalendarEvent["repeat"]) {
  return day >= base && (repeat === "daily" || repeat === "weekly" && dayDiff(base, day) % 7 === 0 || repeat === "monthly" && day.slice(8) === base.slice(8));
}
export function expandEvents(records: EventRecord[], from: string, to: string, zone: string): Occurrence[] {
  const out: Occurrence[] = [];
  for (const r of records) {
    const e = r.data; if (!validCalendarEvent(e)) continue;
    const base = e.all_day ? e.start : dateIn(Date.parse(e.start), e.zone), duration = e.all_day ? dayDiff(e.start, e.end) : Date.parse(e.end) - Date.parse(e.start);
    const append = (start: string, end: string) => {
      const startDate = e.all_day ? start : dateIn(Date.parse(start), zone), lastDate = e.all_day ? addDays(end, -1) : dateIn(Date.parse(end) - 1, zone);
      if (startDate < to && lastDate >= from) out.push({ key: `${r.id}@${start}`, id: r.id, title: e.title, start, end, allDay: e.all_day, calendar: e.calendar, event: r });
    };
    if (e.repeat === "none") { append(e.start, e.end); continue; }
    // Bounded to the visible range, even for a years-old daily series.
    const back = e.all_day ? duration : Math.ceil(duration / 86400000), lower = addDays(from, -back - 2), upper = addDays(to, 2);
    const start = base > lower ? base : lower, time = e.all_day ? "" : wallTime(e.start, e.zone).slice(11);
    for (let day = start; day < upper && (!e.until || day <= e.until); day = addDays(day, 1)) {
      if (!occurs(base, day, e.repeat)) continue;
      if (e.all_day) append(day, addDays(day, duration));
      else { const at = instant(`${day}T${time}`, e.zone, day === base ? e.start : undefined); if (at) append(at, new Date(Date.parse(at) + duration).toISOString()); }
    }
  }
  return out.sort((a, b) => a.start.localeCompare(b.start) || a.title.localeCompare(b.title));
}
export function onDate(o: Occurrence, date: string, zone: string) {
  return o.allDay ? o.start <= date && o.end > date : dateIn(Date.parse(o.start), zone) <= date && dateIn(Date.parse(o.end) - 1, zone) >= date;
}
export function bookingOccurrence(b: Booking): Occurrence { return { key: `booking:${b.id}`, id: b.id, title: b.title, start: b.start, end: b.end, allDay: false, calendar: "booking", booking: b }; }
function midnight(date: string, zone: string) {
  // A few zones skip midnight when DST changes; use their first real time.
  for (let m = 0; m <= 180; m += 15) { const at = wallInstants(date, m, zone)[0]; if (at !== undefined) return new Date(at).toISOString(); }
  return new Date(Date.parse(date + "T12:00:00Z")).toISOString();
}
export function asBooking(o: Occurrence): Booking & { allDay?: boolean } {
  if (o.booking) return o.booking;
  const e = o.event!.data;
  return { id: e.repeat === "none" ? o.id : o.key, type: "calendar", title: e.title, start: o.start, end: o.end, name: e.owner, email: "", notes: e.notes, location: e.location, zone: e.zone, status: "confirmed", before: 0, after: 0, created: "", allDay: e.all_day };
}
export function busyEvents(records: EventRecord[], from: string, to: string, zone: string): Booking[] {
  return expandEvents(records.filter(r => r.data.busy), from, to, zone).map(o => {
    const b = asBooking(o);
    return o.allDay ? { ...b, start: midnight(o.start, o.event!.data.zone), end: midnight(o.end, o.event!.data.zone) } : b;
  });
}
/** Position simultaneous events in columns, keeping each overlap group stable. */
export function timeLayout(items: Occurrence[], date: string, zone: string) {
  const boxes = items.filter(o => !o.allDay && onDate(o, date, zone)).map(o => {
    const start = wallTime(o.start, zone), end = wallTime(o.end, zone);
    const minute = (s: string) => +s.slice(11, 13) * 60 + +s.slice(14, 16);
    const top = start.slice(0, 10) < date ? 0 : minute(start), bottom = end.slice(0, 10) > date ? 1440 : minute(end);
    return { item: o, top, bottom: Math.max(top + 20, bottom), column: 0, columns: 1 };
  }).sort((a, b) => a.top - b.top || b.bottom - a.bottom);
  let group: typeof boxes = [], ends: number[] = [];
  const finish = () => { for (const b of group) b.columns = ends.length; group = []; ends = []; };
  for (const b of boxes) {
    if (group.length && ends.every(end => end <= b.top)) finish();
    let col = ends.findIndex(end => end <= b.top); if (col < 0) col = ends.length;
    b.column = col; ends[col] = b.bottom; group.push(b);
  }
  finish(); return boxes;
}
