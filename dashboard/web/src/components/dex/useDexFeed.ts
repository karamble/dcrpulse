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

// Candle is one OHLCV bar for the price chart, in conventional units.
export interface Candle {
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  startStamp: number; // unix ms
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
  baseConvFactor: number;
  quoteConvFactor: number;
}

export interface DexFeed {
  book: OrderBookState;
  candles: Candle[];
  connected: boolean;
  error: string | null;
}

// statsFromCandles derives a 24h summary from a candle series (the live feed
// has no separate 24h stats route; bisonw's own markets page does the same when
// a spot is unavailable).
export function statsFromCandles(candles: Candle[]): MarketStats | null {
  if (candles.length === 0) return null;
  const last = candles[candles.length - 1].close;
  const dayStart = candles[candles.length - 1].endStamp - 24 * 3600_000;
  const win = candles.filter((c) => c.endStamp >= dayStart);
  const w = win.length ? win : candles;
  const open = w[0].open;
  const change = last - open;
  return {
    last,
    change,
    changePct: open ? (change / open) * 100 : 0,
    high24: Math.max(...w.map((c) => c.high)),
    low24: Math.min(...w.map((c) => c.low)),
    volBase: w.reduce((s, c) => s + c.volume, 0),
    volQuote: w.reduce((s, c) => s + c.volume * c.close, 0),
  };
}

// useDexFeed connects to the dashboard's DCRDEX WebSocket relay, subscribes to a
// market with loadmarket + loadcandles, and maintains a live order book, recent
// trades and candle series from the book / book_order / unbook_order /
// update_remaining / candles / candle_update / epoch_match_summary messages.
// Match and candle rates arrive as atomic message rates and are converted to
// conventional units here using the market's conversion factors. dur is the
// candle bin size; changing it re-requests candles without dropping the order
// book subscription. (Runtime testing is deferred until the DEX server is
// reliable.)
export function useDexFeed(market: DexMarketRef | null, dur = '1h'): DexFeed {
  const [book, setBook] = useState<OrderBookState>({ buys: [], sells: [], recentMatches: [] });
  const [candles, setCandles] = useState<Candle[]>([]);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const durRef = useRef(dur);
  durRef.current = dur;
  const candlesRef = useRef<Candle[]>([]);

  useEffect(() => {
    if (!market) return;
    setBook({ buys: [], sells: [], recentMatches: [] });
    setCandles([]);
    setError(null);

    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${window.location.host}/api/dcrdex/ws`);
    wsRef.current = ws;

    // rateConv converts an atomic message rate to a conventional price; mirrors
    // bisonw's OrderUtil.RateEncodingFactor / baseConv * quoteConv.
    const rateConv = (1e8 / market.baseConvFactor) * market.quoteConvFactor;

    let buys: MiniOrder[] = [];
    let sells: MiniOrder[] = [];
    let recentMatches: Trade[] = [];
    candlesRef.current = [];
    const sortBuys = () => buys.sort((a, b) => b.rate - a.rate);
    const sortSells = () => sells.sort((a, b) => a.rate - b.rate);
    const commit = () => setBook({ buys: [...buys], sells: [...sells], recentMatches: [...recentMatches] });
    // Recent matches (MatchSummary) carry atomic rate/qty, unlike book orders.
    const toTrade = (m: any): Trade => ({
      rate: (Number(m?.rate) || 0) / rateConv,
      qty: (Number(m?.qty) || 0) / market.baseConvFactor,
      sell: !!m?.sell,
      stamp: Number(m?.stamp) || Date.now(),
    });
    const toCandle = (c: any): Candle => ({
      open: (Number(c?.startRate) || 0) / rateConv,
      close: (Number(c?.endRate) || 0) / rateConv,
      high: (Number(c?.highRate) || 0) / rateConv,
      low: (Number(c?.lowRate) || 0) / rateConv,
      volume: (Number(c?.matchVolume) || 0) / market.baseConvFactor,
      startStamp: Number(c?.startStamp) || 0,
      endStamp: Number(c?.endStamp) || 0,
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
      ws.send(
        JSON.stringify({
          type: 1,
          route: 'loadcandles',
          id: 2,
          payload: { host: market.host, base: market.base, quote: market.quote, dur: durRef.current },
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
        case 'candles': {
          if (data?.dur && data.dur !== durRef.current) break;
          candlesRef.current = ((data?.candles || []) as any[]).map(toCandle);
          setCandles([...candlesRef.current]);
          break;
        }
        case 'candle_update': {
          if (data?.dur && data.dur !== durRef.current) break;
          const c = toCandle(data?.candle);
          const series = candlesRef.current;
          const last = series[series.length - 1];
          if (last && last.startStamp === c.startStamp) series[series.length - 1] = c;
          else candlesRef.current = [...series, c];
          setCandles([...candlesRef.current]);
          break;
        }
        case 'epoch_match_summary': {
          const sums = (data?.matchSummaries || []) as any[];
          if (sums.length) {
            recentMatches = [...sums.map(toTrade), ...recentMatches].slice(0, 40);
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
        // epoch_order and notifications are ignored for now.
      }
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [market?.host, market?.base, market?.quote, market?.baseConvFactor, market?.quoteConvFactor]);

  // When only the bin size changes, re-request candles on the live socket
  // instead of reconnecting (the initial load is handled by the socket's
  // onopen above).
  useEffect(() => {
    const ws = wsRef.current;
    if (!market || !ws || ws.readyState !== WebSocket.OPEN) return;
    candlesRef.current = [];
    setCandles([]);
    ws.send(
      JSON.stringify({
        type: 1,
        route: 'loadcandles',
        id: 2,
        payload: { host: market.host, base: market.base, quote: market.quote, dur },
        sig: '',
      }),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dur]);

  return { book, candles, connected, error };
}
