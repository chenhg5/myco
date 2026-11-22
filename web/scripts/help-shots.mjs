#!/usr/bin/env node
// The user guide's pictures (web/src/app/help/img/*.webp) from the UI
// test's screenshots: every ![…](shot:<name>) in the guide's pages is
// ui-smoke/<combo>-<name>.png, made smaller (at most 1600 px wide, WebP) so
// the App carries them lightly. Run the UI test first, at scale 2 for sharp
// pictures:
//
//   UI_SMOKE_SCALE=2 node scripts/ui-smoke.mjs --combo 1200-light
//   node scripts/help-shots.mjs            # writes what is missing or older
//   node scripts/help-shots.mjs --all      # writes them all again
//
// Pictures no page names any more are listed (remove them by hand).

import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const WEB = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const HELP = path.join(WEB, "src/app/help");
const IMG = path.join(HELP, "img");
const SHOTS = path.join(WEB, "ui-smoke");
const COMBO = process.env.HELP_SHOTS_COMBO || "1200x800-light";
const MAX_W = 1400;
const QUALITY = 0.78;
const all = process.argv.includes("--all");

const require = createRequire(import.meta.url);
let playwright;
try {
  playwright = require("playwright");
} catch {
  playwright = require(require.resolve("playwright", { paths: ["/opt/node-tools/node_modules", process.cwd()] }));
}

const names = new Set();
for (const lang of ["zh", "en"]) {
  for (const f of fs.readdirSync(path.join(HELP, lang))) {
    if (!f.endsWith(".md")) continue;
    const text = fs.readFileSync(path.join(HELP, lang, f), "utf8");
    for (const m of text.matchAll(/\]\(shot:([\w-]+)\)/g)) names.add(m[1]);
  }
}
fs.mkdirSync(IMG, { recursive: true });

const browser = await playwright.chromium.launch();
const page = await browser.newPage();
let wrote = 0;
const missing = [];
for (const name of [...names].sort()) {
  const src = path.join(SHOTS, `${COMBO}-${name}.png`);
  const out = path.join(IMG, `${name}.webp`);
  if (!fs.existsSync(src)) {
    if (!fs.existsSync(out)) missing.push(name);
    continue;
  }
  if (!all && fs.existsSync(out) && fs.statSync(out).mtimeMs >= fs.statSync(src).mtimeMs) continue;
  const data = fs.readFileSync(src).toString("base64");
  const webp = await page.evaluate(async ({ data, maxW, q }) => {
    const img = new Image();
    img.src = `data:image/png;base64,${data}`;
    await img.decode();
    const scale = Math.min(1, maxW / img.naturalWidth);
    const c = document.createElement("canvas");
    c.width = Math.round(img.naturalWidth * scale);
    c.height = Math.round(img.naturalHeight * scale);
    const g = c.getContext("2d");
    g.imageSmoothingQuality = "high";
    g.drawImage(img, 0, 0, c.width, c.height);
    return c.toDataURL("image/webp", q).split(",")[1];
  }, { data, maxW: MAX_W, q: QUALITY });
  fs.writeFileSync(out, Buffer.from(webp, "base64"));
  wrote++;
}
await browser.close();

const unused = fs.readdirSync(IMG).filter((f) => f.endsWith(".webp") && !names.has(f.slice(0, -5)));
const size = fs.readdirSync(IMG).reduce((n, f) => n + fs.statSync(path.join(IMG, f)).size, 0);
console.log(`help-shots: ${names.size} named, ${wrote} written, ${(size / 1024 / 1024).toFixed(1)} MB in help/img`);
if (missing.length) {
  console.log(`  no screenshot for: ${missing.join(", ")} (run the UI test, or fix the name)`);
  process.exitCode = 1;
}
if (unused.length) console.log(`  not used by any page: ${unused.join(", ")}`);
