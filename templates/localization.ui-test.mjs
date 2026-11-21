import { fileURLToPath } from "node:url";
// Exercise actual pagebridge.js across origins. The host's explicit theme is
// opposite the OS theme, so merely following prefers-color-scheme is not enough.
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const root = path.resolve(
    path.dirname(fileURLToPath(import.meta.url)),
    "../internal/templates/official",
  ),
  shots = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "localization-shots");
fs.mkdirSync(shots, { recursive: true });
const bridge = fs.readFileSync(
  path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../internal/console/pagebridge.js"),
  "utf8",
);
const serve = (handler) =>
  new Promise((r) => {
    const s = http.createServer(handler);
    s.listen(0, "127.0.0.1", () => r(s));
  });
const frames = await serve((req, res) => {
  const u = new URL(req.url, "http://x");
  if (u.pathname === "/bridge.js") {
    res.setHeader("Content-Type", "text/javascript");
    res.end(bridge);
    return;
  }
  const file = path.resolve(root, "." + u.pathname);
  if (!file.startsWith(root + path.sep) || !fs.existsSync(file)) {
    res.writeHead(404).end();
    return;
  }
  res.setHeader(
    "Content-Type",
    {
      ".html": "text/html",
      ".js": "text/javascript",
      ".css": "text/css",
      ".json": "application/json",
      ".woff2": "font/woff2",
      ".svg": "image/svg+xml",
    }[path.extname(file)] || "application/octet-stream",
  );
  let raw = fs.readFileSync(file);
  if (file.endsWith(".html"))
    raw = String(raw).replace(
      "<head>",
      '<head><script src="/bridge.js"></script>',
    );
  res.end(raw);
});
const frameOrigin = `http://127.0.0.1:${frames.address().port}`;
const host = await serve((req, res) => {
  const q = new URL(req.url, "http://x").searchParams,
    slug = q.get("app"),
    language = q.get("lang") || "en",
    theme = q.get("theme") || "light";
  const sample = JSON.parse(
    fs.readFileSync(path.join(root, slug, "sample.json"), "utf8"),
  );
  const data = {};
  for (const [c, list] of Object.entries(sample))
    for (const [r, item] of list.entries()) {
      const { id, ...v } = item;
      const key = id || `r${r}`;
      data[c + "/" + key] = { collection: c, id: key, data: v, version: 1 };
    }
  res.setHeader("Content-Type", "text/html; charset=utf-8");
  res.end(`<!doctype html><html><body style="margin:0"><iframe id="app" style="border:0;width:100vw;height:100vh;color-scheme:${theme}" src="${frameOrigin}/${slug}/index.html?myco_lang=${language}" sandbox="allow-scripts allow-same-origin allow-forms allow-downloads allow-modals"></iframe><script>
 const preferences={language:${JSON.stringify(language)},theme:${JSON.stringify(theme)}};
 const records=${JSON.stringify(data).replaceAll("<", "\\u003c")};let seq=0;window.__writes=0;
 const f=document.getElementById('app');window.__setTheme=theme=>{preferences.theme=theme;f.style.colorScheme=theme;f.contentWindow.postMessage({myco:'preferences',preferences},${JSON.stringify(frameOrigin)})};
 window.addEventListener('message',async e=>{if(e.source!==f.contentWindow||e.origin!==${JSON.stringify(frameOrigin)}||e.data?.myco!=='call')return;const d=e.data,a=d.args||{};let value,error,code;
 switch(d.method){case'ready':value={page:{id:'pg_test',name:'User title 用户标题'},me:{name:'test'},state:null,version:1,preferences};break;
 case'data.list':value={records:Object.values(records).filter(r=>!a.collection||r.collection===a.collection),seq};break;
 case'data.watch':value=true;break;
 case'data.put':{window.__writes++;const id=a.id||'generated'+seq;const r={collection:a.collection,id,data:a.data,version:(records[a.collection+'/'+id]?.version||0)+1};records[a.collection+'/'+id]=r;seq++;value=r;break}
 case'data.remove':window.__writes++;delete records[a.collection+'/'+a.id];value=true;break;
 case'remote.call':value=a.action==='devices'?[]:{protocol:1,version:'0.1.0',installed:false,supported:false,running:false};break;
 case'os.list':value={os:'linux',arch:'x64',caps:[]};break;
 case'os.call':error='This capability is unavailable in this test';code='unsupported';break;
 case'os.start':error='This capability is unavailable in this test';code='unsupported';break;
 case'update':window.__writes++;value={state:a.state,version:2,ok:true};break;
 case'storage.get':value=null;break;
 case'download':value={path:'Downloads/test'};break;
 default:value=true;
 }f.contentWindow.postMessage({myco:'result',id:d.id,value,error,code},e.origin)});
 </script></body></html>`);
});
let chromium;
try {
  ({ chromium } = createRequire(import.meta.url)("playwright"));
} catch {
  const g = execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim();
  ({ chromium } = createRequire(path.join(g, "noop.js"))("playwright"));
}
const browser = await chromium.launch(
  process.env.UI_SMOKE_EXECUTABLE
    ? { executablePath: process.env.UI_SMOKE_EXECUTABLE }
    : {},
);
const apps = process.argv.slice(2).filter((a) => !a.startsWith("--"));
const all = apps.length ? apps : fs.readdirSync(root);
const results = [];
try {
  for (const width of [1200, 760])
    for (const language of ["en", "zh"])
      for (const theme of ["light", "dark"]) {
        const context = await browser.newContext({
          viewport: { width, height: 900 },
          colorScheme: theme === "dark" ? "light" : "dark",
        });
        for (const app of all) {
          const page = await context.newPage(),
            errors = [];
          page.on("pageerror", (e) => errors.push(e.message));
          await page.goto(
            `http://127.0.0.1:${host.address().port}/?app=${app}&lang=${language}&theme=${theme}`,
          );
          const frame = page.frameLocator("#app");
          await frame
            .locator("#root")
            .locator(":scope > *")
            .first()
            .waitFor({ timeout: 15000 });
          await page.waitForTimeout(200);
          assert.equal(
            await frame.locator("html").getAttribute("lang"),
            language === "en" ? "en" : "zh-CN",
            app + " document language",
          );
          const actual = await frame
            .locator("html")
            .getAttribute("data-myco-theme");
          assert.equal(actual, theme, app + " follows App instead of OS");
          const bg = await frame
            .locator("body")
            .evaluate((n) => getComputedStyle(n).backgroundColor);
          if (bg.startsWith("rgb(")) {
            const values = bg.match(/\d+/g).map(Number);
            assert.equal(
              values.slice(0, 3).reduce((a, b) => a + b, 0) / 3 < 128,
              theme === "dark",
              app + " CSS follows explicit App theme: " + bg,
            );
          }
          assert(
            !(await page
              .frames()[1]
              .evaluate(
                () => document.documentElement.scrollWidth > innerWidth + 2,
              )),
            app + " page overflow",
          );
          const writes = await page.evaluate(() => window.__writes);
          await page.evaluate(() => window.__setTheme("light"));
          await page.waitForTimeout(80);
          assert.equal(
            await page
              .frames()[1]
              .evaluate(() => window.myco.preferences.theme),
            "light",
          );
          await page.evaluate(() => window.__setTheme("dark"));
          await page.waitForTimeout(80);
          assert.equal(
            await page
              .frames()[1]
              .evaluate(() => window.myco.preferences.theme),
            "dark",
          );
          assert.equal(
            await page.evaluate(() => window.__writes),
            writes,
            app + " appearance must not change records",
          );
          if (language === "en" && width === 1200) {
            const chinese = await frame
              .locator("button,label")
              .evaluateAll((nodes) =>
                nodes
                  .map((n) => n.innerText)
                  .filter((t) => /[\u3400-\u9fff]/.test(t))
                  .slice(0, 15),
              );
            results.push({ app, chinese });
          }
          if (
            language === "en" &&
            ["devtools", "design-studio", "proxy-console", "todo"].includes(app)
          )
            await page.screenshot({
              path: path.join(
                shots,
                `${app}-${width}-${language}-${theme}.png`,
              ),
            });
          assert.deepEqual(errors, [], app + " runtime errors");
          await page.close();
          console.log("PASS", app, width, language, theme + " / opposite OS");
        }
        await context.close();
      }
  fs.writeFileSync(
    path.join(shots, "remaining-chinese.json"),
    JSON.stringify(results, null, 2),
  );
} finally {
  await browser.close();
  await Promise.all([
    new Promise((r) => frames.close(r)),
    new Promise((r) => host.close(r)),
  ]);
}
