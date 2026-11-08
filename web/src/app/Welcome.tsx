import { useState } from "react";
import { Laptop, Link2 } from "lucide-react";
import { ErrorText } from "@/app/dialogs";
import { LogoSymbol, LogoWordmark } from "@/components/brand";
import { L } from "@/lib/i18n";

/** The whole window when the App has no hubs yet. */
export function Welcome({ onStartLocal, onConnect }: {
  /** Starts a local hub (`tunnel: none`) and refreshes the hub list. */
  onStartLocal: () => Promise<void>;
  onConnect: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const start = async () => {
    setBusy(true);
    setError(null);
    try {
      await onStartLocal();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="app-welcome">
      <div className="app-welcome-drag" style={{ "--wails-draggable": "drag" } as React.CSSProperties} />
      <div className="app-welcome-inner">
        <div className="app-welcome-brand">
          <LogoSymbol className="h-8 w-8" />
          <LogoWordmark className="h-[19px] w-auto" />
        </div>
        <h1>{L("把你所有的 AI agent 连成一个网络", "Connect all your AI agents into one network")}</h1>

        <div className="app-welcome-cards">
          <section className="app-welcome-card">
            <div className="app-welcome-icon"><Laptop strokeWidth={1.75} /></div>
            <h2>{L("在这台电脑上开始", "Start on this computer")}</h2>
            <p>{L("在本机启动一个 Hub，不需要服务器和账号。之后可以随时开放到公网。", "Start a Hub on this computer. No server or account needed, and you can open it to the internet any time.")}</p>
            <button
              type="button"
              className="app-btn app-btn-primary app-btn-lg"
              onClick={() => void start()}
              disabled={busy}
            >
              {busy ? <><span className="app-spinner" />{L("正在启动…", "Starting…")}</> : error ? L("重试", "Retry") : L("一键启动", "Start now")}
            </button>
          </section>

          <section className="app-welcome-card">
            <div className="app-welcome-icon"><Link2 strokeWidth={1.75} /></div>
            <h2>{L("连接已有的 Hub", "Connect an existing Hub")}</h2>
            <p>{L("粘贴 App 连接链接。还没有 Hub？可以先在一台服务器上装一个。", "Paste an app link. No Hub yet? Install one on a server first.")}</p>
            <button type="button" className="app-btn app-btn-lg" onClick={onConnect}>
              {L("连接已有的 Hub", "Connect an existing Hub")}
            </button>
          </section>
        </div>

        <div className="app-welcome-error">
          <ErrorText error={error} />
        </div>
        <p className="app-welcome-foot">{L("数据和令牌都保存在这台电脑上。", "Data and tokens are saved on this computer.")}</p>
      </div>
    </div>
  );
}
