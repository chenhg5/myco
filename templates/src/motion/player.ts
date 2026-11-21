// The preview's clock: plays, pauses and seeks the scene frame, and keeps
// the music and voice with it.
import { useEffect, useState } from "react";
import { clamp, total } from "./model";
import { syncSound, stopSound } from "./audio";
import type { SceneFrame } from "./stage";

export const player = {
  time: 0, playing: false, frame: null as SceneFrame | null,
  subs: new Set<() => void>(),
  emit() { for (const f of this.subs) f(); },
  seek(t: number) {
    this.time = clamp(t, 0, total());
    void this.frame?.seek(this.time);
    if (this.playing) syncSound(this.time, true, total(), true);
    this.emit();
  },
  play() {
    if (total() <= 0) return;
    if (this.time >= total() - 0.02) this.time = 0;
    this.playing = true; this.emit();
    syncSound(this.time, true, total(), true);
    let last = performance.now();
    const step = (now: number) => {
      if (!this.playing) return;
      this.time = Math.min(total(), this.time + (now - last) / 1000);
      last = now;
      void this.frame?.seek(this.time);
      if (this.time >= total()) { this.playing = false; stopSound(); }
      else syncSound(this.time, true, total());
      this.emit();
      if (this.playing) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  },
  pause() { this.playing = false; stopSound(); this.emit(); },
  toggle() { if (this.playing) this.pause(); else this.play(); },
};

export function usePlayer() {
  const [, set] = useState(0);
  useEffect(() => { const f = () => set((n) => n + 1); player.subs.add(f); return () => { player.subs.delete(f); }; }, []);
  return player;
}
