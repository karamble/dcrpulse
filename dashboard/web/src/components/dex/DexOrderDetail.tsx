// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ArrowLeft, Check, X } from 'lucide-react';
import { isCancellable, type DexCoin, type DexLiveMatch, type DexMarket, type DexMatch, type DexOrder } from '../../services/dcrdexApi';
import { convQty, convRate, fmtAmt, fmtPrice } from './dexFormat';
import { useDexLiveMatches } from './DexLiveProvider';

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

// A DEX swap settles in four stages between the maker and the taker. bisonw's
// myorders match reports the coins from this client's perspective (swap /
// counterSwap / redeem / counterRedeem); these helpers re-key them by role so
// each stage shows the right coin and "you"/"them" label regardless of which
// side this client took. Mirrors bisonw's order.ts coin helpers.
const isMaker = (m: DexMatch) => (m.side || '').toLowerCase().includes('maker');
const makerSwapCoin = (m: DexMatch) => (isMaker(m) ? m.swap : m.counterSwap);
const takerSwapCoin = (m: DexMatch) => (isMaker(m) ? m.counterSwap : m.swap);
const makerRedeemCoin = (m: DexMatch) => (isMaker(m) ? m.redeem : m.counterRedeem);
const takerRedeemCoin = (m: DexMatch) => (isMaker(m) ? m.counterRedeem : m.redeem);

// Which asset each stage settles in. The maker swaps the asset it offers; for
// our order that depends on whether we sell (offer base) and on our match side.
// Mirrors bisonw's order.ts asset matrix.
const stepAssets = (m: DexMatch, sell: boolean, base: string, quote: string) => {
  const baseFirst = (isMaker(m) && sell) || (!isMaker(m) && !sell);
  return baseFirst
    ? { makerSwap: base, takerSwap: quote, makerRedeem: quote, takerRedeem: base }
    : { makerSwap: quote, takerSwap: base, makerRedeem: base, takerRedeem: quote };
};

// Progress index (0..4) from the match status, so the four stages can render a
// filled/pending state. Maps bisonw's order.MatchStatus string forms.
const STEP_BY_STATUS: Record<string, number> = {
  newlymatched: 0,
  makerswapcast: 1,
  takerswapcast: 2,
  makerredeemed: 3,
  matchcomplete: 4,
  matchconfirmed: 4,
};

// Insert spaces into bisonw's CamelCase status (e.g. "TakerSwapCast").
const prettyStatus = (s: string) => (s || '').replace(/([a-z])([A-Z])/g, '$1 $2');

const truncCoin = (c: string) => (c.length > 22 ? `${c.slice(0, 12)}…${c.slice(-8)}` : c);

// One settlement stage row: a status dot, the role/asset/you-them label, and the
// on-chain coin id once it is broadcast (otherwise "Pending"). When a live coin
// is available (from the order/match notes), its formatted id and confirmation
// progress are shown; otherwise the myorders hex id is shown (presence only).
const SwapStep = ({ n, label, asset, you, coin, live }: { n: number; label: string; asset: string; you: boolean; coin?: string; live?: DexCoin }) => {
  const id = live?.stringID || coin;
  const confs = live?.confs;
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
        {id ? (
          <div className="flex items-baseline gap-2">
            <span className="font-mono text-[11px] break-all" title={id}>{truncCoin(id)}</span>
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
// numbered stages plus an optional refund. Cancel matches carry no swap and are
// skipped.
const MatchCard = ({ m, lm, order, baseSym, quoteSym, baseConv, quoteConv }: {
  m: DexMatch;
  lm?: DexLiveMatch;
  order: DexOrder;
  baseSym: string;
  quoteSym: string;
  baseConv: number;
  quoteConv: number;
}) => {
  const a = stepAssets(m, order.sell, baseSym, quoteSym);
  const youMaker = isMaker(m);
  const idx = STEP_BY_STATUS[(m.status || '').toLowerCase()] ?? 0;
  const showRefund = !!m.refund || m.revoked;
  // Re-key the live coins (with confs) by role, mirroring the hex helpers above;
  // the note's match side is numeric (0 = maker).
  const lmMaker = lm ? lm.side === 0 : youMaker;
  const lmMakerSwap = lm ? (lmMaker ? lm.swap : lm.counterSwap) : undefined;
  const lmTakerSwap = lm ? (lmMaker ? lm.counterSwap : lm.swap) : undefined;
  const lmMakerRedeem = lm ? (lmMaker ? lm.redeem : lm.counterRedeem) : undefined;
  const lmTakerRedeem = lm ? (lmMaker ? lm.counterRedeem : lm.redeem) : undefined;
  const refundId = lm?.refund?.stringID || m.refund;
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
        <SwapStep n={1} label="Maker Swap" asset={a.makerSwap} you={youMaker} coin={makerSwapCoin(m)} live={lmMakerSwap} />
        <SwapStep n={2} label="Taker Swap" asset={a.takerSwap} you={!youMaker} coin={takerSwapCoin(m)} live={lmTakerSwap} />
        <SwapStep n={3} label="Maker Redemption" asset={a.makerRedeem} you={youMaker} coin={makerRedeemCoin(m)} live={lmMakerRedeem} />
        <SwapStep n={4} label="Taker Redemption" asset={a.takerRedeem} you={!youMaker} coin={takerRedeemCoin(m)} live={lmTakerRedeem} />
        {showRefund && (
          <div className="flex items-start gap-2 px-3 py-1.5">
            <span className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-destructive/20 text-[9px] text-destructive">!</span>
            <div className="min-w-0 flex-1">
              <div className="text-[11px] text-destructive">Refund ({order.sell ? baseSym : quoteSym}, you)</div>
              {refundId ? (
                <span className="font-mono text-[11px] break-all" title={refundId}>{truncCoin(refundId)}</span>
              ) : (
                <span className="text-[11px] text-muted-foreground/60">&lt;Pending&gt;</span>
              )}
            </div>
          </div>
        )}
      </div>
      <div className="flex gap-1 px-3 pb-2">
        {[1, 2, 3, 4].map((s) => (
          <span key={s} className={`h-1 flex-1 rounded ${s <= idx ? 'bg-success' : 'bg-muted/40'}`} />
        ))}
      </div>
    </div>
  );
};

// DexOrderDetail is the in-tab detail view for a single order: summary, fill
// progress and the per-counterparty matches. Amounts are converted to
// conventional units using the order's market.
export const DexOrderDetail = ({ order, market, onBack, onCancel }: Props) => {
  // Live coins + confirmations from order/match notes, overlaid on the
  // confs-less myorders match below.
  const live = useDexLiveMatches(order.id);
  const baseConv = market?.baseConvFactor || 1e8;
  const quoteConv = market?.quoteConvFactor || 1e8;
  const baseSym = market?.base || order.marketName.split('_')[0]?.toUpperCase() || '';
  const quoteSym = market?.quote || order.marketName.split('_')[1]?.toUpperCase() || '';

  const qty = convQty(order.quantity, baseConv);
  const filled = convQty(order.filled, baseConv);
  const settled = convQty(order.settled, baseConv);
  const price = order.rate ? convRate(order.rate, baseConv, quoteConv) : 0;
  const filledPct = order.quantity > 0 ? Math.round((order.filled / order.quantity) * 100) : 0;

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
          <span className="text-xs text-muted-foreground">{order.status}</span>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <Field label="Quantity">{fmtAmt(qty, 8)} {baseSym}</Field>
          {order.rate > 0 && <Field label="Price">{fmtPrice(price, quoteSym)} {quoteSym}</Field>}
          <Field label="Filled">{fmtAmt(filled, 8)} ({filledPct}%)</Field>
          <Field label="Settled">{fmtAmt(settled, 8)} {baseSym}</Field>
          <Field label="Submitted">{order.submitTime ? new Date(order.submitTime).toLocaleString() : '-'}</Field>
          <Field label="Order ID">{order.id}</Field>
        </div>
      </div>

      <div className="rounded-xl border border-border/50 overflow-hidden">
        <div className="px-4 py-2 text-[10px] uppercase tracking-wider text-muted-foreground/60 border-b border-border/40">
          Matches ({order.matches?.length || 0})
        </div>
        {(() => {
          const swaps = (order.matches || []).filter((m) => !m.isCancel);
          if (swaps.length === 0) {
            return <div className="px-4 py-6 text-xs text-muted-foreground">No matches yet.</div>;
          }
          return swaps.map((m) => (
            <MatchCard
              key={m.matchID}
              m={m}
              lm={live[m.matchID]}
              order={order}
              baseSym={baseSym}
              quoteSym={quoteSym}
              baseConv={baseConv}
              quoteConv={quoteConv}
            />
          ));
        })()}
      </div>
    </div>
  );
};
