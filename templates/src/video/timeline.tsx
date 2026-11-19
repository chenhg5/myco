// The timeline: a ruler, lanes of overlays (text, blur, mosaic) above the
// main track of clips. Clips follow one another; drag one to move it, drag
// its ends to trim. Overlays move freely in time and between lanes. The
// edges of everything and the playhead pull what is dragged near them.
import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type PointerEvent as RPE, type DragEvent as RDE } from "react";
import { Copy, Film, Minus, Plus, Redo2, Scissors, Trash2, Type, Undo2, Grid3x3, Droplets, Maximize2, VolumeX } from "lucide-react";
import { clips, draft, kit, media, notes, overlayList, placed, total, MIN_CLIP, clamp, num, fmt, type Placed, type OverlayItem } from "./model";
import { canRedo, canUndo, duplicateClip, patch, redo, removeClip, removeOverlay, split, undo, change } from "./edit";
import { localFor, onLocal, type Local } from "./media";
import { player } from "./player";
import { overlays } from "./model";

export type Sel = { kind: "clip" | "overlay"; id: string } | null;

const GUTTER = 44, RULER = 28, LANE = 30, MAIN = 66, THUMB_H = 46;
const TYPE_NAME = { text: "文字", blur: "模糊", mosaic: "马赛克" } as const;

function usePlayer() { return useSyncExternalStore(player.subscribe, player.snapshot); }
function useLocal() {
  const [, set] = useState(0);
  useEffect(() => onLocal(() => set((n) => n + 1)), []);
}

function tickStep(pps: number) {
  for (const s of [0.1, 0.2, 0.5, 1, 2, 5, 10, 15, 30, 60, 120, 300, 600]) if (s * pps >= 70) return s;
  return 1200;
}

export function Timeline({ sel, setSel, pps, setPps, onFiles, onNoteClick }: {
  sel: Sel; setSel: (s: Sel) => void; pps: number; setPps: (n: number) => void;
  onFiles: (files: File[], index?: number) => void; onNoteClick: (id: string) => void;
}) {
  useLocal();
  const scroller = useRef<HTMLDivElement>(null);
  const origin = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ left: 0, width: 800 });
  const [dragLane, setDragLane] = useState(false);
  const [dropAt, setDropAt] = useState<number | null>(null);
  const op = useRef<null | { move: (e: PointerEvent) => void; up: (e: PointerEvent) => void }>(null);

  const list = placed();
  const over = overlayList();
  const T = total(list);
  const lanes = Math.max(2, over.reduce((m, o) => Math.max(m, o.lane + 1), 0)) + (dragLane ? 1 : 0);
  const contentW = Math.max(view.width - GUTTER, (T + 30) * pps);

  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const upd = () => setView({ left: el.scrollLeft, width: el.clientWidth });
    upd();
    const ro = new ResizeObserver(upd);
    ro.observe(el);
    el.addEventListener("scroll", upd, { passive: true });
    return () => { ro.disconnect(); el.removeEventListener("scroll", upd); };
  }, []);

  // ctrl / ⌘ + wheel zooms around the pointer
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const wheel = (e: WheelEvent) => {
      if (!(e.ctrlKey || e.metaKey)) return;
      e.preventDefault();
      const x = e.clientX - el.getBoundingClientRect().left - GUTTER + el.scrollLeft;
      const t = x / pps;
      const next = clamp(pps * Math.exp(-e.deltaY * 0.01), 2, 400);
      setPps(next);
      requestAnimationFrame(() => { el.scrollLeft = Math.max(0, t * next - (x - el.scrollLeft)); });
    };
    el.addEventListener("wheel", wheel, { passive: false });
    return () => el.removeEventListener("wheel", wheel);
  }, [pps, setPps]);

  const tAt = (clientX: number) => {
    const r = origin.current!.getBoundingClientRect();
    return Math.max(0, (clientX - r.left) / pps);
  };
  const laneAt = (clientY: number) => {
    const r = origin.current!.getBoundingClientRect();
    return Math.floor((clientY - r.top - RULER) / LANE);
  };

  function snapPoints(exceptClip = "", exceptOverlay = "") {
    const pts = [0, player.time];
    for (const c of placed()) if (c.id !== exceptClip) pts.push(c.start, c.end);
    for (const o of overlayList()) if (o.id !== exceptOverlay) pts.push(o.start, o.end);
    for (const n of notes.rows) pts.push(num(n.data.time, 0));
    return pts;
  }
  const snap = (t: number, pts: number[]) => {
    let best = t, d = 8 / pps;
    for (const p of pts) if (Math.abs(p - t) < d) { d = Math.abs(p - t); best = p; }
    return best;
  };

  function drag(e: RPE, move: (e: PointerEvent, dx: number, dy: number) => void, up: (moved: boolean) => void) {
    e.preventDefault();
    e.stopPropagation();
    const sx = e.clientX, sy = e.clientY;
    let moved = false;
    const mv = (ev: PointerEvent) => {
      const dx = ev.clientX - sx, dy = ev.clientY - sy;
      if (!moved && Math.abs(dx) + Math.abs(dy) < 3) return;
      moved = true;
      move(ev, dx, dy);
      kit.emit();
    };
    const u = () => {
      window.removeEventListener("pointermove", mv);
      window.removeEventListener("pointerup", u);
      op.current = null;
      up(moved);
      kit.emit();
    };
    window.addEventListener("pointermove", mv);
    window.addEventListener("pointerup", u);
    op.current = { move: mv, up: u };
  }

  // ---- clips ----
  function clipDown(e: RPE, c: Placed) {
    if (e.button !== 0) return;
    setSel({ kind: "clip", id: c.id });
    const others = clips.rows.filter((r) => r.id !== c.id);
    const startList = placed().filter((x) => x.id !== c.id);
    let index = -1;
    drag(e, (ev) => {
      const t = tAt(ev.clientX);
      // where it would go: before the first clip whose middle is right of the pointer
      let i = 0;
      let acc = 0;
      for (const x of startList) { const len = x.end - x.start; if (t < acc + len / 2) break; acc += len; i++; }
      if (i !== index) {
        index = i;
        const ids = others.map((r) => r.id);
        ids.splice(i, 0, c.id);
        draft.order = ids;
      }
    }, (moved) => {
      draft.order = null;
      if (!moved) { player.seek(tAt(e.clientX)); return; }
      const cur = clips.rows.findIndex((r) => r.id === c.id);
      const othersIdx = others.map((r) => r.id);
      if (index < 0 || (othersIdx[index - 1] ?? null) === (clips.rows[cur - 1]?.id ?? null)) return;
      const before = index > 0 ? num(others[index - 1].data.order, index - 1) : null;
      const after = index < others.length ? num(others[index].data.order, index + 1) : null;
      const order = before === null && after === null ? 1 : before === null ? (after as number) - 1 : after === null ? before + 1 : (before + after) / 2;
      patch(clips, c.id, { order });
    });
  }
  function trimDown(e: RPE, c: Placed, side: "l" | "r") {
    setSel({ kind: "clip", id: c.id });
    const full = num(media.get(c.media)?.data.duration, c.out) || c.out;
    const pts = snapPoints(c.id);
    drag(e, (_ev, dx) => {
      const dt = dx / pps;
      if (side === "l") {
        const nin = clamp(c.in + dt * c.speed, 0, c.out - MIN_CLIP * c.speed);
        draft.clips.set(c.id, { in: nin });
        player.seek(c.start);
      } else {
        let end = snap(c.end + dt, pts);
        let nout = c.in + (end - c.start) * c.speed;
        nout = clamp(nout, c.in + MIN_CLIP * c.speed, full);
        draft.clips.set(c.id, { out: nout });
        end = c.start + (nout - c.in) / c.speed;
        player.seek(Math.max(c.start, end - 0.001));
      }
    }, (moved) => {
      const d = draft.clips.get(c.id);
      draft.clips.delete(c.id);
      if (moved && d) patch(clips, c.id, d as Record<string, unknown>);
    });
  }

  // ---- overlays ----
  function overDown(e: RPE, o: OverlayItem, mode: "move" | "l" | "r") {
    if (e.button !== 0) return;
    setSel({ kind: "overlay", id: o.id });
    if (player.time < o.start || player.time >= o.end) player.seek(o.start);
    const pts = snapPoints("", o.id);
    if (mode === "move") setDragLane(true);
    drag(e, (ev, dx) => {
      const dt = dx / pps;
      if (mode === "move") {
        let s = Math.max(0, o.start + dt);
        const len = o.end - o.start;
        const a = snap(s, pts), b = snap(s + len, pts) - len;
        s = Math.abs(a - s) <= Math.abs(b - s) ? a : b;
        s = Math.max(0, s);
        const lane = clamp(laneAt(ev.clientY), 0, 11);
        draft.overlays.set(o.id, { start: s, end: s + len, lane });
      } else if (mode === "l") {
        const s = clamp(snap(o.start + dt, pts), 0, o.end - MIN_CLIP);
        draft.overlays.set(o.id, { start: s });
        player.seek(s);
      } else {
        const en = Math.max(o.start + MIN_CLIP, snap(o.end + dt, pts));
        draft.overlays.set(o.id, { end: en });
        player.seek(Math.max(o.start, en - 0.001));
      }
    }, (moved) => {
      setDragLane(false);
      const d = draft.overlays.get(o.id);
      draft.overlays.delete(o.id);
      if (moved && d) patch(overlays, o.id, d as Record<string, unknown>);
    });
  }

  // ---- the ruler and empty space: seek ----
  function scrub(e: RPE) {
    if (e.button !== 0) return;
    setSel(null);
    player.seek(tAt(e.clientX));
    drag(e, (ev) => player.seek(tAt(ev.clientX)), () => undefined);
  }

  // ---- dropping files or library items ----
  const insertIndex = (t: number) => {
    let i = 0;
    for (const c of placed()) { if (t < (c.start + c.end) / 2) break; i++; }
    return i;
  };
  const dragOver = (e: RDE) => {
    const types = Array.from(e.dataTransfer.types);
    if (!types.includes("Files") && !types.includes("application/x-myco-media")) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
    const i = insertIndex(tAt(e.clientX));
    const pl = placed();
    setDropAt(i < pl.length ? pl[i].start : T);
  };
  const drop = (e: RDE) => {
    setDropAt(null);
    const i = insertIndex(tAt(e.clientX));
    const mid = e.dataTransfer.getData("application/x-myco-media");
    if (mid) {
      e.preventDefault();
      const m = media.get(mid);
      if (m) {
        const order = (() => {
          const rows = clips.rows;
          const before = i > 0 ? num(rows[i - 1].data.order, i - 1) : null, after = i < rows.length ? num(rows[i].data.order, i + 1) : null;
          return before === null && after === null ? 1 : before === null ? (after as number) - 1 : after === null ? before + 1 : (before + after) / 2;
        })();
        const id = `c${Math.random().toString(36).slice(2, 9)}`;
        change([[clips, id, { media: mid, in: 0, out: num(m.data.duration, 5), speed: 1, volume: 1, muted: false, order }]]);
        setSel({ kind: "clip", id });
      }
      return;
    }
    const files = Array.from(e.dataTransfer.files).filter((f) => f.type.startsWith("video/") || /\.(mp4|mov|m4v|webm|mkv)$/i.test(f.name));
    if (files.length) { e.preventDefault(); onFiles(files, i); }
  };

  const fitAll = () => {
    const w = (scroller.current?.clientWidth || 800) - GUTTER - 40;
    setPps(clamp(w / Math.max(T, 5), 2, 400));
    if (scroller.current) scroller.current.scrollLeft = 0;
  };

  const selClip = sel?.kind === "clip" ? sel.id : "";
  const selOver = sel?.kind === "overlay" ? sel.id : "";
  const step = tickStep(pps);
  const ticks: number[] = [];
  const t0 = Math.max(0, Math.floor((view.left - 100) / pps / step) * step), t1 = (view.left + view.width + 100) / pps;
  for (let t = t0; t <= t1; t += step) ticks.push(Math.round(t * 1000) / 1000);
  const H = RULER + lanes * LANE + MAIN + 8;
  const visible = (x: number, w: number) => x + w >= view.left - 300 && x <= view.left + view.width + 300;

  return (
    <section className="tl" aria-label="时间线">
      <div className="tl-bar">
        <button type="button" className="icon-btn" title="撤销 (⌘Z)" aria-label="撤销" disabled={!canUndo()} onClick={undo}><Undo2 className="ico" /></button>
        <button type="button" className="icon-btn" title="重做 (⇧⌘Z)" aria-label="重做" disabled={!canRedo()} onClick={redo}><Redo2 className="ico" /></button>
        <span className="toolbar-sep" />
        <button type="button" className="tool" title="在播放头处分割 (S)" disabled={!list.length} onClick={() => { const n = split(player.time); if (n) setSel({ kind: "clip", id: n }); else kit.flash("播放头要在一段视频中间"); }}>
          <Scissors className="ico" />分割</button>
        <button type="button" className="tool" title="复制一段 (⌘D)" disabled={!selClip} onClick={() => { const n = duplicateClip(selClip); if (n) setSel({ kind: "clip", id: n }); }}><Copy className="ico" />复制</button>
        <button type="button" className="tool" title="删除 (Delete)" disabled={!sel} onClick={() => { if (selClip) removeClip(selClip); else if (selOver) removeOverlay(selOver); setSel(null); }}>
          <Trash2 className="ico" />删除</button>
        <span className="tl-gap" />
        <span className="tl-dur">{fmt(T, true)}</span>
        <button type="button" className="icon-btn" aria-label="缩小" title="缩小" onClick={() => setPps(clamp(pps / 1.5, 2, 400))}><Minus className="ico" /></button>
        <input className="tl-zoom" type="range" min={0} max={100} aria-label="缩放时间线" value={Math.round((Math.log(pps / 2) / Math.log(200)) * 100)}
          onChange={(e) => setPps(2 * Math.pow(200, Number(e.target.value) / 100))} />
        <button type="button" className="icon-btn" aria-label="放大" title="放大" onClick={() => setPps(clamp(pps * 1.5, 2, 400))}><Plus className="ico" /></button>
        <button type="button" className="icon-btn" aria-label="适合宽度" title="整条时间线放进视野" onClick={fitAll}><Maximize2 className="ico" /></button>
      </div>
      <div className="tl-body" ref={scroller} onDragOver={dragOver} onDragLeave={() => setDropAt(null)} onDrop={drop}>
        <div className="tl-inner" style={{ width: contentW + GUTTER, height: H }}>
          <div className="tl-gutter" style={{ height: H }}>
            <div style={{ height: RULER }} />
            {Array.from({ length: lanes }, (_, i) => (
              <div key={i} className="tl-gut" style={{ height: LANE }}>
                {over.some((o) => o.lane === i && o.type === "text") || (i === 0 && !over.some((o) => o.lane === 0)) ? <Type className="ico" /> : <Droplets className="ico" />}
              </div>
            ))}
            <div className="tl-gut is-main" style={{ height: MAIN }} title="主轨道"><Film className="ico" /></div>
          </div>
          <div className="tl-origin" ref={origin} style={{ left: GUTTER, width: contentW, height: H }}>
            <div className="tl-ruler" style={{ height: RULER }} onPointerDown={scrub}>
              {ticks.map((t) => (
                <div key={t} className="tl-tick" style={{ left: t * pps }}>{(step >= 1 || Math.abs(t - Math.round(t)) < 1e-6) && <span>{fmt(t)}</span>}</div>
              ))}
              {ticks.map((t) => [1, 2, 3, 4].map((k) => {
                const x = (t + (k * step) / 5) * pps;
                return <div key={`${t}-${k}`} className="tl-minor" style={{ left: x }} />;
              }))}
              {notes.rows.map((n) => (
                <button key={n.id} type="button" className={`tl-note${n.data.done ? " is-done" : ""}`} style={{ left: num(n.data.time, 0) * pps }}
                  title={String(n.data.text || "备注")} aria-label={`备注：${String(n.data.text || "")}`}
                  onPointerDown={(e) => e.stopPropagation()} onClick={() => { player.seek(num(n.data.time, 0)); onNoteClick(n.id); }} />
              ))}
            </div>
            <div className="tl-lanes" style={{ top: RULER, height: lanes * LANE }} onPointerDown={scrub}>
              {Array.from({ length: lanes }, (_, i) => <div key={i} className="tl-lane" style={{ top: i * LANE, height: LANE }} />)}
              {over.map((o) => {
                const x = o.start * pps, w = Math.max(6, (o.end - o.start) * pps);
                const Icon = o.type === "text" ? Type : o.type === "mosaic" ? Grid3x3 : Droplets;
                return (
                  <div key={o.id} className={`tl-ov is-${o.type}${o.id === selOver ? " is-sel" : ""}`} style={{ left: x, width: w, top: o.lane * LANE + 3, height: LANE - 6 }}
                    onPointerDown={(e) => overDown(e, o, "move")} title={`${TYPE_NAME[o.type]} ${fmt(o.start, true)} – ${fmt(o.end, true)}`}>
                    <Icon className="ico" /><span>{o.type === "text" ? o.text || "文字" : TYPE_NAME[o.type]}</span>
                    <i className="tl-h is-l" onPointerDown={(e) => overDown(e, o, "l")} />
                    <i className="tl-h is-r" onPointerDown={(e) => overDown(e, o, "r")} />
                  </div>
                );
              })}
            </div>
            <div className="tl-main" style={{ top: RULER + lanes * LANE + 4, height: MAIN }} onPointerDown={scrub}>
              {!list.length && <div className="tl-empty">把视频拖到这里，或者在左边「素材」里点 ＋</div>}
              {list.map((c) => {
                const m = media.get(c.media)?.data;
                const l = localFor(String(m?.key || ""));
                const x = c.start * pps, w = Math.max(4, (c.end - c.start) * pps);
                return (
                  <div key={c.id} className={`tl-clip${c.id === selClip ? " is-sel" : ""}${l ? "" : " is-missing"}${draft.order && c.id === selClip ? " is-lift" : ""}`}
                    style={{ left: x, width: w, height: MAIN }} onPointerDown={(e) => clipDown(e, c)}
                    title={`${m?.name || "视频"} · ${fmt(c.end - c.start, true)}${c.speed !== 1 ? ` · ${c.speed}x` : ""}`}>
                    {l && visible(x, w) && <Thumbs l={l} c={c} pps={pps} x={x} w={w} view={view} aspect={num(m?.width, 16) / num(m?.height, 9)} />}
                    {!l && <div className="tl-miss">本机没有这个文件</div>}
                    {l && visible(x, w) && <Wave l={l} c={c} pps={pps} x={x} w={w} view={view} />}
                    <div className="tl-clip-tags">
                      {c.speed !== 1 && <span className="tl-tag">{c.speed}x</span>}
                      {(c.muted || c.volume === 0) && <span className="tl-tag"><VolumeX className="ico" /></span>}
                      <span className="tl-tag is-name">{m?.name || "视频"}</span>
                    </div>
                    <i className="tl-h is-l" onPointerDown={(e) => trimDown(e, c, "l")} />
                    <i className="tl-h is-r" onPointerDown={(e) => trimDown(e, c, "r")} />
                  </div>
                );
              })}
            </div>
            {dropAt !== null && <div className="tl-drop" style={{ left: dropAt * pps, top: RULER + lanes * LANE, height: MAIN + 8 }} />}
            <Playhead pps={pps} height={H} scroller={scroller} onDown={(e) => { setSel(sel); drag(e, (ev) => player.seek(tAt(ev.clientX)), () => undefined); }} />
          </div>
        </div>
      </div>
    </section>
  );
}

function Playhead({ pps, height, scroller, onDown }: { pps: number; height: number; scroller: React.RefObject<HTMLDivElement>; onDown: (e: RPE) => void }) {
  usePlayer();
  const x = player.time * pps;
  useEffect(() => {
    const el = scroller.current;
    if (!el || !player.playing) return;
    const w = el.clientWidth - GUTTER;
    if (x < el.scrollLeft || x > el.scrollLeft + w - 40) el.scrollLeft = Math.max(0, x - 60);
  });
  return (
    <div className="tl-play" style={{ left: x, height }}>
      <div className="tl-play-head" onPointerDown={onDown} />
    </div>
  );
}

function Thumbs({ l, c, pps, x, w, view, aspect }: { l: Local; c: Placed; pps: number; x: number; w: number; view: { left: number; width: number }; aspect: number }) {
  const tileW = Math.max(24, Math.round(THUMB_H * (Number.isFinite(aspect) && aspect > 0 ? aspect : 16 / 9)));
  const from = Math.max(0, Math.floor((view.left - 300 - x) / tileW)), to = Math.min(Math.ceil(w / tileW), Math.ceil((view.left + view.width + 300 - x) / tileW));
  const thumbs = l.thumbs;
  if (!thumbs.length) return <div className="tl-thumbs is-loading" />;
  const tiles = [];
  for (let i = from; i < to; i++) {
    const t = c.in + ((i + 0.5) * tileW / pps) * c.speed;
    let best = thumbs[0];
    for (const th of thumbs) if (Math.abs(th.t - t) < Math.abs(best.t - t)) best = th;
    tiles.push(<img key={i} src={best.url} alt="" draggable={false} style={{ left: i * tileW, width: tileW, height: THUMB_H }} />);
  }
  return <div className="tl-thumbs">{tiles}</div>;
}

function Wave({ l, c, pps, x, w, view }: { l: Local; c: Placed; pps: number; x: number; w: number; view: { left: number; width: number } }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const a = Math.max(0, view.left - 100 - x), b = Math.min(w, view.left + view.width + 100 - x);
  const width = Math.max(1, Math.round(b - a));
  useEffect(() => {
    const cv = ref.current;
    if (!cv || !l.peaks) return;
    const dpr = window.devicePixelRatio || 1;
    cv.width = width * dpr; cv.height = 16 * dpr;
    const ctx = cv.getContext("2d")!;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, width, 16);
    ctx.fillStyle = getComputedStyle(cv).color;
    const per = l.peaksPerSec, p = l.peaks;
    const quiet = c.muted || c.volume === 0;
    for (let px = 0; px < width; px++) {
      const s0 = c.in + ((a + px) / pps) * c.speed, s1 = c.in + ((a + px + 1) / pps) * c.speed;
      let m = 0;
      for (let k = Math.floor(s0 * per); k <= Math.floor(s1 * per) && k < p.length; k++) if (p[k] > m) m = p[k];
      const h = Math.max(1, Math.min(1, (m / l.peakMax) * (quiet ? 0 : c.volume)) * 15);
      ctx.fillRect(px, 16 - h, 1, h);
    }
  }, [l.peaks, c.in, c.speed, c.volume, c.muted, pps, a, width]);
  if (!l.peaks) return null;
  return <canvas ref={ref} className="tl-wave" style={{ left: a, width, height: 16 }} />;
}
