// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Activity, Database, Ticket, CheckCircle, XCircle, ArrowRightLeft, Shuffle } from 'lucide-react';
import { MempoolInfo } from '../services/api';

interface MempoolActivityProps {
  data?: MempoolInfo;
}

export const MempoolActivity = ({ data }: MempoolActivityProps) => {
  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  const hasStakingTxs = data && (data.tickets > 0 || data.votes > 0 || data.revocations > 0);
  const hasAnyActivity = data && (hasStakingTxs || data.regularTxs > 0 || data.coinJoinTxs > 0);

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Activity className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Mempool Activity</h3>
          <p className="text-sm text-muted-foreground">Current pending transactions</p>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        {/* Transaction Count */}
        <div className="p-4 rounded-lg bg-gradient-to-br from-primary/5 to-primary/10 border border-primary/20">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">Pending Transactions</span>
            <Activity className="h-4 w-4 text-primary" />
          </div>
          <div className="text-2xl font-bold">
            {data !== undefined ? data.txCount.toLocaleString() : 'Loading...'}
          </div>
          <div className="text-xs text-muted-foreground mt-1">Unconfirmed txs</div>
        </div>

        {/* Mempool Size */}
        <div className="p-4 rounded-lg bg-gradient-to-br from-blue-500/5 to-blue-500/10 border border-blue-500/20">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground font-medium">Mempool Size</span>
            <Database className="h-4 w-4 text-blue-500" />
          </div>
          <div className="text-2xl font-bold">
            {data !== undefined ? formatBytes(data.bytes) : 'Loading...'}
          </div>
          <div className="text-xs text-muted-foreground mt-1">Total size</div>
        </div>
      </div>

      {/* Transaction Breakdown */}
      {data && hasAnyActivity && (
        <div className="space-y-3">
          {/* Staking Activity */}
          {hasStakingTxs && (
            <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
              <div className="text-sm font-medium text-muted-foreground mb-3">Staking Activity</div>
              <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
                {/* Tickets */}
                {data.tickets > 0 && (
                  <div className="flex items-center gap-2">
                    <Ticket className="h-4 w-4 text-warning" />
                    <div>
                      <div className="text-lg font-bold">{data.tickets}</div>
                      <div className="text-xs text-muted-foreground">Tickets</div>
                    </div>
                  </div>
                )}

                {/* Votes */}
                {data.votes > 0 && (
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-4 w-4 text-success" />
                    <div>
                      <div className="text-lg font-bold">{data.votes}</div>
                      <div className="text-xs text-muted-foreground">Votes</div>
                    </div>
                  </div>
                )}

                {/* Revocations */}
                {data.revocations > 0 && (
                  <div className="flex items-center gap-2">
                    <XCircle className="h-4 w-4 text-red-500" />
                    <div>
                      <div className="text-lg font-bold">{data.revocations}</div>
                      <div className="text-xs text-muted-foreground">Revocations</div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Regular Transactions */}
          {(data.regularTxs > 0 || data.coinJoinTxs > 0) && (
            <div className="p-4 rounded-lg bg-muted/10 border border-border/50">
              <div className="text-sm font-medium text-muted-foreground mb-3">Regular Transactions</div>
              <div className="grid grid-cols-2 gap-3">
                {/* Regular */}
                {data.regularTxs > 0 && (
                  <div className="flex items-center gap-2">
                    <ArrowRightLeft className="h-4 w-4 text-blue-500" />
                    <div>
                      <div className="text-lg font-bold">{data.regularTxs}</div>
                      <div className="text-xs text-muted-foreground">Regular</div>
                    </div>
                  </div>
                )}

                {/* CoinJoin */}
                {data.coinJoinTxs > 0 && (
                  <div className="flex items-center gap-2">
                    <Shuffle className="h-4 w-4 text-purple-500" />
                    <div>
                      <div className="text-lg font-bold">{data.coinJoinTxs}</div>
                      <div className="text-xs text-muted-foreground">CoinJoin</div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

