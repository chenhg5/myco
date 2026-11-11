import { useCallback, useEffect, useRef, useState } from "react";
import {
  AlertTriangle, Check, Link2, RefreshCw,
} from "lucide-react";
import { IconAsterisk, IconCode } from "@/components/icons";
import {
  appApi, type Hub, type HubOverview, type JoinLink, type LocalJoinResult,
  type LocalSession, type SpawnCli, type SpawnResult,
} from "@/appapi";
import type { RuntimeAccess } from "@/api";
import { CopyButton } from "@/app/copy";
import { NodePicker } from "@/app/NodePicker";
import { LocalShellAccess } from "@/app/LocalShellAccess";
import { agentKind } from "@/app/nodes";
import { ErrorText, Modal } from "@/app/dialogs";
import { cn, timeAgo } from "@/lib/utils";
import { L } from "@/lib/i18n";

export interface SpawnTarget {
  hubId?: string;
  machineId?: string;
  /** The project a new node joins (opened from a project page). */
  project?: { name: string; title: string };
}

type Tab = "local" | "link" | "new";

const TABS: { value: Tab; label: string }[] = [
  { value: "local", get label() { return L("本机的 agent", "Agent on this computer"); } },
  { value: "link", get label() { return L("其他电脑上的 agent", "Agent on another computer"); } },
  { value: "new", get label() { return L("在机器上新建", "New on a machine"); } },
];

const CLIS: { value: SpawnCli; label: string }[] = [
  { value: "claude", label: "Claude Code" },
  { value: "codex", label: "Codex" },
  { value: "pi", label: "Pi" },
  { value: "kiro", label: "Kiro" },
  { value: "cursor", label: "Cursor" },
  { value: "gemini", label: "Gemini CLI" },
  { value: "opencode", label: "OpenCode" },
  { value: "exec", get label() { return L("Shell 节点（远程执行命令）", "Shell node (runs commands)"); } },
];

const ACCESS: { value: RuntimeAccess; label: string }[] = [
  { value: "readonly", get label() { return L("只读", "Read-only"); } },
  { value: "workspace", get label() { return L("工作区", "Workspace"); } },
  { value: "full", get label() { return L("完全", "Full"); } },
];

const noSpawn = () => L("runner 没在运行，或 myco 版本太旧。", "their runner isn't running, or their myco is too old.");

function sameHub(a?: string, b?: string): boolean {
  if (!a || !b) return false;
  return a.replace(/\/+$/, "").toLowerCase() === b.replace(/\/+$/, "").toLowerCase();
}

function dirName(dir: string): string {
  const parts = dir.replace(/\\/g, "/").split("/").filter(Boolean);
  return parts[parts.length - 1] || dir || L("未命名目录", "Untitled folder");
}

function cliIcon(cli: string) {
  return cli === "codex" ? IconCode : IconAsterisk;
}

const isShell = (cli: string) => cli === "exec";

/** Opens a new session, joins one already on this computer, or hands a link
 *  to another computer. */
// One sentence for the agent on the other computer: install and register
// the machine (safe to re-run if myco is already there), then join the session.
function inviteSentence(command: string): string {
  return L(
    `接入 myco 网络：在终端运行 ${command} ，然后运行 myco join 把当前会话加入，并按它输出里 receive 的说明接收消息（会话开着时，在后台一直挂着 myco inbox wait --timeout 55m）。`,
    `Join the myco network: run ${command} in a terminal, then run myco join to add the current session, and receive messages as its "receive" output says (while the session stays open, keep myco inbox wait --timeout 55m running in the background).`,
  );
}

export function AddNodeDialog({ target, hubs, hubsOv, defaultHubId, onClose, onSpawned, onJoined, onMessage }: {
  target: SpawnTarget | null;
  hubs: Hub[];
  hubsOv: HubOverview[] | undefined;
  /** The Hub selected in the switcher, when it is a specific one. */
  defaultHubId?: string;
  onClose: () => void;
  onSpawned: (hubId: string, name: string) => void;
  /** Opens a new conversation with the node just added. */
  onMessage?: (hubId: string, name: string) => void;
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
  const [senders, setSenders] = useState<string[]>([]);
  const [access, setAccess] = useState<RuntimeAccess>("workspace");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<SpawnResult | null>(null);
  const [startedAt, setStartedAt] = useState(0);
  const [, setTick] = useState(0);
  // The wait is long (the CLI runs a first turn on that machine): show it.
  useEffect(() => {
    if (!busy) return;
    const t = window.setInterval(() => setTick((n) => n + 1), 1000);
    return () => window.clearInterval(t);
  }, [busy]);

  // Local sessions on this computer.
  const [sessions, setSessions] = useState<LocalSession[] | null>(null);
  const [sessionsBusy, setSessionsBusy] = useState(false);
  const [sessionsError, setSessionsError] = useState<unknown>(null);
  const sessionsReq = useRef(false);
  const [selected, setSelected] = useState<LocalSession | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [forkJoin, setForkJoin] = useState(true);
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
    setSenders([]);
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
    if (!hubId || !machineId || !dir.trim() || (isShell(cli) && senders.length === 0)) return;
    setBusy(true);
    setError(null);
    try {
      setStartedAt(Date.now());
      const r = await appApi.spawn(hubId, machineId, isShell(cli)
        ? {
          cli,
          dir: dir.trim(),
          allow_from: senders,
          ...(name.trim() ? { name: name.trim() } : {}),
          ...(description.trim() ? { description: description.trim() } : {}),
        }
        : {
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
        fork: forkJoin,
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
      title={L("添加节点", "Add node")}
      description={L("把一个正在运行的会话接进来，或者开一个新的。", "Connect a running session, or start a new one.")}
      wide
    >
      <div className="seg seg-fill app-addnode-tabs" role="tablist" aria-label={L("添加方式", "How to add")}>
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
                {h.name}{h.ok ? "" : L("（连不上）", " (unreachable)")}
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
                <span>{L(`已加入为 ${joined.agent}`, `Joined as ${joined.agent}`)}</span>
              </div>
              {joined.runner !== "running" && (
                <div className="app-notice">
                  <AlertTriangle />
                  <span>{L(`这台电脑的 runner 没有在运行，节点可能收不到任务。runner：${joined.runner || "未知"}`, `The runner on this computer isn't running, so the node may not receive tasks. Runner: ${joined.runner || "unknown"}`)}</span>
                </div>
              )}
              <div className="app-hint">
                {L(`节点已经加到「${hub?.name || "Hub"}」，会在节点列表里标出来。`, `The node was added to “${hub?.name || "Hub"}” and is highlighted in the node list.`)}
              </div>
              <div className="app-modal-actions">
                <button type="button" className="app-btn app-btn-primary" onClick={onClose}>{L("完成", "Done")}</button>
              </div>
            </>
          ) : (
            <>
              <div className="app-addnode-bar">
                <span className="app-small-note">
                  {L("这台电脑上最近的 Claude Code 和 Codex 会话", "Recent Claude Code and Codex sessions on this computer")}
                </span>
                <button
                  type="button"
                  className="app-btn app-btn-small"
                  onClick={() => { setSessions(null); void loadSessions(); }}
                  disabled={sessionsBusy}
                  title={L("重新读取会话", "Reload sessions")}
                >
                  <RefreshCw className={cn(sessionsBusy && "animate-spin")} />{L("刷新", "Refresh")}
                </button>
              </div>

              {sessionsBusy && sessions === null ? (
                <div className="app-progress"><span className="app-spinner" />{L("正在读取本机会话…", "Reading sessions on this computer…")}</div>
              ) : sessionsError && sessions === null ? (
                <>
                  <ErrorText error={sessionsError} />
                  <div className="app-actions">
                    <button type="button" className="app-btn" onClick={() => void loadSessions()}>
                      <RefreshCw />{L("重试", "Retry")}
                    </button>
                  </div>
                </>
              ) : sessions && sessions.length === 0 ? (
                <div className="app-empty-note app-addnode-empty">
                  {L("这台电脑上最近没有 Claude Code 或 Codex 会话。先在终端里打开一个，再回来刷新。", "No recent Claude Code or Codex sessions on this computer. Open one in a terminal, then come back and refresh.")}
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
                        <span className="app-row-icon"><Icon /></span>
                        <span className="app-row-main">
                          <span className="app-row-title">{s.title || dirName(s.workdir)}</span>
                          <span className="app-row-sub">
                            {joinedElsewhere ? (
                              <span>{L(`已加入另一个 Hub · ${s.joined?.agent}`, `Joined another Hub · ${s.joined?.agent}`)}</span>
                            ) : joinedHere ? (
                              <span>{L(`已加入 · ${s.joined?.agent}`, `Joined · ${s.joined?.agent}`)}</span>
                            ) : (
                              <>
                                <span className="txt">{s.workdir}</span>
                                <span className="sep">·</span>
                                <span>{L(`最后活动 ${timeAgo(s.last_active)}`, `Last active ${timeAgo(s.last_active)}`)}</span>
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
                    <span className="app-field-label">{L("显示名（可选）", "Display name (optional)")}</span>
                    <input
                      className="app-input"
                      value={displayName}
                      onChange={(e) => setDisplayName(e.target.value)}
                      placeholder={L("留空按会话自动命名", "Leave empty to name it after the session")}
                      disabled={joinBusy || !selected}
                    />
                  </label>
                  <div className="app-field">
                    <span className="app-field-label">{L("加入方式", "How to join")}</span>
                    <div className="seg seg-fill" role="tablist">
                      <button type="button" role="tab" aria-selected={forkJoin} className={cn("seg-btn", forkJoin && "active")} onClick={() => setForkJoin(true)} disabled={joinBusy}>{L("复制一份加入（推荐）", "Join a copy (recommended)")}</button>
                      <button type="button" role="tab" aria-selected={!forkJoin} className={cn("seg-btn", !forkJoin && "active")} onClick={() => setForkJoin(false)} disabled={joinBusy}>{L("直接加入", "Join this session")}</button>
                    </div>
                    <span className="app-small-note">
                      {forkJoin
                        ? L("新节点带着这个会话的全部上下文，你原来的会话照常用，两边互不打扰。", "The new node keeps this session’s whole context; you keep using your session, and neither gets in the other’s way.")
                        : L("这个会话本身成为节点：别人的任务会接着它跑。你在终端里用它时，runner 会先让开。", "This session itself becomes the node: tasks continue it. While you use it in a terminal, the runner waits.")}
                    </span>
                  </div>
                  <ErrorText error={joinError} />
                  {joinBusy && (
                    <div className="app-progress"><span className="app-spinner" />{forkJoin ? L("正在复制会话并加入，可能要一分钟…", "Copying the session and joining; this can take a minute…") : L("正在加入…", "Joining…")}</div>
                  )}
                  <div className="app-modal-actions">
                    <button type="button" className="app-btn" onClick={close} disabled={joinBusy}>{L("取消", "Cancel")}</button>
                    <button
                      type="button"
                      className="app-btn app-btn-primary"
                      onClick={() => void join()}
                      disabled={!selected || !hubId || joinBusy}
                    >
                      {joinBusy ? L("正在加入…", "Joining…") : L("加入", "Join")}
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
            {L("给另一台电脑上的 agent 一个加入链接。那边接入后，节点会出现在这个 Hub 上。", "Give an agent on another computer a join link. Once it joins, the node appears on this Hub.")}
          </div>
          {linkBusy && !link && (
            <div className="app-progress"><span className="app-spinner" />{L("正在生成链接…", "Creating link…")}</div>
          )}
          {linkError && (
            <>
              <ErrorText error={linkError} />
              <div className="app-actions">
                <button type="button" className="app-btn" onClick={() => void makeLink(true)} disabled={linkBusy}>
                  <Link2 />{L("重试", "Retry")}
                </button>
              </div>
            </>
          )}
          {link && (
            <>
              <div className="app-field">
                <span className="app-field-label">{L("把这句话发给那台电脑上的 agent（Claude Code / Codex 会话），它会自己装好 myco 并加入：", "Send this sentence to the agent on that computer (a Claude Code or Codex session). It installs myco and joins by itself:")}</span>
                <div className="app-command">
                  <code>{inviteSentence(link.command)}</code>
                  <CopyButton value={inviteSentence(link.command)} />
                </div>
              </div>
              <div className="app-small-note" style={{ marginTop: 10 }}>
                {L("链接 7 天内有效，可加入 5 台机器。没有 agent 的话，在那台电脑的终端里照着这句话运行两条命令也一样。", "The link is valid for 7 days and can join 5 machines. Without an agent, run the two commands from the sentence in a terminal on that computer instead.")}
              </div>
              <div className="app-modal-actions">
                <button type="button" className="app-btn" onClick={() => void makeLink(true)} disabled={linkBusy}>
                  {linkBusy ? L("正在生成…", "Creating…") : L("重新生成", "Create new link")}
                </button>
                <button type="button" className="app-btn app-btn-primary" onClick={onClose}>{L("完成", "Done")}</button>
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
              <span>{L(`${result.agent} 已在 ${machine?.name || "那台机器"} 上启动`, `${result.agent} is running on ${machine?.name || "that machine"}`)}</span>
            </div>
            <div className="app-list app-spawned">
              <div className="app-kv"><span className="app-kv-label">{L("类型", "Type")}</span><span className="app-kv-value">{CLIS.find((c) => c.value === result.cli)?.label || result.cli || "—"}</span></div>
              <div className="app-kv"><span className="app-kv-label">{L("目录", "Directory")}</span><span className="app-kv-value mono">{result.workdir || dir}</span></div>
              {!isShell(cli) && <div className="app-kv"><span className="app-kv-label">{L("权限", "Access")}</span><span className="app-kv-value">{ACCESS.find((a) => a.value === (result.access || access))?.label || result.access}</span></div>}
            </div>
            <div className="app-hint">
              {target?.project
                ? L(`它已加入项目「${target.project.title}」，可以直接给它发任务了。`, `It joined the project “${target.project.title}”; you can give it tasks now.`)
                : L("它已经在节点列表里了（绿色标记），可以直接给它发任务。", "It is in the node list now (marked in green); you can give it tasks now.")}
            </div>
            <div className="app-modal-actions">
              {onMessage && !isShell(cli) && (
                <button type="button" className="app-btn" onClick={() => onMessage(hubId, result.agent)}>{L("给它发任务", "Give it a task")}</button>
              )}
              <button type="button" className="app-btn app-btn-primary" onClick={onClose}>{L("完成", "Done")}</button>
            </div>
          </>
        ) : (
          <form onSubmit={submit}>
            <label className="app-field">
              <span className="app-field-label">{L("机器", "Machine")}</span>
              <select
                className="app-select-field"
                value={machineId}
                onChange={(e) => setMachineId(e.target.value)}
                disabled={busy || machines.length === 0}
                required
              >
                {machines.length === 0 && <option value="">{L("没有可用的机器", "No machines available")}</option>}
                {machines.map((m) => (
                  <option key={m.id} value={m.id} disabled={!m.can_spawn}>
                    {m.name}{!m.can_spawn ? L("（不可用）", " (unavailable)") : !m.online ? L("（离线）", " (offline)") : ""}
                  </option>
                ))}
              </select>
            </label>
            {machines.length === 0 ? (
              <div className="app-small-note" style={{ marginTop: 6 }}>
                {L("这个 Hub 还没有上报机器。先到机器页添加一台。", "This Hub has no machines yet. Add one on the Machines page first.")}
              </div>
            ) : (
              <>
                {machine && !machine.online && (
                  <div className="app-notice">
                    <AlertTriangle />
                    <span>{L("这台机器现在离线，会话可能启动不了。", "This machine is offline right now, so the session may not start.")}</span>
                  </div>
                )}
                {unavailable.length > 0 && (
                  <div className="app-small-note" style={{ marginTop: 6 }} title={unavailable.map((m) => m.name).join("\n")}>
                    {L(`另有 ${unavailable.length} 台机器现在不能新建（列表里标了「不可用」）：${noSpawn()}`, `${unavailable.length} more machine${unavailable.length > 1 ? "s" : ""} can't start nodes now (marked unavailable): ${noSpawn()}`)}
                  </div>
                )}
              </>
            )}

            <label className="app-field">
              <span className="app-field-label">{L("类型", "Type")}</span>
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
              <span className="app-field-label">{L("目录", "Folder")}</span>
              <input
                className="app-input mono"
                value={dir}
                onChange={(e) => setDir(e.target.value)}
                placeholder={L("~/code/项目名", "~/code/project")}
                disabled={busy}
                required
              />
            </label>

            {isShell(cli) && (
              <>
                <div className="app-small-note">
                  {L("Shell 节点在这个目录里运行别的节点发来的命令，并把输出发回去。适合让 agent 在这台机器上跑构建、看日志、查状态。", "A shell node runs the commands other nodes send it, in this folder, and sends back the output. Handy for letting agents build, read logs or check status on this machine.")}
                </div>
                <div className="app-field">
                  <span className="app-field-label">{L("谁能发命令", "Who can send commands")}</span>
                  <NodePicker
                    nodes={(hubsOv?.find((h) => h.id === hubId)?.overview?.agents || []).map((x) => x.agent).filter((x) => agentKind(x) !== "machine")}
                    value={senders}
                    onChange={setSenders}
                    disabled={busy}
                  />
                </div>
                <LocalShellAccess machine={machine} what="shell" />
              </>
            )}

            <label className="app-field">
              <span className="app-field-label">{L("名称（可选）", "Name (optional)")}</span>
              <input
                className="app-input"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={L("留空自动命名", "Leave empty to name it automatically")}
                disabled={busy}
              />
            </label>

            <label className="app-field">
              <span className="app-field-label">{L("描述（可选）", "Description (optional)")}</span>
              <input
                className="app-input"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                disabled={busy}
              />
            </label>

            {!isShell(cli) && (<>
            <label className="app-field">
              <span className="app-field-label">{L("模型（可选）", "Model (optional)")}</span>
              <input
                className="app-input mono"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder={L("留空用 CLI 默认", "Leave empty for the CLI default")}
                disabled={busy}
              />
            </label>

            <div className="app-field">
              <span className="app-field-label">{L("权限", "Access")}</span>
              <div className="seg seg-fill" role="tablist" aria-label={L("权限", "Access")}>
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
            {access === "full" && <LocalShellAccess machine={machine} what="full" />}
            </>)}

            <ErrorText error={error} />
            {busy && (
              <div className="app-progress">
                <span className="app-spinner" />
                <span>
                  {isShell(cli) ? L("正在那台机器上添加…", "Adding it on that machine…") : L(`正在 ${machine?.name || "那台机器"} 上启动 ${CLIS.find((c) => c.value === cli)?.label || cli} 会话… ${Math.max(0, Math.round((Date.now() - startedAt) / 1000))} 秒`, `Starting a ${CLIS.find((c) => c.value === cli)?.label || cli} session on ${machine?.name || "that machine"}… ${Math.max(0, Math.round((Date.now() - startedAt) / 1000))}s`)}
                  {!isShell(cli) && <small>{L("它要在那台机器上先跑一轮简短的对话来建好会话，通常要半分钟到一分钟，取决于模型的速度。", "It runs one short turn on that machine to create the session; usually 30–60 seconds, depending on the model.")}</small>}
                </span>
              </div>
            )}
            <div className="app-modal-actions">
              <button type="button" className="app-btn" onClick={close} disabled={busy}>{L("取消", "Cancel")}</button>
              <button
                type="submit"
                className="app-btn app-btn-primary"
                disabled={busy || !hubId || !machineId || !dir.trim() || (isShell(cli) && senders.length === 0)}
              >
                {busy ? L("正在启动…", "Starting…") : isShell(cli) ? L("添加节点", "Add node") : L("开始会话", "Start session")}
              </button>
            </div>
          </form>
        )
      )}
    </Modal>
  );
}
