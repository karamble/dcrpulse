// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

// One registered game's confinement: what it is called, the account it may
// spend from, and how much of it. Per game because the credential a game
// presents is an identity, and a cap on a principal nobody can tell apart is
// not a cap.
export interface GamePolicy {
  // name is the operator's label; blank means the game is called by its id.
  name: string;
  // account is blank until the operator binds one, and a game with no account
  // bound can stake nothing.
  account: string;
  perTableCapDcr: number;
  perDayCapDcr: number;
  approvalTimeoutSecs: number;
}

// The bridge's own state: whether it runs, which games are registered, and what
// each of them is trusted with. Everything that costs money is per game.
export interface GamingSettings {
  enabled: boolean;
  registeredGames: string[];
  policies: Record<string, GamePolicy>;
  // gameCredentials says which games have been issued a credential and when.
  // Only the fingerprint travels: it is enough to tell two credentials apart
  // and useless for connecting with. It travels outward only - the server
  // issues credentials and never reads one back.
  gameCredentials?: Record<string, GameCredential>;
}

export interface GameCredential {
  fingerprint: string;
  issuedAt: number;
}

// What an operator carries to a game, shown once and never again. The private
// key is in this answer and in no file on the appliance.
export interface GamingCredentialMaterial {
  game: string;
  certPem: string;
  keyPem: string;
  bridgeCertPem: string;
  fingerprint: string;
  issuedAt: number;
}

// What a game's connection wizard needs besides its own credential.
export interface GamingBridgeInfo {
  bridgeCertPem: string;
  port: string;
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

export const getGamingBridgeInfo = async (): Promise<GamingBridgeInfo> => {
  const { data } = await api.get<GamingBridgeInfo>('/br/gaming/bridge');
  return data;
};

// Issuing replaces any credential the game already had, which is what
// regenerating means: the old one stops working at once.
export const issueGamingCredential = async (game: string): Promise<GamingCredentialMaterial> => {
  const { data } = await api.post<GamingCredentialMaterial>('/br/gaming/credential', { game });
  return data;
};

export const revokeGamingCredential = async (game: string): Promise<void> => {
  await api.post('/br/gaming/credential/revoke', { game });
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
