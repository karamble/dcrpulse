// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertTriangle, FileText, Loader2, PowerOff, Store } from 'lucide-react';
import {
  BisonrelayStoreMode,
  getBisonrelayStoreMode,
  getBisonrelayStoreTemplates,
  setBisonrelayStoreMode,
} from '../../services/bisonrelayApi';

// BisonrelayStoreModePanel switches this node between three mutually exclusive
// resource-hosting modes: off (serve nothing), static pages, and a simplestore
// (Bison Relay binds one resource provider at the root). brclientd applies the
// switch at runtime. onModeChange lets the host adjust what it shows per mode.
export const BisonrelayStoreModePanel = ({
  onModeChange,
}: {
  onModeChange?: (mode: BisonrelayStoreMode) => void;
}) => {
  const [mode, setMode] = useState<BisonrelayStoreMode | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [setupOpen, setSetupOpen] = useState(false);
  // When in store mode, leaving stops the invoice watcher; confirm first and
  // remember which mode we are switching to.
  const [leaveTarget, setLeaveTarget] = useState<'off' | 'pages' | null>(null);
  const [payType, setPayType] = useState('ln');
  const [account, setAccount] = useState('');
  const [shipCharge, setShipCharge] = useState('0');
  // Whether a storefront already exists on disk (templates present). When it
  // does, returning to it is a one-click switch using the saved settings rather
  // than a fresh setup.
  const [storeExists, setStoreExists] = useState(false);

  // A storefront's templates persist on disk across off/pages, so their presence
  // means a store was already configured here.
  const refreshExists = () =>
    getBisonrelayStoreTemplates()
      .then((t) => setStoreExists(t.length > 0))
      .catch(() => {});

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
    refreshExists();
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
      setLeaveTarget(null);
      onModeChange?.(m);
      refreshExists();
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
            <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading hosting mode...
          </span>
        )}
      </div>
    );
  }

  const isStore = mode.mode === 'store';
  const isPages = mode.mode === 'pages';

  const Icon = isStore ? Store : isPages ? FileText : PowerOff;
  const title = isStore
    ? 'Hosting a storefront'
    : isPages
      ? 'Hosting static pages'
      : 'Hosting deactivated';
  const subtitle = isStore
    ? `(${mode.pay_type === 'onchain' ? 'on-chain' : 'Lightning'} payments)`
    : isPages
      ? '(markdown pages over the relay)'
      : '(nothing served over the relay)';

  // A plain pill button used for switches that need no confirmation.
  const switchButton = (label: string, onClick: () => void) => (
    <button
      type="button"
      disabled={saving}
      onClick={onClick}
      className="px-3 py-1.5 rounded-md bg-muted/40 text-foreground text-sm font-medium hover:bg-muted/60 disabled:opacity-50"
    >
      {label}
    </button>
  );

  // Entry into the storefront: a one-click switch reusing saved settings when a
  // store already exists, or the first-time setup form otherwise.
  const storeEntryButton = storeExists ? (
    <button
      type="button"
      disabled={saving}
      onClick={() => apply({ mode: 'store', ...settings() })}
      className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50"
    >
      {saving ? 'Switching...' : 'Switch to storefront'}
    </button>
  ) : (
    <button
      type="button"
      onClick={() => setSetupOpen(true)}
      className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30"
    >
      Set up a storefront
    </button>
  );

  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-4 space-y-3 text-sm">
      <div className="flex items-center gap-2">
        <Icon className="h-4 w-4 text-primary" />
        <span className="font-semibold text-foreground">{title}</span>
        <span className="text-xs text-muted-foreground">{subtitle}</span>
      </div>
      <p className="text-xs text-muted-foreground">
        A node serves one at a time: Bison Relay binds a single resource provider at the root.
      </p>

      {err && <div className="text-xs text-rose-300">{err}</div>}

      {/* Store setup form: opened from off or pages mode. */}
      {setupOpen ? (
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
              onClick={() => apply({ mode: 'store', ...settings() })}
              className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50"
            >
              {saving ? 'Enabling...' : 'Enable storefront'}
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
            Enabling the storefront replaces page hosting on this node. Your pages stay on disk and
            return when you switch back.
          </p>
        </div>
      ) : leaveTarget ? (
        <div className="space-y-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-3">
          <div className="flex items-start gap-2 text-amber-300">
            <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="text-xs">
              Leaving the storefront stops its invoice watcher. Orders awaiting payment will not
              settle until you re-enable the storefront. Continue?
            </span>
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={saving}
              onClick={() => apply({ mode: leaveTarget, ...settings() })}
              className="px-3 py-1.5 rounded-md bg-amber-500/20 text-amber-200 text-sm font-semibold hover:bg-amber-500/30 disabled:opacity-50"
            >
              {saving
                ? 'Switching...'
                : leaveTarget === 'pages'
                  ? 'Switch to static pages'
                  : 'Deactivate hosting'}
            </button>
            <button
              type="button"
              onClick={() => setLeaveTarget(null)}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          {isStore ? (
            <>
              {switchButton('Switch to static pages', () => setLeaveTarget('pages'))}
              {switchButton('Deactivate hosting', () => setLeaveTarget('off'))}
            </>
          ) : isPages ? (
            <>
              {switchButton('Deactivate hosting', () => apply({ mode: 'off', ...settings() }))}
              {storeEntryButton}
            </>
          ) : (
            <>
              {switchButton('Host static pages', () => apply({ mode: 'pages', ...settings() }))}
              {storeEntryButton}
            </>
          )}
        </div>
      )}
    </div>
  );
};
