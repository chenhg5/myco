// Orders for a list of records in the order they are shown: as many as can
// keep theirs do (the longest increasing run of the ones they have), the
// others get numbers between their neighbours'. So moving one block writes
// that block, not every block after it.
export function assignOrders(ids: string[], have: Map<string, number>): number[] {
  const vals = ids.map((id) => have.get(id));
  // longest strictly increasing subsequence of the defined values
  const tails: number[] = [], prev = new Array<number>(ids.length).fill(-1), at: number[] = [];
  vals.forEach((v, i) => {
    if (v === undefined || !isFinite(v)) return;
    let lo = 0, hi = tails.length;
    while (lo < hi) { const mid = (lo + hi) >> 1; if ((vals[at[mid]] as number) < v) lo = mid + 1; else hi = mid; }
    if (lo > 0) prev[i] = at[lo - 1];
    at[lo] = i;
    tails[lo] = v;
  });
  const keep = new Set<number>();
  for (let i = at[tails.length - 1] ?? -1; i >= 0; i = prev[i]) keep.add(i);
  const out = new Array<number>(ids.length);
  let i = 0;
  while (i < ids.length) {
    if (keep.has(i)) { out[i] = vals[i] as number; i++; continue; }
    let j = i;
    while (j < ids.length && !keep.has(j)) j++;
    const lo = i > 0 ? out[i - 1] : undefined, hi = j < ids.length ? (vals[j] as number) : undefined, k = j - i;
    for (let n = 0; n < k; n++) {
      out[i + n] = lo !== undefined && hi !== undefined ? lo + ((hi - lo) * (n + 1)) / (k + 1)
        : lo !== undefined ? lo + n + 1
        : hi !== undefined ? hi - (k - n)
        : n + 1;
    }
    i = j;
  }
  return out;
}
