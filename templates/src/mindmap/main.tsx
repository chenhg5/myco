// 思维导图: a tree of topics drawn to the right of the root. Each topic is a
// record (its text and its parent); agents write a whole outline at once.
// Tab adds a child, Enter a sibling, Delete removes a branch, drag a topic
// onto another to move it there, fold branches, export a Markdown outline.
import "../_lib/sheet.css";
import "./mm.css";
import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent as RPE } from "react";
import { Download, Maximize, Minus, Plus, Trash2, CornerDownRight, ArrowDown } from "lucide-react";
import { Store, save } from "../_lib/myco";
import { Bar, Kit, Toast, mount, titleSaver, uid, useKit } from "../_lib/appkit";

type Node = { text?: string; parent?: string; order?: number; collapsed?: boolean };
const meta = new Store<{ title?: string }>("map");
const nodes = new Store<Node & Record<string, unknown>>("nodes");
const kit = new Kit([meta, nodes]);
const title = titleSaver(kit, meta);

// one quiet gray for every branch (mm.css; lighter in the dark)
const BRANCH = ["var(--branch)"];
type Laid = { id: string; text: string; depth: number; x: number; y: number; w: number; h: number; color: string; kids: string[]; collapsed: boolean; hidden: number };

const measure = (() => {
  const c = document.createElement("canvas").getContext("2d")!;
  return (s: string, font: string) => { c.font = font; return c.measureText(s || " ").width; };
})();
const FONT = `-apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif`;
const fontOf = (d: number) => (d === 0 ? `600 20px ${FONT}` : d === 1 ? `500 16px ${FONT}` : `15px ${FONT}`);

function tree() {
  const ids = new Set(nodes.rows.map((r) => r.id));
  const kids = new Map<string, string[]>();
  const roots: string[] = [];
  for (const r of nodes.rows) {
    const p = String(r.data.parent || "");
    if (p && ids.has(p) && p !== r.id) { if (!kids.has(p)) kids.set(p, []); kids.get(p)!.push(r.id); }
    else roots.push(r.id);
  }
  return { kids, roots };
}
const countAll = (id: string, kids: Map<string, string[]>): number => (kids.get(id) || []).reduce((n, k) => n + 1 + countAll(k, kids), 0);

function layout(): { laid: Map<string, Laid>; roots: string[]; kids: Map<string, string[]>; width: number; height: number } {
  const { kids, roots } = tree();
  const laid = new Map<string, Laid>();
  const GAPX = 56, GAPY = 14;
  const sizeOf = (id: string, depth: number) => {
    const text = String(nodes.get(id)?.data.text || "");
    const w = Math.min(320, Math.max(depth === 0 ? 120 : 56, measure(text, fontOf(depth)) + (depth === 0 ? 40 : 28)));
    const lines = Math.max(1, Math.ceil((measure(text, fontOf(depth)) + 28) / 320));
    return { w, h: (depth === 0 ? 52 : depth === 1 ? 38 : 34) + (lines - 1) * 22, text };
  };
  const span = new Map<string, number>();
  const measureTree = (id: string, depth: number, seen: Set<string>): number => {
    if (seen.has(id)) return 0;
    seen.add(id);
    const s = sizeOf(id, depth), collapsed = !!nodes.get(id)?.data.collapsed;
    const ks = collapsed ? [] : kids.get(id) || [];
    const sum = ks.reduce((n, k, i) => n + measureTree(k, depth + 1, seen) + (i ? GAPY : 0), 0);
    const h = Math.max(s.h, sum);
    span.set(id, h);
    return h;
  };
  const place = (id: string, depth: number, x: number, top: number, color: string, seen: Set<string>) => {
    if (seen.has(id)) return;
    seen.add(id);
    const s = sizeOf(id, depth), h = span.get(id) || s.h, collapsed = !!nodes.get(id)?.data.collapsed;
    const all = kids.get(id) || [];
    laid.set(id, { id, text: s.text, depth, x, y: top + h / 2 - s.h / 2, w: s.w, h: s.h, color, kids: all, collapsed, hidden: collapsed ? countAll(id, kids) : 0 });
    if (collapsed) return;
    const sum = all.reduce((n, k, i) => n + (span.get(k) || 0) + (i ? GAPY : 0), 0);
    let y = top + (h - sum) / 2;
    all.forEach((k, i) => {
      place(k, depth + 1, x + s.w + GAPX, y, depth === 0 ? BRANCH[i % BRANCH.length] : color, seen);
      y += (span.get(k) || 0) + GAPY;
    });
  };
  let top = 0, width = 0;
  const seen = new Set<string>(), seen2 = new Set<string>();
  for (const r of roots) { const h = measureTree(r, 0, seen); place(r, 0, 0, top, "var(--branch)", seen2); top += h + 60; }
  for (const l of laid.values()) width = Math.max(width, l.x + l.w);
  return { laid, roots, kids, width, height: top };
}

function useDarkMode() {
  const q = window.matchMedia?.("(prefers-color-scheme: dark)");
  const [d, setD] = useState(!!q?.matches);
  useEffect(() => { if (!q) return; const f = () => setD(q.matches); q.addEventListener("change", f); return () => q.removeEventListener("change", f); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return d;
}

function App() {
  useKit(kit);
  const dark = useDarkMode();
  const [sel, setSel] = useState<string | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [view, setView] = useState({ x: 60, y: 60, k: 1 });
  const [drag, setDrag] = useState<{ id: string; dx: number; dy: number; over: string | null } | null>(null);
  const stage = useRef<HTMLDivElement>(null);
  const touched = useRef(false);
  const op = useRef<null | { kind: "pan" | "node"; id?: string; sx: number; sy: number; vx: number; vy: number; moved: boolean }>(null);
  const { laid, roots, kids } = useMemo(layout, [kit.version]); // eslint-disable-line react-hooks/exhaustive-deps

  const put = (id: string, patch: Node) => { const cur = nodes.get(id); if (cur) kit.track(nodes.put({ ...cur.data, ...patch }, id)); kit.emit(); };
  const siblingsOf = (id: string) => { const p = String(nodes.get(id)?.data.parent || ""); return p ? kids.get(p) || [] : roots; };
  const addChild = (pid: string) => {
    const ks = kids.get(pid) || [], last = ks.length ? Number(nodes.get(ks[ks.length - 1])?.data.order) || ks.length : 0;
    if (nodes.get(pid)?.data.collapsed) put(pid, { collapsed: false });
    const id = uid("n");
    kit.track(nodes.put({ text: "", parent: pid, order: last + 1 }, id));
    kit.emit();
    setSel(id); setEditing(id);
  };
  const addSibling = (id: string) => {
    const p = String(nodes.get(id)?.data.parent || "");
    if (!p) { addChild(id); return; }
    const sib = siblingsOf(id), i = sib.indexOf(id);
    const a = Number(nodes.get(id)?.data.order) || i, b = sib[i + 1] ? Number(nodes.get(sib[i + 1])?.data.order) : a + 2;
    const nid = uid("n");
    kit.track(nodes.put({ text: "", parent: p, order: (a + b) / 2 }, nid));
    kit.emit();
    setSel(nid); setEditing(nid);
  };
  const remove = (id: string) => {
    const all: string[] = [];
    const walk = (x: string) => { all.push(x); for (const k of kids.get(x) || []) walk(k); };
    walk(id);
    const keep = all.map((x) => nodes.get(x)).filter(Boolean).map((r) => ({ id: r!.id, data: r!.data }));
    const p = String(nodes.get(id)?.data.parent || "");
    for (const k of keep) kit.track(nodes.remove(k.id));
    setSel(p || null);
    kit.flash(`删除了 ${keep.length} 个主题`, () => { for (const k of keep) kit.track(nodes.put(k.data, k.id)); kit.note = null; kit.emit(); });
  };
  const isUnder = (x: string, anc: string): boolean => { let p = String(nodes.get(x)?.data.parent || ""); const seen = new Set<string>(); while (p && !seen.has(p)) { if (p === anc) return true; seen.add(p); p = String(nodes.get(p)?.data.parent || ""); } return false; };

  const toWorld = useCallback((cx: number, cy: number) => {
    const r = stage.current!.getBoundingClientRect();
    return { x: (cx - r.left - view.x) / view.k, y: (cy - r.top - view.y) / view.k };
  }, [view]);
  const hit = (w: { x: number; y: number }, not?: string) => [...laid.values()].find((l) => l.id !== not && w.x >= l.x && w.x <= l.x + l.w && w.y >= l.y && w.y <= l.y + l.h);

  const down = (e: RPE, id?: string) => {
    if (e.button !== 0 || editing) return;
    e.stopPropagation();
    if (id) setSel(id);
    else setSel(null);
    op.current = { kind: id ? "node" : "pan", id, sx: e.clientX, sy: e.clientY, vx: view.x, vy: view.y, moved: false };
    stage.current?.setPointerCapture(e.pointerId);
  };
  const move = (e: RPE) => {
    const o = op.current;
    if (!o) return;
    const dx = e.clientX - o.sx, dy = e.clientY - o.sy;
    if (Math.abs(dx) + Math.abs(dy) > 4) o.moved = true;
    if (o.kind === "pan") { if (o.moved) touched.current = true; setView((v) => ({ ...v, x: o.vx + dx, y: o.vy + dy })); }
    else if (o.moved && o.id && String(nodes.get(o.id)?.data.parent || "")) {
      const over = hit(toWorld(e.clientX, e.clientY), o.id);
      setDrag({ id: o.id, dx: dx / view.k, dy: dy / view.k, over: over && !isUnder(over.id, o.id) ? over.id : null });
    }
  };
  const up = () => {
    const o = op.current;
    op.current = null;
    if (drag?.over) {
      const ks = kids.get(drag.over) || [], last = ks.length ? Number(nodes.get(ks[ks.length - 1])?.data.order) || ks.length : 0;
      put(drag.id, { parent: drag.over, order: last + 1 });
      if (nodes.get(drag.over)?.data.collapsed) put(drag.over, { collapsed: false });
    }
    setDrag(null);
    void o;
  };
  const wheel = (e: React.WheelEvent) => {
    touched.current = true;
    if (e.ctrlKey || e.metaKey) {
      const r = stage.current!.getBoundingClientRect(), px = e.clientX - r.left, py = e.clientY - r.top;
      setView((v) => { const k = Math.min(2.5, Math.max(0.25, v.k * Math.exp(-e.deltaY * 0.01))); return { k, x: px - ((px - v.x) * k) / v.k, y: py - ((py - v.y) * k) / v.k }; });
    } else setView((v) => ({ ...v, x: v.x - e.deltaX, y: v.y - e.deltaY }));
  };
  const zoom = (f: number) => {
    touched.current = true;
    const r = stage.current!.getBoundingClientRect(), px = r.width / 2, py = r.height / 2;
    setView((v) => { const k = Math.min(2.5, Math.max(0.25, v.k * f)); return { k, x: px - ((px - v.x) * k) / v.k, y: py - ((py - v.y) * k) / v.k }; });
  };
  const fit = useCallback(() => {
    const el = stage.current, L = layout();
    if (!el || !L.laid.size) return;
    const k = Math.min(1.2, Math.max(0.3, Math.min((el.clientWidth - 120) / (L.width || 1), (el.clientHeight - 80) / (L.height - 60 || 1))));
    setView({ k, x: (el.clientWidth - L.width * k) / 2, y: (el.clientHeight - (L.height - 60) * k) / 2 });
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
    const key = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (editing || t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName)) return;
      if (!sel) { if (e.key === "Enter" && roots[0]) { e.preventDefault(); setSel(roots[0]); } return; }
      const l = laid.get(sel);
      if (e.key === "Tab") { e.preventDefault(); addChild(sel); }
      else if (e.key === "Enter") { e.preventDefault(); addSibling(sel); }
      else if (e.key === "Delete" || e.key === "Backspace") { e.preventDefault(); if (String(nodes.get(sel)?.data.parent || "")) remove(sel); }
      else if (e.key === "F2") { e.preventDefault(); setEditing(sel); }
      else if (e.key === " ") { e.preventDefault(); if (l?.kids.length) put(sel, { collapsed: !l.collapsed }); }
      else if (e.key === "ArrowLeft") { const p = String(nodes.get(sel)?.data.parent || ""); if (p) setSel(p); }
      else if (e.key === "ArrowRight") { if (l && !l.collapsed && l.kids[0]) setSel(l.kids[0]); }
      else if (e.key === "ArrowUp" || e.key === "ArrowDown") { const s = siblingsOf(sel), i = s.indexOf(sel), j = i + (e.key === "ArrowUp" ? -1 : 1); if (s[j]) setSel(s[j]); }
      else if (e.key.length === 1 && !e.metaKey && !e.ctrlKey) { setEditing(sel); }
      else if (e.key === "Escape") setSel(null);
    };
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  });

  const exportMd = async () => {
    const lines = [`# ${title.get() || "思维导图"}`, ""];
    const walk = (id: string, d: number) => { lines.push(`${"  ".repeat(d)}- ${String(nodes.get(id)?.data.text || "")}`); for (const k of kids.get(id) || []) walk(k, d + 1); };
    for (const r of roots) walk(r, 0);
    try { await save(`${(title.get() || "思维导图").replace(/[\\/:*?"<>|]/g, " ")}.md`, lines.join("\n") + "\n", "text/markdown"); kit.flash("已存到「下载」"); }
    catch (e) { kit.flash((e as Error).message || "没存下来"); }
  };

  const edges: JSX.Element[] = [];
  for (const l of laid.values()) {
    if (l.collapsed) continue;
    for (const k of l.kids) {
      const c = laid.get(k);
      if (!c) continue;
      const x1 = l.x + l.w, y1 = l.y + l.h / 2, x2 = c.x, y2 = c.y + c.h / 2, mx = (x1 + x2) / 2;
      edges.push(<path key={`${l.id}-${k}`} d={`M${x1} ${y1} C${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`} style={{ stroke: c.color }} strokeWidth={l.depth === 0 ? 2.5 : 2} fill="none" opacity={0.8} />);
    }
  }
  return (
    <div className="app mm">
      <Bar kit={kit} title={title.get()} placeholder="未命名思维导图" onTitle={title.set}>
        <button type="button" className="btn" onClick={() => void exportMd()}><Download className="ico" />导出大纲</button>
      </Bar>
      <div ref={stage} className={`mm-stage${dark ? " is-dark" : ""}`} onPointerDown={(e) => down(e)} onPointerMove={move} onPointerUp={up} onWheel={wheel}>
        <div className="mm-world" style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.k})` }}>
          <svg className="mm-links" width="1" height="1">{edges}</svg>
          {[...laid.values()].map((l) => {
            const d = drag?.id === l.id ? drag : null;
            return (
              <div key={l.id} className={`mm-node d${Math.min(l.depth, 2)}${sel === l.id ? " is-on" : ""}${drag?.over === l.id ? " is-target" : ""}${d ? " is-drag" : ""}`}
                style={{ left: l.x + (d?.dx || 0), top: l.y + (d?.dy || 0), width: l.w, minHeight: l.h, ["--c" as string]: l.color }}
                onPointerDown={(e) => down(e, l.id)} onDoubleClick={(e) => { e.stopPropagation(); setSel(l.id); setEditing(l.id); }} data-id={l.id}>
                <Text value={l.text} editing={editing === l.id} placeholder={l.depth === 0 ? "中心主题" : "主题"}
                  onDone={(v, next) => {
                    setEditing(null);
                    if (v !== l.text) put(l.id, { text: v });
                    if (next === "sibling") addSibling(l.id); else if (next === "child") addChild(l.id);
                  }} />
                {l.kids.length > 0 && (
                  <button type="button" className={`mm-fold${l.collapsed ? " is-folded" : ""}`} onPointerDown={(e) => e.stopPropagation()}
                    onClick={(e) => { e.stopPropagation(); put(l.id, { collapsed: !l.collapsed }); }} aria-label={l.collapsed ? "展开" : "收起"}>
                    {l.collapsed ? l.hidden : "−"}
                  </button>
                )}
              </div>
            );
          })}
        </div>
        {sel && !editing && (
          <div className="wb-props mm-props" onPointerDown={(e) => e.stopPropagation()}>
            <button type="button" className="tool" onClick={() => addChild(sel)} title="Tab"><CornerDownRight className="ico" />子主题</button>
            {String(nodes.get(sel)?.data.parent || "") && <button type="button" className="tool" onClick={() => addSibling(sel)} title="Enter"><ArrowDown className="ico" />同级主题</button>}
            {String(nodes.get(sel)?.data.parent || "") && <button type="button" className="icon-btn" aria-label="删除" title="删除（Delete）" onClick={() => remove(sel)}><Trash2 className="ico" /></button>}
          </div>
        )}
        <div className="wb-zoom" onPointerDown={(e) => e.stopPropagation()}>
          <button type="button" aria-label="缩小" onClick={() => zoom(1 / 1.2)}><Minus className="ico" /></button>
          <button type="button" className="wb-k" onClick={() => setView((v) => ({ ...v, k: 1 }))}>{Math.round(view.k * 100)}%</button>
          <button type="button" aria-label="放大" onClick={() => zoom(1.2)}><Plus className="ico" /></button>
          <button type="button" aria-label="适应画面" title="适应画面" onClick={fit}><Maximize className="ico" /></button>
        </div>
        <div className="mm-help">选中一个主题：Tab 加子主题，Enter 加同级，空格折叠，Delete 删除；拖到另一个主题上就挪过去</div>
      </div>
      <Toast kit={kit} />
    </div>
  );
}

function Text({ value, editing, placeholder, onDone }: { value: string; editing: boolean; placeholder: string; onDone: (v: string, next?: "sibling" | "child") => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const done = useRef(false);
  useEffect(() => {
    const el = ref.current;
    if (!el || !editing) return;
    done.current = false;
    el.innerText = value;
    el.focus();
    const r = document.createRange(); r.selectNodeContents(el); r.collapse(false);
    const s = getSelection(); s?.removeAllRanges(); s?.addRange(r);
  }, [editing]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!editing) return <div className={`mm-text${value ? "" : " is-empty"}`}>{value || placeholder}</div>;
  const finish = (next?: "sibling" | "child") => { if (done.current) return; done.current = true; onDone((ref.current?.innerText || "").replace(/\n/g, " ").trim(), next); };
  return (
    <div ref={ref} className="mm-text is-editing" contentEditable={"plaintext-only" as unknown as boolean} suppressContentEditableWarning data-ph={placeholder}
      onPointerDown={(e) => e.stopPropagation()} onBlur={() => finish()}
      onKeyDown={(e) => {
        if (e.nativeEvent.isComposing) return;
        if (e.key === "Enter") { e.preventDefault(); finish("sibling"); }
        else if (e.key === "Tab") { e.preventDefault(); finish("child"); }
        else if (e.key === "Escape") { e.preventDefault(); finish(); }
      }} />
  );
}

void mount(kit, App, async () => {
  if (!meta.get("main")) await meta.put({ title: "" }, "main")?.catch(() => meta.load());
  if (!nodes.rows.length) await nodes.put({ text: "中心主题", parent: "", order: 1 }, "root")?.catch(() => nodes.load());
});
