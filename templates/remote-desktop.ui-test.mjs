import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const here = path.dirname(fileURLToPath(import.meta.url)),
  root = path.resolve(here, "../internal/templates/official/remote-desktop"),
  shots = path.join(here, "remote-desktop-shots");
fs.mkdirSync(shots, { recursive: true });
const bridge = fs.readFileSync(
  path.resolve(here, "../internal/console/pagebridge.js"),
  "utf8",
);
const serve = (fn) =>
  new Promise((r) => {
    const s = http.createServer(fn);
    s.listen(0, "127.0.0.1", () => r(s));
  });
const files = await serve((req, res) => {
  if (req.url === "/bridge.js") {
    res.setHeader("Content-Type", "text/javascript");
    res.end(bridge);
    return;
  }
  const name = new URL(req.url, "http://x").pathname,
    p = path.resolve(root, "." + name);
  if (!p.startsWith(root + path.sep) || !fs.existsSync(p)) {
    res.writeHead(404).end();
    return;
  }
  res.setHeader(
    "Content-Type",
    {
      ".html": "text/html",
      ".css": "text/css",
      ".js": "text/javascript",
      ".svg": "image/svg+xml",
    }[path.extname(p)] || "application/octet-stream",
  );
  let raw = fs.readFileSync(p);
  if (p.endsWith(".html"))
    raw = String(raw).replace(
      "<head>",
      '<head><script src="/bridge.js"></script>',
    );
  res.end(raw);
});
const origin = `http://127.0.0.1:${files.address().port}`;
const host = await serve((req, res) => {
  const q = new URL(req.url, "http://x").searchParams,
    language = q.get("lang") || "en",
    theme = q.get("theme") || "light";
  res.setHeader("Content-Type", "text/html; charset=utf-8");
  res.end(`<!doctype html><body style="margin:0"><iframe id="app" src="${origin}/index.html?myco_lang=${language}" style="width:100vw;height:100vh;border:0"></iframe><script>
 const preferences={language:${JSON.stringify(language)},theme:${JSON.stringify(theme)}};
 let status={protocol:1,version:'0.1.0',supported:true,installed:false,running:false};
 const devices=[{machine:'mac',name:'Studio Mac',os:'darwin',control:true},{machine:'win',name:'Windows 工作站',os:'windows',control:true},{machine:'mini',name:'Mac mini',os:'darwin',control:false}];
 const frame=document.getElementById('app');window.__actions=[];
 window.addEventListener('message',e=>{if(e.source!==frame.contentWindow||e.origin!==${JSON.stringify(origin)}||e.data?.myco!=='call')return;
 const d=e.data,a=d.args||{};let value;
 if(d.method==='ready')value={page:{id:'pg_remote',name:'My desktop'},me:{name:'me'},state:null,version:1,preferences};
 else if(d.method==='data.list')value={records:[],seq:0};
 else if(d.method==='remote.call'){
 window.__actions.push(a.action);
 if(a.action==='devices')value=devices;
 if(a.action==='status')value=status;
 if(a.action==='install'){status={...status,installed:true};value=status;}
 if(a.action==='enable'){status={...status,running:true,pairing_code:'a1b2c3d4'};value=status;}
 if(a.action==='disable'){status={...status,running:false,pairing_code:undefined};value=true;}
 if(a.action==='remove'){status={...status,installed:false,running:false,pairing_code:undefined};value=true;}
 }else value=true;
 frame.contentWindow.postMessage({myco:'result',id:d.id,value},e.origin);
 });</script></body>`);
});
let chromium;
try {
  ({ chromium } = createRequire(import.meta.url)("playwright"));
} catch {
  const p = execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim();
  ({ chromium } = createRequire(path.join(p, "noop.js"))("playwright"));
}
const browser = await chromium.launch(
  process.env.UI_SMOKE_EXECUTABLE
    ? { executablePath: process.env.UI_SMOKE_EXECUTABLE }
    : {},
);
try {
  for (const width of [1200, 760, 1040])
    for (const lang of ["zh", "en"])
      for (const theme of ["light", "dark"]) {
        const context = await browser.newContext({
            viewport: { width, height: width === 1040 ? 650 : 860 },
            colorScheme: theme === "dark" ? "light" : "dark",
          }),
          page = await context.newPage(),
          errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.goto(
          `http://127.0.0.1:${host.address().port}/?lang=${lang}&theme=${theme}`,
        );
        const f = page.frameLocator("#app");
        await f.locator(".rd-device").nth(2).waitFor();
        assert.equal(
          await f.locator("h1").innerText(),
          lang === "en" ? "Remote Desktop" : "远程桌面",
        );
        assert.equal(
          await f.locator("html").getAttribute("data-myco-theme"),
          theme,
        );
        const overflow = await f
          .locator("body")
          .evaluate((e) => e.scrollWidth > window.innerWidth + 1);
        assert.equal(overflow, false);
        await page.screenshot({
          path: path.join(shots, `${width}-${lang}-${theme}-connect.png`),
        });
        await f.locator(".rd-search input").fill("Studio");
        assert.equal(await f.locator(".rd-device").count(), 1);
        await f.locator(".rd-device").first().click();
        await f.locator(".rd-field input").fill("abcdef12");
        assert.equal(
          await f.locator(".rd-dialog .rd-primary").isEnabled(),
          true,
        );
        await page.screenshot({
          path: path.join(shots, `${width}-${lang}-${theme}-pair.png`),
        });
        await page.keyboard.press("Escape");
        await f.locator(".rd-dialog").waitFor({ state: "detached" });
        await f.locator(".rd-tabs button").nth(1).click();
        await page.screenshot({
          path: path.join(shots, `${width}-${lang}-${theme}-setup.png`),
        });
        await f.locator(".rd-install-card .rd-primary").click();
        await f.locator(".rd-enable-card").waitFor();
        const permissionRow = f.locator(".rd-enable-card .rd-check");
        assert((await permissionRow.boundingBox()).height >= 64, "the entire permission row must be a large click target");
        const permission = permissionRow.getByRole("switch");
        assert.equal(await permission.isChecked(), false, "control is opt-in");
        await permissionRow.locator("small").click();
        assert.equal(await permission.isChecked(), true, "clicking the explanatory text also toggles permission");
        assert(await f.locator(".rd-share-notes p").first().evaluate(e => parseFloat(getComputedStyle(e).fontSize) >= 14), "sharing notes must be readable");
        await page.screenshot({path:path.join(shots, `${width}-${lang}-${theme}-permission.png`)});
        await f.locator(".rd-enable-card .rd-primary").click();
        await f.locator(".rd-code").waitFor();
        assert.equal(
          (await f.locator(".rd-code").innerText()).trim(),
          "a1b2c3d4",
        );
        await page.screenshot({
          path: path.join(shots, `${width}-${lang}-${theme}-share.png`),
        });
        await f.locator(".rd-header .rd-icon").click();
        assert(await f.locator(".rd-component-update").isDisabled(), "updating must not interrupt active sharing");
        await f.locator(".rd-dialog-close").click();
        await f.locator(".rd-pairing .rd-primary").click();
        await f.locator(".rd-enable-card").waitFor();
        await f.locator(".rd-header .rd-icon").click();
        await f.locator(".rd-component-update").click();await f.locator(".rd-dialog button").filter({hasText:lang==="zh"?"卸载组件":"Uninstall component"}).click();
        await page.keyboard.press("Escape");
        await f.locator(".rd-install-card").waitFor();
        assert.deepEqual(errors, []);
        const actions = await page.evaluate(() => window.__actions);
        assert(
          actions.includes("install") &&
            actions.includes("enable") &&
            actions.includes("disable") &&
            actions.includes("remove"),
        );
        console.log(`ok ${width} ${lang} ${theme}`);
        await context.close();
      }
} finally {
  await browser.close();
  files.close();
  host.close();
}
