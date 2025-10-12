// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import { WalletStatus } from '../components/WalletStatus';
import { AccountInfo } from '../components/AccountInfo';
import { AccountsList } from '../components/AccountsList';
import { ImportXpubModal } from '../components/ImportXpubModal';
import { SyncProgressBar } from '../components/SyncProgressBar';
import { TicketPoolInfo } from '../components/TicketPoolInfo';
import { MyTicketsInfo } from '../components/MyTicketsInfo';
import { TransactionHistory } from '../components/TransactionHistory';
import { AddressBookmarksCard } from '../components/wallet/AddressBookmarksCard';
import { getWalletDashboard, WalletDashboardData, triggerRescan, getSyncProgress, streamRescanProgress, SyncProgressData } from '../services/api';

export const WalletDashboard = () => {
  const [data, setData] = useState<WalletDashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showImportModal, setShowImportModal] = useState(false);
  const [syncProgress, setSyncProgress] = useState<SyncProgressData | null>(null);
  const [showSyncProgress, setShowSyncProgress] = useState(false);
  const [isPreparingRescan, setIsPreparingRescan] = useState(false); // Immediate loading state

  const fetchData = async () => {
    try {
      const walletData = await getWalletDashboard();
      setData(walletData);
      setError(null);
      
      // Check if wallet is syncing and show unified progress bar
      if (walletData.walletStatus.status === 'syncing' && !showSyncProgress) {
        console.log('Wallet syncing detected - activating progress bar stream');
        setSyncProgress({
          progress: walletData.walletStatus.syncProgress || 0,
          scanHeight: walletData.walletStatus.syncHeight || 0,
          chainHeight: 1016874, // Will be updated by WebSocket
          message: walletData.walletStatus.syncMessage || 'Connecting to sync stream...',
          isRescanning: true,
        });
        setShowSyncProgress(true);
      }
    } catch (err: any) {
      console.error('Error fetching wallet data:', err);
      
      // Handle errors appropriately
      if (err.code === 'ECONNABORTED' || err.message?.includes('timeout') || err.response?.status === 408) {
        if (!data) {
          setError('Initializing wallet status. This may take a moment.');
        }
      } else if (err.response?.status === 503) {
        setError('Wallet RPC not connected. Please ensure dcrwallet is running.');
      } else {
        setError(err.message || 'Failed to fetch wallet data');
      }
    } finally {
      setLoading(false);
    }
  };

  // Initial load - check for active rescan first
  useEffect(() => {
    const initialize = async () => {
      try {
        const progress = await getSyncProgress();
        if (progress.isRescanning && progress.progress < 100) {
          // Active rescan detected - show progress bar only
          console.log('Active rescan detected on load - showing progress bar');
          setSyncProgress(progress);
          setShowSyncProgress(true);
          setLoading(false);
          // Still fetch wallet data for the status card, but in background
          fetchData();
          return;
        }
      } catch (err) {
        console.log('No active rescan, loading wallet data normally');
      }
      
      // No active rescan - fetch wallet data normally
      fetchData();
    };
    
    initialize();
  }, []);

  // Auto-refresh wallet data every 10 seconds (but NOT during rescan)
  useEffect(() => {
    if (!showSyncProgress) {
      const interval = setInterval(fetchData, 10000);
      return () => clearInterval(interval);
    }
  }, [showSyncProgress]);

  // WebSocket streaming for wallet sync status (always active, purely reactive)
  useEffect(() => {
    console.log('🔌 Starting continuous wallet sync monitoring stream');
    
    const cleanup = streamRescanProgress(
      // onProgress callback - called every second with wallet state
      (progress) => {
        console.log('📊 Sync update:', {
          isRescanning: progress.isRescanning,
          scanHeight: progress.scanHeight,
          chainHeight: progress.chainHeight,
          progress: progress.progress,
          message: progress.message
        });
        
        // Purely reactive: Show progress bar when rescanning, hide when not
        if (progress.isRescanning) {
          // Wallet is behind chain - show progress bar
          if (!showSyncProgress) {
            console.log('✅ Rescan active - SHOWING progress bar');
          }
          setShowSyncProgress(true);
          setSyncProgress(progress);
          setIsPreparingRescan(false); // Clear preparing state once actual rescan starts
        } else {
          // Wallet is synced - hide progress bar
          if (showSyncProgress) {
            console.log('✅ Wallet synced - HIDING progress bar');
          }
          setShowSyncProgress(false);
          setIsPreparingRescan(false); // Clear preparing state
        }
      },
      // onError callback
      (error) => {
        console.error('❌ WebSocket error:', error);
      },
      // onClose callback
      () => {
        console.log('🔌 WebSocket stream closed - wallet fully synced');
        setShowSyncProgress(false);
        fetchData(); // Refresh wallet data
      }
    );

    // Cleanup WebSocket connection when component unmounts
    return cleanup;
  }, []); // Only run once on mount

  const handleImportSuccess = () => {
    // Show immediate loading state
    setIsPreparingRescan(true);
    setError(null);
    setShowImportModal(false);
    console.log('Xpub import initiated - showing preparing state');
    // WebSocket stream will automatically detect and show rescan progress
  };

  const handleRescan = async () => {
    if (showSyncProgress || isPreparingRescan) return; // Already rescanning
    
    if (!confirm('This will rescan the entire blockchain from block 0. This may take 30+ minutes. Continue?')) {
      return;
    }

    try {
      // Show immediate loading state
      setIsPreparingRescan(true);
      setError(null);
      console.log('Rescan initiated - showing preparing state');
      
      await triggerRescan();
      // WebSocket stream will automatically detect and show rescan progress
    } catch (err: any) {
      console.error('Error triggering rescan:', err);
      setError(err.response?.data?.error || err.message || 'Failed to trigger rescan');
      setIsPreparingRescan(false); // Clear preparing state on error
    }
  };

  return (
    <div className="space-y-6">
      {/* Action Buttons */}
      <div className="flex justify-end gap-3">
        <button
          onClick={handleRescan}
          disabled={showSyncProgress || isPreparingRescan || data?.walletStatus.status === 'no_wallet'}
          className="px-6 py-3 rounded-lg bg-muted/20 text-foreground font-semibold hover:bg-muted/30 transition-all flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <RefreshCw className={`h-5 w-5 ${(showSyncProgress || isPreparingRescan) ? 'animate-spin' : ''}`} />
          {isPreparingRescan ? 'Preparing...' : showSyncProgress ? 'Rescanning...' : 'Rescan'}
        </button>
        <button
          onClick={() => setShowImportModal(true)}
          disabled={showSyncProgress || isPreparingRescan}
          className="px-6 py-3 rounded-lg bg-gradient-primary text-white font-semibold transition-all flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Plus className="h-5 w-5" />
          Add X-Pub
        </button>
      </div>

      {/* Preparing State - immediate feedback when rescan/import is clicked */}
      {isPreparingRescan && (
        <div className="p-8 rounded-lg bg-card border border-border text-center animate-fade-in">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-primary border-t-transparent mb-4"></div>
          <h3 className="text-lg font-semibold mb-2">Preparing Rescan...</h3>
          <p className="text-muted-foreground">
            Discovering addresses and preparing blockchain scan. This may take a few moments.
          </p>
        </div>
      )}

      {/* Sync Progress Bar - shown during rescan */}
      {showSyncProgress && syncProgress && (
        <SyncProgressBar
          progress={syncProgress.progress}
          scanHeight={syncProgress.scanHeight}
          chainHeight={syncProgress.chainHeight}
          message={syncProgress.message}
        />
      )}

      {/* Error Message - hide during rescan */}
      {error && !showSyncProgress && !isPreparingRescan && (
        <div className="p-4 rounded-lg bg-red-500/10 border border-red-500/20 animate-fade-in">
          <p className="text-red-500 font-medium">{error}</p>
        </div>
      )}

      {/* Loading State - hide during rescan/preparing */}
      {loading && !data && !showSyncProgress && !isPreparingRescan && (
        <div className="text-center py-12">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-primary border-t-transparent"></div>
          <p className="mt-4 text-muted-foreground">Loading wallet data...</p>
        </div>
      )}

      {/* Wallet Status - always visible, but hides when unified progress bar or preparing is shown */}
      {data && !showSyncProgress && !isPreparingRescan && (
        <WalletStatus
          status={data.walletStatus.status as any}
          version={data.walletStatus.version}
          unlocked={data.walletStatus.unlocked}
        />
      )}

      {/* Hide wallet data cards during rescan/preparing to prevent RPC flooding */}
      {!showSyncProgress && !isPreparingRescan && (
        <>

          {/* Left Column: Account Balance + Accounts | Right Column: Transaction History */}
          {data && data.walletStatus.status !== 'no_wallet' && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 animate-fade-in">
          {/* Left Column */}
          <div className="space-y-6">
            {/* Account Info */}
            <AccountInfo
              accountName={data.accountInfo.accountName}
              totalBalance={data.accountInfo.totalBalance}
              spendableBalance={data.accountInfo.spendableBalance}
              immatureBalance={data.accountInfo.immatureBalance}
              unconfirmedBalance={data.accountInfo.unconfirmedBalance}
              lockedByTickets={data.accountInfo.lockedByTickets}
              cumulativeTotal={data.accountInfo.cumulativeTotal}
              totalSpendable={data.accountInfo.totalSpendable}
              totalLockedByTickets={data.accountInfo.totalLockedByTickets}
            />

            {/* Accounts List */}
            {data.accounts && (
              <AccountsList accounts={data.accounts} />
            )}
          </div>

          {/* Right Column: Transaction History */}
          {!loading && !showSyncProgress && !isPreparingRescan && (
            <TransactionHistory />
          )}
        </div>
      )}

          {/* Row 2: Ticket Pool | My Tickets */}
          {data && data.walletStatus.status !== 'no_wallet' && data.stakingInfo && (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 animate-fade-in">
              {/* Ticket Pool & Difficulty Info */}
              <TicketPoolInfo
                poolSize={data.stakingInfo.poolSize}
                currentDifficulty={data.stakingInfo.currentDifficulty}
                estimatedMin={data.stakingInfo.estimatedMin}
                estimatedMax={data.stakingInfo.estimatedMax}
                estimatedExpected={data.stakingInfo.estimatedExpected}
                allMempoolTix={data.stakingInfo.allMempoolTix}
              />

              {/* My Tickets Info */}
              <MyTicketsInfo
                ownMempoolTix={data.stakingInfo.ownMempoolTix}
                immature={data.stakingInfo.immature}
                unspent={data.stakingInfo.unspent}
                voted={data.stakingInfo.voted}
                revoked={data.stakingInfo.revoked}
                unspentExpired={data.stakingInfo.unspentExpired}
                totalSubsidy={data.stakingInfo.totalSubsidy}
              />
            </div>
          )}

          {/* Address Bookmarks */}
          <AddressBookmarksCard />

          {/* Last Update */}
          {data && (
            <div className="text-center text-sm text-muted-foreground animate-fade-in">
              Last updated: {new Date(data.lastUpdate).toLocaleString()}
            </div>
          )}
        </>
      )}

      {/* Import Xpub Modal */}
      <ImportXpubModal
        isOpen={showImportModal}
        onClose={() => setShowImportModal(false)}
        onSuccess={handleImportSuccess}
      />
    </div>
  );
};

