import {
  IconAsterisk, IconBot, IconCloud, IconCode, IconLaptop, IconMachine, IconPerson, IconPointer, IconPrompt,
  IconServer, IconStar, IconTerminal, type IconType,
} from "@/components/icons";
import type { Agent, Machine, ModelSource, RuntimeAccess } from "@/api";
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

/**
 * What a node is called in the App: its display name, or else one made from
 * what it is (CLI · machine · folder, e.g. "Codex · Studio Mac · myco"), so
 * a node joined with a generated name still reads clearly. The node's own
 * name is what agents and the CLI use; it stays visible in its details.
 */
export function nodeName(a: Agent, hub?: HubOverview): string {
  if (a.display_name) return a.display_name;
  const dir = a.binding?.workdir;
  if (!a.machine_id || !dir || (a.kind && a.kind !== "agent" && a.kind !== "shell")) return a.name;
  const machine = hub ? machinesOfHub(hub).find((m) => m.id === a.machine_id) : undefined;
  // A name someone chose ("mac-exec", "docs-claude") stays; one made up
  // when the node joined (host name, CLI, a random suffix) is replaced.
  if (!generatedName(a.name, machine?.hostname)) return a.name;
  const folder = dir.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || "";
  const parts = [a.runtime?.cli || a.binding.harness ? cliName(a) : "", machine?.name, folder === "~" ? "" : folder].filter(Boolean);
  return parts.length >= 2 ? parts.join(" · ") : a.name;
}

/** A node name made up by `myco join` or a spawn: it starts with the
 *  machine's host name, or ends in the CLI and a short hex id. */
function generatedName(name: string, hostname?: string): boolean {
  const slug = (hostname || "").toLowerCase().replace(/\.local$/, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  if (slug.length >= 4 && name.toLowerCase().startsWith(slug)) return true;
  return /-(claude|codex|gemini|cursor|opencode|pi|kiro|exec|shell)-[0-9a-f]{3,}$/i.test(name);
}
