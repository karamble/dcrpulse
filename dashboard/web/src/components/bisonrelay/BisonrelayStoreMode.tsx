// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertTriangle, FileText, Loader2, Store } from 'lucide-react';
import {
  BisonrelayStoreMode,
  getBisonrelayStoreMode,
  setBisonrelayStoreMode,
} from '../../services/bisonrelayApi';

// BisonrelayStoreModePanel toggles this node between hosting static pages and a
// simplestore. The two are mutually exclusive (Bison Relay binds one resource
// provider at the root); brclientd applies the switch at runtime. onModeChange
// lets the host hide page CRUD while the storefront is active.
export const BisonrelayStoreModePanel = ({
  onModeChange,
}: {
  onModeChange?: (mode: BisonrelayStoreMode) => void;
}) => {
  const [mode, setMode] = useState<BisonrelayStoreMode | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [setupOpen, setSetupOpen] = useState(false);
  const [confirmDisable, setConfirmDisable] = useState(false);
  const [payType, setPayType] = useState('ln');
  const [account, setAccount] = useState('');
  const [shipCharge, setShipCharge] = useState('0');

  useEffect(() => {
    getBisonrelayStoreMode()
      .then((m) => {
        setMode(m);
        setPayType(m.pay_type || 'ln');
        setAccount(m.account || '');
        setShipCharge(String(m.ship_charge ?? 0));
        setErr(null);
        onModeChange?.(m);
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Could not load hosting mode'));
    // onModeChange is a stable callback from the host; run once on mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const apply = async (next: BisonrelayStoreMode) => {
    setSaving(true);
    setErr(null);
    try {
      const m = await setBisonrelayStoreMode(next);
      setMode(m);
      setSetupOpen(false);
      setConfirmDisable(false);
      onModeChange?.(m);
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Could not switch hosting mode');
    } finally {
      setSaving(false);
    }
  };

  const settings = () => ({
    pay_type: payType,
    account,
    ship_charge: parseFloat(shipCharge) || 0,
  });

  if (!mode) {
    return (
      <div className="rounded-lg border border-border/50 bg-background/40 p-3 text-sm text-muted-foreground">
        {err ? (
          <span className="text-rose-300">{err}</span>
        ) : (
          <span className="flex items-center gap-2">
            <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading hosting mode…
          </span>
        )}
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-4 space-y-3 text-sm">
      <div className="flex items-center gap-2">
        {mode.enabled ? (
          <Store className="h-4 w-4 text-primary" />
        ) : (
          <FileText className="h-4 w-4 text-primary" />
        )}
        <span className="font-semibold text-foreground">
          {mode.enabled ? 'Hosting a storefront' : 'Hosting static pages'}
        </span>
        <span className="text-xs text-muted-foreground">
          {mode.enabled
            ? `(${mode.pay_type === 'onchain' ? 'on-chain' : 'Lightning'} payments)`
            : '(markdown pages over the relay)'}
        </span>
      </div>
      <p className="text-xs text-muted-foreground">
        A node serves one or the other: Bison Relay binds a single resource provider at the root.
      </p>

      {err && <div className="text-xs text-rose-300">{err}</div>}

      {mode.enabled ? (
        confirmDisable ? (
          <div className="space-y-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-3">
            <div className="flex items-start gap-2 text-amber-300">
              <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="text-xs">
                Switching to static pages stops the store's invoice watcher. Orders awaiting payment
                will not settle until you re-enable the storefront. Continue?
              </span>
            </div>
            <div className="flex gap-2">
              <button
                type="button"
                disabled={saving}
                onClick={() => apply({ enabled: false, ...settings() })}
                className="px-3 py-1.5 rounded-md bg-amber-500/20 text-amber-200 text-sm font-semibold hover:bg-amber-500/30 disabled:opacity-50"
              >
                {saving ? 'Switching…' : 'Switch to static pages'}
              </button>
              <button
                type="button"
                onClick={() => setConfirmDisable(false)}
                className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setConfirmDisable(true)}
            className="px-3 py-1.5 rounded-md bg-muted/40 text-foreground text-sm font-medium hover:bg-muted/60"
          >
            Switch to static pages
          </button>
        )
      ) : setupOpen ? (
        <div className="space-y-3 rounded-md border border-border/50 p-3">
          <label className="block">
            <span className="text-xs text-muted-foreground">Payment method</span>
            <select
              value={payType}
              onChange={(e) => setPayType(e.target.value)}
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            >
              <option value="ln">Lightning</option>
              <option value="onchain">On-chain</option>
            </select>
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">
              Wallet account (for on-chain order addresses)
            </span>
            <input
              value={account}
              onChange={(e) => setAccount(e.target.value)}
              placeholder="default"
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Shipping surcharge (USD)</span>
            <input
              type="number"
              value={shipCharge}
              onChange={(e) => setShipCharge(e.target.value)}
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={saving}
              onClick={() => apply({ enabled: true, ...settings() })}
              className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50"
            >
              {saving ? 'Enabling…' : 'Enable storefront'}
            </button>
            <button
              type="button"
              onClick={() => setSetupOpen(false)}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
          <p className="text-xs text-muted-foreground">
            Enabling the storefront replaces static-page hosting on this node. Your pages stay on
            disk and return when you switch back.
          </p>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setSetupOpen(true)}
          className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30"
        >
          Set up a storefront
        </button>
      )}
    </div>
  );
};
