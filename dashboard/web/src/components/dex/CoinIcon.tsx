// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// CoinIcon renders an upstream DCRDEX coin logo, falling back to the
// single-letter placeholder icon when a symbol has no dedicated logo. Token
// symbols use dot-notation for the chain (e.g. usdt.polygon); the icon is keyed
// by the token part before the dot.
export const CoinIcon = ({ symbol, className = '' }: { symbol: string; className?: string }) => {
  const s = (symbol || '').toLowerCase().split('.')[0];
  return (
    <img
      src={`/images/dex-coins/${s}.png`}
      alt={symbol}
      className={`h-5 w-5 rounded-full ${className}`}
      onError={(e) => {
        const img = e.currentTarget;
        const fallback = `/images/dex-coins/${s[0] || 'a'}.png`;
        if (!img.src.endsWith(fallback)) img.src = fallback;
      }}
    />
  );
};
