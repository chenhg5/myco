import { useState } from "react";
import { appApi, type Hub } from "@/appapi";
import { ErrorText, Modal } from "@/app/dialogs";

export function AddHubDialog({ open, onOpenChange, onAdded }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAdded: (hub: Hub) => void;
}) {
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const reset = () => { setName(""); setUrl(""); setToken(""); setError(null); setBusy(false); };
  const change = (o: boolean) => { if (!o) reset(); onOpenChange(o); };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const hub = await appApi.addHub({ name: name.trim(), url: url.trim(), token: token.trim() });
      onAdded(hub);
      reset();
      onOpenChange(false);
    } catch (e) {
      setError(e);
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onOpenChange={change}
      title="添加 Hub"
      description="Hub 会先验证控制台令牌，再把它保存到钥匙串。令牌之后不会再出现在这个窗口里。"
    >
      <form onSubmit={submit}>
        <label className="app-field">
          <span className="app-field-label">名称</span>
          <input className="app-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="工作室 Mac" autoFocus required />
        </label>
        <label className="app-field">
          <span className="app-field-label">地址</span>
          <input className="app-input mono" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="http://192.168.1.20:7780" required />
        </label>
        <label className="app-field">
          <span className="app-field-label">控制台令牌</span>
          <input className="app-input mono" type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder="myco_console_…" autoComplete="off" required />
        </label>
        <div className="app-hint">
          在 Hub 所在机器上运行 <code>myco console token create --name {name.trim() || "<设备名>"}</code> 获取控制台令牌。
        </div>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => change(false)} disabled={busy}>取消</button>
          <button type="submit" className="app-btn app-btn-primary" disabled={busy || !name.trim() || !url.trim() || !token.trim()}>
            {busy ? "验证中…" : "添加 Hub"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
