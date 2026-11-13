/** Whether a hub's version is at least want ("0.19.2"). Unknown is older. */
export function hubAtLeast(version: string | undefined, want: string): boolean {
  if (!version) return false;
  const a = version.replace(/^v/i, "").split(/[.+-]/).map((x) => Number.parseInt(x, 10) || 0);
  const b = want.split(".").map((x) => Number.parseInt(x, 10) || 0);
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const x = a[i] ?? 0;
    const y = b[i] ?? 0;
    if (x !== y) return x > y;
  }
  return true;
}

/** Whether a hub carries agents' resume lines (and the routes that make and
 *  revoke them, from v0.19.5). A build from a later main reports a version
 *  like `v0.19.4-13-g8484653`, above the last release without a new tag, and
 *  has them too; released hubs up to v0.19.4 do not. */
export function hubHasResume(version: string | undefined): boolean {
  if (hubAtLeast(version, "0.19.5")) return true;
  return !!version && /-g[0-9a-f]{4,}$/i.test(version) && hubAtLeast(version, "0.19.4");
}
