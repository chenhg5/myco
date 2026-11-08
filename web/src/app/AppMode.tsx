import { useCallback, useEffect, useMemo, useState } from "react";
import { Moon, Plus, RefreshCw, Server, Sun, WifiOff } from "lucide-react";
import "@/app/app.css";
import { appApi, errorMessage, type AppOverview, type Hub } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow, Filters } from "@/app/types";
import { AddHubDialog } from "@/app/AddHubDialog";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { HubsPage } from "@/app/HubsPage";
import { Inspector } from "@/app/Inspector";
import { NodesPage } from "@/app/NodesPage";
import { LogoSymbol, LogoWordmark } from "@/components/brand";
import { useTheme } from "@/lib/theme";
import { cn, timeAgo } from "@/lib/utils";

const POLL_MS = 5000;

type Page = "nodes" | "hubs";

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
  const [page, setPage] = useState<Page>("nodes");
  const [filters, setFilters] = useState<Filters>({ query: "", hub: "all", machine: "all", state: "all" });
  const [inspect, setInspect] = useState<{ hubId: string; name: string } | null>(null);
  const [thread, setThread] = useState<ThreadState | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<Hub | null>(null);
  const [removeBusy, setRemoveBusy] = useState(false);
  const [removeError, setRemoveError] = useState<unknown>(null);

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
    const onVisible = () => { if (document.visibilityState === "visible") void refresh(); };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(poll);
      clearInterval(tick);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refresh]);

  // A removed hub must not stay selected as a filter.
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
    if (!inspect) return null;
    return rows.find((r) => r.hub.id === inspect.hubId && r.st.agent.name === inspect.name) || null;
  }, [rows, inspect]);

  const openThread = async (hubId: string, threadId: string) => {
    const hubName = ov?.hubs.find((h) => h.id === hubId)?.name || hubId;
    setThread({ hubId, hubName, id: threadId, messages: null });
    try {
      const r = await appApi.thread(hubId, threadId);
      setThread({ hubId, hubName, id: threadId, messages: r.messages });
    } catch (e) {
      setThread({ hubId, hubName, id: threadId, messages: [], error: errorMessage(e) });
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

  const dragStyle = { "--wails-draggable": "drag" } as React.CSSProperties;
  const noHubs = hubs.length === 0;

  return (
    <div className="app-shell">
      <header className="app-topbar" style={dragStyle}>
        <div className="brand">
          <LogoSymbol className="h-[20px] w-[20px]" />
          <LogoWordmark className="h-[15px] w-auto" />
        </div>
        <div className="center">
          <div className="seg" role="tablist" aria-label="页面">
            <button type="button" role="tab" aria-selected={page === "nodes"} className={cn("seg-btn", page === "nodes" && "active")} onClick={() => setPage("nodes")}>节点</button>
            <button type="button" role="tab" aria-selected={page === "hubs"} className={cn("seg-btn", page === "hubs" && "active")} onClick={() => setPage("hubs")}>Hub</button>
          </div>
        </div>
        <div className="actions">
          <button type="button" className="app-btn app-btn-plain" onClick={() => void refresh()} aria-label="刷新" title="刷新">
            <RefreshCw className={cn(loading && "animate-spin")} />
          </button>
          <button type="button" className="app-btn app-btn-plain" onClick={() => setDark(!dark)} aria-label="切换主题" title="切换主题">
            {dark ? <Sun /> : <Moon />}
          </button>
        </div>
      </header>

      <div className="app-scroll">
        {error !== null && (
          <div className="app-banner-wrap">
            <div className="app-error-row">
              <WifiOff />
              <span className="err" title={errorMessage(error)}>无法获取 App 数据：{errorMessage(error)}</span>
            </div>
          </div>
        )}

        {noHubs ? (
          <div className="app-empty">
            <div className="app-empty-icon"><Server /></div>
            <h2>连接第一个 Hub</h2>
            <p>这个 App 会汇总你添加的所有 Hub 上的 agent。每个 Hub 相互独立；控制台令牌保存在钥匙串里，不会再显示。</p>
            <button type="button" className="app-btn app-btn-primary" onClick={() => setAddOpen(true)}>
              <Plus />添加 Hub
            </button>
            <div>
              <code className="app-code">myco console token create --name &lt;设备名&gt;</code>
            </div>
          </div>
        ) : page === "nodes" ? (
          <NodesPage
            rows={rows}
            hubs={ov?.hubs}
            hubMeta={hubs}
            loading={loading}
            filters={filters}
            onFilters={setFilters}
            now={now}
            onOpen={(r) => setInspect({ hubId: r.hub.id, name: r.st.agent.name })}
          />
        ) : (
          <HubsPage
            hubs={hubs}
            hubsOv={ov?.hubs}
            onAdd={() => setAddOpen(true)}
            onRemove={(h) => { setRemoveError(null); setRemoveTarget(h); }}
            onOpenHub={(h) => { setFilters((f) => ({ ...f, hub: h.id })); setPage("nodes"); }}
          />
        )}
      </div>

      <AddHubDialog open={addOpen} onOpenChange={setAddOpen} onAdded={() => void refresh()} />

      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(o) => !o && setRemoveTarget(null)}
        title={removeTarget ? `移除 ${removeTarget.name}？` : "移除 Hub"}
        description="只会从这个 App 里移除。Hub 本身和它上面的 agent 都不受影响，仍在照常运行。"
        consequences={<p>你将看不到这个 Hub 的节点，之后只能凭控制台令牌重新添加。</p>}
        confirmLabel="移除 Hub"
        busy={removeBusy}
        error={removeError ? <ErrorText error={removeError} /> : undefined}
        onConfirm={() => void removeHub()}
      />

      <Inspector
        row={current}
        now={now}
        onClose={() => setInspect(null)}
        onRefresh={refresh}
        onThread={(hubId, threadId) => void openThread(hubId, threadId)}
      />

      <Modal
        open={!!thread}
        onOpenChange={(o) => !o && setThread(null)}
        wide
        title="对话线程"
        description={thread ? <span className="mono">{thread.hubName} · {thread.id}</span> : ""}
      >
        {thread?.error && <ErrorText error={thread.error} />}
        <div className="app-thread-list">
          {!thread?.messages ? (
            <div className="app-empty-note">正在加载…</div>
          ) : thread.messages.length === 0 ? (
            <div className="app-empty-note">这个线程还没有消息。</div>
          ) : (
            thread.messages.map((m) => (
              <div className="app-thread-msg" key={m.id}>
                <div className="meta">
                  <span className="who">{m.from}</span>→<span className="who">{m.to}</span>
                  <span className="time">{timeAgo(m.created_at, now)}</span>
                </div>
                <div className="body">{m.body}</div>
              </div>
            ))
          )}
        </div>
      </Modal>
    </div>
  );
}
