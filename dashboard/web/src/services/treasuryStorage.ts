// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

const STORAGE_KEY = 'decred-pulse-treasury-history';
const SCAN_STATUS_KEY = 'decred-pulse-treasury-scan-status';
const STORAGE_VERSION = 1;

export interface TSpendRecord {
  txHash: string;
  amount: number;
  payee: string;
  blockHeight: number;
  timestamp: string;
  voteResult: 'approved' | 'rejected';
  detectedAt: string;
}

export interface TreasuryStorageData {
  version: number;
  tspends: TSpendRecord[];
  totalSpent: number;
  lastSyncHeight: number;
}

export interface ScanStatus {
  lastScanDate: string;
  lastScanHeight: number;
  totalTSpendsFound: number;
}

// Initialize storage if it doesn't exist
const initStorage = (): void => {
  const existing = localStorage.getItem(STORAGE_KEY);
  if (!existing) {
    const initial: TreasuryStorageData = {
      version: STORAGE_VERSION,
      tspends: [],
      totalSpent: 0,
      lastSyncHeight: 0,
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(initial));
  }
};

// Get storage object
const getStorage = (): TreasuryStorageData => {
  try {
    initStorage();
    const data = localStorage.getItem(STORAGE_KEY);
    if (!data) {
      console.log('Treasury storage not found, returning empty');
      return { version: STORAGE_VERSION, tspends: [], totalSpent: 0, lastSyncHeight: 0 };
    }
    const parsed = JSON.parse(data) as TreasuryStorageData;
    console.log(`Treasury storage loaded: ${parsed.tspends.length} TSpends, lastSyncHeight: ${parsed.lastSyncHeight}`);
    return parsed;
  } catch (error) {
    console.error('Failed to parse treasury storage:', error);
    return { version: STORAGE_VERSION, tspends: [], totalSpent: 0, lastSyncHeight: 0 };
  }
};

// Save storage object
const saveStorage = (storage: TreasuryStorageData): void => {
  try {
    const serialized = JSON.stringify(storage);
    localStorage.setItem(STORAGE_KEY, serialized);
    console.log(`Treasury storage saved: ${storage.tspends.length} TSpends, ${(serialized.length / 1024).toFixed(2)} KB, lastSyncHeight: ${storage.lastSyncHeight}`);
    
    // Verify it was saved correctly
    const verification = localStorage.getItem(STORAGE_KEY);
    if (!verification) {
      console.error('⚠️ STORAGE VERIFICATION FAILED: Data was not persisted to localStorage!');
    }
  } catch (error) {
    console.error('Failed to save treasury data:', error);
    throw new Error('Failed to save treasury data. Storage quota may be exceeded.');
  }
};

// Save a new TSpend to localStorage
export const saveTSpend = (tspend: TSpendRecord): boolean => {
  const storage = getStorage();
  
  // Check for duplicates
  if (storage.tspends.some(t => t.txHash === tspend.txHash)) {
    return false; // Already exists
  }

  storage.tspends.push(tspend);
  storage.totalSpent += tspend.amount;
  
  // Update lastSyncHeight to the highest block height we've seen
  if (tspend.blockHeight > storage.lastSyncHeight) {
    storage.lastSyncHeight = tspend.blockHeight;
  }
  
  saveStorage(storage);
  return true;
};

// Save multiple TSpends (for batch operations like scan results)
export const saveTSpends = (tspends: TSpendRecord[]): number => {
  const storage = getStorage();
  let addedCount = 0;
  let maxHeight = storage.lastSyncHeight;

  const existingHashes = new Set(storage.tspends.map(t => t.txHash));

  for (const tspend of tspends) {
    if (!existingHashes.has(tspend.txHash)) {
      storage.tspends.push(tspend);
      storage.totalSpent += tspend.amount;
      existingHashes.add(tspend.txHash);
      addedCount++;
      
      // Track the highest block height
      if (tspend.blockHeight > maxHeight) {
        maxHeight = tspend.blockHeight;
      }
    }
  }

  if (addedCount > 0) {
    storage.lastSyncHeight = maxHeight;
    saveStorage(storage);
  }

  return addedCount;
};

// Get all stored TSpends
export const getAllTSpends = (): TSpendRecord[] => {
  const storage = getStorage();
  // Sort by block height descending (most recent first)
  return storage.tspends.sort((a, b) => b.blockHeight - a.blockHeight);
};

// Get treasury statistics
export const getTreasuryStats = () => {
  const storage = getStorage();
  const averageAmount = storage.tspends.length > 0 ? storage.totalSpent / storage.tspends.length : 0;

  return {
    totalSpent: storage.totalSpent,
    count: storage.tspends.length,
    averageAmount,
    lastSyncHeight: storage.lastSyncHeight,
  };
};

// Export treasury data as JSON
export const exportTreasuryData = (): string => {
  const storage = getStorage();
  return JSON.stringify(storage, null, 2);
};

// Import treasury data from JSON
export const importTreasuryData = (json: string): { success: boolean; count: number; error?: string } => {
  try {
    const imported = JSON.parse(json) as TreasuryStorageData;

    if (!imported.tspends || !Array.isArray(imported.tspends)) {
      return { success: false, count: 0, error: 'Invalid JSON format' };
    }

    const storage = getStorage();
    const existingHashes = new Set(storage.tspends.map(t => t.txHash));
    
    let importedCount = 0;
    let maxHeight = storage.lastSyncHeight;

    for (const tspend of imported.tspends) {
      if (!tspend.txHash || !tspend.amount) {
        continue;
      }

      if (existingHashes.has(tspend.txHash)) {
        continue; // Skip duplicates
      }

      storage.tspends.push(tspend);
      storage.totalSpent += tspend.amount;
      existingHashes.add(tspend.txHash);
      importedCount++;
      
      // Track the highest block height
      if (tspend.blockHeight && tspend.blockHeight > maxHeight) {
        maxHeight = tspend.blockHeight;
      }
    }

    if (importedCount > 0) {
      storage.lastSyncHeight = maxHeight;
      saveStorage(storage);
    }

    return { success: true, count: importedCount };
  } catch (error) {
    return { 
      success: false, 
      count: 0, 
      error: error instanceof Error ? error.message : 'Invalid JSON format' 
    };
  }
};

// Clear all treasury data
export const clearTreasuryData = (): void => {
  const initial: TreasuryStorageData = {
    version: STORAGE_VERSION,
    tspends: [],
    totalSpent: 0,
    lastSyncHeight: 0,
  };
  saveStorage(initial);
};

// Get the last synced block height
export const getLastSyncHeight = (): number => {
  const storage = getStorage();
  return storage.lastSyncHeight;
};

// Update the last synced block height
export const updateLastSyncHeight = (height: number): void => {
  const storage = getStorage();
  if (height > storage.lastSyncHeight) {
    storage.lastSyncHeight = height;
    saveStorage(storage);
    console.log(`Updated lastSyncHeight to ${height}`);
  }
};

// Scan status management
export const getScanStatus = (): ScanStatus | null => {
  try {
    const data = localStorage.getItem(SCAN_STATUS_KEY);
    if (!data) return null;
    return JSON.parse(data) as ScanStatus;
  } catch (error) {
    console.error('Failed to parse scan status:', error);
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

// Debug function to check localStorage state (can be called from browser console)
export const debugTreasuryStorage = () => {
  console.log('=== TREASURY STORAGE DEBUG ===');
  
  // Check if localStorage is available
  try {
    const testKey = '__localStorage_test__';
    localStorage.setItem(testKey, 'test');
    localStorage.removeItem(testKey);
    console.log('✓ localStorage is available');
  } catch (e) {
    console.error('❌ localStorage is NOT available:', e);
    return;
  }
  
  // Check treasury data
  const rawData = localStorage.getItem(STORAGE_KEY);
  console.log('Raw data exists:', !!rawData);
  console.log('Raw data size:', rawData ? `${(rawData.length / 1024).toFixed(2)} KB` : '0 KB');
  
  if (rawData) {
    try {
      const parsed = JSON.parse(rawData);
      console.log('Parsed successfully:', {
        version: parsed.version,
        tspends: parsed.tspends?.length || 0,
        totalSpent: parsed.totalSpent,
        lastSyncHeight: parsed.lastSyncHeight,
      });
      
      if (parsed.tspends && parsed.tspends.length > 0) {
        console.log('First TSpend:', parsed.tspends[0]);
        console.log('Last TSpend:', parsed.tspends[parsed.tspends.length - 1]);
      }
    } catch (e) {
      console.error('Failed to parse data:', e);
    }
  }
  
  // Check scan status
  const scanStatus = localStorage.getItem(SCAN_STATUS_KEY);
  console.log('Scan status exists:', !!scanStatus);
  if (scanStatus) {
    console.log('Scan status:', JSON.parse(scanStatus));
  }
  
  // List all localStorage keys
  console.log('All localStorage keys:', Object.keys(localStorage));
  
  console.log('=== END DEBUG ===');
};

// Make it globally accessible for browser console debugging
if (typeof window !== 'undefined') {
  (window as any).debugTreasuryStorage = debugTreasuryStorage;
}

// Load and sync with the historical TSpend snapshot
export const syncWithSnapshot = async (): Promise<{ success: boolean; synced: number; error?: string }> => {
  try {
    console.log('🔄 Checking if snapshot sync is needed...');
    
    // Check current storage BEFORE fetching snapshot
    const currentStorage = getStorage();
    const currentCount = currentStorage.tspends.length;
    console.log(`📊 Current localStorage state: ${currentCount} TSpends, lastSyncHeight: ${currentStorage.lastSyncHeight}`);
    
    // Fetch the snapshot from public folder
    const response = await fetch('/tspend-snapshot.json');
    if (!response.ok) {
      throw new Error('Failed to fetch TSpend snapshot');
    }
    
    const snapshot = await response.json() as TreasuryStorageData;
    
    if (!snapshot.tspends || !Array.isArray(snapshot.tspends)) {
      throw new Error('Invalid snapshot format');
    }
    
    const snapshotCount = snapshot.tspends.length;
    console.log(`📦 Snapshot loaded: ${snapshotCount} TSpends, lastSyncHeight: ${snapshot.lastSyncHeight}`);
    
    // If we already have more or equal TSpends than the snapshot, skip
    if (currentCount >= snapshotCount) {
      console.log('✓ localStorage already has complete data, skipping snapshot sync');
      return { success: true, synced: 0 };
    }
    
    // Sync snapshot data
    console.log(`🔄 Syncing ${snapshotCount - currentCount} missing TSpends from snapshot...`);
    const syncedCount = saveTSpends(snapshot.tspends);
    
    // Verify after sync
    const afterSync = getStorage();
    console.log(`✓ Sync complete: ${syncedCount} new TSpends added. Total: ${afterSync.tspends.length}, lastSyncHeight: ${afterSync.lastSyncHeight}`);
    
    return { success: true, synced: syncedCount };
    
  } catch (error) {
    console.error('❌ Failed to sync with snapshot:', error);
    return { 
      success: false, 
      synced: 0, 
      error: error instanceof Error ? error.message : 'Unknown error' 
    };
  }
};

