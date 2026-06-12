import { useEffect, useState } from 'react';
import { toYMDTime } from '../../../utils/date';
import { AlertCircle, Loader2, Search } from 'lucide-react';
import {
  LightningNodeInfo,
  LightningQueryRoutesResponse,
  queryLnNode,
  queryLnRoutes,
} from '../../../services/lightningApi';

const atomsPerDcr = 1e8;
const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';
const trunc = (s: string, head = 10, tail = 6) =>
  s.length <= head + tail + 1 ? s : `${s.slice(0, head)}…${s.slice(-tail)}`;
const fmtDate = (sec: number) => (!sec ? '-' : toYMDTime(new Date(sec * 1000)));

const isPubkey = (s: string) => /^[0-9a-fA-F]{66}$/.test(s);

// ---- Query Node ------------------------------------------------------------

const QueryNodePanel = () => {
  const [pubkey, setPubkey] = useState('');
  const [info, setInfo] = useState<LightningNodeInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Auto-query when the input becomes a valid 66-hex pubkey.
  useEffect(() => {
    const v = pubkey.trim();
    if (!isPubkey(v)) {
      setInfo(null);
      setError(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    queryLnNode(v)
      .then((r) => {
        if (!cancelled) {
          setInfo(r);
          setError(null);
        }
      })
      .catch((err: any) => {
        if (cancelled) return;
        setInfo(null);
        const body = err?.response?.data;
        setError(typeof body === 'string' ? body : err?.message || 'Query failed');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [pubkey]);

  return (
    <div className="space-y-3">
      <div className="relative">
        <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
        <input
          type="text"
          value={pubkey}
          onChange={(e) => setPubkey(e.target.value.trim())}
          placeholder="Paste a 66-hex node pubkey to query its graph entry"
          className="w-full pl-8 pr-3 py-1.5 rounded-lg bg-background border border-border text-sm font-mono focus:outline-none focus:border-primary"
        />
      </div>
      {loading && (
        <div className="text-xs text-muted-foreground inline-flex items-center gap-1">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Querying…
        </div>
      )}
      {error && (
        <div className="text-xs text-destructive inline-flex items-center gap-1">
          <AlertCircle className="h-3.5 w-3.5" /> {error}
        </div>
      )}
      {info && (
        <div className="space-y-3">
          <div className="p-3 rounded-lg bg-background/40 border border-border/60 space-y-1 text-sm">
            <div>
              <span className="text-xs uppercase tracking-wide text-muted-foreground mr-2">Alias</span>
              {info.alias || '-'}
            </div>
            <div>
              <span className="text-xs uppercase tracking-wide text-muted-foreground mr-2">Total capacity</span>
              {fmtDcr(info.totalCapacity)}
            </div>
            <div>
              <span className="text-xs uppercase tracking-wide text-muted-foreground mr-2">Last update</span>
              {fmtDate(info.lastUpdate)}
            </div>
            <div className="font-mono text-xs break-all">
              <span className="text-xs uppercase tracking-wide text-muted-foreground mr-2 font-sans">
                Pubkey
              </span>
              {info.pubKey}
            </div>
          </div>
          <div className="text-sm font-medium">Channels ({info.channels.length})</div>
          <div className="space-y-1.5 max-h-96 overflow-y-auto">
            {info.channels.map((c) => (
              <div
                key={c.channelId}
                className="p-2 rounded-md bg-background/40 border border-border/60 text-xs space-y-1"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono">{c.chanPoint}</span>
                  <span className="text-muted-foreground">{fmtDcr(c.capacity)}</span>
                </div>
                <div className="font-mono text-muted-foreground">
                  {trunc(c.node1Pubkey)} — {trunc(c.node2Pubkey)}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};

// ---- Query Routes ----------------------------------------------------------

const QueryRoutesPanel = () => {
  const [pubkey, setPubkey] = useState('');
  const [amount, setAmount] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<LightningQueryRoutesResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  const amtAtoms = (() => {
    const v = amount.trim();
    if (!/^\d*\.?\d{0,8}$/.test(v) || v === '') return 0;
    const dcr = parseFloat(v);
    return Number.isFinite(dcr) ? Math.round(dcr * atomsPerDcr) : 0;
  })();
  const canQuery = isPubkey(pubkey) && amtAtoms > 0 && !busy;

  const onQuery = async () => {
    if (!canQuery) return;
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const r = await queryLnRoutes(pubkey.trim(), amtAtoms);
      setResult(r);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Query failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <input
          type="text"
          value={pubkey}
          onChange={(e) => setPubkey(e.target.value.trim())}
          placeholder="Destination pubkey (66 hex)"
          className="w-full px-3 py-1.5 rounded-lg bg-background border border-border text-sm font-mono focus:outline-none focus:border-primary"
        />
        <div className="flex items-center gap-2">
          <input
            type="text"
            inputMode="decimal"
            value={amount}
            onChange={(e) => {
              const v = e.target.value.trim();
              if (v === '' || /^\d*\.?\d{0,8}$/.test(v)) setAmount(v);
            }}
            placeholder="Amount"
            className="flex-1 px-3 py-1.5 rounded-lg bg-background border border-border text-sm focus:outline-none focus:border-primary"
          />
          <span className="text-sm text-muted-foreground">DCR</span>
        </div>
      </div>
      <div className="flex justify-end">
        <button
          type="button"
          onClick={onQuery}
          disabled={!canQuery}
          className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Search className="h-4 w-4" />}
          {busy ? 'Querying…' : 'Find routes'}
        </button>
      </div>
      {error && (
        <div className="text-xs text-destructive inline-flex items-center gap-1">
          <AlertCircle className="h-3.5 w-3.5" /> {error}
        </div>
      )}
      {result && (
        <div className="space-y-3">
          <div className="text-sm">
            Success probability:{' '}
            <span className="font-semibold">{(result.successProb * 100).toFixed(1)}%</span>
            <span className="text-muted-foreground"> · {result.routes.length} route(s)</span>
          </div>
          {result.routes.length === 0 ? (
            <div className="text-sm text-muted-foreground">No routes found.</div>
          ) : (
            <div className="space-y-2">
              {result.routes.map((route, i) => (
                <div
                  key={i}
                  className="p-3 rounded-lg bg-background/40 border border-border/60 space-y-2"
                >
                  <div className="text-xs flex items-center justify-between">
                    <span className="text-muted-foreground">Route #{i + 1}</span>
                    <span>
                      total {fmtDcr(route.totalAmtAtoms)} · fees {fmtDcr(route.totalFeesAtoms)}
                    </span>
                  </div>
                  <ol className="text-xs space-y-1 list-decimal list-inside text-foreground/80">
                    {route.hops.map((h, j) => (
                      <li key={j} className="font-mono">
                        {trunc(h.pubKey, 8, 6)}{' '}
                        <span className="text-muted-foreground">fee {fmtDcr(h.feeAtoms)}</span>
                      </li>
                    ))}
                  </ol>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

// ---- Section root ----------------------------------------------------------

type Sub = 'node' | 'routes';

export const NetworkSection = () => {
  const [sub, setSub] = useState<Sub>('node');
  return (
    <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/60 space-y-4">
      <div>
        <h3 className="text-lg font-semibold">Network</h3>
        <p className="text-sm text-muted-foreground">
          Inspect the Lightning channel graph: query nodes, find candidate routes to a
          destination.
        </p>
      </div>
      <nav className="flex gap-2 border-b border-border">
        {(['node', 'routes'] as Sub[]).map((s) => (
          <button
            key={s}
            type="button"
            onClick={() => setSub(s)}
            className={`px-3 py-1.5 -mb-px border-b-2 text-sm transition-colors ${
              sub === s
                ? 'border-primary text-primary font-semibold'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            {s === 'node' ? 'Query node' : 'Query routes'}
          </button>
        ))}
      </nav>
      {sub === 'node' ? <QueryNodePanel /> : <QueryRoutesPanel />}
    </div>
  );
};
