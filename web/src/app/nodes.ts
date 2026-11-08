import {
  Bot, Cloud, Code2, Laptop, Monitor, Server, Sparkles, SquareTerminal, Terminal, UserRound,
} from "lucide-react";
import type { Agent, Machine, ModelSource, RuntimeAccess } from "@/api";
import type { AgentRow, NodeKind } from "@/app/types";
import { L } from "@/lib/i18n";

/** `machine` nodes (`xxx-machine`) are not nodes in the UI. */
export type Kind = NodeKind | "machine";

const CLI_META: Record<string, { label: string; icon: typeof Bot }> = {
  claude: { label: "Claude Code", icon: Sparkles },
  "claude-cloud": { label: "Claude Code", icon: Sparkles },
  codex: { label: "Codex", icon: Code2 },
  "codex-cloud": { label: "Codex", icon: Code2 },
  pi: { label: "Pi", icon: Bot },
  kiro: { label: "Kiro", icon: Bot },
  cursor: { label: "Cursor", icon: Code2 },
  gemini: { label: "Gemini CLI", icon: Sparkles },
  opencode: { label: "OpenCode", icon: SquareTerminal },
  shell: { label: "Shell", icon: Terminal },
  exec: { label: "Exec", icon: SquareTerminal },
  human: { get label() { return L("人", "Human"); }, icon: UserRound },
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
  return CLI_META[cli]?.icon || Bot;
}

/** Icon for a node row / inspector head, by kind. */
export function nodeIcon(a: Agent) {
  const kind = agentKind(a);
  if (kind === "human") return UserRound;
  if (kind === "machine") return Server;
  if (kind === "shell") return Terminal;
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
  return undefined;
}

export function accessLabel(access?: RuntimeAccess): string {
  if (access === "readonly") return L("只读", "Read-only");
  if (access === "workspace") return L("工作区", "Workspace");
  if (access === "full") return L("完全", "Full");
  return "—";
}

export function osInfo(os: Machine["os"]): { label: string; icon: typeof Monitor } {
  if (os === "darwin") return { label: "macOS", icon: Laptop };
  if (os === "linux") return { label: "Linux", icon: Terminal };
  if (os === "windows") return { label: "Windows", icon: Monitor };
  if (os === "cloud") return { label: L("云端", "Cloud"), icon: Cloud };
  // Machines whose runners predate machine reporting: grouped by host name.
  return { label: L("系统未知", "Unknown OS"), icon: Monitor };
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
