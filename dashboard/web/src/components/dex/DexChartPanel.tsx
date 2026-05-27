// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { CandlestickChart } from 'lucide-react';
import { CandleType, dispose, init } from 'klinecharts';
import type { DexMarket } from '../../services/dcrdexApi';
import type { Candle } from './useDexFeed';
import { DexChartToolbar, type DrawTool, type IndicatorMeta } from './DexChartToolbar';
import { loadChartPrefs, saveChartPrefs, type DexChartType } from './dexChartPrefs';

interface Props {
  market: DexMarket;
  candles: Candle[];
  durs: string[];
  dur: string;
  onDur: (d: string) => void;
}

const UP = 'hsl(142 76% 40%)';
const DOWN = 'hsl(0 72% 55%)';
const GRID = 'hsl(217 32% 17%)';
const AXIS = 'hsl(215 20% 55%)';

// Toolbar indicators: MA/EMA/BOLL overlay the price (candle) pane; the rest get
// their own sub-pane. The pane id is stable per indicator so it can be removed.
const INDICATORS: readonly IndicatorMeta[] = [
  { name: 'MA', label: 'MA', pane: 'main' },
  { name: 'EMA', label: 'EMA', pane: 'main' },
  { name: 'BOLL', label: 'BOLL', pane: 'main' },
  { name: 'VOL', label: 'Volume', pane: 'sub' },
  { name: 'MACD', label: 'MACD', pane: 'sub' },
  { name: 'RSI', label: 'RSI', pane: 'sub' },
  { name: 'KDJ', label: 'KDJ', pane: 'sub' },
];
const DRAW_TOOLS: readonly DrawTool[] = [
  { name: 'segment', label: 'Trend line' },
  { name: 'horizontalStraightLine', label: 'Horizontal line' },
  { name: 'rayLine', label: 'Ray' },
  { name: 'fibonacciLine', label: 'Fibonacci' },
];
const paneIdFor = (ind: IndicatorMeta) => (ind.pane === 'main' ? 'candle_pane' : `pane_${ind.name.toLowerCase()}`);
const toCandleType = (t: DexChartType): CandleType => (t === 'area' ? CandleType.Area : CandleType.CandleSolid);

type ChartApi = ReturnType<typeof init>;

// DexChartPanel renders an interactive candlestick + volume chart with
// KLineCharts (v9): crosshair with a cursor-following O/H/L/C legend, wheel zoom
// and drag pan, and live last-bar updates from the candle feed. KLineCharts is a
// self-contained canvas renderer (no network/telemetry); its built-in indicators
// and drawing tools are available for a future toolbar. Candle data and the
// available bin sizes (the DEX server's candleDurs) are supplied by the caller;
// selecting one re-requests the feed.
export const DexChartPanel = ({ market, candles, durs, dur, onDur }: Props) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<ChartApi>(null);
  // State for deciding full-reload vs last-bar update (see the data effect).
  const seriesKeyRef = useRef('');
  const firstTsRef = useRef(0);
  const lenRef = useRef(0);

  // Toolbar selection, persisted across reloads.
  const [chartType, setChartType] = useState<DexChartType>(() => loadChartPrefs().chartType);
  const [active, setActive] = useState<Set<string>>(() => new Set(loadChartPrefs().indicators));
  const activeRef = useRef(active);
  activeRef.current = active;
  const chartTypeRef = useRef(chartType);
  chartTypeRef.current = chartType;

  // Price precision from the market's rate step (in conventional units).
  const rateConv = (1e8 / market.baseConvFactor) * market.quoteConvFactor;
  const stepConv = market.rateStep > 0 ? market.rateStep / rateConv : 0;
  const pricePrecision = stepConv > 0 ? Math.min(8, Math.max(2, Math.ceil(-Math.log10(stepConv)))) : 8;

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const chart = init(el);
    if (!chart) return;
    // KLineCharts draws labels on a canvas with its own default font; match the
    // page font (Inter) by applying the body's resolved family to every text style.
    const family = getComputedStyle(document.body).fontFamily || 'Inter, system-ui, sans-serif';
    chart.setStyles({
      grid: { horizontal: { color: GRID }, vertical: { color: GRID } },
      candle: {
        bar: {
          upColor: UP,
          downColor: DOWN,
          noChangeColor: AXIS,
          upBorderColor: UP,
          downBorderColor: DOWN,
          noChangeBorderColor: AXIS,
          upWickColor: UP,
          downWickColor: DOWN,
          noChangeWickColor: AXIS,
        },
        priceMark: { high: { textFamily: family }, low: { textFamily: family }, last: { text: { family } } },
        tooltip: { text: { family } },
      },
      xAxis: { axisLine: { color: GRID }, tickLine: { color: GRID }, tickText: { color: AXIS, family } },
      yAxis: { axisLine: { color: GRID }, tickLine: { color: GRID }, tickText: { color: AXIS, family } },
      crosshair: {
        horizontal: { line: { color: AXIS }, text: { family } },
        vertical: { line: { color: AXIS }, text: { family } },
      },
      indicator: { tooltip: { text: { family } }, lastValueMark: { text: { family } } },
    });
    // Apply the persisted chart type and indicators (default Volume).
    chart.setStyles({ candle: { type: toCandleType(chartTypeRef.current) } });
    activeRef.current.forEach((name) => {
      const ind = INDICATORS.find((i) => i.name === name);
      if (ind) chart.createIndicator(name, ind.pane === 'main', { id: paneIdFor(ind) });
    });
    chartRef.current = chart;
    seriesKeyRef.current = '';
    firstTsRef.current = 0;
    lenRef.current = 0;

    // KLineCharts does not auto-resize; drive it from the container size.
    const ro = new ResizeObserver(() => chart.resize());
    ro.observe(el);
    return () => {
      ro.disconnect();
      dispose(el);
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    chartRef.current?.setPriceVolumePrecision(pricePrecision, 2);
  }, [pricePrecision]);

  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;

    // Dedupe by bar time (keep latest) and sort ascending.
    const byTime = new Map<number, Candle>();
    for (const c of candles) byTime.set(c.startStamp, c);
    const list = [...byTime.values()]
      .sort((a, b) => a.startStamp - b.startStamp)
      .map((c) => ({ timestamp: c.startStamp, open: c.open, high: c.high, low: c.low, close: c.close, volume: c.volume }));

    // The feed delivers full snapshots (initial load, timeframe/market switch,
    // reconnect), not just last-bar appends. Replace the dataset for any of
    // those (key change, different first bar, or a size jump); only a same-series
    // in-place update or single append goes through updateData, which preserves
    // the current zoom/pan and never receives an out-of-order bar.
    const key = `${market.baseID}-${market.quoteID}-${dur}`;
    const firstTs = list.length ? list[0].timestamp : 0;
    const fresh =
      key !== seriesKeyRef.current ||
      firstTs !== firstTsRef.current ||
      list.length < lenRef.current ||
      list.length > lenRef.current + 1;

    if (list.length === 0) {
      chart.applyNewData([]);
    } else if (fresh) {
      chart.applyNewData(list);
    } else {
      chart.updateData(list[list.length - 1]);
    }
    seriesKeyRef.current = key;
    firstTsRef.current = firstTs;
    lenRef.current = list.length;
  }, [candles, dur, market.baseID, market.quoteID]);

  const applyIndicator = (name: string, on: boolean) => {
    const chart = chartRef.current;
    const ind = INDICATORS.find((i) => i.name === name);
    if (!chart || !ind) return;
    if (on) chart.createIndicator(name, ind.pane === 'main', { id: paneIdFor(ind) });
    else chart.removeIndicator(paneIdFor(ind), name);
  };
  const toggleIndicator = (name: string) => {
    setActive((prev) => {
      const next = new Set(prev);
      const on = !next.has(name);
      if (on) next.add(name);
      else next.delete(name);
      applyIndicator(name, on);
      saveChartPrefs({ chartType: chartTypeRef.current, indicators: [...next] });
      return next;
    });
  };
  const changeChartType = (t: DexChartType) => {
    setChartType(t);
    chartRef.current?.setStyles({ candle: { type: toCandleType(t) } });
    saveChartPrefs({ chartType: t, indicators: [...activeRef.current] });
  };

  return (
    <div className="flex flex-col min-h-0 h-full">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border/50 text-xs">
        <div className="flex items-center gap-1 overflow-x-auto">
          {durs.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => onDur(t)}
              className={`px-2 py-1 rounded font-medium transition-colors ${
                dur === t ? 'bg-muted/40 text-foreground' : 'text-muted-foreground hover:text-foreground/80'
              }`}
            >
              {t}
            </button>
          ))}
        </div>
        <div className="ml-auto shrink-0">
          <DexChartToolbar
            chartType={chartType}
            onChartType={changeChartType}
            indicators={INDICATORS}
            active={active}
            onToggle={toggleIndicator}
            drawTools={DRAW_TOOLS}
            onDraw={(name) => chartRef.current?.createOverlay(name)}
            onClearDraw={() => chartRef.current?.removeOverlay()}
          />
        </div>
      </div>
      <div className="flex-1 min-h-0 relative">
        <div ref={containerRef} className="absolute inset-0" />
        {candles.length === 0 && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-muted-foreground pointer-events-none">
            <CandlestickChart className="h-8 w-8 opacity-40" />
            <span className="text-sm">No candle data yet</span>
            <span className="text-xs text-muted-foreground/60">Waiting for the DEX candle feed</span>
          </div>
        )}
      </div>
    </div>
  );
};
