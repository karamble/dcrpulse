// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useSyncExternalStore } from 'react';
import { getWalletStatus } from '../services/api';
import { startVisiblePoll } from './useVisiblePoll';
import { shallowEqual } from '../utils/shallowEqual';

export interface WalletReadiness {
  // ready is true only when dcrd is past IBD and the wallet is fully synced and
  // answering RPC (status === 'synced'). Gates first-time feature setup.
  ready: boolean;
  message: string;
  progress: number;
  loading: boolean;
  // isWatchOnly mirrors dcrwallet's watching-only flag for the active wallet;
  // gates spend features that cannot work without private keys.
  isWatchOnly: boolean;
  // version is the daemon-reported dcrwallet version, exposed so the footer
  // needs no separate /wallet/status fetch.
  version: string;
}

const POLL_MS = 5000;

let snapshot: WalletReadiness = {
  ready: false,
  message: '',
  progress: 0,
  loading: true,
  isWatchOnly: false,
  version: '',
};
const listeners = new Set<() => void>();
let stopPoll: (() => void) | null = null;
let inflight = false;

async function check() {
  if (inflight) return;
  inflight = true;
  let next: WalletReadiness;
  try {
    const s = await getWalletStatus();
    const ready = s.status === 'synced';
    next = {
      ready,
      message: ready ? '' : s.syncMessage || 'Your wallet is still syncing.',
      progress: typeof s.syncProgress === 'number' ? s.syncProgress : 0,
      loading: false,
      isWatchOnly: !!s.isWatchOnly,
      version: s.version || '',
    };
  } catch (err: any) {
    const body = err?.response?.data;
    next = {
      ready: false,
      message: typeof body === 'string' && body ? body : 'Your wallet is still syncing.',
      progress: 0,
      loading: false,
      isWatchOnly: false,
      version: '',
    };
  } finally {
    inflight = false;
  }
  if (!shallowEqual(next, snapshot)) {
    snapshot = next;
    listeners.forEach((l) => l());
  }
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

function getSnapshot(): WalletReadiness {
  return snapshot;
}

// useWalletReady reports whether the wallet is synced and responsive, backed
// by one status poller shared by the whole app: the first subscriber starts
// it, the last stops it, and a snapshot that compares equal to the previous
// one is never republished, so idle polls cause zero re-renders. A 503 (dcrd
// still in initial block download) or any non-synced status is treated as
// not-ready, with a human-readable message.
export function useWalletReady(): WalletReadiness {
  return useSyncExternalStore(subscribe, getSnapshot);
}
