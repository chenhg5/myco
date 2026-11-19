import { useState } from "react";
import { Bot, Laptop, Link2 } from "lucide-react";
import { setupPrompt } from "@/app/agentSetup";
import { CopyButton } from "@/app/copy";
import { ErrorText } from "@/app/dialogs";
import { Logo } from "@/components/brand";
import { L } from "@/lib/i18n";
import { setSimpleMode } from "@/app/simpleMode";

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
      // A new install, starting on this computer: one person, its own agents.
      setSimpleMode(true);
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
          <Logo className="h-9 w-auto" />
        </div>
        <h1>{L("把你所有的 AI agent 连成一个网络", "Connect all your AI agents into one network")}</h1>

        <div className="app-welcome-cards">
          <section className="app-welcome-card">
            <div className="app-welcome-icon"><Laptop strokeWidth={1.75} /></div>
            <h2>{L("在这台电脑上开始", "Start on this computer")}</h2>
            <p>{L("不需要服务器和账号，点一下就能用。以后想让别的电脑或同事加入，随时可以打开。", "Start a Hub on this computer. No server or account needed, and you can open it to the internet any time.")}</p>
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
            <h2>{L("加入已有的工作区", "Join an existing workspace")}</h2>
            <p>{L("别人给了你一个链接？粘贴进来就能加入。", "Paste an app link. No Hub yet? Install one on a server first.")}</p>
            <button type="button" className="app-btn app-btn-lg" onClick={onConnect}>
              {L("粘贴链接加入", "Join with a link")}
            </button>
          </section>

          {/* The third way: the agent people already have sets it all up. */}
          <section className="app-welcome-card app-welcome-agent" data-testid="welcome-agent">
            <div className="app-welcome-icon"><Bot strokeWidth={1.75} /></div>
            <h2>{L("让 agent 帮我装", "Let my agent set it up")}</h2>
            <p>{L("把这句话发给 Claude Code、Codex 或任何能跑命令的 AI：它会读完安装说明，帮你开工作区、连上这个 App、加电脑和 agent，出了问题自己排查；要你拿主意的地方会先问你。", "Send this to Claude Code, Codex or any AI that runs commands: it reads the setup guide, opens a workspace, connects this App, adds computers and agents, and sorts out problems itself, asking you whenever something is yours to decide.")}</p>
            <div className="add-sentence">
              <p className="add-sentence-text">{setupPrompt()}</p>
              <div className="add-sentence-actions"><CopyButton value={setupPrompt()} /></div>
            </div>
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
