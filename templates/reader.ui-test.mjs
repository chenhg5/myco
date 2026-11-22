// Real production bundle + browser storage, typography and PDF worker. No
// network, desktop OS bridge or mock book content inside the application.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join, extname, resolve } from "node:path";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const require = createRequire(
  join(
    execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(),
    "reader-test.js",
  ),
);
const { chromium } = require("playwright");
const root = resolve("internal/templates/official/reader");
const bridge = `<script>window.__writes=0;window.__downloads=[];const params=new URLSearchParams(location.search);document.documentElement.dataset.mycoTheme=params.get('theme')||'light';const prefListeners=new Set();window.__setPreferences=(language,theme)=>{window.myco.preferences={language,theme};document.documentElement.dataset.mycoTheme=theme;for(const f of prefListeners)f({language,theme});};window.myco={on:(event,callback)=>{prefListeners.add(callback);return()=>prefListeners.delete(callback);},preferences:{language:params.get('myco_lang')||'zh',theme:params.get('theme')||'light'},features:['preferences'],ready:Promise.resolve({page:{id:params.get('page')||'reader-test'},me:{name:params.get('member')||'reader'}}),data:{put:async()=>{window.__writes++;throw Error('no shared writes')},list:async()=>({records:[],seq:0}),on:()=>()=>{}},download:async(name,file)=>{window.__downloads.push({name,bytes:await file.arrayBuffer()});return{path:name}}};</script>`;
const requested = [];
const types = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".bcmap": "application/octet-stream",
  ".pfb": "application/octet-stream",
  ".ttf": "font/ttf",
  ".svg": "image/svg+xml",
};
const server = createServer((req, res) => {
  const pathname = new URL(req.url, "http://local").pathname;
  requested.push(pathname);
  const file = resolve(
    root,
    "." + (pathname === "/" ? "/index.html" : pathname),
  );
  if (!file.startsWith(root + "/") || !existsSync(file)) {
    res.writeHead(404);
    res.end();
    return;
  }
  let body = readFileSync(file);
  if (file.endsWith(".html"))
    body = Buffer.from(body.toString().replace("<head>", "<head>" + bridge));
  res.writeHead(200, {
    "Content-Type": types[extname(file)] || "application/octet-stream",
    "Content-Security-Policy": csp,
  });
  res.end(body);
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const url = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ headless: true });
const artifact = resolve(
  process.env.READER_ARTIFACTS || "templates/reader-shots",
);
mkdirSync(artifact, { recursive: true });
const errors = [];
const source = readFileSync("internal/console/pages.go", "utf8");
const csp = source
  .match(/const pageCSP = ([\s\S]*?)\n\n/)[1]
  .match(/"[^"]*"/g)
  .map((s) => JSON.parse(s))
  .join("");
function pdfFixture() {
  const objects = [
    "<< /Type /Catalog /Pages 2 0 R >>",
    "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 420 595] /Resources << /Font << /F1 5 0 R >> >> /Contents 6 0 R >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 420 595] /Resources << /Font << /F1 5 0 R >> >> /Contents 7 0 R >>",
    "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    ...["Reader PDF test", "Page two last stop"].map((s) => {
      const data = `BT /F1 24 Tf 40 510 Td (${s}) Tj ET\n`;
      return `<< /Length ${data.length} >>\nstream\n${data}endstream`;
    }),
  ];
  let body = "%PDF-1.4\n",
    offsets = [0];
  for (const [i, o] of objects.entries()) {
    offsets.push(Buffer.byteLength(body));
    body += `${i + 1} 0 obj\n${o}\nendobj\n`;
  }
  const xref = Buffer.byteLength(body);
  body +=
    `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n` +
    offsets
      .slice(1)
      .map((o) => `${String(o).padStart(10, "0")} 00000 n \n`)
      .join("") +
    `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF`;
  return Buffer.from(body);
}
function longPDF(count = 80) {
  const font = count + 3,
    contents = count + 4;
  const objects = [
    "<< /Type /Catalog /Pages 2 0 R >>",
    `<< /Type /Pages /Kids [${Array.from({ length: count }, (_, i) => `${i + 3} 0 R`).join(" ")}] /Count ${count} >>`,
    ...Array.from(
      { length: count },
      (_, i) =>
        `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 420 ${i % 3 ? 595 : 820}] /Resources << /Font << /F1 ${font} 0 R >> >> /Contents ${contents + i} 0 R >>`,
    ),
    "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    ...Array.from({ length: count }, (_, i) => {
      const data = `BT /F1 24 Tf 40 510 Td (Page ${i + 1} of ${count}) Tj ET\n`;
      return `<< /Length ${data.length} >>\nstream\n${data}endstream`;
    }),
  ];
  let body = "%PDF-1.4\n",
    offsets = [0];
  for (const [i, o] of objects.entries()) {
    offsets.push(Buffer.byteLength(body));
    body += `${i + 1} 0 obj\n${o}\nendobj\n`;
  }
  const xref = Buffer.byteLength(body);
  body +=
    `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n` +
    offsets
      .slice(1)
      .map((o) => `${String(o).padStart(10, "0")} 00000 n \n`)
      .join("") +
    `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF`;
  return Buffer.from(body);
}
async function screenshot(page, name) {
  await page.screenshot({ path: join(artifact, name + ".png") });
}
try {
  const context = await browser.newContext({
      viewport: { width: 1120, height: 800 },
    }),
    page = await context.newPage();
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (m.type() === "error") console.log("browser:", m.text());
  });
  await page.goto(url);
  await page.getByText("把喜欢的书，放在这里", { exact: true }).waitFor();
  await screenshot(page, "empty");
  await page.getByRole("button", { name: "先读一篇示例", exact: true }).click();
  await page.locator(".rd-book").nth(2).waitFor();
  await screenshot(page, "shelf");
  await page
    .getByRole("button", { name: "阅读《山间来信》", exact: true })
    .click();
  await page.locator(".rd-text-flow").waitFor();
  await page.waitForTimeout(300);
  let total = +(
    await page.locator(".rd-pagination>span").first().textContent()
  ).split("/")[1];
  assert(total > 3, "TXT must actually paginate");
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await page.waitForTimeout(300);
  await page.getByRole("button", { name: "添加书签", exact: true }).click();
  await page.getByRole("button", { name: "阅读设置", exact: true }).click();
  await page.getByRole("button", { name: "羊皮纸", exact: true }).click();
  await page.getByRole("button", { name: "增大字号", exact: true }).click();
  await page.getByRole("button", { name: "书页", exact: true }).click();
  await screenshot(page, "settings");
  await page.getByRole("button", { name: "关闭", exact: true }).click();
  await page.waitForTimeout(250);
  await screenshot(page, "reading");
  const anchor = await page.evaluate(async () => {
    const db = await new Promise((r, j) => {
      const q = indexedDB.open("myco-reader");
      q.onsuccess = () => r(q.result);
      q.onerror = j;
    });
    return await new Promise((r) => {
      const q = db.transaction("books").objectStore("books").getAll();
      q.onsuccess = () =>
        r(q.result.find((b) => b.title === "山间来信").position);
    });
  });
  assert(anchor > 0, "page turns persist a character anchor");
  await page.getByRole("button", { name: "书架", exact: true }).click();
  await page.locator(".rd-continue").waitFor();
  await page.reload();
  await page.locator(".rd-continue").waitFor();
  await page.locator(".rd-continue").click();
  await page.locator(".rd-text-flow").waitFor();
  assert.equal(
    await page
      .locator(".rd-text-flow")
      .evaluate((e) => getComputedStyle(e).fontSize),
    "22px",
  );
  assert.equal(
    await page
      .locator(".rd-reader")
      .evaluate((e) => e.classList.contains("rd-theme-warm")),
    true,
  );
  const reopened = await page.evaluate(async () => {
    const db = await new Promise((r) => {
      const q = indexedDB.open("myco-reader");
      q.onsuccess = () => r(q.result);
    });
    return await new Promise((r) => {
      const q = db.transaction("books").objectStore("books").getAll();
      q.onsuccess = () =>
        r(q.result.find((b) => b.title === "山间来信").position);
    });
  });
  assert.equal(
    reopened,
    anchor,
    "reopening keeps the exact source anchor, independent of page numbers",
  );
  await page.getByRole("button", { name: "书签", exact: true }).click();
  await page.locator(".rd-bookmark-row").waitFor();
  await page.locator(".rd-bookmark-row>button").first().click();
  await page.getByRole("button", { name: "目录", exact: true }).click();
  await page
    .getByRole("button", { name: "第三章 雨后的森林", exact: false })
    .click();
  await page.waitForTimeout(250);
  assert(
    +(await page.locator(".rd-pagination>span").first().textContent()).split(
      "/",
    )[0] > 2,
    "TOC jumps forward",
  );
  await page.getByRole("button", { name: "搜索书中内容", exact: true }).click();
  await page.getByRole("textbox", { name: "关键词", exact: true }).fill("珍贵");
  await page.getByRole("button", { name: "搜索", exact: true }).click();
  await page.locator(".rd-search-result").first().waitFor();
  await page.locator(".rd-search-result").first().click();
  await page.getByRole("button", { name: "书架", exact: true }).click();
  // Byte-based duplicate detection and useful validation.
  const importedText = Buffer.from(
    "Chapter 1\n\nA locally imported book.\n\n" +
      "A calm day for reading. ".repeat(600) +
      "\nChapter 2\n\nThe end.",
  );
  await page.getByTestId("import-books").setInputFiles([
    { name: "real-book.txt", mimeType: "text/plain", buffer: importedText },
    { name: "same-copy.txt", mimeType: "text/plain", buffer: importedText },
    { name: "test.pdf", mimeType: "application/pdf", buffer: pdfFixture() },
  ]);
  await page.locator(".rd-book").nth(4).waitFor();
  assert.equal(await page.locator(".rd-book").count(), 5);
  await page.getByRole("button", { name: "阅读《test》", exact: true }).click();
  await page.locator(".rd-pdf-page canvas").waitFor();
  await page.waitForFunction(() =>
    document
      .querySelector(".rd-pdf-text")
      ?.textContent.includes("Reader PDF test"),
  );
  const pdfPixel = await page.locator(".rd-pdf-page canvas").evaluate((c) => {
    const d = c.getContext("2d").getImageData(0, 0, c.width, c.height).data;
    let n = 0;
    for (let i = 0; i < d.length; i += 4) if (d[i] < 150 && d[i + 3] > 0) n++;
    return n;
  });
  assert(pdfPixel > 100, "PDF paints real text pixels");
  await screenshot(page, "pdf");
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await page.waitForFunction(() =>
    document.querySelector(".rd-pdf-text")?.textContent.includes("Page two"),
  );
  await page.getByRole("button", { name: "阅读设置", exact: true }).click();
  assert.equal(
    await page.getByLabel("字号", { exact: true }).count(),
    0,
    "PDF does not promise font reflow",
  );
  await page.getByRole("button", { name: "关闭", exact: true }).click();
  await page.getByRole("button", { name: "搜索书中内容", exact: true }).click();
  await page
    .getByRole("textbox", { name: "关键词", exact: true })
    .fill("Reader PDF");
  await page.getByRole("button", { name: "搜索", exact: true }).click();
  await page.locator(".rd-search-result").first().waitFor();
  await page.locator(".rd-search-result").first().click();
  await page.waitForFunction(() =>
    document.querySelector(".rd-pdf-text")?.textContent.includes("Reader PDF"),
  );
  await page.getByRole("button", { name: "书架", exact: true }).click();
  await page.getByTestId("import-books").setInputFiles({
    name: "bad.pdf",
    mimeType: "application/pdf",
    buffer: Buffer.from("not a pdf"),
  });
  await page.getByRole("dialog", { name: "没有完成" }).waitFor();
  await page.getByRole("button", { name: "知道了", exact: true }).click();
  assert.equal(await page.locator(".rd-book").count(), 5);
  await page
    .getByRole("button", { name: "管理《real-book》", exact: true })
    .click();
  await page.getByRole("button", { name: "保存原文件", exact: true }).click();
  await page.waitForFunction(() => window.__downloads.length === 1);
  assert.equal(
    await page.evaluate(() => window.__downloads[0].bytes.byteLength),
    importedText.length,
  );
  await page.getByRole("button", { name: "从书架移除", exact: true }).click();
  await page.getByRole("button", { name: "确认移除", exact: true }).click();
  await page.waitForFunction(
    () => document.querySelectorAll(".rd-book").length === 4,
  );
  // A scanned JPX page must paint its image, not silently become a blank PDF.
  await page.getByTestId("import-books").setInputFiles({
    name: "scan.pdf",
    mimeType: "application/pdf",
    buffer: readFileSync("templates/src/reader/testdata/jpx-scan.pdf"),
  });
  await page.getByRole("button", { name: "阅读《scan》", exact: true }).click();
  await page.locator(".rd-pdf-page canvas").waitFor();
  await page.waitForFunction(() => {
    const c = document.querySelector(".rd-pdf-page canvas");
    if (!c?.width) return false;
    const p = c
      .getContext("2d")
      .getImageData(
        Math.floor(c.width / 2),
        Math.floor(c.height / 2),
        1,
        1,
      ).data;
    return p[2] > p[0] + 35 && p[3] === 255;
  });
  assert(
    requested.some((url) => url.endsWith("/pdf/wasm/openjpeg.wasm")),
    "JPEG2000 decoder is loaded from this app",
  );
  await screenshot(page, "pdf-scan");
  await page.getByRole("button", { name: "书架", exact: true }).click();
  // Encrypted books prompt again after a wrong password and never persist it.
  await page.getByTestId("import-books").setInputFiles({
    name: "locked.pdf",
    mimeType: "application/pdf",
    buffer: readFileSync("templates/src/reader/testdata/password.pdf"),
  });
  await page
    .getByRole("button", { name: "阅读《locked》", exact: true })
    .click();
  await page.getByRole("dialog", { name: "这本 PDF 需要密码" }).waitFor();
  await page.getByLabel("PDF 密码", { exact: true }).fill("wrong");
  await page.getByRole("button", { name: "打开书籍", exact: true }).click();
  await page.getByText("密码不正确，请再试一次。", { exact: true }).waitFor();
  await page
    .getByLabel("PDF 密码", { exact: true })
    .fill("reader-test-password");
  await page.getByRole("button", { name: "打开书籍", exact: true }).click();
  await page.waitForFunction(() =>
    document.querySelector(".rd-pdf-text")?.textContent.includes("Reader PDF"),
  );
  await page.getByRole("button", { name: "书架", exact: true }).click();
  await page
    .getByRole("button", { name: "阅读《locked》", exact: true })
    .click();
  await page.getByRole("dialog", { name: "这本 PDF 需要密码" }).waitFor();
  await page.getByRole("button", { name: "关闭", exact: true }).click();
  await page.locator(".rd-shelf").waitFor();
  // A request success followed by transaction abort must not show a phantom book.
  await page.evaluate(() => {
    const original = IDBObjectStore.prototype.add;
    window.__originalAdd = original;
    IDBObjectStore.prototype.add = function (...args) {
      const req = original.apply(this, args);
      if (this.name === "books")
        req.addEventListener("success", () => this.transaction.abort());
      return req;
    };
  });
  await page.getByTestId("import-books").setInputFiles({
    name: "aborted.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("A book whose transaction must fail."),
  });
  await page.getByRole("dialog", { name: "没有完成" }).waitFor();
  await page.getByRole("button", { name: "知道了", exact: true }).click();
  assert.equal(
    await page
      .getByRole("button", { name: "阅读《aborted》", exact: true })
      .count(),
    0,
  );
  await page.evaluate(() => {
    IDBObjectStore.prototype.add = window.__originalAdd;
  });
  assert.equal(
    await page.evaluate(() => window.__writes),
    0,
    "private reader never publishes a book",
  );
  // The same origin must not leak local files into another page or member.
  for (const query of ["?page=another", "?member=another"]) {
    await page.goto(url + query);
    await page.getByText("把喜欢的书，放在这里", { exact: true }).waitFor();
    assert.equal(await page.locator(".rd-book").count(), 0);
  }
  await page.evaluate(() => window.__setPreferences("en", "dark"));
  await page
    .getByText("A home for your favorite books", { exact: true })
    .waitFor();
  assert.equal(
    await page.locator(".rd-shelf").evaluate((e) => getComputedStyle(e).color),
    "rgb(228, 230, 223)",
    "theme follows live host preferences",
  );
  await context.close();
  // Reading modes preserve source anchors and use genuinely different scroll/paging paths.
  {
    const c = await browser.newContext({
        viewport: { width: 1120, height: 800 },
      }),
      p = await c.newPage();
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(url);
    await p.getByRole("button", { name: "先读一篇示例", exact: true }).click();
    await p.locator(".rd-book").nth(2).waitFor();
    await p
      .getByRole("button", { name: "阅读《山间来信》", exact: true })
      .click();
    await p.locator(".rd-text-flow").waitFor();
    const mode = async (label) => {
      await p.getByRole("button", { name: "阅读设置", exact: true }).click();
      await p.getByRole("button", { name: label, exact: true }).click();
      await p.getByRole("button", { name: "关闭", exact: true }).click();
      await p.waitForTimeout(180);
    };
    const currentPage = async () =>
      +(await p.locator(".rd-pagination>span").first().textContent()).split(
        "/",
      )[0];
    const stored = async (title) =>
      p.evaluate(async (title) => {
        const db = await new Promise((r) => {
          const q = indexedDB.open("myco-reader");
          q.onsuccess = () => r(q.result);
        });
        return await new Promise((r) => {
          const q = db.transaction("books").objectStore("books").getAll();
          q.onsuccess = () => r(q.result.find((b) => b.title === title));
        });
      }, title);
    await mode("上下逐页");
    assert.equal(await p.locator(".rd-mode-vertical").count(), 1);
    const first = await currentPage();
    await p.locator(".rd-reading-stage").hover();
    await p.mouse.wheel(0, 160);
    await p.waitForFunction(
      (n) =>
        +document
          .querySelector(".rd-pagination>span")
          .textContent.split("/")[0] === n,
      first + 1,
    );
    await p.keyboard.press("ArrowDown");
    assert.equal(await currentPage(), first + 2);
    await p.keyboard.press("ArrowUp");
    assert.equal(await currentPage(), first + 1);
    await p.waitForTimeout(180);
    const before = await stored("山间来信");
    await mode("连续滚动");
    await p.locator(".rd-text-scroll").waitFor();
    assert.equal(
      (await stored("山间来信")).position,
      before.position,
      "switching to scroll retains exact character anchor",
    );
    const y = await p.locator(".rd-text-scroll").evaluate((e) => e.scrollTop);
    await p.locator(".rd-text-scroll").hover();
    await p.mouse.wheel(0, 600);
    await p.waitForFunction(
      (y) => document.querySelector(".rd-text-scroll").scrollTop > y + 100,
      y,
    );
    await p.waitForTimeout(180);
    assert.equal(
      await p
        .locator(".rd-page-shell")
        .evaluate((e) => e.getAnimations().length),
      0,
      "continuous scrolling does not run page-flip effects",
    );
    await p.getByRole("button", { name: "目录", exact: true }).click();
    await p
      .getByRole("button", { name: "第三章 雨后的森林", exact: false })
      .click();
    await p.waitForTimeout(180);
    const chapter = await stored("山间来信");
    assert(chapter.position > before.position);
    await p.getByRole("button", { name: "添加书签", exact: true }).click();
    await p.locator(".rd-text-scroll").evaluate((e) => {
      e.scrollTop = 0;
    });
    await p.waitForTimeout(180);
    await p.getByRole("button", { name: "书签", exact: true }).click();
    await p.locator(".rd-bookmark-row>button").first().click();
    await p.waitForTimeout(180);
    assert.equal(
      (await stored("山间来信")).position,
      chapter.position,
      "scroll bookmarks restore source position",
    );
    await screenshot(p, "text-continuous");
    await mode("上下逐页");
    assert.equal(
      (await stored("山间来信")).position,
      chapter.position,
      "returning to page mode preserves source position",
    );
    await p.getByRole("button", { name: "阅读设置", exact: true }).click();
    await screenshot(p, "vertical-modes");
    await p.getByRole("button", { name: "关闭", exact: true }).click();
    await p.getByRole("button", { name: "书架", exact: true }).click();
    await p.getByTestId("import-books").setInputFiles({
      name: "vertical-pdf.pdf",
      mimeType: "application/pdf",
      buffer: pdfFixture(),
    });
    await p
      .getByRole("button", { name: "阅读《vertical-pdf》", exact: true })
      .click();
    await p.locator(".rd-pdf-page canvas").waitFor();
    await p.waitForFunction(
      () => document.querySelector(".rd-pdf-page canvas")?.height > 0,
    );
    const fit = await p.locator(".rd-pdf-page canvas").evaluate((e) => ({
      h: e.getBoundingClientRect().height,
      box: e.closest(".rd-pdf-scroll").clientHeight,
    }));
    assert(fit.h <= fit.box, "vertical PDF page fits the screen before paging");
    await p.keyboard.press("ArrowDown");
    await p.waitForFunction(() =>
      document.querySelector(".rd-pdf-text")?.textContent.includes("Page two"),
    );
    await mode("连续滚动");
    await p.locator(".rd-pdf-continuous").waitFor();
    await p.waitForTimeout(300);
    assert.equal(
      (await stored("vertical-pdf")).position,
      1,
      "PDF retains page when changing mode",
    );
    assert(
      await p
        .locator(".rd-pdf-continuous")
        .evaluate((e) => e.scrollHeight > e.clientHeight * 2),
    );
    await p.locator(".rd-pdf-continuous").evaluate((e) => {
      e.scrollTop = 400;
    });
    await p.waitForTimeout(250);
    const location = await stored("vertical-pdf");
    assert(location.pageOffset > 0);
    await p.getByRole("button", { name: "书架", exact: true }).click();
    await p
      .getByRole("button", { name: "阅读《vertical-pdf》", exact: true })
      .click();
    await p.locator(".rd-pdf-continuous").waitFor();
    await p.waitForTimeout(400);
    const reopened = await stored("vertical-pdf");
    assert.equal(reopened.position, location.position);
    assert(
      Math.abs(reopened.pageOffset - location.pageOffset) < 0.025,
      "PDF restores intra-page scroll position",
    );
    await screenshot(p, "pdf-continuous");
    await p.getByRole("button", { name: "书架", exact: true }).click();
    await p
      .getByTestId("import-books")
      .setInputFiles({
        name: "long-pdf.pdf",
        mimeType: "application/pdf",
        buffer: longPDF(),
      });
    await p
      .getByRole("button", { name: "阅读《long-pdf》", exact: true })
      .click();
    await p.locator(".rd-pdf-continuous").waitFor();
    await p.waitForTimeout(300);
    assert(
      (await p.locator(".rd-pdf-slot").count()) <= 6,
      "long PDFs only mount nearby pages",
    );
    await p.getByRole("slider", { name: "阅读进度", exact: true }).fill("79");
    await p.waitForFunction(() =>
      document
        .querySelector(".rd-pdf-continuous")
        .textContent.includes("Page 80 of 80"),
    );
    await p.waitForTimeout(250);
    assert.equal(
      (await stored("long-pdf")).position,
      79,
      "last-page jump survives changing page heights",
    );
    assert((await p.locator(".rd-pdf-slot").count()) <= 6);

    await c.close();
  }
  // Actual narrow viewport + touch scroll/swipe, in both languages and themes.
  for (const [lang, theme] of [
    ["zh", "light"],
    ["en", "dark"],
  ]) {
    const mobile = await browser.newContext({
        viewport: { width: 390, height: 844 },
        hasTouch: true,
        isMobile: true,
        reducedMotion: "reduce",
      }),
      p = await mobile.newPage();
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(`${url}?myco_lang=${lang}&theme=${theme}`);
    await p.locator(".rd-empty>.rd-quiet").click();
    await p.locator(".rd-book").nth(2).waitFor();
    assert.equal(
      await p.evaluate(() => document.documentElement.scrollWidth),
      390,
      "actual viewport width is preserved",
    );
    const scrollSession = await mobile.newCDPSession(p);
    await scrollSession.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: 350, y: 660 }],
    });
    await scrollSession.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: 349, y: 250 }],
    });
    await scrollSession.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });
    await p.waitForTimeout(250);
    assert(
      (await p.locator(".rd-shelf").evaluate((e) => e.scrollTop)) > 10,
      "shelf scrolls with real touch",
    );
    await p.locator(".rd-shelf").evaluate((e) => {
      e.scrollTop = 0;
    });
    await screenshot(p, `mobile-shelf-${lang}`);
    await p.locator(".rd-book-open").first().click();
    await p.locator(".rd-text-flow").waitFor();
    await p.waitForTimeout(200);
    // Browser-native input sequence; no scrollIntoView to mask overflow.
    const session = await mobile.newCDPSession(p);
    await session.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: 320, y: 400 }],
    });
    await session.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: 180, y: 402 }],
    });
    await session.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });
    await p.waitForTimeout(100);
    assert.match(
      await p.locator(".rd-pagination>span").first().textContent(),
      /^2\s*\//,
      "real touch swipe turns a page",
    );
    assert.equal(
      await p
        .locator(".rd-page-shell")
        .evaluate((e) => e.getAnimations().length),
      0,
      "reduced motion disables page animations",
    );

    await screenshot(p, `mobile-reading-${lang}`);
    await p
      .getByRole("button", {
        name: lang === "zh" ? "阅读设置" : "Reading settings",
        exact: true,
      })
      .click();
    const range = p.getByRole("slider", {
      name: lang === "zh" ? "行间距" : "Line spacing",
      exact: true,
    });
    await range.fill("2.4");
    await p
      .getByRole("button", {
        name: lang === "zh" ? "关闭" : "Close",
        exact: true,
      })
      .click();
    assert(
      Math.abs(
        (await p
          .locator(".rd-text-flow")
          .evaluate((e) => parseFloat(getComputedStyle(e).lineHeight))) - 50.4,
      ) < 0.01,
    );
    const chooseMode = async (label) => {
      await p
        .getByRole("button", {
          name: lang === "zh" ? "阅读设置" : "Reading settings",
          exact: true,
        })
        .click();
      await p.getByRole("button", { name: label, exact: true }).click();
      await p
        .getByRole("button", {
          name: lang === "zh" ? "关闭" : "Close",
          exact: true,
        })
        .click();
      await p.waitForTimeout(120);
    };
    await chooseMode(lang === "zh" ? "上下逐页" : "Vertical pages");
    const beforeUp = +(
      await p.locator(".rd-pagination>span").first().textContent()
    ).split("/")[0];
    await session.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: 220, y: 640 }],
    });
    await session.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: 220, y: 310 }],
    });
    await session.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });
    await p.waitForTimeout(150);
    assert.equal(
      +(await p.locator(".rd-pagination>span").first().textContent()).split(
        "/",
      )[0],
      beforeUp + 1,
      "real up swipe turns exactly one vertical page",
    );
    await chooseMode(lang === "zh" ? "连续滚动" : "Continuous scroll");
    const scrollBefore = await p
      .locator(".rd-text-scroll")
      .evaluate((e) => e.scrollTop);
    await session.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: 220, y: 640 }],
    });
    await session.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: 220, y: 310 }],
    });
    await session.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });
    await p.waitForTimeout(200);
    assert(
      (await p.locator(".rd-text-scroll").evaluate((e) => e.scrollTop)) >
        scrollBefore + 100,
      "continuous text supports native vertical touch scrolling",
    );
    await screenshot(p, `mobile-continuous-${lang}`);
    await mobile.close();
  }
  for (const [lang, theme] of [
    ["zh", "dark"],
    ["en", "light"],
  ]) {
    const c = await browser.newContext({
        viewport: { width: 1200, height: 800 },
      }),
      p = await c.newPage();
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(`${url}?myco_lang=${lang}&theme=${theme}`);
    await p.locator(".rd-empty>.rd-quiet").click();
    await p.locator(".rd-book").nth(2).waitFor();
    await screenshot(p, `desktop-${lang}-${theme}`);
    await p.locator(".rd-book-open").first().click();
    await p.locator(".rd-text-flow").waitFor();
    await p
      .getByRole("button", {
        name: lang === "zh" ? "阅读设置" : "Reading settings",
        exact: true,
      })
      .click();
    await screenshot(p, `desktop-settings-${lang}-${theme}`);
    await p.keyboard.press("Escape");
    await c.close();
  }
  assert.deepEqual(errors, [], "no unhandled document/lifecycle errors");
  console.log(
    "reader: import/dedupe, storage isolation, pagination/reflow, progress/settings/bookmarks, TOC/search, PDF worker/pixels/search/JPX scan, encrypted PDF retry/cancel, transaction aborts, original-file download, validation/removal, vertical paging/continuous mode, source anchors, virtual PDF, mobile touch and reduced motion passed",
  );
} finally {
  await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
