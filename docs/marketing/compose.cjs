const { chromium } = require('playwright');
// Marketing images (Product Hunt gallery size, 1270x760 at 2x), English and
// Chinese, from App captures:
//   node compose.cjs <captures dir> <phone shots dir> <work dir>
// <captures dir>/<en|zh>/<home|panes|terminal|chat|apps|automations>.png come
// from docs/promo-video/capture.cjs (SCHEME=light); <phone shots dir> has
// <en|zh-Hans>-<chats|agent>.jpg from the iPhone app in the simulator
// (-myco-start demo). The images go to <work dir>/out/<lang>/ as PNG.
const path = require('path');
const [CAP, G, M] = process.argv.slice(2).map((p) => path.resolve(p));
const LOGO = path.resolve(__dirname, '../assets/logo');
const S = {
  en: [
    { id: '01-hero', eyebrow: 'OPEN SOURCE · MAC · WINDOWS · LINUX', h: 'All your AI agents,<br>one app.', sub: 'Claude Code, Codex and the agents you already pay for: talk to them, run several at once, on any of your computers.', img: 'home' },
    { id: '02-together', eyebrow: 'PROJECTS', h: 'Agents that work together', sub: 'A coordinator hands work to Claude Code and Codex. Watch four of them side by side, every step as it happens.', img: 'panes' },
    { id: '03-computer', eyebrow: 'YOUR COMPUTERS', h: 'Your computers, in the loop', sub: 'Agents run commands on your computers and servers, in a terminal you watch live. Only the agents you allow; no ports to open.', img: 'terminal' },
    { id: '04-chat', eyebrow: 'CONVERSATIONS', h: 'See the work, not just the answer', sub: 'What it read, what it ran, what it changed, folded into one line. Step in while it works.', img: 'chat' },
    { id: '05-apps', eyebrow: 'APPS', h: 'Apps your agents use too', sub: 'Docs, sheets, slides, a whiteboard, video and more. You and your agents work on the same page.', img: 'apps' },
    { id: '06-automations', eyebrow: 'AUTOMATIONS', h: 'Work that starts on its own', sub: 'A morning summary, a Friday report, a look when CI fails: on a schedule, or when something calls.', img: 'automations' },
    { id: '07-phone', phone: true, pill: 'iPhone · Public beta on TestFlight', h: 'Your agents,<br>in your pocket', sub: 'Every conversation on every computer, on your phone. Follow the work and hand out the next task from anywhere.', url: 'github.com/chenhg5/myco' },
  ],
  zh: [
    { id: '01-hero', eyebrow: '开源 · Mac · Windows · Linux', h: '你所有的 AI agent，<br>一个 App', sub: '你已经在用的 Claude Code、Codex 和其他 agent：和它们聊、同时跑好几个，在你的哪台电脑上都行。', img: 'home' },
    { id: '02-together', eyebrow: '项目', h: '让 agent 们一起干活', sub: '协调者把活分给 Claude Code 和 Codex，四个并排看，每一步实时可见。', img: 'panes' },
    { id: '03-computer', eyebrow: '你的电脑', h: '你的电脑，agent 也能用', sub: 'agent 在你的电脑和服务器上跑命令，终端里实时看得见。只有你允许的 agent 能用，不用开任何端口。', img: 'terminal' },
    { id: '04-chat', eyebrow: '对话', h: '不只看答案，也看它怎么做', sub: '读了什么、跑了什么、改了什么，收成一行。它干活时随时插话。', img: 'chat' },
    { id: '05-apps', eyebrow: '应用', h: 'agent 也能用的应用', sub: '文档、表格、幻灯片、白板、视频剪辑……你和 agent 在同一个页面上干活。', img: 'apps' },
    { id: '06-automations', eyebrow: '自动化', h: '到点自己开工', sub: '每天早上的汇总、周五的周报、CI 挂了去看一眼：按时间，或者有事件来时。', img: 'automations' },
    { id: '07-phone', phone: true, pill: 'iPhone · TestFlight 公开测试', h: '口袋里的 agent', sub: '每台电脑上的每段对话，手机上都能看。在哪儿都能跟进进度、派下一件活。', url: 'github.com/chenhg5/myco' },
  ],
};
const css = `
*{box-sizing:border-box;margin:0}
body{width:1270px;height:760px;overflow:hidden;font-family:Inter,"Noto Sans CJK SC","Noto Sans SC",sans-serif;-webkit-font-smoothing:antialiased}
.light{background:#f3f1ec;color:#1b1b1a;position:relative;width:100%;height:100%}
.logo{position:absolute;top:52px;right:72px;height:34px}
.eyebrow{position:absolute;top:56px;left:80px;font-size:15px;letter-spacing:.14em;font-weight:600;color:#2f8a69}
.h{position:absolute;top:84px;left:78px;font-size:52px;line-height:1.08;font-weight:700;letter-spacing:-.02em;max-width:1000px}
.sub{position:absolute;left:80px;font-size:21px;line-height:1.45;color:#5c5a55;max-width:900px}
.win{position:absolute;left:130px;right:130px;bottom:-40px;border-radius:14px;overflow:hidden;background:#fff;box-shadow:0 30px 80px rgba(40,35,25,.18),0 0 0 1px rgba(0,0,0,.08)}
.win img{display:block;width:100%}
.dots{position:absolute;top:13px;left:14px;display:flex;gap:8px}
.dots i{width:12px;height:12px;border-radius:50%;display:block}
.dark{background:radial-gradient(120% 120% at 20% 10%,#1d3b33 0%,#0f1715 55%,#0b0f0e 100%);color:#f4f4f2;width:100%;height:100%;position:relative}
.dark .pill{position:absolute;top:96px;left:80px;border:1px solid rgba(255,255,255,.28);border-radius:999px;padding:8px 18px;font-size:17px;color:#e8ece9;background:rgba(255,255,255,.06)}
.dark .h{top:160px;font-size:64px;max-width:620px}
.dark .sub{color:#c6ccc8;max-width:540px;font-size:21px}
.dark .url{position:absolute;left:80px;bottom:150px;font-size:19px;font-weight:600;color:#fff}
.dark .logo{top:auto;bottom:60px;left:80px;right:auto;height:30px}
.phone{position:absolute;width:300px;height:652px;border-radius:52px;background:#0a0a0a;padding:11px;box-shadow:0 0 0 2px #3a3a3a,0 40px 90px rgba(0,0,0,.55)}
.phone img{width:100%;height:100%;border-radius:42px;object-fit:cover;object-position:top;display:block}
`;
(async () => {
  for (const l of ['en', 'zh']) require('fs').mkdirSync(`${M}/out/${l}`, { recursive: true });
  const b = await chromium.launch();
  for (const lang of ['en', 'zh']) {
    for (const s of S[lang]) {
      const p = await b.newPage({ viewport: { width: 1270, height: 760 }, deviceScaleFactor: 2 });
      let body;
      if (s.phone) {
        const ios = lang === 'zh' ? 'zh-Hans' : 'en';
        body = `<div class="dark"><div class="pill">${s.pill}</div><div class="h">${s.h}</div><div class="sub" id="sub">${s.sub}</div><div class="url">${s.url}</div><img class="logo" src="file://${LOGO}/myco-logo-dark.svg">
          <div class="phone" style="left:690px;top:64px"><img src="file://${G}/${ios}-chats.jpg"></div>
          <div class="phone" style="left:930px;top:120px;transform:scale(.96);transform-origin:top left"><img src="file://${G}/${ios}-agent.jpg"></div></div>`;
      } else {
        body = `<div class="light"><div class="eyebrow">${s.eyebrow}</div><div class="h" id="h">${s.h}</div><div class="sub" id="sub">${s.sub}</div><img class="logo" src="file://${LOGO}/myco-logo.svg">
          <div class="win" id="win"><img src="file://${CAP}/${lang}/${s.img}.png"><div class="dots"><i style="background:#ff5f57"></i><i style="background:#febc2e"></i><i style="background:#28c840"></i></div></div></div>`;
      }
      require('fs').writeFileSync(`${M}/slide.html`, `<!doctype html><meta charset=utf-8><style>${css}</style>${body}`); await p.goto(`file://${M}/slide.html`, { waitUntil: 'load' });
      await p.evaluate(() => {
        const h = document.querySelector('.h'), sub = document.getElementById('sub'), win = document.getElementById('win');
        const hb = h.getBoundingClientRect();
        sub.style.top = (hb.bottom + 14) + 'px';
        if (win) win.style.top = (sub.getBoundingClientRect().bottom + 34) + 'px';
      });
      await p.waitForTimeout(300);
      await p.screenshot({ path: `${M}/out/${lang}/${s.id}.png` });
      await p.close(); console.log(lang, s.id);
    }
  }
  await b.close();
})();
