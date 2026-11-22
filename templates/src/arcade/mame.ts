// Construct native MAME commands from files selected through the OS picker.
// MAME resolves ROM sets by driver name and looks for parent/BIOS ZIPs locally.
export function mameROM(path: string) {
  const separator = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  const filename = path.slice(separator + 1);
  const shortName = /^([a-z0-9_]+)\.zip$/.exec(filename)?.[1];
  if (separator < 0 || !shortName || /[;\x00-\x1f]/.test(path)) return;
  const rootLength = /^[a-z]:[/\\]/i.test(path) ? 3 : 1;
  return { filename, shortName, directory: path.slice(0, Math.max(separator, rootLength)) };
}

export function mameArgs(path: string, windowsKeys = false) {
  const rom = mameROM(path);
  if (!rom) return;
  const base = [rom.shortName, "-rompath", rom.directory];
  return {
    filename: rom.filename,
    verify: [...base, "-verifyroms"],
    play: [...base, "-window", "-skip_gameinfo", ...(windowsKeys ? ["-keyboardprovider", "win32"] : [])],
  };
}

export function mameVersion(stdout: string) {
  return stdout.split(/\r?\n/).slice(0, 4)
    .map(line => /\bMAME(?:\s+version)?\s+v?(0\.\d{2,4})\b/i.exec(line)?.[1])
    .find(Boolean);
}
