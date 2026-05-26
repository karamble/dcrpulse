// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, AlertTriangle, ShieldCheck } from 'lucide-react';
import { getDexConfig, postDexBond, type DexConfig } from '../../services/dcrdexApi';

interface DexRegisterProps {
  host: string;
  onRegistered: () => void;
}

// DexRegister shows a DEX server's bond requirements and lets the user register
// by posting a fidelity bond. Posting spends real DCR, so it is behind an
// explicit confirmation step. All amounts come from the backend already in DCR.
export const DexRegister = ({ host, onRegistered }: DexRegisterProps) => {
  const [cfg, setCfg] = useState<DexConfig | null>(null);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [tiers, setTiers] = useState(1);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    getDexConfig(host)
      .then(setCfg)
      .catch((e: any) =>
        setLoadErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Failed to load DEX config'),
      );
  }, [host]);

  if (loadErr) {
    return (
      <div className="mx-4 p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
        <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
        <span className="break-words">Could not reach {host}: {loadErr}</span>
      </div>
    );
  }
  if (!cfg) {
    return (
      <div className="flex justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  const bondAtoms = cfg.bondPerTierAtoms * tiers;
  const bondDcr = cfg.bondPerTierDcr * tiers;

  const post = async () => {
    setBusy(true);
    setErr(null);
    try {
      await postDexBond(host, bondAtoms);
      onRegistered();
    } catch (e: any) {
      setErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Bond posting failed');
      setBusy(false);
      setConfirming(false);
    }
  };

  const cell = (label: string, value: string) => (
    <div className="p-3 rounded-lg bg-muted/10 border border-border/50">
      <div className="text-xs text-muted-foreground uppercase tracking-wide">{label}</div>
      <div className="font-mono mt-1">{value}</div>
    </div>
  );

  return (
    <div className="mx-4 max-w-2xl space-y-5 p-6 rounded-xl bg-gradient-card border border-border/50">
      <div className="flex items-center gap-2">
        <ShieldCheck className="h-5 w-5 text-primary" />
        <h2 className="text-lg font-semibold">Register with {host}</h2>
      </div>

      <p className="text-sm text-muted-foreground">
        To trade, DCRDEX requires a refundable fidelity bond locked for a set period. Fund your
        wallet's <span className="font-mono">dex</span> account first, then post the bond below.
      </p>

      <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 text-sm">
        {cell('Bond per tier', `${cfg.bondPerTierDcr.toFixed(2)} DCR`)}
        {cell('Confirmations', String(cfg.bondConfs))}
        {cell('Bond expiry', cfg.bondExpiryDays ? `${cfg.bondExpiryDays} days` : 'n/a')}
        {cell('Markets', String(cfg.marketCount))}
      </div>

      <div>
        <label className="block text-xs text-muted-foreground mb-1" htmlFor="dex-tiers">
          Tiers
        </label>
        <input
          id="dex-tiers"
          type="number"
          min={1}
          value={tiers}
          onChange={(e) => setTiers(Math.max(1, parseInt(e.target.value || '1', 10)))}
          className="w-28 px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
        />
        <span className="ml-3 text-sm text-muted-foreground">
          Total bond: <span className="font-mono text-foreground">{bondDcr.toFixed(2)} DCR</span>
        </span>
      </div>

      <div className="p-3 rounded-lg bg-warning/10 border border-warning/30 text-sm text-warning flex items-start gap-2">
        <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
        <span>
          Posting a bond spends real DCR from the dex account on mainnet. The amount is locked until
          the bond expires, then refundable.
        </span>
      </div>

      {err && (
        <div className="p-3 rounded-lg bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      {!confirming ? (
        <button
          type="button"
          disabled={bondAtoms === 0}
          onClick={() => setConfirming(true)}
          className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2.5 transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Post bond &amp; register
        </button>
      ) : (
        <div className="flex gap-3">
          <button
            type="button"
            disabled={busy}
            onClick={() => setConfirming(false)}
            className="flex-1 px-4 py-2.5 border border-border rounded-lg hover:bg-background/50 transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={post}
            className="flex-1 bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2.5 transition-colors hover:bg-primary/90 disabled:opacity-50"
          >
            {busy ? 'Posting…' : `Confirm: post ${bondDcr.toFixed(2)} DCR`}
          </button>
        </div>
      )}
    </div>
  );
};
