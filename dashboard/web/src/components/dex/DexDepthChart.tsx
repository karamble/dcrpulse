// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo } from 'react';
import { fmtAmt, fmtPrice } from './dexFormat';
import type { DexMarket } from '../../services/dcrdexApi';
import type { OrderBookState } from './useDexFeed';

const BID = 'hsl(142 76% 40%)';
const ASK = 'hsl(0 72% 55%)';
const AXIS = 'hsl(215 20% 55%)';

// DexDepthChart renders a cumulative market-depth chart from the order book:
// cumulative bid volume rising toward the mid from the left (green) and ask
// volume from the right (red). Dependency-free SVG, like the candle chart.
export const DexDepthChart = ({ market, book }: { market: DexMarket; book: OrderBookState }) => {
  const chart = useMemo(() => {
    const bids = [...book.buys].sort((a, b) => b.rate - a.rate); // best (highest) first
    const asks = [...book.sells].sort((a, b) => a.rate - b.rate); // best (lowest) first
    if (bids.length === 0 && asks.length === 0) return null;

    let cb = 0;
    const bidPts = bids.map((o) => ({ rate: o.rate, cum: (cb += o.qty) }));
    let ca = 0;
    const askPts = asks.map((o) => ({ rate: o.rate, cum: (ca += o.qty) }));

    const bestBid = bids[0]?.rate ?? asks[0]?.rate;
    const bestAsk = asks[0]?.rate ?? bids[0]?.rate;
    const mid = (bestBid + bestAsk) / 2;
    const xMin = bidPts.length ? bidPts[bidPts.length - 1].rate : mid;
    const xMax = askPts.length ? askPts[askPts.length - 1].rate : mid;
    const yMax = Math.max(bidPts[bidPts.length - 1]?.cum ?? 0, askPts[askPts.length - 1]?.cum ?? 0) || 1;
    if (xMax <= xMin) return null;

    const W = 1000;
    const H = 300;
    const padR = 4;
    const padT = 8;
    const padB = 20;
    const cw = W - padR;
    const chh = H - padT - padB;
    // Split the width at the mid so the two sides always meet in the centre:
    // bids map to the left half [xMin, mid], asks to the right half [mid, xMax].
    // A single [xMin, xMax] scale pushes the mid off-centre whenever one side
    // spans a wider price range than the other (squashing the narrower side).
    const half = cw / 2;
    const X = (r: number) =>
      r <= mid ? ((r - xMin) / (mid - xMin || 1)) * half : half + ((r - mid) / (xMax - mid || 1)) * half;
    const Y = (c: number) => padT + (1 - c / yMax) * chh;
    const baseline = padT + chh;

    const area = (pts: { rate: number; cum: number }[]) => {
      const asc = [...pts].sort((a, b) => a.rate - b.rate);
      const head = `M ${X(asc[0].rate).toFixed(1)} ${baseline.toFixed(1)}`;
      const body = asc.map((p) => `L ${X(p.rate).toFixed(1)} ${Y(p.cum).toFixed(1)}`).join(' ');
      return `${head} ${body} L ${X(asc[asc.length - 1].rate).toFixed(1)} ${baseline.toFixed(1)} Z`;
    };

    return {
      W,
      H,
      padT,
      baseline,
      midX: X(mid),
      bidPath: bidPts.length ? area(bidPts) : '',
      askPath: askPts.length ? area(askPts) : '',
      mid,
      xMin,
      xMax,
      yMax,
    };
  }, [book]);

  if (!chart) {
    return <div className="flex-1 flex items-center justify-center text-xs text-muted-foreground">No depth data</div>;
  }

  const { W, H, padT, baseline, midX, bidPath, askPath, mid, xMin, xMax, yMax } = chart;
  return (
    <div className="flex-1 min-h-0 relative">
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="w-full h-full">
        {bidPath && <path d={bidPath} fill={BID} fillOpacity={0.18} stroke={BID} strokeWidth={1.5} />}
        {askPath && <path d={askPath} fill={ASK} fillOpacity={0.18} stroke={ASK} strokeWidth={1.5} />}
        <line x1={midX} y1={padT} x2={midX} y2={baseline} stroke={AXIS} strokeWidth={1} strokeDasharray="3 3" />
      </svg>
      {/* HTML overlays avoid the SVG text distortion from preserveAspectRatio=none */}
      <div className="pointer-events-none absolute inset-0 font-mono text-[10px] text-muted-foreground">
        <span className="absolute top-1 left-1">{fmtAmt(yMax, 2)} {market.base}</span>
        <span className="absolute bottom-0.5 left-1 text-success">{fmtPrice(xMin, market.quote)}</span>
        <span
          className="absolute bottom-0.5 -translate-x-1/2 text-foreground"
          style={{ left: `${(midX / W) * 100}%` }}
        >
          {fmtPrice(mid, market.quote)}
        </span>
        <span className="absolute bottom-0.5 right-1 text-destructive">{fmtPrice(xMax, market.quote)}</span>
      </div>
    </div>
  );
};
