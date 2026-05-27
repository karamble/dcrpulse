// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// CexIcon renders a centralized-exchange logo for the market-maker arb bots.
// v1.0.6 supports Binance and BinanceUS; the icon is keyed by the lowercased CEX
// name, falling back to a generic placeholder when absent.
export const CexIcon = ({ name, className = '' }: { name: string; className?: string }) => {
  const s = (name || '').toLowerCase();
  return (
    <img
      src={`/images/dex-exchanges/${s}.png`}
      alt={name}
      className={`h-5 w-5 rounded ${className}`}
      onError={(e) => {
        const img = e.currentTarget;
        const fallback = '/images/dex-coins/a.png';
        if (!img.src.endsWith(fallback)) img.src = fallback;
      }}
    />
  );
};

// CEX_DISPLAY maps the bisonw CEX name to a human label. Keep in sync with
// decred.org/dcrdex/client/mm/libxc (v1.0.6: Binance, BinanceUS).
export const CEX_DISPLAY: Record<string, string> = {
  Binance: 'Binance',
  BinanceUS: 'Binance US',
};

// SUPPORTED_CEXES is the list of CEX names the arb bots can use.
export const SUPPORTED_CEXES = ['Binance', 'BinanceUS'] as const;
