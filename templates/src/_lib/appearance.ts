import { myco } from "./myco";
/** A narrow app-aware replacement for color-scheme queries used by canvas,
 * chart and editor hooks. It leaves the browser's matchMedia untouched. */
export function themeMedia() {
  const native = window.matchMedia("(prefers-color-scheme: dark)");
  const callbacks = new Map<() => void, () => void>();
  const current = () =>
    myco?.features?.includes("preferences") && myco.preferences
      ? myco.preferences.theme === "dark"
      : native.matches;
  return {
    get matches() {
      return current();
    },
    addEventListener(_event: string, callback: () => void) {
      let previous = current();
      const changed = () => {
        const next = current();
        if (next !== previous) {
          previous = next;
          callback();
        }
      };
      native.addEventListener("change", changed);
      const off = myco?.on?.("preferences", changed);
      callbacks.set(callback, () => {
        native.removeEventListener("change", changed);
        off?.();
      });
    },
    removeEventListener(_event: string, callback: () => void) {
      callbacks.get(callback)?.();
      callbacks.delete(callback);
    },
  };
}
