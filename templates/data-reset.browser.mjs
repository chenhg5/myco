// When the change feed has moved on past what a page holds (too much
// changed while it was away), the App says so with {reset: true}; the
// actual pagebridge.js passes it on as myco.on("reset"), and the page
// SDK's stores list their records again. Here a parent page plays the App.
// Run: node data-reset.browser.mjs
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";

const here = path.dirname(fileURLToPath(import.meta.url));
const bridge = await fs.readFile(path.resolve(here, "../internal/console/pagebridge.js"), "utf8");
// the SDK's Store and listen, as an app bundles them
const entry = path.join(here, "src/_lib/myco.ts");
const sdk = (await build({ entryPoints: [entry], bundle: true, write: false, format: "iife", globalName: "sdk", logLevel: "silent" })).outputFiles[0].text;

const page = `<!doctype html><html><head><script>${bridge.replace(/<\/script/gi, "<\\/script")}</script><script>${sdk}</script></head><body><script>
  const items = new sdk.Store("items");
  window.log = [];
  (async () => {
    await window.myco.ready;
    await items.load();
    window.log.push("loaded " + items.rows.map((r) => r.id).join(","));
    sdk.listen([items], (c) => window.log.push("change " + c.record), () => window.log.push("reset " + items.rows.map((r) => r.id).join(",")));
    window.log.push("listening " + window.myco.features.includes("data-reset"));
  })();
</script></body></html>`;

// the App: answers ready and data.list (what the hub holds now), then sends a change and a reset
const app = `<!doctype html><html><body><iframe id="f" src="/page"></iframe><script>
  let records = [{ collection: "items", id: "a", data: { order: 1 }, version: 1 }];
  const f = document.getElementById("f");
  window.addEventListener("message", (e) => {
    const d = e.data;
    if (!d || d.myco !== "call") return;
    let value = null;
    if (d.method === "ready") value = { page: { id: "pg_test", name: "t" }, me: { name: "glenn" } };
    else if (d.method === "data.list") value = { records, seq: 10 };
    else if (d.method === "data.watch") value = true;
    f.contentWindow.postMessage({ myco: "result", id: d.id, value }, "*");
  });
  window.change = () => f.contentWindow.postMessage({ myco: "data", changes: [{ seq: 11, collection: "items", record: "b", op: "put", data: { order: 2 }, version: 1 }], reset: false }, "*");
  window.resetFeed = () => {
    // meanwhile, a lot changed: a and b went, c and d came
    records = [{ collection: "items", id: "c", data: { order: 1 }, version: 3 }, { collection: "items", id: "d", data: { order: 2 }, version: 1 }];
    f.contentWindow.postMessage({ myco: "data", changes: [], reset: true }, "*");
  };
</script></body></html>`;

const server = http.createServer((req, res) => {
  res.setHeader("Content-Type", "text/html");
  res.end(req.url.startsWith("/page") ? page : app);
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));

let chromium;
for (const from of [here, "/opt/node-tools/node_modules/"]) {
  try { chromium = createRequire(path.join(from, "x.js"))("playwright").chromium; break; } catch { /* (next) */ }
}
if (!chromium) { console.log("SKIP Playwright is not installed"); process.exit(0); }
const browser = await chromium.launch();
try {
  const p = await browser.newPage();
  const errors = [];
  p.on("pageerror", (e) => errors.push(e.message));
  await p.goto(`http://127.0.0.1:${server.address().port}/`);
  const frame = () => p.frames().find((f) => f.url().endsWith("/page"));
  const log = async () => frame().evaluate(() => window.log);
  await p.waitForFunction(() => document.getElementById("f").contentWindow.log?.length >= 2);
  assert.deepEqual(await log(), ["loaded a", "listening true"]);
  await p.evaluate(() => window.change());
  await p.waitForFunction(() => document.getElementById("f").contentWindow.log.length >= 3);
  await p.evaluate(() => window.resetFeed());
  await p.waitForFunction(() => document.getElementById("f").contentWindow.log.length >= 4);
  assert.deepEqual(await log(), ["loaded a", "listening true", "change b", "reset c,d"], "the store holds what the hub holds after a reset");
  // the App's word to the app's maker (an undeclared permission) lands in the page's console
  const warnings = [];
  p.on("console", (m) => { if (m.type() === "warning") warnings.push(m.text()); });
  await p.evaluate(() => document.getElementById("f").contentWindow.postMessage({ myco: "warn", text: "myco: this app uses download without declaring it" }, "*"));
  await p.waitForTimeout(200);
  assert.deepEqual(warnings, ["myco: this app uses download without declaring it"]);
  assert.deepEqual(errors, []);
  console.log("PASS data reset: the bridge passes it on, the stores list again; warnings reach the console");
} finally {
  await browser.close();
  server.close();
}
