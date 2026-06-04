// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

export type DexStage = 'unavailable' | 'needs-init' | 'needs-unlock' | 'needs-wallet' | 'ready';

export interface DexStatus {
  reachable: boolean;
  initialized: boolean;
  unlocked: boolean;
  seedBackedUp: boolean;
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
// acctID is set). connectionStatus follows comms.ConnectionStatus
// (0=disconnected, 1=connected, 2=invalid cert).
export interface DexExchange {
  host: string;
  acctID: string;
  connectionStatus: number;
}

export const getDexExchanges = async (): Promise<Record<string, DexExchange>> => {
  const { data } = await api.get<Record<string, DexExchange>>('/dcrdex/exchanges');
  return data || {};
};

// DexMarketSpot is a market's last/24h snapshot, present when the client is
// connected to the server. Atomic values match msgjson.Spot; see MarketSpot.
export interface DexMarketSpot {
  rate: number;
  change24: number;
  vol24: number;
  high24: number;
  low24: number;
  bookVolume: number;
  stamp: number;
}

export interface DexMarket {
  base: string;
  quote: string;
  baseID: number;
  quoteID: number;
  lotSize: number; // atomic
  rateStep: number; // atomic message-rate
  baseConvFactor: number; // base atoms per conventional unit
  quoteConvFactor: number; // quote atoms per conventional unit
  spot?: DexMarketSpot; // last/24h snapshot when connected
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
  candleDurs: string[];
}

// discoverDexAccount re-discovers the account on a DEX server (after a seed
// restore) and reports whether it already has a live bond (paid). Uses the
// extended timeout since it opens a DEX-server connection.
export const discoverDexAccount = async (host: string): Promise<{ paid: boolean }> => {
  const { data } = await api.post<{ paid: boolean }>('/dcrdex/discover-account', { host }, {
    timeout: 125000,
  });
  return data;
};

export const getDexConfig = async (host: string): Promise<DexConfig> => {
  // The backend allows up to 2 minutes for an unregistered host's one-shot
  // getdexconfig, past the 25s default client timeout, so extend this call.
  const { data } = await api.get<DexConfig>('/dcrdex/dexconfig', {
    params: { host },
    timeout: 125000,
  });
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

// newDexDepositAddress fetches a fresh Decred deposit address for the DEX wallet
// (dcrwallet hands out its next unused address), avoiding deposit-address reuse.
export const newDexDepositAddress = async (): Promise<string> => {
  const { data } = await api.post<{ address: string }>('/dcrdex/wallet/new-address');
  return data.address;
};

// dexAddressUsed reports whether a Decred address has already received funds.
export const dexAddressUsed = async (addr: string): Promise<boolean> => {
  const { data } = await api.get<{ used: boolean }>('/dcrdex/wallet/address-used', {
    params: { addr },
  });
  return !!data.used;
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

// DexNote is a bisonw notification. severity: 0 data, 1 poke, 2 success,
// 3 warning, 4 error. stamp is in milliseconds.
export interface DexNote {
  type: string;
  topic: string;
  subject: string;
  details: string;
  severity: number;
  stamp: number;
  acked: boolean;
  id: string;
}

export const getDexNotifications = async (n = 50): Promise<DexNote[]> => {
  const { data } = await api.get<DexNote[]>('/dcrdex/notifications', { params: { n } });
  return data || [];
};

// exportDexSeed returns the bisonw app seed for backup. Requires re-entering the
// app password; the seed is never persisted.
export const exportDexSeed = async (appPass: string): Promise<string> => {
  const { data } = await api.post<{ seed: string }>('/dcrdex/seed', { appPass });
  return data.seed;
};

// markDexSeedBackedUp records that the user has backed up the app seed, clearing
// the unlock backup reminder.
export const markDexSeedBackedUp = async (): Promise<void> => {
  await api.post('/dcrdex/seed/backed-up');
};

// DexRates maps an asset symbol (lowercase) to its USD price, sourced from
// Kraken (with a Bison Relay fallback). Covers the coins those sources list;
// others are absent.
export type DexRates = Record<string, number>;

export const getDexRates = async (): Promise<DexRates> => {
  const { data } = await api.get<DexRates>('/dcrdex/rates');
  return data || {};
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
  maxBondedDcr: number;
  penaltyComps: number;
  bondsPendingRefund: number;
  bondAssets: DexBondAsset[];
  autoRenew: boolean;
  pendingBonds: DexPendingBond[];
}

export interface DexBondAsset {
  symbol: string;
  assetID: number;
}

export const getDexAccount = async (host: string): Promise<DexAccount> => {
  const { data } = await api.get<DexAccount>('/dcrdex/account', { params: { host } });
  return data;
};

// DexBondOptions are the auto-bond maintenance options; any omitted field is
// left unchanged. targetTier 0 disables auto-renewal; maxBondedDcr is
// conventional (0 resets to the server default).
export interface DexBondOptions {
  targetTier?: number;
  maxBondedDcr?: number;
  bondAssetID?: number;
  penaltyComps?: number;
}

export const setDexBondOptions = async (host: string, opts: DexBondOptions): Promise<void> => {
  await api.post('/dcrdex/bondopts', { host, ...opts });
};

// Market-maker (bisonw mm) types. The dashboard proxies bisonw's webserver MM
// API; these mirror decred.org/dcrdex/client/mm (v1.0.6) for the fields the UI
// uses. Atomic amounts are keyed by asset ID. v1.0.6 supports Binance/BinanceUS.
export type MMGapStrategy = 'multiplier' | 'absolute' | 'absolute-plus' | 'percent' | 'percent-plus';

export interface MMOrderPlacement {
  lots: number;
  gapFactor: number;
}
export interface MMArbPlacement {
  lots: number;
  multiplier: number;
}
export interface MMBasicConfig {
  gapStrategy: MMGapStrategy;
  buyPlacements: MMOrderPlacement[];
  sellPlacements: MMOrderPlacement[];
  driftTolerance: number;
}
export interface MMSimpleArbConfig {
  profitTrigger: number;
  maxActiveArbs: number;
  numEpochsLeaveOpen: number;
}
export interface MMArbConfig {
  buyPlacements: MMArbPlacement[];
  sellPlacements: MMArbPlacement[];
  profit: number;
  driftTolerance: number;
  orderPersistence: number;
}
export interface MMAllocation {
  dex: Record<number, number>;
  cex: Record<number, number>;
}
export interface MMBotConfig {
  host: string;
  baseID: number;
  quoteID: number;
  cexName?: string;
  lotSize?: number;
  alloc?: MMAllocation;
  basicMarketMakingConfig?: MMBasicConfig;
  simpleArbConfig?: MMSimpleArbConfig;
  arbMarketMakingConfig?: MMArbConfig;
}
export interface MMBotBalance {
  available: number;
  locked: number;
  pending: number;
  reserved: number;
}
export interface MMRunStats {
  startTime: number;
  completedMatches: number;
  tradedUSD: number;
  profitLoss: { profit: number; profitRatio: number };
  dexBalances: Record<number, MMBotBalance>;
  cexBalances: Record<number, MMBotBalance>;
  pendingDeposits: number;
  pendingWithdrawals: number;
  feeGap?: { basisPrice: number; feeGap: number; remoteGap: number; roundTripFees: number };
}
export interface MMEpochReport {
  epochNum: number;
  buysReport: unknown | null;
  sellsReport: unknown | null;
  preOrderProblems: unknown | null;
}
export interface MMBotStatus {
  config: MMBotConfig;
  running: boolean;
  runStats: MMRunStats | null;
  latestEpoch: MMEpochReport | null;
  cexProblems: unknown | null;
}
export interface MMCexStatus {
  config?: { name: string };
  connected: boolean;
  connectErr?: string;
  // markets is the CEX's supported market list (bisonw libxc.Market), keyed by
  // the CEX's own market id; used to tell which pairs a CEX can arbitrage.
  markets?: Record<string, { baseID: number; quoteID: number }>;
}
export interface MMStatus {
  bots: MMBotStatus[];
  cexes: Record<string, MMCexStatus>;
}
// MMStartConfig is bisonw's mm.StartConfig: the market plus optional allocation
// and auto-rebalance. The config must already be saved via updateMMBotConfig.
export interface MMStartConfig {
  host: string;
  baseID: number;
  quoteID: number;
  alloc?: MMAllocation;
  autoRebalance?: { minBaseTransfer: number; minQuoteTransfer: number; internalOnly?: boolean };
}
export interface MMCexConfig {
  name: string;
  apiKey: string;
  apiSecret: string;
}
// MMOracleReport summarizes one external exchange's view of the market, as
// reported by bisonw's price oracles.
export interface MMOracleReport {
  host: string;
  usdVol: number;
  bestBuy: number;
  bestSell: number;
}
// MMMarketReport is bisonw's mm.MarketReport: the aggregate oracle price, the
// per-oracle breakdown, and the base/quote fiat rates used for USD conversion.
export interface MMMarketReport {
  price: number;
  oracles: MMOracleReport[] | null;
  baseFiatRate: number;
  quoteFiatRate: number;
}

export const getMMStatus = async (): Promise<MMStatus | null> => {
  const { data } = await api.get<MMStatus | null>('/dcrdex/mm/status');
  return data;
};
export const getMMMarketReport = async (
  host: string,
  baseID: number,
  quoteID: number,
): Promise<MMMarketReport | null> => {
  const { data } = await api.get<MMMarketReport | null>('/dcrdex/mm/marketreport', {
    params: { host, baseID, quoteID },
  });
  return data;
};
export const updateMMBotConfig = async (cfg: MMBotConfig): Promise<void> => {
  await api.post('/dcrdex/mm/config', cfg);
};
export const removeMMBotConfig = async (host: string, baseID: number, quoteID: number): Promise<void> => {
  await api.post('/dcrdex/mm/config/remove', { host, baseID, quoteID });
};
export const updateMMCexConfig = async (cfg: MMCexConfig): Promise<void> => {
  await api.post('/dcrdex/mm/cexconfig', cfg);
};
export const startMMBot = async (cfg: MMStartConfig): Promise<void> => {
  await api.post('/dcrdex/mm/start', cfg);
};
export const stopMMBot = async (host: string, baseID: number, quoteID: number): Promise<void> => {
  await api.post('/dcrdex/mm/stop', { host, baseID, quoteID });
};
