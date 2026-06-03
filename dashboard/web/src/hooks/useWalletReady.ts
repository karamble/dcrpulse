// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { getWalletStatus } from '../services/api';

export interface WalletReadiness {
  // ready is true only when dcrd is past IBD and the wallet is fully synced and
  // answering RPC (status === 'synced'). Gates first-time feature setup.
  ready: boolean;
  message: string;
  progress: number;
  loading: boolean;
}

// useWalletReady polls the wallet status and reports whether the wallet is
// synced and responsive. A 503 (dcrd still in initial block download) or any
// non-synced status is treated as not-ready, with a human-readable message.
export function useWalletReady(pollMs = 4000): WalletReadiness {
  const [state, setState] = useState<WalletReadiness>({
    ready: false,
    message: '',
    progress: 0,
    loading: true,
  });

  useEffect(() => {
    let cancelled = false;
    const check = async () => {
      try {
        const s = await getWalletStatus();
        if (cancelled) return;
        const ready = s.status === 'synced';
        setState({
          ready,
          message: ready ? '' : s.syncMessage || 'Your wallet is still syncing.',
          progress: typeof s.syncProgress === 'number' ? s.syncProgress : 0,
          loading: false,
        });
      } catch (err: any) {
        if (cancelled) return;
        const body = err?.response?.data;
        setState({
          ready: false,
          message: typeof body === 'string' && body ? body : 'Your wallet is still syncing.',
          progress: 0,
          loading: false,
        });
      }
    };
    check();
    const id = window.setInterval(check, pollMs);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [pollMs]);

  return state;
}
