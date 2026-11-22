// Editors stage changes locally, validate on submit, and keep drafts when a
// save fails. Weekly hours and date overrides share the same range editor.
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Plus, Trash2, X } from "lucide-react";
import { t as uiText, locale } from "../_lib/i18n";
import { defaultEvent, validZone, type EventType, type Range, type Schedule } from "./engine";
import { availability, hostZone, profile, schedule, types } from "./model";
import { saveProfile, saveSchedule, saveType } from "./edit";

export const zoneList = (() => { try { return (Intl as unknown as { supportedValuesOf: (key: string) => string[] }).supportedValuesOf("timeZone"); } catch { return ["Asia/Shanghai", "Asia/Tokyo", "Asia/Kolkata", "Europe/London", "Europe/Paris", "America/New_York", "America/Los_Angeles", "Australia/Sydney"]; } })();
export function ZoneSelect({ value, onChange, disabled = false }: { value: string; onChange: (v: string) => void; disabled?: boolean }) {
  return <select className="sel sc-zone" aria-label={uiText("时区")} value={value} disabled={disabled} onChange={e => onChange(e.target.value)}>{[...new Set([value, "UTC", ...zoneList])].map(zone => <option key={zone} value={zone}>{zone}</option>)}</select>;
}
export function Modal({ title, close, children }: { title: string; close: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => { ref.current?.showModal(); }, []);
  return <dialog className="sc-modal" ref={ref} onCancel={e => { e.preventDefault(); close(); }} aria-label={title}>
    <header><h2>{title}</h2><button className="icon-btn" type="button" title={uiText("关闭")} aria-label={uiText("关闭")} onClick={close}><X className="ico" /></button></header>{children}
  </dialog>;
}
export function Field({ label, children }: { label: string; children: ReactNode }) { return <label className="sc-field"><span>{label}</span>{children}</label>; }
export const ErrorNote = ({ text }: { text: string }) => text ? <p className="sc-error" role="alert">{text}</p> : null;
export function TypeEditor({ id, data, close }: { id: string; data?: EventType; close: () => void }) {
  const [v, set] = useState<EventType>(data || { ...defaultEvent }), [error, setError] = useState(""), [busy, setBusy] = useState(false);
  const [version] = useState(() => types.get(id)?.version || 0);
  const patch = (p: Partial<EventType>) => set(x => ({ ...x, ...p }));
  const number = (key: "duration" | "notice" | "horizon" | "before" | "after", label: string, max: number, min = 0, step = 1) => <Field label={label}><input className="inp" type="number" min={min} max={max} step={step} required value={v[key]} onChange={e => patch({ [key]: Number(e.target.value) })} /></Field>;
  return <Modal title={id ? uiText("编辑预约类型") : uiText("新建预约类型")} close={close}><form onSubmit={async e => { e.preventDefault(); setBusy(true); setError(""); try { if (await saveType(id, v, version)) close(); else setError(uiText("保存失败，可能有人刚修改过。请关闭后重新打开再试。")); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } }}>
    <Field label={uiText("预约名称")}><input autoFocus className="inp" required maxLength={100} value={v.title} placeholder={uiText("例如：30 分钟交流")} onChange={e => patch({ title: e.target.value })} /></Field>
    <Field label={uiText("说明")}><textarea className="inp" rows={3} maxLength={1500} value={v.description} onChange={e => patch({ description: e.target.value })} /></Field>
    <div className="sc-form-grid">{number("duration", uiText("时长（分钟）"), 240, 15, 15)}{number("horizon", uiText("可预约未来几天"), 180, 1)}</div>
    <Field label={uiText("地点或会议链接")}><input className="inp" maxLength={300} value={v.location} placeholder={uiText("例如：线上会议，或填写地址")} onChange={e => patch({ location: e.target.value })} /></Field>
    <div className="sc-form-grid">{number("notice", uiText("至少提前（分钟）"), 10080)}{number("before", uiText("会前留空（分钟）"), 120)}{number("after", uiText("会后留空（分钟）"), 120)}</div>
    <label className="sc-check"><input type="checkbox" checked={v.enabled} onChange={e => patch({ enabled: e.target.checked })} />{uiText("开放预约")}</label>
    <p className="faint">{uiText("所有类型共用一位主持人的时间；留空时间也会避开其他预约。")}</p><ErrorNote text={error} />
    <footer><button type="button" className="btn" onClick={close}>{uiText("取消")}</button><button className="btn btn-primary" disabled={busy}>{busy ? uiText("正在保存…") : uiText("保存")}</button></footer>
  </form></Modal>;
}
function Ranges({ value, onChange }: { value: Range[]; onChange: (v: Range[]) => void }) {
  return <div className="sc-ranges">{value.length === 0 && <span className="faint">{uiText("不开放")}</span>}{value.map((r, i) => <div className="sc-range" key={i}>
    <input className="inp" aria-label={uiText("开始时间")} type="time" required value={r.start} onChange={e => onChange(value.map((x, j) => j === i ? { ...x, start: e.target.value } : x))} /><span className="faint">–</span>
    <input className="inp" aria-label={uiText("结束时间")} type="time" required value={r.end} onChange={e => onChange(value.map((x, j) => j === i ? { ...x, end: e.target.value } : x))} />
    <button type="button" className="icon-btn" title={uiText("移除时段")} aria-label={uiText("移除时段")} onClick={() => onChange(value.filter((_, j) => j !== i))}><X className="ico" /></button></div>)}
    <button type="button" className="icon-btn" title={uiText("添加时段")} aria-label={uiText("添加时段")} disabled={value.length >= 8} onClick={() => onChange([...value, { start: "09:00", end: "17:00" }])}><Plus className="ico" /></button>
  </div>;
}
export function AvailabilityEditor() {
  const [v, set] = useState<Schedule>(() => structuredClone(schedule())), [error, setError] = useState(""), [busy, setBusy] = useState(false), [date, setDate] = useState("");
  const [version, setVersion] = useState(() => availability.get("main")?.version || 0);
  return <form className="sc-availability" onSubmit={async e => { e.preventDefault(); setBusy(true); setError(""); try { if (await saveSchedule(v, version)) { setVersion(availability.get("main")?.version || 0); setError(uiText("可用时间已保存")); } else setError(uiText("保存失败，可能有人刚修改过。请关闭后重新打开再试。")); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } }}>
    <div className="sc-section-head"><div><h2>{uiText("每周可用时间")}</h2><p className="faint">{uiText("按 {0} 安排，所有预约类型共用。", hostZone())}</p></div><button className="btn btn-primary" disabled={busy}>{busy ? uiText("正在保存…") : uiText("保存")}</button></div>
    {[1, 2, 3, 4, 5, 6, 0].map(d => <div className="sc-week-row" key={d}><b>{new Intl.DateTimeFormat(locale(), { weekday: "long", timeZone: "UTC" }).format(new Date(Date.UTC(2026, 10, 22 + d)))}</b><Ranges value={v.week[d] || []} onChange={ranges => set({ ...v, week: { ...v.week, [d]: ranges } })} /></div>)}
    <section className="sc-overrides"><h2>{uiText("例外日期")}</h2><p className="faint">{uiText("假日休息，或为某一天单独安排时间。例外会替代每周安排。")}</p>
      <div className="sc-inline"><input type="date" className="inp" aria-label={uiText("例外日期")} value={date} onChange={e => setDate(e.target.value)} /><button type="button" className="btn" disabled={!date || v.overrides.some(o => o.date === date) || v.overrides.length >= 100} onClick={() => { set({ ...v, overrides: [...v.overrides, { date, ranges: [] }].sort((a, b) => a.date.localeCompare(b.date)) }); setDate(""); }}><Plus className="ico" />{uiText("添加日期")}</button></div>
      {v.overrides.map(o => <div className="sc-week-row" key={o.date}><b>{o.date}</b><Ranges value={o.ranges} onChange={ranges => set({ ...v, overrides: v.overrides.map(x => x.date === o.date ? { ...x, ranges } : x) })} /><button type="button" className="icon-btn" title={uiText("移除例外")} aria-label={uiText("移除例外")} onClick={() => set({ ...v, overrides: v.overrides.filter(x => x.date !== o.date) })}><Trash2 className="ico" /></button></div>)}
    </section><p role="status" className={error === uiText("可用时间已保存") ? "sc-success" : "sc-error"}>{error}</p>
  </form>;
}
export function ProfileEditor({ close }: { close: () => void }) {
  const old = profile.get("main")?.data, localZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const [version] = useState(() => profile.get("main")?.version || 0);
  const [v, set] = useState({ title: old?.title || "", host: old?.host || "", bio: old?.bio || "", zone: old?.zone || (validZone(localZone) ? localZone : "Asia/Shanghai") }), [error, setError] = useState(""), [busy, setBusy] = useState(false);
  return <Modal title={uiText("预约主页")} close={close}><form onSubmit={async e => { e.preventDefault(); setBusy(true); try { if (await saveProfile(v, version)) close(); else setError(uiText("保存失败，可能有人刚修改过。请关闭后重新打开再试。")); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } }}>
    <Field label={uiText("页面名称")}><input autoFocus className="inp" maxLength={100} value={v.title} onChange={e => set({ ...v, title: e.target.value })} placeholder={uiText("我的预约日历")} /></Field>
    <Field label={uiText("主持人")}><input className="inp" required maxLength={100} value={v.host} onChange={e => set({ ...v, host: e.target.value })} /></Field>
    <Field label={uiText("自我介绍")}><textarea className="inp" rows={3} maxLength={1000} value={v.bio} onChange={e => set({ ...v, bio: e.target.value })} /></Field>
    <Field label={uiText("日历时区")}><ZoneSelect value={v.zone} disabled={!!old?.zone} onChange={zone => set({ ...v, zone })} /></Field>
    <p className="faint">{uiText("日历时区在创建后固定；访客可以切换显示时区。")}</p><p className="faint">{uiText("这是共享页面内的预约。能打开此页面的人和 agent 都能查看预约信息；不会发送邮件，也不会同步外部日历。")}</p>
    <ErrorNote text={error} /><footer><button className="btn" type="button" onClick={close}>{uiText("取消")}</button><button className="btn btn-primary" disabled={busy}>{uiText("保存")}</button></footer>
  </form></Modal>;
}
