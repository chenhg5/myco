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
fs.writeFileSync(entry, ["waves", "boat", "islands", "sky", "noise", "fft", "world"].map((m) => `export * as ${m} from "${src}/${m}.ts";`).join("\n"));
const out = path.join(tmp, "bundle.mjs");
await build({ entryPoints: [entry], bundle: true, format: "esm", platform: "node", outfile: out, nodePaths: [path.join(here, "node_modules")], logLevel: "error" });
const { waves: W, boat: B, islands: I, sky: S, fft: F, world: WO } = await import(pathToFileURL(out).href);

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

// ---- the FFT sea's spectrum (the GPU turns it into the surface; the boat
// rides its long waves, summed here) ----
for (const sea of ["calm", "moderate", "rough"]) {
  const n = 64, s = F.spectrum(sea, 200, n, 256);
  // the variance is set by the significant wave height (Hs = 4·σ)
  let v = 0;
  for (let i = 0; i < n * n; i++) v += s.tex[i * 4] ** 2 + s.tex[i * 4 + 1] ** 2 + s.tex[i * 4 + 2] ** 2 + s.tex[i * 4 + 3] ** 2;
  assert.ok(Math.abs(4 * Math.sqrt(v) - F.SPECTRA[sea].hs) < 1e-3, `${sea}: Hs ${(4 * Math.sqrt(v)).toFixed(3)} m`);
  // h(−k) is stored as conj(h0(−k)): the field is real (no imaginary leftovers)
  const at = (i, j) => s.tex.subarray(((j % n) * n + (i % n)) * 4, ((j % n) * n + (i % n)) * 4 + 4);
  for (const [i, j] of [[3, 5], [60, 2], [17, 40]]) {
    const a = at(i, j), b = at(n - i, n - j);
    assert.ok(Math.abs(a[2] - b[0]) < 1e-12 && Math.abs(a[3] + b[1]) < 1e-12, "the mirrored wave is the conjugate");
  }
  // waves run with the wind: more energy toward the north-north-east than against it
  const e = (i, j) => { const t = at(i, j); return t[0] ** 2 + t[1] ** 2; };
  let down = 0, up = 0;
  for (let i = 1; i < 8; i++) { down += e(i, n - 3 * i); up += e(n - i, 3 * i); }
  assert.ok(down > up * 3, `${sea}: downwind ${down.toExponential(2)} vs upwind ${up.toExponential(2)}`);
}
{
  const s = F.spectrum("moderate", 200);
  const lw = new F.LongWaves(s.modes, s.chop);
  assert.ok(s.modes.length > 500 && s.modes.every((m) => (2 * Math.PI) / m.k >= 10 - 1e-9), `${s.modes.length} long waves, all 10 m or longer`);
  // the height asked for at a point is the height of the surface point that lands there
  for (const [x, z, t] of [[0, 0, 0], [33, -71, 4.2], [-500, 260, 61]]) {
    const [h, dx, dz] = lw.displace(x, z, t);
    assert.ok(Math.abs(lw.heightAt(x + dx, z + dz, t) - h) < 0.03, `long waves at (${x}, ${z})`);
  }
}

// ---- the boat ----
{
  const ws = W.makeWaves("calm", 0);
  const open = () => -30;
  const b = B.newBoat(0, 0, 0);
  const run = (inp, secs) => { for (let i = 0; i < secs * 30; i++) B.stepBoat(b, inp, 1 / 30, (x, z) => W.heightAt(ws, x, z, i / 30), open); };
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
  for (let i = 0; i < 30 * 30; i++) B.stepBoat(b, { throttle: 1, steer: 0, stop: false }, 1 / 30, (x, z) => W.heightAt(ws, x, z, i / 30), wall);
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
  const noon = S.skyLook(12, "clear"), morning = S.skyLook(7, "clear"), evening = S.skyLook(17.5, "clear");
  assert.ok(noon.sunPos.y > 0.8 && noon.sunPos.z > 0, "at noon the sun is high in the south (+z)");
  assert.ok(morning.sunPos.x > 0.5 && evening.sunPos.x < -0.5, "it rises in the east (+x) and sets in the west");
  assert.ok(Math.abs(S.skyLook(6, "clear").sunPos.y) < 0.02 && Math.abs(S.skyLook(18, "clear").sunPos.y) < 0.02, "sunrise at 6, sunset at 18");
  assert.ok(S.skyLook(12, "overcast").lightColor.r < noon.lightColor.r * 0.5, "cloud dims the sun");
  // night: the sun is down, the moon is up and gives the (faint, blue) light
  const night = S.skyLook(23, "clear");
  assert.ok(night.sunPos.y < -0.5 && night.moon.y > 0.2 && night.moon.y < 0.45, `the moon rides low at night (${night.moon.y.toFixed(2)})`);
  assert.ok(night.light.distanceTo(night.moon) < 1e-6 && night.lightColor.b > night.lightColor.r && night.lightColor.b < 0.2, "moonlight");
  assert.ok(night.day === 0 && night.stars > 0.8 && night.exposure > 2, "stars, and eyes adjusted to the dark");
  assert.ok(S.skyLook(23, "overcast").stars < 0.3, "cloud hides the stars");
  // weather
  assert.ok(S.skyLook(14, "rain").rain > 0 && S.skyLook(14, "storm").storm && !S.skyLook(14, "rain").storm);
  assert.ok(S.skyLook(9, "fog").haze > S.skyLook(9, "clear").haze * 5, "sea fog closes the distance in");
  assert.equal(S.skyLook(14, "hail").cover, S.skyLook(14, "cloudy").cover, "an unknown weather reads as cloudy");
}

// ---- the shared clock: everyone works out the same hour from the record ----
{
  const at = Date.UTC(2026, 10, 23, 8, 0, 0);
  const s = { time: 22, weather: "clear", sea: "calm", wind: 0, speed: 120, at };
  assert.equal(WO.hourOf(s, at), 22);
  assert.ok(Math.abs(WO.hourOf(s, at + 60_000) - 0) < 1e-9, "at 120× a real minute is two hours: past midnight to 0:00");
  assert.ok(Math.abs(WO.hourOf(s, at + 6 * 60_000) - 10) < 1e-9, "a day every 12 minutes");
  assert.equal(WO.hourOf({ ...s, speed: 0 }, at + 3.6e6), 22, "stopped: the hour stays");
}

fs.rmSync(tmp, { recursive: true, force: true });
console.log("PASS ocean: waves, spectrum, boat, islands, sky (day, night, weather), shared clock");
