import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Maximize2, Minimize2, X } from "lucide-react";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Up to four chats at once, in a grid of at most two rows of at most two
 * panes. The first pane is the space's own (the coordinator, the group, a
 * chat); the others are members opened beside it: dragged in from a list
 * of members (onto an edge of a pane: a new pane there) or opened with
 * 「在新的一栏打开」. The lines between panes drag to share the room. Where
 * the panes do not fit at their least size, they turn into tabs: one
 * shown, the others a click away. One pane can take the whole room.
 *
 * Every pane stays mounted whatever the layout (absolutely placed, in the
 * same order), so a chat keeps its place and its draft when panes move.
 */

/** A pane: its content (handed the pane's own buttons: full room, close)
 *  and the name its tab shows. */
export type PaneSlot = {
  key: string;
  title: string;
  /** the first pane (the space's own): it does not close */
  main?: boolean;
  node: (actions: React.ReactNode) => React.ReactNode;
};

/** Which panes are where: rows of keys, and how the room is shared (the
 *  first pane's share of its row; the first row's share of the height). */
export type PaneLayout = { rows: string[][]; col: number[]; row: number };
export type Edge = "left" | "right" | "top" | "bottom" | "center";
/** Where a member was dropped: an edge of a pane, or its middle (in its
 *  place). Key "": wherever there is room. */
export type DropAt = { key: string; edge: Edge };

/** What is dragged in: a member, by its workspace and name. */
export const DRAG_TYPE = "application/x-myco-agent";
export type DragAgent = { hubId: string; agent: string };
export function dragAgent(e: React.DragEvent, a: DragAgent) {
  e.dataTransfer.setData(DRAG_TYPE, JSON.stringify(a));
  e.dataTransfer.setData("text/plain", a.agent);
  e.dataTransfer.effectAllowed = "copy";
}
const isAgentDrag = (e: React.DragEvent) => Array.from(e.dataTransfer.types).includes(DRAG_TYPE);

export const MAX_PANES = 4;
/** The least a pane needs to be useful. */
export const PANE_MIN_W = 320;
export const PANE_MIN_H = 240;

export function paneCount(l: PaneLayout): number {
  return l.rows.reduce((n, r) => n + r.length, 0);
}

/** The layout with `key` added: where it was dropped, or (no place given)
 *  beside the others, then below them. Null where it cannot go. */
export function placePane(l: PaneLayout, key: string, at?: DropAt): PaneLayout | null {
  if (paneCount(l) >= MAX_PANES) return null;
  const rows = l.rows.map((r) => [...r]);
  const col = [...l.col];
  if (!at) {
    const r = rows.findIndex((x) => x.length < 2);
    if (r >= 0) { rows[r].push(key); col[r] = 0.5; }
    else if (rows.length < 2) { rows.push([key]); col.push(0.5); }
    else return null;
    return { rows, col, row: rows.length === 2 && l.rows.length === 1 ? 0.5 : l.row };
  }
  const r = rows.findIndex((x) => x.includes(at.key));
  if (r < 0 || at.edge === "center") return null;
  if (at.edge === "left" || at.edge === "right") {
    if (rows[r].length >= 2) return null;
    const i = rows[r].indexOf(at.key);
    rows[r].splice(at.edge === "left" ? i : i + 1, 0, key);
    col[r] = 0.5;
    return { rows, col, row: l.row };
  }
  if (rows.length < 2) {
    if (at.edge === "top") { rows.unshift([key]); col.unshift(0.5); }
    else { rows.push([key]); col.push(0.5); }
    return { rows, col, row: 0.5 };
  }
  // Two rows already: into the row above or below, if it has room.
  const into = at.edge === "top" ? r - 1 : r + 1;
  if (into < 0 || into > 1 || rows[into].length >= 2) return null;
  rows[into].push(key);
  col[into] = 0.5;
  return { rows, col, row: l.row };
}

/** The layout without `key` (an emptied row goes; the rest share). */
export function removePane(l: PaneLayout, key: string): PaneLayout {
  const rows: string[][] = [];
  const col: number[] = [];
  l.rows.forEach((r, i) => {
    const left = r.filter((k) => k !== key);
    if (!left.length) return;
    rows.push(left);
    col.push(left.length === r.length ? l.col[i] ?? 0.5 : 0.5);
  });
  return { rows, col, row: rows.length === 2 ? l.row : 0.5 };
}

/** The layout with `key` in place of `old`. */
export function replacePane(l: PaneLayout, old: string, key: string): PaneLayout {
  return { ...l, rows: l.rows.map((r) => r.map((k) => (k === old ? key : k))) };
}

/** Whether every pane gets its least room in a box this size. */
export function panesFit(l: PaneLayout, w: number, h: number): boolean {
  const cols = Math.max(...l.rows.map((r) => r.length), 1);
  return w >= cols * PANE_MIN_W && h >= l.rows.length * PANE_MIN_H;
}

type Rect = { x: number; y: number; w: number; h: number };
const clamp = (v: number) => Math.min(0.8, Math.max(0.2, v));

/** Where each pane is, as shares of the room (0..1). */
function rectsOf(l: PaneLayout): Map<string, Rect> {
  const out = new Map<string, Rect>();
  const rowH = l.rows.length === 2 ? [l.row, 1 - l.row] : [1];
  let y = 0;
  l.rows.forEach((r, i) => {
    const h = rowH[i];
    const ws = r.length === 2 ? [l.col[i] ?? 0.5, 1 - (l.col[i] ?? 0.5)] : [1];
    let x = 0;
    r.forEach((k, j) => { out.set(k, { x, y, w: ws[j], h }); x += ws[j]; });
    y += h;
  });
  return out;
}

const pct = (v: number) => `${(v * 100).toFixed(3)}%`;
const halfOf = (b: Rect, e: Edge): Rect => (e === "left" ? { ...b, w: b.w / 2 } : e === "right" ? { ...b, x: b.x + b.w / 2, w: b.w / 2 }
  : e === "top" ? { ...b, h: b.h / 2 } : e === "bottom" ? { ...b, y: b.y + b.h / 2, h: b.h / 2 } : b);

export function PaneGrid({ slots, layout, onLayout, zoom, onZoom, onClose, onDrop, onFocus, className }: {
  slots: PaneSlot[];
  layout: PaneLayout;
  onLayout: (l: PaneLayout) => void;
  /** the pane taking the whole room ("" for none) */
  zoom: string;
  onZoom: (key: string) => void;
  onClose: (key: string) => void;
  /** a member dropped here; false: it could not go there */
  onDrop?: (a: DragAgent, at: DropAt) => void;
  /** a pane was clicked into (the one "beside" means from now on) */
  onFocus?: (key: string) => void;
  className?: string;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ w: 0, h: 0 });
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const read = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    read();
    const ro = new ResizeObserver(read);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const keys = layout.rows.flat();
  const many = keys.length > 1;
  // Too small for every pane at its least: tabs, one pane shown.
  // (one pane with the whole room needs no tabs: its button brings the rest back)
  const tabbed = many && !zoom && size.w > 0 && !panesFit(layout, size.w, size.h);
  const [tab, setTab] = useState(keys[keys.length - 1] || "");
  // A new pane is the one shown (and slides in).
  const before = useRef(keys);
  const [fresh, setFresh] = useState("");
  useEffect(() => {
    const added = keys.find((k) => !before.current.includes(k));
    before.current = keys;
    if (added) {
      setTab(added);
      setFresh(added);
      const t = window.setTimeout(() => setFresh(""), 400);
      return () => window.clearTimeout(t);
    }
    else if (!keys.includes(tab)) setTab(keys[keys.length - 1] || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keys.join("\u0000")]);
  const alone = zoom && keys.includes(zoom) ? zoom : tabbed ? (keys.includes(tab) ? tab : keys[0]) : "";
  const rects = rectsOf(layout);

  // The lines between panes: dragged, they share the room.
  const drag = useRef<{ kind: "col" | "row"; r: number; from: number; at: number; size: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const startDrag = (kind: "col" | "row", r: number) => (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    const el = box.current!;
    const rect = el.getBoundingClientRect();
    drag.current = { kind, r, from: kind === "col" ? layout.col[r] ?? 0.5 : layout.row, at: kind === "col" ? e.clientX : e.clientY, size: kind === "col" ? rect.width : rect.height };
    setDragging(true);
    document.body.classList.add(kind === "col" ? "is-resizing" : "is-resizing-rows");
  };
  const moveDrag = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d) return;
    const v = clamp(d.from + ((d.kind === "col" ? e.clientX : e.clientY) - d.at) / d.size);
    if (d.kind === "col") { const col = [...layout.col]; col[d.r] = v; onLayout({ ...layout, col }); }
    else onLayout({ ...layout, row: v });
  };
  const endDrag = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    drag.current = null;
    setDragging(false);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
    document.body.classList.remove("is-resizing", "is-resizing-rows");
  };

  // A member dragged over: where it would go (an edge of a pane: half of
  // it; its middle: in its place), or why it cannot.
  const [over, setOver] = useState<{ at: DropAt; ok: boolean } | null>(null);
  const depth = useRef(0);
  const atPoint = (e: React.DragEvent): DropAt | null => {
    const el = box.current;
    if (!el) return null;
    const b = el.getBoundingClientRect();
    const px = (e.clientX - b.left) / b.width, py = (e.clientY - b.top) / b.height;
    const shown = alone ? [[alone, { x: 0, y: 0, w: 1, h: 1 }] as const] : [...rects.entries()];
    for (const [key, r] of shown) {
      if (px < r.x || px > r.x + r.w || py < r.y || py > r.y + r.h) continue;
      const fx = (px - r.x) / r.w, fy = (py - r.y) / r.h;
      const edge: Edge = Math.min(fx, 1 - fx, fy, 1 - fy) > 0.3 ? "center"
        : Math.min(fx, 1 - fx) < Math.min(fy, 1 - fy) ? (fx < 0.5 ? "left" : "right") : (fy < 0.5 ? "top" : "bottom");
      return { key, edge };
    }
    return null;
  };
  // Where a pane dropped there goes (its place in the new layout), or
  // null where it cannot go.
  const placeAt = (at: DropAt): Rect | null => {
    const slot = slots.find((s) => s.key === at.key);
    if (at.edge === "center") return slot && !slot.main ? (alone ? { x: 0, y: 0, w: 1, h: 1 } : rects.get(at.key) || null) : null;
    if (alone) return paneCount(layout) < MAX_PANES ? halfOf({ x: 0, y: 0, w: 1, h: 1 }, at.edge) : null;
    const next = placePane(layout, "\u0000new", at);
    return next ? rectsOf(next).get("\u0000new") || null : null;
  };
  const okAt = (at: DropAt) => !!placeAt(at);
  const dropProps = onDrop ? {
    onDragEnter: (e: React.DragEvent) => { if (isAgentDrag(e)) depth.current++; },
    onDragLeave: (e: React.DragEvent) => {
      if (!isAgentDrag(e)) return;
      depth.current = Math.max(0, depth.current - 1);
      if (!depth.current) setOver(null);
    },
    onDragOver: (e: React.DragEvent) => {
      if (!isAgentDrag(e)) return;
      e.preventDefault();
      const at = atPoint(e);
      if (!at) { setOver(null); return; }
      const ok = okAt(at);
      e.dataTransfer.dropEffect = ok ? "copy" : "none";
      setOver((o) => (o && o.at.key === at.key && o.at.edge === at.edge && o.ok === ok ? o : { at, ok }));
    },
    onDrop: (e: React.DragEvent) => {
      if (!isAgentDrag(e)) return;
      e.preventDefault();
      depth.current = 0;
      const at = atPoint(e);
      setOver(null);
      if (!at || !okAt(at)) return;
      // (one pane shown, as tabs or with the whole room: an edge means
      // "a new pane", wherever there is room; key "")
      const where = alone && at.edge !== "center" ? { key: "", edge: at.edge } : at;
      try { onDrop(JSON.parse(e.dataTransfer.getData(DRAG_TYPE)) as DragAgent, where); } catch { /* not a member */ }
    },
  } : {};
  // The highlighted part of the pane under the pointer.
  const hint = (() => {
    if (!over) return null;
    const base = alone ? { x: 0, y: 0, w: 1, h: 1 } : rects.get(over.at.key);
    if (!base) return null;
    const e = over.at.edge;
    const r: Rect = placeAt(over.at) || (e === "center" ? base : halfOf(base, e));
    const full = paneCount(layout) >= MAX_PANES;
    const why = over.ok ? (e === "center" ? L("换成它", "Show it here instead") : L("在这里多开一栏", "Open it in a new pane here"))
      : full ? L("最多四栏：放到一栏中间换掉它", "Four panes at most: drop on one to show it there instead")
      : e === "center" ? L("放到边上，多开一栏", "Drop on an edge for a new pane") : L("这边放不下了", "No room on this side");
    return { r, why };
  })();

  const buttons = (s: PaneSlot) => {
    if (!many) return null;
    const zoomed = zoom === s.key;
    return (
      <>
        {!tabbed && (
          <button type="button" className="app-btn app-btn-plain sp-pane-btn" onClick={() => onZoom(zoomed ? "" : s.key)}
            title={zoomed ? L("还原所有栏", "Back to all the panes") : L("只看这一栏", "Show only this pane")}
            aria-label={zoomed ? L("还原所有栏", "Back to all the panes") : L("只看这一栏", "Show only this pane")}>
            {zoomed ? <Minimize2 /> : <Maximize2 />}
          </button>
        )}
        {!s.main && (
          <button type="button" className="app-btn app-btn-plain sp-pane-btn" onClick={() => onClose(s.key)} title={L("关掉这一栏", "Close this pane")} aria-label={L("关掉这一栏", "Close this pane")}>
            <X />
          </button>
        )}
      </>
    );
  };

  return (
    <div ref={box} className={cn("pg-grid", tabbed && "is-tabbed", dragging && "is-dragging", className)} {...dropProps}>
      {tabbed && (
        <div className="pg-tabs" role="tablist">
          {keys.map((k) => {
            const s = slots.find((x) => x.key === k);
            return s ? (
              <button key={k} type="button" role="tab" aria-selected={alone === k} className={cn("pg-tab", alone === k && "is-on")}
                onClick={() => { setTab(k); if (zoom) onZoom(""); onFocus?.(k); }} title={s.title}>
                <span>{s.title}</span>
                {!s.main && <span className="pg-tab-x" role="button" aria-label={L("关掉这一栏", "Close this pane")}
                  onClick={(e) => { e.stopPropagation(); onClose(k); }}><X /></span>}
              </button>
            ) : null;
          })}
        </div>
      )}
      <div className="pg-room">
        {slots.filter((s) => keys.includes(s.key)).map((s) => {
          const r = alone ? (alone === s.key ? { x: 0, y: 0, w: 1, h: 1 } : null) : rects.get(s.key);
          return (
            <div key={s.key} data-pane={s.key} className={cn("sp-pane pg-pane", s.main ? "sp-pane-left" : "sp-pane-right", !r && "is-hidden", fresh === s.key && "is-new", r && r.x > 0 && "has-left", r && r.y > 0 && "has-top")}
              style={r ? { left: pct(r.x), top: pct(r.y), width: pct(r.w), height: pct(r.h) } : undefined}
              onPointerDownCapture={() => onFocus?.(s.key)}>
              {s.node(buttons(s))}
            </div>
          );
        })}
        {!alone && layout.rows.map((row, i) => (row.length === 2 ? (() => {
          const a = rects.get(row[0])!;
          return (
            <div key={`c${i}`} className="pg-line is-col" role="separator" aria-orientation="vertical" aria-label={L("拖动调整宽度", "Drag to resize")}
              style={{ left: pct(a.x + a.w), top: pct(a.y), height: pct(a.h) }}
              onPointerDown={startDrag("col", i)} onPointerMove={moveDrag} onPointerUp={endDrag} onPointerCancel={endDrag}
              onDoubleClick={() => { const col = [...layout.col]; col[i] = 0.5; onLayout({ ...layout, col }); }} />
          );
        })() : null))}
        {!alone && layout.rows.length === 2 && (
          <div className="pg-line is-row" role="separator" aria-orientation="horizontal" aria-label={L("拖动调整高度", "Drag to resize")}
            style={{ top: pct(layout.row) }}
            onPointerDown={startDrag("row", 0)} onPointerMove={moveDrag} onPointerUp={endDrag} onPointerCancel={endDrag}
            onDoubleClick={() => onLayout({ ...layout, row: 0.5 })} />
        )}
        {hint && (
          <div className={cn("pg-drop", !over?.ok && "is-no")} style={{ left: pct(hint.r.x), top: pct(hint.r.y), width: pct(hint.r.w), height: pct(hint.r.h) }}>
            <span>{hint.why}</span>
          </div>
        )}
      </div>
    </div>
  );
}
