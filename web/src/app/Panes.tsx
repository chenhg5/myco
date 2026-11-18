import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Maximize2, Minimize2, X } from "lucide-react";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * Up to four chats at once. The room is split the way you split it: a
 * member dropped on an edge of a pane splits that pane (dropped below the
 * left one: the left one is now two, one above the other; the right one
 * stays as it is); dropped on an edge of the whole room, the whole room
 * (a pane across the bottom of all of them). Its middle shows it there
 * instead. 「在新的一栏打开」 splits the largest pane along its longer side.
 * The first pane is the space's own (the coordinator, the group, a chat).
 *
 * The lines between panes drag to share the room. Where the panes do not
 * fit at their least size, they turn into tabs: one shown, the others a
 * click away. One pane can take the whole room. A member already on
 * screen is not opened twice: dragged in, its pane says so.
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

/** How the room is split: a pane (its key), or two parts side by side
 *  ("row": a left, b right) or one above the other ("col": a top, b
 *  bottom), `ratio` being a's share. */
export type PaneNode = string | { dir: "row" | "col"; ratio: number; a: PaneNode; b: PaneNode };
export type PaneLayout = { root: PaneNode };
export type Edge = "left" | "right" | "top" | "bottom" | "center";
/** Where a member was dropped: an edge of a pane, or its middle (in its
 *  place). Key "*": an edge of the whole room. Key "": wherever there is
 *  room. */
export type DropAt = { key: string; edge: Edge };

/** What is dragged in: a member, by its workspace and name. (Kept while
 *  it is dragged, too: a drag over says only what kind it is.) */
export const DRAG_TYPE = "application/x-myco-agent";
export type DragAgent = { hubId: string; agent: string };
let dragged: DragAgent | null = null;
export function dragAgent(e: React.DragEvent, a: DragAgent) {
  e.dataTransfer.setData(DRAG_TYPE, JSON.stringify(a));
  e.dataTransfer.setData("text/plain", a.agent);
  e.dataTransfer.effectAllowed = "copy";
  dragged = a;
  const done = () => { dragged = null; window.removeEventListener("dragend", done, true); window.removeEventListener("drop", done, true); };
  window.addEventListener("dragend", done, true);
  window.addEventListener("drop", done, true);
}
const isAgentDrag = (e: React.DragEvent) => Array.from(e.dataTransfer.types).includes(DRAG_TYPE);

export const MAX_PANES = 4;
/** The least a pane needs to be useful. */
export const PANE_MIN_W = 320;
export const PANE_MIN_H = 240;

export const onePane = (key: string): PaneLayout => ({ root: key });

export function paneKeys(n: PaneNode): string[] {
  return typeof n === "string" ? [n] : [...paneKeys(n.a), ...paneKeys(n.b)];
}
export function paneCount(l: PaneLayout): number {
  return paneKeys(l.root).length;
}
/** A layout as kept (on this computer): sound, or null. */
export function soundLayout(v: unknown): PaneLayout | null {
  const ok = (n: unknown): n is PaneNode => typeof n === "string"
    || (!!n && typeof n === "object" && ((n as { dir?: string }).dir === "row" || (n as { dir?: string }).dir === "col")
      && typeof (n as { ratio?: unknown }).ratio === "number" && ok((n as { a?: unknown }).a) && ok((n as { b?: unknown }).b));
  const root = (v as { root?: unknown } | null)?.root;
  return ok(root) ? { root } : null;
}

/** How many panes `n` puts one after another that way (side by side for
 *  "row", one above the other for "col"). */
const stacked = (n: PaneNode, dir: "row" | "col"): number => (typeof n === "string" ? 1
  : n.dir === dir ? stacked(n.a, dir) + stacked(n.b, dir) : Math.max(stacked(n.a, dir), stacked(n.b, dir)));
/** `n` with `key` on that edge, the new pane getting an equal share (two
 *  panes stacked there: a third). */
const split = (n: PaneNode, key: string, edge: Exclude<Edge, "center">): PaneNode => {
  const dir = edge === "left" || edge === "right" ? "row" : "col";
  const s = stacked(n, dir);
  return edge === "left" || edge === "top" ? { dir, ratio: 1 / (s + 1), a: key, b: n } : { dir, ratio: s / (s + 1), a: n, b: key };
};
const mapLeaf = (n: PaneNode, key: string, f: (leaf: string) => PaneNode): PaneNode => (
  typeof n === "string" ? (n === key ? f(n) : n) : { ...n, a: mapLeaf(n.a, key, f), b: mapLeaf(n.b, key, f) });

/** The layout with `key` added: on that edge of that pane (or of the
 *  whole room, key "*"); with no place given, in the largest pane, split
 *  along its longer side (on a wide room, beside it). Null where it
 *  cannot go (four already). */
export function placePane(l: PaneLayout, key: string, at?: DropAt): PaneLayout | null {
  const keys = paneKeys(l.root);
  if (keys.length >= MAX_PANES) return null;
  if (at && at.edge === "center") return null;
  if (at && at.key === "*") return { root: split(l.root, key, at.edge as Exclude<Edge, "center">) };
  if (at && at.key && keys.includes(at.key)) return { root: mapLeaf(l.root, at.key, (leaf) => split(leaf, key, at.edge as Exclude<Edge, "center">)) };
  // The largest pane (the last of equals: new panes go right, then down),
  // as the room is usually wider than tall.
  const rects = rectsOf(l);
  let best = keys[0];
  for (const k of keys) {
    const r = rects.get(k)!, b = rects.get(best)!;
    if (r.w * r.h >= b.w * b.h - 1e-6) best = k;
  }
  const r = rects.get(best)!;
  return { root: mapLeaf(l.root, best, (leaf) => split(leaf, key, r.w * 1.6 >= r.h ? "right" : "bottom")) };
}

/** The layout without `key` (the other part of its split takes its room). */
export function removePane(l: PaneLayout, key: string): PaneLayout {
  const drop = (n: PaneNode): PaneNode | null => {
    if (typeof n === "string") return n === key ? null : n;
    const a = drop(n.a), b = drop(n.b);
    if (!a) return b;
    if (!b) return a;
    return { ...n, a, b };
  };
  return { root: drop(l.root) || l.root };
}

/** The layout with `key` in place of `old`. */
export function replacePane(l: PaneLayout, old: string, key: string): PaneLayout {
  return { root: mapLeaf(l.root, old, () => key) };
}

type Rect = { x: number; y: number; w: number; h: number };
const clamp = (v: number) => Math.min(0.85, Math.max(0.15, v));
const WHOLE: Rect = { x: 0, y: 0, w: 1, h: 1 };

/** Where each pane is, as shares of the room (0..1). */
function rectsOf(l: PaneLayout): Map<string, Rect> {
  const out = new Map<string, Rect>();
  const walk = (n: PaneNode, r: Rect) => {
    if (typeof n === "string") { out.set(n, r); return; }
    if (n.dir === "row") {
      walk(n.a, { ...r, w: r.w * n.ratio });
      walk(n.b, { ...r, x: r.x + r.w * n.ratio, w: r.w * (1 - n.ratio) });
    } else {
      walk(n.a, { ...r, h: r.h * n.ratio });
      walk(n.b, { ...r, y: r.y + r.h * n.ratio, h: r.h * (1 - n.ratio) });
    }
  };
  walk(l.root, WHOLE);
  return out;
}

/** Each split's line: where it is, and the way to its node. */
type Line = { path: ("a" | "b")[]; dir: "row" | "col"; ratio: number; r: Rect };
function linesOf(l: PaneLayout): Line[] {
  const out: Line[] = [];
  const walk = (n: PaneNode, r: Rect, path: ("a" | "b")[]) => {
    if (typeof n === "string") return;
    out.push({ path, dir: n.dir, ratio: n.ratio, r });
    if (n.dir === "row") {
      walk(n.a, { ...r, w: r.w * n.ratio }, [...path, "a"]);
      walk(n.b, { ...r, x: r.x + r.w * n.ratio, w: r.w * (1 - n.ratio) }, [...path, "b"]);
    } else {
      walk(n.a, { ...r, h: r.h * n.ratio }, [...path, "a"]);
      walk(n.b, { ...r, y: r.y + r.h * n.ratio, h: r.h * (1 - n.ratio) }, [...path, "b"]);
    }
  };
  walk(l.root, WHOLE, []);
  return out;
}
const withRatio = (n: PaneNode, path: ("a" | "b")[], ratio: number): PaneNode => {
  if (typeof n === "string") return n;
  if (!path.length) return { ...n, ratio };
  return path[0] === "a" ? { ...n, a: withRatio(n.a, path.slice(1), ratio) } : { ...n, b: withRatio(n.b, path.slice(1), ratio) };
};

/** The smallest share of the width and of the height any pane has. */
export function leastShare(l: PaneLayout): { w: number; h: number } {
  let w = 1, h = 1;
  for (const r of rectsOf(l).values()) { w = Math.min(w, r.w); h = Math.min(h, r.h); }
  return { w, h };
}
/** Whether every pane gets its least room in a box this size. */
export function panesFit(l: PaneLayout, w: number, h: number): boolean {
  const s = leastShare(l);
  return w * s.w >= PANE_MIN_W - 1 && h * s.h >= PANE_MIN_H - 1;
}

const pct = (v: number) => `${(v * 100).toFixed(3)}%`;
const halfOf = (b: Rect, e: Edge): Rect => (e === "left" ? { ...b, w: b.w / 2 } : e === "right" ? { ...b, x: b.x + b.w / 2, w: b.w / 2 }
  : e === "top" ? { ...b, h: b.h / 2 } : e === "bottom" ? { ...b, y: b.y + b.h / 2, h: b.h / 2 } : b);
/** How near the room's own edge (px) a drop means "across the whole room". */
const OUTER = 22;

export function PaneGrid({ slots, layout, onLayout, zoom, onZoom, onClose, onDrop, openIn, onFocus, className }: {
  slots: PaneSlot[];
  layout: PaneLayout;
  onLayout: (l: PaneLayout) => void;
  /** the pane taking the whole room ("" for none) */
  zoom: string;
  onZoom: (key: string) => void;
  onClose: (key: string) => void;
  /** a member dropped here */
  onDrop?: (a: DragAgent, at: DropAt) => void;
  /** the pane a member is already in (it is not opened twice) */
  openIn?: (a: DragAgent) => string;
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
  const keys = paneKeys(layout.root);
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
    if (!keys.includes(tab)) setTab(keys[keys.length - 1] || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keys.join("\u0000")]);
  const alone = zoom && keys.includes(zoom) ? zoom : tabbed ? (keys.includes(tab) ? tab : keys[0]) : "";
  const rects = rectsOf(layout);
  const lines = alone ? [] : linesOf(layout);

  // The lines between panes: dragged, they share their part of the room.
  const drag = useRef<{ line: Line; from: number; at: number; size: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const startDrag = (line: Line) => (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    const b = box.current!.getBoundingClientRect();
    const row = line.dir === "row";
    drag.current = { line, from: line.ratio, at: row ? e.clientX : e.clientY, size: row ? b.width * line.r.w : b.height * line.r.h };
    setDragging(true);
    document.body.classList.add(row ? "is-resizing" : "is-resizing-rows");
  };
  const moveDrag = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d) return;
    const v = clamp(d.from + ((d.line.dir === "row" ? e.clientX : e.clientY) - d.at) / d.size);
    onLayout({ root: withRatio(layout.root, d.line.path, v) });
  };
  const endDrag = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    drag.current = null;
    setDragging(false);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
    document.body.classList.remove("is-resizing", "is-resizing-rows");
  };

  // A member dragged over: where it would go (an edge of a pane: half of
  // it; near an edge of the room: across it; a pane's middle: in its
  // place), or why it cannot.
  const [over, setOver] = useState<{ at: DropAt; ok: boolean; open?: string } | null>(null);
  const depth = useRef(0);
  const atPoint = (e: React.DragEvent): DropAt | null => {
    const el = box.current?.querySelector(".pg-room");
    if (!el) return null;
    const b = el.getBoundingClientRect();
    if (b.width <= 0 || b.height <= 0) return null;
    const dx = e.clientX - b.left, dy = e.clientY - b.top;
    if (dx < 0 || dy < 0 || dx > b.width || dy > b.height) return null;
    // Near the room's own edge, with more than one pane: across the room.
    if (!alone && many) {
      const near = Math.min(dx, b.width - dx, dy, b.height - dy);
      if (near < OUTER) {
        const edge: Edge = near === dx ? "left" : near === b.width - dx ? "right" : near === dy ? "top" : "bottom";
        return { key: "*", edge };
      }
    }
    const px = dx / b.width, py = dy / b.height;
    const shown = alone ? [[alone, WHOLE] as const] : [...rects.entries()];
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
    if (at.edge === "center") return slot && !slot.main ? (alone ? WHOLE : rects.get(at.key) || null) : null;
    if (alone) return keys.length < MAX_PANES ? halfOf(WHOLE, at.edge) : null;
    const next = placePane(layout, "\u0000new", at);
    return next ? rectsOf(next).get("\u0000new") || null : null;
  };
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
      // Already on screen: its pane says so, wherever the pointer is.
      const open = dragged && openIn ? openIn(dragged) : "";
      if (open && keys.includes(open) && (!alone || alone === open)) {
        e.dataTransfer.dropEffect = "none";
        setOver((o) => (o && o.open === open ? o : { at: { key: open, edge: "center" }, ok: false, open }));
        return;
      }
      const at = atPoint(e);
      if (!at) { setOver(null); return; }
      const ok = !!placeAt(at);
      e.dataTransfer.dropEffect = ok ? "copy" : "none";
      setOver((o) => (o && !o.open && o.at.key === at.key && o.at.edge === at.edge && o.ok === ok ? o : { at, ok }));
    },
    onDrop: (e: React.DragEvent) => {
      if (!isAgentDrag(e)) return;
      e.preventDefault();
      depth.current = 0;
      const was = over;
      setOver(null);
      if (was?.open) return;
      const at = atPoint(e);
      if (!at || !placeAt(at)) return;
      let a: DragAgent;
      try { a = JSON.parse(e.dataTransfer.getData(DRAG_TYPE)) as DragAgent; } catch { return; }
      if (openIn?.(a)) return;
      // (one pane shown, as tabs or with the whole room: an edge means
      // "a new pane", wherever there is room; key "")
      onDrop(a, alone && at.edge !== "center" ? { key: "", edge: at.edge } : at);
    },
  } : {};
  // The highlighted part of the room, and what letting go there does.
  const hint = (() => {
    if (!over) return null;
    if (over.open) {
      const r = alone ? WHOLE : rects.get(over.open);
      const name = slots.find((s) => s.key === over.open)?.title || "";
      return r ? { r, why: L(`${name} 已经开着了`, `${name} is open already`), open: true } : null;
    }
    const e = over.at.edge;
    const base = over.at.key === "*" ? WHOLE : alone ? WHOLE : rects.get(over.at.key);
    if (!base) return null;
    const r: Rect = placeAt(over.at) || (e === "center" ? base : halfOf(base, e));
    const full = keys.length >= MAX_PANES;
    const across = { top: L("在最上面多开一栏，横跨整个", "A new pane across the top"), bottom: L("在最下面多开一栏，横跨整个", "A new pane across the bottom"),
      left: L("在最左边多开一栏，竖着占满", "A new pane down the left"), right: L("在最右边多开一栏，竖着占满", "A new pane down the right"), center: "" };
    const why = over.ok ? (e === "center" ? L("换成它", "Show it here instead") : over.at.key === "*" ? across[e] : L("在这里多开一栏", "Open it in a new pane here"))
      : full ? L("最多四栏：放到一栏中间换掉它", "Four panes at most: drop on one to show it there instead")
      : L("放到边上，多开一栏", "Drop on an edge for a new pane");
    return { r, why, open: false };
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
          const r = alone ? (alone === s.key ? WHOLE : null) : rects.get(s.key);
          return (
            <div key={s.key} data-pane={s.key} className={cn("sp-pane pg-pane", s.main ? "sp-pane-left" : "sp-pane-right", !r && "is-hidden", fresh === s.key && "is-new", r && r.x > 0.0005 && "has-left", r && r.y > 0.0005 && "has-top")}
              style={r ? { left: pct(r.x), top: pct(r.y), width: pct(r.w), height: pct(r.h) } : undefined}
              onPointerDownCapture={() => onFocus?.(s.key)}>
              {s.node(buttons(s))}
            </div>
          );
        })}
        {lines.map((line) => (line.dir === "row" ? (
          <div key={`l${line.path.join("")}`} className="pg-line is-col" role="separator" aria-orientation="vertical" aria-label={L("拖动调整宽度", "Drag to resize")}
            style={{ left: pct(line.r.x + line.r.w * line.ratio), top: pct(line.r.y), height: pct(line.r.h) }}
            onPointerDown={startDrag(line)} onPointerMove={moveDrag} onPointerUp={endDrag} onPointerCancel={endDrag}
            onDoubleClick={() => onLayout({ root: withRatio(layout.root, line.path, 0.5) })} />
        ) : (
          <div key={`l${line.path.join("")}`} className="pg-line is-row" role="separator" aria-orientation="horizontal" aria-label={L("拖动调整高度", "Drag to resize")}
            style={{ top: pct(line.r.y + line.r.h * line.ratio), left: pct(line.r.x), width: pct(line.r.w) }}
            onPointerDown={startDrag(line)} onPointerMove={moveDrag} onPointerUp={endDrag} onPointerCancel={endDrag}
            onDoubleClick={() => onLayout({ root: withRatio(layout.root, line.path, 0.5) })} />
        )))}
        {hint && (
          <div className={cn("pg-drop", !over?.ok && "is-no", hint.open && "is-open")} style={{ left: pct(hint.r.x), top: pct(hint.r.y), width: pct(hint.r.w), height: pct(hint.r.h) }}>
            <span>{hint.why}</span>
          </div>
        )}
      </div>
    </div>
  );
}
