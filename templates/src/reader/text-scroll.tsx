// Continuous TXT layout uses native scrolling. Character anchors let the same
// book move between columns and long-form layout without page-number guesses.
import { useLayoutEffect, useRef } from "react";
import { fonts } from "./model";
import type { PagedTextView, TextNavigation } from "./text-view";
export function ScrollTextView({
  text,
  settings,
  initial,
  onPosition,
  onNavigation,
}: Parameters<typeof PagedTextView>[0]) {
  const viewport = useRef<HTMLDivElement>(null),
    content = useRef<HTMLDivElement>(null);
  const anchor = useRef(initial),
    callbacks = useRef({ onPosition, onNavigation });
  callbacks.current = { onPosition, onNavigation };
  useLayoutEffect(() => {
    const box = viewport.current!,
      el = content.current!,
      node = el.firstChild!;
    let frame = 0,
      disposed = false,
      skipAt: number | null = null;
    const topFor = (offset: number) => {
      const r = document.createRange(),
        pos = Math.max(0, Math.min(text.length - 1, offset));
      r.setStart(node, pos);
      r.setEnd(node, pos + 1);
      return r.getBoundingClientRect().top - el.getBoundingClientRect().top;
    };
    const firstAt = (y: number) => {
      let a = 0,
        b = text.length - 1;
      const target = Math.max(0, y - settings.size * settings.line * 0.7);
      while (a < b) {
        const mid = (a + b) >>> 1;
        if (topFor(mid) < target) a = mid + 1;
        else b = mid;
      }
      return a;
    };
    const report = (keepAnchor = false) => {
      if (disposed) return;
      const max = Math.max(0, box.scrollHeight - box.clientHeight),
        y = box.scrollTop;
      if (!keepAnchor) anchor.current = firstAt(y);
      const total = Math.max(
        1,
        Math.ceil(max / Math.max(1, box.clientHeight)) + 1,
      );
      const page = max
        ? Math.min(total - 1, Math.round((y / max) * (total - 1)))
        : 0;
      const nav: TextNavigation = {
        page,
        total,
        turn: (delta) => {
          box.scrollTop += delta * box.clientHeight * 0.9;
        },
        go: (offset) => {
          anchor.current = offset;
          box.scrollTop = Math.min(max, topFor(offset));
          skipAt = box.scrollTop;
          report(true);
        },
        fraction: (value) => {
          box.scrollTop = value * max;
          report();
        },
      };
      callbacks.current.onNavigation(nav);
      callbacks.current.onPosition(
        anchor.current,
        max ? Math.min(1, y / max) : 1,
      );
    };
    const restore = () => {
      if (disposed) return;
      box.scrollTop = Math.min(
        box.scrollHeight - box.clientHeight,
        topFor(anchor.current),
      );
      skipAt = box.scrollTop;
      report(true);
    };
    const scrolled = () => {
      if (skipAt !== null && Math.abs(box.scrollTop - skipAt) < 0.5) {
        skipAt = null;
        return;
      }
      skipAt = null;
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => report());
    };
    box.addEventListener("scroll", scrolled, { passive: true });
    const observer = new ResizeObserver(restore);
    observer.observe(box);
    restore();
    document.fonts.ready.then(restore);
    return () => {
      disposed = true;
      observer.disconnect();
      box.removeEventListener("scroll", scrolled);
      cancelAnimationFrame(frame);
    };
  }, [
    text,
    settings.size,
    settings.line,
    settings.font,
    settings.bold,
    settings.width,
  ]);
  return (
    <div className="rd-text-scroll" ref={viewport} tabIndex={0}>
      <div
        ref={content}
        className="rd-text-scroll-content"
        style={{
          fontFamily: fonts[settings.font],
          fontSize: settings.size,
          lineHeight: settings.line,
          fontWeight: settings.bold ? 600 : 400,
        }}
      >
        {text}
      </div>
    </div>
  );
}
