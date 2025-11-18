// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Activity, ChevronLeft, ArrowRightLeft, Ticket, CheckCircle, XCircle, Landmark, Shuffle } from 'lucide-react';
import { getMempoolTransactions, MempoolTransactions, TransactionSummary } from '../services/explorerApi';
import { CopyButton } from '../components/explorer/CopyButton';

export const MempoolView = () => {
  const navigate = useNavigate();
  const [mempool, setMempool] = useState<MempoolTransactions | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    fetchMempool();
    
    // Auto-refresh every 30 seconds
    const interval = setInterval(fetchMempool, 30000);
    return () => clearInterval(interval);
  }, []);

  const fetchMempool = async () => {
    try {
      const data = await getMempoolTransactions();
      setMempool(data);
      setLoading(false);
      setError('');
    } catch (err) {
      setError('Failed to load mempool transactions');
      setLoading(false);
    }
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  const getTxTypeIcon = (type: string) => {
    switch (type) {
      case 'ticket':
        return <Ticket className="h-4 w-4 text-warning" />;
      case 'vote':
        return <CheckCircle className="h-4 w-4 text-success" />;
      case 'revocation':
        return <XCircle className="h-4 w-4 text-red-500" />;
      case 'tspend':
        return <Landmark className="h-4 w-4 text-amber-500" />;
      case 'treasurybase':
        return <Landmark className="h-4 w-4 text-amber-600" />;
      case 'coinjoin':
        return <Shuffle className="h-4 w-4 text-purple-500" />;
      default:
        return <ArrowRightLeft className="h-4 w-4 text-blue-500" />;
    }
  };

  const getTxTypeColor = (type: string) => {
    switch (type) {
      case 'ticket':
        return 'text-warning';
      case 'vote':
        return 'text-success';
      case 'revocation':
        return 'text-red-500';
      case 'tspend':
        return 'text-amber-500';
      case 'treasurybase':
        return 'text-amber-600';
      case 'coinjoin':
        return 'text-purple-500';
      default:
        return 'text-blue-500';
    }
  };

  const groupTransactionsByType = (): {
    treasury: TransactionSummary[];
    tickets: TransactionSummary[];
    votes: TransactionSummary[];
    revocations: TransactionSummary[];
    coinjoin: TransactionSummary[];
    regular: TransactionSummary[];
  } => {
    if (!mempool) return { treasury: [], tickets: [], votes: [], revocations: [], coinjoin: [], regular: [] };

    return {
      treasury: mempool.transactions.filter(tx => tx.type === 'tspend' || tx.type === 'treasurybase'),
      tickets: mempool.transactions.filter(tx => tx.type === 'ticket'),
      votes: mempool.transactions.filter(tx => tx.type === 'vote'),
      revocations: mempool.transactions.filter(tx => tx.type === 'revocation'),
      coinjoin: mempool.transactions.filter(tx => tx.type === 'coinjoin'),
      regular: mempool.transactions.filter(tx => tx.type === 'regular'),
    };
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center py-20">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-primary"></div>
            <p className="mt-4 text-muted-foreground">Loading mempool...</p>
          </div>
        </div>
      </div>
    );
  }

  if (error || !mempool) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center py-20">
            <Activity className="h-16 w-16 mx-auto text-muted-foreground/50 mb-4" />
            <h2 className="text-2xl font-bold mb-2">Failed to Load Mempool</h2>
            <p className="text-muted-foreground mb-6">{error}</p>
            <button
              onClick={() => navigate('/explorer')}
              className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
            >
              Back to Explorer
            </button>
          </div>
        </div>
      </div>
    );
  }

  const txGroups = groupTransactionsByType();

  return (
    <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
      <div className="max-w-7xl mx-auto space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <button
              onClick={() => navigate('/explorer')}
              className="p-2 rounded-lg hover:bg-muted/20 transition-colors"
            >
              <ChevronLeft className="h-6 w-6" />
            </button>
            <div>
              <h1 className="text-3xl font-bold">Mempool</h1>
              <p className="text-sm text-muted-foreground">
                {mempool.count} pending transaction{mempool.count !== 1 ? 's' : ''}
              </p>
            </div>
          </div>
        </div>

        {/* Mempool Summary Card */}
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
          <div className="flex items-center gap-2 mb-6">
            <Activity className="h-5 w-5 text-primary" />
            <h2 className="text-xl font-semibold">Mempool Summary</h2>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {/* Transaction Count */}
            <div className="p-4 rounded-lg bg-background/50">
              <p className="text-sm text-muted-foreground mb-2">Pending Transactions</p>
              <p className="text-2xl font-bold">{mempool.count.toLocaleString()}</p>
            </div>

            {/* Mempool Size */}
            <div className="p-4 rounded-lg bg-background/50">
              <p className="text-sm text-muted-foreground mb-2">Total Size</p>
              <p className="text-2xl font-bold">{formatSize(mempool.size)}</p>
            </div>

            {/* Transaction Types */}
            <div className="p-4 rounded-lg bg-background/50">
              <p className="text-sm text-muted-foreground mb-2">Transaction Breakdown</p>
              <div className="text-sm space-y-1">
                {txGroups.treasury.length > 0 && <p>Treasury: {txGroups.treasury.length}</p>}
                {txGroups.tickets.length > 0 && <p>Tickets: {txGroups.tickets.length}</p>}
                {txGroups.votes.length > 0 && <p>Votes: {txGroups.votes.length}</p>}
                {txGroups.revocations.length > 0 && <p>Revocations: {txGroups.revocations.length}</p>}
                {txGroups.coinjoin.length > 0 && <p>CoinJoin: {txGroups.coinjoin.length}</p>}
                {txGroups.regular.length > 0 && <p>Regular: {txGroups.regular.length}</p>}
              </div>
            </div>
          </div>
        </div>

        {/* Transactions List */}
        {mempool.count === 0 ? (
          <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
            <div className="text-center py-8">
              <Activity className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
              <p className="text-muted-foreground">Mempool is empty</p>
            </div>
          </div>
        ) : (
          <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
            <div className="flex items-center gap-2 mb-6">
              <ArrowRightLeft className="h-5 w-5 text-primary" />
              <h2 className="text-xl font-semibold">Transactions ({mempool.count})</h2>
            </div>

            <div className="space-y-6">
              {/* Treasury Transactions */}
              {txGroups.treasury.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-amber-500 mb-3 flex items-center gap-2">
                    <Landmark className="h-4 w-4" />
                    Treasury ({txGroups.treasury.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.treasury.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-amber-500/5 hover:bg-amber-500/10 border border-amber-500/20 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <div className="flex flex-col min-w-0 flex-1">
                              <span className="font-mono text-sm truncate">{tx.txid}</span>
                              <span className="text-xs text-muted-foreground">
                                {tx.type === 'tspend' ? 'Treasury Spend' : 'Treasury Addition'}
                              </span>
                            </div>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type) + ' font-semibold'}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Ticket Transactions */}
              {txGroups.tickets.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-warning mb-3 flex items-center gap-2">
                    <Ticket className="h-4 w-4" />
                    Tickets ({txGroups.tickets.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.tickets.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Vote Transactions */}
              {txGroups.votes.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-success mb-3 flex items-center gap-2">
                    <CheckCircle className="h-4 w-4" />
                    Votes ({txGroups.votes.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.votes.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Revocation Transactions */}
              {txGroups.revocations.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-red-500 mb-3 flex items-center gap-2">
                    <XCircle className="h-4 w-4" />
                    Revocations ({txGroups.revocations.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.revocations.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* CoinJoin Transactions */}
              {txGroups.coinjoin.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-purple-500 mb-3 flex items-center gap-2">
                    <Shuffle className="h-4 w-4" />
                    CoinJoin ({txGroups.coinjoin.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.coinjoin.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Regular Transactions */}
              {txGroups.regular.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-blue-500 mb-3 flex items-center gap-2">
                    <ArrowRightLeft className="h-4 w-4" />
                    Regular ({txGroups.regular.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.regular.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Auto-refresh notice */}
        <div className="text-center text-sm text-muted-foreground animate-fade-in">
          Auto-refreshing every 30 seconds
        </div>
      </div>
    </div>
  );
};

