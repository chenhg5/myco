import { useEffect, useState } from "react";
import { nodeName } from "@/app/nodes";
import { AlertTriangle } from "lucide-react";
import { appApi, type HubOverview } from "@/appapi";
import type { Agent } from "@/api";
import { ErrorText, Modal } from "@/app/dialogs";
import { cn, timeAgo } from "@/lib/utils";
import { L } from "@/lib/i18n";

const DAYS = [7, 30, 90];

/** Removes nodes that have been offline for a while: pick a Hub and how
 * long, see exactly which nodes go, then confirm. Only nodes you may
 * manage are listed (the Hub decides). */
export function CleanupDialog({ open, onOpenChange, hubs, defaultHub, onDone }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hubs: HubOverview[];
  defaultHub?: string;
  onDone: () => void;
}) {
  const ok = hubs.filter((h) => h.ok);
  const [hubId, setHubId] = useState("");
  const [days, setDays] = useState(30);
  const [preview, setPreview] = useState<Agent[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [removed, setRemoved] = useState<number | null>(null);

  useEffect(() => {
    if (!open) return;
    setHubId(ok.find((h) => h.id === defaultHub)?.id || ok[0]?.id || "");
    setDays(30);
    setRemoved(null);
    setError(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open || !hubId || removed !== null) return;
    let cancelled = false;
    setPreview(null);
    setError(null);
    appApi.cleanup(hubId, days, true)
      .then((r) => { if (!cancelled) setPreview(r.agents); })
      .catch((e) => { if (!cancelled) setError(e); });
    return () => { cancelled = true; };
  }, [open, hubId, days, removed]);

  const run = async () => {
    setBusy(true);
    setError(null);
    try {
      const r = await appApi.cleanup(hubId, days, false);
      setRemoved(r.agents.length);
      onDone();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const now = Date.now();
  return (
    <Modal open={open} onOpenChange={(o) => !busy && onOpenChange(o)} title={L("清理离线节点", "Clean up offline nodes")}
      description={removed === null ? L("每个接入过的会话都会留下一个节点，会话结束后就一直离线。这里删掉很久没上线的，只包括你能管理的节点。", "Every session that joins leaves a node, which stays offline after the session ends. This removes the ones not seen for a while, among the nodes you can manage.") : undefined}>
      {removed !== null ? (
        <>
          <p className="app-modal-note">{removed ? L(`已删除 ${removed} 个节点。`, `Removed ${removed} node${removed === 1 ? "" : "s"}.`) : L("没有需要删除的节点。", "Nothing to remove.")}</p>
          <div className="app-modal-actions"><button type="button" className="app-btn app-btn-primary" onClick={() => onOpenChange(false)}>{L("完成", "Done")}</button></div>
        </>
      ) : (
        <>
          <div className="app-cleanup-controls">
            {ok.length > 1 && (
              <select className="app-select-field" value={hubId} onChange={(e) => setHubId(e.target.value)} disabled={busy}>
                {ok.map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
              </select>
            )}
            <span className="app-field-label">{L("离线超过", "Offline for")}</span>
            <div className="seg">
              {DAYS.map((d) => (
                <button key={d} type="button" className={cn("seg-btn", days === d && "active")} onClick={() => setDays(d)} disabled={busy}>
                  {L(`${d} 天`, `${d} days`)}
                </button>
              ))}
            </div>
          </div>
          <div className="app-list app-cleanup-list">
            {preview === null ? (
              error ? null : <div className="app-empty-note">{L("正在查找…", "Looking…")}</div>
            ) : preview.length === 0 ? (
              <div className="app-empty-note">{L(`没有离线超过 ${days} 天的节点。`, `No nodes offline for ${days} days or more.`)}</div>
            ) : (
              preview.map((a) => (
                <div className="app-kv" key={a.name}>
                  <span className="app-kv-label">{nodeName(a)}</span>
                  <span className="app-kv-value">{a.last_seen_at ? L(`${timeAgo(a.last_seen_at, now)}在线`, `seen ${timeAgo(a.last_seen_at, now)}`) : L("从未上线", "never seen")}</span>
                </div>
              ))
            )}
          </div>
          {preview && preview.length > 0 && (
            <div className="app-warn">
              <AlertTriangle />
              <div>{L("删除不能撤销，会记入审计。节点在本机的配置还在；以后那个会话想回来，运行一次 myco join 即可。", "This can’t be undone and is audited. The node’s settings on its machine stay; the session can come back any time with myco join.")}</div>
            </div>
          )}
          <ErrorText error={error} />
          <div className="app-modal-actions">
            <button type="button" className="app-btn" onClick={() => onOpenChange(false)} disabled={busy}>{L("取消", "Cancel")}</button>
            <button type="button" className="app-btn app-btn-danger" onClick={() => void run()} disabled={busy || !preview || preview.length === 0}>
              {busy ? L("正在删除…", "Removing…") : preview && preview.length > 0 ? L(`删除 ${preview.length} 个节点`, `Remove ${preview.length} node${preview.length === 1 ? "" : "s"}`) : L("删除", "Remove")}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
