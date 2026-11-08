import { useEffect, useState } from "react";

export function useTheme() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    try { localStorage.setItem("myco-theme", dark ? "dark" : "light"); } catch { /* storage unavailable */ }
  }, [dark]);
  return [dark, setDark] as const;
}
