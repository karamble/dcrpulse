// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, useEffect } from 'react';
import { toYMDTime } from '../utils/date';
import { SearchCheck } from 'lucide-react';
import { TreasuryValueCard } from '../components/governance/TreasuryValueCard';
import { TreasuryPaymentsCard } from '../components/governance/TreasuryPaymentsCard';
import { TSpendScanProgress } from '../components/governance/TSpendScanProgress';
import { TreasuryStats } from '../components/governance/TreasuryStats';
import { ActiveTreasuryVotes } from '../components/governance/ActiveTreasuryVotes';
import { useVisiblePoll } from '../hooks/useVisiblePoll';
import {
  BalanceSample,
  getTreasuryBalanceHistory,
  getTreasuryInfo,
  TSpend,
} from '../services/treasuryApi';
import { 
  triggerTSpendScan, 
  getTSpendScanProgress,
  getTSpendScanResults,
  TSpendScanProgress as ScanProgressType 
} from '../services/treasuryApi';
import { 
  saveTSpends, 
  getScanStatus, 
  saveScanStatus,
  getLastSyncHeight,
  updateLastSyncHeight,
  syncWithSnapshot,
  TSpendRecord 
} from '../services/treasuryStorage';

export const GovernanceDashboard = () => {
  const [isScanning, setIsScanning] = useState(false);
  const [scanProgress, setScanProgress] = useState<ScanProgressType | null>(null);
  const [lastScanStatus, setLastScanStatus] = useState(getScanStatus());
  const [refreshTrigger, setRefreshTrigger] = useState(0);

  // One treasury fetch for the whole page. Each card used to poll this same
  // endpoint on its own timer, including in background tabs.
  const [treasuryBalance, setTreasuryBalance] = useState<number | null>(null);
  const [activeTSpends, setActiveTSpends] = useState<TSpend[]>([]);
  const [treasuryLoaded, setTreasuryLoaded] = useState(false);
  const [balanceSeries, setBalanceSeries] = useState<BalanceSample[]>([]);

  useVisiblePoll(() => {
    getTreasuryInfo()
      .then((i) => {
        setTreasuryBalance(i.balance);
        setActiveTSpends(i.activeTSpends ?? []);
      })
      .catch(() => {})
      .finally(() => setTreasuryLoaded(true));
  }, 60000);

  useEffect(() => {
    getTreasuryBalanceHistory().then(setBalanceSeries).catch(() => {});
  }, []);

  // Sync with historical snapshot on first load
  useEffect(() => {
    const initializeData = async () => {
      // First, sync with the historical snapshot if needed
      const snapshotResult = await syncWithSnapshot();
      if (snapshotResult.success && snapshotResult.synced > 0) {
        setRefreshTrigger(prev => prev + 1);
      } else if (snapshotResult.success && snapshotResult.synced === 0) {
        // Still trigger a refresh to ensure UI displays existing data
        setRefreshTrigger(prev => prev + 1);
      } else if (!snapshotResult.success) {
        console.warn('Failed to sync with snapshot:', snapshotResult.error);
      }
    };

    initializeData();
  }, []);

  // Check if a scan is already in progress when the page loads
  // This handles browser reconnection after being closed during a scan
  useEffect(() => {
    const checkScanStatus = async () => {
      try {
        const progress = await getTSpendScanProgress();
        
        // IMPORTANT: Sync TSpends that were found while browser was closed
        // The backend's newTSpendBuffer only contains TSpends since the last progress check,
        // so if the browser was closed for a while, we need to fetch ALL scan results
        // from the backend and compare with localStorage to catch any missed TSpends
        if (progress.isScanning || progress.tspendFound > 0) {
          try {
            const allResults = await getTSpendScanResults();
            if (allResults && allResults.length > 0) {
              const records: TSpendRecord[] = allResults.map(t => ({
                txHash: t.txHash,
                amount: t.amount,
                payee: t.payee,
                blockHeight: t.blockHeight,
                timestamp: t.timestamp,
                voteResult: t.voteResult,
                detectedAt: new Date().toISOString(),
              }));
              
              const addedCount = saveTSpends(records);
              if (addedCount > 0) {
                setRefreshTrigger(prev => prev + 1);
              }
            }
          } catch (syncError) {
            console.error('Failed to sync scan results:', syncError);
          }
        }
        
        if (progress.isScanning) {
          setIsScanning(true);
          setScanProgress(progress);
        }
      } catch (error) {
        console.error('Failed to check scan status:', error);
      }
    };

    checkScanStatus();
  }, []);

  // Poll for scan progress while scanning
  useEffect(() => {
    if (!isScanning) return;

    const interval = setInterval(async () => {
      try {
        const progress = await getTSpendScanProgress();
        setScanProgress(progress);

        // Save any new TSpends immediately as they're discovered
        if (progress.newTSpends && progress.newTSpends.length > 0) {
          const records: TSpendRecord[] = progress.newTSpends.map(t => ({
            txHash: t.txHash,
            amount: t.amount,
            payee: t.payee,
            blockHeight: t.blockHeight,
            timestamp: t.timestamp,
            voteResult: t.voteResult,
            detectedAt: new Date().toISOString(),
          }));
          
          saveTSpends(records);

          // Trigger refresh of treasury card to show new data
          setRefreshTrigger(prev => prev + 1);
        }

        if (!progress.isScanning) {
          // Scan completed
          setIsScanning(false);
          clearInterval(interval);

          // Final sync: Fetch all scan results to ensure nothing was missed
          try {
            const allResults = await getTSpendScanResults();
            if (allResults && allResults.length > 0) {
              const records: TSpendRecord[] = allResults.map(t => ({
                txHash: t.txHash,
                amount: t.amount,
                payee: t.payee,
                blockHeight: t.blockHeight,
                timestamp: t.timestamp,
                voteResult: t.voteResult,
                detectedAt: new Date().toISOString(),
              }));
              
              saveTSpends(records);
            }
          } catch (syncError) {
            console.error('Failed to perform final sync:', syncError);
          }

          // Resume above the last height the scan actually read. A block it
          // could not read holds the watermark below itself, so the next scan
          // covers the gap instead of stepping over it for good.
          const resumeHeight = progress.safeHeight && progress.safeHeight > 0
            ? progress.safeHeight
            : progress.currentHeight;
          updateLastSyncHeight(resumeHeight);

          // Save scan completion status
          saveScanStatus({
            lastScanDate: new Date().toISOString(),
            lastScanHeight: resumeHeight,
            totalTSpendsFound: progress.tspendFound,
            failedBlocks: progress.failedBlocks,
          });
          setLastScanStatus(getScanStatus());

          // Final refresh
          setRefreshTrigger(prev => prev + 1);
        }
      } catch (error) {
        console.error('Failed to fetch scan progress:', error);
      }
    }, 3000); // A full-chain scan runs for minutes; 3s is plenty for a progress bar

    return () => clearInterval(interval);
  }, [isScanning]);

  const handleTriggerScan = async () => {
    if (isScanning) {
      return;
    }

    // Get last sync height from localStorage
    // This will be automatically set from the snapshot (block 1,012,032)
    // or from previous scans, so users don't need to rescan from 2021
    const lastSyncHeight = getLastSyncHeight();
    const startHeight = lastSyncHeight > 0 ? lastSyncHeight + 1 : 552448;
    
    const confirmMessage = lastSyncHeight > 0
      ? `Continue scanning for historical Treasury Spends?\n\n` +
        `Last sync: Block ${lastSyncHeight.toLocaleString()}\n` +
        `Will scan from block ${startHeight.toLocaleString()} to current height.\n\n` +
        `Click OK to continue.`
      : `Scan the entire blockchain for historical Treasury Spends?\n\n` +
        `This will scan from block 552,448 (Treasury activation) to the current height.\n` +
        `The process may take a very long time.\n\n` +
        `Click OK to continue.`;

    const confirmed = window.confirm(confirmMessage);

    if (!confirmed) {
      return;
    }

    try {
      await triggerTSpendScan(startHeight);
      setIsScanning(true);
      
      // Start polling immediately
      const progress = await getTSpendScanProgress();
      setScanProgress(progress);
    } catch (error) {
      console.error('Failed to trigger scan:', error);
      alert('Failed to start scan. Please check your connection to the backend.');
    }
  };

  const formatDate = (dateString: string) => {
    try {
      return toYMDTime(new Date(dateString));
    } catch {
      return dateString;
    }
  };

  return (
    <div className="space-y-6">
        {/* Statistics, charts + active votes (top) */}
        <TreasuryStats
          balance={treasuryBalance}
          series={balanceSeries}
          refreshKey={refreshTrigger}
        />
        <ActiveTreasuryVotes active={activeTSpends} loaded={treasuryLoaded} />

        {/* Historical Scan Section */}
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center gap-2 mb-2">
                <SearchCheck className="h-5 w-5 text-primary" />
                <h2 className="text-xl font-semibold">Historical TSpend Scanner</h2>
              </div>
              <p className="text-sm text-muted-foreground">
                Scan the blockchain to find all historical treasury spend transactions since activation
              </p>
            </div>
            <button
              onClick={handleTriggerScan}
              disabled={isScanning}
              className={`px-6 py-3 rounded-lg font-medium transition-colors ${
                isScanning
                  ? 'bg-muted text-muted-foreground cursor-not-allowed'
                  : 'bg-primary text-primary-foreground hover:bg-primary/90'
              }`}
            >
              {isScanning ? 'Scanning...' : 'Scan Historical TSpends'}
            </button>
          </div>

          {/* Last Scan Status */}
          {lastScanStatus && !isScanning && (
            <div className="mt-4 pt-4 border-t border-border/50 text-sm text-muted-foreground">
              <div className="flex items-center gap-4">
                <span>
                  {lastScanStatus.failedBlocks ? 'Last scan incomplete: ' : 'Last scanned: '}
                  {formatDate(lastScanStatus.lastScanDate)}
                </span>
                <span>Height: {lastScanStatus.lastScanHeight.toLocaleString()}</span>
                <span>Found: {lastScanStatus.totalTSpendsFound} TSpends</span>
                {lastScanStatus.failedBlocks ? (
                  <span className="text-yellow-500">
                    {lastScanStatus.failedBlocks} block(s) unread, scan again to cover them
                  </span>
                ) : null}
              </div>
            </div>
          )}
        </div>

        {/* Scan Progress */}
        {isScanning && scanProgress && (
          <TSpendScanProgress
            progress={scanProgress.progress}
            currentHeight={scanProgress.currentHeight}
            totalHeight={scanProgress.totalHeight}
            tspendFound={scanProgress.tspendFound}
            message={scanProgress.message}
          />
        )}

        {/* Treasury Cards */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <TreasuryValueCard balance={treasuryBalance} series={balanceSeries} />
          <TreasuryPaymentsCard refreshKey={refreshTrigger} />
        </div>
      </div>
  );
};

