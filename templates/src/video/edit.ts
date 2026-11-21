import { t as uiText } from "../_lib/i18n";
// Every change people make goes through here: written to the hub, and kept
// for undo and redo (this person's own changes, in this page).
import type { Store } from "../_lib/myco";
import { uid } from "../_lib/appkit";
import {
  clips, kit, media, notes, overlays, placed, clipAt, orderAt, num, MIN_CLIP, overlayList, project,
  type Clip, type Overlay,
} from "./model";

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
 *  joins quick changes of the same thing (a slider) into one undo. */
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
  p = Object.fromEntries(Object.entries(p).map(([k, v]) => [k, typeof v === "number" ? Math.round(v * 10000) / 10000 : v]));
  if (!cur) return;
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

// ---- the actions of the editor ----

/** Puts a file's whole length on the main track, at index (the end by default). */
export function addClip(mediaId: string, index = clips.rows.length) {
  const m = media.get(mediaId)?.data;
  const id = uid("c");
  const data: Clip = { media: mediaId, in: 0, out: num(m?.duration, 5), speed: 1, volume: 1, muted: false, order: orderAt(index) };
  change([[clips, id, data as Rec]]);
  return id;
}

/** Cuts the clip under time t in two. */
export function split(t: number) {
  const list = placed();
  const c = clipAt(list, t);
  if (!c) return null;
  const at = Math.round((c.in + (t - c.start) * c.speed) * 1000) / 1000;
  if (at - c.in < MIN_CLIP * c.speed || c.out - at < MIN_CLIP * c.speed) return null;
  const rec = clips.get(c.id)!;
  const i = clips.rows.indexOf(rec);
  const next = uid("c");
  const nextOrder = i + 1 < clips.rows.length ? (num(rec.data.order, i) + num(clips.rows[i + 1].data.order, i + 2)) / 2 : num(rec.data.order, i) + 1;
  change([[clips, c.id, { ...rec.data, out: at }], [clips, next, { ...rec.data, in: at, order: nextOrder }]]);
  return next;
}

export function removeClip(id: string) {
  if (clips.get(id)) change([[clips, id, null]]);
}

export function duplicateClip(id: string) {
  const rec = clips.get(id);
  if (!rec) return null;
  const i = clips.rows.indexOf(rec);
  const nid = uid("c");
  change([[clips, nid, { ...rec.data, order: orderAt(i + 1) }]]);
  return nid;
}

const r3 = (v: unknown) => (typeof v === "number" ? Math.round(v * 1000) / 1000 : v);
export function addOverlay(o: Overlay) {
  const id = uid("o");
  for (const k of ["start", "end", "x", "y", "w", "h"] as const) o = { ...o, [k]: r3(o[k]) };
  change([[overlays, id, o as Rec]]);
  return id;
}
export const removeOverlay = (id: string) => { if (overlays.get(id)) change([[overlays, id, null]]); };

export function addNote(time: number, text: string, by: string) {
  const id = uid("n");
  change([[notes, id, { time, text, done: false, by }]]);
  return id;
}

/** Removes a file from the project, and its clips with it. */
export function removeMedia(id: string) {
  const list: [Store<any>, string, Rec | null][] = clips.rows.filter((r) => r.data.media === id).map((r) => [clips, r.id, null]);
  list.push([media, id, null]);
  change(list);
}

export function setProject(p: Rec) {
  const cur = project.get("main")?.data || {};
  change([[project, "main", { ...cur, ...p }]]);
}

/** A free lane for an overlay over [start, end), from `from` on. */
export function freeLane(start: number, end: number, from: number, except = "") {
  const all = overlayList().filter((o) => o.id !== except);
  for (let lane = from; lane < 12; lane++) {
    if (!all.some((o) => o.lane === lane && o.start < end - 1e-6 && o.end > start + 1e-6)) return lane;
  }
  return from;
}
