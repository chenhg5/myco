// Dev-only stand-in for the App backend (cmd/myco-app). No dependencies.
//   node mock/app-server.mjs        (or: npm run dev:app)
// Serves the `app/...` contract with two hubs: one healthy, one unreachable.
// Writes mutate in-memory state and are lost on restart.
import http from "node:http";
import { showcase } from "./showcase.mjs";

const PORT = Number(process.env.APP_MOCK_PORT || 7781);
// MOCK_SHELL=mac reserves room for the macOS traffic lights; browser does not.
const SHELL = process.env.MOCK_SHELL === "browser" ? "browser" : "mac";
// MOCK_EMPTY=1 starts with no hubs, to see the welcome page.
const EMPTY = process.env.MOCK_EMPTY === "1";
// MOCK_PENDING_LINK is returned once by GET /app/pending-link.
let pendingLink = process.env.MOCK_PENDING_LINK || "";
const iso = (offsetSec = 0) => new Date(Date.now() - offsetSec * 1000).toISOString();
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// CLI metadata used when a session is spawned from the App.
const CLI_MODELS = {
  claude: "claude-opus-5-5", codex: "gpt-6-sol", pi: "pi-3", kiro: "kiro-2",
  cursor: "cursor-auto", gemini: "gemini-3-pro", opencode: "claude-opus-5-5",
};
const CLI_VERSIONS = {
  claude: "2.1.240", codex: "0.52.0", pi: "1.8.0", kiro: "0.4.2",
  cursor: "0.48.0", gemini: "0.12.0", opencode: "0.6.1",
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
  makeMessage({ thread_id: "th_release", from: "pm-codex", to: "glenn", type: "reply", body: "release/v0.16.8 分支检查全部通过，等你确认后打 tag。", created_at: iso(600), updated_at: iso(600) }),
  makeMessage({ thread_id: "th_release", from: "glenn", to: "pm-codex", type: "task", body: "可以，打 tag 吧。", created_at: iso(300), updated_at: iso(300) }),
];
// MOCK_E2E: the console conversation is end-to-end encrypted, one message
// in it was sealed for someone else.
const studioThread = process.env.MOCK_E2E
  ? studioThreadRaw.map((m, i) => i === 1 ? { ...m, e2e: true, unreadable: "not for this device", body: "[encrypted message]" } : { ...m, e2e: true })
  : studioThreadRaw;

// The App talks as "@console:<device>"; a thread counts as a conversation
// only when it has one of these messages.
const CONSOLE = `@console:${"glenn-mac"}`;

const consoleThreads = {
  th_console_release: [
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
    joined: { agent: "glenn-mac-claude-x9k2", hub: "http://192.168.1.20:7780" },
  },
  {
    cli: "codex", session_id: "019f0f11-3c2b-7a10-8d4e-9f8a7b6c5d4e",
    workdir: "/Users/glenn/code/notes", last_active: iso(7200),
    joined: { agent: "glenn-mac-codex-ab12", hub: "https://other-hub.example.com" },
  },
];


const state = {
  hubs: [
    {
      id: "hub_studio",
      e2e: !!process.env.MOCK_E2E,
      name: "Glenn 的 MacBook Pro 本机 Hub",
      url: "http://192.168.1.20:7780",
      token: "myco_console_demo_studio",
      ok: true,
      hub_version: "0.16.6",
      update_available: true,
      threads: { th_release: studioThread, ...consoleThreads },
      machines: [
        {
          id: "m_studio", name: "Studio Mac", hostname: "studio.local", os: "darwin", arch: "arm64",
          myco_version: "0.16.8", online: true, last_seen_at: iso(5), can_spawn: true,
        },
        {
          id: "m_bench", name: "Bench Linux", hostname: "bench-01", os: "linux", arch: "x86_64",
          myco_version: "0.9.4", online: true, last_seen_at: iso(30), can_spawn: false,
        },
        {
          id: "m_office", name: "Office PC", hostname: "office-pc", os: "windows", arch: "amd64",
          myco_version: "0.16.6", online: true, last_seen_at: iso(90), can_spawn: true,
        },
      ],
      agents: [
        makeAgent({
          name: "pm-codex", owner: "glenn", owner_id: "mem_glenn", display_name: "PM (Codex)", description: "规划发布并审查 PR。",
          kind: "agent", machine_id: "m_studio",
          runtime: { cli: "codex", cli_version: "0.52.0", model: "gpt-6-sol", model_source: "configured", access: "workspace" },
          status_text: "正在审查 multihub 分支", status_at: iso(180),
          labels: ["machine=studio-mac", "group=官网改版", "role=pm", "review", "release"], capabilities: ["review", "release"],
          skills: [
            { name: "review", summary: "审查 Go 和 TypeScript 的改动，指出问题和风险", examples: ["帮我看下 PR 128", "这个分支能合吗"] },
            { name: "release", summary: "写发布说明、打 tag、发版" },
          ],
          binding: { harness: "codex", environment: "macOS 26.1", workdir: "~/code/myco" }, session_in_use: true,
          aliases: ["pm"], allow_from: ["glenn"], enrolled_by: "console",
        }),
        makeAgent({
          name: "docs-claude", owner: "laohu", owner_id: "mem_laohu", description: "保持 README 和 docs/ 与代码同步。", visibility: "private",
          kind: "agent", machine_id: "m_studio",
          runtime: { cli: "claude", cli_version: "2.1.240", model: "claude-opus-5-5", model_source: "observed", access: "readonly", provider: "DeepSeek", provider_source: "machine" },
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
          kind: "agent", runtime: { cli: "claude-cloud", model: "claude-opus-5-5", model_source: "default", access: "readonly" },
          status_text: "正在整理发布说明", status_at: iso(60),
          labels: ["group=release"], binding: { harness: "claude-cloud", environment: "claude.ai" },
        }),
        makeAgent({
          name: "review-cloud", owner: "matthew", owner_id: "mem_matthew", display_name: "Review (Codex 云端)", description: "在 Codex 云端做代码审查。",
          kind: "agent", runtime: { cli: "codex-cloud", model: "gpt-6-sol", model_source: "observed", access: "readonly" },
          labels: ["group=release"], binding: { harness: "codex-cloud", environment: "Codex 云端" },
        }),
        makeAgent({
          name: "legacy-codex", owner: "laohu", owner_id: "mem_laohu", description: "旧版 runner，还没有上报运行信息。",
          kind: "agent", machine_id: "m_bench",
          labels: ["machine=bench-01"], online: true, status_at: iso(20),
          binding: { harness: "codex", environment: "Linux 6.8", workdir: "~/code/legacy" },
        }),
        makeAgent({
          name: "studio-mac-machine", display_name: "Studio Mac", kind: "machine", machine_id: "m_studio",
          labels: ["machine=studio-mac"], binding: { harness: "machine" },
        }),
        makeAgent({
          name: "bench-01-machine", display_name: "Bench Linux", kind: "machine", machine_id: "m_bench",
          labels: ["machine=bench-01"], binding: { harness: "machine" },
        }),
      ],
      recent: [
        makeMessage({ thread_id: "th_release", from: "glenn", to: "pm-codex", type: "task", body: "可以，打 tag 吧。", created_at: iso(300), updated_at: iso(300) }),
        makeMessage({ thread_id: "th_release", from: "pm-codex", to: "glenn", type: "reply", body: "release/v0.16.8 分支检查全部通过，等你确认后打 tag。", created_at: iso(600), updated_at: iso(600) }),
        makeMessage({ thread_id: "th_ios", from: "glenn", to: "ios-shell", type: "task", body: "请跑一遍 iOS 测试套件，附上日志。", created_at: iso(240), updated_at: iso(240) }),
        makeMessage({ thread_id: "th_console_release", from: "pm-codex", to: CONSOLE, type: "reply", status: "done", body: "v0.16.8 发布说明已整理好。", created_at: iso(6800), updated_at: iso(6800) }),
      ],
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
      body: "处理好了。改动集中在 console 的 session 读取和 App 的对话页面，类型检查和构建都已经通过。\n\n需要我继续把发布说明也写好吗？",
    };
  }
  return {
    body: "跑完了，日志在附件里。最后一行是 BUILD SUCCESSFUL。",
    attachments: [{ file_id: `f_demo_${replySeq}`, name: "build.log", size: 20480, sha256: "demo", content_type: "text/plain" }],
  };
}

// A task gets a progress note after 3s and a reply after 6s.
function scheduleReply(hub, threadId, agent, taskId) {
  const later = (ms, fn) => { const t = setTimeout(fn, ms); if (typeof t.unref === "function") t.unref(); };
  later(3000, () => {
    const msgs = hub.threads[threadId];
    if (!msgs) return;
    msgs.push(makeMessage({
      thread_id: threadId, from: agent, to: CONSOLE, type: "progress", status: "done",
      body: "正在处理…", created_at: iso(0), updated_at: iso(0),
    }));
  });
  later(6000, () => {
    const msgs = hub.threads[threadId];
    if (!msgs) return;
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
    }
  });
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

const local = {
  hostname: "Studio.local",
  myco: { path: "/Applications/myco.app/Contents/MacOS/myco", version: "0.16.9" },
  hub: null,
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

function totals(agents) {
  const t = { agents: agents.length, working: 0, pending: 0, idle: 0, offline: 0 };
  for (const a of agents) {
    const s = a.online ? (a.status_text ? "working" : "idle") : "offline";
    t[s] += 1;
  }
  return t;
}

// A node's demo history with the exec node mac-exec: six commands, one thread each.
const execHistories = {};
function execHistory(agent) {
  if (execHistories[agent]) return execHistories[agent];
  const out = [];
  const cmds = ["uptime", "df -h /", "git -C ~/code/myco pull", "make check", "myco upgrade", "myco --version"];
  const outs = ["up 3 days, 4:12, 2 users, load averages: 1.92 2.10 2.31\n", "Filesystem   Size  Used Avail Capacity\n/dev/disk3s1 926Gi 412Gi 498Gi  46%\n", "Already up to date.\n", "ok  github.com/chenhg5/myco/internal/app 1.2s\nsmoke: all steps passed\n", "", "v0.16.7\n"];
  cmds.forEach((c, i) => {
    const t = 6 * 3600 - i * 3000;
    const th = `th_exec_${agent}_${i}`;
    const timedOut = i === cmds.length - 1;
    const task = makeMessage({ thread_id: th, from: agent, to: "mac-exec", type: "task", status: "done", body: c, created_at: iso(t), updated_at: iso(t) });
    out.push(task);
    out.push(makeMessage({ thread_id: th, reply_to: task.id, from: "mac-exec", to: agent, type: "reply", status: "done",
      body: JSON.stringify(i === 4
        ? { exit_code: 1, stdout: "", stderr: "download failed: 504 Gateway Timeout\n", duration_ms: 12400, timed_out: false }
        : { exit_code: timedOut ? 124 : 0, stdout: outs[i], stderr: "", duration_ms: timedOut ? 60000 : 820, timed_out: timedOut, error: timedOut ? "killed after 1m0s (the node's timeout)" : "" }),
      created_at: iso(t - 30), updated_at: iso(t - 30) }));
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
  const state = a.online ? (a.status_text ? "working" : "idle") : "offline";
  // Working nodes also have a stale queued answer waiting (dismissable).
  const staleId = "msg_stale_" + a.name;
  const stale = state === "working" && !dismissed.has(staleId)
    ? [makeMessage({ id: staleId, thread_id: "th_stale_" + a.name, from: "mac-exec", to: a.name, type: "reply", status: "queued",
        body: '{"exit_code":124,"stdout":"v0.16.7\\n","stderr":"","duration_ms":60028,"timed_out":true,"error":"killed after 1m0s"}',
        created_at: iso(12 * 3600), updated_at: iso(12 * 3600) })]
    : [];
  return {
    agent: a, state,
    queued: a.queued ?? (state === "working" ? 1 + stale.length : 0),
    in_progress: state === "working" ? 1 : 0,
    held: 0,
    receiving: a.receiving ?? (!a.online || a.kind === "human" ? undefined : /cloud/.test(a.binding?.harness || "") ? "session" : "runner"),
    session_in_use: a.session_in_use || undefined,
    current: state === "working"
      ? [makeMessage({ thread_id: a.current_thread || "th_release", from: a.current_from || "glenn", to: a.name, body: a.status_text || "处理中…" }), ...stale]
      : [],
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
  if (h.id !== "hub_studio") return undefined;
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
    ],
  },
  {
    name: "ios", title: "iOS 客户端", goal: "", brief: "", links: [],
    visibility: "team", owner: "matthew", owner_id: "mem_matthew", archived: false, created_at: iso(86400 * 2), updated_at: iso(86400),
    members: [{ name: "ios-shell", role: "member", note: "", added_by: "matthew", added_at: iso(86400) }], docs: [],
  },
];
function projectView(hub, p, bodies) {
  return {
    ...p,
    members: p.members.map((m) => ({ ...m, agent: hub.agents.find((a) => a.name === m.name && a.visibility !== "private") })),
    docs: p.docs.map((d) => ({ name: d.name, size: d.size ?? d.body.length, content_type: d.content_type, binary: !!d.binary, updated_by: d.updated_by, updated_at: d.updated_at, ...(bodies && !d.binary ? { body: d.body } : {}) })),
    can_edit: true, can_manage: p.owner_id === "mem_" + meName || meName === "glenn",
  };
}

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

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || "/", `http://${req.headers.host || "localhost"}`);
  const parts = url.pathname.split("/").filter(Boolean).map(decodeURIComponent);
  const method = req.method || "GET";
  console.error(`${method} ${url.pathname}`);

  if (method === "OPTIONS") { res.writeHead(204); res.end(); return; }

  // /app/pending-link — a link the App was opened with, read once.
  if (parts[0] === "app" && parts[1] === "pending-link" && parts.length === 2) {
    if (method !== "GET") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    const link = pendingLink;
    pendingLink = "";
    return send(res, 200, { link });
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

  // /app/info
  if (parts[0] === "app" && parts[1] === "info" && parts.length === 2) {
    if (method !== "GET") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    return send(res, 200, { version: SHOWCASE ? "v0.17.2" : "v0.16.8-demo", shell: SHELL });
  }

  // /app/update: the app's own update (MOCK_UPDATE=none for "up to date")
  if (parts[0] === "app" && parts[1] === "update" && parts.length === 2) {
    const current = SHOWCASE ? "v0.17.2" : "v0.16.8-demo";
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
        return fail(res, 409, "cloudflared_missing", "没有找到 cloudflared。", "在终端运行 brew install cloudflared。");
      }
      if (tunnel === "tailscale" && !local.tools.tailscale) {
        return fail(res, 409, "tailscale_missing", "没有找到 Tailscale。", "先安装 Tailscale 并为这台设备开启 Funnel。");
      }
      if (tunnel === "cloudflare" && hostname && !/^[a-z0-9.-]+$/i.test(hostname)) {
        return fail(res, 400, "invalid_hostname", "域名看起来不对。", "例如 hub.example.com。");
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

      local.hub = {
        running: true,
        local_url: "http://127.0.0.1:7780",
        ...(publicUrl ? { public_url: publicUrl } : {}),
        version: "0.16.8",
        added: true,
      };

      const appUrl = publicUrl || local.hub.local_url;
      let appHub = state.hubs.find((h) => h.id === "hub_local");
      if (!appHub) {
        appHub = {
          id: "hub_local", name: `${local.device} 本机`, url: appUrl,
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
        public_url: publicUrl || local.hub.local_url,
        ...(publicUrl ? { join_url: `${publicUrl}/j/myco_enroll_k3v9q2wd7hx4m8pa` } : {}),
        warnings,
      });
    }

    if (parts.length === 4 && parts[2] === "hub" && parts[3] === "stop" && method === "POST") {
      if (local.hub) local.hub.running = false;
      const appHub = state.hubs.find((h) => h.id === "hub_local");
      if (appHub) {
        appHub.ok = false;
        appHub.error = "本机 Hub 已停止";
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
      if (hub.id === "hub_local" && local.hub) local.hub.added = false;
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
      hub.agents.push(agent);
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
      const threadId = typeof body.thread_id === "string" && body.thread_id.trim()
        ? body.thread_id.trim()
        : `th_c_${state.counter += 1}`;
      const msg = makeMessage({
        thread_id: threadId, from: CONSOLE, to, type: "task", status: "queued",
        body: text, created_at: iso(0), updated_at: iso(0),
        ...(body.project && !body.thread_id ? { project: body.project } : {}),
      });
      if (!hub.threads) hub.threads = {};
      if (!hub.threads[threadId]) hub.threads[threadId] = [];
      hub.threads[threadId].push(msg);
      hub.recent = [msg, ...(hub.recent || [])].slice(0, 20);
      scheduleReply(hub, threadId, to, msg.id);
      return send(res, 201, msg);
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
        return fail(res, 409, "task_finished", "这个任务已经结束了。");
      }
      const steer = makeMessage({
        thread_id: task.thread_id, from: CONSOLE, to: task.to, type: "steer", status: "done",
        body: text, reply_to: task.id, created_at: iso(0), updated_at: iso(0),
      });
      msgs.push(steer);
      task.status = "delivered";
      task.updated_at = iso(0);
      scheduleReply(hub, task.thread_id, task.to, task.id);
      return send(res, 201, steer);
    }

    // /app/hubs/{id}/messages/{mid}/dismiss — close an open message unanswered.
    if (parts.length === 6 && parts[3] === "messages" && parts[5] === "dismiss" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      dismissed.add(parts[4]);
      return send(res, 200, makeMessage({ id: parts[4], status: "done" }));
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
        if (open) state = open.status === "queued" ? "queued" : "working";
        else if (task.status === "failed" || answer?.type === "event") state = "failed";
        else if (node && answer) { try { if (JSON.parse(answer.body).exit_code) state = "failed"; } catch { /* not json */ } }
        return { state, task, answer };
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
            byPair.set(key, { ...it, kind: "pair", other, node, participants: [other, node], messages: (prev?.messages || 0) + it.messages, first: prev?.first || it.first });
          } else prev.messages += it.messages;
        }
        items.splice(0, items.length, ...byPair.values());
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
      if (q.get("a") !== "mac-exec" && q.get("b") !== "mac-exec") {
        // Two parties: every message between them, all threads, newest first.
        const pair = [q.get("a"), q.get("b")];
        const all = Object.values(hub.threads || {}).flat()
          .filter((m) => pair.includes(m.from) && pair.includes(m.to))
          .filter((m) => !before || new Date(m.created_at).getTime() < before)
          .sort((x, y) => new Date(y.created_at) - new Date(x.created_at));
        return send(res, 200, { messages: all.slice(0, 50), more: all.length > 50 });
      }
      const all = execHistory(a).slice().reverse().filter((m) => !before || new Date(m.created_at).getTime() < before);
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
      return send(res, 200, { messages: hub.threads[parts[4]] || [] });
    }

    // /app/hubs/{id}/links — a one-off enrollment link for adding a machine.
    if (parts.length === 4 && parts[3] === "links" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const body = await readBody(req);
      const uses = Number.isInteger(body?.uses) ? body.uses : 1;
      const hours = Number.isInteger(body?.expires_hours) ? body.expires_hours : 24;
      state.counter += 1;
      const joinUrl = `${hub.url.replace(/\/+$/, "")}/j/myco_enroll_demo${state.counter}x7k2m9q4w8`;
      return send(res, 200, {
        join_url: joinUrl,
        command: `curl -fsSL ${joinUrl} | bash`,
        uses,
        expires_hours: hours,
      });
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
        machine.name = name;
        const agents = hub.agents.filter((a) => a.kind !== "machine" && a.machine_id === machine.id).length;
        return send(res, 200, { ...machine, agents });
      }
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
        if (machine.id !== "m_studio") { // only Studio Mac allows full access in the mock
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
      const agent = makeAgent({
        name,
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

    if (parts.length >= 5 && parts[3] === "agents") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const agent = hub.agents.find((a) => a.name === parts[4]);
      if (!agent) return fail(res, 404, "not_found", `没有名为 ${parts[4]} 的节点`);

      if (parts.length === 5 && method === "DELETE") {
        hub.agents = hub.agents.filter((a) => a.name !== agent.name);
        hub.recent = hub.recent.filter((m) => m.from !== agent.name && m.to !== agent.name);
        return send(res, 204);
      }
      if (parts.length === 6 && parts[5] === "profile" && method === "PUT") {
        const body = await readBody(req);
        if (body?.display_name !== undefined) agent.display_name = body.display_name || undefined;
        if (body?.description !== undefined) agent.description = body.description;
        return send(res, 200, agent);
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
