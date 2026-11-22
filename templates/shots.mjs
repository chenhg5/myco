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
    features: [...(fakeOS ? ["download-media", "os"] : ["download-media"]), ...(window.__REMOTE_FIXTURE__ ? ["remote"] : [])],
    remote: window.__REMOTE_FIXTURE__ ? {call: async action => action === "devices" ? [{machine:"mac",name:"Studio Mac",os:"darwin",control:true},{machine:"win",name:"Windows 工作站",os:"windows",control:true},{machine:"mini",name:"Mac mini",os:"darwin",control:false}] : {protocol:1,version:"0.1.0",supported:true,installed:false,running:false}} : undefined,
    os: fakeOS,
    ...(location.pathname.startsWith("/tutor/") ? { send: async () => ({ id: "msg_preview" }) } : {}),
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

/** A photo for 修图 to show: MYCO_SHOTS_PHOTO, or a landscape from Unsplash
 *  (free to use), kept in the temp folder. */
function photo() {
  if (process.env.MYCO_SHOTS_PHOTO) return process.env.MYCO_SHOTS_PHOTO;
  const out = path.join(tmpdir(), "myco-shots-photo.jpg");
  if (!existsSync(out)) {
    execFileSync("curl", ["-sSfL", "--max-time", "60", "-o", out, "https://images.unsplash.com/photo-1506744038136-46273834b3fb?w=2400&q=85&fm=jpg"]);
  }
  return out;
}

// What to show of each app: one or more shots, each after some steps (the
// sample data is there before the first).
const wait = (p, ms = 400) => p.waitForTimeout(ms);
const plans = {
  tutor: [null, async (p) => {
    await p.locator(".tu-example-link").click();
    await p.getByRole("button", { name: "中学基础", exact: true }).click();
    await wait(p);
    await p.locator(".tu-figure").scrollIntoViewIfNeeded();
  }],
  scheduling: [async (p) => { await p.getByRole("combobox", { name: "时区", exact: true }).selectOption("Asia/Shanghai"); await wait(p); }, async (p) => { await p.getByRole("combobox", { name: "日历视图" }).selectOption("week"); await p.getByRole("button", { name: "下一页", exact: true }).click(); await wait(p, 600); }],
  cinema: [null, async (p) => { await p.getByRole("button", {name:"添加影视源", exact:true}).click(); await wait(p); }],
  "remote-desktop": [null, async (p) => { await p.locator(".rd-tabs button").nth(1).click(); await wait(p,300); }],
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
  // 会议记录: a meeting's transcript (the sample), then recording one on a made-up Mac
  meeting: { os: true, steps: [async (p) => { await wait(p, 600); }, async (p) => {
    await p.evaluate(() => { window.__REC_AT__ = 1468; });
    await p.click(".mt-item.is-new"); await wait(p, 600);
    await p.click(".mt-go"); await wait(p, 5200);
  }] },
  // 修图: a landscape with a filter and a title, then in a polaroid frame with a sticker
  photo: { sample: false, steps: [async (p) => {
    // (named here: a path that is not ASCII does not reach the page)
    await p.setInputFiles("input[type=file]", { name: "山谷.jpg", mimeType: "image/jpeg", buffer: readFileSync(photo()) });
    await p.waitForSelector(".ph-canvas", { timeout: 30000 });
    await p.click('.ph-tool:has-text("滤镜")'); await wait(p, 1500);
    await p.click('.ph-filter:has-text("日系")');
    await p.click('.ph-tool:has-text("文字")'); await p.click('.ph-chip:has-text("标题")');
    await p.locator(".ph-text").fill("周末的山谷");
    await p.locator(".ph-text").blur();
    await wait(p, 1200);
  }, async (p) => {
    await p.keyboard.press("Escape");
    await p.click('.ph-tool:has-text("贴纸")'); await p.click('.ph-sticker:has-text("🌈")');
    // (up to the right, off the title)
    const b = await p.locator(".ph-sel").boundingBox();
    await p.mouse.move(b.x + b.width / 2, b.y + b.height / 2); await p.mouse.down();
    await p.mouse.move(b.x + b.width / 2 + 230, b.y + b.height / 2 - 70, { steps: 8 }); await p.mouse.up();
    await p.keyboard.press("Escape");
    await p.click('.ph-tool:has-text("边框")'); await p.click('.ph-frame:has-text("拍立得")');
    await wait(p, 1500);
  }] },
  // 动效视频: the numbers scene of the sample promo, then its image scene with the video's colours
  motion: [async (p) => {
    await wait(p, 1500);
    await p.locator(".mo-card").nth(2).click(); await wait(p, 1800);
  }, async (p) => {
    await p.locator(".mo-card").nth(3).click(); await wait(p, 600);
    await p.click('.mo-tabs button:has-text("视频")'); await wait(p, 1200);
  }],
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
const browser = await chromium.launch(process.env.UI_SMOKE_EXECUTABLE ? {executablePath:process.env.UI_SMOKE_EXECUTABLE} : {});
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
    await page.addInitScript((s) => { window.__SAMPLE__ = s; window.__REMOTE_FIXTURE__ = location.pathname.includes("remote-desktop"); try { localStorage.clear(); } catch { /* (none) */ } }, data);
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
