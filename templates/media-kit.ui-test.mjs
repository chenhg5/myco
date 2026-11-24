// Exercise the built template, the real iframe bridge and the desktop file
// dispatcher. The server below supplies fixture data and project file routes.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(path.join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const web = createRequire(new URL("../web/package.json", import.meta.url));
const { build } = web("esbuild");
const source = fileURLToPath(new URL("../web/src/", import.meta.url));
const { outputFiles } = await build({
  entryPoints: [path.join(source, "app/page-project-files.ts")],
  alias: { "@": source }, bundle: true, format: "esm", write: false,
});
const bridge = readFileSync(new URL("../internal/console/pagebridge.js", import.meta.url), "utf8");
const root = new URL("../internal/templates/official/media-kit/", import.meta.url);
const files = new Map();
const rows = new Map();
let lastDownload;
let requests = 0;
const json = (res, data) => res.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify(data));
const host = `
  import { answerProjectFiles, projectFileScope } from "/dispatcher.js";
  const frame = document.querySelector("iframe");
  const params = new URLSearchParams(location.search);
  const page = { id: "pg_test", project: "pilot", template: { id: "tp_myco_media-kit" } };
  window.scopeTest = async () => {
    const cases = [
      [{ ...page, project: undefined }, "mk-pg_test-entry-valid000.png"],
      [{ ...page, template: { id: "tp_other" } }, "mk-pg_test-entry-valid000.png"],
      [page, "mk-pg_another-entry-valid000.png"],
      [page, "mk-pg_test-../valid000.png"],
      [page, "readme.md"],
    ];
    const denied = answerProjectFiles("hub", { ...page, read_only: true }, "projectFiles.put", {
      name: "mk-pg_test-entry-valid000.png", blob: new Blob(["bytes"], { type: "image/png" }),
    }).then(() => "accepted", error => error.code);
    return Promise.all([denied, ...cases.map(async ([value, name]) => {
      try { await answerProjectFiles("hub", value, "projectFiles.get", { name }); }
      catch (error) { return error.code; }
      return "accepted";
    })]);
  };
  addEventListener("message", async e => {
    const { id, method, args, myco } = e.data || {};
    if (e.source !== frame.contentWindow || e.origin !== location.origin || myco !== "call") return;
    const reply = payload => e.source.postMessage({ myco: "result", id, ...payload }, e.origin);
    try {
      if (method === "ready") return reply({ value: {
        page, me: { name: "tester", can_edit: !params.has("readonly") }, projectFiles: !params.has("legacy") && Boolean(projectFileScope(page)),
        preferences: { theme: params.has("dark") ? "dark" : "light", language: "en" },
      } });
      if (method === "data.watch") return reply({ value: true });
      if (method.startsWith("data.")) {
        const response = await fetch("/records", { method: "POST", body: JSON.stringify({ method, args }) });
        return reply(await response.json());
      }
      const value = await answerProjectFiles("hub", page, method, args);
      reply({ value });
    } catch (error) { reply({ error: error.message, code: error.code }); }
  });
  frame.src = "/template/";
`;
const server = createServer(async (req, res) => {
  const route = new URL(req.url, "http://test").pathname;
  if (route === "/") return res.writeHead(200, { "content-type": "text/html" }).end(
    '<style>html,body,iframe{height:100%;width:100%;margin:0;border:0;display:block}</style><iframe allow="clipboard-write"></iframe><script type="module" src="/host.js"></script>',
  );
  if (route === "/host.js" || route === "/dispatcher.js") return res.writeHead(200, { "content-type": "text/javascript" }).end(route === "/host.js" ? host : outputFiles[0].text);
  if (route.startsWith("/template/")) {
    const name = route.slice(10) || "index.html";
    if (name === "bridge.js") return res.writeHead(200, { "content-type": "text/javascript" }).end(bridge);
    try {
      const body = readFileSync(new URL(name, root));
      if (name === "index.html") return res.writeHead(200, { "content-type": "text/html" }).end(
        body.toString().replace("<head>", '<head><script src="./bridge.js"></script>'));
      const type = name.endsWith(".js") ? "text/javascript" : "text/css";
      return res.writeHead(200, { "content-type": type }).end(body);
    } catch { return res.writeHead(404).end(); }
  }
  if (route === "/app/hubs/hub/projects/pilot") {
    return json(res, { docs: Array.from({ length: 96 + files.size }, () => ({})) });
  }
  const fileRoute = /^\/app\/hubs\/hub\/projects\/pilot\/files\/([^/]+)(\/save)?$/.exec(route);
  if (fileRoute) {
    requests++;
    const [, filename, save] = fileRoute;
    if (save) { lastDownload = filename; return json(res, { path: "/Downloads/" + filename }); }
    if (req.method === "PUT") {
      const chunks = [];
      for await (const chunk of req) chunks.push(chunk);
      files.set(filename, { bytes: Buffer.concat(chunks), type: req.headers["content-type"] });
      return json(res, { name: filename });
    }
    const file = files.get(filename);
    return file ? res.writeHead(200, { "content-type": file.type }).end(file.bytes) : res.writeHead(404).end();
  }
  if (route === "/records") {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const { method, args } = JSON.parse(Buffer.concat(chunks));
    if (method === "data.list") return json(res, { value: { records: [...rows.values()].filter(row => row.collection === args.collection), seq: 0 } });
    const key = args.collection + "/" + args.id;
    const version = rows.get(key)?.version || 0;
    if (args.ifVersion !== version) return json(res, { error: "Record changed", code: "conflict" });
    const row = { id: args.id, collection: args.collection, data: args.data, version: version + 1 };
    rows.set(key, row);
    return json(res, { value: row });
  }
  res.writeHead(404).end();
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
const browser = await chromium.launch({ executablePath: process.env.UI_SMOKE_EXECUTABLE });
try {
  const base = `http://127.0.0.1:${server.address().port}`;
  const page = await browser.newPage({ viewport: { width: 1200, height: 800 } });
  page.setDefaultTimeout(10000);
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.goto(base);
  const content = page.frameLocator("iframe");
  await content.getByText("Nearly full", { exact: false }).waitFor();
  assert.deepEqual(await page.evaluate(() => window.scopeTest()), ["permission_denied", "permission_denied", "permission_denied", "invalid_argument", "invalid_argument", "invalid_argument"]);
  assert.equal(requests, 0, "forbidden requests must stop at the desktop scope");

  await content.getByRole("button", { name: "New entry", exact: true }).click();
  await content.getByLabel("Title").fill("Usage reporting");
  await content.getByLabel("Labels").fill("App, CLI");
  await content.getByLabel("English tagline").fill("Track tokens by agent.");
  await content.getByLabel("English caption").fill("Share usage totals across the workspace.");
  const png = await page.evaluate(() => {
    const canvas = document.createElement("canvas");
    canvas.width = 200; canvas.height = 120;
    const ctx = canvas.getContext("2d");
    ctx.fillStyle = "#f06741"; ctx.fillRect(0, 0, 200, 120);
    return canvas.toDataURL("image/png").split(",")[1];
  });
  const sourcePNG = Buffer.from(png, "base64");
  // PNG decoders stop at IEND. The trailing bytes test the actual original
  // upload size and byte preservation without committing a huge fixture.
  const original = Buffer.concat([sourcePNG, Buffer.alloc(11 * 1024 * 1024)]);
  await content.locator('input[type="file"]').setInputFiles([
    { name: "usage.png", mimeType: "image/png", buffer: original },
    { name: "alt.png", mimeType: "image/png", buffer: sourcePNG },
  ]);
  await content.getByLabel("Image language").selectOption("en");
  await content.getByLabel("Image theme").selectOption("dark");
  await content.getByRole("button", { name: "Save", exact: true }).click();
  await content.getByText("Uploaded 2 images").waitFor();
  const [first, second] = [...rows.values()].filter(row => row.collection === "images");
  assert.equal(first.data.width, 200);
  assert.equal(first.data.height, 120);
  assert.equal(first.data.language, "en");
  assert.match(first.data.file, /^mk-pg_test-entry_[a-z0-9]+-[a-z0-9]{20}\.png$/);
  assert.ok(files.get(first.data.file).bytes.equals(original));
  await content.getByRole("button", { name: "Next image" }).click();
  await content.getByText("2 / 2", { exact: true }).waitFor();
  await content.getByRole("button", { name: "Copy text" }).last().click();
  assert.equal(await page.evaluate(() => navigator.clipboard.readText()), "Share usage totals across the workspace.");
  await content.getByRole("button", { name: "Copy image" }).click();
  await content.getByText("Image copied", { exact: true }).waitFor();
  assert.ok(await page.evaluate(async () => (await navigator.clipboard.read())[0].types.includes("image/png")));
  await content.getByRole("button", { name: "Download original" }).click();
  await content.getByText("Downloaded", { exact: true }).waitFor();
  assert.equal(lastDownload, second.data.file);
  await content.getByRole("button", { name: "Edit image details" }).click();
  await content.getByLabel("Image note").fill("Dark layout");
  await content.getByRole("button", { name: "Save", exact: true }).click();
  await content.getByText(/Dark layout/).waitFor();
  await content.getByRole("button", { name: "Edit or add images" }).click();
  await content.getByLabel("Status").selectOption("published");
  await content.getByLabel("Published to").fill("Community group");
  await content.getByRole("button", { name: "Save", exact: true }).click();
  await content.getByText("Community group", { exact: true }).waitFor();
  await content.getByRole("button", { name: "Back", exact: true }).click();
  await content.getByLabel("Search assets").fill("missing");
  await content.getByText("No entries match these filters.").waitFor();
  await content.getByLabel("Search assets").fill("Track tokens");
  await content.getByLabel("Labels", { exact: true }).selectOption("CLI");
  await content.getByLabel("Status", { exact: true }).selectOption("published");
  await content.getByRole("button", { name: /Usage reporting/ }).waitFor();
  assert.deepEqual(errors, []);

  const narrow = await browser.newPage({ viewport: { width: 400, height: 780 }, colorScheme: "dark" });
  narrow.setDefaultTimeout(10000);
  await narrow.goto(base + "?legacy&dark");
  const fallback = narrow.frameLocator("iframe");
  await fallback.getByText(/Open this project page in an updated desktop app/).waitFor();
  await fallback.getByRole("button", { name: /Usage reporting/ }).click();
  assert.ok(await fallback.getByRole("button", { name: "Download original" }).isDisabled());
  const dimensions = await fallback.locator(".mk-app").evaluate(el => [el.scrollWidth, el.clientWidth]);
  assert.equal(dimensions[0], dimensions[1]);
  await narrow.goto(base + "?readonly&dark");
  await fallback.getByText("View only", { exact: true }).waitFor();
  assert.equal(await fallback.getByRole("button", { name: "New entry" }).count(), 0);
  await fallback.getByRole("button", { name: /Usage reporting/ }).click();
  assert.equal(await fallback.getByRole("button", { name: "Edit or add images" }).count(), 0);
  assert.ok(await fallback.getByRole("button", { name: "Download original" }).isEnabled());
  console.log("Media Kit: desktop iframe file scope, 11 MiB original round-trip, quota, image metadata, clipboard, download, filters and narrow legacy view passed");
} finally {
  await browser.close();
  server.close();
}
