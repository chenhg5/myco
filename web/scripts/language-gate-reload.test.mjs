import assert from "node:assert/strict";
import { reloadCancellation } from "./language-gate-reload.mjs";

const base = "http://127.0.0.1:5173";
const error = url => `Fetch API cannot load ${url} due to access control checks.`;
const cases = [
  [error(`${base}/app/hubs/demo/exchanges?a=demo`), base, true, "/app/hubs/demo/exchanges"],
  [error(`${base}/app/cache`), base, true, "/app/cache"],
  [error(`${base}/app/notices/focus`), base, true, "/app/notices/focus"],
  [error(`${base}/app/update`), base, false, null],
  [error("https://example.com/app/update"), base, true, null],
  [error(`${base}0/app/update`), base, true, null],
  [error(`${base}.example.com/app/update`), base, true, null],
  [error(`${base}/assets/ja-pack.json`), base, true, null],
  [error(`${base}/app/preferences`), base, true, null],
  [error(`${base}/app/preferences?version=1`), base, true, null],
  [error(`${base}/app/preferences/import`), base, true, null],
  [error(`${base}/app/preferences/images/picture.png`), base, true, null],
  [error(`${base}/app/ui-lang`), base, true, null],
  ["Language page crashed", base, true, null],
  ["Failed to load resource: status 500", base, true, null],
];
for (const [text, origin, reloading, expected] of cases) {
  assert.equal(reloadCancellation(text, origin, reloading), expected, text);
}
console.log(`Reload cancellation boundaries pass (${cases.length} cases).`);
