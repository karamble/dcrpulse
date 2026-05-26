// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';

// MiniOrder mirrors bisonw's order book entry (client/webserver site registry).
export interface MiniOrder {
  qty: number;
  qtyAtomic: number;
  rate: number;
  msgRate: number;
  epoch: number;
  sell: boolean;
  token: string;
}

// Trade is a recent match shown in the trades panel.
export interface Trade {
  rate: number;
  qty: number;
  sell: boolean;
  stamp: number; // unix ms
}

// Candle is one OHLCV bar for the price chart.
export interface Candle {
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  endStamp: number; // unix ms
}

export interface OrderBookState {
  buys: MiniOrder[]; // bids, highest rate first
  sells: MiniOrder[]; // asks, lowest rate first
  recentMatches: Trade[]; // most recent first
}

// MarketStats is the 24h summary shown in the stats bar and markets list.
export interface MarketStats {
  last: number;
  lastUsd?: number;
  change: number;
  changePct: number;
  high24: number;
  low24: number;
  volBase: number;
  volQuote: number;
}

export interface DexMarketRef {
  host: string;
  base: number;
  quote: number;
}

export interface DexFeed {
  book: OrderBookState;
  connected: boolean;
  error: string | null;
}

// useDexFeed connects to the dashboard's DCRDEX WebSocket relay, subscribes to a
// market with loadmarket, and maintains a live order book from the book /
// book_order / unbook_order / update_remaining messages. (Runtime testing is
// deferred until the DEX server is reliable.)
export function useDexFeed(market: DexMarketRef | null): DexFeed {
  const [book, setBook] = useState<OrderBookState>({ buys: [], sells: [], recentMatches: [] });
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!market) return;
    setBook({ buys: [], sells: [], recentMatches: [] });
    setError(null);

    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${window.location.host}/api/dcrdex/ws`);
    wsRef.current = ws;

    let buys: MiniOrder[] = [];
    let sells: MiniOrder[] = [];
    let recentMatches: Trade[] = [];
    const sortBuys = () => buys.sort((a, b) => b.rate - a.rate);
    const sortSells = () => sells.sort((a, b) => a.rate - b.rate);
    const commit = () => setBook({ buys: [...buys], sells: [...sells], recentMatches: [...recentMatches] });
    const toTrade = (m: any): Trade => ({
      rate: Number(m?.rate) || 0,
      qty: Number(m?.qty) || 0,
      sell: !!m?.sell,
      stamp: Number(m?.stamp) || Date.now(),
    });

    ws.onopen = () => {
      setConnected(true);
      ws.send(
        JSON.stringify({
          type: 1,
          route: 'loadmarket',
          id: 1,
          payload: { host: market.host, base: market.base, quote: market.quote },
          sig: '',
        }),
      );
    };
    ws.onclose = () => setConnected(false);
    ws.onerror = () => setError('feed connection error');
    ws.onmessage = (ev) => {
      let msg: any;
      try {
        msg = JSON.parse(ev.data);
      } catch {
        return;
      }
      if (msg?.error) {
        setError(typeof msg.error === 'string' ? msg.error : 'feed error');
        return;
      }
      const bu = msg?.payload; // BookUpdate { action, host, marketID, payload }
      const data = bu?.payload;
      switch (msg?.route) {
        case 'book': {
          buys = (data?.book?.buys || []) as MiniOrder[];
          sells = (data?.book?.sells || []) as MiniOrder[];
          recentMatches = ((data?.book?.recentMatches || []) as any[]).map(toTrade);
          sortBuys();
          sortSells();
          commit();
          break;
        }
        case 'epoch_match':
        case 'match_proof': {
          if (data?.rate !== undefined) {
            recentMatches = [toTrade(data), ...recentMatches].slice(0, 40);
            commit();
          }
          break;
        }
        case 'book_order': {
          const ord = data as MiniOrder;
          if (!ord?.token) break;
          if (ord.sell) {
            sells = sells.filter((o) => o.token !== ord.token);
            sells.push(ord);
            sortSells();
          } else {
            buys = buys.filter((o) => o.token !== ord.token);
            buys.push(ord);
            sortBuys();
          }
          commit();
          break;
        }
        case 'unbook_order': {
          const token = data?.token;
          if (!token) break;
          buys = buys.filter((o) => o.token !== token);
          sells = sells.filter((o) => o.token !== token);
          commit();
          break;
        }
        case 'update_remaining': {
          const token = data?.token;
          if (!token) break;
          const apply = (arr: MiniOrder[]) =>
            arr.forEach((o) => {
              if (o.token === token) {
                o.qty = data.qty;
                o.qtyAtomic = data.qtyAtomic;
              }
            });
          apply(buys);
          apply(sells);
          commit();
          break;
        }
        // epoch_order, candles and notify are ignored for now.
      }
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [market?.host, market?.base, market?.quote]);

  return { book, connected, error };
}
