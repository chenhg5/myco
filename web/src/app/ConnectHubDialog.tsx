import { useEffect, useState } from "react";
import { ArrowLeft, ChevronDown, Link2 } from "lucide-react";
import { appApi, type Hub } from "@/appapi";
import { CopyButton } from "@/app/copy";
import { ErrorText, Modal } from "@/app/dialogs";

const INSTALL_COMMAND =
  "curl -fsSL https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-hub.sh | sudo bash -s -- --domain auto";

/**
 * Adds a hub. The normal way is pasting the link the installer printed; the
 * install command and the manual address + token form are the fallbacks.
 */
export function ConnectHubDialog({ open, onOpenChange, prefill, onAdded }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** A link the App was opened with; fills the dialog when it arrives. */
  prefill?: string;
  onAdded: (hub: Hub) => void;
}) {
  const [screen, setScreen] = useState<"link" | "command">("link");
  const [link, setLink] = useState("");
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!open) return;
    setScreen("link");
    setLink(prefill || "");
    setName("");
    setUrl("");
    setToken("");
    setBusy(false);
    setError(null);
  }, [open, prefill]);

  const close = () => { if (!busy) onOpenChange(false); };

  const connect = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const hub = await appApi.addHubByLink({ link: link.trim() });
      onAdded(hub);
      onOpenChange(false);
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  };

  const connectManual = async () => {
    setBusy(true);
    setError(null);
    let fallbackName = url.trim();
    try { fallbackName = new URL(url.trim()).host || fallbackName; } catch { /* keep the raw address */ }
    try {
      const hub = await appApi.addHub({
        name: name.trim() || fallbackName,
        url: url.trim(),
        token: token.trim(),
      });
      onAdded(hub);
      onOpenChange(false);
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onOpenChange={(o) => { if (!o) close(); }}
      title="连接 Hub"
      description={screen === "link" ? "粘贴 App 连接链接，就能看到这个 Hub 上的节点。" : undefined}
      wide
    >
      {screen === "link" ? (
        <form onSubmit={connect}>
          <label className="app-field">
            <span className="app-field-label">App 连接链接</span>
            <input
              className="app-input mono app-input-lg"
              value={link}
              onChange={(e) => setLink(e.target.value)}
              placeholder="https://你的服务器/c/myco_console_…"
              autoFocus
              required
            />
          </label>

          <div className="app-linkline">
            <button type="button" className="app-linklike" onClick={() => { setScreen("command"); setError(null); }}>
              还没有链接？
            </button>
            <span>先在一台服务器上安装 Hub</span>
          </div>

          <details className="app-advanced">
            <summary>
              手动填写地址和令牌
              <ChevronDown />
            </summary>
            <div className="app-advanced-body">
              <label className="app-field">
                <span className="app-field-label">名称（可选）</span>
                <input
                  className="app-input"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="我的服务器"
                  disabled={busy}
                />
              </label>
              <label className="app-field">
                <span className="app-field-label">地址</span>
                <input
                  className="app-input mono"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="https://hub.example.com"
                  disabled={busy}
                />
              </label>
              <label className="app-field">
                <span className="app-field-label">控制台令牌</span>
                <input
                  className="app-input mono"
                  type="password"
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                  placeholder="myco_console_…"
                  autoComplete="off"
                  disabled={busy}
                />
              </label>
              <div className="app-hint">
                在 Hub 所在机器上运行 <code>myco console token create --name {name.trim() || "<设备名>"}</code> 获取令牌。
              </div>
              <div className="app-actions">
                <button
                  type="button"
                  className="app-btn"
                  onClick={() => void connectManual()}
                  disabled={busy || !url.trim() || !token.trim()}
                >
                  {busy ? "验证中…" : "用地址和令牌连接"}
                </button>
              </div>
            </div>
          </details>

          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={close} disabled={busy}>取消</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={busy || !link.trim()}>
              {busy ? <><span className="app-spinner" />正在连接…</> : <><Link2 />连接</>}
            </button>
          </div>
        </form>
      ) : (
        <>
          <div className="app-hint">
            在你的服务器（Linux）上运行下面这行命令，装好后它会打印一个 App 连接链接，复制回来粘贴到上一步。
          </div>
          <div className="app-command">
            <code>{INSTALL_COMMAND}</code>
            <CopyButton value={INSTALL_COMMAND} />
          </div>
          <div className="app-hint" style={{ marginTop: 12 }}>
            已经装过 Hub？在服务器上运行 <code>sudo myco console link</code> 生成新链接。
          </div>
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={() => { setScreen("link"); setError(null); }}>
              <ArrowLeft />返回
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
