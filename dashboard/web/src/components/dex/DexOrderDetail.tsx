// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Check, X } from 'lucide-react';
import {
  getDexOrder,
  isCancellable,
  orderHasActiveMatches,
  orderStatusString,
  type DexCoin,
  type DexFullMatch,
  type DexMarket,
  type DexMatch,
  type DexOrder,
  type DexOrderFull,
} from '../../services/dcrdexApi';
import { convQty, convRate, fmtAmt, fmtPrice } from './dexFormat';
import { dexCoinExplorer } from './dexExplorers';
import { stepIndex, StepBar } from './dexSteps';
import { useDexRefreshOnNotes } from './DexLiveProvider';

interface Props {
  order: DexOrder;
  market?: DexMarket;
  onBack: () => void;
  onCancel: (order: DexOrder, market?: DexMarket) => void;
}

const Field = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <div className="space-y-0.5">
    <div className="text-[10px] uppercase tracking-wider text-muted-foreground/60">{label}</div>
    <div className="text-sm font-mono tabular-nums break-all">{children}</div>
  </div>
);

// A DEX swap settles in four stages between maker and taker. The match reports
// the coins from this client's perspective (swap / counterSwap / redeem /
// counterRedeem); these helpers re-key them by role so each stage shows the
// right coin and "you"/"them" label regardless of which side we took. Mirrors
// bisonw's order.ts coin helpers.
const isMaker = (m: DexFullMatch) => (m.side || '').toLowerCase().includes('maker');
const makerSwapCoin = (m: DexFullMatch) => (isMaker(m) ? m.swap : m.counterSwap);
const takerSwapCoin = (m: DexFullMatch) => (isMaker(m) ? m.counterSwap : m.swap);
const makerRedeemCoin = (m: DexFullMatch) => (isMaker(m) ? m.redeem : m.counterRedeem);
const takerRedeemCoin = (m: DexFullMatch) => (isMaker(m) ? m.counterRedeem : m.redeem);

// richFromDexMatch adapts a confs-less myorders match (hex-string coins) into the
// rich shape used for rendering, assigning each coin its settling asset id from
// the order side (swap/refund are on the asset we offer, redeem on the asset we
// receive, and the counterparty's coins are the mirror). Used as a fallback until
// the single-order fetch (which carries assets + confs) resolves.
const richFromDexMatch = (m: DexMatch, sell: boolean, baseID: number, quoteID: number): DexFullMatch => {
  const offered = sell ? baseID : quoteID;
  const received = sell ? quoteID : baseID;
  const coin = (s: string | undefined, assetID: number): DexCoin | undefined => (s ? { stringID: s, assetID } : undefined);
  return {
    matchID: m.matchID,
    status: m.status,
    revoked: m.revoked,
    rate: m.rate,
    qty: m.qty,
    side: m.side,
    feeRate: m.feeRate,
    stamp: m.stamp,
    isCancel: m.isCancel,
    swap: coin(m.swap, offered),
    counterSwap: coin(m.counterSwap, received),
    redeem: coin(m.redeem, received),
    counterRedeem: coin(m.counterRedeem, offered),
    refund: coin(m.refund, offered),
  };
};

// Lock times must match bisonw's LockTimeMaker / LockTimeTaker (mainnet): a maker
// can refund its own swap after 20h, a taker after 8h, if the counterparty never
// redeems.
const lockTimeMakerMs = 20 * 60 * 60 * 1000;
const lockTimeTakerMs = 8 * 60 * 60 * 1000;
const refundCountdown = (stampMs: number, maker: boolean): string => {
  const after = stampMs + (maker ? lockTimeMakerMs : lockTimeTakerMs);
  if (Date.now() > after) return 'Refund imminent';
  return `Refund available after ${new Date(after).toLocaleString()}`;
};

// Insert spaces into bisonw's CamelCase match status (e.g. "TakerSwapCast").
const prettyStatus = (s: string) => (s || '').replace(/([a-z])([A-Z])/g, '$1 $2');

const truncCoin = (c: string) => (c.length > 22 ? `${c.slice(0, 12)}…${c.slice(-8)}` : c);

const DCR_ASSET_ID = 42;

// CoinID renders a coin id as a link to its block explorer. Decred coins go to
// dcrpulse's own /explorer/tx route (same-tab SPA navigation, like the wallet tx
// history); other assets link out to an external explorer (the global
// ExternalLinkGuard handles the leaving-site confirm). Unknown assets render as
// plain mono text.
const CoinID = ({ assetID, id }: { assetID: number; id: string }) => {
  const cls = 'font-mono text-[11px] break-all text-primary hover:underline';
  if (assetID === DCR_ASSET_ID) {
    return <Link to={`/explorer/tx/${id.split(':')[0]}`} title={id} className={cls}>{truncCoin(id)}</Link>;
  }
  const url = dexCoinExplorer(assetID, id);
  if (!url) return <span className="font-mono text-[11px] break-all" title={id}>{truncCoin(id)}</span>;
  return (
    <a href={url} target="_blank" rel="noopener noreferrer" title={id} className={cls}>
      {truncCoin(id)}
    </a>
  );
};

// One settlement stage row: a status dot, the role/asset/you-them label, the
// on-chain coin id once broadcast (linked) and its live confirmation progress.
const SwapStep = ({ n, label, asset, you, coin }: { n: number; label: string; asset: string; you: boolean; coin?: DexCoin }) => {
  const id = coin?.stringID;
  const confs = coin?.confs;
  const met = confs ? confs.count >= confs.required : !!id;
  return (
    <div className="flex items-start gap-2 px-3 py-1.5">
      <span
        className={`mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full text-[9px] ${
          met ? 'bg-success/20 text-success' : id ? 'bg-warning/20 text-warning' : 'bg-muted/40 text-muted-foreground/60'
        }`}
      >
        {met ? <Check className="h-2.5 w-2.5" /> : n}
      </span>
      <div className="min-w-0 flex-1">
        <div className="text-[11px] text-muted-foreground">
          {n}. {label} <span className="text-muted-foreground/60">({asset}, {you ? 'you' : 'them'})</span>
        </div>
        {id && coin ? (
          <div className="flex items-baseline gap-2">
            <CoinID assetID={coin.assetID} id={id} />
            {confs && (
              <span className={`text-[10px] shrink-0 ${met ? 'text-success' : 'text-warning'}`}>
                {met ? 'confirmed' : `${confs.count}/${confs.required} confs`}
              </span>
            )}
          </div>
        ) : (
          <span className="text-[11px] text-muted-foreground/60">&lt;Pending&gt;</span>
        )}
      </div>
    </div>
  );
};

// MatchCard renders a single match as the maker/taker swap negotiation: four
// numbered stages plus an optional refund, with a 4-step progress bar.
const MatchCard = ({ m, order, baseSym, quoteSym, baseConv, quoteConv }: {
  m: DexFullMatch;
  order: DexOrder;
  baseSym: string;
  quoteSym: string;
  baseConv: number;
  quoteConv: number;
}) => {
  const youMaker = isMaker(m);
  // Asset label each stage settles in: the maker swaps the asset it offers, which
  // for our order depends on whether we sell (offer base) and our match side.
  const baseFirst = (youMaker && order.sell) || (!youMaker && !order.sell);
  const a = baseFirst
    ? { makerSwap: baseSym, takerSwap: quoteSym, makerRedeem: quoteSym, takerRedeem: baseSym }
    : { makerSwap: quoteSym, takerSwap: baseSym, makerRedeem: baseSym, takerRedeem: quoteSym };
  const idx = stepIndex(m.status);
  const showRefund = !!m.refund || m.revoked;
  return (
    <div className="border-t border-border/40 first:border-t-0">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 px-3 py-2 text-xs">
        <span className="font-medium">{youMaker ? 'Maker' : 'Taker'}</span>
        <span className="font-mono tabular-nums text-muted-foreground">
          {fmtAmt(convQty(m.qty, baseConv), 8)} {baseSym} @ {fmtPrice(convRate(m.rate, baseConv, quoteConv), quoteSym)} {quoteSym}
        </span>
        <span className="ml-auto text-muted-foreground">{prettyStatus(m.status)}</span>
        {m.revoked && <span className="text-destructive">(revoked)</span>}
      </div>
      <div className="pb-1.5">
        <SwapStep n={1} label="Maker Swap" asset={a.makerSwap} you={youMaker} coin={makerSwapCoin(m)} />
        <SwapStep n={2} label="Taker Swap" asset={a.takerSwap} you={!youMaker} coin={takerSwapCoin(m)} />
        <SwapStep n={3} label="Maker Redemption" asset={a.makerRedeem} you={youMaker} coin={makerRedeemCoin(m)} />
        <SwapStep n={4} label="Taker Redemption" asset={a.takerRedeem} you={!youMaker} coin={takerRedeemCoin(m)} />
        {showRefund && (
          <div className="flex items-start gap-2 px-3 py-1.5">
            <span className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-destructive/20 text-[9px] text-destructive">!</span>
            <div className="min-w-0 flex-1">
              <div className="text-[11px] text-destructive">Refund ({order.sell ? baseSym : quoteSym}, you)</div>
              {m.refund ? (
                <CoinID assetID={m.refund.assetID} id={m.refund.stringID} />
              ) : (
                <span className="text-[11px] text-muted-foreground/60">{refundCountdown(m.stamp, youMaker)}</span>
              )}
            </div>
          </div>
        )}
      </div>
      <StepBar idx={idx} className="px-3 pb-2" />
    </div>
  );
};

// Bar renders a labelled progress bar (filled / settled) in the summary.
const Bar = ({ label, pct, tint }: { label: string; pct: number; tint: string }) => (
  <div className="space-y-0.5">
    <div className="flex justify-between text-[10px] uppercase tracking-wider text-muted-foreground/60">
      <span>{label}</span>
      <span className="tabular-nums">{pct}%</span>
    </div>
    <span className="block h-1.5 rounded bg-muted/40 overflow-hidden">
      <span className={`block h-full ${tint}`} style={{ width: `${pct}%` }} />
    </span>
  </div>
);

// DexOrderDetail is the in-tab detail view for a single order: summary, fill and
// settlement progress, and the per-counterparty matches. The single order is
// fetched from /dcrdex/order (the only source of live swap confirmations) on open
// and on order/match notes; until it resolves, the confs-less list match is used.
export const DexOrderDetail = ({ order, market, onBack, onCancel }: Props) => {
  const [full, setFull] = useState<DexOrderFull | null>(null);
  const loadFull = useCallback(() => {
    getDexOrder(order.id)
      .then(setFull)
      .catch(() => {});
  }, [order.id]);
  useEffect(() => {
    setFull(null);
    loadFull();
  }, [loadFull]);
  useDexRefreshOnNotes(['order', 'match'], loadFull);
  // While the order is still settling, poll the single-order route so the swap
  // steps + confirmation counts advance on their own, even if a match note is
  // missed or late. Stops once nothing is active.
  const settling = orderHasActiveMatches(order);
  useEffect(() => {
    if (!settling) return;
    const id = window.setInterval(loadFull, 10000);
    return () => window.clearInterval(id);
  }, [settling, loadFull]);

  const baseConv = market?.baseConvFactor || 1e8;
  const quoteConv = market?.quoteConvFactor || 1e8;
  const baseSym = market?.base || order.marketName.split('_')[0]?.toUpperCase() || '';
  const quoteSym = market?.quote || order.marketName.split('_')[1]?.toUpperCase() || '';

  const qty = convQty(order.quantity, baseConv);
  const price = order.rate ? convRate(order.rate, baseConv, quoteConv) : 0;
  const filledPct = order.quantity > 0 ? Math.round((order.filled / order.quantity) * 100) : 0;
  const settledPct = order.quantity > 0 ? Math.round((order.settled / order.quantity) * 100) : 0;

  // Prefer the fetched (confs-bearing) matches; fall back to the list order's
  // matches adapted to the rich shape until the fetch resolves.
  const matches: DexFullMatch[] =
    full?.matches ?? (order.matches || []).map((m) => richFromDexMatch(m, order.sell, order.baseID, order.quoteID));
  const swaps = matches.filter((m) => !m.isCancel);

  return (
    <div className="px-3 lg:px-4 space-y-4">
      <div className="flex items-center justify-between gap-3">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Orders
        </button>
        {isCancellable(order) && (
          <button
            type="button"
            onClick={() => onCancel(order, market)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-destructive/40 text-destructive rounded-lg hover:bg-destructive/10 transition-colors"
          >
            <X className="h-4 w-4" />
            Cancel order
          </button>
        )}
      </div>

      <div className="p-4 rounded-xl bg-gradient-card border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <span className="font-mono">{baseSym}/{quoteSym}</span>
          <span
            className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${
              order.sell ? 'bg-destructive/15 text-destructive' : 'bg-success/15 text-success'
            }`}
          >
            {order.sell ? 'Sell' : 'Buy'}
          </span>
          <span className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-muted/40 text-muted-foreground">{order.type}</span>
          <span className="text-xs text-muted-foreground">{orderStatusString(order)}</span>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <Field label="Quantity">{fmtAmt(qty, 8)} {baseSym}</Field>
          {order.rate > 0 && <Field label="Price">{fmtPrice(price, quoteSym)} {quoteSym}</Field>}
          <Field label="Submitted">{order.submitTime ? new Date(order.submitTime).toLocaleString() : '-'}</Field>
          <Field label="Order ID">{order.id}</Field>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <Bar label="Filled" pct={filledPct} tint="bg-primary" />
          <Bar label="Settled" pct={settledPct} tint="bg-success" />
        </div>
      </div>

      <div className="rounded-xl border border-border/50 overflow-hidden">
        <div className="px-4 py-2 text-[10px] uppercase tracking-wider text-muted-foreground/60 border-b border-border/40">
          Matches ({swaps.length})
        </div>
        {swaps.length === 0 ? (
          <div className="px-4 py-6 text-xs text-muted-foreground">No matches yet.</div>
        ) : (
          swaps.map((m) => (
            <MatchCard
              key={m.matchID}
              m={m}
              order={order}
              baseSym={baseSym}
              quoteSym={quoteSym}
              baseConv={baseConv}
              quoteConv={quoteConv}
            />
          ))
        )}
      </div>
    </div>
  );
};
