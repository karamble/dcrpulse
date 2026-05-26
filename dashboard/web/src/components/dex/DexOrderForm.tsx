// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, AlertTriangle } from 'lucide-react';
import { placeDexOrder, type DexMarket } from '../../services/dcrdexApi';

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

  const sideBtn = (isSell: boolean, label: string) => (
    <button
      type="button"
      onClick={() => setSell(isSell)}
      className={`flex-1 py-1.5 rounded-lg text-sm font-semibold transition-colors ${
        sell === isSell
          ? isSell
            ? 'bg-destructive/20 text-destructive border border-destructive/40'
            : 'bg-success/20 text-success border border-success/40'
          : 'bg-muted/10 text-muted-foreground border border-border/50 hover:bg-muted/20'
      }`}
    >
      {label}
    </button>
  );

  return (
    <div className="rounded-xl bg-gradient-card border border-border/50 p-4 space-y-3 w-full sm:w-72 shrink-0">
      <div className="flex gap-2">
        {sideBtn(false, 'Buy')}
        {sideBtn(true, 'Sell')}
      </div>

      <div className="flex gap-2 text-xs">
        <button
          type="button"
          onClick={() => setIsLimit(true)}
          className={`flex-1 py-1 rounded ${isLimit ? 'bg-primary/20 text-primary' : 'text-muted-foreground hover:bg-muted/10'}`}
        >
          Limit
        </button>
        <button
          type="button"
          onClick={() => setIsLimit(false)}
          className={`flex-1 py-1 rounded ${!isLimit ? 'bg-primary/20 text-primary' : 'text-muted-foreground hover:bg-muted/10'}`}
        >
          Market
        </button>
      </div>

      {isLimit && (
        <div>
          <label className="block text-xs text-muted-foreground mb-1">Price ({market.quote})</label>
          <input
            type="number"
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
          />
        </div>
      )}

      <div>
        <label className="block text-xs text-muted-foreground mb-1">
          Quantity ({market.base}) · lot {lotConventional}
        </label>
        <input
          type="number"
          value={qty}
          onChange={(e) => setQty(e.target.value)}
          className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
        />
        {qty !== '' && (
          <p className="text-xs text-muted-foreground mt-1">
            {lots} lot(s) = {qtyEffective} {market.base}
          </p>
        )}
      </div>

      {err && (
        <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      {!confirming ? (
        <button
          type="button"
          disabled={!valid}
          onClick={() => setConfirming(true)}
          className={`w-full py-2.5 rounded-lg font-semibold text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
            sell ? 'bg-destructive hover:bg-destructive/90' : 'bg-success hover:bg-success/90'
          }`}
        >
          {sell ? 'Sell' : 'Buy'} {market.base}
        </button>
      ) : (
        <div className="space-y-2">
          <div className="p-2.5 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning flex items-start gap-2">
            <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>
              {isLimit ? 'Limit' : 'Market'} {sell ? 'sell' : 'buy'} {qtyEffective} {market.base}
              {isLimit ? ` @ ${rateEffective} ${market.quote}` : ''}. Spends real funds.
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
        </div>
      )}
    </div>
  );
};
