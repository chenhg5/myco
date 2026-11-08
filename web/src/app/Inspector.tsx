import { useEffect, useState } from "react";
import { MessageSquare, X } from "lucide-react";
import { appApi } from "@/appapi";
import type { AgentRow } from "@/app/types";
import {
  accessLabel, agentKind, cliName, cliVersion, cloudLabel, isCloud, kindLabel, machineLabel,
  cleanLabel, groupsOf, machineOfRow, modelSourceLabel, nodeIcon, tagsOf,
} from "@/app/nodes";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
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

// LabelsEditor shows the node's group (a group=… label) and tags (bare
// labels), and edits them in a dialog. Other key=value labels, such as
// machine=…, are left alone.
function LabelsEditor({ row, onRefresh }: { row: AgentRow; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const groups = groupsOf(ag);
  const tags = tagsOf(ag);
  const [open, setOpen] = useState(false);
  const [group, setGroup] = useState("");
  const [draftTags, setDraftTags] = useState<string[]>([]);
  const [tag, setTag] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const others = (row.hub.overview?.agents || []).map((x) => x.agent);
  const knownGroups = [...new Set(others.flatMap(groupsOf))].sort((a, b) => a.localeCompare(b, "zh-CN"));
  const knownTags = [...new Set(others.flatMap(tagsOf))].sort((a, b) => a.localeCompare(b, "zh-CN"));

  const start = () => {
    setGroup(groups[0] || "");
    setDraftTags(tags);
    setTag("");
    setError(null);
    setOpen(true);
  };
  const addTag = (t: string) => {
    const v = cleanLabel(t);
    if (v && !draftTags.includes(v)) setDraftTags([...draftTags, v]);
    setTag("");
  };
  const save = async () => {
    const g = cleanLabel(group);
    const finalTags = cleanLabel(tag) ? [...new Set([...draftTags, cleanLabel(tag)])] : draftTags;
    const add = [...(g && !groups.includes(g) ? ["group=" + g] : []), ...finalTags.filter((t) => !tags.includes(t))];
    const remove = [...groups.filter((x) => x !== g).map((x) => "group=" + x), ...tags.filter((t) => !finalTags.includes(t))];
    if (!add.length && !remove.length) { setOpen(false); return; }
    setBusy(true);
    setError(null);
    try {
      await appApi.setLabels(row.hub.id, ag.name, { add, remove });
      await onRefresh();
      setOpen(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <button type="button" className="app-labels-summary" onClick={start}>
        <span className="app-labels-chips">
          {groups.length === 0 && tags.length === 0 && <span className="app-labels-empty">未分组，没有标签</span>}
          {groups.map((g) => <span key={"g" + g} className="app-label group">{g}</span>)}
          {tags.map((t) => <span key={"t" + t} className="app-label">#{t}</span>)}
        </span>
        <span className="app-labels-edit">编辑<span className="app-chev">›</span></span>
      </button>

      <Modal open={open} onOpenChange={(o) => !busy && setOpen(o)} title="分组和标签" description="分组和标签只用来在 App 里整理和筛选节点。">
        <label className="app-field">
          <span className="app-field-label">分组</span>
          <input className="app-input" value={group} onChange={(e) => setGroup(e.target.value)} placeholder="不分组" disabled={busy} />
        </label>
        {knownGroups.length > 0 && (
          <div className="app-label-picks">
            {knownGroups.map((g) => (
              <button key={g} type="button" className={cn("app-label group", cleanLabel(group) === g && "picked")} onClick={() => setGroup(g)}>{g}</button>
            ))}
          </div>
        )}
        <div className="app-field">
          <span className="app-field-label">标签</span>
          <div className="app-tag-box">
            {draftTags.map((t) => (
              <span key={t} className="app-label editable">
                #{t}
                <button type="button" aria-label={`去掉标签 ${t}`} onClick={() => setDraftTags(draftTags.filter((x) => x !== t))}>×</button>
              </span>
            ))}
            <input
              value={tag}
              onChange={(e) => setTag(e.target.value)}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return;
                if (e.key === "Enter" || e.key === ",") { e.preventDefault(); addTag(tag); }
                if (e.key === "Backspace" && !tag && draftTags.length) setDraftTags(draftTags.slice(0, -1));
              }}
              placeholder={draftTags.length ? "" : "输入后按回车"}
              aria-label="添加标签"
              disabled={busy}
            />
          </div>
        </div>
        {knownTags.filter((t) => !draftTags.includes(t)).length > 0 && (
          <div className="app-label-picks">
            {knownTags.filter((t) => !draftTags.includes(t)).map((t) => (
              <button key={t} type="button" className="app-label" onClick={() => addTag(t)}>+ #{t}</button>
            ))}
          </div>
        )}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          <button type="button" className="app-btn" onClick={() => setOpen(false)} disabled={busy}>取消</button>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void save()} disabled={busy}>
            {busy ? "正在保存…" : "保存"}
          </button>
        </div>
      </Modal>
    </>
  );
}

const hostKey = (h?: string) => (h || "").toLowerCase().replace(/\.local$/, "");

// OfflineCard says why a node is offline and how to bring it back: on this
// computer, restart the runner; on another one, the command to run there;
// a cloud session comes back when its session continues.
function OfflineCard({ row, now, onRefresh }: { row: AgentRow; now: number; onRefresh: () => Promise<void> }) {
  const ag = row.st.agent;
  const machine = machineOfRow(row);
  const [hostname, setHostname] = useState<string | undefined>();
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let off = false;
    setNote("");
    setError(null);
    appApi.local().then((l) => { if (!off) setHostname(l.hostname); }).catch(() => {});
    return () => { off = true; };
  }, [row.hub.id, ag.name]);
  const here = !!machine?.hostname && !!hostname && hostKey(machine.hostname) === hostKey(hostname);

  const restart = async () => {
    setBusy(true);
    setError(null);
    setNote("");
    try {
      const r = await appApi.restartRunner();
      if (r.runner !== "running") throw new Error(r.runner);
      setNote("runner 已重启，通常半分钟内会重新上线。");
      window.setTimeout(() => void onRefresh(), 8000);
      window.setTimeout(() => void onRefresh(), 30000);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const seen = ag.last_seen_at ? timeAgo(ag.last_seen_at, now) : "";
  return (
    <section className="app-offline">
      <div className="app-offline-head">
        <span className="dot offline" />
        <b>离线</b>
        {seen && <span>· 最后一次连接 Hub 是 {seen}</span>}
      </div>
      <p>
        在线是指它最近 90 秒内连过 Hub：节点所在机器上的 runner 会每隔几秒替它连一次。
        离线时发给它的任务会先排队，上线后自动送达。
      </p>
      {isCloud(ag) ? (
        <p>这是云端会话，会话结束就会离线。回到那个会话继续对话，它就会重新上线。</p>
      ) : here ? (
        <>
          <p>它在这台电脑上。重启这里的 runner，它就会重新连上 Hub。</p>
          <button type="button" className="app-btn app-btn-primary" onClick={() => void restart()} disabled={busy}>
            {busy ? <><span className="app-spinner" />正在重启…</> : "重启本机的 runner"}
          </button>
        </>
      ) : (
        <>
          <p>
            {machine ? <>它在「{machine.name}」上。</> : null}
            在那台机器的终端里运行下面这条命令（启动 runner；已经在运行就重启它）。机器关机或睡眠时，开机后会自动上线。
          </p>
          <div className="app-command">
            <code>myco runner service restart || myco runner service install</code>
            <CopyButton value="myco runner service restart || myco runner service install" />
          </div>
        </>
      )}
      {note && <p className="app-offline-ok">{note}</p>}
      <ErrorText error={error} />
    </section>
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

          {row.st.state === "offline" && <OfflineCard row={row} now={now} onRefresh={onRefresh} />}

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
            <div className="app-group-title"><span className="name">分组和标签</span></div>
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
            <div className="app-group-title">
              <span className="name">和你的对话</span>
              {(kind === "agent" || kind === "shell") && (
                <button type="button" className="app-group-action" onClick={() => onMessage(row)}>
                  <MessageSquare />发消息
                </button>
              )}
            </div>
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
