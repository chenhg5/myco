import type { LocalHub, LocalState } from "@/appapi";
import { L } from "@/lib/i18n";

/** A hub on this computer. `name` is "" for the default one. */
export type LocalWorkspace = LocalHub & { name: string };

const stripSlash = (u: string) => u.replace(/\/+$/, "");

/** This computer's hubs. A backend that knows only one sends `hub`. */
export function localWorkspaces(local: LocalState | null | undefined): LocalWorkspace[] {
  if (!local) return [];
  if (local.hubs?.length) return local.hubs.map((h) => ({ ...h, name: h.name || "" }));
  return local.hub ? [{ ...local.hub, name: "" }] : [];
}

/** The hub on this computer that an App hub entry points at, if any. */
export function localWorkspaceOf(local: LocalState | null | undefined, url: string): LocalWorkspace | undefined {
  const want = stripSlash(url);
  return localWorkspaces(local).find((w) =>
    [w.local_url, w.public_url].filter(Boolean).some((u) => stripSlash(u!) === want));
}

export function isLocalUrl(local: LocalState | null | undefined, url: string): boolean {
  return !!localWorkspaceOf(local, url);
}

/** How a hub on this computer can be reached. */
export type LocalAccess = "local" | "cloudflare" | "tailscale";

export function accessOf(w: LocalWorkspace): LocalAccess {
  if (!w.public_url) return "local";
  try {
    const host = new URL(w.public_url).hostname;
    if (/(^|\.)ts\.net$/i.test(host)) return "tailscale";
  } catch { /* not a URL: treat it as a Cloudflare address */ }
  return "cloudflare";
}

export function accessLabel(w: LocalWorkspace): string {
  switch (accessOf(w)) {
    case "cloudflare": return L("Cloudflare", "Cloudflare");
    case "tailscale": return L("Tailscale", "Tailscale");
    default: return L("只在本机/局域网", "This computer and local network only");
  }
}

/** The internal name for a new hub here, from the workspace name typed:
 *  latin names become a slug, others (Chinese, digits first) `ws-2`, `ws-3`, …
 *  after the ones already here. */
export function suggestWorkspaceName(display: string, taken: string[]): string {
  const used = new Set(taken.filter(Boolean));
  const slug = display.trim().toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+/, "")
    .slice(0, 40)
    .replace(/-+$/, "");
  if (slug && /^[a-z]/.test(slug)) {
    if (!used.has(slug)) return slug;
    for (let i = 2; ; i++) {
      const next = `${slug}-${i}`;
      if (!used.has(next)) return next;
    }
  }
  let last = 1;
  for (const name of used) {
    const m = /^ws-(\d+)$/.exec(name);
    if (m) last = Math.max(last, Number(m[1]));
  }
  let n = last + 1;
  while (used.has(`ws-${n}`)) n++;
  return `ws-${n}`;
}
