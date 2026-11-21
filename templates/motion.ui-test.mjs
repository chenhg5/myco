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

  // ---- step 2: notes on a moment, the shot plan, a reference video ----
  const tmp = fs.mkdtempSync(path.join(fs.realpathSync("/tmp"), "motion-"));
  await at(5.2, () => 0);
  await page.getByRole("button", { name: "批注", exact: true }).first().click();
  await page.getByPlaceholder(/留一条批注/).fill("这里慢一点");
  await page.keyboard.press("Enter");
  await page.waitForFunction(() => document.querySelectorAll(".mo-pin").length === 1);
  assert.ok(await page.locator(".mo-note-text", { hasText: "这里慢一点" }).isVisible(), "the note in the list");
  assert.equal(await page.locator(".mo-tabs .mo-dot").count(), 1, "an open note on the tab");
  assert.match(await page.locator(".mo-note-time").first().textContent(), /00:05\.[12]/);
  await page.getByRole("tab", { name: "分镜" }).click();
  await page.getByRole("button", { name: /写分镜|改/ }).first().click();
  await page.locator(".mo-plan-ta").fill("## 1. 开头 0–4 秒\n- **大标题**飞入\n- 文字：记账，拍一张就好");
  await page.getByRole("button", { name: "保存" }).click();
  assert.equal(await page.locator(".mo-md h3").textContent(), "1. 开头 0–4 秒");
  assert.equal(await page.locator(".mo-md li b").textContent(), "大标题");
  // three shots of two seconds (red, green, blue): a frame of each
  const ref = path.join(tmp, "ref.webm");
  execFileSync("ffmpeg", ["-v", "error", "-f", "lavfi", "-i", "color=c=red:s=320x180:d=2:r=25", "-f", "lavfi", "-i", "color=c=green:s=320x180:d=2:r=25", "-f", "lavfi", "-i", "color=c=blue:s=320x180:d=2:r=25",
    "-filter_complex", "[0:v][1:v][2:v]concat=n=3:v=1[v]", "-map", "[v]", "-c:v", "libvpx", "-b:v", "300k", ref]);
  await page.locator('input[type=file][accept="video/*"]').setInputFiles(ref);
  await page.waitForFunction(() => document.querySelectorAll(".mo-ref-frames img").length > 0, null, { timeout: 60000 });
  const shots = await page.$$eval(".mo-ref-frames figure", (fs) => fs.map((f) => {
    const img = f.querySelector("img"), c = document.createElement("canvas");
    c.width = 8; c.height = 8; const g = c.getContext("2d"); g.drawImage(img, 0, 0, 8, 8);
    const [r, gg, b] = g.getImageData(4, 4, 1, 1).data;
    return { time: f.querySelector("figcaption").textContent, colour: r > 150 ? "red" : gg > 90 ? "green" : b > 150 ? "blue" : "?" };
  }));
  for (const c of ["red", "green", "blue"]) assert.ok(shots.some((x) => x.colour === c), `a frame of the ${c} shot: ${JSON.stringify(shots)}`);
  assert.ok(shots.some((x) => /00:02\.[0-9]/.test(x.time)) && shots.some((x) => /00:04\.[0-9]/.test(x.time)), `frames where shots change: ${JSON.stringify(shots)}`);
  assert.ok(await page.getByRole("button", { name: /让 agent 照着做|复制给 agent 的话/ }).isVisible());

  // ---- step 3: music (beats, cut on the beat), captions ----
  // 24 s of clicks at 120 BPM from 0.25 s, the louder every fourth
  const rate = 44100, secs = 24, n = rate * secs, wav = Buffer.alloc(44 + n * 2);
  wav.write("RIFF", 0); wav.writeUInt32LE(36 + n * 2, 4); wav.write("WAVEfmt ", 8); wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20); wav.writeUInt16LE(1, 22);
  wav.writeUInt32LE(rate, 24); wav.writeUInt32LE(rate * 2, 28); wav.writeUInt16LE(2, 32); wav.writeUInt16LE(16, 34); wav.write("data", 36); wav.writeUInt32LE(n * 2, 40);
  for (let i = 0; i < n; i++) {
    const t = i / rate, k = Math.round((t - 0.25) / 0.5), dt = t - 0.25 - k * 0.5;
    const click = k >= 0 && dt >= 0 && dt < 0.06 ? Math.sin(2 * Math.PI * (k % 4 ? 900 : 120) * dt) * Math.exp(-dt * 60) * (k % 4 ? 0.5 : 0.9) : 0;
    const pad = 0.04 * Math.sin(2 * Math.PI * 220 * t);
    wav.writeInt16LE(Math.round((click + pad) * 30000), 44 + i * 2);
  }
  const music = path.join(tmp, "beat.wav");
  fs.writeFileSync(music, wav);
  await page.getByRole("tab", { name: "声音" }).click();
  await page.locator('input[type=file][accept^="audio/*"]').first().setInputFiles(music);
  await page.waitForSelector(".mo-trackcard", { timeout: 30000 });
  await page.waitForFunction(() => /拍每分钟/.test(document.querySelector(".mo-beat")?.textContent || ""), null, { timeout: 30000 });
  const bpm = Number((await page.locator(".mo-beat .faint").textContent()).match(/([\d.]+) 拍每分钟/)[1]);
  assert.ok(Math.abs(bpm - 120) < 2, `tempo: ${bpm}`);
  assert.ok(await page.locator(".mo-tblock .mo-wave").count() === 1, "the track's waveform on the timeline");
  await page.getByRole("button", { name: "切换对上节拍" }).click();
  await page.waitForTimeout(200);
  const ends = await page.evaluate(async () => {
    const rows = (await window.myco.data.list("scenes")).records.sort((a, b) => a.data.order - b.data.order);
    let t = 0;
    return rows.map((r) => Math.round((t += r.data.duration) * 1000) / 1000);
  });
  for (const e of ends.slice(0, -1)) { const off = (e - 0.25) / 0.5; assert.ok(Math.abs(off - Math.round(off)) < 0.06, `a cut on the beat: ${e} (${ends})`); }
  // a caption at 1.5 s, drawn over the scene
  await at(1.5, () => 0);
  await page.getByRole("button", { name: /^在 .* 加一条$/ }).click();
  const cap = page.locator(".mo-cap-row input").first();
  await cap.fill("拍一张，记好账");
  await cap.press("Enter");
  await page.waitForTimeout(300);
  assert.equal(await at(2, () => { const c = document.querySelector(".mo-cap"); return c.style.display === "none" ? "" : c.textContent; }), "拍一张，记好账");
  assert.equal(await at(5, () => document.querySelector(".mo-cap").style.display), "none");
  assert.equal(await page.locator(".mo-capblock").count(), 1, "the caption on the timeline");

  // ---- step 4: save as a template, use it ----
  await page.getByRole("tab", { name: "视频" }).click();
  await page.getByRole("button", { name: "把这个视频存成模板" }).click();
  await page.getByPlaceholder("模板的名字").fill("记账宣传");
  await page.getByRole("button", { name: "存", exact: true }).click();
  await page.waitForSelector(".mo-libitem");
  assert.match(await page.locator(".mo-libitem").textContent(), /记账宣传.*6 个场景/);
  const before = await page.locator(".mo-card").count();
  await page.locator(".mo-libitem").getByRole("button", { name: "用这个" }).click();
  await page.waitForTimeout(300);
  assert.equal(await page.locator(".mo-card").count(), before, "the template's scenes in place of these");
  await page.locator(".mo-card").first().click({ button: "right" });
  await page.getByRole("button", { name: "存成场景模板" }).click();
  await page.locator(".mo-add").click();
  assert.equal(await page.locator(".mo-tpl.is-saved").count(), 1, "the saved scene in the picker");
  await page.keyboard.press("Escape");
  await page.waitForFunction(() => document.querySelectorAll(".mo-thumb img").length === 6, null, { timeout: 30000 });

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
  const file = path.join(tmp, "out.mp4");
  fs.writeFileSync(file, mp4);
  try {
    const info = JSON.parse(execFileSync("ffprobe", ["-v", "error", "-show_entries", "stream=codec_type,codec_name,width,height,nb_frames,duration", "-of", "json", file], { encoding: "utf8" }));
    const v = info.streams.find((x) => x.codec_type === "video"), a = info.streams.find((x) => x.codec_type === "audio");
    const T = ends[ends.length - 1];
    assert.deepEqual([v.width, v.height], [1280, 720]);
    assert.ok(Math.abs(Number(v.nb_frames) - Math.round(T * 24)) <= 1, `frames: ${v.nb_frames} for ${T} s`);
    assert.ok(a && Math.abs(Number(a.duration) - T) < 0.2, `the music in the mp4: ${JSON.stringify(a)}`);
  } catch (e) {
    if (e.code !== "ENOENT") throw e; // (no ffprobe: the header check is enough)
  }
  // the mp4 as a reference video: a frame of each of its scenes (fades and
  // slides between dark scenes too), after each has settled
  if (fs.existsSync(file)) {
    const webm = path.join(tmp, "self.webm");
    execFileSync("ffmpeg", ["-v", "error", "-i", file, "-vf", "scale=480:-2", "-c:v", "libvpx", "-b:v", "600k", "-an", webm]);
    await page.getByRole("button", { name: "好" }).click();
    await page.getByRole("tab", { name: "分镜" }).click();
    await page.locator('input[type=file][accept="video/*"]').setInputFiles(webm);
    await page.waitForFunction(() => /^00:0[0-1]\./.test(document.querySelector(".mo-ref-frames figcaption")?.textContent || "") && !document.querySelector(".mo-taking"), null, { timeout: 90000 });
    const got = (await page.$$eval(".mo-ref-frames figcaption", (f) => f.map((x) => x.textContent))).map((x) => { const [m, s] = x.split(":"); return +m * 60 + +s; });
    // (a frame of each scene once its transition is over)
    const starts = [0, ...ends.slice(0, -1)];
    for (let i = 0; i < starts.length; i++) {
      assert.ok(got.some((t) => t >= starts[i] + 0.6 && t < ends[i]), `a frame of scene ${i + 1} (${starts[i]}–${ends[i]}): ${got}`);
    }
  }
  assert.deepEqual(errors, []);
  console.log(`PASS sample, sandboxed frame, motion at given times; a note, the shot plan, a reference video's shots; music at ${bpm} BPM, cuts on the beat, a caption; templates; ${(mp4.length / 1e6).toFixed(1)} MB 720p mp4 with sound`);
} finally {
  await browser.close();
  srv.close();
}
