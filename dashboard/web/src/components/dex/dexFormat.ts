// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Display helpers for the DEX terminal. Prices in USD-quoted markets read more
// naturally with fewer decimals; crypto-quoted markets keep full atom-ish
// precision.
export const priceDecimals = (quote: string) =>
  quote.toUpperCase().startsWith('USD') ? 4 : 8;

export const fmtPrice = (rate: number, quote: string): string =>
  rate.toLocaleString('en-US', {
    minimumFractionDigits: priceDecimals(quote),
    maximumFractionDigits: priceDecimals(quote),
  });

export const fmtAmt = (qty: number, max = 4): string =>
  qty.toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: max });

export const fmtPct = (pct: number): string => `${pct >= 0 ? '+' : ''}${pct.toFixed(2)}%`;

export const fmtUsd = (v: number): string =>
  v.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 });
