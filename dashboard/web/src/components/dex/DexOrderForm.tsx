// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, AlertTriangle, ChevronDown, ChevronUp } from 'lucide-react';
import { placeDexOrder, type DexMarket } from '../../services/dcrdexApi';
import { fmtPrice } from './dexFormat';

// RateEncodingFactor mirrors bisonw's OrderUtil.RateEncodingFactor: the DEX
// message rate is the conventional price scaled by 1e8 and adjusted by the
// base/quote conversion factors. This conversion is mirrored 1:1 from bisonw's
// own frontend (client/webserver markets.ts) for the funds-critical encoding.
const RateEncodingFactor = 1e8;

interface DexOrderFormProps {
  host: string;
  market: DexMarket;
  preview?: boolean;
  // pick prefills the form from a clicked order book level (price + size) and
  // takes that level's side; seq lets a repeat click re-apply.
  pick?: { rate: number; qty: number; sell: boolean; seq: number } | null;
  onPlaced: () => void;
}

export const DexOrderForm = ({ host, market, preview = false, pick, onPlaced }: DexOrderFormProps) => {
  const [sell, setSell] = useState(false);
  const [isLimit, setIsLimit] = useState(true);
  const [price, setPrice] = useState('');
  const [qty, setQty] = useState('');
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const lotConventional = market.lotSize / market.baseConvFactor;
  // rateConversionFactor: msgRate = price * (1e8 / baseConv * quoteConv).
  const rateConversionFactor = (RateEncodingFactor / market.baseConvFactor) * market.quoteConvFactor;
  const rateStepConventional = market.rateStep > 0 ? market.rateStep / rateConversionFactor : 0;

  const fmtField = (n: number) => (n > 0 ? String(Number(n.toFixed(8))) : '');
  // Snap an amount to the nearest whole lot (min 1 lot) and a price to the
  // nearest rate step (min 1 step), both in conventional units.
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

  // Prefill from a clicked order book level: take the level's side (ask -> buy,
  // bid -> sell), set its price and size, snapped to the market increments.
  useEffect(() => {
    if (!pick) return;
    setIsLimit(true);
    setSell(!pick.sell);
    setPrice(fmtField(snapToStep(pick.rate)));
    setQty(fmtField(snapToLot(pick.qty)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pick?.seq]);

  const qtyFloat = parseFloat(qty || '0');
  const priceFloat = parseFloat(price || '0');
  // Snap quantity down to a whole number of lots.
  const lots = market.lotSize > 0 ? Math.floor(Math.round(qtyFloat * market.baseConvFactor) / market.lotSize) : 0;
  const qtyAtomic = lots * market.lotSize;
  const qtyEffective = qtyAtomic / market.baseConvFactor;
  // Snap rate to a multiple of the market rate step.
  const rawMsgRate = Math.round(priceFloat * rateConversionFactor);
  const msgRate = market.rateStep > 0 ? Math.round(rawMsgRate / market.rateStep) * market.rateStep : rawMsgRate;
  const rateEffective = msgRate / rateConversionFactor;
  const total = isLimit ? rateEffective * qtyEffective : 0;

  const valid = !preview && lots >= 1 && (!isLimit || msgRate > 0);

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
        qty: qtyAtomic,
        rate: isLimit ? msgRate : 0,
        tifNow: !isLimit,
      });
      setConfirming(false);
      setQty('');
      setPrice('');
      onPlaced();
    } catch (e: any) {
      setErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Order failed');
      setBusy(false);
      setConfirming(false);
    }
  };

  // The native number spinner cannot be reliably spaced or repositioned across
  // browsers (e.g. Firefox ignores the WebKit pseudo-element), so it is hidden
  // and replaced with custom up/down buttons that step by one lot / rate step
  // and keep a clear gap from the right-aligned value. Keyboard arrows still
  // work via the input's step/min.
  const field = (
    label: string,
    value: string,
    onChange: (v: string) => void,
    unit: string,
    opts: { readOnly?: boolean; step?: number; min?: number; onBlur?: () => void; onStep?: (dir: number) => void } = {},
  ) => (
    <div>
      <label className="block text-[11px] text-muted-foreground mb-1">{label}</label>
      <div className="flex items-center rounded-lg bg-background border border-border/60 px-3 focus-within:border-primary/60 transition-colors">
        <input
          type="number"
          value={value}
          readOnly={opts.readOnly}
          step={opts.step}
          min={opts.min}
          onChange={(e) => onChange(e.target.value)}
          onBlur={opts.onBlur}
          className="flex-1 min-w-0 bg-transparent py-1.5 font-mono tabular-nums text-right outline-none read-only:text-muted-foreground [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
        />
        {opts.onStep && (
          <div className="flex flex-col ml-2 shrink-0 text-muted-foreground">
            <button type="button" tabIndex={-1} aria-label="Increase" onClick={() => opts.onStep!(1)} className="leading-none hover:text-foreground">
              <ChevronUp className="h-3 w-3" />
            </button>
            <button type="button" tabIndex={-1} aria-label="Decrease" onClick={() => opts.onStep!(-1)} className="leading-none hover:text-foreground">
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
          field('Price', price, setPrice, market.quote.split('.')[0], {
            step: rateStepConventional || undefined,
            min: rateStepConventional || undefined,
            onBlur: () => price && setPrice(fmtField(snapToStep(priceFloat))),
            onStep: (d) => setPrice(fmtField(snapToStep(Math.max((priceFloat || 0) + d * rateStepConventional, rateStepConventional)))),
          })}
        {field('Amount', qty, setQty, market.base, {
          step: lotConventional || undefined,
          min: lotConventional || undefined,
          onBlur: () => qty && setQty(fmtField(snapToLot(qtyFloat))),
          onStep: (d) => setQty(fmtField(snapToLot(Math.max((qtyFloat || 0) + d * lotConventional, lotConventional)))),
        })}
        {qty !== '' && (
          <p className="text-[11px] text-muted-foreground">
            {lots} lot(s) = {qtyEffective} {market.base} · lot size {lotConventional}
          </p>
        )}
        {isLimit && field('Total', total ? String(Number(total.toFixed(8))) : '', () => {}, market.quote.split('.')[0], { readOnly: true })}

        {err && (
          <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
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
                  {isLimit ? 'Limit' : 'Market'} {sell ? 'sell' : 'buy'} {qtyEffective} {market.base}
                  {isLimit ? ` @ ${fmtPrice(rateEffective, market.quote)} ${market.quote.split('.')[0]}` : ''}. Spends real funds.
                </span>
              </div>
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
