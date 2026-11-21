// Dev-only stand-in for the App backend (cmd/myco-app). No dependencies.
//   node mock/app-server.mjs        (or: npm run dev:app)
// Serves the `app/...` contract with two hubs: one healthy, one unreachable.
// Writes mutate in-memory state and are lost on restart.
import http from "node:http";
import zlib from "node:zlib";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { showcase } from "./showcase.mjs";

const PORT = Number(process.env.APP_MOCK_PORT || 7781);
// MOCK_OS_URL: a real App backend for myco.os (see /app/os below).
const OS_URL = (process.env.MOCK_OS_URL || "").replace(/\/$/, "");
// MOCK_SHELL=mac reserves room for the macOS traffic lights; browser does not.
const SHELL = process.env.MOCK_SHELL === "browser" ? "browser" : "mac";
// MOCK_EMPTY=1 starts with no hubs, to see the welcome page.
const EMPTY = process.env.MOCK_EMPTY === "1";
// MOCK_PENDING_LINK is returned once by GET /app/pending-link.
let pendingLink = process.env.MOCK_PENDING_LINK || "";
const iso = (offsetSec = 0) => new Date(Date.now() - offsetSec * 1000).toISOString();
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// The models a computer's Codex lists (its models cache), as runners from
// v0.21.1 report them.
const CODEX_MODELS = [
  { id: "gpt-6.1-sol", name: "GPT-6.1 Sol", efforts: ["low", "medium", "high", "xhigh"] },
  { id: "gpt-6-sol", name: "GPT-6 Sol", efforts: ["low", "medium", "high", "xhigh"] },
  { id: "gpt-6.1-terra", name: "GPT-6.1 Terra", efforts: ["low", "medium", "high"] },
  { id: "gpt-6-luna", name: "GPT-6 Luna", efforts: ["minimal", "low", "medium"] },
];

// CLI metadata used when a session is spawned from the App.
const CLI_MODELS = {
  claude: "claude-opus-5-5", codex: "gpt-6-sol", pi: "pi-3", kiro: "kiro-2",
  cursor: "cursor-auto", gemini: "gemini-3-pro", opencode: "claude-opus-5-5",
  qoder: "qoder-auto", qwen: "qwen3-coder", kimi: "kimi-k3", goose: "goose", copilot: "copilot",
  auggie: "auggie", codebuddy: "codebuddy", kilo: "kilo", cline: "cline", grok: "grok-5",
};
const CLI_VERSIONS = {
  claude: "2.1.240", codex: "0.52.0", pi: "1.8.0", kiro: "0.4.2",
  cursor: "0.48.0", gemini: "0.12.0", opencode: "0.6.1",
  qoder: "0.3.0", qwen: "0.9.0", kimi: "1.2.0", goose: "1.9.0", copilot: "0.0.400",
  auggie: "0.8.0", codebuddy: "1.1.0", kilo: "1.0.0", cline: "1.0.0", grok: "0.2.0",
};

function makeAgent(over) {
  const base = {
    id: "ag_" + Math.random().toString(36).slice(2, 10),
    name: "agent",
    description: "",
    capabilities: [],
    labels: [],
    kind: "agent",
    binding: { harness: "claude", session_id: "sess_" + Math.random().toString(36).slice(2, 12) },
    delivery: { mode: "pull" },
    allow_from: [],
    online: true,
    created_at: iso(86_400 * 12),
    last_seen_at: iso(20),
  };
  return { ...base, ...over, binding: { ...base.binding, ...(over.binding || {}) }, delivery: { ...base.delivery, ...(over.delivery || {}) } };
}

let msgSeq = 0;
// Files the app uploaded to attach (id -> attachment).
const uploads = new Map();
// What was uploaded, by file id (served back to the app).
const uploadBytes = new Map();
// Pictures agents use as faces, by "hub/name".
const avatarPics = new Map();
// Times the page asked the "desktop app" to listen, in which language, and
// what each listening hears.
let dictations = 0;
let translateCfg = { provider: "deepl", base_url: "", model: "" };
const translateKeys = {};
let translations = 0;
let lastDictationLang = "";
const hearings = new Map();

// A real PNG (a soft diagonal gradient with a band), for pictures the mock's
// agents send back: the app shows them as thumbnails.
function demoPng(w = 640, h = 400) {
  const raw = Buffer.alloc((w * 3 + 1) * h);
  for (let y = 0; y < h; y++) {
    raw[y * (w * 3 + 1)] = 0;
    for (let x = 0; x < w; x++) {
      const o = y * (w * 3 + 1) + 1 + x * 3;
      const t = (x + y) / (w + h);
      const band = y > h * 0.42 && y < h * 0.58 ? 30 : 0;
      raw[o] = Math.round(40 + 60 * t) + band;
      raw[o + 1] = Math.round(140 + 60 * (1 - t)) + band;
      raw[o + 2] = Math.round(110 + 80 * t);
    }
  }
  const crc = (buf) => {
    let c = ~0;
    for (const b of buf) { c ^= b; for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1)); }
    return (~c) >>> 0;
  };
  const chunk = (type, data) => {
    const len = Buffer.alloc(4); len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type), data]);
    const c = Buffer.alloc(4); c.writeUInt32BE(crc(td));
    return Buffer.concat([len, td, c]);
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[9] = 2;
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk("IHDR", ihdr), chunk("IDAT", zlib.deflateSync(raw)), chunk("IEND", Buffer.alloc(0))]);
}
const DEMO_PNG = demoPng();
function makeMessage(over) {
  msgSeq += 1;
  const base = {
    id: `msg_${msgSeq}`,
    from: "glenn", to: "agent", thread_id: "th_demo", type: "task",
    body: "", status: "delivered", attempts: 1,
    created_at: iso(120), updated_at: iso(60),
  };
  return { ...base, ...over };
}

const studioThreadRaw = [
  makeMessage({ thread_id: "th_release", from: "glenn", to: "pm-codex", type: "task", body: "发布 v0.16.8：改 VERSION，跑 make check，打 tag 并推送。", created_at: iso(900), updated_at: iso(900) }),
  makeMessage({ thread_id: "th_release", from: "pm-codex", to: "glenn", type: "progress", body: "已改好 VERSION，正在跑 make check（单测、smoke）。", created_at: iso(780), updated_at: iso(780) }),
  makeMessage({ thread_id: "th_release", from: "pm-codex", to: "glenn", type: "reply", body: "release/v0.16.8 分支检查全部通过，等你确认后打 tag。", created_at: iso(600), updated_at: iso(600) }),
  makeMessage({ thread_id: "th_release", from: "glenn", to: "pm-codex", type: "task", body: "可以，打 tag 吧。", created_at: iso(300), updated_at: iso(300) }),
];
// A coordinator giving another node several tasks in a row: it answers
// the first (after two progress updates), is on the second, and the third
// waits behind it. The answer comes after the third task, so it quotes the
// task it answers.
const coordTask = (body, ago, status) => makeMessage({ thread_id: "th_coord", from: "release-cloud", to: "review-cloud", type: "task", body, status, created_at: iso(ago), updated_at: iso(ago) });
const coordT1 = coordTask("审查 PR 128（节点别名）：接口、权限和测试，能合就说能合。", 1500, "done");
const coordT2 = coordTask("再看一下 multihub 分支：hub 拒绝 token 之后，缓存还会不会被写回去。", 1460, "acked");
const coordT3 = coordTask("最后写一段 v0.18.40 的发布说明草稿，放进项目资料。", 1420, "queued");
const coordSays = (type, task, body, ago) => makeMessage({ thread_id: "th_coord", from: "review-cloud", to: "release-cloud", type, reply_to: task.id, status: "done", body, created_at: iso(ago), updated_at: iso(ago) });
// The coordinator handing work out in 官网改版, after it was asked for the
// release notes: the docs page is done, the deploy check is under way.
const dispatchDocs = [
  makeMessage({ thread_id: "th_dispatch_docs", project: "website", from: "pm-codex", to: "docs-claude", type: "task", status: "done", body: "把文档站首页的「升级注意事项」按 v0.16.8 的发布说明更新一下。", created_at: iso(6900), updated_at: iso(6900) }),
  makeMessage({ thread_id: "th_dispatch_docs", project: "website", from: "docs-claude", to: "pm-codex", type: "reply", status: "done", body: "改好了：`docs/upgrade.md` 加了 v0.16.8 一节，首页的链接也指过去了。", created_at: iso(6500), updated_at: iso(6500) }),
];
const dispatchRelease = [
  makeMessage({ thread_id: "th_dispatch_release", project: "website", from: "pm-codex", to: "release-cloud", type: "task", status: "acked", body: "在预发环境装一遍 v0.16.8，确认下载页的链接都能下到对的文件。", created_at: iso(6600), updated_at: iso(240) }),
  makeMessage({ thread_id: "th_dispatch_release", project: "website", from: "release-cloud", to: "pm-codex", type: "progress", status: "done", body: "macOS 和 Linux 的包都下好了，正在核对 Windows 的。", created_at: iso(240), updated_at: iso(240) }),
];
// A member writing to the coordinator on its own: a note, and the
// coordinator's answer (a handoff card on the home and in the member's chat).
const memberNote = [
  makeMessage({ thread_id: "th_member_note", project: "website", from: "docs-claude", to: "pm-codex", type: "note", status: "done", body: "先别让 release 装：升级说明里 Windows 的安装步骤还没改完，我在补。", created_at: iso(6300), updated_at: iso(6300) }),
  makeMessage({ thread_id: "th_member_note", project: "website", from: "pm-codex", to: "docs-claude", type: "note", status: "done", body: "好，等你补完我再让 release 那边装。", created_at: iso(6250), updated_at: iso(6250) }),
];
const coordThread = [
  coordT1, coordT2, coordT3,
  coordSays("progress", coordT1, "已拉下 PR 128，正在看 handler 和权限检查。", 1300),
  coordSays("progress", coordT1, "别名上限没写进错误提示，正在核对测试。", 1120),
  coordSays("reply", coordT1, "PR 128 可以合。\n\n- 别名上限（3 个）已写进错误提示和 `myco schema`\n- 抢别人名字、超上限、非主人设别名都有测试\n- `make vet`、`make test` 通过", 900),
  coordSays("progress", coordT2, "正在复现：hub 拒绝 token 后再刷新一次缓存。", 380),
];

// MOCK_E2E: the console conversation is end-to-end encrypted, one message
// in it was sealed for someone else.
const studioThread = process.env.MOCK_E2E
  ? studioThreadRaw.map((m, i) => i === 1 ? { ...m, e2e: true, unreadable: "not for this device", body: "[encrypted message]" } : { ...m, e2e: true })
  : studioThreadRaw;

// The App talks as "@console:<device>"; a thread counts as a conversation
// only when it has one of these messages.
const CONSOLE = `@console:${"glenn-mac"}`;

// A task the node picked up and then went quiet on: long enough ago that its
// result may never come back. Kept in the node's current work and its chat,
// so the App can say "may be stuck" and offer to send it again.
const stuckTask = makeMessage({
  id: "msg_stuck_legacy",
  thread_id: "th_console_stuck",
  from: CONSOLE,
  to: "legacy-codex",
  type: "task",
  status: "acked",
  body: "把旧版 runner 的日志整理到 docs/legacy.md。",
  created_at: iso(46 * 60),
  updated_at: iso(45 * 60),
  active_at: iso(45 * 60),
});
// A command another agent sent to the exec node on Studio Mac: picked up and
// quiet since, so the computer reads as working while nothing is moving. It
// is not in a thread you take part in (the terminal shows only your own
// commands): the amber bar above the terminal is where it is seen and stopped.
const machineStuckTask = makeMessage({
  id: "msg_stuck_mac_exec",
  thread_id: "th_pm_mac_exec_stuck",
  from: "pm-codex",
  to: "mac-exec",
  type: "task",
  status: "acked",
  body: "cd ~/code/myco && make app VERSION=v0.18.15-preview",
  created_at: iso(65 * 60),
  updated_at: iso(60 * 60),
  active_at: iso(60 * 60),
});
// What it reported on that task before it went quiet: it stays in the run
// (above the line saying it may be stuck), and the quiet counts from it.
const stuckProgress = makeMessage({
  thread_id: "th_console_stuck", from: "legacy-codex", to: CONSOLE, type: "progress", status: "done",
  reply_to: "msg_stuck_legacy", body: "整理到一半：已经列出了 3 种旧版 runner 的日志格式。",
  created_at: iso(40 * 60), updated_at: iso(40 * 60),
});
// The App's proxy setting (/app/proxy).
let proxySetting = { mode: "system" };
let proxyPassword = "";
// Links the App asked to open in the browser (/app/open-url).
const openedLinks = [];
// Pages agents published (/app/hubs/{id}/pages), and the keys the page
// panel opened them under (served on this port at /p/<key>/…).
const checklistPage = `<!doctype html><html lang="zh"><head><meta charset="utf-8"><title>上线清单</title><link rel="stylesheet" href="style.css"></head>
<body><main><h1>上线清单</h1><p class="sub" id="who">正在连上 myco…</p><ul id="list"></ul>
<p><button id="hand">交给 agent</button> <span id="note"></span></p></main>
<script src="app.js"></script></body></html>`;
const checklistCss = "body{font:15px/1.6 -apple-system,system-ui,sans-serif;margin:0;color:#1c1c21;background:#fff}main{max-width:560px;margin:0 auto;padding:28px 24px}h1{margin:0;font-size:22px}.sub{color:#85858d;margin:4px 0 16px}ul{list-style:none;padding:0}li{padding:6px 0}label{display:flex;gap:8px;align-items:center}li.done span{color:#85858d;text-decoration:line-through}button{font:inherit;padding:4px 12px;border-radius:7px;border:1px solid #d0d0d6;background:#fff}#note{color:#2f8a68;font-size:13px}";
const checklistJs = `const items=["升级说明已更新","Windows 安装包已签名","预发环境装过一遍","发布说明已发"];
function draw(state){const done=(state&&state.done)||[];document.getElementById("list").innerHTML=items.map((t,i)=>'<li class="'+(done.includes(i)?"done":"")+'"><label><input type="checkbox" data-i="'+i+'"'+(done.includes(i)?" checked":"")+'><span>'+t+'</span></label></li>').join("");}
draw(null);
myco.ready.then((c)=>{document.getElementById("who").textContent=(c.me&&c.me.name?c.me.name+" · ":"")+"状态第 "+c.version+" 版";draw(c.state);},()=>{document.getElementById("who").textContent="（不在 myco App 里：只能看）";});
myco.on("change",(c)=>{draw(c.state);document.getElementById("who").textContent=(c.by?c.by+" 改了 · ":"")+"状态第 "+c.version+" 版";});
document.addEventListener("change",async(e)=>{const i=Number(e.target.dataset.i);const done=new Set((myco.state&&myco.state.done)||[]);e.target.checked?done.add(i):done.delete(i);const r=await myco.update({done:[...done].sort()});draw(r.state);document.getElementById("who").textContent=(r.ok?"已保存":"别人先改了，已刷新")+" · 状态第 "+r.version+" 版";});
document.getElementById("hand").addEventListener("click",async()=>{try{const n=((myco.state&&myco.state.done)||[]).length;await myco.send("清单勾完了 "+n+" 项，剩下的交给你。");document.getElementById("note").textContent="已发给 agent";}catch(err){document.getElementById("note").textContent=err.message;}});`;
const livePage = `<!doctype html><html lang="zh"><head><meta charset="utf-8"><title>下载页</title><style>body{font:15px/1.6 -apple-system,system-ui,sans-serif;margin:0;color:#1c1c21;background:#fff}main{max-width:560px;margin:0 auto;padding:32px 24px}h1{margin:0 0 6px;font-size:24px}.dl{display:flex;gap:8px;margin-top:18px}.dl a{padding:8px 14px;border-radius:9px;background:#26262c;color:#fff;text-decoration:none;font-size:14px}.tag{font-size:12px;color:#b45309}</style></head>
<body><main><p class="tag">vite dev server · localhost:5173</p><h1>下载 myco</h1><p>Mac、Windows、Linux 都有。</p><div class="dl"><a href="#mac">macOS</a><a href="#win">Windows</a><a href="#linux">Linux</a></div></main></body></html>`;
// A page made from a template: the starter myco template init writes (an
// editable table over its records), as the CLI embeds it.
const kit = (f) => readFileSync(new URL(`../../internal/cli/templatekit/${f}`, import.meta.url), "utf8")
  .replaceAll("{{ID}}", "release/progress").replaceAll("{{NAME}}", "发布进度表");
const tableFiles = { "index.html": kit("index.html"), "myco.json": kit("myco.json") };
// The template library: myco's own (read from internal/templates, as the hub
// embeds them) and the workspace's (the starter above). Using one makes a
// page with its files (madeFiles) and, if asked, its sample data.
const officialRoot = new URL("../../internal/templates/official/", import.meta.url);
const officialFiles = (name) => {
  const out = {};
  const walk = (dir, prefix) => {
    for (const e of readdirSync(new URL(dir, officialRoot), { withFileTypes: true })) {
      if (e.isDirectory()) walk(`${dir}${e.name}/`, `${prefix}${e.name}/`);
      else out[`${prefix}${e.name}`] = /\.(html|js|css|json|md)$/.test(e.name) ? readFileSync(new URL(`${dir}${e.name}`, officialRoot), "utf8") : readFileSync(new URL(`${dir}${e.name}`, officialRoot));
    }
  };
  walk(`${name}/`, "");
  return out;
};
const templateFiles = new Map();
// An app's screenshots, as the hub lists them: pictures right in shots/,
// x.webp with x.dark.webp (undefined when there are none).
const shotOK = (n) => /^shots\/[^/]+\.(png|jpe?g|webp)$/i.test(n);
const shotsOf = (names) => {
  const light = names.filter((n) => shotOK(n) && !/\.dark\.[a-z]+$/i.test(n)).sort();
  const out = light.map((src) => ({ src, ...(names.includes(src.replace(/(\.[a-z]+)$/i, ".dark$1")) ? { dark: src.replace(/(\.[a-z]+)$/i, ".dark$1") } : {}) }));
  return out.length ? out : undefined;
};
const officialOrder = ["table", "base", "doc", "slides", "meeting", "code", "issues", "devtools", "todo", "notes", "disk", "stocks", "proxy-console", "remote-desktop", "ledger", "habit", "design-studio", "whiteboard", "mindmap", "video", "photo", "gomoku", "sudoku"];
const mockTemplates = officialOrder.filter((name) => existsSync(new URL(`${name}/myco.json`, officialRoot))).map((name) => {
  const files = officialFiles(name);
  const m = JSON.parse(files["myco.json"]);
  const id = `tp_myco_${name}`;
  templateFiles.set(id, files);
  return { id, slug: m.id, name: m.name, description: m.description, locales: files["locales.json"] ? JSON.parse(files["locales.json"]) : undefined, kind: m.kind, category: m.category, icon: m.icon ? String(files[m.icon] || "") : undefined, version: m.version, publisher: "myco", official: true, can_manage: false,
    data: m.data, permissions: m.permissions, export: m.export, guide: files["agent.md"], versions: [{ version: m.version, files: Object.keys(files).length, size: 40000, by: "myco", at: iso(0) }],
    shots: shotsOf(Object.keys(files)), created_at: iso(0), updated_at: iso(0) };
});
{
  const files = { ...tableFiles, "agent.md": kit("agent.md"), "sample.json": kit("sample.json") };
  const m = JSON.parse(files["myco.json"]);
  templateFiles.set("tp_progress", files);
  mockTemplates.push({ id: "tp_progress", slug: m.id, name: m.name, description: "", kind: m.kind, version: m.version, publisher: "release-cloud", can_manage: true,
    data: m.data, permissions: m.permissions, export: m.export, guide: files["agent.md"], versions: [{ version: m.version, files: 4, size: 9000, by: "release-cloud", at: iso(17500) }],
    created_at: iso(17500), updated_at: iso(17500) });
}
const madeFiles = new Map();
// 这台电脑: a page trying myco.os (only with MOCK_OS_URL).
if (OS_URL) madeFiles.set("pg_os", { "index.html": readFileSync(new URL("./os-page.html", import.meta.url), "utf8") });
const pageFilesOf = (id, n) => madeFiles.has(id) ? madeFiles.get(id) : (id === "pg_live" ? { "index.html": livePage } : id === "pg_checklist" ? { "index.html": checklistPage, "style.css": checklistCss, "app.js": checklistJs } : id === "pg_table" ? tableFiles : pageFiles(n));
const pageFiles = (n) => ({
  "index.html": `<!doctype html><html lang="zh"><head><meta charset="utf-8"><title>v0.16.8 发布说明</title><link rel="stylesheet" href="style.css"></head>
<body><main><h1>v0.16.8 发布说明</h1><p class="sub">第 ${n} 版 · 由 Release (Claude 云端) 整理</p>
<div id="bars"></div>
<section><h2>新功能</h2><ul><li>对话页支持 Markdown 表格与代码块</li><li>本机会话一键接入</li></ul></section>
<section><h2>修复</h2><ul><li>runner 断线后重连</li></ul></section>
<p><a href="https://github.com/chenhg5/myco/releases" id="out">看完整的变更记录</a></p></main>
<script src="app.js"></script></body></html>`,
  "style.css": "body{font:15px/1.6 -apple-system,system-ui,sans-serif;margin:0;color:#1c1c21;background:#fff}main{max-width:640px;margin:0 auto;padding:28px 24px}h1{margin:0;font-size:24px}.sub{color:#85858d;margin:4px 0 20px}.bar{display:flex;align-items:center;gap:10px;margin:6px 0}.bar i{display:block;height:14px;border-radius:4px;background:#3a9a78}h2{font-size:16px;margin:22px 0 6px}a{color:#2f8a68}",
  "app.js": `const data=[["新功能",${n + 5}],["修复",${n + 2}],["文档",3]];document.getElementById("bars").innerHTML=data.map(([k,v])=>'<div class="bar"><span style="width:56px">'+k+'</span><i style="width:'+v*24+'px"></i><b>'+v+'</b></div>').join("");`,
});
const pages = new Map([["pg_release", {
  id: "pg_release", name: "v0.16.8 发布说明", project: "website", publisher: "release-cloud", version: 1, entry: "index.html",
  url: "http://192.168.1.20:7780/pages/pg_release", created_at: iso(19000), updated_at: iso(19000), can_manage: true,
  versions: [{ n: 1, entry: "index.html", files: 3, size: 2048, sha256: "a1", by: "release-cloud", at: iso(19000) }],
}]]);
pages.set("pg_checklist", {
  id: "pg_checklist", name: "上线清单", project: "website", publisher: "release-cloud", version: 1, entry: "index.html",
  url: "http://192.168.1.20:7780/pages/pg_checklist", created_at: iso(18500), updated_at: iso(18500), can_manage: true,
  versions: [{ n: 1, entry: "index.html", files: 3, size: 3100, sha256: "c1", by: "release-cloud", at: iso(18500) }],
});
if (OS_URL) pages.set("pg_os", {
  id: "pg_os", name: "这台电脑", project: "website", publisher: "release-cloud", version: 1, entry: "index.html",
  url: "http://192.168.1.20:7780/pages/pg_os", created_at: iso(100), updated_at: iso(100), can_manage: true,
  versions: [{ n: 1, entry: "index.html", files: 1, size: 4000, sha256: "o1", by: "release-cloud", at: iso(100) }],
});
pages.set("pg_live", {
  id: "pg_live", name: "下载页（实时）", project: "website", publisher: "release-cloud", version: 0, entry: "",
  url: "http://192.168.1.20:7780/pages/pg_live", created_at: iso(18200), updated_at: iso(18200), can_manage: true,
  versions: [], live: true, port: 5173, online: true,
});
pages.set("pg_table", {
  id: "pg_table", name: "发布进度表", project: "website", publisher: "release-cloud", version: 1, entry: "index.html",
  url: "http://192.168.1.20:7780/pages/pg_table", created_at: iso(17000), updated_at: iso(17000), can_manage: true,
  versions: [{ n: 1, entry: "index.html", files: 2, size: 9000, sha256: "t1", by: "release-cloud", at: iso(17000) }],
  template: { id: "tp_progress", slug: "release/progress", name: "发布进度表", version: "1.0.0", permissions: ["download"],
    data: { rows: { fields: JSON.parse(kit("myco.json")).data.rows.fields } } },
});
// Pages' records and change feeds (myco.data; hubs from v0.20.5).
const pageData = new Map();
const dataOf = (id) => {
  if (!pageData.has(id)) pageData.set(id, { records: [], changes: [], seq: 0 });
  return pageData.get(id);
};
const putRecord = (id, coll, rid, data, ifVersion, by) => {
  const d = dataOf(id);
  let rec = d.records.find((r) => r.collection === coll && r.id === rid);
  if (ifVersion !== null && ifVersion !== undefined && (rec?.version || 0) !== ifVersion) return { conflict: rec };
  if (!rec) { rec = { collection: coll, id: rid || `r_${(state.counter += 1)}`, data: {}, version: 0 }; d.records.push(rec); }
  Object.assign(rec, { data, version: rec.version + 1, updated_by: by, updated_at: iso(0) });
  d.changes.push({ seq: (d.seq += 1), collection: coll, record: rec.id, op: "put", data, version: rec.version, by, at: iso(0) });
  return { rec };
};
for (const r of JSON.parse(kit("sample.json")).rows) {
  const { id, ...data } = r;
  putRecord("pg_table", "rows", id, data, null, "release-cloud");
}
// Pages' shared states (the bridge's myco.update).
const pageStates = new Map([["pg_checklist", { state: { done: [0] }, version: 1, updated_by: "release-cloud", updated_at: iso(18000) }]]);
const pageKeys = new Map();
// The helper and the myco bridge, as the App serves them.
const pageHelperJS = readFileSync(new URL("../../internal/console/pagebridge.js", import.meta.url), "utf8");
const stuckCurrent = new Map([["legacy-codex", stuckTask], ["mac-exec", machineStuckTask]]);

// A task picked up and quiet ever since, with a later task the sender stopped:
// the newest message is closed, but the older one is still stuck, so stopping
// and sending it again has to work from that older task's own row. It has no
// active_at, like a hub from before the field: updated_at is the fallback.
const stuckOlderTask = makeMessage({
  id: "msg_stuck_older",
  thread_id: "th_console_stuck_stopped",
  from: CONSOLE,
  to: "legacy-codex",
  type: "task",
  status: "acked",
  body: "把旧版 runner 的配置文件检查一遍。",
  created_at: iso(19 * 60 * 60),
  updated_at: iso(19 * 60 * 60),
});
const stoppedAfterTask = makeMessage({
  id: "msg_stopped_after",
  thread_id: "th_console_stuck_stopped",
  from: CONSOLE,
  to: "legacy-codex",
  type: "task",
  status: "failed",
  error: "stopped by the sender",
  body: "顺便把旧版 runner 的配置贴一份给我。",
  created_at: iso(2 * 60 * 60),
  updated_at: iso(2 * 60 * 60),
});

// Pulled by the inbox node but not acknowledged: it has the task, it has not
// started it. Working means acknowledged only.
const grokPulledTask = makeMessage({
  id: "msg_grok_pulled",
  thread_id: "th_grok_pull",
  from: CONSOLE,
  to: "grok-agent-7c2e",
  type: "task",
  status: "delivered",
  body: "把 release 分支的 Go 测试跑一遍，结果贴在这个会话里。",
  created_at: iso(3 * 60 * 60),
  updated_at: iso(3 * 60 * 60 - 60),
});

// Taken several times by the same node and never started on: the hub gave
// up on it (the app explains it and offers to send it again).
const grokExhaustedTask = makeMessage({
  id: "msg_grok_exhausted",
  thread_id: "th_grok_exhausted",
  from: CONSOLE,
  to: "grok-agent-7c2e",
  type: "task",
  status: "failed",
  error: "delivery attempts exhausted: recipient pulled but never acknowledged",
  body: "README 的截图换成新版的。",
  created_at: iso(5 * 60 * 60),
  updated_at: iso(4 * 60 * 60),
});

// A machine the smoke test deletes, with two nodes and its own machine node.
// POST /app/mock/machine puts them back between widths and themes.
// A cloud server connected to the workspace: it limits sessions to one
// folder, has Claude Code (not Codex), and a few sessions of its own.
const vpsMachine = () => ({
  id: "m_vps", name: "VPS 东京", hostname: "tokyo-vps", os: "linux", arch: "x86_64",
  myco_version: "0.20.14", online: true, last_seen_at: iso(8), can_spawn: true,
});
// Hosts in the mock SSH config, and computers being connected over SSH or WSL.
const sshHosts = [
  { name: "tokyo-vps", user: "glenn", host: "203.0.113.20", port: 22 },
  { name: "gpu-box", user: "ubuntu", host: "10.0.4.12", port: 22, jump: "bastion" },
  { name: "hk-vps", user: "root", host: "203.0.113.7", port: 2222 },
  { name: "old-pi", user: "pi", host: "192.168.1.40", port: 22 },
  { name: "bastion", user: "glenn", host: "bastion.example.com", port: 22 },
];
const connectJobs = new Map();
// How agents this computer starts sign in to Claude (ClaudeAuthFix).
const claudeAuth = {
  account: "signed_out",
  cc_switch: [{ ref: "cc-switch:kimi", id: "kimi", name: "Kimi", host: "api.moonshot.cn", current: true }],
  shell: ["ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"], shell_host: "api.magpie.example",
  using: "",
};
// Setting up a workspace over SSH: log in (0.8 s), install (2 s), add (0.8 s).
function hubJobView(job) {
  const t = Date.now() - job.started;
  const steps = [{ key: "login", state: "running" }, { key: "install", state: "pending" }, { key: "add", state: "pending" }];
  const view = { id: job.id, route: job.route, steps, done: false };
  const name = job.route.target.replace(/^[^@]*@/, "");
  if (t < 800) return view;
  if (name === "old-pi") {
    steps[0].state = "failed";
    return { ...view, done: true, error: { code: "needs_sudo", message: "installing needs root, and sudo asks for a password there",
      command: "ssh -t old-pi 'curl -fsSL https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-hub.sh | sudo bash -s -- --domain auto'" } };
  }
  steps[0].state = "done";
  view.found = { os: "Linux", host: name, root: "yes", hub: "no" };
  view.log = ["==> installing myco hub v0.20.19", "==> Caddy: certificate for 203-0-113-7.sslip.io"];
  if (t < 2800) { steps[1].state = "running"; return view; }
  steps[1].state = "done";
  if (t < 3600) { steps[2].state = "running"; return view; }
  steps[2].state = "done";
  if (!job.hub) {
    state.counter += 1;
    job.hub = { id: `hub_ssh_${state.counter}`, name: `${name}`, url: "https://203-0-113-7.sslip.io", token: "myco_console_mock",
      ok: true, hub_version: "0.20.19", update_available: false, agents: [], threads: {}, recent: [] };
    state.hubs.push(job.hub);
  }
  return { ...view, done: true, hub_id: job.hub.id };
}

// How the App reaches each computer ("<hub>/<machine>") and what a check-up finds.
const machineReach = new Map([["hub_studio/m_vps", { via: "ssh", target: "tokyo-vps" }]]);
const machineHealth = new Map();
// What a job shows at this moment: log in (0.8 s), install (2 s), show up
// (1 s); old-pi fails to log in. Once done, the computer is on the workspace.
function connectView(job) {
  if (job.kind === "hub") return hubJobView(job);
  const t = Date.now() - job.started;
  const steps = [{ key: "login", state: "running" }, { key: "install", state: "pending" }, { key: "online", state: "pending" }];
  const view = { id: job.id, hub: job.hub, route: job.route, steps, done: false };
  const name = job.route.target.replace(/^[^@]*@/, "");
  if (name === "old-pi") {
    if (t < 700) return view;
    steps[0].state = "failed";
    return { ...view, done: true, error: { code: "auth", message: "pi@192.168.1.40: Permission denied (publickey,password).", command: "ssh old-pi" } };
  }
  if (t < 800) return view;
  steps[0].state = "done";
  view.found = { os: "Linux", arch: "x86_64", host: name, curl: "yes", bash: "yes" };
  view.log = ["==> installing myco", "myco v0.20.17 installed to ~/.local/bin/myco"];
  if (t < 2800) { steps[1].state = "running"; return view; }
  steps[1].state = "done";
  view.log.push("joined: this machine is on myco", "runner service installed (systemd)");
  if (t < 3800) { steps[2].state = "running"; return view; }
  steps[2].state = "done";
  if (!job.machine) {
    const hub = state.hubs.find((h) => h.id === job.hub);
    state.counter += 1;
    const wsl = job.route.via === "wsl";
    job.machine = { ...vpsMachine(), id: `m_cx${state.counter}`, name: wsl ? `Glenn 的 Windows-wsl` : name, hostname: wsl ? "GLENN-PC" : name, last_seen_at: iso(0), myco_version: "0.20.17" };
    if (hub) hub.machines = [...(hub.machines || []), job.machine];
  }
  return { ...view, done: true, machine_id: job.machine.id };
}
const vpsInfo = {
  dirs: ["~/code/myco-team"], new_dir: "~/code/myco-team", max_access: "workspace", home: "/home/glenn",
  ais: [
    { cli: "claude", name: "Claude Code", state: "ready", version: "2.1.3 (Claude Code)", install: "curl -fsSL https://claude.ai/install.sh | bash", login: "claude" },
    { cli: "codex", name: "Codex", state: "missing", install: "npm install -g @openai/codex", login: "codex login" },
    { cli: "gemini", name: "Gemini CLI", state: "missing", install: "npm install -g @google/gemini-cli", login: "gemini" },
    { cli: "opencode", name: "OpenCode", state: "missing", install: "curl -fsSL https://opencode.ai/install | bash", login: "opencode auth login" },
  ],
  sessions: [
    { cli: "claude", session_id: "7f3a2c10-1b2d-4e5f-8a9b-0c1d2e3f4a5b", workdir: "/home/glenn/code/myco-team/api", last_active: iso(12 * 60), title: "把订单接口的分页改成游标分页" },
    { cli: "claude", session_id: "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f", workdir: "/home/glenn/code/myco-team/web", last_active: iso(3 * 3600), title: "修复登录页在 Safari 上的跳转" },
    { cli: "claude", session_id: "9e8d7c6b-5a4f-4e3d-2c1b-0a9f8e7d6c5b", workdir: "/home/glenn/scratch", last_active: iso(26 * 3600), title: "试一下 bun 的启动速度" },
    { cli: "claude", session_id: "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e", workdir: "/home/glenn/code/myco-team/ops", last_active: iso(2 * 86400), title: "给 nginx 加上 gzip", joined: "tokyo-vps-claude-ops" },
    { cli: "codex", session_id: "019b0001-aaaa-7bbb-8ccc-000000000001", workdir: "/home/glenn/code/myco-team/api", last_active: iso(40 * 60), title: "跑一遍测试，把失败的列出来", auto: true },
    { cli: "claude", session_id: "3c4d5e6f-7a8b-4c9d-8e0f-2a3b4c5d6e7f", workdir: "/home/glenn/code/myco-team/web", last_active: iso(5 * 3600), title: "生成本周的更新说明", auto: true },
  ],
};
const versionBelow = (v, want) => {
  const a = String(v || "0").replace(/^v/, "").split(/[.-]/).map(Number);
  const b = want.split(".").map(Number);
  for (let i = 0; i < 3; i++) if ((a[i] || 0) !== b[i]) return (a[i] || 0) < b[i];
  return false;
};
const machineInfoOf = (m) => m.id === "m_vps" ? vpsInfo : {
  dirs: ["~"], new_dir: "~/myco", max_access: m.id.startsWith("m_studio") ? "full" : "workspace", home: m.os === "darwin" ? "/Users/glenn" : "/home/glenn",
  ais: [
    { cli: "claude", name: "Claude Code", state: "ready", version: "2.1.3 (Claude Code)", login: "claude",
      mcp: [{ name: "filesystem", transport: "stdio", scope: "user" }, { name: "figma", transport: "http", host: "mcp.figma.com", scope: "user" }],
      skills: [{ name: "myco", description: "Talk to agents", scope: "user", managed_by: "myco" }],
      plugins: [{ name: "review", marketplace: "official", enabled: true }],
      tool_warnings: ["/Users/glenn/.claude/plugins/extra.json: invalid JSON"] },
    { cli: "codex", name: "Codex", state: "ready", version: "codex-cli 0.52.0", login: "codex login", models: CODEX_MODELS,
      mcp: [{ name: "search", transport: "http", host: "search.example.com", scope: "user" }],
      skills: [{ name: "audit", description: "Review code", scope: "user" }], plugins: [] },
    { cli: "gemini", name: "Gemini CLI", state: "missing", install: "npm install -g @google/gemini-cli", login: "gemini" },
    { cli: "opencode", name: "OpenCode", state: "missing", install: "curl -fsSL https://opencode.ai/install | bash", login: "opencode auth login" },
  ],
  sessions: [],
};
// Folders allowed from the App (POST …/machines/{mid}/dirs), by machine.
const machineDirs = new Map();

const deletableMachine = () => ({
  id: "m_delete", name: "临时 Mac", hostname: "temp-mac.local", os: "darwin", arch: "arm64",
  myco_version: "0.18.39", online: true, last_seen_at: iso(5), can_spawn: true,
  owner: "glenn", owner_id: "mem_glenn",
});
const deletableAgents = () => [
  makeAgent({
    name: "temp-codex", owner: "glenn", owner_id: "mem_glenn", display_name: "临时 Codex",
    description: "临时机器上的 Codex，用来演示删除机器。", kind: "agent", machine_id: "m_delete",
    runtime: { cli: "codex", cli_version: "0.52.0", model: "gpt-6-sol", access: "workspace", max_access: "full" },
    binding: { harness: "codex", environment: "macOS 26.1", workdir: "~/code/temp" },
    labels: ["machine=temp-mac"],
  }),
  makeAgent({
    name: "temp-shell", owner: "glenn", owner_id: "mem_glenn", display_name: "临时 Shell",
    description: "临时机器上的 shell 节点。", kind: "shell", machine_id: "m_delete",
    runtime: { cli: "shell", access: "workspace" },
    binding: { harness: "shell", environment: "macOS 26.1", workdir: "~/code/temp" },
    labels: ["machine=temp-mac"],
  }),
  makeAgent({
    name: "temp-mac-machine", owner: "glenn", owner_id: "mem_glenn", display_name: "临时 Mac",
    kind: "machine", machine_id: "m_delete",
    labels: ["machine=temp-mac"], binding: { harness: "machine" },
  }),
];

// Earlier turns of the conversation with PM (Codex), so the chat is long
// enough to scroll and its turn rail has something to show.
const earlierTurns = [
  ["上周的发布有没有遗留问题？", "没有阻塞的。还剩两个低优先级的界面问题，已经记在看板上。"],
  ["把 CHANGELOG 里 0.16.7 的条目按模块分一下组。", "分好了：App 4 条、Hub 2 条、CLI 1 条。改动在 `CHANGELOG.md`。"],
  ["runner 重连那个修复，影响旧版本的 Hub 吗？", "不影响。旧 Hub 不认识新字段会直接忽略，行为和以前一样。"],
  ["那 Windows 上测过没有？", "CI 的 Windows job 通过了；真机我没有，建议你在台式机上点一遍。"],
  ["帮我看下 make check 现在要跑多久。", "本机 2 分 10 秒：vet 8 秒，test 1 分 40 秒，smoke 22 秒。"],
  ["test 里最慢的是哪个包？", "`internal/infra/sqlite`，约 13 秒，主要是迁移测试。"],
  ["先不管它。发布分支拉了吗？", "拉了，`release/v0.16.8`，基于 main 的最新提交。"],
  ["版本号现在是多少？", "`VERSION` 还是 v0.16.7，等你说发再改。"],
].flatMap(([q, a], i) => {
  const at = 7200 + (8 - i) * 900;
  return [
    makeMessage({ thread_id: "th_console_release", project: "website", from: CONSOLE, to: "pm-codex", type: "task", status: "done", body: q, created_at: iso(at), updated_at: iso(at) }),
    makeMessage({ thread_id: "th_console_release", from: "pm-codex", to: CONSOLE, type: "reply", status: "done", body: a, created_at: iso(at - 120), updated_at: iso(at - 120) }),
  ];
});

const consoleThreads = {
  th_console_release: [
    ...earlierTurns,
    makeMessage({ thread_id: "th_console_release", project: "website", from: CONSOLE, to: "pm-codex", type: "task", status: "done", body: "整理 v0.16.8 的发布说明，列出主要变化和升级注意事项。", created_at: iso(7200), updated_at: iso(7200) }),
    makeMessage({ thread_id: "th_console_release", from: "pm-codex", to: CONSOLE, type: "progress", status: "done", body: "正在读取 git log 和 CHANGELOG…", created_at: iso(7000), updated_at: iso(7000) }),
    makeMessage({
      thread_id: "th_console_release", from: "pm-codex", to: CONSOLE, type: "reply", status: "done",
      body: [
        "## v0.16.8 发布说明",
        "",
        "本次发布主要包含：",
        "",
        "- 对话页支持 Markdown 表格与代码块",
        "- 本机会话一键接入",
        "",
        "| 项目 | 变化 |",
        "| --- | --- |",
        "| App | 新增对话 |",
        "| Hub | 修复 runner 重连 |",
        "",
        "升级前请先跑一遍：",
        "",
        "```bash",
        "make check",
        "```",
        "",
        "详情见 [发布页](https://example.com/releases/v0.16.8)。",
        "",
        '<img src=x onerror=alert(1)>',
      ].join("\n"),
      created_at: iso(6600), updated_at: iso(6600),
    }),
  ],
  th_console_exec: [
    makeMessage({ thread_id: "th_console_exec", from: CONSOLE, to: "ios-shell", type: "task", status: "done", body: "跑一遍 iOS 测试套件，把结果贴给我。", created_at: iso(9000), updated_at: iso(9000) }),
    makeMessage({
      thread_id: "th_console_exec", from: "ios-shell", to: CONSOLE, type: "reply", status: "done",
      body: JSON.stringify({
        exit_code: 0,
        stdout: "Test Suite 'All tests' passed.\nExecuted 128 tests, with 0 failures (0 unexpected) in 41.8 seconds",
        stderr: "",
        duration_ms: 41820,
      }, null, 2),
      created_at: iso(8700), updated_at: iso(8700),
    }),
  ],
  // A page an agent published (myco page publish): its link in the reply
  // shows as a card and opens in the page panel.
  th_console_page: [
    makeMessage({ thread_id: "th_console_page", project: "website", from: CONSOLE, to: "release-cloud", type: "task", status: "done", body: "把 v0.16.8 的发布说明做成一页网页，带上每类改动的数量。", created_at: iso(20000), updated_at: iso(20000) }),
    makeMessage({ thread_id: "th_console_page", project: "website", from: "release-cloud", to: CONSOLE, type: "note", status: "done", body: "下载页的改动在我电脑上跑着，实时预览（改了会自己刷新）：http://192.168.1.20:7780/pages/pg_live", created_at: iso(18200), updated_at: iso(18200) }),
    makeMessage({ thread_id: "th_console_page", project: "website", from: "release-cloud", to: CONSOLE, type: "note", status: "done", body: "上线前的检查也做成了一页清单，大家可以在上面勾：http://192.168.1.20:7780/pages/pg_checklist\n\n勾完了点页面下面的「交给 agent」，我接着往下做。", created_at: iso(18500), updated_at: iso(18500) }),
    makeMessage({ thread_id: "th_console_page", project: "website", from: "release-cloud", to: CONSOLE, type: "reply", status: "done", body: "做好了，发布说明的网页在这里：http://192.168.1.20:7780/pages/pg_release\n\n按「新功能 / 修复 / 文档」分了三栏，图表是页面里自己画的。", created_at: iso(19000), updated_at: iso(19000) }),
  ],
  th_console_fail: [
    makeMessage({ thread_id: "th_console_fail", from: CONSOLE, to: "docs-claude", type: "task", status: "done", body: "把 README 里的截图更新一下。", created_at: iso(15000), updated_at: iso(15000) }),
    makeMessage({
      thread_id: "th_console_fail", from: "docs-claude", to: CONSOLE, type: "event", status: "failed",
      body: "任务失败：docs-claude 的会话已退出（exit 1）。",
      created_at: iso(14700), updated_at: iso(14700),
    }),
  ],
  th_console_attach: [
    makeMessage({ thread_id: "th_console_attach", from: CONSOLE, to: "pm-codex", type: "task", status: "done", body: "把最新的构建日志发我。", created_at: iso(18600), updated_at: iso(18600) }),
    makeMessage({
      thread_id: "th_console_attach", from: "pm-codex", to: CONSOLE, type: "reply", status: "done",
      body: "构建日志见附件，最后一行是 BUILD SUCCESSFUL。",
      attachments: [{ file_id: "f_demo_build", name: "build-0.16.8.log", size: 18432, sha256: "demo", content_type: "text/plain" }],
      created_at: iso(18300), updated_at: iso(18300),
    }),
  ],
  th_console_busy: [
    makeMessage({ thread_id: "th_console_busy", from: CONSOLE, to: "ios-shell", type: "task", status: "delivered", body: "在后台跑性能测试，完成后告诉我。", created_at: iso(180), updated_at: iso(120) }),
    makeMessage({ thread_id: "th_console_busy", from: "ios-shell", to: CONSOLE, type: "progress", status: "done", body: "已经跑完 3/5 个场景…", created_at: iso(60), updated_at: iso(60) }),
  ],
  // Picked up long ago and quiet since: looks stuck in the chat and the list.
  th_console_stuck: [stuckTask, stuckProgress],
  // An older task stuck behind a newer one the sender stopped: it still has
  // its own stop and resend in the chat.
  th_console_stuck_stopped: [stuckOlderTask, stoppedAfterTask],
  // An inbox node pulled this task but never acknowledged it: still queued.
  th_grok_pull: [grokPulledTask],
  // An earlier one it took several times and never started on: given up on.
  th_grok_exhausted: [grokExhaustedTask],
};

// Claude Code / Codex transcripts on this computer, newest first. Two are
// already joined (one here, one on another Hub).
const localSessions = [
  {
    cli: "claude", session_id: "7c1d9f4e-2a6b-4c18-9e0d-4a1f2b3c5d6e",
    workdir: "/Users/glenn/code/myco", last_active: iso(120),
    title: "让 App 支持对话列表",
  },
  {
    cli: "codex", session_id: "019f20c1-a446-7812-bbc8-ce4ac3b889f6",
    workdir: "/Users/glenn/code/app-web", last_active: iso(900),
    title: "整理 App 第二版上线清单",
  },
  {
    cli: "claude", session_id: "9b8a7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
    workdir: "/Users/glenn/code/infra", last_active: iso(3600),
    title: "检查 Hub 的隧道配置",
    joined: { agent: "glenn-mac-claude-x9k2", hub: "http://192.168.1.20:7780", hub_name: "Glenn 的 MacBook Pro 的工作区" },
    // The same session is an agent of a second workspace too.
    joined_all: [
      { agent: "glenn-mac-claude-x9k2", hub: "http://192.168.1.20:7780", hub_name: "Glenn 的 MacBook Pro 的工作区" },
      { agent: "glenn-mac-claude-x9k2-tn", hub: "https://hub.tapnow.example.com/w/tapnow" },
    ],
  },
  {
    cli: "codex", session_id: "019f0f11-3c2b-7a10-8d4e-9f8a7b6c5d4e",
    workdir: "/Users/glenn/code/notes", last_active: iso(7200),
    joined: { agent: "glenn-mac-codex-ab12", hub: "https://other-hub.example.com" },
  },
];
// What was joined at the start: the smoke test's reset puts it back, so each
// window size can join a session on this computer again.
const localJoinedAtStart = localSessions.map((s) => s.joined);


const state = {
  hubs: [
    {
      id: "hub_studio",
      e2e: !!process.env.MOCK_E2E,
      name: "Glenn 的 MacBook Pro 本机 Hub",
      url: "http://192.168.1.20:7780",
      token: "myco_console_demo_studio",
      ok: true,
      hub_version: "0.19.23",
      update_available: true,
      threads: { th_release: studioThread, th_coord: coordThread, th_dispatch_docs: dispatchDocs, th_dispatch_release: dispatchRelease, th_member_note: memberNote, ...consoleThreads },
      machines: [
        {
          id: "m_studio", name: "Studio Mac", hostname: "studio.local", os: "darwin", arch: "arm64",
          myco_version: "0.20.14", online: true, last_seen_at: iso(5), can_spawn: true,
        },
        {
          id: "m_bench", name: "Bench Linux", hostname: "bench-01", os: "linux", arch: "x86_64",
          myco_version: "0.9.4", online: true, last_seen_at: iso(30), can_spawn: false,
        },
        {
          id: "m_office", name: "Office PC", hostname: "office-pc", os: "windows", arch: "amd64",
          myco_version: "0.16.6", online: true, last_seen_at: iso(90), can_spawn: true,
        },
        vpsMachine(),
        deletableMachine(),
      ],
      agents: [
        makeAgent({
          name: "pm-codex", owner: "glenn", owner_id: "mem_glenn", display_name: "PM (Codex)", description: "规划发布并审查 PR。",
          kind: "agent", machine_id: "m_studio",
          // Its 5-hour window is nearly used (amber), the weekly one is not.
          runtime: {
            cli: "codex", cli_version: "0.52.0", model: "gpt-6-sol", model_source: "app",
            effort: "high", effort_source: "app", access: "workspace", max_access: "workspace", models: CODEX_MODELS,
            quota: {
              source: "codex", at: iso(120),
              windows: [
                { name: "five_hour", used: 0.95, resets_at: iso(-(3 * 3600 + 40 * 60)) },
                { name: "seven_day", used: 0.62, resets_at: iso(-(3 * 86_400)) },
              ],
            },
          },
          run_settings: { model: "gpt-6-sol", effort: "high" },
          status_text: "正在审查 multihub 分支", status_at: iso(180),
          labels: ["machine=studio-mac", "group=官网改版", "role=pm", "review", "release"], capabilities: ["review", "release"],
          skills: [
            { name: "review", summary: "审查 Go 和 TypeScript 的改动，指出问题和风险", examples: ["帮我看下 PR 128", "这个分支能合吗"] },
            { name: "release", summary: "写发布说明、打 tag、发版" },
          ],
          binding: { harness: "codex", environment: "macOS 26.1", workdir: "~/code/myco", session_id: "01a1-pm-session" }, session_in_use: true,
          aliases: ["pm"], allow_from: ["glenn"], enrolled_by: "console",
          // A resume line made five days ago and last used an hour ago in a
          // sandbox: the row shows when and where. Making or revoking one is
          // demoed by the smoke test.
          resume: { created_at: iso(86_400 * 5), last_used_at: iso(3_600), last_used_env: "codespaces-4f2a" },
        }),
        makeAgent({
          name: "mcp-claude", owner: "glenn", owner_id: "mem_glenn", display_name: "MCP Claude", description: "用 Claude Code 处理文档和工具调用。",
          kind: "agent", machine_id: "m_studio",
          runtime: { cli: "claude", cli_version: "2.1.240", model: "sonnet", access: "workspace", max_access: "workspace", features: ["mcp_tools"],
            mcp: [{ name: "project-docs", transport: "stdio", scope: "project", enabled: true }],
            skills: [{ name: "release-check", description: "Check release notes", scope: "project" }] },
          binding: { harness: "claude", environment: "macOS 26.1", workdir: "~/code/myco" },
        }),
        makeAgent({
          name: "docs-claude", owner: "laohu", owner_id: "mem_laohu", description: "保持 README 和 docs/ 与代码同步。", visibility: "private",
          kind: "agent", machine_id: "m_studio",
          runtime: {
            cli: "claude", cli_version: "2.1.240", model: "claude-opus-5-5", model_source: "observed",
            effort: "low", effort_source: "app", access: "readonly", max_access: "workspace", provider: "DeepSeek", provider_source: "machine", features: ["mcp_tools"],
            away: "stopped",
            quota: {
              source: "claude", at: iso(600),
              windows: [
                { name: "five_hour", used: 0.4, resets_at: iso(-(2 * 3600)) },
                { name: "seven_day", used: 0.18, resets_at: iso(-(2 * 86_400)) },
              ],
            },
          },
          run_settings: { model: "", effort: "low" },
          labels: ["machine=studio-mac", "group=docs"], capabilities: ["docs"],
          online: false, last_seen_at: iso(5400), status_at: iso(5400),
          binding: { harness: "claude", environment: "macOS 26.1", workdir: "~/code/myco" },
        }),
        makeAgent({
          name: "ios-shell", owner: "matthew", owner_id: "mem_matthew", display_name: "iOS Shell", description: "按需构建和测试 iOS App。",
          kind: "shell", machine_id: "m_bench",
          runtime: { cli: "shell", access: "workspace" },
          status_text: "正在运行 make test-ios", status_at: iso(45),
          labels: ["machine=bench-01", "group=ios", "ci"], capabilities: ["build", "test"],
          binding: { harness: "shell", environment: "Linux 6.8", workdir: "~/code/app-ios" },
          delivery: { mode: "webhook", url: "http://bench-01:7790/hook", has_secret: true },
        }),
        makeAgent({
          name: "mac-exec", owner: "glenn", owner_id: "mem_glenn", description: "在 Studio Mac 上执行命令。",
          kind: "shell", machine_id: "m_studio", runtime: { cli: "exec" },
          labels: ["machine=studio"], binding: { harness: "exec", environment: "macOS", workdir: "~" },
          delivery: { mode: "pull" }, allow_from: ["pm-codex"], status_at: iso(30),
        }),
        makeAgent({
          name: "glenn", owner: "glenn", owner_id: "mem_glenn", display_name: "Glenn", description: "人。负责发任务和验收。",
          kind: "human", binding: { harness: "human" },
          labels: ["role=human"], status_at: iso(3),
        }),
        makeAgent({
          name: "release-cloud", owner: "glenn", owner_id: "mem_glenn", display_name: "Release (Claude 云端)", description: "在 claude.ai 上整理发布说明。",
          // It has not said which model it runs on (an older myco in that session).
          kind: "agent", runtime: { cli: "claude-cloud", access: "readonly" },
          status_text: "正在整理发布说明", status_at: iso(60),
          labels: ["group=release"], binding: { harness: "claude-cloud", environment: "claude.ai" },
        }),
        makeAgent({
          name: "review-cloud", owner: "matthew", owner_id: "mem_matthew", display_name: "Review (Codex 云端)", description: "在 Codex 云端做代码审查。",
          kind: "agent",
          runtime: {
            cli: "codex-cloud", model: "gpt-6-sol", model_source: "observed", access: "readonly",
            // Out of quota two hours ago: red, and the numbers are stale.
            quota: {
              source: "codex", at: iso(2 * 3600),
              windows: [{ name: "five_hour", used: 1, resets_at: iso(-(40 * 60)), status: "rejected" }],
            },
          },
          labels: ["group=release"], binding: { harness: "codex-cloud", environment: "Codex 云端" },
        }),
        makeAgent({
          name: "legacy-codex", owner: "laohu", owner_id: "mem_laohu", description: "旧版 runner，还没有上报运行信息。",
          kind: "agent", machine_id: "m_bench",
          labels: ["machine=bench-01"], online: true, status_at: iso(20),
          binding: { harness: "codex", environment: "Linux 6.8", workdir: "~/code/legacy" },
        }),
        // Joined from a session no runner can wake: it receives only while
        // `myco inbox wait` runs in that session.
        makeAgent({
          name: "grok-agent-7c2e", owner: "glenn", owner_id: "mem_glenn", display_name: "Myco (Grok)", description: "在 Grok 会话里用 inbox wait 接收消息。",
          kind: "agent", machine_id: "m_studio", receiving: "session",
          // It runs its CLI in its own session and reports the model itself.
          runtime: { cli: "inbox", model: "grok-4", model_source: "self" },
          // Pulled from the inbox but not acked: queued, so not working.
          delivered: [grokPulledTask],
          labels: ["machine=studio-mac", "group=release"],
          binding: { harness: "inbox", environment: "macOS 26.1", workdir: "~/code/myco" },
        }),
        makeAgent({
          name: "studio-mac-machine", display_name: "Studio Mac", kind: "machine", machine_id: "m_studio",
          labels: ["machine=studio-mac"], binding: { harness: "machine" }, runtime: { cli: "machine", max_access: "full" },
        }),
        // Office PC has not allowed terminals (spawn.max_access: workspace).
        makeAgent({
          name: "office-pc-machine", display_name: "Office PC", kind: "machine", machine_id: "m_office",
          labels: ["machine=office-pc"], binding: { harness: "machine" }, runtime: { cli: "machine", max_access: "workspace" },
        }),
        makeAgent({
          name: "bench-01-machine", display_name: "Bench Linux", kind: "machine", machine_id: "m_bench",
          labels: ["machine=bench-01"], binding: { harness: "machine" },
        }),
        ...deletableAgents(),
      ],
      recent: [
        makeMessage({ thread_id: "th_release", from: "glenn", to: "pm-codex", type: "task", body: "可以，打 tag 吧。", created_at: iso(300), updated_at: iso(300) }),
        makeMessage({ thread_id: "th_release", from: "pm-codex", to: "glenn", type: "reply", body: "release/v0.16.8 分支检查全部通过，等你确认后打 tag。", created_at: iso(600), updated_at: iso(600) }),
        makeMessage({ thread_id: "th_ios", from: "glenn", to: "ios-shell", type: "task", body: "请跑一遍 iOS 测试套件，附上日志。", created_at: iso(240), updated_at: iso(240) }),
        makeMessage({ thread_id: "th_console_release", from: "pm-codex", to: CONSOLE, type: "reply", status: "done", body: "v0.16.8 发布说明已整理好。", created_at: iso(6800), updated_at: iso(6800) }),
      ],
    },
    {
      // This computer's own workspace: it is in `local.hubs` too, and the
      // settings page lists it under "这台电脑上的工作区".
      id: "hub_local",
      name: "本机 Hub",
      url: "http://127.0.0.1:7780",
      token: "myco_console_local_demo",
      ok: true,
      hub_version: "0.16.8",
      update_available: false,
      agents: [],
      threads: {},
      recent: [],
    },
    {
      id: "hub_lab",
      name: "Lab Linux",
      url: "http://10.0.0.42:7780",
      token: "myco_console_demo_lab",
      ok: false,
      error: "dial tcp 10.0.0.42:7780: connect: connection refused",
      hub_version: "0.16.4",
      update_available: true,
      agents: [],
      threads: {},
      recent: [],
    },
  ],
  counter: 0,
};

// A hub that cannot be reached, only with MOCK_OFFLINE_HUB=1: it otherwise
// shows its red banner in every demo screenshot.
if (!process.env.MOCK_OFFLINE_HUB) state.hubs = state.hubs.filter((h) => h.id !== "hub_lab");
if (EMPTY) state.hubs = [];
// MOCK_SHOWCASE=en|zh: the polished demo data for README screenshots.
const SHOWCASE = process.env.MOCK_SHOWCASE;
if (SHOWCASE) {
  const sc = showcase(SHOWCASE, { makeAgent, makeMessage, iso, CONSOLE });
  state.hubs = sc.hubs;
  localSessions.splice(0, localSessions.length, ...sc.localSessions);
}

// Local joins alternate between a healthy runner and a warning, so both
// states can be seen without restarting the mock.
let joinCount = 0;
let replySeq = 0;
// How many release notes the mock's agent has sent (each one a new version).
let notesSeq = 0;

// The release notes the mock's agent writes: a little different each time,
// so a second one shows what changed.
function demoNotes(n) {
  return [
    "# v0.16.8 发布说明",
    "",
    "## App",
    "",
    "- 对话页支持 Markdown 表格与代码块",
    n > 1 ? "- 进展可以带截图，点开看大图" : "- 本机会话一键接入",
    "- 侧边栏可以拖宽",
    "",
    "## Hub",
    "",
    "- 修复 runner 断线后重连",
    ...(n > 1 ? ["- 文件预览：Markdown 直接在 App 里看"] : []),
    "",
    "升级前请先跑一遍 `make check`。",
    "",
    "### 升级步骤",
    "",
    "1. 停掉 runner：`myco runner stop`",
    "2. 换上新版本，再启动",
    "3. 看一眼 `myco status`，节点都在线就好",
    "",
    "> 已经在跑的任务不受影响：runner 重启后会接着上报进展。",
    "",
    "| 平台 | 安装包 | 大小 |",
    "| --- | --- | --- |",
    "| macOS | myco.dmg | 38 MB |",
    "| Windows | myco-setup.exe | 41 MB |",
    "",
    "```bash",
    "myco runner stop && myco upgrade && myco runner start",
    "```",
    "",
    "---",
    "",
    "#### 已知问题",
    "",
    "- 旧版 App 连新版 Hub 时，头像要刷新一次才显示",
    "",
  ].join("\n");
}

function consoleReply() {
  replySeq += 1;
  const form = (replySeq - 1) % 4;
  if (form === 0) {
    return {
      body: [
        "## 已完成",
        "",
        "我检查了当前工作区，主要结论如下：",
        "",
        "| 检查项 | 结果 |",
        "| --- | --- |",
        "| 类型检查 | 通过 |",
        "| 单元测试 | 通过 |",
        "",
        "```bash",
        "npm run build && make check",
        "```",
        "",
        "还有一处建议：发布前再跑一遍完整的集成测试。",
      ].join("\n"),
    };
  }
  if (form === 1) {
    return {
      body: JSON.stringify({
        exit_code: 0,
        stdout: "go test ./...\nok  github.com/chenhg5/myco/internal/app\nok  github.com/chenhg5/myco/internal/console",
        stderr: "",
        duration_ms: 12340,
      }, null, 2),
    };
  }
  if (form === 2) {
    return {
      body: "处理好了。改动集中在 console 的 session 读取和 App 的对话页面，类型检查和构建都已经通过。\n\n- [x] 改 session 读取\n- [x] 改对话页面\n- [~] 写发布说明\n- [ ] 发版\n\n需要我继续把发布说明也写好吗？",
    };
  }
  return {
    // (long enough to grow in: its pictures come after its words)
    body: "跑完了，截图和日志在附件里。最后一行是 BUILD SUCCESSFUL。\n\n首页在三种宽度下都看过了，导航栏在窄屏下会收起成一个菜单按钮，下载按钮的位置没有变。发布说明按新功能、修复、文档三类整理好了，你看一下措辞合不合适。",
    attachments: [
      { file_id: `f_demo_shot_${replySeq}`, name: "首页截图.png", size: DEMO_PNG.length, sha256: "demo", content_type: "image/png" },
      { file_id: `f_demo_${replySeq}`, name: "build.log", size: 20480, sha256: "demo", content_type: "text/plain" },
      { file_id: `f_demo_md_${(notesSeq += 1)}`, name: "发布说明.md", size: 640, sha256: "demo", content_type: "text/markdown" },
      { file_id: `f_demo_html_${replySeq}`, name: "首页.html", size: 420, sha256: "demo", content_type: "text/html" },
    ],
  };
}

// A task gets a progress note after 3s and a reply after 6s. A supplement
// keeps the same round going: the answer comes 6s after the latest addition,
// not 6s after the task (and there is only ever one timer for a task).
const replyTimers = new Map();
// Agents waiting on a question they asked, by agent name: the task.
const askingBy = new Map();
// Myco (Grok) holds a task PM (Codex) gave it, waiting on a question it
// asked PM: open, but not a decision of yours. Only while the smoke test
// that looks at it runs (/app/mock/held-for-other {on}).
let heldForOtherOn = false;
const heldForOther = makeMessage({
  id: "msg_grok_held", thread_id: "th_grok_held", from: "pm-codex", to: "grok-agent-7c2e", type: "task",
  status: "held", asking: "msg_grok_q", body: "给官网首页写三条 slogan 候选。",
  created_at: iso(3 * 3600), updated_at: iso(2 * 3600),
});
function scheduleReply(hub, threadId, agent, taskId, restart = false) {
  const had = replyTimers.get(taskId);
  if (had && !restart) return;
  if (had) for (const t of had) clearTimeout(t);
  const timers = [];
  replyTimers.set(taskId, timers);
  const later = (ms, fn) => {
    const t = setTimeout(fn, ms);
    if (typeof t.unref === "function") t.unref();
    timers.push(t);
  };
  // Slow startup and live blocks for the conversation streaming smoke cases.
  const preview = hub.threads[threadId]?.find((m) => m.id === taskId);
  // An agent that offers the next step (myco message reply --suggest): the
  // task carries it, the app's input shows it, Tab takes it.
  if (preview && /下一步/.test(preview.body)) {
    later(300, () => { if (["queued", "delivered"].includes(preview.status)) Object.assign(preview, { status: "acked", updated_at: iso(0) }); });
    later(1500, () => {
      replyTimers.delete(taskId);
      const msgs = hub.threads[threadId];
      if (!msgs || preview.status === "failed") return;
      msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done", reply_to: taskId, body: "登录页改好了，表单校验也加上了。", created_at: iso(0), updated_at: iso(0) }));
      const suggest = /长建议/.test(preview.body)
        ? "把注册页、找回密码页和个人设置页也按同样的方式检查一遍，顺便补上缺的表单校验和错误提示，再跑一遍完整测试"
        : "再跑一遍完整测试";
      Object.assign(preview, { status: "done", suggest, updated_at: iso(0) });
      releaseAfter(hub, threadId, taskId);
    });
    return;
  }
  // An agent in one long session (it receives in its conversation): the
  // first task gets a few steps, then it is set aside (held) for you to do
  // something. You write again; it picks that up and goes on, but its steps
  // and progress still go to the first task (the one it took first). The
  // App shows them under your second message, in time.
  if (preview && /长会话：/.test(preview.body)) {
    const msgs = hub.threads[threadId] || [];
    // (the smoke test's windows run at once: each pair carries its own tag;
    // a new message may start a thread of its own)
    const tag = (b) => /（([^）]+)）/.exec(b || "")?.[1] || "";
    const first = Object.values(hub.threads).flat().find((m) => m !== preview && m.type === "task" && /长会话：/.test(m.body || "") && m.status === "held" && tag(m.body) === tag(preview.body));
    const firstMsgs = first ? hub.threads[first.thread_id] || msgs : msgs;
    const stamp = (steps, from, gap) => steps.map((d, i) => ({ ...d, at: new Date(from + i * gap).toISOString() }));
    later(300, () => { if (["queued", "delivered"].includes(preview.status)) Object.assign(preview, { status: "acked", updated_at: iso(0) }); });
    if (!first) {
      preview.trace = stamp([
        { kind: "thinking", text: "先看看授权状态。" },
        { kind: "tool", tool: "Bash", id: "l1", input: "lark-cli auth status" },
        { kind: "tool_result", id: "l1", output: "not logged in" },
        { kind: "text", text: "需要你在会话里允许授权，开通后告诉我。" },
        { kind: "turn_end" },
        { kind: "turn_end" },
      ], Date.now(), 300);
      preview.trace_pace = 300;
      later(2500, () => { if (preview.status === "acked") Object.assign(preview, { status: "held", updated_at: iso(0) }); });
      return;
    }
    // The second: picked up, nothing of its own; it all goes to the first.
    preview.trace = [];
    later(1200, () => {
      const at = Date.now();
      first.trace = [...first.trace.slice(0, -1), ...stamp([
        { kind: "thinking", text: "授权开通了，接着拉群消息。" },
        { kind: "tool", tool: "Bash", id: "l2", input: "lark-cli im chats list" },
        { kind: "tool_result", id: "l2", output: "12 chats" },
        { kind: "text", text: "授权已生效，开始拉群消息。" },
      ], at, 100), { kind: "turn_end" }];
      first.trace_pace = 1;
      firstMsgs.push(makeMessage({ thread_id: first.thread_id, from: agent, to: CONSOLE, type: "progress", status: "done", reply_to: first.id, body: "授权已生效，开始只读收集群消息。", created_at: iso(0), updated_at: iso(0) }));
    });
    later(6000, () => {
      replyTimers.delete(taskId);
      msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done", reply_to: taskId, body: "群消息拉完了。", created_at: iso(0), updated_at: iso(0) }));
      Object.assign(preview, { status: "done", updated_at: iso(0) });
      firstMsgs.push(makeMessage({ thread_id: first.thread_id, from: agent, to: CONSOLE, type: "reply", status: "done", reply_to: first.id, body: "授权这件事好了。", created_at: iso(0), updated_at: iso(0) }));
      Object.assign(first, { status: "done", updated_at: iso(0) });
    });
    return;
  }
  // Set aside the way the skill says: it hands work out, holds the task,
  // then says what it waits for (a progress), and stays in its session
  // (no end of turn). Waiting, not at work: no loading in the App.
  if (preview && /挂起说明/.test(preview.body)) {
    const msgs = hub.threads[threadId] || [];
    const t0 = Date.now();
    preview.trace = [
      { kind: "thinking", text: "先派给 release 做，等结果。" },
      { kind: "tool", tool: "Bash", id: "h1", input: 'myco message send --to release-cloud --body "定时任务和 Webhook 先做第一步"' },
      { kind: "tool_result", id: "h1", output: "sent" },
      { kind: "tool", tool: "Bash", id: "h2", input: `myco message hold --id ${taskId}` },
      { kind: "tool_result", id: "h2", output: "held" },
      { kind: "tool", tool: "Bash", id: "h3", input: `myco message progress --id ${taskId} --body "已派给 release，等它做完"` },
      { kind: "tool_result", id: "h3", output: "ok" },
      { kind: "tool", tool: "Bash", id: "h4", input: "myco inbox wait --timeout 55m --format text" },
    ].map((d, i) => ({ ...d, at: new Date(t0 + i * 250).toISOString() }));
    preview.trace_pace = 250;
    later(300, () => { if (["queued", "delivered"].includes(preview.status)) Object.assign(preview, { status: "acked", updated_at: iso(0) }); });
    later(1100, () => { if (preview.status === "acked") Object.assign(preview, { status: "held", updated_at: iso(0) }); });
    later(1600, () => {
      msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "progress", status: "done", reply_to: taskId, body: "已派给 release，等它做完再汇总给你。", created_at: iso(0), updated_at: iso(0) }));
    });
    later(9000, () => {
      replyTimers.delete(taskId);
      if (preview.status !== "held") return;
      msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done", reply_to: taskId, body: "release 那边做完了。", created_at: iso(0), updated_at: iso(0) }));
      Object.assign(preview, { status: "done", updated_at: iso(0) });
    });
    return;
  }
  if (preview && /逐字预览|启动预览|保留任务/.test(preview.body)) {
    const thinking = "先检查消息是否已经接手，再核对正在写出的文字和完整步骤之间的替换。";
    const text = "消息已经接手，正文正在逐字写出。每次收到的新片段会接在原来的文字后面，";
    const tail = "不会重新开始，也不会整段闪现。现在继续检查计划卡片和补充消息的目标。";
    preview.trace = /逐字预览/.test(preview.body) ? [
      { kind: "delta", stream: "thinking", id: "think-1", text: thinking.slice(0, 15) },
      { kind: "delta", stream: "thinking", id: "think-1", text: thinking.slice(15) },
      { kind: "thinking", text: thinking },
      { kind: "delta", stream: "text", id: "say-1", text },
      { kind: "delta", stream: "text", id: "say-1", text: tail },
      { kind: "text", text: text + tail },
      // A tool replaces unfinished blocks of either stream, too.
      { kind: "delta", stream: "text", id: "say-2", text: "这段临时文字会被后续工具替代。" },
      { kind: "tool", tool: "Read", id: "preview-read", input: "docs/design/app-ui.md" },
      { kind: "tool_result", id: "preview-read", output: "已核对界面规范" },
      { kind: "usage", input_tokens: 900, output_tokens: 120, duration_ms: 20000 },
    ] : [{ kind: "usage", input_tokens: 100, output_tokens: 5, duration_ms: 7000 }];
    preview.trace_pace = 1500;
    later(300, () => { if (["queued", "delivered"].includes(preview.status)) Object.assign(preview, { status: "acked", updated_at: iso(0) }); });
    later(/启动预览/.test(preview.body) ? 7000 : 20000, () => {
      replyTimers.delete(taskId);
      const msgs = hub.threads[threadId];
      if (!msgs || preview.status === "failed") return;
      msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done", reply_to: taskId, body: "预览检查完成。", created_at: iso(0), updated_at: iso(0) }));
      Object.assign(preview, { status: "done", updated_at: iso(0) });
      releaseAfter(hub, threadId, taskId);
    });
    return;
  }
  // Asked to hand the work out: the agent sends a task to another member,
  // says so and ends its turn (turn_end) without answering; the task stays
  // open while the other works. When that one answers, it answers you.
  {
    const task = hub.threads[threadId]?.find((m) => m.id === taskId);
    const helper = agent === "pm-codex" ? "release-cloud" : agent === "review-cloud" ? "release-cloud" : "review-cloud";
    if (task && /派出去/.test(task.body)) {
      task.trace = [
        { kind: "thinking", text: "这件事交给别人做更合适，我先派出去，等结果回来再汇总。" },
        { kind: "tool", tool: "Bash", id: "d1", input: `myco message send --to ${helper} --body "核对一下下载页的链接"` },
        { kind: "tool_result", id: "d1", output: "sent" },
        { kind: "text", text: "已经派出去了，等它做完我再汇总给你。" },
        { kind: "turn_end" },
        // A session that receives in its conversation wakes when its wait
        // times out, starts it again and ends the turn: not work on this
        // task (older CLIs upload it; the App leaves it out).
        { kind: "thinking", text: "等待超时了，重新开始等。" },
        { kind: "tool", tool: "Bash", id: "w1", input: "myco inbox wait --timeout 55m --format text" },
        { kind: "tool_result", id: "w1", output: "Command running in background" },
        { kind: "text", text: "还在等消息，继续等。" },
        { kind: "turn_end" },
        { kind: "usage", input_tokens: 2400, output_tokens: 120, duration_ms: 9000 },
      ];
      task.trace_pace = 900;
      later(300, () => { if (["queued", "delivered"].includes(task.status)) Object.assign(task, { status: "acked", updated_at: iso(0) }); });
      let sub = null;
      later(1200, () => {
        const msgs = hub.threads[threadId];
        if (!msgs || task.status === "failed") return;
        sub = makeMessage({ thread_id: threadId, from: agent, to: helper, type: "task", status: "acked", body: "核对一下下载页的链接，三个系统的包都要能下到。", created_at: iso(0), updated_at: iso(0) });
        msgs.push(sub);
      });
      later(9000, () => {
        const msgs = hub.threads[threadId];
        if (!msgs || !sub) return;
        msgs.push(makeMessage({ thread_id: threadId, from: helper, to: agent, type: "reply", status: "done", reply_to: sub.id, body: "三个链接都能下，校验和也对。", created_at: iso(0), updated_at: iso(0) }));
        Object.assign(sub, { status: "done", updated_at: iso(0) });
      });
      later(10500, () => {
        replyTimers.delete(taskId);
        const msgs = hub.threads[threadId];
        if (!msgs || task.status === "failed") return;
        msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done", reply_to: taskId, body: "下载页核对完了：三个系统的链接都能下，校验和也对。", created_at: iso(0), updated_at: iso(0) }));
        Object.assign(task, { status: "done", updated_at: iso(0) });
        releaseAfter(hub, threadId, taskId);
      });
      return;
    }
  }
  // A plan, reported as a checklist, ticked off as it goes.
  const plan = (body) => () => {
    const msgs = hub.threads[threadId];
    if (!msgs) return;
    msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "progress", status: "done", reply_to: taskId, body, created_at: iso(0), updated_at: iso(0) }));
  };
  later(1500, plan("好，先这样做：\n- [~] 读一下相关代码\n- [ ] 动手改\n- [ ] 跑测试，看有没有破坏别的"));
  later(4500, plan("- [x] 读一下相关代码\n- [x] 动手改\n- [~] 跑测试，看有没有破坏别的"));
  later(3000, () => {
    const msgs = hub.threads[threadId];
    if (!msgs) return;
    msgs.push(makeMessage({
      thread_id: threadId, from: agent, to: CONSOLE, type: "progress", status: "done",
      body: "正在处理…", created_at: iso(0), updated_at: iso(0),
    }));
  });
  // Asked to let you choose: the agent asks (myco message ask) and the task
  // waits for the answer instead of replying.
  {
    const task = hub.threads[threadId]?.find((m) => m.id === taskId);
    if (task && /问我|让我选/.test(task.body) && !task.asked_once) {
      later(2500, () => {
        const msgs = hub.threads[threadId];
        if (!msgs || task.status === "failed") return;
        replyTimers.delete(taskId);
        for (const t of timers) clearTimeout(t);
        const q = makeMessage({
          thread_id: threadId, from: agent, to: CONSOLE, type: "progress", status: "done", reply_to: taskId, ask: true,
          body: "开场先放哪个？\n\n1. 先放结果 — 一开头就看到剪好的成片\n2. 先放操作 — 从打开 App 开始一步步来",
          created_at: iso(0), updated_at: iso(0),
        });
        msgs.push(q);
        task.status = "held";
        task.asking = q.id;
        task.updated_at = iso(0);
        askingBy.set(agent, task);
      });
      return;
    }
  }
  // Asked for a screenshot: one comes along the way, with what it shows.
  later(3800, () => {
    const msgs = hub.threads[threadId];
    const task = msgs?.find((m) => m.id === taskId);
    if (!msgs || !/截图|screenshot/i.test(task?.body || "")) return;
    const id = `f_demo_shot_${(state.counter += 1)}`;
    msgs.push(makeMessage({
      thread_id: threadId, from: agent, to: CONSOLE, type: "progress", status: "done", reply_to: taskId,
      body: "首页改好一半了，先看看效果", attachments: [{ file_id: id, name: "首页-进行中.png", size: DEMO_PNG.length, sha256: "", content_type: "image/png" }],
      created_at: iso(0), updated_at: iso(0),
    }));
  });
  later(6000, () => {
    replyTimers.delete(taskId);
    const msgs = hub.threads[threadId];
    if (!msgs || msgs.find((m) => m.id === taskId)?.status === "failed") return; // stopped
    const r = consoleReply();
    msgs.push(makeMessage({
      thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done",
      body: r.body, ...(r.attachments ? { attachments: r.attachments } : {}),
      reply_to: taskId, created_at: iso(0), updated_at: iso(0),
    }));
    const task = msgs.find((m) => m.id === taskId);
    if (task) {
      task.status = "done";
      task.updated_at = iso(0);
      releaseAfter(hub, threadId, taskId);
    }
  });
}

// Messages sent with `after` wait (held) until that task ends, then go out
// in the order they were written.
function releaseAfter(hub, threadId, taskId) {
  const msgs = hub.threads[threadId];
  if (!msgs) return;
  for (const m of msgs) {
    if (m.after !== taskId || !["held", "queued", "delivered"].includes(m.status)) continue;
    m.status = "queued";
    m.updated_at = iso(0);
    hub.recent = [m, ...(hub.recent || [])].slice(0, 20);
    const target = (hub.agents || []).find((a) => a.name === m.to);
    if (target?.binding?.harness === "exec") scheduleExec(hub, threadId, m.to, m.id);
    else scheduleReply(hub, threadId, m.to, m.id);
  }
}

// A shell command that prints a line every 0.7 s, for the live terminal.
const execLines = () => [
  "Resolving dependencies…",
  "go: downloading github.com/chenhg5/myco v0.18.31",
  "ok  	github.com/chenhg5/myco/internal/domain	0.018s",
  "ok  	github.com/chenhg5/myco/internal/app	1.001s",
  "ok  	github.com/chenhg5/myco/internal/runner	1.158s",
  "--- warning: 2 tests skipped on this platform",
  "ok  	github.com/chenhg5/myco/internal/integration	6.265s",
  "PASS",
];
const EXEC_LINE_MS = 700;

function scheduleExec(hub, threadId, agent, taskId) {
  const lines = execLines();
  // The node picks the command up at once (as a runner would): it is running, not waiting.
  setTimeout(() => {
    const task = hub.threads[threadId]?.find((m) => m.id === taskId);
    if (task && ["queued", "delivered"].includes(task.status)) Object.assign(task, { status: "acked", updated_at: iso(0) });
  }, 300);
  const t = setTimeout(() => {
    const msgs = hub.threads[threadId];
    const task = msgs?.find((m) => m.id === taskId);
    if (!task || task.status === "failed") return; // stopped
    const out = lines.filter((l) => !l.startsWith("---")).join("\n") + "\n";
    const err = lines.filter((l) => l.startsWith("---")).join("\n") + "\n";
    msgs.push(makeMessage({
      thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done",
      body: JSON.stringify({ exit_code: 0, stdout: out, stderr: err, duration_ms: lines.length * EXEC_LINE_MS }),
      reply_to: taskId, created_at: iso(0), updated_at: iso(0),
    }));
    task.status = "done";
    task.updated_at = iso(0);
    releaseAfter(hub, threadId, taskId);
  }, lines.length * EXEC_LINE_MS + 500);
  if (typeof t.unref === "function") t.unref();
}

function conversationAgent(msgs) {
  for (const m of msgs) {
    for (const n of [m.from, m.to]) {
      if (n && !n.startsWith("@console:")) return n;
    }
  }
  return "";
}

// This machine: myco ships with the App, cloudflared is present, Tailscale is not.
// cc-switch on this computer (mock): providers and which local agents can pick one
const ccProviders = [
  { ref: "cc-switch:official", id: "official", name: "Claude 官方", app: "claude" },
  { ref: "cc-switch:deepseek", id: "deepseek", name: "DeepSeek", app: "claude", host: "api.deepseek.com", model: "deepseek-v4", current: true },
  { ref: "cc-switch:glm", id: "glm", name: "智谱 GLM", app: "claude", host: "open.bigmodel.cn", model: "glm-5" },
  { ref: "cc-switch:kimi", id: "kimi", name: "Kimi", app: "claude", host: "api.moonshot.cn" },
];
const localProviderAgents = new Map([["docs-claude", { local: true, harness: "claude", provider: "" }]]);

// Resume lines the smoke test makes and revokes; put the demo ones back
// between widths and themes.
const resumeAtStart = new Map();
for (const h of state.hubs) {
  for (const a of h.agents || []) {
    if (a.resume) resumeAtStart.set(`${h.id}/${a.name}`, JSON.parse(JSON.stringify(a.resume)));
  }
}

// Run settings (and what runtime says about the model and effort) the smoke
// test changes; put the demo ones back between widths and themes. It also
// reads back the last body the App sent there.
const runSettingsAtStart = new Map();
for (const h of state.hubs) {
  for (const a of h.agents || []) {
    runSettingsAtStart.set(`${h.id}/${a.name}`, JSON.parse(JSON.stringify({ run_settings: a.run_settings, runtime: a.runtime })));
  }
}
let lastRunSettings = null;

const local = {
  hostname: "Studio.local",
  myco: { path: "/Applications/myco.app/Contents/MacOS/myco", version: "0.16.9" },
  // The default workspace on this computer (older pages read only this one).
  hub: null,
  // `myco hub up --name` runs several on one machine. "notes" is already
  // there, stopped and not in the app; the smoke test adds "studio".
  hubs: [
    { name: "", running: true, local_url: "http://127.0.0.1:7780", version: "0.16.8", added: true },
    { name: "notes", running: false, local_url: "http://127.0.0.1:7790", version: "0.16.8", added: false },
  ],
  tools: {
    cloudflared: { path: "/opt/homebrew/bin/cloudflared" },
    tailscale: null,
  },
  device: "glenn-mac",
  machines: ["Studio Mac"],
  shell_allowed: false,
};
if (SHOWCASE) {
  local.hostname = "glenns-mbp.local";
  if (local.myco) local.myco.version = "0.17.2";
}
// What was there at the start: the smoke test starts another workspace and
// stops this one, and the mock keeps running across widths and themes.
const localHubsAtStart = JSON.parse(JSON.stringify(local.hubs));
// This computer's entries in the app's workspace list, as they start.
const localAppHubsAtStart = JSON.parse(JSON.stringify(state.hubs.filter((h) => h.id === "hub_local")));
local.hub = JSON.parse(JSON.stringify(local.hubs.find((h) => !h.name) || null));

// A node is working only when it acknowledged an open task. `status_text` is
// this mock's stand-in for an acknowledged task; a task it only pulled
// (`delivered`) is still waiting.
const isWorking = (a) => !!(a.status_text || stuckCurrent.get(a.name) || (a.in_progress ?? 0) > 0);
const nodeState = (a) => (a.online ? (isWorking(a) ? "working" : "idle") : "offline");

function totals(agents) {
  const t = { agents: agents.length, working: 0, pending: 0, idle: 0, offline: 0 };
  for (const a of agents) t[nodeState(a)] += 1;
  return t;
}

// A node's demo history with the exec node mac-exec: one thread per
// command, a couple of minutes apart, among them a long one-liner and a
// few polls of what it started (as an agent driving a build does).
const execHistories = {};
function execHistory(agent) {
  if (execHistories[agent]) return execHistories[agent];
  const out = [];
  const build = "(nohup bash -c 'set -e; cd ~/code/myco-app-build; export GOTOOLCHAIN=auto PATH=$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:$PATH; git fetch -q --depth 1 origin main && git checkout -q FETCH_HEAD; git log -1 --oneline; make app VERSION=v0.18.15-preview; rm -rf ~/Applications/myco-preview.app && cp -R bin/myco.app ~/Applications/myco-preview.app; open -n ~/Applications/myco-preview.app; echo PREVIEW-OK' > ~/code/.myco-preview.log 2>&1 &)";
  const poll = "head -1 ~/code/.myco-preview.log; tail -2 ~/code/.myco-preview.log; pgrep -f \"make app\" >/dev/null && echo RUNNING || echo NOT-RUNNING";
  const ok = (stdout, ms = 820) => ({ exit_code: 0, stdout, stderr: "", duration_ms: ms, timed_out: false, error: "" });
  const runs = [
    ["uptime", ok("up 3 days, 4:12, 2 users, load averages: 1.92 2.10 2.31\n")],
    ["df -h /", ok("Filesystem   Size  Used Avail Capacity\n/dev/disk3s1 926Gi 412Gi 498Gi  46%\n")],
    ["cd ~/code/myco && go test ./...", ok(["domain", "app", "infra/sqlite", "infra/notify", "infra/security", "infra/wake", "transport/httpapi", "hubserver", "client", "cli", "runner", "harness/claude", "harness/codex", "integration"]
      .map((p, i) => `ok  \tgithub.com/chenhg5/myco/internal/${p}\t${(0.2 + i * 0.37).toFixed(3)}s`).join("\n") + "\n", 48200)],
    ["pgrep -fl \"mac-sign|notarytool|make app\" || echo idle", ok("idle\n", 313)],
    [build, ok("", 14)],
    [poll, ok("RUNNING\n", 41)],
    [poll, ok("640d77c App: menus open at the pointer on macOS\nbuilt bin/myco.app (0.18.15)\nPREVIEW-OK\nNOT-RUNNING\n", 523)],
    ["myco upgrade", { exit_code: 1, stdout: "", stderr: "download failed: 504 Gateway Timeout\n", duration_ms: 12400, timed_out: false }],
    ["myco --version", { exit_code: 124, stdout: "v0.16.7\n", stderr: "", duration_ms: 60000, timed_out: true, error: "killed after 1m0s (the node's timeout)" }],
  ];
  // Two sittings: the first five commands, then the rest 40 minutes later.
  runs.forEach(([c, res], i) => {
    const t = 6 * 3600 - i * 70 + (i < 5 ? 2400 : 0);
    const th = `th_exec_${agent}_${i}`;
    const task = makeMessage({ thread_id: th, from: agent, to: "mac-exec", type: "task", status: "done", body: c, created_at: iso(t), updated_at: iso(t) });
    out.push(task);
    out.push(makeMessage({ thread_id: th, reply_to: task.id, from: "mac-exec", to: agent, type: "reply", status: "done",
      body: JSON.stringify(res), created_at: iso(t - 30), updated_at: iso(t - 30) }));
  });
  if (process.env.MOCK_E2E) {
    for (const m of out) m.e2e = true;
    // The newest command came from an agent that is not upgraded yet.
    out[out.length - 2].e2e = false;
    out[out.length - 1].e2e = false;
  }
  execHistories[agent] = out;
  return out;
}

// Ids of messages closed with "dismiss" (/app/hubs/{id}/messages/{mid}/dismiss).
const dismissed = new Set();

function agentStatus(a) {
  // The legacy node has a task it picked up and never reported back on.
  const stuck = stuckCurrent.get(a.name);
  // Pulled but not acknowledged: waiting, not at work.
  const delivered = a.delivered || [];
  const working = isWorking(a);
  const state = nodeState(a);
  // Working nodes also have a stale queued answer waiting (dismissable).
  const staleId = "msg_stale_" + a.name;
  const stale = working && !dismissed.has(staleId)
    ? [makeMessage({ id: staleId, thread_id: "th_stale_" + a.name, from: "mac-exec", to: a.name, type: "reply", status: "queued",
        body: '{"exit_code":124,"stdout":"v0.16.7\\n","stderr":"","duration_ms":60028,"timed_out":true,"error":"killed after 1m0s"}',
        created_at: iso(12 * 3600), updated_at: iso(12 * 3600) })]
    : [];
  const atWork = working
    ? [stuck || makeMessage({ thread_id: a.current_thread || "th_release", from: a.current_from || "glenn", to: a.name, status: "acked", body: a.status_text || "处理中…" })]
    : [];
  if (heldForOtherOn && a.name === heldForOther.to && heldForOther.status === "held" && !dismissed.has(heldForOther.id)) {
    return { agent: a, state: a.online ? "pending" : "offline", queued: 0, in_progress: 0, held: 1, asking: 1, receiving: a.online ? "session" : undefined, current: [heldForOther] };
  }
  const asking = askingBy.get(a.name);
  if (asking) {
    return { agent: a, state: "pending", queued: 0, in_progress: 0, held: 1, asking: 1, receiving: "runner", current: [asking] };
  }
  const receiving = a.receiving ?? (!a.online || a.kind === "human" ? undefined : /cloud/.test(a.binding?.harness || "") ? "session" : "runner");
  // As the hub (v0.20.28): an acked task with no life in it (the agent is
  // away, or quiet for 5 minutes under a runner, 30 in a session) is open
  // but not work in progress.
  const limit = receiving === "runner" ? 5 * 60_000 : 30 * 60_000;
  const quiet = atWork.filter((m) => m.status === "acked" && !dismissed.has(m.id)
    && (!a.online || Date.now() - new Date(m.active_at || m.updated_at).getTime() >= limit));
  const live = atWork.filter((m) => m.status === "acked" && !dismissed.has(m.id)).length - quiet.length;
  return {
    agent: a, state: working && live <= 0 ? (a.online ? (stale.length ? "pending" : "idle") : "offline") : state,
    queued: (a.queued ?? (working ? 1 + stale.length : 0)) + delivered.length,
    in_progress: working ? Math.max(0, live) : 0,
    stale: quiet.length,
    stale_tasks: quiet.map((m) => m.id),
    held: 0,
    receiving,
    session_in_use: a.session_in_use || undefined,
    current: atWork.length || delivered.length ? [...atWork.filter((m) => !dismissed.has(m.id)), ...delivered, ...stale] : [],
  };
}

function hubView(h) {
  const view = { id: h.id, name: h.name, url: h.url, ok: h.ok, hub_version: h.hub_version, update_available: h.update_available };
  if (!h.ok) view.error = h.error;
  return view;
}

function overviewFor(h) {
  return {
    generated_at: iso(0),
    viewer: { kind: "admin" },
    online_window_seconds: 120,
    totals: totals(h.agents),
    agents: h.agents.map(agentStatus),
    // Real machines only; cloud sessions are not machines.
    machines: (h.machines || []).map((m) => ({
      ...m,
      agents: h.agents.filter((a) => a.kind !== "machine" && a.machine_id === m.id).map((a) => a.name),
    })),
    recent: h.recent,
  };
}

// People on the first hub (members): you are the owner; MOCK_ME=laohu views as a member.
const people = [
  { id: "mem_glenn", name: "glenn", display_name: "Glenn", role: "owner", created_at: iso(86400 * 30) },
  { id: "mem_laohu", name: "laohu", display_name: "老胡", role: "member", created_at: iso(86400 * 3) },
  { id: "mem_matthew", name: "matthew", display_name: "Matthew", role: "admin", created_at: iso(86400 * 2) },
];
const meName = process.env.MOCK_ME || "glenn";
const audit = [
  { id: "a1", at: iso(60), actor: "laohu", actor_kind: "member", action: "message.task", target: "pm-codex", detail: "msg_x1" },
  { id: "a2", at: iso(900), actor: "matthew", actor_kind: "member", action: "machine.spawn", target: "studio-app-shell", detail: "shell node, commands from pm-codex in ~/code/app (m_studio)" },
  { id: "a3", at: iso(3600), actor: "glenn", actor_kind: "member", action: "member.invite", target: "matthew", detail: "role admin" },
  { id: "a4", at: iso(86400 * 3), actor: "glenn", actor_kind: "member", action: "member.invite", target: "laohu", detail: "role member" },
  { id: "a5", at: iso(86400 * 3 + 60), actor: "admin", actor_kind: "admin", action: "link.create", target: "enr_abc", detail: "link" },
];
const devices = [
  { id: "con_mac", name: "glenns-mbp", active: true, created_at: iso(86400 * 30), last_used_at: iso(5) },
  { id: "con_win", name: "glenn-pc", active: true, created_at: iso(86400 * 2), last_used_at: iso(7200) },
];
if (process.env.MOCK_OWNER_UNNAMED) { people[0].name = "owner"; people[0].display_name = ""; }
function whoami(h) {
  // The showcase's own workspace is yours too (so its computers open).
  if (h.id !== "hub_studio" && !(SHOWCASE && h.id === "hub_home")) return undefined;
  const m = people.find((p) => p.id === "mem_" + meName);
  return { member: m, can_manage: m.role !== "member", device: devices[0] };
}

function appOverview() {
  return {
    generated_at: iso(0),
    hubs: state.hubs.map((h) => h.ok
      ? { id: h.id, name: h.name, ok: true, overview: overviewFor(h), me: whoami(h), e2e: !!h.e2e }
      : { id: h.id, name: h.name, ok: false, error: h.error }),
  };
}


// Projects (studio hub only).
const projects = [
  {
    name: "website", title: "官网改版", goal: "11 月底前上线新版官网：首页、文档站和下载页，中英双语。",
    brief: "## 仓库\n- 代码在 `myco-web`，分支 `redesign`，PR 合到 `main` 才会部署。\n\n## 规则\n- 改文案先改中文，再同步英文。\n- 截图统一 2x。\n\n## 怎么测试\n`npm run build && npm run preview`，在 1280 和 390 两个宽度下看一遍。",
    links: [{ title: "仓库", url: "https://github.com/acme/myco-web" }, { title: "设计稿", url: "https://figma.com/file/demo" }],
    visibility: "members", owner: "glenn", owner_id: "mem_glenn", archived: false, created_at: iso(86400 * 5), updated_at: iso(3600),
    members: [
      { name: "pm-codex", role: "coordinator", note: "拆任务、验收", added_by: "glenn", added_at: iso(86400 * 5) },
      { name: "docs-claude", role: "member", note: "文档站", added_by: "laohu", added_at: iso(86400 * 4) },
      { name: "release-cloud", role: "member", note: "部署和发版", added_by: "glenn", added_at: iso(86400 * 2) },
    ],
    docs: [
      { name: "design.md", body: "# 设计约定\n\n- 主色 #2f6feb\n- 圆角 12px\n", content_type: "text/markdown; charset=utf-8", updated_by: "pm-codex", updated_at: iso(7200) },
      { name: "首页设计稿.pdf", body: "", content_type: "application/pdf", binary: true, size: 2412032, updated_by: "glenn", updated_at: iso(5400) },
      { name: "copy-zh", body: "# 首页文案\n\n让你的 agent 互相派活。\n", updated_by: "docs-claude", updated_at: iso(86400) },
      { name: "memory", body: "# Project memory\n- 下载页的安装命令以 INSTALL.md 为准，不要自己改写 (pm-codex, 2026-11-19)\n- glenn 希望汇报用中文、三条以内 (docs-claude, 2026-11-20)\n", updated_by: "docs-claude", updated_at: iso(86400) },
    ],
  },
  {
    name: "ios", title: "iOS 客户端", goal: "", brief: "", links: [],
    visibility: "team", owner: "matthew", owner_id: "mem_matthew", archived: false, created_at: iso(86400 * 2), updated_at: iso(86400),
    members: [{ name: "ios-shell", role: "member", note: "", added_by: "matthew", added_at: iso(86400) }], docs: [],
  },
];
// Each project's members as they start: the smoke test puts them back
// between widths and themes (it adds members to a project).
const projectMembersAtStart = projects.map((p) => JSON.stringify(p.members));
function memoryOf(p) {
  const d = p.docs.find((x) => x.name === "memory");
  return d ? { name: d.name, body: d.body, size: d.body.length, updated_by: d.updated_by, updated_at: d.updated_at } : { name: "memory", size: 0, updated_at: "0001-01-01T00:00:00Z" };
}
function projectView(hub, p, bodies) {
  return {
    ...p,
    // The computers its coordinators may start agents on (hubs from v0.21.2;
    // the lab hub is older and has none).
    ...(hub.id === "hub_lab" ? {} : { machines: p.machines || [] }),
    // The project's memory (hubs from v0.21.10; the lab hub is older).
    ...(hub.id === "hub_lab" ? {} : { memory: memoryOf(p) }),
    members: p.members.map((m) => ({ ...m, agent: hub.agents.find((a) => a.name === m.name && a.visibility !== "private") })),
    docs: p.docs.map((d) => ({ name: d.name, size: d.size ?? d.body.length, content_type: d.content_type, binary: !!d.binary, updated_by: d.updated_by, updated_at: d.updated_at, ...(bodies && !d.binary ? { body: d.body } : {}) })),
    can_edit: true, can_manage: p.owner_id === "mem_" + meName || meName === "glenn",
  };
}

const npmMirror = " --registry=https://registry.npmmirror.com";
const localAgents = [
  { cli: "claude", name: "Claude Code", state: "ready", path: "/Users/me/.local/bin/claude", version: "2.1.30 (Claude Code)",
    install: "curl -fsSL https://claude.ai/install.sh | bash", install_mirror: "npm install -g @anthropic-ai/claude-code" + npmMirror, login: "claude" },
  // (the showcase: signed in, as on a computer people use)
  { cli: "codex", name: "Codex", state: process.env.MOCK_SHOWCASE ? "ready" : "signed_out", path: "/opt/homebrew/bin/codex", version: "codex-cli 0.60.1",
    install: "npm install -g @openai/codex", install_mirror: "npm install -g @openai/codex" + npmMirror, login: "codex login", needs: "npm" },
  { cli: "gemini", name: "Gemini CLI", state: "missing",
    install: "npm install -g @google/gemini-cli", install_mirror: "npm install -g @google/gemini-cli" + npmMirror, login: "gemini", needs: "npm" },
  { cli: "opencode", name: "opencode", state: "missing",
    install: "curl -fsSL https://opencode.ai/install | bash", install_mirror: "npm install -g opencode-ai" + npmMirror, login: "opencode auth login" },
];

function send(res, status, data) {
  const body = data === undefined ? "" : JSON.stringify(data);
  res.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
  res.end(body);
}

function fail(res, status, error, message, suggestion) {
  send(res, status, { error, message, ...(suggestion ? { suggestion } : {}) });
}

function readBody(req) {
  return new Promise((resolve) => {
    let raw = "";
    req.on("data", (c) => { raw += c; if (raw.length > 1e6) req.destroy(); });
    req.on("end", () => {
      try { resolve(raw ? JSON.parse(raw) : {}); } catch { resolve(null); }
    });
  });
}

// The body as it came (what is passed on to MOCK_OS_URL).
function rawBody(req) {
  return new Promise((resolve) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => resolve(Buffer.concat(chunks)));
  });
}

const cacheStore = new Map(); // key -> {key, hub, data}, oldest first
const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || "/", `http://${req.headers.host || "localhost"}`);
  const parts = url.pathname.split("/").filter(Boolean).map(decodeURIComponent);
  const method = req.method || "GET";
  console.error(`${method} ${url.pathname}`);

  if (method === "OPTIONS") { res.writeHead(204); res.end(); return; }
  // MOCK_DELAY=ms: answers about a hub take this long (a hub far away).
  if (process.env.MOCK_DELAY && parts[1] === "hubs" && parts.length > 3) {
    await new Promise((r) => setTimeout(r, Number(process.env.MOCK_DELAY)));
  }

  // /app/pending-link — a link the App was opened with, read once.
  if (parts[0] === "app" && parts[1] === "pending-link" && parts.length === 2) {
    if (method !== "GET") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const link = pendingLink;
    pendingLink = "";
    return send(res, 200, { link });
  }

  // /app/island/* — the Mac app's notch island sizes its window and opens
  // the app at a member; the mock just says yes.
  if (parts[0] === "app" && parts[1] === "island" && method === "POST") return send(res, 200, { ok: true });

  // /app/export — the App saves a conversation it rendered; the mock keeps
  // the last one for the UI smoke (GET /app/mock/last-export).
  if (parts[0] === "app" && parts[1] === "export" && parts.length === 2 && method === "POST") {
    const b = await readBody(req);
    if (!/\.(md|jsonl)$/.test(b?.name || "")) return fail(res, 400, "invalid_argument", "an export is a .md or .jsonl file");
    state.lastExport = { name: b.name, content: String(b.content || "") };
    return send(res, 200, { path: `/Users/glenn/Downloads/${b.name}` });
  }
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "last-export" && method === "GET") return send(res, 200, state.lastExport || {});

  // /app/mock/run-join — the UI smoke "runs" the last shell-node command on a
  // computer: the folder it ran in shows up as a computer of the workspace.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "run-join" && parts.length === 3 && method === "POST") {
    const j = state.pendingJoin;
    const hub = j && state.hubs.find((h) => h.id === j.hubId);
    if (!hub) return fail(res, 404, "not_found", "no shell-node command to run");
    state.pendingJoin = null;
    state.counter += 1;
    const body = await readBody(req);
    const dir = (typeof body?.dir === "string" && body.dir) || "~/code/wsl-app";
    const base = dir.split("/").filter(Boolean).pop() || "shell";
    const agent = makeAgent({
      name: `wsl-${base}-shell`, kind: "shell", machine_id: "", created_at: iso(0), last_seen_at: iso(0),
      labels: ["machine=wsl"], binding: { harness: "exec", environment: "WSL Ubuntu", workdir: dir }, allow_from: j.allow,
    });
    hub.agents.push(agent);
    return send(res, 200, { agent: agent.name });
  }

  // /app/connect/{job}: a computer being connected (see machines/connect).
  if (parts[0] === "app" && parts[1] === "connect" && parts.length === 3 && method === "GET") {
    const job = connectJobs.get(parts[2]);
    if (!job) return fail(res, 404, "not_found", "no such connection");
    return send(res, 200, connectView(job));
  }

  // /app/mock/run-connect — the UI smoke "runs" the last connect command on
  // a computer: it shows up as a computer of the workspace.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "run-connect" && parts.length === 3 && method === "POST") {
    const j = state.pendingConnect;
    const hub = j && state.hubs.find((h) => h.id === j.hubId);
    if (!hub) return fail(res, 404, "not_found", "no connect command to run");
    state.pendingConnect = null;
    state.counter += 1;
    const m = { ...vpsMachine(), id: `m_vps${state.counter}`, name: `新服务器 ${state.counter}`, hostname: `new-vps-${state.counter}`, last_seen_at: iso(0) };
    hub.machines = [...(hub.machines || []), m];
    return send(res, 200, { machine: m.id });
  }

  // /app/mock/stuck — the smoke test puts the stale demo task back: it stops
  // and resends it, and the mock keeps running across widths and themes.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "stuck" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    localSessions.forEach((x, k) => { if (localJoinedAtStart[k]) x.joined = localJoinedAtStart[k]; else delete x.joined; });
    heldForOther.status = "held";
    delete heldForOther.error;
    dismissed.delete(heldForOther.id);
    projects.forEach((p, k) => { if (projectMembersAtStart[k]) p.members = JSON.parse(projectMembersAtStart[k]); });
    const hub = state.hubs.find((h) => h.id === "hub_studio") || state.hubs[0];
    stuckTask.status = "acked";
    delete stuckTask.error;
    stuckTask.created_at = iso(46 * 60);
    stuckTask.updated_at = iso(45 * 60);
    stuckTask.active_at = iso(45 * 60);
    // The command another agent left on mac-exec: the sidebar's amber dot and
    // the terminal's bar come back too, because the smoke test stops them.
    machineStuckTask.status = "acked";
    delete machineStuckTask.error;
    machineStuckTask.created_at = iso(65 * 60);
    machineStuckTask.updated_at = iso(60 * 60);
    machineStuckTask.active_at = iso(60 * 60);
    stuckCurrent.set("mac-exec", machineStuckTask);
    // Keep what was sent there since (a real hub never drops messages, and
    // the app may still show them): only the demo tasks go back.
    // Tasks sent there since count as finished, so the demo tasks are the
    // open ones again.
    const keep = (id, fixed) => [...fixed, ...(hub.threads[id] || []).filter((m) => !fixed.includes(m)).map((m) => {
      if (m.type === "task" && !["done", "failed"].includes(m.status)) Object.assign(m, { status: "done", updated_at: iso(60) });
      return m;
    })];
    stuckProgress.created_at = stuckProgress.updated_at = iso(40 * 60);
    hub.threads.th_console_stuck = keep("th_console_stuck", [stuckTask, stuckProgress]);
    stuckOlderTask.status = "acked";
    delete stuckOlderTask.error;
    stuckOlderTask.created_at = iso(19 * 60 * 60);
    stuckOlderTask.updated_at = iso(19 * 60 * 60);
    stoppedAfterTask.status = "failed";
    stoppedAfterTask.error = "stopped by the sender";
    stoppedAfterTask.created_at = iso(2 * 60 * 60);
    stoppedAfterTask.updated_at = iso(2 * 60 * 60);
    hub.threads.th_console_stuck_stopped = keep("th_console_stuck_stopped", [stuckOlderTask, stoppedAfterTask]);
    // The task it never started on, without what was sent again since.
    hub.threads.th_grok_exhausted = [grokExhaustedTask];
    // What the previous pass sent legacy-codex (resends, the text-only and
    // tool-call tasks) goes too, so each window size starts the same.
    for (const [id, msgs] of Object.entries(hub.threads || {})) {
      if (id === "th_console_stuck" || id === "th_console_stuck_stopped") continue;
      if (msgs.some((m) => m.from === "legacy-codex" || m.to === "legacy-codex")) delete hub.threads[id];
      // Your chats with review-cloud: the empty-chat step needs it empty.
      else if (msgs.some((m) => (m.from === "review-cloud" && m.to.startsWith("@console:")) || (m.to === "review-cloud" && m.from.startsWith("@console:")))) delete hub.threads[id];
    }
    const consoleWithReview = (m) => (m.from === "review-cloud" && m.to.startsWith("@console:")) || (m.to === "review-cloud" && m.from.startsWith("@console:"));
    hub.recent = (hub.recent || []).filter((m) => m.from !== "legacy-codex" && m.to !== "legacy-codex" && !consoleWithReview(m));
    return send(res, 200, { ok: true });
  }

  // /app/mock/resume — the smoke test makes and revokes a resume line; put
  // the demo ones back between widths and themes.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "resume" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    for (const h of state.hubs) {
      for (const a of h.agents || []) {
        const was = resumeAtStart.get(`${h.id}/${a.name}`);
        if (was) a.resume = JSON.parse(JSON.stringify(was)); else delete a.resume;
      }
    }
    return send(res, 200, { ok: true });
  }

  // /app/mock/handovers — the smoke test makes a take-over command (a
  // waiting handover) and may take one over on a computer; start each width
  // and theme from the seeded ones.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "handovers" && parts.length === 3 && method === "POST") {
    state.handovers = {};
    return send(res, 200, { ok: true });
  }

  // /app/quick/* — the Mac app's own routes for the ⌘⇧Space panel
  // (quick_darwin.go): here they only note what the panel asked for, and
  // GET /app/mock/quick returns the last "open the chat".
  if (parts[0] === "app" && parts[1] === "quick" && parts.length === 3 && method === "POST") {
    const body = await readBody(req);
    if (parts[2] === "open") state.quickOpen = body;
    return send(res, 200, { ok: true });
  }
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "quick" && parts.length === 3) {
    const last = state.quickOpen || null;
    if (method === "DELETE") state.quickOpen = null;
    return send(res, 200, { open: last });
  }

  // /app/mock/superagent-ask — the superagent asks to do something that
  // needs your approval (body: { hub, what, reason }); DELETE: none again.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "superagent-ask" && parts.length === 3) {
    state.superagents ||= {};
    state.approvals ||= {};
    if (method === "DELETE") {
      for (const pr of projects) delete pr.machines;
      for (const h of state.hubs) {
        const name = state.superagents[h.id];
        delete state.superagents[h.id];
        state.approvals[h.id] = [];
        if (name) h.agents = (h.agents || []).filter((a) => a.name !== name);
      }
      return send(res, 200, { ok: true });
    }
    const body = await readBody(req);
    const hub = state.hubs.find((h) => h.id === (body?.hub || state.hubs[0]?.id));
    const agent = (hub && body?.agent) || (hub && state.superagents[hub.id]);
    if (!agent) return fail(res, 404, "not_found", "no superagent there");
    state.counter += 1;
    const a = { id: `apr_${state.counter}`, agent, what: body.what || "删除项目 old-site", reason: body.reason || "", method: body.method || "DELETE", path: body.path || "/v1/projects/old-site",
      status: "pending", created_at: iso(0), expires_at: new Date(Date.now() + 24 * 3600_000).toISOString() };
    (state.approvals[hub.id] ||= []).push(a);
    return send(res, 201, a);
  }

  // /app/mock/run-settings — the smoke test changes a node's model and
  // effort; put the demo ones back between widths and themes. GET returns
  // the last body the App sent there.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "run-settings" && parts.length === 3) {
    if (method === "GET") return send(res, 200, { last: lastRunSettings });
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    for (const h of state.hubs) {
      for (const a of h.agents || []) {
        const was = runSettingsAtStart.get(`${h.id}/${a.name}`);
        if (!was) continue;
        if (was.run_settings) a.run_settings = JSON.parse(JSON.stringify(was.run_settings));
        else delete a.run_settings;
        if (was.runtime) a.runtime = JSON.parse(JSON.stringify(was.runtime));
        else delete a.runtime;
      }
    }
    lastRunSettings = null;
    return send(res, 200, { ok: true });
  }

  // /app/mock/local — the smoke test starts another workspace on this
  // computer; put the demo ones back between widths and themes.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "local" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    local.hubs = JSON.parse(JSON.stringify(localHubsAtStart));
    local.hub = JSON.parse(JSON.stringify(local.hubs.find((h) => !h.name) || null));
    state.hubs = [
      ...state.hubs.filter((h) => !h.id.startsWith("hub_local")),
      ...JSON.parse(JSON.stringify(localAppHubsAtStart)),
    ];
    return send(res, 200, { ok: true });
  }

  // /app/mock/forks — the smoke test forks PM (Codex); its copies go away
  // between widths and themes.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "forks" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const hub = state.hubs.find((h) => h.id === "hub_studio");
    if (hub) hub.agents = hub.agents.filter((a) => !/^pm-codex-\d+$/.test(a.name));
    projects.forEach((p) => { p.members = p.members.filter((m) => !/^pm-codex-\d+$/.test(m.name)); });
    return send(res, 200, { ok: true });
  }

  // /app/mock/hub-version {version} — how old the studio workspace's myco is.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "hub-version" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const body = await readBody(req);
    const hub = state.hubs.find((h) => h.id === "hub_studio");
    if (hub && body?.version) hub.hub_version = body.version;
    return send(res, 200, { ok: true });
  }

  // /app/mock/held-for-other {on} — Myco (Grok) waits on PM's answer.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "held-for-other" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const body = await readBody(req);
    heldForOtherOn = !!body?.on;
    heldForOther.status = "held";
    delete heldForOther.error;
    dismissed.delete(heldForOther.id);
    return send(res, 200, { ok: true });
  }

  // /app/mock/no-spawn {on} — this computer's part of myco that starts
  // sessions is (not) connected (the smoke test fixes it from the App).
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "no-spawn" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const body = await readBody(req);
    const studio = state.hubs.find((h) => h.id === "hub_studio")?.machines?.find((m) => m.id === "m_studio");
    if (studio) studio.can_spawn = !body.on;
    return send(res, 200, { ok: true });
  }

  // /app/mock/machine — the smoke test deletes the demo machine; put it and its
  // nodes back, because the mock keeps running across widths and themes.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "machine" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const hub = state.hubs.find((h) => h.id === "hub_studio");
    if (hub) {
      const machine = deletableMachine();
      hub.machines = [...(hub.machines || []).filter((m) => m.id !== machine.id), machine];
      hub.agents = hub.agents.filter((a) => a.machine_id !== machine.id);
      hub.agents.push(...deletableAgents());
      // Upgraded from the App in a smoke step: back to its old myco.
      const office = hub.machines.find((m) => m.id === "m_office");
      if (office) office.myco_version = "0.16.6";
    }
    return send(res, 200, { ok: true });
  }

  // /app/hubs
  if (parts[0] === "app" && parts[1] === "hubs" && parts.length === 2) {
    if (method === "GET") return send(res, 200, state.hubs.map(hubView));
    if (method === "POST") {
      const body = await readBody(req);
      if (!body) return fail(res, 400, "invalid_request", "请求内容不是 JSON。");

      // The normal way: paste the link printed by the hub installer.
      if (typeof body.link === "string") {
        let parsed = null;
        try { parsed = new URL(body.link.trim()); } catch { /* handled below */ }
        const m = parsed ? /^\/c\/(myco_console_[A-Za-z0-9_-]+)\/?$/.exec(parsed.pathname) : null;
        if (!parsed || !/^https?:$/.test(parsed.protocol) || !m) {
          return fail(res, 400, "invalid_link", "这不是 App 连接链接",
            "复制 Hub 安装完成后打印的链接，或者用下面的方式手动填写地址和令牌。");
        }
        const url = parsed.origin;
        const existing = state.hubs.find((h) => h.url === url);
        if (existing) {
          existing.token = m[1];
          existing.ok = true;
          delete existing.error;
          return send(res, 200, hubView(existing));
        }
        state.counter += 1;
        const hub = {
          id: `hub_added_${state.counter}`, name: (typeof body.name === "string" && body.name.trim()) || parsed.host,
          url, token: m[1], ok: true, hub_version: "0.16.6", update_available: false,
          agents: [], threads: {}, recent: [],
        };
        state.hubs.push(hub);
        return send(res, 201, hubView(hub));
      }

      if (typeof body.name !== "string" || typeof body.url !== "string" || typeof body.token !== "string") {
        return fail(res, 400, "invalid_request", "name、url 和 token 为必填项");
      }
      if (body.token.trim().length < 8 || !body.token.startsWith("myco_")) {
        return fail(res, 401, "invalid_token", "Hub 拒绝了这个控制台令牌。",
          "请在 Hub 所在机器上运行 myco console token create --name <设备名> 重新创建。");
      }
      const existing = state.hubs.find((h) => h.url === body.url.trim());
      if (existing) {
        existing.token = body.token.trim();
        existing.ok = true;
        delete existing.error;
        return send(res, 200, hubView(existing));
      }
      state.counter += 1;
      const hub = {
        id: `hub_added_${state.counter}`, name: body.name.trim(), url: body.url.trim(),
        token: body.token.trim(), ok: true, hub_version: "0.16.6", update_available: false,
        agents: [], threads: {}, recent: [],
      };
      state.hubs.push(hub);
      return send(res, 201, hubView(hub));
    }
    return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
  }

  // /app/overview
  if (parts[0] === "app" && parts[1] === "overview" && parts.length === 2) {
    if (method !== "GET") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    return send(res, 200, appOverview());
  }

  // /app/cache — what the pages showed last (the real backend encrypts it
  // on disk; here it lives as long as the mock runs: reload the page to see
  // a restart).
  if (parts[0] === "app" && parts[1] === "cache") {
    if (parts.length === 3 && parts[2] === "stats") {
      const bytes = [...cacheStore.values()].reduce((n, e) => n + JSON.stringify(e.data).length, 0);
      return send(res, 200, { enabled: true, entries: cacheStore.size, bytes });
    }
    if (method === "GET") {
      const entries = [...cacheStore.values()].reverse();
      return send(res, 200, { enabled: true, entries });
    }
    if (method === "PUT") {
      const body = await readBody(req);
      for (const k of body?.delete || []) cacheStore.delete(k);
      for (const e of body?.entries || []) { cacheStore.delete(e.key); cacheStore.set(e.key, e); }
      return send(res, 204);
    }
    if (method === "DELETE") { cacheStore.clear(); return send(res, 204); }
  }
  // Remote Desktop: a deterministic UI fixture, never a real host.
  if (parts[0] === "app" && parts[1] === "hubs" && parts[3] === "remote" && method === "POST") {
    const body = await readBody(req);
    if (body.action === "devices") return send(res, 200, [{machine:"mac_studio",name:"Studio Mac",os:"darwin",control:true},{machine:"windows",name:"Windows 工作站",os:"windows",control:false}]);
    if (body.action === "status") return send(res, 200, {protocol:1,version:"0.1.0",installed:false,supported:true,running:false});
    if (["close","disable","disconnect"].includes(body.action)) return send(res, 200, true);
    return fail(res, 501, "unsupported", "The UI preview never installs components or starts remote control.");
  }
  // /app/os: a page using this computer (myco.os). Nothing to use in the
  // mock, unless MOCK_OS_URL names a real App backend to pass it to (go run
  // ./internal/console/devos), with a page to try it (这台电脑).
  if (parts[0] === "app" && parts[1] === "os") {
    if (OS_URL) {
      const up = await fetch(OS_URL + req.url, { method, headers: { "Content-Type": "application/json" }, body: method === "GET" || method === "DELETE" ? undefined : await rawBody(req) });
      res.writeHead(up.status, { "Content-Type": up.headers.get("Content-Type") || "application/json" });
      return res.end(Buffer.from(await up.arrayBuffer()));
    }
    if (parts.length === 2 && method === "GET") return send(res, 200, { os: "mock", arch: "", caps: [] });
    return fail(res, 501, "unsupported", "the mock App does not use this computer");
  }
  // /app/info
  if (parts[0] === "app" && parts[1] === "info" && parts.length === 2) {
    if (method !== "GET") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    return send(res, 200, { version: SHOWCASE ? "v0.18.39" : "v0.16.8-demo", shell: SHELL, dictation: SHELL === "mac", open_url: true });
  }
  // /app/proxy: the App's network proxy (kept here while the mock runs).
  if (parts[0] === "app" && parts[1] === "proxy" && parts.length === 2) {
    if (method === "GET") return send(res, 200, { ...proxySetting, has_password: !!proxyPassword });
    if (method === "PUT") {
      const body = (await readBody(req)) || {};
      if (body.mode === "manual" && (!["http", "https", "socks5"].includes(body.type) || !body.host || !(body.port > 0 && body.port < 65536))) {
        return fail(res, 400, "invalid_argument", "the proxy address is a host name or IP, without http:// or a port");
      }
      proxySetting = body.mode === "manual" ? { mode: "manual", type: body.type, host: body.host, port: body.port, user: body.user || "" } : { mode: body.mode === "none" ? "none" : "system" };
      if (body.clear_password || !body.user) proxyPassword = "";
      else if (body.password) proxyPassword = body.password;
      return send(res, 200, { ...proxySetting, has_password: !!proxyPassword });
    }
  }
  if (parts[0] === "app" && parts[1] === "proxy" && parts[2] === "test" && parts.length === 3) {
    const body = (await readBody(req)) || {};
    // (port 9 stands for a proxy that is not there)
    if (body.port === 9) return send(res, 200, { ok: false, reason: "refused", error: "connection refused" });
    return send(res, 200, { ok: true, ms: 42, status: 200 });
  }
  // A page's files, on this port (another origin than the App's).
  if (parts[0] === "__myco" && parts[1] === "page.js") {
    res.writeHead(200, { "Content-Type": "text/javascript; charset=utf-8" });
    return res.end(pageHelperJS);
  }
  if (parts[0] === "p" && parts.length >= 2) {
    const k = pageKeys.get(parts[1]);
    if (!k) { res.writeHead(404, { "Content-Type": "text/plain" }); return res.end("not found"); }
    const file = parts.slice(2).join("/") || "index.html";
    const body = pageFilesOf(k.page, k.version)[file];
    if (body === undefined) { res.writeHead(404, { "Content-Type": "text/plain" }); return res.end("not found"); }
    const type = file.endsWith(".html") ? "text/html; charset=utf-8" : file.endsWith(".css") ? "text/css" : file.endsWith(".json") ? "application/json" : file.endsWith(".md") ? "text/markdown; charset=utf-8"
      : file.endsWith(".woff2") ? "font/woff2" : file.endsWith(".woff") ? "font/woff" : file.endsWith(".svg") ? "image/svg+xml" : file.endsWith(".png") ? "image/png" : "text/javascript";
    const csp = "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'";
    res.writeHead(200, { "Content-Type": type, "Content-Security-Policy": csp, "X-Content-Type-Options": "nosniff" });
    return res.end(file.endsWith(".html") ? body.replace("<head>", `<head><script src="/__myco/page.js"></script>`) : body);
  }
  // /app/hubs/{id}/templates… — the template library (v0.20.5; MOCK_OLD_HUB: none).
  if (parts[0] === "app" && parts[1] === "hubs" && parts[3] === "templates") {
    if (process.env.MOCK_OLD_HUB || (parts[2] !== "hub_studio" && !SHOWCASE)) return fail(res, 404, "", "404 page not found");
    if (parts.length === 4 && method === "GET") return send(res, 200, { templates: mockTemplates.map(({ guide, ...t }) => t) });
    // (fetching myco's apps: the mock has only the ones in the repo)
    if (parts.length === 5 && parts[4] === "sync" && method === "POST") return send(res, 200, { apps: mockTemplates.length, source: "built in" });
    const t = mockTemplates.find((x) => x.id === decodeURIComponent(parts[4]) || x.slug === decodeURIComponent(parts[4]));
    if (!t) return fail(res, 404, "not_found", `template ${parts[4]} not found`);
    if (parts.length === 5 && method === "GET") return send(res, 200, t);
    if (parts[5] === "files" && method === "GET") {
      const name = parts.slice(6).map(decodeURIComponent).join("/");
      const body = shotOK(name) ? templateFiles.get(t.id)?.[name] : undefined;
      if (!body) return fail(res, 404, "not_found", `${name} is not a screenshot of ${t.slug}`);
      res.writeHead(200, { "Content-Type": /\.png$/i.test(name) ? "image/png" : /\.webp$/i.test(name) ? "image/webp" : "image/jpeg", "Cache-Control": "private, max-age=3600" });
      return res.end(body);
    }
    if (parts.length === 6 && parts[5] === "use" && method === "POST") {
      const body = (await readBody(req)) || {};
      const files = templateFiles.get(t.id);
      const id = `pg_t${(state.counter += 1)}`;
      const p = { id, name: body.name || t.name, project: body.project || "", publisher: "glenn", version: 1, entry: "index.html",
        url: `http://192.168.1.20:7780/pages/${id}`, created_at: iso(0), updated_at: iso(0), can_manage: true,
        versions: [{ n: 1, entry: "index.html", files: Object.keys(files).length, size: 30000, sha256: "x", by: "glenn", at: iso(0) }],
        template: { id: t.id, slug: t.slug, name: t.name, version: t.version, permissions: t.permissions, data: t.data } };
      pages.set(id, p);
      madeFiles.set(id, files);
      if (body.sample) {
        for (const [coll, recs] of Object.entries(JSON.parse(files["sample.json"]))) {
          for (const r of recs) { const { id: rid, ...data } = r; putRecord(id, coll, rid || "", data, null, "glenn"); }
        }
      }
      return send(res, 201, { page: p, template: t });
    }
  }
  // /app/hubs/{id}/pages… — pages agents published (v0.19.24).
  if (parts[0] === "app" && parts[1] === "hubs" && parts[3] === "pages") {
    const mine = parts[2] === "hub_studio";
    if (parts.length === 4 && method === "GET") return send(res, 200, { pages: mine ? [...pages.values()] : [] });
    const p = mine ? pages.get(parts[4]) : undefined;
    if (!p) return fail(res, 404, "not_found", `page ${parts[4]} not found`);
    if (parts.length === 5 && method === "GET") return send(res, 200, p);
    if (parts.length === 5 && method === "DELETE") { pages.delete(p.id); res.writeHead(204); return res.end(); }
    if (parts.length === 6 && parts[5] === "state") {
      const cur = pageStates.get(p.id) || { state: null, version: 0 };
      if (method === "GET") return send(res, 200, cur);
      if (method === "PUT") {
        const body = (await readBody(req)) || {};
        if ((body.if_version || 0) !== cur.version) return send(res, 409, { error: "conflict", message: "the page's state changed", latest: cur });
        const next = { state: body.state ?? null, version: cur.version + 1, updated_by: "glenn", updated_at: iso(0) };
        pageStates.set(p.id, next);
        return send(res, 200, next);
      }
    }
    if (parts[5] === "data") {
      const d = dataOf(p.id);
      const coll = parts[6] || "", rid = parts[7] || "";
      if (parts.length === 6 && method === "GET") {
        const c = url.searchParams.get("collection");
        return send(res, 200, { records: d.records.filter((r) => !c || r.collection === c), seq: d.seq });
      }
      if (parts.length === 8 && method === "GET") {
        const r = d.records.find((x) => x.collection === coll && x.id === rid);
        return r ? send(res, 200, r) : fail(res, 404, "not_found", `no record ${rid} in ${coll}`);
      }
      if ((parts.length === 7 && method === "POST") || (parts.length === 8 && method === "PUT")) {
        const body = (await readBody(req)) || {};
        if (!body.data || typeof body.data !== "object" || Array.isArray(body.data)) return fail(res, 400, "invalid_argument", "a record is a JSON object");
        const out = putRecord(p.id, coll, rid, body.data, body.if_version ?? null, CONSOLE);
        if (out.conflict) return send(res, 409, { error: "conflict", message: `record ${rid} in ${coll} changed`, latest: out.conflict });
        return send(res, 200, out.rec);
      }
      if (parts.length === 8 && method === "DELETE") {
        const i = d.records.findIndex((x) => x.collection === coll && x.id === rid);
        if (i < 0) return fail(res, 404, "not_found", `no record ${rid} in ${coll}`);
        const [gone] = d.records.splice(i, 1);
        d.changes.push({ seq: (d.seq += 1), collection: coll, record: rid, op: "delete", version: gone.version + 1, by: CONSOLE, at: iso(0) });
        res.writeHead(204);
        return res.end();
      }
    }
    if (parts.length === 6 && parts[5] === "changes" && method === "GET") {
      const d = dataOf(p.id);
      const after = Number(url.searchParams.get("after") || 0);
      return send(res, 200, { changes: d.changes.filter((c) => c.seq > after), seq: d.seq });
    }
    if (parts.length === 6 && parts[5] === "open" && method === "POST") {
      const body = (await readBody(req)) || {};
      const version = body.version || p.version;
      const key = `k${Math.random().toString(36).slice(2, 12)}`;
      pageKeys.set(key, { page: p.id, version });
      if (p.live) return send(res, 200, { url: `http://127.0.0.1:${PORT}/p/${key}/`, version: 0, latest: 0, name: p.name, entry: "", live: true, online: p.online, port: p.port });
      return send(res, 200, { url: `http://127.0.0.1:${PORT}/p/${key}/index.html`, version, latest: p.version, name: p.name, entry: "index.html" });
    }
    return fail(res, 404, "not_found", "no such route");
  }
  // (the smoke test: two answers land a moment apart in your chat with an
  // agent, the second with pictures; both long enough to grow in)
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "replies" && method === "POST") {
    const body = (await readBody(req)) || {};
    const hub = state.hubs.find((h) => h.id === "hub_studio");
    const agent = body.agent || "review-cloud";
    const threads = Object.entries(hub.threads).filter(([, ms]) => ms.some((m) => m.from === CONSOLE && m.to === agent));
    let [threadId, msgs] = threads.sort((a, b) => b[1].at(-1).created_at.localeCompare(a[1].at(-1).created_at))[0] || [];
    if (!msgs) {
      // (no chat yet: one, with a task already answered)
      threadId = `th_mock_order_${(state.counter += 1)}`;
      msgs = hub.threads[threadId] = [makeMessage({ thread_id: threadId, from: CONSOLE, to: agent, type: "task", status: "done", body: "把首页改好，截图给我。", created_at: iso(60), updated_at: iso(30) })];
    }
    if (body.prepare) return send(res, 200, { ok: true, thread: threadId });
    const long = (n) => `第 ${n} 段回答：改动已经合进去了，构建和测试都通过。\n\n首页在三种宽度下都看过了，导航栏在窄屏下收起成菜单按钮，下载按钮的位置没变。\n\n发布说明按新功能、修复、文档三类整理好了，措辞你再看一眼。`;
    const tag = String(body.tag || "");
    // One long answer (lines: N), which starts folded in the App.
    if (body.lines) {
      msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done",
        body: Array.from({ length: Number(body.lines) }, (_, i) => `长回复第 ${i + 1} 行${tag}`).join("\n"), created_at: iso(0), updated_at: iso(0) }));
      return send(res, 200, { ok: true, thread: threadId });
    }
    msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done", body: `${long(1)}${tag}`, created_at: iso(0), updated_at: iso(0) }));
    setTimeout(() => {
      msgs.push(makeMessage({ thread_id: threadId, from: agent, to: CONSOLE, type: "reply", status: "done", body: `${long(2)}${tag}`, created_at: iso(0), updated_at: iso(0),
        attachments: [{ file_id: `f_demo_shot_order_${(state.counter += 1)}`, name: "首页截图.png", size: DEMO_PNG.length, sha256: "demo", content_type: "image/png" }] }));
    }, Number(body.gap) || 600);
    return send(res, 200, { ok: true, thread: threadId });
  }
  // (the smoke test writes a page's record, as its agent would)
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "pages" && parts[4] === "data" && method === "POST") {
    const body = (await readBody(req)) || {};
    return send(res, 200, putRecord(parts[3], parts[5], body.id || "", body.data || {}, null, "release-cloud").rec);
  }
  // A file a page saved to Downloads (what the smoke test checks).
  if (parts[0] === "app" && parts[1] === "page-download" && method === "POST") {
    const name = url.searchParams.get("name") || "";
    if (!/\.(csv|tsv|xlsx|xls|pdf|docx|pptx|json|md|txt|html|png|jpe?g|svg|gif|webp|zip|ics|mp4|webm|mov|m4a|mp3|wav)$/i.test(name)) return fail(res, 400, "invalid_argument", "a page saves documents, pictures, video and sound");
    const chunks = [];
    for await (const c of req) chunks.push(c);
    const buf = Buffer.concat(chunks);
    state.lastPageDownload = { name, size: buf.length, text: buf.toString("utf8").slice(0, 2000), base64: buf.length < 4e6 ? buf.toString("base64") : "" };
    return send(res, 200, { path: `/Users/glenn/Downloads/${name}` });
  }
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "last-page-download" && method === "GET") return send(res, 200, state.lastPageDownload || {});
  // (the smoke test writes a page's state, as its agent would)
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "pages" && parts[4] === "state" && method === "POST") {
    const cur = pageStates.get(parts[3]) || { state: null, version: 0 };
    const body = (await readBody(req)) || {};
    const next = { state: body.state ?? null, version: cur.version + 1, updated_by: "release-cloud", updated_at: iso(0) };
    pageStates.set(parts[3], next);
    return send(res, 200, next);
  }
  // (the smoke test publishes a new version, as an agent would)
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "pages" && parts[4] === "publish" && method === "POST") {
    const p = pages.get(parts[3]);
    if (!p) return fail(res, 404, "not_found", "no such page");
    p.version += 1;
    p.updated_at = iso(0);
    p.versions.unshift({ n: p.version, entry: "index.html", files: 3, size: 2100, sha256: `a${p.version}`, by: "release-cloud", at: iso(0) });
    return send(res, 200, p);
  }
  // /app/open-url: the desktop app opens a link in the default browser;
  // here it is only noted (GET /app/mock/opened lists them, for the smoke test).
  if (parts[0] === "app" && parts[1] === "open-url" && parts.length === 2) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const body = await readBody(req);
    if (!/^(https?:\/\/[^/]|mailto:)/i.test(body?.url || "")) return fail(res, 400, "invalid_argument", "only http, https and mailto links open in the browser");
    openedLinks.push(body.url);
    return send(res, 200, { ok: true });
  }
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "opened" && parts.length === 3) {
    return send(res, 200, { urls: openedLinks });
  }
  // /app/ui-lang: the pages' language, for the desktop app's tray menu.
  if (parts[0] === "app" && parts[1] === "ui-lang" && parts.length === 2) {
    if (method === "PUT") {
      const b = await readBody(req);
      if (b?.lang !== "zh" && b?.lang !== "en") return fail(res, 400, "invalid_argument", "lang is zh or en");
      state.uiLang = b.lang;
    }
    return send(res, 200, { lang: state.uiLang || "" });
  }
  // /app/voice: a voice chat with one agent, scripted. Starting it plays a
  // short call (it hears you, passes a task to that agent, which shows in
  // the chat marked as said by voice, and answers), so a test can watch the
  // lines and states change.
  if (parts[0] === "app" && parts[1] === "voice") {
    const v = (state.voice ||= { cfg: { tts_speaker: "zh_male_wennuanahu_uranus_bigtts" }, llmKey: true, ttsKey: true, on: false, muted: false, st: "idle", seq: 0, lines: [], timers: [] });
    // The speech services come from the backend; the settings are kept on
    // the workspaces' hubs unless sharing is off (one hub here is too old).
    const providers = () => [{ id: "doubao", name: "豆包语音", name_en: "Doubao Speech", default_speaker: "zh_male_wennuanahu_uranus_bigtts",
      help: "在火山引擎控制台「豆包语音」里开通语音合成 2.0、创建 API Key。音色从列表里选，选好点「试听」；复刻的声音选「其他」，填 S_ 开头的 ID。",
      help_en: "Turn on speech synthesis 2.0 under Doubao Speech in the Volcengine console and create an API key. Pick a voice from the list and try it; for a cloned voice choose Other and type its ID (it starts with S_).",
      voices: doubaoVoices(), has_key: !!v.ttsKey }];
    if (parts[2] === "tried" && method === "GET") return send(res, 200, { speaker: v.tried || "" });
    const sync = () => (v.share === false ? [] : state.hubs.map((h, i) => ({ id: h.id, name: h.name, state: i === 1 ? "unsupported" : "synced" })));
    const info = () => ({ available: true, ready: v.llmKey && v.ttsKey, config: { tts_provider: "doubao", ...v.cfg }, has_llm_key: v.llmKey, has_tts_key: v.ttsKey,
      tts_providers: providers(), share: v.share !== false, sync: sync(),
      session: { on: v.on, state: v.on ? v.st : "idle", muted: v.muted, ...(v.on ? { hub_id: v.hubId, agent: v.agent } : {}) } });
    const line = (kind, text, extra = {}) => {
      v.seq += 1;
      const id = extra.id || v.seq;
      v.lines = v.lines.filter((l) => l.id !== id && (kind === "hearing" || l.kind !== "hearing"));
      v.lines.push({ seq: v.seq, id, kind, text, at: iso(0), ...extra });
      return id;
    };
    const stopAll = () => { v.timers.forEach(clearTimeout); v.timers = []; };
    if (parts.length === 2 && method === "GET") return send(res, 200, info());
    if (parts[2] === "config" && method === "PUT") {
      const b = await readBody(req);
      if (b?.tts_provider && b.tts_provider !== "doubao") return fail(res, 400, "invalid_argument", "unknown speech service " + b.tts_provider);
      for (const k of ["llm_base_url", "llm_model", "tts_provider", "tts_speaker", "tts_rate"]) if (b?.[k] !== undefined) v.cfg[k] = b[k];
      if (typeof b?.share === "boolean") v.share = b.share;
      if (b?.llm_key) v.llmKey = true;
      if (b?.tts_key) v.ttsKey = true;
      if (b?.clear_llm_key) v.llmKey = false;
      if (b?.clear_tts_key) v.ttsKey = false;
      return send(res, 200, info());
    }
    if (parts[2] === "sync" && method === "POST") return send(res, 200, info());
    // Trying a voice: a short tone stands in for the line (the last voice
    // tried is kept for the UI smoke to check).
    if (parts[2] === "preview" && method === "POST") {
      const b = await readBody(req);
      if (!v.ttsKey && !b?.tts_key) return fail(res, 400, "no_key", "no API key for Doubao Speech", "type the speech service's API key first");
      v.tried = b?.tts_speaker || "zh_male_wennuanahu_uranus_bigtts";
      res.writeHead(200, { "Content-Type": "audio/wav", "Cache-Control": "no-store" });
      return res.end(toneWav());
    }
    if (parts[2] === "start" && method === "POST") {
      const b = await readBody(req);
      if (!b?.agent) return fail(res, 400, "invalid_argument", "agent is required: a voice call is with one agent");
      if (!v.llmKey || !v.ttsKey) return fail(res, 400, "not_configured", "add the speech service's key (doubao)", "设置 › 语音");
      const hub = state.hubs.find((h) => h.id === b.hub_id);
      const agent = (hub?.agents || []).find((a) => a.name === b.agent);
      if (!hub || !agent) return fail(res, 404, "not_found", `没有 ${b.agent}`);
      stopAll();
      v.on = true; v.st = "listening"; v.muted = false; v.hubId = hub.id; v.agent = agent.name;
      const name = agent.display_name || agent.name;
      const at = (ms, f) => v.timers.push(setTimeout(f, ms));
      at(600, () => line("hearing", "帮我看一下"));
      at(1100, () => line("hearing", "帮我看一下下载页的链接对不对"));
      at(1500, () => { line("user", "帮我看一下下载页的链接对不对"); v.st = "thinking"; });
      at(2100, () => {
        // What it passed on is in the chat, as if typed, with the mark.
        const threadId = `th_c_${state.counter += 1}`;
        const msg = makeMessage({ thread_id: threadId, from: CONSOLE, to: agent.name, type: "task", status: "queued",
          body: "核对下载页上的每个下载链接，确认都能下到对的文件；有问题列出来。\n\n——\n（经语音转述。原话：帮我看一下下载页的链接对不对）",
          created_at: iso(0), updated_at: iso(0) });
        (hub.threads ||= {})[threadId] = [msg];
        hub.recent = [msg, ...(hub.recent || [])].slice(0, 20);
        scheduleReply(hub, threadId, agent.name, msg.id);
        line("action", `交给 ${name}：核对下载页链接`, { task: { hub_id: hub.id, id: msg.id, agent: agent.name, thread_id: threadId } });
      });
      let said = "";
      const a = { id: 0 };
      at(2400, () => { v.st = "speaking"; a.id = line("assistant", "好，交给它了，"); said = "好，交给它了，"; });
      at(2900, () => { said += "核对完我告诉你。"; line("assistant", said, { id: a.id }); });
      at(3600, () => { v.st = "listening"; });
      return send(res, 200, info());
    }
    if (parts[2] === "stop" && method === "POST") { stopAll(); v.on = false; v.st = "idle"; return send(res, 200, info()); }
    if (parts[2] === "mute" && method === "POST") { const b = await readBody(req); v.muted = !!b?.muted; return send(res, 200, info()); }
    if (parts[2] === "events" && method === "GET") {
      const after = Number(url.searchParams.get("after") || 0);
      // A short wait for something new, like the App's long-poll.
      const until = Date.now() + 1500;
      while (Date.now() < until && !v.lines.some((l) => l.seq > after) && after >= 0) await new Promise((r) => setTimeout(r, 100));
      const events = v.lines.filter((l) => after < 0 || l.seq > after);
      return send(res, 200, { state: v.on ? v.st : "idle", on: v.on, muted: v.muted, seq: v.seq, events });
    }
  }

  // /app/translate: settings (keys only said to exist) and a stand-in
  // translation: 〔译〕 and the text, so a test can tell it was translated.
  if (parts[0] === "app" && parts[1] === "translate") {
    if (parts[2] === "config" && method === "GET") return send(res, 200, { ...translateCfg, keys: { ...translateKeys } });
    if (parts[2] === "config" && method === "PUT") {
      const body = await readBody(req);
      if (!["deepl", "google", "openai"].includes(body?.provider)) return fail(res, 400, "invalid_argument", "unknown translation service");
      translateCfg = { provider: body.provider, base_url: body.base_url || "", model: body.model || "" };
      if (body.clear_key) delete translateKeys[body.provider];
      else if (body.key) translateKeys[body.provider] = true;
      return send(res, 200, { ...translateCfg, keys: { ...translateKeys } });
    }
    if (parts.length === 2 && method === "POST") {
      const body = await readBody(req);
      if (!translateKeys[translateCfg.provider]) return fail(res, 409, "not_configured", "this translation service needs a key");
      translations += 1;
      return send(res, 200, { text: `〔译〕${body.text}`, provider: translateCfg.provider });
    }
    if (parts[2] === "count" && method === "GET") return send(res, 200, { translations });
  }
  // /app/dictation: the Mac app listens itself in the language asked for;
  // the words grow, then it ends. GET (no id) says how many times and in
  // which language, for the smoke.
  if (parts[0] === "app" && parts[1] === "dictation" && parts.length === 2) {
    if (method === "GET") return send(res, 200, { started: dictations, lang: lastDictationLang });
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const body = await readBody(req);
    dictations += 1;
    lastDictationLang = body?.lang || "";
    const id = `d${dictations}`;
    const words = lastDictationLang === "en-US" ? ["Please", "Please check the", "Please check the download page"] : ["帮我", "帮我看一下", "帮我看一下下载页"];
    hearings.set(id, { words, at: Date.now(), stopped: false });
    return send(res, 200, { mode: "listen", id });
  }
  if (parts[0] === "app" && parts[1] === "dictation" && parts.length === 3) {
    const h = hearings.get(decodeURIComponent(parts[2]));
    if (!h) return fail(res, 404, "not_found", "no such listening");
    if (method === "DELETE") { h.stopped = true; res.writeHead(204); return res.end(); }
    const seq = Number(url.searchParams.get("seq") || 0);
    // A word every 400 ms; it ends after the last one (or when stopped).
    await new Promise((r) => setTimeout(r, 400));
    const n = Math.min(h.words.length, Math.floor((Date.now() - h.at) / 400));
    const done = h.stopped || n >= h.words.length;
    return send(res, 200, { seq: Math.max(seq + 1, n + (done ? 1 : 0)), text: n ? h.words[n - 1] : "", final: done, done });
  }

  // /app/update: the app's own update (MOCK_UPDATE=none for "up to date")
  if (parts[0] === "app" && parts[1] === "update" && parts.length === 2) {
    const current = SHOWCASE ? "v0.18.39" : "v0.16.8-demo";
    const latest = process.env.MOCK_UPDATE === "none" || SHOWCASE ? current : "v0.17.2";
    if (method === "GET") {
      return send(res, 200, { current, latest, available: latest !== current, can_install: SHELL === "mac" });
    }
    if (method === "POST") {
      await new Promise((r) => setTimeout(r, 1200));
      return send(res, 200, { installed: latest, relaunching: true });
    }
    return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
  }

  // /app/local[...]
  if (parts[0] === "app" && parts[1] === "local") {
    if (parts.length === 2 && method === "GET") return send(res, 200, local);
    if (parts.length === 3 && parts[2] === "shell-access" && method === "POST") {
      const body = await readBody(req);
      local.shell_allowed = !!body.allow;
      return send(res, 200, { shell_allowed: local.shell_allowed });
    }

    // /app/local/ssh-hosts and /app/local/wsl: computers this one can reach
    // (ConnectOver.tsx). tokyo-vps is the VPS already connected.
    if (parts.length === 3 && parts[2] === "ssh-hosts" && method === "GET") {
      await sleep(300);
      return send(res, 200, { hosts: sshHosts.map((h) => ({ ...h, connected: h.name === "tokyo-vps" ? "m_vps" : undefined })) });
    }
    // /app/local/hub-over-ssh: set up a workspace on a server (ConnectOver,
    // purpose hub); old-pi has no passwordless sudo.
    if (parts.length === 3 && parts[2] === "hub-over-ssh" && method === "POST") {
      const body = await readBody(req);
      const target = String(body.target || "").trim();
      if (!target || /^-|[\s'"`;|&$<>]/.test(target)) return fail(res, 400, "invalid_argument", "give the server as an SSH name or user@host");
      state.counter += 1;
      const job = { id: `cj_${state.counter}`, kind: "hub", route: { via: "ssh", target }, started: Date.now() };
      connectJobs.set(job.id, job);
      return send(res, 200, connectView(job));
    }
    // /app/local/claude-auth: how agents started here sign in to Claude.
    if (parts.length === 3 && parts[2] === "claude-auth") {
      if (method === "POST") {
        const body = await readBody(req);
        if (body.use === "shell") { claudeAuth.using = "shell"; claudeAuth.using_name = "api.magpie.example"; }
        else if (body.use === "account") { claudeAuth.using = ""; delete claudeAuth.using_name; }
        else if (String(body.use || "").startsWith("cc-switch:")) { claudeAuth.using = body.use; claudeAuth.using_name = "Kimi"; }
        else return fail(res, 400, "invalid_argument", "use is account, shell or cc-switch:<id>");
      }
      return send(res, 200, claudeAuth);
    }
    if (parts.length === 3 && parts[2] === "wsl" && method === "GET") {
      return send(res, 200, { supported: true, distros: [
        { name: "Ubuntu-22.04", default: true, running: true, version: 2 },
        { name: "Debian", running: false, version: 2 },
      ] });
    }

    // /app/local/agents: the agent CLIs on this computer (LocalAgents.tsx).
    if (parts.length === 3 && parts[2] === "agents" && method === "GET") {
      return send(res, 200, { agents: localAgents });
    }
    if (parts.length === 5 && parts[2] === "agents" && parts[4] === "terminal" && method === "POST") {
      const a = localAgents.find((x) => x.cli === decodeURIComponent(parts[3]));
      if (!a) return send(res, 404, { error: "not_found", message: "no such agent CLI" });
      const body = await readBody(req);
      const command = { install: a.install, install_mirror: a.install_mirror, login: a.login }[body.action];
      if (!command) return send(res, 400, { error: "invalid_argument", message: "action must be install, install_mirror or login" });
      // The mock "finishes" right away: the next check shows it done.
      if (body.action === "login") a.state = "ready";
      else { a.state = a.cli === "codex" ? "signed_out" : "ready"; a.version = "1.0.0"; a.path = `/usr/local/bin/${a.cli}`; }
      return send(res, 200, { opened: a.cli !== "opencode", command });
    }

    // /app/local/providers and /app/local/agents/{name}/provider (cc-switch)
    if (parts.length === 3 && parts[2] === "providers" && method === "GET") {
      return send(res, 200, { cc_switch: true, providers: ccProviders });
    }
    if (parts.length === 5 && parts[2] === "agents" && parts[4] === "provider") {
      const name = decodeURIComponent(parts[3]);
      if (!localProviderAgents.has(name)) return method === "GET" ? send(res, 200, { local: false, provider: "" }) : send(res, 404, { error: "not_found", message: "this agent does not run on this computer" });
      if (method === "PUT") {
        const body = await readBody(req);
        const cur = { local: true, harness: "claude", provider: body.manual ? "env_file" : body.provider || "" };
        if (body.manual) cur.env_host = new URL(body.manual.base_url).host;
        localProviderAgents.set(name, cur);
        // what the runner would report on its next message
        for (const h of state.hubs) for (const a of h.agents || []) if (a.name === name && a.runtime) {
          const p = ccProviders.find((x) => x.ref === cur.provider);
          a.runtime.provider = p ? p.name : cur.env_host || ccProviders.find((x) => x.current)?.name;
          a.runtime.provider_source = p ? "pinned" : cur.env_host ? "env_file" : "machine";
        }
      }
      return send(res, 200, localProviderAgents.get(name));
    }

    // /app/local/sessions — Claude Code / Codex transcripts on this computer.
    if (parts.length === 3 && parts[2] === "sessions" && method === "GET") {
      return send(res, 200, { sessions: localSessions });
    }

    if (parts.length === 3 && parts[2] === "myco" && method === "POST") {
      if (!local.myco) {
        await sleep(2000);
        local.myco = { path: "/Users/glenn/.local/bin/myco", version: "0.16.8" };
      }
      return send(res, 200, local.myco);
    }

    // /app/local/myco/upgrade — myco brought up to date here and its runner
    // restarted: this computer starts sessions again.
    if (parts.length === 4 && parts[2] === "myco" && parts[3] === "upgrade" && method === "POST") {
      await sleep(1200);
      const studio = state.hubs.find((h) => h.id === "hub_studio")?.machines?.find((m) => m.id === "m_studio");
      if (studio) studio.can_spawn = true;
      return send(res, 200, { from: "v0.20.19", version: "v0.20.24", restarted: ["runner", "hub"] });
    }

    if (parts.length === 3 && parts[2] === "runner" && method === "POST") {
      await new Promise((r) => setTimeout(r, 600));
      return send(res, 200, { runner: "running" });
    }
    if (parts.length === 3 && parts[2] === "hub" && method === "POST") {
      const body = await readBody(req);
      const tunnel = body?.tunnel;
      if (!["cloudflare", "tailscale", "none"].includes(tunnel)) {
        return fail(res, 400, "invalid_request", "tunnel 必须是 cloudflare、tailscale 或 none。");
      }
      if (!local.myco) {
        return fail(res, 409, "myco_missing", "还没有安装 myco 命令行。", "先在这个页面点“安装”。");
      }
      const hostname = typeof body.hostname === "string" ? body.hostname.trim() : "";
      if (tunnel === "cloudflare" && !local.tools.cloudflared) {
        // The App gets cloudflared itself (cloudflared.go).
        await sleep(1500);
        local.tools.cloudflared = { path: "/Users/glenn/.local/bin/cloudflared" };
      }
      if (tunnel === "tailscale" && !local.tools.tailscale) {
        return fail(res, 409, "tailscale_missing", "没有找到 Tailscale。", "先安装 Tailscale 并为这台设备开启 Funnel。");
      }
      if (tunnel === "cloudflare" && hostname && !/^[a-z0-9.-]+$/i.test(hostname)) {
        return fail(res, 400, "invalid_hostname", "域名看起来不对。", "例如 hub.example.com。");
      }

      // Which hub on this computer; "" (or none) is the default one.
      const name = typeof body.name === "string" ? body.name.trim() : "";
      if (name && !/^[a-z][a-z0-9-]*$/.test(name)) {
        return fail(res, 400, "invalid_name", "工作区名只能用小写字母、数字和横线。", "比如 studio、team-notes。");
      }

      await sleep(tunnel === "none" ? 2000 : 4000);

      const publicUrl = tunnel === "none"
        ? null
        : tunnel === "tailscale"
          ? "https://glenn-mac.tail1a2b.ts.net"
          : hostname
            ? `https://${hostname}`
            : "https://quiet-forest-4821.trycloudflare.com";
      const warnings = tunnel === "none"
        ? ["只在本机和同一局域网内可用。"]
        : tunnel === "tailscale"
          ? ["请确认已在这台设备上开启 Funnel。"]
          : hostname
            ? ["请确认域名已托管在 Cloudflare，并且执行过一次 cloudflared tunnel login。"]
            : ["临时地址会在 Hub 重启后变化。", "这台电脑休眠或关机时，其他机器会连不上。"];

      let entry = local.hubs.find((h) => h.name === name);
      if (!entry) {
        entry = {
          name, running: false, version: "0.16.8", added: false,
          local_url: `http://127.0.0.1:${7790 + local.hubs.length}`,
        };
        local.hubs.push(entry);
      }
      entry.running = true;
      entry.added = true;
      entry.version = "0.16.8";
      if (publicUrl) entry.public_url = publicUrl;
      else delete entry.public_url;
      local.hub = JSON.parse(JSON.stringify(local.hubs.find((h) => !h.name) || entry));

      const appUrl = publicUrl || entry.local_url;
      const appId = name ? `hub_local_${name}` : "hub_local";
      let appHub = state.hubs.find((h) => h.id === appId);
      if (!appHub) {
        appHub = {
          id: appId, name: name ? `${local.device} ${name}` : "本机 Hub", url: appUrl,
          token: "myco_console_local_demo", ok: true, hub_version: "0.16.8",
          update_available: false, agents: [], threads: {}, recent: [],
        };
        state.hubs.push(appHub);
      } else {
        appHub.url = appUrl;
        appHub.ok = true;
        delete appHub.error;
      }

      return send(res, 200, {
        hub: hubView(appHub),
        public_url: publicUrl || entry.local_url,
        ...(publicUrl ? { join_url: `${publicUrl}/j/myco_enroll_k3v9q2wd7hx4m8pa` } : {}),
        warnings,
      });
    }

    if (parts.length === 4 && parts[2] === "hub" && parts[3] === "stop" && method === "POST") {
      const body = await readBody(req);
      const name = typeof body?.name === "string" ? body.name.trim() : "";
      const entry = local.hubs.find((h) => h.name === name) || (name ? null : local.hubs.find((h) => !h.name));
      if (entry) {
        entry.running = false;
        delete entry.public_url;
      }
      local.hub = JSON.parse(JSON.stringify(local.hubs.find((h) => !h.name) || null));
      const appHub = state.hubs.find((h) => h.id === (name ? `hub_local_${name}` : "hub_local"));
      if (appHub) {
        appHub.ok = false;
        appHub.error = name ? `${name} 已停止` : "本机 Hub 已停止";
      }
      return send(res, 200, { stopped: true });
    }

    return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
  }

  // /app/hubs/{id}[/...]
  if (parts[0] === "app" && parts[1] === "hubs" && parts.length >= 3) {
    const hub = state.hubs.find((h) => h.id === parts[2]);
    if (!hub) return fail(res, 404, "not_found", "没有这个 Hub");

    if (parts.length === 3 && method === "DELETE") {
      state.hubs = state.hubs.filter((h) => h.id !== hub.id);
      if (hub.id.startsWith("hub_local")) {
        const name = hub.id === "hub_local" ? "" : hub.id.slice("hub_local_".length);
        const entry = local.hubs.find((h) => h.name === name);
        if (entry) entry.added = false;
        local.hub = JSON.parse(JSON.stringify(local.hubs.find((h) => !h.name) || null));
      }
      return send(res, 204);
    }

    // /app/hubs/{id}/local-join — make a session on this computer an agent.
    if (parts.length === 4 && parts[3] === "local-join" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const body = await readBody(req);
      const s = localSessions.find((x) => x.cli === body?.cli && x.session_id === body?.session_id);
      if (!s) {
        return fail(res, 400, "invalid_argument", "pick a session from the list (its directory must exist)");
      }
      if (s.joined) return fail(res, 409, "already_joined", "这个会话已经加入过了。");

      await sleep(2000);
      joinCount += 1;
      state.counter += 1;
      const suffix = s.session_id.replace(/[^a-z0-9]/gi, "").toLowerCase().slice(-4) || String(state.counter);
      const name = `${local.device}-${s.cli}-${suffix}`;
      const machine = (hub.machines || []).find((m) => m.can_spawn);
      const agent = makeAgent({
        name,
        display_name: typeof body.display_name === "string" && body.display_name.trim() ? body.display_name.trim() : undefined,
        description: "从本机会话加入",
        kind: "agent",
        machine_id: machine?.id,
        runtime: {
          cli: s.cli, cli_version: CLI_VERSIONS[s.cli], model: CLI_MODELS[s.cli],
          model_source: "default", access: "workspace",
        },
        labels: [`machine=${local.device}`],
        binding: { harness: s.cli, workdir: s.workdir, session_id: s.session_id },
        created_at: iso(0), last_seen_at: iso(0),
      });
      // Joining the same session again (the smoke test does, at each window
      // size) gets the same agent back, as myco join does.
      if (!hub.agents.some((x) => x.name === name)) hub.agents.push(agent);
      s.joined = { agent: name, hub: hub.url };
      const runner = joinCount % 2 === 1
        ? "running"
        : "not running: start the myco runner on this computer";
      return send(res, 200, { agent: name, created: true, runner });
    }

    // /app/hubs/{id}/conversations — the console's threads, newest first.
    if (parts.length === 4 && parts[3] === "conversations" && method === "GET") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const q = (url.searchParams.get("q") || "").trim().toLowerCase();
      const agentFilter = url.searchParams.get("agent") || "";
      const before = Number(url.searchParams.get("before") || 0);
      const list = Object.entries(hub.threads || {})
        .map(([threadId, msgs]) => {
          if (!msgs.some((m) => m.from.startsWith("@console:"))) return null;
          const agent = conversationAgent(msgs);
          if (agentFilter && agent !== agentFilter) return null;
          if (q && !(
            agent.toLowerCase().includes(q) ||
            msgs.some((m) => (m.body || "").toLowerCase().includes(q))
          )) return null;
          const last = msgs[msgs.length - 1];
          const updated = last.updated_at || last.created_at;
          if (before && new Date(updated).getTime() >= before) return null;
          return {
            thread_id: threadId, agent, updated_at: updated,
            first: msgs[0], last, messages: msgs.length,
          };
        })
        .filter(Boolean)
        .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
      return send(res, 200, { conversations: list.slice(0, 30) });
    }

    // /app/hubs/{id}/messages — a task from the console.
    // /app/hubs/{id}/files?name= — a file to attach to the next message.
    if (parts.length === 4 && parts[3] === "files" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const chunks = [];
      for await (const c of req) chunks.push(c);
      const data = Buffer.concat(chunks);
      const name = url.searchParams.get("name") || "file";
      const id = `f_mock_${state.counter += 1}`;
      const f = { file_id: id, name, size: data.length, sha256: "", content_type: req.headers["content-type"] || "" };
      uploads.set(id, f);
      uploadBytes.set(id, data);
      return send(res, 201, { id, name, size: data.length, content_type: f.content_type });
    }
    // /app/hubs/{id}/files/{fid} — read an attachment back: a picture in
    // place (plain raster types only), anything else as a download.
    if (parts.length === 5 && parts[3] === "files" && method === "GET") {
      const fid = decodeURIComponent(parts[4]);
      let data = uploadBytes.get(fid);
      let meta = uploads.get(fid);
      if (!data && fid.startsWith("f_demo_shot_")) { data = DEMO_PNG; meta = { name: "首页截图.png", content_type: "image/png" }; }
      if (!data && fid.startsWith("f_demo_html_")) {
        data = Buffer.from('<h1>新版首页</h1><p>把 <b>myco</b> 装上，<a href="https://example.com">看文档</a>。</p><script>document.title = "ran"</script>');
        meta = { name: "首页.html", content_type: "text/html" };
      }
      if (!data && fid.startsWith("f_demo_md_")) { data = Buffer.from(demoNotes(Number(fid.slice(10)) || 1)); meta = { name: "发布说明.md", content_type: "text/markdown" }; }
      if (!data && fid.startsWith("f_demo_")) { data = Buffer.from("BUILD SUCCESSFUL\n"); meta = { name: "build.log", content_type: "text/plain" }; }
      if (!data) return fail(res, 404, "not_found", `file ${fid} not found`);
      if (url.searchParams.get("as") === "text") {
        if (/^image\//.test(meta.content_type || "") || data.includes(0)) return fail(res, 415, "not_text", "this file cannot be shown as text");
        res.writeHead(200, { "Content-Type": "text/plain; charset=utf-8", "Content-Length": data.length });
        return res.end(data);
      }
      const inline = /^image\/(png|jpeg|gif|webp|avif|bmp)$/.test(meta.content_type || "") && !url.searchParams.get("download");
      res.writeHead(200, {
        "Content-Type": inline ? meta.content_type : "application/octet-stream",
        "Content-Disposition": `${inline ? "inline" : "attachment"}; filename*=UTF-8''${encodeURIComponent(meta.name)}`,
        "Content-Length": data.length,
        "Cache-Control": "private, max-age=86400, immutable",
      });
      return res.end(data);
    }
    // /app/hubs/{id}/files/{fid}/save — "saved to Downloads".
    if (parts.length === 6 && parts[3] === "files" && parts[5] === "save" && method === "POST") {
      const fid = decodeURIComponent(parts[4]);
      const meta = uploads.get(fid) || (fid.startsWith("f_demo_shot_") ? { name: "首页截图.png" } : fid.startsWith("f_demo_") ? { name: "build.log" } : null);
      if (!meta) return fail(res, 404, "not_found", `file ${fid} not found`);
      return send(res, 200, { path: `/Users/glenn/Downloads/${meta.name}` });
    }

    if (parts.length === 4 && parts[3] === "messages" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const body = await readBody(req);
      const to = typeof body?.to === "string" ? body.to.trim() : "";
      const text = typeof body?.body === "string" ? body.body.trim() : "";
      const target = (hub.agents || []).find(
        (a) => a.name === to && (a.kind === "agent" || a.kind === "shell"),
      );
      if (!target) return fail(res, 404, "not_found", `没有名为 ${to} 的 AI 或 shell 节点`);
      if (!text) return fail(res, 400, "invalid_argument", "body 不能为空。");
      // `after` waits behind an unfinished task: it is held until that one ends.
      const after = typeof body?.after === "string" && body.after.trim() ? body.after.trim() : "";
      let afterTask = null;
      if (after) {
        afterTask = Object.values(hub.threads || {}).flat().find((m) => m.id === after);
        if (!afterTask) return fail(res, 404, "not_found", "after 指向的消息不存在。");
        if (afterTask.type !== "task") return fail(res, 400, "invalid_argument", "after 只能指向一条任务。");
        if (!["queued", "delivered", "acked", "held"].includes(afterTask.status)) afterTask = null; // already ended
      }
      const threadId = afterTask
        ? afterTask.thread_id
        : typeof body.thread_id === "string" && body.thread_id.trim()
          ? body.thread_id.trim()
          : `th_c_${state.counter += 1}`;
      const atts = (Array.isArray(body.attachments) ? body.attachments : []).map((id) => uploads.get(id)).filter(Boolean);
      if (Array.isArray(body.attachments) && atts.length !== body.attachments.length) return fail(res, 404, "not_found", "附件不存在");
      const msg = makeMessage({
        thread_id: threadId, from: CONSOLE, to, type: "task", status: afterTask ? "held" : "queued",
        body: text, created_at: iso(0), updated_at: iso(0),
        ...(body.project && !body.thread_id ? { project: body.project } : {}),
        ...(atts.length ? { attachments: atts } : {}),
        ...(afterTask ? { after: afterTask.id } : {}),
      });
      if (!hub.threads) hub.threads = {};
      if (!hub.threads[threadId]) hub.threads[threadId] = [];
      hub.threads[threadId].push(msg);
      if (afterTask) return send(res, 201, msg); // goes out from releaseAfter
      hub.recent = [msg, ...(hub.recent || [])].slice(0, 20);
      if (target.binding?.harness === "exec") scheduleExec(hub, threadId, to, msg.id);
      else scheduleReply(hub, threadId, to, msg.id);
      return send(res, 201, msg);
    }

    // /app/hubs/{id}/superagent and /approvals… — your superagent (v0.21.1)
    // and what it asks you to approve. /app/mock/superagent-ask adds one.
    if (parts[3] === "superagent" || parts[3] === "approvals") {
      if (hub.id === "hub_lab") return fail(res, 404, "not_found", "no such route"); // an older hub
      state.superagents ||= {};
      state.approvals ||= {};
      const list = (state.approvals[hub.id] ||= []);
      const cur = () => (hub.agents || []).find((a) => a.name === state.superagents[hub.id]);
      if (parts[3] === "superagent" && parts.length === 4) {
        if (method === "GET") {
          const a = cur();
          return send(res, 200, a ? { agent: a, pending: list.filter((x) => x.status === "pending").length } : {});
        }
        if (method === "PUT") {
          const body = await readBody(req);
          const a = (hub.agents || []).find((x) => x.name === body?.agent);
          if (!a) return fail(res, 404, "not_found", `no agent ${body?.agent}`);
          if (a.owner_id !== "mem_glenn") return fail(res, 403, "permission_denied", `${a.name} is not your agent; a superagent must be one you added`);
          if (state.superagents[hub.id] && state.superagents[hub.id] !== a.name) {
            for (const x of list) if (x.status === "pending") x.status = "withdrew";
          }
          state.superagents[hub.id] = a.name;
          return send(res, 200, { agent: a });
        }
        if (method === "DELETE") {
          for (const x of list) if (x.status === "pending") x.status = "withdrew";
          delete state.superagents[hub.id];
          return send(res, 204);
        }
      }
      if (parts[3] === "approvals" && parts.length === 4 && method === "GET") {
        const status = url.searchParams.get("status") || "pending";
        return send(res, 200, { approvals: list.filter((x) => status === "all" || x.status === status) });
      }
      if (parts[3] === "approvals" && parts.length === 6 && method === "POST") {
        const a = list.find((x) => x.id === parts[4]);
        if (!a) return fail(res, 404, "not_found", "no such approval");
        if (a.status !== "pending") return fail(res, 409, "conflict", `already ${a.status}`);
        a.status = parts[5] === "approve" ? "done" : "denied";
        a.decided_by = "glenn";
        a.decided_at = iso(0);
        if (a.status === "done") a.result = 200;
        // A coordinator's request for a computer: carried out as you.
        const pm = a.status === "done" && a.method === "PUT" && /^\/v1\/projects\/([^/]+)\/machines\/([^/]+)$/.exec(a.path);
        if (pm) {
          const list = hub.id === "hub_studio" ? projects : (hub.projects ||= []);
          const pr = list.find((x) => x.name === decodeURIComponent(pm[1]));
          if (pr) pr.machines = [...(pr.machines || []).filter((x) => x !== pm[2]), decodeURIComponent(pm[2])];
        }
        return send(res, 200, a);
      }
      return fail(res, 404, "not_found", "no such route");
    }

    // /app/hubs/{id}/workflows… and /workflow-runs… — workflows (v0.21):
    // the mock keeps them per hub; a run walks its steps on timers (each
    // agent or computer step done after a moment, the first edge taken),
    // stops at a decision until one is made.
    if (parts[3] === "workflows" || parts[3] === "workflow-runs" || parts[3] === "workflow-decisions") {
      if (hub.id === "hub_lab") return fail(res, 404, "not_found", "no such route"); // an older hub
      state.workflows ||= {};
      const wf = (state.workflows[hub.id] ||= seedWorkflows(hub));
      const view = (w, def) => ({ id: w.id, name: w.name, description: w.def.description, project: w.def.project, owner: "glenn", version: w.version,
        enabled: w.enabled, steps: w.def.steps.length, created_at: w.created_at, updated_at: w.updated_at,
        active: wf.runs.filter((r) => r.workflow_id === w.id && (r.status === "running" || r.status === "waiting")).length,
        last_run: (() => { const r = wf.runs.find((x) => x.workflow_id === w.id); return r ? runView(r, false) : undefined; })(),
        ...(def ? { definition: w.def } : {}) });
      const runView = (r, full) => {
        const { steps, events, def, artifacts, evaluations, ...rest } = r;
        const current = steps.filter((x) => x.status === "running" || x.status === "waiting").map((x) => x.step);
        const duration_ms = wfMs(r.created_at, r.ended_at);
        return full ? { ...rest, current, duration_ms, steps: steps.map(wfStepView), events, definition: def, metrics: wfMetrics(r), artifacts: artifacts || [], evaluations: evaluations || [] }
          : { ...rest, current, duration_ms };
      };
      const findWf = (ref) => wf.list.find((w) => w.id === ref || w.name === ref);
      if (parts[3] === "workflow-decisions" && method === "GET") {
        const out = [];
        for (const r of wf.runs) for (const st of r.steps) if (st.status === "waiting") out.push({ run: runView(r, false), step: st });
        return send(res, 200, { decisions: out });
      }
      if (parts[3] === "workflows") {
        if (parts.length === 4 && method === "GET") return send(res, 200, { workflows: wf.list.map((w) => view(w, false)) });
        if (parts.length === 4 && method === "POST") {
          const b = await readBody(req);
          const d = b?.definition;
          if (!d?.name || !Array.isArray(d.steps) || !d.steps.length) return fail(res, 400, "invalid_argument", "the workflow has 1 problem(s): steps: a workflow needs at least one step");
          let w = wf.list.find((x) => x.name === d.name);
          if (w) { w.def = d; w.version += 1; w.updated_at = iso(0); return send(res, 200, { workflow: view(w, true), created: false, changed: true }); }
          state.counter += 1;
          w = { id: `wf_${state.counter}`, name: d.name, def: d, version: 1, enabled: true, created_at: iso(0), updated_at: iso(0) };
          wf.list.unshift(w);
          return send(res, 201, { workflow: view(w, true), created: true, changed: true });
        }
        if (parts.length === 5 && parts[4] === "validate" && method === "POST") return send(res, 200, { valid: true });
        const w = findWf(decodeURIComponent(parts[4] || ""));
        if (!w) return fail(res, 404, "not_found", "workflow not found");
        if (parts.length === 5 && method === "GET") return send(res, 200, view(w, true));
        if (parts.length === 5 && method === "PATCH") { const b = await readBody(req); if (typeof b?.enabled === "boolean") w.enabled = b.enabled; return send(res, 200, view(w, true)); }
        if (parts.length === 5 && method === "DELETE") { wf.list.splice(wf.list.indexOf(w), 1); wf.runs = wf.runs.filter((r) => r.workflow_id !== w.id); res.writeHead(204); return res.end(); }
        if (parts.length === 6 && parts[5] === "stats" && method === "GET") {
          const q = new URL(req.url, "http://x").searchParams;
          return send(res, 200, wfStats(wf, w, Number(q.get("version")) || 0));
        }
        if (parts.length === 6 && parts[5] === "runs" && method === "POST") {
          if (!w.enabled) return fail(res, 409, "invalid_state", `workflow ${w.name} is turned off`);
          const b = await readBody(req);
          const required = w.def.inputs?.required || [];
          const missing = required.find((k) => !b?.inputs?.[k]);
          if (missing) return fail(res, 400, "invalid_argument", `the inputs do not fit: ${missing} is missing`);
          return send(res, 201, runView(wfStartRun(hub, wf, w, b?.inputs || {}, "manual"), true));
        }
        return fail(res, 404, "not_found", "no such route");
      }
      // workflow-runs
      if (parts.length === 4 && method === "GET") {
        const q = new URL(req.url, "http://x").searchParams;
        let runs = wf.runs;
        if (q.get("workflow")) { const w = findWf(q.get("workflow")); runs = runs.filter((r) => r.workflow_id === w?.id); }
        if (q.get("active") === "true") runs = runs.filter((r) => r.status === "running" || r.status === "waiting");
        if (q.get("status")) runs = runs.filter((r) => r.status === q.get("status"));
        return send(res, 200, { runs: runs.slice(0, Number(q.get("limit")) || 50).map((r) => runView(r, false)) });
      }
      if (parts.length === 5 && parts[4] === "compare" && method === "GET") {
        const q = new URL(req.url, "http://x").searchParams;
        const ra = wf.runs.find((x) => x.id === q.get("a")), rb = wf.runs.find((x) => x.id === q.get("b"));
        if (!ra || !rb) return fail(res, 404, "not_found", "workflow run not found");
        return send(res, 200, wfCompare(ra, rb, (x) => runView(x, false)));
      }
      const r = wf.runs.find((x) => x.id === parts[4]);
      if (!r) return fail(res, 404, "not_found", "workflow run not found");
      if (parts.length === 5 && method === "GET") return send(res, 200, runView(r, true));
      if (parts.length === 8 && parts[5] === "steps" && parts[7] === "log" && method === "GET") {
        const id = decodeURIComponent(parts[6]);
        const st = r.steps.find((x) => x.id === id) || [...r.steps].reverse().find((x) => x.step === id);
        if (!st) return fail(res, 404, "not_found", `run ${r.id} has no step ${id}`);
        return send(res, 200, { run: r.id, step: wfStepView(st), entries: wfStepLog(hub, r, st) });
      }
      if (parts.length === 6 && parts[5] === "evaluations" && method === "POST") {
        const b = await readBody(req);
        if (!/^[A-Za-z][A-Za-z0-9_.-]{0,63}$/.test(b?.name || "")) return fail(res, 400, "invalid_argument", `name "${b?.name || ""}": use letters, digits, _ . -`);
        if (typeof b.value !== "number" || !Number.isFinite(b.value)) return fail(res, 400, "invalid_argument", "the value must be a number");
        state.counter += 1;
        const e = { id: state.counter, run: r.id, name: b.name, value: b.value, ...(b.unit ? { unit: b.unit } : {}), ...(b.direction ? { direction: b.direction } : {}),
          ...(b.note ? { note: b.note } : {}), ...(b.step ? { step: b.step } : {}), by: "glenn", at: iso(0) };
        (r.evaluations ||= []).push(e);
        return send(res, 201, e);
      }
      if (parts.length === 6 && method === "POST") {
        const b = await readBody(req).catch(() => ({}));
        if (parts[5] === "cancel") {
          if (r.status === "running" || r.status === "waiting") wfEnd(r, "cancelled", "glenn 取消了");
          return send(res, 200, runView(r, true));
        }
        if (parts[5] === "retry") {
          if (r.status !== "failed" && r.status !== "cancelled") return fail(res, 409, "invalid_state", "run is still going");
          const from = b?.from || [...r.steps].reverse().find((x) => x.status === "failed" || x.status === "cancelled")?.step;
          if (!from) return fail(res, 400, "invalid_argument", "no step failed");
          r.status = "running"; r.error = undefined; r.ended_at = undefined;
          r.events.push({ at: iso(0), kind: "retry", step: from, text: "glenn 从这一步重新开始" });
          wfActivate(hub, r, from);
          return send(res, 200, runView(r, true));
        }
        if (parts[5] === "decide") {
          const st = r.steps.find((x) => x.step === b?.step && x.status === "waiting");
          if (!st) return fail(res, 409, "invalid_state", "not waiting for a decision");
          if (!(st.options || []).includes(b.choice)) return fail(res, 400, "invalid_argument", `choose one of: ${(st.options || []).join(", ")}`);
          st.status = "done"; st.ended_at = iso(0); st.output = { choice: b.choice, note: b.note || "", by: "glenn" };
          st.summary = b.choice + (b.note ? `：${b.note}` : "");
          r.events.push({ at: iso(0), kind: "decided", step: st.step, text: `glenn 选了「${b.choice}」` });
          if (b.choice === st.options[0]) wfNext(hub, r, st);
          else wfEnd(r, "failed", `选了「${b.choice}」`);
          return send(res, 200, runView(r, true));
        }
      }
      return fail(res, 404, "not_found", "no such route");
    }

    // /app/hubs/{id}/automations… — automations on the hub (v0.19.22): the
    // mock keeps them per hub; a run sends a task from you, marked.
    if (parts[3] === "automations") {
      if (hub.id === "hub_lab") return fail(res, 404, "not_found", "no such route"); // an older hub
      state.automations ||= {};
      const list = (state.automations[hub.id] ||= seedAutomations(hub));
      const find = () => list.find((a) => a.id === parts[4]);
      const runOnce = (a, trigger) => {
        state.counter += 1;
        const run = { id: `run_${state.counter}`, automation_id: a.id, at: iso(0), trigger, outcome: "sent" };
        if (a.workflow) {
          state.workflows ||= {};
          const wfs = (state.workflows[hub.id] ||= seedWorkflows(hub));
          const w = wfs.list.find((x) => x.id === a.workflow);
          const last = wfs.runs.find((x) => x.id === a.last_task);
          if (!w) { run.outcome = "failed"; run.error = "workflow not found"; }
          else if (a.skip_if_busy && last && (last.status === "running" || last.status === "waiting")) { run.outcome = "skipped"; run.error = "the last workflow run it started is not finished"; }
          else { const r = wfStartRun(hub, wfs, w, Object.fromEntries(Object.entries(a.inputs || {}).map(([k, v]) => [k, String(v).replace(/\{\{[^}]*\}\}/g, "")])), "automation"); run.workflow_run = r.id; a.last_task = r.id; }
          a.runs = [run, ...(a.runs || [])];
          a.last_run = run;
          return run;
        }
        const busy = a.skip_if_busy && Object.values(hub.threads || {}).flat().some((m) => m.automation === a.id && ["queued", "delivered", "acked", "held"].includes(m.status));
        if (busy) { run.outcome = "skipped"; run.error = "the last task it sent is not finished"; }
        else {
          const threadId = `th_auto_${state.counter}`;
          const msg = makeMessage({ thread_id: threadId, from: CONSOLE, to: a.to, type: "task", status: "queued", body: a.body.replace("{{payload}}", ""), automation: a.id, created_at: iso(0), updated_at: iso(0) });
          (hub.threads ||= {})[threadId] = [msg];
          hub.recent = [msg, ...(hub.recent || [])].slice(0, 20);
          scheduleReply(hub, threadId, a.to, msg.id);
          run.message_id = msg.id;
        }
        a.runs = [run, ...(a.runs || [])];
        a.last_run = run;
        return run;
      };
      const view = (a) => {
        const { runs, ...rest } = a;
        if (!a.workflow) return rest;
        state.workflows ||= {};
        const wfs = (state.workflows[hub.id] ||= seedWorkflows(hub));
        return { ...rest, workflow_name: wfs.list.find((w) => w.id === a.workflow)?.name || a.workflow };
      };
      const apply = (a, b) => {
        for (const k of ["name", "to", "project", "body", "skip_if_busy", "enabled", "workflow", "inputs"]) if (b?.[k] !== undefined) a[k] = b[k];
        if (a.workflow === "") { delete a.workflow; delete a.inputs; }
        if (b?.schedule) { a.schedule = b.schedule; a.next_run_at = new Date(Date.now() + 3600e3).toISOString(); }
        if (b?.clear_schedule) { delete a.schedule; delete a.next_run_at; }
        let secret;
        if (b?.webhook !== undefined && b.webhook !== a.webhook) { a.webhook = !!b.webhook; if (a.webhook) secret = "whsec_demo_" + Math.random().toString(36).slice(2, 10); }
        if (b?.rotate_secret && a.webhook) secret = "whsec_demo_" + Math.random().toString(36).slice(2, 10);
        a.webhook_url = a.webhook ? `${hub.url.replace(/\/+$/, "")}/hooks/${a.id}` : undefined;
        a.updated_at = iso(0);
        return secret;
      };
      if (parts.length === 4 && method === "GET") return send(res, 200, { automations: list.map(view) });
      if (parts.length === 4 && method === "POST") {
        const b = await readBody(req);
        if (!b?.name || (!b.workflow && (!b?.to || !b?.body)) || (!b.schedule && !b.webhook)) return fail(res, 400, "invalid_argument", "name, to, body (or a workflow) and a schedule or webhook are required");
        state.counter += 1;
        const a = { id: `aut_${state.counter}`, owner: "glenn", from: CONSOLE, webhook: false, skip_if_busy: true, enabled: true, created_at: iso(0), runs: [] };
        const secret = apply(a, b);
        list.unshift(a);
        return send(res, 201, { automation: view(a), ...(secret ? { webhook_secret: secret } : {}) });
      }
      const a = find();
      if (!a) return fail(res, 404, "not_found", "no such automation");
      if (parts.length === 5 && method === "GET") return send(res, 200, { automation: view(a), runs: (a.runs || []).map((r) => (r.request ? { ...r, request: { ...r.request, body: undefined } } : r)) });
      if (parts.length === 7 && parts[5] === "runs" && method === "GET") {
        const run = (a.runs || []).find((r) => r.id === parts[6]);
        return run ? send(res, 200, run) : fail(res, 404, "not_found", "no such run");
      }
      if (parts.length === 5 && method === "PATCH") { const secret = apply(a, await readBody(req)); return send(res, 200, { automation: view(a), ...(secret ? { webhook_secret: secret } : {}) }); }
      if (parts.length === 5 && method === "DELETE") { list.splice(list.indexOf(a), 1); res.writeHead(204); return res.end(); }
      if (parts.length === 6 && parts[5] === "run" && method === "POST") return send(res, 201, runOnce(a, "manual"));
      return fail(res, 404, "not_found", "no such route");
    }

    // /app/hubs/{id}/messages/{mid}/stop — stop an unfinished task.
    if (parts.length === 6 && parts[3] === "messages" && parts[5] === "stop" && method === "POST") {
      // A command one node sent another may not be in a thread the App reads
      // (the terminal lists it from the recipient's current work).
      let task = Object.values(hub.threads || {}).flat().find((m) => m.id === parts[4]);
      if (!task) task = [...stuckCurrent.values()].find((m) => m.id === parts[4]);
      if (!task && parts[4] === heldForOther.id) task = heldForOther;
      if (!task) return fail(res, 404, "not_found", "没有这条消息");
      // Who sent it, or someone who manages the hub.
      if (!task.from.startsWith("@console:") && !whoami(hub)?.can_manage) {
        return fail(res, 403, "permission_denied", "只有发送者或能管理这个工作区的人可以停止。");
      }
      if (!["queued", "delivered", "acked", "held"].includes(task.status)) return fail(res, 409, "invalid_state", "这个任务已经结束了。");
      task.status = "failed";
      task.error = "stopped by the sender";
      task.updated_at = iso(0);
      for (const [name, m] of stuckCurrent) if (m.id === task.id) stuckCurrent.delete(name);
      // The round is over, so what was waiting behind it goes out now.
      releaseAfter(hub, task.thread_id, task.id);
      return send(res, 200, task);
    }

    // /app/hubs/{id}/messages/{mid}/steer — correct an unfinished task.
    if (parts.length === 6 && parts[3] === "messages" && parts[5] === "steer" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const body = await readBody(req);
      const text = typeof body?.body === "string" ? body.body.trim() : "";
      if (!text) return fail(res, 400, "invalid_argument", "body 不能为空。");
      let msgs = null;
      let task = null;
      for (const arr of Object.values(hub.threads || {})) {
        const found = arr.find((m) => m.id === parts[4]);
        if (found) { msgs = arr; task = found; break; }
      }
      if (!msgs || !task) return fail(res, 404, "not_found", "没有这条消息");
      if (!task.from.startsWith("@console:")) {
        return fail(res, 403, "permission_denied", "只有从 App 发出的任务可以这样修正。");
      }
      if (!["queued", "delivered", "acked", "held"].includes(task.status)) {
        return fail(res, 409, "invalid_state", "这个任务已经结束了。");
      }
      const steer = makeMessage({
        thread_id: task.thread_id, from: CONSOLE, to: task.to, type: "steer", status: "done",
        body: text, reply_to: task.id, created_at: iso(0), updated_at: iso(0),
      });
      // While it waits on a question, whatever you say answers it.
      if (task.asking) {
        steer.answers = task.asking;
        delete task.asking;
        task.asked_once = true;
        askingBy.delete(task.to);
      }
      const steerAtts = (Array.isArray(body.attachments) ? body.attachments : []).map((id) => uploads.get(id)).filter(Boolean);
      if (Array.isArray(body.attachments) && steerAtts.length !== body.attachments.length) return fail(res, 404, "not_found", "附件不存在");
      if (steerAtts.length) steer.attachments = steerAtts;
      msgs.push(steer);
      task.status = "delivered";
      task.updated_at = iso(0);
      // Still the same round: the answer comes after the addition.
      scheduleReply(hub, task.thread_id, task.to, task.id, true);
      return send(res, 201, steer);
    }

    // /app/hubs/{id}/messages/{mid}/dismiss — close an open message unanswered.
    if (parts.length === 6 && parts[3] === "messages" && parts[5] === "dismiss" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      dismissed.add(parts[4]);
      return send(res, 200, makeMessage({ id: parts[4], status: "done" }));
    }

    // /app/hubs/{id}/backups — the workspace's backups (hubs from v0.21.12;
    // the lab hub is older), and the copies kept on this computer.
    if (parts[3] === "backups") {
      if (hub.id === "hub_lab") return fail(res, 404, "not_found", "no route for GET /v1/admin/backups");
      const bk = (hub.backupState ||= { server: [{ name: "myco-backup-hub-20261121-031502.tar.gz", size: 18_400_000, created_at: iso(5 * 3600) }], local: [], auto: false });
      const dir = `/Users/glenn/myco-backups/${hub.name}`;
      if (parts.length === 4 && method === "GET") {
        return send(res, 200, { backups: bk.server, keep: 7, daily: true, restore: "on the server, with the hub stopped: myco hub restore --from <backup> --db /var/lib/myco/hub.db", local: bk.local, dir, auto: bk.auto });
      }
      if (parts.length === 4 && method === "POST") {
        await sleep(600);
        const b = { name: `myco-backup-hub-${Date.now()}.tar.gz`, size: 18_600_000, created_at: iso(0) };
        bk.server.unshift(b);
        bk.server = bk.server.slice(0, 7);
        return send(res, 201, b);
      }
      if (parts.length === 5 && parts[4] === "save" && method === "POST") {
        const body = await readBody(req);
        const b = bk.server.find((x) => x.name === body.name) || bk.server[0];
        if (!bk.local.some((x) => x.name === b.name)) bk.local.unshift({ ...b });
        bk.local = bk.local.slice(0, 3);
        return send(res, 200, { path: `${dir}/${b.name}`, dir });
      }
      if (parts.length === 5 && parts[4] === "auto" && method === "PUT") {
        const body = await readBody(req);
        bk.auto = !!body.on;
        return send(res, 200, { auto: bk.auto });
      }
      if (parts.length === 5 && parts[4] === "reveal" && method === "POST") return send(res, 200, { dir });
    }

    // /app/hubs/{id}/settings — end-to-end encryption (MOCK_E2E=1 starts it on).
    if (parts.length === 4 && parts[3] === "settings") {
      if (method === "PUT") { const body = await readBody(req); hub.e2e = !!body.e2e; }
      return send(res, 200, { e2e: !!hub.e2e });
    }

    // /app/hubs/{id}/recent — threads and shell-node exchanges, newest first.
    if (parts.length === 4 && parts[3] === "recent" && method === "GET") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const sp = url.searchParams;
      const q = (sp.get("q") || "").trim().toLowerCase();
      const label = sp.get("label") || "";
      const mine = sp.get("scope") === "mine";
      const before = Number(sp.get("before") || 0);
      const agentOf = (n) => (hub.agents || []).find((a) => a.name === n);
      const labelOk = (names) => {
        if (!label) return true;
        return names.some((n) => {
          const a = agentOf(n);
          if (!a) return false;
          const groups = (a.labels || []).filter((l) => l.startsWith("group=")).map((l) => l.slice(6));
          const tags = (a.labels || []).filter((l) => !l.includes("="));
          if (label === "group:") return groups.length === 0;
          if (label.startsWith("group:")) return groups.includes(label.slice(6));
          if (label.startsWith("tag:")) return tags.includes(label.slice(4));
          return true;
        });
      };
      const stateOf = (msgs, node) => {
        const tasks = msgs.filter((m) => m.type === "task" && (!node || m.to === node));
        const task = tasks[tasks.length - 1];
        if (!task) return { state: "" };
        const answer = msgs.find((m) => m.reply_to === task.id && (m.type === "reply" || m.type === "event"));
        const open = tasks.find((m) => !["done", "failed"].includes(m.status));
        let state = "done";
        // delivered: pulled but not acknowledged, so still queued.
        if (open) state = open.status === "queued" || open.status === "delivered" ? "queued" : "working";
        else if (task.status === "failed" || answer?.type === "event") state = "failed";
        else if (node && answer) { try { if (JSON.parse(answer.body).exit_code) state = "failed"; } catch { /* not json */ } }
        return { state, task, answer, ...(open ? { open } : {}) };
      };
      const items = [];
      for (const [threadId, msgs] of Object.entries(hub.threads || {})) {
        if (!msgs.length) continue;
        const participants = [];
        for (const m of msgs) for (const n of [m.from, m.to]) if (!participants.includes(n)) participants.push(n);
        const last = msgs[msgs.length - 1];
        items.push({ kind: "thread", thread_id: threadId, participants, updated_at: last.updated_at || last.created_at,
          messages: msgs.length, first: msgs[0], last, ...stateOf(msgs), project: msgs[0].project || (threadId === "th_release" && hub.id === "hub_studio" ? "website" : undefined) });
      }
      const ex = execHistory("release-cloud");
      const newest = ex.reduce((a, m) => (new Date(m.updated_at) > new Date(a.updated_at) ? m : a), ex[0]);
      const sorted = ex.slice().sort((a, b) => new Date(a.created_at) - new Date(b.created_at));
      items.push({ kind: "pair", thread_id: newest.thread_id, other: "release-cloud", node: "mac-exec", participants: ["release-cloud", "mac-exec"],
        updated_at: newest.updated_at, messages: ex.length, first: sorted[0], last: sorted[sorted.length - 1], ...stateOf(sorted, "mac-exec") });
      // group=pair: one row per pair of parties (threads merged).
      if (sp.get("group") === "pair") {
        const byPair = new Map();
        for (const it of items) {
          if (it.kind !== "thread") { byPair.set(`${it.other}|${it.node}`, it); continue; }
          const [x, y] = it.participants;
          const con = [x, y].find((n) => n.startsWith("@console:"));
          const other = con || (x < y ? x : y), node = other === x ? y : x;
          const key = [other, node].sort().join("|");
          const prev = byPair.get(key);
          if (!prev || new Date(it.updated_at) > new Date(prev.updated_at)) {
            // An open task in either thread keeps the pair open (newest first).
            const open = it.open || prev?.open;
            const state = it.open ? it.state : prev?.open ? prev.state : it.state;
            byPair.set(key, { ...it, kind: "pair", other, node, participants: [other, node], messages: (prev?.messages || 0) + it.messages, first: prev?.first || it.first, state, ...(open ? { open } : {}) });
          } else {
            prev.messages += it.messages;
            if (!prev.open && it.open) Object.assign(prev, { open: it.open, state: it.state });
          }
        }
        items.splice(0, items.length, ...byPair.values());
      }
      if (sp.get("group") === "pair" && sp.get("side")) {
        const mineSide = (it) => [it.other, it.node, ...(it.participants || [])].some((n) => (n || "").startsWith("@console:"));
        const keep = items.filter((it) => (sp.get("side") === "me") === mineSide(it));
        items.splice(0, items.length, ...keep);
      }
      const list = items
        .filter((it) => !mine || it.participants.some((p) => p.startsWith("@console:")) || it.kind === "pair")
        .filter((it) => labelOk(it.participants))
        .filter((it) => !q || it.participants.some((p) => p.toLowerCase().includes(q)) ||
          [it.first, it.last, it.task, it.answer].some((m) => (m?.body || "").toLowerCase().includes(q)))
        .filter((it) => !before || new Date(it.updated_at).getTime() < before)
        .filter((it) => !sp.get("project") || it.project === sp.get("project"))
        .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
      return send(res, 200, { items: list.slice(0, 30), more: list.length > 30 });
    }

    // /app/hubs/{id}/agents/{name}/handovers | handover-link | handover —
    // 「换一个来接着做」 (hubs after v0.19.22; the others answer 404). pm-codex
    // was taken over from a claude.ai session two days ago: the line in its
    // chat, and 「换回这个」 in its profile. A command waits for whoever runs it
    // (nobody does here); a computer path answers after a moment, with the
    // new session's first words. The agent itself is left as it is.
    if (parts.length === 6 && parts[3] === "agents" && ["handovers", "handover-link", "handover"].includes(parts[5])) {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      if (hub.id !== "hub_studio") return fail(res, 404, "not_found", "no such route");
      const name = parts[4];
      const agent = hub.agents.find((a) => a.name === name);
      if (!agent) return fail(res, 404, "not_found", `agent ${name} not found`);
      state.handovers ||= {};
      const list = (state.handovers[name] ||= seedHandovers(name));
      if (parts[5] === "handovers" && method === "GET") return send(res, 200, { handovers: list });
      if (method !== "POST") return fail(res, 405, "method_not_allowed", "method not allowed");
      const body = (await readBody(req)) || {};
      const machineName = (id) => (hub.machines || []).find((m) => m.id === id)?.name;
      const from = { binding: { ...agent.binding }, machine_id: agent.machine_id, machine_name: machineName(agent.machine_id), runtime: { cli: agent.runtime?.cli || agent.binding?.harness } };
      const id = "hov_" + Math.random().toString(36).slice(2, 10);
      if (parts[5] === "handover-link") {
        const h = { id, agent: name, at: iso(), by: "glenn", note: body.note || "", from, to: { binding: {} }, state: "waiting", open_tasks: [], back: body.back || "", expires_at: iso(-86_400) };
        list.unshift(h);
        const url = `${hub.url.replace(/^http:/, "https:")}/h/myco_takeover_9f3k2demo${id.slice(4)}`;
        return send(res, 201, { handover: h, url, command: `curl -fsSL '${url}' | sh`, command_ps1: `irm '${url}?format=ps1' | iex` });
      }
      const m = (hub.machines || []).find((x) => x.id === body.machine || x.name === body.machine);
      if (!m) return fail(res, 404, "not_found", `machine ${body.machine} not found`, "list them: myco machine list");
      if (!/^0\.(19\.(2[3-9]|[3-9]\d)|[2-9]\d)|^[1-9]/.test(m.myco_version || "")) {
        return fail(res, 409, "invalid_state", `myco on machine ${m.name} (v${m.myco_version}) cannot take a node over`, `upgrade it there: myco upgrade`);
      }
      await sleep(1600);
      const cli = body.cli || agent.runtime?.cli || "claude";
      const h = {
        id, agent: name, at: iso(2), by: "glenn", note: body.note || "", from,
        to: { binding: { harness: cli, session_id: "new-" + id.slice(4), workdir: body.dir || agent.binding?.workdir }, machine_id: m.id, machine_name: m.name, runtime: { cli } },
        state: "done", done_at: iso(), open_tasks: [], back: body.back || "", expires_at: iso(-86_400),
      };
      list.unshift(h);
      return send(res, 200, { handover: h, spawn: { agent: name, session_id: h.to.binding.session_id, cli, workdir: h.to.binding.workdir, access: "workspace",
        reply: "读完交接说明了：我现在是 PM (Codex)，接着审查 multihub 分支（还剩 2 个文件），做完先在这里汇报。" } });
    }

    // /app/hubs/{id}/agents/{name}/peers — other nodes it talked to (demo: an exec node).
    if (parts.length === 6 && parts[3] === "agents" && parts[5] === "peers" && method === "GET") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const ex = execHistory(decodeURIComponent(parts[4]));
      return send(res, 200, { peers: [{ agent: "mac-exec", messages: ex.length, last: ex[ex.length - 1] }] });
    }
    // /app/hubs/{id}/exchanges?a=&b=&before= — newest first, 4 per page (to show paging).
    if (parts.length === 4 && parts[3] === "exchanges" && method === "GET") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const q = new URL(req.url, "http://x").searchParams;
      const a = q.get("a") === "mac-exec" ? q.get("b") : q.get("a");
      const before = Number(q.get("before") || 0);
      const since = process.env.MOCK_OLD_HUB ? 0 : Number(q.get("updated_since") || 0);
      if (since) {
        const pair = [q.get("a"), q.get("b")];
        const canned = pair.includes("mac-exec") ? execHistory(pair.find((n) => n !== "mac-exec")) : [];
        const changed = [...canned, ...Object.values(hub.threads || {}).flat()]
          .filter((m) => pair.includes(m.from) && pair.includes(m.to) && new Date(m.updated_at).getTime() > since)
          .sort((x, y) => new Date(x.updated_at) - new Date(y.updated_at));
        return send(res, 200, { messages: changed, more: false, since });
      }
      if (q.get("a") !== "mac-exec" && q.get("b") !== "mac-exec") {
        // Two parties: every message between them, all threads, newest first.
        const pair = [q.get("a"), q.get("b")];
        const all = Object.values(hub.threads || {}).flat()
          .filter((m) => pair.includes(m.from) && pair.includes(m.to))
          .filter((m) => !before || new Date(m.created_at).getTime() < before)
          .sort((x, y) => new Date(y.created_at) - new Date(x.created_at));
        return send(res, 200, { messages: all.slice(0, 50), more: all.length > 50 });
      }
      // The canned history plus what was typed in this session.
      const typed = Object.values(hub.threads || {}).flat()
        .filter((m) => (m.from === a && m.to === "mac-exec") || (m.from === "mac-exec" && m.to === a));
      const all = [...execHistory(a), ...typed]
        .sort((x, y) => new Date(y.created_at) - new Date(x.created_at))
        .filter((m) => !before || new Date(m.created_at).getTime() < before);
      return send(res, 200, { messages: all.slice(0, 4), more: all.length > 4 });
    }
    // /app/hubs/{id}/projects…
    if (parts[3] === "projects") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const list = hub.id === "hub_studio" ? projects : (hub.projects ||= []);
      if (parts.length === 4 && method === "GET") return send(res, 200, { projects: list.map((p) => projectView(hub, p)) });
      if (parts.length === 4 && method === "POST") {
        const b = await readBody(req);
        if (list.some((p) => p.name === b.name)) return fail(res, 409, "conflict", `project ${b.name} already exists`);
        const p = { name: b.name, title: b.title || b.name, goal: b.goal || "", brief: "", links: [], visibility: b.visibility || "members",
          owner: meName, owner_id: "mem_" + meName, archived: false, created_at: iso(), updated_at: iso(), members: [], docs: [] };
        list.push(p);
        return send(res, 201, projectView(hub, p));
      }
      const p = list.find((x) => x.name === parts[4]);
      if (!p) return fail(res, 404, "not_found", `no project ${parts[4]}`);
      if (parts.length === 5 && method === "GET") return send(res, 200, projectView(hub, p, url.searchParams.get("docs") === "body"));
      if (parts.length === 5 && method === "PATCH") {
        const b = await readBody(req);
        for (const k of ["title", "goal", "brief", "links", "visibility", "archived"]) if (b[k] !== undefined) p[k] = b[k];
        p.updated_at = iso();
        return send(res, 200, projectView(hub, p));
      }
      if (parts.length === 5 && method === "DELETE") { list.splice(list.indexOf(p), 1); return send(res, 204); }
      if (parts.length === 7 && parts[5] === "members") {
        if (method === "PUT") {
          const b = await readBody(req);
          let m = p.members.find((x) => x.name === parts[6]);
          if (!m) { m = { name: parts[6], role: "member", note: "", added_by: meName, added_at: iso() }; p.members.push(m); }
          if (b.role) m.role = b.role;
          if (b.note !== undefined && b.note !== null) m.note = b.note;
          return send(res, 200, projectView(hub, p));
        }
        if (method === "DELETE") { p.members = p.members.filter((x) => x.name !== parts[6]); return send(res, 200, projectView(hub, p)); }
      }
      if (parts.length === 7 && parts[5] === "machines" && (method === "PUT" || method === "DELETE")) {
        if (hub.id === "hub_lab") return fail(res, 404, "not_found", "no such route");
        const id = parts[6];
        p.machines = (p.machines || []).filter((x) => x !== id);
        if (method === "PUT") p.machines.push(id);
        p.updated_at = iso();
        return send(res, 200, projectView(hub, p));
      }
      if (parts[5] === "files" && parts.length >= 7) {
        const i = p.docs.findIndex((d) => d.name === parts[6]);
        if (parts.length === 8 && parts[7] === "save" && method === "POST") {
          return i < 0 ? fail(res, 404, "not_found", "no such file") : send(res, 200, { path: `/Users/glenn/Downloads/${parts[6]}` });
        }
        if (method === "PUT") {
          const chunks = [];
          for await (const c of req) chunks.push(c);
          const buf = Buffer.concat(chunks);
          const ct = req.headers["content-type"] || "application/octet-stream";
          const text = /^text\/|json|markdown/.test(ct) || /\.(md|txt|json)$/i.test(parts[6]);
          const d = { name: parts[6], body: text ? buf.toString("utf8") : "", content_type: ct, binary: !text, bytes: buf, size: buf.length, updated_by: meName, updated_at: iso() };
          if (i < 0) p.docs.unshift(d); else p.docs[i] = d;
          return send(res, 200, { name: d.name, size: d.size, content_type: ct, binary: d.binary, updated_by: d.updated_by, updated_at: d.updated_at });
        }
        if (method === "GET") {
          if (i < 0) return fail(res, 404, "not_found", "no such file");
          const d = p.docs[i];
          res.writeHead(200, { "Content-Type": d.content_type || "text/markdown" });
          return res.end(d.bytes || d.body);
        }
      }
      if (parts.length === 6 && parts[5] === "memory" && method === "POST") {
        if (hub.id === "hub_lab") return fail(res, 404, "not_found", "no such route");
        const b = await readBody(req);
        const text = String(b.text || "").split(/\s+/).filter(Boolean).join(" ");
        if (!text) return fail(res, 400, "invalid_argument", "the memory entry is empty");
        let d = p.docs.find((x) => x.name === "memory");
        if (!d) { d = { name: "memory", body: "# Project memory\n" }; p.docs.push(d); }
        d.body = d.body.replace(/\n*$/, "\n") + `- ${text} (${meName}, ${iso().slice(0, 10)})\n`;
        d.updated_by = meName; d.updated_at = iso();
        return send(res, 200, { ...memoryOf(p) });
      }
      if (parts.length === 7 && parts[5] === "docs") {
        const i = p.docs.findIndex((d) => d.name === parts[6]);
        if (method === "GET") return i < 0 ? fail(res, 404, "not_found", "no such document") : send(res, 200, { name: p.docs[i].name, body: p.docs[i].body, content_type: p.docs[i].content_type, binary: !!p.docs[i].binary, updated_by: p.docs[i].updated_by, updated_at: p.docs[i].updated_at, size: p.docs[i].size ?? p.docs[i].body.length });
        if (method === "PUT") {
          const b = await readBody(req);
          const d = { name: parts[6], body: b.body || "", updated_by: meName, updated_at: iso() };
          if (i < 0) p.docs.push(d); else p.docs[i] = d;
          return send(res, 200, { ...d, size: d.body.length });
        }
        if (method === "DELETE") { if (i >= 0) p.docs.splice(i, 1); return send(res, 204); }
      }
      return fail(res, 404, "not_found", "no such route");
    }
    // /app/hubs/{id}/messages/{mid}/trace — a made-up run: all steps for a
    // finished task, revealed one every two seconds for a running one.
    if (parts.length === 6 && parts[3] === "messages" && parts[5] === "trace" && method === "GET") {
      const all = Object.values(hub.threads || {}).flat();
      const m = all.find((x) => x.id === parts[4]);
      // A message the reset between passes took away, still in the app's
      // saved page data: no steps (a 404 would be a page error in the smoke).
      if (!m) return send(res, 200, { events: [] });
      const node = (hub.agents || []).find((a) => a.name === m.to);
      if (node?.binding?.harness === "exec") {
        const lines = execLines();
        const end = m.status === "failed" ? new Date(m.updated_at).getTime() : Date.now();
        const n = m.status === "done" ? lines.length : Math.min(lines.length, Math.floor((end - new Date(m.created_at).getTime()) / EXEC_LINE_MS));
        const after = Number(url.searchParams.get("after") || 0);
        const events = lines.slice(0, n).map((l, i) => {
          const d = { kind: "output", stream: l.startsWith("---") ? "stderr" : "stdout", text: l + "\n" };
          return { seq: i + 1, kind: "output", data: d };
        }).filter((e) => e.seq > after);
        return send(res, 200, { message: m.id, status: m.status, events });
      }
      const steps = m.trace ? [...m.trace] : [
        { kind: "thinking", text: "先看看 git log 和 CHANGELOG，确认这次改了哪些东西，再按用户能感知的变化分组。" },
        { kind: "tool", tool: "Bash", id: "c1", input: "git log --oneline v0.16.7..HEAD" },
        { kind: "tool_result", id: "c1", output: "a1b2c3d App: 对话页支持 Markdown 表格\n9f8e7d6 Hub: 修复 runner 重连\n5c4b3a2 App: 本机会话一键接入" },
        { kind: "tool", tool: "Read", id: "c2", input: "CHANGELOG.md" },
        { kind: "tool_result", id: "c2", output: "## Unreleased\n- ..." },
        { kind: "text", text: "改动不多，主要是 App 的两项新功能和一个 Hub 修复。我按这个写发布说明。" },
        { kind: "tool", tool: "Edit", id: "c3", input: "CHANGELOG.md" },
        { kind: "tool_result", id: "c3", output: "The file CHANGELOG.md has been updated." },
        { kind: "tool", tool: "Bash", id: "c4", input: "make check" },
        { kind: "tool_result", id: "c4", output: "FAIL internal/web: dist is stale\nmake: *** [check] Error 1", failed: true },
        { kind: "tool", tool: "Bash", id: "c5", input: "make web && make check" },
        { kind: "tool_result", id: "c5", output: "smoke: all steps passed" },
        { kind: "usage", input_tokens: 18400, output_tokens: 2100, cost_usd: 0.08, duration_ms: 84000 },
      ];
      // "Just say 收到": a run with no thinking or tools.
      if (/收到/.test(m.body || "")) steps.splice(0, steps.length, { kind: "text", text: "收到" }, { kind: "usage", input_tokens: 900, output_tokens: 3, duration_ms: 2100 });
      const running = ["queued", "delivered", "acked", "held"].includes(m.status);
      // Steps start once it is picked up: a runner acknowledges first.
      const started = m.status !== "queued" && m.status !== "delivered";
      const shown = !started ? [] : running ? steps.slice(0, Math.min(steps.length - 1, Math.floor((Date.now() - new Date(m.created_at).getTime()) / (m.trace_pace || 2000)))) : steps;
      const after = Number(url.searchParams.get("after") || 0);
      const events = shown.map((d, i) => ({ seq: i + 1, kind: d.kind, data: d, ...(d.at ? { at: d.at } : {}) })).filter((e) => e.seq > after);
      return send(res, 200, { message: m.id, status: m.status, events });
    }
    // /app/hubs/{id}/agents/cleanup — offline nodes (demo: every offline one).
    if (parts.length === 5 && parts[3] === "agents" && parts[4] === "cleanup" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const body = await readBody(req);
      const stale = hub.agents.filter((a) => !a.online && a.kind !== "machine" && a.binding?.harness);
      if (!body.dry_run) hub.agents = hub.agents.filter((a) => !stale.includes(a));
      return send(res, 200, { agents: stale, removed: !body.dry_run, dry_run: !!body.dry_run });
    }

    if (parts.length === 5 && parts[3] === "threads" && method === "GET") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const since = Number(url.searchParams.get("updated_since") || 0);
      if (since && !process.env.MOCK_OLD_HUB) {
        // Only what changed (a hub from v0.18.35).
        const changed = (hub.threads[parts[4]] || []).filter((m) => new Date(m.updated_at).getTime() > since);
        return send(res, 200, { messages: changed, since });
      }
      return send(res, 200, { messages: hub.threads[parts[4]] || [] });
    }

    // /app/hubs/{id}/pins — what a conversation keeps at hand (置顶), the
    // same list for both its parties (hubs from v0.20.2; MOCK_OLD_HUB: none).
    if (parts[3] === "pins" && (parts.length === 4 || parts.length === 5)) {
      if (process.env.MOCK_OLD_HUB) return fail(res, 404, "", "404 page not found");
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const body = method === "GET" || method === "DELETE" ? null : await readBody(req);
      const a = (body?.a || url.searchParams.get("a") || CONSOLE), w = body?.with || url.searchParams.get("with") || "";
      if (!w) return fail(res, 400, "invalid_argument", "with whom: the conversation's other party is required");
      const key = [a, w].sort().join("\u0000");
      const all = (hub.pins ||= {});
      const list = (all[key] ||= []);
      const out = () => send(res, 200, { a, with: w, pins: list, max: 20 });
      if (parts.length === 4 && method === "GET") return out();
      if (parts.length === 4 && method === "POST") {
        const same = list.find((p) => p.kind === body.kind && (p.target || "") === (body.target || "") && (p.file?.file_id || "") === (body.file?.file_id || ""));
        if (same) { if (body.title) same.title = body.title; return send(res, 201, same); }
        if (list.length >= 20) return fail(res, 409, "conflict", "a conversation keeps 20 pins at most");
        if (body.kind === "link" && !/^https?:\/\/\S+$/.test(body.target || "")) return fail(res, 400, "invalid_argument", "a link is an http or https address");
        const title = body.title || (body.kind === "link" ? body.target.replace(/^https?:\/\//, "").replace(/\/$/, "") : body.kind === "file" ? body.file?.name : body.target);
        const p = { id: `pin_${state.counter += 1}`, kind: body.kind, title, ...(body.target ? { target: body.target } : {}), ...(body.file ? { file: body.file } : {}), added_by: body.by || a, at: iso(0) };
        list.push(p);
        return send(res, 201, p);
      }
      if (parts.length === 5 && parts[4] === "order" && method === "PUT") {
        const byId = new Map(list.map((p) => [p.id, p]));
        const first = (body.ids || []).map((id) => byId.get(id)).filter(Boolean);
        all[key] = [...first, ...list.filter((p) => !first.includes(p))];
        return send(res, 200, { a, with: w, pins: all[key], max: 20 });
      }
      const i = list.findIndex((p) => p.id === parts[4]);
      if (parts.length === 5 && i < 0) return fail(res, 404, "not_found", `no pin ${parts[4]} in this conversation`);
      if (parts.length === 5 && method === "PATCH") { list[i].title = body.title; return send(res, 200, list[i]); }
      if (parts.length === 5 && method === "DELETE") { list.splice(i, 1); res.writeHead(204); return res.end(); }
    }

    // /app/hubs/{id}/hidden — messages taken out of your own view.
    if (parts.length === 4 && parts[3] === "hidden") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      hub.hidden = hub.hidden || [];
      if (method === "GET") return send(res, 200, { ids: hub.hidden });
      if (method === "POST") {
        const body = await readBody(req);
        const show = new Set(body?.show || []);
        hub.hidden = [...new Set([...hub.hidden.filter((id) => !show.has(id)), ...(body?.hide || [])])];
        return send(res, 200, { ids: hub.hidden });
      }
    }

    // Workspaces of the hub's server (v0.19.9 hubs serve several at
    // /w/<name>). The studio hub's server: this App owns it.
    const origin = (u) => { try { return new URL(u).origin; } catch { return u; } };
    if (parts.length === 4 && parts[3] === "workspaces" && method === "GET") {
      const root = state.hubs.find((h) => !h.url.includes("/w/") && origin(h.url) === origin(hub.url));
      const owns = root?.id === "hub_studio";
      return send(res, 200, { supported: owns, allowed: owns, server: origin(hub.url), workspaces: (root?.workspaces || []) });
    }
    if (parts.length === 4 && parts[3] === "workspaces" && method === "POST") {
      const body = await readBody(req);
      const name = String(body?.name || "").trim();
      if (!/^[a-z0-9][a-z0-9-]{0,39}$/.test(name)) return fail(res, 400, "invalid_argument", `invalid workspace name "${name}"`);
      const root = state.hubs.find((h) => !h.url.includes("/w/") && origin(h.url) === origin(hub.url));
      root.workspaces = root.workspaces || [];
      if (root.workspaces.some((w) => w.name === name)) return fail(res, 409, "conflict", `workspace "${name}" already exists`);
      const url = `${origin(hub.url)}/w/${name}`;
      root.workspaces.push({ name, url, created_at: iso(0) });
      const h = { id: `hub_ws_${name}`, name: String(body?.display_name || "").trim() || name, url, token: `myco_console_ws_${name}`,
        ok: true, hub_version: hub.hub_version, update_available: false, agents: [], threads: {}, recent: [], machines: [] };
      state.hubs.push(h);
      return send(res, 201, { id: h.id, name: h.name, url: h.url, ok: true });
    }
    // Computers of the server's other workspaces, not in this one yet.
    if (parts.length === 4 && parts[3] === "server-machines" && method === "GET") {
      const mine = new Set((hub.machines || []).map((m) => m.name));
      const out = [];
      for (const h of state.hubs) {
        if (h.id === hub.id || origin(h.url) !== origin(hub.url)) continue;
        for (const m of h.machines || []) {
          if (mine.has(m.name) || m.legacy || out.some((x) => x.name === m.name)) continue;
          out.push({ hub_id: h.id, hub_name: h.name, machine_id: m.id, name: m.name, hostname: m.hostname, os: m.os, online: !!m.online, can_spawn: !!m.can_spawn });
        }
      }
      return send(res, 200, { machines: out });
    }
    if (parts.length === 5 && parts[3] === "machines" && parts[4] === "add-from" && method === "POST") {
      const body = await readBody(req);
      const from = state.hubs.find((h) => h.id === body?.from_hub);
      const m = (from?.machines || []).find((x) => x.id === body?.machine_id);
      if (!m) return fail(res, 404, "not_found", "没有这台电脑");
      hub.machines = hub.machines || [];
      hub.machines.push({ ...m, id: `${m.id}_${hub.id}`, agents: [] });
      return send(res, 200, { joined: hub.url, machine_node: `${m.name}-machine` });
    }

    // /app/hubs/{id}/links — a one-off enrollment link for adding a machine.
    if (parts.length === 4 && parts[3] === "links" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const body = await readBody(req);
      const uses = Number.isInteger(body?.uses) ? body.uses : 1;
      const hours = Number.isInteger(body?.expires_hours) ? body.expires_hours : 24;
      state.counter += 1;
      const joinUrl = `${hub.url.replace(/\/+$/, "")}/j/myco_enroll_demo${state.counter}x7k2m9q4w8`;
      // A shell-node link: the UI smoke "runs" it with /app/mock/run-join.
      // A superagent link: a cloud session "joins" a moment later and is
      // your superagent (as the hub does from v0.21.5).
      if (body?.superagent) {
        setTimeout(() => {
          state.counter += 1;
          const name = `cloud-shroom-${state.counter}`;
          hub.agents.push(makeAgent({ name, display_name: "Shroom", owner: "glenn", owner_id: "mem_glenn", kind: "agent",
            binding: { harness: "claude-cloud", environment: "claude.ai" }, created_at: iso(0), last_seen_at: iso(0), status_text: "刚加入" }));
          state.superagents ||= {};
          state.superagents[hub.id] = name;
        }, 2500);
      }
      if (body?.shell_node) state.pendingJoin = { hubId: hub.id, allow: Array.isArray(body.allow_from) ? body.allow_from : [] };
      else if (uses === 1) state.pendingConnect = { hubId: hub.id };
      return send(res, 200, {
        join_url: joinUrl,
        command: body?.allow_shell && !body?.shell_node ? `curl -fsSL '${joinUrl}?shell=1' | bash` : `curl -fsSL ${joinUrl} | bash`,
        ps_command: `irm '${joinUrl}?format=ps1' | iex`,
        shell_node: !!body?.shell_node,
        agent_guide: true,
        uses,
        expires_hours: hours,
      });
    }

    // /app/hubs/{id}/machines/{mid}/reach|diagnose|fix — a computer's
    // check-up over SSH (ComputerCheck.tsx). VPS 东京 runs its runner through
    // a proxy that no longer gets through (as a WSL did once).
    // /app/hubs/{id}/machines/{mid}/upgrade — the computer upgrades its myco.
    if (parts.length === 6 && parts[3] === "machines" && parts[5] === "upgrade" && method === "POST") {
      const m = (hub.machines || []).find((x) => x.id === parts[4]);
      if (!m) return fail(res, 404, "not_found", "没有这台电脑");
      await sleep(900);
      const from = "v" + String(m.myco_version || "").replace(/^v/, "");
      m.myco_version = "0.17.2";
      return send(res, 200, { from, to: "v0.17.2", restarting: true });
    }
    // /app/hubs/{id}/upgrade — the workspace upgrades itself and restarts.
    if (parts.length === 4 && parts[3] === "upgrade" && method === "POST") {
      await sleep(600);
      const from = "v" + String(hub.hub_version || "").replace(/^v/, "");
      hub.hub_version = "0.17.2";
      return send(res, 202, { from, to: "v0.17.2", via: "service" });
    }
    if (parts.length === 6 && parts[3] === "machines" && ["reach", "diagnose", "fix"].includes(parts[5])) {
      const mid = parts[4];
      const key = `${hub.id}/${mid}`;
      if (parts[5] === "reach" && method === "GET") return send(res, 200, { route: machineReach.get(key) || null });
      if (parts[5] === "reach" && method === "PUT") {
        const body = await readBody(req);
        machineReach.set(key, { via: body.via, target: body.target });
        return send(res, 200, { route: machineReach.get(key) });
      }
      const rt = machineReach.get(key);
      if (!rt) return fail(res, 409, "not_configured", "this App does not know how to reach that computer");
      const machine = (hub.machines || []).find((m) => m.id === mid);
      const st = machineHealth.get(key) || { proxyBroken: mid === "m_vps", service: "active", version: (machine && machine.myco_version) || "0.20.14" };
      machineHealth.set(key, st);
      if (parts[5] === "diagnose" && method === "POST") {
        await sleep(900);
        if (rt.target === "old-pi") return fail(res, 502, "auth", "pi@192.168.1.40: Permission denied (publickey).", "ssh old-pi");
        return send(res, 200, {
          route: rt,
          found: { os: "Linux", myco: `v${st.version}`, myco_path: "/home/glenn/.local/bin/myco", service: st.service, since: "Wed 2026-11-18 02:17:35 CST", linger: "yes", running: "yes", proxy: "http://127.0.0.1:7890" },
          hubs: [{ url: "https://hub.example.com", mine: true, direct: "200", proxy: st.proxyBroken ? "000" : "200" }],
          log: [
            "time=2026-11-19T08:59:41.521+08:00 level=WARN msg=\"pull failed\" agent=vps-claude err=\"unavailable: cannot reach hub https://hub.example.com: Post \\\"https://hub.example.com/v1/inbox/pull\\\": EOF\" retry_in=30s",
            "time=2026-11-19T09:00:14.476+08:00 level=WARN msg=\"pull failed\" agent=vps-claude err=\"unavailable: cannot reach hub https://hub.example.com: Post \\\"https://hub.example.com/v1/inbox/pull\\\": EOF\" retry_in=30s",
          ],
        });
      }
      if (parts[5] === "fix" && method === "POST") {
        const body = await readBody(req);
        await sleep(body.action === "upgrade" ? 1500 : 700);
        if (body.action === "upgrade") { st.version = "0.20.17"; if (machine) machine.myco_version = "0.20.17"; }
        st.service = "active";
        return send(res, 200, { ok: true, log: body.action === "upgrade"
          ? ["==> downloading https://github.com/chenhg5/myco/releases/latest/download/myco-linux-amd64", "==> checksum ok", "==> installed: /home/glenn/.local/bin/myco v0.20.17", "runner service restarted (systemd)"]
          : ["runner service restarted (systemd)"] });
      }
    }

    // /app/hubs/{id}/machines/connect — connect a computer over SSH or WSL:
    // the job moves on with time; old-pi has a key the App cannot use.
    if (parts.length === 5 && parts[3] === "machines" && parts[4] === "connect" && method === "POST") {
      const body = await readBody(req);
      const target = String(body.target || "").trim();
      if (!["ssh", "wsl"].includes(body.via) || !target || /^-|[\s'"`;|&$<>]/.test(target)) {
        return fail(res, 400, "invalid_argument", "say how to reach it: via ssh or wsl, and a plain name");
      }
      state.counter += 1;
      const job = { id: `cj_${state.counter}`, hub: hub.id, route: { via: body.via, target, ...(body.port ? { port: body.port } : {}) }, started: Date.now() };
      connectJobs.set(job.id, job);
      return send(res, 200, connectView(job));
    }

    // /app/hubs/{id}/machines/{mid}
    if (parts.length === 5 && parts[3] === "machines") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const machine = (hub.machines || []).find((m) => m.id === parts[4]);
      if (!machine) return fail(res, 404, "not_found", "没有这台机器");
      if (method === "PATCH") {
        const body = await readBody(req);
        const name = typeof body?.name === "string" ? body.name.trim() : "";
        if (!name) return fail(res, 400, "invalid_request", "name 不能为空。");
        if ((hub.machines || []).some((m) => m.id !== machine.id && m.name.toLowerCase() === name.toLowerCase())) {
          return fail(res, 409, "conflict", `another computer in this workspace is already called "${name}"`, "pick another name");
        }
        machine.name = name;
        const agents = hub.agents.filter((a) => a.kind !== "machine" && a.machine_id === machine.id).length;
        return send(res, 200, { ...machine, agents });
      }
      if (method === "DELETE") {
        const dry = url.searchParams.get("dry_run") === "true" || url.searchParams.get("dry_run") === "1";
        // Like the hub: every node on it, the machine's own node included
        // (the App leaves that one out of the list it shows).
        const removed = hub.agents
          .filter((a) => a.machine_id === machine.id)
          .map((a) => a.name);
        if (!dry) {
          await sleep(400);
          hub.agents = hub.agents.filter((a) => a.machine_id !== machine.id);
          hub.machines = (hub.machines || []).filter((m) => m.id !== machine.id);
        }
        return send(res, 200, { machine, removed, dry_run: dry });
      }
    }

    // /app/hubs/{id}/machines/{mid}/info — what a computer allows and has.
    if (parts.length === 6 && parts[3] === "machines" && parts[5] === "info" && method === "GET") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const machine = (hub.machines || []).find((m) => m.id === parts[4]);
      if (!machine) return fail(res, 404, "not_found", "没有这台机器");
      if (!machine.online) return fail(res, 409, "invalid_state", "the machine did not answer in time");
      if (!machine.can_spawn || versionBelow(machine.myco_version, "0.20.14")) {
        return fail(res, 409, "too_old", "that computer's myco is too old to say what it has", "run myco upgrade on it");
      }
      await sleep(600);
      const info = machineInfoOf(machine);
      return send(res, 200, machineDirs.has(machine.id) ? { ...info, dirs: machineDirs.get(machine.id) } : info);
    }
    // /app/hubs/{id}/machines/{mid}/dirs — the folders agents may start in:
    // changed here (this computer) or over the SSH / WSL route the App knows.
    if (parts.length === 6 && parts[3] === "machines" && parts[5] === "dirs" && method === "POST") {
      const body = await readBody(req);
      const machine = (hub.machines || []).find((m) => m.id === parts[4]);
      if (!machine) return fail(res, 404, "not_found", "没有这台机器");
      const rt = machineReach.get(`${hub.id}/${machine.id}`);
      const q = (d) => `'${d}'`;
      if (!body.here && !rt) {
        return fail(res, 409, "not_reachable", "only that computer can change its folders, and this App does not reach it",
          ["myco machine dirs", ...(body.add || []).map((d) => `--add ${q(d)}`), ...(body.remove || []).map((d) => `--remove ${q(d)}`)].join(" "));
      }
      await sleep(500);
      let dirs = [...(machineDirs.get(machine.id) || machineInfoOf(machine).dirs)];
      const covered = [];
      const under = (d, r) => r === "~" || d === r || d.startsWith(r.replace(/\/$/, "") + "/");
      for (const d of body.add || []) {
        if (dirs.some((r) => under(d, r))) { covered.push(d); continue; }
        dirs = [...dirs.filter((r) => !under(r, d)), d];
      }
      for (const d of body.remove || []) {
        if (dirs.length === 1 && dirs[0] === d) return fail(res, 409, "invalid_state", "cannot remove the last allowed folder: with none, all of the home folder is allowed", "allow another folder first, then remove this one");
        dirs = dirs.filter((r) => r !== d);
      }
      machineDirs.set(machine.id, dirs);
      return send(res, 200, { dirs, changed: true, ...(covered.length ? { already_allowed: covered } : {}), via: body.here ? "here" : rt.via });
    }

    // /app/hubs/{id}/machines/{mid}/spawn — start a session on that machine.
    if (parts.length === 6 && parts[3] === "machines" && parts[5] === "spawn" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const machine = (hub.machines || []).find((m) => m.id === parts[4]);
      if (!machine) return fail(res, 404, "not_found", "没有这台机器");
      if (!machine.can_spawn) {
        return fail(res, 403, "machine_cannot_spawn", "这台机器不能开新会话。",
          "它的 myco 版本太旧，或者 runner 没在运行。");
      }
      const body = await readBody(req);
      if (body && body.cli === "exec") {
        const dir = typeof body.dir === "string" ? body.dir.trim() : "";
        if (!Array.isArray(body.allow_from) || body.allow_from.length === 0) return fail(res, 400, "invalid_argument", "a shell node needs at least one agent allowed to send it commands");
        if (!dir) return fail(res, 400, "invalid_request", "dir 为必填项。");
        if (!machine.id.startsWith("m_studio")) { // only Studio Mac (in any workspace) allows full access in the mock
          return fail(res, 403, "permission_denied", `a shell node runs commands with full rights, but machine ${machine.name} allows at most workspace; the machine's owner can allow it with spawn.max_access: full in ~/.myco/machine.yaml`);
        }
        await sleep(1200);
        state.counter += 1;
        const name = (typeof body.name === "string" && body.name.trim()) || `${dir.split("/").filter(Boolean).pop() || "app"}-shell`;
        const agent = makeAgent({
          name, description: typeof body.description === "string" ? body.description : "",
          kind: "shell", machine_id: machine.id, created_at: iso(0), last_seen_at: iso(0),
          labels: [`machine=${machine.name.toLowerCase().replace(/\s+/g, "-")}`],
          binding: { harness: "exec", environment: "macOS", workdir: dir }, allow_from: body.allow_from,
        });
        hub.agents.push(agent);
        return send(res, 200, { agent: agent.name, session_id: "", cli: "exec", workdir: dir, access: "full" });
      }
      if (!body || !(body.cli in CLI_MODELS)) {
        return fail(res, 400, "invalid_request", "cli 必须是 claude、codex、pi、kiro、cursor、gemini 或 opencode。");
      }
      const dir = typeof body.dir === "string" ? body.dir.trim() : "";
      if (!dir) return fail(res, 400, "invalid_request", "dir 为必填项。");
      if (dir === "/etc" || dir.startsWith("/etc/")) {
        return fail(res, 403, "dir_not_allowed", "目录不在允许范围内。");
      }
      // This computer's Claude account sign-in has expired until a way is
      // picked (ClaudeAuthFix): the terminal uses an API set by Magpie.
      if (machine.id === "m_studio" && body.cli === "claude" && !claudeAuth.using) {
        await sleep(600);
        return fail(res, 502, "spawn_failed", "starting the claude session failed: claude reported an error: Failed to authenticate: OAuth session expired and could not be refreshed");
      }
      if (machine.id === "m_vps" && !/^(~|\/home\/glenn)\/code\/myco-team(\/|$)/.test(dir)) {
        return fail(res, 403, "permission_denied", `directory ${dir} is outside the folders this computer allows (~/code/myco-team); its owner can change spawn.dirs in ~/.myco/machine.yaml there`);
      }
      const access = body.access === undefined ? "workspace" : body.access;
      if (!["readonly", "workspace", "full"].includes(access)) {
        return fail(res, 400, "invalid_request", "access 必须是 readonly、workspace 或 full。");
      }

      await sleep(3000);

      state.counter += 1;
      const wanted = typeof body.name === "string" ? body.name.trim() : "";
      const stem = wanted || `${body.cli}-${state.counter}`;
      let name = stem;
      let n = 2;
      while (hub.agents.some((a) => a.name === name)) { name = `${stem}-${n}`; n += 1; }

      const model = typeof body.model === "string" ? body.model.trim() : "";
      const osLabel = { darwin: "macOS", linux: "Linux", windows: "Windows", cloud: "云端" }[machine.os] || machine.os;
      const agent = makeAgent({ ...(typeof body.display_name === "string" && body.display_name ? { display_name: body.display_name } : {}),
        name, owner: "glenn", owner_id: "mem_glenn",
        description: typeof body.description === "string" ? body.description : "",
        kind: "agent",
        machine_id: machine.id,
        runtime: {
          cli: body.cli,
          cli_version: CLI_VERSIONS[body.cli],
          model: model || CLI_MODELS[body.cli],
          model_source: model ? "configured" : "default",
          access,
        },
        status_text: "刚启动，等待任务",
        status_at: iso(0),
        created_at: iso(0),
        last_seen_at: iso(0),
        labels: [`machine=${machine.name.toLowerCase().replace(/\s+/g, "-")}`],
        binding: { harness: body.cli, environment: osLabel, workdir: dir },
      });
      hub.agents.push(agent);

      return send(res, 200, {
        agent: agent.name,
        session_id: agent.binding.session_id,
        cli: body.cli,
        workdir: dir,
        access,
        reply: `环境已就绪。我在 ${dir} 里，随时可以开始工作。`,
      });
    }

    if (parts.length === 4 && parts[3] === "members" && method === "GET") return send(res, 200, { members: people });
    if (parts.length === 4 && parts[3] === "devices" && method === "GET") return send(res, 200, { devices });
    if (parts.length === 5 && parts[3] === "devices" && method === "DELETE") {
      const i = devices.findIndex((d) => d.id === parts[4]);
      if (i >= 0) devices.splice(i, 1);
      return send(res, 200, { ok: true });
    }
    if (parts.length === 4 && parts[3] === "pairings" && method === "POST") {
      return send(res, 201, { link: "https://hub.example.com/c/myco_pair_" + Math.random().toString(36).slice(2, 18), expires_at: new Date(Date.now() + 86400e3).toISOString() });
    }
    if (parts.length === 4 && parts[3] === "members" && method === "POST") {
      const body = await readBody(req);
      const m = { id: "mem_" + body.name, name: body.name, display_name: body.display_name, role: body.role || "member", created_at: iso(0) };
      people.push(m);
      return send(res, 201, { member: m, link: "https://hub.example.com/c/myco_pair_" + Math.random().toString(36).slice(2, 18) });
    }
    if (parts.length === 5 && parts[3] === "members" && method === "PATCH") {
      const body = await readBody(req);
      const m = people.find((p) => p.name === decodeURIComponent(parts[4]));
      if (!m) return fail(res, 404, "not_found", "member not found");
      Object.assign(m, Object.fromEntries(Object.entries(body).filter(([, v]) => v !== undefined && v !== "")));
      return send(res, 200, m);
    }
    if (parts.length === 5 && parts[3] === "members" && method === "DELETE") {
      const i = people.findIndex((p) => p.name === decodeURIComponent(parts[4]));
      const [m] = people.splice(i, 1);
      return send(res, 200, { ...m, removed: true });
    }
    if (parts.length === 4 && parts[3] === "audit") return send(res, 200, { entries: audit });
    if (parts.length === 6 && parts[3] === "agents" && parts[5] === "owner" && method === "PUT") {
      const body = await readBody(req);
      const a = hub.agents.find((x) => x.name === decodeURIComponent(parts[4]));
      const m = people.find((p) => p.name === body.owner);
      if (a && m) { a.owner = m.name; a.owner_id = m.id; }
      return send(res, 200, a);
    }

    // /app/hubs/{id}/aliases/{alias} — an address others can message a node by.
    if (parts.length === 5 && parts[3] === "aliases" && (method === "PUT" || method === "DELETE")) {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const alias = parts[4];
      if (!/^[a-z0-9][a-z0-9._-]{0,62}$/.test(alias)) {
        return fail(res, 400, "invalid_argument", "别名只能用 1-63 位小写字母、数字、点、下划线或连字符，并以字母或数字开头。");
      }
      const holder = (hub.agents || []).find((a) => (a.aliases || []).includes(alias));
      if (method === "DELETE") {
        if (!holder) return fail(res, 404, "not_found", `没有名为 ${alias} 的别名`);
        holder.aliases = holder.aliases.filter((x) => x !== alias);
        return send(res, 200, { alias: { alias, target: holder.name }, removed: true });
      }
      const body = await readBody(req);
      const target = typeof body?.target === "string" ? body.target.trim() : "";
      const agent = (hub.agents || []).find((a) => a.name === target);
      if (!agent) return fail(res, 404, "not_found", `没有名为 ${target} 的节点`);
      if ((hub.agents || []).some((a) => a.name === alias)) {
        return fail(res, 409, "conflict", `${alias} 已经是节点名，别名和节点名不能重名。`);
      }
      for (const a of hub.agents || []) a.aliases = (a.aliases || []).filter((x) => x !== alias);
      agent.aliases = [...(agent.aliases || []), alias];
      return send(res, 200, { alias: { alias, target: agent.name }, dry_run: false });
    }

    if (parts.length >= 5 && parts[3] === "agents") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const agent = hub.agents.find((a) => a.name === parts[4]);
      if (!agent) return fail(res, 404, "not_found", `没有名为 ${parts[4]} 的节点`);

      if (parts.length === 5 && method === "DELETE") {
        hub.agents = hub.agents.filter((a) => a.name !== agent.name);
        hub.recent = hub.recent.filter((m) => m.from !== agent.name && m.to !== agent.name);
        return send(res, 204);
      }
      if (parts.length === 6 && parts[5] === "avatar" && method === "GET") {
        const pic = avatarPics.get(`${hub.id}/${agent.name}`);
        if (!pic || !String(agent.avatar || "").startsWith("pic:")) return fail(res, 404, "not_found", `${agent.name} has no picture`);
        res.writeHead(200, { "Content-Type": pic.type, "Content-Length": pic.data.length, "Cache-Control": "private, max-age=86400, immutable" });
        return res.end(pic.data);
      }
      if (parts.length === 6 && parts[5] === "profile" && method === "PUT") {
        const body = await readBody(req);
        if (body?.avatar !== undefined) {
          // A hub before faces refuses the field, as a real one does.
          if (process.env.MOCK_OLD_HUB) return fail(res, 400, "invalid_argument", 'invalid JSON body: json: unknown field "avatar"');
          const m = /^data:(image\/(?:png|jpeg|webp|gif));base64,(.+)$/.exec(body.avatar);
          if (m) {
            avatarPics.set(`${hub.id}/${agent.name}`, { type: m[1], data: Buffer.from(m[2], "base64") });
            agent.avatar = `pic:${(state.counter += 1).toString(16)}`;
          } else if (body.avatar === "" || /^look:[a-z0-9-]{1,48}$/.test(body.avatar)) {
            agent.avatar = body.avatar || undefined;
          } else {
            return fail(res, 400, "invalid_argument", "avatar must be empty, look:<parts> or a picture");
          }
        }
        if (body?.display_name !== undefined) agent.display_name = body.display_name || undefined;
        if (body?.description !== undefined) agent.description = body.description;
        return send(res, 200, agent);
      }
      // /app/hubs/{id}/agents/{name}/run-settings — the model and effort this
      // node is woken with; "" follows the computer's setting.
      if (parts.length === 6 && parts[5] === "run-settings" && method === "PUT") {
        if (process.env.MOCK_OLD_HUB) {
          return fail(res, 404, "not_found", `no route for ${method} /app/hubs/${hub.id}/agents/${agent.name}/run-settings`);
        }
        const body = await readBody(req);
        if (!body || typeof body.model !== "string" || typeof body.effort !== "string") {
          return fail(res, 400, "invalid_argument", 'model and effort are required ("" follows the computer)');
        }
        const was = runSettingsAtStart.get(`${hub.id}/${agent.name}`);
        const rt = agent.runtime || (agent.runtime = { cli: agent.binding?.harness || "codex" });
        // Omission preserves access: managers may still change the model,
        // but only the owner can change permissions (including to "").
        const access = body.access === undefined ? agent.run_settings?.access || "" : body.access;
        if (body.access !== undefined) {
          const me = whoami(hub)?.member;
          const owns = me && (agent.owner_id ? agent.owner_id === me.id : !agent.owner && me.role === "owner");
          if (!owns) return fail(res, 403, "permission_denied", "只有这个 agent 的主人能修改权限。");
          const levels = ["readonly", "workspace", "full"];
          if (!["", ...levels].includes(access)) return fail(res, 400, "invalid_argument", "权限必须是 readonly、workspace 或 full。");
          const cap = Math.max(levels.indexOf(rt.max_access), levels.indexOf(rt.access));
          if (access && levels.indexOf(access) > cap) return fail(res, 403, "permission_denied", "这台电脑不允许更高的权限，请在那台电脑上修改 spawn.max_access。");
        }
        const mcpTools = body.mcp_tools === undefined ? !!agent.run_settings?.mcp_tools : !!body.mcp_tools;
        if (body.mcp_tools !== undefined) {
          const me = whoami(hub)?.member;
          const owns = me && (agent.owner_id ? agent.owner_id === me.id : !agent.owner && me.role === "owner");
          if (!owns && mcpTools !== !!agent.run_settings?.mcp_tools) return fail(res, 403, "permission_denied", "只有这个 agent 的主人能修改 MCP 工具权限。");
        }
        agent.run_settings = {};
        if (access) agent.run_settings.access = access;
        if (mcpTools) agent.run_settings.mcp_tools = true;
        if (body.model) agent.run_settings.model = body.model;
        if (body.effort) agent.run_settings.effort = body.effort;
        // What the runtime now reports, as a real hub's runner would.
        if (body.model) {
          rt.model = body.model;
          rt.model_source = "app";
        } else if (was?.runtime) {
          if (was.runtime.model) rt.model = was.runtime.model;
          else delete rt.model;
          // Following the computer again: what it was before the App chose.
          rt.model_source = was.run_settings?.model ? "configured" : was.runtime.model_source;
          if (!rt.model_source) delete rt.model_source;
        }
        if (body.effort) {
          rt.effort = body.effort;
          rt.effort_source = "app";
        } else if (was?.runtime) {
          if (was.runtime.effort) rt.effort = was.runtime.effort;
          else delete rt.effort;
          rt.effort_source = was.run_settings?.effort ? "configured" : was.runtime.effort_source;
          if (!rt.effort_source) delete rt.effort_source;
        }
        lastRunSettings = { hub: hub.id, agent: agent.name, body };
        return send(res, 200, agent);
      }
      // /app/hubs/{id}/agents/{name}/resume — the line that brings the same
      // agent back in a new sandbox. Making one revokes the one before.
      if (parts.length === 6 && parts[5] === "resume") {
        if (process.env.MOCK_OLD_HUB) {
          return fail(res, 404, "not_found", `no route for ${method} /app/hubs/${hub.id}/agents/${agent.name}/resume`);
        }
        if (method === "POST") {
          const token = "rsm_" + Math.random().toString(36).slice(2, 12);
          agent.resume = { created_at: new Date().toISOString() };
          return send(res, 200, {
            url: `${hub.url}/r/${token}`,
            command: `curl -fsSL ${hub.url}/r/${token} | sh`,
          });
        }
        if (method === "DELETE") {
          delete agent.resume;
          return send(res, 200, { ok: true });
        }
      }
      if (parts.length === 6 && parts[5] === "history" && method === "GET") {
        const t = (min) => new Date(Date.now() - min * 60000).toISOString();
        return send(res, 200, { entries: [
          { id: "aud_5", at: t(3), actor: "glenn", actor_kind: "member", action: "agent.edit", target: agent.name, detail: "skills: +release ~review" },
          { id: "aud_4", at: t(40), actor: agent.name, actor_kind: "agent", action: "agent.edit", target: agent.name, detail: "description: 规划发布并审查 PR。\nlabels: +group=官网改版 −group=web" },
          { id: "aud_3", at: t(180), actor: "glenn", actor_kind: "member", action: "agent.visibility", target: agent.name, detail: "team" },
          { id: "aud_2", at: t(60 * 26), actor: agent.name, actor_kind: "agent", action: "agent.edit", target: agent.name, detail: "skills: +review" },
          { id: "aud_1", at: t(60 * 50), actor: "glenn", actor_kind: "member", action: "agent.edit", target: agent.name, detail: "display_name: — → PM (Codex)" },
        ] });
      }
      if (parts.length === 6 && parts[5] === "coordinator" && method === "PUT") {
        const body = await readBody(req);
        agent.coordinator = !!body?.coordinator;
        return send(res, 200, agent);
      }
      if (parts.length === 6 && parts[5] === "visibility" && method === "PUT") {
        const body = await readBody(req);
        if (!["team", "private"].includes(body?.visibility)) return fail(res, 400, "invalid_argument", "invalid visibility");
        agent.visibility = body.visibility;
        return send(res, 200, agent);
      }
      if (parts.length === 6 && parts[5] === "skills" && method === "PUT") {
        const body = await readBody(req);
        const skills = (body?.skills || []).map((x) => ({ name: String(x.name || "").trim().toLowerCase(), summary: x.summary || undefined, examples: (x.examples || []).filter(Boolean) }));
        if (skills.some((x) => !/^[a-z0-9][a-z0-9._-]*$/.test(x.name))) return fail(res, 400, "invalid_argument", "invalid skill name");
        agent.skills = skills; agent.capabilities = skills.map((x) => x.name);
        return send(res, 200, agent);
      }
      if (parts.length === 6 && parts[5] === "labels" && method === "PUT") {
        const body = await readBody(req);
        const add = (body?.add || []).map(String), remove = (body?.remove || []).map(String);
        if ([...add].some((l) => /\s|[A-Z]/.test(l.split("=")[0]))) return fail(res, 400, "invalid_argument", "invalid label");
        let labels = [...new Set([...(agent.labels || []), ...add])];
        labels = labels.filter((l) => !remove.some((r) => l === r || (!r.includes("=") && l.startsWith(r + "=")))).sort();
        agent.labels = labels;
        return send(res, 200, agent);
      }
      if (parts.length === 6 && parts[5] === "allow_from" && method === "PUT") {
        const body = await readBody(req);
        if (!Array.isArray(body?.allow_from)) return fail(res, 400, "invalid_request", "allow_from 必须是字符串数组");
        agent.allow_from = body.allow_from.map(String);
        return send(res, 200, agent);
      }
      if (parts.length === 6 && parts[5] === "kick" && method === "POST") {
        agent.online = false;
        agent.status_text = undefined;
        agent.last_seen_at = iso(0);
        return send(res, 204);
      }
    }
    return fail(res, 404, "not_found", `没有匹配的路由：${method} ${url.pathname}`);
  }

  return fail(res, 404, "not_found", `没有匹配的路由：${method} ${url.pathname}`);
});

server.listen(PORT, "127.0.0.1", () => {
  console.error(`myco app mock listening on http://127.0.0.1:${PORT}`);
});

// Doubao's voices, as the App backend ships them.
let voicesCache;
function doubaoVoices() {
  return (voicesCache ||= JSON.parse(readFileSync(new URL("../../internal/console/voice_doubao_voices.json", import.meta.url), "utf8")));
}

// A quiet 0.3s tone as 16-bit mono WAV, for a voice being tried.
function toneWav() {
  const rate = 24000, n = Math.round(rate * 0.3);
  const buf = Buffer.alloc(44 + n * 2);
  buf.write("RIFF", 0); buf.writeUInt32LE(36 + n * 2, 4); buf.write("WAVEfmt ", 8);
  buf.writeUInt32LE(16, 16); buf.writeUInt16LE(1, 20); buf.writeUInt16LE(1, 22);
  buf.writeUInt32LE(rate, 24); buf.writeUInt32LE(rate * 2, 28); buf.writeUInt16LE(2, 32); buf.writeUInt16LE(16, 34);
  buf.write("data", 36); buf.writeUInt32LE(n * 2, 40);
  for (let i = 0; i < n; i++) buf.writeInt16LE(Math.round(Math.sin((2 * Math.PI * 440 * i) / rate) * 2000 * Math.min(1, (n - i) / 2000)), 44 + i * 2);
  return buf;
}

// One automation to start with: a weekday morning summary for the PM.
/** pm-codex's handovers: from a claude.ai session to Codex on the Studio
 *  Mac, two days ago (done), and an older command nobody ran (expired). */
function seedHandovers(name) {
  if (name !== "pm-codex") return [];
  const cloud = { binding: { harness: "claude-cloud", session_id: "session_01PmCloud7f2", environment: "claude.ai" }, runtime: { cli: "claude-cloud" } };
  const studio = { binding: { harness: "codex", session_id: "01a1-pm-session", workdir: "~/code/myco" }, machine_id: "m_studio", machine_name: "Studio Mac", runtime: { cli: "codex" } };
  return [
    { id: "hov_pm2", agent: name, at: iso(2 * 86_400 + 300), by: "glenn", note: "claude.ai 的订阅到期了，换到 Studio Mac 上用 Codex 继续",
      from: cloud, to: studio, state: "done", done_at: iso(2 * 86_400), open_tasks: ["m_pm_open"], back: "", expires_at: iso(86_400) },
    { id: "hov_pm1", agent: name, at: iso(3 * 86_400), by: "glenn", note: "",
      from: cloud, to: { binding: {} }, state: "expired", open_tasks: [], back: "", expires_at: iso(2 * 86_400) },
  ];
}

/* ---------- workflows (mock): a run walks its steps on timers ---------- */
function wfStartRun(hub, wf, w, inputs, trigger) {
  state.counter += 1;
  const r = { id: `run_${state.counter}`, workflow_id: w.id, name: w.name, version: w.version, status: "running", inputs,
    owner: "glenn", by: "glenn", trigger, created_at: iso(0), updated_at: iso(0), steps: [], events: [], def: w.def };
  wf.runs.unshift(r);
  r.events.push({ at: iso(0), kind: "started", text: trigger === "automation" ? "自动化启动了" : "glenn 启动了" });
  wfActivate(hub, r, w.def.steps[0].id);
  return r;
}
function wfEdges(def) {
  if (def.edges && def.edges.length) return def.edges;
  return def.steps.slice(0, -1).map((x, i) => ({ from: x.id, to: def.steps[i + 1].id }));
}
function wfFill(text, r) {
  return String(text || "").replace(/\{\{#if [^}]*\}\}[\s\S]*?\{\{\/if\}\}/g, "").replace(/\{\{\s*inputs\.([\w-]+)\s*\}\}/g, (_, k) => String(r.inputs?.[k] ?? ""))
    .replace(/\{\{\s*steps\.([\w-]+)\.summary\s*\}\}/g, (_, k) => [...r.steps].reverse().find((x) => x.step === k)?.summary || "")
    .replace(/\{\{[^}]*\}\}/g, "…");
}
function wfActivate(hub, r, id, payload) {
  const def = r.def.steps.find((x) => x.id === id);
  if (!def || r.status === "failed" || r.status === "cancelled") return;
  const kind = def.computer ? "computer" : def.human ? "human" : "agent";
  const st = { id: `rs_${r.id}_${r.steps.length + 1}`, step: id, title: def.title, kind, attempt: r.steps.filter((x) => x.step === id).length + 1,
    status: kind === "human" ? "waiting" : "running", target: kind === "human" ? "glenn" : def.agent || def.computer || (def.role ? "docs-claude" : undefined), started_at: iso(0),
    input: kind === "computer" ? wfFill(def.run, r) : wfFill(def.prompt || def.title, r), payload: payload ?? r.inputs };
  if (kind === "human") { st.options = def.options; st.due_at = iso(-20 * 3600); }
  r.steps.push(st);
  r.status = kind === "human" && !r.steps.some((x) => x.status === "running") ? "waiting" : "running";
  r.events.push({ at: iso(0), kind: "step", step: id, text: kind === "human" ? "等 glenn 决定" : def.role ? `按分工「${def.role}」交给 ${st.target}（空闲）` : `交给 ${st.target}` });
  if (kind === "human") return;
  const threadId = `th_wf_${r.id}_${r.steps.length}`;
  const m = makeMessage({ thread_id: threadId, from: CONSOLE, to: st.target, type: "task", status: "acked", body: `[工作流「${r.name}」· ${def.title || id}]\n\n${st.input}`, workflow: r.id, created_at: iso(0), updated_at: iso(0) });
  (hub.threads ||= {})[threadId] = [m];
  st.message_id = m.id;
  setTimeout(() => {
    if (st.status !== "running") return;
    st.status = "done"; st.ended_at = iso(0);
    st.summary = kind === "computer" ? "退出码 0" : `${def.title || id}做完了`;
    st.output = kind === "computer" ? { exit_code: 0, stdout: "ok\n" } : { summary: st.summary, text: st.summary };
    m.status = "done";
    hub.threads[threadId].push(makeMessage({ thread_id: threadId, from: st.target, to: CONSOLE, type: "reply", status: "done", body: st.summary, reply_to: m.id, created_at: iso(0), updated_at: iso(0) }));
    r.events.push({ at: iso(0), kind: "done", step: id, text: st.summary });
    wfNext(hub, r, st);
  }, 2500);
}
function wfNext(hub, r, st) {
  const out = wfEdges(r.def).filter((e) => e.from === st.step && !e.onError);
  const def = r.def.steps.find((x) => x.id === st.step);
  const take = def?.dispatch === "all" ? out : out.slice(0, 1);
  for (const e of take) {
    if (e.to === "$fail") return wfEnd(r, "failed", `从 ${st.step} 走到了「失败」`);
    if (e.to !== "$end") {
      const target = r.def.steps.find((x) => x.id === e.to);
      if (target?.join === "all") {
        const preds = [...new Set(wfEdges(r.def).filter((x) => x.to === e.to).map((x) => x.from))];
        if (!preds.every((p) => r.steps.some((x) => x.step === p && x.status === "done"))) continue;
      }
      wfActivate(hub, r, e.to, e.map ? Object.fromEntries(Object.keys(e.map).map((k) => [k, st.output?.[k] ?? ""])) : st.output);
    }
  }
  if (!r.steps.some((x) => x.status === "running" || x.status === "waiting") && r.status !== "failed" && r.status !== "cancelled") wfEnd(r, "succeeded", "");
  else if (r.status === "running" && !r.steps.some((x) => x.status === "running")) r.status = "waiting";
}
function wfEnd(r, status, why) {
  for (const x of r.steps) if (x.status === "running" || x.status === "waiting") { x.status = "cancelled"; x.ended_at = iso(0); x.error = "运行结束了"; }
  r.status = status; r.error = why || undefined; r.ended_at = iso(0); r.updated_at = iso(0);
  r.events.push({ at: iso(0), kind: "finished", text: ({ succeeded: "完成了", failed: "失败了", cancelled: "取消了" })[status] + (why ? `：${why}` : "") });
}
/* observability (v0.21.1): metrics, step views, stats, comparison, logs */
function wfMs(from, to) { return Math.max(0, (to ? Date.parse(to) : Date.now()) - Date.parse(from)); }
function wfStepView(st) {
  return { ...st, duration_ms: st.started_at ? wfMs(st.started_at, st.ended_at) : 0, attempts: st.attempts || 1 + (st.retries || 0) + (st.bounces || 0) };
}
function wfCover(spans) {
  const sp = [...spans].sort((a, b) => a[0] - b[0]);
  let total = 0, cur = null;
  for (const s of sp) {
    if (!cur || s[0] > cur[1]) { if (cur) total += cur[1] - cur[0]; cur = [...s]; } else if (s[1] > cur[1]) cur[1] = s[1];
  }
  return cur ? total + cur[1] - cur[0] : 0;
}
function wfMetrics(r) {
  const work = [], wait = [];
  const u = { input_tokens: 0, output_tokens: 0, cost_usd: 0 };
  let done = 0, failed = 0, attempts = 0, retries = 0, bounces = 0;
  for (const st of r.steps) {
    const v = wfStepView(st);
    if (st.status === "done") done++;
    if (st.status === "failed") failed++;
    attempts += v.attempts; retries += st.retries || 0; bounces += st.bounces || 0;
    if (st.usage) { u.input_tokens += st.usage.input_tokens || 0; u.output_tokens += st.usage.output_tokens || 0; u.cost_usd += st.usage.cost_usd || 0; }
    const from = Date.parse(st.started_at);
    (st.kind === "human" ? wait : work).push([from, from + v.duration_ms]);
  }
  const active = wfCover(work);
  return { duration_ms: wfMs(r.created_at, r.ended_at), active_ms: active, waiting_ms: wfCover([...work, ...wait]) - active, steps: r.steps.length, done, failed,
    attempts, retries, bounces, ...(u.input_tokens || u.cost_usd ? { usage: u } : {}) };
}
function wfDist(xs) {
  if (!xs.length) return { n: 0, avg: 0, min: 0, p50: 0, p90: 0, p95: 0, max: 0 };
  const v = [...xs].sort((a, b) => a - b);
  const rank = (p) => v[Math.max(0, Math.ceil(p * v.length) - 1)];
  return { n: v.length, avg: v.reduce((a, b) => a + b, 0) / v.length, min: v[0], p50: rank(0.5), p90: rank(0.9), p95: rank(0.95), max: v[v.length - 1] };
}
function wfAddUsage(a, u) { if (!u) return a; a.input_tokens += u.input_tokens || 0; a.output_tokens += u.output_tokens || 0; a.cost_usd += u.cost_usd || 0; return a; }
function wfStats(wf, w, version) {
  const all = wf.runs.filter((r) => r.workflow_id === w.id);
  const by = {};
  for (const r of all) by[r.status] = (by[r.status] || 0) + 1;
  const ended = (by.succeeded || 0) + (by.failed || 0) + (by.cancelled || 0);
  const runs = all.filter((r) => !version || r.version === version).slice(0, 200);
  const over = (r) => ["succeeded", "failed", "cancelled"].includes(r.status);
  const ms = runs.map((r) => [r, wfMetrics(r)]);
  const usage = { input_tokens: 0, output_tokens: 0, cost_usd: 0 };
  for (const [, m] of ms) wfAddUsage(usage, m.usage);
  const steps = new Map();
  for (const r of runs) for (const st of r.steps) {
    const a = steps.get(st.step) || { step: st.step, title: w.def.steps.find((x) => x.id === st.step)?.title, kind: st.kind, runs: 0, done: 0, failed: 0, retries: 0, bounces: 0, d: [], usage: { input_tokens: 0, output_tokens: 0, cost_usd: 0 } };
    a.runs++; a.retries += st.retries || 0; a.bounces += st.bounces || 0; wfAddUsage(a.usage, st.usage);
    if (st.status === "done") a.done++;
    if (st.status === "failed") { a.failed++; if (!a.last_error_at || st.ended_at > a.last_error_at) Object.assign(a, { last_error: st.error, last_error_run: r.id, last_error_at: st.ended_at }); }
    if (st.ended_at) a.d.push(wfMs(st.started_at, st.ended_at));
    steps.set(st.step, a);
  }
  const order = w.def.steps.map((x) => x.id);
  const stepList = [...steps.values()].sort((a, b) => order.indexOf(a.step) - order.indexOf(b.step))
    .map(({ d, usage: u, ...a }) => ({ ...a, duration_ms: wfDist(d), ...(u.input_tokens || u.cost_usd ? { usage: u } : {}) }));
  const vers = new Map();
  for (const [r, m] of ms) {
    const v = vers.get(r.version) || { version: r.version, runs: 0, succeeded: 0, failed: 0, cancelled: 0, d: [] };
    v.runs++; if (v[r.status] !== undefined) v[r.status]++; if (over(r)) v.d.push(m.duration_ms);
    vers.set(r.version, v);
  }
  const latest = new Map();
  for (const r of runs) for (const e of r.evaluations || []) latest.set(`${r.id}|${e.name}`, e);
  const evs = new Map();
  for (const e of latest.values()) {
    const x = evs.get(e.name) || { name: e.name, unit: e.unit, direction: e.direction, vals: [], last: e.value, last_run: e.run, at: "" };
    x.vals.push(e.value); if (e.at >= x.at) Object.assign(x, { last: e.value, last_run: e.run, at: e.at });
    evs.set(e.name, x);
  }
  const point = ([r, m]) => ({ id: r.id, status: r.status, version: r.version, created_at: r.created_at, duration_ms: m.duration_ms, ...(r.error ? { error: r.error } : {}),
    ...(r.steps.some((x) => x.status === "failed") ? { failed_step: [...r.steps].reverse().find((x) => x.status === "failed").step } : {}), ...(m.usage?.cost_usd ? { cost_usd: m.usage.cost_usd } : {}) });
  const fin = ms.filter(([r]) => over(r)).map(([, m]) => m);
  return {
    workflow: w.id, name: w.name, total: all.length, by_status: by, success_rate: ended ? (by.succeeded || 0) / ended : 0, sample: runs.length, ...(version ? { version } : {}),
    duration_ms: wfDist(fin.map((m) => m.duration_ms)), active_ms: wfDist(fin.map((m) => m.active_ms)), waiting_ms: wfDist(fin.map((m) => m.waiting_ms)),
    ...(usage.input_tokens || usage.cost_usd ? { usage, cost_per_run: usage.cost_usd / Math.max(1, runs.length) } : {}),
    steps: stepList, versions: [...vers.values()].sort((a, b) => b.version - a.version).map(({ d, ...v }) => ({ ...v, duration_ms: wfDist(d) })),
    evaluations: [...evs.values()].sort((a, b) => a.name.localeCompare(b.name)).map(({ vals, at, ...x }) => ({ ...x, values: wfDist(vals) })),
    recent: ms.slice(0, 60).map(point), failures: ms.filter(([r]) => r.status === "failed").slice(0, 10).map(point),
  };
}
function wfCompare(a, b, view) {
  const side = (r, id) => {
    const xs = r.steps.filter((x) => x.step === id);
    if (!xs.length) return undefined;
    const last = xs[xs.length - 1];
    const u = xs.reduce((acc, x) => wfAddUsage(acc, x.usage), { input_tokens: 0, output_tokens: 0, cost_usd: 0 });
    return { status: last.status, runs: xs.length, attempts: xs.reduce((n, x) => n + wfStepView(x).attempts, 0), duration_ms: xs.reduce((n, x) => n + wfStepView(x).duration_ms, 0),
      ...(u.input_tokens || u.cost_usd ? { usage: u } : {}), summary: last.summary, error: last.error };
  };
  const ids = [...new Set([...a.def.steps, ...b.def.steps].map((x) => x.id))];
  const steps = ids.map((id) => {
    const sa = side(a, id), sb = side(b, id);
    if (!sa && !sb) return null;
    const d = { step: id, title: (b.def.steps.find((x) => x.id === id) || a.def.steps.find((x) => x.id === id))?.title, a: sa, b: sb, delta_ms: 0 };
    if (sa && sb) { d.delta_ms = sb.duration_ms - sa.duration_ms; d.delta_cost_usd = (sb.usage?.cost_usd || 0) - (sa.usage?.cost_usd || 0); if (d.delta_ms) d.faster_in = d.delta_ms < 0 ? "b" : "a"; }
    return d;
  }).filter(Boolean);
  const last = (r) => new Map((r.evaluations || []).map((e) => [e.name, e]));
  const ea = last(a), eb = last(b);
  const names = [...new Set([...ea.keys(), ...eb.keys()])].sort();
  const evaluations = names.map((n) => {
    const x = ea.get(n), y = eb.get(n);
    const e = { name: n, unit: (y || x).unit, direction: (y || x).direction, ...(x ? { a: x.value } : {}), ...(y ? { b: y.value } : {}), delta: x && y ? y.value - x.value : 0 };
    if (x && y && x.value !== y.value && e.direction) e.better = (e.direction === "higher") === (x.value > y.value) ? "a" : "b";
    return e;
  });
  return { a: { run: view(a), metrics: wfMetrics(a) }, b: { run: view(b), metrics: wfMetrics(b) }, same_version: a.workflow_id === b.workflow_id && a.version === b.version, steps, evaluations };
}
function wfStepLog(hub, r, st) {
  const out = [];
  const shift = (t, s) => new Date(Date.parse(t) + s * 1000).toISOString();
  if (st.message_id) {
    for (const th of Object.values(hub.threads || {})) {
      if (!th.some((m) => m.id === st.message_id)) continue;
      for (const m of th) {
        if (m.type === "task") out.push({ at: m.created_at, kind: "task", try: 1, from: m.from, text: m.body });
        else if (m.type === "progress") out.push({ at: m.created_at, kind: "progress", try: 1, from: m.from, text: m.body, attachments: m.attachments });
        else if (m.type === "reply") out.push({ at: m.created_at, kind: "reply", try: 1, from: m.from, text: m.body, attachments: m.attachments });
      }
    }
  }
  for (const t of st.trace || []) out.push({ at: shift(st.started_at, t[0]), kind: "trace", sub: t[1], try: 1, text: t[2] });
  if (st.kind === "computer" && st.output) {
    const at = st.ended_at || iso(0);
    if (st.output.stdout) out.push({ at, kind: "stdout", text: st.output.stdout, from: st.target });
    if (st.output.stderr) out.push({ at, kind: "stderr", text: st.output.stderr, from: st.target });
    out.push({ at, kind: "event", sub: "exit", text: `退出码 ${st.output.exit_code ?? 0}` });
  }
  for (const e of r.events) if (e.step === st.step && e.at >= st.started_at && (!st.ended_at || e.at <= shift(st.ended_at, 1))) out.push({ at: e.at, kind: "event", sub: e.kind, text: e.text });
  return out.sort((a, b) => a.at.localeCompare(b.at));
}
function seedWorkflows(hub) {
  if (hub.id !== "hub_studio") return { list: [], runs: [] };
  const review = {
    name: "写-审-改", description: "一个做、一个审，没过就改，最多 3 轮；最后你点头。",
    inputs: { type: "object", required: ["task"], properties: { task: { type: "string", title: "要做什么", format: "textarea", minLength: 4 } } },
    steps: [
      { id: "write", title: "做", agent: "pm-codex", prompt: "完成这件事：{{inputs.task}}",
        output: { type: "object", required: ["where"], properties: { where: { type: "string", description: "改动在哪个分支", pattern: "^feat/" }, summary: { type: "string", maxLength: 200 } } } },
      { id: "review", title: "审", role: "审查", prompt: "审一下 {{input.where}} 的改动：{{steps.write.summary}}",
        inputSchema: { type: "object", required: ["where"], properties: { where: { type: "string", pattern: "^feat/" } } },
        outputSchema: { type: "object", required: ["approved"], properties: { approved: { type: "boolean", description: "能不能合" }, comments: { type: "array", items: { type: "string" }, description: "要改的地方" } } } },
      { id: "ok", title: "你来定", human: "$starter", prompt: "「{{steps.write.summary}}」审过了。就这样吗？", options: ["就这样", "不要了"] },
    ],
    edges: [
      { from: "write", to: "review", map: { where: "{{output.where}}" } }, { from: "review", to: "ok", when: "output.approved" },
      { from: "review", to: "write", when: "!output.approved", maxIterations: 3, else: "$fail" },
      { from: "ok", to: "$end", when: "output.choice == '就这样'" }, { from: "ok", to: "$fail", when: "output.choice != '就这样'" },
    ],
  };
  const test = {
    name: "改完跑测试", description: "改完在 Studio Mac 上跑测试，没过把报错交回去，最多 3 次。",
    inputs: { required: ["task"], properties: { task: { type: "string", title: "要改什么", format: "textarea" }, command: { type: "string", title: "测试命令", default: "make test" } } },
    steps: [
      { id: "fix", title: "改", agent: "pm-codex", prompt: "{{inputs.task}}" },
      { id: "test", title: "跑测试", computer: "mac-exec", run: "{{inputs.command}}" },
    ],
    edges: [{ from: "fix", to: "test" }, { from: "test", to: "fix", onError: true, maxIterations: 3, else: "$fail" }],
  };
  const list = [
    { id: "wf_review", name: review.name, def: review, version: 2, enabled: true, created_at: iso(9 * 86400), updated_at: iso(2 * 86400) },
    { id: "wf_test", name: test.name, def: test, version: 1, enabled: true, created_at: iso(6 * 86400), updated_at: iso(6 * 86400) },
  ];
  const task = (to, body, reply, at, runId, extra = {}) => {
    const threadId = `th_wf_seed_${msgSeq + 1}`;
    const m = makeMessage({ thread_id: threadId, from: CONSOLE, to, type: "task", status: "done", body, workflow: runId, created_at: iso(at), updated_at: iso(at - 60) });
    const replyAt = extra.replyAt ?? at - 60;
    const rep = makeMessage({ thread_id: threadId, from: to, to: CONSOLE, type: "reply", status: "done", body: reply, reply_to: m.id, created_at: iso(replyAt), updated_at: iso(replyAt),
      ...(extra.files ? { attachments: extra.files } : {}) });
    const progress = extra.progress ? [makeMessage({ thread_id: threadId, from: to, to: CONSOLE, type: "progress", status: "done", body: extra.progress.body, reply_to: m.id,
      created_at: iso(extra.progress.at), updated_at: iso(extra.progress.at), ...(extra.progress.files ? { attachments: extra.progress.files } : {}) })] : [];
    (hub.threads ||= {})[threadId] = [m, ...progress, rep];
    return m.id;
  };
  const shot = { file_id: "f_demo_shot_wf1", name: "设置页截图.png", size: 48213, sha256: "", content_type: "image/png" };
  const notes = { file_id: "f_demo_md_2", name: "改动说明.md", size: 1840, sha256: "", content_type: "text/markdown" };
  const st = (runId, i, step, kind, attempt, status, target, at, took, extra = {}) => ({
    id: `rs_${runId}_${i}`, step, kind, attempt, status, target, started_at: iso(at), ...(status === "running" || status === "waiting" ? {} : { ended_at: iso(at - took) }), ...extra,
  });
  const a = "run_seed_a";
  const runA = {
    id: a, workflow_id: "wf_review", name: review.name, version: 2, status: "waiting", inputs: { task: "设置页加一个「导出对话」按钮" },
    owner: "glenn", by: "glenn", trigger: "manual", created_at: iso(5400), updated_at: iso(300), def: review,
    steps: [
      st(a, 1, "write", "agent", 1, "done", "pm-codex", 5400, 1500, { title: "做", summary: "加好了导出按钮，放在设置 › 对话", payload: { task: "设置页加一个「导出对话」按钮" }, output: { where: "feat/export-chat", summary: "加好了导出按钮，放在设置 › 对话" },
        bounces: 1, usage: { input_tokens: 182400, output_tokens: 9100, cost_usd: 0.84 },
        trace: [[20, "text", "先看看设置页现在的结构，再决定按钮放哪。"], [45, "tool", "Read web/src/app/Settings.tsx"], [130, "tool", "Grep \"导出\" web/src"], [260, "thinking", "放在「对话」一组最合适，和清空记录挨着。"],
          [420, "tool", "Edit web/src/app/Settings.tsx"], [700, "tool", "Bash npm run typecheck"], [760, "tool_result", "> tsc -b\n(no errors)"], [1100, "tool", "Bash npm run test:ui -- --only 设置"], [1380, "tool_result", "✓ 设置 · 1200-light (3.2s)"],
          [1490, "usage", "输入 182.4k / 输出 9.1k tokens · $0.84 · 24m50s"]],
        input: "完成这件事：设置页加一个「导出对话」按钮", message_id: task("pm-codex", "[工作流「写-审-改」· 第 1 步 做]\n\n完成这件事：设置页加一个「导出对话」按钮", "加好了导出按钮，放在设置 › 对话。分支 feat/export-chat。", 5400, a,
          { replyAt: 3910, files: [shot, notes], progress: { at: 4600, body: "按钮放好了，正在跑 UI 检查。截图先给你看看位置。", files: [shot] } }) }),
      st(a, 2, "review", "agent", 1, "done", "docs-claude", 3900, 600, { title: "审", summary: "少了导出失败时的提示", payload: { where: "feat/export-chat" }, output: { approved: false, comments: ["导出失败时没有提示", "按钮文字和别处不一致"] },
        usage: { input_tokens: 64000, output_tokens: 2400, cost_usd: 0.31 },
        input: "审一下这个改动：加好了导出按钮", message_id: task("docs-claude", "[工作流「写-审-改」· 第 2 步 审]\n\n审一下这个改动：加好了导出按钮", "两处要改：导出失败时没有提示；按钮文字和别处不一致。", 3900, a) }),
      st(a, 3, "write", "agent", 2, "done", "pm-codex", 3300, 1200, { title: "做", summary: "补上失败提示，按钮改成「导出这段对话」", output: { where: "feat/export-chat" }, usage: { input_tokens: 121000, output_tokens: 5200, cost_usd: 0.52 },
        input: "完成这件事：设置页加一个「导出对话」按钮\n上一轮审查的意见：导出失败时没有提示；按钮文字和别处不一致", message_id: task("pm-codex", "[工作流「写-审-改」· 第 1 步 做 · 第 2 次]\n\n补上审查意见", "补上失败提示，按钮改成「导出这段对话」。", 3300, a) }),
      st(a, 4, "review", "agent", 2, "done", "docs-claude", 2000, 500, { title: "审", summary: "通过", output: { approved: true }, input: "审一下这个改动：补上失败提示", usage: { input_tokens: 58000, output_tokens: 1100, cost_usd: 0.24 },
        message_id: task("docs-claude", "[工作流「写-审-改」· 第 2 步 审 · 第 2 次]\n\n审一下这个改动", "通过，没有别的问题。", 2000, a) }),
      st(a, 5, "ok", "human", 1, "waiting", "glenn", 1500, 0, { title: "你来定", input: "「补上失败提示，按钮改成「导出这段对话」」审过了。就这样吗？", options: ["就这样", "不要了"], due_at: iso(-20 * 3600) }),
    ],
    events: [
      { at: iso(5400), kind: "started", text: "glenn 启动了（第 2 版）" }, { at: iso(5400), kind: "step", step: "write", text: "交给 pm-codex" },
      { at: iso(3900), kind: "done", step: "write", text: "加好了导出按钮，放在设置 › 对话" }, { at: iso(3900), kind: "edge", step: "write", text: "write → review" },
      { at: iso(3300), kind: "done", step: "review", text: "少了导出失败时的提示" }, { at: iso(3300), kind: "edge", step: "review", text: "review → write" },
      { at: iso(2000), kind: "done", step: "write", text: "补上失败提示，按钮改成「导出这段对话」" },
      { at: iso(1500), kind: "done", step: "review", text: "通过" }, { at: iso(1500), kind: "step", step: "ok", text: "等 glenn 决定" },
      { at: iso(4700), kind: "bounced", step: "write", text: "结果不符合，退回重交：where is missing" },
    ].sort((x, y) => x.at.localeCompare(y.at)),
    artifacts: [
      { step: "write", step_run: `rs_${a}_1`, try: 1, kind: "progress", message: "m", from: "pm-codex", at: iso(4600), ...shot },
      { step: "write", step_run: `rs_${a}_1`, try: 2, kind: "reply", message: "m", from: "pm-codex", at: iso(3910), ...shot },
      { step: "write", step_run: `rs_${a}_1`, try: 2, kind: "reply", message: "m", from: "pm-codex", at: iso(3910), ...notes },
    ].map(({ sha256, ...x }) => x).filter((x, i, all) => all.findIndex((y) => y.file_id === x.file_id && y.step_run === x.step_run) === i),
    evaluations: [{ id: 1, run: a, name: "quality", value: 4, unit: "分", direction: "higher", note: "失败提示加得好", by: "glenn", at: iso(1200) },
      { id: 2, run: a, step: "review", name: "comments", value: 2, unit: "条", direction: "lower", by: "docs-claude", at: iso(3300) }],
  };
  const b = "run_seed_b";
  const runB = {
    id: b, workflow_id: "wf_review", name: review.name, version: 1, status: "succeeded", inputs: { task: "README 补上 Windows 安装说明" },
    owner: "glenn", by: "pm-codex", trigger: "agent", created_at: iso(30 * 3600), updated_at: iso(28 * 3600), ended_at: iso(28 * 3600), def: review,
    steps: [
      st(b, 1, "write", "agent", 1, "done", "pm-codex", 30 * 3600, 1800, { title: "做", summary: "写好了 Windows 安装一节", input: "完成这件事：README 补上 Windows 安装说明", usage: { input_tokens: 96000, output_tokens: 6100, cost_usd: 0.46 } }),
      st(b, 2, "review", "agent", 1, "done", "docs-claude", 29.4 * 3600, 900, { title: "审", summary: "通过", output: { approved: true }, usage: { input_tokens: 41000, output_tokens: 900, cost_usd: 0.17 } }),
      st(b, 3, "ok", "human", 1, "done", "glenn", 29 * 3600, 3600, { title: "你来定", summary: "就这样", output: { choice: "就这样", by: "glenn" }, options: ["就这样", "不要了"] }),
    ],
    events: [{ at: iso(30 * 3600), kind: "started", text: "pm-codex 启动了（第 1 版）" }, { at: iso(28 * 3600), kind: "finished", text: "完成了" }],
    evaluations: [{ id: 3, run: b, name: "quality", value: 5, unit: "分", direction: "higher", by: "glenn", at: iso(28 * 3600) },
      { id: 4, run: b, step: "review", name: "comments", value: 0, unit: "条", direction: "lower", by: "docs-claude", at: iso(29 * 3600) }],
  };
  // Earlier runs: enough for the figures to say something.
  const tasks = ["首页加载慢，排查一下", "登录页适配深色模式", "导出 CSV 加上表头", "修复搜索结果重复", "设置页加快捷键说明", "通知改成可以静音",
    "README 加上 FAQ", "清理没用的依赖", "错误提示统一文案", "列表加分页", "上传加进度条", "修 Windows 路径问题"];
  const history = tasks.map((t, i) => {
    const id = `run_seed_h${i + 1}`;
    const at = (2 + i * 1.6) * 86400 + (i % 3) * 3600;
    const v = i < 6 ? 2 : 1;
    const w1 = 900 + ((i * 397) % 1500), r1 = 300 + ((i * 211) % 600), d1 = 600 + ((i * 733) % 5400);
    const status = i === 3 || i === 9 ? "failed" : i === 6 ? "cancelled" : "succeeded";
    const cost = (x) => ({ input_tokens: Math.round(x * 210000), output_tokens: Math.round(x * 9000), cost_usd: Math.round(x * 100) / 100 });
    const steps = [st(id, 1, "write", "agent", 1, "done", "pm-codex", at, w1, { title: "做", summary: `${t}：做好了`, usage: cost(0.3 + (i % 5) * 0.11), ...(i % 4 === 1 ? { bounces: 1 } : {}) })];
    if (status === "failed" && i === 3) {
      steps.push(st(id, 2, "review", "agent", 1, "failed", "docs-claude", at - w1, r1, { title: "审", error: "交的结果不符合要求：approved is missing", bounces: 1, usage: cost(0.15) }));
    } else if (status === "failed") {
      steps.push(st(id, 2, "review", "agent", 1, "failed", "docs-claude", at - w1, 1800, { title: "审", error: "超时了（30m）", retries: 1, usage: cost(0.2) }));
    } else {
      steps.push(st(id, 2, "review", "agent", 1, "done", "docs-claude", at - w1, r1, { title: "审", summary: "通过", output: { approved: true }, usage: cost(0.12 + (i % 3) * 0.05) }));
      steps.push(st(id, 3, "ok", "human", 1, status === "cancelled" ? "cancelled" : "done", "glenn", at - w1 - r1, d1, { title: "你来定", summary: status === "cancelled" ? undefined : "就这样", options: ["就这样", "不要了"] }));
    }
    const last = steps[steps.length - 1];
    const ended = last.ended_at;
    return {
      id, workflow_id: "wf_review", name: review.name, version: v, status, inputs: { task: t }, owner: "glenn", by: i % 3 === 0 ? "pm-codex" : "glenn",
      trigger: i % 3 === 0 ? "agent" : "manual", created_at: iso(at), updated_at: ended, ended_at: ended, def: review, steps,
      ...(status === "failed" ? { error: last.error } : status === "cancelled" ? { error: "glenn 取消了" } : {}),
      events: [{ at: iso(at), kind: "started", text: `glenn 启动了（第 ${v} 版）` }],
      evaluations: status === "succeeded" ? [{ id: 100 + i, run: id, name: "quality", value: [4, 3, 5, 4, 4, 3, 5, 4, 5, 4, 3, 4][i], unit: "分", direction: "higher", by: "glenn", at: ended }] : [],
    };
  });
  const c = "run_seed_c";
  const runC = {
    id: c, workflow_id: "wf_test", name: test.name, version: 1, status: "failed", inputs: { task: "修 Safari 上登录闪退", command: "make test" },
    owner: "glenn", by: "glenn", trigger: "manual", created_at: iso(3 * 86400), updated_at: iso(3 * 86400 - 3000), ended_at: iso(3 * 86400 - 3000), def: test,
    error: "test → fix 到了最多 3 次",
    steps: [1, 2, 3, 4].flatMap((n) => [
      st(c, n * 2 - 1, "fix", "agent", n, "done", "pm-codex", 3 * 86400 - (n - 1) * 700, 400, { title: "改", summary: n === 1 ? "改了登录页的 cookie 处理" : `第 ${n} 次修：调整了重试` }),
      st(c, n * 2, "test", "computer", n, "failed", "mac-exec", 3 * 86400 - (n - 1) * 700 - 400, 120, { title: "跑测试", error: "退出码 2：FAIL web/login.test.ts › keeps the session", input: "make test",
        output: { exit_code: 2, stdout: "", stderr: "FAIL web/login.test.ts › keeps the session\n" } }),
    ]).slice(0, 8),
    events: [{ at: iso(3 * 86400), kind: "started", text: "glenn 启动了（第 1 版）" }, { at: iso(3 * 86400 - 3000), kind: "finished", text: "失败了：test → fix 到了最多 3 次" }],
  };
  return { list, runs: [runA, runB, ...history, runC].sort((x, y) => y.created_at.localeCompare(x.created_at)) };
}

function seedAutomations(hub) {
  if (hub.id !== "hub_studio") return [];
  // Each run that went out has its task in the chat with its agent, done
  // (and answered), so 看对话 lands somewhere.
  const task = (aid, to, at, body) => {
    const threadId = `th_auto_seed_${msgSeq + 1}`;
    const m = makeMessage({ thread_id: threadId, from: CONSOLE, to, type: "task", status: "done", body, automation: aid, created_at: iso(at), updated_at: iso(at - 300) });
    const reply = makeMessage({ thread_id: threadId, from: to, to: CONSOLE, type: "reply", status: "done", body: "看过了，清单在这里：\n1. #212 登录在 Safari 上闪退（要紧）\n2. #215 文档链接坏了", reply_to: m.id, created_at: iso(at - 300), updated_at: iso(at - 300) });
    (hub.threads ||= {})[threadId] = [m, reply];
    return m.id;
  };
  const day = 86400;
  const mk = (aid, to, i, at, trigger, outcome, extra = {}) => ({
    id: `run_seed_${aid}_${i}`, automation_id: aid, at: iso(at), trigger, outcome,
    ...(outcome === "sent" ? { message_id: task(aid, to, at, extra.body || "看一下昨天新开的 issue，挑出要紧的，在这里回我一份清单。") } : {}),
    ...(extra.error ? { error: extra.error } : {}),
    ...(extra.request ? { request: extra.request } : {}),
  });
  const morning = [
    mk("aut_morning", "pm-codex", 1, 10 * 3600, "schedule", "sent"),
    mk("aut_morning", "pm-codex", 2, 10 * 3600 + day, "schedule", "sent"),
    mk("aut_morning", "pm-codex", 3, 10 * 3600 + 2 * day, "schedule", "skipped", { error: "the last task it sent is not finished" }),
    mk("aut_morning", "pm-codex", 4, 10 * 3600 + 3 * day, "manual", "sent"),
    mk("aut_morning", "pm-codex", 5, 10 * 3600 + 5 * day, "schedule", "sent"),
  ];
  const ci = (n, sha, job) => JSON.stringify({ action: "completed", workflow_run: { name: "CI", conclusion: "failure", head_branch: "main", head_sha: sha, run_number: n,
    html_url: `https://github.com/acme/web/actions/runs/${n}`, failed_job: job } });
  const hookReq = (body) => ({ content_type: "application/json", size: body.length,
    headers: { "Content-Type": "application/json", "User-Agent": "GitHub-Hookshot/5c3a2e1", "X-Github-Event": "workflow_run", "X-Github-Delivery": "8f1c0d2e-77aa-11ef-9b6e-1d3c4f5a6b7c" }, body });
  const ciBodies = [ci(4812, "9c1e7a2", "test (ubuntu, go 1.24)"), ci(4807, "41bd0f3", "lint"), ci(4801, "e07c55d", "test (windows)")];
  const hooks = [
    mk("aut_ci", "release-cloud", 1, 2 * 3600, "webhook", "sent", { body: `CI 构建失败了，看一下日志找出原因：\n${ciBodies[0]}`, request: hookReq(ciBodies[0]) }),
    mk("aut_ci", "release-cloud", 2, 2 * 3600 + 400, "webhook", "skipped", { error: "the last task it sent is not finished", request: hookReq(ciBodies[1]) }),
    mk("aut_ci", "release-cloud", 3, 3 * day, "webhook", "sent", { body: `CI 构建失败了，看一下日志找出原因：\n${ciBodies[2]}`, request: hookReq(ciBodies[2]) }),
    mk("aut_ci", "release-cloud", 4, 6 * day, "webhook", "failed", { error: "release-cloud is not in this workspace any more", request: { content_type: "text/plain", size: 0, headers: { "User-Agent": "curl/8.7.1" } } }),
  ];
  const issueBody = JSON.stringify({ action: "opened", issue: { number: 231, title: "设置页加一个「导出对话」按钮" } });
  const issueRuns = [
    { id: "run_seed_aut_issue_1", automation_id: "aut_issue", at: iso(5400), trigger: "webhook", outcome: "sent", workflow_run: "run_seed_a",
      request: { content_type: "application/json", size: issueBody.length, headers: { "User-Agent": "GitHub-Hookshot/5c3a2e1", "X-Github-Event": "issues" }, body: issueBody } },
  ];
  return [{
    id: "aut_morning", name: "每天早上汇总 issue", owner: "glenn", from: CONSOLE, to: "pm-codex",
    body: "看一下昨天新开的 issue，挑出要紧的，在这里回我一份清单。", schedule: { cron: "0 9 * * 1-5", tz: "Asia/Shanghai" },
    webhook: false, skip_if_busy: true, enabled: true, created_at: iso(7 * 86400), updated_at: iso(86400),
    next_run_at: new Date(Date.now() + 14 * 3600e3).toISOString(),
    last_run: morning[0], runs: morning,
  }, {
    id: "aut_ci", name: "CI 失败时排查", owner: "glenn", from: CONSOLE, to: "release-cloud",
    body: "CI 构建失败了，看一下日志找出原因，能修就修，修不了告诉我卡在哪：\n{{payload}}",
    webhook: true, webhook_url: `${hub.url.replace(/\/+$/, "")}/hooks/aut_ci`, skip_if_busy: true, enabled: true, created_at: iso(10 * 86400), updated_at: iso(10 * 86400),
    last_run: hooks[0], runs: hooks,
  }, {
    // Starts a workflow: each new issue goes through 写-审-改.
    id: "aut_issue", name: "新 issue 走写-审-改", owner: "glenn", from: CONSOLE, to: "",
    body: "", workflow: "wf_review", inputs: { task: "修 issue：{{payload.issue.title}}" },
    webhook: true, webhook_url: `${hub.url.replace(/\/+$/, "")}/hooks/aut_issue`, skip_if_busy: false, enabled: true, created_at: iso(4 * 86400), updated_at: iso(4 * 86400),
    last_task: "run_seed_a",
    last_run: issueRuns[0], runs: issueRuns,
  }];
}
