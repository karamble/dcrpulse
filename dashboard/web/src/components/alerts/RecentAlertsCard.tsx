// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link, useNavigate } from 'react-router-dom';
import { Bell } from 'lucide-react';
import type { AlertEntry } from '../../services/api';
import { useAlertEntries } from '../../hooks/useAlertEntries';
import { AlertRow, alertCategoryRoute } from './AlertRow';

const recentFive = (all: AlertEntry[]) => all.slice(0, 5);

// RecentAlertsCard is the quiet-state anchor on the Node page: the pill
// disappears once everything is read, so this card is where alert history
// stays discoverable.
export const RecentAlertsCard = () => {
  const { entries, loading, readOne } = useAlertEntries({ pick: recentFive, pollMs: 60000 });
  const navigate = useNavigate();

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50 hover:border-primary/20 transition duration-300">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Bell className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Recent Alerts</h3>
          <p className="text-sm text-muted-foreground">Operator events across the stack</p>
        </div>
        <Link
          to="/alerts"
          className="ml-auto text-xs text-primary hover:text-primary/80 transition-colors"
        >
          View all
        </Link>
      </div>
      <div className="rounded-lg border border-border/30 overflow-hidden">
        {loading ? (
          <p className="text-sm text-muted-foreground p-4 text-center">Loading…</p>
        ) : entries.length === 0 ? (
          <p className="text-sm text-muted-foreground p-4 text-center">
            All quiet. Alerts will appear here.
          </p>
        ) : (
          entries.map((e) => (
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
