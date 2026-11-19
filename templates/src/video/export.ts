// Makes the mp4 here in the browser: each frame drawn as the player draws
// it, encoded with WebCodecs (mediabunny); the sound of every clip mixed at
// its speed (time-stretched, so voices keep their pitch) and volume.
import {
  ALL_FORMATS, AudioBufferSink, AudioBufferSource, BlobSource, BufferTarget, CanvasSink, CanvasSource, Input, Mp4OutputFormat, Output,
  QUALITY_HIGH, getFirstEncodableAudioCodec, getFirstEncodableVideoCodec,
} from "mediabunny";
import { FPS, frameSize, media, overlayList, placed, project, total, type Placed } from "./model";
import { drawFrame, fit, type Picture } from "./render";
import { localFor } from "./media";

export type Progress = { phase: "audio" | "video" | "finish"; done: number };
const RATE = 48000;

export async function render(onProgress: (p: Progress) => void, cancelled: () => boolean): Promise<{ bytes: Uint8Array; silent: boolean }> {
  const list = placed();
  if (!list.length) throw new Error("时间线上还没有视频");
  const files = new Map<string, Blob>();
  for (const c of list) {
    const m = media.get(c.media)?.data;
    const l = localFor(String(m?.key || ""));
    if (!l) throw new Error(`本机没有「${m?.name || "一段视频"}」，先在素材里选上这个文件`);
    files.set(c.id, l.file);
  }
  const T = total(list);
  const { w: W, h: H } = frameSize(list);
  const vcodec = await getFirstEncodableVideoCodec(["avc", "hevc", "vp9", "av1"], { width: W, height: H });
  if (!vcodec) throw new Error("这个浏览器不能编码视频（需要 WebCodecs）");
  const loud = list.some((c) => !c.muted && c.volume > 0 && media.get(c.media)?.data.audio !== false);
  let acodec = loud ? await getFirstEncodableAudioCodec(["aac", "opus"], { numberOfChannels: 2, sampleRate: RATE }) : null;
  if (loud && !acodec) {
    // (no AAC or Opus encoder in this browser, as in some versions of
    // Safari: FFmpeg's AAC encoder, built to WebAssembly, loaded only now)
    try {
      const { registerAacEncoder } = await import("@mediabunny/aac-encoder");
      registerAacEncoder();
      acodec = await getFirstEncodableAudioCodec(["aac"], { numberOfChannels: 2, sampleRate: RATE });
    } catch { /* (then the mp4 has no sound; said below) */ }
  }
  const silent = loud && !acodec;

  // the sound first, all of it: it is then added a second at a time
  let mix: AudioBuffer | null = null;
  if (acodec) mix = await mixAudio(list, files, T, (d) => onProgress({ phase: "audio", done: d }), cancelled);
  if (cancelled()) throw new Error("cancelled");

  const canvas = document.createElement("canvas");
  canvas.width = W; canvas.height = H;
  const ctx = canvas.getContext("2d", { willReadFrequently: false })!;
  const output = new Output({ format: new Mp4OutputFormat({ fastStart: "in-memory" }), target: new BufferTarget() });
  const vsrc = new CanvasSource(canvas, { codec: vcodec, bitrate: QUALITY_HIGH, keyFrameInterval: 2 });
  output.addVideoTrack(vsrc, { frameRate: FPS });
  const asrc = mix && acodec ? new AudioBufferSource({ codec: acodec, bitrate: QUALITY_HIGH }) : null;
  if (asrc) output.addAudioTrack(asrc);
  await output.start();

  const over = overlayList();
  const background = String(project.get("main")?.data.background || "#000000");
  const frames = Math.max(1, Math.ceil(T * FPS - 1e-6));
  let frame = 0, audioUpTo = 0;
  const feedAudio = async (upTo: number) => {
    if (!asrc || !mix) return;
    while (audioUpTo < Math.min(upTo, T)) {
      const a = Math.round(audioUpTo * RATE), b = Math.min(mix.length, Math.round(Math.min(T, audioUpTo + 1) * RATE));
      if (b <= a) break;
      const chunk = new AudioBuffer({ length: b - a, numberOfChannels: 2, sampleRate: RATE });
      for (let ch = 0; ch < 2; ch++) chunk.getChannelData(ch).set(mix.getChannelData(ch).subarray(a, b));
      await asrc.add(chunk);
      audioUpTo = b / RATE;
    }
  };
  try {
    for (const c of list) {
      const k0 = Math.ceil(c.start * FPS - 1e-6), k1 = Math.min(frames, Math.ceil(c.end * FPS - 1e-6));
      if (k1 <= k0) continue;
      const input = new Input({ source: new BlobSource(files.get(c.id)!), formats: ALL_FORMATS });
      try {
        const vt = await input.getPrimaryVideoTrack();
        let sink: CanvasSink | null = null, sw = 0, sh = 0;
        if (vt && (await vt.canDecode())) {
          const r = fit(vt.displayWidth, vt.displayHeight, W, H);
          sw = Math.max(2, Math.round(r.w)); sh = Math.max(2, Math.round(r.h));
          sink = new CanvasSink(vt, { width: sw, height: sh, fit: "fill", poolSize: 2 });
        }
        const times = function* () { for (let k = k0; k < k1; k++) yield c.in + (k / FPS - c.start) * c.speed; };
        let last: Picture = null;
        const draw = async (k: number, pic: Picture) => {
          drawFrame(ctx, W, H, pic, over, k / FPS, { background });
          await vsrc.add(k / FPS, 1 / FPS);
          frame++;
          if (frame % 5 === 0) onProgress({ phase: "video", done: frame / frames });
          await feedAudio(k / FPS + 1);
        };
        if (sink) {
          let k = k0;
          for await (const wc of sink.canvasesAtTimestamps(times())) {
            if (cancelled()) throw new Error("cancelled");
            if (wc) last = { image: wc.canvas as CanvasImageSource, w: sw, h: sh };
            await draw(k++, last);
          }
          while (k < k1) await draw(k++, last);
        } else {
          for (let k = k0; k < k1; k++) { if (cancelled()) throw new Error("cancelled"); await draw(k, null); }
        }
      } finally { input.dispose(); }
    }
    await feedAudio(T + 1);
    onProgress({ phase: "finish", done: 1 });
    await output.finalize();
  } catch (e) {
    await output.cancel().catch(() => undefined);
    throw e;
  }
  return { bytes: new Uint8Array((output.target as BufferTarget).buffer!), silent };
}

async function mixAudio(list: Placed[], files: Map<string, Blob>, T: number, progress: (d: number) => void, cancelled: () => boolean) {
  const ctx = new OfflineAudioContext(2, Math.max(1, Math.ceil(T * RATE)), RATE);
  let i = 0;
  for (const c of list) {
    progress(i++ / list.length);
    if (cancelled()) throw new Error("cancelled");
    if (c.muted || c.volume <= 0) continue;
    const input = new Input({ source: new BlobSource(files.get(c.id)!), formats: ALL_FORMATS });
    try {
      const at = await input.getPrimaryAudioTrack();
      if (!at || !(await at.canDecode())) continue;
      const parts: { t: number; b: AudioBuffer }[] = [];
      for await (const w of new AudioBufferSink(at).buffers(c.in, c.out)) parts.push({ t: w.timestamp, b: w.buffer });
      if (!parts.length) continue;
      const rate = parts[0].b.sampleRate;
      const n = Math.max(1, Math.round((c.out - c.in) * rate));
      const chans = [new Float32Array(n), new Float32Array(n)];
      for (const p of parts) {
        const off = Math.round((p.t - c.in) * rate);
        for (let ch = 0; ch < 2; ch++) {
          const src = p.b.getChannelData(Math.min(ch, p.b.numberOfChannels - 1));
          const from = Math.max(0, -off), to = Math.min(src.length, n - off);
          if (to > from) chans[ch].set(src.subarray(from, to), off + from);
        }
      }
      const out = Math.abs(c.speed - 1) < 0.02 ? chans : stretch(chans, c.speed, rate);
      const buf = new AudioBuffer({ length: out[0].length, numberOfChannels: 2, sampleRate: rate });
      buf.getChannelData(0).set(out[0]); buf.getChannelData(1).set(out[1]);
      const node = ctx.createBufferSource();
      node.buffer = buf;
      const g = ctx.createGain();
      g.gain.value = c.volume;
      node.connect(g).connect(ctx.destination);
      node.start(c.start);
      node.stop(c.end);
    } finally { input.dispose(); }
  }
  progress(1);
  return ctx.startRendering();
}

/** Changes the length of sound by 1/speed keeping its pitch (WSOLA: pieces
 *  of it laid one after another where they join most smoothly). */
export function stretch(chans: Float32Array[], speed: number, rate: number): Float32Array[] {
  const N = Math.round(rate * 0.04) & ~1, Hs = N / 2, Ha = Hs * speed, tol = Math.round(rate * 0.008);
  const n = chans[0].length, outLen = Math.max(1, Math.floor(n / speed));
  const win = new Float32Array(N);
  for (let i = 0; i < N; i++) win[i] = 0.5 - 0.5 * Math.cos((2 * Math.PI * i) / N);
  const mono = new Float32Array(n);
  for (let i = 0; i < n; i++) mono[i] = (chans[0][i] + chans[1][i]) * 0.5;
  const out = chans.map(() => new Float32Array(outLen + N));
  const norm = new Float32Array(outLen + N);
  let prev = 0; // where the last piece was read from
  for (let m = 0; m * Hs < outLen; m++) {
    const nominal = Math.round(m * Ha);
    let best = nominal;
    if (m > 0) {
      // the piece that would carry on the last one most naturally
      const nat = prev + Hs;
      let bestScore = -Infinity;
      for (let off = -tol; off <= tol; off += 4) {
        const p = nominal + off;
        if (p < 0 || p + N > n || nat + N > n) continue;
        let s = 0;
        for (let i = 0; i < N; i += 4) s += mono[p + i] * mono[nat + i];
        if (s > bestScore) { bestScore = s; best = p; }
      }
    }
    best = Math.max(0, Math.min(n - 1, best));
    const o = m * Hs;
    for (let i = 0; i < N && best + i < n; i++) {
      const w = win[i];
      for (let ch = 0; ch < out.length; ch++) out[ch][o + i] += chans[ch][best + i] * w;
      norm[o + i] += w;
    }
    prev = best;
  }
  return out.map((x) => {
    const y = new Float32Array(outLen);
    for (let i = 0; i < outLen; i++) y[i] = norm[i] > 1e-3 ? x[i] / norm[i] : x[i];
    return y;
  });
}
