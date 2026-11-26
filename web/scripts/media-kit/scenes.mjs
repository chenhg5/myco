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
    id: "project-storage",
    title: "项目文件独立额度 / Project storage limits",
    zh: "图片和视频不再挤占文档名额，还能为每个项目单独增加文件空间。",
    en: "Keep images and videos separate from document slots, and give each project the storage it needs.",
    tags: ["设置", "用量"], themes: ["light", "dark"],
    description: "工作区设置中的项目文件额度，展示文件数量、总大小、独立上限和恢复默认值",
    prepare: async ({page,T}) => {
      const usage={files_used:76,docs_used:24,bytes_used:416*1024**2};
      await page.route("**/app/hubs/hub_studio/projects",async route=>{
        const response=await route.fetch(),data=await response.json();
        await route.fulfill({response,json:{...data,projects:data.projects.map(p=>p.name==="website"?{...p,...usage}:p)}});
      });
      await page.route("**/app/hubs/hub_studio/projects/website/storage",async route=>{
        const response=await route.fetch();await route.fulfill({response,json:{...await response.json(),...usage}});
      });
      await page.evaluate(()=>window.dispatchEvent(new CustomEvent("myco-open-settings",{detail:{section:"hubs"}})));
      const card=page.locator('[data-setting="project-storage"][data-hub="hub_studio"]');
      await card.locator("select").selectOption("website");
      await card.getByRole("spinbutton",{name:T("文件数上限","File count limit"),exact:true}).fill("2000");
      await card.getByRole("spinbutton",{name:T("项目总大小上限（GiB）","Total project size limit (GiB)"),exact:true}).fill("4");
      await card.getByRole("button",{name:T("保存","Save"),exact:true}).click();
      await card.getByRole("status").filter({hasText:T("已保存","Saved")}).waitFor();
      await card.scrollIntoViewIfNeeded();
    },
  },

  {
    id: "browser-approved-version",
    title: "批准固定版本的网页自动化 / Approve a fixed browser script",
    zh: "先看清网址和代码，再让 Agent 在专用浏览器里运行你批准的版本。",
    en: "Review the URLs and code before an agent runs your approved version in a dedicated browser profile.",
    tags: ["应用", "权限与安全", "自动化"], themes: ["light", "dark"],
    description: "宿主的浏览器脚本批准框，展示固定版本、profile、网址和只读代码",
    prepare: async ({page,T}) => {
      await click(page.locator(".app-nav-item:visible",{hasText:T("官网改版","Website redesign")}));
      await page.evaluate(()=>window.dispatchEvent(new CustomEvent("myco-open-page",{detail:{kind:"page",hubId:"hub_studio",id:"pg_browser_review"}})));
      const frame=page.frameLocator(".pg-frame");await frame.locator("h1").waitFor();
      const request={script_id:"monthly_report",version:3,profile:"profile_work",script:{name:T("月报检查","Monthly report check"),profile:"profile_work",start_url:"https://example.com/reports",params:[],on_error:"stop",graph:{nodes:[{id:"check-title",type:"js",code:"return document.title;"},{id:"open-summary",type:"navigate",url:"https://example.com/summary"}],edges:[["check-title","open-summary"]]}},scripts:{}};
      await frame.locator("body").evaluate((_body,request)=>{window.myco.browser.approvals.request(request).catch(()=>{});},request);
      await page.locator("[data-testid=browser-script-review]").waitFor();
    },
  },

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

  {
    id: "watch",
    title: "后台盯着，一眼看到 / Watching in the background",
    zh: "Agent 在后台等回复、等流水线时，会话顶部写着在等什么、还剩多久，可以延长或停止。",
    en: "While an agent waits for a reply or a pipeline, its chat says what it is waiting for and for how long, with Extend and Stop.",
    tags: ["可见性", "协作"], themes: ["light", "dark"],
    description: "Agent 会话顶部的「正在盯」条（展开：最近一次检查、起止时间），会话里结束的灰字，成员栏的「盯着 N」",
    prepare: async ({ page, T }) => {
      await member(page, T);
      const bar = page.locator(".conv-detail:visible [data-testid=watch-bar]").last();
      await bar.waitFor();
      await bar.locator(".watch-toggle").click();
      await bar.locator(".watch-result").waitFor();
    },
  },
  {
    id: "goal",
    title: "给会话一个目标 / A goal for a chat",
    zh: "设定一个目标和完成标准，Agent 空闲时按节奏被唤醒，一轮一轮往前推，每轮做了什么都看得到。",
    en: "Set a goal and when it counts as done; the agent is woken on schedule while idle and pushes it forward round by round, every round on record.",
    tags: ["自动化", "可见性"], themes: ["light", "dark"],
    description: "Agent 会话顶部的目标条（第几轮、下次唤醒、已进行多久，展开：终止条件、上一轮、下一步、每轮记录），以及成员栏的「目标中」",
    prepare: async ({ page, T }) => {
      await member(page, T);
      const bar = page.locator(".conv-detail:visible [data-testid=goal-bar]").last();
      await bar.waitFor();
      await bar.locator(".watch-toggle").click();
      await bar.locator(".goal-round").first().waitFor();
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
