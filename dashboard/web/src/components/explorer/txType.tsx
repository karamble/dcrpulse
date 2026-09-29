// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useNavigate } from 'react-router-dom';
import { ArrowRightLeft, CheckCircle, Coins, Landmark, Shuffle, Ticket, XCircle, type LucideIcon } from 'lucide-react';
import type { TransactionSummary } from '../../services/explorerApi';
import { CopyButton } from './CopyButton';

interface TxTypeMeta {
  Icon: LucideIcon;
  color: string;
  badge: string;
  name: string;
}

const REGULAR: TxTypeMeta = {
  Icon: ArrowRightLeft,
  color: 'text-blue-500',
  badge: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  name: 'Regular Transaction',
};

const TX_TYPES: Record<string, TxTypeMeta> = {
  ticket: { Icon: Ticket, color: 'text-warning', badge: 'bg-warning/10 text-warning border-warning/20', name: 'Ticket Purchase (SSTx)' },
  vote: { Icon: CheckCircle, color: 'text-success', badge: 'bg-success/10 text-success border-success/20', name: 'Vote (SSGen)' },
  revocation: { Icon: XCircle, color: 'text-red-500', badge: 'bg-red-500/10 text-red-500 border-red-500/20', name: 'Revocation (SSRtx)' },
  coinbase: { Icon: Coins, color: 'text-purple-500', badge: 'bg-purple-500/10 text-purple-500 border-purple-500/20', name: 'Coinbase' },
  tspend: { Icon: Landmark, color: 'text-amber-500', badge: 'bg-amber-500/10 text-amber-500 border-amber-500/20', name: 'Treasury Spend (TSpend)' },
  treasurybase: { Icon: Landmark, color: 'text-amber-600', badge: 'bg-amber-600/10 text-amber-600 border-amber-600/20', name: 'Treasury Addition (TBase)' },
  coinjoin: { Icon: Shuffle, color: 'text-purple-500', badge: 'bg-purple-500/10 text-purple-500 border-purple-500/20', name: 'CoinJoin' },
};

// txTypeMeta is the one table of how each explorer transaction type looks;
// anything unknown reads as a regular transaction.
export const txTypeMeta = (type: string): TxTypeMeta => TX_TYPES[type] ?? REGULAR;

export const TxTypeIcon = ({ type, className = 'h-4 w-4' }: { type: string; className?: string }) => {
  const { Icon, color } = txTypeMeta(type);
  return <Icon className={`${className} ${color}`} />;
};

export type TxGroup = 'treasury' | 'coinbase' | 'votes' | 'tickets' | 'revocations' | 'coinjoin' | 'regular';

const GROUP_OF: Record<string, TxGroup> = {
  tspend: 'treasury',
  treasurybase: 'treasury',
  coinbase: 'coinbase',
  vote: 'votes',
  ticket: 'tickets',
  revocation: 'revocations',
  coinjoin: 'coinjoin',
  regular: 'regular',
};

export const groupTxsByType = (txs: TransactionSummary[]): Record<TxGroup, TransactionSummary[]> => {
  const out: Record<TxGroup, TransactionSummary[]> = {
    treasury: [], coinbase: [], votes: [], tickets: [], revocations: [], coinjoin: [], regular: [],
  };
  for (const tx of txs) {
    const g = GROUP_OF[tx.type];
    if (g) out[g].push(tx);
  }
  return out;
};

const SECTIONS: Record<TxGroup, { title: string; type: string }> = {
  treasury: { title: 'Treasury', type: 'tspend' },
  coinbase: { title: 'Coinbase', type: 'coinbase' },
  votes: { title: 'Votes', type: 'vote' },
  tickets: { title: 'Tickets', type: 'ticket' },
  revocations: { title: 'Revocations', type: 'revocation' },
  coinjoin: { title: 'CoinJoin', type: 'coinjoin' },
  regular: { title: 'Regular', type: 'regular' },
};

// TxGroupSections lists a block's or the mempool's transactions under one
// heading per type, in the order the page gives.
export const TxGroupSections = ({ groups, order }: { groups: Record<TxGroup, TransactionSummary[]>; order: TxGroup[] }) => {
  const navigate = useNavigate();
  return (
    <div className="space-y-6">
      {order.map((key) => {
        const txs = groups[key];
        if (txs.length === 0) return null;
        const { title, type } = SECTIONS[key];
        const { Icon, color } = txTypeMeta(type);
        const treasury = key === 'treasury';
        return (
          <div key={key}>
            <h3 className={`text-sm font-medium ${color} mb-3 flex items-center gap-2`}>
              <Icon className="h-4 w-4" />
              {title} ({txs.length})
            </h3>
            <div className="space-y-2">
              {txs.map((tx) => (
                <button
                  key={tx.txid}
                  onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                  className={`w-full p-4 rounded-lg ${
                    treasury
                      ? 'bg-amber-500/5 hover:bg-amber-500/10 border border-amber-500/20'
                      : 'bg-background/50 hover:bg-background/70'
                  } transition-colors text-left`}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 flex-1 min-w-0">
                      <TxTypeIcon type={tx.type} />
                      {treasury ? (
                        <div className="flex flex-col min-w-0 flex-1">
                          <span className="font-mono text-sm truncate">{tx.txid}</span>
                          <span className="text-xs text-muted-foreground">
                            {tx.type === 'tspend' ? 'Treasury Spend' : 'Treasury Addition'}
                          </span>
                        </div>
                      ) : (
                        <span className="font-mono text-sm truncate">{tx.txid}</span>
                      )}
                      <CopyButton text={tx.txid} />
                    </div>
                    <div className="flex items-center gap-4 text-sm">
                      <span className="text-muted-foreground">{tx.size} bytes</span>
                      <span className={`${txTypeMeta(tx.type).color}${treasury ? ' font-semibold' : ''}`}>
                        {tx.totalValue.toFixed(2)} DCR
                      </span>
                    </div>
                  </div>
                </button>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
};
