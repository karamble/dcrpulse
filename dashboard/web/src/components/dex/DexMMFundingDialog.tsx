// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Wallet, X } from 'lucide-react';
import { getDexWallets, type DexMarket, type DexWalletState, type MMAllocation } from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { fmtAmt } from './dexFormat';
import { toAtoms } from './dexMMConfig';

// DexMMFundingDialog collects the allocation for starting a bot. Per v1.0.6,
// allocation is supplied at start time (mm.StartConfig), not stored in the bot
// config. Amounts are conventional and converted to atoms; available DEX
// balances are shown for guidance.
export const DexMMFundingDialog = ({
  market,
  needsCex,
  busy,
  onConfirm,
  onCancel,
}: {
  market: DexMarket;
  needsCex: boolean;
  busy: boolean;
  onConfirm: (alloc: MMAllocation) => void;
  onCancel: () => void;
}) => {
  const [wallets, setWallets] = useState<DexWalletState[]>([]);
  const [dexBase, setDexBase] = useState('0');
  const [dexQuote, setDexQuote] = useState('0');
  const [cexBase, setCexBase] = useState('0');
  const [cexQuote, setCexQuote] = useState('0');
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    getDexWallets()
      .then(setWallets)
      .catch(() => {});
  }, []);

  const avail = (assetID: number, conv: number): string | null => {
    const w = wallets.find((wl) => wl.assetID === assetID);
    if (!w || conv <= 0) return null;
    return fmtAmt(w.available / conv, 4);
  };

  const confirm = () => {
    const alloc: MMAllocation = { dex: {}, cex: {} };
    const db = toAtoms(market.baseConvFactor, dexBase);
    const dq = toAtoms(market.quoteConvFactor, dexQuote);
    if (db > 0) alloc.dex[market.baseID] = db;
    if (dq > 0) alloc.dex[market.quoteID] = dq;
    if (needsCex) {
      const cb = toAtoms(market.baseConvFactor, cexBase);
      const cq = toAtoms(market.quoteConvFactor, cexQuote);
      if (cb > 0) alloc.cex[market.baseID] = cb;
      if (cq > 0) alloc.cex[market.quoteID] = cq;
    }
    if (Object.keys(alloc.dex).length === 0 && Object.keys(alloc.cex).length === 0) {
      setErr('Allocate funds on at least one side before starting.');
      return;
    }
    onConfirm(alloc);
  };

  const inputCls =
    'w-full px-2.5 py-1.5 rounded-lg bg-background border border-border text-sm font-mono focus:outline-none focus:border-primary';

  const allocField = (label: string, assetID: number, conv: number, value: string, set: (v: string) => void) => {
    const a = avail(assetID, conv);
    return (
      <label className="flex flex-col gap-1">
        <span className="flex items-center justify-between text-[11px] uppercase tracking-wider text-muted-foreground/70">
          <span>{label}</span>
          {a !== null && (
            <button type="button" onClick={() => set(a.replace(/,/g, ''))} className="text-primary hover:underline normal-case">
              max {a}
            </button>
          )}
        </span>
        <input value={value} onChange={(e) => set(e.target.value)} className={inputCls} />
      </label>
    );
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-md rounded-xl border border-border bg-card p-5 space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="font-semibold flex items-center gap-2">
            <Wallet className="h-4 w-4 text-primary" />
            Fund {market.base}/{market.quote} bot
          </h3>
          <button type="button" onClick={onCancel} className="p-1 rounded hover:bg-muted/20 text-muted-foreground">
            <X className="h-4 w-4" />
          </button>
        </div>

        <p className="text-xs text-muted-foreground">
          Set how much to reserve for this bot. Starting places orders and trades real funds.
        </p>

        <div className="space-y-3">
          <div className="text-[11px] uppercase tracking-wider text-muted-foreground/70 flex items-center gap-1.5">
            <CoinIcon symbol={market.base} className="h-4 w-4" /> DEX allocation
          </div>
          <div className="grid grid-cols-2 gap-3">
            {allocField(market.base, market.baseID, market.baseConvFactor, dexBase, setDexBase)}
            {allocField(market.quote, market.quoteID, market.quoteConvFactor, dexQuote, setDexQuote)}
          </div>
          {needsCex && (
            <>
              <div className="text-[11px] uppercase tracking-wider text-muted-foreground/70">CEX allocation</div>
              <div className="grid grid-cols-2 gap-3">
                {allocField(market.base, market.baseID, market.baseConvFactor, cexBase, setCexBase)}
                {allocField(market.quote, market.quoteID, market.quoteConvFactor, cexQuote, setCexQuote)}
              </div>
            </>
          )}
        </div>

        {err && (
          <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}

        <div className="flex gap-2 justify-end">
          <button type="button" onClick={onCancel} className="px-4 py-2 rounded-lg border border-border text-sm hover:bg-muted/10">
            Cancel
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={confirm}
            className="px-4 py-2 rounded-lg bg-success/90 text-white text-sm font-medium hover:bg-success disabled:opacity-50"
          >
            {busy ? 'Starting...' : 'Start bot'}
          </button>
        </div>
      </div>
    </div>
  );
};
