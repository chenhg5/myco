// Month cells and time lanes display the same occurrences. Drag/drop moves
// personal events; appointments remain linked to their own management screen.
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { t as uiText, locale } from "../_lib/i18n";
import { addDays, dateIn } from "./engine";
import { onDate, timeLayout, weekStart, type Occurrence } from "./calendar-engine";
import { timeText } from "./booking";
import type { EditDraft } from "./event-editor";
export type View = "month" | "week" | "day";
export const weekday = (date: string, full = false) => new Intl.DateTimeFormat(locale(), { weekday: full ? "long" : "short", timeZone: "UTC" }).format(new Date(date + "T12:00:00Z"));
export const dragType = "application/x-myco-calendar";
export type DragEvent = { id: string; version: number; from: string };
function EventChip({ item, date, zone, open, compact = false }: { item: Occurrence; date: string; zone: string; open: (o: Occurrence) => void; compact?: boolean }) {
  return <button type="button" className={`pc-event pc-${item.calendar}${item.allDay ? " is-all-day" : ""}${compact ? " is-compact" : ""}`} data-event={item.id} title={`${item.allDay ? uiText("全天") : timeText(item.start, zone, true)} · ${item.title}${item.event?.data.repeat !== "none" && item.event ? " · " + uiText("重复日程") : ""}`} draggable={!!item.event}
    onDragStart={e => { if (!item.event) return; e.dataTransfer.setData(dragType, JSON.stringify({ id: item.id, version: item.event.version, from: date })); e.dataTransfer.effectAllowed = "move"; }} onClick={e => { e.stopPropagation(); open(item); }}>
    {!item.allDay && <time>{timeText(item.start, zone)}</time>}<span>{item.title}</span>{item.event?.data.repeat !== "none" && item.event && <small aria-label={uiText("重复日程")}>↻</small>}
  </button>;
}
function drop(e: React.DragEvent, date: string, move: (d: DragEvent, date: string, minute?: number) => void, minute?: number) {
  e.preventDefault(); e.currentTarget.classList.remove("is-drop");
  try { const d = JSON.parse(e.dataTransfer.getData(dragType)); if (typeof d.id === "string" && typeof d.version === "number" && typeof d.from === "string") move(d, date, minute); } catch { /* unrelated drag */ }
}
const over = (e: React.DragEvent) => { if (e.dataTransfer.types.includes(dragType)) { e.preventDefault(); e.dataTransfer.dropEffect = "move"; e.currentTarget.classList.add("is-drop"); } };
export function MonthView({ cursor, zone, items, create, open, move, select }: { cursor: string; zone: string; items: Occurrence[]; create: (d: EditDraft) => void; open: (o: Occurrence) => void; move: (d: DragEvent, date: string) => void; select: (date: string) => void }) {
  const first = weekStart(cursor.slice(0, 7) + "-01"), today = dateIn(Date.now(), zone);
  const grid = useRef<HTMLDivElement>(null), [capacity, setCapacity] = useState(3);
  useEffect(() => { const el = grid.current; if (!el) return; const resize = () => setCapacity(Math.max(1, Math.min(4, Math.floor((el.clientHeight / 6 - 34) / 24)))); const observer = new ResizeObserver(resize); observer.observe(el); resize(); return () => observer.disconnect(); }, []);
  return <div className="pc-month"><div className="pc-weekdays">{Array.from({ length: 7 }, (_, i) => <span key={i}>{weekday(addDays(first, i))}</span>)}</div><div className="pc-month-grid" ref={grid} role="grid" aria-label={uiText("月视图")}>
    {Array.from({ length: 42 }, (_, i) => { const date = addDays(first, i), list = items.filter(o => onDate(o, date, zone)).sort((a, b) => Number(b.allDay) - Number(a.allDay) || a.start.localeCompare(b.start));
      return <section key={date} className={`pc-cell${date.slice(0, 7) !== cursor.slice(0, 7) ? " is-other" : ""}`} data-date={date} role="gridcell" aria-label={date} tabIndex={0} onClick={() => create({ date })} onKeyDown={e => { if (e.target === e.currentTarget && e.key === "Enter") { e.preventDefault(); create({ date }); } }} onDragOver={over} onDragLeave={e => e.currentTarget.classList.remove("is-drop")} onDrop={e => drop(e, date, move)}>
        <button className={`pc-day-number${date === today ? " is-today" : ""}`} type="button" aria-label={uiText("查看 {0}", date)} onClick={e => { e.stopPropagation(); select(date); }}>{Number(date.slice(8))}</button>
        <div className="pc-cell-events">{list.slice(0, list.length > capacity ? capacity - 1 : capacity).map(o => <EventChip key={o.key} item={o} date={date} zone={zone} open={open} compact />)}{list.length > capacity && <button type="button" className="pc-more" onClick={e => { e.stopPropagation(); select(date); }}>{uiText("另外 {0} 项", list.length - capacity + 1)}</button>}</div>
      </section>;
    })}</div></div>;
}
export function TimeView({ cursor, view, zone, items, create, open, move }: { cursor: string; view: "week" | "day"; zone: string; items: Occurrence[]; create: (d: EditDraft) => void; open: (o: Occurrence) => void; move: (d: DragEvent, date: string, minute?: number) => void }) {
  const scroll = useRef<HTMLDivElement>(null), allDay = useRef<HTMLDivElement>(null), [gutters, setGutters] = useState({ timed: 0, allDay: 0 });
  const start = view === "day" ? cursor : weekStart(cursor), dates = Array.from({ length: view === "day" ? 1 : 7 }, (_, i) => addDays(start, i)), today = dateIn(Date.now(), zone);
  // Overlay scrollbars occupy no space. Either scroll pane can also have a
  // classic gutter; compensate only their difference instead of counting twice.
  useLayoutEffect(() => {
    const timed = scroll.current, wholeDay = allDay.current; if (!timed || !wholeDay) return;
    const measure = () => {
      const next = { timed: timed.offsetWidth - timed.clientWidth, allDay: wholeDay.offsetWidth - wholeDay.clientWidth };
      setGutters(old => old.timed === next.timed && old.allDay === next.allDay ? old : next);
    };
    const observer = new ResizeObserver(measure); observer.observe(timed); observer.observe(wholeDay); measure();
    return () => observer.disconnect();
  }, []);
  useEffect(() => { if (scroll.current) scroll.current.scrollTop = 7 * 48; }, [view, start]);
  const style = { gridTemplateColumns: `48px repeat(${dates.length}, minmax(0, 1fr))` };
  return <div className={`pc-time-view pc-${view}`}><div className="pc-time-head" style={{ ...style, paddingRight: gutters.timed }}><span className="pc-zone-label">{uiText("时间")}</span>{dates.map(date => <button type="button" key={date} onClick={() => create({ date })}><span>{weekday(date)}</span><b className={date === today ? "is-today" : ""}>{Number(date.slice(8))}</b></button>)}</div>
    <div className="pc-all-day-row" ref={allDay} style={{ ...style, paddingRight: Math.max(0, gutters.timed - gutters.allDay) }}><span className="pc-zone-label">{uiText("全天")}</span>{dates.map(date => <div className="pc-all-day-cell" key={date} aria-label={uiText("{0} 全天", date)} onClick={() => create({ date, allDay: true })} onDragOver={over} onDragLeave={e => e.currentTarget.classList.remove("is-drop")} onDrop={e => drop(e, date, move)}>{items.filter(o => o.allDay && onDate(o, date, zone)).map(o => <EventChip key={o.key} item={o} date={date} zone={zone} open={open} compact />)}</div>)}</div>
    <div className="pc-time-scroll" ref={scroll}><div className="pc-time-grid" style={style}><div className="pc-hours">{Array.from({ length: 24 }, (_, h) => <span key={h} style={{ top: h * 48 }}>{String(h).padStart(2, "0")}:00</span>)}</div>
      {dates.map(date => <div key={date} className="pc-day-lane" data-date={date} role="gridcell" aria-label={uiText("{0} 时间轴", date)} tabIndex={0} onClick={e => { if (e.target !== e.currentTarget) return; const m = Math.min(1425, Math.max(0, Math.floor((e.clientY - e.currentTarget.getBoundingClientRect().top) / 48 * 4) * 15)); create({ date, minute: m }); }} onKeyDown={e => { if (e.key === "Enter" && e.target === e.currentTarget) create({ date }); }} onDragOver={over} onDragLeave={e => e.currentTarget.classList.remove("is-drop")} onDrop={e => { const m = Math.min(1425, Math.max(0, Math.round((e.clientY - e.currentTarget.getBoundingClientRect().top) / 48 * 4) * 15)); drop(e, date, move, m); }}>
        {timeLayout(items, date, zone).map(b => <div className="pc-timed-event" key={b.item.key} style={{ top: b.top / 60 * 48, height: Math.max(22, (b.bottom - b.top) / 60 * 48 - 2), left: `calc(${b.column / b.columns * 100}% + 2px)`, width: `calc(${100 / b.columns}% - 4px)` }}><EventChip item={b.item} date={date} zone={zone} open={open} compact={(b.bottom-b.top)<50} /></div>)}
      </div>)}
    </div></div>
  </div>;
}
