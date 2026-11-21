// Bounded, best-effort personal history, keyed by page and account. No source
// endpoints, media URLs, credentials or video bytes are persisted here.
import { kit } from "./model";
import { movieKey, type Progress } from "./catalog";
const key = () => `myco:cinema:v1:${kit.page}:${kit.me}`;
export function history(): Progress[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(key()) || "[]");
    return Array.isArray(value)
      ? value
          .filter(
            (v): v is Progress =>
              !!v?.movie &&
              typeof v.movie.movie_id === "string" &&
              typeof v.movie.source === "string" &&
              Number.isFinite(v.seconds) &&
              Number.isFinite(v.at),
          )
          .slice(0, 100)
      : [];
  } catch {
    return [];
  }
}
export function remember(value: Progress) {
  try {
    localStorage.setItem(
      key(),
      JSON.stringify(
        [
          value,
          ...history().filter(
            (p) => movieKey(p.movie) !== movieKey(value.movie),
          ),
        ].slice(0, 100),
      ),
    );
  } catch {
    /* Playback works when storage is unavailable. */
  }
}
export function forget(value: Progress) {
  try {
    localStorage.setItem(
      key(),
      JSON.stringify(
        history().filter((p) => movieKey(p.movie) !== movieKey(value.movie)),
      ),
    );
  } catch {
    /* Storage may be disabled. */
  }
}
