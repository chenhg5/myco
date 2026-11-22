// Verify the conversation against the built UI, including bridge delivery,
// incoming data events, reload, and the fallback for refused messages.
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { createServer } from "node:http";
import { join } from "node:path";

const require = createRequire(join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"));
const { chromium } = require("playwright");
const root = new URL("../internal/templates/official/tutor/", import.meta.url);
const files = new Map([["/", ["text/html", readFileSync(new URL("index.html", root))]]]);
const html = String(files.get("/")[1]);
for (const [, path] of html.matchAll(/(?:src|href)="\.\/(assets\/[^"]+)"/g)) {
  files.set("/" + path, [path.endsWith(".js") ? "text/javascript" : "text/css", readFileSync(new URL(path, root))]);
}
const server = createServer((req, res) => {
  const file = files.get(new URL(req.url, "http://test").pathname);
  if (!file) return res.writeHead(404).end();
  res.writeHead(200, { "Content-Type": file[0] }).end(file[1]);
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
        const data = { text };
        records.push({ collection, id, data, version: 1 });
        localStorage.setItem("rows", JSON.stringify(records));
        for (const listener of listeners) listener({ collection, record: id, data, op: "put", version: 1 });
      };
      window.myco = {
        preferences: { language: lang, theme },
        ready: Promise.resolve({ me: { name: "learner" }, page: { id: "pg_fixture" } }),
        data: {
          list: async (collection) => ({ records: records.filter((r) => r.collection === collection), seq: 0 }),
          put: async (collection, data) => {
            const record = { collection, data, id: "r" + records.length, version: 1 };
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
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log("PASS tutor prompt, real-time answer, reload, denied delivery fallback, and desktop/mobile layout");
} finally {
  await browser?.close();
  server.close();
}
