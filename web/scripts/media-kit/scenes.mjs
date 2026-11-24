import { interactionScenes } from "./interactions.mjs";
import { coedit, design, motion, disk, servers } from "./apps.mjs";
const click = async (x) => {
  await x.first().click();
};
const nav = async (page, sel) => {
  await click(page.locator(sel + ":visible"));
  await page.waitForTimeout(650);
};
const member = async (page, T) => {
  await click(
    page.locator(".sp-side .sp-member:visible", {
      hasText: T("后端（Codex）", "Backend (Codex)"),
    }),
  );
  await page.locator(".conv-detail").last().waitFor();
};
const home = async () => {};
export const scenes = [
  {
    id: "teamwork",
    title: "多 Agent 项目协作 / Multi-agent teamwork",
    zh: "把不同电脑上的 AI 组成团队，在一个项目里一起推进工作。",
    en: "Bring AI agents across computers together to work on one project.",
    tags: ["协作"],
    prepare: home,
  },
  {
    id: "progress",
    title: "任务过程实时可见 / Live task progress",
    zh: "不用反复追问，直接看到 AI 做到了哪一步。",
    en: "See what your agent is doing without asking for updates.",
    tags: ["可见性"],
    prepare: async ({ page, T }) => {
      await member(page, T);
      await page
        .locator(
          '.conv-detail:visible button.trace-head[aria-expanded="false"]',
        )
        .last()
        .click();
    },
  },
  {
    id: "terminal",
    title: "云端 AI 使用本机终端 / Your computer, available to cloud agents",
    zh: "让云端 AI 用上你电脑里的仓库和工具，命令与输出随时可查。",
    en: "Let cloud agents use your local repos and tools, with commands and results visible.",
    tags: ["协作"],
    prepare: async ({ page }) => {
      await nav(page, ".sp-nav-pc");
      const i = page.locator(".term-input").first();
      await i.fill("go test ./...");
      await i.press("Enter");
      await page.waitForTimeout(8500);
    },
  },
  {
    id: "coedit",
    title: "和 Agent 共同编辑 / Co-edit with your agent",
    zh: "你和 AI 在同一个页面工作，修改马上看见。",
    en: "Work with your agent on the same page and see changes as they happen.",
    tags: ["协作", "应用"],
    prepare: coedit,
  },
  {
    id: "workflow",
    title: "可视化工作流 / Visual workflows",
    zh: "把写作、审查和修改串起来，每一步的结果都能追溯。",
    en: "Connect creation, review and revision, with results you can follow at every step.",
    tags: ["自动化", "可见性"],
    prepare: async ({ page }) => {
      await nav(page, ".sp-nav-workflows");
      await click(page.locator("[data-testid=workflows] .au-row"));
      await page.locator("[data-testid=workflow-detail]").waitFor();
    },
  },
  {
    id: "automation",
    title: "定时与事件自动化 / Scheduled and event-driven tasks",
    zh: "日报按时来，CI 出问题也能自动交给 AI 跟进。",
    en: "Get scheduled reports and send CI failures straight to an agent.",
    tags: ["自动化"],
    prepare: async ({ page }) => {
      await nav(page, ".sp-nav-auto");
      await page.locator("[data-testid=automations]").waitFor();
    },
  },
  {
    id: "permissions",
    title: "限时授权与撤销 / Time-limited access",
    zh: "由你决定 AI 能用哪台电脑、做什么，以及什么时候收回。",
    en: "Choose which computer an agent may use, what it may do and when access ends.",
    tags: ["权限与安全"],
    prepare: async ({ api, page, T }) => {
      await api("/app/mock/superagent-ask", "POST", {
        hub: "hub_studio",
        agent: "mbp-frontend",
        kind: "action_grant",
        machine: "Demo Mac",
        what: T(
          "查看 Demo Mac 的屏幕，5 分钟",
          "View Demo Mac’s screen for 5 minutes",
        ),
        reason: T(
          "检查演示应用的布局，不操作键盘鼠标。",
          "Inspect the demo layout without keyboard or mouse input.",
        ),
        permissions: ["remote.desktop.view"],
      });
      await page.reload();
      await page.locator(".sa-approval").first().waitFor();
    },
  },
  {
    id: "usage",
    title: "团队用量分析 / Team usage insights",
    zh: "看清哪些成员、项目和模型用了多少 token。",
    en: "See token usage by member, project and model.",
    tags: ["用量"],
    prepare: async ({ page }) => {
      await click(page.locator(".sp-switch .app-hub-trigger"));
      await click(page.locator("[data-testid=menu-usage]"));
      await page.locator(".usage-page").waitFor();
    },
  },
  {
    id: "sessions",
    title: "接入已有 AI 会话 / Connect existing AI sessions",
    zh: "不必从头配置，把正在用的 Claude Code 或 Codex 加进团队。",
    en: "Bring your existing Claude Code or Codex sessions into the team.",
    tags: ["协作", "设置"],
    prepare: async ({ page, T }) => {
      await click(
        page.locator(`[title="${T("添加 agent", "Add an agent")}"]:visible`),
      );
      await click(
        page
          .getByRole("dialog")
          .getByText(T("这台电脑", "This computer"), { exact: true }),
      );
    },
  },
  {
    id: "focus",
    title: "多对话分屏 / Side-by-side chats",
    zh: "把协调者和成员的对话并排放，少切窗口也能掌握上下文。",
    en: "Keep coordinator and member chats side by side without losing context.",
    description:
      "项目中的双栏对话，左侧协调者、右侧成员，同时查看上下文和任务进展",
    tags: ["协作", "可见性"],
    prepare: async ({ page, T }) => {
      await member(page, T);
      await page.locator(".conv-detail:visible").nth(1).waitFor();
    },
  },
  {
    id: "rich",
    title: "消息里的图表与交互卡片 / Rich answers in chat",
    zh: "AI 的答复可以直接变成图表、步骤和可操作的结果卡。",
    en: "Turn agent answers into diagrams, steps and actionable result cards.",
    tags: ["可见性"],
    prepare: async ({ api, page, T }) => {
      const card = {
        type: "Card",
        props: { title: T("发布检查", "Release check") },
        children: [
          {
            type: "Metric",
            props: {
              label: T("测试通过", "Tests passed"),
              value: "412",
              delta: "+12",
              tone: "success",
            },
          },
          {
            type: "Metric",
            props: {
              label: T("构建时间", "Build time"),
              value: "38s",
              delta: "−8s",
            },
          },
          {
            type: "Callout",
            props: {
              text: T(
                "检查已完成，等待你决定是否发布。",
                "Checks complete. Ready for your release decision.",
              ),
              tone: "success",
            },
          },
          {
            type: "Button",
            props: {
              label: T("准备发布", "Prepare release"),
              action: "reply",
              value: T("整理发布说明", "Prepare release notes"),
            },
          },
        ],
      };
      await api("/app/mock/media-message", "POST", {
        body:
          T("检查结果如下：", "Here are the results:") +
          "\n\n```myco-ui\n" +
          JSON.stringify(card) +
          "\n```",
      });
      await member(page, T);
      await page.locator(".ui-root").last().waitFor();
      await page.locator(".ui-root").last().scrollIntoViewIfNeeded();
    },
  },
  {
    id: "providers",
    title: "统一管理模型与服务商 / Manage models and providers",
    zh: "不用逐台电脑来回配置，在 App 里调整 Agent 的模型和服务商。",
    en: "Adjust agent models and providers from one App instead of visiting every computer.",
    tags: ["设置"],
    prepare: async ({ page }) => {
      await page.locator(".sp-nav-chat-sort").first().click();
      await page
        .getByRole("menuitem", { name: /管理成员|Manage members/ })
        .click();
      await page
        .locator(".mb-card", { hasText: /后端（Codex）|Backend \(Codex\)/ })
        .first()
        .click();
      await page.locator(".ms").waitFor();
      await page.locator(".ms .ms-fold").first().click();
      await page.locator(".ms").evaluate((e) => (e.scrollTop = e.scrollHeight));
    },
  },
  {
    id: "design",
    title: "可编辑设计与白板 / Editable designs and whiteboards",
    zh: "把想法变成能继续修改的画布，而不只是一张图片。",
    en: "Turn ideas into an editable canvas, not just a finished image.",
    tags: ["应用"],
    prepare: design,
  },
  {
    id: "motion",
    title: "动效视频创作 / Motion video creation",
    zh: "让 Agent 帮你组织分镜，在本机做出产品短片。",
    en: "Build a product short with agent-assisted scenes and local video export.",
    tags: ["应用"],
    prepare: motion,
  },
  {
    id: "recording",
    title: "录屏与重点剪辑 / Recording and focused edits",
    zh: "录完直接裁剪、加文字和重点放大，让演示更容易看懂。",
    en: "Trim recordings, add text and zoom into the details that matter.",
    tags: ["应用"],
    skip: "Native recording not available in the web mock. Editing fixture pending.",
  },
  {
    id: "disk",
    title: "可视化空间清理 / Visual disk cleanup",
    zh: "一眼找到占空间的大目录，再决定清理什么。",
    en: "Find the folders taking up space before deciding what to remove.",
    tags: ["应用"],
    prepare: disk,
  },
  {
    id: "remote",
    title: "远程桌面 / Remote desktop",
    zh: "离开电脑也能查看和操作自己的 Mac 或 Windows。",
    en: "View and control your Mac or Windows when you are away.",
    tags: ["应用"],
    skip: "Real desktop control requires the native remote component; no real desktop capture.",
  },
  {
    id: "servers",
    title: "SSH 服务器管理 / SSH Server Console",
    zh: "集中查看 Linux 状态和日志，也能给 Agent 指定服务器的限时权限。",
    en: "Inspect Linux status and logs, and grant an agent temporary access to one server.",
    tags: ["应用", "权限与安全"],
    prepare: servers,
  },
  {
    id: "project-scheduling",
    title: "协调者的权限 / Coordinator permissions",
    zh: "在一处决定协调者能在哪开 Agent、邀请成员和安排定时任务。",
    en: "Choose where coordinators can start agents, invite teammates and schedule tasks in one place.",
    tags: ["协作", "权限与安全", "自动化"], themes: ["light", "dark"],
    description: "项目成员页的协调者权限，集中展示定时任务、邀请和电脑授权",
    prepare: async ({ api, page, T }) => {
      await api("/app/hubs/hub_studio/projects/website/automation-permission", "PUT", { allowed: false });
      await api("/app/hubs/hub_studio/projects/website/agent-invites", "PUT", { allowed: false });
      await page.reload();
      await click(page.locator(".app-nav-item:visible", {hasText:T("官网改版", "Website redesign")}));
      await click(page.locator(".sp-head").getByRole("button", {name:T("项目资料", "Project notes")}));
      await click(page.locator("[data-testid=project-tabs]").getByRole("tab", {name:T("成员", "Members"), exact:false}));
      await page.locator("[data-testid=coordinator-permissions]").scrollIntoViewIfNeeded();
    },
  },
  {
    id: "automation-window",
    title: "有起止日期的自动化 / Bounded schedules",
    zh: "给定时任务设好起止时间，也可以只执行一次，到期自动停用。",
    en: "Set active dates or run a scheduled task once, with automatic stopping.",
    tags: ["自动化", "协作"], themes: ["light", "dark"],
    description: "自动化编辑器中的开始时间、结束时间和只跑一次选项",
    prepare: async ({page,T}) => {
      await click(page.locator(".sp-nav-auto:visible"));
      await click(page.locator(".ph").getByRole("button", {name:T("新建自动化", "New automation")}));
      const editor=page.locator("[data-testid=automation-editor]");await editor.waitFor();
      await editor.getByPlaceholder(T("比如：每天早上汇总 issue", "e.g. Morning issue summary")).fill(T("发布前检查", "Pre-release checks"));
      await editor.locator("textarea").fill(T("核对项目进展，整理发布检查清单。", "Review project progress and prepare the release checklist."));
      await editor.getByLabel(T("开始时间", "Starts at"),{exact:true}).fill("2026-11-26T09:00");
      await editor.getByLabel(T("结束时间", "Ends at"),{exact:true}).fill("2026-11-28T18:00");
      await editor.getByRole("checkbox",{name:T("只跑一次", "Run once"),exact:false}).check();
      await editor.locator("[data-testid=automation-date-editor]").scrollIntoViewIfNeeded();
    },
  },
  {
    id: "project-invites",
    title: "项目协调者邀请 Agent / Coordinator invitations",
    zh: "让项目协调者邀请新 Agent，是否需要逐次批准由你决定。",
    en: "Let project coordinators invite agents, with approval when you need it.",
    tags: ["协作", "权限与安全"],
    themes: ["light", "dark"],
    description: "项目成员设置中的协调者邀请开关，默认关闭，展示一次性邀请和成员权限说明",
    prepare: async ({ api, page, T }) => {
      await api("/app/hubs/hub_studio/projects/website/agent-invites", "PUT", { allowed: false });
      await page.reload();
      await click(page.locator(".app-nav-item:visible", {hasText:T("官网改版", "Website redesign")}));
      await click(page.locator(".sp-head").getByRole("button", {name:T("项目资料", "Project notes")}));
      await click(page.locator("[data-testid=project-tabs]").getByRole("tab", {name:T("成员", "Members"), exact:false}));
      await page.locator("[data-testid=project-agent-invites]").scrollIntoViewIfNeeded();
    },
  },
  {
    id: "project-invite-approval",
    title: "一键批准项目邀请 / Approve project invitations",
    zh: "看清谁将加入、归谁所有，再一键批准一次性邀请。",
    en: "See who will join and who owns the agent before approving an invitation.",
    tags: ["协作", "权限与安全"],
    themes: ["light", "dark"],
    description: "项目协调者的邀请批准卡，展示文档助手、24小时有效和普通成员权限",
    prepare: async ({ api, page, T }) => {
      await api("/app/mock/superagent-ask", "DELETE");
      await api("/app/mock/superagent-ask", "POST", {
        hub: "hub_studio", agent: "mbp-frontend", kind: "project_invite",
        reason: T("协助整理项目文档", "Help organize the project documentation"),
        payload: {invitation_id:"pinv_demo", project:"website", project_title:T("官网改版", "Website redesign"), display_name:T("文档助手", "Documentation assistant"), description:T("整理与校对项目文档", "Organize and review project documentation"), note:T("文档", "Documentation")},
      });
      await page.reload();
      await page.locator(".sa-approval").first().waitFor();
    },
  },

];

scenes.push(...interactionScenes);

// Optional local video actions. Capture the interaction, not a motionless screenshot.
const videos = {
  motion: async ({ page }) => {
    await page
      .locator("iframe:visible")
      .last()
      .contentFrame()
      .locator(".mo-play")
      .evaluate((el) => el.click());
  },
  teamwork: async ({ page, T }) => {
    await member(page, T);
    await page.waitForTimeout(1800);
  },
  progress: async ({ page }) => {
    const b = page.locator("button.trace-head").last();
    await b.click();
    await page.waitForTimeout(800);
    await b.click();
  },
  terminal: async ({ page }) => {
    const i = page.locator(".term-input").first();
    await i.fill("go test ./...");
    await i.press("Enter");
    await page.waitForTimeout(4500);
  },
  workflow: async ({ page }) => {
    await page
      .locator("[data-testid=workflow-runs] .wf-run-row")
      .first()
      .click();
    await page.locator("[data-testid=workflow-run]").waitFor();
  },
  automation: async ({ page }) => {
    await page
      .locator("[data-testid=automations] .au-row-main")
      .first()
      .click();
    await page.locator("[data-testid=automation-detail]").waitFor();
  },
};
for (const s of scenes) if (videos[s.id]) s.video = videos[s.id];
