// Built-page workflows against a shared, strict-version bridge. This exercises
// two independent browser clients competing for the same day, not just mocks
// that accept every write. Screenshots cover both languages/themes and mobile.
import assert from "node:assert/strict";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
const here = path.dirname(fileURLToPath(import.meta.url)), root = path.resolve(here, "../internal/templates/official/scheduling"), out = process.env.SCHEDULING_SHOTS || "/tmp/scheduling-shots";
fs.mkdirSync(out, { recursive: true });
const sample = JSON.parse(fs.readFileSync(path.join(here, "src/scheduling/public/sample.json"), "utf8"));
const recs = new Map(); let seq = 0, events = [], race = false, pending = [];
function reset(empty = false) { recs.clear(); seq = 0; events = []; for (const [c, rows] of Object.entries(empty ? {} : sample)) for (const { id, ...data } of rows) recs.set(c + "/" + id, { collection: c, id, data, version: 1 }); }
function put(body, res) {
  const { c, data, o } = body, key = c + "/" + o.id, old = recs.get(key);
  if (o.ifVersion !== (old?.version || 0)) { res.writeHead(409, { "Content-Type": "application/json" }); res.end(JSON.stringify({ code: "conflict", message: "Someone changed this. Choose again.", latest: old })); return; }
  const row = { collection: c, id: o.id, data, version: (old?.version || 0) + 1 };
  recs.set(key, row); events.push({ seq: ++seq, collection: c, record: o.id, op: "put", data, version: row.version, by: "tester" });
  res.setHeader("Content-Type", "application/json"); res.end(JSON.stringify(row));
}
const bridge = `(() => {
  const request = async (url, options) => { const r = await fetch(url, options); const v = await r.json(); if (!r.ok) throw v; return v; };
  window.__downloads = [];
  window.myco = {features:[], ready: Promise.resolve({page:{id:'pg_test',name:'Scheduling'},me:{name:'tester'}}), data:{
    list: c => request('/data?c='+encodeURIComponent(c)),
    put: (c,data,o) => request('/data',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({c,data,o})}),
    on: f => { let seen = 0, active = true; const t = setInterval(async () => { try { const evs = await request('/events?since='+seen); for(const e of evs) { seen=Math.max(seen,e.seq); if(active) f(e); } } catch {} },100); return () => {active=false;clearInterval(t)}; }
  }, download: async (name, content, type) => {window.__downloads.push({name,content,type});return {path:'~/Downloads/'+name}}};
  Date.now = () => Date.parse('2026-11-22T00:00:00Z');
})();`;
const server = http.createServer(async (req, res) => {
  const u = new URL(req.url, "http://test");
  if (u.pathname === "/bridge.js") { res.setHeader("Content-Type", "text/javascript"); res.end(bridge); return; }
  if (u.pathname === "/events") { res.setHeader("Content-Type", "application/json"); res.end(JSON.stringify(events.filter(e => e.seq > Number(u.searchParams.get("since"))))); return; }
  if (u.pathname === "/data" && req.method === "GET") { res.setHeader("Content-Type", "application/json"); res.end(JSON.stringify({ records: [...recs.values()].filter(r => r.collection === u.searchParams.get("c")), seq })); return; }
  if (u.pathname === "/data" && req.method === "POST") {
    let source = ""; for await (const chunk of req) source += chunk;
    const body = JSON.parse(source);
    if (race && body.c === "days") { pending.push([body, res]); if (pending.length === 2) { race = false; for (const [b, r] of pending) put(b, r); pending = []; } return; }
    put(body, res); return;
  }
  if (u.pathname === "/favicon.ico") { res.writeHead(204); res.end(); return; }
  const f = path.resolve(root, "." + (u.pathname === "/" ? "/index.html" : u.pathname));
  if (!f.startsWith(root + path.sep) || !fs.existsSync(f)) { res.writeHead(404); res.end(); return; }
  res.setHeader("Content-Type", ({ ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".webp": "image/webp" })[path.extname(f)] || "application/octet-stream");
  const content = fs.readFileSync(f); res.end(f.endsWith(".html") ? String(content).replace("<head>", '<head><script src="/bridge.js"></script>') : content);
});
await new Promise(r => server.listen(0, "127.0.0.1", r));
const playwright = createRequire(path.join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "noop.js"))("playwright");
const browser = await playwright.chromium.launch({ headless: true, executablePath: process.env.UI_SMOKE_EXECUTABLE, args: ["--no-sandbox"] });
const base = `http://127.0.0.1:${server.address().port}/`;
try {
  for (const [lang, theme, width] of [["zh","light",1400],["en","dark",1400],["zh","dark",760],["en","light",760],["en","dark",393],["zh","light",393]]) {
    reset();
    const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: theme, isMobile: width < 400, hasTouch: width < 400 }), p = await context.newPage(), errors = [];
    p.on("pageerror", e => errors.push(e.message)); p.setDefaultTimeout(10000);
    const L = (zh, en) => lang === "zh" ? zh : en;
    await p.goto(base + "?myco_lang=" + lang); await p.locator(".sc-type").first().waitFor();
    assert.equal(await p.locator(".sc-type").count(), 3);
    for (const actions of await p.locator(".sc-type-actions").all()) {
      const boxes = await actions.locator("button").evaluateAll(buttons => buttons.map(b => { const r = b.getBoundingClientRect(); return { y:r.y, height:r.height }; }));
      assert.equal(boxes.length, 3);
      assert(boxes.every(b => Math.abs(b.y - boxes[0].y) < 1 && b.height === 30), "Edit, Book and Pause share one row and control height");
    }
    await p.screenshot({ path: `${out}/${lang}-${theme}-${width}-types.png` });
    await p.getByRole("textbox", { name: L("搜索预约类型", "Search event types") }).fill("no such event");
    assert.equal(await p.locator(".sc-type").count(), 0);
    await p.getByRole("textbox", { name: L("搜索预约类型", "Search event types") }).fill("");
    await p.locator(".sc-type-actions .btn").nth(1).click();
    await p.locator(".sc-slot").first().waitFor();
    await p.screenshot({ path: `${out}/${lang}-${theme}-${width}-booking.png` });
    assert.equal(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "No horizontal overflow");
    if (width < 400) {
      // Real touch scroll: body is intentionally fixed; the workspace must scroll.
      const box = await p.locator(".sc-booking-wrap").boundingBox();
      const cdp = await context.newCDPSession(p);
      await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: box.x + 100, y: box.y + Math.min(box.height - 30, 600) }] });
      await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x: box.x + 100, y: box.y + 50 }] });
      await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
      await p.waitForFunction(() => document.querySelector(".sc-booking-wrap").scrollTop > 0);
    }
    await p.locator(".sc-slot").first().click();
    await p.getByLabel(L("你的姓名", "Your name"), { exact: true }).fill("Test Guest");
    await p.getByLabel(L("邮箱", "Email"), { exact: true }).fill("guest@example.com");
    await p.getByRole("button", { name: L("确认预约", "Confirm booking"), exact: true }).click();
    await p.getByRole("heading", { name: L("预约已确认", "Booking confirmed") }).waitFor();
    assert.equal([...recs.values()].filter(r => r.collection === "days").flatMap(r => r.data.entries).filter(b => b.name === "Test Guest").length, 1);
    await p.getByRole("button", { name: L("添加到日历", "Add to calendar") }).click();
    const dl = await p.evaluate(() => window.__downloads[0]); assert(dl.content.includes("BEGIN:VCALENDAR")); assert(dl.content.includes("guest@example.com"));
    await p.locator("nav button").nth(1).click();
    await p.locator(".sc-booking-row", { hasText: "Test Guest" }).click();
    await p.getByRole("button", { name: L("取消预约", "Cancel booking"), exact: true }).click();
    await p.locator("dialog").waitFor({ state: "detached" });
    await p.getByRole("button", { name: L("撤销", "Undo"), exact: true }).click();
    await p.locator(".sc-booking-row", { hasText: "Test Guest" }).waitFor();
    await p.locator("nav button").nth(2).click();
    await p.locator(".sc-week-row").first().waitFor();
    const starts = p.getByLabel(L("开始时间", "Start time"), { exact: true });
    await starts.first().fill("18:00");
    await p.getByRole("button", { name: L("保存", "Save"), exact: true }).click();
    await p.locator(".sc-availability .sc-error").filter({ hasText: /overlap|重叠/ }).waitFor();
    await starts.first().fill("09:00");
    await p.getByRole("button", { name: L("保存", "Save"), exact: true }).click();
    await p.locator(".sc-success").waitFor();
    await p.screenshot({ path: `${out}/${lang}-${theme}-${width}-availability.png` });
    assert.deepEqual(errors, []);
    await context.close();
  }
  // Actual concurrent clients read the same empty day, then race their CAS puts.
  reset();
  const clients = await Promise.all([browser.newPage(), browser.newPage()]);
  await Promise.all(clients.map(async (p, i) => {
    await p.goto(base + "?myco_lang=en"); await p.locator(".sc-type-actions .btn").nth(1).click(); await p.locator(".sc-slot").first().click();
    await p.getByLabel("Your name", { exact: true }).fill("Concurrent " + i); await p.getByLabel("Email", { exact: true }).fill(`guest${i}@example.com`);
  }));
  race = true;
  await Promise.all(clients.map(p => p.getByRole("button", { name: "Confirm booking", exact: true }).click()));
  await Promise.all(clients.map(p => p.waitForFunction(() => document.querySelector(".sc-confirmation") || document.querySelector(".sc-slot-panel [role=alert]"))));
  assert.equal((await Promise.all(clients.map(p => p.locator(".sc-confirmation").count()))).reduce((a,b) => a+b), 1, "Only one client confirms");
  assert.equal([...recs.values()].filter(r => r.collection === "days").flatMap(r => r.data.entries).filter(b => b.name.startsWith("Concurrent")).length, 1);
  await Promise.all(clients.map(p => p.close()));
  // Empty-page setup, creation and peer updates arrive through the event feed.
  reset(true);
  const p = await browser.newPage(); await p.goto(base + "?myco_lang=en");
  await p.getByRole("button", { name: "Set up booking profile", exact: true }).click();
  await p.getByLabel("Host", { exact: true }).fill("New Host"); await p.getByRole("button", { name: "Save", exact: true }).click();
  await p.locator("dialog").waitFor({ state: "detached" });
  await p.locator(".sc-top").getByRole("button", { name: "New event type", exact: true }).click();
  await p.getByLabel("Event name", { exact: true }).fill("New consultation"); await p.getByRole("button", { name: "Save", exact: true }).click();
  await p.locator(".sc-type", { hasText: "New consultation" }).waitFor();
  const row = [...recs.values()].find(r => r.collection === "types");
  const r = await fetch(base + "data", { method: "POST", body: JSON.stringify({ c: "types", o: { id: row.id, ifVersion: row.version }, data: { ...row.data, title: "Agent updated title" } }) });
  assert.equal(r.status, 200); await p.locator(".sc-type", { hasText: "Agent updated title" }).waitFor();
  // An open editor must not overwrite a newer agent edit with its old draft.
  await p.locator(".sc-type-actions .btn").first().click();
  await p.getByLabel("Event name", {exact:true}).fill("Stale user title");
  const newer = recs.get("types/" + row.id);
  const updated = await fetch(base + "data", {method:"POST",body:JSON.stringify({c:"types",o:{id:row.id,ifVersion:newer.version},data:{...newer.data,title:"Latest agent title"}})});
  assert.equal(updated.status,200);
  await p.getByRole("button",{name:"Save",exact:true}).click();
  await p.locator("dialog [role=alert]").waitFor();
  assert.equal(recs.get("types/"+row.id).data.title,"Latest agent title");
  await p.getByRole("button",{name:"Close",exact:true}).click();
  // A bridge without download support still offers a browser calendar file.
  await p.locator(".sc-type-actions .btn").nth(1).click();
  await p.locator(".sc-slot").first().click();
  await p.getByLabel("Your name",{exact:true}).fill("Fallback Guest");
  await p.getByLabel("Email",{exact:true}).fill("fallback@example.com");
  await p.getByRole("button",{name:"Confirm booking",exact:true}).click();
  await p.getByRole("heading",{name:"Booking confirmed"}).waitFor();
  await p.evaluate(()=>{myco.download=async()=>{throw new Error("unsupported")}});
  const downloaded=p.waitForEvent("download");
  await p.getByRole("button",{name:"Add to calendar",exact:true}).click();
  assert.equal((await downloaded).suggestedFilename(),"appointments.ics");
  await p.close();
  console.log("PASS scheduling UI: setup/create, live peer edits, booking, calendar export, cancel/undo, invalid hours, concurrent CAS, languages/themes, 1400/760/393 and real touch scroll");
  console.log("Screenshots: " + out);
} finally { await browser.close(); await new Promise(r => server.close(r)); }
