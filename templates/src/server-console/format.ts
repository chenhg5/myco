// Display helpers preserve missing/unsupported data rather than inventing metrics.
export function percent(used: number, total: number): number | null {
  return Number.isFinite(used) && Number.isFinite(total) && total > 0
    ? Math.max(0, Math.min(100, (used / total) * 100))
    : null;
}
export function bytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let n = value,
    i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i && n < 10 ? 1 : 0)} ${units[i]}`;
}
export function duration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "—";
  const days = Math.floor(seconds / 86400),
    hours = Math.floor((seconds % 86400) / 3600),
    minutes = Math.floor((seconds % 3600) / 60);
  return days ? `${days}d ${hours}h` : `${hours}h ${minutes}m`;
}
export function chartPath(
  samples: readonly (number | null)[],
  width = 240,
  height = 48,
): string {
  // Missing samples start a new segment; do not bridge over disconnects.
  let path = "",
    gap = true;
  samples.forEach((v, i) => {
    if (v === null || !Number.isFinite(v)) {
      gap = true;
      return;
    }
    const x =
        samples.length < 2 ? width / 2 : (i / (samples.length - 1)) * width,
      y = height - (Math.max(0, Math.min(100, v)) / 100) * height;
    path += `${gap ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)} `;
    gap = false;
  });
  return path.trim();
}
