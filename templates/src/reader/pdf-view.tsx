// Render only the active PDF page, with a selectable text layer. Cancel and
// clean up both render tasks when changing pages, zoom, or closing the book.
import { useEffect, useRef, useState } from "react";
import { t as uiText } from "../_lib/i18n";
import type { PDFBook } from "./pdf";
import { pdfScale, type PDFSize } from "./pdf-layout";
import type {
  PDFPageProxy,
  RenderTask,
  TextLayer as PDFTextLayer,
} from "pdfjs-dist";
export function PDFPage({
  doc,
  page,
  zoom,
  width,
  fitHeight,
  onCover,
  onSize,
}: {
  doc: PDFBook;
  page: number;
  zoom: number;
  width: number;
  fitHeight?: number;
  onCover: (cover: string) => void;
  onSize?: (size: PDFSize) => void;
}) {
  const canvas = useRef<HTMLCanvasElement>(null),
    text = useRef<HTMLDivElement>(null);
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(true);
  const coverCallback = useRef(onCover),
    sizeCallback = useRef(onSize);
  coverCallback.current = onCover;
  sizeCallback.current = onSize;
  useEffect(() => {
    if (!width) return;
    let disposed = false,
      render: RenderTask | undefined,
      layer: PDFTextLayer | undefined,
      loadedPage: PDFPageProxy | undefined;
    setBusy(true);
    setError("");
    const currentCanvas = canvas.current!,
      currentText = text.current!;
    currentText.replaceChildren();
    (async () => {
      const pdfPage = await doc.getPage(page + 1);
      loadedPage = pdfPage;
      if (disposed) {
        pdfPage.cleanup();
        return;
      }
      const natural = pdfPage.getViewport({ scale: 1 }),
        scale = pdfScale(natural, width, zoom, fitHeight);
      sizeCallback.current?.({ width: natural.width, height: natural.height });
      const viewport = pdfPage.getViewport({ scale }),
        ratio = Math.min(2, window.devicePixelRatio || 1);
      // Bound canvas area for very large PDF page dimensions and high zoom.
      const density = Math.min(
        ratio,
        8192 / viewport.width,
        8192 / viewport.height,
        Math.sqrt(16000000 / (viewport.width * viewport.height)),
      );
      currentCanvas.width = Math.max(1, Math.floor(viewport.width * density));
      currentCanvas.height = Math.max(1, Math.floor(viewport.height * density));
      currentCanvas.style.width = `${viewport.width}px`;
      currentCanvas.style.height = `${viewport.height}px`;
      currentText.style.setProperty("--scale-factor", String(scale));
      currentText.style.setProperty("--total-scale-factor", String(scale));
      currentText.style.width = `${viewport.width}px`;
      currentText.style.height = `${viewport.height}px`;
      render = pdfPage.render({
        canvas: currentCanvas,
        viewport,
        transform: [density, 0, 0, density, 0, 0],
      });
      await render.promise;
      if (disposed) return;
      if (page === 0) {
        const thumb = document.createElement("canvas");
        thumb.width = 240;
        thumb.height = Math.min(
          600,
          Math.round((240 * viewport.height) / viewport.width),
        );
        thumb
          .getContext("2d")!
          .drawImage(currentCanvas, 0, 0, thumb.width, thumb.height);
        coverCallback.current(thumb.toDataURL("image/jpeg", 0.75));
      }
      const { TextLayer } = await import("pdfjs-dist/legacy/build/pdf.mjs");
      if (disposed) return;
      const content = await pdfPage.getTextContent();
      if (disposed) return;
      layer = new TextLayer({
        textContentSource: content,
        container: currentText,
        viewport,
      });
      await layer.render();
    })()
      .catch((e) => {
        if (!disposed && e?.name !== "RenderingCancelledException")
          setError(uiText("这一页未能显示，请重试或切换页面。"));
      })
      .finally(() => {
        if (!disposed) setBusy(false);
      });
    return () => {
      disposed = true;
      render?.cancel();
      layer?.cancel();
      loadedPage?.cleanup();
      currentText.replaceChildren();
    };
  }, [doc, page, width, zoom, fitHeight]);
  return (
    <div className="rd-pdf-leaf" aria-busy={busy} data-page={page + 1}>
      {error && (
        <p role="alert" className="rd-error">
          {error}
        </p>
      )}
      {busy && (
        <span className="rd-pdf-loading" role="status">
          {uiText("正在绘制页面…")}
        </span>
      )}
      <div
        className="rd-pdf-page"
        style={{ visibility: busy || error ? "hidden" : "visible" }}
      >
        <canvas ref={canvas} />
        <div className="rd-pdf-text textLayer" ref={text} />
      </div>
    </div>
  );
}

export function PDFView({
  doc,
  page,
  zoom,
  fitPage = false,
  onCover,
}: {
  doc: PDFBook;
  page: number;
  zoom: number;
  fitPage?: boolean;
  onCover: (cover: string) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const [geometry, setGeometry] = useState({ width: 0, height: 0 });
  useEffect(() => {
    const el = container.current!;
    const measure = () =>
      setGeometry({ width: el.clientWidth, height: el.clientHeight });
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    measure();
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (container.current) container.current.scrollTop = 0;
  }, [page]);
  return (
    <div className="rd-pdf-scroll" ref={container}>
      <PDFPage
        doc={doc}
        page={page}
        zoom={zoom}
        width={geometry.width}
        fitHeight={fitPage ? geometry.height : undefined}
        onCover={onCover}
      />
    </div>
  );
}
