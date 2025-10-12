// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, useEffect } from 'react';
import { 
  CheckCircle2,
  Coins,
  Download,
  Upload,
  Trash2
} from 'lucide-react';
import { getTreasuryInfo } from '../../services/treasuryApi';
import { 
  getTreasuryStats, 
  exportTreasuryData,
  importTreasuryData,
  clearTreasuryData,
  getAllTSpends,
  TSpendRecord
} from '../../services/treasuryStorage';

export const TreasuryPaymentsCard = () => {
  const [loading, setLoading] = useState(true);
  const [localStats, setLocalStats] = useState(() => {
    const stats = getTreasuryStats();
    console.log('💳 TreasuryPaymentsCard initial state:', stats);
    return stats;
  });
  const [storedTSpends, setStoredTSpends] = useState<TSpendRecord[]>(() => {
    const tspends = getAllTSpends();
    console.log('💳 TreasuryPaymentsCard initial TSpends:', tspends.length);
    return tspends;
  });
  const [importText, setImportText] = useState('');
  const [showImport, setShowImport] = useState(false);

  const fetchData = async () => {
    try {
      console.log('💳 TreasuryPaymentsCard fetching data...');
      setLoading(true);
      await getTreasuryInfo();

      // Refresh stored TSpends from localStorage
      const freshTSpends = getAllTSpends();
      const freshStats = getTreasuryStats();
      console.log('💳 TreasuryPaymentsCard refreshed:', freshStats, `${freshTSpends.length} TSpends`);
      setStoredTSpends(freshTSpends);
      setLocalStats(freshStats);
      setLoading(false);
    } catch (err) {
      console.error('Failed to fetch treasury info:', err);
      setLoading(false);
    }
  };

  useEffect(() => {
    console.log('💳 TreasuryPaymentsCard mounted');
    fetchData();

    // Auto-refresh every 60 seconds
    const interval = setInterval(fetchData, 60000);
    return () => {
      console.log('💳 TreasuryPaymentsCard unmounted');
      clearInterval(interval);
    };
  }, []);

  const handleExport = () => {
    const data = exportTreasuryData();
    const blob = new Blob([data], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `treasury-data-${new Date().toISOString().split('T')[0]}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleImport = () => {
    if (!importText.trim()) {
      alert('Please paste JSON data to import');
      return;
    }

    const result = importTreasuryData(importText);
    if (result.success) {
      alert(`Successfully imported ${result.count} TSpends`);
      setImportText('');
      setShowImport(false);
      setLocalStats(getTreasuryStats());
      setStoredTSpends(getAllTSpends());
    } else {
      alert(`Import failed: ${result.error}`);
    }
  };

  const handleClearDatabase = () => {
    if (!confirm('Are you sure you want to clear all stored TSpend data?\n\nThis will permanently delete all locally stored treasury spend records. This action cannot be undone.')) {
      return;
    }

    try {
      clearTreasuryData();
      setLocalStats(getTreasuryStats());
      setStoredTSpends(getAllTSpends());
      alert('Treasury database cleared successfully');
    } catch (error) {
      console.error('Failed to clear treasury data:', error);
      alert('Failed to clear treasury database');
    }
  };

  const formatAmount = (amount: number) => {
    const integerPart = Math.floor(amount);
    const formattedInteger = integerPart.toLocaleString('en-US');
    const fullAmount = amount.toFixed(8);
    const mainPart = amount.toFixed(2);
    const decimalPart = fullAmount.substring(mainPart.length);
    
    return { mainPart: formattedInteger + mainPart.substring(integerPart.toString().length), decimalPart };
  };

  const formatHash = (hash: string) => {
    return `${hash.slice(0, 8)}...${hash.slice(-8)}`;
  };

  const formatAddress = (address: string) => {
    return `${address.slice(0, 8)}...${address.slice(-6)}`;
  };

  const formatTime = (timestamp: string) => {
    const date = new Date(timestamp);
    const now = new Date();
    const diff = now.getTime() - date.getTime();

    // Less than a day
    if (diff < 86400000) {
      const hours = Math.floor(diff / (1000 * 60 * 60));
      if (hours > 0) return `${hours}h ago`;
      const minutes = Math.floor(diff / (1000 * 60));
      return `${minutes}m ago`;
    }

    // Less than a week
    if (diff < 604800000) {
      const days = Math.floor(diff / (1000 * 60 * 60 * 24));
      return `${days}d ago`;
    }

    // Show date
    return date.toLocaleDateString();
  };

  const { mainPart, decimalPart } = formatAmount(localStats.totalSpent);

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      {/* Header with Total Spent and Icon */}
      <div className="flex items-start justify-between mb-6">
        <div className="flex-1">
          <p className="text-sm text-muted-foreground mb-1">Treasury Payments</p>
          <h3 className="text-3xl font-bold mb-1">
            {mainPart}
            <span className="text-lg opacity-70">{decimalPart}</span>
            {' '}
            <span className="text-xl">DCR</span>
          </h3>
          <p className="text-xs text-muted-foreground">
            {localStats.count} payments • Last sync: Block {localStats.lastSyncHeight.toLocaleString()}
          </p>
        </div>
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Coins className="h-5 w-5 text-primary" />
        </div>
      </div>

      {/* Action Buttons */}
      <div className="flex items-center gap-2 mb-6 pb-4 border-b border-border/50">
        <button
          onClick={handleExport}
          className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border border-border/50 hover:bg-muted/10 transition-colors"
          title="Export TSpend data as JSON"
        >
          <Download className="h-4 w-4" />
          Export
        </button>
        <button
          onClick={() => setShowImport(!showImport)}
          className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border border-border/50 hover:bg-muted/10 transition-colors"
          title="Import TSpend data from JSON"
        >
          <Upload className="h-4 w-4" />
          Import
        </button>
        <button
          onClick={handleClearDatabase}
          className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border border-red-500/50 text-red-500 hover:bg-red-500/10 transition-colors"
          title="Clear all stored TSpend data"
        >
          <Trash2 className="h-4 w-4" />
          Clear
        </button>
      </div>

      {/* Import Section */}
      {showImport && (
        <div className="mb-6 p-4 rounded-lg bg-muted/5 border border-border/30">
          <h3 className="font-semibold mb-2">Import Treasury Data</h3>
          <textarea
            value={importText}
            onChange={(e: any) => setImportText(e.target.value)}
            placeholder="Paste JSON data here..."
            className="w-full h-32 p-3 rounded-lg bg-background border border-border/50 font-mono text-sm"
          />
          <div className="flex gap-2 mt-2">
            <button
              onClick={handleImport}
              className="px-4 py-2 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
            >
              Import
            </button>
            <button
              onClick={() => {
                setShowImport(false);
                setImportText('');
              }}
              className="px-4 py-2 rounded-lg border border-border/50 hover:bg-muted/10 transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Loading State */}
      {loading ? (
        <div className="text-center py-8">
          <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-primary"></div>
          <p className="mt-2 text-sm text-muted-foreground">Loading payments...</p>
        </div>
      ) : (
        <>
          {/* Stored TSpends from localStorage */}
          {storedTSpends && storedTSpends.length > 0 && (
            <div>
              <div className="flex items-center gap-2 mb-3">
                <CheckCircle2 className="h-4 w-4 text-success" />
                <h3 className="font-semibold">Historical Treasury Spends</h3>
                <span className="text-xs text-muted-foreground">({storedTSpends.length} total)</span>
              </div>
              <div className="space-y-2">
                {storedTSpends.slice(0, 10).map((tspend: any) => (
                  <div
                    key={tspend.txHash}
                    className="p-3 rounded-lg bg-muted/5 border border-border/30 hover:bg-muted/10 transition-colors"
                  >
                    <div className="flex items-center justify-between mb-2">
                      <code className="text-sm font-mono">{formatHash(tspend.txHash)}</code>
                      <span className="text-sm font-semibold text-success">
                        {formatAmount(tspend.amount).mainPart}
                        <span className="text-xs opacity-70">{formatAmount(tspend.amount).decimalPart}</span>
                        {' DCR'}
                      </span>
                    </div>
                    <div className="flex items-center justify-between text-sm text-muted-foreground">
                      <span>To: {formatAddress(tspend.payee)}</span>
                      <span>Block {tspend.blockHeight.toLocaleString()} • {formatTime(tspend.timestamp)}</span>
                    </div>
                  </div>
                ))}
                {storedTSpends.length > 10 && (
                  <div className="text-center text-sm text-muted-foreground pt-2">
                    Showing 10 of {storedTSpends.length} TSpends
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Empty State */}
          {(!storedTSpends || storedTSpends.length === 0) && (
            <div className="text-center py-8 text-muted-foreground">
              <CheckCircle2 className="h-12 w-12 mx-auto mb-3 opacity-50" />
              <p>No treasury spends found</p>
              <p className="text-sm mt-1">Treasury spends will appear here when detected or scanned</p>
            </div>
          )}
        </>
      )}
    </div>
  );
};

