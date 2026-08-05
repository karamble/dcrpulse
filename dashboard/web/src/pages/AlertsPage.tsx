// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Bell, CheckCheck } from 'lucide-react';
import { AlertEntry, getAlerts, markAlertRead, markAllAlertsRead } from '../services/api';
import { refreshAlertsSummary } from '../hooks/useAlerts';
import { useVisiblePoll } from '../hooks/useVisiblePoll';
import { useBisonrelayLive } from '../components/bisonrelay/BisonrelayLiveProvider';
import { AlertRow, alertCategoryRoute } from '../components/alerts/AlertRow';

const CATEGORY_LABELS: Record<string, string> = {
  node: 'Node',
  wallet: 'Wallet',
  staking: 'Staking',
  lightning: 'Lightning',
  dex: 'DEX',
  bisonrelay: 'Bison Relay',
  system: 'System',
};

const SEVERITY_LABELS: Record<string, string> = {
  critical: 'Critical',
  warning: 'Warning',
  info: 'Info',
};

const chipClass = (active: boolean) =>
  `px-2.5 py-1 rounded-full border text-xs whitespace-nowrap transition-colors ${
    active
      ? 'bg-primary/15 text-primary border-primary/30'
      : 'bg-muted/10 text-muted-foreground border-border/50 hover:text-foreground'
  }`;

// AlertsPage is the full alert history: every ring entry, newest first, with
// client-side category and severity filters (the ring is capped at a few
// hundred entries, so there is no server-side paging).
export const AlertsPage = () => {
  const [entries, setEntries] = useState<AlertEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [category, setCategory] = useState('');
  const [severity, setSeverity] = useState('');
  const navigate = useNavigate();
  const { addListener } = useBisonrelayLive();

  const load = () =>
    getAlerts()
      .then(setEntries)
      .catch(() => {})
      .finally(() => setLoading(false));

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useVisiblePoll(load, 60000);
  useEffect(() => {
    return addListener((evt) => {
      if (evt.type === 'alerts') void load();
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addListener]);

  const readOne = async (id: string) => {
    setEntries((prev) => prev.map((e) => (e.id === id ? { ...e, read: true } : e)));
    try {
      await markAlertRead(id);
    } catch {
      void load();
    }
    refreshAlertsSummary();
  };

  const readAll = async () => {
    setEntries((prev) => prev.map((e) => ({ ...e, read: true })));
    try {
      await markAllAlertsRead();
    } catch {
      void load();
    }
    refreshAlertsSummary();
  };

  const filtered = entries.filter(
    (e) => (!category || e.category === category) && (!severity || e.severity === severity),
  );
  const hasUnread = entries.some((e) => !e.read);

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Bell className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Alerts</h3>
          <p className="text-sm text-muted-foreground">
            Operator events and conditions across the stack
          </p>
        </div>
        {hasUnread && (
          <button
            type="button"
            onClick={readAll}
            className="ml-auto inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-muted/20 text-muted-foreground text-xs hover:bg-muted/30 hover:text-foreground transition-colors"
          >
            <CheckCheck className="h-3.5 w-3.5" />
            Mark all read
          </button>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-1.5 mb-2">
        <button type="button" onClick={() => setCategory('')} className={chipClass(category === '')}>
          All
        </button>
        {Object.entries(CATEGORY_LABELS).map(([key, label]) => (
          <button
            key={key}
            type="button"
            onClick={() => setCategory(category === key ? '' : key)}
            className={chipClass(category === key)}
          >
            {label}
          </button>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-1.5 mb-4">
        <button type="button" onClick={() => setSeverity('')} className={chipClass(severity === '')}>
          All severities
        </button>
        {Object.entries(SEVERITY_LABELS).map(([key, label]) => (
          <button
            key={key}
            type="button"
            onClick={() => setSeverity(severity === key ? '' : key)}
            className={chipClass(severity === key)}
          >
            {label}
          </button>
        ))}
      </div>

      <div className="rounded-lg border border-border/30 overflow-hidden">
        {loading ? (
          <p className="text-sm text-muted-foreground p-6 text-center">Loading…</p>
        ) : filtered.length === 0 ? (
          <p className="text-sm text-muted-foreground p-6 text-center">
            {entries.length === 0 ? 'No alerts recorded yet.' : 'No alerts match the filters.'}
          </p>
        ) : (
          filtered.map((e) => (
            <AlertRow
              key={e.id}
              entry={e}
              onRead={readOne}
              onOpen={(entry) => navigate(alertCategoryRoute[entry.category] || '/alerts')}
            />
          ))
        )}
      </div>
    </div>
  );
};
