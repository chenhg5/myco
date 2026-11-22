// Virtual, variable-height PDF list. Only visible pages plus a neighbour are
// painted; the rest are space, so long books never create thousands of canvases.
import { useLayoutEffect, useMemo, useRef, useState } from "react";
import type { PDFBook } from "./pdf";
import { pdfPageAt, pdfScale, type PDFSize } from "./pdf-layout";
import { PDFPage } from "./pdf-view";
import type { TextNavigation } from "./text-view";
export function PDFScrollView({
  doc,
  zoom,
  initial,
  initialOffset,
  onPosition,
  onNavigation,
  onCover,
}: {
  doc: PDFBook;
  zoom: number;
  initial: number;
  initialOffset: number;
  onPosition: (page: number, progress: number, pageOffset: number) => void;
  onNavigation: (n: TextNavigation) => void;
  onCover: (cover: string) => void;
}) {
  const box = useRef<HTMLDivElement>(null),
    anchor = useRef({ page: initial, fraction: initialOffset });
  const callbacks = useRef({ onPosition, onNavigation, onCover });
  callbacks.current = { onPosition, onNavigation, onCover };
  const [geometry, setGeometry] = useState({ width: 0, height: 0 });
  const [sizes, setSizes] = useState<Map<number, PDFSize>>(new Map());
  const [visible, setVisible] = useState({
    start: Math.max(0, initial - 1),
    end: Math.min(doc.numPages - 1, initial + 1),
  });
  const offsets = useMemo(() => {
    const fallback = sizes.get(0) || { width: 612, height: 792 },
      positions = [0];
    for (let page = 0; page < doc.numPages; page++) {
      const size = sizes.get(page) || fallback;
      positions.push(
        positions[page] +
          pdfScale(size, Math.max(100, geometry.width), zoom) * size.height +
          20,
      );
    }
    return positions;
  }, [sizes, geometry.width, zoom, doc]);
  useLayoutEffect(() => {
    const el = box.current!,
      measure = () =>
        setGeometry({ width: el.clientWidth, height: el.clientHeight });
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    measure();
    return () => observer.disconnect();
  }, []);
  useLayoutEffect(() => {
    if (!geometry.width || !geometry.height) return;
    const el = box.current!;
    let frame = 0,
      skipAt: number | null = null;
    const report = () => {
      const y = el.scrollTop,
        page = pdfPageAt(offsets, y + 1);
      const height = offsets[page + 1] - offsets[page];
      anchor.current = {
        page,
        fraction: Math.max(0, Math.min(1, (y - offsets[page]) / height)),
      };
      const max = Math.max(0, offsets[doc.numPages] - el.clientHeight);
      setVisible({
        start: Math.max(0, page - 1),
        end: Math.min(
          doc.numPages - 1,
          pdfPageAt(offsets, y + el.clientHeight) + 1,
        ),
      });
      callbacks.current.onPosition(
        page,
        max ? Math.min(1, y / max) : 1,
        anchor.current.fraction,
      );
      callbacks.current.onNavigation({
        page,
        total: doc.numPages,
        turn: (delta) => {
          el.scrollTop += delta * el.clientHeight * 0.9;
        },
        go: (target, fraction = 0) => {
          const page = Math.max(0, Math.min(doc.numPages - 1, target));
          el.scrollTop =
            offsets[page] +
            Math.max(0, Math.min(1, fraction)) *
              (offsets[page + 1] - offsets[page]);
          report();
        },
        fraction: (fraction) => {
          el.scrollTop = fraction * max;
          report();
        },
      });
    };
    const saved = anchor.current;
    el.scrollTop =
      offsets[saved.page] +
      saved.fraction * (offsets[saved.page + 1] - offsets[saved.page]);
    skipAt = el.scrollTop;
    report();
    const scroll = () => {
      if (skipAt !== null && Math.abs(el.scrollTop - skipAt) < 0.5) {
        skipAt = null;
        return;
      }
      skipAt = null;
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(report);
    };
    el.addEventListener("scroll", scroll, { passive: true });
    return () => {
      el.removeEventListener("scroll", scroll);
      cancelAnimationFrame(frame);
    };
  }, [offsets, geometry.height, doc]);
  const rememberSize = (page: number, size: PDFSize) => {
    const before = sizes.get(page);
    if (before?.width === size.width && before.height === size.height) return;
    // Capture a wheel/touch movement even when its animation-frame report has
    // not run yet; resolving a neighbour's real size must not rewind a fling.
    const y = box.current?.scrollTop || 0,
      current = pdfPageAt(offsets, y);
    anchor.current = {
      page: current,
      fraction: Math.max(
        0,
        Math.min(
          1,
          (y - offsets[current]) / (offsets[current + 1] - offsets[current]),
        ),
      ),
    };
    setSizes((old) => {
      const next = new Map(old);
      next.set(page, size);
      return next;
    });
  };
  return (
    <div className="rd-pdf-continuous" ref={box} tabIndex={0}>
      <div
        className="rd-pdf-document"
        style={{ height: offsets[doc.numPages] }}
      >
        {Array.from(
          { length: visible.end - visible.start + 1 },
          (_, i) => visible.start + i,
        ).map((page) => (
          <div
            key={page}
            className="rd-pdf-slot"
            style={{
              top: offsets[page],
              height: offsets[page + 1] - offsets[page],
            }}
          >
            <PDFPage
              doc={doc}
              page={page}
              zoom={zoom}
              width={geometry.width}
              onCover={(cover) => callbacks.current.onCover(cover)}
              onSize={(size) => rememberSize(page, size)}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
