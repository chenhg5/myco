import { useEffect, useRef, useState } from "react";

/*
 * A list in the sidebar you can reorder by dragging (projects, computers).
 * Pointer events, not the browser's drag and drop (unreliable in the app's
 * WebKit and it drags a ghost picture): press, move a few pixels, and the
 * row follows the pointer while the others make room; let go and the order
 * is kept, per workspace, in this browser. A drag is not a click. Rows not
 * placed yet (a new project) keep their usual place, above the ones that
 * were.
 */

const PREFIX = "myco-nav-order";

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
  /** the rows' middles (y) when the drag began, by id */
  mids: Map<string, number>;
  active: boolean;
};

export function useSortable(key: string, ids: string[]) {
  const [kept, setKept] = useState<string[]>(() => load(key));
  const [live, setLive] = useState<string[] | null>(null);
  const [dragging, setDragging] = useState<string | null>(null);
  const drag = useRef<Drag | null>(null);
  const swallowClick = useRef(false);
  useEffect(() => { setKept(load(key)); setLive(null); }, [key]);

  const order = live || inOrder(ids, kept);

  const onPointerDown = (id: string) => (e: React.PointerEvent<HTMLElement>) => {
    if (e.button !== 0) return;
    const list = e.currentTarget.parentElement;
    const mids = new Map<string, number>();
    list?.querySelectorAll<HTMLElement>("[data-sort-id]").forEach((el) => {
      const r = el.getBoundingClientRect();
      mids.set(el.dataset.sortId || "", r.top + r.height / 2);
    });
    drag.current = { id, y0: e.clientY, start: order, mids, active: false };
    const move = (ev: PointerEvent) => {
      const d = drag.current;
      if (!d) return;
      if (!d.active) {
        if (Math.abs(ev.clientY - d.y0) < 5) return;
        d.active = true;
        setDragging(d.id);
        document.body.classList.add("is-sorting");
      }
      // Where it would go: after every other row whose middle is above
      // the pointer.
      const others = d.start.filter((x) => x !== d.id);
      const idx = others.filter((x) => (d.mids.get(x) ?? Infinity) < ev.clientY).length;
      const next = [...others.slice(0, idx), d.id, ...others.slice(idx)];
      setLive((cur) => (cur && cur.join("\u0000") === next.join("\u0000") ? cur : next));
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      const d = drag.current;
      drag.current = null;
      document.body.classList.remove("is-sorting");
      setDragging(null);
      if (!d?.active) return;
      swallowClick.current = true;
      window.setTimeout(() => { swallowClick.current = false; }, 0);
      setLive((cur) => {
        if (cur) { setKept(cur); save(key, cur); }
        return null;
      });
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
  };

  /** What a row needs: spread it on the row's element. */
  const bind = (id: string) => ({
    "data-sort-id": id,
    onPointerDown: onPointerDown(id),
    // A drag that ends on a row must not open it.
    onClickCapture: (e: React.MouseEvent) => {
      if (swallowClick.current) { e.preventDefault(); e.stopPropagation(); }
    },
  });

  return { order, bind, dragging };
}
