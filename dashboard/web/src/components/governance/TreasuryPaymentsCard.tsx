// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, useEffect } from 'react';
import { toYMD } from '../../utils/date';
import { 
  CheckCircle2,
  Coins,
  Download,
  Upload,
  Trash2
} from 'lucide-react';
import { 
  getTreasuryStats, 
  exportTreasuryData,
  importTreasuryData,
  clearTreasuryData,
  getAllTSpends,
  TSpendRecord
} from '../../services/treasuryStorage';

interface TreasuryPaymentsCardProps {
  // Bumped when a scan or snapshot sync writes new TSpends to localStorage.
  refreshKey?: number;
}

export const TreasuryPaymentsCard = ({ refreshKey = 0 }: TreasuryPaymentsCardProps) => {
  const [localStats, setLocalStats] = useState(() => getTreasuryStats());
  const [storedTSpends, setStoredTSpends] = useState<TSpendRecord[]>(() => getAllTSpends());
  const [importText, setImportText] = useState('');
  const [showImport, setShowImport] = useState(false);

  // This card renders only localStorage-backed data. It used to await
  // getTreasuryInfo() every 60s and discard the response.
  const fetchData = () => {
    setStoredTSpends(getAllTSpends());
    setLocalStats(getTreasuryStats());
  };

  useEffect(() => {
    fetchData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshKey]);

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
      alert('Treasury data imported');
      setImportText('');
      setShowImport(false);
      setLocalStats(getTreasuryStats());
      setStoredTSpends(getAllTSpends());
    } else {
      alert(`Import failed: ${result.error}`);
    }
  };

  const handleClearDatabase = () => {
    if (!confirm('Are you sure you want to clear all stored treasury data?\n\nThis deletes the locally stored treasury records; the shipped snapshot reloads with the page. This action cannot be undone.')) {
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

  // Splits an atom amount into "1,234.56" and the remaining six decimals.
  const formatAmount = (atoms: number) => {
    const whole = Math.floor(atoms / 1e8);
    const frac = String(atoms % 1e8).padStart(8, '0');
    return { mainPart: `${whole.toLocaleString('en-US')}.${frac.slice(0, 2)}`, decimalPart: frac.slice(2) };
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
    return toYMD(date);
  };

  const { mainPart, decimalPart } = formatAmount(localStats.totalSpentAtoms);

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
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
            {localStats.count} payments, fees included • Last sync: Block {localStats.lastSyncHeight.toLocaleString()}
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
          title="Export treasury data as JSON"
        >
          <Download className="h-4 w-4" />
          Export
        </button>
        <button
          onClick={() => setShowImport(!showImport)}
          className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border border-border/50 hover:bg-muted/10 transition-colors"
          title="Import treasury data from JSON"
        >
          <Upload className="h-4 w-4" />
          Import
        </button>
        <button
          onClick={handleClearDatabase}
          className="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border border-red-500/50 text-red-500 hover:bg-red-500/10 transition-colors"
          title="Clear all stored treasury data"
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

      <>
          {/* Stored TSpends from localStorage */}
          {storedTSpends && storedTSpends.length > 0 && (
            <div>
              <div className="flex items-center gap-2 mb-3">
                <CheckCircle2 className="h-4 w-4 text-success" />
                <h3 className="font-semibold">Historical Treasury Spends</h3>
                <span className="text-xs text-muted-foreground">({storedTSpends.length} total)</span>
              </div>
              <div className="space-y-2 max-h-96 overflow-y-auto pr-1">
                {storedTSpends.map((tspend) => (
                  <div
                    key={tspend.txHash}
                    className="p-3 rounded-lg bg-muted/5 border border-border/30 hover:bg-muted/10 transition-colors"
                  >
                    <div className="flex items-center justify-between mb-2">
                      <code className="text-sm font-mono">{formatHash(tspend.txHash)}</code>
                      <span className="text-sm font-semibold text-success">
                        {formatAmount(tspend.amountAtoms).mainPart}
                        <span className="text-xs opacity-70">{formatAmount(tspend.amountAtoms).decimalPart}</span>
                        {' DCR'}
                      </span>
                    </div>
                    <div className="flex items-center justify-between text-sm text-muted-foreground">
                      <span>To: {formatAddress(tspend.payee)}</span>
                      <span>Block {tspend.blockHeight.toLocaleString()} • {formatTime(tspend.timestamp)}</span>
                    </div>
                  </div>
                ))}
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
    </div>
  );
};

