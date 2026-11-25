import { useEffect, useRef, useState, useSyncExternalStore, type Dispatch, type SetStateAction } from "react";
import { FileDown, Monitor, ShieldQuestion } from "lucide-react";
import type { ActionGrant, BrowserActionPermission, Agent, Approval, SuperagentState } from "@/api";
import { appApi, type JoinLink } from "@/appapi";
import { CopyButton } from "@/app/copy";
import { joinSentence } from "@/app/AddToProject";
import type { AgentRow } from "@/app/types";
import { ErrorText, Modal } from "@/app/dialogs";
import { agentKind, machinesOfHub, nodeName } from "@/app/nodes";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { markSuperagent, SHROOM } from "@/app/superagentMark";

export { SHROOM };

// One shared copy per workspace: the bottom-left menu, the chat and the
// approval cards read the same state, and only one poll runs per workspace.
type Entry = { state?: SuperagentState; approvals: Approval[]; grants?: ActionGrant[]; at: number; off?: boolean };
const store = new Map<string, Entry>();
const subs = new Set<() => void>();
const users = new Map<string, number>();
const timers = new Map<string, ReturnType<typeof setInterval>>();
let version = 0;
const emit = () => { version += 1; subs.forEach((f) => f()); };

/** Fetches a workspace's superagent and what it waits on you for. A hub
 *  without superagents (before v0.21.1) answers 404: none, quietly. */
export async function refreshSuperagent(hubId: string): Promise<void> {
  try {
    const state = await appApi.superagent(hubId);
    // What waits on you: your superagent's requests, and a project
    // coordinator's (a computer for its project).
    const approvals = (await appApi.approvals(hubId).catch(() => ({ approvals: [] as Approval[] }))).approvals;
    // Include pending grants so approval cards use the exact target and requested
    // permissions, rather than inferring them from a human-readable title.
    const grants = (await appApi.actionGrants(hubId, "").catch(() => ({ grants: [] as ActionGrant[] }))).grants;
    store.set(hubId, { state, approvals, grants, at: Date.now() });
    markSuperagent(hubId, state.agent);
  } catch {
    store.set(hubId, { approvals: [], at: Date.now(), off: true });
  }
  emit();
}

function watch(hubId: string): () => void {
  users.set(hubId, (users.get(hubId) || 0) + 1);
  if (!timers.has(hubId)) {
    void refreshSuperagent(hubId);
    timers.set(hubId, setInterval(() => void refreshSuperagent(hubId), 15_000));
  }
  return () => {
    const n = (users.get(hubId) || 1) - 1;
    users.set(hubId, n);
    if (n <= 0) { clearInterval(timers.get(hubId)); timers.delete(hubId); }
  };
}

/** A workspace's superagent (yours) and its pending approvals. */
export function useSuperagent(hubId?: string): Entry | undefined {
  useSyncExternalStore((f) => { subs.add(f); return () => { subs.delete(f); }; }, () => version);
  useEffect(() => (hubId ? watch(hubId) : undefined), [hubId]);
  return hubId ? store.get(hubId) : undefined;
}

/** Names the superagent: a new Shroom on one of your computers, or one of
 *  your agents. Also changes or removes it. */
export function SuperagentSetup({ open, onOpenChange, hubId, rows, current, onReady }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hubId: string;
  /** the workspace's rows (its agents) */
  rows: AgentRow[];
  /** the superagent now, if any */
  current?: Agent;
  /** it is set: open its chat */
  onReady: (agent: string) => void;
}) {
  const hub = rows.find((r) => r.hub.id === hubId)?.hub;
  const meId = hub?.me?.member?.id;
  const machines = hub ? machinesOfHub(hub).filter((m) => m.online && m.can_spawn && (!meId || !m.owner_id || m.owner_id === meId)) : [];
  const mine = rows.filter((r) => r.hub.id === hubId && agentKind(r.st.agent) === "agent" && (!meId || r.st.agent.owner_id === meId));
  const [how, setHow] = useState<"new" | "pick" | "link">("new");
  // 复制一句话: a one-use link whose agent becomes your superagent (the
  // hub does it from v0.21.5; for older ones this dialog does, as it sees
  // a new agent of yours appear while it waits).
  const [link, setLink] = useState<JoinLink | null>(null);
  const [linkError, setLinkError] = useState<unknown>(null);
  const before = useRef<Set<string> | null>(null);
  // who it was when this opened (current follows the same store, so it
  // changes the moment the new one is named)
  const was = useRef<string | undefined>(undefined);
  const [machine, setMachine] = useState("");
  const [cli, setCli] = useState<"claude" | "codex">("claude");
  const [pick, setPick] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setBusy(false);
    setHow(machines.length || !mine.length ? "new" : "pick");
    setMachine(machines[0]?.id || "");
    setPick(current?.name || "");
    setLink(null);
    setLinkError(null);
    before.current = null;
    was.current = current?.name;
    // Only when it opens: the lists refresh while it is open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open || how !== "link" || link) return;
    before.current = new Set(mine.map((r) => r.st.agent.name));
    appApi.createJoinLink(hubId, { uses: 1, expires_hours: 24, superagent: true }).then(setLink).catch(setLinkError);
    // (once per opening of this tab)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, how, link, hubId]);
  // Waiting for it to join: poll, and when a new agent of yours shows up,
  // it is the one (named superagent here if the hub has not already).
  const s = useSuperagent(open && how === "link" ? hubId : undefined);
  const joinedName = how === "link" && before.current ? mine.find((r) => !before.current?.has(r.st.agent.name))?.st.agent.name : undefined;
  useEffect(() => {
    if (!open || how !== "link" || !link) return;
    const t = setInterval(() => void refreshSuperagent(hubId), 3000);
    return () => clearInterval(t);
  }, [open, how, link, hubId]);
  useEffect(() => {
    if (!open || how !== "link" || !link) return;
    const named = s?.state?.agent?.name;
    if (named && named !== was.current) { onOpenChange(false); onReady(named); return; }
    if (joinedName && named !== joinedName) {
      appApi.setSuperagent(hubId, joinedName).then(() => refreshSuperagent(hubId)).catch(setLinkError);
    }
  }, [open, how, link, s?.state?.agent?.name, joinedName, hubId, onOpenChange, onReady]);
  const sentence = link ? `${joinSentence(link)}${L(" 加入后运行 myco superagent guide，照它说的做：你是我在这个工作区的 superagent。", " Once joined, run myco superagent guide and do as it says: you are my superagent in this workspace.")}` : "";

  const close = () => { if (!busy) onOpenChange(false); };
  const ready = how === "new" ? !!machine : !!pick && pick !== current?.name;
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      let name = pick;
      if (how === "new") {
        const r = await appApi.spawn(hubId, machine, { cli, dir: "~/myco/shroom", access: "workspace", display_name: SHROOM });
        name = r.agent;
      }
      await appApi.setSuperagent(hubId, name);
      await refreshSuperagent(hubId);
      onOpenChange(false);
      onReady(name);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    setBusy(true);
    setError(null);
    try {
      await appApi.clearSuperagent(hubId);
      await refreshSuperagent(hubId);
      onOpenChange(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onOpenChange={(o) => { if (!o) close(); }} wide
      title={current ? L("换一个 superagent", "Change your superagent") : "superagent"}
      description={L(
        `superagent 是你在这个工作区的总管，名字叫 ${SHROOM}：它能看全工作区，帮你开 agent、建项目、派活、盯进度。删东西、停东西、改别人的权限这类事，它会先来问你。它不能以你的名义说话。`,
        `Your superagent, ${SHROOM}, runs this workspace for you: it sees everything, starts agents, makes projects, hands out work and keeps track. Deleting, stopping or changing someone's access, it asks you first. It never speaks as you.`)}>
      <form onSubmit={(e) => { e.preventDefault(); if (ready) void submit(); }} data-testid="superagent-setup">
        <div className="seg seg-fill add-tabs" role="tablist" aria-label={L("怎么配", "How")}>
          <button type="button" role="tab" aria-selected={how === "new"} className={cn("seg-btn", how === "new" && "active")} onClick={() => setHow("new")} disabled={busy}>{L(`新开 ${SHROOM}`, `New ${SHROOM}`)}</button>
          <button type="button" role="tab" aria-selected={how === "pick"} className={cn("seg-btn", how === "pick" && "active")} onClick={() => setHow("pick")} disabled={busy}>{L("用已有的", "Use one")}</button>
          <button type="button" role="tab" aria-selected={how === "link"} className={cn("seg-btn", how === "link" && "active")} onClick={() => setHow("link")} disabled={busy} data-testid="superagent-link-tab">{L("一句话接入", "By a sentence")}</button>
        </div>
        {how === "link" ? (
          <div className="add-pane sa-link" data-testid="superagent-link">
            <div className="app-field-label">{L("把这句话发给一个 Claude Code 或 Codex 会话", "Send this to a Claude Code or Codex session")}</div>
            {link ? (
              <div className="add-sentence">
                <p className="add-sentence-text">{sentence}</p>
                <div className="add-sentence-actions"><CopyButton value={sentence} /></div>
              </div>
            ) : linkError ? <ErrorText error={linkError} /> : (
              <div className="add-sentence"><p className="add-sentence-text is-loading">{L("正在生成…", "Making it…")}</p></div>
            )}
            <p className="app-small-note sa-link-note">{L("本机、别的电脑或 claude.ai 云端的会话都行，它加入后就是你的 superagent。这句话 24 小时内有效、只能用一次；它等同凭据，只发给你自己的会话。", "Here, on another computer or in the cloud; once it joins, it is your superagent. It works once, for 24 hours; it is a credential, so send it only to your own session.")}</p>
          </div>
        ) : how === "new" ? (
          machines.length === 0 ? (
            <div className="app-empty-note">{L("你还没有在线、能开 agent 的电脑。先在「电脑」里连一台，或者用已有的 agent。", "None of your computers is online and able to start agents. Connect one under Computers, or use one of your agents.")}</div>
          ) : (<>
            <div className="app-field">
              <span className="app-field-label">{L("开在哪台电脑", "On which computer")}</span>
              <div className="ws-computers">
                {machines.map((m) => (
                  <button key={m.id} type="button" className={cn("ws-computer", machine === m.id && "active")} onClick={() => setMachine(m.id)} disabled={busy}>
                    <span className="ws-computer-name">{m.name}</span>
                  </button>
                ))}
              </div>
            </div>
            <div className="app-field">
              <span className="app-field-label">{L("用哪个 AI", "Which AI")}</span>
              <div className="seg seg-fill" role="tablist" aria-label={L("用哪个 AI", "Which AI")}>
                {(["claude", "codex"] as const).map((c) => (
                  <button key={c} type="button" role="tab" aria-selected={cli === c} className={cn("seg-btn", cli === c && "active")} onClick={() => setCli(c)} disabled={busy}>{c === "claude" ? "Claude Code" : "Codex"}</button>
                ))}
              </div>
              <span className="app-small-note">{L("它在那台电脑的 ~/myco/shroom 里干活。", "It works in ~/myco/shroom on that computer.")}</span>
            </div>
          </>)
        ) : (
          mine.length === 0 ? (
            <div className="app-empty-note">{L("你还没有自己的 agent。", "You have no agents of your own yet.")}</div>
          ) : (
            <div className="app-field">
              <span className="app-field-label">{L("让哪个 agent 当 superagent", "Which agent")}</span>
              <div className="ws-computers">
                {mine.map((r) => (
                  <button key={r.st.agent.name} type="button" className={cn("ws-computer", pick === r.st.agent.name && "active")} onClick={() => setPick(r.st.agent.name)} disabled={busy}>
                    <span className="ws-computer-name">{nodeName(r.st.agent, r.hub)}</span>
                    {r.st.agent.name === current?.name && <span className="ws-computer-sub">{L("现在是它", "Current")}</span>}
                  </button>
                ))}
              </div>
            </div>
          )
        )}
        <ErrorText error={error} />
        <div className="app-modal-actions">
          {current && <button type="button" className="app-btn app-btn-plain app-btn-danger-quiet" onClick={() => void remove()} disabled={busy} data-testid="superagent-remove">{L("不要 superagent 了", "Remove superagent")}</button>}
          {how === "link" && link ? (
            <span className="app-modal-hint sa-wait" data-testid="superagent-wait"><span className="app-spinner" />{L("等它加入，加入后会自动打开和它的对话", "Waiting for it to join; its chat opens then")}</span>
          ) : <span style={{ flex: 1 }} />}
          <button type="button" className="app-btn" onClick={close} disabled={busy}>{L("取消", "Cancel")}</button>
          {how !== "link" && <button type="submit" className="app-btn app-btn-primary" disabled={busy || !ready}>
            {busy ? <><span className="app-spinner" />{L("正在配…", "Setting up…")}</> : how === "new" ? L(`开一个 ${SHROOM}`, `Start ${SHROOM}`) : L("就用它", "Use it")}
          </button>}
        </div>
      </form>
    </Modal>
  );
}

/** How long until an approval lapses, said briefly. */
function leftOf(expires: string, now: number): string {
  const ms = new Date(expires).getTime() - now;
  if (ms <= 0) return L("已过期", "expired");
  const h = Math.floor(ms / 3_600_000);
  if (h >= 1) return L(`${h} 小时内有效`, `${h}h left`);
  return L(`${Math.max(1, Math.round(ms / 60_000))} 分钟内有效`, `${Math.max(1, Math.round(ms / 60_000))}m left`);
}

/** Whether an action_grant approval asks for keyboard and mouse (its
 *  title says so; the grant, when listed, says exactly). */
function wantsControl(a: Approval, grants?: ActionGrant[]): boolean {
  const g = grants?.find((x) => x.approval_id === a.id);
  if (g) return (g.requested || []).includes("remote.desktop.control");
  return !/profile|browser/i.test(a.what) && /操作|keyboard|control/i.test(a.what);
}

/** What an agent may do right now, above its chat's composer: on which
 *  computer (or which page it may export), until when, and 收回. */
export function GrantStrip({ hubId, agent, name }: { hubId: string; agent: string; name?: string }) {
  const s = useSuperagent(hubId);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  const items = (s?.grants || []).filter((g) => g.agent === agent && g.state === "active");
  if (!items.length) return null;
  const now = Date.now();
  const revoke = async (g: ActionGrant) => {
    setBusy(g.id);
    setError(null);
    try {
      await appApi.revokeActionGrant(hubId, g.id);
      await refreshSuperagent(hubId);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };
  return (
    <div className="sa-approvals" data-testid="action-grants">
      {items.map((g) => {
        const control = g.permissions.includes("remote.desktop.control");
        const page = g.target.kind === "page";
        const ssh = g.target.resource?.provider === "server.ssh";
        const browser = g.target.resource?.provider === "browser";
        const browserControl = g.permissions.includes("browser.control" satisfies BrowserActionPermission);
        const browserRun = g.permissions.includes("browser.run");
        const manage = g.permissions.includes("server.ssh.manage");
        const what = g.target_name || g.target.id;
        const left = g.expires_at ? Math.max(0, Math.round((new Date(g.expires_at).getTime() - now) / 60_000)) : 0;
        return (
          <div className="sa-approval sa-grant" key={g.id}>
            {page ? <FileDown className="sa-approval-icon sa-grant-icon" /> : <Monitor className="sa-approval-icon sa-grant-icon" />}
            <div className="sa-approval-text">
              <div className="sa-approval-what">
                {page
                  ? L(`${name || agent} 可以导出「${what}」`, `${name || agent} may export "${what}"`)
                  : browser
                  ? browserControl
                    ? L(`${name || agent} 可以操作浏览器 profile ${what}。操作会显示在你的屏幕上，你随时可以收回授权`, `${name || agent} may control browser profile ${what}. Actions appear on your screen, and you can revoke access at any time`)
                    : browserRun
                      ? g.permissions.includes("browser.view")
                        ? L(`${name || agent} 可以在 ${what} 上查看状态与运行结果，并运行主人批准的脚本`, `${name || agent} may view status and run results, and run owner-approved scripts on ${what}`)
                        : L(`${name || agent} 可以在 ${what} 上运行主人批准的脚本`, `${name || agent} may run owner-approved scripts on ${what}`)
                      : L(`${name || agent} 可以查看 ${what} 的状态与运行结果`, `${name || agent} may view status and run results in ${what}`)
                  : ssh
                  ? manage
                    ? L(`${name || agent} 可以读取并管理 ${what} 的服务与容器`, `${name || agent} may inspect and manage services and containers on ${what}`)
                    : L(`${name || agent} 可以读取 ${what} 的状态与日志`, `${name || agent} may read status and logs on ${what}`)
                  : control
                  ? L(`${name || agent} 可以看并操作 ${what} 的屏幕`, `${name || agent} may see and use ${what}'s screen`)
                  : L(`${name || agent} 可以看 ${what} 的屏幕`, `${name || agent} may see ${what}'s screen`)}
              </div>
              <div className="sa-approval-meta">{left > 0 ? L(`还剩 ${left} 分钟`, `${left} min left`) : L("快到期了", "Ending soon")}</div>
            </div>
            <div className="sa-approval-actions">
              <button type="button" className="app-btn app-btn-small" disabled={!!busy} onClick={() => void revoke(g)}>
                {busy === g.id ? <span className="app-spinner" /> : null}{L("收回", "Revoke")}
              </button>
            </div>
          </div>
        );
      })}
      <ErrorText error={error} />
    </div>
  );
}

/** What an agent waits on you for, above its chat's composer (your
 *  superagent's requests, a coordinator asking for a computer): what, why,
 *  and 同意 / 不同意. */
function automationRequestSummary(input: Record<string, unknown>): string {
  return [input.to ? L(`发给 ${input.to}`, `To ${input.to}`) : input.workflow ? L(`工作流 ${input.workflow}`, `Workflow ${input.workflow}`) : "",
    input.cron ? `${input.cron} (${input.tz || "UTC"})` : "",
    input.starts_at !== undefined ? (input.starts_at ? L(`开始 ${input.starts_at}`, `Starts ${input.starts_at}`) : L("清除开始限制", "No start bound")) : "",
    input.ends_at !== undefined ? (input.ends_at ? L(`结束 ${input.ends_at}`, `Ends ${input.ends_at}`) : L("清除结束限制", "No end bound")) : "",
    input.once === true ? L("只跑一次", "Run once") : input.once === false ? L("按计划重复执行", "Repeat on schedule") : "",
    input.enabled === false ? L("暂停", "Pause") : input.enabled === true ? L("启用", "Enable") : "",
    input.skip_if_busy === true ? L("忙碌时跳过", "Skip while busy") : input.skip_if_busy === false ? L("忙碌时也发送", "Send while busy") : "",
    input.webhook === false ? L("关闭 Webhook", "Disable webhook") : ""].filter(Boolean).join(" · ");
}
function automationRequestContent(a: Approval): string {
  const input = a.payload?.input as Record<string, unknown> | undefined;
  return typeof input?.body === "string" ? input.body : input?.inputs ? JSON.stringify(input.inputs, null, 2) : "";
}

function invitationWhat(a: Approval, grants?: ActionGrant[]): string {
  const grant = grants?.find(g => g.approval_id === a.id);
  if (a.kind === "action_grant" && grant?.target.resource?.provider === "browser") {
    const profile = grant.target.resource.name || grant.target.resource.id;
    const permissions = grant.requested || [];
    const minutes = /，(\d+) 分钟$/.exec(a.what)?.[1];
    const ttl = minutes ? L(`，${minutes} 分钟`, `, for ${minutes} minutes`) : "";
    if (permissions.includes("browser.control")) return L(`操作浏览器 profile「${profile}」`, `Control browser profile “${profile}”`) + ttl;
    if (permissions.includes("browser.run")) return (permissions.includes("browser.view")
      ? L(`在「${profile}」上查看状态与运行结果，并运行主人批准的脚本`, `View status and run results, and run owner-approved scripts in “${profile}”`)
      : L(`运行「${profile}」中主人批准的脚本`, `Run owner-approved scripts in “${profile}”`)) + ttl;
    return L(`查看「${profile}」的状态与运行结果`, `View status and run results in “${profile}”`) + ttl;
  }
  if (a.kind === "project_automation") {
    const action = String(a.payload?.action || "update");
    const input = a.payload?.input as Record<string, unknown> | undefined;
    const name = String(input?.name || a.payload?.task_name || a.payload?.id || L("定时任务", "scheduled task"));
    const project = String(a.payload?.project || "");
    const verb = action === "create" ? L("创建", "Create") : action === "delete" ? L("删除", "Delete") : L("修改", "Update");
    return L(`${verb}项目「${project}」的定时任务「${name}」`, `${verb} “${name}” in project “${project}”`);
  }
  if (a.kind !== "project_invite") return a.what;
  const project = String(a.payload?.project_title || a.payload?.project || "");
  const name = String(a.payload?.display_name || L("新 agent", "a new agent"));
  return L(`邀请「${name}」加入项目「${project}」`, `Invite ${name} to project “${project}”`);
}

export function ApprovalStrip({ hubId, agent, name }: { hubId: string; agent: string; name?: string }) {
  const s = useSuperagent(hubId);
  // Each request on its own: deciding one leaves the others to click
  // (a slow workspace used to grey them all out until it answered). One
  // decided goes at once; the list is read again behind.
  const [busy, setBusy] = useState<Set<string>>(() => new Set());
  const [done, setDone] = useState<Set<string>>(() => new Set());
  const [errors, setErrors] = useState<Record<string, unknown>>({});
  const items = (s?.approvals || []).filter((a) => a.agent === agent && a.status === "pending" && !done.has(a.id));
  const who = name || (s?.state?.agent?.name === agent ? SHROOM : agent);
  if (!items.length) return null;
  const now = Date.now();
  const mark = (set: Dispatch<SetStateAction<Set<string>>>, id: string, on: boolean) =>
    set((x) => { const n = new Set(x); if (on) n.add(id); else n.delete(id); return n; });
  const decide = async (a: Approval, ok: boolean, viewOnly = false) => {
    mark(setBusy, a.id, true);
    setErrors(({ [a.id]: _, ...rest }) => rest);
    try {
      await appApi.decideApproval(hubId, a.id, ok, viewOnly ? { permissions: ["remote.desktop.view"] } : undefined);
      mark(setDone, a.id, true);
      void refreshSuperagent(hubId).catch(() => undefined);
    } catch (e) {
      setErrors((x) => ({ ...x, [a.id]: e }));
    } finally {
      mark(setBusy, a.id, false);
    }
  };
  return (
    <div className="sa-approvals" data-testid="approvals">
      {items.map((a) => (
        <div className="sa-approval" key={a.id}>
          <ShieldQuestion className="sa-approval-icon" />
          <div className="sa-approval-text">
            <div className="sa-approval-what">{L(`${who} 想：`, `${who} wants to: `)}{invitationWhat(a, s?.grants)}</div>
            {a.kind === "project_automation" && <div className="sa-approval-why">{L("仅批准这一次变更，不会打开项目的长期权限。任务归项目主人所有。", "Approve this change only; ongoing coordinator access stays unchanged. The task belongs to the project owner.")}</div>}
            {a.kind === "project_automation" && !!a.payload?.input && <div className="sa-approval-why">{automationRequestSummary(a.payload.input as Record<string, unknown>)}</div>}
            {a.kind === "project_automation" && automationRequestContent(a) && <details className="sa-approval-why"><summary>{L("查看任务内容", "Review task content")}</summary><pre className="sa-approval-content">{automationRequestContent(a)}</pre></details>}
            {a.kind === "project_invite" && <div className="sa-approval-why">{L("仅这一次邀请：链接 24 小时有效，只能用一次。新 agent 归项目主人所有，只加入为普通成员。", "This invitation only: the link lasts 24 hours and works once. The new agent belongs to the project owner and joins as an ordinary member.")}</div>}
            {a.kind === "action_grant" && (s?.grants?.some(g => g.approval_id === a.id && (g.requested || []).includes("browser.control")) || /这个 profile 登录的所有网站/.test(a.what)) && <div className="sa-approval-why">{L("agent 能看到并操作这个 profile 登录的所有网站，操作会显示在你的屏幕上，你随时可以停止。", "The agent can see and interact with all websites signed in through this profile. Its actions appear on your screen. You can stop it at any time.")}</div>}
            {a.reason && <div className="sa-approval-why">{a.reason}</div>}
            <div className="sa-approval-meta">{leftOf(a.expires_at, now)}</div>
            {!!errors[a.id] && <ErrorText error={errors[a.id]} />}
          </div>
          <div className="sa-approval-actions">
            <button type="button" className="app-btn app-btn-small" disabled={busy.has(a.id)} onClick={() => void decide(a, false)}>{L("不同意", "Deny")}</button>
            {a.kind === "action_grant" && wantsControl(a, s?.grants) && (
              <button type="button" className="app-btn app-btn-small" disabled={busy.has(a.id)} onClick={() => void decide(a, true, true)}
                title={L("只让它看屏幕，不能用键盘和鼠标", "Let it see the screen only, without keyboard or mouse")}>{L("只让它看", "View only")}</button>
            )}
            <button type="button" className="app-btn app-btn-small app-btn-primary" disabled={busy.has(a.id)} onClick={() => void decide(a, true)}>
              {busy.has(a.id) ? <span className="app-spinner" /> : null}{L("同意", "Approve")}
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}
