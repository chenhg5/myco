import { useState } from "react";
import { Loader2 } from "lucide-react";
import { appApi, type Hub } from "@/appapi";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorNote, Modal } from "@/app/dialogs";

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
      <form className="space-y-3" onSubmit={submit}>
        <label className="block space-y-1.5">
          <span className="text-xs font-medium text-muted-foreground">名称</span>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="工作室 Mac" autoFocus required />
        </label>
        <label className="block space-y-1.5">
          <span className="text-xs font-medium text-muted-foreground">地址</span>
          <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="http://192.168.1.20:7780" className="font-mono text-xs" required />
        </label>
        <label className="block space-y-1.5">
          <span className="text-xs font-medium text-muted-foreground">控制台令牌</span>
          <Input type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder="myco_console_…" className="font-mono text-xs" autoComplete="off" required />
        </label>
        <div className="rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
          在 Hub 所在机器上运行 <code className="rounded bg-background px-1 py-0.5 font-mono text-[11px] text-foreground">myco console token create --name {name.trim() || "<设备名>"}</code> 获取控制台令牌。
        </div>
        {error !== null && <ErrorNote error={error} />}
        <div className="flex justify-end gap-2 pt-1">
          <Button type="button" variant="outline" size="sm" onClick={() => change(false)} disabled={busy}>取消</Button>
          <Button type="submit" size="sm" disabled={busy || !name.trim() || !url.trim() || !token.trim()}>
            {busy && <Loader2 className="animate-spin" />}
            {busy ? "验证中…" : "添加 Hub"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
