import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { knownSize, noteSize } from "@/app/imageSizes";
import createDOMPurify from "dompurify";
import { Marked } from "marked";
import { ChevronDown } from "lucide-react";
import { copyText } from "@/app/copy";
import { cn } from "@/lib/utils";
import { L } from "@/lib/i18n";
import type { Attachment } from "@/api";
import { appApi } from "@/appapi";
import { Lightbox, type Picture } from "@/app/lightbox";
import { colourCode, diagramPicture, drawDiagrams, onThemeChange, redrawDiagrams } from "@/app/mdRich";

/** Rough line budget for one message before it starts collapsed. */
const COLLAPSE_LINES = 30;

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function escapeAttr(s: string): string {
  return escapeHtml(s).replace(/`/g, "&#96;");
}

/** Width in columns: CJK and other wide characters count two. */
function displayWidth(s: string): number {
  let w = 0;
  for (const ch of s) w += /[\u1100-\u115f\u2e80-\ua4cf\uac00-\ud7a3\uf900-\ufaff\ufe30-\ufe4f\uff00-\uff60\uffe0-\uffe6]/.test(ch) ? 2 : 1;
  return w;
}

// A bare link ends at Chinese (and other full-width) punctuation: GFM ends
// one only at a space, so "https://a，https://b" read as one link. (Where
// there is none, marked's own rule decides.)
const WIDE_PUNCT = /[\u3000-\u303f\uff00-\uffef\u2018-\u201f\u2026]/;
export function bareLinkEnd(src: string): string | null {
  const m = /^(?:https?:\/\/|www\.)[^\s<]+/i.exec(src);
  if (!m) return null;
  const cut = m[0].search(WIDE_PUNCT);
  if (cut < 0) return null;
  // what GFM leaves out at the end: trailing punctuation, an unmatched ")"
  let raw = m[0].slice(0, cut).replace(/[?!.,:;*_~'"]+$/, "");
  while (raw.endsWith(")") && (raw.match(/\(/g) || []).length < (raw.match(/\)/g) || []).length) raw = raw.slice(0, -1);
  return /^(?:https?:\/\/[^/?#\s]|www\.\S)/i.test(raw) ? raw : null;
}

/** A message's own files, for pictures its text places with
 *  ![words](attachment:<file id or name>). */
export type InlineFiles = { hubId: string; items: Attachment[] };
// (the files of the message being parsed: md.parse is synchronous)
let parsing: InlineFiles | undefined;

const ATTACH_REF = /attachment:([^)\s"']+)/g;
function findAttachment(files: InlineFiles | undefined, ref: string): Attachment | undefined {
  let r = ref;
  try { r = decodeURIComponent(ref); } catch { /* as written */ }
  return files?.items.find((a) => a.file_id === r) ?? files?.items.find((a) => a.name === r);
}
/** The files a body shows in its text (so they are not listed again below). */
export function inlineAttachmentIds(body: string, files: InlineFiles): Set<string> {
  const ids = new Set<string>();
  for (const m of body.matchAll(ATTACH_REF)) {
    const a = findAttachment(files, m[1]);
    if (a) ids.add(a.file_id);
  }
  return ids;
}

function codeBlock(text: string, lang?: string): string {
  const langClass = lang ? ` class="language-${escapeAttr(lang)}"` : "";
  return (
    `<div class="md-code">` +
    `<button type="button" class="md-code-copy" data-code="${encodeURIComponent(text)}">${L("复制", "Copy")}</button>` +
    `<pre><code${langClass}>${escapeHtml(text)}</code></pre>` +
    `</div>`
  );
}

// Code blocks carry their own copy button so every block gets one, without
// mounting a React tree inside the rendered HTML. Links open outside the App.
const md = new Marked({
  gfm: true,
  breaks: true,
  tokenizer: {
    url(src) {
      const raw = bareLinkEnd(src);
      if (!raw) return false;
      const href = /^www\./i.test(raw) ? `http://${raw}` : raw;
      return { type: "link", raw, text: raw, href, tokens: [{ type: "text", raw, text: raw }] };
    },
  },
  renderer: {
    // A checklist's boxes, drawn as the plan card's (no form control).
    checkbox({ checked }) {
      return `<span class="md-check ${checked ? "is-done" : "is-todo"}"></span>`;
    },
    code({ text, lang }) {
      // A diagram (mdRich draws it): its source stays one click away, and
      // is what shows where it cannot be drawn.
      if ((lang || "").trim().toLowerCase() === "mermaid") {
        return (
          `<div class="md-mermaid" data-src="${encodeURIComponent(text)}">` +
          `<div class="md-mermaid-pic" title="${escapeAttr(L("点一下看大图", "Click to enlarge"))}"><span class="md-mermaid-wait">${L("正在画图…", "Drawing…")}</span></div>` +
          `<div class="md-mermaid-why"></div>` +
          `<details class="md-mermaid-src"><summary>${L("源码", "Source")}</summary>${codeBlock(text, "mermaid")}</details>` +
          `</div>`
        );
      }
      return codeBlock(text, lang);
    },
    // A picture of the message's own (attachment:…) shows from its file.
    image({ href, title, text }) {
      let src = href;
      let att = "";
      if (/^attachment:/i.test(href)) {
        const a = findAttachment(parsing, href.slice("attachment:".length));
        if (!a || !parsing) return `<span class="md-img-missing">${escapeHtml(L(`没找到附件：${text || href}`, `Attachment not found: ${text || href}`))}</span>`;
        src = appApi.attachmentUrl(parsing.hubId, a.file_id);
        att = ` data-att="${escapeAttr(a.file_id)}"`;
      }
      const titleAttr = title ? ` title="${escapeAttr(title)}"` : "";
      return `<img src="${escapeAttr(src)}" alt="${escapeAttr(text || "")}"${titleAttr}${att} loading="lazy">`;
    },
    // Pictures side by side in one paragraph (two or more, nothing else):
    // a gallery.
    paragraph({ tokens }) {
      const pics = tokens.filter((t) => t.type === "image");
      const rest = tokens.filter((t) => t.type !== "image" && !(t.type === "text" && !t.raw.trim()) && t.type !== "br");
      const inner = this.parser.parseInline(tokens);
      if (pics.length >= 2 && rest.length === 0) {
        // With titles (![name](src "a line on it")): each with its name and
        // line under it, as a set of things to compare.
        const titled = pics.some((t) => t.type === "image" && t.title);
        const cells = titled
          ? pics.map((t) => {
            const im = t as { text?: string; title?: string | null };
            return `<figure class="md-fig">${this.parser.parseInline([t])}<figcaption>${im.text ? `<b>${escapeHtml(im.text)}</b>` : ""}${im.title ? `<span>${escapeHtml(im.title)}</span>` : ""}</figcaption></figure>`;
          }).join("")
          : this.parser.parseInline(pics);
        return `<div class="md-gallery${titled ? " is-captioned" : ""}" data-n="${Math.min(pics.length, 4)}">${cells}</div>\n`;
      }
      return `<p>${inner}</p>\n`;
    },
    // Short cells (a status, a number, a name) stay on one line: a narrow
    // column would otherwise break 被拒绝 into 被拒 / 绝.
    tablecell(token) {
      const content = this.parser.parseInline(token.tokens);
      const type = token.header ? "th" : "td";
      const short = displayWidth(token.text) <= 14 ? ` class="md-nw"` : "";
      const align = token.align ? ` align="${token.align}"` : "";
      return `<${type}${align}${short}>${content}</${type}>\n`;
    },
    link({ href, title, tokens }) {
      const text = this.parser.parseInline(tokens);
      const titleAttr = title ? ` title="${escapeAttr(title)}"` : "";
      return `<a href="${escapeAttr(href)}"${titleAttr} target="_blank" rel="noopener noreferrer">${text}</a>`;
    },
  },
});

// Our own sanitizer, not the shared one: mermaid (mdRich), in the same
// bundle, adds hooks and settings to that one while it draws.
const DOMPurify = createDOMPurify(window);

/** A list item that starts with a picture (- ![logo](…) **[name](…)** what it
 *  is): a row, the picture on the left and its words beside it. */
function mediaRows(html: string): string {
  return html.replace(/<li>(\s*(?:<p>)?\s*)<img /g, '<li class="md-media">$1<img class="md-media-pic" ');
}

function sanitize(raw: string): string {
  return DOMPurify.sanitize(raw, {
    ADD_ATTR: ["target"],
    // Inline styles and form controls are never part of a message, and both
    // would let a peer change how the console itself looks.
    FORBID_TAGS: ["style", "form", "input", "textarea", "select"],
    FORBID_ATTR: ["style"],
  });
}

function onCopyClick(e: React.MouseEvent<HTMLDivElement>) {
  const target = e.target as HTMLElement;
  const btn = target.closest<HTMLButtonElement>("button.md-code-copy");
  if (!btn) return;
  const code = decodeURIComponent(btn.dataset.code || "");
  const label = btn.textContent;
  void copyText(code).then(() => {
    btn.textContent = L("已复制", "Copied");
    window.setTimeout(() => { btn.textContent = label || L("复制", "Copy"); }, 1500);
  });
}

/** "- [~] step" (being done) is not a Markdown task: it gets the box of a
 *  step in hand, outside code blocks. */
function doingItems(text: string): string {
  if (!text.includes("[~]") && !text.includes("[-]")) return text;
  let fence = false;
  return text.split("\n").map((l) => {
    if (/^\s*(```|~~~)/.test(l)) fence = !fence;
    return fence ? l : l.replace(/^(\s*(?:[-*+]|\d+[.)])\s+)\[(~|-)\]\s+/, '$1<span class="md-check is-doing"></span> ');
  }).join("\n");
}

/** Rendered, sanitized markdown. Raw HTML is allowed through marked but
 *  DOMPurify removes scripts, event handlers and unsafe URLs. */
/** growing: the text is still coming in (Reveal): diagrams wait for all of it.
 *  zoom false: the page around it opens pictures itself (the user guide). */
export function Markdown({ text, files, growing, zoom = true }: { text: string; files?: InlineFiles; growing?: boolean; zoom?: boolean }) {
  const filesKey = files ? `${files.hubId}\u0000${files.items.map((a) => a.file_id).join(",")}` : "";
  const html = useMemo(() => {
    parsing = files;
    try { return sanitize(mediaRows(md.parse(doingItems(text), { async: false }) as string)); } finally { parsing = undefined; }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text, filesKey]);
  const ref = useRef<HTMLDivElement>(null);
  // Diagrams and code colours (mdRich), after the HTML is in; diagrams
  // again in the other theme's colours when it changes.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (!growing) drawDiagrams(el);
    colourCode(el);
    if (!el.querySelector(".md-mermaid")) return;
    return onThemeChange(() => redrawDiagrams(el));
  }, [html, growing]);
  // A picture or a diagram, clicked: large, with the message's others.
  const [big, setBig] = useState<{ items: Picture[]; index: number } | null>(null);
  const onClick = (e: React.MouseEvent<HTMLDivElement>) => {
    onCopyClick(e);
    const t = e.target as HTMLElement;
    const el = ref.current;
    if (!zoom || !el || t.closest("a")) return;
    const pic = t.closest<HTMLElement>(".md-mermaid[data-state=drawn] .md-mermaid-pic");
    const img = t instanceof HTMLImageElement ? t : null;
    if (!pic && !img) return;
    const items: Picture[] = [];
    let index = 0;
    el.querySelectorAll<HTMLElement>("img, .md-mermaid[data-state=drawn]").forEach((n) => {
      if (n.closest("a")) return;
      if (n instanceof HTMLImageElement) {
        if (!n.naturalWidth) return;
        if (n === img) index = items.length;
        const att = n.dataset.att;
        items.push({ name: n.alt || L("图片", "Picture"), src: n.currentSrc || n.src, ...(att && files ? { save: () => appApi.saveAttachment(files.hubId, att) } : {}) });
      } else {
        const src = diagramPicture(n);
        if (!src) return;
        if (pic && n.contains(pic)) index = items.length;
        items.push({ name: L("图", "Diagram"), src });
      }
    });
    if (items.length) setBig({ items, index });
  };
  // Before it is painted: pictures seen before get their size (the box is
  // there before they load: imageSizes); each one loading notes its size.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.querySelectorAll("img").forEach((img) => {
      const known = knownSize(img.getAttribute("src") || "");
      if (known && !img.getAttribute("width")) { img.width = known[0]; img.height = known[1]; }
    });
    const onLoad = (e: Event) => {
      const img = e.target;
      if (!(img instanceof HTMLImageElement) || !img.naturalWidth) return;
      noteSize(img.dataset.src0 || img.getAttribute("src") || "", img.naturalWidth, img.naturalHeight);
    };
    el.addEventListener("load", onLoad, true);
    return () => el.removeEventListener("load", onLoad, true);
  }, [html]);
  // An image's error event does not bubble, so it is caught on the way down.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const replace = (img: EventTarget | null) => {
      if (!(img instanceof HTMLImageElement)) return;
      // A slow or dropped load: tried again twice, a little later each time,
      // before saying so.
      if (retryImage(img)) return;
      const note = document.createElement("button");
      note.type = "button";
      note.className = "md-img-missing";
      note.textContent = img.alt ? L(`图片没加载出来：${img.alt} · 点一下重试`, `Image didn't load: ${img.alt} · click to retry`) : L("图片没加载出来 · 点一下重试", "Image didn't load · click to retry");
      note.title = img.currentSrc || img.src;
      note.onclick = (e) => { e.stopPropagation(); img.dataset.tries = "0"; img.src = withRetry(img.dataset.src0 || img.src, Date.now()); note.replaceWith(img); };
      img.replaceWith(note);
    };
    const onError = (e: Event) => replace(e.target);
    // Ones that had already failed before this ran.
    el.querySelectorAll("img").forEach((img) => { if (img.complete && img.naturalWidth === 0) replace(img); });
    el.addEventListener("error", onError, true);
    return () => el.removeEventListener("error", onError, true);
  }, [html]);
  return (
    <>
      <div ref={ref} className="md" onClick={onClick} dangerouslySetInnerHTML={{ __html: html }} />
      {big && <Lightbox items={big.items} index={big.index} onIndex={(i) => setBig({ ...big, index: i })} onClose={() => setBig(null)} />}
    </>
  );
}

export interface ExecResult {
  exit_code?: number;
  stdout?: string;
  stderr?: string;
  duration_ms?: number;
  [key: string]: unknown;
}

function isExec(v: object): v is ExecResult {
  return "exit_code" in v || "stdout" in v || "stderr" in v;
}

function duration(ms?: number): string {
  if (typeof ms !== "number" || ms < 0) return "";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

function ExecBlock({ data }: { data: ExecResult }) {
  const code = typeof data.exit_code === "number" ? data.exit_code : undefined;
  const ok = code === 0;
  const failed = typeof code === "number" && code !== 0;
  const took = duration(data.duration_ms);
  return (
    <div className="md-exec">
      <div className="md-exec-head">
        <span className={cn("md-exit", ok && "ok", failed && "bad")}>
          {typeof code === "number" ? L(`退出码 ${code}`, `Exit code ${code}`) : L("已结束", "Finished")}
        </span>
        {took && <span className="md-exec-meta">{took}</span>}
      </div>
      {typeof data.stdout === "string" && data.stdout !== "" && (
        <div className="md-exec-stream">
          <div className="md-exec-label">stdout</div>
          <pre>{data.stdout}</pre>
        </div>
      )}
      {typeof data.stderr === "string" && data.stderr !== "" && (
        <div className="md-exec-stream is-err">
          <div className="md-exec-label">stderr</div>
          <pre>{data.stderr}</pre>
        </div>
      )}
      {typeof data.stdout !== "string" && typeof data.stderr !== "string" && (
        <pre className="md-exec-raw">{JSON.stringify(data, null, 2)}</pre>
      )}
    </div>
  );
}

function PrettyJSON({ data }: { data: unknown }) {
  const text = JSON.stringify(data, null, 2);
  const lines = text.split("\n").length;
  return (
    <details className="md-json" open={lines <= 24}>
      <summary>
        <ChevronDown />
        <span>{L("JSON 数据", "JSON data")}</span>
        <span className="md-json-meta">{L(`${lines} 行`, `${lines} line${lines === 1 ? "" : "s"}`)}</span>
      </summary>
      <pre>{text}</pre>
    </details>
  );
}

function parseJSON(body: string): { ok: true; value: unknown } | null {
  const t = body.trim();
  if (!t || (t[0] !== "{" && t[0] !== "[")) return null;
  try {
    const value: unknown = JSON.parse(t);
    if (value && typeof value === "object") return { ok: true, value };
  } catch { /* plain text that happens to start with a brace */ }
  return null;
}

/** The command result in an exec node's reply, or null. */
export function parseExecResult(body: string): ExecResult | null {
  const p = parseJSON(body);
  if (p && !Array.isArray(p.value) && isExec(p.value as object)) return p.value as ExecResult;
  return null;
}

export { duration as execDuration };

function RenderedBody({ body, files, growing }: { body: string; files?: InlineFiles; growing?: boolean }) {
  const parsed = parseJSON(body);
  if (parsed) {
    if (!Array.isArray(parsed.value) && isExec(parsed.value as object)) {
      return <ExecBlock data={parsed.value as ExecResult} />;
    }
    return <PrettyJSON data={parsed.value} />;
  }
  return <Markdown text={body} files={files} growing={growing} />;
}

/** Long bodies unfolded in this window (MessageContent). */
const unfolded = new Set<string>();

/**
 * A message body: markdown, an exec result, or a JSON object. Bodies longer
 * than 30 lines start folded so one long reply cannot take over the thread.
 */
export function MessageContent({ body, foldId, files, growing }: { body: string; foldId?: string; files?: InlineFiles; growing?: boolean }) {
  const lines = body.split("\n").length;
  const collapsible = lines > COLLAPSE_LINES;
  // Unfolded stays unfolded while this window is open, even when the
  // message is drawn anew (a reply that finished growing, one shown
  // translated, a list that changed above it): kept by its message (else
  // its text).
  // (both: what was streamed in a run is then drawn as its reply, another
  // message with the same words)
  const [open, setOpenState] = useState(() => unfolded.has(body) || (!!foldId && unfolded.has(foldId)));
  const setOpen = (f: (o: boolean) => boolean) => setOpenState((o) => {
    const n = f(o);
    for (const k of foldId ? [body, foldId] : [body]) {
      if (n) unfolded.add(k);
      else unfolded.delete(k);
    }
    return n;
  });
  if (!collapsible) return <RenderedBody body={body} files={files} growing={growing} />;
  return (
    <div className="md-fold">
      <div className={cn("md-fold-body", !open && "is-collapsed")}>
        <RenderedBody body={body} files={files} growing={growing} />
      </div>
      <button type="button" className="md-fold-btn" onClick={() => setOpen((o) => !o)}>
        {open ? L("收起", "Show less") : L(`展开（共 ${lines} 行）`, `Show all (${lines} lines)`)}
      </button>
    </div>
  );
}

/** An image URL loaded afresh (http and the App's own only). */
export function withRetry(src: string, n: number): string {
  if (!/^(https?:|\/|app\/)/.test(src) && !src.startsWith(location.origin)) return src;
  return `${src}${src.includes("?") ? "&" : "?"}_r=${n}`;
}
/** Tries a failed image again (twice, after 1.2s then 2.4s): true while it does. */
export function retryImage(img: HTMLImageElement): boolean {
  const tries = Number(img.dataset.tries || "0");
  if (tries >= 2) return false;
  if (!img.dataset.src0) img.dataset.src0 = img.src;
  img.dataset.tries = String(tries + 1);
  window.setTimeout(() => { img.src = withRetry(img.dataset.src0!, tries + 1); }, 1200 * (tries + 1));
  return true;
}
