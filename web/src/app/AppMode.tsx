import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowUpDown, ChevronLeft, ChevronRight, LayoutGrid, Monitor, MoreHorizontal, Pin, Plus, Search, WifiOff, Workflow, X, Zap } from "lucide-react";
import { ComputerSettings } from "@/app/ComputerSettings";
import { AutomationsPage } from "@/app/Automations";
import { WorkflowsPage } from "@/app/Workflows";
import { TemplatesPage } from "@/app/Templates";
import "@/app/app.css";
import "@/app/space.css";
import { appApi, errorMessage, type AppInfo, type AppOverview, type Hub, type LocalState, type WorkspaceState } from "@/appapi";
import type { AgentRow } from "@/app/types";
import type { Project } from "@/api";
import { ConnectHubDialog } from "@/app/ConnectHubDialog";
import { NamePrompt } from "@/app/NamePrompt";
import { recall, remember } from "@/lib/memo";
import { renameHub, useHubNames } from "@/app/hubNames";
import { ProjectPage } from "@/app/ProjectPage";
import { openSettingsSection, SettingsPage } from "@/app/SettingsPage";
import { navBack, navForward, navRecord, navSub, onNavApply, useNavState, type NavPlace } from "@/app/nav";
import { UpdateNotice } from "@/app/UpdateNotice";
import { Welcome } from "@/app/Welcome";
import { Prepare, preparePending, setPreparePending } from "@/app/Prepare";
import { Checklist, startOnboarding, type ChecklistStep } from "@/app/Onboard";
import { ProjectSpace, isLoose, looseRowsOf, type SpaceRef } from "@/app/ProjectSpace";
import { dragAgent } from "@/app/Panes";
import { AddAgentDialog, AddWorkspaceDialog, NewProjectDialog, isNobody } from "@/app/AddToProject";
import { MemberSheet, MembersPage } from "@/app/Members";
import { HubSwitcher } from "@/app/HubSwitcher";
import { useSimpleMode } from "@/app/simpleMode";
import { isLocalUrl, localWorkspaces } from "@/app/localHubs";
import { NewWorkspaceDialog } from "@/app/NewWorkspaceDialog";
import { ComputersPage } from "@/app/Computers";
import { Avatar, isWorkspace, rememberAgentHubs, rememberMachines, whereOf } from "@/app/persona";
import { agentKind, isMe, machinesOfHub, nodeName, stuckCounts } from "@/app/nodes";
import { prefetchMachineInfos } from "@/app/machineInfoCache";
import { LogoWordmark } from "@/components/brand";
import { IconSidebar } from "@/components/icons";
import { recentChats } from "@/app/recentChats";
import { useContextMenu } from "@/app/ContextMenu";
import { useSortable } from "@/app/sortable";
import { useTextSize, useThemeMode } from "@/lib/theme";
import { L, lang, setLangChoice, storedLangChoice, type LangChoice } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { setSystemDictation } from "@/app/Dictate";
import { setSystemPicker } from "@/app/AvatarPicker";
import { handleLinkClick, setOpenLinksViaApp } from "@/app/links";
import { CommandPalette } from "@/app/CommandPalette";
import { PageHost, PageToggle, setPageHubs, setPageMe, setPageNames } from "@/app/pages";
import { VoiceChip } from "@/app/VoiceBar";
import { modKey } from "@/app/ConversationDetail";
import { useSidebarWidth } from "@/app/sidebarWidth";
import { HandoverDialog, type HandoverTarget } from "@/app/Handover";
import { ForkDialog, forkItem, type ForkTarget } from "@/app/Fork";
import { SuperagentSetup, useSuperagent } from "@/app/Superagent";

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
type Page = "space" | "members" | "computers" | "automations" | "workflows" | "templates" | "settings" | "about";

function initialCollapsed() {
  try {
    const v = localStorage.getItem(SIDEBAR_KEY);
    if (v === "open") return false;
    if (v === "closed") return true;
  } catch { /* storage unavailable */ }
  return window.innerWidth < 760;
}

/** Settings › 快捷键: asked for while Settings shows, it moves there;
 *  otherwise Settings opens on it. */
function openShortcuts(settingsShown: boolean) {
  if (settingsShown) window.dispatchEvent(new CustomEvent("myco-open-settings", { detail: { section: "shortcuts" } }));
  else openSettingsSection("shortcuts");
}

/** ⌥↑ / ⌥↓: open the project or chat above or below the one open, in the
 *  sidebar's order (from the top or the bottom when none is). False when
 *  there is nowhere to go (the sidebar hidden, the end of the list). The
 *  order is kept while you keep stepping: a chat opened can move up its
 *  list (most recent first), which must not send the next step back. */
let stepRun: { keys: string[]; at: string; t: number } | null = null;
function stepConversation(by: -1 | 1): boolean {
  const items = Array.from(document.querySelectorAll<HTMLElement>(".app-sidebar .app-nav [data-nav-key]"));
  if (!items.length) return false;
  const live = items.map((el) => el.dataset.navKey || "");
  const now = Date.now();
  let keys = live;
  // (a chat open inside a project marks both: the chat is where you are)
  let at = items.map((el) => el.classList.contains("active")).lastIndexOf(true);
  if (stepRun && now - stepRun.t < 4000 && live.includes(stepRun.at)) {
    keys = stepRun.keys.filter((k) => live.includes(k));
    at = keys.indexOf(stepRun.at);
  }
  const key = keys[at < 0 ? (by > 0 ? 0 : keys.length - 1) : at + by];
  const el = key ? items.find((x) => x.dataset.navKey === key) : undefined;
  if (!key || !el) return false;
  el.click();
  el.scrollIntoView({ block: "nearest" });
  stepRun = { keys, at: key, t: now };
  return true;
}

/** ⌘U: the paperclip of the box being written in, else of the chat that
 *  shows (the last one, with panes side by side). */
function pickFiles(): boolean {
  const visible = (b: HTMLButtonElement) => !b.disabled && b.offsetParent !== null;
  for (let n = document.activeElement as HTMLElement | null; n && n !== document.body; n = n.parentElement) {
    const b = n.querySelector<HTMLButtonElement>(".att-pick");
    if (b && visible(b)) { b.click(); return true; }
    if (n.classList.contains("app-main")) break;
  }
  const all = Array.from(document.querySelectorAll<HTMLButtonElement>(".app-main .att-pick")).filter(visible);
  const b = all[all.length - 1];
  if (!b) return false;
  b.click();
  return true;
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
  const simple = useSimpleMode();
  const [langChoice, setLangChoiceState] = useState<LangChoice>(storedLangChoice());
  const chooseLang = (choice: LangChoice) => {
    setLangChoice(choice);
    setLangChoiceState(choice);
  };
  const hubNames = useHubNames();
  const [info, setInfo] = useState<AppInfo | null>(initialInfo);
  setSystemDictation(!!info?.dictation);
  setSystemPicker(!!info?.pick_image);
  // Links out of the App open in the default browser (its web view opens
  // no new windows): a click on one anywhere in the page.
  const linksViaApp = !!info?.open_url;
  setOpenLinksViaApp(linksViaApp);
  useEffect(() => {
    if (!linksViaApp) return;
    // (after the page's own handlers: one that took the click keeps it)
    const on = (e: MouseEvent) => { handleLinkClick(e); };
    document.addEventListener("click", on);
    return () => document.removeEventListener("click", on);
  }, [linksViaApp]);
  const [collapsed, setCollapsed] = useState(initialCollapsed);
  const [hubs, setHubs] = useState<Hub[]>(initialHubs);
  // (whose a page link is: pages.tsx)
  setPageHubs(hubs);
  // What the hubs showed last (also from before a restart) until they answer.
  const [ov, setOv] = useState<AppOverview | null>(() => recall<AppOverview>("overview") ?? null);
  const [error, setError] = useState<unknown>(null);
  const [now, setNow] = useState(Date.now());
  const [localTick, setLocalTick] = useState(0);
  const [page, setPageRaw] = useState<Page>("space");
  // Settings is a dialog over the page you are on (like Kettu's): asking
  // for the "settings" page opens it and leaves the page as it is; any
  // other page closes it.
  const [settingsOpen, setSettingsOpen] = useState(false);
  const setPage = (p: Page) => {
    if (p === "settings") { setSettingsOpen(true); return; }
    setSettingsOpen(false);
    setPageRaw(p);
  };
  // The workspace on show, chosen at the bottom of the sidebar. Workspaces
  // are kept apart: everything shown and done is in this one.
  const [hubPick, setHubPickState] = useState<string>(() => { try { return localStorage.getItem(HUB_KEY) || ""; } catch { return ""; } });
  const setHubPick = (v: string) => {
    // Leaving a workspace: where you were in it is kept, to come back to.
    if (v !== hubId && hubId) {
      places.current[hubId] = { space: shown || undefined, page: page === "members" || page === "computers" || page === "automations" || page === "workflows" || page === "templates" ? page : "space" };
      try { localStorage.setItem(PLACES_KEY, JSON.stringify(places.current)); } catch { /* not kept */ }
    }
    setHubPickState(v);
    try { localStorage.setItem(HUB_KEY, v); } catch { /* not kept */ }
  };
  // Each workspace's place: the space (and its page) you were on there.
  const places = useRef<Record<string, { space?: SpaceRef; page?: Page }>>((() => {
    try { return JSON.parse(localStorage.getItem(PLACES_KEY) || "{}"); } catch { return {}; }
  })());
  // Switching workspaces (the switcher, ⌘1…⌘9): back where you were in it.
  const switchHub = (v: string) => {
    if (v === hubId) return;
    const back = places.current[v];
    setHubPick(v);
    setPage(back?.page || "space");
    if (back?.space) setSpace(back.space);
    setChatWith(null);
    setProfile(null);
  };
  const switchRef = useRef(switchHub);
  switchRef.current = switchHub;
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
  const [newLocalOpen, setNewLocalOpen] = useState(false);
  const [newServerOpen, setNewServerOpen] = useState(false);
  // The desktop app's tray menu speaks the pages' language: say it when
  // they load and when it changes (AppMode is drawn again then).
  const uiLang = lang();
  useEffect(() => { void appApi.putUILang(uiLang).catch(() => undefined); }, [uiLang]);
  // Settings › Voice, from a chat's voice button (VoiceBar).
  useEffect(() => {
    const on = (e: Event) => {
      const s = (e as CustomEvent<{ section?: string }>).detail?.section;
      if (s === "voice" || s === "hubs") openSettingsSection(s);
      setPage("settings");
    };
    window.addEventListener("myco-open-settings", on);
    return () => window.removeEventListener("myco-open-settings", on);
  }, []);
  // The workspaces of the shown workspace's server (hubs v0.19.9+): whether
  // this App may add one there.
  const [serverWs, setServerWs] = useState<WorkspaceState | null>(null);
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  const [addAgent, setAddAgent] = useState<SpaceRef | null>(null);
  // Where adding an agent begins (the computers page: a computer, or connecting one).
  const [addAgentStart, setAddAgentStart] = useState<string | undefined>();
  // A computer to open a terminal on (the computers page).
  const [addWorkspacePick, setAddWorkspacePick] = useState<string | undefined>();
  const [addWorkspace, setAddWorkspace] = useState<SpaceRef | null>(null);
  // A computer's settings: who may use it, its projects, removing it.
  const [pcSettings, setPcSettings] = useState<{ hubId: string; name: string; focus?: "name" | "remove" } | null>(null);
  useEffect(() => {
    // The terminal's header asks for them too.
    const on = (e: Event) => { const d = (e as CustomEvent<{ hubId: string; name: string }>).detail; if (d) setPcSettings(d); };
    window.addEventListener("myco-computer-settings", on);
    return () => window.removeEventListener("myco-computer-settings", on);
  }, []);
  const [profile, setProfile] = useState<{ hubId: string; name: string; details?: boolean } | null>(null);
  // 「换一个来接着做」: asked for from menus, the profile and a chat's header.
  const [handover, setHandover] = useState<HandoverTarget | null>(null);
  useEffect(() => {
    const on = (e: Event) => { const d = (e as CustomEvent<HandoverTarget>).detail; if (d) setHandover(d); };
    window.addEventListener("myco-handover", on);
    return () => window.removeEventListener("myco-handover", on);
  }, []);
  // An agent's profile (its open work), from anywhere: { hubId, name }.
  useEffect(() => {
    const on = (e: Event) => {
      const d = (e as CustomEvent<{ hubId: string; name: string }>).detail;
      if (!d) return;
      setNavChat(null);
      setPage("members");
      // Its details open: where its open work is.
      setProfile({ hubId: d.hubId, name: d.name, details: true });
      window.setTimeout(() => document.getElementById("agent-current-work")?.scrollIntoView({ block: "start", behavior: "smooth" }), 300);
    };
    window.addEventListener("myco-open-profile", on);
    return () => window.removeEventListener("myco-open-profile", on);
  }, []);
  // 复制一份 (Fork.tsx): the dialog, from any menu.
  const [fork, setFork] = useState<ForkTarget | null>(null);
  useEffect(() => {
    const on = (e: Event) => { const d = (e as CustomEvent<ForkTarget>).detail; if (d) setFork(d); };
    window.addEventListener("myco-fork", on);
    return () => window.removeEventListener("myco-fork", on);
  }, []);
  // Members open a chat: the space to show it in, and with whom.
  const [chatWith, setChatWith] = useState<{ space: SpaceRef; agent: string; n: number; focus?: string; reveal?: boolean } | null>(null);
  // ⌘K / Ctrl+K: find anything (CommandPalette).
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [projectReload, setProjectReload] = useState(0);
  // This computer's workspaces (the switcher tags them, the dialog names them).
  const [local, setLocal] = useState<LocalState | null>(() => recall<LocalState | null>("local") ?? null);

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
      const [h, o, pending, l] = await Promise.all([appApi.hubs(), appApi.overview(), readPendingLink(),
        appApi.local().catch(() => null)]);
      setHubs(h);
      if (l) setLocal(l);
      // The overview says what is on each hub; the hub list says which
      // version it is. The conversation needs both (what the hub can do).
      const versionOf = new Map(h.map((x) => [x.id, x.hub_version]));
      const withVersions = { ...o, hubs: o.hubs.map((x) => ({ ...x, hub_version: versionOf.get(x.id) ?? x.hub_version })) };
      setOv(remember("overview", seenNow(keepOffline(withVersions, recall<AppOverview>("overview")), Date.now())));
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
      // Nothing came back (this computer's network, say): what shows is
      // marked as old, as for a workspace that cannot be reached.
      setOv((o) => (o ? unreached(o, e) : o));
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
  const hubsRef = useRef(namedHubs);
  hubsRef.current = namedHubs;
  const namedOv = useMemo(() => ov && { ...ov, hubs: ov.hubs.map((h) => ({ ...h, name: nameOf(h) })) }, [ov, nameOf]);
  const allRows = useMemo<AgentRow[]>(() => {
    if (!namedOv) return [];
    rememberMachines(namedOv.hubs.flatMap((h) => h.overview?.machines || []));
    const out = namedOv.hubs.filter((h) => h.ok && h.overview).flatMap((h) => h.overview!.agents.map((st) => ({ hub: h, st })));
    rememberAgentHubs(out.map((r) => ({ hubId: r.hub.id, agentId: r.st.agent.id })));
    return out;
  }, [namedOv]);
  setPageMe((hid) => namedOv?.hubs.find((h) => h.id === hid)?.me?.member?.name || "");
  setPageNames((hid, name) => {
    const r = allRows.find((x) => x.hub.id === hid && x.st.agent.name === name);
    return r ? nodeName(r.st.agent, r.hub) : name;
  });

  // A workspace removed from the app is not chosen any more: the first one
  // that answers is shown instead.
  const hubId = hubs.some((h) => h.id === hubPick) ? hubPick
    : namedOv?.hubs.find((h) => h.ok && hubs.some((x) => x.id === h.id))?.id || hubs[0]?.id || "";
  const hub = namedHubs.find((h) => h.id === hubId);
  // Whether this App may add workspaces on the shown one's server (not for
  // a hub on this computer: that has its own way, NewWorkspaceDialog).
  const hubUrl = hub?.url || "";
  const hubIsLocal = !!hub && isLocalUrl(local, hub.url);
  useEffect(() => {
    setServerWs(null);
    if (!hubId || !hubUrl || hubIsLocal) return;
    let off = false;
    appApi.workspaces(hubId).then((s) => { if (!off) setServerWs(s); }).catch(() => { /* older App backend or hub */ });
    return () => { off = true; };
  }, [hubId, hubUrl, hubIsLocal]);
  const rows = useMemo(() => allRows.filter((r) => r.hub.id === hubId), [allRows, hubId]);
  // Ask this workspace's computers what they have ahead of need (at most
  // once a minute each): adding an agent then shows their sessions at once.
  const hubMachines = rows.find((r) => r.hub.id === hubId)?.hub;
  const machinesKey = (hubMachines ? machinesOfHub(hubMachines) : []).map((m) => `${m.id}:${m.online}:${m.myco_version}`).join(",");
  useEffect(() => {
    if (hubMachines) prefetchMachineInfos(hubMachines.id, machinesOfHub(hubMachines));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hubId, machinesKey]);
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
  // Chat (the agents in no project) is always there: you can start by
  // talking to an agent and make a project later, or never.
  const looseHubs = hubId ? [hubId] : [];
  const hubName = (id: string) => namedHubs.find((h) => h.id === id)?.name || namedOv?.hubs.find((h) => h.id === id)?.name || id;
  // The computers agents use, in the sidebar: one click opens a terminal.
  const computers = rows.filter((r) => isWorkspace(r.st.agent))
    .sort((a, b) => Number(a.st.state === "offline") - Number(b.st.state === "offline") || nodeName(a.st.agent, a.hub).localeCompare(nodeName(b.st.agent, b.hub)));
  // Projects and computers in the sidebar go in the order you drag them to
  // (kept per workspace); new ones first.
  const projectSort = useSortable(`${hubId}\u0000projects`, liveProjects.map((x) => x.p.name));
  const navProjects = projectSort.order.map((n) => liveProjects.find((x) => x.p.name === n)!).filter(Boolean);
  const computerSort = useSortable(`${hubId}\u0000computers`, computers.map((r) => r.st.agent.name));
  const navComputers = computerSort.order.map((n) => computers.find((r) => r.st.agent.name === n)!).filter(Boolean);
  const people = rows.filter((r) => agentKind(r.st.agent) === "agent")
    .sort((a, b) => Number(a.st.state === "offline") - Number(b.st.state === "offline"));
  // The sidebar's chat list: everyone you can talk to, one click into a
  // chat, in the order picked (kept on this computer).
  const [chatSort, setChatSort] = useState<ChatSort>(() => {
    try { const v = localStorage.getItem(CHAT_SORT_KEY); return v === "busy" || v === "name" ? v : "recent"; } catch { return "recent"; }
  });
  const pickChatSort = (v: ChatSort) => { setChatSort(v); try { localStorage.setItem(CHAT_SORT_KEY, v); } catch { /* not kept */ } };
  const [navChat, setNavChat] = useState<string | null>(null);
  // Pinned conversations stay on top whatever the order (per workspace,
  // kept on this computer).
  const pinKey = `myco-nav-pinned:${hubId}`;
  const [pinnedByHub, setPinnedByHub] = useState<Record<string, string[]>>({});
  const pinned = pinnedByHub[pinKey] ?? (() => { try { return JSON.parse(localStorage.getItem(pinKey) || "[]") as string[]; } catch { return []; } })();
  const pinChat = (name: string, on: boolean) => {
    const next = on ? [...pinned.filter((n) => n !== name), name] : pinned.filter((n) => n !== name);
    setPinnedByHub((m) => ({ ...m, [pinKey]: next }));
    try { localStorage.setItem(pinKey, JSON.stringify(next)); } catch { /* not kept */ }
  };
  const chatRows = useMemo(() => {
    const at = new Map(recentChats().filter((c) => c.hubId === hubId).map((c) => [c.agent, c.at]));
    const name = (r: AgentRow) => nodeName(r.st.agent, r.hub);
    const busy = (r: AgentRow) => ((r.st.asking || 0) > 0 ? 0 : r.st.state === "working" ? 1 : r.st.state === "offline" ? 3 : 2);
    const pin = (r: AgentRow) => { const i = pinned.indexOf(r.st.agent.name); return i < 0 ? Infinity : i; };
    return [...people].sort((a, b) => (pin(a) === pin(b) ? 0 : pin(a) < pin(b) ? -1 : 1) || (chatSort === "name" ? name(a).localeCompare(name(b))
      : chatSort === "busy" ? busy(a) - busy(b) || name(a).localeCompare(name(b))
      : (at.get(b.st.agent.name) || 0) - (at.get(a.st.agent.name) || 0) || busy(a) - busy(b) || name(a).localeCompare(name(b))));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [people.map((r) => `${r.st.agent.name}:${r.st.state}:${r.st.asking || 0}`).join(","), chatSort, hubId, navChat, pinned.join(",")]);
  useEffect(() => { if (page !== "space") setNavChat(null); }, [page]);
  // The sidebar's lists fold (kept on this computer).
  const [pcFolded, foldPcs] = useFold(PC_FOLD_KEY);
  const [projFolded, foldProjects] = useFold("myco-nav-projects-folded");
  const [chatFolded, foldChats] = useFold("myco-nav-chats-folded");

  // The space on show: the one chosen (while it exists), else the newest
  // project, else chat.
  const spaceExists = (s: SpaceRef) => isLoose(s)
    ? looseHubs.includes(s.hubId)
    : liveProjects.some((x) => x.hubId === s.hubId && x.p.name === s.name);
  const shown: SpaceRef | null = space && spaceExists(space) ? space
    : liveProjects[0] ? { hubId: liveProjects[0].hubId, name: liveProjects[0].p.name }
    : looseHubs[0] ? { hubId: looseHubs[0], loose: true }
    : null;
  const shownProject = shown && !isLoose(shown) ? liveProjects.find((x) => x.hubId === shown.hubId && x.p.name === shown.name)?.p : undefined;

  const meOn = (hubId: string) => {
    const name = namedOv?.hubs.find((h) => h.id === hubId)?.me?.member?.name;
    return name ? `@console:${name}` : undefined;
  };

  // Picking a space (again) shows its group, as a chat app's list does.
  // (counted per space: picking one does not send another back to its home)
  const [spaceTap, setSpaceTap] = useState<Record<string, number>>({});
  const openSpace = (s: SpaceRef) => {
    setSpace(s);
    setPage("space");
    setChatWith(null);
    setSpaceTap((t) => ({ ...t, [spaceKey(s)]: (t[spaceKey(s)] || 0) + 1 }));
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
  const chatWithMember = (r: AgentRow, where?: SpaceRef, reveal?: boolean) => {
    const p = liveProjects.find((x) => x.hubId === r.hub.id && x.p.members.some((m) => m.name === r.st.agent.name));
    const s: SpaceRef = where || (p ? { hubId: r.hub.id, name: p.p.name } : { hubId: r.hub.id, loose: true });
    setProfile(null);
    setSpace(s);
    setPage("space");
    setChatWith({ space: s, agent: r.st.agent.name, n: Date.now(), reveal });
  };
  // ← → (nav.ts): where you are, recorded on each move; going back or
  // forward puts it back. The chat is the one opened from the sidebar, or
  // else the one asked for (a search, a workflow's step) in what is shown.
  const shownKey = shown ? spaceKey(shown) : "";
  const navChatNow = page !== "space" ? null
    : navChat || (chatWith && spaceKey(chatWith.space) === shownKey ? `${chatWith.space.hubId}\u0000${chatWith.agent}` : null);
  useEffect(() => {
    if (!hubId) return;
    navRecord({ hub: hubId, page, space: page === "space" || page === "about" ? shownKey : undefined, chat: navChatNow });
  }, [hubId, page, shownKey, navChatNow]);
  const navApply = useRef<(p: NavPlace) => void>(() => undefined);
  navApply.current = (p) => {
    if (p.hub !== hubId && hubs.some((h) => h.id === p.hub)) setHubPick(p.hub);
    setProfile(null);
    const [sh, sn] = (p.space || "").split("\u0000");
    const ref: SpaceRef | null = !p.space ? null : sn === "*" ? { hubId: sh, loose: true } : { hubId: sh, name: sn };
    if (p.page === "space" && p.chat) {
      const [ch, ca] = p.chat.split("\u0000");
      const r = allRows.find((x) => x.hub.id === ch && x.st.agent.name === ca);
      setNavChat(p.chat);
      if (r) { chatWithMember(r, ref || undefined); return; }
    }
    setNavChat(null);
    if (p.page === "space" && ref) { openSpace(ref); return; }
    if (ref) setSpace(ref);
    setChatWith(null);
    setPage(p.page as Page);
  };
  useEffect(() => onNavApply((p) => navApply.current(p)), []);

  // A message in its conversation (search, an automation's run).
  const openAtMessage = (r: AgentRow, message: string) => {
    const p = (projects || []).find((x) => !x.p.archived && x.hubId === r.hub.id && x.p.members.some((m) => m.name === r.st.agent.name));
    const s: SpaceRef = p ? { hubId: r.hub.id, name: p.p.name } : { hubId: r.hub.id, loose: true };
    setProfile(null);
    setSpace(s);
    setPage("space");
    setChatWith({ space: s, agent: r.st.agent.name, n: Date.now(), focus: message });
  };
  // The top bar's call (VoiceChip): back to the chat with whom it is with.
  const toCallRef = useRef<(hubId: string, agent: string) => void>(() => undefined);
  toCallRef.current = (hubId, agent) => {
    const r = allRows.find((x) => x.hub.id === hubId && x.st.agent.name === agent);
    if (r) { setHubPick(hubId); chatWithMember(r); }
  };
  useEffect(() => {
    const on = (e: Event) => { const d = (e as CustomEvent<{ hubId: string; agent: string }>).detail; toCallRef.current(d.hubId, d.agent); };
    window.addEventListener("myco-voice-open", on);
    return () => window.removeEventListener("myco-voice-open", on);
  }, []);
  // Whether a space has this member (a project's members, or the loose).
  const spaceHas = (s: SpaceRef, r: AgentRow) => (isLoose(s)
    ? s.hubId === r.hub.id && loose.some((x) => x.hub.id === r.hub.id && x.st.agent.name === r.st.agent.name)
    : s.hubId === r.hub.id && !!liveProjects.find((x) => x.hubId === s.hubId && x.p.name === s.name)?.p.members.some((m) => m.name === r.st.agent.name));

  // The notch's island opens the app at a member: its workspace first,
  // then (once its rows are there) its chat.
  const [toOpen, setToOpen] = useState<{ hubId: string; agent: string; project?: string } | null>(null);
  // Your superagent in the workspace on show (bottom-left menu): its chat,
  // or setting one up.
  const sa = useSuperagent(hubId || undefined);
  const [saSetup, setSaSetup] = useState(false);
  const openSuperagent = (name: string) => window.dispatchEvent(new CustomEvent("myco-open-member", { detail: { hubId, agent: name } }));
  // The quick panel (Mac) sends you here to set one up.
  useEffect(() => {
    const on = (e: Event) => {
      const h = (e as CustomEvent<{ hubId?: string }>).detail?.hubId;
      if (h) setHubPick(h);
      setSaSetup(true);
    };
    window.addEventListener("myco-superagent-setup", on);
    return () => window.removeEventListener("myco-superagent-setup", on);
  }, []);
  const onSuperagent = sa && !sa.off && hubId
    ? () => { const n = sa.state?.agent?.name; if (n) openSuperagent(n); else setSaSetup(true); }
    : undefined;
  useEffect(() => {
    const on = (e: Event) => {
      const d = (e as CustomEvent<{ hubId: string; agent: string; project?: string }>).detail;
      if (!d?.hubId || !d.agent) return;
      setHubPick(d.hubId);
      setToOpen(d);
    };
    window.addEventListener("myco-open-member", on);
    return () => window.removeEventListener("myco-open-member", on);
  }, []);
  useEffect(() => {
    if (!toOpen) return;
    const r = rows.find((x) => x.hub.id === toOpen.hubId && x.st.agent.name === toOpen.agent);
    if (!r) return;
    setToOpen(null);
    // Where: the space you are looking at, when it is in it (no jumping
    // away); else the project its work is in; else one it is in. Shown
    // already, it stays as it is (ProjectSpace: reveal).
    const here = page === "space" && shown && spaceHas(shown, r) ? shown : undefined;
    const work: SpaceRef | undefined = toOpen.project ? { hubId: r.hub.id, name: toOpen.project } : undefined;
    chatWithMember(r, here || (work && spaceHas(work, r) ? work : undefined), true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [toOpen, rows]);

  const closeConnect = (open: boolean) => {
    setConnectOpen(open);
    if (!open) setConnectPrefill("");
  };
  const hubAdded = async () => { await refresh(); };

  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  // The window-wide keys (all listed in Settings › 快捷键, Shortcuts.tsx;
  // a new one goes there too). ⌘ on a Mac, Ctrl elsewhere. On the Mac the
  // menu bar offers some of them as well: a key the page takes
  // (preventDefault) does not reach the menu, so each acts once.
  const keysRef = useRef({ settingsOpen, hasHub: false });
  keysRef.current = { settingsOpen, hasHub: !!hub };
  useEffect(() => {
    const overlay = () => !!document.querySelector(".app-modal, .cp, .app-menu, .lightbox");
    const onKey = (e: KeyboardEvent) => {
      if (e.isComposing || e.defaultPrevented) return;
      // ⌥↑ / ⌥↓ (Alt): the conversation above or below in the sidebar.
      if (e.altKey && !e.metaKey && !e.ctrlKey && !e.shiftKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
        if (!overlay() && !keysRef.current.settingsOpen && stepConversation(e.key === "ArrowUp" ? -1 : 1)) e.preventDefault();
        return;
      }
      if (!(e.metaKey || e.ctrlKey) || e.altKey) return;
      // Text size: ⌘= (⌘+ too) / ⌘- / ⌘0, in place of the page zoom.
      if (e.code === "Equal" || e.code === "NumpadAdd" || e.key === "+" || e.key === "=") {
        e.preventDefault();
        setTextSize((s) => (s === "small" ? "standard" : "large"));
        return;
      }
      if (e.code === "Minus" || e.code === "NumpadSubtract" || e.key === "-") {
        e.preventDefault();
        setTextSize((s) => (s === "large" ? "standard" : "small"));
        return;
      }
      if (e.shiftKey) {
        // ⌘⇧L: light ⇄ dark (from what shows now, also when following the system).
        if (e.code === "KeyL") {
          e.preventDefault();
          setTheme(document.documentElement.classList.contains("dark") ? "light" : "dark");
        }
        return;
      }
      if (e.code === "Digit0" || e.code === "Numpad0") {
        e.preventDefault();
        setTextSize("standard");
        return;
      }
      // ⌘1…⌘9 (Ctrl on Windows): the workspaces, in the switcher's order.
      const digit = /^Digit([1-9])$/.exec(e.code);
      if (digit) {
        const h = hubsRef.current[Number(digit[1]) - 1];
        if (h) { e.preventDefault(); switchRef.current(h.id); }
        return;
      }
      if (e.key === ",") {
        e.preventDefault();
        setPage("settings");
      } else if (e.key === "/" || e.code === "Slash") {
        e.preventDefault();
        openShortcuts(keysRef.current.settingsOpen);
        setPage("settings");
      } else if (e.key === "\\" || e.code === "Backslash") {
        e.preventDefault();
        toggleSidebarRef.current();
      } else if (e.code === "KeyN") {
        if (overlay() || !keysRef.current.hasHub) return;
        e.preventDefault();
        setNewProjectOpen(true);
      } else if (e.code === "KeyU") {
        if (overlay()) return;
        if (pickFiles()) e.preventDefault();
      } else if (e.key === "[" || e.code === "BracketLeft") {
        e.preventDefault();
        navBack();
      } else if (e.key === "]" || e.code === "BracketRight") {
        e.preventDefault();
        navForward();
      } else if (e.key === "r" || e.key === "R") {
        e.preventDefault();
        void refreshRef.current();
      } else if (e.key === "k" || e.key === "K") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // A top page picked again while on it goes back to its first view (the
  // list, not the automation or computer opened): it is drawn afresh.
  const [again, setAgain] = useState(0);
  const toPage = (p: typeof page) => { if (page === p) { setAgain((n) => n + 1); navSub(p, null); } setPage(p); };
  // 上手清单 (Onboard.tsx): ticks itself from what the workspace has.
  const [autoCount, setAutoCount] = useState<number | null>(null);
  useEffect(() => {
    let off = false;
    if (!hubId || (() => { try { return localStorage.getItem("myco-onboard") !== "1" || localStorage.getItem("myco-checklist-off") === "1"; } catch { return true; } })()) return;
    const read = () => appApi.automations(hubId).then((r) => { if (!off) setAutoCount((r.automations || []).length); }, () => undefined);
    void read();
    const t = window.setInterval(read, 60_000);
    // The automations page says how many there are each time it reads
    // them (after one is made, at once).
    const on = (e: Event) => { const d = (e as CustomEvent<{ hubId: string; count: number }>).detail; if (d?.hubId === hubId && !off) setAutoCount(d.count); };
    window.addEventListener("myco-automations", on);
    return () => { off = true; window.clearInterval(t); window.removeEventListener("myco-automations", on); };
  }, [hubId, page]);
  // A workflow run to open on the 工作流 page (from an automation's runs).
  const [wfOpen, setWfOpen] = useState<{ workflow: string; run?: string; at: number } | undefined>();
  // Decisions a workflow waits on you for: a count on its nav item.
  const [decisionCount, setDecisionCount] = useState(0);
  useEffect(() => {
    let off = false;
    setDecisionCount(0);
    if (!hubId) return;
    const read = () => appApi.workflowDecisions(hubId).then((r) => { if (!off) setDecisionCount((r.decisions || []).length); }, () => undefined);
    void read();
    const t = window.setInterval(read, 20_000);
    // The workflows page says how many each time it reads them.
    const on = (e: Event) => { const d = (e as CustomEvent<{ hubId: string; count: number }>).detail; if (d?.hubId === hubId && !off) setDecisionCount(d.count); };
    window.addEventListener("myco-workflow-decisions", on);
    return () => { off = true; window.clearInterval(t); window.removeEventListener("myco-workflow-decisions", on); };
  }, [hubId]);
  // 准备这台电脑 (Prepare.tsx): after a new install's first workspace
  // starts, until done or skipped; Settings › About opens it again.
  const [preparing, setPreparing] = useState<string | null>(() => preparePending());
  useEffect(() => {
    const on = () => { if (hubId) { startOnboarding(); setPreparing(hubId); } };
    window.addEventListener("myco-prepare", on);
    return () => window.removeEventListener("myco-prepare", on);
  }, [hubId]);
  const startLocal = async () => {
    const r = await appApi.startLocalHub({ tunnel: "none" });
    setPreparePending(r.hub.id);
    startOnboarding();
    setPreparing(r.hub.id);
    await refresh();
  };

  // Another workspace on this computer: start it, show it under the name
  // that was typed, and switch to it.
  const createOnServer = async (name: string, display: string) => {
    const h = await appApi.createWorkspace(hubId, name, display);
    await refresh();
    setHubPick(h.id);
    setPage("space");
    setChatWith(null);
  };

  const startAnotherLocal = async (name: string, display: string) => {
    const r = await appApi.startLocalHub({ tunnel: "none", name });
    if (display) renameHub(r.hub.id, display);
    await refresh();
    setHubPick(r.hub.id);
    setPage("space");
    setChatWith(null);
  };

  const toggleSidebar = () => {
    setCollapsed((c) => {
      const next = !c;
      try { localStorage.setItem(SIDEBAR_KEY, next ? "closed" : "open"); } catch { /* storage unavailable */ }
      return next;
    });
  };
  const toggleSidebarRef = useRef(toggleSidebar);
  toggleSidebarRef.current = toggleSidebar;
  // The Mac's menu bar (appmenu_darwin.go) asks for what it offers.
  const menuRef = useRef<(cmd: string) => void>(() => undefined);
  menuRef.current = (cmd: string) => {
    switch (cmd) {
      case "settings": setPage("settings"); break;
      case "about": openSettingsSection("about"); setPage("settings"); break;
      case "search": setPaletteOpen((o) => !o); break;
      case "sidebar": toggleSidebar(); break;
      case "new-project": setNewProjectOpen(true); break;
      case "connect": setConnectOpen(true); break;
      case "add-computer": if (hub) setAddWorkspace({ hubId: hub.id, loose: true }); break;
      case "refresh": void refreshRef.current(); break;
      case "shortcuts": openShortcuts(settingsOpen); setPage("settings"); break;
      case "back": navBack(); break;
      case "forward": navForward(); break;
      case "text-bigger": setTextSize((s) => (s === "small" ? "standard" : "large")); break;
      case "text-smaller": setTextSize((s) => (s === "large" ? "standard" : "small")); break;
      case "text-standard": setTextSize("standard"); break;
      case "theme": setTheme(document.documentElement.classList.contains("dark") ? "light" : "dark"); break;
    }
  };
  useEffect(() => {
    const on = (e: Event) => menuRef.current(String((e as CustomEvent<string>).detail || ""));
    window.addEventListener("myco-menu", on);
    return () => window.removeEventListener("myco-menu", on);
  }, []);
  const sidebar = useSidebarWidth(() => {
    setCollapsed(true);
    try { localStorage.setItem(SIDEBAR_KEY, "closed"); } catch { /* storage unavailable */ }
  });
  const dragStyle = { "--wails-draggable": "drag" } as React.CSSProperties;
  const isMac = info?.shell === "mac";
  // On the page's root too: what opens over the window (a picture, a file in
  // full screen) keeps clear of the traffic lights.
  useEffect(() => { document.documentElement.classList.toggle("shell-mac", isMac); }, [isMac]);
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
        <Welcome onStartLocal={startLocal} onConnect={() => setConnectOpen(true)} langChoice={langChoice} onLang={chooseLang} theme={theme} onTheme={setTheme} />
        <ConnectHubDialog open={connectOpen} onOpenChange={closeConnect} prefill={connectPrefill} onAdded={() => void hubAdded()} />
      </div>
    );
  }

  if (preparing && hubs.some((h) => h.id === preparing)) {
    const at = preparing;
    return (
      <div key={lang()} className={cn("app-shell welcome-shell", isMac && "shell-mac")}>
        <Prepare hubId={at} hubName={namedHubs.find((h) => h.id === at)?.name || ""} hubUrl={hubs.find((h) => h.id === at)?.url || ""} now={now} onDone={(joined) => {
          setPreparePending(null);
          setPreparing(null);
          void refresh();
          if (joined[0]) {
            const s: SpaceRef = { hubId: at, loose: true };
            setHubPick(at);
            setSpace(s);
            setPage("space");
            setChatWith({ space: s, agent: joined[0], n: Date.now() });
          }
        }} />
      </div>
    );
  }

  const hereOv = namedOv?.hubs.find((h) => h.id === hubId)?.overview;
  const myAgents = rows.filter((r) => r.hub.id === hubId && agentKind(r.st.agent) === "agent");
  const addAgentHere = (start?: string) => { if (hub) { setAddAgentStart(start); setAddAgent({ hubId: hub.id, loose: true }); } };
  const checklist: ChecklistStep[] = !hub ? [] : [
    { key: "agent", title: L("连上第一个 agent", "Connect your first agent"), hint: L("把这台电脑上的 Claude Code、Codex 会话加进来", "Bring in a Claude Code or Codex session from this computer"),
      done: myAgents.length > 0, go: () => addAgentHere() },
    { key: "task", title: L("交给它第一件事", "Give it something to do"), hint: L("在对话里说一句要做什么，回车发出去", "Say what you want done in its chat and press Enter"),
      done: !!hereOv?.recent?.some((m) => m.type === "task" && m.from.startsWith("@console:")) || myAgents.some((r) => r.st.in_progress + r.st.queued > 0),
      go: () => { if (myAgents[0]) chatWithMember(myAgents[0]); else addAgentHere(); } },
    { key: "workspace", title: L("认识你的工作区", "Meet your workspace"), hint: L("agent 在这里碰头；看看怎么让别的电脑和同事加入", "Where agents meet; see how other computers and people join"),
      done: false, seen: "workspaces", go: () => window.dispatchEvent(new CustomEvent("myco-open-settings", { detail: { section: "hubs" } })) },
    { key: "project", title: L("让两个 agent 一起做一件事", "Have two agents work together"), hint: L("建一个项目，把它们拉进来", "Start a project and bring them in"),
      done: hubProjects.some((x) => !x.p.archived && x.p.members.length >= 2), go: () => setNewProjectOpen(true) },
    { key: "automation", title: L("设一个自动化", "Set up an automation"), hint: L("让它每天到点自己干活", "Have it work on its own, on a schedule"),
      done: (autoCount ?? 0) > 0, go: () => toPage("automations") },
    { key: "computer", title: L("再连一台电脑", "Connect another computer"), hint: L("让 agent 也能在服务器或另一台电脑上干活", "So agents can work on a server or another computer too"),
      done: (hereOv?.machines?.length ?? 0) >= 2, go: () => addAgentHere("connect") },
  ];

  const profileRow = profile ? rows.find((r) => r.hub.id === profile.hubId && r.st.agent.name === profile.name) || null : null;
  const addAgentProject = addAgent && !isLoose(addAgent) ? liveProjects.find((x) => x.hubId === addAgent.hubId && x.p.name === addAgent.name)?.p : undefined;
  const addWsProject = addWorkspace && !isLoose(addWorkspace) ? liveProjects.find((x) => x.hubId === addWorkspace.hubId && x.p.name === addWorkspace.name)?.p : undefined;
  const title = page === "members" ? L("成员", "Members")
    : page === "computers" ? L("电脑", "Computers")
    : page === "automations" ? L("自动化", "Automations")
    : page === "workflows" ? L("工作流", "Workflows")
    : page === "templates" ? L("应用", "Apps")
    : page === "settings" ? L("设置", "Settings")
    : page === "about" ? L("项目资料", "Project notes")
    // A project says its own name, larger, just below.
    : "";
  // A workspace's dot: green while someone is on work that has not gone quiet,
  // amber when all the work in progress may be stuck (it was picked up, but
  // nothing is moving). A node with both reads as working: something is live.
  // An agent waiting on your decision comes first: amber, saying so.
  // Tasks left open with no life in them (the hub says so: stale) only get a
  // quiet ring: nothing is running; their chat says which and offers to
  // close them.
  const askingYou = (r: AgentRow): boolean => {
    if (!(r.st.asking || 0)) return false;
    const held = (r.st.current || []).filter((m) => m.status === "held" && !!m.asking);
    // Not told which (a member who cannot see its messages): as before.
    if (held.length === 0) return true;
    return held.some((m) => isMe(m.from, r.hub));
  };
  const workDot = (rs: AgentRow[]): "asking" | "busy" | "stuck" | "open" | null => {
    // Amber "needs your decision" only for a question to you: a task you
    // gave it, held on a question back to you. One to another agent (it
    // asked whoever gave it the task) only leaves something open.
    if (rs.some((r) => askingYou(r))) return "asking";
    let busy = false;
    let stuck = false;
    let open = false;
    for (const r of rs) {
      if ((r.st.stale || 0) > 0 || (r.st.asking || 0) > 0) open = true;
      if (r.st.state !== "working" || r.st.in_progress <= 0) continue;
      // A hub that tells live from stale: what it calls in progress runs.
      if (r.st.stale !== undefined || r.st.in_progress > stuckCounts(r.st, now, r.hub.seen_at).in_progress) busy = true;
      else stuck = true;
    }
    return busy ? "busy" : stuck ? "stuck" : open ? "open" : null;
  };

  return (
    <div key={lang()} className={cn("app-shell", isMac && "shell-mac", collapsed && "collapsed")}
      style={{ ["--sidebar-w" as string]: `${sidebar.width}px` }}>
      {!collapsed && (
        <aside className="app-sidebar">
          {/* Its right edge: drag to make it wider or narrower. */}
          <div className={cn("app-sidebar-edge", sidebar.dragging && "is-dragging", sidebar.hiding && "is-hiding")}
            {...sidebar.handle} style={{ ["--wails-draggable" as string]: "no-drag" }}
            aria-label={L("拖动调整边栏宽度", "Drag to resize the sidebar")}
            title={L("拖动调整宽度，双击恢复", "Drag to resize; double-click to reset")} />
          <div className="app-sidebar-head" style={dragStyle}>
            <div className="brand"><LogoWordmark className="app-brand-wordmark" /></div>
          </div>
          <button type="button" className="app-nav-search" onClick={() => setPaletteOpen(true)} style={{ ["--wails-draggable" as string]: "no-drag" }}>
            <Search /><span>{L("搜索", "Search")}</span><kbd>{modKey()}K</kbd>
          </button>

          <nav className="app-nav">
            {/* Pages on top, rows like the ones below (an icon each):
                automations, workflows, apps, computers. Then three lists under one heading
                style: projects first (the work), conversations (everyone you
                can talk to, one click into a chat, pinned ones first, in the
                order you pick; the height left, scrolling), computers last.
                The three lists fold. */}
            <div className="app-nav-group sp-nav-autos">
              <button type="button" className={cn("app-nav-item sp-nav-top sp-nav-auto", page === "automations" && "active")} aria-current={page === "automations" ? "page" : undefined}
                onClick={() => { setNavChat(null); toPage("automations"); }}>
                <Zap /><span className="app-nav-label">{L("自动化", "Automations")}</span>
              </button>
              <button type="button" className={cn("app-nav-item sp-nav-top sp-nav-workflows", page === "workflows" && "active")} aria-current={page === "workflows" ? "page" : undefined}
                onClick={() => { setNavChat(null); toPage("workflows"); }}>
                <Workflow /><span className="app-nav-label">{L("工作流", "Workflows")}</span>
                {decisionCount > 0 && <span className="wf-nav-badge" title={L(`${decisionCount} 个决定在等你`, `${decisionCount} decisions wait for you`)}>{decisionCount}</span>}
              </button>
              <button type="button" className={cn("app-nav-item sp-nav-top sp-nav-templates", page === "templates" && "active")} aria-current={page === "templates" ? "page" : undefined}
                onClick={() => { setNavChat(null); toPage("templates"); }}>
                <LayoutGrid /><span className="app-nav-label">{L("应用", "Apps")}</span>
              </button>
              <button type="button" className={cn("app-nav-item sp-nav-top sp-nav-computers", page === "computers" && "active")} aria-current={page === "computers" ? "page" : undefined}
                onClick={() => { setNavChat(null); toPage("computers"); }}>
                <Monitor /><span className="app-nav-label">{L("电脑", "Computers")}</span>
              </button>
            </div>
            <div className="app-nav-group sp-nav-projects">
              <NavHead label={L("项目", "Projects")} folded={projFolded} onFold={() => foldProjects(!projFolded)} count={liveProjects.length}>
                <button type="button" className="app-nav-add" aria-label={L("新建项目", "New project")} title={L("新建项目", "New project")}
                  onClick={() => setNewProjectOpen(true)}><Plus /></button>
              </NavHead>
              {!projFolded && navProjects.map((x) => {
                const ref = { hubId: x.hubId, name: x.p.name };
                const active = page === "space" && !!shown && spaceKey(shown) === spaceKey(ref);
                const names = new Set(x.p.members.map((m) => m.name));
                const dot = workDot(rows.filter((r) => r.hub.id === x.hubId && names.has(r.st.agent.name)));
                return (
                  <button key={`${x.hubId}/${x.p.name}`} type="button" {...projectSort.bind(x.p.name)}
                    className={cn("app-nav-item sp-nav", active && "active", projectSort.dragging === x.p.name && "is-dragging")}
                    aria-current={active ? "page" : undefined} onClick={() => openSpace(ref)} data-nav-key={`p:${x.hubId}/${x.p.name}`}
                    onContextMenu={(e) => navMenu.open(e, [
                      { label: L("添加 agent", "Add an agent"), onSelect: () => { openSpace(ref); setAddAgent(ref); } },
                      { label: L("加一个终端", "Add a terminal"), onSelect: () => { openSpace(ref); setAddWorkspace(ref); } },
                      { label: L("项目资料", "Project notes"), onSelect: () => { setSpace(ref); setPage("about"); } },
                    ])}
                    title={x.p.goal || x.p.title}>
                    <span className="app-nav-label">{x.p.title || x.p.name}</span>
                    {dot && (
                      <span className={cn("sp-nav-busy", dot === "stuck" && "is-stuck", dot === "asking" && "is-asking", dot === "open" && "is-open")}
                        title={dot === "asking" ? L("有 agent 在等你决定", "Someone needs your decision") : dot === "stuck" ? L("有成员可能卡住", "Someone may be stuck") : dot === "open" ? L("有任务没收尾", "Some tasks were left open") : L("有成员在干活", "Someone is at work")} />
                    )}
                  </button>
                );
              })}
              {!projFolded && projects !== null && liveProjects.length === 0 && (
                <button type="button" className="app-nav-item muted" onClick={() => setNewProjectOpen(true)}>
                  <Plus strokeWidth={1.75} /><span>{L("新建项目", "New project")}</span>
                </button>
              )}
            </div>
            <div className={cn("app-nav-group sp-nav-chats", chatFolded && "is-folded")}>
              {looseHubs.map((id) => {
                const ref: SpaceRef = { hubId: id, loose: true };
                const active = page === "space" && !navChat && !!shown && spaceKey(shown) === spaceKey(ref);
                const sorts: { v: ChatSort; label: string }[] = [
                  { v: "recent", label: L("最近聊过的在前", "Recently talked to first") },
                  { v: "busy", label: L("在忙的在前", "Busy first") },
                  { v: "name", label: L("按名字", "By name") },
                ];
                return (
                  <NavHead key={`${id}/*`} label={L("会话", "Conversations")} className="sp-nav-chat" active={active}
                    title={L("不在项目里的 agent：直接找它们聊", "Agents in no project: talk to them directly")}
                    onOpen={() => { setNavChat(null); openSpace(ref); }} folded={chatFolded} onFold={() => foldChats(!chatFolded)} count={chatRows.length}>
                    <button type="button" className="app-nav-add sp-nav-chat-sort" aria-label={L("排序和成员", "Order and members")} title={L("排序方式、管理成员", "How to order, manage members")}
                      onClick={(e) => navMenu.openUnder(e.currentTarget, [
                        { header: L("排序（置顶的总在最上面）", "Order (pinned ones stay on top)") },
                        ...sorts.map((x) => ({ label: x.label, checked: chatSort === x.v, onSelect: () => pickChatSort(x.v) })),
                        "sep",
                        { label: L("管理成员", "Manage members"), onSelect: () => { setNavChat(null); setPage("members"); } },
                      ])}><ArrowUpDown /></button>
                    <button type="button" className="app-nav-add sp-nav-chat-add" aria-label={L("添加 agent", "Add an agent")} title={L("添加 agent：多一个能聊的", "Add an agent: one more to talk to")}
                      onClick={() => hub && setAddAgent({ hubId: hub.id, loose: true })}><Plus /></button>
                  </NavHead>
                );
              })}
              {!chatFolded && <div className="sp-nav-chat-list">
                {chatRows.map((r) => {
                  const key = `${r.hub.id}\u0000${r.st.agent.name}`;
                  const dot = workDot([r]);
                  const isPinned = pinned.includes(r.st.agent.name);
                  return (
                    <button key={key} type="button" className={cn("app-nav-item sp-nav sp-nav-person", r.st.state === "offline" && "is-away", isPinned && "is-pinned", page === "space" && navChat === key && "active")}
                      onClick={() => { setNavChat(key); chatWithMember(r); }} data-nav-key={`c:${key}`}
                      draggable onDragStart={(e) => dragAgent(e, { hubId: r.hub.id, agent: r.st.agent.name })}
                      onContextMenu={(e) => navMenu.open(e, [
                        { label: L("聊天", "Chat"), onSelect: () => { setNavChat(key); chatWithMember(r); } },
                        // Beside what is open (a project, the chats): a pane of its own.
                        ...(page === "space" ? [{ label: L("在新的一栏打开", "Open in a new pane"), onSelect: () => window.dispatchEvent(new CustomEvent("myco-open-pane", { detail: { hubId: r.hub.id, agent: r.st.agent.name } })) }] : []),
                        { label: isPinned ? L("取消置顶", "Unpin") : L("置顶", "Pin to the top"), onSelect: () => pinChat(r.st.agent.name, !isPinned) },
                        { label: L("看它的资料", "Its profile"), onSelect: () => { setNavChat(null); setPage("members"); setProfile({ hubId: r.hub.id, name: r.st.agent.name }); } },
                        ...forkItem(r),
                      ])}
                      title={[nodeName(r.st.agent, r.hub), r.st.agent.description].filter(Boolean).join(" · ")}>
                      <Avatar agent={r.st.agent} size={20} badge={false} />
                      <span className="app-nav-label">{nodeName(r.st.agent, r.hub)}</span>
                      {dot ? (
                        <span className={cn("sp-nav-busy", dot === "stuck" && "is-stuck", dot === "asking" && "is-asking", dot === "open" && "is-open")}
                          title={dot === "asking" ? L("在等你决定", "Needs your decision") : dot === "stuck" ? L("可能卡住", "May be stuck") : dot === "open" ? L("有任务没做完：没在干活（没收尾，或在等别的 agent），点开对话看是哪些", "Tasks still open: nothing is running (left open, or waiting on another agent); open the chat to see which") : L("在干活", "At work")} />
                      ) : isPinned && <Pin className="sp-nav-pinned" aria-label={L("已置顶", "Pinned")} />}
                    </button>
                  );
                })}
                {rows.length > 0 && chatRows.length === 0 && (
                  <button type="button" className="app-nav-item sp-nav muted" onClick={() => hub && setAddAgent({ hubId: hub.id, loose: true })}>
                    <Plus /><span className="app-nav-label">{L("添加 agent", "Add an agent")}</span>
                  </button>
                )}
              </div>}
            </div>
            {!simple && <div className="app-nav-group sp-nav-pcs">
              <NavHead label={L("终端", "Terminals")} className="sp-nav-pcs-head" active={page === "computers"} title={L("电脑和终端", "Computers and terminals")}
                onOpen={() => { setNavChat(null); toPage("computers"); }} folded={pcFolded} onFold={() => foldPcs(!pcFolded)} count={computers.length}>
                <button type="button" className="app-nav-add" aria-label={L("加一个终端", "Add a terminal")} title={L("加一个终端", "Add a terminal")}
                  onClick={() => (shownProject && shown ? setAddWorkspace(shown) : hub && setAddWorkspace({ hubId: hub.id, loose: true }))}><Plus /></button>
              </NavHead>
              {!pcFolded && (<>
              {navComputers.slice(0, NAV_COMPUTERS).map((r) => {
                  const dot = workDot([r]);
                  const pc = { hubId: r.hub.id, name: r.st.agent.name };
                  const menu = [
                    { label: L("打开终端", "Open the terminal"), onSelect: () => chatWithMember(r) },
                    { label: L("改名", "Rename"), onSelect: () => setPcSettings({ ...pc, focus: "name" as const }) },
                    { label: L("谁能用、项目", "Who can use it, projects"), onSelect: () => setPcSettings(pc) },
                    { label: L("移除这个终端", "Remove this terminal"), onSelect: () => setPcSettings({ ...pc, focus: "remove" as const }) },
                  ];
                  return (
                    <div key={r.st.agent.name} className="sp-nav-pc-wrap">
                      <button type="button" {...computerSort.bind(r.st.agent.name)}
                        className={cn("app-nav-item sp-nav sp-nav-pc", r.st.state === "offline" && "is-away", computerSort.dragging === r.st.agent.name && "is-dragging")}
                        onClick={() => chatWithMember(r)} onContextMenu={(e) => navMenu.open(e, menu)}
                        title={L(`打开终端 · ${whereOf(r)}`, `Open the terminal · ${whereOf(r)}`)}>
                        <Avatar agent={r.st.agent} size={20} badge={false} />
                        <span className="app-nav-label">{nodeName(r.st.agent, r.hub)}</span>
                        {dot && (
                          <span className={cn("sp-nav-busy", dot === "stuck" && "is-stuck", dot === "asking" && "is-asking", dot === "open" && "is-open")}
                            title={dot === "asking" ? L("有 agent 在等你决定", "Someone needs your decision") : dot === "stuck" ? L("可能卡住", "May be stuck") : dot === "open" ? L("有命令没收尾", "Commands left open") : L("正在跑命令", "Running a command")} />
                        )}
                      </button>
                      <button type="button" className="sp-nav-more" aria-label={L(`${nodeName(r.st.agent, r.hub)}：设置`, `${nodeName(r.st.agent, r.hub)}: settings`)}
                        title={L("谁能用、项目、移除", "Who can use it, projects, remove")} onClick={(e) => navMenu.openUnder(e.currentTarget, menu)}>
                        <MoreHorizontal />
                      </button>
                    </div>
                  );
                })}
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
              </>)}
            </div>}
          </nav>

          <div className="app-sidebar-foot">
            <Checklist steps={checklist} />
            <UpdateNotice onOpenSettings={() => setPage("settings")} />
            {/* Which workspace the projects above are in: set once, seldom
                switched, so it sits at the bottom. */}
            <div className="sp-switch">
              <HubSwitcher hubs={namedHubs} hubsOv={namedOv?.hubs} value={hubId}
                onChange={switchHub}
                onAdd={() => setConnectOpen(true)} onNewLocal={() => setNewLocalOpen(true)}
                onNewOnServer={serverWs?.allowed ? () => setNewServerOpen(true) : undefined}
                serverName={serverWs ? hostName(serverWs.server) : undefined}
                isLocal={(h) => isLocalUrl(local, h.url)}
                simple={simple}
                onSuperagent={onSuperagent}
                superagentKey={isMac ? "⌘⇧Space" : undefined}
                superagent={sa?.state ? { name: sa.state.agent?.name, pending: sa.approvals.filter((a) => a.status === "pending").length } : undefined}
                onSettings={() => setPage("settings")} />
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
          <NavArrows />
          <h1 className="app-title">{title}</h1>
          <VoiceChip />
          <PageToggle />
        </header>

        {error !== null && (
          <div className="app-banner-wrap">
            <div className="app-error-row">
              <WifiOff />
              <span className="err" title={errorMessage(error)}>{L("暂时拿不到最新的内容，正在重试…", "Can't get the latest right now; retrying…")}</span>
            </div>
          </div>
        )}

        {page === "space" && (shown ? (
          <ProjectSpace
            key={spaceKey(shown)}
            space={shown}
            project={shownProject}
            looseRows={loose}
            hubName={hubName(shown.hubId)}
            rows={rows}
            now={now}
            me={meOn(shown.hubId)}
            openChat={chatWith && spaceKey(chatWith.space) === spaceKey(shown) ? { agent: chatWith.agent, n: chatWith.n, focus: chatWith.focus, reveal: chatWith.reveal } : undefined}
            home={spaceTap[spaceKey(shown)] || 0}
            onAddAgent={() => setAddAgent(shown)}
            onAddWorkspace={() => setAddWorkspace(shown)}
            onOpenMember={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name })}
            onAbout={shownProject ? () => setPage("about") : undefined}
            onRefresh={() => void refresh()}
            projects={liveProjects.filter((x) => x.hubId === shown.hubId).map((x) => x.p)}
            onNewProject={() => setNewProjectOpen(true)}
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

        {page === "automations" && hub && (
          <div className="app-scroll">
            <AutomationsPage key={`${hub.id}/${again}`} hubId={hub.id} rows={rows} now={now}
              onOpenMessage={(agent, message) => { const r = rows.find((x) => x.hub.id === hub.id && x.st.agent.name === agent); if (r) openAtMessage(r, message); }}
              onOpenRun={(workflow, run) => { setWfOpen({ workflow, run, at: Date.now() }); setPage("workflows"); }} />
          </div>
        )}

        {page === "workflows" && hub && (
          <div className="app-scroll">
            <WorkflowsPage key={`${hub.id}/${again}`} hubId={hub.id} rows={rows} now={now} open={wfOpen} onOpened={() => setWfOpen(undefined)}
              onOpenMessage={(agent, message) => { const r = rows.find((x) => x.hub.id === hub.id && x.st.agent.name === agent); if (r) openAtMessage(r, message); }} />
          </div>
        )}

        {page === "templates" && hub && (
          <div className="app-scroll">
            <TemplatesPage key={`${hub.id}/${again}`} hubId={hub.id} now={now} projects={liveProjects.map((x) => ({ name: x.p.name, title: x.p.title || x.p.name }))} />
          </div>
        )}

        {page === "computers" && (
          <div className="app-scroll">
            <ComputersPage key={again} rows={rows} projects={hubProjects}
              onOpen={chatWithMember}
              onProfile={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name })}
              onConnect={() => { if (hub) { setAddAgentStart("connect"); setAddAgent({ hubId: hub.id, loose: true }); } }}
              onAddAgentOn={(m) => { if (hub) { setAddAgentStart(`m:${m.id}`); setAddAgent({ hubId: hub.id, loose: true }); } }}
              onAddTerminal={(m) => { if (hub) { setAddWorkspacePick(m?.id); setAddWorkspace({ hubId: hub.id, loose: true }); } }}
              onSettings={(r) => setPcSettings({ hubId: r.hub.id, name: r.st.agent.name })}
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

        {settingsOpen && (
          <SettingsDialog onClose={() => setSettingsOpen(false)}>
            <SettingsPage
              titleId="settings-title"
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
          </SettingsDialog>
        )}
      </div>

      {navMenu.element}
      <ComputerSettings focus={pcSettings?.focus} row={pcSettings ? rows.find((r) => r.hub.id === pcSettings.hubId && r.st.agent.name === pcSettings.name) || null : null}
        rows={rows} projects={hubProjects} onClose={() => setPcSettings(null)} onChanged={() => { setProjectReload((n) => n + 1); void refresh(); void loadProjects(hubs.map((h) => h.id)); }}
        onOpenTerminal={(r) => chatWithMember(r)} />
      <ForkDialog target={fork} rows={rows} onClose={() => setFork(null)} onDone={(hubId, agent, project) => {
        setFork(null);
        // Its chat opens (in the project it was put in); the overview catches up.
        const sp: SpaceRef = project ? { hubId, name: project.name } : { hubId, loose: true };
        setProfile(null);
        setSpace(sp);
        setPage("space");
        setChatWith({ space: sp, agent, n: Date.now() });
        setProjectReload((n) => n + 1);
        void refresh();
      }} />
      <HandoverDialog target={handover} rows={rows} onClose={() => setHandover(null)} onDone={() => void refresh()}
        onOpenProfile={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name, details: true })} />
      <NamePrompt hubsOv={namedOv?.hubs} onRefresh={refresh} />
      <ConnectHubDialog open={connectOpen} onOpenChange={closeConnect} prefill={connectPrefill} onAdded={() => void hubAdded()} />

      <NewProjectDialog open={newProjectOpen} hubs={hub ? [hub] : []} onClose={() => setNewProjectOpen(false)} onCreated={projectCreated} />
      <AddAgentDialog open={!!addAgent} hub={namedHubs.find((h) => h.id === addAgent?.hubId)} project={addAgentProject} projects={hubProjects.map((x) => x.p)} rows={rows}
        start={addAgentStart} onClose={() => { setAddAgent(null); setAddAgentStart(undefined); }} onAdded={() => { setProjectReload((n) => n + 1); void refresh(); }} />
      <AddWorkspaceDialog open={!!addWorkspace} hub={namedHubs.find((h) => h.id === addWorkspace?.hubId)} project={addWsProject} rows={rows}
        workspace={namedOv?.hubs.find((h) => h.id === addWorkspace?.hubId)} pick={addWorkspacePick}
        onClose={() => { setAddWorkspace(null); setAddWorkspacePick(undefined); }} onAdded={() => void refresh()} />
      {hubId && <SuperagentSetup open={saSetup} onOpenChange={setSaSetup} hubId={hubId} rows={allRows} current={sa?.state?.agent} onReady={openSuperagent} />}
      <NewWorkspaceDialog open={newLocalOpen} onOpenChange={setNewLocalOpen}
        taken={localWorkspaces(local).map((w) => w.name)} onStart={startAnotherLocal} />
      <NewWorkspaceDialog open={newServerOpen} onOpenChange={setNewServerOpen} server={serverWs ? hostName(serverWs.server) : undefined}
        taken={(serverWs?.workspaces || []).map((w) => w.name)} onStart={createOnServer} />


      <PageHost />
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} rows={allRows} hubs={namedOv?.hubs || []} now={now}
        projects={(projects || []).filter((x) => !x.p.archived).map((x) => ({ hubId: x.hubId, name: x.p.name, title: x.p.title || x.p.name, goal: x.p.goal }))}
        onPick={(t) => {
          setProfile(null);
          if (t.kind === "page") { setPage(t.page); return; }
          if (t.kind === "project") { setHubPick(t.hubId); openSpace({ hubId: t.hubId, name: t.name }); return; }
          setHubPick(t.row.hub.id);
          if (t.kind === "member") { chatWithMember(t.row); return; }
          openAtMessage(t.row, t.message);
        }} />
      <MemberSheet row={profileRow} details={!!profile?.details} projects={hubProjects} rows={rows} now={now} onClose={() => setProfile(null)}
        onChat={chatWithMember} onChanged={() => void refresh()} onRefresh={refresh}
        onOpenProject={(ref) => { setProfile(null); openSpace(ref); }}
        onOpenMachine={() => { setProfile(null); setPage("computers"); }} />
    </div>
  );
}

const SPACE_KEY = "myco-app-space";
// Computers listed in the sidebar before "All N".
const NAV_COMPUTERS = 6;
type ChatSort = "recent" | "busy" | "name";
const CHAT_SORT_KEY = "myco-nav-chat-sort";
const PC_FOLD_KEY = "myco-nav-pcs-folded";
const HUB_KEY = "myco-app-hub";
const PLACES_KEY = "myco-app-places";
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

/** The workspaces that answered now carry when (seen_at); those showing
 *  what they said before keep when that was. */
export function seenNow(ov: AppOverview, at: number): AppOverview {
  return { ...ov, hubs: ov.hubs.map((h) => (h.ok && !h.stale ? { ...h, seen_at: at } : h)) };
}

/** No answer at all this time: every workspace shows what it said last,
 *  marked so (as keepOffline does for one that could not be reached). */
export function unreached(ov: AppOverview, e: unknown): AppOverview {
  const error = e instanceof Error ? e.message : String(e);
  return {
    ...ov,
    hubs: ov.hubs.map((h) => (h.stale || !h.ok ? h
      : { ...h, stale: { at: h.seen_at ? new Date(h.seen_at).toISOString() : ov.generated_at, error } })),
  };
}

/** A server's address as people say it: its host name. */
function hostName(u: string): string {
  try { return new URL(u).host; } catch { return u; }
}

/** Whether a sidebar list is folded, kept on this computer. */
function useFold(key: string): [boolean, (v: boolean) => void] {
  const [v, setV] = useState(() => { try { return localStorage.getItem(key) === "1"; } catch { return false; } });
  return [v, (n: boolean) => { setV(n); try { localStorage.setItem(key, n ? "1" : "0"); } catch { /* not kept */ } }];
}

/** A sidebar heading, the same for every section: a fold arrow (or its
 *  space), the name (it opens the section's page, else folds), how many
 *  while folded, and the section's buttons on the right. The name lines
 *  up with the rows under it; the fold arrow comes after it. */
function NavHead({ label, active, className, title, onOpen, folded, onFold, count, children }: {
  label: string;
  active?: boolean;
  className?: string;
  title?: string;
  onOpen?: () => void;
  folded?: boolean;
  onFold?: () => void;
  count?: number;
  children?: React.ReactNode;
}) {
  return (
    <div className="app-nav-title-row sp-nav-head">
      <button type="button" className={cn("sp-nav-title-link", className, active && "active")} title={title} aria-current={active ? "page" : undefined}
        onClick={onOpen ?? onFold}>
        {label}{folded && count ? <span className="sp-nav-head-count">{count}</span> : null}
      </button>
      {onFold && (
        <button type="button" className="sp-nav-fold" aria-expanded={!folded} aria-label={folded ? L(`展开${label}`, `Show ${label}`) : L(`收起${label}`, `Hide ${label}`)}
          onClick={onFold}><ChevronRight className={cn(!folded && "is-open")} /></button>
      )}
      {children && <span className="sp-nav-head-tools">{children}</span>}
    </div>
  );
}

/** Settings, over the App (the page under it stays): a scrim, a large
 *  panel with its nav on the left, ✕ at the top right. Esc or a click on
 *  the scrim closes it, unless a dialog of its own is open on top. */
function SettingsDialog({ onClose, children }: { onClose: () => void; children: React.ReactNode }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      if (document.querySelector(".app-modal, .app-menu, .cp, .lightbox")) return;
      e.preventDefault();
      onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <>
      <div className="set-scrim" onClick={onClose} aria-hidden />
      <div className="set-dialog" role="dialog" aria-modal="true" aria-labelledby="settings-title" data-testid="settings-dialog">
        <button type="button" className="set-close" onClick={onClose} aria-label={L("关闭设置", "Close settings")} title={L("关闭（Esc）", "Close (Esc)")}><X /></button>
        {children}
      </div>
    </>
  );
}

/** ← → in the crumb bar: back to where you were, forward again (nav.ts).
 *  ⌘[ / ⌘] too, and a mouse's back and forward buttons. */
function NavArrows() {
  const { back, forward } = useNavState();
  useEffect(() => {
    const on = (e: MouseEvent) => {
      if (e.button === 3) { e.preventDefault(); navBack(); } else if (e.button === 4) { e.preventDefault(); navForward(); }
    };
    window.addEventListener("mouseup", on);
    return () => window.removeEventListener("mouseup", on);
  }, []);
  const mod = modKey();
  return (
    <span className="app-nav-arrows" data-testid="nav-arrows">
      <button type="button" className="app-btn app-btn-plain" onClick={navBack} disabled={!back}
        aria-label={L("后退", "Back")} title={L(`后退（${mod}[）`, `Back (${mod}[)`)}><ChevronLeft /></button>
      <button type="button" className="app-btn app-btn-plain" onClick={navForward} disabled={!forward}
        aria-label={L("前进", "Forward")} title={L(`前进（${mod}]）`, `Forward (${mod}])`)}><ChevronRight /></button>
    </span>
  );
}
