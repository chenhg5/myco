import {
  IconAsterisk, IconBot, IconCloud, IconCode, IconLaptop, IconMachine, IconPerson, IconPointer, IconPrompt,
  IconServer, IconStar, IconTerminal, type IconType,
} from "@/components/icons";
import type { Agent, AgentStatus, Machine, ModelSource, RuntimeAccess } from "@/api";
import type { AgentRow, NodeKind } from "@/app/types";
import type { HubOverview } from "@/appapi";
import { L } from "@/lib/i18n";

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
export function stateLabel(st: { state: string; session_in_use?: boolean; receiving?: string }, label: string): { dot: string; label: string } {
  return sessionBusy(st) ? { dot: "pending", label: L("会话被占用", "Session in use") } : { dot: st.state, label };
}

/** An accepted task quiet for this long may have lost its result: a runner
 *  touches the message every few seconds while the run goes on. */
export const STUCK_AFTER_MS = 10 * 60_000;
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
 * or held, and nothing has happened to it for longer than its node can
 * reasonably take: ten minutes when a runner wakes the node, thirty when only
 * a session of its own receives. A node that is offline is a signal too: it
 * cannot finish the task while it is away.
 */
export function possiblyStuck(
  m: { status: string; active_at?: string; updated_at: string },
  node: { agent?: Agent; receiving?: string; offline?: boolean } | undefined,
  now: number,
): boolean {
  if (m.status !== "acked" && m.status !== "held") return false;
  if (node?.offline) return true;
  const wait = receivesInSession(node?.agent, node?.receiving) ? STUCK_AFTER_SESSION_MS : STUCK_AFTER_MS;
  const at = lastActivityAt(m);
  return Number.isFinite(at) && now - at >= wait;
}

/** Possibly-stuck tasks among a node's current work, split as its counts are. */
export function stuckCounts(st: AgentStatus, now: number): { in_progress: number; held: number; total: number } {
  let in_progress = 0;
  let held = 0;
  for (const m of st.current || []) {
    if (!possiblyStuck(m, { agent: st.agent, receiving: st.receiving, offline: st.state === "offline" }, now)) continue;
    if (m.status === "held") held += 1;
    else in_progress += 1;
  }
  return { in_progress, held, total: in_progress + held };
}

/**
 * What a node is called in the App: its display name, or else one made from
 * what it is (CLI · machine · folder, e.g. "Codex · Studio Mac · myco"), so
 * a node joined with a generated name still reads clearly. The node's own
 * name is what agents and the CLI use; it stays visible in its details.
 */
export function nodeName(a: Agent, hub?: HubOverview): string {
  if (a.display_name) return a.display_name;
  const kind = agentKind(a);
  if (kind !== "agent" && kind !== "shell") return a.name;
  const machine = hub && a.machine_id ? machinesOfHub(hub).find((m) => m.id === a.machine_id) : undefined;
  // A name someone chose ("mac-exec", "docs-claude") stays; one made up
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
  const mode = st.agent.delivery?.mode || "pull";
  const k = agentKind(st.agent);
  if (mode !== "pull" || (k !== "agent" && k !== "shell") || /cloud/.test(st.agent.binding?.harness || "")) return false;
  return (hub.overview?.agents || []).some((x) => !!x.receiving);
}
