// Reader domain values. Books, progress and preferences belong to this local
// library; no book bytes or reading history are published as page records.
export type Bookmark = { position: number; label: string; at: number };
export type Book = {
  id: string;
  title: string;
  name: string;
  format: "txt" | "pdf";
  size: number;
  added: number;
  opened: number;
  position: number;
  progress: number;
  bookmarks: Bookmark[];
  cover?: string;
  pages?: number;
};
export type StoredBook = Book & { file: Blob };
export type Settings = {
  theme: "auto" | "paper" | "warm" | "sage" | "night";
  font: "serif" | "sans" | "mono";
  size: number;
  line: number;
  width: number;
  effect: "slide" | "fade" | "leaf" | "none";
  bold: boolean;
};
export const defaults: Settings = {
  theme: "auto",
  font: "serif",
  size: 21,
  line: 1.9,
  width: 720,
  effect: "slide",
  bold: false,
};
export const fonts = {
  serif:
    '"Iowan Old Style", "Palatino Linotype", "Songti SC", "Noto Serif CJK SC", "SimSun", serif',
  sans: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
  mono: '"SFMono-Regular", Consolas, "Noto Sans Mono CJK SC", monospace',
};
export function settingsOf(value: Partial<Settings> | null): Settings {
  const s = { ...defaults, ...value };
  const clamp = (v: number, a: number, b: number, d: number) =>
    Number.isFinite(v) ? Math.max(a, Math.min(b, v)) : d;
  return {
    theme: ["auto", "paper", "warm", "sage", "night"].includes(s.theme)
      ? s.theme
      : "auto",
    font: ["serif", "sans", "mono"].includes(s.font) ? s.font : "serif",
    effect: ["slide", "fade", "leaf", "none"].includes(s.effect)
      ? s.effect
      : "slide",
    size: clamp(s.size, 14, 36, 21),
    line: clamp(s.line, 1.4, 2.6, 1.9),
    width: clamp(s.width, 480, 960, 720),
    bold: !!s.bold,
  };
}
export function chapters(text: string): { title: string; position: number }[] {
  const result: { title: string; position: number }[] = [];
  const pattern =
    /^[ \t\u3000]{0,16}(?:第[一二三四五六七八九十百千万零\d]+[章回卷节部篇]|(?:chapter|part|book)[ \t]+|[一二三四五六七八九十]+[、.．]|#{1,3}\s+)[^\n]{0,85}$/gim;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(text)) && result.length < 2000)
    result.push({
      title: match[0].trim().replace(/^#+\s*/, ""),
      position: match.index,
    });
  return result;
}
export function occurrences(
  text: string,
  query: string,
  limit = 100,
): { position: number; excerpt: string }[] {
  const q = query.trim().toLocaleLowerCase();
  if (!q) return [];
  const result: { position: number; excerpt: string }[] = [];
  const pattern = new RegExp(
    query.trim().replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
    "giu",
  );
  let match: RegExpExecArray | null;
  while (result.length < limit && (match = pattern.exec(text))) {
    const position = match.index;
    result.push({
      position,
      excerpt: text
        .slice(Math.max(0, position - 22), position + match[0].length + 62)
        .replace(/\s+/g, " "),
    });
  }
  return result;
}
export function decodeText(bytes: ArrayBuffer): {
  text: string;
  encoding: string;
} {
  const b = new Uint8Array(bytes);
  let encoding =
    b[0] === 0xff && b[1] === 0xfe
      ? "utf-16le"
      : b[0] === 0xfe && b[1] === 0xff
        ? "utf-16be"
        : "utf-8";
  let text: string;
  try {
    text = new TextDecoder(encoding, { fatal: true }).decode(b);
  } catch {
    encoding = "gb18030";
    text = new TextDecoder(encoding, { fatal: true }).decode(b);
  }
  text = text
    .replace(/^\uFEFF/, "")
    .replace(/\r\n?/g, "\n")
    .replace(/\t/g, "    ");
  if (!text.trim() || /\x00/.test(text)) throw new Error("invalid_text");
  return { text, encoding };
}
export const percent = (n: number) =>
  Math.round(Math.max(0, Math.min(1, n || 0)) * 100);
export function bookColor(id: string) {
  let hash = 0;
  for (const c of id) hash = (hash * 31 + c.charCodeAt(0)) | 0;
  return Math.abs(hash) % 6;
}
