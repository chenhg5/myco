import { localPreferences } from "@/lib/localPreferences";
import { formattingLocale, isLang, languageLocale, resolveLanguage, renderMessage, formatMessage, type Lang, type LangChoice, type Message } from "./i18n/locales";
export { E, LANGUAGES, languageName, languageLocale, isChinese } from "./i18n/locales";
export type { Lang, LangChoice } from "./i18n/locales";

type Catalog = Record<string, string>;
type ExtraLang = Exclude<Lang, "zh" | "en">;
// Vite emits these as local assets; only the selected pack is fetched.
const catalogURLs = {
  "zh-Hant": new URL("../../../internal/uilocale/catalogs/zh-Hant.json", import.meta.url).href,
  ja: new URL("../../../internal/uilocale/catalogs/ja.json", import.meta.url).href,
  ko: new URL("../../../internal/uilocale/catalogs/ko.json", import.meta.url).href,
  es: new URL("../../../internal/uilocale/catalogs/es.json", import.meta.url).href,
  "pt-BR": new URL("../../../internal/uilocale/catalogs/pt-BR.json", import.meta.url).href,
} satisfies Record<ExtraLang, string>;
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
const listeners = new Set<() => void>();
export function subscribeLanguage(callback: () => void): () => void {
  listeners.add(callback); return () => { listeners.delete(callback); };
}
let bootFailed = false;
export function languageProblem(): boolean { return bootFailed; }
function setHTML() {
  if (typeof document !== "undefined") document.documentElement.lang = current === "zh" ? "zh-CN" : current;
}
async function load(language: Lang): Promise<void> {
  if (language === "en" || language === "zh" || loaded.has(language)) return;
  let promise = pending.get(language);
  if (!promise) {
    promise = fetch(catalogURLs[language]).then(async (response) => {
      if (!response.ok) throw new Error("Language pack unavailable");
      const data: unknown = await response.json();
      if (!data || typeof data !== "object" || Array.isArray(data) || Object.values(data).some((v) => typeof v !== "string")) throw new Error("Invalid language pack");
      loaded.set(language, data as Catalog);
    }).catch((e: unknown) => { pending.delete(language); throw e; });
    pending.set(language, promise);
  }
  await promise;
}
/** Called after local preferences hydrate, before any window renders its first labels. */
export async function initializeLanguage(): Promise<void> {
  choice = requested();
  current = resolveLanguage(choice, hostLanguages());
  try { await load(current); } catch { current = "en"; bootFailed = true; }
  setHTML();
}
export function lang(): Lang { return current; }
/** Save only after its bundled catalog is available; a late selection cannot win. */
export function setLangChoice(next: LangChoice): Promise<boolean> { return applyLanguage(next, true); }
async function applyLanguage(next: LangChoice, persist: boolean): Promise<boolean> {
  const version = ++revision;
  const language = resolveLanguage(next, hostLanguages());
  await load(language);
  if (version !== revision) return false;
  if (persist) localPreferences.setItem(KEY, next);
  choice = next;
  current = language;
  bootFailed = false;
  setHTML();
  for (const listener of listeners) listener();
  return true;
}
/** Existing Chinese/English call sites remain the source and English fallback. */
export function L(zh: string | Message, en: string | Message): string {
  if (current === "zh") return renderMessage(zh);
  if (current === "en") return renderMessage(en);
  const source = current === "zh-Hant" ? zh : en;
  const key = typeof source === "string" ? source : source.source;
  const catalog = loaded.get(current);
  const translated = catalog && Object.prototype.hasOwnProperty.call(catalog, key) ? catalog[key] : undefined;
  if (!translated) return renderMessage(en);
  return typeof source === "string" ? translated : formatMessage(translated, source.values);
}
export function locale(): string { return formattingLocale(choice, hostLanguages()); }
export function speechLocale(): string { return languageLocale(current); }
setHTML();

if (typeof window !== "undefined") window.addEventListener("languagechange", () => {
  if (choice === "system") void applyLanguage("system", false).catch(() => undefined);
});
