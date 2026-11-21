/** Provider-neutral domain. Prices are never converted between currencies. */
export type Market = "CN" | "HK" | "US";
export type Interval = "1d" | "1wk" | "1mo";
export type Instrument = { market: Market; symbol: string; name: string };
export type Candle = {
  time: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
};
export type Quote = {
  price: number;
  previousClose?: number;
  change?: number;
  changePercent?: number;
  at?: string;
  currency: string;
};
export type MarketData = {
  instrument: Instrument;
  interval: Interval;
  bars: Candle[];
  session?: Candle;
  quote: Quote;
  source: string;
  fetchedAt: string;
  adjustment: "unadjusted";
  volumeUnit: "shares";
  warnings: string[];
  stale?: boolean;
  cached?: boolean;
};
export type WatchItem = Instrument & { order: number; note?: string };
export const marketNames: Record<Market, string> = {
  CN: "A 股",
  HK: "港股",
  US: "美股",
};
export const timezone: Record<Market, string> = {
  CN: "Asia/Shanghai",
  HK: "Asia/Hong_Kong",
  US: "America/New_York",
};
export const currency: Record<Market, string> = {
  CN: "CNY",
  HK: "HKD",
  US: "USD",
};
export const keyOf = (i: Instrument) => `${i.market}:${i.symbol}`;
export function parseInstrument(
  raw: string,
  market: Market,
): Instrument | null {
  const s = raw.trim().toUpperCase().replace(/\s+/g, "");
  if (market === "CN") {
    const m = /^(?:(SH|SZ|BJ))?(\d{6})(?:\.(SH|SS|SZ|BJ))?$/.exec(s);
    if (!m) return null;
    const exchange = (
      m[1] ||
      m[3] ||
      (/^(?:4|8|92)/.test(m[2]) ? "BJ" : /^[569]/.test(m[2]) ? "SH" : "SZ")
    ).replace("SS", "SH");
    if (m[1] && m[3] && m[1] !== m[3].replace("SS", "SH")) return null;
    return { market, symbol: `${m[2]}.${exchange}`, name: m[2] };
  }
  if (market === "HK") {
    const m = /^(?:HK)?(\d{1,5})(?:\.HK)?$/.exec(s);
    return m && Number(m[1]) > 0
      ? {
          market,
          symbol: `${m[1].padStart(5, "0")}.HK`,
          name: m[1].padStart(5, "0"),
        }
      : null;
  }
  return /^[A-Z][A-Z0-9.-]{0,14}$/.test(s)
    ? { market, symbol: s, name: s }
    : null;
}
export const catalog: Instrument[] = [
  { market: "CN", symbol: "600519.SH", name: "贵州茅台" },
  { market: "CN", symbol: "000001.SZ", name: "平安银行" },
  { market: "CN", symbol: "300750.SZ", name: "宁德时代" },
  { market: "CN", symbol: "600036.SH", name: "招商银行" },
  { market: "CN", symbol: "601318.SH", name: "中国平安" },
  { market: "CN", symbol: "000858.SZ", name: "五粮液" },
  { market: "CN", symbol: "002594.SZ", name: "比亚迪" },
  { market: "CN", symbol: "510300.SH", name: "沪深300ETF" },
  { market: "HK", symbol: "00700.HK", name: "腾讯控股" },
  { market: "HK", symbol: "09988.HK", name: "阿里巴巴" },
  { market: "HK", symbol: "01810.HK", name: "小米集团" },
  { market: "HK", symbol: "03690.HK", name: "美团" },
  { market: "HK", symbol: "00941.HK", name: "中国移动" },
  { market: "HK", symbol: "01211.HK", name: "比亚迪股份" },
  { market: "US", symbol: "AAPL", name: "Apple 苹果" },
  { market: "US", symbol: "MSFT", name: "Microsoft 微软" },
  { market: "US", symbol: "NVDA", name: "NVIDIA 英伟达" },
  { market: "US", symbol: "GOOGL", name: "Alphabet 谷歌" },
  { market: "US", symbol: "TSLA", name: "Tesla 特斯拉" },
  { market: "US", symbol: "AMZN", name: "Amazon 亚马逊" },
  { market: "US", symbol: "META", name: "Meta" },
  { market: "US", symbol: "SPY", name: "标普500ETF" },
];
export const finite = (x: unknown): number | undefined =>
  x !== null && x !== undefined && x !== "" && Number.isFinite(Number(x))
    ? Number(x)
    : undefined;
/** Bad/partial bars are omitted, never filled with an invented price. */
export function cleanBars(rows: Candle[]): Candle[] {
  const byTime = new Map<string, Candle>();
  for (const r of rows) {
    if (
      !/^\d{4}-\d{2}-\d{2}$/.test(r.time) ||
      !Number.isFinite(Date.parse(r.time)) ||
      new Date(r.time + "T00:00:00Z").toISOString().slice(0, 10) !== r.time
    )
      continue;
    const o = finite(r.open),
      h = finite(r.high),
      l = finite(r.low),
      c = finite(r.close),
      v = finite(r.volume);
    if (
      v === undefined ||
      v < 0 ||
      o === undefined ||
      h === undefined ||
      l === undefined ||
      c === undefined ||
      Math.min(o, h, l, c) <= 0 ||
      h < Math.max(o, c, l) ||
      l > Math.min(o, c, h)
    )
      continue;
    byTime.set(r.time, {
      time: r.time,
      open: o,
      high: h,
      low: l,
      close: c,
      volume: v !== undefined && v >= 0 ? v : 0,
    });
  }
  return [...byTime.values()].sort((a, b) => a.time.localeCompare(b.time));
}
export function aggregate(bars: Candle[], interval: Interval): Candle[] {
  if (interval === "1d") return bars;
  const groups = new Map<string, Candle>();
  for (const b of bars) {
    const d = new Date(b.time + "T12:00:00Z");
    if (interval === "1mo") d.setUTCDate(1);
    else d.setUTCDate(d.getUTCDate() - ((d.getUTCDay() + 6) % 7));
    const key = d.toISOString().slice(0, 10),
      old = groups.get(key);
    if (!old) groups.set(key, { ...b });
    else
      groups.set(key, {
        ...old,
        high: Math.max(old.high, b.high),
        low: Math.min(old.low, b.low),
        close: b.close,
        volume: old.volume + b.volume,
      });
  }
  return [...groups.values()];
}
export function movingAverage(bars: Candle[], n: number) {
  let sum = 0;
  return bars.flatMap((b, i) => {
    sum += b.close;
    if (i >= n) sum -= bars[i - n].close;
    return i < n - 1 ? [] : [{ time: b.time, value: sum / n }];
  });
}
export function quoteFrom(
  price: unknown,
  previous: unknown,
  at: unknown,
  market: Market,
): Quote {
  const p = finite(price);
  if (p === undefined || p <= 0) throw new Error("数据源没有返回有效报价");
  const prev = finite(previous),
    validPrev = prev !== undefined && prev > 0 ? prev : undefined;
  const dt =
    typeof at === "string" && Number.isFinite(Date.parse(at)) ? at : undefined;
  return {
    price: p,
    previousClose: validPrev,
    change: validPrev === undefined ? undefined : p - validPrev,
    changePercent:
      validPrev === undefined ? undefined : ((p - validPrev) / validPrev) * 100,
    at: dt,
    currency: currency[market],
  };
}
export function price(v?: number) {
  return v === undefined
    ? "—"
    : v.toLocaleString("en-US", {
        minimumFractionDigits: 2,
        maximumFractionDigits: 4,
      });
}
export function compact(v?: number) {
  if (v === undefined) return "—";
  if (v >= 1e8) return (v / 1e8).toFixed(2) + " 亿";
  if (v >= 1e4) return (v / 1e4).toFixed(2) + " 万";
  return v.toLocaleString("en-US");
}
export const sign = (v?: number) => (v === undefined ? "" : v > 0 ? "+" : "");
export function csv(data: MarketData) {
  return (
    "\ufeffdate,open,high,low,close,volume_shares,source,adjustment,currency\r\n" +
    data.bars
      .map((b) =>
        [
          b.time,
          b.open,
          b.high,
          b.low,
          b.close,
          b.volume,
          data.source,
          "unadjusted",
          data.quote.currency,
        ]
          .map((x) => '"' + String(x).replace(/"/g, '""') + '"')
          .join(","),
      )
      .join("\r\n")
  );
}
