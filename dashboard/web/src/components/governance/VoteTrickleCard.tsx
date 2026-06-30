// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Clock,
  Loader2,
  ScrollText,
  X,
} from 'lucide-react';
import {
  VoteTrickleEvent,
  VoteTrickleStatus,
  getVoteTrickleWorkers,
  stopVoteTrickle,
  subscribeVoteTrickleEvents,
} from '../../services/api';

const MAX_EVENTS = 200;

// formatCountdown renders a positive second count as "7h 12m" / "12m 03s" / "3s".
const formatCountdown = (secs: number) => {
  const s = Math.max(0, Math.floor(secs));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${sec.toString().padStart(2, '0')}s`;
  return `${sec}s`;
};

// parseTime parses an ISO timestamp, treating an empty value or Go's zero time
// (which serializes to year 1, i.e. <= 0 ms) as "unset" (0).
const parseTime = (s?: string): number => {
  if (!s) return 0;
  const ms = Date.parse(s);
  return Number.isFinite(ms) && ms > 0 ? ms : 0;
};

const levelClass = (level: VoteTrickleEvent['level']) => {
  switch (level) {
    case 'error':
      return 'text-destructive';
    case 'warn':
      return 'text-warning';
    default:
      return 'text-muted-foreground';
  }
};

// VoteTrickleCard surfaces the background vote-trickle workers (politeiavoter
// mode). Several proposals can trickle at once, so this renders one sub-card per
// running (or finished-but-not-dismissed) proposal, each with its own live
// cast/total progress, countdown, and event log. It is pinned at the top of the
// proposals page and polls the worker list + subscribes the shared event stream.
export const VoteTrickleCard = () => {
  const [workers, setWorkers] = useState<VoteTrickleStatus[]>([]);
  const [eventsByToken, setEventsByToken] = useState<Record<string, VoteTrickleEvent[]>>({});
  const [now, setNow] = useState(() => Date.now());

  const refresh = async () => {
    try {
      setWorkers(await getVoteTrickleWorkers());
    } catch {
      /* ignore transient poll errors */
    }
  };

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 5000);
    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

  useEffect(() => {
    const cleanup = subscribeVoteTrickleEvents(
      (ev) => {
        const tok = ev.token || '';
        setEventsByToken((prev) => {
          const arr = [...(prev[tok] ?? []), ev];
          if (arr.length > MAX_EVENTS) arr.splice(0, arr.length - MAX_EVENTS);
          return { ...prev, [tok]: arr };
        });
      },
      (err) => console.error('Vote-trickle events WebSocket error:', err),
    );
    return cleanup;
  }, []);

  const handleStop = async (token: string) => {
    try {
      await stopVoteTrickle(token);
    } catch {
      /* ignore */
    }
    await refresh();
  };

  if (workers.length === 0) return null;

  return (
    <div className="space-y-3">
      {workers.map((w) => (
        <VoteTrickleWorkerCard
          key={w.token ?? ''}
          status={w}
          events={eventsByToken[w.token ?? ''] ?? []}
          now={now}
          onStop={handleStop}
        />
      ))}
    </div>
  );
};

// VoteTrickleBadge is a compact cross-tab indicator of how many proposals are
// currently trickling, shown in the governance header. It links to the proposals
// page where the full per-proposal cards live. Renders nothing when idle.
export const VoteTrickleBadge = () => {
  const [running, setRunning] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const poll = async () => {
      try {
        const ws = await getVoteTrickleWorkers();
        if (!cancelled) setRunning(ws.filter((w) => w.running).length);
      } catch {
        /* ignore */
      }
    };
    poll();
    const id = window.setInterval(poll, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  if (running === 0) return null;

  return (
    <Link
      to="/wallet/governance/proposals"
      title="Vote trickle in progress"
      className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-primary/15 border border-primary/30 text-primary text-xs font-medium hover:bg-primary/20 transition-colors"
    >
      <Clock className="h-3.5 w-3.5 animate-pulse" />
      Trickling {running}
    </Link>
  );
};

interface WorkerCardProps {
  status: VoteTrickleStatus;
  events: VoteTrickleEvent[];
  now: number;
  onStop: (token: string) => void | Promise<void>;
}

// VoteTrickleWorkerCard renders one proposal's trickle run.
const VoteTrickleWorkerCard = ({ status, events, now, onStop }: WorkerCardProps) => {
  const [expanded, setExpanded] = useState(false);
  const [stopping, setStopping] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (expanded && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [events, expanded]);

  const running = status.running;
  const total = status.total || 0;
  const done = status.cast + status.failed;
  const pct = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0;
  const finishMs = parseTime(status.finishAt);
  const nextMs = parseTime(status.nextAt);
  const finishIn = finishMs > 0 ? (finishMs - now) / 1000 : 0;
  const nextIn = nextMs > 0 ? (nextMs - now) / 1000 : 0;

  const handleClick = async () => {
    setStopping(true);
    try {
      await onStop(status.token || '');
    } finally {
      setStopping(false);
    }
  };

  return (
    <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-primary/30 space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <div
            className={`p-2.5 rounded-xl shrink-0 ${
              running
                ? 'bg-primary/15 border border-primary/30'
                : 'bg-muted/10 border border-border/50'
            }`}
          >
            <Clock
              className={`h-5 w-5 ${running ? 'text-primary animate-pulse' : 'text-muted-foreground'}`}
            />
          </div>
          <div className="min-w-0">
            <h3 className="font-semibold truncate">
              {status.proposalName || status.token}
            </h3>
            <p className="text-xs text-muted-foreground truncate">
              {running ? 'Trickling votes' : 'Trickle complete'}
              {status.voteOption ? (
                <>
                  {' '}
                  &middot; voting{' '}
                  <span className="text-foreground font-medium">{status.voteOption}</span>
                </>
              ) : null}
            </p>
          </div>
        </div>
        <button
          type="button"
          onClick={handleClick}
          disabled={stopping}
          className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors disabled:opacity-50 shrink-0 ${
            running
              ? 'bg-destructive/20 hover:bg-destructive/30 text-destructive'
              : 'bg-muted/20 hover:bg-muted/30'
          }`}
        >
          {stopping ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : running ? (
            <X className="h-3.5 w-3.5" />
          ) : null}
          {running ? 'Stop' : 'Dismiss'}
        </button>
      </div>

      {/* Progress bar */}
      <div className="space-y-1.5">
        <div className="h-2 rounded-full bg-muted/20 overflow-hidden">
          <div className="h-full bg-gradient-primary transition-all" style={{ width: `${pct}%` }} />
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
          <div>
            <span className="text-muted-foreground">Cast </span>
            <span className="font-semibold text-foreground">
              {status.cast}/{total}
            </span>
          </div>
          {status.failed > 0 && (
            <div>
              <span className="text-muted-foreground">Failed </span>
              <span className="font-semibold text-destructive">{status.failed}</span>
            </div>
          )}
          {running && status.pending > 0 && (
            <div>
              <span className="text-muted-foreground">Pending </span>
              <span className="font-semibold text-foreground">{status.pending}</span>
            </div>
          )}
          {running && finishMs > 0 && (
            <div>
              <span className="text-muted-foreground">Finishes in </span>
              <span className="font-semibold text-foreground">{formatCountdown(finishIn)}</span>
            </div>
          )}
          {running && nextMs > 0 && nextIn > 0 && (
            <div>
              <span className="text-muted-foreground">Next in </span>
              <span className="font-semibold text-foreground">{formatCountdown(nextIn)}</span>
            </div>
          )}
        </div>
      </div>

      {status.lastError && (
        <div className="flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span className="break-all">{status.lastError}</span>
        </div>
      )}

      {!running && (
        <p className="text-xs text-muted-foreground">
          Cast {status.cast} of {total} vote{total === 1 ? '' : 's'}
          {status.failed > 0 ? `, ${status.failed} failed` : ''}.
        </p>
      )}

      {/* Event log */}
      <div className="rounded-lg border border-border/40 overflow-hidden">
        <div
          onClick={() => setExpanded(!expanded)}
          className="w-full flex items-center justify-between p-2.5 hover:bg-muted/10 transition-colors cursor-pointer"
        >
          <div className="flex items-center gap-2">
            {expanded ? (
              <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
            )}
            <ScrollText className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-xs font-semibold">Activity</span>
          </div>
          <span className="text-xs text-muted-foreground">
            {events.length} event{events.length === 1 ? '' : 's'}
          </span>
        </div>
        {expanded && (
          <div
            ref={scrollRef}
            className="max-h-48 overflow-y-auto px-3 pb-3 border-t border-border/30 space-y-1 font-mono text-xs"
          >
            {events.length === 0 ? (
              <p className="py-3 text-center text-muted-foreground">No events yet.</p>
            ) : (
              events.map((ev, i) => (
                <div key={i} className="flex gap-2">
                  <span className="text-muted-foreground shrink-0">
                    {new Date(ev.timestamp).toLocaleTimeString()}
                  </span>
                  <span className={`shrink-0 uppercase ${levelClass(ev.level)}`}>{ev.level}</span>
                  <span className="text-foreground break-all">{ev.message}</span>
                </div>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  );
};
