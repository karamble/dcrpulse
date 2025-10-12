// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AccountInfo } from '../services/api';
import { Wallet, Coins, Lock, Clock, AlertCircle, Vote } from 'lucide-react';

interface AccountsListProps {
  accounts: AccountInfo[];
}

export const AccountsList = ({ accounts }: AccountsListProps) => {
  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-2">
          <Wallet className="w-5 h-5 text-primary" />
          <h3 className="text-lg font-semibold">Accounts</h3>
        </div>
        <span className="text-sm text-muted-foreground">{accounts.length} account(s)</span>
      </div>

      {accounts.length === 0 ? (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No accounts found</p>
          <p className="text-sm text-muted-foreground mt-2">
            Import an xpub to create a new account
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {accounts.map((account) => (
            <div
              key={account.accountName}
              className="p-3 rounded-lg bg-background/50 border border-border/30 hover:border-primary/30 transition-colors"
            >
              <div className="flex items-center justify-between mb-2">
                <div>
                  <h4 className="font-medium text-sm text-foreground">{account.accountName}</h4>
                </div>
                <div className="text-right">
                  <p className="text-base font-semibold text-primary">
                    {account.totalBalance.toFixed(2)} DCR
                  </p>
                  <p className="text-xs text-muted-foreground">Total</p>
                </div>
              </div>

              <div className="grid grid-cols-2 md:grid-cols-4 gap-1.5 text-xs">
                <div className="p-1.5 rounded bg-background/30">
                  <div className="flex items-center gap-1 text-muted-foreground mb-0.5">
                    <Coins className="h-3 w-3 text-success" />
                    <p>Spendable</p>
                  </div>
                  <p className="font-medium text-success">{account.spendableBalance.toFixed(2)}</p>
                </div>
                {account.lockedByTickets > 0 && (
                  <div className="p-1.5 rounded bg-background/30">
                    <div className="flex items-center gap-1 text-muted-foreground mb-0.5">
                      <Lock className="h-3 w-3 text-blue-500" />
                      <p>Locked</p>
                    </div>
                    <p className="font-medium text-blue-500">{account.lockedByTickets.toFixed(2)}</p>
                  </div>
                )}
                {account.immatureBalance > 0 && (
                  <div className="p-1.5 rounded bg-background/30">
                    <div className="flex items-center gap-1 text-muted-foreground mb-0.5">
                      <Clock className="h-3 w-3 text-warning" />
                      <p>Immature</p>
                    </div>
                    <p className="font-medium text-warning">{account.immatureBalance.toFixed(2)}</p>
                  </div>
                )}
                {account.unconfirmedBalance > 0 && (
                  <div className="p-1.5 rounded bg-background/30">
                    <div className="flex items-center gap-1 text-muted-foreground mb-0.5">
                      <AlertCircle className="h-3 w-3 text-muted-foreground" />
                      <p>Unconfirmed</p>
                    </div>
                    <p className="font-medium text-muted-foreground">{account.unconfirmedBalance.toFixed(2)}</p>
                  </div>
                )}
                {account.votingAuthority > 0 && (
                  <div className="p-1.5 rounded bg-background/30">
                    <div className="flex items-center gap-1 text-muted-foreground mb-0.5">
                      <Vote className="h-3 w-3 text-purple-500" />
                      <p>Voting Auth</p>
                    </div>
                    <p className="font-medium text-purple-500">{account.votingAuthority.toFixed(2)}</p>
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

