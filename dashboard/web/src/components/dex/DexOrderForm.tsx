// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, AlertTriangle, ChevronDown, ChevronUp } from 'lucide-react';
import {
  getDexWallets,
  placeDexOrder,
  preDexOrder,
  maxDexBuy,
  maxDexSell,
  type DexMarket,
  type DexWalletState,
  type OrderEstimate,
  type OrderOption,
} from '../../services/dcrdexApi';
import { fmtAmt, fmtPrice } from './dexFormat';
import { useDexRefreshOnNotes } from './DexLiveProvider';

// RateEncodingFactor mirrors bisonw's OrderUtil.RateEncodingFactor: the DEX
// message rate is the conventional price scaled by 1e8 and adjusted by the
// base/quote conversion factors. This conversion is mirrored 1:1 from bisonw's
// own frontend (client/webserver markets.ts) for the funds-critical encoding.
const RateEncodingFactor = 1e8;

const serverMsg = (e: any): string =>
  (typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Request failed';

interface DexOrderFormProps {
  host: string;
  market: DexMarket;
  preview?: boolean;
  // pick prefills the form from a clicked order book level (price + size) and
  // takes that level's side; seq lets a repeat click re-apply.
  pick?: { rate: number; qty: number; sell: boolean; seq: number } | null;
  // Best book levels (conventional price) used to estimate the spend/receive of
  // a market order, which has no limit price of its own. bestAsk prices a market
  // buy, bestBid a market sell.
  bestBid?: number;
  bestAsk?: number;
  onPlaced: () => void;
}

export const DexOrderForm = ({ host, market, preview = false, pick, bestBid, bestAsk, onPlaced }: DexOrderFormProps) => {
  const [sell, setSell] = useState(false);
  const [isLimit, setIsLimit] = useState(true);
  const [price, setPrice] = useState('');
  // qty (base coins) is the canonical funding input; lotsStr is the primary
  // user-facing field, kept in sync with qty (a whole number of lots). spend is
  // the quote-amount input used only for a market buy (bisonw sizes a market buy
  // by the quote spend, not by lots).
  const [qty, setQty] = useState('');
  const [lotsStr, setLotsStr] = useState('');
  const [spend, setSpend] = useState('');
  const [opts, setOpts] = useState<Record<string, string>>({});
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [wallets, setWallets] = useState<DexWalletState[]>([]);
  // est is the pre-order fee/option estimate; estErr surfaces a server-side
  // validation failure (insufficient funds/reserves) before the user commits.
  const [est, setEst] = useState<OrderEstimate | null>(null);
  const [estErr, setEstErr] = useState<string | null>(null);
  const [maxLots, setMaxLots] = useState<number | null>(null);

  const baseSym = market.base;
  const quoteSym = market.quote.split('.')[0];
  const isMarketBuy = !isLimit && !sell;

  const lotConventional = market.lotSize / market.baseConvFactor;
  // rateConversionFactor: msgRate = price * (1e8 / baseConv * quoteConv).
  const rateConversionFactor = (RateEncodingFactor / market.baseConvFactor) * market.quoteConvFactor;
  const rateStepConventional = market.rateStep > 0 ? market.rateStep / rateConversionFactor : 0;

  const fmtField = (n: number) => (n > 0 ? String(Number(n.toFixed(8))) : '');
  const snapToLot = (v: number) => {
    if (market.lotSize <= 0 || !isFinite(v) || v <= 0) return 0;
    const n = Math.max(1, Math.round((v * market.baseConvFactor) / market.lotSize));
    return (n * market.lotSize) / market.baseConvFactor;
  };
  const snapToStep = (v: number) => {
    if (market.rateStep <= 0 || !isFinite(v) || v <= 0) return 0;
    const n = Math.max(1, Math.round((v * rateConversionFactor) / market.rateStep));
    return (n * market.rateStep) / rateConversionFactor;
  };

  // Lots and the coin amount are two views of the same quantity. coinsToLots
  // floors a coin amount to whole lots; the two setters keep both fields in step.
  const coinsToLots = (coins: number) =>
    market.lotSize > 0 && isFinite(coins) && coins > 0
      ? Math.floor(Math.round(coins * market.baseConvFactor) / market.lotSize)
      : 0;
  const setFromLots = (v: string) => {
    setLotsStr(v);
    const n = Math.max(0, Math.floor(parseFloat(v || '0')));
    setQty(n > 0 ? fmtField((n * market.lotSize) / market.baseConvFactor) : '');
  };
  const setFromQty = (v: string) => {
    setQty(v);
    const n = coinsToLots(parseFloat(v || '0'));
    setLotsStr(n > 0 ? String(n) : '');
  };
  const snapQtyBlur = () => {
    if (!qty) return;
    const s = snapToLot(parseFloat(qty || '0'));
    setQty(fmtField(s));
    setLotsStr(s > 0 ? String(coinsToLots(s)) : '');
  };

  // Prefill from a clicked order book level: take the level's side (ask -> buy,
  // bid -> sell), set its price and size, snapped to the market increments.
  useEffect(() => {
    if (!pick) return;
    setIsLimit(true);
    setSell(!pick.sell);
    setPrice(fmtField(snapToStep(pick.rate)));
    const q = snapToLot(pick.qty);
    setQty(fmtField(q));
    setLotsStr(q > 0 ? String(coinsToLots(q)) : '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pick?.seq]);

  // Keep wallet balances on hand so the form can flag insufficient funds. Skip
  // in preview (no server, and the form renders outside the live provider).
  const refreshWallets = () => {
    if (preview) return;
    getDexWallets().then(setWallets).catch(() => {});
  };
  useEffect(() => {
    refreshWallets();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preview]);
  useDexRefreshOnNotes(['balance', 'walletstate'], refreshWallets);

  const qtyFloat = parseFloat(qty || '0');
  const priceFloat = parseFloat(price || '0');
  const spendFloat = parseFloat(spend || '0');
  // Snap quantity down to a whole number of lots.
  const lots = market.lotSize > 0 ? Math.floor(Math.round(qtyFloat * market.baseConvFactor) / market.lotSize) : 0;
  const qtyAtomic = lots * market.lotSize;
  const qtyEffective = qtyAtomic / market.baseConvFactor;
  // Snap rate to a multiple of the market rate step.
  const rawMsgRate = Math.round(priceFloat * rateConversionFactor);
  const msgRate = market.rateStep > 0 ? Math.round(rawMsgRate / market.rateStep) * market.rateStep : rawMsgRate;
  const rateEffective = msgRate / rateConversionFactor;
  const total = isLimit ? rateEffective * qtyEffective : 0;
  // A market buy is sized by the quote spend (atoms); everything else by lots.
  const spendAtomic = Math.round(spendFloat * market.quoteConvFactor);
  const submitQty = isMarketBuy ? spendAtomic : qtyAtomic;
  const submitRate = isLimit ? msgRate : 0;
  const hasQty = isMarketBuy ? spendAtomic > 0 : lots >= 1;
  // The order is estimable/placeable once it has a quantity (and, for a limit, a
  // valid rate).
  const orderReady = hasQty && (!isLimit || msgRate > 0);

  // Spend/receive summary. The base side is always exact (whole lots / direct);
  // the quote side is exact for a limit order and estimated from the best
  // opposing book level for a market order. A market buy is the mirror case: the
  // quote spend is exact and the received base is the estimate.
  const mktBuyRecvBase = isMarketBuy && bestAsk && bestAsk > 0 ? spendFloat / bestAsk : 0;
  const mktBuyLots = lotConventional > 0 ? Math.floor(mktBuyRecvBase / lotConventional) : 0;
  const estQuote = isLimit ? total : (sell ? bestBid : bestAsk) && (sell ? bestBid! : bestAsk!) > 0 ? qtyEffective * (sell ? bestBid! : bestAsk!) : 0;
  const quoteKnown = isLimit ? total > 0 : !!(sell ? bestBid : bestAsk);

  let spendVal: number, spendSym: string, spendKnown: boolean, spendEst: boolean;
  let receiveVal: number, receiveSym: string, receiveKnown: boolean, receiveEst: boolean;
  if (isMarketBuy) {
    spendVal = spendFloat; spendSym = quoteSym; spendKnown = spendFloat > 0; spendEst = false;
    receiveVal = mktBuyRecvBase; receiveSym = baseSym; receiveKnown = !!bestAsk && bestAsk > 0 && spendFloat > 0; receiveEst = true;
  } else if (sell) {
    spendVal = qtyEffective; spendSym = baseSym; spendKnown = true; spendEst = false;
    receiveVal = estQuote; receiveSym = quoteSym; receiveKnown = quoteKnown; receiveEst = !isLimit;
  } else {
    spendVal = estQuote; spendSym = quoteSym; spendKnown = quoteKnown; spendEst = !isLimit;
    receiveVal = qtyEffective; receiveSym = baseSym; receiveKnown = true; receiveEst = false;
  }
  // When the quote amount is unknown, a limit order is just missing its price
  // ("-"); a market order has no book level to price against ("at market").
  const unknownQuote = isLimit ? '-' : 'at market';
  const fmtCell = (val: number, known: boolean, est2: boolean, sym: string) =>
    known ? `${est2 ? '~' : ''}${fmtAmt(val, 8)} ${sym}` : `${unknownQuote} ${sym}`;

  // A buy locks the quote asset, a sell the base asset. Compare the order's
  // notional requirement against the funding wallet's available balance.
  const baseAvail = wallets.find((w) => w.assetID === market.baseID)?.available;
  const quoteAvail = wallets.find((w) => w.assetID === market.quoteID)?.available;
  const need = sell ? qtyEffective : isMarketBuy ? spendFloat : isLimit ? total : 0;
  const have = sell ? baseAvail : quoteAvail;
  const insufficient = have != null && hasQty && need > 0 && need > have;
  const overMax = !isMarketBuy && maxLots != null && lots > maxLots;

  const valid = !preview && orderReady && !insufficient;

  // Pre-order estimate. Debounced; refetched whenever the order parameters or the
  // selected options change. A failure (e.g. not enough to cover fees/reserves)
  // is surfaced so the user sees it before committing real funds.
  const optsKey = useMemo(() => JSON.stringify(opts), [opts]);
  useEffect(() => {
    if (preview || !orderReady) {
      setEst(null);
      setEstErr(null);
      return;
    }
    let cancelled = false;
    const t = window.setTimeout(() => {
      preDexOrder({
        host,
        base: market.baseID,
        quote: market.quoteID,
        isLimit,
        sell,
        qty: submitQty,
        rate: submitRate,
        tifNow: !isLimit,
        options: opts,
      })
        .then((e) => {
          if (cancelled) return;
          setEst(e);
          setEstErr(null);
        })
        .catch((e) => {
          if (cancelled) return;
          setEst(null);
          setEstErr(serverMsg(e));
        });
    }, 400);
    return () => {
      cancelled = true;
      window.clearTimeout(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preview, orderReady, host, market.baseID, market.quoteID, isLimit, sell, submitQty, submitRate, optsKey]);

  // Seed any boolean options the estimate reports with their defaults (once per
  // new key), so toggling reflects the server's default state.
  useEffect(() => {
    if (!est) return;
    const all = [...(est.swap?.options || []), ...(est.redeem?.options || [])].filter((o) => o.boolean);
    setOpts((prev) => {
      let changed = false;
      const next = { ...prev };
      for (const o of all) {
        if (!(o.key in next)) {
          next[o.key] = o.default ?? 'false';
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [est]);

  // Max fundable lots for the lots-based modes (guidance, not a hard gate). A
  // market buy is sized by spend, so it has no lot max.
  const availKey = `${baseAvail ?? ''}:${quoteAvail ?? ''}`;
  const rateForMax = isLimit ? msgRate : 0;
  useEffect(() => {
    if (preview || isMarketBuy) {
      setMaxLots(null);
      return;
    }
    let cancelled = false;
    const t = window.setTimeout(() => {
      const pr = sell
        ? maxDexSell(host, market.baseID, market.quoteID)
        : rateForMax > 0
          ? maxDexBuy(host, market.baseID, market.quoteID, rateForMax)
          : null;
      if (!pr) {
        setMaxLots(null);
        return;
      }
      pr.then((r) => !cancelled && setMaxLots(r?.swap?.lots ?? null)).catch(() => !cancelled && setMaxLots(null));
    }, 350);
    return () => {
      cancelled = true;
      window.clearTimeout(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preview, isMarketBuy, sell, host, market.baseID, market.quoteID, rateForMax, availKey]);

  // Network-fee estimate (worst case) in the from-asset (swap) and to-asset
  // (redeem). For a token market the fee is paid in the parent gas asset; the
  // conv-factor display here is approximate for that case.
  const fromConv = sell ? market.baseConvFactor : market.quoteConvFactor;
  const toConv = sell ? market.quoteConvFactor : market.baseConvFactor;
  const fromSym = sell ? baseSym : quoteSym;
  const toSym = sell ? quoteSym : baseSym;
  const swapFee = est ? est.swap.estimate.realisticWorstCase / fromConv : 0;
  const redeemFee = est ? est.redeem.estimate.realisticWorstCase / toConv : 0;
  const boolOpts: OrderOption[] = est
    ? [...(est.swap?.options || []), ...(est.redeem?.options || [])].filter((o) => o.boolean)
    : [];

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      await placeDexOrder({
        host,
        base: market.baseID,
        quote: market.quoteID,
        isLimit,
        sell,
        qty: submitQty,
        rate: submitRate,
        tifNow: !isLimit,
        options: opts,
      });
      setConfirming(false);
      setQty('');
      setLotsStr('');
      setPrice('');
      setSpend('');
      setEst(null);
      setEstErr(null);
      setOpts({});
      onPlaced();
    } catch (e: any) {
      setErr(serverMsg(e));
      setBusy(false);
      setConfirming(false);
    }
  };

  // The native number spinner cannot be reliably spaced or repositioned across
  // browsers (e.g. Firefox ignores the WebKit pseudo-element), so it is hidden
  // and replaced with custom up/down buttons. Keyboard arrows still work via the
  // input's step/min.
  const field = (
    label: string,
    value: string,
    onChange: (v: string) => void,
    unit: string,
    opts2: { readOnly?: boolean; step?: number; min?: number; onBlur?: () => void; onStep?: (dir: number) => void } = {},
  ) => (
    <div>
      <label className="block text-[11px] text-muted-foreground mb-1">{label}</label>
      <div className="flex items-center rounded-lg bg-background border border-border/60 px-3 focus-within:border-primary/60 transition-colors">
        <input
          type="number"
          value={value}
          readOnly={opts2.readOnly}
          step={opts2.step}
          min={opts2.min}
          onChange={(e) => onChange(e.target.value)}
          onBlur={opts2.onBlur}
          className="flex-1 min-w-0 bg-transparent py-1.5 font-mono tabular-nums text-right outline-none read-only:text-muted-foreground [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
        />
        {opts2.onStep && (
          <div className="flex flex-col ml-2 shrink-0 text-muted-foreground">
            <button type="button" tabIndex={-1} aria-label="Increase" onClick={() => opts2.onStep!(1)} className="leading-none hover:text-foreground">
              <ChevronUp className="h-3 w-3" />
            </button>
            <button type="button" tabIndex={-1} aria-label="Decrease" onClick={() => opts2.onStep!(-1)} className="leading-none hover:text-foreground">
              <ChevronDown className="h-3 w-3" />
            </button>
          </div>
        )}
        <span className="text-[11px] text-muted-foreground ml-2 w-12 text-right shrink-0">{unit}</span>
      </div>
    </div>
  );

  return (
    <div className="flex flex-col min-h-0 h-full">
      <div className="grid grid-cols-2 border-b border-border/50 shrink-0">
        <button
          type="button"
          onClick={() => setSell(false)}
          className={`relative py-3 text-sm font-semibold transition-colors ${
            !sell ? 'text-success bg-card after:absolute after:inset-x-0 after:top-0 after:h-0.5 after:bg-success' : 'text-muted-foreground hover:text-foreground/80'
          }`}
        >
          Buy {market.base}
        </button>
        <button
          type="button"
          onClick={() => setSell(true)}
          className={`relative py-3 text-sm font-semibold transition-colors ${
            sell ? 'text-destructive bg-card after:absolute after:inset-x-0 after:top-0 after:h-0.5 after:bg-destructive' : 'text-muted-foreground hover:text-foreground/80'
          }`}
        >
          Sell {market.base}
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-3.5 space-y-2.5">
        <div className="flex gap-4 text-xs border-b border-border/50 pb-2">
          <button
            type="button"
            onClick={() => setIsLimit(true)}
            className={`relative pb-1 font-medium ${isLimit ? 'text-foreground after:absolute after:inset-x-0 after:-bottom-px after:h-0.5 after:bg-primary' : 'text-muted-foreground hover:text-foreground/80'}`}
          >
            Limit
          </button>
          <button
            type="button"
            onClick={() => setIsLimit(false)}
            className={`relative pb-1 font-medium ${!isLimit ? 'text-foreground after:absolute after:inset-x-0 after:-bottom-px after:h-0.5 after:bg-primary' : 'text-muted-foreground hover:text-foreground/80'}`}
          >
            Market
          </button>
        </div>

        {isLimit &&
          field('Price', price, setPrice, quoteSym, {
            step: rateStepConventional || undefined,
            min: rateStepConventional || undefined,
            onBlur: () => price && setPrice(fmtField(snapToStep(priceFloat))),
            onStep: (d) => setPrice(fmtField(snapToStep(Math.max((priceFloat || 0) + d * rateStepConventional, rateStepConventional)))),
          })}

        {isMarketBuy ? (
          field('Spend', spend, setSpend, quoteSym, { min: 0 })
        ) : (
          <>
            {field('Lots', lotsStr, setFromLots, 'lots', {
              step: 1,
              min: 1,
              onStep: (d) => setFromLots(String(Math.max(1, (parseInt(lotsStr || '0', 10) || 0) + d))),
            })}
            {field('Amount', qty, setFromQty, market.base, {
              step: lotConventional || undefined,
              min: lotConventional || undefined,
              onBlur: snapQtyBlur,
              onStep: (d) => {
                const s = snapToLot(Math.max((qtyFloat || 0) + d * lotConventional, lotConventional));
                setQty(fmtField(s));
                setLotsStr(s > 0 ? String(coinsToLots(s)) : '');
              },
            })}
            {maxLots != null && (
              <button
                type="button"
                onClick={() => setFromLots(String(maxLots))}
                className="text-[10px] text-muted-foreground hover:text-foreground transition-colors"
              >
                max {maxLots} lot{maxLots === 1 ? '' : 's'}
              </button>
            )}
          </>
        )}

        {(isMarketBuy ? spendFloat > 0 : lots >= 1) && (
          <div className="rounded-lg bg-background/40 border border-border/40 px-3 py-2 space-y-1 text-xs">
            <div className="flex justify-between">
              <span className="text-muted-foreground">You spend</span>
              <span className="font-mono tabular-nums">{fmtCell(spendVal, spendKnown, spendEst, spendSym)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">You receive</span>
              <span className="font-mono tabular-nums">{fmtCell(receiveVal, receiveKnown, receiveEst, receiveSym)}</span>
            </div>
            <div className="flex justify-between text-[10px] text-muted-foreground/70 pt-1 border-t border-border/30">
              <span>{isMarketBuy ? `~${mktBuyLots} lot${mktBuyLots === 1 ? '' : 's'}` : `${lots} lot${lots === 1 ? '' : 's'}`}</span>
              <span>lot size {fmtAmt(lotConventional, 8)} {baseSym}</span>
            </div>
          </div>
        )}

        {boolOpts.length > 0 && (
          <div className="border-t border-border/40 pt-2">
            <button
              type="button"
              onClick={() => setShowAdvanced((s) => !s)}
              className="flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
            >
              {showAdvanced ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
              Advanced options
            </button>
            {showAdvanced && (
              <div className="mt-2 space-y-2">
                {boolOpts.map((o) => (
                  <label key={o.key} className="flex items-start gap-2 text-[11px] cursor-pointer">
                    <input
                      type="checkbox"
                      checked={opts[o.key] === 'true'}
                      onChange={(e) => setOpts((prev) => ({ ...prev, [o.key]: e.target.checked ? 'true' : 'false' }))}
                      className="mt-0.5 accent-primary"
                    />
                    <span>
                      <span className="text-foreground">{o.displayname || o.key}</span>
                      {(o.boolean?.reason || o.description) && (
                        <span className="block text-muted-foreground/70">{o.boolean?.reason || o.description}</span>
                      )}
                    </span>
                  </label>
                ))}
              </div>
            )}
          </div>
        )}

        {err && (
          <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}

        {insufficient && <p className="text-[11px] text-destructive">Not enough funds available</p>}
        {!insufficient && estErr && orderReady && <p className="text-[11px] text-destructive break-words">{estErr}</p>}
        {!insufficient && !estErr && overMax && (
          <p className="text-[11px] text-warning">Exceeds the estimated max of {maxLots} lots; bisonw may reject it.</p>
        )}

        <div className="mt-auto pt-2 space-y-2">
          {!confirming ? (
            <button
              type="button"
              disabled={!valid}
              onClick={() => setConfirming(true)}
              className={`w-full py-3 rounded-lg font-semibold text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${
                sell ? 'bg-destructive hover:bg-destructive/90' : 'bg-success hover:bg-success/90'
              }`}
            >
              {preview ? 'Orders disabled in preview' : `${sell ? 'Sell' : 'Buy'} ${market.base}`}
            </button>
          ) : (
            <>
              <div className="p-2.5 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning flex items-start gap-2">
                <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
                <span>
                  {isMarketBuy ? (
                    <>Market buy {fmtCell(receiveVal, receiveKnown, receiveEst, receiveSym)} for {fmtAmt(spendFloat, 8)} {quoteSym}.</>
                  ) : (
                    <>
                      {isLimit ? 'Limit' : 'Market'} {sell ? 'sell' : 'buy'} {lots} lot{lots === 1 ? '' : 's'} ({fmtAmt(qtyEffective, 8)} {baseSym})
                      {isLimit ? ` @ ${fmtPrice(rateEffective, market.quote)} ${quoteSym}` : ''}, receive {fmtCell(receiveVal, receiveKnown, receiveEst, receiveSym)}.
                    </>
                  )}{' '}
                  Spends real funds.
                </span>
              </div>
              {est && (
                <div className="px-2.5 text-[11px] text-muted-foreground space-y-0.5">
                  <div className="flex justify-between">
                    <span>Est. swap fee</span>
                    <span className="font-mono tabular-nums">~{fmtAmt(swapFee, 8)} {fromSym}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>Est. redeem fee</span>
                    <span className="font-mono tabular-nums">~{fmtAmt(redeemFee, 8)} {toSym}</span>
                  </div>
                </div>
              )}
              {estErr && <p className="px-2.5 text-[11px] text-destructive break-words">{estErr}</p>}
              <div className="flex gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => setConfirming(false)}
                  className="flex-1 py-2 border border-border rounded-lg text-sm hover:bg-background/50 transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={submit}
                  className="flex-1 py-2 bg-gradient-primary text-white rounded-lg text-sm font-semibold disabled:opacity-50"
                >
                  {busy ? 'Placing…' : 'Confirm'}
                </button>
              </div>
            </>
          )}
          <p className="text-[10px] text-muted-foreground/70 text-center">Atomic swap · self-custody settlement</p>
        </div>
      </div>
    </div>
  );
};
