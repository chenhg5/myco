// 动效视频 in a browser: the built app with its sample, an in-memory myco
// bridge (as shots.mjs), the sandboxed scene frame moving to given times,
// and a real mp4 export. Run: node motion.ui-test.mjs (after npm run build).
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";

const here = path.dirname(fileURLToPath(import.meta.url)),
  official = path.resolve(here, "../internal/templates/official");
const shots = fs.readFileSync(path.join(here, "shots.mjs"), "utf8");
const bridge = shots.slice(shots.indexOf("const bridge = `") + 16, shots.indexOf("})();`;") + 5);
const sample = JSON.parse(fs.readFileSync(path.join(official, "motion/sample.json"), "utf8"));

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

function playwright() {
  for (const from of [here, "/opt/node-tools/node_modules/"]) {
    try { return createRequire(path.join(from, "x.js"))("playwright").chromium; } catch { /* (next) */ }
  }
  throw new Error("playwright is not installed");
}
const browser = await playwright().launch(process.env.UI_SMOKE_EXECUTABLE ? { executablePath: process.env.UI_SMOKE_EXECUTABLE } : {});
const page = await browser.newPage({ viewport: { width: 1400, height: 860 }, locale: "zh-CN" });
const errors = [];
page.on("pageerror", (e) => errors.push(e.message));
page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
await page.addInitScript((s) => { window.__SAMPLE__ = s; }, sample);
try {
  await page.goto(`http://127.0.0.1:${srv.address().port}/motion/`);
  await page.waitForFunction(() => document.querySelectorAll(".mo-thumb img").length === 6, null, { timeout: 30000 });
  assert.equal(await page.locator(".mo-card").count(), 6, "the sample's six scenes");

  // the preview frame: sandboxed (no myco bridge), moved to given times
  const frame = page.frames().find((f) => f !== page.mainFrame());
  assert.ok(frame, "the scene frame");
  assert.equal(await page.locator("iframe.mo-frame").first().getAttribute("sandbox"), "allow-scripts");
  assert.equal(await frame.evaluate(() => typeof window.myco), "undefined", "scene code cannot reach the bridge");
  const at = async (t, f) => {
    const box = await page.locator(".mo-tl").boundingBox();
    await page.mouse.click(box.x + box.width * (t / 28), box.y + 30);
    await page.waitForTimeout(150);
    return frame.evaluate(f);
  };
  // 数据 (9–14 s): the number counts up from 0 to 12,800 by 2 s into the scene
  const early = Number((await at(9.25, () => document.querySelector('[data-scene="s3"] .num').textContent)).replace(/,/g, ""));
  const late = await at(13.5, () => document.querySelector('[data-scene="s3"] .num').textContent);
  assert.ok(early >= 0 && early < 12800, `counting: ${early}`);
  assert.equal(late, "12,800");
  // a bar grows (scaleY from 0) and holds
  const bar = (t) => at(t, () => getComputedStyle(document.querySelectorAll('[data-scene="s3"] .bar i')[3]).transform);
  assert.match(await bar(9.3), /matrix\(1, 0, 0, 0(\.0\d+)?, 0, 0\)|scale\(1, 0/);
  assert.equal(await bar(13.5), "none", "full height at the end");
  // a transition: 0.3 s into 三个要点 (slide), both scenes are shown
  const shown = await at(4.3, () => [...document.querySelectorAll(".scene")].filter((s) => s.style.display !== "none").map((s) => s.dataset.scene));
  assert.deepEqual(shown, ["s1", "s2"]);

  // export: a real mp4, 720p, 24 frames a second
  await page.evaluate(() => {
    window.myco.download = async (name, blob) => {
      const b = new Uint8Array(await blob.arrayBuffer());
      let s = ""; for (let i = 0; i < b.length; i += 32768) s += String.fromCharCode.apply(null, b.subarray(i, i + 32768));
      window.__mp4 = btoa(s);
      return { path: "~/Downloads/" + name };
    };
  });
  await page.getByRole("button", { name: "导出" }).click();
  await page.getByRole("radio", { name: "720p" }).click();
  await page.getByRole("radio", { name: "24" }).click();
  await page.getByTestId("export-start").click();
  await page.waitForFunction(() => window.__mp4 || document.querySelector(".modal-err"), null, { timeout: 120000 });
  assert.equal(await page.locator(".modal-err").count(), 0, await page.locator(".modal-err").first().textContent().catch(() => ""));
  const mp4 = Buffer.from(await page.evaluate(() => window.__mp4), "base64");
  assert.equal(mp4.subarray(4, 8).toString(), "ftyp", "an mp4");
  const file = path.join(fs.mkdtempSync(path.join(fs.realpathSync("/tmp"), "motion-")), "out.mp4");
  fs.writeFileSync(file, mp4);
  try {
    const info = JSON.parse(execFileSync("ffprobe", ["-v", "error", "-show_entries", "stream=width,height,nb_frames", "-of", "json", file], { encoding: "utf8" }));
    const v = info.streams[0];
    assert.deepEqual([v.width, v.height, Number(v.nb_frames)], [1280, 720, 28 * 24]);
  } catch (e) {
    if (e.code !== "ENOENT") throw e; // (no ffprobe: the header check is enough)
  }
  assert.deepEqual(errors, []);
  console.log(`PASS sample loads, sandboxed frame, count/scale/transition at given times, ${(mp4.length / 1e6).toFixed(1)} MB 720p mp4`);
} finally {
  await browser.close();
  srv.close();
}
