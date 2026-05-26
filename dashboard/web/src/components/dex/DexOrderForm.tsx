// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, AlertTriangle } from 'lucide-react';
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
  onPlaced: () => void;
}

export const DexOrderForm = ({ host, market, preview = false, onPlaced }: DexOrderFormProps) => {
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

  const field = (label: string, value: string, onChange: (v: string) => void, unit: string, readOnly = false) => (
    <div>
      <label className="block text-[11px] text-muted-foreground mb-1">{label}</label>
      <div className="flex items-center rounded-lg bg-background border border-border/60 px-3 focus-within:border-primary/60 transition-colors">
        <input
          type="number"
          value={value}
          readOnly={readOnly}
          onChange={(e) => onChange(e.target.value)}
          className="flex-1 bg-transparent py-1.5 font-mono tabular-nums text-right outline-none read-only:text-muted-foreground"
        />
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

        {isLimit && field('Price', price, setPrice, market.quote.split('.')[0])}
        {field('Amount', qty, setQty, market.base)}
        {qty !== '' && (
          <p className="text-[11px] text-muted-foreground">
            {lots} lot(s) = {qtyEffective} {market.base} · lot size {lotConventional}
          </p>
        )}
        {isLimit && field('Total', total ? String(Number(total.toFixed(8))) : '', () => {}, market.quote.split('.')[0], true)}

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
