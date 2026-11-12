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

/** Opens the picker's list on the conversations page (leaving an open
 *  conversation, which the page remembers until you leave it). */
async function openConversationList(page, narrow) {
  await go(page, "节点", narrow);
  await go(page, "对话", narrow);
  await clickVisible(page, page.getByRole("button", { name: /新对话|New conversation/ }));
  await page.getByRole("dialog").waitFor();
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
  // The stuck-task step stops and resends the demo task; the mock keeps
  // running, so put it back before each width and theme.
  const reset = await fetch(`http://${HOST}:${MOCK_PORT}/app/mock/stuck`, { method: "POST" });
  if (!reset.ok) throw new Error(`重置「可能卡住」的演示任务失败：HTTP ${reset.status}`);
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
  // Unique per matrix: the mock keeps running across all four.
  const alias = `smoke-${size.width}-${scheme}`;
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

    // A new conversation: search narrows the node list, picking one opens its
    // chat, and the first message lands there. A task nobody picked up yet
    // says so in a line, with no spinner.
    await step("对话 · 新对话搜索", async () => {
      await openConversationList(page, narrow);
      const search = page.getByRole("dialog").locator(".app-convpick-input");
      await search.fill("Review");
      let onlyOne = true;
      try {
        await page.waitForFunction(
          () => document.querySelectorAll(".app-convpick-list .app-row").length === 1,
          null,
          { timeout: 5000 },
        );
      } catch {
        onlyOne = false;
      }
      const picks = page.locator(".app-convpick-list .app-row");
      const count = await picks.count();
      if (!onlyOne) throw new Error(`搜索「Review」后列表应只剩 1 个节点，实际 ${count} 个`);
      const title = compactText(await picks.first().innerText());
      if (!/Review/.test(title)) throw new Error(`搜索「Review」找到的不是 Review 节点：「${title}」`);
      await settle(page, 400);
      await shot(page, size, scheme, "dialog-new-conversation-review");
      await picks.first().click();
      await page.getByRole("dialog").waitFor({ state: "hidden" });
      const head = page.locator(".conv-detail .conv-head h2");
      await head.waitFor({ timeout: 20_000 });
      const headText = compactText(await head.innerText());
      if (!/Review/.test(headText)) throw new Error(`点选节点后应直接进入它的对话，标题是「${headText}」`);
    });

    await step("对话 · 新对话第一条消息", async () => {
      const body = "帮我看一下这个分支的改动。";
      const input = page.locator(".conv-compose-input");
      await input.waitFor({ timeout: 20_000 });
      // Start watching before sending: the queue line is replaced by the first
      // step a few seconds in, so check for it (and no spinner) as it appears.
      const waitingOk = page.waitForFunction(
        () => {
          const detail = document.querySelector(".conv-detail");
          const line = detail?.querySelector(".trace.is-waiting");
          return !!line
            && (line.textContent || "").includes("排队中")
            && detail.querySelectorAll(".trace-spin").length === 0;
        },
        null,
        { timeout: 20_000 },
      ).then(() => true, () => false);
      await input.fill(body);
      await input.press("Enter");
      await page.locator(".conv-msg", { hasText: body }).last().waitFor({ timeout: 20_000 });
      if (!(await waitingOk)) {
        const spin = await page.locator(".conv-detail .trace-spin").count();
        if (spin > 0) throw new Error(`排队中的任务不应转圈，实际有 ${spin} 个 .trace-spin`);
        throw new Error("还没开始的任务应显示一行「排队中…」的说明，没有找到 .trace.is-waiting");
      }
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-first-message");
    });

    // PM (Codex) has session_in_use in the mock: its node row and chat say
    // the session is busy, the input is disabled, and a copy can talk.
    await step("节点 · 会话被占用", async () => {
      await go(page, "节点", narrow);
      const pm = page.locator(".app-page .app-row:visible", { hasText: "PM (Codex)" }).first();
      await pm.waitFor({ timeout: 20_000 });
      const text = compactText(await pm.innerText());
      if (!text.includes("会话被占用")) throw new Error(`PM (Codex) 节点应显示「会话被占用」，实际是「${text}」`);
      await pm.click();
      await page.locator(".app-inspector").waitFor({ timeout: 20_000 });
      await settle(page, 300);
      await shot(page, size, scheme, "node-session-in-use");
    });

    await step("对话 · 会话被占用", async () => {
      const inspector = page.locator(".app-inspector");
      await clickVisible(page, inspector.getByRole("button", { name: /发消息|Message/ }));
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
      await page.waitForFunction(
        () => (document.querySelector(".conv-detail .conv-head-state")?.textContent || "").includes("会话被占用"),
        null,
        { timeout: 10_000 },
      );
      const input = page.locator(".conv-compose-input");
      if (!(await input.isDisabled())) throw new Error("会话被占用时输入框应禁用，实际可以输入");
      const fork = page.locator(".conv-busy button", { hasText: "复制一份来对话" });
      if (!(await fork.first().isVisible().catch(() => false))) {
        throw new Error("会话被占用时，输入框上方应有「复制一份来对话」按钮");
      }
      await settle(page, 300);
      await shot(page, size, scheme, "conversation-session-in-use");
    });

    // An inbox node (any agent joined from a session) is named after what it
    // is, and its details say it receives only in that session: no runner
    // wake, no copy.
    await step("节点 · 会话内接收", async () => {
      await go(page, "节点", narrow);
      const inbox = page.locator(".app-page .app-row:visible", { hasText: "会话内接收" }).first();
      await inbox.waitFor({ timeout: 20_000 });
      const rowText = compactText(await inbox.innerText());
      if (!rowText.includes("Agent · Studio Mac") || !rowText.includes("myco")) {
        throw new Error(`生成名的 inbox 节点应显示「Agent · 机器 · 目录」，实际是「${rowText}」`);
      }
      await inbox.click();
      const inspector = page.locator(".app-inspector");
      await inspector.waitFor({ timeout: 20_000 });
      const receive = inspector.locator(".app-kv", { hasText: "接收方式" });
      await receive.waitFor({ timeout: 10_000 });
      const receiveText = compactText(await receive.innerText());
      if (!receiveText.includes("只能在会话里接收") || !receiveText.includes("不能后台唤醒")) {
        throw new Error(`inbox 节点的接收方式应说明只能在会话里接收、不能后台唤醒，实际是「${receiveText}」`);
      }
      const copies = await inspector.locator(".app-action-row", { hasText: "复制一个" }).count();
      if (copies > 0) throw new Error("会话内接收的节点不应提供「复制一个」");
      await settle(page, 300);
      await shot(page, size, scheme, "node-inbox");
      await page.keyboard.press("Escape");
      await inspector.waitFor({ state: "hidden" });
    });

    // Aliases are addresses others can message a node by. A node owner adds
    // one inline in the details, and the node lists find the node by it.
    await step("节点 · 别名", async () => {
      await go(page, "节点", narrow);
      const pm = page.locator(".app-page .app-row:visible", { hasText: "PM (Codex)" }).first();
      await pm.waitFor({ timeout: 20_000 });
      const rowText = compactText(await pm.innerText());
      if (!rowText.includes("@pm")) throw new Error(`PM (Codex) 的副标题应带上别名 @pm，实际是「${rowText}」`);
      await pm.click();
      const inspector = page.locator(".app-inspector");
      await inspector.waitFor({ timeout: 20_000 });
      const aliasRow = inspector.locator(".app-kv", { hasText: "别名" });
      await aliasRow.waitFor({ timeout: 10_000 });
      const aliasText = compactText(await aliasRow.innerText());
      if (!aliasText.includes("@pm")) throw new Error(`节点详情应列出别名 @pm，实际是「${aliasText}」`);
      if (!aliasText.includes("别人可以用别名给它发消息")) {
        throw new Error(`别名一行应说明别人可以用别名发消息，实际是「${aliasText}」`);
      }
      await clickVisible(page, aliasRow.getByRole("button", { name: /添加|^Add$/ }));
      const input = aliasRow.locator(".app-alias-form input");
      await input.fill(alias);
      await input.press("Enter");
      await page.waitForFunction(
        (a) => (document.querySelector(".app-inspector")?.textContent || "").includes("@" + a),
        alias,
        { timeout: 10_000 },
      );
      await settle(page, 300);
      await shot(page, size, scheme, "node-aliases");
      await page.keyboard.press("Escape");
      await inspector.waitFor({ state: "hidden" });

      const search = page.locator(".app-page .app-search input");
      await search.fill(alias);
      await page.waitForFunction(
        () => document.querySelectorAll(".app-page .app-row").length === 1,
        null,
        { timeout: 5000 },
      );
      const found = compactText(await page.locator(".app-page .app-row").first().innerText());
      if (!found.includes("PM (Codex)")) throw new Error(`按别名「${alias}」搜索应找到 PM (Codex)，实际是「${found}」`);
      await settle(page, 300);
      await shot(page, size, scheme, "nodes-alias-search");
      await search.fill("");
    });

    await step("对话 · 别名搜索", async () => {
      await openConversationList(page, narrow);
      const search = page.getByRole("dialog").locator(".app-convpick-input");
      await search.fill(alias);
      await page.waitForFunction(
        () => document.querySelectorAll(".app-convpick-list .app-row").length === 1,
        null,
        { timeout: 5000 },
      );
      const found = compactText(await page.locator(".app-convpick-list .app-row").first().innerText());
      if (!found.includes("PM (Codex)")) throw new Error(`新对话里按别名「${alias}」搜索应找到 PM (Codex)，实际是「${found}」`);
      await settle(page, 300);
      await shot(page, size, scheme, "dialog-new-conversation-alias");
      await closeDialog(page);
    });

    // A run that only says something leaves no process block; a run that
    // called tools keeps one.
    await step("对话 · 只回一句话", async () => {
      await openConversationList(page, narrow);
      const search = page.getByRole("dialog").locator(".app-convpick-input");
      await search.fill("legacy");
      await page.waitForFunction(
        () => document.querySelectorAll(".app-convpick-list .app-row").length === 1,
        null,
        { timeout: 5000 },
      ).catch(() => undefined);
      const picks = page.locator(".app-convpick-list .app-row");
      const count = await picks.count();
      if (count !== 1) throw new Error(`搜索「legacy」后列表应只剩 1 个节点，实际 ${count} 个`);
      await picks.first().click();
      await page.getByRole("dialog").waitFor({ state: "hidden" });
      const head = page.locator(".conv-detail .conv-head h2");
      await head.waitFor({ timeout: 20_000 });
      const headText = compactText(await head.innerText());
      if (!/legacy-codex/.test(headText)) throw new Error(`搜索「legacy」后进入的不是 legacy-codex：「${headText}」`);

      const body = "你好，收到请回复 收到";
      // The mock keeps its state across window combinations, so this
      // conversation may already hold earlier tasks with process blocks:
      // only blocks after this message count.
      const input = page.locator(".conv-compose-input");
      await input.fill(body);
      await input.press("Enter");
      await page.locator(".conv-msg", { hasText: body }).last().waitFor({ timeout: 20_000 });
      // The header speaks for the whole conversation (it can say 可能卡住 for
      // an older task here), so wait for this task instead: the mock replies
      // 6 s after it, then its final trace is read.
      await page.waitForTimeout(7_500);
      await page.waitForTimeout(600);
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

    await step("对话 · 有工具调用", async () => {
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
    await step("对话 · 可能卡住", async () => {
      await go(page, "对话", narrow);
      if (narrow) {
        const back = page.locator(".conv-head button").first();
        if (await back.isVisible().catch(() => false)) await back.click();
      }
      const row = page.locator(".conv-row:visible", { hasText: "legacy-codex" }).first();
      await row.waitFor({ timeout: 20_000 });
      const title = await row.getAttribute("title");
      if (!(title || "").includes("可能卡住")) {
        throw new Error(`可能卡住的任务应在列表里标出来，实际提示是「${title}」`);
      }
      if (await row.locator(".conv-row-state.is-stuck").count() === 0) {
        throw new Error("可能卡住的任务在列表里应有单独的标记");
      }
      await row.click();
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
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
    await step("对话 · 更早的卡住任务", async () => {
      await go(page, "对话", narrow);
      if (narrow) {
        const back = page.locator(".conv-head button").first();
        if (await back.isVisible().catch(() => false)) await back.click();
      }
      // The same conversation as the previous step (one per node); its
      // preview now shows the task sent again there.
      const row = page.locator(".conv-row:visible", { hasText: "legacy-codex" }).first();
      await row.waitFor({ timeout: 20_000 });
      await row.click();
      await page.locator(".conv-detail").waitFor({ timeout: 20_000 });
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
