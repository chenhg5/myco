import { useEffect, useRef, useState } from "react";
import { MessageContent } from "@/app/Markdown";
import { cn } from "@/lib/utils";

/*
 * A reply that arrives while you watch grows in, a block at a time
 * (paragraphs, list items; a code block or a table whole), at a reading
 * pace, never longer than about two seconds in all. A short one just
 * appears: typing it out would only make it slower. What is shown is
 * always the start of the text itself, so the last step is the message as
 * it is drawn anyway, and nothing moves when it ends. Scrolling, a click or
 * a key shows the rest at once; with reduced motion there is none of this.
 */

/** What has grown in already (once each), and tasks whose answer was
 *  shown while being written (it does not grow in again as the reply). */
export const grown = new Set<string>();
export const drafted = new Set<string>();

/** Characters a second, and the whole reveal at most (ms). */
const PACE = 450;
const BUDGET = 1800;
const SHORT = 80;
/** Longer bodies start folded (MessageContent): shown at once. */
const FOLDS_AFTER = 30;

/** Where the body's blocks end: line counts after each block. */
export function blockEnds(body: string): number[] {
  const lines = body.split("\n");
  const ends: number[] = [];
  let fence = false;
  let table = false;
  let inBlock = false;
  const item = /^([-*+]|\d+[.)])\s/;
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i];
    if (/^\s*(```|~~~)/.test(l)) {
      if (!fence && inBlock) { ends.push(i); inBlock = false; }
      fence = !fence;
      inBlock = true;
      if (!fence) { ends.push(i + 1); inBlock = false; }
      continue;
    }
    if (fence) continue;
    const isTable = /^\s*\|/.test(l);
    if (!l.trim()) {
      if (inBlock) ends.push(i);
      inBlock = false;
      table = false;
      continue;
    }
    // A new list item, or a table starting, ends what came before.
    if (inBlock && (item.test(l) || (isTable && !table))) ends.push(i);
    inBlock = true;
    table = isTable;
  }
  if (inBlock || fence) ends.push(lines.length);
  return [...new Set(ends)].filter((n) => n > 0);
}

/** Whether a body is worth revealing at all. */
export function revealable(body: string): boolean {
  const lines = body.split("\n").length;
  if (lines > FOLDS_AFTER || /^\s*[[{]/.test(body)) return false;
  return body.trim().length > SHORT || blockEnds(body).length > 1;
}

const reducedMotion = () => {
  try { return window.matchMedia("(prefers-reduced-motion: reduce)").matches; } catch { return false; }
};

/** `streaming` accepts an unfinished body: each appended suffix grows in
 * before the next poll, while completed replies keep the block reveal. */
export function Reveal({ body, onDone, streaming = false, intervalMs = 1500, plain, cursor, onGrow }: {
  body: string; onDone?: () => void; streaming?: boolean; intervalMs?: number;
  plain?: boolean; cursor?: boolean; onGrow?: () => void;
}) {
  const lines = body.split("\n");
  const ends = blockEnds(body).map((n) => lines.slice(0, n).join("\n").length);
  const [shown, setShown] = useState(() => reducedMotion() ? body.length
    : streaming ? 0 : !revealable(body) ? body.length : ends[0] || 0);
  const visible = useRef(shown);
  const frame = useRef(0);
  const done = shown >= body.length;
  const doneRef = useRef(onDone);
  doneRef.current = onDone;
  const growRef = useRef(onGrow);
  growRef.current = onGrow;

  useEffect(() => { growRef.current?.(); }, [shown]);

  useEffect(() => {
    if (streaming) return;
    if (done) { doneRef.current?.(); return; }
    // Each block waits for the one before to be read, within the budget.
    const sizes = ends.map((e, i) => e - (i ? ends[i - 1] : 0));
    const raw = sizes.map((n) => Math.min(420, Math.max(70, (n / PACE) * 1000)));
    const total = raw.slice(0, -1).reduce((a, b) => a + b, 0);
    const scale = total > BUDGET ? BUDGET / total : 1;
    const next = ends.findIndex((n) => n > shown);
    const t = window.setTimeout(() => setShown(ends[next] ?? body.length), (raw[Math.max(0, next - 1)] || 70) * scale);
    return () => window.clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shown, done, streaming, body]);

  useEffect(() => {
    if (!streaming) return;
    const from = Math.min(visible.current, body.length);
    const start = performance.now();
    const draw = (n: number) => { visible.current = n; setShown(n); };
    const tick = (now: number) => {
      const fraction = Math.min(1, (now - start) / Math.max(1, intervalMs - 100));
      let n = from + Math.ceil((body.length - from) * fraction);
      // Keep emoji and other UTF-16 surrogate pairs together.
      if (n < body.length && /[\uD800-\uDBFF]/.test(body[n - 1] || "")) n++;
      draw(n);
      if (fraction < 1) frame.current = requestAnimationFrame(tick);
    };
    const motion = window.matchMedia("(prefers-reduced-motion: reduce)");
    const finish = () => {
      if (motion.matches) { cancelAnimationFrame(frame.current); draw(body.length); }
    };
    if (motion.matches) draw(body.length);
    else frame.current = requestAnimationFrame(tick);
    motion.addEventListener("change", finish);
    return () => { cancelAnimationFrame(frame.current); motion.removeEventListener("change", finish); };
  }, [body, streaming, intervalMs]);

  // Any sign of reading on your own: all of the received body now.
  useEffect(() => {
    if (done) return;
    const all = () => {
      cancelAnimationFrame(frame.current);
      visible.current = body.length;
      setShown(body.length);
    };
    const opts = { capture: true, passive: true } as const;
    window.addEventListener("wheel", all, opts);
    window.addEventListener("pointerdown", all, opts);
    window.addEventListener("keydown", all, opts);
    return () => {
      window.removeEventListener("wheel", all, opts);
      window.removeEventListener("pointerdown", all, opts);
      window.removeEventListener("keydown", all, opts);
    };
  }, [done, body.length]);

  const text = done ? body : body.slice(0, shown);
  return (
    <div className={cn("reveal", !done && "is-growing", streaming && "is-streaming")}>
      {plain ? text : <MessageContent body={text} />}
      {cursor && <span className="trace-cursor" aria-hidden />}
    </div>
  );
}
