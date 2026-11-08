import { useCallback, useEffect, useMemo, useState } from "react";
import {
  LayoutGrid, Monitor, PanelLeft, RefreshCw, Settings, WifiOff,
} from "lucide-react";
import "@/app/app.css";
import { appApi, errorMessage, type AppInfo, type AppOverview, type Hub } from "@/appapi";
import type { Message } from "@/api";
import type { AgentRow, Filters } from "@/app/types";
import { AddMachineDialog } from "@/app/AddMachineDialog";
import { AddNodeDialog, type SpawnTarget } from "@/app/AddNodeDialog";
import { ConnectHubDialog } from "@/app/ConnectHubDialog";
import { ErrorText, Modal } from "@/app/dialogs";
import { HubSwitcher } from "@/app/HubSwitcher";
import { useHubNames } from "@/app/hubNames";
import { Inspector } from "@/app/Inspector";
import { MachineInspector, type MachineRef } from "@/app/MachineInspector";
import { MachinesPage } from "@/app/MachinesPage";
import { NodesPage } from "@/app/NodesPage";
import { SettingsPage } from "@/app/SettingsPage";
import { Welcome } from "@/app/Welcome";
import { useThemeMode } from "@/lib/theme";
import { cn, timeAgo } from "@/lib/utils";

const POLL_MS = 5000;
const SIDEBAR_KEY = "myco-app-sidebar";

// One request at a time: React StrictMode mounts the window twice in dev and
// the backend hands a pending link out only once.
let pendingInFlight: Promise<string> | null = null;
function readPendingLink(): Promise<string> {
  if (pendingInFlight) return pendingInFlight;
  const p = appApi.pendingLink()
    .then((r) => r?.link || "")
    .catch(() => "")
    .finally(() => { if (pendingInFlight === p) pendingInFlight = null; });
  pendingInFlight = p;
  return p;
}

type Page = "nodes" | "machines" | "settings";

const PAGE_META: Record<Page, { group: string; label: string }> = {
  nodes: { group: "工作", label: "节点" },
  machines: { group: "工作", label: "机器" },
  settings: { group: "App", label: "设置" },
};

const NAV: { group: string; items: { page: Page; label: string; icon: typeof LayoutGrid }[] }[] = [
  {
    group: "工作",
    items: [
      { page: "nodes", label: "节点", icon: LayoutGrid },
      { page: "machines", label: "机器", icon: Monitor },
    ],
  },
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

export default function AppMode({ initialHubs, initialInfo }: {
  initialHubs: Hub[];
  initialInfo: AppInfo | null;
}) {
  const [theme, setTheme] = useThemeMode();
  const hubNames = useHubNames();
  const [info, setInfo] = useState<AppInfo | null>(initialInfo);
  const [collapsed, setCollapsed] = useState(initialCollapsed);
  const [hubs, setHubs] = useState<Hub[]>(initialHubs);
  const [ov, setOv] = useState<AppOverview | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const [now, setNow] = useState(Date.now());
  const [localTick, setLocalTick] = useState(0);
  const [page, setPage] = useState<Page>("nodes");
  const [filters, setFilters] = useState<Filters>({ query: "", hub: "all", machine: "all", state: "all", kind: "all" });
  const [inspect, setInspect] = useState<{ hubId: string; name: string } | null>(null);
  const [machineInspect, setMachineInspect] = useState<MachineRef | null>(null);
  const [spawnTarget, setSpawnTarget] = useState<SpawnTarget | null>(null);
  const [addMachineOpen, setAddMachineOpen] = useState(false);
  const [connectOpen, setConnectOpen] = useState(false);
  const [connectPrefill, setConnectPrefill] = useState("");
  const [highlight, setHighlight] = useState<{ hubId: string; name: string } | null>(null);
  const [thread, setThread] = useState<ThreadState | null>(null);

  // The boot probe already carries app/info; retry once in case it failed.
  useEffect(() => {
    let cancelled = false;
    appApi.info().then((i) => { if (!cancelled) setInfo(i); }).catch(() => { /* keep the probe's answer */ });
    return () => { cancelled = true; };
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [h, o, pending] = await Promise.all([
        appApi.hubs(),
        appApi.overview(),
        // The backend may not know this endpoint yet; never break polling.
        readPendingLink(),
      ]);
      setHubs(h);
      setOv(o);
      setError(null);
      setNow(Date.now());
      setLocalTick((t) => t + 1);
      if (pending) {
        setConnectPrefill(pending);
        setConnectOpen(true);
      }
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
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

  // A removed hub must not stay selected.
  useEffect(() => {
    if (filters.hub !== "all" && !hubs.some((h) => h.id === filters.hub)) {
      setFilters((f) => ({ ...f, hub: "all" }));
    }
  }, [hubs, filters.hub]);

  useEffect(() => {
    if (page !== "machines") setMachineInspect(null);
  }, [page]);

  useEffect(() => {
    if (!highlight) return;
    const t = window.setTimeout(() => setHighlight(null), 15_000);
    return () => window.clearTimeout(t);
  }, [highlight]);

  // Renames live only in this App; fold them in before anything renders a name.
  const namedHubs = useMemo(
    () => hubs.map((h) => (hubNames[h.id] ? { ...h, name: hubNames[h.id] } : h)),
    [hubs, hubNames],
  );
  const namedOv = useMemo(
    () => ov && {
      ...ov,
      hubs: ov.hubs.map((h) => (hubNames[h.id] ? { ...h, name: hubNames[h.id] } : h)),
    },
    [ov, hubNames],
  );

  const rows = useMemo<AgentRow[]>(() => {
    if (!namedOv) return [];
    return namedOv.hubs
      .filter((h) => h.ok && h.overview)
      .flatMap((h) => h.overview!.agents.map((st) => ({ hub: h, st })));
  }, [namedOv]);

  const current = useMemo(() => {
    if (!inspect) return null;
    return rows.find((r) => r.hub.id === inspect.hubId && r.st.agent.name === inspect.name) || null;
  }, [rows, inspect]);

  const visibleHubs = useMemo(
    () => filters.hub === "all" ? namedHubs : namedHubs.filter((h) => h.id === filters.hub),
    [namedHubs, filters.hub],
  );

  const openThread = async (hubId: string, threadId: string) => {
    const hubName = namedOv?.hubs.find((h) => h.id === hubId)?.name || hubId;
    setThread({ hubId, hubName, id: threadId, messages: null });
    try {
      const r = await appApi.thread(hubId, threadId);
      setThread({ hubId, hubName, id: threadId, messages: r.messages });
    } catch (e) {
      setThread({ hubId, hubName, id: threadId, messages: [], error: errorMessage(e) });
    }
  };

  const closeConnect = (open: boolean) => {
    setConnectOpen(open);
    if (!open) setConnectPrefill("");
  };

  const hubAdded = async (hub: Hub) => {
    await refresh();
    setFilters((f) => ({ ...f, hub: hub.id }));
    setPage("nodes");
  };

  const startLocal = async () => {
    const r = await appApi.startLocalHub({ tunnel: "none" });
    await refresh();
    setFilters((f) => ({ ...f, hub: r.hub.id }));
    setPage("nodes");
  };

  // Keep explicit sidebar toggles; an untouched window uses the 860px default.
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

  if (hubs.length === 0) {
    return (
      <div className={cn("app-shell welcome-shell", isMac && "shell-mac")}>
        <Welcome onStartLocal={startLocal} onConnect={() => setConnectOpen(true)} />
        <ConnectHubDialog
          open={connectOpen}
          onOpenChange={closeConnect}
          prefill={connectPrefill}
          onAdded={(hub) => void hubAdded(hub)}
        />
      </div>
    );
  }

  return (
    <div className={cn("app-shell", isMac && "shell-mac", collapsed && "collapsed")}>
      {!collapsed && (
        <aside className="app-sidebar">
          <div className="app-sidebar-head" style={dragStyle}>
            <HubSwitcher
              hubs={namedHubs}
              value={filters.hub}
              onChange={(hub) => setFilters((f) => ({ ...f, hub }))}
              onAdd={() => setConnectOpen(true)}
            />
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
              className={cn("app-nav-item", page === "settings" && "active")}
              onClick={() => setPage("settings")}
            >
              <Settings strokeWidth={1.75} />
              <span>设置</span>
            </button>
            <div
              className="app-sidebar-status"
              title={namedHubs.map((h) => `${h.name}：${h.ok ? "已连接" : "连不上"}`).join("\n")}
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
          {collapsed && (
            <HubSwitcher
              hubs={namedHubs}
              value={filters.hub}
              onChange={(hub) => setFilters((f) => ({ ...f, hub }))}
              onAdd={() => setConnectOpen(true)}
            />
          )}
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
            <NodesPage
              rows={rows}
              hubs={namedOv?.hubs}
              hubMeta={hubs}
              loading={loading}
              filters={filters}
              onFilters={setFilters}
              highlight={highlight}
              onOpen={(r) => setInspect({ hubId: r.hub.id, name: r.st.agent.name })}
              onAdd={() => setSpawnTarget({})}
            />
          )}

          {page === "machines" && (
            <MachinesPage
              hubs={visibleHubs}
              hubsOv={namedOv?.hubs}
              loading={loading}
              groupByHub={filters.hub === "all"}
              onOpenMachine={(hubId, machineId) => setMachineInspect({ hubId, machineId })}
              onAdd={() => setAddMachineOpen(true)}
            />
          )}

          {page === "settings" && (
            <SettingsPage
              hubs={namedHubs}
              info={info}
              theme={theme}
              onTheme={setTheme}
              refreshKey={localTick}
              onRefresh={refresh}
              onAddHub={() => setConnectOpen(true)}
            />
          )}
        </div>
      </div>

      <ConnectHubDialog
        open={connectOpen}
        onOpenChange={closeConnect}
        prefill={connectPrefill}
        onAdded={(hub) => void hubAdded(hub)}
      />

      <Inspector
        row={current}
        now={now}
        onClose={() => setInspect(null)}
        onRefresh={refresh}
        onThread={(hubId, threadId) => void openThread(hubId, threadId)}
        onOpenMachine={(hubId, machineId) => {
          setInspect(null);
          setPage("machines");
          setMachineInspect({ hubId, machineId });
        }}
      />

      <MachineInspector
        target={machineInspect}
        hubs={namedOv?.hubs}
        rows={rows}
        now={now}
        onClose={() => setMachineInspect(null)}
        onRefresh={refresh}
        onOpenNode={(r) => {
          setMachineInspect(null);
          setInspect({ hubId: r.hub.id, name: r.st.agent.name });
        }}
        onSpawn={(hubId, machineId) => setSpawnTarget({ hubId, machineId })}
      />

      <AddNodeDialog
        target={spawnTarget}
        hubs={namedHubs}
        hubsOv={namedOv?.hubs}
        onClose={() => setSpawnTarget(null)}
        onSpawned={(hubId, agent) => {
          setHighlight({ hubId, name: agent.name });
          setMachineInspect(null);
          setPage("nodes");
          void refresh();
        }}
      />

      <AddMachineDialog
        open={addMachineOpen}
        onOpenChange={setAddMachineOpen}
        hubs={namedHubs}
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
