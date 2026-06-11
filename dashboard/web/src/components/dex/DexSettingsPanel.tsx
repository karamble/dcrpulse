// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertTriangle, Bell, Check, KeyRound } from 'lucide-react';
import { getDexStatus } from '../../services/dcrdexApi';
import { DexSeedBackup } from './DexSeedBackup';
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

  // Whether the app seed has been backed up (from the dashboard's persisted flag).
  const [backedUp, setBackedUp] = useState<boolean | null>(null);
  const refreshBackedUp = () => {
    getDexStatus()
      .then((s) => setBackedUp(s.seedBackedUp))
      .catch(() => {});
  };
  useEffect(refreshBackedUp, []);

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
        {backedUp !== null && (
          <div className={`text-xs flex items-center gap-1.5 ${backedUp ? 'text-success' : 'text-warning'}`}>
            {backedUp ? <Check className="h-3.5 w-3.5 shrink-0" /> : <AlertTriangle className="h-3.5 w-3.5 shrink-0" />}
            {backedUp ? 'Backed up' : 'Not backed up yet'}
          </div>
        )}
        <DexSeedBackup onDone={refreshBackedUp} />
      </Card>
    </div>
  );
};
