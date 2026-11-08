import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { harnessInfo, stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";

type Action = null | "profile" | "allow" | "kick" | "delete";

const pillClass: Record<string, string> = { working: "green", pending: "amber", idle: "", offline: "" };

function MessageRows({ row, now, onThread }: { row: AgentRow; now: number; onThread: (hubId: string, threadId: string) => void }) {
  const msgs = (row.hub.overview?.recent || []).filter((m) => m.from === row.st.agent.name || m.to === row.st.agent.name).slice(0, 8);
  if (!msgs.length) return <div className="app-empty-note">最近没有消息</div>;
  return (
    <>
      {msgs.map((m) => (
        <button key={m.id} type="button" className="app-msg" onClick={() => onThread(row.hub.id, m.thread_id)}>
          <span className="meta">
            <span className="who">{m.from}</span>→<span className="who">{m.to}</span>
            <span className="time">{timeAgo(m.updated_at, now)}</span>
          </span>
          <span className="body">{m.body}</span>
        </button>
      ))}
    </>
  );
}

export function Inspector({ row, now, onClose, onRefresh, onThread }: {
  row: AgentRow | null;
  now: number;
  onClose: () => void;
  onRefresh: () => Promise<void>;
  onThread: (hubId: string, threadId: string) => void;
}) {
  const ag = row?.st.agent;
  const [action, setAction] = useState<Action>(null);
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [allowFrom, setAllowFrom] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    setAction(null);
    setError(null);
    setBusy(false);
    setDisplayName(ag?.display_name || "");
    setDescription(ag?.description || "");
    setAllowFrom((ag?.allow_from || []).join(", "));
    // Only reset when a different agent is opened, not on every 5s poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [row?.hub.id, ag?.id]);

  useEffect(() => {
    if (!row) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape" && !action) onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [row, action, onClose]);

  if (!row || !ag) return null;

  const hubId = row.hub.id;
  const state = stateMeta[row.st.state];
  const Icon = harnessInfo(ag.binding.harness).icon;
  const machine = ag.labels.find((l) => l.startsWith("machine="))?.slice("machine=".length);

  const openAction = (a: Exclude<Action, null>) => { setError(null); setAction(a); };
  const closeAction = () => { if (!busy) { setAction(null); setError(null); } };

  const run = async (fn: () => Promise<unknown>, after: "close-action" | "close-panel" | "stay") => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await onRefresh();
      if (after === "close-action") { setAction(null); }
      if (after === "close-panel") { setAction(null); onClose(); }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const saveProfile = () => void run(
    () => appApi.updateProfile(hubId, ag.name, { display_name: displayName.trim(), description: description.trim() }),
    "close-action",
  );
  const saveAllowFrom = () => void run(
    () => appApi.updateAllowFrom(hubId, ag.name, allowFrom.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean)),
    "close-action",
  );

  const current = row.st.current || [];

  return (
    <>
      <div className="app-inspector-backdrop" onClick={onClose} />
      <aside className="app-inspector" role="dialog" aria-label={`${ag.display_name || ag.name} 详情`}>
        <div className="app-inspector-head">
          <span className="app-row-icon"><Icon strokeWidth={1.75} /></span>
          <div className="titles">
            <h2>{ag.display_name || ag.name}</h2>
            <div className="sub">
              {ag.display_name ? `${ag.name} · ` : ""}{row.hub.name}{machine ? ` · ${machine}` : ""}
            </div>
          </div>
          <span className={cn("app-pill", pillClass[row.st.state])}>
            <span className={cn("dot", row.st.state)} />{state.label}
          </span>
          <button type="button" className="app-btn app-btn-plain" onClick={onClose} aria-label="关闭">
            <X />
          </button>
        </div>

        <div className="app-inspector-body">
          <section className="app-group">
            <div className="app-group-title"><span className="name">当前工作</span></div>
            <div className="app-list">
              <div className="app-counts">
                <span>处理中<b>{row.st.in_progress}</b></span>
                <span>待处理<b>{row.st.queued}</b></span>
                <span>已挂起<b>{row.st.held}</b></span>
              </div>
              {current.length === 0 && <div className="app-empty-note">暂无进行中的任务</div>}
              {current.map((m) => (
                <button key={m.id} type="button" className="app-msg" onClick={() => onThread(hubId, m.thread_id)}>
                  <span className="meta">
                    <span className="who">{m.from}</span>→<span className="who">{m.to}</span>
                    <span className="time">{timeAgo(m.updated_at, now)}</span>
                  </span>
                  <span className="body">{m.body}</span>
                </button>
              ))}
            </div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">最近消息</span></div>
            <div className="app-list"><MessageRows row={row} now={now} onThread={onThread} /></div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">身份与绑定</span></div>
            <div className="app-list">
              <div className="app-kv"><span className="app-kv-label">Agent ID</span><span className="app-kv-value mono">{ag.id}</span></div>
              <div className="app-kv"><span className="app-kv-label">会话 ID</span><span className="app-kv-value mono">{ag.binding.session_id || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">环境</span><span className="app-kv-value">{ag.binding.environment || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">工作目录</span><span className="app-kv-value mono">{ag.binding.workdir || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">投递方式</span><span className="app-kv-value">{ag.delivery.mode}</span></div>
              <div className="app-kv"><span className="app-kv-label">入网令牌</span><span className="app-kv-value mono">{ag.enrolled_by || "管理员注册"}</span></div>
              <div className="app-kv"><span className="app-kv-label">允许来源</span><span className="app-kv-value">{ag.allow_from.length ? ag.allow_from.join(", ") : "任何人"}</span></div>
              <div className="app-kv"><span className="app-kv-label">最近在线</span><span className="app-kv-value">{timeAgo(ag.last_seen_at, now)}</span></div>
              <div className="app-kv"><span className="app-kv-label">注册时间</span><span className="app-kv-value">{new Date(ag.created_at).toLocaleString()}</span></div>
            </div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">操作</span></div>
            <div className="app-list">
              <button type="button" className="app-action-row" onClick={() => openAction("profile")}>
                修改显示名和描述…<span className="app-chev">›</span>
              </button>
              <button type="button" className="app-action-row" onClick={() => openAction("allow")}>
                允许谁派任务…<span className="app-chev">›</span>
              </button>
              <button type="button" className="app-action-row danger" onClick={() => openAction("kick")}>
                踢下线<span className="app-chev">›</span>
              </button>
              <button type="button" className="app-action-row danger" onClick={() => openAction("delete")}>
                删除<span className="app-chev">›</span>
              </button>
            </div>
          </section>
        </div>
      </aside>

      <Modal
        open={action === "profile"}
        onOpenChange={(o) => !o && closeAction()}
        title="修改显示名和描述"
        description="显示名和描述会在所有 Hub 上生效。显示名留空则只显示名字。"
      >
        <label className="app-field">
          <span className="app-field-label">显示名</span>
          <input className="app-input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder={ag.name} />
        </label>
        <label className="app-field">
          <span className="app-field-label">描述</span>
          <textarea className="app-input" value={description} onChange={(e) => setDescription(e.target.value)} />
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>取消</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveProfile} disabled={busy}>{busy ? "保存中…" : "保存"}</button>
        </div>
      </Modal>

      <Modal
        open={action === "allow"}
        onOpenChange={(o) => !o && closeAction()}
        title="允许谁派任务"
        description="只有这些来源可以给它发消息。留空表示任何人都可以。"
      >
        <label className="app-field">
          <span className="app-field-label">允许的来源（逗号分隔）</span>
          <input className="app-input mono" value={allowFrom} onChange={(e) => setAllowFrom(e.target.value)} placeholder="glenn, pm-codex" />
        </label>
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={closeAction} disabled={busy}>取消</button>
          <button type="button" className="app-btn app-btn-primary" onClick={saveAllowFrom} disabled={busy}>{busy ? "保存中…" : "保存"}</button>
        </div>
      </Modal>

      <ConfirmDialog
        open={action === "kick"}
        onOpenChange={(o) => !o && closeAction()}
        title={`将 ${ag.name} 踢下线？`}
        description={`它会立即从 ${row.hub.name} 下线，并收不到新消息。`}
        consequences={<p>令牌立即作废，正在进行的任务会被中断。节点记录、消息和资料仍保留在 Hub 上，重新接入即可回来。</p>}
        confirmLabel="踢下线"
        busy={busy}
        error={error ? <ErrorText error={error} /> : undefined}
        onConfirm={() => void run(() => appApi.kick(hubId, ag.name), "close-action")}
      />
      <ConfirmDialog
        open={action === "delete"}
        onOpenChange={(o) => !o && closeAction()}
        title={`从 ${row.hub.name} 删除 ${ag.name}？`}
        description="这个节点会从 Hub 上永久删除。"
        consequences={<p>节点记录和令牌都会删除，必须重新接入才能回来。已有线程里的消息仍会保留。</p>}
        confirmLabel="删除"
        busy={busy}
        error={error ? <ErrorText error={error} /> : undefined}
        onConfirm={() => void run(() => appApi.removeAgent(hubId, ag.name), "close-panel")}
      />
    </>
  );
}
