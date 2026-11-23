// The shared graph stores one record per node. Each node has one upstream.
export type Kind = "note" | "text" | "image" | "video";
export type CanvasNode = {
  kind: Kind; title: string; prompt: string; model: string; options: string;
  from: string; x: number; y: number; output: string;
};
export type Entry = { id: string; version: number; data: CanvasNode };
export const nodeWidth = 220;
export const nodeHeight = 116;

export function chain(nodes: Entry[], target: string): Entry[] {
  const entries = new Map(nodes.map(node => [node.id, node]));
  const seen = new Set<string>();
  const path: Entry[] = [];
  for (let id = target; id;) {
    if (seen.has(id)) throw new Error("cycle");
    seen.add(id);
    const entry = entries.get(id);
    if (!entry) throw new Error("missing_input");
    path.push(entry);
    id = entry.data.from || "";
  }
  return path.reverse();
}

export function mediaAddress(raw: string) {
  try {
    const parsed = new URL(raw);
    return parsed.protocol === "https:" && !parsed.username && !parsed.password ? parsed.href : "";
  } catch { return ""; }
}

export function cutOutput(value: string) {
  return value.replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, "").slice(0, 9000);
}
