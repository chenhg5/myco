import { useEffect, useMemo, useRef, useState } from "react";
import DOMPurify from "dompurify";
import { Marked } from "marked";
import { ChevronDown } from "lucide-react";
import { copyText } from "@/app/copy";
import { cn } from "@/lib/utils";
import { L } from "@/lib/i18n";

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

// Code blocks carry their own copy button so every block gets one, without
// mounting a React tree inside the rendered HTML. Links open outside the App.
const md = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    // A checklist's boxes, drawn as the plan card's (no form control).
    checkbox({ checked }) {
      return `<span class="md-check ${checked ? "is-done" : "is-todo"}"></span>`;
    },
    code({ text, lang }) {
      const langClass = lang ? ` class="language-${escapeAttr(lang)}"` : "";
      return (
        `<div class="md-code">` +
        `<button type="button" class="md-code-copy" data-code="${encodeURIComponent(text)}">${L("复制", "Copy")}</button>` +
        `<pre><code${langClass}>${escapeHtml(text)}</code></pre>` +
        `</div>`
      );
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
export function Markdown({ text }: { text: string }) {
  const html = useMemo(() => sanitize(md.parse(doingItems(text), { async: false }) as string), [text]);
  const ref = useRef<HTMLDivElement>(null);
  // An image's error event does not bubble, so it is caught on the way down.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const replace = (img: EventTarget | null) => {
      if (!(img instanceof HTMLImageElement)) return;
      const note = document.createElement("span");
      note.className = "md-img-missing";
      note.textContent = img.alt ? L(`图片加载不了：${img.alt}`, `Image didn't load: ${img.alt}`) : L("图片加载不了", "Image didn't load");
      note.title = img.currentSrc || img.src;
      img.replaceWith(note);
    };
    const onError = (e: Event) => replace(e.target);
    // Ones that had already failed before this ran.
    el.querySelectorAll("img").forEach((img) => { if (img.complete && img.naturalWidth === 0) replace(img); });
    el.addEventListener("error", onError, true);
    return () => el.removeEventListener("error", onError, true);
  }, [html]);
  return <div ref={ref} className="md" onClick={onCopyClick} dangerouslySetInnerHTML={{ __html: html }} />;
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

function RenderedBody({ body }: { body: string }) {
  const parsed = parseJSON(body);
  if (parsed) {
    if (!Array.isArray(parsed.value) && isExec(parsed.value as object)) {
      return <ExecBlock data={parsed.value as ExecResult} />;
    }
    return <PrettyJSON data={parsed.value} />;
  }
  return <Markdown text={body} />;
}

/**
 * A message body: markdown, an exec result, or a JSON object. Bodies longer
 * than 30 lines start folded so one long reply cannot take over the thread.
 */
export function MessageContent({ body }: { body: string }) {
  const lines = body.split("\n").length;
  const collapsible = lines > COLLAPSE_LINES;
  const [open, setOpen] = useState(false);
  if (!collapsible) return <RenderedBody body={body} />;
  return (
    <div className="md-fold">
      <div className={cn("md-fold-body", !open && "is-collapsed")}>
        <RenderedBody body={body} />
      </div>
      <button type="button" className="md-fold-btn" onClick={() => setOpen((o) => !o)}>
        {open ? L("收起", "Show less") : L(`展开（共 ${lines} 行）`, `Show all (${lines} lines)`)}
      </button>
    </div>
  );
}
