import { useState } from "react";
import { Modal } from "./dialogs";
import { L, E } from "@/lib/i18n";
import type { CredentialConfirmation } from "@/appapi";

export type CredentialAnswer = { token: string; source: string; allowPrivate: boolean } | null;
/** Trusted host UI: the application never receives token input or CLI identity choices. */
export function CredentialPrompt({ prompt, answer }: { prompt: CredentialConfirmation; answer: (value: CredentialAnswer) => void }) {
  const [token, setToken] = useState("");
  const [source, setSource] = useState("");
  const [allowPrivate, setAllowPrivate] = useState(false);
  const finish = (accepted: boolean) => {
    answer(accepted ? { token, source, allowPrivate } : null);
    setToken("");
  };
  return <Modal open title={L("授权连接", "Authorize a connection")} onOpenChange={open => { if (!open) finish(false); }}
    description={L(E`连接 ${prompt.name}：${prompt.origin}`, E`Connect ${prompt.name}: ${prompt.origin}`)}>
    <form onSubmit={event => { event.preventDefault(); finish(true); }}>
      <p className="app-modal-hint">{L("凭证保存在本机，不会交给应用页面。仅允许读取以下地址。", "Credentials stay on this computer and are never given to the app page. Only reads to the following address are allowed.")}</p>
      <div className="app-kv"><div><span>{L("服务器", "Server")}</span><strong>{prompt.origin}</strong></div>
        <div><span>{prompt.verified ? L("已验证地址", "Verified address") : L("自定义地址", "Custom address")}</span></div>
        <div><span>{L("允许的路径", "Allowed paths")}</span><span>{prompt.paths.map(p => p.path + (p.prefix ? "*" : "")).join(", ")}</span></div></div>
      <label className="app-field"><span><input type="radio" name="credential-source" checked={!source} onChange={() => setSource("")} /> {L("访问令牌", "Access token")}</span></label>
      {prompt.sources.map(s => <label key={s.id} className="app-field"><span><input type="radio" name="credential-source" checked={source === s.id} onChange={() => setSource(s.id)} /> {s.cli} · {s.host} · {s.login}</span></label>)}
      {!source && <label className="app-field"><span className="app-field-label">{L("访问令牌", "Access token")}</span>
        <input className="app-input" type="password" value={token} onChange={e => setToken(e.target.value)} required minLength={8} maxLength={8192} autoComplete="new-password" autoFocus /></label>}
      {prompt.private_network && <label className="app-field"><span>{L("此服务器指向本机或内网。仅在你信任该服务器时允许。", "This server resolves to this computer or a private network. Allow it only if you trust this server.")}</span>
        <span><input type="checkbox" checked={allowPrivate} onChange={e => setAllowPrivate(e.target.checked)} /> {L("允许连接此内网服务器", "Allow this private-network server")}</span></label>}
      <div className="app-modal-actions"><button type="button" className="app-btn" onClick={() => finish(false)}>{L("取消", "Cancel")}</button>
        <button type="submit" className="app-btn app-btn-primary" disabled={(!source && token.trim().length < 8) || (prompt.private_network && !allowPrivate)}>{L("连接", "Connect")}</button></div>
    </form>
  </Modal>;
}
