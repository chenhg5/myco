import * as React from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { L } from "@/lib/i18n";
import { ApiError, isOldHub, isTransient } from "@/appapi";
import { CopyButton, useCopy } from "@/app/copy";
import { debugPrompt } from "@/app/agentSetup";
import { cn } from "@/lib/utils";

/** macOS-style centered sheet. Portals carry .app-portal so the scoped tokens apply. */
export function Modal({ open, onOpenChange, title, description, wide, className, children }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: React.ReactNode;
  wide?: boolean;
  /** a size or layout of its own (e.g. the new-conversation picker) */
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="app-portal app-overlay" />
        <Dialog.Content
          className={cn("app-portal app-modal", wide && "app-modal-wide", className)}
        >
          <Dialog.Title asChild><h3>{title}</h3></Dialog.Title>
          {description ? (
            <Dialog.Description asChild><p>{description}</p></Dialog.Description>
          ) : (
            <Dialog.Description className="app-sr-only">{title}</Dialog.Description>
          )}
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** Asks before something that cannot be undone. Calm: a short question,
 *  what it is about (subject), what happens in plain words, and the
 *  answer; no red block (the button says it). */
export function ConfirmDialog({ open, onOpenChange, title, subject, description, consequences, confirmLabel, onConfirm, busy, error }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  /** who or what it is about (an avatar and a name), under the title */
  subject?: React.ReactNode;
  description?: React.ReactNode;
  consequences?: React.ReactNode;
  confirmLabel: string;
  onConfirm: () => void;
  busy?: boolean;
  error?: React.ReactNode;
}) {
  return (
    <Modal open={open} onOpenChange={onOpenChange} title={title} description={subject ? undefined : description} className="app-confirm">
      {subject && <div className="app-confirm-subject">{subject}</div>}
      {subject && description && <p className="app-confirm-text">{description}</p>}
      {consequences && <div className="app-confirm-note">{consequences}</div>}
      {error && <div className="app-error-text">{error}</div>}
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => onOpenChange(false)} disabled={busy}>{L("取消", "Cancel")}</button>
        <button type="button" className="app-btn app-btn-danger" onClick={onConfirm} disabled={busy}>
          {busy ? L("处理中…", "Working…") : confirmLabel}
        </button>
      </div>
    </Modal>
  );
}

/** What an error says underneath, for "详细信息": the status, the code and
 *  the raw text, so people can see or pass on why it failed. */
function errorDetail(error: unknown): string {
  if (error instanceof ApiError) {
    const head = [error.status ? `HTTP ${error.status}` : "", typeof error.code === "string" ? error.code : ""].filter(Boolean).join(" · ");
    return head && error.message !== head && error.message !== `HTTP ${error.status}` ? `${head}\n${error.message}` : head || error.message;
  }
  if (error instanceof Error) return error.message || error.name;
  return String(error);
}

/**
 * Inline error: the backend message plus its suggestion. A hub that is
 * down or slow is said in words, with the raw reason one click away
 * (详细信息) and, when the caller can try again, a 重试 button. Nothing
 * here retries by itself, so it never says it will.
 */
export function ErrorText({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const [details, setDetails] = React.useState(false);
  React.useEffect(() => setDetails(false), [error]);
  const ask = useCopy(error ? debugPrompt(errorDetail(error)) : "");
  if (!error) return null;
  const transient = isTransient(error);
  const old = isOldHub(error);
  const message = old
    ? L("你的工作区版本有点旧，还不支持这个功能。", "This hub runs an older version that doesn't support this yet.")
    : transient
      ? L("暂时连不上，或者响应太慢。", "Can't reach this hub right now, or it is answering too slowly.")
      : error instanceof Error ? error.message : String(error);
  const suggestion = old
    ? L("到设置里升级一下再试。", "Upgrade the hub in Settings → Hubs, then try again.")
    : transient
      ? onRetry
        ? L("点「重试」再试一次。一直这样的话，看看左下角工作区连没连上，或者到设置 › 网络代理里看看。", "Press Retry to try again. If it keeps happening, check that the hub shows as connected (bottom left), or look at Settings → Network proxy.")
        : L("等一下再试。一直这样的话，看看左下角工作区连没连上，或者到设置 › 网络代理里看看。", "Try again in a moment. If it keeps happening, check that the hub shows as connected (bottom left), or look at Settings → Network proxy.")
      : (error as { suggestion?: string } | null)?.suggestion;
  // The raw text is only hidden when it was replaced by words above.
  const detail = transient || old ? errorDetail(error) : "";
  return (
    <div className="app-error-text">
      <div>{message}</div>
      {suggestion && <div style={{ color: "var(--muted)", marginTop: 2 }}>{suggestion}</div>}
      {(onRetry || detail || transient) && (
        <div className="app-error-actions">
          {onRetry && <button type="button" className="app-btn app-btn-small" onClick={onRetry}>{L("重试", "Retry")}</button>}
          {/* A sentence for the person's own agent: the raw error and where to look. */}
          <button type="button" className="app-error-more" onClick={() => void ask.copy()} title={L("复制一句话发给你的 agent（Claude Code、Codex…），它会帮你查原因", "Copy a sentence for your agent (Claude Code, Codex…) to look into it")}>
            {ask.copied ? L("已复制，发给你的 agent", "Copied: send it to your agent") : L("让 agent 帮我查", "Ask my agent")}
          </button>
          {detail && (
            <button type="button" className="app-error-more" aria-expanded={details} onClick={() => setDetails((d) => !d)}>
              {details ? L("收起详细信息", "Hide details") : L("详细信息", "Details")}
            </button>
          )}
        </div>
      )}
      {details && detail && (
        <div className="app-error-detail">
          <pre>{detail}</pre>
          <CopyButton value={detail} />
        </div>
      )}
    </div>
  );
}
