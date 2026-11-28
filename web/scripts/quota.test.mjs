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
          ? 'export const timeAgo = (iso, now = Date.now()) => { const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000)); if (s < 60) return "刚刚"; if (s < 3600) return `${Math.round(s / 60)} 分钟前`; return `${Math.round(s / 3600)} 小时前`; }; export const cn = (...x) => x.filter(Boolean).join(" ");'
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
const { quotaPillLevel, quotaLiveWindows, quotaHasNumbers, quotaEffectiveStatus, quotaStatusText, quotaSummary, quotaSourceText, quotaOldNote, quotaOutNote, quotaWorst } = await import(
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

// Hour-old numbers stay (a cloud session cannot refresh them), greyed and
// with their age, but never as the account's current use or as ok.
const stale = quotaSummary({ ...two, at: iso(now - 2 * hour), checked_at: iso(now - 2 * hour) }, [], now);
assert.equal(stale.level, "old");
assert.equal(stale.short, "剩 80% · 2 小时前");
assert.ok(stale.line.includes("剩 80%"), stale.line);
assert.ok(stale.line.includes("可能已过时"), stale.line);
assert.equal(stale.stale, true);
assert.equal(quotaOldNote({ ...two, at: iso(now - 2 * hour), checked_at: iso(now - 2 * hour) }, now), "上次更新：2 小时前，可能已过时");
// (the snapshot's collection time is the age, even when a weekly window
// kept an older observation time)
const fromProfile = quotaSummary({ ...two, profile_ref: "qp_x", at: iso(now - 20 * hour), checked_at: iso(now - 3 * hour) }, [], now);
assert.equal(fromProfile.level, "old");
assert.ok(fromProfile.short.includes("3 小时前"), fromProfile.short);
// Old numbers never turn into today's red warning.
const staleSpent = quotaSummary({ ...two, at: iso(now - 2 * hour), checked_at: iso(now - 2 * hour), windows: [win("five_hour", 1, now + hour)] }, [], now);
assert.equal(staleSpent.level, "old");
assert.ok(staleSpent.short.startsWith("额度用完 · "), staleSpent.short);
// A login to fix keeps its own words and colour.
const auth = quotaSummary({ at: iso(now - 26 * hour), checked_at: iso(now - 5 * 60e3), status: "auth_required", windows: [win("five_hour", 0.3)] }, [], now);
assert.equal(auth.level, "action");
assert.equal(auth.short, "需重新登录");
const fresh = quotaSummary(two, [], now);
assert.equal(fresh.level, "ok");
assert.equal(fresh.short, "剩 80%");
assert.ok(!fresh.short.includes("前"), fresh.short);
assert.ok(fresh.line.includes("5 小时"), fresh.line);
assert.ok(!fresh.line.includes("每周"), fresh.line);
const tight = quotaSummary({ ...two, windows: [win("five_hour", 0.95, now + hour)] }, [], now);
assert.equal(tight.level, "tight");
// A window whose reset has passed leaves nothing to show: unknown, never
// yesterday's numbers as a stale full quota.
const expired = quotaSummary({ at: iso(now - 2 * hour), checked_at: iso(now - 2 * hour), status: "ok", windows: [win("five_hour", 0.2, now - hour)] }, [], now);
assert.equal(expired.level, "failed");
assert.equal(expired.short, "额度未知");
assert.ok(!expired.line.includes("剩 "), expired.line);

// Where the numbers came from, and that a reset window is not "used up".
assert.equal(quotaSourceText({ ...two, source_kind: "statusline", provider: "claude" }), "Claude · 状态栏上报");
assert.equal(quotaOutNote(two, now), undefined, "an expired spent window is not a block");
assert.ok(quotaOutNote({ ...two, windows: [win("five_hour", 1, now + hour)] }, now).includes("5 小时额度已用完"));

console.log("quota: live windows, expiry, staleness, summary and source passed");

// Claude warns before a window is spent. Both old and normalized states
// remain available and visible as near-limit even below the 90% fallback.
for (const status of ["warning", "allowed_warning"]) {
  const warning = quotaSummary({ ...two, windows: [{ ...win("seven_day", .81, now + hour), status }] }, [], now);
  assert.equal(warning.level, "tight");
  assert.equal(warning.short, "剩 19%");
  assert.ok(warning.line.includes("接近上限"), warning.line);
  assert.ok(!warning.line.includes("用完"), warning.line);
}

assert.equal(quotaPillLevel({ name: "seven_day", used: .76, status: "warning" }), "low");
