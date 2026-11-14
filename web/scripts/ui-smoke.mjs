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
  // Computers is a heading over the computers listed in the sidebar.
  const item = label === "电脑" ? page.locator(".sp-nav-title-link:visible") : page.locator(".app-nav-item:visible", { hasText: label });
  await clickVisible(page, item);
  // At narrow widths the sidebar covers the window; show the page as a person
  // would after picking it, with the compact Hub pill in the crumb bar.
  if (narrow) await sidebar(page, false);
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
      await page.locator(".sp-dispatch").first().waitFor({ timeout: 20_000 });
      const card = compactText(await page.locator(".sp-dispatch").first().innerText());
      if (!card.startsWith("派给")) throw new Error(`派活卡片应写明派给谁，实际是「${card}」`);
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

    // The members column folds to a strip of faces, and stays so.
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
      await clickVisible(page, page.locator(".sp-dispatch", { hasText: text }));
      const right = page.locator(".sp-pane-right");
      await right.locator(".conv-msg.is-other", { hasText: text }).first().waitFor({ timeout: 20_000 });
      await page.waitForFunction((t) => {
        const box = document.querySelector(".sp-pane-right .conv-scroll")?.getBoundingClientRect();
        const el = [...document.querySelectorAll(".sp-pane-right .conv-msg")].find((m) => (m.textContent || "").includes(t));
        if (!el || !box) return false;
        const r = el.getBoundingClientRect();
        return r.bottom > box.top && r.top < box.bottom;
      }, text, { timeout: 10_000 });
      const head = compactText(await right.locator(".conv-head h2").innerText());
      if (!/docs-claude/.test(head)) throw new Error(`派活卡片应打开接活的成员，实际是「${head}」`);
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
        await clickVisible(page, right.getByRole("button", { name: "只看这一栏" }));
        await page.locator(".sp-split.show-right").waitFor({ timeout: 5_000 });
        await settle(page, 200);
        await shot(page, size, scheme, "project-chat-full");
        // Full width stays while you switch: the coordinator takes it too.
        await clickVisible(page, page.locator(".sp-side .sp-member").first());
        await page.locator(".sp-split.show-left").waitFor({ timeout: 5_000 });
        await clickVisible(page, page.locator(".sp-side .sp-member").nth(1));
        await page.locator(".sp-split.show-right").waitFor({ timeout: 5_000 });
        await clickVisible(page, right.getByRole("button", { name: "还原成两栏" }));
        await page.locator(".sp-split.show-both").waitFor({ timeout: 5_000 });
      }
      await clickVisible(page, right.getByRole("button", { name: "关掉这一栏" }));
      await page.locator(".sp-home").waitFor({ timeout: 10_000 });
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
      await dialog.locator("input.app-input.mono").fill("~/code/website");
      await settle(page, 300);
      await shot(page, size, scheme, "add-agent-new");
      await clickVisible(page, dialog.getByRole("button", { name: "开一个" }));
      await dialog.locator(".add-arrived").nth(2).waitFor({ timeout: 20_000 });
      await clickVisible(page, dialog.getByRole("button", { name: "好了" }));
      await page.locator(".app-modal").waitFor({ state: "hidden" });
    });

    if (!narrow) {
      await step("项目 · 添加工作空间", async () => {
        await clickVisible(page, page.locator(".sp-side").getByRole("button", { name: "加一台电脑" }));
        const dialog = page.locator(".app-modal-add");
        await dialog.waitFor();
        await dialog.locator(".ws-computer.active").waitFor({ timeout: 10_000 });
        // What it means is said calmly (no red block).
        if (await dialog.locator(".app-warn").count()) throw new Error("加电脑的弹窗不该有红色警告块");
        const folder = `~/code/smoke-${size.width}-${scheme}`;
        await dialog.locator("input.app-input").fill(folder);
        await settle(page, 300);
        await shot(page, size, scheme, "add-workspace");
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
      if (title !== "聊天") throw new Error(`这里应该是聊天（不在项目里的 agent），实际标题「${title}」`);
      // Chat comes first in the sidebar, above the projects: you can start
      // talking without making a project.
      await sidebar(page, true);
      const first = compactText(await page.locator(".app-nav .app-nav-item:visible").first().innerText());
      if (!first.startsWith("聊天")) throw new Error(`边栏第一项应是「聊天」，实际是「${first}」`);
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
      const wait = page.locator(".conv-detail .trace.is-waiting").first();
      await wait.waitFor({ timeout: 20_000 });
      const line = compactText(await wait.innerText());
      if (!line.includes("排队中")) throw new Error(`拉走没开始的任务应写「排队中」，实际是「${line}」`);
      if (await page.locator(".conv-detail .trace-spin").count()) throw new Error("拉走没开始的任务不该转圈");
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
    await step("单聊 · 图片附件", async () => {
      await openMember(page, narrow, "聊天", "Review (Codex 云端)");
      await noRound(page);
      // The mock's answers take turns, one in four carries a picture: four
      // tasks (as from another device, so none is a supplement to another).
      // The first asks for a screenshot: one comes with a progress update.
      for (let i = 0; i < 4; i++) await sendFromElsewhere("hub_studio", "review-cloud", i === 0 ? "图片附件 1：改完截图给我看" : `图片附件 ${i + 1}`);
      // Several at work at once: one loading in the chat, the newest; the
      // older ones still open stand still.
      let quietSeen = false;
      for (let t = 0; t < 20; t++) {
        const n = await page.locator(".conv-detail .work-sprite:not(.is-idle)").count();
        if (n > 1) throw new Error(`聊天里同时只该有一个在动的 loading，现在有 ${n} 个`);
        quietSeen ||= (await page.locator(".conv-detail .trace.is-quiet, .conv-detail .trace-foot", { hasText: "还没做完" }).count()) > 0;
        await page.waitForTimeout(150);
      }
      if (!quietSeen) log("    （这次没有同时在做的任务，没看到「还没做完」）");
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
      if ((await frame.getAttribute("sandbox")) !== "") throw new Error("HTML 预览的框不该允许运行脚本");
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
      // At work first (the mushroom moves), then waiting.
      await page.locator(".conv-detail .work-sprite:not(.is-idle)").first().waitFor({ timeout: 15_000 });
      const wait = page.locator(".conv-detail .trace-paused", { hasText: "在等" }).last();
      await wait.waitFor({ timeout: 15_000 });
      await settle(page, 300);
      if (await page.locator(".conv-detail .work-sprite:not(.is-idle)").count()) throw new Error("它说完了在等别人，不该还有在动的 loading");
      await shot(page, size, scheme, "chat-waiting");
      // The answer comes when the other one is done; the line goes.
      await page.locator(".conv-detail .conv-msg", { hasText: "下载页核对完了" }).waitFor({ timeout: 20_000 });
      await wait.waitFor({ state: "detached", timeout: 10_000 });
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
      // It has said something and is waiting for the one it handed out.
      await page.locator(".conv-detail .trace-paused").last().waitFor({ timeout: 15_000 });
      await box.fill("补充：顺便看看 Windows 的包");
      await box.press("Enter");
      const added = page.locator(".conv-detail .trace .trace-steer", { hasText: "顺便看看 Windows 的包" });
      await added.waitFor({ timeout: 10_000 });
      if (await page.locator(".conv-detail .conv-supplement", { hasText: "顺便看看 Windows 的包" }).count()) throw new Error("补充应接在过程里，不该再画进任务气泡");
      // Nothing of the run (its words, its state) is drawn above what you added after it.
      const order = await added.evaluate((el) => {
        const run = el.closest(".trace");
        const said = [...(run?.querySelectorAll(".trace-say, .trace-group") || [])].filter((x) => x.textContent?.includes("已经派出去了"));
        const state = run?.querySelector(".trace-paused, .trace-wait");
        const y = el.getBoundingClientRect().top;
        return { saidAbove: said.every((x) => x.getBoundingClientRect().top < y), stateBelow: !state || state.getBoundingClientRect().top > y };
      });
      if (!order.saidAbove) throw new Error("补充之前它说的话应在补充上面");
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
        await scroller.evaluate((el) => { el.scrollTop = 0; });
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
      // A folder's terminal opens from here.
      await clickVisible(page, page.locator(".pc-row .app-btn-small"));
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
    });

    // The sidebar lists the computers too: one click opens a terminal.
    await step("电脑 · 边栏直达终端", async () => {
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-nav-pc").first());
      if (narrow) await sidebar(page, false);
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
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
      const dialog = page.locator(".app-modal");
      await dialog.waitFor({ timeout: 10_000 });
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

    await step("工作区切换", async () => {
      await sidebar(page, true);
      await clickVisible(page, page.locator(".sp-switch .app-hub-trigger"));
      await page.locator(".app-hub-menu").waitFor();
      await settle(page, 300);
      await shot(page, size, scheme, "workspace-switch");
      await page.keyboard.press("Escape");
      await page.locator(".app-hub-menu").waitFor({ state: "hidden" }).catch(() => undefined);
      if (await page.locator(".app-hub-menu").isVisible().catch(() => false)) await page.locator(".sp-switch .app-hub-trigger").click();
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
      // The new workspace offers the server's computers.
      await go(page, "电脑", narrow);
      await clickVisible(page, page.getByRole("button", { name: "连上另一台电脑" }));
      const add = page.locator(".app-modal");
      await add.locator(".app-picker-row").first().waitFor({ timeout: 10_000 });
      const offered = add.locator(".app-picker-row", { has: page.getByRole("button", { name: "加进来" }) }).first();
      await offered.waitFor({ timeout: 10_000 });
      const machineName = (await offered.locator(".app-picker-name").innerText()).trim();
      const row = add.locator(".app-picker-row", { has: page.locator(".app-picker-name", { hasText: machineName }) }).first();
      await clickVisible(page, row.getByRole("button", { name: "加进来" }));
      await row.locator(".app-picker-done", { hasText: "已加进来" }).waitFor({ timeout: 10_000 });
      await settle(page, 200);
      await shot(page, size, scheme, "dialog-add-computer-same-server");
      await page.keyboard.press("Escape");
      await page.locator(".app-modal").waitFor({ state: "hidden", timeout: 5_000 }).catch(() => undefined);
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

    await step("设置", async () => {
      await go(page, "设置", narrow);
      await page.locator(".app-settings").waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "settings");
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