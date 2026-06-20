// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode } from 'react';
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
