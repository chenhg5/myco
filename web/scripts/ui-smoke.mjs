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
    await page.locator(".sp-group, .sp-first").first().waitFor({ timeout: 20_000 });
    await settle(page);

    // A project: its members' tasks as a group, a box to give one a task.
    await step("项目 · 群聊", async () => {
      await go(page, "官网改版", narrow);
      await page.locator(".sp-card").first().waitFor({ timeout: 20_000 });
      const to = compactText(await page.locator(".sp-to-name").innerText());
      if (!to) throw new Error("群聊的输入框应写明发给谁");
      await settle(page);
      await shot(page, size, scheme, "project-group");
    });

    await step("项目 · 派活", async () => {
      const body = `smoke ${size.width} ${scheme}：把首页的标题改短一点。`;
      const input = page.locator(".sp-compose-input");
      // The box grows with several lines instead of scrolling one.
      const oneLine = await input.evaluate((el) => el.getBoundingClientRect().height);
      await input.fill("第一行\n第二行\n第三行\n第四行");
      const grown = await input.evaluate((el) => ({ h: el.getBoundingClientRect().height, scrolls: el.scrollHeight > el.clientHeight + 1 }));
      if (grown.h < oneLine * 2.5 || grown.scrolls) throw new Error(`输入框应随内容变高而不出现滚动条：一行 ${oneLine}px，四行 ${grown.h}px，滚动 ${grown.scrolls}`);
      await settle(page, 200);
      await shot(page, size, scheme, "project-compose-lines");
      await input.fill(body);
      await input.press("Enter");
      const card = page.locator(".sp-card", { hasText: body });
      await card.waitFor({ timeout: 20_000 });
      const who = compactText(await card.locator(".sp-card-who").innerText());
      if (!who.startsWith("你")) throw new Error(`派出去的任务应写明是你发的，实际是「${who}」`);
      await settle(page, 300);
      await shot(page, size, scheme, "project-sent");
      // A task opens its conversation; Back returns to the group.
      await card.click();
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
      await page.locator(".conv-msg", { hasText: body }).first().waitFor({ timeout: 20_000 });
      await clickVisible(page, page.locator(".conv-head button").first());
      await page.locator(".sp-group").waitFor({ timeout: 10_000 });
    });

    await step("项目 · 单聊", async () => {
      if (narrow) await clickVisible(page, page.locator(".sp-strip-face:not(.is-add)"));
      else await clickVisible(page, page.locator(".sp-side .sp-member"));
      await page.locator(".conv-detail .conv-head-faces .pa").first().waitFor({ timeout: 20_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "project-chat");
      await clickVisible(page, page.locator(".conv-head button").first());
      await page.locator(".sp-group").waitFor({ timeout: 10_000 });
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
        await clickVisible(page, page.locator(".sp-side .sp-side-add").nth(1));
        const dialog = page.locator(".app-modal-add");
        await dialog.waitFor();
        await dialog.locator(".ws-computer.active").waitFor({ timeout: 10_000 });
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
      await clickVisible(page, page.locator(".sp-head .app-btn", { hasText: "项目资料" }));
      await page.locator(".app-page").first().waitFor({ timeout: 20_000 });
      await settle(page);
      await shot(page, size, scheme, "project-notes");
      await clickVisible(page, page.getByRole("button", { name: "返回项目" }));
      await page.locator(".sp-group").waitFor({ timeout: 10_000 });
    });

    await step("还没进项目的", async () => {
      await go(page, "还没进项目的", narrow);
      await page.locator(".sp-group").waitFor({ timeout: 20_000 });
      const title = compactText(await page.locator(".sp-head h2").innerText());
      if (title !== "还没进项目的") throw new Error(`这里应该是还没进项目的 agent，实际标题「${title}」`);
      await settle(page);
      await shot(page, size, scheme, "loose");
    });

    await step("单聊 · 只回一句话", async () => {
      await openMember(page, narrow, "还没进项目的", "legacy-codex");
      const head = page.locator(".conv-detail .conv-head h2");
      const headText = compactText(await head.innerText());
      if (!/legacy-codex/.test(headText)) throw new Error(`点成员 legacy-codex 进入的不是它的对话：「${headText}」`);

      const body = "你好，收到请回复 收到";
      // The mock keeps its state across window combinations, so this
      // conversation may already hold earlier tasks with process blocks:
      // only blocks after this message count.
      const input = page.locator(".conv-compose-input");
      await input.fill(body);
      await input.press("Enter");
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
        return [...detail.querySelectorAll(".trace")].filter((t) => sent.compareDocumentPosition(t) & Node.DOCUMENT_POSITION_FOLLOWING).length;
      }, body);
      if (traces !== 0) throw new Error(`只回一句话的任务不应出现过程块，实际有 ${traces} 个 .trace`);
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-text-only");
    });

    await step("单聊 · 有工具调用", async () => {
      const body = "整理一下这次发布的主要变化，列成要点。";
      const input = page.locator(".conv-compose-input");
      await input.fill(body);
      await input.press("Enter");
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
      await openMember(page, narrow, "还没进项目的", "legacy-codex");
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
      await openMember(page, narrow, "还没进项目的", "legacy-codex");
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
