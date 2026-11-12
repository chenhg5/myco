import type { Agent } from "@/api";
import type { AgentRow } from "@/app/types";
import { agentKind, cliIcon } from "@/app/nodes";
import { cn } from "@/lib/utils";
import { L } from "@/lib/i18n";

/*
 * How agents and workspaces look to people: a name that says what they do,
 * and a face. Every agent is a little pixel mushroom of its own (myco is
 * mycelium): its cap colour, spots and face come from its id, so it always
 * looks the same, everywhere, and no two look quite alike. A workspace (a
 * folder agents can run commands in) is a little terminal.
 */

const CAPS = ["#3fa37a", "#e0675a", "#e7a33e", "#5b8def", "#9b6bd6", "#e46aa5", "#33aebf", "#8a9a3c", "#d9824a", "#6f7fd8"];

// The cap (rows 0-5), with spots (o) in one of three patterns.
const CAP_PATTERNS = [
  ["....CCCC....", "..CCCCCCCC..", ".CCoCCCCoCC.", "CCCCCCCCCCCC", "CCCCCoCCCCCC", ".CCCCCCCCCC."],
  ["....CCCC....", "..CCCoCCCC..", ".CCCCCCCCoC.", "CoCCCCCCCCCC", "CCCCCCCoCCCC", ".CCCCCCCCCC."],
  ["....CoCC....", "..CCCCCCCC..", ".CCCCoCCCCC.", "CCCCCCCCCCoC", "CCoCCCCCCCCC", ".CCCCCCCCCC."],
];
// The stem and face (rows 6-11): eyes (e), glasses (g), blush (b).
const FACES = [
  ["...SSSSSS...", "...SeSSeS...", "...SSSSSS...", "...SSSSSS...", "...SS..SS...", "............"],
  ["...SSSSSS...", "..ggggggggg.", "...SeSSeS...", "...SSSSSS...", "...SS..SS...", "............"],
  ["...SSSSSS...", "...SeSSeS...", "...bSSSSb...", "...SSSSSS...", "...SS..SS...", "............"],
  ["...SSSSSS...", "...SSSSSS...", "...SeSSeS...", "...SSSSSS...", "...SS..SS...", "............"],
];

function hash(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export function looksOf(seed: string) {
  const h = hash(seed || "?");
  return {
    cap: CAPS[h % CAPS.length],
    pattern: CAP_PATTERNS[(h >>> 8) % CAP_PATTERNS.length],
    face: FACES[(h >>> 12) % FACES.length],
    // a sprout on top, now and then
    sprout: ((h >>> 16) % 4) === 0,
  };
}

const FILL: Record<string, string> = {
  C: "var(--av-cap)", o: "rgba(255,255,255,0.85)", S: "var(--av-stem)", e: "var(--av-eye)",
  g: "var(--av-eye)", b: "rgba(232,110,110,0.55)",
};

function MushroomFace({ seed }: { seed: string }) {
  const l = looksOf(seed);
  const rows = [...l.pattern, ...l.face];
  return (
    <svg className="pa-sprite" viewBox="0 -2 12 14" shapeRendering="crispEdges" aria-hidden="true">
      {l.sprout && <><rect x="6" y="-2" width="1" height="2" fill="#5aa86a" /><rect x="7" y="-2" width="1" height="1" fill="#5aa86a" /></>}
      {rows.flatMap((row, y) => [...row].map((k, x) => (k === "." ? null : <rect key={`${x},${y}`} x={x} y={y} width="1" height="1" fill={FILL[k]} />)))}
    </svg>
  );
}

// A workspace: a small window with a prompt.
const SCREEN = [
  "............",
  ".WWWWWWWWWW.",
  ".WFFFFFFFFW.",
  ".WFPFFFFFFW.",
  ".WFFPFFFFFW.",
  ".WFPFFFFFFW.",
  ".WFFFFLLLFW.",
  ".WFFFFFFFFW.",
  ".WWWWWWWWWW.",
  "....WWWW....",
  "...WWWWWW...",
  "............",
];
const SCREEN_FILL: Record<string, string> = { W: "var(--av-frame)", F: "var(--av-screen)", P: "#5fd7a7", L: "#cdd0d8" };

function WorkspaceFace() {
  return (
    <svg className="pa-sprite" viewBox="0 0 12 12" shapeRendering="crispEdges" aria-hidden="true">
      {SCREEN.flatMap((row, y) => [...row].map((k, x) => (k === "." ? null : <rect key={`${x},${y}`} x={x} y={y} width="1" height="1" fill={SCREEN_FILL[k]} />)))}
    </svg>
  );
}

/** Whether a node is a workspace (a place agents run commands in). */
export function isWorkspace(a: Agent): boolean {
  return agentKind(a) === "shell";
}

/**
 * An agent's or a workspace's face. With state, a small dot says whether it
 * is at work (green), waiting (amber) or away (grey); idle shows none.
 */
export function Avatar({ agent, size = 32, state, badge = true, className }: {
  agent: Agent;
  size?: number;
  state?: "working" | "pending" | "idle" | "offline" | string;
  /** a small mark for its kind (Claude, Codex, ...) */
  badge?: boolean;
  className?: string;
}) {
  const ws = isWorkspace(agent);
  const l = looksOf(agent.id || agent.name);
  // A person is a person: the first letter of their name, in a circle.
  if (agent.kind === "human" || agent.binding.harness === "human") {
    const name = agent.display_name || agent.name;
    return (
      <span className={cn("pa is-person", className)} style={{ width: size, height: size, fontSize: Math.round(size * 0.45), ["--av-cap" as string]: l.cap }} aria-hidden="true">
        {[...name.trim()][0]?.toUpperCase() || "?"}
        {state && state !== "idle" && <span className={cn("pa-state", `is-${state}`)} />}
      </span>
    );
  }
  const Badge = cliIcon(agent);
  const cli = agent.runtime?.cli || agent.binding.harness || "";
  return (
    <span
      className={cn("pa", ws && "is-workspace", state === "offline" && "is-away", className)}
      style={{ width: size, height: size, ["--av-cap" as string]: l.cap }}
      aria-hidden="true"
    >
      {ws ? <WorkspaceFace /> : <MushroomFace seed={agent.id || agent.name} />}
      {badge && !ws && size >= 28 && (
        <span className={cn("pa-badge", cli.startsWith("claude") && "is-claude", cli.startsWith("codex") && "is-codex")}><Badge /></span>
      )}
      {state && state !== "idle" && <span className={cn("pa-state", `is-${state}`)} />}
    </span>
  );
}

/** A few faces overlapping, for a project's members in a small space. */
export function AvatarStack({ agents, size = 22, max = 4 }: { agents: Agent[]; size?: number; max?: number }) {
  const shown = agents.slice(0, max);
  return (
    <span className="pa-stack">
      {shown.map((a) => <Avatar key={a.id || a.name} agent={a} size={size} badge={false} />)}
      {agents.length > max && <span className="pa-more" style={{ height: size, minWidth: size }}>+{agents.length - max}</span>}
    </span>
  );
}

/** What a state is called, in plain words. */
export function stateWords(state: string, statusText?: string): string {
  if (state === "working") return statusText || L("在干活", "Working");
  if (state === "pending") return L("等着接活", "About to start");
  if (state === "offline") return L("不在线", "Away");
  return L("空闲", "Free");
}

/** Where a workspace is, in words: its folder, on which computer. */
export function whereOf(r: Pick<AgentRow, "hub" | "st">): string {
  const a = r.st.agent;
  const m = (r.hub.overview?.machines || []).find((x) => x.id === a.machine_id);
  const dir = a.binding.workdir || "";
  return [m?.name, dir].filter(Boolean).join(" · ") || L("一个文件夹", "A folder");
}
