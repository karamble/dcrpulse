// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Ticket, Clock, CheckCircle, XCircle, AlertCircle, ChevronDown, ChevronUp } from 'lucide-react';
import { getWalletTransactions, WalletTransaction } from '../services/api';
import { sortByBlockHeight, filterTickets, filterVotes, groupByTxid, filterTicketsByStatus } from '../services/ticketService';
import { TicketDetailRow } from './TicketDetailRow';

interface MyTicketsInfoProps {
  ownMempoolTix: number;
  immature: number;
  unspent: number;
  voted: number;
  revoked: number;
  unspentExpired: number;
  totalSubsidy: number;
}

export const MyTicketsInfo = ({ 
  ownMempoolTix,
  immature,
  unspent,
  voted,
  revoked,
  unspentExpired,
  totalSubsidy
}: MyTicketsInfoProps) => {
  // Props from getstakeinfo (may be 0 for xpub wallets)
  const total = ownMempoolTix + immature + unspent;
  const hasTicketsFromRPC = total > 0 || voted > 0 || revoked > 0;
  
  const [isExpanded, setIsExpanded] = useState(false);
  const [transactions, setTransactions] = useState<WalletTransaction[]>([]);
  const [loading, setLoading] = useState(true); // Start with loading state
  const [error, setError] = useState<string | null>(null);
  const [visibleTicketCount, setVisibleTicketCount] = useState(5);
  const [filterStatus, setFilterStatus] = useState<'all' | 'live' | 'voted' | 'purchased'>('all');
  const loadMoreCount = 10;

  // Always fetch transactions on mount to build stats for xpub wallets
  useEffect(() => {
    fetchTicketTransactions();
  }, []);

  const fetchTicketTransactions = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getWalletTransactions(200); // Fetch more transactions for tickets
      setTransactions(data.transactions);
    } catch (err) {
      console.error('Error fetching ticket transactions:', err);
      setError('Failed to load ticket details');
    } finally {
      setLoading(false);
    }
  };

  const tickets = filterTickets(transactions);
  const votes = filterVotes(transactions);
  const revocations = transactions.filter(tx => tx.txType === 'revocation');
  
  // Group by txid to avoid duplicates from multiple outputs (stakesubmission, sstxcommitment, stakechange)
  const uniqueTickets = groupByTxid(tickets);
  const uniqueVotes = groupByTxid(votes);
  const uniqueRevocations = groupByTxid(revocations);
  
  const allTicketsUnfiltered = sortByBlockHeight([...uniqueTickets, ...uniqueVotes, ...uniqueRevocations]);
  
  // Apply status filter
  const allTickets = filterTicketsByStatus(allTicketsUnfiltered, filterStatus);
  
  // Calculate stats from transactions (for xpub wallets where getstakeinfo doesn't work)
  // Use unique counts to avoid counting multiple outputs from same transaction
  const ticketCount = uniqueTickets.length;
  const voteCount = uniqueVotes.length;
  const revocationCount = uniqueRevocations.length;
  const hasTicketsFromTransactions = ticketCount > 0 || voteCount > 0 || revocationCount > 0;
  
  // Use RPC stats if available, otherwise use transaction-based stats
  const hasTickets = hasTicketsFromRPC || hasTicketsFromTransactions;
  
  // Pagination for ticket details
  const displayedTickets = allTickets.slice(0, visibleTicketCount);
  const hasMoreTickets = allTickets.length > visibleTicketCount;
  const remainingTicketsCount = allTickets.length - visibleTicketCount;
  
  const toggleExpanded = () => {
    setIsExpanded(!isExpanded);
  };

  // Show loading state while fetching initial transactions
  if (loading && transactions.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-center gap-3 mb-4">
          <div className="p-3 rounded-xl bg-muted/10 border border-border/50">
            <Ticket className="h-6 w-6 text-muted-foreground" />
          </div>
          <div>
            <h3 className="text-lg font-semibold">My Tickets</h3>
            <p className="text-sm text-muted-foreground">Your staking tickets</p>
          </div>
        </div>
        <div className="flex items-center justify-center py-8">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          <span className="ml-3 text-muted-foreground">Loading tickets...</span>
        </div>
      </div>
    );
  }

  if (!hasTickets) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-center gap-3 mb-4">
          <div className="p-3 rounded-xl bg-muted/10 border border-border/50">
            <Ticket className="h-6 w-6 text-muted-foreground" />
          </div>
          <div>
            <h3 className="text-lg font-semibold">My Tickets</h3>
            <p className="text-sm text-muted-foreground">Your staking tickets</p>
          </div>
        </div>
        <div className="text-center py-8">
          <Ticket className="h-12 w-12 mx-auto text-muted-foreground/50 mb-3" />
          <p className="text-muted-foreground">No tickets found</p>
          <p className="text-sm text-muted-foreground/70 mt-1">Connect to an external wallet via RPC to see stats</p>
          <p className="text-xs text-muted-foreground/60 mt-3 max-w-md mx-auto">
            Tickets cannot be detected on watch-only wallets with imported x-pub keys. 
            A full wallet is required to track staking activity.
          </p>
        </div>
      </div>
    );
  }

  // Determine which stats to display
  // For xpub wallets, use transaction-based counts; for full wallets, use RPC stats
  const displayTickets = hasTicketsFromRPC ? ownMempoolTix : 0;
  const displayImmature = hasTicketsFromRPC ? immature : 0;
  const displayLive = hasTicketsFromRPC ? unspent : ticketCount;
  const displayVoted = hasTicketsFromRPC ? voted : voteCount;
  const displayRevoked = hasTicketsFromRPC ? revoked : revocationCount;
  const displayExpired = hasTicketsFromRPC ? unspentExpired : 0;

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-success/10 border border-success/20">
          <Ticket className="h-6 w-6 text-success" />
        </div>
        <div className="flex-1">
          <h3 className="text-lg font-semibold">My Tickets</h3>
          <p className="text-sm text-muted-foreground">
            {hasTicketsFromRPC ? 'Your staking tickets' : 'From transaction history'}
          </p>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {/* Own Mempool */}
        {displayTickets > 0 && (
          <div className="p-4 rounded-lg bg-warning/10 border border-warning/20">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Mempool</span>
              <Clock className="h-4 w-4 text-warning" />
            </div>
            <div className="text-2xl font-bold text-warning">{displayTickets}</div>
            <div className="text-xs text-muted-foreground mt-1">Pending</div>
          </div>
        )}

        {/* Immature */}
        {displayImmature > 0 && (
          <div className="p-4 rounded-lg bg-info/10 border border-info/20">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Immature</span>
              <Clock className="h-4 w-4 text-info" />
            </div>
            <div className="text-2xl font-bold text-info">{displayImmature}</div>
            <div className="text-xs text-muted-foreground mt-1">Maturing</div>
          </div>
        )}

        {/* Purchased Tickets / Live */}
        {displayLive > 0 && (
          <div className="p-4 rounded-lg bg-success/10 border border-success/20">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">
                {hasTicketsFromRPC ? 'Live' : 'Purchased'}
              </span>
              <Ticket className="h-4 w-4 text-success" />
            </div>
            <div className="text-2xl font-bold text-success">{displayLive}</div>
            <div className="text-xs text-muted-foreground mt-1">
              {hasTicketsFromRPC ? 'Active' : 'Tickets'}
            </div>
          </div>
        )}

        {/* Voted */}
        {displayVoted > 0 && (
          <div className="p-4 rounded-lg bg-primary/10 border border-primary/20">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Voted</span>
              <CheckCircle className="h-4 w-4 text-primary" />
            </div>
            <div className="text-2xl font-bold text-primary">{displayVoted}</div>
            <div className="text-xs text-muted-foreground mt-1">Successful</div>
          </div>
        )}

        {/* Revoked */}
        {displayRevoked > 0 && (
          <div className="p-4 rounded-lg bg-red-500/10 border border-red-500/20">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Revoked</span>
              <XCircle className="h-4 w-4 text-red-500" />
            </div>
            <div className="text-2xl font-bold text-red-500">{displayRevoked}</div>
            <div className="text-xs text-muted-foreground mt-1">Missed</div>
          </div>
        )}

        {/* Expired */}
        {displayExpired > 0 && (
          <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Expired</span>
              <AlertCircle className="h-4 w-4 text-muted-foreground" />
            </div>
            <div className="text-2xl font-bold">{displayExpired}</div>
            <div className="text-xs text-muted-foreground mt-1">Unrevoked</div>
          </div>
        )}
      </div>

      {/* Total Rewards */}
      {totalSubsidy > 0 && (
        <div className="mt-4 p-4 rounded-lg bg-gradient-to-r from-success/10 to-primary/10 border border-success/20">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-muted-foreground mb-1">Total Staking Rewards</div>
              <div className="text-2xl font-bold text-success">{totalSubsidy.toFixed(8)} DCR</div>
            </div>
            <CheckCircle className="h-8 w-8 text-success" />
          </div>
        </div>
      )}

      {/* Expand/Collapse Button */}
      {hasTickets && (
        <button
          onClick={toggleExpanded}
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
      )}

      {/* Detailed Ticket List */}
      {isExpanded && (
        <div className="mt-4 pt-4 border-t border-border/50 animate-fade-in">
          <h4 className="text-sm font-semibold mb-3 text-muted-foreground uppercase tracking-wide">
            Ticket History
          </h4>
          
          {loading && (
            <div className="flex items-center justify-center py-8">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
              <span className="ml-3 text-muted-foreground">Loading ticket details...</span>
            </div>
          )}

          {error && (
            <div className="text-center py-8 text-red-500">
              <p>{error}</p>
            </div>
          )}

          {!loading && !error && allTickets.length === 0 && (
            <div className="text-center py-8 text-muted-foreground">
              <p>No ticket transactions found</p>
            </div>
          )}

          {!loading && !error && allTicketsUnfiltered.length > 0 && (
            <>
              {/* Filter Buttons */}
              <div className="flex items-center gap-2 mb-4 flex-wrap">
                <button
                  onClick={() => { setFilterStatus('all'); setVisibleTicketCount(5); }}
                  className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                    filterStatus === 'all'
                      ? 'bg-primary text-white'
                      : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                  }`}
                >
                  All ({allTicketsUnfiltered.length})
                </button>
                <button
                  onClick={() => { setFilterStatus('purchased'); setVisibleTicketCount(5); }}
                  className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                    filterStatus === 'purchased'
                      ? 'bg-primary text-white'
                      : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                  }`}
                >
                  Purchased ({uniqueTickets.length})
                </button>
                <button
                  onClick={() => { setFilterStatus('live'); setVisibleTicketCount(5); }}
                  className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                    filterStatus === 'live'
                      ? 'bg-primary text-white'
                      : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                  }`}
                >
                  Live ({uniqueTickets.filter(t => t.confirmations >= 256).length})
                </button>
                <button
                  onClick={() => { setFilterStatus('voted'); setVisibleTicketCount(5); }}
                  className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
                    filterStatus === 'voted'
                      ? 'bg-primary text-white'
                      : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                  }`}
                >
                  Voted ({uniqueVotes.length})
                </button>
              </div>

              <div className="flex items-center justify-between mb-3">
                <span className="text-sm text-muted-foreground">
                  Showing {displayedTickets.length} of {allTickets.length} tickets
                </span>
              </div>
              
              <div className="space-y-2">
                {displayedTickets.map((tx, index) => (
                  <TicketDetailRow key={`${tx.txid}-${index}`} transaction={tx} />
                ))}
              </div>

              {hasMoreTickets && (
                <button
                  onClick={() => setVisibleTicketCount(prev => prev + loadMoreCount)}
                  className="mt-4 w-full flex items-center justify-center gap-2 py-2 px-4 rounded-lg border border-border hover:bg-background transition-colors text-sm"
                >
                  <ChevronDown className="h-4 w-4" />
                  Load {Math.min(loadMoreCount, remainingTicketsCount)} More
                  {remainingTicketsCount > loadMoreCount && ` (${remainingTicketsCount} remaining)`}
                </button>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
};

