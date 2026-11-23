// Exercise the built page under the real sandbox policy with a range-capable
// synthetic MP3 and a network bridge fixture. No publisher content is copied.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
const require = createRequire(
  path.join(
    execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(),
    "noop.js",
  ),
);
const { chromium } = require("playwright");
const root = path.resolve("internal/templates/official/podcast");
const out = process.env.PODCAST_SHOTS || "/tmp/podcast-shots";
fs.mkdirSync(out, { recursive: true });
const mp3 = execFileSync("ffmpeg", [
  "-hide_banner",
  "-loglevel",
  "error",
  "-f",
  "lavfi",
  "-i",
  "sine=frequency=220:sample_rate=44100",
  "-t",
  "20",
  "-f",
  "mp3",
  "pipe:1",
]);
const rss = `<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel><itunes:image href="https://publisher.example/cover.png"/><title>Fixture Show</title><language>en</language><description>Public fixture</description>${[1, 2].map((n) => `<item><guid>ep${n}</guid><title>Episode ${n}</title><enclosure url="https://publisher.example/audio${n}.mp3" type="audio/mpeg"/><link>https://publisher.example/episode${n}</link></item>`).join("")}</channel></rss>`;
let ranges = 0;
const server = http.createServer((req, res) => {
  const u = new URL(req.url, "http://test");
  if (u.pathname === "/audio.mp3") {
    const range = req.headers.range;
    let start = 0,
      end = mp3.length - 1;
    if (range) {
      ranges++;
      const m = /bytes=(\d+)-(\d*)/.exec(range);
      if (m) {
        start = +m[1];
        if (m[2]) end = Math.min(+m[2], end);
      }
      res.writeHead(206, {
        "Content-Type": "audio/mpeg",
        "Accept-Ranges": "bytes",
        "Content-Range": `bytes ${start}-${end}/${mp3.length}`,
        "Content-Length": end - start + 1,
      });
    } else
      res.writeHead(200, {
        "Content-Type": "audio/mpeg",
        "Content-Length": mp3.length,
        "Accept-Ranges": "bytes",
      });
    res.end(mp3.subarray(start, end + 1));
    return;
  }
  if (u.pathname === "/net") {
    const target = new URL(u.searchParams.get("url"));
    if(target.pathname === '/cover.png'){
      res.writeHead(200,{'Content-Type':'application/json'});
      res.end(JSON.stringify({status:200,headers:{'content-type':'image/png'},body:'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII='}));return;
    }
    const body =
      target.hostname === "itunes.apple.com"
        ? JSON.stringify({
            results: [1, 2].map((i) => ({
              collectionId: i,
              collectionName: `${target.searchParams.get("country")} Show ${i}`,
              artistName: "Publisher",
              feedUrl: `https://publisher.example/feed${i}.rss`,
              collectionViewUrl: "https://podcasts.apple.com/podcast/id1",
            })),
          })
        : rss;
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        status: 200,
        headers: { "content-type": "application/xml" },
        body,
      }),
    );
    return;
  }
  const name = path.resolve(root, "." + u.pathname);
  if (!name.startsWith(root + path.sep) && name !== root) {
    res.writeHead(404);
    res.end();
    return;
  }
  const file = u.pathname === "/" ? path.join(root, "index.html") : name;
  if (!fs.existsSync(file)) {
    res.writeHead(404);
    res.end();
    return;
  }
  const type =
    {
      ".html": "text/html",
      ".js": "text/javascript",
      ".css": "text/css",
      ".svg": "image/svg+xml",
      ".json": "application/json",
      ".webp": "image/webp",
    }[path.extname(file)] || "application/octet-stream";
  res.setHeader("Content-Type", type);
  if (type === "text/html")
    res.setHeader(
      "Content-Security-Policy",
      "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' data: blob:; connect-src 'self' data: blob:; object-src 'none'",
    );
  res.end(fs.readFileSync(file));
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const base = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({
  args: ["--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
try {
  for (const [lang, theme, width] of [
    ["zh", "light", 1200],
    ["en", "dark", 1200],
    ["zh", "dark", 760],
    ["en", "light", 393],
  ]) {
    const context = await browser.newContext({
      viewport: { width, height: 900 },
      isMobile: width === 393,
      hasTouch: width === 393,
      colorScheme: theme,
    });
    await context.addInitScript(
      ({ lang, theme }) => {
        window.myco = {
          preferences: { language: lang, theme },
          features: ["net", "net-media"],
          ready: Promise.resolve({
            page: { id: "test", name: "Podcasts" },
            me: { name: "alice" },
          }),
          data: {
            list: async () => ({ records: [], seq: 0 }),
            on: () => () => {},
          },
          net: {
            fetch: async (o) =>
              fetch("/net?url=" + encodeURIComponent(o.url)).then((r) =>
                r.json(),
              ),
          },
          media: { url: () => "/audio.mp3" },
        };
      },
      { lang, theme },
    );
    const page = await context.newPage(),
      errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    if(lang === "zh" && width === 1200) await page.clock.install();
    await page.goto(base + "/?myco_lang=" + lang);
    await page.locator(".pg-show-grid button").first().waitFor();
    assert.ok(
      (await page.evaluate(() => document.documentElement.scrollWidth)) <=
        width,
      `page overflow at ${width}`,
    );
    await page.locator(".pg-show-grid button").first().click();
    await page.waitForFunction(
      () => document.querySelector("audio").readyState >= 2,
    );
    if (await page.locator("audio").evaluate((a) => a.paused))
      await page.locator(".pg-play").click();
    await page.waitForFunction(
      () => document.querySelector("audio").currentTime > 0.2,
    );
    await page.locator(".pg-play").click();
    await page.waitForFunction(() => document.querySelector("audio").paused);
    await page.waitForFunction(()=>document.querySelector(".pg-cover img")?.naturalWidth > 0);
    await page.locator(".pg-actions .pg-icon").first().click();
    assert.equal(
      await page
        .locator(".pg-actions .pg-icon")
        .first()
        .getAttribute("aria-pressed"),
      "true",
    );
    await page.locator(".pg-player-options button").click();
    await page.locator(".pg-player-options select").first().selectOption("1.5");
    assert.equal(
      await page.locator("audio").evaluate((a) => a.playbackRate),
      1.5,
    );
    await page.locator(".pg-progress input").fill("6");
    await page.waitForFunction(
      () => document.querySelector("audio").currentTime >= 5,
    );
    await page.locator(".pg-player-options select").nth(1).selectOption("15");
    if(lang === "zh" && width === 1200){
      await page.locator(".pg-play").click();
      await page.waitForFunction(()=>!document.querySelector("audio").paused);
      await page.clock.fastForward(15*60000+1);
      await page.waitForFunction(()=>document.querySelector("audio").paused);
      assert.equal(await page.locator(".pg-player-options select").nth(1).inputValue(),"0");
    }
    await page.screenshot({
      path: path.join(out, `${lang}-${theme}-${width}.png`),
      fullPage: true,
    });
    await page.locator(".pg-header nav button").nth(1).click();
    assert.equal(await page.locator(".pg-directory > .pg-episode").count(), 1);
    await page.reload();
    await page.waitForFunction(()=>document.querySelector("audio").currentTime>=5);
    await page.locator(".pg-show-grid button").first().waitFor();
    await page.locator(".pg-header nav button").nth(1).click();
    assert.equal(await page.locator(".pg-directory > .pg-episode").count(), 1);
    await page.locator(".pg-tuning select").selectOption("jp");
    await page.waitForFunction(() =>
      document.querySelector(".pg-show-grid")?.textContent.includes("jp Show"),
    );
    await page.waitForFunction(()=>document.querySelector("audio").readyState>=2);
    assert.equal(await page.locator("audio").getAttribute("src"), "/audio.mp3");
    assert.deepEqual(errors, []);
    await context.close();
    console.log(
      `PASS production page ${lang}/${theme}/${width}: MP3, pause, seek, speed, likes, follows, persistence, region and layout`,
    );
  }
  const old = await browser.newContext();
  await old.addInitScript(() => {
    window.myco = {
      preferences: { language: "en", theme: "light" },
      features: [],
      ready: Promise.resolve({ page: { id: "old" }, me: { name: "alice" } }),
      data: { list: async () => ({ records: [], seq: 0 }), on: () => () => {} },
    };
  });
  const page = await old.newPage();
  await page.goto(base + "/?myco_lang=en");
  await page
    .getByText("Update the App to stream podcasts on this device.", {
      exact: true,
    })
    .waitFor();
  assert.equal(await page.locator(".pg-play").isDisabled(), true);
  await old.close();
  assert.ok(ranges > 0);
  console.log(
    "PASS legacy-client upgrade hint and range-capable media requests",
  );
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
