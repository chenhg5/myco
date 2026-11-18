#!/usr/bin/env node
// App UI smoke test: builds the App once, then opens every App page and the
// main dialogs with Playwright at two window sizes in light and dark, saves
// one screenshot each under web/ui-smoke/, and fails on console errors or
// uncaught page errors. The four sizes and themes run at the same time, each
// against its own mock backend and a small server for the built App, so a
// step in one never sees what another did.
//
//   cd web && npm run test:ui
//   npm run test:ui -- --only 单聊            # just the steps whose name has 单聊
//   npm run test:ui -- --combo 1200-light     # one size and theme (or 760-dark, ...)
//   npm run test:ui -- --serial               # one after another (easier to read)
//
// Environment:
//   UI_SMOKE_PORT        first port of the App servers (default 5173; one each)
//   UI_SMOKE_SCALE       screenshots' device scale (default 1; 2 for ones people see)
//   UI_SMOKE_EXECUTABLE  a Chromium/Chrome binary to drive, if Playwright's
//                        own browser is not downloaded on this machine
//
// This script never downloads browsers: Playwright and its browser must
// already be installed where Node can find them (web/node_modules, a global
// npm root, or NODE_PATH).
import { spawn, spawnSync, execFileSync } from "node:child_process";
import { createServer } from "node:net";
import http from "node:http";
import { AsyncLocalStorage } from "node:async_hooks";
import { mkdir, access, readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const WEB = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const OUT = path.join(WEB, "ui-smoke");
const HOST = "127.0.0.1";
// A 1x1 PNG, for choosing a picture as a face.
const TINY_PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";
const APP_PORT = Number(process.env.UI_SMOKE_PORT || 5173);
const MOCK_PORT = 7781;
const VIEWPORTS = [
  { width: 1200, height: 800 },
  { width: 760, height: 800 },
];
const SCHEMES = ["light", "dark"];

// Command line: --only <text> (steps whose name has it), --combo <w-scheme>
// (repeatable or comma separated), --serial.
function flag(name) {
  const out = [];
  const argv = process.argv.slice(2);
  argv.forEach((a, i) => {
    if (a === `--${name}` && argv[i + 1]) out.push(argv[i + 1]);
    else if (a.startsWith(`--${name}=`)) out.push(a.slice(name.length + 3));
  });
  return out.flatMap((x) => x.split(",")).map((x) => x.trim()).filter(Boolean);
}
const ONLY = flag("only");
const COMBOS = flag("combo");
const SERIAL = process.argv.includes("--serial");

// What each running size and theme has of its own: its mock backend, the
// App's address, its log and the step it is on.
const runCtx = new AsyncLocalStorage();
const run = () => runCtx.getStore();
const mockPort = () => run()?.mockPort ?? MOCK_PORT;

const children = [];
let stopping = false;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Inside a size and theme the lines are kept and printed together when it
// ends, so four at once do not interleave.
function log(...args) {
  const r = run();
  if (r) r.lines.push(args.join(" "));
  else console.log(...args);
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
      return { label, chromium: mod.chromium ?? mod.default?.chromium };
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
        return { label: `${pkg} (${resolved})`, chromium: mod.chromium ?? mod.default?.chromium };
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
const problems = [];
const seenProblems = new Set();

function addProblem(r, kind, text, where) {
  const key = `${r.matrix} ${r.step} ${kind} ${text}`;
  if (seenProblems.has(key)) return;
  seenProblems.add(key);
  problems.push({ matrix: r.matrix, step: r.step, kind, text, where });
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
  // Page events come outside this run's async context: keep it here.
  const r = run();
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const loc = msg.location();
    const where = loc.url ? `${loc.url}:${loc.lineNumber || 0}:${loc.columnNumber || 0}` : "";
    addProblem(r, "console", msg.text(), where);
  });
  page.on("pageerror", (err) => {
    addProblem(r, "pageerror", err.stack || String(err), "");
  });
  page.on("crash", () => addProblem(r, "crash", "The page crashed", `${size.width}×${size.height} ${scheme}`));
}

async function settle(page, ms = 700) {
  await page.waitForTimeout(ms);
  await page.evaluate(async () => { await document.fonts?.ready; }).catch(() => undefined);
}

// The App at rest uses little CPU: a render loop (a state set on every
// render) keeps the main thread busy without showing anything wrong.
async function staysIdle(page, what, ms = 2000) {
  const cdp = await page.context().newCDPSession(page);
  try {
    await cdp.send("Performance.enable");
    const busy = async () => (await cdp.send("Performance.getMetrics")).metrics.find((x) => x.name === "TaskDuration").value;
    const a = await busy();
    await page.waitForTimeout(ms);
    const share = (await busy() - a) / (ms / 1000);
    if (share > 0.5) throw new Error(`${what}时 App 应该闲着，实际主线程忙了 ${Math.round(share * 100)}%（可能在一直重新渲染）`);
  } finally {
    await cdp.detach().catch(() => undefined);
  }
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

function compactText(s) {
  return (s || "").replace(/\s+/g, " ").trim();
}

// ⌘ on a Mac, Ctrl elsewhere: the key that puts the next message behind the
// running one.
const NEXT_KEY = process.platform === "darwin" ? "Meta+Enter" : "Control+Enter";

/** A message written from "another device": straight to the mock, so two
 *  tasks can be on their way at once (the box adds to the running one now). */
async function sendFromElsewhere(hubId, agent, body) {
  const r = await fetch(`http://${HOST}:${mockPort()}/app/hubs/${hubId}/messages`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ to: agent, body }),
  });
  if (!r.ok) throw new Error(`直接发一条失败：HTTP ${r.status}`);
  return r.json();
}

/** Waits until no round is running: what was said is on screen and the box
 *  says "发给" again, not "补充给". */
async function noRound(page) {
  await page.waitForFunction(() => {
    const d = document.querySelector(".conv-detail");
    const el = d?.querySelector(".conv-compose-input");
    // Loaded (some messages, or the empty chat when a step runs alone).
    return !!d && !!el && (d.querySelectorAll(".conv-msg").length > 0 || !!d.querySelector(".conv-empty"))
      && !(el.getAttribute("placeholder") || "").includes("补充");
  }, null, { timeout: 30_000 });
}

/** Opens your chat with a member of a space (a project, or the agents in
 *  none): from the members column, or the row of faces in a narrow window. */
async function openMember(page, narrow, space, name) {
  await go(page, space, narrow);
  await page.locator(".sp-group").waitFor({ timeout: 20_000 });
  if (narrow) await clickVisible(page, page.locator(`.sp-strip-face[title="${name}"]`));
  else await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: name }));
  await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
}

async function go(page, label, narrow) {
  await sidebar(page, true);
  if (label === "设置") {
    // Settings sit in the workspace menu at the bottom of the sidebar.
    await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
    await clickVisible(page, page.locator(".app-hub-menu-item", { hasText: "设置" }));
    if (narrow) await sidebar(page, false);
    return;
  }
  // Computers is a heading over the computers listed in the sidebar;
  // members are managed from the chat list's menu.
  if (label === "成员") {
    await clickVisible(page, page.locator(".sp-nav-chat-sort"));
    await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "管理成员" }));
  } else {
    const item = label === "电脑" ? page.locator(".sp-nav-pcs-head:visible")
      : label === "聊天" ? page.locator(".sp-nav-chat:visible")
      : label === "自动化" ? page.locator(".sp-nav-auto:visible")
      : label === "应用" ? page.locator(".sp-nav-templates:visible")
      : page.locator(".app-nav-item:visible", { hasText: label });
    await clickVisible(page, item);
  }
  // At narrow widths the sidebar covers the window; show the page as a person
  // would after picking it, with the compact Hub pill in the crumb bar.
  if (narrow) await sidebar(page, false);
}

// Right-click a row and pick from its menu. Scrolling the row into view
// fires a scroll a moment later, which closes a menu (as it should for a
// person scrolling): so again, until the menu stays.
async function menuPick(page, row, item) {
  for (let i = 0; ; i++) {
    await row.scrollIntoViewIfNeeded();
    await page.waitForTimeout(300);
    await row.click({ button: "right" });
    const it = page.locator(".conv-menu").getByRole("menuitem", { name: item, exact: true });
    if (await it.waitFor({ timeout: 1_500 }).then(() => true, () => false)
      && await it.click({ timeout: 1_500 }).then(() => true, () => false)) return;
    await page.keyboard.press("Escape");
    if (i >= 4) throw new Error(`右键菜单没出现「${item}」`);
  }
}

async function closeDialog(page) {
  await page.keyboard.press("Escape");
  await page.locator(".app-modal").waitFor({ state: "hidden" });
  await page.waitForTimeout(200);
}

async function runMatrix(browser, size, scheme) {
  const r = run();
  log(`\n▶ ${r.matrix}`);
  // The stuck-task step stops and resends the demo task; the mock keeps
  // running, so put it back before each width and theme.
  const reset = await fetch(`http://${HOST}:${mockPort()}/app/mock/stuck`, { method: "POST" });
  if (!reset.ok) throw new Error(`重置「可能卡住」的演示任务失败：HTTP ${reset.status}`);
  // (its times count from now: its last progress 40 minutes ago, the task 45)
  const stuckResetAt = Date.now();
  // The machine-delete step removes the demo machine and its nodes.
  const resetMachine = await fetch(`http://${HOST}:${mockPort()}/app/mock/machine`, { method: "POST" });
  if (!resetMachine.ok) throw new Error(`重置演示机器失败：HTTP ${resetMachine.status}`);
  // A workspace on this computer is started by name below; put the demo
  // ones back so every width and theme starts with the same two.
  const resetLocal = await fetch(`http://${HOST}:${mockPort()}/app/mock/local`, { method: "POST" });
  if (!resetLocal.ok) throw new Error(`重置这台电脑上的工作区失败：HTTP ${resetLocal.status}`);
  // The resume step makes and revokes a line; put the demo one back.
  const resetResume = await fetch(`http://${HOST}:${mockPort()}/app/mock/resume`, { method: "POST" });
  if (!resetResume.ok) throw new Error(`重置重新接入链接失败：HTTP ${resetResume.status}`);
  // The run step changes a node's model and effort; put the demo ones back.
  const resetRun = await fetch(`http://${HOST}:${mockPort()}/app/mock/run-settings`, { method: "POST" });
  if (!resetRun.ok) throw new Error(`重置模型设置失败：HTTP ${resetRun.status}`);
  const resetHandovers = await fetch(`http://${HOST}:${mockPort()}/app/mock/handovers`, { method: "POST" });
  if (!resetHandovers.ok) throw new Error(`重置接手记录失败：HTTP ${resetHandovers.status}`);
  const context = await browser.newContext({
    viewport: size,
    colorScheme: scheme,
    locale: "zh-CN",
    deviceScaleFactor: Number(process.env.UI_SMOKE_SCALE) || 1,
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
    // --only: just the steps named; the others are left out.
    if (ONLY.length && !ONLY.some((x) => label.includes(x))) return;
    r.step = label;
    const t0 = Date.now();
    log(`  · ${label}`);
    await fn();
    await page.waitForTimeout(250);
    r.timings.push({ label, ms: Date.now() - t0 });
  };

  try {
    r.step = "启动";
    await page.goto(`${r.base}/?lang=zh`, { waitUntil: "domcontentloaded" });
    await page.locator(".app-shell").waitFor();
    await page.locator(".sp-home, .sp-group, .sp-first").first().waitFor({ timeout: 20_000 });
    await settle(page);

    // A project opens on your chat with its coordinator; the tasks it hands
    // out are cards in that chat; the members column says what each does.
    await step("项目 · 和协调者聊", async () => {
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 20_000 });
      // The project's head says where you are: the chat has no second header.
      if (await page.locator(".sp-home .conv-head").count()) throw new Error("项目首页的对话不该再有自己的标题栏（和项目标题重复）");
      if (!(await page.locator(".sp-home .conv-compose").first().isVisible())) throw new Error("项目首页应能直接和协调者说话");
      await page.locator(".sp-home .handoff").first().waitFor({ timeout: 20_000 });
      const card = compactText(await page.locator(".sp-home .handoff").first().innerText());
      if (!card.startsWith("派给")) throw new Error(`派活卡片应写明派给谁，实际是「${card}」`);
      // What came back is under it, one line.
      const back = compactText(await page.locator(".sp-home .handoff", { hasText: "升级注意事项" }).locator(".handoff-result").innerText());
      if (!back.includes("改好了")) throw new Error(`交回的结果应在派活卡片下面一行，实际是「${back}」`);
      // What a member sent the coordinator is a card too (it was not shown at all).
      const sent = page.locator(".sp-home .handoff.is-in", { hasText: "先别让 release 装" });
      await sent.waitFor({ timeout: 10_000 });
      const sentText = compactText(await sent.innerText());
      if (!/^docs-claude\s*发来/.test(sentText)) throw new Error(`成员发给协调者的应写「X 发来」，实际是「${sentText}」`);
      if (!sentText.includes("等你补完")) throw new Error("协调者回它的那句应在卡片下面一行");
      if (!narrow) await page.locator(".sp-side .sp-role", { hasText: "协调者" }).waitFor({ timeout: 10_000 });
      // The coordinator heads the members; whoever is at work comes next.
      if (!narrow) {
        const order = await page.locator(".sp-side .sp-side-group").first().locator(".sp-member").evaluateAll((els) => els.map((e) => ({
          coord: !!e.querySelector(".sp-role"), working: !!e.querySelector(".is-working"), away: e.classList.contains("is-away"),
        })));
        if (!order[0]?.coord) throw new Error("成员列表第一个应是协调者");
        const rest = order.slice(1);
        const lastWorking = rest.map((x) => x.working).lastIndexOf(true);
        const firstIdle = rest.findIndex((x) => !x.working);
        if (lastWorking >= 0 && firstIdle >= 0 && firstIdle < lastWorking) throw new Error("在干活的成员应排在没在干活的前面");
        // The box you write in lines up with the messages above it.
        const edges = await page.evaluate(() => {
          const tl = document.querySelector(".sp-home .conv-timeline");
          const box = document.querySelector(".sp-home .conv-compose-box")?.getBoundingClientRect();
          if (!tl || !box) return null;
          const cs = getComputedStyle(tl); const t = tl.getBoundingClientRect();
          return [t.left + parseFloat(cs.paddingLeft) - box.left, t.right - parseFloat(cs.paddingRight) - box.right];
        });
        if (edges && (Math.abs(edges[0]) > 6 || Math.abs(edges[1]) > 6)) throw new Error(`输入框应和上面的对话左右对齐，差了 ${edges.map(Math.round).join(" / ")}px`);
      }
      await settle(page);
      await shot(page, size, scheme, "project-home");
    });

    // A link in a chat opens in the default browser (the desktop app's web
    // view opens no new windows): the App hands it over, no tab of its own.
    await step("链接用浏览器打开", async () => {
      const link = page.locator(".sp-home .conv-timeline a", { hasText: "发布页" }).first();
      await link.scrollIntoViewIfNeeded();
      const before = page.context().pages().length;
      await link.click();
      await page.waitForFunction(async () => {
        const r = await (await fetch("/app/mock/opened")).json();
        return r.urls.includes("https://example.com/releases/v0.16.8");
      }, null, { timeout: 5_000 });
      await page.waitForTimeout(300);
      if (page.context().pages().length !== before) throw new Error("链接应交给默认浏览器打开，不该在 App 里开新窗口");
    });

    // The model and effort beside the message box: switched from there for
    // an agent a runner wakes (from its next run).
    await step("输入框 · 模型和推理强度", async () => {
      await go(page, "官网改版", narrow);
      const pill = page.locator(".sp-home .conv-compose .run-pill");
      await pill.waitFor({ timeout: 10_000 });
      const text = compactText(await pill.innerText());
      if (!/GPT-6 Sol/.test(text) || !/高/.test(text)) throw new Error(`输入框旁应写着模型和推理强度，实际「${text}」`);
      await pill.click();
      const menu = page.locator(".sp-home .run-pill-menu");
      await menu.waitFor({ timeout: 5_000 });
      await settle(page, 150);
      await shot(page, size, scheme, "composer-model");
      await menu.getByRole("menuitemradio", { name: "中", exact: true }).click();
      await page.waitForFunction(() => /中/.test(document.querySelector(".sp-home .conv-compose .run-pill")?.textContent || ""), null, { timeout: 10_000 });
      // (back as it was: the other windows share the mock)
      await pill.click();
      await page.locator(".sp-home .run-pill-menu").getByRole("menuitemradio", { name: "高", exact: true }).click();
      await page.waitForFunction(() => /高/.test(document.querySelector(".sp-home .conv-compose .run-pill")?.textContent || ""), null, { timeout: 10_000 });
      // In its own session and not said yet: 模型未知, and how it comes to be known.
      if (narrow) await clickVisible(page, page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" }));
      const chat = page.locator(".sp-pane-right");
      const unknown = chat.locator(".conv-compose .run-pill");
      await unknown.waitFor({ timeout: 10_000 });
      const said = compactText(await unknown.innerText());
      if (said !== "模型未知") throw new Error(`没上报模型的 agent 应写「模型未知」，实际「${said}」`);
      await unknown.click();
      const note = chat.locator(".run-pill-menu");
      await note.waitFor({ timeout: 5_000 });
      if (!/runtime set/.test(await note.innerText())) throw new Error("点开应说明怎么让它报上模型");
      await settle(page, 150);
      await shot(page, size, scheme, "composer-model-unknown");
      await chat.locator(".run-pill-scrim").click({ position: { x: 5, y: 5 } });
      await note.waitFor({ state: "detached", timeout: 5_000 });
      await clickVisible(page, chat.getByRole("button", { name: "关掉这一栏" }));
    });

    // A page an agent published: its link in the reply is a card; it opens
    // in the page panel (beside the chat when there is room), from another
    // origin; a link in it goes to the browser; a newer version is said.
    await step("网页 · agent 做的网页", async () => {
      await go(page, "官网改版", narrow);
      if (narrow) await clickVisible(page, page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" }));
      const chat = page.locator(".sp-pane-right");
      const card = chat.locator(".pg-card", { hasText: "v0.16.8 发布说明" });
      await card.waitFor({ timeout: 15_000 });
      if (!(await card.innerText()).includes("Release (Claude 云端) 发布")) throw new Error("卡片上应写由谁发布");
      await card.scrollIntoViewIfNeeded();
      await settle(page, 200);
      await shot(page, size, scheme, "page-card");
      await clickVisible(page, card);
      const panel = page.locator(".pg-panel");
      await panel.waitFor({ timeout: 10_000 });
      const frame = page.frameLocator(".pg-frame");
      await frame.locator("h1", { hasText: "发布说明" }).waitFor({ timeout: 15_000 });
      if ((await frame.locator(".bar").count()) !== 3) throw new Error("网页里的脚本应该跑起来，画出三条");
      const src = await page.locator(".pg-frame").getAttribute("src");
      if (!src || new URL(src).origin === new URL(page.url()).origin) throw new Error(`网页应该在另一个源里打开：${src}`);
      if (!narrow && !(await page.evaluate(() => document.documentElement.classList.contains("fp-docked")))) throw new Error("宽窗口里网页应停在对话旁边");
      await settle(page, 300);
      await shot(page, size, scheme, "page-panel");
      // A link out of the page: the browser.
      await frame.locator("#out").dispatchEvent("click");
      await page.waitForFunction(async () => {
        const r = await (await fetch("/app/mock/opened")).json();
        return r.urls.includes("https://github.com/chenhg5/myco/releases");
      }, null, { timeout: 5_000 });
      // A newer version: said at the top; shown when asked.
      const v = await page.evaluate(async () => (await (await fetch("/app/mock/pages/pg_release/publish", { method: "POST" })).json()).version);
      await panel.locator(".pg-new", { hasText: `第 ${v} 版` }).waitFor({ timeout: 25_000 });
      await clickVisible(page, panel.locator(".pg-new").getByRole("button", { name: "看新版本" }));
      await frame.locator(".sub", { hasText: `第 ${v} 版` }).waitFor({ timeout: 10_000 });
      await clickVisible(page, panel.getByRole("button", { name: "关闭" }));
      await panel.waitFor({ state: "detached", timeout: 5_000 });
      await clickVisible(page, chat.getByRole("button", { name: "关掉这一栏" }));
    });

    // A live page: a dev server on the agent's computer, forwarded; its
    // card says so and whether it is on.
    await step("网页 · 实时预览", async () => {
      await go(page, "官网改版", narrow);
      if (narrow) await clickVisible(page, page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" }));
      const chat = page.locator(".sp-pane-right");
      const card = chat.locator(".pg-card", { hasText: "下载页（实时）" });
      await card.waitFor({ timeout: 15_000 });
      await card.locator(".pg-live.is-on", { hasText: "实时 · 在线" }).waitFor({ timeout: 5_000 });
      if (!(await card.innerText()).includes("5173 端口")) throw new Error("实时网页的卡片应写是哪台电脑的哪个端口");
      await clickVisible(page, card);
      const panel = page.locator(".pg-panel");
      await page.frameLocator(".pg-frame").locator("h1", { hasText: "下载 myco" }).waitFor({ timeout: 15_000 });
      if (!(await panel.locator(".fp-titles").innerText()).includes("实时")) throw new Error("面板上应写这是实时的");
      const src = await page.locator(".pg-frame").getAttribute("src");
      if (!src || !/\/$/.test(new URL(src).pathname)) throw new Error(`实时网页应从根路径打开：${src}`);
      await settle(page, 300);
      await shot(page, size, scheme, "page-live");
      await clickVisible(page, panel.getByRole("button", { name: "关闭" }));
      await panel.waitFor({ state: "detached", timeout: 5_000 });
      await clickVisible(page, chat.getByRole("button", { name: "关掉这一栏" }));
    });

    // Several pages open: a tab each, under the panel's head; opening one
    // already open shows it; each keeps its page while another is shown.
    if (!narrow) await step("网页 · 多个标签", async () => {
      await go(page, "官网改版", narrow);
      await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" }));
      const chat = page.locator(".sp-pane-right");
      const panel = page.locator(".pg-panel");
      await clickVisible(page, chat.locator(".pg-card", { hasText: "v0.16.8 发布说明" }));
      await page.frameLocator(".pg-view:not(.is-hidden) .pg-frame").locator("h1").waitFor({ timeout: 15_000 });
      if (await panel.locator(".pg-tab").count()) throw new Error("只开一个网页时不该有标签");
      await clickVisible(page, chat.locator(".pg-card", { hasText: "下载页（实时）" }));
      await panel.locator(".pg-tab.is-on", { hasText: "下载页" }).waitFor({ timeout: 10_000 });
      await page.frameLocator(".pg-view:not(.is-hidden) .pg-frame").locator("h1", { hasText: "下载 myco" }).waitFor({ timeout: 15_000 });
      if ((await panel.locator(".pg-tab").count()) !== 2) throw new Error("开两个网页应有两个标签");
      await settle(page, 300);
      await shot(page, size, scheme, "page-tabs");
      // Back to the first: its page was kept (one frame each).
      await clickVisible(page, panel.locator(".pg-tab", { hasText: "发布说明" }).locator(".pg-tab-main"));
      await panel.locator(".fp-titles h3", { hasText: "发布说明" }).waitFor({ timeout: 5_000 });
      if ((await page.locator(".pg-frame").count()) !== 2) throw new Error("切换标签不该重新加载另一个网页");
      // Tucked away (收起): the pages stay, the chat gets its room back, a
      // button in the corner brings them back as they were.
      const mainRight = () => page.evaluate(() => document.querySelector(".app-main").getBoundingClientRect().right);
      const docked = await mainRight();
      await clickVisible(page, panel.getByRole("button", { name: "收起" }));
      // (the panel is one for the whole App: its toggle is at the top bar's right end)
      const restore = page.locator(".app-crumbbar .pg-toggle");
      await restore.waitFor({ timeout: 5_000 });
      if (!(await restore.innerText()).includes("2") || (await restore.getAttribute("aria-pressed")) !== "false") throw new Error("收起后顶栏右边的「网页」应写着有几个、不是按下的");
      if (await panel.isVisible()) throw new Error("收起后面板应藏起来");
      if ((await page.locator(".pg-frame").count()) !== 2) throw new Error("收起时网页应保留");
      await page.waitForFunction((d) => document.querySelector(".app-main").getBoundingClientRect().right > d + 100, docked, { timeout: 5_000 });
      await settle(page, 250);
      await shot(page, size, scheme, "page-tucked");
      await clickVisible(page, restore);
      await panel.locator(".fp-titles h3", { hasText: "发布说明" }).waitFor({ timeout: 5_000 });
      if ((await restore.getAttribute("aria-pressed")) !== "true") throw new Error("面板展开时顶栏的「网页」应是按下的");
      await page.waitForFunction((d) => Math.abs(document.querySelector(".app-main").getBoundingClientRect().right - d) <= 2, docked, { timeout: 5_000 })
        .catch(() => { throw new Error("展开后对话应回到原来的宽度"); });
      // Dragging the panel's edge: the chat's edge follows (no gap).
      const edge = await panel.locator(".fp-edge").boundingBox();
      await page.mouse.move(edge.x + edge.width / 2, edge.y + 200);
      await page.mouse.down();
      for (let x = 0; x <= 120; x += 20) await page.mouse.move(edge.x + edge.width / 2 + x, edge.y + 200);
      await page.mouse.up();
      await page.waitForTimeout(400);
      const gap = await page.evaluate(() => document.querySelector(".pg-panel").getBoundingClientRect().left - document.querySelector(".app-main").getBoundingClientRect().right);
      if (gap < 0 || gap > 20) throw new Error(`拖动面板后，对话和面板之间应只有一条缝，实际 ${Math.round(gap)}px`);
      await panel.locator(".fp-edge").dblclick();
      // Opening one already open shows it, no new tab.
      await clickVisible(page, chat.locator(".pg-card", { hasText: "下载页（实时）" }));
      await panel.locator(".pg-tab.is-on", { hasText: "下载页" }).waitFor({ timeout: 5_000 });
      if ((await panel.locator(".pg-tab").count()) !== 2) throw new Error("开着的网页再点一次不该多一个标签");
      await staysIdle(page, "开着两个网页");
      // Closing a tab; the last one closes the panel.
      await clickVisible(page, panel.locator(".pg-tab", { hasText: "下载页" }).getByRole("button", { name: /关闭/ }));
      await panel.locator(".fp-titles h3", { hasText: "发布说明" }).waitFor({ timeout: 5_000 });
      if (await panel.locator(".pg-tab").count()) throw new Error("剩一个网页时标签应收起");
      // × closes them all.
      await clickVisible(page, chat.locator(".pg-card", { hasText: "下载页（实时）" }));
      await panel.locator(".pg-tab", { hasText: "下载页" }).waitFor({ timeout: 5_000 });
      await clickVisible(page, panel.getByRole("button", { name: "关闭全部 2 个网页" }));
      await panel.waitFor({ state: "detached", timeout: 5_000 });
      if (await page.locator(".pg-toggle").count()) throw new Error("全部关闭后顶栏不该再有「网页」");
      await clickVisible(page, chat.getByRole("button", { name: "关掉这一栏" }));
    });

    // The myco bridge: the page's shared state, written by you, changed by
    // its agent (it comes to the page), and a message to its agent, asked
    // once. (The four windows share the mock: versions are read, not assumed.)
    await step("网页 · 和 agent 一起动手", async () => {
      await go(page, "官网改版", narrow);
      if (narrow) await clickVisible(page, page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" }));
      const chat = page.locator(".sp-pane-right");
      await clickVisible(page, chat.locator(".pg-card", { hasText: "上线清单" }));
      const panel = page.locator(".pg-panel");
      const frame = page.frameLocator(".pg-frame");
      await frame.locator("#who", { hasText: "状态第" }).waitFor({ timeout: 15_000 });
      const st = () => page.evaluate(async () => (await (await fetch("/app/hubs/hub_studio/pages/pg_checklist/state")).json()));
      // You tick an item: it is saved on the hub.
      const before = await st();
      const box = frame.locator('input[data-i="3"]');
      await box.dispatchEvent("click");
      await page.waitForFunction(async (v) => (await (await fetch("/app/hubs/hub_studio/pages/pg_checklist/state")).json()).version > v, before.version, { timeout: 10_000 });
      // Its agent writes: the page shows it.
      const mark = 100 + Math.floor(Math.random() * 1000);
      await page.evaluate(async (m) => { await fetch("/app/mock/pages/pg_checklist/state", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ state: { done: [0, 1, 2], mark: m } }) }); }, mark);
      await frame.locator("#who", { hasText: "Release (Claude 云端) 改了" }).waitFor({ timeout: 10_000 });
      if (!(await frame.locator('input[data-i="2"]').isChecked())) throw new Error("agent 改的状态应推到网页里");
      if ((await st()).state.mark !== mark) throw new Error("agent 写的状态应是最新的");
      await settle(page, 200);
      await shot(page, size, scheme, "page-bridge");
      // A message to its agent: asked once.
      await frame.locator("#hand").dispatchEvent("click");
      const ask = page.locator(".pg-ask");
      await ask.waitFor({ timeout: 5_000 });
      await settle(page, 250);
      await shot(page, size, scheme, "page-bridge-ask");
      await clickVisible(page, ask.getByRole("button", { name: "允许并发送" }));
      await frame.locator("#note", { hasText: "已发给 agent" }).waitFor({ timeout: 10_000 });
      await chat.locator(".conv-msg.is-me", { hasText: "来自网页「上线清单」" }).first().waitFor({ timeout: 10_000 });
      await clickVisible(page, panel.getByRole("button", { name: "关闭" }));
      await panel.waitFor({ state: "detached", timeout: 5_000 });
      await clickVisible(page, chat.getByRole("button", { name: "关掉这一栏" }));
    });

    // 置顶: what a conversation keeps at hand, a row of tabs under its
    // header. The agent pins (myco pin; here the mock's API), you pin a
    // message and an address; a tab opens its thing (the message is
    // brought to the middle, a page opens in the panel); right-click
    // renames and unpins.
    await step("置顶 · 对话上方的标签", async () => {
      await go(page, "官网改版", narrow);
      if (narrow) await clickVisible(page, page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" }));
      const chat = page.locator(".sp-pane-right");
      await chat.locator(".pg-card", { hasText: "v0.16.8 发布说明" }).waitFor({ timeout: 15_000 });
      const api = (method, path, data) => fetch(`http://${HOST}:${mockPort()}/app/hubs/hub_studio/pins${path}`, { method, headers: { "Content-Type": "application/json" }, body: data ? JSON.stringify(data) : undefined });
      // Start clean (the windows share nothing, but a step may run again).
      for (const p of (await (await api("GET", "?with=release-cloud")).json()).pins) await api("DELETE", `/${p.id}?with=release-cloud`);
      await api("POST", "", { with: "release-cloud", kind: "link", target: "http://192.168.1.20:7780/pages/pg_release", title: "发布说明网页", by: "release-cloud" });
      await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-pins-changed", { detail: { hubId: "hub_studio" } })));
      const bar = chat.locator("[data-testid=pin-bar]");
      await bar.locator(".pin-chip", { hasText: "发布说明网页" }).waitFor({ timeout: 10_000 });
      // You pin a message: the tab is its first words, not its link.
      const msg = chat.locator(".conv-msg").filter({ has: page.locator(".pg-card", { hasText: "v0.16.8 发布说明" }) }).first();
      await msg.hover();
      await clickVisible(page, msg.locator(".conv-msg-pin"));
      const msgTab = bar.locator(".pin-chip.is-message");
      await msgTab.waitFor({ timeout: 5_000 });
      if ((await msgTab.innerText()).includes("http")) throw new Error(`置顶消息的标签不该带链接，实际「${await msgTab.innerText()}」`);
      await msg.hover();
      await msg.getByRole("button", { name: "取消置顶" }).waitFor({ timeout: 5_000 });
      // An address, from the header's ⋯ menu.
      await clickVisible(page, chat.locator("[data-testid=chat-more]"));
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "置顶一个网址" }));
      await page.locator("[data-testid=pin-url]").fill("https://example.com/design");
      await page.locator("[data-testid=pin-title]").fill("设计稿");
      await clickVisible(page, page.getByRole("button", { name: "置顶", exact: true }));
      await bar.locator(".pin-chip", { hasText: "设计稿" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "pins");
      // The message's tab brings it to the middle and lights it up.
      await chat.locator(".conv-scroll").first().evaluate((el) => { el.scrollTop = el.scrollHeight; }).catch(() => undefined);
      await clickVisible(page, msgTab);
      await chat.locator(".is-flash").first().waitFor({ timeout: 5_000 });
      // The page's tab opens it in the panel.
      await clickVisible(page, bar.locator(".pin-chip", { hasText: "发布说明网页" }));
      const panel = page.locator(".pg-panel");
      await panel.waitFor({ timeout: 10_000 });
      await clickVisible(page, panel.getByRole("button", { name: "关闭" }));
      await panel.waitFor({ state: "detached", timeout: 5_000 });
      // Right-click: rename, then unpin. (The menu closes on any scroll,
      // and the chat may still be settling after the panel closed: again.)
      const menuOn = (chip, item) => menuPick(page, chip, item);
      const design = bar.locator(".pin-chip", { hasText: "设计稿" });
      await menuOn(design, "重命名");
      await page.locator("[data-testid=pin-title]").fill("设计稿 v2");
      await clickVisible(page, page.getByRole("button", { name: "保存", exact: true }));
      const renamed = bar.locator(".pin-chip", { hasText: "设计稿 v2" });
      await renamed.waitFor({ timeout: 5_000 });
      await menuOn(renamed, "取消置顶");
      await renamed.waitFor({ state: "detached", timeout: 5_000 });
      // (the tab goes at once; the hub hears of it a moment later)
      let left = [];
      for (let i = 0; i < 25; i++) {
        left = (await (await api("GET", "?with=release-cloud")).json()).pins.map((p) => p.title);
        if (left.length === 2 && !left.includes("设计稿 v2")) break;
        await page.waitForTimeout(200);
      }
      if (left.length !== 2 || left.includes("设计稿 v2")) throw new Error(`取消置顶后 Hub 上应剩两个，实际 ${left.join(" / ")}`);
      // A page's card in the chat: right-click pins it above the chat (here
      // it already is, as 发布说明网页: the card says so), or takes it off.
      const card = chat.locator(".pg-card", { hasText: "v0.16.8 发布说明" }).first();
      await card.locator(".pg-card-pinned").waitFor({ timeout: 5_000 });
      await menuOn(card, "取消置顶");
      await bar.locator(".pin-chip", { hasText: "发布说明网页" }).waitFor({ state: "detached", timeout: 5_000 });
      await menuOn(card, "置顶到对话上方");
      await bar.locator(".pin-chip", { hasText: "v0.16.8 发布说明" }).waitFor({ timeout: 5_000 });
      await card.locator(".pg-card-pinned").waitFor({ timeout: 5_000 });
      await clickVisible(page, chat.getByRole("button", { name: "关掉这一栏" }));
    });

    // A page made from a template (the table myco template init writes):
    // its records through the bridge; you edit a cell (saved with its
    // version), the agent adds a row (it shows up), and it exports a CSV to
    // Downloads.
    await step("应用 · 人和 agent 一起填表", async () => {
      await go(page, "官网改版", narrow);
      await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-open-page", { detail: { kind: "page", hubId: "hub_studio", id: "pg_table" } })));
      const frame = page.frameLocator(".pg-frame");
      await frame.locator("tbody tr").first().waitFor({ timeout: 15_000 });
      const before = await frame.locator("tbody tr").count();
      const title = frame.locator("tbody tr").first().locator("input").first();
      const mark = `${size.width}-${scheme}`;
      await title.fill(`第一行（${mark}）`);
      await title.press("Tab");
      await frame.locator("#note", { hasText: "已保存" }).waitFor({ timeout: 10_000 });
      const saved = await (await fetch(`http://${HOST}:${mockPort()}/app/hubs/hub_studio/pages/pg_table/data?collection=rows`)).json();
      if (!saved.records.some((r) => r.data.title === `第一行（${mark}）`)) throw new Error("改的格子应存到网页的记录里");
      // The agent adds a row: it shows up, saying who.
      await fetch(`http://${HOST}:${mockPort()}/app/mock/pages/pg_table/data/rows`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ data: { title: `agent 加的（${mark}）`, status: "待办" } }) });
      await frame.locator("tbody tr").nth(before).waitFor({ timeout: 10_000 });
      await frame.locator("#note", { hasText: "刚改了" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "template-table");
      // Exported: a CSV saved to Downloads, with what is in the table.
      await frame.locator("#export").dispatchEvent("click");
      await frame.locator("#note", { hasText: "已存到" }).waitFor({ timeout: 10_000 });
      const dl = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/last-page-download`)).json();
      if (!dl.name?.endsWith(".csv") || !dl.text?.includes(`agent 加的（${mark}）`)) throw new Error(`导出的 CSV 不对：${JSON.stringify(dl).slice(0, 200)}`);
      await clickVisible(page, page.locator(".pg-panel").getByRole("button", { name: /关闭/ }).first());
    });

    // 应用 (templates): myco's own first, then the workspace's; one makes a page in the
    // panel (with sample data), listed under them to open again. The board
    // and calendar of 多维表格, and a document typed with shortcuts.
    await step("应用 · 应用库和官方应用", async () => {
      await go(page, "应用", narrow);
      const lib = page.locator("[data-testid=templates-official] [data-shelf=office] .tp-card");
      await lib.first().waitFor({ timeout: 15_000 });
      const names = (await lib.allInnerTexts()).map(compactText);
      if (names.length !== 4 || !names[0].startsWith("表格") || !names[1].startsWith("多维表格") || !names[2].startsWith("文档") || !names[3].startsWith("幻灯片")) throw new Error(`办公套件应是 表格、多维表格、文档、幻灯片，实际 ${names.join(" / ")}`);
      await page.locator("[data-testid=templates-own] .tp-card").first().waitFor({ timeout: 5_000 });
      // myco's own apps update on their own; fetched now on request.
      await clickVisible(page, page.locator("[data-testid=apps-sync]"));
      await page.locator(".tp-sync-note", { hasText: "已是最新" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "templates");
      const mark = `${size.width}-${scheme}`;
      // (a narrow window: the page panel covers the sidebar; tucked away first)
      const tuck = async () => {
        const panel = page.locator(".pg-panel:visible");
        if (await panel.count()) {
          await clickVisible(page, panel.getByRole("button", { name: "收起" }).first());
          await page.locator(".pg-panel").waitFor({ state: "hidden", timeout: 5_000 }).catch(() => undefined);
        }
      };
      const make = async (slug, name) => {
        await tuck();
        await go(page, "应用", narrow);
        await clickVisible(page, page.locator(`[data-testid="template-${slug}"]`));
        await page.locator("[data-testid=template-name]").fill(name);
        await clickVisible(page, page.getByRole("button", { name: "新建", exact: true }));
        const frame = page.locator(`.pg-frame[title="${name}"]`);
        await frame.waitFor({ timeout: 15_000 });
        return page.frameLocator(`.pg-frame[title="${name}"]`);
      };
      // 表格: a grid (canvas) with the sample rows.
      const sheet = await make("myco/table", `本周（${mark}）`);
      await sheet.locator("[data-testid=grid] canvas").first().waitFor({ timeout: 15_000 });
      await sheet.locator(".toolbar-count", { hasText: "共 3 行" }).waitFor({ state: "attached", timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "template-grid");
      // 多维表格: its records, as a board and a calendar.
      const base = await make("myco/base", `招聘（${mark}）`);
      await base.locator("[data-testid=grid] canvas").first().waitFor({ timeout: 15_000 });
      await base.locator(".toolbar-count", { hasText: "共 4 条" }).waitFor({ state: "attached", timeout: 10_000 });
      await base.locator(".tab", { hasText: "看板" }).dispatchEvent("click");
      await base.locator(".col-head", { hasText: "面试中" }).waitFor({ timeout: 5_000 });
      if ((await base.locator(".card").count()) !== 4) throw new Error("看板上应有 4 张卡片");
      await settle(page, 300);
      await shot(page, size, scheme, "template-board");
      await base.locator(".card", { hasText: "李想" }).dispatchEvent("click");
      await base.locator(".drawer .drawer-title").waitFor({ timeout: 5_000 });
      await base.locator(".drawer").getByRole("button", { name: "关闭" }).dispatchEvent("click");
      await base.locator(".tab", { hasText: "日历" }).dispatchEvent("click");
      await base.locator(".cal").waitFor({ timeout: 5_000 });
      await base.locator(".tab", { hasText: "表格" }).dispatchEvent("click");
      // 幻灯片: the sample deck, laid out; a point added in place.
      const deck = await make("myco/slides", `发布会（${mark}）`);
      await deck.locator(".stage .sl").waitFor({ timeout: 15_000 });
      if ((await deck.locator(".thumb").count()) !== 8) throw new Error("幻灯片应带着 8 页示例");
      await deck.locator(".thumb-face").nth(3).dispatchEvent("click");
      await deck.locator(".stage .sl.is-two .chart").waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "template-slides");
      // 文档: typed with shortcuts, each block saved.
      const doc = await make("myco/doc", `计划（${mark}）`);
      await doc.locator(".bn-editor .bn-inline-content").first().waitFor({ timeout: 15_000 });
      const last = doc.locator(".bn-editor .bn-inline-content").last();
      await doc.locator(".bn-editor").focus();
      await last.evaluate((n) => { const r = document.createRange(); r.selectNodeContents(n); r.collapse(false); const sel = getSelection(); sel.removeAllRanges(); sel.addRange(r); });
      await settle(page, 200);
      await page.keyboard.press("End");
      await page.keyboard.press("Enter");
      await page.keyboard.type("## 风险");
      await page.keyboard.press("Enter");
      await page.keyboard.type("- 下载页 CDN 可能没刷新");
      await doc.locator('[data-content-type="bulletListItem"]', { hasText: "下载页 CDN" }).waitFor({ timeout: 5_000 });
      await doc.locator('[data-content-type="heading"]', { hasText: "风险" }).waitFor({ timeout: 5_000 });
      await settle(page, 1200);
      await shot(page, size, scheme, "template-doc");
      // The pages made are listed under the templates.
      await tuck();
      await go(page, "应用", narrow);
      await page.locator(".tp-made-row", { hasText: `计划（${mark}）` }).waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".pg-toggle"));
      await clickVisible(page, page.locator(".pg-panel").getByRole("button", { name: /关闭/ }).first());
      // (back where the steps after this one start: the project)
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home, .sp-group").first().waitFor({ timeout: 10_000 });
    });

    // A name picked in the notch's island (the Mac app): a chat already on
    // screen stays as it is (its pane is pointed out); a member of the
    // project you look at opens beside the coordinator; from elsewhere
    // (Settings), the app goes to it.
    if (!narrow) await step("灵动岛 · 打开成员", async () => {
      const pick = (agent, project) => page.evaluate(async ({ agent, project }) => {
        const ov = await (await fetch("/app/overview")).json();
        const hub = ov.hubs.find((h) => (h.overview?.agents || []).some((a) => a.agent.name === agent));
        window.dispatchEvent(new CustomEvent("myco-open-member", { detail: { hubId: hub.id, agent, project } }));
      }, { agent, project });
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
      // The coordinator, on screen: nothing moves, its pane is pointed out.
      await pick("pm-codex", "website");
      await page.locator(".sp-home.is-called").waitFor({ timeout: 5_000 });
      if (await page.locator(".sp-pane-right").count()) throw new Error("协调者已经在屏幕上，不该再打开别的栏");
      // A member of this project: beside the coordinator.
      await pick("docs-claude", "website");
      const right = page.locator(".sp-pane-right");
      await right.locator(".conv-head h2", { hasText: "docs-claude" }).waitFor({ timeout: 10_000 });
      if (!(await page.locator(".sp-pane-left .sp-home").isVisible())) throw new Error("打开成员时，协调者应还在左边");
      // Again, while it shows: it stays, pointed out.
      await pick("docs-claude", "website");
      await page.locator(".sp-pane-right.is-called").waitFor({ timeout: 5_000 });
      // From Settings: the app goes to it.
      await go(page, "设置", narrow);
      await pick("docs-claude", "website");
      await page.locator(".sp-pane-right .conv-head h2", { hasText: "docs-claude" }).waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".sp-pane-right").getByRole("button", { name: "关掉这一栏" }));
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    // The members column folds to a strip of faces, and stays so.
    // Where another session took the coordinator over, its chat has a quiet
    // line saying so; its menu in the members column offers it again.
    await step("项目 · 换过会话的地方有一条线", async () => {
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 20_000 });
      const line = page.locator(".sp-home [data-testid=handover-divider]").first();
      await line.waitFor({ state: "attached", timeout: 10_000 });
      const said = compactText(await line.innerText());
      if (!said.includes("换成 Studio Mac · Codex 接着做") || !said.includes("你换的")) throw new Error(`分隔线的字不对：「${said}」`);
      const sc = page.locator(".sp-home .conv-scroll").first();
      const box = await sc.boundingBox();
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
      for (let i = 0; i < 12; i++) { await page.mouse.wheel(0, -600); await page.waitForTimeout(50); }
      await settle(page, 300);
      await shot(page, size, scheme, "handover-divider");
      if (!narrow) {
        await page.locator(".sp-side .sp-member:visible", { hasText: "PM (Codex)" }).first().click({ button: "right" });
        await page.getByRole("menuitem", { name: "换一个来接着做" }).waitFor({ timeout: 5_000 });
        await page.keyboard.press("Escape");
      }
    });

    if (!narrow) await step("项目 · 成员栏收起", async () => {
      await page.getByRole("button", { name: "收起成员" }).click();
      await page.locator(".sp-side.is-folded").waitFor({ timeout: 5_000 });
      const faces = await page.locator(".sp-side.is-folded .sp-face:not(.is-add)").count();
      if (faces < 2) throw new Error(`收起后应只留头像，实际 ${faces} 个`);
      if (await page.locator(".sp-side.is-folded .sp-member").count()) throw new Error("收起后不该再有名字和状态");
      await shot(page, size, scheme, "project-members-folded");
      await page.reload({ waitUntil: "domcontentloaded" });
      await page.locator(".sp-side.is-folded").waitFor({ timeout: 20_000 });
      await page.getByRole("button", { name: "展开成员" }).click();
      await page.locator(".sp-side:not(.is-folded) .sp-member").first().waitFor({ timeout: 5_000 });
    });

    // A card opens that member's work beside the coordinator: what the
    // coordinator asked (named) and its answers, with yours, at that task.
    await step("项目 · 派出的活", async () => {
      const text = "升级注意事项";
      await clickVisible(page, page.locator(".sp-home .handoff", { hasText: text }));
      const right = page.locator(".sp-pane-right");
      // Their whole exchange, to read: the task, its answer, the notes.
      await right.locator(".conv-msg", { hasText: text }).first().waitFor({ timeout: 20_000 });
      await right.locator(".conv-msg", { hasText: "等你补完" }).first().waitFor({ timeout: 10_000 });
      await page.waitForFunction((t) => {
        const box = document.querySelector(".sp-pane-right .conv-scroll")?.getBoundingClientRect();
        const el = [...document.querySelectorAll(".sp-pane-right .conv-msg")].find((m) => (m.textContent || "").includes(t));
        if (!el || !box) return false;
        const r = el.getBoundingClientRect();
        return r.bottom > box.top && r.top < box.bottom;
      }, text, { timeout: 10_000 });
      const head = compactText(await right.locator(".conv-head h2").innerText());
      if (!/docs-claude/.test(head) || !/PM/.test(head)) throw new Error(`派活卡片应打开它们俩的往来，实际是「${head}」`);
      // Wide enough, the coordinator stays on the left.
      if (!narrow && !(await page.locator(".sp-pane-left .sp-home").isVisible())) throw new Error("打开成员时，和协调者的对话应该还在左边");
      // Side by side, the two headers are one height: their lines meet.
      if (!narrow) {
        const hs = await page.evaluate(() => [".sp-pane-left .sp-head", ".sp-pane-right .conv-head"].map((q) => {
          const r = document.querySelector(q)?.getBoundingClientRect();
          return r ? Math.round(r.bottom) : -1;
        }));
        if (hs.includes(-1) || Math.abs(hs[0] - hs[1]) > 1) throw new Error(`左右两栏的标题栏应一样高，底边在 ${hs.join(" / ")}`);
      }
      // Its head's buttons (find, full width, close) are one group: even
      // gaps, one icon size.
      const btns = await right.locator(".conv-head button:visible").evaluateAll((els) => els
        .filter((b) => b.closest(".conv-head-actions"))
        .map((b) => ({ l: b.getBoundingClientRect().left, r: b.getBoundingClientRect().right, icon: Math.round(b.querySelector("svg")?.getBoundingClientRect().width || 0) })));
      if (btns.length >= 2) {
        const gaps = btns.slice(1).map((b, i) => Math.round(b.l - btns[i].r));
        if (Math.max(...gaps) - Math.min(...gaps) > 1) throw new Error(`右栏标题栏的按钮间距应一样，实际 ${gaps.join(" / ")}px`);
        if (new Set(btns.map((b) => b.icon)).size > 1) throw new Error(`右栏标题栏的图标应一样大，实际 ${btns.map((b) => b.icon).join(" / ")}px`);
      }
      await settle(page, 300);
      await shot(page, size, scheme, "project-member-work");
      const hb = await right.locator(".conv-head").boundingBox();
      if (hb) await page.screenshot({ path: path.join(OUT, `${size.width}x${size.height}-${scheme}-project-member-head.png`), clip: { x: hb.x + hb.width - 220, y: hb.y, width: 220, height: hb.height } });
      await clickVisible(page, right.getByRole("button", { name: "关掉这一栏" }));
      await page.locator(".sp-pane-right").waitFor({ state: "detached", timeout: 10_000 });
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    // In a member's chat, what it and the others said to each other is a
    // card each, not bubbles in your conversation; a card opens theirs.
    if (!narrow) await step("项目 · 成员之间的往来", async () => {
      await clickVisible(page, page.locator(".sp-side .sp-member", { hasText: "docs-claude" }).first());
      const right = page.locator(".sp-pane-right");
      const note = right.locator(".handoff.is-out", { hasText: "先别让 release 装" });
      await note.waitFor({ timeout: 20_000 });
      const line = compactText(await note.innerText());
      if (!line.startsWith("发给")) throw new Error(`它发给别人的应写「发给 X」，实际是「${line}」`);
      await right.locator(".handoff.is-in", { hasText: "升级注意事项" }).waitFor({ timeout: 10_000 });
      if (await right.locator(".conv-msg", { hasText: "先别让 release 装" }).count()) throw new Error("成员之间的往来不该画成你对话里的气泡");
      if (await right.locator(".conv-msg", { hasText: "升级注意事项" }).count()) throw new Error("别人派给它的活不该画成你对话里的气泡");
      await settle(page, 300);
      await shot(page, size, scheme, "project-member-handoffs");
      await clickVisible(page, note);
      await right.locator(".conv-head h2", { hasText: "PM" }).waitFor({ timeout: 10_000 });
      await clickVisible(page, right.getByRole("button", { name: "关掉这一栏" }));
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    // Everything between the members, to look back at: no box to write in.
    await step("项目 · 全部动态", async () => {
      await clickVisible(page, page.locator(".sp-head").getByRole("button", { name: "全部动态" }));
      await page.locator(".sp-group .sp-card").first().waitFor({ timeout: 20_000 });
      if (await page.locator(".sp-group .sp-compose").count()) throw new Error("全部动态是记录，不该有输入框");
      await settle(page);
      await shot(page, size, scheme, "project-activity");
      await clickVisible(page, page.locator(".sp-head button[aria-label='返回']"));
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    // A member opens on the right; either pane can take the whole width.
    await step("项目 · 单聊", async () => {
      if (narrow) await clickVisible(page, page.locator(".sp-strip-face:not(.is-add)").nth(1));
      else await clickVisible(page, page.locator(".sp-side .sp-member").nth(1));
      const right = page.locator(".sp-pane-right");
      await right.locator(".conv-head-faces .pa").first().waitFor({ timeout: 20_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "project-chat");
      if (!narrow) {
        const mainPane = page.locator('.pg-pane[data-pane="main"]');
        await clickVisible(page, right.getByRole("button", { name: "只看这一栏" }));
        await page.locator('.pg-pane[data-pane="main"].is-hidden').waitFor({ state: "attached", timeout: 5_000 });
        await settle(page, 200);
        await shot(page, size, scheme, "project-chat-full");
        // Full width stays while you switch: the coordinator takes it too.
        await clickVisible(page, page.locator(".sp-side .sp-member").first());
        await page.locator(".pg-pane.sp-pane-right.is-hidden").waitFor({ state: "attached", timeout: 5_000 });
        await clickVisible(page, page.locator(".sp-side .sp-member").nth(1));
        await page.locator('.pg-pane[data-pane="main"].is-hidden').waitFor({ state: "attached", timeout: 5_000 });
        await clickVisible(page, right.getByRole("button", { name: "还原所有栏" }));
        await page.locator('.pg-pane[data-pane="main"]:not(.is-hidden)').waitFor({ timeout: 5_000 });
        if (await page.locator(".pg-pane.is-hidden").count()) throw new Error("还原后每一栏都该看得到");
        void mainPane;
      }
      await clickVisible(page, right.getByRole("button", { name: "关掉这一栏" }));
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    // Up to four panes: a member opened with 「在新的一栏打开」 goes beside
    // the others, then below; one dragged onto an edge of a pane opens
    // there. A window too small for them all shows them as tabs.
    await step("项目 · 多栏", async () => {
      const grid = page.locator(".pg-grid");
      const dropOn = async (from, x, y) => {
        const dt = await page.evaluateHandle(() => new DataTransfer());
        await from.dispatchEvent("dragstart", { dataTransfer: dt });
        const at = { dataTransfer: dt, clientX: x, clientY: y };
        await grid.dispatchEvent("dragenter", at);
        await grid.dispatchEvent("dragover", at);
        return async () => grid.dispatchEvent("drop", at);
      };
      if (narrow) {
        // (no members column: the faces in a row) One beside, one below.
        const room = async () => page.locator(".pg-room").boundingBox();
        let b = await room();
        await (await dropOn(page.locator(".sp-strip-face:not(.is-add)").nth(1), b.x + b.width - 10, b.y + b.height / 2))();
        await page.locator(".pg-pane").nth(1).waitFor({ state: "attached", timeout: 10_000 });
        b = await room();
        await (await dropOn(page.locator(".sp-strip-face:not(.is-add)").nth(2), b.x + b.width / 4, b.y + b.height - 10))();
        await page.locator(".pg-pane").nth(2).waitFor({ state: "attached", timeout: 10_000 });
        // Smaller than they need: tabs, one shown.
        const vp = page.viewportSize();
        await page.setViewportSize({ width: 560, height: vp.height });
        await page.locator(".pg-tab").nth(2).waitFor({ timeout: 10_000 });
        if ((await page.locator(".pg-pane:not(.is-hidden)").count()) !== 1) throw new Error("标签页时应只显示一栏");
        await settle(page, 300);
        await shot(page, size, scheme, "project-panes-tabs");
        await page.setViewportSize(vp);
        for (let i = 0; i < 2; i++) {
          const close = page.locator(".pg-pane.sp-pane-right:not(.is-hidden)").getByRole("button", { name: "关掉这一栏" }).first();
          if (await close.count()) await close.click();
          else await page.locator(".pg-tab .pg-tab-x").last().click();
          await page.waitForTimeout(200);
        }
        await page.locator(".pg-pane.sp-pane-right").first().waitFor({ state: "detached", timeout: 5_000 });
        return;
      }
      const members = page.locator(".sp-side .sp-member:not(.is-add)");
      await clickVisible(page, members.nth(1));
      await page.locator(".pg-pane.sp-pane-right").first().waitFor({ timeout: 10_000 });
      // One already on screen is not opened again: dragged in, its pane says so.
      {
        const rb = await page.locator(".pg-room").boundingBox();
        const again = await dropOn(members.nth(1), rb.x + rb.width / 4, rb.y + rb.height / 2);
        await page.locator(".pg-drop.is-open").waitFor({ timeout: 5_000 });
        await again();
        await page.waitForTimeout(300);
        if ((await page.locator(".pg-pane").count()) !== 2) throw new Error("已经开着的成员拖进来不该再开一栏");
      }
      // (the members column folds into faces once two panes share the
      // room: the next comes from the sidebar's chats)
      await menuPick(page, page.locator(".sp-nav-person", { hasText: "docs-claude" }), "在新的一栏打开");
      await page.locator(".pg-pane.sp-pane-right").nth(1).waitFor({ timeout: 10_000 });
      // A third, dragged onto the bottom edge of the coordinator's pane:
      // where it would go lights up first.
      const mb = await page.locator('.pg-pane[data-pane="main"]').boundingBox();
      const drop = await dropOn(page.locator(".sp-nav-person", { hasText: "legacy-codex" }), mb.x + mb.width / 2, mb.y + mb.height - 12);
      await page.locator(".pg-drop:not(.is-no)").waitFor({ timeout: 5_000 });
      await drop();
      await page.locator(".pg-pane").nth(3).waitFor({ state: "attached", timeout: 10_000 });
      const shown = await page.locator(".pg-pane:not(.is-hidden)").count();
      const tabs = await page.locator(".pg-tab").count();
      if (shown !== 4 && tabs !== 4) throw new Error(`应有四栏（或四个标签），实际 ${shown} 栏、${tabs} 个标签`);
      await settle(page, 400);
      await shot(page, size, scheme, "project-panes");
      // Close them all: the coordinator's pane alone again.
      for (let i = 0; i < 3; i++) {
        const close = page.locator(".pg-pane.sp-pane-right:not(.is-hidden)").getByRole("button", { name: "关掉这一栏" }).first();
        if (await close.count()) await close.click();
        else await page.locator(".pg-tab .pg-tab-x").last().click();
        await page.waitForTimeout(200);
      }
      await page.locator(".pg-pane.sp-pane-right").first().waitFor({ state: "detached", timeout: 5_000 });
    });

    // Three ways in, as tabs: a session on this computer joins with a click;
    // the sentence is short (the link carries the project and the steps);
    // a new session starts on a computer. Each shows up at once.
    await step("项目 · 添加 agent", async () => {
      if (narrow) await clickVisible(page, page.locator(".sp-strip-face.is-add"));
      else await clickVisible(page, page.locator(".sp-side .sp-side-add").first());
      const dialog = page.locator(".app-modal-add");
      await dialog.waitFor();
      // Members of the workspace not in the project come first.
      await dialog.getByRole("tab", { name: "已有的", selected: true }).waitFor({ timeout: 10_000 });
      await dialog.locator(".add-local-row button", { hasText: "加进来" }).first().waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-members");
      await dialog.locator(".add-local-row button", { hasText: "加进来" }).first().click();
      await dialog.locator(".add-arrived").first().waitFor({ timeout: 20_000 });

      await clickVisible(page, dialog.getByRole("tab", { name: "这台电脑上的" }));
      await dialog.locator(".add-local-row button", { hasText: "加入" }).first().waitFor({ timeout: 10_000 });
      // A session that joined two workspaces names both.
      await dialog.locator(".add-local-row.is-taken .add-local-sub", { hasText: /已经加入了：.*的 glenn-mac-claude-x9k2、.*tapnow.* 的 glenn-mac-claude-x9k2-tn/ }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent");
      await dialog.locator(".add-local-row button", { hasText: "加入" }).first().click();
      await dialog.locator(".add-arrived").nth(1).waitFor({ timeout: 20_000 });

      await clickVisible(page, dialog.getByRole("tab", { name: "发一句话" }));
      await page.waitForFunction(() => {
        const t = document.querySelector(".add-sentence-text")?.textContent || "";
        return /\/j\/\S+\/\S+ \| bash/.test(t) && t.includes("最后的输出") && !t.includes("inbox wait");
      }, null, { timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-link");

      await clickVisible(page, dialog.getByRole("tab", { name: "新开一个" }));
      await dialog.locator(".ws-computer.active").first().waitFor({ timeout: 10_000 });
      // A newer computer makes ~/myco/<name> when no folder is given; model,
      // effort and access are under "more".
      await dialog.locator(".app-field-label", { hasText: "文件夹干活（可以不填）" }).waitFor({ timeout: 5_000 });
      const folder = dialog.locator("input.app-input.mono").first();
      if (!/~\/myco\//.test(await folder.getAttribute("placeholder") || "")) throw new Error("文件夹可以不填时，提示里应写会新建的 ~/myco/… 文件夹");
      await clickVisible(page, dialog.locator("details.app-advanced summary", { hasText: "更多选项" }));
      await dialog.locator("select.app-select-field").selectOption("high");
      await clickVisible(page, dialog.getByRole("tab", { name: "只读" }));
      await dialog.locator(".app-field-hint", { hasText: "只看不改" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-new-options");
      await clickVisible(page, dialog.getByRole("tab", { name: "这个文件夹" }));
      await clickVisible(page, dialog.locator("details.app-advanced summary", { hasText: "更多选项" }));
      await folder.fill("~/code/website");
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-new");
      await clickVisible(page, dialog.getByRole("button", { name: "开一个" }));
      await dialog.locator(".add-arrived").nth(2).waitFor({ timeout: 20_000 });
      await clickVisible(page, dialog.getByRole("button", { name: "好了" }));
      await page.locator(".app-modal").waitFor({ state: "hidden" });
    });

    if (!narrow) {
      await step("项目 · 添加工作空间", async () => {
        // Adding a computer is one command, as adding an agent is: run it in
        // the folder on that computer, and the dialog ticks itself; from a
        // project, the folder is in the project at once.
        await clickVisible(page, page.locator(".sp-side").getByRole("button", { name: "加一台电脑" }));
        const dialog = page.locator(".app-modal-add");
        await dialog.waitFor();
        const box = dialog.locator("[data-testid=computer-command]");
        await box.waitFor({ timeout: 10_000 });
        // What it means is said calmly (no red block).
        if (await dialog.locator(".app-warn").count()) throw new Error("加电脑的弹窗不该有红色警告块");
        const cmd = box.locator(".app-command code");
        await cmd.filter({ hasText: "curl -fsSL" }).waitFor({ timeout: 10_000 });
        if (/shell=1|machine\.yaml|max_access/.test(await cmd.innerText())) throw new Error("一条命令就够，不该再带权限开关或让人改文件");
        await clickVisible(page, box.getByRole("tab", { name: "Windows（PowerShell）" }));
        await cmd.filter({ hasText: "irm " }).waitFor({ timeout: 5_000 });
        await clickVisible(page, box.getByRole("tab", { name: "Mac / Linux / WSL" }));
        await box.locator(".ws-command-wait").waitFor({ timeout: 5_000 });
        // Or the agent on that computer runs it: the sentence carries this command.
        const way = box.locator("[data-testid=computer-agent-way] .add-sentence-text");
        if (!(await way.innerText()).includes("这台电脑加进 myco 工作区") || !(await way.innerText()).includes((await cmd.innerText()).trim())) throw new Error("加电脑的弹窗里应有发给 agent 的那句话，带着同一条命令");
        await settle(page, 300);
        await shot(page, size, scheme, "add-workspace");
        // "Running" it there: the folder shows up, in the project.
        const ran = await fetch(`http://${HOST}:${mockPort()}/app/mock/run-join`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ dir: `~/code/wsl-${size.width}-${scheme}` }) });
        if (!ran.ok) throw new Error(`mock run-join: ${ran.status}`);
        await box.locator("[data-testid=computer-arrived]", { hasText: `~/code/wsl-${size.width}-${scheme}` }).waitFor({ timeout: 20_000 });
        await settle(page, 200);
        await shot(page, size, scheme, "add-workspace-done");
        await clickVisible(page, box.getByRole("button", { name: "完成" }));
        await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 10_000 });
        await page.locator(".sp-side .sp-member", { hasText: `wsl-${size.width}-${scheme}` }).waitFor({ timeout: 20_000 });

        // A folder on a computer already connected can still be picked; a
        // computer that has not allowed terminals says so there, with the
        // command to run on it, and cannot be added yet.
        await clickVisible(page, page.locator(".sp-side").getByRole("button", { name: "加一台电脑" }));
        await dialog.waitFor();
        await clickVisible(page, dialog.getByRole("button", { name: "用已经连上的电脑选文件夹" }));
        await dialog.locator(".ws-computer.active").waitFor({ timeout: 10_000 });
        const office = dialog.locator(".ws-computer", { hasText: "Office PC" });
        await office.locator(".ws-computer-sub", { hasText: "还没允许" }).waitFor({ timeout: 10_000 });
        await clickVisible(page, office);
        await dialog.locator("[data-testid=shell-ask] code", { hasText: "myco machine shell-access --allow" }).waitFor({ timeout: 5_000 });
        await dialog.locator("input.app-input").fill("~/code/office");
        if (await dialog.locator("button[type=submit]").isEnabled()) throw new Error("没允许开终端的电脑不该能点添加");
        await settle(page, 200);
        await shot(page, size, scheme, "add-workspace-not-allowed");
        await dialog.locator("input.app-input").fill("");
        await clickVisible(page, dialog.locator(".ws-computer").first());
        const folder = `~/code/smoke-${size.width}-${scheme}`;
        await dialog.locator("input.app-input").fill(folder);
        await dialog.locator("button[type=submit]").click();
        await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 20_000 });
        const shell = `smoke-${size.width}-${scheme}-shell`;
        await page.locator(".sp-side .sp-member", { hasText: shell }).waitFor({ timeout: 20_000 });
      });
    }

    await step("项目资料", async () => {
      await clickVisible(page, page.locator(".sp-head").getByRole("button", { name: "项目资料" }));
      await page.locator(".app-page").first().waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "project-notes");
      await clickVisible(page, page.getByRole("button", { name: "返回项目" }));
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    // Projects in the sidebar go where you drag them, and stay there.
    if (!narrow) await step("边栏 · 拖动排序", async () => {
      await sidebar(page, true);
      const rows = page.locator(".app-nav .app-nav-item[data-sort-id]:not(.sp-nav-pc)");
      const names = () => rows.evaluateAll((els) => els.map((e) => e.getAttribute("data-sort-id")));
      const before = await names();
      if (before.length < 2) return;
      const dragTo = async (from, to) => {
        const a = await rows.nth(from).boundingBox();
        const c = await rows.nth(to).boundingBox();
        const y0 = a.y + a.height / 2, y1 = c.y + c.height * (to > from ? 0.85 : 0.15);
        await page.mouse.move(a.x + 30, y0);
        await page.mouse.down();
        for (let i = 1; i <= 8; i++) await page.mouse.move(a.x + 30, y0 + (y1 - y0) * i / 8);
        await page.mouse.up();
        await page.waitForTimeout(200);
      };
      await dragTo(0, 1);
      const after = await names();
      if (after[0] !== before[1] || after[1] !== before[0]) throw new Error(`拖动后顺序应变成 ${before[1]}、${before[0]}，实际 ${after.join("、")}`);
      await page.reload({ waitUntil: "domcontentloaded" });
      await page.locator(".app-shell").waitFor();
      await rows.first().waitFor({ timeout: 20_000 });
      const kept = await names();
      if (kept[0] !== before[1]) throw new Error("拖动后的顺序重开后应还在");
      await dragTo(0, 1);
    });

    if (!narrow) await step("边栏 · 拖宽", async () => {
      await sidebar(page, true);
      const side = page.locator(".app-sidebar");
      const width = async () => (await side.boundingBox()).width;
      const w0 = await width();
      const edge = await page.locator(".app-sidebar-edge").boundingBox();
      const x = edge.x + edge.width / 2, y = edge.y + 160;
      await page.mouse.move(x, y);
      await page.mouse.down();
      for (let i = 1; i <= 6; i++) await page.mouse.move(x + 15 * i, y);
      await page.mouse.up();
      await page.waitForTimeout(150);
      const w1 = await width();
      // The edge follows the pointer, whatever the text size.
      if (Math.abs(w1 - w0 - 90) > 3) throw new Error(`边栏拖宽 90px 后应宽 ${Math.round(w0 + 90)}，实际 ${Math.round(w1)}`);
      await page.reload({ waitUntil: "domcontentloaded" });
      await side.waitFor();
      if (Math.abs(await width() - w1) > 2) throw new Error("边栏宽度重开后应还在");
      await page.locator(".app-sidebar-edge").dblclick();
      await page.waitForTimeout(150);
      if (Math.abs(await width() - w0) > 2) throw new Error("双击边缘应恢复默认宽度");
    });

    await step("聊天", async () => {
      await go(page, "聊天", narrow);
      await page.locator(".sp-group").waitFor({ timeout: 20_000 });
      const title = compactText(await page.locator(".sp-head h2").innerText());
      if (title !== "会话") throw new Error(`这里应该是会话（不在项目里的 agent），实际标题「${title}」`);
      // The sidebar: automations one line at the top; projects first, then
      // chat with everyone to talk to listed under it (one click into a
      // chat, in the order picked), computers last.
      await sidebar(page, true);
      const order = await page.evaluate(() => [...document.querySelectorAll(".app-nav > .app-nav-group")].map((g) => g.className));
      if (!order[0]?.includes("sp-nav-autos") || !order[1]?.includes("sp-nav-projects") || !order[2]?.includes("sp-nav-chats") || !order[3]?.includes("sp-nav-pcs")) throw new Error(`边栏应是自动化、项目、会话、电脑，实际 ${order.join(" / ")}`);
      // The three headings look alike: one size and weight, no icon.
      const heads = await page.evaluate(() => [...document.querySelectorAll(".app-nav .sp-nav-head .sp-nav-title-link")].map((e) => {
        const st = getComputedStyle(e);
        return { text: (e.textContent || "").trim(), size: st.fontSize, icon: !!e.querySelector("svg") };
      }));
      if (heads.map((h) => h.text).join("|") !== "项目|会话|电脑") throw new Error(`边栏标题应是 项目、会话、电脑，实际 ${heads.map((h) => h.text).join(" / ")}`);
      // Automations and apps are rows on top, like the rows below, with an icon.
      const tops = await page.evaluate(() => [...document.querySelectorAll(".sp-nav-autos .sp-nav-top")].map((e) => ({ text: (e.textContent || "").trim(), icon: !!e.querySelector("svg"), size: getComputedStyle(e).fontSize })));
      const row = await page.evaluate(() => getComputedStyle(document.querySelector(".sp-nav-projects .app-nav-item")).fontSize);
      if (tops.map((t) => t.text).join("|") !== "自动化|应用" || tops.some((t) => !t.icon || t.size !== row)) throw new Error(`顶上应是带图标的「自动化」「应用」两行，和项目一样大：${JSON.stringify(tops)} ${row}`);
      if (new Set(heads.map((h) => h.size)).size !== 1 || heads.some((h) => h.icon)) throw new Error(`边栏标题字号应一致、不带图标：${JSON.stringify(heads)}`);
      // Projects fold, and stay folded.
      await clickVisible(page, page.locator(".sp-nav-projects .sp-nav-fold"));
      if (await page.locator(".sp-nav-projects .app-nav-item:visible").count()) throw new Error("项目折叠后不该再列出项目");
      await clickVisible(page, page.locator(".sp-nav-projects .sp-nav-fold"));
      await page.locator(".sp-nav-projects .app-nav-item", { hasText: "官网改版" }).waitFor({ timeout: 5_000 });
      if (await page.locator(".app-nav .app-nav-item:visible", { hasText: /^成员$/ }).count()) throw new Error("成员不该再单独占边栏一行");
      const people = page.locator(".sp-nav-chat-list .sp-nav-person:visible");
      if ((await people.count()) < 2) throw new Error("聊天下面应列出能聊的 agent");
      // Ordered by name: the names come in order; the choice is kept.
      await clickVisible(page, page.locator(".sp-nav-chat-sort"));
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "按名字" }));
      const names = (await people.allInnerTexts()).map(compactText);
      const sorted = await page.evaluate((xs) => [...xs].sort((a, b) => a.localeCompare(b)), names);
      if (names.join("|") !== sorted.join("|")) throw new Error(`按名字排序不对：${names.join(" / ")}`);
      await shot(page, size, scheme, "sidebar-chats");
      await clickVisible(page, page.locator(".sp-nav-chat-sort"));
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "最近聊过的在前" }));
      // Pinned: on top whatever the order, with a small pin; unpinned again.
      const last = people.last();
      const lastName = compactText(await last.innerText());
      await menuPick(page, last, "置顶");
      await page.waitForFunction((n) => (document.querySelector(".sp-nav-chat-list .sp-nav-person")?.textContent || "").includes(n), lastName, { timeout: 5_000 });
      await menuPick(page, page.locator(".sp-nav-chat-list .sp-nav-person").first(), "取消置顶");
      // + adds an agent to talk to.
      await clickVisible(page, page.locator(".sp-nav-chat-add"));
      await page.locator(".app-modal", { hasText: "添加 agent" }).waitFor({ timeout: 5_000 });
      await closeDialog(page);
      // One click into a chat with one of them (a coordinator's is its
      // project's home).
      const who = "Release (Claude 云端)";
      await clickVisible(page, people.filter({ hasText: who }));
      await page.locator(".conv-detail .conv-head h2", { hasText: who }).first().waitFor({ timeout: 10_000 });
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-nav-chat"));
      await page.locator(".sp-head h2", { hasText: "会话" }).waitFor({ timeout: 10_000 });
      if (narrow) await sidebar(page, false);
      await settle(page);
      await shot(page, size, scheme, "loose");
      // An agent here can be put in a project from its menu.
      if (!narrow) {
        await page.locator(".sp-side .sp-member:visible").first().click({ button: "right" });
        const menu = page.locator(".conv-menu");
        await menu.waitFor({ timeout: 5_000 });
        if (!(await menu.locator(".conv-menu-header", { hasText: "放进项目" }).count())) throw new Error("聊天里的 agent 右键应能「放进项目」");
        await page.keyboard.press("Escape");
        await menu.waitFor({ state: "hidden", timeout: 5_000 });
      }
    });

    // A task the inbox node pulled but never acknowledged is not work: the
    // task and the node read as queued, and its chat waits without a spinner.
    await step("聊天 · 拉走没开始", async () => {
      await go(page, "聊天", narrow);
      await page.locator(".sp-group").waitFor({ timeout: 20_000 });
      const card = page.locator(".sp-card", { hasText: "把 release 分支的 Go 测试跑一遍" }).first();
      await card.waitFor({ timeout: 10_000 });
      const state = compactText(await card.locator(".sp-card-state").innerText());
      if (state !== "排队中") throw new Error(`只被拉走、没开始的任务应显示「排队中」，实际是「${state}」`);
      if (!narrow) {
        const member = page.locator(".sp-side .sp-member:visible", { hasText: "Myco (Grok)" }).first();
        await member.waitFor({ timeout: 10_000 });
        const sub = compactText(await member.locator(".sp-member-sub").innerText());
        if (sub !== "1 个活在排队") throw new Error(`只被拉走、没开始的节点应显示「1 个活在排队」，实际是「${sub}」`);
        if (await member.locator(".is-working").count()) throw new Error("只被拉走、没开始的节点不该显示在干活");
      }
      await openMember(page, narrow, "聊天", "Myco (Grok)");
      // The header has no running state any more; it must not say it is at it.
      const head = compactText(await page.locator(".conv-detail .conv-head").innerText());
      if (/在干活|处理中/.test(head)) throw new Error(`拉走没开始的对话不该说在干活，标题栏是「${head}」`);
      // Not a line of its own: a few words under your message, like a receipt.
      const receipt = page.locator(".conv-detail .conv-msg.is-me .conv-receipt").first();
      await receipt.waitFor({ timeout: 20_000 });
      const line = compactText(await receipt.innerText());
      if (line !== "还没开始看") throw new Error(`拉走没开始的任务，消息下面应写「还没开始看」，实际是「${line}」`);
      if (!(await receipt.isVisible())) throw new Error("「还没开始看」应一直看得到，不用指上去");
      if (await page.locator(".conv-detail .trace.is-waiting, .conv-detail .trace-spin, .conv-detail .work-sprite").count()) throw new Error("拉走没开始的任务不该有 loading 或单独一行状态");
      await settle(page, 300);
      await shot(page, size, scheme, "pulled-not-started");
    });

    await step("单聊 · 没发的字留着", async () => {
      await openMember(page, narrow, "聊天", "legacy-codex");
      const box = page.locator(".conv-detail .conv-compose-input");
      const h = await box.evaluate((el) => el.getBoundingClientRect().height);
      if (h > 60) throw new Error(`空输入框应只有一行高，实际 ${Math.round(h)}px`);
      await box.fill("还没发出去的一句话");
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      if (await page.locator(".conv-detail .conv-compose-input").inputValue()) throw new Error("别的会话的输入框不该带着这句话");
      await openMember(page, narrow, "聊天", "legacy-codex");
      const back = await page.locator(".conv-detail .conv-compose-input").inputValue();
      if (back !== "还没发出去的一句话") throw new Error(`切回来时没发的字应还在，实际「${back}」`);
      await page.locator(".conv-detail .conv-compose-input").fill("");
    });

    await step("单聊 · 空对话", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      const ask = compactText(await page.locator(".conv-empty h3").innerText({ timeout: 20_000 }));
      if (!ask.includes("Review (Codex 云端)") || ask.includes("还没有消息")) throw new Error(`空对话应该问一句和它有关的话，实际是「${ask}」`);
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-empty");
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // Files: one picked with the paperclip, one pasted; sent alone, the
    // message says what they are and carries both.
    await step("单聊 · 接手后正在启动", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      // Keep the conversation snapshot queued: only the trace poll may
      // learn about ack, even after the 3-second conversation poll.
      const freeze = async (route) => {
        const response = await route.fetch();
        const data = await response.json();
        if (data.messages) for (const m of data.messages) {
          if (m.type === "task" && m.body === "启动预览") m.status = "queued";
        }
        await route.fulfill({ response, json: data });
      };
      await page.route("**/app/hubs/*/exchanges?*", freeze);
      try {
        const box = page.locator(".conv-detail .conv-compose-input");
        await box.fill("启动预览");
        await box.press("Enter");
        const startup = page.locator(".conv-detail .trace.is-live", { hasText: "已接手，正在启动" }).last();
        await startup.waitFor({ timeout: 5000 });
        if (await startup.locator(".work-sprite.is-idle").count()) throw new Error("已接手时蘑菇应该走动");
        await shot(page, size, scheme, "conversation-starting");
      } finally {
        await page.unroute("**/app/hubs/*/exchanges?*", freeze);
      }
      await noRound(page);
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    await step("单聊 · delta 正在长出来", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("逐字预览");
      await box.press("Enter");
      const thinking = page.locator(".conv-detail .trace-group.is-live", { hasText: "正在想…" }).last();
      await thinking.waitFor({ timeout: 6000 });
      if (await thinking.locator(".trace-body").count()) throw new Error("增量思考应默认折在步骤行里");
      const run = page.locator(".conv-detail .trace").last();
      const delta = run.locator(".trace-delta").last();
      await delta.waitFor({ timeout: 8000 });
      // Observe frames inside one poll, then the next suffix on the same
      // DOM node. A complete blob would have just one length throughout.
      const growth = await delta.evaluate(async (el) => {
        const samples = [];
        const until = performance.now() + 2300;
        while (performance.now() < until) {
          samples.push(el.textContent.length);
          await new Promise((r) => requestAnimationFrame(r));
        }
        return { lengths: [...new Set(samples)], connected: el.isConnected };
      });
      if (!growth.connected || growth.lengths.length < 5) throw new Error(`delta 应在原段落持续增长：${JSON.stringify(growth)}`);
      if (growth.lengths.some((n, i, a) => i && n < a[i - 1])) throw new Error("追加片段不应重播已有的字");
      await shot(page, size, scheme, "conversation-delta-growing");
      await delta.waitFor({ state: "hidden", timeout: 5000 });
      const complete = run.locator(".trace-say", { hasText: "不会重新开始，也不会整段闪现" });
      if (await complete.count() !== 1) throw new Error("完整正文应替换 delta，不能重复显示");
      const temporary = page.locator(".conv-detail .trace-delta", { hasText: "这段临时文字" });
      await temporary.waitFor({ timeout: 6000 });
      await temporary.waitFor({ state: "hidden", timeout: 5000 });
      // Opening the existing step line exposes the completed thinking once.
      await run.locator(".trace-group").first().locator("button.trace-head").click();
      if (await run.locator(".trace-row.is-thinking").count() !== 1) throw new Error("完整思考应替换增量思考");
      await noRound(page);
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    await step("单聊 · 补充最近未关闭任务", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const older = await sendFromElsewhere("hub_studio", "review-cloud", "保留任务，等我补充");
      // (The mock answers 启动预览 after 7 seconds, 保留任务 after 20.)
      const newer = await sendFromElsewhere("hub_studio", "review-cloud", "启动预览：先回复这条新任务");
      await page.waitForFunction((id) => !!document.querySelector(`[data-msg="${id}"]`), newer.id, { timeout: 15000 });
      // The newer one is answered; the older one stays open.
      let reply = null;
      for (let i = 0; i < 30 && !reply; i++) {
        const th = await (await fetch(`http://${HOST}:${mockPort()}/app/hubs/hub_studio/threads/${newer.thread_id}`)).json();
        reply = th.messages?.find((m) => m.reply_to === newer.id && m.type === "reply") || null;
        if (!reply) await new Promise((r) => setTimeout(r, 500));
      }
      if (!reply) throw new Error("新任务没有收到回复");
      await page.locator(`[data-msg="${reply.id}"]`).waitFor({ timeout: 10_000 });
      const box = page.locator(".conv-detail .conv-compose-input");
      if (!(await box.getAttribute("placeholder")).includes("补充")) throw new Error("最新任务结束后，仍应补充给旧的未关闭任务");
      const sent = page.waitForRequest((req) => req.method() === "POST" && req.url().includes(`/messages/${older.id}/steer`));
      await box.fill("再检查旧任务的链接");
      await box.press("Enter");
      await sent;
      await page.locator(".trace-steer", { hasText: "再检查旧任务的链接" }).waitFor({ timeout: 10000 });
      await shot(page, size, scheme, "conversation-supplement-older-open");
      await fetch(`http://${HOST}:${mockPort()}/app/hubs/hub_studio/messages/${older.id}/stop`, { method: "POST" });
      await noRound(page);
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // A chat with nothing in it yet: the agent, large, and a question.
    // What was typed and not sent stays with its chat when you go to
    // another and come back; an empty box is one line, whatever its hint.
    await step("单聊 · 发文件", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      const box = page.locator(".conv-compose-box");
      await box.locator("input[type=file]").setInputFiles({ name: "2026 年第四季度发布计划与官网改版需求说明.txt", mimeType: "text/plain", buffer: Buffer.from("hello") });
      await page.locator(".conv-compose-input").evaluate((el) => {
        const dt = new DataTransfer();
        dt.items.add(new File(["png"], "image.png", { type: "image/png" }));
        el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
      });
      await box.locator(".att-chip").nth(1).waitFor({ timeout: 10_000 });
      await page.waitForFunction(() => !document.querySelector(".conv-compose-box .att-spin"), null, { timeout: 10_000 });
      // Tiles of one size: a picture is its thumbnail alone; a file shows
      // its name cut in the middle, the kind kept.
      const tiles = await box.locator(".att-chip").evaluateAll((els) => els.map((e) => {
        const r = e.getBoundingClientRect();
        return { w: r.width, h: r.height, picture: e.classList.contains("is-picture"), name: e.querySelector(".att-name")?.textContent || "" };
      }));
      if (tiles.some((x) => Math.abs(x.w - tiles[0].w) > 1 || Math.abs(x.h - tiles[0].h) > 1)) throw new Error(`文件块应一样大：${JSON.stringify(tiles)}`);
      if (tiles.some((x) => x.picture && x.name)) throw new Error("图片只显示缩略图，不显示文件名");
      const file = tiles.find((x) => !x.picture);
      if (!file || !file.name.endsWith(".txt") || !file.name.includes("…")) throw new Error(`长文件名应从中间省略、保留扩展名，实际「${file?.name}」`);
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-attach");
      // A picture not sent yet opens full size, to see what it was; no
      // Save (it is yours already); Esc closes it and it is still there.
      await box.locator(".att-chip.is-picture .att-thumb-btn").first().click();
      const big = page.locator(".lightbox");
      await big.waitFor({ timeout: 5_000 });
      if (!(await big.locator(".lightbox-img").getAttribute("src"))?.startsWith("blob:") && !(await big.locator(".lightbox-img").getAttribute("src"))?.startsWith("data:")) throw new Error("没发出去的图片应能看大图");
      if (await big.locator(".lightbox-btn", { hasText: "保存" }).count()) throw new Error("没发出去的图片不该有「保存」");
      await page.keyboard.press("Escape");
      await big.waitFor({ state: "detached", timeout: 5_000 });
      if ((await box.locator(".att-chip").count()) !== 2) throw new Error("看完大图，待发的文件应还在");
      // A tall picture (a long screenshot) fits the window, in the middle.
      await page.locator(".conv-compose-input").evaluate(async (el) => {
        const c = document.createElement("canvas");
        c.width = 300; c.height = 3000;
        const g = c.getContext("2d"); g.fillStyle = "#4a8"; g.fillRect(0, 0, 300, 3000);
        const blob = await new Promise((ok) => c.toBlob(ok, "image/png"));
        const dt = new DataTransfer();
        dt.items.add(new File([blob], "长截图.png", { type: "image/png" }));
        el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
      });
      await box.locator(".att-chip").nth(2).waitFor({ timeout: 10_000 });
      await page.waitForFunction(() => !document.querySelector(".conv-compose-box .att-spin"), null, { timeout: 10_000 });
      await box.locator(".att-chip.is-picture .att-thumb-btn").last().click();
      await big.waitFor({ timeout: 5_000 });
      await page.waitForFunction(() => (document.querySelector(".lightbox-img")?.naturalHeight || 0) > 0, null, { timeout: 5_000 });
      const fit = await page.locator(".lightbox-img").evaluate((img) => {
        const r = img.getBoundingClientRect();
        return { top: r.top, bottom: r.bottom, mid: (r.top + r.bottom) / 2, h: window.innerHeight };
      });
      if (fit.top < 0 || fit.bottom > fit.h) throw new Error(`长图应整张在窗口里，实际 ${Math.round(fit.top)}–${Math.round(fit.bottom)} / ${fit.h}`);
      if (Math.abs(fit.mid - (56 + (fit.h - 88) / 2)) > 4) throw new Error(`长图应上下居中，中线在 ${Math.round(fit.mid)}`);
      await page.keyboard.press("Escape");
      await big.waitFor({ state: "detached", timeout: 5_000 });
      await box.locator(".att-chip").nth(2).locator(".att-x").click();
      await page.waitForFunction(() => document.querySelectorAll(".conv-compose-box .att-chip").length === 2, null, { timeout: 5_000 });
      await page.locator(".conv-compose-input").focus();
      await page.locator(".conv-compose-input").press("Enter");
      const sent = page.locator(".conv-msg.is-me", { hasText: "发来 2 个文件" }).last();
      await sent.waitFor({ timeout: 20_000 });
      if ((await sent.locator(".conv-attach").count()) !== 2) throw new Error("发出去的消息应带上两个文件");
      if (await box.locator(".att-chip").count()) throw new Error("发出去后，输入框里的文件应该清空");
      // Its progress (the mock reports one after 3s) is part of the run
      // under the task, not a loose grey line; while it works, the
      // mushroom stays at the very bottom.
      const run = page.locator(".conv-detail .trace").last();
      await run.locator(".trace-reports .trace-report", { hasText: "正在处理" }).waitFor({ timeout: 10_000 });
      // A plan reported as a checklist is a to-do card at the top of the
      // run (ticked off as it goes), not a line of text.
      await run.locator(".trace-plan .trace-plan-item", { hasText: "读一下相关代码" }).waitFor({ timeout: 10_000 });
      if (await run.locator(".trace-reports .trace-report", { hasText: "读一下相关代码" }).count()) throw new Error("清单式的进展应显示成计划卡片，不再是一行文字");
      if (await page.locator(".conv-detail .conv-progress", { hasText: "正在处理" }).count()) throw new Error("进展应写在任务的工作过程里，不是单独一行");
      const wrongOrder = await run.evaluate((el) => {
        const kids = [...el.children];
        const say = kids.findIndex((c) => c.classList.contains("trace-reports"));
        const wait = kids.findIndex((c) => c.classList.contains("trace-wait"));
        return wait >= 0 && wait < say;
      });
      if (wrongOrder) throw new Error("“捣鼓中”应在进展下面");
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // Two messages before either is answered: each answer that does not
    // follow its question quotes it; a run's time and cost sit with its
    // answer, not under the steps.
    await step("单聊 · 晚到的回复引用原话", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      // The file task of the previous step may still be running: Enter would
      // add to it, so wait for it to end.
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("第一件：把发布说明整理一下");
      await box.press("Enter");
      await page.locator(".conv-msg", { hasText: "第一件" }).last().waitFor({ timeout: 20_000 });
      // The second task arrives from "another device" while the first runs:
      // in the box it would now be a supplement, not a second task.
      await sendFromElsewhere("hub_studio", "review-cloud", "第二件：顺便看看下载页的链接");
      await page.locator(".conv-detail .conv-quote-text", { hasText: "第一件" }).waitFor({ timeout: 25_000 });
      if (await page.locator(".conv-detail .trace-foot").count()) throw new Error("耗时和费用应跟着回复（复制按钮那一行），不在过程下面");
      await page.locator(".conv-detail .conv-usage").first().waitFor({ state: "attached", timeout: 10_000 });
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // A picture an agent sends shows as a thumbnail and opens full size; a
    // file that is not one saves to Downloads.
    // Replies that grow in while you watch: their pictures wait for their
    // words, and one still growing finishes at once when a newer message
    // comes below it (several answers arriving together read in order).
    await step("单聊 · 回复长出来的顺序", async () => {
      const replies = (extra) => fetch(`http://${HOST}:${mockPort()}/app/mock/replies`, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ agent: "review-cloud", ...extra }) });
      await replies({ prepare: true });
      await page.reload();
      await page.locator(".app-shell").waitFor({ timeout: 20_000 });
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      await page.waitForTimeout(2500);
      await page.evaluate(() => {
        const w = window;
        w.__revealBad = [];
        w.__revealSeen = 0;
        const check = () => {
          for (const chat of document.querySelectorAll(".conv-detail")) {
            const msgs = [...chat.querySelectorAll(".conv-msg")];
            msgs.forEach((m, i) => {
              if (!m.querySelector(".reveal.is-growing")) return;
              w.__revealSeen++;
              if (m.querySelector(".conv-attach")) w.__revealBad.push("图片先于文字出来");
              if (i < msgs.length - 1) w.__revealBad.push("新消息出来了，旧消息还在长");
            });
          }
        };
        // (a moment after each change: a newer message and the older one
        // finishing land in the same breath)
        let last = "";
        w.__revealTimer = window.setInterval(() => {
          const now = [...document.querySelectorAll(".conv-detail .conv-msg")].map((m) => (m.querySelector(".reveal.is-growing") ? "g" : "-")).join("");
          if (now !== last) { last = now; window.setTimeout(check, 120); }
        }, 40);
      });
      await replies({ gap: 700, tag: `（${size.width} ${scheme}）` });
      await page.waitForTimeout(6_000);
      const r = await page.evaluate(() => { window.clearInterval(window.__revealTimer); return { bad: [...new Set(window.__revealBad)], seen: window.__revealSeen }; });
      if (r.bad.length) throw new Error(`回复长出来的顺序不对：${r.bad.join("；")}`);
      if (!r.seen) throw new Error("两段长回答应该长出来（不是一下子出现）");
    });

    await step("单聊 · 图片附件", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      // The mock's answers take turns, one in four carries a picture: four
      // tasks (as from another device, so none is a supplement to another).
      // The first asks for a screenshot: one comes with a progress update.
      for (let i = 0; i < 4; i++) await sendFromElsewhere("hub_studio", "review-cloud", i === 0 ? "图片附件 1：改完截图给我看" : `图片附件 ${i + 1}`);
      // Several at work at once: one loading in the chat, the newest; the
      // older ones still open stand still.
      for (let t = 0; t < 20; t++) {
        // (a standing mushroom reads as a loading too: the others get a dot)
        const n = await page.locator(".conv-detail .work-sprite").count();
        if (n > 1) throw new Error(`聊天里同时只该有一个 loading（蘑菇），现在有 ${n} 个`);
        await page.waitForTimeout(150);
      }
      const thumb = page.locator(".conv-detail .conv-attach.is-image").last();
      await thumb.waitFor({ timeout: 25_000 });
      const w = await thumb.locator("img").evaluate((img) => new Promise((ok) => (img.complete ? ok(img.naturalWidth) : img.addEventListener("load", () => ok(img.naturalWidth), { once: true }))));
      if (!w) throw new Error("图片附件应显示缩略图");
      // At its own shape: not cropped to a square, not stretched.
      const ratio = await thumb.locator("img").evaluate((img) => {
        const r = img.getBoundingClientRect();
        return { shown: r.width / r.height, own: img.naturalWidth / img.naturalHeight };
      });
      if (Math.abs(ratio.shown / ratio.own - 1) > 0.05) throw new Error(`缩略图应按原图比例，实际 ${ratio.shown.toFixed(2)}，原图 ${ratio.own.toFixed(2)}`);
      await thumb.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "message-picture");
      await thumb.click();
      await page.locator(".lightbox .lightbox-img").waitFor({ timeout: 5_000 });
      // In the Mac app its name keeps clear of the traffic lights (to x≈80).
      if (await page.evaluate(() => document.documentElement.classList.contains("shell-mac"))) {
        const x = await page.locator(".lightbox-bar").evaluate((el) => el.getBoundingClientRect().left);
        if (x < 80) throw new Error(`看大图时左上角的名字会压到红绿灯（从 ${Math.round(x)}px 开始）`);
      }
      await shot(page, size, scheme, "message-picture-open");
      await page.keyboard.press("Escape");
      await page.locator(".lightbox").waitFor({ state: "hidden", timeout: 5_000 });
      // A Markdown file opens to read in the App, rendered; Save is there.
      await page.locator(".conv-detail button.conv-attach:not(.is-image)", { hasText: "发布说明.md" }).last().click();
      // It opens as a panel from the right at its full height: it does
      // not grow (or jump) when the file arrives.
      const doc = page.locator(".fp-panel");
      await doc.waitFor({ timeout: 5_000 });
      const h0 = await doc.evaluate((el) => el.getBoundingClientRect().height);
      await doc.locator(".fp-body.is-doc .md h1").waitFor({ timeout: 5_000 });
      await settle(page, 200);
      const h1 = await doc.evaluate((el) => el.getBoundingClientRect().height);
      if (Math.abs(h1 - h0) > 1) throw new Error(`文件面板打开后不该变高（${h0} → ${h1}）`);
      await shot(page, size, scheme, "file-preview");
      // Wide enough, it sits beside the chat (the App makes room): you can
      // read it and go on writing; working in the chat does not close it.
      if (!narrow) {
        const gap = await page.evaluate(() => document.querySelector(".fp-panel").getBoundingClientRect().left - document.querySelector(".app-main").getBoundingClientRect().right);
        if (gap < 0) throw new Error(`文件面板应在对话旁边，不该盖住对话（重叠 ${Math.round(-gap)}px）`);
        if (await page.locator(".fp-scrim").count()) throw new Error("文件在对话旁边时不该有遮罩");
        const chatBox = page.locator(".conv-detail .conv-compose-input");
        await chatBox.click();
        await chatBox.fill("边看文件边聊");
        await page.waitForTimeout(200);
        if (!(await doc.isVisible())) throw new Error("一边看文件一边在聊天框里打字，文件面板不该关掉");
        await chatBox.fill("");
        await doc.locator(".fp-head").click();
      }
      // Full screen, and back (a narrow window has it all already: no
      // button there).
      const fullBtn = doc.getByRole("button", { name: "全屏" });
      if (narrow) {
        if (await fullBtn.isVisible()) throw new Error("窄窗口里文件面板不该有全屏按钮");
      } else {
        const w0 = await doc.evaluate((el) => el.getBoundingClientRect().width);
        await fullBtn.click();
        await page.waitForFunction((w) => {
          const el = document.querySelector(".fp-panel.is-full");
          return !!el && el.getBoundingClientRect().width > w + 1 && el.getBoundingClientRect().width >= window.innerWidth - 40;
        }, w0, { timeout: 5_000 });
        await settle(page, 250);
        await shot(page, size, scheme, "file-preview-full");
        await doc.getByRole("button", { name: "退出全屏" }).click();
        await page.locator(".fp-panel:not(.is-full)").waitFor({ timeout: 5_000 });
        await settle(page, 250);
        // Its left edge drags it wider; a double-click puts it back.
        const before = await doc.evaluate((el) => el.getBoundingClientRect().width);
        const e = await doc.locator(".fp-edge").boundingBox();
        await page.mouse.move(e.x + e.width / 2, e.y + e.height / 2);
        await page.mouse.down();
        await page.mouse.move(e.x - 150, e.y + e.height / 2, { steps: 6 });
        await page.mouse.up();
        const wider = await doc.evaluate((el) => el.getBoundingClientRect().width);
        if (Math.abs(wider - before - 150) > 12) throw new Error(`拖左边应变宽 150px 左右，实际 ${Math.round(before)} → ${Math.round(wider)}`);
        await doc.locator(".fp-edge").dblclick();
        await page.waitForFunction((w) => Math.abs(document.querySelector(".fp-panel").getBoundingClientRect().width - w) < 2, before, { timeout: 5_000 });
      }
      await doc.getByRole("button", { name: "存到「下载」" }).click();
      await doc.getByRole("button", { name: "已存到「下载」" }).waitFor({ timeout: 5_000 });
      await doc.getByRole("button", { name: "关闭" }).click();
      await doc.waitFor({ state: "hidden", timeout: 5_000 });
      // An HTML file shows as a page, in a frame that runs nothing; its
      // source is a click away.
      await page.locator(".conv-detail button.conv-attach:not(.is-image)", { hasText: "首页.html" }).last().click();
      const frame = doc.locator(".fp-body.is-page iframe.fp-frame");
      await frame.waitFor({ timeout: 5_000 });
      // (same origin only so its links can be opened from the App; no scripts)
      const sandbox = (await frame.getAttribute("sandbox")) ?? "missing";
      if (sandbox === "missing" || /allow-scripts|allow-popups|allow-top-navigation/.test(sandbox)) throw new Error(`HTML 预览的框不该允许运行脚本，实际 sandbox="${sandbox}"`);
      if (!((await frame.getAttribute("srcdoc")) || "").includes("新版首页")) throw new Error("HTML 预览应显示这个页面");
      await doc.getByRole("tab", { name: "源码" }).click();
      await doc.locator(".fp-code", { hasText: "<h1>新版首页</h1>" }).waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await doc.waitFor({ state: "hidden", timeout: 5_000 });
      // A log reads as plain text.
      await page.locator(".conv-detail button.conv-attach:not(.is-image)", { hasText: "build.log" }).last().click();
      await doc.locator(".fp-code", { hasText: "BUILD SUCCESSFUL" }).waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await doc.waitFor({ state: "hidden", timeout: 5_000 });
      // Every one of them answered before the next step uses this chat.
      await page.waitForFunction(() => {
        const d = document.querySelector(".conv-detail");
        return !!d && d.querySelectorAll(".trace.is-live, .trace.is-waiting").length === 0;
      }, null, { timeout: 45_000 });
      // A screenshot sent with a progress update shows under that update.
      const along = page.locator(".conv-detail .trace-report-files .conv-attach.is-image img").last();
      await along.waitFor({ timeout: 5_000 });
      if (!await along.evaluate((img) => img.complete && img.naturalWidth > 0)) throw new Error("进展里的截图应显示缩略图");
      await noRound(page);
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // What was sent in a chat, in one place: 图片和文件 in the header's ⋯
    // opens a panel with the pictures as a grid and the files as rows, each
    // a way back to its message. The header shows only ⋯ and find (find
    // last, next to a pane's own buttons).
    await step("单聊 · 图片和文件", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      const head = page.locator(".conv-detail .conv-head");
      const icons = await head.locator(".conv-head-actions > .conv-head-find:visible").evaluateAll((els) => els.map((e) => e.getAttribute("aria-label")));
      if (icons.join("|") !== "更多|在这段对话里找") throw new Error(`单聊顶栏右边应是「更多」「找」两个，实际 ${icons.join("、")}`);
      await clickVisible(page, head.locator("[data-testid=chat-more]"));
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "图片和文件" }));
      const panel = page.locator(".cf-panel");
      await panel.waitFor({ timeout: 5_000 });
      const pics = panel.locator("[data-testid=chat-files-pictures] .cf-shot");
      await pics.first().waitFor({ timeout: 5_000 });
      if ((await pics.count()) < 2) throw new Error(`图片和文件里应列出这段对话发过的图片，实际 ${await pics.count()} 张`);
      if (!(await panel.locator("[data-testid=chat-files-files] .cf-file", { hasText: "发布说明.md" }).count())) throw new Error("图片和文件里应列出发过的文件（发布说明.md）");
      await settle(page, 300);
      await shot(page, size, scheme, "chat-files");
      await pics.first().click();
      await page.locator(".lightbox .lightbox-img").waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await page.locator(".lightbox").waitFor({ state: "hidden", timeout: 5_000 });
      await clickVisible(page, panel.getByRole("tab", { name: /^文件/ }));
      if (await pics.count()) throw new Error("只看文件时不该有图片");
      await clickVisible(page, panel.locator(".cf-locate").first());
      if (await panel.isVisible()) await panel.getByRole("button", { name: "关闭" }).click();
      await panel.waitFor({ state: "hidden", timeout: 5_000 });
    });

    // Speaking instead of typing: the microphone beside Send listens in the
    // app's language (the mock is the Mac app), the words grow into the box
    // after what was typed, and it stops by itself at the end.
    await step("单聊 · 语音输入", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      const mic = page.locator(".conv-detail .conv-compose-bar > .dictate-btn");
      await mic.waitFor({ timeout: 10_000 });
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("先说一句：");
      await mic.click();
      await page.locator(".conv-detail .dictate-btn.is-listening").waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "dictation");
      await page.waitForFunction(() => (document.querySelector(".conv-detail .conv-compose-input")?.value || "") === "先说一句：帮我看一下下载页", null, { timeout: 10_000 });
      await page.locator(".conv-detail .dictate-btn.is-listening").waitFor({ state: "detached", timeout: 5_000 });
      const st = await (await fetch(`http://${HOST}:${mockPort()}/app/dictation`)).json();
      if (st.lang !== "zh-CN") throw new Error(`应按 App 的语言听（zh-CN），实际 ${st.lang}`);
      if (await mic.evaluate((el) => el.classList.contains("is-note"))) throw new Error(`语音输入不该报错：${await mic.getAttribute("title")}`);
      // Typing while it listens takes over: it stops and writes nothing more.
      await box.fill("");
      await mic.click();
      await page.locator(".conv-detail .dictate-btn.is-listening").waitFor({ timeout: 5_000 });
      await box.fill("我自己打");
      await page.locator(".conv-detail .dictate-btn.is-listening").waitFor({ state: "detached", timeout: 5_000 });
      await page.waitForTimeout(1500);
      if ((await box.inputValue()) !== "我自己打") throw new Error(`自己打字后语音不该再写进来，实际「${await box.inputValue()}」`);
      // The round button: with nothing typed it starts a voice call, with
      // words it sends; no hint under the box unless a round is running.
      await noRound(page);
      await box.fill("");
      await page.locator(".conv-detail .conv-send.is-call").waitFor({ timeout: 5_000 });
      await settle(page, 150);
      await shot(page, size, scheme, "compose-empty");
      await box.fill("有字了");
      await page.locator(".conv-detail .conv-send:not(.is-call)").waitFor({ timeout: 5_000 });
      if ((await page.locator(".conv-detail .conv-compose-hint").innerText()).trim()) throw new Error("没在干活时输入框下面不该有提示文字");
      // Hold Ctrl+M (⌥Space on a Mac) to talk; letting go stops.
      await box.fill("");
      await box.focus();
      await page.keyboard.down("Control");
      await page.keyboard.down("KeyM");
      await page.locator(".conv-detail .dictate-btn.is-listening").waitFor({ timeout: 5_000 });
      await page.keyboard.up("KeyM");
      await page.keyboard.up("Control");
      await page.locator(".conv-detail .dictate-btn.is-listening").waitFor({ state: "detached", timeout: 8_000 });
      await box.fill("");
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // An agent asks you to decide: a card with the choices, the round waits
    // (no loading), a click answers it and the round goes on.
    // An agent that hands the work out and ends its turn (turn_end) is
    // waiting, not at work: no loading while the task stays open.
    await step("单聊 · 派出去等结果", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("核对下载页，派出去给别人做");
      await box.press("Enter");
      // At work first (the mushroom moves); once its turn is over, nothing:
      // what it said last stands, as in a chat (no "在等" line, no loading).
      await page.locator(".conv-detail .work-sprite:not(.is-idle)").first().waitFor({ timeout: 15_000 });
      await page.locator(".conv-detail .trace-say", { hasText: "已经派出去了" }).last().waitFor({ timeout: 15_000 });
      await page.waitForFunction(() => !document.querySelector(".conv-detail .work-sprite"), null, { timeout: 15_000 });
      await settle(page, 300);
      const lines = await page.locator(".conv-detail .trace").last().innerText();
      if (/在等|还没做完|先放着/.test(lines)) throw new Error(`它这一轮停了，不该再挂一行状态，实际有「${compactText(lines)}」`);
      if (await page.locator(".conv-detail .conv-msg.is-me .conv-receipt").count()) throw new Error("开始做之后，消息下面不该还有「还没开始看」");
      // It stopped: the box is a plain one again (a new message, nothing to stop).
      const ph = await page.locator(".conv-detail .conv-compose-input").getAttribute("placeholder");
      if ((ph || "").startsWith("补充给")) throw new Error(`它这一轮停了，输入框不该还是「补充给…」，实际「${ph}」`);
      if (await page.locator(".conv-detail .conv-send.is-stop").count()) throw new Error("它这一轮停了，不该还有停止按钮");
      await shot(page, size, scheme, "chat-waiting");
      // Meanwhile its session woke only to keep listening (its wait timed
      // out): no loading again, none of that turn shown.
      for (let t = 0; t < 30; t++) {
        if (await page.locator(".conv-detail .work-sprite").count()) throw new Error("它醒来只是重新等消息，不该又冒出 loading");
        if (await page.locator(".conv-detail .conv-msg", { hasText: "下载页核对完了" }).count()) break;
        await page.waitForTimeout(400);
      }
      if (await page.locator(".conv-detail .trace", { hasText: "还在等消息" }).count()) throw new Error("只为继续等消息醒来的那一轮不该显示在过程里");
      // The answer comes when the other one is done.
      await page.locator(".conv-detail .conv-msg", { hasText: "下载页核对完了" }).waitFor({ timeout: 20_000 });
      await noRound(page);
    });

    // What you add to a task still running goes into its run at the time
    // you sent it: what the agent does after is under it, its state last.
    await step("单聊 · 补充接在过程里", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      // (sent from the box: a chat with nothing in it yet starts from it)
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("补充接在过程里：派出去核对链接");
      await box.press("Enter");
      const aMsg = page.locator(".conv-detail .conv-msg.is-me", { hasText: "补充接在过程里：派出去核对链接" });
      await aMsg.waitFor({ timeout: 10_000 });
      // While it is at it (it has run its command, not yet said it is done):
      // what you add goes into the run.
      await page.locator(".conv-detail .trace-group").last().waitFor({ timeout: 15_000 });
      await box.fill("补充：顺便看看 Windows 的包");
      await box.press("Enter");
      const added = page.locator(".conv-detail .trace .trace-steer", { hasText: "顺便看看 Windows 的包" });
      await added.waitFor({ timeout: 10_000 });
      if (await page.locator(".conv-detail .conv-supplement", { hasText: "顺便看看 Windows 的包" }).count()) throw new Error("补充应接在过程里，不该再画进任务气泡");
      // Nothing of the run (its words, its state) is drawn above what you added after it.
      const order = await added.evaluate((el) => {
        const run = el.closest(".trace");
        const said = [...(run?.querySelectorAll(".trace-group") || [])].slice(0, 1);
        const state = run?.querySelector(".trace-wait");
        const y = el.getBoundingClientRect().top;
        return { saidAbove: said.every((x) => x.getBoundingClientRect().top < y), stateBelow: !state || state.getBoundingClientRect().top > y };
      });
      if (!order.saidAbove) throw new Error("补充之前它做的步骤应在补充上面");
      if (!order.stateBelow) throw new Error("它现在的状态（在做 / 在等）应在补充下面");
      await added.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "chat-supplement-in-run");
      await page.locator(".conv-detail .conv-msg", { hasText: "下载页核对完了" }).last().waitFor({ timeout: 25_000 });
      await noRound(page);
    });

    // Finding a message (⌘F), and back to the bottom after reading above.
    await step("单聊 · 搜索和回到底部", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("搜索用的一句：蓝莓派");
      await box.press("Enter");
      await page.locator(".conv-detail .conv-msg", { hasText: "蓝莓派" }).waitFor({ timeout: 10_000 });
      // (⌘F goes to the chat last clicked in)
      await page.locator(".conv-detail .conv-msg", { hasText: "蓝莓派" }).first().click();
      await page.keyboard.press("Control+f");
      const bar = page.locator(".conv-detail .conv-search");
      await bar.waitFor({ timeout: 5_000 });
      await bar.locator("input").fill("蓝莓派");
      await bar.locator(".conv-search-count", { hasText: /^\d+\/\d+$/ }).waitFor({ timeout: 5_000 });
      await page.locator(".conv-detail .conv-msg.is-flash", { hasText: "蓝莓派" }).first().waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "chat-search");
      await bar.locator("input").fill("这句话不会有");
      await bar.locator(".conv-search-count.is-none").waitFor({ timeout: 5_000 });
      await bar.locator("input").press("Escape");
      await bar.waitFor({ state: "detached", timeout: 5_000 });
      await noRound(page);
      const scroller = page.locator(".conv-detail .conv-scroll");
      if (await scroller.evaluate((el) => el.scrollHeight - el.clientHeight > 600)) {
        // (by wheel, as you would: a scroll from code alone does not leave the bottom)
        const sb = await scroller.boundingBox();
        await page.mouse.move(sb.x + sb.width / 2, sb.y + sb.height / 2);
        for (let k = 0; k < 6; k++) { await page.mouse.wheel(0, -400); await page.waitForTimeout(40); }
        const btn = page.locator(".conv-detail .conv-to-bottom");
        await btn.waitFor({ timeout: 5_000 });
        await shot(page, size, scheme, "chat-to-bottom");
        await btn.click();
        await page.waitForFunction(() => {
          const el = document.querySelector(".conv-detail .conv-scroll");
          return !!el && el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        }, null, { timeout: 5_000 });
        await btn.waitFor({ state: "detached", timeout: 5_000 });
      }
    });

    // 删除 takes a message out of your own view (kept on the workspace):
    // 撤销 brings it back; gone again, it stays gone after a reload.
    await step("单聊 · 删除消息", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("删除用的一句：不要了");
      await box.press("Enter");
      const mine = page.locator(".conv-detail .conv-msg.is-me", { hasText: "删除用的一句" }).last();
      await mine.waitFor({ timeout: 10_000 });
      await noRound(page);
      // (clicked on the button itself, not at a spot on screen: the chat may
      // still move as the answer comes, and the button only shows on hover)
      await mine.locator(".conv-msg-del").evaluate((b) => b.click());
      await page.locator(".conv-detail .conv-undo").waitFor({ timeout: 5_000 });
      if (await page.locator(".conv-detail .conv-msg.is-me", { hasText: "删除用的一句" }).count()) throw new Error("删除后这条应从对话里消失");
      await settle(page, 200);
      await shot(page, size, scheme, "chat-deleted");
      await page.locator(".conv-detail .conv-undo button", { hasText: "撤销" }).click();
      await mine.waitFor({ timeout: 5_000 });
      // (clicked on the button itself, not at a spot on screen: the chat may
      // still move as the answer comes, and the button only shows on hover)
      await mine.locator(".conv-msg-del").evaluate((b) => b.click());
      await page.locator(".conv-detail .conv-msg.is-me", { hasText: "删除用的一句" }).waitFor({ state: "detached", timeout: 5_000 });
      await page.reload({ waitUntil: "domcontentloaded" });
      await page.locator(".app-shell").waitFor();
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      await page.waitForTimeout(800);
      if (await page.locator(".conv-detail .conv-msg.is-me", { hasText: "删除用的一句" }).count()) throw new Error("删掉的消息刷新后不该回来");
    });

    // Opening a chat: at the bottom, or where you left it higher up (not
    // somewhere in between as what is above loads).
    await step("单聊 · 打开时的位置", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const below = () => page.evaluate(() => { const el = document.querySelector(".conv-detail .conv-scroll"); return el ? Math.round(el.scrollHeight - el.scrollTop - el.clientHeight) : -1; });
      await page.waitForTimeout(1500);
      if ((await below()) > 4) throw new Error(`打开对话应停在最底下，实际离底部 ${await below()}px`);
      if (await page.locator(".conv-detail .conv-scroll").evaluate((el) => el.scrollHeight - el.clientHeight < 800)) return; // too short to read higher up
      const r = await page.locator(".conv-detail .conv-scroll").boundingBox();
      await page.mouse.move(r.x + r.width / 2, r.y + r.height / 2);
      for (let k = 0; k < 4; k++) { await page.mouse.wheel(0, -200); await page.waitForTimeout(60); }
      await page.waitForTimeout(400);
      const left = await below();
      await openMember(page, narrow, "聊天", "legacy-codex");
      await page.waitForTimeout(500);
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await page.waitForTimeout(1600);
      const back = await below();
      if (Math.abs(back - left) > 80) throw new Error(`回到对话应停在上次看的位置（离底部 ${left}px），实际 ${back}px`);
      await page.locator(".conv-detail .conv-to-bottom").click();
      await page.waitForFunction(() => { const el = document.querySelector(".conv-detail .conv-scroll"); return !!el && el.scrollHeight - el.scrollTop - el.clientHeight < 5; }, null, { timeout: 5_000 });
    });

    // ⌘K: find a member and open its chat; find words in your messages and
    // open the chat there, with the words marked.
    await step("全局搜索", async () => {
      await page.keyboard.press("Control+k");
      const cp = page.locator(".cp");
      await cp.waitFor({ timeout: 5_000 });
      await cp.locator("input").fill("Review");
      await cp.locator(".cp-item", { hasText: "Review (Codex 云端)" }).first().waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "search-anything");
      await cp.locator("input").press("Enter");
      await cp.waitFor({ state: "detached", timeout: 5_000 });
      await page.locator(".conv-detail .conv-head h2", { hasText: "Review" }).waitFor({ timeout: 10_000 });
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("全局搜索用：芒果布丁");
      await box.press("Enter");
      await page.locator(".conv-detail .conv-msg", { hasText: "芒果布丁" }).waitFor({ timeout: 10_000 });
      await noRound(page);
      if (narrow) await sidebar(page, false);
      await page.keyboard.press("Control+k");
      await cp.waitFor({ timeout: 5_000 });
      await cp.locator("input").fill("芒果布丁");
      const msg = cp.locator(".cp-item", { hasText: "芒果布丁" }).first();
      await msg.waitFor({ timeout: 8_000 });
      await msg.click();
      await page.locator(".conv-detail .conv-search input").first().waitFor({ timeout: 10_000 });
      const v = await page.locator(".conv-detail .conv-search input").first().inputValue();
      if (v !== "芒果布丁") throw new Error(`从搜索打开的对话应标出搜的词，实际「${v}」`);
      await page.locator(".conv-detail .conv-search input").first().press("Escape");
      // Empty, it lists the chats opened last first: back to one in a keystroke.
      await page.keyboard.press("Control+k");
      await cp.waitFor({ timeout: 5_000 });
      const firstGroup = compactText(await cp.locator(".cp-group").first().innerText());
      if (firstGroup !== "最近") throw new Error(`⌘K 空着时第一组应是「最近」，实际「${firstGroup}」`);
      const firstItem = compactText(await cp.locator(".cp-item").first().innerText());
      if (!firstItem.includes("Review")) throw new Error(`最近聊的应排第一，实际「${firstItem}」`);
      await page.keyboard.press("Escape");
      await cp.waitFor({ state: "detached", timeout: 5_000 });
    });

    // Set aside as the skill says (hold, then a progress saying what it
    // waits for), still in its session: waiting, not at work — no loading,
    // 稍后回复 under your message, a plain box.
    await step("单聊 · 挂起后说明在等", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill(`挂起说明：定时和 Webhook 先做第一步（${size.width}-${scheme}）`);
      await box.press("Enter");
      await page.locator(".conv-detail .trace-report", { hasText: "已派给 release" }).last().waitFor({ timeout: 15_000 });
      await page.locator(".conv-detail .conv-receipt", { hasText: "稍后回复" }).last().waitFor({ timeout: 10_000 });
      await page.waitForTimeout(2500);
      if (await page.locator(".conv-detail .work-sprite").count()) throw new Error("挂起后只是说明在等什么，不该一直显示 loading");
      const ph = await box.getAttribute("placeholder");
      if ((ph || "").startsWith("补充给")) throw new Error(`挂起的任务不该让输入框变成「补充给…」，实际「${ph}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "chat-held-waiting");
      await page.locator(".conv-detail .conv-msg", { hasText: "release 那边做完了" }).last().waitFor({ timeout: 15_000 });
      await noRound(page);
    });

    // An agent in one long session goes on with what you sent next, its
    // steps and progress still filed under the first task (still open):
    // they show under your newer message, in time, not up above.
    await step("单聊 · 长会话按时间显示", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const tag = `${size.width}-${scheme}`;
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill(`长会话：先查授权（${tag}）`);
      await box.press("Enter");
      await page.locator(".conv-detail .trace-say", { hasText: "需要你在会话里允许授权" }).last().waitFor({ timeout: 15_000 });
      await page.locator(".conv-detail .conv-receipt", { hasText: "稍后回复" }).last().waitFor({ timeout: 15_000 });
      await box.fill(`长会话：接着拉群消息（${tag}）`);
      await box.press("Enter");
      const second = page.locator(".conv-detail .conv-msg.is-me", { hasText: `长会话：接着拉群消息（${tag}）` }).last();
      await second.waitFor({ timeout: 10_000 });
      const said = page.locator(".conv-detail .trace-say", { hasText: "授权已生效，开始拉群消息" }).last();
      await said.waitFor({ timeout: 15_000 });
      const report = page.locator(".conv-detail .trace-report", { hasText: "授权已生效，开始只读收集" }).last();
      await report.waitFor({ timeout: 15_000 });
      const below = await second.evaluate((el, ts) => {
        const y = el.getBoundingClientRect().bottom;
        return ts.map((t) => {
          const hit = [...document.querySelectorAll(".conv-detail .trace-say, .conv-detail .trace-report")].filter((x) => (x.textContent || "").includes(t)).pop();
          return !!hit && hit.getBoundingClientRect().top >= y - 1;
        });
      }, ["授权已生效，开始拉群消息", "授权已生效，开始只读收集"]);
      if (below.some((b) => !b)) throw new Error("它接着做的步骤和进展应在你后发的那条消息下面，不该跑到上面去");
      if (await page.locator(".conv-detail .trace-say", { hasText: "授权已生效，开始拉群消息" }).count() !== 1) throw new Error("接着做的步骤应只出现一次");
      await settle(page, 300);
      await shot(page, size, scheme, "chat-long-session");
      // Both are answered: nothing left open for the steps after this one.
      await page.locator(".conv-detail .conv-msg", { hasText: "授权这件事好了" }).last().waitFor({ timeout: 15_000 });
      await noRound(page);
    });

    // A long answer starts folded; unfolded, it stays unfolded while it
    // settles (it grows in, then is drawn as a plain reply) and you scroll
    // down through it.
    await step("单聊 · 展开的长消息不会自己收起", async () => {
      const tag = `（${size.width}-${scheme}）`;
      const mock = (data) => fetch(`http://${HOST}:${mockPort()}/app/mock/replies`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data) });
      await mock({ agent: "review-cloud", prepare: true });
      await page.reload();
      await page.locator(".app-shell").waitFor({ timeout: 20_000 });
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      await page.waitForTimeout(1500);
      await mock({ agent: "review-cloud", lines: 45, tag });
      const long = page.locator(".conv-detail .conv-msg", { hasText: `长回复第 1 行${tag}` }).last();
      const unfold = long.locator(".md-fold-btn", { hasText: "展开" });
      await unfold.waitFor({ timeout: 15_000 });
      await unfold.click();
      await long.locator(".md-fold-btn", { hasText: "收起" }).waitFor({ timeout: 5_000 });
      // It settles; you scroll down through it.
      await settle(page, 2500);
      await long.evaluate((el) => el.closest(".conv-scroll")?.scrollBy(0, 600));
      await settle(page, 800);
      if (await long.locator(".md-fold-body.is-collapsed").count()) throw new Error("展开的长回复不该自己收起");
      await noRound(page);
    });

    // Links one after another with Chinese commas are each a link of their own.
    await step("单聊 · 中文标点隔开的链接", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("三个链接：https://a.example/x，https://b.example/y，https://c.example/z 看看");
      await box.press("Enter");
      const bubble = page.locator(".conv-detail .conv-msg.is-me", { hasText: "三个链接" }).last();
      await bubble.waitFor({ timeout: 10_000 });
      const hrefs = await bubble.locator("a").evaluateAll((els) => els.map((a) => a.getAttribute("href")));
      if (hrefs.join(" ") !== "https://a.example/x https://b.example/y https://c.example/z") throw new Error(`中文逗号隔开的应是三个链接，实际 ${JSON.stringify(hrefs)}`);
      await noRound(page);
    });

    // 简单模式 (Settings › 外观): one person on this computer. Workspaces,
    // people and other computers hide; turning it off brings them back.
    await step("设置 · 简单模式", async () => {
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "外观" }));
      const sw = page.locator('[data-setting="simple-mode"] [role=switch]');
      await sw.waitFor({ timeout: 10_000 });
      if (await sw.getAttribute("aria-checked") !== "false") throw new Error("已有工作区的用户默认不是简单模式");
      await sw.click();
      await page.locator('[data-setting="simple-mode"] [role=switch][aria-checked="true"]').waitFor({ timeout: 5_000 });
      if (await page.locator(".app-settings-tab", { hasText: "工作区" }).count()) throw new Error("简单模式下设置里不该有「工作区」");
      if (await page.locator(".app-settings-tab", { hasText: "成员" }).count()) throw new Error("简单模式下设置里不该有「成员」");
      if (!narrow && await page.locator(".sp-nav-pcs").count()) throw new Error("简单模式下侧栏不该有「电脑」");
      await settle(page, 200);
      await shot(page, size, scheme, "settings-simple-mode");
      await sw.click();
      await page.locator(".app-settings-tab", { hasText: "工作区" }).waitFor({ timeout: 5_000 });
      if (!narrow) await page.locator(".sp-nav-pcs").waitFor({ timeout: 5_000 });
      if (narrow) await sidebar(page, false);
    });

    // Settings › Agents: the agent CLIs on this computer, installed or not,
    // signed in or not; a button opens a terminal for what is missing, or
    // shows the command when no terminal could be opened.
    await step("设置 · 本机 agent", async () => {
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "本机 agent" }));
      const lines = page.locator(".la-line");
      await lines.first().waitFor({ timeout: 10_000 });
      if (await lines.count() !== 4) throw new Error(`应列出 4 个 agent，实际 ${await lines.count()}`);
      const codex = page.locator('.la-line[data-agent-cli="codex"]');
      await codex.locator(".app-pill", { hasText: "未登录" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "settings-agents");
      await codex.getByRole("button", { name: "登录" }).click();
      await codex.locator(".la-opened", { hasText: "已经在终端里打开了" }).waitFor({ timeout: 5_000 });
      await page.locator(".la-foot").getByRole("button", { name: "重新检查" }).click();
      await page.locator('.la-line[data-agent-cli="codex"][data-agent-state="ready"]').waitFor({ timeout: 5_000 });
      // No terminal here: the command to copy instead.
      const oc = page.locator('.la-line[data-agent-cli="opencode"]');
      await oc.getByRole("button", { name: "安装" }).click();
      await clickVisible(page, page.locator(".app-ctx-item, [role=menuitem]", { hasText: "用官方方式安装" }).first());
      await oc.locator(".app-command code", { hasText: "opencode.ai/install" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "settings-agents-command");
      if (narrow) await sidebar(page, false);
    });

    // A proxy for the App: from the environment (as before), none, or one
    // set here; tried before it is saved, in force once saved.
    await step("设置 · 代理", async () => {
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "代理" }));
      const card = page.locator(".app-setting-card", { hasText: "代理" });
      await card.getByRole("tab", { name: "跟系统环境", selected: true }).waitFor({ timeout: 10_000 });
      await card.getByRole("tab", { name: "手动设置" }).click();
      await card.getByRole("tab", { name: "SOCKS5" }).click();
      await card.locator("label", { hasText: "地址" }).locator("input").fill("127.0.0.1");
      await card.locator("label", { hasText: "端口" }).locator("input").fill("9");
      await card.getByRole("button", { name: "试一下" }).click();
      await card.locator(".tr-tried.is-error", { hasText: "代理拒绝了连接" }).waitFor({ timeout: 5_000 });
      await card.locator("label", { hasText: "端口" }).locator("input").fill("1080");
      await card.getByRole("button", { name: "试一下" }).click();
      await card.locator(".tr-tried", { hasText: "连通了" }).waitFor({ timeout: 5_000 });
      await card.getByRole("button", { name: "保存" }).click();
      await card.locator(".tr-tried", { hasText: "已保存" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "settings-proxy");
      // Back to the environment's (the other windows share the mock).
      await card.getByRole("tab", { name: "跟系统环境" }).click();
      await card.getByRole("button", { name: "保存" }).click();
      await card.locator(".tr-tried", { hasText: "已保存" }).waitFor({ timeout: 5_000 });
      if (narrow) await sidebar(page, false);
    });

    // Translating what an agent wrote: nothing until a service has a key
    // (Settings › 翻译); then by hand from its time, the original a click away.
    await step("翻译", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const reply = page.locator(".conv-detail .conv-msg.is-agent").last();
      await reply.waitFor({ timeout: 10_000 });
      await reply.hover();
      if (await reply.locator(".conv-translate-btn").count()) throw new Error("还没填翻译服务的密钥时，消息旁不该有「翻译」");
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "翻译" }));
      if (await page.getByRole("tab", { name: /MyMemory/ }).count()) throw new Error("不该再有 MyMemory");
      await page.getByRole("tab", { name: "DeepL" }).click();
      await page.locator(".tr-fields input[type=password]").fill("demo-key:fx");
      await page.getByRole("button", { name: "保存" }).click();
      await page.locator('.tr-fields input[placeholder*="钥匙串"]').waitFor({ timeout: 5_000 });
      await page.getByRole("button", { name: "试一下" }).click();
      await page.locator(".tr-tried", { hasText: "〔译〕" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "settings-translate");
      if (narrow) await sidebar(page, false);
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const again = page.locator(".conv-detail .conv-msg.is-agent").last();
      await again.hover();
      await again.locator(".conv-translate-btn").click();
      await again.getByText("〔译〕").first().waitFor({ timeout: 5_000 });
      await again.locator(".conv-translated", { hasText: "已翻译" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "chat-translated");
      await again.locator(".conv-translated button", { hasText: "显示原文" }).click();
      await again.locator(".conv-translated").waitFor({ state: "detached", timeout: 5_000 });
      // The key goes again, for the next pass.
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "翻译" }));
      await page.getByRole("button", { name: "删掉密钥" }).click();
      await page.locator('.tr-fields input[placeholder="粘贴 API 密钥"]').waitFor({ timeout: 5_000 });
      if (narrow) await sidebar(page, false);
    });

    // 只看进度: runs show what the agent reported, not its steps.
    await step("设置 · 只看进度", async () => {
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "外观" }));
      await page.getByRole("tab", { name: "只看进度" }).click();
      if (narrow) await sidebar(page, false);
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      await settle(page, 600);
      const steps = await page.locator(".conv-detail .trace-group, .conv-detail .trace-think").count();
      if (steps) throw new Error(`只看进度时不该显示思考和命令，还有 ${steps} 处`);
      await shot(page, size, scheme, "chat-progress-only");
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "外观" }));
      await page.getByRole("tab", { name: "全部" }).click();
      if (narrow) await sidebar(page, false);
    });

    await step("单聊 · 点选卡", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      const box = page.locator(".conv-detail .conv-compose-input");
      // No round running (the chat may still be empty when run alone).
      await page.waitForFunction(() => {
        const el = document.querySelector(".conv-detail .conv-compose-input");
        return !!el && !(el.getAttribute("placeholder") || "").includes("补充");
      }, null, { timeout: 30_000 });
      await box.fill("把演示剪成 40 秒，开场怎么放你问我");
      await box.press("Enter");
      const card = page.locator(".conv-detail .ask.is-live").last();
      await card.waitFor({ timeout: 15_000 });
      if (await page.locator(".conv-detail .trace.is-live").count()) throw new Error("等你决定时不该还显示在干活");
      if (!((await box.getAttribute("placeholder")) || "").includes("回答")) throw new Error("等你决定时输入框应提示回答它的问题");
      await settle(page, 300);
      await shot(page, size, scheme, "ask-card");
      await card.getByRole("button", { name: /先放结果/ }).click();
      await page.locator(".conv-detail .ask.is-answered .ask-answer", { hasText: "你选了：先放结果" }).last().waitFor({ timeout: 10_000 });
      if (await page.locator(".conv-detail .conv-supplement", { hasText: "先放结果" }).count()) throw new Error("回答不该再显示成一条补充");
      await page.waitForFunction(() => {
        const d = document.querySelector(".conv-detail");
        return !!d && d.querySelectorAll(".trace.is-live, .trace.is-waiting").length === 0;
      }, null, { timeout: 30_000 });
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // While a round is running the box speaks for that round: Enter adds to
    // it (one run, one loading) and ⌘/Ctrl+Enter puts the next message
    // behind it, listed above the box until the round ends.
    await step("单聊 · 补充和排队", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      const task = "整理发布说明，按模块分组";
      await box.fill(task);
      await box.press("Enter");
      await page.locator(".conv-msg.is-me", { hasText: task }).last().waitFor({ timeout: 20_000 });
      // Queued first, while there is time before this round ends.
      const nextText = "然后写个标题";
      await box.fill(nextText);
      await box.press(NEXT_KEY);
      const rows = page.locator(".conv-detail .conv-queue-row");
      await rows.first().waitFor({ timeout: 10_000 });
      const dropText = "这条撤回";
      await box.fill(dropText);
      // (with a picture: editing it gives the picture back too)
      await page.locator(".conv-detail .conv-compose input[type=file]").setInputFiles({ name: "排队配图.png", mimeType: "image/png", buffer: Buffer.from(TINY_PNG, "base64") });
      await page.locator(".conv-detail .conv-compose .att-chip:not(.is-busy)").first().waitFor({ timeout: 10_000 });
      await box.press(NEXT_KEY);
      await page.waitForFunction(() => !document.querySelector(".conv-detail .conv-compose .att-chip"), null, { timeout: 10_000 });
      // (A long conversation takes a moment to draw the second row.)
      await page.waitForFunction(() => document.querySelectorAll(".conv-detail .conv-queue-row").length === 2, null, { timeout: 5_000 }).catch(() => undefined);
      if ((await rows.count()) !== 2) throw new Error(`连着排两条应有两行，实际 ${await rows.count()} 行`);
      const order = (await rows.allInnerTexts()).map(compactText);
      if (!order[0].includes(nextText) || !order[1].includes(dropText)) throw new Error(`排队应按先后排：${JSON.stringify(order)}`);
      if (!order[0].includes("排队中")) throw new Error(`排队条应写「排队中：」，实际「${order[0]}」`);
      // Big enough to hit: the row and its buttons.
      const sizes = await rows.first().evaluate((el) => ({ row: el.getBoundingClientRect().height, btn: Math.min(...[...el.querySelectorAll("button")].map((b) => b.getBoundingClientRect().height)) }));
      if (sizes.row < 32 || sizes.btn < 24) throw new Error(`排队条太小，不好点：行 ${sizes.row}px，按钮 ${sizes.btn}px`);
      // Enter now adds to the running task: in its run, where you sent it,
      // not a second task.
      const addText = "补充：下载页的链接也顺带检查";
      await box.fill(addText);
      await box.press("Enter");
      const add = page.locator(".conv-detail .trace .trace-steer", { hasText: "下载页的链接" });
      await add.waitFor({ timeout: 20_000 });
      const oneBubble = await page.evaluate(({ addText }) => {
        const said = [...document.querySelectorAll(".conv-detail .conv-msg.is-me")].filter((el) => (el.textContent || "").includes(addText));
        return said.length === 1 && !!said[0].closest(".trace");
      }, { addText });
      if (!oneBubble) throw new Error("补充应接在那条任务的过程里，只有一处");
      await page.waitForFunction(() => {
        const d = document.querySelector(".conv-detail");
        return !!d && d.querySelectorAll(".trace.is-live, .trace.is-waiting, .trace-group.is-live").length === 1;
      }, null, { timeout: 10_000 }).catch(() => undefined);
      const live = await page.locator(".conv-detail .trace.is-live, .conv-detail .trace.is-waiting, .conv-detail .trace-group.is-live").count();
      if (live !== 1) throw new Error(`补充之后应只有这一轮在转，实际 ${live} 处`);
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-supplement");
      // Edit takes a queued message back into the box (it is withdrawn first).
      await clickVisible(page, rows.nth(1).getByRole("button", { name: "编辑" }));
      await page.waitForFunction(() => document.querySelectorAll(".conv-detail .conv-queue-row").length === 1, null, { timeout: 10_000 });
      if ((await rows.count()) !== 1) throw new Error("编辑之后那一行应消失");
      const back = await box.inputValue();
      if (back !== dropText) throw new Error(`编辑应把内容放回输入框，实际「${back}」`);
      if (!(await page.locator(".conv-detail .conv-compose .att-chip").count())) throw new Error("编辑应把图片附件也放回输入框");
      // Put it back in the queue, then withdraw it for good.
      await box.press(NEXT_KEY);
      await rows.nth(1).waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "conversation-queue");
      await clickVisible(page, rows.nth(1).getByRole("button", { name: "撤回" }));
      await page.waitForFunction(() => document.querySelectorAll(".conv-detail .conv-queue-row").length === 1, null, { timeout: 10_000 });
      if ((await rows.count()) !== 1) throw new Error("撤回之后排队条应只剩一条");
      // This round's answer ends it: the message queued behind goes out.
      await page.locator(".conv-msg.is-me", { hasText: nextText }).last().waitFor({ timeout: 30_000 });
      await page.waitForFunction(() => !document.querySelector(".conv-detail .conv-queue"), null, { timeout: 10_000 });
      await page.waitForTimeout(1500);
      if (await page.locator(".conv-msg.is-me", { hasText: dropText }).count()) throw new Error("撤回的消息不该发出去");
      // Never sent: not in the conversation as 已停止 either, even after a reload.
      await page.reload({ waitUntil: "domcontentloaded" });
      await page.locator(".app-shell").waitFor();
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await page.locator(".conv-detail .conv-msg.is-me", { hasText: nextText }).last().waitFor({ timeout: 20_000 });
      await page.waitForTimeout(800);
      if (await page.locator(".conv-detail .conv-msg.is-me", { hasText: dropText }).count()) throw new Error("没发出去的排队消息不该出现在对话里（已停止）");
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-queue-sent");
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // Two queued behind a round go out one after another, each after the
    // answer before it (not both at once, not in the middle); taking out
    // the first does not let the second go early. ↑ / ↓ from an empty box
    // go through what was sent.
    await step("单聊 · 排队依次发", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      const rows = page.locator(".conv-detail .conv-queue-row");
      const t = "先列一下要改的页面";
      await box.fill(t);
      await box.press("Enter");
      await page.locator(".conv-msg.is-me", { hasText: t }).last().waitFor({ timeout: 20_000 });
      const gone = "这条先撤掉";
      const a = "第一条排队：改首页标题";
      const b = "第二条排队：改下载页按钮";
      for (const text of [gone, a, b]) {
        await box.fill(text);
        await box.press(NEXT_KEY);
        await page.waitForFunction((n) => document.querySelectorAll(".conv-detail .conv-queue-row").length === n, [gone, a, b].indexOf(text) + 1, { timeout: 10_000 });
      }
      await clickVisible(page, rows.first().getByRole("button", { name: "撤回" }));
      await page.waitForFunction(() => document.querySelectorAll(".conv-detail .conv-queue-row").length === 2, null, { timeout: 10_000 });
      await page.waitForTimeout(1500);
      if (await page.locator(".conv-msg.is-me", { hasText: a }).count()) throw new Error("撤回第一条后，后面的不该提前发出");
      // The round ends: only the first goes; the second still waits.
      await page.locator(".conv-msg.is-me", { hasText: a }).last().waitFor({ timeout: 30_000 });
      if (await page.locator(".conv-msg.is-me", { hasText: b }).count()) throw new Error("排的第二条应等第一条做完再发，不能同时发出");
      if ((await rows.count()) !== 1) throw new Error(`第一条发出后应还剩一条在排队，实际 ${await rows.count()} 条`);
      await page.locator(".conv-msg.is-me", { hasText: b }).last().waitFor({ timeout: 30_000 });
      const placed = await page.evaluate(({ t, a, b }) => {
        const els = [...document.querySelectorAll(".conv-detail .conv-msg")];
        const at = (text) => els.findIndex((e) => e.classList.contains("is-me") && (e.textContent || "").includes(text));
        const answerAfter = (i) => els.findIndex((e, k) => k > i && e.classList.contains("is-agent"));
        const it = at(t), ia = at(a), ib = at(b);
        return { ok: ia > answerAfter(it) && answerAfter(it) > it && ib > answerAfter(ia) && answerAfter(ia) > ia, it, ia, ib };
      }, { t, a, b });
      if (!placed.ok) throw new Error(`排队的消息应接在前一条的回复后面：${JSON.stringify(placed)}`);
      await page.waitForFunction(() => !document.querySelector(".conv-detail .trace.is-live, .conv-detail .trace.is-waiting"), null, { timeout: 30_000 });
      // ↑ / ↓: newest first, back down to the empty box.
      await box.fill("");
      await box.press("ArrowUp");
      if ((await box.inputValue()) !== b) throw new Error(`↑ 应拿出最后发的一条，实际「${await box.inputValue()}」`);
      await box.press("ArrowUp");
      if ((await box.inputValue()) !== a) throw new Error(`再 ↑ 应是前一条，实际「${await box.inputValue()}」`);
      await box.press("ArrowDown");
      await box.press("ArrowDown");
      if ((await box.inputValue()) !== "") throw new Error(`↓ 到底应回到空的输入框，实际「${await box.inputValue()}」`);
      await box.fill("正在写的字");
      await box.press("ArrowUp");
      if ((await box.inputValue()) !== "正在写的字") throw new Error("有字时 ↑ 不该换掉正在写的内容");
      await box.fill("");
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // Without a project, the group has a box that says whom a task goes to.
    await step("聊天 · 派活", async () => {
      const to = compactText(await page.locator(".sp-to-name").innerText());
      if (!to) throw new Error("输入框应写明发给谁");
      const body = `smoke ${size.width} ${scheme}：把首页的标题改短一点。`;
      const input = page.locator(".sp-compose-input");
      // The box grows with several lines instead of scrolling one.
      const oneLine = await input.evaluate((el) => el.getBoundingClientRect().height);
      await input.fill("第一行\n第二行\n第三行\n第四行");
      const grown = await input.evaluate((el) => ({ h: el.getBoundingClientRect().height, scrolls: el.scrollHeight > el.clientHeight + 1 }));
      if (grown.h < oneLine * 2.5 || grown.scrolls) throw new Error(`输入框应随内容变高而不出现滚动条：一行 ${oneLine}px，四行 ${grown.h}px，滚动 ${grown.scrolls}`);
      await settle(page, 200);
      await shot(page, size, scheme, "compose-lines");
      await input.fill(body);
      await input.press("Enter");
      const card = page.locator(".sp-card", { hasText: body });
      await card.waitFor({ timeout: 20_000 });
      const who = compactText(await card.locator(".sp-card-who").innerText());
      if (!who.startsWith("你")) throw new Error(`派出去的任务应写明是你发的，实际是「${who}」`);
    });

    await step("单聊 · 只回一句话", async () => {
      await openMember(page, narrow, "聊天", "legacy-codex");
      const head = page.locator(".conv-detail .conv-head h2");
      const headText = compactText(await head.innerText());
      if (!/legacy-codex/.test(headText)) throw new Error(`点成员 legacy-codex 进入的不是它的对话：「${headText}」`);

      const body = "你好，收到请回复 收到";
      // This chat holds the stuck demo task, so Enter would add to that one
      // instead of sending: the task arrives from "another device" here.
      await sendFromElsewhere("hub_studio", "legacy-codex", body);
      await page.locator(".conv-msg", { hasText: body }).last().waitFor({ timeout: 20_000 });
      // The header speaks for the whole conversation (it can say 可能卡住 for
      // an older task here), so wait for this task's own answer instead (the
      // mock replies 6 s after it), then for its final trace to be read.
      await page.waitForFunction((text) => {
        const msgs = [...document.querySelectorAll(".conv-detail .conv-msg")];
        const i = msgs.map((el) => (el.textContent || "").includes(text)).lastIndexOf(true);
        return i >= 0 && msgs.slice(i + 1).some((el) => el.classList.contains("is-agent"));
      }, body, { timeout: 30_000 });
      // Its run line goes once its answer is in.
      await page.waitForFunction((text) => {
        const d = document.querySelector(".conv-detail");
        const sent = [...(d?.querySelectorAll(".conv-msg") || [])].filter((el) => (el.textContent || "").includes(text)).pop();
        return !!sent && ![...d.querySelectorAll(".trace.is-live")].some((t) => sent.compareDocumentPosition(t) & Node.DOCUMENT_POSITION_FOLLOWING);
      }, body, { timeout: 15_000 }).catch(() => undefined);
      await page.waitForTimeout(800);
      const traces = await page.evaluate((text) => {
        const detail = document.querySelector(".conv-detail");
        const sent = [...(detail?.querySelectorAll(".conv-msg") || [])].filter((el) => (el.textContent || "").includes(text)).pop();
        if (!sent) return -1;
        // (what it reported along the way may be there, as text: no steps)
        return [...detail.querySelectorAll(".trace-group, .trace-head:not(.trace-wait)")].filter((t) => sent.compareDocumentPosition(t) & Node.DOCUMENT_POSITION_FOLLOWING).length;
      }, body);
      if (traces !== 0) throw new Error(`只回一句话的任务不应出现过程（步骤）块，实际有 ${traces} 个`);
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-text-only");
    });

    await step("单聊 · 有工具调用", async () => {
      const body = "整理一下这次发布的主要变化，列成要点。";
      // As above: the stuck demo task keeps the box in "add" mode here.
      await sendFromElsewhere("hub_studio", "legacy-codex", body);
      await page.locator(".conv-msg", { hasText: body }).last().waitFor({ timeout: 20_000 });
      // As above: the mock finishes this task 6 s after it is sent.
      await page.waitForTimeout(7_500);
      await page.waitForFunction(
        () => {
          const detail = document.querySelector(".conv-detail");
          return !!detail && detail.querySelectorAll(".trace").length > 0
            && detail.querySelectorAll(".trace-spin").length === 0;
        },
        null,
        { timeout: 10_000 },
      );
      const traces = await page.locator(".conv-detail .trace").count();
      if (traces === 0) throw new Error("有工具调用的任务完成后应有过程块，实际没有 .trace");
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-tool-trace");
    });

    // A task the node accepted long ago and went quiet on may have lost its
    // result: the chat says so instead of spinning, and one click stops it
    // and sends the same message again.
    await step("单聊 · 可能卡住", async () => {
      await openMember(page, narrow, "聊天", "legacy-codex");
      await page.waitForFunction(
        () => (document.querySelector(".conv-detail .conv-head-state")?.textContent || "").includes("可能卡住"),
        null,
        { timeout: 10_000 },
      );
      // This conversation also holds the 19-hour task of the next step: pick
      // the one quiet for minutes.
      const note = page.locator(".conv-stuck", { hasText: "分钟没有动静" });
      await note.waitFor({ timeout: 10_000 });
      const noteText = compactText(await note.innerText());
      if (!noteText.includes("没有动静") || !noteText.includes("可能卡住")) {
        throw new Error(`卡住的任务应有一行说明，实际是「${noteText}」`);
      }
      const spins = await page.locator(".conv-detail .trace-spin").count();
      if (spins > 0) throw new Error(`卡住的任务不应转圈，实际有 ${spins} 个 .trace-spin`);
      // What it did and reported stays (its run), the line goes under it;
      // the quiet counts from what it last reported.
      const said = page.locator(".conv-detail .trace .trace-report", { hasText: "整理到一半" });
      await said.waitFor({ timeout: 10_000 });
      const quietMin = Number(/^(\d+) 分钟没有动静/.exec(noteText)?.[1]);
      const sinceReport = 40 + Math.floor((Date.now() - stuckResetAt) / 60_000);
      if (!(Math.abs(quietMin - sinceReport) <= 1)) throw new Error(`没有动静的时间应从它最后报的进度算起（约 ${sinceReport} 分钟），实际「${noteText}」`);
      const below = await note.evaluate((el, s) => {
        const r = [...document.querySelectorAll(".conv-detail .trace-report")].find((x) => (x.textContent || "").includes(s));
        return !!r && r.getBoundingClientRect().bottom <= el.getBoundingClientRect().top + 1;
      }, "整理到一半");
      if (!below) throw new Error("「可能卡住」那一行应在它的过程下面");
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-stuck");
      const body = "把旧版 runner 的日志整理到 docs/legacy.md。";
      await clickVisible(page, note.getByRole("button", { name: "重发" }));
      await page.waitForFunction((t) => ![...document.querySelectorAll(".conv-stuck")].some((e) => (e.textContent || "").includes(t)), "分钟没有动静", { timeout: 10_000 });
      await page.waitForFunction(
        () => (document.querySelector(".conv-detail")?.textContent || "").includes("已停止"),
        null,
        { timeout: 10_000 },
      );
      const sent = page.locator(".conv-msg", { hasText: body });
      const count = await sent.count();
      if (count < 2) throw new Error(`重发后应有旧任务和新任务两条，实际 ${count} 条`);
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-stuck-resend");
    });

    // An older task can be the stuck one after a newer task was stopped:
    // the newest message being closed must not hide it, and its own row has
    // the stop and resend. It also has no active_at, so updated_at decides.
    await step("单聊 · 更早的卡住任务", async () => {
      // The same chat as the previous step.
      await openMember(page, narrow, "聊天", "legacy-codex");
      await page.waitForFunction(
        () => (document.querySelector(".conv-detail .conv-head-state")?.textContent || "").includes("可能卡住"),
        null,
        { timeout: 10_000 },
      );
      const note = page.locator(".conv-stuck", { hasText: "19 小时没有动静" });
      await note.waitFor({ timeout: 10_000 });
      // 可能卡住 in the header takes you to the stuck task.
      await page.locator(".conv-detail .conv-head-state.is-link").click();
      await page.waitForFunction(() => {
        const el = document.querySelector(".conv-stuck.is-flash");
        const box = document.querySelector(".conv-scroll")?.getBoundingClientRect();
        const r = el?.getBoundingClientRect();
        return !!el && !!box && !!r && r.top >= box.top && r.bottom <= box.bottom;
      }, null, { timeout: 5_000 });
      const noteText = compactText(await note.innerText());
      // 19 hours ago and no active_at in the mock: updated_at is the fallback.
      if (!noteText.includes("19 小时没有动静") || !noteText.includes("可能卡住")) {
        throw new Error(`更早的卡住任务应说明它 19 小时没有动静，实际是「${noteText}」`);
      }
      await note.getByRole("button", { name: "停止" }).waitFor({ timeout: 10_000 });
      await note.getByRole("button", { name: "重发" }).waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-stuck-older");
      const body = "把旧版 runner 的配置文件检查一遍。";
      await clickVisible(page, note.getByRole("button", { name: "重发" }));
      await page.waitForFunction((t) => ![...document.querySelectorAll(".conv-stuck")].some((e) => (e.textContent || "").includes(t)), "小时没有动静", { timeout: 10_000 });
      await page.waitForFunction(
        () => (document.querySelector(".conv-detail")?.textContent || "").includes("已停止"),
        null,
        { timeout: 10_000 },
      );
      const sent = page.locator(".conv-msg", { hasText: body });
      const count = await sent.count();
      if (count < 2) throw new Error(`更早的任务重发后应有旧任务和新任务两条，实际 ${count} 条`);
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-stuck-older-resend");
    });


    // An offline agent says why, as far as can be told: here its runner
    // said goodbye as it was stopped.
    await step("成员 · 离线原因", async () => {
      await go(page, "成员", narrow);
      await clickVisible(page, page.locator(".mb-card", { hasText: "docs-claude" }).first());
      const why = page.locator('.ms .app-offline-why[data-offline-why="stopped"]');
      await why.waitFor({ timeout: 10_000 });
      if (!(await why.innerText()).includes("被停掉了")) throw new Error("runner 停下时说过再见，离线说明应写它被停掉了");
      await why.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "member-offline-why");
      await page.keyboard.press("Escape");
    });

    await step("成员", async () => {
      await go(page, "成员", narrow);
      await page.locator(".mb-card").first().waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "members");
      await page.locator(".mb-card").first().click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "member-card");
      // One card: the description stays on one line, and the details
      // unfold inside it instead of opening another panel.
      const descLines = await page.locator(".ms .ms-desc").evaluate((el) => el.tagName === "INPUT" && getComputedStyle(el).whiteSpace === "nowrap");
      if (!descLines) throw new Error("成员卡片的描述应该只有一行");
      await clickVisible(page, page.locator(".ms .ms-fold"));
      await page.locator(".ms .ms-fold-body .app-kv").first().waitFor({ timeout: 5_000 });
      if (await page.locator(".app-inspector").count()) throw new Error("详细信息不该再弹出另一个面板");
      await page.locator(".ms .ms-fold-body").scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "member-card-details");
      // Delete asks calmly: who it is about, plain words, no red block.
      const del = page.locator(".ms .app-action-row.danger", { hasText: "删除这个 agent" });
      if (await del.count() && await del.isEnabled()) {
        await del.scrollIntoViewIfNeeded();
        await del.click();
        const dlg = page.locator(".app-modal.app-confirm");
        await dlg.waitFor({ timeout: 5_000 });
        await dlg.locator(".app-confirm-subject .pa").waitFor({ timeout: 5_000 });
        if (await dlg.locator(".app-warn").count()) throw new Error("删除确认不该再有红色警告块");
        await settle(page, 300);
        await shot(page, size, scheme, "member-delete-confirm");
        await dlg.getByRole("button", { name: "取消" }).click();
        await dlg.waitFor({ state: "hidden" });
      }
      // A face of its own: a mushroom picked from the grid, then a picture,
      // then back to the original.
      await page.locator(".ms").evaluate((el) => el.scrollTo(0, 0));
      await clickVisible(page, page.locator(".ms .ms-face-btn"));
      const face = page.locator(".app-modal.av-modal");
      await face.waitFor({ timeout: 5_000 });
      await face.locator(".av-color").nth(2).click();
      await face.locator(".av-cell").nth(5).click();
      await settle(page, 200);
      await shot(page, size, scheme, "member-face");
      await face.getByRole("button", { name: "用这个头像" }).click();
      await face.waitFor({ state: "hidden", timeout: 5_000 });
      await page.waitForFunction(() => {
        const cap = document.querySelector(".ms .ms-face .pa")?.style.getPropertyValue("--av-cap");
        return cap && cap.toLowerCase() === "#e7a33e";
      }, null, { timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-face-btn"));
      await face.waitFor({ timeout: 5_000 });
      // Only a picture: anything else is turned away with a word.
      await face.locator("input[type=file]").setInputFiles({ name: "notes.txt", mimeType: "text/plain", buffer: Buffer.from("not a picture") });
      await face.getByText("只能用图片").waitFor({ timeout: 5_000 });
      await face.locator("input[type=file]").setInputFiles({ name: "me.png", mimeType: "image/png", buffer: Buffer.from(TINY_PNG, "base64") });
      await face.locator(".av-top .pa-pic").waitFor({ timeout: 5_000 });
      await face.getByRole("button", { name: "用这个头像" }).click();
      await face.waitFor({ state: "hidden", timeout: 5_000 });
      await page.waitForFunction(() => {
        const img = document.querySelector(".ms .ms-face .pa-pic");
        return img && img.complete && img.naturalWidth > 0;
      }, null, { timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-face-btn"));
      await face.waitFor({ timeout: 5_000 });
      await face.getByRole("button", { name: "用回原来的" }).click();
      await face.getByRole("button", { name: "用这个头像" }).click();
      await face.waitFor({ state: "hidden", timeout: 5_000 });
      await page.locator(".ms .ms-face .pa-pic").waitFor({ state: "detached", timeout: 10_000 });
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });
    });

    await step("成员 · 重新接入", async () => {
      // PM has a resume line last used in a sandbox: making a new one kills
      // it, the dialog shows the curl command, and revoking goes back to none.
      const pm = page.locator(".mb-card", { hasText: "PM (Codex)" });
      await pm.scrollIntoViewIfNeeded();
      await pm.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const resume = page.locator(".ms .ms-fold-body .app-kv", { hasText: "重新接入" });
      await resume.waitFor({ timeout: 5_000 });
      await resume.scrollIntoViewIfNeeded();
      if (!(await resume.innerText()).includes("codespaces-4f2a")) {
        throw new Error("重新接入应该说明上次在哪个沙箱用过");
      }
      await settle(page, 200);
      await shot(page, size, scheme, "member-resume");
      await resume.getByRole("button", { name: "生成新链接" }).click();
      const resumeConfirm = page.locator(".app-modal.app-confirm");
      await resumeConfirm.waitFor({ timeout: 5_000 });
      if (!(await resumeConfirm.innerText()).includes("旧的链接会立刻作废")) {
        throw new Error("生成新链接前应说明旧链接会作废");
      }
      await resumeConfirm.getByRole("button", { name: "生成新链接" }).click();
      const linkDlg = page.locator(".app-modal", { hasText: "重新接入链接" });
      await linkDlg.waitFor({ timeout: 5_000 });
      const resumeCmd = (await linkDlg.locator(".app-command code").innerText()).trim();
      if (!resumeCmd.includes("curl -fsSL")) throw new Error(`弹窗里的重新接入命令不对：${resumeCmd}`);
      await settle(page, 200);
      await shot(page, size, scheme, "member-resume-link");
      await linkDlg.getByRole("button", { name: "完成" }).click();
      await linkDlg.waitFor({ state: "hidden" });
      await resume.getByText("还没用过").waitFor({ timeout: 5_000 });
      await resume.getByRole("button", { name: "作废", exact: true }).click();
      await resumeConfirm.waitFor({ timeout: 5_000 });
      await resumeConfirm.getByRole("button", { name: "作废", exact: true }).click();
      await resumeConfirm.waitFor({ state: "hidden" });
      await resume.getByText("还没有链接").waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });
    });

    // 「换一个来接着做」: PM was taken over from a claude.ai session; its card
    // lists that, with a way back, and the dialog offers a new session on a
    // computer (old computers need an update first) or a one-time command.
    await step("成员 · 换一个来接着做", async () => {
      await go(page, "成员", narrow);
      const pm = page.locator(".mb-card", { hasText: "PM (Codex)" });
      await pm.scrollIntoViewIfNeeded();
      await pm.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      const history = page.locator(".ms [data-testid=handover-history]");
      await history.waitFor({ timeout: 10_000 });
      const said = compactText(await history.innerText());
      for (const want of ["claude.ai 云端", "Studio Mac · Codex", "换回这个", "没用上"]) {
        if (!said.includes(want)) throw new Error(`接手记录里应有「${want}」，实际「${said}」`);
      }
      await history.scrollIntoViewIfNeeded();
      await settle(page, 200);
      await shot(page, size, scheme, "member-handovers");
      await clickVisible(page, page.locator(".ms [data-testid=handover-open]"));
      const dlg = page.locator(".app-modal", { hasText: "换一个来接着做" });
      await dlg.waitFor({ timeout: 5_000 });
      const form = compactText(await dlg.innerText());
      for (const want of ["只是换个账号", "Studio Mac", "要先升级", "原来的会话会停下", "最近 30 条"]) {
        if (!form.includes(want)) throw new Error(`接手弹窗里应有「${want}」，实际「${form}」`);
      }
      // The folder of the computer it is on now is filled in.
      if ((await dlg.locator("input.mono").inputValue()) !== "~/code/myco") throw new Error("同一台电脑上应默认原来的文件夹");
      await settle(page, 200);
      await shot(page, size, scheme, "handover-machine");
      await clickVisible(page, dlg.locator("[data-testid=handover-way-link]"));
      await clickVisible(page, dlg.locator("[data-testid=handover-start]"));
      const cmd = dlg.locator("[data-testid=handover-command]");
      await cmd.waitFor({ timeout: 5_000 });
      const line = (await cmd.locator(".app-command code").innerText()).trim();
      if (!line.startsWith("curl -fsSL") || !line.includes("/h/myco_takeover_")) throw new Error(`接手命令不对：${line}`);
      await clickVisible(page, cmd.getByRole("tab", { name: "Windows（PowerShell）" }));
      if (!(await cmd.locator(".app-command code").innerText()).includes("iex")) throw new Error("PowerShell 的接手命令不对");
      await clickVisible(page, cmd.getByRole("tab", { name: "Mac / Linux / 云端" }));
      await settle(page, 200);
      await shot(page, size, scheme, "handover-command");
      // Esc closes the dialog, not the card under it.
      await page.keyboard.press("Escape");
      await dlg.waitFor({ state: "hidden" });
      if (!(await page.locator(".ms").isVisible())) throw new Error("按 Esc 只该关掉弹窗，不该连资料卡一起关掉");
      await history.getByText("等它接手").waitFor({ timeout: 10_000 });
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });
    });

    await step("成员 · 模型和额度", async () => {
      // A manager picks a model and an effort: each goes to the App backend
      // at once with both fields. The quota rows say how much each window
      // used and when it resets; a nearly-used one is amber.
      const lastBody = async (pred, what) => {
        for (let i = 0; i < 50; i++) {
          const r = await fetch(`http://${HOST}:${mockPort()}/app/mock/run-settings`);
          if (r.ok) {
            const { last } = await r.json();
            if (last && pred(last)) return last;
          }
          await sleep(100);
        }
        throw new Error(`App 后端没有收到${what}`);
      };
      const color = (name) => page.evaluate((n) => {
        // Inside the App: its colour tokens are set on .app-shell, not body.
        const el = document.createElement("span");
        el.style.color = `var(${n})`;
        (document.querySelector(".app-shell") || document.body).appendChild(el);
        const c = getComputedStyle(el).color;
        el.remove();
        return c;
      }, name);

      const pm = page.locator(".mb-card", { hasText: "PM (Codex)" });
      await pm.scrollIntoViewIfNeeded();
      await pm.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const body = page.locator(".ms .ms-fold-body");
      await body.locator(".app-kv", { hasText: "模型" }).first().waitFor({ timeout: 5_000 });

      const modelSelect = body.locator(".app-kv", { hasText: "模型" }).first().locator("select");
      const effortSelect = body.locator(".app-kv", { hasText: "推理强度" }).first().locator("select");
      await modelSelect.waitFor({ timeout: 5_000 });
      await effortSelect.waitFor({ timeout: 5_000 });
      await modelSelect.selectOption("gpt-6");
      await lastBody((x) => x.agent === "pm-codex" && x.body.model === "gpt-6", "模型的修改");
      await page.waitForFunction(() => (document.querySelector(".ms .ms-fold-body")?.textContent || "").includes("现在是：gpt-6（"), null, { timeout: 5_000 });
      await effortSelect.selectOption("low");
      const effortBody = await lastBody((x) => x.agent === "pm-codex" && x.body.effort === "low", "推理强度的修改");
      if (effortBody.body.model !== "gpt-6") {
        throw new Error(`改推理强度时应带上当前模型，实际是「${effortBody.body.model}」`);
      }

      const five = body.locator(".app-quota", { hasText: "5 小时" });
      await five.waitFor({ timeout: 5_000 });
      const fiveText = compactText(await five.innerText());
      if (!fiveText.includes("95%") || !fiveText.includes("重置")) {
        throw new Error(`5 小时窗口应写已用 95% 和重置时间，实际是「${fiveText}」`);
      }
      if (!(await five.getAttribute("class") || "").includes("is-tight")) {
        throw new Error("用了 95% 的窗口应标成琥珀色");
      }
      const amber = await color("--amber");
      const fiveColor = await five.locator(".app-kv-value").evaluate((el) => getComputedStyle(el).color);
      if (fiveColor !== amber) throw new Error(`用了 95% 的窗口应显示成琥珀色，实际是 ${fiveColor}`);
      const sevenText = compactText(await body.locator(".app-quota", { hasText: "7 天" }).innerText());
      if (!sevenText.includes("62%")) throw new Error(`7 天窗口应写已用 62%，实际是「${sevenText}」`);
      await five.scrollIntoViewIfNeeded();
      await settle(page, 200);
      await shot(page, size, scheme, "member-run");
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });

      // A spent window is red, and usage two hours old says so.
      const review = page.locator(".mb-card", { hasText: "Review (Codex 云端)" });
      await review.scrollIntoViewIfNeeded();
      await review.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const spent = page.locator(".ms .ms-fold-body .app-quota", { hasText: "5 小时" });
      await spent.waitFor({ timeout: 5_000 });
      if (!(await spent.innerText()).includes("额度用完")) throw new Error("用完的窗口应写「额度用完」");
      if (!(await spent.getAttribute("class") || "").includes("is-over")) throw new Error("用完的窗口应标成红色");
      const red = await color("--red");
      const spentColor = await spent.locator(".app-kv-value").evaluate((el) => getComputedStyle(el).color);
      if (spentColor !== red) throw new Error(`用完的窗口应显示成红色，实际是 ${spentColor}`);
      await page.locator(".ms .ms-fold-body .app-quota-stale", { hasText: "的数据" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "member-quota-spent");
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });

      // An agent that receives in its own session: no controls, and the
      // model is what it reported itself.
      const grok = page.locator(".mb-card", { hasText: "Myco (Grok)" });
      await grok.scrollIntoViewIfNeeded();
      await grok.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const inbox = page.locator(".ms .ms-fold-body");
      await inbox.getByText("模型和额度由它自己的会话管理，myco 拿不到也改不了。").waitFor({ timeout: 5_000 });
      if (await inbox.locator("select").count()) throw new Error("inbox wait 的 agent 不该有可编辑控件");
      const inboxText = compactText(await inbox.innerText());
      if (!inboxText.includes("grok-4") || !inboxText.includes("它自己报的")) {
        throw new Error(`inbox agent 应显示它自己报的模型，实际是「${inboxText}」`);
      }
      await settle(page, 200);
      await shot(page, size, scheme, "member-run-inbox");
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });
    });

    await step("成员 · 修改权限", async () => {
      await go(page, "成员", narrow);
      const openRun = async (name) => {
        const card = page.locator(".mb-card", { hasText: name }).first();
        await card.scrollIntoViewIfNeeded();
        await card.click();
        await page.locator(".ms").waitFor({ timeout: 10000 });
        await clickVisible(page, page.locator(".ms .ms-fold"));
        return page.locator(".ms .ms-fold-body .app-kv").filter({ has: page.locator(".app-kv-label", { hasText: /^权限$/ }) });
      };
      const closeRun = async () => {
        await page.keyboard.press("Escape");
        await page.locator(".ms").waitFor({ state: "hidden" });
      };
      const savedAccess = async (agent, access) => {
        for (let i = 0; i < 60; i++) {
          const data = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/run-settings`)).json();
          if (data.last?.agent === agent && data.last.body.access === access) return data.last.body;
          await sleep(100);
        }
        throw new Error(`没有保存 ${agent} 的权限 ${access}`);
      };
      const permission = await openRun("PM (Codex)");
      const select = permission.getByRole("combobox", { name: "权限", exact: true });
      await select.waitFor({ timeout: 5000 });
      if (!(await select.locator('option[value="full"]').isDisabled())) throw new Error("超过工作区上限的完全权限应禁用");
      if (!(await permission.innerText()).includes('这台电脑最多允许「工作区」')) throw new Error("需要说明这台电脑的权限上限");
      const model = await page.locator('.ms select[aria-label="模型"]').inputValue();
      const effort = await page.locator('.ms select[aria-label="推理强度"]').inputValue();
      await select.selectOption("readonly");
      const request = await savedAccess("pm-codex", "readonly");
      if (request.model !== model || request.effort !== effort || Object.keys(request).sort().join(",") !== "access,effort,model") throw new Error("修改权限只能发送当前模型、推理强度和新权限");
      await permission.getByText(/现在是：工作区（App）/).waitFor({ timeout: 5000 });
      if (!(await permission.innerText()).includes("下次唤醒时生效")) throw new Error("需要说明下次唤醒生效");
      await shot(page, size, scheme, "member-access-capped");
      await select.selectOption("");
      await savedAccess("pm-codex", "");
      await closeRun();

      const full = await openRun("临时 Codex");
      const fullSelect = full.getByRole("combobox", { name: "权限", exact: true });
      await fullSelect.selectOption("full");
      const confirm = page.locator(".app-modal.app-confirm");
      await confirm.waitFor({ timeout: 5000 });
      if (!(await confirm.innerText()).includes("没有沙箱，可以读写这台电脑上的任何文件")) throw new Error("完全权限需要先说明风险");
      const before = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/run-settings`)).json();
      if (before.last?.agent === "temp-codex" && before.last.body.access === "full") throw new Error("确认之前不能保存完全权限");
      await shot(page, size, scheme, "member-access-full-confirm");
      await confirm.getByRole("button", { name: "取消", exact: true }).click();
      await confirm.waitFor({ state: "hidden" });
      await fullSelect.selectOption("full");
      await confirm.getByRole("button", { name: "使用完全权限", exact: true }).click();
      await savedAccess("temp-codex", "full");
      await confirm.waitFor({ state: "hidden", timeout: 5000 });
      await closeRun();

      const other = await openRun("docs-claude");
      if (await other.locator("select").count()) throw new Error("即使是工作区主人，也不能改别人的 agent 权限");
      await closeRun();
      const legacy = await openRun("legacy-codex");
      if (await legacy.locator("select").count()) throw new Error("旧 runner 没有上报上限时不能改权限");
      if (!(await legacy.innerText()).includes("升级那台电脑上的 myco")) throw new Error("旧 runner 应显示升级提示");
      await closeRun();
    });

    await step("电脑", async () => {
      await go(page, "电脑", narrow);
      await page.locator(".pc-row").first().waitFor({ timeout: 20_000 });
      // An agent on that computer has nearly used its quota: the rows of
      // that machine say so, quietly.
      await page.locator(".pc-quota", { hasText: "额度 95%" }).first().waitFor({ timeout: 10_000 });
      await settle(page);
      await shot(page, size, scheme, "computers");
      // A computer that has not allowed terminals is marked, and says how to
      // allow it: on that computer, never from here.
      const office = page.locator(".pc-machine", { hasText: "Office PC" });
      await office.locator(".pc-machine-tag", { hasText: "没允许开终端" }).waitFor({ timeout: 10_000 });
      await clickVisible(page, office);
      const view = page.locator("[data-testid=machine-view]");
      await view.locator("[data-testid=shell-ask] code", { hasText: "myco machine shell-access --allow" }).waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "computer-permission");
      await clickVisible(page, view.getByRole("button", { name: "好" }));
      await view.waitFor({ state: "hidden", timeout: 5_000 });
      // Who may use a computer, which projects have it, and removing it are
      // in one place, a click from its row.
      // (The folder added in the project step, at full width; else the first.)
      const pcRow = narrow ? page.locator(".pc-row").first() : page.locator(".pc-row", { hasText: `wsl-${size.width}-${scheme}` });
      await clickVisible(page, pcRow.getByRole("button", { name: "设置" }));
      const set = page.locator("[data-testid=computer-settings]");
      await set.waitFor({ timeout: 10_000 });
      await set.locator(".pcs-agent").first().waitFor({ timeout: 10_000 });
      if (!narrow) await set.locator(".pcs-project").first().waitFor({ timeout: 10_000 });
      await clickVisible(page, set.locator(".pcs-agent").last());
      await clickVisible(page, set.getByRole("button", { name: "保存" }));
      await set.getByText("已保存").waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "computer-settings");
      if (!narrow) {
        await clickVisible(page, set.getByRole("button", { name: "移除这台电脑" }));
        const sure = page.locator(".app-modal.app-confirm");
        await sure.waitFor({ timeout: 5_000 });
        await clickVisible(page, sure.getByRole("button", { name: "移除", exact: true }));
        await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 10_000 });
        await pcRow.waitFor({ state: "detached", timeout: 10_000 });
      } else {
        await clickVisible(page, set.getByRole("button", { name: "好" }));
        await set.waitFor({ state: "hidden", timeout: 5_000 });
      }
      // A folder's terminal opens from here.
      await clickVisible(page, page.locator(".pc-row").first().getByRole("button", { name: "打开", exact: true }));
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
      // And the terminal's header opens its settings too.
      await clickVisible(page, page.locator("[data-testid=computer-settings-open]").first());
      await set.waitFor({ timeout: 10_000 });
      await clickVisible(page, set.getByRole("button", { name: "好" }));
      await set.waitFor({ state: "hidden", timeout: 5_000 });
    });

    // Automations: a task sent for you on a schedule or by webhook. The list
    // says to whom and when; a new one with a webhook shows its address and
    // secret once; running one now sends it, marked 自动 in the chat.
    await step("自动化", async () => {
      // None yet: what it is for, and a few common ones to start from.
      const empty = /\/app\/hubs\/[^/]+\/automations$/;
      await page.route(empty, (r) => (r.request().method() === "GET" ? r.fulfill({ json: { automations: [] } }) : r.continue()));
      await go(page, "自动化", narrow);
      const start = page.locator("[data-testid=automations-empty]");
      await start.waitFor({ timeout: 20_000 });
      if ((await start.locator(".au-template").count()) < 5) throw new Error("空的自动化页应给几个常用模板和「从空白开始」");
      await settle(page, 200);
      await shot(page, size, scheme, "automations-empty");
      await clickVisible(page, start.locator(".au-template", { hasText: "CI 失败时排查" }));
      const ed = page.locator("[data-testid=automation-editor]");
      await ed.waitFor({ timeout: 10_000 });
      // The template fills the editor as it opens: wait for it rather than read at once.
      const body = ed.locator("textarea").first();
      for (let i = 0; !(await body.inputValue()).includes("{{payload}}"); i++) {
        if (i >= 50) throw new Error("模板应带好要它做什么");
        await page.waitForTimeout(100);
      }
      await ed.locator(".au-kind.is-on", { hasText: "Webhook" }).waitFor({ timeout: 5_000 });
      await clickVisible(page, ed.getByRole("button", { name: "取消" }));
      await ed.waitFor({ state: "detached", timeout: 5_000 });
      await page.unroute(empty);
      await go(page, "聊天", narrow);

      await go(page, "自动化", narrow);
      const list = page.locator("[data-testid=automations]");
      await list.locator(".au-row", { hasText: "每天早上汇总 issue" }).locator(".au-row-what", { hasText: "工作日 09:00" }).waitFor({ timeout: 20_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "automations");
      await clickVisible(page, page.locator(".ph").getByRole("button", { name: "新建自动化" }));
      await ed.waitFor({ timeout: 10_000 });
      const name = `CI 失败时看日志 ${size.width} ${scheme}`;
      await ed.getByPlaceholder("比如：每天早上汇总 issue").fill(name);
      await ed.locator("textarea").fill("CI 又挂了，看下日志找原因：{{payload}}");
      await clickVisible(page, ed.getByRole("radiogroup", { name: "多久一次" }).getByRole("radio", { name: "每天" }));
      await ed.getByRole("combobox", { name: "几点" }).selectOption("8");
      await ed.getByRole("combobox", { name: "几分" }).selectOption("30");
      await clickVisible(page, ed.locator(".au-kind", { hasText: "Webhook" }));
      await ed.locator(".au-summary", { hasText: "每天 08:30" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "automation-editor");
      // The buttons stay in sight however tall the form is.
      const create = ed.getByRole("button", { name: "创建" });
      const box = await create.boundingBox();
      const vh = page.viewportSize().height;
      if (!box || box.y + box.height > vh) throw new Error("编辑自动化的按钮应在窗口里看得到");
      await clickVisible(page, create);
      const sec = page.locator("[data-testid=automation-secret]");
      await sec.waitFor({ timeout: 10_000 });
      if (!(await sec.innerText()).includes("/hooks/")) throw new Error("Webhook 地址应该显示出来");
      await settle(page, 200);
      await shot(page, size, scheme, "automation-secret");
      await clickVisible(page, sec.getByRole("button", { name: "好了" }));
      const row = list.locator(".au-row", { hasText: name });
      await row.waitFor({ timeout: 10_000 });
      await row.hover();
      await clickVisible(page, row.getByRole("button", { name: "立即运行" }));
      await row.locator(".au-row-status", { hasText: "已发送" }).waitFor({ timeout: 10_000 });
      // Off and on again: a switch.
      await clickVisible(page, row.getByRole("switch", { name: "暂停" }));
      await row.locator(".au-row-status", { hasText: "已暂停" }).waitFor({ timeout: 10_000 });
      await clickVisible(page, row.getByRole("switch", { name: "启用" }));
      await row.getByRole("switch", { name: "暂停" }).waitFor({ timeout: 10_000 });
      // Right-click: the same, and delete.
      await row.click({ button: "right", position: { x: 120, y: 12 } });
      await clickVisible(page, page.getByRole("menuitem", { name: "删除" }));
      await clickVisible(page, page.locator(".app-modal.app-confirm").getByRole("button", { name: "删除" }));
      await row.waitFor({ state: "detached", timeout: 10_000 });
    });

    // A conversation is exported from its header: Markdown for people or
    // JSON Lines for programs, saved to Downloads.
    await step("单聊 · 导出对话", async () => {
      await openMember(page, narrow, "聊天", "Myco (Grok)");
      await clickVisible(page, page.locator("[data-testid=chat-more]").first());
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "导出这段对话" }));
      const dlg = page.locator("[data-testid=export-chat]");
      await dlg.waitFor({ timeout: 10_000 });
      await clickVisible(page, dlg.getByRole("checkbox"));
      await settle(page, 200);
      await shot(page, size, scheme, "export-chat");
      await clickVisible(page, page.getByRole("button", { name: "导出", exact: true }));
      await dlg.locator("[data-testid=export-saved]").waitFor({ timeout: 20_000 });
      const md = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/last-export`)).json();
      if (!/\.md$/.test(md.name) || !md.name.includes("Myco (Grok)")) throw new Error(`导出的文件名不对：「${md.name}」`);
      for (const want of ["# 和 Myco (Grok) 的对话", "**你** → Myco (Grok)", "把 release 分支的 Go 测试跑一遍"]) {
        if (!md.content.includes(want)) throw new Error(`导出的 Markdown 里应有「${want}」`);
      }
      // JSON Lines: one message per line, each a whole object.
      await clickVisible(page, dlg.getByRole("tab", { name: "JSONL" }));
      await clickVisible(page, page.getByRole("button", { name: "导出", exact: true }));
      await page.waitForFunction(async () => true);
      for (let i = 0; ; i++) {
        const j = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/last-export`)).json();
        if (/\.jsonl$/.test(j.name || "")) {
          const rows = j.content.trim().split("\n").map((l) => JSON.parse(l));
          if (!rows.length || !rows.every((r) => r.id && r.from && r.to && "body" in r)) throw new Error("JSONL 每行应是一条完整的消息");
          break;
        }
        if (i > 50) throw new Error("没导出 JSONL");
        await settle(page, 200);
      }
      await clickVisible(page, dlg.getByRole("button", { name: "好" }));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
    });

    // A task the hub gave up on (taken, never started on) says so in words,
    // not the hub's reason, and is sent again with one click.
    await step("单聊 · 没送到可以重新发送", async () => {
      await openMember(page, narrow, "聊天", "Myco (Grok)");
      const failed = page.locator("[data-failed]").last();
      await failed.waitFor({ timeout: 20_000 });
      const said = compactText(await failed.innerText());
      if (!said.includes("没送到") || /delivery attempts|acknowledged/.test(said)) throw new Error(`没送到的说明应该是人话，实际「${said}」`);
      if (/delivery attempts/.test(await page.locator(".conv-detail").first().innerText())) throw new Error("气泡下不该再出现英文原因");
      await settle(page, 200);
      await shot(page, size, scheme, "failed-retry");
      const before = await page.locator(".conv-msg.is-me", { hasText: "README 的截图换成新版的。" }).count();
      await clickVisible(page, failed.getByRole("button", { name: "重新发送" }));
      await page.waitForFunction((n) => [...document.querySelectorAll(".conv-msg.is-me")].filter((x) => (x.textContent || "").includes("README 的截图换成新版的。")).length > n, before, { timeout: 10_000 });
    });

    // The sidebar lists the computers too: one click opens a terminal.
    await step("电脑 · 边栏直达终端", async () => {
      await sidebar(page, true);
      // ⋯ on hover: its settings, without opening the terminal first.
      const wrap = page.locator(".sp-nav-pc-wrap").first();
      await wrap.hover();
      await clickVisible(page, wrap.locator(".sp-nav-more"));
      await clickVisible(page, page.getByRole("menuitem", { name: "谁能用、项目" }));
      const set = page.locator("[data-testid=computer-settings]");
      await set.waitFor({ timeout: 10_000 });
      await clickVisible(page, set.getByRole("button", { name: "好" }));
      await set.waitFor({ state: "hidden", timeout: 5_000 });
      // Right click: rename it in place, or go straight to removing it.
      const last = page.locator(".sp-nav-pc").last();
      const oldName = (await last.locator(".app-nav-label").innerText()).trim();
      await menuPick(page, last, "改名");
      await set.waitFor({ timeout: 10_000 });
      const nameBox = set.getByRole("textbox", { name: "名字" });
      await page.waitForFunction(() => document.activeElement?.getAttribute("aria-label") === "名字", null, { timeout: 5_000 });
      const newName = `电脑 ${size.width} ${scheme}`;
      await nameBox.fill(newName);
      await nameBox.press("Enter");
      await set.getByText("已改").waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "computer-rename");
      await clickVisible(page, set.getByRole("button", { name: "好" }));
      await page.locator(".sp-nav-pc", { hasText: newName }).waitFor({ timeout: 20_000 });
      await menuPick(page, page.locator(".sp-nav-pc", { hasText: newName }), "移除这台电脑");
      const sure = page.locator(".app-modal.app-confirm");
      await sure.waitFor({ timeout: 5_000 });
      if (await set.isVisible()) throw new Error("从右键直接移除时只该出现确认框");
      await clickVisible(page, sure.getByRole("button", { name: "取消" }));
      await sure.waitFor({ state: "hidden", timeout: 5_000 });
      // Back to its name for the steps after this one.
      await menuPick(page, page.locator(".sp-nav-pc", { hasText: newName }), "改名");
      await set.waitFor({ timeout: 10_000 });
      await nameBox.fill(oldName);
      await nameBox.press("Enter");
      await set.getByText("已改").waitFor({ timeout: 10_000 });
      await clickVisible(page, set.getByRole("button", { name: "好" }));
      await page.locator(".sp-nav-pc", { hasText: oldName }).waitFor({ timeout: 20_000 });
      await clickVisible(page, page.locator(".sp-nav-pc").first());
      if (narrow) await sidebar(page, false);
      await page.locator(".conv-detail").first().waitFor({ timeout: 20_000 });
    });

    // A command another agent left on a computer: the sidebar dot turns
    // amber (it is "working", but nothing is moving), and the terminal says
    // so above it, with a way to stop it.
    await step("电脑 · 可能卡住的命令", async () => {
      await sidebar(page, true);
      const pc = page.locator(".sp-nav-pc", { hasText: "mac-exec" });
      await pc.waitFor({ timeout: 10_000 });
      if (!(await pc.locator(".sp-nav-busy.is-stuck").count())) {
        throw new Error("mac-exec 的侧栏小点应表示可能卡住（琥珀色）");
      }
      if (await pc.locator(".sp-nav-busy:not(.is-stuck)").count()) throw new Error("mac-exec 不该同时亮绿点");
      await clickVisible(page, pc);
      if (narrow) await sidebar(page, false);
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
      const bar = page.locator(".shell-stuck");
      await bar.waitFor({ timeout: 20_000 });
      const barText = compactText(await bar.innerText());
      for (const want of ["这台电脑上有 1 条命令可能卡住", "PM (Codex)", "全部停止"]) {
        if (!barText.includes(want)) throw new Error(`终端上方的提示条应写到「${want}」，实际是「${barText}」`);
      }
      await clickVisible(page, bar.locator(".shell-stuck-toggle"));
      const row = bar.locator(".shell-stuck-row").first();
      await row.waitFor({ timeout: 10_000 });
      const rowText = compactText(await row.innerText());
      for (const want of ["make app", "小时没有动静", "停止"]) {
        if (!rowText.includes(want)) throw new Error(`展开后的命令应写到「${want}」，实际是「${rowText}」`);
      }
      await settle(page, 300);
      await shot(page, size, scheme, "computer-stuck");
      await clickVisible(page, bar.getByRole("button", { name: "全部停止" }));
      await page.waitForFunction(() => !document.querySelector(".shell-stuck"), null, { timeout: 15_000 });
      // Stopping it leaves the computer idle: no green, no amber.
      await sidebar(page, true);
      await page.waitForFunction(() => {
        const r = [...document.querySelectorAll(".sp-nav-pc")].find((e) => (e.textContent || "").includes("mac-exec"));
        return !!r && !r.querySelector(".sp-nav-busy");
      }, null, { timeout: 15_000 });
      await shot(page, size, scheme, "computer-stuck-stopped");
      if (narrow) await sidebar(page, false);
    });

    // Removing a whole computer: a dry run lists the agent and folder on it,
    // the online one shows the uninstall command, and both are gone after.
    await step("电脑 · 移除", async () => {
      await go(page, "电脑", narrow);
      await clickVisible(page, page.locator(".pc-machine", { hasText: "临时 Mac" }));
      const view = page.locator("[data-testid=machine-view]");
      await view.waitFor({ timeout: 10_000 });
      await clickVisible(page, view.getByRole("button", { name: "移除这台电脑" }));
      const dialog = page.locator(".app-modal");
      await dialog.locator(".pc-gone-row").nth(1).waitFor({ timeout: 10_000 });
      const dialogText = compactText(await dialog.innerText());
      for (const want of ["2 个 agent 和电脑会一起移除", "聊过的内容都保留", "myco runner service uninstall"]) {
        if (!dialogText.includes(want)) throw new Error(`移除电脑弹窗里应提到「${want}」，实际是「${dialogText}」`);
      }
      await settle(page, 300);
      await shot(page, size, scheme, "dialog-remove-computer");
      await clickVisible(page, dialog.getByRole("button", { name: "移除这台电脑和上面的 2 个" }));
      await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 10_000 });
      await page.waitForFunction(
        () => ![...document.querySelectorAll(".pc-machine, .pc-row")].some((r) => /临时/.test(r.textContent || "")),
        null,
        { timeout: 10_000 },
      );
    });

    // Switching workspaces (the switcher, or Ctrl/⌘ + its number) and back
    // finds you where you were: the chat you had open is open again.
    await step("工作区切换", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await page.locator(".conv-detail .conv-head h2", { hasText: "Review" }).waitFor({ timeout: 10_000 });
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await page.locator(".app-hub-menu").waitFor();
      const hubs = await page.locator(".app-hub-menu-item[role=menuitemradio]").evaluateAll((els) => els.map((e) => ({
        on: e.getAttribute("aria-checked") === "true", ok: !!e.querySelector(".dot.working"), key: e.querySelector(".app-hub-menu-key")?.textContent || "",
        name: e.querySelector(".app-hub-menu-name")?.textContent || "",
      })));
      if (hubs.some((h, i) => i < 9 && h.key !== `Ctrl+${i + 1}`)) throw new Error(`工作区菜单应标出切换快捷键，实际 ${hubs.map((h) => h.key).join(" / ")}`);
      await settle(page, 300);
      await shot(page, size, scheme, "workspace-switch");
      await page.keyboard.press("Escape");
      await page.locator(".app-hub-menu").waitFor({ state: "hidden" }).catch(() => undefined);
      if (await page.locator(".app-hub-menu").isVisible().catch(() => false)) await page.locator(".sp-switch .app-hub-trigger").click();
      const here = hubs.findIndex((h) => h.on);
      const other = hubs.findIndex((h, i) => i !== here && h.ok);
      if (here >= 0 && other >= 0) {
        await page.keyboard.press(`Control+${other + 1}`);
        await page.locator(".sp-switch .app-hub-name", { hasText: hubs[other].name }).waitFor({ timeout: 10_000 });
        await page.keyboard.press(`Control+${here + 1}`);
        await page.locator(".sp-switch .app-hub-name", { hasText: hubs[here].name }).waitFor({ timeout: 10_000 });
        if (narrow) await sidebar(page, false);
        await page.locator(".conv-detail .conv-head h2", { hasText: "Review" }).waitFor({ timeout: 10_000 });
      } else if (narrow) await sidebar(page, false);
    });

    // An agent offers the next step with its answer: the empty input shows
    // it, Tab fills it in.
    await step("单聊 · Tab 填入 agent 建议的下一步", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("改好登录页，然后告诉我下一步");
      await box.press("Enter");
      await page.locator(".conv-detail .conv-msg", { hasText: "登录页改好了" }).waitFor({ timeout: 15_000 });
      await page.waitForFunction(() => (document.querySelector(".conv-detail .conv-compose-input")?.getAttribute("placeholder") || "") === "再跑一遍完整测试", null, { timeout: 15_000 });
      await box.focus();
      await settle(page, 200);
      await shot(page, size, scheme, "compose-suggestion");
      await box.press("Tab");
      if ((await box.inputValue()) !== "再跑一遍完整测试") throw new Error(`Tab 应填入建议，实际「${await box.inputValue()}」`);
      // A long one shows on one line, cut with "…"; Tab still takes all of it.
      await box.fill("还是登录页，下一步给个长建议");
      await box.press("Enter");
      await page.waitForFunction(() => {
        const ph = document.querySelector(".conv-detail .conv-compose-input")?.getAttribute("placeholder") || "";
        return ph.startsWith("把注册页") && ph.endsWith("…");
      }, null, { timeout: 15_000 });
      const ph = await box.getAttribute("placeholder");
      if (ph.length > 60) throw new Error(`长建议应截短到一行，实际 ${ph.length} 字`);
      await box.focus();
      await box.press("Tab");
      if (!(await box.inputValue()).endsWith("再跑一遍完整测试")) throw new Error(`Tab 应填入完整的建议，实际「${await box.inputValue()}」`);
      await box.fill("");
    });

    // Voice chat with one agent, in its chat (as in ChatGPT and Claude): the
    // round button, with nothing typed, docks a bar above the box; what it passes on
    // is in the chat as a task marked 经语音 (the trailer hidden); mute and
    // hang up are on the bar. No voice button at the top any more.
    await step("语音 · 和一个 agent 语音聊天", async () => {
      if (await page.locator(".app-topbar .vc-entry, .vc-overlay").count()) throw new Error("顶栏不该再有全局语音入口");
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      await page.locator(".conv-detail .conv-compose-input").fill("");
      // Not set up: no way in; the round button is Send, as usual.
      const voiceConfig = (body) => fetch(`http://${HOST}:${mockPort()}/app/voice/config`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      await voiceConfig({ clear_tts_key: true });
      await page.evaluate(() => window.dispatchEvent(new Event("myco-voice-changed")));
      await page.locator(".conv-detail .conv-send.is-call").waitFor({ state: "detached", timeout: 5_000 });
      await page.locator(".conv-detail .conv-send").waitFor({ timeout: 5_000 });
      await voiceConfig({ tts_key: "set" });
      await page.evaluate(() => window.dispatchEvent(new Event("myco-voice-changed")));
      const voiceBtn = page.locator(".conv-detail .conv-send.is-call");
      await voiceBtn.waitFor({ timeout: 10_000 });
      await voiceBtn.hover();
      await settle(page, 200);
      await shot(page, size, scheme, "voice-button");
      await clickVisible(page, voiceBtn);
      const call = page.locator(".conv-detail .vc-bar");
      await call.waitFor({ timeout: 10_000 });
      if (!compactText(await call.innerText()).includes("和 Review")) throw new Error(`语音条应写明和谁聊，实际「${compactText(await call.innerText())}」`);
      await call.locator(".vc-line.is-me", { hasText: "帮我看一下下载页的链接对不对" }).waitFor({ timeout: 10_000 });
      await call.locator(".vc-line.is-them", { hasText: "核对完我告诉你" }).waitFor({ timeout: 10_000 });
      if (await call.locator(".vc-line.is-hearing").count() > 1) throw new Error("听到一半的字应该只有一行");
      const sent = page.locator(".conv-detail .conv-msg.is-me", { hasText: "核对下载页上的每个下载链接" }).last();
      await sent.waitFor({ timeout: 10_000 });
      await sent.locator(".conv-voice-tag").waitFor({ timeout: 5_000 });
      if (compactText(await sent.innerText()).includes("经语音转述")) throw new Error("经语音转述那行应收成一个小标记");
      // The bar lines up with the box under it; what was said starts in one
      // place whoever said it.
      const edges = await page.evaluate(() => {
        const a = document.querySelector(".conv-detail .vc-bar")?.getBoundingClientRect();
        const b = document.querySelector(".conv-detail .conv-compose-box")?.getBoundingClientRect();
        const texts = [...document.querySelectorAll(".conv-detail .vc-line")].map((l) => {
          const who = l.querySelector(".vc-line-who");
          const r = document.createRange();
          const node = [...l.childNodes].find((n) => n !== who && (n.textContent || "").trim());
          if (!node) return null;
          r.selectNodeContents(node);
          return Math.round(r.getBoundingClientRect().left);
        }).filter((x) => x !== null);
        return { dl: a && b ? Math.round(a.left - b.left) : 99, dr: a && b ? Math.round(a.right - b.right) : 99, texts };
      });
      if (Math.abs(edges.dl) > 1 || Math.abs(edges.dr) > 1) throw new Error(`语音条应和输入框左右对齐，差 ${edges.dl} / ${edges.dr}px`);
      if (new Set(edges.texts).size > 1) throw new Error(`语音条里每行说的话应从同一处开始，实际 ${edges.texts.join(" / ")}`);
      await settle(page, 300);
      await shot(page, size, scheme, "voice-chat");
      // Mute; going elsewhere does not hang up: the call is in the top bar,
      // and its name goes back to its chat.
      await clickVisible(page, call.getByRole("button", { name: "静音" }));
      await call.getByRole("button", { name: "取消静音" }).waitFor({ timeout: 5_000 });
      const voiceOnNow = async () => (await (await fetch(`http://${HOST}:${mockPort()}/app/voice/events?after=-1`)).json()).on;
      await go(page, "设置", narrow);
      const chip = page.locator(".vc-chip");
      await chip.waitFor({ timeout: 5_000 });
      if (!(await voiceOnNow())) throw new Error("切到别的页面不该挂断语音");
      if (!compactText(await chip.innerText()).includes("Review")) throw new Error(`顶栏的通话应写明和谁，实际「${compactText(await chip.innerText())}」`);
      await chip.getByRole("button", { name: "取消静音" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "voice-chip");
      await clickVisible(page, chip.locator(".vc-chip-main"));
      await page.locator(".conv-detail .vc-bar").waitFor({ timeout: 10_000 });
      await chip.waitFor({ state: "detached", timeout: 5_000 });
      // Then hang up.
      await clickVisible(page, call.getByRole("button", { name: "挂断" }));
      await call.waitFor({ state: "detached", timeout: 5_000 });
      if (await voiceOnNow()) throw new Error("挂断后语音应停下");
      // From the space's box: the chat with whoever it is addressed to opens
      // with the call on.
      await go(page, "聊天", narrow);
      if (narrow) await sidebar(page, false);
      await clickVisible(page, page.locator(".sp-send.is-call"));
      const call2 = page.locator(".conv-detail .vc-bar");
      await call2.waitFor({ timeout: 10_000 });
      await clickVisible(page, call2.getByRole("button", { name: "挂断" }));
      await call2.waitFor({ state: "detached", timeout: 5_000 });
      // Its settings.
      await go(page, "设置", narrow);
      if (narrow) await sidebar(page, true);
      await clickVisible(page, page.locator("button", { hasText: /^语音$/ }).first());
      // The speech service is one of those the backend lists, not a fixed
      // one: the heading names none, and the service is a choice.
      await page.locator(".app-setting-group", { hasText: /^语音合成$/ }).waitFor({ timeout: 10_000 });
      if (await page.locator(".app-setting-group", { hasText: "豆包" }).count()) throw new Error("语音合成的标题不该写死豆包");
      const services = page.locator(".vs-services .seg-btn");
      if ((await services.count()) < 1 || !(await services.first().innerText()).includes("豆包语音")) throw new Error("语音合成服务应来自后端的列表");
      if (await page.locator("input[type=password]").evaluateAll((els) => els.some((e) => e.value))) throw new Error("密钥不应显示出来");
      // Kept on the workspaces for the person's other devices, and it says
      // where: one hub has them, one is too old to.
      const where = page.locator("[data-testid=voice-sync]");
      await where.waitFor({ timeout: 10_000 });
      const said = await where.innerText();
      if (!said.includes("已存到") || !said.includes("版本太旧")) throw new Error(`应写明设置存到了哪些工作区，实际「${said}」`);
      // The voice is picked from the service's list (by language), and tried
      // before it is saved; one not on the list is typed in under Other.
      const pick = page.locator("[data-testid=voice-speaker]");
      if ((await pick.locator("option").count()) < 100) throw new Error("音色应是豆包的列表");
      await pick.selectOption("zh_female_vv_uranus_bigtts");
      await clickVisible(page, page.locator("[data-testid=voice-try]"));
      for (let i = 0; ; i++) {
        const t = await (await fetch(`http://${HOST}:${mockPort()}/app/voice/tried`)).json();
        if (t.speaker === "zh_female_vv_uranus_bigtts") break;
        if (i > 50) throw new Error(`试听应合成选中的音色，实际「${t.speaker}」`);
        await settle(page, 200);
      }
      await page.locator("[data-testid=voice-try]", { hasText: "试听" }).waitFor({ timeout: 5_000 });
      await pick.selectOption({ label: "其他（填音色 ID）" });
      await page.locator("input[placeholder='S_…']").waitFor({ timeout: 5_000 });
      await pick.selectOption("zh_female_vv_uranus_bigtts");
      await settle(page, 200);
      await shot(page, size, scheme, "settings-voice");
      // Turned off: nothing says where any more; and back on.
      const share = page.getByRole("tablist", { name: "把这些设置存到工作区" });
      await clickVisible(page, share.getByRole("tab", { name: "关" }));
      await where.waitFor({ state: "detached", timeout: 5_000 });
      await clickVisible(page, share.getByRole("tab", { name: "开" }));
      await where.waitFor({ timeout: 5_000 });
      if (narrow) await sidebar(page, false);
    });

    // On a hub server this App owns, the switcher adds a workspace there
    // (same address, kept apart) and switches to it; its add-computer dialog
    // offers the server's other computers, one click each.
    await step("工作区 · 在服务器上再开一个", async () => {
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await page.locator(".app-hub-menu").waitFor();
      const entry = page.locator(".app-hub-menu-item", { hasText: "上新建工作区" }).filter({ hasNotText: "这台电脑" });
      await entry.waitFor({ timeout: 10_000 });
      await clickVisible(page, entry);
      const dialog = page.locator(".app-modal");
      await dialog.waitFor({ timeout: 10_000 });
      const title = compactText(await dialog.locator("h3").first().innerText());
      if (!title.includes("192.168.1.20")) throw new Error(`应说明建在哪台服务器上，实际「${title}」`);
      await dialog.locator(".app-field .app-input").fill("team-b");
      await settle(page, 200);
      await shot(page, size, scheme, "dialog-new-workspace-server");
      await clickVisible(page, dialog.getByRole("button", { name: "新建并切换" }));
      await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 30_000 });
      await page.locator(".sp-switch .app-hub-name", { hasText: "team-b" }).waitFor({ timeout: 20_000 });
      // Adding a computer to the new workspace, outside any project: the
      // same one command; the folder it is run in is in the sidebar after.
      await go(page, "电脑", narrow);
      await clickVisible(page, page.locator(".pc-head").getByRole("button", { name: "加一台电脑" }));
      const pick = page.locator(".app-modal-add");
      await pick.waitFor({ timeout: 10_000 });
      if (!(await pick.locator("h3").first().innerText()).includes("加一台电脑")) throw new Error("工作区里加电脑不该说「给项目」");
      const box = pick.locator("[data-testid=computer-command]");
      await box.locator(".app-command code", { hasText: "curl -fsSL" }).waitFor({ timeout: 10_000 });
      const ran = await fetch(`http://${HOST}:${mockPort()}/app/mock/run-join`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ dir: "~/code/team-b" }) });
      if (!ran.ok) throw new Error(`mock run-join: ${ran.status}`);
      await box.locator("[data-testid=computer-arrived]").waitFor({ timeout: 20_000 });
      await clickVisible(page, box.getByRole("button", { name: "完成" }));
      await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 10_000 });
      // It is a computer of the workspace now: in the sidebar, and on the page.
      if (narrow) await sidebar(page, true);
      await page.locator(".sp-nav-pc", { hasText: "team-b" }).waitFor({ timeout: 20_000 });
      if (narrow) await sidebar(page, false);
      await page.locator(".pc-row", { hasText: "team-b" }).waitFor({ timeout: 20_000 });
      if (narrow) await sidebar(page, true);
      await settle(page, 300);
      await shot(page, size, scheme, "workspace-computer-added");
      if (narrow) await sidebar(page, false);
    });

    // A computer can run more than one workspace: the switcher starts
    // another by name, tags the ones on this computer, and Settings lists
    // them under one heading.
    await step("工作区 · 这台电脑上再开一个", async () => {
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await page.locator(".app-hub-menu").waitFor();
      await clickVisible(page, page.locator(".app-hub-menu-item", { hasText: "在这台电脑上新建工作区" }));
      const dialog = page.locator(".app-modal");
      await dialog.waitFor({ timeout: 10_000 });
      const field = dialog.locator(".app-field .app-input");
      await field.fill("Studio");
      // The internal name follows what was typed: latin names become a slug.
      const note = compactText(await dialog.locator(".app-field .app-small-note").innerText());
      if (!/内部名[:：]\s*studio\b(?!-)/.test(note)) throw new Error(`内部名应按工作区名生成，实际「${note}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "dialog-new-workspace");
      await clickVisible(page, dialog.getByRole("button", { name: "新建并切换" }));
      await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 30_000 });
      // It switches over: the switcher at the bottom shows the new name.
      await page.locator(".sp-switch .app-hub-name", { hasText: "Studio" }).waitFor({ timeout: 20_000 });
      // Both workspaces this computer runs carry the tag; the other one does not.
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await page.locator(".app-hub-menu").waitFor();
      await page.waitForFunction(() => {
        const rows = [...document.querySelectorAll(".app-hub-menu-item")];
        return rows.filter((r) => r.querySelector(".app-hub-menu-tag")).length === 2;
      }, null, { timeout: 10_000 });
      const tagged = await page.locator(".app-hub-menu-item", { has: page.locator(".app-hub-menu-tag") }).allInnerTexts();
      for (const row of tagged.map(compactText)) {
        if (!row.includes("这台电脑")) throw new Error(`这台电脑上的工作区应带标签，实际「${row}」`);
      }
      await settle(page, 200);
      await shot(page, size, scheme, "workspace-local-tag");
      await page.keyboard.press("Escape");
      await page.locator(".app-hub-menu").waitFor({ state: "hidden" }).catch(() => undefined);
      // Settings lists every workspace this computer runs, one card each,
      // the stopped "notes" too.
      await go(page, "设置", narrow);
      await page.locator("[data-local-ws]").first().waitFor({ timeout: 20_000 });
      // (they come in as each answers: give the last one a moment)
      await page.waitForFunction(() => document.querySelectorAll("[data-local-ws]").length >= 3, null, { timeout: 5_000 }).catch(() => undefined);
      const localCount = await page.locator("[data-local-ws]").count();
      if (localCount !== 3) throw new Error(`设置页应列出这台电脑上的三个工作区（含停着的 notes），实际 ${localCount} 个`);
      const heading = compactText(await page.locator(".app-setting-group").first().innerText());
      if (!heading.includes("这台电脑上的工作区")) throw new Error(`设置页应有「这台电脑上的工作区」，实际「${heading}」`);
      await settle(page, 300);
      await shot(page, size, scheme, "settings-local-workspaces");
      if (narrow) await sidebar(page, false);
    });

    await step("新建项目弹窗", async () => {
      await sidebar(page, true);
      await clickVisible(page, page.locator(".app-nav-add"));
      await page.locator(".app-modal .np-input").waitFor();
      await settle(page, 300);
      await shot(page, size, scheme, "dialog-new-project");
      await closeDialog(page);
      if (narrow) await sidebar(page, false);
    });

    // The Mac's menu bar asks the page for what it offers (a "myco-menu"
    // event): search, then settings.
    await step("菜单栏 · 搜索、设置", async () => {
      await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-menu", { detail: "search" })));
      await page.locator(".cp").waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await page.locator(".cp").waitFor({ state: "detached", timeout: 5_000 });
      await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-menu", { detail: "settings" })));
      await page.locator(".app-settings-tab").first().waitFor({ timeout: 5_000 });
    });

    await step("设置", async () => {
      await go(page, "设置", narrow);
      await page.locator(".app-settings").waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "settings");
      // No hub of your own yet: a server's command, or a sentence for your agent.
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "工作区" }).first());
      const add = page.getByRole("button", { name: "添加工作区" });
      await add.first().waitFor({ timeout: 10_000 });
      {
        await clickVisible(page, add.first());
        const dlg = page.locator(".app-modal");
        await clickVisible(page, dlg.getByRole("button", { name: "还没有链接？" }));
        const way = dlg.locator("[data-testid=connect-agent-way] .add-sentence-text");
        await way.waitFor({ timeout: 5_000 });
        if (!(await way.innerText()).includes("agent-setup.md")) throw new Error("添加工作区里应有让 agent 帮你装的那句话");
        await settle(page, 200);
        await shot(page, size, scheme, "connect-agent");
        await page.keyboard.press("Escape");
        await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      }
    });
  } finally {
    await context.close().catch(() => undefined);
  }
}

/** Builds the App into a folder of its own (not internal/web/dist). */
async function buildApp(viteBin) {
  const dir = path.join(OUT, ".app");
  const build = start("vite build", [viteBin, "build", "--outDir", dir, "--emptyOutDir", "--logLevel", "warn"], {});
  const code = await new Promise((resolve) => build.on("exit", resolve));
  if (code !== 0) throw new Error(`Building the App failed (exit ${code}).\n${build.output()}`);
  return dir;
}

const TYPES = { ".html": "text/html; charset=utf-8", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml",
  ".png": "image/png", ".woff2": "font/woff2", ".json": "application/json", ".ico": "image/x-icon" };

/** Serves the built App on port, and /app and /api from that run's mock. */
function serveApp(dir, port, mock) {
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url || "/", `http://${HOST}`);
    if (url.pathname.startsWith("/app/") || url.pathname.startsWith("/api/")) {
      const up = http.request({ host: HOST, port: mock, path: req.url, method: req.method, headers: req.headers }, (r) => {
        res.writeHead(r.statusCode || 502, r.headers);
        r.pipe(res);
      });
      up.on("error", () => { if (!res.headersSent) res.writeHead(502); res.end(); });
      req.pipe(up);
      return;
    }
    const file = path.join(dir, path.normalize(url.pathname === "/" ? "/index.html" : url.pathname));
    if (!file.startsWith(dir)) { res.writeHead(403); res.end(); return; }
    try {
      const data = await readFile(file);
      res.writeHead(200, { "Content-Type": TYPES[path.extname(file)] || "application/octet-stream" });
      res.end(data);
    } catch {
      // Like Vite's dev server: any other path is the App (a missing
      // built file is still a 404).
      if (url.pathname.startsWith("/assets/")) { res.writeHead(404); res.end(); return; }
      res.writeHead(200, { "Content-Type": TYPES[".html"] });
      res.end(await readFile(path.join(dir, "index.html")));
    }
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, HOST, () => resolve(server));
  });
}

function seconds(ms) {
  return ms >= 60_000 ? `${Math.floor(ms / 60_000)}m${Math.round((ms % 60_000) / 1000)}s` : `${(ms / 1000).toFixed(1)}s`;
}

async function main() {
  const t0 = Date.now();
  const combos = VIEWPORTS.flatMap((size) => SCHEMES.map((scheme) => ({ size, scheme, key: `${size.width}-${scheme}` })))
    .filter((c) => !COMBOS.length || COMBOS.some((x) => c.key === x || `${c.size.width}x${c.size.height}-${c.scheme}` === x));
  if (!combos.length) throw new Error(`--combo matches none of: ${VIEWPORTS.flatMap((v) => SCHEMES.map((s) => `${v.width}-${s}`)).join(", ")}`);
  log("myco App · 界面冒烟测试");
  log(`  ${combos.length} 个窗口组合${SERIAL || combos.length === 1 ? "，一个接一个" : "，同时跑"}；mock 后端从 ${MOCK_PORT}、App 从 ${APP_PORT} 起各用一个端口`);
  if (ONLY.length) log(`  只跑名字里有「${ONLY.join("」或「")}」的步骤`);

  for (let i = 0; i < combos.length; i++) {
    await requirePort(MOCK_PORT + i, "a mock backend");
    await requirePort(APP_PORT + i, "the App");
  }

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
  const servers = [];
  const runs = [];
  try {
    const tb = Date.now();
    const dir = await buildApp(viteBin);
    log(`  构建 App ${seconds(Date.now() - tb)}`);
    for (let i = 0; i < combos.length; i++) {
      const mock = start(`mock 后端 ${combos[i].key}`, [mockEntry], { APP_MOCK_PORT: String(MOCK_PORT + i) });
      await waitForHttp(`http://${HOST}:${MOCK_PORT + i}/app/hubs`, mock);
      servers.push(await serveApp(dir, APP_PORT + i, MOCK_PORT + i));
    }

    const go = (c, i) => {
      const r = {
        matrix: `${c.size.width}×${c.size.height} ${c.scheme}`, step: "启动", lines: [], timings: [],
        mockPort: MOCK_PORT + i, base: `http://${HOST}:${APP_PORT + i}`, ok: false, error: null, ms: 0,
      };
      runs.push(r);
      const t = Date.now();
      return runCtx.run(r, () => runMatrix(browser, c.size, c.scheme))
        .then(() => { r.ok = true; }, (err) => { r.error = err; })
        .finally(() => {
          r.ms = Date.now() - t;
          // Its lines, together, as soon as it ends.
          console.log(r.lines.join("\n"));
          console.log(r.ok ? `  ✓ ${r.matrix} ${seconds(r.ms)}` : `  ✖ ${r.matrix} 失败于「${r.step}」`);
        });
    };
    if (SERIAL) {
      for (let i = 0; i < combos.length; i++) await go(combos[i], i);
    } else {
      await Promise.all(combos.map(go));
    }
  } finally {
    await browser.close().catch(() => undefined);
    for (const s of servers) s.close();
    stopAll();
  }

  const failed = runs.filter((r) => !r.ok);
  // The slowest steps, to know where the time goes.
  const slow = runs.flatMap((r) => r.timings.map((t) => ({ ...t, matrix: r.matrix }))).sort((a, b) => b.ms - a.ms).slice(0, 6);
  if (slow.length) {
    log("\n最慢的步骤：");
    for (const t of slow) log(`  ${seconds(t.ms).padStart(6)}  ${t.label}（${t.matrix}）`);
  }
  log(`\n${runs.length} 个窗口组合用了 ${seconds(Date.now() - t0)}。`);
  if (failed.length) {
    for (const r of failed) console.error(`\n✖ 冒烟测试失败于「${r.matrix} · ${r.step}」：\n${r.error?.stack || r.error?.message || r.error}`);
    if (problems.length > 0) reportProblems();
    process.exitCode = 1;
    return;
  }
  log("全部页面截图完成。");
  reportProblems();
}

main().catch((err) => {
  console.error(`\n✖ 冒烟测试没能开始：\n${err.stack || err.message}`);
  if (problems.length > 0) reportProblems();
  stopAll();
  process.exitCode = 1;
});