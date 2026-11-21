import { t as uiText } from "../_lib/i18n";
// Scenes to start from. Each is plain HTML + CSS + an animation script, the
// same as an agent writes (public/agent.md): elements marked data-field are
// what people change in the form; sizes are in cqmin / cqw / cqh (parts of
// the scene), so a scene fits 16:9, 9:16 and square alike.

export type SceneTemplate = {
  id: string; name: string; hint: string; duration: number; transition?: string;
  html: string; css: string; script: string; fields?: Record<string, string>;
};

const title: SceneTemplate = {
  id: "title", name: uiText("标题"), hint: uiText("大标题逐字升起"), duration: 4, transition: "fade",
  html: `<div class="glow"></div>
<div class="wrap">
  <div class="kicker" data-field="kicker" data-label="${uiText("小标题")}">NEW · 2026</div>
  <h1 data-field="title" data-label="${uiText("标题")}">${uiText("让 agent 一起做事")}</h1>
  <div class="line"></div>
  <p data-field="subtitle" data-label="${uiText("副标题")}">${uiText("一个地方，看见每个 agent 在做什么")}</p>
</div>`,
  css: `.glow { position: absolute; left: 50%; top: 50%; width: 110cqmin; height: 110cqmin; margin: -55cqmin 0 0 -55cqmin; border-radius: 50%;
  background: radial-gradient(circle, color-mix(in srgb, var(--accent) 30%, transparent), transparent 62%); }
.wrap { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 3.2cqmin; padding: 0 8cqw; text-align: center; }
.kicker { font-size: 2.6cqmin; font-weight: 600; letter-spacing: .42em; color: var(--accent); }
h1 { margin: 0; font-size: 11cqmin; font-weight: 800; line-height: 1.08; letter-spacing: -.02em; }
.line { width: 14cqmin; height: .7cqmin; border-radius: 1cqmin; background: var(--accent); transform-origin: center; }
p { margin: 0; font-size: 3.8cqmin; color: var(--muted); }`,
  script: `const chars = util.split("h1", "chars");
tl.from(".glow", { scale: 0.5, opacity: 0, duration: 1.8, ease: "power2.out" }, 0)
  .from(".kicker", { y: 30, opacity: 0, duration: 0.6 }, 0.2)
  .from(chars, { yPercent: 70, opacity: 0, duration: 0.7, stagger: 0.045, ease: "back.out(1.7)" }, 0.35)
  .from(".line", { scaleX: 0, duration: 0.7, ease: "power3.out" }, ">-0.3")
  .from("p", { y: 24, opacity: 0, duration: 0.7 }, "<0.15")
  .to(".wrap", { scale: 1.04, duration: util.duration, ease: "none" }, 0);`,
};

const punch: SceneTemplate = {
  id: "punch", name: uiText("冲击字"), hint: uiText("几个词一个接一个砸出来"), duration: 3, transition: "cut",
  html: `<div class="w" data-field="w1" data-label="${uiText("第 1 个词")}">${uiText("不用装")}</div>
<div class="w" data-field="w2" data-label="${uiText("第 2 个词")}">${uiText("不用等")}</div>
<div class="w alt" data-field="w3" data-label="${uiText("第 3 个词")}">${uiText("打开就能做")}</div>`,
  css: `.w { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; padding: 0 6cqw; text-align: center;
  font-size: 16cqmin; font-weight: 900; letter-spacing: -.03em; line-height: 1; }
.alt { color: var(--accent); }`,
  script: `const ws = util.$$(".w");
const slot = util.duration / ws.length;
ws.forEach((w, i) => {
  tl.fromTo(w, { scale: 1.6, opacity: 0 }, { scale: 1, opacity: 1, duration: 0.32, ease: "power4.out" }, i * slot);
  if (i < ws.length - 1) tl.to(w, { scale: 0.92, opacity: 0, duration: 0.16, ease: "power1.in" }, (i + 1) * slot - 0.16);
});`,
};

const features: SceneTemplate = {
  id: "features", name: uiText("三个要点"), hint: uiText("标题加三张卡片依次出现"), duration: 5, transition: "slide",
  html: `<div class="wrap">
  <h2 data-field="title" data-label="${uiText("标题")}">${uiText("三件事，一个地方")}</h2>
  <div class="items">
    <div class="item"><b>01</b><h3 data-field="t1" data-label="${uiText("第 1 条")}">${uiText("派活")}</h3><p data-field="d1" data-label="${uiText("第 1 条说明")}">${uiText("把任务发给最合适的 agent")}</p></div>
    <div class="item"><b>02</b><h3 data-field="t2" data-label="${uiText("第 2 条")}">${uiText("盯进度")}</h3><p data-field="d2" data-label="${uiText("第 2 条说明")}">${uiText("谁在做什么，做到哪了")}</p></div>
    <div class="item"><b>03</b><h3 data-field="t3" data-label="${uiText("第 3 条")}">${uiText("拿结果")}</h3><p data-field="d3" data-label="${uiText("第 3 条说明")}">${uiText("做完的东西直接能用")}</p></div>
  </div>
</div>`,
  css: `.wrap { position: absolute; inset: 0; display: flex; flex-direction: column; justify-content: center; gap: 6cqmin; padding: 0 8cqw; }
h2 { margin: 0; font-size: 7cqmin; font-weight: 800; letter-spacing: -.01em; }
.items { display: flex; gap: 3cqmin; }
.item { flex: 1; padding: 4.5cqmin 4cqmin; border-radius: 3cqmin; background: color-mix(in srgb, var(--fg) 6%, transparent);
  box-shadow: inset 0 0 0 .15cqmin color-mix(in srgb, var(--fg) 10%, transparent); }
.item b { display: block; font-size: 2.6cqmin; font-weight: 700; color: var(--accent); letter-spacing: .1em; margin-bottom: 2.4cqmin; }
.item h3 { margin: 0 0 1.4cqmin; font-size: 4.6cqmin; font-weight: 700; }
.item p { margin: 0; font-size: 2.8cqmin; line-height: 1.45; color: var(--muted); }
@container (aspect-ratio < 1) {
  .items { flex-direction: column; }
  .item { padding: 4cqmin 5cqmin; }
  h2 { font-size: 9cqmin; }
}`,
  script: `tl.from("h2", { y: 40, opacity: 0, duration: 0.7, ease: "power3.out" }, 0.1)
  .from(".item", { y: 80, opacity: 0, duration: 0.8, stagger: 0.18, ease: "power3.out" }, 0.45)
  .from(".item b", { x: -20, opacity: 0, duration: 0.5, stagger: 0.18 }, 0.75);`,
};

const stat: SceneTemplate = {
  id: "stat", name: uiText("数据"), hint: uiText("数字滚动，柱子按数值长出来"), duration: 5, transition: "fade",
  html: `<div class="wrap">
  <div class="head">
    <div class="big"><span class="num" data-field="value" data-label="${uiText("大数字")}">12800</span><span class="unit" data-field="unit" data-label="${uiText("单位")}">+</span></div>
    <p data-field="label" data-label="${uiText("说明")}">${uiText("条消息每天在 agent 之间往来")}</p>
  </div>
  <div class="bars">
    <div class="bar"><span class="val" data-field="v1" data-label="${uiText("第 1 项数值")}">1200</span><i></i><em data-field="l1" data-label="${uiText("第 1 项")}">${uiText("1 月")}</em></div>
    <div class="bar"><span class="val" data-field="v2" data-label="${uiText("第 2 项数值")}">3400</span><i></i><em data-field="l2" data-label="${uiText("第 2 项")}">${uiText("2 月")}</em></div>
    <div class="bar"><span class="val" data-field="v3" data-label="${uiText("第 3 项数值")}">6100</span><i></i><em data-field="l3" data-label="${uiText("第 3 项")}">${uiText("3 月")}</em></div>
    <div class="bar"><span class="val" data-field="v4" data-label="${uiText("第 4 项数值")}">12800</span><i></i><em data-field="l4" data-label="${uiText("第 4 项")}">${uiText("4 月")}</em></div>
  </div>
</div>`,
  css: `.wrap { position: absolute; inset: 0; display: flex; align-items: center; gap: 8cqmin; padding: 0 9cqw; }
.head { flex: 1; }
.big { font-size: 15cqmin; font-weight: 800; line-height: 1; letter-spacing: -.03em; }
.unit { color: var(--accent); }
p { margin: 2.6cqmin 0 0; font-size: 3.6cqmin; color: var(--muted); }
.bars { flex: 1; height: 52cqmin; display: flex; align-items: flex-end; gap: 3cqmin; }
.bar { flex: 1; height: 100%; display: flex; flex-direction: column; justify-content: flex-end; align-items: center; gap: 1.4cqmin; }
.bar i { display: block; width: 100%; border-radius: 1.6cqmin 1.6cqmin .4cqmin .4cqmin; transform-origin: bottom;
  background: linear-gradient(180deg, var(--accent), color-mix(in srgb, var(--accent) 45%, transparent)); }
.bar:last-child i { background: linear-gradient(180deg, var(--accent2), color-mix(in srgb, var(--accent2) 45%, transparent)); }
.val { font-size: 2.6cqmin; font-weight: 600; }
em { font-style: normal; font-size: 2.4cqmin; color: var(--muted); }
@container (aspect-ratio < 1) {
  .wrap { flex-direction: column; justify-content: center; align-items: stretch; gap: 10cqmin; }
  .head { flex: none; }
  .bars { flex: none; height: 60cqmin; }
}`,
  script: `const bars = util.$$(".bar");
const vals = bars.map((b) => parseFloat(b.querySelector(".val").textContent.replace(/[^\\d.]/g, "")) || 0);
const max = Math.max(1, ...vals);
// (each bar as tall as its value; room left for the number and the label)
bars.forEach((b, i) => { b.querySelector("i").style.height = (vals[i] / max * 78) + "%"; });
tl.from(".big", { y: 40, opacity: 0, duration: 0.6 }, 0.1)
  .from(".num", { count: 0, duration: 1.8, ease: "power2.out" }, 0.2)
  .from("p", { y: 20, opacity: 0, duration: 0.6 }, 0.5)
  .from(".bar i", { scaleY: 0, duration: 0.9, stagger: 0.12, ease: "power3.out" }, 0.5)
  .from(".val", { count: 0, opacity: 0, duration: 0.9, stagger: 0.12, ease: "power3.out" }, 0.5)
  .from("em", { opacity: 0, duration: 0.4, stagger: 0.12 }, 0.6);`,
};

const quote: SceneTemplate = {
  id: "quote", name: uiText("金句"), hint: uiText("一句话像读出来一样逐字亮起"), duration: 5, transition: "fade",
  html: `<div class="wrap">
  <div class="mark">“</div>
  <blockquote data-field="quote" data-label="${uiText("金句")}">${uiText("好的工具，让人和 agent 都专注在真正重要的事上。")}</blockquote>
  <div class="who"><span class="dash"></span><span data-field="author" data-label="${uiText("署名")}">${uiText("myco 团队")}</span></div>
</div>`,
  css: `.wrap { position: absolute; inset: 0; display: flex; flex-direction: column; justify-content: center; padding: 0 12cqw; }
.mark { font-size: 24cqmin; line-height: .6; height: 10cqmin; font-family: Georgia, serif; color: var(--accent); }
blockquote { margin: 0; font-size: 7cqmin; font-weight: 700; line-height: 1.35; letter-spacing: -.01em; }
.who { display: flex; align-items: center; gap: 2cqmin; margin-top: 5cqmin; font-size: 3.2cqmin; color: var(--muted); }
.dash { width: 6cqmin; height: .3cqmin; background: var(--muted); transform-origin: left; }`,
  script: `const chars = util.split("blockquote", "chars");
tl.from(".mark", { scale: 0.4, opacity: 0, duration: 0.7, ease: "back.out(2)" }, 0)
  .fromTo(chars, { opacity: 0.14 }, { opacity: 1, duration: 0.25, stagger: Math.min(0.06, 2.6 / Math.max(1, chars.length)), ease: "none" }, 0.3)
  .from(".dash", { scaleX: 0, duration: 0.5, ease: "power2.out" }, ">")
  .from(".who span:last-child", { x: -16, opacity: 0, duration: 0.5 }, "<0.1");`,
};

const image: SceneTemplate = {
  id: "image", name: uiText("图片展示"), hint: uiText("截图或产品图缓缓推近，旁边配文字"), duration: 5, transition: "slide",
  html: `<div class="wrap">
  <div class="cap">
    <span class="tag" data-field="tag" data-label="${uiText("标签")}">${uiText("对话")}</span>
    <h2 data-field="title" data-label="${uiText("标题")}">${uiText("所有 agent，一眼看清")}</h2>
    <p data-field="text" data-label="${uiText("说明")}">${uiText("谁在做什么、卡在哪里、做完了什么，都在一页里")}</p>
  </div>
  <div class="pic"><img data-field="image" data-label="${uiText("图片")}" alt=""></div>
</div>`,
  css: `.wrap { position: absolute; inset: 0; display: flex; align-items: center; gap: 6cqmin; padding: 0 7cqw; }
.cap { flex: 0 0 34%; display: flex; flex-direction: column; align-items: flex-start; gap: 2.4cqmin; }
.tag { padding: .8cqmin 2cqmin; border-radius: 99px; font-size: 2.4cqmin; font-weight: 600; color: var(--bg); background: var(--accent); }
h2 { margin: 0; font-size: 6.4cqmin; font-weight: 800; line-height: 1.15; }
p { margin: 0; font-size: 3cqmin; line-height: 1.5; color: var(--muted); }
.pic { flex: 1; height: 72cqh; border-radius: 2.4cqmin; overflow: hidden; background: color-mix(in srgb, var(--fg) 8%, transparent);
  box-shadow: 0 3cqmin 8cqmin rgba(0,0,0,.35), 0 0 0 .15cqmin color-mix(in srgb, var(--fg) 12%, transparent); }
.pic img { width: 100%; height: 100%; object-fit: cover; }
@container (aspect-ratio < 1) {
  .wrap { flex-direction: column-reverse; justify-content: center; align-items: stretch; gap: 6cqmin; padding: 0 7cqw; }
  .pic { flex: none; height: 46cqh; }
  .cap { flex: none; }
  h2 { font-size: 8cqmin; }
  p { font-size: 4cqmin; }
}`,
  script: `tl.from(".pic", { x: 120, opacity: 0, duration: 0.9, ease: "power3.out" }, 0)
  .fromTo(".pic img", { scale: 1.12 }, { scale: 1, duration: util.duration, ease: "power1.out" }, 0)
  .from(".tag", { y: 20, opacity: 0, duration: 0.5 }, 0.3)
  .from("h2", { y: 30, opacity: 0, duration: 0.6, ease: "power3.out" }, 0.4)
  .from("p", { y: 20, opacity: 0, duration: 0.6 }, 0.55);`,
};

const steps: SceneTemplate = {
  id: "steps", name: uiText("三个步骤"), hint: uiText("一条线连起三步，依次点亮"), duration: 5, transition: "fade",
  html: `<div class="wrap">
  <h2 data-field="title" data-label="${uiText("标题")}">${uiText("三步开始")}</h2>
  <div class="steps">
    <div class="track"></div>
    <div class="step"><i>1</i><div><h3 data-field="s1" data-label="${uiText("第 1 步")}">${uiText("装上 App")}</h3><p data-field="p1" data-label="${uiText("第 1 步说明")}">${uiText("Mac 和 Windows 都行")}</p></div></div>
    <div class="step"><i>2</i><div><h3 data-field="s2" data-label="${uiText("第 2 步")}">${uiText("连上 agent")}</h3><p data-field="p2" data-label="${uiText("第 2 步说明")}">${uiText("Claude Code、Codex 一条命令")}</p></div></div>
    <div class="step"><i>3</i><div><h3 data-field="s3" data-label="${uiText("第 3 步")}">${uiText("开始派活")}</h3><p data-field="p3" data-label="${uiText("第 3 步说明")}">${uiText("说一句话，它们就动起来")}</p></div></div>
  </div>
</div>`,
  css: `.wrap { position: absolute; inset: 0; display: flex; flex-direction: column; justify-content: center; gap: 9cqmin; padding: 0 8cqw; }
h2 { margin: 0; font-size: 7cqmin; font-weight: 800; text-align: center; }
.steps { position: relative; display: flex; gap: 4cqmin; }
.track { position: absolute; left: 8%; right: 8%; top: 4.5cqmin; height: .5cqmin; margin-top: -.25cqmin; border-radius: 1cqmin;
  background: linear-gradient(90deg, var(--accent), var(--accent2)); }
.step { position: relative; flex: 1; display: flex; flex-direction: column; align-items: center; text-align: center; gap: 2.4cqmin; }
.step i { display: flex; align-items: center; justify-content: center; width: 9cqmin; height: 9cqmin; border-radius: 50%;
  font-style: normal; font-size: 4cqmin; font-weight: 800; color: var(--bg); background: var(--accent); box-shadow: 0 0 0 1.2cqmin var(--bg); }
.step:last-child i { background: var(--accent2); }
h3 { margin: 0 0 1cqmin; font-size: 4.2cqmin; font-weight: 700; }
p { margin: 0; font-size: 2.8cqmin; color: var(--muted); }
@container (aspect-ratio < 1) {
  .steps { flex-direction: column; gap: 8cqmin; }
  .track { left: 4.5cqmin; right: auto; top: 4%; bottom: 4%; width: .5cqmin; height: auto; margin: 0 0 0 -.25cqmin; }
  .step { flex-direction: row; text-align: left; gap: 4cqmin; }
  h2 { text-align: left; font-size: 9cqmin; }
}`,
  script: `tl.from("h2", { y: 30, opacity: 0, duration: 0.6 }, 0.1)
  .fromTo(".track", { clipPath: "inset(0% 100% 100% 0%)" }, { clipPath: "inset(0% 0% 0% 0%)", duration: 1.8, ease: "power1.inOut" }, 0.5)
  .from(".step i", { scale: 0, duration: 0.5, stagger: 0.6, ease: "back.out(2.2)" }, 0.5)
  .from(".step div", { y: 24, opacity: 0, duration: 0.5, stagger: 0.6 }, 0.7);`,
};

const end: SceneTemplate = {
  id: "end", name: uiText("片尾"), hint: uiText("Logo、名字和一句话，最后定格"), duration: 4, transition: "zoom",
  html: `<div class="ring"></div>
<div class="wrap">
  <img class="logo" data-field="logo" data-label="Logo" data-optional alt="">
  <h1 data-field="name" data-label="${uiText("名字")}">myco</h1>
  <p data-field="tagline" data-label="${uiText("一句话")}">${uiText("让你的 agent 们一起干活")}</p>
  <div class="cta" data-field="cta" data-label="${uiText("网址或按钮")}">myco.dev</div>
</div>`,
  css: `.ring { position: absolute; left: 50%; top: 50%; width: 80cqmin; height: 80cqmin; margin: -40cqmin 0 0 -40cqmin; border-radius: 50%;
  border: .3cqmin solid color-mix(in srgb, var(--accent) 40%, transparent); }
.wrap { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 2.6cqmin; text-align: center; padding: 0 8cqw; }
.logo { width: 14cqmin; height: 14cqmin; object-fit: contain; margin-bottom: 1cqmin; }
h1 { margin: 0; font-size: 13cqmin; font-weight: 800; letter-spacing: -.03em; line-height: 1; }
p { margin: 0; font-size: 3.8cqmin; color: var(--muted); }
.cta { margin-top: 3cqmin; padding: 1.6cqmin 4.4cqmin; border-radius: 99px; font-size: 3.2cqmin; font-weight: 600; color: var(--bg); background: var(--fg); }`,
  script: `tl.fromTo(".ring", { scale: 0.3, opacity: 0 }, { scale: 1.15, opacity: 1, duration: 1.6, ease: "expo.out" }, 0)
  .from(".logo", { scale: 0.5, opacity: 0, duration: 0.7, ease: "back.out(1.8)" }, 0.15)
  .from("h1", { y: 40, opacity: 0, duration: 0.7, ease: "power3.out" }, 0.3)
  .from("p", { y: 20, opacity: 0, duration: 0.6 }, 0.55)
  .from(".cta", { scale: 0.8, opacity: 0, duration: 0.6, ease: "back.out(2)" }, 0.8);`,
};

export const TEMPLATES: SceneTemplate[] = [title, punch, features, stat, quote, image, steps, end];

/** Whole videos to start from: a ratio and the templates of their scenes. */
export const STARTERS: { id: string; name: string; hint: string; ratio: string; scenes: string[] }[] = [
  { id: "promo", name: uiText("产品宣传片"), hint: uiText("标题、要点、数据、截图、步骤、片尾，约 28 秒"), ratio: "16:9", scenes: ["title", "features", "stat", "image", "steps", "end"] },
  { id: "short", name: uiText("竖版短视频"), hint: uiText("冲击字开头，适合抖音、视频号，约 17 秒"), ratio: "9:16", scenes: ["punch", "title", "image", "quote", "end"] },
  { id: "report", name: uiText("数据快报"), hint: uiText("一个结论、一组数据、一句总结"), ratio: "16:9", scenes: ["title", "stat", "quote", "end"] },
];
