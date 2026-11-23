// Native media streaming handles ranges and cancellation. Never fetch a full
// podcast through the bounded text/binary request bridge.
import { myco } from "../_lib/myco";
export function canStream() {
  return (
    !!myco?.features?.includes("net-media") &&
    typeof myco.media?.url === "function"
  );
}
export function mediaAddress(url: string) {
  if (!canStream()) throw new Error("streaming_unavailable");
  const u = new URL(url);
  if (!/^https?:$/.test(u.protocol) || u.username || u.password)
    throw new Error("invalid_audio");
  return myco!.media!.url(u.href);
}
export function releaseAudio(audio: HTMLAudioElement) {
  audio.pause();
  audio.removeAttribute("src");
  audio.load();
}
