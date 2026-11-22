import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { join } from "node:path";

const require = createRequire(join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
function testCartridge() {
  const rom = Buffer.alloc(16 + 16384);
  rom.set([0x4e, 0x45, 0x53, 0x1a, 1], 0);
  // Read the first two bits from controller 1 and latch the result in the
  // PPU backdrop. A gives green, B gives red, and a fresh boot gives blue.
  rom.set([
    0xa9, 0x21, 0x85, 0x00,
    0xa9, 0x01, 0x8d, 0x16, 0x40, 0xa9, 0x00, 0x8d, 0x16, 0x40,
    0xad, 0x16, 0x40, 0x4a, 0xb0, 0x0b, 0xad, 0x16, 0x40, 0x4a, 0x90, 0x09,
    0xa9, 0x26, 0x4c, 0x21, 0x80, 0xa9, 0x2a, 0x85, 0x00,
    0xa9, 0x3f, 0x8d, 0x06, 0x20, 0xa9, 0x00, 0x8d, 0x06, 0x20,
    0xa5, 0x00, 0x8d, 0x07, 0x20, 0x4c, 0x04, 0x80,
  ], 16);
  rom.writeUInt16LE(0x8000, 16 + 0x3ffc);
  return { name: "test.nes", mimeType: "application/octet-stream", buffer: rom };
}

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
  for (const [lang, theme, width] of [["zh", "light", 1080], ["en", "dark", 390]]) {
    const mobile = width < 400;
    const page = await browser.newPage({
      viewport: { width, height: 820 }, colorScheme: theme, isMobile: mobile, hasTouch: mobile,
    });
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    await page.goto(`http://127.0.0.1:${server.address().port}/?myco_lang=${lang}#nes`);
    const panel = page.locator(".arc-nes");
    const importButton = panel.getByRole("button", { name: mobile ? "Import .nes" : "导入 .nes" });
    await importButton.waitFor();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth), width);
    const file = panel.locator('input[type="file"]');
    await file.setInputFiles({ name: "invalid.nes", mimeType: "application/octet-stream", buffer: Buffer.alloc(16) });
    await panel.getByRole("alert").waitFor();
    const provided = process.env.NES_TEST_ROM;
    const chooserPromise = page.waitForEvent("filechooser");
    await importButton.click();
    const chooser = await chooserPromise;
    await chooser.setFiles(provided || testCartridge());
    await panel.getByRole("status").getByText(mobile ? "Running" : "运行中").waitFor();
    await page.waitForFunction(() => {
      const pixels = document.querySelector(".arc-nes canvas").getContext("2d").getImageData(0, 0, 256, 240).data;
      return pixels.some((value, index) => (index & 3) !== 3 && value > 20);
    });
    if (mobile) await page.evaluate(() => Object.defineProperty(document, "fullscreenEnabled", { value: false }));
    await panel.getByRole("button", { name: mobile ? "Fullscreen" : "全屏", exact: true }).click();
    await page.waitForFunction(() => !!document.querySelector(".arc-nes-full"));
    if (mobile) assert.equal(await page.evaluate(() => !!document.fullscreenElement), false);
    else await page.waitForFunction(() => document.fullscreenElement === document.querySelector(".arc-nes"));
    if (!provided) {
      const color = channel => page.waitForFunction(channel => {
        const data = document.querySelector(".arc-nes canvas").getContext("2d").getImageData(128, 120, 1, 1).data;
        return channel === "green" ? data[1] > data[0] + 30 : data[0] > data[1] + 30;
      }, channel);
      const pressA = async () => {
        if (mobile) await panel.getByRole("button", { name: "Action A", exact: true }).tap();
        else await page.evaluate(() => {
          // Dispatch both events synchronously so they fall in the same frame.
          for (const type of ["keydown", "keyup"]) window.dispatchEvent(new KeyboardEvent(type, { code: "KeyX" }));
        });
      };
      await pressA();
      await color("green");
      await panel.getByRole("checkbox", { name: mobile ? "Swap A/B" : "交换 A/B" }).check();
      await pressA();
      await color("red");
      if (mobile) {
        await page.setViewportSize({ width: 700, height: 350 });
        const [screen, directions, actions] = await Promise.all([
          panel.locator("canvas").boundingBox(), panel.locator(".arc-nes-direction").boundingBox(), panel.locator(".arc-nes-action").boundingBox(),
        ]);
        assert.ok(directions.x + directions.width <= screen.x && actions.x >= screen.x + screen.width);
        assert.ok(actions.y + actions.height <= 350);
      }
    }
    await panel.getByRole("button", { name: mobile ? "Exit fullscreen" : "退出全屏" }).click();
    await page.waitForFunction(() => !document.querySelector(".arc-nes-full") && !document.fullscreenElement);
    const direction = panel.getByRole("button", { name: mobile ? "Right" : "方向键右", exact: true });
    if (mobile) {
      const action = panel.getByRole("button", { name: "Action A", exact: true });
      await direction.scrollIntoViewIfNeeded();
      const [left, right] = await Promise.all([direction.boundingBox(), action.boundingBox()]);
      const touchPoints = [left, right].map((box, id) => ({ id, x: box.x + box.width / 2, y: box.y + box.height / 2 }));
      const cdp = await page.context().newCDPSession(page);
      await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints });
      await panel.getByRole("button", { name: "Right", exact: true, pressed: true }).waitFor();
      await panel.getByRole("button", { name: "Action A", exact: true, pressed: true }).waitFor();
      await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
      assert.equal(await direction.getAttribute("aria-pressed"), "false");
      assert.equal(await action.getAttribute("aria-pressed"), "false");
    } else {
      await page.keyboard.down("ArrowRight");
      await panel.getByRole("button", { name: "方向键右", exact: true, pressed: true }).waitFor();
      await page.keyboard.up("ArrowRight");
      assert.equal(await direction.getAttribute("aria-pressed"), "false");
    }
    await panel.getByRole("button", { name: mobile ? "Pause" : "暂停", exact: true }).click();
    await panel.getByRole("status").getByText(mobile ? "Paused" : "已暂停").waitFor();
    await panel.getByRole("button", { name: mobile ? "Restart" : "重启", exact: true }).click();
    await panel.getByRole("status").getByText(mobile ? "Running" : "运行中").waitFor();
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log("PASS NES cartridge validation, local play, keyboard and multi-touch");
  const page = await browser.newPage({ viewport: { width: 1040, height: 650 }, colorScheme: "dark" });
  await page.addInitScript(() => {
    let failure = true;
    window.mameCalls = [];
    window.myco = { features: ["os"], os: {
      system: { info: async () => ({ os: "windows" }) },
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
    ["mame", "gridlee", "-rompath", "/Local ROMs", "-window", "-skip_gameinfo", "-keyboardprovider", "win32"],
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
