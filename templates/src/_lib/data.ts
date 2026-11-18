// A table's data, as both 表格 and 多维表格 keep it: one record with the
// title and the fields (columns), and one record a row with its values by
// field id and an order. Changes are made here at once and saved behind.
import { Store, listen, myco, type MycoChange } from "./myco";
import { assignOrders } from "./order";
import { uid, type Field } from "./fields";

export type Cfg = {
  meta: string;              // the collection with the one record "main"
  fieldsKey: "columns" | "fields";
  rows: string;              // one record a row
  valuesKey: "cells" | "values";
  name: string;              // 表格, 多维表格
  rowWord: string;           // 行, 记录
  defaults: Field[];
};
export type Row = { id: string; values: Record<string, unknown>; order: number };
type Rec = Record<string, unknown>;

const empty = (v: unknown) => v === undefined || v === null || v === "" || (Array.isArray(v) && !v.length);

export class Sheet {
  readonly meta: Store<Rec>;
  readonly rows: Store<Rec>;
  me = "";
  page = "";
  pending = 0;
  failed = false;
  note: { text: string; undo?: () => void } | null = null;
  version = 0;
  private subs = new Set<() => void>();
  private noteTimer = 0;
  private titleTimer = 0;
  private draftTitle: string | null = null;
  constructor(readonly cfg: Cfg) {
    this.meta = new Store<Rec>(cfg.meta);
    this.rows = new Store<Rec>(cfg.rows);
  }
  async start() {
    const ctx = await myco!.ready;
    this.me = ctx.me?.name || "";
    this.page = ctx.page?.id || "";
    await Promise.all([this.meta.load(), this.rows.load()]);
    if (!this.meta.get("main")) await this.meta.put({ title: "", [this.cfg.fieldsKey]: this.cfg.defaults }, "main")?.catch(() => this.meta.load());
    listen([this.meta, this.rows], (c: MycoChange) => {
      this.emit();
      if (c.by && c.by !== this.me) this.flash(`${c.by} 刚改了`);
    });
  }
  subscribe = (f: () => void) => { this.subs.add(f); return () => { this.subs.delete(f); }; };
  emit() { this.version++; for (const f of this.subs) f(); }
  flash(text: string, undo?: () => void) {
    this.note = { text, undo };
    window.clearTimeout(this.noteTimer);
    this.noteTimer = window.setTimeout(() => { this.note = null; this.emit(); }, undo ? 6000 : 2600);
    this.emit();
  }
  private track<T>(p: Promise<T> | null | undefined) {
    if (!p) return;
    this.pending++;
    this.emit();
    p.then(() => { this.pending--; this.failed = false; this.emit(); }, (e: { code?: string; message?: string }) => {
      this.pending--;
      this.failed = e?.code !== "conflict";
      this.flash(e?.code === "conflict" ? "别人刚改过，已显示最新的" : e?.message || "没保存上");
    });
  }

  // ---- reading ----
  get title(): string { return this.draftTitle ?? String(this.meta.get("main")?.data.title ?? ""); }
  get fields(): Field[] {
    const fs = this.meta.get("main")?.data[this.cfg.fieldsKey];
    return Array.isArray(fs) ? (fs as Field[]).filter((f) => f && f.id) : this.cfg.defaults;
  }
  get list(): Row[] {
    return this.rows.rows.map((r) => ({ id: r.id, values: ((r.data[this.cfg.valuesKey] as Rec) || {}), order: typeof r.data.order === "number" ? r.data.order : 0 }));
  }
  row(id: string): Row | null { return this.list.find((r) => r.id === id) || null; }
  /** Everyone named in person fields, and me. */
  people(): string[] {
    const ps = new Set<string>();
    if (this.me) ps.add(this.me);
    const fs = this.fields.filter((f) => f.type === "person");
    for (const r of this.list) for (const f of fs) { const v = r.values[f.id]; if (typeof v === "string" && v.trim()) ps.add(v.trim()); }
    return [...ps];
  }

  // ---- writing ----
  setTitle(t: string) {
    this.draftTitle = t;
    this.emit();
    window.clearTimeout(this.titleTimer);
    this.titleTimer = window.setTimeout(() => {
      const cur = this.meta.get("main")?.data || {};
      this.track(this.meta.put({ ...cur, title: t.trim(), [this.cfg.fieldsKey]: this.fields }, "main")?.finally(() => { this.draftTitle = null; }));
    }, 500);
  }
  saveFields(fields: Field[]) {
    const cur = this.meta.get("main")?.data || {};
    this.track(this.meta.put({ ...cur, title: this.title.trim(), [this.cfg.fieldsKey]: fields }, "main"));
    this.emit();
  }
  /** Values changed in rows (by field id); new options are added to their fields. */
  edit(changes: { id: string; values: Rec }[]) {
    const byId = new Map<string, Rec>();
    for (const c of changes) byId.set(c.id, { ...(byId.get(c.id) || {}), ...c.values });
    const fields = this.fields, grow = new Map<string, Set<string>>();
    for (const [id, patch] of byId) {
      const cur = this.rows.get(id);
      if (!cur) continue;
      const values: Rec = { ...((cur.data[this.cfg.valuesKey] as Rec) || {}) };
      for (const k of Object.keys(patch)) {
        const v = patch[k];
        if (empty(v)) delete values[k]; else values[k] = v;
        const f = fields.find((x) => x.id === k);
        if (f && (f.type === "select" || f.type === "multi") && !empty(v)) {
          for (const o of Array.isArray(v) ? v : [v]) if (!(f.options || []).includes(String(o))) {
            if (!grow.has(f.id)) grow.set(f.id, new Set());
            grow.get(f.id)!.add(String(o));
          }
        }
      }
      this.track(this.rows.put({ ...cur.data, [this.cfg.valuesKey]: values }, id));
    }
    if (grow.size) this.saveFields(fields.map((f) => grow.has(f.id) ? { ...f, options: [...(f.options || []), ...grow.get(f.id)!] } : f));
    this.emit();
  }
  /** A new row (before or after another, or at the end); its id. */
  add(values: Rec = {}, where?: { before?: string; after?: string }): string {
    const list = this.list;
    let order = (list.length ? list[list.length - 1].order : 0) + 1;
    const at = where?.before ? list.findIndex((r) => r.id === where.before) : where?.after ? list.findIndex((r) => r.id === where.after) + 1 : -1;
    if (at >= 0 && at < list.length) order = at === 0 ? list[0].order - 1 : (list[at - 1].order + list[at].order) / 2;
    const id = uid("r");
    const clean: Rec = {};
    for (const k of Object.keys(values)) if (!empty(values[k])) clean[k] = values[k];
    this.track(this.rows.put({ [this.cfg.valuesKey]: clean, order }, id));
    this.emit();
    return id;
  }
  remove(ids: string[]) {
    const gone = ids.map((id) => this.rows.get(id)).filter(Boolean).map((r) => ({ id: r!.id, data: r!.data }));
    for (const g of gone) this.track(this.rows.remove(g.id));
    this.flash(`删除了 ${gone.length} ${this.cfg.rowWord === "行" ? "行" : "条" + this.cfg.rowWord}`, () => {
      for (const g of gone) this.track(this.rows.put(g.data, g.id));
      this.note = null;
      this.emit();
    });
  }
  /** Rows shown in this order now (ids); only those whose place changed are written. */
  reorder(ids: string[]) {
    const have = new Map(this.list.map((r) => [r.id, r.order]));
    const orders = assignOrders(ids, have);
    ids.forEach((id, i) => {
      if (have.get(id) === orders[i]) return;
      const cur = this.rows.get(id);
      if (cur) this.track(this.rows.put({ ...cur.data, order: orders[i] }, id));
    });
    this.emit();
  }
  /** Moves one row to sit between two others (ids or "" at an end). */
  place(id: string, before: string | null, after: string | null, values?: Rec) {
    const a = after ? this.row(after)?.order : undefined, b = before ? this.row(before)?.order : undefined;
    const order = a !== undefined && b !== undefined ? (a + b) / 2 : a !== undefined ? a + 1 : b !== undefined ? b - 1 : this.row(id)?.order ?? 0;
    const cur = this.rows.get(id);
    if (!cur) return;
    const next: Rec = { ...cur.data, order };
    if (values) {
      const vs: Rec = { ...((cur.data[this.cfg.valuesKey] as Rec) || {}) };
      for (const k of Object.keys(values)) if (empty(values[k])) delete vs[k]; else vs[k] = values[k];
      next[this.cfg.valuesKey] = vs;
    }
    this.track(this.rows.put(next, id));
    this.emit();
  }
}
