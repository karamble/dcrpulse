// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Notification preferences for the DEX view, persisted client-side. They gate
// which bisonw notifications fire a desktop (OS) notification via the bell.

export type NotifCategory = 'orders' | 'bonds' | 'wallets' | 'connection' | 'security';

export interface DexNotifPrefs {
  desktop: boolean;
  categories: Record<NotifCategory, boolean>;
}

// CATEGORY_TYPES maps a category to the bisonw note-type strings it covers
// (decred.org/dcrdex/client/core/notification.go). High-frequency types (epoch,
// spots, fiatrateupdate, bot) are intentionally excluded from desktop alerts.
export const CATEGORY_TYPES: Record<NotifCategory, string[]> = {
  orders: ['order', 'match'],
  bonds: ['bondpost', 'bondrefund', 'unknownbond'],
  wallets: ['walletstate', 'walletsync', 'walletconfig', 'walletnote', 'balance', 'send', 'createwallet'],
  connection: ['conn', 'dex_auth', 'login'],
  security: ['security', 'actionrequired', 'upgrade'],
};

export const CATEGORY_LABELS: Record<NotifCategory, string> = {
  orders: 'Orders & matches',
  bonds: 'Bonds',
  wallets: 'Wallets',
  connection: 'Connection',
  security: 'Security & alerts',
};

const KEY = 'dexNotifPrefs';

const DEFAULT: DexNotifPrefs = {
  desktop: false,
  categories: { orders: true, bonds: true, wallets: false, connection: false, security: true },
};

export const loadNotifPrefs = (): DexNotifPrefs => {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return DEFAULT;
    const p = JSON.parse(raw);
    return {
      desktop: !!p.desktop,
      categories: { ...DEFAULT.categories, ...(p.categories || {}) },
    };
  } catch {
    return DEFAULT;
  }
};

export const saveNotifPrefs = (p: DexNotifPrefs): void => {
  localStorage.setItem(KEY, JSON.stringify(p));
};

// shouldNotify reports whether a note of the given bisonw type should fire a
// desktop notification under the current prefs.
export const shouldNotify = (prefs: DexNotifPrefs, noteType: string): boolean => {
  if (!prefs.desktop) return false;
  return (Object.keys(CATEGORY_TYPES) as NotifCategory[]).some(
    (c) => prefs.categories[c] && CATEGORY_TYPES[c].includes(noteType),
  );
};
