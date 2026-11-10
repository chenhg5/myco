#!/usr/bin/env node
// App UI smoke test: starts the mock backend and Vite, opens every App page
// and the main dialogs with Playwright at two window sizes in light and dark,
// saves one screenshot each under web/ui-smoke/, and fails on console errors
// or uncaught page errors.
//
//   cd web && npm run test:ui
//
// Environment:
//   UI_SMOKE_PORT        Vite port (default 5173)
//   UI_SMOKE_EXECUTABLE  a Chromium/Chrome binary to drive, if Playwright's
//                        own browser is not downloaded on this machine
//
// This script never downloads browsers: Playwright and its browser must
// already be installed where Node can find them (web/node_modules, a global
// npm root, or NODE_PATH).
import { spawn, spawnSync, execFileSync } from "node:child_process";
import { createServer } from "node:net";
import { mkdir, access } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const WEB = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const OUT = path.join(WEB, "ui-smoke");
const HOST = "127.0.0.1";
const VITE_PORT = Number(process.env.UI_SMOKE_PORT || 5173);
// vite.config.ts proxies /app to this port, so it is not configurable.
const MOCK_PORT = 7781;
const BASE = `http://${HOST}:${VITE_PORT}`;
const VIEWPORTS = [
  { width: 1200, height: 800 },
  { width: 760, height: 800 },
];
const SCHEMES = ["light", "dark"];

const children = [];
let stopping = false;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function log(...args) {
  console.log(...args);
}

function portHint(port) {
  return process.platform === "win32"
    ? `find the process with: netstat -ano | findstr :${port}`
    : `find the process with: lsof -nP -iTCP:${port} -sTCP:LISTEN   (or: ss -ltnp 'sport = :${port}')`;
}

function portFree(port) {
  return new Promise((resolve, reject) => {
    const srv = createServer();
    srv.once("error", (err) => (err.code === "EADDRINUSE" ? resolve(false) : reject(err)));
    srv.once("listening", () => srv.close(() => resolve(true)));
    srv.listen({ port, host: HOST, exclusive: true });
  });
}

async function requirePort(port, what) {
  if (await portFree(port)) return;
  throw new Error(
    `Port ${port} is already in use (needed for ${what}).\n` +
    `Stop whatever is using it, then run again. ${portHint(port)}`,
  );
}

function start(name, args, env) {
  const child = spawn(process.execPath, args, {
    cwd: WEB,
    env: { ...process.env, NO_COLOR: "1", FORCE_COLOR: "0", ...env },
    stdio: ["ignore", "pipe", "pipe"],
    detached: process.platform !== "win32",
  });
  let output = "";
  const collect = (chunk) => {
    output = (output + chunk.toString()).slice(-6000);
  };
  child.stdout.on("data", collect);
  child.stderr.on("data", collect);
  child.on("error", (err) => {
    output += `\n${err.stack || err}`;
  });
  child.label = name;
  child.output = () => output.trim() || "(no output)";
  children.push(child);
  return child;
}

function stop(child) {
  if (!child || child.exitCode !== null || child.signalCode) return;
  try {
    if (process.platform === "win32") {
      spawnSync("taskkill", ["/pid", String(child.pid), "/T", "/F"], { stdio: "ignore" });
    } else {
      // Negative pid: the whole process group (Vite may start esbuild).
      process.kill(-child.pid, "SIGTERM");
    }
  } catch {
    try { child.kill("SIGTERM"); } catch { /* already gone */ }
  }
}

function stopAll() {
  if (stopping) return;
  stopping = true;
  for (const child of children) stop(child);
}

process.on("exit", stopAll);
process.on("SIGINT", () => { stopAll(); process.exit(130); });
process.on("SIGTERM", () => { stopAll(); process.exit(143); });

async function waitForHttp(url, child) {
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`${child.label} stopped before it was ready (exit ${child.exitCode}).\n${child.output()}`);
    }
    try {
      const res = await fetch(url);
      if (res.ok || res.status < 500) return;
    } catch {
      // not listening yet
    }
    await sleep(250);
  }
  throw new Error(`Timed out waiting for ${child.label} at ${url}.\n${child.output()}`);
}

async function loadPlaywright() {
  const attempts = [];
  const tryImport = async (specifier, label) => {
    try {
      const mod = await import(specifier);
      return { label, chromium: mod.chromium };
    } catch (err) {
      attempts.push(`${label}: ${err.code || err.message}`);
      return null;
    }
  };

  const local = await tryImport("playwright", "playwright (web/node_modules)");
  if (local) return local;
  const core = await tryImport("playwright-core", "playwright-core (web/node_modules)");
  if (core) return core;

  const roots = [];
  try {
    const npm = process.platform === "win32" ? "npm.cmd" : "npm";
    roots.push(execFileSync(npm, ["root", "-g"], { encoding: "utf8" }).trim());
  } catch { /* npm not available */ }
  for (const dir of (process.env.NODE_PATH || "").split(path.delimiter)) {
    if (dir) roots.push(dir);
  }
  for (const root of roots.filter(Boolean)) {
    // These are node_modules directories themselves, not project roots.
    const fromRoot = createRequire(path.join(root, "noop.js"));
    for (const pkg of ["playwright", "playwright-core"]) {
      try {
        const resolved = fromRoot.resolve(pkg);
        const mod = await import(pathToFileURL(resolved).href);
        return { label: `${pkg} (${resolved})`, chromium: mod.chromium };
      } catch (err) {
        attempts.push(`${pkg} under ${root}: ${err.code || err.message}`);
      }
    }
  }

  throw new Error(
    "Playwright is not installed where this script can find it.\n" +
    "This script does not run `playwright install`; install the package and its browser first " +
    "(or point UI_SMOKE_EXECUTABLE at an existing Chromium/Chrome).\n" +
    `Checked:\n  ${attempts.join("\n  ")}`,
  );
}

// The page a failure belongs to, set by step(); the window it happened in.
let currentStep = "启动";
let currentMatrix = "";
const problems = [];
const seenProblems = new Set();

function addProblem(kind, text, where) {
  const key = `${currentMatrix} ${currentStep} ${kind} ${text}`;
  if (seenProblems.has(key)) return;
  seenProblems.add(key);
  problems.push({ matrix: currentMatrix, step: currentStep, kind, text, where });
}

function reportProblems() {
  if (problems.length === 0) {
    log("✓ 没有 console error 或 pageerror。");
    return;
  }
  console.error(`\n✖ 发现 ${problems.length} 个页面错误：`);
  for (const p of problems) {
    const matrix = p.matrix ? `${p.matrix} · ` : "";
    console.error(`  [${matrix}${p.step}] ${p.kind}: ${p.text}${p.where ? `\n    ${p.where}` : ""}`);
  }
  process.exitCode = 1;
}

function watchPage(page, size, scheme) {
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const loc = msg.location();
    const where = loc.url ? `${loc.url}:${loc.lineNumber || 0}:${loc.columnNumber || 0}` : "";
    addProblem("console", msg.text(), where);
  });
  page.on("pageerror", (err) => {
    addProblem("pageerror", err.stack || String(err), "");
  });
  page.on("crash", () => addProblem("crash", "The page crashed", `${size.width}×${size.height} ${scheme}`));
}

async function settle(page, ms = 700) {
  await page.waitForTimeout(ms);
  await page.evaluate(async () => { await document.fonts?.ready; }).catch(() => undefined);
}

async function shot(page, size, scheme, slug) {
  const name = `${size.width}x${size.height}-${scheme}-${slug}.png`;
  await page.screenshot({ path: path.join(OUT, name) });
  log(`    截图 ui-smoke/${name}`);
}

async function sidebar(page, open) {
  const visible = await page.locator(".app-sidebar").isVisible().catch(() => false);
  if (visible === open) return;
  await page.getByRole("button", { name: /显示边栏|隐藏边栏|Show sidebar|Hide sidebar/ }).first().click();
  await page.waitForTimeout(250);
}

async function clickVisible(page, locator) {
  await locator.first().waitFor({ state: "visible" });
  await locator.first().click();
}

async function go(page, label, narrow) {
  await sidebar(page, true);
  await clickVisible(page, page.locator(".app-nav-item:visible", { hasText: label }));
  // At narrow widths the sidebar covers the window; show the page as a person
  // would after picking it, with the compact Hub pill in the crumb bar.
  if (narrow) await sidebar(page, false);
}

async function openHubSettings(page, narrow) {
  await sidebar(page, !narrow);
  const trigger = page.locator(narrow ? ".app-hub-pill:visible" : ".app-hub-trigger:visible");
  await clickVisible(page, trigger);
  await clickVisible(page, page.locator(".app-hub-menu-item:visible", { hasText: "设置" }));
}

async function closeDialog(page) {
  await page.keyboard.press("Escape");
  await page.locator(".app-modal").waitFor({ state: "hidden" });
  await page.waitForTimeout(200);
}

async function runMatrix(browser, size, scheme) {
  currentMatrix = `${size.width}×${size.height} ${scheme}`;
  log(`\n▶ ${currentMatrix}`);
  const context = await browser.newContext({
    viewport: size,
    colorScheme: scheme,
    locale: "zh-CN",
    deviceScaleFactor: 1,
  });
  await context.addInitScript(({ scheme }) => {
    try {
      localStorage.setItem("myco-theme", scheme);
      localStorage.setItem("myco-lang", "zh");
    } catch { /* storage unavailable */ }
  }, { scheme });

  const page = await context.newPage();
  watchPage(page, size, scheme);
  const narrow = size.width < 860;
  const step = async (label, fn) => {
    currentStep = label;
    log(`  · ${label}`);
    await fn();
    await page.waitForTimeout(250);
  };

  try {
    currentStep = "启动";
    await page.goto(`${BASE}/?lang=zh`, { waitUntil: "domcontentloaded" });
    await page.locator(".app-shell").waitFor();
    await page.locator(".app-row:visible").first().waitFor({ timeout: 20_000 });
    await settle(page);

    await step("节点", async () => {
      await shot(page, size, scheme, "nodes");
    });

    await step("节点 · 添加节点弹窗", async () => {
      await clickVisible(page, page.getByRole("button", { name: /添加节点|Add node/ }));
      await page.getByRole("dialog").waitFor();
      await settle(page, 500);
      await shot(page, size, scheme, "dialog-add-node");
      await closeDialog(page);
    });

    await step("机器", async () => {
      await go(page, "机器", narrow);
      await page.locator(".app-page .app-row:visible, .app-page .app-empty:visible").first().waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "machines");
    });

    await step("对话", async () => {
      await go(page, "对话", narrow);
      await page.locator(".conv-row:visible").first().waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "conversations");
    });

    await step("对话 · 新对话弹窗", async () => {
      await clickVisible(page, page.getByRole("button", { name: /新对话|New conversation/ }));
      await page.getByRole("dialog").waitFor();
      await settle(page, 500);
      await shot(page, size, scheme, "dialog-new-conversation");
      await closeDialog(page);
    });

    await step("对话详情", async () => {
      await clickVisible(page, page.locator(".conv-row:visible"));
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "conversation-detail");
    });

    await step("项目页", async () => {
      await sidebar(page, true);
      const project = page.locator(".app-nav-item:visible", { hasText: "官网改版" });
      await project.waitFor({ timeout: 20_000 });
      await project.first().click();
      if (narrow) await sidebar(page, false);
      await page.locator(".app-project").waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "project");
    });

    await step("项目页 · 新建项目弹窗", async () => {
      await sidebar(page, true);
      await clickVisible(page, page.locator(".app-nav-add:visible"));
      await page.getByRole("dialog").waitFor();
      await settle(page, 400);
      await shot(page, size, scheme, "dialog-new-project");
      await closeDialog(page);
    });

    await step("设置", async () => {
      await openHubSettings(page, narrow);
      await page.locator(".app-settings").waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "settings");
    });
  } finally {
    await context.close().catch(() => undefined);
  }
}

async function main() {
  log("myco App · 界面冒烟测试");
  log(`  mock 后端 http://${HOST}:${MOCK_PORT}`);
  log(`  Vite      ${BASE}`);

  await requirePort(MOCK_PORT, "the mock backend");
  await requirePort(VITE_PORT, "Vite");

  const viteBin = path.join(WEB, "node_modules", "vite", "bin", "vite.js");
  try {
    await access(viteBin);
  } catch {
    throw new Error(`Vite is not installed (${viteBin} is missing). Run: cd web && npm install`);
  }
  const mockEntry = path.join(WEB, "mock", "app-server.mjs");
  await access(mockEntry);

  const { label, chromium } = await loadPlaywright();
  log(`  Playwright ${label}`);

  let browser;
  try {
    browser = await chromium.launch({
      headless: true,
      ...(process.env.UI_SMOKE_EXECUTABLE ? { executablePath: process.env.UI_SMOKE_EXECUTABLE } : {}),
    });
  } catch (err) {
    throw new Error(
      `Playwright is installed (${label}) but could not start a browser.\n${err.message}\n` +
      "This script does not download browsers. Install the browser first, or set " +
      "UI_SMOKE_EXECUTABLE to a Chromium/Chrome binary.",
    );
  }

  await mkdir(OUT, { recursive: true });

  try {
    const mock = start("mock 后端", [mockEntry], { APP_MOCK_PORT: String(MOCK_PORT) });
    await waitForHttp(`http://${HOST}:${MOCK_PORT}/app/hubs`, mock);

    const vite = start("Vite", [viteBin, "--host", HOST, "--port", String(VITE_PORT), "--strictPort"], {
      UI_SMOKE: "1",
    });
    await waitForHttp(BASE, vite);

    for (const size of VIEWPORTS) {
      for (const scheme of SCHEMES) {
        await runMatrix(browser, size, scheme);
      }
    }
  } finally {
    await browser.close().catch(() => undefined);
    stopAll();
  }

  const count = VIEWPORTS.length * SCHEMES.length;
  log(`\n${count} 个窗口组合，全部页面截图完成。`);
  reportProblems();
}

main().catch((err) => {
  console.error(`\n✖ 冒烟测试失败于「${currentMatrix ? `${currentMatrix} · ` : ""}${currentStep}」：\n${err.stack || err.message}`);
  if (problems.length > 0) reportProblems();
  stopAll();
  process.exitCode = 1;
});
