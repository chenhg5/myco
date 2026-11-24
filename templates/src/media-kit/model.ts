import { Kit } from "../_lib/appkit";
import { myco, Store } from "../_lib/myco";

export type Status = "draft" | "ready" | "published";
export type VideoLink = { url: string; label: string };
export type Entry = {
  title: string; date: string; version: string; pitch_zh: string; pitch_en: string;
  copy_zh: string; copy_en: string; tags: string[]; status: Status; channels: string; video_links?: VideoLink[];
};
export type Picture = {
  kind?: "image" | "video";
  entry: string; file: string; name: string; language: "zh" | "en" | "none";
  theme: "light" | "dark"; width: number; height: number; bytes: number;
  caption: string; preview: string; order: number;
};
export const entries = new Store<Entry>("entries");
export const images = new Store<Picture>("images");
export const kit = new Kit([entries, images], { quiet: true });
export const statuses = ["draft", "ready", "published"] as const;
export const maxFile = 20 * 1024 * 1024;
export const imageTypes = { png: "image/png", jpg: "image/jpeg", webp: "image/webp" } as const;
export const mediaTypes = { ...imageTypes, gif: "image/gif", mp4: "video/mp4", webm: "video/webm" } as const;
export const maxLinks = 10;
export const maxLinkLength = 2048;
export let canUseMedia = false;

export function videoURL(value: string) {
  try {
    const url = new URL(value.trim());
    if (["http:", "https:"].includes(url.protocol) && !url.username && !url.password && url.href.length <= maxLinkLength) return url.href;
  } catch { /* Unparseable URL. */ }
  return "";
}

export function videoLinks(value: unknown): VideoLink[] {
  if (!Array.isArray(value)) return [];
  return value.slice(0, maxLinks).flatMap(link => {
    const url = typeof link?.url === "string" ? videoURL(link.url) : "";
    return url ? [{ url, label: typeof link.label === "string" ? link.label.trim().slice(0, 240) : "" }] : [];
  });
}

export function needsMediaBridge(picture?: Picture) {
  return picture?.kind === "video" || /\.gif$/i.test(picture?.file || "");
}
export let canUseOriginals = false;
export async function initialize() {
  const context = await myco!.ready;
  canUseOriginals = Boolean(context.projectFiles && myco?.projectFiles);
  canUseMedia = canUseOriginals && context.projectMedia === true;
}

export function dateToday() {
  const date = new Date();
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
}

export function pictures(entry: string) {
  return images.rows.filter(record => record.data.entry === entry).sort((a, b) =>
    (a.data.order || 0) - (b.data.order || 0) || a.id.localeCompare(b.id));
}

export function thumbnail(value: string | undefined) {
  return typeof value === "string" && value.length <= 18000 &&
    /^data:image\/(?:jpeg|png|webp);base64,[a-z\d+/]+=*$/i.test(value) ? value : "";
}
