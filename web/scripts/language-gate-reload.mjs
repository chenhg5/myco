/** Only expected WebKit cancellations from a test-requested reload. */
export function reloadCancellation(text, base, reloading) {
  if (!reloading) return null;
  const prefix = `Fetch API cannot load ${base}`;
  const suffix = " due to access control checks.";
  if (!text.startsWith(prefix)) return null;
  const end = text.indexOf(suffix, prefix.length);
  if (end < 0) return null;
  const resource = text.slice(prefix.length, end).split("?")[0];
  if (!resource.startsWith("/app/") || /^\/app\/(?:preferences|ui-lang)(?:\/|$)/.test(resource)) return null;
  return resource;
}
