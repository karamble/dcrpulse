// Copyright (c) 2015-2026 The Decred developers
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
import { TreasurySpendLimitCard } from '../components/governance/TreasurySpendLimitCard';
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
  applyScanResults,
  getScanStatus,
  saveScanStatus,
  getLastSyncHeight,
  syncWithSnapshot,
  TREASURY_ACTIVATION_HEIGHT,
} from '../services/treasuryStorage';

export const GovernanceDashboard = () => {
  const [isScanning, setIsScanning] = useState(false);
  const [scanProgress, setScanProgress] = useState<ScanProgressType | null>(null);
  const [lastScanStatus, setLastScanStatus] = useState(getScanStatus());
  const [refreshTrigger, setRefreshTrigger] = useState(0);

  // One treasury fetch for the whole page. Each card used to poll this same
  // endpoint on its own timer, including in background tabs.
  const [treasuryBalance, setTreasuryBalance] = useState<number | null>(null);
  const [treasuryBalanceAtoms, setTreasuryBalanceAtoms] = useState<number | null>(null);
  const [activeTSpends, setActiveTSpends] = useState<TSpend[]>([]);
  const [treasuryLoaded, setTreasuryLoaded] = useState(false);
  const [balanceSeries, setBalanceSeries] = useState<BalanceSample[]>([]);

  useVisiblePoll(() => {
    getTreasuryInfo()
      .then((i) => {
        setTreasuryBalance(i.balance);
        setTreasuryBalanceAtoms(i.balanceAtoms);
        setActiveTSpends(i.activeTSpends ?? []);
      })
      .catch(() => {})
      .finally(() => setTreasuryLoaded(true));
  }, 60000);

  useEffect(() => {
    getTreasuryBalanceHistory().then(setBalanceSeries).catch(() => {});
  }, []);

  // Load the shipped snapshot, then take in a scan that finished while the
  // page was closed, or follow one still running.
  useEffect(() => {
    const initializeData = async () => {
      const snapshotResult = await syncWithSnapshot();
      if (!snapshotResult.success) {
        console.warn('Failed to sync with snapshot:', snapshotResult.error);
      }
      try {
        const progress = await getTSpendScanProgress();
        if (progress.isScanning) {
          setIsScanning(true);
          setScanProgress(progress);
        } else {
          applyScanResults(await getTSpendScanResults());
        }
      } catch (error) {
        console.error('Failed to check scan status:', error);
      }
      setRefreshTrigger((prev) => prev + 1);
    };

    initializeData();
  }, []);

  // Poll for scan progress while scanning
  useEffect(() => {
    if (!isScanning) return;

    const interval = setInterval(async () => {
      try {
        const progress = await getTSpendScanProgress();
        setScanProgress(progress);
        if (progress.isScanning) return;

        setIsScanning(false);
        clearInterval(interval);
        // The results cover the blocks the scan read; a block it could not
        // read ends them, so the next scan starts there.
        try {
          applyScanResults(await getTSpendScanResults());
        } catch (syncError) {
          console.error('Failed to take in the scan results:', syncError);
        }
        saveScanStatus({
          lastScanDate: new Date().toISOString(),
          lastScanHeight: getLastSyncHeight(),
          totalTSpendsFound: progress.tspendFound,
          failedBlocks: progress.failedBlocks,
        });
        setLastScanStatus(getScanStatus());
        setRefreshTrigger((prev) => prev + 1);
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

    // The shipped snapshot and earlier scans set the stored height, so a scan
    // only reads the blocks after it.
    const lastSyncHeight = getLastSyncHeight();
    const startHeight = lastSyncHeight > 0 ? lastSyncHeight + 1 : TREASURY_ACTIVATION_HEIGHT;

    const confirmMessage = lastSyncHeight > 0
      ? `Scan the blocks since the last sync for treasury flows?\n\n` +
        `Last sync: Block ${lastSyncHeight.toLocaleString()}\n` +
        `Will scan every block from ${startHeight.toLocaleString()} to the current height.\n\n` +
        `Click OK to continue.`
      : `Scan every block since treasury activation for treasury flows?\n\n` +
        `This reads every block from ${TREASURY_ACTIVATION_HEIGHT.toLocaleString()} to the current height.\n` +
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
      alert(error instanceof Error ? error.message : 'Failed to start scan.');
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
          balanceAtoms={treasuryBalanceAtoms}
          series={balanceSeries}
          refreshKey={refreshTrigger}
        />
        <ActiveTreasuryVotes active={activeTSpends} loaded={treasuryLoaded} />
        <TreasurySpendLimitCard />

        {/* Historical Scan Section */}
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center gap-2 mb-2">
                <SearchCheck className="h-5 w-5 text-primary" />
                <h2 className="text-xl font-semibold">Treasury Flow Scanner</h2>
              </div>
              <p className="text-sm text-muted-foreground">
                Read every block since the last sync for the treasury's block reward, contributions and spends
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
              {isScanning ? 'Scanning...' : 'Scan New Blocks'}
            </button>
          </div>

          {/* Last Scan Status */}
          {lastScanStatus && !isScanning && (
            <div className="mt-4 pt-4 border-t border-border/50 text-sm text-muted-foreground">
              <div className="flex items-center gap-4">
                <span>
                  {lastScanStatus.failedBlocks ? 'Last scan stopped early: ' : 'Last scanned: '}
                  {formatDate(lastScanStatus.lastScanDate)}
                </span>
                <span>Height: {lastScanStatus.lastScanHeight.toLocaleString()}</span>
                <span>Found: {lastScanStatus.totalTSpendsFound} TSpends</span>
                {lastScanStatus.failedBlocks ? (
                  <span className="text-yellow-500">
                    a block could not be read, scan again to continue from it
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

