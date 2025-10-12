// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { 
  Landmark, 
  ArrowDownToLine, 
  ArrowUpFromLine, 
  Clock, 
  CheckCircle2,
  AlertCircle,
  Download,
  Upload,
  Trash2
} from 'lucide-react';
import { getTreasuryInfo, TreasuryInfo } from '../../services/treasuryApi';
import { 
  getTreasuryStats, 
  exportTreasuryData,
  importTreasuryData,
  clearTreasuryData,
  getAllTSpends,
  TSpendRecord
} from '../../services/treasuryStorage';

export const TreasuryCard = () => {
  const [info, setInfo] = useState<TreasuryInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [localStats, setLocalStats] = useState(getTreasuryStats());
  const [storedTSpends, setStoredTSpends] = useState<TSpendRecord[]>(getAllTSpends());
  const [importText, setImportText] = useState('');
  const [showImport, setShowImport] = useState(false);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getTreasuryInfo();
      setInfo(data);

      // Refresh stored TSpends from localStorage
      setStoredTSpends(getAllTSpends());
      setLocalStats(getTreasuryStats());
      setLoading(false);
    } catch (err) {
      console.error('Failed to fetch treasury info:', err);
      setError(err instanceof Error ? err.message : 'Failed to load treasury data');
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();

    // Auto-refresh every 60 seconds
    const interval = setInterval(fetchData, 60000);
    return () => clearInterval(interval);
  }, []);

  const handleExport = () => {
    const json = exportTreasuryData();
    const blob = new Blob([json], { type: 'application/json' });
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
    return amount.toLocaleString(undefined, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 8,
    });
  };

  const formatHash = (hash: string) => {
    if (!hash) return 'N/A';
    return `${hash.substring(0, 8)}...${hash.substring(hash.length - 8)}`;
  };

  const formatAddress = (address: string) => {
    if (!address) return 'N/A';
    return `${address.substring(0, 10)}...${address.substring(address.length - 10)}`;
  };

  const formatTime = (timestamp: string) => {
    try {
      const date = new Date(timestamp);
      const now = new Date();
      const diff = now.getTime() - date.getTime();
      const days = Math.floor(diff / (1000 * 60 * 60 * 24));
      
      if (days === 0) {
        const hours = Math.floor(diff / (1000 * 60 * 60));
        if (hours === 0) {
          const minutes = Math.floor(diff / (1000 * 60));
          return `${minutes}m ago`;
        }
        return `${hours}h ago`;
      }
      if (days < 30) return `${days}d ago`;
      return date.toLocaleDateString();
    } catch {
      return timestamp;
    }
  };

  if (error) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-center gap-2 mb-4">
          <Landmark className="h-5 w-5 text-warning" />
          <h2 className="text-xl font-semibold">Treasury Status</h2>
        </div>
        <div className="flex items-center gap-2 text-destructive">
          <AlertCircle className="h-5 w-5" />
          <span>{error}</span>
        </div>
      </div>
    );
  }

  if (loading && !info) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-center gap-2 mb-4">
          <Landmark className="h-5 w-5 text-warning" />
          <h2 className="text-xl font-semibold">Treasury Status</h2>
        </div>
        <div className="text-center py-8">
          <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          <p className="mt-4 text-muted-foreground">Loading treasury data...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-2">
          <Landmark className="h-5 w-5 text-warning" />
          <h2 className="text-xl font-semibold">Treasury Status</h2>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleExport}
            className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border border-border/50 hover:bg-muted/10 transition-colors"
            title="Export treasury data"
          >
            <Download className="h-4 w-4" />
            Export
          </button>
          <button
            onClick={() => setShowImport(!showImport)}
            className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border border-border/50 hover:bg-muted/10 transition-colors"
            title="Import treasury data"
          >
            <Upload className="h-4 w-4" />
            Import
          </button>
          <button
            onClick={handleClearDatabase}
            className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border border-destructive/50 text-destructive hover:bg-destructive/10 transition-colors"
            title="Clear all stored TSpend data"
          >
            <Trash2 className="h-4 w-4" />
            Clear
          </button>
        </div>
      </div>

      {/* Import Section */}
      {showImport && (
        <div className="mb-6 p-4 rounded-lg border border-border/50 bg-muted/5">
          <h3 className="font-medium mb-2">Import Treasury Data</h3>
          <textarea
            className="w-full h-32 px-3 py-2 rounded-lg bg-background border border-border/50 text-sm font-mono"
            placeholder="Paste JSON data here..."
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
          />
          <div className="flex gap-2 mt-2">
            <button
              onClick={handleImport}
              className="px-4 py-2 text-sm rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
            >
              Import
            </button>
            <button
              onClick={() => {
                setShowImport(false);
                setImportText('');
              }}
              className="px-4 py-2 text-sm rounded-lg border border-border/50 hover:bg-muted/10 transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Balance Section */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <div className="p-4 rounded-lg bg-muted/5 border border-border/30">
          <div className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
            <ArrowDownToLine className="h-4 w-4" />
            Current Balance
          </div>
          <div className="text-2xl font-semibold">
            {info ? formatAmount(info.balance) : 'N/A'} DCR
          </div>
          {info && info.balanceUsd > 0 && (
            <div className="text-sm text-muted-foreground mt-1">
              ${info.balanceUsd.toLocaleString()}
            </div>
          )}
        </div>

        <div className="p-4 rounded-lg bg-muted/5 border border-border/30">
          <div className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
            <ArrowUpFromLine className="h-4 w-4" />
            Total Spent (Tracked)
          </div>
          <div className="text-2xl font-semibold">
            {formatAmount(localStats.totalSpent)} DCR
          </div>
          <div className="text-sm text-muted-foreground mt-1">
            {localStats.count} TSpends recorded
          </div>
        </div>
      </div>

      {/* Active TSpends in Mempool */}
      {info && info.activeTSpends && Array.isArray(info.activeTSpends) && info.activeTSpends.length > 0 && (
        <div className="mb-6">
          <div className="flex items-center gap-2 mb-3">
            <Clock className="h-4 w-4 text-warning" />
            <h3 className="font-semibold">Active Treasury Spends (Voting)</h3>
          </div>
          <div className="space-y-2">
            {info.activeTSpends.map((tspend) => (
              <div
                key={tspend.txHash}
                className="p-3 rounded-lg bg-muted/5 border border-border/30 hover:bg-muted/10 transition-colors"
              >
                <div className="flex items-center justify-between mb-2">
                  <code className="text-sm font-mono">{formatHash(tspend.txHash)}</code>
                  <span className="text-sm font-semibold text-warning">
                    {formatAmount(tspend.amount)} DCR
                  </span>
                </div>
                <div className="flex items-center justify-between text-sm text-muted-foreground">
                  <span>To: {formatAddress(tspend.payee)}</span>
                  <span>{tspend.blocksRemaining} blocks remaining</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Stored TSpends from localStorage */}
      {storedTSpends && storedTSpends.length > 0 && (
        <div>
          <div className="flex items-center gap-2 mb-3">
            <CheckCircle2 className="h-4 w-4 text-success" />
            <h3 className="font-semibold">Stored Treasury Spends</h3>
            <span className="text-xs text-muted-foreground">({storedTSpends.length} total)</span>
          </div>
          <div className="space-y-2">
            {storedTSpends.slice(0, 10).map((tspend) => (
              <div
                key={tspend.txHash}
                className="p-3 rounded-lg bg-muted/5 border border-border/30 hover:bg-muted/10 transition-colors"
              >
                <div className="flex items-center justify-between mb-2">
                  <code className="text-sm font-mono">{formatHash(tspend.txHash)}</code>
                  <span className="text-sm font-semibold text-success">
                    {formatAmount(tspend.amount)} DCR
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
      {info && 
        (!info.activeTSpends || !Array.isArray(info.activeTSpends) || info.activeTSpends.length === 0) && 
        (!storedTSpends || storedTSpends.length === 0) && (
        <div className="text-center py-8 text-muted-foreground">
          <Landmark className="h-12 w-12 mx-auto mb-3 opacity-50" />
          <p>No treasury spends found</p>
          <p className="text-sm mt-1">Treasury spends will appear here when detected or scanned</p>
        </div>
      )}
    </div>
  );
};

