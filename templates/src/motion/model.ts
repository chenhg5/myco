import { t as uiText } from "../_lib/i18n";
// 动效视频's data: the video (size, theme), its scenes one after another
// (each its own HTML, CSS and animation script, written by an agent or
// taken from a template, with the texts and pictures people change in a
// form), and the pictures the scenes show.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";

export type Project = {
  title?: string; ratio?: string; fps?: number;
  bg?: string; fg?: string; accent?: string; accent2?: string; font?: string; motion?: string;
};
export type Scene = {
  name?: string; order?: number; duration?: number; transition?: string; transition_time?: number;
  template?: string; html?: string; css?: string; script?: string; fields?: Record<string, string>;
};
export type Asset = { name?: string; data?: string; width?: number; height?: number };
export type Note = { time?: number; text?: string; done?: boolean; by?: string };

export const project = new Store<Project & Record<string, unknown>>("project");
export const scenes = new Store<Scene & Record<string, unknown>>("scenes");
export const assets = new Store<Asset & Record<string, unknown>>("assets");
export const notes = new Store<Note & Record<string, unknown>>("notes");
export const kit = new Kit([project, scenes, assets, notes], { quiet: true });

export const RATIOS: { id: string; name: string; w: number; h: number }[] = [
  { id: "16:9", name: uiText("16:9 横屏"), w: 1920, h: 1080 },
  { id: "9:16", name: uiText("9:16 竖屏"), w: 1080, h: 1920 },
  { id: "1:1", name: uiText("1:1 方形"), w: 1080, h: 1080 },
  { id: "4:5", name: "4:5", w: 1080, h: 1350 },
];
export const TRANSITIONS: [string, string][] = [
  ["cut", uiText("直接切")], ["fade", uiText("淡入")], ["slide", uiText("推入")], ["up", uiText("上推")],
  ["zoom", uiText("缩放")], ["wipe", uiText("擦除")], ["blur", uiText("模糊")],
];
export const FONTS: { id: string; name: string; css: string }[] = [
  { id: "sans", name: uiText("黑体"), css: "system-ui, -apple-system, 'PingFang SC', 'Microsoft YaHei', 'Noto Sans CJK SC', sans-serif" },
  { id: "serif", name: uiText("宋体"), css: "'Songti SC', 'STSong', 'Noto Serif CJK SC', 'SimSun', Georgia, serif" },
  { id: "round", name: uiText("圆体"), css: "ui-rounded, 'SF Pro Rounded', 'Yuanti SC', 'PingFang SC', 'Microsoft YaHei', sans-serif" },
  { id: "mono", name: uiText("等宽"), css: "ui-monospace, 'SF Mono', Menlo, Consolas, 'PingFang SC', monospace" },
];
/** Colour sets for the theme: background, text, accent, second accent. */
export const PALETTES: { id: string; c: [string, string, string, string] }[] = [
  { id: "night", c: ["#0e0f12", "#f4f4f5", "#7c9cff", "#f5b971"] },
  { id: "paper", c: ["#f6f4ef", "#1d1d1f", "#d9480f", "#2f6f4f"] },
  { id: "ink", c: ["#111111", "#ffffff", "#ffd60a", "#ff5c5c"] },
  { id: "ocean", c: ["#06202b", "#eaf6f6", "#2ec4b6", "#ffbf69"] },
  { id: "plum", c: ["#1b1026", "#f6eefc", "#c77dff", "#7ae7c7"] },
  { id: "snow", c: ["#ffffff", "#16181d", "#3b5bfd", "#ff8a3d"] },
];

export const num = (v: unknown, d: number) => (typeof v === "number" && Number.isFinite(v) ? v : d);
export const clamp = (v: number, a: number, b: number) => Math.min(b, Math.max(a, v));
export const MIN_SCENE = 0.5;

export function sizeOf(ratio = String(project.get("main")?.data.ratio || "16:9")) {
  const r = RATIOS.find((x) => x.id === ratio) || RATIOS[0];
  return { w: r.w, h: r.h, ratio: r.id };
}
export function fpsOf() { return clamp(Math.round(num(project.get("main")?.data.fps, 30)), 12, 60); }

export function theme() {
  const p = project.get("main")?.data || {};
  const font = FONTS.find((f) => f.id === p.font)?.css || (p.font && !FONTS.some((f) => f.id === p.font) ? String(p.font) : FONTS[0].css);
  return { bg: p.bg || PALETTES[0].c[0], fg: p.fg || PALETTES[0].c[1], accent: p.accent || PALETTES[0].c[2], accent2: p.accent2 || PALETTES[0].c[3], font };
}

/** A scene where it is in the video now. */
export type Placed = { id: string; start: number; end: number; dur: number; name: string; index: number };
/** Changes a drag shows before it is saved. */
export const draft = { durations: new Map<string, number>(), order: null as string[] | null };

export function sceneRows() {
  let rows = scenes.rows;
  if (draft.order) rows = draft.order.map((id) => scenes.get(id)).filter((r): r is NonNullable<typeof r> => !!r);
  return rows;
}
export function durationOf(id: string) {
  const d = draft.durations.get(id) ?? num(scenes.get(id)?.data.duration, 3);
  return clamp(d, MIN_SCENE, 120);
}
export function placed(): Placed[] {
  let t = 0;
  return sceneRows().map((r, index) => {
    const dur = durationOf(r.id);
    const p = { id: r.id, start: t, end: t + dur, dur, name: String(r.data.name || ""), index };
    t += dur;
    return p;
  });
}
export const total = (list = placed()) => (list.length ? list[list.length - 1].end : 0);
export function sceneAt(t: number, list = placed()) {
  for (const p of list) if (t >= p.start - 1e-6 && t < p.end - 1e-6) return p;
  return list[list.length - 1] || null;
}

/** The parts of a scene a person changes in the form: elements marked
 *  data-field in its HTML (a text, or a picture for <img>). */
export type Field = { key: string; label: string; kind: "text" | "image"; value: string; multiline: boolean };
const parsed = new Map<string, Field[]>();
export function fieldsOf(html: string): Field[] {
  const had = parsed.get(html);
  if (had) return had;
  const out: Field[] = [];
  try {
    // (an inert document: nothing in it runs or loads)
    const doc = new DOMParser().parseFromString(`<body>${html}</body>`, "text/html");
    const seen = new Set<string>();
    doc.querySelectorAll("[data-field]").forEach((el) => {
      const key = el.getAttribute("data-field") || "";
      if (!key || seen.has(key)) return;
      seen.add(key);
      const image = el.tagName === "IMG";
      const text = image ? "" : textOf(el);
      out.push({ key, label: el.getAttribute("data-label") || key, kind: image ? "image" : "text", value: text, multiline: text.includes("\n") || text.length > 40 || el.hasAttribute("data-multiline") });
    });
  } catch { /* (shown without fields) */ }
  parsed.set(html, out);
  if (parsed.size > 200) parsed.delete(parsed.keys().next().value as string);
  return out;
}
function textOf(el: Element) {
  let s = "";
  el.childNodes.forEach((n) => { s += n.nodeName === "BR" ? "\n" : n.nodeType === 1 ? textOf(n as Element) : n.textContent || ""; });
  return s.replace(/[ \t]*\n[ \t]*/g, "\n").replace(/[ \t]+/g, " ").trim();
}

/** What the scene frame needs to show the video. */
export function frameDoc(mode: "preview" | "capture", only?: string[]) {
  const { w, h } = sizeOf();
  const list = sceneRows().filter((r) => !only || only.includes(r.id));
  const usedAssets: Record<string, string> = {};
  for (const a of assets.rows) if (a.data.data) usedAssets[a.id] = String(a.data.data);
  return {
    w, h, mode, theme: theme(), assets: usedAssets,
    scenes: list.map((r) => ({
      id: r.id, html: String(r.data.html || ""), css: String(r.data.css || ""), script: String(r.data.script || ""),
      fields: (r.data.fields && typeof r.data.fields === "object" ? r.data.fields : {}) as Record<string, string>,
      duration: durationOf(r.id), transition: String(r.data.transition || "cut"), transition_time: num(r.data.transition_time, 0.6),
    })),
  };
}

export const fmt = (s: number, frac = false) => {
  s = Math.max(0, s);
  const m = Math.floor(s / 60), sec = Math.floor(s % 60);
  const base = `${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
  return frac ? `${base}.${String(Math.floor((s % 1) * 10 + 1e-6))}` : base;
};
export const secs = (s: number) => `${Math.round(s * 10) / 10}s`;

/** Orders between two neighbours, for a scene put in at index. */
export function orderAt(index: number) {
  const rows = scenes.rows;
  const before = index > 0 ? num(rows[index - 1]?.data.order, index - 1) : null;
  const after = index < rows.length ? num(rows[index]?.data.order, index + 1) : null;
  if (before === null && after === null) return 1;
  if (before === null) return (after as number) - 1;
  if (after === null) return before + 1;
  return (before + after) / 2;
}
