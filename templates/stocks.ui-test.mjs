// Browser tests exercise the built app with real captured response fixtures and the myco bridge contract.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const here = path.dirname(fileURLToPath(import.meta.url)),
  root = path.resolve(here, "../internal/templates/official/stocks"),
  out = path.join(here, "stocks-shots");
fs.mkdirSync(out, { recursive: true });
const sample = JSON.parse(
    fs.readFileSync(path.join(root, "sample.json"), "utf8"),
  ),
  fixture = {};
for (const name of ["cn", "hk", "us", "pingan", "alibaba", "msft"])
  fixture["yahoo-" + name] = JSON.parse(
    fs.readFileSync(
      path.join(here, "src/stocks/testdata", "yahoo-" + name + ".json"),
      "utf8",
    ),
  );
for (const name of ["cn", "hk"])
  fixture["tencent-" + name] = JSON.parse(
    fs.readFileSync(
      path.join(here, "src/stocks/testdata", "tencent-" + name + ".json"),
      "utf8",
    ),
  );
const srv = http.createServer((req, res) => {
  let p = decodeURIComponent(new URL(req.url, "http://x").pathname);
  if (p === "/") p = "/index.html";
  const f = path.resolve(root, "." + p);
  if (!f.startsWith(root + path.sep) || !fs.existsSync(f)) {
    res.writeHead(404);
    res.end();
    return;
  }
  res.setHeader(
    "Content-Type",
    {
      ".html": "text/html",
      ".js": "text/javascript",
      ".css": "text/css",
      ".txt": "text/plain",
    }[path.extname(f)] || "application/octet-stream",
  );
  res.end(fs.readFileSync(f));
});
await new Promise((r) => srv.listen(0, "127.0.0.1", r));
let chromium;
try {
  ({ chromium } = createRequire(import.meta.url)("playwright"));
} catch {
  const globalRoot = execFileSync(
    process.platform === "win32" ? "npm.cmd" : "npm",
    ["root", "-g"],
    { encoding: "utf8" },
  ).trim();
  ({ chromium } = createRequire(path.join(globalRoot, "noop.js"))(
    "playwright",
  ));
}
const browser = await chromium.launch(
  process.env.UI_SMOKE_EXECUTABLE
    ? { executablePath: process.env.UI_SMOKE_EXECUTABLE }
    : {},
);
try {
  for (const [width, scheme] of [
    [1200, "light"],
    [1200, "dark"],
    [760, "light"],
    [760, "dark"],
  ]) {
    const ctx = await browser.newContext({
      viewport: { width, height: 900 },
      colorScheme: scheme,
    });
    await ctx.addInitScript(
      ({ sample, fixture }) => {
        let records =
          JSON.parse(sessionStorage.getItem("records") || "null") ||
          sample.watchlist.map(({ id, ...data }) => ({
            id,
            collection: "watchlist",
            data,
            version: 1,
          }));
        window.__records = () => records;
        window.__offline = false;
        window.__downloads = [];
        window.__requests = [];
        window.myco = {
          features: ["os"],
          ready: Promise.resolve({
            page: { id: "pg_stocks" },
            me: { name: "Glenn" },
          }),
          data: {
            list: async () => ({ records: structuredClone(records), seq: 0 }),
            put: async (c, data, o = {}) => {
              const id = o.id || Math.random().toString(36),
                old = records.find((r) => r.id === id);
              if (
                old &&
                o.ifVersion !== undefined &&
                o.ifVersion !== old.version
              )
                throw { code: "conflict", latest: old };
              const r = {
                id,
                collection: c,
                data: structuredClone(data),
                version: (old?.version || 0) + 1,
              };
              records = records.filter((r) => r.id !== id);
              records.push(r);
              sessionStorage.setItem("records", JSON.stringify(records));
              return structuredClone(r);
            },
            remove: async (c, id) => {
              records = records.filter((r) => r.id !== id);
              sessionStorage.setItem("records", JSON.stringify(records));
              return true;
            },
            on: () => () => {},
          },
          download: async (name, data) => {
            window.__downloads.push({
              name,
              data: data instanceof Blob ? await data.text() : data,
            });
            return { path: name };
          },
          os: {
            net: {
              fetch: async ({ url }) => {
                window.__requests.push(url);
                if (window.__offline) throw Error("Test offline");
                await new Promise((r) => setTimeout(r, 10));
                let raw;
                if (url.includes("yahoo")) {
                  const symbol = decodeURIComponent(
                    new URL(url).pathname.split("/").pop(),
                  );
                  const id = {
                    "600519.SS": "cn",
                    "000001.SZ": "pingan",
                    "0700.HK": "hk",
                    "9988.HK": "alibaba",
                    AAPL: "us",
                    MSFT: "msft",
                  }[symbol];
                  raw = id
                    ? fixture["yahoo-" + id]
                    : { chart: { error: { description: "not found" } } };
                } else if (url.includes("gtimg"))
                  raw =
                    fixture[
                      new URL(url).searchParams.get("param").startsWith("hk")
                        ? "tencent-hk"
                        : "tencent-cn"
                    ];
                else return { status: 404, body: "{}", encoding: "utf8" };
                return {
                  status: 200,
                  body: JSON.stringify(raw),
                  encoding: "utf8",
                };
              },
            },
          },
        };
      },
      { sample, fixture },
    );
    const page = await ctx.newPage(),
      errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    const b = (name) => page.getByRole("button", { name, exact: true }),
      chart = page.locator("[data-testid=price-chart]");
    await page.goto(`http://127.0.0.1:${srv.address().port}/`);
    await chart.waitFor();
    await page.waitForTimeout(500);
    assert.equal(await page.locator(".sw-stock").count(), 6);
    await page.screenshot({
      path: path.join(out, `${width}-${scheme}-cn.png`),
    });
    // The built chart is interactive and backed by actual canvas pixels.
    assert((await chart.locator("canvas").count()) > 0);
    const box = await chart.boundingBox();
    await page.mouse.move(box.x + box.width * 0.5, box.y + box.height * 0.4);
    await page.waitForTimeout(100);
    assert(await page.locator(".sw-ohlc").innerText());
    await page.locator(".sw-stock").filter({ hasText: "腾讯控股" }).click();
    await page.waitForFunction(
      () => document.querySelector(".sw-currency")?.textContent === "HKD",
    );
    await chart.waitFor();
    await page.locator(".sw-stock").filter({ hasText: "Apple 苹果" }).click();
    await page.waitForFunction(
      () => document.querySelector(".sw-currency")?.textContent === "USD",
    );
    await chart.waitFor();
    await b("周 K").click();
    await chart.waitFor();
    await b("月 K").click();
    await chart.waitFor();
    await b("日 K").click();
    await chart.waitFor();
    await b("收盘价折线").click();
    await chart.waitFor();
    await b("K 线图").click();
    await page.getByRole("button", { name: "MA", exact: true }).click();
    await chart.waitFor();
    await page.getByRole("button", { name: "MA", exact: true }).click();
    await page.getByLabel("观察笔记").fill("关注下一次财报");
    await page.waitForFunction(() =>
      window.__records().some((r) => r.data.note === "关注下一次财报"),
    );
    await page.screenshot({
      path: path.join(out, `${width}-${scheme}-us.png`),
    });
    for (const name of [
      "导出 CSV",
      "导出 JSON（含来源与时间）",
      "导出图表 PNG",
    ]) {
      await b("导出行情").click();
      await b(name).click();
      await page.waitForTimeout(100);
    }
    await page.waitForFunction(() => window.__downloads.length === 3);
    assert(
      (
        await page.evaluate(
          () => window.__downloads.find((x) => x.name.endsWith(".csv")).data,
        )
      ).includes("volume_shares"),
    );
    // Explicitly selected unsupported providers show an error, never fake US candles.
    await b("设置").click();
    await page.getByLabel("行情数据源").selectOption("tencent");
    await b("保存设置").click();
    await page
      .getByRole("alert")
      .filter({ hasText: "不支持所选市场" })
      .waitFor();
    assert.equal(await chart.count(), 0);
    await page.locator(".sw-stock").filter({ hasText: "贵州茅台" }).click();
    await chart.waitFor();
    await page
      .locator(".sw-data-footer strong", { hasText: "腾讯行情" })
      .waitFor();
    await b("设置").click();
    await page.getByLabel("行情数据源").selectOption("gateway");
    await page
      .getByLabel("行情网关地址")
      .fill("https://user:secret@example.com");
    await b("保存设置").click();
    await page.getByRole("alert").filter({ hasText: "不带凭证" }).waitFor();
    assert(
      !(await page.evaluate(() =>
        window.__requests.some((x) => x.includes("secret")),
      )),
    );
    await page.getByLabel("行情数据源").selectOption("auto");
    await b("保存设置").click();
    await chart.waitFor();
    await page.locator(".sw-stock").filter({ hasText: "Apple 苹果" }).click();
    await chart.waitFor();
    await page.evaluate(() => {
      window.__offline = true;
    });
    await b("刷新").click();
    await page.locator(".sw-data-footer.stale").waitFor();
    assert(
      (await page.locator(".sw-data-footer").innerText()).includes(
        "不是最新行情",
      ),
    );
    await page.evaluate(() => {
      window.__offline = false;
    });
    await b("刷新").click();
    await page.locator(".sw-data-footer:not(.stale)").waitFor();
    await b("移出自选").click();
    await page.waitForFunction(() => window.__records().length === 5);
    await b("添加自选").click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "美股", exact: true })
      .click();
    await page.getByLabel("股票名称或代码").fill("AAPL");
    await page
      .locator(".sw-add-results button")
      .filter({ hasText: "Apple 苹果" })
      .click();
    await page.waitForFunction(() => window.__records().length === 6);
    await chart.waitFor();
    await page.reload();
    await chart.waitFor();
    assert.equal(await page.locator(".sw-stock").count(), 6);
    assert.deepEqual(errors, []);
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    );
    // Catalog thumbnails show the untouched sample, using the captured real CN response.
    if (width === 1200) {
      await page.locator(".sw-stock").filter({ hasText: "贵州茅台" }).click();
      await chart.waitFor();
      await page.waitForTimeout(200);
      const png = await page.screenshot();
      const webp = await page.evaluate(async (b64) => {
        const img = new Image();
        await new Promise((r) => {
          img.onload = r;
          img.src = "data:image/png;base64," + b64;
        });
        const c = document.createElement("canvas");
        c.width = img.width;
        c.height = img.height;
        c.getContext("2d").drawImage(img, 0, 0);
        return c.toDataURL("image/webp", 0.86).split(",")[1];
      }, png.toString("base64"));
      const dir = path.join(here, "src/stocks/public/shots");
      fs.mkdirSync(dir, { recursive: true });
      fs.writeFileSync(
        path.join(dir, scheme === "dark" ? "1.dark.webp" : "1.webp"),
        Buffer.from(webp, "base64"),
      );
    }
    console.log(
      `PASS ${width} ${scheme}: CN/HK/US, real chart, periods, modes, MA, notes, exports, provider switch, unsupported/error/stale, safe gateway, add/remove, reload`,
    );
    await ctx.close();
  }
} finally {
  await browser.close();
  await new Promise((r) => srv.close(r));
}
