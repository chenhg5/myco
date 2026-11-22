/*
 * The size of each picture seen in this window (by its address or file
 * id): drawn again (another workspace and back, a chat reopened, a pane
 * switched), it takes its room at once instead of growing as it loads,
 * which moved everything under it (and, a chat kept at its foot, every
 * message above it). Kept while the App is open, the newest few hundred.
 */
const sizes = new Map<string, [number, number]>();

export function knownSize(key: string): [number, number] | undefined {
  return key ? sizes.get(key) : undefined;
}

export function noteSize(key: string, w: number, h: number) {
  if (!key || !w || !h) return;
  const was = sizes.get(key);
  if (was && was[0] === w && was[1] === h) return;
  sizes.delete(key);
  sizes.set(key, [w, h]);
  if (sizes.size > 600) sizes.delete(sizes.keys().next().value as string);
}
