import { RichText } from "@/app/RichText";
import { useEffect, useState } from "react";
import { ArrowLeft, ChevronDown, Link2 } from "lucide-react";
import { confirmNameLater } from "@/app/NamePrompt";
import { appApi, type Hub } from "@/appapi";
import { CopyButton } from "@/app/copy";
import { ErrorText, Modal } from "@/app/dialogs";
import { setupPrompt } from "@/app/agentSetup";
import { ConnectOver } from "@/app/ConnectOver";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

const INSTALL_COMMAND =
  "curl -fsSL https://raw.githubusercontent.com/myco-ai/myco/main/deploy/install-hub.sh | sudo bash -s -- --domain auto";

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
  // Set up over SSH: the server's workspace is in this App now.
  const [how, setHow] = useState<"ssh" | "command">("ssh");
  const added = async (hubId: string) => {
    const hub = (await appApi.hubs().catch((): Hub[] => [])).find((h) => h.id === hubId);
    if (hub) onAdded(hub);
    onOpenChange(false);
  };

  const connect = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const hub = await appApi.addHubByLink({ link: link.trim() });
      // Joined through a one-time link (an invite): confirm the name there.
      if (link.includes("/c/myco_pair_")) confirmNameLater(hub.id);
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
      title={L("加入工作区", "Join a workspace")}
      description={screen === "link" ? L("粘贴别人给你的链接，就能看到那里的项目和 agent。", "Paste an app link to see the nodes on this Hub.") : undefined}
      wide
    >
      {screen === "link" ? (
        <form onSubmit={connect}>
          <label className="app-field">
            <span className="app-field-label">{L("App 连接链接", "App link")}</span>
            <input
              className="app-input mono app-input-lg"
              value={link}
              onChange={(e) => setLink(e.target.value)}
              placeholder={L("https://你的服务器/c/myco_console_…", "https://your-server/c/myco_console_…")}
              autoFocus
              required
            />
          </label>

          <div className="app-linkline">
            <button type="button" className="app-linklike" onClick={() => { setScreen("command"); setError(null); }}>
              {L("还没有链接？", "No link yet?")}
            </button>
            <span>{L("自己在服务器上搭一个，或者让 agent 帮你装", "Install a Hub on a server, or let your agent do it")}</span>
          </div>

          <details className="app-advanced">
            <summary>
              {L("手动填写地址和令牌", "Enter address and token manually")}
              <ChevronDown />
            </summary>
            <div className="app-advanced-body">
              <label className="app-field">
                <span className="app-field-label">{L("名称（可选）", "Name (optional)")}</span>
                <input
                  className="app-input"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={L("我的服务器", "My server")}
                  disabled={busy}
                />
              </label>
              <label className="app-field">
                <span className="app-field-label">{L("地址", "Address")}</span>
                <input
                  className="app-input mono"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="https://hub.example.com"
                  disabled={busy}
                />
              </label>
              <label className="app-field">
                <span className="app-field-label">{L("控制台令牌", "Console token")}</span>
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
                <RichText text={L("在工作区所在电脑上运行 {0} 获取令牌。", "Run {0} on the workspace's computer to get a token.")}
                  values={[<code>myco console token create --name {name.trim() || "<device-name>"}</code>]} />
              </div>
              <div className="app-actions">
                <button
                  type="button"
                  className="app-btn"
                  onClick={() => void connectManual()}
                  disabled={busy || !url.trim() || !token.trim()}
                >
                  {busy ? L("验证中…", "Verifying…") : L("用地址和令牌连接", "Connect with address and token")}
                </button>
              </div>
            </div>
          </details>

          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={close} disabled={busy}>{L("取消", "Cancel")}</button>
            <button type="submit" className="app-btn app-btn-primary" disabled={busy || !link.trim()}>
              {busy ? <><span className="app-spinner" />{L("正在连接…", "Connecting…")}</> : <><Link2 />{L("连接", "Connect")}</>}
            </button>
          </div>
        </form>
      ) : (
        <>
          <div className="seg seg-fill add-tabs" role="tablist" aria-label={L("怎么装", "How")}>
            {([["ssh", L("用 SSH 装到服务器上", "Over SSH")], ["command", L("复制命令", "Copy a command")]] as const).map(([v, label]) => (
              <button key={v} type="button" role="tab" aria-selected={how === v} className={cn("seg-btn", how === v && "active")} onClick={() => setHow(v)}>{label}</button>
            ))}
          </div>
          {how === "ssh" ? (
            <ConnectOver hubId="" via="ssh" purpose="hub" onConnected={(id) => void added(id)} />
          ) : (
          <>
          <div className="app-hint">
            {L("在你的服务器（Linux）上运行下面这行命令，装好后它会打印一个 App 连接链接，复制回来粘贴到上一步。", "Run this command on your server (Linux). When it’s done it prints an app link; copy it and paste it in the previous step.")}
          </div>
          <div className="app-command">
            <code>{INSTALL_COMMAND}</code>
            <CopyButton value={INSTALL_COMMAND} />
          </div>
          <div className="app-hint" style={{ marginTop: 12 }}>
            <RichText text={L("已经安装过工作区？在服务器上运行 {0} 生成新链接。", "Already installed a workspace? Run {0} on the server to make a new link.")} values={[<code>sudo myco console link</code>]} />
          </div>
          {/* Or the agent the person already has does all of it. */}
          <div className="add-agent-way" data-testid="connect-agent-way">
            <div className="app-field-label">{L("或者把这句话发给你的 agent，它会帮你装好、连上这个 App：", "Or send this to your agent; it installs everything and connects this App:")}</div>
            <div className="add-sentence">
              <p className="add-sentence-text">{setupPrompt()}</p>
              <div className="add-sentence-actions"><CopyButton value={setupPrompt()} /></div>
            </div>
          </div>
          </>
          )}
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={() => { setScreen("link"); setError(null); }}>
              <ArrowLeft />{L("返回", "Back")}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
