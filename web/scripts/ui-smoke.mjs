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
//   npm run test:ui -- --shard 2/3            # the second third of the steps (CI runs the parts in parallel)
//   npm run test:ui -- --write-weights        # after a full run, save each step's time for balancing the parts
//
// A step that fails is tried once more after reloading the App: if it passes
// then, it was flaky (listed in ui-smoke/flakes.json and the CI job summary)
// and the run still passes; failing twice fails the run.
//
// Environment:
//   UI_SMOKE_PORT        first port of the App servers (default 5173; one each)
//   UI_SMOKE_MOCK_PORT   first port of the mock backends (default 7781)
//   UI_SMOKE_OUT         where screenshots and the built App go (default web/ui-smoke)
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
import ts from "typescript";
import { appsPath } from "../apps-paths.mjs";
import { AsyncLocalStorage } from "node:async_hooks";
import { mkdir, access, readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const pageRouteScript = ts.transpileModule(await readFile(appsPath("templates", "src", "_lib", "page-route.ts"),"utf8"),{compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2022}}).outputText.replace(/^export /gm,"") + "\nwindow.__routeModule={subscribeAppRoute};";
const WEB = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
// UI_SMOKE_OUT: where screenshots and the built App go (default web/ui-smoke;
// give each run its own when several run on one machine).
const OUT = process.env.UI_SMOKE_OUT ? path.resolve(process.env.UI_SMOKE_OUT) : path.join(WEB, "ui-smoke");
const HOST = "127.0.0.1";
// A 1x1 PNG, for choosing a picture as a face.
const TINY_PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";
const APP_PORT = Number(process.env.UI_SMOKE_PORT || 5173);
const MOCK_PORT = Number(process.env.UI_SMOKE_MOCK_PORT || 7781);
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
// --shard i/n: the i-th of n consecutive parts of each window's steps (steps
// build on the ones before them, so a part keeps its run of them in order).
const SHARD = (() => {
  const v = flag("shard")[0];
  if (!v) return null;
  const m = /^(\d+)\/(\d+)$/.exec(v);
  if (!m || +m[1] < 1 || +m[1] > +m[2]) throw new Error(`--shard wants i/n (1 ≤ i ≤ n), got ${v}`);
  return { i: +m[1], n: +m[2] };
})();
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

const intentionalPublicNotReady = new WeakSet();
const intentionalPageOriginBusy = new WeakSet();
// Pages whose app library is made to fail on purpose (出错 · 显示请求编号).
const intentionalTemplatesFail = new WeakSet();

function watchPage(page, size, scheme) {
  // Page events come outside this run's async context: keep it here.
  const r = run();
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const loc = msg.location();
    const where = loc.url ? `${loc.url}:${loc.lineNumber || 0}:${loc.columnNumber || 0}` : "";
    // On purpose: Claude on this computer cannot sign in until a way is
    // picked (添加 agent: the sign-in card, then it starts).
    if (msg.text().includes("502") && where.includes("/machines/m_studio/spawn")) return;
    // On purpose: 设置 · 模型供应商 tests a wrong key (the vendor says 401).
    if (msg.text().includes("502") && /\/providers\/test$/.test(where.replace(/:\d+:\d+$/, ""))) return;
    // On purpose: 单聊 · 网络不好 cuts the chat's polls off.
    if (msg.text().includes("ERR_FAILED") && /\/(exchanges|thread)\b/.test(where)) return;
    // On purpose: a page asks before it uses this computer (网页 · 用电脑前先问:
    // the App backend answers consent_required, a 403, and the App asks).
    if (msg.text().includes("403") && /\/app\/os\/call\b/.test(where)) return;
    // This fixture deliberately returns the real backend's not-ready status.
    if (intentionalPublicNotReady.has(page) && msg.text().includes("503") && /\/app\/local\/hub:/.test(where)) return;
    // This fixture deliberately keeps the saved page origin occupied.
    if (intentionalTemplatesFail.has(page) && msg.text().includes("500") && /\/app\/hubs\/[^/]+\/templates:/.test(where)) return;
    if (intentionalPageOriginBusy.has(page) && msg.text().includes("503") && /\/app\/hubs\/[^/]+\/pages\/pg_release\/open:/.test(where)) return;
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

// The steps in this file, in order, and the ones that start by going to a
// page themselves (a part may start there: steps build on the ones before
// them). Part k ends at the first such step at or after k/n of the steps,
// so the parts keep their runs of steps in order.
const SOURCE = await readFile(fileURLToPath(import.meta.url), "utf8");
const STEP_LABELS = [...SOURCE.matchAll(/await step\("([^"]+)"/g)].map((m) => m[1]);
const STARTS_ITSELF = new Set([...SOURCE.matchAll(/await step\("([^"]+)", async \(\) => \{\n\s*await (?:go|page\.goto|openMember)\(/g)].map((m) => m[1]));
const STEPS = STEP_LABELS.length;
// How long each step took (scripts/ui-smoke-weights.json, seconds, from a
// run's ui-smoke/timings.json; refresh with --write-weights): the parts are
// cut so that each gets about the same time. Steps not listed count 3 s.
const WEIGHTS = await readFile(path.join(path.dirname(fileURLToPath(import.meta.url)), "ui-smoke-weights.json"), "utf8")
  .then((t) => JSON.parse(t)).catch(() => ({}));
function shardBounds() {
  const w = STEP_LABELS.map((l) => WEIGHTS[l] ?? 3);
  const before = [0];
  for (const x of w) before.push(before[before.length - 1] + x);
  const total = before[STEPS];
  const cuts = [0];
  for (let k = 1; k < SHARD.n; k++) {
    // The step that starts by itself nearest to k/n of the time.
    const want = (k * total) / SHARD.n;
    let best = -1;
    for (let at = cuts[k - 1] + 1; at < STEPS; at++) {
      if (!STARTS_ITSELF.has(STEP_LABELS[at])) continue;
      if (best < 0 || Math.abs(before[at] - want) < Math.abs(before[best] - want)) best = at;
    }
    cuts.push(best < 0 ? STEPS : best);
  }
  cuts.push(STEPS);
  return { from: cuts[SHARD.i - 1], to: cuts[SHARD.i] };
}
function shardRange() { return shardBounds(); }
function inShard(label) {
  const at = STEP_LABELS.indexOf(label);
  if (at < 0) return SHARD.i === 1; // not found in this file: run it once, in the first part
  const { from, to } = shardBounds();
  return at >= from && at < to;
}

// Steps that passed only on their second try: ui-smoke/flakes.json, and in CI
// a table in the job summary (they do not fail the run).
async function reportFlakes(flakes) {
  const { writeFile, appendFile } = await import("node:fs/promises");
  await writeFile(path.join(OUT, "flakes.json"), JSON.stringify(flakes, null, 2) + "\n").catch(() => undefined);
  if (!flakes.length) return;
  log(`\n⚠ ${flakes.length} 个步骤第二次才通过（偶发，不算失败）：`);
  for (const f of flakes) log(`  ${f.matrix} · ${f.step}：${f.error}`);
  const summary = process.env.GITHUB_STEP_SUMMARY;
  if (summary) {
    const cell = (t) => String(t).replace(/\|/g, "\\|").replace(/\n/g, " ");
    const rows = flakes.map((f) => `| ${cell(f.matrix)} | ${cell(f.step)} | ${cell(f.error)} |`).join("\n");
    await appendFile(summary, `### UI smoke: flaky steps (passed on the second try)\n\n| Window | Step | First error |\n|---|---|---|\n${rows}\n`).catch(() => undefined);
  }
}

async function shot(page, size, scheme, slug) {
  const name = `${size.width}x${size.height}-${scheme}-${slug}.png`;
  await page.screenshot({ path: path.join(OUT, name) });
  log(`    截图 ui-smoke/${name}`);
}

async function sidebar(page, open) {
  // (under the Settings dialog nothing is clickable: it goes first)
  await closeSettings(page);
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
async function sendFromElsewhere(hubId, agent, body, extra = {}) {
  const r = await fetch(`http://${HOST}:${mockPort()}/app/hubs/${hubId}/messages`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ to: agent, body, ...extra }),
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

/** Opens every run's process in the chat: answered ones fold to one line,
 *  and a long one keeps its earlier parts behind a line. */
async function revealRuns(page) {
  for (const b of await page.locator(".conv-detail .trace-fold").all()) {
    if ((await b.getAttribute("aria-expanded")) === "false") await b.click();
  }
  for (const b of await page.locator(".conv-detail .trace-more").all()) {
    if (await b.isVisible().catch(() => false)) await b.click();
  }
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
  // Settings is a dialog over the page: closed first (as Esc does).
  await closeSettings(page);
  await sidebar(page, true);
  if (label === "用量") {
    // Usage sits in the workspace menu too, not at the top of the sidebar;
    // the menu shows all its items without scrolling.
    await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
    const menu = page.locator(".app-hub-menu");
    await menu.waitFor({ timeout: 5_000 });
    if (await menu.evaluate((el) => el.scrollHeight > el.clientHeight + 1)) throw new Error("工作区菜单不该要滚动");
    if ((await menu.innerText()).includes("使用文档")) throw new Error("使用文档在设置里，不在工作区菜单");
    await clickVisible(page, page.locator("[data-testid=menu-usage]"));
    await page.locator(".usage-page").waitFor({ timeout: 10_000 });
    if (narrow) await sidebar(page, false);
    return;
  }
  if (label === "设置") {
    // Settings sit in the workspace menu at the bottom of the sidebar.
    await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
    await clickVisible(page, page.locator(".app-hub-menu-item", { hasText: "设置" }));
    await page.locator("[data-testid=settings-dialog]").waitFor({ timeout: 10_000 });
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
      : label === "工作流" ? page.locator(".sp-nav-workflows:visible")
      : label === "应用" ? page.locator(".sp-nav-templates:visible")
      : page.locator(".app-nav-item:visible", { hasText: label });
    await clickVisible(page, item);
  }
  // At narrow widths the sidebar covers the window; show the page as a person
  // would after picking it, with the compact Hub pill in the crumb bar.
  if (narrow) await sidebar(page, false);
}

// The ‹ at the top (back to where you were: nav.ts).
async function navBackClick(page) {
  await clickVisible(page, page.locator("[data-testid=nav-arrows]").getByRole("button", { name: "后退" }));
}

async function closeSettings(page) {
  const dlg = page.locator("[data-testid=settings-dialog]");
  if (!(await dlg.count())) return;
  await page.locator(".set-close").click();
  await dlg.waitFor({ state: "detached", timeout: 5_000 });
}

// Right-click a row and pick from its menu. Scrolling the row into view
// fires a scroll a moment later, which closes a menu (as it should for a
// person scrolling): so again, until the menu stays.
// A computer's page: one of its tabs (Agent 和终端, 权限, AI 和工具, 检查和修复).
async function pcTab(page, name) {
  await clickVisible(page, page.locator("[data-testid=pc-tabs]").getByRole("tab", { name: new RegExp(`^${name}`) }));
}
// A computer's page: an item of its ⋯ (改名, 移除).
async function pcMore(page, item) {
  await clickVisible(page, page.locator("[data-testid=computer-more]"));
  await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: item, exact: true }));
}

async function menuPick(page, row, item) {
  for (let i = 0; ; i++) {
    await row.scrollIntoViewIfNeeded();
    await page.waitForTimeout(300);
    await row.click({ button: "right" });
    const it = page.locator(".conv-menu").getByRole("menuitem", typeof item === "string" ? { name: item, exact: true } : { name: item });
    if (await it.waitFor({ timeout: 1_500 }).then(() => true, () => false)
      && await it.click({ timeout: 1_500 }).then(() => true, () => false)) return;
    await page.keyboard.press("Escape");
    if (i >= 4) throw new Error(`右键菜单没出现「${item}」`);
  }
}

/** A member's menu → 移到分组 → the group (a short list in the menu's place). */
async function moveTo(page, row, group) {
  await menuPick(page, row, /^移到分组/);
  const it = page.locator(".conv-menu").getByRole("menuitem", { name: group, exact: true });
  await it.click({ timeout: 5_000 });
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
  // The superagent step sets up a Shroom; none again for the next run.
  const resetSuper = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, { method: "DELETE" });
  if (!resetSuper.ok) throw new Error(`重置 superagent 失败：HTTP ${resetSuper.status}`);
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
  let stepNo = 0;
  const step = async (label, fn) => {
    // --only: just the steps named; the others are left out.
    if (ONLY.length && !ONLY.some((x) => label.includes(x))) return;
    // --shard i/n: this window's share of the steps.
    const no = stepNo++;
    if (SHARD && !inShard(label)) return;
    r.step = label;
    const t0 = Date.now();
    log(`  · ${label}`);
    try {
      await fn();
    } catch (first) {
      // Once more from a freshly loaded App: a pass then is a flake, noted
      // but not failing the run; a second failure is the real thing.
      const where = String(first?.stack || "").split("\n").find((l) => l.includes("ui-smoke.mjs:"))?.trim().replace(/^at\s+/, "") || "";
      log(`    ⚠ 第一次失败，重新加载后再试一次：${String(first?.message || first).split("\n")[0]}${where ? `（${where.replace(/^.*\//, "")}）` : ""}`);
      await page.screenshot({ path: path.join(OUT, `flaky-${size.width}x${size.height}-${scheme}-${no}.png`) }).catch(() => undefined);
      await page.keyboard.press("Escape").catch(() => undefined);
      await page.goto(`${r.base}/?lang=zh`, { waitUntil: "domcontentloaded" });
      await page.locator(".app-shell").waitFor();
      await settle(page);
      await fn();
      r.flakes.push({ matrix: r.matrix, step: label, error: String(first?.message || first).split("\n")[0], where: where.replace(/^.*\//, "") });
      log(`    ✓ 第二次通过（记为偶发）`);
    }
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
      // The chat's own ⋯ (background, goal, files, find…) is in the
      // project's header, as in any chat.
      await page.locator(".sp-home [data-testid=home-more]").click();
      for (const item of ["文件和应用", "聊天背景", "在这段对话里找", "导出这段对话"]) {
        await page.locator(".conv-menu").getByRole("menuitem", { name: item }).waitFor({ timeout: 5_000 });
      }
      await settle(page, 200);
      await shot(page, size, scheme, "project-home-more");
      await page.keyboard.press("Escape");
      await page.locator(".conv-menu").waitFor({ state: "hidden", timeout: 5_000 });
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
      // A card right after your own message keeps clear of the time and
      // buttons that float under your bubble while the pointer is on it.
      // (A card as it would come there: a copy of one, put after your last
      // message, then taken out again.)
      const placed = await page.evaluate(() => {
        const tl = document.querySelector(".sp-home .conv-timeline");
        // (one whose time floats: shown only with the pointer on it)
        const me = [...(tl?.querySelectorAll(":scope > .conv-msg.is-me") || [])].filter((m) => m.querySelector(":scope > .conv-msg-meta:not(.is-shown)")).pop();
        const card = tl?.querySelector(":scope > .handoff");
        if (!me || !card) return false;
        const copy = card.cloneNode(true);
        copy.setAttribute("data-smoke-copy", "1");
        me.after(copy);
        me.setAttribute("data-smoke-me", "1");
        return true;
      });
      if (placed) {
        try {
          const me = page.locator(".sp-home [data-smoke-me] .conv-bubble").first();
          await me.scrollIntoViewIfNeeded();
          await me.hover();
          await settle(page, 250);
          const clear = await page.evaluate(() => {
            const meta = document.querySelector(".sp-home [data-smoke-me] .conv-msg-meta");
            const card = document.querySelector(".sp-home [data-smoke-copy]");
            return meta && card ? Math.round(card.getBoundingClientRect().top - meta.getBoundingClientRect().bottom) : null;
          });
          if (clear === null || clear < 6) throw new Error(`你的消息后面紧跟卡片时，时间和按钮下面应留出空隙，实际只有 ${clear}px`);
        } finally {
          await page.evaluate(() => {
            document.querySelector(".sp-home [data-smoke-copy]")?.remove();
            document.querySelector(".sp-home [data-smoke-me]")?.removeAttribute("data-smoke-me");
          });
          await page.mouse.move(0, 0);
        }
      }
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
      const pill = page.locator(".sp-home .conv-compose .run-pill:not([data-testid=access-pill])");
      await pill.waitFor({ timeout: 10_000 });
      const text = compactText(await pill.innerText());
      if (!/GPT-6 Sol/.test(text) || !/高/.test(text)) throw new Error(`输入框旁应写着模型和推理强度，实际「${text}」`);
      await pill.click();
      const menu = page.locator(".sp-home .run-pill-menu");
      await menu.waitFor({ timeout: 5_000 });
      // The models its computer's Codex lists, by their names.
      await menu.getByRole("menuitemradio", { name: /GPT-6\.1 Sol/ }).waitFor({ timeout: 5_000 });
      if (await menu.getByRole("menuitemradio", { name: /O3/ }).count()) throw new Error("电脑报了它的模型列表，就不该再列内置的旧模型");
      await settle(page, 150);
      await shot(page, size, scheme, "composer-model");
      await menu.getByRole("menuitemradio", { name: "中", exact: true }).click();
      await page.waitForFunction(() => /中/.test(document.querySelector(".sp-home .conv-compose .run-pill:not([data-testid=access-pill])")?.textContent || ""), null, { timeout: 10_000 });
      // (back as it was: the other windows share the mock)
      await pill.click();
      await page.locator(".sp-home .run-pill-menu").getByRole("menuitemradio", { name: "高", exact: true }).click();
      await page.waitForFunction(() => /高/.test(document.querySelector(".sp-home .conv-compose .run-pill:not([data-testid=access-pill])")?.textContent || ""), null, { timeout: 10_000 });
      // Its access beside it (what it may touch), switched there by its owner.
      const access = page.locator(".sp-home .conv-compose [data-testid=access-pill]");
      await access.waitFor({ timeout: 10_000 });
      if (compactText(await access.innerText()) !== "工作区权限") throw new Error(`输入框旁应写着权限，实际「${compactText(await access.innerText())}」`);
      await access.click();
      const accessMenu = page.locator(".sp-home .run-pill-menu");
      await accessMenu.getByRole("menuitemradio", { name: /^只读/ }).waitFor({ timeout: 5_000 });
      await settle(page, 150);
      await shot(page, size, scheme, "composer-access");
      await accessMenu.getByRole("menuitemradio", { name: /^只读/ }).click();
      await page.waitForFunction(() => /只读/.test(document.querySelector(".sp-home [data-testid=access-pill]")?.textContent || ""), null, { timeout: 10_000 });
      await access.click();
      await page.locator(".sp-home .run-pill-menu").getByRole("menuitemradio", { name: /^跟随电脑设置/ }).click();
      await page.waitForFunction(() => /工作区权限/.test(document.querySelector(".sp-home [data-testid=access-pill]")?.textContent || ""), null, { timeout: 10_000 });
      // In its own session and not said yet: 模型未知, and how it comes to be known.
      if (narrow) await clickVisible(page, page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" }));
      const chat = page.locator(".sp-pane-right");
      const unknown = chat.locator(".conv-compose .run-pill:not([data-testid=access-pill])");
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
      // The loading scene (a hopping mushroom) is gone once it has loaded.
      await page.locator("[data-testid=page-loading]").waitFor({ state: "detached", timeout: 10_000 });
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
        return r.urls.includes("https://github.com/myco-ai/myco/releases");
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

    await step("网页 · 应用内直达链接", async () => {
      await go(page,"官网改版",narrow);
      if (narrow) await clickVisible(page,page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page,page.locator(".sp-side .sp-member:visible",{hasText:"Release (Claude 云端)"}));
      const chat = page.locator(".sp-pane-right"), panel = page.locator(".pg-panel");
      const first = chat.locator(".conv-msg", {hasText:"直达链接一"}).first();
      const second = chat.locator(".conv-msg", {hasText:"直达链接二"}).first();
      // A route card must retain the link, rather than opening only the page ID.
      const navigation = fragment => page.waitForEvent("framenavigated",{predicate:f=>f.url().includes("#watch") && f.url().includes(fragment),timeout:15000});
      const firstNavigation = navigation("%E7%AC%AC01%E9%9B%86");
      await clickVisible(page, first.locator(".pg-card"));
      await firstNavigation;
      const frame = () => page.frames().find(f => f.url().includes("#watch"));
      await page.waitForFunction(() => document.querySelector(".pg-frame")?.getAttribute("src")?.includes("%E7%AC%AC01%E9%9B%86"));
      await frame().waitForFunction(() => !!window.myco);
      const ctx = await frame().evaluate(() => window.myco.ready);
      if (ctx.page.url !== "http://192.168.1.20:7780/pages/pg_routes") throw Error("ready must expose the public page URL, not the private frame URL");
      // The normal Markdown link follows exactly the same route path.
      await frame().evaluate(() => { window.__routeMarker = "kept"; });
      if (narrow) await clickVisible(page,panel.getByRole("button",{name:"收起",exact:true}));
      const secondNavigation = navigation("%E7%AC%AC02%E9%9B%86");
      await clickVisible(page,second.locator('a[href*="#watch"]'));
      await secondNavigation;
      await page.waitForFunction(() => document.querySelector(".pg-frame")?.getAttribute("src")?.includes("%E7%AC%AC02%E9%9B%86"));
      if (await panel.locator(".pg-tab").count()) throw Error("two routes to one page must reuse one tab");
      await frame().waitForFunction(() => !!window.myco);
      if (await frame().evaluate(() => window.__routeMarker) !== "kept") throw Error("changing the fragment must preserve the page document");
      await frame().addScriptTag({content:pageRouteScript});
      await frame().evaluate(() => { window.__routeCount=0; window.__stopRoute=window.__routeModule.subscribeAppRoute(()=>window.__routeCount++); });
      if (narrow) await clickVisible(page,panel.getByRole("button",{name:"收起",exact:true}));
      await clickVisible(page, second.locator(".pg-card"));
      await frame().waitForFunction(()=>window.__routeCount===1);
      if (await frame().evaluate(() => window.__routeMarker) !== "kept") throw Error("repeated deep links must keep the page document");
      // Returning through an ordinary page link clears the route, in place.
      if (narrow) await clickVisible(page,panel.getByRole("button",{name:"收起",exact:true}));
      await clickVisible(page,chat.locator(".conv-msg",{hasText:"直达普通入口"}).first().locator(".pg-card"));
      const sameFrame=page.frames().find(f=>f.url().includes("/p/"));
      await sameFrame.waitForFunction(()=>location.hash==="");
      if (await sameFrame.evaluate(()=>window.__routeMarker)!=="kept") throw Error("a plain link back to the page must keep its document");
      await shot(page,size,scheme,"page-route");
      await clickVisible(page, panel.getByRole("button",{name:"关闭",exact:true}));
      await panel.waitFor({state:"detached"});
      await clickVisible(page,chat.getByRole("button",{name:"关掉这一栏"}));
    });

    await step("网页 · 另一个 App 正在打开", async () => {
      const zh = scheme === "dark";
      await page.goto(`${r.base}/?lang=${zh ? "zh" : "en"}`);
      await page.locator(".app-shell").waitFor();
      const routeURL = "**/app/hubs/*/pages/pg_release/open";
      let busy = true;
      intentionalPageOriginBusy.add(page);
      await page.route(routeURL, route => busy
        // An older backend's wording must also receive the localized guidance.
        ? route.fulfill({ status: 503, json: { error: "page_origin_unavailable", message: "the page's local storage address is unavailable; close the other app using it and retry" } })
        : route.continue());
      const panel = page.locator(".pg-panel");
      const openPage = () => page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-open-page", {
        detail: { kind: "page", hubId: "hub_studio", id: "pg_release" },
      })));
      const closePage = async () => {
        await panel.getByRole("button", { name: zh ? "关闭" : "Close", exact: true }).click();
        await panel.waitFor({ state: "detached" });
      };
      await openPage();
      await panel.getByText(zh
        ? "这个页面正在另一个 myco App 里打开，请先退出另一个 App 再试。"
        : "This page is open in another myco App. Quit that App, then try again.", { exact: true }).waitFor();
      if (await panel.locator(".pg-frame").count()) throw Error("a busy page origin must not open a fallback frame");
      await shot(page, size, scheme, "page-origin-in-use");
      await closePage();
      busy = false; // The other App has quit; retry the same page.
      intentionalPageOriginBusy.delete(page);
      await openPage();
      await panel.frameLocator(".pg-frame").locator("h1").waitFor();
      await closePage();
      await page.unroute(routeURL);
      await page.goto(`${r.base}/?lang=zh`);
      await page.locator(".app-shell").waitFor();
    });

    await step("网页 · 恢复旧本机文件", async () => {
      const zh = scheme === "dark";
      await page.goto(`${r.base}/?lang=${zh ? "zh" : "en"}`);
      await page.locator(".app-shell").waitFor();
      const recoveryOrigin = `http://${HOST}:${mockPort()}`;
      const recovery = `${recoveryOrigin}/__myco/recovery-test/`;
      let scans = 0;
      await page.route("**/app/hubs/*/pages/pg_release", async route => {
        const response = await route.fetch();
        await route.fulfill({ response, json: { ...await response.json(), template: { id: "tp_myco_photo", name: "Photo", version: "1.0.0" } } });
      });
      await page.route("**/app/hubs/*/pages/pg_release/open", async route => {
        const response = await route.fetch();
        await route.fulfill({ response, json: { ...await response.json(), recovery: true } });
      });
      await page.route("**/app/hubs/*/browser", route => route.fulfill({ json: { ok: true } }));
      await page.route("**/app/page-storage-recovery", route => route.fulfill({ json: { url: recovery } }));
      await page.route(`${recovery}**`, async route => {
        const url = new URL(route.request().url());
        if (url.pathname.endsWith("/seed")) return route.fulfill({ contentType: "text/html", body: "<!doctype html><title>Seed</title>" });
        if (url.pathname.endsWith("/recover.js")) return route.fulfill({ contentType: "text/javascript", body: await readFile(path.join(WEB, "../internal/console/page_recovery.js")) });
        if (url.searchParams.has("probe")) scans++;
        return route.fulfill({ contentType: "text/html", body: await readFile(path.join(WEB, "../internal/console/page_recovery.html")) });
      });
      const panel = page.locator(".pg-panel");
      const more = panel.getByRole("button", { name: zh ? "更多页面操作" : "More page actions" });
      const recover = zh ? "恢复旧本机文件" : "Recover older local files";
      const close = zh ? "关闭" : "Close";
      const openPage = () => page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-open-page", {
        detail: { kind: "page", hubId: "hub_studio", id: "pg_release" },
      })));
      const closePage = async () => {
        await panel.getByRole("button", { name: close, exact: true }).click();
        await panel.waitFor({ state: "detached" });
      };
      await openPage();
      await page.frameLocator(".pg-frame").locator("h1").waitFor();
      for (let i = 0; !scans && i < 30; i++) await page.waitForTimeout(100);
      if (scans !== 1) throw Error("host should probe the retired origin once");
      await page.locator('iframe[title="Recovery inventory"][data-checked="true"]').waitFor({ state: "attached" });
      if (await more.count() || await panel.locator(".fp-title-hint").count()) throw Error("empty legacy storage must not show a recovery menu or hint");
      await shot(page, size, scheme, "page-recovery-empty");
      await closePage();

      const seedNavigation = page.waitForEvent("framenavigated", frame => frame.url() === recovery + "seed");
      await page.evaluate(url => {
        const frame = document.createElement("iframe"); frame.id = "recovery-seed"; frame.hidden = true; frame.src = url; document.body.append(frame);
      }, recovery + "seed");
      await (await seedNavigation).evaluate(() => Promise.all(["myco-photo", "myco-video"].map(name => new Promise((resolve, reject) => {
        const request = indexedDB.open(name, 1);
        request.onupgradeneeded = () => request.result.createObjectStore("files");
        request.onerror = () => reject(request.error);
        request.onsuccess = () => {
          const db = request.result;
          const tx = db.transaction("files", "readwrite");
          tx.objectStore("files").put({ file: new Blob(["synthetic fixture"]), name: name + ".png" }, "fixture");
          tx.oncomplete = () => { db.close(); resolve(); };
          tx.onabort = () => reject(tx.error);
        };
      }))));
      await page.locator("#recovery-seed").evaluate(node => node.remove());
      await openPage();
      await more.waitFor();
      await panel.locator(".fp-title-hint").waitFor();
      if (await panel.locator(".pg-new").count()) throw Error("recovery must only appear in the title bar");
      const show = async () => {
        await more.click();
        await page.getByRole("menuitem", { name: recover, exact: true }).click();
        const dialog = page.getByRole("dialog", { name: recover, exact: true });
        await dialog.getByText("myco-photo.png", { exact: false }).waitFor();
        if (await dialog.locator("select option").count() !== 2 || await dialog.locator("select").inputValue() !== "1") throw Error("the viewer should select Photo and only list sources with data");
        return dialog;
      };
      let dialog = await show();
      await shot(page, size, scheme, "page-recovery-available");
      await dialog.getByRole("button", { name: close, exact: true }).click();
      await dialog.waitFor({ state: "detached" });
      dialog = await show();
      await page.keyboard.press("Escape");
      await dialog.waitFor({ state: "detached" });
      dialog = await show();
      const [download] = await Promise.all([
        page.waitForRequest(request => request.method() === "POST" && new URL(request.url()).pathname === "/app/page-download", { timeout: 10_000 }),
        dialog.getByRole("button", { name: zh ? "保存副本" : "Save copy", exact: true }).click({ timeout: 7_000 }),
      ]);
      if (!download.url().endsWith("name=myco-photo.png") || download.postData() !== "synthetic fixture") throw Error("could not save the original");
      await dialog.getByRole("button", { name: zh ? "标记已恢复" : "Mark recovered", exact: true }).click();
      await more.waitFor({ state: "detached" });
      await dialog.getByRole("button", { name: close, exact: true }).click();
      await closePage();
      const previousScans = scans;
      await openPage();
      for (let i = 0; scans === previousScans && i < 30; i++) await page.waitForTimeout(100);
      if (scans !== previousScans + 1) throw Error("reopening should check once");
      await page.locator('iframe[title="Recovery inventory"][data-checked="true"]').waitFor({ state: "attached" });
      if (await more.count() || await panel.locator(".fp-title-hint").count()) throw Error("the recovered app must stay hidden after reopening");
      await closePage();
      // These interceptions and language belong only to the recovery scene.
      for (const pattern of ["**/app/hubs/*/pages/pg_release", "**/app/hubs/*/pages/pg_release/open",
        "**/app/hubs/*/browser", "**/app/page-storage-recovery", `${recovery}**`]) await page.unroute(pattern);
      // Restore the suite's fixture language even at narrow widths, where
      // the next (multi-tab) scene is skipped.
      await page.goto(`${r.base}/?lang=zh`);
      await page.locator(".app-shell").waitFor();
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
      await clickVisible(page, panel.getByRole("button", { name: "收起", exact: true }));
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

    await step("浏览器 · 主人批准固定版本", async () => {
      await go(page, "官网改版", narrow);
      if (narrow) await clickVisible(page, page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page, page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" }));
      await clickVisible(page, page.locator(".sp-pane-right .pg-card", {hasText:"上线清单"}));
      const frame=page.frameLocator(".pg-frame");
      await frame.locator("h1").waitFor({timeout:15000});
      const request={script_id:"monthly_report",version:3,profile:"profile_work",script:{name:"月报检查",profile:"profile_work",start_url:"https://example.com/reports",params:[],on_error:"stop",graph:{nodes:[{id:"check-title",type:"js",code:"return document.title;"},{id:"open-summary",type:"navigate",url:"https://example.com/summary"}],edges:[["check-title","open-summary"]]}},scripts:{}};
      const ask=()=>frame.locator("body").evaluate((_body,request)=>{
        window.__browserApprovalResult=null;
        window.myco.browser.approvals.request(request).then(value=>window.__browserApprovalResult={value},error=>window.__browserApprovalResult={error:error.code});
      },request);
      await ask();
      const modal=page.getByRole("dialog",{name:"浏览器自动化授权"});await modal.waitFor();
      const review=modal.locator("[data-testid=browser-script-review]");await review.waitFor();
      const text=await review.innerText();
      if(!text.includes("v3")||!text.includes("document.title")||!text.includes("https://example.com/reports")||!text.includes("https://example.com/summary"))throw new Error("Trusted review omitted version, URL or executable code");
      await shot(page,size,scheme,"browser-script-owner-review");
      await clickVisible(page,modal.getByRole("button",{name:"不允许",exact:true}));await modal.waitFor({state:"hidden"});
      await frame.locator("body").evaluate(async()=>{for(let i=0;i<50&&!window.__browserApprovalResult;i++)await new Promise(r=>setTimeout(r,20));if(window.__browserApprovalResult?.error!=="cancelled")throw new Error("declined script was not cancelled");});
      const before=await frame.locator("body").evaluate(()=>window.myco.browser.approvals.list());if(before.length)throw new Error("Decline saved approval");
      await ask();await modal.waitFor();await clickVisible(page,modal.getByRole("button",{name:"允许",exact:true}));await modal.waitFor({state:"hidden"});
      const result=await frame.locator("body").evaluate(async()=>{for(let i=0;i<100&&!window.__browserApprovalResult;i++)await new Promise(r=>setTimeout(r,20));return window.__browserApprovalResult;});
      if(result?.value?.version!==3||result?.value?.script_id!=="monthly_report"||"confirmation" in result.value||"script" in result.value)throw new Error("Page did not receive private-safe approval metadata");
      await clickVisible(page,page.locator(".pg-panel").getByRole("button",{name:"关闭",exact:true}));
    });

    await step("浏览器 · 配置需要主人确认", async () => {
      await go(page, "官网改版", narrow);
      if (narrow) await clickVisible(page, page.locator('.sp-strip-face[title="Release (Claude 云端)"]'));
      else await clickVisible(page, page.locator(".sp-side .sp-member:visible", {hasText:"Release (Claude 云端)"}));
      await clickVisible(page,page.locator(".sp-pane-right .pg-card",{hasText:"上线清单"}));
      const frame=page.frameLocator(".pg-frame");await frame.locator("h1").waitFor({timeout:15000});
      const settings={proxy:{server:"http://proxy.example:8080",bypass:["*.local","<local>"],username:"fixture-user",password:"write-only-fixture"},user_agent:"Mozilla/5.0 Fixture/1.0",lang:"zh-CN",args:["--window-size=1280,900","--disable-gpu"]};
      const ask=(requested=settings)=>frame.locator("body").evaluate((_body,settings)=>{window.__browserSettingsResult=null;window.myco.browser.profiles.configure("profile_work",settings).then(value=>window.__browserSettingsResult={value},error=>window.__browserSettingsResult={error:error.code});},requested);
      await ask();
      const modal=page.getByRole("dialog",{name:"浏览器自动化授权"});await modal.waitFor();
      const review=modal.locator("[data-testid=browser-settings-review]");await review.waitFor();
      const text=await review.innerText();
      for(const value of ["proxy.example:8080","*.local","Mozilla/5.0 Fixture/1.0","zh-CN","--window-size=1280,900","--disable-gpu"])if(!text.includes(value))throw new Error("Settings review omitted "+value);
      if(text.includes("write-only-fixture"))throw new Error("Settings modal disclosed proxy password");
      const allow=modal.getByRole("button",{name:"允许",exact:true});
      const readAll=async()=>{
        await review.evaluate(el=>{el.scrollTop=el.scrollHeight;});
        await allow.waitFor({state:"visible"});
        await page.waitForFunction(()=>!document.querySelector('.pg-remote-ask .app-btn-primary')?.disabled);
      };
      await modal.evaluate(el=>Promise.all(el.getAnimations().map(a=>a.finished)));
      const actionsInitial=await modal.locator('.app-modal-actions').boundingBox();
      const overflow=await review.evaluate(el=>el.scrollHeight>el.clientHeight+2);
      if(overflow && !await allow.isDisabled())throw new Error("Overflowing settings can be allowed before reading");
      await readAll();
      // Check geometry inside the scroll viewport: locator visibility alone does
      // not detect content clipped by a scroll container.
      const visible=await review.evaluate(el=>{
        const box=el.getBoundingClientRect();
        const args=el.querySelector('dd:last-of-type').getBoundingClientRect();
        const warning=el.querySelector('p:last-child').getBoundingClientRect();
        return args.top>=box.top && args.bottom<=box.bottom+1 && warning.bottom<=box.bottom+1;
      });
      if(!visible)throw new Error("Launch arguments or UA warning cannot be scrolled into view");
      const actionsBefore=await modal.locator('.app-modal-actions').boundingBox();
      if(Math.abs(actionsInitial.y-actionsBefore.y)>1)throw new Error("Reading settings shifted approval buttons");
      await review.evaluate(el=>{el.scrollTop=0;});
      const actionsAfter=await modal.locator('.app-modal-actions').boundingBox();
      if(Math.abs(actionsBefore.y-actionsAfter.y)>1)throw new Error("Review scrolling moved approval buttons");
      await readAll();
      await shot(page,size,scheme,"browser-profile-settings-owner-review-bottom");
      await clickVisible(page,modal.getByRole("button",{name:"不允许",exact:true}));await modal.waitFor({state:"hidden"});
      let result=await frame.locator("body").evaluate(async()=>{for(let i=0;i<100&&!window.__browserSettingsResult;i++)await new Promise(r=>setTimeout(r,20));return window.__browserSettingsResult;});
      if(result?.error!=="cancelled")throw new Error("Declined settings were not cancelled");
      const before=await frame.locator("body").evaluate(()=>window.myco.browser.status());if(before.profiles[0].settings.proxy)throw new Error("Decline changed profile settings");
      await ask();await modal.waitFor();
      if(overflow && !await allow.isDisabled())throw new Error("New settings review did not reset the read requirement");
      await readAll();await clickVisible(page,allow);await modal.waitFor({state:"hidden"});
      result=await frame.locator("body").evaluate(async()=>{for(let i=0;i<100&&!window.__browserSettingsResult;i++)await new Promise(r=>setTimeout(r,20));return window.__browserSettingsResult;});
      if(!result?.value?.settings?.proxy?.has_password||JSON.stringify(result).includes("write-only-fixture")||"confirmation" in result.value)throw new Error("Configured profile returned private credentials");
      const {password:_password,...storedProxy}=settings.proxy;
      for(const [kind,proxy,retains] of [
        ["same",storedProxy,true],
        ["server",{...storedProxy,server:"http://other.example:8080"},false],
        ["scheme",{...storedProxy,server:"https://proxy.example:8080"},false],
        ["username",{...storedProxy,username:"other-user"},false],
      ]) {
        await ask({...settings,proxy});await modal.waitFor();
        const summary=await review.innerText();
        if(!summary.includes(retains?"保留已保存的密码":"清除已保存的密码"))throw new Error("Wrong saved-password review for "+kind);
        await clickVisible(page,modal.getByRole("button",{name:"不允许",exact:true}));await modal.waitFor({state:"hidden"});
      }
      const remove=()=>frame.locator("body").evaluate(()=>{window.__browserRemoval=null;window.myco.browser.profiles.remove("profile_work").then(value=>window.__browserRemoval={value},error=>window.__browserRemoval={error:error.code});});
      await remove();await modal.waitFor();
      const deletionText=await modal.innerText();
      for(const part of ["永久删除","工作资料","登录状态","本机下载","所有工作区","不能恢复"])if(!deletionText.includes(part))throw new Error("Deletion review omitted "+part);
      await shot(page,size,scheme,"browser-profile-delete-owner-review");
      await clickVisible(page,modal.getByRole("button",{name:"不允许",exact:true}));await modal.waitFor({state:"hidden"});
      let removal=await frame.locator("body").evaluate(async()=>{for(let i=0;i<100&&!window.__browserRemoval;i++)await new Promise(r=>setTimeout(r,20));return window.__browserRemoval;});
      if(removal?.error!=="cancelled")throw new Error("Declined profile deletion was not cancelled");
      if(!(await frame.locator("body").evaluate(()=>window.myco.browser.status())).profiles.length)throw new Error("Decline deleted a profile");
      await remove();await modal.waitFor();await clickVisible(page,modal.getByRole("button",{name:"允许",exact:true}));await modal.waitFor({state:"hidden"});
      removal=await frame.locator("body").evaluate(async()=>{for(let i=0;i<100&&!window.__browserRemoval;i++)await new Promise(r=>setTimeout(r,20));return window.__browserRemoval;});
      if(removal?.value!==true || (await frame.locator("body").evaluate(()=>window.myco.browser.status())).profiles.length)throw new Error("Confirmed deletion did not remove the profile");
      await clickVisible(page,page.locator(".pg-panel").getByRole("button",{name:"关闭",exact:true}));
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

    await step("网页 · 记录查询", async () => {
      await go(page, "官网改版", narrow);
      const pattern = "**/app/hubs/hub_studio/pages/pg_checklist/data?*";
      const queries = [];
      await page.route(pattern, async route => {
        const q = new URL(route.request().url()).searchParams;
        queries.push(q);
        const queried = q.has("query_version");
        await route.fulfill({ json: q.get("count") === "true"
          ? { count: 1, seq: 999, query_version: 1 }
          : { records: [{ collection: "issues", id: "i42", version: 3, data: queried ? { n: 42 } : { n: 42, title: "Demo" } }], seq: queried ? 999 : 10, query_version: 1 } });
      });
      try {
        await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-open-page", { detail: { kind: "page", hubId: "hub_studio", id: "pg_checklist" } })));
        const frame = () => page.frames().find(f => f.url().includes("/p/"));
        await page.frameLocator(".pg-frame").locator("#who").waitFor();
        await frame().evaluate(async () => {
          await window.myco.ready;
          await window.myco.data.list("issues");
          const r = await window.myco.data.list("issues", { fields: ["n"], where: ["status:in:todo,progress", "assignee:alex"], sort: "-n", limit: 1 });
          if (r.records[0].data.n !== 42 || "title" in r.records[0].data) throw new Error("projection was lost");
          const c = await window.myco.data.list("issues", { count: true });
          if (c.count !== 1 || "records" in c) throw new Error("count was lost");
        });
        const q = queries.find(q => q.get("fields") === "n");
        if (!q || q.getAll("where").join("|") !== "status:in:todo,progress|assignee:alex" || q.get("sort") !== "-n" || q.get("limit") !== "1") throw new Error("bridge query options changed in transit");
        const watch = page.waitForRequest(r => r.url().includes("/pages/pg_checklist/changes?"), { timeout: 10_000 });
        await frame().evaluate(() => { window.stopQueryWatch = window.myco.data.on(() => {}); });
        const request = await watch;
        if (new URL(request.url()).searchParams.get("after") !== "10") throw new Error("query/count advanced the shared editor watch cursor");
        await frame().evaluate(() => window.stopQueryWatch());
      } finally {
        await page.unroute(pattern);
        await clickVisible(page, page.locator(".pg-panel").getByRole("button", { name: /关闭/ }).first());
      }
    });

    await step("网页 · 页面附件", async () => {
      await go(page, "官网改版", narrow);
      const meta = "**/app/hubs/hub_studio/pages/pg_checklist";
      const files = "**/app/hubs/hub_studio/pages/pg_checklist/attachments**";
      let oldHub = false, stored, bytes = Buffer.alloc(0), projectWrites = 0;
      const observe = request => { if (/\/projects\/.*\/files\//.test(request.url()) && ["POST", "PUT"].includes(request.method())) projectWrites++; };
      page.on("request", observe);
      await page.route(meta, async route => {
        const response = await route.fetch(), value = await response.json();
        value.template = { id: "tp_myco_media-kit", name: "Media Kit", version: oldHub ? "1.3.1" : "1.4.0", permissions: ["download"], declares: !oldHub };
        if (oldHub) delete value.attachments_version;
        else value.attachments_version = 1;
        await route.fulfill({json:value});
      });
      await page.route(files, async route => {
        const request=route.request(), url=new URL(request.url());
        if(request.method()==="POST" && url.pathname.endsWith("/attachments")) {
          bytes=request.postDataBuffer();
          const key=request.headers()["idempotency-key"];
          if(url.searchParams.get("collection")==="items" ? key!=="stable-key" : !/^desktop:[a-f0-9]{64}$/.test(key)) throw new Error("upload key was lost");
          stored={file_id:"paf_fixture",page:"pg_checklist",collection:url.searchParams.get("collection"),name:url.searchParams.get("name"),size:bytes.length,sha256:"fixture",content_type:request.headers()["content-type"],state:"pending",created_at:new Date().toISOString()};
          if(url.searchParams.get("legacy_name")) stored.legacy_name=url.searchParams.get("legacy_name");
          return route.fulfill({status:201,json:stored});
        }
        if(url.pathname.endsWith("/content")) return route.fulfill({contentType:stored?.content_type || "text/plain",body:bytes});
        if(url.pathname.endsWith("/save")) return route.fulfill({json:{path:"/Downloads/sample.txt"}});
        if(request.method()==="DELETE") {stored=undefined;return route.fulfill({status:204});}
        if(url.searchParams.has("name")) return route.fulfill({json:stored});
        return route.fulfill({json:{attachments_version:1,attachments:stored?[stored]:[],usage:{files_used:stored?1:0,files_max:1000,bytes_used:stored?bytes.length:0,bytes_max:2*1024**3,max_file_bytes:20*1024**2}}});
      });
      const open = async () => {
        await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-open-page", {detail:{kind:"page",hubId:"hub_studio",id:"pg_checklist"}})));
        await page.frameLocator(".pg-frame").locator("#who").waitFor();
        return page.frames().find(f=>f.url().includes("/p/"));
      };
      const close = () => clickVisible(page,page.locator(".pg-panel").getByRole("button",{name:/关闭/}).first());
      try {
        const frame=await open();
        await frame.evaluate(async () => {
          const ctx=await myco.ready;
          if(!ctx.pageFiles||!myco.features.includes("page-files")) throw new Error("missing page file negotiation: " + JSON.stringify({ctx, features:myco.features}));
          const file=await myco.pageFiles.put("items","sample.txt",new Blob(["page bytes"],{type:"text/plain"}),{key:"stable-key"});
          if(await (await myco.pageFiles.get({...file,page:"pg_other"})).text()!=="page bytes") throw new Error("page file bytes changed");
          if((await myco.pageFiles.info()).files_used!==1||(await myco.pageFiles.list({limit:1})).attachments[0].file_id!==file.file_id) throw new Error("page quota/list changed");
          if((await myco.pageFiles.resolve("old.txt")).file_id!==file.file_id) throw new Error("legacy resolution changed");
          await myco.pageFiles.download(file);
          await myco.pageFiles.remove(file);
          if((await myco.pageFiles.info()).files_used!==0) throw new Error("remove did not release quota");
          const legacyName="mk-pg_checklist-entry-valid000.png";
          await myco.projectFiles.put(legacyName,new Blob(["png"],{type:"image/png"}));
          const resolved=await myco.pageFiles.resolve(legacyName);
          if(resolved.legacy_name!==legacyName||resolved.collection!=="images") throw new Error("legacy upload did not become a page attachment");
          const row=await myco.data.put("images",{file:legacyName},{id:"legacy-file",ifVersion:0});
          if(await (await myco.projectFiles.get(legacyName)).text()!=="png") throw new Error("legacy read lost original bytes");
          if((await myco.projectFiles.info()).max!==1000) throw new Error("legacy bridge did not report page quota");
          await myco.projectFiles.download(legacyName);
          await myco.data.remove("images",row.id,{ifVersion:row.version});
          await myco.pageFiles.remove(resolved);

        });
        await close();oldHub=true;
        const legacy=await open();
        await page.getByText("当前只能查看。请更新工作区后使用页面附件。",{exact:true}).waitFor();
        await legacy.evaluate(async () => {
          const ctx=await myco.ready;
          if(ctx.pageFiles||ctx.me.can_edit!==false) throw new Error("old Hub must be view only");
          for(const run of [()=>myco.pageFiles.put("images","sample.png",new Blob(["png"])),()=>myco.projectFiles.put("mk-pg_checklist-entry-valid000.png",new Blob(["png"],{type:"image/png"})),()=>myco.data.put("images",{}),()=>myco.update({})]) {
            try { await run(); throw new Error("old Hub write was accepted"); } catch(error) {if(error.code!=="unsupported") throw error;}
          }
        });
        if(projectWrites) throw new Error("page storage fell back to project uploads");
        await shot(page,size,scheme,"page-files-upgrade");
      } finally {
        page.off("request",observe);await page.unroute(meta);await page.unroute(files);await close();
      }
    });

    // A page made from the starter `myco template init` writes: its own
    // small HTML table (internal/cli/templatekit), not the official 表格 app
    // (that one is the canvas grid, made from the locked apps checkout and
    // checked in 应用 · 应用库和官方应用). Its records go through the
    // bridge; you edit a cell (saved with its version), the agent adds a row
    // (it shows up), and it exports a CSV to Downloads.
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
      // The suite grows (学习工作台 joined it): check the core ones are there, in order.
      const core = ["表格", "多维表格", "文档", "幻灯片", "会议记录"];
      const at = core.map((c) => names.findIndex((n) => n.startsWith(c) && !(c === "表格" && n.startsWith("多维表格"))));
      if (at.some((i) => i < 0) || at.some((i, k) => k > 0 && i < at[k - 1])) throw new Error(`办公套件应依次有 ${core.join("、")}，实际 ${names.join(" / ")}`);
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
        // No sample-data choice: the first of a kind comes with it, and says so.
        if (await page.locator(".app-modal input[type=checkbox]").count()) throw new Error("新建应用的弹框不该再让人选要不要示例数据");
        await page.locator("[data-testid=template-sample-note]").waitFor({ timeout: 5_000 });
        await clickVisible(page, page.getByRole("button", { name: "新建", exact: true }));
        const frame = page.locator(`.pg-frame[aria-label="${name}"]`);
        await frame.waitFor({ timeout: 15_000 });
        return page.frameLocator(`.pg-frame[aria-label="${name}"]`);
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
      // Remote Desktop stays in Apps. Installation and sharing need the
      // trusted App's permission; the embedded page cannot grant it itself.
      let remoteInstalled = false, remoteSharing = false, installs = 0;
      const remoteRoute = "**/app/hubs/hub_studio/remote";
      let stopped;
      const stopSeen = new Promise((resolve) => { stopped = resolve; });
      await page.route(remoteRoute, async route => {
        const body = route.request().postDataJSON();
        if (body.action === "devices") return route.continue();
        if (body.action === "install") { installs++; remoteInstalled = true; }
        if (body.action === "enable") remoteSharing = true;
        if (body.action === "close") { remoteSharing = false; stopped(); }
        if (body.action === "status" || body.action === "install" || body.action === "enable") {
          return route.fulfill({json:{protocol:1,version:"0.1.0",installed:remoteInstalled,supported:true,running:remoteSharing,...(remoteSharing?{pairing_code:"a1b2c3d4"}:{})}});
        }
        return route.fulfill({json:true});
      });
      const remote = await make("myco/remote-desktop", `远程（${mark}）`);
      await remote.locator(".rd-device").first().waitFor({timeout:15000});
      // Chromium/Playwright disagree on cross-frame coordinates under CSS
      // zoom. Convert the child viewport position to App viewport pixels so
      // these remain real mouse clicks, including at the App's default 108%.
      const remoteFrame = page.locator(`.pg-frame[aria-label="远程（${mark}）"]`);
      const clickRemote = async (locator) => {
        await locator.scrollIntoViewIfNeeded();
        const point = await locator.evaluate(el => {const r=el.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2};});
        const frame = await remoteFrame.evaluate(el => {const r=el.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,cw:el.clientWidth,ch:el.clientHeight};});
        await page.mouse.click(frame.x+point.x*frame.width/frame.cw,frame.y+point.y*frame.height/frame.ch);
      };
      await clickRemote(remote.locator(".rd-tabs button").nth(1));
      await remote.locator(".rd-install-card").waitFor();
      await clickRemote(remote.locator(".rd-install-card .rd-primary"));
      const permission = page.locator(".pg-remote-ask");
      await permission.waitFor();
      if (installs !== 0) throw new Error("远控组件在用户授权前就安装了");
      await clickVisible(page, permission.getByRole("button", {name:"不允许",exact:true}));
      await remote.locator(".rd-error").waitFor();
      if (installs !== 0) throw new Error("拒绝授权后仍安装了远控组件");
      await clickRemote(remote.locator(".rd-install-card .rd-primary"));
      await permission.waitFor();
      await settle(page,200);
      await shot(page,size,scheme,"remote-desktop-permission");
      await clickVisible(page, permission.getByRole("button", {name:"允许",exact:true}));
      await remote.locator(".rd-enable-card").waitFor();
      if (installs !== 1) throw new Error("应只在授权后安装一次远控组件");
      await clickRemote(remote.locator(".rd-enable-card .rd-primary"));
      await permission.waitFor();
      await clickVisible(page, permission.getByRole("button", {name:"允许",exact:true}));
      await remote.locator(".rd-code", {hasText:"a1b2c3d4"}).waitFor();
      await shot(page,size,scheme,"remote-desktop-sharing");
      await clickVisible(page,page.locator(".pg-panel").getByRole("button",{name:/关闭/}).first());
      await Promise.race([stopSeen,new Promise((_,reject)=>setTimeout(()=>reject(new Error("关闭网页后没有结束远控共享")),5000))]);
      await page.unroute(remoteRoute);
      // The pages made are listed under the templates.
      await tuck();
      await go(page, "应用", narrow);
      await page.locator(".tp-made-row", { hasText: `计划（${mark}）` }).waitFor({ timeout: 10_000 });
      if (await page.locator(".pg-toggle").count()) {
        await clickVisible(page, page.locator(".pg-toggle"));
        await clickVisible(page, page.locator(".pg-panel").getByRole("button", { name: /关闭/ }).first());
      }
      // (back where the steps after this one start: the project)
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home, .sp-group").first().waitFor({ timeout: 10_000 });
    });

    // A failed request names its id (X-Request-Id), the one the App sent:
    // the hub logs it, so the error can be found there. This one answers
    // without an id of its own, as a timeout would.
    await step("出错 · 显示请求编号", async () => {
      const routeURL = /\/app\/hubs\/[^/]+\/templates(\?|$)/;
      let sent = "";
      intentionalTemplatesFail.add(page);
      await page.route(routeURL, (route) => {
        if (route.request().method() !== "GET") return route.continue();
        sent = route.request().headers()["x-request-id"] || "";
        return route.fulfill({ status: 500, json: { error: "internal", message: "database is locked" } });
      });
      try {
        await go(page, "应用", narrow);
        const id = page.locator(".tp .app-error-id").first();
        await id.waitFor({ timeout: 10_000 });
        if (!/^[0-9a-f]{16}$/.test(sent)) throw new Error(`App 应给每个请求带上 X-Request-Id，实际「${sent}」`);
        const text = (await id.innerText()).trim();
        if (text !== `请求编号 ${sent}`) throw new Error(`出错时应显示这次请求的编号「${sent}」，实际「${text}」`);
        // (details show only where the message replaced the raw text)
        const more = page.locator(".tp .app-error-more", { hasText: "详细信息" }).first();
        if (await more.count()) {
          await clickVisible(page, more);
          const detail = await page.locator(".tp .app-error-detail").first().innerText();
          if (!detail.includes(sent) || !/用时 \d/.test(detail)) throw new Error(`详细信息里也应有请求编号和用时，实际「${detail}」`);
        }
        await shot(page, size, scheme, "error-request-id");
      } finally {
        await page.unroute(routeURL);
        intentionalTemplatesFail.delete(page);
      }
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home, .sp-group").first().waitFor({ timeout: 10_000 });
    });

    await step("应用 · English metadata and legacy Hub", async () => {
      const pattern="**/app/hubs/hub_studio/templates";
      try {
        await page.goto(`${r.base}/?lang=en`, {waitUntil:"domcontentloaded"});
        await page.locator(".app-shell").waitFor();
        await go(page,"应用",narrow);
        const table=page.locator('[data-testid="template-myco/table"]');
        await table.locator('.tp-card-name',{hasText:"Table"}).waitFor();
        if(await page.locator('[data-testid="template-myco/poll"]').count())throw new Error("The retired Poll app must not be offered");
        await page.locator('[data-testid="template-myco/notes"] .tp-card-name',{hasText:"Notes"}).waitFor();
        await table.click();
        await page.locator('[data-testid="template-name"]').waitFor();
        if(await page.locator('[data-testid="template-name"]').inputValue()!=="Table")throw new Error("New app name must follow the App language");
        await clickVisible(page,page.getByRole("button",{name:"Cancel",exact:true}));
        await shot(page,size,scheme,"apps-english");
        await page.route(pattern,async route=>{const response=await route.fetch();const data=await response.json();for(const t of data.templates||[])delete t.locales;data.templates.push({...data.templates[0],id:"tp_retired_poll",slug:"myco/poll",name:"投票"});await route.fulfill({response,json:data});});
        await page.reload({waitUntil:"domcontentloaded"});await page.locator(".app-shell").waitFor();await go(page,"应用",narrow);
        await page.locator('[data-testid="template-myco/notes"] .tp-card-name',{hasText:"Notes"}).waitFor();
        if(await page.locator('[data-testid="template-myco/poll"]').count())throw new Error("Legacy Hub returned a retired Poll app");
      } finally {
        await page.unroute(pattern);
        await page.goto(`${r.base}/?lang=zh`,{waitUntil:"domcontentloaded"});await page.locator(".app-shell").waitFor();
      }
    });

    // The notch's island (the Mac app's own window, /?island=1): how many
    // are at work, never "0". A workspace that cannot be read keeps who was
    // at work there; someone who stops before the window is ready leaves
    // it folded away (a slow network made it stay out showing 0).
    if (!narrow) await step("灵动岛 · 不显示 0", async () => {
      const ip = await page.context().newPage();
      try {
        const real = await (await fetch(`http://${HOST}:${mockPort()}/app/overview`)).json();
        let mode = "working";
        const overview = () => {
          const ov = JSON.parse(JSON.stringify(real));
          for (const h of ov.hubs) {
            if (h.id !== "hub_studio") continue;
            if (mode === "down") { h.ok = false; delete h.overview; continue; }
            for (const st of h.overview?.agents || []) {
              // Its shell nodes run commands too: they are not counted.
              const shell = st.agent.kind === "shell" || ["shell", "exec"].includes(st.agent.binding?.harness);
              const on = mode === "working" && (st.agent.name === "review-cloud" || shell);
              st.state = on ? "working" : "idle";
              st.in_progress = on ? 1 : 0;
              st.stale = 0;
              st.stale_tasks = [];
            }
          }
          for (const h of ov.hubs) if (h.id !== "hub_studio" && h.overview) for (const st of h.overview.agents || []) st.state = "idle";
          return ov;
        };
        await ip.route("**/app/overview", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(overview()) }));
        let frameDelay = 0;
        await ip.route("**/app/island/frame", async (route) => {
          if (frameDelay) await new Promise((ok) => setTimeout(ok, frameDelay));
          await route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
        });
        await ip.setViewportSize({ width: 520, height: 320 });
        await ip.goto(`${r.base}/?island=1&nw=200&nh=32&lang=zh`, { waitUntil: "domcontentloaded" });
        // (every number the island ever draws, recorded)
        await ip.evaluate(() => {
          window.__islandNumbers = [];
          const note = () => { const b = document.querySelector(".isl.is-shown .isl-wing.is-right"); if (b) window.__islandNumbers.push((b.textContent || "").trim()); };
          new MutationObserver(note).observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true });
        });
        const right = ip.locator(".isl.is-shown .isl-wing.is-right b");
        await right.filter({ hasText: /^1$/ }).waitFor({ timeout: 15_000 });
        // The workspace cannot be read for a while: still 1, not gone.
        mode = "down";
        await ip.waitForTimeout(6_000);
        if ((await right.innerText().catch(() => "")) !== "1") throw new Error("工作区暂时连不上时，灵动岛应保持原来的人数");
        // Nobody at work: it folds away.
        mode = "idle";
        await ip.locator(".isl.is-shown").waitFor({ state: "detached", timeout: 10_000 });
        // Someone starts and stops before the window is ready: it stays in.
        frameDelay = 3_500;
        mode = "working";
        await ip.waitForTimeout(3_000);
        mode = "idle";
        await ip.waitForTimeout(6_000);
        if (await ip.locator(".isl.is-shown").count()) throw new Error("开始又马上停下时，灵动岛不该停在外面");
        const seen = await ip.evaluate(() => window.__islandNumbers);
        if (seen.some((t) => t === "0" || /^0\D/.test(t))) throw new Error(`灵动岛不该显示 0：${JSON.stringify([...new Set(seen)])}`);
      } finally {
        await ip.close();
      }
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
      // (pointed out for a moment: a busy machine may not look in time, so
      // whether it happened at all is recorded)
      const watchCalled = (sel) => page.evaluate((sel) => {
        window.__called = false;
        const mo = new MutationObserver(() => { if (document.querySelector(sel)) { window.__called = true; mo.disconnect(); } });
        mo.observe(document.body, { subtree: true, attributes: true, attributeFilter: ["class"] });
      }, sel);
      const wasCalled = () => page.waitForFunction(() => window.__called === true, null, { timeout: 5_000 });
      // The coordinator, on screen: nothing moves, its pane is pointed out.
      await watchCalled(".sp-home.is-called");
      await pick("pm-codex", "website");
      await wasCalled();
      if (await page.locator(".sp-pane-right").count()) throw new Error("协调者已经在屏幕上，不该再打开别的栏");
      // A member of this project: beside the coordinator.
      await pick("docs-claude", "website");
      const right = page.locator(".sp-pane-right");
      await right.locator(".conv-head h2", { hasText: "docs-claude" }).waitFor({ timeout: 10_000 });
      if (!(await page.locator(".sp-pane-left .sp-home").isVisible())) throw new Error("打开成员时，协调者应还在左边");
      // Again, while it shows: it stays, pointed out.
      await watchCalled(".sp-pane-right.is-called");
      await pick("docs-claude", "website");
      await wasCalled();
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
      // As a group chat: a line per task and per answer, "@" whom it was
      // for, in time; a click opens the thread. The choice is kept.
      await page.locator(".sp-head").getByRole("tab", { name: "群聊" }).click();
      const chat = page.locator("[data-testid=activity-chat]");
      await chat.waitFor({ timeout: 5_000 });
      const lines = chat.locator(".sp-gchat-line");
      if ((await lines.count()) < 2) throw new Error("群聊视图应有好几行");
      if (!(await chat.locator(".sp-gchat-at").first().innerText()).startsWith("@")) throw new Error("每一行应写明 @ 谁");
      if (!(await chat.locator(".sp-gchat-quote").count())) throw new Error("回复应引用它答的那一条");
      const times = await lines.evaluateAll((els) => els.map((e) => e.querySelector(".sp-gchat-when")?.textContent || ""));
      if (!times.every(Boolean)) throw new Error("每一行应有时间");
      await settle(page);
      await shot(page, size, scheme, "project-activity-chat");
      await page.locator(".sp-head").getByRole("tab", { name: "卡片" }).click();
      await page.locator(".sp-group .sp-card").first().waitFor({ timeout: 5_000 });
      await clickVisible(page, page.locator(".sp-head button[aria-label='返回']"));
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    // A restart on a slow network: what was there last time shows at once
    // from the App's own saved cache (the sidebar's projects and chats, the
    // project's chat), and is refreshed when the workspace answers.
    await step("启动 · 先显示上次的数据", async () => {
      await page.waitForTimeout(2000); // (the cache is saved a moment after a change)
      await page.evaluate(() => window.dispatchEvent(new Event("pagehide")));
      await page.waitForTimeout(500);
      const slow = async (route) => {
        const u = route.request().url();
        // (what goes to a workspace: its overview, anything of one hub;
        // the App's own list of workspaces is on this computer)
        if (/\/app\/(overview\b|hubs\/[^/?]+\/)/.test(u)) await new Promise((ok) => setTimeout(ok, 8000));
        await route.continue().catch(() => {});
      };
      await page.route("**/app/**", slow);
      try {
        const t0 = Date.now();
        await page.reload({ waitUntil: "domcontentloaded" });
        if (narrow) await sidebar(page, true);
        await page.locator(".app-sidebar").getByText("官网改版", { exact: true }).first().waitFor({ timeout: 4_000 });
        await page.locator(".app-sidebar").getByText("PM (Codex)", { exact: true }).first().waitFor({ timeout: 2_000 });
        const ms = Date.now() - t0;
        if (ms > 5_000) throw new Error(`重启后应先显示上次的项目和会话，实际等了 ${ms} 毫秒`);
        await settle(page, 200);
        await shot(page, size, scheme, "start-from-cache");
      } finally {
        await page.unroute("**/app/**", slow);
      }
      if (narrow) await sidebar(page, false);
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

    // First where the agent is: one already in the workspace, this computer,
    // another computer (a VPS), one to connect, or an AI elsewhere; on a
    // computer, a session already there or a new one. Each shows up at once.
    await step("项目 · 添加 agent", async () => {
      if (narrow) await clickVisible(page, page.locator(".sp-strip-face.is-add"));
      else await clickVisible(page, page.locator(".sp-side .sp-side-add").first());
      const dialog = page.locator(".app-modal-add");
      await dialog.waitFor();
      // Members of the workspace not in the project come first.
      await dialog.getByRole("tab", { name: "工作区里已有的", selected: true }).waitFor({ timeout: 10_000 });
      await dialog.locator(".add-local-row button", { hasText: "加进来" }).first().waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-members");
      await dialog.locator(".add-local-row button", { hasText: "加进来" }).first().click();
      await dialog.locator(".add-arrived").first().waitFor({ timeout: 20_000 });

      await clickVisible(page, dialog.getByRole("tab", { name: "这台电脑" }));
      await clickVisible(page, dialog.getByRole("tab", { name: "用上面已有的会话" }));
      await dialog.locator(".add-local-row button", { hasText: "加入" }).first().waitFor({ timeout: 10_000 });
      // A session that joined two workspaces names both.
      await dialog.locator(".add-local-row.is-taken .add-local-sub", { hasText: /已经加入了：.*的 alice-mac-claude-x9k2、.*acme.* 的 alice-mac-claude-x9k2-tn/ }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent");
      await dialog.locator(".add-local-row button", { hasText: "加入" }).first().click();
      await dialog.locator(".add-arrived").nth(1).waitFor({ timeout: 20_000 });

      // A cloud server: its own sessions, asked of it; one outside the
      // folders it allows cannot join; a copy of another one joins.
      await clickVisible(page, dialog.getByRole("tab", { name: "VPS 东京" }));
      const remote = dialog.locator("[data-testid=remote-sessions]");
      await remote.locator(".add-local-row", { hasText: "把订单接口的分页改成游标分页" }).waitFor({ timeout: 10_000 });
      const outside = remote.locator(".add-local-row", { hasText: "试一下 bun 的启动速度" });
      await outside.locator(".add-local-sub", { hasText: "不在那台电脑允许的文件夹里" }).waitFor({ timeout: 5_000 });
      if (!(await outside.locator("button").isDisabled())) throw new Error("允许的文件夹之外的会话不能加入");
      await remote.locator(".add-local-row.is-taken", { hasText: "已经加入了：tokyo-vps-claude-ops" }).waitFor({ timeout: 5_000 });
      // What programs started (myco, scripts) is there too, folded away.
      const byProgram = remote.locator("[data-testid=by-program]");
      await byProgram.locator("summary", { hasText: "还有 2 个是程序开的会话" }).waitFor({ timeout: 5_000 });
      if (await byProgram.locator(".add-local-row", { hasText: "跑一遍测试" }).isVisible()) throw new Error("程序开的会话应先折起来");
      await clickVisible(page, byProgram.locator("summary"));
      await byProgram.locator(".add-local-row", { hasText: "跑一遍测试" }).waitFor({ timeout: 5_000 });
      await clickVisible(page, byProgram.locator("summary"));
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-remote");
      await remote.locator(".add-local-row", { hasText: "把订单接口的分页改成游标分页" }).locator("button").click();
      await dialog.locator(".add-arrived").nth(2).waitFor({ timeout: 20_000 });
      // A new one there: it lacks Codex (with the command to install it),
      // and keeps to its folder (a new one goes inside it).
      await clickVisible(page, dialog.getByRole("tab", { name: "新开一个" }));
      await dialog.locator("[data-testid=allowed-dirs]", { hasText: "~/code/demo-team" }).waitFor({ timeout: 10_000 });
      const vpsFolder = dialog.locator("input.app-input.mono").first();
      if (!/^不填就新建 ~\/code\/demo-team\//.test(await vpsFolder.getAttribute("placeholder") || "")) throw new Error("只允许一个文件夹的电脑，新文件夹应建在它里面");
      await clickVisible(page, dialog.locator(".ws-computer", { hasText: "Codex" }));
      await dialog.locator("[data-testid=remote-ai-missing] code", { hasText: "npm install -g @openai/codex" }).waitFor({ timeout: 5_000 });
      await vpsFolder.fill("~/elsewhere");
      await dialog.locator("[data-testid=allowed-dirs].is-bad").waitFor({ timeout: 5_000 });
      // This App reaches it: it can allow that folder there at once.
      await dialog.locator("[data-testid=allow-folder]", { hasText: "~/elsewhere" }).waitFor({ timeout: 10_000 });
      // (The footer's button follows a render later.)
      await page.waitForFunction(() => [...document.querySelectorAll(".app-modal-add button")].some((b) => b.textContent?.trim() === "开一个" && b.disabled), null, { timeout: 5_000 })
        .catch(() => { throw new Error("允许范围外的文件夹不能开"); });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-remote-new");
      await vpsFolder.fill("");
      await clickVisible(page, dialog.locator(".ws-computer", { hasText: "Claude Code" }));

      // An older computer cannot say what it has: upgrade it.
      await clickVisible(page, dialog.getByRole("tab", { name: "Office PC" }));
      await dialog.locator("[data-testid=info-too-old]").waitFor({ timeout: 10_000 });

      // Connecting a computer: it says this workspace is on the local
      // network only (a VPS cannot reach it), gives the command, and picks
      // the computer once it connects.
      await clickVisible(page, dialog.getByRole("tab", { name: "连一台电脑" }));
      // Over SSH (the default where the App runs): the hosts in the SSH
      // config, the one already connected marked; a key the App cannot use
      // is told in words with the command to try.
      const ssh = dialog.locator("[data-testid=connect-ssh]");
      await ssh.locator(".cx-row", { hasText: "gpu-box" }).waitFor({ timeout: 10_000 });
      if (!(await ssh.locator(".cx-row", { hasText: "tokyo-vps" }).innerText()).includes("连过")) throw new Error("已经连过的 SSH 主机应标出来");
      if (!(await ssh.locator(".cx-row", { hasText: "gpu-box" }).innerText()).includes("经 bastion")) throw new Error("经跳板机的主机应写出跳板");
      if (await ssh.locator(".cx-row", { hasText: "github" }).count()) throw new Error("只给 git 用的主机不该列出来");
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-connect-ssh");
      await clickVisible(page, ssh.locator(".cx-row", { hasText: "old-pi" }));
      await clickVisible(page, ssh.getByRole("button", { name: "连上" }));
      await ssh.locator(".cx-error", { hasText: "ssh-add" }).waitFor({ timeout: 10_000 });
      await ssh.locator(".cx-error .app-command code", { hasText: "ssh old-pi" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-connect-ssh-error");
      await clickVisible(page, ssh.getByRole("button", { name: "再连别的" }));
      // WSL on a Windows computer: its distributions, the default marked.
      await clickVisible(page, dialog.getByRole("tab", { name: "这台 Windows 里的 WSL" }));
      await dialog.locator("[data-testid=connect-wsl] .cx-row", { hasText: "Ubuntu-22.04" }).filter({ hasText: "默认" }).waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-connect-wsl");
      await clickVisible(page, dialog.getByRole("tab", { name: "复制命令" }));
      await dialog.locator("[data-testid=reach-note]", { hasText: "局域网" }).waitFor({ timeout: 10_000 });
      await dialog.locator("[data-testid=connect-computer] .app-command code", { hasText: "| bash" }).waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-connect");
      const ran = await page.evaluate(async () => (await fetch("/app/mock/run-connect", { method: "POST" })).status);
      if (ran !== 200) throw new Error(`run-connect: ${ran}`);
      await dialog.getByRole("tab", { name: /新服务器/, selected: true }).waitFor({ timeout: 20_000 });

      // Connecting one over SSH: each step ticks, then it is picked.
      await clickVisible(page, dialog.getByRole("tab", { name: "连一台电脑" }));
      await clickVisible(page, dialog.getByRole("tab", { name: "用 SSH 连" }));
      await clickVisible(page, ssh.locator(".cx-row", { hasText: "hk-vps" }));
      await clickVisible(page, ssh.getByRole("button", { name: "连上" }));
      await ssh.locator(".cx-step.is-running").first().waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "add-agent-connect-ssh-steps");
      await dialog.getByRole("tab", { name: "hk-vps", selected: true }).waitFor({ timeout: 30_000 });

      await clickVisible(page, dialog.getByRole("tab", { name: "云端或别处的 AI" }));
      await page.waitForFunction(() => {
        const t = document.querySelector(".add-sentence-text")?.textContent || "";
        return /\/j\/\S+\/\S+ \| bash/.test(t) && t.includes("最后的输出") && !t.includes("inbox wait");
      }, null, { timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-link");

      await clickVisible(page, dialog.getByRole("tab", { name: "这台电脑" }));
      await clickVisible(page, dialog.getByRole("tab", { name: "新开一个" }));
      // A newer computer makes ~/myco/<name> when no folder is given; model,
      // effort and access are under "more".
      await dialog.locator(".app-field-label", { hasText: "文件夹干活（可以不填）" }).waitFor({ timeout: 10_000 });
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
      // Claude's account sign-in has expired here: the card says so in
      // words and offers the ways this computer has; one tries again.
      const fix = dialog.locator("[data-testid=claude-auth-fix]");
      await fix.waitFor({ timeout: 10_000 });
      if (!(await fix.innerText()).includes("用 cc-switch 的「Kimi」")) throw new Error("登录不上时应列出 cc-switch 里的服务商");
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-claude-auth");
      await clickVisible(page, fix.getByRole("button", { name: /沿用终端里的 API 设置/ }));
      await dialog.locator(".add-arrived").nth(3).waitFor({ timeout: 20_000 });
      await clickVisible(page, dialog.getByRole("button", { name: "好了" }));
      await page.locator(".app-modal").waitFor({ state: "hidden" });
    });

    await step("会话 · 右键复制一份", async () => {
      // A Claude Code or Codex agent: right-click, 复制一份, a name, and the
      // copy's chat opens (the original carries on).
      try {
        await go(page, "官网改版", narrow);
        const who = narrow ? page.locator('.sp-strip-face[title="PM (Codex)"]') : page.locator(".sp-nav-person", { hasText: "PM (Codex)" });
        // (Scrolled into view first: a menu closes when the list scrolls.)
        const forkMenu = page.locator(".conv-menu").getByRole("menuitem", { name: "复制一份", exact: true });
        for (let i = 0; i < 3 && !(await forkMenu.isVisible()); i++) {
          await who.first().scrollIntoViewIfNeeded();
          await settle(page, 250);
          await who.first().click({ button: "right" });
          await forkMenu.waitFor({ timeout: 3_000 }).catch(() => {});
        }
        await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "复制一份", exact: true }));
        const dialog = page.locator("[data-testid=fork-dialog]");
        await dialog.waitFor({ timeout: 10_000 });
        const name = dialog.locator("input.app-input");
        if ((await name.inputValue()) !== "PM (Codex) 副本") throw new Error(`副本的名字默认应是「PM (Codex) 副本」，实际「${await name.inputValue()}」`);
        await settle(page, 200);
        await shot(page, size, scheme, "fork-dialog");
        await name.fill("PM 试验版");
        await clickVisible(page, dialog.getByRole("button", { name: "复制", exact: true }));
        await dialog.waitFor({ state: "detached", timeout: 20_000 });
        await page.getByText("PM 试验版").first().waitFor({ timeout: 20_000 });
      } finally {
        await page.evaluate(async () => { await fetch("/app/mock/forks", { method: "POST" }); });
      }
      // Back to the project as it was (the copy's chat closes with it).
      await go(page, "官网改版", narrow);
    });

    await step("这台电脑 · 一键修复", async () => {
      // This computer's part of myco that starts sessions is not connected:
      // the dialog says so (not "too old") and fixes it from the App.
      await page.evaluate(async () => { await fetch("/app/mock/no-spawn", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ on: true }) }); });
      try {
        if (narrow) await clickVisible(page, page.locator(".sp-strip-face.is-add"));
        else await clickVisible(page, page.locator(".sp-side .sp-side-add").first());
        const dialog = page.locator(".app-modal-add");
        await dialog.waitFor();
        await clickVisible(page, dialog.getByRole("tab", { name: "这台电脑" }));
        await clickVisible(page, dialog.getByRole("tab", { name: "新开一个" }));
        const fix = dialog.locator("[data-testid=info-no-spawn]");
        await fix.waitFor({ timeout: 30_000 });
        if ((await fix.innerText()).includes("myco upgrade")) throw new Error("这台电脑能在 App 里修，不该让人去终端");
        await settle(page, 300);
        await shot(page, size, scheme, "add-agent-fix-here");
        await clickVisible(page, fix.getByRole("button", { name: "修复" }));
        await fix.locator(".ca-text", { hasText: "已升级到 v0.20.24 并重启" }).waitFor({ timeout: 10_000 });
        // Back once the overview says it can start sessions again.
        await dialog.locator(".app-field-label", { hasText: "文件夹干活" }).waitFor({ timeout: 30_000 });
        await clickVisible(page, dialog.getByRole("button", { name: "取消" }));
        await page.locator(".app-modal").waitFor({ state: "hidden" });
      } finally {
        await page.evaluate(async () => { await fetch("/app/mock/no-spawn", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ on: false }) }); });
      }
    });

    if (!narrow) {
      await step("项目 · 添加工作空间", async () => {
        // Adding a computer is one command, as adding an agent is: run it in
        // the folder on that computer, and the dialog ticks itself; from a
        // project, the folder is in the project at once.
        await clickVisible(page, page.locator(".sp-side").getByRole("button", { name: "加一个终端" }));
        const dialog = page.locator(".app-modal-add");
        await dialog.waitFor();
        // With computers connected it starts at picking one; a computer not
        // connected yet is one command, run in the folder there.
        await clickVisible(page, dialog.getByRole("button", { name: "在还没连上的电脑上开" }));
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
        await clickVisible(page, page.locator(".sp-side").getByRole("button", { name: "加一个终端" }));
        await dialog.waitFor();
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

    // Opened again, the project's notes show at once from what was
    // loaded before (no loading placeholders while it asks again).
    await step("项目资料 · 再次打开立刻显示", async () => {
      await clickVisible(page, page.locator(".sp-head").getByRole("button", { name: "项目资料" }));
      await page.locator(".app-project-brief").first().waitFor({ timeout: 20_000 });
      await settle(page, 600);
      await navBackClick(page);
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".sp-head").getByRole("button", { name: "项目资料" }));
      await page.waitForTimeout(80);
      const state = await page.evaluate(() => ({
        brief: !!document.querySelector(".app-project-brief"),
        skeleton: document.querySelectorAll(".app-page .skel, .app-page [class*=skeleton], .app-page [class*=Skeleton]").length,
      }));
      if (!state.brief || state.skeleton) throw new Error(`再次打开项目资料应立刻显示上次的内容：${JSON.stringify(state)}`);
      await navBackClick(page);
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    await step("项目资料", async () => {
      await clickVisible(page, page.locator(".sp-head").getByRole("button", { name: "项目资料" }));
      await page.locator(".app-page").first().waitFor({ timeout: 20_000 });
      // The project's memory: its own section (not a row among the files),
      // and you add to it here.
      const memory = page.locator("[data-testid=project-memory]");
      await memory.getByText("汇报用中文").waitFor({ timeout: 10_000 });
      if (await page.locator(".app-project-doc", { hasText: /^memory/ }).count()) throw new Error("项目记忆不该再出现在文件列表里");
      await clickVisible(page, memory.getByRole("button", { name: "添加" }));
      const add = page.getByRole("dialog", { name: "添加到项目记忆" });
      await add.locator("textarea").fill("首页截图统一 2x");
      await clickVisible(page, add.getByRole("button", { name: "添加" }));
      await memory.getByText("首页截图统一 2x").waitFor({ timeout: 10_000 });
      await settle(page);
      await shot(page, size, scheme, "project-notes");
      await navBackClick(page);
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    await step("项目 · 邀请 agent", async () => {
      await go(page, "官网改版", narrow);
      await clickVisible(page, page.locator(".sp-head").getByRole("button", {name:"项目资料"}));
      await clickVisible(page, page.locator("[data-testid=project-tabs]").getByRole("tab", {name:/^成员/}));
      const section=page.locator("[data-testid=project-agent-invites]");
      await section.waitFor({timeout:20000});
      const toggle=section.getByRole("switch");
      if(await toggle.getAttribute("aria-checked")!=="false")throw new Error("Invitations should default off");
      await clickVisible(page,toggle);
      await page.waitForFunction(()=>document.querySelector("[data-testid=project-agent-invites] [role=switch]")?.getAttribute("aria-checked")==="true");
      await clickVisible(page,toggle);
      await page.waitForFunction(()=>document.querySelector("[data-testid=project-agent-invites] [role=switch]")?.getAttribute("aria-checked")==="false");
      await section.scrollIntoViewIfNeeded();await settle(page,200);
      await shot(page,size,scheme,"project-agent-invites");
      await navBackClick(page);
      await page.locator(".sp-home .conv-detail").waitFor({timeout:10000});
      const request=async()=>{
        const response=await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({agent:"pm-codex",kind:"project_invite",reason:"协助整理项目文档",payload:{invitation_id:"pinv_fixture",project:"website",project_title:"官网改版",display_name:"文档助手",description:"整理与校对项目文档",note:"文档"}})});
        if(!response.ok)throw new Error("Invitation mock request failed");
      };
      const card=page.locator(".sp-home [data-testid=approvals] .sa-approval");
      await request();await card.waitFor({timeout:25000});
      const text=await card.innerText();
      if(!text.includes("文档助手")||!text.includes("普通成员")||!text.includes("24 小时"))throw new Error("Approval must show invitee and limited permissions");
      await settle(page,200);await shot(page,size,scheme,"project-invite-request");
      await clickVisible(page,card.getByRole("button",{name:"不同意",exact:true}));await card.waitFor({state:"hidden"});
      await request();await card.waitFor({timeout:25000});
      await clickVisible(page,card.getByRole("button",{name:"同意",exact:true}));await card.waitFor({state:"hidden"});
    });

    // Which computers the project's coordinator may start agents on: a
    // setting of the project (项目资料), apart from its terminals; the
    // coordinator asks for another, and you approve it in its chat.
    await step("项目 · 可以开 agent 的电脑", async () => {
      await go(page, "官网改版", narrow);
      await clickVisible(page, page.locator(".sp-head").getByRole("button", { name: "项目资料" }));
      // (the computers are under 成员; the page opens on 资料)
      await clickVisible(page, page.locator("[data-testid=project-tabs]").getByRole("tab", { name: /^成员/ }));
      const section = page.locator("[data-testid=project-machines]");
      await section.waitFor({ timeout: 20_000 });
      const studio = section.locator(".app-row", { hasText: "Studio Mac" }).getByRole("switch");
      if ((await studio.getAttribute("aria-checked")) !== "false") throw new Error("项目一开始不该允许任何电脑");
      await clickVisible(page, studio);
      await page.waitForFunction(() => document.querySelector("[data-testid=project-machines] .app-row [role=switch][aria-checked=true]"), null, { timeout: 10_000 });
      await section.scrollIntoViewIfNeeded();
      await settle(page, 200);
      await shot(page, size, scheme, "project-machines");
      // The coordinator asks for another computer: a card in its chat.
      const ask = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent: "pm-codex", what: "让项目「website」能在「Office PC」上开 agent", reason: "要在 Windows 上开一个 Codex 测安装包。",
          method: "PUT", path: "/v1/projects/website/machines/m_office" }),
      });
      if (!ask.ok) throw new Error(`协调者申请电脑失败：HTTP ${ask.status}`);
      await navBackClick(page);
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 10_000 });
      const card = page.locator(".sp-home [data-testid=approvals] .sa-approval");
      await card.waitFor({ timeout: 25_000 });
      const text = compactText(await card.innerText());
      if (!text.includes("PM (Codex) 想") || !text.includes("Office PC")) throw new Error(`申请卡片应写明谁、要哪台电脑，实际「${text}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "project-machine-request");
      await clickVisible(page, card.getByRole("button", { name: "同意", exact: true }));
      await card.waitFor({ state: "hidden", timeout: 10_000 });
      await clickVisible(page, page.locator(".sp-head").getByRole("button", { name: "项目资料" }));
      await clickVisible(page, page.locator("[data-testid=project-tabs]").getByRole("tab", { name: /^成员/ }));
      const office = page.locator("[data-testid=project-machines] .app-row", { hasText: "Office PC" }).getByRole("switch");
      await page.waitForFunction(() => [...document.querySelectorAll("[data-testid=project-machines] .app-row")]
        .some((r) => r.textContent?.includes("Office PC") && r.querySelector("[role=switch]")?.getAttribute("aria-checked") === "true"), null, { timeout: 15_000 });
      if ((await office.getAttribute("aria-checked")) !== "true") throw new Error("同意后，项目应能在 Office PC 上开 agent");
      await navBackClick(page);
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
    });

    await step("agent 浏览器授权 · 版本快照 · 收回", async () => {
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 10_000 });
      const ask = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent: "pm-codex", kind: "action_grant", permissions: ["browser.view", "browser.run"],
          resource: { provider: "browser", id: "profile_reports", binding: "policy-1", name: "报告专用", summary: "Dedicated profile" },
          machine: "Studio Mac / 报告专用", what: "运行主人批准的报告脚本，5 分钟", reason: "运行已批准的月报 v3，不读取共享脚本的最新版本。" }),
      });
      if (!ask.ok) throw new Error(`浏览器申请失败：HTTP ${ask.status}`);
      const card = page.locator(".sp-home [data-testid=approvals] .sa-approval", { hasText: "报告专用" });
      await card.waitFor({ timeout: 25_000 });
      const text = compactText(await card.innerText());
      if (!text.includes("主人批准的脚本") || text.includes("键盘") || await card.getByRole("button", { name: "只让它看", exact: true }).count()) throw new Error(`浏览器申请不应使用桌面权限文案：${text}`);
      await shot(page, size, scheme, "browser-grant-request");
      await clickVisible(page, card.getByRole("button", { name: "同意", exact: true }));
      await card.waitFor({ state: "hidden", timeout: 10_000 });
      const grant = page.locator(".sp-home [data-testid=action-grants] .sa-grant", { hasText: "报告专用" });
      await grant.waitFor({ timeout: 10_000 });
      if (!(await grant.innerText()).includes("主人批准的脚本")) throw new Error("浏览器授权范围文案丢失");
      await shot(page, size, scheme, "browser-grant-active");
      await clickVisible(page, grant.getByRole("button", { name: "收回", exact: true }));
      await grant.waitFor({ state: "hidden", timeout: 10_000 });
    });

    await step("agent 浏览器实时控制授权", async () => {
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 10_000 });
      const ask = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent: "pm-codex", kind: "action_grant", permissions: ["browser.control"],
          resource: { provider: "browser", id: "profile_sample", binding: "policy-2", name: "示例浏览器" },
          machine: "Studio Mac / 示例浏览器", what: "操作浏览器，30 分钟", reason: "检查示例站点。" }),
      });
      if (!ask.ok) throw new Error(`浏览器实时控制申请失败：HTTP ${ask.status}`);
      const card = page.locator(".sp-home [data-testid=approvals] .sa-approval", { hasText: "示例浏览器" });
      await card.waitFor({ timeout: 25_000 });
      const copy = compactText(await card.innerText());
      for (const fragment of ["可能有你的登录状态", "显示在你的屏幕上", "随时可以停止", "30 分钟", "不限域名"]) {
        if (!copy.includes(fragment)) throw new Error(`控制授权卡缺少提示：${fragment}`);
      }
      await shot(page, size, scheme, "browser-control-grant");
      await clickVisible(page, card.getByRole("button", { name: "不同意", exact: true }));
      await card.waitFor({ state: "hidden", timeout: 10_000 });

      const limitedPersonal = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
        method:"POST",headers:{"Content-Type":"application/json"},
        body:JSON.stringify({agent:"pm-codex",kind:"action_grant",permissions:["browser.control"],
          resource:{provider:"browser",id:"profile_sample",binding:"policy-2",name:"示例浏览器"},hosts:["example.com"],reason:"检查示例站点。"}),
      });
      if(!limitedPersonal.ok)throw new Error("Scoped personal-profile request failed");
      await card.waitFor();
      const limitedText=await card.innerText();
      if(!limitedText.includes("只能在所选网站（example.com）中看到并使用这个配置的登录状态") || limitedText.includes("agent 能看到并操作这些网站"))throw new Error("Personal-profile warning broadened the allowed hosts: "+limitedText);
      await clickVisible(page,card.getByRole("button",{name:"不同意",exact:true}));await card.waitFor({state:"hidden"});

      const dedicated = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent: "pm-codex", kind: "action_grant", permissions: ["browser.control"],
          resource: { provider: "browser", id: "p_agent", binding: "policy-agent", name: "agent 专用", kind: "agent" },
          machine: "Studio Mac / agent 专用", hosts: ["example.com", "*.docs.example.com"], reason: "检查公开文档。" }),
      });
      if (!dedicated.ok) throw new Error(`专用浏览器申请失败：HTTP ${dedicated.status}`);
      const approval = await dedicated.json();
      const scoped = page.locator(".sp-home [data-testid=approvals] .sa-approval", { hasText: "操作浏览器配置「agent 专用」" });
      await scoped.waitFor({ timeout: 25_000 });
      if ((await scoped.innerText()).includes("可能有你的登录状态")) throw new Error("专用配置显示了个人配置提示");
      const exact = scoped.getByRole("checkbox", { name: "example.com", exact: true });
      const wildcard = scoped.getByRole("checkbox", { name: "*.docs.example.com", exact: true });
      const approve = scoped.getByRole("button", { name: "同意", exact: true });
      await shot(page, size, scheme, "browser-control-scoped-grant");
      await exact.uncheck();
      await wildcard.uncheck();
      if (await approve.isEnabled()) throw new Error("能把有域名限制的申请批准为不限域名");
      await wildcard.check();
      await clickVisible(page, approve);
      await scoped.waitFor({ state: "hidden", timeout: 10_000 });
      const grants = await page.evaluate(async () => {
        const hub = await fetch("/app/hubs").then(r => r.json());
        return fetch(`/app/hubs/${(hub.hubs || hub)[0].id}/action-grants?state=active`).then(r => r.json());
      });
      if (grants.grants.find(g => g.approval_id === approval.id)?.hosts?.join() !== "*.docs.example.com") {
        throw new Error("批准后仍包含已取消的域名");
      }
      // (taken back again, so the next step starts with no grant active)
      const scopedGrant = page.locator(".sp-home [data-testid=action-grants] .sa-grant", { hasText: "可以操作浏览器" });
      await scopedGrant.waitFor({ timeout: 15_000 });
      await clickVisible(page, scopedGrant.getByRole("button", { name: "收回", exact: true }));
      await scopedGrant.waitFor({ state: "detached", timeout: 10_000 });
    });

    // An agent asks to act on one of your computers (agent actions,
    // v0.21.21): the card says which computer and how, and can allow
    // seeing only; what it may do then shows above its chat, revocable.
    await step("agent 申请用电脑 · 只让它看 · 收回", async () => {
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 10_000 });
      const ask = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent: "pm-codex", kind: "action_grant", reason: "看一下构建窗口卡在哪一步。" }),
      });
      if (!ask.ok) throw new Error(`申请用电脑失败：HTTP ${ask.status}`);
      const card = page.locator(".sp-home [data-testid=approvals] .sa-approval", { hasText: "Studio Mac" });
      await card.waitFor({ timeout: 25_000 });
      const text = compactText(await card.innerText());
      if (!text.includes("看并操作") || !text.includes("构建窗口")) throw new Error(`申请卡片应写明要看并操作哪台电脑、为什么，实际「${text}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "action-grant-request");
      await clickVisible(page, card.getByRole("button", { name: "只让它看", exact: true }));
      await card.waitFor({ state: "hidden", timeout: 10_000 });
      const grant = page.locator(".sp-home [data-testid=action-grants] .sa-grant", { hasText: "可以看 Studio Mac" });
      await grant.waitFor({ timeout: 10_000 });
      const g = compactText(await grant.innerText());
      if (!g.includes("可以看 Studio Mac") || g.includes("操作")) throw new Error(`只让它看以后，应写「可以看 Studio Mac 的屏幕」，实际「${g}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "action-grant-active");
      await clickVisible(page, grant.getByRole("button", { name: "收回", exact: true }));
      await grant.waitFor({ state: "hidden", timeout: 10_000 });
      // A page it may export reads as that, not as a screen.
      const pg = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent: "pm-codex", kind: "page_grant", page: "产品白板" }),
      });
      if (!pg.ok) throw new Error(`页面导出授权失败：HTTP ${pg.status}`);
      const pgGrant = page.locator(".sp-home [data-testid=action-grants] .sa-grant", { hasText: "产品白板" });
      await pgGrant.waitFor({ timeout: 25_000 });
      const pt = compactText(await pgGrant.innerText());
      if (!pt.includes("可以导出「产品白板」") || pt.includes("屏幕")) throw new Error(`页面授权应写「可以导出「产品白板」」，实际「${pt}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "action-grant-page");
      await clickVisible(page, pgGrant.getByRole("button", { name: "收回", exact: true }));
      await pgGrant.waitFor({ state: "hidden", timeout: 10_000 });
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
      if (!order[0]?.includes("sp-nav-autos") || !order[1]?.includes("sp-nav-projects") || !order[2]?.includes("sp-nav-chats") || !order[3]?.includes("sp-nav-pcs")) throw new Error(`边栏应是自动化、项目、会话、终端，实际 ${order.join(" / ")}`);
      // The three headings look alike: one size and weight, no icon.
      const heads = await page.evaluate(() => [...document.querySelectorAll(".app-nav .sp-nav-head .sp-nav-title-link")].map((e) => {
        const st = getComputedStyle(e);
        return { text: (e.textContent || "").trim(), size: st.fontSize, icon: !!e.querySelector("svg") };
      }));
      if (heads.map((h) => h.text).join("|") !== "项目|会话|终端") throw new Error(`边栏标题应是 项目、会话、终端，实际 ${heads.map((h) => h.text).join(" / ")}`);
      // Automations and apps are rows on top, like the rows below, with an icon.
      const tops = await page.evaluate(() => [...document.querySelectorAll(".sp-nav-autos .sp-nav-top")].map((e) => ({ text: (e.querySelector(".app-nav-label")?.textContent || "").trim(), icon: !!e.querySelector("svg"), size: getComputedStyle(e).fontSize })));
      const row = await page.evaluate(() => getComputedStyle(document.querySelector(".sp-nav-projects .app-nav-item")).fontSize);
      if (tops.map((t) => t.text).join("|") !== "自动化|工作流|应用|电脑" || tops.some((t) => !t.icon || t.size !== row)) throw new Error(`顶上应是带图标的「自动化」「工作流」「应用」「电脑」四行，和项目一样大（用量在左下角的工作区菜单里）：${JSON.stringify(tops)} ${row}`);
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

    // The composer carries the account's tightest quota window beside the
    // model: amber under 20% left; a click shows every window, whose it is
    // and when it was read. Out of quota, a line above the box says so
    // before a send (and does not stop it).
    await step("单聊 · 输入框的额度", async () => {
      // (the project's home is the chat with its coordinator, PM)
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 20_000 });
      const pill = page.locator(".sp-home .conv-detail [data-testid=quota-pill]");
      await pill.waitFor({ timeout: 10_000 });
      const text = compactText(await pill.innerText());
      if (text !== "5h 剩 5%") throw new Error(`输入框的额度应写最紧的窗口「5h 剩 5%」，实际是「${text}」`);
      if (!(await pill.getAttribute("class") || "").includes("is-low")) throw new Error("剩不到 20% 应变黄");
      await pill.click();
      const menu = page.locator(".sp-home .quota-pill-menu");
      await menu.waitFor({ timeout: 5_000 });
      const all = compactText(await menu.innerText());
      if (!all.includes("每周") || !all.includes("剩 38%") || !all.includes("Studio Mac") || !all.includes("读取")) throw new Error(`点开应列出全部窗口、来源和读取时间，实际是「${all}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "chat-quota-pill");
      // It stays in the chat column (a members panel beside it, a narrow
      // window): moved left as far as it must, never cut off at the right.
      const inColumn = async () => menu.evaluate((el) => {
        const r = el.getBoundingClientRect();
        const b = el.closest(".conv-detail").getBoundingClientRect();
        return { ok: r.left >= b.left - 1 && r.right <= b.right + 1, r: [Math.round(r.left), Math.round(r.right)], b: [Math.round(b.left), Math.round(b.right)] };
      });
      let fit = await inColumn();
      if (!fit.ok) throw new Error(`额度弹窗应留在对话栏里，弹窗 ${fit.r}，对话栏 ${fit.b}`);
      if (!narrow) {
        // (a narrower window, the members panel still beside the chat)
        await page.keyboard.press("Escape");
        await page.locator(".sp-home .quota-pill-scrim").click({ force: true }).catch(() => {});
        await page.setViewportSize({ width: 860, height: size.height });
        try {
          await settle(page, 300);
          // (a chat opened beside the home by an earlier step: two panes
          // turn into tabs at this width, and the home's is not the one
          // shown; its tab brings it back)
          const homeTab = page.getByRole("tab", { name: /协调者/ });
          if (await homeTab.count() && !(await pill.isVisible())) {
            await homeTab.first().click();
            await settle(page, 300);
          }
          await pill.click();
          await menu.waitFor({ timeout: 5_000 });
          fit = await inColumn();
          if (!fit.ok) throw new Error(`窗口窄一些时，额度弹窗也应留在对话栏里，弹窗 ${fit.r}，对话栏 ${fit.b}`);
          await shot(page, { ...size, width: 860 }, scheme, "chat-quota-pill-narrow");
          await page.keyboard.press("Escape");
          await page.locator(".sp-home .quota-pill-scrim").click({ force: true }).catch(() => {});
        } finally {
          await page.setViewportSize({ width: size.width, height: size.height });
        }
        await settle(page, 300);
        await pill.click();
        await menu.waitFor({ timeout: 5_000 });
      }
      await page.keyboard.press("Escape");
      await page.locator(".sp-home .quota-pill-scrim").click({ force: true }).catch(() => {});
      if (await page.locator(".sp-home .conv-quota-out").count()) throw new Error("没用完时不该提示");
      // Review's account is out: said above the box, the box still sends.
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      const out = page.locator(".conv-detail .conv-quota-out");
      await out.waitFor({ timeout: 10_000 });
      if (!(await out.innerText()).includes("已用完")) throw new Error(`用完时应提示，实际是「${await out.innerText()}」`);
      if (await page.locator(".conv-detail .conv-compose-input").isDisabled()) throw new Error("用完也不该拦着发");
      if (!(await page.locator(".conv-detail [data-testid=quota-pill]").getAttribute("class") || "").includes("is-out")) throw new Error("用完应变红");
    });

    // A message on its way: a turning ring and dots under the faint
    // bubble. When a refresh brings it back before the send answers (a
    // slow workspace), it shows once, not twice.
    await step("单聊 · 发送中", async () => {
      // (one at rest: what you send is a new message, not an addition)
      await openMember(page, narrow, "聊天", "MCP Claude");
      const box0 = page.locator(".conv-detail .conv-compose-input");
      // (a chat with something in it, the agent done with it: the next one
      // is a new message, sent while the chat refreshes)
      if (!(await page.locator(".conv-detail .conv-msg.is-me").count())) {
        await box0.fill("先打个招呼");
        await box0.press("Enter");
        await page.locator(".conv-detail .conv-msg.is-me", { hasText: "先打个招呼" }).first().waitFor({ timeout: 15_000 });
      }
      await page.waitForFunction(() => !/排到下一条/.test(document.querySelector(".conv-detail .conv-compose-input")?.getAttribute("placeholder") || ""), null, { timeout: 60_000 });
      const text = `发送中只出现一次 ${size.width} ${scheme}`;
      const slowSend = async (route) => {
        if (route.request().method() !== "POST") return route.continue();
        const resp = await route.fetch();
        await new Promise((ok) => setTimeout(ok, 7000));
        await route.fulfill({ response: resp }).catch(() => {});
      };
      await page.route("**/app/hubs/*/messages", slowSend);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill(text);
      await box.press("Enter");
      const sending = page.locator(".conv-detail [data-testid=chat-sending]");
      await sending.waitFor({ timeout: 5_000 });
      if (!(await sending.locator(".send-ring").count())) throw new Error("发送中应有转圈");
      await settle(page, 300);
      await shot(page, size, scheme, "chat-sending");
      // The refresh brings it (the send has not answered yet): one bubble.
      await page.locator(".conv-detail .conv-msg.is-me", { hasText: text }).first().waitFor({ timeout: 10_000 });
      for (let i = 0; i < 4; i++) {
        const n = await page.locator(".conv-detail .conv-msg", { hasText: text }).count();
        if (n !== 1) throw new Error(`同一条消息应只出现一次，实际 ${n} 次`);
        await page.waitForTimeout(500);
      }
      await page.waitForFunction(() => !document.querySelector(".conv-detail [data-testid=chat-sending]"), null, { timeout: 15_000 });
      await page.unroute("**/app/hubs/*/messages", slowSend);
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

    await step("单聊 · 等待电脑名额", async () => {
      await openMember(page,narrow,"聊天","Review (Codex 云端)");
      await noRound(page);
      const box=page.locator(".conv-detail .conv-compose-input");
      await box.fill("名额排队预览");await box.press("Enter");
      const queue=page.locator('[data-testid="runner-queue"]').last();
      await queue.waitFor({timeout:15000});
      if (!(await queue.innerText()).includes("3/3 在忙，排第 2")) throw Error("未显示名额和排队位置");
      if (await page.locator('.conv-detail .trace.is-live').count()) throw Error("等名额时不能假装正在启动");
      if (await page.locator(".conv-detail .conv-stuck").count()) throw Error("等待名额20分钟的任务不能误报卡住");
      await shot(page,size,scheme,"runner-queue");
      await go(page,"电脑",narrow);
      await clickVisible(page,page.locator(".pc-card",{hasText:"Studio Mac"}));
      const slots=page.locator('[data-testid="computer-runner-slots"]');await slots.waitFor();
      if (!(await slots.innerText()).includes("3/3 在忙，2 个排队中") || !(await slots.innerText()).includes("myco runner service install --max-concurrent N")) throw Error("电脑详情未显示名额和调整命令");
      await shot(page,size,scheme,"computer-runner-slots");
      await navBackClick(page);
      await openMember(page,narrow,"聊天","Review (Codex 云端)");
      await fetch(`http://${HOST}:${mockPort()}/app/mock/runner-queue`,{method:"DELETE"});
      await page.locator('[data-testid="runner-queue"]').waitFor({state:"hidden",timeout:15000});
      await page.locator(".conv-detail .conv-send.is-stop").click();
      await noRound(page);
      await clickVisible(page,page.locator(".conv-head button").first());
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
      // The answer is in (20 s after it was sent): the whole process folds
      // to one line, steps and time, and opening that line shows the steps.
      await page.locator(".conv-detail .conv-msg", { hasText: "预览检查完成。" }).last().waitFor({ timeout: 25_000 });
      await page.waitForFunction(() => !document.querySelector(".conv-detail .trace.is-live, .conv-detail .trace.is-waiting"), null, { timeout: 10_000 });
      const folded = await run.evaluate((el) => {
        const btn = el.querySelector(".trace-fold");
        return { has: !!btn, open: btn?.getAttribute("aria-expanded"),
          head: btn?.querySelector(".trace-title")?.textContent || "" };
      });
      if (!folded.has) throw new Error("回复到了之后，过程应有可折叠的一行");
      if (folded.open !== "false") throw new Error("回复到了之后，过程应默认收起");
      if (!/^(过程 · \d+ 步 · |Steps · \d+ · )/.test(folded.head)) throw new Error(`收起的过程应写明步数和用时，实际是「${folded.head}」`);
      await shot(page, size, scheme, "conversation-process-folded");
      await run.locator(".trace-fold").click();
      await page.waitForFunction(() => [...document.querySelectorAll(".conv-detail .trace")].pop()
        ?.querySelector(".trace-fold")?.getAttribute("aria-expanded") === "true", null, { timeout: 5_000 });
      const inside = await run.evaluate((el) => ({
        steps: el.querySelectorAll(".trace-group, .trace-row, .trace-say").length,
        live: el.classList.contains("is-live"),
      }));
      if (inside.steps === 0) throw new Error("点开收起的过程后应能看到里面的步骤");
      if (inside.live) throw new Error("回复到了之后不该还有在动的过程");
      await shot(page, size, scheme, "conversation-process-opened");
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
      // In time: sent after the newer task's answer, it is drawn there (at
      // the foot, quoting the older task it adds to), not back up inside
      // the older task's run above that answer.
      const added = page.locator(".conv-detail .conv-msg.is-me", { hasText: "再检查旧任务的链接" }).last();
      await added.waitFor({ timeout: 10000 });
      if (!(await added.innerText()).includes("保留任务")) throw new Error("补充给旧任务的话应引用那条旧任务");
      const below = await page.evaluate(([a, r]) => {
        const add = [...document.querySelectorAll(".conv-detail .conv-msg.is-me")].filter((e) => e.textContent.includes(a)).pop();
        const rep = document.querySelector(`[data-msg="${r}"]`);
        return !!add && !!rep && !!(rep.compareDocumentPosition(add) & Node.DOCUMENT_POSITION_FOLLOWING);
      }, ["再检查旧任务的链接", reply.id]);
      if (!below) throw new Error("新任务回答之后才补充的话，应排在那条回答下面");
      await shot(page, size, scheme, "conversation-supplement-older-open");
      // What the older task does after that (a long session goes on filing
      // its steps under it) goes on under what you added, never back up
      // above it, in the newer task's run (where it showed before).
      const after = new Date(Date.now() + 500).toISOString();
      const traceURL = `**/app/hubs/hub_studio/messages/${older.id}/trace*`;
      await page.route(traceURL, async (route) => {
        const response = await route.fetch();
        const json = await response.json();
        const seq = Math.max(0, ...(json.events || []).map((e) => e.seq)) + 1;
        json.events = [...(json.events || []),
          { seq, kind: "step", at: after, data: { kind: "text", text: "补充之后接着查旧任务的链接" } },
          { seq: seq + 1, kind: "step", at: after, data: { kind: "tool", tool: "Bash", id: "after-steer", input: "curl -I https://example.com" } }];
        await route.fulfill({ response, json });
      });
      try {
        const later = page.locator(".conv-detail .trace-say", { hasText: "补充之后接着查旧任务的链接" });
        await later.first().waitFor({ timeout: 15_000 });
        const order = await page.evaluate(([a]) => {
          const add = [...document.querySelectorAll(".conv-detail .conv-msg.is-me")].filter((e) => e.textContent.includes(a)).pop();
          const says = [...document.querySelectorAll(".conv-detail .trace-say")].filter((e) => e.textContent.includes("补充之后接着查旧任务的链接"));
          return { n: says.length, below: !!add && says.every((x) => !!(add.compareDocumentPosition(x) & Node.DOCUMENT_POSITION_FOLLOWING)) };
        }, ["再检查旧任务的链接"]);
        if (order.n !== 1 || !order.below) throw new Error(`补充之后旧任务做的事应接在补充下面，只出现一处（实际 ${order.n} 处，${order.below ? "在下面" : "有在上面的"}）`);
        await shot(page, size, scheme, "conversation-supplement-older-goes-on");
      } finally {
        await page.unroute(traceURL);
      }
      await fetch(`http://${HOST}:${mockPort()}/app/hubs/hub_studio/messages/${older.id}/stop`, { method: "POST" });
      await noRound(page);
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // A chat with nothing in it yet: the agent, large, and a question.
    // What was typed and not sent stays with its chat when you go to
    // another and come back; an empty box is one line, whatever its hint.
    // A note in your name (a workflow sends one when it is done) that comes
    // while your task is still at work: what the agent does after it goes
    // on under the note, never above it.
    await step("单聊 · 运行中来的备注", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill("保留任务：运行中来一条备注");
      await box.press("Enter");
      const mine = page.locator(".conv-detail .conv-msg.is-me", { hasText: "保留任务：运行中来一条备注" }).last();
      await mine.waitFor({ timeout: 10_000 });
      await page.waitForFunction(() => {
        const els = [...document.querySelectorAll(".conv-detail .conv-msg.is-me[data-msg]")].filter((e) => e.textContent.includes("保留任务：运行中来一条备注"));
        return els.length && !els[els.length - 1].getAttribute("data-msg").startsWith("pending");
      }, null, { timeout: 10_000 });
      const task = { id: await mine.getAttribute("data-msg") };
      await sleep(600);
      const note = await sendFromElsewhere("hub_studio", "review-cloud", "[工作流「发版演练」完成了] 运行 run_smoke", { type: "note", of: task.id });
      const noteEl = page.locator(`.conv-detail [data-msg="${note.id}"]`);
      await noteEl.waitFor({ timeout: 15_000 });
      const after = new Date(new Date(note.created_at).getTime() + 500).toISOString();
      const traceURL = `**/app/hubs/hub_studio/messages/${task.id}/trace*`;
      await page.route(traceURL, async (route) => {
        const response = await route.fetch();
        const json = await response.json();
        const seq = Math.max(0, ...(json.events || []).map((e) => e.seq)) + 1;
        json.events = [...(json.events || []),
          { seq, kind: "step", at: after, data: { kind: "text", text: "备注之后接着做的事" } },
          { seq: seq + 1, kind: "step", at: after, data: { kind: "tool", tool: "Bash", id: "after-note", input: "git status" } }];
        await route.fulfill({ response, json });
      });
      try {
        await page.locator(".conv-detail .trace-say", { hasText: "备注之后接着做的事" }).first().waitFor({ timeout: 15_000 });
        const order = await page.evaluate((id) => {
          const n = document.querySelector(`.conv-detail [data-msg="${id}"]`);
          const says = [...document.querySelectorAll(".conv-detail .trace-say")].filter((e) => e.textContent.includes("备注之后接着做的事"));
          return { n: says.length, below: !!n && says.every((x) => !!(n.compareDocumentPosition(x) & Node.DOCUMENT_POSITION_FOLLOWING)) };
        }, note.id);
        if (order.n !== 1 || !order.below) throw new Error(`备注之后对方做的事应接在备注下面，只出现一处（实际 ${order.n} 处，${order.below ? "在下面" : "有在上面的"}）`);
        await settle(page, 200);
        await shot(page, size, scheme, "chat-note-mid-run");
      } finally {
        await page.unroute(traceURL);
        await fetch(`http://${HOST}:${mockPort()}/app/hubs/hub_studio/messages/${task.id}/stop`, { method: "POST" });
      }
      await noRound(page);
    });

    // An agent's own name in a message (cloud-7, review-cloud) reads as its
    // name for people and opens its chat; not in a path or in code.
    await step("单聊 · agent 名字变成链接", async () => {
      // (and a short name people write: cloud-7 for cloud-7-claude-rmv6)
      const routeURL = /\/app\/overview(\?|$)/;
      await page.route(routeURL, async (route) => {
        const response = await route.fetch();
        const json = await response.json();
        const list = (json.hubs || []).find((h) => h.id === "hub_studio")?.overview?.agents;
        const model = list?.find((x) => x.agent?.kind === "agent");
        if (list && model && !list.some((x) => x.agent.name === "cloud-7-claude-rmv6")) {
          list.push({ ...model, agent: { ...model.agent, id: "ag_cloud7", name: "cloud-7-claude-rmv6", display_name: "移动端工程师", avatar: undefined } });
        }
        await route.fulfill({ response, json });
      });
      await page.reload({ waitUntil: "domcontentloaded" });
      await page.locator(".app-shell").waitFor();
      try {
      await openMember(page, narrow, "聊天", "Myco (Grok)");
      const anchor = await page.locator(".conv-detail .conv-msg[data-msg]").last().getAttribute("data-msg");
      const note = await sendFromElsewhere("hub_studio", "grok-agent-7c2e", "请交给 review-cloud 看看，@pm-codex 也看一下，再转给 cloud-7。路径 ~/code/review-cloud/x 和 `review-cloud` 保持原样。", { type: "note", of: anchor });
      const bubble = page.locator(`.conv-detail [data-msg="${note.id}"]`);
      await bubble.waitFor({ timeout: 15_000 });
      const links = bubble.locator("a.md-agent");
      await links.first().waitFor({ timeout: 10_000 });
      const got = await links.evaluateAll((els) => els.map((a) => [a.textContent, a.getAttribute("title")]));
      if (JSON.stringify(got) !== JSON.stringify([["Review (Codex 云端)", "review-cloud"], ["PM (Codex)", "pm-codex"], ["移动端工程师", "cloud-7-claude-rmv6"]])) throw new Error(`正文里的 agent 名字应换成它的名字并做成链接，实际 ${JSON.stringify(got)}`);
      const text = await bubble.innerText();
      if (!text.includes("~/code/review-cloud/x")) throw new Error(`路径里的名字不该替换，实际「${text}」`);
      if ((await bubble.locator("code").innerText()) !== "review-cloud") throw new Error("代码里的名字不该替换");
      if (text.includes("@pm-codex") || text.includes("@PM")) throw new Error(`@名字应整个换成链接，实际「${text}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "chat-agent-name-links");
      await links.first().click();
      await page.locator(".conv-detail .conv-head", { hasText: "Review (Codex 云端)" }).first().waitFor({ timeout: 15_000 });
      } finally {
        await page.unroute(routeURL);
        await page.reload({ waitUntil: "domcontentloaded" });
        await page.locator(".app-shell").waitFor();
      }
    });

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
      // Files not sent yet stay with their chat, as typed words do: to
      // another chat (its box has none) and back, they are still there.
      await openMember(page, narrow, "聊天", "legacy-codex");
      await page.locator(".conv-detail .conv-head h2", { hasText: "legacy-codex" }).waitFor({ timeout: 10_000 });
      if (await page.locator(".conv-compose-box .att-chip").count()) throw new Error("别的会话的输入框不该带着这里没发的文件");
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await box.locator(".att-chip").nth(2).waitFor({ timeout: 10_000 });
      if ((await box.locator(".att-chip").count()) !== 3) throw new Error("切走再回来，没发的文件应还在");
      await page.waitForFunction(() => { const img = [...document.querySelectorAll(".conv-compose-box .att-chip.is-picture .att-thumb")].pop(); return !!img && img.complete && img.naturalWidth > 0; }, null, { timeout: 5_000 })
        .catch(() => { throw new Error("切回来后图片缩略图应还能显示"); });
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
      // The plan of the task at work is pinned at the top of the chat, by
      // itself: one line with how far it is, opened to the list, folded
      // back, and closed for this task.
      const bar = page.locator(".conv-detail [data-testid=plan-bar]");
      await bar.waitFor({ timeout: 10_000 });
      await page.waitForFunction(() => /计划 2\/3/.test(document.querySelector(".conv-detail [data-testid=plan-bar]")?.textContent || ""), null, { timeout: 10_000 });
      // (正在 while it runs; 下一步 once the mock's short task is over)
      if (!/(正在|下一步)：跑测试/.test(await bar.innerText())) throw new Error(`置顶的计划应说做到哪一步，实际「${await bar.innerText()}」`);
      const barTop = await bar.evaluate((el) => el.getBoundingClientRect().top);
      const scrollTop = await page.locator(".conv-detail .conv-scroll").evaluate((el) => el.getBoundingClientRect().top);
      if (barTop > scrollTop) throw new Error("置顶的计划应在聊天内容上面，不随聊天滚动");
      await bar.locator(".plan-bar-toggle").click();
      await bar.locator(".trace-plan-item", { hasText: "动手改" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "chat-plan-bar");
      await bar.locator(".plan-bar-toggle").click();
      await bar.locator(".trace-plan").waitFor({ state: "detached", timeout: 5_000 });
      // Once the task is over with a step still open, the plan stays
      // pinned, saying what is next (a coordinator's board outlives a task).
      await page.waitForFunction(() => /下一步：跑测试/.test(document.querySelector(".conv-detail [data-testid=plan-bar]")?.textContent || ""), null, { timeout: 20_000 })
        .catch(() => { throw new Error("任务结束后还有没做完的计划，应继续置顶并写「下一步：…」"); });
      await page.waitForTimeout(9000);
      if (!(await bar.count())) throw new Error("没做完的计划不该自己消失");
      await bar.locator(".plan-bar-close").click();
      await bar.waitFor({ state: "detached", timeout: 5_000 });
      await page.waitForTimeout(2500);
      if (await page.locator(".conv-detail [data-testid=plan-bar]").count()) throw new Error("关掉的计划不该再置顶回来");
      await clickVisible(page, page.locator(".conv-head button").first());
    });

    // A poor network: the chat's polls get no answer. After a few polls
    // it says so (the agent may well be at work) instead of calling a
    // task stuck, and the notice goes once answers come again.
    await step("单聊 · 网络不好", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const polls = /\/app\/hubs\/[^/]+\/(exchanges|thread)/;
      // Nothing at work: however long the polls go unanswered, no notice
      // (it has nothing more to say).
      await page.route(polls, (route) => route.abort());
      try {
        await page.waitForTimeout(22_000);
        if (await page.locator("[data-testid=chat-behind]").count()) throw new Error("没有在干活的任务时，不该提示网络不好");
      } finally {
        await page.unroute(polls);
      }
      // At work, then the polls go unanswered: it says the network is poor.
      const box = page.locator(".conv-detail .conv-compose-input");
      const said = page.locator(".conv-detail .trace-reports .trace-report", { hasText: "正在处理" });
      const before = await said.count();
      await box.fill("把发布分支同步一下 main");
      await box.press("Enter");
      // (at work: it has picked this one up and said something; an earlier
      // task's progress does not count)
      await page.waitForFunction((n) => document.querySelectorAll(".conv-detail .trace-reports .trace-report").length > 0
        && [...document.querySelectorAll(".conv-detail .trace-reports .trace-report")].filter((e) => e.textContent?.includes("正在处理")).length > n, before, { timeout: 20_000 });
      await page.route(polls, (route) => route.abort());
      try {
        await page.locator("[data-testid=chat-behind]").waitFor({ timeout: 40_000 });
        if (await page.locator(".conv-detail .conv-stuck").count()) throw new Error("网络不好时不该说任务卡住");
        // (in the chat's column: as wide as the input box, not the pane)
        {
          const note = await page.locator("[data-testid=chat-behind]").boundingBox();
          const box = await page.locator("[data-testid=chat-behind]").locator("xpath=..").locator(".conv-compose-box").boundingBox();
          if (!note || !box || Math.abs(note.x - box.x) > 1 || Math.abs(note.width - box.width) > 1) {
            throw new Error(`「网络不太好」应和输入框一样宽、对齐：提示 ${Math.round(note?.x)}+${Math.round(note?.width)}，输入框 ${Math.round(box?.x)}+${Math.round(box?.width)}`);
          }
        }
        await settle(page, 150);
        await shot(page, size, scheme, "chat-behind");
      } finally {
        await page.unroute(polls);
      }
      await page.locator("[data-testid=chat-behind]").waitFor({ state: "detached", timeout: 20_000 });
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
      // A log reads as plain text. While it comes (slowly here), the
      // mushroom with its magnifying glass looks for it.
      const slowFile = async (route) => {
        if (route.request().url().includes("as=text")) await new Promise((ok) => setTimeout(ok, 1500));
        await route.continue();
      };
      await page.route("**/files/**", slowFile);
      await page.locator(".conv-detail button.conv-attach:not(.is-image)", { hasText: "build.log" }).last().click();
      const fileLoading = doc.locator("[data-testid=file-loading]");
      await fileLoading.waitFor({ timeout: 5_000 });
      if (!(await fileLoading.locator("svg.seek-sprite").count())) throw new Error("打开文件时应有拿放大镜找东西的蘑菇");
      await shot(page, size, scheme, "file-loading");
      await doc.locator(".fp-code", { hasText: "BUILD SUCCESSFUL" }).waitFor({ timeout: 5_000 });
      await page.unroute("**/files/**", slowFile);
      await page.keyboard.press("Escape");
      await doc.waitFor({ state: "hidden", timeout: 5_000 });
      // Every one of them answered before the next step uses this chat.
      await page.waitForFunction(() => {
        const d = document.querySelector(".conv-detail");
        return !!d && d.querySelectorAll(".trace.is-live, .trace.is-waiting").length === 0;
      }, null, { timeout: 45_000 });
      // A screenshot sent with a progress update shows under that update
      // (in the run's process: answered, it is folded to one line first).
      await revealRuns(page);
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
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "文件和应用" }));
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

    // 聊天背景: from the header's ⋯, for this chat or as the default; the
    // chat shows it at once, filling its pane; back to none afterwards.
    await step("单聊 · 聊天背景", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      const chat = page.locator(".conv-detail").first();
      await clickVisible(page, chat.locator("[data-testid=chat-more]"));
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "聊天背景" }));
      const dlg = page.locator(".cbg-dialog");
      await dlg.waitFor({ timeout: 5_000 });
      await clickVisible(page, dlg.locator(".cbg-tile", { hasText: "点阵" }));
      await chat.locator(".conv-bg .cbg-layer").waitFor({ timeout: 5_000 });
      if (!(await chat.evaluate((el) => el.classList.contains("has-bg")))) throw new Error("选了背景，对话应马上换上");
      // The default, for every conversation without its own.
      await clickVisible(page, dlg.getByRole("tab", { name: "所有对话的默认" }));
      await clickVisible(page, dlg.locator(".cbg-tile", { hasText: "晨雾" }));
      await settle(page, 300);
      await shot(page, size, scheme, "chat-background-dialog");
      await clickVisible(page, dlg.getByRole("button", { name: "好" }));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "chat-background");
      // Another conversation has the default.
      await openMember(page, narrow, "聊天", "Myco (Grok)");
      await page.locator(".conv-detail.has-bg").first().waitFor({ timeout: 5_000 });
      // Back to none, for the steps after this one.
      await page.evaluate(() => { localStorage.removeItem("myco-chat-bg"); window.dispatchEvent(new CustomEvent("myco-chat-bg-changed")); });
      await page.locator(".conv-detail.has-bg").waitFor({ state: "detached", timeout: 5_000 });
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
      await page.locator(".conv-detail .conv-msg.is-me .conv-receipt", { hasText: "暂挂：在等其他 agent 回复" }).last().waitFor({ timeout: 15_000 });
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
      // Done, the process folds, and what you added is not folded into it:
      // it stays out, the steps before it folded on their own line above.
      await page.waitForFunction(() => {
        const el = [...document.querySelectorAll(".conv-detail .trace-steer")].find((x) => (x.textContent || "").includes("顺便看看 Windows 的包"));
        return !!el && !el.closest(".trace")?.classList.contains("is-live");
      }, null, { timeout: 10_000 }).catch(() => undefined);
      if (!(await added.isVisible())) throw new Error("过程折起后，你在运行中补充的话不该被收进过程里");
      const folds = await added.evaluate((el) => {
        const run = el.closest(".trace");
        const y = el.getBoundingClientRect().top;
        const heads = [...(run?.querySelectorAll(".trace-fold") || [])];
        return { above: heads.filter((h) => h.getBoundingClientRect().top < y).map((h) => (h.textContent || "").trim()), shut: heads.every((h) => h.getAttribute("aria-expanded") === "false") };
      });
      if (!folds.above.length || !/过程 · \d+ 步/.test(folds.above[0])) throw new Error(`补充之前的步骤应单独折成一行在它上面，实际 ${JSON.stringify(folds.above)}`);
      if (!folds.shut) throw new Error("做完后过程应折起");
      await added.scrollIntoViewIfNeeded();
      await shot(page, size, scheme, "chat-supplement-after-done");
    });

    // A line you add after something came in between (another member
    // handed it work: a card) is drawn where you sent it; what the agent
    // does after it goes under it, in time, not on growing above it.
    await step("项目 · 插话排在后面", async () => {
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 20_000 });
      // Earlier scenes may leave multiple panes open: even a wide window
      // then shows member faces rather than the expanded members column.
      const member = page.locator(".sp-side .sp-member:visible", { hasText: "Release (Claude 云端)" })
        .or(page.locator('.sp-side .sp-face:visible[aria-label="Release (Claude 云端)"]'))
        .or(page.locator('.sp-strip-face:visible[title="Release (Claude 云端)"]'));
      await clickVisible(page, member);
      const pane = page.locator(".conv-detail", { has: page.locator(".conv-head", { hasText: "Release (Claude 云端)" }) }).last();
      await pane.waitFor({ timeout: 20_000 });
      // The coordinator's pane can be idle while this member is still
      // working on the task sent by the page-bridge scene.
      await pane.locator('.conv-compose-input:not([placeholder*="补充"])').waitFor({ timeout: 30_000 });
      const box = pane.locator(".conv-compose-input");
      await box.fill("插在别人后面：整理发布说明");
      await box.press("Enter");
      await pane.locator(".conv-msg.is-me", { hasText: "插在别人后面：整理发布说明" }).waitFor({ timeout: 10_000 });
      await pane.locator(".handoff", { hasText: "顺手看一下 CI 为什么挂了" }).waitFor({ timeout: 20_000 });
      await box.fill("插话：顺便把下载链接也更新了");
      await box.press("Enter");
      const added = pane.locator(".conv-msg.is-me", { hasText: "插话：顺便把下载链接也更新了" });
      await added.waitFor({ timeout: 10_000 });
      await pane.locator(".trace-report", { hasText: "收到补充" }).waitFor({ timeout: 20_000 });
      await page.waitForTimeout(3000); // (a few more steps after it)
      const order = await added.evaluate((el) => {
        const y = el.getBoundingClientRect().top;
        const report = [...el.closest(".conv-detail").querySelectorAll(".trace-report")].find((r) => r.textContent?.includes("收到补充"));
        const under = el.nextElementSibling?.classList.contains("trace") ? el.nextElementSibling : null;
        // the task's own run, above: its last step before the line you added
        const runs = [...el.closest(".conv-detail").querySelectorAll(".trace")].filter((t) => t !== under && t.getBoundingClientRect().top < y);
        return {
          reportBelow: !!report && report.getBoundingClientRect().top > y,
          reportUnder: !!under && !!report && under.contains(report),
          stepsUnder: !!under && under.querySelectorAll(".trace-group, .trace-step, .trace-row").length > 0,
          loadingAbove: runs.some((t) => t.classList.contains("is-live")),
        };
      });
      if (!order.reportBelow) throw new Error("补充之后它说的话应在补充下面");
      if (!order.reportUnder) throw new Error("补充之后的进展应接在补充下面的过程里");
      if (!order.stepsUnder) throw new Error("补充之后它做的步骤应在补充下面");
      if (order.loadingAbove) throw new Error("补充上面的过程不该还在转");
      await added.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "chat-addition-after-card");
      await page.locator(".conv-detail .conv-msg", { hasText: "发布说明整理好了" }).last().waitFor({ timeout: 30_000 });
      await noRound(page);
      // Only work in progress stands out on a card: done and handed back
      // are grey, like the words of a finished card.
      const tones = await page.evaluate(() => {
        const probe = document.createElement("span");
        probe.style.color = "var(--muted)";
        document.querySelector(".conv-detail")?.appendChild(probe);
        const muted = getComputedStyle(probe).color;
        probe.remove();
        return [...document.querySelectorAll(".handoff-state.is-done")].map((x) => getComputedStyle(x).color === muted);
      });
      if (tones.some((grey) => !grey)) throw new Error("「做完了 / 交回了」应是灰色，只有「在做」显眼");
      // A card opens the two agents' exchange; its back button returns to
      // the chat the card was in.
      await clickVisible(page, pane.locator(".handoff", { hasText: "顺手看一下 CI 为什么挂了" }));
      const backBtn = page.getByRole("button", { name: "返回和 Release (Claude 云端) 的对话" }).first();
      await backBtn.waitFor({ timeout: 10_000 });
      await clickVisible(page, backBtn);
      await page.locator(".conv-detail .conv-msg.is-me", { hasText: "插话：顺便把下载链接也更新了" }).last().waitFor({ timeout: 10_000 });
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
      // (only the workspace you are in is searched: no other one is asked)
      const asked = new Set();
      const onReq = (req) => { const m = /\/app\/hubs\/([^/]+)\/recent/.exec(req.url()); if (m) asked.add(decodeURIComponent(m[1])); };
      page.on("request", onReq);
      // (a slow search: the mushroom with its magnifying glass, not a bare line)
      const slow = async (route) => {
        await new Promise((ok) => setTimeout(ok, 1500));
        try { await route.continue(); }
        catch (error) {
          // A cancelled search may have completed this route while delayed.
          // (Playwright words it with or without a "!" across versions.)
          if (!/Route is already handled/i.test(String(error))) throw error;
        }
      };
      await page.route("**/app/hubs/*/recent*", slow);
      await cp.locator("input").fill("芒果布丁");
      const seek = cp.locator(".cp-seek");
      await seek.waitFor({ timeout: 3_000 });
      if (!(await seek.locator("svg.seek-sprite").count())) throw new Error("搜索中应有拿放大镜的蘑菇");
      if (!/芒果布丁|菌丝|成员|消息|快找到/.test(await seek.innerText())) throw new Error(`搜索中的提示不对：${await seek.innerText()}`);
      await settle(page, 300);
      await shot(page, size, scheme, "search-seeking");
      const msg = cp.locator(".cp-item", { hasText: "芒果布丁" }).first();
      await msg.waitFor({ timeout: 8_000 });
      await page.unroute("**/app/hubs/*/recent*", slow);
      if (await seek.count()) throw new Error("找到以后蘑菇应该收起来");
      page.off("request", onReq);
      if ([...asked].some((h) => h !== "hub_studio")) throw new Error(`⌘K 只该在当前工作区里搜，实际还问了：${[...asked].join("、")}`);
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
    // a waiting receipt under your message and a plain compose box.
    await step("单聊 · 挂起后说明在等", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      const box = page.locator(".conv-detail .conv-compose-input");
      await box.fill(`挂起说明：定时和 Webhook 先做第一步（${size.width}-${scheme}）`);
      await box.press("Enter");
      await page.locator(".conv-detail .trace-report", { hasText: "已派给 release" }).last().waitFor({ timeout: 15_000 });
      await page.locator(".conv-detail .conv-receipt", { hasText: "暂挂：在等其他 agent 回复" }).last().waitFor({ timeout: 10_000 });
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
      await page.locator(".conv-detail .conv-receipt", { hasText: "暂挂中，没人在做" }).last().waitFor({ timeout: 15_000 });
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
      if (!(await page.locator(".app-settings-tab", { hasText: "工作区" }).count())) throw new Error("简单模式下设置里也该有「工作区」");
      if (await page.locator(".app-settings-tab", { hasText: "成员" }).count()) throw new Error("简单模式下设置里不该有「成员」");
      if (!narrow && await page.locator(".sp-nav-pcs").count()) throw new Error("简单模式下侧栏不该有「电脑」");
      // Adding your phone stays: 我的设备 on the Workspaces page.
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "工作区" }));
      await page.getByRole("button", { name: "添加另一台设备" }).waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "外观" }));
      await sw.waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "settings-simple-mode");
      // The workspace menu still joins or starts a workspace (onboarding
      // sends people there).
      if (!narrow) {
        await page.keyboard.press("Escape");
        await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
        await page.locator(".app-hub-menu", { hasText: "加入或新建工作区" }).waitFor({ timeout: 5_000 });
        await page.keyboard.press("Escape");
        await go(page, "设置", narrow);
        await clickVisible(page, page.locator(".app-settings-tab", { hasText: "外观" }));
        await sw.waitFor({ timeout: 10_000 });
      }
      await sw.click();
      await page.locator(".app-settings-tab", { hasText: "工作区" }).waitFor({ timeout: 5_000 });
      if (!narrow) await page.locator(".sp-nav-pcs").waitFor({ timeout: 5_000 });
      if (narrow) await sidebar(page, false);
    });

    // Settings search: typing finds a row in any section; Enter opens its
    // section and points the row out.
    await step("设置 · 搜索", async () => {
      await go(page, "设置", narrow);
      await page.locator(".app-settings-head h2").waitFor({ timeout: 10_000 });
      const box = page.locator(".app-settings-search input");
      await box.fill("字号");
      const hit = page.locator(".app-settings-result", { hasText: "文字大小" });
      await hit.waitFor({ timeout: 5_000 });
      if (await page.locator(".app-settings-tab").count()) throw new Error("搜索时导航应只显示结果");
      await settle(page, 150);
      await shot(page, size, scheme, "settings-search");
      await box.press("Enter");
      await page.locator('[data-setting="text-size"].is-found').waitFor({ timeout: 5_000 });
      if (await page.locator(".app-settings-head h2").innerText() !== "外观") throw new Error("应换到「外观」");
      if (await box.inputValue()) throw new Error("选了结果后搜索框应清空");
      await box.fill("不存在的设置xyz");
      await page.locator(".app-settings-none").waitFor({ timeout: 5_000 });
      await box.press("Escape");
      await page.locator(".app-settings-tab", { hasText: "外观" }).waitFor({ timeout: 5_000 });
      if (!narrow && !(await page.locator(".app-settings-group-title", { hasText: "这台电脑" }).count())) throw new Error("导航应有「这台电脑」分组");
      if (narrow) await sidebar(page, false);
    });

    // Settings › 快捷键: ⌘/ (Ctrl+/) opens it from anywhere, and moves
    // there when Settings is already open; a list to read, by group.
    await step("设置 · 快捷键", async () => {
      await closeSettings(page);
      await page.keyboard.press("Control+Slash");
      const list = page.locator("[data-testid=shortcuts]");
      await list.waitFor({ timeout: 10_000 });
      for (const g of ["全局", "对话", "输入框", "终端"]) {
        if (!(await list.locator(".app-setting-group", { hasText: g }).count())) throw new Error(`快捷键应有「${g}」一组`);
      }
      if (await list.locator(".kb-line").count() < 20) throw new Error("快捷键应列全");
      await settle(page, 150);
      await shot(page, size, scheme, "settings-shortcuts");
      await page.locator(".app-settings-search input").fill("外观");
      await page.locator(".app-settings-search input").press("Enter");
      await page.locator('[data-setting="theme"]').waitFor({ timeout: 5_000 });
      await page.keyboard.press("Control+Slash");
      await list.waitFor({ timeout: 5_000 });
      await closeSettings(page);
      // ⌘, afterwards opens Settings where it usually does, not here.
      await page.keyboard.press("Control+Comma");
      await page.locator(".app-settings-head h2").waitFor({ timeout: 5_000 });
      if (await list.count()) throw new Error("⌘, 不应还停在快捷键");
      await closeSettings(page);
    });

    // 使用文档: from the last row of Settings' nav (and Settings › About;
    // no longer in the workspace menu); pages in
    // groups on the left, the page on the right, links between pages, a
    // search; a narrow window picks the page from one list.
    await step("使用文档", async () => {
      await go(page, "设置", narrow);
      const navGuide = page.locator("[data-testid=settings-nav-guide]");
      if (await navGuide.isVisible()) {
        await clickVisible(page, navGuide);
      } else {
        // (a narrow window: Settings' search finds it under About)
        const box = page.locator(".app-settings-search input");
        await box.fill("使用文档");
        await box.press("Enter");
        await clickVisible(page, page.locator("[data-testid=settings-guide]"));
      }
      const help = page.locator("[data-testid=help]");
      await help.waitFor({ timeout: 10_000 });
      const doc = help.locator("[data-testid=help-doc]");
      if (narrow) {
        await help.locator(".hp-pick").selectOption("projects");
      } else {
        if ((await help.locator(".hp-item").count()) < 12) throw new Error("使用文档左边应列出全部文档");
        await clickVisible(page, help.locator(".hp-item", { hasText: /^项目$/ }));
      }
      await page.waitForFunction(() => document.querySelector("[data-testid=help-doc]")?.getAttribute("data-doc") === "projects", null, { timeout: 5_000 });
      if (!(await doc.innerText()).includes("协调者")) throw new Error("项目那篇应讲协调者");
      await settle(page, 200);
      await shot(page, size, scheme, "help");
      // Its pictures are there (each screenshot a page names is bundled)
      // and one opens full size.
      const pics = doc.locator(".md img");
      if ((await pics.count()) < 1) throw new Error("项目那篇应有配图");
      await page.waitForFunction(() => [...document.querySelectorAll("[data-testid=help-doc] .md img")].every((i) => i.complete && i.naturalWidth > 0), null, { timeout: 10_000 });
      await clickVisible(page, pics.first());
      await page.locator(".lightbox").waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await page.locator(".lightbox").waitFor({ state: "detached", timeout: 5_000 });
      // A link to another page opens it here.
      await clickVisible(page, help.locator(".hp-pager-btn.is-next"));
      await page.waitForFunction(() => document.querySelector("[data-testid=help-doc]")?.getAttribute("data-doc") === "superagent", null, { timeout: 5_000 });
      if (!narrow) {
        await help.locator(".hp-search input").fill("webhook");
        await help.locator(".hp-item", { hasText: "自动化" }).first().waitFor({ timeout: 5_000 });
        await help.locator(".hp-search input").press("Enter");
        await page.waitForFunction(() => document.querySelector("[data-testid=help-doc]")?.getAttribute("data-doc") === "automations", null, { timeout: 5_000 });
      }
      await navBackClick(page);
    });

    // 上手引导 again (Settings › About): this computer's agent CLIs and its
    // sessions to bring in; skipping leaves things as they were.
    await step("设置 · 上手引导", async () => {
      await go(page, "设置", narrow);
      const box = page.locator(".app-settings-search input");
      await box.fill("上手引导");
      await box.press("Enter");
      await clickVisible(page, page.locator('[data-setting="prepare"]').getByRole("button", { name: "重新打开" }));
      const prep = page.locator("[data-testid=prepare]");
      await prep.waitFor({ timeout: 10_000 });
      await prep.locator('[data-agent-cli="claude"]').waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "prepare");
      await clickVisible(page, prep.getByRole("button", { name: "先跳过" }));
      await prep.waitFor({ state: "detached", timeout: 10_000 });
      await page.locator(".app-settings-head").waitFor({ timeout: 10_000 });
      await closeSettings(page);
      // With it, the 上手清单 at the foot of the sidebar ticks what is done,
      // and pages say what they are for the first time.
      if (narrow) await sidebar(page, true);
      const list = page.locator("[data-testid=checklist]");
      await list.waitFor({ timeout: 10_000 });
      await list.locator('.ob-step.is-done[data-step="agent"]').waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "checklist");
      // 认识你的工作区: Settings opens on 工作区, and it is ticked.
      await clickVisible(page, list.locator('.ob-step[data-step="workspace"]'));
      await page.locator(".app-settings-head h2", { hasText: "工作区" }).waitFor({ timeout: 10_000 });
      if (narrow) await sidebar(page, true);
      await list.locator('.ob-step.is-done[data-step="workspace"]').waitFor({ timeout: 10_000 });
      // The last step: what else myco does, in the user guide; ticked.
      await closeSettings(page);
      if (narrow) await sidebar(page, true);
      await clickVisible(page, list.locator('.ob-step[data-step="guide"]'));
      await page.locator("[data-testid=help]").waitFor({ timeout: 10_000 });
      if (narrow) await sidebar(page, true);
      await list.locator('.ob-step.is-done[data-step="guide"]').waitFor({ timeout: 10_000 });
      if (narrow) await sidebar(page, false);
      await go(page, "应用", narrow);
      const intro = page.locator("[data-testid=intro-apps]");
      await intro.waitFor({ timeout: 10_000 });
      await clickVisible(page, intro.getByRole("button", { name: "知道了" }));
      await intro.waitFor({ state: "detached", timeout: 5_000 });
      // Put away, for the steps after this one.
      if (narrow) await sidebar(page, true);
      await clickVisible(page, list.getByRole("button", { name: "不再显示上手清单" }));
      await list.waitFor({ state: "detached", timeout: 5_000 });
      await page.evaluate(() => { localStorage.removeItem("myco-onboard"); window.dispatchEvent(new CustomEvent("myco-onboard-changed")); });
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

    await step("设置 · CLI 登录状态", async () => {
      const zh = scheme === "dark";
      let checked = false;
      const fixtures = [
        { cli: "codebuddy", name: "CodeBuddy Code", state: "unknown" },
        { cli: "qoder", name: "Qoder CLI", state: "ready" },
        { cli: "kimi", name: "Kimi Code", state: "unknown" },
        { cli: "gemini", name: "Gemini CLI", state: "signed_out" },
      ].map(a => ({ ...a, path: `/demo/bin/${a.cli}`, version: "1.0.0", install: "example-install", login: a.cli }));
      await page.route("**/app/local/agents", route => route.fulfill({ json: {
        agents: fixtures.map(a => a.cli === "codebuddy" && checked ? { ...a, state: "ready" } : a),
      } }));
      let loginRequested = false;
      await page.route("**/app/local/agents/kimi/terminal", async route => {
        if (route.request().postDataJSON().action !== "login") throw Error("secondary link must request login");
        loginRequested = true;
        await route.fulfill({ json: { opened: true, command: "kimi" } });
      });
      await page.goto(`${r.base}/?lang=${zh ? "zh" : "en"}`);
      await page.locator(".app-shell").waitFor();
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await clickVisible(page, page.locator(".app-hub-menu-item", { hasText: zh ? "设置" : "Settings" }));
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: zh ? "本机 agent" : "Agents" }));
      const unknown = page.locator('.la-line[data-agent-cli="codebuddy"]');
      await unknown.locator(".app-pill", { hasText: zh ? "已安装" : "Installed" }).waitFor();
      await unknown.getByText(zh ? "看不出是否登录，能用就不用管。需要时可以去登录。" : "Sign-in status is unknown. If it works, no action is needed. Sign in only if needed.", { exact: true }).waitFor();
      await unknown.locator(".app-linklike", { hasText: zh ? "去登录" : "Go to sign in" }).waitFor();
      if (await unknown.locator(".app-btn").count()) throw Error("unknown auth must not have a primary sign-in button");
      await page.locator('.la-line[data-agent-cli="gemini"]').getByRole("button", { name: zh ? "登录" : "Sign in", exact: true }).waitFor();
      const kimi = page.locator('.la-line[data-agent-cli="kimi"]');
      await kimi.getByRole("button", { name: zh ? "去登录" : "Go to sign in", exact: true }).click();
      await kimi.locator(".la-opened").waitFor();
      if (!loginRequested) throw Error("secondary sign-in must remain available");
      checked = true;
      await page.locator(".la-foot").getByRole("button", { name: zh ? "重新检查" : "Check again", exact: true }).click();
      for (const cli of ["codebuddy", "qoder"]) {
        const line = page.locator(`.la-line[data-agent-cli="${cli}"][data-agent-state="ready"]`);
        await line.waitFor();
        if (await line.locator("button").count()) throw Error("ready CLI must not ask for login");
      }
      await shot(page, size, scheme, "settings-cli-auth");
      await closeSettings(page);
      await page.unroute("**/app/local/agents");
      await page.unroute("**/app/local/agents/kimi/terminal");
      await page.goto(`${r.base}/?lang=zh`);
      await page.locator(".app-shell").waitFor();
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
    // Before a page uses this computer the App asks the person (one
    // question; 始终允许 is kept, and the screen may be allowed always while
    // commands are only ever for this run); Settings › 应用权限 lists what
    // pages may do and takes it back, after which the page is asked again.
    await step("网页 · 用电脑前先问", async () => {
      await go(page, "官网改版", narrow);
      await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-open-page", { detail: { kind: "page", hubId: "hub_studio", id: "pg_consent" } })));
      const frame = page.frameLocator('iframe.pg-frame[aria-label="提醒我"]');
      await frame.locator("#go").waitFor({ timeout: 15_000 });
      await page.locator("[data-testid=page-loading]").waitFor({ state: "detached", timeout: 10_000 });
      await settle(page, 300);
      // (a click event on the button: the docked frame's hit test is not what is tried here)
      await frame.locator("#go").dispatchEvent("click");
      const ask = page.locator(".pg-ask", { hasText: "发系统通知" });
      await ask.waitFor({ timeout: 10_000 });
      if (!(await ask.innerText()).includes("「提醒我」")) throw new Error(`应写明是哪个应用要用：${await ask.innerText()}`);
      if (!(await ask.getByRole("button", { name: "这次允许" }).count())) throw new Error("应能只允许这一次");
      await settle(page, 200);
      if (!(await ask.innerText()).includes("以后这个应用不再问")) throw new Error(`应说明始终允许以后不再问：${await ask.innerText()}`);
      await shot(page, size, scheme, "page-os-consent");
      await ask.getByRole("button", { name: "始终允许" }).click();
      await frame.locator("#out", { hasText: "已提醒" }).waitFor({ timeout: 10_000 });
      // the second time: no question
      await frame.locator("#go").dispatchEvent("click");
      await settle(page, 400);
      if (await ask.count()) throw new Error("始终允许后不该再问");
      // the screen: may be allowed always
      await frame.locator("#shot").dispatchEvent("click");
      const askScreen = page.locator(".pg-ask", { hasText: "截取或录制屏幕" });
      await askScreen.waitFor({ timeout: 10_000 });
      await askScreen.getByRole("button", { name: "始终允许" }).click();
      await frame.locator("#shot-out", { hasText: "已截屏" }).waitFor({ timeout: 10_000 });
      // commands: only this run, and the dialog says why
      await frame.locator("#run").dispatchEvent("click");
      const askRun = page.locator(".pg-ask", { hasText: "运行命令" });
      await askRun.waitFor({ timeout: 10_000 });
      if (await askRun.getByRole("button", { name: "始终允许" }).count()) throw new Error("运行命令不该有始终允许");
      if (!(await askRun.innerText()).includes("撤不回")) throw new Error(`应说明为什么没有始终允许：${await askRun.innerText()}`);
      await settle(page, 200);
      await shot(page, size, scheme, "page-os-consent-once");
      await askRun.getByRole("button", { name: "这次允许" }).click();
      await frame.locator("#run-out", { hasText: "已运行" }).waitFor({ timeout: 10_000 });
      // (the page panel goes away first: in a narrow window it covers the sidebar)
      const panel = page.locator(".pg-panel:visible");
      if (await panel.count()) {
        await clickVisible(page, panel.getByRole("button", { name: "收起" }).first());
        await panel.waitFor({ state: "hidden", timeout: 5_000 });
      }
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "应用权限" }));
      const card = page.locator(".app-setting-card[data-setting='app-permissions']");
      const row = card.locator(".app-perm-page", { hasText: "提醒我" });
      await row.waitFor({ timeout: 10_000 });
      const rowText = await row.innerText();
      if (!rowText.includes("发系统通知") || !rowText.includes("始终允许") || !rowText.includes("截取或录制屏幕")) throw new Error(`应用权限应列出它：${rowText}`);
      if (!/运行命令[^\n]*\n[^\n]*这次允许/.test(rowText)) throw new Error(`运行命令应是这次允许：${rowText}`);
      if (!(await card.locator(".app-perm-page", { hasText: "空间清理" }).innerText()).includes("这次允许")) throw new Error("应分清始终允许和这次允许");
      await settle(page, 200);
      await shot(page, size, scheme, "settings-app-permissions");
      for (const what of ["发系统通知", "截取或录制屏幕", "运行命令"]) {
        await row.locator(".app-perm-item", { hasText: what }).getByRole("button", { name: "收回" }).click();
        await row.locator(".app-perm-item", { hasText: what }).waitFor({ state: "detached", timeout: 5_000 });
      }
      await row.waitFor({ state: "detached", timeout: 5_000 });
    });

    // 侧栏显示: every entry shows until hidden, from Settings or by
    // right-clicking it in the sidebar; shown again from Settings.
    await step("设置 · 侧栏显示", async () => {
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "外观" }));
      const row = page.locator("[data-setting=nav-entries]");
      await row.waitFor({ timeout: 5_000 });
      const chip = (name) => row.locator(".nav-show-chip", { hasText: name });
      if ((await row.locator(".nav-show-chip.is-on").count()) !== (await row.locator(".nav-show-chip").count())) throw new Error("默认应全部显示");
      await chip("自动化").click();
      await page.waitForFunction(() => !document.querySelector(".sp-nav-auto"), null, { timeout: 5_000 });
      if (await chip("自动化").getAttribute("aria-pressed") !== "false") throw new Error("隐藏后按钮应显示为关");
      await settle(page, 200);
      await shot(page, size, scheme, "settings-nav-entries");
      await page.keyboard.press("Escape");
      await sidebar(page, true);
      // right-click an entry: hide it
      await page.locator(".sp-nav-workflows").click({ button: "right" });
      await page.locator(".conv-menu").getByRole("menuitem", { name: "在侧栏隐藏「工作流」" }).click();
      await page.locator(".sp-nav-workflows").waitFor({ state: "detached", timeout: 5_000 });
      // Hidden is never silent: the sidebar says what, and a click opens the setting.
      const note = page.locator("[data-testid=nav-hidden-note]");
      if (!(await note.innerText()).includes("工作流")) throw new Error(`侧栏应写明隐藏了什么，实际「${await note.innerText()}」`);
      // and back, from Settings
      await go(page, "设置", narrow);
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "外观" }));
      await chip("自动化").click();
      await chip("工作流").click();
      await page.keyboard.press("Escape");
      await sidebar(page, true);
      await page.locator(".sp-nav-auto").waitFor({ timeout: 5_000 });
      await page.locator(".sp-nav-workflows").waitFor({ timeout: 5_000 });
      if (await page.locator("[data-testid=nav-hidden-note]").count()) throw new Error("全部显示时不该有「已隐藏」提示");
      if (narrow) await sidebar(page, false);
    });

    // An App updated from before 侧栏显示, or with a setting it cannot read:
    // everything shows (projects and chats above all).
    await step("设置 · 侧栏显示（升级上来）", async () => {
      for (const v of [null, "not json", "{\"projects\":true}"]) {
        await page.evaluate((x) => { if (x === null) localStorage.removeItem("myco-nav-hidden"); else localStorage.setItem("myco-nav-hidden", x); }, v);
        await page.reload({ waitUntil: "domcontentloaded" });
        // (the shell first: sidebar() reads it before it is there as hidden,
        // and its click would then close the sidebar)
        await page.locator(".app-crumbbar").waitFor({ timeout: 15_000 });
        await sidebar(page, true);
        await page.locator(".app-sidebar").getByText("官网改版", { exact: true }).first().waitFor({ timeout: 15_000 });
        await page.locator(".app-sidebar").getByText("PM (Codex)", { exact: true }).first().waitFor({ timeout: 5_000 });
        for (const sel of [".sp-nav-auto", ".sp-nav-workflows", ".sp-nav-templates", ".sp-nav-computers", ".sp-nav-projects", ".sp-nav-chats"]) {
          if (!(await page.locator(sel).count())) throw new Error(`没有存过或读不出侧栏设置时（${v}）应全部显示，缺 ${sel}`);
        }
        if (await page.locator("[data-testid=nav-hidden-note]").count()) throw new Error("没有隐藏时不该有「已隐藏」提示");
      }
      await page.evaluate(() => localStorage.removeItem("myco-nav-hidden"));
      if (narrow) await sidebar(page, false);
    });

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
      const first = page.locator(".conv-msg.is-me", { hasText: t }).last();
      await first.waitFor({ timeout: 20_000 });
      // (this task and the first one queued run until the step ends them
      // (POST /app/mock/finish): on a slow runner the mock's own six seconds
      // ran out before the third ⌘↩, which then sent instead of queuing)
      const finish = async (text) => {
        const id = await page.locator(".conv-detail .conv-msg.is-me[data-msg]", { hasText: text }).last().getAttribute("data-msg");
        const r = await fetch(`http://${HOST}:${mockPort()}/app/mock/finish`, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ id }) });
        if (!r.ok) throw new Error(`没能让「${text}」那一轮结束：HTTP ${r.status}`);
      };
      // ⌘↩ queues only while that one is at work (the box then offers to
      // queue the next one); before the agent takes it up, it would just
      // send. Wait for that, not for a fixed time.
      await page.waitForFunction(() => /排到下一条/.test(document.querySelector(".conv-detail .conv-compose-input")?.getAttribute("placeholder") || ""), null, { timeout: 20_000 });
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
      await finish(t);
      await page.locator(".conv-msg.is-me", { hasText: a }).last().waitFor({ timeout: 30_000 });
      await page.waitForFunction(() => document.querySelectorAll(".conv-detail .conv-queue-row").length === 1, null, { timeout: 10_000 }).catch(() => undefined);
      if (await page.locator(".conv-msg.is-me", { hasText: b }).count()) throw new Error("排的第二条应等第一条做完再发，不能同时发出");
      if ((await rows.count()) !== 1) throw new Error(`第一条发出后应还剩一条在排队，实际 ${await rows.count()} 条`);
      await page.locator(".conv-detail .conv-msg.is-me[data-msg]", { hasText: a }).last().waitFor({ timeout: 10_000 });
      await finish(a);
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

    // A long thing said between steps is held short: fading out, with
    // 展开全文 under it, never cut through a line with nothing to say so.
    await step("单聊 · 过程里的长段话", async () => {
      const body = "长段汇总：各个循环现在怎么样";
      await openMember(page, narrow, "聊天", "legacy-codex");
      await sendFromElsewhere("hub_studio", "legacy-codex", body);
      await page.locator(".conv-msg", { hasText: body }).last().waitFor({ timeout: 20_000 });
      await page.waitForTimeout(7_500);
      const fold = page.locator(".conv-detail .trace-fold").last();
      if ((await fold.getAttribute("aria-expanded")) !== "true") await fold.click();
      const said = page.locator(".conv-detail .trace-say", { hasText: "先说结论" }).last();
      await said.waitFor({ timeout: 10_000 });
      if (!(await said.evaluate((el) => el.classList.contains("is-clamped")))) throw new Error("长段话在过程里应先收起");
      const mask = await said.evaluate((el) => getComputedStyle(el).maskImage || getComputedStyle(el).webkitMaskImage || "");
      if (!/gradient/.test(mask)) throw new Error("收起的长段话底部应渐隐，而不是一刀切");
      const more = said.locator("xpath=following-sibling::*[1]");
      if (!(await more.evaluate((el) => el.classList.contains("trace-say-more")))) throw new Error("收起的长段话下面应有「展开全文」");
      await said.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-long-said");
      await clickVisible(page, more);
      await page.waitForFunction(() => {
        const el = [...document.querySelectorAll(".conv-detail .trace-say")].filter((x) => x.textContent?.includes("先说结论")).pop();
        return !!el && !el.classList.contains("is-clamped") && el.scrollHeight <= el.clientHeight + 2 && el.textContent.includes("最后一行");
      }, null, { timeout: 5_000 });
      if (await page.locator(".conv-detail .trace-say-more").count()) throw new Error("展开以后「展开全文」应收起来");
    });

    // 盯着 (myco watch): what an agent waits for in the background is at the
    // top of its chat, the clock moving, with 延长 and 停止; how earlier
    // ones ended is a grey line each; members show 盯着 N.
    await step("单聊 · 盯着", async () => {
      // The mock's watches are timed from their first read; start them again
      // so 已12分钟 holds however long the steps before took.
      const reset = await fetch(`http://${HOST}:${mockPort()}/app/mock/watches-reset`, { method: "POST" });
      if (!reset.ok) throw new Error(`重置盯着失败：HTTP ${reset.status}`);
      await openMember(page, narrow, "聊天", "legacy-codex");
      const bar = page.locator(".conv-detail [data-testid=watch-bar]");
      await bar.waitFor({ timeout: 10_000 });
      const first = (await bar.locator(".watch-line").innerText()).replace(/\s+/g, "");
      if (!/正在盯：mac2的回复/.test(first) || !/已12分钟/.test(first) || !/剩1[78]分钟/.test(first) || !/秒前看过/.test(first)) throw new Error(`盯着的那条不对：「${first}」`);
      await page.waitForTimeout(2200);
      if ((await bar.locator(".watch-line").innerText()).replace(/\s+/g, "") === first) throw new Error("「多久前看过」应每秒在走");
      // Its last check and times, opened.
      await bar.locator(".watch-toggle").click();
      await bar.locator(".watch-result", { hasText: "mac2 的任务仍在处理中" }).waitFor({ timeout: 5_000 });
      if (!/开始.*截止/.test(await bar.locator(".watch-times").innerText())) throw new Error("展开后应有开始和截止时间");
      await settle(page, 300);
      await shot(page, size, scheme, "chat-watching");
      // Longer: from now, an hour.
      await bar.getByRole("button", { name: "延长" }).click();
      await page.locator(".conv-menu").getByRole("menuitem", { name: "1 小时" }).click();
      await page.waitForFunction(() => /剩(1小时|59分钟)/.test((document.querySelector(".conv-detail [data-testid=watch-bar] .watch-line")?.textContent || "").replace(/\s+/g, "")), null, { timeout: 5_000 });
      // How the ones before ended: met, the time up, stopped.
      for (const t of ["盯到了：Release 的发布说明", "没等到：CI 跑完", "你停止了盯：Review 的审查意见"]) {
        if (!(await page.locator(".conv-detail .watch-ended", { hasText: t }).count())) throw new Error(`会话里应有一行「${t}」`);
      }
      // Stopped, after asking: the bar goes, a grey line stays.
      await bar.getByRole("button", { name: "停止" }).click();
      const ask = page.locator(".app-confirm");
      await ask.waitFor({ timeout: 5_000 });
      await ask.getByRole("button", { name: "停止", exact: true }).click();
      await bar.waitFor({ state: "detached", timeout: 5_000 });
      await page.locator(".conv-detail .watch-ended", { hasText: "你停止了盯：mac2 的回复" }).waitFor({ timeout: 5_000 });
      // Members: 盯着 2 beside Myco (Grok); its chat says 正在盯 2 件事.
      const grok = page.locator(".sp-side .sp-member", { hasText: "Myco (Grok)" }).first();
      if (!narrow) {
        await grok.locator(".sp-watch-tag", { hasText: "盯着 2" }).waitFor({ timeout: 10_000 });
      }
      await openMember(page, narrow, "聊天", "Myco (Grok)");
      const two = page.locator(".conv-detail [data-testid=watch-bar]");
      await two.locator(".watch-what", { hasText: "正在盯 2 件事" }).waitFor({ timeout: 10_000 });
      await two.locator(".watch-toggle").click();
      await two.locator(".watch-item", { hasText: "构建流水线跑完" }).waitFor({ timeout: 5_000 });
      await two.locator(".watch-item", { hasText: "构建流水线跑完" }).locator(".watch-result code", { hasText: "build: pending" }).waitFor({ timeout: 5_000 });
      if ((await two.locator(".watch-item").count()) !== 2) throw new Error("展开后应列出两件");
      await settle(page, 300);
      await shot(page, size, scheme, "chat-watching-two");
    });

    // 目标 (myco goal): a goal set on a chat is a bar at the top (round,
    // next wake-up, how long), with 暂停/继续, 改间隔 and 停止; opened, what
    // done means and every round; ended ones are a grey line; ⋯ sets one.
    await step("单聊 · 目标", async () => {
      // The mock's goals are timed from their first read; start them again
      // so 还有 11–12 分钟 and 已进行 1 小时 hold however long the steps before took.
      const reset = await fetch(`http://${HOST}:${mockPort()}/app/mock/goals-reset`, { method: "POST" });
      if (!reset.ok) throw new Error(`重置目标失败：HTTP ${reset.status}`);
      await openMember(page, narrow, "聊天", "legacy-codex");
      const bar = page.locator(".conv-detail [data-testid=goal-bar]");
      await bar.waitFor({ timeout: 10_000 });
      const line = (await bar.locator(".watch-line").innerText()).replace(/\s+/g, "");
      for (const t of ["目标：把旧版runner的日志格式", "第3/16轮", "下次唤醒", "还有1", "已进行1小时"]) if (!line.includes(t)) throw new Error(`目标条应有「${t}」，实际「${line}」`);
      await bar.locator(".watch-toggle").click();
      await bar.locator(".goal-facts dd", { hasText: "文档覆盖全部 3 种日志格式" }).waitFor({ timeout: 5_000 });
      if ((await bar.locator(".goal-round").count()) !== 3) throw new Error("展开后应列出 3 轮");
      if (!(await bar.locator(".goal-round.is-stuck", { hasText: "没进展" }).count())) throw new Error("没进展的那轮应标出来");
      await settle(page, 300);
      await shot(page, size, scheme, "chat-goal");
      // An earlier goal, achieved: a grey line that opens to how it ended.
      const done = page.locator(".conv-detail .goal-ended", { hasText: "目标达成：把 runner 的启动脚本改成能重入" });
      await done.waitFor({ timeout: 5_000 });
      await done.locator(".goal-ended-line").click();
      await done.locator(".goal-ended-detail", { hasText: "重复启动只会留一个进程" }).waitFor({ timeout: 5_000 });
      // Every hour now; paused, then on again.
      await bar.getByRole("button", { name: "改间隔" }).click();
      await page.locator(".conv-menu").getByRole("menuitem", { name: "1 小时" }).click();
      await page.waitForFunction(() => /还有(1小时|59分钟)/.test((document.querySelector(".conv-detail [data-testid=goal-bar] .watch-line")?.textContent || "").replace(/\s+/g, "")), null, { timeout: 5_000 });
      await bar.getByRole("button", { name: "暂停" }).click();
      await bar.locator(".goal-note.is-paused").waitFor({ timeout: 5_000 });
      await bar.getByRole("button", { name: "继续" }).click();
      await bar.locator(".goal-note").waitFor({ state: "detached", timeout: 5_000 });
      // Stopped, after asking: the bar goes, a grey line stays.
      await bar.getByRole("button", { name: "停止" }).click();
      const ask = page.locator(".app-confirm");
      await ask.waitFor({ timeout: 5_000 });
      await ask.getByRole("button", { name: "停止", exact: true }).click();
      await bar.waitFor({ state: "detached", timeout: 5_000 });
      await page.locator(".conv-detail .goal-ended", { hasText: "你停止了目标" }).waitFor({ timeout: 5_000 });
      // A new one from the chat's ⋯.
      await page.locator(".conv-head").getByRole("button", { name: "更多" }).click();
      await page.locator(".conv-menu").getByRole("menuitem", { name: "设定目标…" }).click();
      const dlg = page.locator(".goal-dialog");
      await dlg.waitFor({ timeout: 5_000 });
      const start = dlg.getByRole("button", { name: "开始", exact: true });
      if (await start.isEnabled()) throw new Error("没填目标和终止条件不该能开始");
      await dlg.locator("textarea").nth(0).fill("把第三种日志格式也整理进去");
      await dlg.locator("textarea").nth(1).fill("docs/legacy.md 里有第三种格式和示例");
      await dlg.getByRole("tab", { name: "15 分钟" }).click();
      await settle(page, 200);
      await shot(page, size, scheme, "chat-goal-set");
      await start.click();
      await dlg.waitFor({ state: "detached", timeout: 5_000 });
      await bar.locator(".watch-what", { hasText: "目标：把第三种日志格式也整理进去" }).waitFor({ timeout: 10_000 });
      // Paused to ask you, and waiting longer after rounds without progress.
      await openMember(page, narrow, "聊天", "Myco (Grok)");
      await page.locator(".conv-detail [data-testid=goal-bar] .goal-note.is-paused", { hasText: "等你决定" }).waitFor({ timeout: 10_000 });
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await page.locator(".conv-detail [data-testid=goal-bar] .goal-note.is-backoff", { hasText: "间隔放宽到 1 小时" }).waitFor({ timeout: 10_000 });
      if (!narrow) {
        await page.locator(".sp-side .sp-member", { hasText: "Myco (Grok)" }).locator(".sp-goal-tag", { hasText: "目标暂停" }).waitFor({ timeout: 10_000 });
        await page.locator(".sp-side .sp-member", { hasText: "Review (Codex 云端)" }).locator(".sp-goal-tag", { hasText: "目标中" }).waitFor({ timeout: 10_000 });
      }
    });

    // 清爽 (界面风格): Settings → Appearance turns the decoration off at once
    // for the whole App: no faces in the lists and titles, no chat picture,
    // a dot for the walking mushroom; back to 标准 puts them back.
    await step("设置 · 清爽模式", async () => {
      const style = async (label) => {
        await go(page, "设置", narrow);
        await page.locator(".app-settings-search input").fill("界面风格");
        await page.locator(".app-settings-search input").press("Enter");
        const row = page.locator('[data-setting="ui-style"]');
        await row.waitFor({ timeout: 5_000 });
        await row.getByRole("tab", { name: label, exact: true }).click();
        await closeSettings(page);
      };
      await style("清爽");
      if (!(await page.evaluate(() => document.documentElement.classList.contains("ui-clean")))) throw new Error("选了清爽，界面应马上变");
      await go(page, "官网改版", narrow);
      await page.locator(".sp-home .conv-detail").waitFor({ timeout: 10_000 });
      const look = await page.evaluate(() => {
        const shown = (el) => !!el && el.getClientRects().length > 0 && getComputedStyle(el).display !== "none";
        return {
          sideFaces: [...document.querySelectorAll(".app-sidebar .app-nav-item > .pa, .app-hub-switch .app-hub-avatar")].filter(shown).length,
          headFaces: [...document.querySelectorAll(".conv-head-faces")].filter(shown).length,
          memberFace: Math.max(0, ...[...document.querySelectorAll(".sp-side .sp-member > .pa")].map((e) => e.getBoundingClientRect().width)),
          mushrooms: [...document.querySelectorAll("svg.work-sprite")].filter((e) => e.getBoundingClientRect().width > 12).length,
        };
      });
      if (look.sideFaces) throw new Error("清爽模式下侧栏（含左下工作区）不该有头像");
      if (look.headFaces) throw new Error("清爽模式下会话标题不该有头像");
      if (look.memberFace > 12) throw new Error(`清爽模式下成员栏只留状态点，实际宽 ${look.memberFace}px`);
      if (look.mushrooms) throw new Error("清爽模式下不该有走路的小蘑菇");
      await settle(page, 300);
      await shot(page, size, scheme, "clean-mode");
      await style("标准");
      if (await page.evaluate(() => document.documentElement.classList.contains("ui-clean"))) throw new Error("换回标准，装饰应回来");
      await go(page, "官网改版", narrow);
      if (!narrow) await page.locator(".app-sidebar .app-nav-item > .pa").first().waitFor({ timeout: 5_000 });
    });

    // A task the node accepted long ago and went quiet on may have lost its
    // result: the chat says so instead of spinning, and one click stops it
    // and sends the same message again.
    // An agent holding a task on a question to another agent: not amber
    // ("needs your decision") but a quiet ring; its chat says what waits on
    // whom and leads to its open work, where it can be stopped.
    await step("单聊 · 在等别的 agent 回答", async () => {
      const held = (on) => page.evaluate(async (v) => { await fetch("/app/mock/held-for-other", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ on: v }) }); }, on);
      await held(true);
      try {
      await sidebar(page, true);
      const nav = page.locator(".sp-nav-person", { hasText: "Myco (Grok)" });
      await nav.waitFor({ timeout: 10_000 });
      if (await nav.locator(".sp-nav-busy.is-asking").count()) throw new Error("问题是问别的 agent 的，不该显示「在等你决定」");
      await nav.locator(".sp-nav-busy.is-open").waitFor({ timeout: 10_000 });
      await clickVisible(page, nav);
      if (narrow) await sidebar(page, false);
      const note = page.locator("[data-testid=open-elsewhere]");
      await note.waitFor({ timeout: 20_000 });
      const text = compactText(await note.innerText());
      if (!text.includes("在等 PM (Codex) 回答")) throw new Error(`对话里应说明在等谁回答，实际「${text}」`);
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-open-elsewhere");
      await clickVisible(page, note.getByRole("button", { name: "查看并处理" }));
      const row = page.locator(".app-msg-wrap", { hasText: "给官网首页写三条 slogan 候选" });
      await row.waitFor({ timeout: 20_000 });
      if (!compactText(await row.innerText()).includes("在等 PM (Codex) 回答")) throw new Error("当前工作里应写在等谁回答");
      await row.hover();
      await settle(page, 200);
      await shot(page, size, scheme, "profile-open-work");
      await clickVisible(page, row.getByRole("button", { name: "停止" }));
      await row.waitFor({ state: "detached", timeout: 15_000 });
      await clickVisible(page, page.locator(".ms-close"));
      await page.locator(".ms-scrim").waitFor({ state: "detached", timeout: 10_000 });
      } finally {
        await held(false);
      }
    });

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
      // Left open or stuck: close it as done, ask where it stands, stop or resend.
      await note.getByRole("button", { name: "标记完成" }).waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-stuck");
      await clickVisible(page, note.getByRole("button", { name: "再催一下" }));
      await note.getByRole("button", { name: "已经催了" }).waitFor({ timeout: 10_000 });
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
    await step("成员 · 正在更新", async () => {
      const change = (phase) => fetch(`http://${HOST}:${mockPort()}/app/mock/runner-restart${phase ? `?phase=${phase}` : ""}`, {method: phase ? "POST" : "DELETE"});
      await change("updating");
      try {
        await go(page, "成员", narrow);
        const card = page.locator(".mb-card", {hasText: "docs-claude"}).first();
        await card.getByText("正在更新", {exact:true}).waitFor({timeout:15000});
        await clickVisible(page, card);
        const updating = page.locator('[data-testid="runner-updating"]');
        await updating.waitFor({timeout:10000});
        if (await page.locator('.ms .app-offline-why').count()) throw Error("更新时不能显示停止/离线诊断");
        await shot(page,size,scheme,"runner-updating");
        await page.keyboard.press("Escape");
        await go(page,"电脑",narrow);
        const computer=page.locator(".pc-card",{hasText:"Studio Mac"});
        await computer.locator(".pc-online").filter({hasText:"正在更新"}).waitFor({timeout:15000});
        await shot(page,size,scheme,"computer-updating");
        await change("expired");
        await computer.locator(".pc-online").filter({hasText:"不在线"}).waitFor({timeout:15000});
      } finally { await change(""); }
    });

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

    // Which provider an agent on this computer uses: following this
    // computer says where requests really go (a local gateway here); a
    // custom address shows its host and model, never its key, and Codex
    // picks a protocol.
    await step("成员 · 服务商", async () => {
      const openRow = async (who) => {
        await go(page, "成员", narrow);
        await clickVisible(page, page.locator(".mb-card", { hasText: who }).first());
        await page.locator(".ms").waitFor({ timeout: 10_000 });
        await clickVisible(page, page.locator(".ms .ms-fold"));
        const row = page.locator(".ms [data-testid=provider-open]");
        await row.waitFor({ timeout: 10_000 });
        await row.scrollIntoViewIfNeeded();
        return row;
      };
      let row = await openRow("docs-claude");
      if (!(await row.innerText()).includes("magpie 网关（127.0.0.1:3425）")) throw new Error(`跟随这台电脑时应写实际走的地址，实际「${await row.innerText()}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "provider-follow");
      await page.keyboard.press("Escape");
      row = await openRow("PM (Codex)");
      const rowText = await row.innerText();
      if (!rowText.includes("api.example-llm.com · gpt-5-codex")) throw new Error(`自定义地址应显示 host 和模型，实际「${rowText}」`);
      await row.click();
      const dlg = page.locator(".app-modal", { has: page.locator("[data-testid=provider-options]") });
      await dlg.waitFor({ timeout: 5_000 });
      const form = dlg.locator("[data-testid=provider-custom]");
      await form.waitFor({ timeout: 5_000 });
      if ((await form.locator("[data-testid=provider-url]").inputValue()) !== "https://api.example-llm.com/v1") throw new Error("自定义地址应带出保存的 base URL");
      const key = form.locator("[data-testid=provider-key]");
      if ((await key.inputValue()) !== "" || !(await form.innerText()).includes("已经存了一个，不会显示出来")) throw new Error("密钥只写不回显：输入框应是空的，并说已经存了一个");
      // An address with a key in it is refused before saving.
      const url = form.locator("[data-testid=provider-url]");
      await url.fill("https://user:sk-x@api.example-llm.com/v1");
      await form.locator(".app-field-hint.is-bad").waitFor({ timeout: 5_000 });
      await url.fill("https://api.example-llm.com/v1");
      if (await dlg.locator("[data-testid=provider-save]").isEnabled()) throw new Error("没填密钥时不能保存（每次都要重新填）");
      await key.fill("sk-test-123");
      if (!(await form.locator("[data-testid=provider-wire-responses]").getAttribute("aria-selected"))?.includes("true")) throw new Error("Codex 默认协议应是 Responses");
      await clickVisible(page, form.locator("[data-testid=provider-wire-chat]"));
      await settle(page, 200);
      await shot(page, size, scheme, "provider-custom");
      await clickVisible(page, dlg.locator("[data-testid=provider-save]"));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      await page.waitForFunction(() => (document.querySelector(".ms [data-testid=provider-open]")?.textContent || "").includes("api.example-llm.com"), null, { timeout: 5_000 });
      // Back to Responses (the next combo starts from the same data).
      await clickVisible(page, page.locator(".ms [data-testid=provider-open]"));
      await dlg.waitFor({ timeout: 5_000 });
      if (!(await dlg.locator("[data-testid=provider-options]").innerText()).includes("Chat Completions")) throw new Error("保存后自定义地址那项应写协议");
      await clickVisible(page, dlg.locator("[data-testid=provider-wire-responses]"));
      await dlg.locator("[data-testid=provider-key]").fill("sk-test-123");
      await clickVisible(page, dlg.locator("[data-testid=provider-save]"));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      await page.keyboard.press("Escape");
    });

    // An agent on another computer: its runner lets the App switch the
    // provider (its computer's list); a custom address needs end-to-end
    // encryption (the key is sealed for it), else it is entered there. An
    // older runner stays read-only.
    await step("成员 · 远程服务商", async () => {
      const mock = `http://${HOST}:${mockPort()}`;
      const setE2E = (on) => fetch(`${mock}/app/hubs/hub_studio/settings`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ e2e: on }) });
      const openRow = async (who) => {
        await go(page, "成员", narrow);
        await clickVisible(page, page.locator(".mb-card", { hasText: who }).first());
        await page.locator(".ms").waitFor({ timeout: 10_000 });
        await clickVisible(page, page.locator(".ms .ms-fold"));
        const row = page.locator(".ms .app-kv", { has: page.locator(".app-kv-label", { hasText: /^服务商$/ }) });
        await row.waitFor({ timeout: 10_000 });
        await row.scrollIntoViewIfNeeded();
        return row;
      };
      const dlg = page.locator(".app-modal", { has: page.locator("[data-testid=provider-options]") });
      let row = await openRow("MCP Claude");
      if (!(await row.innerText()).includes("DeepSeek（api.deepseek.com）")) throw new Error(`远程 agent 的服务商应写它那台电脑实际走的，实际「${await row.innerText()}」`);
      await clickVisible(page, row.locator("[data-testid=provider-open]"));
      await dlg.waitFor({ timeout: 5_000 });
      const opts = dlg.locator("[data-testid=provider-options]");
      const text = await opts.innerText();
      if (!text.includes("跟随它那台电脑") || !text.includes("智谱 GLM")) throw new Error(`远程时应列出它那台电脑上的服务商，实际「${text}」`);
      const customOpt = dlg.locator("[data-testid=provider-custom-opt]");
      if (!(await customOpt.locator("input").isDisabled()) || !(await customOpt.innerText()).includes("密钥不经过 hub")) throw new Error("没开端到端加密时，远程的自定义地址应置灰并说明");
      await settle(page, 200);
      await shot(page, size, scheme, "provider-remote");
      await clickVisible(page, opts.locator(".app-provider-opt", { hasText: "智谱 GLM" }));
      await clickVisible(page, dlg.locator("[data-testid=provider-save]"));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      const sent = await (await fetch(`${mock}/app/mock/run-settings`)).json();
      if (sent.last?.agent !== "mcp-claude" || sent.last.body.provider !== "cc-switch:glm") throw new Error(`远程切换服务商应随运行设置发出，实际 ${JSON.stringify(sent.last)}`);
      await page.waitForFunction(() => (document.querySelector(".ms [data-testid=provider-open]")?.textContent || "").includes("智谱 GLM"), null, { timeout: 10_000 });
      // With end-to-end encryption on, a custom address can be entered here.
      await setE2E(true);
      await page.keyboard.press("Escape");
      await page.waitForTimeout(6_000); // the App reads the workspace's settings on its next refresh
      row = await openRow("MCP Claude");
      await clickVisible(page, row.locator("[data-testid=provider-open]"));
      await dlg.waitFor({ timeout: 5_000 });
      await clickVisible(page, dlg.locator("[data-testid=provider-custom-opt]"));
      const form = dlg.locator("[data-testid=provider-custom]");
      await form.waitFor({ timeout: 5_000 });
      await form.locator("[data-testid=provider-url]").fill("https://llm.example.org/anthropic");
      await form.locator("[data-testid=provider-model]").fill("claude-opus-5-5");
      await form.locator("[data-testid=provider-key]").fill("sk-remote-1");
      if (!(await dlg.innerText()).includes("hub 只经手密文")) throw new Error("开了端到端加密时应说明密钥先加密");
      await settle(page, 200);
      await shot(page, size, scheme, "provider-remote-sealed");
      await clickVisible(page, dlg.locator("[data-testid=provider-save]"));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      // Sent sealed; its computer takes it before its next task (said under the row).
      await page.waitForFunction(() => (document.querySelector(".ms [data-testid=provider-open]")?.textContent || "").includes("llm.example.org"), null, { timeout: 10_000 });
      await page.locator(".ms .app-kv-help", { hasText: "等它那台电脑收下" }).waitFor({ timeout: 10_000 });
      // Back as it was: cleared, encryption off.
      await clickVisible(page, page.locator(".ms [data-testid=provider-open]"));
      await dlg.waitFor({ timeout: 5_000 });
      await clickVisible(page, dlg.locator("[data-testid=provider-clear]"));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      const cleared = await (await fetch(`${mock}/app/mock/run-settings`)).json();
      if (cleared.last?.body.provider !== "clear") throw new Error("清除远程的自定义地址应发 provider: clear");
      await setE2E(false);
      await page.keyboard.press("Escape");
      // An older runner on another computer: read-only, and says why.
      await go(page, "成员", narrow);
      if (await page.locator(".mb-card", { hasText: "临时 Codex" }).count()) {
        row = await openRow("临时 Codex");
        if (await row.locator("[data-testid=provider-open]").count()) throw new Error("老 runner 的服务商应只读");
        if (!(await row.innerText()).includes("升级它那台电脑上的 myco")) throw new Error("老 runner 应提示升级那台电脑上的 myco");
        await page.keyboard.press("Escape");
      }
    });

    // 模型供应商 (i115): kept in the workspace, keys sealed for the member's
    // own devices; added and tested in Settings, given to an agent from its
    // provider row, a new key pushed, and deleted with what that means.
    await step("设置 · 模型供应商", async () => {
      const mock = `http://${HOST}:${mockPort()}`;
      const setE2E = (on) => fetch(`${mock}/app/hubs/hub_studio/settings`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ e2e: on }) });
      const openSection = async () => {
        await go(page, "设置", narrow);
        await clickVisible(page, page.locator(".app-settings-tab", { hasText: "模型供应商" }));
        await page.locator("[data-testid=providers]").waitFor({ timeout: 10_000 });
      };
      await setE2E(false);
      await openSection();
      await page.locator("[data-testid=providers-e2e-off]").waitFor({ timeout: 10_000 });
      if (!(await page.locator("[data-testid=providers-add]").isDisabled())) throw new Error("没开端到端加密时不该能新增");
      await settle(page, 200);
      await shot(page, size, scheme, "settings-providers-e2e-off");
      await setE2E(true);
      await go(page, "成员", narrow);
      await openSection();
      await page.locator("[data-testid=providers-e2e-off]").waitFor({ state: "hidden", timeout: 10_000 });
      await clickVisible(page, page.locator("[data-testid=providers-add]"));
      const dlg = page.locator(".app-modal", { has: page.locator("[data-testid=provider-vendors]") });
      await dlg.waitFor({ timeout: 5_000 });
      if (!(await dlg.locator("[data-testid=provider-vendors]").innerText()).includes("OpenCode Go")) throw new Error("应列出内置的供应商");
      await dlg.locator("[data-testid=provider-api-key]").fill("mm-test-key-wrong-1234");
      await clickVisible(page, dlg.locator("[data-testid=provider-test]"));
      await dlg.locator(".app-error-text", { hasText: "HTTP 401" }).first().waitFor({ timeout: 5_000 });
      await dlg.locator("[data-testid=provider-api-key]").fill("mm-test-key-1234abcd");
      await clickVisible(page, dlg.locator("[data-testid=provider-test]"));
      await dlg.locator("[data-testid=provider-tested]", { hasText: "读到 2 个模型" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "settings-provider-add");
      await clickVisible(page, dlg.locator("[data-testid=provider-save-workspace]"));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      const row = page.locator("[data-testid^=provider-prv_]").first();
      await row.waitFor({ timeout: 5_000 });
      if (!(await row.innerText()).includes("key ···abcd")) throw new Error(`应显示 key 的末四位，实际「${await row.innerText()}」`);
      await clickVisible(page, row.getByRole("button", { name: "测试" }));
      await page.locator("[data-testid=providers] .app-small-note", { hasText: "读到 2 个模型" }).waitFor({ timeout: 5_000 });
      const pid = (await row.getAttribute("data-testid")).slice("provider-".length);
      await settle(page, 200);
      await shot(page, size, scheme, "settings-providers");

      // An agent on another computer takes it, with one of its models.
      await go(page, "成员", narrow);
      await clickVisible(page, page.locator(".mb-card", { hasText: "MCP Claude" }).first());
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const prow = page.locator(".ms .app-kv", { has: page.locator(".app-kv-label", { hasText: /^服务商$/ }) });
      await prow.waitFor({ timeout: 10_000 });
      await clickVisible(page, prow.locator("[data-testid=provider-open]"));
      const pdlg = page.locator(".app-modal", { has: page.locator("[data-testid=provider-options]") });
      await pdlg.waitFor({ timeout: 5_000 });
      await clickVisible(page, pdlg.locator(`[data-testid=provider-mine-${pid}]`));
      await pdlg.locator("[data-testid=provider-mine-model] select").selectOption("MiniMax-M2");
      await settle(page, 200);
      await shot(page, size, scheme, "provider-mine");
      await clickVisible(page, pdlg.locator("[data-testid=provider-save]"));
      await pdlg.waitFor({ state: "hidden", timeout: 5_000 });
      const sent = await (await fetch(`${mock}/app/mock/run-settings`)).json();
      if (sent.last?.agent !== "mcp-claude" || sent.last.body.workspace !== pid || sent.last.body.model !== "MiniMax-M2") throw new Error(`应把供应商和模型发给它，实际 ${JSON.stringify(sent.last)}`);
      await page.waitForFunction(() => (document.querySelector(".ms [data-testid=provider-open]")?.textContent || "").includes("MiniMax"), null, { timeout: 10_000 });
      await page.keyboard.press("Escape");

      // A new key: the agent has the older one; pushed from Settings.
      await openSection();
      await clickVisible(page, page.locator(`[data-testid=provider-${pid}]`).getByRole("button", { name: "编辑" }));
      await dlg.waitFor({ timeout: 5_000 });
      await dlg.locator("[data-testid=provider-api-key]").fill("mm-test-key-second-wxyz");
      await clickVisible(page, dlg.locator("[data-testid=provider-save-workspace]"));
      await dlg.waitFor({ state: "hidden", timeout: 5_000 });
      await page.waitForTimeout(6_000); // the App reads the agents again on its next refresh
      await openSection();
      const pushBtn = page.locator(`[data-testid=provider-${pid}] [data-testid=provider-push]`);
      await pushBtn.waitFor({ timeout: 15_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "settings-provider-push");
      await clickVisible(page, pushBtn);
      await page.locator("[data-testid=providers] .app-small-note", { hasText: "新 key 已发给 1 个 agent" }).waitFor({ timeout: 5_000 });

      // Deleted: said how many agents use it first; the agent says so after.
      await clickVisible(page, page.locator(`[data-testid=provider-${pid}]`).getByRole("button", { name: "删除" }));
      const confirm = page.locator(".app-modal", { hasText: "1 个 agent 在用它" });
      await confirm.waitFor({ timeout: 5_000 });
      await clickVisible(page, confirm.getByRole("button", { name: "删除" }));
      await page.locator(`[data-testid=provider-${pid}]`).waitFor({ state: "hidden", timeout: 5_000 });
      await go(page, "成员", narrow);
      await clickVisible(page, page.locator(".mb-card", { hasText: "MCP Claude" }).first());
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      await page.locator(".ms [data-testid=provider-deleted]").waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "provider-deleted");
      await page.keyboard.press("Escape");
      // (the demo as it was, for the steps after)
      await setE2E(false);
      await fetch(`${mock}/app/mock/run-settings`, { method: "POST" });
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
      // The details are four tabs, one subject at a time (settings first).
      const tabs = page.locator(".ms [data-testid=ms-tabs] [role=tab]");
      if ((await tabs.allInnerTexts()).join("|") !== "设置|能力|记录|身份") throw new Error(`详细信息应分成 设置/能力/记录/身份 四页，实际「${(await tabs.allInnerTexts()).join("|")}」`);
      await page.locator(".ms [data-testid=ms-tabs]").scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "member-card-details");
      for (const t of ["skills", "records", "identity"]) {
        await clickVisible(page, page.locator(`.ms [data-testid=ms-tabs] [data-tab=${t}]`));
        await settle(page, 200);
        await shot(page, size, scheme, `member-card-details-${t}`);
      }
      await clickVisible(page, page.locator(".ms [data-testid=ms-tabs] [data-tab=settings]"));
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
      await face.locator("[data-group=face] .av-cell").nth(5).click();
      // a shape and something to wear: the flat cap and a bow
      await face.locator("[data-group=shape] .av-cell").nth(2).click();
      await face.locator("[data-group=acc] .av-cell", { hasNot: page.locator(":disabled") }).nth(3).click();
      // (a sprout needs room on top: the cone has none)
      await face.locator("[data-group=shape] .av-cell").nth(1).click();
      if (!(await face.locator("[data-group=acc] .av-cell").nth(1).isDisabled())) throw new Error("尖顶的菌盖上不该还能选小芽");
      await face.locator("[data-group=shape] .av-cell").nth(2).click();
      const preview = face.locator(".av-top .pa-sprite");
      if ((await preview.getAttribute("data-shape")) !== "2" || (await preview.getAttribute("data-acc")) !== "3") throw new Error(`预览应是扁平菌盖戴蝴蝶结，实际形状 ${await preview.getAttribute("data-shape")}、配饰 ${await preview.getAttribute("data-acc")}`);
      await settle(page, 200);
      await shot(page, size, scheme, "member-face");
      await face.getByRole("button", { name: "用这个头像" }).click();
      await face.waitFor({ state: "hidden", timeout: 5_000 });
      await page.waitForFunction(() => {
        const cap = document.querySelector(".ms .ms-face .pa")?.style.getPropertyValue("--av-cap");
        const sprite = document.querySelector(".ms .ms-face .pa-sprite");
        return cap && cap.toLowerCase() === "#e7a33e" && sprite?.getAttribute("data-shape") === "2" && sprite?.getAttribute("data-acc") === "3";
      // at once: the saved agent comes back with the save (not the next refresh)
      }, null, { timeout: 2_000 });
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
      }, null, { timeout: 2_000 });
      await clickVisible(page, page.locator(".ms .ms-face-btn"));
      await face.waitFor({ timeout: 5_000 });
      await face.getByRole("button", { name: "用回原来的" }).click();
      await face.getByRole("button", { name: "用这个头像" }).click();
      await face.waitFor({ state: "hidden", timeout: 5_000 });
      await page.locator(".ms .ms-face .pa-pic").waitFor({ state: "detached", timeout: 2_000 });
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });
    });

    // Faces: six cap shapes and a few things to wear, from each agent's id;
    // in one workspace no two drawn faces share a colour and a shape while
    // a free one is left. A sheet of 24 at a list's size and a card's.
    await step("成员 · 头像的形状和配饰", async () => {
      const N = 24;
      const routeURL = /\/app\/overview(\?|$)/;
      await page.route(routeURL, async (route) => {
        const response = await route.fetch();
        const json = await response.json();
        const hub = (json.hubs || []).find((h) => h.id === "hub_studio");
        const list = hub?.overview?.agents;
        const model = list?.find((x) => x.agent?.kind === "agent" && !x.agent.avatar);
        if (list && model) {
          for (let i = 0; i < N; i++) {
            const id = `ag_face_${(i * 7919 + 17).toString(36)}`;
            list.push({ ...model, agent: { ...model.agent, id, name: `face-${i}`, display_name: `样张 ${i + 1}`, avatar: undefined, created_at: new Date(Date.UTC(2026, 0, 1, 0, i)).toISOString() } });
          }
        }
        await route.fulfill({ response, json });
      });
      try {
        await go(page, "成员", narrow);
        const cards = page.locator(".mb-card", { hasText: "样张" });
        await page.waitForFunction((n) => document.querySelectorAll(".mb-card .pa-sprite").length >= n, N, { timeout: 15_000 });
        const faces = await cards.evaluateAll((els) => els.map((el) => {
          const pa = el.querySelector(".pa");
          const sprite = el.querySelector(".pa-sprite");
          return { cap: pa?.style.getPropertyValue("--av-cap"), shape: sprite?.getAttribute("data-shape"), acc: sprite?.getAttribute("data-acc") };
        }));
        if (faces.length !== N) throw new Error(`应有 ${N} 张样张，实际 ${faces.length}`);
        const pairs = new Set(faces.map((f) => `${f.cap}/${f.shape}`));
        const shapes = new Set(faces.map((f) => f.shape));
        const accs = new Set(faces.map((f) => f.acc));
        if (pairs.size !== N) throw new Error(`同一工作区里颜色加形状应各不相同，${N} 个里只有 ${pairs.size} 种`);
        if (shapes.size < 5 || accs.size < 5) throw new Error(`形状和配饰应多样：形状 ${shapes.size} 种，配饰 ${accs.size} 种`);
        // The sheet: the same faces (the App's own drawing), small and large.
        await page.evaluate((n) => {
          const src = [...document.querySelectorAll(".mb-card")].filter((c) => c.textContent.includes("样张")).map((c) => c.querySelector(".pa"));
          const sheet = document.createElement("div");
          sheet.className = "smoke-face-sheet";
          // (in the shell, where the colours are; a card's own background)
          const card = getComputedStyle(document.querySelector(".mb-card"));
          sheet.style.cssText = `position:fixed;inset:0;z-index:9999;background:${card.backgroundColor === "rgba(0, 0, 0, 0)" ? getComputedStyle(document.body).backgroundColor : card.backgroundColor};color:${card.color};font-size:12px;padding:24px;display:flex;flex-direction:column;gap:12px;overflow:auto`;
          for (const [size, gap] of [[22, 8], [48, 12]]) {
            const label = document.createElement("div");
            label.textContent = `${size}px`;
            sheet.append(label);
            const row = document.createElement("div");
            row.style.cssText = `display:flex;flex-wrap:wrap;gap:${gap}px;align-items:center`;
            for (const pa of src.slice(0, n)) {
              const c = pa.cloneNode(true);
              c.style.width = c.style.height = `${size}px`;
              c.querySelector(".pa-badge")?.remove();
              c.querySelector(".pa-state")?.remove();
              row.append(c);
            }
            sheet.append(row);
          }
          (document.querySelector(".app-shell") || document.body).append(sheet);
        }, N);
        await settle(page, 200);
        await shot(page, size, scheme, "avatar-sheet");
        await page.evaluate(() => document.querySelector(".smoke-face-sheet")?.remove());
      } finally {
        await page.unroute(routeURL);
      }
    });

    await step("成员 · 重新接入", async () => {
      // PM has a resume line last used in a sandbox: making a new one kills
      // it, the dialog shows the curl command, and revoking goes back to none.
      const pm = page.locator(".mb-card", { hasText: "PM (Codex)" });
      await pm.scrollIntoViewIfNeeded();
      await pm.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      await clickVisible(page, page.locator(".ms [data-testid=ms-tabs] [data-tab=identity]"));
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

    // An agent's own chat-app bots (ChatApps.tsx), in its details: PM has a
    // Feishu bot; binding Slack pastes two tokens and shows the /claim line;
    // binding Feishu shows a QR code; unbinding asks first.
    await step("成员 · 聊天软件机器人", async () => {
      await go(page, "成员", narrow);
      const pm = page.locator(".mb-card", { hasText: "PM (Codex)" });
      await pm.scrollIntoViewIfNeeded();
      await pm.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      await clickVisible(page, page.locator(".ms [data-testid=ms-tabs] [data-tab=identity]"));
      const row = page.locator('.ms [data-testid="chat-apps-row"]');
      await row.locator('.im-bot[data-bridge="feishu-pm-codex"] .dot.working').waitFor({ timeout: 5_000 });
      await row.scrollIntoViewIfNeeded();
      await row.getByRole("button", { name: "绑定 Slack" }).click();
      const form = page.locator('.app-modal [data-testid="slack-form"]');
      await form.locator('input[placeholder="xoxb-…"]').fill("xoxb-1-2-3");
      await form.locator('input[placeholder="xapp-…"]').fill("xapp-1-2-3");
      await settle(page, 200);
      await shot(page, size, scheme, "member-chat-apps-slack");
      await form.getByRole("button", { name: "连接" }).click();
      await row.locator(".im-claim code", { hasText: "/claim" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "member-chat-apps");
      // Unbind Feishu, then bind it again with a QR code.
      await row.locator('.im-bot[data-bridge="feishu-pm-codex"]').getByRole("button", { name: "解绑" }).click();
      const confirm = page.locator(".app-modal.app-confirm");
      await confirm.getByRole("button", { name: "解绑" }).click();
      await row.locator('.im-bot[data-bridge="feishu-pm-codex"]').waitFor({ state: "detached", timeout: 5_000 });
      await row.getByRole("button", { name: "绑定飞书" }).click();
      await page.locator('.app-modal [data-testid="feishu-scan"] img.im-qr').waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "member-chat-apps-feishu");
      await page.locator('.app-modal [data-testid="feishu-scan"]').getByRole("button", { name: "取消" }).click();
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

    // An agent in its own session (inbox wait: a Claude cloud session) has
    // no computer to read a quota on; its session reports it
    // (myco agent runtime quota: source_kind cloud_session, no machine_id).
    // The member card, its card's 额度 and the composer all show it.
    await step("成员 · 会话上报的额度", async () => {
      const at = new Date(Date.now() - 5 * 60e3).toISOString();
      const quota = {
        source: "claude", provider: "claude", source_kind: "cloud_session", status: "ok", at, checked_at: at,
        windows: [
          { name: "five_hour", used: 0.17, status: "allowed", duration_sec: 18000, resets_at: new Date(Date.now() + 3 * 3600e3).toISOString() },
          { name: "seven_day", used: 0.48, status: "allowed", duration_sec: 604800, resets_at: new Date(Date.now() + 4 * 86400e3).toISOString() },
        ],
      };
      const put = (v) => {
        if (Array.isArray(v)) { v.forEach(put); return; }
        if (!v || typeof v !== "object") return;
        if (v.name === "grok-agent-7c2e" && v.runtime) v.runtime = { ...v.runtime, quota };
        for (const x of Object.values(v)) put(x);
      };
      const routeURL = /\/app\/overview(\?|$)/;
      await page.route(routeURL, async (route) => {
        const response = await route.fetch();
        const json = await response.json();
        put(json);
        await route.fulfill({ response, json });
      });
      try {
        await go(page, "成员", narrow);
        const card = page.locator(".mb-card", { hasText: "Myco (Grok)" });
        const tag = card.locator(".app-quota-tag");
        await tag.waitFor({ timeout: 15_000 });
        if (!compactText(await tag.innerText()).includes("剩 52%")) throw new Error(`会话上报的额度应在成员卡上显示最紧的窗口「剩 52%」，实际「${await tag.innerText()}」`);
        if (!(await tag.getAttribute("title")).includes("会话上报")) throw new Error(`额度应说明是会话上报的，实际「${await tag.getAttribute("title")}」`);
        await card.click();
        await page.locator(".ms").waitFor({ timeout: 10_000 });
        await clickVisible(page, page.locator(".ms .ms-fold"));
        const body = page.locator(".ms .ms-fold-body");
        await body.getByText("模型由它自己的会话管理，myco 改不了；额度是它的会话上报的。").waitFor({ timeout: 5_000 });
        const rows = compactText(await body.locator(".app-quota").allInnerTexts().then((x) => x.join(" | ")));
        if (!rows.includes("剩 83%") || !rows.includes("剩 52%")) throw new Error(`卡片的额度应列出两个窗口，实际「${rows}」`);
        await settle(page, 200);
        await shot(page, size, scheme, "member-quota-session");
        await page.keyboard.press("Escape");
        await page.locator(".ms").waitFor({ state: "hidden" });
        await openMember(page, narrow, "聊天", "Myco (Grok)");
        const pill = page.locator(".conv-detail [data-testid=quota-pill]");
        await pill.waitFor({ timeout: 10_000 });
        if (!compactText(await pill.innerText()).includes("剩 52%")) throw new Error(`输入框的额度应是「剩 52%」，实际「${await pill.innerText()}」`);
      } finally {
        await page.unroute(routeURL);
      }
      // (back where the next step starts: the members page)
      await go(page, "成员", narrow);
      await page.locator(".mb-card", { hasText: "PM (Codex)" }).waitFor({ timeout: 15_000 });
    });

    await step("成员 · 模型和额度", async () => {
      // A manager picks a model and an effort: each goes to the App backend
      // at once with both fields. The quota rows say whose usage it is
      // (provider · plan · computer), how much each window has left and
      // when it resets; a nearly-used one is amber. The member cards carry
      // the tightest window; a login to fix and a failed read say so.
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
      const pmTag = pm.locator(".app-quota-tag");
      await pmTag.waitFor({ timeout: 5_000 });
      if ((await pmTag.locator(".app-quota-tag-text").innerText()).trim() !== "剩 5%" || (await pmTag.getAttribute("data-quota")) !== "tight") throw new Error(`成员卡片应写剩 5%（琥珀色），实际是「${await pmTag.innerText()}」`);
      const pmTitle = await pmTag.getAttribute("title") || "";
      if (!pmTitle.includes("Codex · ChatGPT 订阅 · Studio Mac") || !pmTitle.includes("小时后重置")) throw new Error(`悬停应写来源和重置：${pmTitle}`);
      const mcpTag = page.locator(".mb-card", { hasText: "MCP Claude" }).locator(".app-quota-tag");
      if (await page.locator(".mb-card", { hasText: "Release (Claude 云端)" }).locator(".app-quota-tag").count()) throw new Error("没有可读额度的连接不该显示额度标记");
      if ((await mcpTag.locator(".app-quota-tag-text").innerText()).trim() !== "需重新登录") throw new Error(`登录过期应写需重新登录，实际是「${await mcpTag.innerText()}」`);
      await pm.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const body = page.locator(".ms .ms-fold-body");
      await body.locator(".app-kv", { hasText: "模型" }).first().waitFor({ timeout: 5_000 });

      const modelSelect = body.locator(".app-kv", { hasText: "模型" }).first().locator("select");
      const effortSelect = body.locator(".app-kv", { hasText: "推理强度" }).first().locator("select");
      await modelSelect.waitFor({ timeout: 5_000 });
      await effortSelect.waitFor({ timeout: 5_000 });
      // The models are what the computer's Codex lists (its runner reports them).
      const offered = await modelSelect.locator("option").allTextContents();
      if (!offered.some((o) => o.includes("GPT-6.1 Sol"))) throw new Error(`模型应列出电脑上 Codex 给的，实际是「${offered.join("、")}」`);
      await modelSelect.selectOption("gpt-6.1-sol");
      await lastBody((x) => x.agent === "pm-codex" && x.body.model === "gpt-6.1-sol", "模型的修改");
      await page.waitForFunction(() => (document.querySelector(".ms .ms-fold-body")?.textContent || "").includes("现在用的：gpt-6.1-sol（"), null, { timeout: 5_000 });
      await effortSelect.selectOption("low");
      const effortBody = await lastBody((x) => x.agent === "pm-codex" && x.body.effort === "low", "推理强度的修改");
      if (effortBody.body.model !== "gpt-6.1-sol") {
        throw new Error(`改推理强度时应带上当前模型，实际是「${effortBody.body.model}」`);
      }

      const five = body.locator(".app-quota", { hasText: "5 小时" });
      await five.waitFor({ timeout: 5_000 });
      const fiveText = compactText(await five.innerText());
      if (!fiveText.includes("剩 5%") || !fiveText.includes("小时后重置")) {
        throw new Error(`5 小时窗口应写剩 5% 和几小时后重置，实际是「${fiveText}」`);
      }
      const source = compactText(await body.locator(".app-quota-source").innerText());
      if (!source.includes("Codex · ChatGPT 订阅 · Studio Mac")) throw new Error(`应写来源 · 套餐 · 电脑，实际是「${source}」`);
      if (!(await five.getAttribute("class") || "").includes("is-tight")) {
        throw new Error("用了 95% 的窗口应标成琥珀色");
      }
      const amber = await color("--amber");
      const fiveColor = await five.locator(".app-kv-value").evaluate((el) => getComputedStyle(el).color);
      if (fiveColor !== amber) throw new Error(`用了 95% 的窗口应显示成琥珀色，实际是 ${fiveColor}`);
      const sevenText = compactText(await body.locator(".app-quota", { hasText: "每周" }).innerText());
      if (!sevenText.includes("剩 38%")) throw new Error(`每周窗口应写剩 38%，实际是「${sevenText}」`);
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
      await page.locator(".ms .ms-fold-body .app-quota-stale", { hasText: "取到的数据" }).waitFor({ timeout: 5_000 });
      await page.locator(".ms .ms-fold-body .app-quota-status", { hasText: "获取失败" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "member-quota-spent");
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });

      // A sign-in that expired: told to sign in again, the last numbers
      // kept and marked old (never shown as a full quota).
      const mcp = page.locator(".mb-card", { hasText: "MCP Claude" });
      await mcp.scrollIntoViewIfNeeded();
      await mcp.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const mcpBody = page.locator(".ms .ms-fold-body");
      await mcpBody.locator(".app-quota-status.is-action", { hasText: "需要在 CLI 重新登录" }).waitFor({ timeout: 5_000 });
      const mcpWeek = compactText(await mcpBody.locator(".app-quota", { hasText: "每周" }).innerText());
      if (!mcpWeek.includes("剩 45%")) throw new Error(`登录过期时应保留上次的数字，实际是「${mcpWeek}」`);
      if (!(await mcpBody.locator(".app-quota", { hasText: "每周" }).getAttribute("class") || "").includes("is-stale")) throw new Error("保留的旧数字应标成旧的");
      await mcpBody.locator(".app-quota-stale", { hasText: "取到的数据" }).waitFor({ timeout: 5_000 });
      await mcpBody.locator(".app-quota-status").scrollIntoViewIfNeeded();
      await settle(page, 200);
      await shot(page, size, scheme, "member-quota-login");
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });

      // A model's own bucket folds under the windows.
      const docs = page.locator(".mb-card", { hasText: "docs-claude" });
      await docs.scrollIntoViewIfNeeded();
      await docs.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const docsBody = page.locator(".ms .ms-fold-body");
      const fold = docsBody.locator(".app-quota-more", { hasText: "按模型的 1 个窗口" });
      await fold.waitFor({ timeout: 5_000 });
      if (await docsBody.locator(".app-quota", { hasText: "Opus" }).count()) throw new Error("按模型的窗口应先收起");
      await fold.click();
      const opus = compactText(await docsBody.locator(".app-quota", { hasText: "每周（Opus）" }).innerText());
      if (!opus.includes("剩 29%")) throw new Error(`展开后应看到 Opus 的窗口，实际是「${opus}」`);
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
      await inbox.getByText("模型和额度由它自己的会话管理；会话上报额度后，这里就能看到。").waitFor({ timeout: 5_000 });
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

    await step("运行设置 · MCP 工具", async () => {
      await go(page, "成员", narrow);
      const card = page.locator(".mb-card", { hasText: "MCP Claude" }).first();
      await card.scrollIntoViewIfNeeded(); await card.click();
      await page.locator(".ms").waitFor({ timeout: 10000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const toggle = page.locator('.ms [role="switch"][aria-label="允许 MCP 工具"]');
      await toggle.waitFor({ timeout: 5000 });
      if ((await toggle.getAttribute("aria-checked")) === "true") throw new Error("MCP 工具应默认关闭");
      await toggle.click();
      await page.waitForFunction(() => document.querySelector('.ms [role="switch"][aria-label="允许 MCP 工具"]')?.getAttribute("aria-checked") === "true");
      const enabled = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/run-settings`)).json();
      if (enabled.last?.agent !== "mcp-claude" || enabled.last.body.mcp_tools !== true) throw new Error("MCP 开关打开后未保存");
      if (size.width === 1200 && scheme === "light") await shot(page, size, scheme, "member-mcp-tools-on");
      await toggle.click();
      await page.waitForFunction(() => document.querySelector('.ms [role="switch"][aria-label="允许 MCP 工具"]')?.getAttribute("aria-checked") === "false");
      const disabled = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/run-settings`)).json();
      if (disabled.last?.body.mcp_tools !== false) throw new Error("MCP 开关关闭后未保存");
      await page.keyboard.press("Escape"); await page.locator(".ms").waitFor({ state: "hidden" });
    });

    // An agent on another vendor with an API key: the models are that
    // vendor's, the line says where they came from and when, and its owner
    // can have them listed again.
    await step("成员 · 服务商的模型列表", async () => {
      await go(page, "成员", narrow);
      const card = page.locator(".mb-card", { hasText: "MCP Claude" }).first();
      await card.scrollIntoViewIfNeeded(); await card.click();
      await page.locator(".ms").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator(".ms .ms-fold"));
      const body = page.locator(".ms .ms-fold-body");
      const offered = await body.locator(".app-kv", { hasText: "模型" }).first().locator("select option").allTextContents();
      if (!offered.some((o) => o.includes("DeepSeek V4 Flash (deepseek-v4-flash)")) || offered.some((o) => o === "fable" || o === "haiku")) {
        throw new Error(`应列出服务商给的模型，不再列 Claude 的别名，实际「${offered.join("、")}」`);
      }
      const source = body.locator("[data-testid=run-models-source]");
      await source.waitFor({ timeout: 5_000 });
      const said = compactText(await source.innerText());
      if (!/模型列表从 .+ 读取 · \d+ 分钟前/.test(said)) throw new Error(`应写模型列表从哪读的、多久前，实际「${said}」`);
      const again = source.getByRole("button", { name: "重新读取" });
      await source.scrollIntoViewIfNeeded();
      await settle(page, 200);
      await shot(page, size, scheme, "member-provider-models");
      await clickVisible(page, again);
      let sent;
      for (let i = 0; i < 50 && !sent; i++) {
        const { last } = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/run-settings`)).json();
        if (last?.agent === "mcp-claude" && last.body.models_refresh) sent = last; else await sleep(100);
      }
      if (!sent) throw new Error("点重新读取后 App 后端没有收到请求");
      await page.waitForFunction(() => /刚刚/.test(document.querySelector(".ms [data-testid=run-models-source]")?.textContent || ""), null, { timeout: 10_000 });
      await source.getByRole("button", { name: "重新读取" }).waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape"); await page.locator(".ms").waitFor({ state: "hidden" });
    });

    await step("成员 · 工具", async () => {
      await go(page, "成员", narrow);
      const card = page.locator(".mb-card", { hasText: "MCP Claude" }).first();
      await card.scrollIntoViewIfNeeded(); await card.click();
      await clickVisible(page, page.locator(".ms .ms-fold"));
      await clickVisible(page, page.locator(".ms [data-testid=ms-tabs] [data-tab=skills]"));
      const tools = page.locator('[data-testid="agent-tools"]');
      await tools.waitFor({ timeout: 10_000 });
      for (const want of ["filesystem", "project-docs", "release-check", "工作区权限下用不了"]) {
        if (!(await tools.innerText()).includes(want)) throw new Error(`成员工具缺少 ${want}`);
      }
      await tools.scrollIntoViewIfNeeded();
      if ((size.width === 1200 && scheme === "light") || (size.width === 760 && scheme === "dark")) await shot(page, size, scheme, "member-tools");
      await page.keyboard.press("Escape"); await page.locator(".ms").waitFor({ state: "hidden" });
    });

    await step("电脑 · 工具清单", async () => {
      await go(page, "电脑", narrow);
      await clickVisible(page, page.locator(".pc-card", { hasText: "Studio Mac" }));
      await pcTab(page, "AI 和工具");
      const tools = page.locator('[data-testid="computer-tools-claude"]');
      await tools.waitFor({ timeout: 10_000 });
      await tools.locator("summary").click();
      for (const want of ["filesystem", "figma", "myco 装的", "配置文件没读懂"]) {
        if (!(await tools.innerText()).includes(want)) throw new Error(`电脑工具清单缺少 ${want}`);
      }
      if ((size.width === 1200 && scheme === "light") || (size.width === 760 && scheme === "dark")) await shot(page, size, scheme, "computer-tools");
      await go(page, "成员", narrow);
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
      await permission.getByText(/现在用的：工作区（App）/).waitFor({ timeout: 5000 });
      if (!(await page.locator(".ms .ms-fold-body").innerText()).includes("改了从下次唤醒起生效")) throw new Error("需要说明下次唤醒生效");
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
      // A card each: what it is, whether it is on, what runs there.
      await page.locator(".pc-card").first().waitFor({ timeout: 20_000 });
      // An agent on that computer must sign in again to read its quota:
      // its card says so, quietly (before another's nearly used window).
      await page.locator(".pc-card", { hasText: "Studio Mac" }).locator(".pc-quota", { hasText: "需重新登录" }).waitFor({ timeout: 10_000 });
      await settle(page);
      await shot(page, size, scheme, "computers");
      const back = () => navBackClick(page);
      // A card opens the computer: its agents and terminals, and what it
      // allows and its AIs (asked of it).
      await clickVisible(page, page.locator(".pc-card", { hasText: "Studio Mac" }));
      const detail = page.locator("[data-testid=pc-detail]");
      await detail.locator(".pc-row").first().waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "computer-detail");
      await back();
      await clickVisible(page, page.locator(".pc-card", { hasText: "VPS 东京" }));
      // What it allows and its AIs are tabs of its page.
      await pcTab(page, "AI 和工具");
      await detail.locator("[data-testid=pc-ais] .pcd-line", { hasText: "Codex" }).filter({ hasText: "没装" }).waitFor({ timeout: 10_000 });
      await pcTab(page, "权限");
      await detail.locator("[data-testid=pc-details] .pcd-line", { hasText: "~/code/demo-team" }).waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "computer-details");
      // Its folders are its own rule; this App reaches it over SSH, so it
      // changes them there (and puts them back: the other windows share the mock).
      const dirsBox = detail.locator("[data-testid=machine-dirs]");
      const folder = `~/code/acme-${size.width}-${scheme}`;
      await dirsBox.getByRole("textbox", { name: "要允许的文件夹" }).fill(folder);
      await clickVisible(page, dirsBox.getByRole("button", { name: "允许" }));
      const chip = dirsBox.locator(".mdirs-chip", { hasText: folder });
      await chip.waitFor({ timeout: 10_000 });
      if (!(await dirsBox.locator(".mdirs-note").innerText()).includes("SSH tokyo-vps")) throw new Error("应说明是通过 SSH 在那台电脑上改的");
      await settle(page, 200);
      await shot(page, size, scheme, "computer-dirs");
      await clickVisible(page, chip.getByRole("button", { name: `不再允许 ${folder}` }));
      await chip.waitFor({ state: "detached", timeout: 10_000 });
      await back();
      // A computer that has not allowed terminals says how to allow it: on
      // that computer, never from here.
      await clickVisible(page, page.locator(".pc-card", { hasText: "Office PC" }));
      await pcTab(page, "权限");
      await detail.locator("[data-testid=shell-ask] code", { hasText: "myco machine shell-access --allow" }).waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "computer-permission");
      await back();
      // Renaming a computer, from its page: another computer's name is refused.
      await clickVisible(page, page.locator(".pc-card", { hasText: "Bench Linux" }));
      await pcMore(page, "改名");
      const form = page.locator(".pc-rename-form");
      const nameBox = form.getByRole("textbox", { name: "电脑的名字" });
      await nameBox.fill("office pc");
      await form.locator(".app-field-hint.is-bad").waitFor({ timeout: 5_000 });
      if (await form.getByRole("button", { name: "保存" }).isEnabled()) throw new Error("和别的电脑重名时不该能保存");
      await settle(page, 200);
      await shot(page, size, scheme, "computer-rename-clash");
      await nameBox.fill(`测试机 ${size.width} ${scheme}`);
      await nameBox.press("Enter");
      await detail.locator(".pcd-titles h1", { hasText: `测试机 ${size.width} ${scheme}` }).waitFor({ timeout: 10_000 });
      // Back to its name for the steps after this one.
      await pcMore(page, "改名");
      await nameBox.fill("Bench Linux");
      await nameBox.press("Enter");
      await detail.locator(".pcd-titles h1", { hasText: "Bench Linux" }).waitFor({ timeout: 10_000 });
      await back();
      await page.locator(".pc-card", { hasText: "Bench Linux" }).waitFor({ timeout: 10_000 });
      // Who may use a computer, which projects have it, and removing it are
      // in one place, a click from its row.
      // (The folder added in the project step, at full width; else the first.)
      // (terminals of a known computer are on its page: open one when the
      // list has none of its own)
      const someRow = async () => {
        if (!(await page.locator(".pc-term:visible").count())) await clickVisible(page, page.locator(".pc-card", { hasText: "Studio Mac" }));
        await page.locator(".pc-term").first().waitFor({ timeout: 10_000 });
      };
      const isWsl = !narrow && (await page.locator(".pc-term", { hasText: `wsl-${size.width}-${scheme}` }).count()) > 0;
      if (!isWsl) await someRow();
      const pcRow = isWsl ? page.locator(".pc-term", { hasText: `wsl-${size.width}-${scheme}` }) : page.locator(".pc-term").first();
      await clickVisible(page, pcRow.getByRole("button", { name: "设置" }));
      const set = page.locator("[data-testid=computer-settings]");
      await set.waitFor({ timeout: 10_000 });
      await set.locator(".pcs-agent").first().waitFor({ timeout: 10_000 });
      if (isWsl) await set.locator(".pcs-project").first().waitFor({ timeout: 10_000 });
      await clickVisible(page, set.locator(".pcs-agent").last());
      await clickVisible(page, set.getByRole("button", { name: "保存" }));
      await set.getByText("已保存").waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "computer-settings");
      if (isWsl) {
        await clickVisible(page, set.getByRole("button", { name: "移除这个终端" }));
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
      await someRow();
      await clickVisible(page, page.locator(".pc-term").first().getByRole("button", { name: "打开", exact: true }));
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
    await step("项目 · 协调者定时任务", async () => {
      await go(page,"官网改版",narrow);
      await clickVisible(page,page.locator(".sp-head").getByRole("button",{name:"项目资料"}));
      await clickVisible(page,page.locator("[data-testid=project-tabs]").getByRole("tab",{name:/^成员/}));
      const group=page.locator("[data-testid=coordinator-permissions]");
      const permission=group.locator("[data-testid=project-automation-permission]");await permission.waitFor();
      if(!await group.locator("[data-testid=project-agent-invites]").count()||!await group.locator("[data-testid=project-machines]").count())throw Error("Coordinator permissions must be grouped together");
      const toggle=permission.getByRole("switch");if(await toggle.getAttribute("aria-checked")!=="false")throw Error("Scheduling permission must default off");
      await clickVisible(page,toggle);await page.waitForFunction(()=>document.querySelector("[data-testid=project-automation-permission] [role=switch]")?.getAttribute("aria-checked")==="true");
      await clickVisible(page,toggle);await page.waitForFunction(()=>document.querySelector("[data-testid=project-automation-permission] [role=switch]")?.getAttribute("aria-checked")==="false");
      await group.scrollIntoViewIfNeeded();await settle(page,200);await shot(page,size,scheme,"coordinator-permissions");
      await navBackClick(page);await page.locator(".sp-home .conv-detail").waitFor();
      const ask=async()=>{const response=await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({agent:"pm-codex",kind:"project_automation",payload:{project:"website",action:"create",input:{name:"发布检查",body:"核对项目发布状态，列出检查清单。",to:"docs-claude",cron:"0 9 * * *",tz:"Asia/Shanghai",starts_at:"2026-11-26T00:00:00Z",ends_at:"2026-11-29T00:00:00Z",once:true}}})});if(!response.ok)throw Error("Automation approval mock failed")};
      const card=page.locator(".sp-home [data-testid=approvals] .sa-approval");await ask();await card.waitFor();
      const text=await card.innerText();if(!text.includes("发布检查")||!text.includes("只跑一次")||!text.includes("2026-11-26"))throw Error("Approval must show the specific task and timing");
      await clickVisible(page,card.getByText("查看任务内容",{exact:true}));await card.locator(".sa-approval-content",{hasText:"核对项目发布状态"}).waitFor();
      await shot(page,size,scheme,"project-automation-approval");await clickVisible(page,card.getByRole("button",{name:"不同意",exact:true}));await card.waitFor({state:"hidden"});
      await ask();await card.waitFor();await clickVisible(page,card.getByRole("button",{name:"同意",exact:true}));await card.waitFor({state:"hidden"});
      // What you decided shows in the chat as one quiet line, in words (the
      // note the hub sends the agent carries ids, for the agent).
      const line=page.locator(".sp-home .conv-detail .conv-decision",{hasText:"同意了：创建项目「website」的定时任务「发布检查」"});
      await line.waitFor({timeout:15_000});
      if(/\[myco\]|apr_|aut_/.test(await line.innerText()))throw Error("同意后的那行不该带 [myco] 和编号");
      if(await page.locator(".sp-home .conv-detail .conv-msg",{hasText:"[myco]"}).count())throw Error("同意的通知不该再是一条备注消息");
      await line.scrollIntoViewIfNeeded();await settle(page,200);await shot(page,size,scheme,"approval-decided-line");
      // Two at once, the workspace slow to answer: deciding one leaves the
      // other to click (they all went grey until it answered).
      const slowDecide=async(route)=>{await new Promise((ok)=>setTimeout(ok,2500));await route.continue();};
      await page.route("**/app/hubs/*/approvals/*/approve",slowDecide);
      await ask();await ask();await card.nth(1).waitFor();
      await clickVisible(page,card.first().getByRole("button",{name:"同意",exact:true}));
      await card.first().locator(".app-spinner").waitFor({timeout:2_000});
      if(await card.nth(1).getByRole("button",{name:"同意",exact:true}).isDisabled())throw Error("同意了一条，另一条的「同意」不该变灰");
      await clickVisible(page,card.nth(1).getByRole("button",{name:"同意",exact:true}));
      await page.waitForFunction(()=>!document.querySelector(".sp-home [data-testid=approvals] .sa-approval"),null,{timeout:10_000});
      await page.unroute("**/app/hubs/*/approvals/*/approve",slowDecide);
    });

    await step("自动化 · 起止日期",async()=>{
      await go(page,"自动化",narrow);await clickVisible(page,page.locator(".ph").getByRole("button",{name:"新建自动化"}));
      const ed=page.locator("[data-testid=automation-editor]");await ed.waitFor();
      const name=`定期发布检查 ${size.width} ${scheme}`;await ed.getByPlaceholder("比如：每天早上汇总 issue").fill(name);await ed.locator("textarea").fill("核对项目发布状态");
      await ed.getByLabel("开始时间",{exact:true}).fill("2026-11-26T09:00");await ed.getByLabel("结束时间",{exact:true}).fill("2026-11-28T18:00");
      await ed.getByRole("checkbox",{name:/^只跑一次/}).check();await ed.locator("[data-testid=automation-date-editor]").scrollIntoViewIfNeeded();
      await shot(page,size,scheme,"automation-date-editor");
      const response=page.waitForResponse(r=>r.url().includes("/automations")&&r.request().method()==="POST");await clickVisible(page,ed.getByRole("button",{name:"创建",exact:true}));
      const json=await(await response).json();if(!json.automation.once||!json.automation.starts_at||!json.automation.ends_at)throw Error("Date/once fields lost when saving");
      await ed.waitFor({state:"detached"});const row=page.locator(".au-row",{hasText:name});await row.locator("[data-testid=automation-bounds]",{hasText:"只跑一次"}).waitFor();
      await shot(page,size,scheme,"automation-date-list");
    });

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
      await clickVisible(page, row.getByRole("button", { name: "运行", exact: true }));
      await row.locator(".au-row-status", { hasText: "已发送" }).waitFor({ timeout: 10_000 });
      // Off and on again: under ⋯ (no switch beside the buttons).
      if (await row.getByRole("switch").count()) throw new Error("行上不该再有开关");
      await clickVisible(page, row.locator(".wf-more"));
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "暂停" }));
      await row.locator(".au-row-status", { hasText: "已暂停" }).waitFor({ timeout: 10_000 });
      await clickVisible(page, row.locator(".wf-more"));
      await clickVisible(page, page.locator(".conv-menu").getByRole("menuitem", { name: "启用" }));
      await row.locator(".au-row-status", { hasText: "已暂停" }).waitFor({ state: "detached", timeout: 10_000 });
      // Right-click: the same, and delete.
      await row.click({ button: "right", position: { x: 120, y: 12 } });
      await clickVisible(page, page.getByRole("menuitem", { name: "删除" }));
      await clickVisible(page, page.locator(".app-modal.app-confirm").getByRole("button", { name: "删除" }));
      await row.waitFor({ state: "detached", timeout: 10_000 });
    });

    // An automation opened: its settings, then every run; a webhook run
    // shows what the call carried, and 看对话 opens the task it sent.
    await step("自动化 · 详情", async () => {
      await go(page, "自动化", narrow);
      const row = page.locator("[data-testid=automations] .au-row", { hasText: "CI 失败时排查" });
      await row.waitFor({ timeout: 20_000 });
      await clickVisible(page, row.locator(".au-row-main"));
      const det = page.locator("[data-testid=automation-detail]");
      await det.waitFor({ timeout: 10_000 });
      const set = det.locator("[data-testid=automation-settings]");
      for (const w of ["发给", "Webhook 地址", "让它做什么", "{{payload}}"]) if (!(await set.innerText()).includes(w)) throw new Error(`设置卡里应有「${w}」`);
      const runs = det.locator("[data-testid=automation-run]");
      await runs.first().waitFor({ timeout: 10_000 });
      if ((await runs.count()) < 3) throw new Error("应列出每一次运行");
      await settle(page, 200);
      await shot(page, size, scheme, "automation-detail");
      // Only the skipped ones.
      await clickVisible(page, det.locator(".aud-filters .wf-chip", { hasText: "跳过" }));
      if ((await runs.count()) !== 1 || !(await runs.first().innerText()).includes("跳过")) throw new Error("「跳过」只看跳过的");
      await clickVisible(page, det.locator(".aud-filters .wf-chip", { hasText: "全部" }));
      // What the newest call carried.
      const first = runs.first();
      await clickVisible(page, first.getByRole("button", { name: "调用内容" }));
      const call = first.locator("[data-testid=automation-call]");
      await call.locator("pre", { hasText: "workflow_run" }).waitFor({ timeout: 10_000 });
      if (!(await call.innerText()).includes("X-Github-Event")) throw new Error("应显示记下的请求头");
      await settle(page, 200);
      await shot(page, size, scheme, "automation-call");
      // Its chat, at the task it sent.
      await clickVisible(page, first.getByRole("button", { name: "看对话" }));
      await page.locator(".conv-detail", { hasText: "CI 构建失败了" }).first().waitFor({ timeout: 15_000 });
      if (narrow) await sidebar(page, false);
    });

    // An automation can start a workflow instead of sending a task: its
    // inputs filled from the webhook call; a run opens the workflow run.
    await step("自动化 · 启动工作流", async () => {
      await go(page, "自动化", narrow);
      const row = page.locator("[data-testid=automations] .au-row", { hasText: "新 issue 走写-审-改" });
      await row.waitFor({ timeout: 20_000 });
      if (!(await row.innerText()).includes("启动工作流")) throw new Error("启动工作流的自动化应写清启动哪个");
      await clickVisible(page, row.locator(".au-row-main"));
      const det = page.locator("[data-testid=automation-detail]");
      const set = det.locator("[data-testid=automation-settings]");
      await set.waitFor({ timeout: 10_000 });
      for (const w of ["启动工作流", "写-审-改", "{{payload.issue.title}}"]) if (!(await set.innerText()).includes(w)) throw new Error(`设置卡里应有「${w}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "automation-workflow");
      await clickVisible(page, det.locator("[data-testid=automation-run]").first().getByRole("button", { name: "看运行" }));
      await page.locator("[data-testid=workflow-run] [data-testid=workflow-step]").first().waitFor({ timeout: 10_000 });
      // The editor: start a workflow, its inputs as templates.
      await go(page, "自动化", narrow);
      await clickVisible(page, page.locator(".ph").getByRole("button", { name: "新建自动化" }));
      const ed = page.locator("[data-testid=automation-editor]");
      await ed.waitFor({ timeout: 10_000 });
      await clickVisible(page, ed.getByRole("radio", { name: "启动一个工作流" }));
      await ed.getByRole("combobox", { name: "哪个工作流" }).selectOption({ label: "改完跑测试" });
      const task = ed.locator("label.app-field", { hasText: "要改什么" }).locator("input");
      await task.waitFor({ timeout: 10_000 });
      await clickVisible(page, ed.locator(".au-kind", { hasText: "Webhook" }));
      await task.fill("修 CI：{{payload.workflow_run.name}}");
      await ed.locator(".au-payload", { hasText: "payload.issue.title" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "automation-editor-workflow");
      await clickVisible(page, ed.getByRole("button", { name: "取消" }));
      await ed.waitFor({ state: "detached", timeout: 5_000 });
    });

    // 工作流: decisions waiting for you on top, then the workflows, then
    // ready-made ones (pick who does what) and "have an agent write one".
    await step("工作流", async () => {
      const empty = /\/app\/hubs\/[^/]+\/workflows$/;
      await page.route(empty, (r) => (r.request().method() === "GET" ? r.fulfill({ json: { workflows: [] } }) : r.continue()));
      await go(page, "工作流", narrow);
      const none = page.locator("[data-testid=workflows-empty]");
      await none.waitFor({ timeout: 20_000 });
      if ((await page.locator("[data-testid=workflow-templates] .au-template").count()) < 5) throw new Error("空的工作流页应给几个现成的和「让 agent 写一个」");
      await settle(page, 200);
      await shot(page, size, scheme, "workflows-empty");
      await page.unroute(empty);
      await go(page, "聊天", narrow);

      await go(page, "工作流", narrow);
      const list = page.locator("[data-testid=workflows]");
      await list.locator(".au-row", { hasText: "写-审-改" }).waitFor({ timeout: 20_000 });
      const dec = page.locator("[data-testid=workflow-decisions] [data-testid=workflow-decision]").first();
      await dec.waitFor({ timeout: 10_000 });
      if (!(await dec.innerText()).includes("就这样吗")) throw new Error("等你决定的卡片应写清问的是什么");
      const badge = page.locator(".sp-nav-workflows .wf-nav-badge");
      if (!narrow && !(await badge.count())) throw new Error("工作流导航上应标出有几个决定在等你");
      await settle(page, 300);
      await shot(page, size, scheme, "workflows");
      // The catalog keeps live runs visible even when a newer run completed,
      // including after disabling the workflow. Counts are not a recent sample.
      let catalogRows;
      await page.route(empty, async (route) => {
        if (route.request().method() !== "GET") return route.continue();
        const response = await route.fetch();
        const json = await response.json();
        if (!catalogRows) {
          const base = json.workflows[0];
          const counts = { total: 1200, running: 1, waiting: 1, succeeded: 1100, failed: 90, cancelled: 8 };
          const make = (id, title, enabled, active, run_counts, status, project) => ({ ...base, id, title, name: id, enabled, active, run_counts, project, last_run: status ? { ...base.last_run, status } : undefined });
          catalogRows = [
            make("wf_live_demo", "运行中的发布流程", false, 2, counts, "succeeded", "demo"),
            make("wf_failed_demo", "失败的检查流程", true, 0, { ...counts, total: 1, running: 0, waiting: 0, succeeded: 0, failed: 1, cancelled: 0 }, "failed", "demo"),
            make("wf_new_demo", "还没运行的流程", true, 0, { total: 0, running: 0, waiting: 0, succeeded: 0, failed: 0, cancelled: 0 }, undefined),
          ];
        }
        await route.fulfill({ json: { workflows: catalogRows } });
      });
      const liveRow = list.locator(".au-row", { hasText: "运行中的发布流程" });
      await liveRow.waitFor({ timeout: 12_000 });
      if (!(await liveRow.locator("[data-testid=workflow-active]").innerText()).includes("2 次进行中")) throw new Error("较早且已停用的运行仍应显示进行中");
      const tally = await liveRow.locator("[data-testid=workflow-counts]").innerText();
      if (!tally.includes("1,200") || !tally.includes("1,100") || !tally.includes("90") || !tally.includes("8")) throw new Error("累计次数不能只数最近一页运行");
      const filters = page.locator("[data-testid=workflow-filters]");
      const chips = page.getByRole("group", { name: "工作流状态", exact: true });
      await chips.getByRole("button", { name: /^进行中/ }).click();
      if (await list.locator(".au-row").count() !== 1) throw new Error("进行中筛选应保留已停用但仍有活动运行的工作流");
      await chips.getByRole("button", { name: /^等决定/ }).click();
      if (await list.locator(".au-row").count() !== 1) throw new Error("等待决定计数应独立于最近成功记录");
      await chips.getByRole("button", { name: /^最近失败/ }).click();
      await list.locator(".au-row", { hasText: "失败的检查流程" }).waitFor();
      await chips.getByRole("button", { name: /^未运行/ }).click();
      await list.locator(".au-row", { hasText: "还没运行的流程" }).waitFor();
      await chips.getByRole("button", { name: /^已停用/ }).click();
      await liveRow.waitFor();
      await chips.getByRole("button", { name: /^全部/ }).click();
      await filters.getByRole("searchbox").fill("失败的检查");
      if (await list.locator(".au-row").count() !== 1) throw new Error("工作流搜索应按名称过滤");
      await filters.getByRole("searchbox").fill("");
      await filters.getByLabel("工作流项目", { exact: true }).selectOption("!none");
      await list.locator(".au-row", { hasText: "还没运行的流程" }).waitFor();
      if (await list.locator(".au-row").count() !== 1) throw new Error("无项目筛选不应混入项目工作流");
      await filters.getByLabel("工作流项目", { exact: true }).selectOption("");
      await filters.getByLabel("工作流排序", { exact: true }).selectOption("active");
      if (!(await list.locator(".au-row").first().innerText()).includes("运行中的发布流程")) throw new Error("进行中优先排序");
      await shot(page, size, scheme, "workflow-catalog");
      // Export this user-visible change in both languages at 2x, using a
      // separate mock-only context so the suite's navigation stays untouched.
      if (size.width === 1200) {
        for (const language of ["zh", "en"]) {
          const preview = await browser.newContext({ viewport: size, colorScheme: scheme, deviceScaleFactor: 2, storageState: await context.storageState() });
          try {
            await preview.addInitScript(({ language, scheme }) => { localStorage.setItem("myco-lang", language); localStorage.setItem("myco-theme", scheme); }, { language, scheme });
            const capture = await preview.newPage();
            const titles = ["Release pipeline", "Checks to investigate", "New workflow"];
            const translated = catalogRows.map((w, i) => ({ ...w, title: language === "en" ? titles[i] : w.title, description: language === "en" ? "Create, review and deliver with your agents." : "让 agent 协作完成编写、审查和交付。", last_run: w.last_run ? { ...w.last_run, inputs: { task: language === "en" ? "Export conversations from Settings" : "设置页加一个导出对话按钮" } } : undefined }));
            await capture.route(empty, (r) => r.fulfill({ json: { workflows: translated } }));
            await capture.route(/\/app\/hubs\/[^/]+\/workflow-decisions$/, (r) => r.fulfill({ json: { decisions: [] } }));
            const previewURL = new URL(page.url()); previewURL.searchParams.set("lang", language);
            await capture.goto(previewURL.href);
            await capture.locator(".app-shell").waitFor();
            await go(capture, "工作流", false);
            await capture.locator("[data-testid=workflow-counts]").first().waitFor();
            await capture.getByRole("searchbox", { name: language === "en" ? "Search workflows" : "搜索工作流" }).waitFor();
            await shot(capture, size, scheme, `workflow-catalog-${language}`);
          } finally { await preview.close(); }
        }
      }

      await filters.getByRole("searchbox").fill("no-such-workflow");
      await page.locator("[data-testid=workflows-no-results]").waitFor();
      await page.getByRole("button", { name: "清除筛选", exact: true }).click();
      await liveRow.waitFor();
      // Older Hubs have no counters. Never turn missing data into zero runs.
      catalogRows = [{ ...catalogRows[0], run_counts: undefined }];
      await page.getByText("累计统计暂不可用", { exact: true }).waitFor({ timeout: 12_000 });
      const overview = await page.locator("[data-testid=workflow-overview]").innerText();
      if (!overview.includes("升级工作区") || !overview.includes("—")) throw new Error("旧工作区应明确累计统计未知");
      await page.unroute(empty);
      await liveRow.waitFor({ state: "detached", timeout: 12_000 });
      await list.locator(".au-row", { hasText: "写-审-改" }).waitFor();
      // A ready-made one (新建工作流 offers them): pick who does what, see it drawn, save it.
      if (await page.locator(".wf-row-last").count() < 1) throw new Error("每个工作流应写出最近一次运行");
      if (!(await list.locator(".au-row", { hasText: "写-审-改" }).getByRole("button", { name: "运行", exact: true }).isVisible())) throw new Error("每行应有看得见的「运行」");
      if (await page.locator(".pc > [data-testid=workflow-templates]").count()) throw new Error("有工作流时列表页不该再列一遍模板");
      await clickVisible(page, page.locator(".ph").getByRole("button", { name: "新建工作流" }));
      await page.locator("[data-testid=workflow-new]").waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "workflow-new");
      await clickVisible(page, page.locator("[data-testid=workflow-new] .au-template", { hasText: "改完跑测试" }));
      const mk = page.locator("[data-testid=workflow-make]");
      await mk.waitFor({ timeout: 10_000 });
      await mk.locator("[data-testid=workflow-graph] .wf-node").first().waitFor({ timeout: 5_000 });
      if ((await mk.locator(".wf-node").count()) !== 2) throw new Error("「改完跑测试」应画出两步");
      await settle(page, 200);
      await shot(page, size, scheme, "workflow-make");
      const name = `改完跑测试 ${size.width} ${scheme}`;
      await mk.locator("input.app-input").first().fill(name);
      await clickVisible(page, mk.getByRole("button", { name: "保存工作流" }));
      const det = page.locator("[data-testid=workflow-detail]");
      await det.waitFor({ timeout: 10_000 });
      await det.locator("h1", { hasText: name }).waitFor({ timeout: 10_000 });
      // A new one opens on how it goes; its runs are a tab away.
      await det.locator("[data-testid=workflow-graph] .wf-node", { hasText: "跑测试" }).waitFor({ timeout: 10_000 });
      if (!(await det.locator(".wf-fail-note").count())) throw new Error("走向失败的出口应写在那一步旁边");
      await settle(page, 200);
      await shot(page, size, scheme, "workflow-made");
      await clickVisible(page, det.locator("[data-testid=workflow-tabs]").getByRole("tab", { name: /运行记录/ }));
      await det.locator("[data-testid=workflow-no-runs]").waitFor({ timeout: 10_000 });
      // Run it: the form asks for what it takes; the run opens at once.
      await clickVisible(page, det.locator(".pcd-head").getByRole("button", { name: "运行", exact: true }));
      const form = page.locator("[data-testid=workflow-start]");
      await form.waitFor({ timeout: 10_000 });
      if (!(await form.getByRole("button", { name: "开始运行" }).isDisabled())) throw new Error("没填要改什么时不该能开始");
      await form.locator("textarea").fill("修 Safari 上登录闪退");
      if ((await form.locator("input.app-input").inputValue()) !== "make test") throw new Error("测试命令应带默认值");
      await settle(page, 150);
      await shot(page, size, scheme, "workflow-start");
      await clickVisible(page, form.getByRole("button", { name: "开始运行" }));
      const run = page.locator("[data-testid=workflow-run]");
      await run.waitFor({ timeout: 10_000 });
      await run.locator(".wf-node.is-running").waitFor({ timeout: 10_000 });
      // Back, and remove it again (right-click).
      // Back (the ‹ at the top) to the workflow, and again to the list.
      await navBackClick(page);
      await page.locator("[data-testid=workflow-detail]").waitFor({ timeout: 10_000 });
      await navBackClick(page);
      const row = list.locator(".au-row", { hasText: name });
      await row.waitFor({ timeout: 10_000 });
      await row.click({ button: "right", position: { x: 120, y: 12 } });
      await clickVisible(page, page.getByRole("menuitem", { name: "删除" }));
      await clickVisible(page, page.locator(".app-modal.app-confirm").getByRole("button", { name: "删除" }));
      await row.waitFor({ state: "detached", timeout: 10_000 });
      // Have an agent write one: it goes to that agent as a task.
      await clickVisible(page, page.locator(".ph").getByRole("button", { name: "新建工作流" }));
      await clickVisible(page, page.locator("[data-testid=workflow-new] .au-template", { hasText: "让 agent 写一个" }));
      const ask = page.locator("[data-testid=workflow-ask]");
      await ask.waitFor({ timeout: 10_000 });
      await ask.locator("textarea").fill("先让 PM 写方案，再审，审过了问我");
      await settle(page, 150);
      await shot(page, size, scheme, "workflow-ask");
      await clickVisible(page, ask.getByRole("button", { name: "取消" }));
      await ask.waitFor({ state: "detached", timeout: 5_000 });
    });

    // A workflow opened: its steps drawn, then its runs; a run: how each
    // step went, a decision to make, each step's chat.
    await step("工作流 · 运行", async () => {
      await go(page, "工作流", narrow);
      const row = page.locator("[data-testid=workflows] .au-row", { hasText: "写-审-改" });
      await row.waitFor({ timeout: 20_000 });
      await clickVisible(page, row.locator(".au-row-main"));
      const det = page.locator("[data-testid=workflow-detail]");
      await det.waitFor({ timeout: 10_000 });
      const tab = (name) => clickVisible(page, det.locator("[data-testid=workflow-tabs]").getByRole("tab", { name }));
      // One that has run opens on its runs: each says what it was about.
      const runs = det.locator("[data-testid=workflow-runs] .wf-run-row");
      await runs.first().waitFor({ timeout: 10_000 });
      if (!(await runs.first().innerText()).includes("导出对话")) throw new Error("运行记录每条应写这次做的是什么");
      await settle(page, 300);
      await shot(page, size, scheme, "workflow-detail");
      await tab("流程");
      await det.locator("[data-testid=workflow-graph] .wf-node", { hasText: "审" }).waitFor({ timeout: 10_000 });
      if (!(await det.locator(".wf-edge.is-back").count())) throw new Error("审没过回到做的那条边应画成回去的虚线");
      if (await det.locator(".wf-edge text", { hasText: "{" }).count()) throw new Error("流程图的边上不该露出数据怎么传的模板");
      if (!(await det.locator(".wf-node", { hasText: "审" }).innerText()).includes("分工：审查")) throw new Error("按分工挑人的一步应写出分工");
      await settle(page, 200);
      await shot(page, size, scheme, "workflow-flow");
      // Inputs and outputs: what a run is given, what each step gets and hands back.
      await tab("输入和输出");
      const io = det.locator("[data-testid=workflow-contracts]");
      await io.scrollIntoViewIfNeeded();
      const ioText = await io.innerText();
      for (const w of ["运行时要填的", "task", "至少 4 个字", "收到", "交回", "approved", "格式 ^feat/", "选一个：就这样 / 不要了"]) {
        if (!ioText.includes(w)) throw new Error(`「输入和输出」里应有「${w}」`);
      }
      await clickVisible(page, io.locator("[data-step=review] .wfc-json").last());
      await io.locator("[data-step=review] .wfc-raw", { hasText: "\"approved\"" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "workflow-contracts");
      await tab(/运行记录/);
      await clickVisible(page, runs.filter({ hasText: "等你决定" }).first());
      const run = page.locator("[data-testid=workflow-run]");
      await run.waitFor({ timeout: 10_000 });
      const steps = run.locator("[data-testid=workflow-step]");
      await steps.nth(4).waitFor({ timeout: 10_000 });
      if (!(await run.locator("h1").innerText()).includes("导出对话")) throw new Error("一次运行的标题应是这次做的事");
      if ((await run.locator("[data-testid=workflow-step] .wf-mark.is-done").count()) < 4) throw new Error("做完的步骤应打勾");
      if (!(await run.locator(".wf-node.is-waiting").count())) throw new Error("等你决定的那一步应标出来");
      if (!(await steps.nth(2).innerText()).includes("第 2 次")) throw new Error("第二轮的「做」应写第 2 次");
      await clickVisible(page, steps.nth(1).getByRole("button", { name: "详情" }));
      await steps.nth(1).locator(".wf-step-detail pre", { hasText: "comments" }).waitFor({ timeout: 5_000 });
      await steps.nth(1).locator(".wf-step-detail", { hasText: "收到的输入" }).locator("pre", { hasText: "feat/export-chat" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "workflow-run");
      // Only one step's times: click it in the drawing.
      await clickVisible(page, run.locator(".wf-node", { hasText: "审" }));
      if ((await steps.count()) !== 2) throw new Error("点「审」只看它的两次");
      await clickVisible(page, run.getByRole("button", { name: "看全部" }));
      // Decide; the run is done.
      const card = run.locator("[data-testid=workflow-decision]");
      await clickVisible(page, card.getByRole("button", { name: "就这样" }));
      await run.locator("[data-testid=workflow-run-state]", { hasText: "完成了" }).waitFor({ timeout: 10_000 });
      if (await card.count()) throw new Error("决定之后卡片应收起");
      await settle(page, 200);
      await shot(page, size, scheme, "workflow-run-done");
      // A step's chat, at its task.
      await clickVisible(page, steps.first().getByRole("button", { name: "看对话" }));
      await page.locator(".conv-detail", { hasText: "导出对话" }).first().waitFor({ timeout: 15_000 });
      await page.locator("[data-testid=via-workflow]").first().waitFor({ timeout: 10_000 });
      if (narrow) await sidebar(page, false);
    });

    // Watching how a workflow goes: its figures, its runs by state; a run's
    // metrics, timeline, files, scores, a step's log, two runs side by side.
    await step("工作流 · 观测", async () => {
      await go(page, "工作流", narrow);
      const row = page.locator("[data-testid=workflows] .au-row", { hasText: "写-审-改" });
      await row.waitFor({ timeout: 20_000 });
      await clickVisible(page, row.locator(".au-row-main"));
      const det = page.locator("[data-testid=workflow-detail]");
      const tab = (name) => clickVisible(page, det.locator("[data-testid=workflow-tabs]").getByRole("tab", { name }));
      await tab("运行情况");
      const stats = det.locator("[data-testid=workflow-stats]");
      await stats.waitFor({ timeout: 10_000 });
      const statsText = await stats.innerText();
      for (const w of ["跑了", "成功率", "用时", "每次花费", "$"]) if (!statsText.includes(w)) throw new Error(`运行情况里应有「${w}」`);
      if ((await det.locator("[data-testid=workflow-recent] .wfo-bar").count()) < 8) throw new Error("最近几次的用时应画成一排柱子");
      const table = det.locator("[data-testid=workflow-step-stats]");
      if (!(await table.innerText()).includes("最近一次失败")) throw new Error("每一步的表里应写出最近一次失败");
      await stats.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "workflow-stats");
      await table.scrollIntoViewIfNeeded();
      await settle(page, 200);
      await shot(page, size, scheme, "workflow-step-stats");
      // Only the failed runs.
      await tab(/运行记录/);
      await clickVisible(page, det.locator("[data-testid=workflow-runs-filter]").getByRole("tab", { name: "失败的" }));
      await page.waitForFunction(() => {
        const rows = [...document.querySelectorAll("[data-testid=workflow-runs] .wf-run-row")];
        return rows.length > 0 && rows.every((r) => r.textContent.includes("失败了"));
      }, null, { timeout: 10_000 });
      await clickVisible(page, det.locator("[data-testid=workflow-runs-filter]").getByRole("tab", { name: "全部" }));
      // The run with the latest score: the export button.
      await tab("运行情况");
      await clickVisible(page, det.locator("[data-testid=workflow-score-stats]").getByRole("button", { name: /最近/ }).first());
      const run = page.locator("[data-testid=workflow-run]");
      const metrics = run.locator("[data-testid=workflow-run-metrics]");
      await metrics.waitFor({ timeout: 10_000 });
      for (const w of ["总用时", "干活", "发了", "$"]) if (!(await metrics.innerText()).includes(w)) throw new Error(`运行的指标里应有「${w}」`);
      const arts = run.locator("[data-testid=workflow-artifacts]");
      await arts.locator("img[alt='设置页截图.png']").first().waitFor({ timeout: 10_000 });
      if (!(await arts.innerText()).includes("改动说明.md")) throw new Error("产物里应有交回的文件");
      await run.locator("[data-testid=workflow-run-scores]", { hasText: "quality" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "workflow-run-metrics");
      await arts.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "workflow-artifacts");
      // The timeline.
      await clickVisible(page, run.locator("[data-testid=workflow-steps-view]").getByRole("tab", { name: "时间线" }));
      const tl = run.locator("[data-testid=workflow-timeline]");
      await tl.waitFor({ timeout: 5_000 });
      if ((await tl.locator(".wfo-tl-row").count()) < 5) throw new Error("时间线上每一步应有一条");
      await tl.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "workflow-timeline");
      // A step's log, from its bar.
      await clickVisible(page, tl.locator(".wfo-tl-row").first());
      const log = page.locator("[data-testid=workflow-step-log]");
      await log.locator(".wfo-log-line", { hasText: "Read web/src/app/Settings.tsx" }).waitFor({ timeout: 10_000 });
      const logText = await log.innerText();
      for (const w of ["交给它", "进展", "回复", "用工具", "用量", "退回"]) if (!logText.includes(w)) throw new Error(`步骤日志里应有「${w}」`);
      await settle(page, 300);
      await shot(page, size, scheme, "workflow-step-log");
      await page.keyboard.press("Escape");
      await log.waitFor({ state: "detached", timeout: 5_000 });
      // A score.
      await clickVisible(page, run.getByRole("button", { name: "打个分" }));
      const form = page.locator("[data-testid=workflow-score]");
      await form.locator("input[inputmode=decimal]").fill("5");
      await form.getByPlaceholder("分").fill("分");
      await clickVisible(page, form.getByRole("button", { name: "记下" }));
      await form.waitFor({ state: "detached", timeout: 5_000 });
      await run.locator("[data-testid=workflow-run-scores] .wfo-score", { hasText: "5分" }).waitFor({ timeout: 10_000 });
      // Side by side with the run before.
      await clickVisible(page, run.getByRole("button", { name: "对比" }));
      const cmp = page.locator("[data-testid=workflow-compare]");
      await cmp.locator(".wfo-tr", { hasText: "总用时" }).waitFor({ timeout: 10_000 });
      if (!(await cmp.innerText()).includes("quality")) throw new Error("对比里应有两次的分数");
      await settle(page, 300);
      await shot(page, size, scheme, "workflow-compare");
      await page.keyboard.press("Escape");
      await cmp.waitFor({ state: "detached", timeout: 5_000 });
      if (narrow) await sidebar(page, false);
    });

    // Editing a workflow: who does each step, timeouts, retries, rounds;
    // saved as a new version.
    await step("工作流 · 编辑", async () => {
      await go(page, "工作流", narrow);
      const row = page.locator("[data-testid=workflows] .au-row", { hasText: "写-审-改" });
      await row.waitFor({ timeout: 20_000 });
      await clickVisible(page, row.locator(".au-row-main"));
      const det = page.locator("[data-testid=workflow-detail]");
      await det.waitFor({ timeout: 10_000 });
      const before = await det.locator(".pcd-sub").innerText();
      await clickVisible(page, det.locator(".pcd-head").getByRole("button", { name: "编辑" }));
      const ed = page.locator("[data-testid=workflow-edit]");
      await ed.locator("[data-testid=workflow-edit-basic]").waitFor({ timeout: 10_000 });
      if (!(await ed.getByRole("button", { name: "保存新版本" }).isDisabled())) throw new Error("没改什么时不该能保存");
      // What a run is given uses a rule the form cannot show: read as words, JSON only when asked.
      const inEd = ed.locator("[data-testid=workflow-edit-inputs]");
      if (!(await inEd.innerText()).includes("至少 4 个字")) throw new Error("表单写不了的规则应写成看得懂的样子");
      if (await inEd.locator("textarea").count()) throw new Error("不该一打开就是 JSON");
      if (!(await inEd.getByRole("button", { name: "用 JSON 改" }).count())) throw new Error("应能「用 JSON 改」");
      await ed.locator(".wf-edit-loop select").selectOption("5");
      // Step 2 (picked in the drawing) goes by role; make it a named agent, then back to a role.
      await clickVisible(page, ed.locator(".wf-node", { hasText: "审" }));
      const st = ed.locator("[data-testid=workflow-edit-step][data-step=review]");
      await st.waitFor({ timeout: 5_000 });
      if ((await st.getByRole("textbox", { name: "第 2 步的分工" }).inputValue()) !== "审查") throw new Error("按分工的一步应带出分工");
      await st.getByRole("combobox", { name: "第 2 步交给谁" }).selectOption({ index: 0 });
      await st.getByRole("textbox", { name: "第 2 步的分工" }).waitFor({ state: "detached", timeout: 5_000 });
      await st.getByRole("combobox", { name: "第 2 步交给谁" }).selectOption("@role");
      if (!(await ed.getByRole("button", { name: "保存新版本" }).isDisabled())) throw new Error("分工空着不该能保存");
      await st.getByRole("textbox", { name: "第 2 步的分工" }).fill("审查");
      // What it hands back: fields.
      const outEd = st.locator("[data-testid=workflow-edit-output]");
      if ((await outEd.locator(".wfc-row").count()) !== 2) throw new Error("「审」交回的应列出 approved 和 comments 两个字段");
      await clickVisible(page, outEd.getByRole("button", { name: "加一个字段" }));
      await outEd.locator(".wfc-row").nth(2).getByRole("textbox", { name: "字段名" }).fill("risk");
      await outEd.locator(".wfc-row").nth(2).getByRole("combobox", { name: "类型" }).selectOption("number");
      await settle(page, 200);
      await shot(page, size, scheme, "workflow-edit");
      // The chips above the drawing pick too.
      await clickVisible(page, ed.locator(".wf-ed-chip", { hasText: "1. 做" }));
      await ed.locator("[data-testid=workflow-edit-step][data-step=write]").waitFor({ timeout: 5_000 });
      await clickVisible(page, ed.getByRole("button", { name: "保存新版本" }));
      await ed.waitFor({ state: "detached", timeout: 10_000 });
      await clickVisible(page, det.locator("[data-testid=workflow-tabs]").getByRole("tab", { name: "流程" }));
      await det.locator(".wf-edge text", { hasText: "最多 5 次" }).waitFor({ timeout: 10_000 });
      if ((await det.locator(".pcd-sub").innerText()) === before) throw new Error("保存后应是新的一版");
    });

    // ⌃Tab: the agents as large faces, last talked to first; letting go of
    // ⌃ opens the one picked (the one before, after a single press).
    await step("切换 agent · ⌃Tab", async () => {
      await openMember(page, narrow, "聊天", "Myco (Grok)");
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      if (narrow) await sidebar(page, false);
      await page.keyboard.down("Control");
      await page.keyboard.press("Tab");
      const sw = page.locator("[data-testid=agent-switcher]");
      await sw.waitFor({ timeout: 5_000 });
      // The open chat first; one press picks the next (at work first).
      const picked = (await sw.locator(".asw-name").innerText()).trim();
      if (!picked || picked === "Review (Codex 云端)") throw new Error("按一下 ⌃Tab 应选中另一个 agent");
      if (await sw.locator(".asw-item.is-away").count()) throw new Error("不在线的不该出现在切换框里");
      if (!(await sw.locator(".asw-item").first().getAttribute("title") || "").includes("Review")) throw new Error("正开着的对话应排第一");
      // ← → pick too.
      await page.keyboard.press("ArrowRight");
      const next = (await sw.locator(".asw-name").innerText()).trim();
      if (next === picked) throw new Error("→ 应换到下一个");
      await page.keyboard.press("ArrowLeft");
      if ((await sw.locator(".asw-name").innerText()).trim() !== picked) throw new Error("← 应回到上一个");
      await settle(page, 150);
      await shot(page, size, scheme, "agent-switcher");
      await page.keyboard.up("Control");
      await sw.waitFor({ state: "detached", timeout: 5_000 });
      // (its chat: highlighted in the sidebar; a coordinator's is its project's home)
      await sidebar(page, true);
      await page.locator(".app-sidebar .sp-nav-person.active", { hasText: picked }).first().waitFor({ state: "attached", timeout: 10_000 });
      if (narrow) await sidebar(page, false);
      // Esc puts it away, nothing opened.
      await page.keyboard.down("Control");
      await page.keyboard.press("Backquote");
      await sw.waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await page.keyboard.up("Control");
      await sw.waitFor({ state: "detached", timeout: 5_000 });
      await sidebar(page, true);
      if (!(await page.locator(".app-sidebar .sp-nav-person.active", { hasText: picked }).count())) throw new Error("Esc 收起时不该换对话");
      if (narrow) await sidebar(page, false);
    });

    // (待处理, the list of what waits for you, is hidden for now: NOTICES_LIST
    // in AppMode.tsx; its step comes back with it.)

    // A reply with more than words: a diagram (```mermaid) drawn, code in
    // colour, one diagram that does not parse showing why and its source,
    // pictures placed in the text side by side (and not listed again below).
    await step("消息 · 图、代码和插图", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      if (narrow) await sidebar(page, false);
      await noRound(page);
      const conv = page.locator(".conv-detail:visible").last();
      await conv.locator(".conv-compose-input").fill("画一张架构图");
      await conv.locator(".conv-compose-input").press("Enter");
      const msg = conv.locator(".conv-msg", { has: page.locator(".md-mermaid") }).last();
      await msg.locator(".md-mermaid[data-state=drawn] svg").first().waitFor({ timeout: 30_000 });
      // The reply is in: the run before it is folded to its one line, so
      // read it opened.
      for (const b of await conv.locator(".trace-fold").all()) {
        if ((await b.getAttribute("aria-expanded")) === "false") await b.click();
      }
      // The progress update along the way placed its two sketches in its
      // text: there, side by side, not "not found", not listed again.
      const report = conv.locator(".trace-report", { hasText: "先放两张草图" }).last();
      await report.waitFor({ timeout: 10_000 });
      if (await report.locator(".md-img-missing").count()) throw new Error("进展里引用的附件应能找到");
      if ((await report.locator(".md-gallery img").count()) !== 2) throw new Error("进展里的两张草图应并排显示在正文里");
      if (await report.locator(".trace-report-files").count()) throw new Error("进展正文里已经显示的图不该在下面再列一次");
      const bad = msg.locator(".md-mermaid[data-state=error]");
      await bad.waitFor({ timeout: 15_000 });
      if (!(await bad.locator(".md-mermaid-why").innerText()).includes("没画出来")) throw new Error("画不出来的图应说明原因");
      if (!(await bad.locator(".md-code").isVisible())) throw new Error("画不出来的图应直接显示源码");
      await msg.locator("code.hljs .hljs-keyword").first().waitFor({ timeout: 10_000 });
      // (a long reply starts folded: opened to see it all)
      if (await msg.locator(".md-fold-body.is-collapsed").count()) await msg.locator(".md-fold-btn").click();
      const gallery = msg.locator(".md-gallery:not(.is-captioned) img");
      if ((await gallery.count()) !== 2) throw new Error(`正文里应有两张并排的图，实际 ${await gallery.count()}`);
      // A list whose items start with a picture: rows, picture on the left.
      if ((await msg.locator("li.md-media").count()) !== 2) throw new Error("以图片开头的列表项应排成左图右文");
      const row = await msg.locator("li.md-media").first().evaluate((li) => { const i = li.querySelector("img").getBoundingClientRect(); const t = li.querySelector("strong").getBoundingClientRect(); return { img: i.right, text: t.left, top: Math.abs(i.top - t.top) }; });
      if (row.text < row.img || row.top > 12) throw new Error(`左图右文的排版不对：${JSON.stringify(row)}`);
      // Each picture stays inside its row (not over the line below it).
      const spill = await msg.locator("li.md-media").evaluateAll((lis) => lis.map((li) => Math.round(li.querySelector("img").getBoundingClientRect().bottom - li.getBoundingClientRect().bottom)));
      if (spill.some((d) => d > 0)) throw new Error(`左图右文的图超出了这一行：${spill.join(", ")}px`);
      // Titled pictures side by side: each with its name and line under it.
      const figs = msg.locator(".md-gallery.is-captioned figure");
      if ((await figs.count()) !== 3) throw new Error(`带说明的图片组应有三张，实际 ${await figs.count()}`);
      if ((await figs.nth(1).locator("figcaption").innerText()).replace(/\s+/g, " ").trim() !== "棕色香菇 自然、食物") throw new Error("图片下面应显示名字和说明");
      await page.waitForFunction(() => [...document.querySelectorAll(".md-gallery img")].every((i) => i.complete && i.naturalWidth > 0), null, { timeout: 10_000 });
      await msg.locator(".md-gallery.is-captioned").evaluate((el) => el.scrollIntoView({ block: "center" }));
      await settle(page, 300);
      await shot(page, size, scheme, "message-rich-cards");
      // The two placed in the text are not listed again; the log still is.
      if (await msg.locator(".conv-shots").count()) throw new Error("正文里已经显示的图不该在下面再列一次");
      await msg.locator(".conv-attach-list", { hasText: "build.log" }).waitFor({ timeout: 5_000 });
      await msg.locator(".md-mermaid").first().evaluate((el) => el.scrollIntoView({ block: "start" }));
      await settle(page, 300);
      await shot(page, size, scheme, "message-rich");
      // The diagram opens large; so do the pictures.
      await msg.locator(".md-mermaid[data-state=drawn] .md-mermaid-pic").first().click();
      const lb = page.locator(".lightbox");
      await lb.waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "message-rich-diagram");
      await page.keyboard.press("Escape");
      await lb.waitFor({ state: "detached", timeout: 5_000 });
      await gallery.nth(1).click();
      await lb.waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await lb.waitFor({ state: "detached", timeout: 5_000 });
      if (narrow) await sidebar(page, false);
    });

    // A ```myco-ui block (ui/UiBlock.tsx): the catalog's components drawn
    // in place of its JSON; an unknown one is a small note; a reply button
    // puts its words in the box.
    await step("消息 · 界面块", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      if (narrow) await sidebar(page, false);
      await noRound(page);
      const conv = page.locator(".conv-detail:visible").last();
      await conv.locator(".conv-compose-input").fill("做一下发版检查");
      await conv.locator(".conv-compose-input").press("Enter");
      const msg = conv.locator(".conv-msg", { has: page.locator(".ui-root") }).last();
      await msg.locator(".ui-root .ui-card").first().waitFor({ timeout: 30_000 });
      if (await msg.locator(".md-fold-body.is-collapsed").count()) await msg.locator(".md-fold-btn").click();
      if (await msg.getByText('"type": "Card"').count()) throw new Error("界面块不该显示成 JSON 原文");
      for (const [sel, n] of [[".ui-metric", 3], [".ui-steps li", 4], [".ui-gallery figure", 3], [".ui-media li", 1], [".ui-kv dt", 2], [".ui-table tbody tr", 2], [".ui-btn", 3]]) {
        const c = await msg.locator(sel).count();
        if (c !== n) throw new Error(`${sel} 应有 ${n} 个，实际 ${c}`);
      }
      await msg.locator(".ui-unknown", { hasText: "Sparkles" }).waitFor({ timeout: 5_000 });
      await page.waitForFunction(() => [...document.querySelectorAll(".ui-gallery img")].length === 3 && [...document.querySelectorAll(".ui-gallery img")].every((i) => i.complete && i.naturalWidth > 0), null, { timeout: 10_000 });
      // The pictures it shows are not listed again below.
      if (await msg.locator(".conv-shots").count()) throw new Error("界面块里用过的图不该在下面再列一次");
      await msg.locator(".ui-card").first().evaluate((el) => el.scrollIntoView({ block: "start" }));
      await settle(page, 300);
      await shot(page, size, scheme, "message-ui");
      // (and the whole block on its own, top to bottom)
      await page.setViewportSize({ width: size.width, height: 2600 });
      await settle(page, 300);
      await msg.locator(".ui-root").first().evaluate((el) => el.scrollIntoView({ block: "center" }));
      await settle(page, 200);
      await msg.locator(".ui-root").first().screenshot({ path: path.join(OUT, `${size.width}x${size.height}-${scheme}-message-ui-block.png`) });
      await page.setViewportSize(size);
      await settle(page, 300);
      // Tabs: the changes, then the chart.
      await msg.locator(".ui-tabs").getByRole("tab", { name: "改动" }).click();
      await msg.locator(".ui-file").first().waitFor({ timeout: 5_000 });
      await msg.locator(".ui-tabs").getByRole("tab", { name: "体积" }).click();
      if ((await msg.locator(".ui-chart-xy rect").count()) !== 8) throw new Error("柱状图应有 8 根柱子");
      await msg.locator(".ui-gallery").evaluate((el) => el.scrollIntoView({ block: "center" }));
      await settle(page, 200);
      await shot(page, size, scheme, "message-ui-more");
      // 「发吧」: into the box, not sent.
      await msg.locator(".ui-btn", { hasText: "发吧" }).click();
      await page.waitForFunction(() => (document.querySelector(".conv-detail .conv-compose-input")?.value || "") === "发吧", null, { timeout: 5_000 });
      await conv.locator(".conv-compose-input").fill("");
      if (narrow) await sidebar(page, false);
    });

    // 专注模式: ⋯ › 专注模式 (or ⌘⇧F) leaves one chat alone in the middle;
    // ⌃Tab still switches; Esc puts everything back.
    await step("专注模式", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      // (narrow: the sidebar slides away first; a click on ⋯ while it
      // still covers the chat does not reach the button)
      if (narrow) {
        await sidebar(page, false);
        await page.locator(".app-sidebar").waitFor({ state: "hidden", timeout: 5_000 });
      }
      const conv = page.locator(".conv-detail:visible").last();
      const item = page.locator(".conv-menu").getByRole("menuitem", { name: /专注模式/ });
      // The menu is open when its item shows; a click that landed while
      // the chat was still settling opens nothing, so it is tried again.
      for (let i = 0; ; i++) {
        await clickVisible(page, conv.getByRole("button", { name: "更多" }).first());
        if (await item.waitFor({ timeout: 3_000 }).then(() => true, () => false)) break;
        if (i === 2) throw new Error("⋯ 菜单点了三次都没打开");
        await page.keyboard.press("Escape");
      }
      if (!/⌘⇧F|Ctrl\+Shift\+F/.test(await item.innerText())) throw new Error("菜单里应写着专注模式的快捷键");
      await item.click();
      await page.locator(".app-shell.is-focus").waitFor({ timeout: 5_000 });
      for (const sel of [".app-sidebar", ".app-crumbbar", ".sp-side", ".sp-strip", ".sp-pane-zoom"]) {
        if (await page.locator(sel).first().isVisible().catch(() => false)) throw new Error(`专注模式下 ${sel} 应藏起来`);
      }
      if ((await page.locator("[data-pane]:visible").count()) !== 1) throw new Error("专注模式下只留一栏");
      const box = await page.locator(".conv-detail:visible").first().boundingBox();
      const vw = page.viewportSize().width;
      if (!box || Math.abs(box.x + box.width / 2 - vw / 2) > 24) throw new Error("对话应在中间");
      const ex = await page.locator(".focus-exit").boundingBox();
      if (!ex || ex.x < box.x + box.width) throw new Error("退出按钮不该压在对话上");
      await page.locator(".focus-exit").waitFor({ timeout: 5_000 });
      // A pinned tab above the chat: its row lines up too.
      // (as whoever this App is on the hub now: read off its own request,
      // asked for again here; the resource timing buffer is emptied first,
      // as a long run fills it and later requests are no longer recorded)
      await page.evaluate(() => {
        performance.clearResourceTimings();
        performance.setResourceTimingBufferSize(1000);
        window.dispatchEvent(new CustomEvent("myco-pins-changed", { detail: { hubId: "hub_studio" } }));
      });
      const pinAs = await page.waitForFunction(() => {
        const hit = performance.getEntriesByType("resource").map((e) => e.name).filter((n) => n.includes("/pins?with=review-cloud")).pop();
        return hit ? new URL(hit).searchParams.get("a") || "" : null;
      }, null, { timeout: 10_000 }).then((h) => h.jsonValue());
      const asQ = pinAs ? `&a=${encodeURIComponent(pinAs)}` : "";
      const pinApi = (method, path, data) => fetch(`http://${HOST}:${mockPort()}/app/hubs/hub_studio/pins${path}`, { method, headers: { "Content-Type": "application/json" }, body: data ? JSON.stringify(data) : undefined });
      for (const p of (await (await pinApi("GET", `?with=review-cloud${asQ}`)).json()).pins) await pinApi("DELETE", `/${p.id}?with=review-cloud${asQ}`);
      await pinApi("POST", "", { ...(pinAs ? { a: pinAs } : {}), with: "review-cloud", kind: "link", target: "http://192.168.1.20:7780/pages/pg_release", title: "审查清单", by: "review-cloud" });
      await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-pins-changed", { detail: { hubId: "hub_studio" } })));
      await page.locator(".conv-detail:visible .pin-bar").first().waitFor({ timeout: 10_000 });
      // The header's line, the pinned row's and the box you write in end at
      // the same place.
      const cbox = await page.locator(".conv-detail:visible .conv-compose-box").first().boundingBox();
      for (const sel of [".conv-head", ".pin-bar"]) {
        const b = await page.locator(`.conv-detail:visible > ${sel}`).first().boundingBox();
        if (!b || !cbox || Math.abs(b.x - cbox.x) > 2 || Math.abs(b.x + b.width - cbox.x - cbox.width) > 2) {
          throw new Error(`专注模式的 ${sel} 应和输入框对齐（${b?.x}–${b && b.x + b.width}，输入框 ${cbox?.x}–${cbox && cbox.x + cbox.width}）`);
        }
      }
      await settle(page, 250);
      await shot(page, size, scheme, "focus-mode");
      // The background: the quiet button beside the way out opens its panel.
      const colourOf = (sel) => page.locator(sel).first().evaluate((el) => getComputedStyle(el).backgroundColor);
      const themeBg = await colourOf(".app-shell");
      await clickVisible(page, page.locator(".focus-tone"));
      const pop = page.locator(".focus-bg-pop");
      await pop.waitFor({ timeout: 5_000 });
      if ((await pop.locator(".focus-bg-tile").count()) < 12) throw new Error("专注背景应有十几种可选");
      await pop.getByRole("radio", { name: "纸张" }).click();
      await page.locator(".app-shell[data-focus-bg=paper]").waitFor({ timeout: 5_000 });
      if ((await colourOf(".app-main")) === themeBg) throw new Error("整个页面换成纸张，对话也应换色");
      await settle(page, 200);
      await shot(page, size, scheme, "focus-bg-panel");
      // Outside only: the chat keeps the theme's colour, around it changes.
      await pop.getByRole("radio", { name: "只改外面" }).click();
      await page.locator(".app-shell[data-focus-scope=outside]").waitFor({ timeout: 5_000 });
      if ((await colourOf(".sp-main")) !== themeBg) throw new Error("只改外面时，对话应保持主题的底色");
      if ((await colourOf(".app-shell")) === themeBg) throw new Error("只改外面时，对话外面应换色");
      // Esc closes the panel, not 专注模式.
      await page.keyboard.press("Escape");
      await pop.waitFor({ state: "detached", timeout: 5_000 });
      if (!(await page.locator(".app-shell.is-focus").count())) throw new Error("Esc 应只关掉背景面板");
      await settle(page, 200);
      await shot(page, size, scheme, "focus-bg-outside");
      // A pattern, outside only.
      await clickVisible(page, page.locator(".focus-tone"));
      await pop.getByRole("radio", { name: "点阵" }).click();
      await page.locator(".app-shell[data-focus-kind=pattern]").waitFor({ timeout: 5_000 });
      await page.mouse.click(5, 400);
      await pop.waitFor({ state: "detached", timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "focus-bg-dots");
      // A colour of your own (the system's picker): it applies to the theme
      // you are in; one the words cannot be read on says so, with a way out.
      await clickVisible(page, page.locator(".focus-tone"));
      await pop.getByRole("radio", { name: "整个页面" }).click();
      const picker = pop.locator("input[type=color]");
      const good = scheme === "dark" ? "#24323a" : "#fbe9d0";
      await picker.fill(good);
      await page.locator(".app-shell[data-focus-bg=custom]").waitFor({ timeout: 5_000 });
      const want = `rgb(${[1, 3, 5].map((i) => parseInt(good.slice(i, i + 2), 16)).join(", ")})`;
      if ((await colourOf(".app-main")) !== want) throw new Error(`自定义颜色应成为背景：${await colourOf(".app-main")}，应为 ${want}`);
      if (await pop.locator(".focus-bg-hint").count()) throw new Error("能看清的颜色不该提示");
      await picker.fill(scheme === "dark" ? "#f0f0f0" : "#202020");
      await pop.locator(".focus-bg-hint").waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "focus-bg-custom-hint");
      await pop.locator(".focus-bg-hint").getByRole("button", { name: "改成只改外面" }).click();
      await page.locator(".app-shell[data-focus-scope=outside]").waitFor({ timeout: 5_000 });
      await picker.fill(good);
      await page.keyboard.press("Escape");
      await pop.waitFor({ state: "detached", timeout: 5_000 });
      // Back to how it was, for the steps after.
      await clickVisible(page, page.locator(".focus-tone"));
      await pop.getByRole("radio", { name: "整个页面" }).click();
      await pop.getByRole("radio", { name: "跟随主题" }).click();
      await page.locator(".app-shell[data-focus-bg]").waitFor({ state: "detached", timeout: 5_000 });
      await page.keyboard.press("Escape");
      await pop.waitFor({ state: "detached", timeout: 5_000 });
      for (const p of (await (await pinApi("GET", `?with=review-cloud${asQ}`)).json()).pins) await pinApi("DELETE", `/${p.id}?with=review-cloud${asQ}`);
      await page.evaluate(() => window.dispatchEvent(new CustomEvent("myco-pins-changed", { detail: { hubId: "hub_studio" } })));
      // ⌃Tab still switches, and the one opened stays alone.
      // (the chat shown: the start of its pane; the sidebar may be closed)
      const activeChat = async () => ((await page.locator("[data-pane]:visible").first().innerText().catch(() => "")) || "").slice(0, 80);
      const before = await activeChat();
      await page.keyboard.down("Control");
      await page.keyboard.press("Tab");
      await page.locator("[data-testid=agent-switcher]").waitFor({ timeout: 5_000 });
      await page.keyboard.up("Control");
      await page.locator("[data-testid=agent-switcher]").waitFor({ state: "detached", timeout: 5_000 });
      await settle(page, 400);
      if (!(await page.locator(".app-shell.is-focus").count())) throw new Error("⌃Tab 换对话后应还在专注模式");
      if ((await activeChat()) === before) throw new Error("⌃Tab 应换到别的对话");
      if ((await page.locator("[data-pane]:visible").count()) !== 1) throw new Error("换过去的对话也只留一栏");
      // Esc leaves; the key comes back in.
      await page.keyboard.press("Escape");
      await page.locator(".app-shell.is-focus").waitFor({ state: "detached", timeout: 5_000 });
      if (!(await page.locator(".app-crumbbar").isVisible())) throw new Error("退出专注后顶栏应回来");
      await page.keyboard.press("ControlOrMeta+Shift+KeyF");
      await page.locator(".app-shell.is-focus").waitFor({ timeout: 5_000 });
      // ⌘⇧F is focus mode only, not the search in the chat (⌘F) as well.
      await settle(page, 200);
      if (await page.locator(".conv-search").first().isVisible().catch(() => false)) throw new Error("⌘⇧F 不该同时打开对话里的搜索");
      await clickVisible(page, page.locator(".focus-exit"));
      await page.locator(".app-shell.is-focus").waitFor({ state: "detached", timeout: 5_000 });
      if (narrow) await sidebar(page, false);
    });

    // ‹ › at the top: back to where you were and forward again, pages and
    // what they had open; no back buttons on the pages themselves.
    await step("导航 · 后退前进", async () => {
      await go(page, "工作流", narrow);
      const row = page.locator("[data-testid=workflows] .au-row", { hasText: "写-审-改" });
      await clickVisible(page, row.locator(".au-row-main"));
      const det = page.locator("[data-testid=workflow-detail]");
      await det.waitFor({ timeout: 10_000 });
      if (await page.locator(".pcd-back").count()) throw new Error("页面里不该再有返回按钮");
      await clickVisible(page, det.locator("[data-testid=workflow-runs] .wf-run-row").first());
      await page.locator("[data-testid=workflow-run]").waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "nav-arrows");
      await navBackClick(page);
      await det.waitFor({ timeout: 10_000 });
      await navBackClick(page);
      await page.locator("[data-testid=workflows]").waitFor({ timeout: 10_000 });
      await clickVisible(page, page.locator("[data-testid=nav-arrows]").getByRole("button", { name: "前进" }));
      await det.waitFor({ timeout: 10_000 });
      await page.keyboard.press("Control+BracketRight");
      await page.locator("[data-testid=workflow-run]").waitFor({ timeout: 10_000 });
      await page.keyboard.press("Control+BracketLeft");
      await det.waitFor({ timeout: 10_000 });
      if (narrow) await sidebar(page, false);
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
      const menu = page.getByRole("menu");
      await menu.waitFor();
      // A background conversation's scroll notification must not dismiss
      // a menu belonging to the sidebar (streaming can emit it mid-click).
      await page.locator(".conv-scroll:visible").first().dispatchEvent("scroll");
      await menu.dispatchEvent("scroll");
      await clickVisible(page, page.getByRole("menuitem", { name: "谁能用、项目" }));
      const set = page.locator("[data-testid=computer-settings]");
      await set.waitFor({ timeout: 10_000 });
      await clickVisible(page, set.getByRole("button", { name: "好" }));
      await set.waitFor({ state: "hidden", timeout: 5_000 });
      // Scrolling the source still dismisses its menu: its anchor moved.
      await clickVisible(page, wrap.locator(".sp-nav-more"));
      await menu.waitFor();
      await page.locator(".app-nav").dispatchEvent("scroll");
      await menu.waitFor({ state: "detached", timeout: 5_000 });
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
      await menuPick(page, page.locator(".sp-nav-pc", { hasText: newName }), "移除这个终端");
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
    // Computers behind the latest myco: upgraded from the App, all at once
    // or one by one; one that cannot start sessions gets the command.
    await step("电脑 · 一键升级 myco", async () => {
      await go(page, "电脑", narrow);
      const all = page.locator("[data-testid=upgrade-all]");
      await all.waitFor({ timeout: 15_000 });
      const text = compactText(await all.innerText());
      for (const want of ["Office PC", "Bench Linux", "全部升级（1 台）"]) {
        if (!text.includes(want)) throw new Error(`升级提示应写到「${want}」，实际「${text}」`);
      }
      await settle(page, 300);
      await shot(page, size, scheme, "computers-upgrade");
      await clickVisible(page, all.getByRole("button", { name: /全部升级/ }));
      await all.locator(".mu-state", { hasText: "已升级到" }).waitFor({ timeout: 15_000 });
      // One that cannot be reached is told the command, on its own page.
      await clickVisible(page, page.locator(".pc-card", { hasText: "Bench Linux" }));
      const one = page.locator("[data-testid=machine-upgrade]");
      await one.locator("code", { hasText: "myco upgrade" }).waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "computer-upgrade-command");
      await page.evaluate(async () => { await fetch("/app/mock/machine", { method: "POST" }); });
    });

    // A workspace behind the latest myco: upgraded from Settings, which
    // waits for it to come back on the new version.
    // A workspace's backups, for its owner: made daily on the server; a
    // copy kept on this computer; backed up now and saved here.
    await step("设置 · 工作区备份", async () => {
      await go(page, "设置", narrow);
      const dialog = page.locator("[data-testid=settings-dialog]");
      await clickVisible(page, page.locator(".app-settings-tab", { hasText: "工作区" }).first());
      const row = dialog.locator("[data-testid=backup-row]").first();
      await row.waitFor({ timeout: 15_000 });
      if (!(await row.innerText()).includes("最近一次")) throw new Error("备份一行应写最近一次是什么时候");
      await row.scrollIntoViewIfNeeded();
      await clickVisible(page, row.getByRole("button", { name: "管理" }));
      const bd = page.locator("[data-testid=backup-dialog]");
      await bd.waitFor();
      const modal = page.getByRole("dialog", { name: "工作区备份" });
      await clickVisible(page, modal.getByRole("switch"));
      await modal.locator("[role=switch][aria-checked=true]").waitFor({ timeout: 10_000 });
      await clickVisible(page, modal.getByRole("button", { name: "立即备份并存到这台电脑" }));
      await modal.locator("[data-testid=backup-saved]", { hasText: "myco-backups" }).waitFor({ timeout: 15_000 });
      await modal.getByRole("button", { name: "在文件夹中显示" }).waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "settings-backups");
      await clickVisible(page, modal.getByRole("button", { name: "关闭" }));
      await closeSettings(page);
    });

    await step("设置 · 升级工作区", async () => {
      const setVersion = (v) => page.evaluate(async (x) => { await fetch("/app/mock/hub-version", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ version: x }) }); }, v);
      await setVersion("0.16.9");
      try {
        await go(page, "设置", narrow);
        const dialog = page.locator("[data-testid=settings-dialog]");
        await clickVisible(page, page.locator(".app-settings-tab", { hasText: "工作区" }).first());
        // (the workspace on this computer, older too, has its own)
        const up = dialog.locator("[data-testid=hub-upgrade]", { hasText: "v0.16.9" });
        await up.waitFor({ timeout: 15_000 });
        await up.scrollIntoViewIfNeeded();
        await settle(page, 300);
        await shot(page, size, scheme, "settings-hub-upgrade");
        await clickVisible(page, up.getByRole("button", { name: "升级工作区" }));
        await dialog.locator("[data-testid=hub-upgrade] .mu-text", { hasText: "已升级到 v0.17.2" }).waitFor({ timeout: 30_000 });
      } finally {
        await setVersion("0.19.23");
        await closeSettings(page);
      }
    });

    await step("电脑 · 可能卡住的命令", async () => {
      await sidebar(page, true);
      const pc = page.locator(".sp-nav-pc", { hasText: "demo-shell" });
      await pc.waitFor({ timeout: 10_000 });
      // The hub says the command shows no life: not "working", only a quiet
      // ring (a command left open).
      if (!(await pc.locator(".sp-nav-busy.is-open").count())) {
        throw new Error("demo-shell 只剩没收尾的命令：侧栏应是一个空心圈");
      }
      if (await pc.locator(".sp-nav-busy:not(.is-open)").count()) throw new Error("demo-shell 不该亮绿点或琥珀点");
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
        const r = [...document.querySelectorAll(".sp-nav-pc")].find((e) => (e.textContent || "").includes("demo-shell"));
        return !!r && !r.querySelector(".sp-nav-busy");
      }, null, { timeout: 15_000 });
      await shot(page, size, scheme, "computer-stuck-stopped");
      if (narrow) await sidebar(page, false);
    });

    // Removing a whole computer: a dry run lists the agent and folder on it,
    // the online one shows the uninstall command, and both are gone after.
    // A computer's check-up over SSH: VPS 东京's runner goes through a proxy
    // that no longer gets through; restarting says so; a computer the App
    // did not connect asks which SSH host it is.
    await step("电脑 · 检查", async () => {
      await go(page, "电脑", narrow);
      await clickVisible(page, page.locator(".pc-card", { hasText: "VPS 东京" }));
      await pcTab(page, "检查和修复");
      const check = page.locator("[data-testid=computer-check]");
      await check.locator(".cc-route", { hasText: "tokyo-vps" }).waitFor({ timeout: 10_000 });
      await clickVisible(page, check.getByRole("button", { name: "检查" }));
      const bad = check.locator(".cc-row.is-bad").first();
      await bad.waitFor({ timeout: 10_000 });
      const badText = compactText(await bad.innerText());
      if (!badText.includes("经代理连不上工作区，直连是通的") || !badText.includes("127.0.0.1:7890")) throw new Error(`检查应先说代理连不上、直连通，实际是「${badText}」`);
      await check.locator(".cc-row.is-ok", { hasText: "myco 后台在运行" }).waitFor({ timeout: 5_000 });
      await check.locator(".cc-row").last().scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "computer-check");
      await clickVisible(page, check.getByRole("button", { name: "重启后台" }));
      await check.locator(".cc-fixed", { hasText: "后台重启好了" }).waitFor({ timeout: 10_000 });
      // One the App did not connect: which SSH host is it?
      await navBackClick(page);
      await clickVisible(page, page.locator(".pc-card", { hasText: "临时 Mac" }));
      await pcTab(page, "检查和修复");
      await page.locator("[data-testid=computer-check] select.app-select-field").waitFor({ timeout: 10_000 });
      await page.locator("[data-testid=computer-check]").scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "computer-check-pick");
      await navBackClick(page);
      await page.locator(".pc-card", { hasText: "临时 Mac" }).waitFor({ timeout: 10_000 });
    });

    await step("电脑 · 移除", async () => {
      await go(page, "电脑", narrow);
      await clickVisible(page, page.locator(".pc-card", { hasText: "临时 Mac" }));
      const view = page.locator("[data-testid=pc-detail]");
      await view.waitFor({ timeout: 10_000 });
      // Removing it is under ⋯, as a workflow's 删除 is.
      if (await view.locator(".pcd-danger, .pcd-icon").count()) throw new Error("电脑页不该再有红色移除块或图标块");
      await pcMore(page, "移除");
      const dialog = page.locator(".app-modal");
      await dialog.locator(".pc-gone-row").nth(1).waitFor({ timeout: 10_000 });
      // The computer's own node goes too, but is not listed as one of them.
      const gone = await dialog.locator(".pc-gone-row").count();
      if (gone !== 2) throw new Error(`移除电脑弹窗应列出它上面的 2 个，实际 ${gone} 个`);
      const dialogText = compactText(await dialog.innerText());
      for (const want of ["2 个 agent 和终端会一起移除", "聊过的内容都保留", "myco runner service uninstall"]) {
        if (!dialogText.includes(want)) throw new Error(`移除电脑弹窗里应提到「${want}」，实际是「${dialogText}」`);
      }
      await settle(page, 300);
      await shot(page, size, scheme, "dialog-remove-computer");
      await clickVisible(page, dialog.getByRole("button", { name: "移除这台电脑和上面的 2 个" }));
      await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 10_000 });
      await page.waitForFunction(
        () => ![...document.querySelectorAll(".pc-card, .pc-row")].some((r) => /临时/.test(r.textContent || "")),
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

    // 成员分组: a project's members in groups (the members column's ⋯):
    // a new group, a member moved in from its menu and one dragged in;
    // folded, its faces and who is at work stay on its heading; the other
    // two ways to show them; renamed, then deleted (all back in one list).
    if (!narrow) await step("项目 · 成员分组", async () => {
      await go(page, "官网改版", narrow);
      // (from wherever the steps before left it: one pane, the members column open)
      for (let i = 0; i < 4; i++) {
        const close = page.locator(".pg-pane.sp-pane-right:not(.is-hidden)").getByRole("button", { name: "关掉这一栏" }).first();
        if (!(await close.count())) break;
        await close.click();
        await page.waitForTimeout(200);
      }
      if (await page.locator(".sp-side.is-folded").count()) await page.locator(".sp-side.is-folded .sp-side-fold").click();
      const side = page.locator(".sp-side:not(.is-folded)");
      // A hub too old for groups (its projects carry no `groups`): the ⋯
      // says the workspace needs upgrading, instead of nothing at all.
      {
        const strip = async (route) => {
          const r = await route.fetch();
          const j = await r.json();
          const drop = (p) => { if (p && typeof p === "object") delete p.groups; };
          if (Array.isArray(j.projects)) j.projects.forEach(drop); else drop(j);
          await route.fulfill({ response: r, json: j });
        };
        await page.route("**/app/hubs/hub_studio/projects**", strip);
        await page.evaluate(() => window.dispatchEvent(new Event("focus")));
        await page.waitForTimeout(6_000);
        await clickVisible(page, side.locator("[data-testid=member-groups-menu]"));
        await page.locator(".conv-menu").getByText("要先把工作区升级到 v0.21.34 或更新").waitFor({ timeout: 5_000 });
        await page.keyboard.press("Escape");
        await page.unroute("**/app/hubs/hub_studio/projects**", strip);
        await page.waitForTimeout(6_000);
      }
      await side.waitFor({ timeout: 10_000 });
      const groupsMenu = async (item) => {
        await clickVisible(page, side.locator("[data-testid=member-groups-menu]"));
        await page.locator(".conv-menu").getByRole("menuitem", { name: item }).click();
      };
      const named = async (name) => {
        const dlg = page.locator(".app-modal");
        await dlg.locator("input").waitFor({ timeout: 5_000 });
        await dlg.locator("input").fill(name);
        await dlg.getByRole("button", { name: "好" }).click();
        await dlg.waitFor({ state: "detached", timeout: 5_000 });
      };
      await groupsMenu("新建分组");
      await page.locator(".app-modal input").waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "member-groups-new");
      await named("前端");
      const group = (name) => side.locator(`.mg-group[data-group="${name}"]`);
      await group("前端").locator(".mg-empty").waitFor({ timeout: 10_000 });
      await group("其他").waitFor({ timeout: 5_000 });
      // From its menu: docs-claude into 前端 (移到分组 opens the short list of groups).
      await menuPick(page, side.locator(".sp-member", { hasText: "docs-claude" }), /^移到分组/);
      await page.locator(".conv-menu").getByRole("menuitem", { name: "前端", exact: true }).waitFor({ timeout: 5_000 });
      await page.waitForTimeout(400);
      await shot(page, size, scheme, "member-groups-move");
      await page.locator(".conv-menu").getByRole("menuitem", { name: "前端", exact: true }).click();
      await group("前端").locator(".sp-member", { hasText: "docs-claude" }).waitFor({ timeout: 10_000 });
      // Dragged onto the group: Release too.
      {
        const dt = await page.evaluateHandle(() => new DataTransfer());
        await side.locator(".sp-member", { hasText: "Release (Claude 云端)" }).dispatchEvent("dragstart", { dataTransfer: dt });
        await group("前端").dispatchEvent("dragover", { dataTransfer: dt });
        await group("前端").dispatchEvent("drop", { dataTransfer: dt });
      }
      await group("前端").locator(".sp-member", { hasText: "Release (Claude 云端)" }).waitFor({ timeout: 10_000 });
      // Groups in another order: one dragged by its heading above another
      // (a line shows where it would go); in the chips too.
      {
        await groupsMenu("新建分组");
        await named("后端");
        await group("后端").waitFor({ timeout: 10_000 });
        const order = () => side.locator(".mg-group[data-group]").evaluateAll((els) => els.map((e) => e.getAttribute("data-group")).filter((g) => g !== "其他"));
        if ((await order()).join(",") !== "前端,后端") throw new Error(`新建的分组应排在最后：${(await order()).join(",")}`);
        const dt = await page.evaluateHandle(() => new DataTransfer());
        await group("后端").locator(".mg-head").dispatchEvent("dragstart", { dataTransfer: dt });
        const box = await group("前端").boundingBox();
        const at = { dataTransfer: dt, clientX: box.x + box.width / 2, clientY: box.y + 4 };
        await group("前端").dispatchEvent("dragover", at);
        if (!(await group("前端").evaluate((el) => el.classList.contains("is-slot-before")))) throw new Error("拖分组时应显示放下的位置");
        await group("前端").dispatchEvent("drop", at);
        await page.waitForFunction(() => [...document.querySelectorAll(".sp-side .mg-group[data-group]")].map((e) => e.getAttribute("data-group")).join(",").startsWith("后端,前端"), null, { timeout: 5_000 });
        await page.waitForTimeout(6_000);
        if ((await order()).join(",") !== "后端,前端") throw new Error(`拖动后的分组顺序应保存下来：${(await order()).join(",")}`);
        // (the chips: 前端 dragged back before 后端)
        await groupsMenu("顶部筛选");
        const chip = (g) => side.locator(`.mg-chip[data-group="${g}"]`);
        const dt2 = await page.evaluateHandle(() => new DataTransfer());
        await chip("前端").dispatchEvent("dragstart", { dataTransfer: dt2 });
        const cb = await chip("后端").boundingBox();
        await chip("后端").dispatchEvent("dragover", { dataTransfer: dt2, clientX: cb.x + 2, clientY: cb.y + cb.height / 2 });
        await chip("后端").dispatchEvent("drop", { dataTransfer: dt2, clientX: cb.x + 2, clientY: cb.y + cb.height / 2 });
        await page.waitForFunction(() => [...document.querySelectorAll(".sp-side .mg-chip[data-group]")].map((e) => e.getAttribute("data-group")).filter((g) => g !== "其他").join(",") === "前端,后端", null, { timeout: 5_000 }).catch(async (e) => {
          const got = await page.evaluate(() => [...document.querySelectorAll(".sp-side .mg-chip")].map((e) => `${e.getAttribute("data-group")}:${e.getAttribute("draggable")}:${e.className}`).join(" | "));
          throw new Error(`拖动筛选标签后的顺序不对：${got}；${e.message}`);
        });
        await groupsMenu("可折叠的分组");
        await group("后端").locator(".mg-head").hover();
        await group("后端").locator(".mg-more").click();
        await page.locator(".conv-menu").getByRole("menuitem", { name: /删除分组/ }).click();
        await group("后端").waitFor({ state: "detached", timeout: 10_000 });
      }
      // A slow hub (each save takes a few seconds) and two moves in a row:
      // what shows never falls back while the saves and the 5-second
      // refreshes cross (it went back to 0 on a real hub).
      {
        const slow = async (route) => { if (route.request().method() === "PATCH") await new Promise((ok) => setTimeout(ok, 3_000)); await route.continue(); };
        await page.route("**/app/hubs/hub_studio/projects/website", slow);
        await moveTo(page, group("前端").locator(".sp-member", { hasText: "docs-claude" }), "其他");
        await moveTo(page, side.locator(".sp-member", { hasText: "docs-claude" }), "前端");
        await moveTo(page, group("前端").locator(".sp-member", { hasText: "Release (Claude 云端)" }), "其他");
        await moveTo(page, side.locator(".sp-member", { hasText: "Release (Claude 云端)" }), "前端");
        const counts = [];
        for (let i = 0; i < 56; i++) { counts.push(await group("前端").locator(".sp-member").count()); await page.waitForTimeout(250); }
        await page.unroute("**/app/hubs/hub_studio/projects/website", slow);
        if (counts.some((n) => n !== 2)) throw new Error(`保存慢的时候分组里的人数不该跳回去：${[...new Set(counts)].join(" → ")}`);
      }
      // The coordinator stays on top, in no group.
      if (await side.locator(".mg-group .sp-member", { hasText: "PM (Codex)" }).count()) throw new Error("协调者不该进分组");
      await settle(page, 200);
      await shot(page, size, scheme, "member-groups");
      // Folded: the rows go, its faces stay on the heading.
      await group("前端").locator(".mg-head").click();
      await group("前端").locator(".mg-faces").waitFor({ timeout: 5_000 });
      if (await group("前端").locator(".sp-member").count()) throw new Error("折叠后不该显示组里的成员");
      await settle(page, 200);
      await shot(page, size, scheme, "member-groups-folded");
      await group("前端").locator(".mg-head").click();
      // A row of faces each.
      await groupsMenu("每组一行头像");
      await group("前端").locator(".mg-tile-row").click();
      await group("前端").locator(".mg-inside .sp-member", { hasText: "docs-claude" }).waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "member-groups-faces");
      // Filters on top: 前端 shows its two (and the coordinator).
      await groupsMenu("顶部筛选");
      await side.locator(".mg-chip", { hasText: "前端" }).click();
      // (the chips first, under the column's title, then the coordinator)
      await page.waitForFunction(() => document.querySelectorAll(".sp-side .mg .sp-member").length === 3, null, { timeout: 5_000 });
      if (!(await side.locator(".mg > .mg-chips + .sp-member", { hasText: "PM (Codex)" }).count())) throw new Error("筛选标签应在最上面，协调者在它下面");
      await settle(page, 200);
      await shot(page, size, scheme, "member-groups-chips");
      // Back to folding groups: renamed (its members with it), then deleted.
      await groupsMenu("可折叠的分组");
      await group("前端").locator(".mg-head").hover();
      await group("前端").locator(".mg-more").click();
      await page.locator(".conv-menu").getByRole("menuitem", { name: "改名" }).click();
      await named("Web 前端");
      await group("Web 前端").locator(".sp-member", { hasText: "docs-claude" }).waitFor({ timeout: 10_000 });
      await group("Web 前端").locator(".mg-head").hover();
      await group("Web 前端").locator(".mg-more").click();
      await page.locator(".conv-menu").getByRole("menuitem", { name: /删除分组/ }).click();
      await side.locator(".mg").waitFor({ state: "detached", timeout: 10_000 });
      if ((await side.locator(".sp-member", { hasText: "docs-claude" }).count()) !== 1) throw new Error("删掉分组后成员应回到一列里");
      // Folded to faces, the column still offers groups (its own ⋯).
      await side.locator(".sp-side-tools button[aria-label='收起成员']").click();
      const foldedSide = page.locator(".sp-side.is-folded");
      await clickVisible(page, foldedSide.locator("[data-testid=member-groups-menu-folded]"));
      await page.locator(".conv-menu").getByRole("menuitem", { name: "新建分组" }).waitFor({ timeout: 5_000 });
      await page.keyboard.press("Escape");
      await foldedSide.locator(".sp-side-fold").first().click();
      await side.waitFor({ timeout: 5_000 });
    });

    // A project with two members open beside the coordinator: switching to
    // another workspace and back puts the panes back as they were, at once
    // (the first one beside did widen and shrink back).
    if (!narrow) await step("项目 · 多栏 · 切换工作区不抖", async () => {
      // (a wide window, where the members column has room beside two panes)
      const vp = page.viewportSize();
      await page.setViewportSize({ width: 1800, height: 900 });
      await go(page, "官网改版", narrow);
      await page.locator(".pg-pane[data-pane=main]").waitFor({ timeout: 10_000 });
      const members = page.locator(".sp-side .sp-member:not(.is-add)");
      await clickVisible(page, members.nth(1));
      await page.locator(".pg-pane.sp-pane-right").first().waitFor({ timeout: 10_000 });
      await menuPick(page, page.locator(".sp-nav-person", { hasText: "docs-claude" }), "在新的一栏打开");
      await page.locator(".pg-pane.sp-pane-right").nth(1).waitFor({ timeout: 10_000 });
      await settle(page, 400);
      const steady = await page.locator(".pg-pane.sp-pane-right").evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().width)));
      // (every frame: the panes beside, their widths)
      await page.evaluate(() => {
        window.__paneWidths = [];
        const tick = () => {
          window.__paneWidths.push([...document.querySelectorAll(".pg-pane.sp-pane-right:not(.is-hidden)")].map((e) => Math.round(e.getBoundingClientRect().width)).join(","));
          if (window.__paneWidths.length < 600) requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      });
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await page.locator(".app-hub-menu").waitFor();
      const hubs = await page.locator(".app-hub-menu-item[role=menuitemradio]").evaluateAll((els) => els.map((e) => ({ on: e.getAttribute("aria-checked") === "true", ok: !!e.querySelector(".dot.working"), name: e.querySelector(".app-hub-menu-name")?.textContent || "" })));
      await page.keyboard.press("Escape");
      const here = hubs.findIndex((h) => h.on);
      const other = hubs.findIndex((h, i) => i !== here && h.ok);
      await page.keyboard.press(`Control+${other + 1}`);
      await page.locator(".sp-switch .app-hub-name", { hasText: hubs[other].name }).waitFor({ timeout: 10_000 });
      await settle(page, 300);
      await page.evaluate(() => { window.__paneWidths.push("|back|"); });
      await page.keyboard.press(`Control+${here + 1}`);
      await page.locator(".sp-switch .app-hub-name", { hasText: hubs[here].name }).waitFor({ timeout: 10_000 });
      await page.locator(".pg-pane.sp-pane-right").nth(1).waitFor({ timeout: 10_000 });
      await settle(page, 600);
      const seen = await page.evaluate(() => window.__paneWidths);
      const after = seen.slice(seen.indexOf("|back|") + 1).filter((x) => x !== "");
      const want = steady.join(",");
      const odd = [...new Set(after)].filter((x) => x !== want);
      if (odd.length) throw new Error(`切回工作区时旁边的栏宽度不该变来变去：应一直是 ${want}，实际出现过 ${odd.join(" / ")}`);
      // Close them: the coordinator's pane alone again.
      for (let i = 0; i < 2; i++) {
        await page.locator(".pg-pane.sp-pane-right:not(.is-hidden)").getByRole("button", { name: "关掉这一栏" }).first().click();
        await page.waitForTimeout(200);
      }
      await page.locator(".pg-pane.sp-pane-right").first().waitFor({ state: "detached", timeout: 5_000 });
      await page.setViewportSize(vp);
    });

    // Superagent: set up from the bottom-left menu (a new Shroom on a
    // computer of yours), its chat opens; what it asks you shows above the
    // composer, with 同意 / 不同意; the menu counts what waits.
    await step("superagent · 配一个、确认卡片", async () => {
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await page.locator(".app-hub-menu").waitFor();
      const entry = page.locator("[data-testid=menu-superagent]");
      await entry.waitFor({ timeout: 10_000 });
      const label = compactText(await entry.locator(".app-hub-menu-name").innerText());
      if (label.trim() !== "superagent") throw new Error(`菜单里这一项应就叫「superagent」，实际「${label}」`);
      await clickVisible(page, entry);
      const dialog = page.locator("[data-testid=superagent-setup]");
      await dialog.waitFor({ timeout: 10_000 });
      await clickVisible(page, dialog.locator(".ws-computer", { hasText: "Office PC" }));
      await clickVisible(page, dialog.getByRole("tab", { name: "Codex" }));
      await settle(page, 200);
      await shot(page, size, scheme, "superagent-setup");
      await clickVisible(page, dialog.getByRole("button", { name: "开一个 Shroom" }));
      await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 30_000 });
      await page.locator(".conv-detail .conv-head h2", { hasText: "Shroom" }).waitFor({ timeout: 20_000 });
      // It wears a crown, and is not one of the sidebar's chats. Its empty
      // chat says what it is (no things to ask), and there is no way back.
      await page.locator(".conv-detail .pa-crown").first().waitFor({ timeout: 10_000 });
      await page.locator(".conv-detail [data-testid=superagent-intro]").waitFor({ timeout: 10_000 });
      if (await page.locator(".conv-detail [data-testid=chat-starters]").count()) throw new Error("superagent 的空对话不该有「想问的」建议");
      if (await page.locator(".conv-detail .conv-head > button[aria-label]", { has: page.locator("svg.lucide-arrow-left") }).count()) throw new Error("superagent 的对话左上角不该有返回");
      await sidebar(page, true);
      await page.waitForFunction(() => ![...document.querySelectorAll(".sp-nav-chat-list .sp-nav-person")].some((b) => b.textContent?.includes("Shroom")), null, { timeout: 10_000 })
        .catch(() => { throw new Error("superagent 不应出现在侧栏的会话列表里"); });
      if (narrow) await sidebar(page, false);
      if (size.width === 1200 && scheme === "light") await shot(page, size, scheme, "superagent-crown");
      const ask = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ what: "删除项目 old-site", reason: "这个项目三个月没动了，文档已经并进 website。" }),
      });
      if (!ask.ok) throw new Error(`superagent 发起确认失败：HTTP ${ask.status}`);
      const card = page.locator("[data-testid=approvals] .sa-approval");
      await card.waitFor({ timeout: 25_000 });
      const text = compactText(await card.innerText());
      if (!text.includes("删除项目 old-site") || !text.includes("三个月没动")) throw new Error(`确认卡片应写明要做什么和为什么，实际「${text}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "superagent-approval");
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      const badge = page.locator("[data-testid=menu-superagent] .app-hub-menu-badge");
      await badge.waitFor({ timeout: 10_000 });
      if ((await badge.innerText()).trim() !== "1") throw new Error(`菜单应标出 1 件等你点头的事，实际「${await badge.innerText()}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "superagent-menu");
      await page.keyboard.press("Escape");
      await page.locator(".app-hub-menu").waitFor({ state: "hidden" }).catch(() => undefined);
      if (narrow) await sidebar(page, false);
      await clickVisible(page, card.getByRole("button", { name: "同意", exact: true }));
      await card.waitFor({ state: "hidden", timeout: 10_000 });
    });

    // A superagent from a sentence: copied to any session (a cloud one too),
    // it joins and is your superagent; its chat opens.
    await step("superagent · 复制一句话接入", async () => {
      await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, { method: "DELETE" });
      // (the App keeps what it knew for a while: start it afresh)
      await page.reload({ waitUntil: "domcontentloaded" });
      await page.locator(".sp-home, .sp-group, .sp-first").first().waitFor({ timeout: 20_000 });
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await page.locator(".app-hub-menu").waitFor();
      await clickVisible(page, page.locator("[data-testid=menu-superagent]"));
      const dialog = page.locator("[data-testid=superagent-setup]");
      await dialog.waitFor({ timeout: 10_000 });
      await clickVisible(page, dialog.locator("[data-testid=superagent-link-tab]"));
      const sentence = dialog.locator("[data-testid=superagent-link] .add-sentence-text");
      await sentence.waitFor({ timeout: 10_000 });
      // (the link is made when the tab opens: "正在生成…" until then)
      await page.waitForFunction(() => (document.querySelector("[data-testid=superagent-link] .add-sentence-text")?.textContent || "").includes("/j/"), null, { timeout: 15_000 }).catch(() => undefined);
      const text = compactText(await sentence.innerText());
      if (!text.includes("myco superagent guide") || !text.includes("/j/")) throw new Error(`那句话应带加入命令和 superagent 说明，实际「${text}」`);
      await settle(page, 200);
      await shot(page, size, scheme, "superagent-link");
      await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 30_000 });
      await page.locator(".conv-detail .conv-head h2", { hasText: "Shroom" }).waitFor({ timeout: 20_000 });
    });

    // The ⌘⇧Space panel (the Mac app's own window, /?quick=1): one line to
    // hand Shroom a thing; sent, the app opens the chat (its answer is
    // there). What waits on you shows as one line.
    await step("superagent · 快捷面板", async () => {
      const qp = await page.context().newPage();
      try {
        await qp.setViewportSize({ width: 664, height: 760 });
        await qp.goto(`${r.base}/?quick=1&lang=zh`, { waitUntil: "domcontentloaded" });
        const panel = qp.locator("[data-testid=quick-panel]");
        await panel.waitFor({ timeout: 15_000 });
        await panel.locator(".qp-name", { hasText: "Shroom" }).waitFor({ timeout: 15_000 });
        const ask = await fetch(`http://${HOST}:${mockPort()}/app/mock/superagent-ask`, {
          method: "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ what: "停掉自动化「每晚跑测试」", reason: "它连着 5 天都失败在同一个地方，先停下来等修好。" }),
        });
        if (!ask.ok) throw new Error(`superagent 发起确认失败：HTTP ${ask.status}`);
        await fetch(`http://${HOST}:${mockPort()}/app/mock/quick`, { method: "DELETE" });
        const waiting = qp.locator("[data-testid=quick-waiting]");
        await waiting.waitFor({ timeout: 25_000 });
        if (!compactText(await waiting.innerText()).includes("1 件事等你点头")) throw new Error(`应写明有几件事等你点头，实际「${await waiting.innerText()}」`);
        if (await qp.locator(".qp-answer, .qp-mine, .sa-approval").count()) throw new Error("面板里不该再显示对话内容或确认卡片：那些在对话页");
        const box = qp.locator("[data-testid=quick-input]");
        await box.fill("帮我看看今天哪些任务卡住了");
        await settle(qp, 300);
        const name = `${size.width}x${size.height}-${scheme}-superagent-quick.png`;
        await panel.screenshot({ path: path.join(OUT, name) });
        log(`    截图 ui-smoke/${name}`);
        // Sent: the message goes, the input empties, and the app is asked to open the chat.
        await box.press("Enter");
        await qp.waitForFunction(() => (document.querySelector("[data-testid=quick-input]")?.value || "") === "", null, { timeout: 10_000 });
        const opened = await (await fetch(`http://${HOST}:${mockPort()}/app/mock/quick`)).json();
        if (!opened.open?.agent || !opened.open?.hub) throw new Error(`发出去后应打开对话，实际 ${JSON.stringify(opened.open)}`);
      } finally {
        await qp.close();
      }
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
      // The voice is the agent itself: its lines carry its face, not a "语音" label.
      if (!(await call.locator(".vc-line.is-them .vc-line-who.is-face .pa").count())) throw new Error("语音里 agent 说的行前面应是它的头像");
      if (compactText(await call.locator(".vc-line.is-them .vc-line-who").first().innerText()) === "语音") throw new Error("不该再标「语音」");
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
      // (another page: Settings is a dialog over the chat, which stays)
      await go(page, "自动化", narrow);
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
      // Words to hear right: one per line, saved with the rest.
      const wordsBox = page.locator("[data-setting=voice-words] textarea");
      await wordsBox.fill("Kettu\nClaude = 克劳德, cloud");
      await clickVisible(page, page.locator("[data-setting=voice-words] .app-btn-primary"));
      for (let i = 0; ; i++) {
        const got = (await (await fetch(`http://${HOST}:${mockPort()}/app/voice`)).json()).config?.words || [];
        if (got.join("|") === "Kettu|Claude = 克劳德, cloud") break;
        if (i > 50) throw new Error(`要听准的词应一行一个存下，实际「${got.join("|")}」`);
        await settle(page, 200);
      }
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
      await page.locator("[data-testid=pc-empty]", { hasText: "还没有连上的电脑" }).waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "computers-empty");
      if (narrow) await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-nav-pcs").getByRole("button", { name: "加一个终端" }));
      const pick = page.locator(".app-modal-add");
      await pick.waitFor({ timeout: 10_000 });
      if (!(await pick.locator("h3").first().innerText()).includes("开一个终端") || (await pick.locator("h3").first().innerText()).includes("项目")) throw new Error("工作区里开终端不该说「给项目」");
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
      // A name, a goal (optional) and its files (optional): listed before
      // it is made, one taken away again.
      const form = page.locator("[data-testid=new-project-form]");
      await form.locator(".np-input").fill("官网改版");
      await form.locator(".np-goal").fill("11 月底前上线新版官网：首页、文档站和下载页，中英双语。");
      await form.locator("input[type=file]").setInputFiles([
        { name: "需求.md", mimeType: "text/markdown", buffer: Buffer.from("# 需求\n") },
        { name: "设计稿.png", mimeType: "image/png", buffer: Buffer.alloc(2048) },
      ]);
      await form.locator("[data-testid=new-project-files] .np-file").nth(1).waitFor({ timeout: 5_000 });
      await form.getByRole("button", { name: "建好，并上传 2 个资料" }).waitFor({ timeout: 5_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "dialog-new-project");
      await clickVisible(page, form.getByRole("button", { name: "去掉 设计稿.png" }));
      await form.getByRole("button", { name: "建好，并上传 1 个资料" }).waitFor({ timeout: 5_000 });
      await closeDialog(page);
      if (narrow) await sidebar(page, false);
    });

    // A project just made is in the sidebar at once, even while the
    // workspaces answer slowly (reading them all again used to hold it back
    // for up to a minute), and stays there.
    await step("新建项目 · 立刻出现在边栏", async () => {
      const slow = async (r) => {
        if (r.request().method() !== "GET") return r.continue();
        await new Promise((res) => setTimeout(res, 8000));
        await r.continue().catch(() => undefined);
      };
      await page.route(/\/app\/hubs\/[^/]+\/projects(\?|$)/, slow);
      await page.route("**/app/overview", slow);
      try {
        await sidebar(page, true);
        await clickVisible(page, page.locator(".app-nav-add"));
        const form = page.locator("[data-testid=new-project-form]");
        await form.locator(".np-input").fill("发布清单");
        const t0 = Date.now();
        await clickVisible(page, form.getByRole("button", { name: "建好了" }));
        await page.locator(".app-sidebar .app-nav-item", { hasText: "发布清单" }).first().waitFor({ timeout: 2_500 });
        if (Date.now() - t0 > 2_500) throw new Error("新建的项目应立刻出现在边栏");
        await closeDialog(page).catch(() => undefined);
        await page.waitForTimeout(9_000); // the slow reads come back (from before it was made)
        if (!(await page.locator(".app-sidebar .app-nav-item", { hasText: "发布清单" }).count())) throw new Error("新建的项目不该被慢回来的旧列表冲掉");
      } finally {
        await page.unroute(/\/app\/hubs\/[^/]+\/projects(\?|$)/, slow);
        await page.unroute("**/app/overview", slow);
      }
      if (narrow) await sidebar(page, false);
    });

    // Adding a member already in the workspace: it shows in the members
    // column at once, while the workspaces answer slowly (the column used
    // to wait for every workspace to be read again).
    await step("项目 · 加成员立刻出现", async () => {
      const slow = async (r) => {
        if (r.request().method() !== "GET") return r.continue();
        await new Promise((res) => setTimeout(res, 8000));
        await r.continue().catch(() => undefined);
      };
      await sidebar(page, true);
      // (in the demo workspace, which has agents to add; earlier steps may
      // have left another on show)
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      const studio = page.locator(".app-hub-menu [role=menuitemradio]", { hasText: "Alice" }).first();
      if (await studio.getAttribute("aria-checked") === "true") await page.keyboard.press("Escape");
      else await clickVisible(page, studio);
      await page.locator(".app-hub-menu").waitFor({ state: "detached", timeout: 5_000 });
      await clickVisible(page, page.locator(".app-nav-add"));
      const form = page.locator("[data-testid=new-project-form]");
      await form.locator(".np-input").fill("加人测试");
      await clickVisible(page, form.getByRole("button", { name: "建好了" }));
      // (a new project opens with 添加 agent)
      const dlg = page.locator(".app-modal-add");
      await dlg.waitFor({ timeout: 10_000 });
      await page.route(/\/app\/hubs\/[^/]+\/projects(\/[^/?]+)?(\?|$)/, slow);
      await page.route("**/app/overview", slow);
      try {
        const have = dlg.getByRole("tab", { name: "工作区里已有的" });
        if (!(await have.waitFor({ timeout: 5_000 }).then(() => true, () => false))) {
          throw new Error(`添加 agent 里没有「工作区里已有的」：${(await dlg.innerText()).replace(/\s+/g, " ").slice(0, 300)}`);
        }
        await clickVisible(page, have);
        const row = dlg.locator(".add-local-row").first();
        const name = (await row.locator(".add-local-name").innerText()).trim();
        const t0 = Date.now();
        await clickVisible(page, row.getByRole("button", { name: "加进来" }));
        // (done when the other rows' buttons work again)
        await page.waitForFunction(() => [...document.querySelectorAll(".app-modal-add .add-local-row button")].every((b) => !b.disabled), null, { timeout: 3_000 });
        await closeDialog(page);
        if (narrow) await page.locator(".sp-strip, .sp-side").first().waitFor({ timeout: 5_000 });
        await page.locator(".sp-side .sp-member, .sp-strip .sp-strip-face", { hasText: narrow ? undefined : name }).first().waitFor({ timeout: 3_000 });
        if (Date.now() - t0 > 3_000) throw new Error(`加进项目的「${name}」应立刻出现在成员栏`);
        await page.waitForTimeout(9_000);
        if (!narrow && !(await page.locator(".sp-side .sp-member", { hasText: name }).count())) throw new Error(`「${name}」不该被慢回来的旧列表冲掉`);
      } finally {
        await page.unroute(/\/app\/hubs\/[^/]+\/projects(\/[^/?]+)?(\?|$)/, slow);
        await page.unroute("**/app/overview", slow);
      }
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

    await step("用量", async () => {
      await go(page, "用量", narrow);
      const usage = page.locator(".usage-page");
      await usage.locator(".usage-metrics dd").first().waitFor({ timeout: 15000 });
      if (await usage.locator(".usage-metrics dd").count() !== 4) throw new Error("用量页应有四项概览");
      if (await usage.locator(".usage-agents tbody tr").count() < 8) throw new Error("成员用量应有八个成员");
      if (await usage.locator(".usage-projects tbody tr").count() < 3) throw new Error("项目用量应包含三个项目");
      if (await usage.locator(".usage-model-table tbody tr").count() < 3) throw new Error("模型汇总应有内容");
      if (await usage.locator(".usage-day").count() !== 30) throw new Error("默认显示 30 天");
      const width = await page.evaluate(() => document.documentElement.scrollWidth);
      if (width > size.width) throw new Error(`用量页横向溢出：${width} > ${size.width}`);
      await shot(page, size, scheme, "usage");
      await usage.getByRole("button", { name: "7 天", exact: true }).click();
      await page.waitForFunction(() => document.querySelectorAll(".usage-day").length === 7);
      const project = usage.getByRole("combobox", { name: "项目", exact: true });
      // (a project with usage: the first in the projects table; a project
      // made by an earlier step has none)
      const title = (await usage.locator(".usage-projects tbody th").first().innerText()).trim();
      await project.selectOption({ label: title });
      await usage.locator(".usage-agents tbody tr").first().waitFor({ timeout: 10000 });
      if (await usage.locator(".usage-projects").count()) throw new Error("指定项目时不应再显示项目汇总");
      await project.selectOption("-");
      await usage.locator(".usage-agents tbody tr").first().waitFor({ timeout: 10000 });
      await project.selectOption("");
      await usage.getByRole("button", { name: "30 天", exact: true }).click();
      await usage.locator(".usage-projects tbody tr").first().waitFor({ timeout: 10000 });
      await usage.locator(".usage-member").first().click();
      await page.locator(".conv-detail").first().waitFor({ timeout: 15000 });
      await go(page, "用量", narrow);
      await usage.locator(".usage-metrics dd").first().waitFor({ timeout: 10000 });
      if (await usage.locator(".usage-day").count() !== 30) throw new Error("返回用量页应记住时间范围");
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
        // Over SSH first: the SSH hosts, one server picked at a time.
        const hubSSH = dlg.locator("[data-testid=connect-ssh]");
        await hubSSH.locator(".cx-row", { hasText: "hk-vps" }).waitFor({ timeout: 10_000 });
        await settle(page, 200);
        await shot(page, size, scheme, "connect-hub-ssh");
        await clickVisible(page, dlg.getByRole("tab", { name: "复制命令" }));
        const way = dlg.locator("[data-testid=connect-agent-way] .add-sentence-text");
        await way.waitFor({ timeout: 5_000 });
        if (!(await way.innerText()).includes("agent-setup.md")) throw new Error("添加工作区里应有让 agent 帮你装的那句话");
        await settle(page, 200);
        await shot(page, size, scheme, "connect-agent");
        // A server where sudo wants a password: what to run instead.
        await clickVisible(page, dlg.getByRole("tab", { name: "用 SSH 装到服务器上" }));
        await clickVisible(page, hubSSH.locator(".cx-row", { hasText: "old-pi" }));
        await clickVisible(page, hubSSH.getByRole("button", { name: "装好并加进来" }));
        await hubSSH.locator(".cx-error", { hasText: "sudo" }).waitFor({ timeout: 10_000 });
        await hubSSH.locator(".cx-error .app-command code", { hasText: "install-hub.sh" }).waitFor({ timeout: 5_000 });
        await settle(page, 200);
        await shot(page, size, scheme, "connect-hub-ssh-error");
        // One that works: installed, added, and the dialog closes.
        await clickVisible(page, hubSSH.getByRole("button", { name: "再连别的" }));
        await clickVisible(page, hubSSH.locator(".cx-row", { hasText: "hk-vps" }));
        await clickVisible(page, hubSSH.getByRole("button", { name: "装好并加进来" }));
        await hubSSH.locator(".cx-step.is-running").first().waitFor({ timeout: 10_000 });
        await dlg.waitFor({ state: "hidden", timeout: 20_000 });
      }
      // Opening a workspace of this computer to the internet can be
      // cancelled while it opens (it used to hang there with no way out).
      await clickVisible(page, page.getByRole("button", { name: "开放到公网" }));
      const pub = page.locator(".app-modal", { hasText: "让不在同一局域网的电脑" });
      // One click is the default, even before cloudflared is installed.
      await pub.locator(".app-choice:not(.disabled)", { hasText: "一键开放" }).locator("input:checked").waitFor({ state: "attached", timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "open-to-internet");
      // A missing Tailscale must lead to install/login guidance, not a dead row.
      await clickVisible(page, pub.locator(".pub-option", { hasText: "Tailscale Funnel" }));
      const setup = pub.getByTestId("tailscale-setup");
      await setup.waitFor();
      if (!(await pub.getByRole("button", { name: "开放", exact: true }).isDisabled())) throw new Error("未安装 Tailscale 时应先引导安装");
      for (const [platform, command] of [["Windows", "winget install --exact --id Tailscale.Tailscale"], ["macOS", "brew install --cask tailscale-app"], ["Linux", "https://tailscale.com/install.sh"]]) {
        await clickVisible(page, setup.getByRole("button", { name: platform, exact: true }));
        await setup.locator(".app-command code").first().filter({ hasText: command }).waitFor();
      }
      await clickVisible(page, setup.getByRole("button", { name: "Windows", exact: true }));
      let downloadURL = "";
      const downloadRoute = /\/app\/open-url$/;
      const downloadHandler = async route => { downloadURL = route.request().postDataJSON().url; await route.fulfill({ status: 200, contentType: "application/json", body: '{"ok":true}' }); };
      await page.route(downloadRoute, downloadHandler);
      try {
        await clickVisible(page, setup.getByRole("button", { name: "打开官方下载" }));
        await page.waitForTimeout(100);
        if (downloadURL !== "https://tailscale.com/download/windows") throw new Error("应打开 Tailscale 官方 Windows 下载页");
      } finally { await page.unroute(downloadRoute, downloadHandler); }
      await clickVisible(page, setup.locator("summary"));
      await setup.locator("details code", { hasText: "myco hub up --tunnel tailscale" }).waitFor();
      await clickVisible(page, setup.locator("summary"));
      await shot(page, size, scheme, "open-to-internet-tailscale-setup");
      await fetch(`http://${HOST}:${mockPort()}/app/mock/local`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ tailscale_installed: true }) });
      try {
        await clickVisible(page, setup.getByRole("button", { name: "我已安装，重新检测" }));
        await setup.waitFor({ state: "hidden" });
        if (!(await pub.getByRole("button", { name: "开放", exact: true }).isEnabled())) throw new Error("安装后检测应解锁下一步");
      } finally {
        await fetch(`http://${HOST}:${mockPort()}/app/mock/local`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ tailscale_installed: false }) });
      }
      await clickVisible(page, pub.locator(".pub-option", { hasText: "一键开放" }));
      await clickVisible(page, pub.getByRole("button", { name: "开放", exact: true }));
      await pub.getByRole("button", { name: "正在开放…" }).waitFor({ timeout: 5_000 });
      await pub.locator("[data-testid=opening] .pub-steps li").first().waitFor({ timeout: 5_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "open-to-internet-opening");
      await clickVisible(page, pub.getByRole("button", { name: "取消" }));
      await pub.locator("button.app-btn-primary:enabled", { hasText: "开放" }).waitFor({ timeout: 3_000 });
      // An older Windows backend could return HTTP 200 with no public URL.
      // Neither that response nor a not-ready error may show the success panel.
      let publicResponse = "empty";
      const localHubRoute = /\/app\/local\/hub$/;
      const publicHandler = async (route) => {
        const body = publicResponse === "pending"
          ? { error: "tunnel_not_ready", message: "tunnel not ready", retryable: true }
          : { hub: {}, public_url: publicResponse === "ready" ? "https://ready.example.test" : "", warnings: [] };
        if (publicResponse === "pending") intentionalPublicNotReady.add(page);
        await route.fulfill({ status: publicResponse === "pending" ? 503 : 200, contentType: "application/json", body: JSON.stringify(body) });
      };
      await page.route(localHubRoute, publicHandler);
      try {
        await clickVisible(page, pub.getByRole("button", { name: "开放", exact: true }));
        await pub.getByText("尚未拿到有效的公网地址", { exact: false }).waitFor({ timeout: 5_000 });
        if (await pub.locator("[data-testid=opened]").count()) throw new Error("空地址不能显示已开放");
        publicResponse = "pending";
        await clickVisible(page, pub.getByRole("button", { name: "开放", exact: true }));
        await pub.getByText("公网连接尚未就绪", { exact: false }).waitFor({ timeout: 5_000 });
        if (await pub.locator("[data-testid=opened]").count()) throw new Error("公网未就绪不能显示已开放");
        await shot(page, size, scheme, "open-to-internet-not-ready");
        publicResponse = "ready";
        await clickVisible(page, pub.getByRole("button", { name: "开放", exact: true }));
        await pub.locator("[data-testid=opened]").waitFor({ timeout: 5_000 });
        await pub.locator(".pub-link code", { hasText: "https://ready.example.test" }).waitFor();
        await clickVisible(page, pub.getByRole("button", { name: "完成", exact: true }));
      } finally { intentionalPublicNotReady.delete(page); await page.unroute(localHubRoute, publicHandler); }
      await pub.waitFor({ state: "hidden", timeout: 5_000 });
    });

    // Removing a workspace (last: it changes what the later steps would
    // see) only touches this App, so it is done at once even when the
    // workspaces answer slowly; the one removed leaves every page and
    // nothing asks it again.
    await step("设置 · 项目文件额度", async () => {
      await go(page,"设置",narrow);
      await clickVisible(page,page.locator(".app-settings-tab",{hasText:"工作区"}));
      const card=page.locator('[data-setting="project-storage"][data-hub="hub_studio"]');
      await card.locator("select").selectOption("website");
      const files=card.getByRole("spinbutton",{name:"文件数上限",exact:true});
      const capacity=card.getByRole("spinbutton",{name:"项目总大小上限（GiB）",exact:true});
      await files.waitFor();
      await files.fill("10001");
      if(!await card.getByRole("button",{name:"保存",exact:true}).isDisabled())throw Error("file limit must enforce hard cap");
      await files.fill("2000");await capacity.fill("4");
      await card.getByRole("button",{name:"保存",exact:true}).click();
      await card.getByRole("status").filter({hasText:"已保存"}).waitFor();
      const quota=await page.evaluate(async()=>await(await fetch("/app/hubs/hub_studio/projects/website/storage")).json());
      if(quota.files_max!==2000||quota.bytes_max!==4*1024**3)throw Error("saved project quota did not round-trip");
      await shot(page,size,scheme,"project-storage");
      await card.getByRole("button",{name:"使用工作区默认值",exact:true}).click();
      await page.waitForFunction(()=>document.querySelector('[data-setting="project-storage"][data-hub="hub_studio"] input[type=number]')?.value==="1000");
      await closeSettings(page);
    });

    await step("设置 · 移除工作区", async () => {
      await closeSettings(page);
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      const studio = page.locator(".app-hub-menu [role=menuitemradio]", { hasText: "Alice" }).first();
      if (await studio.getAttribute("aria-checked") === "true") await page.keyboard.press("Escape");
      else await clickVisible(page, studio);
      await go(page, "用量", narrow);
      await page.locator(".usage-metrics dd").first().waitFor({ timeout: 10_000 });
      const shownBefore = (await page.locator(".sp-switch .app-hub-name, .app-hub-pill-name").first().innerText().catch(() => "")).trim();
      await go(page, "设置", narrow);
      // Hold a refresh that read the workspace list before the removal.
      // Its late overview must not restore that list and remount UsagePage.
      let releaseOverview;
      const held = new Promise((resolve) => { releaseOverview = resolve; });
      let overviewRead;
      const ready = new Promise((resolve) => { overviewRead = resolve; });
      const slowOverview = async (r) => {
        const response = await r.fetch();
        overviewRead();
        await held;
        await r.fulfill({ response }).catch(() => undefined);
      };
      await page.route("**/app/overview", slowOverview);
      await page.evaluate(() => window.dispatchEvent(new KeyboardEvent("keydown", { key: "r", ctrlKey: true, metaKey: true, bubbles: true })));
      await Promise.race([ready, page.waitForTimeout(10_000).then(() => { throw new Error("工作区刷新没有开始"); })]);
      const asked = [];
      let removed = "";
      const onResponse = (r) => { if (r.status() === 404 && /\/app\/hubs\//.test(r.url())) asked.push(r.url()); };
      page.on("response", onResponse);
      try {
        const rm = page.locator("[data-testid=settings-dialog]").getByRole("button", { name: /^移除/ }).first();
        await clickVisible(page, rm);
        const dlg = page.locator(".app-modal", { hasText: "只会从这个 App 里移除" });
        await dlg.waitFor({ timeout: 5_000 });
        removed = ((await dlg.innerText()).match(/移除 (.+?)？/) || [])[1] || "";
        if (!removed) throw new Error("移除的确认框应写着要移除哪个工作区");
        const t0 = Date.now();
        await clickVisible(page, dlg.getByRole("button", { name: "移除工作区" }));
        await dlg.waitFor({ state: "detached", timeout: 3_000 });
        if (Date.now() - t0 > 2_000) throw new Error(`移除工作区不该等工作区回话：用了 ${Date.now() - t0}ms`);
        releaseOverview();
        await page.waitForTimeout(1_500);
        if (asked.length) throw new Error(`移除后不该再问那个工作区：${asked[0]}`);
      } finally {
        releaseOverview();
        page.off("response", onResponse);
        await page.unroute("**/app/overview", slowOverview);
      }
      await closeSettings(page);
      await sidebar(page, true);
      const shownAfter = (await page.locator(".sp-switch .app-hub-name").first().innerText()).trim();
      if (removed === shownBefore && shownAfter === removed) throw new Error(`移除了正在看的工作区，应换到别的：还是「${shownAfter}」`);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      const names = await page.locator(".app-hub-menu [role=menuitemradio] .app-hub-menu-name").allInnerTexts();
      await page.keyboard.press("Escape");
      if (names.map((n) => n.trim()).includes(removed)) throw new Error(`移除的「${removed}」不该还在工作区菜单里`);
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
  if (SHARD) log(`  第 ${SHARD.i}/${SHARD.n} 片：每个窗口只跑第 ${shardRange().from + 1}–${shardRange().to} 个步骤（共 ${STEPS} 个）`);

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
        matrix: `${c.size.width}×${c.size.height} ${c.scheme}`, step: "启动", lines: [], timings: [], flakes: [],
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
  await reportFlakes(runs.flatMap((r) => r.flakes));
  // Each step's time (for the shard weights), and with --write-weights the
  // weights file itself: per step, its slowest time across the windows.
  {
    const { writeFile } = await import("node:fs/promises");
    const timings = runs.flatMap((r) => r.timings.map((t) => ({ matrix: r.matrix, label: t.label, ms: t.ms })));
    await writeFile(path.join(OUT, "timings.json"), JSON.stringify(timings, null, 2) + "\n").catch(() => undefined);
    if (process.argv.includes("--write-weights") && !failed.length) {
      const file = path.join(path.dirname(fileURLToPath(import.meta.url)), "ui-smoke-weights.json");
      const fresh = {};
      for (const t of timings) fresh[t.label] = Math.max(fresh[t.label] || 0, Math.round(t.ms / 100) / 10);
      const w = { ...WEIGHTS, ...fresh };
      const ordered = Object.fromEntries(STEP_LABELS.filter((l) => l in w).map((l) => [l, w[l]]));
      await writeFile(file, JSON.stringify(ordered, null, 2) + "\n");
      log(`写好了 ${path.relative(WEB, file)}（${Object.keys(ordered).length} 个步骤）`);
    }
  }
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
