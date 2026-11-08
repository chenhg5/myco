import { useEffect, useState } from "react";
import { Loader2, LogOut, Pencil, Trash2 } from "lucide-react";
import { appApi, errorMessage } from "@/appapi";
import type { AgentRow } from "@/app/types";
import { ConfirmDialog, ErrorNote } from "@/app/dialogs";
import { Aliases, Labels, StatusLine } from "@/components/agents";
import { CopyText } from "@/components/copy";
import { MessageRow } from "@/components/messages";
import { HarnessBadge, StateBadge } from "@/components/status";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { cn, timeAgo } from "@/lib/utils";

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[110px_1fr] items-start gap-3 py-2 text-sm">
      <div className="text-muted-foreground">{label}</div>
      <div className="min-w-0 break-words">{children}</div>
    </div>
  );
}

type Confirm = null | "kick" | "delete";

export function AgentDrawer({ row, now, onClose, onRefresh, onThread }: {
  row: AgentRow | null;
  now: number;
  onClose: () => void;
  onRefresh: () => Promise<void>;
  onThread: (hubId: string, threadId: string) => void;
}) {
  const ag = row?.st.agent;
  const [editing, setEditing] = useState(false);
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [allowFrom, setAllowFrom] = useState("");
  const [editingAllow, setEditingAllow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [confirm, setConfirm] = useState<Confirm>(null);
  const [confirmError, setConfirmError] = useState<unknown>(null);

  useEffect(() => {
    setEditing(false);
    setEditingAllow(false);
    setError(null);
    setConfirm(null);
    setConfirmError(null);
    setDisplayName(ag?.display_name || "");
    setDescription(ag?.description || "");
    setAllowFrom((ag?.allow_from || []).join(", "));
    // Reset the forms only when a different agent is opened, not on every poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [row?.hub.id, ag?.id]);

  if (!row || !ag) {
    return <Sheet open={false} onOpenChange={(o) => !o && onClose()} />;
  }

  const hubId = row.hub.id;
  const hubName = row.hub.name;
  const cur = row.st;
  const recent = (row.hub.overview?.recent || []).filter((m) => m.from === ag.name || m.to === ag.name).slice(0, 8);

  const run = async (fn: () => Promise<void>, after?: "close" | "reload") => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await onRefresh();
      setEditing(false);
      setEditingAllow(false);
      if (after === "close") onClose();
    } catch (e) {
      setError(e);
      throw e;
    } finally {
      setBusy(false);
    }
  };

  const saveProfile = () => run(() => appApi.updateProfile(hubId, ag.name, {
    display_name: displayName.trim(),
    description: description.trim(),
  })).catch(() => { /* shown inline */ });

  const saveAllowFrom = () => run(() => appApi.updateAllowFrom(hubId, ag.name,
    allowFrom.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean))).catch(() => { /* shown inline */ });

  return (
    <>
      <Sheet open onOpenChange={(o) => !o && onClose()}>
        <SheetContent>
          <div className="border-b p-6 pr-12">
            <div className="mb-2 flex items-center gap-1.5 text-xs text-muted-foreground">
              <span className="h-1.5 w-1.5 rounded-full bg-working" />
              <span>{hubName}</span>
            </div>
            <div className="flex items-center gap-2">
              <SheetTitle className={cn("text-lg font-semibold", !ag.display_name && "font-mono")}>
                {ag.display_name || ag.name}
              </SheetTitle>
              <StateBadge state={cur.state} />
            </div>
            {ag.display_name && <div className="mt-0.5 font-mono text-xs text-muted-foreground"><CopyText text={ag.name} display={ag.name} /></div>}
            <SheetDescription className="mt-1.5 text-sm text-muted-foreground">{ag.description || "（没有描述）"}</SheetDescription>
            {ag.status_text && <div className="mt-3"><StatusLine text={ag.status_text} at={ag.status_at} now={now} /></div>}
            <div className="mt-3 flex flex-wrap gap-1.5">
              <HarnessBadge harness={ag.binding.harness} />
              <Aliases aliases={ag.aliases} />
              {ag.capabilities.map((c) => <span key={c} className="rounded-md bg-secondary px-1.5 py-0.5 text-[11px]">{c}</span>)}
            </div>
          </div>

          <div className="flex-1 space-y-6 overflow-y-auto p-6 scrollbar-thin">
            {error !== null && <ErrorNote error={error} />}

            <section>
              <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">当前工作</h4>
              <div className="grid grid-cols-3 gap-2">
                {[
                  { k: "处理中", v: cur.in_progress, c: "text-working" },
                  { k: "待处理", v: cur.queued, c: "text-pending" },
                  { k: "已挂起", v: cur.held, c: "text-idle" },
                ].map((x) => (
                  <Card key={x.k} className="p-3 text-center">
                    <div className={cn("text-xl font-semibold tabular-nums", x.v ? x.c : "text-muted-foreground/60")}>{x.v}</div>
                    <div className="text-xs text-muted-foreground">{x.k}</div>
                  </Card>
                ))}
              </div>
              {cur.current && cur.current.length > 0 && (
                <div className="mt-3 space-y-2">
                  {cur.current.map((m) => (
                    <Card key={m.id} className="cursor-pointer p-3 text-sm transition hover:shadow-md" onClick={() => onThread(hubId, m.thread_id)}>
                      <div className="mb-1 flex items-center gap-2 text-xs text-muted-foreground">
                        <span className="font-mono">{m.from}</span>→<span className="font-mono">{m.to}</span>
                        <span className="ml-auto">{timeAgo(m.updated_at, now)}</span>
                      </div>
                      <p className="line-clamp-3 whitespace-pre-line text-muted-foreground">{m.body}</p>
                    </Card>
                  ))}
                </div>
              )}
              {recent.length > 0 && (
                <div className="mt-4">
                  <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">最近消息</h4>
                  <Card className="divide-y overflow-hidden">
                    {recent.map((m) => <MessageRow key={m.id} m={m} now={now} onOpen={() => onThread(hubId, m.thread_id)} />)}
                  </Card>
                </div>
              )}
            </section>

            <section>
              <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">身份与绑定</h4>
              <div className="divide-y rounded-xl border bg-card px-4">
                <Field label="Agent ID"><CopyText text={ag.id} /></Field>
                <Field label="会话 ID">{ag.binding.session_id ? <CopyText text={ag.binding.session_id} /> : "—"}</Field>
                <Field label="环境">{ag.binding.environment || "—"}</Field>
                <Field label="工作目录"><span className="font-mono text-xs">{ag.binding.workdir || "—"}</span></Field>
                <Field label="投递方式">{ag.delivery.mode}</Field>
                <Field label="入网令牌">{ag.enrolled_by ? <span className="font-mono text-xs">{ag.enrolled_by}</span> : "admin"}</Field>
                <Field label="最近在线">{timeAgo(ag.last_seen_at, now)}</Field>
                <Field label="注册时间">{new Date(ag.created_at).toLocaleString()}</Field>
              </div>
              <div className="mt-2 flex flex-wrap gap-1"><Labels labels={ag.labels} max={32} /></div>
            </section>

            <section>
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">管理</h4>
              <Card className="space-y-4 p-4">
                {editing ? (
                  <div className="space-y-2">
                    <label className="block space-y-1">
                      <span className="text-xs font-medium text-muted-foreground">显示名</span>
                      <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder={ag.name} />
                    </label>
                    <label className="block space-y-1">
                      <span className="text-xs font-medium text-muted-foreground">描述</span>
                      <textarea
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                        rows={3}
                        className="flex w-full rounded-md border border-input bg-card px-3 py-2 text-sm transition-colors placeholder:text-muted-foreground/80 hover:border-foreground/20 focus-visible:border-foreground/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20"
                      />
                    </label>
                    <div className="flex justify-end gap-2">
                      <Button variant="ghost" size="sm" onClick={() => setEditing(false)} disabled={busy}>取消</Button>
                      <Button size="sm" onClick={() => void saveProfile()} disabled={busy}>
                        {busy && <Loader2 className="animate-spin" />}保存
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="text-sm font-medium">资料</div>
                      <div className="truncate text-xs text-muted-foreground">显示名和描述会在所有 Hub 上生效。</div>
                    </div>
                    <Button variant="outline" size="sm" onClick={() => setEditing(true)}><Pencil />编辑</Button>
                  </div>
                )}

                <div className="border-t pt-4">
                  {editingAllow ? (
                    <div className="space-y-2">
                      <label className="block space-y-1">
                        <span className="text-xs font-medium text-muted-foreground">允许的来源（逗号分隔）</span>
                        <Input value={allowFrom} onChange={(e) => setAllowFrom(e.target.value)} placeholder="glenn, pm-codex（留空 = 任何人）" className="font-mono text-xs" />
                      </label>
                      <div className="flex justify-end gap-2">
                        <Button variant="ghost" size="sm" onClick={() => { setEditingAllow(false); setAllowFrom((ag.allow_from || []).join(", ")); }} disabled={busy}>取消</Button>
                        <Button size="sm" onClick={() => void saveAllowFrom()} disabled={busy}>
                          {busy && <Loader2 className="animate-spin" />}保存
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        <div className="text-sm font-medium">允许来源</div>
                        <div className="truncate text-xs text-muted-foreground">{ag.allow_from.length ? ag.allow_from.join(", ") : "任何人都可以给它发消息"}</div>
                      </div>
                      <Button variant="outline" size="sm" onClick={() => setEditingAllow(true)}><Pencil />编辑</Button>
                    </div>
                  )}
                </div>

                <div className="flex flex-wrap gap-2 border-t pt-4">
                  <Button variant="outline" size="sm" onClick={() => { setConfirmError(null); setConfirm("kick"); }} disabled={busy}>
                    <LogOut />踢下线
                  </Button>
                  <Button variant="outline" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => { setConfirmError(null); setConfirm("delete"); }} disabled={busy}>
                    <Trash2 />删除节点
                  </Button>
                </div>
              </Card>
            </section>
          </div>
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        open={confirm === "kick"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`将 ${ag.name} 从 ${hubName} 踢下线？`}
        description="会立即作废它的令牌。节点记录、消息和资料都保留在 Hub 上，重新接入即可回来。"
        consequences={<p>重新接入前它收不到任何消息，正在进行的任务会被中断。</p>}
        confirmLabel="踢下线"
        busy={busy}
        error={confirmError !== null ? errorMessage(confirmError) : undefined}
        onConfirm={() => { setConfirmError(null); void run(() => appApi.kick(hubId, ag.name), "close").catch((e) => setConfirmError(e)); }}
      />
      <ConfirmDialog
        open={confirm === "delete"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`从 ${hubName} 删除 ${ag.name}？`}
        description="会从 Hub 上永久删除这个节点。"
        consequences={<p>节点记录和令牌都会被删除，必须重新接入才能回来。已有线程里的消息仍会保留。</p>}
        confirmLabel="删除节点"
        busy={busy}
        error={confirmError !== null ? errorMessage(confirmError) : undefined}
        onConfirm={() => { setConfirmError(null); void run(() => appApi.removeAgent(hubId, ag.name), "close").catch((e) => setConfirmError(e)); }}
      />
    </>
  );
}
