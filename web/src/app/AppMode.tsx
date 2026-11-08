import { useCallback, useEffect, useMemo, useState } from "react";
import {
  LayoutGrid, MessageSquare, Monitor, PanelLeft, RefreshCw, Settings, WifiOff,
} from "lucide-react";
import "@/app/app.css";
import { appApi, errorMessage, type AppInfo, type AppOverview, type Hub } from "@/appapi";
import type { AgentRow, Filters } from "@/app/types";
import { AddMachineDialog } from "@/app/AddMachineDialog";
import { AddNodeDialog, type SpawnTarget } from "@/app/AddNodeDialog";
import { ConnectHubDialog } from "@/app/ConnectHubDialog";
import { ConversationDetail, type ConversationTarget } from "@/app/ConversationDetail";
import { ConversationsPage, type ConversationRow } from "@/app/ConversationsPage";
import { HubSwitcher } from "@/app/HubSwitcher";
import { useHubNames } from "@/app/hubNames";
import { Inspector } from "@/app/Inspector";
import { MachineInspector, type MachineRef } from "@/app/MachineInspector";
import { MachinesPage } from "@/app/MachinesPage";
import { NewConversationDialog, type NewConversationTarget } from "@/app/NewConversationDialog";
import { NodesPage } from "@/app/NodesPage";
import { SettingsPage } from "@/app/SettingsPage";
import { Welcome } from "@/app/Welcome";
import { LogoWordmark } from "@/components/brand";
import { useTextSize, useThemeMode } from "@/lib/theme";
import { L, lang, setLangChoice, storedLangChoice, type LangChoice } from "@/lib/i18n";
import { cn } from "@/lib/utils";

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

type Page = "conversations" | "nodes" | "machines" | "settings";

const PAGE_META: Record<Page, { group: string; label: string }> = {
  conversations: { get group() { return L("消息", "Messages"); }, get label() { return L("对话", "Conversations"); } },
  nodes: { get group() { return L("管理", "Manage"); }, get label() { return L("节点", "Nodes"); } },
  machines: { get group() { return L("管理", "Manage"); }, get label() { return L("机器", "Machines"); } },
  settings: { group: "App", get label() { return L("设置", "Settings"); } },
};

const NAV: { group: string; items: { page: Page; label: string; icon: typeof LayoutGrid }[] }[] = [
  {
    get group() { return L("管理", "Manage"); },
    items: [
      { page: "nodes", get label() { return L("节点", "Nodes"); }, icon: LayoutGrid },
      { page: "machines", get label() { return L("机器", "Machines"); }, icon: Monitor },
    ],
  },
  {
    get group() { return L("消息", "Messages"); },
    items: [
      { page: "conversations", get label() { return L("对话", "Conversations"); }, icon: MessageSquare },
    ],
  },
];

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
  const [textSize, setTextSize] = useTextSize();
  const [langChoice, setLangChoiceState] = useState<LangChoice>(storedLangChoice());
  // Labels come from L() at render time; the shells below key on lang() so
  // the whole tree re-renders in the new language while this state stays.
  const chooseLang = (choice: LangChoice) => {
    setLangChoice(choice);
    setLangChoiceState(choice);
  };
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
  const [filters, setFilters] = useState<Filters>({ query: "", hub: "all", machine: "all", state: "all", kind: "all", label: "all" });
  const [inspect, setInspect] = useState<{ hubId: string; name: string } | null>(null);
  const [machineInspect, setMachineInspect] = useState<MachineRef | null>(null);
  const [spawnTarget, setSpawnTarget] = useState<SpawnTarget | null>(null);
  const [addMachineOpen, setAddMachineOpen] = useState(false);
  const [connectOpen, setConnectOpen] = useState(false);
  const [connectPrefill, setConnectPrefill] = useState("");
  const [highlight, setHighlight] = useState<{ hubId: string; name: string } | null>(null);
  const [conv, setConv] = useState<ConversationTarget | null>(null);
  const [newConv, setNewConv] = useState<NewConversationTarget | null>(null);
  /** Bumped to make the settings page show its Hub section. */
  const [hubSettingsRequest, setHubSettingsRequest] = useState(0);

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

  // Leaving the conversations page returns to the list next time.
  useEffect(() => {
    if (page !== "conversations") setConv(null);
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

  const openConversation = (hubId: string, threadId: string, agent: string) => {
    const hubName = namedHubs.find((h) => h.id === hubId)?.name
      || namedOv?.hubs.find((h) => h.id === hubId)?.name
      || hubId;
    setInspect(null);
    setPage("conversations");
    setConv({ hubId, hubName, threadId, agent });
  };

  const openConversationRow = (row: ConversationRow) => {
    openConversation(row.hubId, row.conv.thread_id, row.conv.agent || "");
  };

  const nodeAdded = (hubId: string, name: string) => {
    setHighlight({ hubId, name });
    setMachineInspect(null);
    setPage("nodes");
    void refresh();
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

  const openHubSettings = () => {
    setHubSettingsRequest((n) => n + 1);
    setPage("settings");
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
  const isMac = info?.shell === "mac";

  if (hubs.length === 0) {
    return (
      <div key={lang()} className={cn("app-shell welcome-shell", isMac && "shell-mac")}>
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
    <div key={lang()} className={cn("app-shell", isMac && "shell-mac", collapsed && "collapsed")}>
      {!collapsed && (
        <aside className="app-sidebar">
          <div className="app-sidebar-head" style={dragStyle}>
            <div className="brand">
              <LogoWordmark className="app-brand-wordmark" />
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
              className={cn("app-nav-item", page === "settings" && "active")}
              onClick={() => setPage("settings")}
            >
              <Settings strokeWidth={1.75} />
              <span>{L("设置", "Settings")}</span>
            </button>
            <HubSwitcher
              hubs={namedHubs}
              hubsOv={namedOv?.hubs}
              value={filters.hub}
              onChange={(hub) => setFilters((f) => ({ ...f, hub }))}
              onAdd={() => setConnectOpen(true)}
              onSettings={openHubSettings}
            />
          </div>
        </aside>
      )}

      <div className="app-main">
        <header className="app-crumbbar" style={dragStyle}>
          <button
            type="button"
            className="app-btn app-btn-plain"
            onClick={toggleSidebar}
            aria-label={collapsed ? L("显示边栏", "Show sidebar") : L("隐藏边栏", "Hide sidebar")}
            title={collapsed ? L("显示边栏", "Show sidebar") : L("隐藏边栏", "Hide sidebar")}
          >
            <PanelLeft />
          </button>
          {collapsed && (
            <HubSwitcher
              variant="pill"
              hubs={namedHubs}
              hubsOv={namedOv?.hubs}
              value={filters.hub}
              onChange={(hub) => setFilters((f) => ({ ...f, hub }))}
              onAdd={() => setConnectOpen(true)}
              onSettings={openHubSettings}
            />
          )}
          <nav className="app-crumbs" aria-label={L("位置", "Location")}>
            <span>{meta.group}</span>
            <span className="sep">/</span>
            <span className="cur">{meta.label}</span>
          </nav>
          <div className="app-crumb-actions">
            <button type="button" className="app-btn app-btn-plain" onClick={() => void refresh()} aria-label={L("刷新", "Refresh")} title={L("刷新", "Refresh")}>
              <RefreshCw className={cn(loading && "animate-spin")} />
            </button>
          </div>
        </header>

        {page === "conversations" && conv ? (
          <ConversationDetail
            target={conv}
            rows={rows}
            now={now}
            onBack={() => setConv(null)}
            onNewConversation={(hubId, agent) => setNewConv({ hubId, agent })}
          />
        ) : (
          <div className="app-scroll">
            {error !== null && (
              <div className="app-banner-wrap">
                <div className="app-error-row">
                  <WifiOff />
                  <span className="err" title={errorMessage(error)}>{L(`无法获取 App 数据：${errorMessage(error)}`, `Couldn’t load app data: ${errorMessage(error)}`)}</span>
                </div>
              </div>
            )}

            {page === "conversations" && (
              <ConversationsPage
                hubs={namedHubs}
                hub={filters.hub}
                rows={rows}
                now={now}
                onOpen={openConversationRow}
                onNew={() => setNewConv({ hubId: filters.hub === "all" ? undefined : filters.hub })}
              />
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
                textSize={textSize}
                onTextSize={setTextSize}
                langChoice={langChoice}
                onLangChoice={chooseLang}
                refreshKey={localTick}
                sectionRequest={hubSettingsRequest}
                onRefresh={refresh}
                onAddHub={() => setConnectOpen(true)}
              />
            )}
          </div>
        )}
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
        onThread={(hubId, threadId, agent) => openConversation(hubId, threadId, agent)}
        onMessage={(r) => {
          setInspect(null);
          setNewConv({ hubId: r.hub.id, agent: r.st.agent.name });
        }}
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
        defaultHubId={filters.hub === "all" ? undefined : filters.hub}
        onClose={() => setSpawnTarget(null)}
        onSpawned={(hubId, agent) => nodeAdded(hubId, agent.name)}
        onJoined={nodeAdded}
      />

      <NewConversationDialog
        target={newConv}
        hubs={namedHubs}
        rows={rows}
        onClose={() => setNewConv(null)}
        onSent={(hubId, threadId, agent) => {
          setNewConv(null);
          openConversation(hubId, threadId, agent);
          void refresh();
        }}
      />

      <AddMachineDialog
        open={addMachineOpen}
        onOpenChange={setAddMachineOpen}
        hubs={namedHubs}
      />
    </div>
  );
}
