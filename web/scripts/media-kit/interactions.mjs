// Keyboard-first and conversation-level interaction scenes. Synthetic data only.
async function chat({ api, page, lang, T }) {
  await api("/app/mock/media-interactions", "POST", { lang });
  await page
    .locator(".sp-side .sp-member:visible", {
      hasText: T("后端（Codex）", "Backend (Codex)"),
    })
    .first()
    .click();
  await page.locator(".conv-detail").last().waitFor();
  await page
    .locator(".conv-detail")
    .last()
    .getByText(T("总结这次改动", "Summarize the changes"), { exact: false })
    .last()
    .waitFor();
  await page.waitForTimeout(500);
}
const lastChat = (page) => page.locator(".conv-detail:visible").last();
export const interactionScenes = [
  {
    id: "agent-switcher",
    title: "Ctrl+Tab 快速切换 Agent / Quick agent switching",
    zh: "按住 Ctrl 连按 Tab，松开就回到你想找的 Agent。",
    en: "Hold Ctrl and press Tab to pick an agent, then release to open its chat.",
    tags: ["协作", "可见性"],
    description:
      "项目对话上方的Agent快速切换浮层，头像横排、当前选择和键盘提示",
    prepare: async (c) => {
      await chat(c);
      await c.page.keyboard.down("Control");
      await c.page.keyboard.press("Tab");
      await c.page.keyboard.press("Tab");
      await c.page.locator(".asw").waitFor();
    },
    video: async ({ page }) => {
      await page.keyboard.press("Tab");
      await page.waitForTimeout(700);
      await page.keyboard.press("Shift+Tab");
      await page.waitForTimeout(700);
      const picked = await page.locator(".asw-name").innerText();
      await page.keyboard.up("Control");
      await page
        .locator(".conv-head:visible", { hasText: picked })
        .first()
        .waitFor();
      await page.locator(".asw").waitFor({ state: "hidden" });
    },
  },
  {
    id: "focus-mode",
    title: "独立专注模式 / A quiet space for one task",
    zh: "只留当前任务和安静的背景，把注意力放回正在做的事。",
    en: "Keep one task on a quiet background and focus on the work in front of you.",
    tags: ["协作", "设置"],
    description:
      "专注模式中的单个任务对话，居中排版、安静背景，侧栏和其他面板收起",
    prepare: async (c) => {
      await chat(c);
      await c.page.keyboard.press("Control+Shift+f");
      await c.page.locator(".app-shell.is-focus").waitFor();
      await c.page.locator(".focus-tone").click();
      await c.page
        .locator(".focus-bg-tile", { hasText: c.T("纸张", "Paper") })
        .first()
        .click();
      await c.page.keyboard.press("Escape");
    },
    video: async ({ page }) => {
      await page.locator(".focus-exit").click();
      await page.waitForTimeout(700);
      await page.keyboard.press("Control+Shift+f");
      await page.locator(".app-shell.is-focus").waitFor();
    },
  },
  {
    id: "next-suggestion",
    title: "Tab 填入下一句建议 / Pick up the next suggestion",
    zh: "Agent 把下一步放在输入框里，按 Tab 填入，改好再发。",
    en: "Pick up an agent’s suggested next step with Tab, then edit it before sending.",
    tags: ["协作"],
    description:
      "任务完成后的输入框，显示下一句建议及Tab填入提示，内容尚未发送",
    prepare: async (c) => {
      await chat(c);
      await lastChat(c.page)
        .getByRole("button", {
          name: c.T("只看这一栏", "Show only this pane"),
          exact: true,
        })
        .click();
      await lastChat(c.page).locator(".conv-compose-input").click();
      await lastChat(c.page)
        .locator(".conv-compose-hint")
        .waitFor({ state: "visible" });
    },
    video: async ({ page }) => {
      const input = lastChat(page).locator(".conv-compose-input");
      await input.focus();
      await page.keyboard.press("Tab");
      await page.waitForTimeout(700);
      await page.keyboard.type(" + checklist");
    },
  },
  {
    id: "turn-nav",
    title: "对话轮次导航 / Navigate a conversation by turns",
    zh: "沿着对话侧边的刻度预览和跳转，不用在长对话里反复滚动。",
    en: "Preview and jump between turns from the chat’s edge instead of scrolling through a long conversation.",
    tags: ["可见性"],
    description: "长对话右侧的轮次导航刻度，悬停预览某轮问题和答复",
    prepare: async (c) => {
      await chat(c);
      await lastChat(c.page).locator(".conv-rail-tick").nth(2).hover();
      await c.page.locator(".conv-rail-card").last().waitFor();
    },
    video: async ({ page }) => {
      const ticks = lastChat(page).locator(".conv-rail-tick");
      await ticks.nth(1).click();
      await page.waitForTimeout(800);
      await ticks.nth(5).hover();
      await page.waitForTimeout(700);
      await ticks.nth(5).click();
    },
  },
  {
    id: "session-handover",
    title: "让另一个会话接着做 / Continue in another session",
    zh: "需要换会话或电脑时，让新的会话带上最近的上下文继续做。",
    en: "Move to another session or computer while carrying recent context forward.",
    tags: ["协作", "设置"],
    description: "成员的会话交接对话框，选择接手电脑和会话方式，尚未执行交接",
    prepare: async ({ page, T }) => {
      await page.locator(".sp-nav-chat-sort").first().click();
      await page
        .getByRole("menuitem", { name: T("管理成员", "Manage members") })
        .click();
      await page
        .locator(".mb-card", { hasText: T("后端（Codex）", "Backend (Codex)") })
        .first()
        .click();
      await page.locator("[data-testid=handover-open]").click();
      await page.locator(".app-modal").last().waitFor();
    },
  },
  {
    id: "chat-background",
    title: "每段对话自己的背景 / A background for each conversation",
    zh: "给不同对话选不同背景，用熟悉的画面找回上下文。",
    en: "Give conversations distinct backgrounds so familiar surroundings help you find your context.",
    tags: ["设置"],
    description: "单个对话的聊天背景设置，纹理选择与即时预览，只修改本机外观",
    prepare: async (c) => {
      await chat(c);
      await lastChat(c.page).locator("[data-testid=chat-more]").click();
      await c.page
        .getByRole("menuitem", { name: c.T("聊天背景", "Chat background") })
        .click();
      await c.page
        .locator(".cbg-tile", { hasText: c.T("点阵", "Dots") })
        .first()
        .click();
      await c.page
        .locator(".cbg-dialog")
        .getByRole("button", { name: c.T("好", "Done"), exact: true })
        .click();
      await lastChat(c.page).locator(".cbg-layer").waitFor();
    },
  },
  {
    id: "runner-queue",
    title: "电脑名额与任务排队 / See when a task is waiting",
    zh: "看清任务排第几、电脑有多忙，不再把排队误当成启动卡住。",
    en: "See your task’s queue position and the computer’s capacity instead of guessing why it has not started.",
    tags: ["可见性"],
    description: "任务对话中的排队提示，显示电脑3/3名额在忙、任务排第2",
    prepare: async (c) => {
      await c.api("/app/hubs/hub_studio/messages", "POST", {
        to: "build-backend",
        body: c.T("名额排队预览", "Slot queue preview"),
      });
      await c.page
        .locator(".sp-side .sp-member:visible", {
          hasText: c.T("后端（Codex）", "Backend (Codex)"),
        })
        .first()
        .click();
      await c.page.locator('[data-testid="runner-queue"]').last().waitFor();
      await c.page
        .locator(".conv-detail:visible")
        .last()
        .getByRole("button", {
          name: c.T("只看这一栏", "Show only this pane"),
          exact: true,
        })
        .click();
    },
  },
];
