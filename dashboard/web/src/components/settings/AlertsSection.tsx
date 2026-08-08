// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Bell } from 'lucide-react';
import { getAlertsSettings, saveAlertsSettings } from '../../services/api';
import { refreshAlertsSummary } from '../../hooks/useAlerts';
import { Toggle } from './Toggle';

const CATEGORIES: { key: string; label: string; description: string }[] = [
  { key: 'node', label: 'Node', description: 'dcrd unreachable, zero peers, stalled chain sync' },
  { key: 'wallet', label: 'Wallet', description: 'Incoming payments' },
  { key: 'staking', label: 'Staking', description: 'Ticket purchases, votes, misses, expiries, revocations' },
  { key: 'lightning', label: 'Lightning', description: 'Force-closed channels, peer connectivity' },
  { key: 'dex', label: 'DEX', description: 'Completed trades, bisonw connectivity' },
  { key: 'bisonrelay', label: 'Bison Relay', description: 'brclientd connectivity' },
  { key: 'system', label: 'System', description: 'Disk space on the data volumes' },
];

// AlertsSection is the per-category opt-out list. Disabled categories
// generate nothing at the source (the engine filters at raise time);
// disabling also resolves that category's active conditions. Already
// recorded entries stay until read.
export const AlertsSection = () => {
  const [disabled, setDisabled] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    getAlertsSettings()
      .then((s) => setDisabled(new Set(s.disabled ?? [])))
      .catch(() => {});
  }, []);

  const toggle = async (key: string, enabled: boolean) => {
    const next = new Set(disabled);
    if (enabled) {
      next.delete(key);
    } else {
      next.add(key);
    }
    setDisabled(next);
    setBusy(true);
    setError('');
    try {
      await saveAlertsSettings({ schema: 1, disabled: Array.from(next) });
      refreshAlertsSummary();
    } catch {
      setError('Failed to save alert settings.');
      setDisabled(disabled);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50 space-y-4">
      <div className="flex items-center gap-3">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Bell className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Alerts</h3>
          <p className="text-sm text-muted-foreground">
            Disabled categories stop producing alerts entirely; active conditions in a category
            resolve when it is turned off.
          </p>
        </div>
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <div className="space-y-3">
        {CATEGORIES.map((c) => (
          <Toggle
            key={c.key}
            label={c.label}
            description={c.description}
            checked={!disabled.has(c.key)}
            disabled={busy}
            onChange={(next) => toggle(c.key, next)}
          />
        ))}
      </div>
    </div>
  );
};
