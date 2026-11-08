// A polished demo dataset for README screenshots and videos, in English or
// Chinese:  MOCK_SHOWCASE=en node mock/app-server.mjs   (or zh)
// One team building a website with agents on a MacBook, a build server, a
// Windows desktop and in the cloud, on two hubs.

export function showcase(lang, { makeAgent, makeMessage, iso, CONSOLE }) {
  const T = (zh, en) => (lang === "zh" ? zh : en);
  const group = (zh, en) => "group=" + T(zh, en);
  const site = group("官网改版", "website");
  const research = group("调研", "research");
  const infra = group("运维", "infra");

  const machines = [
    { id: "m_mbp", name: "MacBook Pro", hostname: "glenns-mbp.local", os: "darwin", arch: "arm64", myco_version: "0.17.2", online: true, last_seen_at: iso(4), can_spawn: true },
    { id: "m_build", name: T("构建服务器", "Build server"), hostname: "build-01", os: "linux", arch: "x86_64", myco_version: "0.17.2", online: true, last_seen_at: iso(9), can_spawn: true },
    { id: "m_win", name: T("Windows 台式机", "Windows desktop"), hostname: "desk-win", os: "windows", arch: "amd64", myco_version: "0.17.2", online: false, last_seen_at: iso(3 * 3600), can_spawn: false },
  ];

  const agents = [
    makeAgent({
      name: "mbp-frontend", display_name: T("前端（Claude Code）", "Frontend (Claude Code)"),
      description: T("负责官网的页面和组件。", "Builds the website's pages and components."),
      machine_id: "m_mbp", labels: ["machine=mbp", site, "ui"],
      runtime: { cli: "claude", cli_version: "2.1.240", model: "claude-opus-5-5", model_source: "configured", access: "workspace" },
      status_text: T("好的，在套餐下面加 FAQ。", "Yes, add an FAQ below the plans."), status_at: iso(40),
      binding: { harness: "claude", environment: "macOS 26.1", workdir: "~/code/website" },
      allow_from: ["build-backend", "mbp-reviewer"], last_seen_at: iso(3),
      queued: 1, current_from: CONSOLE, current_thread: "th_pricing",
    }),
    makeAgent({
      name: "build-backend", display_name: T("后端（Codex）", "Backend (Codex)"),
      description: T("负责 API 和数据库。", "Owns the API and the database."),
      machine_id: "m_build", labels: ["machine=build-01", site, "api"],
      runtime: { cli: "codex", cli_version: "0.52.0", model: "gpt-6-sol", model_source: "configured", access: "workspace" },
      status_text: T("正在写 /api/plans", "Writing /api/plans"), status_at: iso(70),
      binding: { harness: "codex", environment: "Ubuntu 24.04", workdir: "~/srv/website-api" }, last_seen_at: iso(6),
      queued: 0, current_from: CONSOLE, current_thread: "th_api",
    }),
    makeAgent({
      name: "mbp-reviewer", display_name: T("代码审查（Codex）", "Reviewer (Codex)"),
      description: T("审查每个 PR。", "Reviews every pull request."),
      machine_id: "m_mbp", labels: ["machine=mbp", site, "review"],
      runtime: { cli: "codex", cli_version: "0.52.0", model: "gpt-6-sol", model_source: "cli-config", access: "readonly" },
      binding: { harness: "codex", environment: "macOS 26.1", workdir: "~/code/website" }, last_seen_at: iso(8),
    }),
    makeAgent({
      name: "cloud-research", display_name: T("调研（claude.ai）", "Research (claude.ai)"),
      description: T("在云端做调研和对比。", "Researches and compares options in the cloud."),
      labels: [research],
      runtime: { cli: "claude-cloud", model: "claude-opus-5-5", model_source: "observed" },
      binding: { harness: "claude-cloud", environment: "claude.ai" }, last_seen_at: iso(15),
    }),
    makeAgent({
      name: "build-shell", display_name: T("服务器 shell", "Server shell"), kind: "shell",
      description: T("在构建服务器上执行命令。", "Runs commands on the build server."),
      machine_id: "m_build", labels: ["machine=build-01", infra],
      runtime: { cli: "exec", access: "workspace" },
      binding: { harness: "exec", environment: "Ubuntu 24.04", workdir: "~" }, allow_from: ["mbp-frontend", "build-backend"], last_seen_at: iso(5),
    }),
    makeAgent({
      name: "desk-docs", display_name: T("文档（Claude Code）", "Docs (Claude Code)"),
      description: T("维护用户文档。", "Keeps the user docs up to date."),
      machine_id: "m_win", labels: ["machine=desk-win", site, "docs"],
      runtime: { cli: "claude", cli_version: "2.1.236", model: "claude-sonnet-5-5", model_source: "configured", access: "workspace" },
      binding: { harness: "claude", environment: "Windows 11", workdir: "C:\\code\\website-docs" },
      online: false, last_seen_at: iso(3 * 3600), status_at: iso(3 * 3600),
    }),
    makeAgent({ name: "mbp-machine", display_name: "MacBook Pro", kind: "machine", machine_id: "m_mbp", labels: ["machine=mbp"], binding: { harness: "machine" } }),
    makeAgent({ name: "build-01-machine", display_name: T("构建服务器", "Build server"), kind: "machine", machine_id: "m_build", labels: ["machine=build-01"], binding: { harness: "machine" } }),
  ];

  const pricingReply = T(
    [
      "定价页做好了，预览：`npm run dev` → `/pricing`。",
      "",
      "| 文件 | 改动 |",
      "|---|---|",
      "| `app/pricing/page.tsx` | 新页面：三档套餐，月付 / 年付切换 |",
      "| `components/PlanCard.tsx` | 套餐卡片 |",
      "| `lib/plans.ts` | 从 `/api/plans` 读取（后端今天交付）|",
      "",
      "`npm run build` 和 `npm test` 都已通过。要我顺便加上 FAQ 吗？",
    ].join("\n"),
    [
      "The pricing page is ready. Preview: `npm run dev` → `/pricing`.",
      "",
      "| File | Change |",
      "|---|---|",
      "| `app/pricing/page.tsx` | New page: three plans, monthly / yearly toggle |",
      "| `components/PlanCard.tsx` | Plan card |",
      "| `lib/plans.ts` | Reads `/api/plans` (backend ships it today) |",
      "",
      "`npm run build` and `npm test` both pass. Want me to add an FAQ too?",
    ].join("\n"),
  );

  const threads = {
    th_pricing: [
      makeMessage({ thread_id: "th_pricing", from: CONSOLE, to: "mbp-frontend", type: "task", status: "done", body: T("按设计稿做新的定价页，三档套餐，支持月付 / 年付切换。", "Build the new pricing page from the design: three plans, with a monthly / yearly toggle."), created_at: iso(2400), updated_at: iso(2400) }),
      makeMessage({ thread_id: "th_pricing", from: "mbp-frontend", to: CONSOLE, type: "progress", status: "done", body: T("读完设计稿，先搭页面骨架…", "Read the design; laying out the page…"), created_at: iso(2200), updated_at: iso(2200) }),
      makeMessage({ thread_id: "th_pricing", from: "mbp-frontend", to: CONSOLE, type: "reply", status: "done", body: pricingReply, created_at: iso(1500), updated_at: iso(1500) }),
      makeMessage({ thread_id: "th_pricing", from: CONSOLE, to: "mbp-frontend", type: "task", status: "delivered", body: T("好的，在套餐下面加 FAQ。", "Yes, add an FAQ below the plans."), created_at: iso(60), updated_at: iso(40) }),
      makeMessage({ thread_id: "th_pricing", from: "mbp-frontend", to: CONSOLE, type: "progress", status: "done", body: T("在写 FAQ，先列 6 个常见问题…", "Writing the FAQ: six common questions first…"), created_at: iso(30), updated_at: iso(30) }),
    ],
    th_api: [
      makeMessage({ thread_id: "th_api", from: CONSOLE, to: "build-backend", type: "task", status: "delivered", body: T("加一个 /api/plans 接口，返回三档套餐和价格，写好测试。", "Add a /api/plans endpoint that returns the three plans and prices, with tests."), created_at: iso(600), updated_at: iso(70) }),
      makeMessage({ thread_id: "th_api", from: "build-backend", to: CONSOLE, type: "progress", status: "done", body: T("接口写好了，正在补测试（3/5）…", "Handler done; writing tests (3/5)…"), created_at: iso(70), updated_at: iso(70) }),
    ],
    th_review: [
      makeMessage({ thread_id: "th_review", from: CONSOLE, to: "mbp-reviewer", type: "task", status: "done", body: T("审查 PR #128（定价页）。", "Review PR #128 (pricing page)."), created_at: iso(1300), updated_at: iso(1300) }),
      makeMessage({
        thread_id: "th_review", from: "mbp-reviewer", to: CONSOLE, type: "reply", status: "done", created_at: iso(900), updated_at: iso(900),
        body: T(
          "可以合并，有两处小建议：\n\n1. `PlanCard` 的价格用 `Intl.NumberFormat` 格式化，别手拼货币符号。\n2. 年付折扣写在 `lib/plans.ts` 里，不要散在组件中。\n\n没有发现阻塞问题。",
          "Good to merge, with two small suggestions:\n\n1. Format prices in `PlanCard` with `Intl.NumberFormat` instead of concatenating the currency symbol.\n2. Keep the yearly discount in `lib/plans.ts`, not inside components.\n\nNo blocking issues found.",
        ),
      }),
    ],
    th_research: [
      makeMessage({ thread_id: "th_research", from: CONSOLE, to: "cloud-research", type: "task", status: "done", body: T("对比三家支付服务在欧洲的费率。", "Compare the fees of three payment providers in Europe."), created_at: iso(5400), updated_at: iso(5400) }),
      makeMessage({
        thread_id: "th_research", from: "cloud-research", to: CONSOLE, type: "reply", status: "done", created_at: iso(4800), updated_at: iso(4800),
        body: T(
          "| 服务商 | 欧洲卡费率 | 月费 |\n|---|---|---|\n| A | 1.5% + €0.25 | 无 |\n| B | 1.4% + €0.25 | 无 |\n| C | 1.2% + €0.30 | €25 |\n\n月流水低于 €8,000 时 B 最便宜。",
          "| Provider | EU card fee | Monthly |\n|---|---|---|\n| A | 1.5% + €0.25 | none |\n| B | 1.4% + €0.25 | none |\n| C | 1.2% + €0.30 | €25 |\n\nBelow €8,000 a month, B is the cheapest.",
        ),
      }),
    ],
    // Two nodes working together (read-only in the app).
    th_handoff: [
      makeMessage({ thread_id: "th_handoff", from: "mbp-frontend", to: "build-backend", type: "task", status: "done", body: T("定价页需要 /api/plans：返回 id、名称、月价、年价。", "The pricing page needs /api/plans: id, name, monthly and yearly price."), created_at: iso(2000), updated_at: iso(2000) }),
      makeMessage({ thread_id: "th_handoff", from: "build-backend", to: "mbp-frontend", type: "reply", status: "done", body: T("收到，今天下午部署。", "Got it, deploying this afternoon."), created_at: iso(1900), updated_at: iso(1900) }),
    ],
  };
  const recent = Object.values(threads).flat().sort((a, b) => b.updated_at.localeCompare(a.updated_at));

  const team = [
    makeAgent({
      name: "vps-pipeline", display_name: T("数据管道（Codex）", "Data pipeline (Codex)"),
      labels: [group("数据", "data")], runtime: { cli: "codex", cli_version: "0.52.0", model: "gpt-6-sol", model_source: "configured", access: "workspace" },
      binding: { harness: "codex", environment: "Debian 13", workdir: "~/pipeline" }, last_seen_at: iso(12),
    }),
    makeAgent({
      name: "vps-oncall", display_name: T("值班（Claude Code）", "On-call (Claude Code)"),
      labels: [infra], runtime: { cli: "claude", cli_version: "2.1.240", model: "claude-opus-5-5", model_source: "configured", access: "readonly" },
      status_text: T("正在看告警", "Looking at an alert"), status_at: iso(30),
      binding: { harness: "claude", environment: "Debian 13", workdir: "~/ops" }, last_seen_at: iso(4),
      queued: 0,
    }),
  ];

  const hubs = [
    {
      id: "hub_home", name: T("本机 Hub", "Home hub"), url: "http://192.168.1.20:7780",
      token: "myco_console_demo_home", ok: true, hub_version: "0.17.2", update_available: false,
      threads, machines, agents, recent,
    },
    {
      id: "hub_team", name: T("团队 VPS", "Team VPS"), url: "https://hub.example.com",
      token: "myco_console_demo_team", ok: true, hub_version: "0.17.2", update_available: false,
      threads: {}, machines: [{ id: "m_vps", name: "vps-1", hostname: "vps-1", os: "linux", arch: "x86_64", myco_version: "0.17.2", online: true, last_seen_at: iso(6), can_spawn: true }],
      agents: team.map((a) => ({ ...a, machine_id: "m_vps" })), recent: [],
    },
  ];

  const localSessions = [
    { cli: "claude", session_id: "7c1d9f4e-2a6b-4c18-9e0d-4a1f2b3c5d6e", workdir: "/Users/glenn/code/website", last_active: iso(300), title: T("把首页的 hero 区改成新设计", "Redo the home page hero from the new design") },
    { cli: "codex", session_id: "019f20c1-a446-7812-bbc8-ce4ac3b889f6", workdir: "/Users/glenn/code/mobile", last_active: iso(1800), title: T("修复 iOS 上登录页的布局", "Fix the login screen layout on iOS") },
    { cli: "claude", session_id: "9b8a7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d", workdir: "/Users/glenn/code/infra", last_active: iso(7200), title: T("检查 Hub 的隧道配置", "Check the hub's tunnel settings"), joined: { agent: "mbp-infra", hub: "http://192.168.1.20:7780" } },
  ];

  return { hubs, localSessions };
}
