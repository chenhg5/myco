import { createContext, useCallback, useContext, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { AppWindow, Check, Copy, Download, ExternalLink, Link2, Lock, Monitor, MoreHorizontal, PanelRight, PanelRightClose, Pin as PinIcon, PinOff, Radio, RotateCw, Trash2, Users, X } from "lucide-react";
import { canInstall, installApp, installed, isInstalled, uninstallApp } from "@/app/install";
import type { Page, PageOpen } from "@/api";
import { ApiError, appApi, isOldHub, type OsAnswer, type OsConsent, type BrowserStatus, type BrowserApprovalReview, type BrowserProfileSettingsInput } from "@/appapi";
import { copyText } from "@/app/copy";
import { useContextMenu, type MenuItem } from "@/app/ContextMenu";
import { DockPanel } from "@/app/preview";
import { looksOf, MushroomFace } from "@/app/persona";
import { Modal } from "@/app/dialogs";
import { openOutside, setInAppLinks } from "@/app/links";
import { L, E } from "@/lib/i18n";
import { BrowserProfileSettingsDetails, browserSettingsReview, type BrowserSettingsReview } from "./browser-profile-settings";
import { answerProjectFiles, projectFileScope } from "./page-project-files";
import { answerPageFiles, pageFileScope, pageFilesUpgrade } from "./page-files";
import { canRecoverTemplate, PageRecovery, recoveryLabel } from "./page-recovery";
import { usePagePreferences, localizedPageURL } from "./page-preferences";
import { parsePageLink, pageFragmentURL, type PageLink } from "@/lib/page-links";
import { cn, timeAgo } from "@/lib/utils";

/*
 * Web pages agents publish. A link to one,
 * <hub>/pages/<id>, shows under its message as a card; it opens in the page
 * panel (the panel files open in: beside the chat when there is room). Its
 * files come from this App's own loopback port, not the App's origin, in a
 * sandbox: it cannot reach the App or the internet. A link to this computer
 * (http://localhost:5173, an agent's dev server here) opens in the same
 * panel.
 */

type HubRef = { id: string; url: string; aliases?: string[] };
let hubs: HubRef[] = [];
// An agent's name as people see it (AppMode knows the members).
let nameOf: (hubId: string, name: string) => string = (_h, n) => n;

/** How a publisher's name is shown. */
export function setPageNames(f: (hubId: string, name: string) => string) {
  nameOf = f;
}
const base = (u: string) => u.replace(/\/+$/, "");

/** The App's workspaces, to know whose a page link is. */
export function setPageHubs(list: HubRef[]) {
  hubs = list.map((h) => ({ id: h.id, url: base(h.url), aliases: (h.aliases || []).map(base) }));
}

/** A known workspace page, with an optional application-owned fragment. */
export function pageLinkOf(href: string): PageLink | null {
  return parsePageLink(href, hubs);
}

/** A link to a server on this computer (not the App itself). */
export function localLinkOf(href: string): URL | null {
  try {
    const u = new URL(href);
    if (u.protocol !== "http:" && u.protocol !== "https:") return null;
    if (!["localhost", "127.0.0.1", "[::1]"].includes(u.hostname)) return null;
    if (u.origin === window.location.origin) return null;
    return u;
  } catch { return null; }
}

export type PageTarget = { kind: "page"; hubId: string; id: string; fragment?: string; routeVisit?: number } | { kind: "local"; url: string };

/** Opens a page (or a server on this computer) in the page panel. */
export function openPage(t: PageTarget) {
  window.dispatchEvent(new CustomEvent<PageTarget>("myco-open-page", { detail: t }));
}

/** For a link click: a page or a local server opens in the panel. */
export function pageTargetOf(href: string): PageTarget | null {
  const p = pageLinkOf(href);
  if (p) return { kind: "page", ...p };
  const u = localLinkOf(href);
  return u ? { kind: "local", url: u.href } : null;
}

setInAppLinks((href) => {
  const t = pageTargetOf(href);
  if (t) openPage(t);
  return !!t;
});

const URLS = /https?:\/\/[^\s<>()"'`，。；、]+/g;

/** The page links in a message's text, once each. */
export function pageLinksIn(body: string): { hubId: string; id: string; url: string }[] {
  const seen = new Set<string>();
  const out: { hubId: string; id: string; url: string }[] = [];
  for (const m of body.matchAll(URLS)) {
    const url = m[0].replace(/[.,;:!?)\]]+$/, "");
    const p = pageLinkOf(url);
    if (p && !seen.has(`${p.hubId}/${p.id}${p.fragment || ""}`)) { seen.add(`${p.hubId}/${p.id}${p.fragment || ""}`); out.push({ ...p, url }); }
  }
  return out;
}

/** Pinning a page's card above the conversation it is in (pins.tsx): the
 *  conversation provides it; without one (or on a hub without pins) a
 *  card's menu has no 置顶. */
export type PagePins = { pinned: (id: string) => boolean; toggle: (p: { id: string; url: string; name: string }) => void };
export const PagePinsContext = createContext<PagePins | null>(null);

// Who you are on a workspace (for a page's myco.ready).
let meOf: (hubId: string) => string = () => "";
export function setPageMe(f: (hubId: string) => string) {
  meOf = f;
}

// Pages you let send messages as you (this time the App runs), and when
// they did (at most 5 a minute).
const allowedSend = new Set<string>();
const sends = new Map<string, number[]>();

// A page's own storage (myco.storage), kept by the App: pages cannot see
// each other's.
const storeKey = (hubId: string, id: string) => `myco-page-store:${hubId}:${id}`;
function storeOf(key: string): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(key) || "{}");
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) return parsed as Record<string, unknown>;
  } catch { /* unavailable or invalid */ }
  return {};
}

// The panel as the top bar's toggle sees it (PageToggle), and the toggle
// asking it to show or hide.
type PanelState = { count: number; title: string; open: boolean };
let panelState: PanelState = { count: 0, title: "", open: false };
const panelSubs = new Set<() => void>();
function publishPanel(next: PanelState) {
  if (next.count === panelState.count && next.title === panelState.title && next.open === panelState.open) return;
  panelState = next;
  panelSubs.forEach((f) => f());
}

/** In the App's top bar, at its right end: the pages open in the panel
 *  (it is one for the whole App, whichever chat opened them). Shown while
 *  there are some; pressed while the panel shows; a click shows or tucks it
 *  away. */
export function PageToggle() {
  const st = useSyncExternalStore((f) => { panelSubs.add(f); return () => { panelSubs.delete(f); }; }, () => panelState);
  if (!st.count) return null;
  const label = st.open ? L("收起网页面板", "Tuck the pages away") : L(E`展开网页面板：${st.title}${st.count > 1 ? ` 等 ${st.count} 个` : ""}`, E`Show the pages: ${st.title}${st.count > 1 ? ` and ${st.count - 1} more` : ""}`);
  return (
    <button type="button" className={cn("pg-toggle", st.open && "is-on")} aria-pressed={st.open} title={label} aria-label={label}
      onClick={() => window.dispatchEvent(new Event("myco-pages-toggle"))}>
      <PanelRight />
      <span>{L("网页", "Pages")}</span>
      {st.count > 1 && <span className="pg-toggle-count">{st.count}</span>}
    </button>
  );
}

// What the App knows of a page (a card asks once; the panel asks again).
const known = new Map<string, Promise<Page>>();
function pageOf(hubId: string, id: string, fresh = false): Promise<Page> {
  const k = `${hubId}/${id}`;
  if (fresh || !known.has(k)) {
    const p = appApi.page(hubId, id);
    known.set(k, p);
    p.catch(() => known.delete(k));
  }
  return known.get(k)!;
}

/** Cards for the pages a message links to. */
export function PageCards({ body }: { body: string }) {
  const links = pageLinksIn(body);
  if (!links.length) return null;
  return (
    <div className="pg-cards">
      {links.map((l) => <PageCard key={l.url} hubId={l.hubId} id={l.id} url={l.url} />)}
    </div>
  );
}

export function PageCard({ hubId, id, url }: { hubId: string; id: string; url: string }) {
  const [page, setPage] = useState<Page | null>(null);
  const [gone, setGone] = useState(false);
  const [inst, setInst] = useState(false);
  useEffect(() => { if (canInstall()) void isInstalled(hubId, id).then(setInst); }, [hubId, id]);
  const pins = useContext(PagePinsContext);
  const menu = useContextMenu();
  useEffect(() => {
    let live = true;
    pageOf(hubId, id).then((p) => live && setPage(p), () => live && setGone(true));
    return () => { live = false; };
  }, [hubId, id]);
  // Not there (deleted, not yours, an older hub): the link in the text stays.
  if (gone) return null;
  const who = page ? nameOf(hubId, page.publisher) : "";
  const about = !page ? ""
    : page.live ? L(E`${who} 电脑上的 ${page.port} 端口`, E`Port ${page.port} on ${who}'s computer`)
      : [page.version > 1 ? L(E`第 ${page.version} 版`, E`Version ${page.version}`) : "", L(E`${who} 发布`, E`by ${who}`), timeAgo(page.updated_at),
        page.editors === "managers" ? (page.read_only ? L("你只能看", "View only for you") : L("只有管理者能改", "Only managers edit")) : ""].filter(Boolean).join(" · ");
  // Right-click: open, pin it above the conversation (or unpin), copy its link.
  const pinned = !!pins?.pinned(id);
  const managersOnly = page?.editors === "managers";
  const setEditors = (editors: "everyone" | "managers") => {
    appApi.setPageEditors(hubId, id, editors).then((p) => { known.set(`${hubId}/${id}`, Promise.resolve(p)); setPage(p); }, () => undefined);
  };
  const items: MenuItem[] = [
    { label: L("打开", "Open"), icon: PanelRight, onSelect: () => openPage({ kind: "page", hubId, id, fragment: pageLinkOf(url)?.fragment }) },
    ...(pageWindows && !page?.live ? [{ label: L("在单独窗口打开", "Open in its own window"), icon: AppWindow, onSelect: () => openPageWindow(hubId, id, page?.name || "") }] : []),
    ...(canInstall() && page && !page.live ? [inst
      ? { label: L("从「应用程序」里移除", "Remove from Applications"), icon: Trash2, onSelect: () => { uninstallApp(hubId, id).then(() => setInst(false), (e) => console.error(e)); } }
      : { label: L("装成应用", "Install as app"), icon: Download, onSelect: () => { installApp(hubId, id, page.name, page.template?.id).then(() => { setInst(true); openPageWindow(hubId, id, page.name); }, (e) => console.error(e)); } }] : []),
    ...(pins ? [{ label: pinned ? L("取消置顶", "Unpin") : L("置顶到对话上方", "Pin above the chat"), icon: pinned ? PinOff : PinIcon, onSelect: () => pins.toggle({ id, url, name: page?.name || L("网页", "Page") }) }] : []),
    ...(page?.can_manage && !page.live ? [managersOnly
      ? { label: L("让能看到的人都能改", "Let everyone who sees it edit"), icon: Users, onSelect: () => setEditors("everyone") }
      : { label: L("只让管理者改", "Only managers may edit"), icon: Lock, onSelect: () => setEditors("managers") }] : []),
    "sep",
    { label: L("复制链接", "Copy the link"), icon: Copy, onSelect: () => void copyText(url) },
  ];
  return (
    <>
    <button type="button" className={cn("pg-card", pinned && "is-pinned")} data-page={id} onClick={() => openPage({ kind: "page", hubId, id, fragment: pageLinkOf(url)?.fragment })}
      onContextMenu={(e) => menu.open(e, items)}
      title={L("在旁边打开这个网页（右键：置顶、复制链接）", "Open this page beside the chat (right-click: pin, copy the link)")}>
      <span className={cn("pg-card-icon", page?.live && "is-live")}>{page?.live ? <Radio /> : <AppWindow />}</span>
      <span className="pg-card-text">
        <span className="pg-card-name">
          {page?.name || L("网页", "Page")}
          {page?.live && <span className={cn("pg-live", page.online && "is-on")}>{page.online ? L("实时 · 在线", "Live") : L("实时 · 离线", "Live · offline")}</span>}
        </span>
        <span className="pg-card-about">{page ? about : L("正在读取…", "Loading…")}</span>
      </span>
      {pinned && <PinIcon className="pg-card-pinned" aria-label={L("已置顶", "Pinned")} />}
      <span className="pg-card-open">{L("打开", "Open")}</span>
    </button>
    {menu.element}
    </>
  );
}

/** The page panel: one, for the whole App; links and cards open it. */
type Tab = { key: string; target: PageTarget; reload: number; meta: PageMeta };
/** What a tab says about itself, for the panel's head and its tab. */
export type PageMeta = { title: string; about: string; icon: "page" | "live" | "local"; copyUrl?: string; localUrl?: string; loaded: boolean; recover?: string; exportPreferences?: boolean };
const keyOf = (t: PageTarget) => (t.kind === "page" ? `${t.hubId}/${t.id}` : t.url);
const MAX_TABS = 8;

/** The page panel: one, for the whole App, with a tab for each page open
 *  in it; links and cards open it. A page already open is shown again. */
export function PageHost() {
  const [tabs, setTabs] = useState<Tab[]>([]);
  const [active, setActive] = useState("");
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const [recovery, setRecovery] = useState("");
  const [available, setAvailable] = useState<string[]>([]);
  const recoveryMenu = useContextMenu("pg-recovery-menu");
  const preferences = usePagePreferences();
  const [inst, setInst] = useState<Set<string>>(new Set());
  const refreshInstalled = () => { if (canInstall()) void installed().then(setInst); };
  useEffect(refreshInstalled, [tabs.length]);
  useEffect(() => {
    const on = (e: Event) => {
      const t = (e as CustomEvent<PageTarget>).detail;
      const k = keyOf(t);
      setTabs((list) => {
        if (list.some((x) => x.key === k)) return list.map(x => {
          if (x.key !== k || t.kind !== "page" || x.target.kind !== "page") return x;
          // A user clicking a deep link means navigate, including clicking the
          // same link after closing an app's inner view. Keep one page tab.
          return t.fragment || x.target.fragment ? { ...x, target: { ...t, routeVisit: (x.target.routeVisit || 0) + 1 } } : x;
        });
        const next = [...list, { key: k, target: t, reload: 0, meta: { title: "", about: "", icon: t.kind === "local" ? "local" : "page", loaded: false } as PageMeta }];
        // At most a few: the oldest goes (not the one shown).
        while (next.length > MAX_TABS) next.splice(next.findIndex((x) => x.key !== k), 1);
        return next;
      });
      setActive(k);
      setOpen(true);
    };
    window.addEventListener("myco-open-page", on);
    // A click on a page's link, anywhere (in a browser too, where links
    // otherwise open in a new tab).
    const click = (e: MouseEvent) => {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey) return;
      const a = (e.target as Element | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
      const t = a ? pageTargetOf(a.href) : null;
      if (!t) return;
      e.preventDefault();
      openPage(t);
    };
    document.addEventListener("click", click);
    const toggle = () => setOpen((o) => !o);
    window.addEventListener("myco-pages-toggle", toggle);
    return () => {
      window.removeEventListener("myco-open-page", on);
      document.removeEventListener("click", click);
      window.removeEventListener("myco-pages-toggle", toggle);
    };
  }, []);
  // (what the top bar's toggle shows)
  const shownTab = tabs.find((x) => x.key === active) || tabs[0];
  useEffect(() => {
    publishPanel({ count: tabs.length, title: shownTab?.meta.title || L("网页", "Page"), open: open && tabs.length > 0 });
  }, [tabs.length, shownTab?.meta.title, open]);
  // Tucked away (收起, Esc, a click beside it): the pages stay open, a
  // small button brings them back. Closed (×): they go.
  const setPanel = (o: boolean) => setOpen(o);
  const closeAll = () => { setOpen(false); setTabs([]); setActive(""); setRecovery(""); setAvailable([]); };
  const close = (k: string) => {
    const at = tabs.findIndex((x) => x.key === k);
    const rest = tabs.filter((x) => x.key !== k);
    if (!rest.length) { closeAll(); return; }
    setTabs(rest);
    if (k === active) setActive(rest[Math.min(at, rest.length - 1)].key);
  };
  const meta = useCallback((k: string, m: PageMeta) => {
    // (the same list when nothing changed: a new one renders the tabs again,
    // and they tell their meta again, round and round at full CPU)
    setTabs((list) => {
      const i = list.findIndex((x) => x.key === k);
      if (i < 0 || JSON.stringify(list[i].meta) === JSON.stringify(m)) return list;
      const next = list.slice();
      next[i] = { ...list[i], meta: m };
      return next;
    });
  }, []);
  if (!tabs.length) return null;
  const cur = tabs.find((x) => x.key === active) || tabs[0];
  const m = cur.meta;
  const recoverable = !!m.recover && available.includes(m.recover);
  const items: MenuItem[] = [];
  if (pageWindows && cur.target.kind === "page" && m.icon === "page") {
    const t = cur.target;
    items.push({ label: L("在单独窗口打开", "Open in its own window"), onSelect: () => { openPageWindow(t.hubId, t.id, m.title); close(cur.key); } });
  }
  if (canInstall() && cur.target.kind === "page" && m.icon === "page") {
    const t = cur.target;
    items.push(inst.has(`${t.hubId}/${t.id}`)
      ? { label: L("从「应用程序」里移除", "Remove from Applications"), onSelect: () => { uninstallApp(t.hubId, t.id).then(refreshInstalled, (e) => console.error(e)); } }
      : { label: L("装成应用", "Install as app"), onSelect: () => {
        void pageOf(t.hubId, t.id).then((p) => installApp(t.hubId, t.id, p.name, p.template?.id)).then(() => { refreshInstalled(); openPageWindow(t.hubId, t.id, m.title); close(cur.key); }, (e) => console.error(e));
      } });
  }
  if (recoverable) items.push({ label: recoveryLabel(), onSelect: () => setRecovery(cur.key) });
  if (m.exportPreferences && cur.target.kind === "page") {
    const previous = storeOf(storeKey(cur.target.hubId, cur.target.id));
    items.push({ label: L("导出旧页面设置", "Export older page settings"), onSelect: () => {
      const url = URL.createObjectURL(new Blob([JSON.stringify(previous, null, 2)], { type: "application/json" }));
      const link = document.createElement("a");
      link.href = url;
      link.download = "myco-older-page-settings.json";
      link.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 30000);
    } });
  }
  const copy = async () => {
    if (!m.copyUrl) return;
    await copyText(m.copyUrl);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  const tabIcon = (x: Tab) => (x.meta.icon === "live" ? <Radio /> : x.meta.icon === "local" ? <Monitor /> : <AppWindow />);
  return (
    <>
    <DockPanel open={open} onOpenChange={setPanel} onClose={closeAll} keepMounted keepOpenOn=".pg-recovery-menu, .pg-recovery-dialog"
      closeLabel={tabs.length > 1 ? L(E`关闭全部 ${tabs.length} 个网页`, E`Close all ${tabs.length} pages`) : L("关闭", "Close")}
      title={m.title || L("网页", "Page")} titleHint={recoverable ? recoveryLabel() : undefined} about={m.about} className="pg-panel" bodyClass="is-page pg-body"
      tools={(
        <>
          {items.length > 0 && <button type="button" className="app-btn app-btn-plain" aria-haspopup="menu"
            title={L("更多页面操作", "More page actions")} aria-label={L("更多页面操作", "More page actions")}
            onClick={(event) => recoveryMenu.openUnder(event.currentTarget, items)}><MoreHorizontal /></button>}
          <button type="button" className="app-btn app-btn-plain" onClick={() => setOpen(false)}
            title={L("收起（网页保留，点顶栏右边的「网页」展开）", "Tuck away (the pages stay; Pages at the top right brings them back)")} aria-label={L("收起", "Tuck away")}><PanelRightClose /></button>
          <button type="button" className="app-btn app-btn-plain" onClick={() => setTabs((list) => list.map((x) => (x.key === cur.key ? { ...x, reload: x.reload + 1 } : x)))} disabled={!m.loaded}
            title={L("刷新", "Reload")} aria-label={L("刷新", "Reload")}><RotateCw /></button>
          {m.localUrl ? (
            <button type="button" className="app-btn app-btn-plain" onClick={() => openOutside(m.localUrl || "")}
              title={L("用浏览器打开", "Open in the browser")} aria-label={L("用浏览器打开", "Open in the browser")}><ExternalLink /></button>
          ) : (
            <button type="button" className="app-btn app-btn-plain" onClick={() => void copy()} disabled={!m.copyUrl}
              title={copied ? L("已复制链接", "Link copied") : L("复制链接", "Copy link")} aria-label={copied ? L("已复制链接", "Link copied") : L("复制链接", "Copy link")}>
              {copied ? <Check /> : <Link2 />}
            </button>
          )}
        </>
      )}>
      {tabs.length > 1 && (
        <div className="pg-tabs" role="tablist" aria-label={L("打开的网页", "Open pages")}>
          {tabs.map((x) => (
            <div key={x.key} className={cn("pg-tab", x.key === cur.key && "is-on")} title={x.meta.title}
              onMouseDown={(e) => { if (e.button === 1) { e.preventDefault(); close(x.key); } }}>
              <button type="button" role="tab" aria-selected={x.key === cur.key} className="pg-tab-main" onClick={() => setActive(x.key)}>
                <span className="pg-tab-icon">{tabIcon(x)}</span>
                <span className="pg-tab-name">{x.meta.title || L("正在打开…", "Opening…")}</span>
              </button>
              <button type="button" className="pg-tab-close" onClick={() => close(x.key)}
                title={L("关闭这个标签", "Close this tab")} aria-label={L(E`关闭 ${x.meta.title || "这个标签"}`, E`Close ${x.meta.title || "this tab"}`)}><X /></button>
            </div>
          ))}
        </div>
      )}
      {tabs.map((x) => (
        <PageView key={x.key} target={x.target} open={open} active={x.key === cur.key} reload={x.reload} onMeta={(mm) => meta(x.key, mm)} />
      ))}
    </DockPanel>
    {recoveryMenu.element}
    <PageRecovery enabled={open && !!m.recover} template={m.recover} language={preferences.language}
      showing={open && recovery === cur.key} onClose={() => setRecovery("")} onAvailable={setAvailable} />
    </>
  );
}

// Whether this App opens pages in windows of their own (the desktop App).
let pageWindows = false;
export function setPageWindows(on: boolean) { pageWindows = on; }
/** Opens a page in a window of its own (the desktop App). */
export function openPageWindow(hubId: string, id: string, name: string) {
  void appApi.openPageWindow(hubId, id, name).catch(() => openPage({ kind: "page", hubId, id }));
}

/** A page alone, filling a window of its own (PageWindow.tsx). */
export function PageWindowView({ target, onMeta }: { target: PageTarget; onMeta: (m: PageMeta) => void }) {
  return <PageView target={target} open active reload={0} onMeta={onMeta} />;
}

/** One page in the panel (a tab): its frame, its bridge, its notices. It
 *  stays loaded while another tab is shown. */
function PageView({ target, open, active, reload, onMeta }: {
  target: PageTarget; open: boolean; active: boolean; reload: number; onMeta: (m: PageMeta) => void;
}) {
  const preferences = usePagePreferences();
  const [opened, setOpened] = useState<PageOpen | null>(null);
  const [page, setPage] = useState<Page | null>(null);
  const [error, setError] = useState("");
  const frame = useRef<HTMLIFrameElement>(null);
  const [loadedKey, setLoadedKey] = useState("");
  const key = target.kind === "page" ? `${target.hubId}/${target.id}` : target.url;

  const loadGeneration = useRef(0);
  useEffect(() => () => { loadGeneration.current++; }, [key]);
  const lease = opened?.lease;
  useEffect(() => {
    if (target.kind !== "page" || !lease) return;
    const { hubId, id } = target;
    return () => { void appApi.closePage(hubId, id, lease).catch(() => undefined); };
  }, [key, lease]);

  const load = async (version?: number) => {
    if (target.kind !== "page") return;
    const loadID = ++loadGeneration.current;
    setError("");
    try {
      const p = await pageOf(target.hubId, target.id, true);
      if (loadGeneration.current !== loadID) return;
      const o = await appApi.openPage(target.hubId, target.id, version);
      if (loadGeneration.current !== loadID) {
        if (o.lease) void appApi.closePage(target.hubId, target.id, o.lease).catch(() => undefined);
        return;
      }
      if (p.template?.requires_isolation && !o.isolated) {
        if (o.lease) void appApi.closePage(target.hubId, target.id, o.lease).catch(() => undefined);
        setOpened(null); setPage(p);
        setError(L("这个应用需要独立的页面存储，请升级 myco App。", "This app requires isolated page storage. Update the myco App."));
        return;
      }
      setOpened(o); setPage(p);
    } catch (e) {
      if (loadGeneration.current !== loadID) return;
      setOpened(null);
      setError(e instanceof ApiError && e.code === "page_origin_unavailable"
        ? L("这个页面正在另一个 myco App 里打开，请先退出另一个 App 再试。", "This page is open in another myco App. Quit that App, then try again.")
        : isOldHub(e) || (e instanceof ApiError && /not enabled/.test(e.message))
        ? L("这个工作区的 Hub 还不支持网页：在服务器上运行 sudo myco upgrade，升级到 v0.19.24 或更新。", "This workspace's hub has no pages yet: upgrade it to v0.19.24 or later (sudo myco upgrade on the server).")
        : e instanceof ApiError && e.status === 404
          ? L("这个网页已经删除了，或者你没有权限看。", "This page was deleted, or you may not see it.")
          : (e as Error).message || L("打不开这个网页", "Could not open this page"));
    }
  };
  // Once for its tab (tucked away and back, it is as it was).
  useEffect(() => {
    setOpened(null); setPage(null);
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  // A newer version: said at the top; the page you look at stays as it is.
  useEffect(() => {
    if (!open || !active || target.kind !== "page") return;
    const t = window.setInterval(() => { pageOf(target.hubId, target.id, true).then(setPage, () => undefined); }, 15_000);
    return () => window.clearInterval(t);
  }, [open, active, key]);

  // The page's script (pagebridge.js) asks the App: a link to open in the
  // browser, and the myco bridge's calls. Only from this panel's frame;
  // answers go to the page's own origin only.
  const live = useRef({ page, opened, preferences });
  live.current = { page, opened, preferences };
  const known = useRef(0); // the state version the page has
  const bridged = useRef(false);
  // Its records (myco.data): watched once the page listens; the change feed
  // from where it last read.
  const dataWatch = useRef(false);
  const dataSeq = useRef(0);
  const [asking, setAsking] = useState<{ text: string; to: string; answer: (ok: boolean) => void } | null>(null);
  // a page wanting a group of the computer's capabilities the person has not allowed yet
  const [consent, setConsent] = useState<(OsConsent & { name: string; answer: (a: OsAnswer) => void }) | null>(null);
  const [remoteAsking, setRemoteAsking] = useState<{text:string; title?:string; review?:BrowserApprovalReview; settingsReview?:BrowserSettingsReview; answer:(ok:boolean)=>void} | null>(null);
  const [settingsRead, setSettingsRead] = useState<BrowserSettingsReview | null>(null);
  const remoteConsent = useRef<((ok:boolean)=>void) | null>(null);
  // (while the page is there: tucked away, it still runs and is answered,
  // and what it started on the computer goes on)
  useEffect(() => {
    known.current = 0;
    bridged.current = false;
    let remoteUsed = false;
    const remoteInstance = Array.from(crypto.getRandomValues(new Uint8Array(16)), (n) => n.toString(16).padStart(2, "0")).join("");
    const on = (e: MessageEvent) => {
      const win = frame.current?.contentWindow;
      if (!win || e.source !== win) return;
      const d = e.data as { myco?: string; url?: string; id?: number; method?: string; args?: Record<string, unknown> } | null;
      const origin = (() => { try { return new URL(frame.current?.src || "").origin; } catch { return ""; } })();
      if (!origin || e.origin !== origin) return;
      if (d?.myco === "open-url" && typeof d.url === "string" && /^(https?:|mailto:)/i.test(d.url)) { openOutside(d.url); return; }
      if (d?.myco !== "call" || typeof d.id !== "number") return;
      const reply = (value: unknown, error?: string, code?: string, latest?: unknown, reason?: string) => {
        if (closed) return;
        win.postMessage(error ? { myco: "result", id: d.id, error, code, ...(latest ? { latest } : {}), ...(reason ? { reason } : {}) } : { myco: "result", id: d.id, value }, origin);
      };
      if (target.kind !== "page") { reply(null, L("这个网页不在 myco 上，没有桥", "This page is not on myco: no bridge"), "not_a_page"); return; }
      void answer(target, d.method || "", d.args || {}).then((v) => reply(v), (err: unknown) => reply(null, (err as Error).message || "error", (err as ApiError).code, (err as ApiError).latest, (err as ApiError).reason));
    };
    // The page's records (hubs from v0.20.5) and files it saves.
    const answerData = async (t: { hubId: string; id: string }, method: string, args: Record<string, unknown>): Promise<unknown> => {
      const coll = String(args.collection || "");
      switch (method) {
        case "data.list": {
          const query = args.query as import("@/api").PageRecordQuery | undefined;
          const r = await appApi.pageRecords(t.hubId, t.id, coll || undefined, query);
          // Partial/count reads must not skip changes the editing Store has
          // not consumed. Only the historical full list initializes watch.
          if (query === undefined) dataSeq.current = Math.max(dataSeq.current, r.seq);
          return r;
        }
        case "data.get":
          return appApi.pageRecord(t.hubId, t.id, coll, String(args.id || ""));
        case "data.put":
          return appApi.putPageRecord(t.hubId, t.id, coll, String(args.id || ""), args.data ?? {},
            args.ifVersion === null || args.ifVersion === undefined ? null : Number(args.ifVersion));
        case "data.remove":
          await appApi.deletePageRecord(t.hubId, t.id, coll, String(args.id || ""), Number(args.ifVersion) || 0);
          return true;
        case "data.watch":
          if (!dataSeq.current) dataSeq.current = (await appApi.pageRecords(t.hubId, t.id)).seq;
          dataWatch.current = true;
          return true;
        case "download": {
          undeclared("download");
          // a Blob from the page (its bridge since the video app), or base64 (older pages' bridge)
          if (args.blob instanceof Blob) return appApi.pageDownload(String(args.name || ""), args.blob, String(args.type || ""));
          const bin = atob(String(args.base64 || ""));
          const bytes = new Uint8Array(bin.length);
          for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
          return appApi.pageDownload(String(args.name || ""), bytes, String(args.type || ""));
        }
      }
      throw new Error(`myco: no such call: ${method}`);
    };
    const answer = async (t: { hubId: string; id: string }, method: string, args: Record<string, unknown>): Promise<unknown> => {
      const { page: p, opened: o } = live.current;
      const legacyFilesReadOnly = !!projectFileScope(p) && !pageFileScope(p);
      if (legacyFilesReadOnly && ["data.put", "data.remove", "update"].includes(method)) throw pageFilesUpgrade();
      switch (method) {
        case "ready": {
          const st = await appApi.pageState(t.hubId, t.id);
          known.current = st.version;
          bridged.current = true;
          const projectFiles = !!projectFileScope(p);
          // (can_edit: false when only those who manage it change it, and not you)
          return { page: { id: t.id, url: p?.url || "", name: o?.name || p?.name || "", version: o?.version || 0 }, me: { name: meOf(t.hubId), can_edit: !p?.read_only && !legacyFilesReadOnly, can_manage: !!p?.can_manage }, state: st.state, version: st.version, preferences: live.current.preferences, isolation: o?.isolated === true, pageFiles: !!pageFileScope(p), projectFiles, projectMedia: projectFiles };
        }
        case "update": {
          const r = await appApi.putPageState(t.hubId, t.id, args.state ?? null, Number(args.version) || 0);
          known.current = Math.max(known.current, r.version);
          return { ok: r.ok, state: r.state, version: r.version };
        }
        case "send": {
          undeclared("send");
          const text = String(args.text || "").trim();
          if (!text) throw new Error(L("消息是空的", "The message is empty"));
          const to = p?.publisher || "";
          if (!to) throw new Error(L("不知道这个网页是谁发布的", "Who published this page is not known"));
          const k = `${t.hubId}/${t.id}`;
          const recent = (sends.get(k) || []).filter((at) => Date.now() - at < 60_000);
          if (recent.length >= 5) throw Object.assign(new Error(L("发得太快了：一分钟最多 5 条", "Too many: at most 5 a minute")), { code: "rate_limited" });
          if (!allowedSend.has(k)) {
            const ok = await new Promise<boolean>((resolve) => setAsking({ text, to: nameOf(t.hubId, to), answer: resolve }));
            setAsking(null);
            if (!ok) throw Object.assign(new Error(L("你没有允许这个网页发消息", "You did not let this page send messages")), { code: "denied" });
            allowedSend.add(k);
          }
          sends.set(k, [...recent, Date.now()]);
          const name = o?.name || p?.name || L("网页", "a page");
          const m = await appApi.sendMessage(t.hubId, {
            to, body: `${text}\n\n${L(E`（来自网页「${name}」${p?.url || ""}，状态第 ${known.current} 版）`, E`(from the page "${name}" ${p?.url || ""}, state version ${known.current})`)}`,
            ...(p?.project ? { project: p.project } : {}),
          });
          return { id: m.id };
        }
        case "storage.get":
        case "storage.set": {
          const storageKey = o?.storage_scope ? `myco-page-store-isolated:${o.storage_scope}` : storeKey(t.hubId, t.id);
          const all = storeOf(storageKey);
          if (method === "storage.get") return all[String(args.key)] ?? null;
          if (args.value === null || args.value === undefined) delete all[String(args.key)];
          else all[String(args.key)] = args.value;
          const raw = JSON.stringify(all);
          if (raw.length > 1 << 20) throw Object.assign(new Error(L("这个网页的存储满了（1 MB）", "This page's storage is full (1 MB)")), { code: "too_large" });
          try { localStorage.setItem(storageKey, raw); } catch { throw new Error(L("存不下", "Could not store it")); }
          return true;
        }
      }
      if (method.startsWith("pageFiles.")) return answerPageFiles(t.hubId, p, method, args);
      if (method.startsWith("projectFiles.")) return answerProjectFiles(t.hubId, p, method, args);
      if (method.startsWith("data.") || method === "download") return answerData(t, method, args);
      if (method === "browser.call") {
        const action = String(args.action || "");
        const input = (args.args && typeof args.args === "object" ? args.args : {}) as Record<string, unknown>;
        const who = { hub: t.hubId, id: t.id, name: o?.name || p?.name || "" };
        if (action === "install" || action === "approvals.request" || action === "profiles.configure" || action === "profiles.remove" || (action === "profiles.allowAgents" && input.allowed === true)) {
          let review: BrowserApprovalReview | undefined;
          let settingsReview: BrowserSettingsReview | undefined;
          let confirmedInput = JSON.parse(JSON.stringify(input)) as Record<string,unknown>;
          let text = L("安装浏览器组件，用 myco 专用的浏览器资料录制和回放网页操作。它不使用你日常的 Chrome 资料，也不会自动开放 agent 控制。", "Install the browser component to record and replay web actions in a dedicated myco browser profile. It does not use your everyday Chrome profile or enable agent control automatically.");
          if (action === "profiles.allowAgents") {
            const status = await withConsent(who, () => appApi.browserCall<BrowserStatus>(t.hubId, t.id, remoteInstance, "status"));
            const profile = status.profiles.find(item => item.id === String(input.id || ""));
            if (!profile) throw Object.assign(new Error(L("找不到这个浏览器资料", "Browser profile not found")), { code: "not_found" });
            text = L(E`允许 agent 在当前工作区申请使用「${profile.name}」浏览器资料？它们仍需你批准短期授权，并且只能运行你在本机批准的脚本版本。`, E`Allow agents in this workspace to request access to “${profile.name}”? They still need your approval for a short-lived grant and can run only script versions you approved on this computer.`);
          }
          if (action === "profiles.remove") {
            const status = await withConsent(who, () => appApi.browserCall<BrowserStatus>(t.hubId,t.id,remoteInstance,"status"));
            const profile = status.profiles.find(item=>item.id===String(confirmedInput.id||""));
            if (!profile) throw Object.assign(new Error(L("找不到这个浏览器资料","Browser profile not found")),{code:"not_found"});
            if (profile.running) throw Object.assign(new Error(L("请先关闭这个 profile 的浏览器窗口或停止运行","Close this profile’s browser window or stop its run first")),{code:"busy"});
            if (!status.features?.includes("profile_delete")) throw Object.assign(new Error(L("请先更新浏览器组件","Update the browser component first")),{code:"unsupported"});
            text=L(E`永久删除「${profile.name}」？这会删除它的登录状态、本机下载和所有工作区中的脚本授权，不能恢复。`,E`Permanently delete “${profile.name}”? This removes its sign-ins, local downloads and script approvals in every workspace. This cannot be undone.`);
          }
          if (action === "profiles.configure") {
            const status = await withConsent(who, () => appApi.browserCall<BrowserStatus>(t.hubId, t.id, remoteInstance, "status"));
            const profile = status.profiles.find(item => item.id === String(confirmedInput.id || ""));
            if (!profile) throw Object.assign(new Error(L("找不到这个浏览器资料", "Browser profile not found")), {code:"not_found"});
            if (!profile.settings) throw Object.assign(new Error(L("请先更新浏览器组件", "Update the browser component first")), {code:"unsupported"});
            settingsReview = browserSettingsReview(profile, confirmedInput.settings as BrowserProfileSettingsInput);
            text = L("更改这个浏览器资料的网络与启动设置？", "Change this browser profile’s network and launch settings?");
          }
          if (action === "approvals.request") {
            review = await withConsent(who, () => appApi.browserCall<BrowserApprovalReview>(t.hubId, t.id, remoteInstance, "approvals.review", input));
            confirmedInput = {script_id: review.script_id, version: review.version, profile: review.profile, script: review.script, scripts: review.scripts};
            text = L("允许 agent 使用此浏览器资料的登录状态运行这个脚本版本？批准会结束该资料现有的 agent 授权；后续运行仍需你批准新的短期授权。", "Allow agents to run this script version using this profile’s sign-ins? Approving ends its current agent grants. Future runs still require a new short-lived grant approved by you.");
          }
          if (closed) throw Object.assign(new Error("cancelled"), { code: "cancelled" });
          if (remoteConsent.current) throw Object.assign(new Error(L("请先处理当前的授权请求", "Finish the current permission request first")), { code: "busy" });
          const accepted = await new Promise<boolean>(resolve => {
            remoteConsent.current = resolve;
            setRemoteAsking({ text, title: L("浏览器自动化授权", "Browser automation permission"), review, settingsReview, answer: resolve });
          });
          remoteConsent.current = null;
          setRemoteAsking(null);
          if (!accepted || closed) throw Object.assign(new Error(L("已取消", "Cancelled")), { code: "cancelled" });
          // Minted after the host modal, never supplied by or returned to the child.
          const approved = await appApi.browserConfirm(t.hubId, t.id, remoteInstance, action, confirmedInput);
          if (closed) throw Object.assign(new Error("cancelled"), { code: "cancelled" });
          return withConsent(who, () => appApi.browserCall(t.hubId, t.id, remoteInstance, action, confirmedInput, approved.confirmation));
        }
        const result = await withConsent(who, () => appApi.browserCall(t.hubId, t.id, remoteInstance, action, input));
        if (closed) throw Object.assign(new Error("cancelled"), { code: "cancelled" });
        return result;
      }
      if (method === "remote.call") {
        const action = String(args.action || "");
        const remoteArgs = (args.args || {}) as Record<string, unknown>;
        const explanations: Record<string, string> = {
          install: L("下载并安装可选远控组件。它不会随 App 自动启动，不会自动开放远程连接。", "Download the optional remote component. It will not start with the App or enable incoming connections automatically."),
          remove: L("卸载本机远控组件，并停止本机正在共享的画面。", "Uninstall the remote component and stop sharing this computer."),
          enable: remoteArgs.control ? L("允许配对成功的连接查看画面、操作键盘和鼠标。请只把配对码交给你信任的人；关闭此应用即停止共享。", "Let paired connections see this screen and use its keyboard and mouse. Share the pairing code only with someone you trust. Closing this app stops sharing.") : L("允许配对成功的连接查看这台电脑的画面。不能操作键鼠；关闭此应用即停止共享。", "Let paired connections view this screen, without keyboard or mouse control. Closing this app stops sharing."),
          connect: L("允许此应用建立远程桌面连接。远端画面只在当前应用中显示。", "Let this app establish a remote desktop connection and show the screen here."),
        };
        if (action === "connect") {
          const devices = await appApi.remoteCall<{machine:string;name:string}[]>(t.hubId,t.id,"devices",{},remoteInstance);
          if(closed) throw new Error("cancelled");
          const device = devices.find(d=>d.machine===String(remoteArgs.machine||""));
          if(!device) throw new Error(L("这台电脑已停止共享，请刷新后重试", "This computer stopped sharing. Refresh and try again."));
          const mode = remoteArgs.offer && (remoteArgs.offer as {control?:boolean}).control;
          explanations.connect = mode
            ? L(E`连接到「${device.name}」，查看它的画面并操作键盘和鼠标。`, E`Connect to “${device.name}”, view its screen and use its keyboard and mouse.`)
            : L(E`连接到「${device.name}」，仅查看画面，不操作键鼠。`, E`Connect to “${device.name}” to view its screen, without keyboard or mouse control.`);
        }
        if (explanations[action]) {
          if (remoteConsent.current) throw new Error(L("请先处理当前的授权请求", "Finish the current permission request first"));
          const ok = await new Promise<boolean>((resolve) => {
            remoteConsent.current = resolve;
            setRemoteAsking({text:explanations[action], answer:resolve});
          });
          remoteConsent.current = null;setRemoteAsking(null);
          if (!ok || closed) throw Object.assign(new Error(L("你没有允许这个操作", "You did not allow this action")), {code:"permission_denied"});
        }
        if (action === "enable" || action === "connect") remoteUsed = true;
        return appApi.remoteCall(t.hubId, t.id, action, remoteArgs, remoteInstance);
      }
      if (method.startsWith("os.")) return answerOS(t, method, args);
      throw new Error(`myco: no such call: ${method}`);
    };
    // What the page uses of this computer (myco.os): calls go to the App
    // backend as they are; a job's events are read there and sent on to
    // the page until it ends. Closing the page stops its jobs.
    const jobs = new Set<string>();
    let closed = false;
    const toPage = (msg: Record<string, unknown>) => {
      const win = frame.current?.contentWindow;
      const origin = (() => { try { return new URL(frame.current?.src || "").origin; } catch { return ""; } })();
      if (win && origin) win.postMessage(msg, origin);
    };
    // What an app does that its myco.json does not declare (permissions:
    // send, download): allowed for now, with a warning in the page's
    // console (once each); a later App refuses it.
    const warned = new Set<string>();
    const undeclared = (perm: "send" | "download") => {
      const tp = live.current.page?.template;
      if (!tp || (tp.permissions || []).includes(perm)) return;
      // From hubs that say so, what an app does not declare it may not use
      // (they do not for pages of myco's own apps kept from before declarations).
      if (tp.declares)
        throw Object.assign(new Error(L(E`这个应用没有在 myco.json 的 "permissions" 里写 "${perm}"，不能用`, E`This app does not declare "${perm}" in myco.json's "permissions"`)), { code: "permission_denied" });
      if (warned.has(perm)) return;
      warned.add(perm);
      toPage({ myco: "warn", text: `myco: this app uses ${perm} without declaring it in myco.json ("permissions": ["${perm}"]); a later myco App will refuse it` });
    };
    const pump = async (id: string) => {
      let after = 0, fails = 0;
      while (!closed) {
        try {
          const b = await appApi.osJob(id, after, 15_000);
          fails = 0;
          after = b.next;
          if ((b.events && b.events.length) || b.done) {
            toPage({ myco: "os.event", job: id, events: b.events || [], dropped: !!b.dropped, done: b.done, value: b.value, error: b.error?.message, code: b.error?.code });
          }
          if (b.done) break;
        } catch (e) {
          // Gone (ended over 5 minutes ago, or the App restarted): say so.
          // Anything else (the App asleep, a moment offline) passes: the
          // job goes on in the App, so read it again, more slowly.
          const code = (e as ApiError).code;
          if (code === "not_found") {
            toPage({ myco: "os.event", job: id, events: [], done: true, error: (e as Error).message || "the job is gone", code });
            break;
          }
          fails++;
          await new Promise((r) => window.setTimeout(r, Math.min(5000, 500 * fails)));
        }
      }
      jobs.delete(id);
    };
    // The person's say before a page uses the computer (consent_required):
    // one question at a time; calls waiting on the same thing share it.
    const asks = new Map<string, Promise<OsAnswer>>();
    let askChain: Promise<unknown> = Promise.resolve();
    const withConsent = async <T,>(who: { hub: string; id: string; name: string }, run: () => Promise<T>): Promise<T> => {
      try {
        return await run();
      } catch (err) {
        const c = (err as ApiError).consent;
        if (!c || closed) throw err;
        const k = `${c.group}/${c.host || ""}`;
        let a = asks.get(k);
        if (!a) {
          a = (askChain = askChain.then(() => new Promise<OsAnswer>((resolve) => setConsent({ ...c, name: who.name, answer: resolve }))))
            .then(async (answer) => {
              setConsent(null);
              await appApi.osGrant(who, c.group, answer as OsAnswer, c.host);
              return answer as OsAnswer;
            })
            .finally(() => asks.delete(k)) as Promise<OsAnswer>;
          asks.set(k, a);
        }
        if ((await a) === "deny") throw Object.assign(new Error(L("你没有允许这个应用这样用这台电脑", "You did not let this app do that on this computer")), { code: "permission_denied" });
        return run();
      }
    };
    const answerOS = async (t: { hubId: string; id: string }, method: string, args: Record<string, unknown>): Promise<unknown> => {
      const { page: p, opened: o } = live.current;
      const who = { hub: t.hubId, id: t.id, name: o?.name || p?.name || "" };
      switch (method) {
        case "os.list":
          return appApi.osList();
        case "os.call":
          return withConsent(who, () => appApi.osCall(String(args.name || ""), args.args ?? {}, who));
        case "os.start": {
          const id = await withConsent(who, () => appApi.osStart(String(args.name || ""), args.args ?? {}, who));
          jobs.add(id);
          // (after the page has the id: its job is waiting for these)
          window.setTimeout(() => void pump(id), 0);
          return { id };
        }
        case "os.input":
          return appApi.osInput(String(args.job || ""), String(args.data ?? ""), !!args.close);
        case "os.cancel":
          return appApi.osCancel(String(args.job || ""));
      }
      throw new Error(`myco: no such call: ${method}`);
    };
    window.addEventListener("message", on);
    return () => {
      window.removeEventListener("message", on);
      closed = true;
      remoteConsent.current?.(false);remoteConsent.current = null;
      const t = target.kind === "page" ? target : null;
      if (t) void appApi.browserCall(t.hubId, t.id, remoteInstance, "close").catch(() => undefined);
      if (t && remoteUsed) void appApi.remoteCall(t.hubId, t.id, "close", {}, remoteInstance).catch(() => undefined);
      for (const id of jobs) void appApi.osCancel(id).catch(() => undefined);
    };
  }, [key, opened?.url, reload, preferences.language]);

  // While it is shown: what others (an agent, another person) change comes
  // to the page.
  useEffect(() => {
    if (!open || !active || target.kind !== "page") return;
    const t = window.setInterval(() => {
      if (!bridged.current) return;
      appApi.pageState(target.hubId, target.id).then((st) => {
        if (st.version <= known.current) return;
        known.current = st.version;
        const win = frame.current?.contentWindow;
        const origin = (() => { try { return new URL(frame.current?.src || "").origin; } catch { return ""; } })();
        if (win && origin) win.postMessage({ myco: "change", state: st.state, version: st.version, by: st.updated_by ? nameOf(target.hubId, st.updated_by) : "" }, origin);
      }, () => undefined);
    }, 2000);
    return () => window.clearInterval(t);
  }, [open, active, key]);
  // What others changed in its records, once it listens (myco.data.on).
  useEffect(() => {
    if (!open || !active || target.kind !== "page") return;
    let busy = false;
    const t = window.setInterval(() => {
      if (!dataWatch.current || busy) return;
      busy = true;
      appApi.pageChanges(target.hubId, target.id, dataSeq.current).then((ch) => {
        dataSeq.current = Math.max(dataSeq.current, ch.seq);
        if (!ch.changes.length && !ch.reset) return;
        const win = frame.current?.contentWindow;
        const origin = (() => { try { return new URL(frame.current?.src || "").origin; } catch { return ""; } })();
        if (win && origin) win.postMessage({ myco: "data", changes: ch.changes.map((c) => ({ ...c, by: c.by ? nameOf(target.hubId, c.by) : "" })), reset: !!ch.reset }, origin);
      }, () => undefined).finally(() => { busy = false; });
    }, 1500);
    return () => window.clearInterval(t);
  }, [open, active, key]);

  const local = target.kind === "local" ? new URL(target.url) : null;
  // Keep an explicit empty fragment for the root view: navigating an iframe
  // to a URL without any fragment would reload its document.
  const rawSrc = local ? target.kind === "local" && target.url : opened?.url ? pageFragmentURL(opened.url, target.kind === "page" ? target.fragment || "#" : undefined) : undefined;
  const src = rawSrc && !local ? localizedPageURL(rawSrc, preferences.language) : rawSrc;
  // A route is same-document navigation. It must not recreate the iframe or
  // cancel its active jobs. A repeated link also notifies opted-in templates.
  const frameIdentity = src && target.kind === "page" ? pageFragmentURL(src) : src;
  const frameKey = `${frameIdentity}#${reload}`;
  const lastRoute = useRef<string | undefined>(undefined);
  const lastRouteVisit = useRef(0);
  useEffect(() => {
    if (target.kind !== "page") return;
    const fragment = target.fragment || "";
    if (target.routeVisit && target.routeVisit !== lastRouteVisit.current && lastRoute.current === fragment && loadedKey === frameKey && frame.current?.contentWindow && src)
      frame.current.contentWindow.postMessage({myco:"page-route",fragment},new URL(src).origin);
    lastRoute.current = fragment;
    lastRouteVisit.current = target.routeVisit || 0;
  }, [target, src, frameKey, loadedKey]);
  const sendPreferences = () => {
    if (!src || !frame.current?.contentWindow) return;
    frame.current.contentWindow.postMessage({myco:"preferences", preferences},new URL(src).origin);
  };
  useEffect(sendPreferences,[preferences.language,preferences.theme,src]);
  const title = local ? L(E`这台电脑 · ${local.port || (local.protocol === "https:" ? "443" : "80")} 端口`, E`This computer · port ${local.port || 80}`)
    : opened?.name || page?.name || L("网页", "Page");
  const about = local ? local.host + local.pathname.replace(/\/$/, "")
    : opened?.live && target.kind === "page" ? L(E`实时 · ${page ? nameOf(target.hubId, page.publisher) : ""} 电脑上的 ${opened.port || page?.port || ""} 端口`, E`Live · port ${opened.port || page?.port || ""} on ${page ? nameOf(target.hubId, page.publisher) : ""}'s computer`)
    : opened ? [opened.latest > 1 || opened.version > 1 ? L(E`第 ${opened.version} 版`, E`Version ${opened.version}`) : "", page && target.kind === "page" ? L(E`${nameOf(target.hubId, page.publisher)} 发布`, E`by ${nameOf(target.hubId, page.publisher)}`) : "", page ? timeAgo(page.versions.find((v) => v.n === opened.version)?.at || page.updated_at) : ""].filter(Boolean).join(" · ")
      : "";
  const newer = opened && page && page.version > opened.version ? page.version : 0;
  // Until the page has loaded: a loading scene over it, not a blank frame.
  const [upgrading, setUpgrading] = useState(false);
  const oldPreferences = !!opened?.storage_scope && target.kind === "page" && Object.keys(storeOf(storeKey(target.hubId, target.id))).length > 0;
  const template = page?.template?.id;
  const recover = opened?.recovery && loadedKey === frameKey && canRecoverTemplate(template) ? template : undefined;
  const icon: PageMeta["icon"] = local ? "local" : opened?.live || page?.live ? "live" : "page";
  useEffect(() => {
    onMeta({ title, about, icon, copyUrl: page?.url ? pageFragmentURL(page.url, target.kind === "page" ? target.fragment : undefined) : undefined, localUrl: target.kind === "local" ? target.url : undefined, loaded: !!src, recover, exportPreferences: oldPreferences });
  }, [title, about, icon, page?.url, local, target, src, recover, oldPreferences, onMeta]);
  return (
    <div className={cn("pg-view", !active && "is-hidden")} role="tabpanel" aria-hidden={!active}>
      {newer > 0 && (
        <div className="pg-new" role="status">
          <span>{L(E`有新版本：第 ${newer} 版`, E`A new version: version ${newer}`)}</span>
          <button type="button" className="app-btn app-btn-small" onClick={() => void load()}>{L("看新版本", "Show it")}</button>
        </div>
      )}
      {!newer && page?.template?.latest && page.can_manage && target.kind === "page" && (
        <div className="pg-new" role="status">
          <span>{L(E`「${page.template.name}」有新版本 ${page.template.latest}（现在是 ${page.template.version}），升级后数据不变`, E`${page.template.name} has a new version ${page.template.latest} (this is ${page.template.version}); your data stays`)}</span>
          <button type="button" className="app-btn app-btn-small" disabled={upgrading} onClick={() => {
            setUpgrading(true);
            appApi.upgradePage(target.hubId, target.id).then(() => load(), (e) => setError(e instanceof Error ? e.message : String(e))).finally(() => setUpgrading(false));
          }}>{upgrading ? L("正在升级…", "Upgrading…") : L("升级", "Upgrade")}</button>
        </div>
      )}
      {projectFileScope(page) && !pageFileScope(page) && <div className="pg-local-note" role="status">{L("当前只能查看。请更新工作区后使用页面附件。", "View only. Update this workspace to use page attachments.")}</div>}
      {local && <div className="pg-local-note"><Monitor />{L("这台电脑上正在运行的网页。是空白的话，可能这个端口没有在跑。", "A server running on this computer. Blank? Nothing may be running on this port.")}</div>}
      {error ? <p className="fp-note">{error}</p> : (
        <div className="pg-stage">
          {/* (named by aria-label, not title: WebKit shows a frame's title as a tooltip over all of the page) */}
          {src && <iframe ref={frame} key={frameKey} className={cn("fp-frame pg-frame", loadedKey !== frameKey && "is-loading")} aria-label={title} src={src || undefined} style={{colorScheme:preferences.theme}}
            sandbox="allow-scripts allow-forms allow-popups allow-modals allow-downloads allow-same-origin" referrerPolicy="no-referrer"
            allow="clipboard-write; fullscreen" onLoad={() => { setLoadedKey(frameKey); sendPreferences(); }} />}
          {loadedKey !== frameKey && (
            <PageLoading key={`loading:${frameKey}`} name={title} seed={page?.publisher || (target.kind === "local" ? target.url : target.hubId)} who={page && target.kind === "page" ? nameOf(target.hubId, page.publisher) : ""} />
          )}
        </div>
      )}
    <Modal className={cn("pg-remote-ask", remoteAsking?.settingsReview && "pg-browser-settings-dialog")} wide={!!remoteAsking?.review || !!remoteAsking?.settingsReview} open={!!remoteAsking} onOpenChange={(o) => {if (!o) remoteAsking?.answer(false);}}
      title={remoteAsking?.title || L(E`允许「${opened?.name || page?.name || "此应用"}」使用远程桌面？`, E`Allow “${opened?.name || page?.name || "this app"}” to use Remote Desktop?`)} description={remoteAsking?.text || ""}>
      {remoteAsking?.review && <BrowserApprovalDetails review={remoteAsking.review} />}
      {remoteAsking?.settingsReview && <BrowserProfileSettingsDetails review={remoteAsking.settingsReview} onRead={setSettingsRead} />}
      {remoteAsking?.settingsReview && <p className="pg-browser-settings-hint" style={{visibility: settingsRead === remoteAsking.settingsReview ? "hidden" : undefined}} role="status">{L("请滚动查看完整设置，再允许。", "Scroll through all settings before allowing.")}</p>}
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => remoteAsking?.answer(false)}>{L("不允许", "Don't allow")}</button>
        <button type="button" className="app-btn app-btn-primary" disabled={!!remoteAsking?.settingsReview && settingsRead !== remoteAsking.settingsReview} onClick={() => remoteAsking?.answer(true)}>{L("允许", "Allow")}</button>
      </div>
    </Modal>
    <Modal open={!!consent} onOpenChange={(o) => { if (!o) consent?.answer("deny"); }} className="pg-ask"
      title={consent ? L(E`允许「${consent.name || "这个应用"}」${osWant(consent.group, consent.host).zh}吗？`, E`Let "${consent.name || "this app"}" ${osWant(consent.group, consent.host).en}?`) : ""}
      description={consent ? [
        consent.declared ? "" : L("这个应用没有在说明里声明要用这一项。", "This app does not declare that it uses this."),
        consent.once_only ? osOnceWhy(consent.group) : L("选「始终允许」，以后这个应用不再问（升级后也不问，只问新版本新加的）；可以在设置 → 应用权限里收回。", "With \"Always allow\", this app won't ask again, not even after an update (only for what a new version adds). You can take it back in Settings → App permissions."),
      ].filter(Boolean).join(" ") : ""}>
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => consent?.answer("deny")}>{L("不允许", "Don't allow")}</button>
        {consent && !consent.once_only && <button type="button" className="app-btn" onClick={() => consent?.answer("once")}>{L("这次允许", "Allow this time")}</button>}
        <button type="button" className="app-btn app-btn-primary" onClick={() => consent?.answer(consent.once_only ? "once" : "always")}>{consent?.once_only ? L("这次允许", "Allow this time") : L("始终允许", "Always allow")}</button>
      </div>
    </Modal>
    <Modal open={!!asking} onOpenChange={(o) => { if (!o) asking?.answer(false); }} className="pg-ask"
      title={L(E`允许这个网页以你的名义给 ${asking?.to || ""} 发消息吗？`, E`Let this page message ${asking?.to || ""} as you?`)}
      description={L("网页是 agent 写的。允许后，这次打开 App 期间它可以直接发，一分钟最多 5 条；发出的消息在你和它的对话里。", "An agent wrote this page. If you allow it, it may send messages until you quit the App, at most 5 a minute; they show in your chat with the agent.")}>
      <blockquote className="pg-ask-text">{asking?.text}</blockquote>
      <div className="app-modal-actions">
        <button type="button" className="app-btn" onClick={() => asking?.answer(false)}>{L("不允许", "Don't allow")}</button>
        <button type="button" className="app-btn app-btn-primary" onClick={() => asking?.answer(true)}>{L("允许并发送", "Allow and send")}</button>
      </div>
    </Modal>
    </div>
  );
}

/**
 * A page on its way (PageView): instead of a blank or black frame, the
 * mushroom of the agent that made it hops in place, its shadow with it,
 * over a few words that change as the wait goes on, and a thin light runs
 * along the bottom. It fades when the page has loaded.
 */
function PageLoading({ name, seed, who }: { name: string; seed: string; who: string }) {
  const [t, setT] = useState(0);
  useEffect(() => {
    const started = Date.now();
    const id = window.setInterval(() => setT(Date.now() - started), 600);
    return () => window.clearInterval(id);
  }, []);
  const tips = [
    L("正在把网页搬过来…", "Bringing the page over…"),
    who ? L(E`${who} 做的，马上就好`, E`Made by ${who}; nearly there`) : L("马上就好", "Nearly there"),
    L("第一次打开会慢一点", "The first time takes a little longer"),
  ];
  const tip = t > 15_000 ? L("还没打开？可以点右上角的刷新再试一次", "Still not here? Try reload at the top right")
    : t > 8_000 ? L("大一点的网页要多等几秒", "A bigger page takes a few more seconds")
      : tips[Math.floor(t / 2400) % tips.length];
  return (
    <div className="pg-loading" role="status" aria-live="polite" data-testid="page-loading">
      <div className="pg-loading-hop" aria-hidden>
        <span className="pg-loading-mush" style={{ ["--av-cap" as string]: looksOf(seed).cap }}><MushroomFace seed={seed} /></span>
        <span className="pg-loading-shadow" />
      </div>
      <b className="pg-loading-name">{L(E`正在打开「${name}」`, E`Opening "${name}"`)}</b>
      <span key={tip} className="pg-loading-tip">{tip}</span>
      <span className="pg-loading-bar" aria-hidden><i /></span>
    </div>
  );
}


/** What a group of the computer's capabilities lets a page do, said plainly. */
/** Why a group may only be allowed for this run (never "always"). */
function osOnceWhy(group: string): string {
  return group === "files.delete"
    ? L("删掉的文件找不回来，所以这一项没有「始终允许」：每次打开 App 后第一次用都会问你。", "Deleted files can't be brought back, so there is no \"Always allow\" for this: the App asks the first time it's used each time you open the App.")
    : L("命令能改动或删掉电脑上的东西，事后撤不回，所以这一项没有「始终允许」：每次打开 App 后第一次用都会问你。", "Commands can change or delete things on this computer that can't be undone, so there is no \"Always allow\" for this: the App asks the first time it's used each time you open the App.");
}

export function osWant(group: string, host?: string): { zh: string; en: string } {
  switch (group) {
    case "files": return { zh: "读这台电脑上的文件", en: "read files on this computer" };
    case "files.write": return { zh: "在这台电脑上新建和修改文件", en: "create and change files on this computer" };
    case "files.delete": return { zh: "删除这台电脑上的文件", en: "delete files on this computer" };
    case "exec": return { zh: "在这台电脑上运行命令和程序", en: "run commands and programs on this computer" };
    case "screen": return { zh: "截取或录制屏幕", en: "capture or record the screen" };
    case "microphone": return { zh: "录下麦克风和电脑里的声音", en: "record the microphone and the computer's sound" };
    case "speech": return { zh: "在这台电脑上把语音转成文字", en: "turn speech into text on this computer" };
    case "clipboard": return { zh: "读取剪贴板", en: "read the clipboard" };
    case "notify": return { zh: "发系统通知", en: "show system notifications" };
    case "network": return host ? { zh: `访问 ${host}`, en: `reach ${host}` } : { zh: "联网", en: "use the network" };
    case "ssh": return { zh: "连接你保存的服务器", en: "connect to servers you saved" };
    case "database": return { zh: "连接你保存的数据库", en: "connect to databases you saved" };
    case "browser": return { zh: "在 myco 专用浏览器资料中录制和运行网页操作", en: "record and run web actions in dedicated myco browser profiles" };
    case "proxy": return { zh: "控制这台电脑上的代理", en: "control this computer's proxy" };
    default: return { zh: "查看这台电脑的信息（系统、磁盘、进程、已装的应用）", en: "see this computer's details (system, disks, processes, apps)" };
  }
}

function BrowserApprovalDetails({review}: {review: BrowserApprovalReview}) {
  return <div className="pg-browser-approval" data-testid="browser-script-review">
    <div className="pg-browser-approval-title">{review.name} · v{review.version}</div>
    <div>{L("浏览器资料", "Browser profile")}: {review.profile_name}</div>
    <p>{L("仅批准此版本及列出的子流程。脚本会使用这个资料的登录状态，请检查地址和代码；带变量的地址由运行参数决定。", "Only this version and the listed child snapshots are approved. The script uses this profile’s sign-ins: review its addresses and code. Variable addresses depend on run parameters.")}</p>
    <h4>{L("地址概览", "Addresses")}</h4>
    <ul>{review.urls.map((url,i)=><li key={i}><code>{url}</code></li>)}</ul>
    <h4>{L("JavaScript 与表达式", "JavaScript and expressions")}</h4>
    {review.code.length ? review.code.map((item,i)=><div key={i}><div className="app-muted">{item.script} / {item.node} · {item.type}</div><pre>{item.code}</pre></div>) : <div className="app-muted">{L("没有代码步骤", "No code steps")}</div>}
    <h4>{L("子流程快照", "Child snapshots")}</h4>
    <div>{review.children.length ? review.children.join(", ") : L("无", "None")}</div>
    <details><summary>{L("完整脚本快照", "Full script snapshots")}</summary><pre>{JSON.stringify({script:review.script,scripts:review.scripts},null,2)}</pre></details>
  </div>;
}
