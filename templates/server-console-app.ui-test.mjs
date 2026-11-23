import { installSSHFixture } from "./server-console-fixture.mjs";
// The production bundle uses a strict bridge fixture. Real SSH output parsing
// and Hub approvals are additionally covered by Go's isolated sshd journeys.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const require = createRequire(
    path.join(
      execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(),
      "noop.js",
    ),
  ),
  { chromium } = require("playwright");
const root = path.resolve("internal/templates/official/server-console"),
  out = process.env.SERVER_SHOTS || "/tmp/server-console-app";
fs.mkdirSync(out, { recursive: true });
const server = http.createServer((req, res) => {
  const u = new URL(req.url, "http://x"),
    f = path.resolve(
      root,
      "." + (u.pathname === "/" ? "/index.html" : u.pathname),
    );
  if (!f.startsWith(root + path.sep) || !fs.existsSync(f)) {
    res.writeHead(404);
    res.end();
    return;
  }
  res.setHeader(
    "Content-Type",
    {
      ".html": "text/html",
      ".js": "text/javascript",
      ".css": "text/css",
      ".svg": "image/svg+xml",
      ".json": "application/json",
      ".webp": "image/webp",
    }[path.extname(f)] || "application/octet-stream",
  );
  res.end(fs.readFileSync(f));
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const browser = await chromium.launch();
try {
  for (const [lang, theme, width] of [
    ["zh", "light", 1200],
    ["en", "dark", 1200],
    ["zh", "dark", 760],
    ["en", "light", 393],
  ]) {
    const c = await browser.newContext({
      viewport: { width, height: 900 },
      colorScheme: theme,
    });
    await c.addInitScript(installSSHFixture, { lang, theme });
    const p = await c.newPage(),
      errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(
      "http://127.0.0.1:" + server.address().port + "/?myco_lang=" + lang,
    );
    const L = (zh, en) => (lang === "zh" ? zh : en);
    await p
      .getByRole("button", {
        name: L("添加 SSH 主机", "Add SSH host"),
        exact: true,
      })
      .click();
    await p
      .getByLabel(L("显示名称", "Display name"), { exact: true })
      .fill("Fixture Linux");
    await p
      .getByLabel(L("SSH 配置别名或 user@host", "SSH alias or user@host"), {
        exact: true,
      })
      .fill("fixture");
    await p
      .getByRole("button", {
        name: L("保存并连接", "Save and connect"),
        exact: true,
      })
      .click();
    await p.locator(".sv-metrics").waitFor();
    assert.ok(
      (await p.evaluate(() => document.documentElement.scrollWidth)) <= width,
    );
    if (lang === "en")
      assert.equal(
        /[\u4e00-\u9fff]/.test(await p.locator(".sv-app").innerText()),
        false,
      );
    assert.equal(
      await p.evaluate(() =>
        Object.hasOwn(
          window.calls.find((c) => c.args.op === "create").args,
          "port",
        ),
      ),
      false,
    );
    await p.screenshot({
      path: path.join(out, `${lang}-${theme}-${width}.png`),
      fullPage: true,
    });
    await p.locator(".sv-sidebar nav button").nth(1).click();
    await p
      .getByRole("button", {
        name: L("重启", "Restart") + " nginx.service",
        exact: true,
      })
      .click();
    await p.getByRole("dialog").waitFor();
    assert.equal(
      await p.evaluate(
        () =>
          window.calls.filter((c) => c.args.action === "services.restart")
            .length,
      ),
      0,
    );
    await p
      .getByRole("button", {
        name: L("确认重启", "Confirm Restart"),
        exact: true,
      })
      .click();
    await p.waitForFunction(() =>
      window.calls.some((c) => c.args.action === "services.restart"),
    );
    await p.waitForFunction(() => !document.querySelector("dialog"));
    await p
      .getByRole("button", {
        name: L("日志", "Logs") + " nginx.service",
        exact: true,
      })
      .click();
    await p.locator(".sv-log").waitFor();
    assert.ok((await p.locator(".sv-log").textContent()).includes("<img"));
    assert.equal(await p.evaluate(() => window.injected), undefined);
    await p
      .getByRole("button", { name: L("关闭", "Close"), exact: true })
      .click();
    await p.evaluate(() => (window.failMutation = true));
    await p
      .getByRole("button", {
        name: L("重启", "Restart") + " nginx.service",
        exact: true,
      })
      .click();
    await p
      .getByRole("button", {
        name: L("确认重启", "Confirm Restart"),
        exact: true,
      })
      .click();
    await p.getByRole("dialog").getByRole("alert").waitFor();
    await p
      .getByRole("button", { name: L("取消", "Cancel"), exact: true })
      .click();
    await p.locator(".sv-sidebar nav button").nth(3).click();
    await p
      .getByText(L("结果未知", "Outcome unknown"), { exact: true })
      .waitFor();
    assert.equal(
      await p.evaluate(
        () =>
          window.calls.filter((c) => c.args.action === "services.restart")
            .length,
      ),
      2,
    );
    await p.locator(".sv-sidebar nav button").nth(4).click();
    await p
      .getByRole("button", {
        name: L("为当前工作区启用 Agent", "Enable agents for this workspace"),
        exact: true,
      })
      .click();
    await p
      .getByRole("button", {
        name: L("关闭 Agent 访问", "Disable agent access"),
        exact: true,
      })
      .waitFor();
    await p.evaluate(() => (window.dockerDenied = true));
    await p
      .getByRole("button", { name: L("刷新", "Refresh"), exact: true })
      .click();
    await p.locator(".sv-sidebar nav button").nth(2).click();
    await p.getByText("Docker permission denied", { exact: true }).waitFor();
    assert.equal(await p.locator("tbody tr").count(), 0);
    assert.deepEqual(errors, []);
    await c.close();
    console.log(
      `PASS production UI ${lang}/${theme}/${width}: bound profiles, real-output format, mutation review, unknown outcomes, inert logs, agent opt-in and unavailable resources`,
    );
  }
  const c = await browser.newContext();
  await c.addInitScript(
    () =>
      (window.myco = {
        features: [],
        ready: Promise.resolve({ page: { id: "old" }, me: { name: "owner" } }),
        data: {
          list: async () => ({ records: [], seq: 0 }),
          on: () => () => {},
        },
      }),
  );
  const p = await c.newPage();
  await p.goto("http://127.0.0.1:" + server.address().port + "/?myco_lang=en");
  await p
    .getByText("Manage your servers in the desktop App", { exact: true })
    .waitFor();
  assert.equal(
    await p.locator(".sv-sidebar-heading button").isDisabled(),
    true,
  );
  await c.close();
  console.log("PASS legacy/mobile client has no fake SSH access");
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
