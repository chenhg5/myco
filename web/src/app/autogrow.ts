import { useEffect, useLayoutEffect, type RefObject } from "react";

/*
 * A text box that grows with what is typed, as chat apps' do, up to max
 * pixels, then scrolls. (field-sizing: content is not in every WebKit the
 * app runs on.)
 *
 * It is measured on a hidden copy, never by setting the box's own height
 * to auto first: that collapses it for a moment, the conversation above
 * gets taller, the browser clamps its scroll position, and the
 * conversation jumps on every key. It runs before the frame is drawn, and
 * the box only gets a scroll bar once it is past max.
 *
 * Only what is typed counts (an empty box is one line, whatever its
 * placeholder says), and nothing is measured while the box has no width
 * yet (a pane still opening): that made every letter a line and the empty
 * box very tall. It is measured again when its width changes.
 */

let mirror: HTMLTextAreaElement | null = null;

const COPIED = [
  "box-sizing", "font-family", "font-size", "font-weight", "font-style", "letter-spacing",
  "line-height", "text-transform", "word-spacing", "text-indent", "white-space", "word-break",
  "overflow-wrap", "tab-size", "padding-top", "padding-right", "padding-bottom", "padding-left",
  "border-top-width", "border-right-width", "border-bottom-width", "border-left-width",
] as const;

function heightOf(el: HTMLTextAreaElement): number | null {
  const width = el.getBoundingClientRect().width;
  if (width < 40) return null;
  if (!mirror) {
    mirror = document.createElement("textarea");
    mirror.setAttribute("aria-hidden", "true");
    mirror.tabIndex = -1;
    Object.assign(mirror.style, {
      position: "fixed", top: "-10000px", left: "-10000px", visibility: "hidden",
      height: "0", minHeight: "0", maxHeight: "none", overflow: "hidden", pointerEvents: "none",
    });
    document.body.appendChild(mirror);
  }
  const cs = getComputedStyle(el);
  for (const p of COPIED) mirror.style.setProperty(p, cs.getPropertyValue(p));
  mirror.style.width = `${width}px`;
  mirror.value = el.value || " ";
  const border = cs.boxSizing === "border-box"
    ? parseFloat(cs.borderTopWidth) + parseFloat(cs.borderBottomWidth) : 0;
  return mirror.scrollHeight + border;
}

/** Keeps the box as tall as its text (at least its CSS min-height), up to max. */
function fit(el: HTMLTextAreaElement, max: number) {
  const h = heightOf(el);
  if (h === null) return;
  const next = `${Math.min(h, max)}px`;
  if (el.style.height !== next) el.style.height = next;
  const over = h > max ? "auto" : "hidden";
  if (el.style.overflowY !== over) el.style.overflowY = over;
}

export function useAutoGrow(ref: RefObject<HTMLTextAreaElement | null>, value: string, max: number) {
  useLayoutEffect(() => {
    const el = ref.current;
    if (el) fit(el, max);
  }, [ref, value, max]);
  // A new width (the pane opened, was dragged, the window resized): the
  // same text may take more or fewer lines.
  useEffect(() => {
    const el = ref.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    let w = el.getBoundingClientRect().width;
    const ro = new ResizeObserver(() => {
      const nw = el.getBoundingClientRect().width;
      if (Math.abs(nw - w) < 0.5) return;
      w = nw;
      fit(el, max);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref, max]);
}
