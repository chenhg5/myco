// The photos themselves stay on this computer (the hub keeps only a small
// picture of each and its recipe): each file is kept in this page's
// IndexedDB under a key made from its size and bytes, so the same photo
// dropped in again, or on another computer, is known by the same key.
// A kept photo is decoded once; the editor works on a smaller copy of it
// (`view`) and exports from the full one.

export type Local = { key: string; file: Blob; name: string; full: ImageBitmap | HTMLImageElement; view: HTMLCanvasElement | ImageBitmap | HTMLImageElement; width: number; height: number };

const local = new Map<string, Local>();
const loading = new Map<string, Promise<Local | null>>();
const listeners = new Set<() => void>();
export const onLocal = (f: () => void) => { listeners.add(f); return () => { listeners.delete(f); }; };
const changed = () => { for (const f of listeners) f(); };
export const localFor = (key: string | undefined) => (key ? local.get(key) || null : null);

// ---- IndexedDB: key → the file ----
let dbp: Promise<IDBDatabase | null> | null = null;
function db() {
  if (!dbp) dbp = new Promise((resolve) => {
    try {
      const r = indexedDB.open("myco-photo", 1);
      r.onupgradeneeded = () => { r.result.createObjectStore("files"); };
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => resolve(null);
    } catch { resolve(null); }
  });
  return dbp;
}
async function idb<T>(mode: IDBTransactionMode, f: (s: IDBObjectStore) => IDBRequest): Promise<T | null> {
  const d = await db();
  if (!d) return null;
  return new Promise((resolve) => {
    try {
      const req = f(d.transaction("files", mode).objectStore("files"));
      req.onsuccess = () => resolve(req.result as T);
      req.onerror = () => resolve(null);
    } catch { resolve(null); }
  });
}

/** A file's key: its size and a hash of its first and last 256 KB. */
export async function keyOf(file: Blob) {
  const n = 256 * 1024;
  const parts = [file.slice(0, n), file.size > n ? file.slice(Math.max(n, file.size - n)) : new Blob()];
  let h1 = 0x811c9dc5, h2 = 0x01000193 ^ file.size;
  for (const p of parts) {
    const b = new Uint8Array(await p.arrayBuffer());
    for (let i = 0; i < b.length; i++) { h1 = Math.imul(h1 ^ b[i], 16777619); h2 = Math.imul(h2 ^ b[i], 2246822519) ^ (h2 >>> 13); }
  }
  return `${file.size.toString(36)}-${(h1 >>> 0).toString(36)}${(h2 >>> 0).toString(36)}`;
}

/** Decodes an image file (turned the way its camera says). */
export async function decode(file: Blob): Promise<HTMLImageElement> {
  const url = URL.createObjectURL(file);
  try {
    const img = new Image();
    img.src = url;
    await img.decode();
    if (!img.naturalWidth) throw new Error("empty");
    return img;
  } finally {
    // (the decoded image keeps its pixels; the url is not needed after)
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
}

/** The largest side of the copy the editor works on. */
export const VIEW_MAX = 2048;

function shrink(img: CanvasImageSource, w: number, h: number, max: number) {
  const k = Math.min(1, max / Math.max(w, h));
  if (k >= 1) return null;
  const c = document.createElement("canvas");
  c.width = Math.max(1, Math.round(w * k)); c.height = Math.max(1, Math.round(h * k));
  const ctx = c.getContext("2d")!;
  ctx.imageSmoothingQuality = "high";
  ctx.drawImage(img, 0, 0, c.width, c.height);
  return c;
}

async function register(key: string, file: Blob, name: string) {
  const had = local.get(key);
  if (had) return had;
  let p = loading.get(key);
  if (!p) {
    p = (async () => {
      try {
        const img = await decode(file);
        const w = img.naturalWidth, h = img.naturalHeight;
        const l: Local = { key, file, name, full: img, view: shrink(img, w, h, VIEW_MAX) || img, width: w, height: h };
        local.set(key, l);
        changed();
        return l;
      } catch { return null; } finally { loading.delete(key); }
    })();
    loading.set(key, p);
  }
  return p;
}

/** Keeps a photo on this computer: its key and the decoded photo (null when
 *  it is not an image this browser reads). */
export async function keep(file: File | Blob, name = (file as File).name || "图片") {
  const key = await keyOf(file);
  const l = await register(key, file, name);
  if (!l) return null;
  await idb("readwrite", (s) => s.put({ file, name, at: Date.now() }, key));
  return l;
}

/** Finds the photos of these keys that this computer kept. */
export async function restore(keys: string[]) {
  await Promise.all(keys.map(async (k) => {
    if (!k || local.has(k)) return;
    const v = await idb<{ file: Blob; name: string }>("readonly", (s) => s.get(k));
    if (v?.file) await register(k, v.file, v.name);
  }));
}

export async function forget(key: string) {
  local.delete(key);
  changed();
  await idb("readwrite", (s) => s.delete(key));
}

/** A small picture (a JPEG data URL) for the hub: what others see. */
export function posterOf(src: CanvasImageSource, w: number, h: number, max = 360) {
  const k = Math.min(1, max / Math.max(w, h));
  const c = document.createElement("canvas");
  c.width = Math.max(1, Math.round(w * k)); c.height = Math.max(1, Math.round(h * k));
  const ctx = c.getContext("2d")!;
  ctx.imageSmoothingQuality = "high";
  ctx.drawImage(src, 0, 0, c.width, c.height);
  try { return c.toDataURL("image/jpeg", 0.72); } catch { return ""; }
}

export const IMAGE_TYPES = "image/jpeg,image/png,image/webp,image/gif,image/bmp,image/avif,image/heic,image/heif,.jpg,.jpeg,.png,.webp,.gif,.bmp,.avif,.heic,.heif";
export const isImage = (f: File) => f.type.startsWith("image/") || /\.(jpe?g|png|webp|gif|bmp|avif|heic|heif)$/i.test(f.name);
