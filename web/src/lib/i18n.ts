import { localPreferences } from "@/lib/localPreferences";
/**
 * The App's two languages. Strings stay next to their use as
 * L("中文", "English"); the language follows the system unless chosen in
 * Settings → Appearance (saved in this App profile, or in browser storage on the web).
 */
export type Lang = "zh" | "en";
export type LangChoice = "system" | Lang;

const KEY = "myco-lang";

export function storedLangChoice(): LangChoice {
  try {
    const v = localPreferences.getItem(KEY);
    if (v === "zh" || v === "en" || v === "system") return v;
  } catch { /* storage unavailable */ }
  return "system";
}

function systemLang(): Lang {
  const langs = typeof navigator !== "undefined" ? (navigator.languages?.length ? navigator.languages : [navigator.language]) : [];
  return langs.some((l) => /^zh\b/i.test(l || "")) ? "zh" : "en";
}

let current: Lang = resolve(storedLangChoice());

function resolve(choice: LangChoice): Lang {
  return choice === "system" ? systemLang() : choice;
}

/** The language in use. */
export function lang(): Lang {
  return current;
}

/** Sets the language choice; the caller re-renders (AppMode keys on lang()). */
export function setLangChoice(choice: LangChoice): Lang {
  try { localPreferences.setItem(KEY, choice); } catch { /* storage unavailable */ }
  current = resolve(choice);
  document.documentElement.lang = current === "zh" ? "zh-CN" : "en";
  return current;
}

/** L picks the Chinese or English text. */
export function L(zh: string, en: string): string {
  return current === "zh" ? zh : en;
}

/** The locale for dates and sorting. */
export function locale(): string {
  return current === "zh" ? "zh-CN" : "en-US";
}

/** ?lang=en|zh in the URL overrides the choice (screenshots, tests). */
if (typeof location !== "undefined") {
  const q = new URLSearchParams(location.search).get("lang");
  if (q === "en" || q === "zh") current = q;
}
if (typeof document !== "undefined") document.documentElement.lang = current === "zh" ? "zh-CN" : "en";
