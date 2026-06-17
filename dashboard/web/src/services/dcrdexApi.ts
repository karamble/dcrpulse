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
  message?: string;
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
  qty: number; // atomic (base atoms; for a market buy this is quote atoms)
  rate: number; // atomic message-rate (0 for market orders)
  tifNow: boolean;
  // options maps an order-option key (e.g. the split-tx toggle) to its stringy
  // value, mirroring core.TradeForm.Options. Surfaced from the pre-order result.
  options?: Record<string, string>;
}

// placeDexOrder submits an order. qty and rate are atomic; the caller converts
// from conventional units mirroring bisonw's frontend. Spends real funds.
export const placeDexOrder = async (p: PlaceOrderParams): Promise<void> => {
  await api.post('/dcrdex/trade', p);
};

// OrderOption is an available order-time option from the pre-order estimate,
// mirroring dcrdex's asset.OrderOption (asset/options.go, asset/interface.go).
// Only boolean options (e.g. the split-tx toggle) are surfaced in the form.
export interface OrderOption {
  key: string;
  displayname: string;
  description: string;
  default?: string;
  quoteAssetOnly?: boolean;
  boolean?: { reason: string };
  xyRange?: unknown; // range options are not rendered
}

// SwapEstimate / RedeemEstimate / OrderEstimate mirror dcrdex's asset + core
// estimate structs. Fee fields are in the fee asset's atoms (swap: the from
// asset, redeem: the to asset).
export interface SwapEstimate {
  lots: number;
  value: number;
  maxFees: number;
  realisticWorstCase: number;
  realisticBestCase: number;
  feeReservesPerLot: number;
}
export interface RedeemEstimate {
  realisticWorstCase: number;
  realisticBestCase: number;
}
export interface OrderEstimate {
  swap: { estimate: SwapEstimate; options?: OrderOption[] };
  redeem: { estimate: RedeemEstimate; options?: OrderOption[] };
}
// MaxOrderEstimate is the largest fundable order from maxbuy/maxsell.
export interface MaxOrderEstimate {
  swap: SwapEstimate;
  redeem: RedeemEstimate;
}

// PreOrderForm carries the prospective order to the pre-order estimate. Same
// fields as a trade; no funds are committed.
export interface PreOrderForm {
  host: string;
  base: number;
  quote: number;
  isLimit: boolean;
  sell: boolean;
  qty: number;
  rate: number;
  tifNow: boolean;
  options?: Record<string, string>;
}

// preDexOrder fetches the swap/redeem fee estimate and available options for a
// prospective order. The backend proxies bisonw's webserver /api/preorder.
export const preDexOrder = async (form: PreOrderForm): Promise<OrderEstimate> => {
  const { data } = await api.post<OrderEstimate>('/dcrdex/preorder', form);
  return data;
};

// maxDexBuy / maxDexSell return the largest order fundable on the market. The
// buy side needs a rate (atomic message-rate); the sell side does not.
export const maxDexBuy = async (host: string, base: number, quote: number, rate: number): Promise<MaxOrderEstimate> => {
  const { data } = await api.post<MaxOrderEstimate>('/dcrdex/maxbuy', { host, base, quote, rate });
  return data;
};
export const maxDexSell = async (host: string, base: number, quote: number): Promise<MaxOrderEstimate> => {
  const { data } = await api.post<MaxOrderEstimate>('/dcrdex/maxsell', { host, base, quote });
  return data;
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

// DexCoin is an on-chain coin as carried by the core `match` notification
// (decred.org/dcrdex/client/core Coin) - unlike the myorders match, which gives
// only a hex id string, the note carries the formatted id, asset and live
// confirmation progress.
export interface DexCoin {
  id?: string;
  stringID: string;
  assetID: number;
  symbol?: string;
  confs?: { count: number; required: number };
}

// DexFullMatch is a match from the single-order route (/dcrdex/order): the same
// fields as the myorders match, but each swap coin is a DexCoin carrying its
// asset and live confirmation counts. status/side are normalized to the same
// strings the myorders match uses.
export interface DexFullMatch {
  matchID: string;
  status: string;
  revoked: boolean;
  rate: number;
  qty: number;
  side: string;
  feeRate: number;
  stamp: number;
  isCancel: boolean;
  swap?: DexCoin;
  counterSwap?: DexCoin;
  redeem?: DexCoin;
  counterRedeem?: DexCoin;
  refund?: DexCoin;
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

// DexOrderFull is one order from the single-order route, with rich (confs-bearing)
// matches. Same scalar fields as DexOrder.
export type DexOrderFull = Omit<DexOrder, 'matches'> & { matches?: DexFullMatch[] };

export const getDexMyOrders = async (host?: string): Promise<DexOrder[]> => {
  const { data } = await api.get<DexOrder[]>('/dcrdex/myorders', { params: host ? { host } : {} });
  return data || [];
};

// isCancellable mirrors bisonw's OrderUtil.isCancellable: only a standing limit
// order still in the epoch/booked stage (and not already cancelling) can be
// cancelled. A fully matched/executed order, a market order, or an
// immediate-or-cancel order is never cancellable. Field strings come from the
// myorders route (type "limit", tif "standing", status "epoch"/"booked").
export const isCancellable = (o: DexOrder): boolean =>
  o.type === 'limit' && o.tif === 'standing' && (o.status === 'booked' || o.status === 'epoch') && !o.cancelling;

// orderHasActiveMatches reports whether any of an order's matches is still
// settling (a swap not yet fully confirmed and not refunded). The myorders match
// has no `active` flag, so this infers it from the match status: a match is done
// only at MatchConfirmed. Cancel matches don't settle coins, so they're ignored.
export const orderHasActiveMatches = (o: DexOrder): boolean =>
  (o.matches || []).some((m) => !m.isCancel && !m.refund && m.status !== 'MatchConfirmed');

// orderStatusString composes the user-facing order status, mirroring bisonw's
// OrderUtil.statusString. An order that has matched but whose swaps are still
// confirming reads "Settling" (or "<status>/Settling"); it only reads "Executed"
// once every match is fully confirmed and the funds are in hand. This is why a
// freshly matched order must not be shown as "executed" outright.
export const orderStatusString = (o: DexOrder): string => {
  const live = orderHasActiveMatches(o);
  switch (o.status) {
    case 'epoch':
      return 'Epoch';
    case 'booked':
      return o.cancelling ? 'Canceling' : live ? 'Booked/Settling' : 'Booked';
    case 'executed':
      return live ? 'Settling' : o.filled === 0 && o.type !== 'cancel' ? 'No match' : 'Executed';
    case 'canceled':
      return live ? 'Canceled/Settling' : 'Canceled';
    case 'revoked':
      return live ? 'Revoked/Settling' : 'Revoked';
    default:
      return o.status || 'Unknown';
  }
};

// DexOrderFilter selects a page of the order history. status is one of
// epoch/booked/executed/canceled/revoked (omit for all); market restricts to one
// market; offset is the id of the last order from the previous page.
export interface DexOrderFilter {
  host: string;
  n?: number;
  offset?: string;
  status?: string;
  market?: { baseID: number; quoteID: number };
}

// getDexOrders returns the full order history - including canceled, executed and
// revoked orders - from the archive route, normalized to the same DexOrder shape
// as getDexMyOrders. The RPC myorders route returns only active/recent orders;
// this reads the full orders database, filterable and paginated.
export const getDexOrders = async (filter: DexOrderFilter): Promise<DexOrder[]> => {
  const { data } = await api.post<DexOrder[]>('/dcrdex/orders', filter);
  return data || [];
};

// getDexOrder fetches a single order with live swap-coin confirmation counts
// (the only source of confs; myorders and the archive omit them). Used by the
// order-detail swap tracker, refreshed on open and on order/match notes.
export const getDexOrder = async (id: string): Promise<DexOrderFull> => {
  const { data } = await api.post<DexOrderFull>('/dcrdex/order', { id });
  return data;
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
  total: number;
}

export const getDexWallets = async (): Promise<DexWalletState[]> => {
  const { data } = await api.get<DexWalletState[]>('/dcrdex/wallets');
  return data || [];
};

// newDexDepositAddress fetches a fresh deposit address for an asset's DEX wallet
// (the asset backend hands out its next unused address), avoiding address reuse.
export const newDexDepositAddress = async (assetID: number): Promise<string> => {
  const { data } = await api.post<{ address: string }>('/dcrdex/wallet/new-address', null, {
    params: { assetID },
  });
  return data.address;
};

// dexAddressUsed reports whether an address has already received funds.
export const dexAddressUsed = async (assetID: number, addr: string): Promise<boolean> => {
  const { data } = await api.get<{ used: boolean }>('/dcrdex/wallet/address-used', {
    params: { assetID, addr },
  });
  return !!data.used;
};

// WalletTrait bits (decred.org/dcrdex/client/asset). Used to gate per-wallet
// actions, mirroring the upstream wallet UI.
export const WalletTrait = {
  Rescanner: 1 << 0,
  NewAddresser: 1 << 1,
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
  dependsOn?: string;
}

// DexXYRange describes a numeric option's range (bisonw asset.XYRange); only the
// x axis is used here (start.x..end.x in xUnit) to render a slider.
export interface DexXYRange {
  start: { label: string; x: number };
  end: { label: string; x: number };
  xUnit: string;
}

// DexOrderOption is a per-order funding option (bisonw asset.OrderOption): a
// config option plus the quote-only flag and an optional numeric range. Used for
// the market-maker multi-split funding controls (multisplit/multisplitbuffer).
export interface DexOrderOption {
  key: string;
  displayName: string;
  description: string;
  default: string;
  isBoolean: boolean;
  quoteAssetOnly?: boolean;
  dependsOn?: string;
  xyRange?: DexXYRange;
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
  multiFundingOpts?: DexOrderOption[];
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

// DexAssetInfo is the catalog metadata for one wallet's asset, resolved from a
// base coin or a nested token. account-based / conversionFactor drive the
// wallet view's send/receive UX (fee handling, display precision).
export interface DexAssetInfo {
  symbol: string;
  isAccountBased: boolean;
  conversionFactor: number;
  parentSymbol?: string; // set when the asset is a token on another chain
}

// dexAssetForId resolves the catalog entry for a wallet's asset id, matching
// either a base coin or a nested token (tokens are always account-based).
export const dexAssetForId = (catalog: DexAsset[], assetID: number): DexAssetInfo | null => {
  for (const a of catalog) {
    if (a.id === assetID) {
      return { symbol: a.symbol, isAccountBased: a.isAccountBased, conversionFactor: a.unitInfo.conversionFactor };
    }
    for (const t of a.tokens ?? []) {
      if (t.id === assetID) {
        return { symbol: t.symbol, isAccountBased: true, conversionFactor: t.unitInfo.conversionFactor, parentSymbol: a.symbol };
      }
    }
  }
  return null;
};

// multiFundingOptsForAsset returns an asset's multi-order funding options (the
// multisplit/multisplitbuffer controls). They are identical across an asset's
// wallet types, so the first definition that declares them is used.
export const multiFundingOptsForAsset = (catalog: DexAsset[], assetID: number): DexOrderOption[] => {
  const defsOf = (a: DexAsset): DexWalletDefinition[] => {
    if (a.id === assetID) return a.availableWallets;
    const t = (a.tokens ?? []).find((tok) => tok.id === assetID);
    return t ? [t.definition] : [];
  };
  for (const a of catalog) {
    for (const def of defsOf(a)) {
      if (def.multiFundingOpts && def.multiFundingOpts.length > 0) return def.multiFundingOpts;
    }
  }
  return [];
};

export interface DexSendFee {
  fee: number;
  feeSymbol: string;
  validAddress: boolean;
}

// estimateDexSendFee returns the estimated network fee (in the fee asset's
// conventional units, with its symbol - the parent chain for a token) and whether
// the address is valid for the asset. Backed by the bisonw webserver /api/txfee.
export const estimateDexSendFee = async (
  assetID: number,
  value: number,
  address: string,
  subtract = false,
): Promise<DexSendFee> => {
  const { data } = await api.post<DexSendFee>('/dcrdex/wallet/txfee', { assetID, value, address, subtract });
  return data;
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
  // Per-order funding options for the base/quote wallets (e.g. multisplit), keyed
  // by option key; bisonw forwards these into each order's funding options.
  baseWalletOptions?: Record<string, string>;
  quoteWalletOptions?: Record<string, string>;
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
// MMStampedError mirrors bisonw's mm.StampedError.
export interface MMStampedError {
  stamp: number;
  error: string;
}
// MMBotProblems mirrors bisonw's mm.BotProblems: the reasons a bot could not
// place orders in an epoch. The per-asset maps are keyed by asset id.
export interface MMBotProblems {
  walletNotSynced?: Record<number, boolean>;
  noWalletPeers?: Record<number, boolean>;
  accountSuspended?: boolean;
  userLimitTooLow?: boolean;
  noPriceSource?: boolean;
  oracleFiatMismatch?: boolean;
  cexOrderbookUnsynced?: boolean;
  causesSelfMatch?: boolean;
  unknownError?: string;
}
// MMTradePlacement is one placement's per-epoch execution detail (bisonw
// mm.TradePlacement). Amounts are atomic; balance maps are keyed by asset id.
export interface MMTradePlacement {
  rate: number;
  lots: number;
  standingLots: number;
  orderedLots: number;
  counterTradeRate: number;
  requiredDex: Record<number, number>;
  requiredCex: number;
  usedDex: Record<number, number>;
  usedCex: number;
  error: MMBotProblems | null;
}
// MMOrderReport is a buy- or sell-side per-epoch placement report (bisonw
// mm.OrderReport). Balance maps are keyed by asset id; amounts are atomic.
export interface MMOrderReport {
  placements: MMTradePlacement[];
  availableDexBals: Record<number, MMBotBalance>;
  requiredDexBals: Record<number, number>;
  usedDexBals: Record<number, number>;
  remainingDexBals: Record<number, number>;
  availableCexBal?: MMBotBalance | null;
  requiredCexBal: number;
  usedCexBal: number;
  remainingCexBal: number;
  error: MMBotProblems | null;
}
// MMCEXProblems mirrors bisonw's mm.CEXProblems: the last deposit/withdraw/trade
// errors (deposit/withdraw keyed by asset id).
export interface MMCEXProblems {
  depositErr?: Record<number, MMStampedError>;
  withdrawErr?: Record<number, MMStampedError>;
  tradeErr?: MMStampedError | null;
}
export interface MMEpochReport {
  epochNum: number;
  buysReport: MMOrderReport | null;
  sellsReport: MMOrderReport | null;
  preOrderProblems: MMBotProblems | null;
}
export interface MMBotStatus {
  config: MMBotConfig;
  running: boolean;
  runStats: MMRunStats | null;
  latestEpoch: MMEpochReport | null;
  cexProblems: MMCEXProblems | null;
}
// MMEventTx is the wallet transaction carried by a run-log event (a subset of
// bisonw's asset.WalletTransaction).
export interface MMEventTx {
  id: string;
  type?: number;
  amount: number;
  fees: number;
  blockNumber: number;
  timestamp: number;
}
export interface MMDexOrderEvent {
  id: string;
  rate: number;
  qty: number;
  sell: boolean;
  transactions?: MMEventTx[];
}
export interface MMCexOrderEvent {
  id: string;
  rate: number;
  qty: number;
  sell: boolean;
  baseFilled: number;
  quoteFilled: number;
}
export interface MMDepositEvent {
  assetID: number;
  cexCredit: number;
  transaction?: MMEventTx;
}
export interface MMWithdrawalEvent {
  id: string;
  assetID: number;
  cexDebit: number;
  transaction?: MMEventTx;
}
// MMMarketMakingEvent is one run-log event (bisonw mm.MarketMakingEvent); only
// one of the *Event fields is set.
export interface MMMarketMakingEvent {
  id: number;
  timestamp: number;
  pending: boolean;
  dexOrderEvent?: MMDexOrderEvent;
  cexOrderEvent?: MMCexOrderEvent;
  depositEvent?: MMDepositEvent;
  withdrawalEvent?: MMWithdrawalEvent;
}
// MMRunOverview is a market-maker run summary (bisonw mm.MarketMakingRunOverview);
// profitLoss matches MMRunStats.profitLoss, endTime is set once the run stops.
export interface MMRunOverview {
  endTime?: number | null;
  profitLoss?: { profit: number; profitRatio: number } | null;
}
// MMRunLogs is the /api/mmrunlogs payload: a page of run events (newest first)
// plus the run overview. updatedLogs carries events whose state changed.
export interface MMRunLogs {
  overview: MMRunOverview | null;
  logs: MMMarketMakingEvent[] | null;
  updatedLogs: MMMarketMakingEvent[] | null;
}
export interface MMCexStatus {
  config?: { name: string };
  connected: boolean;
  connectErr?: string;
  // markets is the CEX's supported market list (bisonw libxc.Market), keyed by
  // the CEX's own market id; used to tell which pairs a CEX can arbitrage.
  markets?: Record<string, { baseID: number; quoteID: number }>;
  // balances are the CEX's per-asset holdings (bisonw libxc.ExchangeBalance),
  // keyed by asset id, in atomic units. Populated once the CEX is connected.
  balances?: Record<number, { available: number; locked: number }>;
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
// MMLotFees mirrors bisonw's mm.LotFees: per-lot swap/redeem/refund fees in the
// fee asset's atomic units.
export interface MMLotFees {
  swap: number;
  redeem: number;
  refund: number;
}
// MMLotFeeRange mirrors bisonw's mm.LotFeeRange: the estimated and worst-case
// per-lot fees, used to size a bot's funding reserves.
export interface MMLotFeeRange {
  max: MMLotFees;
  estimated: MMLotFees;
}
// MMMarketReport is bisonw's mm.MarketReport: the aggregate oracle price, the
// per-oracle breakdown, the base/quote fiat rates used for USD conversion, and
// the per-lot fee ranges (consumed by the funding allocation math).
export interface MMMarketReport {
  price: number;
  oracles: MMOracleReport[] | null;
  baseFiatRate: number;
  quoteFiatRate: number;
  baseFees?: MMLotFeeRange;
  quoteFees?: MMLotFeeRange;
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
export const getMMRunLogs = async (
  host: string,
  baseID: number,
  quoteID: number,
  startTime: number,
  n = 50,
  refID?: number,
): Promise<MMRunLogs | null> => {
  const { data } = await api.get<MMRunLogs | null>('/dcrdex/mm/runlogs', {
    params: { host, baseID, quoteID, startTime, n, ...(refID !== undefined ? { refID } : {}) },
  });
  return data;
};
// MMArchivedRun is one past market-maker run (bisonw mm.MarketMakingRun): when
// it started and the market it ran on. profit is optional - bisonw v1.0.6 does
// not include it in the archived-runs list (the realized P/L is available in the
// run's log overview), so treat it as absent unless present.
export interface MMArchivedRun {
  startTime: number;
  market: { host: string; baseID: number; quoteID: number };
  profit?: number;
}
export const getMMArchivedRuns = async (): Promise<MMArchivedRun[]> => {
  const { data } = await api.get<MMArchivedRun[] | null>('/dcrdex/mm/archivedruns');
  return data ?? [];
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
