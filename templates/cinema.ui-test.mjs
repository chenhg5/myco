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
const phoneBridge = `if (window.__PHONE_NETWORK__) { window.myco.net = window.myco.os.net; delete window.myco.os; window.myco.features = ["net", "preferences"]; }`;
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
                : Array.from(
                    { length: u.searchParams.has("long") ? 24 : 2 },
                    (_, i) => movie(String(i + 1)),
                  ),
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
    res.end(bridge.replace('page: { id: "pg_shots", name: "shots" }', 'page: { id: "pg_shots", name: "shots", url: "https://hub.test/pages/pg_shots" }') + phoneBridge);
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
  for (const [lang, theme, width, phone] of [
    ["zh", "light", 1400],
    ["zh", "dark", 760],
    ["en", "light", 760],
    ["en", "dark", 1400],
    ["zh", "light", 393, true],
    ["en", "dark", 393, true],
  ]) {
    const page = await browser.newPage({
      viewport: { width, height: 900 },
      colorScheme: theme,
      hasTouch: !!phone,
      isMobile: !!phone,
    });
    const errors = [],
      network = [];
    page.on("pageerror", (e) => errors.push(e.message));
    page.on("console", (m) => {
      if (m.type() === "error") console.log("BROWSER", m.text());
    });
    page.on("request", (r) => network.push(r.url()));
    await page.addInitScript(
      ({ lang, theme, phone }) => {
        window.__PHONE_NETWORK__ = phone;
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
                  (args.response === "base64" ? "1" : "0") +
                  (phone ? "&long=1" : ""),
              );
              return r.json();
            },
          },
        };
        document.addEventListener("DOMContentLoaded", () => {
          document.documentElement.setAttribute("data-myco-theme", theme);
        });
      },
      { lang, theme, phone },
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
    assert.equal(await page.locator(".ci-film").count(), phone ? 24 : 2);
    if (phone) {
      const touch = await page.context().newCDPSession(page);
      const swipe = async (from, to) => {
        await touch.send("Input.dispatchTouchEvent", {
          type: "touchStart",
          touchPoints: [{ x: 100, y: from }],
        });
        for (let i = 1; i <= 12; i++) {
          await touch.send("Input.dispatchTouchEvent", {
            type: "touchMove",
            touchPoints: [{ x: 100, y: from + ((to - from) * i) / 12 }],
          });
          await page.waitForTimeout(20);
        }
        await touch.send("Input.dispatchTouchEvent", {
          type: "touchEnd",
          touchPoints: [],
        });
        await page.waitForTimeout(500);
      };
      // Finger gestures on film buttons must scroll a long catalog in both
      // directions; click() and scrollIntoView() bypass overflow:hidden.
      await swipe(720, 250);
      await page.waitForFunction(() => scrollY > 100, null, { timeout: 5000 });
      const down = await page.evaluate(() => scrollY);
      await swipe(250, 720);
      await page.waitForFunction((down) => scrollY < down - 100, down, {
        timeout: 5000,
      });
      await touch.detach();
      await page.evaluate(() => scrollTo(0, 0));
      console.log("PASS phone finger swipes scroll the catalog down and back up");
    }
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
    // A successful click must actually enter element fullscreen, not merely
    // resolve a mocked request. Fullscreen has no app-added corner overlays.
    await page.getByRole("button", {name: L("全屏", "Fullscreen"), exact:true}).click();
    await page.waitForFunction(() => document.fullscreenElement?.classList.contains("ci-screen"));
    await page.screenshot({path:path.join(out, `${lang}-${theme}-${width}-fullscreen.png`)});
    assert.equal(await page.locator(".ci-screen button").count(), 0);
    await page.evaluate(() => document.exitFullscreen());
    await page.waitForFunction(() => !document.fullscreenElement);
    await page.getByRole("button", {name:L("全屏", "Fullscreen"), exact:true}).click();
    await page.waitForFunction(() => !!document.fullscreenElement);
    await page.evaluate(() => document.exitFullscreen());
    await page.waitForFunction(() => !document.fullscreenElement);
    // Permission failures must remain visible without replacing playable video.
    await page.locator(".ci-screen").evaluate(el => {
      el.requestFullscreen = () => Promise.reject(new Error("Permission denied"));
    });
    await page.getByRole("button", {name:L("全屏", "Fullscreen"), exact:true}).click();
    await page.getByRole("alert").getByText(L("这里不能全屏", "Fullscreen is unavailable here"),{exact:true}).waitFor();
    assert.equal(await page.locator("video").count(),1);
    await page.locator("video").evaluate(video => {
      window.__nativeFullscreen = 0;
      video.webkitEnterFullscreen = () => window.__nativeFullscreen++;
    });
    await page.getByRole("button", {name:L("全屏", "Fullscreen"), exact:true}).click();
    await page.waitForFunction(() => window.__nativeFullscreen === 1);
    assert.equal(await page.getByRole("alert").count(),0);
    await page.locator(".ci-screen").evaluate(el => {
      el.requestFullscreen = undefined;
      el.webkitRequestFullscreen = Element.prototype.requestFullscreen.bind(el);
    });
    await page.getByRole("button", {name:L("全屏", "Fullscreen"), exact:true}).click();
    await page.waitForFunction(() => !!document.fullscreenElement);
    assert.equal(await page.locator(".ci-screen button").count(), 0);
    await page.evaluate(() => document.exitFullscreen());
    await page.waitForFunction(() => !document.fullscreenElement);
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
    // Deep-link navigation is a view choice: no source or watchlist writes.
    const sourceId = await page.evaluate(async () => (await window.myco.data.list("sources")).records[0].id);
    const route = (movie, line="HLS", episode="第02集") => "#watch?" + new URLSearchParams({source:sourceId,movie,line,episode});
    await page.evaluate(fragment => { location.hash = fragment; }, route("1"));
    await page.getByRole("combobox",{name:L("播放线路","Playback line"),exact:true}).waitFor();
    assert.equal(await page.getByRole("combobox",{name:L("播放线路","Playback line"),exact:true}).inputValue(),"0");
    assert.equal(await page.getByRole("button",{name:"第02集",exact:true}).getAttribute("aria-pressed"),"true");
    assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth) <= width, "deep-link player fits the configured viewport");
    // A clipboard refusal still exposes the canonical, answer-free public URL.
    await page.evaluate(() => { Object.defineProperty(navigator,"clipboard",{configurable:true,value:{writeText:async()=>{throw Error("denied")}}}); });
    await page.getByRole("button",{name:L("复制播放页链接","Copy playback link"),exact:true}).click();
    const copied = new URL(await page.getByRole("textbox",{name:L("播放页链接","Playback link"),exact:true}).inputValue());
    assert.equal(copied.origin,"https://hub.test");assert.equal(copied.pathname,"/pages/pg_shots");assert.equal(copied.hash,route("1"));assert.equal(copied.search,"");assert.ok(!copied.href.includes("media.test"));
    await page.getByRole("dialog",{name:L("复制播放页链接","Copy playback link"),exact:true}).getByRole("button",{name:L("关闭","Close"),exact:true}).click();
    await page.evaluate(fragment => { location.hash = fragment; },route("2","备用","第02集"));
    await page.getByRole("heading",{name:"城市的另一面",exact:true}).waitFor();
    assert.equal(await page.getByRole("combobox",{name:L("播放线路","Playback line"),exact:true}).inputValue(),"1");
    await page.evaluate(fragment => { location.hash = fragment; },route("1","HLS","已移除的集数"));
    await page.getByRole("alert").getByText(L("链接指定的集数已不存在，请重新选择集数","The linked episode is no longer available. Choose an episode again."),{exact:true}).waitFor();assert.equal(await page.locator("video").count(),0);
    await page.getByRole("button",{name:L("关闭","Close"),exact:true}).click();
    assert.equal(await page.evaluate(()=>location.hash), "");
    // Startup with a fragment follows the same path, before any catalog card click.
    await page.evaluate(({sourceId,source})=>{window.__savedSource={id:sourceId,...source}}, {sourceId,source});
    const startup=await browser.newPage({viewport:{width,height:900},colorScheme:theme});
    await startup.addInitScript(({sourceId,source,phone})=>{window.__PHONE_NETWORK__=phone;window.__SAMPLE__={sources:[{id:sourceId,...source}]};window.__FAKE_OS__={net:{fetch:async args=>(await fetch("/fixture?url="+encodeURIComponent(args.url)+"&binary="+(args.response==="base64"?"1":"0"))).json()}};},{sourceId,source,phone});
    await startup.goto(`http://127.0.0.1:${server.address().port}/?myco_lang=${lang}`+route("1","备用","第02集"));
    await startup.getByRole("combobox",{name:L("播放线路","Playback line"),exact:true}).waitFor();assert.equal(await startup.getByRole("combobox",{name:L("播放线路","Playback line"),exact:true}).inputValue(),"1");
    await startup.screenshot({path:path.join(out,`${lang}-${theme}-${width}-deep-link.png`)});
    await startup.close();
    assert.deepEqual(errors, []);
    await page.close();
    console.log(
      `PASS ${lang}/${theme}/${width}: empty defaults, setup, catalog, search, real HLS, watchlist, local resume, line/episode selection, sandbox, fullscreen entry/exit/failure/fallback and layout`,
    );
  }
  const legacy = await browser.newPage({viewport:{width:393,height:852}});
  await legacy.addInitScript(() => { window.__SAMPLE__ = {sources:[{id:"legacy",name:"My catalog",endpoint:"https://catalog.test/api",enabled:true,order:1}]}; });
  await legacy.goto(`http://127.0.0.1:${server.address().port}/?myco_lang=zh`);
  await legacy.getByText("当前 myco App 尚不支持应用联网，请更新 App 后再试",{exact:true}).waitFor();
  assert.equal(await legacy.getByRole("button",{name:"搜索",exact:true}).isDisabled(),true);
  assert.equal(await legacy.getByText("这个 myco App 还不能让应用用电脑：更新 App 后再试",{exact:true}).count(),0);
  await legacy.close();
  console.log("PASS old iPhone App shows a network update hint without claiming desktop OS access");
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
