// Verify the conversation against the built UI, including bridge delivery,
// incoming data events, reload, and the fallback for refused messages.
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import { createServer } from "node:http";
import { join } from "node:path";

const require = createRequire(join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const root = new URL("../internal/templates/official/tutor/", import.meta.url);
const files = new Map([["/", ["text/html", readFileSync(new URL("index.html", root))]]]);
for (const asset of readdirSync(new URL("assets/", root))) {
  const path = "assets/" + asset;
  files.set("/" + path, [path.endsWith(".css") ? "text/css" : "text/javascript", readFileSync(new URL(path, root))]);
}
const server = createServer((req, res) => {
  const file = files.get(new URL(req.url, "http://test").pathname);
  if (!file) return res.writeHead(404).end();
  res.writeHead(200, {
    "Content-Type": file[0],
    "Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' data: blob:; worker-src 'self' blob:; img-src 'self' data: blob:",
  }).end(file[1]);
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
let browser;
try {
  browser = await chromium.launch({ executablePath: process.env.UI_SMOKE_EXECUTABLE });
  const url = `http://127.0.0.1:${server.address().port}/`;
  for (const [lang, theme, width, refused] of [["zh", "light", 1400, false], ["en", "dark", 390, true]]) {
    const page = await browser.newPage({ viewport: { width, height: 740 }, colorScheme: theme, permissions: ["clipboard-read", "clipboard-write"] });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.addInitScript(({ refused, lang, theme }) => {
      const records = JSON.parse(localStorage.getItem("rows") || "[]");
      const listeners = [];
      window.answer = (id, text) => {
        const collection = "tutor_answers";
        const data = typeof text === "string" ? { text } : text;
        records.push({ collection, id, data, version: 1 });
        localStorage.setItem("rows", JSON.stringify(records));
        for (const listener of listeners) listener({ collection, record: id, data, op: "put", version: 1 });
      };
      window.myco = {
        preferences: { language: lang, theme },
        ready: Promise.resolve({ me: { name: "learner" }, page: { id: "pg_fixture" } }),
        data: {
          list: async (collection) => ({ records: records.filter((r) => r.collection === collection), seq: 0 }),
          put: async (collection, data, options) => {
            const record = { collection, data, id: options?.id || "r" + records.length, version: 1 };
            records.push(record);
            localStorage.setItem("rows", JSON.stringify(records));
            return record;
          },
          on: (listener) => { listeners.push(listener); return () => {}; },
        },
        send: async (message) => {
          window.sent = message;
          if (!records.some((row) => row.collection === "tutor_requests" && message.includes(row.id))) throw new Error("Question was not saved");
          if (refused) throw new Error("denied");
          return { id: "msg_tutor" };
        },
      };
    }, { refused, lang, theme });
    await page.goto(url);
    const ask = lang === "zh" ? "问导师" : "Ask tutor";
    await page.getByRole("heading", { name: lang === "zh" ? "今天想学什么？" : "What would you like to learn?" }).waitFor();
    assert.equal(await page.locator(".tu-main").evaluate((el) => el.scrollWidth <= el.clientWidth), true);
    await page.getByRole("button", { name: lang === "zh" ? "一步步学" : "Learn step by step" }).click();
    const question = await page.getByRole("textbox", { name: ask }).inputValue();
    assert.ok(question.includes("SQL"));
    await page.getByRole("button", { name: ask, exact: true }).click();
    await page.waitForFunction(() => typeof window.sent === "string");
    const sent = await page.evaluate(() => window.sent);
    assert.ok(sent.includes("pg_fixture") && sent.includes("r0") && sent.includes(question));
    if (refused) {
      assert.equal(await page.getByRole("textbox", { name: "Message for your tutor" }).inputValue(), sent);
      await page.locator(".tu-copy").getByRole("button", { name: "Copy for an agent" }).click();
      assert.equal(await page.evaluate(() => navigator.clipboard.readText()), sent);
    }
    const answer = "SQL helps you ask questions of tabular data.";
    await page.evaluate((answer) => window.answer("r0", answer), answer);
    await page.getByText(answer, { exact: true }).waitFor();
    await page.reload();
    await page.getByText(answer, { exact: true }).waitFor();

    if (!refused) {
      // Feed a real PDF through the browser File API and packaged PDF.js worker.
      const blank = await browser.newPage();
      await blank.setContent(`<body><section style="break-after:page">The first page introduces prime numbers.</section>
        <section>教材第二页。The real part is the horizontal coordinate.</section></body>`);
      const pdf = await blank.pdf();
      await blank.close();
      await page.getByRole("button", { name: "教材库", exact: true }).click();
      await page.locator('input[type="file"]').setInputFiles({ name: "chapter.pdf", mimeType: "application/pdf", buffer: pdf });
      const reader = page.getByRole("complementary", { name: "教材原文" });
      await reader.getByText("The first page introduces prime numbers.").waitFor();
      await reader.getByRole("button", { name: "下一页" }).click();
      await reader.getByText(/教材第二页。.*The real part is the horizontal coordinate\./).waitFor();
      await reader.getByRole("button", { name: "结合这页问导师" }).click();
      await page.getByRole("combobox", { name: "学习基础" }).selectOption("school");
      await page.getByRole("textbox", { name: ask }).fill("用教材第二页讲实部");
      await page.getByRole("button", { name: ask, exact: true }).click();
      await page.waitForFunction(() => window.sent?.includes("用教材第二页讲实部"));
      const request = await page.evaluate(() => JSON.parse(localStorage.rows).filter((r) => r.collection === "tutor_requests").pop());
      assert.equal(request.data.level, "school");
      assert.equal(request.data.page, 2);
      assert.ok(request.data.material);
      const lesson = {
        title: "实部是横坐标",
        stages: [{ title: "从平面坐标想起", text: "横坐标表示这个点向左或向右的位置。" }],
        diagram: { type: "coordinate", title: "看临界线", line: 0.5, xLabel: "Re(s)", yLabel: "Im(s)", point: { x: 0.5, y: 1, label: "示意点" } },
        check: { question: "哪一步是找实部？", choices: ["看横坐标", "看纵坐标"], answer: 0, feedback: "实部画在横轴上。" },
        sources: [{ page: 1, quote: "This quote is invented." }, { page: 2, quote: "The real part is the horizontal coordinate." }],
      };
      await page.evaluate(({ request, lesson }) => window.answer(request.id, { text: "The real part is the horizontal coordinate.", lesson }), { request, lesson });
      const lessonView = page.locator(".tu-lesson");
      await lessonView.getByRole("heading", { name: "实部是横坐标" }).waitFor();
      assert.equal(await lessonView.getByText("This quote is invented.").count(), 0);
      await lessonView.getByRole("slider").fill("0.6");
      await lessonView.getByText("Re(s) = 0.6 · 点在线外", { exact: true }).waitFor();
      await lessonView.getByRole("button", { name: "看横坐标", exact: true }).click();
      await lessonView.getByText(/答对了.*实部画在横轴上/).waitFor();
      await reader.getByRole("button", { name: "上一页" }).click();
      await lessonView.getByRole("button", { name: /The real part is the horizontal coordinate/ }).click();
      await reader.getByText(/The real part is the horizontal coordinate/).waitFor();
      await page.reload();
      await page.locator(".tu-lesson").getByRole("heading", { name: "实部是横坐标" }).waitFor();
    } else {
      await page.locator(".tu-example-link").click();
      await page.getByRole("heading", { name: "The Riemann hypothesis: From primes to zeros" }).waitFor();
      await page.getByRole("slider").fill("100");
      await page.getByText("There are 25 primes among the first 100 positive integers.").waitFor();
      await page.getByRole("button", { name: "University", exact: true }).click();
      await page.getByRole("heading", { name: "Start from the Euler product" }).waitFor();
      assert.equal(await page.locator(".tu-main").evaluate((el) => el.scrollWidth <= el.clientWidth), true);
    }
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log("PASS tutor transport, PDF import, source verification and navigation, interactive lesson and quiz, reload, and mobile example");
} finally {
  await browser?.close();
  server.close();
}
