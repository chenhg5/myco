import { useEffect, useMemo, useRef, useState } from "react";
import { Check } from "lucide-react";
import type { Handover, HandoverLink, HandoverResult, HandoverSide } from "@/api";
import { appApi, isOldHub } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { ErrorText, Modal } from "@/app/dialogs";
import { CopyButton } from "@/app/copy";
import { Elapsed, Mushroom } from "@/app/Working";
import { shortDir, usedFolders } from "@/app/AddToProject";
import { agentKind, canManage, cliLabel, isCloud, isInbox, isMe, machinesOfHub, nodeName, osInfo } from "@/app/nodes";
import { hubHasHandover } from "@/app/hubVersion";
import { L, locale, E } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/*
 * 「换一个来接着做」: another session takes a node over. The name, its
 * conversations, projects and aliases stay; the session behind it changes,
 * and the old one stops (it can be switched back to). Two ways: a new
 * session on one of your computers, or a one-time command run in any
 * session (another claude.ai account). Only changing the account of a
 * session on a computer needs neither: its run settings change the
 * provider and keep the context.
 *
 * Opened anywhere with
 *   window.dispatchEvent(new CustomEvent("myco-handover", { detail: { hubId, name, back? } }))
 * (AppMode shows the dialog). A handover done fires "myco-handovers-changed".
 */

export type HandoverTarget = { hubId: string; name: string; back?: Handover };

export function openHandover(t: HandoverTarget) {
  window.dispatchEvent(new CustomEvent("myco-handover", { detail: t }));
}

/** Whether the node can be handed over from here, and if not shown at all
 *  (hidden) or shown greyed out (why). */
export function handoverEntry(row: AgentRow | undefined): { show: boolean; why?: string } {
  if (!row) return { show: false };
  const a = row.st.agent;
  if (agentKind(a) !== "agent") return { show: false };
  if (!hubHasHandover(row.hub.hub_version)) return { show: false };
  if (!canManage(row.hub, a.owner_id)) return { show: false };
  if (row.hub.e2e) return { show: true, why: L("还不支持端到端加密的工作区", "Not yet in end-to-end encrypted workspaces") };
  return { show: true };
}

/** The menu item for a node's right-click menu (none where it does not apply). */
export function handoverItem(row: AgentRow | undefined): { label: string; onSelect: () => void; disabled?: boolean }[] {
  const e = handoverEntry(row);
  if (!row || !e.show) return [];
  return [{ label: L("换一个来接着做", "Hand over to another session"), disabled: !!e.why, onSelect: () => openHandover({ hubId: row.hub.id, name: row.st.agent.name }) }];
}

// Handovers by agent, as last read (a conversation and the profile share them).
const cache = new Map<string, { list: Handover[]; at: number }>();
// Non-secret IDs only: closing the command dialog must not lose its receipt.
const commandHandovers = new Map<string, string>();
const FRESH_MS = 30_000;
const keyOf = (hubId: string, name: string) => `${hubId}\u0000${name}`;
const commandKeyOf = (hubId: string, name: string, back?: string) => `${keyOf(hubId, name)}\u0000${back || ""}`;

/** An agent's handovers (newest first); [] on hubs without them. */
export function useHandovers(hubId: string | undefined, name: string | undefined, enabled = true): Handover[] {
  const key = hubId && name ? keyOf(hubId, name) : "";
  const [list, setList] = useState<Handover[]>(() => (key && cache.get(key)?.list) || []);
  useEffect(() => {
    if (!key || !enabled || !hubId || !name) { setList([]); return; }
    let live = true;
    setList(cache.get(key)?.list || []);
    const load = (force: boolean) => {
      const had = cache.get(key);
      if (!force && had && Date.now() - had.at < FRESH_MS) return;
      appApi.handovers(hubId, name)
        .then((r) => { cache.set(key, { list: r.handovers || [], at: Date.now() }); if (live) setList(r.handovers || []); })
        .catch((e) => { if (isOldHub(e) || (e as { status?: number }).status === 404) cache.set(key, { list: [], at: Date.now() }); });
    };
    load(false);
    const on = (e: Event) => {
      const d = (e as CustomEvent<{ hubId: string; name: string }>).detail;
      if (d && d.hubId === hubId && d.name === name) load(true);
    };
    window.addEventListener("myco-handovers-changed", on);
    return () => { live = false; window.removeEventListener("myco-handovers-changed", on); };
  }, [key, enabled, hubId, name]);
  return list;
}

function changed(hubId: string, name: string) {
  cache.delete(keyOf(hubId, name));
  window.dispatchEvent(new CustomEvent("myco-handovers-changed", { detail: { hubId, name } }));
}

/** One side of a handover for people: "mac-mini · Codex", "claude.ai 云端". */
export function sideLabel(side: HandoverSide | undefined, hub: AgentRow["hub"]): string {
  if (!side) return "";
  const cli = side.runtime?.cli || side.binding?.harness || "";
  const machine = side.machine_name || (side.machine_id ? machinesOfHub(hub).find((m) => m.id === side.machine_id)?.name : "");
  if (machine) return [machine, cliLabel(cli)].filter(Boolean).join(" · ");
  if (cli.endsWith("-cloud")) return cli.startsWith("codex") ? L("Codex 云端", "Codex cloud") : L("claude.ai 云端", "claude.ai cloud");
  if (cli === "inbox") return L("一个开着的会话", "an open session");
  return [cliLabel(cli), side.binding?.environment].filter(Boolean).join(" · ") || L("另一个会话", "another session");
}

/** Who started it, for people. */
function byLabel(by: string | undefined, hub: AgentRow["hub"]): string {
  if (!by) return "";
  if (by === "admin") return L("管理员", "an admin");
  if (hub.me?.member && hub.me.member.name === by) return L("你", "you");
  if (isMe(`@console:${by}`, hub)) return L("你", "you");
  return by;
}

/** As the chat's day lines read: "11月15日 04:26". */
const stamp = (iso: string) =>
  new Date(iso).toLocaleString(locale(), { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hour12: false });

/** The line a conversation shows where a handover took effect. */
export function HandoverDivider({ h, hub }: { h: Handover; hub: AgentRow["hub"] }) {
  const who = byLabel(h.by, hub);
  const to = sideLabel(h.to, hub);
  const what = h.back
    ? L(E`换回原来的会话 · ${to}`, E`Switched back to the earlier session · ${to}`)
    : L(E`换成 ${to} 接着做`, E`${to} carries on from here`);
  return (
    <div className="conv-handover" data-testid="handover-divider" title={h.note || undefined}>
      <span>{stamp(h.done_at || h.at)} · {what}{who ? L(E`（${who}换的）`, E` (by ${who})`) : ""}</span>
    </div>
  );
}

/** Whether a done handover's earlier session can be switched back to: not
 *  already undone, and not the session answering now. */
function canGoBack(h: Handover, all: Handover[], row: AgentRow): boolean {
  if (h.state !== "done") return false;
  if (all.some((x) => x.back === h.id && x.state === "done")) return false;
  const from = h.from?.binding?.session_id;
  return !!from && from !== row.st.agent.binding.session_id;
}

/** The profile's 接手记录: each handover, newest first, and a way back. */
export function HandoverHistory({ row, now }: { row: AgentRow; now: number }) {
  const entry = handoverEntry(row);
  const list = useHandovers(row.hub.id, row.st.agent.name, hubHasHandover(row.hub.hub_version) && agentKind(row.st.agent) === "agent");
  if (!list.length) return null;
  return (
    <section className="app-group" data-testid="handover-history">
      <div className="app-group-title"><span className="name">{L("接手记录", "Handovers")}</span></div>
      <div className="app-list">
        {list.map((h) => {
          const who = byLabel(h.by, row.hub);
          const state = h.state === "waiting"
            ? L("等它接手", "Waiting for it")
            : h.state === "expired" ? L("没用上", "Not used") : "";
          return (
            <div key={h.id} className="ho-row">
              <div className="ho-row-main">
                <div className="ho-row-top">
                  <span>{sideLabel(h.from, row.hub)}</span>
                  <span className="ho-arrow">→</span>
                  <span>{h.state === "done" ? sideLabel(h.to, row.hub) : L("一个新的会话", "a new session")}</span>
                  {state && <span className="app-pill">{state}</span>}
                </div>
                <div className="ho-row-sub">
                  <span title={new Date(h.done_at || h.at).toLocaleString()}>{timeAgo(h.done_at || h.at, now)}</span>
                  {who && <span>{L(E`${who}换的`, E`by ${who}`)}</span>}
                  {h.back && <span>{L("换回", "switch back")}</span>}
                  {h.note && <span className="ho-note" title={h.note}>{h.note}</span>}
                </div>
              </div>
              {entry.show && !entry.why && canGoBack(h, list, row) && (
                <button type="button" className="app-btn app-btn-small" onClick={() => openHandover({ hubId: row.hub.id, name: row.st.agent.name, back: h })}
                  title={L(E`换回 ${sideLabel(h.from, row.hub)} 那个会话，它自己的上下文还在`, E`Switch back to the session on ${sideLabel(h.from, row.hub)}; it still has its own context`)}>
                  {L("换回这个", "Switch back")}
                </button>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}

const CLIS = [
  { value: "claude", label: "Claude Code" },
  { value: "codex", label: "Codex" },
];

type Way = "machine" | "link";

/** The dialog: which way, what will happen, then it waits for the new session. */
export function HandoverDialog({ target, rows, onClose, onDone, onOpenProfile }: {
  target: HandoverTarget | null;
  rows: AgentRow[];
  onClose: () => void;
  onDone: () => void;
  /** opens the agent's profile (its run settings: the provider) */
  onOpenProfile: (row: AgentRow) => void;
}) {
  const row = target ? rows.find((r) => r.hub.id === target.hubId && r.st.agent.name === target.name) : undefined;
  const back = target?.back;
  const a = row?.st.agent;
  const hub = row?.hub;
  const machines = useMemo(() => (hub ? machinesOfHub(hub) : []).filter((m) => m.can_spawn), [hub]);
  const [way, setWay] = useState<Way>("machine");
  const [machineId, setMachineId] = useState("");
  const [cli, setCli] = useState("claude");
  const [dir, setDir] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [result, setResult] = useState<HandoverResult | null>(null);
  const [link, setLink] = useState<HandoverLink | null>(null);
  const [os, setOs] = useState<"sh" | "ps">("sh");
  const [arrived, setArrived] = useState<Handover | null>(null);
  const [commandId, setCommandId] = useState<string>();
  const onDoneRef = useRef(onDone);
  onDoneRef.current = onDone;

  // A fresh start each time it opens.
  useEffect(() => {
    if (!target || !a) return;
    const own = machines.find((m) => m.id === a.machine_id);
    const first = own || machines.find((m) => m.online) || machines[0];
    setWay(back ? (back.from.machine_id ? "machine" : "link") : machines.length ? "machine" : "link");
    setMachineId(first?.id || "");
    const h = (a.runtime?.cli || a.binding.harness || "").replace(/-cloud$/, "");
    setCli(h === "codex" ? "codex" : "claude");
    setDir(own ? a.binding.workdir || "" : "");
    setNote("");
    setBusy(null); setError(null); setResult(null); setLink(null); setArrived(null);
    setCommandId(commandHandovers.get(commandKeyOf(target.hubId, target.name, back?.id)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target?.hubId, target?.name, back?.id]);

  // Parent clocks recreate callbacks every second. Depend on the receipt,
  // not callback/target identity, so a 3-second poll can actually fire.
  const hubId = target?.hubId;
  const agentName = target?.name;
  useEffect(() => {
    if (!commandId || arrived || !hubId || !agentName) return;
    let off = false;
    let loading = false;
    const load = async () => {
      if (off || loading) return;
      loading = true;
      try {
        const r = await appApi.handovers(hubId, agentName);
        if (off) return;
        const h = (r.handovers || []).find((x) => x.id === commandId);
        if (h?.state === "done") {
          off = true;
          setArrived(h);
          changed(hubId, agentName);
          onDoneRef.current();
        }
      } catch {
        // A transient failure is not a completed handover. Retry the same
        // list route supported by older handover-capable hubs.
      } finally { loading = false; }
    };
    void load();
    const t = window.setInterval(() => void load(), 3000);
    return () => { off = true; window.clearInterval(t); };
  }, [commandId, arrived, hubId, agentName]);

  if (!target || !row || !a || !hub) return null;
  const name = nodeName(a, hub);
  const entry = handoverEntry(row);
  const machine = machines.find((m) => m.id === machineId);
  const machineOld = !!machine && !hubHasHandover(machine.myco_version);
  const open = (row.st.in_progress || 0) + (row.st.held || 0);
  const local = !!a.machine_id && !isCloud(a) && !isInbox(a) && row.st.receiving !== "session";
  const folders = machineId ? usedFolders(rows, hub.id, machineId) : [];
  const done = !!result || !!arrived;
  const close = () => { if (!busy) onClose(); };

  const start = async () => {
    if (busy) return;
    setError(null);
    setBusy(new Date().toISOString());
    try {
      if (way === "machine") {
        const r = await appApi.handOver(hub.id, a.name, back
          ? { machine: back.from.machine_id || machineId, back: back.id, ...(note.trim() ? { note: note.trim() } : {}) }
          : { machine: machineId, cli, ...(dir.trim() ? { dir: dir.trim() } : {}), ...(note.trim() ? { note: note.trim() } : {}) });
        setResult(r);
        changed(hub.id, a.name);
        onDone();
      } else {
        const made = await appApi.handoverLink(hub.id, a.name, { ...(note.trim() ? { note: note.trim() } : {}), ...(back ? { back: back.id } : {}) });
        commandHandovers.set(commandKeyOf(hub.id, a.name, back?.id), made.handover.id);
        setCommandId(made.handover.id);
        setLink(made);
        changed(hub.id, a.name);
      }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const ready = way === "link" || (!!machine && !machineOld && (!!back || !!dir.trim()));
  const command = link ? (os === "ps" ? link.command_ps1 || "" : link.command || "") : "";

  return (
    <Modal open onOpenChange={(o) => !o && close()} className="app-modal-add"
      title={back ? L("换回原来的会话", "Switch back to the earlier session") : L("换一个来接着做", "Hand it over to another session")}
      description={back
        ? L(E`让 ${sideLabel(back.from, hub)} 那个会话重新回答 ${name}。它自己的上下文还在，只补上它离开期间的对话。`, E`The session on ${sideLabel(back.from, hub)} answers for ${name} again. It still has its own context; it only catches up on what it missed.`)
        : L(E`换一个会话来接着做 ${name}：名字、聊天记录和项目都不变。`, E`Another session carries on as ${name}: its name, chats and projects stay.`)}>
      {entry.why ? (
        <div className="ho-body">
          <div className="app-small-note" data-testid="handover-e2e">{L("这个工作区开了端到端加密，Hub 读不了消息，还没法替新的会话写交接说明。这个功能暂时不支持端到端加密的工作区。", "This workspace is end-to-end encrypted: the hub cannot read the messages to write the new session's handover note. Handing over is not available in encrypted workspaces yet.")}</div>
          <div className="app-modal-actions"><button type="button" className="app-btn" onClick={onClose}>{L("关闭", "Close")}</button></div>
        </div>
      ) : done ? (
        <div className="ho-body" data-testid="handover-done">
          <div className="ws-command-done">
            <Check />
            <span>{L(E`已接手：现在由 ${sideLabel((result?.handover || arrived)?.to, hub) || L("新的会话", "the new session")} 回答 ${name}。原来的会话不会再收到消息。`,
              E`Taken over: ${sideLabel((result?.handover || arrived)?.to, hub) || "the new session"} answers for ${name} now. The earlier session gets no more messages.`)}</span>
          </div>
          {result?.spawn?.reply && (
            <div className="ho-reply-wrap">
              <div className="app-field-label">{L("它读完交接说明后说：", "After reading the handover note, it said:")}</div>
              <div className="ho-reply">{result.spawn.reply}</div>
            </div>
          )}
          <div className="app-modal-actions">
            {arrived && <button type="button" className="app-btn" onClick={() => {
              commandHandovers.delete(commandKeyOf(hub.id, a.name, back?.id));
              setCommandId(undefined); setArrived(null); setLink(null); setError(null);
            }}>{L("再换一个会话", "Hand over again")}</button>}
            <button type="button" className="app-btn app-btn-primary" onClick={onClose}>{L("完成", "Done")}</button>
          </div>
        </div>
      ) : (
        <div className="ho-body">
          {!back && local && !link && (
            <div className="ho-account">
              <span>{L("只是换个账号？在它的「运行」里换服务商就行，会话和上下文都不动。", "Only changing the account? Switch the provider in its run settings: the session and its context stay.")}</span>
              <button type="button" className="app-btn app-btn-quiet app-btn-small" onClick={() => { onClose(); onOpenProfile(row); }}>{L("去换服务商", "Change the provider")}</button>
            </div>
          )}
          {!back && !link && (
            <div className="seg ho-ways" role="tablist" aria-label={L("怎么接手", "How")}>
              <button type="button" role="tab" aria-selected={way === "machine"} className={cn("seg-btn", way === "machine" && "active")} onClick={() => setWay("machine")} disabled={!!busy || !!link}>{L("在一台电脑上新开", "Start one on a computer")}</button>
              <button type="button" role="tab" aria-selected={way === "link"} className={cn("seg-btn", way === "link" && "active")} onClick={() => setWay("link")} disabled={!!busy || !!link} data-testid="handover-way-link">{L("用一条命令接手", "Take over with a command")}</button>
            </div>
          )}

          {way === "machine" && !back && (machines.length === 0 ? (
            <div className="app-small-note">{L("还没有能新开会话的电脑。可以用一条命令接手，或者先在「电脑」里连上一台。", "No computer can start sessions yet. Take over with a command, or connect one under Computers first.")}</div>
          ) : (
            <>
              <div className="app-field">
                <span className="app-field-label">{L("在哪台电脑上", "On which computer")}</span>
                <div className="ws-computers">
                  {machines.map((m) => {
                    const Icon = osInfo(m.os).icon;
                    const old = !hubHasHandover(m.myco_version);
                    return (
                      <button key={m.id} type="button" className={cn("ws-computer", m.id === machineId && "active", (!m.online || old) && "is-away")}
                        onClick={() => { setMachineId(m.id); setDir(m.id === a.machine_id ? a.binding.workdir || "" : ""); }} disabled={!!busy}>
                        <Icon />
                        <span className="ws-computer-name">{m.name}</span>
                        {old ? <span className="ws-computer-sub">{L("要先升级", "Needs an update")}</span>
                          : !m.online ? <span className="ws-computer-sub">{L("不在线", "Away")}</span>
                            : m.id === a.machine_id ? <span className="ws-computer-sub">{L("原来那台", "Where it is now")}</span> : null}
                      </button>
                    );
                  })}
                </div>
                {machineOld && (
                  <div className="app-small-note" data-testid="handover-machine-old">
                    {L(E`${machine!.name} 上的 myco 太旧，还不会接手。在那台电脑上运行 myco upgrade 再来。`, E`The myco on ${machine!.name} is too old to take over. Run myco upgrade there first.`)}
                  </div>
                )}
              </div>
              <div className="app-field">
                <span className="app-field-label">{L("用哪个 AI", "Which AI")}</span>
                <div className="ws-computers">
                  {CLIS.map((c) => (
                    <button key={c.value} type="button" className={cn("ws-computer", cli === c.value && "active")} onClick={() => setCli(c.value)} disabled={!!busy}>
                      <span className="ws-computer-name">{c.label}</span>
                    </button>
                  ))}
                </div>
              </div>
              <label className="app-field">
                <span className="app-field-label">{L("在哪个文件夹干活", "Which folder it works in")}</span>
                <input className="app-input mono" value={dir} onChange={(e) => setDir(e.target.value)} placeholder={L("比如 ~/code/我的项目", "e.g. ~/code/my-project")} disabled={!!busy} />
              </label>
              {folders.length > 0 && (
                <div className="app-convpick-folders">
                  {folders.map((d) => <button key={d} type="button" className={cn("app-convpick-folder mono", d === dir.trim() && "active")} onClick={() => setDir(d)} disabled={!!busy} title={d}>{shortDir(d)}</button>)}
                </div>
              )}
            </>
          ))}

          {way === "link" && link ? (
            <div className="ws-command" data-testid="handover-command">
              <div className="app-field-label">{back
                ? L(E`在原来那个会话（${sideLabel(back.from, hub)}）里运行这一行：`, E`Run this in the earlier session (${sideLabel(back.from, hub)}):`)
                : L("在要接手的会话里运行这一行（比如另一个 claude.ai 账号的云端会话，或者任何一台电脑的终端）：", "Run this in the session that should take over (another claude.ai account's cloud session, or a terminal on any computer):")}</div>
              <div className="seg ws-command-os" role="tablist" aria-label={L("在哪种系统上运行", "Where it runs")}>
                <button type="button" role="tab" aria-selected={os === "sh"} className={cn("seg-btn", os === "sh" && "active")} onClick={() => setOs("sh")}>{L("Mac / Linux / 云端", "Mac / Linux / cloud")}</button>
                <button type="button" role="tab" aria-selected={os === "ps"} className={cn("seg-btn", os === "ps" && "active")} onClick={() => setOs("ps")}>{L("Windows（PowerShell）", "Windows (PowerShell)")}</button>
              </div>
              <div className="app-command"><code>{command}</code><CopyButton value={command} /></div>
              <div className="ws-command-wait"><span className="app-spinner" />{L("等它接手…运行完这里会自己打勾。", "Waiting for it… this ticks itself once it has run.")}</div>
              <div className="ws-who">{L("命令 24 小时内有效，只能用一次。它等同于这个 agent 的凭证，别发到别人能看到的地方。", "The command works once, for 24 hours. It is as good as this agent's credential: keep it where only you can see it.")}</div>
              <div className="app-modal-actions"><button type="button" className="app-btn" onClick={onClose}>{L("关闭", "Close")}</button></div>
            </div>
          ) : (
            <>
              {way === "link" && (
                <div className="app-small-note">{back
                  ? L("会生成一条一次性的命令，到原来那个会话里运行，它就接回来了。", "You get a one-time command; run it in the earlier session and it takes the node back.")
                  : L("会生成一条一次性的命令。在任何会话里运行它（缺 myco 会先装上），那个会话就接手了，适合换到另一个 claude.ai 账号。", "You get a one-time command. Run it in any session (it installs myco if needed) and that session takes over: the way to move to another claude.ai account.")}</div>
              )}
              <label className="app-field">
                <span className="app-field-label">{L("为什么换（可以不填，会写进交接说明）", "Why (optional, goes into the handover note)")}</span>
                <input className="app-input" value={note} onChange={(e) => setNote(e.target.value)} placeholder={L("比如 原来的订阅到期了", "e.g. the old subscription ran out")} disabled={!!busy} />
              </label>
              <ul className="ho-what" data-testid="handover-what">
                {back
                  ? <li>{L("现在回答它的会话会停下，不再收消息。", "The session answering it now stops and gets no more messages.")}</li>
                  : <li>{L("名字、聊天记录和项目都不变。新的会话先读一份交接说明：最近 30 条对话和还没做完的事。", "Its name, chats and projects stay. The new session first reads a handover note: the last 30 messages and what is not done yet.")}</li>}
                {!back && <li>{L("原来的会话会停下，不再收消息；之后在「接手记录」里可以换回来。", "The earlier session stops and gets no more messages; you can switch back under Handovers.")}</li>}
                {open > 0 && <li>{L(E`它手上没做完的 ${open} 件事，交给${back ? "换回去的" : "新的"}会话重新做。`, E`The ${open} task${open > 1 ? "s" : ""} it has not finished go to the ${back ? "earlier" : "new"} session to redo.`)}</li>}
                {row.st.state === "working" && <li>{L("它现在正在干活，那件事也会交过去重新做。", "It is at work right now; that task goes over too.")}</li>}
              </ul>
              <ErrorText error={error} />
              {busy && way === "machine" && (
                <div className="app-convpick-starting"><Mushroom /><span>{L("正在开新的会话，它在读交接说明", "Starting the new session; it is reading the handover note")} · <Elapsed since={busy} /></span></div>
              )}
              <div className="app-modal-actions">
                <button type="button" className="app-btn" onClick={close} disabled={!!busy}>{L("取消", "Cancel")}</button>
                <button type="button" className="app-btn app-btn-primary" onClick={() => void start()} disabled={!!busy || !ready} data-testid="handover-start">
                  {way === "link"
                    ? busy ? L("正在生成…", "Making it…") : L("生成命令", "Make the command")
                    : busy ? L("正在接手…", "Handing over…") : back ? L("换回去", "Switch back") : L("开始接手", "Start the handover")}
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </Modal>
  );
}
