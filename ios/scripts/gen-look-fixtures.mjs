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
// The part that decides a face: from the cap colours to just before drawing.
const start = source.indexOf("const CAPS");
const end = source.indexOf("const FILL");
if (start < 0 || end < start) throw new Error("persona.tsx no longer has the part between CAPS and FILL");

const seeds = [
  "", "?", "a", "agt_52w7ewbmgalpumqb", "agt_yfww5cuhqre2ypbt", "后端工程师", "🙂 emoji", "studio-claude-2b9c",
  ...Array.from({ length: 40 }, (_, i) => `ag_${(i * 7919).toString(36)}`),
];
const avatars = [
  "look:c3-p1-f2-s1", "look:c0-p0-f0-s0", "look:c9-p2-f3-s1", "look:c10-p0-f0-s0", "look:c1-p3-f0-s0",
  "look:c1-p0-f4-s0", "look:c1-p0-f0-s2", "look:c1-p0-f0", "pic:abcdef", "", "look:cx-p0-f0-s0",
];
const program = `${source.slice(start, end)}
const seeds = ${JSON.stringify(seeds)};
const avatars = ${JSON.stringify(avatars)};
console.log(JSON.stringify({
  seeds: seeds.map((seed) => ({ seed, ...lookOfSeed(seed), word: lookWord(lookOfSeed(seed)), color: CAPS[lookOfSeed(seed).cap] })),
  avatars: avatars.map((avatar) => ({ avatar, look: parseLook(avatar) })),
}, null, 1));
`;
const dir = mkdtempSync(join(tmpdir(), "myco-looks-"));
try {
  const file = join(dir, "looks.ts");
  writeFileSync(file, program);
  const out = execFileSync(process.execPath, ["--experimental-strip-types", "--no-warnings", file], { encoding: "utf8" });
  writeFileSync(join(here, "../MycoKit/Tests/MycoUITests/Fixtures/looks.json"), out);
  console.log(`wrote ${JSON.parse(out).seeds.length} seeds and ${JSON.parse(out).avatars.length} avatars`);
} finally {
  rmSync(dir, { recursive: true, force: true });
}
