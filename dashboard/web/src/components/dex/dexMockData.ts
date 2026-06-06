// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Sample data for the DEX trading-view preview mode (/dex?preview). Lets the UI
// be developed and reviewed without a reachable DEX server. NOT used in the
// live flow.

import type { DexMarket, DexOrder } from '../../services/dcrdexApi';
import type { Candle, MarketStats, MiniOrder, OrderBookState, Trade } from './useDexFeed';
import { RateEncodingFactor } from './dexFormat';

export const mockMarkets: DexMarket[] = [
  { base: 'DCR', quote: 'BTC', baseID: 42, quoteID: 0, lotSize: 1e8, rateStep: 1, baseConvFactor: 1e8, quoteConvFactor: 1e8 },
  { base: 'DCR', quote: 'USDC.POLYGON', baseID: 42, quoteID: 966001, lotSize: 1e8, rateStep: 1, baseConvFactor: 1e8, quoteConvFactor: 1e6 },
  { base: 'BTC', quote: 'USDC.POLYGON', baseID: 0, quoteID: 966001, lotSize: 1e6, rateStep: 1, baseConvFactor: 1e8, quoteConvFactor: 1e6 },
  { base: 'LTC', quote: 'USDT.POLYGON', baseID: 2, quoteID: 966002, lotSize: 1e8, rateStep: 1, baseConvFactor: 1e8, quoteConvFactor: 1e6 },
  { base: 'ETH', quote: 'BTC', baseID: 60, quoteID: 0, lotSize: 1e8, rateStep: 1, baseConvFactor: 1e9, quoteConvFactor: 1e8 },
  { base: 'ZEC', quote: 'BTC', baseID: 133, quoteID: 0, lotSize: 1e8, rateStep: 1, baseConvFactor: 1e8, quoteConvFactor: 1e8 },
];

// Per-market reference mid-price driving the sample book, stats and candles.
const MIDS: Record<string, number> = {
  'DCR/BTC': 0.00041208,
  'DCR/USDC.POLYGON': 27.84,
  'BTC/USDC.POLYGON': 67541.2,
  'LTC/USDT.POLYGON': 84.12,
  'ETH/BTC': 0.05214,
  'ZEC/BTC': 0.00052,
};

const key = (m: DexMarket) => `${m.base}/${m.quote}`;
export const mockMid = (m: DexMarket): number => MIDS[key(m)] ?? 0.0005;

// Deterministic pseudo-random so the preview is stable across renders.
const rng = (seed: number) => {
  let s = seed % 2147483647;
  if (s <= 0) s += 2147483646;
  return () => (s = (s * 16807) % 2147483647) / 2147483647;
};

export function mockBook(m: DexMarket): OrderBookState {
  const mid = mockMid(m);
  const r = rng(Math.round(mid * 1e6) + 7);
  const step = mid * 0.0006;
  const mk = (rate: number, qty: number, sell: boolean, i: number): MiniOrder => ({
    rate: Number(rate.toFixed(8)),
    qty: Number(qty.toFixed(4)),
    qtyAtomic: Math.round(qty * m.baseConvFactor),
    msgRate: Math.round(rate * (RateEncodingFactor / m.baseConvFactor) * m.quoteConvFactor),
    epoch: 0,
    sell,
    token: `mock-${sell ? 's' : 'b'}-${i}`,
  });
  const buys = Array.from({ length: 14 }, (_, i) => mk(mid - (i + 1) * step, 40 + r() * 420, false, i));
  const sells = Array.from({ length: 14 }, (_, i) => mk(mid + (i + 1) * step, 40 + r() * 420, true, i));

  let t = Date.now();
  const recentMatches: Trade[] = Array.from({ length: 18 }, () => {
    t -= Math.floor(r() * 9000 + 1500);
    return {
      rate: Number((mid + (r() - 0.5) * step * 6).toFixed(8)),
      qty: Number((8 + r() * 180).toFixed(4)),
      sell: r() > 0.5,
      stamp: t,
    };
  });
  return { buys, sells, recentMatches };
}

export function mockStats(m: DexMarket): MarketStats {
  const mid = mockMid(m);
  const r = rng(Math.round(mid * 1e5) + 13);
  const changePct = (r() - 0.42) * 6;
  const change = mid * (changePct / 100);
  return {
    last: mid,
    lastUsd: m.quote.toUpperCase().startsWith('USD') ? mid : mid * 67541.2,
    change,
    changePct,
    high24: mid * (1 + r() * 0.03),
    low24: mid * (1 - r() * 0.03),
    volBase: Math.round(40000 + r() * 200000),
    volQuote: Number((mid * (40000 + r() * 200000)).toFixed(2)),
  };
}

export function mockCandles(m: DexMarket, n = 90): Candle[] {
  const mid = mockMid(m);
  const r = rng(Math.round(mid * 1e7) + 3);
  const span = mid * 0.00002;
  const candles: Candle[] = [];
  let last = mid - span * 8;
  let stamp = Date.now() - n * 3600_000;
  for (let i = 0; i < n; i++) {
    const drift = (Math.sin(i / 6) + Math.cos(i / 3.2)) * span;
    const open = last + (r() - 0.5) * span;
    const close = open + drift + (r() - 0.5) * span * 3;
    const high = Math.max(open, close) + r() * span * 2.5;
    const low = Math.min(open, close) - r() * span * 2.5;
    const startStamp = stamp;
    stamp += 3600_000;
    candles.push({ open, high, low, close, volume: r() * 200 + 30, startStamp, endStamp: stamp });
    last = close;
  }
  candles[n - 1].close = mid;
  candles[n - 1].high = Math.max(candles[n - 1].high, mid + span * 2);
  return candles;
}

export const mockOrders: DexOrder[] = [
  { id: 'mockbookedorder', host: 'dex.decred.org:7232', marketName: 'dcr_btc', baseID: 42, quoteID: 0, type: 'limit', sell: false, status: 'booked', stamp: 0, submitTime: 0, quantity: 5e8, filled: 1e8, settled: 0, rate: 52000 },
  { id: 'mockbookedsell', host: 'dex.decred.org:7232', marketName: 'dcr_btc', baseID: 42, quoteID: 0, type: 'limit', sell: true, status: 'booked', stamp: 0, submitTime: 0, quantity: 1.8e8, filled: 0, settled: 0, rate: 53000 },
  { id: 'mockexecorder', host: 'dex.decred.org:7232', marketName: 'dcr_btc', baseID: 42, quoteID: 0, type: 'limit', sell: true, status: 'executed', stamp: 0, submitTime: 0, quantity: 2e8, filled: 2e8, settled: 2e8, rate: 53000 },
];
