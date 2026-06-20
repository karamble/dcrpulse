// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, AlertTriangle, Check, Copy, Info, ShieldCheck } from 'lucide-react';
import {
  createDexAssetWallet,
  discoverDexAccount,
  getDexAssetCatalog,
  getDexBondsFeeBuffer,
  getDexConfig,
  getDexExchanges,
  getDexPostBondStatus,
  getDexWallets,
  postDexBond,
  type DexAsset,
  type DexBondAsset,
  type DexConfig,
  type DexWalletDefinition,
  type DexWalletState,
} from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { DexServerBanner } from './DexServerBanner';
import { DexWalletConfigForm } from './DexWalletConfigForm';
import { fmtAmt } from './dexFormat';
import { useDexConn, useDexRefreshOnNotes } from './DexLiveProvider';

interface DexRegisterProps {
  host: string;
  onRegistered: () => void;
}

// DexRegister lets the user register with a DEX by posting a fidelity bond. The
// bond can be posted in any asset the server accepts (mirroring dcrdex upstream):
// pick the bond asset, create its wallet if needed, fund it, then post. Posting
// spends real funds, so it is gated on the bond wallet being synced and funded and
// behind an explicit confirmation.
export const DexRegister = ({ host, onRegistered }: DexRegisterProps) => {
  const [cfg, setCfg] = useState<DexConfig | null>(null);
  const [wallets, setWallets] = useState<DexWalletState[]>([]);
  const [catalog, setCatalog] = useState<DexAsset[]>([]);
  const [assetID, setAssetID] = useState<number | null>(null);
  const [feeBuffer, setFeeBuffer] = useState(0);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [tiers, setTiers] = useState(1);
  const [copied, setCopied] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [discovering, setDiscovering] = useState(true);
  const [discoverRun, setDiscoverRun] = useState(0);
  // Wallet-creation state for a bond asset that has no wallet yet.
  const [wtypeIdx, setWtypeIdx] = useState(0);
  const [wconfig, setWconfig] = useState<Record<string, string>>({});
  const [wpass, setWpass] = useState('');

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
  }, [host, discoverRun]);

  const refreshWallets = () => getDexWallets().then(setWallets).catch(() => {});
  useEffect(() => {
    refreshWallets();
    getDexAssetCatalog().then(setCatalog).catch(() => {});
    const id = window.setInterval(refreshWallets, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useDexRefreshOnNotes(['balance', 'walletstate', 'walletsync', 'createwallet'], refreshWallets);
  const conn = useDexConn(host);

  // The bond assets the server accepts (with per-asset per-tier amounts). Fall
  // back to a DCR entry from the legacy single-asset fields if the server config
  // predates the per-asset list.
  const bondAssets = useMemo<DexBondAsset[]>(() => {
    if (!cfg) return [];
    if (cfg.bondAssets?.length) return cfg.bondAssets;
    return [{ symbol: 'DCR', assetID: 42, confs: cfg.bondConfs, amtAtoms: cfg.bondPerTierAtoms, amt: cfg.bondPerTierDcr }];
  }, [cfg]);

  // Default to DCR (dcrpulse's primary bond asset) when offered, else the first.
  useEffect(() => {
    if (assetID != null || bondAssets.length === 0) return;
    const dcr = bondAssets.find((a) => a.assetID === 42);
    setAssetID(dcr ? 42 : bondAssets[0].assetID);
  }, [bondAssets, assetID]);

  // Per-asset bond fee buffer (best effort) for the funding target.
  useEffect(() => {
    if (assetID == null) return;
    let cancelled = false;
    setFeeBuffer(0);
    getDexBondsFeeBuffer(assetID)
      .then((f) => {
        if (!cancelled) setFeeBuffer(f);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [assetID]);

  // When the server connection comes back while the page is stuck on a failed
  // load, re-run the registration discovery so it resolves without a reload.
  const serverConnected = conn?.status === 1;
  useEffect(() => {
    if (!serverConnected || discovering || cfg) return;
    setLoadErr(null);
    setDiscoverRun((n) => n + 1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverConnected]);

  // After submitting, the bond posts in the background, so poll until the account
  // appears (its acctID is set once the bond broadcasts) and hand off to the
  // trading view, or surface a pre-broadcast failure from the status endpoint.
  useEffect(() => {
    if (!submitting) return;
    let cancelled = false;
    const check = async () => {
      try {
        const s = await getDexPostBondStatus(host);
        if (cancelled) return;
        if (s.phase === 'error') {
          setErr(s.error || 'Bond posting failed');
          setSubmitting(false);
          setBusy(false);
          setConfirming(false);
          return;
        }
        const ex = await getDexExchanges();
        if (!cancelled && ex[host]?.acctID) onRegistered();
      } catch {
        /* keep polling */
      }
    };
    check();
    const id = window.setInterval(check, 3000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [submitting, host]);

  // Wallet types creatable for the selected bond asset (base coin or token).
  const walletDefs = useMemo<DexWalletDefinition[]>(() => {
    if (assetID == null) return [];
    for (const a of catalog) {
      if (a.id === assetID) return a.availableWallets;
      const t = (a.tokens ?? []).find((tok) => tok.id === assetID);
      if (t) return [t.definition];
    }
    return [];
  }, [catalog, assetID]);

  if (loadErr) {
    return (
      <div className="flex flex-col items-center gap-3 p-6">
        <DexServerBanner host={host} />
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
        Checking for an existing registration on {host}...
        <DexServerBanner host={host} />
        {conn && conn.status !== 1 && (
          <span className="text-xs">
            The check continues automatically once the server connection is restored.
          </span>
        )}
      </div>
    );
  }
  if (!cfg || assetID == null) {
    return (
      <div className="flex justify-center py-16">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  const bondAsset = bondAssets.find((a) => a.assetID === assetID);
  const wallet = wallets.find((w) => w.assetID === assetID);
  const sym = bondAsset?.symbol ?? '';
  const bondAtoms = bondAsset ? bondAsset.amtAtoms * tiers : 0;
  const bondConv = bondAsset ? bondAsset.amt * tiers : 0;
  const target = bondConv + feeBuffer;
  const needsWallet = !wallet;
  const connected = conn ? conn.status === 1 : cfg.connectionStatus === 1;
  const funded = !!wallet && wallet.synced && wallet.available >= target;
  const wdef = walletDefs[wtypeIdx];

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

  const selectAsset = (id: number) => {
    if (id === assetID) return;
    setAssetID(id);
    setConfirming(false);
    setErr(null);
    setWtypeIdx(0);
    setWconfig({});
    setWpass('');
  };

  const createWallet = async () => {
    if (!wdef) return;
    setBusy(true);
    setErr(null);
    try {
      // Seeded wallets (Native/SPV) derive their password from the app seed and
      // reject an external password; only external wallet types take one.
      await createDexAssetWallet(assetID, wdef.type, wconfig, wdef.seeded ? '' : wpass);
      setWconfig({});
      setWpass('');
      await refreshWallets();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Wallet creation failed');
    } finally {
      setBusy(false);
    }
  };

  const post = async () => {
    setBusy(true);
    setErr(null);
    try {
      await postDexBond(host, bondAtoms, assetID);
      // The backend posts the bond in the background; the effect above polls for
      // the broadcast (or a pre-broadcast error) and then hands off to trading.
      setSubmitting(true);
    } catch (e: any) {
      setErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Bond posting failed');
      setBusy(false);
      setConfirming(false);
    }
  };

  // Per-asset readiness badge for the selector (rough: funded for one tier).
  const assetState = (a: DexBondAsset): 'ready' | 'fund' | 'setup' => {
    const wl = wallets.find((w) => w.assetID === a.assetID);
    if (!wl) return 'setup';
    if (wl.synced && wl.available >= a.amt) return 'ready';
    return 'fund';
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
              connected ? 'bg-success/10 border-success/30 text-success' : 'bg-warning/10 border-warning/30 text-warning'
            }`}
          >
            <span className={`h-2 w-2 rounded-full ${connected ? 'bg-success' : 'bg-warning'}`} />
            {connected ? 'Connected' : 'Connecting'}
          </span>
        </div>

        <p className="text-sm text-muted-foreground">
          To trade, DCRDEX requires a refundable fidelity bond locked for a set period. Choose a bond
          asset, fund its wallet, then post the bond to register.
        </p>

        {/* Markets */}
        <div>
          <div className="text-xs uppercase tracking-wide text-muted-foreground mb-2">{cfg.markets.length} markets</div>
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

        {/* Bond asset selector (only when the server accepts more than one) */}
        {bondAssets.length > 1 && (
          <div>
            <div className="text-xs uppercase tracking-wide text-muted-foreground mb-2">Bond asset</div>
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
              {bondAssets.map((a) => {
                const state = assetState(a);
                const active = a.assetID === assetID;
                return (
                  <button
                    key={a.assetID}
                    type="button"
                    onClick={() => selectAsset(a.assetID)}
                    className={`flex items-center gap-2 p-2.5 rounded-xl border text-left transition-colors ${
                      active ? 'border-primary bg-primary/5' : 'border-border/50 hover:border-primary/50'
                    }`}
                  >
                    <CoinIcon symbol={a.symbol} />
                    <span className="min-w-0 flex-1">
                      <span className="block text-sm font-medium truncate">{a.symbol}</span>
                      <span className="block text-[11px] text-muted-foreground">{fmtAmt(a.amt, 8)}/tier</span>
                    </span>
                    <span
                      className={`text-[10px] px-1.5 py-0.5 rounded-full border ${
                        state === 'ready'
                          ? 'bg-success/10 border-success/30 text-success'
                          : 'bg-warning/10 border-warning/30 text-warning'
                      }`}
                    >
                      {state === 'ready' ? 'Ready' : state === 'fund' ? 'Fund' : 'Setup'}
                    </span>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* Step 1: create or fund the bond wallet */}
        <div className="rounded-lg bg-muted/10 border border-border/50 p-4 space-y-3">
          <div className="flex items-center gap-2">
            <span className="flex h-5 w-5 items-center justify-center rounded-full bg-primary/15 text-primary text-xs font-semibold shrink-0">
              1
            </span>
            <h3 className="text-sm font-semibold">{needsWallet ? `Create your ${sym} wallet` : `Fund your ${sym} wallet`}</h3>
          </div>

          {needsWallet ? (
            <div className="space-y-3">
              <p className="text-sm text-foreground/90 flex items-start gap-2">
                <Info className="h-4 w-4 mt-0.5 shrink-0 text-primary" />
                <span>Posting a bond in {sym} needs a {sym} wallet. Create one below, then fund it.</span>
              </p>
              {walletDefs.length === 0 ? (
                <div className="text-xs text-muted-foreground">No wallet type is available for {sym}.</div>
              ) : (
                <>
                  {walletDefs.length > 1 && (
                    <nav className="flex gap-2 border-b border-border">
                      {walletDefs.map((d, i) => (
                        <button
                          key={d.type}
                          type="button"
                          onClick={() => {
                            setWtypeIdx(i);
                            setWconfig({});
                          }}
                          className={`px-3 py-1.5 -mb-px border-b-2 text-sm transition-colors ${
                            i === wtypeIdx
                              ? 'border-primary text-primary font-semibold'
                              : 'border-transparent text-muted-foreground hover:text-foreground'
                          }`}
                        >
                          {d.tab}
                        </button>
                      ))}
                    </nav>
                  )}
                  {wdef && <p className="text-xs text-muted-foreground">{wdef.description}</p>}
                  {wdef && (
                    <DexWalletConfigForm opts={wdef.configOpts} values={wconfig} onChange={(k, v) => setWconfig((c) => ({ ...c, [k]: v }))} />
                  )}
                  {wdef && !wdef.seeded && (
                    <div>
                      <label className="block text-xs text-muted-foreground mb-1">Wallet password</label>
                      <input
                        type="password"
                        value={wpass}
                        onChange={(e) => setWpass(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
                      />
                    </div>
                  )}
                  <button
                    type="button"
                    disabled={busy || !wdef}
                    onClick={createWallet}
                    className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2.5 transition-colors hover:bg-primary/90 disabled:opacity-50"
                  >
                    {busy ? 'Creating...' : `Create ${sym} wallet`}
                  </button>
                </>
              )}
            </div>
          ) : (
            <>
              <p className="text-sm text-foreground/90 flex items-start gap-2">
                <Info className="h-4 w-4 mt-0.5 shrink-0 text-primary" />
                <span>
                  Send at least {fmtAmt(target, 8)} {sym} (the bond plus a network-fee buffer) to the deposit
                  address below. Your balance updates automatically, and registration unlocks once it covers
                  the bond.
                </span>
              </p>
              {wallet?.address && (
                <div className="space-y-1">
                  <span className="text-xs uppercase tracking-wide text-muted-foreground">Deposit address</span>
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
                <span className="text-xs uppercase tracking-wide text-muted-foreground">Current balance</span>
                <span className="font-mono font-semibold">{wallet ? `${fmtAmt(wallet.available, 8)} ${sym}` : '…'}</span>
              </div>
              {wallet && !wallet.synced && (
                <div className="text-xs text-warning">Wallet syncing… {Math.round(wallet.syncProgress * 100)}%</div>
              )}
            </>
          )}
        </div>

        {err && (
          <div className="p-3 rounded-lg bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}

        {/* Step 2: post the bond (once a wallet exists) */}
        {!needsWallet && (
          <>
            <div className="flex items-center gap-2">
              <span className="flex h-5 w-5 items-center justify-center rounded-full bg-primary/15 text-primary text-xs font-semibold shrink-0">
                2
              </span>
              <h3 className="text-sm font-semibold">Post your bond</h3>
            </div>

            <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
              <div>
                <div className="flex items-center gap-1 text-xs text-muted-foreground mb-1">
                  Tiers
                  <span className="relative inline-flex group">
                    <Info className="h-3.5 w-3.5 cursor-help" />
                    <span className="pointer-events-none absolute left-0 bottom-full mb-2 w-72 rounded-lg bg-card border border-border/60 p-3 text-xs text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity z-10 shadow-lg">
                      Each tier locks {bondAsset ? fmtAmt(bondAsset.amt, 8) : '…'} {sym} for {cfg.bondExpiryDays} days as a
                      refundable fidelity bond ({bondAsset?.confs ?? cfg.bondConfs} conf). More tiers raise your trading
                      limit (how much you can keep in active orders and settlements) and your reputation on the DEX.
                      Bonds auto-renew while maintained and are refundable after expiry.
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
                <div className="font-mono text-lg font-semibold">{fmtAmt(bondConv, 8)} {sym}</div>
              </div>
              <div className="text-sm text-muted-foreground">
                {cfg.bondExpiryDays} day expiry, {bondAsset?.confs ?? cfg.bondConfs} conf
              </div>
            </div>

            <div className="p-3 rounded-lg bg-warning/10 border border-warning/30 text-sm text-warning flex items-start gap-2">
              <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>
                Posting a bond spends real {sym} from your dex wallet on mainnet. The bond is what lets you
                trade without an account fee: it deters spam and fake orders by holding you accountable for the
                trades you start. Your funds are time-locked for {cfg.bondExpiryDays} days and are refundable
                after they expire. If you back out of a trade during settlement, you are penalised: your
                effective trading tier drops and you may have to post additional bond to restore it.
              </span>
            </div>

            {submitting ? (
              <div className="flex items-center justify-center gap-3 rounded-lg bg-muted/10 border border-border/50 px-4 py-3 text-sm text-muted-foreground">
                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary" />
                Submitting bond to {host}. Waiting for it to broadcast...
              </div>
            ) : !confirming ? (
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
                  {busy ? 'Posting…' : `Confirm: post ${fmtAmt(bondConv, 8)} ${sym}`}
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
};
