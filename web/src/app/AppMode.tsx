import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, Plus, WifiOff } from "lucide-react";
import "@/app/app.css";
import "@/app/space.css";
import { appApi, errorMessage, type AppInfo, type AppOverview, type Hub } from "@/appapi";
import type { AgentRow } from "@/app/types";
import type { Project } from "@/api";
import { AddMachineDialog } from "@/app/AddMachineDialog";
import { ConnectHubDialog } from "@/app/ConnectHubDialog";
import { NamePrompt } from "@/app/NamePrompt";
import { recall, remember } from "@/lib/memo";
import { useHubNames } from "@/app/hubNames";
import { ProjectPage } from "@/app/ProjectPage";
import { SettingsPage } from "@/app/SettingsPage";
import { UpdateNotice } from "@/app/UpdateNotice";
import { Welcome } from "@/app/Welcome";
import { ProjectSpace, isLoose, looseRowsOf, type SpaceRef } from "@/app/ProjectSpace";
import { AddAgentDialog, AddWorkspaceDialog, NewProjectDialog, NewProjectForm, isNobody } from "@/app/AddToProject";
import { MemberSheet, MembersPage } from "@/app/Members";
import { HubSwitcher } from "@/app/HubSwitcher";
import { ComputersPage } from "@/app/Computers";
import { Avatar, AvatarStack, isWorkspace, rememberMachines, whereOf } from "@/app/persona";
import { agentKind, nodeName } from "@/app/nodes";
import { LogoWordmark } from "@/components/brand";
import { IconSidebar } from "@/components/icons";
import { useContextMenu } from "@/app/ContextMenu";
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

/*
 * The App, as people think of it: projects, the agents in them, and the
 * workspaces they run commands in. A project is where you watch your
 * agents work together and hand out tasks; Members is everyone you work
 * with; Settings holds the rest (team spaces, computers, the app itself).
 */
type Page = "space" | "members" | "computers" | "settings" | "about";

function initialCollapsed() {
  try {
    const v = localStorage.getItem(SIDEBAR_KEY);
    if (v === "open") return false;
    if (v === "closed") return true;
  } catch { /* storage unavailable */ }
  return window.innerWidth < 760;
}

function spaceKey(s: SpaceRef): string {
  return isLoose(s) ? `${s.hubId}\u0000*` : `${s.hubId}\u0000${s.name}`;
}

export default function AppMode({ initialHubs, initialInfo }: {
  initialHubs: Hub[];
  initialInfo: AppInfo | null;
}) {
  const [theme, setTheme] = useThemeMode();
  const [textSize, setTextSize] = useTextSize();
  const [langChoice, setLangChoiceState] = useState<LangChoice>(storedLangChoice());
  const chooseLang = (choice: LangChoice) => {
    setLangChoice(choice);
    setLangChoiceState(choice);
  };
  const hubNames = useHubNames();
  const [info, setInfo] = useState<AppInfo | null>(initialInfo);
  const [collapsed, setCollapsed] = useState(initialCollapsed);
  const [hubs, setHubs] = useState<Hub[]>(initialHubs);
  // What the hubs showed last (also from before a restart) until they answer.
  const [ov, setOv] = useState<AppOverview | null>(() => recall<AppOverview>("overview") ?? null);
  const [error, setError] = useState<unknown>(null);
  const [now, setNow] = useState(Date.now());
  const [localTick, setLocalTick] = useState(0);
  const [page, setPage] = useState<Page>("space");
  // The workspace on show, chosen at the bottom of the sidebar. Workspaces
  // are kept apart: everything shown and done is in this one.
  const [hubPick, setHubPickState] = useState<string>(() => { try { return localStorage.getItem(HUB_KEY) || ""; } catch { return ""; } });
  const setHubPick = (v: string) => {
    setHubPickState(v);
    try { localStorage.setItem(HUB_KEY, v); } catch { /* not kept */ }
  };
  const [space, setSpaceState] = useState<SpaceRef | null>(() => {
    try { const v = localStorage.getItem(SPACE_KEY); return v ? JSON.parse(v) as SpaceRef : null; } catch { return null; }
  });
  const setSpace = (s: SpaceRef | null) => {
    setSpaceState(s);
    try { if (s) localStorage.setItem(SPACE_KEY, JSON.stringify(s)); } catch { /* not kept */ }
  };
  const [projects, setProjects] = useState<{ hubId: string; p: Project }[] | null>(() => recall<{ hubId: string; p: Project }[]>("projects") ?? null);
  const [connectOpen, setConnectOpen] = useState(false);
  const [connectPrefill, setConnectPrefill] = useState("");
  const [addMachineOpen, setAddMachineOpen] = useState(false);
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  const [addAgent, setAddAgent] = useState<SpaceRef | null>(null);
  const [addWorkspace, setAddWorkspace] = useState<SpaceRef | null>(null);
  const [profile, setProfile] = useState<{ hubId: string; name: string } | null>(null);
  // Members open a chat: the space to show it in, and with whom.
  const [chatWith, setChatWith] = useState<{ space: SpaceRef; agent: string; n: number } | null>(null);
  const [projectReload, setProjectReload] = useState(0);
  // The first project was put off: show the agents not in one instead.
  const [skipFirst, setSkipFirst] = useState(() => { try { return localStorage.getItem("myco-skip-first-project") === "1"; } catch { return false; } });

  useEffect(() => {
    let cancelled = false;
    appApi.info().then((i) => { if (!cancelled) setInfo(i); }).catch(() => { /* keep the probe's answer */ });
    return () => { cancelled = true; };
  }, []);

  const refreshing = useRef(false);
  // Projects of the workspaces that answer are fetched again; a workspace that
  // did not answer this time (a network blip) keeps the projects it had, so
  // they do not vanish and the App does not ask for a first project.
  const loadProjects = useCallback(async (okIds: string[], allIds: string[] = okIds) => {
    const lists = await Promise.all(okIds.map((id) =>
      appApi.projects(id).then((r) => r.projects.map((p) => ({ hubId: id, p }))).catch(() => null)));
    setProjects((prev) => {
      const keep = (id: string) => (prev || []).filter((x) => x.hubId === id);
      return remember("projects", allIds.flatMap((id) => { const i = okIds.indexOf(id); return (i >= 0 ? lists[i] : null) ?? keep(id); }));
    });
  }, []);
  const refresh = useCallback(async () => {
    if (refreshing.current) return;
    refreshing.current = true;
    try {
      const [h, o, pending] = await Promise.all([appApi.hubs(), appApi.overview(), readPendingLink()]);
      setHubs(h);
      setOv(remember("overview", keepOffline(o, recall<AppOverview>("overview"))));
      await loadProjects(o.hubs.filter((x) => x.ok).map((x) => x.id), o.hubs.map((x) => x.id));
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

  // A name given in the app wins; the local hub's old default name ("本机
  // Hub") reads as the workspace it now is.
  const nameOf = useCallback((h: { id: string; name: string }) => {
    if (hubNames[h.id]) return hubNames[h.id];
    const m = /^(.*?)\s*(本机 Hub|Local Hub)$/.exec(h.name);
    if (!m) return h.name;
    return m[1] ? L(`${m[1]} 的工作区`, `${m[1]}'s workspace`) : L("我的工作区", "My workspace");
  }, [hubNames]);
  const namedHubs = useMemo(() => hubs.map((h) => ({ ...h, name: nameOf(h) })), [hubs, nameOf]);
  const namedOv = useMemo(() => ov && { ...ov, hubs: ov.hubs.map((h) => ({ ...h, name: nameOf(h) })) }, [ov, nameOf]);
  const allRows = useMemo<AgentRow[]>(() => {
    if (!namedOv) return [];
    rememberMachines(namedOv.hubs.flatMap((h) => h.overview?.machines || []));
    return namedOv.hubs.filter((h) => h.ok && h.overview).flatMap((h) => h.overview!.agents.map((st) => ({ hub: h, st })));
  }, [namedOv]);

  // A workspace removed from the app is not chosen any more: the first one
  // that answers is shown instead.
  const hubId = hubs.some((h) => h.id === hubPick) ? hubPick
    : namedOv?.hubs.find((h) => h.ok && hubs.some((x) => x.id === h.id))?.id || hubs[0]?.id || "";
  const hub = namedHubs.find((h) => h.id === hubId);
  const rows = useMemo(() => allRows.filter((r) => r.hub.id === hubId), [allRows, hubId]);
  const hubProjects = useMemo(() => (projects || []).filter((x) => x.hubId === hubId), [projects, hubId]);
  // A project's computers are for every agent in it: one that joined by
  // itself (myco join --project) is let in here. Each pair is tried once.
  const shared = useRef(new Set<string>());
  useEffect(() => {
    for (const x of projects || []) {
      if (x.p.archived) continue;
      const names = new Set(x.p.members.map((m) => m.name));
      const inIt = allRows.filter((r) => r.hub.id === x.hubId && names.has(r.st.agent.name));
      const agents = inIt.filter((r) => agentKind(r.st.agent) === "agent").map((r) => r.st.agent.name);
      for (const w of inIt.filter((r) => isWorkspace(r.st.agent))) {
        const allow = w.st.agent.allow_from || [];
        if (allow.includes("*")) continue;
        const missing = agents.filter((a) => !allow.includes(a) && !shared.current.has(`${x.hubId}\u0000${w.st.agent.name}\u0000${a}`));
        if (!missing.length) continue;
        missing.forEach((a) => shared.current.add(`${x.hubId}\u0000${w.st.agent.name}\u0000${a}`));
        // A computer added before any agent was locked to nobody: they take its place.
        void appApi.updateAllowFrom(x.hubId, w.st.agent.name, [...allow.filter((n) => !isNobody(n)), ...missing]).catch(() => undefined);
      }
    }
  }, [projects, allRows]);

  const liveProjects = hubProjects.filter((x) => !x.p.archived)
    .sort((a, b) => (b.p.updated_at || "").localeCompare(a.p.updated_at || ""));
  const loose = useMemo(() => looseRowsOf(rows, hubProjects), [rows, hubProjects]);
  const looseHubs = loose.length ? [hubId] : [];
  const hubName = (id: string) => namedHubs.find((h) => h.id === id)?.name || namedOv?.hubs.find((h) => h.id === id)?.name || id;
  // The computers agents use, in the sidebar: one click opens a terminal.
  const computers = rows.filter((r) => isWorkspace(r.st.agent))
    .sort((a, b) => Number(a.st.state === "offline") - Number(b.st.state === "offline") || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub)));
  const people = rows.filter((r) => agentKind(r.st.agent) === "agent")
    .sort((a, b) => Number(a.st.state === "offline") - Number(b.st.state === "offline"));

  // The space on show: the one chosen (while it exists), else the newest
  // project, else the agents in none.
  const spaceExists = (s: SpaceRef) => isLoose(s)
    ? looseHubs.includes(s.hubId)
    : liveProjects.some((x) => x.hubId === s.hubId && x.p.name === s.name);
  const shown: SpaceRef | null = space && spaceExists(space) ? space
    : liveProjects[0] ? { hubId: liveProjects[0].hubId, name: liveProjects[0].p.name }
    : looseHubs[0] ? { hubId: looseHubs[0], loose: true }
    : null;
  const shownProject = shown && !isLoose(shown) ? liveProjects.find((x) => x.hubId === shown.hubId && x.p.name === shown.name)?.p : undefined;
  // Only a workspace that answers can say it has no project yet.
  const firstRun = projects !== null && liveProjects.length === 0 && !skipFirst && !!hub?.ok;

  const meOn = (hubId: string) => {
    const name = namedOv?.hubs.find((h) => h.id === hubId)?.me?.member?.name;
    return name ? `@console:${name}` : undefined;
  };

  // Picking a space (again) shows its group, as a chat app's list does.
  const [spaceTap, setSpaceTap] = useState(0);
  const openSpace = (s: SpaceRef) => {
    setSpace(s);
    setPage("space");
    setSpaceTap((n) => n + 1);
  };
  const projectCreated = (ref: { hubId: string; name: string }) => {
    setNewProjectOpen(false);
    void loadProjects(hubs.map((h) => h.id)).then(() => {
      openSpace(ref);
      // A new project's first step: bring agents in.
      setAddAgent(ref);
    });
  };
  // A member's chat opens in a project it is in (else among the loose).
  const chatWithMember = (r: AgentRow) => {
    const p = liveProjects.find((x) => x.hubId === r.hub.id && x.p.members.some((m) => m.name === r.st.agent.name));
    const s: SpaceRef = p ? { hubId: r.hub.id, name: p.p.name } : { hubId: r.hub.id, loose: true };
    setProfile(null);
    setSpace(s);
    setPage("space");
    setChatWith({ space: s, agent: r.st.agent.name, n: Date.now() });
  };

  const closeConnect = (open: boolean) => {
    setConnectOpen(open);
    if (!open) setConnectPrefill("");
  };
  const hubAdded = async () => { await refresh(); };

  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) return;
      if (e.key === ",") {
        e.preventDefault();
        setPage("settings");
      } else if (e.key === "r" || e.key === "R") {
        e.preventDefault();
        void refreshRef.current();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const startLocal = async () => {
    await appApi.startLocalHub({ tunnel: "none" });
    await refresh();
  };

  const toggleSidebar = () => {
    setCollapsed((c) => {
      const next = !c;
      try { localStorage.setItem(SIDEBAR_KEY, next ? "closed" : "open"); } catch { /* storage unavailable */ }
      return next;
    });
  };
  const dragStyle = { "--wails-draggable": "drag" } as React.CSSProperties;
  const isMac = info?.shell === "mac";
  const navMenu = useContextMenu();

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
        <ConnectHubDialog open={connectOpen} onOpenChange={closeConnect} prefill={connectPrefill} onAdded={() => void hubAdded()} />
      </div>
    );
  }

  const profileRow = profile ? rows.find((r) => r.hub.id === profile.hubId && r.st.agent.name === profile.name) || null : null;
  const addAgentProject = addAgent && !isLoose(addAgent) ? liveProjects.find((x) => x.hubId === addAgent.hubId && x.p.name === addAgent.name)?.p : undefined;
  const addWsProject = addWorkspace && !isLoose(addWorkspace) ? liveProjects.find((x) => x.hubId === addWorkspace.hubId && x.p.name === addWorkspace.name)?.p : undefined;
  const title = page === "members" ? L("成员", "Members")
    : page === "computers" ? L("电脑", "Computers")
    : page === "settings" ? L("设置", "Settings")
    : page === "about" ? L("项目资料", "Project notes")
    // A project says its own name, larger, just below.
    : "";
  const working = (names: string[], hubId: string) => rows.some((r) => r.hub.id === hubId && names.includes(r.st.agent.name) && r.st.state === "working");

  return (
    <div key={lang()} className={cn("app-shell", isMac && "shell-mac", collapsed && "collapsed")}>
      {!collapsed && (
        <aside className="app-sidebar">
          <div className="app-sidebar-head" style={dragStyle}>
            <div className="brand"><LogoWordmark className="app-brand-wordmark" /></div>
          </div>

          <nav className="app-nav">
            <div className="app-nav-group">
              <div className="app-nav-title app-nav-title-row">
                <span>{L("项目", "Projects")}</span>
                <button type="button" className="app-nav-add" aria-label={L("新建项目", "New project")} title={L("新建项目", "New project")}
                  onClick={() => setNewProjectOpen(true)}><Plus /></button>
              </div>
              {liveProjects.map((x) => {
                const ref = { hubId: x.hubId, name: x.p.name };
                const active = page === "space" && !!shown && spaceKey(shown) === spaceKey(ref);
                const busy = working(x.p.members.map((m) => m.name), x.hubId);
                return (
                  <button key={`${x.hubId}/${x.p.name}`} type="button" className={cn("app-nav-item sp-nav", active && "active")}
                    aria-current={active ? "page" : undefined} onClick={() => openSpace(ref)}
                    onContextMenu={(e) => navMenu.open(e, [
                      { label: L("添加 agent", "Add an agent"), onSelect: () => { openSpace(ref); setAddAgent(ref); } },
                      { label: L("加一台电脑", "Add a computer"), onSelect: () => { openSpace(ref); setAddWorkspace(ref); } },
                      { label: L("项目资料", "Project notes"), onSelect: () => { setSpace(ref); setPage("about"); } },
                    ])}
                    title={x.p.goal || x.p.title}>
                    <span className="app-nav-label">{x.p.title || x.p.name}</span>
                    {busy && <span className="sp-nav-busy" title={L("有成员在干活", "Someone is at work")} />}
                  </button>
                );
              })}
              {looseHubs.map((id) => {
                const ref: SpaceRef = { hubId: id, loose: true };
                const active = page === "space" && !!shown && spaceKey(shown) === spaceKey(ref);
                const n = loose.filter((r) => r.hub.id === id).length;
                return (
                  <button key={`${id}/*`} type="button" className={cn("app-nav-item sp-nav is-loose", active && "active")} onClick={() => openSpace(ref)}>
                    <span className="app-nav-label">{L("还没进项目的", "Not in a project")}</span>
                    <span className="sp-nav-count">{n}</span>
                  </button>
                );
              })}
              {projects !== null && liveProjects.length === 0 && (
                <button type="button" className="app-nav-item muted" onClick={() => setNewProjectOpen(true)}>
                  <Plus strokeWidth={1.75} /><span>{L("新建项目", "New project")}</span>
                </button>
              )}
            </div>
            <div className="app-nav-group">
              <div className="app-nav-title app-nav-title-row">
                <button type="button" className={cn("sp-nav-title-link", page === "computers" && "active")} onClick={() => setPage("computers")}
                  title={L("管理电脑", "Manage computers")}>{L("电脑", "Computers")}</button>
                <button type="button" className="app-nav-add" aria-label={L("加一台电脑", "Add a computer")} title={L("加一台电脑", "Add a computer")}
                  onClick={() => (shownProject && shown ? setAddWorkspace(shown) : setPage("computers"))}><Plus /></button>
              </div>
              {computers.slice(0, NAV_COMPUTERS).map((r) => (
                <button key={r.st.agent.name} type="button" className={cn("app-nav-item sp-nav sp-nav-pc", r.st.state === "offline" && "is-away")}
                  onClick={() => chatWithMember(r)} title={L(`打开终端 · ${whereOf(r)}`, `Open the terminal · ${whereOf(r)}`)}>
                  <Avatar agent={r.st.agent} size={20} badge={false} />
                  <span className="app-nav-label">{nodeName(r.st.agent, r.hub)}</span>
                  {r.st.state === "working" && <span className="sp-nav-busy" title={L("正在跑命令", "Running a command")} />}
                </button>
              ))}
              {computers.length > NAV_COMPUTERS && (
                <button type="button" className="app-nav-item sp-nav muted" onClick={() => setPage("computers")}>
                  <span className="app-nav-label">{L(`全部 ${computers.length} 台`, `All ${computers.length}`)}</span>
                </button>
              )}
              {computers.length === 0 && (
                <button type="button" className="app-nav-item sp-nav muted" onClick={() => setPage("computers")}>
                  <span className="app-nav-label">{L("还没有", "None yet")}</span>
                </button>
              )}
            </div>
            <div className="app-nav-group">
              <button type="button" className={cn("app-nav-item sp-nav sp-nav-members", page === "members" && "active")} onClick={() => setPage("members")}>
                <span className="app-nav-label">{L("成员", "Members")}</span>
                {people.length > 0 && <AvatarStack agents={people.map((r) => r.st.agent)} size={18} max={3} />}
              </button>
            </div>
          </nav>

          <div className="app-sidebar-foot">
            <UpdateNotice onOpenSettings={() => setPage("settings")} />
            {/* Which workspace the projects above are in: set once, seldom
                switched, so it sits at the bottom. */}
            <div className="sp-switch">
              <HubSwitcher hubs={namedHubs} hubsOv={namedOv?.hubs} value={hubId}
                onChange={(v) => { setHubPick(v); setPage("space"); setChatWith(null); }}
                onAdd={() => setConnectOpen(true)} onSettings={() => setPage("settings")} />
            </div>
          </div>
        </aside>
      )}

      <div className="app-main">
        <header className="app-crumbbar" style={dragStyle}>
          <button type="button" className="app-btn app-btn-plain" onClick={toggleSidebar}
            aria-label={collapsed ? L("显示边栏", "Show sidebar") : L("隐藏边栏", "Hide sidebar")}
            title={collapsed ? L("显示边栏", "Show sidebar") : L("隐藏边栏", "Hide sidebar")}>
            <IconSidebar />
          </button>
          {page === "about" && (
            <button type="button" className="app-btn app-btn-plain" onClick={() => setPage("space")} aria-label={L("返回项目", "Back to the project")} title={L("返回项目", "Back to the project")}>
              <ArrowLeft />
            </button>
          )}
          <h1 className="app-title">{title}</h1>
        </header>

        {error !== null && (
          <div className="app-banner-wrap">
            <div className="app-error-row">
              <WifiOff />
              <span className="err" title={errorMessage(error)}>{L("暂时拿不到最新的内容，正在重试…", "Can't get the latest right now; retrying…")}</span>
            </div>
          </div>
        )}

        {page === "space" && (firstRun ? (
          // No project yet: start one (or put it off and see the agents).
          <div className="app-scroll">
            <div className="sp-first">
              <h2>{L("先建一个项目吧", "Start with a project")}</h2>
              <p>{L("一个项目就是一件你想做成的事。建好以后，把你的 agent 拉进来，它们就能一起干活，你在这里派活、看进展。", "A project is one thing you want done. Once it's there, bring your agents in: they work on it together while you hand out tasks and watch.")}</p>
              <NewProjectForm big hubs={hub ? [hub] : []} onCreated={projectCreated} onCancel={() => {
                setSkipFirst(true);
                try { localStorage.setItem("myco-skip-first-project", "1"); } catch { /* not kept */ }
              }} />
            </div>
          </div>
        ) : shown ? (
          <ProjectSpace
            key={spaceKey(shown)}
            space={shown}
            project={shownProject}
            looseRows={loose}
            hubName={hubName(shown.hubId)}
            rows={rows}
            now={now}
            me={meOn(shown.hubId)}
            openChat={chatWith && spaceKey(chatWith.space) === spaceKey(shown) ? { agent: chatWith.agent, n: chatWith.n } : undefined}
            home={spaceTap}
            onAddAgent={() => setAddAgent(shown)}
            onAddWorkspace={() => setAddWorkspace(shown)}
            onOpenMember={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name })}
            onAbout={shownProject ? () => setPage("about") : undefined}
            onRefresh={() => void refresh()}
          />
        ) : (
          <div className="app-scroll">
            <div className="sp-first">
              <h2>{L("这里还空着", "Nothing here yet")}</h2>
              <p>{L("建一个项目，把你的 agent 拉进来。", "Start a project and bring your agents in.")}</p>
              <button type="button" className="app-btn app-btn-primary" onClick={() => setNewProjectOpen(true)}><Plus />{L("新建项目", "New project")}</button>
            </div>
          </div>
        ))}

        {page === "members" && (
          <div className="app-scroll">
            <MembersPage rows={rows} projects={hubProjects} now={now} onOpen={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name })} />
          </div>
        )}

        {page === "computers" && (
          <div className="app-scroll">
            <ComputersPage rows={rows} projects={hubProjects}
              onOpen={chatWithMember}
              onProfile={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name })}
              onAddComputer={() => setAddMachineOpen(true)}
              onChanged={() => void refresh()} />
          </div>
        )}

        {page === "about" && shownProject && shown && (
          <div className="app-scroll">
            <ProjectPage
              key={spaceKey(shown)}
              target={{ hubId: shown.hubId, name: shownProject.name }}
              reloadKey={projectReload}
              hubName={hubName(shown.hubId)}
              rows={rows}
              now={now}
              onOpenNode={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name })}
              onNewNode={() => { setPage("space"); setAddAgent(shown); }}
              onChanged={() => void loadProjects(hubs.map((h) => h.id))}
              onOpenConversation={() => setPage("space")}
              onAllConversations={() => setPage("space")}
              onNewConversation={(agent) => { setPage("space"); if (agent) setChatWith({ space: shown, agent, n: Date.now() }); }}
              onDeleted={() => { setSpace(null); setPage("space"); void loadProjects(hubs.map((h) => h.id)); }}
            />
          </div>
        )}

        {page === "settings" && (
          <div className="app-scroll">
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
              onRefresh={refresh}
              onAddHub={() => setConnectOpen(true)}
            />
          </div>
        )}
      </div>

      {navMenu.element}
      <NamePrompt hubsOv={namedOv?.hubs} onRefresh={refresh} />
      <ConnectHubDialog open={connectOpen} onOpenChange={closeConnect} prefill={connectPrefill} onAdded={() => void hubAdded()} />

      <NewProjectDialog open={newProjectOpen} hubs={hub ? [hub] : []} onClose={() => setNewProjectOpen(false)} onCreated={projectCreated} />
      <AddAgentDialog open={!!addAgent} hub={namedHubs.find((h) => h.id === addAgent?.hubId)} project={addAgentProject} projects={hubProjects.map((x) => x.p)} rows={rows}
        onClose={() => setAddAgent(null)} onAdded={() => { setProjectReload((n) => n + 1); void refresh(); }} />
      <AddWorkspaceDialog open={!!addWorkspace} hub={namedHubs.find((h) => h.id === addWorkspace?.hubId)} project={addWsProject} rows={rows}
        onClose={() => setAddWorkspace(null)} onAdded={() => void refresh()} onNewComputer={() => setAddMachineOpen(true)} />
      <AddMachineDialog open={addMachineOpen} onOpenChange={setAddMachineOpen} hubs={hub ? [hub] : []} />

      <MemberSheet row={profileRow} projects={hubProjects} rows={rows} now={now} onClose={() => setProfile(null)}
        onChat={chatWithMember} onChanged={() => void refresh()} onRefresh={refresh}
        onOpenProject={(ref) => { setProfile(null); openSpace(ref); }}
        onOpenMachine={() => { setProfile(null); setPage("computers"); }} />
    </div>
  );
}

const SPACE_KEY = "myco-app-space";
// Computers listed in the sidebar before "All N".
const NAV_COMPUTERS = 6;
const HUB_KEY = "myco-app-hub";
/** A hub that answered 401: it no longer takes this app's token. The hub says
 *  so as "admin token missing or invalid" (or "console token missing or
 *  invalid"), or the proxy answers with the 401 itself. */
function rejectedApp(error?: string): boolean {
  if (!error) return false;
  if (/\b401\b|unauthori[sz]ed|unauthenticated/i.test(error)) return true;
  return /\btoken\b/i.test(error) && /\b(?:missing|invalid|expired|revoked)\b/i.test(error);
}

/**
 * A hub that cannot be reached now but answered before keeps what it showed
 * (marked stale, with when), instead of its nodes disappearing. A hub that
 * rejected this app (401) keeps nothing: what it showed is no longer this
 * person's to see, so it is not shown stale or written back to the cache.
 */
export function keepOffline(fresh: AppOverview, prev: AppOverview | undefined): AppOverview {
  if (!prev) return fresh;
  return {
    ...fresh,
    hubs: fresh.hubs.map((h) => {
      if (h.ok || rejectedApp(h.error)) return h;
      const before = prev.hubs.find((p) => p.id === h.id);
      if (!before?.ok || !before.overview) return h;
      return { ...before, name: h.name, stale: { at: before.stale?.at ?? prev.generated_at, error: h.error } };
    }),
  };
}
