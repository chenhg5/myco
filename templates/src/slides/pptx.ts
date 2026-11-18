// The deck as a real PowerPoint file (pptxgenjs): texts, cards, charts and
// tables stay editable in PowerPoint, WPS and Keynote.
import PptxGenJS from "pptxgenjs";
import { dark, themeOf, type SlideData } from "./model";

const FONT = "Microsoft YaHei";
const c = (hex: string) => hex.replace("#", "").toUpperCase();
// The slide is 13.333 × 7.5 inches (1280 × 720 at 96 per inch).
const IN = (px: number) => px / 96;

export async function toPptx(title: string, theme: string | undefined, slides: SlideData[]): Promise<Uint8Array> {
  const t = themeOf(theme);
  const p = new PptxGenJS();
  p.layout = "LAYOUT_WIDE";
  p.title = title;
  slides.forEach((s, i) => {
    const layout = s.layout || "points", inv = dark(layout);
    const fg = c(inv ? t.coverFg : t.fg), muted = c(inv ? t.coverMuted : t.muted);
    const sl = p.addSlide();
    sl.background = { color: c(inv ? t.coverBg : t.bg) };
    const text = (v: string | undefined, x: number, y: number, w: number, h: number, size: number, o: Record<string, unknown> = {}) => {
      if (v) sl.addText(v, { x: IN(x), y: IN(y), w: IN(w), h: IN(h), fontFace: FONT, fontSize: size, color: fg, valign: "top", margin: 0, ...o });
    };
    const head = () => {
      text(s.title, 88, 64, 1104, 70, 30, { bold: true, valign: "middle" });
      if (s.subtitle) text(s.subtitle, 88, 134, 1104, 40, 15, { color: muted });
    };
    const top = s.subtitle ? 196 : 168;
    const points = (x: number, w: number) => {
      const list = (s.points || []).filter(Boolean);
      if (list.length) sl.addText(list.map((v) => ({ text: v, options: { bullet: { indent: 18 }, breakLine: true } })),
        { x: IN(x), y: IN(top), w: IN(w), h: IN(720 - top - 80), fontFace: FONT, fontSize: 20, color: fg, valign: "top", paraSpaceAfter: 10, margin: 0 });
    };
    const chart = (x: number, y: number, w: number, h: number) => {
      const ch = s.chart, labels = ch?.labels || [], series = (ch?.series || []).filter((x) => x && Array.isArray(x.values));
      if (!labels.length || !series.length) return;
      const data = series.map((x, k) => ({ name: x.name || `系列${k + 1}`, labels, values: labels.map((_, j) => Number(x.values?.[j]) || 0) }));
      const type = ch?.type === "pie" ? p.ChartType.pie : ch?.type === "line" ? p.ChartType.line : p.ChartType.bar;
      sl.addChart(type, ch?.type === "pie" ? data.slice(0, 1) : data, {
        x: IN(x), y: IN(y), w: IN(w), h: IN(h), chartColors: (ch?.type !== "pie" && series.length === 1 ? [t.chart[0]] : t.chart).map(c), barGapWidthPct: 60,
        showLegend: ch?.type === "pie" || series.length > 1, legendPos: ch?.type === "pie" ? "r" : "b", legendFontFace: FONT, legendColor: fg,
        catAxisLabelColor: muted, valAxisLabelColor: muted, catAxisLabelFontFace: FONT, valAxisLabelFontFace: FONT,
        valGridLine: { color: c(t.line), size: 1 }, showPercent: ch?.type === "pie", dataLabelColor: "FFFFFF", lineSize: 3, lineDataSymbolSize: 8,
      });
    };
    switch (layout) {
      case "cover":
        sl.addShape(p.ShapeType.rect, { x: IN(120), y: IN(250), w: IN(64), h: IN(8), fill: { color: c(t.accent) }, line: { color: c(t.accent) } });
        text(s.title, 120, 280, 1040, 120, 44, { bold: true });
        text(s.subtitle, 120, 410, 1040, 60, 20, { color: muted });
        break;
      case "section":
        text(String(i + 1).padStart(2, "0"), 120, 230, 400, 70, 40, { color: c(t.accent), bold: true });
        text(s.title, 120, 300, 1040, 100, 40, { bold: true });
        text(s.subtitle, 120, 410, 1040, 60, 18, { color: muted });
        break;
      case "end":
        text(s.title, 0, 280, 1280, 110, 48, { bold: true, align: "center", valign: "middle" });
        text(s.subtitle, 0, 400, 1280, 50, 18, { color: muted, align: "center" });
        break;
      case "quote":
        text(s.body, 160, 220, 960, 220, 30, { bold: true, valign: "middle" });
        text(s.subtitle, 160, 460, 960, 50, 18, { color: muted });
        break;
      case "two": head(); points(88, 520); chart(660, top - 10, 540, 720 - top - 70); break;
      case "cards": {
        head();
        const cards = (s.cards || []).slice(0, 4), n = Math.max(1, cards.length), gap = 24, w = (1104 - gap * (n - 1)) / n;
        cards.forEach((cd, k) => {
          const x = 88 + k * (w + gap);
          sl.addShape(p.ShapeType.roundRect, { x: IN(x), y: IN(top + 20), w: IN(w), h: IN(300), fill: { color: c(t.card) }, line: { color: c(t.card) }, rectRadius: 0.12 });
          text(cd.title, x + 28, top + 56, w - 56, 90, 36, { bold: true, color: c(t.accent) });
          text(cd.text, x + 28, top + 160, w - 56, 140, 16, { color: muted });
        });
        break;
      }
      case "chart": head(); chart(88, top, 1104, 720 - top - 70); break;
      case "table": {
        head();
        const cols = s.table?.columns || [], rows = s.table?.rows || [];
        if (cols.length) {
          const cell = (v: string, h: boolean) => ({ text: v || "", options: { bold: h, color: h ? "FFFFFF" : fg, fill: { color: h ? c(t.accent) : c(t.bg) }, fontFace: FONT, fontSize: 14 } });
          sl.addTable([cols.map((v) => cell(v, true)), ...rows.map((r) => cols.map((_, k) => cell(r[k] || "", false)))],
            { x: IN(88), y: IN(top), w: IN(1104), border: { type: "solid", color: c(t.line), pt: 1 }, rowH: 0.45, margin: 0.08 });
        }
        break;
      }
      default: head(); points(88, 1104);
    }
    if (!inv && layout !== "quote") {
      text(title, 88, 670, 800, 24, 10, { color: muted });
      text(`${i + 1} / ${slides.length}`, 1000, 670, 192, 24, 10, { color: muted, align: "right" });
    }
    if (s.notes) sl.addNotes(s.notes);
  });
  const out = await p.write({ outputType: "uint8array" });
  return out as Uint8Array;
}

/** The deck as a Markdown outline. */
export function toMarkdown(title: string, slides: SlideData[]): string {
  const out = [`# ${title || "幻灯片"}`, ""];
  slides.forEach((s, i) => {
    out.push(`## ${i + 1}. ${s.title || (s.layout === "quote" ? "引言" : "")}`.trimEnd());
    if (s.subtitle) out.push("", s.subtitle);
    if (s.body) out.push("", `> ${s.body.replace(/\n/g, "\n> ")}`);
    if (s.points?.length) out.push("", ...s.points.filter(Boolean).map((p) => `- ${p}`));
    if (s.cards?.length) out.push("", ...s.cards.map((c) => `- **${c.title || ""}** ${c.text || ""}`));
    if (s.table?.columns?.length) {
      const cols = s.table.columns;
      out.push("", `| ${cols.join(" | ")} |`, `| ${cols.map(() => "---").join(" | ")} |`, ...(s.table.rows || []).map((r) => `| ${cols.map((_, k) => r[k] || "").join(" | ")} |`));
    }
    if (s.chart?.labels?.length) out.push("", ...(s.chart.series || []).map((x) => `- ${x.name || "数值"}：${s.chart!.labels!.map((l, k) => `${l} ${x.values?.[k] ?? ""}`).join("，")}`));
    if (s.notes) out.push("", `备注：${s.notes}`);
    out.push("");
  });
  return out.join("\n");
}
