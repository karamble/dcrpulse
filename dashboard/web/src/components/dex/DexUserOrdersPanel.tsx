// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { convQty, convRate, fmtAmt, fmtPrice } from './dexFormat';
import { isCancellable, orderStatusString, type DexMarket, type DexOrder } from '../../services/dcrdexApi';
import { DexOrderDetail } from './DexOrderDetail';

interface Props {
  orders: DexOrder[];
  market: DexMarket;
  preview?: boolean;
  onCancel: (order: DexOrder, market?: DexMarket) => void;
}

const isOpen = (o: DexOrder) => o.status === 'booked' || o.status === 'epoch';

// fmtAge renders a compact relative age from a unix-ms submit time.
const fmtAge = (ms: number): string => {
  if (!ms) return '-';
  const s = Math.max(0, Math.floor((Date.now() - ms) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
};

const Datum = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <div className="flex justify-between gap-2">
    <span className="text-muted-foreground">{label}</span>
    <span className="font-mono tabular-nums">{children}</span>
  </div>
);

// DexUserOrdersPanel is the dcrdex-style per-market open-orders list shown on the
// trade terminal: the selected market's active orders, each an expandable row
// (collapsed: side, qty @ price, status; expanded: type/age/filled/settled with
// cancel and a link to the multi-step order detail). Mirrors bisonw's markets.ts
// "Your Orders" panel (resolveUserOrders).
export const DexUserOrdersPanel = ({ orders, market, preview, onCancel }: Props) => {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [selectedID, setSelectedID] = useState<string | null>(null);

  const baseConv = market.baseConvFactor || 1e8;
  const quoteConv = market.quoteConvFactor || 1e8;
  const quoteSym = market.quote.split('.')[0];

  const rows = orders
    .filter((o) => o.baseID === market.baseID && o.quoteID === market.quoteID && isOpen(o))
    .sort((a, b) => (b.submitTime || b.stamp) - (a.submitTime || a.stamp));

  // A clicked row opens the in-panel detail (reused multi-step view); it clears
  // if the order leaves the tracked set after a refresh.
  const selected = selectedID ? orders.find((o) => o.id === selectedID) : null;
  if (selected) {
    return (
      <div className="py-3">
        <DexOrderDetail order={selected} market={market} onBack={() => setSelectedID(null)} onCancel={onCancel} />
      </div>
    );
  }

  const toggle = (id: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <div className="overflow-hidden">
      <div className="px-4 py-2 text-[10px] uppercase tracking-wider text-muted-foreground/60 border-b border-border/40 flex items-center gap-2">
        Your Orders
        <span className="text-muted-foreground/50 normal-case tracking-normal">
          {market.base}/{quoteSym}
        </span>
        <span className="ml-auto font-mono bg-muted/40 px-1.5 rounded-full text-muted-foreground normal-case">{rows.length}</span>
      </div>

      {rows.length === 0 ? (
        <div className="px-4 py-3 text-xs text-muted-foreground">
          No open orders for {market.base}/{quoteSym}
        </div>
      ) : (
        <div className="divide-y divide-border/40 max-h-[220px] overflow-y-auto">
          {rows.map((o) => {
            const open = expanded.has(o.id);
            const price = o.rate > 0 ? fmtPrice(convRate(o.rate, baseConv, quoteConv), market.quote) : 'market';
            const filledPct = o.quantity > 0 ? Math.round((o.filled / o.quantity) * 100) : 0;
            const settledPct = o.quantity > 0 ? Math.round((o.settled / o.quantity) * 100) : 0;
            return (
              <div key={o.id}>
                <button
                  type="button"
                  onClick={() => toggle(o.id)}
                  className="w-full flex items-center gap-2 px-3 py-2 text-xs hover:bg-muted/10 transition-colors"
                >
                  <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${o.sell ? 'bg-destructive' : 'bg-success'}`} />
                  <span className={`font-medium ${o.sell ? 'text-destructive' : 'text-success'}`}>{o.sell ? 'Sell' : 'Buy'}</span>
                  <span className="font-mono tabular-nums">{fmtAmt(convQty(o.quantity, baseConv), 4)}</span>
                  <span className="text-muted-foreground/60">{market.base}</span>
                  <span className="text-muted-foreground">@</span>
                  <span className="font-mono tabular-nums">{price}</span>
                  <span className="ml-auto text-muted-foreground">{orderStatusString(o)}</span>
                  {open ? <ChevronUp className="h-3.5 w-3.5 text-muted-foreground" /> : <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />}
                </button>
                {open && (
                  <div className="px-3 pb-2.5 pt-0.5 space-y-1 text-[11px]">
                    <Datum label="Type">{o.type}</Datum>
                    <Datum label="Age">{fmtAge(o.submitTime || o.stamp)}</Datum>
                    <Datum label="Filled">{filledPct}%</Datum>
                    <Datum label="Settled">{settledPct}%</Datum>
                    <div className="flex gap-2 pt-1.5">
                      {!preview && isCancellable(o) && (
                        <button
                          type="button"
                          onClick={() => onCancel(o, market)}
                          className="px-2 py-1 border border-destructive/40 text-destructive rounded-md hover:bg-destructive/10 transition-colors"
                        >
                          Cancel
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={() => setSelectedID(o.id)}
                        className="px-2 py-1 border border-border rounded-md hover:bg-background/50 transition-colors"
                      >
                        Open detail &rarr;
                      </button>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
