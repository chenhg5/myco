// Source-neutral catalog types and MacCMS JSON adapters. Remote strings stay
// plain text; resolved playback URLs are kept in memory, never in page records.
import { t as uiText } from "../_lib/i18n";
export type Source = {
  name: string;
  endpoint: string;
  enabled: boolean;
  order: number;
};
export type Movie = {
  source: string;
  movie_id: string;
  title: string;
  poster: string;
  year: string;
  category: string;
  remarks: string;
  description: string;
  actors: string;
  director: string;
  area: string;
  lines: PlayLine[];
};
export type Episode = { name: string; url: string };
export type PlayLine = { name: string; episodes: Episode[] };
export type Category = { id: string; name: string };
export type Catalog = {
  movies: Movie[];
  categories: Category[];
  page: number;
  pages: number;
};
export type SavedMovie = Pick<
  Movie,
  "source" | "movie_id" | "title" | "poster" | "year" | "category" | "remarks"
>;
export type Progress = {
  movie: SavedMovie;
  line: string;
  episode: string;
  index: number;
  seconds: number;
  duration: number;
  at: number;
  intro: number;
  outro: number;
};
const object = (v: unknown): Record<string, unknown> =>
  v && typeof v === "object" && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
const text = (v: unknown, limit = 200) =>
  typeof v === "string" || typeof v === "number"
    ? String(v).slice(0, limit).trim()
    : "";
export const positive = (v: unknown, fallback = 0) =>
  Number.isFinite(Number(v)) && Number(v) >= 0 ? Number(v) : fallback;
export const movieKey = (m: Pick<Movie, "source" | "movie_id">) =>
  JSON.stringify([m.source, m.movie_id]);
export const savedMovie = (m: Movie): SavedMovie => ({
  source: m.source,
  movie_id: m.movie_id,
  title: m.title,
  poster: m.poster,
  year: m.year,
  category: m.category,
  remarks: m.remarks,
});
export function httpURL(raw: string, base?: string): string {
  try {
    const u = new URL(raw, base);
    return /^(https?:)$/.test(u.protocol) && !u.username && !u.password
      ? u.href
      : "";
  } catch {
    return "";
  }
}
export function sourceURL(raw: string): string {
  const url = httpURL(raw.trim());
  if (!url || new URL(url).search || new URL(url).hash)
    throw new Error(
      uiText("请输入不带密钥、查询参数或片段的 HTTP(S) 接口地址"),
    );
  return url;
}
export function requestURL(
  source: Source,
  params: {
    query?: string;
    page?: number;
    category?: string;
    id?: string;
  } = {},
): string {
  const u = new URL(sourceURL(source.endpoint));
  u.searchParams.set("ac", "detail");
  if (params.query) u.searchParams.set("wd", params.query.slice(0, 100));
  if (params.id) u.searchParams.set("ids", params.id);
  if (params.category) u.searchParams.set("t", params.category);
  u.searchParams.set("pg", String(Math.max(1, Math.floor(params.page || 1))));
  return u.href;
}
export function plainText(v: unknown) {
  return text(v, 10000)
    .replace(/<[^>]*>/g, " ")
    .replace(/&nbsp;|&#160;/gi, " ")
    .replace(/&amp;/gi, "&")
    .replace(/&lt;/gi, "<")
    .replace(/&gt;/gi, ">")
    .replace(/&quot;/gi, '"')
    .replace(/\s+/g, " ")
    .trim();
}
export function parseLines(
  from: unknown,
  urls: unknown,
  base: string,
): PlayLine[] {
  const names = text(from, 10000).split("$$$");
  return text(urls, 1_000_000)
    .split("$$$")
    .slice(0, 30)
    .map((raw, index) => ({
      name: names[index] || uiText("线路 {0}", index + 1),
      episodes: raw
        .split("#")
        .slice(0, 5000)
        .flatMap((part, i) => {
          const split = part.indexOf("$"),
            address = split < 0 ? part : part.slice(split + 1);
          const url = httpURL(address.trim(), base);
          return url && address.trim()
            ? [
                {
                  name:
                    split < 0
                      ? uiText("第 {0} 集", i + 1)
                      : plainText(part.slice(0, split)) ||
                        uiText("第 {0} 集", i + 1),
                  url,
                },
              ]
            : [];
        }),
    }))
    .filter((l) => l.episodes.length);
}
export function parseCatalog(
  raw: unknown,
  source: string,
  endpoint: string,
): Catalog {
  const r = object(raw);
  if (!Array.isArray(r.list))
    throw new Error(
      uiText("这个地址没有返回兼容的影视列表，请核对 MacCMS JSON 接口"),
    );
  const movies = r.list.slice(0, 200).flatMap((value) => {
    const m = object(value),
      id = text(m.vod_id),
      title = plainText(m.vod_name);
    if (!id || !title) return [];
    return [
      {
        source,
        movie_id: id,
        title,
        poster: m.vod_pic ? httpURL(text(m.vod_pic, 4000), endpoint) : "",
        year: text(m.vod_year),
        category: text(m.type_name),
        remarks: plainText(m.vod_remarks),
        description: plainText(m.vod_content),
        actors: plainText(m.vod_actor),
        director: plainText(m.vod_director),
        area: text(m.vod_area),
        lines: parseLines(m.vod_play_from, m.vod_play_url, endpoint),
      },
    ];
  });
  return {
    movies,
    categories: Array.isArray(r.class)
      ? r.class.flatMap((v) => {
          const c = object(v),
            id = text(c.type_id),
            name = text(c.type_name);
          return id && name ? [{ id, name }] : [];
        })
      : [],
    page: Math.max(1, positive(r.page, 1)),
    pages: Math.max(1, positive(r.pagecount, 1)),
  };
}
// Match an episode by its label, never just by index when changing lines.
export function matchEpisode(episodes: Episode[], name: string): number {
  const normalized = (s: string) =>
    s
      .normalize("NFKC")
      .replace(/[\s\p{P}\p{S}]/gu, "")
      .toLowerCase();
  const exact = episodes.findIndex(
    (e) => normalized(e.name) === normalized(name),
  );
  if (exact >= 0) return exact;
  const number = /第\s*0*(\d+)\s*[集话期]/.exec(name)?.[1];
  return number
    ? episodes.findIndex(
        (e) =>
          Number(/第\s*0*(\d+)\s*[集话期]/.exec(e.name)?.[1]) ===
          Number(number),
      )
    : -1;
}
export function clock(n: number) {
  const s = Math.max(0, Math.floor(Number.isFinite(n) ? n : 0));
  return s >= 3600
    ? `${Math.floor(s / 3600)}:${String(Math.floor(s / 60) % 60).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`
    : `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}
