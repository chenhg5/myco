// The layer inspector edits one selected shape with explicit save.
import { useState } from "react";
import { Circle, Layers3, Minus, PenTool, Plus, Square, Trash2 } from "lucide-react";
import { uid } from "../_lib/appkit";
import { t as uiText } from "../_lib/i18n";
import { saveLayers } from "./edit";
import { maxLayers, readLayers, type Layer, type Shape } from "./model";

const icons = { circle: Circle, rect: Square, line: Minus, path: PenTool };

function shapeNames() {
  return { circle: uiText("圆"), rect: uiText("矩形"), line: uiText("线段"), path: uiText("路径") };
}

function LayerForm({ layer, apply, remove, busy }: {
  layer: Layer; apply: (layer: Layer) => Promise<void>; remove: () => Promise<void>; busy: boolean;
}) {
  const [draft, setDraft] = useState(layer);
  const [invalid, setInvalid] = useState(false);
  const set = (key: keyof Layer, value: string | number | boolean) => setDraft((old) => ({ ...old, [key]: value }));
  return <form className="logo-properties" onSubmit={(event) => {
    event.preventDefault();
    const result = readLayers([draft])[0];
    setInvalid(!result);
    if (result) void apply(result);
  }}>
    <label>{uiText("图层名称")}<input className="inp" value={draft.name} maxLength={60} onChange={(event) => set("name", event.target.value)}/></label>
    <div className="logo-fields">
      <label>X<input className="inp" type="number" value={draft.x} min="-512" max="512" step="any" onChange={(event) => set("x", Number(event.target.value))}/></label>
      <label>Y<input className="inp" type="number" value={draft.y} min="-512" max="512" step="any" onChange={(event) => set("y", Number(event.target.value))}/></label>
      {draft.shape !== "path" && <label>{draft.shape === "circle" ? uiText("直径") : draft.shape === "line" ? "ΔX" : uiText("宽度")}
        <input className="inp" type="number" value={draft.w} min={draft.shape === "line" ? -512 : 1} max="512" step="any" onChange={(event) => set("w", Number(event.target.value))}/></label>}
      {(draft.shape === "rect" || draft.shape === "line") && <label>{draft.shape === "line" ? "ΔY" : uiText("高度")}
        <input className="inp" type="number" value={draft.h} min={draft.shape === "line" ? -512 : 1} max="512" step="any" onChange={(event) => set("h", Number(event.target.value))}/></label>}
      <label>{uiText("旋转（°）")}<input className="inp" type="number" value={draft.rotation} min="-360" max="360" step="any" onChange={(event) => set("rotation", Number(event.target.value))}/></label>
      <label>{uiText("描边宽度")}<input className="inp" type="number" value={draft.weight} min="0" max="64" step="any" onChange={(event) => set("weight", Number(event.target.value))}/></label>
      {(["fill", "stroke"] as const).map((key) => <label key={key}>{key === "fill" ? uiText("填充") : uiText("描边")}
        <input className="inp" value={draft[key]} required pattern="none|#[a-fA-F0-9]{6}" maxLength={7} onChange={(event) => set(key, event.target.value)}/></label>)}
    </div>
    {draft.shape === "path" && <label>{uiText("路径数据")}<textarea className="inp" rows={3} required maxLength={4000} value={draft.d} onChange={(event) => set("d", event.target.value)}/></label>}
    <label className="logo-guide-checkbox"><input type="checkbox" checked={draft.guide} onChange={(event) => set("guide", event.target.checked)}/>{uiText("仅用作辅助线")}</label>
    {invalid && <p role="alert">{uiText("请填写有效的 SVG 路径。")}</p>}
    <div className="logo-row">
      <button className="btn" disabled={busy}>{uiText("保存图层")}</button>
      <button type="button" className="icon-btn" disabled={busy} onClick={() => void remove()} aria-label={uiText("删除图层")}><Trash2 size={16}/></button>
    </div>
  </form>;
}

export function Inspector({ id, version, layers, selected, onSelect }: {
  id: string; version: number; layers: Layer[]; selected?: string; onSelect: (layer?: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const layer = layers.find((item) => item.id === selected);
  const names = shapeNames();
  async function update(next: Layer[]) {
    setBusy(true);
    try { return await saveLayers(id, version, next); }
    finally { setBusy(false); }
  }
  async function add(shape: Shape) {
    const id = uid("layer-");
    const next: Layer = { id, name: names[shape], shape, x: 128, y: 128, w: 48, h: 48,
      d: "M -24 24 L 0 -24 L 24 24 Z", rotation: 0, fill: "#355E53", stroke: shape === "line" ? "#355E53" : "none",
      weight: 5, guide: false };
    if (await update([...layers, next])) onSelect(id);
  }
  return <section className="logo-inspector">
    <h2><Layers3 size={17}/>{uiText("图层")}</h2>
    <div className="logo-layer-list">
      {layers.map((item) => {
        const Icon = icons[item.shape];
        return <button type="button" key={item.id} aria-pressed={selected === item.id} onClick={() => onSelect(item.id)}>
          <Icon size={15}/><span>{item.name || names[item.shape]}</span>{item.guide && <small>{uiText("辅助线")}</small>}
        </button>;
      })}
    </div>
    <label className="logo-add"><Plus size={15}/><select className="sel" aria-label={uiText("添加图层")} disabled={busy || layers.length >= maxLayers}
      value="" onChange={(event) => void add(event.target.value as Shape)}>
      <option value="" disabled>{uiText("添加图层")}</option>
      {Object.entries(names).map(([key, name]) => <option key={key} value={key}>{name}</option>)}
    </select></label>
    {layer && <LayerForm key={layer.id + ":" + version} layer={layer} busy={busy}
      apply={async (next) => { await update(layers.map((item) => item.id === next.id ? next : item)); }}
      remove={async () => { if (await update(layers.filter((item) => item.id !== layer.id))) onSelect(undefined); }}/>}
  </section>;
}
