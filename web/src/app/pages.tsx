import { useEffect, useRef, useState } from "react";
import { AppWindow, Check, ExternalLink, Link2, Monitor, RotateCw } from "lucide-react";
import type { Page, PageOpen } from "@/api";
import { ApiError, appApi, isOldHub } from "@/appapi";
import { copyText } from "@/app/copy";
import { DockPanel } from "@/app/preview";
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
  const about = page ? [page.version > 1 ? L(`第 ${page.version} 版`, `Version ${page.version}`) : "", L(`${nameOf(hubId, page.publisher)} 发布`, `by ${nameOf(hubId, page.publisher)}`), timeAgo(page.updated_at)].filter(Boolean).join(" · ") : "";
  return (
    <button type="button" className="pg-card" data-page={id} onClick={() => openPage({ kind: "page", hubId, id })}
      title={L("在旁边打开这个网页", "Open this page beside the chat")}>
      <span className="pg-card-icon"><AppWindow /></span>
      <span className="pg-card-text">
        <span className="pg-card-name">{page?.name || L("网页", "Page")}</span>
        <span className="pg-card-about">{page ? about : L("正在读取…", "Loading…")}</span>
      </span>
      <span className="pg-card-open">{L("打开", "Open")}</span>
    </button>
  );
}

/** The page panel: one, for the whole App; links and cards open it. */
export function PageHost() {
  const [target, setTarget] = useState<PageTarget | null>(null);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const on = (e: Event) => { setTarget((e as CustomEvent<PageTarget>).detail); setOpen(true); };
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
  if (!target) return null;
  return <PagePanel target={target} open={open} onOpenChange={setOpen} />;
}

function PagePanel({ target, open, onOpenChange }: { target: PageTarget; open: boolean; onOpenChange: (o: boolean) => void }) {
  const [opened, setOpened] = useState<PageOpen | null>(null);
  const [page, setPage] = useState<Page | null>(null);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [copied, setCopied] = useState(false);
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
    setOpened(null); setPage(null); setCopied(false);
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, key]);

  // A newer version: said at the top; the page you look at stays as it is.
  useEffect(() => {
    if (!open || target.kind !== "page") return;
    const t = window.setInterval(() => { pageOf(target.hubId, target.id, true).then(setPage, () => undefined); }, 15_000);
    return () => window.clearInterval(t);
  }, [open, target]);

  // A link inside the page asks (its helper script) to go to the browser.
  useEffect(() => {
    if (!open) return;
    const on = (e: MessageEvent) => {
      if (!frame.current || e.source !== frame.current.contentWindow) return;
      const d = e.data as { myco?: string; url?: string } | null;
      if (d?.myco === "open-url" && typeof d.url === "string" && /^(https?:|mailto:)/i.test(d.url)) openOutside(d.url);
    };
    window.addEventListener("message", on);
    return () => window.removeEventListener("message", on);
  }, [open]);

  const local = target.kind === "local" ? new URL(target.url) : null;
  const src = local ? target.kind === "local" && target.url : opened?.url;
  const title = local ? L(`这台电脑 · ${local.port || (local.protocol === "https:" ? "443" : "80")} 端口`, `This computer · port ${local.port || 80}`)
    : opened?.name || page?.name || L("网页", "Page");
  const about = local ? local.host + local.pathname.replace(/\/$/, "")
    : opened ? [opened.latest > 1 || opened.version > 1 ? L(`第 ${opened.version} 版`, `Version ${opened.version}`) : "", page && target.kind === "page" ? L(`${nameOf(target.hubId, page.publisher)} 发布`, `by ${nameOf(target.hubId, page.publisher)}`) : "", page ? timeAgo(page.versions.find((v) => v.n === opened.version)?.at || page.updated_at) : ""].filter(Boolean).join(" · ")
      : "";
  const newer = opened && page && page.version > opened.version ? page.version : 0;
  const copy = async () => {
    if (!page) return;
    await copyText(page.url);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  return (
    <DockPanel open={open} onOpenChange={onOpenChange} title={title} about={about} className="pg-panel" bodyClass="is-page pg-body"
      tools={(
        <>
          <button type="button" className="app-btn app-btn-plain" onClick={() => setReload((n) => n + 1)} disabled={!src}
            title={L("刷新", "Reload")} aria-label={L("刷新", "Reload")}><RotateCw /></button>
          {local ? (
            <button type="button" className="app-btn app-btn-plain" onClick={() => openOutside(target.kind === "local" ? target.url : "")}
              title={L("用浏览器打开", "Open in the browser")} aria-label={L("用浏览器打开", "Open in the browser")}><ExternalLink /></button>
          ) : (
            <button type="button" className="app-btn app-btn-plain" onClick={() => void copy()} disabled={!page}
              title={copied ? L("已复制链接", "Link copied") : L("复制链接", "Copy link")} aria-label={copied ? L("已复制链接", "Link copied") : L("复制链接", "Copy link")}>
              {copied ? <Check /> : <Link2 />}
            </button>
          )}
        </>
      )}>
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
    </DockPanel>
  );
}
