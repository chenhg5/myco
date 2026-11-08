import { Inbox, Loader2, PauseCircle, Server } from "lucide-react";
import type { AgentStatus } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Tip } from "@/components/ui/tooltip";
import { CopyText } from "@/components/copy";
import { HarnessBadge, HarnessIcon, StateBadge, harnessInfo } from "@/components/status";
import { cn, timeAgo } from "@/lib/utils";

function shortId(s?: string, n = 10) {
  if (!s) return "—";
  return s.length > n + 2 ? `${s.slice(0, n)}…` : s;
}

export function Counters({ a, compact }: { a: AgentStatus; compact?: boolean }) {
  const items = [
    { icon: Loader2, n: a.in_progress, label: "处理中", cls: "text-working", spin: a.in_progress > 0 },
    { icon: Inbox, n: a.queued, label: "待处理", cls: "text-pending" },
    { icon: PauseCircle, n: a.held, label: "已挂起", cls: "text-idle" },
  ];
  return (
    <div className={cn("flex items-center", compact ? "gap-3" : "gap-4")}>
      {items.map(({ icon: Icon, n, label, cls, spin }) => (
        <Tip key={label} content={label}>
          <span className={cn("inline-flex items-center gap-1 text-xs tabular-nums", n > 0 ? cls : "text-muted-foreground/40")}>
            <Icon className={cn("h-3.5 w-3.5", spin && "animate-spin")} />
            {n}
          </span>
        </Tip>
      ))}
    </div>
  );
}

export function Aliases({ aliases }: { aliases?: string[] }) {
  if (!aliases?.length) return null;
  return (
    <>
      {aliases.map((al) => (
        <Tip key={al} content="别名：发给它的消息会送到这个节点">
          <span className="inline-flex items-center rounded-md border border-foreground/20 bg-foreground/[0.03] px-1.5 py-0.5 font-mono text-[11px] text-foreground">@{al}</span>
        </Tip>
      ))}
    </>
  );
}

/** Display name prominently with the immutable address below (or just the address). */
export function AgentName({ name, displayName, className }: { name: string; displayName?: string; className?: string }) {
  if (!displayName) return <div className={cn("truncate font-mono font-semibold", className)}>{name}</div>;
  return (
    <div className="min-w-0">
      <div className={cn("truncate font-semibold", className)}>{displayName}</div>
      <div className="truncate font-mono text-xs text-muted-foreground">{name}</div>
    </div>
  );
}

/** The agent's public status line (myco agent status set). */
export function StatusLine({ text, at, now }: { text: string; at?: string; now: number }) {
  return (
    <div className="flex items-start gap-2 rounded-md border border-working/20 bg-working/5 px-2.5 py-1.5 text-xs">
      <span className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-working" />
      <span className="min-w-0 flex-1 break-words text-foreground/90">{text}</span>
      {at && <span className="shrink-0 text-muted-foreground">{timeAgo(at, now)}</span>}
    </div>
  );
}

export function Labels({ labels, onPick, max = 6 }: { labels: string[]; onPick?: (l: string) => void; max?: number }) {
  if (!labels.length) return <span className="text-xs text-muted-foreground/60">无标签</span>;
  const shown = labels.slice(0, max);
  return (
    <div className="contents">
      {shown.map((l) => (
        <Badge
          key={l}
          variant="label"
          className={cn(onPick && "cursor-pointer hover:border-foreground/30 hover:text-foreground")}
          onClick={(e) => {
            if (!onPick) return;
            e.stopPropagation();
            onPick(l);
          }}
        >
          {l}
        </Badge>
      ))}
      {labels.length > max && <Badge variant="secondary">+{labels.length - max}</Badge>}
    </div>
  );
}

export function AgentCard({ a, onOpen, onLabel, now }: { a: AgentStatus; onOpen: () => void; onLabel: (l: string) => void; now: number }) {
  const ag = a.agent;
  return (
    <Card
      onClick={onOpen}
      className={cn(
        "group relative flex cursor-pointer flex-col gap-4 p-5 transition-[border-color,box-shadow] animate-fade-in hover:border-foreground/20 hover:shadow-[0_4px_16px_-6px_rgb(0_0_0/0.08)]",
        a.state === "offline" && "opacity-60",
      )}
    >
      <div className="flex items-start gap-3">
        <HarnessIcon harness={ag.binding.harness} />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm leading-5"><AgentName name={ag.name} displayName={ag.display_name} /></h3>
        </div>
        <StateBadge state={a.state} />
      </div>

      <p className="line-clamp-2 min-h-[2.5rem] text-[13px] leading-5 text-muted-foreground">{ag.description || <span className="opacity-50">没有描述</span>}</p>

      {ag.status_text && <StatusLine text={ag.status_text} at={ag.status_at} now={now} />}

      {(ag.labels.length > 0 || !!ag.aliases?.length || ag.delivery.mode !== "pull") && (
        <div className="flex flex-wrap items-center gap-1">
          <Aliases aliases={ag.aliases} />
          {ag.delivery.mode !== "pull" && <Badge variant="label">{ag.delivery.mode}</Badge>}
          {ag.labels.length > 0 && <Labels labels={ag.labels} onPick={onLabel} max={4} />}
        </div>
      )}

      <div className="mt-auto flex items-center justify-between gap-3 border-t pt-3 text-xs text-muted-foreground">
        <div className="flex min-w-0 items-center gap-1.5">
          <span className="truncate text-foreground/70">{harnessInfo(ag.binding.harness).label}</span>
          <span className="text-border">/</span>
          <span className="inline-flex min-w-0 items-center gap-1"><Server className="h-3 w-3 shrink-0" /><span className="truncate">{ag.binding.environment || "—"}</span></span>
          <span className="text-border">/</span>
          <span className="shrink-0">{timeAgo(ag.last_seen_at, now)}</span>
        </div>
        <Counters a={a} compact />
      </div>
    </Card>
  );
}

export function AgentTable({ agents, onOpen, onLabel, now }: { agents: AgentStatus[]; onOpen: (a: AgentStatus) => void; onLabel: (l: string) => void; now: number }) {
  return (
    <Card className="overflow-hidden">
      <div className="overflow-x-auto scrollbar-thin">
        <table className="w-full text-sm">
          <thead className="border-b bg-muted/40 text-left text-xs text-muted-foreground">
            <tr>
              {["节点", "状态", "运行时", "环境 / 会话", "标签", "任务", "最近在线"].map((h) => (
                <th key={h} className="whitespace-nowrap px-4 py-2.5 font-medium">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y">
            {agents.map((a) => (
              <tr key={a.agent.id} onClick={() => onOpen(a)} className="cursor-pointer transition hover:bg-muted/40">
                <td className="max-w-[260px] px-4 py-3">
                  <AgentName name={a.agent.name} displayName={a.agent.display_name} className="font-medium" />
                  {!!a.agent.aliases?.length && <div className="mt-1 flex flex-wrap gap-1"><Aliases aliases={a.agent.aliases} /></div>}
                  {a.agent.status_text
                    ? <div className="truncate text-xs text-working">● {a.agent.status_text}</div>
                    : <div className="truncate text-xs text-muted-foreground">{a.agent.description}</div>}
                </td>
                <td className="px-4 py-3"><StateBadge state={a.state} /></td>
                <td className="px-4 py-3"><HarnessBadge harness={a.agent.binding.harness} /></td>
                <td className="max-w-[200px] px-4 py-3">
                  <div className="truncate text-xs">{a.agent.binding.environment || "—"}</div>
                  <CopyText text={a.agent.binding.session_id || ""} display={shortId(a.agent.binding.session_id, 16)} />
                </td>
                <td className="max-w-[240px] px-4 py-3"><div className="flex flex-wrap gap-1"><Labels labels={a.agent.labels} onPick={onLabel} max={3} /></div></td>
                <td className="px-4 py-3"><Counters a={a} /></td>
                <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">{timeAgo(a.agent.last_seen_at, now)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}
