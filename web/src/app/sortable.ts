import { useEffect, useRef, useState } from "react";

/*
 * A list in the sidebar you can reorder by dragging (projects, computers),
 * the way a phone's list does it: press and move, and the row lifts and
 * follows the pointer while the others slide out of its way; let go and it
 * glides into its place. Nothing is reordered under the pointer while it
 * moves (only moved by transforms), so the rows never jump.
 *
 * Pointer events, not the browser's drag and drop (unreliable in the
 * app's WebKit, and it drags a ghost picture). The order is kept per
 * workspace in this browser. A drag is not a click. Rows not placed yet (a
 * new project) keep their usual place, above the ones that were.
 */

const PREFIX = "myco-nav-order";
const SLIDE = "transform 0.18s cubic-bezier(0.2, 0.8, 0.2, 1)";

function load(key: string): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(`${PREFIX}\u0000${key}`) || "[]");
    return Array.isArray(v) ? v.filter((x) => typeof x === "string") : [];
  } catch {
    return [];
  }
}

function save(key: string, ids: string[]) {
  try { localStorage.setItem(`${PREFIX}\u0000${key}`, JSON.stringify(ids)); } catch { /* not kept */ }
}

/** ids in the kept order; ones never placed first, in their own order. */
export function inOrder(ids: string[], kept: string[]): string[] {
  const at = new Map(kept.map((id, i) => [id, i]));
  const fresh = ids.filter((id) => !at.has(id));
  const placed = ids.filter((id) => at.has(id)).sort((a, b) => at.get(a)! - at.get(b)!);
  return [...fresh, ...placed];
}

type Drag = {
  id: string;
  y0: number;
  start: string[];
  from: number;
  /** each row's middle (y) when the drag began, in start order */
  mids: number[];
  /** how far one row is from the next (its height and the gap) */
  step: number;
  active: boolean;
};

type View = { id: string; dy: number; target: number } | null;
type Settle = { id: string; offset: number; moving: boolean } | null;

export function useSortable(key: string, ids: string[]) {
  const [kept, setKept] = useState<string[]>(() => load(key));
  const [view, setView] = useState<View>(null);
  const [settle, setSettle] = useState<Settle>(null);
  const drag = useRef<Drag | null>(null);
  const swallowClick = useRef(false);
  useEffect(() => { setKept(load(key)); setView(null); setSettle(null); }, [key]);

  const order = inOrder(ids, kept);

  const onPointerDown = (id: string) => (e: React.PointerEvent<HTMLElement>) => {
    if (e.button !== 0) return;
    const list = e.currentTarget.parentElement;
    const at = new Map<string, number>();
    list?.querySelectorAll<HTMLElement>("[data-sort-id]").forEach((el) => {
      const r = el.getBoundingClientRect();
      at.set(el.dataset.sortId || "", r.top + r.height / 2);
    });
    const start = order.filter((x) => at.has(x));
    const mids = start.map((x) => at.get(x)!);
    const from = start.indexOf(id);
    if (from < 0) return;
    const step = mids.length > 1 ? (mids[mids.length - 1] - mids[0]) / (mids.length - 1) : 0;
    drag.current = { id, y0: e.clientY, start, from, mids, step, active: false };

    let frame = 0;
    let lastY = e.clientY;
    const paint = () => {
      frame = 0;
      const d = drag.current;
      if (!d) return;
      const dy = lastY - d.y0;
      // Where its middle is now, among the others' middles.
      const mid = d.mids[d.from] + dy;
      let target = d.from;
      while (target < d.mids.length - 1 && mid > d.mids[target + 1]) target++;
      while (target > 0 && mid < d.mids[target - 1]) target--;
      setView({ id: d.id, dy, target });
    };
    const move = (ev: PointerEvent) => {
      const d = drag.current;
      if (!d) return;
      lastY = ev.clientY;
      if (!d.active) {
        if (Math.abs(ev.clientY - d.y0) < 4) return;
        d.active = true;
        setSettle(null);
        document.body.classList.add("is-sorting");
      }
      if (!frame) frame = requestAnimationFrame(paint);
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      if (frame) cancelAnimationFrame(frame);
      const d = drag.current;
      drag.current = null;
      document.body.classList.remove("is-sorting");
      if (!d?.active) return;
      swallowClick.current = true;
      window.setTimeout(() => { swallowClick.current = false; }, 0);
      // Where it was let go, against where it lands: it glides the rest.
      const dy = lastY - d.y0;
      let target = d.from;
      const mid = d.mids[d.from] + dy;
      while (target < d.mids.length - 1 && mid > d.mids[target + 1]) target++;
      while (target > 0 && mid < d.mids[target - 1]) target--;
      const next = d.start.filter((x) => x !== d.id);
      next.splice(target, 0, d.id);
      const rest = dy - (target - d.from) * d.step;
      setView(null);
      setKept(next);
      save(key, next);
      setSettle({ id: d.id, offset: rest, moving: false });
      requestAnimationFrame(() => requestAnimationFrame(() => setSettle({ id: d.id, offset: 0, moving: true })));
      window.setTimeout(() => setSettle((s) => (s && s.id === d.id ? null : s)), 260);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
  };

  // How a row is moved while one is dragged: the dragged one with the
  // pointer, the ones between where it was and where it would go by one
  // row, the other way.
  const styleOf = (id: string): React.CSSProperties | undefined => {
    if (settle && settle.id === id) {
      return { transform: `translateY(${settle.offset}px)`, transition: settle.moving ? SLIDE : "none", position: "relative", zIndex: 2 };
    }
    const d = drag.current;
    if (!view || !d) return undefined;
    if (id === view.id) return { transform: `translateY(${view.dy}px)`, transition: "none", position: "relative", zIndex: 3 };
    const i = d.start.indexOf(id);
    let shift = 0;
    if (d.from < view.target && i > d.from && i <= view.target) shift = -d.step;
    else if (d.from > view.target && i >= view.target && i < d.from) shift = d.step;
    return { transform: shift ? `translateY(${shift}px)` : undefined, transition: SLIDE };
  };

  /** What a row needs: spread it on the row's element. */
  const bind = (id: string) => ({
    "data-sort-id": id,
    onPointerDown: onPointerDown(id),
    style: styleOf(id),
    // A drag that ends on a row must not open it.
    onClickCapture: (e: React.MouseEvent) => {
      if (swallowClick.current) { e.preventDefault(); e.stopPropagation(); }
    },
  });

  return { order, bind, dragging: view?.id ?? (settle?.moving === false ? settle.id : null) };
}
