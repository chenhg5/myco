import createDOMPurify from "dompurify";
import { L, E } from "@/lib/i18n";

/*
 * What a message's Markdown can hold beyond text (Markdown.tsx draws the
 * HTML, this brings parts of it to life after it is in the page):
 *
 * - ```mermaid blocks: architecture, flow, sequence, ER diagrams... drawn
 *   as pictures. The library (large) loads only when a message has one; a
 *   diagram that does not parse keeps showing its source, with why.
 * - Code blocks with a language: coloured (highlight.js, loaded on first use).
 *
 * Both run on the sanitized HTML, never on raw text: mermaid in its strict
 * mode (no scripts, no clicks, no HTML labels) and its SVG sanitized again.
 */

// Our own sanitizer: mermaid, loaded in the same bundle, adds hooks and
// settings to the shared DOMPurify while it draws.
const DOMPurify = createDOMPurify(window);

const isDark = () => document.documentElement.classList.contains("dark");

// ---------------------------------------------------------------------------
// Diagrams

type Mermaid = typeof import("mermaid")["default"];
let mermaidP: Promise<Mermaid> | null = null;
let mermaidTheme = "";
function loadMermaid(): Promise<Mermaid> {
  mermaidP ||= import("mermaid").then((m) => m.default);
  return mermaidP.then((m) => {
    const theme = isDark() ? "dark" : "neutral";
    if (theme !== mermaidTheme) {
      mermaidTheme = theme;
      m.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme,
        // (named, not "inherit": the picture opened large stands alone)
        fontFamily: '-apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", "Segoe UI", sans-serif',
        htmlLabels: false,
        flowchart: { htmlLabels: false },
      });
    }
    return m;
  });
}

/** Drawn diagrams by source and theme: a message drawn again (it grew, the
 *  list moved) shows its picture at once instead of flashing the source. */
const drawn = new Map<string, string>();
let seq = 0;
let queue: Promise<unknown> = Promise.resolve();

function cleanSvg(svg: string): string {
  // (the default rules keep SVG and its <style>; no scripts, no handlers,
  // no HTML inside it)
  return DOMPurify.sanitize(svg, { FORBID_TAGS: ["foreignObject", "script"], FORBID_ATTR: ["onclick", "onload", "onerror"] });
}

async function drawOne(box: HTMLElement): Promise<void> {
  const src = decodeURIComponent(box.dataset.src || "");
  if (!src.trim()) return;
  const key = `${isDark() ? "d" : "l"}\u0000${src}`;
  let svg = drawn.get(key);
  let id = "";
  if (!svg) {
    try {
      const m = await loadMermaid();
      // One at a time: mermaid's render is not safe to run side by side.
      id = `md-mmd-${++seq}`;
      const rid = id;
      const run = queue.then(() => m.render(rid, src));
      queue = run.catch(() => undefined);
      const out = await run;
      svg = cleanSvg(out.svg);
      drawn.set(key, svg);
    } catch (e) {
      // (mermaid leaves this render's error drawing in the page: removed)
      if (id) document.querySelectorAll(`#d${id}, body > #${id}`).forEach((n) => n.remove());
      box.dataset.state = "error";
      // (its source shown at once)
      box.querySelector("details.md-mermaid-src")?.setAttribute("open", "");
      const why = box.querySelector(".md-mermaid-why");
      if (why) why.textContent = L(E`这张图没画出来：${firstLine(e)}`, E`This diagram could not be drawn: ${firstLine(e)}`);
      return;
    }
  }
  if (!box.isConnected) return;
  const pic = box.querySelector<HTMLElement>(".md-mermaid-pic");
  if (!pic) return;
  pic.innerHTML = svg;
  const el = pic.querySelector("svg");
  if (el) {
    el.removeAttribute("height");
    el.setAttribute("role", "img");
    el.setAttribute("aria-label", L("图", "Diagram"));
  }
  box.dataset.state = "drawn";
}

function firstLine(e: unknown): string {
  const s = e instanceof Error ? e.message : String(e);
  return s.split("\n").find((l) => l.trim())?.trim().slice(0, 160) || "";
}

/** Draws the diagrams in el (each once per source and theme). */
export function drawDiagrams(el: HTMLElement): void {
  el.querySelectorAll<HTMLElement>(".md-mermaid:not([data-state=drawn]):not([data-state=error])").forEach((b) => { void drawOne(b); });
}

/** A diagram as a picture you can open large (the lightbox): its SVG. */
export function diagramPicture(box: HTMLElement): string | null {
  const svg = box.querySelector(".md-mermaid-pic svg");
  if (!svg) return null;
  const c = svg.cloneNode(true) as SVGElement;
  const vb = c.getAttribute("viewBox")?.split(/\s+/).map(Number);
  if (vb && vb.length === 4) { c.setAttribute("width", String(Math.round(vb[2] * 2))); c.setAttribute("height", String(Math.round(vb[3] * 2))); }
  c.setAttribute("xmlns", "http://www.w3.org/2000/svg");
  // On the theme's own background, so a dark diagram stays readable alone.
  const bg = getComputedStyle(box).getPropertyValue("--bg").trim() || (isDark() ? "#1b1b1f" : "#ffffff");
  c.setAttribute("style", `background:${bg}`);
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(new XMLSerializer().serializeToString(c))}`;
}

/** The theme changed: the diagrams are drawn again in its colours. */
export function onThemeChange(f: () => void): () => void {
  const mo = new MutationObserver(() => f());
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => mo.disconnect();
}
export function redrawDiagrams(el: HTMLElement): void {
  el.querySelectorAll<HTMLElement>(".md-mermaid[data-state=drawn]").forEach((b) => { b.dataset.state = ""; });
  drawDiagrams(el);
}

// ---------------------------------------------------------------------------
// Code colours

type Hljs = typeof import("highlight.js/lib/common")["default"];
let hljsP: Promise<Hljs> | null = null;

/** Colours the code blocks in el that name a language highlight.js knows. */
export function colourCode(el: HTMLElement): void {
  const blocks = [...el.querySelectorAll<HTMLElement>("pre > code[class*='language-']:not([data-hl])")]
    .filter((c) => !c.closest(".md-mermaid"));
  if (!blocks.length) return;
  hljsP ||= import("highlight.js/lib/common").then((m) => m.default);
  void hljsP.then((hljs) => {
    for (const c of blocks) {
      if (!c.isConnected || c.dataset.hl) continue;
      const lang = /language-([\w+#.-]+)/.exec(c.className)?.[1]?.toLowerCase();
      c.dataset.hl = "1";
      if (!lang || !hljs.getLanguage(lang) || (c.textContent || "").length > 60_000) continue;
      // (its own escaped HTML, from the text alone)
      c.innerHTML = hljs.highlight(c.textContent || "", { language: lang, ignoreIllegals: true }).value;
      c.classList.add("hljs");
    }
  }).catch(() => undefined);
}
