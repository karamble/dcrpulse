// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, ExternalLink, Filter, Loader2, RefreshCw, ShieldOff } from 'lucide-react';
import { Proposal, getProposals, refreshProposals } from '../../services/api';
import { VoteResultsBar } from './VoteResultsBar';

const statusBuckets = ['all', 'voting', 'pre-vote', 'finished', 'abandoned'] as const;

// Ordering of status buckets in the "All statuses" view: pre-vote first, then
// active votes, then resolved/abandoned proposals.
const bucketOrder: Record<string, number> = {
  'pre-vote': 0,
  voting: 1,
  finished: 2,
  abandoned: 3,
};

const statusBadge = (voteStatus: string) => {
  switch (voteStatus) {
    case 'approved':
      return 'bg-success/15 text-success border border-success/30';
    case 'rejected':
      return 'bg-warning/15 text-warning border border-warning/30';
    case 'abandoned':
      return 'bg-destructive/15 text-destructive border border-destructive/30';
    case 'started':
      return 'bg-info/15 text-info border border-info/30';
    case 'authorized':
    case 'unauthorized':
      return 'bg-primary/15 text-primary border border-primary/30';
    default:
      return 'bg-muted/15 text-muted-foreground border border-border/50';
  }
};

// formatDuration renders a positive second count as "7h 12m" / "12m" / "<1m".
const formatDuration = (secs: number) => {
  const s = Math.max(0, secs);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return '<1m';
};

// formatAgo renders elapsed seconds as "just now" / "12m ago" / "7h 12m ago".
const formatAgo = (secs: number) => {
  const s = Math.max(0, secs);
  if (s < 60) return 'just now';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}h ${m}m ago`;
  return `${m}m ago`;
};

export const ProposalsTab = () => {
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [fetchedAt, setFetchedAt] = useState(0);
  const [refreshAvailableAt, setRefreshAvailableAt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [disabled, setDisabled] = useState(false);
  const [filter, setFilter] = useState<(typeof statusBuckets)[number]>('all');
  // now (unix seconds) drives the live countdown / "updated X ago" display.
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const r = await getProposals();
        if (cancelled) return;
        setProposals(r.proposals);
        setFetchedAt(r.fetchedAt);
        setRefreshAvailableAt(r.refreshAvailableAt);
      } catch (err: any) {
        if (cancelled) return;
        if (err?.response?.status === 503) {
          setDisabled(true);
        } else {
          const body = err?.response?.data;
          setError(typeof body === 'string' ? body : err?.message || 'Failed to load proposals');
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Tick the clock once a minute so the countdown and age stay current.
  useEffect(() => {
    const id = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 60 * 1000);
    return () => clearInterval(id);
  }, []);

  const cooldownRemaining = refreshAvailableAt - now;
  const canRefresh = !refreshing && cooldownRemaining <= 0;

  const handleRefresh = async () => {
    setRefreshing(true);
    setRefreshError(null);
    try {
      const r = await refreshProposals();
      setProposals(r.proposals);
      setFetchedAt(r.fetchedAt);
      setRefreshAvailableAt(r.refreshAvailableAt);
    } catch (err: any) {
      const status = err?.response?.status;
      const data = err?.response?.data;
      if (status === 429 && data && typeof data === 'object') {
        // Cooling down: re-sync timestamps so the countdown matches the server.
        if (Array.isArray(data.proposals)) setProposals(data.proposals);
        if (data.fetchedAt) setFetchedAt(data.fetchedAt);
        if (data.refreshAvailableAt) setRefreshAvailableAt(data.refreshAvailableAt);
      } else if (status === 503) {
        setDisabled(true);
      } else {
        setRefreshError(typeof data === 'string' ? data : err?.message || 'Refresh failed');
      }
    } finally {
      setRefreshing(false);
      setNow(Math.floor(Date.now() / 1000));
    }
  };

  const filtered = useMemo(() => {
    if (filter !== 'all') return proposals.filter((p) => p.status === filter);
    // Group by status bucket (pre-vote, then voting, then the rest), keeping the
    // backend's within-bucket order (vote-end block, descending).
    return proposals
      .map((p, i) => ({ p, i }))
      .sort((a, b) => {
        const oa = bucketOrder[a.p.status] ?? 99;
        const ob = bucketOrder[b.p.status] ?? 99;
        return oa - ob || a.i - b.i;
      })
      .map((x) => x.p);
  }, [proposals, filter]);

  if (disabled) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-warning/30 bg-warning/5 space-y-3">
        <div className="flex items-center gap-2">
          <ShieldOff className="h-5 w-5 text-warning" />
          <h3 className="font-semibold">Politeia is disabled in Settings</h3>
        </div>
        <p className="text-sm text-muted-foreground">
          Enable the Politeia external-request toggle to fetch off-chain proposals from
          proposals.decred.org.
        </p>
        <Link
          to="/wallet/settings/privacy"
          className="inline-block text-sm text-primary hover:underline"
        >
          Open Privacy &amp; Security settings
        </Link>
      </div>
    );
  }

  if (loading && proposals.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading proposals...
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
        <AlertCircle className="h-4 w-4" />
        {error}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Filter className="h-4 w-4 text-muted-foreground" />
        <select
          value={filter}
          onChange={(e) => setFilter(e.target.value as (typeof statusBuckets)[number])}
          className="px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
        >
          {statusBuckets.map((b) => (
            <option key={b} value={b}>
              {b === 'all' ? 'All statuses' : b}
            </option>
          ))}
        </select>
        <span className="text-xs text-muted-foreground">
          {filtered.length} of {proposals.length} proposals
        </span>
        <div className="ml-auto flex items-center gap-3">
          {fetchedAt > 0 && (
            <span className="text-xs text-muted-foreground">updated {formatAgo(now - fetchedAt)}</span>
          )}
          <button
            type="button"
            onClick={handleRefresh}
            disabled={!canRefresh}
            title={
              canRefresh
                ? 'Refresh proposals from Politeia'
                : `Next refresh available in ${formatDuration(cooldownRemaining)}`
            }
            className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm border border-border/50 bg-background hover:bg-muted/10 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
            {refreshing
              ? 'Refreshing...'
              : cooldownRemaining > 0
                ? `Refresh in ${formatDuration(cooldownRemaining)}`
                : 'Refresh'}
          </button>
        </div>
      </div>

      {refreshError && (
        <div className="text-xs text-destructive flex items-center gap-1">
          <AlertCircle className="h-3 w-3" />
          {refreshError}
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
          No proposals match the current filter.
        </div>
      ) : (
        <div className="space-y-3">
          {filtered.map((p) => (
            <Link
              key={p.token}
              to={`/wallet/governance/proposals/${p.token}`}
              className="block p-4 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 hover:bg-muted/10 transition-colors"
            >
              <div className="flex items-start justify-between gap-3 mb-2">
                <div className="flex-1 min-w-0">
                  <h3 className="font-semibold truncate">{p.name || p.token}</h3>
                  <p className="text-xs text-muted-foreground">
                    by {p.username || 'unknown'} &middot; token{' '}
                    <span className="font-mono">{p.token.slice(0, 8)}...</span>
                  </p>
                </div>
                <div className="flex flex-col items-end gap-1 shrink-0">
                  <span
                    className={`px-2 py-0.5 rounded text-xs font-medium ${statusBadge(p.voteStatus)}`}
                  >
                    {p.voteStatus}
                  </span>
                  {p.blocksLeft > 0 && (
                    <span className="text-xs text-muted-foreground">
                      {p.blocksLeft.toLocaleString()} blocks left
                    </span>
                  )}
                </div>
              </div>
              {p.eligibleTickets > 0 && (
                <VoteResultsBar
                  yes={p.voteCounts.yes ?? 0}
                  no={p.voteCounts.no ?? 0}
                  abstain={p.voteCounts.abstain ?? 0}
                  eligibleTickets={p.eligibleTickets}
                  quorumMin={p.quorumMin}
                />
              )}
              <div className="flex items-center gap-3 text-xs mt-2">
                {p.currentChoice && (
                  <span className="text-success">you voted: {p.currentChoice}</span>
                )}
                <span className="ml-auto inline-flex items-center gap-1 text-primary">
                  open <ExternalLink className="h-3 w-3" />
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
};
