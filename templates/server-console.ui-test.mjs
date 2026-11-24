// Isolated visual/interaction preview, explicitly synthetic; this does not
// claim a real SSH connection or agent authorization has been implemented.
import assert from "node:assert/strict";
import { build } from "esbuild";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";
const require = createRequire(
  path.join(
    execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(),
    "noop.js",
  ),
);
const { chromium } = require("playwright");
const out = process.env.SERVER_SHOTS || "/tmp/server-console-shots";
fs.mkdirSync(out, { recursive: true });
const result = await build({
  stdin: {
    resolveDir: path.resolve("templates"),
    loader: "tsx",
    contents: `import React,{useState} from 'react';import{createRoot}from'react-dom/client';import{Shell}from'./src/server-console/shell';import{Dashboard}from'./src/server-console/dashboard';import{Resources}from'./src/server-console/resources';import'./src/server-console/server-console.css';
const profiles=[{id:'p1',name:'Production / sample',target:'deploy@example.test',port:22},{id:'p2',name:'Staging / sample',target:'ops@staging.test',port:2222}];
const data={hostname:'sample-web-01',os:'Ubuntu 24.04 LTS',kernel:'6.8.0-generic',arch:'x86_64',uptime:1843200,cpus:4,cpu_percent:23.4,memory_total:8589934592,memory_available:5368709120,swap_total:2147483648,swap_free:2147483648,load:[0.42,0.56,0.38],disks:[{mount:'/',total:107374182400,used:41943040000,available:65431142400}],sampled_at:'2026-01-01T00:00:00Z'};
function App(){const[view,setView]=useState('overview'),[selected,setSelected]=useState(profiles[0]),[output,setOutput]=useState('');return <Shell profiles={profiles} selected={selected} view={view} connected busy={false} onSelect={id=>setSelected(profiles.find(p=>p.id===id))} onView={setView} onAdd={()=>{}} onRefresh={()=>{}} onDisconnect={()=>{}}><p style={{fontSize:11,color:'#888',margin:'0 0 20px'}}>DESIGN PREVIEW · SYNTHETIC DATA · NO SSH CONNECTION</p>{view==='overview'?<Dashboard data={data} history={[15,28,24,18,22,35,27,23].map((v,i)=>({at:i,cpu:v,memory:37.5}))}/>:<Resources kind="service" data={{available:true,items:[{name:'nginx.service',description:'A web server',active:'active',sub:'running'},{name:'postgresql.service',description:'Database server',active:'inactive',sub:'dead'}]}} busy={false} onAction={(id,op)=>setOutput(id+':'+op)} onLogs={id=>setOutput(id+':logs')}/>}<output>{output}</output></Shell>};createRoot(document.getElementById('root')).render(<App/>);`,
  },
  outfile: "preview.js",
  bundle: true,
  write: false,
  format: "iife",
});
const js = result.outputFiles.find((f) => f.path.endsWith(".js")).text,
  css = result.outputFiles.find((f) => f.path.endsWith(".css")).text;
const server = http.createServer((req, res) => {
  res.setHeader("Content-Type", "text/html");
  res.end(
    '<html><meta name="viewport" content="width=device-width, initial-scale=1"><style>body{margin:0;font-family:system-ui}' +
      css +
      '</style><div id="root"></div><script>' +
      js +
      "</script></html>",
  );
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const browser = await chromium.launch();
try {
  for (const [lang, theme, width] of [
    ["zh", "light", 1200],
    ["en", "dark", 1200],
    ["zh", "dark", 760],
    ["en", "light", 393],
  ]) {
    const c = await browser.newContext({
      viewport: { width, height: 820 },
      colorScheme: theme,
    });
    const p = await c.newPage();
    await p.goto(
      "http://127.0.0.1:" + server.address().port + "/?myco_lang=" + lang,
    );
    await p.locator(".sv-metrics").waitFor();
    if (lang === "en")
      assert.equal(
        /[\u4e00-\u9fff]/.test(await p.locator(".sv-app").innerText()),
        false,
        "English overview must contain no untranslated UI",
      );
    assert.ok(
      (await p.evaluate(() => document.documentElement.scrollWidth)) <= width,
    );
    await p.screenshot({
      path: path.join(out, lang + "-" + theme + "-" + width + ".png"),
      fullPage: true,
    });
    await p.locator(".sv-sidebar nav button").nth(1).click();
    await p
      .getByRole("button", {
        name: (lang === "zh" ? "重启" : "Restart") + " nginx.service",
        exact: true,
      })
      .click();
    assert.equal(
      await p.locator("output").textContent(),
      "nginx.service:restart",
    );
    await p.getByRole("textbox").fill("postgres");
    assert.equal(await p.locator("tbody tr").count(), 1);
    await c.close();
    console.log("PASS synthetic UI " + lang + "/" + theme + "/" + width);
  }
} finally {
  await browser.close();
  await new Promise((r) => server.close(r));
}
