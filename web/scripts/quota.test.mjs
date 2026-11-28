import assert from "node:assert/strict";
import { build } from "esbuild";

// quota.ts reads two UI helpers; the rules tested here do not depend on
// them. Chinese strings are returned so the assertions match the source.
const stub = {
  name: "stub",
  setup(b) {
    b.onResolve({ filter: /^@\/(lib\/(i18n|utils)|api)$/ }, (a) => ({ path: a.path, namespace: "stub" }));
    b.onLoad({ filter: /.*/, namespace: "stub" }, (a) => ({
      contents: a.path.endsWith("i18n")
        ? "export const L = (zh) => zh; export const E = (parts, ...values) => parts.map((p, i) => (i ? String(values[i-1]) : '') + p).join(''); export const locale = () => 'zh-CN';"
        : a.path.endsWith("utils")
          ? 'export const timeAgo = () => "2 小时前"; export const cn = (...x) => x.filter(Boolean).join(" ");'
          : "export {};",
      loader: "js",
    }));
  },
};
const built = await build({
  entryPoints: [new URL("../src/app/quota.ts", import.meta.url).pathname],
  bundle: true,
  write: false,
  format: "esm",
  plugins: [stub],
});
const { quotaLiveWindows, quotaHasNumbers, quotaEffectiveStatus, quotaStatusText, quotaSummary, quotaSourceText, quotaOutNote, quotaWorst } = await import(
  `data:text/javascript;base64,${Buffer.from(built.outputFiles[0].text).toString("base64")}`
);

const now = Date.UTC(2026, 10, 28, 12, 0, 0);
const hour = 3600e3;
const iso = (ms) => new Date(ms).toISOString();
const win = (name, used, resetsAt) => ({ name, used, resets_at: resetsAt ? iso(resetsAt) : undefined });

// A window whose reset time has passed no longer describes the account.
const two = { at: iso(now - 60e3), checked_at: iso(now - 60e3), status: "ok", windows: [win("five_hour", 0.2, now + hour), win("seven_day", 0.95, now - hour)] };
assert.deepEqual(quotaLiveWindows(two, now).map((w) => w.name), ["five_hour"]);
assert.equal(quotaWorst(two, now).name, "five_hour", "an expired window never is the worst");
assert.equal(quotaHasNumbers(two, now), true);
assert.equal(quotaHasNumbers({ at: iso(now - 60e3), status: "ok", windows: [win("five_hour", 0.2, now - hour)] }, now), false, "all windows reset: no numbers");

// Numbers nobody refreshed for an hour are unknown, not ok.
assert.equal(quotaEffectiveStatus(two, now), "ok");
assert.equal(quotaEffectiveStatus({ ...two, at: iso(now - 2 * hour) }, now), "unknown");
// A runner's shared snapshot may keep an old observation time on purpose;
// its collection time is what counts.
assert.equal(quotaEffectiveStatus({ ...two, profile_ref: "qp_x", at: iso(now - 20 * hour), checked_at: iso(now - 60e3) }, now), "ok");
assert.equal(quotaEffectiveStatus({ ...two, profile_ref: "qp_x", at: iso(now - 20 * hour), checked_at: iso(now - 2 * hour) }, now), "unknown");
// A failure keeps its own words.
assert.equal(quotaEffectiveStatus({ ...two, at: iso(now - 9 * hour), status: "auth_required" }, now), "auth_required");
assert.equal(quotaStatusText({ ...two, status: "unknown" }, now), "超过一小时没更新");

// The summary never presents hour-old numbers as the account's current use.
const stale = quotaSummary({ ...two, at: iso(now - 2 * hour), checked_at: iso(now - 2 * hour) }, [], now);
assert.equal(stale.level, "failed");
assert.equal(stale.short, "额度未知");
assert.ok(!stale.line.includes("剩 "), stale.line);
const fresh = quotaSummary(two, [], now);
assert.equal(fresh.level, "ok");
assert.equal(fresh.short, "剩 80%");
assert.ok(fresh.line.includes("5 小时"), fresh.line);
assert.ok(!fresh.line.includes("每周"), fresh.line);
const tight = quotaSummary({ ...two, windows: [win("five_hour", 0.95, now + hour)] }, [], now);
assert.equal(tight.level, "tight");

// Where the numbers came from, and that a reset window is not "used up".
assert.equal(quotaSourceText({ ...two, source_kind: "statusline", provider: "claude" }), "Claude · 状态栏上报");
assert.equal(quotaOutNote(two, now), undefined, "an expired spent window is not a block");
assert.ok(quotaOutNote({ ...two, windows: [win("five_hour", 1, now + hour)] }, now).includes("5 小时额度已用完"));

console.log("quota: live windows, expiry, staleness, summary and source passed");
