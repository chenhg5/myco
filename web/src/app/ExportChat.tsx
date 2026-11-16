import { useEffect, useState } from "react";
import { Check } from "lucide-react";
import type { Message } from "@/api";
import { appApi } from "@/appapi";
import { ErrorText, Modal } from "@/app/dialogs";
import { exportName, stepsOf, toJsonl, toMarkdown, type ExportFormat, type ExportStep } from "@/app/exportChat";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/**
 * Exporting a conversation: Markdown (to read and share) or JSON Lines (for
 * programs), with or without the agents' work steps; saved to Downloads.
 * What the App shows is what is exported, decrypted: an end-to-end
 * encrypted chat says that the file is not.
 */
export function ExportChat({ open, onClose, hubId, withWhom, messages, nameOf }: {
  open: boolean;
  onClose: () => void;
  hubId: string;
  /** whom the conversation is with (its title and the file's name) */
  withWhom: string;
  messages: Message[];
  nameOf: (n: string) => string;
}) {
  const [format, setFormat] = useState<ExportFormat>("md");
  const [steps, setSteps] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState("");
  useEffect(() => { if (open) { setError(null); setSaved(""); setBusy(false); } }, [open]);
  const encrypted = messages.some((m) => m.e2e);
  const tasks = messages.filter((m) => m.type === "task");

  const go = async () => {
    setBusy(true);
    setError(null);
    try {
      let byTask: Map<string, ExportStep[]> | undefined;
      if (steps) {
        byTask = new Map();
        for (const t of tasks) {
          // A task without a run (or a hub without traces) has no steps.
          const tr = await appApi.trace(hubId, t.id).catch(() => null);
          const ls = tr ? stepsOf(tr.events || []) : [];
          if (ls.length) byTask.set(t.id, ls);
        }
      }
      const ordered = [...messages].sort((a, b) => a.created_at.localeCompare(b.created_at));
      const content = format === "md"
        ? toMarkdown(ordered, { title: L(`和 ${withWhom} 的对话`, `Conversation with ${withWhom}`), nameOf, steps: byTask })
        : toJsonl(ordered, byTask);
      const r = await appApi.saveExport(exportName(withWhom, format), content);
      setSaved(r.path);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onOpenChange={(o) => !o && !busy && onClose()} title={L("导出对话", "Export the conversation")}
      description={L(`和 ${withWhom} 的 ${messages.length} 条消息，存到「下载」文件夹。`, `${messages.length} messages with ${withWhom}, saved to Downloads.`)}>
      <div data-testid="export-chat">
        <div className="app-field">
          <span className="app-field-label">{L("格式", "Format")}</span>
          <div className="seg" role="tablist" aria-label={L("格式", "Format")}>
            <button type="button" role="tab" aria-selected={format === "md"} className={cn("seg-btn", format === "md" && "active")} onClick={() => setFormat("md")} disabled={busy}>Markdown</button>
            <button type="button" role="tab" aria-selected={format === "jsonl"} className={cn("seg-btn", format === "jsonl" && "active")} onClick={() => setFormat("jsonl")} disabled={busy}>JSONL</button>
          </div>
          <div className="app-small-note">{format === "md"
            ? L("给人看：发给别人、存档。按时间排，写着谁说的、什么时候、附件名。", "For people: to share or keep. In time order, with who said it, when, and attachment names.")
            : L("给程序用：一行一条消息，myco 自己的字段（和 myco thread export 一样）。", "For programs: one message per line, myco's own fields (as myco thread export writes).")}</div>
        </div>
        <label className="app-check-line">
          <input type="checkbox" checked={steps} onChange={(e) => setSteps(e.target.checked)} disabled={busy || tasks.length === 0} />
          <span>{L("带上 agent 干活的过程", "Include the agents' work steps")}
            <small>{L("每个任务下面列出它用了哪些工具：读了什么、跑了什么命令。", "Under each task: the tools it used, what it read, which commands it ran.")}</small></span>
        </label>
        {encrypted && <div className="app-small-note" data-testid="export-plain">{L("这段对话是端到端加密的，导出的文件是解密后的明文，注意保管。", "This conversation is end-to-end encrypted; the exported file is decrypted plain text. Keep it safe.")}</div>}
        <ErrorText error={error} />
        {saved && <div className="ws-command-done" data-testid="export-saved"><Check /><span>{L("已存到：", "Saved to: ")}<span className="mono">{saved}</span></span></div>}
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={onClose} disabled={busy}>{saved ? L("好", "OK") : L("取消", "Cancel")}</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void go()} disabled={busy || messages.length === 0}>
            {busy ? L("正在导出…", "Exporting…") : L("导出", "Export")}
          </button>
        </div>
      </div>
    </Modal>
  );
}
