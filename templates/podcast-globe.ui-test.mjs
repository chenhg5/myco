import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";

const require = createRequire(join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const bundle = await build({
  stdin: {
    resolveDir: fileURLToPath(new URL(".", import.meta.url)),
    loader: "tsx",
    contents: `
      import { StrictMode, useState } from "react";
      import { createRoot } from "react-dom/client";
      import { Globe } from "./src/podcast/globe";
      const regions = [
        { id: "europe", label: "Europe", lat: 49, lon: 15 },
        { id: "west-asia", label: "Western Asia", lat: 31, lon: 49 },
        { id: "africa", label: "Africa", lat: -5, lon: 22 },
        { id: "south-asia", label: "South Asia", lat: 19, lon: 78 },
        { id: "east-asia", label: "East Asia", lat: 35, lon: 123 },
        { id: "pacific", label: "Pacific", lat: -18, lon: -158 },
      ];
      function Demo() {
        const [selected, setSelected] = useState("europe");
        return <>
          <Globe regions={regions} selected={selected} onSelect={id => {
            window.selected.push(id);
            setSelected(id);
          }}/>
          <output>{selected}</output>
        </>;
      }
      window.selected = [];
      const root = createRoot(document.querySelector("#root"));
      root.render(<StrictMode><Demo/></StrictMode>);
      window.unmountGlobe = () => root.unmount();
    `,
  },
  jsx: "automatic", bundle: true, write: false, outdir: "globe-test",
});
const css = bundle.outputFiles.find(file => file.path.endsWith(".css")).text;
const js = bundle.outputFiles.find(file => file.path.endsWith(".js")).text;
const browser = await chromium.launch({
  executablePath: process.env.UI_SMOKE_EXECUTABLE,
  args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader"],
});

async function openPage(options, fallback = false) {
  const page = await browser.newPage(options);
  await page.setContent('<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body style="margin:0"><div id="root"></div></body></html>');
  if (fallback) await page.evaluate(() => {
    const context = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (name, ...args) {
      return name.startsWith("webgl") || name === "experimental-webgl" ? null : context.call(this, name, ...args);
    };
  });
  await page.addStyleTag({ content: css });
  await page.addScriptTag({ content: js });
  return page;
}

try {
  const page = await openPage({ viewport: { width: 960, height: 570 } });
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  const canvas = page.locator(".pg-globe-canvas");
  await page.getByRole("button", { name: "Europe", pressed: true }).waitFor();
  const remote = page.getByRole("button", { name: "Pacific", includeHidden: true });
  assert.equal(await remote.isVisible(), false);
  await remote.evaluate(button => button.click());
  assert.deepEqual(await page.evaluate(() => window.selected), []);
  await page.getByRole("button", { name: "Western Asia", exact: true }).click();
  await page.getByRole("button", { name: "Western Asia", pressed: true }).waitFor();

  if (process.env.PODCAST_GLOBE_SHOT) {
    await page.locator(".pg-globe").screenshot({ path: process.env.PODCAST_GLOBE_SHOT, animations: "disabled" });
  }
  const mark = page.getByRole("button", { name: "Western Asia", exact: true });
  const before = await mark.boundingBox();
  await page.mouse.move(480, 200);
  await page.mouse.wheel(0, -620);
  await page.waitForFunction(before => {
    const rect = document.querySelector('[aria-label="Western Asia"]').getBoundingClientRect();
    return Math.abs(rect.x - before.x) > 2 || Math.abs(rect.y - before.y) > 2;
  }, before);

  await page.mouse.move(500, 110);
  await page.mouse.down();
  await page.mouse.move(180, 110, { steps: 15 });
  await page.mouse.up();
  // Rotating from Europe across Asia exposes the previously hidden Pacific.
  await page.getByRole("button", { name: "Pacific" }).waitFor();
  await page.getByRole("button", { name: "Pacific" }).click();
  assert.deepEqual(await page.evaluate(() => window.selected), ["west-asia", "pacific"]);

  await canvas.focus();
  await page.keyboard.press("ArrowLeft");
  await page.evaluate(() => window.unmountGlobe());
  assert.equal(await canvas.count(), 0);
  assert.deepEqual(errors, []);
  await page.close();

  const phone = await openPage({
    viewport: { width: 390, height: 760 }, isMobile: true, hasTouch: true, reducedMotion: "reduce",
  });
  const dot = phone.getByRole("button", { name: "Africa" });
  await dot.waitFor();
  assert.equal(await dot.locator(".pg-globe-dot").evaluate(element => getComputedStyle(element, "::before").animationName), "none");
  await dot.tap();
  assert.deepEqual(await phone.evaluate(() => window.selected), ["africa"]);
  assert.equal(await phone.evaluate(() => document.documentElement.scrollWidth), 390);
  const mobileCanvas = phone.locator(".pg-globe-canvas");
  const cdp = await phone.context().newCDPSession(phone);
  await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: 300, y: 78 }] });
  for (const x of [277, 238, 190, 152, 130]) {
    await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x, y: 78 }] });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await phone.getByRole("button", { name: "Pacific" }).waitFor();
  await mobileCanvas.evaluate(canvas => canvas.getContext("webgl2").getExtension("WEBGL_lose_context").loseContext());
  await phone.getByText("Choose a podcast region", { exact: true }).waitFor();
  await phone.close();

  const fallback = await openPage({ viewport: { width: 390, height: 650 } }, true);
  await fallback.getByText("Choose a podcast region", { exact: true }).waitFor();
  assert.equal(await fallback.locator("canvas").count(), 0);
  await fallback.getByRole("button", { name: "Pacific" }).click();
  await fallback.getByRole("button", { name: "Pacific", pressed: true }).waitFor();
  assert.deepEqual(await fallback.evaluate(() => window.selected), ["pacific"]);
  console.log("PASS podcast globe drag, zoom, back-face picking, touch and WebGL fallback");
} finally {
  await browser.close();
}
