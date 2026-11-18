// 待办清单: a list people and agents keep together. Type a to-do and press
// Enter ("明天", "周五", "@名字" and "!" are understood); tick it off; open
// it for a due date, who does it and a note; drag to reorder.
import "../_lib/sheet.css";
import "./todo.css";
import { useMemo, useRef, useState } from "react";
import { Calendar, ChevronDown, ChevronRight, GripVertical, Plus, Star, Trash2, X } from "lucide-react";
import { Store } from "../_lib/myco";
import { Bar, Kit, Toast, mount, titleSaver, uid, useKit } from "../_lib/appkit";
import { addDays, dayName, today, WEEK } from "../_lib/dates";
import { personColor } from "../_lib/cells";

type Task = { title?: string; done?: boolean; due?: string | null; owner?: string | null; star?: boolean; note?: string; order?: number; done_at?: string | null };
const list = new Store<{ title?: string }>("list");
const tasks = new Store<Task & Record<string, unknown>>("tasks");
const kit = new Kit([list, tasks]);
const title = titleSaver(kit, list);

/** "周五交周报 @Ann !" → title, due, owner, star. */
function understand(s: string): Task {
  let t = s.trim(), due: string | null = null, owner: string | null = null, star = false;
  const at = /(^|\s)@(\S+)/.exec(t);
  if (at) { owner = at[2]; t = t.replace(at[0], " "); }
  if (/(^|\s)!(\s|$)|！/.test(t)) { star = true; t = t.replace(/(^|\s)!(?=\s|$)|！/g, " "); }
  const words: [RegExp, () => string][] = [
    [/今天/, () => today()], [/明天/, () => addDays(today(), 1)], [/后天/, () => addDays(today(), 2)],
    [/(下)?(周|星期)([一二三四五六日天])/, () => ""],
  ];
  for (const [re, f] of words) {
    const m = re.exec(t);
    if (!m) continue;
    if (re.source.includes("周")) {
      const want = "日一二三四五六".indexOf(m[3] === "天" ? "日" : m[3]);
      const now = new Date().getDay();
      let n = (want - now + 7) % 7 || 7;
      if (m[1]) n += 7;
      due = addDays(today(), n);
    } else due = f();
    t = t.replace(m[0], " ");
    break;
  }
  const md = /(\d{1,2})月(\d{1,2})[日号]?/.exec(t);
  if (!due && md) { const y = new Date().getFullYear(); due = `${y}-${md[1].padStart(2, "0")}-${md[2].padStart(2, "0")}`; t = t.replace(md[0], " "); }
  return { title: t.replace(/\s+/g, " ").trim() || s.trim(), due, owner, star };
}

type Filter = "all" | "mine" | "today" | "star";
function App() {
  useKit(kit);
  const [filter, setFilter] = useState<Filter>("all");
  const [open, setOpen] = useState<string | null>(null);
  const [showDone, setShowDone] = useState(false);
  const [text, setText] = useState("");
  const [drag, setDrag] = useState<string | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const t0 = today();
  const all = tasks.rows;
  const keep = (r: (typeof all)[number]) => filter === "all" ? true : filter === "mine" ? r.data.owner === kit.me : filter === "today" ? !!r.data.due && r.data.due <= t0 : !!r.data.star;
  const todo = all.filter((r) => !r.data.done && keep(r));
  const done = all.filter((r) => r.data.done && keep(r)).sort((a, b) => String(b.data.done_at || "").localeCompare(String(a.data.done_at || "")));
  const counts = useMemo(() => ({
    all: all.filter((r) => !r.data.done).length, mine: all.filter((r) => !r.data.done && r.data.owner === kit.me).length,
    today: all.filter((r) => !r.data.done && r.data.due && r.data.due <= t0).length, star: all.filter((r) => !r.data.done && r.data.star).length,
  }), [kit.version]); // eslint-disable-line react-hooks/exhaustive-deps
  const put = (id: string, patch: Task) => { const cur = tasks.get(id); if (cur) kit.track(tasks.put({ ...cur.data, ...patch }, id)); kit.emit(); };
  const add = () => {
    if (!text.trim()) return;
    const t = understand(text), first = tasks.rows[0]?.data.order;
    if (filter === "mine" && !t.owner) t.owner = kit.me;
    if (filter === "today" && !t.due) t.due = t0;
    if (filter === "star") t.star = true;
    kit.track(tasks.put({ ...t, done: false, order: typeof first === "number" ? first - 1 : 1 }, uid("t")));
    kit.emit();
    setText("");
  };
  const toggle = (id: string) => { const d = !tasks.get(id)?.data.done; put(id, { done: d, done_at: d ? new Date().toISOString() : null }); };
  const remove = (id: string) => {
    const cur = tasks.get(id);
    if (!cur) return;
    const data = cur.data;
    kit.track(tasks.remove(id));
    setOpen(null);
    kit.flash("删除了一条待办", () => { kit.track(tasks.put(data, id)); kit.note = null; kit.emit(); });
  };
  const dropOn = (target: string) => {
    if (!drag || drag === target) return;
    const ids = tasks.rows.map((r) => r.id).filter((x) => x !== drag);
    ids.splice(ids.indexOf(target), 0, drag);
    const i = ids.indexOf(drag), a = tasks.get(ids[i - 1] || "")?.data.order, b = tasks.get(ids[i + 1] || "")?.data.order;
    const order = typeof a === "number" && typeof b === "number" ? (a + b) / 2 : typeof b === "number" ? b - 1 : typeof a === "number" ? a + 1 : 0;
    put(drag, { order });
    setDrag(null);
  };
  const row = (r: (typeof all)[number]) => {
    const d = r.data, late = !d.done && d.due && d.due < t0, isOpen = open === r.id;
    return (
      <li key={r.id} className={`td-row${d.done ? " is-done" : ""}${isOpen ? " is-open" : ""}${drag === r.id ? " is-drag" : ""}`}
        draggable={!d.done && !isOpen} onDragStart={() => setDrag(r.id)} onDragEnd={() => setDrag(null)}
        onDragOver={(e) => { if (drag) e.preventDefault(); }} onDrop={() => dropOn(r.id)}>
        <div className="td-line">
          {!d.done && <GripVertical className="ico td-grip" />}
          <button type="button" className={`td-check${d.done ? " is-on" : ""}`} aria-label={d.done ? "标为没做完" : "标为做完"} onClick={() => toggle(r.id)} />
          <TitleInput value={d.title || ""} onSave={(v) => put(r.id, { title: v })} />
          <span className="td-meta" onClick={() => setOpen(isOpen ? null : r.id)}>
            {d.due && <span className={`td-due${late ? " is-late" : d.due === t0 ? " is-today" : ""}`}><Calendar className="ico" />{dayName(d.due)}</span>}
            {d.owner && <span className="person-dot" title={d.owner} style={{ background: personColor(d.owner) }}>{d.owner.slice(0, 1).toUpperCase()}</span>}
            {d.note && <span className="td-note-mark" title={d.note}>…</span>}
          </span>
          <button type="button" className={`td-star${d.star ? " is-on" : ""}`} aria-label={d.star ? "取消重要" : "标为重要"} onClick={() => put(r.id, { star: !d.star })}><Star className="ico" /></button>
          <button type="button" className="icon-btn td-more" aria-label="详情" onClick={() => setOpen(isOpen ? null : r.id)}>{isOpen ? <ChevronDown className="ico" /> : <ChevronRight className="ico" />}</button>
        </div>
        {isOpen && (
          <div className="td-detail">
            <label><span>截止</span>
              <input className="inp" type="date" value={d.due || ""} onChange={(e) => put(r.id, { due: e.target.value || null })} />
              {d.due && <button type="button" className="icon-btn" aria-label="去掉截止日期" onClick={() => put(r.id, { due: null })}><X className="ico" /></button>}
              <span className="td-quick">
                {[0, 1, 7].map((n) => <button key={n} type="button" className="tool" onClick={() => put(r.id, { due: addDays(t0, n) })}>{n === 0 ? "今天" : n === 1 ? "明天" : "下周"}</button>)}
              </span>
            </label>
            <label><span>谁来做</span>
              <input className="inp" value={d.owner || ""} placeholder="名字" list="td-people" onChange={(e) => put(r.id, { owner: e.target.value || null })} />
              {kit.me && d.owner !== kit.me && <button type="button" className="tool" onClick={() => put(r.id, { owner: kit.me })}>我来</button>}
            </label>
            <textarea className="inp td-note" rows={3} placeholder="备注" value={d.note || ""} onChange={(e) => put(r.id, { note: e.target.value })} />
            <div className="td-detail-foot">
              <span className="faint">{r.updated_by ? `${r.updated_by} 最后改的` : ""}</span>
              <button type="button" className="tool td-del" onClick={() => remove(r.id)}><Trash2 className="ico" />删除</button>
            </div>
          </div>
        )}
      </li>
    );
  };
  const people = [...new Set([kit.me, ...all.map((r) => r.data.owner).filter(Boolean) as string[]])].filter(Boolean);
  return (
    <div className="app td">
      <Bar kit={kit} title={title.get()} placeholder="待办清单" onTitle={title.set} />
      <div className="td-main"><div className="td-inner">
        <nav className="td-tabs">
          {([["all", "全部"], ["mine", "我的"], ["today", "今天"], ["star", "重要"]] as [Filter, string][]).map(([f, n]) => (
            <button key={f} type="button" className={`tab${filter === f ? " is-on" : ""}`} onClick={() => setFilter(f)}>{n}{counts[f] ? <span className="td-n">{counts[f]}</span> : null}</button>
          ))}
          <span className="td-date faint">{new Date().getMonth() + 1}月{new Date().getDate()}日 周{WEEK[new Date().getDay()]}</span>
        </nav>
        <form className="td-add" onSubmit={(e) => { e.preventDefault(); add(); }}>
          <Plus className="ico" />
          <input ref={input} value={text} onChange={(e) => setText(e.target.value)} placeholder="添加待办，回车保存（可以写「明天」「周五」「@名字」「!」）" aria-label="添加待办" />
        </form>
        <ul className="td-list">{todo.map(row)}</ul>
        {!todo.length && <div className="td-empty">{filter === "all" ? "都做完了。在上面加一条，或者让 agent 把会议里的待办整理进来。" : "这里没有要做的。"}</div>}
        {done.length > 0 && (
          <>
            <button type="button" className="td-done-head" onClick={() => setShowDone(!showDone)}>{showDone ? <ChevronDown className="ico" /> : <ChevronRight className="ico" />}已完成 {done.length}</button>
            {showDone && <ul className="td-list">{done.map(row)}</ul>}
          </>
        )}
        <datalist id="td-people">{people.map((p) => <option key={p} value={p} />)}</datalist>
      </div></div>
      <Toast kit={kit} />
    </div>
  );
}

/** A to-do's title, edited in place and saved after a pause (or Enter). */
function TitleInput({ value, onSave }: { value: string; onSave: (v: string) => void }) {
  const [draft, setDraft] = useState<string | null>(null);
  const timer = useRef(0);
  const commit = (v: string) => { window.clearTimeout(timer.current); setDraft(null); if (v.trim() && v !== value) onSave(v.trim()); };
  return (
    <input className="td-title" value={draft ?? value} aria-label="待办"
      onChange={(e) => { const v = e.target.value; setDraft(v); window.clearTimeout(timer.current); timer.current = window.setTimeout(() => { if (v.trim() && v !== value) onSave(v.trim()); }, 700); }}
      onBlur={(e) => commit(e.target.value)}
      onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) (e.target as HTMLInputElement).blur(); }} />
  );
}

void mount(kit, App, async () => {
  if (!list.get("main")) await list.put({ title: "" }, "main")?.catch(() => list.load());
});
