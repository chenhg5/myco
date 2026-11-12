import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";
import { L } from "@/lib/i18n";

/*
 * A node at work, at the foot of a conversation: a little pixel mushroom
 * (myco is mycelium) shuffling on the spot and shedding spores, with what
 * the node says it is doing or, when it says nothing, a playful verb that
 * changes every few seconds, and how long it has been at it.
 */

// The sprite on a 12×12 grid: C cap, o spot, S stem, e eye, . empty.
const BODY = [
  "....CCCC....",
  "..CCCCCCCC..",
  ".CCoCCCCoCC.",
  "CCCCCCCCCCCC",
  "CCCCCoCCCCCC",
  ".CCCCCCCCCC.",
  "...SSSSSS...",
  "...SeSSeS...",
  "...SSSSSS...",
  "...SSSSSS...",
];
// Two frames of legs, rows 10 and 11, right under the stem: one leg is
// lifted while the other stands, then the other way round.
const FEET = [
  ["...SS..SS...", "...SS......."],
  ["...SS..SS...", ".......SS..."],
];

function cells(rows: string[], top = 0) {
  const out: { x: number; y: number; k: string }[] = [];
  rows.forEach((row, y) => [...row].forEach((k, x) => { if (k !== ".") out.push({ x, y: y + top, k }); }));
  return out;
}

const KIND: Record<string, string> = { C: "is-cap", o: "is-spot", S: "is-stem", e: "is-eye" };

/** The mushroom: standing when idle (still blinking), else shuffling. */
export function Mushroom({ idle }: { idle?: boolean }) {
  return (
    <svg className={cn("work-sprite", idle && "is-idle")} viewBox="0 -4 12 16" width="30" height="40" shapeRendering="crispEdges" aria-hidden="true">
      {/* spores drifting up off the cap */}
      <rect className="work-spore" x="2" y="1" width="1" height="1" />
      <rect className="work-spore" x="9" y="0" width="1" height="1" />
      <rect className="work-spore" x="6" y="-1" width="1" height="1" />
      <g className="work-body">
        {cells(BODY).map((c) => <rect key={`${c.x},${c.y}`} className={KIND[c.k]} x={c.x} y={c.y} width="1" height="1" />)}
      </g>
      {FEET.map((frame, i) => (
        <g key={i} className={cn("work-feet", i === 1 && "is-b")}>
          {cells(frame, 10).map((c) => <rect key={`${c.x},${c.y}`} className="is-stem" x={c.x} y={c.y} width="1" height="1" />)}
        </g>
      ))}
    </svg>
  );
}

const VERBS: [string, string][] = [
  ["埋头干活", "Heads down"],
  ["琢磨中", "Mulling it over"],
  ["理一理头绪", "Untangling threads"],
  ["长菌丝中", "Growing mycelium"],
  ["推敲细节", "Weighing details"],
  ["捣鼓中", "Tinkering"],
  ["翻一翻资料", "Leafing through notes"],
  ["酝酿中", "Brewing"],
  ["一点点拼起来", "Piecing it together"],
  ["啃硬骨头", "Chewing on it"],
  ["散播孢子", "Spreading spores"],
  ["盘一盘", "Sorting it out"],
];

const TURN_MS = 3600;

function elapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return L(`${s} 秒`, `${s}s`);
  const m = Math.floor(s / 60);
  if (m < 60) return L(`${m} 分 ${s % 60} 秒`, `${m}m ${s % 60}s`);
  return L(`${Math.floor(m / 60)} 小时 ${m % 60} 分`, `${Math.floor(m / 60)}h ${m % 60}m`);
}

/** A playful verb for a node that says nothing about what it is doing,
 * changing every few seconds (off: a fixed one is not needed). */
export function useVerb(on = true): string {
  // A random one to start with, so two conversations do not say the same.
  const [turn, setTurn] = useState(() => Math.floor(Math.random() * VERBS.length));
  useEffect(() => {
    if (!on) return;
    const t = window.setInterval(() => setTurn((v) => (v + 1 + Math.floor(Math.random() * (VERBS.length - 1))) % VERBS.length), TURN_MS);
    return () => window.clearInterval(t);
  }, [on]);
  const [zh, en] = VERBS[turn];
  return L(zh, en);
}

/** How long since then, ticking every second. */
export function Elapsed({ since, className }: { since: string; className?: string }) {
  const start = new Date(since).getTime();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);
  if (Number.isNaN(start)) return null;
  return <span className={className}>{elapsed(now - start)}</span>;
}

export function WorkingLine({ name, since, status, side, queued, behind = 0 }: {
  /** who is working */
  name: string;
  /** when it took the task (for the clock) */
  since: string;
  /** what the node says it is doing, when it says */
  status?: string;
  /** the side of the conversation its messages are on */
  side: "left" | "right";
  /** not picked up yet: the mushroom stands and waits */
  queued?: boolean;
  /** more tasks for it waiting behind this one */
  behind?: number;
}) {
  const verb = useVerb(!status && !queued);
  const text = queued ? L("排队中，等它接手", "Queued, waiting to be picked up") : status || verb;
  return (
    <div className={cn("work-line", side === "right" && "is-right")} role="status" aria-live="off">
      <Mushroom idle={queued} />
      <div className="work-text">
        <span className="work-name">{name}</span>
        {/* keyed, so a new verb fades in */}
        <span key={text} className={cn("work-verb", queued && "is-still")}>{text}</span>
        <Elapsed since={since} className="work-time" />
        {behind > 0 && <span className="work-time">{L(`后面还有 ${behind} 个任务`, `${behind} more queued`)}</span>}
      </div>
    </div>
  );
}
