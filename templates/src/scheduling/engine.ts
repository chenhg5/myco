// Calendar calculations are independent of the bridge and React. Wall-clock
// availability belongs to the host's zone; reservations keep UTC instants.
export type Range = { start: string; end: string };
export type Schedule = { week: Record<string, Range[]>; overrides: { date: string; ranges: Range[] }[] };
export type EventType = { title: string; description: string; duration: number; location: string; notice: number; horizon: number; before: number; after: number; enabled: boolean };
export type Booking = { id: string; type: string; title: string; start: string; end: string; name: string; email: string; notes: string; location: string; zone: string; status: "confirmed" | "cancelled"; before: number; after: number; created: string };
export type Slot = { start: string; end: string; day: string };
export const defaultWeek: Schedule["week"] = Object.fromEntries(Array.from({ length: 7 }, (_, d) => [d, d > 0 && d < 6 ? [{ start: "09:00", end: "12:00" }, { start: "13:00", end: "17:00" }] : []]));
export const defaultEvent: EventType = { title: "", description: "", duration: 30, location: "", notice: 120, horizon: 30, before: 0, after: 0, enabled: true };
const formatters = new Map<string, Intl.DateTimeFormat>();
export function validZone(zone: string) { try { new Intl.DateTimeFormat("en", { timeZone: zone }).format(); return true; } catch { return false; } }
function parts(ms: number, zone: string) {
  if (!formatters.has(zone)) formatters.set(zone, new Intl.DateTimeFormat("en-CA", { timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }));
  const p = Object.fromEntries(formatters.get(zone)!.formatToParts(ms).map(x => [x.type, x.value]));
  return { date: `${p.year}-${p.month}-${p.day}`, minute: +p.hour * 60 + +p.minute };
}
export const dateIn = (ms: number, zone: string) => parts(ms, zone).date;
export function addDays(date: string, n: number) { return new Date(Date.parse(date + "T12:00:00Z") + n * 86400000).toISOString().slice(0, 10); }
export const clock = (m: number) => `${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`;
export function minutes(v: string) { if (typeof v !== "string" || !/^\d{2}:\d{2}$/.test(v)) return NaN; const [h, m] = v.split(":").map(Number); return h < 24 && m < 60 ? h * 60 + m : NaN; }
export function validDate(date: string) { return /^\d{4}-\d\d-\d\d$/.test(date) && !isNaN(Date.parse(date)) && new Date(date + "T12:00:00Z").toISOString().slice(0, 10) === date; }
export function validRanges(ranges: Range[]) {
  if (!Array.isArray(ranges) || ranges.length > 8) return false;
  if (ranges.some(r => !r || typeof r !== "object")) return false;
  const sorted = [...ranges].sort((a, b) => minutes(a.start) - minutes(b.start));
  return sorted.every((r, i) => Number.isFinite(minutes(r.start)) && Number.isFinite(minutes(r.end)) && minutes(r.start) < minutes(r.end) && (i === 0 || minutes(sorted[i - 1].end) <= minutes(r.start)));
}
export function validSchedule(s: Schedule) { return !!s && !!s.week && typeof s.week === "object" && Object.keys(s.week).length === 7 && Array.from({length:7}, (_, d) => validRanges(s.week[d])).every(Boolean) && Array.isArray(s.overrides) && s.overrides.length <= 100 && s.overrides.every(o => o && validDate(o.date) && validRanges(o.ranges)) && new Set(s.overrides.map(o => o.date)).size === s.overrides.length; }
export function validEvent(e: EventType) { return !!e && typeof e.title === "string" && !!e.title.trim() && e.title.length <= 100 && typeof e.description === "string" && e.description.length <= 1500 && typeof e.location === "string" && e.location.length <= 300 && typeof e.enabled === "boolean" && [e.duration, e.notice, e.horizon, e.before, e.after].every(Number.isInteger) && e.duration >= 15 && e.duration <= 240 && e.duration % 15 === 0 && e.notice >= 0 && e.notice <= 10080 && e.horizon >= 1 && e.horizon <= 180 && e.before >= 0 && e.before <= 120 && e.after >= 0 && e.after <= 120; }
export function validBooking(b: Booking) { return !!b && ["id", "type", "title", "start", "end", "name", "email", "notes", "location", "zone", "created"].every(k => typeof b[k as keyof Booking] === "string") && !!b.id && Number.isFinite(Date.parse(b.start)) && Number.isFinite(Date.parse(b.end)) && Date.parse(b.end) > Date.parse(b.start) && [b.before, b.after].every(v => Number.isInteger(v) && v >= 0 && v <= 120) && (b.status === "confirmed" || b.status === "cancelled"); }
/** All real instants for this wall time: none in a DST gap, two in a fold. */
export function wallInstants(date: string, minute: number, zone: string): number[] {
  const target = Date.parse(date + "T00:00:00Z") + minute * 60000;
  const offsets = new Set<number>();
  for (const hours of [-36, -12, 0, 12, 36]) {
    const instant = target + hours * 3600000, p = parts(instant, zone);
    offsets.add(Date.parse(p.date + "T00:00:00Z") + p.minute * 60000 - instant);
  }
  return [...offsets].map(o => target - o).filter(ms => { const p = parts(ms, zone); return p.date === date && p.minute === minute; }).sort((a, b) => a - b);
}
export function overlaps(start: number, end: number, before: number, after: number, b: Booking) {
  return b.status !== "cancelled" && start - before * 60000 < Date.parse(b.end) + b.after * 60000 && end + after * 60000 > Date.parse(b.start) - b.before * 60000;
}
export function slotsForDay(viewDate: string, viewZone: string, hostZone: string, event: EventType, schedule: Schedule, bookings: Booking[], now = Date.now()): Slot[] {
  if (!event.enabled || !validEvent(event) || !validSchedule(schedule) || !validDate(viewDate) || !validZone(viewZone) || !validZone(hostZone)) return [];
  const result = new Map<string, Slot>(), today = dateIn(now, hostZone), last = addDays(today, event.horizon);
  // Time zones differ by at most 26 hours: ±2 dates covers both date lines.
  for (const n of [-2, -1, 0, 1, 2]) {
    const day = addDays(viewDate, n);
    if (day < today || day > last) continue;
    const weekday = new Date(day + "T12:00:00Z").getUTCDay();
    const ranges = schedule.overrides.find(o => o.date === day)?.ranges ?? schedule.week[weekday] ?? [];
    for (const range of ranges) {
      const from = minutes(range.start), until = minutes(range.end), begins = wallInstants(day, from, hostZone), ends = wallInstants(day, until, hostZone);
      if (!begins.length || !ends.length) continue;
      for (let m = Math.ceil(from / 15) * 15; m < until; m += 15) for (const start of wallInstants(day, m, hostZone)) {
        const end = start + event.duration * 60000;
        if (start < now + event.notice * 60000 || dateIn(start, viewZone) !== viewDate || dateIn(end, hostZone) !== day || start - event.before * 60000 < begins[0] || end + event.after * 60000 > ends[ends.length - 1]) continue;
        if (bookings.some(b => overlaps(start, end, event.before, event.after, b))) continue;
        const iso = new Date(start).toISOString();
        result.set(iso, { start: iso, end: new Date(end).toISOString(), day });
      }
    }
  }
  return [...result.values()].sort((a, b) => a.start.localeCompare(b.start));
}
const escapeICS = (s: string) => s.replace(/\\/g, "\\\\").replace(/\r\n|\r|\n/g, "\\n").replace(/;/g, "\\;").replace(/,/g, "\\,");
const stamp = (s: string) => new Date(s).toISOString().replace(/[-:]/g, "").replace(/\.\d{3}/, "");
export function calendarFile(bookings: Booking[], now = new Date().toISOString()) {
  const lines = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//myco//Scheduling//EN", "CALSCALE:GREGORIAN"];
  for (const b of bookings) lines.push("BEGIN:VEVENT", `UID:${encodeURIComponent(b.id)}@myco-scheduling`, `DTSTAMP:${stamp(now)}`, `DTSTART:${stamp(b.start)}`, `DTEND:${stamp(b.end)}`, `SUMMARY:${escapeICS(b.title)}`, `DESCRIPTION:${escapeICS([b.name, b.email, b.notes].filter(Boolean).join("\n"))}`, `LOCATION:${escapeICS(b.location)}`, `STATUS:${b.status === "cancelled" ? "CANCELLED" : "CONFIRMED"}`, "END:VEVENT");
  lines.push("END:VCALENDAR");
  // RFC 5545 folds at 75 octets, preserving Unicode code points.
  return lines.map(line => { let out = "", width = 0; for (const c of line) { const len = new TextEncoder().encode(c).length; if (width + len > 75) { out += "\r\n "; width = 1; } out += c; width += len; } return out; }).join("\r\n") + "\r\n";
}
