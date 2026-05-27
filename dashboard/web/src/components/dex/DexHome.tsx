// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Lock, TrendingUp } from 'lucide-react';
import { getDexExchanges, lockDex, type DexExchange } from '../../services/dcrdexApi';
import { DexLiveProvider } from './DexLiveProvider';
import { DexRegister } from './DexRegister';
import { DexShell, type DexTab } from './DexShell';

// The canonical mainnet DEX server.
const MAINNET_DEX = 'dex.decred.org:7232';

interface DexHomeProps {
  bisonwVersion?: string;
  onLock: () => void;
}

// DexHome is the unlocked DEX view. It shows the registration screen until the
// account is registered with a DEX server, then the trading view.
export const DexHome = ({ bisonwVersion, onLock }: DexHomeProps) => {
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
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4 px-4">
        <div className="flex items-center gap-2">
          <TrendingUp className="h-5 w-5 text-primary" />
          <h1 className="text-lg font-semibold">DCRDEX</h1>
          {bisonwVersion && (
            <span className="text-xs text-muted-foreground font-mono">bisonw {bisonwVersion}</span>
          )}
        </div>
        <button
          type="button"
          onClick={async () => {
            await lockDex();
            onLock();
          }}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 transition-colors"
        >
          <Lock className="h-4 w-4" />
          Lock
        </button>
      </div>

      {exchanges === null ? (
        <div className="flex justify-center py-12">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
        </div>
      ) : registered || forcedTab ? (
        <DexShell initialTab={forcedTab ?? 'trade'} />
      ) : (
        <DexRegister host={MAINNET_DEX} onRegistered={refresh} />
      )}
    </div>
    </DexLiveProvider>
  );
};
