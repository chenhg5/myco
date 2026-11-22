// Public page identity and an opaque application-owned fragment. The fragment
// never changes which page is authorized or which file the backend serves.
export type PageHub = { id: string; url: string; aliases?: string[] };
export type PageLink = { hubId: string; id: string; fragment?: string };
const base = (s: string) => s.replace(/\/+$/, "");
export function parsePageLink(href: string, hubs: PageHub[]): PageLink | null {
  const raw = href.trim();
  if (raw.length > 8192 || /[\u0000-\u0020\u007f]/.test(raw)) return null;
  try {
    const u = new URL(raw);
    if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password || u.search || u.hash.length > 1024) return null;
    const m = /^(https?:\/\/[^?#]+?)\/pages\/(pg_[a-z0-9]+)\/?$/i.exec(u.origin + u.pathname);
    if (!m) return null;
    const at = base(m[1]);
    const hub = hubs.find(h => base(h.url) === at || h.aliases?.some(a => base(a) === at));
    return hub ? { hubId: hub.id, id: m[2], ...(u.hash ? { fragment: u.hash } : {}) } : null;
  } catch { return null; }
}
export function pageFragmentURL(address: string, fragment?: string): string {
  const url = new URL(address);
  url.hash = fragment || "";
  return url.href;
}
