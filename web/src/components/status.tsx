import { Bot, Cloud, Code2, Sparkles, Terminal, Boxes, SquareTerminal, Server } from "lucide-react";
import type { State } from "@/api";
import { cn } from "@/lib/utils";

export const stateMeta: Record<State, { label: string; dot: string; text: string }> = {
  working: { label: "工作中", dot: "bg-working", text: "text-working" },
  pending: { label: "有待办", dot: "bg-pending", text: "text-pending" },
  idle: { label: "空闲", dot: "bg-idle", text: "text-idle" },
  offline: { label: "离线", dot: "bg-offline", text: "text-offline" },
};

export function StatusDot({ state, className }: { state: State; className?: string }) {
  const m = stateMeta[state];
  return (
    <span className={cn("relative inline-flex h-2 w-2 shrink-0", className)}>
      {state === "working" && <span className={cn("absolute inline-flex h-full w-full rounded-full animate-pulse-ring", m.dot)} />}
      <span className={cn("relative inline-flex h-2 w-2 rounded-full", m.dot)} />
    </span>
  );
}

export function StateBadge({ state }: { state: State }) {
  const m = stateMeta[state];
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border bg-card px-2 py-0.5 text-[11px] font-medium text-foreground/80">
      <StatusDot state={state} />
      {m.label}
    </span>
  );
}

const harnessMeta: Record<string, { label: string; icon: typeof Bot }> = {
  claude: { label: "Claude Code", icon: Sparkles },
  "claude-cloud": { label: "Claude 云端", icon: Cloud },
  codex: { label: "Codex", icon: Code2 },
  "codex-cloud": { label: "Codex 云端", icon: Cloud },
  shell: { label: "Shell", icon: Terminal },
  exec: { label: "Exec", icon: SquareTerminal },
  machine: { label: "机器节点", icon: Server },
};

export function harnessInfo(harness?: string) {
  return (harness && harnessMeta[harness]) || { label: harness || "未绑定", icon: harness ? Boxes : Bot };
}

/** Square runtime icon used as the agent's avatar. */
export function HarnessIcon({ harness, className }: { harness?: string; className?: string }) {
  const Icon = harnessInfo(harness).icon;
  return (
    <span className={cn("flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border bg-gradient-to-b from-card to-muted text-foreground/80", className)}>
      <Icon className="h-4 w-4" strokeWidth={1.75} />
    </span>
  );
}

export function HarnessBadge({ harness }: { harness?: string }) {
  const m = harnessInfo(harness);
  const Icon = m.icon;
  return (
    <span className="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium text-foreground/80">
      <Icon className="h-3 w-3" />
      {m.label}
    </span>
  );
}
