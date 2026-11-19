// Makes the mp4 here in the browser: each frame drawn as the player draws
// it, encoded with WebCodecs (mediabunny); the sound of every clip mixed at
// its speed (time-stretched, so voices keep their pitch) and volume.
import {
  ALL_FORMATS, AudioBufferSink, AudioBufferSource, BlobSource, BufferTarget, Input, Mp4OutputFormat, Output, StreamTarget, VideoSample, VideoSampleSink, VideoSampleSource,
  QUALITY_HIGH, QUALITY_LOW, QUALITY_MEDIUM, QUALITY_VERY_HIGH, getFirstEncodableAudioCodec, getFirstEncodableVideoCodec,
} from "mediabunny";
import { frameSize, media, overlayList, placed, project, total, type Placed } from "./model";
import { drawFrame, type Picture } from "./render";
import { localFor } from "./media";

export type Progress = { phase: "audio" | "video" | "finish"; done: number };
/** What the person picks before exporting: the short side in pixels, frames
 *  a second, and quality (the bitrate). */
export type ExportOptions = { short: number; fps: number; quality: "best" | "high" | "medium" | "low" };
export const QUALITIES: [ExportOptions["quality"], string][] = [["best", "最高"], ["high", "高"], ["medium", "标准"], ["low", "小文件"]];
const QUALITY = { best: QUALITY_VERY_HIGH, high: QUALITY_HIGH, medium: QUALITY_MEDIUM, low: QUALITY_LOW };
/** Rough bits a pixel a frame of each quality (H.264), for the size shown before exporting. */
const BPP = { best: 0.16, high: 0.09, medium: 0.05, low: 0.025 };
export function estimate(o: ExportOptions, list = placed()) {
  const { w, h } = frameSize(list, o.short);
  const mbits = (w * h * o.fps * BPP[o.quality] * Math.pow(1080 * 1920 / (w * h), 0.25) + 160_000) * total(list) / 1e6;
  return { w, h, mb: mbits / 8 };
}
const RATE = 48000;

export type Rendered = { file: Blob; silent: boolean; drop: () => void };

export async function render(opts: ExportOptions, onProgress: (p: Progress) => void, cancelled: () => boolean): Promise<Rendered> {
  const FPS = opts.fps;
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
  const { w: W, h: H } = frameSize(list, opts.short);
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

  // The sound is mixed a few seconds at a time as the frames go (never all
  // of it at once: a long video's would take hundreds of MB).
  const sound = acodec ? mixAudio(list, files, T, cancelled) : null;
  onProgress({ phase: "audio", done: 1 });

  const canvas = document.createElement("canvas");
  canvas.width = W; canvas.height = H;
  const ctx = canvas.getContext("2d", { willReadFrequently: false })!;
  // The mp4 is written to this page's private storage on disk as it is
  // made (where the browser has one), not kept in memory.
  const disk = await diskFile();
  const output = disk
    ? new Output({ format: new Mp4OutputFormat({ fastStart: false }), target: new StreamTarget(disk.writable, { chunked: true, chunkSize: 4 << 20 }) })
    : new Output({ format: new Mp4OutputFormat({ fastStart: "in-memory" }), target: new BufferTarget() });
  const vsrc = new VideoSampleSource({ codec: vcodec, bitrate: QUALITY[opts.quality], keyFrameInterval: 2 });
  output.addVideoTrack(vsrc, { frameRate: FPS });
  const asrc = sound && acodec ? new AudioBufferSource({ codec: acodec, bitrate: QUALITY_HIGH }) : null;
  if (asrc) output.addAudioTrack(asrc);
  await output.start();

  const over = overlayList();
  const background = String(project.get("main")?.data.background || "#000000");
  const frames = Math.max(1, Math.ceil(T * FPS - 1e-6));
  let frame = 0, audioUpTo = 0;
  const feedAudio = async (upTo: number) => {
    if (!asrc || !sound) return;
    while (audioUpTo < Math.min(upTo, T) - 1e-6) {
      const r = await sound.next();
      if (r.done) { audioUpTo = T; break; }
      await asrc.add(r.value);
      audioUpTo += r.value.duration;
    }
  };
  // Decoding the next frame overlaps encoding the last: the frame drawn is
  // taken (a VideoSample of the canvas) before the next is drawn, and the
  // encoder is waited for only when it falls behind.
  let pending: Promise<void> | null = null;
  const emit = async (k: number, pic: Picture) => {
    drawFrame(ctx, W, H, pic, over, k / FPS, { background });
    const sample = new VideoSample(canvas, { timestamp: k / FPS, duration: 1 / FPS });
    if (pending) await pending;
    pending = vsrc.add(sample).finally(() => sample.close());
    frame++;
    if (frame % 5 === 0) onProgress({ phase: "video", done: frame / frames });
    if (frame % FPS === 0) await feedAudio(k / FPS + 1);
  };
  try {
    for (const c of list) {
      const k0 = Math.ceil(c.start * FPS - 1e-6), k1 = Math.min(frames, Math.ceil(c.end * FPS - 1e-6));
      if (k1 <= k0) continue;
      const input = new Input({ source: new BlobSource(files.get(c.id)!), formats: ALL_FORMATS });
      let last: VideoSample | null = null;
      try {
        const vt = await input.getPrimaryVideoTrack();
        const sink = vt && (await vt.canDecode()) ? new VideoSampleSink(vt) : null;
        const pic = (s: VideoSample | null): Picture => s ? {
          w: s.displayWidth, h: s.displayHeight,
          draw: (cx, x, y, w, h) => s.draw(cx, x, y, w, h),
        } : null;
        let k = k0;
        if (sink) {
          // the decoded frames in order; each output frame shows the last
          // one that has begun by its time
          const it = sink.samples(c.in, c.out + 1 / FPS)[Symbol.asyncIterator]();
          let next: VideoSample | null = (await it.next()).value || null;
          for (; k < k1; k++) {
            if (cancelled()) throw new Error("cancelled");
            const at = c.in + (k / FPS - c.start) * c.speed;
            while (next && next.timestamp <= at + 1e-4) {
              last?.close(); last = next;
              next = (await it.next()).value || null;
            }
            if (!last && next) { last = next; next = (await it.next()).value || null; }
            await emit(k, pic(last));
          }
          next?.close();
          await it.return?.(undefined);
        }
        while (k < k1) { if (cancelled()) throw new Error("cancelled"); await emit(k++, pic(last)); }
      } finally { last?.close(); input.dispose(); }
    }
    if (pending) await pending;
    await feedAudio(T + 1);
    onProgress({ phase: "finish", done: 1 });
    await output.finalize();
  } catch (e) {
    await output.cancel().catch(() => undefined);
    await sound?.return(undefined);
    disk?.drop();
    throw e;
  }
  if (disk) return { file: await disk.file(), silent, drop: disk.drop };
  return { file: new Blob([(output.target as BufferTarget).buffer!], { type: "video/mp4" }), silent, drop: () => undefined };
}

/** A file in this page's private storage (OPFS) to write the mp4 to; null
 *  where there is none. Exports left from before are cleared first. */
async function diskFile() {
  try {
    const root = await navigator.storage.getDirectory();
    const old: string[] = [];
    for await (const name of (root as unknown as { keys(): AsyncIterable<string> }).keys()) if (name.startsWith("export-")) old.push(name);
    for (const n of old) await root.removeEntry(n).catch(() => undefined);
    const name = `export-${Date.now()}.mp4`;
    const fh = await root.getFileHandle(name, { create: true });
    const writable = await (fh as unknown as { createWritable(): Promise<WritableStream> }).createWritable();
    return { writable, file: () => fh.getFile(), drop: () => { void root.removeEntry(name).catch(() => undefined); } };
  } catch { return null; }
}

/** The edit's sound, mixed at 48 kHz stereo, a piece (SEG seconds) at a
 *  time in timeline order: each clip's part of the piece decoded, time-
 *  stretched to its speed (with a little sound either side, so the pieces
 *  join), at its volume. */
async function* mixAudio(list: Placed[], files: Map<string, Blob>, T: number, cancelled: () => boolean): AsyncGenerator<AudioBuffer> {
  const SEG = 10, PAD = 0.2;
  const open = new Map<string, { input: Input; sink: AudioBufferSink | null }>();
  const sinkOf = async (c: Placed) => {
    let o = open.get(c.id);
    if (!o) {
      const input = new Input({ source: new BlobSource(files.get(c.id)!), formats: ALL_FORMATS });
      const at = await input.getPrimaryAudioTrack().catch(() => null);
      o = { input, sink: at && (await at.canDecode()) ? new AudioBufferSink(at) : null };
      open.set(c.id, o);
    }
    return o.sink;
  };
  try {
    for (let a = 0; a < T - 1e-6; a += SEG) {
      if (cancelled()) throw new Error("cancelled");
      const b = Math.min(T, a + SEG);
      const ctx = new OfflineAudioContext(2, Math.max(1, Math.round((b - a) * RATE)), RATE);
      for (const c of list) {
        if (c.end <= a || c.start >= b || c.muted || c.volume <= 0) continue;
        const sink = await sinkOf(c);
        if (!sink) continue;
        // the source sound this piece needs, with some either side
        const o0 = Math.max(a, c.start) - c.start, o1 = Math.min(b, c.end) - c.start;
        const s0 = Math.max(c.in, c.in + (o0 - PAD) * c.speed), s1 = Math.min(c.out, c.in + (o1 + PAD) * c.speed);
        if (s1 <= s0) continue;
        const parts: { t: number; b: AudioBuffer }[] = [];
        for await (const w of sink.buffers(s0, s1)) parts.push({ t: w.timestamp, b: w.buffer });
        if (!parts.length) continue;
        const rate = parts[0].b.sampleRate;
        const n = Math.max(1, Math.round((s1 - s0) * rate));
        const chans = [new Float32Array(n), new Float32Array(n)];
        for (const p of parts) {
          const off = Math.round((p.t - s0) * rate);
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
        // where the buffer's first sample falls on the timeline
        const at = c.start + (s0 - c.in) / c.speed;
        const from = Math.max(a, c.start), to = Math.min(b, c.end);
        node.start(from - a, Math.max(0, from - at));
        node.stop(to - a);
      }
      yield await ctx.startRendering();
    }
  } finally {
    for (const o of open.values()) o.input.dispose();
  }
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
