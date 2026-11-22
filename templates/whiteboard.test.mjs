// Geometry, clipboard isolation, safe exports and reversible multi-record edits.
import assert from "node:assert/strict";
import fs from "node:fs";
import ts from "typescript";
import { inlineI18n } from "./test-i18n.mjs";
const url = (s) =>
  "data:text/javascript;base64," + Buffer.from(s).toString("base64");
const compile = (name) =>
  inlineI18n(
    ts.transpile(
      fs.readFileSync(
        new URL("src/whiteboard/" + name, import.meta.url),
        "utf8",
      ),
      { target: 99, module: 99 },
    ),
  );
const geometryURL = url(compile("geometry.ts")),
  g = await import(geometryURL);
const records = [
  { id: "a", data: { type: "rect", text: "A", x: 0, y: 0, w: 200, h: 100 } },
  { id: "b", data: { type: "rect", text: "B", x: 400, y: 0, w: 200, h: 100 } },
  { id: "c", data: { type: "sticky", text: "C", x: 400, y: 300 } },
  { id: "ab", data: { type: "arrow", from: "a", to: "b", text: "Review" } },
  { id: "bc", data: { type: "arrow", from: "b", to: "c" } },
];
const bs = g.boxes(records);
assert.equal(bs.length, 3);
assert.equal(bs[0].color, "gray");
assert.equal(
  g.boxes([{ id: "bad", data: { type: "__proto__", x: NaN, w: Infinity } }])[0]
    .type,
  "sticky",
);
assert.deepEqual(
  g.copyGraph(records, ["a", "b"]).map((x) => x.id),
  ["a", "b", "ab"],
);
assert.deepEqual(
  g.copyGraph(records, ["ab"]).map((x) => x.id),
  ["a", "b", "ab"],
);
let id = 0;
const copied = g.cloneGraph(
  g.copyGraph(records, ["a", "b"]),
  () => `copy${++id}`,
  { x: 32, y: 40 },
);
assert.equal(copied[0].data.x, 32);
assert.equal(copied[2].data.from, copied[0].id);
assert.equal(copied[2].data.to, copied[1].id);
assert.equal(records[0].data.x, 0);
assert.equal(
  g.cloneGraph([records[3]], () => `bad${++id}`, { x: 0, y: 0 }).length,
  0,
);
const payload = g.clipboardPrefix + JSON.stringify(copied);
assert.deepEqual(g.parseClipboard(payload), copied);
assert.equal(g.parseClipboard("arbitrary text"), null);
assert.equal(g.parseClipboard(g.clipboardPrefix + "["), null);
assert.equal(
  g.parseClipboard(
    g.clipboardPrefix +
      JSON.stringify([{ id: "x", data: { type: "__proto__" } }]),
  ),
  null,
);
assert.equal(
  g.parseClipboard(
    g.clipboardPrefix + JSON.stringify([records[0], records[0]]),
  ),
  null,
);
for (const route of ["elbow", "curve", "straight"]) {
  const line = g.connector(bs[0], bs[1], { route });
  assert.deepEqual(line.a, { x: 200, y: 50 });
  assert.deepEqual(line.b, { x: 400, y: 50 });
  assert(!/NaN|Infinity/.test(line.path));
  assert.equal(line.path.includes("C"), route === "curve");
}
const pinned = g.connector(bs[0], bs[1], {
  from_side: "bottom",
  to_side: "top",
});
assert.deepEqual(pinned.a, { x: 100, y: 100 });
assert.deepEqual(pinned.b, { x: 500, y: 0 });
assert.deepEqual(g.nearPort(bs, { x: 401, y: 50 }, "a")?.side, "left");
const moved = g.connector({ ...bs[0], x: 20, y: 40 }, bs[1], {
  from_side: "right",
  to_side: "left",
});
assert.deepEqual(moved.a, { x: 220, y: 90 });
const svg = await import(
  url(compile("export.ts").replace('"./geometry"', JSON.stringify(geometryURL)))
);
const exported = svg.svgDocument([
  ...records,
  { id: "unsafe", data: { type: "text", text: '<script>&"', x: 0, y: 200 } },
]);
assert(exported.includes("&lt;script&gt;&amp;&quot;"));
assert(!exported.includes("<script>"));
assert(exported.includes("Review"));
assert(exported.includes("marker-end"));
// Exercise the actual edit module against a synchronous in-memory Store port.
let rows = structuredClone(records),
  sequence = 0;
globalThis.__wb = {
  items: {
    get: (id) => rows.find((r) => r.id === id),
    get rows() {
      return rows;
    },
    put: (data, id) => {
      const r = rows.find((r) => r.id === id);
      if (r) r.data = structuredClone(data);
      else rows.push({ id, data: structuredClone(data) });
      return Promise.resolve({ id, data });
    },
    remove: (id) => {
      rows = rows.filter((r) => r.id !== id);
      return Promise.resolve();
    },
  },
  kit: { track: (p) => Promise.resolve(p), emit: () => {}, flash: () => {} },
  currentBoxes: () => g.boxes(rows),
  maxZ: () => Math.max(0, ...rows.map((r) => r.data.z || 0)),
  uid: () => `new${++sequence}`,
};
const fakeModel = url(
    "export const {items,kit,currentBoxes,maxZ}=globalThis.__wb;",
  ),
  fakeKit = url("export const uid=globalThis.__wb.uid;");
const edit = await import(
  url(
    compile("edit.ts")
      .replace('"./model"', JSON.stringify(fakeModel))
      .replace('"../_lib/appkit"', JSON.stringify(fakeKit))
      .replace('"./geometry"', JSON.stringify(geometryURL)),
  )
);
edit.align(["a", "b"], "left");
assert.equal(rows.find((r) => r.id === "b").data.x, 0);
edit.undo();
assert.equal(rows.find((r) => r.id === "b").data.x, 400);
edit.redo();
assert.equal(rows.find((r) => r.id === "b").data.x, 0);
edit.undo();
const ids = edit.duplicate(edit.selectedGraph(["a", "b"]));
assert.equal(ids.length, 3);
const line = rows.find((r) => ids.includes(r.id) && r.data.type === "arrow");
assert(ids.includes(line.data.from) && ids.includes(line.data.to));
edit.undo();
assert.equal(rows.length, 5);
edit.redo();
assert.equal(rows.length, 8);
edit.remove(["a"]);
assert(!rows.some((r) => r.id === "a" || r.id === "ab"));
edit.undo();
assert(rows.some((r) => r.id === "ab"));
assert.equal(rows.length, 8);
edit.patchMany(["c"], { locked: true });
edit.remove(["c"]);
assert(rows.some((r) => r.id === "c"));
console.log(
  "PASS backwards-compatible nodes, anchors/routes, graph clipboard remapping, safe SVG, grouped undo/redo, cascading delete and locked nodes",
);

edit.patchMany(["a"], { text: "My edit" });
rows.find((r) => r.id === "a").data.text = "Agent edit";
edit.undo();
assert.equal(
  rows.find((r) => r.id === "a").data.text,
  "Agent edit",
  "undo keeps newer collaborator changes",
);
assert.equal(
  g.parseClipboard(
    g.clipboardPrefix +
      JSON.stringify([{ id: "x", data: { type: "sticky", x: "invalid" } }]),
  ),
  null,
);
console.log("PASS collaborator-safe undo and malformed clipboard rejection");
// Slow hub writes must not race a rapid delete/undo recreation. Exercise the
// real shared Store with version checks, rather than a permissive fake.
let backend = {
    collection: "items",
    id: "node",
    version: 1,
    data: { text: "Original" },
  },
  calls = [];
const pause = (ms) => new Promise((r) => setTimeout(r, ms));
globalThis.window = {
  myco: {
    data: {
      list: async () => ({
        records: backend ? [structuredClone(backend)] : [],
        seq: 0,
      }),
      put: async (collection, data, opts) => {
        calls.push("put");
        await pause(10);
        assert.equal(
          opts.ifVersion,
          backend?.version ?? 0,
          "version matches the ordered remote state",
        );
        backend = {
          collection,
          id: opts.id,
          version: (backend?.version ?? 0) + 1,
          data: structuredClone(data),
        };
        return structuredClone(backend);
      },
      remove: async () => {
        calls.push("delete");
        await pause(30);
        backend = null;
        return true;
      },
    },
  },
};
const sdk = await import(
  url(
    inlineI18n(
      ts.transpile(
        fs.readFileSync(new URL("src/_lib/myco.ts", import.meta.url), "utf8"),
        { target: 99, module: 99 },
      ),
    ),
  )
);
const store = new sdk.Store("items");
await store.load();
const saving = store.put({ text: "Edited" }, "node");
await Promise.resolve();
const deleting = store.remove("node"),
  restoring = store.put({ text: "Restored" }, "node");
await Promise.all([saving, deleting, restoring]);
assert.equal(backend.data.text, "Restored");
assert.equal(store.get("node").data.text, "Restored");
assert.deepEqual(calls, ["put", "delete", "put"]);
const savingAgain = store.put({ text: "Last edit" }, "node");
await Promise.resolve();
const deletingAgain = store.remove("node");
await Promise.all([savingAgain, deletingAgain]);
assert.equal(backend, null);
assert.equal(store.get("node"), null);
console.log(
  "PASS real Store serializes slow put/delete/recreate with strict hub versions and no ghost nodes",
);
