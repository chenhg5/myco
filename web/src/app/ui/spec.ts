import catalog from "@/lib/ui-catalog.json";

/*
 * myco-ui blocks: structured parts of a message (a ```myco-ui fence holding
 * JSON), drawn with the components of the shared catalog
 * (internal/uispec/catalog.json, copied to web/src/lib/ui-catalog.json; a Go
 * test keeps the two the same). Agents check theirs with `myco ui check`;
 * here the App only takes the block apart, leniently: a part it does not
 * know shows as a small note, a block that is not JSON at all shows its
 * source.
 */

export type UiElement = { type: string; props: Record<string, unknown>; children: string[] };
export type UiSpec = { root: string; elements: Record<string, UiElement> };
export type UiCatalog = typeof catalog;

export const UI_FENCE = catalog.fence;
export const uiCatalog = catalog;
const LIMITS = catalog.limits;

const isObj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);

/** The block's JSON as a flat spec (the nested form, or an array of
 *  elements, made flat), or why it cannot be drawn. */
export function parseUi(src: string): { spec: UiSpec } | { error: string } {
  if (src.length > LIMITS.max_bytes) return { error: `too large (${src.length} bytes, at most ${LIMITS.max_bytes})` };
  let v: unknown;
  try { v = JSON.parse(src); } catch (e) { return { error: `not JSON: ${e instanceof Error ? e.message : String(e)}` }; }
  if (isObj(v) && v.version !== undefined && v.version !== catalog.version) return { error: `version ${String(v.version)} (this App draws version ${catalog.version})` };
  // The flat form (json-render's): root and elements by id.
  if (isObj(v) && typeof v.root === "string" && isObj(v.elements)) {
    const elements: Record<string, UiElement> = {};
    for (const [id, e] of Object.entries(v.elements)) {
      if (!isObj(e) || typeof e.type !== "string") return { error: `element ${id} has no type` };
      elements[id] = { type: e.type, props: isObj(e.props) ? e.props : {}, children: Array.isArray(e.children) ? e.children.filter((c): c is string => typeof c === "string") : [] };
    }
    if (!elements[v.root]) return { error: `root ${v.root} is not among the elements` };
    if (Object.keys(elements).length > LIMITS.max_elements) return { error: `too many elements (at most ${LIMITS.max_elements})` };
    return { spec: { root: v.root, elements } };
  }
  // The nested form: an element with its children inside, or a list of them.
  const elements: Record<string, UiElement> = {};
  let n = 0;
  let tooMany = false;
  const add = (e: unknown, depth: number): string | null => {
    if (!isObj(e) || typeof e.type !== "string") return null;
    if (++n > LIMITS.max_elements || depth > LIMITS.max_depth) { tooMany = true; return null; }
    const id = `e${n}`;
    const el: UiElement = { type: e.type, props: isObj(e.props) ? e.props : {}, children: [] };
    elements[id] = el;
    if (Array.isArray(e.children)) {
      for (const c of e.children) {
        const cid = add(c, depth + 1);
        if (cid) el.children.push(cid);
      }
    }
    return id;
  };
  let root: string | null;
  if (Array.isArray(v)) {
    root = "e0";
    const kids = v.map((c) => add(c, 1)).filter((c): c is string => !!c);
    elements.e0 = { type: "Stack", props: {}, children: kids };
  } else {
    root = add(v, 0);
  }
  if (tooMany) return { error: `too many elements or too deep (at most ${LIMITS.max_elements}, ${LIMITS.max_depth} deep)` };
  if (!root) return { error: "no element: expected {\"type\": …} or a list of them" };
  return { spec: { root, elements } };
}

/** A body as its parts: Markdown, and the ```myco-ui blocks in it (not
 *  those inside another code block). */
export type BodyPart = { kind: "md"; text: string } | { kind: "ui"; src: string };
export function splitUi(body: string): BodyPart[] {
  if (!body.includes(UI_FENCE)) return [{ kind: "md", text: body }];
  const lines = body.split("\n");
  const parts: BodyPart[] = [];
  let md: string[] = [];
  let ui: string[] | null = null;
  let other: string | null = null; // the fence of another code block we are in
  for (const l of lines) {
    if (ui) {
      if (/^\s*```+\s*$/.test(l)) {
        if (md.length) parts.push({ kind: "md", text: md.join("\n") });
        md = [];
        parts.push({ kind: "ui", src: ui.join("\n") });
        ui = null;
      } else ui.push(l);
      continue;
    }
    const f = /^\s*(`{3,}|~{3,})\s*([\w-]*)/.exec(l);
    if (other) {
      if (f && f[1][0] === other[0] && f[1].length >= other.length && !f[2]) other = null;
      md.push(l);
      continue;
    }
    if (f && f[1].startsWith("```") && f[2] === UI_FENCE) { ui = []; continue; }
    if (f) other = f[1];
    md.push(l);
  }
  // An unclosed block (still coming in): left as text.
  if (ui) md.push("```" + UI_FENCE, ...ui);
  if (md.length) parts.push({ kind: "md", text: md.join("\n") });
  return parts.filter((p) => p.kind === "ui" || p.text.trim() !== "");
}

/** Tones the catalog knows (anything else is neutral). */
export function tone(v: unknown): string {
  return typeof v === "string" && catalog.tones.includes(v) ? v : "neutral";
}
