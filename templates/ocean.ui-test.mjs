// 航海 in a browser: the built app with its sample and an in-memory myco
// bridge (as shots.mjs), drawn by Chromium's software GPU. Drives the boat
// with the keyboard, changes the shared weather, reaches an island, runs
// aground, opens the map, and checks English and a browser without WebGL.
// Run: node ocean.ui-test.mjs (after node build.mjs ocean).
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const here = path.dirname(fileURLToPath(import.meta.url)),
  official = path.resolve(here, "../internal/templates/official");
const shots = fs.readFileSync(path.join(here, "shots.mjs"), "utf8");
const bridge = shots.slice(shots.indexOf("const bridge = `") + 16, shots.indexOf("})();`;") + 5);
const sample = JSON.parse(fs.readFileSync(path.join(official, "ocean/sample.json"), "utf8"));

const srv = http.createServer((req, res) => {
  let p = decodeURIComponent(new URL(req.url, "http://x").pathname);
  if (p.endsWith("/")) p += "index.html";
  if (p === "/__bridge.js") { res.writeHead(200, { "Content-Type": "text/javascript" }); res.end(bridge); return; }
  const f = path.resolve(official, "." + p);
  if (!f.startsWith(official + path.sep) || !fs.existsSync(f)) { res.writeHead(404); res.end(); return; }
  let body = fs.readFileSync(f);
  if (f.endsWith(".html")) body = Buffer.from(String(body).replace("<head>", '<head><script src="/__bridge.js"></script>'));
  const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".json": "application/json", ".svg": "image/svg+xml" };
  res.writeHead(200, { "Content-Type": types[path.extname(f)] || "application/octet-stream" });
  res.end(body);
});
await new Promise((r) => srv.listen(0, "127.0.0.1", r));
const url = `http://127.0.0.1:${srv.address().port}/ocean/`;

function playwright() {
  for (const from of [here, "/opt/node-tools/node_modules/"]) {
    try { return createRequire(path.join(from, "x.js"))("playwright").chromium; } catch { /* (next) */ }
  }
  throw new Error("playwright is not installed");
}
const browser = await playwright().launch({
  args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
  ...(process.env.UI_SMOKE_EXECUTABLE ? { executablePath: process.env.UI_SMOKE_EXECUTABLE } : {}),
});
const open = async (opts = {}, init) => {
  const page = await browser.newPage({ viewport: { width: 1100, height: 700 }, locale: "zh-CN", ...opts });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
  await page.addInitScript((s) => { window.__SAMPLE__ = s; }, sample);
  if (init) await page.addInitScript(init);
  return { page, errors };
};
const world = (page, f, arg) => page.evaluate(([src, a]) => new Function("w", "a", src)(window.__ocean, a), [`return (${f})(w, a)`, arg]);

try {
  const { page, errors } = await open();
  await page.goto(url);
  await page.waitForFunction(() => window.__ocean && document.querySelector(".oc-dash"), null, { timeout: 30000 });
  assert.equal(await page.locator(".oc-chip b").first().textContent(), "椰影群岛");
  assert.match(await page.locator(".oc-chip").first().textContent(), /开阔海面/);
  assert.equal(await world(page, (w) => w.shapes.length), 8, "every island record is in the world");
  assert.equal(await world(page, (w) => w.fftSea), true, "the FFT sea (this browser renders to float textures)");
  // the picture is not blank: sky above, sea below, differently coloured
  const px = await page.evaluate(() => {
    window.__ocean.frame(1 / 60); // (a WebGL canvas can be read only right after it is drawn)
    const c = document.querySelector(".oc-canvas"), g = document.createElement("canvas");
    g.width = 4; g.height = 4;
    const x = g.getContext("2d"); x.drawImage(c, 0, 0, 4, 4);
    return [...x.getImageData(0, 0, 4, 4).data];
  });
  const sky = px.slice(0, 3), sea = px.slice(48, 51);
  assert.ok(sky[2] > 120 && sea[2] > 40 && Math.abs(sky[0] - sea[0]) + Math.abs(sky[1] - sea[1]) > 30, `sky ${sky} and sea ${sea}`);

  // the keyboard: W pushes the lever, the boat gathers way to the north
  await page.locator(".oc-canvas").click({ position: { x: 600, y: 600 } });
  await page.keyboard.down("KeyW");
  await page.waitForFunction(() => window.__ocean.boat.throttle > 0.6, null, { timeout: 15000 });
  await page.keyboard.up("KeyW");
  await page.waitForFunction(() => window.__ocean.boat.speed > 3, null, { timeout: 20000 });
  assert.match(await page.locator(".oc-gauge-note").textContent(), /油门 \d+%/);
  assert.ok(Number(await page.locator(".oc-speed b").textContent()) > 3, "the speed in knots");
  assert.ok(await world(page, (w) => w.boat.z < 0), "north is −z");
  await page.keyboard.press("Space");
  await page.waitForFunction(() => window.__ocean.boat.throttle === 0);
  assert.equal(await page.locator(".oc-hint").count(), 0, "the keys hint goes once you have moved");

  // shared settings: a rough sea, saved to the page for everyone
  await page.getByRole("button", { name: "海况和天气" }).click();
  await page.locator(".oc-panel .seg button", { hasText: "大浪" }).click();
  await page.waitForFunction(async () => (await window.myco.data.get("world", "main")).data.sea === "rough", null, { timeout: 5000 });
  const kept = await page.evaluate(async () => (await window.myco.data.get("world", "main")).data);
  assert.equal(kept.title, "椰影群岛", "the other settings are kept");
  assert.equal(kept.weather, "cloudy");
  // the day runs on for everyone: a speed and the moment it was set are saved
  await page.locator(".oc-panel .seg button", { hasText: "一天 12 分钟" }).click();
  await page.waitForFunction(async () => (await window.myco.data.get("world", "main")).data.time_speed === 120, null, { timeout: 5000 });
  const clockRec = await page.evaluate(async () => (await window.myco.data.get("world", "main")).data);
  assert.ok(Math.abs(clockRec.time_at - Date.now()) < 60_000 && Math.abs(clockRec.time - 15.5) < 0.6, `the clock starts from the hour shown (${clockRec.time})`);
  await page.waitForFunction(() => window.__ocean.status.hour > 15.55, null, { timeout: 15000 });
  await page.locator(".oc-panel .seg button", { hasText: "停住" }).click();
  // storm: the record, and the chip says so
  await page.locator(".oc-panel .seg button", { hasText: "雷雨" }).click();
  await page.waitForFunction(async () => (await window.myco.data.get("world", "main")).data.weather === "storm", null, { timeout: 5000 });
  await page.waitForFunction(() => /\d\d:\d\d · 雷雨/.test(document.querySelector(".oc-chip").textContent));
  await page.locator(".oc-panel .inp").fill("新群岛");
  await page.waitForFunction(async () => (await window.myco.data.get("world", "main")).data.title === "新群岛", null, { timeout: 5000 });
  await page.keyboard.press("Escape");
  assert.equal(await page.locator(".oc-panel").count(), 0);

  // an island: close by, its card says what is written about it; found once
  await world(page, (w) => w.moveBoat(-300, -300, Math.PI));
  const card = page.locator(".oc-card");
  await card.waitFor({ timeout: 10000 });
  assert.match(await card.textContent(), /椰子岛.*东岸有一片白沙滩/s);
  await card.getByRole("button", { name: "收起" }).click();
  assert.equal(await card.count(), 0, "closed until you leave");
  assert.match(await page.locator(".oc-chip").first().textContent(), /在 椰子岛 附近/);
  // running into it: stopped short of the land, told why
  await world(page, (w) => { w.moveBoat(-330, -300, 0); w.boat.speed = 8; w.boat.throttle = 1; });
  await page.locator(".oc-alert").waitFor({ timeout: 15000 });
  assert.match(await page.locator(".oc-alert").textContent(), /触底了/);
  assert.ok(await world(page, (w) => w.ground(w.boat.x, w.boat.z) < -0.3), "the boat is still in water");
  await world(page, (w) => { w.boat.throttle = 0; w.boat.speed = 0; });

  // the map: every island, the one found counted, a course from it
  await page.locator(".oc-canvas").click({ position: { x: 600, y: 500 } });
  await page.keyboard.press("KeyM");
  const map = page.getByRole("dialog", { name: "地图" });
  await map.waitFor();
  assert.match(await map.textContent(), /到过 1 \/ 8 处/);
  assert.equal(await map.locator(".oc-place").count(), 8, "every named place, nearest first");
  assert.match(await map.locator(".oc-place").first().textContent(), /椰子岛/);
  assert.equal(await map.locator(".oc-place-dot.is-found").count(), 1);
  await map.locator(".oc-place", { hasText: "双子岩" }).click();
  assert.match(await map.locator(".oc-pick").textContent(), /双子岩.*海蚀柱/s);
  await map.getByRole("button", { name: "开往这里" }).click();
  await map.waitFor({ state: "detached" });
  assert.match(await page.locator(".oc-goal").textContent(), /开往 双子岩 · \d/);
  // the compass marks the course
  assert.equal(await page.locator(".oc-course").count(), 1);
  await page.locator(".oc-goal").click();
  assert.equal(await page.locator(".oc-goal").count(), 0, "a click clears the course");

  // the round map and the dash are there; nothing went wrong
  assert.equal(await page.locator(".oc-mini canvas").count(), 1);
  assert.deepEqual(errors, []);
  await page.close();

  // English
  {
    const { page: en, errors: e2 } = await open({ locale: "en-US" });
    await en.goto(url + "?myco_lang=en");
    await en.waitForFunction(() => document.querySelector(".oc-dash"), null, { timeout: 30000 });
    assert.match(await en.locator(".oc-chip").first().textContent(), /Open sea/);
    assert.match(await en.locator(".oc-gauge-note").textContent(), /Neutral/);
    await en.getByRole("button", { name: "Sea and weather" }).click();
    assert.ok(await en.locator(".oc-panel .seg button", { hasText: "Rough" }).isVisible());
    assert.deepEqual(e2, []);
    await en.close();
  }
  // no WebGL: says so instead of a blank page
  {
    const { page: p3 } = await open({}, () => {
      const get = HTMLCanvasElement.prototype.getContext;
      HTMLCanvasElement.prototype.getContext = function (k, ...a) { return /webgl/.test(k) ? null : get.call(this, k, ...a); };
    });
    await p3.goto(url);
    await p3.locator(".oc-fail").waitFor({ timeout: 20000 });
    assert.match(await p3.locator(".oc-fail").textContent(), /打不开 3D 海面.*WebGL/s);
    await p3.close();
  }
  console.log("PASS ocean ui: draws, keyboard, shared settings, island card, aground, map, English, no WebGL");
} finally {
  await browser.close();
  srv.close();
}
