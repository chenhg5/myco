import { t as uiText } from "../_lib/i18n";
// 视频剪辑's data: what the hub keeps (the edit, never the video itself)
// and the arithmetic of the timeline. The main track is magnetic: clips
// follow one another by `order`, so a clip's place is the sum of the ones
// before it. Overlays (text, blur, mosaic) and notes sit at timeline
// seconds of their own.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";

export type Project = { title?: string; ratio?: string; background?: string; fps?: number };
export type Media = { name?: string; size?: number; duration?: number; width?: number; height?: number; audio?: boolean; key?: string; poster?: string };
export type Clip = { media?: string; in?: number; out?: number; speed?: number; volume?: number; muted?: boolean; order?: number };
export type OverlayKind = "text" | "blur" | "mosaic" | "zoom";
export type Overlay = {
  type?: OverlayKind; start?: number; end?: number; x?: number; y?: number; w?: number; h?: number;
  strength?: number; text?: string; color?: string; size?: number; style?: string; lane?: number;
};
export type Note = { time?: number; text?: string; done?: boolean; by?: string };

export const project = new Store<Project & Record<string, unknown>>("project");
export const media = new Store<Media & Record<string, unknown>>("media");
export const clips = new Store<Clip & Record<string, unknown>>("clips");
export const overlays = new Store<Overlay & Record<string, unknown>>("overlays");
export const notes = new Store<Note & Record<string, unknown>>("notes");
export const kit = new Kit([project, media, clips, overlays, notes], { quiet: true });

/** What a drag shows before it is saved: changes over the records. */
export const draft = { clips: new Map<string, Partial<Clip>>(), overlays: new Map<string, Partial<Overlay>>(), order: null as string[] | null };

export const SPEEDS = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4];
export const MIN_CLIP = 0.1; // seconds on the timeline
export const FPS = 30;

export const RATIOS: { id: string; name: string; w: number; h: number }[] = [
  { id: "source", name: uiText("原始比例"), w: 0, h: 0 },
  { id: "16:9", name: uiText("16:9 横屏"), w: 1920, h: 1080 },
  { id: "9:16", name: uiText("9:16 竖屏"), w: 1080, h: 1920 },
  { id: "1:1", name: uiText("1:1 方形"), w: 1080, h: 1080 },
  { id: "4:3", name: "4:3", w: 1440, h: 1080 },
  { id: "3:4", name: "3:4", w: 1080, h: 1440 },
];

export const num = (v: unknown, d: number) => (typeof v === "number" && Number.isFinite(v) ? v : d);
export const clamp = (v: number, a: number, b: number) => Math.min(b, Math.max(a, v));

/** One clip of the main track where it is now on the timeline. */
export type Placed = { id: string; media: string; in: number; out: number; speed: number; volume: number; muted: boolean; start: number; end: number };

export function placed(): Placed[] {
  let t = 0;
  const out: Placed[] = [];
  let rows = clips.rows;
  if (draft.order) rows = draft.order.map((id) => clips.get(id)).filter((r): r is NonNullable<typeof r> => !!r);
  for (const r of rows) {
    const d = { ...r.data, ...(draft.clips.get(r.id) || {}) };
    const m = media.get(String(d.media || ""));
    const full = num(m?.data.duration, 0);
    const a = Math.max(0, num(d.in, 0));
    let b = num(d.out, full || a + 5);
    if (full > 0) b = Math.min(b, full);
    if (b <= a) b = a + 0.1;
    const speed = clamp(num(d.speed, 1), 0.1, 16);
    const len = (b - a) / speed;
    out.push({ id: r.id, media: String(d.media || ""), in: a, out: b, speed, volume: clamp(num(d.volume, 1), 0, 2), muted: !!d.muted, start: t, end: t + len });
    t += len;
  }
  return out;
}

export function overlayList() {
  return overlays.rows.map((r) => {
    const d = { ...r.data, ...(draft.overlays.get(r.id) || {}) };
    const type = (d.type === "blur" || d.type === "mosaic" || d.type === "zoom" ? d.type : "text") as OverlayKind;
    const start = Math.max(0, num(d.start, 0));
    const end = Math.max(start + MIN_CLIP, num(d.end, start + 3));
    const defW = type === "text" ? 0.6 : 0.25, defH = type === "text" ? 0.12 : 0.25;
    return {
      id: r.id, type, start, end,
      x: clamp(num(d.x, (1 - defW) / 2), -1, 2), y: clamp(num(d.y, type === "text" ? 0.78 : (1 - defH) / 2), -1, 2),
      w: clamp(num(d.w, defW), 0.01, 3), h: clamp(num(d.h, defH), 0.01, 3),
      strength: clamp(num(d.strength, 0.5), 0, 1), text: String(d.text ?? ""), color: String(d.color || "#ffffff"),
      size: clamp(num(d.size, 0.07), 0.02, 0.4), style: String(d.style || "shadow"), lane: Math.max(0, Math.round(num(d.lane, type === "text" ? 0 : 1))),
    };
  });
}
export type OverlayItem = ReturnType<typeof overlayList>[number];

export function total(list = placed()) {
  const ov = overlayList().reduce((m, o) => Math.max(m, o.end), 0);
  return Math.max(list.length ? list[list.length - 1].end : 0, list.length ? 0 : ov);
}

/** The clip playing at timeline time t (the last one at the very end). */
export function clipAt(list: Placed[], t: number): Placed | null {
  for (const c of list) if (t >= c.start - 1e-6 && t < c.end - 1e-6) return c;
  return null;
}
export const sourceTime = (c: Placed, t: number) => clamp(c.in + (t - c.start) * c.speed, c.in, c.out);

/** The output frame's size: the ratio chosen, or the first clip's own, at
 *  `short` pixels on the short side (never larger than the video itself
 *  for the original ratio); even numbers for the encoder. */
export function frameSize(list = placed(), short = 1080) {
  const even = (v: number) => Math.max(2, Math.round(v / 2) * 2);
  const ratio = String(project.get("main")?.data.ratio || "source");
  const r = RATIOS.find((x) => x.id === ratio);
  if (r && r.w) { const k = short / Math.min(r.w, r.h); return { w: even(r.w * k), h: even(r.h * k) }; }
  const first = list.length ? media.get(list[0].media)?.data : null;
  const w = num(first?.width, 1920), h = num(first?.height, 1080);
  const k = Math.min(1, short / Math.max(1, Math.min(w, h)));
  return { w: even(w * k), h: even(h * k) };
}

/** The short sides an export can have: up to the video's own (4K at most). */
export function sizeChoices(list = placed()) {
  const ratio = String(project.get("main")?.data.ratio || "source");
  let own = 2160;
  if (ratio === "source") {
    const first = list.length ? media.get(list[0].media)?.data : null;
    own = Math.min(num(first?.width, 1920), num(first?.height, 1080));
  }
  const all = [2160, 1440, 1080, 720, 480];
  const out = all.filter((s) => s <= own);
  if (ratio === "source" && own < 2160 && !all.includes(own) && own > 480) out.unshift(own);
  return out.length ? out : [Math.max(2, own)];
}

export const fmt = (s: number, frames = false) => {
  s = Math.max(0, s);
  const m = Math.floor(s / 60), sec = Math.floor(s % 60);
  const base = `${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
  return frames ? `${base}.${String(Math.floor((s % 1) * 100 + 1e-6)).padStart(2, "0")}` : base;
};

/** Orders between two neighbours, for a clip put in between. */
export function orderAt(index: number) {
  const rows = clips.rows;
  const before = index > 0 ? num(rows[index - 1]?.data.order, index - 1) : null;
  const after = index < rows.length ? num(rows[index]?.data.order, index + 1) : null;
  if (before === null && after === null) return 1;
  if (before === null) return (after as number) - 1;
  if (after === null) return before + 1;
  return (before + after) / 2;
}
