// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { getWalletTransactions, WalletTransaction } from '../services/api';
import { ArrowDownCircle, ArrowUpCircle, Ticket, Check, X, Coins, Clock, ChevronDown, Shuffle } from 'lucide-react';

export const TransactionHistory = () => {
  const [transactions, setTransactions] = useState<WalletTransaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [visibleCount, setVisibleCount] = useState(10);
  const loadMoreCount = 50;

  useEffect(() => {
    fetchTransactions();
  }, []);

  const fetchTransactions = async () => {
    try {
      setLoading(true);
      const data = await getWalletTransactions(100); // Fetch up to 100 transactions
      setTransactions(data.transactions);
      setError(null);
    } catch (err) {
      console.error('Error fetching transactions:', err);
      setError('Failed to load transaction history');
    } finally {
      setLoading(false);
    }
  };

  const getCategoryIcon = (category: string, txType: string, isMixed: boolean) => {
    if (txType === 'ticket') return <Ticket className="h-5 w-5 text-warning" />;
    if (txType === 'vote') return <Check className="h-5 w-5 text-success" />;
    if (txType === 'revocation') return <X className="h-5 w-5 text-destructive" />;
    if (category === 'send' && isMixed) return <Shuffle className="h-5 w-5 text-purple-500" />;
    if (category === 'send') return <ArrowUpCircle className="h-5 w-5 text-red-500" />;
    if (category === 'receive' && isMixed) return <Shuffle className="h-5 w-5 text-purple-500" />;
    if (category === 'receive') return <ArrowDownCircle className="h-5 w-5 text-success" />;
    if (category === 'generate') return <Coins className="h-5 w-5 text-primary" />;
    if (category === 'immature') return <Clock className="h-5 w-5 text-muted-foreground" />;
    return <Coins className="h-5 w-5 text-muted-foreground" />;
  };

  const getCategoryLabel = (category: string, txType: string, isMixed: boolean) => {
    if (txType === 'ticket') return 'Ticket Purchase';
    if (txType === 'vote') return 'Vote';
    if (txType === 'revocation') return 'Revocation';
    if (category === 'send' && isMixed) return 'Sent (CoinJoin)';
    if (category === 'send') return 'Sent';
    if (category === 'receive' && isMixed) return 'Received (CoinJoin)';
    if (category === 'receive') return 'Received';
    if (category === 'generate') return 'Mined';
    if (category === 'immature') return 'Immature';
    return 'Transaction';
  };

  const getCategoryColor = (category: string, txType: string) => {
    if (txType === 'ticket') return 'text-warning';
    if (txType === 'vote') return 'text-success';
    if (txType === 'revocation') return 'text-destructive';
    if (category === 'send') return 'text-red-500';
    if (category === 'receive') return 'text-success';
    if (category === 'generate') return 'text-primary';
    if (category === 'immature') return 'text-muted-foreground';
    return 'text-muted-foreground';
  };

  const formatAmount = (amount: number, category: string) => {
    const abs = Math.abs(amount);
    const sign = category === 'send' || amount < 0 ? '-' : '+';
    return `${sign}${abs.toFixed(8)} DCR`;
  };

  const formatDate = (tx: WalletTransaction) => {
    // Use blockTime for confirmed transactions (when it was included in a block)
    // Fall back to time for pending transactions
    const timestamp = tx.blockTime ? tx.blockTime * 1000 : new Date(tx.time).getTime();
    const date = new Date(timestamp);
    const now = new Date();
    const diffInSeconds = Math.floor((now.getTime() - date.getTime()) / 1000);

    if (diffInSeconds < 60) return 'Just now';
    if (diffInSeconds < 3600) return `${Math.floor(diffInSeconds / 60)}m ago`;
    if (diffInSeconds < 86400) return `${Math.floor(diffInSeconds / 3600)}h ago`;
    if (diffInSeconds < 604800) return `${Math.floor(diffInSeconds / 86400)}d ago`;

    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined,
    });
  };

  const truncateTxid = (txid: string) => {
    return `${txid.substring(0, 8)}...${txid.substring(txid.length - 8)}`;
  };

  const displayedTransactions = transactions.slice(0, visibleCount);
  const hasMore = transactions.length > visibleCount;
  const remainingCount = transactions.length - visibleCount;

  if (loading) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-center gap-3 mb-6">
          <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
            <Clock className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">Transaction History</h2>
            <p className="text-sm text-muted-foreground">Recent wallet activity</p>
          </div>
        </div>
        <div className="flex items-center justify-center py-8">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          <span className="ml-3 text-muted-foreground">Loading transactions...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-center gap-3 mb-6">
          <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
            <Clock className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">Transaction History</h2>
            <p className="text-sm text-muted-foreground">Recent wallet activity</p>
          </div>
        </div>
        <div className="text-center py-8 text-muted-foreground">
          <p>{error}</p>
        </div>
      </div>
    );
  }

  if (transactions.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-center gap-3 mb-6">
          <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
            <Clock className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">Transaction History</h2>
            <p className="text-sm text-muted-foreground">Recent wallet activity</p>
          </div>
        </div>
        <div className="text-center py-8 text-muted-foreground">
          <p>No transactions found</p>
          <p className="text-sm mt-2">Transactions will appear here once your wallet receives or sends DCR</p>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
          <Clock className="h-5 w-5 text-primary" />
        </div>
        <div className="flex-1">
          <div className="flex items-center justify-between">
            <h2 className="text-xl font-semibold">Transaction History</h2>
            <span className="text-sm text-muted-foreground">{transactions.length} transactions</span>
          </div>
          <p className="text-sm text-muted-foreground">Recent wallet activity</p>
        </div>
      </div>

      <div className="space-y-2">
        {displayedTransactions.map((tx, index) => (
          <Link
            key={`${tx.txid}-${tx.vout}-${index}`}
            to={`/explorer/tx/${tx.txid}`}
            className="flex items-center justify-between p-4 rounded-lg bg-background/50 hover:bg-background transition-colors border border-border/30 hover:border-primary/30 cursor-pointer"
          >
            <div className="flex items-center gap-4 flex-1 min-w-0">
              <div className="flex-shrink-0">
                {getCategoryIcon(tx.category, tx.txType, tx.isMixed || false)}
              </div>
              
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <span className="font-medium">{getCategoryLabel(tx.category, tx.txType, tx.isMixed || false)}</span>
                  {tx.confirmations === 0 && (
                    <span className="text-xs px-2 py-0.5 rounded bg-warning/10 text-warning">
                      Pending
                    </span>
                  )}
                  {tx.confirmations > 0 && tx.confirmations < 6 && (
                    <span className="text-xs px-2 py-0.5 rounded bg-muted text-muted-foreground">
                      {tx.confirmations} conf
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <code className="font-mono text-xs">{truncateTxid(tx.txid)}</code>
                  <span>•</span>
                  <span>{formatDate(tx)}</span>
                  {tx.address && (
                    <>
                      <span>•</span>
                      <code className="font-mono text-xs">{tx.address.substring(0, 10)}...</code>
                    </>
                  )}
                </div>
              </div>
            </div>

            <div className="text-right ml-4">
              <div className={`text-lg font-semibold ${getCategoryColor(tx.category, tx.txType)}`}>
                {formatAmount(tx.amount, tx.category)}
              </div>
              {tx.fee && tx.fee > 0 && (
                <div className="text-xs text-muted-foreground">
                  Fee: {tx.fee.toFixed(8)} DCR
                </div>
              )}
            </div>
          </Link>
        ))}
      </div>

      {hasMore && (
        <button
          onClick={() => setVisibleCount(prev => prev + loadMoreCount)}
          className="mt-4 w-full flex items-center justify-center gap-2 py-2 px-4 rounded-lg border border-border hover:bg-background transition-colors text-sm"
        >
          <ChevronDown className="h-4 w-4" />
          Load {Math.min(loadMoreCount, remainingCount)} More
          {remainingCount > loadMoreCount && ` (${remainingCount} remaining)`}
        </button>
      )}
    </div>
  );
};

