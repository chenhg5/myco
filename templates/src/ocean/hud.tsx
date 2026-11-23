import { t as uiText } from "../_lib/i18n";
// What is drawn over the sea: the round map, compass and speed, the
// island you are near, names over the islands, the settings, the big map,
// the keys, and buttons for touch screens. They read the world's status;
// they never reach into the scene.

import { useEffect, useRef, useState, type ReactNode } from "react";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Navigation, Square, X } from "lucide-react";
import { charts, distText, drawMap, palette, type Chart, type Course } from "./map";
import { type Isle, type Kind } from "./islands";
import type { Label, Status } from "./world";
import type { Controls, Key } from "./controls";
import type { Settings } from "./world";
import { WEATHERS, type Weather } from "./sky";
import { SPEEDS } from "./model";
import type { Sea } from "./waves";

export const KNOTS = 1.94384;

const KIND_NAME: Record<Kind, () => string> = {
  island: () => uiText("岛"), rocks: () => uiText("礁石"), stack: () => uiText("海蚀柱"), sandbar: () => uiText("沙洲"),
};
export const kindName = (k: Kind) => KIND_NAME[k]?.() ?? k;
const WEATHER_NAME: Record<Weather, () => string> = {
  clear: () => uiText("晴"), cloudy: () => uiText("多云"), overcast: () => uiText("阴"), rain: () => uiText("雨"), storm: () => uiText("雷雨"), fog: () => uiText("海雾"),
};
export const weatherName = (w: Weather) => WEATHER_NAME[w]?.() ?? w;
const SPEED_NAME = [() => uiText("停住"), () => uiText("一天 48 分钟"), () => uiText("一天 12 分钟")];
const SEA_NAME: Record<Sea, () => string> = { calm: () => uiText("平静"), moderate: () => uiText("中浪"), rough: () => uiText("大浪") };
const POINTS = () => [uiText("北"), uiText("东北"), uiText("东"), uiText("东南"), uiText("南"), uiText("西南"), uiText("西"), uiText("西北")];
export const pointOf = (deg: number) => POINTS()[Math.round((((deg % 360) + 360) % 360) / 45) % 8];
export const clock = (h: number) => `${String(Math.floor(h)).padStart(2, "0")}:${String(Math.round((h % 1) * 60) % 60).padStart(2, "0")}`;

/** The compass strip: where the bow points, and a course if one is set. */
export function Compass({ heading, course }: { heading: number; course: number | null }) {
  const deg = (heading * 180) / Math.PI;
  const ticks: ReactNode[] = [];
  for (let d = Math.floor((deg - 70) / 15) * 15; d <= deg + 70; d += 15) {
    const off = (d - deg) * 2.2; // px per degree
    const n = ((d % 360) + 360) % 360;
    const main = n % 45 === 0;
    ticks.push(<span key={d} className={`oc-tick${main ? " is-main" : ""}`} style={{ transform: `translateX(${off}px)` }}>{main ? <b>{POINTS()[n / 45]}</b> : null}</span>);
  }
  let mark: ReactNode = null;
  if (course != null) {
    let d = ((course * 180) / Math.PI - deg) % 360;
    if (d > 180) d -= 360; if (d < -180) d += 360;
    mark = <span className="oc-course" style={{ transform: `translateX(${Math.max(-150, Math.min(150, d * 2.2))}px)` }} />;
  }
  return (
    <div className="oc-compass" aria-label={uiText("航向 {0}°", Math.round(deg) % 360)}>
      <div className="oc-ticks">{ticks}{mark}</div>
      <span className="oc-needle" />
    </div>
  );
}

/** Speed, heading and the throttle lever. */
export function Gauges({ s }: { s: Status }) {
  const b = s.boat;
  const deg = Math.round((b.heading * 180) / Math.PI) % 360;
  const th = b.throttle;
  return (
    <div className="oc-gauges">
      <div className="oc-speed"><b>{(Math.abs(b.speed) * KNOTS).toFixed(1)}</b><span>{uiText("节")}</span></div>
      <div className="oc-lever" title={uiText("油门（W / S）")}>
        <span className="oc-lever-zero" />
        <span className={`oc-lever-fill${th < 0 ? " is-back" : ""}`} style={th >= 0 ? { left: "28.57%", width: `${th * 71.43}%` } : { left: `${28.57 + th * 71.43}%`, width: `${-th * 71.43}%` }} />
      </div>
      <div className="oc-gauge-note">{th < -0.01 ? uiText("倒车") : th > 0.01 ? uiText("油门 {0}%", Math.round(th * 100)) : uiText("空挡")} · {pointOf(deg)} {deg}°</div>
    </div>
  );
}

/** The round map in the corner (north up). */
export function MiniMap({ get, isles, course, onOpen }: { get: () => Status | null; isles: Isle[]; course: Course; onOpen: () => void }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const c = ref.current!;
    let cs: Chart[] = [], raf = 0, pal = palette(c), at = 0;
    const redo = () => { pal = palette(c); cs = charts(isles, pal); };
    redo();
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    mq.addEventListener?.("change", redo);
    const draw = (now: number) => {
      raf = requestAnimationFrame(draw);
      if (now - at < 66) return;
      at = now;
      const s = get();
      if (!s) return;
      const dpr = window.devicePixelRatio || 1, size = c.clientWidth;
      if (c.width !== Math.round(size * dpr)) { c.width = c.height = Math.round(size * dpr); }
      const g = c.getContext("2d")!;
      g.setTransform(dpr, 0, 0, dpr, 0, 0);
      drawMap(g, size, size, { cx: s.boat.x, cz: s.boat.z, scale: size / 1500 }, cs, s.boat, pal, { course });
    };
    raf = requestAnimationFrame(draw);
    return () => { cancelAnimationFrame(raf); mq.removeEventListener?.("change", redo); };
  }, [isles, course, get]);
  return (
    <button type="button" className="oc-mini" onClick={onOpen} aria-label={uiText("打开地图（M）")} title={uiText("打开地图（M）")}>
      <canvas ref={ref} />
      <span className="oc-mini-n">{uiText("北")}</span>
    </button>
  );
}

/** The big map: every island, names, the boat; pick an island to steer for. */
export function BigMap({ get, isles, found, course, onCourse, onClose }: {
  get: () => Status | null; isles: Isle[]; found: Set<string>; course: Course; onCourse: (c: Course) => void; onClose: () => void;
}) {
  const ref = useRef<HTMLCanvasElement>(null);
  const boat0 = get()?.boat;
  const [sel, setSel] = useState<string | null>(course ? isles.find((i) => i.name === course.name)?.id ?? null : null);
  const view = useRef({ cx: boat0?.x ?? 0, cz: boat0?.z ?? 0, scale: 0 });
  const drag = useRef<{ x: number; y: number; moved: boolean } | null>(null);
  const isle = isles.find((i) => i.id === sel) || null;
  useEffect(() => {
    const c = ref.current!;
    const pal = palette(c), cs = charts(isles, pal);
    if (!view.current.scale) {
      // fit every island and the boat
      const b = get()?.boat;
      const xs = [...isles.map((i) => i.x - i.size), ...isles.map((i) => i.x + i.size), b?.x ?? 0], zs = [...isles.map((i) => i.z - i.size), ...isles.map((i) => i.z + i.size), b?.z ?? 0];
      const w = c.clientWidth || 800, h = c.clientHeight || 600;
      const spanX = Math.max(400, Math.max(...xs) - Math.min(...xs)), spanZ = Math.max(400, Math.max(...zs) - Math.min(...zs));
      view.current = { cx: (Math.max(...xs) + Math.min(...xs)) / 2, cz: (Math.max(...zs) + Math.min(...zs)) / 2, scale: Math.min(w / spanX, h / spanZ) * 0.8 };
    }
    let raf = 0;
    const draw = () => {
      raf = requestAnimationFrame(draw);
      const s = get();
      if (!s) return;
      const dpr = window.devicePixelRatio || 1, w = c.clientWidth, h = c.clientHeight;
      if (c.width !== Math.round(w * dpr) || c.height !== Math.round(h * dpr)) { c.width = Math.round(w * dpr); c.height = Math.round(h * dpr); }
      const g = c.getContext("2d")!;
      g.setTransform(dpr, 0, 0, dpr, 0, 0);
      drawMap(g, w, h, view.current, cs, s.boat, pal, { names: true, course, selected: sel ?? undefined, found });
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, [isles, course, sel, found, get]);
  const at = (e: { clientX: number; clientY: number }) => {
    const r = ref.current!.getBoundingClientRect(), v = view.current;
    return { x: v.cx + (e.clientX - r.left - r.width / 2) / v.scale, z: v.cz + (e.clientY - r.top - r.height / 2) / v.scale };
  };
  const b = get()?.boat;
  return (
    <div className="oc-sheet" role="dialog" aria-label={uiText("地图")}>
      <div className="oc-sheet-head">
        <b>{uiText("地图")}</b>
        <span className="faint">{uiText("到过 {0} / {1} 处", isles.filter((i) => found.has(i.id)).length, isles.length)}</span>
        <span className="top-gap" />
        <button type="button" className="icon-btn" onClick={onClose} aria-label={uiText("关闭")}><X className="ico" /></button>
      </div>
      <div className="oc-map-wrap">
        <canvas ref={ref} className="oc-map"
          onPointerDown={(e) => { (e.target as HTMLElement).setPointerCapture(e.pointerId); drag.current = { x: e.clientX, y: e.clientY, moved: false }; }}
          onPointerMove={(e) => {
            const d = drag.current; if (!d) return;
            const dx = e.clientX - d.x, dy = e.clientY - d.y;
            if (Math.abs(dx) + Math.abs(dy) > 3) d.moved = true;
            view.current.cx -= dx / view.current.scale; view.current.cz -= dy / view.current.scale;
            d.x = e.clientX; d.y = e.clientY;
          }}
          onPointerUp={(e) => {
            const d = drag.current; drag.current = null;
            if (d?.moved) return;
            const p = at(e), v = view.current;
            const hit = isles.map((i) => ({ i, d: Math.hypot(i.x - p.x, i.z - p.z) - Math.max(i.size * 1.1, 14 / v.scale) })).filter((x) => x.d < 0).sort((a, b2) => a.d - b2.d)[0];
            setSel(hit ? hit.i.id : null);
          }}
          onWheel={(e) => { const v = view.current; v.scale = Math.min(4, Math.max(0.02, v.scale * Math.exp(-e.deltaY * 0.0015))); }} />
        <div className="oc-places" aria-label={uiText("地点")}>
          {isles.filter((i) => i.name).map((i) => ({ i, d: b ? Math.max(0, Math.hypot(b.x - i.x, b.z - i.z) - i.size) : 0 })).sort((x, y) => x.d - y.d).map(({ i, d }) => (
            <button key={i.id} type="button" className={`oc-place${sel === i.id ? " is-on" : ""}`} onClick={() => { setSel(i.id); const v = view.current; v.cx = i.x; v.cz = i.z; }}>
              <span className={`oc-place-dot${found.has(i.id) ? " is-found" : ""}`} />
              <b>{i.name}</b><span className="faint">{distText(d)}</span>
            </button>
          ))}
        </div>
        {isle && (
          <div className="oc-pick">
            <div className="oc-pick-head"><b>{isle.name || uiText("无名")}</b><span className="faint">{kindName(isle.kind)}{b ? ` · ${distText(Math.max(0, Math.hypot(b.x - isle.x, b.z - isle.z) - isle.size))}` : ""}</span></div>
            {isle.note && <p className="oc-pick-note">{isle.note.split(/\n{2,}/)[0].slice(0, 160)}</p>}
            <div className="oc-pick-row">
              {course?.name === isle.name
                ? <button type="button" className="btn" onClick={() => onCourse(null)}>{uiText("取消航向")}</button>
                : <button type="button" className="btn btn-primary" onClick={() => { onCourse({ x: isle.x, z: isle.z, name: isle.name || uiText("无名") }); onClose(); }}><Navigation className="ico" />{uiText("开往这里")}</button>}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/** The island you are near: its name and what is written about it. */
export function IslandCard({ isle, dist, onClose }: { isle: Isle; dist: number; onClose: () => void }) {
  const paras = isle.note.split(/\n{2,}/).map((p) => p.trim()).filter(Boolean).slice(0, 3);
  return (
    <div className="oc-card" role="status">
      <div className="oc-card-head">
        <b>{isle.name || uiText("无名")}</b>
        <span className="faint">{kindName(isle.kind)} · {dist < 15 ? uiText("就在旁边") : distText(dist)}</span>
        <span className="top-gap" />
        <button type="button" className="icon-btn" onClick={onClose} aria-label={uiText("收起")}><X className="ico" /></button>
      </div>
      {paras.map((p, i) => <p key={i}>{p.replace(/[*_`#>]/g, "")}</p>)}
    </div>
  );
}

/** Names over the islands in view. */
export function Labels({ labels }: { labels: Label[] }) {
  return <>{labels.map((l) => <div key={l.id} className="oc-label" style={{ left: Math.round(l.x), top: Math.round(l.y) }}><b>{l.name}</b><span>{distText(l.dist)}</span></div>)}</>;
}

/** The shared settings: hour, weather, sea, wind (everyone sees the same). */
export function SettingsPanel({ value, hour, title, onChange, onTitle, onStartHere, onGoStart, onClose }: {
  value: Settings; hour: number; title: string; onChange: (s: Settings) => void; onTitle: (t: string) => void; onStartHere: () => void; onGoStart: () => void; onClose: () => void;
}) {
  return (
    <div className="oc-panel pop" role="dialog" aria-label={uiText("海况和天气")}>
      <div className="oc-panel-head"><b>{uiText("海况和天气")}</b><span className="faint">{uiText("大家看到的一样")}</span><span className="top-gap" /><button type="button" className="icon-btn" onClick={onClose} aria-label={uiText("关闭")}><X className="ico" /></button></div>
      <label className="oc-field"><span>{uiText("名字")}</span><input className="inp" value={title} maxLength={60} placeholder={uiText("这片海叫什么")} onChange={(e) => onTitle(e.target.value)} /></label>
      <label className="oc-field"><span>{uiText("时间")} <b className="oc-num">{clock(hour)}</b></span>
        <input type="range" min={0} max={23.75} step={0.25} value={Math.round(hour * 4) / 4} onChange={(e) => onChange({ ...value, time: Number(e.target.value), at: Date.now() })} /></label>
      <div className="oc-field"><span>{uiText("时间流动")}</span>
        <div className="seg">{SPEEDS.map((sp, i) => <button key={sp} type="button" className={value.speed === sp ? "is-on" : ""} onClick={() => onChange({ ...value, time: hour, at: Date.now(), speed: sp })}>{SPEED_NAME[i]()}</button>)}</div></div>
      <div className="oc-field"><span>{uiText("天气")}</span>
        <div className="seg oc-seg-wrap">{WEATHERS.map((w) => <button key={w} type="button" className={value.weather === w ? "is-on" : ""} onClick={() => onChange({ ...value, weather: w })}>{WEATHER_NAME[w]()}</button>)}</div></div>
      <div className="oc-field"><span>{uiText("海况")}</span>
        <div className="seg">{(["calm", "moderate", "rough"] as Sea[]).map((w) => <button key={w} type="button" className={value.sea === w ? "is-on" : ""} onClick={() => onChange({ ...value, sea: w })}>{SEA_NAME[w]()}</button>)}</div></div>
      <label className="oc-field"><span>{uiText("风向")} <b className="oc-num">{uiText("{0}风", pointOf(value.wind))} {Math.round(value.wind)}°</b></span>
        <input type="range" min={0} max={359} step={1} value={value.wind} onChange={(e) => onChange({ ...value, wind: Number(e.target.value) })} /></label>
      <div className="oc-panel-foot">
        <button type="button" className="btn" onClick={onGoStart}>{uiText("回到出发点")}</button>
        <button type="button" className="btn" onClick={onStartHere}>{uiText("把这里设为出发点")}</button>
      </div>
    </div>
  );
}

/** Keys, shown at the start and from the keyboard button. */
export function KeysHelp({ onClose }: { onClose: () => void }) {
  const rows: [string, string][] = [
    ["W / ↑", uiText("加油门")], ["S / ↓", uiText("减油门、倒车")], ["A D / ← →", uiText("转舵")], [uiText("空格"), uiText("停车")],
    [uiText("拖动"), uiText("转视角")], [uiText("滚轮"), uiText("拉近拉远")], [uiText("双击"), uiText("视角回到船后")], ["M", uiText("地图")],
  ];
  return (
    <div className="oc-keys pop" role="dialog" aria-label={uiText("操作方法")}>
      <div className="oc-panel-head"><b>{uiText("操作方法")}</b><span className="top-gap" /><button type="button" className="icon-btn" onClick={onClose} aria-label={uiText("关闭")}><X className="ico" /></button></div>
      <dl>{rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
    </div>
  );
}

/** Buttons for touch screens: rudder on the left, throttle on the right. */
export function TouchPad({ controls }: { controls: Controls }) {
  const hold = (k: Key, label: string, icon: ReactNode) => (
    <button type="button" className="oc-pad-btn" aria-label={label}
      onPointerDown={(e) => { (e.target as HTMLElement).setPointerCapture(e.pointerId); controls.press(k, true); }}
      onPointerUp={() => controls.press(k, false)} onPointerCancel={() => controls.press(k, false)} onContextMenu={(e) => e.preventDefault()}>{icon}</button>
  );
  return (
    <>
      <div className="oc-pad is-left">{hold("left", uiText("左舵"), <ChevronLeft className="ico" />)}{hold("right", uiText("右舵"), <ChevronRight className="ico" />)}</div>
      <div className="oc-pad is-right">{hold("up", uiText("加油门"), <ChevronUp className="ico" />)}{hold("stop", uiText("停车"), <Square className="ico" />)}{hold("down", uiText("减油门"), <ChevronDown className="ico" />)}</div>
    </>
  );
}
