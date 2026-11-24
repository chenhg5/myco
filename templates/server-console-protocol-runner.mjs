// Cross-language test entry: stdin is a real native sshcap.Result, not a mock.
import fs from "node:fs";
import assert from "node:assert/strict";
import ts from "typescript";
const source = fs.readFileSync(
  new URL("./src/server-console/protocol.ts", import.meta.url),
  "utf8",
);
const code = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
  },
}).outputText;
const { parseOverview } = await import(
  "data:text/javascript;base64," + Buffer.from(code).toString("base64")
);
const overview = parseOverview(JSON.parse(fs.readFileSync(0, "utf8")));
assert.ok(overview.memory_total > 0);
assert.ok(
  overview.memory_available >= 0 &&
    overview.memory_available <= overview.memory_total,
);
assert.ok(
  overview.cpu_percent === null ||
    (overview.cpu_percent >= 0 && overview.cpu_percent <= 100),
);
assert.ok(overview.uptime >= 0);
assert.equal(overview.load.length, 3);
assert.ok(overview.disks.length > 0);
assert.ok(overview.kernel.length > 0);
console.log(
  "PASS real SSH overview -> native JSON -> template protocol adapter",
);
