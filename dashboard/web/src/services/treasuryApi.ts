// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

const API_BASE_URL = '/api';

export interface TSpend {
  txHash: string;
  amount: number;
  payee: string;
  expiryHeight: number;
  currentHeight: number;
  blocksRemaining: number;
  status: 'voting' | 'approved' | 'rejected';
  detectedAt: string;
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
  const response = await fetch(`${API_BASE_URL}/treasury/info`);
  if (!response.ok) {
    throw new Error('Failed to fetch treasury info');
  }
  return response.json();
}

// Trigger historical TSpend scan
export async function triggerTSpendScan(startHeight?: number): Promise<{ success: boolean; message: string }> {
  const response = await fetch(`${API_BASE_URL}/treasury/scan-history`, {
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
  const response = await fetch(`${API_BASE_URL}/treasury/scan-progress`);
  if (!response.ok) {
    throw new Error('Failed to fetch scan progress');
  }
  return response.json();
}

// Get scan results
export async function getTSpendScanResults(): Promise<TSpendHistory[]> {
  const response = await fetch(`${API_BASE_URL}/treasury/scan-results`);
  if (!response.ok) {
    throw new Error('Failed to fetch scan results');
  }
  return response.json();
}

