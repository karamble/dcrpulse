// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { AlertCircle, RefreshCw, ScrollText } from 'lucide-react';
import { LogComponent, getLogs } from '../../services/api';

const components: { value: LogComponent; label: string }[] = [
  { value: 'dcrwallet', label: 'dcrwallet' },
  { value: 'dcrd', label: 'dcrd' },
];

const lineOptions = [200, 500, 1000, 2000];

const levelClass = (line: string) => {
  if (/\[ERR\]|\bERROR\b/i.test(line)) return 'text-destructive';
  if (/\[WRN\]|\bWARN\b/i.test(line)) return 'text-warning';
  if (/\[DBG\]|\bDEBUG\b/i.test(line)) return 'text-muted-foreground/70';
  return 'text-foreground/80';
};

export const LogsSection = () => {
  const [component, setComponent] = useState<LogComponent>('dcrwallet');
  const [lines, setLines] = useState<number>(500);
  const [data, setData] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const r = await getLogs(component, lines);
      setData(r.lines);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to load logs');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [component, lines]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [data]);

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2 flex-1">
          <ScrollText className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Logs</h3>
        </div>

        <select
          value={component}
          onChange={(e) => setComponent(e.target.value as LogComponent)}
          disabled={loading}
          className="px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
        >
          {components.map((c) => (
            <option key={c.value} value={c.value}>
              {c.label}
            </option>
          ))}
        </select>

        <select
          value={lines}
          onChange={(e) => setLines(Number(e.target.value))}
          disabled={loading}
          className="px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
        >
          {lineOptions.map((n) => (
            <option key={n} value={n}>
              last {n} lines
            </option>
          ))}
        </select>

        <button
          onClick={load}
          disabled={loading}
          className="inline-flex items-center gap-2 px-3 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-wait"
        >
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      <p className="text-xs text-muted-foreground">
        Read-only tail of <span className="font-mono">/app-data/{component}/logs/mainnet/{component}.log</span>.
        Logs are written by the {component} container; the dashboard does not interpret them.
      </p>

      {error && (
        <div className="flex items-center gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}

      <div
        ref={scrollRef}
        className="h-[60vh] overflow-y-auto rounded-lg bg-background/50 border border-border/30 p-3 font-mono text-xs leading-relaxed"
      >
        {loading && data.length === 0 ? (
          <p className="text-muted-foreground">Loading…</p>
        ) : data.length === 0 ? (
          <p className="text-muted-foreground">No log lines.</p>
        ) : (
          data.map((line, i) => (
            <div key={i} className={`whitespace-pre-wrap break-words ${levelClass(line)}`}>
              {line}
            </div>
          ))
        )}
      </div>
    </div>
  );
};
