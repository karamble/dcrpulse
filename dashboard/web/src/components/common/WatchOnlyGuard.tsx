// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { useWalletReady } from '../../hooks/useWalletReady';
import { WatchOnlyGate } from './WatchOnlyGate';

// WatchOnlyGuard renders a watch-only notice in place of its children when the
// active wallet is watch-only. Used to wrap route elements for spend-only
// features without threading the flag through each component.
export const WatchOnlyGuard = ({ feature, children }: { feature: string; children: ReactNode }) => {
  const { isWatchOnly } = useWalletReady();
  if (isWatchOnly) {
    return <WatchOnlyGate feature={feature} />;
  }
  return <>{children}</>;
};

// RequireWatchOnly is the inverse of WatchOnlyGuard: it renders its children only
// for a watch-only wallet and redirects a full wallet to Send. The offline-signing
// flow needs no in-app keys, so it is meaningful only for watch-only wallets.
export const RequireWatchOnly = ({ children }: { children: ReactNode }) => {
  const { isWatchOnly, loading } = useWalletReady();
  if (loading) return null;
  if (!isWatchOnly) return <Navigate to="../send" replace />;
  return <>{children}</>;
};
