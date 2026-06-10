// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Activity, AlertTriangle, TrendingDown, TrendingUp } from 'lucide-react';
import type { DexMarket, MMBotStatus, MMOrderReport } from '../../services/dcrdexApi';
import { fmtAmt, fmtUsd } from './dexFormat';
import { botProblemMessages, cexProblemMessages, placedCount } from './dexMMProblems';
import { DexMMOrderReport } from './DexMMOrderReport';

// AssetInfo resolves an asset id to a display ticker and its atoms-per-unit
// conversion factor.
export type AssetInfo = (assetID: number) => { symbol: string; convFactor: number };
const DEFAULT_ASSET: AssetInfo = (id) => ({ symbol: `#${id}`, convFactor: 1e8 });

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

// SideBadge shows how many of a side's placements have their target lots on the
// book this epoch (green when all placed, amber when some fell short); clickable
// to open the detailed order report.
const SideBadge = ({
  label,
  placed,
  total,
  onClick,
}: {
  label: string;
  placed: number;
  total: number;
  onClick?: () => void;
}) => {
  const cls =
    total === 0
      ? 'bg-muted/40 text-muted-foreground'
      : placed >= total
        ? 'bg-success/10 text-success'
        : 'bg-warning/10 text-warning';
  const body = `${label} ${placed}/${total}`;
  return onClick ? (
    <button type="button" onClick={onClick} className={`px-1.5 py-0.5 rounded hover:underline ${cls}`}>
      {body}
    </button>
  ) : (
    <span className={`px-1.5 py-0.5 rounded ${cls}`}>{body}</span>
  );
};

// DexMMActivity renders a running market-maker bot's live stats: profit/loss,
// match/volume metrics, per-asset inventory, per-epoch placed/failed counts (open
// the detailed order report), and any problems explaining why orders were not
// placed. Shown in the Market Maker tab and, compactly, in the trade view's right
// sidebar. Data comes from the shared MM status, refetched on each MM note.
export const DexMMActivity = ({
  bot,
  compact = false,
  market,
  assetOf = DEFAULT_ASSET,
}: {
  bot: MMBotStatus;
  compact?: boolean;
  market?: DexMarket;
  assetOf?: AssetInfo;
}) => {
  const [, tick] = useState(0);
  const [reportModal, setReportModal] = useState<{ report: MMOrderReport; side: string } | null>(null);
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

  const symbolOf = (id: number) => assetOf(id).symbol;
  const pl = stats.profitLoss;
  const up = (pl?.profit ?? 0) >= 0;
  const plCls = up ? 'text-success' : 'text-destructive';
  const feeGapPct =
    stats.feeGap && stats.feeGap.basisPrice > 0 ? (stats.feeGap.feeGap / stats.feeGap.basisPrice) * 100 : null;
  const epoch = bot.latestEpoch;
  const problemMsgs = [
    ...botProblemMessages(epoch?.preOrderProblems, {
      cexName: bot.config.cexName,
      dexHost: bot.config.host,
      symbolOf,
    }),
    ...cexProblemMessages(bot.cexProblems, symbolOf),
  ];
  const buys = placedCount(epoch?.buysReport);
  const sells = placedCount(epoch?.sellsReport);
  const dexBalances = Object.entries(stats.dexBalances ?? {});

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
          <span className="text-sm font-mono tabular-nums">{((pl?.profitRatio ?? 0) * 100).toFixed(2)}%</span>
        </div>
      </div>

      <div className={`grid gap-3 ${compact ? 'grid-cols-2' : 'grid-cols-3'}`}>
        <Metric label="Matches">{stats.completedMatches}</Metric>
        <Metric label="Traded">{fmtUsd(stats.tradedUSD)}</Metric>
        {feeGapPct !== null && <Metric label="Fee gap">{feeGapPct.toFixed(2)}%</Metric>}
        {(stats.pendingDeposits > 0 || stats.pendingWithdrawals > 0) && (
          <Metric label="Pending">{stats.pendingDeposits + stats.pendingWithdrawals}</Metric>
        )}
        {epoch && <Metric label="Epoch">{epoch.epochNum}</Metric>}
      </div>

      {dexBalances.length > 0 && (
        <div className="space-y-1">
          <div className="text-[10px] uppercase tracking-wider text-muted-foreground/60">Inventory</div>
          <div className="space-y-0.5 text-[11px] font-mono tabular-nums">
            {dexBalances.map(([id, bal]) => {
              const { symbol, convFactor } = assetOf(Number(id));
              const conv = (v: number) => fmtAmt(v / (convFactor || 1), 4);
              const held = bal.locked + bal.pending + bal.reserved;
              return (
                <div key={id} className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">{symbol}</span>
                  <span>
                    {conv(bal.available)}
                    {held > 0 && <span className="text-muted-foreground/60"> (+{conv(held)} held)</span>}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {(buys || sells) && (
        <div className="flex gap-2 text-[11px]">
          {buys && (
            <SideBadge
              label="Buys"
              placed={buys.placed}
              total={buys.total}
              onClick={epoch?.buysReport ? () => setReportModal({ report: epoch.buysReport!, side: 'Buy' }) : undefined}
            />
          )}
          {sells && (
            <SideBadge
              label="Sells"
              placed={sells.placed}
              total={sells.total}
              onClick={epoch?.sellsReport ? () => setReportModal({ report: epoch.sellsReport!, side: 'Sell' }) : undefined}
            />
          )}
        </div>
      )}

      {problemMsgs.length > 0 && (
        <div className="space-y-1 rounded-lg bg-warning/10 border border-warning/30 p-2">
          {problemMsgs.map((m, i) => (
            <div key={i} className="flex items-start gap-1.5 text-[11px] text-warning">
              <AlertTriangle className="h-3 w-3 mt-0.5 shrink-0" />
              <span className="break-words">{m}</span>
            </div>
          ))}
        </div>
      )}

      {reportModal && (
        <DexMMOrderReport
          report={reportModal.report}
          side={reportModal.side}
          market={market}
          assetOf={assetOf}
          onClose={() => setReportModal(null)}
        />
      )}
    </div>
  );
};
