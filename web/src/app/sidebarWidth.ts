import { useCallback, useEffect, useRef, useState } from "react";

/*
 * The sidebar is as wide as you drag it: its right edge is a handle. The
 * width stays within limits (the page beside it keeps room), is kept in
 * this App, and a double-click puts it back. Dragged far to the left it
 * hides, as the sidebar button does; shown again it has the width it had.
 * Arrow keys move the edge too.
 */

const KEY = "myco-app-sidebar-w";
export const SIDEBAR_DEFAULT = 232;
const MIN = 200;
const MAX = 420;
/** Let go to the left of this and the sidebar hides. */
const HIDE_BELOW = 140;
/** The page beside it keeps at least this much. */
const PAGE_MIN = 520;

/** How many screen pixels one CSS pixel of the sidebar takes (the text
 *  size zooms the whole App), and how wide the App is in CSS pixels. */
function scale(): { ratio: number; room: number } {
  const side = document.querySelector<HTMLElement>(".app-sidebar");
  const shell = document.querySelector<HTMLElement>(".app-shell");
  const css = side ? parseFloat(getComputedStyle(side).width) : 0;
  const ratio = side && css > 0 ? side.getBoundingClientRect().width / css : 1;
  const room = (shell?.getBoundingClientRect().width ?? window.innerWidth) / (ratio || 1);
  return { ratio: ratio || 1, room };
}

function clamp(w: number, room = scale().room): number {
  const most = Math.max(MIN, Math.min(MAX, room - PAGE_MIN));
  return Math.round(Math.min(most, Math.max(MIN, w)));
}

function load(): number {
  try {
    const v = Number(localStorage.getItem(KEY));
    if (Number.isFinite(v) && v > 0) return clamp(v, window.innerWidth);
  } catch { /* storage unavailable */ }
  return SIDEBAR_DEFAULT;
}

function save(w: number) {
  try {
    if (w === SIDEBAR_DEFAULT) localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, String(w));
  } catch { /* storage unavailable */ }
}

export function useSidebarWidth(onHide: () => void) {
  const [width, setWidth] = useState(load);
  const [dragging, setDragging] = useState(false);
  // Under the hiding line while dragging: the edge says so before you let go.
  const [hiding, setHiding] = useState(false);
  const hideRef = useRef(onHide);
  hideRef.current = onHide;

  // A smaller window leaves the page its room.
  useEffect(() => {
    const fit = () => setWidth((w) => clamp(w));
    window.addEventListener("resize", fit);
    return () => window.removeEventListener("resize", fit);
  }, []);

  const onPointerDown = useCallback((e: React.PointerEvent<HTMLElement>) => {
    if (e.button !== 0) return;
    e.preventDefault();
    const x0 = e.clientX;
    const w0 = width;
    const { ratio, room } = scale();
    let last = w0;
    let raw = w0;
    let frame = 0;
    setDragging(true);
    document.body.classList.add("is-resizing");
    const move = (ev: PointerEvent) => {
      raw = w0 + (ev.clientX - x0) / ratio;
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        last = clamp(raw, room);
        setWidth(last);
        setHiding(raw < HIDE_BELOW);
      });
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      if (frame) cancelAnimationFrame(frame);
      document.body.classList.remove("is-resizing");
      setDragging(false);
      setHiding(false);
      if (raw < HIDE_BELOW) {
        // Hidden with the width it had before this drag.
        setWidth(w0);
        hideRef.current();
        return;
      }
      last = clamp(raw, room);
      setWidth(last);
      save(last);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
  }, [width]);

  const set = (w: number) => {
    const next = clamp(w);
    setWidth(next);
    save(next);
  };

  /** What the edge needs: spread it on the handle. */
  const handle = {
    role: "separator",
    "aria-orientation": "vertical" as const,
    "aria-valuenow": width,
    "aria-valuemin": MIN,
    "aria-valuemax": MAX,
    tabIndex: 0,
    onPointerDown,
    onDoubleClick: () => set(SIDEBAR_DEFAULT),
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === "ArrowLeft") { e.preventDefault(); set(width - 16); }
      else if (e.key === "ArrowRight") { e.preventDefault(); set(width + 16); }
      else if (e.key === "Home" || e.key === "Enter") { e.preventDefault(); set(SIDEBAR_DEFAULT); }
    },
  };

  return { width, dragging, hiding, handle };
}
