import { ArrowRight, Megaphone, Paperclip, Reply, StickyNote, ListTodo, AlertTriangle, Activity, Navigation } from "lucide-react";
import type { Message } from "@/api";
import { Badge } from "@/components/ui/badge";
import { cn, bytes, clock, timeAgo } from "@/lib/utils";

const typeMeta: Record<string, { label: string; icon: typeof ListTodo; cls: string }> = {
  task: { label: "任务", icon: ListTodo, cls: "text-primary" },
  reply: { label: "回复", icon: Reply, cls: "text-idle" },
  note: { label: "通知", icon: StickyNote, cls: "text-muted-foreground" },
  event: { label: "事件", icon: AlertTriangle, cls: "text-pending" },
  progress: { label: "进展", icon: Activity, cls: "text-working" },
  steer: { label: "调整", icon: Navigation, cls: "text-pending" },
};

const statusMeta: Record<string, { label: string; cls: string }> = {
  queued: { label: "排队中", cls: "bg-pending/10 text-pending" },
  delivered: { label: "已投递", cls: "bg-idle/10 text-idle" },
  acked: { label: "处理中", cls: "bg-working/10 text-working" },
  held: { label: "已挂起", cls: "bg-idle/10 text-idle" },
  done: { label: "完成", cls: "bg-secondary text-muted-foreground" },
  failed: { label: "失败", cls: "bg-destructive/10 text-destructive" },
};

export function TypeIcon({ type }: { type: string }) {
  const m = typeMeta[type] || typeMeta.note;
  const Icon = m.icon;
  return <Icon className={cn("h-4 w-4 shrink-0", m.cls)} aria-label={m.label} />;
}

export function MsgStatus({ status }: { status: string }) {
  const m = statusMeta[status] || { label: status, cls: "bg-secondary" };
  return <span className={cn("inline-flex rounded px-1.5 py-0.5 text-[11px] font-medium", m.cls)}>{m.label}</span>;
}

export function MessageRow({ m, onOpen, now }: { m: Message; onOpen: () => void; now: number }) {
  return (
    <button type="button" onClick={onOpen} className="flex w-full items-start gap-3 px-4 py-3 text-left transition hover:bg-muted/40">
      <TypeIcon type={m.type} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
          <span className="font-mono font-medium">{m.from}</span>
          <ArrowRight className="h-3 w-3 text-muted-foreground" />
          <span className="font-mono font-medium">{m.to}</span>
          {m.type !== "progress" && <MsgStatus status={m.status} />}
          {m.broadcast_id && (
            <Badge variant="outline" className="gap-1 text-[10px]"><Megaphone className="h-3 w-3" />广播</Badge>
          )}
          {!!m.attachments?.length && (
            <Badge variant="outline" className="gap-1 text-[10px]"><Paperclip className="h-3 w-3" />{m.attachments.length}</Badge>
          )}
          <span className="ml-auto text-muted-foreground" title={m.updated_at}>{timeAgo(m.updated_at, now)}</span>
        </div>
        <p className="mt-1 line-clamp-2 whitespace-pre-line break-words text-sm text-muted-foreground">{m.body}</p>
      </div>
    </button>
  );
}

export function MessageBubble({ m, self }: { m: Message; self?: string }) {
  const mine = self && m.from === self;
  return (
    <div className={cn("flex flex-col gap-1", mine ? "items-end" : "items-start")}>
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <TypeIcon type={m.type} />
        <span className="font-mono font-medium text-foreground">{m.from}</span>
        <span>→ {m.to}</span>
        <span>· {clock(m.created_at)}</span>
        {m.type !== "progress" && <MsgStatus status={m.status} />}
      </div>
      <div className={cn("max-w-[92%] rounded-xl border px-3.5 py-2.5 text-sm shadow-sm", mine ? "bg-accent/60" : "bg-card")}>
        <pre className="whitespace-pre-wrap break-words font-sans leading-relaxed">{m.body}</pre>
        {m.error && <p className="mt-2 text-xs text-destructive">错误：{m.error}</p>}
        {!!m.attachments?.length && (
          <div className="mt-2 space-y-1 border-t pt-2">
            {m.attachments.map((a) => (
              <div key={a.file_id} className="flex items-center gap-2 text-xs text-muted-foreground">
                <Paperclip className="h-3.5 w-3.5" />
                <span className="font-medium text-foreground">{a.name}</span>
                <span>{bytes(a.size)}</span>
                <code className="ml-auto rounded bg-muted px-1">{a.file_id}</code>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
