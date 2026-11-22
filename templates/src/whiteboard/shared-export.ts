// A projection of shared board content, not a command queue or desktop capture.
// Keep provenance beside the SVG: a cached export never proves current content.
import { svgDocument } from "./export";
import type { Item } from "./geometry";
import type { MycoRecord } from "../_lib/myco";

type Source = { board: MycoRecord[]; items: MycoRecord<Item>[] };
export type SharedExport = {
  schema: 1;
  source_fingerprint: string;
  source_records: number;
  generated_at: string;
  status: "ready" | "too_large";
  media_type: "image/svg+xml";
  svg?: string;
  width?: number;
  height?: number;
};
export type ExportData = {
  get(collection: string, id: string): Promise<MycoRecord>;
  put(collection: string, data: SharedExport, opts: { id: string; ifVersion: number }): Promise<unknown>;
};
const bytes = (s: string) => new TextEncoder().encode(s);
// The desktop Go transport HTML-escapes JSON strings before forwarding them.
// Account for that representation, not just the smaller browser JSON payload.
const recordBytes = (data: SharedExport) => bytes(JSON.stringify(data).replace(/[<>&\u2028\u2029]/g, c => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"))).length;
export const sharedExportLimit = 56 * 1024; // Leave room below the Hub's 64 KB record limit.

function canonical(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([key, v]) => [key, canonical(v)]));
  return value;
}
/** Include data as well as versions: deleting/recreating an ID can reset its
 * version. Sorting makes the fingerprint independent of JSON object key order. */
export async function sourceFingerprint(source: Source): Promise<string> {
  if (!globalThis.crypto?.subtle) throw Object.assign(new Error("Shared export needs Web Crypto"), { code: "unsupported" });
  type Revision = [string, string, number, unknown];
  const versions = [...source.board.map((r): Revision => ["board", r.id, r.version, canonical(r.data)]), ...source.items.map((r): Revision => ["items", r.id, r.version, canonical(r.data)])]
    .sort((a, b) => a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : a[1] < b[1] ? -1 : a[1] > b[1] ? 1 : 0);
  const digest = await crypto.subtle.digest("SHA-256", bytes(JSON.stringify(versions)));
  return Array.from(new Uint8Array(digest), b => b.toString(16).padStart(2, "0")).join("");
}

export async function sharedExport(source: Source, at = new Date().toISOString()): Promise<SharedExport> {
  const data: SharedExport = {
    schema: 1, source_fingerprint: await sourceFingerprint(source),
    source_records: source.board.length + source.items.length, generated_at: at,
    status: "too_large", media_type: "image/svg+xml",
  };
  // Bound work before connector routing and SVG text wrapping.
  if (source.items.length > 256 || source.items.reduce((n, r) => n + String(r.data.text || "").length, 0) > 16_000) return data;
  const svg = svgDocument(source.items);
  const size = /^<svg[^>]* width="([\d.]+)" height="([\d.]+)"/.exec(svg);
  const ready: SharedExport = { ...data, status: "ready", svg, width: Number(size?.[1]), height: Number(size?.[2]) };
  return recordBytes(ready) <= sharedExportLimit ? ready : data;
}

export function sharedExportState(record: SharedExport | undefined, fingerprint: string): "ready" | "stale" | "too_large" {
  if (record?.schema !== 1 || record.source_fingerprint !== fingerprint) return "stale";
  return record.status === "ready" && record.svg ? "ready" : "too_large";
}

/** Read only immutable snapshots of committed, synchronized editor records.
 * Reload after CAS conflicts; another open page may be publishing the same SVG.
 * Never list collections here: that would advance the App's shared watch cursor
 * past changes the editor has not consumed. Singleton get leaves it untouched. */
export function startSharedExport(data: ExportData, source: () => Source | null, subscribe: (handler: () => void) => () => void, delay = 500): () => void {
  let stopped = false, running = false, dirty = false, retries = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const schedule = () => {
    if (stopped) return;
    dirty = true;
    clearTimeout(timer);
    timer = setTimeout(() => void publish(), delay);
  };
  const publish = async () => {
    if (stopped || running) return;
    running = true;
    dirty = false;
    try {
      const before = source();
      if (!before) return; // Saving/failed edits must not be projected as committed.
      const next = await sharedExport(before);
      if (stopped) return;
      const existing = await data.get("exports", "main").catch((error: { code?: string }) => {
        if (error?.code === "not_found") return null;
        throw error;
      });
      const previous = existing?.data as SharedExport | undefined;
      if (previous?.source_fingerprint === next.source_fingerprint && previous.status === next.status && previous.svg === next.svg) { retries = 0; return; }
      // Do not publish work already superseded while reading/rendering.
      const current = source();
      if (!current) return;
      if (await sourceFingerprint(current) !== next.source_fingerprint) { dirty = true; return; }
      if (!stopped) await data.put("exports", next, { id: "main", ifVersion: existing?.version ?? 0 });
      retries = 0;
    } catch (error) {
      const code = (error as { code?: string })?.code;
      // Read-only clients cannot publish. A retry must reload, never overwrite
      // another producer with an old version or turn a failure into "ready".
      if (code === "conflict") dirty = true;
      else if (!stopped && !["permission_denied", "invalid_argument", "too_large", "unsupported"].includes(code || ""))
        timer = setTimeout(schedule, Math.min(30_000, 1000 * 2 ** Math.min(retries++, 5)));
    } finally {
      running = false;
      if (dirty && !stopped) schedule();
    }
  };
  const off = subscribe(schedule);
  schedule();
  return () => { stopped = true; clearTimeout(timer); off(); };
}
