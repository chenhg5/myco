import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Laptop, LayoutGrid, ListTodo, Monitor, PanelLeft, RefreshCw, Server, Settings, WifiOff,
} from "lucide-react";
import "@/app/app.css";
import { appApi, errorMessage, type AppInfo, type AppOverview, type Hub } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow, Filters } from "@/app/types";
import { AddHubDialog } from "@/app/AddHubDialog";
import { ConfirmDialog, ErrorText, Modal } from "@/app/dialogs";
import { HubsPage } from "@/app/HubsPage";
import { Inspector } from "@/app/Inspector";
import { LocalPage } from "@/app/LocalPage";
import { MachinesPage } from "@/app/MachinesPage";
import { NodesPage } from "@/app/NodesPage";
import { Onboarding } from "@/app/Onboarding";
import { TasksPage } from "@/app/TasksPage";
import { LogoSymbol, LogoWordmark } from "@/components/brand";
import { useThemeMode, type ThemeMode } from "@/lib/theme";
import { cn, timeAgo } from "@/lib/utils";

const POLL_MS = 5000;
const SIDEBAR_KEY = "myco-app-sidebar";

type Page = "nodes" | "tasks" | "hubs" | "machines" | "local";

const PAGE_META: Record<Page, { group: string; label: string }> = {
  nodes: { group: "工作", label: "节点" },
  tasks: { group: "工作", label: "任务" },
  hubs: { group: "网络", label: "Hub" },
  machines: { group: "网络", label: "机器" },
  local: { group: "本机", label: "本机 Hub" },
};

const NAV: { group: string; items: { page: Page; label: string; icon: typeof LayoutGrid }[] }[] = [
  {
    group: "工作",
    items: [
      { page: "nodes", label: "节点", icon: LayoutGrid },
      { page: "tasks", label: "任务", icon: ListTodo },
    ],
  },
  {
    group: "网络",
    items: [
      { page: "hubs", label: "Hub", icon: Server },
      { page: "machines", label: "机器", icon: Monitor },
    ],
  },
  {
    group: "本机",
    items: [{ page: "local", label: "本机 Hub", icon: Laptop }],
  },
];

const THEMES: { mode: ThemeMode; label: string }[] = [
  { mode: "system", label: "跟随系统" },
  { mode: "light", label: "浅色" },
  { mode: "dark", label: "深色" },
];

interface ThreadState {
  hubId: string;
  hubName: string;
  id: string;
  messages: Message[] | null;
  error?: string;
}

function initialCollapsed() {
  try {
    const v = localStorage.getItem(SIDEBAR_KEY);
    if (v === "open") return false;
    if (v === "closed") return true;
  } catch { /* storage unavailable */ }
  return window.innerWidth < 860;
}

const stripSlash = (u: string) => u.replace(/\/+$/, "");

export default function AppMode({ initialHubs, initialInfo }: {
  initialHubs: Hub[];
  initialInfo: AppInfo | null;
}) {
  const [theme, setTheme] = useThemeMode();
  const [info, setInfo] = useState<AppInfo | null>(initialInfo);
  const [collapsed, setCollapsed] = useState(initialCollapsed);
  const [hubs, setHubs] = useState<Hub[]>(initialHubs);
  const [ov, setOv] = useState<AppOverview | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const [now, setNow] = useState(Date.now());
  const [localTick, setLocalTick] = useState(0);
  const [page, setPage] = useState<Page>("nodes");
  const [filters, setFilters] = useState<Filters>({ query: "", hub: "all", machine: "all", state: "all" });
  const [inspect, setInspect] = useState<{ hubId: string; name: string } | null>(null);
  const [thread, setThread] = useState<ThreadState | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<Hub | null>(null);
  const [removeBusy, setRemoveBusy] = useState(false);
  const [removeError, setRemoveError] = useState<unknown>(null);

  // The boot probe already carries app/info; retry once in case it failed.
  useEffect(() => {
    let cancelled = false;
    appApi.info().then((i) => { if (!cancelled) setInfo(i); }).catch(() => { /* keep the probe's answer */ });
    return () => { cancelled = true; };
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [h, o] = await Promise.all([appApi.hubs(), appApi.overview()]);
      setHubs(h);
      setOv(o);
      setError(null);
      setLocalTick((t) => t + 1);
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

  const viewLocalHub = (localUrl: string) => {
    const h = hubs.find((x) => stripSlash(x.url) === stripSlash(localUrl));
    if (h) setFilters((f) => ({ ...f, hub: h.id }));
    setPage("nodes");
  };

  const openLocal = () => setPage("local");

  // Remember explicit toggles; an untouched window uses the 860px default.
  const toggleSidebar = () => {
    setCollapsed((c) => {
      const next = !c;
      try { localStorage.setItem(SIDEBAR_KEY, next ? "closed" : "open"); } catch { /* storage unavailable */ }
      return next;
    });
  };
  const dragStyle = { "--wails-draggable": "drag" } as React.CSSProperties;
  const meta = PAGE_META[page];
  const connected = hubs.filter((h) => h.ok).length;
  const allConnected = hubs.length > 0 && connected === hubs.length;
  const isMac = info?.shell === "mac";

  return (
    <div className={cn("app-shell", isMac && "shell-mac", collapsed && "collapsed")}>
      {!collapsed && (
        <aside className="app-sidebar">
          <div className="app-sidebar-head" style={dragStyle}>
            <div className="brand">
              <LogoSymbol className="h-[18px] w-[18px]" />
              <LogoWordmark className="h-[13px] w-auto" />
            </div>
          </div>

          <nav className="app-nav">
            {NAV.map((g) => (
              <div className="app-nav-group" key={g.group}>
                <div className="app-nav-title">{g.group}</div>
                {g.items.map((it) => (
                  <button
                    key={it.page}
                    type="button"
                    className={cn("app-nav-item", page === it.page && "active")}
                    aria-current={page === it.page ? "page" : undefined}
                    onClick={() => setPage(it.page)}
                  >
                    <it.icon strokeWidth={1.75} />
                    <span>{it.label}</span>
                  </button>
                ))}
              </div>
            ))}
          </nav>

          <div className="app-sidebar-foot">
            <button
              type="button"
              className={cn("app-nav-item", settingsOpen && "active")}
              onClick={() => setSettingsOpen(true)}
            >
              <Settings strokeWidth={1.75} />
              <span>设置</span>
            </button>
            <div
              className="app-sidebar-status"
              title={hubs.length
                ? hubs.map((h) => `${h.name}：${h.ok ? "已连接" : "连不上"}`).join("\n")
                : "还没有添加 Hub"}
            >
              <span className="dot" style={{ background: allConnected ? "var(--green)" : "var(--red)" }} />
              <span>{connected} 个 Hub 已连接</span>
            </div>
          </div>
        </aside>
      )}

      <div className="app-main">
        <header className="app-crumbbar" style={dragStyle}>
          <button
            type="button"
            className="app-btn app-btn-plain"
            onClick={toggleSidebar}
            aria-label={collapsed ? "显示边栏" : "隐藏边栏"}
            title={collapsed ? "显示边栏" : "隐藏边栏"}
          >
            <PanelLeft />
          </button>
          <nav className="app-crumbs" aria-label="位置">
            <span>{meta.group}</span>
            <span className="sep">/</span>
            <span className="cur">{meta.label}</span>
          </nav>
          <div className="app-crumb-actions">
            <button type="button" className="app-btn app-btn-plain" onClick={() => void refresh()} aria-label="刷新" title="刷新">
              <RefreshCw className={cn(loading && "animate-spin")} />
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

          {page === "nodes" && (
            hubs.length === 0
              ? <Onboarding onAdd={() => setAddOpen(true)} onLocal={openLocal} />
              : (
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
              )
          )}

          {page === "tasks" && <TasksPage />}

          {page === "hubs" && (
            hubs.length === 0
              ? <Onboarding onAdd={() => setAddOpen(true)} onLocal={openLocal} />
              : (
                <HubsPage
                  hubs={hubs}
                  hubsOv={ov?.hubs}
                  onAdd={() => setAddOpen(true)}
                  onRemove={(h) => { setRemoveError(null); setRemoveTarget(h); }}
                  onOpenHub={(h) => { setFilters((f) => ({ ...f, hub: h.id })); setPage("nodes"); }}
                />
              )
          )}

          {page === "machines" && (
            <MachinesPage
              rows={rows}
              now={now}
              onOpen={(r) => setInspect({ hubId: r.hub.id, name: r.st.agent.name })}
            />
          )}

          {page === "local" && (
            <LocalPage refreshKey={localTick} onRefresh={refresh} onViewHub={viewLocalHub} />
          )}
        </div>
      </div>

      <AddHubDialog open={addOpen} onOpenChange={setAddOpen} onAdded={() => void refresh()} />

      <Modal
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        title="设置"
        description="外观会立即生效。"
      >
        <div className="app-field">
          <span className="app-field-label">外观</span>
          <div className="seg seg-fill" role="tablist" aria-label="外观">
            {THEMES.map((t) => (
              <button
                key={t.mode}
                type="button"
                role="tab"
                aria-selected={theme === t.mode}
                className={cn("seg-btn", theme === t.mode && "active")}
                onClick={() => setTheme(t.mode)}
              >
                {t.label}
              </button>
            ))}
          </div>
        </div>
        {info?.version && <div className="app-hint">myco App {info.version}</div>}
      </Modal>

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
