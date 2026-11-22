// Real clipboard, pointer and touch events against the built official page.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
const here = path.dirname(fileURLToPath(import.meta.url)),
  root = path.resolve(here, "../internal/templates/official/whiteboard"),
  out = process.env.WHITEBOARD_SHOTS || "/tmp/whiteboard-shots";
fs.mkdirSync(out, { recursive: true });
const fixture = {
  board: [{ id: "main", title: "Project canvas" }],
  items: [
    {
      id: "a",
      type: "rect",
      text: "Research",
      x: 200,
      y: 200,
      w: 200,
      h: 100,
      z: 1,
    },
    {
      id: "b",
      type: "rect",
      text: "Prototype",
      x: 520,
      y: 200,
      w: 200,
      h: 100,
      z: 2,
    },
    {
      id: "c",
      type: "sticky",
      text: "Validate ideas",
      x: 200,
      y: 400,
      w: 200,
      h: 140,
      z: 3,
    },
    { id: "ab", type: "arrow", from: "a", to: "b", text: "Next" },
  ],
};
const bridgeSource = fs.readFileSync(path.join(here, "shots.mjs"), "utf8");
const bridge = bridgeSource.slice(
  bridgeSource.indexOf("const bridge = `") + 16,
  bridgeSource.indexOf("})();`;") + 5,
);
const server = http.createServer((req, res) => {
  const pathname = new URL(req.url, "http://test").pathname;
  if (pathname === "/bridge.js") {
    res.setHeader("Content-Type", "text/javascript");
    res.end(bridge);
    return;
  }
  if (pathname === "/favicon.ico") {
    res.writeHead(204);
    res.end();
    return;
  }
  const f = path.resolve(
    root,
    "." + (pathname === "/" ? "/index.html" : pathname),
  );
  if (!f.startsWith(root + path.sep) || !fs.existsSync(f)) {
    res.writeHead(404);
    res.end();
    return;
  }
  const types = {
    ".html": "text/html",
    ".js": "text/javascript",
    ".css": "text/css",
    ".svg": "image/svg+xml",
    ".json": "application/json",
    ".webp": "image/webp",
  };
  res.setHeader(
    "Content-Type",
    types[path.extname(f)] || "application/octet-stream",
  );
  let body = fs.readFileSync(f);
  if (f.endsWith(".html"))
    body = Buffer.from(
      String(body).replace(
        "<head>",
        '<head><script src="/bridge.js"></script>',
      ),
    );
  res.end(body);
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const playwright = createRequire(
  path.join(
    execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(),
    "noop.js",
  ),
)("playwright");
const browser = await playwright.chromium.launch({
  headless: true,
  executablePath: process.env.UI_SMOKE_EXECUTABLE,
  args: ["--no-sandbox"],
});
try {
  for (const [lang, theme, width, phone] of [
    ["zh", "light", 1400],
    ["en", "dark", 1040],
    ["en", "light", 760],
    ["zh", "dark", 393, true],
  ]) {
    const context = await browser.newContext({
      viewport: { width, height: 900 },
      colorScheme: theme,
      isMobile: !!phone,
      hasTouch: !!phone,
      permissions: ["clipboard-read", "clipboard-write"],
    });
    const p = await context.newPage(),
      errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    p.setDefaultTimeout(8000);
    await p.addInitScript((f) => {
      window.__SAMPLE__ = f;
      window.__downloads = [];
    }, fixture);
    const base = `http://127.0.0.1:${server.address().port}/?myco_lang=${lang}`;
    const L = (zh, en) => (lang === "zh" ? zh : en);
    const records = () =>
      p.evaluate(async () => (await myco.data.list("items")).records);
    const load = async () => {
      await p.goto(base);
      await p.locator("[data-id=a].wb-item").waitFor();
    };
    const clickNode = async (id) =>
      p.locator(`.wb-item[data-id="${id}"] .wb-text`).click();
    const drag = async (a, b) => {
      await p.mouse.move(a.x, a.y);
      await p.mouse.down();
      await p.mouse.move(b.x, b.y, { steps: 15 });
      await p.mouse.up();
    };
    await load();
    assert.equal(await p.locator(".wb-item").count(), 3);
    assert((await p.locator(".wb-line").getAttribute("d")).includes("L"));
    await clickNode("a");
    await p.keyboard.press("Control+d");
    assert.equal((await records()).length, 5);
    await p.keyboard.press("Control+z");
    assert.equal((await records()).length, 4);
    await p.keyboard.press("Control+Shift+z");
    assert.equal((await records()).length, 5);
    await p.keyboard.press("Control+z");
    await p.keyboard.press("Control+a");
    await p.keyboard.press("Control+c");
    await p.keyboard.press("Control+v");
    await p.waitForFunction(
      async () => (await myco.data.list("items")).records.length === 8,
    );
    let rows = await records(),
      clones = rows.filter(
        (r) => !fixture.items.some((old) => old.id === r.id),
      ),
      copiedLine = clones.find((r) => r.data.type === "arrow");
    assert(
      clones.some((r) => r.id === copiedLine.data.from) &&
        clones.some((r) => r.id === copiedLine.data.to),
    );
    await p.keyboard.press("Control+x");
    assert.equal((await records()).length, 4);
    await p.keyboard.press("Control+z");
    assert.equal((await records()).length, 8);
    await p.keyboard.press("Control+z");
    assert.equal((await records()).length, 4);
    await p.evaluate(() =>
      navigator.clipboard.writeText("Pasted outside\nSecond line"),
    );
    await p.keyboard.press("Control+v");
    await p.getByText("Pasted outside\nSecond line", { exact: true }).waitFor();
    assert.equal(
      (await records()).filter((r) => r.data.type === "sticky").length,
      2,
    );
    await p.keyboard.press("Control+z");
    await p.keyboard.press("Escape");
    await clickNode("a");
    await p
      .getByRole("button", { name: L("复制", "Copy"), exact: true })
      .click();
    await p
      .getByRole("button", { name: L("粘贴", "Paste"), exact: true })
      .click();
    await p.waitForFunction(
      () => document.querySelectorAll(".wb-item").length === 4,
    );
    await p.keyboard.press("Control+z");
    await p.waitForFunction(
      () => document.querySelectorAll(".wb-item").length === 3,
    );
    await p.keyboard.press("Escape");
    if (width === 1400) {
      await clickNode("a");
      await p.evaluate(() =>
        Object.defineProperty(navigator, "clipboard", {
          value: undefined,
          configurable: true,
        }),
      );
      await p.getByRole("button", { name: "复制", exact: true }).click();
      await p.getByRole("button", { name: "粘贴", exact: true }).click();
      await p.getByRole("textbox", { name: "在此使用系统粘贴" }).waitFor();
      await p.keyboard.press("Control+v");
      await p.waitForFunction(
        () => document.querySelectorAll(".wb-item").length === 4,
      );
      assert.equal(await p.locator(".wb-paste-prompt").count(), 0);
      await p.keyboard.press("Control+z");
      await p.waitForFunction(
        () => document.querySelectorAll(".wb-item").length === 3,
      );
      await p.evaluate(() => delete navigator.clipboard);
      await p.keyboard.press("Escape");
    }
    const lineBox = await p.locator(".wb-link-hit").boundingBox();
    await p.mouse.click(
      lineBox.x + lineBox.width / 2,
      lineBox.y + lineBox.height / 2,
    );
    await p
      .getByRole("combobox", { name: L("连线类型", "Connector type") })
      .selectOption("curve");
    assert((await p.locator(".wb-line").getAttribute("d")).includes("C"));
    const label = p.getByRole("textbox", {
      name: L("连线文字", "Connector label"),
    });
    await label.fill("Approved");
    await label.press("Enter");
    assert.equal(
      (await records()).find((r) => r.id === "ab").data.text,
      "Approved",
    );
    await p.keyboard.press("Escape");
    await clickNode("a");
    const before = (await records()).find((r) => r.id === "a").data;
    let a = await p.locator(".wb-item[data-id=a]").boundingBox();
    const oldPath = await p.locator(".wb-line").getAttribute("d");
    await drag(
      { x: a.x + a.width / 2, y: a.y + a.height / 2 },
      { x: a.x + a.width / 2 + 60, y: a.y + a.height / 2 + 70 },
    );
    assert.notEqual(await p.locator(".wb-line").getAttribute("d"), oldPath);
    await p.keyboard.press("Control+z");
    assert.equal((await records()).find((r) => r.id === "a").data.x, before.x);
    await clickNode("a");
    const port = await p
        .locator(".wb-item[data-id=a] .wb-port.is-right")
        .boundingBox(),
      b = await p.locator(".wb-item[data-id=b]").boundingBox();
    await drag(
      { x: port.x + port.width / 2, y: port.y + port.height / 2 },
      { x: b.x, y: b.y + b.height / 2 },
    );
    rows = await records();
    assert.equal(rows.filter((r) => r.data.type === "arrow").length, 2);
    const joined = rows.find((r) => r.id !== "ab" && r.data.type === "arrow");
    assert.equal(joined.data.from, "a");
    assert.equal(joined.data.to, "b");
    // Drag the selected connector's endpoint to another node.
    const end = await p.locator(".wb-link-end").last().boundingBox();
    const target = await p.locator(".wb-item[data-id=c]").boundingBox();
    await drag(
      { x: end.x + end.width / 2 - 3, y: end.y + end.height / 2 },
      { x: target.x + target.width / 2, y: target.y },
    );
    assert.equal(
      (await records()).find((r) => r.id === joined.id).data.to,
      "c",
    );
    await p.keyboard.press("Control+z");
    assert.equal(
      (await records()).find((r) => r.id === joined.id).data.to,
      "b",
    );
    await p.keyboard.press("Control+z");
    await clickNode("c");
    await p
      .getByRole("button", { name: L("锁定", "Lock"), exact: true })
      .click();
    a = await p.locator(".wb-item[data-id=c]").boundingBox();
    const locked = (await records()).find((r) => r.id === "c").data;
    await drag(
      { x: a.x + a.width / 2, y: a.y + a.height / 2 },
      { x: a.x + a.width / 2 + 50, y: a.y + a.height / 2 + 50 },
    );
    assert.deepEqual((await records()).find((r) => r.id === "c").data, locked);
    await p
      .getByRole("button", { name: L("解锁", "Unlock"), exact: true })
      .click();
    await p.keyboard.press("Escape");
    await p.keyboard.press("Control+a");
    const arrange = p.getByRole("combobox", {
      name: L("排列元素", "Arrange elements"),
    });
    await arrange.selectOption("left");
    rows = await records();
    assert.equal(
      new Set(rows.filter((r) => r.data.type !== "arrow").map((r) => r.data.x))
        .size,
      1,
    );
    await p.keyboard.press("Control+z");
    await p.keyboard.press("Escape");
    await clickNode("a");
    await p.keyboard.press("Delete");
    rows = await records();
    assert(!rows.some((r) => r.id === "a" || r.id === "ab"));
    await p.keyboard.press("Control+z");
    assert.equal((await records()).length, 4, JSON.stringify(await records()));
    await p.keyboard.press("Escape");
    const boxes = await p.locator(".wb-item").evaluateAll((es) =>
      es.map((e) => {
        const r = e.getBoundingClientRect();
        return { x: r.left, y: r.top, w: r.width, h: r.height };
      }),
    );
    await drag(
      {
        x: Math.min(...boxes.map((b) => b.x)) - 14,
        y: Math.min(...boxes.map((b) => b.y)) - 14,
      },
      {
        x: Math.max(...boxes.map((b) => b.x + b.w)) + 14,
        y: Math.max(...boxes.map((b) => b.y + b.h)) + 14,
      },
    );
    assert.equal(
      await p.locator(".wb-item.is-on").count(),
      3,
      "marquee selects multiple elements",
    );
    await p.keyboard.press("Escape");
    await clickNode("a");
    a = await p.locator(".wb-item[data-id=a]").boundingBox();
    await p.mouse.move(a.x + a.width / 2, a.y + a.height / 2);
    await p.mouse.down();
    await p.mouse.move(
      a.x + a.width / 2 + (4 * a.width) / 200,
      a.y + a.height / 2 + (60 * a.width) / 200,
      { steps: 12 },
    );
    await p.locator(".wb-guide.is-x").waitFor();
    await p.mouse.up();
    assert.equal(
      (await records()).find((r) => r.id === "a").data.x,
      200,
      "nearby edges snap while dragging",
    );
    await p.keyboard.press("Control+z");
    await p.keyboard.press("Escape");
    if (width === 1400) {
      await clickNode("a");
      const start = await p
        .locator(".wb-item[data-id=a] .wb-port.is-right")
        .boundingBox();
      await drag(
        { x: start.x + start.width / 2, y: start.y + start.height / 2 },
        { x: width - 120, y: 650 },
      );
      await p.locator(".wb-text.is-editing").waitFor();
      assert.equal(
        await p.locator(".wb-item").count(),
        4,
        "drop on empty space creates a connected node",
      );
      assert.equal(await p.locator(".wb-connector").count(), 2);
      await p.keyboard.press("Escape");
      await p.keyboard.press("Control+z");
      await p.waitForFunction(
        () =>
          document.querySelectorAll(".wb-item").length === 3 &&
          document.querySelectorAll(".wb-connector").length === 1,
      );
      const text = p.locator(".wb-item[data-id=a] .wb-text");
      await text.dblclick();
      const editor = p.locator(".wb-text.is-editing");
      await editor.fill("Cancelled text");
      await editor.press("Escape");
      assert.equal(
        (await records()).find((r) => r.id === "a").data.text,
        "Research",
        "Escape cancels text changes",
      );
    }
    await p.evaluate(() => {
      myco.download = async (name, content, type) => {
        window.__export = { name, content, type };
        return { path: "Downloads/" + name };
      };
    });
    await p
      .getByRole("button", { name: L("导出 SVG", "Export SVG"), exact: true })
      .click();
    const exported = await p.evaluate(() => window.__export);
    assert.equal(exported.type, "image/svg+xml");
    assert(exported.content.includes("Approved"));
    assert(exported.content.includes("Research"));
    await p.keyboard.press("Escape");
    await p.screenshot({
      path: path.join(out, `${lang}-${theme}-${width}.png`),
    });
    if (phone) {
      const cdp = await context.newCDPSession(p),
        old = await p.locator(".wb-world").getAttribute("style");
      const touch = (type, points) =>
        cdp.send("Input.dispatchTouchEvent", { type, touchPoints: points });
      await clickNode("a");
      const props = await p.locator(".wb-props").boundingBox();
      await touch("touchStart", [
        { x: props.x + props.width - 20, y: props.y + props.height - 4, id: 1 },
      ]);
      await touch("touchMove", [
        { x: props.x + 30, y: props.y + props.height - 4, id: 1 },
      ]);
      await touch("touchEnd", []);
      await p.waitForFunction(
        () => document.querySelector(".wb-props").scrollLeft > 20,
      );
      await p.keyboard.press("Escape");
      await touch("touchStart", [
        { x: 80, y: 320, id: 1 },
        { x: 250, y: 320, id: 2 },
      ]);
      await touch("touchMove", [
        { x: 55, y: 340, id: 1 },
        { x: 285, y: 340, id: 2 },
      ]);
      await touch("touchEnd", []);
      assert.notEqual(await p.locator(".wb-world").getAttribute("style"), old);
      await cdp.detach();
      await p.getByRole("button", { name: "便签", exact: true }).tap();
      await p.touchscreen.tap(180, 650);
      await p.locator(".wb-text.is-editing").waitFor();
      assert.equal(
        (await records()).length,
        5,
        "a phone tool tap creates a note",
      );
      await p.keyboard.press("Escape");
      await p.keyboard.press("Control+z");
    }
    assert.equal(
      await p.evaluate(() => document.documentElement.scrollWidth > innerWidth),
      false,
    );
    assert.deepEqual(errors, []);
    await context.close();
    console.log(
      `PASS ${lang}/${theme}/${width}: real copy/cut/paste, graph remapping, grouped undo, labels/routes, attached connectors, lock, alignment, delete/restore${phone ? ", two-finger canvas gestures" : ""}`,
    );
  }
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
