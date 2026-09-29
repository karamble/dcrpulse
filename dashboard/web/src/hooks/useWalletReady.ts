// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { getWalletStatus } from '../services/api';
import { createPolledStore } from './polledStore';

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

const store = createPolledStore<WalletReadiness>({
  ms: 5000,
  initial: { ready: false, message: '', progress: 0, loading: true, isWatchOnly: false, version: '' },
  load: async () => {
    const s = await getWalletStatus();
    const ready = s.status === 'synced';
    return {
      ready,
      message: ready ? '' : s.syncMessage || 'Your wallet is still syncing.',
      progress: typeof s.syncProgress === 'number' ? s.syncProgress : 0,
      loading: false,
      isWatchOnly: !!s.isWatchOnly,
      version: s.version || '',
    };
  },
  onError: (err: any) => {
    const body = err?.response?.data;
    return {
      ready: false,
      message: typeof body === 'string' && body ? body : 'Your wallet is still syncing.',
      progress: 0,
      loading: false,
      isWatchOnly: false,
      version: '',
    };
  },
});

// useWalletReady reports whether the wallet is synced and responsive, from one
// 5s status poll shared app-wide. A 503 (dcrd still in initial block download)
// or any non-synced status is treated as not-ready, with a human-readable
// message.
export function useWalletReady(): WalletReadiness {
  return store.use();
}
