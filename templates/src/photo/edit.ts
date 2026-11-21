import { t as uiText } from "../_lib/i18n";
// Every change people make goes through here: written to the hub, and kept
// for undo and redo (this person's own changes, in this page).
import { uid } from "../_lib/appkit";
import { kit, photos, type Layer, type Photo } from "./model";

type Rec = Record<string, unknown>;
type Op = { id: string; before: Rec | null; after: Rec | null };
type Step = { ops: Op[]; key?: string; at: number };

const undos: Step[] = [];
const redos: Step[] = [];
export const canUndo = () => undos.length > 0;
export const canRedo = () => redos.length > 0;

function apply(ops: Op[], dir: "after" | "before") {
  for (const o of ops) {
    const v = o[dir];
    if (v) void kit.track(photos.put(v as Photo & Rec, o.id));
    else if (photos.get(o.id)) void kit.track(photos.remove(o.id));
  }
  kit.emit();
}

/** Changes photos: [id, the new data or null to delete]. `key` joins quick
 *  changes of the same thing (a slider) into one undo. */
export function change(list: [string, Rec | null][], key?: string) {
  const ops: Op[] = list.map(([id, after]) => ({ id, before: photos.get(id) ? { ...photos.get(id)!.data } : null, after }));
  const last = undos[undos.length - 1];
  if (key && last && last.key === key && Date.now() - last.at < 1500) {
    for (const o of ops) {
      const had = last.ops.find((x) => x.id === o.id);
      if (had) had.after = o.after; else last.ops.push(o);
    }
    last.at = Date.now();
  } else undos.push({ ops, key, at: Date.now() });
  if (undos.length > 100) undos.shift();
  redos.length = 0;
  apply(ops, "after");
}

const round = (v: unknown): unknown => {
  if (typeof v === "number") return Math.round(v * 10000) / 10000;
  if (Array.isArray(v)) return v.map(round);
  if (v && typeof v === "object") return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, round(x)]));
  return v;
};

/** Changes some of a photo's fields. */
export function patch(id: string, p: Partial<Photo>, key?: string) {
  const cur = photos.get(id);
  if (!cur) return;
  change([[id, { ...cur.data, ...(round(p) as Rec) }]], key);
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

export function addPhoto(p: Photo) {
  const id = uid("p");
  const last = photos.rows[photos.rows.length - 1];
  change([[id, { ...p, order: (typeof last?.data.order === "number" ? last.data.order : photos.rows.length) + 1 } as Rec]]);
  return id;
}
export const removePhoto = (id: string) => { if (photos.get(id)) change([[id, null]]); };

/** Back to the original (the photo itself is kept). */
export function reset(id: string) {
  const cur = photos.get(id)?.data;
  if (!cur) return;
  const { name, key, width, height, poster, order, by } = cur;
  change([[id, { name, key, width, height, poster, order, by } as Rec]]);
}

// ---- layers ----
export const layersOf = (id: string): Layer[] => (Array.isArray(photos.get(id)?.data.layers) ? photos.get(id)!.data.layers! : []);
export function setLayers(id: string, layers: Layer[], key?: string) { patch(id, { layers }, key); }
export function addLayer(id: string, l: Layer) { setLayers(id, [...layersOf(id), l]); return l.id; }
export function changeLayer(id: string, lid: string, p: Partial<Layer>, key?: string) {
  setLayers(id, layersOf(id).map((l) => (l.id === lid ? ({ ...l, ...p } as Layer) : l)), key);
}
export function removeLayer(id: string, lid: string) { setLayers(id, layersOf(id).filter((l) => l.id !== lid)); }
export function layerToTop(id: string, lid: string) {
  const ls = layersOf(id), l = ls.find((x) => x.id === lid);
  if (l && ls[ls.length - 1] !== l) setLayers(id, [...ls.filter((x) => x !== l), l]);
}
