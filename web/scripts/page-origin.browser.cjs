const assert = require("node:assert/strict");
const { execFileSync } = require("node:child_process");
const { createHash } = require("node:crypto");
const { mkdtemp, readFile, rm } = require("node:fs/promises");
const { tmpdir } = require("node:os");
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
    const cache = await caches.open("origin-fixture");
    const cacheKey = new URL("/cache-fixture", location.href).href;
    if (write) await cache.put(cacheKey, new Response("only this page"));
    const cached = await cache.match(cacheKey);
    return [localStorage.getItem("origin-fixture"), value, cached ? await cached.text() : null];
  }, write);
}

(async () => {
  const profile = await mkdtemp(join(tmpdir(), "myco-page-origin-browser-"));
  const start = () => chromium.launchPersistentContext(profile, { executablePath: process.env.UI_SMOKE_EXECUTABLE });
  let context;
  try {
    context = await start();
    let page = context.pages()[0];
    await page.goto(origins[0]);
    assert.deepEqual(await storage(page, true), ["only this page", "only this page", "only this page"]);
    for (const address of origins.slice(1, 4)) {
      await page.goto(address);
      assert.deepEqual(await storage(page, false), [null, null, null]);
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
    await context.close();
    context = await start();
    page = context.pages()[0];
    await page.goto(restarted);
    assert.deepEqual(await storage(page, false), ["only this page", "only this page", "only this page"]);
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
      const add = frame.getByRole("button", { name: "Add", exact: true });
      await add.click({ timeout: 8000 }).catch(async error => {
        console.error(await frame.locator("body").innerText());
        throw error;
      });
      await frame.getByLabel("Title").fill("Studio mail");
      await frame.getByLabel("Password", { exact: true }).fill("Synthetic fixture only");
      await frame.getByRole("button", { name: "Save", exact: true }).click();
      await frame.getByRole("heading", { name: "Studio mail" }).waitFor();
      if (process.env.MYCO_IN_APP_VAULT_SHOT) {
        await page.locator("#vault").screenshot({ path: process.env.MYCO_IN_APP_VAULT_SHOT });
      }
      await frame.getByRole("button", { name: "Lock", exact: true }).click();
      await frame.getByRole("heading", { name: "Unlock vault" }).waitFor();
    }
    console.log("PASS Chromium isolated page/workspace/account storage, restart, and repeat recovery SHA-256");
    if (vault) console.log("PASS actual vault bundle and bridge: old-client gate, isolated create and lock");
  } finally {
    await context?.close();
    await rm(profile, { recursive: true, force: true });
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
