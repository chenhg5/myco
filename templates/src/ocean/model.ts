// The page's records: the world (one record: its name and the shared
// weather, hour, sea and wind; where a voyage starts) and the islands
// (each: where, how big, what kind, a note people read on arriving).
// Where your own boat is stays on this computer only.

import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
import { isleOf, type Isle } from "./islands";
import type { Settings } from "./world";
import { WEATHERS, type Weather } from "./sky";
import type { Sea } from "./waves";

export type WorldRec = { title?: string; time?: number; time_speed?: number; time_at?: number; weather?: Weather; sea?: Sea; wind?: number; start_x?: number; start_z?: number; start_heading?: number };
export type IslandRec = { name?: string; x?: number; z?: number; size?: number; height?: number; kind?: string; palms?: number; seed?: number; note?: string };

export const world = new Store<WorldRec & Record<string, unknown>>("world");
export const islands = new Store<IslandRec & Record<string, unknown>>("islands");
export const kit = new Kit([world, islands], { quiet: true });

const num = (v: unknown, d: number) => (typeof v === "number" && Number.isFinite(v) ? v : d);

export const SEA_STATES: Sea[] = ["calm", "moderate", "rough"];

export function worldOf(): WorldRec {
  return world.get("main")?.data || {};
}

export function settingsOf(w = worldOf()): Settings {
  return {
    time: ((num(w.time, 15) % 24) + 24) % 24,
    weather: WEATHERS.includes(w.weather as Weather) ? (w.weather as Weather) : "cloudy",
    sea: SEA_STATES.includes(w.sea as Sea) ? (w.sea as Sea) : "moderate",
    wind: ((num(w.wind, 225) % 360) + 360) % 360,
    speed: Math.min(1440, Math.max(0, num(w.time_speed, 0))),
    at: num(w.time_at, 0),
  };
}

/** How fast the sea's day can run: hours of it per real hour. */
export const SPEEDS = [0, 30, 120];

export function startOf(w = worldOf()) {
  return { x: num(w.start_x, 0), z: num(w.start_z, 0), heading: (num(w.start_heading, 0) * Math.PI) / 180 };
}

export function isleList(): Isle[] {
  return islands.rows.map((r) => isleOf(r.id, r.data));
}

/** Changes some of the world's settings (everyone sees them). */
export function setWorld(patch: Partial<WorldRec>) {
  return kit.track(world.put({ ...worldOf(), ...patch }, "main"));
}

// ---- this computer only: where my boat is, what I have found ----

type Local = { x: number; z: number; heading: number; found: string[]; soundOn: boolean };
const key = () => `myco-ocean:${kit.page || "page"}`;

export function loadLocal(): Partial<Local> {
  try { return JSON.parse(localStorage.getItem(key()) || "{}"); } catch { return {}; }
}
export function saveLocal(v: Partial<Local>) {
  try { localStorage.setItem(key(), JSON.stringify({ ...loadLocal(), ...v })); } catch { /* (private window) */ }
}
