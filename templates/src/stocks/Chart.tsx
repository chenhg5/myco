import { useEffect, useRef, useState } from "react";
import {
  CandlestickSeries,
  ColorType,
  CrosshairMode,
  HistogramSeries,
  LineSeries,
  createChart,
  type IChartApi,
  type Time,
} from "lightweight-charts";
import {
  movingAverage,
  price,
  compact,
  type Candle,
  type MarketData,
} from "./model";
export function PriceChart({
  data,
  dark,
  redUp,
  mode,
  averages,
  onCapture,
}: {
  data: MarketData;
  dark: boolean;
  redUp: boolean;
  mode: "candles" | "line";
  averages: boolean;
  onCapture: (capture: (() => Promise<Blob>) | null) => void;
}) {
  const host = useRef<HTMLDivElement>(null),
    api = useRef<IChartApi | null>(null),
    [hover, setHover] = useState<Candle | null>(null);
  const bars = data.bars,
    last = bars[bars.length - 1],
    shown = hover || last;
  useEffect(() => {
    if (!host.current) return;
    const fg = dark ? "#aaa" : "#747474",
      bg = dark ? "#202020" : "#ffffff",
      line = dark ? "#303030" : "#efefef",
      up = redUp ? "#d65c57" : "#478c73",
      down = redUp ? "#478c73" : "#d65c57";
    const chart = createChart(host.current, {
      autoSize: true,
      layout: {
        background: { type: ColorType.Solid, color: bg },
        textColor: fg,
        fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, sans-serif",
        fontSize: 11,
        attributionLogo: true,
      },
      grid: { vertLines: { visible: false }, horzLines: { color: line } },
      rightPriceScale: {
        borderVisible: false,
        scaleMargins: { top: 0.12, bottom: 0.26 },
      },
      timeScale: { borderVisible: false, timeVisible: false, rightOffset: 6 },
      crosshair: {
        mode: CrosshairMode.Normal,
        vertLine: { color: fg, labelBackgroundColor: dark ? "#444" : "#555" },
        horzLine: { color: fg, labelBackgroundColor: dark ? "#444" : "#555" },
      },
      localization: { locale: "zh-CN" },
    });
    api.current = chart;
    const precision =
      data.instrument.market === "HK" ||
      (data.instrument.market === "CN" &&
        /^[15]\d{5}\./.test(data.instrument.symbol))
        ? 3
        : data.quote.price < 1
          ? 4
          : 2;
    if (mode === "candles")
      chart
        .addSeries(CandlestickSeries, {
          upColor: up,
          downColor: down,
          borderVisible: false,
          wickUpColor: up,
          wickDownColor: down,
          priceFormat: { type: "price", precision, minMove: 10 ** -precision },
        })
        .setData(bars.map((b) => ({ ...b, time: b.time as Time })));
    else
      chart
        .addSeries(LineSeries, {
          color: dark ? "#ddd" : "#333",
          lineWidth: 2,
          lastValueVisible: true,
          priceLineVisible: true,
          priceFormat: { type: "price", precision, minMove: 10 ** -precision },
        })
        .setData(bars.map((b) => ({ time: b.time as Time, value: b.close })));

    const vol = chart.addSeries(HistogramSeries, {
      priceFormat: { type: "volume" },
      priceScaleId: "volume",
      lastValueVisible: false,
      priceLineVisible: false,
    });
    chart
      .priceScale("volume")
      .applyOptions({ scaleMargins: { top: 0.82, bottom: 0 }, visible: false });
    vol.setData(
      bars.map((b) => ({
        time: b.time as Time,
        value: b.volume,
        color: (b.close >= b.open ? up : down) + "66",
      })),
    );
    if (averages)
      [5, 10, 20].forEach((n, j) => {
        const s = chart.addSeries(LineSeries, {
          color: [
            dark ? "#dedede" : "#555555",
            dark ? "#a0a0a0" : "#888888",
            dark ? "#696969" : "#b6b6b6",
          ][j],
          lineWidth: 1,
          lastValueVisible: false,
          priceLineVisible: false,
          crosshairMarkerVisible: false,
        });
        s.setData(
          movingAverage(bars, n).map((b) => ({ ...b, time: b.time as Time })),
        );
      });
    const byTime = new Map(bars.map((b) => [b.time, b]));
    chart.subscribeCrosshairMove((p) => {
      const t = p.time;
      const date =
        typeof t === "string"
          ? t
          : typeof t === "number"
            ? new Date(t * 1000).toISOString().slice(0, 10)
            : t
              ? `${t.year}-${String(t.month).padStart(2, "0")}-${String(t.day).padStart(2, "0")}`
              : "";
      setHover(byTime.get(date) || null);
    });
    chart.timeScale().setVisibleLogicalRange({
      from: Math.max(0, bars.length - 100),
      to: bars.length + 3,
    });
    setHover(null);
    onCapture(
      () =>
        new Promise<Blob>((resolve, reject) => {
          const canvas = chart.takeScreenshot();
          canvas.toBlob(
            (b) => (b ? resolve(b) : reject(new Error("图表导出失败"))),
            "image/png",
          );
        }),
    );
    return () => {
      onCapture(null);
      api.current = null;
      chart.remove();
    };
  }, [data, dark, redUp, mode, averages, onCapture]);
  return (
    <div className="sw-chart-wrap">
      <div className="sw-ohlc">
        <span>{shown?.time || "—"}</span>
        {shown && (
          <>
            <span>
              开 <b>{price(shown.open)}</b>
            </span>
            <span>
              高 <b>{price(shown.high)}</b>
            </span>
            <span>
              低 <b>{price(shown.low)}</b>
            </span>
            <span>
              收 <b>{price(shown.close)}</b>
            </span>
            <span>
              量 <b>{compact(shown.volume)}</b>
            </span>
          </>
        )}
        {averages && <span className="sw-ma-legend">MA5 · MA10 · MA20</span>}
      </div>
      <div className="sw-chart" ref={host} data-testid="price-chart" />
      <span className="sw-volume-label">成交量 · 股</span>
    </div>
  );
}
