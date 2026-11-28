import { useState } from "react";
import { Modal } from "./dialogs";
import { L, E } from "@/lib/i18n";
import type { CodeHostConfirmation } from "@/appapi";

export type CodeHostAnswer = { token: string; allowPrivate: boolean } | null;
/** Runs in the trusted App origin. The application iframe never sees this input. */
export function CodeHostPrompt({ prompt, answer }: { prompt: CodeHostConfirmation; answer: (value: CodeHostAnswer) => void }) {
  const [token, setToken] = useState("");
  const [allowPrivate, setAllowPrivate] = useState(false);
  const provider = prompt.provider === "github" ? "GitHub" : prompt.provider === "gitlab" ? "GitLab" : "Gitea / Forgejo";
  const finish = (accepted: boolean) => {
    answer(accepted ? { token, allowPrivate } : null);
    setToken("");
  };
  return <Modal open title={L("连接代码托管账户", "Connect a code-host account")} onOpenChange={open => { if (!open) finish(false); }}
    description={L(E`连接 ${provider}：${prompt.host}`, E`Connect to ${provider}: ${prompt.host}`)}>
    <form onSubmit={event => { event.preventDefault(); finish(true); }}>
      <p className="app-modal-hint">{L("只读访问仓库、PR 和 Issue。凭证保存在本机，不会交给应用页面。", "Read repositories, pull requests and issues. Credentials stay on this computer and are never given to the app page.")}</p>
      {prompt.source === "token" ? <label className="app-field"><span className="app-field-label">{L("访问令牌", "Access token")}</span>
        <input className="app-input" type="password" value={token} onChange={e => setToken(e.target.value)} required minLength={8} maxLength={8192} autoComplete="new-password" autoFocus /></label>
        : <p className="app-modal-hint">{L("将使用这台电脑上该服务器的 CLI 登录，凭证会保存为此页面的独立副本。", "Use the CLI login for this server on this computer, saving a separate credential copy for this page.")}</p>}
      {prompt.private_network && <label className="app-field"><span>{L("此服务器指向本机或内网。仅在你信任该服务器时允许。", "This server resolves to this computer or a private network. Allow it only if you trust this server.")}</span>
        <span><input type="checkbox" checked={allowPrivate} onChange={e => setAllowPrivate(e.target.checked)} /> {L("允许连接此内网服务器", "Allow this private-network server")}</span></label>}
      <div className="app-modal-actions"><button type="button" className="app-btn" onClick={() => finish(false)}>{L("取消", "Cancel")}</button>
        <button type="submit" className="app-btn app-btn-primary" disabled={(prompt.source === "token" && token.trim().length < 8) || (prompt.private_network && !allowPrivate)}>{L("连接", "Connect")}</button></div>
    </form>
  </Modal>;
}
