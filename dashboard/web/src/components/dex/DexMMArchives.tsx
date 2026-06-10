// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, ChevronLeft, ChevronRight, History } from 'lucide-react';
import { getMMArchivedRuns, type DexMarket, type MMArchivedRun } from '../../services/dcrdexApi';
import { fmtUsd } from './dexFormat';
import { DexMMRunLogs } from './DexMMRunLogs';

type AssetInfo = (assetID: number) => { symbol: string; convFactor: number };

// DexMMArchives lists past market-maker runs (newest first) with their market
// and realized profit, plus the cumulative profit across all runs. A row opens
// that run's event log in the shared run-logs view. Mirrors bisonw's mmarchives.
export const DexMMArchives = ({
  markets,
  assetOf,
  onBack,
}: {
  markets: DexMarket[];
  assetOf: AssetInfo;
  onBack: () => void;
}) => {
  const [runs, setRuns] = useState<MMArchivedRun[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [selected, setSelected] = useState<MMArchivedRun | null>(null);

  useEffect(() => {
    getMMArchivedRuns()
      .then((r) => {
        setRuns([...r].sort((a, b) => b.startTime - a.startTime));
        setErr(null);
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Failed to load run history'));
  }, []);

  const marketOf = (r: MMArchivedRun) =>
    markets.find((mk) => mk.baseID === r.market.baseID && mk.quoteID === r.market.quoteID);
  const pairLabel = (r: MMArchivedRun) => {
    const mk = marketOf(r);
    if (mk) return `${mk.base}/${mk.quote.split('.')[0]}`;
    return `${assetOf(r.market.baseID).symbol}/${assetOf(r.market.quoteID).symbol}`;
  };
  // bisonw v1.0.6 omits profit from this list; only total when some run has it.
  const withProfit = (runs ?? []).filter((r) => typeof r.profit === 'number');
  const total = withProfit.reduce((s, r) => s + (r.profit ?? 0), 0);

  return (
    <div className="px-3 lg:px-4 space-y-3">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1 text-sm px-2.5 py-1.5 rounded-lg border border-border hover:bg-muted/10"
        >
          <ChevronLeft className="h-4 w-4" /> Back
        </button>
        <h3 className="font-semibold flex items-center gap-2">
          <History className="h-4 w-4 text-primary" />
          Run history
        </h3>
        {withProfit.length > 0 && (
          <span className="ml-auto text-xs text-muted-foreground">
            Total{' '}
            <span className={`font-mono tabular-nums ${total >= 0 ? 'text-success' : 'text-destructive'}`}>{fmtUsd(total)}</span>
          </span>
        )}
      </div>

      {err && (
        <div className="p-3 rounded-lg bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      {!err && runs === null && (
        <div className="min-h-[20vh] flex items-center justify-center">
          <div className="animate-spin rounded-full h-7 w-7 border-b-2 border-primary" />
        </div>
      )}

      {runs && runs.length === 0 && !err && (
        <div className="px-4 py-8 text-sm text-muted-foreground rounded-xl border border-border/50">No past runs yet.</div>
      )}

      {runs && runs.length > 0 && (
        <div className="rounded-xl border border-border/50 overflow-hidden divide-y divide-border/40">
          {runs.map((r) => (
            <button
              key={`${r.market.host}-${r.market.baseID}-${r.market.quoteID}-${r.startTime}`}
              type="button"
              onClick={() => setSelected(r)}
              className="w-full flex items-center gap-3 px-4 py-2.5 text-sm hover:bg-muted/10 transition-colors text-left"
            >
              <span className="font-mono tabular-nums">{pairLabel(r)}</span>
              <span className="text-xs text-muted-foreground">{new Date((r.startTime || 0) * 1000).toLocaleString()}</span>
              {typeof r.profit === 'number' ? (
                <span className={`ml-auto font-mono tabular-nums ${r.profit >= 0 ? 'text-success' : 'text-destructive'}`}>{fmtUsd(r.profit)}</span>
              ) : (
                <span className="ml-auto text-xs text-muted-foreground/60">View P/L</span>
              )}
              <ChevronRight className="h-4 w-4 text-muted-foreground shrink-0" />
            </button>
          ))}
        </div>
      )}

      {selected && (
        <DexMMRunLogs
          host={selected.market.host}
          baseID={selected.market.baseID}
          quoteID={selected.market.quoteID}
          startTime={selected.startTime}
          running={false}
          profitFallback={selected.profit}
          market={marketOf(selected)}
          assetOf={assetOf}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
};
