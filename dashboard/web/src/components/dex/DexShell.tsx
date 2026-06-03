// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Bot, CandlestickChart, ListOrdered, Lock, Settings, ShieldCheck, Wallet } from 'lucide-react';
import { DexMarketView } from './DexMarketView';
import { DexWalletsPanel } from './DexWalletsPanel';
import { DexOrdersHistoryPanel } from './DexOrdersHistoryPanel';
import { DexAccountPanel } from './DexAccountPanel';
import { DexSettingsPanel } from './DexSettingsPanel';
import { DexMMPanel } from './DexMMPanel';
import { DexNotifications } from './DexNotifications';

// The canonical mainnet DEX server.
const HOST = 'dex.decred.org:7232';

export type DexTab = 'trade' | 'wallets' | 'orders' | 'account' | 'mm' | 'settings';

const tabs: { id: DexTab; label: string; Icon: typeof Wallet }[] = [
  { id: 'trade', label: 'Trade', Icon: CandlestickChart },
  { id: 'wallets', label: 'Wallets', Icon: Wallet },
  { id: 'orders', label: 'Orders', Icon: ListOrdered },
  { id: 'account', label: 'Account', Icon: ShieldCheck },
  { id: 'mm', label: 'Market Maker', Icon: Bot },
  { id: 'settings', label: 'Settings', Icon: Settings },
];

// DexShell is the registered-account view. It hosts the DEX sub-pages behind a
// local-state sub-nav so switching tabs does not unmount the trading grid via
// the router. The trading terminal lives under the Trade tab.
export const DexShell = ({ initialTab = 'trade', onLock }: { initialTab?: DexTab; onLock?: () => void }) => {
  const [tab, setTab] = useState<DexTab>(initialTab);

  return (
    <div className="space-y-3">
      <nav className="flex items-center gap-2 border-b border-border px-4">
        <div className="flex items-center gap-2 overflow-x-auto">
          {tabs.map(({ id, label, Icon }) => {
            const isActive = tab === id;
            return (
              <button
                key={id}
                type="button"
                onClick={() => setTab(id)}
                className={`flex items-center gap-1.5 px-4 py-2 -mb-px border-b-2 whitespace-nowrap shrink-0 text-sm transition-colors ${
                  isActive
                    ? 'border-primary text-primary font-semibold'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                }`}
              >
                <Icon className="h-4 w-4" />
                {label}
              </button>
            );
          })}
        </div>
        <div className="ml-auto flex items-center gap-1 shrink-0">
          <DexNotifications />
          {onLock && (
            <button
              type="button"
              onClick={onLock}
              title="Lock"
              className="flex items-center gap-1.5 px-2.5 py-1.5 text-sm text-muted-foreground rounded-lg hover:text-foreground hover:bg-background/50 transition-colors"
            >
              <Lock className="h-4 w-4" />
              Lock
            </button>
          )}
        </div>
      </nav>

      {tab === 'trade' && <DexMarketView />}
      {tab === 'wallets' && <DexWalletsPanel />}
      {tab === 'orders' && <DexOrdersHistoryPanel host={HOST} />}
      {tab === 'account' && <DexAccountPanel host={HOST} />}
      {tab === 'mm' && <DexMMPanel />}
      {tab === 'settings' && <DexSettingsPanel />}
    </div>
  );
};
