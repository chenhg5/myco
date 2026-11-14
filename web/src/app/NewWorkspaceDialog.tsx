import { useEffect, useState } from "react";
import { ErrorText, Modal } from "@/app/dialogs";
import { suggestWorkspaceName } from "@/app/localHubs";
import { L } from "@/lib/i18n";

/** Starts another workspace on this computer (a name, then the same path as
 *  the first one: start, join, switch), or, with server, adds one to the
 *  hub server of the workspace on show (same address, kept apart). */
export function NewWorkspaceDialog({ open, onOpenChange, taken, onStart, server }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Internal names already used by this computer's workspaces. */
  taken: string[];
  /** Starts it, adds it to the App and switches to it; rejects with an error. */
  onStart: (name: string, display: string) => Promise<void>;
  /** the server it goes on (its host); unset: this computer */
  server?: string;
}) {
  const [display, setDisplay] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!open) return;
    setDisplay("");
    setBusy(false);
    setError(null);
  }, [open]);

  const name = suggestWorkspaceName(display, taken);
  const close = () => { if (!busy) onOpenChange(false); };
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      await onStart(name, display.trim());
      onOpenChange(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onOpenChange={(o) => { if (!o) close(); }}
      title={server ? L(`在 ${server} 上新建工作区`, `New workspace on ${server}`) : L("在这台电脑上新建工作区", "New workspace on this computer")}
      description={server
        ? L("在同一台服务器上再开一个工作区：成员、agent、项目、消息都和现在的分开，地址、证书都不用另配。建好后你就是它的主人，自动切换过去。", "Adds a workspace on the same server: its members, agents, projects and messages are kept apart from this one, on the same address. You own it; the app switches to it.")
        : L("在这台电脑上再起一个工作区，自动加入并切换过去。它和已有的工作区互不影响。", "Starts another workspace on this computer, adds it to the app and switches to it. It keeps its nodes apart from your other workspaces.")}
    >
      <form onSubmit={(e) => { e.preventDefault(); void submit(); }}>
        <label className="app-field">
          <span className="app-field-label">{L("工作区名", "Workspace name")}</span>
          <input
            className="app-input"
            value={display}
            onChange={(e) => setDisplay(e.target.value)}
            placeholder={L("比如：设计、team-notes", "e.g. Design, team-notes")}
            disabled={busy}
            autoFocus
            required
          />
          <span className="app-small-note">{L(`内部名：${name}（只能用小写字母、数字和横线，之后也能在设置里改名）`, `Internal name: ${name} (lowercase letters, digits and dashes; you can rename the workspace later in Settings)`)}</span>
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={close} disabled={busy}>{L("取消", "Cancel")}</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy || !display.trim()}>
            {busy ? <><span className="app-spinner" />{server ? L("正在新建…", "Creating…") : L("正在启动…", "Starting…")}</> : L("新建并切换", "Create and switch")}
          </button>
        </div>
      </form>
    </Modal>
  );
}
