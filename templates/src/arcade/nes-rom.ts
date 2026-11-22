// Validate the cartridge container before allocating memory in the emulator.
export const MAX_NES_ROM = 16 * 1024 * 1024;
export function validNESROM(bytes: Uint8Array) {
  if (bytes.length < 16 || bytes.length > MAX_NES_ROM ||
      bytes[0] !== 0x4e || bytes[1] !== 0x45 || bytes[2] !== 0x53 || bytes[3] !== 0x1a) return false;
  const nes2 = (bytes[7] & 0x0c) === 0x08;
  function size(low: number, high: number, unit: number) {
    return high === 15 ? 2 ** (low >> 2) * ((low & 3) * 2 + 1) : (low + (high << 8)) * unit;
  }
  const prg = size(bytes[4], nes2 ? bytes[9] & 15 : 0, 16384);
  const chr = size(bytes[5], nes2 ? bytes[9] >> 4 : 0, 8192);
  return prg > 0 && bytes.length >= 16 + (bytes[6] & 4 ? 512 : 0) + prg + chr;
}
