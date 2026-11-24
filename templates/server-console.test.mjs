// Missing data must stay missing; metric charts must not hide disconnects.
import assert from "node:assert/strict";
import fs from "node:fs";
import ts from "typescript";
const source = fs.readFileSync(
  new URL("./src/server-console/format.ts", import.meta.url),
  "utf8",
);
const js = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ES2022 },
}).outputText;
const f = await import(
  "data:text/javascript;base64," + Buffer.from(js).toString("base64")
);
assert.equal(f.percent(1, 0), null);
assert.equal(f.percent(NaN, 10), null);
assert.equal(f.percent(2, 10), 20);
assert.equal(f.percent(12, 10), 100);
assert.equal(f.bytes(NaN), "—");
assert.equal(f.bytes(1024), "1.0 KiB");
assert.equal(f.duration(-1), "—");
assert.equal(f.duration(90000), "1d 1h");
const graph = f.chartPath([20, null, 40], 100, 100);
assert.equal(graph, "M0.0,80.0 M100.0,60.0");
assert.equal(f.chartPath([null, null]), "");
console.log(
  "PASS missing metrics, byte/duration formatting and chart disconnect gaps",
);
