// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Wallet, TrendingUp, Clock, AlertCircle, Lock } from 'lucide-react';

interface AccountInfoProps {
  accountName: string;
  totalBalance: number;
  spendableBalance: number;
  immatureBalance: number;
  unconfirmedBalance: number;
  lockedByTickets?: number;
  cumulativeTotal?: number;
  totalSpendable?: number;
  totalLockedByTickets?: number;
}

export const AccountInfo = ({
  accountName,
  totalBalance,
  spendableBalance,
  immatureBalance,
  unconfirmedBalance,
  lockedByTickets = 0,
  cumulativeTotal,
  totalSpendable,
  totalLockedByTickets
}: AccountInfoProps) => {
  const formatDCR = (amount: number) => {
    return amount.toLocaleString('en-US', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 8
    });
  };

  const formatDCRWithDecimals = (amount: number) => {
    const formatted = amount.toFixed(8);
    const [integerPart, decimalPart] = formatted.split('.');
    const mainDecimals = decimalPart.substring(0, 2);
    const extraDecimals = decimalPart.substring(2);
    
    return {
      integer: parseInt(integerPart).toLocaleString('en-US'),
      mainDecimals,
      extraDecimals
    };
  };

  const formatDCR2Decimals = (amount: number) => {
    return amount.toLocaleString('en-US', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    });
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
          <Wallet className="h-5 w-5 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Account Balance</h3>
          <p className="text-sm text-muted-foreground">{accountName}</p>
        </div>
      </div>

      {/* Total Balance - Use cumulativeTotal if available */}
      <div className="mb-6 p-4 rounded-lg bg-primary/5 border border-primary/10">
        <p className="text-sm text-muted-foreground mb-2">Cumulative Total</p>
        <p className="text-4xl font-bold text-primary">
          {(() => {
            const parts = formatDCRWithDecimals(cumulativeTotal || totalBalance);
            return (
              <>
                {parts.integer}.{parts.mainDecimals}
                <span className="text-xl opacity-60">{parts.extraDecimals}</span>
                <span className="text-2xl"> DCR</span>
              </>
            );
          })()}
        </p>
      </div>

      {/* Balance Breakdown */}
      <div className="space-y-4">
        <div className="flex items-center justify-between p-3 rounded-lg bg-success/5 border border-success/10">
          <div className="flex items-center gap-2">
            <TrendingUp className="h-4 w-4 text-success" />
            <span className="text-sm font-medium">Total Spendable</span>
          </div>
          <span className="font-semibold text-success">{formatDCR2Decimals(totalSpendable || spendableBalance)} DCR</span>
        </div>

        {(totalLockedByTickets || lockedByTickets) > 0 && (
          <div className="flex items-center justify-between p-3 rounded-lg bg-blue-500/5 border border-blue-500/10">
            <div className="flex items-center gap-2">
              <Lock className="h-4 w-4 text-blue-500" />
              <span className="text-sm font-medium">Locked by Tickets</span>
            </div>
            <span className="font-semibold text-blue-500">{formatDCR2Decimals(totalLockedByTickets || lockedByTickets)} DCR</span>
          </div>
        )}

        {immatureBalance > 0 && (
          <div className="flex items-center justify-between p-3 rounded-lg bg-warning/5 border border-warning/10">
            <div className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-warning" />
              <span className="text-sm font-medium">Immature</span>
            </div>
            <span className="font-semibold text-warning">{formatDCR(immatureBalance)} DCR</span>
          </div>
        )}

        {unconfirmedBalance > 0 && (
          <div className="flex items-center justify-between p-3 rounded-lg bg-muted/5 border border-muted/10">
            <div className="flex items-center gap-2">
              <AlertCircle className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm font-medium">Unconfirmed</span>
            </div>
            <span className="font-semibold text-muted-foreground">{formatDCR(unconfirmedBalance)} DCR</span>
          </div>
        )}
      </div>

      <div className="mt-4 p-3 rounded-lg bg-muted/5 border border-muted/10">
        <p className="text-xs text-muted-foreground">
          <strong>Watch-Only Mode:</strong> This wallet can monitor balances and transactions but cannot spend funds.
        </p>
      </div>
    </div>
  );
};

