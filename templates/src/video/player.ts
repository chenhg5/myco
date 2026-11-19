// Plays the edit: one hidden <video> a file, the clip under the playhead
// playing at its speed, drawn with the overlays onto the canvas each frame.
// While a clip's video plays, its clock is the timeline's; without one (a
// file not on this computer) a plain clock runs on.
import { clipAt, frameSize, kit, media, overlayList, placed, project, sourceTime, total, type Placed } from "./model";
import { drawFrame } from "./render";
import { localFor } from "./media";

class Player {
  time = 0;
  playing = false;
  canvas: HTMLCanvasElement | null = null;
  private els = new Map<string, HTMLVideoElement>();
  private gains = new Map<HTMLVideoElement, GainNode>();
  private audio: AudioContext | null = null;
  private host: HTMLDivElement | null = null;
  private active = "";
  private last = 0;
  private dirty = true;
  private subs = new Set<() => void>();
  private ver = 0;
  private lastEmit = 0;
  selected = "";
  /** The name of the file under the playhead when this computer lacks it. */
  missing = "";

  constructor() {
    const loop = (now: number) => { this.tick(now); requestAnimationFrame(loop); };
    requestAnimationFrame(loop);
  }
  subscribe = (f: () => void) => { this.subs.add(f); return () => { this.subs.delete(f); }; };
  snapshot = () => this.ver;
  private emit(force = false) {
    const now = performance.now();
    if (!force && now - this.lastEmit < 40) return;
    this.lastEmit = now;
    this.ver++;
    for (const f of this.subs) f();
  }
  redraw = () => { this.dirty = true; };

  private video(c: Placed | null) {
    if (!c) return null;
    const key = String(media.get(c.media)?.data.key || "");
    const l = localFor(key);
    if (!l) return null;
    let v = this.els.get(key);
    if (!v) {
      if (!this.host) {
        this.host = document.createElement("div");
        this.host.style.cssText = "position:fixed;left:0;top:0;width:2px;height:2px;opacity:0;pointer-events:none;overflow:hidden";
        document.body.appendChild(this.host);
      }
      v = document.createElement("video");
      v.playsInline = true; v.preload = "auto"; v.src = l.url;
      (v as HTMLVideoElement & { preservesPitch?: boolean }).preservesPitch = true;
      v.addEventListener("seeked", this.redraw);
      v.addEventListener("loadeddata", this.redraw);
      this.host.appendChild(v);
      this.els.set(key, v);
      if (this.audio) this.route(v);
    }
    return v;
  }
  private route(v: HTMLVideoElement) {
    if (!this.audio || this.gains.has(v)) return;
    try {
      const g = this.audio.createGain();
      this.audio.createMediaElementSource(v).connect(g).connect(this.audio.destination);
      this.gains.set(v, g);
    } catch { /* (the element's own volume then) */ }
  }
  private level(v: HTMLVideoElement, c: Placed) {
    const vol = c.muted ? 0 : c.volume;
    const g = this.gains.get(v);
    if (g) { v.volume = 1; g.gain.value = vol; } else v.volume = Math.min(1, vol);
    v.muted = vol === 0;
  }

  private stopAll(except?: HTMLVideoElement | null) {
    for (const v of this.els.values()) if (v !== except && !v.paused) v.pause();
  }
  /** Puts the clip's video where timeline time t is, playing if we play. */
  private activate(c: Placed, t: number) {
    const v = this.video(c);
    this.stopAll(v);
    this.active = c.id;
    if (!v) return;
    const want = sourceTime(c, t);
    if (Math.abs(v.currentTime - want) > 0.05) v.currentTime = want;
    v.playbackRate = c.speed;
    this.level(v, c);
    if (this.playing) v.play().catch(() => { /* (autoplay refused: the clock runs) */ });
    else v.pause();
  }

  play() {
    const list = placed();
    if (!list.length) return;
    if (this.time >= total(list) - 0.02) this.time = 0;
    if (!this.audio) {
      try {
        const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
        this.audio = new Ctx();
        for (const v of this.els.values()) this.route(v);
      } catch { /* (no Web Audio: volumes above 100% play at 100%) */ }
    }
    void this.audio?.resume();
    this.playing = true;
    this.active = "";
    this.last = performance.now();
    this.emit(true);
  }
  pause() {
    this.playing = false;
    this.stopAll();
    this.active = "";
    this.dirty = true;
    this.emit(true);
  }
  toggle() { if (this.playing) this.pause(); else this.play(); }
  seek(t: number) {
    const T = total();
    this.time = Math.max(0, Math.min(T, t));
    this.active = "";
    if (this.playing) { this.last = performance.now(); }
    else {
      const c = clipAt(placed(), this.time);
      const v = this.video(c);
      if (c && v) { this.stopAll(); const want = sourceTime(c, this.time); if (Math.abs(v.currentTime - want) > 0.01) v.currentTime = want; }
    }
    this.dirty = true;
    this.emit(true);
  }
  step(frames: number) { this.seek(Math.round(this.time * 30 + frames) / 30); }

  private tick(now: number) {
    const dt = Math.min(0.25, (now - this.last) / 1000);
    this.last = now;
    if (this.playing) {
      const list = placed();
      const T = total(list);
      let c = clipAt(list, this.time);
      if (!c) { this.pause(); this.time = T; this.emit(true); return; }
      if (this.active !== c.id) this.activate(c, this.time);
      const v = this.video(c);
      if (v && v.readyState >= 2 && !v.seeking && !v.paused) {
        this.time = c.start + (v.currentTime - c.in) / c.speed;
        if (v.currentTime >= c.out - 0.01 || v.ended) this.time = c.end;
      } else this.time += dt;
      if (this.time >= c.end - 1e-4) {
        const next = list[list.indexOf(c) + 1];
        if (!next) { this.time = T; this.pause(); return; }
        this.time = next.start;
        c = next;
        this.activate(next, next.start);
      }
      this.dirty = true;
      this.emit();
    }
    if (this.dirty) this.draw();
  }

  draw() {
    const cv = this.canvas;
    if (!cv) return;
    this.dirty = false;
    const ctx = cv.getContext("2d");
    if (!ctx) return;
    const list = placed();
    const c = clipAt(list, this.time) || (this.time > 0 && list.length && this.time >= total(list) - 0.05 ? list[list.length - 1] : null);
    const v = this.video(c);
    const m = c ? media.get(c.media)?.data : null;
    const pic = v && v.readyState >= 2 ? { image: v, w: v.videoWidth, h: v.videoHeight } : null;
    const miss = c && !v ? String(m?.name || "这段视频") : "";
    if (miss !== this.missing) { this.missing = miss; this.emit(true); }
    drawFrame(ctx, cv.width, cv.height, pic, overlayList(), this.time, { background: String(project.get("main")?.data.background || "#000000") });
  }

  size() { return frameSize(); }
}

export const player = new Player();
kit.subscribe(() => player.redraw());
