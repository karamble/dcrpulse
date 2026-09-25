// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { TreasuryScanResults } from './treasuryApi';

const STORAGE_KEY = 'decred-pulse-treasury-history';
const SCAN_STATUS_KEY = 'decred-pulse-treasury-scan-status';
const STORAGE_VERSION = 2;

// The first block with a treasury.
export const TREASURY_ACTIVATION_HEIGHT = 552448;

// Amounts are in atoms throughout.
export interface TSpendRecord {
  txHash: string;
  amountAtoms: number;
  feeAtoms: number;
  payee: string;
  blockHeight: number;
  timestamp: string;
  voteResult: 'approved' | 'rejected';
}

export interface TAddRecord {
  txHash: string;
  amountAtoms: number;
  blockHeight: number;
  timestamp: string;
}

// Everything the treasury received and paid from activation through
// lastSyncHeight: spends, contributions, and the block reward per UTC month.
export interface TreasuryStorageData {
  version: number;
  tspends: TSpendRecord[];
  tadds: TAddRecord[];
  tbaseByMonth: Record<string, number>;
  lastSyncHeight: number;
}

export interface ScanStatus {
  lastScanDate: string;
  lastScanHeight: number;
  totalTSpendsFound: number;
  failedBlocks?: number;
}

const empty = (): TreasuryStorageData => ({
  version: STORAGE_VERSION,
  tspends: [],
  tadds: [],
  tbaseByMonth: {},
  lastSyncHeight: 0,
});

const isStorageData = (d: unknown): d is TreasuryStorageData => {
  const s = d as TreasuryStorageData;
  return (
    !!s &&
    s.version === STORAGE_VERSION &&
    Array.isArray(s.tspends) &&
    Array.isArray(s.tadds) &&
    typeof s.tbaseByMonth === 'object' &&
    s.tbaseByMonth !== null &&
    Number.isSafeInteger(s.lastSyncHeight)
  );
};

// Data from an older storage version reads as empty; the snapshot refills it.
const getStorage = (): TreasuryStorageData => {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null');
    return isStorageData(parsed) ? parsed : empty();
  } catch {
    return empty();
  }
};

const saveStorage = (storage: TreasuryStorageData): void => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(storage));
  } catch (error) {
    console.error('Failed to save treasury data:', error);
    throw new Error('Failed to save treasury data. Storage quota may be exceeded.');
  }
};

export const getAllTSpends = (): TSpendRecord[] =>
  getStorage().tspends.sort((a, b) => b.blockHeight - a.blockHeight);

export const getAllTAdds = (): TAddRecord[] =>
  getStorage().tadds.sort((a, b) => b.blockHeight - a.blockHeight);

export const getTBaseByMonth = (): Record<string, number> => getStorage().tbaseByMonth;

export const getTreasuryStats = () => {
  const storage = getStorage();
  const totalSpentAtoms = storage.tspends.reduce((s, t) => s + t.amountAtoms + t.feeAtoms, 0);
  return {
    totalSpentAtoms,
    count: storage.tspends.length,
    lastSyncHeight: storage.lastSyncHeight,
  };
};

// applyScanResults adds a scan's results when they continue exactly where the
// stored data ends, and reports whether it did. Results that overlap or leave a
// gap are refused, so nothing is counted twice.
export const applyScanResults = (r: TreasuryScanResults): boolean => {
  const storage = getStorage();
  const next = storage.lastSyncHeight > 0 ? storage.lastSyncHeight + 1 : TREASURY_ACTIVATION_HEIGHT;
  if (r.fromHeight !== next || r.toHeight < r.fromHeight) {
    return false;
  }
  const spends = new Set(storage.tspends.map((t) => t.txHash));
  for (const t of r.tspends ?? []) {
    if (spends.has(t.txHash)) continue;
    storage.tspends.push({
      txHash: t.txHash,
      amountAtoms: t.amountAtoms,
      feeAtoms: t.feeAtoms,
      payee: t.payee,
      blockHeight: t.blockHeight,
      timestamp: t.timestamp,
      voteResult: t.voteResult,
    });
  }
  const adds = new Set(storage.tadds.map((t) => t.txHash));
  for (const t of r.tadds ?? []) {
    if (adds.has(t.txHash)) continue;
    storage.tadds.push({
      txHash: t.txHash,
      amountAtoms: t.amountAtoms,
      blockHeight: t.blockHeight,
      timestamp: t.timestamp,
    });
  }
  for (const [month, atoms] of Object.entries(r.tbaseByMonth ?? {})) {
    storage.tbaseByMonth[month] = (storage.tbaseByMonth[month] ?? 0) + atoms;
  }
  storage.lastSyncHeight = r.toHeight;
  saveStorage(storage);
  return true;
};

// The whole stored data set; the shipped snapshot is one of these.
export const exportTreasuryData = (): string => JSON.stringify(getStorage(), null, 2);

// Replaces the stored data with an export that reaches further.
export const importTreasuryData = (json: string): { success: boolean; error?: string } => {
  try {
    const imported: unknown = JSON.parse(json);
    if (!isStorageData(imported)) {
      return { success: false, error: `Not a version ${STORAGE_VERSION} treasury export` };
    }
    if (imported.lastSyncHeight <= getStorage().lastSyncHeight) {
      return { success: false, error: 'The stored data already reaches this height' };
    }
    saveStorage(imported);
    return { success: true };
  } catch (error) {
    return { success: false, error: error instanceof Error ? error.message : 'Invalid JSON format' };
  }
};

export const clearTreasuryData = (): void => saveStorage(empty());

export const getLastSyncHeight = (): number => getStorage().lastSyncHeight;

export const getScanStatus = (): ScanStatus | null => {
  try {
    const data = localStorage.getItem(SCAN_STATUS_KEY);
    return data ? (JSON.parse(data) as ScanStatus) : null;
  } catch {
    return null;
  }
};

export const saveScanStatus = (status: ScanStatus): void => {
  try {
    localStorage.setItem(SCAN_STATUS_KEY, JSON.stringify(status));
  } catch (error) {
    console.error('Failed to save scan status:', error);
  }
};

// Loads the shipped snapshot when it reaches further than the stored data.
export const syncWithSnapshot = async (): Promise<{ success: boolean; synced: boolean; error?: string }> => {
  try {
    const response = await fetch('/tspend-snapshot.json');
    if (!response.ok) {
      throw new Error('Failed to fetch the treasury snapshot');
    }
    const snapshot: unknown = await response.json();
    if (!isStorageData(snapshot)) {
      throw new Error('Invalid snapshot format');
    }
    if (snapshot.lastSyncHeight <= getStorage().lastSyncHeight) {
      return { success: true, synced: false };
    }
    saveStorage(snapshot);
    return { success: true, synced: true };
  } catch (error) {
    return { success: false, synced: false, error: error instanceof Error ? error.message : 'Unknown error' };
  }
};
