import { themeMedia } from "../_lib/appearance";
import { t as uiText, locale } from "../_lib/i18n";
import "../_lib/sheet.css";
import "./stocks.css";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  ArrowDown,
  ArrowUpRight,
  ChartCandlestick,
  Check,
  ChevronDown,
  Download,
  Globe2,
  LineChart,
  LoaderCircle,
  Maximize2,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  ShieldCheck,
  Star,
  X,
} from "lucide-react";
import { Store, save, myco } from "../_lib/myco";
import { Kit, Toast, mount, useKit } from "../_lib/appkit";
import { Popover, rectOf } from "../_lib/ui";
import {
  cache,
  defaults,
  loadSettings,
  saveSettings,
  transport,
  type LocalSettings,
} from "./local";
import { MarketClient, gatewayURL, type ProviderID } from "./providers";
import {
  catalog,
  csv,
  keyOf,
  marketNames,
  parseInstrument,
  price,
  sign,
  timezone,
  type Instrument,
  type Interval,
  type Market,
  type MarketData,
  type WatchItem,
} from "./model";
import { PriceChart } from "./Chart";
const watchlist = new Store<WatchItem & Record<string, unknown>>("watchlist"),
  kit = new Kit([watchlist]);
const client = new MarketClient(transport, cache);
const sources: Record<ProviderID, string> = {
  auto: uiText("自动选择"),
  yahoo: "Yahoo Finance",
  tencent: uiText("腾讯行情"),
  gateway: uiText("自定义网关"),
};
const labelTime = (s?: string, market: Market = "CN") => {
  if (!s || !Number.isFinite(Date.parse(s))) return "时间未提供";
  return new Date(s).toLocaleString(locale(), {
    timeZone: timezone[market],
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
};
function App() {
  useKit(kit);
  const [settings, setSettings] = useState<LocalSettings>(defaults),
    [loaded, setLoaded] = useState(false),
    [active, setActive] = useState("");
  const [filter, setFilter] = useState<Market | "all">("all"),
    [query, setQuery] = useState(""),
    [interval, setInterval] = useState<Interval>("1d"),
    [mode, setMode] = useState<"candles" | "line">("candles"),
    [ma, setMA] = useState(true);
  const [rawData, setChartData] = useState<MarketData | null>(null),
    [quotes, setQuotes] = useState<Record<string, MarketData>>({}),
    [quoteErrors, setQuoteErrors] = useState<Record<string, string>>({}),
    [loading, setLoading] = useState(false),
    [error, setError] = useState(""),
    [refresh, setRefresh] = useState(0),
    [force, setForce] = useState(false);
  const [add, setAdd] = useState(false),
    [addMarket, setAddMarket] = useState<Market>("CN"),
    [addQuery, setAddQuery] = useState(""),
    [adding, setAdding] = useState(false),
    [newError, setNewError] = useState("");
  const [showSettings, setShowSettings] = useState(false),
    [draft, setDraft] = useState(settings),
    [settingsError, setSettingsError] = useState("");
  const [menu, setMenu] = useState<ReturnType<typeof rectOf> | null>(null),
    [expanded, setExpanded] = useState(false),
    [systemDark, setSystemDark] = useState(
      themeMedia().matches,
    );
  const capture = useRef<(() => Promise<Blob>) | null>(null),
    optionsKey = JSON.stringify(settings);
  const record =
      watchlist.rows.find((r) => keyOf(r.data) === active) || watchlist.rows[0],
    instrument = record?.data;
  const rows = watchlist.rows.filter(
    (r) =>
      (filter === "all" || r.data.market === filter) &&
      `${r.data.symbol} ${r.data.name}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  const dark =
      myco?.features?.includes("preferences") ? systemDark : settings.theme === "dark" || (settings.theme === "system" && systemDark),
    up = settings.redUp ? "#ce5853" : "#38886c",
    down = settings.redUp ? "#38886c" : "#ce5853";
  const instrumentKey = instrument ? keyOf(instrument) : "";
  const chartData =
    rawData &&
    keyOf(rawData.instrument) === instrumentKey &&
    rawData.interval === interval
      ? rawData
      : null;
  const setCapture = useCallback((f: (() => Promise<Blob>) | null) => {
    capture.current = f;
  }, []);
  useEffect(() => {
    let live = true;
    loadSettings().then((s) => {
      if (live) {
        setSettings(s);
        setLoaded(true);
      }
    });
    return () => {
      live = false;
    };
  }, []);
  useEffect(() => {
    const q = themeMedia(),
      f = () => setSystemDark(q.matches);
    q.addEventListener("change", f);
    return () => q.removeEventListener("change", f);
  }, []);
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
  }, [dark]);
  useEffect(() => {
    if (!loaded || !instrument) {
      setChartData(null);
      return;
    }
    let off = false;
    setLoading(true);
    setError("");
    setChartData(null);
    client
      .load(instrument, interval, settings, force)
      .then(
        (data) => {
          if (off) return;
          setChartData(data);
          if (interval === "1d")
            setQuotes((q) => ({ ...q, [instrumentKey]: data }));
        },
        (e) => {
          if (!off) setError((e as Error).message || uiText("行情加载失败"));
        },
      )
      .finally(() => {
        if (!off) setLoading(false);
      });
    return () => {
      off = true;
    };
  }, [instrumentKey, interval, optionsKey, loaded, refresh]);
  useEffect(() => {
    setQuotes({});
    setQuoteErrors({});
  }, [optionsKey]);
  const watchKeys = watchlist.rows.map((r) => keyOf(r.data)).join("|");
  useEffect(() => {
    if (!loaded) return;
    let off = false;
    setQuoteErrors({});
    for (const r of watchlist.rows) {
      const i = r.data,
        key = keyOf(i);
      client.load(i, "1d", settings, force).then(
        (d) => {
          if (!off) setQuotes((q) => ({ ...q, [key]: d }));
        },
        (e) => {
          if (!off)
            setQuoteErrors((es) => ({
              ...es,
              [key]: (e as Error).message || "暂不可用",
            }));
        },
      );
    }
    return () => {
      off = true;
    };
  }, [watchKeys, optionsKey, loaded, refresh]);
  useEffect(() => {
    if (!settings.autoRefresh) return;
    const t = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        setForce(true);
        setRefresh((x) => x + 1);
      }
    }, 60000);
    return () => clearInterval(t);
  }, [settings.autoRefresh]);
  const doRefresh = () => {
    setForce(true);
    setRefresh((x) => x + 1);
  };
  const addInstrument = async (i: Instrument) => {
    if (watchlist.rows.some((r) => keyOf(r.data) === keyOf(i))) {
      setActive(keyOf(i));
      setAdd(false);
      kit.flash(uiText("已在自选中"));
      return;
    }
    setAdding(true);
    setNewError("");
    try {
      const d = await client.load(i, "1d", settings);
      const id = "s_" + Math.random().toString(36).slice(2, 10),
        r = await kit.track(watchlist.put({ ...i, order: Date.now() }, id));
      if (r) {
        setActive(keyOf(i));
        setAdd(false);
        setQuotes((q) => ({ ...q, [keyOf(i)]: d }));
      }
    } catch (e) {
      setNewError((e as Error).message || "无法确认这个代码，请核对后重试");
    } finally {
      setAdding(false);
    }
  };
  const candidates = catalog.filter(
    (i) =>
      i.market === addMarket &&
      (!addQuery ||
        `${i.name} ${i.symbol}`.toLowerCase().includes(addQuery.toLowerCase())),
  );
  const parsed = parseInstrument(addQuery, addMarket),
    customCandidate =
      parsed && !candidates.some((i) => i.symbol === parsed.symbol)
        ? parsed
        : null;
  const remove = () => {
    if (!record) return;
    const keep = { id: record.id, data: { ...record.data } };
    kit.track(watchlist.remove(record.id));
    kit.flash(uiText("已移出自选"), () => {
      kit.track(watchlist.put(keep.data, keep.id));
    });
    setActive("");
  };
  const exportData = async (kind: "csv" | "json" | "png") => {
    setMenu(null);
    if (!chartData || !instrument) return;
    try {
      const name = `${instrument.symbol}-${interval}-${chartData.stale ? "cached-" : ""}${new Date().toISOString().slice(0, 10)}`;
      if (kind === "csv")
        await save(name + ".csv", csv(chartData), "text/csv;charset=utf-8");
      else if (kind === "json")
        await save(
          name + ".json",
          JSON.stringify(chartData, null, 2),
          "application/json",
        );
      else {
        if (!capture.current) throw new Error(uiText("图表还没准备好"));
        await save(name + ".png", await capture.current(), "image/png");
      }
      kit.flash(uiText("已保存到下载文件夹"));
    } catch (e) {
      kit.flash((e as Error).message || uiText("导出失败，请重试"));
    }
  };
  const openSettings = () => {
    setDraft({ ...settings });
    setSettingsError("");
    setShowSettings(true);
  };
  const applySettings = () => {
    try {
      if (draft.source === "gateway") gatewayURL(draft.gateway);
      setSettings(draft);
      void saveSettings(draft);
      setForce(false);
      setShowSettings(false);
      setChartData(null);
      kit.flash(uiText("数据源设置已保存到本机"));
    } catch (e) {
      setSettingsError((e as Error).message);
    }
  };
  const selectedQuote = chartData?.quote || quotes[instrumentKey]?.quote;
  const changeClass = (n?: number) =>
    n === undefined || n === 0 ? "" : n > 0 ? "is-up" : "is-down";
  const refreshDisabled = loading || !loaded || !instrument;
  return (
    <div
      className={`sw-app ${expanded ? "is-expanded" : ""}`}
      style={{ "--sw-up": up, "--sw-down": down } as React.CSSProperties}
    >
      <header className="sw-header">
        <div className="sw-brand">
          <span>
            <ChartCandlestick />
          </span>
          <strong>{uiText("行情工作台")}</strong>
          <small>/ myco</small>
        </div>
        <div className="sw-market-tags">
          <span>{uiText("A 股")}</span>
          <i />
          <span>{uiText("港股")}</span>
          <i />
          <span>{uiText("美股")}</span>
        </div>
        <div className="sw-header-right">
          <button className="sw-source" onClick={openSettings}>
            <Globe2 />
            {sources[settings.source]}
            <ChevronDown />
          </button>
          <button
            className="sw-icon"
            title={uiText("设置")}
            aria-label={uiText("设置")}
            onClick={openSettings}
          >
            <Settings2 />
          </button>
          <button
            className="sw-button primary"
            onClick={() => {
              setAdd(true);
              setAddQuery("");
              setNewError("");
            }}
          >
            <Plus />
            {uiText("添加自选")}{" "}</button>
        </div>
      </header>
      <div className="sw-body">
        <aside className="sw-sidebar">
          <div className="sw-list-heading">
            <div>
              <Star />
              <h2>{uiText("我的自选")}</h2>
              <span>{watchlist.rows.length}</span>
            </div>
            <span className="sw-small">{uiText("跨市场，一处关注")}</span>
          </div>
          <label className="sw-search">
            <Search />
            <input
              aria-label={uiText("搜索自选")}
              placeholder={uiText("名称或代码")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            {query && (
              <button aria-label={uiText("清空搜索")} onClick={() => setQuery("")}>
                <X />
              </button>
            )}
          </label>
          <div className="sw-filters">
            {(["all", "CN", "HK", "US"] as const).map((m) => (
              <button
                key={m}
                className={filter === m ? "active" : ""}
                onClick={() => setFilter(m)}
              >
                {m === "all" ? uiText("全部") : marketNames[m]}
              </button>
            ))}
          </div>
          <div className="sw-list-columns">
            <span>{uiText("名称 / 代码")}</span>
            <span>{uiText("价格 / 涨跌幅")}</span>
          </div>
          <div className="sw-watchlist">
            {rows.map((r) => {
              const i = r.data,
                k = keyOf(i),
                q = quotes[k],
                quote = q?.quote;
              return (
                <button
                  key={r.id}
                  className={`sw-stock ${instrumentKey === k ? "active" : ""}`}
                  onClick={() => setActive(k)}
                >
                  <div>
                    <strong>{i.name}</strong>
                    <small>
                      <span>{i.market}</span>
                      {i.symbol}
                    </small>
                  </div>
                  <div className={changeClass(quote?.changePercent)}>
                    <strong>{price(quote?.price)}</strong>
                    <small>
                      {quote?.changePercent === undefined
                        ? quoteErrors[k]
                          ? uiText("暂不可用")
                          : uiText("读取中…")
                        : `${sign(quote.changePercent)}${quote.changePercent.toFixed(2)}%`}
                      {q?.stale && uiText(" · 缓存")}
                    </small>
                  </div>
                </button>
              );
            })}
            {!rows.length && (
              <div className="sw-list-empty">
                <Search />
                <p>{query ? uiText("没有找到自选股") : uiText("这个市场还没有自选股")}</p>
                <button
                  onClick={() => {
                    setAdd(true);
                    setAddMarket(filter === "all" ? "CN" : filter);
                  }}
                >
                  {uiText("添加一只")}{" "}</button>
              </div>
            )}
          </div>
          <div className="sw-sidebar-bottom">
            <ShieldCheck />
            <span>
              {uiText("行情从这台电脑获取")}{" "}<br />
              <small>{uiText("自选列表与备注在工作区共享")}</small>
            </span>
          </div>
        </aside>
        <main className="sw-main">
          {instrument ? (
            <>
              <section className="sw-security">
                <div className="sw-security-top">
                  <div className="sw-security-title">
                    <span className="sw-market-label">
                      {marketNames[instrument.market]}
                    </span>
                    <h1>{instrument.name}</h1>
                    <span className="sw-symbol">{instrument.symbol}</span>
                  </div>
                  <div className="sw-security-actions">
                    <button
                      className="sw-icon"
                      title={uiText("移出自选")}
                      aria-label={uiText("移出自选")}
                      onClick={remove}
                    >
                      <Star fill="currentColor" />
                    </button>
                    <button
                      className="sw-icon"
                      title={uiText("导出行情")}
                      aria-label={uiText("导出行情")}
                      disabled={!chartData}
                      onClick={(e) => setMenu(rectOf(e.currentTarget))}
                    >
                      <Download />
                    </button>
                  </div>
                </div>
                <div className="sw-price-line">
                  <span
                    className={`sw-price ${changeClass(selectedQuote?.changePercent)}`}
                  >
                    {price(selectedQuote?.price)}
                  </span>
                  <span className="sw-currency">
                    {selectedQuote?.currency ||
                      { CN: "CNY", HK: "HKD", US: "USD" }[instrument.market]}
                  </span>
                  <div
                    className={`sw-change ${changeClass(selectedQuote?.changePercent)}`}
                  >
                    {selectedQuote?.changePercent !== undefined ? (
                      <>
                        {selectedQuote.changePercent >= 0 ? (
                          <ArrowUpRight />
                        ) : (
                          <ArrowDown />
                        )}
                        {sign(selectedQuote.change)}
                        {price(selectedQuote.change)}
                        <span>
                          ({sign(selectedQuote.changePercent)}
                          {selectedQuote.changePercent.toFixed(2)}%)
                        </span>
                      </>
                    ) : (
                      <span>
                        {selectedQuote ? uiText("涨跌基准未提供") : uiText("等待有效报价")}
                      </span>
                    )}
                  </div>
                </div>
                <div className="sw-price-meta">
                  <span>
                    {selectedQuote?.at
                      ? uiText("源站报价 {0}", labelTime(selectedQuote.at, instrument.market))
                      : uiText("报价时间未提供")}
                  </span>
                  <i />
                  <span>{timezone[instrument.market]}</span>
                  <i />
                  <span>{uiText("与前一交易日收盘相比")}</span>
                </div>
                <div className="sw-stats">
                  {[
                    ["昨收", selectedQuote?.previousClose],
                    ["最新日开盘", chartData?.session?.open],
                    ["最新日最高", chartData?.session?.high],
                    ["最新日最低", chartData?.session?.low],
                  ].map(([label, value]) => (
                    <div key={String(label)}>
                      <span>{uiText(String(label))}</span>
                      <strong>{price(value as number | undefined)}</strong>
                    </div>
                  ))}
                </div>
              </section>
              <section className="sw-chart-section">
                <div className="sw-chart-toolbar">
                  <div className="sw-periods">
                    {(
                      [
                        ["1d", "日 K"],
                        ["1wk", "周 K"],
                        ["1mo", "月 K"],
                      ] as const
                    ).map(([id, label]) => (
                      <button
                        className={interval === id ? "active" : ""}
                        key={id}
                        onClick={() => {
                          setInterval(id);
                          setForce(false);
                        }}
                      >
                        {uiText(String(label))}
                      </button>
                    ))}
                  </div>
                  <span className="sw-adjustment">{uiText("不复权")}</span>
                  <div className="sw-chart-tools">
                    <button
                      className={`sw-icon ${mode === "candles" ? "active" : ""}`}
                      title={uiText("K 线图")}
                      aria-label={uiText("K 线图")}
                      onClick={() => setMode("candles")}
                    >
                      <ChartCandlestick />
                    </button>
                    <button
                      className={`sw-icon ${mode === "line" ? "active" : ""}`}
                      title={uiText("收盘价折线")}
                      aria-label={uiText("收盘价折线")}
                      onClick={() => setMode("line")}
                    >
                      <LineChart />
                    </button>
                    <button
                      className={`sw-mini ${ma ? "active" : ""}`}
                      onClick={() => setMA(!ma)}
                      aria-pressed={ma}
                    >
                      MA
                    </button>
                    <span className="sw-tool-separator" />
                    <button
                      className="sw-icon"
                      title={uiText("展开图表")}
                      aria-label={uiText("展开图表")}
                      onClick={() => setExpanded(!expanded)}
                    >
                      <Maximize2 />
                    </button>
                  </div>
                </div>
                {loading ? (
                  <div className="sw-chart-loading">
                    <div className="sw-skeleton-line" />
                    <div className="sw-skeleton-line" />
                    <div className="sw-skeleton-line" />
                    <span>
                      <LoaderCircle />
                      {uiText("正在读取")}{marketNames[instrument.market]}{uiText("行情…")}{" "}</span>
                  </div>
                ) : error ? (
                  <div className="sw-chart-empty" role="alert">
                    <Globe2 />
                    <h3>{uiText("这次没能取到行情")}</h3>
                    <p>{error}</p>
                    <div>
                      <button className="sw-button" onClick={doRefresh}>
                        <RefreshCw />
                        {uiText("重试")}{" "}</button>
                      <button className="sw-button" onClick={openSettings}>
                        {uiText("切换数据源")}{" "}</button>
                    </div>
                  </div>
                ) : chartData ? (
                  <PriceChart
                    data={chartData}
                    dark={dark}
                    redUp={settings.redUp}
                    mode={mode}
                    averages={ma}
                    onCapture={setCapture}
                  />
                ) : (
                  <div className="sw-chart-empty">{uiText("选择股票后显示 K 线")}</div>
                )}
                <div
                  className={`sw-data-footer ${chartData?.stale ? "stale" : ""}`}
                >
                  <div>
                    <span className="sw-data-dot" />
                    <strong>
                      {chartData?.source || sources[settings.source]}
                    </strong>
                    <span>
                      {chartData?.stale
                        ? uiText("离线缓存 · 不是最新行情")
                        : chartData?.cached
                          ? uiText("本机缓存")
                          : uiText("源站数据")}
                    </span>
                    {chartData && (
                      <span>
                        {chartData.bars.length} {" "}{uiText("根 · 获取于")}{" "}
                        {labelTime(chartData.fetchedAt, instrument.market)}
                      </span>
                    )}
                  </div>
                  <button onClick={doRefresh} disabled={refreshDisabled}>
                    <RefreshCw className={loading ? "spin" : ""} />
                    {uiText("刷新")}{" "}</button>
                </div>
              </section>
              <section className="sw-bottom">
                <div className="sw-notes">
                  <div className="sw-section-title">
                    <h2>{uiText("观察笔记")}</h2>
                    <span>{kit.pending ? uiText("正在保存…") : uiText("与 agent 共享")}</span>
                  </div>
                  <textarea
                    aria-label={uiText("观察笔记")}
                    placeholder={uiText("写下关注这只股票的原因、重要价格或待观察的变化…")}
                    value={record?.data.note || ""}
                    maxLength={6000}
                    onChange={(e) => {
                      if (record)
                        kit.track(
                          watchlist.put(
                            { ...record.data, note: e.target.value },
                            record.id,
                          ),
                        );
                    }}
                  />
                </div>
                <div className="sw-data-info">
                  <div className="sw-section-title">
                    <h2>{uiText("数据说明")}</h2>
                    <button onClick={openSettings}>
                      {uiText("管理来源")}{" "}<ArrowUpRight />
                    </button>
                  </div>
                  <p>
                    {chartData?.warnings.join("；") ||
                      uiText("报价与 K 线均来自所选数据源。源站不可用时会提示错误，不使用模拟价格。")}
                  </p>
                  <p>
                    {uiText("末根 K 线可能尚未收盘；成交量单位为股。各市场币种独立，不进行汇率换算。")}{" "}</p>
                </div>
              </section>
            </>
          ) : (
            <div className="sw-welcome">
              <span className="sw-welcome-icon">
                <ChartCandlestick />
              </span>
              <span className="sw-eyebrow">YOUR MARKETS, ONE WORKSPACE</span>
              <h1>
                {uiText("把关注的市场，")}{" "}<br />
                {uiText("放在同一个视野里。")}{" "}</h1>
              <p>
                {uiText("A 股、港股、美股。自选、K 线和观察笔记，")}{" "}<br />
                {uiText("跟着你的思路走。")}{" "}</p>
              <button
                className="sw-button primary"
                onClick={() => setAdd(true)}
              >
                <Plus />
                {uiText("添加第一只自选股")}{" "}</button>
              <div className="sw-welcome-cards">
                {(["CN", "HK", "US"] as const).map((m) => (
                  <button
                    key={m}
                    onClick={() => {
                      setAddMarket(m);
                      setAdd(true);
                    }}
                  >
                    <span>{m}</span>
                    <strong>{marketNames[m]}</strong>
                    <ArrowUpRight />
                  </button>
                ))}
              </div>
            </div>
          )}
        </main>
      </div>
      <footer className="sw-app-footer">
        <span>
          <span
            className={settings.autoRefresh ? "sw-live-dot" : "sw-paused-dot"}
          />
          {settings.autoRefresh ? uiText("每 60 秒刷新 · 后台暂停") : uiText("手动刷新")}
        </span>
        <span>{uiText("行情仅作信息展示 · 延迟以源站为准")}</span>
        <button onClick={openSettings}>
          <Settings2 />
          {uiText("显示与数据设置")}{" "}</button>
      </footer>
      {add && (
        <div
          className="sw-overlay"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget && !adding) setAdd(false);
          }}
        >
          <section
            className="sw-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="sw-add-title"
          >
            <header>
              <div>
                <span className="sw-eyebrow">FOLLOW WHAT MATTERS</span>
                <h2 id="sw-add-title">{uiText("添加自选股")}</h2>
              </div>
              <button
                className="sw-icon"
                aria-label={uiText("关闭")}
                disabled={adding}
                onClick={() => setAdd(false)}
              >
                <X />
              </button>
            </header>
            <div className="sw-dialog-markets">
              {(["CN", "HK", "US"] as const).map((m) => (
                <button
                  className={addMarket === m ? "active" : ""}
                  key={m}
                  disabled={adding}
                  onClick={() => {
                    setAddMarket(m);
                    setAddQuery("");
                    setNewError("");
                  }}
                >
                  {marketNames[m]}
                </button>
              ))}
            </div>
            <label className="sw-dialog-search">
              <Search />
              <input
                autoFocus
                aria-label={uiText("股票名称或代码")}
                placeholder={
                  addMarket === "CN"
                    ? uiText("常用名称，或输入 600519 / 000001.SZ")
                    : addMarket === "HK"
                      ? uiText("常用名称，或输入 00700 / 0700.HK")
                      : uiText("常用名称，或输入 AAPL / MSFT")
                }
                value={addQuery}
                disabled={adding}
                onChange={(e) => {
                  setAddQuery(e.target.value);
                  setNewError("");
                }}
                onKeyDown={(e) => {
                  if (e.key === "Escape" && !adding) setAdd(false);
                }}
              />
            </label>
            <div className="sw-search-caption">
              {addQuery
                ? uiText("名称匹配常用列表；任意股票可直接输入代码")
                : uiText("常用股票 · 添加前验证行情是否可用")}
            </div>
            <div className="sw-add-results">
              {[
                ...candidates,
                ...(customCandidate ? [customCandidate] : []),
              ].map((i) => {
                const exists = watchlist.rows.some(
                  (r) => keyOf(r.data) === keyOf(i),
                );
                return (
                  <button
                    key={i.symbol}
                    disabled={adding}
                    onClick={() => void addInstrument(i)}
                  >
                    <span className="sw-result-market">{i.market}</span>
                    <div>
                      <strong>{i.name}</strong>
                      <small>{i.symbol}</small>
                    </div>
                    {exists ? <Check /> : <Plus />}
                  </button>
                );
              })}
              {!candidates.length && !customCandidate && (
                <div className="sw-no-results">
                  {uiText("没有匹配。请直接输入这个市场的证券代码。")}{" "}</div>
              )}
            </div>
            {newError && (
              <p className="sw-form-error" role="alert">
                {newError}
              </p>
            )}
            {adding && (
              <p className="sw-form-hint">
                <LoaderCircle className="spin" />
                {uiText("正在验证代码与行情…")}{" "}</p>
            )}
            <footer>
              <span>{uiText("数据源：")}{sources[settings.source]}</span>
              <button
                className="sw-button"
                disabled={adding}
                onClick={() => setAdd(false)}
              >
                {uiText("完成")}{" "}</button>
            </footer>
          </section>
        </div>
      )}
      {showSettings && (
        <div className="sw-overlay">
          <section
            className="sw-dialog sw-settings"
            role="dialog"
            aria-modal="true"
            aria-labelledby="sw-settings-title"
          >
            <header>
              <div>
                <span className="sw-eyebrow">YOUR DATA, YOUR CHOICE</span>
                <h2 id="sw-settings-title">{uiText("显示与数据设置")}</h2>
              </div>
              <button
                className="sw-icon"
                aria-label={uiText("关闭")}
                onClick={() => setShowSettings(false)}
              >
                <X />
              </button>
            </header>
            <label className="sw-field">
              {uiText("行情数据源")}{" "}<select
                aria-label={uiText("行情数据源")}
                value={draft.source}
                onChange={(e) =>
                  setDraft({ ...draft, source: e.target.value as ProviderID })
                }
              >
                {Object.entries(sources).map(([id, label]) => (
                  <option key={id} value={id}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <div className="sw-provider-info">
              {draft.source === "auto"
                ? uiText("优先 Yahoo Finance；不可用时，A 股和港股尝试腾讯行情。美股不会降级到不完整的数据源。")
                : draft.source === "yahoo"
                  ? uiText("覆盖 A 股、港股、美股。使用公开接口，无需密钥；可能限流或变更。")
                  : draft.source === "tencent"
                    ? uiText("支持沪深 A 股与港股历史行情。美股和北交所历史数据不在此适配器的支持范围。")
                    : uiText("接入自己的 AKShare、Tushare 或其他服务。网关统一返回证券、报价与 OHLCV，界面无需修改。")}
            </div>
            {draft.source === "gateway" && (
              <label className="sw-field">
                {uiText("网关地址")}{" "}<input
                  aria-label={uiText("行情网关地址")}
                  placeholder="https://quotes.example.com/v1"
                  value={draft.gateway}
                  onChange={(e) =>
                    setDraft({ ...draft, gateway: e.target.value })
                  }
                />
                <small>
                  {uiText("将请求此地址下的 /bars。请在网关服务端管理密钥，地址中不要放凭证。")}{" "}</small>
              </label>
            )}
            <label className="sw-setting-row">
              <span>
                <strong>{uiText("自动刷新")}</strong>
                <small>{uiText("每 60 秒获取；窗口在后台时暂停")}</small>
              </span>
              <input
                type="checkbox"
                role="switch"
                aria-label={uiText("自动刷新")}
                checked={draft.autoRefresh}
                onChange={(e) =>
                  setDraft({ ...draft, autoRefresh: e.target.checked })
                }
              />
            </label>
            <label className="sw-field">
              {uiText("涨跌颜色")}{" "}<select
                aria-label={uiText("涨跌颜色")}
                value={draft.redUp ? "red" : "green"}
                onChange={(e) =>
                  setDraft({ ...draft, redUp: e.target.value === "red" })
                }
              >
                <option value="red">{uiText("红涨绿跌")}</option>
                <option value="green">{uiText("绿涨红跌")}</option>
              </select>
            </label>
            <label className="sw-field">
              {uiText("外观")}{" "}<select
                aria-label={uiText("外观")}
                disabled={myco?.features?.includes("preferences")}
                value={myco?.features?.includes("preferences") ? "system" : draft.theme}
                onChange={(e) =>
                  setDraft({
                    ...draft,
                    theme: e.target.value as LocalSettings["theme"],
                  })
                }
              >
                <option value="system">{uiText(myco?.features?.includes("preferences") ? "跟随 App" : "跟随系统")}</option>
                <option value="light">{uiText("浅色")}</option>
                <option value="dark">{uiText("深色")}</option>
              </select>
            </label>
            <p className="sw-form-hint">
              {uiText("设置与行情缓存只保存在这台电脑；自选股与观察笔记通过工作区共享。使用公开接口时，请遵守数据源的使用条款。")}{" "}</p>
            {settingsError && (
              <p className="sw-form-error" role="alert">
                {settingsError}
              </p>
            )}
            <footer>
              <span>{uiText("不绑定任何一家数据商")}</span>
              <button className="sw-button primary" onClick={applySettings}>
                {uiText("保存设置")}{" "}</button>
            </footer>
          </section>
        </div>
      )}
      {menu && (
        <Popover at={menu} align="right" onClose={() => setMenu(null)}>
          <div className="menu">
            <button
              className="menu-item"
              onClick={() => void exportData("csv")}
            >
              {uiText("导出 CSV")}{" "}</button>
            <button
              className="menu-item"
              onClick={() => void exportData("json")}
            >
              {uiText("导出 JSON（含来源与时间）")}{" "}</button>
            <button
              className="menu-item"
              onClick={() => void exportData("png")}
            >
              {uiText("导出图表 PNG")}{" "}</button>
          </div>
        </Popover>
      )}
      <Toast kit={kit} />
    </div>
  );
}
void mount(kit, App);
