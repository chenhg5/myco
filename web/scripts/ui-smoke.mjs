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

function compactText(s) {
  return (s || "").replace(/\s+/g, " ").trim();
}

// ⌘ on a Mac, Ctrl elsewhere: the key that puts the next message behind the
// running one.
const NEXT_KEY = process.platform === "darwin" ? "Meta+Enter" : "Control+Enter";

/** A message written from "another device": straight to the mock, so two
 *  tasks can be on their way at once (the box adds to the running one now). */
async function sendFromElsewhere(hubId, agent, body) {
  const r = await fetch(`http://${HOST}:${MOCK_PORT}/app/hubs/${hubId}/messages`, {
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
    return !!d && !!el && d.querySelectorAll(".conv-msg").length > 0
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
  currentMatrix = `${size.width}×${size.height} ${scheme}`;
  log(`\n▶ ${currentMatrix}`);
  // The stuck-task step stops and resends the demo task; the mock keeps
  // running, so put it back before each width and theme.
  const reset = await fetch(`http://${HOST}:${MOCK_PORT}/app/mock/stuck`, { method: "POST" });
  if (!reset.ok) throw new Error(`重置「可能卡住」的演示任务失败：HTTP ${reset.status}`);
  // The machine-delete step removes the demo machine and its nodes.
  const resetMachine = await fetch(`http://${HOST}:${MOCK_PORT}/app/mock/machine`, { method: "POST" });
  if (!resetMachine.ok) throw new Error(`重置演示机器失败：HTTP ${resetMachine.status}`);
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
      await settle(page, 300);
      await shot(page, size, scheme, "project-member-work");
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

    // A chat with nothing in it yet: the agent, large, and a question.
    // What was typed and not sent stays with its chat when you go to
    // another and come back; an empty box is one line, whatever its hint.
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
      const box = page.locator(".conv-detail .conv-compose-input");
      // The mock's answers take turns; one in four carries a picture.
      for (let i = 0; i < 4 && !(await page.locator(".conv-detail .conv-attach.is-image").count()); i++) {
        await box.fill(`图片附件 ${i + 1}`);
        await box.press("Enter");
        await page.waitForTimeout(6800);
      }
      const thumb = page.locator(".conv-detail .conv-attach.is-image").last();
      await thumb.waitFor({ timeout: 20_000 });
      const w = await thumb.locator("img").evaluate((img) => new Promise((ok) => (img.complete ? ok(img.naturalWidth) : img.addEventListener("load", () => ok(img.naturalWidth), { once: true }))));
      if (!w) throw new Error("图片附件应显示缩略图");
      await thumb.scrollIntoViewIfNeeded();
      await settle(page, 300);
      await shot(page, size, scheme, "message-picture");
      await thumb.click();
      await page.locator(".lightbox .lightbox-img").waitFor({ timeout: 5_000 });
      await shot(page, size, scheme, "message-picture-open");
      await page.keyboard.press("Escape");
      await page.locator(".lightbox").waitFor({ state: "hidden", timeout: 5_000 });
      const file = page.locator(".conv-detail button.conv-attach:not(.is-image)").last();
      await file.click();
      await page.locator(".conv-detail button.conv-attach:not(.is-image)", { hasText: "已存到「下载」" }).first().waitFor({ timeout: 5_000 });
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
      await box.press(NEXT_KEY);
      if ((await rows.count()) !== 2) throw new Error(`连着排两条应有两行，实际 ${await rows.count()} 行`);
      const order = (await rows.allInnerTexts()).map(compactText);
      if (!order[0].includes(nextText) || !order[1].includes(dropText)) throw new Error(`排队应按先后排：${JSON.stringify(order)}`);
      if (!order[0].includes("排队中")) throw new Error(`排队条应写「排队中：」，实际「${order[0]}」`);
      // Enter now adds to the running task: drawn inside it, not a second one.
      const addText = "补充：下载页的链接也顺带检查";
      await box.fill(addText);
      await box.press("Enter");
      const add = page.locator(".conv-detail .conv-supplement", { hasText: "下载页的链接" });
      await add.waitFor({ timeout: 20_000 });
      const oneBubble = await page.evaluate(({ task, addText }) => {
        const mine = [...document.querySelectorAll(".conv-detail .conv-msg.is-me")];
        const said = mine.filter((el) => (el.textContent || "").includes(addText));
        return said.length === 1 && (said[0].textContent || "").includes(task);
      }, { task, addText });
      if (!oneBubble) throw new Error("补充应贴在那条任务里，不是单独一条消息");
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
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-queue-sent");
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
      await page.keyboard.press("Escape");
      await page.locator(".ms").waitFor({ state: "hidden" });
    });

    await step("电脑", async () => {
      await go(page, "电脑", narrow);
      await page.locator(".pc-row").first().waitFor({ timeout: 20_000 });
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
