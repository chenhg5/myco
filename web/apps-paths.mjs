// Shared development/test resource paths. Application sources and built files
// belong to myco-ai/apps; no fallback to retired main-repository copies.
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export const appsRoot = path.resolve(
  process.env.APPS_TEST_ROOT || fileURLToPath(new URL("../.cache/apps-source/", import.meta.url)),
);
export function appsPath(...parts) {
  const file = path.join(appsRoot, ...parts);
  if (!existsSync(file)) {
    throw new Error(`Missing official app resource: ${file}. Set APPS_TEST_ROOT to the apps checkout at apps.lock.json's commit and run python3 scripts/prepare-apps.py --apps-dir <apps-checkout>/dist/official; see docs/building.md.`);
  }
  return file;
}
export function officialAppsURL() {
  return pathToFileURL(appsPath("dist", "official") + path.sep);
}
