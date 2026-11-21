// HLS transport for the sandbox: hls.js demuxes into MediaSource while every
// playlist, key and segment travels through myco.os.net.fetch. Disposal ignores
// in-flight bridge replies and releases the previous stream and blob URLs.
import Hls, {
  type Loader,
  type LoaderContext,
  type LoaderCallbacks,
  type LoaderConfiguration,
  type LoaderStats,
} from "hls.js";
import { fromBase64 } from "../_lib/os";
import { t as uiText } from "../_lib/i18n";
import { request } from "./network";
import { httpURL } from "./catalog";
export class BridgeLoader implements Loader<LoaderContext> {
  context: LoaderContext | null = null;
  stats: LoaderStats = {
    aborted: false,
    loaded: 0,
    total: 0,
    retry: 0,
    chunkCount: 0,
    bwEstimate: 0,
    loading: { start: 0, first: 0, end: 0 },
    parsing: { start: 0, end: 0 },
    buffering: { start: 0, first: 0, end: 0 },
  };
  private timer = 0;
  private generation = 0;
  private headers: Record<string, string> = {};
  private completed = true;
  private callbacks: LoaderCallbacks<LoaderContext> | null = null;
  abort() {
    this.generation++;
    window.clearTimeout(this.timer);
    if (!this.completed) {
      this.completed = true;
      this.stats.aborted = true;
      if (this.context)
        this.callbacks?.onAbort?.(this.stats, this.context, null);
    }
  }
  destroy() {
    this.abort();
    this.context = null;
    this.callbacks = null;
  }
  getResponseHeader = (name: string) =>
    this.headers[name.toLowerCase()] || null;
  load(
    context: LoaderContext,
    config: LoaderConfiguration,
    callbacks: LoaderCallbacks<LoaderContext>,
  ) {
    this.abort();
    const n = this.generation;
    this.context = context;
    this.callbacks = callbacks;
    this.completed = false;
    this.stats.aborted = false;
    this.stats.loading.start = performance.now();
    const headers = { ...context.headers },
      start = context.rangeStart || 0,
      end = context.rangeEnd;
    if (end !== undefined && end > start)
      headers.Range = `bytes=${start}-${end - 1}`;
    this.timer = window.setTimeout(
      () => {
        if (n !== this.generation) return;
        this.completed = true;
        this.generation++;
        callbacks.onTimeout(this.stats, context, null);
      },
      Math.min(
        25000,
        config.loadPolicy?.maxLoadTimeMs || config.timeout || 25000,
      ),
    );
    void request(
      context.url,
      context.responseType === "arraybuffer",
      headers,
      32 * 1024 * 1024,
    )
      .then((r) => {
        if (n !== this.generation) return;
        window.clearTimeout(this.timer);
        this.headers = r.headers;
        const binary = context.responseType === "arraybuffer";
        let data: string | ArrayBuffer = binary
          ? (fromBase64(r.body).buffer as ArrayBuffer)
          : r.body;
        if (binary && end !== undefined && end > start && r.status === 200)
          data = (data as ArrayBuffer).slice(start, end);
        const size =
          typeof data === "string"
            ? new TextEncoder().encode(data).length
            : data.byteLength;
        this.stats.loading.first = this.stats.loading.end = performance.now();
        this.stats.loaded = this.stats.total = size;
        this.completed = true;
        callbacks.onProgress?.(this.stats, context, data, null);
        callbacks.onSuccess(
          { url: httpURL(r.url) || context.url, data, code: r.status },
          this.stats,
          context,
          null,
        );
      })
      .catch(() => {
        if (n !== this.generation) return;
        window.clearTimeout(this.timer);
        this.completed = true;
        callbacks.onError(
          { code: 0, text: uiText("播放线路连接失败，请重试或切换线路") },
          context,
          null,
          this.stats,
        );
      });
  }
}
export function playStream(
  video: HTMLVideoElement,
  address: string,
  onError: (text: string) => void,
): () => void {
  let disposed = false,
    blob = "",
    hls: Hls | undefined;
  const url = httpURL(address);
  const fail = (message: string) => {
    if (!disposed) onError(message);
  };
  if (!url) {
    fail(uiText("这个播放地址不受支持"));
    return () => {};
  }
  if (/\.(mp4|webm)$/i.test(new URL(url).pathname)) {
    // Small progressive files only; films should use a segmented HLS source.
    void request(url, true, {}, 32 * 1024 * 1024)
      .then((r) => {
        if (disposed) return;
        blob = URL.createObjectURL(
          new Blob([fromBase64(r.body)], {
            type: /\.webm$/i.test(new URL(url).pathname)
              ? "video/webm"
              : "video/mp4",
          }),
        );
        video.src = blob;
      })
      .catch(() =>
        fail(uiText("这个文件无法载入或超过 32 MB，请选择 HLS 播放线路")),
      );
  } else if (Hls.isSupported()) {
    hls = new Hls({
      loader: BridgeLoader,
      enableWorker: false,
      maxBufferLength: 30,
      backBufferLength: 30,
      maxMaxBufferLength: 60,
      capLevelToPlayerSize: true,
      contentSteeringController: undefined,
    });
    hls.on(Hls.Events.ERROR, (_e, data) => {
      if (data.fatal) {
        hls?.stopLoad();
        fail(uiText("播放线路连接失败，请重试或切换线路"));
      }
    });
    hls.loadSource(url);
    hls.attachMedia(video);
  } else
    fail(uiText("当前 App 不支持分段视频播放，请更新 App 或换到桌面端打开"));
  return () => {
    disposed = true;
    hls?.destroy();
    video.pause();
    video.removeAttribute("src");
    video.load();
    if (blob) URL.revokeObjectURL(blob);
  };
}
