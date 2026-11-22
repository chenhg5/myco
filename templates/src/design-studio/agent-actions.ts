// Export one explicitly named shared artifact. HTML is returned as bytes only.
import { myco } from "../_lib/myco";
import { type PageAction } from "../_lib/register-actions";
import { pngFromSVG } from "../_render/svg";
import { documentHTML, normalizeFrame, svg, imageOK, type Artifact } from "./model";

export const designExport: PageAction = {
  name: "page.export", version: 1,
  input_schema: { type: "object", additionalProperties: false, required: ["format", "artifact_id"], properties: {
    format: { enum: ["svg", "png", "html", "json"] }, artifact_id: { type: "string", minLength: 1 },
    frame_id: { type: "string", minLength: 1, description: "Required for SVG/PNG when the artifact has multiple frames." },
    max_width: { type: "integer", minimum: 320, maximum: 2560, description: "PNG only; defaults to 1600." },
  } },
  async handler(input, { signal }) {
    const a = input as { format?: unknown; artifact_id?: unknown; frame_id?: unknown; max_width?: unknown };
    if (!a || typeof a !== "object" || Array.isArray(a) || Object.keys(a).some(k => !["format", "artifact_id", "frame_id", "max_width"].includes(k)) ||
      !["svg", "png", "html", "json"].includes(String(a.format)) || typeof a.artifact_id !== "string" || !a.artifact_id ||
      (a.frame_id !== undefined && (typeof a.frame_id !== "string" || !a.frame_id || !["svg", "png"].includes(String(a.format)))) ||
      (a.max_width !== undefined && (a.format !== "png" || !Number.isSafeInteger(a.max_width) || Number(a.max_width) < 320 || Number(a.max_width) > 2560)))
      throw Object.assign(new Error("Choose a format and artifact_id; frame_id selects an SVG/PNG frame, max_width applies only to PNG."), { code: "invalid_argument" });
    const record = await myco!.data.get("artifacts", a.artifact_id);
    if (signal.aborted) throw Object.assign(new Error("Export cancelled"), { code: "cancelled" });
    const artifact = record.data as unknown as Artifact;
    if (!artifact || !Array.isArray(artifact.frames) || artifact.frames.length > 32 || new Blob([JSON.stringify(artifact)]).size > 3_000_000)
      throw Object.assign(new Error("The artifact must contain at most 32 frames and 3 MB of shared data."), { code: "too_large" });
    if (a.format === "json") return { media_type: "application/json", data: new Blob([JSON.stringify(artifact, null, 2)]) };
    if (artifact.frames.some(f => !f || !Array.isArray(f.nodes) || f.nodes.length > 300 || f.nodes.some(n => !n || !["text", "rect", "ellipse", "line", "image"].includes(n.kind))))
      throw Object.assign(new Error("Each frame needs at most 300 valid canvas nodes."), { code: "invalid_argument" });
    const candidates = a.frame_id ? artifact.frames.filter(f => f.id === a.frame_id) : artifact.frames;
    if (a.format === "html") return { media_type: "text/html", data: new Blob([documentHTML(artifact)]), pages: Math.max(1, artifact.frames.length) };
    if (candidates.length !== 1) throw Object.assign(new Error("Select exactly one existing frame using frame_id."), { code: "invalid_argument" });
    const frame = normalizeFrame(candidates[0]);
    if (frame.nodes.some(n => !n.hidden && n.kind === "image" && !imageOK(n.src)))
      throw Object.assign(new Error("Canvas export only supports shared inline PNG/JPEG/WebP images."), { code: "unsupported" });
    const document = svg(frame), width = Math.ceil(frame.width), height = Math.ceil(frame.height);
    if (a.format === "png") return { media_type: "image/png", ...await pngFromSVG(document, width, height, Number(a.max_width ?? 1600), signal) };
    return { media_type: "image/svg+xml", data: new Blob([document]), width, height };
  },
};
