// Captures the app states the promo film shows, from the showcase mock (dark,
// 1280x800 @2x; the phone at 390x844 @3x), and writes boxes.json: where the
// film aims, in app px.
//   cd web && MOCK_SHELL=mac MOCK_SHOWCASE=zh node mock/app-server.mjs & npx vite --port 5174
//   node capture.cjs zh <assets dir> [scale]   (restart the mock with MOCK_SHOWCASE=en, then the same with en)
// Start the mock fresh for each language: its demo tasks are dated from when it started.
const { chromium } = require('playwright');
(async () => {
  const [lang, out, scaleArg] = process.argv.slice(2);
  const scale = Number(scaleArg || 2);
  const zh = lang === 'zh'; const T = (z, e) => (zh ? z : e);
  const b = await chromium.launch(); const notes = []; const boxes = {};
  const box = async (key, loc) => {
    try { const r = await loc.first().boundingBox({ timeout: 4000 }); boxes[key] = r && [Math.round(r.x), Math.round(r.y), Math.round(r.width), Math.round(r.height)]; }
    catch (e) { notes.push(key + ' ' + e.message.split('\n')[0]); }
  };
  const shot = async (name, fn, { measure, wait = 700, phone = false } = {}) => {
    const ctx = await b.newContext(phone
      ? { viewport: { width: 390, height: 844 }, deviceScaleFactor: scale === 1 ? 1 : 3, colorScheme: (process.env.SCHEME || 'dark'), locale: zh ? 'zh-CN' : 'en-US', isMobile: true, hasTouch: true }
      : { viewport: { width: 1280, height: 800 }, deviceScaleFactor: scale, colorScheme: (process.env.SCHEME || 'dark'), locale: zh ? 'zh-CN' : 'en-US' });
    const p = await ctx.newPage(); p.setDefaultTimeout(8000);
    p.on('pageerror', (e) => notes.push(name + ' pageerror ' + e.message));
    await p.goto(`http://127.0.0.1:5174/?lang=${lang}`, { waitUntil: 'networkidle' }); await p.waitForTimeout(1200);
    try { await fn(p); } catch (e) { notes.push(name + ' FAILED ' + e.message.split('\n')[0]); }
    await p.waitForTimeout(wait);
    await p.screenshot({ path: `${out}/${name}.png` });
    if (measure) { try { await measure(p); } catch (e) { notes.push(name + ' measure ' + e.message.split('\n')[0]); } }
    await ctx.close();
  };
  const FE = T('前端（Claude Code）', 'Frontend (Claude Code)');
  const BE = T('后端（Codex）', 'Backend (Codex)');
  const REVIEW = T('代码审查（Codex）', 'Reviewer (Codex)');
  const RESEARCH = T('调研（claude.ai）', 'Research (claude.ai)');
  const SHELL = T('服务器 shell', 'Server shell');
  const navChat = (p, name) => p.locator('.sp-nav', { hasText: name }).first();

  // 1. The project: your chat with its coordinator (at work), the members beside it.
  await shot('home', async () => {}, {
    measure: async (p) => {
      await box('home.nav', p.locator('.app-sidebar'));
      await box('home.chats', p.locator('.sp-nav-chats, .app-nav-group').nth(1));
      await box('home.dispatch', p.locator('.handoff').first());
      await box('home.working', p.locator('.trace-head').last());
      await box('home.members', p.locator('.sp-pane-right, .sp-members, .sp-side').first());
    },
  });

  // 2. Settings › Agents: the agents on this computer, signed in.
  await shot('agents', async (p) => {
    await p.locator('.app-hub-switch, .sp-switch button').first().click(); await p.waitForTimeout(300);
    await p.locator('.app-hub-menu-item', { hasText: T('设置', 'Settings') }).click(); await p.waitForTimeout(500);
    await p.locator('.app-settings-tab', { hasText: T('本机 agent', 'Agents') }).click(); await p.waitForTimeout(800);
  }, { measure: async (p) => { await box('agents.list', p.locator('.la-line').first().locator('..')); } });

  // 3. A new conversation: the agent, a folder (may stay empty).
  await shot('new', async (p) => {
    await p.locator(`[title="${T('添加 agent', 'Add an agent')}"]:visible`).first().click(); await p.waitForTimeout(600);
    await p.getByRole('dialog').getByRole('tab', { name: T('新开一个', 'Start a new one') }).click(); await p.waitForTimeout(800);
  }, { measure: async (p) => { await box('new.dialog', p.getByRole('dialog')); } });

  // 4. One chat: the agent's run folded into a line, its answer.
  await shot('chat', async (p) => {
    await navChat(p, BE).click(); await p.waitForTimeout(2500);
  }, { measure: async (p) => { await box('chat.detail', p.locator('.conv-detail').last()); } });

  // 5. Four agents side by side.
  await shot('panes', async (p) => {
    await navChat(p, BE).click(); await p.waitForTimeout(1200);
    for (const n of [FE, REVIEW, RESEARCH]) {
      await navChat(p, n).click({ button: 'right' }); await p.waitForTimeout(300);
      const item = p.getByRole('menuitem', { name: T('在新的一栏打开', 'Open in a new pane') });
      if (await item.count()) { await item.first().click(); } else { await p.keyboard.press('Escape'); notes.push('panes: no 在新的一栏打开 for ' + n); }
      await p.waitForTimeout(900);
    }
    await p.waitForTimeout(1500);
  }, { wait: 1200 });

  // 6. Another computer: its terminal, a command running.
  await shot('terminal', async (p) => {
    await p.locator('.sp-nav-pc').first().click(); await p.waitForTimeout(1500);
    if (!(await p.getByText('go test ./...').count())) {
      const c = p.locator('.term-input').first(); await c.fill('go test ./...'); await c.press('Enter');
      await p.waitForTimeout(8000);
    }
  }, { wait: 300 });

  // 7. Automations and apps.
  await shot('automations', async (p) => { await p.locator('.app-nav-item', { hasText: T('自动化', 'Automations') }).first().click(); await p.waitForTimeout(1500); });
  await shot('apps', async (p) => { await p.locator('.app-nav-item', { hasText: T('应用', 'Apps') }).first().click(); await p.waitForTimeout(1500); });

  // 8. The phone: the same project.
  await shot('phone', async () => {}, { phone: true, wait: 1500 });

  require('fs').writeFileSync(`${out}/boxes.json`, JSON.stringify(boxes, null, 1));
  console.log(notes.join('\n') || 'ok'); await b.close();
})();
