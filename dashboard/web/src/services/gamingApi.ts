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
  mode: 'approval' | 'autopay';
  perTableCapDcr: number;
  perDayCapDcr: number;
  // maxOpenTables bounds concurrently funded escrows. Distinct from the
  // per-table cap because stakes overlap: total outstanding is the real
  // exposure, not the largest single buy-in.
  maxOpenTables: number;
  approvalTimeoutSecs: number;
  installedGames: string[];
}

export interface GamingGame {
  id: string;
  name: string;
  description: string;
  // protocolVersion is the `gv=` key in the Bison Relay wire envelope.
  protocolVersion: number;
  installed: boolean;
  // ready reports whether the game's backend is reachable. A game can be
  // installed and not ready while its service is starting or absent.
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

// acceptGamingInvite hands an accepted invitation to the game that can act on
// it. The dashboard speaks to the game as the host; what the terms mean is the
// game's business, not this one's.
export const acceptGamingInvite = async (
  game: string,
  invite: string,
  gcid: string,
): Promise<void> => {
  await api.post('/br/gaming/invite', { game, invite, gcid });
};
