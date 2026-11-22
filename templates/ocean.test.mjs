// 航海's engine without a browser: the waves (the height the boat feels
// is the surface the GPU draws), the boat (speed, turning, running
// aground), the islands (shapes, shelves, palms) and the sky's compass.
// Run: node ocean.test.mjs
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { build } from "esbuild";

const here = path.dirname(fileURLToPath(import.meta.url));
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "myco-ocean-test-"));
const entry = path.join(tmp, "entry.ts");
const src = path.join(here, "src/ocean").replace(/\\/g, "/");
fs.writeFileSync(entry, ["waves", "boat", "islands", "sky", "noise"].map((m) => `export * as ${m} from "${src}/${m}.ts";`).join("\n"));
const out = path.join(tmp, "bundle.mjs");
await build({ entryPoints: [entry], bundle: true, format: "esm", platform: "node", outfile: out, nodePaths: [path.join(here, "node_modules")], logLevel: "error" });
const { waves: W, boat: B, islands: I, sky: S } = await import(pathToFileURL(out).href);

// ---- waves ----
for (const sea of ["calm", "moderate", "rough"]) {
  const ws = W.makeWaves(sea, 225);
  assert.equal(ws.length, W.GEO_WAVES);
  const sharp = ws.reduce((t, w) => t + w.q * w.k * w.a, 0);
  assert.ok(Math.abs(sharp - W.SEAS[sea].chop) < 1e-9 && sharp < 1, `crests never fold (${sea}: ${sharp})`);
  assert.deepEqual(W.makeWaves(sea, 225), ws, "the same sea for everyone");
}
{
  const ws = W.makeWaves("moderate", 200);
  // the height found for a point is the height of the surface drawn there
  for (const [x, z, t] of [[0, 0, 0], [12.5, -40, 3.3], [-300, 220, 17.9], [1000, 1000, 120]]) {
    const [px, py, pz] = W.displace(ws, x, z, t);
    assert.ok(Math.abs(W.heightAt(ws, px, pz, t) - py) < 0.02, `height at (${px.toFixed(1)}, ${pz.toFixed(1)})`);
  }
  // waves go where the wind blows to: from 200° (SSW) toward NNE, so north (−z) and a little east
  const big = ws[0];
  assert.ok(big.dz < -0.5, `the longest wave runs north-ish (${big.dx.toFixed(2)}, ${big.dz.toFixed(2)})`);
  // a rough sea is taller than a calm one
  const range = (sea) => { const w = W.makeWaves(sea, 0); let lo = 0, hi = 0; for (let i = 0; i < 400; i++) { const h = W.heightAt(w, i * 7.3, i * 3.1, i * 0.37); lo = Math.min(lo, h); hi = Math.max(hi, h); } return hi - lo; };
  assert.ok(range("rough") > range("calm") * 2.5, `rough ${range("rough").toFixed(2)} m vs calm ${range("calm").toFixed(2)} m`);
}

// ---- the boat ----
{
  const ws = W.makeWaves("calm", 0);
  const open = () => -30;
  const b = B.newBoat(0, 0, 0);
  const run = (inp, secs) => { for (let i = 0; i < secs * 30; i++) B.stepBoat(b, inp, 1 / 30, ws, i / 30, open); };
  run({ throttle: 1, steer: 0, stop: false }, 40);
  assert.equal(b.throttle, 1, "the lever goes to full");
  assert.ok(b.speed > B.MAX_SPEED * 0.9 && b.speed < B.MAX_SPEED * 1.1, `top speed ${b.speed.toFixed(1)} m/s`);
  assert.ok(b.z < -300 && Math.abs(b.x) < 1, `heading north is going to −z (${b.x.toFixed(1)}, ${b.z.toFixed(1)})`);
  const h0 = b.heading;
  run({ throttle: 0, steer: 1, stop: false }, 3);
  const turned = ((b.heading - h0 + Math.PI * 3) % (Math.PI * 2)) - Math.PI;
  assert.ok(turned > 0.3, `right rudder turns clockwise (${turned.toFixed(2)} rad)`);
  run({ throttle: 0, steer: 0, stop: true }, 60);
  assert.equal(b.throttle, 0);
  assert.ok(Math.abs(b.speed) < 0.3, `cut the engine and it drifts to a stop (${b.speed.toFixed(2)})`);
  assert.ok(Math.abs(b.y) < 1.5 && Math.abs(b.pitch) < 0.3 && Math.abs(b.roll) < 0.3, "floats level enough on a calm sea");
}
{
  // a wall of ground north of z = −50: the boat stops short of it
  const ws = W.makeWaves("calm", 0);
  const wall = (x, z) => (z < -50 ? 5 : -30);
  const b = B.newBoat(0, 0, 0);
  for (let i = 0; i < 30 * 30; i++) B.stepBoat(b, { throttle: 1, steer: 0, stop: false }, 1 / 30, ws, i / 30, wall);
  assert.ok(b.z > -50, `never inside the ground (${b.z.toFixed(1)})`);
  assert.ok(b.aground < 30, "it knows it touched");
}

// ---- islands ----
{
  const sample = JSON.parse(fs.readFileSync(path.join(here, "src/ocean/public/sample.json"), "utf8"));
  const isles = sample.islands.map(({ id, ...d }) => I.isleOf(id, d));
  const shapes = isles.map((i) => new I.Shape(i));
  for (const s of shapes) {
    let top = -Infinity;
    for (let i = -20; i <= 20; i++) for (let j = -20; j <= 20; j++) top = Math.max(top, s.at(s.isle.x + (i / 20) * s.isle.size, s.isle.z + (j / 20) * s.isle.size));
    assert.ok(top > 0.3, `${s.isle.name} stands out of the water (${top.toFixed(1)} m)`);
    assert.ok(s.at(s.isle.x + s.reach + 5, s.isle.z) < -20, `${s.isle.name}: open sea beyond its shelf`);
    const mask = I.landMask(s, Math.max(1.5, s.reach / 90));
    assert.ok(mask.cells.some((c) => c === 2), `${s.isle.name} shows on the map`);
  }
  assert.ok(I.groundAt(shapes, 0, 0) < -10, "the start is in deep water");
  // palms: on the island's land, the same each time
  const coco = shapes.find((s) => s.isle.id === "coco");
  const palms = I.palmSpots(coco);
  assert.ok(palms.length > 30, `椰子岛 has palms (${palms.length})`);
  assert.ok(palms.every((p) => coco.at(p.x, p.z) > 1), "palms stand on land");
  assert.deepEqual(I.palmSpots(coco), palms);
  assert.equal(I.palmSpots(shapes.find((s) => s.isle.kind === "stack")).length, 0, "no palms on a sea stack");
  // records with missing or odd values still make islands
  const odd = I.isleOf("x1", { kind: "volcano", size: -5, height: "tall", x: "3" });
  assert.equal(odd.kind, "island");
  assert.ok(odd.size >= 4 && odd.height > 0 && odd.x === 0);
  assert.notEqual(I.isleOf("a", {}).seed, I.isleOf("b", {}).seed, "the seed comes from the id");
}

// ---- the sky ----
{
  const noon = S.skyLook(12, "clear"), morning = S.skyLook(7, "clear"), evening = S.skyLook(18, "clear");
  assert.ok(noon.sun.y > 0.8 && noon.sun.z > 0, "at noon the sun is high in the south (+z)");
  assert.ok(morning.sun.x > 0.5 && evening.sun.x < -0.5, "it rises in the east (+x) and sets in the west");
  assert.ok(S.skyLook(12, "overcast").sunColor.r < noon.sunColor.r * 0.5, "cloud dims the sun");
  assert.ok(S.skyLook(3, "clear").sun.y > 0, "no night: the hour is kept in daylight");
}

fs.rmSync(tmp, { recursive: true, force: true });
console.log("PASS ocean: waves, boat, islands, sky");
