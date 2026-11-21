// Shared sources and watchlist live in page records. Playback progress is
// personal to this computer and account; it does not upload viewing history.
import { Store } from "../_lib/myco";
import { Kit } from "../_lib/appkit";
import type { Source, SavedMovie } from "./catalog";
export const sources = new Store<Source & Record<string, unknown>>("sources");
export const watchlist = new Store<
  SavedMovie & { order: number } & Record<string, unknown>
>("watchlist");
export const kit = new Kit([sources, watchlist], { quiet: true });
export const sourceName = (id: string) => sources.get(id)?.data.name || "";
