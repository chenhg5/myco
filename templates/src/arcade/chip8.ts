// Original CHIP-8 interpreter, with classic and modern opcode behavior.
export const width = 64;
export const height = 32;
const entry = 0x200;
export const maxROM = 4096 - entry;
const font = "f0909090f02060202070f010f080f0f010f010f09090f01010f080f010f0f080f090f0f010204040f090f090f0f090f010f0f090f09090e090e090e0f0808080f0e0909090e0f080f080f0f080f08080";

export class Chip8 {
  memory = new Uint8Array(4096);
  screen = new Uint8Array(width * height);
  v = new Uint8Array(16);
  keys = new Uint8Array(16);
  stack: number[] = [];
  pc = entry;
  i = 0;
  delay = 0;
  sound = 0;
  dirty = true;
  private waiting?: { register: number; key: number };
  private drawReady = true;

  constructor(rom: Uint8Array, readonly classic = false) {
    if (rom.length < 2 || rom.length > maxROM) throw new Error("ROM size");
    this.memory.set(Uint8Array.from(font.match(/../g)!, byte => parseInt(byte, 16)), 0x50);
    this.memory.set(rom, entry);
  }

  key(index: number, down: boolean) {
    if (index < 0 || index > 15) return;
    this.keys[index] = Number(down);
    if (this.waiting && down && this.waiting.key < 0) this.waiting.key = index;
    if (this.waiting && !down && this.waiting.key === index) {
      this.v[this.waiting.register] = index;
      this.waiting = undefined;
    }
  }

  tick() {
    if (this.delay > 0) this.delay--;
    if (this.sound > 0) this.sound--;
    this.drawReady = true;
  }

  // False means the interpreter is waiting for the next frame or input.
  step(): boolean {
    if (this.waiting) return false;
    if (this.pc < 0 || this.pc + 1 >= this.memory.length) throw new Error("PC out of range");
    const op = (this.memory[this.pc] << 8) | this.memory[this.pc + 1];
    const x = (op >> 8) & 15, y = (op >> 4) & 15, n = op & 15, byte = op & 255, address = op & 4095;
    const v = this.v, vx = v[x], vy = v[y];
    const invalid = () => { throw new Error(op.toString(16).toUpperCase().padStart(4, "0")); };
    const check = (count: number) => { if (this.i + count > this.memory.length) throw new Error("Memory out of range"); };
    const flag = (value: number, carry: boolean) => { v[x] = value; v[15] = Number(carry); };
    this.pc += 2;
    switch (op >> 12) {
      case 0:
        if (op === 0x00e0) { this.screen.fill(0); this.dirty = true; }
        else if (op === 0x00ee && this.stack.length) this.pc = this.stack.pop()!;
        else invalid();
        break;
      case 1: this.pc = address; break;
      case 2:
        if (this.stack.length >= 16) throw new Error("Stack overflow");
        this.stack.push(this.pc); this.pc = address; break;
      case 3: if (vx === byte) this.pc += 2; break;
      case 4: if (vx !== byte) this.pc += 2; break;
      case 5: if (n) invalid(); if (vx === vy) this.pc += 2; break;
      case 6: v[x] = byte; break;
      case 7: v[x] += byte; break;
      case 8: {
        const shift = this.classic ? vy : vx;
        switch (n) {
          case 0: v[x] = vy; break;
          case 1: case 2: case 3:
            v[x] = n === 1 ? vx | vy : n === 2 ? vx & vy : vx ^ vy;
            if (this.classic) v[15] = 0;
            break;
          case 4: flag(vx + vy, vx + vy > 255); break;
          case 5: flag(vx - vy, vx >= vy); break;
          case 6: flag(shift >> 1, !!(shift & 1)); break;
          case 7: flag(vy - vx, vy >= vx); break;
          case 14: flag(shift << 1, !!(shift & 128)); break;
          default: invalid();
        }
        break;
      }
      case 9: if (n) invalid(); if (vx !== vy) this.pc += 2; break;
      case 10: this.i = address; break;
      case 11: this.pc = address + v[this.classic ? 0 : x]; break;
      case 12: v[x] = Math.floor(Math.random() * 256) & byte; break;
      case 13: {
        if (!n) invalid();
        if (this.classic && !this.drawReady) { this.pc -= 2; return false; }
        check(n);
        this.drawReady = false;
        v[15] = 0;
        for (let row = 0; row < n && (vy % height) + row < height; row++) {
          for (let col = 0; col < 8 && (vx % width) + col < width; col++) {
            if (!(this.memory[this.i + row] & (128 >> col))) continue;
            const pixel = ((vy % height) + row) * width + (vx % width) + col;
            if (this.screen[pixel]) v[15] = 1;
            this.screen[pixel] ^= 1;
          }
        }
        this.dirty = true;
        break;
      }
      case 14:
        if (byte !== 0x9e && byte !== 0xa1) invalid();
        if (!!this.keys[vx] === (byte === 0x9e)) this.pc += 2;
        break;
      case 15:
        switch (byte) {
          case 0x07: v[x] = this.delay; break;
          case 0x0a: this.waiting = { register: x, key: -1 }; return false;
          case 0x15: this.delay = vx; break;
          case 0x18: this.sound = vx; break;
          case 0x1e: this.i += vx; break;
          case 0x29: this.i = 0x50 + (vx & 15) * 5; break;
          case 0x33:
            check(3);
            this.memory[this.i] = Math.floor(vx / 100);
            this.memory[this.i + 1] = Math.floor(vx / 10) % 10;
            this.memory[this.i + 2] = vx % 10;
            break;
          case 0x55: case 0x65:
            check(x + 1);
            if (byte === 0x55) this.memory.set(v.subarray(0, x + 1), this.i);
            else v.set(this.memory.subarray(this.i, this.i + x + 1));
            if (this.classic) this.i += x + 1;
            break;
          default: invalid();
        }
        break;
    }
    return true;
  }
}
