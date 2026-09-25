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
  amountAtoms: number;
  feeAtoms: number;
  payee: string;
  blockHeight: number;
  blockHash: string;
  timestamp: string;
  voteResult: 'approved' | 'rejected';
}

export interface TreasuryInfo {
  balance: number;
  balanceAtoms: number;
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
  taddFound: number;
  newTSpends: TSpendHistory[];
  message: string;
  failedBlocks?: number;
  safeHeight?: number;
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
    const reason = (await response.text()).trim();
    throw new Error(reason || 'Failed to trigger TSpend scan');
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

export interface TreasuryTAdd {
  txHash: string;
  amountAtoms: number;
  blockHeight: number;
  blockHash: string;
  timestamp: string;
}

// What the last scan recorded over blocks fromHeight through toHeight.
export interface TreasuryScanResults {
  fromHeight: number;
  toHeight: number;
  tspends: TSpendHistory[];
  tadds: TreasuryTAdd[];
  tbaseByMonth: Record<string, number>;
}

// Get scan results
export async function getTSpendScanResults(): Promise<TreasuryScanResults> {
  const response = await authFetch(`${API_BASE_URL}/treasury/scan-results`);
  if (!response.ok) {
    throw new Error('Failed to fetch scan results');
  }
  return response.json();
}

// Get the treasury balance-over-time series (first block of every UTC month plus the tip, cached server-side)
export async function getTreasuryBalanceHistory(): Promise<BalanceSample[]> {
  const response = await authFetch(`${API_BASE_URL}/treasury/balance-history`);
  if (!response.ok) {
    throw new Error('Failed to fetch treasury balance history');
  }
  return (await response.json()) ?? [];
}


// dcrd's DCP-0013 spend limit for a TVI block following the tip.
export interface TreasurySpendLimit {
  active: boolean;
  height: number;
  nextTvi: number;
  atTvi: boolean;
  policyWindowBlocks: number;
  spentInWindowAtoms: number;
  balanceAtoms: number;
  maxSpendableAtoms: number;
  floorAtoms: number;
  allowedAtoms: number;
}

export async function getTreasurySpendLimit(): Promise<TreasurySpendLimit> {
  const response = await authFetch(`${API_BASE_URL}/treasury/spend-limit`);
  if (!response.ok) {
    throw new Error('Failed to fetch the treasury spend limit');
  }
  return response.json();
}

// Projected block reward per calendar month, at the target block time.
export interface TreasuryOutlook {
  fromHeight: number;
  targetBlockSeconds: number;
  months: { month: string; blocks: number; tbaseAtoms: number }[];
}

export async function getTreasuryOutlook(): Promise<TreasuryOutlook> {
  const response = await authFetch(`${API_BASE_URL}/treasury/outlook`);
  if (!response.ok) {
    throw new Error('Failed to fetch the treasury outlook');
  }
  return response.json();
}

// How long the treasury balance lasts at a monthly spend, with the block
// reward following dcrd's schedule at the target block time.
export interface TreasuryRunway {
  fromHeight: number;
  balanceAtoms: number;
  monthlySpendAtoms: number;
  firstMonthNetAtoms: number;
  targetBlockSeconds: number;
  projectionMonths: number;
  months: number;
  exhaustedMonth?: string;
  beyond: boolean;
}

export async function getTreasuryRunway(monthlySpendAtoms: number): Promise<TreasuryRunway> {
  const response = await authFetch(`${API_BASE_URL}/treasury/runway?monthlySpendAtoms=${monthlySpendAtoms}`);
  if (!response.ok) {
    throw new Error('Failed to fetch the treasury runway');
  }
  return response.json();
}
