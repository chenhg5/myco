// What steers the boat: the keyboard (W/S or ↑/↓ for the throttle lever,
// A/D or ←/→ for the rudder, space to cut the engine) and the on-screen
// buttons on touch screens; both press the same keys here.

import type { BoatInput } from "./boat";

export type Key = "up" | "down" | "left" | "right" | "stop";

const KEYMAP: Record<string, Key> = {
  KeyW: "up", ArrowUp: "up", KeyS: "down", ArrowDown: "down",
  KeyA: "left", ArrowLeft: "left", KeyD: "right", ArrowRight: "right", Space: "stop",
};

export class Controls {
  private held = new Set<Key>();
  /** Keys pressed since the last look (a tap still counts once). */
  private tapped = new Set<Key>();
  onAny: () => void = () => {};

  press(k: Key, down: boolean) {
    if (down) { this.held.add(k); this.tapped.add(k); this.onAny(); } else this.held.delete(k);
  }
  release() { this.held.clear(); }

  /** Listens to the keyboard on a window; returns how to stop. */
  bind(win: Window) {
    const typing = (e: KeyboardEvent) => { const t = e.target as HTMLElement | null; return !!t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable); };
    const down = (e: KeyboardEvent) => {
      const k = KEYMAP[e.code];
      if (!k || typing(e) || e.metaKey || e.ctrlKey || e.altKey) return;
      e.preventDefault();
      this.press(k, true);
    };
    const up = (e: KeyboardEvent) => { const k = KEYMAP[e.code]; if (k) this.press(k, false); };
    const blur = () => this.release();
    win.addEventListener("keydown", down);
    win.addEventListener("keyup", up);
    win.addEventListener("blur", blur);
    return () => { win.removeEventListener("keydown", down); win.removeEventListener("keyup", up); win.removeEventListener("blur", blur); };
  }

  /** The boat's input now. */
  read(): BoatInput {
    const has = (k: Key) => this.held.has(k) || this.tapped.has(k);
    const out: BoatInput = {
      throttle: has("up") && !has("down") ? 1 : has("down") && !has("up") ? -1 : 0,
      steer: has("left") && !has("right") ? -1 : has("right") && !has("left") ? 1 : 0,
      stop: has("stop"),
    };
    this.tapped.clear();
    return out;
  }
}
