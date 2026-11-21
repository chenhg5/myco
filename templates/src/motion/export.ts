// Makes the mp4 here in the page: the hidden scene frame draws each frame
// (frame.js), WebCodecs encodes it (mediabunny), as 视频剪辑 does.
import { AudioBufferSource, BufferTarget, Mp4OutputFormat, Output, StreamTarget, VideoSample, VideoSampleSource,
  QUALITY_HIGH, QUALITY_LOW, QUALITY_MEDIUM, QUALITY_VERY_HIGH, getFirstEncodableAudioCodec, getFirstEncodableVideoCodec } from "mediabunny";
import { MIX_RATE, mix, slices } from "./audio";
import { t as uiText } from "../_lib/i18n";
import { fpsOf, frameDoc, sizeOf, total } from "./model";
import type { SceneFrame } from "./stage";

export type ExportOptions = { short: number; fps: number; quality: "best" | "high" | "medium" | "low" };
export const QUALITIES: [ExportOptions["quality"], string][] = [["best", uiText("最高")], ["high", uiText("高")], ["medium", uiText("标准")], ["low", uiText("小文件")]];
const QUALITY = { best: QUALITY_VERY_HIGH, high: QUALITY_HIGH, medium: QUALITY_MEDIUM, low: QUALITY_LOW };
const BPP = { best: 0.12, high: 0.07, medium: 0.04, low: 0.02 };

export function outSize(short: number) {
  const { w, h } = sizeOf();
  const k = short / Math.min(w, h);
  const even = (v: number) => Math.max(2, Math.round(v / 2) * 2);
  return { w: even(w * k), h: even(h * k) };
}
export function estimate(o: ExportOptions) {
  const { w, h } = outSize(o.short);
  return { w, h, mb: (w * h * o.fps * BPP[o.quality] * total()) / 8e6 };
}
export const defaultOptions = (): ExportOptions => ({ short: 1080, fps: fpsOf(), quality: "high" });

export type Rendered = { file: Blob; silent: boolean; missing: string[] };

/** Its AAC (or Opus) encoder; FFmpeg's AAC, built to WebAssembly, where
 *  the browser has none (some versions of Safari). */
async function audioCodec() {
  let c = await getFirstEncodableAudioCodec(["aac", "opus"], { numberOfChannels: 2, sampleRate: MIX_RATE });
  if (!c) {
    try {
      const { registerAacEncoder } = await import("@mediabunny/aac-encoder");
      registerAacEncoder();
      c = await getFirstEncodableAudioCodec(["aac"], { numberOfChannels: 2, sampleRate: MIX_RATE });
    } catch { /* (then the mp4 has no sound; said after) */ }
  }
  return c;
}

export async function render(frame: SceneFrame, opts: ExportOptions, onProgress: (done: number) => void, cancelled: () => boolean): Promise<Rendered> {
  const T = total();
  if (T <= 0) throw new Error(uiText("还没有场景"));
  const { w: W, h: H } = outSize(opts.short);
  const codec = await getFirstEncodableVideoCodec(["avc", "hevc", "vp9", "av1"], { width: W, height: H });
  if (!codec) throw new Error(uiText("这个浏览器不能编码视频（需要 WebCodecs）"));
  const r = await frame.load(frameDoc("capture"), true);
  const bad = Object.keys(r.errors);
  if (bad.length) throw new Error(uiText("有场景的动画代码出错了，先修好再导出：{0}", r.errors[bad[0]]));

  // the sound first: every track mixed over the video's length
  const sound = await mix(T);
  const acodec = sound.buffer ? await audioCodec() : null;
  const silent = !!sound.buffer && !acodec;

  const canvas = document.createElement("canvas");
  canvas.width = W; canvas.height = H;
  const ctx = canvas.getContext("2d")!;
  const disk = await diskFile();
  const output = disk
    ? new Output({ format: new Mp4OutputFormat({ fastStart: false }), target: new StreamTarget(disk.writable, { chunked: true, chunkSize: 4 << 20 }) })
    : new Output({ format: new Mp4OutputFormat({ fastStart: "in-memory" }), target: new BufferTarget() });
  const src = new VideoSampleSource({ codec, bitrate: QUALITY[opts.quality], keyFrameInterval: 2 });
  output.addVideoTrack(src, { frameRate: opts.fps });
  const asrc = sound.buffer && acodec ? new AudioBufferSource({ codec: acodec, bitrate: QUALITY_HIGH }) : null;
  if (asrc) output.addAudioTrack(asrc);
  await output.start();
  // (the sound a second at a time, along with the frames)
  const pieces = asrc && sound.buffer ? slices(sound.buffer, 1) : null;
  let heard = 0;
  const feed = async (upTo: number) => {
    if (!asrc || !pieces) return;
    while (heard < upTo) {
      const r = pieces.next();
      if (r.done) { heard = Infinity; break; }
      await asrc.add(r.value);
      heard += r.value.duration;
    }
  };
  const frames = Math.max(1, Math.round(T * opts.fps));
  try {
    // the next frame is drawn while the last is encoded
    let next = frame.capture(0, W, H);
    let pending: Promise<void> | null = null;
    for (let k = 0; k < frames; k++) {
      if (cancelled()) throw new Error("cancelled");
      const bmp = await next;
      if (k + 1 < frames) next = frame.capture((k + 1) / opts.fps, W, H);
      ctx.drawImage(bmp, 0, 0, W, H);
      bmp.close();
      const sample = new VideoSample(canvas, { timestamp: k / opts.fps, duration: 1 / opts.fps });
      if (pending) await pending;
      pending = src.add(sample).finally(() => sample.close());
      if (k % 5 === 0) onProgress(k / frames);
      if (k % opts.fps === 0) await feed(k / opts.fps + 1);
    }
    if (pending) await pending;
    await feed(Infinity);
    onProgress(1);
    await output.finalize();
  } catch (e) {
    await output.cancel().catch(() => undefined);
    disk?.drop();
    throw e;
  }
  const file = disk ? await disk.file() : new Blob([(output.target as BufferTarget).buffer!], { type: "video/mp4" });
  return { file, silent, missing: sound.missing };
}

/** A file in this page's private storage (OPFS) for the mp4; null where
 *  there is none. Exports left from before are cleared first. */
async function diskFile() {
  try {
    const root = await navigator.storage.getDirectory();
    const old: string[] = [];
    for await (const name of (root as unknown as { keys(): AsyncIterable<string> }).keys()) if (name.startsWith("motion-")) old.push(name);
    for (const n of old) await root.removeEntry(n).catch(() => undefined);
    const name = `motion-${Date.now()}.mp4`;
    const fh = await root.getFileHandle(name, { create: true });
    const writable = await (fh as unknown as { createWritable(): Promise<WritableStream> }).createWritable();
    return { writable, file: () => fh.getFile(), drop: () => { void root.removeEntry(name).catch(() => undefined); } };
  } catch { return null; }
}
