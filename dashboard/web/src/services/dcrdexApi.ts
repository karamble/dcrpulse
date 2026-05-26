// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

export type DexStage = 'unavailable' | 'needs-init' | 'needs-unlock' | 'needs-wallet' | 'ready';

export interface DexStatus {
  reachable: boolean;
  initialized: boolean;
  unlocked: boolean;
  stage: DexStage;
  bisonwVersion?: string;
  rpcServerVersion?: string;
  error?: string;
}

export const getDexStatus = async (): Promise<DexStatus> => {
  const { data } = await api.get<DexStatus>('/dcrdex/status');
  return data;
};

// initDex initializes the bisonw client with a user-set app password (optionally
// restoring from a hex seed) and unlocks it for the session.
export const initDex = async (appPass: string, seed?: string): Promise<void> => {
  await api.post('/dcrdex/init', { appPass, seed: seed || undefined });
};

export const unlockDex = async (appPass: string): Promise<void> => {
  await api.post('/dcrdex/unlock', { appPass });
};

export const lockDex = async (): Promise<void> => {
  await api.post('/dcrdex/lock');
};

// createDexWallet configures DCRDEX's Decred wallet against the dashboard's
// dcrwallet, creating a dedicated `dex` account using the wallet passphrase.
export const createDexWallet = async (walletPass: string): Promise<void> => {
  await api.post('/dcrdex/wallet', { walletPass });
};
