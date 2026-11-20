// Screenshots of the official apps, for the App's new-page dialog: each app
// opened with its sample data (an in-memory bridge stands in for the App)
// in a 1040×650 window (at twice the pixels), light and dark, saved as
// src/<app>/public/shots/<n>.webp and <n>.dark.webp. Run after building
// (node build.mjs), then build again so the shots go into the apps:
//
//   node build.mjs && node shots.mjs [app…] && node build.mjs
//
// Needs Playwright with its Chromium (npm install -g playwright, as the UI
// smoke test does) and, for 视频剪辑, ffmpeg (a clip to edit).
import http from "node:http";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const official = path.join(here, "..", "internal", "templates", "official");

async function playwright() {
  for (const spec of ["playwright", "playwright-core"]) {
    try { const m = await import(spec); return m.chromium ?? m.default.chromium; } catch { /* (try the global ones) */ }
  }
  let root = "";
  try { root = execFileSync(process.platform === "win32" ? "npm.cmd" : "npm", ["root", "-g"], { encoding: "utf8" }).trim(); } catch { /* (no npm) */ }
  for (const dir of [root, ...(process.env.NODE_PATH || "").split(path.delimiter)].filter(Boolean)) {
    for (const spec of ["playwright", "playwright-core"]) {
      try {
        const m = await import(pathToFileURL(createRequire(path.join(dir, "noop.js")).resolve(spec)).href);
        return m.chromium ?? m.default.chromium;
      } catch { /* (not here) */ }
    }
  }
  throw new Error("Playwright is not installed: npm install -g playwright && npx playwright install chromium");
}

// The App's bridge, in memory: the records of the page (the sample's), and
// downloads dropped.
const bridge = `(() => {
  const recs = {}; let seq = 0; const subs = [];
  const rid = () => "r" + Math.random().toString(36).slice(2, 9);
  for (const [c, list] of Object.entries(window.__SAMPLE__ || {})) for (const r of list) { const { id, ...data } = r; const k = id || rid(); recs[c + "/" + k] = { collection: c, id: k, data, version: 1 }; }
  const fakeOS = window.__FAKE_OS__;
  window.myco = {
    features: fakeOS ? ["download-media", "os"] : ["download-media"],
    os: fakeOS,
    ready: Promise.resolve({ page: { id: "pg_shots", name: "shots" }, me: { name: "glenn" } }),
    data: {
      list: async (c) => ({ records: Object.values(recs).filter((r) => !c || r.collection === c).map((r) => ({ ...r })), seq }),
      put: async (c, data, o = {}) => { const id = o.id || rid(); const cur = recs[c + "/" + id]; const r = { collection: c, id, data, version: (cur ? cur.version : 0) + 1 }; recs[c + "/" + id] = r; seq++; return { ...r }; },
      remove: async (c, id) => { delete recs[c + "/" + id]; seq++; return true; },
      on: (f) => { subs.push(f); return () => {}; },
    },
    download: async (name) => ({ path: "~/Downloads/" + name }),
  };
})();`;

function serve() {
  const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".json": "application/json", ".webp": "image/webp", ".png": "image/png", ".woff2": "font/woff2" };
  const srv = http.createServer((req, res) => {
    let p = decodeURIComponent(new URL(req.url, "http://x").pathname);
    if (p.endsWith("/")) p += "index.html";
    if (p === "/__bridge.js") { res.writeHead(200, { "Content-Type": "text/javascript" }); res.end(bridge); return; }
    if (p === "/__os.js") { res.writeHead(200, { "Content-Type": "text/javascript" }); res.end(readFileSync(path.join(here, "shots-os.js"))); return; }
    const f = path.join(official, p);
    if (!f.startsWith(official) || !existsSync(f)) { res.writeHead(404); res.end(); return; }
    let body = readFileSync(f);
    if (f.endsWith(".html")) {
      // (a made-up computer first, for the apps that use one)
      const os = new URL(req.url, "http://x").searchParams.get("os") ? '<script src="/__os.js"></script>' : "";
      body = Buffer.from(String(body).replace("<head>", `<head>${os}<script src="/__bridge.js"></script>`));
    }
    res.writeHead(200, { "Content-Type": types[path.extname(f)] || "application/octet-stream" });
    res.end(body);
  });
  return new Promise((resolve) => srv.listen(0, "127.0.0.1", () => resolve(srv)));
}

/** A clip for 视频剪辑 to show: soft moving gradients, 8 seconds. */
function clip() {
  const out = path.join(tmpdir(), "myco-shots-clip.mp4");
  if (!existsSync(out)) {
    execFileSync("ffmpeg", ["-loglevel", "error", "-y", "-f", "lavfi", "-i", "gradients=s=1280x720:c0=#e9e6e1:c1=#9fb3c8:c2=#c9b8a6:speed=0.02:d=8",
      "-f", "lavfi", "-i", "sine=frequency=220:duration=8", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", out]);
  }
  return out;
}

// What to show of each app: one or more shots, each after some steps (the
// sample data is there before the first).
const wait = (p, ms = 400) => p.waitForTimeout(ms);
const plans = {
  base: [null, async (p) => { await p.locator("text=看板").first().click(); await wait(p, 600); }],
  issues: [async (p) => { await p.locator(".is-row").nth(5).click(); await wait(p, 500); }, async (p) => { await p.keyboard.press("Escape"); await p.click('button[title="看板"]'); await wait(p, 500); }],
  notes: [null, async (p) => { await p.locator('.nt-kid:has-text("发版流程")').click(); await wait(p, 700); }],
  // 空间清理: a made-up Mac (shots-os.js): the disks, then the map with two things to clean
  disk: { os: true, steps: [null, async (p) => {
    await p.locator(".dk-disk .btn-primary").first().click();
    await p.waitForSelector(".sb-seg", { timeout: 10000 });
    await p.locator(".dk-row", { hasText: "Applications" }).hover();
    await p.locator(".dk-row", { hasText: "Applications" }).locator('button[title="加到待清理"]').click();
    await p.locator(".dk-tabs button", { hasText: "最大的文件" }).click();
    await p.locator(".dk-row", { hasText: "Xcode_16.1.xip" }).hover();
    await p.locator(".dk-row", { hasText: "Xcode_16.1.xip" }).locator('button[title="加到待清理"]').click();
    await p.locator(".dk-tabs button", { hasText: "这里有什么" }).click();
    await p.locator(".dk-row", { hasText: "Users" }).hover();
    await wait(p, 400);
  }, async (p) => {
    await p.locator(".dk-row", { hasText: "Users" }).click();
    await p.locator(".dk-row", { hasText: "glenn" }).click();
    await p.locator(".dk-row", { hasText: "Library" }).hover();
    await wait(p, 400);
  }] },
  video: { sample: false, steps: [async (p) => {
    await p.setInputFiles("input[type=file]", clip());
    await p.waitForSelector(".tl-clip", { timeout: 30000 });
    await p.locator(".tl-ruler").click({ position: { x: 60, y: 10 } });
    await p.click('.tab:has-text("文字")'); await p.click('.preset:has-text("标题")');
    await p.locator(".props textarea").fill("三分钟上手 myco");
    await p.locator(".tl-ruler").click({ position: { x: 90, y: 10 } });
    await wait(p, 1500);
  }] },
};

const chromium = await playwright();
const srv = await serve();
const base = `http://127.0.0.1:${srv.address().port}`;
const browser = await chromium.launch();
const only = process.argv.slice(2);
const apps = readdirSync(official).filter((a) => existsSync(path.join(official, a, "myco.json")) && (!only.length || only.includes(a)));
for (const app of apps) {
  const plan = plans[app] || [null];
  const { sample = true, steps = plan, os: fakeOS = false } = Array.isArray(plan) ? {} : plan;
  const dir = path.join(here, "src", app, "public", "shots");
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(dir, { recursive: true });
  for (const scheme of ["light", "dark"]) {
    const ctx = await browser.newContext({ viewport: { width: 1040, height: 650 }, deviceScaleFactor: 2, colorScheme: scheme, locale: "zh-CN" });
    const page = await ctx.newPage();
    const data = sample ? JSON.parse(readFileSync(path.join(official, app, "sample.json"), "utf8")) : {};
    await page.addInitScript((s) => { window.__SAMPLE__ = s; try { localStorage.clear(); } catch { /* (none) */ } }, data);
    await page.goto(`${base}/${app}/${fakeOS ? "?os=1" : ""}`);
    await page.waitForTimeout(2500);
    for (let n = 0; n < steps.length; n++) {
      if (steps[n]) await steps[n](page);
      const png = await page.screenshot();
      // (WebP, made by the browser itself: no other tool needed)
      const webp = await page.evaluate(async (b64) => {
        const img = new Image();
        img.src = `data:image/png;base64,${b64}`;
        await img.decode();
        const c = document.createElement("canvas");
        c.width = img.width; c.height = img.height;
        c.getContext("2d").drawImage(img, 0, 0);
        const blob = await new Promise((r) => c.toBlob(r, "image/webp", 0.82));
        const buf = new Uint8Array(await blob.arrayBuffer());
        let s = "";
        for (let i = 0; i < buf.length; i += 0x8000) s += String.fromCharCode(...buf.subarray(i, i + 0x8000));
        return btoa(s);
      }, png.toString("base64"));
      const file = path.join(dir, `${n + 1}${scheme === "dark" ? ".dark" : ""}.webp`);
      writeFileSync(file, Buffer.from(webp, "base64"));
      console.log(`${app}: ${path.relative(here, file)} (${Math.round(Buffer.from(webp, "base64").length / 1024)} KB)`);
    }
    await ctx.close();
  }
}
await browser.close();
srv.close();
