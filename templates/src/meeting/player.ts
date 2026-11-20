// Playing a meeting: its tracks (me, them) as <audio> elements kept in step,
// from the recordings on this computer (myco.os.mediaURL). <audio> seeks in
// the WAV and plays faster without raising the voice.
import { os } from "../_lib/os";

export class Player {
  private els: HTMLAudioElement[];
  private subs = new Set<() => void>();
  private raf = 0;
  playing = false;
  rate = 1;
  /** Skip what nobody says: gaps longer than this many seconds (0: off). */
  skip = 0;
  private speech: [number, number][] = [];
  time = 0;
  duration = 0;

  constructor(paths: string[]) {
    this.els = paths.map((p) => {
      const a = new Audio();
      a.preload = "metadata";
      a.src = os().mediaURL(p);
      a.addEventListener("loadedmetadata", () => { this.duration = Math.max(this.duration, Number.isFinite(a.duration) ? a.duration : 0); this.emit(); });
      a.addEventListener("ended", () => { if (this.els.every((e) => e.ended || e.paused)) { this.playing = false; this.stopLoop(); this.emit(); } });
      return a;
    });
  }
  subscribe(f: () => void) { this.subs.add(f); return () => { this.subs.delete(f); }; }
  private emit() { for (const f of this.subs) f(); }
  /** The longest track leads; the others follow it. */
  private get lead() { return this.els.reduce((a, b) => ((b.duration || 0) > (a.duration || 0) ? b : a), this.els[0]); }

  play() {
    if (!this.els.length) return;
    const t = this.time;
    for (const a of this.els) {
      a.playbackRate = this.rate;
      if (Math.abs(a.currentTime - t) > 0.05) a.currentTime = Math.min(t, a.duration || t);
      if (!a.duration || t < a.duration) void a.play().catch(() => undefined);
    }
    this.playing = true;
    this.loop();
    this.emit();
  }
  pause() {
    for (const a of this.els) a.pause();
    this.playing = false;
    this.stopLoop();
    this.emit();
  }
  seek(t: number) {
    t = Math.max(0, Math.min(t, this.duration || t));
    this.time = t;
    for (const a of this.els) a.currentTime = Math.min(t, a.duration || t);
    if (this.playing) for (const a of this.els) if (a.paused && (!a.duration || t < a.duration)) void a.play().catch(() => undefined);
    this.emit();
  }
  /** When people speak (sorted), for skipping the silence between. */
  setSpeech(spans: [number, number][]) { this.speech = spans; }
  setSkip(on: boolean) { this.skip = on ? 1.5 : 0; this.emit(); }
  setRate(r: number) {
    this.rate = r;
    for (const a of this.els) a.playbackRate = r;
    this.emit();
  }
  private loop() {
    this.stopLoop();
    const tick = () => {
      const lead = this.lead;
      if (lead) {
        this.time = lead.currentTime;
        if (this.skip && this.playing && this.speech.length) {
          // in a long silence: on to the next one who speaks
          const t = this.time;
          const inside = this.speech.some(([a, b]) => t >= a - 0.3 && t <= b + 0.3);
          if (!inside) {
            const next = this.speech.find(([a]) => a > t);
            if (next && next[0] - t > this.skip) this.seek(next[0] - 0.3);
          }
        }
        for (const a of this.els) {
          if (a !== lead && !a.ended && Math.abs(a.currentTime - lead.currentTime) > 0.12 && lead.currentTime < (a.duration || Infinity)) a.currentTime = lead.currentTime;
        }
      }
      this.emit();
      this.raf = requestAnimationFrame(tick);
    };
    this.raf = requestAnimationFrame(tick);
  }
  private stopLoop() { cancelAnimationFrame(this.raf); this.raf = 0; }
  destroy() {
    this.pause();
    for (const a of this.els) { a.removeAttribute("src"); a.load(); }
    this.subs.clear();
  }
}
