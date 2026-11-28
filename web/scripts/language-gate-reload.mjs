/** Only expected WebKit cancellations from an explicitly retired test Page. */
export function reloadCancellation(text, base, retired) {
  if (!retired) return null;
  const prefix = `Fetch API cannot load ${base}`;
  const suffix = " due to access control checks.";
  if (!text.startsWith(prefix)) return null;
  const end = text.indexOf(suffix, prefix.length);
  if (end < 0) return null;
  const resource = text.slice(prefix.length, end).split("?")[0];
  if (!resource.startsWith("/app/") || /^\/app\/(?:preferences|ui-lang)(?:\/|$)/.test(resource)) return null;
  return resource;
}
