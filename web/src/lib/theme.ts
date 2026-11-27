import { localPreferences, PREFERENCES_CHANGED } from "@/lib/localPreferences";
import { useCallback, useEffect, useState } from "react";

/** Two-state hook used by the classic `myco ui` window. */
export function useTheme() {
  const [dark, setDark] = useState(() => storedMode() === "dark" || (storedMode() === "system" && prefersDark()));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);
  const change = useCallback((next: boolean | ((value: boolean) => boolean)) => { const value = typeof next === "function" ? next(dark) : next; setDark(value); try { localPreferences.setItem("myco-theme", value ? "dark" : "light"); } catch { /* older browser storage may be full */ } }, [dark]);
  return [dark, change] as const;
}

/** Appearance of the App window: system-following by default. */
export type ThemeMode = "system" | "light" | "dark";

const KEY = "myco-theme";
const prefersDark = () => window.matchMedia("(prefers-color-scheme: dark)").matches;

function storedMode(): ThemeMode {
  try {
    const v = localPreferences.getItem(KEY);
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch { /* storage unavailable */ }
  return "system";
}

/**
 * Three-state appearance hook for App mode. Stores the same preference key as
 * the classic window so the two never disagree about light/dark.
 */
export function useThemeMode() {
  const [mode, setMode] = useState<ThemeMode>(storedMode);
  useEffect(() => {
    const apply = () => {
      document.documentElement.classList.toggle("dark", mode === "dark" || (mode === "system" && prefersDark()));
    };
    apply();
    if (mode !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, [mode]);
  useEffect(() => { const on = () => setMode(storedMode()); window.addEventListener(PREFERENCES_CHANGED, on); return () => window.removeEventListener(PREFERENCES_CHANGED, on); }, []);
  const change = useCallback((next: ThemeMode | ((value: ThemeMode) => ThemeMode)) => { const value = typeof next === "function" ? next(mode) : next; setMode(value); try { localPreferences.setItem(KEY, value); } catch { /* older browser storage may be full */ } }, [mode]);
  return [mode, change] as const;
}

/** Text size of the App window (the whole page is scaled). */
export type TextSize = "small" | "standard" | "large";

const SIZE_KEY = "myco-text-size";
export const TEXT_SCALE: Record<TextSize, number> = { small: 1, standard: 1.08, large: 1.16 };

function storedSize(): TextSize {
  try {
    const v = localPreferences.getItem(SIZE_KEY);
    if (v === "small" || v === "standard" || v === "large") return v;
  } catch { /* storage unavailable */ }
  return "standard";
}

export function useTextSize() {
  const [size, setSize] = useState<TextSize>(storedSize);
  useEffect(() => {
    document.documentElement.style.setProperty("zoom", String(TEXT_SCALE[size]));
    return () => { document.documentElement.style.removeProperty("zoom"); };
  }, [size]);
  useEffect(() => { const on = () => setSize(storedSize()); window.addEventListener(PREFERENCES_CHANGED, on); return () => window.removeEventListener(PREFERENCES_CHANGED, on); }, []);
  const change = useCallback((next: TextSize | ((value: TextSize) => TextSize)) => { const value = typeof next === "function" ? next(size) : next; setSize(value); try { localPreferences.setItem(SIZE_KEY, value); } catch { /* older browser storage may be full */ } }, [size]);
  return [size, change] as const;
}
