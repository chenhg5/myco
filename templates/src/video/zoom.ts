/** A focus point in normalized output coordinates; preview and export share it.
 * Ease both ends and keep edges in frame, including corner focus points. */
export function zoomTransform(o: {start: number; end: number; x: number; y: number; w: number; h: number; strength: number} | undefined, t: number) {
  if (!o || t < o.start || t >= o.end) return {scale: 1, x: .5, y: .5};
  const fade = Math.min(.35, (o.end - o.start) / 3);
  const a = Math.max(0, Math.min(1, (t - o.start) / fade, (o.end - t) / fade));
  const ease = a * a * (3 - 2 * a);
  const scale = 1 + (1 + o.strength * 2) * ease;
  const bound = .5 / scale;
  const limit = (v: number) => Math.min(1 - bound, Math.max(bound, v));
  return {scale, x: limit(.5 + (o.x + o.w / 2 - .5) * ease), y: limit(.5 + (o.y + o.h / 2 - .5) * ease)};
}
