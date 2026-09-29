// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import { AlertEntry, getAlerts, markAlertRead, markAllAlertsRead } from '../services/api';
import { useBisonrelayLive } from '../components/bisonrelay/BisonrelayLiveProvider';
import { refreshAlertsSummary } from './useAlerts';
import { useVisiblePoll } from './useVisiblePoll';

// useAlertEntries loads the alert list for the panel, the Node page card and
// the Alerts page: pick narrows it, pollMs (0 = no poll) refreshes it, and the
// live "alerts" nudge reloads it. Reads are marked optimistically and reloaded
// if the server refuses them.
export function useAlertEntries({
  pick,
  pollMs = 0,
}: { pick?: (all: AlertEntry[]) => AlertEntry[]; pollMs?: number } = {}) {
  const [entries, setEntries] = useState<AlertEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const { addListener } = useBisonrelayLive();
  const pickRef = useRef(pick);
  pickRef.current = pick;

  const load = useCallback(
    () =>
      getAlerts()
        .then((all) => setEntries(pickRef.current ? pickRef.current(all) : all))
        .catch(() => {})
        .finally(() => setLoading(false)),
    [],
  );

  useEffect(() => {
    void load();
  }, [load]);
  useVisiblePoll(load, pollMs, { enabled: pollMs > 0, immediate: false });
  useEffect(
    () =>
      addListener((evt) => {
        if (evt.type === 'alerts') void load();
      }),
    [addListener, load],
  );

  const mark = async (update: (e: AlertEntry) => AlertEntry, send: () => Promise<unknown>) => {
    setEntries((prev) => prev.map(update));
    try {
      await send();
    } catch {
      void load();
    }
    refreshAlertsSummary();
  };
  const readOne = (id: string) => mark((e) => (e.id === id ? { ...e, read: true } : e), () => markAlertRead(id));
  const readAll = () => mark((e) => ({ ...e, read: true }), markAllAlertsRead);

  return { entries, loading, readOne, readAll };
}
