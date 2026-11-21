// Offline contract tests using public responses captured in testdata; no live keys or network.
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";
const here = path.dirname(fileURLToPath(import.meta.url)),
  tmp = fs.mkdtempSync(path.join(os.tmpdir(), "myco-stocks-test-"));
for (const name of ["model", "providers"]) {
  const s = fs.readFileSync(
    path.join(here, "src/stocks", name + ".ts"),
    "utf8",
  );
  const js = ts
    .transpileModule(s, {
      compilerOptions: {
        module: ts.ModuleKind.ES2022,
        target: ts.ScriptTarget.ES2022,
      },
    })
    .outputText.replaceAll('"./model"', '"./model.mjs"');
  fs.writeFileSync(path.join(tmp, name + ".mjs"), js);
}
try {
  const m = await import(pathToFileURL(path.join(tmp, "model.mjs")).href),
    p = await import(pathToFileURL(path.join(tmp, "providers.mjs")).href);
  const fixture = (n) =>
    JSON.parse(
      fs.readFileSync(
        path.join(here, "src/stocks/testdata", n + ".json"),
        "utf8",
      ),
    );
  const cn = { market: "CN", symbol: "600519.SH", name: "贵州茅台" },
    hk = { market: "HK", symbol: "00700.HK", name: "腾讯" },
    us = { market: "US", symbol: "AAPL", name: "Apple" };
  assert.equal(m.parseInstrument("sh600519", "CN").symbol, "600519.SH");
  assert.equal(m.parseInstrument("600519.SS", "CN").symbol, "600519.SH");
  assert.equal(m.parseInstrument("0700.HK", "HK").symbol, "00700.HK");
  assert.equal(m.parseInstrument("510300", "CN").symbol, "510300.SH");
  assert.equal(m.parseInstrument("920019", "CN").symbol, "920019.BJ");
  assert.equal(p.yahooSymbol(hk), "0700.HK");
  assert.equal(p.yahooSymbol(cn), "600519.SS");
  assert.equal(p.yahooSymbol({ ...us, symbol: "BRK.B" }), "BRK-B");
  assert.equal(m.parseInstrument("sh600519.SZ", "CN"), null);
  assert.equal(m.parseInstrument("<script>", "US"), null);
  assert.equal(m.parseInstrument("0", "HK"), null);
  for (const [i, name] of [
    [cn, "cn"],
    [hk, "hk"],
    [us, "us"],
  ]) {
    const d = p.parseYahoo(fixture("yahoo-" + name), i, "1d");
    assert(d.bars.length > 10);
    assert.equal(d.quote.currency, m.currency[i.market]);
    assert.equal(d.volumeUnit, "shares");
    assert.equal(d.adjustment, "unadjusted");
    assert(d.quote.price > 0);
    assert.equal(d.quote.previousClose, d.bars[d.bars.length - 2].close);
    assert(
      Math.abs(
        d.quote.changePercent -
          (d.quote.price / d.quote.previousClose - 1) * 100,
      ) < 1e-8,
    );
    const week = p.parseYahoo(fixture("yahoo-" + name), i, "1wk");
    assert(week.bars.length < d.bars.length);
    assert.equal(week.session.close, d.session.close);
  }
  // Localized display dates must not influence prior-session comparisons (older WebKit CLDR).
  const displayFormat = Object.getOwnPropertyDescriptor(
    Intl.DateTimeFormat.prototype,
    "format",
  );
  try {
    Object.defineProperty(Intl.DateTimeFormat.prototype, "format", {
      configurable: true,
      get: () => () => "11/20/2026",
    });
    const d = p.parseYahoo(fixture("yahoo-us"), us, "1d");
    assert.equal(d.quote.previousClose, d.bars[d.bars.length - 2].close);
  } finally {
    Object.defineProperty(
      Intl.DateTimeFormat.prototype,
      "format",
      displayFormat,
    );
  }
  for (const [i, name] of [
    [cn, "cn"],
    [hk, "hk"],
  ]) {
    const raw = fixture("tencent-" + name),
      d = p.parseTencent(raw, i, "1d");
    assert(d.bars.length);
    assert(d.quote.at);
    const r = raw.data[p.tencentSymbol(i)].day[0];
    assert.equal(d.bars[0].open, Number(r[1]));
    assert.equal(d.bars[0].close, Number(r[2]));
    assert.equal(
      d.bars[0].volume,
      Number(r[5]) * (i.market === "CN" ? 100 : 1),
    );
  }
  assert.throws(
    () => p.parseYahoo(fixture("yahoo-cn"), us, "1d"),
    /不同的证券/,
  );
  assert.throws(() => p.parseTencent({}, us, "1d"), /不支持美股/);
  assert(
    !new p.TencentProvider(async () => {}).supports(
      m.parseInstrument("430047.BJ", "CN"),
    ),
  );
  assert.equal(
    m.cleanBars([
      { time: "2026-02-31", open: 1, high: 2, low: 1, close: 2, volume: 1 },
    ]).length,
    0,
  );
  assert.equal(
    m.cleanBars([
      { time: "2026-01-01", open: null, high: 2, low: 1, close: 2, volume: 1 },
    ]).length,
    0,
  );
  assert.throws(() => m.quoteFrom(null, 10, null, "CN"));
  const bars = m.cleanBars([
    { time: "2026-11-02", open: 10, high: 15, low: 9, close: 12, volume: 100 },
    { time: "2026-11-03", open: 12, high: 18, low: 11, close: 17, volume: 200 },
    { time: "2026-11-09", open: 17, high: 20, low: 16, close: 19, volume: 300 },
  ]);
  const weeks = m.aggregate(bars, "1wk");
  assert.deepEqual(weeks[0], {
    time: "2026-11-02",
    open: 10,
    high: 18,
    low: 9,
    close: 17,
    volume: 300,
  });
  assert.equal(m.movingAverage(bars, 2)[0].value, 14.5);
  for (const u of [
    "http://example.com",
    "https://user:secret@example.com",
    "https://example.com?token=secret",
    "javascript:alert(1)",
  ])
    assert.throws(() => p.gatewayURL(u));
  assert.equal(p.gatewayURL("http://localhost:9000/v1").hostname, "localhost");
  const custom = {
    instrument: us,
    interval: "1d",
    bars,
    quote: {
      price: 19,
      previousClose: 17,
      currency: "USD",
      at: "2026-11-09T21:00:00Z",
    },
    source: "Test gateway",
    adjustment: "unadjusted",
    volumeUnit: "shares",
  };
  assert.equal(p.parseGateway(custom, us, "1d").source, "Test gateway");
  assert.throws(() =>
    p.parseGateway({ ...custom, adjustment: "forward" }, us, "1d"),
  );
  assert.throws(() => p.parseGateway(custom, hk, "1d"));
  const map = new Map(),
    cache = {
      get: async (k) => map.get(k) || null,
      set: async (k, d) => map.set(k, d),
    },
    opts = { source: "auto", gateway: "" };
  let calls = 0;
  const c = new p.MarketClient(async (url) => {
    calls++;
    await new Promise((r) => setTimeout(r, 2));
    return fixture("yahoo-cn");
  }, cache);
  await Promise.all([c.load(cn, "1d", opts), c.load(cn, "1d", opts)]);
  assert.equal(calls, 1);
  assert((await c.load(cn, "1d", opts)).cached);
  await c.load(cn, "1d", opts, true);
  assert.equal(calls, 2);
  let attempts = [];
  const fallback = new p.MarketClient(
    async (url) => {
      attempts.push(url);
      if (url.includes("yahoo")) throw Error("HTTP 429");
      return fixture("tencent-cn");
    },
    { get: async () => null, set: async () => {} },
  );
  const f = await fallback.load(cn, "1d", opts);
  assert.equal(f.source, "腾讯行情");
  assert(f.warnings.some((x) => x.includes("切换")));
  assert.equal(attempts.length, 2);
  attempts = [];
  await assert.rejects(fallback.load(us, "1d", opts));
  assert.equal(attempts.length, 1);
  const stale = new p.MarketClient(
    async () => {
      throw Error("offline");
    },
    cache,
    () => Date.now() + 10000000,
  );
  const old = await stale.load(cn, "1d", opts);
  assert(old.stale);
  assert(old.warnings[0].includes("不是最新行情"));
  const different = new p.MarketClient(async () => {
    throw Error("offline");
  }, cache);
  await assert.rejects(
    different.load(cn, "1d", {
      source: "gateway",
      gateway: "https://example.com",
    }),
  );
  let inflight = 0,
    peak = 0;
  const limited = new p.MarketClient(
    async () => {
      inflight++;
      peak = Math.max(peak, inflight);
      await new Promise((r) => setTimeout(r, 5));
      inflight--;
      return fixture("yahoo-us");
    },
    { get: async () => null, set: async () => {} },
  );
  await Promise.all(
    ["1d", "1wk", "1mo"].map((interval) =>
      limited.load(us, interval, { source: "yahoo", gateway: "" }),
    ),
  );
  assert(peak <= 2);
  console.log(
    "PASS provider contracts: real CN/HK/US fixtures, symbol and currency validation, OHLCV, raw-volume conversion, prior-close change, aggregation, gateway, cache/dedupe/limits, fallback, stale state",
  );
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}
