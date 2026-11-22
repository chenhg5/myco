import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import ts from "typescript";

async function load(name) {
  const source = readFileSync(new URL(`./src/arcade/${name}.ts`, import.meta.url), "utf8");
  const js = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } }).outputText;
  return import("data:text/javascript;base64," + Buffer.from(js).toString("base64"));
}
const { Chip8 } = await load("chip8");
const { demoROM } = await load("demo");
const rom = (...ops) => Uint8Array.from(ops.flatMap(op => [op >> 8, op & 255]));
const arithmetic = new Chip8(rom(0x60ff, 0x6101, 0x8014, 0x622a, 0xa350, 0xf233));
for (let i = 0; i < 6; i++) arithmetic.step();
assert.equal(arithmetic.v[0], 0);
assert.equal(arithmetic.v[15], 1);
assert.deepEqual([...arithmetic.memory.subarray(0x350, 0x353)], [0, 4, 2]);

const input = new Chip8(rom(0xf30a, 0x7301));
assert.equal(input.step(), false);
input.key(6, true);
assert.equal(input.step(), false);
input.key(6, false);
assert.equal(input.step(), true);
assert.equal(input.v[3], 7);

for (const classic of [true, false]) {
  const machine = new Chip8(rom(0x6008, 0x6102, 0x8016, 0xa300, 0xf155), classic);
  for (let i = 0; i < 5; i++) machine.step();
  assert.equal(machine.v[0], classic ? 1 : 4);
  assert.equal(machine.i, classic ? 0x302 : 0x300);
}

const demo = new Chip8(demoROM());
function frames(count) {
  for (let frame = 0; frame < count; frame++) {
    demo.tick();
    for (let i = 0; i < 12 && demo.step(); i++) {}
  }
}
frames(20);
assert.ok(demo.screen.includes(1));
const x = demo.v[0];
demo.key(6, true);
frames(30);
demo.key(6, false);
assert.ok(demo.v[0] > x);
assert.throws(() => new Chip8(rom(0xffff)).step(), /FFFF/);
assert.throws(() => new Chip8(new Uint8Array(3585)), /ROM size/);

// These upstream test cartridges are optional and are never bundled.
if (process.env.CHIP8_TEST_DIR) {
  for (const file of ["1-chip8-logo.ch8", "2-ibm-logo.ch8", "3-corax+.ch8", "4-flags.ch8"]) {
    const core = new Chip8(readFileSync(`${process.env.CHIP8_TEST_DIR}/${file}`), true);
    for (let frame = 0; frame < 1000; frame++) {
      core.tick();
      for (let i = 0; i < 16 && core.step(); i++) {}
    }
    assert.ok(core.screen.includes(1), file);
    if (process.env.CHIP8_DUMP) {
      console.log(file);
      for (let y = 0; y < 32; y++) console.log(Array.from(core.screen.subarray(y * 64, y * 64 + 64), n => n ? "█" : " ").join(""));
    }
  }
}
console.log("PASS CHIP-8 registers, timers and demo movement, keyboard wait, dialects and errors");
