import { t as uiText } from "../_lib/i18n";
// A deck's data: one record (deck/main: title, theme) and one record a
// slide, in order. Agents write the content (a layout, a title, points,
// cards, a chart, a table); the page lays it out in the deck's theme.
export type Layout = "cover" | "section" | "points" | "two" | "cards" | "chart" | "table" | "quote" | "end";
export type Card = { title?: string; text?: string };
export type Chart = { type?: "bar" | "line" | "pie"; labels?: string[]; series?: { name?: string; values?: (number | null)[] }[] };
export type TableData = { columns?: string[]; rows?: string[][] };
export type SlideData = {
  layout?: Layout; title?: string; subtitle?: string; points?: string[]; body?: string;
  cards?: Card[]; chart?: Chart; table?: TableData; notes?: string; order?: number;
};
export type DeckData = { title?: string; theme?: string };

export const LAYOUTS: [Layout, string][] = [
  ["cover", uiText("封面")], ["section", uiText("章节页")], ["points", uiText("标题和要点")], ["two", uiText("要点和图表")],
  ["cards", uiText("数字卡片")], ["chart", uiText("图表")], ["table", uiText("表格")], ["quote", uiText("引言")], ["end", uiText("结束页")],
];
export const layoutName = (l?: string) => LAYOUTS.find((x) => x[0] === l)?.[1] || "标题和要点";

export type Theme = {
  name: string; bg: string; fg: string; muted: string; accent: string; soft: string; card: string; line: string;
  coverBg: string; coverFg: string; coverMuted: string; chart: string[];
};
export const THEMES: Record<string, Theme> = {
  light: { name: uiText("简约白"), bg: "#ffffff", fg: "#1f2329", muted: "#646a73", accent: "#1f2329", soft: "#f2f3f5", card: "#f5f6f7", line: "#dee0e3",
    coverBg: "#ffffff", coverFg: "#1f2329", coverMuted: "#646a73", chart: ["#1f2329", "#5b6068", "#8f959e", "#bfc3c9", "#dcdfe3"] },
  navy: { name: uiText("深蓝"), bg: "#ffffff", fg: "#1b2440", muted: "#5b6785", accent: "#2a4cc7", soft: "#e8edfb", card: "#f3f5fb", line: "#d9deeb",
    coverBg: "#16213e", coverFg: "#ffffff", coverMuted: "#aab4d4", chart: ["#2a4cc7", "#7d96e8", "#22a39f", "#f2a93b", "#d9534f"] },
  warm: { name: uiText("暖色"), bg: "#fffaf3", fg: "#3b2a1a", muted: "#8a7461", accent: "#e8710a", soft: "#fdebd3", card: "#fff1df", line: "#f0dcc4",
    coverBg: "#fde7c8", coverFg: "#3b2a1a", coverMuted: "#8a6a4f", chart: ["#e8710a", "#f4a259", "#5b8e7d", "#bc4b51", "#8cb369"] },
  tech: { name: uiText("科技"), bg: "#0f172a", fg: "#e2e8f0", muted: "#94a3b8", accent: "#2dd4bf", soft: "#134e4a", card: "#1e293b", line: "#334155",
    coverBg: "#020617", coverFg: "#f8fafc", coverMuted: "#94a3b8", chart: ["#2dd4bf", "#60a5fa", "#a78bfa", "#fbbf24", "#f472b6"] },
};
export const themeOf = (t?: string) => THEMES[t || ""] || THEMES.light;
export const dark = (l?: Layout) => l === "cover" || l === "section" || l === "end";

/** What a new slide of a layout starts with. */
export function starter(layout: Layout): SlideData {
  switch (layout) {
    case "cover": return { layout, title: "标题", subtitle: "副标题" };
    case "section": return { layout, title: "第一部分", subtitle: "" };
    case "two": return { layout, title: "标题", points: ["要点一", "要点二"], chart: { type: "bar", labels: ["一月", "二月", "三月"], series: [{ name: "数值", values: [30, 45, 60] }] } };
    case "cards": return { layout, title: "标题", cards: [{ title: "42%", text: "说明" }, { title: "1.2 秒", text: "说明" }, { title: "18k", text: "说明" }] };
    case "chart": return { layout, title: "标题", chart: { type: "bar", labels: ["Q1", "Q2", "Q3", "Q4"], series: [{ name: "数值", values: [20, 35, 50, 68] }] } };
    case "table": return { layout, title: "标题", table: { columns: ["项目", "负责人", "状态"], rows: [["", "", ""], ["", "", ""]] } };
    case "quote": return { layout, body: "一句值得记住的话。", subtitle: "—— 出处" };
    case "end": return { layout, title: "谢谢", subtitle: "" };
    default: return { layout: "points", title: "标题", points: ["要点一", "要点二", "要点三"] };
  }
}
