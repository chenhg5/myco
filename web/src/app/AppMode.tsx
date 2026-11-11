import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  FolderKanban, LayoutGrid, MessageSquare, Monitor, PanelLeft, PanelLeftOpen, Plus, RefreshCw, WifiOff,
} from "lucide-react";
import "@/app/app.css";
import { appApi, errorMessage, type AppInfo, type AppOverview, type Hub } from "@/appapi";
import type { AgentRow, Filters } from "@/app/types";
import { AddMachineDialog } from "@/app/AddMachineDialog";
import { AddNodeDialog, type SpawnTarget } from "@/app/AddNodeDialog";
import { ConnectHubDialog } from "@/app/ConnectHubDialog";
import { NamePrompt } from "@/app/NamePrompt";
import { CleanupDialog } from "@/app/CleanupDialog";
import { ConversationDetail, type ConversationTarget } from "@/app/ConversationDetail";
import { ConversationsPage, pairKey, type ConversationRow } from "@/app/ConversationsPage";
import { HubSwitcher } from "@/app/HubSwitcher";
import { useHubNames } from "@/app/hubNames";
import { Inspector } from "@/app/Inspector";
import { MachineInspector, type MachineRef } from "@/app/MachineInspector";
import { MachinesPage } from "@/app/MachinesPage";
import { NewConversationDialog, type NewConversationTarget } from "@/app/NewConversationDialog";
import { NodesPage } from "@/app/NodesPage";
import { NewProjectDialog, ProjectPage, type ProjectRef } from "@/app/ProjectPage";
import type { Project } from "@/api";
import { SettingsPage } from "@/app/SettingsPage";
import { UpdateNotice } from "@/app/UpdateNotice";
import { Welcome } from "@/app/Welcome";
import { LogoSymbol, LogoWordmark } from "@/components/brand";
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

type Page = "conversations" | "nodes" | "machines" | "settings" | "project";

const PAGE_META: Record<Page, { group: string; label: string }> = {
  conversations: { get group() { return L("消息", "Messages"); }, get label() { return L("对话", "Conversations"); } },
  nodes: { get group() { return L("管理", "Manage"); }, get label() { return L("节点", "Nodes"); } },
  machines: { get group() { return L("管理", "Manage"); }, get label() { return L("机器", "Machines"); } },
  settings: { group: "App", get label() { return L("设置", "Settings"); } },
  project: { get group() { return L("项目", "Projects"); }, label: "" },
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
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const [project, setProject] = useState<ProjectRef | null>(null);
  // The conversations page's project filter.
  const [convProject, setConvProject] = useState<ProjectRef | null>(null);
  // Bumped when a node joins a project from here: the page reloads it.
  const [projectReload, setProjectReload] = useState(0);
  const [projects, setProjects] = useState<{ hubId: string; p: Project }[] | null>(null);
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  // A node created from a project page joins that project.
  const joinProject = useRef<ProjectRef | null>(null);
  /** Bumped to make the settings page show its Hub section. */
  const [hubSettingsRequest] = useState(0);

  // The boot probe already carries app/info; retry once in case it failed.
  useEffect(() => {
    let cancelled = false;
    appApi.info().then((i) => { if (!cancelled) setInfo(i); }).catch(() => { /* keep the probe's answer */ });
    return () => { cancelled = true; };
  }, []);

  // One refresh at a time: with a slow hub a refresh can take longer than
  // the poll interval, and piling them up only makes it slower.
  const refreshing = useRef(false);
  // Projects of every reachable hub; a hub too old to have them has none.
  const loadProjects = useCallback(async (hubIds: string[]) => {
    const lists = await Promise.all(hubIds.map((id) =>
      appApi.projects(id).then((r) => r.projects.map((p) => ({ hubId: id, p }))).catch(() => null)));
    setProjects((prev) => {
      // Keep what a hub showed last time when it fails just now.
      const keep = (id: string) => (prev || []).filter((x) => x.hubId === id);
      return hubIds.flatMap((id, i) => lists[i] ?? keep(id));
    });
  }, []);
  const refresh = useCallback(async () => {
    if (refreshing.current) return;
    refreshing.current = true;
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
      void loadProjects(o.hubs.filter((x) => x.ok).map((x) => x.id));
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
      refreshing.current = false;
      setLoading(false);
    }
  }, [loadProjects]);

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

  // From a node's panel, Back returns to that node (on the page it was on).
  const openConversation = (hubId: string, threadId: string, agent: string, extra?: { peer?: string; fromNode?: boolean; fromProject?: boolean }) => {
    const hubName = namedHubs.find((h) => h.id === hubId)?.name
      || namedOv?.hubs.find((h) => h.id === hubId)?.name
      || hubId;
    const returnTo = extra?.fromNode && inspect ? { page, node: inspect }
      : extra?.fromProject && project ? { page: "project", project } : undefined;
    setInspect(null);
    setPage("conversations");
    setConv({ hubId, hubName, threadId, agent, peer: extra?.peer, returnTo });
  };
  const closeConversation = () => {
    const back = conv?.returnTo;
    setConv(null);
    if (back) {
      setPage(back.page as Page);
      if (back.node) setInspect(back.node);
      if (back.project) setProject(back.project);
    }
  };

  // The split view needs room for both columns.
  const wide = useWideWindow(1100);
  // Its list: as wide as the person dragged it, or hidden.
  const [listWidth, setListWidthState] = useState(() => clampWidth(Number(readPref("myco-conv-list-width")) || LIST_WIDTH));
  const [listCollapsed, setListCollapsedState] = useState(() => readPref("myco-conv-list-collapsed") === "1");
  const setListWidth = (w: number) => {
    const v = clampWidth(w);
    setListWidthState(v);
    writePref("myco-conv-list-width", String(v));
  };
  const setListCollapsed = (v: boolean) => {
    setListCollapsedState(v);
    writePref("myco-conv-list-collapsed", v ? "1" : "0");
  };
  const startResize = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const startX = e.clientX;
    const startW = listWidth;
    const el = e.currentTarget;
    el.setPointerCapture(e.pointerId);
    document.body.classList.add("is-resizing");
    const move = (ev: PointerEvent) => setListWidthState(clampWidth(startW + ev.clientX - startX));
    const up = (ev: PointerEvent) => {
      el.releasePointerCapture(ev.pointerId);
      el.removeEventListener("pointermove", move);
      el.removeEventListener("pointerup", up);
      document.body.classList.remove("is-resizing");
      setListWidth(startW + ev.clientX - startX);
    };
    el.addEventListener("pointermove", move);
    el.addEventListener("pointerup", up);
  };

  const openConversationRow = (row: ConversationRow, fromProject?: boolean) => {
    const it = row.item;
    // A shell node's exchange opens in the existing terminal view.
    if (it.kind === "pair") {
      // With you on one side, the node is the one you talk to.
      if (it.other?.startsWith("@console:")) openConversation(row.hubId, "", it.node || "", { peer: it.other, fromProject });
      else openConversation(row.hubId, "", it.other || "", { peer: it.node, fromProject });
      return;
    }
    const agent = it.participants.find((p) => !p.startsWith("@")) || "";
    openConversation(row.hubId, it.thread_id, agent, { fromProject });
  };

  const openProject = (ref: ProjectRef) => {
    setInspect(null);
    setProject(ref);
    setPage("project");
  };
  const nodeAdded = (hubId: string, name: string) => {
    const join = joinProject.current;
    joinProject.current = null;
    if (join && join.hubId === hubId) {
      void appApi.putProjectMember(hubId, join.name, name).catch(() => undefined).finally(() => {
        openProject(join);
        setProjectReload((n) => n + 1);
        void loadProjects(hubs.map((h) => h.id));
        void refresh();
      });
      return;
    }
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

  // Settings live in the hub menu at the bottom of the sidebar, and on
  // ⌘, / Ctrl+, as in other apps.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "," && (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey) {
        e.preventDefault();
        setPage("settings");
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

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
  const sideProjects = (projects || [])
    .filter((x) => filters.hub === "all" || x.hubId === filters.hub)
    .filter((x) => !x.p.archived || (project && project.hubId === x.hubId && project.name === x.p.name));
  const currentProject = project && projects?.find((x) => x.hubId === project.hubId && x.p.name === project.name);
  const meta = page === "project"
    ? { group: PAGE_META.project.group, label: currentProject?.p.title || project?.name || "" }
    : PAGE_META[page];
  const isMac = info?.shell === "mac";

  // macOS: double-clicking the top bar zooms the window (and again restores
  // it), as in a title bar. The window runtime does this itself only when
  // the double-click lands on the bar's own box, not on its text.
  useEffect(() => {
    if (!isMac) return;
    const onDbl = (e: MouseEvent) => {
      const t = e.target as Element | null;
      if (!t?.closest?.(".app-crumbbar, .app-sidebar-head, .app-welcome-drag")) return;
      if (t.closest("button, a, input, textarea, select, [role=button]")) return;
      const invoke = (window as unknown as { _wails?: { invoke?: (m: string) => void } })._wails?.invoke;
      if (!invoke) return;
      e.preventDefault();
      invoke("wails:drag:doubleclick");
    };
    window.addEventListener("dblclick", onDbl);
    return () => window.removeEventListener("dblclick", onDbl);
  }, [isMac]);

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
            <div className="app-nav-group">
              <div className="app-nav-title app-nav-title-row">
                <span>{L("项目", "Projects")}</span>
                <button type="button" className="app-nav-add" aria-label={L("新建项目", "New project")} title={L("新建项目", "New project")}
                  onClick={() => setNewProjectOpen(true)}><Plus /></button>
              </div>
              {sideProjects.map((x) => {
                const active = page === "project" && project?.hubId === x.hubId && project.name === x.p.name;
                return (
                  <button
                    key={`${x.hubId}/${x.p.name}`}
                    type="button"
                    className={cn("app-nav-item", active && "active", x.p.archived && "muted")}
                    aria-current={active ? "page" : undefined}
                    onClick={() => openProject({ hubId: x.hubId, name: x.p.name })}
                    title={x.p.goal || x.p.title}
                  >
                    <FolderKanban strokeWidth={1.75} />
                    <span className="app-nav-label">{x.p.title || x.p.name}</span>
                  </button>
                );
              })}
              {projects !== null && sideProjects.length === 0 && (
                <button type="button" className="app-nav-item muted" onClick={() => setNewProjectOpen(true)}>
                  <Plus strokeWidth={1.75} />
                  <span>{L("新建项目", "New project")}</span>
                </button>
              )}
            </div>
          </nav>

          <div className="app-sidebar-foot">
            <UpdateNotice onOpenSettings={() => setPage("settings")} />
            <HubSwitcher
              hubs={namedHubs}
              hubsOv={namedOv?.hubs}
              value={filters.hub}
              onChange={(hub) => setFilters((f) => ({ ...f, hub }))}
              onAdd={() => setConnectOpen(true)}
              onSettings={() => setPage("settings")}
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
              onSettings={() => setPage("settings")}
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

        {page === "conversations" && wide ? (
          // Wide windows: the list on the left, the open conversation on the
          // right; another row switches it, like any messaging app.
          <div className={cn("conv-split", listCollapsed && "is-collapsed")}>
            {listCollapsed ? (
              <div className="conv-split-rail">
                <button type="button" className="app-btn app-btn-plain" onClick={() => setListCollapsed(false)}
                  aria-label={L("展开对话列表", "Show the conversation list")} title={L("展开对话列表", "Show the conversation list")}>
                  <PanelLeftOpen />
                </button>
              </div>
            ) : (<>
            <div className="conv-split-list app-scroll" style={{ width: listWidth }}>
              <ConversationsPage
                hubs={namedHubs}
                hub={filters.hub}
                rows={rows}
                now={now}
                compact
                onCollapse={() => setListCollapsed(true)}
                selectedKey={conv ? convKey(conv) : ""}
                onOpen={(r) => openConversationRow(r)}
                onNew={() => setNewConv(convProject ? { hubId: convProject.hubId, project: convProject.name } : { hubId: filters.hub === "all" ? undefined : filters.hub })}
                projects={projects}
                project={convProject}
                onProject={setConvProject}
              />
            </div>
            <div
              className="conv-split-handle"
              role="separator"
              aria-orientation="vertical"
              aria-label={L("拖动调整列表宽度，双击恢复", "Drag to resize the list; double-click to reset")}
              title={L("拖动调整宽度，双击恢复", "Drag to resize; double-click to reset")}
              onPointerDown={startResize}
              onDoubleClick={() => setListWidth(LIST_WIDTH)}
            />
            </>)}
            <div className="conv-split-detail">
              {conv ? (
                <ConversationDetail
                  key={convKey(conv)}
                  target={conv}
                  rows={rows}
                  now={now}
                  showBack={false}
                  onBack={closeConversation}
                  onNewConversation={(hubId, agent) => setNewConv({ hubId, agent })}
                />
              ) : (
                <div className="conv-split-empty">
                  <LogoSymbol flat className="conv-split-mark" />
                  <p>{listCollapsed ? L("展开左边的列表，选一个对话", "Show the list on the left and pick a conversation") : L("在左边选一个对话", "Pick a conversation on the left")}</p>
                </div>
              )}
            </div>
          </div>
        ) : page === "conversations" && conv ? (
          <ConversationDetail
            target={conv}
            rows={rows}
            now={now}
            onBack={closeConversation}
            onNewConversation={(hubId, agent) => setNewConv({ hubId, agent })}
          />
        ) : (
          <div className="app-scroll">
            {error !== null && (
              <div className="app-banner-wrap">
                <div className="app-error-row">
                  <WifiOff />
                  <span className="err" title={errorMessage(error)}>{L("App 暂时拿不到最新数据，正在重试…", "The app can't get fresh data right now; retrying…")}</span>
                </div>
              </div>
            )}

            {page === "conversations" && (
              <ConversationsPage
                hubs={namedHubs}
                hub={filters.hub}
                rows={rows}
                now={now}
                onOpen={(r) => openConversationRow(r)}
                onNew={() => setNewConv(convProject ? { hubId: convProject.hubId, project: convProject.name } : { hubId: filters.hub === "all" ? undefined : filters.hub })}
                projects={projects}
                project={convProject}
                onProject={setConvProject}
              />
            )}

            {page === "project" && project && (
              <ProjectPage
                key={`${project.hubId}/${project.name}`}
                target={project}
                reloadKey={projectReload}
                hubName={namedHubs.find((h) => h.id === project.hubId)?.name || project.hubId}
                rows={rows}
                now={now}
                onOpenNode={(r) => setInspect({ hubId: r.hub.id, name: r.st.agent.name })}
                onNewNode={() => {
                  joinProject.current = project;
                  setSpawnTarget({ hubId: project.hubId, project: { name: project.name, title: currentProject?.p.title || project.name } });
                }}
                onChanged={() => void loadProjects(hubs.map((h) => h.id))}
                onOpenConversation={(r) => openConversationRow(r, true)}
                onAllConversations={() => { setConvProject(project); setPage("conversations"); }}
                onNewConversation={(agent) => setNewConv({ hubId: project.hubId, agent, project: project.name })}
                onDeleted={() => {
                  setProject(null);
                  setConvProject(null);
                  setPage("nodes");
                  void loadProjects(hubs.map((h) => h.id));
                }}
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
                onCleanup={() => setCleanupOpen(true)}
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
                hubsOv={namedOv?.hubs}
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

      <NamePrompt hubsOv={namedOv?.hubs} onRefresh={refresh} />
      <CleanupDialog
        open={cleanupOpen}
        onOpenChange={setCleanupOpen}
        hubs={namedOv?.hubs || []}
        defaultHub={filters.hub === "all" ? undefined : filters.hub}
        onDone={() => void refresh()}
      />
      <ConnectHubDialog
        open={connectOpen}
        onOpenChange={closeConnect}
        prefill={connectPrefill}
        onAdded={(hub) => void hubAdded(hub)}
      />

      <Inspector
        row={current}
        now={now}
        projects={projects}
        onOpenProject={openProject}
        onProjectsChanged={() => void loadProjects(hubs.map((h) => h.id))}
        onClose={() => setInspect(null)}
        onRefresh={refresh}
        onThread={(hubId, threadId, agent) => openConversation(hubId, threadId, agent, { fromNode: true })}
        onPeer={(hubId, agent, peer) => openConversation(hubId, "", agent, { peer, fromNode: true })}
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
        onClose={() => { setSpawnTarget(null); joinProject.current = null; }}
        onSpawned={nodeAdded}
        onMessage={(hubId, name) => {
          const join = spawnTarget?.project;
          setSpawnTarget(null);
          setNewConv({ hubId, agent: name, ...(join ? { project: join.name } : {}) });
        }}
        onJoined={nodeAdded}
      />

      <NewConversationDialog
        target={newConv}
        hubs={namedHubs}
        rows={rows}
        projects={projects}
        onClose={() => setNewConv(null)}
        onSent={(hubId, threadId, agent, from) => {
          setNewConv(null);
          // The whole conversation with that node, this task at the end.
          openConversation(hubId, threadId, agent, from ? { peer: from } : undefined);
          void refresh();
        }}
      />

      <NewProjectDialog
        open={newProjectOpen}
        hubs={namedHubs.filter((h) => namedOv?.hubs.find((o) => o.id === h.id)?.ok !== false)}
        defaultHubId={filters.hub === "all" ? undefined : filters.hub}
        onClose={() => setNewProjectOpen(false)}
        onCreated={(ref) => {
          setNewProjectOpen(false);
          openProject(ref);
          void loadProjects(hubs.map((h) => h.id));
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

/** The list row key of an open conversation (see rowKey). */
function convKey(c: { hubId: string; threadId: string; agent: string; peer?: string }): string {
  return c.peer ? `${c.hubId}\u0000${pairKey(c.agent, c.peer)}` : `${c.hubId}\u0000${c.threadId}`;
}

/** Whether the window is at least min pixels wide (follows resizing). */
function useWideWindow(min: number): boolean {
  const query = `(min-width: ${min}px)`;
  const [wide, setWide] = useState(() => window.matchMedia(query).matches);
  useEffect(() => {
    const mq = window.matchMedia(query);
    const on = () => setWide(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, [query]);
  return wide;
}

const LIST_WIDTH = 380;
function clampWidth(w: number): number {
  return Math.min(600, Math.max(280, Math.round(w)));
}
function readPref(key: string): string | null {
  try { return localStorage.getItem(key); } catch { return null; }
}
function writePref(key: string, value: string) {
  try { localStorage.setItem(key, value); } catch { /* not kept */ }
}
