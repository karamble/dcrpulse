// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Activity, TrendingDown, TrendingUp } from 'lucide-react';
import type { MMBotStatus } from '../../services/dcrdexApi';
import { fmtUsd } from './dexFormat';

// elapsed renders a HH:MM:SS run time from a start stamp that may be in seconds
// or milliseconds (bisonw reports seconds).
const elapsed = (startStamp: number): string => {
  const startMs = startStamp > 1e12 ? startStamp : startStamp * 1000;
  const s = Math.max(0, Math.floor((Date.now() - startMs) / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const pad = (n: number) => n.toString().padStart(2, '0');
  return `${pad(h)}:${pad(m)}:${pad(sec)}`;
};

const Metric = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <div className="flex flex-col gap-0.5">
    <span className="text-[10px] uppercase tracking-wider text-muted-foreground/60">{label}</span>
    <span className="text-sm font-mono tabular-nums">{children}</span>
  </div>
);

// DexMMActivity renders a running market-maker bot's live stats. It is shown in
// the Market Maker tab and, compactly, in the trade view's right sidebar when a
// bot is running on the selected market. Stats come from the shared MM status,
// which the live provider refetches on each runstats/epoch notification.
export const DexMMActivity = ({ bot, compact = false }: { bot: MMBotStatus; compact?: boolean }) => {
  const [, tick] = useState(0);
  const stats = bot.runStats;

  // Re-render once a second so the run timer advances while a bot is running.
  useEffect(() => {
    if (!bot.running || !stats) return;
    const id = window.setInterval(() => tick((n) => n + 1), 1000);
    return () => window.clearInterval(id);
  }, [bot.running, stats]);

  if (!bot.running || !stats) {
    return (
      <div className="flex items-center gap-2 px-3 py-4 text-xs text-muted-foreground">
        <Activity className="h-4 w-4 shrink-0" />
        Bot is not running.
      </div>
    );
  }

  const pl = stats.profitLoss;
  const up = (pl?.profit ?? 0) >= 0;
  const plCls = up ? 'text-success' : 'text-destructive';
  const feeGapPct =
    stats.feeGap && stats.feeGap.basisPrice > 0 ? (stats.feeGap.feeGap / stats.feeGap.basisPrice) * 100 : null;
  const epoch = bot.latestEpoch;

  return (
    <div className="space-y-3 p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="h-1.5 w-1.5 rounded-full bg-success animate-pulse" />
          Running
        </div>
        <span className="font-mono tabular-nums text-xs text-muted-foreground">{elapsed(stats.startTime)}</span>
      </div>

      <div className="rounded-lg bg-gradient-card border border-border/50 p-3">
        <div className="text-[10px] uppercase tracking-wider text-muted-foreground/60 mb-1">Profit / loss</div>
        <div className={`flex items-baseline gap-2 ${plCls}`}>
          {up ? <TrendingUp className="h-4 w-4" /> : <TrendingDown className="h-4 w-4" />}
          <span className="text-xl font-mono tabular-nums">{fmtUsd(pl?.profit ?? 0)}</span>
          <span className="text-sm font-mono tabular-nums">
            {((pl?.profitRatio ?? 0) * 100).toFixed(2)}%
          </span>
        </div>
      </div>

      <div className={`grid gap-3 ${compact ? 'grid-cols-2' : 'grid-cols-3'}`}>
        <Metric label="Matches">{stats.completedMatches}</Metric>
        <Metric label="Traded">{fmtUsd(stats.tradedUSD)}</Metric>
        {feeGapPct !== null && <Metric label="Fee gap">{feeGapPct.toFixed(2)}%</Metric>}
        {(stats.pendingDeposits > 0 || stats.pendingWithdrawals > 0) && (
          <Metric label="Pending">
            {stats.pendingDeposits + stats.pendingWithdrawals}
          </Metric>
        )}
        {epoch && <Metric label="Epoch">{epoch.epochNum}</Metric>}
      </div>

      {epoch && (
        <div className="flex gap-2 text-[11px]">
          <span
            className={`px-1.5 py-0.5 rounded ${
              epoch.buysReport ? 'bg-success/10 text-success' : 'bg-muted/40 text-muted-foreground'
            }`}
          >
            Buys {epoch.buysReport ? 'ok' : 'idle'}
          </span>
          <span
            className={`px-1.5 py-0.5 rounded ${
              epoch.sellsReport ? 'bg-success/10 text-success' : 'bg-muted/40 text-muted-foreground'
            }`}
          >
            Sells {epoch.sellsReport ? 'ok' : 'idle'}
          </span>
        </div>
      )}
    </div>
  );
};
