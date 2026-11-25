import { runnerUpdating } from "@/lib/runner-state";
import {
  IconAsterisk, IconBot, IconCloud, IconCode, IconLaptop, IconMachine, IconPerson, IconPointer, IconPrompt,
  IconServer, IconStar, IconTerminal, type IconType,
} from "@/components/icons";
import type { Agent, AgentStatus, Machine, Message, ModelSource, RuntimeAccess } from "@/api";
import type { AgentRow, NodeKind } from "@/app/types";
import type { HubOverview } from "@/appapi";
import { L } from "@/lib/i18n";
import { currentSlots } from "@/app/RunnerQueue";
import { isSuperagent, SHROOM } from "@/app/superagentMark";

/** `machine` nodes (`xxx-machine`) are not nodes in the UI. */
export type Kind = NodeKind | "machine";

const CLI_META: Record<string, { label: string; icon: IconType }> = {
  claude: { label: "Claude Code", icon: IconAsterisk },
  "claude-cloud": { label: "Claude Code", icon: IconAsterisk },
  codex: { label: "Codex", icon: IconCode },
  "codex-cloud": { label: "Codex", icon: IconCode },
  pi: { label: "Pi", icon: IconBot },
  kiro: { label: "Kiro", icon: IconBot },
  cursor: { label: "Cursor", icon: IconPointer },
  gemini: { label: "Gemini CLI", icon: IconStar },
  opencode: { label: "OpenCode", icon: IconTerminal },
  qoder: { label: "Qoder CLI", icon: IconBot },
  qwen: { label: "Qwen Code", icon: IconBot },
  kimi: { label: "Kimi Code", icon: IconBot },
  goose: { label: "goose", icon: IconBot },
  copilot: { label: "GitHub Copilot", icon: IconBot },
  auggie: { label: "Auggie", icon: IconBot },
  codebuddy: { label: "CodeBuddy Code", icon: IconBot },
  kilo: { label: "Kilo Code", icon: IconBot },
  cline: { label: "Cline", icon: IconBot },
  grok: { label: "Grok", icon: IconBot },
  antigravity: { label: "Antigravity", icon: IconBot },

  shell: { label: "Shell", icon: IconPrompt },
  exec: { label: "Exec", icon: IconTerminal },
  // Any agent joined from a session without a runner it can wake: it receives
  // only through `myco inbox wait` running in that session.
  inbox: { get label() { return L("会话内接收", "In session"); }, icon: IconBot },
  human: { get label() { return L("人", "Human"); }, icon: IconPerson },
};

export function agentKind(a: Agent): Kind {
  if (a.kind) return a.kind;
  const h = a.binding.harness || "";
  if (h === "machine") return "machine";
  if (h === "shell" || h === "exec") return "shell";
  if (h === "human") return "human";
  return "agent";
}

/** A cloud session (claude.ai, Codex cloud) has no machine and cannot spawn. */
export function isCloud(a: Agent): boolean {
  const cli = a.runtime?.cli || a.binding.harness || "";
  return !a.machine_id && cli.endsWith("-cloud");
}

/** A node that receives only while its own session runs `myco inbox wait`:
 *  no runner can wake it after the session closes. */
export function isInbox(a: Agent): boolean {
  return (a.runtime?.cli || a.binding.harness || "") === "inbox";
}

export function cliName(a: Agent): string {
  const cli = a.runtime?.cli || a.binding.harness;
  return cli ? CLI_META[cli]?.label || cli : L("未上报", "Not reported");
}

/** A CLI's name for people ("codex" → Codex); "" for none. */
export function cliLabel(cli?: string): string {
  return cli ? CLI_META[cli]?.label || cli : "";
}

export function cliVersion(a: Agent): string | undefined {
  return a.runtime?.cli_version;
}

export function cliIcon(a: Agent) {
  const cli = a.runtime?.cli || a.binding.harness || "";
  return CLI_META[cli]?.icon || IconBot;
}

/** Icon for a node row / inspector head, by kind. */
export function nodeIcon(a: Agent) {
  const kind = agentKind(a);
  if (kind === "human") return IconPerson;
  if (kind === "machine") return IconServer;
  if (kind === "shell") return cliIcon(a) === IconTerminal ? IconTerminal : IconPrompt;
  return cliIcon(a);
}

/** How a cloud session is shown where a machine name would go. */
export function cloudLabel(a: Agent): string {
  const cli = a.runtime?.cli || a.binding.harness || "";
  if (cli.startsWith("claude")) return L("云端（claude.ai）", "Cloud (claude.ai)");
  if (cli.startsWith("codex")) return L("云端（Codex）", "Cloud (Codex)");
  return L("云端", "Cloud");
}

export function machinesOfHub(hub: AgentRow["hub"]): Machine[] {
  return hub.overview?.machines || [];
}

export function machineOfRow(row: AgentRow): Machine | undefined {
  const id = row.st.agent.machine_id;
  if (!id) return undefined;
  return machinesOfHub(row.hub).find((m) => m.id === id);
}

/**
 * The machine column of a node: a real machine name, a cloud label, or
 * nothing for a human (who has no machine).
 */
export function machineLabel(row: AgentRow): string | undefined {
  const a = row.st.agent;
  if (isCloud(a)) return cloudLabel(a);
  const m = machineOfRow(row);
  if (m) return m.name;
  return a.machine_id || undefined;
}

export function kindLabel(kind: Kind): string {
  if (kind === "agent") return "AI";
  if (kind === "shell") return "Shell";
  if (kind === "human") return L("人", "Human");
  return L("机器节点", "Machine node");
}

export function modelSourceLabel(source?: ModelSource): string | undefined {
  if (source === "app") return L("App 设置", "App setting");
  if (source === "self") return L("它自己报的", "Reported by itself");
  if (source === "configured") return L("配置", "Configured");
  if (source === "observed") return L("实际使用", "In use");
  if (source === "default") return L("CLI 默认", "CLI default");
  if (source === "cli-config") return L("CLI 设置", "CLI setting");
  if (source === "provider") return L("服务商设置", "Provider setting");
  return undefined;
}

export function accessLabel(access?: RuntimeAccess): string {
  if (access === "readonly") return L("只读", "Read-only");
  if (access === "workspace") return L("工作区", "Workspace");
  if (access === "full") return L("完全", "Full");
  return "—";
}

export function osInfo(os: Machine["os"]): { label: string; icon: IconType } {
  if (os === "darwin") return { label: "macOS", icon: IconLaptop };
  if (os === "linux") return { label: "Linux", icon: IconServer };
  if (os === "windows") return { label: "Windows", icon: IconMachine };
  if (os === "cloud") return { label: L("云端", "Cloud"), icon: IconCloud };
  // Machines whose runners predate machine reporting: grouped by host name.
  return { label: L("系统未知", "Unknown OS"), icon: IconMachine };
}

/** A version for display: "v0.16.9" whether or not the source has the v. */
export function ver(v?: string): string {
  return v ? `v${v.replace(/^v/, "")}` : "";
}

export function machineSubtitle(m: Machine): string {
  return [
    osInfo(m.os).label,
    m.arch,
    ver(m.myco_version),
    L(`${m.agents.length} 个节点`, `${m.agents.length} node${m.agents.length === 1 ? "" : "s"}`),
    m.legacy ? L("升级 myco 后识别", "Upgrade myco to identify") : "",
  ].filter(Boolean).join(" · ");
}

/** Filter value for cloud sessions, alongside "all" and real machine ids. */
export const CLOUD_FILTER = "cloud";

export function machineFilterValue(hubId: string, machineId: string): string {
  return `m:${hubId}:${machineId}`;
}

export function parseMachineFilter(value: string): { hubId: string; machineId: string } | null {
  if (!value.startsWith("m:")) return null;
  const body = value.slice(2);
  const i = body.indexOf(":");
  if (i <= 0) return null;
  return { hubId: body.slice(0, i), machineId: body.slice(i + 1) };
}

/** A node's groups: its `group=…` labels. */
export function groupsOf(a: Agent): string[] {
  return (a.labels || []).filter((l) => l.startsWith("group=")).map((l) => l.slice(6));
}

/** A node's tags: its bare labels (key=value labels such as machine=… are not tags). */
export function tagsOf(a: Agent): string[] {
  return (a.labels || []).filter((l) => !l.includes("="));
}

/** The label filter: "all", "group:<name>", "group:" (no group) or "tag:<name>". */
export function matchesLabelFilter(a: Agent, value: string): boolean {
  if (value === "all") return true;
  if (value === "group:") return groupsOf(a).length === 0;
  if (value.startsWith("group:")) return groupsOf(a).includes(value.slice(6));
  if (value.startsWith("tag:")) return tagsOf(a).includes(value.slice(4));
  return true;
}

/** Turns what someone typed into a label value: no spaces, lower-case Latin letters. */
export function cleanLabel(s: string): string {
  return s.trim().replace(/\s+/g, "-").toLowerCase();
}

const CONSOLE = "@console:";

/** Whether a sender/recipient is this app's person on the hub (any person,
 *  on hubs without members). */
export function isMe(name: string, hub?: HubOverview): boolean {
  if (!name.startsWith(CONSOLE)) return false;
  const me = hub?.me?.member;
  if (!me) return true;
  const who = name.slice(CONSOLE.length);
  // Before members, tasks were signed with the device; they were the owner's.
  return who === me.name || (me.role === "owner" && !(hub?.me && knownPeople(hub).has(who)));
}

function knownPeople(hub: HubOverview): Set<string> {
  const s = new Set<string>();
  for (const a of hub.overview?.agents || []) if (a.agent.owner) s.add(a.agent.owner);
  if (hub.me?.member) s.add(hub.me.member.name);
  return s;
}

/** "你", a person's name (for messages from the app), or the node name. */
export function personLabel(name: string, hub?: HubOverview): string {
  if (isMe(name, hub)) return L("你", "You");
  if (name.startsWith(CONSOLE)) return name.slice(CONSOLE.length);
  return name;
}

/** Whether this app may manage a node or machine owned by ownerId. */
export function canManage(hub: HubOverview | undefined, ownerId?: string): boolean {
  const me = hub?.me;
  if (!me) return true; // hubs without members: every app is the admin
  return me.can_manage || (!!me.member && !!ownerId && ownerId === me.member.id);
}

/** The runner found the node's session open elsewhere (a terminal, an editor,
 *  a desktop app) and leaves it alone: its messages wait. */
export function sessionBusy(st: { state: string; session_in_use?: boolean; receiving?: string }): boolean {
  return st.state !== "offline" && !!st.session_in_use && st.receiving !== "session";
}

/** The state shown for a node: a busy session reads as such, not "Working". */
export function stateLabel(st: { agent?: Agent; state: string; session_in_use?: boolean; receiving?: string }, label: string): { dot: string; label: string } {
  if (st.agent && runnerUpdating(st.agent)) return { dot: "pending", label: L("正在更新", "Updating") };
  return sessionBusy(st) ? { dot: "pending", label: L("会话被占用", "Session in use") } : { dot: st.state, label };
}

/** An accepted task quiet for this long may have lost its result: a runner
 *  touches the message every few seconds while the run goes on. */
export const STUCK_AFTER_MS = 20 * 60_000;
/** A session-only node works while its own session runs and may report less
 *  often; give it longer before saying the task looks stuck. */
export const STUCK_AFTER_SESSION_MS = 30 * 60_000;

/** When a task last had activity (unix ms): the hub's active_at, or else when
 *  the message itself last changed. */
export function lastActivityAt(m: { active_at?: string; updated_at: string }): number {
  return new Date(m.active_at || m.updated_at).getTime();
}

/** How long a task has been quiet, said the way people say it: minutes,
 *  then hours, then days. Not a running timer: it does not count seconds. */
export function quietFor(now: number, at: number): string {
  const mins = Math.max(1, Math.floor((now - at) / 60_000));
  if (mins < 60) return L(`${mins} 分钟`, `${mins} min`);
  const hours = Math.floor(mins / 60);
  if (hours < 48) return L(`${hours} 小时`, `${hours} h`);
  return L(`${Math.floor(hours / 24)} 天`, `${Math.floor(hours / 24)} d`);
}

/** Whether a node handles messages only while one of its sessions runs: an
 *  inbox node, a cloud session, or a node reporting receiving "session". */
export function receivesInSession(a?: Agent, receiving?: string): boolean {
  return receiving === "session" || (!!a && (isInbox(a) || isCloud(a)));
}

/**
 * An accepted task whose result may never come back. It was picked up (acked)
 * and nothing has happened to it for longer than its node can
 * reasonably take: ten minutes when a runner wakes the node, thirty when only
 * a session of its own receives. A node that is offline is a signal too: it
 * cannot finish the task while it is away.
 */
export function possiblyStuck(
  m: { id?: string; status: string; active_at?: string; updated_at: string; asking?: string },
  node: { agent?: Agent; receiving?: string; offline?: boolean; seenAt?: number } | undefined,
  now: number,
): boolean {
  if (m.status !== "acked") return false;
  if (node?.agent && runnerUpdating(node.agent, now)) return false;
  if (node?.offline) return true;
  if (m.id && currentSlots(node?.agent, now)?.queue?.some((q) => q.message_id === m.id)) return false;
  const wait = receivesInSession(node?.agent, node?.receiving) ? STUCK_AFTER_SESSION_MS : STUCK_AFTER_MS;
  const at = lastActivityAt(m);
  return Number.isFinite(at) && seenNow(now, node?.seenAt) - at >= wait;
}

export function heldTaskLabel(m: Pick<Message, "asking" | "held_for">, sender: string): string {
  if (m.asking) return L(`暂挂：在等 ${sender} 回答`, `On hold: waiting for ${sender} to answer`);
  if (m.held_for === "reply") return L("暂挂：在等其他 agent 回复", "On hold: waiting for another agent to reply");
  return L("暂挂中，没人在做", "On hold · no one is working on it");
}

/** How far what is shown reaches: now, or, when the last answer (seenAt)
 *  is older than a few polls, then. Quiet is only known up to the last
 *  answer: on a poor network the news may not have come through, and the
 *  work may be going on all the same. */
export const SEEN_SLACK_MS = 20_000;
export function seenNow(now: number, seenAt?: number): number {
  return seenAt && now - seenAt > SEEN_SLACK_MS ? seenAt : now;
}

/** Possibly-stuck tasks among a node's current work, split as its counts are.
 *  seenAt: when they were fetched (the workspace's seen_at). */
export function stuckCounts(st: AgentStatus, now: number, seenAt?: number): { in_progress: number; total: number } {
  let in_progress = 0;
  for (const m of st.current || []) {
    if (!possiblyStuck(m, { agent: st.agent, receiving: st.receiving, offline: st.state === "offline", seenAt }, now)) continue;
    in_progress += 1;
  }
  return { in_progress, total: in_progress };
}

/**
 * What a node is called in the App: its display name, or else one made from
 * what it is (CLI · machine · folder, e.g. "Codex · Studio Mac · myco"), so
 * a node joined with a generated name still reads clearly. The node's own
 * name is what agents and the CLI use; it stays visible in its details.
 */
export function nodeName(a: Agent, hub?: HubOverview): string {
  // Your superagent is Shroom, whatever it called itself.
  if (isSuperagent(a)) return SHROOM;
  if (a.display_name) return a.display_name;
  const kind = agentKind(a);
  if (kind !== "agent" && kind !== "shell") return a.name;
  const machine = hub && a.machine_id ? machinesOfHub(hub).find((m) => m.id === a.machine_id) : undefined;
  // A name someone chose ("demo-shell", "docs-claude") stays; one made up
  // when the node joined (host name, CLI, a random suffix) is replaced by
  // what it is in plain words: an agent by the folder it works in ("myco
  // 助手"), a computer by which one and the folder ("Studio Mac · myco").
  if (!generatedName(a.name, machine?.hostname)) return a.name;
  const base = friendlyName(a, machine?.name);
  // Several of the same: the second is 二号, the third 三号 (in the order
  // they joined), so each still has a name of its own.
  const same = (hub?.overview?.agents || []).map((x) => x.agent)
    .filter((x) => !x.display_name && agentKind(x) === kind && friendlyName(x, machinesOfHub(hub!).find((m) => m.id === x.machine_id)?.name) === base)
    .sort((x, y) => (x.created_at || "").localeCompare(y.created_at || "") || x.name.localeCompare(y.name));
  const i = same.findIndex((x) => x.name === a.name);
  return i > 0 ? `${base} ${ordinal(i + 1)}` : base;
}

/** "myco 助手" / "Studio Mac · myco": the folder, and what it is. */
function friendlyName(a: Agent, machine?: string): string {
  const dir = a.binding?.workdir || "";
  const folder = dir.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || "";
  const where = folder && folder !== "~" ? folder : "";
  if (agentKind(a) === "shell") return [machine || L("电脑", "Computer"), where].filter(Boolean).join(" · ");
  return where ? L(`${where} 助手`, `${where} assistant`) : L("助手", "Assistant");
}

function ordinal(n: number): string {
  const zh = ["", "一", "二", "三", "四", "五", "六", "七", "八", "九", "十"];
  return L(n <= 10 ? `${zh[n]}号` : `${n}号`, `#${n}`);
}

/** A node's aliases as one short line for a row subtitle: "@a, @b +1".
 *  Two at most, so the rest of the subtitle stays readable. */
export function aliasSummary(a: Agent): string | undefined {
  const aliases = (a.aliases || []).filter(Boolean);
  if (!aliases.length) return undefined;
  const shown = aliases.slice(0, 2).map((x) => "@" + x).join(", ");
  return aliases.length > 2 ? `${shown} +${aliases.length - 2}` : shown;
}

/** A node name made up by `myco join` or a spawn: it starts with the
 *  machine's host name, or ends in the CLI and a short hex id. */
function generatedName(name: string, hostname?: string): boolean {
  const slug = (hostname || "").toLowerCase().replace(/\.local$/, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  if (slug.length >= 4 && name.toLowerCase().startsWith(slug)) return true;
  // `myco join` names an inbox node `<machine>-agent-<id>` (or
  // `<machine>-inbox-<id>`); both are generated, not chosen.
  return /-(claude|codex|gemini|cursor|opencode|pi|kiro|exec|shell|agent|inbox)-[0-9a-f]{3,}$/i.test(name);
}

/**
 * A node a runner serves, online, but nothing takes its messages now (the
 * runner stopped or is restarting). Only said when the hub reports how its
 * nodes receive (it then does for every node that does).
 */
export function runnerAway(st: AgentStatus, hub: HubOverview): boolean {
  // An inbox node has no runner to take its messages: it is online exactly
  // while its session runs `myco inbox wait`.
  if (isInbox(st.agent) || st.state === "offline" || st.receiving || sessionBusy(st)) return false;
  // A runner works one task at a time and does not pull while it does: no
  // pulls then means busy, not away.
  if (st.in_progress > 0) return false;
  const mode = st.agent.delivery?.mode || "pull";
  const k = agentKind(st.agent);
  if (mode !== "pull" || (k !== "agent" && k !== "shell") || /cloud/.test(st.agent.binding?.harness || "")) return false;
  return (hub.overview?.agents || []).some((x) => !!x.receiving);
}

/**
 * Whether a computer lets agents open a terminal there (a shell node): its
 * own machine.yaml decides (spawn.max_access: full), and its agents report
 * that limit as runtime.max_access. "unknown": nothing on it has said (its
 * myco is older, or no agent of it reported yet).
 */
export function machineShell(hub: AgentRow["hub"], machineId: string): "yes" | "no" | "unknown" {
  let said: string | undefined;
  for (const s of hub.overview?.agents || []) {
    if (s.agent.machine_id !== machineId) continue;
    const max = s.agent.runtime?.max_access;
    if (max === "full") return "yes";
    if (max) said = max;
  }
  return said ? "no" : "unknown";
}

/** The command that lets a computer run terminals, run there. */
export const ALLOW_SHELL_COMMAND = "myco machine shell-access --allow";
/** And the one that stops new ones. */
export const DENY_SHELL_COMMAND = "myco machine shell-access --deny";

/**
 * Which of a workspace's computers is the one this App runs on. Its
 * machine node there names it exactly; host names do not (WSL and its
 * Windows share one, and a workspace renames a clashing name). Without
 * that, by the name it joined with, then by host name.
 */
export function thisComputerId(hub: AgentRow["hub"], local: { machines?: string[]; machine_nodes?: string[]; hostname?: string } | null): string | undefined {
  if (!local) return undefined;
  const nodes = local.machine_nodes || [];
  if (nodes.length) {
    const node = (hub.overview?.agents || []).find((s) => nodes.includes(s.agent.name) && !!s.agent.machine_id);
    if (node) return node.agent.machine_id;
  }
  const ms = machinesOfHub(hub);
  return (ms.find((m) => (local.machines || []).includes(m.name))
    || ms.find((m) => !!local.hostname && m.hostname === local.hostname))?.id;
}

/** The name a session joins with: its title (its own, or its first
 *  request), on one line and short; undefined when it has none. */
export function sessionDisplayName(title?: string): string | undefined {
  const t = (title || "").replace(/\s+/g, " ").trim().replace(/…$/, "");
  if (!t) return undefined;
  const chars = Array.from(t);
  return chars.length > 24 ? chars.slice(0, 24).join("").trim() + "…" : t;
}
