// 笔记's data: a space (its name), pages in a tree (each with an icon, a
// cover, its parent and place), and the pages' blocks: each top-level block
// a record of its own, so two people (or a person and an agent) writing
// different blocks never overwrite each other.
import { Store, type MycoRecord } from "../_lib/myco";
import { Kit } from "../_lib/appkit";

export type Space = { title?: string };
export type Page = {
  title?: string; icon?: string; cover?: string; parent?: string | null; order?: number;
  favorite?: boolean; trashed?: boolean; by?: string; created?: string;
};
export type Block = { page?: string; type?: string; text?: string; checked?: boolean; block?: unknown; order?: number };

export const space = new Store<Space & Record<string, unknown>>("space");
export const pages = new Store<Page & Record<string, unknown>>("pages");
export const blocks = new Store<Block & Record<string, unknown>>("blocks");
export const kit = new Kit([space, pages, blocks], { quiet: true });
export type PageRec = MycoRecord<Page & Record<string, unknown>>;

export const num = (v: unknown, d: number) => (typeof v === "number" && Number.isFinite(v) ? v : d);
export const nowIso = () => new Date().toISOString();
export const titleOf = (p: PageRec | null | undefined) => String(p?.data.title || "").trim() || "无标题";

const byOrder = (a: PageRec, b: PageRec) => num(a.data.order, 0) - num(b.data.order, 0) || String(a.data.created || "").localeCompare(String(b.data.created || ""));
/** A page's children (not in the trash), in their order; null: the top. */
export function childrenOf(parent: string | null) {
  return pages.rows.filter((p) => !p.data.trashed && (p.data.parent || null) === parent && (parent === null || pages.get(parent))).sort(byOrder)
    .concat(parent === null ? orphans() : []);
}
/** Pages whose parent is gone (deleted for good): shown at the top. */
function orphans() {
  return pages.rows.filter((p) => !p.data.trashed && p.data.parent && !pages.get(String(p.data.parent))).sort(byOrder);
}
/** The page's ancestors, from the top. */
export function pathOf(id: string) {
  const out: PageRec[] = [];
  const seen = new Set<string>();
  let p = pages.get(id);
  while (p && p.data.parent && !seen.has(p.id)) {
    seen.add(p.id);
    p = pages.get(String(p.data.parent));
    if (p) out.unshift(p);
  }
  return out;
}
/** The page and everything under it. */
export function subtree(id: string): string[] {
  const out = [id];
  for (let i = 0; i < out.length; i++) for (const p of pages.rows) if (p.data.parent === out[i] && !out.includes(p.id)) out.push(p.id);
  return out;
}
/** Is `a` inside `b` (or `b` itself)? Moving b into a would make a loop. */
export const within = (a: string, b: string) => subtree(b).includes(a);

/** The order for a page put at index among its new siblings. */
export function orderAt(siblings: PageRec[], index: number) {
  const prev = siblings[index - 1], next = siblings[index];
  if (prev && next) return (num(prev.data.order, 0) + num(next.data.order, 0)) / 2;
  if (prev) return num(prev.data.order, 0) + 1;
  if (next) return num(next.data.order, 0) - 1;
  return 1;
}

/** Covers: soft gradients, quiet colours. */
export const COVERS: Record<string, string> = {
  stone: "linear-gradient(135deg, #e9e6e1, #cfc9c0)",
  sage: "linear-gradient(135deg, #dfe8e1, #b9cbbd)",
  mist: "linear-gradient(135deg, #e1e7ef, #bac7d8)",
  clay: "linear-gradient(135deg, #f0e2da, #d8b9a8)",
  dusk: "linear-gradient(135deg, #e5e0ec, #c3b8d1)",
  ink: "linear-gradient(135deg, #3d4148, #1f2329)",
};
export const ICONS = [
  "📄", "📝", "📒", "📚", "📌", "📎", "🗂️", "🗒️", "📅", "✅", "💡", "🎯", "🚀", "🧭", "🧪", "🛠️", "⚙️", "🔍",
  "📈", "📊", "💬", "📣", "🧩", "🌱", "🌊", "⭐", "❤️", "🔥", "🏠", "🏢", "✈️", "🍀", "☕", "🎵", "🎨", "📷",
  "🤖", "👋", "🙂", "🤝", "🧠", "💼", "🗺️", "🔒", "🏁", "⏱️", "📦", "🧾",
];
