// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDown, ArrowUp } from 'lucide-react';
import { fmtAge, fmtAmt, fmtPrice } from './dexFormat';
import type { DexMarket } from '../../services/dcrdexApi';
import type { MiniOrder, OrderBookState } from './useDexFeed';
import { DexDepthChart } from './DexDepthChart';
import { useSecondTick } from './useSecondTick';

const TABS = [
  { id: 'book', label: 'Order book' },
  { id: 'depth', label: 'Depth' },
  { id: 'trades', label: 'Trades' },
] as const;
type BookTab = (typeof TABS)[number]['id'];

interface Props {
  market: DexMarket;
  book: OrderBookState;
  mineTokens?: Set<string>;
  onPick?: (p: { rate: number; qty: number; sell: boolean }) => void;
}

const ROWS = 12;

// EMPTY_MINE is a stable empty set so the aggregation memo is not defeated when
// the caller omits mineTokens.
const EMPTY_MINE = new Set<string>();

// BookLevel is one aggregated price level: all orders resting at a single
// message-rate, summed, with the running depth total and a flag for whether one
// of the user's own orders sits at this price.
interface BookLevel {
  rate: number;
  msgRate: number;
  qty: number;
  qtyAtomic: number;
  count: number;
  total: number;
  mine: boolean;
}

// aggregateLevels bins already-sorted orders by message-rate into one level per
// distinct price (mirroring bisonw's web book), caps to ROWS distinct levels,
// then annotates the running depth total (in base units) from the best price
// outward. mine flags a level holding one of the user's active orders, matched
// by the book token, which is the order id's first 4 bytes.
const aggregateLevels = (orders: MiniOrder[], mineTokens: Set<string>): BookLevel[] => {
  const levels: BookLevel[] = [];
  let last: BookLevel | undefined;
  for (const o of orders) {
    if (last && last.msgRate === o.msgRate) {
      last.qty += o.qty;
      last.qtyAtomic += o.qtyAtomic;
      last.count += 1;
      if (mineTokens.has(o.token)) last.mine = true;
      continue;
    }
    if (levels.length >= ROWS) break;
    last = {
      rate: o.rate,
      msgRate: o.msgRate,
      qty: o.qty,
      qtyAtomic: o.qtyAtomic,
      count: 1,
      total: 0,
      mine: mineTokens.has(o.token),
    };
    levels.push(last);
  }
  let cum = 0;
  for (const lv of levels) lv.total = cum += lv.qty;
  return levels;
};

// OrderRow renders one aggregated book level. It briefly flashes when the
// level's summed size changes or when it first appears after the initial book
// load (live), so the user notices live updates. It is a stable top-level
// component so React keeps each row instance across updates (keyed by price
// level); the flash is re-triggered by remounting a keyed overlay whose
// animation runs once. A subtle count badge marks prices with more than one
// order, and a middle dot marks a price where the user has an order.
const OrderRow = ({
  o,
  max,
  sell,
  quote,
  live,
  onPick,
}: {
  o: BookLevel;
  max: number;
  sell: boolean;
  quote: string;
  live: boolean;
  onPick?: (p: { rate: number; qty: number; sell: boolean }) => void;
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
      onClick={() => onPick?.({ rate: o.rate, qty: o.qty, sell })}
      className="relative grid w-full grid-cols-3 px-3 py-[3px] font-mono tabular-nums text-[12px] text-left hover:bg-muted/20"
    >
      {nonce > 0 && (
        <span
          key={nonce}
          className={`absolute inset-0 pointer-events-none ${sell ? 'animate-flash-sell' : 'animate-flash-buy'}`}
        />
      )}
      <span
        className={`absolute inset-y-0 right-0 ${sell ? 'bg-destructive/15' : 'bg-success/15'}`}
        style={{ width: `${(o.total / max) * 100}%` }}
      />
      <span className={`relative flex items-center gap-1 ${sell ? 'text-destructive' : 'text-success'}`}>
        <span
          className={`w-1.5 text-center ${o.mine ? 'text-foreground' : 'text-transparent'}`}
          title={o.mine ? 'you have an order at this price' : undefined}
          aria-hidden={!o.mine}
        >
          ·
        </span>
        {fmtPrice(o.rate, quote)}
      </span>
      <span className="relative flex items-center justify-end gap-1 text-muted-foreground">
        {o.count > 1 && (
          <span
            className="font-mono text-[9px] leading-none bg-muted/40 px-1 py-0.5 rounded-full text-muted-foreground/80"
            title={`quantity is comprised of ${o.count} orders`}
          >
            {o.count}
          </span>
        )}
        {fmtAmt(o.qty, 2)}
      </span>
      <span className="relative text-right text-muted-foreground/70">{fmtAmt(o.total, 2)}</span>
    </button>
  );
};

export const DexOrderBook = ({ market, book, mineTokens, onPick }: Props) => {
  const [tab, setTab] = useState<BookTab>('book');

  // Track whether the book for the current market has loaded, so the initial
  // snapshot does not flash every row; only changes/additions afterwards do.
  const marketKey = `${market.baseID}-${market.quoteID}`;
  const [liveKey, setLiveKey] = useState<string | null>(null);
  useEffect(() => {
    if (book.buys.length || book.sells.length) setLiveKey(marketKey);
  }, [book, marketKey]);
  const live = liveKey === marketKey;

  // Advance the trades tab's relative ages once a second while it is open.
  useSecondTick(tab === 'trades' && book.recentMatches.length > 0);

  const mine = mineTokens ?? EMPTY_MINE;
  const asks = useMemo(() => aggregateLevels(book.sells, mine), [book.sells, mine]); // best ask first
  const bids = useMemo(() => aggregateLevels(book.buys, mine), [book.buys, mine]); // best bid first
  // Defensive newest-first ordering (the feed already prepends new matches).
  const trades = useMemo(() => [...book.recentMatches].sort((a, b) => b.stamp - a.stamp), [book.recentMatches]);
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
          <div className="grid grid-cols-3 px-3 py-1.5 text-[11px] uppercase tracking-wider text-muted-foreground/60">
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
                  <OrderRow key={o.msgRate} o={o} max={maxA} sell quote={market.quote} live={live} onPick={onPick} />
                ))}
            </div>

            <div className="flex items-center justify-between px-3 py-1.5 bg-muted/15 border-y border-border/50 text-[12px]">
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
                <OrderRow key={o.msgRate} o={o} max={maxB} sell={false} quote={market.quote} live={live} onPick={onPick} />
              ))}
            </div>
          </div>
        </div>
      ) : tab === 'trades' ? (
        <div className="flex flex-col min-h-0 flex-1">
          <div className="grid grid-cols-3 px-3 py-1.5 text-[11px] uppercase tracking-wider text-muted-foreground/60">
            <span>Price ({market.quote.split('.')[0]})</span>
            <span className="text-right">Size ({market.base})</span>
            <span className="text-right">Age</span>
          </div>
          <div className="flex-1 overflow-y-auto">
            {trades.length === 0 && (
              <div className="px-3 py-4 text-xs text-muted-foreground">No recent trades</div>
            )}
            {trades.map((t, i) => (
              <div key={i} className="grid grid-cols-3 px-3 py-[3px] font-mono tabular-nums text-[12px]">
                <span className={t.sell ? 'text-destructive' : 'text-success'}>{fmtPrice(t.rate, market.quote)}</span>
                <span className="text-right text-muted-foreground">{fmtAmt(t.qty, 2)}</span>
                <span
                  className="text-right text-muted-foreground/70"
                  title={new Date(t.stamp).toLocaleString()}
                >
                  {fmtAge(t.stamp)}
                </span>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
};
