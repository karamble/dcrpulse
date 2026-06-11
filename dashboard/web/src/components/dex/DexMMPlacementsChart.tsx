// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo } from 'react';
import type { DexMarket, MMGapStrategy, MMMarketReport } from '../../services/dcrdexApi';
import { fmtPrice } from './dexFormat';
import type { BotType, PlacementRow } from './dexMMConfig';

const BID = 'hsl(142 76% 40%)';
const ASK = 'hsl(0 72% 55%)';
const AXIS = 'hsl(215 20% 55%)';

// percentStrategy reports whether the gap factor is a direct price fraction, in
// which case the chart can place levels at their true distance from mid.
const percentStrategy = (s: MMGapStrategy): boolean => s === 'percent' || s === 'percent-plus';

interface Level {
  lots: number;
  factor: number;
}

const parse = (rows: PlacementRow[]): Level[] =>
  rows
    .map((p) => ({ lots: Math.max(0, Math.floor(Number(p.lots) || 0)), factor: Number(p.factor) || 0 }))
    .filter((p) => p.lots > 0);

// DexMMPlacementsChart previews where the bot's orders sit relative to the mid
// price: buy levels to the left (green), sell levels to the right (red), bar
// height proportional to lots. For percent gap strategies the x distance is the
// true price offset; otherwise levels are spaced by rank. Mirrors the v1.0.6
// placements chart.
export const DexMMPlacementsChart = ({
  market,
  botType,
  gapStrategy,
  buys,
  sells,
  report,
}: {
  market: DexMarket;
  botType: BotType;
  gapStrategy: MMGapStrategy;
  buys: PlacementRow[];
  sells: PlacementRow[];
  report: MMMarketReport | null;
}) => {
  const chart = useMemo(() => {
    const b = parse(buys);
    const s = parse(sells);
    if (b.length === 0 && s.length === 0) return null;

    // True fractional offsets only for percent-based basic MM; otherwise rank.
    const useFraction = botType === 'basicmm' && percentStrategy(gapStrategy);
    const offsetOf = (lvl: Level, idx: number): number => (useFraction ? lvl.factor : idx + 1);

    const all = [...b, ...s];
    const maxLots = Math.max(...all.map((l) => l.lots), 1);
    const maxOff = Math.max(...b.map(offsetOf), ...s.map(offsetOf), useFraction ? 0.0001 : 1);

    const W = 1000;
    const H = 200;
    const padX = 8;
    const padT = 10;
    const padB = 22;
    const mid = W / 2;
    const half = mid - padX;
    const chh = H - padT - padB;
    const baseline = padT + chh;
    const barW = Math.min(36, (half / Math.max(b.length, s.length, 1)) * 0.5);

    const bar = (lvl: Level, idx: number, side: 'buy' | 'sell') => {
      const off = offsetOf(lvl, idx) / maxOff;
      const dx = off * (half - barW);
      const x = side === 'buy' ? mid - dx - barW : mid + dx;
      const h = Math.max(2, (lvl.lots / maxLots) * chh);
      return { x, y: baseline - h, h, w: barW };
    };

    return {
      W,
      H,
      mid,
      padT,
      baseline,
      useFraction,
      buyBars: b.map((l, i) => bar(l, i, 'buy')),
      sellBars: s.map((l, i) => bar(l, i, 'sell')),
      maxFrac: useFraction ? maxOff : 0,
    };
  }, [buys, sells, botType, gapStrategy]);

  if (!chart) {
    return (
      <div className="rounded-lg border border-border/50 bg-background/40 h-28 flex items-center justify-center text-xs text-muted-foreground">
        Add a placement to preview the order layout.
      </div>
    );
  }

  const { W, H, mid, padT, baseline, useFraction, buyBars, sellBars, maxFrac } = chart;
  const price = report?.price ?? 0;
  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-2">
      <div className="relative">
        <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="w-full h-28">
          {buyBars.map((r, i) => (
            <rect key={`b${i}`} x={r.x} y={r.y} width={r.w} height={r.h} fill={BID} fillOpacity={0.6} />
          ))}
          {sellBars.map((r, i) => (
            <rect key={`s${i}`} x={r.x} y={r.y} width={r.w} height={r.h} fill={ASK} fillOpacity={0.6} />
          ))}
          <line x1={mid} y1={padT} x2={mid} y2={baseline} stroke={AXIS} strokeWidth={1} strokeDasharray="3 3" />
        </svg>
        <div className="pointer-events-none absolute inset-0 font-mono text-[10px] text-muted-foreground">
          <span className="absolute bottom-0 left-1/2 -translate-x-1/2 text-foreground">
            {price > 0 ? `${fmtPrice(price, market.quote)} mid` : 'mid'}
          </span>
          <span className="absolute top-0 left-1 text-success">buys</span>
          <span className="absolute top-0 right-1 text-destructive">sells</span>
          {useFraction && maxFrac > 0 && (
            <>
              <span className="absolute bottom-0 left-1">-{(maxFrac * 100).toFixed(2)}%</span>
              <span className="absolute bottom-0 right-1">+{(maxFrac * 100).toFixed(2)}%</span>
            </>
          )}
        </div>
      </div>
      {!useFraction && (
        <div className="text-[10px] text-muted-foreground mt-1">
          {botType === 'arbmm'
            ? 'Bars ranked by multiplier (depth into the CEX book), height by lots.'
            : 'Bars ranked by level, height by lots.'}
        </div>
      )}
    </div>
  );
};
