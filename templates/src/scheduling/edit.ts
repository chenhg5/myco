// All writes live here. Reservation mutations use explicit versions, reload
// before validation and never retry stale choices after a conflict.
import { t as uiText } from "../_lib/i18n";
import { myco, type Store } from "../_lib/myco";
import { uid } from "../_lib/appkit";
import { availability, bookings, days, eventData, hostZone, invalidLedger, kit, profile, schedule, types, type Profile } from "./model";
import { overlaps, slotsForDay, validEvent, validSchedule, validZone, type Booking, type EventType, type Schedule, type Slot } from "./engine";

export async function write(store: Store<any>, id: string, value: Record<string, unknown>, version = store.get(id)?.version || 0) {
  const before = store.get(id)?.data;
  const result = await kit.track(myco!.data.put(store.collection, value, { id, ifVersion: version }));
  await store.load(); kit.emit();
  if (result && before) kit.flash(uiText("已保存"), () => { void write(store, id, before, result.version).catch(e => kit.flash(e.message)); });
  return !!result;
}
export function saveType(id: string, value: EventType, version = types.get(id)?.version || 0) {
  if (!validEvent(value)) throw new Error(uiText("检查标题、时长和预约范围，再保存。"));
  return write(types, id || uid("type"), { ...types.get(id)?.data, ...value, order: types.get(id)?.data.order ?? types.rows.length + 1 }, version);
}
export function saveSchedule(value: Schedule, version = availability.get("main")?.version || 0) {
  if (!validSchedule(value)) throw new Error(uiText("时段不能重叠，结束时间要晚于开始时间。"));
  return write(availability, "main", { ...availability.get("main")?.data, ...value }, version);
}
export async function saveProfile(value: Profile, version = profile.get("main")?.version || 0) {
  if (!value.host?.trim() || !value.zone || !validZone(value.zone)) throw new Error(uiText("填写主持人和有效时区。"));
  // The ledger day is in this zone. Freeze it after the first reservation so
  // later settings cannot put the same instant in two independently locked days.
  await profile.load();
  if (profile.get("main")?.data.zone && value.zone !== hostZone()) throw new Error(uiText("日历时区在创建后固定；访客可以切换显示时区。"));
  // Explicit creation CAS also covers two people setting up an empty page.
  const current = profile.get("main");
  const result = await kit.track(myco!.data.put("profile", { ...current?.data, ...value }, { id: "main", ifVersion: version }));
  await profile.load(); kit.emit();
  return !!result;
}
async function commitDay(day: string, entries: Booking[], version: number) {
  if (entries.length > 80 || new TextEncoder().encode(JSON.stringify({ entries })).length > 58000) throw new Error(uiText("这一天的预约记录已满，请选择其他日期。"));
  let failed: { code?: string } | undefined;
  const result = await kit.track(myco!.data.put("days", { entries }, { id: day, ifVersion: version }).catch(e => { failed = e; throw e; }));
  if (!result) {
    await days.load().catch(() => {}); kit.emit();
    throw new Error(failed?.code === "conflict" ? uiText("这个时段刚被其他人修改，请重新选择。") : uiText("未能确认保存结果，请先查看预约记录。"));
  }
  // The successful response is authoritative even if a subsequent refresh
  // loses connectivity. Do not label a confirmed write as a failed booking.
  const current = days.get(day);
  if (!current || current.version <= result.version) days.rows = [...days.rows.filter(r => r.id !== day), { ...result, data: { entries } }];
  kit.emit();
}
let changing = false;
async function exclusive<T>(f: () => Promise<T>) {
  if (changing) throw new Error(uiText("请等上一项预约保存完成。"));
  changing = true;
  try { return await f(); } finally { changing = false; }
}
export function reserve(typeId: string, slot: Slot, guest: { name: string; email: string; notes: string; zone: string }) {
  return exclusive(async () => {
    await Promise.all([days.load(), types.load(), availability.load(), profile.load()]);
    if (invalidLedger()) throw new Error(uiText("预约数据有误，请让 agent 检查后再预约。"));
    const rec = types.get(typeId), event = eventData(rec?.data || {});
    if (!profile.get("main")?.data.zone || !rec || !slotsForDay(slot.day, hostZone(), hostZone(), event, schedule(), bookings()).some(s => s.start === slot.start && s.end === slot.end && s.day === slot.day)) { kit.emit(); throw new Error(uiText("这个时段已不可预约，请重新选一个。")); }
    if (!guest.name.trim() || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(guest.email.trim()) || guest.name.length > 100 || guest.email.length > 200 || guest.notes.length > 1000 || !validZone(guest.zone)) throw new Error(uiText("填写姓名和有效邮箱，备注不超过 1000 字。"));
    const row = days.get(slot.day), entry: Booking = { ...guest, name: guest.name.trim(), email: guest.email.trim(), id: uid("booking"), type: typeId, title: event.title, start: slot.start, end: slot.end, location: event.location, before: event.before, after: event.after, status: "confirmed", created: new Date().toISOString() };
    await commitDay(slot.day, [...(row?.data.entries || []), entry], row?.version || 0);
    return entry;
  });
}
export function setStatus(day: string, id: string, status: Booking["status"]) {
  return exclusive(async () => {
    await days.load();
    if (invalidLedger()) throw new Error(uiText("预约数据有误，请让 agent 检查后再预约。"));
    const row = days.get(day), b = row?.data.entries?.find(e => e.id === id);
    if (!row || !b) throw new Error(uiText("没有找到这项预约，请刷新页面。"));
    if (status === "confirmed") {
      if (bookings().some(o => o.id !== id && overlaps(Date.parse(b.start), Date.parse(b.end), b.before, b.after, o))) throw new Error(uiText("原时段已经被占用，无法恢复预约。"));
    }
    await commitDay(day, row.data.entries!.map(e => e.id === id ? { ...e, status } : e), row.version);
    if (status === "cancelled") kit.flash(uiText("已取消预约"), () => { void setStatus(day, id, "confirmed").catch(e => kit.flash(e.message)); });
  });
}
