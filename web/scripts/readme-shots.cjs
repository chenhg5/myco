// README screenshots of the app, from the showcase mock (2x, light and dark).
//   cd web && MOCK_SHOWCASE=zh node mock/app-server.mjs & npx vite --port 5174
//   node scripts/readme-shots.cjs zh /tmp/shots/zh      (restart the mock with en for English)
//   python3 scripts/readme-frame.py /tmp/shots/zh/nodes.png ../docs/assets/app/zh/nodes.webp   (each file)
// Playwright must be installed where node can find it.
const { chromium } = require('playwright');
(async () => {
  const [lang, out] = process.argv.slice(2);
  const zh = lang === 'zh';
  const b = await chromium.launch(); const notes = [];
  const shot = async (name, dark, fn) => {
    const ctx = await b.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2, colorScheme: dark ? 'dark' : 'light', locale: zh ? 'zh-CN' : 'en-US' });
    const p = await ctx.newPage(); p.setDefaultTimeout(8000);
    p.on('pageerror', e => notes.push(name + ' pageerror ' + e.message));
    await p.goto(`http://127.0.0.1:5174/?lang=${lang}`, { waitUntil: 'networkidle' }); await p.waitForTimeout(900);
    await fn(p);
    await p.waitForTimeout(600);
    await p.screenshot({ path: `${out}/${name}${dark ? '-dark' : ''}.png` });
    await ctx.close();
  };
  const T = (z, e) => zh ? z : e;
  for (const dark of [false, true]) {
    await shot('nodes', dark, async () => {});
    await shot('conversation', dark, async (p) => {
      await p.getByRole('button', { name: T('对话', 'Conversations') }).first().click(); await p.waitForTimeout(500);
      await p.getByText(T('在写 FAQ', 'Writing the FAQ')).first().click();
    });
    await shot('add-node', dark, async (p) => {
      await p.getByRole('button', { name: T('添加节点', 'Add node') }).first().click(); await p.waitForTimeout(900);
    });
  }
  await shot('node', false, async (p) => {
    await p.getByText(T('前端（Claude Code）', 'Frontend (Claude Code)')).first().click();
  });
  await shot('machines', false, async (p) => {
    await p.getByRole('button', { name: T('机器', 'Machines') }).first().click(); await p.waitForTimeout(500);
    await p.getByText('MacBook Pro').first().click();
  });
  console.log(notes.join('\n') || 'ok'); await b.close();
})();
