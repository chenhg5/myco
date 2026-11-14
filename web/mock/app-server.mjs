// Dev-only stand-in for the App backend (cmd/myco-app). No dependencies.
//   node mock/app-server.mjs        (or: npm run dev:app)
// Serves the `app/...` contract with two hubs: one healthy, one unreachable.
// Writes mutate in-memory state and are lost on restart.
import http from "node:http";
import zlib from "node:zlib";
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

// A machine the smoke test deletes, with two nodes and its own machine node.
// POST /app/mock/machine puts them back between widths and themes.
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
  th_console_stuck: [stuckTask],
  // An older task stuck behind a newer one the sender stopped: it still has
  // its own stop and resend in the chat.
  th_console_stuck_stopped: [stuckOlderTask, stoppedAfterTask],
  // An inbox node pulled this task but never acknowledged it: still queued.
  th_grok_pull: [grokPulledTask],
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
      hub_version: "0.19.5",
      update_available: true,
      threads: { th_release: studioThread, th_coord: coordThread, th_dispatch_docs: dispatchDocs, th_dispatch_release: dispatchRelease, ...consoleThreads },
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
        deletableMachine(),
      ],
      agents: [
        makeAgent({
          name: "pm-codex", owner: "glenn", owner_id: "mem_glenn", display_name: "PM (Codex)", description: "规划发布并审查 PR。",
          kind: "agent", machine_id: "m_studio",
          // Its 5-hour window is nearly used (amber), the weekly one is not.
          runtime: {
            cli: "codex", cli_version: "0.52.0", model: "gpt-6-sol", model_source: "app",
            effort: "high", effort_source: "app", access: "workspace", max_access: "workspace",
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
          name: "docs-claude", owner: "laohu", owner_id: "mem_laohu", description: "保持 README 和 docs/ 与代码同步。", visibility: "private",
          kind: "agent", machine_id: "m_studio",
          runtime: {
            cli: "claude", cli_version: "2.1.240", model: "claude-opus-5-5", model_source: "observed",
            effort: "low", effort_source: "app", access: "readonly", max_access: "workspace", provider: "DeepSeek", provider_source: "machine",
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
          kind: "agent", runtime: { cli: "claude-cloud", model: "claude-opus-5-5", model_source: "default", access: "readonly" },
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
          labels: ["machine=studio-mac"], binding: { harness: "machine" },
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
    body: "跑完了，截图和日志在附件里。最后一行是 BUILD SUCCESSFUL。",
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
  const asking = askingBy.get(a.name);
  if (asking) {
    return { agent: a, state: "pending", queued: 0, in_progress: 0, held: 1, asking: 1, receiving: "runner", current: [asking] };
  }
  return {
    agent: a, state,
    queued: (a.queued ?? (working ? 1 + stale.length : 0)) + delivered.length,
    in_progress: working ? 1 : 0,
    held: 0,
    receiving: a.receiving ?? (!a.online || a.kind === "human" ? undefined : /cloud/.test(a.binding?.harness || "") ? "session" : "runner"),
    session_in_use: a.session_in_use || undefined,
    current: atWork.length || delivered.length ? [...atWork, ...delivered, ...stale] : [],
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

  // /app/mock/stuck — the smoke test puts the stale demo task back: it stops
  // and resends it, and the mock keeps running across widths and themes.
  if (parts[0] === "app" && parts[1] === "mock" && parts[2] === "stuck" && parts.length === 3) {
    if (method !== "POST") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    localSessions.forEach((x, k) => { if (localJoinedAtStart[k]) x.joined = localJoinedAtStart[k]; else delete x.joined; });
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
    hub.threads.th_console_stuck = keep("th_console_stuck", [stuckTask]);
    stuckOlderTask.status = "acked";
    delete stuckOlderTask.error;
    stuckOlderTask.created_at = iso(19 * 60 * 60);
    stuckOlderTask.updated_at = iso(19 * 60 * 60);
    stoppedAfterTask.status = "failed";
    stoppedAfterTask.error = "stopped by the sender";
    stoppedAfterTask.created_at = iso(2 * 60 * 60);
    stoppedAfterTask.updated_at = iso(2 * 60 * 60);
    hub.threads.th_console_stuck_stopped = keep("th_console_stuck_stopped", [stuckOlderTask, stoppedAfterTask]);
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
  // /app/info
  if (parts[0] === "app" && parts[1] === "info" && parts.length === 2) {
    if (method !== "GET") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    return send(res, 200, { version: SHOWCASE ? "v0.18.39" : "v0.16.8-demo", shell: SHELL, dictation: SHELL === "mac" });
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

    // /app/hubs/{id}/messages/{mid}/stop — stop an unfinished task.
    if (parts.length === 6 && parts[3] === "messages" && parts[5] === "stop" && method === "POST") {
      // A command one node sent another may not be in a thread the App reads
      // (the terminal lists it from the recipient's current work).
      let task = Object.values(hub.threads || {}).flat().find((m) => m.id === parts[4]);
      if (!task) task = [...stuckCurrent.values()].find((m) => m.id === parts[4]);
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
      const events = shown.map((d, i) => ({ seq: i + 1, kind: d.kind, data: d })).filter((e) => e.seq > after);
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
        agent_guide: true,
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
      if (method === "DELETE") {
        const dry = url.searchParams.get("dry_run") === "true" || url.searchParams.get("dry_run") === "1";
        // The nodes shown to the user; the machine's own node goes too, but it
        // is not a node anyone can see in the list.
        const removed = hub.agents
          .filter((a) => a.machine_id === machine.id && a.kind !== "machine")
          .map((a) => a.name);
        if (!dry) {
          await sleep(400);
          hub.agents = hub.agents.filter((a) => a.machine_id !== machine.id);
          hub.machines = (hub.machines || []).filter((m) => m.id !== machine.id);
        }
        return send(res, 200, { machine, removed, dry_run: dry });
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
      const agent = makeAgent({ ...(typeof body.display_name === "string" && body.display_name ? { display_name: body.display_name } : {}),
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
        agent.run_settings = {};
        if (access) agent.run_settings.access = access;
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
