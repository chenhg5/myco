// Deck exports reuse the editor's PowerPoint/Markdown engines, never local files.
import { myco } from "../_lib/myco";
import { type PageAction } from "../_lib/register-actions";
import { toMarkdown, toPptx } from "./pptx";
import { type DeckData, type SlideData } from "./model";

function validSlide(s: SlideData): boolean {
  const strings = (v: unknown, limit: number) => v === undefined || Array.isArray(v) && v.length <= limit && v.every(x => typeof x === "string");
  return !!s && typeof s === "object" && !Array.isArray(s) &&
    [s.title, s.subtitle, s.body, s.notes].every(v => v === undefined || typeof v === "string") &&
    (s.layout === undefined || ["cover", "section", "points", "two", "cards", "chart", "table", "quote", "end"].includes(s.layout)) &&
    strings(s.points, 100) && (s.cards === undefined || Array.isArray(s.cards) && s.cards.length <= 4 && s.cards.every(c => c && [c.title, c.text].every(v => v === undefined || typeof v === "string"))) &&
    (s.table === undefined || !!s.table && typeof s.table === "object" && strings(s.table.columns, 30) && (s.table.rows === undefined || Array.isArray(s.table.rows) && s.table.rows.length <= 100 && s.table.rows.every(r => strings(r, 30)))) &&
    (s.chart === undefined || !!s.chart && typeof s.chart === "object" && (s.chart.type === undefined || ["bar", "line", "pie"].includes(s.chart.type)) && strings(s.chart.labels, 100) &&
      (s.chart.series === undefined || Array.isArray(s.chart.series) && s.chart.series.length <= 20 && s.chart.series.every(r => r && (r.name === undefined || typeof r.name === "string") && Array.isArray(r.values) && r.values.length <= 100 && r.values.every(n => n === null || typeof n === "number" && Number.isFinite(n)))));
}
export const slidesExport: PageAction = {
  name: "page.export", version: 1,
  input_schema: { type: "object", additionalProperties: false, required: ["format"], properties: {
    format: { enum: ["pptx", "md"] }, slide_ids: { type: "array", minItems: 1, maxItems: 100, uniqueItems: true, items: { type: "string", minLength: 1 }, description: "Optional explicit slide IDs in export order; defaults to all shared slides." },
  } },
  async handler(input, { signal }) {
    const a = input as { format?: unknown; slide_ids?: unknown };
    if (!a || typeof a !== "object" || Array.isArray(a) || Object.keys(a).some(k => !["format", "slide_ids"].includes(k)) || !["pptx", "md"].includes(String(a.format)) ||
      (a.slide_ids !== undefined && (!Array.isArray(a.slide_ids) || !a.slide_ids.length || a.slide_ids.length > 100 || a.slide_ids.some(id => typeof id !== "string" || !id) || new Set(a.slide_ids).size !== a.slide_ids.length)))
      throw Object.assign(new Error("Choose pptx or md and optionally 1–100 unique slide_ids in export order."), { code: "invalid_argument" });
    const { records } = await myco!.data.list();
    if (signal.aborted) throw Object.assign(new Error("Export cancelled"), { code: "cancelled" });
    const deck = records.find(r => r.collection === "deck" && r.id === "main")?.data as DeckData | undefined;
    const rows = records.filter(r => r.collection === "slides").sort((a, b) => Number(a.data.order ?? 0) - Number(b.data.order ?? 0));
    const selected = a.slide_ids === undefined ? rows : (a.slide_ids as string[]).map(id => {
      const row = rows.find(r => r.id === id);
      if (!row) throw Object.assign(new Error("A selected slide no longer exists: " + id), { code: "not_found" });
      return row;
    });
    if (!selected.length || selected.length > 100) throw Object.assign(new Error("Export requires 1–100 slides."), { code: "too_large" });
    const slides = selected.map(r => r.data as SlideData);
    if (!slides.every(validSlide) || (deck?.title !== undefined && typeof deck.title !== "string") || (deck?.theme !== undefined && typeof deck.theme !== "string"))
      throw Object.assign(new Error("Invalid slide content, chart or table; fix the shared records before exporting."), { code: "invalid_argument" });
    if (new Blob([JSON.stringify(slides)]).size > 1_000_000) throw Object.assign(new Error("Slide content exceeds the 1 MB render limit."), { code: "too_large" });
    if (a.format === "md") return { media_type: "text/markdown", data: new Blob([toMarkdown(deck?.title ?? "", slides)]), pages: slides.length };
    return { media_type: "application/vnd.openxmlformats-officedocument.presentationml.presentation", data: await toPptx(deck?.title || "Slides", deck?.theme, slides), pages: slides.length };
  },
};
