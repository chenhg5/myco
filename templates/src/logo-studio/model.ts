// Shared geometry and briefs. Agent proposals use data fields rather than SVG documents.
import { Kit } from "../_lib/appkit";
import { Store } from "../_lib/myco";

export const size = 256;
export const maxLayers = 32;
export const maxLayersBytes = 45_000;
export type Shape = "path" | "circle" | "rect" | "line";
export type Layer = {
  id: string; name: string; shape: Shape; x: number; y: number;
  w: number; h: number; d: string; rotation: number;
  fill: string; stroke: string; weight: number; guide: boolean;
};
export type Brief = { brand: string; brief: string; order: number; base?: string; };
export type Concept = { request: string; title: string; rationale: string; layers: unknown; order: number };
export const briefs = new Store<Brief>("briefs");
export const concepts = new Store<Concept>("concepts");
export const kit = new Kit([briefs, concepts], { quiet: true });

const color = /^(?:none|#[0-9a-f]{6})$/i;
const pathData = /^[Mm][0-9eE ,.+\-MmLlHhVvCcSsQqTtAaZz]*$/;
function bounded(value: unknown, fallback: number, min: number, max: number) {
  return typeof value === "number" && Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback;
}

export function readLayers(input: unknown): Layer[] {
  if (!Array.isArray(input)) return [];
  const ids = new Set<string>();
  return input.slice(0, maxLayers).flatMap((item) => {
    if (!item || typeof item !== "object" || !["circle", "rect", "line", "path"].includes(item.shape)) return [];
    if (typeof item.id !== "string" || !item.id || item.id.length > 40 || ids.has(item.id)) return [];
    const d = typeof item.d === "string" ? item.d : "";
    if (item.shape === "path" && (d.length > 4000 || !pathData.test(d))) return [];
    ids.add(item.id);
    return [{
      id: item.id, name: typeof item.name === "string" ? item.name.slice(0, 60) : "", shape: item.shape,
      x: bounded(item.x, 0, -size * 2, size * 2), y: bounded(item.y, 0, -size * 2, size * 2),
      w: bounded(item.w, 64, -size * 2, size * 2), h: bounded(item.h, 64, -size * 2, size * 2),
      d, rotation: bounded(item.rotation, 0, -360, 360),
      fill: typeof item.fill === "string" && color.test(item.fill) ? item.fill : "none",
      stroke: typeof item.stroke === "string" && color.test(item.stroke) ? item.stroke : "none",
      weight: bounded(item.weight, 0, 0, 64), guide: item.guide === true,
    }];
  });
}

export function shapeAttributes(layer: Layer) {
  const { x, y, w, h, d, rotation, shape } = layer;
  const transform = `translate(${x} ${y}) rotate(${rotation})`;
  if (shape === "path") return { d, transform, fillRule: "evenodd" as const };
  if (shape === "circle") return { r: Math.max(1, w / 2), transform };
  if (shape === "rect") return { width: Math.max(1, w), height: Math.max(1, h), transform };
  return { x2: w, y2: h, transform };
}
