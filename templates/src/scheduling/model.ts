// Shared page records and safe defaults. A day is one versioned reservation
// record, so two people cannot confirm overlapping bookings concurrently.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
import { dateIn, defaultEvent, defaultWeek, overlaps, validBooking, validDate, validSchedule, validZone, type Booking, type EventType, type Schedule } from "./engine";
export type Profile = { title?: string; host?: string; bio?: string; zone?: string };
export type Day = { entries?: Booking[] };
export const profile = new Store<Profile & Record<string, unknown>>("profile");
export const types = new Store<Partial<EventType> & { order?: number } & Record<string, unknown>>("types");
export const availability = new Store<Partial<Schedule> & Record<string, unknown>>("availability");
export const days = new Store<Day & Record<string, unknown>>("days");
export const kit = new Kit([profile, types, availability, days], { quiet: true });
export const hostZone = () => validZone(String(profile.get("main")?.data.zone || "")) ? String(profile.get("main")!.data.zone) : "Asia/Shanghai";
export const schedule = (): Schedule => {
  const s = { week: availability.get("main")?.data.week || structuredClone(defaultWeek), overrides: availability.get("main")?.data.overrides || [] };
  return validSchedule(s) ? s : { week: Object.fromEntries(Array.from({length:7}, (_, d) => [d, []])), overrides: [] };
};
export const eventData = (data: Partial<EventType>): EventType => ({ ...defaultEvent, ...data });
export const bookings = () => days.rows.flatMap(r => Array.isArray(r.data.entries) ? r.data.entries.filter(validBooking).map(b => ({ ...b, day: r.id })) : []);
export const invalidLedger = () => {
  const ids = new Set<string>();
  return days.rows.some(r => !validDate(r.id) || !Array.isArray(r.data.entries) || r.data.entries.some((b, i, entries) => {
    if (!validBooking(b) || dateIn(Date.parse(b.start) - b.before * 60000, hostZone()) !== r.id || dateIn(Date.parse(b.end) + b.after * 60000, hostZone()) !== r.id || ids.has(b.id)) return true;
    if (b.status === "confirmed" && entries.slice(0, i).some(other => validBooking(other) && overlaps(Date.parse(b.start), Date.parse(b.end), b.before, b.after, other))) return true;
    ids.add(b.id); return false;
  }));
};
