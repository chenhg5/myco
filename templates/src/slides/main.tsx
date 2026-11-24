import { t as uiText } from "../_lib/i18n";
// 幻灯片: a deck people and agents make together. Agents write each slide's
// content (layout, title, points, cards, chart, table, notes); the page lays
// it out in the deck's theme. Edit in place, reorder by dragging, present
// full screen, export a PowerPoint file people can keep editing.
import "../_lib/sheet.css";
import "./slides.css";
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { createRoot } from "react-dom/client";
import { ChevronLeft, ChevronRight, Copy, Download, Play, Plus, Trash2, X } from "lucide-react";
import { Store, listen, myco, save as download, type MycoChange } from "../_lib/myco";
import { assignOrders } from "../_lib/order";
import { MenuItem, Popover, rectOf } from "../_lib/ui";
import { LAYOUTS, THEMES, layoutName, starter, type DeckData, type Layout, type SlideData } from "./model";
import { Editable, H, SlideView, W } from "./slide";
import { toMarkdown, toPptx } from "./pptx";
import { registerActions } from "../_lib/register-actions";
import { slidesExport } from "./agent-actions";

const deck = new Store<DeckData & Record<string, unknown>>("deck");
const slides = new Store<SlideData & Record<string, unknown>>("slides");

// ---- following the data -------------------------------------------------------------
let version = 0, pending = 0, failed = false, me = "";
const subs = new Set<() => void>();
const emit = () => { version++; for (const f of subs) f(); };
let note: { text: string; undo?: () => void } | null = null, noteTimer = 0;
function flash(text: string, undo?: () => void) {
  note = { text, undo };
  window.clearTimeout(noteTimer);
  noteTimer = window.setTimeout(() => { note = null; emit(); }, undo ? 6000 : 2600);
  emit();
}
function track(p: Promise<unknown> | null | undefined) {
  if (!p) return;
  pending++;
  emit();
  p.then(() => { pending--; failed = false; emit(); }, (e: { code?: string; message?: string }) => {
    pending--;
    failed = e?.code !== "conflict";
    flash(e?.code === "conflict" ? uiText("别人刚改过，已显示最新的") : e?.message || uiText("没保存上"));
  });
}
const uid = () => "s" + Math.random().toString(36).slice(2, 8);
const meta = (): DeckData => deck.get("main")?.data || {};
function setDeck(patch: Partial<DeckData>) { track(deck.put({ ...meta(), ...patch }, "main")); emit(); }
function update(id: string, patch: Partial<SlideData>) {
  const cur = slides.get(id);
  if (!cur) return;
  track(slides.put({ ...cur.data, ...patch }, id));
  emit();
}
function addSlide(data: SlideData, after?: string): string {
  const list = slides.rows, at = after ? list.findIndex((r) => r.id === after) : list.length - 1;
  const a = list[at]?.data.order, b = list[at + 1]?.data.order;
  const order = typeof a === "number" && typeof b === "number" ? (a + b) / 2 : typeof a === "number" ? a + 1 : typeof b === "number" ? b - 1 : 1;
  const id = uid();
  track(slides.put({ ...data, order }, id));
  emit();
  return id;
}
function removeSlide(id: string) {
  const cur = slides.get(id);
  if (!cur) return;
  const data = cur.data;
  track(slides.remove(id));
  flash(uiText("删除了一页"), () => { track(slides.put(data, id)); note = null; emit(); });
}
function reorder(ids: string[]) {
  const have = new Map(slides.rows.map((r) => [r.id, typeof r.data.order === "number" ? r.data.order : NaN]));
  const orders = assignOrders(ids, have as Map<string, number>);
  ids.forEach((id, i) => { if (have.get(id) !== orders[i]) { const cur = slides.get(id); if (cur) track(slides.put({ ...cur.data, order: orders[i] }, id)); } });
  emit();
}
const useData = () => useSyncExternalStore((f) => { subs.add(f); return () => { subs.delete(f); }; }, () => version);

// ---- the page -------------------------------------------------------------------------
function App() {
  useEffect(() => { const actions = registerActions([slidesExport]); return actions.dispose; }, []);
  useData();
  const list = slides.rows;
  const [cur, setCur] = useState<string>(() => list[0]?.id || "");
  const [present, setPresent] = useState<number | null>(null);
  const [addAt, setAddAt] = useState<ReturnType<typeof rectOf> | null>(null);
  const [exp, setExp] = useState<ReturnType<typeof rectOf> | null>(null);
  const [drag, setDrag] = useState<string | null>(null);
  const [over, setOver] = useState<string | null>(null);
  const index = Math.max(0, list.findIndex((r) => r.id === cur));
  const slide = list[index];
  useEffect(() => { if (!slide && list[0]) setCur(list[list.length - 1].id); }, [slide, list]);
  const d = meta(), title = d.title || "";
  const state = pending ? "saving" : failed ? "error" : "saved";

  // the stage fits the slide
  const stageRef = useRef<HTMLDivElement>(null);
  const [k, setK] = useState(0.5);
  useEffect(() => {
    const el = stageRef.current;
    if (!el) return;
    const fit = () => setK(Math.max(0.1, Math.min((el.clientWidth - 48) / W, (el.clientHeight - 32) / H)));
    fit();
    const ro = new ResizeObserver(fit);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const add = (layout: Layout) => { const id = addSlide(starter(layout), slide?.id); setCur(id); setAddAt(null); };
  const fileName = (title.trim() || "幻灯片").replace(/[\\/:*?"<>|]/g, " ");
  const exportAs = async (kind: "pptx" | "md") => {
    setExp(null);
    (document.activeElement as HTMLElement | null)?.blur?.();
    const data = list.map((r) => r.data);
    try {
      if (kind === "pptx") await download(`${fileName}.pptx`, await toPptx(title || "幻灯片", d.theme, data), "application/vnd.openxmlformats-officedocument.presentationml.presentation");
      else await download(`${fileName}.md`, toMarkdown(title, data), "text/markdown");
      flash(uiText("已存到「下载」"));
    } catch (e) {
      flash((e as Error).message || uiText("没存下来"));
    }
  };

  const go = useCallback((n: number) => { const r = slides.rows[n]; if (r) setCur(r.id); }, []);
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (present !== null || t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName)) return;
      if (e.key === "ArrowDown" || e.key === "PageDown") { e.preventDefault(); go(index + 1); }
      else if (e.key === "ArrowUp" || e.key === "PageUp") { e.preventDefault(); go(index - 1); }
      else if ((e.key === "Delete" || e.key === "Backspace") && slide && t.closest(".thumbs")) { e.preventDefault(); removeSlide(slide.id); }
      else if (e.key === "F5" || (e.key === "Enter" && (e.metaKey || e.ctrlKey))) { e.preventDefault(); setPresent(index); }
    };
    document.addEventListener("keydown", k);
    return () => document.removeEventListener("keydown", k);
  }, [index, present, slide, go]);

  return (
    <div className="app sd">
      <header className="top">
        <Editable className="top-title sd-title" value={title} placeholder={uiText("未命名幻灯片")} editable onSave={(v) => setDeck({ title: v.trim() })} />
        <span className={`top-state is-${state}`}>{state === "saving" ? uiText("正在保存…") : state === "error" ? uiText("没保存上") : uiText("已保存")}</span>
        <span className="top-gap" />
        {me && <span className="top-me">{uiText("你是")}{" "}{me}</span>}
        <button type="button" className="btn" onClick={(e) => setExp(rectOf(e.currentTarget))}><Download className="ico" />{uiText("导出")}</button>
        <button type="button" className="btn btn-primary" onClick={() => setPresent(index)} disabled={!list.length}><Play className="ico" />{uiText("放映")}</button>
        {exp && (
          <Popover at={exp} align="right" onClose={() => setExp(null)}>
            <div className="menu">
              <MenuItem onClick={() => void exportAs("pptx")}>{uiText("PowerPoint（.pptx，能接着改）")}</MenuItem>
              <MenuItem onClick={() => void exportAs("md")}>{uiText("Markdown 大纲（.md）")}</MenuItem>
            </div>
          </Popover>
        )}
      </header>
      <div className="sd-main">
        <nav className="thumbs" aria-label={uiText("所有页")}>
          {list.map((r, i) => (
            <div key={r.id} className={`thumb${r.id === slide?.id ? " is-cur" : ""}${over === r.id && drag !== r.id ? " is-over" : ""}`}
              draggable onDragStart={(e) => { e.dataTransfer.setData("text/plain", r.id); setDrag(r.id); }}
              onDragEnd={() => { setDrag(null); setOver(null); }}
              onDragOver={(e) => { e.preventDefault(); setOver(r.id); }}
              onDrop={(e) => {
                e.preventDefault();
                if (!drag || drag === r.id) return;
                const ids = list.map((x) => x.id).filter((x) => x !== drag);
                ids.splice(ids.indexOf(r.id) + (list.findIndex((x) => x.id === drag) < i ? 1 : 0), 0, drag);
                reorder(ids);
                setDrag(null);
                setOver(null);
              }}>
              <span className="thumb-n">{i + 1}</span>
              <button type="button" className="thumb-face" onClick={() => setCur(r.id)} aria-label={uiText("第 {0} 页：{1}", i + 1, r.data.title || layoutName(r.data.layout))} aria-current={r.id === slide?.id}>
                <div className="thumb-in" style={{ transform: `scale(${152 / W})` }}>
                  <SlideView s={r.data} deckTitle={title} theme={d.theme} n={i + 1} total={list.length} editable={false} />
                </div>
              </button>
              <span className="thumb-acts">
                <button type="button" title={uiText("复制这一页")} aria-label={uiText("复制这一页")} onClick={() => { const { order: _o, ...rest } = r.data; setCur(addSlide(rest, r.id)); }}><Copy className="ico" /></button>
                <button type="button" title={uiText("删除这一页")} aria-label={uiText("删除这一页")} onClick={() => removeSlide(r.id)}><Trash2 className="ico" /></button>
              </span>
            </div>
          ))}
          <button type="button" className="thumb-add" onClick={(e) => setAddAt(rectOf(e.currentTarget))}><Plus className="ico" />{uiText("新建页")}</button>
        </nav>
        <section className="stage" ref={stageRef}>
          {slide ? (
            <div className="stage-slide" style={{ width: W * k, height: H * k }}>
              <div style={{ transform: `scale(${k})`, transformOrigin: "0 0" }}>
                <SlideView key={slide.id} s={slide.data} deckTitle={title} theme={d.theme} n={index + 1} total={list.length} editable save={(p) => update(slide.id, p)} />
              </div>
            </div>
          ) : (
            <div className="stage-empty">
              <p>{uiText("还没有内容。新建一页，或者让 agent 按大纲写：「用这个幻灯片做一份 Q4 发布会的 PPT，8 页左右」。")}</p>
              <button type="button" className="btn btn-primary" onClick={() => add("cover")}><Plus className="ico" />{uiText("新建封面")}</button>
            </div>
          )}
          {slide && (
            <textarea className="notes" placeholder={uiText("演讲者备注（放映时不显示，导出到 PPT 的备注里）")} value={slide.data.notes || ""} aria-label={uiText("演讲者备注")}
              onChange={(e) => update(slide.id, { notes: e.target.value })} />
          )}
        </section>
        <aside className="side">
          {slide && <Side id={slide.id} s={slide.data} />}
          <h4>{uiText("主题")}</h4>
          <div className="themes">
            {Object.entries(THEMES).map(([key, t]) => (
              <button key={key} type="button" className={`theme${(d.theme || "light") === key ? " is-on" : ""}`} onClick={() => setDeck({ theme: key })}
                style={{ background: t.coverBg, color: t.coverFg }} aria-pressed={(d.theme || "light") === key}>
                <span className="theme-dot" style={{ background: t.accent }} />{t.name}
              </button>
            ))}
          </div>
        </aside>
      </div>
      {addAt && (
        <Popover at={addAt} onClose={() => setAddAt(null)} width={300}>
          <div className="layouts in-pop">
            {LAYOUTS.map(([l, name]) => <button key={l} type="button" className="layout" onClick={() => add(l)}><LayoutIcon l={l} /><span>{name}</span></button>)}
          </div>
        </Popover>
      )}
      {present !== null && <Present start={present} theme={d.theme} title={title} onClose={(n) => { setPresent(null); go(n); }} />}
      {note && <div className="note" role="status">{note.text}{note.undo && <button type="button" onClick={note.undo}>{uiText("撤销")}</button>}</div>}
    </div>
  );
}

function LayoutIcon({ l }: { l: Layout }) {
  const bar = (w: string, h = 3, extra = {}) => <i style={{ width: w, height: h, ...extra }} />;
  return (
    <span className={`lay-ico is-${l}`}>
      {l === "cover" && <>{bar("20%", 2, { background: "var(--fg)" })}{bar("60%", 5)}{bar("40%")}</>}
      {l === "section" && <>{bar("15%", 4, { background: "var(--fg)" })}{bar("55%", 5)}</>}
      {l === "end" && bar("40%", 6, { margin: "auto" })}
      {l === "quote" && <>{bar("70%", 5)}{bar("30%")}</>}
      {l === "points" && <>{bar("45%", 4)}{bar("80%")}{bar("70%")}{bar("75%")}</>}
      {l === "two" && <>{bar("45%", 4)}<span className="c2"><span>{bar("90%")}{bar("80%")}</span><b /></span></>}
      {l === "cards" && <>{bar("45%", 4)}<span className="c3"><b /><b /><b /></span></>}
      {l === "chart" && <>{bar("45%", 4)}<span className="bars"><b style={{ height: "40%" }} /><b style={{ height: "65%" }} /><b style={{ height: "90%" }} /></span></>}
      {l === "table" && <>{bar("45%", 4)}<span className="grid3"><b /><b /><b /><b /><b /><b /></span></>}
    </span>
  );
}

/** The panel on the right: this slide's layout, and its chart, table or cards. */
function Side({ id, s }: { id: string; s: SlideData }) {
  const layout = s.layout || "points";
  const ch = s.chart || {}, labels = ch.labels || [], series = ch.series?.length ? ch.series : [{ name: "数值", values: [] }];
  const setChart = (c: typeof ch) => update(id, { chart: c });
  const cards = s.cards || [], tb = s.table || {}, cols = tb.columns || [], rows = tb.rows || [];
  return (
    <>
      <h4>{uiText("版式")}</h4>
      <div className="layouts">
        {LAYOUTS.map(([l, name]) => (
          <button key={l} type="button" className={`layout${layout === l ? " is-on" : ""}`} title={name} aria-pressed={layout === l}
            onClick={() => {
              const base = starter(l), patch: Partial<SlideData> = { layout: l };
              // (what the new layout needs and this slide does not have yet)
              if ((l === "two" || l === "chart") && !s.chart) patch.chart = base.chart;
              if (l === "cards" && !s.cards?.length) patch.cards = base.cards;
              if (l === "table" && !s.table?.columns?.length) patch.table = base.table;
              if ((l === "points" || l === "two") && !s.points?.length) patch.points = base.points;
              if (l === "quote" && !s.body) patch.body = s.points?.[0] || s.title || base.body;
              update(id, patch);
            }}>
            <LayoutIcon l={l} /><span>{name}</span>
          </button>
        ))}
      </div>
      {(layout === "two" || layout === "chart") && (
        <>
          <h4>{uiText("图表")}</h4>
          <div className="seg sd-seg">
            {([["bar", "柱状"], ["line", "折线"], ["pie", "饼图"]] as const).map(([t, n]) => (
              <button key={t} type="button" className={(ch.type || "bar") === t ? "is-on" : ""} onClick={() => setChart({ ...ch, type: t })}>{n}</button>
            ))}
          </div>
          <table className="data">
            <thead><tr><th />{series.map((x, si) => (
              <th key={si}><input className="inp" value={x.name || ""} placeholder={uiText("系列{0}", si + 1)} aria-label={uiText("系列名")}
                onChange={(e) => setChart({ ...ch, series: series.map((y, j) => (j === si ? { ...y, name: e.target.value } : y)) })} /></th>
            ))}{ch.type !== "pie" && series.length < 3 && <th><button type="button" className="icon-btn" title={uiText("加一个系列")} aria-label={uiText("加一个系列")}
              onClick={() => setChart({ ...ch, series: [...series, { name: `系列${series.length + 1}`, values: labels.map(() => 0) }] })}><Plus className="ico" /></button></th>}</tr></thead>
            <tbody>{labels.map((l, i) => (
              <tr key={i}>
                <td><input className="inp" value={l} aria-label={uiText("类别")} onChange={(e) => setChart({ ...ch, labels: labels.map((x, j) => (j === i ? e.target.value : x)) })} /></td>
                {series.map((x, si) => (
                  <td key={si}><input className="inp num" inputMode="decimal" value={x.values?.[i] ?? ""} aria-label={uiText("数值")}
                    onChange={(e) => {
                      const v = e.target.value.trim() === "" ? null : Number(e.target.value);
                      setChart({ ...ch, series: series.map((y, j) => (j === si ? { ...y, values: labels.map((_, k) => (k === i ? (v !== null && isFinite(v) ? v : null) : y.values?.[k] ?? null)) } : y)) });
                    }} /></td>
                ))}
                <td><button type="button" className="icon-btn" aria-label={uiText("删掉这一行")} onClick={() => setChart({ ...ch, labels: labels.filter((_, j) => j !== i), series: series.map((y) => ({ ...y, values: (y.values || []).filter((_, j) => j !== i) })) })}><X className="ico" /></button></td>
              </tr>
            ))}</tbody>
          </table>
          <button type="button" className="link-btn" onClick={() => setChart({ ...ch, labels: [...labels, `类别${labels.length + 1}`], series: series.map((y) => ({ ...y, values: [...labels.map((_, k) => y.values?.[k] ?? null), 0] })) })}>
            <Plus className="ico" />{uiText("加一行")}{" "}</button>
        </>
      )}
      {layout === "cards" && (
        <>
          <h4>{uiText("卡片")}</h4>
          <div className="row-btns">
            <button type="button" className="btn" disabled={cards.length >= 4} onClick={() => update(id, { cards: [...cards, { title: "", text: "" }] })}><Plus className="ico" />{uiText("加一张")}</button>
            <button type="button" className="btn" disabled={cards.length <= 1} onClick={() => update(id, { cards: cards.slice(0, -1) })}>{uiText("去掉最后一张")}</button>
          </div>
        </>
      )}
      {layout === "table" && (
        <>
          <h4>{uiText("表格")}</h4>
          <div className="row-btns">
            <button type="button" className="btn" onClick={() => update(id, { table: { columns: cols, rows: [...rows, cols.map(() => "")] } })}><Plus className="ico" />{uiText("加一行")}</button>
            <button type="button" className="btn" disabled={cols.length >= 8} onClick={() => update(id, { table: { columns: [...cols, `列${cols.length + 1}`], rows: rows.map((r) => [...r, ""]) } })}><Plus className="ico" />{uiText("加一列")}</button>
            <button type="button" className="btn" disabled={!rows.length} onClick={() => update(id, { table: { columns: cols, rows: rows.slice(0, -1) } })}>{uiText("去掉最后一行")}</button>
            <button type="button" className="btn" disabled={cols.length <= 1} onClick={() => update(id, { table: { columns: cols.slice(0, -1), rows: rows.map((r) => r.slice(0, cols.length - 1)) } })}>{uiText("去掉最后一列")}</button>
          </div>
        </>
      )}
    </>
  );
}

/** Full screen: arrows, space and clicks move on; Esc ends. */
function Present({ start, theme, title, onClose }: { start: number; theme?: string; title: string; onClose: (n: number) => void }) {
  const [n, setN] = useState(start);
  const [size, setSize] = useState({ w: window.innerWidth, h: window.innerHeight });
  const ref = useRef<HTMLDivElement>(null);
  const list = slides.rows;
  useEffect(() => {
    if (document.fullscreenEnabled) ref.current?.requestFullscreen?.().catch(() => undefined);
    const r = () => setSize({ w: window.innerWidth, h: window.innerHeight });
    window.addEventListener("resize", r);
    const fs = () => { if (!document.fullscreenElement) onCloseRef.current(nRef.current); };
    document.addEventListener("fullscreenchange", fs);
    return () => { window.removeEventListener("resize", r); document.removeEventListener("fullscreenchange", fs); if (document.fullscreenElement) void document.exitFullscreen().catch(() => undefined); };
  }, []);
  const nRef = useRef(n); nRef.current = n;
  const onCloseRef = useRef(onClose); onCloseRef.current = onClose;
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (["ArrowRight", "ArrowDown", "PageDown", " ", "Enter"].includes(e.key)) { e.preventDefault(); setN((x) => Math.min(list.length - 1, x + 1)); }
      else if (["ArrowLeft", "ArrowUp", "PageUp", "Backspace"].includes(e.key)) { e.preventDefault(); setN((x) => Math.max(0, x - 1)); }
      else if (e.key === "Home") setN(0);
      else if (e.key === "End") setN(list.length - 1);
      else if (e.key === "Escape") { e.preventDefault(); onClose(n); }
    };
    document.addEventListener("keydown", k);
    return () => document.removeEventListener("keydown", k);
  }, [list.length, n, onClose]);
  const k = Math.min(size.w / W, size.h / H), s = list[n];
  return (
    <div className="present" ref={ref} onClick={() => setN((x) => Math.min(list.length - 1, x + 1))} role="dialog" aria-label={uiText("放映")}>
      {s && <div style={{ width: W * k, height: H * k }}><div style={{ transform: `scale(${k})`, transformOrigin: "0 0" }}>
        <SlideView s={s.data} deckTitle={title} theme={theme} n={n + 1} total={list.length} editable={false} />
      </div></div>}
      <div className="present-bar" onClick={(e) => e.stopPropagation()}>
        <button type="button" aria-label={uiText("上一页")} onClick={() => setN((x) => Math.max(0, x - 1))}><ChevronLeft className="ico" /></button>
        <span>{n + 1} / {list.length}</span>
        <button type="button" aria-label={uiText("下一页")} onClick={() => setN((x) => Math.min(list.length - 1, x + 1))}><ChevronRight className="ico" /></button>
        <button type="button" aria-label={uiText("结束放映")} onClick={() => onClose(n)}><X className="ico" /></button>
      </div>
    </div>
  );
}

async function start() {
  const root = createRoot(document.getElementById("root")!);
  const outside = <div className="outside">{uiText("在 myco App 里打开这个网页，才能读写它的内容。")}</div>;
  if (!myco) { root.render(outside); return; }
  try {
    const ctx = await myco.ready;
    me = ctx.me?.name || "";
    await Promise.all([deck.load(), slides.load()]);
    if (!ctx.render && !deck.get("main")) await deck.put({ title: "", theme: "light" }, "main")?.catch(() => deck.load());
  } catch { root.render(outside); return; }
  listen([deck, slides], (c: MycoChange) => { emit(); if (c.by && c.by !== me) flash(uiText("{0} 刚改了", c.by)); }, emit);
  root.render(<App />);
}
void start();
