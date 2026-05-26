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

// DexExchange is the minimal shape of a known DEX server (registered when
// acctID is set).
export interface DexExchange {
  host: string;
  acctID: string;
}

export const getDexExchanges = async (): Promise<Record<string, DexExchange>> => {
  const { data } = await api.get<Record<string, DexExchange>>('/dcrdex/exchanges');
  return data || {};
};

export interface DexMarket {
  base: string;
  quote: string;
  baseID: number;
  quoteID: number;
  lotSize: number; // atomic
  rateStep: number; // atomic message-rate
  baseConvFactor: number; // base atoms per conventional unit
  quoteConvFactor: number; // quote atoms per conventional unit
}

export interface PlaceOrderParams {
  host: string;
  base: number;
  quote: number;
  isLimit: boolean;
  sell: boolean;
  qty: number; // atomic
  rate: number; // atomic message-rate (0 for market orders)
  tifNow: boolean;
}

// placeDexOrder submits an order. qty and rate are atomic; the caller converts
// from conventional units mirroring bisonw's frontend. Spends real funds.
export const placeDexOrder = async (p: PlaceOrderParams): Promise<void> => {
  await api.post('/dcrdex/trade', p);
};

// DexConfig is the registration view of a DEX server. The backend converts the
// bond amount to DCR (via dcrutil), so the frontend does no atoms math.
export interface DexConfig {
  host: string;
  connectionStatus: number;
  registered: boolean;
  bondExpiryDays: number;
  bondConfs: number;
  bondPerTierAtoms: number;
  bondPerTierDcr: number;
  marketCount: number;
  markets: DexMarket[];
}

export const getDexConfig = async (host: string): Promise<DexConfig> => {
  const { data } = await api.get<DexConfig>('/dcrdex/dexconfig', { params: { host } });
  return data;
};

// DexWalletInfo is the DCRDEX Decred wallet's funding view (balance in DCR,
// converted in the backend) used to gate bond posting.
export interface DexWalletInfo {
  configured: boolean;
  availableDcr: number;
  address: string;
  synced: boolean;
  syncProgress: number;
}

export const getDexWallet = async (): Promise<DexWalletInfo> => {
  const { data } = await api.get<DexWalletInfo>('/dcrdex/wallet');
  return data;
};

// DexOrder is a user order from the myorders route (amounts are atomic).
export interface DexOrder {
  id: string;
  host: string;
  marketName: string;
  type: string;
  sell: boolean;
  status: string;
  quantity: number;
  filled: number;
  rate: number;
}

export const getDexMyOrders = async (host?: string): Promise<DexOrder[]> => {
  const { data } = await api.get<DexOrder[]>('/dcrdex/myorders', { params: host ? { host } : {} });
  return data || [];
};

export const cancelDexOrder = async (orderID: string): Promise<void> => {
  await api.post('/dcrdex/cancel', { orderID });
};

// postDexBond posts a fidelity bond (in atoms) to register with a DEX server.
// This spends real funds; only call on explicit user action.
export const postDexBond = async (host: string, bond: number): Promise<void> => {
  await api.post('/dcrdex/postbond', { host, bond });
};
