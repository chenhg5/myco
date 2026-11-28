/** Stable language IDs are shared by preferences, HTML, and the desktop bridge. */
export const LANGUAGES = [
  { id: "zh", name: "简体中文", locale: "zh-CN" },
  { id: "zh-Hant", name: "繁體中文", locale: "zh-TW" },
  { id: "en", name: "English", locale: "en-US" },
  { id: "ja", name: "日本語", locale: "ja-JP" },
  { id: "ko", name: "한국어", locale: "ko-KR" },
  { id: "es", name: "Español", locale: "es-ES" },
  { id: "pt-BR", name: "Português (Brasil)", locale: "pt-BR" },
] as const;
export type Lang = typeof LANGUAGES[number]["id"];
export type LangChoice = "system" | Lang;
export const isLang = (value: unknown): value is Lang => LANGUAGES.some((x) => x.id === value);
export const languageName = (value: Lang): string => LANGUAGES.find((x) => x.id === value)!.name;
export const languageLocale = (value: Lang): string => LANGUAGES.find((x) => x.id === value)!.locale;
export const isChinese = (value: Lang): boolean => value === "zh" || value === "zh-Hant";

function match(tag: string): Lang | undefined {
  const normalized = tag.replaceAll("_", "-").toLowerCase();
  if (/^zh(?:-|$)/.test(normalized)) return /^zh-(?:hant|tw|hk|mo)(?:-|$)/.test(normalized) ? "zh-Hant" : "zh";
  if (/^pt(?:-|$)/.test(normalized)) return "pt-BR";
  return (["en", "ja", "ko", "es"] as const).find((l) => normalized === l || normalized.startsWith(`${l}-`));
}
/** Follow the first supported host preference, rather than finding any Chinese tag. */
export function resolveLanguage(choice: string, host: readonly string[]): Lang {
  if (isLang(choice)) return choice;
  for (const tag of host) {
    const found = match(tag);
    if (found) return found;
  }
  return "en";
}
export function formattingLocale(choice: string, host: readonly string[]): string {
  const language = resolveLanguage(choice, host);
  if (choice === "system" || !isLang(choice)) {
    const tag = host.find((x) => match(x) === language);
    if (tag) {
      try { return Intl.getCanonicalLocales(tag.replaceAll("_", "-"))[0]; } catch { /* use the language's default region */ }
    }
  }
  return languageLocale(language);
}

export type Message = { readonly source: string; readonly values: readonly unknown[] };
/** Tag dynamic copy without guessing which user-provided substrings are variables. */
export function E(parts: TemplateStringsArray, ...values: unknown[]): Message {
  return { source: parts.reduce((s, p, i) => s + (i ? `{${i - 1}}` : "") + p, ""), values };
}
export function formatMessage(source: string, values: readonly unknown[]): string {
  return source.replace(/\{(\d+)\}/g, (token, index: string) => Number(index) < values.length ? String(values[Number(index)]) : token);
}
export function renderMessage(value: string | Message): string {
  return typeof value === "string" ? value : formatMessage(value.source, value.values);
}
