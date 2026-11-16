import { appApi } from "@/appapi";

/*
 * Links out of the App (a chat's links, a file's): in the desktop app the
 * web view opens no new windows, so a link with target=_blank did nothing.
 * There they go to the default browser through the app (POST
 * /app/open-url); in a browser, to a new tab as usual.
 */

let viaApp = false;

/** The desktop app can open links in the default browser (/app/info). */
export function setOpenLinksViaApp(on: boolean) {
  viaApp = on;
}

/** A link that leaves the App: http(s) elsewhere, or mailto. */
export function isOutside(href: string): boolean {
  if (/^mailto:/i.test(href)) return true;
  if (!/^https?:\/\//i.test(href)) return false;
  try { return new URL(href).origin !== window.location.origin; } catch { return false; }
}

export function openOutside(href: string) {
  const tab = () => { window.open(href, "_blank", "noopener,noreferrer"); };
  if (viaApp) void appApi.openUrl(href).catch(tab);
  else tab();
}

/** For a click: when it is on a link that leaves the App, opens it outside
 *  and says so (the click is handled). */
export function handleLinkClick(e: MouseEvent): boolean {
  if (e.defaultPrevented || e.button !== 0) return false;
  const a = (e.target as Element | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
  if (!a || !isOutside(a.href)) return false;
  e.preventDefault();
  openOutside(a.href);
  return true;
}
