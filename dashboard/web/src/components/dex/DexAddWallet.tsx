// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo, useState } from 'react';
import { AlertCircle, ArrowLeft } from 'lucide-react';
import { createDexAssetWallet, type DexAsset, type DexWalletDefinition } from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { DexWalletConfigForm } from './DexWalletConfigForm';

interface Creatable {
  id: number;
  symbol: string;
  name: string;
  defs: DexWalletDefinition[];
  isToken: boolean;
}

interface Props {
  catalog: DexAsset[];
  existingIDs: number[];
  onCreated: () => void;
  onCancel: () => void;
}

// DexAddWallet drives wallet creation for any supported asset: pick an asset,
// pick a wallet type, fill the schema-driven config form, and create. Tokens are
// included (they reuse their parent wallet's backend).
export const DexAddWallet = ({ catalog, existingIDs, onCreated, onCancel }: Props) => {
  const [sel, setSel] = useState<Creatable | null>(null);
  const [typeIdx, setTypeIdx] = useState(0);
  const [config, setConfig] = useState<Record<string, string>>({});
  const [walletPass, setWalletPass] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const creatable = useMemo<Creatable[]>(() => {
    const have = new Set(existingIDs);
    const out: Creatable[] = [];
    for (const a of catalog) {
      if (!have.has(a.id)) {
        out.push({ id: a.id, symbol: a.symbol, name: a.name, defs: a.availableWallets, isToken: false });
      }
      for (const t of a.tokens || []) {
        if (!have.has(t.id)) {
          out.push({ id: t.id, symbol: t.symbol, name: t.name, defs: [t.definition], isToken: true });
        }
      }
    }
    return out;
  }, [catalog, existingIDs]);

  const pick = (c: Creatable) => {
    setSel(c);
    setTypeIdx(0);
    setConfig({});
    setWalletPass('');
    setErr(null);
  };

  const create = async () => {
    if (!sel) return;
    setBusy(true);
    setErr(null);
    try {
      // Seeded wallets (Native/SPV) derive their password from the app seed and
      // reject an external password; only external wallet types take one.
      const def = sel.defs[typeIdx];
      await createDexAssetWallet(sel.id, def.type, config, def.seeded ? '' : walletPass);
      onCreated();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Wallet creation failed');
      setBusy(false);
    }
  };

  if (!sel) {
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 transition-colors"
          >
            <ArrowLeft className="h-4 w-4" />
            Back
          </button>
          <h2 className="font-semibold">Add a wallet</h2>
        </div>
        {creatable.length === 0 ? (
          <p className="text-sm text-muted-foreground">All supported assets already have a wallet.</p>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2">
            {creatable.map((c) => (
              <button
                key={c.id}
                type="button"
                onClick={() => pick(c)}
                className="flex items-center gap-2 p-3 rounded-xl bg-gradient-card border border-border/50 hover:border-primary/50 transition-colors text-left"
              >
                <CoinIcon symbol={c.symbol} />
                <span className="min-w-0">
                  <span className="block text-sm font-medium truncate">{c.name}</span>
                  <span className="block text-[11px] text-muted-foreground uppercase">
                    {c.symbol}
                    {c.isToken && ' token'}
                  </span>
                </span>
              </button>
            ))}
          </div>
        )}
      </div>
    );
  }

  const def = sel.defs[typeIdx];
  return (
    <div className="space-y-4 max-w-2xl">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => setSel(null)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Assets
        </button>
        <CoinIcon symbol={sel.symbol} />
        <h2 className="font-semibold">{sel.name}</h2>
      </div>

      {sel.defs.length > 1 && (
        <nav className="flex gap-2 border-b border-border">
          {sel.defs.map((d, i) => (
            <button
              key={d.type}
              type="button"
              onClick={() => {
                setTypeIdx(i);
                setConfig({});
              }}
              className={`px-3 py-1.5 -mb-px border-b-2 text-sm transition-colors ${
                i === typeIdx
                  ? 'border-primary text-primary font-semibold'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              }`}
            >
              {d.tab}
            </button>
          ))}
        </nav>
      )}

      <p className="text-sm text-muted-foreground">{def.description}</p>
      {def.guideLink && (
        <a href={def.guideLink} target="_blank" rel="noreferrer" className="text-xs text-primary hover:underline">
          Setup guide
        </a>
      )}

      <DexWalletConfigForm
        opts={def.configOpts}
        values={config}
        onChange={(k, v) => setConfig((c) => ({ ...c, [k]: v }))}
      />

      {def.seeded ? (
        <p className="text-[11px] text-muted-foreground">
          This built-in wallet is encrypted with your DCRDEX app password; no separate wallet password is needed.
        </p>
      ) : (
        <div>
          <label className="block text-xs text-muted-foreground mb-1">Wallet password</label>
          <input
            type="password"
            value={walletPass}
            onChange={(e) => setWalletPass(e.target.value)}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
          />
          <p className="text-[11px] text-muted-foreground mt-1">The external wallet's own passphrase.</p>
        </div>
      )}

      {err && (
        <div className="p-3 rounded-lg bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      <button
        type="button"
        disabled={busy}
        onClick={create}
        className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2.5 transition-colors hover:bg-primary/90 disabled:opacity-50"
      >
        {busy ? 'Creating...' : `Create ${sel.symbol.toUpperCase()} wallet`}
      </button>
    </div>
  );
};
