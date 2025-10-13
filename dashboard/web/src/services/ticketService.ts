// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { WalletTransaction } from './api';

export interface TicketWithVote {
  ticket: WalletTransaction;
  vote?: WalletTransaction;
}

export interface TicketStats {
  tickets: WalletTransaction[];
  votes: WalletTransaction[];
  matureVotes: WalletTransaction[];
  immatureVotes: WalletTransaction[];
}

/**
 * Filter transactions to get only ticket purchases
 */
export const filterTickets = (transactions: WalletTransaction[]): WalletTransaction[] => {
  return transactions.filter(tx => tx.txType === 'ticket');
};

/**
 * Filter transactions to get only ticket votes
 */
export const filterVotes = (transactions: WalletTransaction[]): WalletTransaction[] => {
  return transactions.filter(tx => tx.txType === 'vote');
};

/**
 * Filter transactions to get only ticket revocations
 */
export const filterRevocations = (transactions: WalletTransaction[]): WalletTransaction[] => {
  return transactions.filter(tx => tx.txType === 'revocation');
};

/**
 * Group tickets and votes, separating mature and immature votes
 */
export const organizeTickets = (transactions: WalletTransaction[]): TicketStats => {
  const tickets = filterTickets(transactions);
  const votes = filterVotes(transactions);
  
  const matureVotes = votes.filter(vote => vote.isTicketMature === true);
  const immatureVotes = votes.filter(vote => vote.isTicketMature === false);

  return {
    tickets,
    votes,
    matureVotes,
    immatureVotes,
  };
};

/**
 * Group transactions by txid to avoid duplicates from multiple outputs
 * (e.g., ticket purchases have stakesubmission, sstxcommitment, stakechange outputs)
 * Prioritizes keeping the transaction with the largest amount (the actual ticket/vote value)
 */
export const groupByTxid = (transactions: WalletTransaction[]): WalletTransaction[] => {
  const grouped = new Map<string, WalletTransaction>();
  
  for (const tx of transactions) {
    const existing = grouped.get(tx.txid);
    
    if (!existing) {
      // First time seeing this txid
      grouped.set(tx.txid, tx);
    } else {
      // If this transaction has a larger amount, replace the existing one
      // This ensures we keep the stakesubmission output (actual ticket value) for tickets
      // and the actual vote/revocation amount for votes/revocations
      if (Math.abs(tx.amount) > Math.abs(existing.amount)) {
        grouped.set(tx.txid, tx);
      }
    }
  }
  
  return Array.from(grouped.values());
};

/**
 * Sort transactions by block height (most recent first)
 */
export const sortByBlockHeight = (transactions: WalletTransaction[]): WalletTransaction[] => {
  return [...transactions].sort((a, b) => {
    const heightA = a.blockHeight || 0;
    const heightB = b.blockHeight || 0;
    return heightB - heightA; // Descending order
  });
};

/**
 * Get ticket status label
 */
export const getTicketStatus = (ticket: WalletTransaction): string => {
  if (ticket.txType === 'vote') {
    if (ticket.isTicketMature) {
      return 'Voted (Spendable)';
    } else {
      return 'Voted (Maturing)';
    }
  }
  
  if (ticket.txType === 'ticket') {
    if (ticket.confirmations === 0) {
      return 'Pending';
    } else if (ticket.confirmations < 256) {
      return 'Immature';
    } else {
      return 'Live';
    }
  }

  if (ticket.txType === 'revocation') {
    return 'Revoked';
  }

  return 'Unknown';
};

/**
 * Filter tickets by status
 */
export const filterTicketsByStatus = (tickets: WalletTransaction[], status: 'all' | 'live' | 'voted' | 'purchased'): WalletTransaction[] => {
  if (status === 'all') {
    return tickets;
  }
  
  if (status === 'voted') {
    return tickets.filter(tx => tx.txType === 'vote');
  }
  
  if (status === 'purchased') {
    // Purchased includes immature and live tickets
    return tickets.filter(tx => tx.txType === 'ticket');
  }
  
  if (status === 'live') {
    // Live tickets are those with 256+ confirmations
    return tickets.filter(tx => tx.txType === 'ticket' && tx.confirmations >= 256);
  }
  
  return tickets;
};

/**
 * Calculate ticket maturity for purchases (256 block maturity period)
 */
export const calculateTicketMaturity = (ticket: WalletTransaction): { isImmature: boolean; blocksUntilMature: number; progress: number } => {
  if (ticket.txType !== 'ticket') {
    return { isImmature: false, blocksUntilMature: 0, progress: 100 };
  }
  
  const confirmations = ticket.confirmations || 0;
  const blocksUntilMature = Math.max(0, 256 - confirmations);
  const isImmature = confirmations < 256;
  const progress = (confirmations / 256) * 100;
  
  return {
    isImmature,
    blocksUntilMature,
    progress: Math.min(100, progress),
  };
};

/**
 * Get ticket status color class
 */
export const getTicketStatusColor = (ticket: WalletTransaction): string => {
  const status = getTicketStatus(ticket);
  
  if (status === 'Voted (Spendable)') return 'text-success';
  if (status === 'Voted (Maturing)') return 'text-warning';
  if (status === 'Pending') return 'text-yellow-500';
  if (status === 'Immature') return 'text-info';
  if (status === 'Live') return 'text-primary';
  if (status === 'Revoked') return 'text-destructive';
  
  return 'text-muted-foreground';
};

/**
 * Format maturity countdown text
 */
export const formatMaturityCountdown = (blocksUntilSpendable?: number): string => {
  if (!blocksUntilSpendable || blocksUntilSpendable === 0) {
    return 'Funds Spendable';
  }
  
  if (blocksUntilSpendable === 1) {
    return '1 block until spendable';
  }
  
  return `${blocksUntilSpendable} blocks until spendable`;
};

/**
 * Calculate maturity progress (0-100)
 */
export const calculateMaturityProgress = (blocksUntilSpendable?: number): number => {
  if (!blocksUntilSpendable || blocksUntilSpendable === 0) {
    return 100;
  }
  
  const blocksPassed = 256 - blocksUntilSpendable;
  return (blocksPassed / 256) * 100;
};

