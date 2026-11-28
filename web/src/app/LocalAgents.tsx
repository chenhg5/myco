import { Fragment, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { Bot, ChevronDown, RotateCcw, Sparkles } from "lucide-react";
import { appApi, type LocalAgent } from "@/appapi";
import { IconAsterisk, IconCode } from "@/components/icons";
import { CopyButton } from "@/app/copy";
import { ErrorText } from "@/app/dialogs";
import { useContextMenu } from "@/app/ContextMenu";
import { recall, remember } from "@/lib/memo";
import { L, E } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** What a terminal was opened for, per CLI, and whether it could be. */
type Opened = { action: "install" | "install_mirror" | "login"; opened: boolean; command: string };

function iconOf(cli: string) {
  if (cli === "claude") return IconAsterisk;
  if (cli === "codex") return IconCode;
  if (cli === "gemini") return Sparkles;
  return Bot;
}

function stateText(a: LocalAgent): string {
  switch (a.state) {
    case "ready": return a.version ? L(E`可以用了 · ${a.version}`, E`Ready · ${a.version}`) : L("可以用了", "Ready");
    case "signed_out": return L("装好了，还没登录", "Installed, not signed in");
    case "unknown": return L("看不出是否登录，能用就不用管。需要时可以去登录。", "Sign-in status is unknown. If it works, no action is needed. Sign in only if needed.");
    default: return a.needs === "npm"
      ? L("这台电脑上没装。安装要用 npm（装了 Node.js 就有）", "Not installed. Installing it needs npm (comes with Node.js)")
      : L("这台电脑上没装", "Not installed on this computer");
  }
}

function pillOf(a: LocalAgent): { cls: string; text: string } {
  switch (a.state) {
    case "ready": return { cls: "green", text: L("可用", "Ready") };
    case "signed_out": return { cls: "amber", text: L("未登录", "Signed out") };
    case "unknown": return { cls: "", text: L("已安装", "Installed") };
    default: return { cls: "", text: L("未安装", "Not installed") };
  }
}

/** The agent CLIs on this computer (Claude Code, Codex, Gemini, opencode):
 *  installed or not, signed in or not, with a button that opens a terminal
 *  to install or sign in. `only` limits it to some CLIs (the new-agent form
 *  shows the one picked). Checks again when the window comes back, which is
 *  when someone returns from the terminal. */
export function LocalAgentLines({ only, onReady, hideReady }: { only?: string[]; onReady?: (cli: string) => void; hideReady?: boolean }) {
  const [agents, setAgents] = useState<LocalAgent[] | null>(() => recall<LocalAgent[]>("local-agents") ?? null);
  const [error, setError] = useState<unknown>(null);
  const [checking, setChecking] = useState(false);
  const [opened, setOpened] = useState<Record<string, Opened>>({});
  const [busy, setBusy] = useState("");
  const menu = useContextMenu();
  const readyRef = useRef(onReady);
  readyRef.current = onReady;

  const load = useCallback(async () => {
    setChecking(true);
    try {
      const r = await appApi.localAgents();
      setAgents((prev) => {
        for (const a of r.agents) {
          const was = prev?.find((p) => p.cli === a.cli);
          if (a.state === "ready" && was && was.state !== "ready") readyRef.current?.(a.cli);
        }
        return r.agents;
      });
      remember("local-agents", r.agents);
      setError(null);
    } catch (e) {
      setError(e);
    } finally {
      setChecking(false);
    }
  }, []);

  useEffect(() => {
    void load();
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [load]);

  const open = async (cli: string, action: Opened["action"]) => {
    setBusy(cli);
    try {
      const r = await appApi.agentTerminal(cli, action);
      setOpened((o) => ({ ...o, [cli]: { action, ...r } }));
      setError(null);
    } catch (e) {
      setError(e);
    } finally {
      setBusy("");
    }
  };

  if (!agents) {
    return error ? <ErrorText error={error} onRetry={() => void load()} /> : <div className="app-empty-note">{L("正在看这台电脑上装了哪些 agent…", "Looking for agents on this computer…")}</div>;
  }
  // The main four always; the many others only once installed (else a new
  // computer lists a dozen "not installed").
  const shown = only ? agents.filter((a) => only.includes(a.cli)) : agents.filter((a) => MAIN_CLIS.includes(a.cli) || a.state !== "missing");
  const list = shown.filter((a) => !hideReady || a.state !== "ready");
  const more = only ? [] : agents.filter((a) => !MAIN_CLIS.includes(a.cli) && a.state === "missing");
  if (hideReady && list.length === 0 && !error) return null;
  const Wrap = hideReady ? CardWrap : Fragment;
  return (
    <Wrap>
      {list.map((a) => {
        const Icon = iconOf(a.cli);
        const pill = pillOf(a);
        // Once it is ready the terminal note has done its job.
        const o = a.state === "ready" ? undefined : opened[a.cli];
        return (
          <div className="app-setting-line la-line" key={a.cli} data-agent-cli={a.cli} data-agent-state={a.state}>
            <span className="la-icon"><Icon className="h-4 w-4" /></span>
            <div className="app-setting-line-text">
              <b>{a.name}</b>
              <span>{stateText(a)}</span>
              {o && (o.opened
                ? <span className="la-opened">{o.action === "login"
                  ? L("已经在终端里打开了。登录好之后回到这里，会自动重新检查。", "Opened in a terminal. Come back here once signed in: it checks again by itself.")
                  : L("已经在终端里打开了。装好之后回到这里，会自动重新检查。", "Opened in a terminal. Come back here once it is installed: it checks again by itself.")}</span>
                : <>
                  <span className="la-opened">{L("没能打开终端。复制这一行，到终端里运行：", "No terminal could be opened. Copy this line and run it in a terminal:")}</span>
                  <div className="app-command"><code>{o.command}</code><CopyButton value={o.command} /></div>
                </>)}
            </div>
            <span className={cn("app-pill", pill.cls)}>{pill.text}</span>
            {a.state === "missing" && (
              <button type="button" className="app-btn app-btn-quiet" disabled={busy === a.cli}
                onClick={(e) => {
                  if (!a.install_mirror) { void open(a.cli, "install"); return; }
                  menu.openUnder(e.currentTarget, [
                    { label: L("用官方方式安装", "Install the official way"), onSelect: () => void open(a.cli, "install") },
                    { label: L("用国内镜像安装（npm）", "Install from the China npm mirror"), onSelect: () => void open(a.cli, "install_mirror") },
                  ]);
                }}>
                {L("安装", "Install")}{a.install_mirror && <ChevronDown className="h-3.5 w-3.5" />}
              </button>
            )}
            {a.state === "signed_out" && (
              <button type="button" className="app-btn app-btn-quiet" disabled={busy === a.cli} onClick={() => void open(a.cli, "login")}>{L("登录", "Sign in")}</button>
            )}
            {a.state === "unknown" && (
              <button type="button" className="app-linklike" disabled={busy === a.cli} onClick={() => void open(a.cli, "login")}>{L("去登录", "Go to sign in")}</button>
            )}
          </div>
        );
      })}
      {error ? <div className="app-setting-line"><ErrorText error={error} onRetry={() => void load()} /></div> : null}
      {!only && (
        <div className="app-setting-line la-foot">
          <span className="la-foot-text">{L("App 会在你回到这个窗口时重新检查。", "The app checks again when you come back to this window.")}</span>
          <button type="button" className="app-btn app-btn-quiet" disabled={checking} onClick={() => void load()}>
            <RotateCcw className={cn("h-3.5 w-3.5", checking && "animate-spin")} />{L("重新检查", "Check again")}
          </button>
        </div>
      )}
      {more.length > 0 && !hideReady && (
        <div className="app-small-note la-more" data-testid="more-agents">
          {L(E`还支持 ${more.map((a) => a.name).join("、")}：装好后会出现在这里。`, E`Also supported: ${more.map((a) => a.name).join(", ")}; once installed they show here.`)}
        </div>
      )}
      {menu.element}
    </Wrap>
  );
}

/** Always listed, installed or not; the others once installed. */
const MAIN_CLIS = ["claude", "codex", "gemini", "opencode"];

/** Used outside Settings (the new-agent form): its own card. */
function CardWrap({ children }: { children: ReactNode }) {
  return <section className="app-setting-card app-setting-lines la-card">{children}</section>;
}

/** Settings › Agents: the agent CLIs on this computer. */
export function LocalAgentsSection() {
  return (
    <>
      <section className="app-setting-card app-setting-lines">
        <div data-setting="local-agents" className="app-setting-line">
          <div className="app-setting-line-text">
            <b>{L("这台电脑上的 agent", "Agents on this computer")}</b>
            <span>{L("myco 用你自己装好、登录好的 Claude Code、Codex 这些 agent，用的是你已有的订阅，myco 不另外收费。没装或没登录的，点按钮会打开一个终端窗口帮你做。",
              "myco runs the agents you installed and signed in to yourself (Claude Code, Codex…), on the subscriptions you already have. For one that is missing or signed out, the button opens a terminal window that does it.")}</span>
          </div>
        </div>
      </section>
      <section className="app-setting-card app-setting-lines">
        <LocalAgentLines />
      </section>
    </>
  );
}
