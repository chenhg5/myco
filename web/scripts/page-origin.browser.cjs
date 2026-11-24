const assert = require("node:assert/strict");
const { execFileSync } = require("node:child_process");
const { createHash } = require("node:crypto");
const { readFile } = require("node:fs/promises");
const { join } = require("node:path");
const globalModules = execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim();
const { chromium } = require(join(globalModules, "playwright"));
const { origins, old, control, vault } = JSON.parse(process.argv[2]);

async function storage(page, write) {
  return page.evaluate(async (write) => {
    if (write) localStorage.setItem("origin-fixture", "only this page");
    const db = await new Promise((resolve, reject) => {
      const request = indexedDB.open("origin-fixture", 1);
      request.onupgradeneeded = () => request.result.createObjectStore("items");
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
    const value = await new Promise((resolve, reject) => {
      const tx = db.transaction("items", write ? "readwrite" : "readonly");
      const store = tx.objectStore("items");
      if (write) store.put("only this page", "item");
      const request = store.get("item");
      tx.oncomplete = () => resolve(request.result ?? null);
      tx.onabort = () => reject(tx.error);
    });
    db.close();
    return [localStorage.getItem("origin-fixture"), value];
  }, write);
}

(async () => {
  const browser = await chromium.launch({ executablePath: process.env.UI_SMOKE_EXECUTABLE });
  try {
    const context = await browser.newContext();
    const page = await context.newPage();
    await page.goto(origins[0]);
    assert.deepEqual(await storage(page, true), ["only this page", "only this page"]);
    for (const address of origins.slice(1, 4)) {
      await page.goto(address);
      assert.deepEqual(await storage(page, false), [null, null]);
    }
    await page.goto(control + "/dom");
    await page.frameLocator("#one").getByText("Isolated page").waitFor();
    await page.frameLocator("#two").getByText("Isolated page").waitFor();
    const firstFrame = page.frames().find(frame => frame.url() === origins[0]);
    assert.equal(await firstFrame.evaluate(() => {
      try { void window.parent.frames[1].document; return false; }
      catch (error) { return error.name === "SecurityError"; }
    }), true);
    const restarted = await (await fetch(control + "/restart")).text();
    assert.equal(new URL(restarted).origin, new URL(origins[0]).origin);
    await page.goto(restarted);
    assert.deepEqual(await storage(page, false), ["only this page", "only this page"]);
    await page.goto(old);
    await page.evaluate(() => new Promise((resolve, reject) => {
      const request = indexedDB.open("myco-video", 1);
      request.onupgradeneeded = () => request.result.createObjectStore("files");
      request.onsuccess = () => {
        const db = request.result;
        const tx = db.transaction("files", "readwrite");
        tx.objectStore("files").put({file: new Blob(["synthetic original"]), name: "fixture.webm"}, "fixture");
        tx.oncomplete = () => { db.close(); resolve(); };
        tx.onabort = () => reject(tx.error);
      };
    }));
    const hash = createHash("sha256").update("synthetic original").digest("hex");
    for (let attempt = 0; attempt < 2; attempt++) {
      const recovery = await (await fetch(control + "/recover")).text();
      await page.goto(recovery);
      const downloadPromise = page.waitForEvent("download");
      await page.getByRole("button", { name: "Save copy" }).click();
      const download = await downloadPromise;
      assert.equal(download.suggestedFilename(), "fixture.webm");
      const saved = await readFile(await download.path());
      assert.equal(createHash("sha256").update(saved).digest("hex"), hash);
    }
    const shots = process.env.MYCO_PAGE_ORIGIN_SHOT;
    if (shots) await page.screenshot({ path: shots });
    if (vault) {
      await page.goto(control + "/vault?old=1");
      const frame = page.frameLocator("#vault");
      await frame.getByRole("heading", { name: "Update the myco App" }).waitFor({ timeout: 10000 }).catch(async error => {
        console.error("vault frame:", page.frames().map(frame => frame.url()));
        console.error(await frame.locator("body").innerText({ timeout: 1000 }).catch(() => "unavailable"));
        throw error;
      });
      assert.equal(await frame.locator('input[type="password"]').count(), 0);
      await page.goto(control + "/vault");
      await frame.getByRole("heading", { name: "Create vault" }).waitFor();
      await frame.getByLabel("Master password", { exact: true }).fill("fixture master password only!");
      await frame.getByLabel("Confirm master password").fill("fixture master password only!");
      await frame.getByRole("button", { name: "Create vault" }).click();
      await frame.getByRole("button", { name: "Lock", exact: true }).click();
      await frame.getByRole("heading", { name: "Unlock vault" }).waitFor();
    }
    const gallery = process.env.MYCO_VAULT_SHOTS;
    if (vault && gallery) {
      for (const [lang, colorScheme] of [["en", "dark"], ["zh", "light"]]) {
        const artwork = await browser.newContext({ colorScheme, viewport: { width: 1200, height: 760 }, deviceScaleFactor: 2 });
        const scene = await artwork.newPage();
        await scene.goto(control + "/vault?lang=" + lang);
        const frame = scene.frameLocator("#vault");
        const zh = lang === "zh";
        await frame.getByLabel(zh ? "主密码" : "Master password", { exact: true }).fill("fixture master password only!");
        await frame.getByLabel(zh ? "确认主密码" : "Confirm master password").fill("fixture master password only!");
        await frame.getByRole("button", { name: zh ? "创建密码库" : "Create vault" }).click();
        for (const [title, site] of [
          [zh ? "知识库" : "Knowledge base", "notes.example.invalid"],
          [zh ? "设计工作台" : "Design workspace", "canvas.example.invalid"],
          [zh ? "工作室邮箱" : "Studio mail", "mail.example.invalid"],
        ]) {
          await frame.getByRole("button", { name: zh ? "新增" : "Add", exact: true }).click();
          await frame.getByLabel(zh ? "标题" : "Title").fill(title);
          await frame.getByLabel(zh ? "网站" : "Website").fill(site);
          await frame.getByLabel(zh ? "用户名" : "Username").fill("alex@example.invalid");
          await frame.getByLabel(zh ? "密码" : "Password", { exact: true }).fill("Synthetic fixture only");
          await frame.getByLabel(zh ? "备注" : "Note").fill(zh ? "工作室项目通知与协作邀请。" : "Studio project notices and invitations.");
          await frame.getByRole("button", { name: zh ? "保存" : "Save", exact: true }).click();
          await frame.getByRole("heading", { name: title }).waitFor();
        }
        await scene.locator("#vault").screenshot({ path: join(gallery, `vault-${lang}-${colorScheme}.png`) });
        await artwork.close();
      }
    }

    console.log("PASS Chromium isolated page/workspace/account storage, restart, and repeat recovery SHA-256");
    if (vault) console.log("PASS actual vault bundle and bridge: old-client gate, isolated create and lock");
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
