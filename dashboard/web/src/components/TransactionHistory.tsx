// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { getWalletTransactions, WalletTransaction } from '../services/api';
import { ArrowDownCircle, ArrowUpCircle, Ticket, Check, X, Coins, Clock, ChevronDown, ChevronUp, Shuffle, BadgeDollarSign } from 'lucide-react';

export const TransactionHistory = () => {
  const [transactions, setTransactions] = useState<WalletTransaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [visibleCount, setVisibleCount] = useState(5);
  const [isExpanded, setIsExpanded] = useState(false);
  const [filterCategory, setFilterCategory] = useState<'all' | 'send' | 'receive' | 'coinjoin' | 'ticket' | 'vote'>('all');
  const [hasMoreToFetch, setHasMoreToFetch] = useState(true);
  const loadMoreCount = 10;
  const lazyLoadBatchSize = 50;

  useEffect(() => {
    fetchTransactions();
  }, []);

  const fetchTransactions = async () => {
    try {
      setLoading(true);
      const data = await getWalletTransactions(200); // Initial load: 200 transactions
      setTransactions(data.transactions);
      setHasMoreToFetch(data.transactions.length === 200);
      setError(null);
    } catch (err) {
      console.error('Error fetching transactions:', err);
      setError('Failed to load transaction history');
    } finally {
      setLoading(false);
    }
  };

  const loadMoreTransactions = async () => {
    if (loadingMore || !hasMoreToFetch) return;
    
    try {
      setLoadingMore(true);
      const currentCount = transactions.length;
      const data = await getWalletTransactions(lazyLoadBatchSize, currentCount);
      
      if (data.transactions.length > 0) {
        setTransactions(prev => [...prev, ...data.transactions]);
        setHasMoreToFetch(data.transactions.length === lazyLoadBatchSize);
        // Increase visible count to show the newly loaded transactions
        setVisibleCount(prev => prev + data.transactions.length);
      } else {
        setHasMoreToFetch(false);
      }
    } catch (err) {
      console.error('Error loading more transactions:', err);
    } finally {
      setLoadingMore(false);
    }
  };

  // Format large numbers with "k" suffix for thousands
  const formatCount = (count: number): string => {
    if (count >= 1000) {
      return (count / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
    }
    return count.toString();
  };

  const getCategoryIcon = (category: string, txType: string, isMixed: boolean) => {
    if (txType === 'ticket') return <Ticket className="h-5 w-5 text-warning" />;
    if (txType === 'vote') return <Check className="h-5 w-5 text-success" />;
    if (txType === 'revocation') return <X className="h-5 w-5 text-destructive" />;
    if (category === 'vspfee') return <BadgeDollarSign className="h-5 w-5 text-orange-500" />;
    if (category === 'coinjoin') return <Shuffle className="h-5 w-5 text-purple-500" />;
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
    if (category === 'vspfee') return 'VSP Fee';
    if (category === 'coinjoin') return 'CoinJoin';
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
    if (category === 'vspfee') return 'text-orange-500';
    if (category === 'coinjoin') return 'text-purple-500';
    if (category === 'send') return 'text-red-500';
    if (category === 'receive') return 'text-success';
    if (category === 'generate') return 'text-primary';
    if (category === 'immature') return 'text-muted-foreground';
    return 'text-muted-foreground';
  };

  const formatAmount = (amount: number) => {
    const abs = Math.abs(amount);
    const sign = amount < 0 ? '-' : '+';
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

  // Calculate CoinJoin statistics for display card
  const getCoinJoinStats = () => {
    const coinjoins = transactions.filter(tx => tx.category === 'coinjoin');
    if (coinjoins.length === 0) {
      return null;
    }

    const fees = coinjoins.map(tx => Math.abs(tx.amount));
    const minFee = Math.min(...fees);
    const maxFee = Math.max(...fees);
    const avgFee = fees.reduce((sum, fee) => sum + fee, 0) / fees.length;

    return {
      count: coinjoins.length,
      minFee,
      maxFee,
      avgFee
    };
  };

  const coinJoinStats = getCoinJoinStats();
  
  // Filter transactions based on selected category
  const filteredTransactions = transactions.filter(tx => {
    if (filterCategory === 'all') return true;
    // Send: only regular sends + VSP fees (exclude ticket purchases)
    if (filterCategory === 'send') {
      return (tx.category === 'send' && tx.txType === 'regular') || tx.category === 'vspfee';
    }
    if (filterCategory === 'receive') return tx.category === 'receive';
    if (filterCategory === 'coinjoin') return tx.category === 'coinjoin';
    if (filterCategory === 'ticket') return tx.txType === 'ticket';
    if (filterCategory === 'vote') return tx.txType === 'vote';
    return true;
  });

  const displayedTransactions = filteredTransactions.slice(0, visibleCount);
  const hasMoreVisible = filteredTransactions.length > visibleCount;
  const showLoadMore = hasMoreVisible || (filterCategory === 'all' && hasMoreToFetch);
  
  const handleLoadMore = () => {
    if (hasMoreVisible) {
      // Show more from already loaded transactions
      setVisibleCount(prev => prev + loadMoreCount);
    } else if (filterCategory === 'all' && hasMoreToFetch) {
      // Fetch more transactions from server
      loadMoreTransactions();
    }
  };

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
            <span className="text-sm text-muted-foreground">
              {formatCount(transactions.length)}{hasMoreToFetch ? '+' : ''} transactions
            </span>
          </div>
          <p className="text-sm text-muted-foreground">Recent wallet activity</p>
        </div>
      </div>

      {/* Statistics Section - matches MyTicketsInfo height */}
      <div className="mb-6 p-4 rounded-lg bg-muted/20 border border-border/30">
        {coinJoinStats ? (
          <>
            <div className="flex items-center gap-2 mb-3">
              <Shuffle className="h-4 w-4 text-purple-500" />
              <h3 className="text-sm font-semibold text-purple-500">CoinJoin Statistics</h3>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div>
                <p className="text-xs text-muted-foreground mb-1">Total CoinJoins</p>
                <p className="text-base font-semibold">{coinJoinStats.count}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">Min Fee</p>
                <p className="text-base font-semibold">{coinJoinStats.minFee.toFixed(8)}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">Max Fee</p>
                <p className="text-base font-semibold">{coinJoinStats.maxFee.toFixed(8)}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">Avg Fee</p>
                <p className="text-base font-semibold">{coinJoinStats.avgFee.toFixed(8)}</p>
              </div>
            </div>
          </>
        ) : (
          <div className="text-center py-2 text-muted-foreground text-sm">
            No CoinJoin transactions yet
          </div>
        )}
      </div>

      {/* Show Details Button */}
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="mt-4 w-full flex items-center justify-center gap-2 py-2 px-4 rounded-lg border border-border hover:bg-background/50 transition-colors text-sm"
      >
        {isExpanded ? (
          <>
            <ChevronUp className="h-4 w-4" />
            Hide Details
          </>
        ) : (
          <>
            <ChevronDown className="h-4 w-4" />
            Show Details
          </>
        )}
      </button>

      {isExpanded && (
        <div className="mt-4 pt-4 border-t border-border/50 animate-fade-in">
          <h4 className="text-sm font-semibold mb-3 text-muted-foreground uppercase tracking-wide">
            Transaction History
          </h4>

        {/* Filter Buttons */}
        <div className="flex items-center gap-2 mb-4 flex-wrap">
          <button
            onClick={() => { setFilterCategory('all'); setVisibleCount(5); }}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              filterCategory === 'all'
                ? 'bg-primary text-white'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            }`}
          >
            All
          </button>
          <button
            onClick={() => { setFilterCategory('send'); setVisibleCount(5); }}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              filterCategory === 'send'
                ? 'bg-primary text-white'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            }`}
          >
            Send
          </button>
          <button
            onClick={() => { setFilterCategory('receive'); setVisibleCount(5); }}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              filterCategory === 'receive'
                ? 'bg-primary text-white'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            }`}
          >
            Receive
          </button>
          <button
            onClick={() => { setFilterCategory('coinjoin'); setVisibleCount(5); }}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              filterCategory === 'coinjoin'
                ? 'bg-primary text-white'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            }`}
          >
            CoinJoin
          </button>
          <button
            onClick={() => { setFilterCategory('ticket'); setVisibleCount(5); }}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              filterCategory === 'ticket'
                ? 'bg-primary text-white'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            }`}
          >
            Tickets
          </button>
          <button
            onClick={() => { setFilterCategory('vote'); setVisibleCount(5); }}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              filterCategory === 'vote'
                ? 'bg-primary text-white'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            }`}
          >
            Votes
          </button>
        </div>

        <div className="flex items-center justify-between mb-3">
          <span className="text-sm text-muted-foreground">
            Showing {displayedTransactions.length} of {filteredTransactions.length} transactions
          </span>
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
                  {tx.account && (
                    <span className="text-xs px-2 py-0.5 rounded bg-primary/10 text-primary font-medium">
                      {tx.account}
                    </span>
                  )}
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
                {formatAmount(tx.amount)}
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

        {showLoadMore && (
          <button
            onClick={handleLoadMore}
            disabled={loadingMore}
            className="mt-4 w-full flex items-center justify-center gap-2 py-2 px-4 rounded-lg border border-border hover:bg-background transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loadingMore ? (
              <>
                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary"></div>
                Loading...
              </>
            ) : (
              <>
                <ChevronDown className="h-4 w-4" />
                Load More
              </>
            )}
          </button>
        )}
        </div>
      )}
    </div>
  );
};

