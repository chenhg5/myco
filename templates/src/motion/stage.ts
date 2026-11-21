// The page's side of a scene frame (frame.js): a sandboxed iframe that
// shows the video at any time, or draws one frame into a bitmap. One is
// the preview people see; a hidden one draws thumbnails and the export.
import frameSource from "./frame.js?raw";

type Doc = ReturnType<typeof import("./model").frameDoc>;
export type Loaded = { errors: Record<string, string>; total: number };

const SRC = `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;height:100%;overflow:hidden;background:transparent}
#view{position:absolute;left:0;top:0;transform-origin:0 0;box-shadow:0 0 0 1px rgba(127,127,127,.18),0 10px 40px rgba(0,0,0,.22)}
</style></head><body><div id="view"><div id="stage"></div></div><script>${frameSource}</script></body></html>`;

export class SceneFrame {
  readonly el: HTMLIFrameElement;
  private ready: Promise<void>;
  private seq = 0;
  private waits = new Map<number, (v: Loaded) => void>();
  private frames = new Map<number, { ok: (b: ImageBitmap) => void; fail: (e: Error) => void }>();
  private loading: Promise<Loaded> | null = null;
  private wanted: Doc | null = null;
  private docKey = "";
  onPick: (scene: string, field: string) => void = () => undefined;
  onError: (scene: string, message: string) => void = () => undefined;
  last: Loaded = { errors: {}, total: 0 };

  constructor(parent: HTMLElement, hidden = false) {
    const f = document.createElement("iframe");
    // scripts only: an origin of its own, no access to this page
    f.setAttribute("sandbox", "allow-scripts");
    f.setAttribute("title", "scene");
    f.className = hidden ? "mo-frame is-hidden" : "mo-frame";
    if (hidden) f.setAttribute("aria-hidden", "true");
    f.srcdoc = SRC;
    this.el = f;
    let resolve: () => void = () => undefined;
    this.ready = new Promise((r) => { resolve = r; });
    window.addEventListener("message", (e) => {
      if (e.source !== f.contentWindow) return;
      const m = e.data || {};
      if (m.t === "ready") resolve();
      else if (m.t === "loaded") { this.last = { errors: m.errors || {}, total: m.total || 0 }; this.waits.get(m.seq)?.(this.last); this.waits.delete(m.seq); }
      else if (m.t === "frame") {
        const w = this.frames.get(m.id);
        this.frames.delete(m.id);
        if (!w) { m.bitmap?.close?.(); return; }
        if (m.bitmap) w.ok(m.bitmap); else w.fail(new Error(m.error || "draw failed"));
      } else if (m.t === "pick") this.onPick(String(m.scene || ""), String(m.field || ""));
      else if (m.t === "error") this.onError(String(m.scene || ""), String(m.message || ""));
    });
    parent.appendChild(f);
  }
  private send(m: unknown) { this.el.contentWindow?.postMessage(m, "*"); }

  /** Shows this video (again only when it changed); the newest wins. */
  async load(doc: Doc, force = false): Promise<Loaded> {
    const key = JSON.stringify(doc);
    if (!force && key === this.docKey) return this.loading || this.last;
    this.docKey = key;
    this.wanted = doc;
    await this.ready;
    if (this.wanted !== doc) return this.loading || this.last;
    const seq = ++this.seq;
    const p = new Promise<Loaded>((r) => this.waits.set(seq, r));
    this.send({ t: "load", seq, doc });
    this.loading = p;
    const r = await p;
    if (this.loading === p) this.loading = null;
    return r;
  }
  async seek(time: number) { await this.ready; this.send({ t: "seek", time }); }
  /** One frame at `time`, w × h pixels. */
  async capture(time: number, w: number, h: number): Promise<ImageBitmap> {
    await this.ready;
    if (this.loading) await this.loading;
    const id = ++this.seq;
    return new Promise((ok, fail) => {
      this.frames.set(id, { ok, fail });
      this.send({ t: "capture", id, time, w, h });
    });
  }
  remove() { this.el.remove(); }
}
