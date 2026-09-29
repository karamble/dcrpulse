// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Bell, CheckCheck } from 'lucide-react';
import { useAlertEntries } from '../hooks/useAlertEntries';
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
  const { entries, loading, readOne, readAll } = useAlertEntries({ pollMs: 60000 });
  const [category, setCategory] = useState('');
  const [severity, setSeverity] = useState('');
  const navigate = useNavigate();

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
