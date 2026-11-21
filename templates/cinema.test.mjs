// Catalog compatibility, safe source addresses and HLS bridge transport behavior.
import assert from "node:assert/strict";
import fs from "node:fs";
import ts from "typescript";
import { inlineI18n } from "./test-i18n.mjs";
const dataURL = (js) =>
  "data:text/javascript;base64," + Buffer.from(js).toString("base64");
const compile = (file) =>
  inlineI18n(
    ts.transpile(fs.readFileSync(new URL(file, import.meta.url), "utf8"), {
      target: 99,
      module: 99,
    }),
  );
const modelURL = dataURL(compile("src/cinema/catalog.ts"));
const m = await import(modelURL);
const source = {
  name: "My source",
  endpoint: "https://catalog.test/api",
  enabled: true,
  order: 1,
};
for (const bad of [
  "javascript:alert(1)",
  "https://user:password@catalog.test/api",
  "https://catalog.test/api?key=secret",
  "https://catalog.test/api#fragment",
  "",
])
  assert.throws(() => m.sourceURL(bad));
assert.equal(
  m.sourceURL("http://127.0.0.1:9000/vod"),
  "http://127.0.0.1:9000/vod",
);
const query = new URL(
  m.requestURL(source, {
    query: "山 & sea",
    id: "id/1",
    category: "film",
    page: 2,
  }),
);
assert.equal(query.searchParams.get("wd"), "山 & sea");
assert.equal(query.searchParams.get("ids"), "id/1");
assert.equal(query.searchParams.get("pg"), "2");
assert.throws(() => m.parseCatalog({ code: 0 }, "s", source.endpoint));
const parsed = m.parseCatalog(
  {
    page: "2",
    pagecount: "3",
    class: [{ type_id: 9, type_name: "剧集" }],
    list: [
      {
        vod_id: 1,
        vod_name: "<b>山间来信</b>",
        vod_content: "<script>alert(1)</script>&lt;b&gt;plain",
        vod_pic: "/cover.jpg",
        vod_play_from: "HD$$$SD",
        vod_play_url:
          "第01集$https://media.test/one.m3u8#第02集$https://media.test/two.m3u8$$$第02集$https://media.test/two-sd.m3u8",
      },
      null,
      {},
      { vod_name: "invalid" },
    ],
  },
  "s",
  source.endpoint,
);
assert.equal(parsed.movies.length, 1);
assert.equal(parsed.movies[0].title, "山间来信");
assert.equal(parsed.movies[0].poster, "https://catalog.test/cover.jpg");
assert.equal(parsed.movies[0].lines.length, 2);
assert.equal(parsed.categories[0].id, "9");
assert.equal(parsed.pages, 3);
assert.equal(
  m.parseLines("bad", "x$javascript:alert(1)", source.endpoint).length,
  0,
);
assert.equal(m.matchEpisode(parsed.movies[0].lines[1].episodes, "第2集"), 0);
assert.equal(
  m.matchEpisode(parsed.movies[0].lines[1].episodes, "第1集"),
  -1,
  "must not fall back to array index",
);
assert.equal(
  m.movieKey({ source: "a:b", movie_id: "c" }) ===
    m.movieKey({ source: "a", movie_id: "b:c" }),
  false,
);
assert.equal(m.clock(3661), "1:01:01");
assert.equal(m.clock(NaN), "0:00");
let fulfill;
const calls = [];
globalThis.window = globalThis;
globalThis.__request = (...args) => {
  calls.push(args);
  return new Promise((r) => (fulfill = r));
};
let js = compile("src/cinema/player.ts")
  .replace(/import Hls[^;]+;/, "const Hls={};")
  .replace(
    /import \{ fromBase64 \}[^;]+;/,
    'const fromBase64=s=>new Uint8Array(Buffer.from(s,"base64"));',
  )
  .replace(/import \{ request \}[^;]+;/, "const request=globalThis.__request;")
  .replace('"./catalog"', JSON.stringify(modelURL));
const { BridgeLoader } = await import(dataURL(js));
const cfg = { timeout: 50, loadPolicy: { maxLoadTimeMs: 50 } };
const loader = new BridgeLoader();
let response;
loader.load(
  {
    url: "https://media.test/segment",
    responseType: "arraybuffer",
    rangeStart: 2,
    rangeEnd: 5,
  },
  cfg,
  {
    onSuccess: (r) => (response = r),
    onError: () => assert.fail(),
    onTimeout: () => assert.fail(),
  },
);
assert.equal(calls[0][2].Range, "bytes=2-4");
fulfill({
  body: Buffer.from([0, 1, 2, 3, 4, 5]).toString("base64"),
  status: 200,
  url: "https://cdn.test/segment",
  headers: {},
});
await new Promise((r) => setTimeout(r, 0));
assert.deepEqual([...new Uint8Array(response.data)], [2, 3, 4]);
assert.equal(response.url, "https://cdn.test/segment");
assert.equal(loader.stats.loaded, 3);
assert.equal(
  loader.stats.aborted,
  false,
  "completed fragments remain usable after loader disposal",
);
loader.destroy();
assert.equal(loader.stats.aborted, false);
loader.load(
  {
    url: "https://media.test/full",
    responseType: "arraybuffer",
    rangeStart: 0,
    rangeEnd: 0,
  },
  cfg,
  {
    onSuccess: (r) => (response = r),
    onError: () => assert.fail(),
    onTimeout: () => assert.fail(),
  },
);
assert.equal(
  calls.at(-1)[2].Range,
  undefined,
  "hls.js uses zero rangeEnd to request a full fragment",
);
fulfill({
  body: Buffer.from([71, 64, 17]).toString("base64"),
  status: 200,
  headers: {},
});
await new Promise((r) => setTimeout(r, 0));
assert.equal(response.data.byteLength, 3);
let notified = false;
loader.load({ url: "https://media.test/old", responseType: "text" }, cfg, {
  onSuccess: () => (notified = true),
  onError: () => assert.fail(),
  onTimeout: () => assert.fail(),
});
loader.abort();
fulfill({ body: "old", status: 200, headers: {} });
await new Promise((r) => setTimeout(r, 0));
assert.equal(notified, false, "late responses after disposal are ignored");
loader.load({ url: "https://media.test/stall", responseType: "text" }, cfg, {
  onSuccess: () => assert.fail(),
  onError: () => assert.fail(),
  onTimeout: () => (notified = true),
});
await new Promise((r) => setTimeout(r, 80));
assert.equal(notified, true);
loader.destroy();
const manifest = JSON.parse(
  fs.readFileSync(new URL("src/cinema/public/sample.json", import.meta.url)),
);
assert.deepEqual(manifest.sources, [], "never ship default sources");
console.log(
  "PASS custom sources, catalog/episode adapters, text safety, stable keys, HLS ranges/redirects/abort/timeout and empty defaults",
);
