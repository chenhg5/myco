import { useSyncExternalStore } from "react";

/*
 * The way into a voice call (VoiceCall), for the round button of a box you
 * type in: with nothing typed it starts a call, as in ChatGPT. AppMode says
 * whether this App can make calls (the Mac app) and how to open one (or
 * Settings › 语音 until they are set up).
 */

export type VoiceEntry = { ready: boolean; open: () => void };

let entry: VoiceEntry | null = null;
const listeners = new Set<() => void>();

export function setVoiceEntry(e: VoiceEntry | null) {
  entry = e;
  listeners.forEach((l) => l());
}

export function useVoiceEntry(): VoiceEntry | null {
  return useSyncExternalStore(
    (cb) => { listeners.add(cb); return () => { listeners.delete(cb); }; },
    () => entry,
    () => entry,
  );
}
