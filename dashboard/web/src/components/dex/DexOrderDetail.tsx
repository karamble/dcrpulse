// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ArrowLeft, X } from 'lucide-react';
import type { DexMarket, DexOrder } from '../../services/dcrdexApi';
import { convQty, convRate, fmtAmt, fmtPrice } from './dexFormat';

interface Props {
  order: DexOrder;
  market?: DexMarket;
  onBack: () => void;
  onCancel: (id: string) => void;
}

const isOpen = (o: DexOrder) => o.status === 'booked' || o.status === 'epoch';

const Field = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <div className="space-y-0.5">
    <div className="text-[10px] uppercase tracking-wider text-muted-foreground/60">{label}</div>
    <div className="text-sm font-mono tabular-nums break-all">{children}</div>
  </div>
);

// DexOrderDetail is the in-tab detail view for a single order: summary, fill
// progress and the per-counterparty matches. Amounts are converted to
// conventional units using the order's market.
export const DexOrderDetail = ({ order, market, onBack, onCancel }: Props) => {
  const baseConv = market?.baseConvFactor || 1e8;
  const quoteConv = market?.quoteConvFactor || 1e8;
  const baseSym = market?.base || order.marketName.split('_')[0]?.toUpperCase() || '';
  const quoteSym = market?.quote || order.marketName.split('_')[1]?.toUpperCase() || '';

  const qty = convQty(order.quantity, baseConv);
  const filled = convQty(order.filled, baseConv);
  const settled = convQty(order.settled, baseConv);
  const price = order.rate ? convRate(order.rate, baseConv, quoteConv) : 0;
  const filledPct = order.quantity > 0 ? Math.round((order.filled / order.quantity) * 100) : 0;

  return (
    <div className="px-3 lg:px-4 space-y-4">
      <div className="flex items-center justify-between gap-3">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Orders
        </button>
        {isOpen(order) && (
          <button
            type="button"
            onClick={() => onCancel(order.id)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-destructive/40 text-destructive rounded-lg hover:bg-destructive/10 transition-colors"
          >
            <X className="h-4 w-4" />
            Cancel order
          </button>
        )}
      </div>

      <div className="p-4 rounded-xl bg-gradient-card border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <span className="font-mono">{baseSym}/{quoteSym}</span>
          <span
            className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${
              order.sell ? 'bg-destructive/15 text-destructive' : 'bg-success/15 text-success'
            }`}
          >
            {order.sell ? 'Sell' : 'Buy'}
          </span>
          <span className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-muted/40 text-muted-foreground">{order.type}</span>
          <span className="text-xs text-muted-foreground">{order.status}</span>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <Field label="Quantity">{fmtAmt(qty, 8)} {baseSym}</Field>
          {order.rate > 0 && <Field label="Price">{fmtPrice(price, quoteSym)} {quoteSym}</Field>}
          <Field label="Filled">{fmtAmt(filled, 8)} ({filledPct}%)</Field>
          <Field label="Settled">{fmtAmt(settled, 8)} {baseSym}</Field>
          <Field label="Submitted">{order.submitTime ? new Date(order.submitTime).toLocaleString() : '-'}</Field>
          <Field label="Order ID">{order.id}</Field>
        </div>
      </div>

      <div className="rounded-xl border border-border/50 overflow-hidden">
        <div className="px-4 py-2 text-[10px] uppercase tracking-wider text-muted-foreground/60 border-b border-border/40">
          Matches ({order.matches?.length || 0})
        </div>
        {!order.matches || order.matches.length === 0 ? (
          <div className="px-4 py-6 text-xs text-muted-foreground">No matches yet.</div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-[10px] uppercase tracking-wider text-muted-foreground/60 text-left">
                <th className="font-medium px-4 py-2">Side</th>
                <th className="font-medium px-2 py-2">Status</th>
                <th className="font-medium px-2 py-2 text-right">Qty</th>
                <th className="font-medium px-2 py-2 text-right">Rate</th>
              </tr>
            </thead>
            <tbody>
              {order.matches.map((m) => (
                <tr key={m.matchID} className="border-t border-border/40">
                  <td className="px-4 py-2">{m.side}</td>
                  <td className="px-2 py-2 text-xs text-muted-foreground">
                    {m.status}
                    {m.revoked && <span className="ml-1 text-destructive">(revoked)</span>}
                  </td>
                  <td className="px-2 py-2 text-right font-mono tabular-nums">{fmtAmt(convQty(m.qty, baseConv), 8)}</td>
                  <td className="px-2 py-2 text-right font-mono tabular-nums">{fmtPrice(convRate(m.rate, baseConv, quoteConv), quoteSym)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
};
