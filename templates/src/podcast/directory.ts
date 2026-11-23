// Podcast discovery uses Apple's public directory; playback comes from the
// publisher's RSS enclosure, never from promotional audio previews.
import { network } from "../_lib/net";
export type DirectoryShow = {
  id: string;
  title: string;
  author: string;
  feedUrl: string;
  artworkUrl: string;
  directoryUrl: string;
  genre: string;
};
export const MARKETS = [
  "cn",
  "us",
  "gb",
  "jp",
  "de",
  "fr",
  "es",
  "br",
  "kr",
  "in",
  "au",
] as const;
export type Market = (typeof MARKETS)[number];
export function publicURL(value: unknown): string {
  if (typeof value !== "string" || /[\u0000-\u001f\u007f]/.test(value))
    return "";
  try {
    const u = new URL(value);
    return /^https?:$/.test(u.protocol) && !u.username && !u.password
      ? u.href
      : "";
  } catch {
    return "";
  }
}
export function parseDirectory(raw: unknown): DirectoryShow[] {
  if (
    !raw ||
    typeof raw !== "object" ||
    !Array.isArray((raw as { results?: unknown }).results)
  )
    throw new Error("invalid_directory");
  const rows = (raw as { results: unknown[] }).results,
    seen = new Set<string>();
  const result: DirectoryShow[] = [];
  const text = (v: unknown) => (typeof v === "string" ? v.slice(0, 2000) : "");
  for (const row of rows.slice(0, 100)) {
    if (!row || typeof row !== "object") continue;
    const r = row as Record<string, unknown>,
      feedUrl = publicURL(r.feedUrl);
    if (!feedUrl || seen.has(feedUrl) || !text(r.collectionName)) continue;
    seen.add(feedUrl);
    result.push({
      id: String(r.collectionId ?? feedUrl),
      title: text(r.collectionName),
      author: text(r.artistName),
      feedUrl,
      artworkUrl: publicURL(r.artworkUrl600) || publicURL(r.artworkUrl100),
      directoryUrl: publicURL(r.collectionViewUrl),
      genre: text(r.primaryGenreName),
    });
  }
  return result;
}
export async function searchShows(
  term: string,
  market: Market,
  signal?: AbortSignal,
): Promise<DirectoryShow[]> {
  const query = term.trim().slice(0, 160);
  if (!query) return [];
  if (!MARKETS.includes(market)) throw new Error("invalid_market");
  signal?.throwIfAborted();
  const url = new URL("https://itunes.apple.com/search");
  for (const [key, value] of Object.entries({
    term: query,
    media: "podcast",
    entity: "podcast",
    country: market,
    limit: "24",
  }))
    url.searchParams.set(key, value);
  const response = await network().fetch({
    url: url.href,
    response: "text",
    max_bytes: 1024 * 1024,
    timeout_ms: 12000,
  });
  signal?.throwIfAborted();
  if (response.status < 200 || response.status >= 300 || response.truncated)
    throw new Error("directory_unavailable");
  return parseDirectory(JSON.parse(response.body));
}
