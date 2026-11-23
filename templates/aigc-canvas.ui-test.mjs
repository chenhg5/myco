import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const require = createRequire(join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const root = new URL("../internal/templates/official/aigc-canvas/", import.meta.url);
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://test").pathname;
  try {
    const body = readFileSync(new URL(path === "/" ? "index.html" : path.slice(1), root));
    const type = path.endsWith(".js") ? "text/javascript" : path.endsWith(".css") ? "text/css" : "text/html";
    res.writeHead(200, { "Content-Type": type, "Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self'; img-src blob:" });
    res.end(body);
  } catch { res.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
const browser = await chromium.launch({ executablePath: process.env.UI_SMOKE_EXECUTABLE });
try {
  const page = await browser.newPage({ viewport: { width: 1100, height: 700 } });
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.addInitScript(() => {
    const rows = new Map();
    window.rows = rows;
    const listeners = [];
    const requests = [];
    window.providerRequests = requests;
    window.myco = {
      features: ["net", "preferences"], preferences: { theme: "light", language: "en" },
      ready: Promise.resolve({ me: { name: "tester" }, page: { id: "pg_canvas" } }),
      data: {
        list: async () => ({ records: [...rows.values()], seq: 0 }),
        put: async (collection, data, { id, ifVersion }) => {
          const version = rows.get(id)?.version || 0;
          if (ifVersion !== version) throw { code: "conflict" };
          const result = { id, collection, data, version: version + 1 };
          rows.set(id, result);
          for (const callback of listeners) callback({ op: "put", record: id, collection, data, version: result.version });
          return result;
        },
        on: callback => { listeners.push(callback); return () => {}; },
      },
      net: { fetch: async options => {
        requests.push({ url: options.url, body: options.body, authenticated: options.headers?.Authorization?.startsWith("Bearer "), redirect: options.redirect });
        const response = json => ({ status: 200, body: JSON.stringify(json) });
        if (options.url === "https://openrouter.ai/api/v1/chat/completions")
          return response({ choices: [{ message: { content: "A lighthouse in fog" } }] });
        if (options.url.includes("/models/example/image/"))
          return response({ id: "image_id", status: "succeeded", output: ["https://media.example/frame.png"] });
        if (options.url.includes("/models/example/video/"))
          return response({ id: "video_id", status: "starting" });
        if (options.url.endsWith("/predictions/video_id"))
          return response({ status: "succeeded", output: "https://media.example/clip.mp4" });
        return { status: 404, headers: {} };
      } },
    };
  });
  await page.goto(`http://127.0.0.1:${server.address().port}/`);
  const header = page.locator(".ac-header");
  const inspector = page.locator(".ac-inspector");
  await header.getByText("AIGC Canvas").waitFor();
  await header.getByText("Generation services").click();
  await header.getByLabel("OpenRouter").fill("test-text-key");
  await header.getByLabel("Replicate").fill("test-media-key");
  await header.getByText("Generation services").click();
  await header.getByRole("button", { name: "Sticky note", exact: true }).click();
  await inspector.getByLabel("Title").fill("Brief");
  await inspector.getByLabel("Note contents").fill("Sea mist and a warm light");
  await inspector.getByRole("button", { name: "Save", exact: true }).click();
  await page.locator(".ac-node").getByText("Brief", { exact: true }).waitFor();

  async function add(kind, title, prompt, model, from, options = "") {
    await header.getByRole("button", { name: kind, exact: true }).click();
    await inspector.getByLabel("Title").fill(title);
    await inspector.getByLabel("Prompt").fill(prompt);
    await inspector.getByLabel("Model ID").fill(model);
    await inspector.getByLabel("Upstream node").selectOption({ label: from });
    if (options) await inspector.getByLabel("Model inputs (JSON)").fill(options);
    await inspector.getByRole("button", { name: "Save", exact: true }).click();
    await page.locator(".ac-node").getByText(title, { exact: true }).waitFor();
  }

  await add("Text", "Script", "Create a line", "example/text", "Brief");
  await add("Image", "Frame", "Draw the frame", "example/image", "Script");
  await add("Video", "Clip", "Animate the frame", "example/video", "Frame", '{"image":"{{upstream}}"}');
  assert.equal(await page.locator(".ac-links path").count(), 3);
  await inspector.getByRole("button", { name: "Generate" }).click();
  await inspector.getByRole("link", { name: "Open or save original" }).waitFor();
  assert.equal(await inspector.getByRole("link").getAttribute("href"), "https://media.example/clip.mp4");
  const state = await page.evaluate(() => ({ rows: [...window.rows.values()], calls: window.providerRequests }));
  assert.equal(state.calls.length, 4);
  assert.ok(state.calls.every(call => call.authenticated && call.redirect === false));
  assert.equal(JSON.parse(state.calls[0].body).messages[0].content, "Create a line\n\nSea mist and a warm light");
  assert.equal(JSON.parse(state.calls[1].body).input.prompt, "Draw the frame\n\nA lighthouse in fog");
  assert.deepEqual(JSON.parse(state.calls[2].body).input, { image: "https://media.example/frame.png", prompt: "Animate the frame" });
  assert.ok(!JSON.stringify(state.rows).includes("test-media-key") && !JSON.stringify(state.rows).includes("test-text-key"));
  const card = page.locator(".ac-node").filter({ hasText: "Brief" });
  const position = await card.boundingBox();
  await page.mouse.move(position.x + 40, position.y + 30);
  await page.mouse.down();
  await page.mouse.move(position.x + 75, position.y + 68, { steps: 4 });
  await page.mouse.up();
  const moved = await card.boundingBox();
  assert.ok(moved.x > position.x + 25 && moved.y > position.y + 25);

  await page.locator(".ac-node").filter({ hasText: "Script" }).click();
  await inspector.getByLabel("Upstream node").selectOption({ label: "Clip" });
  await inspector.getByRole("button", { name: "Save", exact: true }).click();
  await page.getByText("This connection would create a cycle.").waitFor();
  await page.setViewportSize({ width: 390, height: 820 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth), 390);
  assert.deepEqual(errors, []);
  console.log("PASS AIGC Canvas shared graph, CAS, connected generation through both providers, secrets, drag, and cycle validation");
} finally {
  await browser.close();
  server.close();
}
