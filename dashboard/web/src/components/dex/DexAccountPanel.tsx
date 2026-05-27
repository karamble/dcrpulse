// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, AlertTriangle, ShieldCheck } from 'lucide-react';
import { getDexAccount, postDexBond, setDexBondOptions, type DexAccount } from '../../services/dcrdexApi';
import { useDexRefreshOnNotes } from './DexLiveProvider';

const Card = ({ title, children }: { title: string; children: React.ReactNode }) => (
  <div className="p-4 rounded-xl bg-gradient-card border border-border/50 space-y-2">
    <div className="text-[10px] uppercase tracking-wider text-muted-foreground/60">{title}</div>
    {children}
  </div>
);

const Stat = ({ label, value }: { label: string; value: React.ReactNode }) => (
  <div className="flex items-center justify-between text-sm">
    <span className="text-muted-foreground">{label}</span>
    <span className="font-mono tabular-nums">{value}</span>
  </div>
);

// DexAccountPanel shows the per-server account: connection, tier, reputation and
// bonds, with controls to set the auto-renew target tier and post more bonds.
export const DexAccountPanel = ({ host }: { host: string }) => {
  const [acct, setAcct] = useState<DexAccount | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [actionErr, setActionErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Bond-options form state, seeded from the account once loaded.
  const [seeded, setSeeded] = useState(false);
  const [autoRenew, setAutoRenew] = useState(false);
  const [targetTier, setTargetTier] = useState(1);
  const [maxBonded, setMaxBonded] = useState('');
  const [penaltyComps, setPenaltyComps] = useState('');
  const [bondAssetID, setBondAssetID] = useState(0);

  // Post-bond form state.
  const [postTiers, setPostTiers] = useState(1);
  const [confirming, setConfirming] = useState(false);

  const refresh = () => {
    getDexAccount(host)
      .then((a) => {
        setAcct(a);
        setErr(null);
        if (!seeded) {
          setAutoRenew(a.autoRenew);
          setTargetTier(a.targetTier > 0 ? a.targetTier : 1);
          setMaxBonded(a.maxBondedDcr > 0 ? String(a.maxBondedDcr) : '');
          setPenaltyComps(a.penaltyComps > 0 ? String(a.penaltyComps) : '');
          setBondAssetID(a.bondAssetID || a.bondAssets[0]?.assetID || 0);
          setSeeded(true);
        }
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Failed to load account'));
  };
  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host]);
  useDexRefreshOnNotes(['bondpost', 'bondrefund', 'reputation'], refresh);

  const saveBondOpts = async () => {
    setBusy(true);
    setActionErr(null);
    try {
      await setDexBondOptions(host, {
        targetTier: autoRenew ? Math.max(1, targetTier) : 0,
        bondAssetID: bondAssetID || undefined,
        maxBondedDcr: maxBonded.trim() === '' ? undefined : Math.max(0, parseFloat(maxBonded) || 0),
        penaltyComps: penaltyComps.trim() === '' ? undefined : Math.max(0, parseInt(penaltyComps, 10) || 0),
      });
      refresh();
    } catch (e: any) {
      setActionErr(e?.response?.data || e?.message || 'Failed to update bond options');
    } finally {
      setBusy(false);
    }
  };

  const postBond = async () => {
    if (!acct) return;
    setBusy(true);
    setActionErr(null);
    try {
      await postDexBond(host, acct.bondPerTierAtoms * postTiers);
      setConfirming(false);
      refresh();
    } catch (e: any) {
      setActionErr(e?.response?.data || e?.message || 'Bond posting failed');
    } finally {
      setBusy(false);
    }
  };

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
  if (!acct) {
    return (
      <div className="min-h-[30vh] flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  const connected = acct.connectionStatus === 1;

  return (
    <div className="px-3 lg:px-4 space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-primary" />
          <h2 className="font-semibold">{host}</h2>
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

      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
        <Card title="Trading tier">
          <div className="text-2xl font-mono tabular-nums">{acct.effectiveTier}</div>
          <Stat label="Target" value={acct.targetTier} />
          <Stat label="From bonds" value={acct.bondedTier} />
        </Card>

        <Card title="Reputation">
          <Stat label="Score" value={`${acct.score} / ${acct.maxScore}`} />
          <Stat label="Penalties" value={acct.penalties} />
          <Stat label="Penalty threshold" value={acct.penaltyThreshold} />
        </Card>

        <Card title="Bonds">
          <Stat label="Per tier" value={`${acct.bondPerTierDcr.toFixed(2)} DCR`} />
          <Stat label="Expiry" value={`${acct.bondExpiryDays} days`} />
          <Stat label="Pending" value={acct.pendingBonds.length} />
          <Stat label="Pending refund" value={acct.bondsPendingRefund} />
        </Card>
      </div>

      {acct.pendingBonds.length > 0 && (
        <Card title="Pending bonds">
          {acct.pendingBonds.map((b, i) => (
            <Stat key={i} label={b.symbol} value={`${b.confs} confs`} />
          ))}
        </Card>
      )}

      {actionErr && (
        <div className="p-3 rounded-lg bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{actionErr}</span>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Card title="Bond options">
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={autoRenew} onChange={(e) => setAutoRenew(e.target.checked)} />
            Auto-renew bonds to maintain the trading tier
          </label>
          <div className="grid grid-cols-2 gap-2 pt-1">
            <div>
              <div className="text-xs text-muted-foreground mb-1">Target tier</div>
              <input
                type="number"
                min={1}
                disabled={!autoRenew}
                value={targetTier}
                onChange={(e) => setTargetTier(Math.max(1, parseInt(e.target.value || '1', 10)))}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
              />
            </div>
            <div>
              <div className="text-xs text-muted-foreground mb-1">Max bonded (DCR)</div>
              <input
                type="number"
                min={0}
                step="0.01"
                placeholder="default"
                value={maxBonded}
                onChange={(e) => setMaxBonded(e.target.value)}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
              />
            </div>
            <div>
              <div className="text-xs text-muted-foreground mb-1">Penalty compensation</div>
              <input
                type="number"
                min={0}
                placeholder="0"
                value={penaltyComps}
                onChange={(e) => setPenaltyComps(e.target.value)}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
              />
            </div>
            <div>
              <div className="text-xs text-muted-foreground mb-1">Bond asset</div>
              <select
                value={bondAssetID}
                onChange={(e) => setBondAssetID(parseInt(e.target.value, 10))}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
              >
                {acct.bondAssets.map((a) => (
                  <option key={a.assetID} value={a.assetID}>
                    {a.symbol}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <button
            type="button"
            disabled={busy}
            onClick={saveBondOpts}
            className="w-full px-4 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors disabled:opacity-50"
          >
            Save bond options
          </button>
          <p className="text-xs text-muted-foreground">
            Auto-renew re-posts bonds to keep the target tier as they expire. Max bonded caps locked
            bond value (blank leaves it unchanged, 0 resets to the default); penalty compensation auto-
            tops-up tiers lost to penalties.
          </p>
        </Card>

        <Card title="Post additional bond">
          <div className="flex items-end gap-2">
            <div>
              <div className="text-xs text-muted-foreground mb-1">Tiers</div>
              <input
                type="number"
                min={1}
                value={postTiers}
                onChange={(e) => setPostTiers(Math.max(1, parseInt(e.target.value || '1', 10)))}
                className="w-24 px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
              />
            </div>
            <div className="text-sm">
              <div className="text-xs text-muted-foreground mb-1">Total</div>
              <div className="font-mono font-semibold">{(acct.bondPerTierDcr * postTiers).toFixed(2)} DCR</div>
            </div>
          </div>
          {!confirming ? (
            <button
              type="button"
              onClick={() => setConfirming(true)}
              className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90"
            >
              Post bond
            </button>
          ) : (
            <>
              <div className="p-2.5 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning flex items-start gap-2">
                <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
                <span>Spends {(acct.bondPerTierDcr * postTiers).toFixed(2)} DCR from the dex account, locked until expiry.</span>
              </div>
              <div className="flex gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => setConfirming(false)}
                  className="flex-1 px-4 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors disabled:opacity-50"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={postBond}
                  className="flex-1 bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90 disabled:opacity-50"
                >
                  {busy ? 'Posting…' : 'Confirm'}
                </button>
              </div>
            </>
          )}
        </Card>
      </div>
    </div>
  );
};
