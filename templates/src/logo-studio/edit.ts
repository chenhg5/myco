// Persist briefs before notifying the page publisher. Use version-checked
// updates for geometry so a stale inspector cannot overwrite new agent edits.
import { t as uiText } from "../_lib/i18n";
import { myco, save } from "../_lib/myco";
import { briefs, concepts, kit, maxLayersBytes, shapeAttributes, size, type Brief, type Layer } from "./model";

const bridge = myco as (typeof myco & { send?: (message: string) => Promise<unknown> });
export const canSend = typeof bridge?.send === "function";

export async function saveBrief(data: Brief) {
  return kit.track(briefs.put(data));
}

export function agentMessage(id: string) {
  return uiText("请帮我设计 Logo。页面 {0}，briefs 记录 {1}。请阅读本页 agent.md 和需求，设计原创的几何矢量方案，把方案写入 concepts，并附构图说明与辅助线。", kit.page, id);
}

export async function sendBrief(id: string) {
  await bridge!.send!(agentMessage(id));
}

export async function saveLayers(id: string, version: number, layers: Layer[]) {
  const current = concepts.get(id);
  if (!current || current.version !== version) {
    kit.flash(uiText("别人刚改过，已显示最新的"));
    return false;
  }
  if (new TextEncoder().encode(JSON.stringify(layers)).length > maxLayersBytes) {
    kit.flash(uiText("图层数据过大，请减少路径节点。"));
    return false;
  }
  const saved = await kit.track(myco!.data.put(concepts.collection, { ...current.data, layers }, { id, ifVersion: version }).catch((error) => {
    if (error.code === "conflict" && error.latest) {
      concepts.apply({ seq: 0, collection: concepts.collection, record: id, op: "put", data: error.latest.data, version: error.latest.version });
    }
    throw error;
  }));
  if (!saved) return false;
  concepts.apply({ seq: 0, collection: concepts.collection, record: id, op: "put", data: saved.data, version: saved.version });
  kit.emit();
  return true;
}

export async function downloadLogo(title: string, layers: Layer[]) {
  // Only known numeric attributes, hex paint and path commands reach this document.
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", `0 0 ${size} ${size}`);
  svg.setAttribute("width", String(size));
  svg.setAttribute("height", String(size));
  for (const layer of layers.filter((item) => !item.guide)) {
    const element = document.createElementNS(svg.namespaceURI, layer.shape);
    const { fillRule, ...geometry } = shapeAttributes(layer);
    const attrs = { ...geometry, ...(fillRule ? { "fill-rule": fillRule } : {}),
      fill: layer.fill, stroke: layer.stroke, "stroke-width": layer.weight, "stroke-linecap": "round", "stroke-linejoin": "round" };
    for (const [key, value] of Object.entries(attrs)) element.setAttribute(key, String(value));
    svg.append(element);
  }
  const filename = (title.replace(/[<>:"/\\|?*\u0000-\u001F]/g, "_").trim().slice(0, 80) || "logo") + ".svg";
  const source = new XMLSerializer().serializeToString(svg);
  if (myco?.download) await save(filename, source, "image/svg+xml");
  else {
    const url = URL.createObjectURL(new Blob([source], { type: "image/svg+xml" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
}

export async function createBlank(title: string, request: string) {
  return kit.track(concepts.put({ request, title, rationale: "", layers: [], order: Date.now() }));
}
