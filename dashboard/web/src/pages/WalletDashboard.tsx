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
import { WalletSetup } from '../components/WalletSetup';
import { getWalletDashboard, WalletDashboardData, triggerRescan, getSyncProgress, streamRescanProgress, SyncProgressData, checkWalletExists, checkWalletLoaded, openWallet } from '../services/api';

export const WalletDashboard = () => {
  const [walletExists, setWalletExists] = useState<boolean | null>(null);
  const [data, setData] = useState<WalletDashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showImportModal, setShowImportModal] = useState(false);
  const [syncProgress, setSyncProgress] = useState<SyncProgressData | null>(null);
  const [showSyncProgress, setShowSyncProgress] = useState(false);
  const [isPreparingRescan, setIsPreparingRescan] = useState(false); // Immediate loading state
  const [showPassphraseModal, setShowPassphraseModal] = useState(false);
  const [publicPassphrase, setPublicPassphrase] = useState('');
  const [passphraseError, setPassphraseError] = useState<string | null>(null);
  const [isOpeningWallet, setIsOpeningWallet] = useState(false);

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

  const handleOpenWallet = async () => {
    setIsOpeningWallet(true);
    setPassphraseError(null);
    
    try {
      await openWallet({ publicPassphrase });
      console.log('Wallet opened successfully with provided passphrase');
      setShowPassphraseModal(false);
      setPublicPassphrase('');
      
      // Reload the page to initialize everything with opened wallet
      window.location.reload();
    } catch (err: any) {
      const errorMsg = err.response?.data?.message || err.message || 'Failed to open wallet';
      setPassphraseError(errorMsg);
    } finally {
      setIsOpeningWallet(false);
    }
  };

  // Initial load - check if wallet exists first
  useEffect(() => {
    const initialize = async () => {
      try {
        // First check if wallet exists
        const existsResponse = await checkWalletExists();
        setWalletExists(existsResponse.exists);
        
        if (!existsResponse.exists) {
          // No wallet exists, show setup wizard
          setLoading(false);
          return;
        }

        // Wallet exists, check if it's already loaded
        try {
          const loadedResponse = await checkWalletLoaded();
          
          if (!loadedResponse.loaded) {
            // Wallet not loaded, try to open it with empty passphrase
            console.log('Wallet exists but not loaded, attempting to open...');
            try {
              await openWallet({ publicPassphrase: '' });
              console.log('Wallet opened successfully with empty passphrase');
            } catch (err: any) {
              const errorMsg = err.response?.data?.message || err.message || '';
              
              // Check if error is due to wrong passphrase
              if (errorMsg.includes('passphrase') || errorMsg.includes('invalid') || errorMsg.includes('incorrect')) {
                console.log('Wallet requires public passphrase - showing modal');
                setShowPassphraseModal(true);
                setLoading(false);
                return;
              }
              
              console.log('Failed to open wallet:', errorMsg);
              setLoading(false);
              return;
            }
          } else {
            console.log('Wallet already loaded and ready');
          }
        } catch (err: any) {
          console.log('Error checking wallet load status:', err);
        }

        // Check for active rescan
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
      } catch (err) {
        console.error('Error checking wallet existence:', err);
        setError('Failed to check wallet status');
        setLoading(false);
      }
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
        // Purely reactive: Show progress bar when rescanning, hide when not
        if (progress.isRescanning) {
          // Wallet is behind chain - show progress bar
          if (!showSyncProgress) {
            console.log('✅ Rescan active - showing progress bar');
          }
          setShowSyncProgress(true);
          setSyncProgress(progress);
          setIsPreparingRescan(false); // Clear preparing state once actual rescan starts
        } else {
          // Wallet is synced - hide progress bar
          if (showSyncProgress) {
            console.log('✅ Wallet synced - hiding progress bar');
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

  // Show wallet setup if no wallet exists
  if (walletExists === false) {
    return <WalletSetup />;
  }

  // Show loading state while checking wallet existence
  if (walletExists === null && loading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-center space-y-4">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-primary border-t-transparent"></div>
          <p className="text-muted-foreground">Checking wallet status...</p>
        </div>
      </div>
    );
  }

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

          {/* Row 1: Account Balance | Accounts */}
          {data && data.walletStatus.status !== 'no_wallet' && (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 animate-fade-in">
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
          )}

          {/* Row 2: Transaction History | My Tickets */}
          {data && data.walletStatus.status !== 'no_wallet' && (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 animate-fade-in">
              {/* Transaction History */}
              {!loading && !showSyncProgress && !isPreparingRescan && (
                <TransactionHistory />
              )}

              {/* My Tickets Info */}
              {data.stakingInfo && (
                <MyTicketsInfo
                  ownMempoolTix={data.stakingInfo.ownMempoolTix}
                  immature={data.stakingInfo.immature}
                  unspent={data.stakingInfo.unspent}
                  voted={data.stakingInfo.voted}
                  revoked={data.stakingInfo.revoked}
                  unspentExpired={data.stakingInfo.unspentExpired}
                  totalSubsidy={data.stakingInfo.totalSubsidy}
                />
              )}
            </div>
          )}

          {/* Row 3: Ticket Pool & Difficulty | Address Bookmarks */}
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

              {/* Address Bookmarks */}
              <AddressBookmarksCard />
            </div>
          )}

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

      {/* Public Passphrase Modal */}
      {showPassphraseModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-card rounded-lg shadow-xl max-w-md w-full p-6 space-y-4">
            <h2 className="text-2xl font-bold">Wallet Passphrase Required</h2>
            <p className="text-sm text-muted-foreground">
              Your wallet is protected with a public passphrase. Please enter it to open the wallet.
            </p>
            
            <div className="space-y-2">
              <label className="text-sm font-medium">Public Passphrase</label>
              <input
                type="password"
                value={publicPassphrase}
                onChange={(e) => {
                  setPublicPassphrase(e.target.value);
                  if (passphraseError) setPassphraseError(null);
                }}
                onKeyDown={(e) => e.key === 'Enter' && !isOpeningWallet && handleOpenWallet()}
                className="w-full px-4 py-2 bg-background border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                placeholder="Enter your public passphrase"
                autoFocus
              />
            </div>

            {passphraseError && (
              <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-lg">
                <p className="text-sm text-red-500">{passphraseError}</p>
              </div>
            )}

            <div className="flex gap-3">
              <button
                onClick={() => {
                  setShowPassphraseModal(false);
                  setPublicPassphrase('');
                  setPassphraseError(null);
                }}
                disabled={isOpeningWallet}
                className="flex-1 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                onClick={handleOpenWallet}
                disabled={isOpeningWallet || !publicPassphrase}
                className="flex-1 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-semibold"
              >
                {isOpeningWallet ? 'Opening...' : 'Open Wallet'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

