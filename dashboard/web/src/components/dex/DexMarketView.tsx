// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, X } from 'lucide-react';
import {
  getDexConfig,
  getDexMyOrders,
  cancelDexOrder,
  type DexMarket,
  type DexOrder,
} from '../../services/dcrdexApi';
import { useDexFeed, type MiniOrder } from './useDexFeed';
import { CoinIcon } from './CoinIcon';

const HOST = 'dex.decred.org:7232';

const OrderRows = ({ orders, sell }: { orders: MiniOrder[]; sell: boolean }) => {
  if (orders.length === 0) {
    return <div className="px-2 py-3 text-xs text-muted-foreground">No orders</div>;
  }
  return (
    <div className="text-sm font-mono">
      {orders.slice(0, 18).map((o) => (
        <div key={o.token} className="flex justify-between px-2 py-0.5">
          <span className={sell ? 'text-destructive' : 'text-success'}>{o.rate.toFixed(8)}</span>
          <span className="text-muted-foreground">{o.qty.toFixed(8)}</span>
        </div>
      ))}
    </div>
  );
};

// DexMarketView is the live trading view: a market selector and a real-time
// order book fed by the DCRDEX WebSocket relay. (Runtime testing deferred until
// the DEX server is reliable.)
export const DexMarketView = () => {
  const [markets, setMarkets] = useState<DexMarket[]>([]);
  const [sel, setSel] = useState<DexMarket | null>(null);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [orders, setOrders] = useState<DexOrder[]>([]);

  useEffect(() => {
    getDexConfig(HOST)
      .then((c) => {
        setMarkets(c.markets);
        setSel((prev) => prev || c.markets[0] || null);
      })
      .catch((e: any) =>
        setLoadErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Failed to load markets'),
      );
  }, []);

  const refreshOrders = () => {
    getDexMyOrders(HOST)
      .then(setOrders)
      .catch(() => {});
  };
  useEffect(() => {
    refreshOrders();
    const id = window.setInterval(refreshOrders, 10000);
    return () => window.clearInterval(id);
  }, []);

  const market = sel ? { host: HOST, base: sel.baseID, quote: sel.quoteID } : null;
  const { book, connected, error } = useDexFeed(market);

  if (loadErr) {
    return (
      <div className="px-4">
        <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{loadErr}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="px-4 space-y-4">
      <div className="flex flex-wrap gap-2">
        {markets.map((m) => {
          const active = !!sel && m.baseID === sel.baseID && m.quoteID === sel.quoteID;
          return (
            <button
              key={`${m.baseID}-${m.quoteID}`}
              type="button"
              onClick={() => setSel(m)}
              className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border text-sm transition-colors ${
                active ? 'bg-primary/20 border-primary/40' : 'bg-muted/10 border-border/50 hover:bg-muted/20'
              }`}
            >
              <span className="flex -space-x-1.5">
                <CoinIcon symbol={m.base} className="ring-1 ring-card" />
                <CoinIcon symbol={m.quote} className="ring-1 ring-card" />
              </span>
              {m.base}/{m.quote}
            </button>
          );
        })}
      </div>

      {sel && (
        <div className="rounded-xl bg-gradient-card border border-border/50 p-4">
          <div className="flex items-center justify-between mb-3">
            <h3 className="font-semibold">
              {sel.base}/{sel.quote} order book
            </h3>
            <span className={`text-xs flex items-center gap-1.5 ${connected ? 'text-success' : 'text-muted-foreground'}`}>
              <span className={`h-2 w-2 rounded-full ${connected ? 'bg-success' : 'bg-muted-foreground/50'}`} />
              {connected ? 'live' : 'connecting…'}
            </span>
          </div>
          {error && <div className="text-xs text-warning mb-2">{error}</div>}
          <div className="flex flex-col sm:flex-row gap-6">
            <div className="flex-1">
              <div className="text-xs text-success mb-1 px-2">Bids · price / size</div>
              <OrderRows orders={book.buys} sell={false} />
            </div>
            <div className="flex-1">
              <div className="text-xs text-destructive mb-1 px-2">Asks · price / size</div>
              <OrderRows orders={book.sells} sell />
            </div>
          </div>
        </div>
      )}

      {orders.length > 0 && (
        <div className="rounded-xl bg-gradient-card border border-border/50 p-4">
          <h3 className="font-semibold mb-3">Your orders</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-xs text-muted-foreground text-left">
                  <th className="py-1 pr-3">Market</th>
                  <th className="py-1 pr-3">Side</th>
                  <th className="py-1 pr-3">Type</th>
                  <th className="py-1 pr-3">Status</th>
                  <th className="py-1 pr-3">Filled</th>
                  <th className="py-1" />
                </tr>
              </thead>
              <tbody>
                {orders.map((o) => {
                  const pct = o.quantity > 0 ? Math.round((o.filled / o.quantity) * 100) : 0;
                  return (
                    <tr key={o.id} className="border-t border-border/40">
                      <td className="py-1.5 pr-3 font-mono">{o.marketName}</td>
                      <td className={`py-1.5 pr-3 ${o.sell ? 'text-destructive' : 'text-success'}`}>
                        {o.sell ? 'sell' : 'buy'}
                      </td>
                      <td className="py-1.5 pr-3">{o.type}</td>
                      <td className="py-1.5 pr-3">{o.status}</td>
                      <td className="py-1.5 pr-3">{pct}%</td>
                      <td className="py-1.5">
                        {o.status === 'booked' && (
                          <button
                            type="button"
                            title="Cancel order"
                            onClick={async () => {
                              try {
                                await cancelDexOrder(o.id);
                                refreshOrders();
                              } catch {
                                /* ignore */
                              }
                            }}
                            className="p-1 rounded hover:bg-background/60 text-muted-foreground hover:text-destructive transition-colors"
                          >
                            <X className="h-4 w-4" />
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
};
