// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Globe } from 'lucide-react';
import type { DexMarket, MMMarketReport } from '../../services/dcrdexApi';
import { fmtPrice, fmtUsd } from './dexFormat';

// DexMMOracleTable shows the external price oracles bisonw aggregates for the
// market: the consolidated price plus each oracle's volume and best buy/sell.
// It mirrors the oracles table on the v1.0.6 market-maker settings page.
export const DexMMOracleTable = ({ market, report }: { market: DexMarket; report: MMMarketReport | null }) => {
  const oracles = report?.oracles ?? [];
  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-3">
      <div className="flex items-center justify-between mb-2">
        <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70 flex items-center gap-1.5">
          <Globe className="h-3.5 w-3.5" /> Oracle price
        </span>
        {report && report.price > 0 && (
          <span className="font-mono text-sm">
            {fmtPrice(report.price, market.quote)} {market.quote}
          </span>
        )}
      </div>
      {oracles.length === 0 ? (
        <div className="text-xs text-muted-foreground py-1">
          {report ? 'No oracle data for this market.' : 'Loading oracle data...'}
        </div>
      ) : (
        <table className="w-full text-xs">
          <thead>
            <tr className="text-muted-foreground/70 text-left">
              <th className="font-normal py-1">Exchange</th>
              <th className="font-normal py-1 text-right">USD volume</th>
              <th className="font-normal py-1 text-right">Best buy</th>
              <th className="font-normal py-1 text-right">Best sell</th>
            </tr>
          </thead>
          <tbody className="font-mono">
            {oracles.map((o) => (
              <tr key={o.host} className="border-t border-border/30">
                <td className="py-1 font-sans">{o.host}</td>
                <td className="py-1 text-right">{fmtUsd(o.usdVol)}</td>
                <td className="py-1 text-right text-success">{fmtPrice(o.bestBuy, market.quote)}</td>
                <td className="py-1 text-right text-destructive">{fmtPrice(o.bestSell, market.quote)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
};
