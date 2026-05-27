// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, ChevronRight } from 'lucide-react';
import {
  getDexConfig,
  getDexMyOrders,
  cancelDexOrder,
  type DexMarket,
  type DexOrder,
} from '../../services/dcrdexApi';
import { convQty, convRate, fmtAmt, fmtPrice } from './dexFormat';
import { DexOrderDetail } from './DexOrderDetail';
import { useDexRefreshOnNotes } from './DexLiveProvider';

const marketKey = (baseID: number, quoteID: number) => `${baseID}-${quoteID}`;

const Pill = ({ children, kind }: { children: string; kind: 'buy' | 'sell' | 'type' }) => {
  const cls =
    kind === 'buy'
      ? 'bg-success/15 text-success'
      : kind === 'sell'
        ? 'bg-destructive/15 text-destructive'
        : 'bg-muted/40 text-muted-foreground';
  return <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-medium ${cls}`}>{children}</span>;
};

// DexOrdersHistoryPanel lists the user's orders for a host (active and recently
// tracked; bisonw v1.0.6 has no full archive route). Orders are filterable by
// market and status, and a row opens the in-tab detail view.
export const DexOrdersHistoryPanel = ({ host }: { host: string }) => {
  const [orders, setOrders] = useState<DexOrder[] | null>(null);
  const [markets, setMarkets] = useState<DexMarket[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [marketFilter, setMarketFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all');
  const [selectedID, setSelectedID] = useState<string | null>(null);

  const refresh = () => {
    getDexMyOrders(host)
      .then((o) => {
        setOrders(o);
        setErr(null);
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Failed to load orders'));
  };
  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host]);
  useDexRefreshOnNotes(['order', 'match'], refresh);

  useEffect(() => {
    getDexConfig(host)
      .then((c) => setMarkets(c.markets))
      .catch(() => {});
  }, [host]);

  const marketByKey = useMemo(() => {
    const m: Record<string, DexMarket> = {};
    markets.forEach((mk) => {
      m[marketKey(mk.baseID, mk.quoteID)] = mk;
    });
    return m;
  }, [markets]);

  const statuses = useMemo(() => Array.from(new Set((orders || []).map((o) => o.status))).sort(), [orders]);
  const marketNames = useMemo(() => Array.from(new Set((orders || []).map((o) => o.marketName))).sort(), [orders]);

  const rows = useMemo(
    () =>
      (orders || [])
        .filter((o) => marketFilter === 'all' || o.marketName === marketFilter)
        .filter((o) => statusFilter === 'all' || o.status === statusFilter)
        .sort((a, b) => (b.submitTime || b.stamp) - (a.submitTime || a.stamp)),
    [orders, marketFilter, statusFilter],
  );

  const onCancel = async (id: string) => {
    try {
      await cancelDexOrder(id);
      refresh();
    } catch {
      /* ignore */
    }
  };

  if (err) {
    return (
      <div className="px-3 lg:px-4">
        <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      </div>
    );
  }

  if (orders === null) {
    return (
      <div className="min-h-[30vh] flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  const selected = selectedID ? orders.find((o) => o.id === selectedID) : null;
  if (selected) {
    return (
      <DexOrderDetail
        order={selected}
        market={marketByKey[marketKey(selected.baseID, selected.quoteID)]}
        onBack={() => setSelectedID(null)}
        onCancel={onCancel}
      />
    );
  }

  const selCls = 'bg-background/50 border border-border rounded-lg px-2 py-1.5 text-xs';

  return (
    <div className="px-3 lg:px-4 space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <select className={selCls} value={marketFilter} onChange={(e) => setMarketFilter(e.target.value)}>
          <option value="all">All markets</option>
          {marketNames.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </select>
        <select className={selCls} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
          <option value="all">All statuses</option>
          {statuses.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <span className="text-xs text-muted-foreground ml-auto">{rows.length} orders</span>
      </div>

      <div className="rounded-xl border border-border/50 overflow-hidden">
        {rows.length === 0 ? (
          <div className="px-4 py-8 text-sm text-muted-foreground">No orders.</div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-[10px] uppercase tracking-wider text-muted-foreground/60 text-left">
                <th className="font-medium px-4 py-2">Market</th>
                <th className="font-medium px-2 py-2">Side</th>
                <th className="font-medium px-2 py-2">Type</th>
                <th className="font-medium px-2 py-2 text-right">Amount</th>
                <th className="font-medium px-2 py-2 text-right">Price</th>
                <th className="font-medium px-2 py-2 text-right">Filled</th>
                <th className="font-medium px-2 py-2">Status</th>
                <th className="px-2 py-2" />
              </tr>
            </thead>
            <tbody>
              {rows.map((o) => {
                const mk = marketByKey[marketKey(o.baseID, o.quoteID)];
                const baseConv = mk?.baseConvFactor || 1e8;
                const quoteConv = mk?.quoteConvFactor || 1e8;
                const pct = o.quantity > 0 ? Math.round((o.filled / o.quantity) * 100) : 0;
                return (
                  <tr
                    key={o.id}
                    onClick={() => setSelectedID(o.id)}
                    className="border-t border-border/40 hover:bg-muted/10 cursor-pointer"
                  >
                    <td className="px-4 py-2 font-mono tabular-nums">{o.marketName}</td>
                    <td className="px-2 py-2">
                      <Pill kind={o.sell ? 'sell' : 'buy'}>{o.sell ? 'Sell' : 'Buy'}</Pill>
                    </td>
                    <td className="px-2 py-2">
                      <Pill kind="type">{o.type}</Pill>
                    </td>
                    <td className="px-2 py-2 text-right font-mono tabular-nums">{fmtAmt(convQty(o.quantity, baseConv), 4)}</td>
                    <td className="px-2 py-2 text-right font-mono tabular-nums text-muted-foreground">
                      {o.rate > 0 ? fmtPrice(convRate(o.rate, baseConv, quoteConv), mk?.quote || '') : 'market'}
                    </td>
                    <td className="px-2 py-2 text-right font-mono tabular-nums text-xs text-muted-foreground">{pct}%</td>
                    <td className="px-2 py-2 text-xs text-muted-foreground">{o.status}</td>
                    <td className="px-2 py-2 text-right">
                      <ChevronRight className="h-4 w-4 text-muted-foreground inline-block" />
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
