// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Activity, ChevronLeft, ArrowRightLeft } from 'lucide-react';
import { getMempoolTransactions, MempoolTransactions } from '../services/explorerApi';
import { TxGroupSections, groupTxsByType } from '../components/explorer/txType';
import { useVisiblePoll } from '../hooks/useVisiblePoll';

export const MempoolView = () => {
  const navigate = useNavigate();
  const [mempool, setMempool] = useState<MempoolTransactions | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const fetchMempool = async () => {
    try {
      const data = await getMempoolTransactions();
      setMempool(data);
      setLoading(false);
      setError('');
    } catch (err) {
      setError('Failed to load mempool transactions');
      setLoading(false);
    }
  };

  // Auto-refresh every 30 seconds.
  useVisiblePoll(fetchMempool, 30000);

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  // Six full filter passes over the mempool; recompute only when it changes,
  // not on every render.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const txGroups = useMemo(() => groupTxsByType(mempool?.transactions ?? []), [mempool]);

  if (loading) {
    return (
      <div className="text-center py-20">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-primary"></div>
        <p className="mt-4 text-muted-foreground">Loading mempool...</p>
      </div>
    );
  }

  if (error || !mempool) {
    return (
      <div className="text-center py-20">
        <Activity className="h-16 w-16 mx-auto text-muted-foreground/50 mb-4" />
        <h2 className="text-2xl font-bold mb-2">Failed to Load Mempool</h2>
        <p className="text-muted-foreground mb-6">{error}</p>
        <button
          onClick={() => navigate('/explorer')}
          className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
        >
          Back to Explorer
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <button
              onClick={() => navigate('/explorer')}
              className="p-2 rounded-lg hover:bg-muted/20 transition-colors"
            >
              <ChevronLeft className="h-6 w-6" />
            </button>
            <div>
              <h1 className="text-3xl font-bold">Mempool</h1>
              <p className="text-sm text-muted-foreground">
                {mempool.count} pending transaction{mempool.count !== 1 ? 's' : ''}
              </p>
            </div>
          </div>
        </div>

        {/* Mempool Summary Card */}
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
          <div className="flex items-center gap-2 mb-6">
            <Activity className="h-5 w-5 text-primary" />
            <h2 className="text-xl font-semibold">Mempool Summary</h2>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {/* Transaction Count */}
            <div className="p-4 rounded-lg bg-background/50">
              <p className="text-sm text-muted-foreground mb-2">Pending Transactions</p>
              <p className="text-2xl font-bold">{mempool.count.toLocaleString()}</p>
            </div>

            {/* Mempool Size */}
            <div className="p-4 rounded-lg bg-background/50">
              <p className="text-sm text-muted-foreground mb-2">Total Size</p>
              <p className="text-2xl font-bold">{formatSize(mempool.size)}</p>
            </div>

            {/* Transaction Types */}
            <div className="p-4 rounded-lg bg-background/50">
              <p className="text-sm text-muted-foreground mb-2">Transaction Breakdown</p>
              <div className="text-sm space-y-1">
                {txGroups.treasury.length > 0 && <p>Treasury: {txGroups.treasury.length}</p>}
                {txGroups.tickets.length > 0 && <p>Tickets: {txGroups.tickets.length}</p>}
                {txGroups.votes.length > 0 && <p>Votes: {txGroups.votes.length}</p>}
                {txGroups.revocations.length > 0 && <p>Revocations: {txGroups.revocations.length}</p>}
                {txGroups.coinjoin.length > 0 && <p>CoinJoin: {txGroups.coinjoin.length}</p>}
                {txGroups.regular.length > 0 && <p>Regular: {txGroups.regular.length}</p>}
              </div>
            </div>
          </div>
        </div>

        {/* Transactions List */}
        {mempool.count === 0 ? (
          <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
            <div className="text-center py-8">
              <Activity className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
              <p className="text-muted-foreground">Mempool is empty</p>
            </div>
          </div>
        ) : (
          <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
            <div className="flex items-center gap-2 mb-6">
              <ArrowRightLeft className="h-5 w-5 text-primary" />
              <h2 className="text-xl font-semibold">Transactions ({mempool.count})</h2>
            </div>

            <TxGroupSections groups={txGroups} order={['treasury', 'tickets', 'votes', 'revocations', 'coinjoin', 'regular']} />
          </div>
        )}

        {/* Auto-refresh notice */}
        <div className="text-center text-sm text-muted-foreground">
          Auto-refreshing every 30 seconds
        </div>
      </div>
  );
};

