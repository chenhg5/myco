import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { Check, Download } from "lucide-react";
import { appApi } from "@/appapi";
import type { Attachment, Message } from "@/api";
import { Modal } from "@/app/dialogs";
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
 */

const TEXT_TYPE = /^(text\/|application\/(json|xml|x-yaml|yaml|toml|x-sh|javascript|typescript|x-ndjson))/;
const TEXT_EXT = /\.(md|markdown|txt|text|log|json|jsonl|ya?ml|toml|csv|tsv|ini|conf|cfg|env|go|ts|tsx|js|jsx|mjs|cjs|py|rb|rs|java|kt|swift|c|h|cc|cpp|hpp|cs|php|sh|bash|zsh|ps1|sql|html?|css|scss|xml|diff|patch|lock)$/i;
const MARKDOWN = /\.(md|markdown)$/i;

/** Whether a file can be read in the App as text. */
export function isText(a: Attachment): boolean {
  const t = (a.content_type || "").split(";")[0].trim().toLowerCase();
  return TEXT_TYPE.test(t) || TEXT_EXT.test(a.name);
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
  const [view, setView] = useState<"file" | "changes">("file");
  const [saved, setSaved] = useState(false);
  const prev = version?.prev;

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
  const save = async () => {
    try { await appApi.saveAttachment(hubId, file.file_id); setSaved(true); } catch (e) { setError((e as Error).message); }
  };

  return (
    <Modal open={open} onOpenChange={onOpenChange} wide className="fp-modal"
      title={file.name}
      description={[bytes(file.size), version && version.n > 1 ? L(`第 ${version.n} 版`, `version ${version.n}`) : ""].filter(Boolean).join(" · ")}>
      {prev && prevText !== null && text !== null && (
        <div className="seg seg-sm fp-views" role="tablist" aria-label={L("看什么", "Show")}>
          <button type="button" role="tab" aria-selected={view === "file"} className={cn("seg-btn", view === "file" && "active")} onClick={() => setView("file")}>
            {L("这一版", "This version")}
          </button>
          <button type="button" role="tab" aria-selected={view === "changes"} className={cn("seg-btn", view === "changes" && "active")} onClick={() => setView("changes")}>
            {L("和上一版比", "Changes since the last")}
          </button>
        </div>
      )}
      <div className={cn("fp-body", md && view === "file" && "is-doc")}>
        {error ? <p className="fp-note">{error}</p>
          : text === null ? <p className="fp-note">{L("正在打开…", "Opening…")}</p>
          : changes ? (
            <pre className="fp-diff">{changes.map((l, k) => (
              <span key={k} className={cn("fp-line", `is-${l.kind}`)}>{l.kind === "add" ? "+ " : l.kind === "del" ? "− " : "  "}{l.text}{"\n"}</span>
            ))}</pre>
          ) : md ? <Markdown text={text} />
          : <pre className="fp-code">{text}</pre>}
      </div>
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => void save()}>
          {saved ? <Check /> : <Download />}{saved ? L("已存到「下载」", "Saved to Downloads") : L("存到「下载」", "Save to Downloads")}
        </button>
        <button type="button" className="app-btn app-btn-primary" onClick={() => onOpenChange(false)}>{L("关闭", "Close")}</button>
      </div>
    </Modal>
  );
}
