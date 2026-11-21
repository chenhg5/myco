// All shared writes go through this module, with reversible source deletion
// and watchlist changes. Source edits keep IDs so saved films remain usable.
import { uid } from "../_lib/appkit";
import { t as uiText } from "../_lib/i18n";
import { sources, watchlist, kit } from "./model";
import {
  movieKey,
  savedMovie,
  sourceURL,
  type Movie,
  type Source,
} from "./catalog";
export function saveSource(data: Source, id = uid("src")) {
  const endpoint = sourceURL(data.endpoint);
  if (!data.name.trim()) throw new Error(uiText("请给影视源起个名字"));
  if (sources.rows.some((r) => r.id !== id && r.data.endpoint === endpoint))
    throw new Error(uiText("这个影视源已经添加过了"));
  void kit.track(
    sources.put({ ...data, name: data.name.trim().slice(0, 80), endpoint }, id),
  );
  kit.emit();
  return id;
}
export function removeSource(id: string) {
  const data = sources.get(id)?.data;
  if (!data) return;
  void kit.track(sources.remove(id));
  kit.emit();
  kit.flash(uiText("已删除影视源"), () => {
    void kit.track(sources.put(data, id));
    kit.emit();
  });
}
export function toggleFavorite(movie: Movie) {
  const row = watchlist.rows.find((r) => movieKey(r.data) === movieKey(movie));
  if (row) {
    removeFavorite(row.id);
    return;
  }
  void kit.track(
    watchlist.put({ ...savedMovie(movie), order: Date.now() }, uid("film")),
  );
  kit.emit();
}
export function removeFavorite(id: string) {
  const data = watchlist.get(id)?.data;
  if (!data) return;
  void kit.track(watchlist.remove(id));
  kit.emit();
  kit.flash(uiText("已移出待看"), () => {
    void kit.track(watchlist.put(data, id));
    kit.emit();
  });
}
