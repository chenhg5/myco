// 习惯打卡: habits a team or a family keeps, each person checking off their
// own days. A habit is a record; a check is a record (its id: the habit,
// the day, the person), so checking twice is the same check.
import "../_lib/sheet.css";
import "./habit.css";
import { useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Flame, Plus, Trash2 } from "lucide-react";
import { Store } from "../_lib/myco";
import { Bar, Kit, Toast, mount, titleSaver, uid, useKit } from "../_lib/appkit";
import { addDays, today, WEEK } from "../_lib/dates";
import { personColor } from "../_lib/cells";

type Habit = { name?: string; icon?: string; goal?: number; order?: number };
type Check = { habit?: string; date?: string; who?: string };
const meta = new Store<{ title?: string }>("tracker");
const habits = new Store<Habit & Record<string, unknown>>("habits");
const checks = new Store<Check & Record<string, unknown>>("checks");
const kit = new Kit([meta, habits, checks], { quiet: true });
const title = titleSaver(kit, meta);
const ICONS = ["💧", "🏃", "📖", "🧘", "🥗", "😴", "✍️", "🎸", "🧹", "💊", "🌱", "☀️"];
const slug = (s: string) => { let h = 5381; for (const c of s) h = ((h * 33) ^ c.charCodeAt(0)) >>> 0; return h.toString(36); };

function App() {
  useKit(kit);
  const t0 = today();
  const [end, setEnd] = useState(t0); // the week shown ends on this day
  const [adding, setAdding] = useState("");
  const [icon, setIcon] = useState(ICONS[0]);
  const days = Array.from({ length: 7 }, (_, i) => addDays(end, i - 6));
  const me = kit.me;
  const by = useMemo(() => {
    const m = new Map<string, Map<string, Set<string>>>(); // habit → date → people
    for (const r of checks.rows) {
      const h = String(r.data.habit || ""), d = String(r.data.date || ""), w = String(r.data.who || r.updated_by || "");
      if (!h || !d) continue;
      if (!m.has(h)) m.set(h, new Map());
      if (!m.get(h)!.has(d)) m.get(h)!.set(d, new Set());
      m.get(h)!.get(d)!.add(w);
    }
    return m;
  }, [kit.version]); // eslint-disable-line react-hooks/exhaustive-deps
  const did = (h: string, d: string, who = me) => !!by.get(h)?.get(d)?.has(who);
  const streak = (h: string) => { let n = 0, d = did(h, t0) ? t0 : addDays(t0, -1); while (did(h, d)) { n++; d = addDays(d, -1); } return n; };
  const monthCount = (h: string) => [...(by.get(h) || new Map())].filter(([d, s]) => d.startsWith(t0.slice(0, 7)) && s.has(me)).length;
  const toggle = (h: string, d: string) => {
    if (d > t0 || !me) return;
    const id = `${h}-${d.replace(/-/g, "")}-${slug(me)}`;
    if (checks.get(id)) kit.track(checks.remove(id));
    else kit.track(checks.put({ habit: h, date: d, who: me }, id));
    kit.emit();
  };
  const add = () => {
    if (!adding.trim()) return;
    const last = habits.rows[habits.rows.length - 1]?.data.order;
    kit.track(habits.put({ name: adding.trim(), icon, goal: 7, order: (typeof last === "number" ? last : 0) + 1 }, uid("h")));
    setAdding("");
    setIcon(ICONS[(ICONS.indexOf(icon) + 1) % ICONS.length]);
    kit.emit();
  };
  const remove = (id: string) => {
    const r = habits.get(id);
    if (!r || !window.confirm(`不再打卡「${r.data.name}」？打过的卡会留着。`)) return;
    const data = r.data;
    kit.track(habits.remove(id));
    kit.flash("删除了一个习惯", () => { kit.track(habits.put(data, id)); kit.note = null; kit.emit(); });
  };
  const doneToday = habits.rows.filter((h) => did(h.id, t0)).length;
  const people = useMemo(() => { const s = new Set<string>(); for (const m of by.values()) for (const p of m.values()) for (const w of p) s.add(w); return [...s]; }, [by]);
  return (
    <div className="app hb">
      <Bar kit={kit} title={title.get()} placeholder="习惯打卡" onTitle={title.set} />
      <div className="hb-main"><div className="hb-inner">
        <div className="hb-top">
          <div className="hb-today"><b>{doneToday}/{habits.rows.length}</b><span>今天打了卡</span></div>
          <span className="top-gap" />
          <button type="button" className="icon-btn" aria-label="前一周" onClick={() => setEnd(addDays(end, -7))}><ChevronLeft className="ico" /></button>
          <span className="hb-range">{days[0].slice(5).replace("-", "/")} – {days[6].slice(5).replace("-", "/")}</span>
          <button type="button" className="icon-btn" aria-label="后一周" disabled={end >= t0} onClick={() => setEnd(addDays(end, 7) > t0 ? t0 : addDays(end, 7))}><ChevronRight className="ico" /></button>
        </div>
        <div className="hb-grid">
          <div className="hb-row hb-head">
            <span />
            {days.map((d) => <span key={d} className={`hb-day${d === t0 ? " is-today" : ""}`}>周{WEEK[new Date(`${d}T12:00:00`).getDay()]}<b>{Number(d.slice(8))}</b></span>)}
            <span className="hb-stat">连续</span>
          </div>
          {habits.rows.map((h) => {
            const s = streak(h.id);
            return (
              <div key={h.id} className="hb-row">
                <span className="hb-name"><span className="hb-icon">{h.data.icon || "✅"}</span><span className="hb-text"><b>{h.data.name}</b><small className="faint">本月 {monthCount(h.id)} 天</small></span>
                  <button type="button" className="icon-btn hb-del" aria-label="删除这个习惯" onClick={() => remove(h.id)}><Trash2 className="ico" /></button></span>
                {days.map((d) => {
                  const mine = did(h.id, d), others = [...(by.get(h.id)?.get(d) || [])].filter((w) => w !== me);
                  return (
                    <span key={d} className="hb-cell">
                      <button type="button" className={`hb-check${mine ? " is-on" : ""}${d > t0 ? " is-future" : ""}`} disabled={d > t0} onClick={() => toggle(h.id, d)}
                        aria-label={`${d} ${h.data.name}${mine ? "：已打卡" : ""}`}>{mine ? "✓" : ""}</button>
                      {others.length > 0 && <span className="hb-others">{others.slice(0, 3).map((w) => <i key={w} title={w} style={{ background: personColor(w) }} />)}</span>}
                    </span>
                  );
                })}
                <span className={`hb-stat${s >= 3 ? " is-hot" : ""}`}>{s > 0 ? <><Flame className="ico" />{s}</> : "–"}</span>
              </div>
            );
          })}
          <form className="hb-row hb-add" onSubmit={(e) => { e.preventDefault(); add(); }}>
            <span className="hb-name">
              <button type="button" className="hb-icon is-pick" title="换个图标" onClick={() => setIcon(ICONS[(ICONS.indexOf(icon) + 1) % ICONS.length])}>{icon}</button>
              <input className="inp" value={adding} onChange={(e) => setAdding(e.target.value)} placeholder="加一个习惯：每天喝 8 杯水" aria-label="新习惯" />
            </span>
            <button type="submit" className="btn" disabled={!adding.trim()}><Plus className="ico" />添加</button>
          </form>
        </div>
        {people.length > 1 && (
          <div className="hb-people">
            {people.map((w) => <span key={w}><i style={{ background: personColor(w) }} />{w}{w === me ? "（你）" : ""}</span>)}
            <span className="faint">格子里的小圆点是别人打的卡</span>
          </div>
        )}
      </div></div>
      <Toast kit={kit} />
    </div>
  );
}

void mount(kit, App, async () => {
  if (!meta.get("main")) await meta.put({ title: "" }, "main")?.catch(() => meta.load());
});
