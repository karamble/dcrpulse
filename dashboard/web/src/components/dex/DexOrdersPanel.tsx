// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { X } from 'lucide-react';
import { fmtAmt } from './dexFormat';
import type { DexOrder } from '../../services/dcrdexApi';

interface Props {
  orders: DexOrder[];
  preview?: boolean;
  onCancel: (id: string) => void;
}

const isOpen = (o: DexOrder) => o.status === 'booked' || o.status === 'epoch';

const Pill = ({ children, kind }: { children: string; kind: 'buy' | 'sell' | 'type' }) => {
  const cls =
    kind === 'buy'
      ? 'bg-success/15 text-success'
      : kind === 'sell'
        ? 'bg-destructive/15 text-destructive'
        : 'bg-muted/40 text-muted-foreground';
  return <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-medium ${cls}`}>{children}</span>;
};

export const DexOrdersPanel = ({ orders, preview, onCancel }: Props) => {
  const [tab, setTab] = useState<'open' | 'history'>('open');
  const open = orders.filter(isOpen);
  const history = orders.filter((o) => !isOpen(o));
  const rows = tab === 'open' ? open : history;

  return (
    <div className="flex flex-col min-h-0 h-full">
      <div className="flex items-center border-b border-border/50 text-xs">
        <button
          type="button"
          onClick={() => setTab('open')}
          className={`relative px-4 py-3 font-medium flex items-center gap-2 transition-colors ${
            tab === 'open' ? 'text-foreground after:absolute after:inset-x-3 after:-bottom-px after:h-0.5 after:bg-primary' : 'text-muted-foreground hover:text-foreground/80'
          }`}
        >
          Open orders
          <span className="font-mono text-[10px] bg-muted/40 px-1.5 rounded-full text-muted-foreground">{open.length}</span>
        </button>
        <button
          type="button"
          onClick={() => setTab('history')}
          className={`relative px-4 py-3 font-medium transition-colors ${
            tab === 'history' ? 'text-foreground after:absolute after:inset-x-3 after:-bottom-px after:h-0.5 after:bg-primary' : 'text-muted-foreground hover:text-foreground/80'
          }`}
        >
          Order history
        </button>
      </div>

      <div className="flex-1 overflow-auto">
        {rows.length === 0 ? (
          <div className="px-4 py-6 text-xs text-muted-foreground">No {tab === 'open' ? 'open orders' : 'order history'}</div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-[10px] uppercase tracking-wider text-muted-foreground/60 text-left">
                <th className="font-medium px-4 py-2">Market</th>
                <th className="font-medium px-2 py-2">Side</th>
                <th className="font-medium px-2 py-2">Type</th>
                <th className="font-medium px-2 py-2 text-right">Amount</th>
                <th className="font-medium px-2 py-2 text-right">Filled</th>
                <th className="font-medium px-2 py-2">Status</th>
                <th className="px-2 py-2" />
              </tr>
            </thead>
            <tbody>
              {rows.map((o) => {
                const pct = o.quantity > 0 ? Math.round((o.filled / o.quantity) * 100) : 0;
                return (
                  <tr key={o.id} className="border-t border-border/40 hover:bg-muted/10">
                    <td className="px-4 py-2 font-mono tabular-nums">{o.marketName}</td>
                    <td className="px-2 py-2"><Pill kind={o.sell ? 'sell' : 'buy'}>{o.sell ? 'Sell' : 'Buy'}</Pill></td>
                    <td className="px-2 py-2"><Pill kind="type">{o.type}</Pill></td>
                    <td className="px-2 py-2 text-right font-mono tabular-nums">{fmtAmt(o.quantity / 1e8, 4)}</td>
                    <td className="px-2 py-2 text-right">
                      <span className="inline-flex items-center gap-2 font-mono tabular-nums text-xs text-muted-foreground">
                        <span className="inline-block w-12 h-1 rounded bg-muted/50 overflow-hidden align-middle">
                          <span className="block h-full bg-primary" style={{ width: `${pct}%` }} />
                        </span>
                        {pct}%
                      </span>
                    </td>
                    <td className="px-2 py-2 text-xs text-muted-foreground">{o.status}</td>
                    <td className="px-2 py-2 text-right">
                      {!preview && isOpen(o) && (
                        <button
                          type="button"
                          title="Cancel order"
                          onClick={() => onCancel(o.id)}
                          className="p-1 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors"
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
};
