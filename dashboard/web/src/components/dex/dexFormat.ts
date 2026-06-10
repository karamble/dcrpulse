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

// fmtAmtParts splits an amount for the big-balance style used on the wallet
// page: integer + first two decimals render large, the remaining precision
// renders smaller and faded.
export const fmtAmtParts = (
  qty: number,
  decimals = 8,
): { integer: string; mainDecimals: string; extraDecimals: string } => {
  const [integerPart, decimalPart] = qty.toFixed(decimals).split('.');
  return {
    integer: parseInt(integerPart, 10).toLocaleString('en-US'),
    mainDecimals: decimalPart.substring(0, 2),
    extraDecimals: decimalPart.substring(2),
  };
};

export const fmtPct = (pct: number): string => `${pct >= 0 ? '+' : ''}${pct.toFixed(2)}%`;

export const fmtUsd = (v: number): string =>
  v.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 });

// fmtAge renders a compact relative age (s/m/h/d) from a unix-ms timestamp.
export const fmtAge = (ms: number): string => {
  if (!ms) return '-';
  const s = Math.max(0, Math.floor((Date.now() - ms) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
};

// usdRateFor returns the USD price for an asset symbol from a symbol->price map
// (token symbols like "usdc.eth" resolve by their base symbol). 0 when absent.
export const usdRateFor = (symbol: string, rates: Record<string, number> | null): number => {
  if (!rates) return 0;
  const s = symbol.toLowerCase().split('.')[0];
  return rates[s] || 0;
};

// RateEncodingFactor mirrors bisonw's OrderUtil.RateEncodingFactor: the DEX
// message rate is the conventional price scaled by this factor and adjusted by
// the base/quote conversion factors.
export const RateEncodingFactor = 1e8;

// convQty converts an atomic base quantity to conventional units.
export const convQty = (atomic: number, baseConvFactor: number): number =>
  baseConvFactor > 0 ? atomic / baseConvFactor : 0;

// convRate converts an atomic message-rate to a conventional price.
export const convRate = (msgRate: number, baseConvFactor: number, quoteConvFactor: number): number => {
  const f = (RateEncodingFactor / baseConvFactor) * quoteConvFactor;
  return f > 0 ? msgRate / f : 0;
};
