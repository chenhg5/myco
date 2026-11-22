// A link refers to an existing source and catalog ID, never to a stream URL.
// Explicit line and episode labels must exist; no silent fallback to episode 1.
import { appFragment, readAppRoute } from "../_lib/page-route";
import { t as uiText } from "../_lib/i18n";
import type { Movie } from "./catalog";
export type WatchRoute = { source: string; movie: string; line?: string; episode?: string };
export class WatchRouteError extends Error {}
export function readWatchRoute(fragment: string): WatchRoute | null {
  let route;
  try { route = readAppRoute(fragment); } catch { throw new WatchRouteError(uiText("这个播放页链接不完整或格式不正确")); }
  if (!route) return null;
  if (route.name !== "watch") throw new WatchRouteError(uiText("这个播放页链接不受支持"));
  const p = route.params;
  for (const key of p.keys()) if (!["source","movie","line","episode"].includes(key) || p.getAll(key).length !== 1) throw new WatchRouteError(uiText("这个播放页链接不完整或格式不正确"));
  const source = p.get("source") || "", movie = p.get("movie") || "", line = p.get("line") ?? undefined, episode = p.get("episode") ?? undefined;
  if (!source || source.length > 128 || !movie || movie.length > 200 || [source,movie,line,episode].some(v => v !== undefined && (!v.trim() || v.length > 200 || /[\u0000-\u001f\u007f\ufffd]/.test(v)))) throw new WatchRouteError(uiText("这个播放页链接不完整或格式不正确"));
  if (episode && !line) throw new WatchRouteError(uiText("指定集数的链接还需要包含播放线路"));
  return { source, movie, ...(line ? {line} : {}), ...(episode ? {episode} : {}) };
}
export function watchFragment(route: WatchRoute): string {
  const fragment = appFragment("watch", route);
  readWatchRoute(fragment);
  return fragment;
}
export function watchSelection(movie: Movie, route: WatchRoute): { line: number; episode: number } {
  const line = route.line === undefined ? 0 : movie.lines.findIndex(l => l.name === route.line);
  if (line < 0 || !movie.lines[line]) throw new WatchRouteError(uiText("链接指定的播放线路已不存在，请重新选择线路"));
  const episode = route.episode === undefined ? 0 : movie.lines[line].episodes.findIndex(e => e.name === route.episode);
  if (episode < 0 || !movie.lines[line].episodes[episode]) throw new WatchRouteError(uiText("链接指定的集数已不存在，请重新选择集数"));
  return { line, episode };
}
