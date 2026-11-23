// Test the directory and device library without making network requests.
import assert from "node:assert/strict";
import fs from "node:fs";
import ts from "typescript";
function compile(file, a, b) {
  let source = fs.readFileSync(new URL(file, import.meta.url), "utf8");
  if (a) source = source.replaceAll(a, b);
  return (
    "data:text/javascript;base64," +
    Buffer.from(
      ts.transpileModule(source, {
        compilerOptions: {
          target: ts.ScriptTarget.ES2022,
          module: ts.ModuleKind.ES2022,
        },
      }).outputText,
    ).toString("base64")
  );
}
const address = compile(
  "./src/podcast/directory.ts",
  'import { network } from "../_lib/net";',
  'const network=()=>{throw new Error("Unexpected network");};',
);
const d = await import(address),
  l = await import(
    compile("./src/podcast/local.ts", '"./directory"', JSON.stringify(address))
  );
for (const url of [
  "javascript:alert(1)",
  "file:///etc/passwd",
  "https://user:secret@example.org/rss",
  "https://example.org/\nfeed",
])
  assert.equal(d.publicURL(url), "");
const shows = d.parseDirectory({
  results: [
    {
      collectionName: "Title",
      feedUrl: "https://example.org/rss",
      artworkUrl600: "data:bad",
    },
    { collectionName: "Duplicate", feedUrl: "https://example.org/rss" },
    { collectionName: "Bad", feedUrl: "file:///etc/passwd" },
  ],
});
assert.equal(shows.length, 1);
assert.equal(shows[0].artworkUrl, "");
assert.throws(() => d.parseDirectory({ results: "wrong" }));
assert.notEqual(l.listeningKey("p1", "a"), l.listeningKey("p1", "b"));
assert.notEqual(l.listeningKey("p1", "a"), l.listeningKey("p2", "a"));
assert.deepEqual(
  l.readListening({ getItem: () => "{broken" }, "key"),
  l.emptyListening(),
);
assert.equal(l.saveListening(null, "key", l.emptyListening()), false);
assert.equal(
  l.saveListening(
    {
      setItem() {
        throw Error("quota");
      },
    },
    "key",
    l.emptyListening(),
  ),
  false,
);
let state = l.emptyListening();
const ep = {
  guid: "1",
  title: "Episode",
  audioUrl: "https://example.org/audio.mp3",
};
for (let i = 0; i < 120; i++)
  state = l.rememberProgress(state, { ...ep, guid: String(i) }, i, i);
assert.equal(Object.keys(state.progress).length, 100);
assert.equal(l.rememberProgress(state, ep, NaN), state);
assert.equal(l.rememberProgress(state, ep, -1), state);
assert.deepEqual(
  l.readListening(
    {
      getItem: () =>
        JSON.stringify({
          follows: [{ title: "Bad", feedUrl: "javascript:1" }],
          likes: [{ ...ep, audioUrl: "file:///audio" }],
          progress: { bad: { seconds: -1, updatedAt: 0 } },
        }),
    },
    "key",
  ),
  l.emptyListening(),
);
console.log(
  "PASS URL hygiene, directory deduplication, scoped/bounded listening state and storage failures",
);
