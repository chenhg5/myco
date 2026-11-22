// Browser column layout provides real typography-aware pagination. Persist a
// character anchor, not a page index, so resizing or changing type keeps place.
import { useLayoutEffect, useRef, useState } from "react";
import type { Settings } from "./model";
import { fonts } from "./model";
export type TextNavigation = {
  page: number;
  total: number;
  turn: (delta: number) => void;
  go: (position: number) => void;
  fraction: (value: number) => void;
};
export function TextView({
  text,
  settings,
  initial,
  onPosition,
  onNavigation,
}: {
  text: string;
  settings: Settings;
  initial: number;
  onPosition: (offset: number, progress: number) => void;
  onNavigation: (nav: TextNavigation) => void;
}) {
  const viewport = useRef<HTMLDivElement>(null),
    flow = useRef<HTMLDivElement>(null);
  const anchor = useRef(Math.max(0, Math.min(initial, text.length - 1))),
    index = useRef(0);
  const [layout, setLayout] = useState({ width: 1, height: 1, total: 1 }),
    [page, setPage] = useState(0);
  const stride = layout.width + 64;
  const callbacks = useRef({ onPosition, onNavigation });
  callbacks.current = { onPosition, onNavigation };
  function columnFor(offset: number, width: number) {
    const el = flow.current,
      node = el?.firstChild;
    if (!el || !node || !text.length) return 0;
    const range = document.createRange(),
      pos = Math.max(0, Math.min(text.length - 1, offset));
    range.setStart(node, pos);
    range.setEnd(node, pos + 1);
    const rect = range.getBoundingClientRect(),
      origin = el.getBoundingClientRect();
    return Math.max(
      0,
      Math.floor((rect.left - origin.left + 1) / (width + 64)),
    );
  }
  function firstOffset(column: number) {
    let a = 0,
      b = text.length - 1;
    while (a < b) {
      const m = (a + b) >>> 1;
      if (columnFor(m, layout.width) < column) a = m + 1;
      else b = m;
    }
    return a;
  }
  function navigate(p: number, offset?: number) {
    const next = Math.max(0, Math.min(layout.total - 1, p));
    index.current = next;
    setPage(next);
    anchor.current = offset ?? firstOffset(next);
    callbacks.current.onPosition(
      anchor.current,
      layout.total <= 1 ? 1 : next / (layout.total - 1),
    );
  }
  useLayoutEffect(() => {
    const box = viewport.current,
      el = flow.current;
    if (!box || !el) return;
    let cancelled = false;
    const measure = () => {
      if (cancelled) return;
      const width = Math.floor(box.clientWidth),
        height = Math.floor(box.clientHeight);
      if (!width || !height) return;
      el.style.width = `${width}px`;
      el.style.height = `${height}px`;
      el.style.columnWidth = `${width}px`;
      const total = Math.max(
        1,
        Math.round((el.scrollWidth + 64) / (width + 64)),
      );
      const next = Math.min(total - 1, columnFor(anchor.current, width));
      index.current = next;
      setLayout({ width, height, total });
      setPage(next);
      callbacks.current.onPosition(
        anchor.current,
        total <= 1 ? 1 : next / (total - 1),
      );
    };
    const observer = new ResizeObserver(measure);
    observer.observe(box);
    measure();
    document.fonts?.ready.then(measure);
    return () => {
      cancelled = true;
      observer.disconnect();
    };
  }, [
    text,
    settings.size,
    settings.line,
    settings.font,
    settings.bold,
    settings.width,
  ]);
  useLayoutEffect(() => {
    callbacks.current.onNavigation({
      page,
      total: layout.total,
      turn: (delta) => navigate(index.current + delta),
      go: (offset) => navigate(columnFor(offset, layout.width), offset),
      fraction: (f) => navigate(Math.round(f * (layout.total - 1))),
    });
  }, [page, layout, text]);
  return (
    <div className="rd-text-viewport" ref={viewport}>
      <div
        ref={flow}
        className="rd-text-flow"
        style={{
          transform: `translateX(${-page * stride}px)`,
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
