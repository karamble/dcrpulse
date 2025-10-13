// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link } from 'react-router-dom';
import { Ticket, Check, Clock, AlertCircle } from 'lucide-react';
import { WalletTransaction } from '../services/api';
import { 
  getTicketStatus, 
  getTicketStatusColor, 
  formatMaturityCountdown,
  calculateMaturityProgress,
  calculateTicketMaturity
} from '../services/ticketService';

interface TicketDetailRowProps {
  transaction: WalletTransaction;
}

export const TicketDetailRow = ({ transaction }: TicketDetailRowProps) => {
  const status = getTicketStatus(transaction);
  const statusColor = getTicketStatusColor(transaction);
  const isVote = transaction.txType === 'vote';
  const isTicket = transaction.txType === 'ticket';
  const isRevocation = transaction.txType === 'revocation';
  
  // Calculate maturity for ticket purchases
  const ticketMaturity = calculateTicketMaturity(transaction);
  
  const truncateTxid = (txid: string) => {
    return `${txid.substring(0, 8)}...${txid.substring(txid.length - 8)}`;
  };

  const getStatusIcon = () => {
    if (isVote && transaction.isTicketMature) {
      return <Check className="h-4 w-4 text-success" />;
    }
    if (isVote && !transaction.isTicketMature) {
      return <Clock className="h-4 w-4 text-warning" />;
    }
    if (isTicket && ticketMaturity.isImmature) {
      return <Clock className="h-4 w-4 text-info" />;
    }
    if (isRevocation) {
      return <AlertCircle className="h-4 w-4 text-destructive" />;
    }
    return <Ticket className="h-4 w-4 text-primary" />;
  };

  const maturityProgress = calculateMaturityProgress(transaction.blocksUntilSpendable);

  return (
    <Link
      to={`/explorer/tx/${transaction.txid}`}
      className="flex items-center justify-between p-4 rounded-lg bg-background/50 hover:bg-background transition-colors border border-border/30 hover:border-primary/30 cursor-pointer"
    >
      <div className="flex items-center gap-4 flex-1 min-w-0">
        {/* Status Icon */}
        <div className="flex-shrink-0">
          {getStatusIcon()}
        </div>
        
        {/* Transaction Details */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className={`font-medium ${statusColor}`}>{status}</span>
            {transaction.confirmations === 0 && (
              <span className="text-xs px-2 py-0.5 rounded bg-warning/10 text-warning">
                Pending
              </span>
            )}
          </div>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <code className="font-mono text-xs">{truncateTxid(transaction.txid)}</code>
            {transaction.blockHeight && (
              <>
                <span>•</span>
                <span>Block {transaction.blockHeight.toLocaleString()}</span>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Maturity Info for Votes */}
      {isVote && (
        <div className="ml-4 text-right flex flex-col items-end">
          <div className={`text-sm font-medium ${transaction.isTicketMature ? 'text-success' : 'text-warning'}`}>
            {formatMaturityCountdown(transaction.blocksUntilSpendable)}
          </div>
          {!transaction.isTicketMature && transaction.blocksUntilSpendable !== undefined && (
            <div className="mt-1 w-32">
              <div className="h-1.5 bg-gray-300 dark:bg-gray-700 rounded-full overflow-hidden">
                <div 
                  className="h-full bg-warning transition-all duration-300"
                  style={{ width: `${maturityProgress}%` }}
                />
              </div>
              <div className="text-xs text-muted-foreground mt-1 text-right">
                {Math.round(maturityProgress)}% mature
              </div>
            </div>
          )}
        </div>
      )}

      {/* Amount and Maturity for Tickets */}
      {isTicket && (
        <div className="ml-4 text-right flex flex-col items-end">
          <div className={`text-lg font-semibold ${statusColor}`}>
            {Math.abs(transaction.amount).toFixed(2)} DCR
          </div>
          {ticketMaturity.isImmature && ticketMaturity.blocksUntilMature > 0 && (
            <div className="mt-1 w-32">
              <div className="h-1.5 bg-gray-300 dark:bg-gray-700 rounded-full overflow-hidden">
                <div 
                  className="h-full bg-blue-500 transition-all duration-300"
                  style={{ width: `${ticketMaturity.progress}%` }}
                />
              </div>
              <div className="text-xs text-muted-foreground mt-1 text-right">
                {ticketMaturity.blocksUntilMature} blocks to live
              </div>
            </div>
          )}
        </div>
      )}

      {/* Amount for Revocations */}
      {isRevocation && (
        <div className="text-right ml-4">
          <div className={`text-lg font-semibold ${statusColor}`}>
            {Math.abs(transaction.amount).toFixed(2)} DCR
          </div>
        </div>
      )}
    </Link>
  );
};

