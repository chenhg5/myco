import type { Template } from "@/api";
import builtins from "./app-locales.json";
import { lang, L } from "./i18n";
/** The catalog is translated. A page's own name and records never are. */
export function templateDisplay(t: Template): Template {
  const all =
    t.locales ||
    (t.official
      ? (builtins as Record<string, NonNullable<Template["locales"]>>)[t.slug]
      : undefined);
  const language = lang();
  const match =
    all?.[language] ||
    Object.entries(all || {}).find(
      ([key]) => key.toLowerCase().split("-")[0] === language,
    )?.[1];
  return match
    ? {
        ...t,
        name: match.name || t.name,
        description: match.description ?? t.description,
      }
    : all?.en ? { ...t, name: L(all.zh?.name || t.name, all.en.name || t.name), description: L(all.zh?.description || t.description || "", all.en.description ?? t.description ?? "") } : t;
}
