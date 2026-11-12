// README screenshots of the app, from the showcase mock (2x, 1280x800).
//   cd web && MOCK_SHELL=mac MOCK_SHOWCASE=zh node mock/app-server.mjs & npx vite --port 5174
//   node scripts/readme-shots.cjs zh /tmp/shots/zh      (restart the mock with en for English)
//   python3 scripts/readme-frame.py /tmp/shots/zh/nodes.png ../docs/assets/app/zh/nodes.webp   (each file)
// Start the mock fresh for each language: its demo tasks are dated from when it started.
// Playwright must be installed where node can find it.
const { chromium } = require('playwright');
(async () => {
  const [lang, out] = process.argv.slice(2);
  const zh = lang === 'zh'; const T = (z, e) => (zh ? z : e);
  const b = await chromium.launch(); const notes = [];
  const shot = async (name, dark, fn, wait = 700) => {
    const ctx = await b.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2, colorScheme: dark ? 'dark' : 'light', locale: zh ? 'zh-CN' : 'en-US' });
    const p = await ctx.newPage(); p.setDefaultTimeout(8000);
    p.on('pageerror', (e) => notes.push(name + ' pageerror ' + e.message));
    await p.goto(`http://127.0.0.1:5174/?lang=${lang}`, { waitUntil: 'networkidle' }); await p.waitForTimeout(1200);
    try { await fn(p); } catch (e) { notes.push(name + ' FAILED ' + e.message.split('\n')[0]); }
    await p.waitForTimeout(wait);
    await p.screenshot({ path: `${out}/${name}${dark ? '-dark' : ''}.png` });
    await ctx.close();
  };
  const nav = (p) => p.locator('nav, aside').first();
  // the project: your chat with its coordinator, the members and computers beside it
  const home = async () => {};
  // the same, with a computer's terminal open and a command running in it
  const terminal = async (p) => {
    await p.locator('.sp-nav-pc').first().click(); await p.waitForTimeout(1500);
    // The mock keeps what ran before (the light pass): run it only once.
    if (!(await p.getByText('go test ./...').count())) {
      const c = p.locator('.term-input').first(); await c.fill('go test ./...'); await c.press('Enter');
      await p.waitForTimeout(4200);
    }
  };
  // add an agent: the sessions found on this computer
  const addAgent = async (p) => {
    await p.locator(`[title="${T('添加 agent', 'Add an agent')}"]:visible`).first().click(); await p.waitForTimeout(600);
    await p.getByRole('dialog').getByText(T('这台电脑上的', 'On this computer'), { exact: true }).click(); await p.waitForTimeout(800);
  };
  const members = async (p) => { await nav(p).getByText(T('成员', 'Members'), { exact: true }).click(); await p.waitForTimeout(800); };
  const memberCard = async (p) => { await members(p); await p.getByText(T('后端（Codex）', 'Backend (Codex)')).first().click(); };
  for (const dark of [false, true]) {
    await shot('nodes', dark, home);
    await shot('conversation', dark, terminal, 300);
    await shot('add-node', dark, addAgent);
  }
  await shot('node', false, memberCard);
  await shot('machines', false, members);
  console.log(notes.join('\n') || 'ok'); await b.close();
})();
