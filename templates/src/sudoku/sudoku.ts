// Making and checking sudoku: a full grid by backtracking with shuffled
// digits, then digits taken away while the puzzle keeps one answer.
export type Grid = number[]; // 81 cells, 0 for empty

const rnd = <T,>(a: T[]) => { for (let i = a.length - 1; i > 0; i--) { const j = Math.floor(Math.random() * (i + 1)); [a[i], a[j]] = [a[j], a[i]]; } return a; };
export const rowOf = (i: number) => Math.floor(i / 9), colOf = (i: number) => i % 9, boxOf = (i: number) => Math.floor(rowOf(i) / 3) * 3 + Math.floor(colOf(i) / 3);
const PEERS: number[][] = Array.from({ length: 81 }, (_, i) => {
  const s = new Set<number>();
  for (let j = 0; j < 81; j++) if (j !== i && (rowOf(j) === rowOf(i) || colOf(j) === colOf(i) || boxOf(j) === boxOf(i))) s.add(j);
  return [...s];
});
export const peers = (i: number) => PEERS[i];
const ok = (g: Grid, i: number, v: number) => PEERS[i].every((j) => g[j] !== v);

function fill(g: Grid): boolean {
  const i = g.indexOf(0);
  if (i < 0) return true;
  for (const v of rnd([1, 2, 3, 4, 5, 6, 7, 8, 9])) { if (ok(g, i, v)) { g[i] = v; if (fill(g)) return true; g[i] = 0; } }
  return false;
}
/** How many answers (stopping at 2). */
export function count(g: Grid, limit = 2): number {
  let best = -1, bestN = 10;
  for (let i = 0; i < 81; i++) if (!g[i]) {
    let n = 0;
    for (let v = 1; v <= 9; v++) if (ok(g, i, v)) n++;
    if (n < bestN) { best = i; bestN = n; if (n <= 1) break; }
  }
  if (best < 0) return 1;
  let total = 0;
  for (let v = 1; v <= 9 && total < limit; v++) if (ok(g, best, v)) { g[best] = v; total += count(g, limit - total); g[best] = 0; }
  return total;
}
export function solve(givens: Grid): Grid | null {
  const g = givens.slice();
  const go = (): boolean => {
    let best = -1, bestN = 10;
    for (let i = 0; i < 81; i++) if (!g[i]) { let n = 0; for (let v = 1; v <= 9; v++) if (ok(g, i, v)) n++; if (n < bestN) { best = i; bestN = n; } }
    if (best < 0) return true;
    for (let v = 1; v <= 9; v++) if (ok(g, best, v)) { g[best] = v; if (go()) return true; g[best] = 0; }
    return false;
  };
  return go() ? g : null;
}
export const LEVELS: [string, string, number][] = [["easy", "简单", 38], ["medium", "中等", 31], ["hard", "困难", 26]];
export function make(level: string): Grid {
  const target = LEVELS.find((l) => l[0] === level)?.[2] ?? 31;
  const g: Grid = Array(81).fill(0);
  fill(g);
  let left = 81;
  for (const i of rnd([...Array(81).keys()])) {
    if (left <= target) break;
    const v = g[i];
    g[i] = 0;
    if (count(g.slice()) !== 1) g[i] = v; else left--;
  }
  return g;
}
export const parse = (s: string): Grid => Array.from({ length: 81 }, (_, i) => { const c = s[i]; return c >= "1" && c <= "9" ? Number(c) : 0; });
export const show = (g: Grid) => g.map((v) => (v ? String(v) : ".")).join("");
