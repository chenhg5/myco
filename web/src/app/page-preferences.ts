import { useEffect, useState } from "react";
export type PagePreferences = {
  language: "zh" | "en";
  theme: "light" | "dark";
};
export function pagePreferences(): PagePreferences {
  return {
    language: document.documentElement.lang.toLowerCase().startsWith("zh")
      ? "zh"
      : "en",
    theme: document.documentElement.classList.contains("dark")
      ? "dark"
      : "light",
  };
}
export function usePagePreferences() {
  const [value, setValue] = useState(pagePreferences);
  useEffect(() => {
    const observer = new MutationObserver(() =>
      setValue((old) => {
        const next = pagePreferences();
        return old.language === next.language && old.theme === next.theme
          ? old
          : next;
      }),
    );
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class", "lang"],
    });
    return () => observer.disconnect();
  }, []);
  return value;
}
// Only the local page frame gets these display hints, never its public URL.
export function localizedPageURL(raw: string, language: string): string {
  const u = new URL(raw);
  u.searchParams.set("myco_lang", language);
  return u.href;
}
