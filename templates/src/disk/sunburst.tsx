import { t as uiText } from "../_lib/i18n";
// The map of a folder: rings around its name, one ring a level down, each
// thing as wide as the room it takes (as DaisyDisk draws a disk). Colours
// run round the circle; deeper rings are lighter; files are greyer;
// smaller items folded together are grey.
import { useMemo } from "react";
import type { OSScanNode } from "../_lib/myco";


export type Seg = {
  key: string; node: OSScanNode | null; parent: OSScanNode; depth: number; a0: number; a1: number;
  /** The smaller items of `parent`, folded together. */
  more?: boolean;
};

const TAU = Math.PI * 2;
// (narrower than this is not drawn: about a third of a degree)
const MIN_ANGLE = 0.006;

export function layout(root: OSScanNode, levels: number): Seg[] {
  const out: Seg[] = [];
  const walk = (n: OSScanNode, a0: number, a1: number, depth: number) => {
    if (depth > levels || !n.disk) return;
    const span = a1 - a0;
    let a = a0;
    for (const k of n.children || []) {
      const w = (k.disk / n.disk) * span;
      if (w < MIN_ANGLE) { a += w; continue; }
      out.push({ key: k.path, node: k, parent: n, depth, a0: a, a1: a + w });
      if (k.dir) walk(k, a, a + w, depth + 1);
      a += w;
    }
    const more = n.more_disk || 0;
    if (more > 0) {
      const w = (more / n.disk) * span;
      if (w >= MIN_ANGLE) out.push({ key: `${n.path}\u0000more`, node: null, parent: n, depth, a0: a, a1: a + w, more: true });
    }
  };
  walk(root, 0, TAU, 1);
  return out;
}

export function colorOf(s: Seg, dark: boolean): string {
  if (s.more) return dark ? "hsl(220 6% 34%)" : "hsl(220 8% 80%)";
  const mid = (s.a0 + s.a1) / 2;
  const hue = (205 + (mid / TAU) * 330) % 360;
  const file = s.node && !s.node.dir;
  // (calmer in the dark, where colour glows)
  const sat = Math.max(14, (file ? (dark ? 18 : 26) : dark ? 40 : 56) - (s.depth - 1) * (dark ? 5 : 7));
  const light = dark ? 44 + (s.depth - 1) * 5 : 62 + (s.depth - 1) * 5;
  return `hsl(${hue.toFixed(0)} ${sat}% ${Math.min(light, dark ? 64 : 84)}%)`;
}

function pt(r: number, a: number, c: number) {
  return `${(c + r * Math.sin(a)).toFixed(2)},${(c - r * Math.cos(a)).toFixed(2)}`;
}
function arc(r0: number, r1: number, a0: number, a1: number, c: number) {
  if (a1 - a0 >= TAU - 1e-4) a1 = a0 + TAU - 1e-4;
  const large = a1 - a0 > Math.PI ? 1 : 0;
  return `M${pt(r1, a0, c)}A${r1},${r1} 0 ${large} 1 ${pt(r1, a1, c)}L${pt(r0, a1, c)}A${r0},${r0} 0 ${large} 0 ${pt(r0, a0, c)}Z`;
}

const SIZE = 600, C = SIZE / 2, R = 292, R0 = 96;

export function Sunburst({ node, levels, dark, hover, onHover, onOpen, onUp, canUp, busy, formatSize }: {
  formatSize: (bytes: number) => string; node: OSScanNode; levels: number; dark: boolean; hover: string | null; busy: boolean; canUp: boolean;
  onHover: (key: string | null, s: Seg | null) => void; onOpen: (s: Seg) => void; onUp: () => void;
}) {
  const segs = useMemo(() => layout(node, levels), [node, node.disk, node.children, levels]);
  const deepest = Math.max(1, ...segs.map((s) => s.depth));
  const ring = (R - R0) / Math.max(levels, deepest);
  const shown = segs.find((s) => s.key === hover);
  // the hovered item, else its own folder, is what the middle says
  const label = shown ? (shown.more ? { name: `${shown.parent.more} 个更小的项目`, disk: shown.parent.more_disk || 0, files: shown.parent.more_files || 0 } : { name: shown.node!.name, disk: shown.node!.disk, files: shown.node!.files })
    : { name: node.name || node.path, disk: node.disk, files: node.files };
  return (
    <svg className={`sb${busy ? " is-busy" : ""}`} viewBox={`0 0 ${SIZE} ${SIZE}`} role="img" aria-label={uiText("{0} 的占用图", node.name)}
      onMouseLeave={() => onHover(null, null)}>
      <g>
        {/* (the one pointed at, with what is in it, drawn last: it comes out over the others) */}
        {order(segs, shown).map((s) => {
          const r0 = R0 + (s.depth - 1) * ring + 1, r1 = R0 + s.depth * ring;
          const on = hover === s.key;
          const inside = !!shown && (on || (!!shown.node && s.key.startsWith(shown.key + "/")));
          const dim = hover !== null && !inside;
          // what is pointed at comes out a little, along its own direction (as DaisyDisk does)
          const out = inside && shown ? pop(shown) : null;
          return (
            <path key={s.key} d={arc(r0, r1, s.a0, s.a1, C)} fill={colorOf(s, dark)}
              className={`sb-seg${on ? " is-on" : ""}${inside ? " is-out" : ""}${dim ? " is-dim" : ""}${s.node?.dir ? " is-dir" : ""}`}
              style={out ? { transform: `translate(${out.x}px, ${out.y}px)` } : undefined}
              onMouseEnter={() => onHover(s.key, s)} onClick={() => onOpen(s)} />
          );
        })}
      </g>
      <circle cx={C} cy={C} r={R0 - 6} className={`sb-mid${canUp ? " can-up" : ""}`} onClick={onUp} onMouseEnter={() => onHover(null, null)} />
      <text x={C} y={C - 16} className="sb-name">{clip(label.name, 18)}</text>
      <text x={C} y={C + 16} className="sb-size">{formatSize(label.disk)}</text>
      <text x={C} y={C + 40} className="sb-hint">{shown ? (shown.node?.dir ? uiText("点开看里面") : shown.more ? uiText("太小，没有分开画") : `${label.files ? uiText("文件") : ""}`) : canUp ? uiText("点中间回上一层") : uiText("{0} 个文件", label.files.toLocaleString())}</text>
    </svg>
  );
}

// How far a segment comes out when pointed at (in the map's units: it is 600 across).
const POP = 9;
function pop(s: Seg) {
  const mid = (s.a0 + s.a1) / 2;
  return { x: +(POP * Math.sin(mid)).toFixed(2), y: +(-POP * Math.cos(mid)).toFixed(2) };
}
function order(segs: Seg[], shown: Seg | undefined) {
  if (!shown) return segs;
  const out = (s: Seg) => s.key === shown.key || (!!shown.node && s.key.startsWith(shown.key + "/"));
  return [...segs.filter((s) => !out(s)), ...segs.filter(out)];
}

function clip(s: string, n: number) {
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
}
