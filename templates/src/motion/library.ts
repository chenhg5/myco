import { t as uiText } from "../_lib/i18n";
// Templates people save: a whole video (its scenes, look and shot plan) or
// one scene, kept in the page for everyone who opens it, and as a file to
// take to another page. A template's scenes are rows of library_scenes
// (a scene can be most of a record on its own).
import { uid } from "../_lib/appkit";
import { assets, durationOf, library, libraryScenes, num, orderAt, project, sceneRows, scenes, sizeOf, total, type LibraryItem } from "./model";
import { change } from "./edit";

const SCENE_KEYS = ["name", "duration", "transition", "transition_time", "template", "html", "css", "script", "fields"] as const;
const THEME_KEYS = ["bg", "fg", "accent", "accent2", "font"] as const;
type Rec = Record<string, unknown>;

const pick = (d: Rec, keys: readonly string[]) => Object.fromEntries(keys.filter((k) => d[k] !== undefined).map((k) => [k, d[k]]));

export const items = (kind?: "video" | "scene") => library.rows.filter((r) => !kind || (r.data.kind || "video") === kind).sort((a, b) => num(b.data.at, 0) - num(a.data.at, 0));
export const scenesOf = (item: string) => libraryScenes.rows.filter((r) => r.data.item === item).sort((a, b) => num(a.data.order, 0) - num(b.data.order, 0));

/** The pictures a scene shows (asset ids). */
function assetsOf(d: Rec) {
  const out = new Set<string>();
  for (const v of Object.values((d.fields as Record<string, string>) || {})) if (typeof v === "string" && assets.get(v)) out.add(v);
  for (const m of String(d.html || "").matchAll(/data-asset=["']([^"']+)["']/g)) if (assets.get(m[1])) out.add(m[1]);
  return out;
}

/** Saves the video, or one scene of it, as a template: its id. */
export function saveTemplate(name: string, kind: "video" | "scene", thumb: string, by: string, sceneId?: string) {
  const id = uid("t");
  const rows = kind === "scene" ? sceneRows().filter((r) => r.id === sceneId) : sceneRows();
  if (!rows.length) return "";
  const p = project.get("main")?.data || {};
  const item: LibraryItem & Rec = {
    name: name.trim() || uiText("未命名模板"), kind, ratio: sizeOf().ratio,
    theme: pick(p, THEME_KEYS) as Record<string, string>,
    motion: kind === "video" ? String(p.motion || "").slice(0, 30000) : "",
    scenes: rows.length, duration: kind === "video" ? total() : durationOf(rows[0].id),
    thumb, by, at: Date.now(),
  };
  const ops: Parameters<typeof change>[0] = [[library, id, item]];
  rows.forEach((r, i) => ops.push([libraryScenes, uid("ls"), { ...pick(r.data, SCENE_KEYS), duration: durationOf(r.id), item: id, order: i + 1 }]));
  change(ops);
  return id;
}

/** Makes this video from a template: its scenes in place of the ones here,
 *  its look and shot plan (one undo). The first scene's id. */
export function useTemplate(itemId: string) {
  const item = library.get(itemId);
  if (!item) return "";
  const rows = scenesOf(itemId);
  const ops: Parameters<typeof change>[0] = [];
  for (const r of scenes.rows) ops.push([scenes, r.id, null]);
  let first = "";
  rows.forEach((r, i) => {
    const id = uid("s");
    if (!first) first = id;
    ops.push([scenes, id, { ...pick(r.data, SCENE_KEYS), order: i + 1 }]);
  });
  const cur = project.get("main")?.data || {};
  ops.push([project, "main", { ...cur, ...(item.data.theme || {}), ratio: item.data.ratio || cur.ratio, ...(item.data.motion ? { motion: item.data.motion } : {}) }]);
  change(ops);
  return first;
}

/** Puts a saved scene in at index: its id. */
export function addSavedScene(itemId: string, index = scenes.rows.length) {
  const r = scenesOf(itemId)[0];
  if (!r) return "";
  const id = uid("s");
  change([[scenes, id, { ...pick(r.data, SCENE_KEYS), order: orderAt(index) }]]);
  return id;
}

export function removeTemplate(itemId: string) {
  const ops: Parameters<typeof change>[0] = [[library, itemId, null]];
  for (const r of scenesOf(itemId)) ops.push([libraryScenes, r.id, null]);
  change(ops);
}

// ---- as a file ----

export type TemplateFile = { myco: "motion-template"; version: 1; item: Rec; scenes: Rec[]; assets: Record<string, Rec> };

export function templateFile(itemId: string): TemplateFile | null {
  const item = library.get(itemId);
  if (!item) return null;
  const rows = scenesOf(itemId);
  const used: Record<string, Rec> = {};
  for (const r of rows) for (const a of assetsOf(r.data)) used[a] = { ...assets.get(a)!.data };
  return { myco: "motion-template", version: 1, item: { ...item.data }, scenes: rows.map((r) => pick(r.data, SCENE_KEYS)), assets: used };
}

/** Adds a template from a file (its pictures too): its id. */
export async function importTemplate(file: Blob) {
  let t: TemplateFile;
  try { t = JSON.parse(await file.text()); } catch { throw new Error(uiText("这不是动效视频的模板文件")); }
  if (!t || t.myco !== "motion-template" || !Array.isArray(t.scenes) || !t.item) throw new Error(uiText("这不是动效视频的模板文件"));
  const id = uid("t");
  const ops: Parameters<typeof change>[0] = [];
  for (const [aid, a] of Object.entries(t.assets || {})) {
    if (!assets.get(aid) && typeof a?.data === "string" && /^data:image\//.test(a.data) && a.data.length < 60000) ops.push([assets, aid, pick(a, ["name", "data", "width", "height"])]);
  }
  const kind = t.item.kind === "scene" ? "scene" : "video";
  ops.push([library, id, { ...pick(t.item, ["name", "ratio", "theme", "motion", "scenes", "duration", "thumb", "by"]), kind, at: Date.now() }]);
  t.scenes.forEach((s, i) => ops.push([libraryScenes, uid("ls"), { ...pick(s, SCENE_KEYS), item: id, order: i + 1 }]));
  change(ops);
  return id;
}
