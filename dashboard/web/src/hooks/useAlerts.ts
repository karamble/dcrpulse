// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useSyncExternalStore } from 'react';
import { getAlertsSummary } from '../services/api';
import { startVisiblePoll } from './useVisiblePoll';
import { shallowEqual } from '../utils/shallowEqual';

export interface AlertsSummarySnapshot {
  unread: number;
  active: number;
  severity: '' | 'info' | 'warning' | 'critical';
  loading: boolean;
}

const POLL_MS = 15000;

let snapshot: AlertsSummarySnapshot = { unread: 0, active: 0, severity: '', loading: true };
const listeners = new Set<() => void>();
let stopPoll: (() => void) | null = null;
let inflight = false;

async function check() {
  if (inflight) return;
  inflight = true;
  let next: AlertsSummarySnapshot;
  try {
    const s = await getAlertsSummary();
    next = {
      unread: s.unread ?? 0,
      active: s.active ?? 0,
      severity: s.severity || '',
      loading: false,
    };
  } catch {
    // Keep the last known counts on a transient failure so the pill does not
    // flicker away because one poll missed.
    next = { ...snapshot, loading: false };
  } finally {
    inflight = false;
  }
  if (!shallowEqual(next, snapshot)) {
    snapshot = next;
    listeners.forEach((l) => l());
  }
}

// refreshAlertsSummary refetches immediately: called after read mutations and
// from the live "alerts" nudge on the BR event socket.
export function refreshAlertsSummary() {
  void check();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  if (listeners.size === 1) {
    stopPoll = startVisiblePoll(check, POLL_MS);
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) {
      stopPoll?.();
      stopPoll = null;
    }
  };
}

function getSnapshot(): AlertsSummarySnapshot {
  return snapshot;
}

// useAlertsSummary is the pill's data source: one 15s visibility-gated poll
// shared app-wide (same store shape as useWalletReady), refreshed out of band
// by the "alerts" nudge. Flat snapshot only, per the shallowEqual dedupe.
export function useAlertsSummary(): AlertsSummarySnapshot {
  return useSyncExternalStore(subscribe, getSnapshot);
}
