import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Activity, CircleDot, Filter, LayoutGrid, List, Moon, Network, RefreshCw, Search, ShieldCheck, Sun, Users, Wifi, WifiOff, X, Zap, Inbox,
} from "lucide-react";
import { api, type AgentStatus, type Message, type Overview, type Session, type State, type ViewAs } from "@/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tip, TooltipProvider } from "@/components/ui/tooltip";
import { AgentCard, AgentTable, Aliases, Labels } from "@/components/agents";
import { MessageBubble, MessageRow } from "@/components/messages";
import { HarnessBadge, StateBadge } from "@/components/status";
import { CopyText } from "@/components/copy";
import { cn, timeAgo } from "@/lib/utils";

const POLL_MS = 5000;

// --- selector matching (mirrors the hub's label selector) ---
function matches(a: AgentStatus, selector: string): boolean {
  const vals = (key: string): string[] => {
    if (key === "name") return [a.agent.name];
    if (key === "harness") return a.agent.binding.harness ? [a.agent.binding.harness] : [];
    if (key === "capability") return a.agent.capabilities;
    return a.agent.labels.filter((l) => l.split("=")[0] === key).map((l) => l.split("=").slice(1).join("="));
  };
  return selector.split(",").map((t) => t.trim()).filter(Boolean).every((t) => {
    if (t.includes("!=")) { const [k, v] = t.split("!="); return !vals(k.trim()).includes(v.trim()); }
    if (t.includes("=")) { const [k, ...v] = t.split("="); return vals(k.trim()).includes(v.join("=").trim()); }
    if (t.startsWith("!")) return vals(t.slice(1)).length === 0;
    return vals(t).length > 0;
  });
}

function useTheme() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    try { localStorage.setItem("myco-theme", dark ? "dark" : "light"); } catch { /* storage unavailable */ }
  }, [dark]);
  return [dark, setDark] as const;
}

function StatCard({ label, value, icon: Icon, tone, active, onClick }: {
  label: string; value: number | string; icon: typeof Users; tone: string; active?: boolean; onClick?: () => void;
}) {
  return (
    <Card
      onClick={onClick}
      className={cn("flex cursor-pointer items-center gap-4 p-4 transition hover:shadow-md", active && "ring-2 ring-primary/40")}
    >
      <div className={cn("flex h-10 w-10 items-center justify-center rounded-lg", tone)}>
        <Icon className="h-5 w-5" />
      </div>
      <div>
        <div className="text-2xl font-semibold tabular-nums tracking-tight">{value}</div>
        <div className="text-xs text-muted-foreground">{label}</div>
      </div>
    </Card>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[110px_1fr] items-start gap-3 py-2 text-sm">
      <div className="text-muted-foreground">{label}</div>
      <div className="min-w-0 break-words">{children}</div>
    </div>
  );
}

export default function App() {
  const [dark, setDark] = useTheme();
  const [session, setSession] = useState<Session | null>(null);
  const [viewAs, setViewAs] = useState<ViewAs>("agent");
  const [ov, setOv] = useState<Overview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [now, setNow] = useState(Date.now());
  const [query, setQuery] = useState("");
  const [selector, setSelector] = useState("");
  const [stateFilter, setStateFilter] = useState<State | "all">("all");
  const [layout, setLayout] = useState<"grid" | "table">(() => (localStorage.getItem("myco-layout") as "grid" | "table") || "grid");
  const [openAgent, setOpenAgent] = useState<string | null>(null);
  const [thread, setThread] = useState<{ id: string; messages: Message[] | null; error?: string } | null>(null);

  useEffect(() => {
    api.session().then((s) => {
      setSession(s);
      if (!s.agent && s.admin) setViewAs("admin");
    }).catch((e) => setError(String(e.message || e)));
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setOv(await api.overview(viewAs));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
      setNow(Date.now());
    }
  }, [viewAs]);

  useEffect(() => {
    if (!session) return;
    refresh();
    const t = setInterval(() => { if (!document.hidden) refresh(); }, POLL_MS);
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => { clearInterval(t); clearInterval(tick); };
  }, [session, refresh]);

  useEffect(() => { try { localStorage.setItem("myco-layout", layout); } catch { /* ignore */ } }, [layout]);

  const selectorValid = useMemo(() => !selector || /^\s*!?[a-z0-9][a-z0-9._/-]*(\s*!?=\s*[^,]+)?(\s*,\s*!?[a-z0-9][a-z0-9._/-]*(\s*!?=\s*[^,]+)?)*\s*$/.test(selector), [selector]);

  const agents = useMemo(() => {
    if (!ov) return [];
    const q = query.trim().toLowerCase();
    return ov.agents.filter((a) =>
      (stateFilter === "all" || a.state === stateFilter) &&
      (!selector || !selectorValid || matches(a, selector)) &&
      (!q || [a.agent.name, a.agent.display_name, ...(a.agent.aliases || []), a.agent.description, a.agent.id, a.agent.binding.environment, a.agent.binding.session_id, ...a.agent.labels]
        .filter(Boolean).some((x) => String(x).toLowerCase().includes(q))),
    );
  }, [ov, query, selector, selectorValid, stateFilter]);

  const allLabels = useMemo(() => {
    const m = new Map<string, number>();
    ov?.agents.forEach((a) => a.agent.labels.forEach((l) => m.set(l, (m.get(l) || 0) + 1)));
    return [...m.entries()].sort((a, b) => b[1] - a[1]).slice(0, 12);
  }, [ov]);

  const pickLabel = (l: string) => {
    const terms = selector.split(",").map((t) => t.trim()).filter(Boolean);
    setSelector(terms.includes(l) ? terms.filter((t) => t !== l).join(",") : [...terms, l].join(","));
  };

  const current = ov?.agents.find((a) => a.agent.name === openAgent) || null;
  const self = ov?.viewer.kind === "agent" ? ov.viewer.name : undefined;

  const openThread = async (id: string) => {
    setThread({ id, messages: null });
    try {
      const r = await api.thread(id, viewAs);
      setThread({ id, messages: r.messages });
    } catch (e) {
      setThread({ id, messages: [], error: e instanceof Error ? e.message : String(e) });
    }
  };

  const fresh = ov ? (now - new Date(ov.generated_at).getTime()) / 1000 < POLL_MS / 1000 * 3 : false;

  return (
    <TooltipProvider>
      <div className="min-h-full bg-grid">
        {/* Header */}
        <header className="sticky top-0 z-30 border-b bg-background/80 backdrop-blur-md">
          <div className="container flex h-14 items-center gap-2 px-4 sm:gap-4 sm:px-6">
            <div className="flex items-center gap-2.5">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
                <Network className="h-4 w-4" />
              </div>
              <div className="leading-tight">
                <div className="text-sm font-semibold tracking-tight">myco</div>
                <div className="hidden max-w-[260px] truncate text-[11px] text-muted-foreground sm:block">{session?.hub || "…"}</div>
              </div>
            </div>
            <div className="ml-auto flex min-w-0 items-center gap-1 sm:gap-2">
              <Tip content={error ? "无法连接 Hub" : fresh ? `每 ${POLL_MS / 1000} 秒自动刷新` : "数据可能已过期"}>
                <span className={cn("hidden items-center gap-1.5 rounded-full px-2.5 py-1 text-xs sm:inline-flex", error ? "bg-destructive/10 text-destructive" : "bg-muted text-muted-foreground")}>
                  {error ? <WifiOff className="h-3.5 w-3.5" /> : <Wifi className={cn("h-3.5 w-3.5", fresh && "text-working")} />}
                  {ov ? `更新于 ${timeAgo(ov.generated_at, now)}` : "连接中"}
                </span>
              </Tip>
              {session && (session.admin && session.agent) ? (
                <Tabs value={viewAs} onValueChange={(v) => setViewAs(v as ViewAs)}>
                  <TabsList className="h-8">
                    <TabsTrigger value="agent" className="px-2 text-xs sm:px-3" title={session.agent}><CircleDot className="h-3.5 w-3.5" /><span className="hidden max-w-[140px] truncate sm:inline">{session.agent}</span></TabsTrigger>
                    <TabsTrigger value="admin" className="px-2 text-xs sm:px-3" title="管理员视图"><ShieldCheck className="h-3.5 w-3.5" /><span className="hidden sm:inline">管理员</span></TabsTrigger>
                  </TabsList>
                </Tabs>
              ) : session && (
                <span className="inline-flex items-center gap-1.5 rounded-full bg-accent px-2.5 py-1 text-xs font-medium text-accent-foreground">
                  {session.admin ? <ShieldCheck className="h-3.5 w-3.5" /> : <CircleDot className="h-3.5 w-3.5" />}
                  {session.admin && !session.agent ? "管理员视图" : session.agent}
                </span>
              )}
              <Button variant="ghost" size="icon" onClick={refresh} aria-label="刷新">
                <RefreshCw className={cn(loading && "animate-spin")} />
              </Button>
              <Button variant="ghost" size="icon" onClick={() => setDark(!dark)} aria-label="切换主题">
                {dark ? <Sun /> : <Moon />}
              </Button>
            </div>
          </div>
        </header>

        <main className="container space-y-6 px-4 py-6 sm:px-6">
          {error && (
            <Card className="flex items-start gap-3 border-destructive/30 bg-destructive/5 p-4 text-sm">
              <WifiOff className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
              <div>
                <div className="font-medium text-destructive">无法获取网络状态</div>
                <div className="mt-0.5 text-muted-foreground">{error}</div>
              </div>
            </Card>
          )}

          {/* Stats */}
          <section className="grid grid-cols-2 gap-3 md:grid-cols-5">
            {ov ? (
              <>
                <StatCard label="全部节点" value={ov.totals.agents} icon={Users} tone="bg-primary/10 text-primary" active={stateFilter === "all"} onClick={() => setStateFilter("all")} />
                <StatCard label="工作中" value={ov.totals.working} icon={Zap} tone="bg-working/10 text-working" active={stateFilter === "working"} onClick={() => setStateFilter("working")} />
                <StatCard label="有待办" value={ov.totals.pending} icon={Inbox} tone="bg-pending/10 text-pending" active={stateFilter === "pending"} onClick={() => setStateFilter("pending")} />
                <StatCard label="空闲" value={ov.totals.idle} icon={Activity} tone="bg-idle/10 text-idle" active={stateFilter === "idle"} onClick={() => setStateFilter("idle")} />
                <StatCard label="离线" value={ov.totals.offline} icon={WifiOff} tone="bg-offline/15 text-offline" active={stateFilter === "offline"} onClick={() => setStateFilter("offline")} />
              </>
            ) : (
              Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-[74px] rounded-xl" />)
            )}
          </section>

          <Tabs defaultValue="agents">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <TabsList>
                <TabsTrigger value="agents"><Users className="h-4 w-4" />节点</TabsTrigger>
                <TabsTrigger value="activity"><Activity className="h-4 w-4" />动态{ov ? <span className="ml-1 rounded bg-muted px-1 text-[10px] tabular-nums">{ov.recent.length}</span> : null}</TabsTrigger>
              </TabsList>
              {ov?.viewer.kind === "agent" && (
                <p className="text-xs text-muted-foreground">以 <span className="font-mono">{ov.viewer.name}</span> 的身份查看：其他节点的消息内容不可见</p>
              )}
            </div>

            <TabsContent value="agents" className="mt-4 space-y-4">
              {/* Toolbar */}
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
                <div className="relative flex-1">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索名字、别名、描述、环境、会话…" className="pl-9" />
                </div>
                <div className="relative lg:w-[340px]">
                  <Filter className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={selector}
                    onChange={(e) => setSelector(e.target.value)}
                    placeholder="标签选择器，如 group=loops,!gpu"
                    className={cn("pl-9 font-mono text-xs", !selectorValid && "border-destructive focus-visible:ring-destructive")}
                  />
                  {selector && (
                    <button className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground hover:text-foreground" onClick={() => setSelector("")} aria-label="清除">
                      <X className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>
                <Tabs value={layout} onValueChange={(v) => setLayout(v as "grid" | "table")}>
                  <TabsList>
                    <TabsTrigger value="grid" aria-label="卡片"><LayoutGrid className="h-4 w-4" /></TabsTrigger>
                    <TabsTrigger value="table" aria-label="表格"><List className="h-4 w-4" /></TabsTrigger>
                  </TabsList>
                </Tabs>
              </div>
              {allLabels.length > 0 && (
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="mr-1 text-xs text-muted-foreground">常用标签</span>
                  {allLabels.map(([l, n]) => {
                    const on = selector.split(",").map((t) => t.trim()).includes(l);
                    return (
                      <button
                        key={l}
                        onClick={() => pickLabel(l)}
                        className={cn(
                          "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 font-mono text-[11px] transition",
                          on ? "border-primary bg-primary text-primary-foreground" : "bg-card hover:border-primary/50",
                        )}
                      >
                        {l}<span className={cn("tabular-nums", on ? "opacity-80" : "text-muted-foreground")}>{n}</span>
                      </button>
                    );
                  })}
                </div>
              )}

              {!ov ? (
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-[230px] rounded-xl" />)}
                </div>
              ) : agents.length === 0 ? (
                <Card className="flex flex-col items-center justify-center gap-2 p-12 text-center">
                  <Users className="h-8 w-8 text-muted-foreground/50" />
                  <div className="font-medium">没有匹配的节点</div>
                  <div className="text-sm text-muted-foreground">调整搜索、选择器或状态筛选试试</div>
                </Card>
              ) : layout === "grid" ? (
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  {agents.map((a) => <AgentCard key={a.agent.id} a={a} onOpen={() => setOpenAgent(a.agent.name)} onLabel={pickLabel} now={now} />)}
                </div>
              ) : (
                <AgentTable agents={agents} onOpen={(a) => setOpenAgent(a.agent.name)} onLabel={pickLabel} now={now} />
              )}
            </TabsContent>

            <TabsContent value="activity" className="mt-4">
              <Card className="overflow-hidden">
                <div className="flex items-center justify-between border-b px-4 py-3">
                  <div className="text-sm font-medium">{ov?.viewer.kind === "admin" ? "全网最近消息" : "与我相关的最近消息"}</div>
                  <div className="text-xs text-muted-foreground">点击查看完整对话</div>
                </div>
                {!ov ? (
                  <div className="space-y-2 p-4">{Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-12" />)}</div>
                ) : ov.recent.length === 0 ? (
                  <div className="p-12 text-center text-sm text-muted-foreground">还没有消息</div>
                ) : (
                  <div className="divide-y">{ov.recent.map((m) => <MessageRow key={m.id} m={m} now={now} onOpen={() => openThread(m.thread_id)} />)}</div>
                )}
              </Card>
            </TabsContent>
          </Tabs>

          <footer className="pb-4 pt-2 text-center text-xs text-muted-foreground">
            myco {session?.version} · 只读视图 · 数据来自 Hub 的 /v1/overview
          </footer>
        </main>

        {/* Agent detail */}
        <Sheet open={!!current} onOpenChange={(o) => !o && setOpenAgent(null)}>
          {current && (
            <SheetContent>
              <div className="border-b p-6 pr-12">
                <div className="flex items-center gap-2">
                  <SheetTitle className={cn("text-lg font-semibold", !current.agent.display_name && "font-mono")}>
                    {current.agent.display_name || current.agent.name}
                  </SheetTitle>
                  <StateBadge state={current.state} />
                </div>
                {current.agent.display_name && <div className="mt-0.5 font-mono text-xs text-muted-foreground"><CopyText text={current.agent.name} display={current.agent.name} /></div>}
                <SheetDescription className="mt-1.5 text-sm text-muted-foreground">{current.agent.description || "（没有描述）"}</SheetDescription>
                <div className="mt-3 flex flex-wrap gap-1.5">
                  <HarnessBadge harness={current.agent.binding.harness} />
                  <Aliases aliases={current.agent.aliases} />
                  {current.agent.capabilities.map((c) => <span key={c} className="rounded-md bg-secondary px-1.5 py-0.5 text-[11px]">{c}</span>)}
                </div>
              </div>
              <div className="flex-1 space-y-6 overflow-y-auto p-6 scrollbar-thin">
                <section>
                  <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">当前工作</h4>
                  <div className="grid grid-cols-3 gap-2">
                    {[
                      { k: "处理中", v: current.in_progress, c: "text-working" },
                      { k: "待处理", v: current.queued, c: "text-pending" },
                      { k: "已挂起", v: current.held, c: "text-idle" },
                    ].map((x) => (
                      <Card key={x.k} className="p-3 text-center">
                        <div className={cn("text-xl font-semibold tabular-nums", x.v ? x.c : "text-muted-foreground/60")}>{x.v}</div>
                        <div className="text-xs text-muted-foreground">{x.k}</div>
                      </Card>
                    ))}
                  </div>
                  {current.current && current.current.length > 0 ? (
                    <div className="mt-3 space-y-2">
                      {current.current.map((m) => (
                        <Card key={m.id} className="cursor-pointer p-3 text-sm transition hover:shadow-md" onClick={() => openThread(m.thread_id)}>
                          <div className="mb-1 flex items-center gap-2 text-xs text-muted-foreground">
                            <span className="font-mono">{m.from}</span>→<span className="font-mono">{m.to}</span>
                            <span className="ml-auto">{timeAgo(m.updated_at, now)}</span>
                          </div>
                          <p className="line-clamp-3 whitespace-pre-line text-muted-foreground">{m.body}</p>
                        </Card>
                      ))}
                    </div>
                  ) : current.in_progress + current.held > 0 ? (
                    <p className="mt-2 text-xs text-muted-foreground">任务内容仅对该节点本身和管理员可见。</p>
                  ) : null}
                </section>

                <section>
                  <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">身份与绑定</h4>
                  <div className="divide-y rounded-xl border bg-card px-4">
                    <Field label="Agent ID"><CopyText text={current.agent.id} /></Field>
                    <Field label="会话 ID">{current.agent.binding.session_id ? <CopyText text={current.agent.binding.session_id} /> : "—"}</Field>
                    <Field label="环境">{current.agent.binding.environment || "—"}</Field>
                    <Field label="工作目录"><span className="font-mono text-xs">{current.agent.binding.workdir || "—"}</span></Field>
                    <Field label="投递方式">{current.agent.delivery.mode}</Field>
                    <Field label="入网令牌">{current.agent.enrolled_by ? <span className="font-mono text-xs">{current.agent.enrolled_by}</span> : "管理员注册"}</Field>
                    <Field label="允许来源">{current.agent.allow_from.length ? current.agent.allow_from.join(", ") : "任何人"}</Field>
                    <Field label="最近在线">{timeAgo(current.agent.last_seen_at, now)}</Field>
                    <Field label="注册时间">{new Date(current.agent.created_at).toLocaleString()}</Field>
                  </div>
                </section>

                <section>
                  <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">标签</h4>
                  <Labels labels={current.agent.labels} max={32} />
                </section>

                {ov && (
                  <section>
                    <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">最近往来</h4>
                    {(() => {
                      const rel = ov.recent.filter((m) => m.from === current.agent.name || m.to === current.agent.name).slice(0, 8);
                      return rel.length ? (
                        <Card className="divide-y overflow-hidden">{rel.map((m) => <MessageRow key={m.id} m={m} now={now} onOpen={() => openThread(m.thread_id)} />)}</Card>
                      ) : (
                        <p className="text-xs text-muted-foreground">{ov.viewer.kind === "admin" ? "最近没有消息" : "你和它最近没有往来"}</p>
                      );
                    })()}
                  </section>
                )}
              </div>
            </SheetContent>
          )}
        </Sheet>

        {/* Thread viewer */}
        <Sheet open={!!thread} onOpenChange={(o) => !o && setThread(null)}>
          {thread && (
            <SheetContent className="max-w-2xl">
              <div className="border-b p-6 pr-12">
                <SheetTitle className="text-lg font-semibold">对话线程</SheetTitle>
                <SheetDescription className="mt-1"><CopyText text={thread.id} /></SheetDescription>
              </div>
              <div className="flex-1 space-y-4 overflow-y-auto p-6 scrollbar-thin">
                {thread.error && <p className="text-sm text-destructive">{thread.error}</p>}
                {!thread.messages ? (
                  Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-20" />)
                ) : (
                  thread.messages.map((m) => <MessageBubble key={m.id} m={m} self={self} />)
                )}
              </div>
            </SheetContent>
          )}
        </Sheet>
      </div>
    </TooltipProvider>
  );
}
