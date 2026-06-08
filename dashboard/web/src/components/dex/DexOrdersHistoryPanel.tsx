// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertCircle, ChevronRight } from 'lucide-react';
import {
  getDexConfig,
  getDexOrders,
  type DexMarket,
  type DexOrder,
  type DexOrderFilter,
} from '../../services/dcrdexApi';
import { convQty, convRate, fmtAmt, fmtPrice } from './dexFormat';
import { DexOrderDetail } from './DexOrderDetail';
import { useDexCancel } from './DexCancelOrder';
import { useDexRefreshOnNotes } from './DexLiveProvider';

const PAGE = 50;
const STATUS_OPTIONS = ['epoch', 'booked', 'executed', 'canceled', 'revoked'];

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

// DexOrdersHistoryPanel is the full order history for a host: every order in the
// archive, including canceled/executed/revoked, from the /dcrdex/orders route
// (the RPC myorders feed used by the trade-view live panels returns only
// active/recent orders). Filterable by market and status server-side, paginated
// with a Load more button, and a row opens the in-tab detail view.
export const DexOrdersHistoryPanel = ({ host }: { host: string }) => {
  const [orders, setOrders] = useState<DexOrder[] | null>(null);
  const [markets, setMarkets] = useState<DexMarket[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [marketFilter, setMarketFilter] = useState('all'); // 'all' | `${baseID}-${quoteID}`
  const [statusFilter, setStatusFilter] = useState('all');
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [more, setMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);

  const buildFilter = useCallback(
    (offset?: string): DexOrderFilter => {
      const f: DexOrderFilter = { host, n: PAGE };
      if (offset) f.offset = offset;
      if (statusFilter !== 'all') f.status = statusFilter;
      if (marketFilter !== 'all') {
        const [b, q] = marketFilter.split('-').map(Number);
        f.market = { baseID: b, quoteID: q };
      }
      return f;
    },
    [host, statusFilter, marketFilter],
  );

  // Load (or reload) the first page; resets pagination. Filters drive the
  // request, so changing them re-queries the whole archive, not just the loaded
  // rows.
  const loadFirst = useCallback(() => {
    getDexOrders(buildFilter())
      .then((o) => {
        setOrders(o);
        setMore(o.length === PAGE);
        setErr(null);
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Failed to load orders'));
  }, [buildFilter]);

  useEffect(() => {
    setOrders(null);
    loadFirst();
  }, [loadFirst]);
  // New orders/fills land on page 1; reloading it keeps the history fresh
  // without disturbing already-loaded older pages mid-scroll.
  useDexRefreshOnNotes(['order', 'match'], loadFirst);

  const loadMore = () => {
    if (!orders || orders.length === 0 || loadingMore) return;
    setLoadingMore(true);
    getDexOrders(buildFilter(orders[orders.length - 1].id))
      .then((o) => {
        setOrders((prev) => [...(prev || []), ...o]);
        setMore(o.length === PAGE);
      })
      .catch(() => {})
      .finally(() => setLoadingMore(false));
  };

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

  const { requestCancel, modal: cancelModal } = useDexCancel(loadFirst);

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
      <>
        <DexOrderDetail
          order={selected}
          market={marketByKey[marketKey(selected.baseID, selected.quoteID)]}
          onBack={() => setSelectedID(null)}
          onCancel={requestCancel}
        />
        {cancelModal}
      </>
    );
  }

  const selCls = 'bg-background/50 border border-border rounded-lg px-2 py-1.5 text-xs';

  return (
    <div className="px-3 lg:px-4 space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <select className={selCls} value={marketFilter} onChange={(e) => setMarketFilter(e.target.value)}>
          <option value="all">All markets</option>
          {markets.map((m) => (
            <option key={marketKey(m.baseID, m.quoteID)} value={marketKey(m.baseID, m.quoteID)}>
              {m.base}/{m.quote.split('.')[0]}
            </option>
          ))}
        </select>
        <select className={selCls} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
          <option value="all">All statuses</option>
          {STATUS_OPTIONS.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <span className="text-xs text-muted-foreground ml-auto">{orders.length} orders</span>
      </div>

      <div className="rounded-xl border border-border/50 overflow-hidden">
        {orders.length === 0 ? (
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
                <th className="font-medium px-2 py-2 hidden md:table-cell">Date</th>
                <th className="px-2 py-2" />
              </tr>
            </thead>
            <tbody>
              {orders.map((o) => {
                const mk = marketByKey[marketKey(o.baseID, o.quoteID)];
                const baseConv = mk?.baseConvFactor || 1e8;
                const quoteConv = mk?.quoteConvFactor || 1e8;
                const pct = o.quantity > 0 ? Math.round((o.filled / o.quantity) * 100) : 0;
                const ts = o.submitTime || o.stamp;
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
                    <td className="px-2 py-2 text-xs text-muted-foreground hidden md:table-cell whitespace-nowrap" title={ts ? new Date(ts).toLocaleString() : ''}>
                      {ts ? new Date(ts).toLocaleDateString() : '-'}
                    </td>
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

      {more && (
        <div className="flex justify-center">
          <button
            type="button"
            onClick={loadMore}
            disabled={loadingMore}
            className="px-4 py-2 text-sm border border-border rounded-lg hover:bg-background/50 transition-colors disabled:opacity-50"
          >
            {loadingMore ? 'Loading…' : 'Load more'}
          </button>
        </div>
      )}
    </div>
  );
};
