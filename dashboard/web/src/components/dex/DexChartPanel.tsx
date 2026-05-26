// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo } from 'react';
import { CandlestickChart } from 'lucide-react';
import { fmtPrice } from './dexFormat';
import type { DexMarket } from '../../services/dcrdexApi';
import type { Candle } from './useDexFeed';

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

// DexChartPanel renders a lightweight SVG candlestick + volume chart. No
// external charting dependency: a single scalable <svg> keeps the trading view
// fast to paint. Candle data and the available bin sizes (the DEX server's
// candleDurs) are supplied by the caller; selecting one re-requests the feed.
export const DexChartPanel = ({ market, candles, durs, dur, onDur }: Props) => {
  const svg = useMemo(() => {
    if (candles.length === 0) return null;
    const W = 1000;
    const H = 440;
    const padR = 60;
    const padT = 10;
    const padB = 6;
    const cw = W - padR;
    const ch = H - padT - padB;
    const priceH = ch * 0.78;
    const volTop = padT + ch * 0.84;
    const volBot = padT + ch;

    const N = candles.length;
    const highs = candles.map((c) => c.high);
    const lows = candles.map((c) => c.low);
    const pMax = Math.max(...highs);
    const pMin = Math.min(...lows);
    const pad = (pMax - pMin) * 0.06 || pMax * 0.01;
    const hi = pMax + pad;
    const lo = pMin - pad;
    const vMax = Math.max(...candles.map((c) => c.volume));

    const xStep = cw / N;
    const bodyW = xStep * 0.6;
    const yP = (p: number) => padT + (1 - (p - lo) / (hi - lo)) * priceH;
    const yV = (v: number) => volBot - (v / vMax) * (volBot - volTop);

    const grid: JSX.Element[] = [];
    for (let i = 0; i <= 4; i++) {
      const y = padT + (priceH * i) / 4;
      const lbl = hi - ((hi - lo) * i) / 4;
      grid.push(<line key={`g${i}`} x1={0} y1={y} x2={cw} y2={y} stroke={GRID} strokeWidth={1} strokeDasharray="2 4" />);
      grid.push(
        <text key={`gt${i}`} x={cw + 5} y={y + 3.5} fontSize={11} fontFamily="monospace" fill={AXIS}>
          {fmtPrice(lbl, market.quote)}
        </text>,
      );
    }

    const bars: JSX.Element[] = [];
    candles.forEach((c, i) => {
      const x = i * xStep + xStep / 2;
      const up = c.close >= c.open;
      const color = up ? UP : DOWN;
      const bodyTop = yP(Math.max(c.open, c.close));
      const bodyH = Math.max(1, Math.abs(yP(c.open) - yP(c.close)));
      bars.push(<line key={`w${i}`} x1={x} y1={yP(c.high)} x2={x} y2={yP(c.low)} stroke={color} strokeWidth={1} />);
      bars.push(<rect key={`b${i}`} x={x - bodyW / 2} y={bodyTop} width={bodyW} height={bodyH} fill={color} />);
      bars.push(<rect key={`v${i}`} x={x - bodyW / 2} y={yV(c.volume)} width={bodyW} height={volBot - yV(c.volume)} fill={color} opacity={0.3} />);
    });

    const last = candles[N - 1].close;
    const lastY = yP(last);

    return (
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="w-full h-full">
        {grid}
        {bars}
        <line x1={0} y1={lastY} x2={cw} y2={lastY} stroke={UP} strokeWidth={1} strokeDasharray="3 3" opacity={0.8} />
        <rect x={cw} y={lastY - 9} width={padR - 2} height={18} fill={UP} rx={2} />
        <text x={cw + 4} y={lastY + 4} fontSize={11} fontFamily="monospace" fontWeight={600} fill="hsl(222 47% 8%)">
          {fmtPrice(last, market.quote)}
        </text>
      </svg>
    );
  }, [candles, market.quote]);

  const lastCandle = candles[candles.length - 1];

  return (
    <div className="flex flex-col min-h-0 h-full">
      <div className="flex items-center gap-1 px-3 py-2 border-b border-border/50 text-xs overflow-x-auto">
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
        {lastCandle && (
          <span className="ml-auto flex items-center gap-3 font-mono tabular-nums text-[11px] whitespace-nowrap">
            <span><span className="text-muted-foreground/60 mr-1">O</span>{fmtPrice(lastCandle.open, market.quote)}</span>
            <span><span className="text-muted-foreground/60 mr-1">H</span><span className="text-success">{fmtPrice(lastCandle.high, market.quote)}</span></span>
            <span><span className="text-muted-foreground/60 mr-1">L</span><span className="text-destructive">{fmtPrice(lastCandle.low, market.quote)}</span></span>
            <span><span className="text-muted-foreground/60 mr-1">C</span>{fmtPrice(lastCandle.close, market.quote)}</span>
          </span>
        )}
      </div>
      <div className="flex-1 min-h-0 relative">
        {svg ?? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-muted-foreground">
            <CandlestickChart className="h-8 w-8 opacity-40" />
            <span className="text-sm">No candle data yet</span>
            <span className="text-xs text-muted-foreground/60">Waiting for the DEX candle feed</span>
          </div>
        )}
      </div>
    </div>
  );
};
