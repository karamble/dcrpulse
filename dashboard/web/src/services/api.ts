// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import axios from 'axios';

const API_BASE_URL = '/api';

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 25000, // 25 seconds to accommodate wallet rescans
  withCredentials: true, // send the app-password session cookie (same-origin)
  headers: {
    'Content-Type': 'application/json',
  },
});

// When the optional app-password gate is enabled and the session is missing or
// expired, the backend replies 401. The AuthGate registers a handler here so
// any API call routes a 401 back to the login screen.
let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: (() => void) | null) {
  onUnauthorized = fn;
}

api.interceptors.response.use(
  (resp) => resp,
  (error) => {
    // Only re-lock on the app-password gate's OWN 401 (tagged with the
    // X-Dashboard-Auth header), never on a downstream daemon/wallet 401 such as
    // a wallet's public-passphrase prompt after a wallet switch.
    if (
      error?.response?.status === 401 &&
      error.response.headers?.['x-dashboard-auth'] === 'required' &&
      onUnauthorized
    ) {
      onUnauthorized();
    }
    return Promise.reject(error);
  },
);

// authFetch wraps native fetch for the two services that bypass this axios
// instance (explorer, treasury); it routes a 401 to the same login handler so
// session expiry on those pages also bounces to the login screen.
export async function authFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const res = await fetch(input, { credentials: 'same-origin', ...init });
  if (res.status === 401 && res.headers.get('x-dashboard-auth') === 'required' && onUnauthorized) {
    onUnauthorized();
  }
  return res;
}

export interface NodeStatus {
  status: string;
  syncProgress: number;
  version: string;
  syncPhase: string;
  syncMessage: string;
}

export interface RecentBlock {
  height: number;
  hash: string;
  timestamp: number; // Unix timestamp
}

export interface BlockchainInfo {
  blockHeight: number;
  blockHash: string;
  difficulty: number;
  chainSize: number;
  blockTime: string;
  recentBlocks: RecentBlock[];
}

export interface NetworkInfo {
  peerCount: number;
  hashrate: string;
  networkHashPS: number;
}

export interface Peer {
  id: number;
  address: string;
  protocol: string;
  latency: string;
  connTime: string;
  traffic: string;
  version: string;
  isSyncNode: boolean;
  inbound: boolean;
  tor: boolean;
}

export interface SupplyInfo {
  circulatingSupply: string;
  stakedSupply: string;
  stakedPercent: number;
  exchangeRate: string;
  treasurySize: string;
  mixedPercent: string;
}

export interface StakingInfo {
  ticketPrice: number;
  nextTicketPrice: number;
  poolSize: number;
  lockedDCR: number;
  participationRate: number;
  allMempoolTix: number;
  immature: number;
  live: number;
  voted: number;
  missed: number;
  revoked: number;
}

export interface MempoolInfo {
  size: number;
  bytes: number;
  txCount: number;
  totalFee: number;
  averageFeeRate: number;
  tickets: number;
  votes: number;
  revocations: number;
  regularTxs: number;
  coinJoinTxs: number;
}

export interface DashboardData {
  nodeStatus: NodeStatus;
  blockchainInfo: BlockchainInfo;
  networkInfo: NetworkInfo;
  peers: Peer[];
  supplyInfo: SupplyInfo;
  stakingInfo: StakingInfo;
  mempoolInfo: MempoolInfo;
  lastUpdate: string;
}

// API functions
export const getDashboardData = async (): Promise<DashboardData> => {
  const response = await api.get<DashboardData>('/dashboard');
  return response.data;
};

export const getNodeStatus = async (): Promise<NodeStatus> => {
  const response = await api.get<NodeStatus>('/node/status');
  return response.data;
};

export const getBlockchainInfo = async (): Promise<BlockchainInfo> => {
  const response = await api.get<BlockchainInfo>('/blockchain/info');
  return response.data;
};

export const getPeers = async (): Promise<Peer[]> => {
  const response = await api.get<Peer[]>('/network/peers');
  return response.data;
};

export const checkHealth = async (): Promise<any> => {
  const response = await api.get('/health');
  return response.data;
};

export interface HealthStatus {
  status: string;
  rpcConnected: boolean;
  walletRPCConnected: boolean;
  dcrdTLS: boolean;
  walletTLS: boolean;
}

export const getHealth = async (): Promise<HealthStatus> => {
  const response = await api.get<HealthStatus>('/health');
  return response.data;
};

// Wallet Types
export interface WalletStatus {
  status: string;
  syncProgress: number;
  syncHeight: number;
  bestBlockHash: string;
  version: string;
  unlocked: boolean;
  daemonConnected: boolean;
  rescanInProgress: boolean;
  syncMessage: string;
  isWatchOnly: boolean;
}

export interface AccountInfo {
  accountName: string;
  totalBalance: number;
  spendableBalance: number;
  immatureBalance: number;
  unconfirmedBalance: number;
  lockedByTickets: number;
  votingAuthority: number;
  immatureCoinbaseRewards: number;
  immatureStakeGeneration: number;
  accountNumber: number;
  // Per-account encryption state: accountUnlocked means the account's signing
  // key is currently usable (a running mixer/autobuyer or a pending spend).
  accountEncrypted: boolean;
  accountUnlocked: boolean;
  // Reserved accounts (mixed/unmixed/lightning/dex/imported) cannot be renamed.
  reserved?: boolean;
  // Wallet-wide totals (only on primary AccountInfo)
  cumulativeTotal?: number;
  totalSpendable?: number;
  totalLockedByTickets?: number;
}

export interface WalletTransaction {
  txid: string;
  amount: number;
  fee?: number;
  confirmations: number;
  blockHash?: string;
  blockTime?: number;
  time: string;
  category: string; // "send", "receive", "immature", "generate", "vspfee"
  txType: string;   // "regular", "ticket", "vote", "revocation"
  address?: string;
  account?: string;
  vout: number;
  generated?: boolean;
  isMixed?: boolean; // true if from CoinJoin/StakeShuffle
  isVSPFee?: boolean; // true if VSP fee payment
  relatedTicket?: string; // For VSP fees: the ticket txid this fee is for
  // Ticket-specific fields
  blockHeight?: number;         // Block height where transaction was confirmed
  isTicketMature?: boolean;     // For votes: whether the 256-block maturity period has passed
  blocksUntilSpendable?: number; // For votes: remaining blocks until funds are spendable (0 if already spendable)
}

export interface TransactionListResponse {
  transactions: WalletTransaction[];
  total: number;
}

export interface WalletAddress {
  address: string;
  account: string;
  used: boolean;
  path: string;
}

export interface StakingInfo {
  blockHeight: number;
  difficulty: number;
  totalSubsidy: number;
  ownMempoolTix: number;
  immature: number;
  unspent: number;
  voted: number;
  revoked: number;
  unspentExpired: number;
  poolSize: number;
  allMempoolTix: number;
  estimatedMin: number;
  estimatedMax: number;
  estimatedExpected: number;
  currentDifficulty: number;
  nextDifficulty: number;
  blockSubsidyHeight: number;
  blockSubsidyTotal: number;
  blockSubsidyPos: number;
  blockSubsidyPow: number;
  blockSubsidyTreasury: number;
  blocksUntilSubsidyReduction: number;
  subsidyReductionInterval: number;
}

export interface WalletDashboardData {
  walletStatus: WalletStatus;
  accountInfo: AccountInfo;
  accounts: AccountInfo[];
  stakingInfo?: StakingInfo;
  lastUpdate: string;
}

export interface ImportXpubRequest {
  xpub: string;
  accountName: string;
  rescan: boolean;
}

export interface ImportXpubResponse {
  success: boolean;
  message: string;
  accountNum?: number;
}

// Wallet API Functions
export const getWalletStatus = async (): Promise<WalletStatus> => {
  const response = await api.get<WalletStatus>('/wallet/status');
  return response.data;
};

export const getWalletDashboard = async (): Promise<WalletDashboardData> => {
  const response = await api.get<WalletDashboardData>('/wallet/dashboard');
  return response.data;
};

export const importXpub = async (xpub: string, accountName: string, rescan: boolean): Promise<ImportXpubResponse> => {
  const response = await api.post<ImportXpubResponse>('/wallet/importxpub', {
    xpub,
    accountName,
    rescan
  });
  return response.data;
};

export interface NextAddressResponse {
  address: string;
  accountNumber: number;
}

export const getAccounts = async (): Promise<AccountInfo[]> => {
  const response = await api.get<AccountInfo[]>('/wallet/accounts');
  return response.data;
};

export const createAccount = async (
  accountName: string,
  passphrase: string,
): Promise<{ accountNumber: number }> => {
  const response = await api.post<{ accountNumber: number }>('/wallet/create-account', {
    accountName,
    passphrase,
  });
  return response.data;
};

export const renameAccount = async (accountNumber: number, newName: string): Promise<void> => {
  await api.post('/wallet/rename-account', { accountNumber, newName });
};

export const getAccountExtendedPubKey = async (accountNumber: number): Promise<string> => {
  const response = await api.get<{ xpub: string }>('/wallet/account-extended-pubkey', {
    params: { accountNumber },
  });
  return response.data.xpub;
};

// Privacy / Mixer
export interface PrivacyStatus {
  configured: boolean;
  mixedAccount?: number;
  changeAccount?: number;
  mixerRunning: boolean;
  lastError?: string;
}

export interface MixerEvent {
  timestamp: string;
  level: 'info' | 'warn' | 'error';
  message: string;
}

export const getPrivacyStatus = async (): Promise<PrivacyStatus> => {
  const response = await api.get<PrivacyStatus>('/wallet/privacy/status');
  return response.data;
};

export const setupPrivacy = async (
  passphrase: string,
): Promise<{ mixedAccount: number; changeAccount: number }> => {
  const response = await api.post<{ mixedAccount: number; changeAccount: number }>(
    '/wallet/privacy/setup',
    { passphrase },
  );
  return response.data;
};

export const startMixer = async (passphrase: string): Promise<void> => {
  await api.post('/wallet/privacy/start', { passphrase });
};

export const stopMixer = async (): Promise<void> => {
  await api.post('/wallet/privacy/stop');
};

export const subscribeMixerEvents = (
  onEvent: (e: MixerEvent) => void,
  onError?: (e: Error) => void,
  onClose?: () => void,
): (() => void) => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/api/wallet/privacy/events`;
  const ws = new WebSocket(wsUrl);

  ws.onmessage = (event) => {
    try {
      onEvent(JSON.parse(event.data) as MixerEvent);
    } catch (err) {
      onError?.(new Error('Failed to parse mixer event'));
    }
  };
  ws.onerror = () => onError?.(new Error('Mixer events WebSocket error'));
  ws.onclose = () => onClose?.();

  return () => {
    if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
      ws.close();
    }
  };
};

export const getNextAddress = async (account: number): Promise<NextAddressResponse> => {
  const response = await api.get<NextAddressResponse>('/wallet/next-address', {
    params: { account },
  });
  return response.data;
};

export interface ValidateAddressResponse {
  isValid: boolean;
  isMine: boolean;
  accountNumber: number;
}

export interface ConstructTransactionRequest {
  sourceAccount: number;
  address: string;
  amountAtoms: number;
  sendAll: boolean;
}

export interface ConstructTransactionResponse {
  unsignedTxHex: string;
  inputsTotalAtoms: number;
  outputsTotalAtoms: number;
  changeAtoms: number;
  feeAtoms: number;
  totalDebitedAtoms: number;
  estimatedSignedSize: number;
}

export interface SignPublishTransactionResponse {
  txHash: string;
}

export const validateAddress = async (address: string): Promise<ValidateAddressResponse> => {
  const response = await api.get<ValidateAddressResponse>('/wallet/validate-address', {
    params: { address },
  });
  return response.data;
};

export const constructTransaction = async (req: ConstructTransactionRequest): Promise<ConstructTransactionResponse> => {
  const response = await api.post<ConstructTransactionResponse>('/wallet/construct-transaction', req);
  return response.data;
};

export const signPublishTransaction = async (
  sourceAccount: number,
  unsignedTxHex: string,
  passphrase: string,
): Promise<SignPublishTransactionResponse> => {
  const response = await api.post<SignPublishTransactionResponse>('/wallet/sign-publish-transaction', {
    sourceAccount,
    unsignedTxHex,
    passphrase,
  });
  return response.data;
};

export interface SignedTxPreviewOutput {
  index: number;
  address?: string;
  amountAtoms: number;
  scriptClass: string;
  isMine: boolean;
}

export interface SignedTxPreview {
  txid: string;
  sizeBytes: number;
  inputsTotalAtoms: number;
  outputsTotalAtoms: number;
  feeAtoms: number;
  feeKnown: boolean;
  outputs: SignedTxPreviewOutput[];
  txHex: string;
}

export interface BroadcastSignedTxResponse {
  txHash: string;
  alreadyBroadcast?: boolean;
}

// SignedTxInput carries a signed transaction as either base64 of a hardware-wallet
// file's raw bytes (signedTxB64, binary-safe, used for an uploaded .dcrtx file) or
// plain text (signedTx: a hex string or a "=== ... ===" export).
export interface SignedTxInput {
  signedTxB64?: string;
  signedTx?: string;
}

export const decodeSignedTransaction = async (input: SignedTxInput): Promise<SignedTxPreview> => {
  const response = await api.post<SignedTxPreview>('/wallet/decode-signed-transaction', input);
  return response.data;
};

export const broadcastSignedTransaction = async (input: SignedTxInput): Promise<BroadcastSignedTxResponse> => {
  const response = await api.post<BroadcastSignedTxResponse>('/wallet/broadcast-signed-transaction', input);
  return response.data;
};

// SignRequestExport carries the base64 CBOR SignRequest an air-gapped hardware
// wallet signs, plus the same amount/fee summary the send preview shows.
export interface SignRequestExport {
  signRequestB64: string;
  signRequestUR: string;
  inputsTotalAtoms: number;
  outputsTotalAtoms: number;
  changeAtoms: number;
  feeAtoms: number;
  totalDebitedAtoms: number;
  estimatedSignedSize: number;
}

export const buildSignRequest = async (req: ConstructTransactionRequest): Promise<SignRequestExport> => {
  const response = await api.post<SignRequestExport>('/wallet/build-sign-request', req);
  return response.data;
};

export const triggerRescan = async (): Promise<any> => {
  const response = await api.post('/wallet/rescan', { beginHeight: 0 });
  return response.data;
};

export interface SyncProgressData {
  isRescanning: boolean;
  scanHeight: number;
  chainHeight: number;
  progress: number;
  message: string;
  phase?: string;
  daemonConnected?: boolean;
  peerCount?: number;
  cfiltersStart?: number;
  cfiltersEnd?: number;
  headersCount?: number;
  // Unix seconds. firstHeaderTime is the timestamp of the FIRST header
  // received in this sync cycle; lastHeaderTime is the most recent. Their
  // difference is a usable wall-clock elapsed for the headers-fetch phase.
  firstHeaderTime?: number;
  lastHeaderTime?: number;
}

export const getSyncProgress = async (): Promise<SyncProgressData> => {
  const response = await api.get<SyncProgressData>('/wallet/sync-progress');
  return response.data;
};

// WebSocket streaming for rescan progress
export const streamRescanProgress = (
  onProgress: (data: SyncProgressData) => void,
  onError?: (error: Error) => void,
  onClose?: () => void
): (() => void) => {
  // Get WebSocket URL from current origin
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/api/wallet/grpc/stream-rescan`;
  
  console.log('Connecting to gRPC WebSocket:', wsUrl);
  const ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    console.log('WebSocket connection established');
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data) as SyncProgressData;
      onProgress(data);
    } catch (err) {
      console.error('Failed to parse WebSocket message:', err);
      onError?.(new Error('Failed to parse progress data'));
    }
  };

  ws.onerror = (event) => {
    console.error('WebSocket error:', event);
    onError?.(new Error('WebSocket connection error'));
  };

  ws.onclose = () => {
    console.log('WebSocket connection closed');
    onClose?.();
  };

  // Return cleanup function
  return () => {
    if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
      ws.close();
    }
  };
};

export const getWalletTransactions = async (count: number = 50, from: number = 0): Promise<TransactionListResponse> => {
  const response = await api.get<TransactionListResponse>(`/wallet/transactions?count=${count}&from=${from}`);
  return response.data;
};

// exportWalletCsv downloads a Decrediton-format CSV export of the wallet's
// transaction history or statistics. The full-history Balances exports can take
// a while, so a long per-request timeout is used.
export const exportWalletCsv = async (type: string) => {
  return api.get(`/wallet/export?type=${encodeURIComponent(type)}`, {
    responseType: 'blob',
    timeout: 300000,
  });
};

// Wallet Creation/Loader Types
export interface WalletExistsResponse {
  exists: boolean;
}

export interface WalletLoadedResponse {
  loaded: boolean;
  error?: string;
}

export interface GenerateSeedRequest {
  // Seed length in BYTES (not words). Zero or unset -> dcrwallet's
  // recommended 32 bytes -> 33-word Decred-standard mnemonic.
  seedLength?: number;
}

export interface GenerateSeedResponse {
  seedMnemonic: string; // 33-word mnemonic phrase
  seedHex: string;      // Hex-encoded seed
}

export interface CreateWalletRequest {
  name?: string;                     // Optional: target wallet name; empty uses the default wallet
  publicPassphrase: string;          // Optional: Encrypts wallet database for viewing
  confirmPublicPassphrase: string;   // Must equal publicPassphrase when public is non-empty
  privatePassphrase: string;         // Required: Encrypts private keys for spending
  confirmPrivatePassphrase: string;  // Must equal privatePassphrase
  seedHex: string;                   // Required: Hex-encoded seed
  discoverAccounts: boolean;         // True when restoring from existing seed
  watchOnly?: boolean;               // True to create a watching-only wallet from extendedPubKey (no seed)
  extendedPubKey?: string;           // Required when watchOnly: dpub/tpub extended public key
}

export interface CreateWalletResponse {
  success: boolean;
  message?: string;
}

export interface OpenWalletRequest {
  publicPassphrase: string; // Optional: Wallet database passphrase (empty if wallet created without one)
}

export interface OpenWalletResponse {
  success: boolean;
  message?: string;
}

// Wallet Creation/Loader API Functions
export const checkWalletExists = async (): Promise<WalletExistsResponse> => {
  const response = await api.get<WalletExistsResponse>('/wallet/exists');
  return response.data;
};

export const checkWalletLoaded = async (): Promise<WalletLoadedResponse> => {
  const response = await api.get<WalletLoadedResponse>('/wallet/loaded');
  return response.data;
};

// generateSeed lets dcrwallet pick the recommended length by default
// (zero -> 32 bytes -> 33-word mnemonic). Pass a non-zero byte count only if
// you need a non-standard seed length.
export const generateSeed = async (seedLength: number = 0): Promise<GenerateSeedResponse> => {
  const response = await api.post<GenerateSeedResponse>('/wallet/generate-seed', { seedLength });
  return response.data;
};

export const decodeSeed = async (userInput: string): Promise<{ seedHex: string }> => {
  const response = await api.post<{ seedHex: string }>('/wallet/decode-seed', { userInput });
  return response.data;
};

export const createWallet = async (request: CreateWalletRequest): Promise<CreateWalletResponse> => {
  const response = await api.post<CreateWalletResponse>('/wallet/create', request);
  return response.data;
};

export const openWallet = async (request: OpenWalletRequest): Promise<OpenWalletResponse> => {
  const response = await api.post<OpenWalletResponse>('/wallet/open', request);
  return response.data;
};

// Multi-wallet API

export interface WalletInfo {
  name: string;
  network: string;
  hasDb: boolean;
  isDefault: boolean;
  isWatchOnly: boolean;
  isPrivacy: boolean;
  lastAccess?: number;
  active: boolean;
}

export interface ListWalletsResponse {
  wallets: WalletInfo[];
  active: string;
}

export const listWallets = async (): Promise<ListWalletsResponse> => {
  const response = await api.get<ListWalletsResponse>('/wallets');
  return { wallets: response.data.wallets ?? [], active: response.data.active ?? '' };
};

// selectWallet relaunches the dcrwallet daemon against the chosen wallet, which
// can take several seconds; callers should show a switching state.
export const selectWallet = async (name: string, publicPassphrase = ''): Promise<{ success: boolean; active: string }> => {
  const response = await api.post<{ success: boolean; active: string }>('/wallets/select', { name, publicPassphrase });
  return response.data;
};

export const closeActiveWallet = async (): Promise<{ success: boolean }> => {
  const response = await api.post<{ success: boolean }>('/wallet/close', {});
  return response.data;
};

export const createNamedWallet = async (request: CreateWalletRequest): Promise<CreateWalletResponse> => {
  const response = await api.post<CreateWalletResponse>('/wallets/create', request);
  return response.data;
};

export const renameWallet = async (from: string, to: string): Promise<{ success: boolean }> => {
  const response = await api.post<{ success: boolean }>('/wallets/rename', { from, to });
  return response.data;
};

export const deleteWallet = async (name: string): Promise<{ success: boolean }> => {
  const response = await api.post<{ success: boolean }>('/wallets/delete', { name });
  return response.data;
};

export const getMixerDebug = async (): Promise<{ enabled: boolean }> => {
  const response = await api.get<{ enabled: boolean }>('/wallet/mixer/debug');
  return response.data;
};

export const setMixerDebug = async (enabled: boolean): Promise<{ enabled: boolean }> => {
  const response = await api.post<{ enabled: boolean }>('/wallet/mixer/debug', { enabled });
  return response.data;
};

// Staking / VSP

export interface VSPInfo {
  host: string;
  pubkey: string;
  network: string;
  apiVersions?: number[];
  feePercentage: number;
  vspdVersion?: string;
  blockHeight?: number;
  networkProportion?: number;
  voting?: number;
  voted?: number;
  expired?: number;
  missed?: number;
  outdated?: boolean;
}

export interface PurchaseTicketsRequest {
  account: number;
  numTickets: number;
  vspHost: string;
  vspPubkey: string;
  changeAccount: number;
  passphrase: string;
}

export interface PurchaseTicketsResponse {
  ticketHashes: string[];
  splitTxHash?: string;
}

export interface ListVSPsResponse {
  vsps: VSPInfo[];
  usedVSPs: VSPInfo[];
  registryEnabled: boolean;
  registryError?: string;
}

export const listVSPs = async (): Promise<ListVSPsResponse> => {
  const response = await api.get<ListVSPsResponse>('/wallet/staking/vsps');
  return response.data;
};

export const getVSPInfo = async (host: string): Promise<VSPInfo> => {
  const response = await api.get<VSPInfo>(`/wallet/staking/vsp-info?host=${encodeURIComponent(host)}`);
  return response.data;
};

// A privacy/mixed purchase is dispatched to a background worker and answered
// with HTTP 202 { async: true }; the result then arrives over the
// purchase-events WebSocket. A plain purchase completes synchronously and
// returns its ticket hashes directly.
export type PurchaseTicketsResult = PurchaseTicketsResponse | { async: true };

export const isAsyncPurchase = (r: PurchaseTicketsResult): r is { async: true } =>
  (r as { async?: boolean }).async === true;

export const purchaseTickets = async (
  req: PurchaseTicketsRequest,
): Promise<PurchaseTicketsResult> => {
  const response = await api.post<PurchaseTicketsResponse | { async?: boolean }>(
    '/wallet/staking/purchase',
    req,
  );
  if (response.status === 202 || (response.data as { async?: boolean })?.async) {
    return { async: true };
  }
  return response.data as PurchaseTicketsResponse;
};

export interface PurchaseEvent {
  timestamp: string;
  level: 'info' | 'warn' | 'error';
  message: string;
  kind: 'progress' | 'done' | 'error';
  ticketHashes?: string[];
  splitTxHash?: string;
}

export interface PurchaseStatus {
  inProgress: boolean;
  lastError: string;
  ticketHashes?: string[];
  splitTxHash?: string;
}

export const getPurchaseStatus = async (): Promise<PurchaseStatus> => {
  const response = await api.get<PurchaseStatus>('/wallet/staking/purchase/status');
  return response.data;
};

// Streams progress/result events for a background (mixed) ticket purchase.
// Mirrors subscribeAutobuyerEvents.
export const subscribePurchaseEvents = (
  onEvent: (e: PurchaseEvent) => void,
  onError?: (e: Error) => void,
  onClose?: () => void,
): (() => void) => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/api/wallet/staking/purchase/events`;
  const ws = new WebSocket(wsUrl);

  ws.onmessage = (event) => {
    try {
      onEvent(JSON.parse(event.data) as PurchaseEvent);
    } catch (err) {
      onError?.(new Error('Failed to parse purchase event'));
    }
  };
  ws.onerror = () => onError?.(new Error('Purchase events WebSocket error'));
  ws.onclose = () => onClose?.();

  return () => {
    if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
      ws.close();
    }
  };
};

export type TicketLifecycleStatus =
  | 'UNKNOWN'
  | 'UNMINED'
  | 'IMMATURE'
  | 'LIVE'
  | 'VOTED'
  | 'MISSED'
  | 'EXPIRED'
  | 'REVOKED';

export type TicketFeeStatus = '' | 'UNPAID' | 'PAID' | 'ERRORED' | 'CONFIRMED';

export interface TicketRecord {
  hash: string;
  status: TicketLifecycleStatus;
  feeStatus: TicketFeeStatus;
  vspHost: string;
  blockHeight: number;
  blockTime: number;
  ticketPrice: number;
  spenderHash: string;
  spenderHeight: number;
  spenderTime: number;
  reward: number;
  blocksUntilMature: number;
}

export const listTickets = async (): Promise<TicketRecord[]> => {
  const response = await api.get<TicketRecord[]>('/wallet/staking/tickets');
  return response.data ?? [];
};

export interface SyncFailedVSPTicketsRequest {
  vspHost: string;
  vspPubkey: string;
  account: number;
  changeAccount: number;
  passphrase: string;
}

export interface VSPFeeStatusCounts {
  unpaid: number;
  paid: number;
  errored: number;
  confirmed: number;
}

export interface SyncFailedVSPTicketsResponse {
  vspHost: string;
  before: VSPFeeStatusCounts;
  after: VSPFeeStatusCounts;
}

// The sync runs against the VSP server-side with a 120s context, well past the
// 25s default client timeout, so allow more time for this one call.
export const syncFailedVSPTickets = async (
  req: SyncFailedVSPTicketsRequest,
): Promise<SyncFailedVSPTicketsResponse> => {
  const response = await api.post<SyncFailedVSPTicketsResponse>(
    '/wallet/staking/sync-failed-vsp-tickets',
    req,
    { timeout: 125000 },
  );
  return response.data;
};

// Re-associates untracked tickets with a VSP. Runs server-side with a 120s
// context, so allow more time than the 25s default like the sync call above.
export const processUnmanagedVSPTickets = async (
  req: SyncFailedVSPTicketsRequest,
): Promise<SyncFailedVSPTicketsResponse> => {
  const response = await api.post<SyncFailedVSPTicketsResponse>(
    '/wallet/staking/process-unmanaged-vsp-tickets',
    req,
    { timeout: 125000 },
  );
  return response.data;
};

export interface AutobuyerSettings {
  account: number;
  vspHost: string;
  vspPubkey: string;
  balanceToMaintain: number;
}

export interface AutobuyerEvent {
  timestamp: string;
  level: 'info' | 'warn' | 'error';
  message: string;
}

export interface AutobuyerStatus {
  running: boolean;
  lastError: string;
  settings: AutobuyerSettings | null;
}

export const getAutobuyerStatus = async (): Promise<AutobuyerStatus> => {
  const response = await api.get<AutobuyerStatus>('/wallet/staking/autobuyer/status');
  return response.data;
};

export const getAutobuyerSettings = async (): Promise<AutobuyerSettings | null> => {
  const response = await api.get<AutobuyerSettings | null>('/wallet/staking/autobuyer/settings');
  return response.data;
};

export const saveAutobuyerSettings = async (s: AutobuyerSettings): Promise<void> => {
  await api.post('/wallet/staking/autobuyer/settings', s);
};

export const startAutobuyer = async (req: AutobuyerSettings & { passphrase: string }): Promise<void> => {
  await api.post('/wallet/staking/autobuyer/start', req);
};

export const stopAutobuyer = async (): Promise<void> => {
  await api.post('/wallet/staking/autobuyer/stop');
};

export interface WalletSettings {
  gapLimit: number;
  currencyDisplay?: string;
}

export interface ExternalRequestSettings {
  vspListing: boolean;
  politeia: boolean;
  brseeder: boolean;
}

export interface GlobalSettings {
  externalRequests: ExternalRequestSettings;
  decredPulseBotUrl?: string;
}

export interface SettingsEnvelope {
  wallet?: WalletSettings;
  global?: GlobalSettings;
}

export const getSettings = async (): Promise<SettingsEnvelope> => {
  const response = await api.get<SettingsEnvelope>('/wallet/settings');
  return response.data;
};

export const saveSettings = async (e: SettingsEnvelope): Promise<void> => {
  await api.post('/wallet/settings', e);
};

export const changePassphrase = async (oldPassphrase: string, newPassphrase: string): Promise<void> => {
  await api.post('/wallet/settings/change-passphrase', { oldPassphrase, newPassphrase });
};

export type LogComponent = 'dcrd' | 'dcrwallet' | 'dcrlnd' | 'brclientd' | 'dcrdex';

export interface LogTail {
  component: LogComponent;
  lines: string[];
}

export const getLogs = async (component: LogComponent, lines = 500): Promise<LogTail> => {
  const response = await api.get<LogTail>(
    `/wallet/settings/logs?component=${component}&lines=${lines}`,
  );
  return response.data;
};

export const discoverAddresses = async (
  passphrase: string,
  gapLimit?: number,
): Promise<void> => {
  await api.post(
    '/wallet/settings/discover-addresses',
    { passphrase, gapLimit },
    { timeout: 10 * 60 * 1000 },
  );
};

export const subscribeAutobuyerEvents = (
  onEvent: (e: AutobuyerEvent) => void,
  onError?: (e: Error) => void,
  onClose?: () => void,
): (() => void) => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/api/wallet/staking/autobuyer/events`;
  const ws = new WebSocket(wsUrl);

  ws.onmessage = (event) => {
    try {
      onEvent(JSON.parse(event.data) as AutobuyerEvent);
    } catch (err) {
      onError?.(new Error('Failed to parse autobuyer event'));
    }
  };
  ws.onerror = () => onError?.(new Error('Autobuyer events WebSocket error'));
  ws.onclose = () => onClose?.();

  return () => {
    if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
      ws.close();
    }
  };
};

// ---- Governance ----------------------------------------------------------

export interface AgendaChoice {
  id: string;
  description: string;
  isAbstain: boolean;
  isNo: boolean;
}

export interface Agenda {
  id: string;
  description: string;
  status: string;
  startHeight: number;
  expireHeight: number;
  choices: AgendaChoice[];
  currentChoice: string;
}

export interface TreasuryKeyPolicy {
  key: string;
  policy: string;
}

export interface TSpendPolicyEntry {
  hash: string;
  policy: string;
  amount?: number;
  expiry?: number;
  blockHeight?: number;
}

export interface Proposal {
  token: string;
  name: string;
  username: string;
  status: string;
  voteStatus: string;
  voteCounts: Record<string, number>;
  totalVotes: number;
  quorumMin: number;
  eligibleTickets: number;
  endBlock: number;
  blocksLeft: number;
  currentChoice: string;
}

export interface ProposalVoteOption {
  id: string;
  bit: number;
}

// ProposalComment is one Politeia comment. parentID is 0 for a top-level
// comment, otherwise the commentID of the comment it replies to. Deleted
// comments carry no text, only the admin deletion reason.
export interface ProposalComment {
  commentID: number;
  parentID: number;
  username: string;
  comment: string;
  commentHtml?: string;
  createdAt: number;
  upvotes: number;
  downvotes: number;
  deleted: boolean;
  reason?: string;
}

export interface ProposalDetail extends Proposal {
  description: string;
  descriptionHtml?: string;
  submittedAt: number;
  voteOptions: ProposalVoteOption[];
  comments?: ProposalComment[];
}

export interface CastVoteResult {
  cast: number;
  skipped: number;
  errors?: string[];
}

export const getAgendas = async (): Promise<Agenda[]> => {
  const response = await api.get<Agenda[]>('/wallet/governance/agendas');
  return response.data ?? [];
};

export const setAgendaChoice = async (
  agendaID: string,
  choiceID: string,
  passphrase: string,
): Promise<void> => {
  await api.post('/wallet/governance/agendas/set', { agendaID, choiceID, passphrase });
};

export const getTreasuryKeyPolicies = async (): Promise<TreasuryKeyPolicy[]> => {
  const response = await api.get<TreasuryKeyPolicy[]>('/wallet/governance/treasury/keys');
  return response.data ?? [];
};

export const setTreasuryKeyPolicy = async (
  key: string,
  policy: string,
  passphrase: string,
): Promise<void> => {
  await api.post('/wallet/governance/treasury/keys/set', { key, policy, passphrase });
};

export const getTSpendPolicies = async (): Promise<TSpendPolicyEntry[]> => {
  const response = await api.get<TSpendPolicyEntry[]>('/wallet/governance/treasury/tspends');
  return response.data ?? [];
};

export const setTSpendPolicy = async (
  hash: string,
  policy: string,
  passphrase: string,
): Promise<void> => {
  await api.post('/wallet/governance/treasury/tspends/set', { hash, policy, passphrase });
};

// ProposalsResponse is the proposals list envelope: the cached list plus the
// last successful fetch time and when a manual refresh is next allowed (both
// unix seconds; 0 when never fetched).
export interface ProposalsResponse {
  proposals: Proposal[];
  fetchedAt: number;
  refreshAvailableAt: number;
}

// getProposals returns the cached proposals envelope. The backend caches the
// list indefinitely and auto-fetches once when empty; a cold fetch can take up
// to ~1 min, so allow more than the default client timeout.
export const getProposals = async (): Promise<ProposalsResponse> => {
  const response = await api.get<ProposalsResponse>('/wallet/governance/proposals', {
    timeout: 65 * 1000,
  });
  return response.data;
};

// refreshProposals forces a backend re-fetch from Politeia. Throws on 429 while
// the 8h cooldown is active; the error response body carries the envelope so
// callers can re-sync the countdown.
export const refreshProposals = async (): Promise<ProposalsResponse> => {
  const response = await api.post<ProposalsResponse>(
    '/wallet/governance/proposals/refresh',
    undefined,
    { timeout: 65 * 1000 },
  );
  return response.data;
};

// ProposalDetailResponse is the proposal-detail envelope: the cached record
// plus the last successful fetch time and when a manual refresh is next allowed
// (both unix seconds; 0 when never fetched).
export interface ProposalDetailResponse {
  detail: ProposalDetail;
  fetchedAt: number;
  refreshAvailableAt: number;
}

// getProposalDetail returns the cached detail envelope. Cached indefinitely per
// token and auto-fetched once; a cold fetch can take a while, so allow more
// than the default client timeout.
export const getProposalDetail = async (token: string): Promise<ProposalDetailResponse> => {
  const response = await api.get<ProposalDetailResponse>(
    `/wallet/governance/proposals/${encodeURIComponent(token)}`,
    { timeout: 65 * 1000 },
  );
  return response.data;
};

// refreshProposalDetail forces a backend re-fetch of one proposal. Throws on
// 429 while the 8h cooldown is active; the error response body carries the
// envelope so callers can re-sync the countdown.
export const refreshProposalDetail = async (token: string): Promise<ProposalDetailResponse> => {
  const response = await api.post<ProposalDetailResponse>(
    `/wallet/governance/proposals/${encodeURIComponent(token)}/refresh`,
    undefined,
    { timeout: 65 * 1000 },
  );
  return response.data;
};

// VoteEligibility is what the vote modal needs when the user opens it: how many
// of the proposal's eligible tickets the wallet owns, the vote options, and
// whether the wallet already voted. Computed on demand (the heavy eligible-
// ticket snapshot work runs only here, not on a plain detail view).
export interface VoteEligibility {
  ownedEligibleCount: number;
  eligibleTickets: number;
  voteOptions: ProposalVoteOption[];
  alreadyVoted: boolean;
  currentChoice: string;
}

// getVoteEligibility triggers the on-demand eligibility computation for a
// proposal. Returns instantly when a local vote record already exists; the
// heavy path (snapshot + committed-ticket intersection + recorded-vote
// reconciliation) can take a while, so allow more than the default timeout.
export const getVoteEligibility = async (token: string): Promise<VoteEligibility> => {
  const response = await api.post<VoteEligibility>(
    `/wallet/governance/proposals/${encodeURIComponent(token)}/vote-eligibility`,
    undefined,
    { timeout: 65 * 1000 },
  );
  return response.data;
};

export const castPoliteiaVote = async (
  token: string,
  voteOption: string,
  passphrase: string,
): Promise<CastVoteResult> => {
  const response = await api.post<CastVoteResult>(
    '/wallet/governance/proposals/cast-vote',
    { token, voteOption, passphrase },
    { timeout: 2 * 60 * 1000 },
  );
  return response.data;
};

export default api;

