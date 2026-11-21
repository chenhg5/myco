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

/** Where the shots change: the difference between each sample and the one
 *  `win` samples before (so a fade over half a second counts as much as a
 *  cut), its peaks above what is usual for this video, at least `gap`
 *  seconds apart. Returns the sample indexes where a new shot has begun. */
export function cutsOf(sigs: Float32Array[], times: number[], win = 1, gap = 0.8, max = 20): { cuts: number[]; diffs: number[] } {
  const n = sigs.length;
  if (n < 3) return { cuts: [0], diffs: sigs.map(() => 0) };
  const dist = (a: Float32Array, b: Float32Array) => { let d = 0; for (let i = 0; i < a.length; i++) d += Math.abs(a[i] - b[i]); return d / a.length; };
  const diffs = sigs.map((s, i) => (i >= win ? dist(s, sigs[i - win]) : i > 0 ? dist(s, sigs[0]) : 0));
  const sorted = [...diffs].sort((a, b) => a - b);
  const med = sorted[Math.floor(n / 2)];
  const mad = sorted.map((d) => Math.abs(d - med)).sort((a, b) => a - b)[Math.floor(n / 2)];
  const limit = Math.max(0.03, med + 5 * mad);
  // the peaks, strongest first, each keeping others `gap` away
  const peaks = diffs.map((d, i) => ({ d, i })).filter((p) => p.d > limit && (p.i === 0 || p.d >= diffs[p.i - 1]) && (p.i === n - 1 || p.d >= diffs[p.i + 1]))
    .sort((a, b) => b.d - a.d);
  const out = [0];
  for (const p of peaks) if (out.length < max && out.every((j) => Math.abs(times[j] - times[p.i]) >= gap)) out.push(p.i);
  return { cuts: out.sort((a, b) => a - b).map((i) => (i === 0 ? 0 : Math.max(0, i - Math.floor(win / 2)))), diffs };
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
    const times: number[] = [], sigs: Float32Array[] = [];
    for (let t = 0; t < D - 0.05; t += step) {
      if (signal.aborted) throw new Error("cancelled");
      if (!(await seek(v, t))) continue;
      sg.drawImage(v, 0, 0, sw, sh);
      const px = sg.getImageData(0, 0, sw, sh).data;
      // (colour too: two scenes as dark as each other still differ in it)
      const g = new Float32Array(sw * sh * 3);
      for (let i = 0; i < sw * sh; i++) { g[i * 3] = px[i * 4] / 255; g[i * 3 + 1] = px[i * 4 + 1] / 255; g[i * 3 + 2] = px[i * 4 + 2] / 255; }
      times.push(t); sigs.push(g);
      onStage({ step: "frames", done: (t / D) * 0.7 });
    }
    // 2. the shots, the strongest changes when there are too many, and
    // more in long shots so the pacing shows
    const { cuts, diffs } = cutsOf(sigs, times, Math.max(1, Math.round(0.6 / step)), 0.8, MAX_FRAMES);
    // each shot's steadiest moment (the least change around it), later
    // ones preferred: motion graphics settle after they build
    let picks: number[] = cuts.map((c, k) => {
      const end = k + 1 < cuts.length ? cuts[k + 1] : times.length;
      let best = -1, bestScore = Infinity;
      for (let i = c; i < end; i++) {
        if (times[i] < times[c] + 0.3 && end - c > 3) continue;
        const score = (diffs[i] + (diffs[i + 1] ?? diffs[i])) / 2 - (i - c) * 1e-4;
        if (score <= bestScore) { bestScore = score; best = i; }
      }
      return Math.min(times[best >= 0 ? best : c], D - 0.05);
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
    else try {
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
    } catch (e) {
      // (the frames are kept: the lines are only missing)
      if (signal.aborted) throw new Error("cancelled");
      heard = "skipped"; why = (e as Error).message || uiText("没识别成");
    }
    return { name: file.name, duration: Math.round(D * 100) / 100, width: W, height: H, frames, transcript: transcript.slice(0, 40000), heard, why };
  } finally {
    v.removeAttribute("src"); v.load();
    URL.revokeObjectURL(url);
  }
}
