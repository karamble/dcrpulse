// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, ArrowDownUp, X } from 'lucide-react';
import {
  getMMAvailableBalances,
  updateRunningMMBotInventory,
  type MMBalanceDiffs,
  type MMBotStatus,
} from '../../services/dcrdexApi';
import type { AssetInfo } from './DexMMActivity';
import { CoinIcon } from './CoinIcon';
import { fmtAmt } from './dexFormat';
import { toAtoms } from './dexMMConfig';
import { apiError } from '../../utils/apiError';

// DexMMInventoryDialog moves funds between a running bot's allocation and the
// wallet without stopping it, which is the point: a stop cancels the bot's book
// and a start replaces it, while this leaves the standing orders alone.
//
// Amounts are signed deltas in conventional units - positive funds the bot,
// negative returns funds. bisonw clamps a withdrawal larger than the bot holds
// to what is available and still reports success, so the balances are refetched
// after applying rather than assumed.
export const DexMMInventoryDialog = ({
  bot,
  assetOf,
  onApplied,
  onClose,
}: {
  bot: MMBotStatus;
  assetOf: AssetInfo;
  onApplied: () => void;
  onClose: () => void;
}) => {
  const [deltas, setDeltas] = useState<Record<string, string>>({});
  const [avail, setAvail] = useState<Record<string, number> | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const { host, baseID, quoteID, cexName } = bot.config;
  const dexIDs = useMemo(() => Object.keys(bot.runStats?.dexBalances ?? {}).map(Number), [bot.runStats]);
  const cexIDs = useMemo(
    () => (cexName ? Object.keys(bot.runStats?.cexBalances ?? {}).map(Number) : []),
    [bot.runStats, cexName],
  );

  // The wallet side of the ceiling: what the daemon says is still free to
  // allocate. Advisory - the deposit is sized against it but not blocked on it.
  useEffect(() => {
    getMMAvailableBalances(host, baseID, quoteID)
      .then((b) => setAvail(b.dexBalances ?? {}))
      .catch(() => setAvail(null));
  }, [host, baseID, quoteID]);

  const apply = async () => {
    const collect = (ids: number[], venue: string): MMBalanceDiffs => {
      const out: MMBalanceDiffs = {};
      ids.forEach((id) => {
        const atoms = toAtoms(assetOf(id).convFactor || 1, deltas[`${venue}:${id}`] ?? '');
        if (atoms !== 0) out[id] = atoms;
      });
      return out;
    };
    const dexDiffs = collect(dexIDs, 'dex');
    const cexDiffs = collect(cexIDs, 'cex');
    if (Object.keys(dexDiffs).length === 0 && Object.keys(cexDiffs).length === 0) {
      setErr('Enter an amount to add or remove.');
      return;
    }
    setBusy(true);
    setErr(null);
    try {
      await updateRunningMMBotInventory(host, baseID, quoteID, dexDiffs, cexDiffs);
      onApplied();
      onClose();
    } catch (e: any) {
      setErr(apiError(e, 'Failed to adjust inventory'));
    } finally {
      setBusy(false);
    }
  };

  const row = (venue: 'dex' | 'cex', id: number) => {
    const { symbol, convFactor } = assetOf(id);
    const conv = convFactor || 1;
    const key = `${venue}:${id}`;
    const bal = (venue === 'dex' ? bot.runStats?.dexBalances : bot.runStats?.cexBalances)?.[id];
    const botHas = (bal?.available ?? 0) / conv;
    const walletHas = venue === 'dex' && avail ? (avail[id] ?? 0) / conv : null;
    const value = deltas[key] ?? '';
    const delta = Number(value) || 0;
    const overWithdraw = delta < 0 && -delta > botHas;
    const overDeposit = walletHas !== null && delta > walletHas;
    return (
      <div key={key} className="grid grid-cols-[1fr_auto] items-center gap-3">
        <div className="flex flex-col gap-0.5 min-w-0">
          <span className="flex items-center gap-1.5 text-sm">
            <CoinIcon symbol={symbol} className="h-3.5 w-3.5" />
            {symbol}
          </span>
          <span className="text-[10px] text-muted-foreground font-mono tabular-nums">
            bot {fmtAmt(botHas, 4)}
            {walletHas !== null && ` · wallet ${fmtAmt(walletHas, 4)}`}
          </span>
        </div>
        <div className="flex flex-col items-end gap-1">
          <input
            value={value}
            placeholder="0"
            inputMode="decimal"
            onChange={(e) => setDeltas((d) => ({ ...d, [key]: e.target.value }))}
            className={`w-36 px-2.5 py-1.5 rounded-lg bg-background border text-sm font-mono text-right focus:outline-none focus:border-primary ${
              overWithdraw || overDeposit ? 'border-destructive' : 'border-border'
            }`}
          />
          <div className="flex gap-2 text-[10px]">
            {walletHas !== null && walletHas > 0 && (
              <button
                type="button"
                onClick={() => setDeltas((d) => ({ ...d, [key]: fmtAmt(walletHas, 8).replace(/,/g, '') }))}
                className="text-primary hover:underline"
              >
                add all
              </button>
            )}
            {botHas > 0 && (
              <button
                type="button"
                onClick={() => setDeltas((d) => ({ ...d, [key]: `-${fmtAmt(botHas, 8).replace(/,/g, '')}` }))}
                className="text-primary hover:underline"
              >
                remove all
              </button>
            )}
          </div>
          {overWithdraw && <span className="text-[10px] text-destructive">More than the bot holds</span>}
          {overDeposit && !overWithdraw && (
            <span className="text-[10px] text-destructive">More than the wallet has free</span>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-md rounded-xl border border-border bg-card p-5 space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="font-semibold flex items-center gap-2">
            <ArrowDownUp className="h-4 w-4 text-primary" />
            Adjust inventory
          </h3>
          <button type="button" onClick={onClose} className="p-1 rounded hover:bg-muted/20 text-muted-foreground">
            <X className="h-4 w-4" />
          </button>
        </div>

        <p className="text-xs text-muted-foreground">
          Move funds between the bot and your wallet while it keeps running. A positive amount funds the bot, a
          negative one returns funds. Its orders stay on the book.
        </p>

        <div className="space-y-3">
          <div className="text-[11px] uppercase tracking-wider text-muted-foreground/70">DEX</div>
          {dexIDs.map((id) => row('dex', id))}
          {cexIDs.length > 0 && (
            <>
              <div className="text-[11px] uppercase tracking-wider text-muted-foreground/70">{cexName}</div>
              {cexIDs.map((id) => row('cex', id))}
            </>
          )}
        </div>

        <p className="text-[11px] text-muted-foreground">
          A removal larger than the bot holds is reduced to what is available rather than refused.
        </p>

        {err && (
          <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}

        <div className="flex gap-2 justify-end">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 rounded-lg border border-border text-sm hover:bg-muted/10"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={apply}
            className="px-4 py-2 rounded-lg bg-primary text-white text-sm font-medium hover:bg-primary/90 disabled:opacity-50"
          >
            {busy ? 'Applying...' : 'Apply'}
          </button>
        </div>
      </div>
    </div>
  );
};
