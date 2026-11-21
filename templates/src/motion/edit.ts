import { t as uiText } from "../_lib/i18n";
// Every change people make goes through here: written to the hub, and kept
// for undo and redo (this person's own changes, in this page).
import type { Store } from "../_lib/myco";
import { uid } from "../_lib/appkit";
import { assets, kit, num, orderAt, project, scenes } from "./model";
import { TEMPLATES, type SceneTemplate } from "./templates";

type Rec = Record<string, unknown>;
type Op = { store: Store<any>; id: string; before: Rec | null; after: Rec | null };
type Step = { ops: Op[]; key?: string; at: number };

const undos: Step[] = [];
const redos: Step[] = [];
export const canUndo = () => undos.length > 0;
export const canRedo = () => redos.length > 0;

function apply(ops: Op[], dir: "after" | "before") {
  for (const o of ops) {
    const v = o[dir];
    if (v) void kit.track(o.store.put(v, o.id));
    else if (o.store.get(o.id)) void kit.track(o.store.remove(o.id));
  }
  kit.emit();
}

/** Changes records: [store, id, the new data or null to delete]. `key`
 *  joins quick changes of the same thing (typing, a slider) into one undo. */
export function change(list: [Store<any>, string, Rec | null][], key?: string) {
  const ops: Op[] = list.map(([store, id, after]) => ({ store, id, before: store.get(id) ? { ...store.get(id)!.data } : null, after }));
  const last = undos[undos.length - 1];
  if (key && last && last.key === key && Date.now() - last.at < 1500) {
    for (const o of ops) {
      const had = last.ops.find((x) => x.store === o.store && x.id === o.id);
      if (had) had.after = o.after; else last.ops.push(o);
    }
    last.at = Date.now();
  } else undos.push({ ops, key, at: Date.now() });
  if (undos.length > 100) undos.shift();
  redos.length = 0;
  apply(ops, "after");
}
export function patch(store: Store<any>, id: string, p: Rec, key?: string) {
  const cur = store.get(id);
  if (!cur) return;
  p = Object.fromEntries(Object.entries(p).map(([k, v]) => [k, typeof v === "number" ? Math.round(v * 1000) / 1000 : v]));
  change([[store, id, { ...cur.data, ...p }]], key);
}
export function undo() {
  const s = undos.pop();
  if (!s) return;
  redos.push(s);
  apply([...s.ops].reverse(), "before");
  kit.flash(uiText("已撤销"));
}
export function redo() {
  const s = redos.pop();
  if (!s) return;
  undos.push(s);
  apply(s.ops, "after");
  kit.flash(uiText("已重做"));
}

export function setProject(p: Rec, key?: string) {
  const cur = project.get("main")?.data || {};
  change([[project, "main", { ...cur, ...p }]], key);
}

export function sceneData(t: SceneTemplate, order: number): Rec {
  return {
    name: t.name, order, duration: t.duration, transition: t.transition || "fade", transition_time: 0.6,
    template: t.id, html: t.html, css: t.css, script: t.script, fields: { ...(t.fields || {}) },
  };
}

/** Puts a scene made from a template at index (the end by default). */
export function addScene(templateId: string, index = scenes.rows.length) {
  const t = TEMPLATES.find((x) => x.id === templateId) || TEMPLATES[0];
  const id = uid("s");
  change([[scenes, id, sceneData(t, orderAt(index))]]);
  return id;
}
export function removeScene(id: string) { if (scenes.get(id)) change([[scenes, id, null]]); }
export function duplicateScene(id: string) {
  const rec = scenes.get(id);
  if (!rec) return null;
  const i = scenes.rows.indexOf(rec);
  const nid = uid("s");
  change([[scenes, nid, { ...rec.data, fields: { ...(rec.data.fields || {}) }, order: orderAt(i + 1) }]]);
  return nid;
}
/** Moves a scene to index (counted without it). */
export function moveScene(id: string, index: number) {
  const rec = scenes.get(id);
  if (!rec) return;
  const rest = scenes.rows.filter((r) => r.id !== id);
  const before = index > 0 ? num(rest[index - 1]?.data.order, index - 1) : null;
  const after = index < rest.length ? num(rest[index]?.data.order, index + 1) : null;
  const order = before === null && after === null ? 1 : before === null ? (after as number) - 1 : after === null ? before + 1 : (before + after) / 2;
  change([[scenes, id, { ...rec.data, order }]]);
}
export function setField(id: string, key: string, value: string) {
  const rec = scenes.get(id);
  if (!rec) return;
  const fields = { ...((rec.data.fields as Record<string, string>) || {}), [key]: value };
  change([[scenes, id, { ...rec.data, fields }]], `field:${id}:${key}`);
}

/** Keeps a picture in the video (small enough for one record), its id. */
export async function addAsset(file: File) {
  const { data, width, height } = await shrink(file);
  const id = uid("a");
  change([[assets, id, { name: file.name, data, width, height }]]);
  return id;
}

/** A picture as a data URL under ~56 KB: PNG when small and with
 *  transparency (logos), else WebP or JPEG, smaller until it fits. */
export async function shrink(file: Blob, limit = 56_000): Promise<{ data: string; width: number; height: number }> {
  const url = URL.createObjectURL(file);
  try {
    const img = new Image();
    img.src = url;
    await img.decode();
    let w = img.naturalWidth, h = img.naturalHeight;
    if (file.type === "image/svg+xml" && file.size < limit * 0.7) {
      const text = await file.text();
      return { data: "data:image/svg+xml;base64," + btoa(unescape(encodeURIComponent(text))), width: w || 800, height: h || 600 };
    }
    const fit = (max: number) => { const k = Math.min(1, max / Math.max(w, h)); return [Math.max(1, Math.round(w * k)), Math.max(1, Math.round(h * k))]; };
    const c = document.createElement("canvas");
    const g = c.getContext("2d")!;
    let best = "";
    for (const max of [1600, 1280, 1024, 800, 640, 480, 360]) {
      [c.width, c.height] = fit(max);
      g.clearRect(0, 0, c.width, c.height);
      g.drawImage(img, 0, 0, c.width, c.height);
      const tries: [string, number?][] = max <= 800 ? [["image/png"]] : [];
      tries.push(["image/webp", 0.86], ["image/webp", 0.72], ["image/jpeg", 0.8], ["image/jpeg", 0.65]);
      for (const [type, q] of tries) {
        const d = c.toDataURL(type, q);
        if (!d.startsWith(`data:${type}`)) continue;
        if (d.length <= limit) return { data: d, width: c.width, height: c.height };
        if (!best || d.length < best.length) best = d;
      }
    }
    throw new Error(uiText("图片太大，放不进视频里"));
  } finally { URL.revokeObjectURL(url); }
}
