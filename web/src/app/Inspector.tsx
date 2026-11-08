import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import {
  accessLabel, agentKind, cliName, cliVersion, cloudLabel, isCloud, kindLabel, machineLabel,
  cleanLabel, groupsOf, machineOfRow, modelSourceLabel, nodeIcon, tagsOf,
} from "@/app/nodes";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { stateMeta } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";

type Action = null | "profile" | "allow" | "kick" | "delete";

const pillClass: Record<string, string> = { working: "green", pending: "amber", idle: "", offline: "" };

const isConsole = (name: string) => name.startsWith("@console:");

// MessageRows lists the node's recent threads, one row per thread: either
// the ones you (the console) take part in, or its exchanges with other
// nodes, which you can only read.
function MessageRows({ row, now, mine, onThread }: {
  row: AgentRow;
  now: number;
  mine: boolean;
  onThread: (hubId: string, threadId: string, agent: string) => void;
}) {
  const name = row.st.agent.name;
  const seen = new Set<string>();
  const msgs = (row.hub.overview?.recent || [])
    .filter((m) => m.from === name || m.to === name)
    .filter((m) => (isConsole(m.from) || isConsole(m.to)) === mine)
    .filter((m) => (seen.has(m.thread_id) ? false : (seen.add(m.thread_id), true)))
    .slice(0, 6);
  if (!msgs.length) return <div className="app-empty-note">{mine ? "你还没有和它对话" : "最近没有和其他节点的往来"}</div>;
  const label = (n: string) => (isConsole(n) ? "你" : n === name ? (row.st.agent.display_name || n) : n);
  return (
    <>
      {msgs.map((m) => (
        <button key={m.id} type="button" className="app-msg" onClick={() => onThread(row.hub.id, m.thread_id, name)}>
          <span className="meta">
            <span className="who">{label(m.from)}</span>→<span className="who">{label(m.to)}</span>
            <span className="time">{timeAgo(m.updated_at, now)}</span>
          </span>
          <span className="body">{m.body}</span>
        </button>
      ))}
    </>
  );
}

// LabelsEditor sets the node's group (one group=… label) and its tags (bare
// labels). Other key=value labels, such as machine=…, are left alone.
function LabelsEditor({ row, onRefresh }: { row: AgentRow; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const groups = groupsOf(ag);
  const tags = tagsOf(ag);
  const [group, setGroup] = useState(groups[0] || "");
  const [tag, setTag] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    setGroup(groupsOf(row.st.agent)[0] || "");
    setTag("");
    setError(null);
  }, [row.hub.id, row.st.agent.name, row.st.agent.labels.join(",")]);

  const known = [...new Set((row.hub.overview?.agents || []).flatMap((x) => groupsOf(x.agent)))].sort();

  const change = async (add: string[], remove: string[]) => {
    if (!add.length && !remove.length) return;
    setBusy(true);
    setError(null);
    try {
      await appApi.setLabels(row.hub.id, ag.name, { add, remove });
      await onRefresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const saveGroup = () => {
    const g = cleanLabel(group);
    if (g === (groups[0] || "") && groups.length <= 1) return;
    void change(g ? ["group=" + g] : [], groups.filter((x) => x !== g).map((x) => "group=" + x));
  };
  const addTag = () => {
    const t = cleanLabel(tag);
    if (!t || tags.includes(t)) { setTag(""); return; }
    void change([t], []).then(() => setTag(""));
  };

  return (
    <>
      <div className="app-list">
        <div className="app-kv app-labels-row">
          <span className="app-kv-label">分组</span>
          <span className="app-kv-value">
            <input
              className="app-label-input"
              value={group}
              list={`groups-${row.hub.id}`}
              placeholder="未分组"
              disabled={busy}
              aria-label="分组"
              onChange={(e) => setGroup(e.target.value)}
              onBlur={saveGroup}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) (e.target as HTMLInputElement).blur(); }}
            />
            <datalist id={`groups-${row.hub.id}`}>{known.map((g) => <option key={g} value={g} />)}</datalist>
          </span>
        </div>
        <div className="app-kv app-labels-row">
          <span className="app-kv-label">标签</span>
          <span className="app-kv-value app-label-list">
            {tags.map((t) => (
              <span key={t} className="app-label editable">
                #{t}
                <button type="button" aria-label={`去掉标签 ${t}`} disabled={busy} onClick={() => void change([], [t])}>×</button>
              </span>
            ))}
            <input
              className="app-label-input small"
              value={tag}
              placeholder="+ 标签"
              disabled={busy}
              aria-label="添加标签"
              onChange={(e) => setTag(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) addTag(); }}
              onBlur={() => tag.trim() && addTag()}
            />
          </span>
        </div>
      </div>
      <ErrorText error={error} />
    </>
  );
}

export function Inspector({ row, now, onClose, onRefresh, onThread, onMessage, onOpenMachine }: {
  row: AgentRow | null;
  now: number;
  onClose: () => void;
  onRefresh: () => Promise<void>;
  onThread: (hubId: string, threadId: string, agent: string) => void;
  onMessage: (row: AgentRow) => void;
  onOpenMachine: (hubId: string, machineId: string) => void;
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
  const kind = agentKind(ag);
  const Icon = nodeIcon(ag);
  const rt = ag.runtime;
  const machine = machineLabel(row);
  const machineObj = machineOfRow(row);

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
          {(kind === "agent" || kind === "shell") && (
            <button
              type="button"
              className="app-btn app-btn-primary app-inspector-send"
              onClick={() => onMessage(row)}
            >
              发消息
            </button>
          )}

          <section className="app-group">
            <div className="app-group-title"><span className="name">运行</span></div>
            <div className="app-list">
              <div className="app-kv">
                <span className="app-kv-label">类型</span>
                <span className="app-kv-value">{kindLabel(kind)}</span>
              </div>
              <div className="app-kv">
                <span className="app-kv-label">CLI</span>
                <span className="app-kv-value">
                  {kind === "human" ? "—" : <>{cliName(ag)}{cliVersion(ag) ? ` ${cliVersion(ag)}` : ""}</>}
                </span>
              </div>
              <div className="app-kv">
                <span className="app-kv-label">模型</span>
                <span className="app-kv-value">
                  {rt?.model || "—"}
                  {rt?.model && modelSourceLabel(rt.model_source) && (
                    <span className="app-kv-note">（{modelSourceLabel(rt.model_source)}）</span>
                  )}
                </span>
              </div>
              <div className="app-kv">
                <span className="app-kv-label">权限</span>
                <span className="app-kv-value">{accessLabel(rt?.access)}</span>
              </div>
              <div className="app-kv">
                <span className="app-kv-label">机器</span>
                <span className="app-kv-value">
                  {machineObj ? (
                    <button
                      type="button"
                      className="app-kv-link"
                      onClick={() => onOpenMachine(hubId, machineObj.id)}
                    >
                      {machineObj.name}<span className="app-chev">›</span>
                    </button>
                  ) : isCloud(ag) ? (
                    cloudLabel(ag)
                  ) : (
                    machine || "—"
                  )}
                </span>
              </div>
            </div>
          </section>

          <section className="app-group">
            <div className="app-group-title">
              <span className="name">分组和标签</span>
              <span className="app-group-note">用来筛选节点</span>
            </div>
            <LabelsEditor row={row} onRefresh={onRefresh} />
          </section>

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
                <button key={m.id} type="button" className="app-msg" onClick={() => onThread(hubId, m.thread_id, ag.name)}>
                  <span className="meta">
                    <span className="who">{isConsole(m.from) ? "你" : m.from}</span>→<span className="who">{isConsole(m.to) ? "你" : m.to}</span>
                    <span className="time">{timeAgo(m.updated_at, now)}</span>
                  </span>
                  <span className="body">{m.body}</span>
                </button>
              ))}
            </div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">和你的对话</span></div>
            <div className="app-list"><MessageRows row={row} now={now} mine onThread={onThread} /></div>
          </section>

          <section className="app-group">
            <div className="app-group-title">
              <span className="name">与其他节点的往来</span>
              <span className="app-group-note">只读</span>
            </div>
            <div className="app-list"><MessageRows row={row} now={now} mine={false} onThread={onThread} /></div>
          </section>

          <section className="app-group">
            <div className="app-group-title"><span className="name">身份与绑定</span></div>
            <div className="app-list">
              <div className="app-kv"><span className="app-kv-label">Agent ID</span><span className="app-kv-value mono">{ag.id}</span></div>
              <div className="app-kv"><span className="app-kv-label">会话 ID</span><span className="app-kv-value mono">{ag.binding.session_id || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">工作目录</span><span className="app-kv-value mono">{ag.binding.workdir || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">投递方式</span><span className="app-kv-value">{ag.delivery.mode}</span></div>
              <div className="app-kv"><span className="app-kv-label">入网令牌</span><span className="app-kv-value mono">{ag.enrolled_by || "管理员注册"}</span></div>
              <div className="app-kv"><span className="app-kv-label">允许来源</span><span className="app-kv-value">{ag.allow_from.length ? ag.allow_from.join(", ") : "任何人"}</span></div>
              <div className="app-kv"><span className="app-kv-label">最近在线</span><span className="app-kv-value">{timeAgo(ag.last_seen_at, now)}</span></div>
              <div className="app-kv"><span className="app-kv-label">注册时间</span><span className="app-kv-value">{new Date(ag.created_at).toLocaleString("zh-CN", { hour12: false })}</span></div>
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
