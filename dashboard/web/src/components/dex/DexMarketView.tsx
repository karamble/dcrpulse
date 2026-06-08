// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, FlaskConical } from 'lucide-react';
import { getDexConfig, getDexMyOrders, orderHasActiveMatches, type DexConfig, type DexMarket, type DexOrder } from '../../services/dcrdexApi';
import { useDexFeed, statsFromCandles, spotToStats, type MarketStats, type MarketSpot } from './useDexFeed';
import { useDexConn, useDexRefreshOnNotes, useDexSpots, useMMBotRun, useSeedDexSpots } from './DexLiveProvider';
import { loadDexConfigCache, saveDexConfigCache } from './dexConfigCache';
import { DexMMActivity } from './DexMMActivity';
import { DexStatsBar } from './DexStatsBar';
import { DexMarketsPanel } from './DexMarketsPanel';
import { DexChartPanel } from './DexChartPanel';
import { DexOrderBook } from './DexOrderBook';
import { DexOrdersPanel } from './DexOrdersPanel';
import { DexUserOrdersPanel } from './DexUserOrdersPanel';
import { DexOrderForm } from './DexOrderForm';
import { useDexCancel } from './DexCancelOrder';
import { mockMarkets, mockBook, mockStats, mockCandles, mockOrders } from './dexMockData';

const HOST = 'dex.decred.org:7232';
const EMPTY_BOOK = { buys: [], sells: [], recentMatches: [] };

// Default to the DCR/BTC market (asset ids 42/0), falling back to the first
// market the server lists.
const defaultMarket = (ms: DexMarket[] | undefined) =>
  ms?.find((m) => m.baseID === 42 && m.quoteID === 0) || ms?.[0] || null;

// DexMarketView is the trading terminal: a market stats bar, a markets sidebar,
// a price chart, a live order book (with depth visualization) and recent
// trades, an order-entry form, and an open-orders panel. In preview mode it
// renders sample data (no server) so the UI can be developed without a
// reachable DEX server; the live candle and 24h-stats feeds are not wired yet.
export const DexMarketView = ({ preview = false }: { preview?: boolean }) => {
  // The last good market list is cached per host so the view renders even
  // while the DEX server can not be connected; the live fetch refreshes it.
  const cachedRef = useRef(preview ? null : loadDexConfigCache(HOST));
  const cached = cachedRef.current;
  const [markets, setMarkets] = useState<DexMarket[]>(preview ? mockMarkets : cached?.markets ?? []);
  const [sel, setSel] = useState<DexMarket | null>(preview ? mockMarkets[0] : defaultMarket(cached?.markets));
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [liveOrders, setLiveOrders] = useState<DexOrder[]>([]);
  const [durs, setDurs] = useState<string[]>(cached?.candleDurs ?? []);
  const [dur, setDur] = useState(
    cached?.candleDurs?.length && !cached.candleDurs.includes('1h') ? cached.candleDurs[0] : '1h',
  );
  // Clicking a book level prefills the order form; seq lets a repeat click on
  // the same level re-apply.
  const pickSeq = useRef(0);
  const [pick, setPick] = useState<{ rate: number; qty: number; sell: boolean; seq: number } | null>(null);
  const onPick = (p: { rate: number; qty: number; sell: boolean }) => setPick({ ...p, seq: ++pickSeq.current });
  const seedSpots = useSeedDexSpots();
  const conn = useDexConn(HOST);
  const marketsRef = useRef(markets);
  marketsRef.current = markets;

  const fetchConfig = useCallback(() => {
    getDexConfig(HOST)
      .then((c: DexConfig) => {
        if (!c.markets?.length) return;
        setMarkets(c.markets);
        setSel((prev) => prev || defaultMarket(c.markets));
        if (c.candleDurs?.length) {
          setDurs(c.candleDurs);
          setDur((d) => (c.candleDurs.includes(d) ? d : c.candleDurs[0]));
        }
        // Seed the shared spots map so the markets list shows last/24h for every
        // market immediately, before the first live `spots` update arrives.
        const snap: Record<string, MarketSpot> = {};
        c.markets.forEach((m) => {
          if (m.spot) snap[`${m.baseID}-${m.quoteID}`] = { ...m.spot, baseID: m.baseID, quoteID: m.quoteID };
        });
        seedSpots(snap);
        saveDexConfigCache(HOST, { markets: c.markets, candleDurs: c.candleDurs ?? [] });
        setLoadErr(null);
      })
      .catch((e: any) => {
        // With a cached market list on screen the failure is already conveyed
        // by the server banner; the error screen is for the nothing-to-render
        // case only.
        if (marketsRef.current.length) return;
        setLoadErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Failed to load markets');
      });
  }, [seedSpots]);

  useEffect(() => {
    if (preview) return;
    fetchConfig();
  }, [preview, fetchConfig]);

  // Refetch when the server connection comes back so a view opened while the
  // server was down picks up the live market list without a reload.
  const prevConnStatus = useRef<number | null>(null);
  useEffect(() => {
    const status = conn?.status ?? null;
    const prev = prevConnStatus.current;
    prevConnStatus.current = status;
    if (!preview && status === 1 && prev !== null && prev !== 1) fetchConfig();
  }, [conn?.status, preview, fetchConfig]);

  const refreshOrders = () => {
    if (preview) return;
    getDexMyOrders(HOST)
      .then(setLiveOrders)
      .catch(() => {});
  };
  // Poll fast while any order is still settling so the panels' swap-status bars
  // and statuses advance on their own; fall back to a slow idle backstop when
  // nothing is active. Live order/match notes still trigger an immediate refresh.
  const settling = !preview && liveOrders.some(orderHasActiveMatches);
  useEffect(() => {
    if (preview) return;
    refreshOrders();
    const id = window.setInterval(refreshOrders, settling ? 10000 : 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preview, settling]);
  useDexRefreshOnNotes(['order', 'match'], refreshOrders);

  // dcrdex-style cancel: a confirmation modal gated on isCancellable, shared by
  // the per-market and all-markets order panels. Refetches orders on success.
  const { requestCancel, modal: cancelModal } = useDexCancel(refreshOrders);

  const marketRef = sel
    ? { host: HOST, base: sel.baseID, quote: sel.quoteID, baseConvFactor: sel.baseConvFactor, quoteConvFactor: sel.quoteConvFactor }
    : null;
  const live = useDexFeed(preview ? null : marketRef, dur);
  const chartDurs = preview ? ['1h', '24h'] : durs.length ? durs : ['1h'];

  const book = preview ? (sel ? mockBook(sel) : EMPTY_BOOK) : live.book;
  // Best opposing book levels (conventional price) let the order form estimate a
  // market order's spend/receive: a market buy lifts the best ask, a market sell
  // hits the best bid.
  const bestBid = book.buys[0]?.rate;
  const bestAsk = book.sells[0]?.rate;
  // The connection dot reflects the real DEX server state (connected and
  // authenticated) from the live conn/dex_auth feed; until the first such note
  // arrives, fall back to the order-book relay socket's health.
  const connected = preview ? true : conn ? conn.status === 1 && conn.authed : live.connected;
  const previewCandles = useMemo(() => (preview && sel ? mockCandles(sel) : []), [preview, sel]);
  const candles = preview ? previewCandles : live.candles;
  const liveStats = useMemo(() => statsFromCandles(live.candles), [live.candles]);
  const orders = preview ? mockOrders : liveOrders;
  const sameSel = (m: DexMarket) => !!sel && m.baseID === sel.baseID && m.quoteID === sel.quoteID;
  const spots = useDexSpots();
  const spotStats = (m: DexMarket): MarketStats | null => {
    const s = spots[`${m.baseID}-${m.quoteID}`];
    return s ? spotToStats(s, m) : null;
  };
  // Selected market keeps its richer candle-derived stats (live with
  // candle_update); other rows use the streamed spot feed, falling back to the
  // spot for the selected market before its candles load.
  const statsFor = (m: DexMarket): MarketStats | null =>
    preview ? mockStats(m) : sameSel(m) ? liveStats ?? spotStats(m) : spotStats(m);

  // When a market-maker bot is running on the selected market, the right-bottom
  // panel offers a Bot activity tab alongside the order form, defaulting to Bot.
  const botRun = useMMBotRun(HOST, sel?.baseID ?? -1, sel?.quoteID ?? -1);
  const botRunning = !preview && !!botRun?.running;
  const [rightTab, setRightTab] = useState<'order' | 'bot'>('order');
  // Below lg the four trading panes don't fit side by side; a segmented control
  // shows one at a time. The lg grid is unchanged.
  const [mobilePane, setMobilePane] = useState<'markets' | 'chart' | 'book' | 'trade'>('chart');
  useEffect(() => {
    setRightTab(botRunning ? 'bot' : 'order');
  }, [botRunning]);

  if (!sel) {
    // No market list at all (nothing cached yet). While the server is not
    // connected the config fetch cannot succeed, so show a quiet note rather
    // than an endless spinner; the spinner is for the connected loading case.
    if (!preview && conn && conn.status !== 1) {
      return (
        <div className="min-h-[40vh] flex items-center justify-center px-4">
          <span className="text-sm text-muted-foreground text-center">
            Market data loads once the DEX server is connected.
          </span>
        </div>
      );
    }
    if (loadErr) {
      return (
        <div className="px-4">
          <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{loadErr}</span>
          </div>
        </div>
      );
    }
    return (
      <div className="min-h-[40vh] flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  return (
    <div className="px-3 lg:px-4 space-y-3">
      {preview && (
        <div className="flex items-center gap-2 p-2 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning">
          <FlaskConical className="h-3.5 w-3.5 shrink-0" />
          Preview mode — sample data, not connected to a DEX server. Orders are disabled.
        </div>
      )}

      <DexStatsBar market={sel} stats={statsFor(sel)} connected={connected} preview={preview} />
      {!preview && live.error && <div className="text-xs text-warning px-1">{live.error}</div>}

      <div className="lg:hidden grid grid-cols-4 gap-1 rounded-lg bg-muted/20 p-1 text-xs">
        {([['markets', 'Markets'], ['chart', 'Chart'], ['book', 'Order book'], ['trade', 'Trade']] as const).map(
          ([id, label]) => (
            <button
              key={id}
              type="button"
              onClick={() => setMobilePane(id)}
              className={`py-1.5 rounded-md font-medium transition-colors ${
                mobilePane === id ? 'bg-primary/20 text-primary' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {label}
            </button>
          ),
        )}
      </div>

      <div className="grid grid-cols-1 gap-px rounded-xl overflow-hidden border border-border/60 bg-border/60 lg:grid-cols-[256px_1fr_330px] lg:grid-rows-[minmax(0,1fr)_340px] lg:h-[calc(100vh-11rem)]">
        <section className={`bg-card min-h-0 min-w-0 h-[70vh] lg:h-auto lg:max-h-none lg:block lg:col-start-1 lg:row-start-1 lg:row-span-2 ${mobilePane === 'markets' ? '' : 'hidden'}`}>
          <DexMarketsPanel markets={markets} selected={sel} onSelect={setSel} statsFor={statsFor} />
        </section>

        <section className={`bg-card min-h-0 min-w-0 h-[70vh] lg:h-auto lg:block lg:col-start-2 lg:row-start-1 lg:row-span-2 ${mobilePane === 'chart' ? '' : 'hidden'}`}>
          <DexChartPanel market={sel} candles={candles} durs={chartDurs} dur={dur} onDur={setDur} />
        </section>

        <section className={`bg-card min-h-0 min-w-0 h-[70vh] lg:h-auto lg:block lg:col-start-3 lg:row-start-1 ${mobilePane === 'book' ? '' : 'hidden'}`}>
          <DexOrderBook market={sel} book={book} onPick={onPick} />
        </section>

        <section className={`bg-card min-h-0 min-w-0 h-[70vh] overflow-y-auto lg:h-auto lg:block lg:col-start-3 lg:row-start-2 ${mobilePane === 'trade' ? '' : 'hidden'}`}>
          {botRunning && botRun ? (
            <div className="flex flex-col h-full">
              <div className="flex border-b border-border/50 text-xs">
                <button
                  type="button"
                  onClick={() => setRightTab('order')}
                  className={`flex-1 py-2 ${rightTab === 'order' ? 'text-foreground border-b-2 border-primary -mb-px' : 'text-muted-foreground'}`}
                >
                  Order form
                </button>
                <button
                  type="button"
                  onClick={() => setRightTab('bot')}
                  className={`flex-1 py-2 flex items-center justify-center gap-1.5 ${rightTab === 'bot' ? 'text-foreground border-b-2 border-primary -mb-px' : 'text-muted-foreground'}`}
                >
                  <span className="h-1.5 w-1.5 rounded-full bg-success animate-pulse" />
                  Bot
                </button>
              </div>
              {rightTab === 'bot' ? (
                <DexMMActivity bot={botRun} compact />
              ) : (
                <DexOrderForm host={HOST} market={sel} preview={preview} pick={pick} bestBid={bestBid} bestAsk={bestAsk} onPlaced={refreshOrders} />
              )}
            </div>
          ) : (
            <DexOrderForm host={HOST} market={sel} preview={preview} pick={pick} bestBid={bestBid} bestAsk={bestAsk} onPlaced={refreshOrders} />
          )}
        </section>
      </div>

      <DexUserOrdersPanel orders={orders} market={sel} preview={preview} onCancel={requestCancel} />

      <section className="h-[280px] rounded-xl overflow-hidden border border-border/60 bg-card">
        <DexOrdersPanel orders={orders} markets={markets} preview={preview} onCancel={requestCancel} />
      </section>

      {cancelModal}
    </div>
  );
};
