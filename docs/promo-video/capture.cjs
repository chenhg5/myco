// Captures the app states the promo film uses, from the showcase mock (dark, 1280x800 @2x),
// and writes boxes.json: where the film aims (app px). film.html's BOX holds these numbers.
//   cd web && MOCK_SHELL=mac MOCK_SHOWCASE=en node mock/app-server.mjs & npx vite --port 5174
//   node capture.cjs en <assets dir>     (restart the mock with MOCK_SHOWCASE=zh, then the same with zh)
// Start the mock fresh for each language: its demo tasks are dated from when it started, and a
// task quiet for over 10 minutes shows as possibly stuck.
const { chromium } = require('playwright');
(async () => {
  const [lang, out] = process.argv.slice(2); const zh = lang === 'zh'; const T = (z, e) => zh ? z : e;
  const b = await chromium.launch(); const notes = []; const boxes = {};
  const box = async (key, loc) => { try { const r = await loc.first().boundingBox({ timeout: 4000 }); boxes[key] = r && [Math.round(r.x), Math.round(r.y), Math.round(r.width), Math.round(r.height)]; } catch (e) { notes.push(key + ' ' + e.message.split('\n')[0]); } };
  const shot = async (name, fn, measure, wait = 700) => {
    const ctx = await b.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2, colorScheme: 'dark' });
    const p = await ctx.newPage(); p.setDefaultTimeout(8000);
    p.on('pageerror', e => notes.push(name + ' ' + e.message));
    await p.goto(`http://127.0.0.1:5174/?lang=${lang}`, { waitUntil: 'networkidle' }); await p.waitForTimeout(800);
    try { await fn(p); } catch (e) { notes.push(name + ' FAILED ' + e.message.split('\n')[0]); }
    await p.waitForTimeout(wait); await p.screenshot({ path: `${out}/${name}.png` });
    if (measure) await measure(p);
    await ctx.close();
  };
  const FE = T('前端（Claude Code）', 'Frontend (Claude Code)');
  const SHELL = T('服务器 shell', 'Server shell');
  // 1. every agent
  await shot('nodes-dark', async () => {}, async (p) => { await box('nodes.frontend', p.locator('.app-row', { hasText: FE })); });
  // 2. what it is doing: the running task and its steps
  await shot('conversation-dark', async (p) => {
    await p.getByRole('button', { name: T('对话', 'Conversations') }).first().click(); await p.waitForTimeout(500);
    await p.locator('.conv-row', { hasText: FE }).first().click(); await p.waitForTimeout(2500);
  }, async (p) => {
    await box('conv.row', p.locator('.conv-row', { hasText: FE }));
    await box('conv.task', p.locator('.conv-msg', { hasText: T('在套餐下面加 FAQ', 'add an FAQ below the plans') }).last());
    await box('conv.trace', p.locator('.conv-detail .trace').last());
    await box('conv.head', p.locator('.conv-head'));
  }, 1200);
  // 3. a shell on your computer, its output as it runs
  await shot('shell-dark', async (p) => {
    await p.getByRole('button', { name: T('对话', 'Conversations') }).first().click(); await p.waitForTimeout(500);
    await p.getByRole('button', { name: /新对话|New conversation/ }).first().click(); await p.waitForTimeout(500);
    const dlg = p.getByRole('dialog');
    const input = dlg.locator('input').first(); await input.fill('shell'); await p.waitForTimeout(400);
    await dlg.locator('.app-row', { hasText: SHELL }).first().click(); await p.waitForTimeout(800);
    const c = p.locator('.conv-compose-input, .term-input, textarea').first();
    await c.fill('go test ./...'); await c.press('Enter');
    await p.waitForTimeout(4200);
  }, async (p) => {
    await box('shell.body', p.locator('.conv-detail'));
    await box('shell.head', p.locator('.conv-head'));
    await box('shell.cmd', p.getByText('go test ./...').last());
  }, 300);
  // 4. add one
  await shot('add-node-dark', async (p) => {
    await p.getByRole('button', { name: T('添加节点', 'Add node') }).first().click(); await p.waitForTimeout(900);
  }, async (p) => {
    await box('add.session', p.locator('.app-modal').getByText(T('把首页的 hero 区改成新设计', 'Redo the home page hero from the new design')).locator('xpath=ancestor::*[self::button or contains(@class,"row")][1]'));
    await box('add.join', p.locator('.app-modal').getByRole('button', { name: T('加入', 'Join'), exact: true }));
  });
  await shot('added', async (p) => {
    await p.getByRole('button', { name: T('添加节点', 'Add node') }).first().click(); await p.waitForTimeout(800);
    await p.getByText(T('把首页的 hero 区改成新设计', 'Redo the home page hero from the new design')).click(); await p.waitForTimeout(200);
    await p.locator('.app-modal').getByRole('button', { name: T('加入', 'Join'), exact: true }).click(); await p.waitForTimeout(2500);
    const done = p.getByRole('button', { name: T('完成', 'Done') }); if (await done.count()) await done.first().click();
    await p.waitForTimeout(600);
  }, async (p) => { await box('added.row', p.locator('.app-page .app-row', { hasText: 'MacBook Pro · website' })); });
  require('fs').writeFileSync(`${out}/boxes.json`, JSON.stringify(boxes));
  console.log(notes.join('\n') || 'ok'); await b.close();
})();
