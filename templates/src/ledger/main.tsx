// 记账本: income and spending, a record an entry. A month at a time: what
// came in, what went out, what is left of the budget, where it went (by
// category), and the entries day by day. Agents add entries from receipts
// or a chat ("午饭 32").
import "../_lib/sheet.css";
import "./ledger.css";
import { useMemo, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, Download, Trash2 } from "lucide-react";
import { Store, save } from "../_lib/myco";
import { Bar, Kit, Toast, mount, titleSaver, uid, useKit } from "../_lib/appkit";
import { csv } from "../_lib/export";
import { dayName, today } from "../_lib/dates";

type Entry = { kind?: "out" | "in"; amount?: number; category?: string; note?: string; date?: string };
type Book = { title?: string; currency?: string; budget?: number | null };
const book = new Store<Book & Record<string, unknown>>("book");
const entries = new Store<Entry & Record<string, unknown>>("entries");
const kit = new Kit([book, entries]);
const title = titleSaver(kit, book);

const OUT = ["餐饮", "交通", "购物", "住房", "日用", "娱乐", "医疗", "学习", "人情", "其他"];
const IN = ["工资", "奖金", "报销", "理财", "其他"];
const EMOJI: Record<string, string> = { 餐饮: "🍜", 交通: "🚇", 购物: "🛍️", 住房: "🏠", 日用: "🧻", 娱乐: "🎬", 医疗: "💊", 学习: "📚", 人情: "🎁", 其他: "📦", 工资: "💰", 奖金: "🎉", 报销: "🧾", 理财: "📈" };
// grays from strong to faint (ledger.css; reversed in the dark)
const SHADES = Array.from({ length: 10 }, (_, i) => `var(--shade-${i})`);

function App() {
  useKit(kit);
  const b = book.get("main")?.data || {};
  const cur = b.currency || "¥";
  const [month, setMonth] = useState(today().slice(0, 7));
  const [kind, setKind] = useState<"out" | "in">("out");
  const [amount, setAmount] = useState("");
  const [cat, setCat] = useState("餐饮");
  const [note, setNote] = useState("");
  const [date, setDate] = useState(today());
  const amt = useRef<HTMLInputElement>(null);
  const money = (n: number) => `${cur}${n.toLocaleString("zh-CN", { minimumFractionDigits: n % 1 ? 2 : 0, maximumFractionDigits: 2 })}`;
  const list = useMemo(() => entries.rows.filter((r) => String(r.data.date || "").startsWith(month))
    .sort((a, b2) => String(b2.data.date).localeCompare(String(a.data.date))), [kit.version, month]); // eslint-disable-line react-hooks/exhaustive-deps
  const sum = (k: "in" | "out") => list.filter((r) => (r.data.kind || "out") === k).reduce((n, r) => n + (Number(r.data.amount) || 0), 0);
  const out = sum("out"), inc = sum("in");
  const byCat = useMemo(() => {
    const m = new Map<string, number>();
    for (const r of list) if ((r.data.kind || "out") === "out") m.set(r.data.category || "其他", (m.get(r.data.category || "其他") || 0) + (Number(r.data.amount) || 0));
    return [...m].sort((a, c) => c[1] - a[1]);
  }, [list]);
  const days = useMemo(() => {
    const m = new Map<string, typeof list>();
    for (const r of list) { const d = String(r.data.date); if (!m.has(d)) m.set(d, []); m.get(d)!.push(r); }
    return [...m];
  }, [list]);
  const go = (n: number) => { const [y, mo] = month.split("-").map(Number); const d = new Date(y, mo - 1 + n, 1); setMonth(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`); };
  const add = () => {
    const n = Number(amount.replace(/[,，¥$\s]/g, ""));
    if (!isFinite(n) || n <= 0) { kit.flash("金额要是正数"); amt.current?.focus(); return; }
    kit.track(entries.put({ kind, amount: Math.round(n * 100) / 100, category: cat, note: note.trim(), date }, uid("e")));
    kit.emit();
    setAmount(""); setNote("");
    amt.current?.focus();
    if (!date.startsWith(month)) setMonth(date.slice(0, 7));
  };
  const remove = (id: string) => {
    const r = entries.get(id);
    if (!r) return;
    const data = r.data;
    kit.track(entries.remove(id));
    kit.flash("删除了一笔", () => { kit.track(entries.put(data, id)); kit.note = null; kit.emit(); });
  };
  const exportCsv = async () => {
    const rows = [["日期", "收支", "分类", "金额", "备注"], ...entries.rows.slice().sort((a, c) => String(a.data.date).localeCompare(String(c.data.date)))
      .map((r) => [r.data.date, r.data.kind === "in" ? "收入" : "支出", r.data.category, r.data.amount, r.data.note || ""])];
    try { await save(`${title.get() || "记账本"}.csv`, csv(rows), "text/csv"); kit.flash("已存到「下载」"); } catch (e) { kit.flash((e as Error).message || "没存下来"); }
  };
  const budget = typeof b.budget === "number" && b.budget > 0 ? b.budget : null;
  const [y, mo] = month.split("-").map(Number);
  // the donut
  let a0 = -Math.PI / 2;
  const arcs = byCat.slice(0, 10).map(([c, v], i) => {
    const a1 = a0 + (v / (out || 1)) * Math.PI * 2, large = a1 - a0 > Math.PI ? 1 : 0, R = 70, r = 46;
    const p = (a: number, rr: number) => `${80 + rr * Math.cos(a)} ${80 + rr * Math.sin(a)}`;
    const d = byCat.length === 1 ? `M 10 80 A 70 70 0 1 1 150 80 A 70 70 0 1 1 10 80 M 34 80 A 46 46 0 1 0 126 80 A 46 46 0 1 0 34 80`
      : `M ${p(a0, R)} A ${R} ${R} 0 ${large} 1 ${p(a1, R)} L ${p(a1, r)} A ${r} ${r} 0 ${large} 0 ${p(a0, r)} Z`;
    a0 = a1;
    return <path key={c} d={d} style={{ fill: SHADES[i] }} fillRule="evenodd" />;
  });
  return (
    <div className="app lg">
      <Bar kit={kit} title={title.get()} placeholder="记账本" onTitle={title.set}>
        <button type="button" className="btn" onClick={() => void exportCsv()}><Download className="ico" />导出</button>
      </Bar>
      <div className="lg-main"><div className="lg-inner">
        <div className="lg-month">
          <button type="button" className="icon-btn" aria-label="上个月" onClick={() => go(-1)}><ChevronLeft className="ico" /></button>
          <b>{y} 年 {mo} 月</b>
          <button type="button" className="icon-btn" aria-label="下个月" onClick={() => go(1)}><ChevronRight className="ico" /></button>
          <span className="top-gap" />
          <label className="lg-budget">月预算 <input className="inp" inputMode="decimal" defaultValue={budget ?? ""} placeholder="不设"
            onBlur={(e) => { const n = Number(e.target.value); kit.track(book.put({ ...(book.get("main")?.data || {}), budget: e.target.value.trim() && isFinite(n) && n > 0 ? n : null }, "main")); }} /></label>
        </div>
        <div className="lg-cards">
          <div className="lg-card"><span>支出</span><b>{money(out)}</b></div>
          <div className="lg-card"><span>收入</span><b className="is-in">{money(inc)}</b></div>
          <div className="lg-card"><span>结余</span><b>{money(inc - out)}</b></div>
          <div className="lg-card"><span>预算剩余</span><b className={budget && out > budget ? "is-over" : ""}>{budget ? money(budget - out) : "—"}</b>
            {budget && <i className="lg-meter"><i style={{ width: `${Math.min(100, (out / budget) * 100)}%` }} className={out > budget ? "is-over" : ""} /></i>}</div>
        </div>
        <form className="lg-add" onSubmit={(e) => { e.preventDefault(); add(); }}>
          <div className="seg">
            <button type="button" className={kind === "out" ? "is-on" : ""} onClick={() => { setKind("out"); setCat("餐饮"); }}>支出</button>
            <button type="button" className={kind === "in" ? "is-on" : ""} onClick={() => { setKind("in"); setCat("工资"); }}>收入</button>
          </div>
          <input ref={amt} className="inp lg-amount" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="金额" aria-label="金额" />
          <input className="inp lg-note" value={note} onChange={(e) => setNote(e.target.value)} placeholder="备注（午饭、打车…）" aria-label="备注" />
          <input className="inp" type="date" value={date} onChange={(e) => setDate(e.target.value || today())} aria-label="日期" />
          <button type="submit" className="btn btn-primary">记一笔</button>
          <div className="lg-cats">
            {(kind === "out" ? OUT : IN).map((c) => <button key={c} type="button" className={`lg-cat${cat === c ? " is-on" : ""}`} onClick={() => setCat(c)}><span>{EMOJI[c]}</span>{c}</button>)}
          </div>
        </form>
        <div className="lg-body">
          <section className="lg-days">
            {!days.length && <div className="lg-empty">这个月还没有记录。在上面记一笔，或者让 agent 把账单整理进来。</div>}
            {days.map(([d, rs]) => {
              const o = rs.filter((r) => (r.data.kind || "out") === "out").reduce((n, r) => n + (Number(r.data.amount) || 0), 0);
              const i = rs.filter((r) => r.data.kind === "in").reduce((n, r) => n + (Number(r.data.amount) || 0), 0);
              return (
                <div key={d} className="lg-day">
                  <div className="lg-day-head"><span>{dayName(d)}{dayName(d).includes("月") ? null : <span className="faint"> · {d.slice(5).replace("-", "/")}</span>}</span><span className="faint">{o ? `支出 ${money(o)}` : ""}{o && i ? " · " : ""}{i ? `收入 ${money(i)}` : ""}</span></div>
                  {rs.map((r) => (
                    <div key={r.id} className="lg-row">
                      <span className="lg-emoji">{EMOJI[r.data.category || "其他"] || "📦"}</span>
                      <span className="lg-what"><b>{r.data.category || "其他"}</b>{r.data.note ? <span className="faint">{r.data.note}</span> : null}</span>
                      {r.updated_by && <span className="faint lg-by">{r.updated_by}</span>}
                      <span className={`lg-amt${r.data.kind === "in" ? " is-in" : ""}`}>{r.data.kind === "in" ? "+" : "−"}{money(Number(r.data.amount) || 0)}</span>
                      <button type="button" className="icon-btn lg-del" aria-label="删除这一笔" onClick={() => remove(r.id)}><Trash2 className="ico" /></button>
                    </div>
                  ))}
                </div>
              );
            })}
          </section>
          {byCat.length > 0 && (
            <aside className="lg-chart">
              <div className="lg-chart-head">支出分类</div>
              <svg viewBox="0 0 160 160" className="lg-donut">{arcs}<text x="80" y="76" textAnchor="middle" className="lg-donut-k">支出</text><text x="80" y="96" textAnchor="middle" className="lg-donut-v">{money(out)}</text></svg>
              <ul>
                {byCat.map(([c, v], i) => <li key={c}><i style={{ background: SHADES[Math.min(i, 9)] }} />{c}<span className="faint">{Math.round((v / (out || 1)) * 100)}%</span><b>{money(v)}</b></li>)}
              </ul>
            </aside>
          )}
        </div>
      </div></div>
      <Toast kit={kit} />
    </div>
  );
}

void mount(kit, App, async () => {
  if (!book.get("main")) await book.put({ title: "", currency: "¥" }, "main")?.catch(() => book.load());
});
