// Real HLS decoding under the App CSP, plus source setup, catalog/search,
// watchlist and local resume. Every upstream response is a synthetic fixture.
import assert from "node:assert/strict";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
const here = path.dirname(fileURLToPath(import.meta.url)),
  root = path.resolve(here, "../internal/templates/official/cinema");
const fixture = process.env.CINEMA_FIXTURE || "/tmp/myco-cinema-fixture",
  out = process.env.CINEMA_SHOTS || "/tmp/cinema-shots";
fs.mkdirSync(fixture, { recursive: true });
fs.mkdirSync(out, { recursive: true });
if (!fs.existsSync(path.join(fixture, "index.m3u8")))
  execFileSync("ffmpeg", [
    "-hide_banner",
    "-loglevel",
    "error",
    "-y",
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=640x360:rate=24",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=220:sample_rate=44100",
    "-t",
    "16",
    "-c:v",
    "libx264",
    "-preset",
    "ultrafast",
    "-g",
    "48",
    "-pix_fmt",
    "yuv420p",
    "-c:a",
    "aac",
    "-hls_time",
    "2",
    "-hls_list_size",
    "0",
    path.join(fixture, "index.m3u8"),
  ]);
const bridgeSource = fs.readFileSync(path.join(here, "shots.mjs"), "utf8");
const bridge = bridgeSource
  .slice(
    bridgeSource.indexOf("const bridge = `") + 16,
    bridgeSource.indexOf("})();`;") + 5,
  )
  .replace(
    "seq++; return { ...r };",
    'seq++; for (const f of subs) f({seq,collection:c,record:id,op:"put",data,version:r.version}); return { ...r };',
  );
const csp =
  "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' data: blob:; connect-src 'self' data: blob:; worker-src 'self' blob:; object-src 'none'";
const source = {
  name: "我的片库",
  endpoint: "https://catalog.test/api",
  enabled: true,
  order: 1,
};
const movie = (id) => ({
  vod_id: id,
  vod_name: id === "1" ? "山间来信" : "城市的另一面",
  vod_pic: "https://media.test/poster.png",
  vod_year: "2026",
  type_name: "剧集",
  vod_remarks: "全 2 集",
  vod_actor: "示例演员",
  vod_content:
    '<b>一段走进山野的旅程。</b><img src=x onerror="window.__injected=1">',
  vod_play_from: "HLS$$$备用",
  vod_play_url:
    "第01集$https://media.test/start.m3u8#第02集$https://media.test/hls/index.m3u8?episode=2$$$第02集$https://media.test/hls/index.m3u8?line=2",
});
const pixel = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jJmkAAAAASUVORK5CYII=",
  "base64",
);
const server = http.createServer((req, res) => {
  const u = new URL(req.url, "http://test"),
    p = u.pathname;
  if (p === "/favicon.ico") {
    res.writeHead(204);
    res.end();
    return;
  }
  if (p === "/fixture") {
    const remote = new URL(u.searchParams.get("url"));
    let body,
      type = "application/json",
      url = remote.href,
      status = 200;
    if (remote.hostname === "broken.test") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          status: 503,
          body: "",
          headers: {},
          encoding: "utf8",
        }),
      );
      return;
    }
    if (remote.hostname === "catalog.test")
      body = Buffer.from(
        JSON.stringify({
          page: Number(remote.searchParams.get("pg") || 1),
          pagecount: 2,
          class: [{ type_id: "9", type_name: "剧集" }],
          list:
            remote.searchParams.get("wd") === "missing"
              ? []
              : remote.searchParams.has("ids")
                ? [movie(remote.searchParams.get("ids"))]
                : [movie("1"), movie("2")],
        }),
      );
    else if (remote.pathname === "/poster.png") {
      body = pixel;
      type = "image/png";
    } else {
      const name = path.basename(
        remote.pathname === "/"
          ? "index.m3u8"
          : remote.pathname === "\/start.m3u8"
            ? "index.m3u8"
            : remote.pathname,
      );
      body = fs.readFileSync(path.join(fixture, name));
      type = name.endsWith(".m3u8")
        ? "application/vnd.apple.mpegurl"
        : "video/mp2t";
      if (remote.pathname === "/start.m3u8")
        url = "https://media.test/hls/index.m3u8";
    }
    const binary = u.searchParams.get("binary") === "1";
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        status,
        url,
        headers: { "content-type": type },
        body: binary ? body.toString("base64") : body.toString("utf8"),
        encoding: binary ? "base64" : "utf8",
      }),
    );
    return;
  }
  if (p === "/bridge.js") {
    res.writeHead(200, { "Content-Type": "text/javascript" });
    res.end(bridge);
    return;
  }
  const f = path.resolve(root, "." + (p === "/" ? "/index.html" : p));
  if (!f.startsWith(root + path.sep) || !fs.existsSync(f)) {
    res.writeHead(404);
    res.end();
    return;
  }
  const types = {
    ".html": "text/html",
    ".js": "text/javascript",
    ".css": "text/css",
    ".svg": "image/svg+xml",
    ".json": "application/json",
    ".webp": "image/webp",
  };
  res.setHeader(
    "Content-Type",
    types[path.extname(f)] || "application/octet-stream",
  );
  res.setHeader("Content-Security-Policy", csp);
  let body = fs.readFileSync(f);
  if (f.endsWith(".html"))
    body = Buffer.from(
      body
        .toString()
        .replace("<head>", '<head><script src="/bridge.js"></script>'),
    );
  res.end(body);
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
let playwright;
for (const directory of [
  here,
  execFileSync(
    process.platform === "win32" ? "npm.cmd" : "npm",
    ["root", "-g"],
    { encoding: "utf8" },
  ).trim(),
]) {
  try {
    playwright = createRequire(path.join(directory, "noop.js"))("playwright");
    break;
  } catch {
    /* Try the next installed location. */
  }
}
if (!playwright)
  throw new Error("Install Playwright before running the cinema UI test");
const browser = await playwright.chromium.launch({
  headless: true,
  executablePath: process.env.UI_SMOKE_EXECUTABLE,
  args: ["--no-sandbox"],
});
try {
  for (const [lang, theme, width] of [
    ["zh", "light", 1400],
    ["zh", "dark", 760],
    ["en", "light", 760],
    ["en", "dark", 1400],
  ]) {
    const page = await browser.newPage({
      viewport: { width, height: 900 },
      colorScheme: theme,
    });
    const errors = [],
      network = [];
    page.on("pageerror", (e) => errors.push(e.message));
    page.on("console", (m) => {
      if (m.type() === "error") console.log("BROWSER", m.text());
    });
    page.on("request", (r) => network.push(r.url()));
    await page.addInitScript(
      ({ lang, theme }) => {
        window.__SAMPLE__ = {};
        window.__requests = [];
        window.__FAKE_OS__ = {
          net: {
            fetch: async (args) => {
              window.__requests.push(args);
              if (window.__fail) throw new Error("fixture failure");
              const r = await fetch(
                "/fixture?url=" +
                  encodeURIComponent(args.url) +
                  "&binary=" +
                  (args.response === "base64" ? "1" : "0"),
              );
              return r.json();
            },
          },
        };
        document.addEventListener("DOMContentLoaded", () => {
          document.documentElement.setAttribute("data-myco-theme", theme);
        });
      },
      { lang, theme },
    );
    const L = (zh, en) => (lang === "zh" ? zh : en);
    await page.goto(
      `http://127.0.0.1:${server.address().port}/?myco_lang=${lang}`,
    );
    await page
      .getByRole("heading", {
        name: L("先添加你的影视源", "Add your first source"),
      })
      .waitFor();
    assert.equal(
      await page.evaluate(() => window.__requests.length),
      0,
      "first use makes no source requests",
    );
    await page.screenshot({
      path: path.join(out, `${lang}-${theme}-${width}-empty.png`),
    });
    await page
      .getByRole("button", { name: L("添加影视源", "Add source"), exact: true })
      .click();
    await page
      .getByRole("textbox", { name: L("影视源名称", "Source name") })
      .fill("我的片库");
    await page
      .getByRole("textbox", { name: L("接口地址", "API address"), exact: true })
      .fill("https://catalog.test/api?key=secret");
    await page
      .getByRole("button", {
        name: L("测试连接", "Test connection"),
        exact: true,
      })
      .click();
    await page.getByRole("alert").waitFor();
    assert.equal(
      await page.evaluate(() => window.__requests.length),
      0,
      "reject credentials before contacting a source",
    );
    await page
      .getByRole("textbox", { name: L("接口地址", "API address"), exact: true })
      .fill(source.endpoint);
    await page
      .getByRole("button", {
        name: L("测试连接", "Test connection"),
        exact: true,
      })
      .click();
    await page
      .getByText(L("接口可用", "API is available"), { exact: true })
      .waitFor();
    await page.screenshot({
      path: path.join(out, `${lang}-${theme}-${width}-sources.png`),
    });
    await page
      .getByRole("button", {
        name: L("保存影视源", "Save source"),
        exact: true,
      })
      .click();
    await page
      .getByRole("button", { name: L("关闭", "Close"), exact: true })
      .click();
    await page.locator(".ci-film").first().waitFor();
    assert.equal(await page.locator(".ci-film").count(), 2);
    await page.screenshot({
      path: path.join(out, `${lang}-${theme}-${width}-catalog.png`),
    });
    await page
      .getByRole("textbox", {
        name: L("搜索影片", "Search films"),
        exact: true,
      })
      .fill("missing");
    await page
      .getByRole("button", { name: L("搜索", "Search"), exact: true })
      .click();
    await page
      .getByRole("heading", { name: L("没有找到影片", "No films found") })
      .waitFor();
    await page
      .getByRole("button", { name: L("清除搜索", "Clear search"), exact: true })
      .click();
    await page.locator(".ci-film").first().waitFor();
    await page.locator(".ci-film-open").first().click();
    await page.locator("video").waitFor();
    await page
      .waitForFunction(
        () => document.querySelector("video").readyState >= 2,
        null,
        { timeout: 15000 },
      )
      .catch(async (e) => {
        console.log(
          "DEBUG",
          await page.evaluate(() => ({
            requests: window.__requests.map((r) => r.url),
            body: document.body.innerText,
            errors: document.querySelector("video")?.error?.message,
          })),
        );
        await page.screenshot({ path: "/tmp/cinema-failure.png" });
        throw e;
      });
    await page
      .getByRole("button", {
        name: L("加入待看", "Add to watchlist"),
        exact: true,
      })
      .click();
    assert.equal(await page.evaluate(() => window.__injected), undefined);
    await page.locator("video").evaluate(async (v) => {
      v.muted = true;
      await v.play();
    });
    await page.waitForFunction(
      () => document.querySelector("video").currentTime > 1,
      null,
      { timeout: 15000 },
    );
    await page.locator("video").evaluate((v) => {
      v.currentTime = 6;
      v.pause();
    });
    await page
      .getByRole("combobox", { name: L("播放速度", "Playback speed") })
      .selectOption("1.5");
    assert.equal(
      await page.locator("video").evaluate((v) => v.playbackRate),
      1.5,
    );
    await page.screenshot({
      path: path.join(out, `${lang}-${theme}-${width}-player.png`),
    });
    await page
      .getByRole("button", { name: L("关闭", "Close"), exact: true })
      .click();
    await page
      .getByRole("tab", { name: L("最近播放", "Recently played"), exact: true })
      .click();
    assert.equal(await page.locator(".ci-film").count(), 1);
    await page.locator(".ci-film-open").first().click();
    await page.waitForFunction(
      () => document.querySelector("video")?.currentTime >= 5,
      null,
      { timeout: 30000 },
    );
    await page
      .getByRole("button", { name: L("下一集", "Next episode"), exact: true })
      .click();
    await page.waitForFunction(
      () => document.querySelector("video")?.readyState >= 2,
      null,
      { timeout: 30000 },
    );
    await page
      .getByRole("combobox", {
        name: L("播放线路", "Playback line"),
        exact: true,
      })
      .selectOption("1");
    await page.waitForFunction(
      () => document.querySelector("video")?.readyState >= 2,
      null,
      { timeout: 30000 },
    );
    assert.equal(
      await page
        .getByRole("button", { name: "第02集", exact: true })
        .getAttribute("aria-pressed"),
      "true",
    );
    await page
      .getByRole("button", { name: L("关闭", "Close"), exact: true })
      .click();
    await page
      .getByRole("tab", { name: new RegExp(L("待看", "Watchlist")) })
      .click();
    assert.equal(await page.locator(".ci-film").count(), 1);
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
      "no horizontal overflow",
    );
    const requests = await page.evaluate(() =>
      window.__requests.map((r) => r.url),
    );
    assert(
      requests.some((u) => u.includes("/hls/index0.ts")),
      "redirected playlist uses its final base URL",
    );
    assert(
      network.every(
        (u) => u.startsWith("blob:") || new URL(u).hostname === "127.0.0.1",
      ),
      "no direct upstream connections",
    );
    if (lang === "en" && theme === "dark") {
      // An agent edits these same records through page data commands.
      await page.evaluate(async () => {
        const r = (await window.myco.data.list("sources")).records[0];
        await window.myco.data.put(
          "sources",
          { ...r.data, name: "Agent catalog", enabled: false },
          { id: r.id, ifVersion: r.version },
        );
      });
      await page.getByRole("tab", { name: "Discover", exact: true }).click();
      await page
        .getByRole("heading", { name: "No sources are enabled" })
        .waitFor();
      await page.evaluate(async () => {
        const r = (await window.myco.data.list("sources")).records[0];
        await window.myco.data.put(
          "sources",
          { ...r.data, enabled: true },
          { id: r.id, ifVersion: r.version },
        );
      });
      await page.locator(".ci-film").first().waitFor();
      assert.equal(
        (await page
          .getByRole("combobox", { name: "Choose source" })
          .inputValue()) !== "",
        true,
      );
      console.log("PASS live agent source edits and enable/disable");
    }
    assert.deepEqual(errors, []);
    await page.close();
    console.log(
      `PASS ${lang}/${theme}/${width}: empty defaults, setup, catalog, search, real HLS, watchlist, local resume, line/episode selection, sandbox and layout`,
    );
  }
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
