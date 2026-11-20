import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { ShieldQuestion } from "lucide-react";
import type { Agent, Approval, SuperagentState } from "@/api";
import { appApi, type JoinLink } from "@/appapi";
import { CopyButton } from "@/app/copy";
import { joinSentence } from "@/app/AddToProject";
import type { AgentRow } from "@/app/types";
import { ErrorText, Modal } from "@/app/dialogs";
import { agentKind, machinesOfHub, nodeName } from "@/app/nodes";
import { L } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** The superagent's fixed name (docs/design/chief-of-staff.md). */
export const SHROOM = "Shroom";

// One shared copy per workspace: the bottom-left menu, the chat and the
// approval cards read the same state, and only one poll runs per workspace.
type Entry = { state?: SuperagentState; approvals: Approval[]; at: number; off?: boolean };
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
    store.set(hubId, { state, approvals, at: Date.now() });
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
          <div className="add-agent-way" data-testid="superagent-link">
            <div className="app-field-label">{L("把这句话发给一个 Claude Code、Codex 会话（本机、别的电脑或 claude.ai 云端都行），它加入后就是你的 superagent：", "Send this to a Claude Code or Codex session (here, on another computer or in the cloud); once it joins, it is your superagent:")}</div>
            {link ? (
              <div className="add-sentence">
                <p className="add-sentence-text">{sentence}</p>
                <div className="add-sentence-actions"><CopyButton value={sentence} /></div>
              </div>
            ) : linkError ? <ErrorText error={linkError} /> : (
              <div className="app-command"><span className="app-spinner" /><code>{L("正在生成…", "Making it…")}</code></div>
            )}
            {link && <div className="ws-command-wait"><span className="app-spinner" />{L("等它加入…加入后这里会自动打开和它的对话。这句话 24 小时内有效，只能用一次，等同凭据，只发给你自己的会话。", "Waiting for it to join… its chat opens here once it has. The sentence works once, for 24 hours; it is a credential, so send it only to your own session.")}</div>}
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
          <span style={{ flex: 1 }} />
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

/** What an agent waits on you for, above its chat's composer (your
 *  superagent's requests, a coordinator asking for a computer): what, why,
 *  and 同意 / 不同意. */
export function ApprovalStrip({ hubId, agent, name }: { hubId: string; agent: string; name?: string }) {
  const s = useSuperagent(hubId);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  const items = (s?.approvals || []).filter((a) => a.agent === agent && a.status === "pending");
  const who = name || (s?.state?.agent?.name === agent ? SHROOM : agent);
  if (!items.length) return null;
  const now = Date.now();
  const decide = async (a: Approval, ok: boolean) => {
    setBusy(a.id);
    setError(null);
    try {
      await appApi.decideApproval(hubId, a.id, ok);
      await refreshSuperagent(hubId);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };
  return (
    <div className="sa-approvals" data-testid="approvals">
      {items.map((a) => (
        <div className="sa-approval" key={a.id}>
          <ShieldQuestion className="sa-approval-icon" />
          <div className="sa-approval-text">
            <div className="sa-approval-what">{L(`${who} 想：`, `${who} wants to: `)}{a.what}</div>
            {a.reason && <div className="sa-approval-why">{a.reason}</div>}
            <div className="sa-approval-meta">{leftOf(a.expires_at, now)}</div>
          </div>
          <div className="sa-approval-actions">
            <button type="button" className="app-btn app-btn-small" disabled={!!busy} onClick={() => void decide(a, false)}>{L("不同意", "Deny")}</button>
            <button type="button" className="app-btn app-btn-small app-btn-primary" disabled={!!busy} onClick={() => void decide(a, true)}>
              {busy === a.id ? <span className="app-spinner" /> : null}{L("同意", "Approve")}
            </button>
          </div>
        </div>
      ))}
      <ErrorText error={error} />
    </div>
  );
}
