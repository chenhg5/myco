import assert from "node:assert/strict";
import fs from "node:fs";
import ts from "typescript";
const source = fs.readFileSync(
  new URL("src/proxy-console/model.ts", import.meta.url),
  "utf8",
);
const js = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ES2022,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const m = await import(
  "data:text/javascript;base64," + Buffer.from(js).toString("base64")
);
const prev = structuredClone(m.demoSnapshot),
  next = structuredClone(prev);
prev.at = "2026-11-21T00:00:00Z";
next.at = "2026-11-21T00:00:02Z";
next.traffic.downloadTotal += 2048;
next.traffic.uploadTotal += 1024;
assert.deepEqual(m.rate(prev, next), {
  at: Date.parse(next.at),
  down: 1024,
  up: 512,
});
assert.equal(m.rate(undefined, next), undefined);
for (const changed of [
  { at: prev.at },
  { at: "invalid" },
  { at: "2026-11-21T00:01:00Z" },
  { warnings: ["/connections: unsupported"] },
  { traffic: { ...next.traffic, downloadTotal: 0 } },
])
  assert.equal(m.rate(prev, { ...next, ...changed }), undefined);
assert.equal(
  m.rate({ ...prev, warnings: ["/connections: unavailable"] }, next),
  undefined,
);
assert.equal(m.bytes(NaN), "—");
assert.equal(m.bytes(-1), "—");
assert.equal(m.bytes(2048), "2 KB");
console.log(
  "PASS rate calculation, counter reset, long gap, unavailable section, invalid timestamp and formatting",
);
