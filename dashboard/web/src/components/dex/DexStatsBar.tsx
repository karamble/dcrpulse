// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { ReactNode } from 'react';
import { CoinIcon } from './CoinIcon';
import { fmtAmt, fmtPct, fmtPrice, fmtUsd } from './dexFormat';
import type { DexMarket } from '../../services/dcrdexApi';
import type { MarketStats } from './useDexFeed';

interface Props {
  market: DexMarket;
  stats: MarketStats | null;
  connected: boolean;
  preview?: boolean;
}

const Stat = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex flex-col justify-center px-4 lg:px-5 border-l border-border/50 min-w-max">
    <div className="text-[10px] uppercase tracking-[0.12em] text-muted-foreground/70 mb-0.5">{label}</div>
    <div className="text-sm font-mono tabular-nums">{children}</div>
  </div>
);

// DexStatsBar is the market context strip: pair, connection state, and the 24h
// price summary. Stats are absent (null) in the live flow until a stats source
// is wired; the bar then shows the pair and connection only.
export const DexStatsBar = ({ market, stats, connected, preview }: Props) => {
  const up = (stats?.changePct ?? 0) >= 0;
  const dirCls = up ? 'text-success' : 'text-destructive';
  return (
    <div className="flex items-stretch overflow-x-auto rounded-xl bg-card border border-border/60">
      <div className="flex items-center gap-3 px-4 py-2.5 border-r border-border/50 min-w-max">
        <span className="flex -space-x-1.5">
          <CoinIcon symbol={market.base} className="h-6 w-6 ring-1 ring-card" />
          <CoinIcon symbol={market.quote} className="h-6 w-6 ring-1 ring-card" />
        </span>
        <div>
          <div className="font-semibold leading-tight">
            {market.base}
            <span className="text-muted-foreground/50 font-normal">/</span>
            {market.quote}
          </div>
          <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
            <span className={`h-1.5 w-1.5 rounded-full ${connected ? 'bg-success' : 'bg-muted-foreground/50'}`} />
            {preview ? 'sample data' : connected ? 'live' : 'connecting'}
          </div>
        </div>
      </div>

      {stats && (
        <>
          <Stat label="Last price">
            <span className={dirCls}>{fmtPrice(stats.last, market.quote)}</span>
            {stats.lastUsd !== undefined && (
              <span className="ml-1.5 text-[11px] font-normal text-muted-foreground">≈ {fmtUsd(stats.lastUsd)}</span>
            )}
          </Stat>
          <Stat label="24h change">
            <span className={dirCls}>
              {fmtPct(stats.changePct)}
            </span>
          </Stat>
          <Stat label="24h high">{fmtPrice(stats.high24, market.quote)}</Stat>
          <Stat label="24h low">{fmtPrice(stats.low24, market.quote)}</Stat>
          <Stat label={`24h vol (${market.base})`}>{fmtAmt(stats.volBase, 0)}</Stat>
          <Stat label={`24h vol (${market.quote})`}>{fmtAmt(stats.volQuote, 2)}</Stat>
        </>
      )}
    </div>
  );
};
