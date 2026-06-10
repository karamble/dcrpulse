// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { getDexExchanges, type DexExchange } from '../../services/dcrdexApi';
import { DexLiveProvider } from './DexLiveProvider';
import { DexRegister } from './DexRegister';
import { DexShell, type DexTab } from './DexShell';

// The canonical mainnet DEX server.
const MAINNET_DEX = 'dex.decred.org:7232';

interface DexHomeProps {
  onLock: () => void;
}

// DexHome is the unlocked DEX view. It shows the registration screen until the
// account is registered with a DEX server, then the trading view. The lock
// control lives in the trading view's sub-nav (next to notifications).
export const DexHome = ({ onLock }: DexHomeProps) => {
  const [exchanges, setExchanges] = useState<Record<string, DexExchange> | null>(null);

  const refresh = () => {
    getDexExchanges()
      .then(setExchanges)
      .catch(() => setExchanges({}));
  };
  useEffect(refresh, []);

  const registered = !!exchanges && Object.values(exchanges).some((x) => x && x.acctID);

  // Dev affordance: /dex?tab=<wallets|orders|account|trade> opens the shell at a
  // given tab and bypasses the registration gate, so the non-trading tabs can be
  // reviewed while the DEX server (and thus registration) is unreachable.
  const tabParam = new URLSearchParams(window.location.search).get('tab');
  const forcedTab = (['trade', 'wallets', 'orders', 'account', 'settings'] as const).find((t) => t === tabParam) as
    | DexTab
    | undefined;

  return (
    <DexLiveProvider>
      {exchanges === null ? (
        <div className="flex justify-center py-12">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
        </div>
      ) : registered || forcedTab ? (
        <DexShell initialTab={forcedTab ?? 'trade'} onLocked={onLock} />
      ) : (
        <DexRegister host={MAINNET_DEX} onRegistered={refresh} />
      )}
    </DexLiveProvider>
  );
};
