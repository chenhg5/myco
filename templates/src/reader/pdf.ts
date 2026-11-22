// PDF parsing stays inside a local worker. Fonts and CMaps are shipped with
// this app; a book never triggers an external font or document fetch.
import type { PDFDocumentProxy, PDFDocumentLoadingTask } from "pdfjs-dist";
export type PDFBook = PDFDocumentProxy;
export async function loadPDF(
  file: Blob,
  password: (retry: boolean) => Promise<string>,
  signal: AbortSignal,
): Promise<PDFBook> {
  const [pdf, worker] = await Promise.all([
    import("pdfjs-dist/legacy/build/pdf.mjs"),
    import("pdfjs-dist/legacy/build/pdf.worker.min.mjs?url"),
  ]);
  if (signal.aborted) throw new DOMException("Aborted", "AbortError");
  pdf.GlobalWorkerOptions.workerSrc = worker.default;
  const task: PDFDocumentLoadingTask = pdf.getDocument({
    data: await file.arrayBuffer(),
    isEvalSupported: false,
    useWasm: true,
    wasmUrl: new URL("pdf/wasm/", document.baseURI).href,
    iccUrl: new URL("pdf/iccs/", document.baseURI).href,
    cMapUrl: new URL("pdf/cmaps/", document.baseURI).href,
    cMapPacked: true,
    standardFontDataUrl: new URL("pdf/standard_fonts/", document.baseURI).href,
  });
  const abort = () => {
    void task.destroy().catch(() => undefined);
  };
  signal.addEventListener("abort", abort, { once: true });
  task.onPassword = (update: (password: string) => void, reason: number) => {
    password(reason === 2).then((value) => {
      if (!signal.aborted) update(value);
    }, abort);
  };
  try {
    const doc = await task.promise;
    if (doc.numPages > 10000) {
      await doc.destroy();
      throw new Error("pdf_pages");
    }
    return doc;
  } catch (error) {
    signal.removeEventListener("abort", abort);
    throw error;
  }
}
export async function pdfChapters(
  doc: PDFBook,
): Promise<{ title: string; position: number }[]> {
  const outline = await doc.getOutline();
  const result: { title: string; position: number }[] = [];
  async function visit(items: NonNullable<typeof outline>, depth = 0) {
    if (depth > 10) return;
    for (const item of items) {
      if (result.length >= 500) return;
      try {
        const dest =
          typeof item.dest === "string"
            ? await doc.getDestination(item.dest)
            : item.dest;
        if (dest?.[0] != null) {
          const p =
            typeof dest[0] === "number"
              ? dest[0]
              : await doc.getPageIndex(dest[0]);
          result.push({ title: item.title, position: p });
        }
      } catch {
        /* Invalid bookmarks must not prevent reading the book. */
      }
      await visit(item.items, depth + 1);
    }
  }
  if (outline) await visit(outline);
  return result;
}
export async function searchPDF(
  doc: PDFBook,
  query: string,
  signal: AbortSignal,
  progress: (n: number) => void,
) {
  const result: { position: number; excerpt: string }[] = [],
    q = query.trim().toLocaleLowerCase();
  if (!q) return result;
  for (let p = 1; p <= doc.numPages && result.length < 100; p++) {
    if (signal.aborted) throw new DOMException("Aborted", "AbortError");
    const page = await doc.getPage(p),
      content = await page.getTextContent();
    const text = content.items.map((i) => ("str" in i ? i.str : "")).join(" "),
      at = text.toLocaleLowerCase().indexOf(q);
    if (at >= 0)
      result.push({
        position: p - 1,
        excerpt: text.slice(Math.max(0, at - 24), at + q.length + 65),
      });
    progress(p);
  }
  return result;
}
