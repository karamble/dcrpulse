// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo, useState } from 'react';
import { ChevronRight, Search } from 'lucide-react';
import type { DexMarket, MMCexStatus } from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { CexIcon } from './CexIcon';
import { convRate, fmtPrice } from './dexFormat';
import { cexesSupportingMarket } from './dexMMConfig';

// DexMMMarketSelector is step 1 of the bot setup wizard: pick the DEX market to
// market make on. Mirrors the v1.0.6 market table; each row is annotated with
// the logos of CEXes that can arbitrage that pair, and clicking a row advances.
export const DexMMMarketSelector = ({
  markets,
  cexes,
  onSelect,
}: {
  markets: DexMarket[];
  cexes: Record<string, MMCexStatus>;
  onSelect: (m: DexMarket) => void;
}) => {
  const [filter, setFilter] = useState('');

  const shown = useMemo(() => {
    const f = filter.trim().toLowerCase();
    const list = f
      ? markets.filter((m) => `${m.base}/${m.quote}`.toLowerCase().includes(f))
      : markets;
    return [...list].sort((a, b) => (b.spot?.vol24 ?? 0) - (a.spot?.vol24 ?? 0));
  }, [markets, filter]);

  return (
    <div className="space-y-3 max-w-2xl">
      <div className="text-sm text-muted-foreground">Choose a market to market make on.</div>

      <div className="relative">
        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter markets"
          className="w-full pl-8 pr-3 py-1.5 rounded-lg bg-background border border-border text-sm focus:outline-none focus:border-primary"
        />
      </div>

      {shown.length === 0 ? (
        <div className="px-4 py-8 text-sm text-muted-foreground text-center rounded-lg border border-border/50">
          No markets {markets.length ? 'match the filter' : 'available'}.
        </div>
      ) : (
        <div className="rounded-xl border border-border/60 divide-y divide-border/40 overflow-hidden">
          {shown.map((m) => {
            const arbs = cexesSupportingMarket(cexes, m.baseID, m.quoteID);
            return (
              <button
                key={`${m.baseID}-${m.quoteID}`}
                type="button"
                onClick={() => onSelect(m)}
                className="w-full flex items-center justify-between gap-3 p-3 text-left hover:bg-muted/10"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <span className="flex -space-x-1.5 shrink-0">
                    <CoinIcon symbol={m.base} className="h-5 w-5 ring-1 ring-card" />
                    <CoinIcon symbol={m.quote} className="h-5 w-5 ring-1 ring-card" />
                  </span>
                  <span className="font-medium">
                    {m.base}/{m.quote}
                  </span>
                  {arbs.length > 0 && (
                    <span className="flex items-center gap-1 ml-1" title={`Arbitrage: ${arbs.join(', ')}`}>
                      {arbs.map((c) => (
                        <CexIcon key={c} name={c} className="h-3.5 w-3.5" />
                      ))}
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-3 shrink-0">
                  {m.spot && m.spot.rate > 0 && (
                    <span className="font-mono text-sm text-muted-foreground">
                      {fmtPrice(convRate(m.spot.rate, m.baseConvFactor, m.quoteConvFactor), m.quote)} {m.quote}
                    </span>
                  )}
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                </div>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
};
