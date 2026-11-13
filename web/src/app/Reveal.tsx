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

export function Reveal({ body, onDone }: { body: string; onDone?: () => void }) {
  const lines = body.split("\n");
  const ends = blockEnds(body);
  const [shown, setShown] = useState(() => (reducedMotion() || !revealable(body) ? ends.length : 1));
  const done = shown >= ends.length;
  const doneRef = useRef(onDone);
  doneRef.current = onDone;

  useEffect(() => {
    if (done) { doneRef.current?.(); return; }
    // Each block waits for the one before to be read, within the budget.
    const sizes = ends.map((e, i) => lines.slice(i ? ends[i - 1] : 0, e).join("\n").length);
    const raw = sizes.map((n) => Math.min(420, Math.max(70, (n / PACE) * 1000)));
    const total = raw.slice(0, -1).reduce((a, b) => a + b, 0);
    const scale = total > BUDGET ? BUDGET / total : 1;
    const t = window.setTimeout(() => setShown((s) => s + 1), raw[shown - 1] * scale);
    return () => window.clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shown, done]);

  // Any sign of reading on your own: all of it now.
  useEffect(() => {
    if (done) return;
    const all = () => setShown(ends.length);
    const opts = { capture: true, passive: true } as const;
    window.addEventListener("wheel", all, opts);
    window.addEventListener("pointerdown", all, opts);
    window.addEventListener("keydown", all, opts);
    return () => {
      window.removeEventListener("wheel", all, opts);
      window.removeEventListener("pointerdown", all, opts);
      window.removeEventListener("keydown", all, opts);
    };
  }, [done, ends.length]);

  const text = done ? body : lines.slice(0, ends[shown - 1]).join("\n");
  return (
    <div className={cn("reveal", !done && "is-growing")}>
      <MessageContent body={text} />
    </div>
  );
}
