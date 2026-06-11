// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, Bot, CandlestickChart, ListOrdered, Lock, Settings, ShieldCheck, Wallet, X } from 'lucide-react';
import { lockDex } from '../../services/dcrdexApi';
import { DexMarketView } from './DexMarketView';
import { DexWalletsPanel } from './DexWalletsPanel';
import { DexOrdersHistoryPanel } from './DexOrdersHistoryPanel';
import { DexAccountPanel } from './DexAccountPanel';
import { DexSettingsPanel } from './DexSettingsPanel';
import { DexMMPanel } from './DexMMPanel';
import { DexNotifications } from './DexNotifications';
import { DexServerBanner } from './DexServerBanner';

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
export const DexShell = ({ initialTab = 'trade', onLocked }: { initialTab?: DexTab; onLocked?: () => void }) => {
  const [tab, setTab] = useState<DexTab>(initialTab);
  const [locking, setLocking] = useState(false);
  const [lockErr, setLockErr] = useState<string | null>(null);

  // Lock only flips the dashboard to the locked screen when bisonw actually
  // locked. It refuses while any order is still active, so the daemon's message
  // is surfaced and the DEX stays unlocked (state always matches the daemon).
  const attemptLock = async () => {
    setLocking(true);
    setLockErr(null);
    try {
      await lockDex();
      onLocked?.();
    } catch (e: any) {
      setLockErr(e?.response?.data || e?.message || 'Failed to lock');
    } finally {
      setLocking(false);
    }
  };

  return (
    <div className="space-y-3">
      <DexServerBanner host={HOST} />
      <nav className="flex items-center gap-2 border-b border-border px-4">
        <div className="flex items-center gap-2 overflow-x-auto overflow-y-hidden">
          {tabs.map(({ id, label, Icon }) => {
            const isActive = tab === id;
            return (
              <button
                key={id}
                type="button"
                onClick={() => setTab(id)}
                className={`flex items-center gap-1.5 px-4 py-2 border-b-2 whitespace-nowrap shrink-0 text-sm transition-colors ${
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
          {onLocked && (
            <button
              type="button"
              onClick={attemptLock}
              disabled={locking}
              title="Lock"
              className="flex items-center gap-1.5 px-2.5 py-1.5 text-sm text-muted-foreground rounded-lg hover:text-foreground hover:bg-background/50 transition-colors disabled:opacity-50"
            >
              <Lock className="h-4 w-4" />
              {locking ? 'Locking…' : 'Lock'}
            </button>
          )}
        </div>
      </nav>

      {lockErr && (
        <div className="mx-4 p-3 rounded-lg bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <div className="min-w-0 flex-1">
            <div className="break-words">{lockErr}</div>
            <div className="mt-0.5 text-xs text-destructive/80">
              To lock, stop all market-maker bots, cancel all standing orders, and let matched trades finish settling, then lock again.
            </div>
          </div>
          <button
            type="button"
            onClick={() => setLockErr(null)}
            aria-label="Dismiss"
            className="p-0.5 rounded hover:bg-destructive/10 shrink-0"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {tab === 'trade' && <DexMarketView />}
      {tab === 'wallets' && <DexWalletsPanel />}
      {tab === 'orders' && <DexOrdersHistoryPanel host={HOST} />}
      {tab === 'account' && <DexAccountPanel host={HOST} />}
      {tab === 'mm' && <DexMMPanel />}
      {tab === 'settings' && <DexSettingsPanel />}
    </div>
  );
};
