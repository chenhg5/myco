import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, ArrowUpDown, ChevronRight, MoreHorizontal, Plus, Search, WifiOff, Zap } from "lucide-react";
import { ComputerSettings } from "@/app/ComputerSettings";
import { AutomationsPage } from "@/app/Automations";
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
import { UpdateNotice } from "@/app/UpdateNotice";
import { Welcome } from "@/app/Welcome";
import { ProjectSpace, isLoose, looseRowsOf, type SpaceRef } from "@/app/ProjectSpace";
import { AddAgentDialog, AddWorkspaceDialog, NewProjectDialog, isNobody } from "@/app/AddToProject";
import { MemberSheet, MembersPage } from "@/app/Members";
import { HubSwitcher } from "@/app/HubSwitcher";
import { isLocalUrl, localWorkspaces } from "@/app/localHubs";
import { NewWorkspaceDialog } from "@/app/NewWorkspaceDialog";
import { ComputersPage } from "@/app/Computers";
import { Avatar, isWorkspace, rememberAgentHubs, rememberMachines, whereOf } from "@/app/persona";
import { agentKind, nodeName, stuckCounts } from "@/app/nodes";
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
type Page = "space" | "members" | "computers" | "automations" | "settings" | "about";

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
  const [page, setPage] = useState<Page>("space");
  // The workspace on show, chosen at the bottom of the sidebar. Workspaces
  // are kept apart: everything shown and done is in this one.
  const [hubPick, setHubPickState] = useState<string>(() => { try { return localStorage.getItem(HUB_KEY) || ""; } catch { return ""; } });
  const setHubPick = (v: string) => {
    // Leaving a workspace: where you were in it is kept, to come back to.
    if (v !== hubId && hubId) {
      places.current[hubId] = { space: shown || undefined, page: page === "members" || page === "computers" || page === "automations" ? page : "space" };
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
      if ((e as CustomEvent<{ section?: string }>).detail?.section === "voice") openSettingsSection("voice");
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
      setOv(remember("overview", keepOffline(withVersions, recall<AppOverview>("overview"))));
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
  const chatRows = useMemo(() => {
    const at = new Map(recentChats().filter((c) => c.hubId === hubId).map((c) => [c.agent, c.at]));
    const name = (r: AgentRow) => nodeName(r.st.agent, r.hub);
    const busy = (r: AgentRow) => ((r.st.asking || 0) > 0 ? 0 : r.st.state === "working" ? 1 : r.st.state === "offline" ? 3 : 2);
    return [...people].sort((a, b) => chatSort === "name" ? name(a).localeCompare(name(b))
      : chatSort === "busy" ? busy(a) - busy(b) || name(a).localeCompare(name(b))
      : (at.get(b.st.agent.name) || 0) - (at.get(a.st.agent.name) || 0) || busy(a) - busy(b) || name(a).localeCompare(name(b)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [people.map((r) => `${r.st.agent.name}:${r.st.state}:${r.st.asking || 0}`).join(","), chatSort, hubId, navChat]);
  useEffect(() => { if (page !== "space") setNavChat(null); }, [page]);
  // Computers: folded or not (kept on this computer).
  const [pcFolded, setPcFolded] = useState(() => { try { return localStorage.getItem(PC_FOLD_KEY) === "1"; } catch { return false; } });
  const foldPcs = (v: boolean) => { setPcFolded(v); try { localStorage.setItem(PC_FOLD_KEY, v ? "1" : "0"); } catch { /* not kept */ } };

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
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) return;
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
  }, []);

  const startLocal = async () => {
    await appApi.startLocalHub({ tunnel: "none" });
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
    : page === "automations" ? L("自动化", "Automations")
    : page === "settings" ? L("设置", "Settings")
    : page === "about" ? L("项目资料", "Project notes")
    // A project says its own name, larger, just below.
    : "";
  // A workspace's dot: green while someone is on work that has not gone quiet,
  // amber when all the work in progress may be stuck (it was picked up, but
  // nothing is moving). A node with both reads as working: something is live.
  // An agent waiting on your decision comes first: amber, saying so.
  const workDot = (rs: AgentRow[]): "asking" | "busy" | "stuck" | null => {
    if (rs.some((r) => (r.st.asking || 0) > 0)) return "asking";
    let busy = false;
    let stuck = false;
    for (const r of rs) {
      if (r.st.state !== "working" || r.st.in_progress <= 0) continue;
      if (r.st.in_progress > stuckCounts(r.st, now).in_progress) busy = true;
      else stuck = true;
    }
    return busy ? "busy" : stuck ? "stuck" : null;
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

          <button type="button" className={cn("app-nav-item sp-nav sp-nav-auto", page === "automations" && "active")} onClick={() => setPage("automations")}
            style={{ ["--wails-draggable" as string]: "no-drag" }}>
            <Zap />
            <span className="app-nav-label">{L("自动化", "Automations")}</span>
          </button>

          <nav className="app-nav">
            {/* Projects first: the work. Then chat: everyone you can talk to,
                one click into a chat, in the order you pick; it takes the
                height left and scrolls. Computers last, foldable. */}
            <div className="app-nav-group sp-nav-projects">
              <div className="app-nav-title app-nav-title-row">
                <span>{L("项目", "Projects")}</span>
                <button type="button" className="app-nav-add" aria-label={L("新建项目", "New project")} title={L("新建项目", "New project")}
                  onClick={() => setNewProjectOpen(true)}><Plus /></button>
              </div>
              {navProjects.map((x) => {
                const ref = { hubId: x.hubId, name: x.p.name };
                const active = page === "space" && !!shown && spaceKey(shown) === spaceKey(ref);
                const names = new Set(x.p.members.map((m) => m.name));
                const dot = workDot(rows.filter((r) => r.hub.id === x.hubId && names.has(r.st.agent.name)));
                return (
                  <button key={`${x.hubId}/${x.p.name}`} type="button" {...projectSort.bind(x.p.name)}
                    className={cn("app-nav-item sp-nav", active && "active", projectSort.dragging === x.p.name && "is-dragging")}
                    aria-current={active ? "page" : undefined} onClick={() => openSpace(ref)}
                    onContextMenu={(e) => navMenu.open(e, [
                      { label: L("添加 agent", "Add an agent"), onSelect: () => { openSpace(ref); setAddAgent(ref); } },
                      { label: L("加一台电脑", "Add a computer"), onSelect: () => { openSpace(ref); setAddWorkspace(ref); } },
                      { label: L("项目资料", "Project notes"), onSelect: () => { setSpace(ref); setPage("about"); } },
                    ])}
                    title={x.p.goal || x.p.title}>
                    <span className="app-nav-label">{x.p.title || x.p.name}</span>
                    {dot && (
                      <span className={cn("sp-nav-busy", dot === "stuck" && "is-stuck", dot === "asking" && "is-asking")}
                        title={dot === "asking" ? L("有 agent 在等你决定", "Someone needs your decision") : dot === "stuck" ? L("有成员可能卡住", "Someone may be stuck") : L("有成员在干活", "Someone is at work")} />
                    )}
                  </button>
                );
              })}
              {projects !== null && liveProjects.length === 0 && (
                <button type="button" className="app-nav-item muted" onClick={() => setNewProjectOpen(true)}>
                  <Plus strokeWidth={1.75} /><span>{L("新建项目", "New project")}</span>
                </button>
              )}
            </div>
            <div className="app-nav-group sp-nav-chats">
              {looseHubs.map((id) => {
                const ref: SpaceRef = { hubId: id, loose: true };
                const active = page === "space" && !navChat && !!shown && spaceKey(shown) === spaceKey(ref);
                const sorts: { v: ChatSort; label: string }[] = [
                  { v: "recent", label: L("最近聊过的在前", "Recently talked to first") },
                  { v: "busy", label: L("在忙的在前", "Busy first") },
                  { v: "name", label: L("按名字", "By name") },
                ];
                return (
                  <div key={`${id}/*`} className="sp-nav-chat-wrap">
                    <button type="button" className={cn("app-nav-item sp-nav sp-nav-chat", active && "active")} onClick={() => { setNavChat(null); openSpace(ref); }}
                      title={L("不在项目里的 agent：直接找它们聊", "Agents in no project: talk to them directly")}>
                      <span className="app-nav-label">{L("聊天", "Chat")}</span>
                    </button>
                    <button type="button" className="app-nav-add sp-nav-chat-sort" aria-label={L("排序和成员", "Order and members")} title={L("排序方式、管理成员", "How to order, manage members")}
                      onClick={(e) => navMenu.openUnder(e.currentTarget, [
                        { header: L("排序", "Order") },
                        ...sorts.map((x) => ({ label: x.label, checked: chatSort === x.v, onSelect: () => pickChatSort(x.v) })),
                        "sep",
                        { label: L("管理成员", "Manage members"), onSelect: () => { setNavChat(null); setPage("members"); } },
                      ])}><ArrowUpDown /></button>
                  </div>
                );
              })}
              <div className="sp-nav-chat-list">
                {chatRows.map((r) => {
                  const key = `${r.hub.id}\u0000${r.st.agent.name}`;
                  const dot = workDot([r]);
                  return (
                    <button key={key} type="button" className={cn("app-nav-item sp-nav sp-nav-person", r.st.state === "offline" && "is-away", page === "space" && navChat === key && "active")}
                      onClick={() => { setNavChat(key); chatWithMember(r); }}
                      onContextMenu={(e) => navMenu.open(e, [
                        { label: L("聊天", "Chat"), onSelect: () => { setNavChat(key); chatWithMember(r); } },
                        { label: L("看它的资料", "Its profile"), onSelect: () => { setNavChat(null); setPage("members"); setProfile({ hubId: r.hub.id, name: r.st.agent.name }); } },
                      ])}
                      title={[nodeName(r.st.agent, r.hub), r.st.agent.description].filter(Boolean).join(" · ")}>
                      <Avatar agent={r.st.agent} size={20} badge={false} />
                      <span className="app-nav-label">{nodeName(r.st.agent, r.hub)}</span>
                      {dot && (
                        <span className={cn("sp-nav-busy", dot === "stuck" && "is-stuck", dot === "asking" && "is-asking")}
                          title={dot === "asking" ? L("在等你决定", "Needs your decision") : dot === "stuck" ? L("可能卡住", "May be stuck") : L("在干活", "At work")} />
                      )}
                    </button>
                  );
                })}
                {rows.length > 0 && chatRows.length === 0 && (
                  <button type="button" className="app-nav-item sp-nav muted" onClick={() => hub && setAddAgent({ hubId: hub.id, loose: true })}>
                    <Plus /><span className="app-nav-label">{L("添加 agent", "Add an agent")}</span>
                  </button>
                )}
              </div>
            </div>
            <div className="app-nav-group sp-nav-pcs">
              <div className="app-nav-title app-nav-title-row">
                <button type="button" className="sp-nav-fold" aria-expanded={!pcFolded} aria-label={pcFolded ? L("展开电脑", "Show computers") : L("收起电脑", "Hide computers")}
                  onClick={() => foldPcs(!pcFolded)}><ChevronRight className={cn(!pcFolded && "is-open")} /></button>
                <button type="button" className={cn("sp-nav-title-link", page === "computers" && "active")} onClick={() => setPage("computers")}
                  title={L("管理电脑", "Manage computers")}>{L("电脑", "Computers")}{pcFolded && computers.length > 0 ? ` · ${computers.length}` : ""}</button>
                <button type="button" className="app-nav-add" aria-label={L("加一台电脑", "Add a computer")} title={L("加一台电脑", "Add a computer")}
                  onClick={() => (shownProject && shown ? setAddWorkspace(shown) : hub && setAddWorkspace({ hubId: hub.id, loose: true }))}><Plus /></button>
              </div>
              {!pcFolded && (<>
              {navComputers.slice(0, NAV_COMPUTERS).map((r) => {
                  const dot = workDot([r]);
                  const pc = { hubId: r.hub.id, name: r.st.agent.name };
                  const menu = [
                    { label: L("打开终端", "Open the terminal"), onSelect: () => chatWithMember(r) },
                    { label: L("改名", "Rename"), onSelect: () => setPcSettings({ ...pc, focus: "name" as const }) },
                    { label: L("谁能用、项目", "Who can use it, projects"), onSelect: () => setPcSettings(pc) },
                    { label: L("移除这台电脑", "Remove this computer"), onSelect: () => setPcSettings({ ...pc, focus: "remove" as const }) },
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
                          <span className={cn("sp-nav-busy", dot === "stuck" && "is-stuck", dot === "asking" && "is-asking")}
                            title={dot === "asking" ? L("有 agent 在等你决定", "Someone needs your decision") : dot === "stuck" ? L("可能卡住", "May be stuck") : L("正在跑命令", "Running a command")} />
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
            </div>
          </nav>

          <div className="app-sidebar-foot">
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
          {page === "about" && (
            <button type="button" className="app-btn app-btn-plain" onClick={() => setPage("space")} aria-label={L("返回项目", "Back to the project")} title={L("返回项目", "Back to the project")}>
              <ArrowLeft />
            </button>
          )}
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
            <AutomationsPage key={hub.id} hubId={hub.id} rows={rows} now={now} />
          </div>
        )}

        {page === "computers" && (
          <div className="app-scroll">
            <ComputersPage rows={rows} projects={hubProjects}
              onOpen={chatWithMember}
              onProfile={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name })}
              onAddComputer={() => hub && setAddWorkspace({ hubId: hub.id, loose: true })}
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
      <ComputerSettings focus={pcSettings?.focus} row={pcSettings ? rows.find((r) => r.hub.id === pcSettings.hubId && r.st.agent.name === pcSettings.name) || null : null}
        rows={rows} projects={hubProjects} onClose={() => setPcSettings(null)} onChanged={() => { setProjectReload((n) => n + 1); void refresh(); void loadProjects(hubs.map((h) => h.id)); }}
        onOpenTerminal={(r) => chatWithMember(r)} />
      <HandoverDialog target={handover} rows={rows} onClose={() => setHandover(null)} onDone={() => void refresh()}
        onOpenProfile={(r) => setProfile({ hubId: r.hub.id, name: r.st.agent.name, details: true })} />
      <NamePrompt hubsOv={namedOv?.hubs} onRefresh={refresh} />
      <ConnectHubDialog open={connectOpen} onOpenChange={closeConnect} prefill={connectPrefill} onAdded={() => void hubAdded()} />

      <NewProjectDialog open={newProjectOpen} hubs={hub ? [hub] : []} onClose={() => setNewProjectOpen(false)} onCreated={projectCreated} />
      <AddAgentDialog open={!!addAgent} hub={namedHubs.find((h) => h.id === addAgent?.hubId)} project={addAgentProject} projects={hubProjects.map((x) => x.p)} rows={rows}
        onClose={() => setAddAgent(null)} onAdded={() => { setProjectReload((n) => n + 1); void refresh(); }} />
      <AddWorkspaceDialog open={!!addWorkspace} hub={namedHubs.find((h) => h.id === addWorkspace?.hubId)} project={addWsProject} rows={rows}
        workspace={namedOv?.hubs.find((h) => h.id === addWorkspace?.hubId)}
        onClose={() => setAddWorkspace(null)} onAdded={() => void refresh()} />
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
          // A message: its conversation, at that message.
          const r = t.row;
          const p = (projects || []).find((x) => !x.p.archived && x.hubId === r.hub.id && x.p.members.some((m) => m.name === r.st.agent.name));
          const s: SpaceRef = p ? { hubId: r.hub.id, name: p.p.name } : { hubId: r.hub.id, loose: true };
          setSpace(s);
          setPage("space");
          setChatWith({ space: s, agent: r.st.agent.name, n: Date.now(), focus: t.message });
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

/** A server's address as people say it: its host name. */
function hostName(u: string): string {
  try { return new URL(u).host; } catch { return u; }
}
