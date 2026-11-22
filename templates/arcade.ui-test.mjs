import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { join } from "node:path";

const require = createRequire(join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const root = new URL("../internal/templates/official/arcade/", import.meta.url);
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://test").pathname;
  try {
    const type = path.endsWith(".css") ? "text/css" : path.endsWith(".js") ? "text/javascript" : "text/html";
    const data = readFileSync(new URL(path === "/" ? "index.html" : path.slice(1), root));
    res.writeHead(200, { "Content-Type": type, "Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self'" });
    res.end(data);
  } catch { res.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
let browser;
try {
  browser = await chromium.launch({ executablePath: process.env.UI_SMOKE_EXECUTABLE });
  for (const [lang, theme, width] of [["zh", "light", 1080], ["en", "dark", 390]]) {
    const page = await browser.newPage({ viewport: { width, height: 760 }, colorScheme: theme, hasTouch: width < 400 });
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    await page.goto(`http://127.0.0.1:${server.address().port}/?myco_lang=${lang}`);
    await page.getByRole("heading", { name: lang === "zh" ? "轨道信使" : "Orbit Courier" }).waitFor();
    assert.equal(await page.locator(".arc-app").evaluate(el => el.scrollWidth), width);
    if (lang === "zh") {
      await page.getByRole("button", { name: "开始", exact: true }).click();
      await page.keyboard.down("ArrowRight");
      await page.getByRole("button", { name: "按键 6", exact: true, pressed: true }).waitFor();
      await page.keyboard.up("ArrowRight");
      await page.getByRole("button", { name: "暂停", exact: true }).click();
    } else {
      const button = page.getByRole("button", { name: "Key 6", exact: true });
      await button.tap();
      assert.equal(await button.getAttribute("aria-pressed"), "false");
    }
    const file = page.locator('input[type="file"]');
    await file.setInputFiles({ name: "invalid.zip", mimeType: "application/zip", buffer: Buffer.from([0x12, 0]) });
    await page.getByRole("alert").waitFor();
    await file.setInputFiles({ name: "loop.ch8", mimeType: "application/octet-stream", buffer: Buffer.from([0x12, 0]) });
    await page.getByRole("heading", { name: "loop.ch8" }).waitFor();
    await page.getByRole("status").getByText(lang === "zh" ? "运行中" : "Running").waitFor();
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log("PASS Arcade Emulator keyboard, mobile touch, ROM import and validation in both languages and themes");
} finally {
  await browser?.close();
  server.close();
}
