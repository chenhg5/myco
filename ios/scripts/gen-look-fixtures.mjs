// Writes MycoKit/Tests/MycoUITests/Fixtures/looks.json from the desktop
// app's own code (web/src/app/persona.tsx), so the Swift port of an agent's
// face is checked against the original, not against a copy of it.
//
//   node ios/scripts/gen-look-fixtures.mjs
//
// Needs Node 22.6 or later (it runs the TypeScript as it is).
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(join(here, "../../web/src/app/persona.tsx"), "utf8");
// The parts that decide a face: from the cap colours to just before the
// colours it is drawn in, what it wears, and how a workspace spreads faces.
function part(from, to) {
  const start = source.indexOf(from);
  const end = source.indexOf(to, start);
  if (start < 0 || end < start) throw new Error(`persona.tsx no longer has the part from ${from} to ${to}`);
  return source.slice(start, end);
}
const code = [
  part("const CAPS", "const FILL"),
  part("function accessoryPixels", "export function MushroomFace"),
  part("export function rememberWorkspaceFaces", "// A workspace"),
].join("\n");

const seeds = [
  "", "?", "a", "agt_52w7ewbmgalpumqb", "agt_yfww5cuhqre2ypbt", "后端工程师", "🙂 emoji", "studio-claude-2b9c",
  ...Array.from({ length: 40 }, (_, i) => `ag_${(i * 7919).toString(36)}`),
];
const avatars = [
  "look:c3-p1-f2-s1", "look:c0-p0-f0-s0", "look:c9-p2-f3-s1", "look:c10-p0-f0-s0", "look:c1-p3-f0-s0",
  "look:c1-p0-f4-s0", "look:c1-p0-f0-s2", "look:c1-p0-f0", "pic:abcdef", "", "look:cx-p0-f0-s0",
  "look:c2-p1-f1-s0-h3-a6", "look:c4-p2-f0-s1-h0-a1", "look:c4-p2-f0-s0-h1-a4", "look:c5-p0-f2-s0-h5-a7",
  "look:c5-p0-f2-s0-h6-a0", "look:c5-p0-f2-s0-h0-a8", "look:c5-p0-f2-s0-h2", "look:c5-p0-f2-s0-hx-a1",
  "look:c0-p0-f0-s1-h0-a0", "look:c7-p1-f3-s0-h4-a4",
];
// Every shape with every accessory (also the ones that do not fit, which
// draw none), on a few colours, spots and faces.
const drawn = [];
for (let shape = 0; shape < 6; shape++) {
  for (let acc = 0; acc < 8; acc++) {
    const n = shape * 8 + acc;
    drawn.push(`look:c${n % 10}-p${n % 3}-f${n % 4}-s${acc === 1 ? 1 : 0}-h${shape}-a${acc}`);
  }
}
// One workspace, in the order the hub lists it: more agents than there are
// colours and shapes to go round without clashes, a chosen face, a picture,
// and two that joined at the same moment.
const members = Array.from({ length: 70 }, (_, i) => ({
  id: `agt_${hashless(i)}`,
  created_at: new Date(Date.UTC(2026, 10, 1, 8, 0, 0) + (i === 41 ? 40 : i) * 60_000).toISOString().replace(".000Z", "Z"),
}));
members[5].avatar = "look:c3-p0-f0-s0";
members[9].avatar = "pic:abcdef";
members[12].avatar = "look:c3-p0-f0-s0-h2-a5";
members.reverse();
function hashless(i) {
  return ((i + 1) * 2654435761 % 4294967296).toString(36).padStart(7, "0");
}
const program = `${code}
const seeds = ${JSON.stringify(seeds)};
const avatars = ${JSON.stringify(avatars)};
const drawn = ${JSON.stringify(drawn)};
const members = ${JSON.stringify(members)};
// A face as the desktop draws it: 14 rows of 12 (y = -2 at the top), each
// pixel a part's letter, an accessory's colour, or null where it is clear.
function grid(seed, avatar) {
  const l = looksOf(seed, avatar);
  const rows = [...l.rows, ...l.face].map((row) => [...row].map((k) => (k === "." ? null : k)));
  for (const [x, y, c] of accessoryPixels(l.acc, l.shape)) rows[y + 2][x] = c;
  return rows;
}
const parts = (l) => l && { cap: l.cap, pattern: l.pattern, face: l.face, shape: l.shape, acc: l.acc };
const out = {
  seeds: seeds.map((seed) => ({ seed, ...parts(lookOfSeed(seed)), word: lookWord(lookOfSeed(seed)), color: CAPS[lookOfSeed(seed).cap], grid: grid(seed) })),
  avatars: avatars.map((avatar) => ({ avatar, look: parts(parseLook(avatar)) })),
  drawn: drawn.map((avatar) => ({ avatar, grid: grid("x", avatar) })),
};
rememberWorkspaceFaces(members);
out.workspace = members.map((m) => ({ ...m, ...parts(lookOf(m.id, m.avatar)), grid: m.avatar?.startsWith("pic:") ? null : grid(m.id, m.avatar) }));
console.log(JSON.stringify(out));
`;
// One line per face, so a diff of the fixture reads.
function compact(all) {
  const lines = Object.entries(all).map(([key, list]) => `"${key}": [\n${list.map((x) => " " + JSON.stringify(x)).join(",\n")}\n]`);
  return `{\n${lines.join(",\n")}\n}\n`;
}
const dir = mkdtempSync(join(tmpdir(), "myco-looks-"));
try {
  const file = join(dir, "looks.ts");
  writeFileSync(file, program);
  const out = execFileSync(process.execPath, ["--experimental-strip-types", "--no-warnings", file], { encoding: "utf8" });
  writeFileSync(join(here, "../MycoKit/Tests/MycoUITests/Fixtures/looks.json"), compact(JSON.parse(out)));
  const all = JSON.parse(out);
  console.log(`wrote ${all.seeds.length} seeds, ${all.avatars.length} avatars, ${all.drawn.length} drawn faces and a workspace of ${all.workspace.length}`);
} finally {
  rmSync(dir, { recursive: true, force: true });
}
