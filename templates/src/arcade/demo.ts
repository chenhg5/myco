// Orbit Courier: an original CHIP-8 test cartridge. Move the shuttle with
// 2/4/6/8, catch the beacon, then follow it to the next spot.
export function demoROM() {
  const bytes: number[] = [];
  const labels = new Map<string, number>();
  const jumps: { at: number; code: number; label: string }[] = [];
  const emit = (...ops: number[]) => ops.forEach(op => bytes.push(op >> 8, op & 255));
  const label = (name: string) => { labels.set(name, 0x200 + bytes.length); };
  const target = (code: number, name: string) => { jumps.push({ at: bytes.length, code, label: name }); emit(0); };

  emit(0x6009, 0x6111, 0x622c, 0x630b, 0x6800);
  label("frame");
  emit(0x00e0);
  target(0xa000, "beacon"); emit(0xd235);
  target(0xa000, "shuttle"); emit(0xd015);
  emit(0x3f01); target(0x1000, "score");
  emit(0x7801, 0x4810, 0x6800, 0xc23f, 0xc31f, 0x6404, 0xf418);
  label("score");
  emit(0x6c02, 0x6d02, 0xf829, 0xdcd5);
  emit(0x6403, 0xf415);
  label("wait");
  emit(0xf407, 0x3400); target(0x1000, "wait");
  emit(0x6404, 0xe4a1, 0x70ff, 0x6406, 0xe4a1, 0x7001, 0x6402, 0xe4a1, 0x71ff, 0x6408, 0xe4a1, 0x7101);
  emit(0x643f, 0x8042, 0x641f, 0x8142);
  target(0x1000, "frame");
  label("shuttle"); bytes.push(0x20, 0x70, 0xf8, 0x70, 0x50);
  label("beacon"); bytes.push(0x70, 0x88, 0xa8, 0x88, 0x70);
  for (const jump of jumps) {
    const op = jump.code | labels.get(jump.label)!;
    bytes[jump.at] = op >> 8;
    bytes[jump.at + 1] = op & 255;
  }
  return Uint8Array.from(bytes);
}
