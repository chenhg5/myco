import { t as uiText } from "../_lib/i18n";
// 修图's data: what the hub keeps of each photo (never the photo itself:
// it stays on the computer that opened it), and the recipe that turns the
// original into what is shown and exported. Nothing is ever baked in:
// every change is a number in the recipe, so it can be changed again, or
// undone, at any time, by people or agents alike.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";

export type Adjust = {
  brightness?: number; contrast?: number; saturation?: number; warmth?: number; tint?: number;
  highlights?: number; shadows?: number; sharpen?: number; vignette?: number; fade?: number; grain?: number;
};
export type Beauty = { smooth?: number; whiten?: number };
export type Crop = { x: number; y: number; w: number; h: number };

/** Something drawn over the photo. Places and sizes are fractions of the
 *  photo after turning (before cropping): x, y and `size` of its width. */
export type Layer =
  | { id: string; type: "text"; x: number; y: number; size: number; rot?: number; text: string; color?: string; style?: TextStyle; font?: FontId; bold?: boolean }
  | { id: string; type: "sticker"; x: number; y: number; size: number; rot?: number; emoji: string }
  | { id: string; type: "draw"; color: string; width: number; pen?: PenId; points: number[] }
  | { id: string; type: "mosaic"; width: number; style?: "pixel" | "blur"; points: number[] };
export type TextStyle = "plain" | "stroke" | "shadow" | "box";
export type FontId = "sans" | "serif" | "kai" | "mono";
export type PenId = "pen" | "marker" | "neon";

export type Photo = {
  name?: string; key?: string; width?: number; height?: number; poster?: string; order?: number; by?: string;
  rotate?: number; flip_h?: boolean; flip_v?: boolean; straighten?: number; crop?: Crop | null; ratio?: string;
  adjust?: Adjust; filter?: string; filter_strength?: number; beauty?: Beauty;
  layers?: Layer[]; frame?: string; frame_color?: string;
};

export const photos = new Store<Photo & Record<string, unknown>>("photos");
export const kit = new Kit([photos], { quiet: true });

export const num = (v: unknown, d: number) => (typeof v === "number" && Number.isFinite(v) ? v : d);
export const clamp = (v: number, a: number, b: number) => Math.min(b, Math.max(a, v));

export const ADJUSTS: { id: keyof Adjust; name: string; min: number }[] = [
  { id: "brightness", name: uiText("亮度"), min: -100 },
  { id: "contrast", name: uiText("对比度"), min: -100 },
  { id: "saturation", name: uiText("饱和度"), min: -100 },
  { id: "warmth", name: uiText("色温"), min: -100 },
  { id: "tint", name: uiText("色调"), min: -100 },
  { id: "highlights", name: uiText("高光"), min: -100 },
  { id: "shadows", name: uiText("阴影"), min: -100 },
  { id: "sharpen", name: uiText("锐化"), min: 0 },
  { id: "vignette", name: uiText("暗角"), min: -100 },
  { id: "fade", name: uiText("褪色"), min: 0 },
  { id: "grain", name: uiText("颗粒"), min: 0 },
];

/** A filter: changes to the adjustments, a colour cast (per channel: gain,
 *  then offset in 0..255), and how far towards black and white. All of it
 *  scaled by the filter's strength. */
export type Filter = { id: string; name: string; adjust?: Adjust; r?: [number, number]; g?: [number, number]; b?: [number, number]; mono?: number };
export const FILTERS: Filter[] = [
  { id: "", name: uiText("原图") },
  { id: "fresh", name: uiText("清新"), adjust: { brightness: 12, contrast: -8, saturation: 8, warmth: -10, shadows: 14 }, g: [1.02, 2], b: [1.04, 4] },
  { id: "sunny", name: uiText("暖阳"), adjust: { brightness: 6, contrast: 6, saturation: 10, warmth: 30, highlights: -8 }, r: [1.04, 6], b: [0.94, 0] },
  { id: "film", name: uiText("胶片"), adjust: { contrast: 10, saturation: -18, fade: 22, grain: 18, warmth: 8 }, g: [1, 4], b: [0.96, 10] },
  { id: "japan", name: uiText("日系"), adjust: { brightness: 18, contrast: -18, saturation: -14, shadows: 24, fade: 14, warmth: -6 }, g: [1.01, 3], b: [1.02, 8] },
  { id: "vivid", name: uiText("鲜艳"), adjust: { contrast: 14, saturation: 36, sharpen: 16 } },
  { id: "cool", name: uiText("冷调"), adjust: { warmth: -34, tint: -6, contrast: 6 }, b: [1.05, 6] },
  { id: "retro", name: uiText("复古"), adjust: { contrast: -6, saturation: -24, fade: 26, vignette: 30, warmth: 22 }, r: [1.04, 10], g: [1, 6], b: [0.86, 14] },
  { id: "hk", name: uiText("港风"), adjust: { contrast: 18, saturation: 12, highlights: -14, vignette: 22, warmth: 10, tint: 10 }, r: [1.05, 0], g: [0.98, 0], b: [0.96, 8] },
  { id: "food", name: uiText("美食"), adjust: { brightness: 8, contrast: 10, saturation: 26, warmth: 16, sharpen: 20 } },
  { id: "night", name: uiText("夜景"), adjust: { contrast: 16, shadows: 18, highlights: -18, warmth: -16, saturation: 10 }, b: [1.06, 6] },
  { id: "mono", name: uiText("黑白"), adjust: { contrast: 14 }, mono: 1 },
  { id: "silver", name: uiText("银盐"), adjust: { contrast: 26, grain: 22, vignette: 26, fade: 8 }, mono: 1 },
  { id: "sepia", name: uiText("旧照"), adjust: { contrast: 4, fade: 18, vignette: 20 }, mono: 0.9, r: [1.06, 18], g: [0.98, 8], b: [0.82, 0] },
];
export const filterOf = (id: string | undefined) => FILTERS.find((f) => f.id === (id || "")) || FILTERS[0];

export const RATIOS: { id: string; name: string; r: number }[] = [
  { id: "free", name: uiText("自由"), r: 0 },
  { id: "source", name: uiText("原比例"), r: -1 },
  { id: "1:1", name: "1:1", r: 1 },
  { id: "4:3", name: "4:3", r: 4 / 3 },
  { id: "3:4", name: "3:4", r: 3 / 4 },
  { id: "16:9", name: "16:9", r: 16 / 9 },
  { id: "9:16", name: "9:16", r: 9 / 16 },
  { id: "3:2", name: "3:2", r: 3 / 2 },
];

export const FRAMES: { id: string; name: string }[] = [
  { id: "", name: uiText("无") },
  { id: "white", name: uiText("白边") },
  { id: "thin", name: uiText("细边") },
  { id: "polaroid", name: uiText("拍立得") },
  { id: "round", name: uiText("圆角") },
  { id: "film", name: uiText("胶片") },
];

export const COLORS = ["#ffffff", "#1f2329", "#f54a45", "#ff8a00", "#ffd60a", "#34c759", "#0a84ff", "#5e5ce6", "#ff5fa2", "#a2845e"];
export const FRAME_COLORS = ["#ffffff", "#f4efe6", "#1f2329", "#e8eef5", "#f7e1e3", "#e5efe1"];

export const FONTS: { id: FontId; name: string; css: string }[] = [
  { id: "sans", name: uiText("黑体"), css: `-apple-system, "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif` },
  { id: "serif", name: uiText("宋体"), css: `"Songti SC", "STSong", "SimSun", "Noto Serif CJK SC", serif` },
  { id: "kai", name: uiText("楷体"), css: `"Kaiti SC", "STKaiti", "KaiTi", "AR PL UKai CN", serif` },
  { id: "mono", name: uiText("等宽"), css: `ui-monospace, "SF Mono", Menlo, Consolas, monospace` },
];
export const fontOf = (id: string | undefined) => FONTS.find((f) => f.id === id) || FONTS[0];

export const STICKERS = [
  "😀", "😂", "🥰", "😎", "🤩", "😭", "😡", "🥳", "😴", "🤔", "🙈", "👍",
  "👏", "🙌", "✌️", "💪", "❤️", "💖", "💯", "🔥", "✨", "⭐", "🌟", "💫",
  "🌈", "☀️", "🌙", "☁️", "❄️", "🌸", "🌻", "🍀", "🎉", "🎂", "🎁", "🎈",
  "🐱", "🐶", "🐰", "🐼", "🦊", "🐥", "🍓", "🍰", "☕", "🍜", "📷", "💬",
];

/** The photo's recipe with everything filled in. */
export function recipeOf(p: Photo) {
  const r = ((num(p.rotate, 0) % 360) + 360) % 360;
  return {
    rotate: (Math.round(r / 90) * 90) % 360,
    flipH: !!p.flip_h, flipV: !!p.flip_v,
    straighten: clamp(num(p.straighten, 0), -45, 45),
    crop: validCrop(p.crop),
    adjust: p.adjust || {}, filter: p.filter || "", filterStrength: clamp(num(p.filter_strength, 80), 0, 100),
    beauty: p.beauty || {}, layers: Array.isArray(p.layers) ? p.layers : [],
    frame: p.frame || "", frameColor: p.frame_color || "#ffffff",
  };
}
export type Recipe = ReturnType<typeof recipeOf>;

function validCrop(c: unknown): Crop | null {
  if (!c || typeof c !== "object") return null;
  const o = c as Crop;
  const x = clamp(num(o.x, 0), 0, 1), y = clamp(num(o.y, 0), 0, 1);
  const w = clamp(num(o.w, 1), 0.01, 1 - x), h = clamp(num(o.h, 1), 0.01, 1 - y);
  if (x === 0 && y === 0 && w >= 0.9999 && h >= 0.9999) return null;
  return { x, y, w, h };
}

/** Is anything changed from the original? (for "修过" in the list) */
export function edited(p: Photo) {
  const r = recipeOf(p);
  return !!(r.rotate || r.flipH || r.flipV || r.straighten || r.crop || r.filter || r.layers.length || r.frame
    || Object.values(r.adjust).some((v) => v) || Object.values(r.beauty).some((v) => v));
}
