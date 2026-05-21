// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, ArrowDownUp, ExternalLink, Filter, History, Search } from 'lucide-react';
import { TicketRecord, TicketLifecycleStatus, listTickets } from '../../services/api';

const TERMINAL_STATES: TicketLifecycleStatus[] = ['VOTED', 'MISSED', 'EXPIRED', 'REVOKED'];

const truncateHash = (h: string) => (h.length > 16 ? `${h.slice(0, 8)}…${h.slice(-8)}` : h);
const formatDcr = (v: number) => v.toFixed(8);
const formatAge = (unixSec: number) => {
  if (!unixSec) return '-';
  const seconds = Math.floor(Date.now() / 1000 - unixSec);
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
};

const statusBadge = (status: TicketLifecycleStatus) => {
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
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${styles[status]}`}>{status}</span>
  );
};

const BATCH = 50;

export const TicketHistoryTab = () => {
  const [tickets, setTickets] = useState<TicketRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<'ALL' | TicketLifecycleStatus>('ALL');
  const [sort, setSort] = useState<'NEWEST' | 'OLDEST'>('NEWEST');
  const [query, setQuery] = useState('');
  const [shown, setShown] = useState(BATCH);

  const load = async () => {
    try {
      const list = await listTickets();
      setTickets(list);
      setError(null);
    } catch (err: any) {
      setError(err?.message || 'Failed to load tickets');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    const id = window.setInterval(load, 30000);
    return () => window.clearInterval(id);
  }, []);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return tickets
      .filter((t) => TERMINAL_STATES.includes(t.status))
      .filter((t) => (statusFilter === 'ALL' ? true : t.status === statusFilter))
      .filter((t) => {
        if (!q) return true;
        return t.hash.toLowerCase().includes(q) || t.spenderHash.toLowerCase().includes(q);
      })
      .sort((a, b) => {
        const ah = a.spenderHeight || a.blockHeight;
        const bh = b.spenderHeight || b.blockHeight;
        return sort === 'NEWEST' ? bh - ah : ah - bh;
      });
  }, [tickets, statusFilter, sort, query]);

  const visible = filtered.slice(0, shown);

  return (
    <div className="space-y-4">
      <div className="p-4 rounded-xl bg-gradient-card border border-border/50 flex flex-wrap gap-3 items-end">
        <div className="flex items-center gap-2">
          <History className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Ticket History</h3>
        </div>
        <div className="flex-1" />
        <div>
          <label className="text-xs text-muted-foreground flex items-center gap-1 mb-1">
            <Filter className="h-3 w-3" /> Status
          </label>
          <select
            value={statusFilter}
            onChange={(e) => {
              setStatusFilter(e.target.value as any);
              setShown(BATCH);
            }}
            className="px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
          >
            <option value="ALL">All</option>
            {TERMINAL_STATES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="text-xs text-muted-foreground flex items-center gap-1 mb-1">
            <ArrowDownUp className="h-3 w-3" /> Sort
          </label>
          <select
            value={sort}
            onChange={(e) => setSort(e.target.value as any)}
            className="px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
          >
            <option value="NEWEST">Newest</option>
            <option value="OLDEST">Oldest</option>
          </select>
        </div>
        <div className="flex-1 min-w-[200px]">
          <label className="text-xs text-muted-foreground flex items-center gap-1 mb-1">
            <Search className="h-3 w-3" /> Search hash
          </label>
          <input
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setShown(BATCH);
            }}
            placeholder="Ticket or vote tx hash"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-sm font-mono"
          />
        </div>
      </div>

      {loading && tickets.length === 0 && (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
          Loading tickets…
        </div>
      )}

      {error && (
        <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}

      {!loading && filtered.length === 0 && !error && (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
          No tickets match the current filter.
        </div>
      )}

      {visible.length > 0 && (
        <div className="p-4 rounded-xl bg-gradient-card border border-border/50 overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-muted-foreground border-b border-border/30">
                <th className="py-2 pr-3">Status</th>
                <th className="py-2 pr-3">Ticket</th>
                <th className="py-2 pr-3">Vote / Revoke</th>
                <th className="py-2 pr-3 text-right">Price</th>
                <th className="py-2 pr-3 text-right">Reward</th>
                <th className="py-2 pr-3">Resolved</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((t) => (
                <tr key={t.hash} className="border-b border-border/20">
                  <td className="py-2 pr-3">{statusBadge(t.status)}</td>
                  <td className="py-2 pr-3 font-mono">
                    <Link
                      to={`/explorer/tx/${t.hash}`}
                      className="text-primary hover:underline inline-flex items-center gap-1"
                    >
                      {truncateHash(t.hash)}
                      <ExternalLink className="h-3 w-3" />
                    </Link>
                  </td>
                  <td className="py-2 pr-3 font-mono">
                    {t.spenderHash ? (
                      <Link
                        to={`/explorer/tx/${t.spenderHash}`}
                        className="text-primary hover:underline inline-flex items-center gap-1"
                      >
                        {truncateHash(t.spenderHash)}
                        <ExternalLink className="h-3 w-3" />
                      </Link>
                    ) : (
                      <span className="text-muted-foreground">-</span>
                    )}
                  </td>
                  <td className="py-2 pr-3 text-right font-mono whitespace-nowrap">
                    {formatDcr(t.ticketPrice)} DCR
                  </td>
                  <td className="py-2 pr-3 text-right font-mono whitespace-nowrap">
                    {t.status === 'VOTED' ? (
                      <span className="text-success">+{formatDcr(t.reward)} DCR</span>
                    ) : (
                      <span className="text-muted-foreground">-</span>
                    )}
                  </td>
                  <td className="py-2 pr-3 text-muted-foreground whitespace-nowrap">
                    {t.spenderHeight > 0 ? (
                      <>
                        {t.spenderHeight.toLocaleString()} · {formatAge(t.spenderTime)}
                      </>
                    ) : (
                      '-'
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {filtered.length > shown && (
        <div className="flex justify-center">
          <button
            onClick={() => setShown((n) => n + BATCH)}
            className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm font-medium transition-colors"
          >
            Load more ({filtered.length - shown} remaining)
          </button>
        </div>
      )}
    </div>
  );
};
