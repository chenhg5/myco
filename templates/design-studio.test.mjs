import assert from "node:assert/strict";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const here = path.dirname(fileURLToPath(import.meta.url)),
  root = path.resolve(here, "../internal/templates/official/design-studio"),
  out = path.join(here, "design-studio-shots");
fs.mkdirSync(out, { recursive: true });
const js = ts.transpileModule(
    fs.readFileSync(path.join(here, "src/design-studio/model.ts"), "utf8"),
    { compilerOptions: { module: ts.ModuleKind.ES2020 } },
  ).outputText,
  m = await import(
    "data:text/javascript;base64," + Buffer.from(js).toString("base64")
  );
for (const p of m.PRESETS) {
  const a = m.fresh(p.id, "<script>unsafe</script>");
  assert(a.frames[0].nodes.length);
  assert(!m.svg(a.frames[0]).includes("<script>"));
}
assert(!m.imageOK("https://secret"));
assert(!m.imageOK("data:image/svg+xml;base64,abcd"));
assert.equal(m.dimension(Infinity), 1080);
const sample = JSON.parse(
  fs.readFileSync(path.join(root, "sample.json"), "utf8"),
);
const srv = http.createServer((req, res) => {
  let p = decodeURIComponent(new URL(req.url, "http://x").pathname);
  if (p === "/") p = "/index.html";
  const f = path.resolve(root, "." + p);
  if (!f.startsWith(root + path.sep) || !fs.existsSync(f)) {
    res.writeHead(404);
    res.end();
    return;
  }
  res.setHeader(
    "Content-Type",
    { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" }[
      path.extname(f)
    ] || "application/octet-stream",
  );
  res.end(fs.readFileSync(f));
});
await new Promise((r) => srv.listen(0, "127.0.0.1", r));
let chromium;
try {
  ({ chromium } = createRequire(import.meta.url)("playwright"));
} catch {
  const globalRoot = execFileSync(
    process.platform === "win32" ? "npm.cmd" : "npm",
    ["root", "-g"],
    { encoding: "utf8" },
  ).trim();
  ({ chromium } = createRequire(path.join(globalRoot, "noop.js"))(
    "playwright",
  ));
}
const browser = await chromium.launch(
  process.env.UI_SMOKE_EXECUTABLE
    ? { executablePath: process.env.UI_SMOKE_EXECUTABLE }
    : {},
);

try {
  for (const [width, scheme] of [
    [1200, "light"],
    [1200, "dark"],
    [760, "light"],
    [760, "dark"],
  ]) {
    const ctx = await browser.newContext({
      viewport: { width, height: 850 },
      colorScheme: scheme,
    });
    await ctx.addInitScript((seed) => {
      if (window.top !== window) return;
      let records =
        JSON.parse(sessionStorage.getItem("records") || "null") ||
        Object.entries(seed).flatMap(([collection, rs]) =>
          rs.map(({ id, ...data }) => ({ collection, id, data, version: 1 })),
        );
      const persist = () =>
        sessionStorage.setItem("records", JSON.stringify(records));
      window.__downloads = [];
      window.__sent = [];
      window.__feed = [];
      window.__records = () => records;
      window.myco = {
        ready: Promise.resolve({
          page: { id: "pg_test" },
          me: { name: "Glenn" },
        }),
        data: {
          list: async (c) => ({
            records: structuredClone(records.filter((r) => r.collection === c)),
            seq: 0,
          }),
          put: async (c, data, o = {}) => {
            const id = o.id || Math.random().toString(36),
              old = records.find((r) => r.collection === c && r.id === id);
            if (old && o.ifVersion !== undefined && o.ifVersion !== old.version)
              throw { code: "conflict", latest: old };
            const next = {
              collection: c,
              id,
              data: structuredClone(data),
              version: (old?.version || 0) + 1,
            };
            records = records.filter(
              (r) => !(r.collection === c && r.id === id),
            );
            records.push(next);
            persist();
            return structuredClone(next);
          },
          remove: async (c, id) => {
            records = records.filter(
              (r) => !(r.collection === c && r.id === id),
            );
            persist();
            return true;
          },
          on: (f) => {
            window.__feed.push(f);
            return () => {};
          },
        },
        download: async (name, content) => {
          window.__downloads.push({
            name,
            data: content instanceof Blob ? await content.text() : content,
          });
          return { path: name };
        },
        send: async (s) => {
          window.__sent.push(s);
        },
      };
    }, sample);
    const page = await ctx.newPage(),
      errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    const b = (name) => page.getByRole("button", { name, exact: true });
    await page.goto(`http://127.0.0.1:${srv.address().port}/`);
    await page.locator(".ds-card").first().waitFor();
    await page.screenshot({
      path: path.join(out, `${width}-${scheme}-home.png`),
    });
    assert.equal(await page.locator(".ds-card").count(), 6);
    await page.getByLabel("搜索作品").fill("不存在");
    assert.equal(await page.locator(".ds-card").count(), 0);
    await b("清空搜索").first().click();
    await page.locator(".ds-card").filter({ hasText: "日常的形状" }).click();
    await page.locator(".ds-artboard").waitFor();
    if (width < 821) await b("显示属性").click();
    await page
      .locator(".ds-layer-main")
      .filter({ hasText: "把日常，" })
      .click();
    await page.getByLabel("图层文字").fill("设计，让想法生长。");
    await page.waitForFunction(() =>
      window
        .__records()
        .some(
          (r) =>
            r.collection === "artifacts" &&
            r.data.frames.some((f) =>
              f.nodes.some((n) => n.text === "设计，让想法生长。"),
            ),
        ),
    );
    // Real pointer movement must persist coordinates, not only move an overlay.
    if (width === 1200 && scheme === "light") {
      const hit = page.locator(".ds-node-hit.selected"),
        box = await hit.boundingBox();
      const before = await page.getByLabel("图层X").inputValue();
      await page.mouse.move(box.x + 20, box.y + 10);
      await page.mouse.down();
      await page.mouse.move(box.x + 50, box.y + 30, { steps: 4 });
      await page.mouse.up();
      assert(
        Number(await page.getByLabel("图层X").inputValue()) > Number(before),
      );
    }
    // An agent's feed event updates the open design without a reload.
    await page.evaluate(() => {
      const r = window
        .__records()
        .find((r) => r.collection === "artifacts" && r.id === "design-website");
      const data = structuredClone(r.data);
      data.frames[0].nodes.find((n) => n.text === "设计，让想法生长。").text =
        "来自 agent 的修改";
      r.data = data;
      r.version++;
      for (const f of window.__feed)
        f({
          seq: 1,
          collection: r.collection,
          record: r.id,
          op: "put",
          data,
          version: r.version,
          by: "agent",
        });
    });
    await page.waitForFunction(
      () =>
        document.querySelector('textarea[aria-label="图层文字"]')?.value ===
        "来自 agent 的修改",
    );
    await b("复制图层 Ctrl+D").click();
    await b("撤销 Ctrl+Z").click();
    await b("添加画布").click();
    await page.getByLabel("画布名称").fill("第二方案");
    if (width < 821) {
      await b("版本").click();
      await b("保存当前版本").click();
    } else await b("保存版本").click();
    await b("需求").click();
    await page.getByLabel("设计需求").fill("请优化首屏，保持黑白灰。");
    await b("交给 agent 继续设计").click();
    assert.equal(await page.evaluate(() => window.__sent.length), 1);
    await b("反馈").click();
    await page.getByLabel("反馈内容").fill("按钮更有层次");
    await b("添加反馈").click();
    await page
      .locator(".ds-comment")
      .filter({ hasText: "按钮更有层次" })
      .getByRole("button", { name: "标记已解决" })
      .click();
    await b("版本").click();
    await page
      .locator(".ds-version")
      .first()
      .getByRole("button", { name: "恢复" })
      .click();
    await b("确认").click();
    await b("属性").click();
    await page.screenshot({
      path: path.join(out, `${width}-${scheme}-editor.png`),
    });
    if (width < 821) await b("显示属性").click();
    for (const label of [
      "SVG · 当前画布",
      "PNG · 当前画布",
      "HTML · 完整作品",
      "JSON · 可编辑设计数据",
    ]) {
      await b("导出").click();
      await b(label).click();
      await page.waitForTimeout(200);
    }
    await page.waitForFunction(() => window.__downloads.length === 4);
    await b("HTML").click();
    await page
      .getByLabel("HTML 源码")
      .fill(
        '<h1>独立原型</h1><button onclick="this.textContent=\'已交互\'">试试交互</button><script>try{parent.document.title="BROKEN"}catch(e){}</script>',
      );
    await b("应用").click();
    await b("预览").click();
    await page
      .frameLocator("iframe")
      .getByRole("button", { name: "试试交互" })
      .click();
    await page
      .frameLocator("iframe")
      .getByRole("button", { name: "已交互" })
      .waitFor();
    assert.notEqual(await page.title(), "BROKEN");
    await page.reload();
    await page.locator(".ds-card").filter({ hasText: "日常的形状" }).click();
    await b("预览").click();
    await page
      .frameLocator("iframe")
      .getByRole("heading", { name: "独立原型" })
      .waitFor();
    await b("回到工作台").click();
    await b("新建设计").click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "海报", exact: true })
      .click();
    await page.getByLabel("新作品名称").fill("测试海报");
    await b("创建画布").click();
    await page.getByLabel("作品标题").waitFor();
    assert.equal(await page.getByLabel("作品标题").inputValue(), "测试海报");
    assert.deepEqual(errors, []);
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    );
    if (width === 1200) {
      await page.evaluate(() => sessionStorage.clear());
      await page.reload();
      await page.locator(".ds-card").first().waitFor();
      const shots = path.join(here, "src/design-studio/public/shots");
      fs.mkdirSync(shots, { recursive: true });
      const png = await page.screenshot();
      const webp = await page.evaluate(async (b64) => {
        const img = new Image();
        await new Promise((r) => {
          img.onload = r;
          img.src = "data:image/png;base64," + b64;
        });
        const c = document.createElement("canvas");
        c.width = img.width;
        c.height = img.height;
        c.getContext("2d").drawImage(img, 0, 0);
        return c.toDataURL("image/webp", 0.86).split(",")[1];
      }, png.toString("base64"));
      fs.writeFileSync(
        path.join(shots, scheme === "dark" ? "1.dark.webp" : "1.webp"),
        Buffer.from(webp, "base64"),
      );
    }
    console.log(
      `PASS ${width} ${scheme}: edit, undo, frames, snapshots, feedback, handoff, SVG/PNG/HTML/JSON export, isolated prototype, reload, creation`,
    );
    await ctx.close();
  }
} finally {
  await browser.close();
  await new Promise((r) => srv.close(r));
}
