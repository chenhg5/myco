import { createContext, useContext, useEffect, useMemo, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { Check, Download, Maximize2, Minimize2, X } from "lucide-react";
import { appApi } from "@/appapi";
import type { Attachment, Message } from "@/api";
import { Markdown } from "@/app/Markdown";
import { bytes } from "@/app/attach";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/*
 * A file someone sent, read in the App: Markdown rendered as a document,
 * other text (a log, code, JSON) as it is. A file sent again under the same
 * name is its next version: its row says 第 2 版, and the preview can show
 * what changed since the one before. Anything that is not text is saved to
 * Downloads instead.
 *
 * It opens in a panel from the right, as a member's card does, at its full
 * height from the start (nothing grows or jumps as the file arrives), and
 * floats over the page, so a split view keeps both its columns. Full screen
 * (the button, or a double click on its head) gives it the whole window;
 * the choice is kept for the next file.
 */

const TEXT_TYPE = /^(text\/|application\/(json|xml|x-yaml|yaml|toml|x-sh|javascript|typescript|x-ndjson))/;
const TEXT_EXT = /\.(md|markdown|txt|text|log|json|jsonl|ya?ml|toml|csv|tsv|ini|conf|cfg|env|go|ts|tsx|js|jsx|mjs|cjs|py|rb|rs|java|kt|swift|c|h|cc|cpp|hpp|cs|php|sh|bash|zsh|ps1|sql|html?|css|scss|xml|diff|patch|lock)$/i;
const MARKDOWN = /\.(md|markdown)$/i;

/** Whether a file can be read in the App as text. */
export function isText(a: Attachment): boolean {
  const t = (a.content_type || "").split(";")[0].trim().toLowerCase();
  return TEXT_TYPE.test(t) || TEXT_EXT.test(a.name);
}

export function isHtml(a: Attachment): boolean {
  return /\.html?$/i.test(a.name) || /^text\/html\b/i.test(a.content_type || "");
}

/** A page from a file, for a sandboxed frame: no scripts (the frame's
 *  sandbox), and nothing it may load but pictures, styles and fonts. */
function pageDoc(html: string): string {
  const csp = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: https: http:; style-src 'unsafe-inline' https: http:; font-src data: https: http:; media-src data: https:">`;
  // Its scripts could not run in the frame anyway: left out, so the frame
  // does not report each one it refused.
  const quiet = html.replace(/<script\b[\s\S]*?(<\/script\s*>|$)/gi, "");
  return `<!doctype html><html><head><meta charset="utf-8">${csp}<base target="_blank"></head><body>${quiet}</body></html>`;
}

export function isMarkdown(a: Attachment): boolean {
  return MARKDOWN.test(a.name) || /markdown/i.test(a.content_type || "");
}

/** Which version of its name a file is in a conversation, and the one
 *  before it. */
export type Version = { n: number; prev?: Attachment };

/** Versions by file id: files are counted by name, oldest first. */
export function versionsOf(messages: Message[]): Map<string, Version> {
  const seen = new Map<string, Attachment[]>();
  const out = new Map<string, Version>();
  const sorted = [...messages].sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime());
  for (const m of sorted) {
    for (const a of m.attachments || []) {
      if (out.has(a.file_id)) continue;
      const list = seen.get(a.name) || [];
      out.set(a.file_id, { n: list.length + 1, prev: list[list.length - 1] });
      list.push(a);
      seen.set(a.name, list);
    }
  }
  return out;
}

export const VersionsContext = createContext<Map<string, Version> | null>(null);
export const useVersion = (id: string) => useContext(VersionsContext)?.get(id);

async function readText(hubId: string, fileId: string): Promise<string> {
  const res = await fetch(`${appApi.attachmentUrl(hubId, fileId)}?as=text`);
  if (!res.ok) {
    let code = "";
    try { code = (await res.json())?.error || ""; } catch { /* not JSON */ }
    throw Object.assign(new Error(code === "not_text"
      ? L("这个文件不是文字，没法在这里看", "This file is not text and cannot be shown here")
      : code === "not_found"
        ? L("拿不到这个文件了：可能已过期，或者工作区的 myco 版本太旧", "This file is not available: it may have expired, or the workspace's myco is too old")
        : L("文件读不出来", "Could not read the file")), { code });
  }
  return res.text();
}

// A line diff (longest common subsequence), with unchanged stretches folded
// to a few lines around each change. Long files are compared by their
// first lines only.
type Line = { kind: "same" | "add" | "del" | "fold"; text: string };
const MAX_LINES = 1500;
const CONTEXT = 3;

function diffLines(before: string, after: string): Line[] {
  const a = before.split("\n").slice(0, MAX_LINES);
  const b = after.split("\n").slice(0, MAX_LINES);
  const n = a.length, m = b.length;
  const lcs: Uint16Array[] = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }
  const all: Line[] = [];
  let i = 0, j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) { all.push({ kind: "same", text: a[i] }); i++; j++; }
    else if (lcs[i + 1][j] >= lcs[i][j + 1]) { all.push({ kind: "del", text: a[i] }); i++; }
    else { all.push({ kind: "add", text: b[j] }); j++; }
  }
  while (i < n) all.push({ kind: "del", text: a[i++] });
  while (j < m) all.push({ kind: "add", text: b[j++] });
  // Fold what did not change, away from the changes.
  const near = all.map(() => false);
  all.forEach((l, k) => {
    if (l.kind === "same") return;
    for (let d = -CONTEXT; d <= CONTEXT; d++) if (all[k + d]) near[k + d] = true;
  });
  const out: Line[] = [];
  let folded = 0;
  all.forEach((l, k) => {
    if (l.kind !== "same" || near[k]) {
      if (folded) out.push({ kind: "fold", text: L(`… ${folded} 行没变`, `… ${folded} unchanged lines`) });
      folded = 0;
      out.push(l);
    } else folded++;
  });
  if (folded) out.push({ kind: "fold", text: L(`… ${folded} 行没变`, `… ${folded} unchanged lines`) });
  return out;
}

const FULL_KEY = "myco-preview-full";
const keptFull = () => { try { return localStorage.getItem(FULL_KEY) === "1"; } catch { return false; } };
const keepFull = (on: boolean) => { try { localStorage.setItem(FULL_KEY, on ? "1" : "0"); } catch { /* not kept */ } };

/** A text file read in the App, with Save, and what changed since its
 *  previous version when there is one. */
export function FilePreview({ hubId, file, version, open, onOpenChange }: {
  hubId: string;
  file: Attachment;
  version?: Version;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [text, setText] = useState<string | null>(null);
  const [prevText, setPrevText] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [view, setView] = useState<"file" | "source" | "changes">("file");
  const [saved, setSaved] = useState(false);
  const [full, setFull] = useState(keptFull);
  const prev = version?.prev;
  const toggleFull = () => setFull((f) => { keepFull(!f); return !f; });

  useEffect(() => {
    if (!open) return;
    let live = true;
    setText(null); setPrevText(null); setError(""); setView("file"); setSaved(false);
    readText(hubId, file.file_id).then((t) => live && setText(t), (e) => live && setError((e as Error).message));
    if (prev) readText(hubId, prev.file_id).then((t) => live && setPrevText(t), () => undefined);
    return () => { live = false; };
  }, [open, hubId, file.file_id, prev]);

  const changes = useMemo(() => (view === "changes" && text !== null && prevText !== null ? diffLines(prevText, text) : null), [view, text, prevText]);
  const md = isMarkdown(file);
  const html = isHtml(file);
  // What it can be seen as: a page and its source, and the changes since
  // the version before.
  const views: { key: "file" | "source" | "changes"; label: string }[] = [
    { key: "file", label: html ? L("页面", "Page") : prev ? L("这一版", "This version") : "" },
    ...(html ? [{ key: "source" as const, label: L("源码", "Source") }] : []),
    ...(prev && prevText !== null ? [{ key: "changes" as const, label: L("和上一版比", "Changes since the last") }] : []),
  ];
  const save = async () => {
    try { await appApi.saveAttachment(hubId, file.file_id); setSaved(true); } catch (e) { setError((e as Error).message); }
  };

  const about = [bytes(file.size), version && version.n > 1 ? L(`第 ${version.n} 版`, `version ${version.n}`) : ""].filter(Boolean).join(" · ");
  const fullLabel = full ? L("退出全屏", "Exit full screen") : L("全屏", "Full screen");
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="app-portal fp-scrim" />
        <Dialog.Content className={cn("app-portal fp-panel", full && "is-full")}>
          <div className="fp-head" onDoubleClick={(e) => { if (!(e.target as HTMLElement).closest("button")) toggleFull(); }}>
            <div className="fp-titles">
              <Dialog.Title asChild><h3 title={file.name}>{file.name}</h3></Dialog.Title>
              <Dialog.Description asChild><p>{about}</p></Dialog.Description>
            </div>
            {views.length > 1 && text !== null && (
              <div className="seg seg-sm fp-views" role="tablist" aria-label={L("看什么", "Show")}>
                {views.map((v) => (
                  <button key={v.key} type="button" role="tab" aria-selected={view === v.key} className={cn("seg-btn", view === v.key && "active")} onClick={() => setView(v.key)}>
                    {v.label}
                  </button>
                ))}
              </div>
            )}
            <div className="fp-tools">
              <button type="button" className="app-btn app-btn-plain" onClick={() => void save()}
                title={saved ? L("已存到「下载」", "Saved to Downloads") : L("存到「下载」", "Save to Downloads")}
                aria-label={saved ? L("已存到「下载」", "Saved to Downloads") : L("存到「下载」", "Save to Downloads")}>
                {saved ? <Check /> : <Download />}
              </button>
              <button type="button" className="app-btn app-btn-plain fp-full-btn" onClick={toggleFull} title={fullLabel} aria-label={fullLabel} aria-pressed={full}>
                {full ? <Minimize2 /> : <Maximize2 />}
              </button>
              <Dialog.Close asChild>
                <button type="button" className="app-btn app-btn-plain" title={L("关闭", "Close")} aria-label={L("关闭", "Close")}><X /></button>
              </Dialog.Close>
            </div>
          </div>
          <div className={cn("fp-body", md && view === "file" && "is-doc", html && view === "file" && "is-page")}>
            {error ? <p className="fp-note">{error}</p>
              : text === null ? <p className="fp-note">{L("正在打开…", "Opening…")}</p>
              : changes ? (
                <pre className="fp-diff">{changes.map((l, k) => (
                  <span key={k} className={cn("fp-line", `is-${l.kind}`)}>{l.kind === "add" ? "+ " : l.kind === "del" ? "− " : "  "}{l.text}{"\n"}</span>
                ))}</pre>
              ) : md && view === "file" ? <div className="fp-doc"><Markdown text={text} /></div>
              // A page as a page, in a frame of its own that runs nothing.
              : html && view === "file" ? <iframe className="fp-frame" title={file.name} sandbox="" referrerPolicy="no-referrer" srcDoc={pageDoc(text)} />
              : <pre className="fp-code">{text}</pre>}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
