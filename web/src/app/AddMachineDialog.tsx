import { useEffect, useState } from "react";
import { Link2 } from "lucide-react";
import { appApi, type Hub, type JoinLink } from "@/appapi";
import { CopyButton } from "@/app/copy";
import { ErrorText, Modal } from "@/app/dialogs";
import { L } from "@/lib/i18n";

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
      title={L("连上另一台电脑", "Connect another computer")}
      description={L("连上以后，就能把这台电脑交给项目里的 agent 用。", "Once connected, you can let your projects' agents use it.")}
      wide
    >
      {hubs.length === 0 ? (
        <div className="app-hint">{L("先建一个工作区。", "Add a Hub first, then you can add machines to it.")}</div>
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
                  {h.name}{h.ok ? "" : L("（连不上）", " (unreachable)")}
                </option>
              ))}
            </select>
          </label>

          {!link ? (
            <>
              <div className="app-hint">
                {L("生成一条入网链接，在要添加的电脑上运行它。链接只能用一次，24 小时内有效。", "Create a join link and run it on the computer you want to add. The link works once and expires in 24 hours.")}
              </div>
              <ErrorText error={error} />
              <div className="app-actions">
                <button
                  type="button"
                  className="app-btn app-btn-primary"
                  onClick={() => void generate()}
                  disabled={busy || !hubId}
                >
                  {busy ? <><span className="app-spinner" />{L("正在生成…", "Creating…")}</> : <><Link2 />{L("生成入网链接", "Create join link")}</>}
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
                {L("在那台电脑的终端里运行这一行。装好以后，给项目加电脑时就能选它了。", "Run this command in a terminal on the computer or server you want to add. It installs myco and joins the machine to the Hub; it then shows up on the Machines page.")}
              </div>
              <ErrorText error={error} />
              <div className="app-modal-actions">
                <button type="button" className="app-btn" onClick={() => void generate()} disabled={busy}>
                  {busy ? L("正在生成…", "Creating…") : L("重新生成", "Create new link")}
                </button>
                <button type="button" className="app-btn app-btn-primary" onClick={close} disabled={busy}>
                  {L("完成", "Done")}
                </button>
              </div>
            </>
          )}
        </>
      )}
    </Modal>
  );
}
