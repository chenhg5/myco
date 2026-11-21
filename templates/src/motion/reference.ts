import { t as uiText } from "../_lib/i18n";
// A reference video taken apart for the agent, here on this computer: the
// frames where its shots change (and enough between them to see the
// pacing), and what is said in it. The agent reads them from the page's
// data (reference, ref_frames) and rebuilds the video shot by shot.
import { decode, wav16k } from "./audio";
import { transcribe, speechState } from "./speech";
import type { OSSegment } from "../_lib/myco";

export type RefFrameOut = { time: number; data: string; text: string };
export type Taken = { name: string; duration: number; width: number; height: number; frames: RefFrameOut[]; transcript: string; heard: "yes" | "no-speech" | "silent" | "skipped"; why: string };
export type Stage = { step: "frames" | "sound" | "speech"; done: number };

const MAX_FRAMES = 20;

const once = (el: HTMLMediaElement, ev: string, ms = 8000) => new Promise<boolean>((resolve) => {
  const done = (ok: boolean) => { el.removeEventListener(ev, yes); el.removeEventListener("error", no); clearTimeout(t); resolve(ok); };
  const yes = () => done(true), no = () => done(false);
  const t = setTimeout(() => done(false), ms);
  el.addEventListener(ev, yes); el.addEventListener("error", no);
});
async function seek(v: HTMLVideoElement, t: number) {
  if (Math.abs(v.currentTime - t) < 0.005 && v.readyState >= 2) return true;
  v.currentTime = t;
  return once(v, "seeked");
}

/** Which sample times start a new shot: the change from the sample before,
 *  above what is usual for this video. */
export function cutsOf(diffs: number[], times: number[]) {
  if (diffs.length < 3) return [0];
  const sorted = [...diffs].sort((a, b) => a - b);
  const med = sorted[Math.floor(sorted.length / 2)];
  const mad = sorted.map((d) => Math.abs(d - med)).sort((a, b) => a - b)[Math.floor(sorted.length / 2)];
  const limit = Math.max(0.09, med + 6 * mad);
  const out = [0];
  for (let i = 1; i < diffs.length; i++) if (diffs[i] > limit && times[i] - times[out[out.length - 1]] >= 0.6) out.push(i);
  return out;
}

export async function takeApart(file: File, onStage: (s: Stage) => void, signal: AbortSignal): Promise<Taken> {
  const url = URL.createObjectURL(file);
  const v = document.createElement("video");
  v.muted = true; v.playsInline = true; v.preload = "auto"; v.src = url;
  try {
    if (!(await once(v, "loadeddata", 15000))) throw new Error(uiText("这个视频打不开（换成 mp4 试试）"));
    const D = v.duration;
    if (!Number.isFinite(D) || D <= 0) throw new Error(uiText("这个视频打不开（换成 mp4 试试）"));
    const W = v.videoWidth, H = v.videoHeight;
    // 1. a small grey picture every step, and how much each differs from the last
    const step = Math.max(0.2, D / 480);
    const sw = 32, sh = 18;
    const small = document.createElement("canvas");
    small.width = sw; small.height = sh;
    const sg = small.getContext("2d", { willReadFrequently: true })!;
    const times: number[] = [], diffs: number[] = [];
    let prev: Float32Array | null = null;
    for (let t = 0; t < D - 0.05; t += step) {
      if (signal.aborted) throw new Error("cancelled");
      if (!(await seek(v, t))) continue;
      sg.drawImage(v, 0, 0, sw, sh);
      const px = sg.getImageData(0, 0, sw, sh).data;
      const g = new Float32Array(sw * sh);
      for (let i = 0; i < g.length; i++) g[i] = (px[i * 4] * 0.3 + px[i * 4 + 1] * 0.59 + px[i * 4 + 2] * 0.11) / 255;
      let d = 0;
      if (prev) { for (let i = 0; i < g.length; i++) d += Math.abs(g[i] - prev[i]); d /= g.length; }
      times.push(t); diffs.push(d); prev = g;
      onStage({ step: "frames", done: (t / D) * 0.7 });
    }
    // 2. the shots, the strongest changes when there are too many, and
    // more in long shots so the pacing shows
    let cuts = cutsOf(diffs, times);
    if (cuts.length > MAX_FRAMES) cuts = [0, ...cuts.slice(1).sort((a, b) => diffs[b] - diffs[a]).slice(0, MAX_FRAMES - 1)].sort((a, b) => a - b);
    const starts = cuts.map((i) => times[i]);
    let picks: number[] = starts.map((s, i) => {
      const end = i + 1 < starts.length ? starts[i + 1] : D;
      return Math.min(s + Math.min(0.5, (end - s) * 0.35), D - 0.05);
    });
    while (picks.length < Math.min(MAX_FRAMES, Math.max(6, Math.ceil(D / 4)))) {
      const all = [0, ...picks, D].sort((a, b) => a - b);
      let gap = 0, at = 0;
      for (let i = 1; i < all.length; i++) if (all[i] - all[i - 1] > gap) { gap = all[i] - all[i - 1]; at = (all[i] + all[i - 1]) / 2; }
      if (gap < 2) break;
      picks.push(at);
    }
    picks = picks.sort((a, b) => a - b);
    // 3. each picked moment as a JPEG small enough for one record
    const big = document.createElement("canvas");
    const k = Math.min(1, 640 / Math.max(W, H));
    big.width = Math.max(2, Math.round(W * k)); big.height = Math.max(2, Math.round(H * k));
    const bg = big.getContext("2d")!;
    const frames: RefFrameOut[] = [];
    for (let i = 0; i < picks.length; i++) {
      if (signal.aborted) throw new Error("cancelled");
      if (!(await seek(v, picks[i]))) continue;
      bg.drawImage(v, 0, 0, big.width, big.height);
      let data = "";
      for (const q of [0.75, 0.6, 0.45]) { data = big.toDataURL("image/jpeg", q); if (data.length < 56000) break; }
      frames.push({ time: Math.round(picks[i] * 100) / 100, data, text: "" });
      onStage({ step: "frames", done: 0.7 + (0.3 * (i + 1)) / picks.length });
    }
    // 4. what is said
    let transcript = "", heard: Taken["heard"] = "skipped", why = "";
    const sp = await speechState();
    if (!sp.ready) why = sp.can ? uiText("本机还没有语音识别（在「声音」里下载后再试）") : sp.why;
    else {
      onStage({ step: "sound", done: 0 });
      let sound: AudioBuffer | null = null;
      try { sound = await decode(file); } catch { heard = "silent"; }
      if (sound) {
        onStage({ step: "speech", done: 0 });
        const segs: OSSegment[] = await transcribe(wav16k(sound), (p) => onStage({ step: "speech", done: p }), signal);
        heard = segs.length ? "yes" : "no-speech";
        transcript = segs.map((s) => `[${s.start.toFixed(1)}–${s.end.toFixed(1)}] ${s.text.trim()}`).join("\n");
        for (const f of frames) {
          f.text = segs.filter((s) => s.end > f.time - 1.5 && s.start < f.time + 1.5).map((s) => s.text.trim()).join(" ").slice(0, 300);
        }
      }
    }
    return { name: file.name, duration: Math.round(D * 100) / 100, width: W, height: H, frames, transcript: transcript.slice(0, 40000), heard, why };
  } finally {
    v.removeAttribute("src"); v.load();
    URL.revokeObjectURL(url);
  }
}
