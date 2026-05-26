// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Sample data for the DEX trading-view preview mode (/dex?preview). Lets the UI
// be developed and reviewed without a reachable DEX server. NOT used in the
// live flow.

import type { DexMarket, DexOrder } from '../../services/dcrdexApi';
import type { MiniOrder, OrderBookState } from './useDexFeed';

export const mockMarkets: DexMarket[] = [
  { base: 'DCR', quote: 'BTC', baseID: 42, quoteID: 0, lotSize: 1e8, rateStep: 1, baseConvFactor: 1e8, quoteConvFactor: 1e8 },
  { base: 'DCR', quote: 'USDC.POLYGON', baseID: 42, quoteID: 966001, lotSize: 1e8, rateStep: 1, baseConvFactor: 1e8, quoteConvFactor: 1e6 },
  { base: 'LTC', quote: 'USDT.POLYGON', baseID: 2, quoteID: 966002, lotSize: 1e8, rateStep: 1, baseConvFactor: 1e8, quoteConvFactor: 1e6 },
];

export function mockBook(mid = 0.00052): OrderBookState {
  const mk = (rate: number, qty: number, sell: boolean, i: number): MiniOrder => ({
    rate: Number(rate.toFixed(8)),
    qty,
    qtyAtomic: Math.round(qty * 1e8),
    msgRate: Math.round(rate * 1e8),
    epoch: 0,
    sell,
    token: `mock-${sell ? 's' : 'b'}-${i}`,
  });
  const step = 0.0000005;
  const buys = Array.from({ length: 12 }, (_, i) => mk(mid - (i + 1) * step, 1 + i * 0.5, false, i));
  const sells = Array.from({ length: 12 }, (_, i) => mk(mid + (i + 1) * step, 1 + i * 0.5, true, i));
  return { buys, sells };
}

export const mockOrders: DexOrder[] = [
  { id: 'mockbookedorder', host: 'dex.decred.org:7232', marketName: 'dcr_btc', type: 'limit', sell: false, status: 'booked', quantity: 5e8, filled: 1e8, rate: 52000 },
  { id: 'mockexecorder', host: 'dex.decred.org:7232', marketName: 'dcr_btc', type: 'limit', sell: true, status: 'executed', quantity: 2e8, filled: 2e8, rate: 53000 },
];
