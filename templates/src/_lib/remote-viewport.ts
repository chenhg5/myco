/** Local viewport only. Zoom and pan never send input to the remote computer. */
export type View = { scale: number; x: number; y: number };
export type Point = { x: number; y: number };
const clamp = (n: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, n));
export function boundedView(view: View, width: number, height: number): View {
  const scale = clamp(view.scale, 1, 5);
  return {scale, x: clamp(view.x, width * (1 - scale), 0), y: clamp(view.y, height * (1 - scale), 0)};
}
export function zoomView(view: View, scale: number, anchor: Point, width: number, height: number): View {
  scale = clamp(scale, 1, 5);
  const ratio = scale / view.scale;
  return boundedView({scale, x: anchor.x - (anchor.x - view.x) * ratio, y: anchor.y - (anchor.y - view.y) * ratio}, width, height);
}
export function pinchView(start: View, ratio: number, anchor: Point, center: Point, width: number, height: number): View {
  const scale = clamp(start.scale * ratio, 1, 5), factor = scale / start.scale;
  return boundedView({scale, x: center.x - (anchor.x - start.x) * factor, y: center.y - (anchor.y - start.y) * factor}, width, height);
}
/** Keep a moved remote pointer visible without forcing a pinched view to follow. */
export function followPointer(view: View, point: Point, width: number, height: number): View {
  const margin = Math.min(32, width / 8, height / 8);
  const x = point.x * view.scale + view.x, y = point.y * view.scale + view.y;
  return boundedView({...view, x: view.x + (x < margin ? margin-x : x > width-margin ? width-margin-x : 0), y: view.y + (y < margin ? margin-y : y > height-margin ? height-margin-y : 0)}, width, height);
}
