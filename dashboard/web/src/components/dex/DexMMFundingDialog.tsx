// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Wallet, X } from 'lucide-react';
import {
  getDexWallets,
  type DexAsset,
  type DexMarket,
  type DexWalletState,
  type MMAllocation,
  type MMBotConfig,
  type MMMarketReport,
} from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { fmtAmt } from './dexFormat';
import { toAtoms } from './dexMMConfig';
import { suggestedAllocation, TRANSFER_FACTOR } from './dexMMAlloc';

// DexMMFundingDialog collects the allocation for starting a bot. Per v1.0.6,
// allocation is supplied at start time (mm.StartConfig), not stored in the bot
// config. It pre-fills bisonw's suggested allocation (book inventory + order
// reserves + booking/swap fees + slippage, sized to the bot's placements) for
// each asset, including a token's fee asset, and flags amounts that exceed the
// available DEX balance. Amounts stay editable and are sent as atoms.
export const DexMMFundingDialog = ({
  market,
  config,
  report,
  catalog,
  needsCex,
  busy,
  onConfirm,
  onCancel,
}: {
  market: DexMarket;
  config: MMBotConfig;
  report: MMMarketReport | null;
  catalog: DexAsset[];
  needsCex: boolean;
  busy: boolean;
  onConfirm: (alloc: MMAllocation, autoRebalance?: { minBaseTransfer: number; minQuoteTransfer: number }) => void;
  onCancel: () => void;
}) => {
  const [wallets, setWallets] = useState<DexWalletState[]>([]);
  const [amts, setAmts] = useState<Record<string, string>>({});
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    getDexWallets()
      .then(setWallets)
      .catch(() => {});
  }, []);

  const suggested = useMemo(
    () => suggestedAllocation(config, market, report, catalog),
    [config, market, report, catalog],
  );

  // metaFor resolves an asset's symbol + conversion factor, preferring the
  // suggestion's catalog data and falling back to the market's base/quote.
  const metaFor = (id: number): { symbol: string; convFactor: number } =>
    suggested?.assets[id] ??
    (id === market.baseID
      ? { symbol: market.base, convFactor: market.baseConvFactor }
      : id === market.quoteID
        ? { symbol: market.quote, convFactor: market.quoteConvFactor }
        : { symbol: String(id), convFactor: 1 });

  const dexIDs = suggested ? Object.keys(suggested.dex).map(Number) : [market.baseID, market.quoteID];
  const cexIDs = needsCex
    ? suggested && Object.keys(suggested.cex).length
      ? Object.keys(suggested.cex).map(Number)
      : [market.baseID, market.quoteID]
    : [];

  // Pre-fill each field with the suggested conventional amount once the
  // suggestion (which needs the market report's fees) is available.
  useEffect(() => {
    if (!suggested) return;
    const init: Record<string, string> = {};
    const set = (venue: string, src: Record<number, number>, id: number) => {
      const atoms = src[id] ?? 0;
      const conv = metaFor(id).convFactor || 1;
      init[`${venue}:${id}`] = atoms > 0 ? String(Number((atoms / conv).toFixed(8))) : '0';
    };
    Object.keys(suggested.dex).forEach((id) => set('dex', suggested.dex, Number(id)));
    Object.keys(suggested.cex).forEach((id) => set('cex', suggested.cex, Number(id)));
    setAmts(init);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [suggested]);

  // DexWalletState.available is already in conventional units (the backend
  // converts it), the same as every other wallet view.
  const availConv = (assetID: number): number | null => {
    const w = wallets.find((wl) => wl.assetID === assetID);
    return w ? w.available : null;
  };

  const confirm = () => {
    const alloc: MMAllocation = { dex: {}, cex: {} };
    const fill = (venue: string, ids: number[], target: Record<number, number>) => {
      ids.forEach((id) => {
        const atoms = toAtoms(metaFor(id).convFactor, amts[`${venue}:${id}`] ?? '0');
        if (atoms > 0) target[id] = atoms;
      });
    };
    fill('dex', dexIDs, alloc.dex);
    if (needsCex) fill('cex', cexIDs, alloc.cex);
    if (Object.keys(alloc.dex).length === 0 && Object.keys(alloc.cex).length === 0) {
      setErr('Allocate funds on at least one side before starting.');
      return;
    }
    // For CEX bots, enable auto-rebalance with a minimum transfer sized off a
    // lot so the bot keeps inventory balanced without churning on dust.
    const autoRebalance =
      needsCex && suggested
        ? {
            minBaseTransfer: Math.round(TRANSFER_FACTOR * market.lotSize),
            minQuoteTransfer: Math.round(TRANSFER_FACTOR * suggested.quoteLot),
          }
        : undefined;
    onConfirm(alloc, autoRebalance);
  };

  const inputCls =
    'w-full px-2.5 py-1.5 rounded-lg bg-background border text-sm font-mono focus:outline-none focus:border-primary';

  const field = (venue: string, id: number) => {
    const meta = metaFor(id);
    const key = `${venue}:${id}`;
    const value = amts[key] ?? '0';
    const av = venue === 'dex' ? availConv(id) : null;
    const over = av !== null && (Number(value) || 0) > av;
    return (
      <label key={key} className="flex flex-col gap-1">
        <span className="flex items-center justify-between text-[11px] uppercase tracking-wider text-muted-foreground/70">
          <span className="flex items-center gap-1.5 normal-case">
            <CoinIcon symbol={meta.symbol} className="h-3.5 w-3.5" />
            {meta.symbol}
          </span>
          {av !== null && (
            <button
              type="button"
              onClick={() => setAmts((a) => ({ ...a, [key]: fmtAmt(av, 8).replace(/,/g, '') }))}
              className="text-primary hover:underline normal-case"
            >
              max {fmtAmt(av, 4)}
            </button>
          )}
        </span>
        <input
          value={value}
          onChange={(e) => setAmts((a) => ({ ...a, [key]: e.target.value }))}
          className={`${inputCls} ${over ? 'border-destructive' : 'border-border'}`}
        />
        {over && <span className="text-[10px] text-destructive">Exceeds available balance</span>}
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
          {suggested
            ? 'Suggested amounts cover the bot’s orders, reserves and fees. Adjust if needed. Starting trades real funds.'
            : 'Set how much to reserve for this bot. Starting places orders and trades real funds.'}
        </p>

        <div className="space-y-3">
          <div className="text-[11px] uppercase tracking-wider text-muted-foreground/70">DEX allocation</div>
          <div className="grid grid-cols-2 gap-3">{dexIDs.map((id) => field('dex', id))}</div>
          {needsCex && cexIDs.length > 0 && (
            <>
              <div className="text-[11px] uppercase tracking-wider text-muted-foreground/70">CEX allocation</div>
              <div className="grid grid-cols-2 gap-3">{cexIDs.map((id) => field('cex', id))}</div>
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
