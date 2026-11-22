// Size/index calculations shared by paged and virtual continuous PDF views.
export type PDFSize = { width: number; height: number };
export function pdfScale(
  size: PDFSize,
  width: number,
  zoom: number,
  fitHeight?: number,
) {
  return (
    Math.max(
      0.01,
      Math.min(
        (width - 24) / size.width,
        1.8,
        fitHeight ? (fitHeight - 24) / size.height : Infinity,
      ),
    ) * zoom
  );
}
export function pdfPageAt(offsets: number[], y: number) {
  let a = 0,
    b = offsets.length - 2;
  while (a < b) {
    const m = Math.ceil((a + b) / 2);
    if (offsets[m] <= y) a = m;
    else b = m - 1;
  }
  return a;
}
