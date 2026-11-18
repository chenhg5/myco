// 白板: an endless canvas people and agents fill together: sticky notes,
// text, rectangles, ellipses, and arrows joining them. Each thing is a
// record (where it is, how big, its text and colour); agents add stickies
// with the CLI and they appear at once.
import "../_lib/sheet.css";
import "./wb.css";
import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent as RPE } from "react";
import { ArrowUpRight, Circle, Download, Hand, Maximize, Minus, MousePointer2, Plus, Square, StickyNote, Trash2, Type } from "lucide-react";
import { Store, save } from "../_lib/myco";
import { Bar, Kit, Toast, mount, titleSaver, uid, useKit } from "../_lib/appkit";

type Kind = "sticky" | "text" | "rect" | "ellipse" | "arrow";
type Item = { type?: Kind; x?: number; y?: number; w?: number; h?: number; text?: string; color?: string; from?: string; to?: string; z?: number };
type Box = { id: string; type: Kind; x: number; y: number; w: number; h: number; text: string; color: string; z: number };

const board = new Store<{ title?: string }>("board");
const items = new Store<Item & Record<string, unknown>>("items");
const kit = new Kit([board, items]);
const title = titleSaver(kit, board);

const STICKY: Record<string, [string, string]> = { // name → [light, dark]
  yellow: ["#fff3b0", "#5c5222"], pink: ["#ffd6e0", "#5c2f3b"], blue: ["#d4e6ff", "#26405e"],
  green: ["#d6f5d6", "#2a4d2c"], purple: ["#e6dcff", "#3f3361"], orange: ["#ffe1c2", "#5c3d20"],
};
const STROKE: Record<string, string> = { yellow: "#d4a106", pink: "#e8478b", blue: "#3370ff", green: "#2ea043", purple: "#7f3bf5", orange: "#f77234", gray: "#646a73" };
const SIZE: Record<Kind, [number, number]> = { sticky: [200, 160], text: [240, 44], rect: [200, 120], ellipse: [180, 120], arrow: [0, 0] };

function boxes(): Box[] {
  const all = items.rows.filter((r) => (r.data.type || "sticky") !== "arrow");
  let free = 0;
  return all.map((r) => {
    const d = r.data, type = (d.type || "sticky") as Kind, [dw, dh] = SIZE[type] || SIZE.sticky;
    let x = d.x, y = d.y;
    if (typeof x !== "number" || typeof y !== "number") {
      // (an agent left the place out: a grid to the right of the rest)
      const i = free++;
      x = 40 + (i % 4) * 230; y = 40 + Math.floor(i / 4) * 190;
    }
    return { id: r.id, type, x, y, w: d.w || dw, h: d.h || dh, text: d.text || "", color: d.color || (type === "sticky" ? "yellow" : "gray"), z: d.z ?? 0 };
  }).sort((a, b) => a.z - b.z);
}
const maxZ = () => items.rows.reduce((m, r) => Math.max(m, Number(r.data.z) || 0), 0);

/** Where a line from a box's centre towards a point leaves the box. */
function edge(b: Box, tx: number, ty: number) {
  const cx = b.x + b.w / 2, cy = b.y + b.h / 2, dx = tx - cx, dy = ty - cy;
  if (!dx && !dy) return [cx, cy];
  if (b.type === "ellipse") {
    const a = b.w / 2, c = b.h / 2, t = 1 / Math.sqrt((dx * dx) / (a * a) + (dy * dy) / (c * c));
    return [cx + dx * t, cy + dy * t];
  }
  const t = Math.min(Math.abs((b.w / 2) / (dx || 1e-9)), Math.abs((b.h / 2) / (dy || 1e-9)));
  return [cx + dx * t, cy + dy * t];
}

type Tool = "select" | "hand" | Kind;
const TOOLS: [Tool, string, typeof Hand, string][] = [
  ["select", "选择", MousePointer2, "V"], ["hand", "拖动画布", Hand, "H"], ["sticky", "便签", StickyNote, "N"], ["text", "文字", Type, "T"],
  ["rect", "矩形", Square, "R"], ["ellipse", "椭圆", Circle, "O"], ["arrow", "连线：从一个拖到另一个", ArrowUpRight, "L"],
];

function useDarkMode() {
  const q = window.matchMedia?.("(prefers-color-scheme: dark)");
  const [d, setD] = useState(!!q?.matches);
  useEffect(() => { if (!q) return; const f = () => setD(q.matches); q.addEventListener("change", f); return () => q.removeEventListener("change", f); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return d;
}

function App() {
  useKit(kit);
  const dark = useDarkMode();
  const [tool, setTool] = useState<Tool>("select");
  const [view, setView] = useState({ x: 0, y: 0, k: 1 });
  const [sel, setSel] = useState<string[]>([]);
  const [editing, setEditing] = useState<string | null>(null);
  const [drag, setDrag] = useState<Record<string, { x: number; y: number; w?: number; h?: number }>>({});
  const [link, setLink] = useState<{ from: string; x: number; y: number } | null>(null);
  const stage = useRef<HTMLDivElement>(null);
  const touched = useRef(false);
  const op = useRef<null | { kind: "move" | "pan" | "resize" | "link"; sx: number; sy: number; vx: number; vy: number; start: Record<string, { x: number; y: number; w: number; h: number }>; moved: boolean }>(null);
  const list = boxes();
  const byId = useMemo(() => new Map(list.map((b) => [b.id, b])), [list]);
  const arrows = items.rows.filter((r) => r.data.type === "arrow");
  const at = (b: Box) => ({ ...b, ...(drag[b.id] || {}) });

  const toWorld = useCallback((cx: number, cy: number) => {
    const r = stage.current!.getBoundingClientRect();
    return { x: (cx - r.left - view.x) / view.k, y: (cy - r.top - view.y) / view.k };
  }, [view]);

  const put = (id: string, patch: Item) => { const cur = items.get(id); if (cur) kit.track(items.put({ ...cur.data, ...patch }, id)); kit.emit(); };
  const create = (type: Kind, x: number, y: number, extra: Item = {}) => {
    const [w, h] = SIZE[type];
    const id = uid("i");
    kit.track(items.put({ type, x: Math.round(x - w / 2), y: Math.round(y - h / 2), w, h, text: "", color: type === "sticky" ? "yellow" : "gray", z: maxZ() + 1, ...extra }, id));
    kit.emit();
    return id;
  };
  const remove = (ids: string[]) => {
    const gone = [...ids, ...arrows.filter((a) => ids.includes(String(a.data.from)) || ids.includes(String(a.data.to))).map((a) => a.id)];
    const keep = [...new Set(gone)].map((id) => items.get(id)).filter(Boolean).map((r) => ({ id: r!.id, data: r!.data }));
    for (const k of keep) kit.track(items.remove(k.id));
    setSel([]);
    kit.flash(`删除了 ${ids.length} 个`, () => { for (const k of keep) kit.track(items.put(k.data, k.id)); kit.note = null; kit.emit(); });
  };

  // ---- pointer -----------------------------------------------------------------
  const down = (e: RPE, id?: string, handle?: boolean) => {
    if (e.button !== 0 || editing) return;
    e.stopPropagation();
    const w = toWorld(e.clientX, e.clientY);
    if (!id && (tool === "sticky" || tool === "text" || tool === "rect" || tool === "ellipse")) {
      const nid = create(tool, w.x, w.y);
      setSel([nid]); setTool("select"); setEditing(nid);
      return;
    }
    if (id && tool === "arrow") {
      op.current = { kind: "link", sx: e.clientX, sy: e.clientY, vx: 0, vy: 0, start: {}, moved: false };
      setLink({ from: id, x: w.x, y: w.y });
      (e.target as Element).setPointerCapture?.(e.pointerId);
      return;
    }
    if (!id || tool === "hand") {
      if (!id) setSel([]);
      op.current = { kind: "pan", sx: e.clientX, sy: e.clientY, vx: view.x, vy: view.y, start: {}, moved: false };
      stage.current?.setPointerCapture(e.pointerId);
      return;
    }
    const now = e.shiftKey ? (sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id]) : sel.includes(id) ? sel : [id];
    setSel(now);
    const start: Record<string, { x: number; y: number; w: number; h: number }> = {};
    for (const s of handle ? [id] : now) { const b = byId.get(s); if (b) start[s] = { x: b.x, y: b.y, w: b.w, h: b.h }; }
    op.current = { kind: handle ? "resize" : "move", sx: e.clientX, sy: e.clientY, vx: 0, vy: 0, start, moved: false };
    stage.current?.setPointerCapture(e.pointerId);
  };
  const move = (e: RPE) => {
    const o = op.current;
    if (!o) return;
    const dx = (e.clientX - o.sx) / (o.kind === "pan" ? 1 : view.k), dy = (e.clientY - o.sy) / (o.kind === "pan" ? 1 : view.k);
    if (Math.abs(dx) + Math.abs(dy) > 2) o.moved = true;
    if (o.kind === "pan") { if (o.moved) touched.current = true; setView((v) => ({ ...v, x: o.vx + dx, y: o.vy + dy })); }
    else if (o.kind === "link") { const w = toWorld(e.clientX, e.clientY); setLink((l) => (l ? { ...l, x: w.x, y: w.y } : l)); }
    else if (o.kind === "move") {
      const next: typeof drag = {};
      for (const [id, s] of Object.entries(o.start)) next[id] = { x: Math.round(s.x + dx), y: Math.round(s.y + dy) };
      setDrag(next);
    } else {
      const [id, s] = Object.entries(o.start)[0];
      setDrag({ [id]: { x: s.x, y: s.y, w: Math.max(60, Math.round(s.w + dx)), h: Math.max(32, Math.round(s.h + dy)) } });
    }
  };
  const up = (e: RPE) => {
    const o = op.current;
    op.current = null;
    if (!o) return;
    if (o.kind === "link" && link) {
      const w = toWorld(e.clientX, e.clientY);
      const target = [...list].reverse().find((b) => b.id !== link.from && w.x >= b.x && w.x <= b.x + b.w && w.y >= b.y && w.y <= b.y + b.h);
      if (target) { kit.track(items.put({ type: "arrow", from: link.from, to: target.id, color: "gray", z: maxZ() + 1 }, uid("a"))); kit.emit(); }
      setLink(null);
      setTool("select");
      return;
    }
    if ((o.kind === "move" || o.kind === "resize") && o.moved) {
      for (const [id, d] of Object.entries(drag)) put(id, d);
      if (o.kind === "move") { const z = maxZ(); Object.keys(drag).forEach((id, i) => put(id, { z: z + 1 + i })); }
    }
    setDrag({});
  };
  const wheel = (e: React.WheelEvent) => {
    touched.current = true;
    if (e.ctrlKey || e.metaKey) {
      const r = stage.current!.getBoundingClientRect(), px = e.clientX - r.left, py = e.clientY - r.top;
      setView((v) => { const k = Math.min(3, Math.max(0.2, v.k * Math.exp(-e.deltaY * 0.01))); return { k, x: px - ((px - v.x) * k) / v.k, y: py - ((py - v.y) * k) / v.k }; });
    } else setView((v) => ({ ...v, x: v.x - e.deltaX, y: v.y - e.deltaY }));
  };
  const zoom = (f: number) => {
    touched.current = true;
    const r = stage.current!.getBoundingClientRect(), px = r.width / 2, py = r.height / 2;
    setView((v) => { const k = Math.min(3, Math.max(0.2, v.k * f)); return { k, x: px - ((px - v.x) * k) / v.k, y: py - ((py - v.y) * k) / v.k }; });
  };
  const fit = useCallback(() => {
    const el = stage.current, bs = boxes();
    if (!el) return;
    if (!bs.length) { setView({ x: 0, y: 0, k: 1 }); return; }
    const x0 = Math.min(...bs.map((b) => b.x)), y0 = Math.min(...bs.map((b) => b.y));
    const x1 = Math.max(...bs.map((b) => b.x + b.w)), y1 = Math.max(...bs.map((b) => b.y + b.h));
    const k = Math.min(1.2, Math.max(0.2, Math.min((el.clientWidth - 80) / (x1 - x0), (el.clientHeight - 80) / (y1 - y0))));
    setView({ k, x: (el.clientWidth - (x1 - x0) * k) / 2 - x0 * k, y: (el.clientHeight - (y1 - y0) * k) / 2 - y0 * k });
  }, []);
  // (fitted at first, and again when the panel grows, until you move it yourself)
  useEffect(() => {
    fit();
    const el = stage.current;
    if (!el) return;
    const ro = new ResizeObserver(() => { if (!touched.current) fit(); });
    ro.observe(el);
    return () => ro.disconnect();
  }, [fit]);

  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (editing || t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName)) return;
      if ((e.key === "Delete" || e.key === "Backspace") && sel.length) { e.preventDefault(); remove(sel); return; }
      if (e.key === "Escape") { setSel([]); setTool("select"); return; }
      if (e.metaKey || e.ctrlKey) {
        if (e.key === "a") { e.preventDefault(); setSel(boxes().map((b) => b.id)); }
        return;
      }
      const t2 = TOOLS.find((x) => x[3].toLowerCase() === e.key.toLowerCase());
      if (t2) setTool(t2[0]);
      if (e.key === "Enter" && sel.length === 1) { e.preventDefault(); setEditing(sel[0]); }
    };
    document.addEventListener("keydown", k);
    return () => document.removeEventListener("keydown", k);
  });

  const exportSvg = async () => {
    const bs = boxes();
    if (!bs.length) { kit.flash("白板上还什么都没有"); return; }
    const x0 = Math.min(...bs.map((b) => b.x)) - 40, y0 = Math.min(...bs.map((b) => b.y)) - 40;
    const x1 = Math.max(...bs.map((b) => b.x + b.w)) + 40, y1 = Math.max(...bs.map((b) => b.y + b.h)) + 40;
    const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;");
    const lines = (b: Box, size: number) => {
      const per = Math.max(4, Math.floor((b.w - 24) / (size * 0.95)));
      const out: string[] = [];
      for (const para of b.text.split("\n")) { let s = para; do { out.push(s.slice(0, per)); s = s.slice(per); } while (s); }
      return out;
    };
    const parts = bs.map((b) => {
      const size = b.type === "text" ? 20 : 16, ls = lines(b, size), center = b.type === "rect" || b.type === "ellipse";
      const shape = b.type === "sticky" ? `<rect x="${b.x}" y="${b.y}" width="${b.w}" height="${b.h}" rx="6" fill="${STICKY[b.color]?.[0] || STICKY.yellow[0]}"/>`
        : b.type === "rect" ? `<rect x="${b.x}" y="${b.y}" width="${b.w}" height="${b.h}" rx="8" fill="#fff" stroke="${STROKE[b.color] || STROKE.gray}" stroke-width="2"/>`
        : b.type === "ellipse" ? `<ellipse cx="${b.x + b.w / 2}" cy="${b.y + b.h / 2}" rx="${b.w / 2}" ry="${b.h / 2}" fill="#fff" stroke="${STROKE[b.color] || STROKE.gray}" stroke-width="2"/>` : "";
      const top = center ? b.y + b.h / 2 - ((ls.length - 1) * size * 1.4) / 2 + size * 0.35 : b.y + 12 + size;
      const text = ls.map((l, i) => `<text x="${center ? b.x + b.w / 2 : b.x + 12}" y="${top + i * size * 1.4}" font-size="${size}"${center ? ' text-anchor="middle"' : ""}>${esc(l)}</text>`).join("");
      return shape + text;
    });
    const arr = arrows.map((a) => {
      const f = bs.find((b) => b.id === a.data.from), t = bs.find((b) => b.id === a.data.to);
      if (!f || !t) return "";
      const [ax, ay] = edge(f, t.x + t.w / 2, t.y + t.h / 2), [bx, by] = edge(t, f.x + f.w / 2, f.y + f.h / 2);
      return `<line x1="${ax}" y1="${ay}" x2="${bx}" y2="${by}" stroke="#646a73" stroke-width="2" marker-end="url(#h)"/>`;
    });
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${x0} ${y0} ${x1 - x0} ${y1 - y0}" width="${x1 - x0}" height="${y1 - y0}" font-family="PingFang SC, Microsoft YaHei, sans-serif" fill="#1f2329"><defs><marker id="h" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto"><path d="M0 0L10 5L0 10z" fill="#646a73"/></marker></defs><rect x="${x0}" y="${y0}" width="${x1 - x0}" height="${y1 - y0}" fill="#fff"/>${arr.join("")}${parts.join("")}</svg>`;
    try { await save(`${(title.get() || "白板").replace(/[\\/:*?"<>|]/g, " ")}.svg`, svg, "image/svg+xml"); kit.flash("已存到「下载」"); }
    catch (e) { kit.flash((e as Error).message || "没存下来"); }
  };

  const one = sel.length === 1 ? byId.get(sel[0]) : undefined;
  const selArrow = sel.length === 1 && items.get(sel[0])?.data.type === "arrow" ? sel[0] : null;
  return (
    <div className="app wb">
      <Bar kit={kit} title={title.get()} placeholder="未命名白板" onTitle={title.set}>
        <button type="button" className="btn" onClick={() => void exportSvg()}><Download className="ico" />导出图片</button>
      </Bar>
      <div ref={stage} className={`wb-stage tool-${tool}${dark ? " is-dark" : ""}`} onPointerDown={(e) => down(e)} onPointerMove={move} onPointerUp={up} onWheel={wheel}
        onDoubleClick={(e) => { if (e.target === stage.current || (e.target as Element).classList.contains("wb-world")) { const w = toWorld(e.clientX, e.clientY); const id = create("sticky", w.x, w.y); setSel([id]); setEditing(id); } }}
        style={{ backgroundPosition: `${view.x}px ${view.y}px`, backgroundSize: `${24 * view.k}px ${24 * view.k}px` }}>
        <div className="wb-world" style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.k})` }}>
          <svg className="wb-links" width="1" height="1">
            <defs><marker id="head" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" fill="context-stroke" /></marker></defs>
            {arrows.map((a) => {
              const f = byId.get(String(a.data.from)), t = byId.get(String(a.data.to));
              if (!f || !t) return null;
              const F = at(f), T = at(t);
              const [ax, ay] = edge(F, T.x + T.w / 2, T.y + T.h / 2), [bx, by] = edge(T, F.x + F.w / 2, F.y + F.h / 2);
              const on = sel.includes(a.id);
              return (
                <g key={a.id} onPointerDown={(e) => { e.stopPropagation(); setSel([a.id]); }}>
                  <line x1={ax} y1={ay} x2={bx} y2={by} stroke="transparent" strokeWidth="14" />
                  <line x1={ax} y1={ay} x2={bx} y2={by} className={`wb-line${on ? " is-on" : ""}`} markerEnd="url(#head)" />
                </g>
              );
            })}
            {link && (() => { const f = byId.get(link.from); if (!f) return null; const [ax, ay] = edge(f, link.x, link.y); return <line x1={ax} y1={ay} x2={link.x} y2={link.y} className="wb-line is-on" markerEnd="url(#head)" strokeDasharray="6 4" />; })()}
          </svg>
          {list.map((b0) => {
            const b = at(b0), on = sel.includes(b.id), fill = STICKY[b.color] || STICKY.yellow;
            const style: React.CSSProperties = { left: b.x, top: b.y, width: b.w, height: b.type === "text" ? undefined : b.h, minHeight: b.type === "text" ? b.h : undefined, zIndex: b.z };
            if (b.type === "sticky") style.background = dark ? fill[1] : fill[0];
            if (b.type === "rect" || b.type === "ellipse") style.borderColor = STROKE[b.color] || STROKE.gray;
            return (
              <div key={b.id} className={`wb-item is-${b.type}${on ? " is-on" : ""}`} style={style} data-id={b.id}
                onPointerDown={(e) => down(e, b.id)} onDoubleClick={(e) => { e.stopPropagation(); setSel([b.id]); setEditing(b.id); }}>
                <Text id={b.id} value={b.text} editing={editing === b.id} onDone={(v) => { setEditing(null); if (v !== b.text) put(b.id, { text: v }); }} />
                {on && sel.length === 1 && editing !== b.id && <span className="wb-handle" onPointerDown={(e) => down(e, b.id, true)} />}
              </div>
            );
          })}
        </div>
        <div className="wb-tools" onPointerDown={(e) => e.stopPropagation()}>
          {TOOLS.map(([t, name, Icon, key]) => (
            <button key={t} type="button" className={tool === t ? "is-on" : ""} title={`${name}（${key}）`} aria-label={name} aria-pressed={tool === t} onClick={() => setTool(t)}><Icon className="ico" /></button>
          ))}
        </div>
        {(one || selArrow) && !editing && (
          <div className="wb-props" onPointerDown={(e) => e.stopPropagation()}>
            {one && Object.keys(one.type === "sticky" ? STICKY : STROKE).map((c) => (
              <button key={c} type="button" className={`wb-swatch${one.color === c ? " is-on" : ""}`} aria-label={c}
                style={{ background: one.type === "sticky" ? (dark ? STICKY[c][1] : STICKY[c][0]) : STROKE[c] }} onClick={() => put(one.id, { color: c })} />
            ))}
            <span className="wb-sep" />
            <button type="button" className="icon-btn" title="删除（Delete）" aria-label="删除" onClick={() => remove(sel)}><Trash2 className="ico" /></button>
          </div>
        )}
        {sel.length > 1 && !editing && (
          <div className="wb-props" onPointerDown={(e) => e.stopPropagation()}>
            <span className="wb-count">选中 {sel.length} 个</span>
            <button type="button" className="icon-btn" title="删除（Delete）" aria-label="删除" onClick={() => remove(sel)}><Trash2 className="ico" /></button>
          </div>
        )}
        <div className="wb-zoom" onPointerDown={(e) => e.stopPropagation()}>
          <button type="button" aria-label="缩小" onClick={() => zoom(1 / 1.2)}><Minus className="ico" /></button>
          <button type="button" className="wb-k" onClick={() => setView((v) => ({ ...v, k: 1 }))} title="恢复 100%">{Math.round(view.k * 100)}%</button>
          <button type="button" aria-label="放大" onClick={() => zoom(1.2)}><Plus className="ico" /></button>
          <button type="button" aria-label="适应画面" title="适应画面" onClick={fit}><Maximize className="ico" /></button>
        </div>
        {!list.length && <div className="wb-empty">双击空白处贴一张便签，或者从左边选个工具。也可以让 agent 来：「在白板上把这次复盘的要点贴成便签」。</div>}
      </div>
      <Toast kit={kit} />
    </div>
  );
}

/** An item's text: shown, or edited in place (Esc or a click outside ends). */
function Text({ value, editing, onDone }: { id: string; value: string; editing: boolean; onDone: (v: string) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (editing) {
      el.innerText = value;
      el.focus();
      const r = document.createRange(); r.selectNodeContents(el); r.collapse(false);
      const s = getSelection(); s?.removeAllRanges(); s?.addRange(r);
    }
  }, [editing]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!editing) return <div className="wb-text">{value}</div>;
  return (
    <div ref={ref} className="wb-text is-editing" contentEditable={"plaintext-only" as unknown as boolean} suppressContentEditableWarning
      onPointerDown={(e) => e.stopPropagation()}
      onBlur={() => onDone((ref.current?.innerText || "").replace(/\n$/, ""))}
      onKeyDown={(e) => { if (e.key === "Escape" || (e.key === "Enter" && (e.metaKey || e.ctrlKey))) { e.preventDefault(); (e.target as HTMLElement).blur(); } }} />
  );
}

void mount(kit, App, async () => {
  if (!board.get("main")) await board.put({ title: "" }, "main")?.catch(() => board.load());
});
