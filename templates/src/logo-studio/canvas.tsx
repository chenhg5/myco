// Draw the editable mark and its construction guides from the shared model.
import { useEffect, useState } from "react";
import { Grid2X2, Pause, Play } from "lucide-react";
import { t as uiText } from "../_lib/i18n";
import { shapeAttributes, size, type Layer } from "./model";

export function Mark({ layers, guides = false, selected, onLayer }: {
  layers: Layer[]; guides?: boolean; selected?: string; onLayer?: (id: string) => void;
}) {
  function draw(layer: Layer) {
    const Tag = layer.shape;
    return <Tag key={layer.id} {...shapeAttributes(layer)}
      fill={layer.guide ? "none" : layer.fill} stroke={layer.guide ? "var(--logo-guide)" : layer.stroke}
      strokeWidth={layer.guide ? 1 : layer.weight} strokeDasharray={layer.guide ? "3 4" : undefined}
      strokeLinecap="round" strokeLinejoin="round" data-active={selected === layer.id ? "true" : undefined}
      onClick={onLayer && !layer.guide ? () => onLayer(layer.id) : undefined}/>;
  }
  return <svg className="logo-mark" viewBox={`0 0 ${size} ${size}`} aria-hidden="true">
    {layers.filter((layer) => !layer.guide).map(draw)}
    {guides && <g pointerEvents="none">{layers.filter((layer) => layer.guide).map(draw)}</g>}
  </svg>;
}

export function Canvas({ layers, selected, onLayer }: { layers: Layer[]; selected?: string; onLayer: (id: string) => void }) {
  const [guides, setGuides] = useState(true);
  const [limit, setLimit] = useState<number>();
  const [playing, setPlaying] = useState(false);
  const marks = layers.filter((layer) => !layer.guide);
  const count = Math.min(limit ?? marks.length, marks.length);
  useEffect(() => {
    if (!playing || count >= marks.length) return;
    const timer = window.setTimeout(() => {
      if (count + 1 === marks.length) setPlaying(false);
      setLimit(count + 1);
    }, 480);
    return () => window.clearTimeout(timer);
  }, [count, marks.length, playing]);
  const visible = [...marks.slice(0, count), ...layers.filter((layer) => layer.guide)];
  return <>
    <div className="logo-board">
      <div className="logo-paper" data-guides={guides}>
        <Mark layers={visible} guides={guides} selected={selected} onLayer={onLayer}/>
      </div>
    </div>
    <div className="logo-tools">
      <button type="button" className="btn" aria-pressed={guides} onClick={() => setGuides(!guides)}>
        <Grid2X2 size={15}/>{uiText("构图辅助线")}
      </button>
      <button type="button" className="btn" disabled={!marks.length} onClick={() => {
        if (!playing && count === marks.length) setLimit(0);
        setPlaying(!playing);
      }}>
        {playing ? <Pause size={15}/> : <Play size={15}/>}
        {playing ? uiText("暂停构图") : uiText("播放构图")}
      </button>
      <input type="range" min="0" max={marks.length || 1} value={count} disabled={!marks.length}
        aria-label={uiText("构图进度")} onChange={(event) => { setPlaying(false); setLimit(Number(event.target.value)); }}/>
      <output>{count}/{marks.length}</output>
    </div>
  </>;
}
