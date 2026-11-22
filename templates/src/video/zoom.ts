/** Preview and export share the same camera motion. Default transitions leave
 * time to orient, hold the focus, then ease back without a velocity jump. */
export type FocusZoom = {start: number; end: number; x: number; y: number; w: number; h: number; strength: number; fade_in?: number; fade_out?: number};
export function zoomTransform(o: FocusZoom | undefined, t: number) {
  if (!o || t < o.start || t >= o.end) return {scale: 1, x: .5, y: .5};
  const duration = o.end - o.start;
  const seconds = (v: number | undefined) => Number.isFinite(v) ? Math.max(.1, Math.min(3, v!)) : .9;
  let enter = seconds(o.fade_in), leave = seconds(o.fade_out);
  // Fit short clips without losing their fully zoomed hold (at least 20%).
  const fit = Math.min(1, duration * .8 / (enter + leave));
  enter *= fit; leave *= fit;
  const a = Math.max(0, Math.min(1, (t - o.start) / enter, (o.end - t) / leave));
  const ease = a * a * a * (a * (a * 6 - 15) + 10); // smootherstep: zero velocity and acceleration at both ends
  const scale = 1 + (1 + o.strength * 2) * ease;
  const bound = .5 / scale;
  const limit = (v: number) => Math.min(1 - bound, Math.max(bound, v));
  return {scale, x: limit(.5 + (o.x + o.w / 2 - .5) * ease), y: limit(.5 + (o.y + o.h / 2 - .5) * ease)};
}
