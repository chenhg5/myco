import { t as uiText } from "../_lib/i18n";
/** The portable, editable design format. No executable code in a canvas. */
export type Node = {
  id: string;
  kind: "text" | "rect" | "ellipse" | "line" | "image";
  name: string;
  x: number;
  y: number;
  w: number;
  h: number;
  text?: string;
  fill: string;
  size?: number;
  weight?: number;
  radius?: number;
  opacity?: number;
  src?: string;
  hidden?: boolean;
  locked?: boolean;
};
export type Frame = {
  id: string;
  name: string;
  width: number;
  height: number;
  background: string;
  nodes: Node[];
};
export type Artifact = {
  title: string;
  kind: string;
  description: string;
  brief: string;
  status: string;
  frames: Frame[];
  html?: string;
  order: number;
  updated: string;
};
export const PRESETS = [
  {
    id: "website",
    name: uiText("网页"),
    detail: uiText("落地页、官网、交互原型"),
    w: 1440,
    h: 1000,
  },
  {
    id: "app",
    name: uiText("移动应用"),
    detail: uiText("界面、流程、产品概念"),
    w: 390,
    h: 844,
  },
  {
    id: "poster",
    name: uiText("海报"),
    detail: uiText("活动、品牌、宣传物料"),
    w: 900,
    h: 1200,
  },
  {
    id: "slides",
    name: uiText("演示文稿"),
    detail: uiText("提案、发布会、故事讲述"),
    w: 1280,
    h: 720,
  },
  {
    id: "social",
    name: uiText("社交图片"),
    detail: uiText("封面、卡片、内容配图"),
    w: 1080,
    h: 1080,
  },
  {
    id: "dashboard",
    name: uiText("数据看板"),
    detail: uiText("指标、报告、管理界面"),
    w: 1440,
    h: 960,
  },
] as const;
export const uid = () => Math.random().toString(36).slice(2, 10);
export const clone = <T>(x: T): T => JSON.parse(JSON.stringify(x));
const palette = new Set(["none", "transparent"]);
export const color = (s: string | undefined, fallback = "#202020") =>
  s && (/^#[\da-f]{3,8}$/i.test(s) || palette.has(s)) ? s : fallback;
export const esc = (s: unknown) =>
  String(s ?? "").replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\ufffe\uffff\ud800-\udfff]/gu, "").replace(
    /[&<>"']/g,
    (c) =>
      ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&apos;",
      })[c]!,
  );
const num = (x: unknown, fallback = 0) =>
  typeof x === "number" && Number.isFinite(x) ? x : fallback;
export const dimension = (x: unknown) =>
  Math.max(100, Math.min(4096, num(x, 1080)));
export function normalizeFrame(f: Frame): Frame {
  return {
    ...f,
    width: dimension(f.width),
    height: dimension(f.height),
    background: color(f.background, "#ffffff"),
    nodes: Array.isArray(f.nodes)
      ? f.nodes
          .filter(
            (n) =>
              n &&
              ["text", "rect", "ellipse", "line", "image"].includes(n.kind),
          )
          .slice(0, 300)
      : [],
  };
}
export const imageOK = (s?: string) =>
  !!s &&
  /^data:image\/(png|jpeg|webp);base64,[a-z\d+/=\s]+$/i.test(s) &&
  s.length < 220000;
export function svg(f: Frame): string {
  f = normalizeFrame(f);
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${f.width}" height="${f.height}" viewBox="0 0 ${f.width} ${f.height}"><rect width="100%" height="100%" fill="${color(f.background, "#fff")}"/>${f.nodes
    .filter((n) => !n.hidden)
    .map((n) => {
      const x = num(n.x),
        y = num(n.y),
        w = Math.max(1, num(n.w, 100)),
        h = Math.max(1, num(n.h, 40));
      const common = `opacity="${Math.max(0, Math.min(1, num(n.opacity, 1)))}" fill="${color(n.fill)}"`;
      if (n.kind === "rect")
        return `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="${Math.max(0, num(n.radius))}" ${common}/>`;
      if (n.kind === "ellipse")
        return `<ellipse cx="${x + w / 2}" cy="${y + h / 2}" rx="${w / 2}" ry="${h / 2}" ${common}/>`;
      if (n.kind === "line")
        return `<path d="M${x},${y} L${x + w},${y + h}" stroke="${color(n.fill)}" stroke-width="${Math.max(1, num(n.size, 2))}" opacity="${num(n.opacity, 1)}"/>`;
      if (n.kind === "image")
        return imageOK(n.src)
          ? `<image x="${x}" y="${y}" width="${w}" height="${h}" href="${esc(n.src)}" preserveAspectRatio="xMidYMid slice"/>`
          : "";
      const size = Math.max(6, Math.min(400, num(n.size, 24)));
      // Nodes use the top of a line box. An explicit alphabetic baseline also
      // works in WebKit, where text-before-edge can fall back to alphabetic
      // without adjusting y, moving glyphs above the node and its button.
      return `<text x="${x}" y="${y + size}" dominant-baseline="alphabetic" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,PingFang SC,sans-serif" font-size="${size}" font-weight="${num(n.weight, 400)}" ${common}>${String(
        n.text || "",
      )
        .split("\n")
        .map(
          (line, i) =>
            `<tspan x="${x}" dy="${i ? size * 1.25 : 0}">${esc(line)}</tspan>`,
        )
        .join("")}</text>`;
    })
    .join("")}</svg>`;
}
export function documentHTML(a: Artifact) {
  if (a.html) return a.html;
  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${esc(a.title)}</title><style>body{margin:0;background:#eee}section{margin:24px auto;max-width:1440px}svg{width:100%;height:auto;display:block}@media print{body{background:white}section{margin:0;break-after:page;max-width:none}svg{max-height:95vh}}</style></head><body>${a.frames.map((f) => `<section>${svg(f)}</section>`).join("")}</body></html>`;
}
/** A generated prototype gets scripts, but no same-origin, network, forms or popups. */
export function previewHTML(html: string) {
  return `<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: blob:; style-src 'unsafe-inline'; script-src 'unsafe-inline'; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'"><style>body{margin:0}</style></head><body>${html}</body></html>`;
}
export function fresh(kind: string, title: string, brief = ""): Artifact {
  const p = PRESETS.find((p) => p.id === kind) || PRESETS[0];
  return {
    title: title || "未命名设计",
    kind: p.id,
    description: p.detail,
    brief,
    status: "draft",
    frames: [starter(p.id, title || "新的想法")],
    order: Date.now(),
    updated: new Date().toISOString(),
  };
}
export function starter(kind: string, title: string): Frame {
  const p = PRESETS.find((p) => p.id === kind) || PRESETS[0],
    w = p.w,
    h = p.h;
  const nodes: Node[] = [];
  const rect = (
    name: string,
    x: number,
    y: number,
    w: number,
    h: number,
    fill: string,
    radius = 0,
  ) => nodes.push({ id: uid(), kind: "rect", name, x, y, w, h, fill, radius });
  const text = (
    text: string,
    x: number,
    y: number,
    size: number,
    fill = "#222222",
    weight = 400,
  ) =>
    nodes.push({
      id: uid(),
      kind: "text",
      name: text.split("\n")[0].slice(0, 24),
      text,
      x,
      y,
      w: w - x * 2,
      h: size * 1.4 * text.split("\n").length,
      size,
      fill,
      weight,
    });
  if (kind === "poster" || kind === "social") {
    rect("深色画布", 0, 0, w, h, "#171717");
    text("STUDIO / 026", 64, 55, 16, "#aaaaaa");
    text(title, 64, 132, 74, "#f7f7f5", 600);
    text("让想法，\n成为看得见的作品。", 64, 250, 35, "#bcbcbc");
    for (let i = 0; i < 12; i++)
      nodes.push({
        id: uid(),
        kind: "ellipse",
        name: `轨道 ${i + 1}`,
        x: 140 + i * 22,
        y: h * 0.41 + i * 13,
        w: 650 - i * 33,
        h: 470 - i * 21,
        fill: i % 2 ? "#171717" : "#d4d4d0",
      });
    text("DESIGNED TO EXPLORE", 64, h - 110, 17, "#bbbbbb");
    text("myco design studio", 64, h - 72, 14, "#888888");
  } else if (kind === "app") {
    text("9:41", 26, 20, 14, "#222222", 600);
    text("你的灵感收藏", 26, 90, 28, "#222222", 600);
    text("从一个小小的想法开始", 26, 137, 13, "#888888");
    rect("主卡片", 24, 195, w - 48, 250, "#202020", 18);
    text(title, 44, 225, 25, "#ffffff", 600);
    text("收集 · 整理 · 创造", 44, 267, 13, "#aaaaaa");
    for (let i = 0; i < 4; i++) {
      rect(`条目 ${i + 1}`, 24, 478 + i * 66, w - 48, 50, "#f1f1f1", 9);
      text(
        ["最近收集", "进行中的设计", "我的参考库", "已完成的作品"][i],
        42,
        493 + i * 66,
        14,
      );
    }
    rect("底部导航", 24, h - 62, w - 48, 40, "#202020", 20);
    text("工作台           收藏           我的", 50, h - 50, 12, "#ffffff");
  } else if (kind === "slides") {
    text("MYCO / DESIGN NOTES", 64, 50, 17, "#888888");
    text(title, 64, 172, 70, "#222222", 600);
    text("好的设计，让想法自然发生。", 68, 285, 28, "#777777");
    rect("底部装饰", 64, 458, w - 128, 2, "#dddddd");
    text("01 / 概念提案", 64, 500, 18, "#777777");
    text("2026", w - 150, h - 70, 17, "#777777");
    for (let i = 0; i < 8; i++)
      rect(
        `几何 ${i}`,
        w - 375 + i * 25,
        140 + i * 34,
        220 - i * 20,
        220 - i * 20,
        i % 2 ? "#aaaaaa" : "#222222",
        8,
      );
  } else if (kind === "dashboard") {
    rect("侧边栏", 0, 0, 200, h, "#171717");
    text("WORKSPACE", 24, 34, 18, "#ffffff", 600);
    ["概览", "项目", "成员", "设置"].forEach((s, i) =>
      text(s, 28, 125 + i * 54, 15, "#aaaaaa"),
    );
    text(title, 250, 46, 32, "#222222", 600);
    text("把重要的事，放在眼前。", 250, 100, 16, "#888888");
    ["12,480", "86.4%", "1,284"].forEach((s, i) => {
      rect(`指标卡 ${i}`, 250 + i * 370, 180, 340, 155, "#f3f3f3", 12);
      text(
        ["总访问量", "完成率", "活跃用户"][i],
        275 + i * 370,
        206,
        15,
        "#777777",
      );
      text(s, 275 + i * 370, 245, 38, "#222222", 600);
    });
    rect("图表背景", 250, 380, 1080, 420, "#f5f5f5", 12);
    text("每周趋势", 280, 410, 20, "#222222", 500);
    for (let i = 0; i < 12; i++)
      rect(
        `柱 ${i}`,
        290 + i * 80,
        715 - [55, 95, 75, 160, 120, 210, 185, 245, 205, 280, 255, 310][i],
        42,
        [55, 95, 75, 160, 120, 210, 185, 245, 205, 280, 255, 310][i],
        i === 11 ? "#222222" : "#bbbbbb",
        4,
      );
  } else {
    text("FORM / STUDIO", 72, 44, 20, "#222222", 600);
    text("作品         关于         联系", w - 420, 48, 15, "#777777");
    rect("分隔线", 72, 98, w - 144, 1, "#dddddd");
    text("把日常，\n设计成喜欢的样子。", 72, 190, 78, "#222222", 600);
    text(title, 77, 424, 23, "#777777");
    rect("行动按钮", 76, 495, 176, 52, "#222222", 26);
    text("一起创造  ↗", 110, 509, 18, "#ffffff");
    for (let i = 0; i < 9; i++)
      nodes.push({
        id: uid(),
        kind: "ellipse",
        name: `空间 ${i}`,
        x: 820 + i * 16,
        y: 170 + i * 22,
        w: 460 - i * 33,
        h: 460 - i * 33,
        fill: i % 2 ? "#e6e6e3" : "#333333",
      });
    rect("底部内容", 72, 715, w - 144, 190, "#f3f3f1", 16);
    text("01 / 少一点，更进一步", 108, 750, 28, "#222222", 500);
    text("从需求到作品，在同一个空间里，不断尝试。", 108, 806, 18, "#888888");
  }
  return {
    id: uid(),
    name: kind === "slides" ? "封面" : "主画布",
    width: w,
    height: h,
    background: "#ffffff",
    nodes,
  };
}
