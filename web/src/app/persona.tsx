import { runnerUpdating } from "@/lib/runner-state";
import { useState } from "react";
import type { Agent } from "@/api";
import { appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { agentKind, cliIcon } from "@/app/nodes";
import { cn } from "@/lib/utils";
import { useIsSuperagent } from "@/app/superagentMark";
import { L } from "@/lib/i18n";

/*
 * How agents and workspaces look to people: a name that says what they do,
 * and a face. Every agent is a little pixel mushroom of its own (myco is
 * mycelium): its cap colour, spots and face come from its id, so it always
 * looks the same, everywhere, and no two look quite alike. A workspace (a
 * folder agents can run commands in) is a little terminal.
 */

const CAPS = ["#3fa37a", "#e0675a", "#e7a33e", "#5b8def", "#9b6bd6", "#e46aa5", "#33aebf", "#8a9a3c", "#d9824a", "#6f7fd8"];

// The cap's outline (rows y = -2..5, 12 wide): six shapes that tell apart
// even at 20px. 0 is the dome every face had before shapes came.
const SHAPES = [
  // dome
  ["............", "............", "....CCCC....", "..CCCCCCCC..", ".CCCCCCCCCC.", "CCCCCCCCCCCC", "CCCCCCCCCCCC", ".CCCCCCCCCC."],
  // cone: a pointed top
  [".....CC.....", "....CCCC....", "....CCCC....", "...CCCCCC...", "..CCCCCCCC..", ".CCCCCCCCCC.", "CCCCCCCCCCCC", ".CCCCCCCCCC."],
  // flat: wide and low
  ["............", "............", "............", "............", "............", ".CCCCCCCCCC.", "CCCCCCCCCCCC", "CCCCCCCCCCCC"],
  // bell: tall and narrow
  ["............", "....CCCC....", "...CCCCCC...", "...CCCCCC...", "...CCCCCC...", "..CCCCCCCC..", "..CCCCCCCC..", "..CCCCCCCC.."],
  // frill: a dome with a scalloped edge
  ["............", "............", "....CCCC....", "..CCCCCCCC..", ".CCCCCCCCCC.", "CCCCCCCCCCCC", "CCCCCCCCCCCC", "C.CC.CC.CC.C"],
  // brim: a small crown on a wide brim
  ["............", "............", "............", "....CCCC....", "...CCCCCC...", "...CCCCCC...", "CCCCCCCCCCCC", ".CCCCCCCCCC."],
];
// Where accessories sit on each shape: its top, left and right shoulders [x, y].
const ANCHORS: { top: [number, number]; left: [number, number]; right: [number, number] }[] = [
  { top: [6, 0], left: [2, 1], right: [9, 1] },
  { top: [6, -2], left: [3, 1], right: [8, 1] },
  { top: [6, 3], left: [1, 3], right: [10, 3] },
  { top: [6, -1], left: [3, 0], right: [8, 0] },
  { top: [6, 0], left: [2, 1], right: [9, 1] },
  { top: [6, 1], left: [4, 1], right: [7, 1] },
];
// The spots, as [x, y] on the cap (one of three patterns; a spot falls
// only where the shape has cap).
const SPOTS: [number, number][][] = [
  [[3, 2], [8, 2], [5, 4]],
  [[5, 1], [9, 2], [1, 3], [7, 4]],
  [[5, 0], [5, 2], [10, 3], [2, 4]],
];
// The stem and face (rows 6-11): eyes (e), glasses (g), blush (b); the
// stem's right edge in shade (T), so it stands out from a pale tile.
const FACES = [
  ["...SSSSST...", "...SeSSeT...", "...SSSSST...", "...SSSSST...", "...SS..ST...", "............"],
  ["...SSSSST...", "..ggggggggg.", "...SeSSeT...", "...SSSSST...", "...SS..ST...", "............"],
  ["...SSSSST...", "...SeSSeT...", "...bSSSSb...", "...SSSSST...", "...SS..ST...", "............"],
  ["...SSSSST...", "...SSSSST...", "...SeSSeT...", "...SSSSST...", "...SS..ST...", "............"],
];
// What it wears (one at most): 0 nothing, 1 a sprout (as before), then a
// leaf, a bow, headphones, a scarf, a star and a flower; with the shapes
// each one fits (a sprout needs room on top, headphones a dome).
const ACCESSORIES: { name: string; fits?: number[] }[] = [
  { name: "none" }, { name: "sprout", fits: [0, 2, 4, 5] }, { name: "leaf" }, { name: "bow" },
  { name: "headphones", fits: [0, 4] }, { name: "scarf" }, { name: "star" }, { name: "flower" },
];

function hash(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

/** Whether an accessory can be worn on a shape. */
export function accessoryFits(acc: number, shape: number): boolean {
  const f = ACCESSORIES[acc]?.fits;
  return !f || f.includes(shape);
}

/** A drawn face's parts, as indexes: cap colour, spots, face, cap shape and
 *  what it wears (sprout is acc 1, kept for words from before shapes). */
export type Look = { cap: number; pattern: number; face: number; sprout: boolean; shape: number; acc: number };

/** How many of each part there are (for a picker). */
export const LOOK_PARTS = { caps: CAPS, patterns: SPOTS.length, faces: FACES.length, shapes: SHAPES.length, accessories: ACCESSORIES.map((a) => a.name) };

/** The parts drawn from a seed (an agent's id): its default face. The
 *  colour, spots and face are what they always were; the shape and what it
 *  wears come from a second hash, so they vary apart from the colour. */
export function lookOfSeed(seed: string): Look {
  const h = hash(seed || "?");
  const h2 = hash("\u0001" + (seed || "?"));
  const shape = h2 % SHAPES.length;
  const fit = ACCESSORIES.map((_, i) => i).filter((i) => i > 0 && accessoryFits(i, shape));
  // about one in four wears nothing
  const k = (h2 >>> 8) % (fit.length + 2);
  const acc = k < fit.length ? fit[k] : 0;
  return { cap: h % CAPS.length, pattern: (h >>> 8) % SPOTS.length, face: (h >>> 12) % FACES.length, sprout: acc === 1, shape, acc };
}

/** The parts of a chosen drawn face, or null. "look:c3-p1-f2-s1" is one
 *  chosen before shapes: the dome, a sprout or nothing, as it always was;
 *  newer ones add "-h<shape>-a<accessory>". */
export function parseLook(avatar?: string): Look | null {
  const m = /^look:c(\d+)-p(\d+)-f(\d+)-s([01])(?:-h(\d+)-a(\d+))?$/.exec(avatar || "");
  if (!m) return null;
  const [cap, pattern, face] = [Number(m[1]), Number(m[2]), Number(m[3])];
  if (cap >= CAPS.length || pattern >= SPOTS.length || face >= FACES.length) return null;
  const sprout = m[4] === "1";
  const shape = m[5] === undefined ? 0 : Number(m[5]);
  const acc = m[6] === undefined ? (sprout ? 1 : 0) : Number(m[6]);
  if (shape >= SHAPES.length || acc >= ACCESSORIES.length) return null;
  return { cap, pattern, face, sprout: acc === 1, shape, acc };
}

/** The avatar word a hub keeps for a drawn face: as before for a dome with
 *  a sprout or nothing (older apps read it), else with its shape and what
 *  it wears. */
export function lookWord(l: Look): string {
  const acc = l.acc ?? (l.sprout ? 1 : 0);
  const shape = l.shape ?? 0;
  const base = `look:c${l.cap}-p${l.pattern}-f${l.face}-s${acc === 1 ? 1 : 0}`;
  return shape === 0 && acc <= 1 ? base : `${base}-h${shape}-a${acc}`;
}

/** In one workspace, faces drawn from a seed that would share a colour and
 *  a shape with an earlier agent's take the next free shape, then colour
 *  (rememberWorkspaceFaces). By agent id. */
const assigned = new Map<string, Look>();

/** The parts of a face: the chosen ones, else the workspace's pick for it,
 *  else the ones from its seed. */
export function lookOf(seed: string, avatar?: string): Look {
  return parseLook(avatar) || assigned.get(seed) || lookOfSeed(seed);
}

/** What a face looks like: its parts' rows and colours. */
export function looksOf(seed: string, avatar?: string) {
  const l = lookOf(seed, avatar);
  const outline = SHAPES[l.shape] || SHAPES[0];
  const spots = new Set(SPOTS[l.pattern].map(([x, y]) => `${x},${y}`));
  // the cap rows y = -2..5, spots where the cap is
  const cap = outline.map((row, i) => [...row].map((k, x) => (k === "C" && spots.has(`${x},${i - 2}`) ? "o" : k)).join(""));
  return { cap: CAPS[l.cap], rows: cap, face: FACES[l.face], shape: l.shape, acc: accessoryFits(l.acc, l.shape) ? l.acc : 0 };
}

const FILL: Record<string, string> = {
  C: "var(--av-cap)", o: "rgba(255,255,255,0.85)", S: "var(--av-stem)", T: "var(--av-stem-shade)", e: "var(--av-eye)",
  g: "var(--av-eye)", b: "rgba(232,110,110,0.55)",
};

/** What it wears, as [x, y, colour] pixels on its shape. */
function accessoryPixels(acc: number, shape: number): [number, number, string][] {
  const a = ANCHORS[shape] || ANCHORS[0];
  const [tx, ty] = a.top;
  const [lx, ly] = a.left;
  const [rx, ry] = a.right;
  const green = "#5aa86a";
  switch (ACCESSORIES[acc]?.name) {
    case "sprout": return [[tx, ty - 2, green], [tx, ty - 1, green], [tx + 1, ty - 2, green]];
    case "leaf": return [[lx, ly - 1, "#3f8f4f"], [lx + 1, ly - 1, green], [lx - 1, ly, green], [lx, ly, "#3f8f4f"]];
    case "bow": return [[rx - 1, ry - 1, "#f0507a"], [rx + 1, ry - 1, "#f0507a"], [rx - 1, ry, "#f0507a"], [rx, ry, "#b8325a"], [rx + 1, ry, "#f0507a"]];
    case "headphones": {
      const ink = "#3b3f4a";
      return [...[2, 3, 4, 5, 6, 7, 8, 9].map((x) => [x, -1, ink] as [number, number, string]),
        [1, 0, ink], [10, 0, ink], [0, 1, ink], [1, 1, ink], [0, 2, ink], [1, 2, ink], [10, 1, ink], [11, 1, ink], [10, 2, ink], [11, 2, ink]];
    }
    case "scarf": return [...[2, 3, 4, 5, 6, 7, 8, 9].map((x) => [x, 9, "#d9433f"] as [number, number, string]), [9, 10, "#d9433f"], [9, 11, "#b8322f"]];
    case "star": return [[10, -2, "#f2b632"], [9, -1, "#f2b632"], [10, -1, "#fff3c4"], [11, -1, "#f2b632"], [10, 0, "#f2b632"]];
    case "flower": {
      const [fx, fy] = [lx + 1, ly];
      return [[fx, fy - 1, "#ffffff"], [fx - 1, fy, "#ffffff"], [fx + 1, fy, "#ffffff"], [fx, fy + 1, "#ffffff"], [fx, fy, "#f2b632"]];
    }
    default: return [];
  }
}

export function MushroomFace({ seed, avatar }: { seed: string; avatar?: string }) {
  const l = looksOf(seed, avatar);
  const px = (row: string, y: number) => [...row].map((k, x) => (k === "." ? null : <rect key={`${x},${y}`} x={x} y={y} width="1" height="1" fill={FILL[k]} />));
  return (
    <svg className="pa-sprite" viewBox="0 -2 12 14" shapeRendering="crispEdges" aria-hidden="true" data-shape={l.shape} data-acc={l.acc}>
      {l.rows.flatMap((row, i) => px(row, i - 2))}
      {l.face.flatMap((row, i) => px(row, i + 6))}
      {accessoryPixels(l.acc, l.shape).map(([x, y, c]) => <rect key={`a${x},${y}`} x={x} y={y} width="1" height="1" fill={c} />)}
    </svg>
  );
}

/** Spreads the faces drawn from seeds in one workspace: agents in the order
 *  they joined (so a newcomer never changes an older one's face), each
 *  keeping its own face unless an earlier one already has its colour and
 *  shape; then it takes the next free shape, else the next free colour.
 *  Chosen faces count as taken and stay as they are. */
export function rememberWorkspaceFaces(agents: { id: string; created_at?: string; avatar?: string }[]) {
  const byAge = [...agents].sort((a, b) => (a.created_at || "").localeCompare(b.created_at || "") || a.id.localeCompare(b.id));
  const taken = new Set<string>();
  const key = (l: Look) => `${l.cap}/${l.shape}`;
  for (const a of byAge) {
    const chosen = parseLook(a.avatar);
    if (chosen) { taken.add(key(chosen)); continue; }
    if (a.avatar?.startsWith("pic:")) continue;
    const own = lookOfSeed(a.id);
    let pick = own;
    if (taken.has(key(own))) {
      search: for (let dc = 0; dc < CAPS.length; dc++) {
        for (let ds = 0; ds < SHAPES.length; ds++) {
          const shape = (own.shape + ds) % SHAPES.length;
          const cap = (own.cap + dc) % CAPS.length;
          if (!taken.has(`${cap}/${shape}`)) {
            const acc = accessoryFits(own.acc, shape) ? own.acc : 0;
            pick = { ...own, cap, shape, acc, sprout: acc === 1 };
            break search;
          }
        }
      }
    }
    taken.add(key(pick));
    if (pick === own) assigned.delete(a.id); else assigned.set(a.id, pick);
  }
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

// A computer looks like the system it runs: an apple (graphite, or silver
// in the dark) for a Mac, the four window panes for Windows, a penguin for Linux (WSL
// included); a plain screen when the system is not known (or in a cloud).
const MAC = [
  "......aa....",
  ".....aa.....",
  "..aaa..aaa..",
  ".aaaaaaaaaa.",
  ".aaaaaaaaa..",
  ".aaaaaaaa...",
  ".aaaaaaaa...",
  ".aaaaaaaaa..",
  ".aaaaaaaaaa.",
  "..aaaaaaaa..",
  "...aa..aa...",
  "............",
];
const WINDOWS = [
  "............",
  "............",
  "..1111.2222.",
  "..1111.2222.",
  "..1111.2222.",
  "..1111.2222.",
  "............",
  "..3333.4444.",
  "..3333.4444.",
  "..3333.4444.",
  "..3333.4444.",
  "............",
];
const PENGUIN = [
  "............",
  "....KKKK....",
  "...KKKKKK...",
  "..KKEKKEKK..",
  "..KKKOOKKK..",
  "..KYYYYYYK..",
  ".KKYYYYYYKK.",
  ".KKYYYYYYKK.",
  "..KYYYYYYK..",
  "..KKYYYYKK..",
  "...OO..OO...",
  "............",
];
const FACE_FILL: Record<string, string> = {
  ...SCREEN_FILL,
  S: "#c9ccd4", C: "#e7e9ee", M: "#6d5ef0", N: "#9a5fe6", R: "#d96aa8",
  B: "#2f6fe0", T: "#ffffff", b: "#a9c8ff",
  K: "var(--av-penguin)", E: "#ffffff", O: "#f0a43a", Y: "#f4f1e8",
  // the apple: graphite on light, silver on dark
  a: "var(--av-apple)",
  // the four window panes
  "1": "#f25022", "2": "#7fba00", "3": "#00a4ef", "4": "#ffb900",
};
const OS_FACE: Record<string, string[]> = { darwin: MAC, windows: WINDOWS, linux: PENGUIN };

/** Each machine's system, by machine id, as the hubs report it: the app
 *  tells it here once per refresh, and computers' faces follow it. */
const machineOs = new Map<string, string>();
export function rememberMachines(ms: { id: string; os?: string }[]) {
  for (const m of ms) if (m.os) machineOs.set(m.id, m.os);
}

function WorkspaceFace({ os }: { os?: string }) {
  const grid = (os && OS_FACE[os]) || SCREEN;
  return (
    <svg className={cn("pa-sprite", os && OS_FACE[os] && `is-${os}`)} viewBox="0 0 12 12" shapeRendering="crispEdges" aria-hidden="true">
      {grid.flatMap((row, y) => [...row].map((k, x) => (k === "." ? null : <rect key={`${x},${y}`} x={x} y={y} width="1" height="1" fill={FACE_FILL[k]} />)))}
    </svg>
  );
}

/** Which workspace each agent is on (by agent id), so a face that is a
 *  picture can be asked for: the app tells it here once per refresh. */
const agentHub = new Map<string, string>();
export function rememberAgentHubs(rows: { hubId: string; agentId: string }[]) {
  for (const r of rows) agentHub.set(r.agentId, r.hubId);
}

/** Where the picture an agent chose as its face is, if it chose one. */
export function pictureOf(agent: Agent): string | null {
  if (!agent.avatar?.startsWith("pic:")) return null;
  const hub = agentHub.get(agent.id);
  return hub ? appApi.avatarUrl(hub, agent.name, agent.avatar) : null;
}

/** Whether a node is a workspace (a place agents run commands in). */
export function isWorkspace(a: Agent): boolean {
  return agentKind(a) === "shell";
}

/**
 * An agent's or a workspace's face. With state, a small dot says whether it
 * is at work (green), waiting (amber) or away (grey); idle shows none.
 */
/** A superagent's crown: pixel gold, like the mushrooms, worn on the tile. */
function Crown() {
  return (
    <svg className="pa-crown" viewBox="0 0 9 7" shapeRendering="crispEdges" aria-hidden="true">
      <g fill="#f2b632">
        <rect x="0" y="1" width="1" height="5" /><rect x="1" y="2" width="1" height="4" />
        <rect x="2" y="3" width="1" height="3" /><rect x="3" y="1" width="1" height="5" />
        <rect x="4" y="0" width="1" height="6" /><rect x="5" y="1" width="1" height="5" />
        <rect x="6" y="3" width="1" height="3" /><rect x="7" y="2" width="1" height="4" />
        <rect x="8" y="1" width="1" height="5" />
      </g>
      <rect x="0" y="6" width="9" height="1" fill="#c8861b" />
      <rect x="2" y="4" width="1" height="1" fill="#e5484d" />
      <rect x="4" y="4" width="1" height="1" fill="#3e9cf0" />
      <rect x="6" y="4" width="1" height="1" fill="#e5484d" />
    </svg>
  );
}

export function Avatar({ agent, size = 32, state, badge = true, className }: {
  agent: Agent;
  size?: number;
  state?: "working" | "pending" | "idle" | "offline" | string;
  /** a small mark for its kind (Claude, Codex, ...) */
  badge?: boolean;
  className?: string;
}) {
  if (runnerUpdating(agent)) state = "pending";
  const ws = isWorkspace(agent);
  // Your superagent wears a crown.
  const isSuper = useIsSuperagent(agent);
  const l = looksOf(agent.id || agent.name, agent.avatar);
  const pic = pictureOf(agent);
  // A picture that will not load (an older workspace, gone) falls back.
  const [broken, setBroken] = useState<string | null>(null);
  const picture = pic && broken !== pic ? <img className="pa-pic" src={pic} alt="" draggable={false} onError={() => setBroken(pic)} /> : null;
  // A person is a person: the first letter of their name, in a circle.
  if (agent.kind === "human" || agent.binding.harness === "human") {
    const name = agent.display_name || agent.name;
    return (
      <span className={cn("pa is-person", picture && "has-pic", className)} style={{ width: size, height: size, fontSize: Math.round(size * 0.45), ["--av-cap" as string]: l.cap }} aria-hidden="true">
        {picture || [...name.trim()][0]?.toUpperCase() || "?"}
        {state && state !== "idle" && <span className={cn("pa-state", `is-${state}`)} />}
      </span>
    );
  }
  const Badge = cliIcon(agent);
  const cli = agent.runtime?.cli || agent.binding.harness || "";
  return (
    <span
      className={cn("pa", ws && "is-workspace", picture && "has-pic", state === "offline" && "is-away", isSuper && "is-super", className)}
      style={{ width: size, height: size, ["--av-cap" as string]: l.cap }}
      aria-hidden="true"
    >
      {isSuper && <Crown />}
      {picture || (ws ? <WorkspaceFace os={agent.machine_id ? machineOs.get(agent.machine_id) : undefined} /> : <MushroomFace seed={agent.id || agent.name} avatar={agent.avatar} />)}
      {badge && !ws && size >= 28 && (
        <span className={cn("pa-badge", cli.startsWith("claude") && "is-claude", cli.startsWith("codex") && "is-codex")}>
          {cli.startsWith("codex") ? <CodexMark /> : <Badge />}
        </span>
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
export function stateWords(state: string, statusText?: string, updating = false): string {
  if (updating) return L("正在更新", "Updating");
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

/** Codex's small mark: a blue-violet disc with a white prompt (>_), as
 *  Claude has its orange asterisk. */
function CodexMark() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true" className="pa-codex">
      <path d="M3.4 5.5 6.6 8 3.4 10.5M8.2 10.6h4.2" fill="none" stroke="#fff" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
