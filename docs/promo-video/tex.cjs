// Textures for film3d.html: the five agent windows (windows.html) and the marks, into $WORK/tex.
//   WORK=... node tex.cjs        (needs $WORK/logos/{claude,codex,cursor,grok,gemini}.png from fetch-logos.sh)
const { chromium } = require('playwright'); const { join } = require('path'); const fs = require('fs');
(async () => {
  const W = process.env.WORK; const out = join(W, 'tex'); fs.mkdirSync(out, { recursive: true });
  const b = await chromium.launch({ args: ['--allow-file-access-from-files'] });
  const p = await b.newPage({ viewport: { width: 900, height: 1800 }, deviceScaleFactor: 2 });
  await p.goto('file://' + join(__dirname, 'windows.html') + '?logos=' + join(W, 'logos'));
  await p.waitForTimeout(600);
  for (let i = 0; i < 5; i++) await p.locator('#w' + i).screenshot({ path: join(out, `w${i}.png`), omitBackground: true });
  for (const n of ['claude', 'codex', 'cursor', 'grok', 'gemini']) fs.copyFileSync(join(W, 'logos', n + '.png'), join(out, 'm-' + n + '.png'));
  await b.close(); console.log('ok');
})();
