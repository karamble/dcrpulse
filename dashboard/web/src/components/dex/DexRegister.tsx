// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, AlertTriangle, Check, Copy, Info, ShieldCheck } from 'lucide-react';
import {
  discoverDexAccount,
  getDexConfig,
  getDexWallet,
  postDexBond,
  type DexConfig,
  type DexWalletInfo,
} from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { useDexConn, useDexRefreshOnNotes } from './DexLiveProvider';

interface DexRegisterProps {
  host: string;
  onRegistered: () => void;
}

// DexRegister shows a DEX server's markets and bond requirements and lets the
// user register by posting a fidelity bond. Posting spends real DCR, so it is
// gated on the dex account being funded and behind an explicit confirmation.
export const DexRegister = ({ host, onRegistered }: DexRegisterProps) => {
  const [cfg, setCfg] = useState<DexConfig | null>(null);
  const [wallet, setWallet] = useState<DexWalletInfo | null>(null);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [tiers, setTiers] = useState(1);
  const [copied, setCopied] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [discovering, setDiscovering] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const loadConfig = () =>
      getDexConfig(host)
        .then((c) => {
          if (!cancelled) setCfg(c);
        })
        .catch((e: any) => {
          if (!cancelled)
            setLoadErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Failed to load DEX config');
        });
    // After a seed restore the client has no record of this server; discover the
    // account first. If it already has a live bond, skip straight to trading.
    setDiscovering(true);
    discoverDexAccount(host)
      .then((r) => {
        if (cancelled) return;
        if (r.paid) {
          onRegistered();
          return;
        }
        setDiscovering(false);
        loadConfig();
      })
      .catch(() => {
        if (cancelled) return;
        setDiscovering(false);
        loadConfig();
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host]);

  const refresh = () => getDexWallet().then(setWallet).catch(() => {});
  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useDexRefreshOnNotes(['balance', 'walletstate', 'walletsync'], refresh);
  const conn = useDexConn(host);

  if (loadErr) {
    return (
      <div className="flex justify-center p-6">
        <div className="w-full max-w-2xl p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">Could not reach {host}: {loadErr}</span>
        </div>
      </div>
    );
  }
  if (discovering) {
    return (
      <div className="flex flex-col items-center justify-center py-16 gap-3 text-sm text-muted-foreground">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
        Checking for an existing registration...
      </div>
    );
  }
  if (!cfg) {
    return (
      <div className="flex justify-center py-16">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  const bondAtoms = cfg.bondPerTierAtoms * tiers;
  const bondDcr = cfg.bondPerTierDcr * tiers;
  // Prefer the live conn feed (pre-auth, so connection only) over the REST
  // snapshot when a note has arrived.
  const connected = conn ? conn.status === 1 : cfg.connectionStatus === 1;
  const funded = !!wallet && wallet.availableDcr >= bondDcr;

  const copyAddr = async () => {
    if (!wallet?.address) return;
    try {
      await navigator.clipboard.writeText(wallet.address);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  };

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

  return (
    <div className="flex justify-center p-6">
      <div className="w-full max-w-2xl space-y-5 p-6 rounded-xl bg-gradient-card border border-border/50">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5 text-primary" />
            <h2 className="text-lg font-semibold">Register with {host}</h2>
          </div>
          <span
            className={`flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-full border ${
              connected
                ? 'bg-success/10 border-success/30 text-success'
                : 'bg-warning/10 border-warning/30 text-warning'
            }`}
          >
            <span className={`h-2 w-2 rounded-full ${connected ? 'bg-success' : 'bg-warning'}`} />
            {connected ? 'Connected' : 'Connecting'}
          </span>
        </div>

        <p className="text-sm text-muted-foreground">
          To trade, DCRDEX requires a refundable fidelity bond locked for a set period. Fund the dex
          account below, then post the bond to register.
        </p>

        {/* Markets */}
        <div>
          <div className="text-xs uppercase tracking-wide text-muted-foreground mb-2">
            {cfg.markets.length} markets
          </div>
          <div className="flex flex-wrap gap-2">
            {cfg.markets.map((m) => (
              <div
                key={`${m.base}-${m.quote}`}
                className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-muted/10 border border-border/50 text-sm"
              >
                <span className="flex -space-x-1.5">
                  <CoinIcon symbol={m.base} className="ring-1 ring-card" />
                  <CoinIcon symbol={m.quote} className="ring-1 ring-card" />
                </span>
                <span className="font-medium">
                  {m.base}/{m.quote}
                </span>
              </div>
            ))}
          </div>
        </div>

        {/* Step 1: fund the dex account */}
        <div className="rounded-lg bg-muted/10 border border-border/50 p-4 space-y-3">
          <div className="flex items-center gap-2">
            <span className="flex h-5 w-5 items-center justify-center rounded-full bg-primary/15 text-primary text-xs font-semibold shrink-0">
              1
            </span>
            <h3 className="text-sm font-semibold">Fund your DEX account</h3>
          </div>
          <p className="text-sm text-foreground/90 flex items-start gap-2">
            <Info className="h-4 w-4 mt-0.5 shrink-0 text-primary" />
            <span>
              Send at least {bondDcr.toFixed(2)} DCR (plus a little for network fees) to the deposit
              address below. Your balance updates automatically once the deposit confirms, and
              registration unlocks once it covers the bond.
            </span>
          </p>
          {wallet?.address && (
            <div className="space-y-1">
              <span className="text-xs uppercase tracking-wide text-muted-foreground">
                Deposit address
              </span>
              <div className="flex items-center gap-2">
                <code className="text-xs font-mono break-all flex-1 text-foreground">{wallet.address}</code>
                <button
                  type="button"
                  onClick={copyAddr}
                  title="Copy address"
                  className="p-1.5 rounded-md hover:bg-background/60 transition-colors shrink-0"
                >
                  {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4 text-muted-foreground" />}
                </button>
              </div>
            </div>
          )}
          <div className="flex items-center justify-between border-t border-border/40 pt-2">
            <span className="text-xs uppercase tracking-wide text-muted-foreground">
              Current balance
            </span>
            <span className="font-mono font-semibold">
              {wallet ? `${wallet.availableDcr.toFixed(8)} DCR` : '…'}
            </span>
          </div>
          {wallet && !wallet.synced && (
            <div className="text-xs text-warning">
              Wallet syncing… {Math.round(wallet.syncProgress * 100)}%
            </div>
          )}
        </div>

        {/* Step 2: post the bond */}
        <div className="flex items-center gap-2">
          <span className="flex h-5 w-5 items-center justify-center rounded-full bg-primary/15 text-primary text-xs font-semibold shrink-0">
            2
          </span>
          <h3 className="text-sm font-semibold">Post your bond</h3>
        </div>

        {/* Bond requirement + tiers */}
        <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
          <div>
            <div className="flex items-center gap-1 text-xs text-muted-foreground mb-1">
              Tiers
              <span className="relative inline-flex group">
                <Info className="h-3.5 w-3.5 cursor-help" />
                <span className="pointer-events-none absolute left-0 bottom-full mb-2 w-72 rounded-lg bg-card border border-border/60 p-3 text-xs text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity z-10 shadow-lg">
                  Each tier locks {cfg.bondPerTierDcr.toFixed(2)} DCR for {cfg.bondExpiryDays} days as a
                  refundable fidelity bond ({cfg.bondConfs} conf). More tiers raise your trading limit
                  (how much you can keep in active orders and settlements) and your reputation on the
                  DEX. Bonds auto-renew while maintained and are refundable after expiry.
                </span>
              </span>
            </div>
            <input
              id="dex-tiers"
              type="number"
              min={1}
              value={tiers}
              onChange={(e) => setTiers(Math.max(1, parseInt(e.target.value || '1', 10)))}
              className="w-24 px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
            />
          </div>
          <div className="text-sm">
            <div className="text-xs text-muted-foreground mb-1">Total bond</div>
            <div className="font-mono text-lg font-semibold">{bondDcr.toFixed(2)} DCR</div>
          </div>
          <div className="text-sm text-muted-foreground">
            {cfg.bondExpiryDays} day expiry · {cfg.bondConfs} conf
          </div>
        </div>

        <div className="p-3 rounded-lg bg-warning/10 border border-warning/30 text-sm text-warning flex items-start gap-2">
          <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>
            Posting a bond spends real DCR from the dex account on mainnet. The bond is what lets you
            trade without an account fee: it deters spam and fake orders by holding you accountable
            for the trades you start. Your DCR is time-locked for {cfg.bondExpiryDays} days and is
            refundable to your wallet after it expires. If you back out of a trade during settlement,
            you are penalised: your effective trading tier drops and you may have to post additional
            bond to restore it.
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
            disabled={bondAtoms === 0 || !funded}
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
    </div>
  );
};
