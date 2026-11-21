// Built UI + bridge contract tests; no real proxy or system network changes.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "../internal/templates/official/proxy-console");
const out = path.join(here, "proxy-console-shots");
fs.mkdirSync(out, { recursive: true });
const server = http.createServer((req, res) => {
  const p = path.resolve(
    root,
    "." + (req.url === "/" ? "/index.html" : req.url),
  );
  if (!p.startsWith(root + path.sep) || !fs.existsSync(p)) {
    res.writeHead(404).end();
    return;
  }
  res.setHeader(
    "Content-Type",
    { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" }[
      path.extname(p)
    ] || "application/octet-stream",
  );
  res.end(fs.readFileSync(p));
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
let chromium;
try {
  ({ chromium } = createRequire(import.meta.url)("playwright"));
} catch {
  const global = execFileSync("npm", ["root", "-g"], {
    encoding: "utf8",
  }).trim();
  ({ chromium } = createRequire(path.join(global, "noop.js"))("playwright"));
}
const browser = await chromium.launch(
  process.env.UI_SMOKE_EXECUTABLE
    ? { executablePath: process.env.UI_SMOKE_EXECUTABLE }
    : {},
);
const url = `http://127.0.0.1:${server.address().port}`;
async function inject(context) {
  await context.addInitScript(() => {
    if (window.top !== window) return;
    window.__calls = [];
    window.__offline = false;
    window.__badSecret = false;
    window.__profile = null;
    window.__snapshot = {
      version: "test-1",
      at: new Date().toISOString(),
      config: {
        mode: "rule",
        "mixed-port": 7890,
        port: 0,
        "socks-port": 0,
        "allow-lan": false,
        "log-level": "info",
        tun: { enable: false },
      },
      proxies: {
        Choose: {
          name: "Choose",
          type: "Selector",
          now: "Node A",
          all: ["Node A", "Node B"],
        },
        "Node A": {
          name: "Node A",
          type: "Shadowsocks",
          history: [{ delay: 23 }],
        },
        "Node B": { name: "Node B", type: "Trojan", history: [] },
      },
      traffic: {
        downloadTotal: 4096,
        uploadTotal: 1024,
        connections: [
          {
            id: "test-connection",
            metadata: {
              network: "tcp",
              host: "example.com",
              destinationIP: "1.1.1.1",
              destinationPort: "443",
              process: "Safari",
            },
            download: 1024,
            upload: 128,
            start: new Date().toISOString(),
            chains: ["Node A", "Choose"],
            rule: "DomainSuffix",
          },
        ],
      },
      warnings: [],
    };
    window.myco = {
      ready: Promise.resolve({
        page: { id: "pg_proxy", name: "Proxy" },
        me: { name: "Tester" },
      }),
      features: ["os"],
      data: {
        list: async () => ({ records: [], seq: 0 }),
        on: () => () => {},
        put: async () => {
          throw Error("never share controller data");
        },
      },
      os: {
        call: async (name, args) => {
          window.__calls.push({ name, args });
          await new Promise((r) => setTimeout(r, 10));
          if (name === "proxy.profile") {
            if (args.action === "save") {
              window.__profile = {
                endpoint: args.endpoint,
                engine: args.engine,
                has_secret: !!args.secret,
              };
              return window.__profile;
            }
            if (args.action === "forget") {
              window.__profile = null;
              return true;
            }
            if (!window.__profile)
              throw Object.assign(Error("connect first"), {
                code: "not_found",
              });
            return window.__profile;
          }
          if (window.__offline)
            throw Error("cannot reach the local controller");
          if (window.__badSecret)
            throw Object.assign(Error("controller rejected the secret"), {
              code: "permission_denied",
            });
          if (args.action === "snapshot") {
            window.__snapshot.at = new Date().toISOString();
            window.__snapshot.traffic.downloadTotal += 2048;
            return structuredClone(window.__snapshot);
          }
          if (args.action === "select")
            window.__snapshot.proxies[args.group].now = args.name;
          if (args.action === "mode") window.__snapshot.config.mode = args.mode;
          if (args.action === "close")
            window.__snapshot.traffic.connections =
              window.__snapshot.traffic.connections.filter(
                (c) => c.id !== args.id,
              );
          return args.action === "delay" ? { delay: 42 } : true;
        },
      },
    };
  });
}
try {
  for (const [width, scheme] of [
    [1200, "light"],
    [1200, "dark"],
    [760, "light"],
    [760, "dark"],
  ]) {
    const context = await browser.newContext({
      viewport: { width, height: 1040 },
      colorScheme: scheme,
    });
    await inject(context);
    const page = await context.newPage(),
      errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto(url);
    await page.getByRole("button", { name: "先看示例" }).click();
    await page.getByText("演示数据", { exact: true }).waitFor();
    await page.screenshot({ path: path.join(out, `${width}-${scheme}.png`) });
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
      "no horizontal page overflow",
    );
    if (width === 1200) {
      const png = fs
        .readFileSync(path.join(out, `${width}-${scheme}.png`))
        .toString("base64");
      const webp = await page.evaluate(async (p) => {
        const i = new Image();
        i.src = "data:image/png;base64," + p;
        await i.decode();
        const c = document.createElement("canvas");
        c.width = i.width;
        c.height = i.height;
        c.getContext("2d").drawImage(i, 0, 0);
        return c.toDataURL("image/webp", 0.87).split(",")[1];
      }, png);
      const name = scheme === "dark" ? "1.dark.webp" : "1.webp";
      for (const dir of [
        path.join(root, "shots"),
        path.join(here, "src/proxy-console/public/shots"),
      ]) {
        fs.mkdirSync(dir, { recursive: true });
        fs.writeFileSync(path.join(dir, name), Buffer.from(webp, "base64"));
      }
    }
    await page.getByRole("button", { name: "策略与节点", exact: true }).click();
    assert.equal(await page.locator(".node-grid").count(), 3);
    assert.equal(
      await page.locator(".node-select:not([disabled])").count(),
      0,
      "demo never controls a real core",
    );
    await page.getByRole("button", { name: "连接", exact: true }).click();
    await page.getByRole("textbox", { name: "搜索连接" }).fill("figma");
    assert.equal(await page.locator("tbody tr").count(), 1);
    await page.getByRole("button", { name: "管理连接" }).click();
    await page.getByLabel("控制密钥").fill("private-controller-secret");
    await page.getByRole("button", { name: "保存并连接" }).click();
    await page.getByText("已连接", { exact: true }).waitFor();
    assert.equal(await page.getByText("private-controller-secret").count(), 0);
    await page.getByRole("button", { name: "概览", exact: true }).click();
    await page.getByRole("button", { name: "全局", exact: true }).click();
    await page
      .locator(".mode-switch .selected")
      .filter({ hasText: "全局" })
      .waitFor();
    await page.getByRole("button", { name: "策略与节点", exact: true }).click();
    await page.locator(".node-select").filter({ hasText: "Node B" }).click();
    await page.getByText("当前：Node B").waitFor();
    await page
      .getByRole("button", { name: "测速", exact: true })
      .first()
      .click();
    await page.getByRole("button", { name: "操作记录", exact: true }).click();
    await page.getByText("测速 Node A · 42 ms").waitFor();
    await page.getByRole("button", { name: "连接", exact: true }).click();
    await page.getByRole("textbox", { name: "搜索连接" }).fill("");
    await page.getByRole("button", { name: "断开", exact: true }).click();
    await page.getByText("当前没有活跃连接").waitFor();
    await page.evaluate(() => (window.__offline = true));
    await page
      .getByRole("alert")
      .filter({ hasText: "cannot reach" })
      .waitFor({ timeout: 6000 });
    assert.equal(await page.getByText("连接中断", { exact: true }).count(), 1);
    await page.evaluate(() => (window.__offline = false));
    await page.getByText("已连接", { exact: true }).waitFor({ timeout: 6000 });
    await page.getByRole("button", { name: "连接设置", exact: true }).click();
    assert.equal(await page.getByLabel("控制密钥").inputValue(), "");
    await page.getByRole("button", { name: "断开并忘记" }).click();
    await page.getByRole("button", { name: "先看示例" }).waitFor();
    assert.deepEqual(errors, []);
    await context.close();
    console.log(
      `PASS ${width} ${scheme}: preview, live connect, mode, select, delay, close, offline/recovery, forget`,
    );
  }
  // A missing native bridge remains usable for a harmless preview.
  const context = await browser.newContext();
  await context.addInitScript(() => {
    window.myco = {
      ready: Promise.resolve({ page: { id: "old" }, me: { name: "x" } }),
      data: { list: async () => ({ records: [], seq: 0 }), on: () => () => {} },
    };
  });
  const p = await context.newPage();
  await p.goto(url);
  await p
    .getByText("请更新 myco 桌面 App 后使用本机控制接口", { exact: false })
    .waitFor();
  await p.getByRole("button", { name: "先看示例" }).click();
  await p.getByText("演示数据", { exact: true }).waitFor();
  await context.close();
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
