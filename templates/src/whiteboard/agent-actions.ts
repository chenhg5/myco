// Whiteboard business contract. Only shared item records participate in export.
import { myco } from "../_lib/myco";
import { type PageAction } from "../_lib/register-actions";
import { pngFromSVG } from "../_render/svg";
import { svgDocument } from "./export";
import { type Item } from "./geometry";

export const whiteboardExport: PageAction = {
  name: "page.export", version: 1,
  input_schema: { type: "object", additionalProperties: false, required: ["format"], properties: {
    format: { enum: ["svg", "png"] }, max_width: { type: "integer", minimum: 320, maximum: 2560, description: "PNG only; defaults to 1600. Never enlarges the board." },
  } },
  async handler(input, { signal }) {
    const a = input as { format?: unknown; max_width?: unknown };
    if (!a || typeof a !== "object" || Array.isArray(a) || Object.keys(a).some(k => !["format", "max_width"].includes(k)) ||
      !["svg", "png"].includes(String(a.format)) || (a.max_width !== undefined && (a.format !== "png" || !Number.isSafeInteger(a.max_width) || Number(a.max_width) < 320 || Number(a.max_width) > 2560)))
      throw Object.assign(new Error("Choose svg or png; max_width is an integer from 320 to 2560 for PNG only."), { code: "invalid_argument" });
    const { records } = await myco!.data.list("items");
    if (signal.aborted) throw Object.assign(new Error("Export cancelled"), { code: "cancelled" });
    const rows = records.filter(r => r.collection === "items");
    if (rows.length > 256 || rows.reduce((n, r) => n + String(r.data.text ?? "").length, 0) > 32_000)
      throw Object.assign(new Error("Export is limited to 256 items and 32000 text characters."), { code: "too_large" });
    if (rows.some(r => !r.data || typeof r.data !== "object" || !["sticky", "text", "rect", "ellipse", "arrow", undefined].includes(r.data.type as string)))
      throw Object.assign(new Error("The board contains an invalid item type."), { code: "invalid_argument" });
    const document = svgDocument(rows.map(r => ({ id: r.id, data: r.data as Item })));
    const match = document.match(/^<svg[^>]* width="([\d.]+)" height="([\d.]+)"/)!;
    const width = Math.ceil(Number(match[1])), height = Math.ceil(Number(match[2]));
    if (a.format === "png") return { media_type: "image/png", ...await pngFromSVG(document, width, height, Number(a.max_width ?? 1600), signal) };
    return { media_type: "image/svg+xml", data: new Blob([document], { type: "image/svg+xml" }), width, height };
  },
};
