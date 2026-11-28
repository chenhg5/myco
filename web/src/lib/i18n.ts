import { localPreferences } from "@/lib/localPreferences";
import { formattingLocale, isLang, languageLocale, resolveLanguage, renderMessage, formatMessage, type Lang, type LangChoice, type Message } from "./i18n/locales";
export { E, LANGUAGES, languageName, languageLocale, isChinese } from "./i18n/locales";
export type { Lang, LangChoice } from "./i18n/locales";

type Catalog = Record<string, string>;
type ExtraLang = Exclude<Lang, "zh" | "en">;
const loaders = {
  "zh-Hant": () => import("../../../internal/uilocale/catalogs/zh-Hant.json"),
  ja: () => import("../../../internal/uilocale/catalogs/ja.json"),
  ko: () => import("../../../internal/uilocale/catalogs/ko.json"),
  es: () => import("../../../internal/uilocale/catalogs/es.json"),
  "pt-BR": () => import("../../../internal/uilocale/catalogs/pt-BR.json"),
} satisfies Record<ExtraLang, () => Promise<{ default: Catalog }>>;
const loaded = new Map<ExtraLang, Catalog>();
const pending = new Map<ExtraLang, Promise<void>>();
const KEY = "myco-lang";
export function storedLangChoice(): LangChoice {
  try {
    const value = localPreferences.getItem(KEY);
    if (value === "system" || isLang(value)) return value;
  } catch { /* keep the host preference when storage is unavailable */ }
  return "system";
}
function hostLanguages(): readonly string[] {
  return typeof navigator === "undefined" ? [] : navigator.languages?.length ? navigator.languages : [navigator.language];
}
function requested(): LangChoice {
  if (typeof location !== "undefined") {
    const query = new URLSearchParams(location.search).get("lang");
    if (isLang(query)) return query;
  }
  return storedLangChoice();
}
let choice = requested();
let current = resolveLanguage(choice, hostLanguages());
let revision = 0;
function setHTML() {
  if (typeof document !== "undefined") document.documentElement.lang = current === "zh" ? "zh-CN" : current;
}
async function load(language: Lang): Promise<void> {
  if (language === "en" || language === "zh" || loaded.has(language)) return;
  let promise = pending.get(language);
  if (!promise) {
    promise = loaders[language]().then((m) => { loaded.set(language, m.default); }).catch((e: unknown) => { pending.delete(language); throw e; });
    pending.set(language, promise);
  }
  await promise;
}
/** Called after local preferences hydrate, before any window renders its first labels. */
export async function initializeLanguage(): Promise<void> {
  choice = requested();
  current = resolveLanguage(choice, hostLanguages());
  await load(current);
  setHTML();
}
export function lang(): Lang { return current; }
/** Save only after its bundled catalog is available; a late selection cannot win. */
export async function setLangChoice(next: LangChoice): Promise<boolean> {
  const version = ++revision;
  const language = resolveLanguage(next, hostLanguages());
  await load(language);
  if (version !== revision) return false;
  localPreferences.setItem(KEY, next);
  choice = next;
  current = language;
  setHTML();
  return true;
}
/** Existing Chinese/English call sites remain the source and English fallback. */
export function L(zh: string | Message, en: string | Message): string {
  if (current === "zh") return renderMessage(zh);
  if (current === "en") return renderMessage(en);
  const source = current === "zh-Hant" ? zh : en;
  const key = typeof source === "string" ? source : source.source;
  const translated = loaded.get(current)?.[key];
  if (!translated) return renderMessage(en);
  return typeof source === "string" ? translated : formatMessage(translated, source.values);
}
export function locale(): string { return formattingLocale(choice, hostLanguages()); }
export function speechLocale(): string { return languageLocale(current); }
setHTML();
