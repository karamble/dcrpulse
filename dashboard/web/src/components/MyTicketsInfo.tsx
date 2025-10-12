// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Ticket, Clock, CheckCircle, XCircle, AlertCircle } from 'lucide-react';

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
  const total = ownMempoolTix + immature + unspent;
  const hasTickets = total > 0 || voted > 0 || revoked > 0;

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

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-success/10 border border-success/20">
          <Ticket className="h-6 w-6 text-success" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">My Tickets</h3>
          <p className="text-sm text-muted-foreground">Your staking tickets</p>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {/* Own Mempool */}
        {ownMempoolTix > 0 && (
          <div className="p-4 rounded-lg bg-warning/10 border border-warning/20">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Mempool</span>
              <Clock className="h-4 w-4 text-warning" />
            </div>
            <div className="text-2xl font-bold text-warning">{ownMempoolTix}</div>
            <div className="text-xs text-muted-foreground mt-1">Pending</div>
          </div>
        )}

        {/* Immature */}
        {immature > 0 && (
          <div className="p-4 rounded-lg bg-info/10 border border-info/20">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Immature</span>
              <Clock className="h-4 w-4 text-info" />
            </div>
            <div className="text-2xl font-bold text-info">{immature}</div>
            <div className="text-xs text-muted-foreground mt-1">Maturing</div>
          </div>
        )}

        {/* Live (Unspent) */}
        <div className="p-4 rounded-lg bg-success/10 border border-success/20">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-muted-foreground font-medium">Live</span>
            <CheckCircle className="h-4 w-4 text-success" />
          </div>
          <div className="text-2xl font-bold text-success">{unspent}</div>
          <div className="text-xs text-muted-foreground mt-1">Active</div>
        </div>

        {/* Voted */}
        <div className="p-4 rounded-lg bg-primary/10 border border-primary/20">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-muted-foreground font-medium">Voted</span>
            <CheckCircle className="h-4 w-4 text-primary" />
          </div>
          <div className="text-2xl font-bold text-primary">{voted}</div>
          <div className="text-xs text-muted-foreground mt-1">Successful</div>
        </div>

        {/* Revoked */}
        {revoked > 0 && (
          <div className="p-4 rounded-lg bg-red-500/10 border border-red-500/20">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Revoked</span>
              <XCircle className="h-4 w-4 text-red-500" />
            </div>
            <div className="text-2xl font-bold text-red-500">{revoked}</div>
            <div className="text-xs text-muted-foreground mt-1">Missed</div>
          </div>
        )}

        {/* Expired */}
        {unspentExpired > 0 && (
          <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-muted-foreground font-medium">Expired</span>
              <AlertCircle className="h-4 w-4 text-muted-foreground" />
            </div>
            <div className="text-2xl font-bold">{unspentExpired}</div>
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
    </div>
  );
};

