// Pages rendered for agents (page.export, see internal/console/page_render.go):
// while the App runs, this asks its backend for the next export, opens that
// page in a hidden, sandboxed frame with the page's read-only data, calls
// the app's export handler (pagebridge.js myco.actions) and sends the file
// back. The frame gets nothing else: no writes, no messages, no computer.
import { useEffect } from "react";
import { appApi, type PageRenderJob, type PageRenderResult } from "../appapi";
import { localizedPageURL } from "./page-preferences";

const READY_MS = 15_000; // for the page to load and register its actions
const RUN_MS = 25_000;   // for the app to make the file

type Fail = { code: string; message: string; retryable?: boolean };

function toBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

// render opens the page for one export and returns its file. It stops at
// once (the frame cancelled and removed) when the Hub says the job may no
// longer run, or when the App's window closes (signal).
async function render(j: PageRenderJob, signal: AbortSignal): Promise<PageRenderResult> {
  const opened = await appApi.openPage(j.hub_id, j.page_id);
  const lang = document.documentElement.lang.startsWith("zh") ? "zh" : "en";
  const src = localizedPageURL(opened.url, lang);
  const origin = new URL(src).origin;
  const instance = Array.from(crypto.getRandomValues(new Uint8Array(16)), (n) => n.toString(16).padStart(2, "0")).join("");
  const callID = `x${instance.slice(0, 8)}`;
  const frame = document.createElement("iframe");
  // Its own origin (as in the panel), scripts; no forms, popups, downloads.
  frame.setAttribute("sandbox", "allow-scripts allow-same-origin");
  frame.setAttribute("aria-hidden", "true");
  frame.tabIndex = -1;
  Object.assign(frame.style, { position: "fixed", left: "-20000px", top: "0", width: "1280px", height: "800px", border: "0", opacity: "0", pointerEvents: "none" });
  let onMessage: ((e: MessageEvent) => void) | null = null;
  const watch = new AbortController();
  let stopRef: (() => void) | null = null;
  try {
    const result = await new Promise<PageRenderResult>((resolve, reject) => {
      const stopNow = () => {
        frame.contentWindow?.postMessage({ myco: "actions.cancel", instance, id: callID }, origin);
        frame.remove();
        reject({ code: "cancelled", message: "stopped (revoked, cancelled or the App closed)" } as Fail);
      };
      stopRef = stopNow;
      if (signal.aborted) { stopNow(); return; }
      signal.addEventListener("abort", stopNow, { once: true });
      // The Hub's word, through the backend: stop as soon as it may not run.
      void (async () => {
        while (!watch.signal.aborted) {
          try {
            const w = await appApi.pageRenderWait(j.id, watch.signal);
            if (w.stop) { stopNow(); return; }
          } catch {
            if (!watch.signal.aborted) stopNow();
            return;
          }
        }
      })();
      let registered = false;
      const readyTimer = window.setTimeout(() => reject({ code: "unsupported", message: "this page's app does not export files (or is an older version)" } as Fail), READY_MS);
      let runTimer = 0;
      const post = (msg: Record<string, unknown>) => frame.contentWindow?.postMessage(msg, origin);
      onMessage = (e: MessageEvent) => {
        if (e.source !== frame.contentWindow || e.origin !== origin) return;
        const d = e.data as Record<string, unknown> | null;
        if (!d || typeof d !== "object") return;
        if (d.myco === "call" && typeof d.id === "number") {
          const reply = (value: unknown, error?: string, code?: string) => post(error ? { myco: "result", id: d.id, error, code } : { myco: "result", id: d.id, value });
          const args = (d.args || {}) as Record<string, unknown>;
          const coll = String(args.collection || "");
          const answer = async (): Promise<unknown> => {
            switch (d.method) {
              case "ready": {
                const st = await appApi.pageRenderState(j.id);
                return { page: { id: j.page_id, name: opened.name, version: opened.version }, me: { name: "" }, state: st.state, version: st.version,
                  preferences: { language: lang, theme: "light" }, render: { instance } };
              }
              case "data.list": return appApi.pageRenderData(j.id, coll);
              case "data.get": return appApi.pageRenderData(j.id, coll, String(args.id || ""));
              case "data.watch": return true;
              case "storage.get": return null;
            }
            throw Object.assign(new Error("not while exporting for an agent"), { code: "unavailable" });
          };
          void answer().then((v) => reply(v), (err: { message?: string; code?: string }) => reply(null, err.message || "error", err.code || "unavailable"));
          return;
        }
        if (d.instance !== instance) return;
        if (d.myco === "actions.registered" && !registered) {
          registered = true;
          window.clearTimeout(readyTimer);
          const list = Array.isArray(d.actions) ? (d.actions as { name: string; version: number }[]) : [];
          if (!list.some((a) => a.name === j.action && a.version === j.version)) {
            reject({ code: "unsupported", message: `this page's app does not offer ${j.action} v${j.version}` } as Fail);
            return;
          }
          runTimer = window.setTimeout(() => {
            post({ myco: "actions.cancel", instance, id: callID });
            reject({ code: "timeout", message: "the page's app took too long to make the file", retryable: true } as Fail);
          }, RUN_MS);
          post({ myco: "actions.invoke", instance, id: callID, action: j.action, version: j.version, args: j.args || {} });
          return;
        }
        if (d.myco === "actions.result" && d.id === callID) {
          window.clearTimeout(runTimer);
          const err = d.error as Fail | undefined;
          if (err) { reject({ code: String(err.code || "export_failed"), message: String(err.message || "export failed") } as Fail); return; }
          const r = d.result as { media_type?: string; data?: ArrayBuffer; width?: number; height?: number; pages?: number } | undefined;
          if (!r || typeof r.media_type !== "string" || !(r.data instanceof ArrayBuffer)) { reject({ code: "export_failed", message: "the page's app returned no file" } as Fail); return; }
          if (r.data.byteLength > 4 << 20) { reject({ code: "too_large", message: "the file is over 4 MB" } as Fail); return; }
          resolve({ media_type: r.media_type, data: toBase64(r.data), width: r.width, height: r.height, pages: r.pages });
        }
      };
      window.addEventListener("message", onMessage);
      frame.src = src;
      document.body.appendChild(frame);
    });
    return result;
  } catch (e) {
    const f = e as Partial<Fail> & { message?: string };
    return { error: { code: f.code || "export_failed", message: f.message || "export failed", retryable: !!f.retryable } };
  } finally {
    watch.abort();
    if (stopRef) signal.removeEventListener("abort", stopRef);
    if (onMessage) window.removeEventListener("message", onMessage);
    frame.remove();
  }
}

/** PageRenderer runs for as long as the App's window is open. */
export function PageRenderer() {
  useEffect(() => {
    const stop = new AbortController();
    let alive = true;
    void (async () => {
      while (alive) {
        try {
          const j = await appApi.pageRenderNext(stop.signal);
          if (!alive) break;
          if (!j) continue;
          const r = await render(j, stop.signal);
          await appApi.pageRenderResult(j.id, r).catch(() => undefined);
        } catch {
          if (!alive) break;
          // (an older backend without page rendering, or a hiccup)
          await new Promise((r) => window.setTimeout(r, 30_000));
        }
      }
    })();
    return () => { alive = false; stop.abort(); };
  }, []);
  return null;
}
