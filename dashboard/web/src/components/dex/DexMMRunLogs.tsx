// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { toYMDTime } from '../../utils/date';
import { Link } from 'react-router-dom';
import { Loader2, ScrollText, X } from 'lucide-react';
import {
  getMMRunLogs,
  type DexMarket,
  type MMMarketMakingEvent,
  type MMRunOverview,
} from '../../services/dcrdexApi';
import { useDexRefreshOnNotes } from './DexLiveProvider';
import { convQty, convRate, fmtAmt, fmtPrice, fmtUsd } from './dexFormat';
import { dexCoinExplorer } from './dexExplorers';

type AssetInfo = (assetID: number) => { symbol: string; convFactor: number };

const DCR_ASSET_ID = 42;
const PAGE = 50;
// asset.TransactionType values carried by a DEX order event's transactions.
const TX_TYPE_LABEL: Record<number, string> = { 3: 'Swap', 4: 'Redeem', 5: 'Refund', 6: 'Split' };

// txAssetID attributes a DEX order transaction to a chain, mirroring bisonw's
// mmlogs txAsset: swap/refund/split settle on the asset being sold, redeems on
// the asset being received.
const txAssetID = (txType: number | undefined, sell: boolean, baseID: number, quoteID: number): number | null => {
  switch (txType) {
    case 3:
    case 5:
    case 6:
      return sell ? baseID : quoteID;
    case 4:
      return sell ? quoteID : baseID;
    default:
      return null;
  }
};

const shortId = (id: string) => (id.length > 18 ? `${id.slice(0, 8)}…${id.slice(-6)}` : id);

// Event kinds the run-log feed can be filtered by. A bot event is exactly one of
// a DEX order, a CEX order, a deposit, or a withdrawal; order events split by side.
type EventKind = 'dexBuy' | 'dexSell' | 'cexBuy' | 'cexSell' | 'deposit' | 'withdrawal';
const EVENT_KINDS: { k: EventKind; label: string }[] = [
  { k: 'dexBuy', label: 'DEX Buy' },
  { k: 'dexSell', label: 'DEX Sell' },
  { k: 'cexBuy', label: 'CEX Buy' },
  { k: 'cexSell', label: 'CEX Sell' },
  { k: 'deposit', label: 'Deposit' },
  { k: 'withdrawal', label: 'Withdrawal' },
];
const kindOf = (ev: MMMarketMakingEvent): EventKind | null => {
  if (ev.dexOrderEvent) return ev.dexOrderEvent.sell ? 'dexSell' : 'dexBuy';
  if (ev.cexOrderEvent) return ev.cexOrderEvent.sell ? 'cexSell' : 'cexBuy';
  if (ev.depositEvent) return 'deposit';
  if (ev.withdrawalEvent) return 'withdrawal';
  return null;
};

// hadActivity reports whether an order event actually traded. The run log
// records every order a market maker places, and a bot re-quotes each epoch, so
// the bulk are placed-then-canceled orders that never matched. There is no
// explicit cancel status, so "traded" is inferred: a DEX order with at least one
// settlement transaction, or a CEX order with any fill. Deposits/withdrawals
// always count as activity.
const hadActivity = (ev: MMMarketMakingEvent): boolean => {
  if (ev.dexOrderEvent) return (ev.dexOrderEvent.transactions?.length ?? 0) > 0;
  if (ev.cexOrderEvent) return ev.cexOrderEvent.baseFilled > 0 || ev.cexOrderEvent.quoteFilled > 0;
  return true;
};

// fmtDuration renders an HH:MM:SS span between two stamps that may be in seconds
// or milliseconds (bisonw reports seconds).
const fmtDuration = (start: number, end: number): string => {
  const norm = (t: number) => (t > 1e12 ? t / 1000 : t);
  const s = Math.max(0, Math.floor(norm(end) - norm(start)));
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(Math.floor(s / 3600))}:${pad(Math.floor((s % 3600) / 60))}:${pad(s % 60)}`;
};

// TxLink links a transaction id to its block explorer: Decred to dcrpulse's own
// /explorer/tx route, other assets to their external explorer.
const TxLink = ({ assetID, id }: { assetID: number | null; id: string }) => {
  const cls = 'font-mono text-primary hover:underline break-all';
  if (assetID === DCR_ASSET_ID) {
    return (
      <Link to={`/explorer/tx/${id.split(':')[0]}`} title={id} className={cls}>
        {shortId(id)}
      </Link>
    );
  }
  const url = assetID !== null ? dexCoinExplorer(assetID, id) : null;
  if (!url) {
    return (
      <span className="font-mono break-all text-muted-foreground" title={id}>
        {shortId(id)}
      </span>
    );
  }
  return (
    <a href={url} target="_blank" rel="noopener noreferrer" title={id} className={cls}>
      {shortId(id)}
    </a>
  );
};

const LogRow = ({ ev, market, assetOf }: { ev: MMMarketMakingEvent; market?: DexMarket; assetOf: AssetInfo }) => {
  const time = toYMDTime(new Date(ev.timestamp * 1000));
  const price = (rate: number) =>
    market ? fmtPrice(convRate(rate, market.baseConvFactor, market.quoteConvFactor), market.quote) : String(rate);
  const baseQty = (qty: number) => (market ? fmtAmt(convQty(qty, market.baseConvFactor), 6) : String(qty));

  let label = '';
  let cls = 'text-foreground';
  let body: React.ReactNode = null;

  if (ev.dexOrderEvent) {
    const o = ev.dexOrderEvent;
    label = o.sell ? 'DEX Sell' : 'DEX Buy';
    cls = o.sell ? 'text-destructive' : 'text-success';
    body = (
      <>
        <div>
          {baseQty(o.qty)} {market?.base ?? ''} @ {price(o.rate)}
        </div>
        {o.transactions && o.transactions.length > 0 && (
          <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-[10px]">
            {o.transactions.map((tx, i) => (
              <span key={i} className="text-muted-foreground">
                {TX_TYPE_LABEL[tx.type ?? -1] ?? 'Tx'}:{' '}
                <TxLink assetID={txAssetID(tx.type, o.sell, market?.baseID ?? -1, market?.quoteID ?? -1)} id={tx.id} />
              </span>
            ))}
          </div>
        )}
      </>
    );
  } else if (ev.cexOrderEvent) {
    const o = ev.cexOrderEvent;
    label = o.sell ? 'CEX Sell' : 'CEX Buy';
    cls = o.sell ? 'text-destructive' : 'text-success';
    const bFill = market ? fmtAmt(convQty(o.baseFilled, market.baseConvFactor), 6) : String(o.baseFilled);
    const qFill = market ? fmtAmt(convQty(o.quoteFilled, market.quoteConvFactor), 6) : String(o.quoteFilled);
    body = (
      <>
        <div>
          {baseQty(o.qty)} {market?.base ?? ''} @ {price(o.rate)}
        </div>
        <div className="text-[10px] text-muted-foreground">
          filled {bFill} {market?.base ?? ''} / {qFill} {market?.quote ?? ''}
        </div>
      </>
    );
  } else if (ev.depositEvent) {
    const d = ev.depositEvent;
    const { symbol, convFactor } = assetOf(d.assetID);
    label = 'Deposit';
    cls = 'text-primary';
    body = (
      <>
        <div>
          {fmtAmt((d.transaction?.amount ?? 0) / (convFactor || 1), 6)} {symbol}
          <span className="text-muted-foreground"> &rarr; CEX +{fmtAmt(d.cexCredit / (convFactor || 1), 6)}</span>
        </div>
        {d.transaction && (
          <div className="text-[10px]">
            <TxLink assetID={d.assetID} id={d.transaction.id} />
          </div>
        )}
      </>
    );
  } else if (ev.withdrawalEvent) {
    const wd = ev.withdrawalEvent;
    const { symbol, convFactor } = assetOf(wd.assetID);
    label = 'Withdrawal';
    cls = 'text-primary';
    body = (
      <>
        <div>
          {fmtAmt((wd.transaction?.amount ?? 0) / (convFactor || 1), 6)} {symbol}
          <span className="text-muted-foreground"> &larr; CEX -{fmtAmt(wd.cexDebit / (convFactor || 1), 6)}</span>
        </div>
        {wd.transaction && (
          <div className="text-[10px]">
            <TxLink assetID={wd.assetID} id={wd.transaction.id} />
          </div>
        )}
      </>
    );
  } else {
    return null;
  }

  return (
    <div className="py-2 border-b border-border/20 text-xs">
      <div className="flex items-center gap-2">
        <span className={`font-medium ${cls}`}>{label}</span>
        {ev.pending && <span className="px-1 rounded bg-warning/10 text-warning text-[10px]">pending</span>}
        <span className="ml-auto text-[10px] text-muted-foreground">{time}</span>
      </div>
      <div className="mt-0.5 font-mono tabular-nums">{body}</div>
    </div>
  );
};

// DexMMRunLogs is a modal feed of a market-maker run's event log: its DEX/CEX
// orders, deposits, and withdrawals (with explorer links), newest first. It
// pages older events on demand and refreshes live on each runevent
// notification. It is identified by a run (host/market/startTime) so it serves
// both the currently running bot and an archived past run.
export const DexMMRunLogs = ({
  host,
  baseID,
  quoteID,
  startTime,
  running = false,
  profitFallback,
  market,
  assetOf,
  onClose,
}: {
  host: string;
  baseID: number;
  quoteID: number;
  startTime: number;
  running?: boolean;
  profitFallback?: number;
  market?: DexMarket;
  assetOf: AssetInfo;
  onClose: () => void;
}) => {
  const [eventsById, setEventsById] = useState<Record<number, MMMarketMakingEvent>>({});
  const [overview, setOverview] = useState<MMRunOverview | null>(null);
  const [loading, setLoading] = useState(false);
  const [done, setDone] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(
    async (refID?: number) => {
      if (!startTime) return;
      setLoading(true);
      setErr(null);
      try {
        const res = await getMMRunLogs(host, baseID, quoteID, startTime, PAGE, refID);
        const logs = res?.logs ?? [];
        const updated = res?.updatedLogs ?? [];
        setOverview(res?.overview ?? null);
        setEventsById((prev) => {
          const next = { ...prev };
          for (const e of logs) next[e.id] = e;
          for (const e of updated) next[e.id] = e;
          return next;
        });
        // A paged fetch that returns only the refID echo (or nothing) is the end.
        if (refID !== undefined && logs.length <= 1) setDone(true);
      } catch (e: any) {
        setErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Failed to load run logs');
      } finally {
        setLoading(false);
      }
    },
    [host, baseID, quoteID, startTime],
  );

  useEffect(() => {
    setEventsById({});
    setDone(false);
    load();
  }, [load]);

  // Live: a new event for any bot fires runevent; refetch the newest page and
  // merge by id (cheap, debounced in the hook).
  useDexRefreshOnNotes(['runevent'], () => load());

  const sorted = useMemo(() => Object.values(eventsById).sort((a, b) => b.id - a.id), [eventsById]);
  const oldestId = sorted.length ? sorted[sorted.length - 1].id : undefined;

  // Client-side event-type filter (mirrors bisonw mmlogs). Empty set = show all.
  // Counts and filtering run over the events loaded so far, not the whole run.
  const [active, setActive] = useState<Set<EventKind>>(new Set());
  // Hide placed-then-canceled (unfilled) orders by default - they flood the log.
  const [hideUnfilled, setHideUnfilled] = useState(true);
  const counts = useMemo(() => {
    const c = {} as Record<EventKind, number>;
    for (const ev of sorted) {
      const k = kindOf(ev);
      if (k) c[k] = (c[k] || 0) + 1;
    }
    return c;
  }, [sorted]);
  const hiddenUnfilled = useMemo(() => (hideUnfilled ? sorted.filter((ev) => !hadActivity(ev)).length : 0), [sorted, hideUnfilled]);
  const visible = useMemo(
    () =>
      sorted.filter((ev) => {
        if (hideUnfilled && !hadActivity(ev)) return false;
        if (active.size === 0) return true;
        const k = kindOf(ev);
        return k !== null && active.has(k);
      }),
    [sorted, active, hideUnfilled],
  );
  const toggleKind = (k: EventKind) =>
    setActive((prev) => {
      const next = new Set(prev);
      if (next.has(k)) next.delete(k);
      else next.add(k);
      return next;
    });

  const profit = overview?.profitLoss?.profit ?? profitFallback ?? 0;
  const profitRatio = overview?.profitLoss?.profitRatio ?? 0;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div
        className="w-full max-w-xl max-h-[85vh] flex flex-col rounded-xl border border-border/60 bg-card shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-border/50">
          <h3 className="font-semibold flex items-center gap-2">
            <ScrollText className="h-4 w-4 text-primary" />
            Run logs
            {market && (
              <span className="text-muted-foreground font-normal">
                {market.base}/{market.quote}
              </span>
            )}
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded-lg hover:bg-muted/20 text-muted-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="px-4 py-3 border-b border-border/50 space-y-2">
          <div className="grid grid-cols-3 gap-2">
            <div className="flex flex-col gap-0.5">
              <span className="text-[10px] uppercase tracking-wider text-muted-foreground/60">Profit / loss</span>
              <span className={`text-xs font-mono tabular-nums ${profit >= 0 ? 'text-success' : 'text-destructive'}`}>
                {fmtUsd(profit)} <span className="text-muted-foreground">{(profitRatio * 100).toFixed(2)}%</span>
              </span>
            </div>
            <div className="flex flex-col gap-0.5">
              <span className="text-[10px] uppercase tracking-wider text-muted-foreground/60">Duration</span>
              <span className="text-xs font-mono tabular-nums">
                {startTime ? fmtDuration(startTime, overview?.endTime || (running ? Date.now() / 1000 : sorted[0]?.timestamp || Date.now() / 1000)) : '-'}
              </span>
            </div>
            <div className="flex flex-col gap-0.5">
              <span className="text-[10px] uppercase tracking-wider text-muted-foreground/60">Status</span>
              <span className={`text-xs ${running ? 'text-success' : 'text-muted-foreground'}`}>{running ? 'Running' : 'Stopped'}</span>
            </div>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {EVENT_KINDS.map(({ k, label }) => {
              const n = counts[k] || 0;
              const on = active.has(k);
              return (
                <button
                  key={k}
                  type="button"
                  disabled={n === 0 && !on}
                  onClick={() => toggleKind(k)}
                  className={`px-2 py-0.5 rounded-full text-[10px] border transition-colors disabled:opacity-40 disabled:hover:bg-transparent ${
                    on ? 'bg-primary/15 border-primary/40 text-primary' : 'border-border/60 text-muted-foreground hover:bg-muted/10'
                  }`}
                >
                  {label}
                  {n > 0 && <span className="ml-1 opacity-70">{n}</span>}
                </button>
              );
            })}
            {active.size > 0 && (
              <button type="button" onClick={() => setActive(new Set())} className="px-2 py-0.5 rounded-full text-[10px] text-muted-foreground hover:text-foreground">
                Clear
              </button>
            )}
          </div>
          <label className="flex items-center gap-1.5 text-[11px] text-muted-foreground cursor-pointer select-none">
            <input type="checkbox" checked={hideUnfilled} onChange={(e) => setHideUnfilled(e.target.checked)} className="accent-primary" />
            Hide unfilled (canceled) orders
            {hideUnfilled && hiddenUnfilled > 0 && <span className="text-muted-foreground/50">{hiddenUnfilled} hidden</span>}
          </label>
        </div>

        <div className="overflow-y-auto px-4 grow">
          {err && <div className="py-3 text-xs text-destructive">{err}</div>}
          {sorted.length === 0 && !loading && !err && (
            <div className="py-8 text-center text-xs text-muted-foreground">No events yet.</div>
          )}
          {sorted.length > 0 && visible.length === 0 && (
            <div className="py-8 text-center text-xs text-muted-foreground">No events match the filter.</div>
          )}
          {visible.map((ev) => (
            <LogRow key={ev.id} ev={ev} market={market} assetOf={assetOf} />
          ))}
          {loading && (
            <div className="flex items-center justify-center py-4 text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
            </div>
          )}
          {!done && sorted.length > 0 && oldestId !== undefined && (
            <div className="py-3 text-center">
              <button
                type="button"
                disabled={loading}
                onClick={() => load(oldestId)}
                className="text-xs px-3 py-1.5 rounded-lg border border-border hover:bg-muted/10 disabled:opacity-50"
              >
                Load older
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
