// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

// The gaming section's confinement policy. Games are untrusted plugins running
// beside a wallet, dcrlnd and a BR identity, so they never hold wallet
// credentials: they reach one account, under these caps, through the host.
export interface GamingSettings {
  enabled: boolean;
  // account is the only wallet account games may spend from or be paid into.
  account: string;
  perTableCapDcr: number;
  perDayCapDcr: number;
  approvalTimeoutSecs: number;
  installedGames: string[];
  // gameNames is the label the operator gave each game, for display only.
  gameNames?: Record<string, string>;
  // gameTokens is what each game authenticates with - its identity, so
  // anything holding one is that game as far as the bridge is concerned. It
  // travels outward only: the server issues tokens and never reads one back.
  gameTokens?: Record<string, string>;
}

export interface GamingGame {
  // id is the routing key, and the only name the wire carries.
  id: string;
  // name is the operator's label, or the id when they gave none.
  name: string;
  // ready reports whether the game is connected to the bridge right now. A
  // game runs on a machine of the person's choosing, so it can be registered
  // and simply not running.
  ready: boolean;
}

export const getGamingSettings = async (): Promise<GamingSettings> => {
  const { data } = await api.get<GamingSettings>('/br/gaming/settings');
  return data;
};

export const setGamingSettings = async (s: GamingSettings): Promise<GamingSettings> => {
  const { data } = await api.post<GamingSettings>('/br/gaming/settings', s);
  return data;
};

export const getGamingGames = async (): Promise<GamingGame[]> => {
  const { data } = await api.get<{ games: GamingGame[] }>('/br/gaming/games');
  return data.games ?? [];
};

export type GamingSpendState = 'pending' | 'approved' | 'denied' | 'expired' | 'failed';

export interface GamingSpend {
  id: string;
  game: string;
  address: string;
  amountAtoms: number;
  reason?: string;
  state: GamingSpendState;
  txid?: string;
  error?: string;
  requestedAt: number;
  decidedAt?: number;
  expiresAt: number;
}

export const getGamingSpends = async (): Promise<GamingSpend[]> => {
  const { data } = await api.get<{ spends: GamingSpend[] }>('/br/gaming/spends');
  return data.spends ?? [];
};

// decideGamingSpend answers a game's request. Approving needs the wallet
// passphrase, because that is the only thing that can move money and it belongs
// to the person, not to the dashboard and never to the game.
export const decideGamingSpend = async (
  id: string,
  approve: boolean,
  passphrase?: string,
): Promise<GamingSpend> => {
  const { data } = await api.post<GamingSpend>('/br/gaming/spends/decide', {
    id,
    approve,
    passphrase: passphrase ?? '',
  });
  return data;
};
