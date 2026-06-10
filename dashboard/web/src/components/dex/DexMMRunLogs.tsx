// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Loader2, ScrollText, X } from 'lucide-react';
import {
  getMMRunLogs,
  type DexMarket,
  type MMBotStatus,
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
  const time = new Date(ev.timestamp * 1000).toLocaleString();
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

// DexMMRunLogs is a modal feed of a running bot's event log: its DEX/CEX orders,
// deposits, and withdrawals (with explorer links), newest first. It pages older
// events on demand and refreshes live on each runevent notification.
export const DexMMRunLogs = ({
  bot,
  market,
  assetOf,
  onClose,
}: {
  bot: MMBotStatus;
  market?: DexMarket;
  assetOf: AssetInfo;
  onClose: () => void;
}) => {
  const { host, baseID, quoteID } = bot.config;
  const startTime = bot.runStats?.startTime ?? 0;
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
          <div className="flex items-center gap-3">
            {overview?.profitLoss && (
              <span
                className={`text-xs font-mono tabular-nums ${
                  overview.profitLoss.profit >= 0 ? 'text-success' : 'text-destructive'
                }`}
              >
                {fmtUsd(overview.profitLoss.profit)}
              </span>
            )}
            <button
              type="button"
              onClick={onClose}
              className="p-1 rounded-lg hover:bg-muted/20 text-muted-foreground"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        <div className="overflow-y-auto px-4 grow">
          {err && <div className="py-3 text-xs text-destructive">{err}</div>}
          {sorted.length === 0 && !loading && !err && (
            <div className="py-8 text-center text-xs text-muted-foreground">No events yet.</div>
          )}
          {sorted.map((ev) => (
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
