// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { getAlertsSummary } from '../services/api';
import { createPolledStore } from './polledStore';

export interface AlertsSummarySnapshot {
  unread: number;
  active: number;
  severity: '' | 'info' | 'warning' | 'critical';
  loading: boolean;
}

const store = createPolledStore<AlertsSummarySnapshot>({
  ms: 15000,
  initial: { unread: 0, active: 0, severity: '', loading: true },
  load: async () => {
    const s = await getAlertsSummary();
    return { unread: s.unread ?? 0, active: s.active ?? 0, severity: s.severity || '', loading: false };
  },
  // Keep the last known counts on a transient failure so the pill does not
  // flicker away because one poll missed.
  onError: (_err, prev) => ({ ...prev, loading: false }),
});

// refreshAlertsSummary refetches immediately: called after read mutations and
// from the live "alerts" nudge on the BR event socket.
export const refreshAlertsSummary = store.refresh;

// useAlertsSummary is the pill's data source: one 15s poll shared app-wide,
// refreshed out of band by the "alerts" nudge.
export function useAlertsSummary(): AlertsSummarySnapshot {
  return store.use();
}
