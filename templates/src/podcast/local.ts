// Personal listening activity stays on this device, scoped to page and member.
// Persist metadata only, never audio, credentials or full RSS response bodies.
import type { DirectoryShow } from "./directory";
import { publicURL } from "./directory";
import type { Episode } from "./feed";
export type ListeningState = {
  follows: DirectoryShow[];
  likes: Episode[];
  progress: Record<string, { seconds: number; updatedAt: number }>;
};
export const emptyListening = (): ListeningState => ({
  follows: [],
  likes: [],
  progress: {},
});
export function episodeKey(e: Episode) {
  return JSON.stringify([e.audioUrl, e.guid]);
}
export function listeningKey(page: string, member: string) {
  return `myco.podcast.v1:${encodeURIComponent(page)}:${encodeURIComponent(member)}`;
}
export function readListening(
  storage: Pick<Storage, "getItem"> | null,
  key: string,
): ListeningState {
  try {
    const raw = storage?.getItem(key);
    if (!raw || raw.length > 500000) return emptyListening();
    const data = JSON.parse(raw) as ListeningState;
    return {
      follows: Array.isArray(data.follows)
        ? data.follows
            .filter(
              (s) => s && typeof s.title === "string" && publicURL(s.feedUrl),
            )
            .slice(0, 100)
        : [],
      likes: Array.isArray(data.likes)
        ? data.likes
            .filter(
              (e) =>
                e &&
                typeof e.guid === "string" &&
                typeof e.title === "string" &&
                publicURL(e.audioUrl),
            )
            .slice(0, 100)
        : [],
      progress:
        data.progress &&
        typeof data.progress === "object" &&
        !Array.isArray(data.progress)
          ? Object.fromEntries(
              Object.entries(data.progress)
                .filter(
                  ([, v]) =>
                    v &&
                    Number.isFinite(v.seconds) &&
                    v.seconds >= 0 &&
                    Number.isFinite(v.updatedAt),
                )
                .sort((a, b) => b[1].updatedAt - a[1].updatedAt)
                .slice(0, 100),
            )
          : {},
    };
  } catch {
    return emptyListening();
  }
}
export function saveListening(
  storage: Pick<Storage, "setItem"> | null,
  key: string,
  state: ListeningState,
): boolean {
  try {
    if (!storage) return false;
    const compact = {
      ...state,
      likes: state.likes.map((e) => ({
        ...e,
        description: e.description?.slice(0, 1000) || "",
      })),
    };
    const text = JSON.stringify(compact);
    if (text.length > 500000) return false;
    storage.setItem(key, text);
    return true;
  } catch {
    return false;
  }
}
export function rememberProgress(
  state: ListeningState,
  episode: Episode,
  seconds: number,
  now = Date.now(),
): ListeningState {
  if (!Number.isFinite(seconds) || seconds < 0) return state;
  const progress = {
    ...state.progress,
    [episodeKey(episode)]: { seconds, updatedAt: now },
  };
  return {
    ...state,
    progress: Object.fromEntries(
      Object.entries(progress)
        .sort((a, b) => b[1].updatedAt - a[1].updatedAt)
        .slice(0, 100),
    ),
  };
}
