import * as React from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { AlertTriangle } from "lucide-react";
import { L } from "@/lib/i18n";
import { isOldHub, isTransient } from "@/appapi";
import { cn } from "@/lib/utils";

/** macOS-style centered sheet. Portals carry .app-portal so the scoped tokens apply. */
export function Modal({ open, onOpenChange, title, description, wide, children }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: React.ReactNode;
  wide?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="app-portal app-overlay" />
        <Dialog.Content
          className={cn("app-portal app-modal", wide && "app-modal-wide")}
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

export function ConfirmDialog({ open, onOpenChange, title, description, consequences, confirmLabel, onConfirm, busy, error }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: React.ReactNode;
  consequences?: React.ReactNode;
  confirmLabel: string;
  onConfirm: () => void;
  busy?: boolean;
  error?: React.ReactNode;
}) {
  return (
    <Modal open={open} onOpenChange={onOpenChange} title={title} description={description}>
      {consequences && (
        <div className="app-warn">
          <AlertTriangle />
          <div>{consequences}</div>
        </div>
      )}
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

/** Inline error: the backend message plus its suggestion. */
export function ErrorText({ error }: { error: unknown }) {
  if (!error) return null;
  // A hub that is down or slow: say so in words, not the error text.
  const transient = isTransient(error);
  const old = isOldHub(error);
  const message = old
    ? L("这个 Hub 的版本较旧，还不支持这个功能。", "This hub runs an older version that doesn't support this yet.")
    : transient
      ? L("暂时连不上这个 Hub，或者它响应太慢。", "Can't reach this hub right now, or it is answering too slowly.")
      : error instanceof Error ? error.message : String(error);
  const suggestion = old
    ? L("在「设置 → Hub」里升级这个 Hub 后再试。", "Upgrade the hub in Settings → Hubs, then try again.")
    : transient
      ? L("会自动重试，不用做什么。一直这样的话，到「设置 → Hub」看看它的状态。", "It retries by itself. If this keeps happening, check the hub in Settings → Hubs.")
      : (error as { suggestion?: string } | null)?.suggestion;
  return (
    <div className="app-error-text">
      <div>{message}</div>
      {suggestion && <div style={{ color: "var(--muted)", marginTop: 2 }}>{suggestion}</div>}
    </div>
  );
}
