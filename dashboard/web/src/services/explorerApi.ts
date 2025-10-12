// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

const API_BASE_URL = '/api';

export interface BlockSummary {
  height: number;
  hash: string;
  previousHash: string;
  timestamp: string;
  confirmations: number;
  txCount: number;
  size: number;
  difficulty: number;
}

export interface BlockDetail extends BlockSummary {
  nextHash?: string;
  merkleRoot: string;
  stakeRoot: string;
  version: number;
  voteBits: number;
  transactions: TransactionSummary[];
  stakeVersion: number;
  nonce: number;
}

export interface TransactionSummary {
  txid: string;
  type: string; // regular, ticket, vote, revocation, coinbase
  blockHeight: number;
  blockHash?: string;
  timestamp: string;
  confirmations: number;
  totalValue: number;
  fee: number;
  size: number;
}

export interface TransactionDetail extends TransactionSummary {
  version: number;
  lockTime: number;
  expiry: number;
  inputs: TxInput[];
  outputs: TxOutput[];
  rawHex?: string;
}

export interface TxInput {
  prevTxid?: string;
  vout: number;
  tree: number;
  sequence: number;
  amountIn: number;
  blockHeight: number;
  blockIndex: number;
  scriptSig?: string;
  address?: string;
  stakebase?: string;
  coinbase?: string;
}

export interface TxOutput {
  value: number;
  index: number;
  version: number;
  scriptPubKey: Script;
  spent?: boolean;
  spentBy?: string;
}

export interface Script {
  asm: string;
  hex: string;
  type: string;
  reqSigs?: number;
  addresses?: string[];
}

export interface SearchResult {
  type: string; // block, transaction, address, unknown
  found: boolean;
  data?: BlockDetail | TransactionDetail;
  error?: string;
}

export interface AddressInfo {
  address: string;
  isValid: boolean;
  exists: boolean;
  tickets: string[];
  hasIndex: boolean;
}

export interface PaginatedBlocksResponse {
  blocks: BlockSummary[];
  currentPage: number;
  pageSize: number;
  totalBlocks: number;
  totalPages: number;
}

// API Functions

export async function searchExplorer(query: string): Promise<SearchResult> {
  const response = await fetch(`${API_BASE_URL}/explorer/search?q=${encodeURIComponent(query)}`);
  if (!response.ok) {
    throw new Error('Search failed');
  }
  return response.json();
}

export async function getRecentBlocks(count: number = 10): Promise<BlockSummary[]> {
  const response = await fetch(`${API_BASE_URL}/explorer/blocks/recent?count=${count}`);
  if (!response.ok) {
    throw new Error('Failed to fetch recent blocks');
  }
  return response.json();
}

export async function getRecentBlocksPaginated(page: number = 1, pageSize: number = 10): Promise<PaginatedBlocksResponse> {
  const response = await fetch(`${API_BASE_URL}/explorer/blocks/recent?page=${page}&pageSize=${pageSize}`);
  if (!response.ok) {
    throw new Error('Failed to fetch recent blocks');
  }
  return response.json();
}

export async function getBlockByHeight(height: number): Promise<BlockDetail> {
  const response = await fetch(`${API_BASE_URL}/explorer/blocks/${height}`);
  if (!response.ok) {
    throw new Error('Block not found');
  }
  return response.json();
}

export async function getBlockByHash(hash: string): Promise<BlockDetail> {
  const response = await fetch(`${API_BASE_URL}/explorer/blocks/hash/${hash}`);
  if (!response.ok) {
    throw new Error('Block not found');
  }
  return response.json();
}

export async function getTransaction(txhash: string): Promise<TransactionDetail> {
  const response = await fetch(`${API_BASE_URL}/explorer/transactions/${txhash}`);
  if (!response.ok) {
    throw new Error('Transaction not found');
  }
  return response.json();
}

export async function getAddressInfo(address: string): Promise<AddressInfo> {
  const response = await fetch(`${API_BASE_URL}/explorer/address/${address}`);
  if (!response.ok) {
    throw new Error('Failed to fetch address information');
  }
  return response.json();
}

