// Exercise the bundled editor with versioned records and a page send bridge.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { join } from "node:path";

const require = createRequire(join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const root = new URL("../internal/templates/official/logo-studio/", import.meta.url);
const sample = JSON.parse(readFileSync(new URL("sample.json", root)));
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://test").pathname;
  try {
    const type = path.endsWith(".css") ? "text/css" : path.endsWith(".js") ? "text/javascript" : "text/html";
    const data = readFileSync(new URL(path === "/" ? "index.html" : path.slice(1), root));
    res.writeHead(200, { "Content-Type": type });
    res.end(data);
  } catch { res.writeHead(404).end(); }
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
let browser;
try {
  browser = await chromium.launch({ executablePath: process.env.UI_SMOKE_EXECUTABLE });
  for (const [language, theme, width] of [["zh", "light", 1100], ["en", "dark", 390]]) {
    const page = await browser.newPage({ viewport: { width, height: 750 }, colorScheme: theme === "light" ? "dark" : "light" });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.addInitScript(({ sample, language, theme }) => {
      document.addEventListener("DOMContentLoaded", () => document.documentElement.setAttribute("data-myco-theme", theme));
      const listeners = [];
      const rows = Object.entries(sample).flatMap(([collection, data]) =>
        data.map(({ id, ...row }) => ({ id, collection, data: row, version: 1 })));
      window.myco = {
        features: ["preferences"],
        preferences: { language, theme },
        ready: Promise.resolve({ me: { name: "learner" }, page: { id: "pg_logo" } }),
        data: {
          list: async (collection) => ({ records: rows.filter((row) => row.collection === collection), seq: 0 }),
          put: async (collection, data, options) => {
            const id = options?.id || "r" + rows.length;
            const index = rows.findIndex((row) => row.id === id && row.collection === collection);
            const version = rows[index]?.version || 0;
            if (options?.ifVersion !== undefined && options.ifVersion !== version) throw Object.assign(new Error("conflict"), { code: "conflict", latest: rows[index] });
            const row = { id, collection, data, version: version + 1 };
            if (index < 0) rows.push(row);
            else rows[index] = row;
            return row;
          },
          on: (listener) => { listeners.push(listener); return () => {}; },
        },
        send: async (message) => {
          window.request = rows.filter((row) => row.collection === "briefs").at(-1);
          if (!window.request || !message.includes(window.request.id)) throw new Error("Request not saved");
          return { id: "msg_logo" };
        },
        download: async (name, file, type) => {
          window.savedLogo = { name, file, type };
          return { path: name };
        },
      };
      window.addConcept = (data) => {
        const id = "reply-logo";
        const collection = "concepts";
        for (const listener of listeners) listener({ collection, record: id, data, op: "put", version: 1 });
      };
    }, { sample, language, theme });
    await page.goto(`http://127.0.0.1:${server.address().port}/`);
    const zh = language === "zh";
    const copy = (cn, en) => zh ? cn : en;
    await page.getByRole("heading", { name: copy("标志工坊", "Logo Studio") }).waitFor();
    assert.equal(await page.locator(".logo-app").evaluate((node) => getComputedStyle(node).backgroundColor),
      theme === "dark" ? "rgb(31, 31, 31)" : "rgb(255, 255, 255)");
    assert.equal(await page.locator(".logo-app").evaluate((node) => node.scrollWidth <= document.documentElement.clientWidth), true);
    await page.getByRole("button", { name: "Daybreak" }).click();
    const paper = page.locator(".logo-paper");
    assert.equal(await paper.locator("svg g").count(), 1);
    await page.getByRole("button", { name: copy("构图辅助线", "Construction guides") }).click();
    assert.equal(await paper.locator("svg g").count(), 0);
    const progress = page.getByRole("slider", { name: copy("构图进度", "Construction progress") });
    await progress.fill("1");
    assert.equal(await paper.locator("path").count(), 1);
    await progress.fill("2");
    await page.getByRole("button", { name: "半轮日光" }).click();
    await page.getByRole("spinbutton", { name: "X", exact: true }).fill("4");
    await page.getByRole("button", { name: copy("保存图层", "Save layer") }).click();
    await page.waitForFunction(() => document.querySelector(".logo-paper path")?.getAttribute("transform")?.startsWith("translate(4 "));
    await page.getByRole("button", { name: copy("导出 SVG", "Export SVG"), exact: true }).click();
    const output = await page.evaluate(() => window.savedLogo);
    assert.equal(output.name, "Daybreak.svg");
    assert.equal(output.type, "image/svg+xml");
    assert.match(output.file, /translate\(4 0\)/);
    assert.doesNotMatch(output.file, /<circle|<line|stroke-dasharray/);
    await page.getByRole("button", { name: copy("请 agent 出方案", "Ask agent for concepts") }).click();
    await page.getByText(copy("等待设计方案…", "Waiting for logo concepts…"), { exact: true }).waitFor();
    await page.evaluate(() => {
      window.addConcept({ title: "New concept", request: window.request.id, rationale: "A two-color mark.", order: Date.now(),
        layers: [{ id: "one", shape: "circle", x: 128, y: 110, w: 100, fill: "#110011" },
          { id: "two", shape: "rect", x: 88, y: 122, w: 80, h: 40, fill: "#F1AA70" }] });
    });
    await page.getByRole("heading", { name: "New concept", exact: true }).waitFor();
    assert.equal(await paper.locator("rect").count(), 1);
    await page.getByRole("button", { name: copy("空白方案", "Blank concept") }).click();
    await page.getByRole("heading", { name: "Ridgeline Coffee", exact: true }).waitFor();
    await page.getByRole("combobox", { name: copy("添加图层", "Add layer") }).selectOption("circle");
    await page.getByRole("spinbutton", { name: copy("直径", "Diameter") }).waitFor();
    assert.equal(await paper.locator("circle").count(), 1);
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log("PASS logo briefs, agent updates, guides, playback scrub, edits and SVG export in light Chinese and dark English");
} finally {
  await browser?.close();
  server.close();
}
