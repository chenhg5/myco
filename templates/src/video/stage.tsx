import { t as uiText } from "../_lib/i18n";
// The player: the canvas the edit is drawn on, the boxes of the text and
// regions under the playhead (drag to move, the corner to resize), and the
// transport under it.
import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type PointerEvent as RPE } from "react";
import { ChevronFirst, ChevronLast, Expand, Pause, Play, StepBack, StepForward, Upload } from "lucide-react";
import { draft, fmt, frameSize, kit, overlayList, overlays, placed, total, clamp } from "./model";
import { patch } from "./edit";
import { zoomTransform } from "./zoom";
import { player } from "./player";
import type { Sel } from "./timeline";

function usePlayer() { return useSyncExternalStore(player.subscribe, player.snapshot); }

export function Stage({ sel, setSel, onImport, onRecord }: { onRecord?: () => void; sel: Sel; setSel: (s: Sel) => void; onImport: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  const wrap = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [area, setArea] = useState({ w: 640, h: 360 });
  usePlayer();
  const list = placed();
  const { w: FW, h: FH } = frameSize(list);
  const k = Math.min(area.w / FW, area.h / FH);
  const cw = Math.max(1, Math.floor(FW * k)), ch = Math.max(1, Math.floor(FH * k));

  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setArea({ w: el.clientWidth, h: el.clientHeight }));
    ro.observe(el);
    setArea({ w: el.clientWidth, h: el.clientHeight });
    return () => ro.disconnect();
  }, []);
  useEffect(() => {
    const cv = canvas.current;
    if (!cv) return;
    const dpr = window.devicePixelRatio || 1;
    const scale = Math.min(1, (cw * dpr) / FW);
    cv.width = Math.max(2, Math.round(FW * scale)); cv.height = Math.max(2, Math.round(FH * scale));
    player.canvas = cv;
    player.redraw();
  }, [cw, ch, FW, FH]);

  const t = player.time;
  const live = overlayList().filter((o) => t >= o.start - 1e-6 && t < o.end - 1e-6);
  const z = zoomTransform(live.filter(o => o.type === "zoom").at(-1), t);
  const selected = sel?.kind === "overlay" ? live.find((o) => o.id === sel.id) : undefined;

  const selectionRect = selected && selected.type !== "text" ? {x:(selected.x-z.x)*z.scale+.5,y:(selected.y-z.y)*z.scale+.5,w:selected.w*z.scale,h:selected.h*z.scale}:selected;

  function pick(e: RPE) {
    const r = wrap.current!.getBoundingClientRect();
    const x = (e.clientX - r.left) / r.width, y = (e.clientY - r.top) / r.height;
    const hit = [...live].sort((a, b) => (a.type === "text" ? -1 : 1) - (b.type === "text" ? -1 : 1))
      .find((o) => { const px=o.type === "text" ? x : (x-.5)/z.scale+z.x, py=o.type === "text" ? y : (y-.5)/z.scale+z.y; return px >= o.x && px <= o.x+o.w && py >= o.y && py <= o.y+o.h; });
    if (hit) { setSel({ kind: "overlay", id: hit.id }); move(e, hit.id, "move"); }
    else setSel(null);
  }
  function move(e: RPE, id: string, mode: "move" | "se" | "nw" | "ne" | "sw") {
    e.preventDefault(); e.stopPropagation();
    const o = overlayList().find((x) => x.id === id);
    if (!o) return;
    const r = wrap.current!.getBoundingClientRect();
    const sx = e.clientX, sy = e.clientY;
    const mv = (ev: PointerEvent) => {
      const k = o.type === "text" ? 1 : z.scale;
      const dx = (ev.clientX - sx) / r.width / k, dy = (ev.clientY - sy) / r.height / k;
      let { x, y, w, h } = o;
      if (mode === "move") { x = clamp(o.x + dx, -o.w + 0.02, 0.98); y = clamp(o.y + dy, -o.h + 0.02, 0.98); }
      else {
        if (mode.includes("e")) w = Math.max(0.02, o.w + dx);
        if (mode.includes("s")) h = Math.max(0.02, o.h + dy);
        if (mode.includes("w")) { w = Math.max(0.02, o.w - dx); x = o.x + o.w - w; }
        if (mode.includes("n")) { h = Math.max(0.02, o.h - dy); y = o.y + o.h - h; }
      }
      draft.overlays.set(id, { x, y, w, h });
      kit.emit();
    };
    const up = () => {
      window.removeEventListener("pointermove", mv);
      window.removeEventListener("pointerup", up);
      const d = draft.overlays.get(id);
      draft.overlays.delete(id);
      if (d) patch(overlays, id, d as Record<string, unknown>);
      kit.emit();
    };
    window.addEventListener("pointermove", mv);
    window.addEventListener("pointerup", up);
  }

  const full = () => {
    const el = box.current?.parentElement;
    if (!el) return;
    if (document.fullscreenElement) void document.exitFullscreen();
    else void el.requestFullscreen?.().catch(() => kit.flash(uiText("这里不能全屏")));
  };
  const T = total(list);

  return (
    <div className="stage">
      <div className="stage-box" ref={box}>
        <div className="stage-frame" ref={wrap} style={{ width: cw, height: ch }} onPointerDown={pick}>
          <canvas ref={canvas} className="stage-canvas" />
          {selected && (
            <div className={`stage-sel is-${selected.type}`} style={{ left: `${selectionRect!.x * 100}%`, top: `${selectionRect!.y * 100}%`, width: `${selectionRect!.w * 100}%`, height: `${selectionRect!.h * 100}%` }}
              onPointerDown={(e) => move(e, selected.id, "move")}>
              {(["nw", "ne", "sw", "se"] as const).map((m) => <i key={m} className={`stage-h is-${m}`} onPointerDown={(e) => move(e, selected.id, m)} />)}
            </div>
          )}
        </div>
        {player.missing && list.length > 0 && (
          <div className="stage-missing">{uiText("本机没有「")}{player.missing}{uiText("」，在左边「素材」里点「本机没有，选文件」选一下它")}</div>
        )}
        {!list.length && (
          <div className="stage-empty">
            {onRecord && <button className="btn btn-primary" onClick={onRecord}>{uiText("录制屏幕，开始创作")}</button>}
            <button type="button" className="stage-drop" onClick={onImport}>
              <Upload className="ico" />
              <b>{uiText("导入视频开始剪辑")}</b>
              <span>{uiText("点这里选文件，或者把 mp4 拖进来。视频只存在这台电脑上，剪辑方案存在 myco 里，agent 也能改。")}</span>
            </button>
          </div>
        )}
      </div>
      <div className="transport">
        <span className="transport-time"><b>{fmt(t, true)}</b> / {fmt(T, true)}</span>
        <div className="transport-mid">
          <button type="button" className="icon-btn" title={uiText("到开头 (Home)")} aria-label={uiText("到开头")} onClick={() => player.seek(0)}><ChevronFirst className="ico" /></button>
          <button type="button" className="icon-btn" title={uiText("上一帧 (←)")} aria-label={uiText("上一帧")} onClick={() => player.step(-1)}><StepBack className="ico" /></button>
          <button type="button" className="play-btn" title={uiText("播放 / 暂停 (空格)")} aria-label={player.playing ? uiText("暂停") : uiText("播放")} disabled={!list.length} onClick={() => player.toggle()}>
            {player.playing ? <Pause className="ico" /> : <Play className="ico" />}
          </button>
          <button type="button" className="icon-btn" title={uiText("下一帧 (→)")} aria-label={uiText("下一帧")} onClick={() => player.step(1)}><StepForward className="ico" /></button>
          <button type="button" className="icon-btn" title={uiText("到结尾 (End)")} aria-label={uiText("到结尾")} onClick={() => player.seek(T)}><ChevronLast className="ico" /></button>
        </div>
        <span className="transport-right">
          <span className="faint">{FW}×{FH}</span>
          <button type="button" className="icon-btn" title={uiText("全屏")} aria-label={uiText("全屏")} onClick={full}><Expand className="ico" /></button>
        </span>
      </div>
    </div>
  );
}
