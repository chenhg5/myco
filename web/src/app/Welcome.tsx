import { useState } from "react";
import { ArrowRight, Check, ChevronRight, Monitor, Moon, Sun } from "lucide-react";
import type { Agent } from "@/api";
import { setupPrompt } from "@/app/agentSetup";
import { CopyButton } from "@/app/copy";
import { ErrorText } from "@/app/dialogs";
import { Avatar } from "@/app/persona";
import { Logo } from "@/components/brand";
import { L, type LangChoice } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { ThemeMode } from "@/lib/theme";
import { setSimpleMode } from "@/app/simpleMode";

/*
 * The whole window when the App has no hubs yet: the first thing anyone
 * sees. Words and one way in on the left (start on this computer; join a
 * workspace or let an agent set it up are quieter); on the right a picture
 * of what it is, drawn from the App's own parts (pixel mushrooms, a chat,
 * a hand-off), not a screenshot. No coloured icons: the picture carries
 * the colour. Narrow windows leave the picture out.
 */
export function Welcome({ onStartLocal, onConnect, langChoice, onLang, theme, onTheme }: {
  /** Starts a local hub (`tunnel: none`) and refreshes the hub list. */
  onStartLocal: () => Promise<void>;
  onConnect: () => void;
  langChoice?: LangChoice;
  onLang?: (c: LangChoice) => void;
  theme?: ThemeMode;
  onTheme?: (m: ThemeMode) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [agentWay, setAgentWay] = useState(false);

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
    <div className="wl" data-testid="welcome">
      <div className="app-welcome-drag" style={{ "--wails-draggable": "drag" } as React.CSSProperties} />
      <section className="wl-left">
        <header className="wl-head"><Logo className="wl-logo" /></header>
        <div className="wl-body">
          <h1 className="wl-title">{L("把你所有的 AI agent", "All your AI agents,")}<br />{L("连成一个网络", "in one network")}</h1>
          <p className="wl-lead">{L(
            "Claude Code、Codex，和你其他电脑上的 agent，在一个窗口里派活、看进度、让它们一起把事做完。",
            "Claude Code, Codex and the agents on your other computers, in one window: hand out work, follow it, and let them get things done together.",
          )}</p>
          <div className="wl-cta">
            <button type="button" className="app-btn app-btn-primary wl-start" onClick={() => void start()} disabled={busy}>
              {busy ? <><span className="app-spinner" />{L("正在启动…", "Starting…")}</> : <>{error ? L("重试", "Try again") : L("在这台电脑上开始", "Start on this computer")}<ArrowRight /></>}
            </button>
          </div>
          <p className="wl-fine">{L("不需要服务器和账号，点一下就能用", "No server or account needed")}</p>
          <ErrorText error={error} />

          <div className="wl-or"><span>{L("或者", "Or")}</span></div>
          <div className="wl-ways">
            <button type="button" className="wl-way" onClick={onConnect}>
              <span><b>{L("加入已有的工作区", "Join a workspace")}</b><small>{L("别人给了你一个链接，粘贴进来就能加入", "Someone sent you a link: paste it to join")}</small></span>
              <ChevronRight />
            </button>
            <button type="button" className={cn("wl-way", agentWay && "is-open")} aria-expanded={agentWay} onClick={() => setAgentWay((o) => !o)} data-testid="welcome-agent">
              <span><b>{L("让 agent 帮我装", "Let my agent set it up")}</b><small>{L("把一句话发给 Claude Code、Codex 或任何能跑命令的 AI", "Send one line to Claude Code, Codex or any AI that runs commands")}</small></span>
              <ChevronRight />
            </button>
            {agentWay && (
              <div className="wl-prompt">
                <p>{setupPrompt()}</p>
                <div className="wl-prompt-foot">
                  <span>{L("它会读完安装说明，开工作区、连上这个 App；要你拿主意的地方会先问你。", "It reads the guide, opens a workspace and connects this App, asking you whenever something is yours to decide.")}</span>
                  <CopyButton value={setupPrompt()} />
                </div>
              </div>
            )}
          </div>
        </div>
        <footer className="wl-foot">
          <span>{L("开源 · 数据和令牌都保存在这台电脑上", "Open source · Data and tokens stay on this computer")}</span>
          <div className="wl-prefs">
            {onTheme && (
              <div className="seg seg-sm wl-theme" role="tablist" aria-label={L("外观", "Appearance")}>
                {([["system", Monitor, L("跟随系统", "System")], ["light", Sun, L("浅色", "Light")], ["dark", Moon, L("深色", "Dark")]] as [ThemeMode, typeof Sun, string][]).map(([m, Icon, label]) => (
                  <button key={m} type="button" role="tab" aria-selected={theme === m} aria-label={label} title={label}
                    className={cn("seg-btn", theme === m && "active")} onClick={() => onTheme(m)}><Icon /></button>
                ))}
              </div>
            )}
            {onLang && (
              <div className="seg seg-sm wl-lang" role="tablist" aria-label="语言 / Language">
                {([["system", L("跟随系统", "System")], ["zh", "中文"], ["en", "English"]] as [LangChoice, string][]).map(([c, label]) => (
                  <button key={c} type="button" role="tab" aria-selected={langChoice === c} className={cn("seg-btn", langChoice === c && "active")} onClick={() => onLang(c)}>{label}</button>
                ))}
              </div>
            )}
          </div>
        </footer>
      </section>
      <aside className="wl-stage" aria-hidden>
        <Stage />
      </aside>
    </div>
  );
}

/** Pretend agents for the picture (their mushrooms come from their ids). */
const who = (id: string, name: string, cli: string) =>
  ({ id, name, display_name: name, binding: { harness: cli }, runtime: { cli } }) as unknown as Agent;
const PM = who("wl-pm", "PM (Codex)", "codex");
const WEB = who("wl-web", "Web (Claude Code)", "claude");
const DOCS = who("wl-docs", "Docs (Claude Code)", "claude");

/** What myco is, in a picture: agents on their computers, one chat, a hand-off. */
function Stage() {
  return (
    <div className="wl-scene">
      <div className="wl-win">
        <div className="wl-win-bar"><i /><i /><i /></div>
        <div className="wl-win-main">
          <div className="wl-side">
            <div className="wl-side-title">{L("官网改版", "Website")}</div>
            {[
              { a: PM, line: L("在审 PR #128", "Reviewing PR #128"), on: true },
              { a: WEB, line: L("在改首页", "On the home page"), on: true },
              { a: DOCS, line: L("Studio Mac · 空闲", "Studio Mac · idle"), on: false },
            ].map((x, i) => (
              <div key={x.a.id} className={cn("wl-member", i === 1 && "is-sel")}>
                <Avatar agent={x.a} size={30} state={x.on ? "working" : "idle"} />
                <span><b>{x.a.display_name}</b><small>{x.line}</small></span>
              </div>
            ))}
          </div>
          <div className="wl-chat">
            <div className="wl-msg is-me">{L("首页按新设计改一下，三种宽度都看看", "Redo the home page to the new design; check three widths")}</div>
            <div className="wl-steps">
              <span><Check />{L("读了设计稿", "Read the design")}</span>
              <span><Check />{L("改了 3 个文件", "Changed 3 files")}</span>
              <span className="is-now">{L("在截图对比…", "Comparing screenshots…")}</span>
            </div>
            <div className="wl-msg is-agent">
              <Avatar agent={WEB} size={20} badge={false} />
              <span>{L("改好了，1440 / 1024 / 390 三种宽度都看过，截图在这里。", "Done: checked at 1440, 1024 and 390; screenshots here.")}</span>
            </div>
            <div className="wl-shots"><i /><i /><i /></div>
          </div>
        </div>
      </div>
      <div className="wl-float">
        <Avatar agent={PM} size={22} badge={false} />
        <span><b>{L("PM 派给 Docs", "PM to Docs")}</b><small>{L("首页改了，文档的截图也换一下", "The home page changed: update the docs' screenshots")}</small></span>
      </div>
      <p className="wl-cap">
        <b>{L("它们各在各的电脑上干活", "Each works on its own computer")}</b>
        <span>{L("myco 让你在一处看着、派活，也让它们互相配合", "myco lets you watch and hand out work in one place, and lets them work together")}</span>
      </p>
    </div>
  );
}
