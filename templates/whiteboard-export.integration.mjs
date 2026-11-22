// Real Hub + built whiteboard + ordinary agent data access. Only the App's
// bridge transport is adapted for the test browser; records/schema/CAS are real.
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import http from "node:http";
import { createRequire } from "node:module";
import { execFileSync, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { actionModule } from "./action-test-modules.mjs";
const here = path.dirname(fileURLToPath(import.meta.url));
const fixture = JSON.parse(await new Promise(resolve => {
  let s = ""; process.stdin.setEncoding("utf8"); process.stdin.on("data", d => s += d); process.stdin.on("end", () => resolve(s));
}));
let chromium;
try {
  const require = createRequire(path.join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
  chromium = require("playwright").chromium;
} catch { console.log("SKIP Playwright is not installed"); process.exit(0); }
let browser;
try { browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] }); }
catch { console.log("SKIP Playwright Chromium is not installed"); process.exit(0); }
const { sourceFingerprint, sharedExportState } = await import(await actionModule("whiteboard/shared-export"));
const root = path.resolve(here, "../internal/templates/official/whiteboard");
const api = async (route = "", method = "GET", body) => {
  const r = await fetch(fixture.url + "/v1/pages/" + fixture.page + "/data" + route, {
    method, headers: { Authorization: "Bearer " + fixture.token, "Content-Type": "application/json" },
    // Match the desktop Go bridge's HTML-escaped JSON representation.
    ...(body === undefined ? {} : { body: JSON.stringify(body).replace(/[<>&\u2028\u2029]/g, c => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0")) }),
  });
  const value = r.status === 204 ? null : await r.json();
  if (!r.ok) throw Object.assign(new Error("Hub data request failed: " + value?.error), { code: value?.error });
  return value;
};
const write = (c, id, data, v = 0) => api("/" + c + "/" + id, "PUT", { data, if_version: v });
const fingerprint = records => sourceFingerprint({ board: records.filter(r => r.collection === "board"), items: records.filter(r => r.collection === "items") });
const snapshot = async () => {
  const all = await api();
  return { all, exported: all.records.find(r => r.collection === "exports" && r.id === "main")?.data, fingerprint: await fingerprint(all.records) };
};
const wait = async (test, label) => {
  const end = Date.now() + 8000;
  while (Date.now() < end) { const x = await snapshot(); if (test(x)) return x; await new Promise(r => setTimeout(r, 75)); }
  throw new Error(label);
};
const server = http.createServer(async (req, res) => {
  const pathname = new URL(req.url, "http://test").pathname;
  const name = path.resolve(root, "." + (pathname === "/" ? "/index.html" : pathname));
  if (!name.startsWith(root + path.sep)) { res.writeHead(403); res.end(); return; }
  try {
    const bytes = await fs.readFile(name);
    const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml" };
    res.writeHead(200, { "Content-Type": types[path.extname(name)] || "application/octet-stream" }); res.end(bytes);
  } catch { res.writeHead(404); res.end(); }
});
await new Promise(r => server.listen(0, "127.0.0.1", r));
const context = await browser.newContext({ viewport: { width: 1200, height: 700 } });
let page;
const errors = [];
await context.exposeBinding("whiteboardData", async (_, method, collection, data, opts) => {
  try {
    const value = await (method === "list" ? api("?collection=" + encodeURIComponent(collection || ""))
      : method === "get" ? api("/" + collection + "/" + opts.id)
      : write(collection, opts.id, data, opts.ifVersion));
    return { value };
  } catch (error) {
    // Playwright does not preserve custom Error fields across this binding.
    // Carry the same explicit code/message envelope as the real App bridge.
    return { error: { code: error.code, message: error.message } };
  }
});
await context.addInitScript(({ id }) => {
  const listeners = new Set();
  window.whiteboardChange = c => { for (const f of listeners) f(c); };
  const call = async (...args) => {
    const response = await window.whiteboardData(...args);
    if (response.error) throw Object.assign(new Error(response.error.message), { code: response.error.code });
    return response.value;
  };
  window.myco = {
    ready: Promise.resolve({ page: { id, name: "Agent export acceptance" }, me: { name: "whiteboard-export-agent" } }),
    data: {
      list: c => call("list", c),
      get: (c, id) => call("get", c, null, { id }),
      put: (c, data, opts) => call("put", c, data, opts),
      on: f => { listeners.add(f); return () => listeners.delete(f); },
    },
  };
}, { id: fixture.page });
const open = async () => {
  page = await context.newPage(); page.on("pageerror", e => errors.push(e.message));
  await page.goto(`http://127.0.0.1:${server.address().port}/`);
  await page.locator(".wb-item").first().waitFor();
};
const notify = r => page.evaluate(r => window.whiteboardChange({ seq: 1, collection: r.collection, record: r.id, op: "put", data: r.data, version: r.version }), r);
try {
  await write("board", "main", { title: "Agent export acceptance" });
  let first = await write("items", "brief", { type: "rect", text: "Read the brief", x: 80, y: 120, w: 240, h: 110 });
  await write("items", "draft", { type: "rect", text: "Create the draft", x: 440, y: 120, w: 240, h: 110 });
  await write("items", "flow", { type: "arrow", from: "brief", to: "draft", text: "Next", route: "elbow" });
  await open();
  const initial = await wait(x => sharedExportState(x.exported, x.fingerprint) === "ready", "initial SVG was not published");
  first = await write("items", "brief", { ...first.data, text: "核对源数据，再交付文件" }, first.version);
  await notify(first);
  const fresh = await wait(x => x.fingerprint !== initial.fingerprint && sharedExportState(x.exported, x.fingerprint) === "ready", "agent edit did not produce fresh SVG");
  assert(fresh.exported.svg.replace(/<[^>]+>/g, "").includes("核对源数据，再交付文件"));
  await fs.mkdir(fixture.output, { recursive: true });
  const recordsPath = path.join(fixture.output, "records.json"), svgPath = path.join(fixture.output, "whiteboard.svg");
  await fs.writeFile(recordsPath, JSON.stringify(fresh.all));
  const guide = await fs.readFile(path.join(root, "agent.md"), "utf8");
  const recipe = guide.match(/<<'JS'\n([\s\S]*?)\nJS/)[1];
  // The guide creates a new output; avoid retaining a prior test artifact.
  await fs.rm(svgPath, { force: true });
  const extracted = spawnSync(process.execPath, ["--input-type=module", "-", recordsPath, svgPath], { input: recipe, encoding: "utf8" });
  assert.equal(extracted.status, 0, "documented extraction recipe failed");
  assert.equal(await fs.readFile(svgPath, "utf8"), fresh.exported.svg);
  const preview = await context.newPage();
  await preview.goto("data:image/svg+xml;base64," + Buffer.from(fresh.exported.svg).toString("base64"));
  await preview.screenshot({ path: path.join(fixture.output, "whiteboard-export.png") });
  await preview.close();
  await page.close();
  first = await write("items", "brief", { ...first.data, text: "Edit while the page is closed" }, first.version);
  const stale = await snapshot();
  assert.equal(sharedExportState(stale.exported, stale.fingerprint), "stale");
  await fs.writeFile(recordsPath, JSON.stringify(stale.all));
  const refusedPath = path.join(fixture.output, "must-not-exist.svg");
  const refused = spawnSync(process.execPath, ["--input-type=module", "-", recordsPath, refusedPath], { input: recipe, encoding: "utf8" });
  assert.notEqual(refused.status, 0);
  assert(refused.stderr.includes("stale:"));
  await assert.rejects(fs.stat(refusedPath), { code: "ENOENT" });
  await open();
  await wait(x => sharedExportState(x.exported, x.fingerprint) === "ready", "reopening did not regenerate");
  first = await write("items", "brief", { ...first.data, text: "x".repeat(16_001) }, first.version);
  await notify(first);
  const large = await wait(x => sharedExportState(x.exported, x.fingerprint) === "too_large", "oversized export did not become too_large");
  assert.equal(large.exported.svg, undefined);
  assert.equal(errors.length, 0, "browser exceptions");
  console.log("PASS real Hub + built whiteboard: agent edit -> fresh shared SVG -> documented extraction -> rendered image; closed-page stale rejection -> regenerate -> too_large without truncation");
} finally {
  await context.close(); await browser.close(); await new Promise(r => server.close(r));
}
