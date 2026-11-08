import { useCallback, useEffect, useRef, useState } from "react";
import {
  AlertTriangle, Check, Code2, Link2, RefreshCw, Sparkles,
} from "lucide-react";
import {
  appApi, type Hub, type HubOverview, type JoinLink, type LocalJoinResult,
  type LocalSession, type SpawnCli, type SpawnResult,
} from "@/appapi";
import type { Agent, RuntimeAccess } from "@/api";
import { CopyButton } from "@/app/copy";
import { ErrorText, Modal } from "@/app/dialogs";
import { cn, timeAgo } from "@/lib/utils";

export interface SpawnTarget { hubId?: string; machineId?: string }

type Tab = "local" | "link" | "new";

const TABS: { value: Tab; label: string }[] = [
  { value: "local", label: "本机的 agent" },
  { value: "link", label: "其他电脑上的 agent" },
  { value: "new", label: "在机器上开新会话" },
];

const CLIS: { value: SpawnCli; label: string }[] = [
  { value: "claude", label: "Claude Code" },
  { value: "codex", label: "Codex" },
  { value: "pi", label: "Pi" },
  { value: "kiro", label: "Kiro" },
  { value: "cursor", label: "Cursor" },
  { value: "gemini", label: "Gemini CLI" },
  { value: "opencode", label: "OpenCode" },
];

const ACCESS: { value: RuntimeAccess; label: string }[] = [
  { value: "readonly", label: "只读" },
  { value: "workspace", label: "工作区" },
  { value: "full", label: "完全" },
];

const NO_SPAWN = "这台机器的 myco 版本太旧，或者它的 runner 没在运行。";

function sameHub(a?: string, b?: string): boolean {
  if (!a || !b) return false;
  return a.replace(/\/+$/, "").toLowerCase() === b.replace(/\/+$/, "").toLowerCase();
}

function dirName(dir: string): string {
  const parts = dir.replace(/\\/g, "/").split("/").filter(Boolean);
  return parts[parts.length - 1] || dir || "未命名目录";
}

function cliIcon(cli: string) {
  return cli === "codex" ? Code2 : Sparkles;
}

/** Opens a new session, joins one already on this computer, or hands a link
 *  to another computer. */
// One sentence for the agent on the other computer: install and register
// the machine (safe to re-run if myco is already there), then join the session.
function inviteSentence(command: string): string {
  return `接入 myco 网络：在终端运行 ${command} ，然后运行 myco join 把当前会话加入。`;
}

export function AddNodeDialog({ target, hubs, hubsOv, defaultHubId, onClose, onSpawned, onJoined }: {
  target: SpawnTarget | null;
  hubs: Hub[];
  hubsOv: HubOverview[] | undefined;
  /** The Hub selected in the switcher, when it is a specific one. */
  defaultHubId?: string;
  onClose: () => void;
  onSpawned: (hubId: string, agent: Agent) => void;
  onJoined: (hubId: string, agent: string) => void;
}) {
  const [tab, setTab] = useState<Tab>("local");
  const [hubId, setHubId] = useState("");
  const [machineId, setMachineId] = useState("");
  const [cli, setCli] = useState<SpawnCli>("claude");
  const [dir, setDir] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [model, setModel] = useState("");
  const [access, setAccess] = useState<RuntimeAccess>("workspace");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<SpawnResult | null>(null);

  // Local sessions on this computer.
  const [sessions, setSessions] = useState<LocalSession[] | null>(null);
  const [sessionsBusy, setSessionsBusy] = useState(false);
  const [sessionsError, setSessionsError] = useState<unknown>(null);
  const sessionsReq = useRef(false);
  const [selected, setSelected] = useState<LocalSession | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [joinBusy, setJoinBusy] = useState(false);
  const [joinError, setJoinError] = useState<unknown>(null);
  const [joined, setJoined] = useState<LocalJoinResult | null>(null);

  // A link for another computer / machine.
  const [link, setLink] = useState<JoinLink | null>(null);
  const [linkBusy, setLinkBusy] = useState(false);
  const [linkError, setLinkError] = useState<unknown>(null);
  const linkReq = useRef<string | null>(null);

  const machinesFor = useCallback(
    (id: string) => hubsOv?.find((h) => h.id === id)?.overview?.machines || [],
    [hubsOv],
  );
  const machines = machinesFor(hubId);
  const machine = machines.find((m) => m.id === machineId);
  const hub = hubs.find((h) => h.id === hubId);

  useEffect(() => {
    if (!target) return;
    const firstHub = hubs.find((h) => h.ok && h.id !== "") || hubs[0];
    const initialHub = target.hubId || defaultHubId || firstHub?.id || "";
    const ms = machinesFor(initialHub);
    setTab(target.machineId ? "new" : "local");
    setHubId(initialHub);
    setMachineId(target.machineId || ms.find((m) => m.can_spawn)?.id || "");
    setCli("claude");
    setDir("");
    setName("");
    setDescription("");
    setModel("");
    setAccess("workspace");
    setBusy(false);
    setError(null);
    setResult(null);
    setSessions(null);
    setSessionsBusy(false);
    setSessionsError(null);
    sessionsReq.current = false;
    setSelected(null);
    setDisplayName("");
    setJoinBusy(false);
    setJoinError(null);
    setJoined(null);
    setLink(null);
    setLinkBusy(false);
    setLinkError(null);
    linkReq.current = null;
    // Only reset when the dialog opens for a new target, not on every poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target]);

  // The overview may arrive after the dialog opened; then pick a machine.
  useEffect(() => {
    if (!target || tab !== "new" || machineId) return;
    const first = machinesFor(hubId).find((m) => m.can_spawn);
    if (first) setMachineId(first.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, tab, hubId, machineId, hubsOv]);

  const loadSessions = useCallback(async () => {
    if (sessionsReq.current) return;
    sessionsReq.current = true;
    setSessionsBusy(true);
    setSessionsError(null);
    try {
      const r = await appApi.localSessions();
      setSessions(r.sessions);
    } catch (e) {
      setSessionsError(e);
    } finally {
      sessionsReq.current = false;
      setSessionsBusy(false);
    }
  }, []);

  useEffect(() => {
    if (target && tab === "local" && sessions === null && !sessionsError) void loadSessions();
  }, [target, tab, sessions, sessionsError, loadSessions]);

  const makeLink = useCallback(async (force = false) => {
    if (!hubId) return;
    if (!force && (linkBusy || linkReq.current === hubId)) return;
    linkReq.current = hubId;
    setLinkBusy(true);
    setLinkError(null);
    try {
      const r = await appApi.createJoinLink(hubId, { uses: 5, expires_hours: 168 });
      setLink(r);
    } catch (e) {
      linkReq.current = null;
      setLinkError(e);
    } finally {
      setLinkBusy(false);
    }
  }, [hubId, linkBusy]);

  useEffect(() => {
    if (target && tab === "link" && !link && linkReq.current !== hubId) void makeLink();
  }, [target, tab, hubId, link, makeLink]);

  const changeHub = (id: string) => {
    setHubId(id);
    setMachineId(machinesFor(id).find((m) => m.can_spawn)?.id || "");
    setSelected(null);
    setJoined(null);
    setJoinError(null);
    setLink(null);
    setLinkError(null);
    linkReq.current = null;
  };

  const close = () => { if (!busy && !joinBusy) onClose(); };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!hubId || !machineId || !dir.trim()) return;
    setBusy(true);
    setError(null);
    try {
      const r = await appApi.spawn(hubId, machineId, {
        cli,
        dir: dir.trim(),
        ...(name.trim() ? { name: name.trim() } : {}),
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(model.trim() ? { model: model.trim() } : {}),
        access,
      });
      setResult(r);
      onSpawned(hubId, r.agent);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  const join = async () => {
    if (!selected || !hubId) return;
    setJoinBusy(true);
    setJoinError(null);
    try {
      const r = await appApi.localJoin(hubId, {
        cli: selected.cli,
        session_id: selected.session_id,
        workdir: selected.workdir,
        ...(displayName.trim() ? { display_name: displayName.trim() } : {}),
      });
      setJoined(r);
      onJoined(hubId, r.agent);
    } catch (err) {
      setJoinError(err);
    } finally {
      setJoinBusy(false);
    }
  };

  const unavailable = machines.filter((m) => !m.can_spawn);

  return (
    <Modal
      open={!!target}
      onOpenChange={(o) => !o && close()}
      title="添加节点"
      description="把一个正在运行的会话接进来，或者开一个新的。"
      wide
    >
      <div className="seg seg-fill app-addnode-tabs" role="tablist" aria-label="添加方式">
        {TABS.map((t) => (
          <button
            key={t.value}
            type="button"
            role="tab"
            aria-selected={tab === t.value}
            className={cn("seg-btn", tab === t.value && "active")}
            onClick={() => { setTab(t.value); setError(null); }}
            disabled={busy || joinBusy}
          >
            {t.label}
          </button>
        ))}
      </div>

      {hubs.length > 1 && tab !== "link" && (
        <label className="app-field">
          <span className="app-field-label">Hub</span>
          <select
            className="app-select-field"
            value={hubId}
            onChange={(e) => changeHub(e.target.value)}
            disabled={busy || joinBusy}
          >
            {hubs.map((h) => (
              <option key={h.id} value={h.id} disabled={!h.ok}>
                {h.name}{h.ok ? "" : "（连不上）"}
              </option>
            ))}
          </select>
        </label>
      )}

      {tab === "local" && (
        <>
          {joined ? (
            <>
              <div className="app-success">
                <Check />
                <span>已加入为 {joined.agent}</span>
              </div>
              {joined.runner !== "running" && (
                <div className="app-notice">
                  <AlertTriangle />
                  <span>这台电脑的 runner 没有在运行，节点可能收不到任务。runner：{joined.runner || "未知"}</span>
                </div>
              )}
              <div className="app-hint">
                节点已经加到「{hub?.name || "Hub"}」，会在节点列表里标出来。
              </div>
              <div className="app-modal-actions">
                <button type="button" className="app-btn app-btn-primary" onClick={onClose}>完成</button>
              </div>
            </>
          ) : (
            <>
              <div className="app-addnode-bar">
                <span className="app-small-note">
                  这台电脑上最近的 Claude Code 和 Codex 会话
                </span>
                <button
                  type="button"
                  className="app-btn app-btn-small"
                  onClick={() => { setSessions(null); void loadSessions(); }}
                  disabled={sessionsBusy}
                  title="重新读取会话"
                >
                  <RefreshCw className={cn(sessionsBusy && "animate-spin")} />刷新
                </button>
              </div>

              {sessionsBusy && sessions === null ? (
                <div className="app-progress"><span className="app-spinner" />正在读取本机会话…</div>
              ) : sessionsError && sessions === null ? (
                <>
                  <ErrorText error={sessionsError} />
                  <div className="app-actions">
                    <button type="button" className="app-btn" onClick={() => void loadSessions()}>
                      <RefreshCw />重试
                    </button>
                  </div>
                </>
              ) : sessions && sessions.length === 0 ? (
                <div className="app-empty-note app-addnode-empty">
                  这台电脑上最近没有 Claude Code 或 Codex 会话。先在终端里打开一个，再回来刷新。
                </div>
              ) : sessions ? (
                <div className="app-list app-session-list">
                  {sessions.map((s) => {
                    const Icon = cliIcon(s.cli);
                    const joinedHere = !!s.joined && (sameHub(s.joined.hub, hub?.url) || !s.joined.hub);
                    const joinedElsewhere = !!s.joined && !joinedHere;
                    const isSel = selected?.session_id === s.session_id && selected?.cli === s.cli;
                    return (
                      <button
                        key={`${s.cli}:${s.session_id}`}
                        type="button"
                        className={cn("app-row app-session", isSel && "is-selected", s.joined && "is-joined")}
                        onClick={() => {
                          if (s.joined) return;
                          setSelected(s);
                          setJoinError(null);
                        }}
                        disabled={!!s.joined}
                      >
                        <span className="app-row-icon"><Icon strokeWidth={1.75} /></span>
                        <span className="app-row-main">
                          <span className="app-row-title">{s.title || dirName(s.workdir)}</span>
                          <span className="app-row-sub">
                            {joinedElsewhere ? (
                              <span>已加入另一个 Hub · {s.joined?.agent}</span>
                            ) : joinedHere ? (
                              <span>已加入 · {s.joined?.agent}</span>
                            ) : (
                              <>
                                <span className="txt">{s.workdir}</span>
                                <span className="sep">·</span>
                                <span>最后活动 {timeAgo(s.last_active)}</span>
                              </>
                            )}
                          </span>
                        </span>
                        {isSel && <span className="app-session-check"><Check /></span>}
                      </button>
                    );
                  })}
                </div>
              ) : null}

              {sessions && sessions.length > 0 && (
                <>
                  <label className="app-field">
                    <span className="app-field-label">显示名（可选）</span>
                    <input
                      className="app-input"
                      value={displayName}
                      onChange={(e) => setDisplayName(e.target.value)}
                      placeholder="留空按会话自动命名"
                      disabled={joinBusy || !selected}
                    />
                  </label>
                  <ErrorText error={joinError} />
                  {joinBusy && (
                    <div className="app-progress"><span className="app-spinner" />正在加入…</div>
                  )}
                  <div className="app-modal-actions">
                    <button type="button" className="app-btn" onClick={close} disabled={joinBusy}>取消</button>
                    <button
                      type="button"
                      className="app-btn app-btn-primary"
                      onClick={() => void join()}
                      disabled={!selected || !hubId || joinBusy}
                    >
                      {joinBusy ? "正在加入…" : "加入"}
                    </button>
                  </div>
                </>
              )}
            </>
          )}
        </>
      )}

      {tab === "link" && (
        <>
          <div className="app-hint" style={{ marginTop: 10 }}>
            给另一台电脑上的 agent 一个加入链接。那边接入后，节点会出现在这个 Hub 上。
          </div>
          {linkBusy && !link && (
            <div className="app-progress"><span className="app-spinner" />正在生成链接…</div>
          )}
          {linkError && (
            <>
              <ErrorText error={linkError} />
              <div className="app-actions">
                <button type="button" className="app-btn" onClick={() => void makeLink(true)} disabled={linkBusy}>
                  <Link2 />重试
                </button>
              </div>
            </>
          )}
          {link && (
            <>
              <div className="app-field">
                <span className="app-field-label">把这句话发给那台电脑上的 agent（Claude Code / Codex 会话），它会自己装好 myco 并加入：</span>
                <div className="app-command">
                  <code>{inviteSentence(link.command)}</code>
                  <CopyButton value={inviteSentence(link.command)} />
                </div>
              </div>
              <div className="app-small-note" style={{ marginTop: 10 }}>
                链接 7 天内有效，可加入 5 台机器。没有 agent 的话，在那台电脑的终端里照着这句话运行两条命令也一样。
              </div>
              <div className="app-modal-actions">
                <button type="button" className="app-btn" onClick={() => void makeLink(true)} disabled={linkBusy}>
                  {linkBusy ? "正在生成…" : "重新生成"}
                </button>
                <button type="button" className="app-btn app-btn-primary" onClick={onClose}>完成</button>
              </div>
            </>
          )}
        </>
      )}

      {tab === "new" && (
        result ? (
          <>
            <div className="app-success">
              <Check />
              <span>已启动 {result.agent.display_name || result.agent.name}</span>
            </div>
            <div className="app-reply">{result.reply}</div>
            <div className="app-hint">新节点已经加到节点列表里，会用绿色标记一会儿。</div>
            <div className="app-modal-actions">
              <button type="button" className="app-btn app-btn-primary" onClick={onClose}>完成</button>
            </div>
          </>
        ) : (
          <form onSubmit={submit}>
            <label className="app-field">
              <span className="app-field-label">机器</span>
              <select
                className="app-select-field"
                value={machineId}
                onChange={(e) => setMachineId(e.target.value)}
                disabled={busy || machines.length === 0}
                required
              >
                {machines.length === 0 && <option value="">没有可用的机器</option>}
                {machines.map((m) => (
                  <option key={m.id} value={m.id} disabled={!m.can_spawn}>
                    {m.name}{!m.can_spawn ? "（不可用）" : !m.online ? "（离线）" : ""}
                  </option>
                ))}
              </select>
            </label>
            {machines.length === 0 ? (
              <div className="app-small-note" style={{ marginTop: 6 }}>
                这个 Hub 还没有上报机器。先到机器页添加一台。
              </div>
            ) : (
              <>
                {machine && !machine.online && (
                  <div className="app-notice">
                    <AlertTriangle />
                    <span>这台机器现在离线，会话可能启动不了。</span>
                  </div>
                )}
                {unavailable.map((m) => (
                  <div className="app-small-note" key={m.id} style={{ marginTop: 6 }}>
                    {m.name} 不可用：{NO_SPAWN}
                  </div>
                ))}
              </>
            )}

            <label className="app-field">
              <span className="app-field-label">CLI</span>
              <select
                className="app-select-field"
                value={cli}
                onChange={(e) => setCli(e.target.value as SpawnCli)}
                disabled={busy}
              >
                {CLIS.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
              </select>
            </label>

            <label className="app-field">
              <span className="app-field-label">目录</span>
              <input
                className="app-input mono"
                value={dir}
                onChange={(e) => setDir(e.target.value)}
                placeholder="~/code/项目名"
                disabled={busy}
                required
              />
            </label>

            <label className="app-field">
              <span className="app-field-label">名称（可选）</span>
              <input
                className="app-input"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="留空自动命名"
                disabled={busy}
              />
            </label>

            <label className="app-field">
              <span className="app-field-label">描述（可选）</span>
              <input
                className="app-input"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                disabled={busy}
              />
            </label>

            <label className="app-field">
              <span className="app-field-label">模型（可选）</span>
              <input
                className="app-input mono"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder="留空用 CLI 默认"
                disabled={busy}
              />
            </label>

            <div className="app-field">
              <span className="app-field-label">权限</span>
              <div className="seg seg-fill" role="tablist" aria-label="权限">
                {ACCESS.map((o) => (
                  <button
                    key={o.value}
                    type="button"
                    role="tab"
                    aria-selected={access === o.value}
                    className={cn("seg-btn", access === o.value && "active")}
                    onClick={() => setAccess(o.value)}
                    disabled={busy}
                  >
                    {o.label}
                  </button>
                ))}
              </div>
            </div>
            {access === "full" && (
              <div className="app-warn">
                <AlertTriangle />
                <span>完全权限可以让它读写那台机器上的任何文件，需要在那台机器上本地允许。</span>
              </div>
            )}

            <ErrorText error={error} />
            {busy && (
              <div className="app-progress">
                <span className="app-spinner" />正在那台机器上启动会话，可能需要一两分钟…
              </div>
            )}
            <div className="app-modal-actions">
              <button type="button" className="app-btn" onClick={close} disabled={busy}>取消</button>
              <button
                type="submit"
                className="app-btn app-btn-primary"
                disabled={busy || !hubId || !machineId || !dir.trim()}
              >
                {busy ? "正在启动…" : "开始会话"}
              </button>
            </div>
          </form>
        )
      )}
    </Modal>
  );
}
