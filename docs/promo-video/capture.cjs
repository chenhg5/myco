// Captures the app states the promo film uses, from the showcase mock (2x, 1280x800):
//   cd web && MOCK_SHELL=mac MOCK_SHOWCASE=en node mock/app-server.mjs & npx vite --port 5174
//   node capture.cjs en <assets dir>     (then the same with zh)
const { chromium } = require('playwright');
(async () => {
  const [lang, out] = process.argv.slice(2); const zh = lang === 'zh'; const T = (z, e) => zh ? z : e;
  const b = await chromium.launch(); const notes = [];
  const shot = async (name, dark, fn) => {
    const ctx = await b.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2, colorScheme: dark ? 'dark' : 'light' });
    const p = await ctx.newPage(); p.setDefaultTimeout(8000);
    p.on('pageerror', e => notes.push(name + ' ' + e.message));
    await p.goto(`http://127.0.0.1:5174/?lang=${lang}`, { waitUntil: 'networkidle' }); await p.waitForTimeout(800);
    try { await fn(p); } catch (e) { notes.push(name + ' FAILED ' + e.message.split('\n')[0]); }
    await p.waitForTimeout(700); await p.screenshot({ path: `${out}/${name}.png` }); await ctx.close();
  };
  await shot('nodes', false, async () => {});
  await shot('nodes-dark', true, async () => {});
  await shot('add-node-dark', true, async (p) => {
    await p.getByRole('button', { name: T('添加节点', 'Add node') }).first().click(); await p.waitForTimeout(900);
  });
  await shot('invite', true, async (p) => {
    await p.getByRole('button', { name: T('添加节点', 'Add node') }).first().click(); await p.waitForTimeout(500);
    await p.getByRole('tab', { name: T('其他电脑上的 agent', 'Agent on another computer') }).click(); await p.waitForTimeout(1500);
  });
  await shot('added', true, async (p) => {
    await p.getByRole('button', { name: T('添加节点', 'Add node') }).first().click(); await p.waitForTimeout(800);
    await p.getByText(T('把首页的 hero 区改成新设计', 'Redo the home page hero from the new design')).click(); await p.waitForTimeout(200);
    await p.locator('.app-modal').getByRole('button', { name: T('加入', 'Join'), exact: true }).click(); await p.waitForTimeout(2500);
    const done = p.getByRole('button', { name: T('完成', 'Done') }); if (await done.count()) await done.first().click();
    await p.waitForTimeout(600);
  });
  await shot('filtered', true, async (p) => {
    await p.getByLabel(T('按分组和标签筛选', 'Filter by group and tag')).selectOption(`group:${T('官网改版', 'website')}`);
  });
  await shot('appearance', true, async (p) => {
    await p.getByRole('button', { name: T('设置', 'Settings') }).first().click(); await p.waitForTimeout(300);
    await p.getByText(T('外观', 'Appearance'), { exact: true }).first().click();
  });
  await shot('machines-list', true, async (p) => {
    await p.getByRole('button', { name: T('机器', 'Machines') }).first().click();
  });
  await shot('conversation-dark', true, async (p) => {
    await p.getByRole('button', { name: T('对话', 'Conversations') }).first().click(); await p.waitForTimeout(500);
    await p.getByText(T('在写 FAQ', 'Writing the FAQ')).first().click(); await p.waitForTimeout(400);
    await p.locator('.conv-scroll').evaluate((e) => { e.scrollTop = 0; });
  });
  await shot('node-dark', true, async (p) => {
    await p.getByText(T('前端（Claude Code）', 'Frontend (Claude Code)')).first().click();
  });
  console.log(notes.join('\n') || 'ok'); await b.close();
})();
