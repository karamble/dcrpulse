// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, AlertTriangle, Bell, Check, Copy, KeyRound } from 'lucide-react';
import { exportDexSeed } from '../../services/dcrdexApi';
import {
  CATEGORY_LABELS,
  loadNotifPrefs,
  saveNotifPrefs,
  type DexNotifPrefs,
  type NotifCategory,
} from './dexNotifPrefs';

const Card = ({ title, icon: Icon, children }: { title: string; icon: typeof Bell; children: React.ReactNode }) => (
  <div className="p-4 rounded-xl bg-gradient-card border border-border/50 space-y-3">
    <div className="flex items-center gap-2 text-sm font-semibold">
      <Icon className="h-4 w-4 text-primary" />
      {title}
    </div>
    {children}
  </div>
);

const Toggle = ({
  label,
  checked,
  disabled,
  onChange,
}: {
  label: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (v: boolean) => void;
}) => (
  <label className={`flex items-center justify-between gap-3 text-sm ${disabled ? 'opacity-50' : ''}`}>
    <span>{label}</span>
    <input type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
  </label>
);

// DexSettingsPanel holds DEX-view preferences: desktop notification settings
// (client-side) and app-seed backup (password-gated).
export const DexSettingsPanel = () => {
  const [prefs, setPrefs] = useState<DexNotifPrefs>(loadNotifPrefs);
  const [permission, setPermission] = useState(typeof Notification !== 'undefined' ? Notification.permission : 'denied');

  const update = (next: DexNotifPrefs) => {
    setPrefs(next);
    saveNotifPrefs(next);
  };

  const setDesktop = async (on: boolean) => {
    if (on && typeof Notification !== 'undefined' && Notification.permission !== 'granted') {
      const p = await Notification.requestPermission();
      setPermission(p);
      if (p !== 'granted') {
        update({ ...prefs, desktop: false });
        return;
      }
    }
    update({ ...prefs, desktop: on });
  };

  const setCategory = (c: NotifCategory, v: boolean) => update({ ...prefs, categories: { ...prefs.categories, [c]: v } });

  // Seed backup state.
  const [appPass, setAppPass] = useState('');
  const [seed, setSeed] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [seedErr, setSeedErr] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const revealSeed = async () => {
    setBusy(true);
    setSeedErr(null);
    try {
      setSeed(await exportDexSeed(appPass));
      setAppPass('');
    } catch (e: any) {
      setSeedErr(e?.response?.data || e?.message || 'Failed to export seed');
    } finally {
      setBusy(false);
    }
  };

  const copySeed = async () => {
    if (!seed) return;
    try {
      await navigator.clipboard.writeText(seed);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  };

  return (
    <div className="px-3 lg:px-4 grid grid-cols-1 lg:grid-cols-2 gap-3">
      <Card title="Notifications" icon={Bell}>
        <Toggle label="Desktop notifications" checked={prefs.desktop} onChange={setDesktop} />
        {permission === 'denied' && (
          <p className="text-[11px] text-warning">Browser notifications are blocked; allow them in your browser settings.</p>
        )}
        <div className="space-y-1.5 pt-1 border-t border-border/40">
          {(Object.keys(CATEGORY_LABELS) as NotifCategory[]).map((c) => (
            <Toggle
              key={c}
              label={CATEGORY_LABELS[c]}
              checked={prefs.categories[c]}
              disabled={!prefs.desktop}
              onChange={(v) => setCategory(c, v)}
            />
          ))}
        </div>
        <p className="text-[11px] text-muted-foreground">
          Fires an OS notification for new DEX activity in the enabled categories. The bell always shows them.
        </p>
      </Card>

      <Card title="Back up app seed" icon={KeyRound}>
        {seed ? (
          <>
            <div className="p-2.5 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning flex items-start gap-2">
              <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>Anyone with this seed can restore your DCRDEX app and spend its funds. Store it offline; never share it.</span>
            </div>
            <div className="flex items-start gap-2">
              <code className="text-xs font-mono break-all flex-1 p-2 rounded bg-background border border-border">{seed}</code>
              <button type="button" onClick={copySeed} title="Copy" className="p-1.5 rounded-md hover:bg-background/60 shrink-0">
                {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4 text-muted-foreground" />}
              </button>
            </div>
            <button
              type="button"
              onClick={() => setSeed(null)}
              className="w-full px-4 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors text-sm"
            >
              Hide
            </button>
          </>
        ) : (
          <>
            <p className="text-xs text-muted-foreground">Re-enter your DCRDEX app password to reveal the recovery seed.</p>
            <input
              type="password"
              value={appPass}
              onChange={(e) => setAppPass(e.target.value)}
              placeholder="App password"
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
            />
            {seedErr && (
              <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
                <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                <span className="break-words">{seedErr}</span>
              </div>
            )}
            <button
              type="button"
              disabled={busy || !appPass}
              onClick={revealSeed}
              className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {busy ? 'Revealing...' : 'Reveal seed'}
            </button>
          </>
        )}
      </Card>
    </div>
  );
};
