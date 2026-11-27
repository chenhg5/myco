import { localPreferences, savePreferenceImage } from "@/lib/localPreferences";
import { useEffect, useState } from "react";
import { L } from "@/lib/i18n";

/*
 * 聊天背景: a background per conversation, drawn in that conversation's own
 * pane (cover: filled, centred, its edges cut), so panes split side by side
 * or one above the other each crop their own picture; one default for the
 * conversations without their own. Kept on this computer only: the choice
 * in localStorage, a picture in IndexedDB (never sent to a workspace).
 */

/** A background: one of the presets, or a picture you chose; `veil` lays
 *  the theme's own colour over it (0..0.9) so the words stay readable. */
export type ChatBg = { preset?: string; image?: string; veil?: number };
type Kept = { def?: ChatBg; by: Record<string, ChatBg> };

const KEY = "myco-chat-bg";
const EVENT = "myco-chat-bg-changed";

/** The presets: quiet, from the theme's own colours (light and dark alike). */
export const PRESETS: { id: string; name: () => string; css: string }[] = [
  { id: "mist", name: () => L("晨雾", "Mist"), css: "linear-gradient(160deg, color-mix(in srgb, #6b9be0 16%, var(--bg)) 0%, color-mix(in srgb, #e0a96b 13%, var(--bg)) 100%)" },
  { id: "mint", name: () => L("薄荷", "Mint"), css: "linear-gradient(200deg, color-mix(in srgb, #3fae8a 15%, var(--bg)) 0%, color-mix(in srgb, #6bb7e0 12%, var(--bg)) 100%)" },
  { id: "lilac", name: () => L("丁香", "Lilac"), css: "linear-gradient(150deg, color-mix(in srgb, #a07be0 14%, var(--bg)) 0%, color-mix(in srgb, #e07ba9 11%, var(--bg)) 100%)" },
  { id: "sand", name: () => L("沙丘", "Sand"), css: "linear-gradient(170deg, color-mix(in srgb, #d9b98a 18%, var(--bg)) 0%, color-mix(in srgb, #c9a27a 10%, var(--bg)) 100%)" },
  { id: "dots", name: () => L("点阵", "Dots"), css: "radial-gradient(color-mix(in srgb, var(--fg) 13%, transparent) 1px, transparent 1.4px) 0 0 / 18px 18px, var(--bg)" },
  { id: "grid", name: () => L("方格", "Grid"), css: "linear-gradient(color-mix(in srgb, var(--fg) 6%, transparent) 1px, transparent 1px) 0 0 / 24px 24px, linear-gradient(90deg, color-mix(in srgb, var(--fg) 6%, transparent) 1px, transparent 1px) 0 0 / 24px 24px, var(--bg)" },
];

function read(): Kept {
  try {
    const v = JSON.parse(localPreferences.getItem(KEY) || "null") as Kept | null;
    return v && typeof v === "object" && v.by && typeof v.by === "object" ? v : { by: {} };
  } catch {
    return { by: {} };
  }
}
function write(k: Kept) {
  try { localPreferences.setItem(KEY, JSON.stringify(k)); } catch { /* not kept */ }
  window.dispatchEvent(new CustomEvent(EVENT));
}

export const chatKey = (hubId: string, agent: string) => `${hubId}\u0000${agent}`;

/** This conversation's own background (undefined: it follows the default). */
export function ownBg(key: string): ChatBg | undefined {
  return read().by[key];
}
export function defaultBg(): ChatBg | undefined {
  return read().def;
}
/** Sets (or, with undefined, clears) a conversation's own background, or
 *  the default (key ""). */
export function setBg(key: string, bg: ChatBg | undefined) {
  const k = read();
  if (key) {
    if (bg) k.by[key] = bg;
    else delete k.by[key];
  } else {
    k.def = bg;
  }
  write(k);
  void sweepImages(k);
}
const isEmpty = (bg?: ChatBg) => !bg || (!bg.preset && !bg.image);

// ---------------------------------------------------------------------------
// Pictures, in IndexedDB (a few MB each would not fit in localStorage).

const DB = "myco-chat-bg";
const STORE = "images";
function db(): Promise<IDBDatabase> {
  return new Promise((ok, fail) => {
    const r = indexedDB.open(DB, 1);
    r.onupgradeneeded = () => r.result.createObjectStore(STORE);
    r.onsuccess = () => ok(r.result);
    r.onerror = () => fail(r.error);
  });
}
async function idb<T>(mode: IDBTransactionMode, f: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const d = await db();
  return new Promise((ok, fail) => {
    const req = f(d.transaction(STORE, mode).objectStore(STORE));
    req.onsuccess = () => ok(req.result);
    req.onerror = () => fail(req.error);
  });
}
const urls = new Map<string, string>();
async function imageUrl(id: string): Promise<string | null> {
  if (/^\/app\/preferences\/images\/[a-f0-9]{64}\.(png|jpg|gif|webp)$/.test(id)) return id;
  const have = urls.get(id);
  if (have) return have;
  try {
    const blob = await idb<Blob | undefined>("readonly", (s) => s.get(id) as IDBRequest<Blob | undefined>);
    if (!blob) return null;
    const url = URL.createObjectURL(blob);
    urls.set(id, url);
    return url;
  } catch {
    return null;
  }
}
/** Keeps a picture you chose (made no larger than the screen needs): its id. */
export async function keepImage(file: File): Promise<string> {
  let blob: Blob = file;
  try {
    const bmp = await createImageBitmap(file);
    const max = 2560;
    const k = Math.min(1, max / Math.max(bmp.width, bmp.height));
    if (k < 1 || file.size > 1.5e6) {
      const c = document.createElement("canvas");
      c.width = Math.round(bmp.width * k);
      c.height = Math.round(bmp.height * k);
      c.getContext("2d")!.drawImage(bmp, 0, 0, c.width, c.height);
      blob = await new Promise<Blob>((ok, fail) => c.toBlob((b) => (b ? ok(b) : fail(new Error("toBlob"))), "image/jpeg", 0.88));
    }
    bmp.close();
  } catch { /* kept as it is */ }
  const saved = await savePreferenceImage(blob);
  if (saved) return saved;
  const id = `img-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  await idb("readwrite", (s) => s.put(blob, id));
  return id;
}
/** Pictures no conversation uses any more go. */
async function sweepImages(k: Kept) {
  const used = new Set([k.def?.image, ...Object.values(k.by).map((b) => b.image)].filter(Boolean) as string[]);
  try {
    const keys = await idb<IDBValidKey[]>("readonly", (s) => s.getAllKeys());
    for (const id of keys) {
      if (typeof id === "string" && !used.has(id)) {
        await idb("readwrite", (s) => s.delete(id));
        const u = urls.get(id);
        if (u) { URL.revokeObjectURL(u); urls.delete(id); }
      }
    }
  } catch { /* left for next time */ }
}

/** What a conversation shows: its own background, else the default; as a
 *  CSS background and its veil. Null: none. */
export type ShownBg = { css: string; veil: number; image: boolean } | null;
export function useChatBg(key: string): ShownBg {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const on = () => setTick((n) => n + 1);
    window.addEventListener(EVENT, on);
    window.addEventListener("storage", on);
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener("storage", on); };
  }, []);
  const k = read();
  const bg = !key ? undefined : k.by[key] ?? k.def;
  const [img, setImg] = useState<{ id: string; url: string | null } | null>(null);
  useEffect(() => {
    if (!bg?.image) { setImg(null); return; }
    let off = false;
    const id = bg.image;
    void imageUrl(id).then((url) => { if (!off) setImg({ id, url }); });
    return () => { off = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bg?.image, tick]);
  if (isEmpty(bg)) return null;
  if (bg!.image) {
    if (!img || img.id !== bg!.image || !img.url) return null;
    return { css: `url("${img.url}") center / cover no-repeat`, veil: bg!.veil ?? 0.55, image: true };
  }
  const p = PRESETS.find((x) => x.id === bg!.preset);
  return p ? { css: p.css, veil: bg!.veil ?? 0, image: false } : null;
}

/** A picture's address, for a preview tile. */
export function useImageUrl(id?: string): string | null {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    if (!id) { setUrl(null); return; }
    let off = false;
    void imageUrl(id).then((u) => { if (!off) setUrl(u); });
    return () => { off = true; };
  }, [id]);
  return url;
}
