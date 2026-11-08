import * as React from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { AlertTriangle } from "lucide-react";
import { L } from "@/lib/i18n";
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
  const message = error instanceof Error ? error.message : String(error);
  const suggestion = (error as { suggestion?: string } | null)?.suggestion;
  return (
    <div className="app-error-text">
      <div>{message}</div>
      {suggestion && <div style={{ color: "var(--muted)", marginTop: 2 }}>{suggestion}</div>}
    </div>
  );
}
