import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, CircleHelp, Gavel, Hourglass, ShieldCheck, XCircle } from "lucide-react";
import type { Notice } from "@/api";
import { appApi } from "@/appapi";
import { L } from "@/lib/i18n";
import { timeAgo } from "@/lib/utils";
import { Modal, ErrorText } from "@/app/dialogs";

/*
 * 待处理: what still waits for you (the hub's open needs-you notices, the
 * same the Dock badge counts), across your workspaces. A row takes you
 * there; for what looking settles (a task that failed or seems stuck) it
 * is marked read on the way. An ask, a decision or an approval stays until
 * you answer, decide or approve: the list says so.
 */

/** Asks, decisions and approvals: settled by doing, not by reading. */
export const actionable = (k: string) => k === "asking" || k === "decision" || k === "approval";

const KIND: Record<string, { label: () => string; icon: typeof CircleHelp }> = {
  asking: { label: () => L("在问你", "Asking you"), icon: CircleHelp },
  decision: { label: () => L("等你决定", "Waiting for your decision"), icon: Gavel },
  approval: { label: () => L("等你同意", "Waiting for your OK"), icon: ShieldCheck },
  failed: { label: () => L("没完成", "Not done"), icon: XCircle },
  run_failed: { label: () => L("工作流失败", "Workflow failed"), icon: AlertTriangle },
  stuck: { label: () => L("好像卡住了", "Seems stuck"), icon: Hourglass },
};

export type HubNotices = { hubId: string; hubName: string; notices: Notice[]; error?: unknown };

export function NoticesPanel({ open, onOpenChange, hubs, nameOf, onGo, onChanged }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** the workspaces to look in */
  hubs: { id: string; name: string }[];
  /** an agent's name as people see it */
  nameOf: (hubId: string, agent: string) => string;
  /** takes you to where a notice leads */
  onGo: (hubId: string, n: Notice) => void;
  /** the count changed (read here) */
  onChanged: () => void;
}) {
  const [lists, setLists] = useState<HubNotices[] | null>(null);
  const [busy, setBusy] = useState(false);
  const hubKey = hubs.map((h) => h.id).join(",");
  const load = useCallback(async () => {
    const out = await Promise.all(hubs.map(async (h): Promise<HubNotices> => {
      try {
        const f = await appApi.openNotices(h.id);
        return { hubId: h.id, hubName: h.name, notices: f.notices || [] };
      } catch (error) {
        return { hubId: h.id, hubName: h.name, notices: [], error };
      }
    }));
    setLists(out);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hubKey]);
  useEffect(() => {
    if (!open) return;
    setLists(null);
    void load();
  }, [open, load]);

  const all = (lists || []).flatMap((l) => l.notices.map((n) => ({ hubId: l.hubId, n })));
  const readable = all.filter(({ n }) => !actionable(n.kind));
  const many = hubs.length > 1;

  const go = (hubId: string, n: Notice) => {
    onOpenChange(false);
    onGo(hubId, n);
    if (!actionable(n.kind) && n.target) {
      void appApi.readNotices(hubId, n.target).then(onChanged).catch(() => undefined);
    }
  };
  const readAll = async () => {
    setBusy(true);
    try {
      await Promise.all([...new Set(readable.map((x) => x.hubId))].map((h) => appApi.readNotices(h)));
      onChanged();
      await load();
    } finally {
      setBusy(false);
    }
  };

  const who = (hubId: string, n: Notice) => {
    const a = n.actor || (n.target?.startsWith("agent:") ? n.target.slice(6) : "");
    return a.startsWith("@console:") ? a.slice(9) : a ? nameOf(hubId, a) : "";
  };

  return (
    <Modal open={open} onOpenChange={onOpenChange} title={L("待处理", "Needs you")} className="nt-modal"
      description={L("在等你的事，和 Dock 上的数字一致。点一下就去那里。", "What waits for you, as the number on the Dock counts it. Click one to go there.")}>
      {lists === null ? (
        <div className="nt-empty">{L("正在看…", "Looking…")}</div>
      ) : (
        <>
          {lists.filter((l) => l.error).map((l) => (
            <div key={l.hubId} className="nt-error">{many && <b>{l.hubName}</b>}<ErrorText error={l.error} onRetry={() => void load()} /></div>
          ))}
          {all.length === 0 && !lists.some((l) => l.error) && <div className="nt-empty">{L("没有在等你的事。", "Nothing waits for you.")}</div>}
          {all.length > 0 && (
            <ul className="nt-list">
              {all.map(({ hubId, n }) => {
                const k = KIND[n.kind] || { label: () => n.kind, icon: CircleHelp };
                const Icon = k.icon;
                const from = who(hubId, n);
                return (
                  <li key={`${hubId}:${n.id}`}>
                    <button type="button" className="nt-row" onClick={() => go(hubId, n)}>
                      <span className="nt-icon"><Icon /></span>
                      <span className="nt-main">
                        <span className="nt-head">
                          <span className="nt-kind">{k.label()}</span>
                          {from && <span className="nt-who">{from}</span>}
                          {many && <span className="nt-hub">{lists.find((l) => l.hubId === hubId)?.hubName}</span>}
                        </span>
                        {n.subject && <span className="nt-subject">{n.subject}</span>}
                      </span>
                      <time className="nt-time" dateTime={n.created_at}>{timeAgo(n.created_at)}</time>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
          <div className="nt-foot">
            <span className="nt-hint">
              {L("在问你、等你决定、等你同意的，回答或决定后才会消失；其余的点开看过就消失。", "Asks, decisions and approvals go once you answer or decide; the rest go once you have looked.")}
            </span>
            {readable.length > 0 && (
              <button type="button" className="app-btn" disabled={busy} onClick={() => void readAll()}>
                {L(`全部标为已读（${readable.length} 条）`, `Mark ${readable.length} as read`)}
              </button>
            )}
          </div>
        </>
      )}
    </Modal>
  );
}
