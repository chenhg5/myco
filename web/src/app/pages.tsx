import { useCallback, useEffect, useRef, useState } from "react";
import { AppWindow, Check, ExternalLink, Link2, Monitor, Radio, RotateCw, X } from "lucide-react";
import type { Page, PageOpen } from "@/api";
import { ApiError, appApi, isOldHub } from "@/appapi";
import { copyText } from "@/app/copy";
import { DockPanel } from "@/app/preview";
import { Modal } from "@/app/dialogs";
import { openOutside, setInAppLinks } from "@/app/links";
import { L } from "@/lib/i18n";
import { cn, timeAgo } from "@/lib/utils";

/*
 * Web pages agents publish (docs/design/browser.md). A link to one,
 * <hub>/pages/<id>, shows under its message as a card; it opens in the page
 * panel (the panel files open in: beside the chat when there is room). Its
 * files come from this App's own loopback port, not the App's origin, in a
 * sandbox: it cannot reach the App or the internet. A link to this computer
 * (http://localhost:5173, an agent's dev server here) opens in the same
 * panel.
 */

type HubRef = { id: string; url: string };
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
  hubs = list.map((h) => ({ id: h.id, url: base(h.url) }));
}

const PAGE = /^(https?:\/\/[^\s?#]+?)\/pages\/(pg_[a-z0-9]+)\/?$/i;

/** A link to a page of one of the App's workspaces. */
export function pageLinkOf(href: string): { hubId: string; id: string } | null {
  const m = PAGE.exec(href.trim());
  if (!m) return null;
  const h = hubs.find((x) => x.url === base(m[1]));
  return h ? { hubId: h.id, id: m[2] } : null;
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

export type PageTarget = { kind: "page"; hubId: string; id: string } | { kind: "local"; url: string };

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
export function pageLinksIn(body: string): { hubId: string; id: string }[] {
  const seen = new Set<string>();
  const out: { hubId: string; id: string }[] = [];
  for (const m of body.matchAll(URLS)) {
    const p = pageLinkOf(m[0].replace(/[.,;:!?)\]]+$/, ""));
    if (p && !seen.has(p.id)) { seen.add(p.id); out.push(p); }
  }
  return out;
}

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
function storeOf(hubId: string, id: string): Record<string, unknown> {
  try { return JSON.parse(localStorage.getItem(storeKey(hubId, id)) || "{}") as Record<string, unknown>; } catch { return {}; }
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
      {links.map((l) => <PageCard key={l.id} hubId={l.hubId} id={l.id} />)}
    </div>
  );
}

function PageCard({ hubId, id }: { hubId: string; id: string }) {
  const [page, setPage] = useState<Page | null>(null);
  const [gone, setGone] = useState(false);
  useEffect(() => {
    let live = true;
    pageOf(hubId, id).then((p) => live && setPage(p), () => live && setGone(true));
    return () => { live = false; };
  }, [hubId, id]);
  // Not there (deleted, not yours, an older hub): the link in the text stays.
  if (gone) return null;
  const who = page ? nameOf(hubId, page.publisher) : "";
  const about = !page ? ""
    : page.live ? L(`${who} 电脑上的 ${page.port} 端口`, `Port ${page.port} on ${who}'s computer`)
      : [page.version > 1 ? L(`第 ${page.version} 版`, `Version ${page.version}`) : "", L(`${who} 发布`, `by ${who}`), timeAgo(page.updated_at)].filter(Boolean).join(" · ");
  return (
    <button type="button" className="pg-card" data-page={id} onClick={() => openPage({ kind: "page", hubId, id })}
      title={L("在旁边打开这个网页", "Open this page beside the chat")}>
      <span className={cn("pg-card-icon", page?.live && "is-live")}>{page?.live ? <Radio /> : <AppWindow />}</span>
      <span className="pg-card-text">
        <span className="pg-card-name">
          {page?.name || L("网页", "Page")}
          {page?.live && <span className={cn("pg-live", page.online && "is-on")}>{page.online ? L("实时 · 在线", "Live") : L("实时 · 离线", "Live · offline")}</span>}
        </span>
        <span className="pg-card-about">{page ? about : L("正在读取…", "Loading…")}</span>
      </span>
      <span className="pg-card-open">{L("打开", "Open")}</span>
    </button>
  );
}

/** The page panel: one, for the whole App; links and cards open it. */
type Tab = { key: string; target: PageTarget; reload: number; meta: PageMeta };
/** What a tab says about itself, for the panel's head and its tab. */
type PageMeta = { title: string; about: string; icon: "page" | "live" | "local"; copyUrl?: string; localUrl?: string; loaded: boolean };
const keyOf = (t: PageTarget) => (t.kind === "page" ? `${t.hubId}/${t.id}` : t.url);
const MAX_TABS = 8;

/** The page panel: one, for the whole App, with a tab for each page open
 *  in it; links and cards open it. A page already open is shown again. */
export function PageHost() {
  const [tabs, setTabs] = useState<Tab[]>([]);
  const [active, setActive] = useState("");
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    const on = (e: Event) => {
      const t = (e as CustomEvent<PageTarget>).detail;
      const k = keyOf(t);
      setTabs((list) => {
        if (list.some((x) => x.key === k)) return list;
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
    return () => { window.removeEventListener("myco-open-page", on); document.removeEventListener("click", click); };
  }, []);
  // Closed, the tabs go with it.
  const setPanel = (o: boolean) => {
    setOpen(o);
    if (!o) { setTabs([]); setActive(""); }
  };
  const close = (k: string) => {
    const at = tabs.findIndex((x) => x.key === k);
    const rest = tabs.filter((x) => x.key !== k);
    if (!rest.length) { setPanel(false); return; }
    setTabs(rest);
    if (k === active) setActive(rest[Math.min(at, rest.length - 1)].key);
  };
  const meta = useCallback((k: string, m: PageMeta) => {
    setTabs((list) => list.map((x) => (x.key === k && JSON.stringify(x.meta) !== JSON.stringify(m) ? { ...x, meta: m } : x)));
  }, []);
  if (!tabs.length) return null;
  const cur = tabs.find((x) => x.key === active) || tabs[0];
  const m = cur.meta;
  const copy = async () => {
    if (!m.copyUrl) return;
    await copyText(m.copyUrl);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  const tabIcon = (x: Tab) => (x.meta.icon === "live" ? <Radio /> : x.meta.icon === "local" ? <Monitor /> : <AppWindow />);
  return (
    <DockPanel open={open} onOpenChange={setPanel} title={m.title || L("网页", "Page")} about={m.about} className="pg-panel" bodyClass="is-page pg-body"
      tools={(
        <>
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
                title={L("关闭这个标签", "Close this tab")} aria-label={L(`关闭 ${x.meta.title || "这个标签"}`, `Close ${x.meta.title || "this tab"}`)}><X /></button>
            </div>
          ))}
        </div>
      )}
      {tabs.map((x) => (
        <PageView key={x.key} target={x.target} open={open} active={x.key === cur.key} reload={x.reload} onMeta={(mm) => meta(x.key, mm)} />
      ))}
    </DockPanel>
  );
}

/** One page in the panel (a tab): its frame, its bridge, its notices. It
 *  stays loaded while another tab is shown. */
function PageView({ target, open, active, reload, onMeta }: {
  target: PageTarget; open: boolean; active: boolean; reload: number; onMeta: (m: PageMeta) => void;
}) {
  const [opened, setOpened] = useState<PageOpen | null>(null);
  const [page, setPage] = useState<Page | null>(null);
  const [error, setError] = useState("");
  const frame = useRef<HTMLIFrameElement>(null);
  const key = target.kind === "page" ? `${target.hubId}/${target.id}` : target.url;

  const load = async (version?: number) => {
    if (target.kind !== "page") return;
    setError("");
    try {
      const [o, p] = await Promise.all([appApi.openPage(target.hubId, target.id, version), pageOf(target.hubId, target.id, true)]);
      setOpened(o); setPage(p);
    } catch (e) {
      setOpened(null);
      setError(isOldHub(e) || (e instanceof ApiError && /not enabled/.test(e.message))
        ? L("这个工作区的 Hub 还不支持网页：在服务器上运行 sudo myco upgrade，升级到 v0.19.24 或更新。", "This workspace's hub has no pages yet: upgrade it to v0.19.24 or later (sudo myco upgrade on the server).")
        : e instanceof ApiError && e.status === 404
          ? L("这个网页已经删除了，或者你没有权限看。", "This page was deleted, or you may not see it.")
          : (e as Error).message || L("打不开这个网页", "Could not open this page"));
    }
  };
  useEffect(() => {
    if (!open) return;
    setOpened(null); setPage(null);
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, key]);

  // A newer version: said at the top; the page you look at stays as it is.
  useEffect(() => {
    if (!open || !active || target.kind !== "page") return;
    const t = window.setInterval(() => { pageOf(target.hubId, target.id, true).then(setPage, () => undefined); }, 15_000);
    return () => window.clearInterval(t);
  }, [open, active, target]);

  // The page's script (pagebridge.js) asks the App: a link to open in the
  // browser, and the myco bridge's calls. Only from this panel's frame;
  // answers go to the page's own origin only.
  const live = useRef({ page, opened });
  live.current = { page, opened };
  const known = useRef(0); // the state version the page has
  const bridged = useRef(false);
  const [asking, setAsking] = useState<{ text: string; to: string; answer: (ok: boolean) => void } | null>(null);
  useEffect(() => {
    if (!open) return;
    known.current = 0;
    bridged.current = false;
    const on = (e: MessageEvent) => {
      const win = frame.current?.contentWindow;
      if (!win || e.source !== win) return;
      const d = e.data as { myco?: string; url?: string; id?: number; method?: string; args?: Record<string, unknown> } | null;
      if (d?.myco === "open-url" && typeof d.url === "string" && /^(https?:|mailto:)/i.test(d.url)) { openOutside(d.url); return; }
      if (d?.myco !== "call" || typeof d.id !== "number") return;
      const origin = (() => { try { return new URL(frame.current?.src || "").origin; } catch { return ""; } })();
      if (!origin || e.origin !== origin) return;
      const reply = (value: unknown, error?: string, code?: string) =>
        win.postMessage(error ? { myco: "result", id: d.id, error, code } : { myco: "result", id: d.id, value }, origin);
      if (target.kind !== "page") { reply(null, L("这个网页不在 myco 上，没有桥", "This page is not on myco: no bridge"), "not_a_page"); return; }
      void answer(target, d.method || "", d.args || {}).then((v) => reply(v), (err: unknown) => reply(null, (err as Error).message || "error", (err as ApiError).code));
    };
    const answer = async (t: { hubId: string; id: string }, method: string, args: Record<string, unknown>): Promise<unknown> => {
      const { page: p, opened: o } = live.current;
      switch (method) {
        case "ready": {
          const st = await appApi.pageState(t.hubId, t.id);
          known.current = st.version;
          bridged.current = true;
          return { page: { id: t.id, name: o?.name || p?.name || "", version: o?.version || 0 }, me: { name: meOf(t.hubId) }, state: st.state, version: st.version };
        }
        case "update": {
          const r = await appApi.putPageState(t.hubId, t.id, args.state ?? null, Number(args.version) || 0);
          known.current = Math.max(known.current, r.version);
          return { ok: r.ok, state: r.state, version: r.version };
        }
        case "send": {
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
            to, body: `${text}\n\n${L(`（来自网页「${name}」${p?.url || ""}，状态第 ${known.current} 版）`, `(from the page "${name}" ${p?.url || ""}, state version ${known.current})`)}`,
            ...(p?.project ? { project: p.project } : {}),
          });
          return { id: m.id };
        }
        case "storage.get": {
          const all = storeOf(t.hubId, t.id);
          return all[String(args.key)] ?? null;
        }
        case "storage.set": {
          const all = storeOf(t.hubId, t.id);
          if (args.value === null || args.value === undefined) delete all[String(args.key)];
          else all[String(args.key)] = args.value;
          const raw = JSON.stringify(all);
          if (raw.length > 1 << 20) throw Object.assign(new Error(L("这个网页的存储满了（1 MB）", "This page's storage is full (1 MB)")), { code: "too_large" });
          try { localStorage.setItem(storeKey(t.hubId, t.id), raw); } catch { throw new Error(L("存不下", "Could not store it")); }
          return true;
        }
      }
      throw new Error(`myco: no such call: ${method}`);
    };
    window.addEventListener("message", on);
    return () => window.removeEventListener("message", on);
  }, [open, target]);

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
  }, [open, active, target]);

  const local = target.kind === "local" ? new URL(target.url) : null;
  const src = local ? target.kind === "local" && target.url : opened?.url;
  const title = local ? L(`这台电脑 · ${local.port || (local.protocol === "https:" ? "443" : "80")} 端口`, `This computer · port ${local.port || 80}`)
    : opened?.name || page?.name || L("网页", "Page");
  const about = local ? local.host + local.pathname.replace(/\/$/, "")
    : opened?.live && target.kind === "page" ? L(`实时 · ${page ? nameOf(target.hubId, page.publisher) : ""} 电脑上的 ${opened.port || page?.port || ""} 端口`, `Live · port ${opened.port || page?.port || ""} on ${page ? nameOf(target.hubId, page.publisher) : ""}'s computer`)
    : opened ? [opened.latest > 1 || opened.version > 1 ? L(`第 ${opened.version} 版`, `Version ${opened.version}`) : "", page && target.kind === "page" ? L(`${nameOf(target.hubId, page.publisher)} 发布`, `by ${nameOf(target.hubId, page.publisher)}`) : "", page ? timeAgo(page.versions.find((v) => v.n === opened.version)?.at || page.updated_at) : ""].filter(Boolean).join(" · ")
      : "";
  const newer = opened && page && page.version > opened.version ? page.version : 0;
  const icon: PageMeta["icon"] = local ? "local" : opened?.live || page?.live ? "live" : "page";
  useEffect(() => {
    onMeta({ title, about, icon, copyUrl: page?.url, localUrl: target.kind === "local" ? target.url : undefined, loaded: !!src });
  }, [title, about, icon, page?.url, local, target, src, onMeta]);
  return (
    <div className={cn("pg-view", !active && "is-hidden")} role="tabpanel" aria-hidden={!active}>
      {newer > 0 && (
        <div className="pg-new" role="status">
          <span>{L(`有新版本：第 ${newer} 版`, `A new version: version ${newer}`)}</span>
          <button type="button" className="app-btn app-btn-small" onClick={() => void load()}>{L("看新版本", "Show it")}</button>
        </div>
      )}
      {local && <div className="pg-local-note"><Monitor />{L("这台电脑上正在运行的网页。是空白的话，可能这个端口没有在跑。", "A server running on this computer. Blank? Nothing may be running on this port.")}</div>}
      {error ? <p className="fp-note">{error}</p>
        : !src ? <p className="fp-note">{L("正在打开…", "Opening…")}</p>
        : <iframe ref={frame} key={`${src}#${reload}`} className={cn("fp-frame pg-frame")} title={title} src={src || undefined}
            sandbox="allow-scripts allow-forms allow-popups allow-modals allow-downloads allow-same-origin" referrerPolicy="no-referrer"
            allow="clipboard-write" />}
    <Modal open={!!asking} onOpenChange={(o) => { if (!o) asking?.answer(false); }} className="pg-ask"
      title={L(`允许这个网页以你的名义给 ${asking?.to || ""} 发消息吗？`, `Let this page message ${asking?.to || ""} as you?`)}
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
