import { Bot, Cloud, Code2, Sparkles, Terminal, Boxes } from "lucide-react";
import type { State } from "@/api";
import { cn } from "@/lib/utils";

export const stateMeta: Record<State, { label: string; dot: string; text: string; ring: string }> = {
  working: { label: "工作中", dot: "bg-working", text: "text-working", ring: "ring-working/30" },
  pending: { label: "有待办", dot: "bg-pending", text: "text-pending", ring: "ring-pending/30" },
  idle: { label: "空闲", dot: "bg-idle", text: "text-idle", ring: "ring-idle/30" },
  offline: { label: "离线", dot: "bg-offline", text: "text-offline", ring: "ring-offline/30" },
};

export function StatusDot({ state, className }: { state: State; className?: string }) {
  const m = stateMeta[state];
  return (
    <span className={cn("relative inline-flex h-2.5 w-2.5", className)}>
      {state === "working" && <span className={cn("absolute inline-flex h-full w-full rounded-full animate-pulse-ring", m.dot)} />}
      <span className={cn("relative inline-flex h-2.5 w-2.5 rounded-full", m.dot)} />
    </span>
  );
}

export function StateBadge({ state }: { state: State }) {
  const m = stateMeta[state];
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-full bg-card px-2 py-0.5 text-xs font-medium ring-1", m.ring, m.text)}>
      <StatusDot state={state} className="scale-90" />
      {m.label}
    </span>
  );
}

const harnessMeta: Record<string, { label: string; icon: typeof Bot; cls: string }> = {
  claude: { label: "Claude Code", icon: Sparkles, cls: "bg-[#d97757]/10 text-[#c2613f] dark:text-[#e8977a] ring-[#d97757]/25" },
  "claude-cloud": { label: "Claude 云端", icon: Cloud, cls: "bg-[#d97757]/10 text-[#c2613f] dark:text-[#e8977a] ring-[#d97757]/25" },
  codex: { label: "Codex", icon: Code2, cls: "bg-foreground/5 text-foreground ring-foreground/15" },
  shell: { label: "Shell", icon: Terminal, cls: "bg-secondary text-secondary-foreground ring-border" },
};

export function HarnessBadge({ harness }: { harness?: string }) {
  const m = (harness && harnessMeta[harness]) || { label: harness || "未绑定", icon: harness ? Boxes : Bot, cls: "bg-secondary text-muted-foreground ring-border" };
  const Icon = m.icon;
  return (
    <span className={cn("inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium ring-1", m.cls)}>
      <Icon className="h-3 w-3" />
      {m.label}
    </span>
  );
}
