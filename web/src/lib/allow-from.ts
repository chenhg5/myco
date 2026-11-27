export interface AllowFromEdit { allow_from?: string[]; allow_add?: string[]; allow_remove?: string[] }
export interface AllowFromChange { added: string[]; removed: string[]; unchanged: string[] }

const placeholder = (n: string) => /^nobody-[a-z0-9]{8}$/.test(n);

export function removedSenders(before: string[], after: string[]): string[] {
  if ((!before.length || before.includes("*")) && !after.includes("*")) return ["*"];
  return before.filter((n) => !placeholder(n) && !after.includes(n));
}

/** Diff the editor's original snapshot so concurrent additions survive. The
 * legacy unrestricted list needs an explicit replacement to become restricted. */
export function senderEdit(before: string[], after: string[], nobody: string): AllowFromEdit {
  const next = after.length ? after : [nobody];
  if (!before.length || before.includes("*")) return { allow_from: next };
  return {
    allow_add: next.filter((n) => !before.includes(n)),
    allow_remove: before.filter((n) => !next.includes(n)),
  };
}
