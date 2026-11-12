import { useLayoutEffect, type RefObject } from "react";

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
 */

let mirror: HTMLTextAreaElement | null = null;

const COPIED = [
  "box-sizing", "font-family", "font-size", "font-weight", "font-style", "letter-spacing",
  "line-height", "text-transform", "word-spacing", "text-indent", "white-space", "word-break",
  "overflow-wrap", "tab-size", "padding-top", "padding-right", "padding-bottom", "padding-left",
  "border-top-width", "border-right-width", "border-bottom-width", "border-left-width",
] as const;

function heightOf(el: HTMLTextAreaElement): number {
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
  mirror.style.width = `${el.getBoundingClientRect().width}px`;
  mirror.value = el.value || el.placeholder || "";
  const border = cs.boxSizing === "border-box"
    ? parseFloat(cs.borderTopWidth) + parseFloat(cs.borderBottomWidth) : 0;
  return mirror.scrollHeight + border;
}

/** Keeps the box as tall as its text (at least its CSS min-height), up to max. */
export function useAutoGrow(ref: RefObject<HTMLTextAreaElement | null>, value: string, max: number) {
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const h = heightOf(el);
    const next = `${Math.min(h, max)}px`;
    if (el.style.height !== next) el.style.height = next;
    const over = h > max ? "auto" : "hidden";
    if (el.style.overflowY !== over) el.style.overflowY = over;
  }, [ref, value, max]);
}
