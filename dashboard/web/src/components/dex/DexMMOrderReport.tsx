// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AlertTriangle, X } from 'lucide-react';
import type { DexMarket, MMOrderReport } from '../../services/dcrdexApi';
import { convRate, fmtAmt, fmtPrice } from './dexFormat';
import { botProblemMessages } from './dexMMProblems';

type AssetInfo = (assetID: number) => { symbol: string; convFactor: number };

// DexMMOrderReport is a modal detailing one side's per-epoch placement report
// (bisonw's market-maker order report): a per-asset balances table (available /
// required / used / remaining, atoms converted with the asset catalog factors)
// and a placements table (price, target/working/sent lots, and the reason any
// placement was skipped). Opened from the buys/sells badge on DexMMActivity.
export const DexMMOrderReport = ({
  report,
  side,
  market,
  assetOf,
  onClose,
}: {
  report: MMOrderReport;
  side: string;
  market?: DexMarket;
  assetOf: AssetInfo;
  onClose: () => void;
}) => {
  const symbolOf = (id: number) => assetOf(id).symbol;
  const conv = (id: number, atoms: number) => {
    const { convFactor } = assetOf(id);
    return fmtAmt(atoms / (convFactor || 1), 6);
  };
  const priceOf = (rate: number): string =>
    market ? fmtPrice(convRate(rate, market.baseConvFactor, market.quoteConvFactor), market.quote) : String(rate);

  const ids = Array.from(
    new Set<number>(
      [
        ...Object.keys(report.availableDexBals ?? {}),
        ...Object.keys(report.requiredDexBals ?? {}),
        ...Object.keys(report.usedDexBals ?? {}),
        ...Object.keys(report.remainingDexBals ?? {}),
      ].map(Number),
    ),
  ).sort((a, b) => a - b);

  // A DEX buy is hedged by a CEX sell of base; a DEX sell by a CEX buy paid in
  // quote. Convert the CEX columns with that counter-asset's factor.
  const cexAssetID = market ? (side === 'Buy' ? market.baseID : market.quoteID) : undefined;
  const cexConv = (atoms: number) => (cexAssetID !== undefined ? conv(cexAssetID, atoms) : fmtAmt(atoms / 1e8, 6));

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div
        className="w-full max-w-2xl max-h-[85vh] overflow-y-auto rounded-xl border border-border/60 bg-card shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="sticky top-0 flex items-center justify-between px-4 py-3 border-b border-border/50 bg-card">
          <h3 className="font-semibold">
            {side} order report
            {market && (
              <span className="text-muted-foreground font-normal">
                {' '}
                {market.base}/{market.quote}
              </span>
            )}
          </h3>
          <button type="button" onClick={onClose} className="p-1 rounded-lg hover:bg-muted/20 text-muted-foreground">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="p-4 space-y-5">
          <div className="space-y-2">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground/60">Balances</div>
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-left text-muted-foreground border-b border-border/40">
                    <th className="py-1 pr-2 font-medium">Asset</th>
                    <th className="py-1 px-2 font-medium text-right">Available</th>
                    <th className="py-1 px-2 font-medium text-right">Required</th>
                    <th className="py-1 px-2 font-medium text-right">Used</th>
                    <th className="py-1 pl-2 font-medium text-right">Remaining</th>
                  </tr>
                </thead>
                <tbody className="font-mono tabular-nums">
                  {ids.map((id) => {
                    const avail = report.availableDexBals?.[id]?.available ?? 0;
                    const req = report.requiredDexBals?.[id] ?? 0;
                    return (
                      <tr key={id} className="border-b border-border/20">
                        <td className="py-1 pr-2 font-sans">{symbolOf(id)}</td>
                        <td className={`py-1 px-2 text-right ${req > avail ? 'text-warning' : ''}`}>
                          {conv(id, avail)}
                        </td>
                        <td className="py-1 px-2 text-right">{conv(id, req)}</td>
                        <td className="py-1 px-2 text-right">{conv(id, report.usedDexBals?.[id] ?? 0)}</td>
                        <td className="py-1 pl-2 text-right">{conv(id, report.remainingDexBals?.[id] ?? 0)}</td>
                      </tr>
                    );
                  })}
                  {report.availableCexBal && (
                    <tr className="border-b border-border/20">
                      <td className="py-1 pr-2 font-sans">
                        CEX{cexAssetID !== undefined ? ` ${symbolOf(cexAssetID)}` : ''}
                      </td>
                      <td className="py-1 px-2 text-right">{cexConv(report.availableCexBal.available)}</td>
                      <td className="py-1 px-2 text-right">{cexConv(report.requiredCexBal)}</td>
                      <td className="py-1 px-2 text-right">{cexConv(report.usedCexBal)}</td>
                      <td className="py-1 pl-2 text-right">{cexConv(report.remainingCexBal)}</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>

          <div className="space-y-2">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground/60">
              Placements ({report.placements?.length ?? 0})
            </div>
            {report.placements?.length ? (
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="text-left text-muted-foreground border-b border-border/40">
                      <th className="py-1 pr-2 font-medium">#</th>
                      <th className="py-1 px-2 font-medium text-right">Price</th>
                      <th className="py-1 px-2 font-medium text-right">Lots</th>
                      <th className="py-1 px-2 font-medium text-right">Book</th>
                      <th className="py-1 px-2 font-medium text-right">Sent</th>
                      <th className="py-1 pl-2 font-medium">Status</th>
                    </tr>
                  </thead>
                  <tbody className="font-mono tabular-nums">
                    {report.placements.map((p, i) => {
                      const errs = botProblemMessages(p.error, { dexHost: 'the exchange', symbolOf });
                      const placed = p.standingLots + p.orderedLots >= p.lots;
                      return (
                        <tr key={i} className="border-b border-border/20">
                          <td className="py-1 pr-2">{i + 1}</td>
                          <td className="py-1 px-2 text-right">{priceOf(p.rate)}</td>
                          <td className="py-1 px-2 text-right">{p.lots}</td>
                          <td className="py-1 px-2 text-right">{p.standingLots}</td>
                          <td className="py-1 px-2 text-right">{p.orderedLots}</td>
                          <td className="py-1 pl-2 font-sans">
                            {errs.length ? (
                              <span className="inline-flex items-center gap-1 text-warning" title={errs.join('; ')}>
                                <AlertTriangle className="h-3 w-3 shrink-0" />
                                {errs[0]}
                              </span>
                            ) : placed ? (
                              <span className="text-success">placed</span>
                            ) : (
                              <span className="text-muted-foreground">pending</span>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="text-xs text-muted-foreground">No placements this epoch.</div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
