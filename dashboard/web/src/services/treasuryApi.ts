// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { authFetch } from './api';

const API_BASE_URL = '/api';

export interface TSpend {
  txHash: string;
  amount: number;
  payee: string;
  expiryHeight: number;
  currentHeight: number;
  blocksRemaining: number;
  status: 'voting' | 'approved' | 'rejected';
  yesVotes: number;
  noVotes: number;
  detectedAt: string;
}

export interface BalanceSample {
  height: number;
  time: number; // block unix time (seconds)
  balance: number; // treasury balance in DCR
}

export interface TSpendHistory {
  txHash: string;
  amount: number;
  payee: string;
  blockHeight: number;
  blockHash: string;
  timestamp: string;
  voteResult: 'approved' | 'rejected';
}

export interface TreasuryInfo {
  balance: number;
  balanceUsd: number;
  totalAdded: number;
  totalSpent: number;
  activeTSpends: TSpend[];
  recentTSpends: TSpendHistory[];
  lastUpdate: string;
}

export interface TSpendScanProgress {
  isScanning: boolean;
  currentHeight: number;
  totalHeight: number;
  progress: number;
  tspendFound: number;
  newTSpends: TSpendHistory[];
  message: string;
}

// Fetch current treasury information
export async function getTreasuryInfo(): Promise<TreasuryInfo> {
  const response = await authFetch(`${API_BASE_URL}/treasury/info`);
  if (!response.ok) {
    throw new Error('Failed to fetch treasury info');
  }
  return response.json();
}

// Trigger historical TSpend scan
export async function triggerTSpendScan(startHeight?: number): Promise<{ success: boolean; message: string }> {
  const response = await authFetch(`${API_BASE_URL}/treasury/scan-history`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ startHeight: startHeight || 552448 }),
  });
  if (!response.ok) {
    throw new Error('Failed to trigger TSpend scan');
  }
  return response.json();
}

// Get scan progress
export async function getTSpendScanProgress(): Promise<TSpendScanProgress> {
  const response = await authFetch(`${API_BASE_URL}/treasury/scan-progress`);
  if (!response.ok) {
    throw new Error('Failed to fetch scan progress');
  }
  return response.json();
}

// Get scan results
export async function getTSpendScanResults(): Promise<TSpendHistory[]> {
  const response = await authFetch(`${API_BASE_URL}/treasury/scan-results`);
  if (!response.ok) {
    throw new Error('Failed to fetch scan results');
  }
  return response.json();
}

// Get the treasury balance-over-time series (sampled ~monthly, cached server-side)
export async function getTreasuryBalanceHistory(): Promise<BalanceSample[]> {
  const response = await authFetch(`${API_BASE_URL}/treasury/balance-history`);
  if (!response.ok) {
    throw new Error('Failed to fetch treasury balance history');
  }
  return (await response.json()) ?? [];
}

