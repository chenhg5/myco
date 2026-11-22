import assert from "node:assert/strict";
import fs from "node:fs";
import ts from "typescript";
const source = fs.readFileSync(
  new URL("./src/reader/model.ts", import.meta.url),
  "utf8",
);
const output = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2020,
    module: ts.ModuleKind.ESNext,
  },
}).outputText;
const { decodeText, chapters, occurrences, settingsOf, percent } = await import(
  "data:text/javascript;base64," + Buffer.from(output).toString("base64")
);
const input = (s) => Uint8Array.from(Buffer.from(s)).buffer;
assert.deepEqual(decodeText(input("\uFEFF你好\r\nreader\rbook")), {
  text: "你好\nreader\nbook",
  encoding: "utf-8",
});
assert.equal(
  decodeText(Uint8Array.from([0xff, 0xfe, 0x60, 0x4f, 0x7d, 0x59]).buffer).text,
  "你好",
);
assert.equal(
  decodeText(Uint8Array.from([0xfe, 0xff, 0x4f, 0x60, 0x59, 0x7d]).buffer).text,
  "你好",
);
assert.deepEqual(decodeText(Uint8Array.from([0xc4, 0xe3, 0xba, 0xc3]).buffer), {
  text: "你好",
  encoding: "gb18030",
});
for (const value of ["", "   ", "\x00a"])
  assert.throws(() => decodeText(input(value)));
const book =
  "Title\n\n第一章 起点\n内容\nChapter 2 A new day\nText\n第三章 终点";
assert.deepEqual(
  chapters(book).map((c) => c.title),
  ["第一章 起点", "Chapter 2 A new day", "第三章 终点"],
);
for (const c of chapters(book))
  assert.equal(book.slice(c.position, c.position + c.title.length), c.title);
assert.deepEqual(
  occurrences("abc [test] ABC", "[test]").map((r) => r.position),
  [4],
);
assert.deepEqual(
  occurrences("A😀 hello a😀 HELLO", "hello").map((r) => r.position),
  [4, 14],
);
assert.equal(
  occurrences("İ case", "case")[0].position,
  2,
  "Unicode case folding must not shift source offsets",
);
assert.equal(occurrences("a".repeat(1000), "a").length, 100);
assert.equal(occurrences("text", "  ").length, 0);
const settings = settingsOf({
  size: 100,
  line: NaN,
  width: 5,
  font: "bad",
  effect: "bad",
  theme: "bad",
});
assert.equal(settings.size, 36);
assert.equal(settings.line, 1.9);
assert.equal(settings.width, 480);
assert.equal(settings.font, "serif");
assert.equal(settings.effect, "slide");
assert.equal(settings.theme, "auto");
assert.equal(percent(Infinity), 100);
assert.equal(percent(-3), 0);
console.log(
  "reader: BOM/GB18030 decoding, newline normalization, invalid text, chapter offsets, literal/Unicode bounded search and preference migration passed",
);

assert.deepEqual(
  chapters("　　第一章 起点\n内容\nChapter One\n内容").map((c) => c.title),
  ["第一章 起点", "Chapter One"],
);
