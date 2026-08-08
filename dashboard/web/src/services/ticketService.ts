// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { TicketLifecycleStatus, WalletTransaction } from './api';

// Vote-reward maturity for a VOTED ticket, sourced from the vote transaction
// (listtransactions). Lets the overview show "Voted (Maturing)/(Spendable)" on
// top of the authoritative ticket status.
export interface VoteMaturity {
  isTicketMature?: boolean;
  blocksUntilSpendable?: number;
}

// Badge styling per lifecycle status, shared by the overview ticket history and
// the staking history tab so both read one mapping.
export const ticketStatusBadgeClass = (status: TicketLifecycleStatus): string => {
  const styles: Record<TicketLifecycleStatus, string> = {
    UNMINED: 'bg-muted/20 text-muted-foreground',
    IMMATURE: 'bg-warning/10 text-warning',
    LIVE: 'bg-success/10 text-success',
    VOTED: 'bg-success/10 text-success',
    MISSED: 'bg-destructive/10 text-destructive',
    EXPIRED: 'bg-destructive/10 text-destructive',
    REVOKED: 'bg-destructive/10 text-destructive',
    UNKNOWN: 'bg-muted/20 text-muted-foreground',
  };
  return styles[status] ?? styles.UNKNOWN;
};

// Display label for a ticket keyed on its authoritative lifecycle status. A
// voted ticket also reflects whether its returned funds have matured.
export const ticketRecordStatusLabel = (
  status: TicketLifecycleStatus,
  voteMaturity?: VoteMaturity,
): string => {
  switch (status) {
    case 'LIVE':
      return 'Live';
    case 'IMMATURE':
      return 'Immature';
    case 'UNMINED':
      return 'Pending';
    case 'MISSED':
      return 'Missed';
    case 'EXPIRED':
      return 'Expired';
    case 'REVOKED':
      return 'Revoked';
    case 'VOTED':
      if (voteMaturity?.isTicketMature === true) return 'Voted (Spendable)';
      if (voteMaturity?.isTicketMature === false) return 'Voted (Maturing)';
      return 'Voted';
    default:
      return 'Unknown';
  }
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
 * Calculate maturity progress (0-100)
 */
export const calculateMaturityProgress = (blocksUntilSpendable?: number): number => {
  if (!blocksUntilSpendable || blocksUntilSpendable === 0) {
    return 100;
  }
  
  const blocksPassed = 256 - blocksUntilSpendable;
  return (blocksPassed / 256) * 100;
};
