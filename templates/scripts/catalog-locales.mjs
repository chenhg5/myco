// Regenerate display metadata without changing application versions or records.
import fs from "node:fs";
import path from "node:path";
const here = path.resolve(import.meta.dirname, ".."),
  catalog = JSON.parse(
    fs.readFileSync(path.join(here, "localization/catalog.json"), "utf8"),
  ),
  fallback = {};
for (const [app, [name, description]] of Object.entries(catalog)) {
  const dir = path.join(here, "src", app, "public"),
    manifest = JSON.parse(fs.readFileSync(path.join(dir, "myco.json"), "utf8"));
  const locales = {
    en: { name, description },
    zh: { name: manifest.name, description: manifest.description },
  };
  fs.writeFileSync(
    path.join(dir, "locales.json"),
    JSON.stringify(locales, null, 2) + "\n",
  );
  fallback[manifest.id] = locales;
}
fs.writeFileSync(
  path.resolve(here, "../web/src/lib/app-locales.json"),
  JSON.stringify(fallback, null, 2) + "\n",
);
