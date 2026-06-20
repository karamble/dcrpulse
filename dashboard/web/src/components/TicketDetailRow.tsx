// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link } from 'react-router-dom';
import { Ticket, Check, Clock, AlertCircle } from 'lucide-react';
import { TicketRecord } from '../services/api';
import {
  VoteMaturity,
  ticketRecordStatusLabel,
  ticketStatusBadgeClass,
} from '../services/ticketService';
import { MaturityBar } from './MaturityBar';

interface TicketDetailRowProps {
  ticket: TicketRecord;
  // Vote-reward maturity for a VOTED ticket, keyed off its spender (vote) tx.
  voteMaturity?: VoteMaturity;
}

export const TicketDetailRow = ({ ticket, voteMaturity }: TicketDetailRowProps) => {
  const label = ticketRecordStatusLabel(ticket.status, voteMaturity);
  const badgeClass = ticketStatusBadgeClass(ticket.status);

  const truncateHash = (hash: string) =>
    hash.length > 16 ? `${hash.substring(0, 8)}...${hash.substring(hash.length - 8)}` : hash;

  // A voted ticket whose returned funds are still maturing.
  const voteMaturing =
    ticket.status === 'VOTED' &&
    voteMaturity?.isTicketMature === false &&
    voteMaturity.blocksUntilSpendable !== undefined &&
    voteMaturity.blocksUntilSpendable > 0;

  const getStatusIcon = () => {
    switch (ticket.status) {
      case 'VOTED':
        return voteMaturing ? (
          <Clock className="h-4 w-4 text-warning" />
        ) : (
          <Check className="h-4 w-4 text-success" />
        );
      case 'LIVE':
        return <Ticket className="h-4 w-4 text-success" />;
      case 'IMMATURE':
      case 'UNMINED':
        return <Clock className="h-4 w-4 text-info" />;
      case 'MISSED':
      case 'EXPIRED':
      case 'REVOKED':
        return <AlertCircle className="h-4 w-4 text-destructive" />;
      default:
        return <Ticket className="h-4 w-4 text-primary" />;
    }
  };

  return (
    <Link
      to={`/explorer/tx/${ticket.hash}`}
      className="flex items-center justify-between p-4 rounded-lg bg-background/50 hover:bg-background transition-colors border border-border/30 hover:border-primary/30 cursor-pointer"
    >
      <div className="flex items-center gap-4 flex-1 min-w-0">
        {/* Status Icon */}
        <div className="flex-shrink-0">{getStatusIcon()}</div>

        {/* Ticket Details */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className={`px-2 py-0.5 rounded text-xs font-medium ${badgeClass}`}>{label}</span>
          </div>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <code className="font-mono text-xs">{truncateHash(ticket.hash)}</code>
            {ticket.blockHeight > 0 && (
              <>
                <span>•</span>
                <span>Block {ticket.blockHeight.toLocaleString()}</span>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Amount + maturity */}
      <div className="ml-4 text-right flex flex-col items-end">
        <div className="text-lg font-semibold">{ticket.ticketPrice.toFixed(2)} DCR</div>
        {ticket.status === 'VOTED' && ticket.reward > 0 && (
          <div className="text-xs text-success">+{ticket.reward.toFixed(4)} DCR reward</div>
        )}
        {ticket.status === 'IMMATURE' && ticket.blocksUntilMature > 0 && (
          <MaturityBar
            blocksRemaining={ticket.blocksUntilMature}
            pendingSuffix="to live"
            className="mt-1 w-32 text-right"
          />
        )}
        {voteMaturing && (
          <MaturityBar
            blocksRemaining={voteMaturity?.blocksUntilSpendable}
            className="mt-1 w-32 text-right"
          />
        )}
      </div>
    </Link>
  );
};
