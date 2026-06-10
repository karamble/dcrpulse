// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Plus } from 'lucide-react';
import { getDexWallets, getDexAssetCatalog, getDexRates, type DexAsset, type DexRates, type DexWalletState } from '../../services/dcrdexApi';
import { fmtAmt, fmtUsd, usdRateFor } from './dexFormat';
import { CoinIcon } from './CoinIcon';
import { DexWalletDetail } from './DexWalletDetail';
import { DexAddWallet } from './DexAddWallet';
import { useDexRefreshOnNotes } from './DexLiveProvider';

const statusDot = (w: DexWalletState) => {
  if (w.disabled || !w.running) return 'bg-muted-foreground/40';
  return w.synced ? 'bg-success' : 'bg-warning';
};

// DexWalletsPanel is the master-detail Wallets view: a selector of the
// DCRDEX-managed wallets plus an add-wallet flow (any supported coin), and a
// detail pane for the selected wallet.
export const DexWalletsPanel = () => {
  const [wallets, setWallets] = useState<DexWalletState[] | null>(null);
  const [catalog, setCatalog] = useState<DexAsset[]>([]);
  const [rates, setRates] = useState<DexRates | null>(null);
  const [selected, setSelected] = useState<number | null>(null);
  const [adding, setAdding] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const refresh = () => {
    getDexWallets()
      .then((w) => {
        setWallets(w);
        setErr(null);
        setSelected((cur) => (cur != null && w.some((x) => x.assetID === cur) ? cur : w[0]?.assetID ?? null));
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Failed to load wallets'));
  };

  useEffect(() => {
    getDexAssetCatalog().then(setCatalog).catch(() => {});
    getDexRates().then(setRates).catch(() => {});
    refresh();
    const id = window.setInterval(refresh, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useDexRefreshOnNotes(['balance', 'walletstate', 'walletconfig', 'walletnote', 'createwallet'], refresh);

  if (err) {
    return (
      <div className="px-3 lg:px-4">
        <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      </div>
    );
  }
  if (wallets === null) {
    return (
      <div className="min-h-[30vh] flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  const sel = wallets.find((w) => w.assetID === selected) || null;

  return (
    <div className="px-3 lg:px-4 grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-3">
      <div className="space-y-2">
        {wallets.map((w) => (
          <button
            key={w.assetID}
            type="button"
            onClick={() => {
              setSelected(w.assetID);
              setAdding(false);
            }}
            className={`w-full flex items-center gap-2 p-3 rounded-xl border transition-colors text-left ${
              !adding && w.assetID === selected ? 'bg-card border-primary/50' : 'bg-gradient-card border-border/50 hover:border-border'
            }`}
          >
            <CoinIcon symbol={w.symbol} />
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-1.5 text-sm font-medium">
                <span className={`h-1.5 w-1.5 rounded-full ${statusDot(w)}`} />
                {w.symbol}
              </span>
              {w.running && !w.synced ? (
                <span className="block mt-1">
                  <span className="flex items-center justify-between text-[10px] text-warning">
                    <span>Syncing</span>
                    <span className="font-mono tabular-nums">{Math.round((w.syncProgress || 0) * 100)}%</span>
                  </span>
                  <span className="mt-0.5 block h-1 rounded bg-muted/50 overflow-hidden">
                    <span
                      className="block h-full bg-warning transition-[width]"
                      style={{ width: `${Math.round((w.syncProgress || 0) * 100)}%` }}
                    />
                  </span>
                </span>
              ) : (
                <span className="block text-xs font-mono tabular-nums text-muted-foreground">
                  {fmtAmt(w.total, 4)}
                  {usdRateFor(w.symbol, rates) > 0 && (
                    <span className="text-muted-foreground/60"> &middot; {fmtUsd(w.total * usdRateFor(w.symbol, rates))}</span>
                  )}
                </span>
              )}
            </span>
          </button>
        ))}
        <button
          type="button"
          onClick={() => setAdding(true)}
          className={`w-full flex items-center justify-center gap-1.5 p-3 rounded-xl border border-dashed text-sm transition-colors ${
            adding ? 'border-primary text-primary' : 'border-border/60 text-muted-foreground hover:text-foreground hover:border-border'
          }`}
        >
          <Plus className="h-4 w-4" /> Add wallet
        </button>
      </div>

      <div className="min-w-0">
        {adding ? (
          <DexAddWallet
            catalog={catalog}
            existingIDs={wallets.map((w) => w.assetID)}
            onCancel={() => setAdding(false)}
            onCreated={() => {
              setAdding(false);
              refresh();
            }}
          />
        ) : sel ? (
          <DexWalletDetail wallet={sel} rates={rates} catalog={catalog} onChanged={refresh} />
        ) : (
          <div className="py-12 text-center text-sm text-muted-foreground">No wallets yet. Add one to get started.</div>
        )}
      </div>
    </div>
  );
};
