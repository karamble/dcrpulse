// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, FlaskConical } from 'lucide-react';
import { getDexConfig, getDexMyOrders, cancelDexOrder, type DexMarket, type DexOrder } from '../../services/dcrdexApi';
import { useDexFeed, statsFromCandles, type MarketStats } from './useDexFeed';
import { useDexRefreshOnNotes } from './DexLiveProvider';
import { DexStatsBar } from './DexStatsBar';
import { DexMarketsPanel } from './DexMarketsPanel';
import { DexChartPanel } from './DexChartPanel';
import { DexOrderBook } from './DexOrderBook';
import { DexOrdersPanel } from './DexOrdersPanel';
import { DexOrderForm } from './DexOrderForm';
import { mockMarkets, mockBook, mockStats, mockCandles, mockOrders } from './dexMockData';

const HOST = 'dex.decred.org:7232';
const EMPTY_BOOK = { buys: [], sells: [], recentMatches: [] };

// DexMarketView is the trading terminal: a market stats bar, a markets sidebar,
// a price chart, a live order book (with depth visualization) and recent
// trades, an order-entry form, and an open-orders panel. In preview mode it
// renders sample data (no server) so the UI can be developed without a
// reachable DEX server; the live candle and 24h-stats feeds are not wired yet.
export const DexMarketView = ({ preview = false }: { preview?: boolean }) => {
  const [markets, setMarkets] = useState<DexMarket[]>(preview ? mockMarkets : []);
  const [sel, setSel] = useState<DexMarket | null>(preview ? mockMarkets[0] : null);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [liveOrders, setLiveOrders] = useState<DexOrder[]>([]);
  const [durs, setDurs] = useState<string[]>([]);
  const [dur, setDur] = useState('1h');
  // Clicking a book level prefills the order form; seq lets a repeat click on
  // the same level re-apply.
  const pickSeq = useRef(0);
  const [pick, setPick] = useState<{ rate: number; qty: number; sell: boolean; seq: number } | null>(null);
  const onPick = (p: { rate: number; qty: number; sell: boolean }) => setPick({ ...p, seq: ++pickSeq.current });

  useEffect(() => {
    if (preview) return;
    getDexConfig(HOST)
      .then((c) => {
        setMarkets(c.markets);
        setSel((prev) => prev || c.markets[0] || null);
        if (c.candleDurs?.length) {
          setDurs(c.candleDurs);
          setDur((d) => (c.candleDurs.includes(d) ? d : c.candleDurs[0]));
        }
      })
      .catch((e: any) =>
        setLoadErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Failed to load markets'),
      );
  }, [preview]);

  const refreshOrders = () => {
    if (preview) return;
    getDexMyOrders(HOST)
      .then(setLiveOrders)
      .catch(() => {});
  };
  useEffect(() => {
    if (preview) return;
    refreshOrders();
    // Live order/match notifications drive refreshes; the slow interval is only
    // a backstop in case a note is missed or the notify socket is down.
    const id = window.setInterval(refreshOrders, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preview]);
  useDexRefreshOnNotes(['order', 'match'], refreshOrders);

  const marketRef = sel
    ? { host: HOST, base: sel.baseID, quote: sel.quoteID, baseConvFactor: sel.baseConvFactor, quoteConvFactor: sel.quoteConvFactor }
    : null;
  const live = useDexFeed(preview ? null : marketRef, dur);
  const chartDurs = preview ? ['1h', '24h'] : durs.length ? durs : ['1h'];

  const book = preview ? (sel ? mockBook(sel) : EMPTY_BOOK) : live.book;
  const connected = preview ? true : live.connected;
  const previewCandles = useMemo(() => (preview && sel ? mockCandles(sel) : []), [preview, sel]);
  const candles = preview ? previewCandles : live.candles;
  const liveStats = useMemo(() => statsFromCandles(live.candles), [live.candles]);
  const orders = preview ? mockOrders : liveOrders;
  const sameSel = (m: DexMarket) => !!sel && m.baseID === sel.baseID && m.quoteID === sel.quoteID;
  const statsFor = (m: DexMarket): MarketStats | null => (preview ? mockStats(m) : sameSel(m) ? liveStats : null);

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

  if (!sel) {
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

      <div className="grid grid-cols-1 gap-px rounded-xl overflow-hidden border border-border/60 bg-border/60 lg:grid-cols-[256px_1fr_330px] lg:grid-rows-[minmax(0,1fr)_340px] lg:h-[calc(100vh-11rem)]">
        <section className="bg-card min-h-0 min-w-0 max-h-[44vh] lg:max-h-none lg:col-start-1 lg:row-start-1 lg:row-span-2">
          <DexMarketsPanel markets={markets} selected={sel} onSelect={setSel} statsFor={statsFor} />
        </section>

        <section className="bg-card min-h-0 min-w-0 h-[60vh] lg:h-auto lg:col-start-2 lg:row-start-1 lg:row-span-2">
          <DexChartPanel market={sel} candles={candles} durs={chartDurs} dur={dur} onDur={setDur} />
        </section>

        <section className="bg-card min-h-0 min-w-0 h-[420px] lg:h-auto lg:col-start-3 lg:row-start-1">
          <DexOrderBook market={sel} book={book} onPick={onPick} />
        </section>

        <section className="bg-card min-h-0 min-w-0 lg:col-start-3 lg:row-start-2">
          <DexOrderForm host={HOST} market={sel} preview={preview} pick={pick} onPlaced={refreshOrders} />
        </section>
      </div>

      <section className="h-[280px] rounded-xl overflow-hidden border border-border/60 bg-card">
        <DexOrdersPanel orders={orders} preview={preview} onCancel={async (id) => {
          try {
            await cancelDexOrder(id);
            refreshOrders();
          } catch {
            /* ignore */
          }
        }} />
      </section>
    </div>
  );
};
