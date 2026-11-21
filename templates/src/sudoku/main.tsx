import { t as uiText } from "../_lib/i18n";
// 数独: a puzzle everyone fills together. The puzzle (its given digits) is
// one record; each filled cell is a record of its own, so two people
// filling different cells never clash. Conflicts show in red; the page
// makes new puzzles (one answer each) at three levels.
import "../_lib/sheet.css";
import "./sudoku.css";
import { useEffect, useMemo, useState } from "react";
import { Eraser, Lightbulb, RotateCcw, Sparkles } from "lucide-react";
import { Store } from "../_lib/myco";
import { Bar, Kit, Toast, mount, titleSaver, useKit } from "../_lib/appkit";
import { LEVELS, boxOf, colOf, make, parse, peers, rowOf, show, solve } from "./sudoku";
import { personColor } from "../_lib/cells";

type Puzzle = { title?: string; givens?: string; level?: string; round?: number };
type Cell = { i?: number; v?: number | null; round?: number };
const puzzle = new Store<Puzzle & Record<string, unknown>>("puzzle");
const cells = new Store<Cell & Record<string, unknown>>("cells");
const kit = new Kit([puzzle, cells], { quiet: true });
const title = titleSaver(kit, puzzle);

function App() {
  useKit(kit);
  const p = puzzle.get("main")?.data || {};
  const round = p.round || 1;
  const givens = useMemo(() => parse(p.givens || ""), [p.givens]);
  const [at, setAt] = useState(() => Math.max(0, givens.indexOf(0)));
  const [level, setLevel] = useState(p.level || "medium");
  const filled = useMemo(() => {
    const m = new Map<number, { v: number; by?: string }>();
    for (const r of cells.rows) if ((r.data.round ?? 1) === round && typeof r.data.i === "number" && r.data.v) m.set(r.data.i, { v: Number(r.data.v), by: r.updated_by });
    return m;
  }, [kit.version, round]); // eslint-disable-line react-hooks/exhaustive-deps
  const value = (i: number) => givens[i] || filled.get(i)?.v || 0;
  const bad = useMemo(() => {
    const s = new Set<number>();
    for (let i = 0; i < 81; i++) { const v = value(i); if (v && peers(i).some((j) => value(j) === v)) s.add(i); }
    return s;
  }, [givens, filled]); // eslint-disable-line react-hooks/exhaustive-deps
  const done = givens.some(Boolean) && Array.from({ length: 81 }, (_, i) => value(i)).every(Boolean) && !bad.size;
  const people = useMemo(() => { const m = new Map<string, number>(); for (const f of filled.values()) if (f.by) m.set(f.by, (m.get(f.by) || 0) + 1); return [...m].sort((a, b) => b[1] - a[1]); }, [filled]);

  const write = (i: number, v: number | null) => {
    if (givens[i] || done) return;
    kit.track(cells.put({ i, v, round }, `r${round}-${i}`));
    kit.emit();
  };
  const fresh = (level: string) => {
    if (filled.size && !done && !window.confirm(uiText("换一道新题？这道题填的会清掉。"))) return;
    const g = make(level);
    kit.track(puzzle.put({ ...(puzzle.get("main")?.data || {}), givens: show(g), level, round: round + 1 }, "main"));
    setAt(g.indexOf(0));
    kit.emit();
  };
  const hint = () => {
    const sol = solve(givens);
    if (!sol) { kit.flash(uiText("这道题没有答案")); return; }
    const i = !value(at) ? at : sol.findIndex((_, j) => !value(j));
    if (i < 0) return;
    write(i, sol[i]);
    setAt(i);
  };
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).tagName === "INPUT") return;
      if (/^[1-9]$/.test(e.key)) { write(at, Number(e.key)); return; }
      if (e.key === "Backspace" || e.key === "Delete" || e.key === "0") { write(at, null); return; }
      const d = { ArrowUp: -9, ArrowDown: 9, ArrowLeft: -1, ArrowRight: 1 }[e.key];
      if (d) { e.preventDefault(); setAt((x) => Math.min(80, Math.max(0, x + d))); }
    };
    document.addEventListener("keydown", k);
    return () => document.removeEventListener("keydown", k);
  });
  const sel = value(at);
  const left = 81 - Array.from({ length: 81 }, (_, i) => value(i)).filter(Boolean).length;
  return (
    <div className="app sd2">
      <Bar kit={kit} title={title.get()} placeholder={uiText("数独")} onTitle={title.set}>
        <button type="button" className="btn" onClick={hint} disabled={done}><Lightbulb className="ico" />{uiText("提示一格")}</button>
        <select className="sel" value={level} onChange={(e) => setLevel(e.target.value)} aria-label={uiText("难度")}>
          {LEVELS.map(([l, name]) => <option key={l} value={l}>{name}</option>)}
        </select>
        <button type="button" className="btn btn-primary" onClick={() => fresh(level)}><RotateCcw className="ico" />{uiText("换一道")}</button>
      </Bar>
      <div className="su-main">
        <div className="su-board" role="grid" aria-label={uiText("数独")}>
          {Array.from({ length: 81 }, (_, i) => {
            const v = value(i), f = filled.get(i);
            const same = sel && v === sel, near = rowOf(i) === rowOf(at) || colOf(i) === colOf(at) || boxOf(i) === boxOf(at);
            return (
              <button key={i} type="button" role="gridcell" aria-label={uiText("第 {0} 行第 {1} 列{2}", rowOf(i) + 1, colOf(i) + 1, v ? `：${v}` : "")}
                className={`su-cell${givens[i] ? " is-given" : ""}${i === at ? " is-at" : near ? " is-near" : ""}${same ? " is-same" : ""}${bad.has(i) ? " is-bad" : ""}${colOf(i) % 3 === 2 && colOf(i) < 8 ? " r3" : ""}${rowOf(i) % 3 === 2 && rowOf(i) < 8 ? " b3" : ""}`}
                onClick={() => setAt(i)} title={f?.by ? uiText("{0} 填的", f.by) : undefined}
                style={f?.by && f.by !== kit.me ? { color: personColor(f.by) } : undefined}>
                {v || ""}
              </button>
            );
          })}
        </div>
        <aside className="su-side">
          <div className={`su-status${done ? " is-done" : ""}`}>
            {done ? <><Sparkles className="ico" />{uiText("全部填对了！")}</> : !givens.some(Boolean) ? uiText("还没有题：点上面的「换一道」") : uiText("还差 {0} 格{1}", left, bad.size ? `，有 ${bad.size} 格冲突` : "")}
          </div>
          <div className="su-pad">
            {[1, 2, 3, 4, 5, 6, 7, 8, 9].map((n) => {
              const used = Array.from({ length: 81 }, (_, i) => value(i)).filter((x) => x === n).length;
              return <button key={n} type="button" disabled={done || used >= 9} onClick={() => write(at, n)}>{n}<small>{9 - used}</small></button>;
            })}
            <button type="button" className="su-erase" onClick={() => write(at, null)} disabled={done}><Eraser className="ico" />{uiText("擦掉")}</button>
          </div>
          {people.length > 0 && (
            <div className="su-people">
              <div className="faint">{uiText("谁填了几格")}</div>
              {people.map(([name, n]) => <div key={name} className="su-person"><span className="person-dot" style={{ background: personColor(name) }}>{name.slice(0, 1).toUpperCase()}</span>{name}{name === kit.me ? uiText("（你）") : ""}<b>{n}</b></div>)}
            </div>
          )}
          <p className="su-help faint">{uiText("点一格，按数字键或下面的数字填；方向键移动，退格擦掉。别人填的格子是他的颜色。")}</p>
        </aside>
      </div>
      <Toast kit={kit} />
    </div>
  );
}

void mount(kit, App, async () => {
  if (!puzzle.get("main")?.data.givens) await puzzle.put({ title: "数独", givens: show(make("easy")), level: "easy", round: 1 }, "main")?.catch(() => puzzle.load());
});
