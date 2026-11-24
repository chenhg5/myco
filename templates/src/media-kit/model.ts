import { Kit } from "../_lib/appkit";
import { myco, Store } from "../_lib/myco";

export type Status = "draft" | "ready" | "published";
export type Entry = {
  title: string; date: string; version: string; pitch_zh: string; pitch_en: string;
  copy_zh: string; copy_en: string; tags: string[]; status: Status; channels: string;
};
export type Picture = {
  entry: string; file: string; name: string; language: "zh" | "en" | "none";
  theme: "light" | "dark"; width: number; height: number; bytes: number;
  caption: string; preview: string; order: number;
};
export const entries = new Store<Entry>("entries");
export const images = new Store<Picture>("images");
export const kit = new Kit([entries, images], { quiet: true });
export const statuses = ["draft", "ready", "published"] as const;
export const maxImage = 20 * 1024 * 1024;
export const types = { "image/png": "png", "image/jpeg": "jpg", "image/webp": "webp" } as const;
export let canUseOriginals = false;
export async function initialize() {
  const context = await myco!.ready;
  canUseOriginals = Boolean(context.projectFiles && myco?.projectFiles);
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
