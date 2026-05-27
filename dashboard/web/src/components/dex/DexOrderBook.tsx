// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { ArrowDown, ArrowUp } from 'lucide-react';
import { fmtAmt, fmtPrice } from './dexFormat';
import type { DexMarket } from '../../services/dcrdexApi';
import type { MiniOrder, OrderBookState } from './useDexFeed';
import { DexDepthChart } from './DexDepthChart';

const TABS = [
  { id: 'book', label: 'Order book' },
  { id: 'depth', label: 'Depth' },
  { id: 'trades', label: 'Trades' },
] as const;
type BookTab = (typeof TABS)[number]['id'];

interface Props {
  market: DexMarket;
  book: OrderBookState;
  onPickPrice?: (rate: number) => void;
}

const ROWS = 12;

// withCumulative annotates each level with the running depth total (in base
// units) accumulated from the best price outward.
const withCumulative = (orders: MiniOrder[]) => {
  let cum = 0;
  return orders.slice(0, ROWS).map((o) => ({ ...o, total: (cum += o.qty) }));
};

// OrderRow renders one book level. It briefly flashes when the level's size
// changes or when it first appears after the initial book load (live), so the
// user notices live updates. It is a stable top-level component so React keeps
// each row instance across updates (keyed by order token); the flash is
// re-triggered by remounting a keyed overlay whose animation runs once.
const OrderRow = ({
  o,
  total,
  max,
  sell,
  quote,
  live,
  onPick,
}: {
  o: MiniOrder;
  total: number;
  max: number;
  sell: boolean;
  quote: string;
  live: boolean;
  onPick?: (rate: number) => void;
}) => {
  const [nonce, setNonce] = useState(0);
  const prevQty = useRef<number | null>(null);

  useEffect(() => {
    const first = prevQty.current === null;
    const changed = !first && prevQty.current !== o.qty;
    prevQty.current = o.qty;
    if ((first && live) || changed) setNonce((n) => n + 1);
  }, [o.qty, live]);

  return (
    <button
      type="button"
      onClick={() => onPick?.(o.rate)}
      className="relative grid w-full grid-cols-3 px-3 py-[3px] font-mono tabular-nums text-[11px] text-left hover:bg-muted/20"
    >
      {nonce > 0 && (
        <span
          key={nonce}
          className={`absolute inset-0 pointer-events-none ${sell ? 'animate-flash-sell' : 'animate-flash-buy'}`}
        />
      )}
      <span
        className={`absolute inset-y-0 right-0 ${sell ? 'bg-destructive/15' : 'bg-success/15'}`}
        style={{ width: `${(total / max) * 100}%` }}
      />
      <span className={`relative ${sell ? 'text-destructive' : 'text-success'}`}>{fmtPrice(o.rate, quote)}</span>
      <span className="relative text-right text-muted-foreground">{fmtAmt(o.qty, 2)}</span>
      <span className="relative text-right text-muted-foreground/70">{fmtAmt(total, 2)}</span>
    </button>
  );
};

export const DexOrderBook = ({ market, book, onPickPrice }: Props) => {
  const [tab, setTab] = useState<BookTab>('book');

  // Track whether the book for the current market has loaded, so the initial
  // snapshot does not flash every row; only changes/additions afterwards do.
  const marketKey = `${market.baseID}-${market.quoteID}`;
  const [liveKey, setLiveKey] = useState<string | null>(null);
  useEffect(() => {
    if (book.buys.length || book.sells.length) setLiveKey(marketKey);
  }, [book, marketKey]);
  const live = liveKey === marketKey;

  const asks = withCumulative(book.sells); // best ask first
  const bids = withCumulative(book.buys); // best bid first
  const maxA = asks.length ? asks[asks.length - 1].total : 1;
  const maxB = bids.length ? bids[bids.length - 1].total : 1;

  const bestAsk = book.sells[0]?.rate ?? 0;
  const bestBid = book.buys[0]?.rate ?? 0;
  const mid = bestAsk && bestBid ? (bestAsk + bestBid) / 2 : bestAsk || bestBid;
  const spread = bestAsk && bestBid ? bestAsk - bestBid : 0;
  const spreadPct = mid ? (spread / mid) * 100 : 0;

  return (
    <div className="flex flex-col min-h-0 h-full">
      <div className="flex items-center border-b border-border/50 text-[11px] uppercase tracking-wider">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            className={`relative px-4 py-2.5 font-medium transition-colors ${
              tab === t.id ? 'text-foreground after:absolute after:inset-x-3 after:-bottom-px after:h-0.5 after:bg-primary' : 'text-muted-foreground hover:text-foreground/80'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'depth' && <DexDepthChart market={market} book={book} />}

      {tab === 'book' ? (
        <div className="flex flex-col min-h-0 flex-1">
          <div className="grid grid-cols-3 px-3 py-1.5 text-[10px] uppercase tracking-wider text-muted-foreground/60">
            <span>Price ({market.quote.split('.')[0]})</span>
            <span className="text-right">Size ({market.base})</span>
            <span className="text-right">Total</span>
          </div>

          <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
            <div className="flex-1 flex flex-col justify-end overflow-hidden">
              {asks
                .slice()
                .reverse()
                .map((o) => (
                  <OrderRow key={o.token} o={o} total={o.total} max={maxA} sell quote={market.quote} live={live} onPick={onPickPrice} />
                ))}
            </div>

            <div className="flex items-center justify-between px-3 py-1.5 bg-muted/15 border-y border-border/50 text-[11px]">
              <span className="font-mono tabular-nums text-sm font-semibold text-success flex items-center gap-1">
                {spread >= 0 ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />}
                {fmtPrice(mid, market.quote)}
              </span>
              <span className="font-mono tabular-nums text-muted-foreground">
                <span className="text-muted-foreground/60 mr-1">Spread</span>
                {fmtPrice(spread, market.quote)} · {spreadPct.toFixed(3)}%
              </span>
            </div>

            <div className="flex-1 flex flex-col overflow-hidden">
              {bids.map((o) => (
                <OrderRow key={o.token} o={o} total={o.total} max={maxB} sell={false} quote={market.quote} live={live} onPick={onPickPrice} />
              ))}
            </div>
          </div>
        </div>
      ) : tab === 'trades' ? (
        <div className="flex flex-col min-h-0 flex-1">
          <div className="grid grid-cols-3 px-3 py-1.5 text-[10px] uppercase tracking-wider text-muted-foreground/60">
            <span>Price ({market.quote.split('.')[0]})</span>
            <span className="text-right">Size ({market.base})</span>
            <span className="text-right">Time</span>
          </div>
          <div className="flex-1 overflow-y-auto">
            {book.recentMatches.length === 0 && (
              <div className="px-3 py-4 text-xs text-muted-foreground">No recent trades</div>
            )}
            {book.recentMatches.map((t, i) => (
              <div key={i} className="grid grid-cols-3 px-3 py-[3px] font-mono tabular-nums text-[11px]">
                <span className={t.sell ? 'text-destructive' : 'text-success'}>{fmtPrice(t.rate, market.quote)}</span>
                <span className="text-right text-muted-foreground">{fmtAmt(t.qty, 2)}</span>
                <span className="text-right text-muted-foreground/70">
                  {new Date(t.stamp).toLocaleTimeString('en-US', { hour12: false })}
                </span>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
};
