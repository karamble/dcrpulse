// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import axios from 'axios';

const API_BASE_URL = '/api';

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 25000, // 25 seconds to accommodate wallet rescans
  headers: {
    'Content-Type': 'application/json',
  },
});

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

export interface RPCConnectionRequest {
  host: string;
  port: string;
  username: string;
  password: string;
}

export interface RPCConnectionResponse {
  success: boolean;
  message: string;
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

export const connectRPC = async (config: RPCConnectionRequest): Promise<RPCConnectionResponse> => {
  const response = await api.post<RPCConnectionResponse>('/connect', config);
  return response.data;
};

export const checkHealth = async (): Promise<any> => {
  const response = await api.get('/health');
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
  rescanInProgress: boolean;
  syncMessage: string;
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

// Wallet Creation/Loader Types
export interface WalletExistsResponse {
  exists: boolean;
}

export interface GenerateSeedRequest {
  seedLength?: number; // Optional, defaults to 33
}

export interface GenerateSeedResponse {
  seedMnemonic: string; // 33-word mnemonic phrase
  seedHex: string;      // Hex-encoded seed
}

export interface CreateWalletRequest {
  publicPassphrase: string;  // Optional: Encrypts wallet database for viewing
  privatePassphrase: string; // Required: Encrypts private keys for spending
  seedHex: string;           // Required: Hex-encoded seed
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

export const generateSeed = async (seedLength: number = 33): Promise<GenerateSeedResponse> => {
  const response = await api.post<GenerateSeedResponse>('/wallet/generate-seed', { seedLength });
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

export default api;

