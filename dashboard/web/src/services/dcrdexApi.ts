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

// DexMatch is a single match within an order (amounts are atomic). Swap-related
// fields hold the on-chain coin IDs when present.
export interface DexMatch {
  matchID: string;
  status: string;
  revoked: boolean;
  rate: number;
  qty: number;
  side: string;
  feeRate: number;
  stamp: number;
  isCancel: boolean;
  swap?: string;
  counterSwap?: string;
  redeem?: string;
  counterRedeem?: string;
  refund?: string;
}

// DexOrder is a user order from the myorders route (amounts are atomic; rate is
// an atomic message-rate). The myorders route returns active and recently
// tracked orders across all markets, not the full archived history.
export interface DexOrder {
  id: string;
  host: string;
  marketName: string;
  baseID: number;
  quoteID: number;
  type: string;
  sell: boolean;
  status: string;
  stamp: number;
  submitTime: number;
  quantity: number;
  filled: number;
  settled: number;
  rate: number;
  cancelling?: boolean;
  canceled?: boolean;
  tif?: string;
  matches?: DexMatch[];
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

// DexWalletState is a DCRDEX-managed wallet's funding view. Balances are in
// conventional units (converted in the backend).
export interface DexWalletState {
  assetID: number;
  symbol: string;
  walletType: string;
  traits: number;
  running: boolean;
  open: boolean;
  encrypted: boolean;
  disabled: boolean;
  synced: boolean;
  syncProgress: number;
  peerCount: number;
  units: string;
  address: string;
  available: number;
  locked: number;
  immature: number;
  orderLocked: number;
  bondLocked: number;
}

export const getDexWallets = async (): Promise<DexWalletState[]> => {
  const { data } = await api.get<DexWalletState[]>('/dcrdex/wallets');
  return data || [];
};

// WalletTrait bits (decred.org/dcrdex/client/asset). Used to gate per-wallet
// actions, mirroring the upstream wallet UI.
export const WalletTrait = {
  Rescanner: 1 << 0,
  Withdrawer: 1 << 6,
  PeerManager: 1 << 10,
  Historian: 1 << 16,
} as const;

export const hasTrait = (traits: number, bit: number): boolean => (traits & bit) !== 0;

// DexConfigOption is one field in a wallet's config form (from the catalog).
export interface DexConfigOption {
  key: string;
  displayName: string;
  description: string;
  default: string;
  noEcho: boolean;
  isBoolean: boolean;
  isDate: boolean;
  repeatable?: string;
  required: boolean;
}

// DexWalletDefinition is one wallet type available for an asset.
export interface DexWalletDefinition {
  type: string;
  tab: string;
  seeded: boolean;
  description: string;
  configPath?: string;
  guideLink?: string;
  noAuth: boolean;
  configOpts: DexConfigOption[];
}

export interface DexUnitInfo {
  atomicUnit: string;
  conventionalUnit: string;
  conversionFactor: number;
}

export interface DexAssetToken {
  id: number;
  symbol: string;
  name: string;
  parentID: number;
  unitInfo: DexUnitInfo;
  definition: DexWalletDefinition;
}

// DexAsset is one supported coin in the catalog (served from the embedded,
// generated catalog the bisonw RPC does not expose).
export interface DexAsset {
  id: number;
  symbol: string;
  name: string;
  isAccountBased: boolean;
  unitInfo: DexUnitInfo;
  availableWallets: DexWalletDefinition[];
  tokens?: DexAssetToken[];
}

export const getDexAssetCatalog = async (): Promise<DexAsset[]> => {
  const { data } = await api.get<DexAsset[]>('/dcrdex/assets');
  return data || [];
};

// createDexAssetWallet creates a wallet for any supported asset from a
// schema-driven config map. (The DCR onboarding wallet uses createDexWallet.)
export const createDexAssetWallet = async (
  assetID: number,
  walletType: string,
  config: Record<string, string>,
  walletPass: string,
): Promise<void> => {
  await api.post('/dcrdex/wallet/create', { assetID, walletType, config, walletPass });
};

// DexWalletTx is a wallet transaction (amounts in conventional units).
export interface DexWalletTx {
  type: number;
  id: string;
  amount: number;
  fees: number;
  blockNumber: number;
  timestamp: number;
  recipient?: string;
  tokenID?: number;
}

export const getDexWalletTxs = async (assetID: number, n = 0, refID = '', past = false): Promise<DexWalletTx[]> => {
  const params: Record<string, string> = { assetID: String(assetID) };
  if (n > 0) params.n = String(n);
  if (refID) {
    params.refID = refID;
    params.past = String(past);
  }
  const { data } = await api.get<DexWalletTx[]>('/dcrdex/wallet/txs', { params });
  return data || [];
};

// sendDexWallet sends a conventional amount of an asset to an address. Spends
// real funds; only call on explicit user action.
export const sendDexWallet = async (assetID: number, value: number, address: string): Promise<string> => {
  const { data } = await api.post<{ coin: string }>('/dcrdex/wallet/send', { assetID, value, address });
  return data.coin;
};

export const openDexWallet = async (assetID: number): Promise<void> => {
  await api.post('/dcrdex/wallet/open', { assetID });
};
export const closeDexWallet = async (assetID: number): Promise<void> => {
  await api.post('/dcrdex/wallet/close', { assetID });
};
export const toggleDexWallet = async (assetID: number, disable: boolean): Promise<void> => {
  await api.post('/dcrdex/wallet/toggle', { assetID, disable });
};
export const rescanDexWallet = async (assetID: number, force = false): Promise<void> => {
  await api.post('/dcrdex/wallet/rescan', { assetID, force });
};

export interface DexWalletPeer {
  addr: string;
  source: number;
  connected: boolean;
}

export const getDexWalletPeers = async (assetID: number): Promise<DexWalletPeer[]> => {
  const { data } = await api.get<DexWalletPeer[]>('/dcrdex/wallet/peers', { params: { assetID: String(assetID) } });
  return data || [];
};
export const addDexWalletPeer = async (assetID: number, address: string): Promise<void> => {
  await api.post('/dcrdex/wallet/peers', { assetID, address });
};
export const removeDexWalletPeer = async (assetID: number, address: string): Promise<void> => {
  await api.delete('/dcrdex/wallet/peers', { data: { assetID, address } });
};

// DexPendingBond is a bond awaiting confirmations.
export interface DexPendingBond {
  symbol: string;
  assetID: number;
  confs: number;
}

// DexAccount is the per-server account view (tier, reputation, bonds). The bond
// amount is converted to DCR in the backend.
export interface DexAccount {
  host: string;
  acctID: string;
  registered: boolean;
  connectionStatus: number;
  viewOnly: boolean;
  disabled: boolean;
  targetTier: number;
  effectiveTier: number;
  bondedTier: number;
  penalties: number;
  score: number;
  penaltyThreshold: number;
  maxScore: number;
  bondAssetID: number;
  bondExpiryDays: number;
  bondPerTierAtoms: number;
  bondPerTierDcr: number;
  autoRenew: boolean;
  pendingBonds: DexPendingBond[];
}

export const getDexAccount = async (host: string): Promise<DexAccount> => {
  const { data } = await api.get<DexAccount>('/dcrdex/account', { params: { host } });
  return data;
};

// setDexBondOptions sets the auto-bond target tier (0 disables auto-renewal).
export const setDexBondOptions = async (host: string, targetTier: number): Promise<void> => {
  await api.post('/dcrdex/bondopts', { host, targetTier });
};
