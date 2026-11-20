# Stock Watch

`myco/stocks` (Tools shelf) is a shared watchlist and observation notebook for mainland China, Hong Kong, and US stocks. It displays unadjusted daily/weekly/monthly OHLCV, MA5/10/20 and closing-price lines using TradingView Lightweight Charts 5.3.0 (Apache-2.0, with attribution and license included).

## Use and storage

Add securities by common names or exchange codes (`600519.SH`, `000001.SZ`, `00700.HK`, `AAPL`). Name matching covers a small built-in list; arbitrary supported codes are validated against the provider. Charts show the last 100 loaded periods initially; scroll/zoom for earlier history, hover for OHLCV. Weekly/monthly bars aggregate daily sessions, so the first/last period can be partial. The last candle can change before close. Price change uses the prior trading close, never the baseline before the full chart range. Currencies are not converted; volume is normalized to shares.

An App with `myco.os.net.fetch` is required: market requests leave from the person's desktop, not the static iframe or the Hub. The watchlist and notes are shared page records. Provider settings and market cache stay in local IndexedDB, with a session-memory fallback if storage is unavailable. No credentials or market prices are stored in shared records. Automatic refresh is off by default; enabled, it runs every 60 seconds while the page is visible. Requests are deduplicated, limited to two simultaneous upstream calls, and cached for 60 seconds. Manual refresh bypasses cache. Closing the page stops polling.

The app displays source, quote time, fetch time, market currency and stale state. Missing data is never replaced with simulated prices. After a failed refresh, prior cache may be shown with an explicit warning that it is not current. CSV/JSON exports retain metadata; PNG exports the visible chart.

## Provider abstraction

`templates/src/stocks/model.ts` defines the vendor-neutral data types. `providers.ts` defines `MarketProvider` (`id`, `label`, `supports(instrument)`, `load(instrument, interval)`). Transport, cache and clock are injected. Vendor symbol/field mapping stays in adapters, outside the UI and shared watchlist schema.

| Adapter | Coverage | Behavior |
| --- | --- | --- |
| Yahoo Finance | Mainland China, Hong Kong, US | Public chart endpoint, no key; up to five years of unadjusted daily bars and quote timestamp. Symbol and currency are checked. |
| Tencent | Shanghai/Shenzhen and Hong Kong | Public historical endpoint, no key; mainland volume is converted from lots to shares. US and Beijing history are excluded: live probes found incomplete/unsupported history. |
| Custom gateway | As supplied by the gateway | Supports AKShare, Tushare, a broker or another vendor through a normalized response; credentials stay in the gateway service. |

The public interfaces are unofficial and can change or rate-limit; no exchange-grade real-time guarantee is made. Observe their usage and redistribution terms. Auto tries Yahoo, then Tencent only for supported markets. An entire response is replaced on failover; data from different providers is never spliced into one series. Explicit provider selection never silently changes vendors. Cache keys include provider/gateway/security/interval.

### Custom gateway

Configure a base such as `https://quotes.example.com/v1`; the adapter sends:

```
GET /v1/bars?market=US&symbol=AAPL&interval=1d&limit=1500
```

Return JSON (prices here are illustrative):

```json
{
  "instrument": { "market": "US", "symbol": "AAPL", "name": "Apple" },
  "interval": "1d",
  "bars": [{ "time": "2026-11-19", "open": 100, "high": 104, "low": 98, "close": 102, "volume": 100000 }],
  "quote": { "price": 102, "previousClose": 100, "currency": "USD", "at": "2026-11-19T21:00:00Z" },
  "source": "Your vendor",
  "adjustment": "unadjusted",
  "volumeUnit": "shares"
}
```

`market` is CN/HK/US, `symbol` is the canonical code, `interval` is 1d/1wk/1mo. Dates are exchange-local session dates, ascending and unique. Gateway weekly/monthly bars are already aggregated. An optional raw daily Candle in `session` supplies latest daily open/high/low in the summary; without it those fields remain blank. Bars need finite positive OHLC, consistent bounds and nonnegative volume. Invalid bars are omitted; identity, currency, adjustment and unit mismatches are rejected. Unsupported intervals should return an HTTP error, not another period. HTTPS is required except loopback HTTP. Base URLs cannot contain credentials, query parameters or fragments; manage vendor authentication server-side.

## Build and verification

```
cd templates
npm ci
npm run typecheck
node stocks.test.mjs
node build.mjs stocks
node stocks.ui-test.mjs
node build.mjs stocks
```

The browser test needs Playwright/Chromium (`UI_SMOKE_EXECUTABLE` may select an existing browser). Offline contract tests use public responses captured on 2026-11-21. They cover CN/HK/US normalization, symbol/currency, OHLCV order/units, prior-close percentage, calendar aggregation, gateway validation, cache/deduplication/concurrency, failover and stale states. Browser tests run light/dark at 1200/760, covering markets, periods, modes, MA, chart canvases, notes, exports, unsupported providers, unsafe gateway input, stale cache, watchlist changes and reloads. Screenshots go to `templates/stocks-shots`; catalog thumbnails go to `src/stocks/public/shots`. Shipped samples contain only names/codes, not prices or fabricated candles. The app adds no Hub protocol or database changes.
