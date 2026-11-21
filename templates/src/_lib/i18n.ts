import english from "../../localization/en.json";

/** Translate source UI text only. User records, editor content and enum keys
 * must never be passed here. Display hints come from the host App, not the OS. */
export function language(): "zh" | "en" {
  const hint = new URLSearchParams(
    typeof location === "undefined" ? "" : location.search,
  ).get("myco_lang");
  if (hint === "en" || hint === "zh") return hint;
  const bridge = (
    globalThis as { myco?: { preferences?: { language?: "en" | "zh" } } }
  ).myco;
  return (
    bridge?.preferences?.language ||
    (typeof document !== "undefined" &&
    document.documentElement.lang.toLowerCase().startsWith("en")
      ? "en"
      : "zh")
  );
}
export function locale() {
  return language() === "en" ? "en-US" : "zh-CN";
}
export function t(source: string, ...values: unknown[]): string {
  const text =
    language() === "en"
      ? ((english as Record<string, string>)[source] ?? source)
      : source;
  return text.replace(/\{(\d+)\}/g, (original, index) =>
    Number(index) < values.length
      ? String(values[Number(index)] ?? "")
      : original,
  );
}
