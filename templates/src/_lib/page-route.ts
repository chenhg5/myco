// Small URL helpers only. Each app owns its route names and input validation;
// a route selects a view and never grants access or performs shared writes.
export type AppRoute = { name: string; params: URLSearchParams };
export function readAppRoute(fragment: string): AppRoute | null {
  if (!fragment || fragment === "#") return null;
  if (fragment.length > 1024 || !fragment.startsWith("#") || /[\u0000-\u0020\u007f]/.test(fragment)) throw new Error("invalid_route");
  const [name, ...query] = fragment.slice(1).split("?");
  if (!/^[a-z][a-z0-9-]{0,63}$/.test(name) || query.length > 1) throw new Error("invalid_route");
  return { name, params: new URLSearchParams(query[0] || "") };
}
export function appFragment(name: string, values: Record<string, string | undefined>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) if (value !== undefined && value !== "") params.set(key, value);
  const query = params.toString();
  const fragment = `#${name}${query ? `?${query}` : ""}`;
  readAppRoute(fragment);
  return fragment;
}
export function publicAppLink(address: string, fragment: string): string {
  readAppRoute(fragment);
  const u = new URL(address);
  if (!["http:", "https:"].includes(u.protocol) || u.username || u.password || u.search || !/\/pages\/pg_[a-z0-9]+\/?$/i.test(u.pathname)) throw new Error("invalid_page_url");
  u.hash = fragment;
  return u.href;
}

/** Follow ordinary same-document navigation and deliberate repeated opens from
 * the trusted host. Other frames/windows cannot navigate this page through it. */
export function subscribeAppRoute(navigate: () => void): () => void {
  const message = (event: MessageEvent) => {
    if (event.source !== window.parent || event.data?.myco !== "page-route") return;
    const fragment = event.data.fragment;
    if (typeof fragment !== "string" || fragment.length > 1024 || (fragment && !fragment.startsWith("#")) || /[\u0000-\u0020\u007f]/.test(fragment)) return;
    if (location.hash === fragment) navigate();
    else location.hash = fragment;
  };
  window.addEventListener("hashchange", navigate);
  window.addEventListener("message", message);
  return () => { window.removeEventListener("hashchange", navigate); window.removeEventListener("message", message); };
}
