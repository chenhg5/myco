import { useEffect, useState } from "react";

/** Two-state hook used by the classic `myco ui` window. */
export function useTheme() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    try { localStorage.setItem("myco-theme", dark ? "dark" : "light"); } catch { /* storage unavailable */ }
  }, [dark]);
  return [dark, setDark] as const;
}

/** Appearance of the App window: system-following by default. */
export type ThemeMode = "system" | "light" | "dark";

const KEY = "myco-theme";
const prefersDark = () => window.matchMedia("(prefers-color-scheme: dark)").matches;

function storedMode(): ThemeMode {
  try {
    const v = localStorage.getItem(KEY);
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
    try { localStorage.setItem(KEY, mode); } catch { /* storage unavailable */ }
    if (mode !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, [mode]);
  return [mode, setMode] as const;
}
