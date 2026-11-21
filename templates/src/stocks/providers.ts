import {
  aggregate,
  cleanBars,
  currency,
  finite,
  keyOf,
  quoteFrom,
  timezone,
  type Candle,
  type Instrument,
  type Interval,
  type MarketData,
} from "./model";
export type Transport = (url: string) => Promise<unknown>;
export type ProviderID = "auto" | "yahoo" | "tencent" | "gateway";
export interface MarketProvider {
  id: string;
  label: string;
  supports(i: Instrument): boolean;
  load(i: Instrument, interval: Interval): Promise<MarketData>;
}
export type ProviderOptions = { source: ProviderID; gateway: string };
const obj = (v: unknown): Record<string, any> =>
  v && typeof v === "object" && !Array.isArray(v)
    ? (v as Record<string, any>)
    : {};
const arr = (v: unknown): any[] => (Array.isArray(v) ? v : []);
function result(
  i: Instrument,
  interval: Interval,
  bars: Candle[],
  price: unknown,
  prev: unknown,
  at: unknown,
  source: string,
  warnings: string[] = [],
): MarketData {
  const clean = cleanBars(bars);
  if (!clean.length)
    throw new Error("没有可用的 K 线数据，请核对代码或切换数据源");
  return {
    instrument: i,
    interval,
    bars: aggregate(clean, interval),
    session: clean[clean.length - 1],
    quote: quoteFrom(price, prev, at, i.market),
    source,
    adjustment: "unadjusted",
    volumeUnit: "shares",
    fetchedAt: new Date().toISOString(),
    warnings,
  };
}
export function yahooSymbol(i: Instrument) {
  if (i.market === "HK")
    return String(Number(i.symbol.split(".")[0])).padStart(4, "0") + ".HK";
  if (i.market === "CN") return i.symbol.replace(/\.SH$/, ".SS");
  return i.symbol.replace(/\./g, "-");
}
export function parseYahoo(
  raw: unknown,
  i: Instrument,
  interval: Interval,
): MarketData {
  const root = obj(obj(raw).chart);
  if (root.error) throw new Error("Yahoo Finance 暂时没有这只股票的数据");
  const r = obj(arr(root.result)[0]),
    meta = obj(r.meta),
    q = obj(arr(obj(r.indicators).quote)[0]);
  if (
    !meta.symbol ||
    String(meta.symbol).toUpperCase() !== yahooSymbol(i).toUpperCase()
  )
    throw new Error("数据源返回了不同的证券代码");
  if (meta.currency && meta.currency !== currency[i.market])
    throw new Error("数据源币种与所选市场不一致");
  // Build ISO session dates from parts: locale display order varies across WebKit/Chromium versions.
  const formatter = new Intl.DateTimeFormat("en-CA", {
    timeZone: meta.exchangeTimezoneName || timezone[i.market],
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
  const sessionDate = (epoch: number) => {
    const parts = formatter.formatToParts(new Date(epoch * 1000));
    const part = (name: string) =>
      parts.find((p) => p.type === name)?.value || "";
    return `${part("year")}-${part("month")}-${part("day")}`;
  };
  const dates = arr(r.timestamp),
    bars = dates.map((t, k) => {
      let time = "";
      try {
        time = sessionDate(Number(t));
      } catch {}
      return {
        time,
        open: arr(q.open)[k],
        high: arr(q.high)[k],
        low: arr(q.low)[k],
        close: arr(q.close)[k],
        volume: arr(q.volume)[k],
      };
    });
  const clean = cleanBars(bars);
  const at = finite(meta.regularMarketTime);
  const price = finite(meta.regularMarketPrice) ?? clean.at(-1)?.close;
  // previousClose/chartPreviousClose can be the close BEFORE the entire chart range.
  // Derive the prior session from daily bars for a current quote; never use the range baseline.
  const quoteDate = at ? sessionDate(at) : undefined;
  const previous = quoteDate
    ? clean.filter((b) => b.time < quoteDate).at(-1)?.close
    : clean.at(-2)?.close;
  return result(
    i,
    interval,
    clean,
    price,
    previous,
    at ? new Date(at * 1000).toISOString() : undefined,
    "Yahoo Finance",
    ["公开接口，延迟以源站为准；不复权日线，周/月线由日线聚合"],
  );
}
export class YahooProvider implements MarketProvider {
  id = "yahoo";
  label = "Yahoo Finance";
  constructor(private net: Transport) {}
  supports() {
    return true;
  }
  async load(i: Instrument, interval: Interval) {
    const url = `https://query1.finance.yahoo.com/v8/finance/chart/${encodeURIComponent(yahooSymbol(i))}?interval=1d&range=5y`;
    return parseYahoo(await this.net(url), i, interval);
  }
}
export function tencentSymbol(i: Instrument) {
  if (i.market === "HK") return "hk" + i.symbol.split(".")[0];
  const [code, exchange] = i.symbol.split(".");
  return exchange.toLowerCase() + code;
}
export function parseTencent(
  raw: unknown,
  i: Instrument,
  interval: Interval,
): MarketData {
  if (i.market === "US")
    throw new Error("腾讯历史行情适配器不支持美股，请选择 Yahoo 或自定义网关");
  const root = obj(raw);
  if (Number(root.code) !== 0) throw new Error("腾讯行情暂时不可用");
  const r = obj(obj(root.data)[tencentSymbol(i)]),
    q = arr(obj(r.qt)[tencentSymbol(i)]);
  const bars = arr(r.day).map((row) => {
    const a = arr(row);
    return {
      time: String(a[0] || ""),
      open: a[1],
      close: a[2],
      high: a[3],
      low: a[4],
      volume: (finite(a[5]) ?? 0) * (i.market === "CN" ? 100 : 1),
    };
  });
  let at: string | undefined;
  const rawAt = String(q[30] || "").replace(/\D/g, "");
  if (/^\d{14}$/.test(rawAt))
    at = `${rawAt.slice(0, 4)}-${rawAt.slice(4, 6)}-${rawAt.slice(6, 8)}T${rawAt.slice(8, 10)}:${rawAt.slice(10, 12)}:${rawAt.slice(12, 14)}+08:00`;
  return result(i, interval, bars, q[3], q[4], at, "腾讯行情", [
    "公开接口，延迟以源站为准；不复权；A 股成交量由手换算为股",
  ]);
}
export class TencentProvider implements MarketProvider {
  id = "tencent";
  label = "腾讯行情";
  constructor(private net: Transport) {}
  supports(i: Instrument) {
    return (
      i.market === "HK" || (i.market === "CN" && !i.symbol.endsWith(".BJ"))
    );
  }
  async load(i: Instrument, interval: Interval) {
    if (!this.supports(i))
      throw new Error("腾讯历史行情不支持美股，请切换数据源");
    return parseTencent(
      await this.net(
        `https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=${encodeURIComponent(tencentSymbol(i) + ",day,,,1250,")}`,
      ),
      i,
      interval,
    );
  }
}
export function gatewayURL(base: string): URL {
  let u: URL;
  try {
    u = new URL(base);
  } catch {
    throw new Error("请填写有效的行情网关地址");
  }
  if (
    (u.protocol !== "https:" &&
      !(
        u.protocol === "http:" &&
        ["localhost", "127.0.0.1", "[::1]"].includes(u.hostname)
      )) ||
    u.username ||
    u.password ||
    u.search ||
    u.hash
  )
    throw new Error(
      "网关需使用 HTTPS（本机可用 HTTP），地址不带凭证、查询参数或片段",
    );
  return u;
}
export function parseGateway(
  raw: unknown,
  i: Instrument,
  interval: Interval,
): MarketData {
  const r = obj(raw),
    ins = obj(r.instrument),
    q = obj(r.quote);
  if (
    ins.market !== i.market ||
    ins.symbol !== i.symbol ||
    r.interval !== interval
  )
    throw new Error("网关返回的证券或周期不匹配");
  if (
    r.adjustment !== "unadjusted" ||
    r.volumeUnit !== "shares" ||
    q.currency !== currency[i.market]
  )
    throw new Error("网关需返回不复权价格、股数成交量和正确币种");
  const bars = cleanBars(arr(r.bars));
  if (!bars.length) throw new Error("网关没有返回有效 K 线");
  const source =
    typeof r.source === "string" ? r.source.slice(0, 80) : "自定义网关";
  return {
    instrument: i,
    interval,
    bars,
    session: r.session
      ? cleanBars([r.session])[0]
      : interval === "1d"
        ? bars[bars.length - 1]
        : undefined,
    quote: quoteFrom(q.price, q.previousClose, q.at, i.market),
    source,
    fetchedAt: new Date().toISOString(),
    adjustment: "unadjusted",
    volumeUnit: "shares",
    warnings: ["由你的行情网关提供，延迟与授权以该服务为准"],
  };
}
export class GatewayProvider implements MarketProvider {
  id = "gateway";
  label = "自定义网关";
  constructor(
    private net: Transport,
    private base: string,
  ) {}
  supports() {
    return true;
  }
  async load(i: Instrument, interval: Interval) {
    const u = gatewayURL(this.base);
    u.pathname = u.pathname.replace(/\/$/, "") + "/bars";
    u.search = new URLSearchParams({
      market: i.market,
      symbol: i.symbol,
      interval,
      limit: "1500",
    }).toString();
    return parseGateway(await this.net(u.toString()), i, interval);
  }
}
export type CacheStore = {
  get(key: string): Promise<MarketData | null>;
  set(key: string, value: MarketData): Promise<void>;
};
/** Entire responses are replaced on failover. No bars or currencies are spliced. */
export class MarketClient {
  private pending = new Map<string, Promise<MarketData>>();
  private active = 0;
  private waiters: (() => void)[] = [];
  constructor(
    private net: Transport,
    private cache: CacheStore,
    private clock = () => Date.now(),
  ) {}
  private async limited<T>(f: () => Promise<T>) {
    if (this.active >= 2) await new Promise<void>((r) => this.waiters.push(r));
    this.active++;
    try {
      return await f();
    } finally {
      this.active--;
      this.waiters.shift()?.();
    }
  }
  async load(
    i: Instrument,
    interval: Interval,
    o: ProviderOptions,
    force = false,
  ): Promise<MarketData> {
    const k = `v1:${o.source}:${o.gateway}:${keyOf(i)}:${interval}`;
    const running = this.pending.get(k);
    if (running) return running;
    const job = (async () => {
      const old = await this.cache.get(k).catch(() => null);
      if (!force && old && this.clock() - Date.parse(old.fetchedAt) < 60000)
        return { ...old, cached: true };
      const list: MarketProvider[] =
        o.source === "auto"
          ? [new YahooProvider(this.net), new TencentProvider(this.net)]
          : o.source === "yahoo"
            ? [new YahooProvider(this.net)]
            : o.source === "tencent"
              ? [new TencentProvider(this.net)]
              : [new GatewayProvider(this.net, o.gateway)];
      let failed = 0;
      let error: Error | undefined;
      for (const p of list) {
        if (!p.supports(i)) continue;
        try {
          const data = await this.limited(() => p.load(i, interval));
          if (failed)
            data.warnings.unshift("首选数据源暂不可用，已切换至 " + p.label);
          await this.cache.set(k, data).catch(() => {});
          return data;
        } catch (e) {
          error = e instanceof Error ? e : new Error("行情请求失败");
          failed++;
        }
      }
      if (old)
        return {
          ...old,
          cached: true,
          stale: true,
          warnings: ["刷新失败，显示上次缓存；不是最新行情", ...old.warnings],
        };
      throw error || new Error("这个数据源不支持所选市场，请切换数据源");
    })();
    this.pending.set(k, job);
    try {
      return await job;
    } finally {
      this.pending.delete(k);
    }
  }
}
