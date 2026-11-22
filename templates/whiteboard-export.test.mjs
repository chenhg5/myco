// Derived-content correctness: stale snapshots, delete/recreate, bounded records,
// concurrent producers and disposal must not create misleading fresh exports.
import assert from "node:assert/strict";
import { actionModule } from "./action-test-modules.mjs";
const { sharedExport, sharedExportLimit, sourceFingerprint, sharedExportState, startSharedExport } = await import(await actionModule("whiteboard/shared-export"));
const row = (collection, id, data, version = 1) => ({ collection, id, data, version });
const initial = { board: [row("board", "main", { title: "Design review" })], items: [row("items", "note", { type: "sticky", text: "中文 <script>&\"", x: 40, y: 40 })] };
const fp = await sourceFingerprint(initial), rendered = await sharedExport(initial);
assert.equal(sharedExportState(rendered, fp), "ready");
for (const entity of ["&lt;", "&gt;", "&amp;", "&quot;"]) assert(rendered.svg.includes(entity));
assert(!rendered.svg.includes("<script>"));
const controls = structuredClone(initial);
controls.items[0].data.text = "Hi\u0001\uffff\ud800😀";
const clean = await sharedExport(controls);
assert(!/[\u0001\uffff\ud800-\udfff]/u.test(clean.svg));
assert(clean.svg.includes("😀"), "valid surrogate pairs remain intact");
assert.equal(rendered.width, 376);
assert.equal(rendered.height, 324);
assert(Buffer.byteLength(JSON.stringify(rendered)) <= sharedExportLimit);
assert.equal(sharedExportState(undefined, fp), "stale");
const changed = structuredClone(initial);
changed.items[0].data.text = "Changed but recreated at version one";
assert.notEqual(await sourceFingerprint(changed), fp, "delete/recreate cannot pass freshness with the same ID/version");
assert.equal(sharedExportState(rendered, await sourceFingerprint(changed)), "stale");
const reordered = structuredClone(initial);
reordered.items[0].data = { y: 40, x: 40, text: "中文 <script>&\"", type: "sticky" };
assert.equal(await sourceFingerprint(reordered), fp, "object key order does not make identical content stale");
const deleted = { ...initial, items: [] };
assert.equal(sharedExportState(rendered, await sourceFingerprint(deleted)), "stale");
const huge = { ...initial, items: Array.from({ length: 257 }, (_, i) => row("items", String(i), { text: "Note" })) };
const tooLarge = await sharedExport(huge);
assert.equal(tooLarge.status, "too_large");
assert.equal(tooLarge.svg, undefined);
assert.equal(sharedExportState(tooLarge, tooLarge.source_fingerprint), "too_large");
const long = structuredClone(initial);
long.items[0].data.text = "长".repeat(16_001);
assert.equal((await sharedExport(long)).status, "too_large");
long.items[0].data.text = "x".repeat(12_000);
assert.equal((await sharedExport(long)).status, "too_large", "the encoded SVG must fit even when source text is within the work limit");

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(test) {
  const end = Date.now() + 3000;
  while (!test()) { if (Date.now() > end) throw new Error("publisher did not settle"); await sleep(5); }
}
function fixture(input = initial) {
  const state = structuredClone({ ...input, exports: [] });
  const handlers = new Set(), writes = [];
  let conflict = false;
  const data = {
    get: async (c, id) => {
      const record = state[c].find(r => r.id === id);
      if (!record) throw { code: "not_found" };
      return structuredClone(record);
    },
    put: async (c, value, opts) => {
      const old = state[c].find(r => r.id === opts.id);
      if (conflict) { conflict = false; throw { code: "conflict" }; }
      if ((old?.version ?? 0) !== opts.ifVersion) throw { code: "conflict" };
      const next = row(c, opts.id, structuredClone(value), (old?.version ?? 0) + 1);
      state[c] = [next]; writes.push(next);
      return next;
    },
  };
  const source = () => structuredClone({ board: state.board, items: state.items });
  const subscribe = h => { handlers.add(h); return () => handlers.delete(h); };
  return { state, handlers, writes, data, source, subscribe, conflict: () => { conflict = true; } };
}
const f = fixture();
const start = f => startSharedExport(f.data, f.source, f.subscribe, 0);
const stop1 = start(f), stop2 = start(f);
await until(() => f.writes.length === 1);
await sleep(35);
assert.equal(f.writes.length, 1, "concurrent open pages converge without feedback writes");
f.conflict();
f.state.items[0].data.text = "Agent update"; f.state.items[0].version++;
for (const h of f.handlers) h({ collection: "items" });
await until(() => f.writes.length === 2);
assert(f.writes[1].data.svg.replace(/<[^>]+>/g, "").includes("Agent update"));
stop1(); stop2();
assert.equal(f.handlers.size, 0);

const superseded = fixture();
const normalGet = superseded.data.get;
let once = true;
superseded.data.get = async (c, id) => {
  if (once) {
    once = false;
    superseded.state.items[0].data.text = "Newest content";
    superseded.state.items[0].version++;
  }
  return normalGet(c, id);
};
const stop3 = start(superseded);
await until(() => superseded.writes.length === 1);
assert(superseded.writes[0].data.svg.replace(/<[^>]+>/g, "").includes("Newest content"), "never publish a render superseded before the CAS");
stop3();
const disposed = fixture();
let unblock;
disposed.data.get = () => new Promise(resolve => { unblock = resolve; });
const stop4 = start(disposed);
await until(() => !!unblock);
stop4(); unblock(null);
await sleep(20);
assert.equal(disposed.writes.length, 0);
const unsaved = fixture();
unsaved.source = () => null;
const stop5 = start(unsaved);
await sleep(30);
assert.equal(unsaved.writes.length, 0, "pending/failed optimistic edits are not shared as committed exports");
stop5();
console.log("PASS shared SVG provenance, freshness, delete/recreate, bounded data, CAS conflict recovery, producer convergence, superseded snapshots and disposal");
