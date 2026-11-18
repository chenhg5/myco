// The myco bridge (internal/console/pagebridge.js), as the App puts it in a
// page: a page's records and the person's Downloads.
export type MycoRecord<T = Record<string, unknown>> = { collection: string; id: string; data: T; version: number; updated_by?: string };
export type MycoChange = { seq: number; collection: string; record: string; op: "put" | "delete"; data?: Record<string, unknown>; version: number; by?: string };
type Ctx = { page: { id: string; name: string }; me: { name: string } };

interface Bridge {
  ready: Promise<Ctx>;
  data: {
    list(collection?: string): Promise<{ records: MycoRecord[]; seq: number }>;
    put(collection: string, data: unknown, opts?: { id?: string; ifVersion?: number }): Promise<MycoRecord>;
    remove(collection: string, id: string, opts?: { ifVersion?: number }): Promise<boolean>;
    on(f: (c: MycoChange) => void): () => void;
  };
  download(name: string, content: string | Blob | ArrayBuffer | Uint8Array, type?: string): Promise<{ path: string }>;
}

export const myco = (window as unknown as { myco?: Bridge }).myco;

/** One collection of the page's records, kept here in order (data.order),
 *  with what others change applied as it comes. Writes to one record go
 *  one after another; while some wait, what is here is newer than the
 *  answers and the feed. */
export class Store<T extends Record<string, unknown> = Record<string, unknown>> {
  rows: MycoRecord<T>[] = [];
  private queues = new Map<string, Promise<unknown>>();
  private waiting = new Map<string, number>();
  private listeners: ((c: MycoChange) => void)[] = [];
  constructor(readonly collection: string) {}

  private sort() {
    this.rows.sort((a, b) => {
      const x = a.data.order, y = b.data.order;
      return typeof x === "number" && typeof y === "number" ? x - y : 0;
    });
  }
  get(id: string) { return this.rows.find((r) => r.id === id) || null; }
  async load() {
    const r = await myco!.data.list(this.collection);
    this.rows = r.records as MycoRecord<T>[];
    this.sort();
    return this.rows;
  }
  private upsert(rec: MycoRecord) {
    const cur = this.get(rec.id);
    if (cur) { if (cur.version <= rec.version) { cur.data = rec.data as T; cur.version = rec.version; } }
    else this.rows.push({ collection: this.collection, id: rec.id, data: rec.data as T, version: rec.version });
    this.sort();
  }
  pending(id: string) { return (this.waiting.get(id) || 0) > 0; }
  /** Writes a record (all of it) based on the version here; id "" for a
   *  new one. The record is changed here at once. */
  put(data: T, id = ""): Promise<MycoRecord<T> | null> {
    if (id) {
      const cur = this.get(id);
      if (cur) cur.data = data;
      else { this.rows.push({ collection: this.collection, id, data, version: 0 }); this.sort(); }
    }
    const run = async () => {
      const cur = id ? this.get(id) : null;
      try {
        const rec = await myco!.data.put(this.collection, data, { id, ifVersion: cur ? cur.version : undefined });
        if (id) this.waiting.set(id, (this.waiting.get(id) || 1) - 1);
        const now = this.get(rec.id);
        if (id && this.pending(id) && now) now.version = Math.max(now.version, rec.version);
        else this.upsert(rec);
        return this.get(rec.id);
      } catch (e) {
        if (id) this.waiting.set(id, (this.waiting.get(id) || 1) - 1);
        const err = e as { code?: string; latest?: MycoRecord };
        if (err.code === "conflict" && err.latest) {
          this.upsert(err.latest);
          this.fire({ seq: 0, collection: this.collection, record: err.latest.id, op: "put", data: err.latest.data, version: err.latest.version });
        }
        throw e;
      }
    };
    if (!id) return run();
    this.waiting.set(id, (this.waiting.get(id) || 0) + 1);
    const p = (this.queues.get(id) || Promise.resolve()).then(run, run);
    this.queues.set(id, p.catch(() => undefined));
    return p;
  }
  async remove(id: string) {
    const i = this.rows.findIndex((r) => r.id === id);
    if (i >= 0) this.rows.splice(i, 1);
    await (this.queues.get(id) || Promise.resolve());
    await myco!.data.remove(this.collection, id).catch((e: { code?: string }) => { if (e.code !== "not_found") throw e; });
  }
  on(f: (c: MycoChange) => void) { this.listeners.push(f); }
  private fire(c: MycoChange) { for (const f of this.listeners) f(c); }
  /** A change from the feed; false when this page has it already. */
  apply(c: MycoChange): boolean {
    if (c.collection !== this.collection) return false;
    const cur = this.get(c.record);
    if (c.op === "delete") {
      if (!cur) return false;
      this.rows.splice(this.rows.indexOf(cur), 1);
    } else {
      if (this.pending(c.record)) return false;
      if (cur && cur.version >= c.version) return false;
      this.upsert({ collection: c.collection, id: c.record, data: c.data || {}, version: c.version });
    }
    this.fire(c);
    return true;
  }
}

/** Listens once: each store takes what is its own. */
export function listen(stores: Store<any>[], onAny?: (c: MycoChange) => void) {
  myco!.data.on((c) => {
    let news = false;
    for (const s of stores) if (s.apply(c)) news = true;
    if (news && onAny) onAny(c);
  });
}

/** Saves a file to the person's Downloads. */
export async function save(name: string, content: string | Uint8Array | Blob, type: string) {
  return myco!.download(name, content, type);
}

export function dark() {
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
}
