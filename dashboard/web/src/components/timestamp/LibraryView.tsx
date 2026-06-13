// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { AlertCircle, Download, RefreshCw } from 'lucide-react';
import { listTimestamps, refreshTimestamps, exportUrl, type TimestampRecord } from '../../services/timestampApi';
import { StatusBadge } from './StatusBadge';
import { fromUnix, shortHash } from './util';
import { toYMDTime } from '../../utils/date';

interface Props {
  onOpen: (digest: string) => void;
  reloadKey?: number;
}

export const LibraryView = ({ onOpen, reloadKey }: Props) => {
  const [records, setRecords] = useState<TimestampRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [q, setQ] = useState('');
  const [status, setStatus] = useState('');
  const [sort, setSort] = useState('newest');
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async () => {
    setError(null);
    try {
      const data = await listTimestamps({ q: q.trim() || undefined, status: status || undefined, sort });
      setRecords(data);
    } catch (e: any) {
      setError((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'failed to load archive');
    } finally {
      setLoading(false);
    }
  }, [q, status, sort]);

  useEffect(() => {
    const t = setTimeout(load, 250);
    return () => clearTimeout(t);
  }, [load, reloadKey]);

  const refresh = async () => {
    setRefreshing(true);
    try {
      await refreshTimestamps(sort);
      await load();
    } catch {
      /* surfaced by load() on next call */
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row gap-2 sm:items-center">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search filename, title, description…"
          className="flex-1 px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60"
        />
        <select
          value={status}
          onChange={(e) => setStatus(e.target.value)}
          className="px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60"
        >
          <option value="">All statuses</option>
          <option value="anchored">Anchored</option>
          <option value="pending">Confirming</option>
          <option value="awaiting">Awaiting anchor</option>
          <option value="submitted">Submitted</option>
          <option value="failed">Failed</option>
        </select>
        <select
          value={sort}
          onChange={(e) => setSort(e.target.value)}
          className="px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60"
        >
          <option value="newest">Newest</option>
          <option value="oldest">Oldest</option>
          <option value="title">Title</option>
        </select>
        <button
          onClick={refresh}
          disabled={refreshing}
          className="inline-flex items-center justify-center gap-2 px-3 py-2 rounded-lg border border-border/60 text-sm hover:bg-muted/20 disabled:opacity-50"
          title="Check dcrtime for newly anchored proofs"
        >
          <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
          <span className="hidden sm:inline">Refresh</span>
        </button>
        <a
          href={exportUrl()}
          className="inline-flex items-center justify-center gap-2 px-3 py-2 rounded-lg border border-border/60 text-sm hover:bg-muted/20"
          title="Download the whole archive as JSON"
        >
          <Download className="h-4 w-4" />
          <span className="hidden sm:inline">Export</span>
        </a>
      </div>

      {error && (
        <div className="p-4 rounded-xl bg-destructive/10 border border-destructive/30 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {loading ? (
        <div className="p-12 rounded-xl bg-gradient-card border border-border/50 text-center text-muted-foreground">
          Loading archive…
        </div>
      ) : records.length === 0 ? (
        <div className="p-12 rounded-xl bg-gradient-card border border-border/50 text-center text-muted-foreground">
          No timestamps yet. Use the Stamp tab to add your first one.
        </div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-border/50">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-muted-foreground border-b border-border/40 bg-muted/10">
                <th className="py-2.5 px-3 font-medium">File</th>
                <th className="py-2.5 px-3 font-medium">Status</th>
                <th className="py-2.5 px-3 font-medium">Submitted</th>
                <th className="py-2.5 px-3 font-medium">Anchored</th>
              </tr>
            </thead>
            <tbody>
              {records.map((r) => {
                const anchored = fromUnix(r.anchorTime);
                return (
                  <tr
                    key={r.digest}
                    onClick={() => onOpen(r.digest)}
                    className="border-b border-border/20 last:border-0 hover:bg-muted/10 cursor-pointer"
                  >
                    <td className="py-2.5 px-3">
                      <div className="font-medium truncate max-w-[16rem]">{r.title || r.filename}</div>
                      <div className="text-xs text-muted-foreground font-mono">{shortHash(r.digest)}</div>
                    </td>
                    <td className="py-2.5 px-3">
                      <StatusBadge status={r.status} />
                    </td>
                    <td className="py-2.5 px-3 text-muted-foreground whitespace-nowrap">
                      {r.submittedAt ? toYMDTime(new Date(r.submittedAt)) : '-'}
                    </td>
                    <td className="py-2.5 px-3 text-muted-foreground whitespace-nowrap">
                      {anchored ? toYMDTime(anchored) : '-'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};
