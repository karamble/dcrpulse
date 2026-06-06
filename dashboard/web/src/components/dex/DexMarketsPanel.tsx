// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { Search } from 'lucide-react';
import { CoinIcon } from './CoinIcon';
import { fmtPct, fmtPrice } from './dexFormat';
import type { DexMarket } from '../../services/dcrdexApi';
import type { MarketStats } from './useDexFeed';

interface Props {
  markets: DexMarket[];
  selected: DexMarket | null;
  onSelect: (m: DexMarket) => void;
  statsFor: (m: DexMarket) => MarketStats | null;
}

const sameMarket = (a: DexMarket, b: DexMarket | null) =>
  !!b && a.baseID === b.baseID && a.quoteID === b.quoteID;

// quoteTab keys group markets by the quote asset's base symbol (USDC.POLYGON ->
// USDC) so the tab row stays short.
const quoteKey = (m: DexMarket) => m.quote.toUpperCase().split('.')[0];

// MarketRow renders one market and flashes its Last price on change, mirroring
// the order book's OrderRow: a keyed overlay remounts to replay the animation
// (green on an uptick, red on a downtick). The first-populate value does not
// flash, so seeding the whole list at once does not trigger a flash storm.
const MarketRow = ({
  m,
  st,
  active,
  onSelect,
}: {
  m: DexMarket;
  st: MarketStats | null;
  active: boolean;
  onSelect: (m: DexMarket) => void;
}) => {
  const up = (st?.changePct ?? 0) >= 0;
  const [nonce, setNonce] = useState(0);
  const [flashUp, setFlashUp] = useState(true);
  const prevLast = useRef<number | null>(null);

  useEffect(() => {
    const cur = st?.last ?? null;
    const first = prevLast.current === null;
    if (!first && cur !== null && cur !== prevLast.current) {
      setFlashUp(cur > (prevLast.current as number));
      setNonce((n) => n + 1);
    }
    prevLast.current = cur;
  }, [st?.last]);

  return (
    <button
      type="button"
      onClick={() => onSelect(m)}
      className={`grid w-full grid-cols-[1.5fr_1fr_0.7fr] items-center px-4 py-2 text-sm text-left border-l-2 transition-colors ${
        active ? 'bg-muted/20 border-primary' : 'border-transparent hover:bg-muted/10'
      }`}
    >
      <span className="flex items-center gap-2 min-w-0">
        <span className="flex -space-x-1.5 shrink-0">
          <CoinIcon symbol={m.base} className="h-4 w-4 ring-1 ring-card" />
          <CoinIcon symbol={m.quote} className="h-4 w-4 ring-1 ring-card" />
        </span>
        <span className="truncate">
          {m.base}
          <span className="text-muted-foreground/50">/{m.quote.split('.')[0]}</span>
        </span>
      </span>
      <span className="relative text-right font-mono tabular-nums text-xs">
        {nonce > 0 && (
          <span
            key={nonce}
            className={`absolute inset-0 pointer-events-none ${flashUp ? 'animate-flash-buy' : 'animate-flash-sell'}`}
          />
        )}
        <span className="relative">{st ? fmtPrice(st.last, m.quote) : '–'}</span>
      </span>
      <span
        className={`text-right font-mono tabular-nums text-xs ${st ? (up ? 'text-success' : 'text-destructive') : 'text-muted-foreground'}`}
      >
        {st ? fmtPct(st.changePct) : '–'}
      </span>
    </button>
  );
};

export const DexMarketsPanel = ({ markets, selected, onSelect, statsFor }: Props) => {
  const [query, setQuery] = useState('');
  const [tab, setTab] = useState('all');

  const tabs = useMemo(() => {
    const counts = new Map<string, number>();
    markets.forEach((m) => counts.set(quoteKey(m), (counts.get(quoteKey(m)) || 0) + 1));
    return [['all', markets.length] as const, ...Array.from(counts.entries()).sort((a, b) => b[1] - a[1])];
  }, [markets]);

  const filtered = useMemo(() => {
    const q = query.trim().toUpperCase();
    return markets.filter((m) => {
      if (tab !== 'all' && quoteKey(m) !== tab) return false;
      if (q && !`${m.base}/${m.quote}`.toUpperCase().includes(q)) return false;
      return true;
    });
  }, [markets, query, tab]);

  return (
    <div className="flex flex-col min-h-0 h-full">
      <div className="p-3 border-b border-border/50">
        <div className="flex items-center gap-2 rounded-lg bg-background border border-border/60 px-2.5 py-1.5">
          <Search className="h-3.5 w-3.5 text-muted-foreground/70 shrink-0" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search markets"
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground/50"
          />
        </div>
      </div>

      <div className="flex gap-4 px-4 py-2 border-b border-border/50 text-xs overflow-x-auto overflow-y-hidden">
        {tabs.map(([k, n]) => (
          <button
            key={k}
            type="button"
            onClick={() => setTab(k)}
            className={`relative pb-1 whitespace-nowrap font-medium transition-colors ${
              tab === k ? 'text-foreground after:absolute after:inset-x-0 after:-bottom-px after:h-0.5 after:bg-primary' : 'text-muted-foreground hover:text-foreground/80'
            }`}
          >
            {k === 'all' ? 'All' : k} <span className="text-muted-foreground/50">{n}</span>
          </button>
        ))}
      </div>

      <div className="grid grid-cols-[1.5fr_1fr_0.7fr] px-4 py-1.5 text-[10px] uppercase tracking-wider text-muted-foreground/60 border-b border-border/50">
        <span>Market</span>
        <span className="text-right">Last</span>
        <span className="text-right">24h</span>
      </div>

      <div className="flex-1 overflow-y-auto">
        {filtered.map((m) => (
          <MarketRow
            key={`${m.baseID}-${m.quoteID}`}
            m={m}
            st={statsFor(m)}
            active={sameMarket(m, selected)}
            onSelect={onSelect}
          />
        ))}
        {filtered.length === 0 && (
          <div className="px-4 py-6 text-xs text-muted-foreground text-center">No markets</div>
        )}
      </div>
    </div>
  );
};
