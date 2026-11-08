import { useEffect, useState } from "react";
import { Link2 } from "lucide-react";
import { appApi, type Hub, type JoinLink } from "@/appapi";
import { CopyButton } from "@/app/copy";
import { ErrorText, Modal } from "@/app/dialogs";

/** Pick a hub, create a one-off join link, show the command. */
export function AddMachineDialog({ open, onOpenChange, hubs }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hubs: Hub[];
}) {
  const [hubId, setHubId] = useState("");
  const [link, setLink] = useState<JoinLink | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!open) return;
    const first = hubs.find((h) => h.ok) || hubs[0];
    setHubId(first?.id || "");
    setLink(null);
    setBusy(false);
    setError(null);
    // Reset only when the dialog opens, not when the hub list polls.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // Hubs can arrive just after the dialog opened.
  useEffect(() => {
    if (!open || hubId || hubs.length === 0) return;
    const first = hubs.find((h) => h.ok) || hubs[0];
    if (first) setHubId(first.id);
  }, [open, hubId, hubs]);

  const close = () => { if (!busy) onOpenChange(false); };

  const changeHub = (id: string) => {
    setHubId(id);
    setLink(null);
    setError(null);
  };

  const generate = async () => {
    if (!hubId) return;
    setBusy(true);
    setError(null);
    try {
      setLink(await appApi.createJoinLink(hubId, { uses: 1, expires_hours: 24 }));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onOpenChange={(o) => !o && close()}
      title="添加机器"
      description="在另一台电脑上接入这个 Hub。"
      wide
    >
      {hubs.length === 0 ? (
        <div className="app-hint">先添加一个 Hub，才能给 Hub 添加机器。</div>
      ) : (
        <>
          <label className="app-field">
            <span className="app-field-label">Hub</span>
            <select
              className="app-select-field"
              value={hubId}
              onChange={(e) => changeHub(e.target.value)}
              disabled={busy}
            >
              {hubs.map((h) => (
                <option key={h.id} value={h.id} disabled={!h.ok}>
                  {h.name}{h.ok ? "" : "（连不上）"}
                </option>
              ))}
            </select>
          </label>

          {!link ? (
            <>
              <div className="app-hint">
                生成一条入网链接，在要添加的电脑上运行它。链接只能用一次，24 小时内有效。
              </div>
              <ErrorText error={error} />
              <div className="app-actions">
                <button
                  type="button"
                  className="app-btn app-btn-primary"
                  onClick={() => void generate()}
                  disabled={busy || !hubId}
                >
                  {busy ? <><span className="app-spinner" />正在生成…</> : <><Link2 />生成入网链接</>}
                </button>
              </div>
            </>
          ) : (
            <>
              <div className="app-command">
                <code>{link.command}</code>
                <CopyButton value={link.command} />
              </div>
              <div className="app-hint">
                在要添加的电脑或服务器的终端里运行这条命令。它会装好 myco 并把这台机器加入 Hub，之后会出现在机器页。
              </div>
              <ErrorText error={error} />
              <div className="app-modal-actions">
                <button type="button" className="app-btn" onClick={() => void generate()} disabled={busy}>
                  {busy ? "正在生成…" : "重新生成"}
                </button>
                <button type="button" className="app-btn app-btn-primary" onClick={close} disabled={busy}>
                  完成
                </button>
              </div>
            </>
          )}
        </>
      )}
    </Modal>
  );
}
