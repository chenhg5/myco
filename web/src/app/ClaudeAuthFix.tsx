import { useEffect, useState } from "react";
import { KeyRound, LogIn, Terminal } from "lucide-react";
import { appApi, type ClaudeAuth } from "@/appapi";
import { ErrorText } from "@/app/dialogs";
import { L, E } from "@/lib/i18n";

/** Claude could not sign in when an agent started (expired account sign-in, no key). */
export function isClaudeAuthError(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e ?? "");
  return /failed to authenticate|oauth|not logged in|please run \/login|invalid api key|authentication_error|credit balance/i.test(msg);
}

/**
 * When Claude Code cannot sign in for an agent this computer starts: say so
 * plainly and offer the ways this computer has: a provider picked in
 * cc-switch, the API settings the terminal uses (Magpie and the like), or
 * signing in to a Claude account; picking one tries again.
 */
export function ClaudeAuthFix({ onRetry }: { onRetry: () => void }) {
  const [auth, setAuth] = useState<ClaudeAuth | null>(null);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [loginOpened, setLoginOpened] = useState(false);
  useEffect(() => { appApi.claudeAuth().then(setAuth).catch(setError); }, []);
  const use = async (way: string) => {
    setBusy(way);
    setError(null);
    try {
      setAuth(await appApi.setClaudeAuth(way));
      onRetry();
    } catch (e) {
      setError(e);
    } finally {
      setBusy("");
    }
  };
  const login = async () => {
    setBusy("account");
    setError(null);
    try {
      await appApi.setClaudeAuth("account");
      await appApi.agentTerminal("claude", "login");
      setLoginOpened(true);
    } catch (e) {
      setError(e);
    } finally {
      setBusy("");
    }
  };
  const ways = auth ? auth.cc_switch.length + (auth.shell?.length ? 1 : 0) : 0;
  return (
    <div className="app-notice ca" data-testid="claude-auth-fix">
      <KeyRound />
      <div>
        <div className="ca-title">{L("Claude 登录不上", "Claude could not sign in")}</div>
        <div className="ca-text">
          {auth?.using
            ? L(E`这台电脑启动的 agent 现在用「${auth.using_name || auth.using}」，它没能登录。换一种试试：`, E`Agents started here use “${auth.using_name || auth.using}”, which could not sign in. Try another way:`)
            : ways > 0
              ? L("这台电脑启动的 agent 用的是 Claude 账号登录，它过期了。你在终端里用的是另一种方式，选一个沿用：", "Agents started here use the Claude account sign-in, which has expired. Your terminal uses another way; pick one to use:")
              : L("这台电脑启动的 agent 用的是 Claude 账号登录，它过期了。重新登录一次就好：", "Agents started here use the Claude account sign-in, which has expired. Sign in again:")}
        </div>
        <div className="ca-ways">
          {auth?.cc_switch.map((p) => (
            <button key={p.ref} type="button" className="app-btn app-btn-small" disabled={!!busy} onClick={() => void use(p.ref)}>
              {busy === p.ref ? <span className="app-spinner" /> : null}
              {L(E`用 cc-switch 的「${p.name}」`, E`cc-switch: ${p.name}`)}{p.host ? <span className="ca-host">{p.host}</span> : null}{p.current ? <span className="ca-host">{L("终端正在用", "in use")}</span> : null}
            </button>
          ))}
          {!!auth?.shell?.length && (
            <button type="button" className="app-btn app-btn-small" disabled={!!busy} onClick={() => void use("shell")}>
              {busy === "shell" ? <span className="app-spinner" /> : <Terminal />}
              {L("沿用终端里的 API 设置", "Use the terminal's API settings")}{auth.shell_host ? <span className="ca-host">{auth.shell_host}</span> : null}
            </button>
          )}
          <button type="button" className="app-btn app-btn-small" disabled={!!busy} onClick={() => void login()}>
            {busy === "account" ? <span className="app-spinner" /> : <LogIn />}
            {L("重新登录 Claude 账号", "Sign in to a Claude account")}
          </button>
        </div>
        {loginOpened && (
          <div className="ca-text">
            {L("在打开的终端里输入 /login 登录，完成后回来点「再试一次」。", "Type /login in the terminal that opened and sign in, then come back and try again.")}
            {" "}<button type="button" className="app-group-action" onClick={onRetry}>{L("再试一次", "Try again")}</button>
          </div>
        )}
        <ErrorText error={error} />
      </div>
    </div>
  );
}
