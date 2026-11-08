import { useCallback, useEffect, useMemo, useState } from "react";
import { CircleDot, Moon, Plus, RefreshCw, Server, Sun, Terminal, WifiOff } from "lucide-react";
import { appApi, errorMessage, type AppOverview, type Hub } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow } from "@/app/types";
import { AddHubDialog } from "@/app/AddHubDialog";
import { AgentDrawer } from "@/app/AgentDrawer";
import { AgentsSummary, type Filters } from "@/app/AgentsSummary";
import { ConfirmDialog, ErrorNote } from "@/app/dialogs";
import { HubsSidebar } from "@/app/HubsSidebar";
import { LogoSymbol, LogoWordmark } from "@/components/brand";
import { MessageBubble } from "@/components/messages";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Tip, TooltipProvider } from "@/components/ui/tooltip";
import { useTheme } from "@/lib/theme";
import { cn, timeAgo } from "@/lib/utils";

const POLL_MS = 5000;

interface ThreadState {
  hubId: string;
  hubName: string;
  id: string;
  messages: Message[] | null;
  error?: string;
}

export default function AppMode({ initialHubs }: { initialHubs: Hub[] }) {
  const [dark, setDark] = useTheme();
  const [hubs, setHubs] = useState<Hub[]>(initialHubs);
  const [ov, setOv] = useState<AppOverview | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const [now, setNow] = useState(Date.now());
  const [addOpen, setAddOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<Hub | null>(null);
  const [removeBusy, setRemoveBusy] = useState(false);
  const [removeError, setRemoveError] = useState<unknown>(null);
  const [open, setOpen] = useState<{ hubId: string; name: string } | null>(null);
  const [thread, setThread] = useState<ThreadState | null>(null);
  const [filters, setFilters] = useState<Filters>({ query: "", hub: "all", machine: "all", state: "all" });

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [h, o] = await Promise.all([appApi.hubs(), appApi.overview()]);
      setHubs(h);
      setOv(o);
      setError(null);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
      setNow(Date.now());
    }
  }, []);

  useEffect(() => {
    void refresh();
    const poll = setInterval(() => { if (document.visibilityState === "visible") void refresh(); }, POLL_MS);
    const tick = setInterval(() => setNow(Date.now()), 1000);
    // Resume immediately when the window becomes visible again.
    const onVisible = () => { if (document.visibilityState === "visible") void refresh(); };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(poll);
      clearInterval(tick);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refresh]);

  // A hub that was removed must not stay selected as a filter.
  useEffect(() => {
    if (filters.hub !== "all" && !hubs.some((h) => h.id === filters.hub)) {
      setFilters((f) => ({ ...f, hub: "all" }));
    }
  }, [hubs, filters.hub]);

  const rows = useMemo<AgentRow[]>(() => {
    if (!ov) return [];
    return ov.hubs
      .filter((h) => h.ok && h.overview)
      .flatMap((h) => h.overview!.agents.map((st) => ({ hub: h, st })));
  }, [ov]);

  const current = useMemo(() => {
    if (!open) return null;
    return rows.find((r) => r.hub.id === open.hubId && r.st.agent.name === open.name) || null;
  }, [rows, open]);

  const up = hubs.filter((h) => h.ok).length;
  const fresh = ov ? (now - new Date(ov.generated_at).getTime()) / 1000 < (POLL_MS / 1000) * 3 : false;

  const openThread = async (hubId: string, hubName: string, id: string) => {
    setThread({ hubId, hubName, id, messages: null });
    try {
      const r = await appApi.thread(hubId, id);
      setThread({ hubId, hubName, id, messages: r.messages });
    } catch (e) {
      setThread({ hubId, hubName, id, messages: [], error: errorMessage(e) });
    }
  };

  const removeHub = async () => {
    if (!removeTarget) return;
    setRemoveBusy(true);
    setRemoveError(null);
    try {
      await appApi.removeHub(removeTarget.id);
      setRemoveTarget(null);
      await refresh();
    } catch (e) {
      setRemoveError(e);
    } finally {
      setRemoveBusy(false);
    }
  };

  return (
    <TooltipProvider>
      <div className="flex h-full flex-col bg-glow">
        <header className="sticky top-0 z-30 shrink-0 border-b bg-background/80 backdrop-blur-md">
          <div className="flex h-14 items-center gap-3 px-4">
            <div className="flex min-w-0 items-center gap-2">
              <LogoSymbol className="h-[22px] w-[22px]" />
              <LogoWordmark className="h-[17px] w-auto text-foreground" />
            </div>
            <span className="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
              <CircleDot className="h-3 w-3" />App
            </span>
            <div className="ml-auto flex min-w-0 items-center gap-1 sm:gap-2">
              <Tip content={error ? "Cannot reach the app backend" : fresh ? `Refreshes every ${POLL_MS / 1000} seconds` : "Data may be stale"}>
                <span className={cn("mr-1 hidden items-center gap-2 text-xs sm:inline-flex", error ? "text-destructive" : "text-muted-foreground")}>
                  {error ? <WifiOff className="h-3.5 w-3.5" /> : <span className={cn("h-1.5 w-1.5 rounded-full", fresh ? "bg-working shadow-[0_0_0_3px_hsl(var(--working)/0.15)]" : "bg-offline")} />}
                  {hubs.length ? `${up}/${hubs.length} hubs` : "no hubs"}
                  {ov ? ` · ${timeAgo(ov.generated_at, now)}` : ""}
                </span>
              </Tip>
              <Button variant="ghost" size="icon" onClick={() => void refresh()} aria-label="Refresh">
                <RefreshCw className={cn(loading && "animate-spin")} />
              </Button>
              <Button variant="ghost" size="icon" onClick={() => setDark(!dark)} aria-label="Toggle theme">
                {dark ? <Sun /> : <Moon />}
              </Button>
            </div>
          </div>
        </header>

        <div className="flex min-h-0 flex-1 flex-col md:flex-row">
          <HubsSidebar
            hubs={hubs}
            ov={ov?.hubs}
            selected={filters.hub}
            onSelect={(hub) => setFilters((f) => ({ ...f, hub }))}
            onAdd={() => setAddOpen(true)}
            onRemove={(hub) => { setRemoveError(null); setRemoveTarget(hub); }}
          />

          <main className="min-w-0 flex-1 overflow-y-auto scrollbar-thin">
            <div className="mx-auto max-w-6xl space-y-6 px-4 py-6 md:px-6">
              {error !== null && (
                <Card className="flex items-start gap-3 border-destructive/30 bg-destructive/5 p-4 text-sm">
                  <WifiOff className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
                  <div className="min-w-0">
                    <div className="font-medium text-destructive">Cannot load app data</div>
                    <div className="mt-0.5 break-words text-muted-foreground">{errorMessage(error)}</div>
                  </div>
                </Card>
              )}

              {hubs.length === 0 ? (
                <Card className="mx-auto mt-10 max-w-lg p-8 text-center">
                  <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl border bg-gradient-to-b from-card to-muted">
                    <Server className="h-5 w-5 text-muted-foreground" />
                  </span>
                  <h2 className="mt-4 text-lg font-semibold">Connect your first hub</h2>
                  <p className="mt-1.5 text-sm text-muted-foreground">
                    This app aggregates the agents of every hub you add. Each hub stays independent; the console token is stored in your keychain and never shown again.
                  </p>
                  <div className="mt-5 rounded-lg border bg-muted/40 p-4 text-left">
                    <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                      <Terminal className="h-3.5 w-3.5" />On the hub machine, create a console token:
                    </div>
                    <code className="mt-2 block break-all rounded-md bg-background px-3 py-2 font-mono text-xs">
                      myco console token create --name {`<device name>`}
                    </code>
                  </div>
                  <Button className="mt-5" onClick={() => setAddOpen(true)}><Plus />Add hub</Button>
                </Card>
              ) : !ov ? (
                <div className="space-y-4">
                  <Skeleton className="h-8 w-64" />
                  <Skeleton className="h-[86px] rounded-xl" />
                  <Skeleton className="h-72 rounded-xl" />
                </div>
              ) : (
                <>
                  <AgentsSummary
                    rows={rows}
                    hubs={ov.hubs}
                    loading={loading}
                    now={now}
                    filters={filters}
                    onFilters={setFilters}
                    onOpen={(r) => setOpen({ hubId: r.hub.id, name: r.st.agent.name })}
                  />
                  <div className="flex flex-wrap items-center justify-between gap-2 pb-4 text-xs text-muted-foreground/70">
                    <span>Every 5 seconds, paused while the window is hidden. Data comes from each hub's /v1/overview.</span>
                    <Button variant="outline" size="sm" className="md:hidden" onClick={() => setAddOpen(true)}><Plus />Add hub</Button>
                  </div>
                </>
              )}
            </div>
          </main>
        </div>

        <AddHubDialog open={addOpen} onOpenChange={setAddOpen} onAdded={() => void refresh()} />

        <ConfirmDialog
          open={!!removeTarget}
          onOpenChange={(o) => !o && setRemoveTarget(null)}
          title={removeTarget ? `Remove ${removeTarget.name}?` : ""}
          description="The hub is removed from this app only. Nothing changes on the hub itself, and its agents keep running."
          consequences={<p>You will stop seeing this hub's agents and can only re-add it with its console token.</p>}
          confirmLabel="Remove hub"
          busy={removeBusy}
          error={removeError !== null ? errorMessage(removeError) : undefined}
          onConfirm={() => void removeHub()}
        />

        <AgentDrawer
          row={current}
          now={now}
          onClose={() => setOpen(null)}
          onRefresh={refresh}
          onThread={(hubId, tid) => {
            const hub = ov?.hubs.find((h) => h.id === hubId);
            void openThread(hubId, hub?.name || hubId, tid);
          }}
        />

        <Sheet open={!!thread} onOpenChange={(o) => !o && setThread(null)}>
          {thread && (
            <SheetContent className="max-w-2xl">
              <div className="border-b p-6 pr-12">
                <div className="mb-1 text-xs text-muted-foreground">{thread.hubName}</div>
                <SheetTitle className="text-lg font-semibold">Thread</SheetTitle>
                <SheetDescription className="mt-1 font-mono text-xs">{thread.id}</SheetDescription>
              </div>
              <div className="flex-1 space-y-4 overflow-y-auto p-6 scrollbar-thin">
                {thread.error && <ErrorNote error={thread.error} />}
                {!thread.messages ? (
                  Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-20" />)
                ) : thread.messages.length === 0 ? (
                  !thread.error && <p className="text-sm text-muted-foreground">This thread has no messages.</p>
                ) : (
                  thread.messages.map((m) => <MessageBubble key={m.id} m={m} />)
                )}
              </div>
            </SheetContent>
          )}
        </Sheet>
      </div>
    </TooltipProvider>
  );
}
