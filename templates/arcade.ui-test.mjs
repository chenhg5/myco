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
  const page = await browser.newPage({ viewport: { width: 1040, height: 650 }, colorScheme: "dark" });
  await page.addInitScript(() => {
    let failure = true;
    window.mameCalls = [];
    window.myco = { features: ["os"], os: {
      list: async () => ({ caps: ["fs.pick", "exec.run", "exec.spawn"].map(name => ({ name, available: true })) }),
      fs: { pick: async () => ({ paths: ["/Local ROMs/gridlee.zip"] }) },
      exec: {
        run: async ({ argv }) => {
          window.mameCalls.push(argv);
          if (argv[1] === "-help") return { code: 0, stdout: "MAME v0.285 (mame0285)", stderr: "" };
          if (failure) {
            failure = false;
            return { code: 2, stdout: "", stderr: "gridlee ROM not found" };
          }
          return { code: 0, stdout: "romset gridlee is good", stderr: "" };
        },
        spawn: ({ argv }) => {
          window.mameCalls.push(argv);
          let finish;
          const done = new Promise(resolve => { finish = resolve; });
          return { done, on() {}, write: async () => {}, end: async () => {},
            cancel: async () => { finish({ code: 0 }); return true; } };
        },
      },
    } };
  });
  await page.goto(`http://127.0.0.1:${server.address().port}/`);
  await page.getByRole("button", { name: "查找 MAME" }).click();
  await page.getByText("MAME 0.285").waitFor();
  await page.getByRole("button", { name: "选择 MAME ROM ZIP" }).click();
  await page.getByText("gridlee.zip", { exact: true }).waitFor();
  await page.getByRole("button", { name: "启动游戏" }).click();
  await page.getByText("gridlee ROM not found").waitFor();
  assert.equal(await page.evaluate(() => window.mameCalls.length), 2);
  if (process.env.MAME_SCREENSHOT) {
    await page.reload();
    await page.getByRole("button", { name: "查找 MAME" }).click();
    await page.getByRole("button", { name: "选择 MAME ROM ZIP" }).click();
    await page.getByText("gridlee.zip", { exact: true }).waitFor();
    await page.screenshot({ path: process.env.MAME_SCREENSHOT });
    await page.getByRole("button", { name: "启动游戏" }).click();
    await page.getByText("gridlee ROM not found").waitFor();
  }
  await page.getByRole("button", { name: "启动游戏" }).click();
  await page.getByText("MAME 已启动。切换到游戏窗口开始玩。").waitFor();
  assert.deepEqual(await page.evaluate(() => window.mameCalls.slice(-2)), [
    ["mame", "gridlee", "-rompath", "/Local ROMs", "-verifyroms"],
    ["mame", "gridlee", "-rompath", "/Local ROMs", "-window", "-skip_gameinfo"],
  ]);
  await page.getByRole("button", { name: "停止游戏" }).click();
  await page.getByText("游戏窗口已关闭。").waitFor();
  await page.close();
  console.log("PASS MAME desktop selection, failed verification, native launch and stop");
  console.log("PASS Arcade Emulator keyboard, mobile touch, ROM import and validation in both languages and themes");
} finally {
  await browser?.close();
  server.close();
}
