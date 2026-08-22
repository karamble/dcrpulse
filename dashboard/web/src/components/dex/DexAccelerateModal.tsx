// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Loader2, Rocket, X } from 'lucide-react';
import {
  DexOrderFull,
  DexPreAccelerate,
  accelerateDexOrder,
  dexAccelerationEstimate,
  preAccelerateDexOrder,
} from '../../services/dcrdexApi';
import { dexCoinExplorer } from './dexExplorers';
import { convQty, fmtAmt } from './dexFormat';
import { apiError } from '../../utils/apiError';

// DexAccelerateModal lifts an order's stuck swap chain by broadcasting a child
// transaction that pays for its parents. Mirrors bisonw's accelerate form: the
// daemon supplies the rate range and the cost estimate, and the slider starts
// at the cheapest rate that improves on the current one. Spending is behind a
// two-step confirmation, as elsewhere in the wallet.
export const DexAccelerateModal = ({
  order,
  fromSym,
  fromConv,
  onClose,
  onAccelerated,
}: {
  order: DexOrderFull;
  // The asset the order pays with, which is the one the fee comes out of.
  fromSym: string;
  fromConv: number;
  onClose: () => void;
  onAccelerated: () => void;
}) => {
  const [pre, setPre] = useState<DexPreAccelerate | null>(null);
  const [preErr, setPreErr] = useState<string | null>(null);
  const [rate, setRate] = useState(0);
  const [fee, setFee] = useState<number | null>(null);
  const [feeErr, setFeeErr] = useState<string | null>(null);
  const [estimating, setEstimating] = useState(false);
  const [early, setEarly] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [appPass, setAppPass] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [txID, setTxID] = useState<string | null>(null);

  const fromAssetID = order.sell ? order.baseID : order.quoteID;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !busy) onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [busy, onClose]);

  useEffect(() => {
    let cancelled = false;
    preAccelerateDexOrder(order.id)
      .then((p) => {
        if (cancelled) return;
        setPre(p);
        setRate(Math.round(p.suggestedRange.start.y));
      })
      .catch((e: any) => {
        if (!cancelled) setPreErr(apiError(e, 'Could not read the acceleration range'));
      });
    return () => {
      cancelled = true;
    };
  }, [order.id]);

  // The estimate costs a round trip, so it runs when the slider is released
  // rather than on every step, which is bisonw's behaviour too.
  const estimate = (newRate: number) => {
    setEstimating(true);
    setFeeErr(null);
    dexAccelerationEstimate(order.id, newRate)
      .then(setFee)
      .catch((e: any) => setFeeErr(apiError(e, 'Could not estimate the fee')))
      .finally(() => setEstimating(false));
  };

  useEffect(() => {
    if (rate > 0) estimate(rate);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pre]);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      setTxID(await accelerateDexOrder(order.id, rate, appPass));
      setAppPass('');
      onAccelerated();
    } catch (e: any) {
      setErr(apiError(e, 'Acceleration failed'));
      setAppPass('');
      setConfirming(false);
      setEarly(false);
    } finally {
      setBusy(false);
    }
  };

  // x is the multiple of the chain's current effective rate; the daemon's range
  // is linear through the origin, so it divides out exactly.
  const multiple = pre && pre.swapRate > 0 ? rate / pre.swapRate : 0;
  const unit = pre?.suggestedRange.yUnit || '';
  const explorer = txID ? dexCoinExplorer(fromAssetID, txID) : null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4"
      onClick={() => {
        if (!busy) onClose();
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md max-h-[85vh] overflow-y-auto rounded-xl bg-card border border-border/50 shadow-xl"
      >
        <div className="flex items-start justify-between p-5 pb-3 gap-3">
          <div className="min-w-0">
            <h3 className="flex items-center gap-2 text-base font-semibold text-foreground">
              <Rocket className="h-4 w-4 text-primary" />
              Accelerate order
            </h3>
            <p className="text-xs text-muted-foreground mt-1">
              Sends a follow-up transaction that pays a higher fee on your own swap change, lifting
              the effective rate of every unconfirmed swap in this order. It helps when the rate has
              fallen below what is being mined, not when blocks are simply slow.
            </p>
          </div>
          {!busy && (
            <button
              type="button"
              onClick={onClose}
              aria-label="Close"
              className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>

        {preErr && (
          <div className="px-5 pb-5 flex items-start gap-2 text-xs text-destructive">
            <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span className="break-words">{preErr}</span>
          </div>
        )}

        {txID && (
          <div className="px-5 pb-5 space-y-2">
            <p className="text-sm text-foreground">Acceleration submitted.</p>
            {explorer ? (
              <a
                href={explorer}
                target="_blank"
                rel="noreferrer"
                className="block font-mono text-[11px] text-primary underline break-all"
              >
                {txID}
              </a>
            ) : (
              <p className="font-mono text-[11px] text-muted-foreground break-all">{txID}</p>
            )}
            <div className="flex justify-end pt-2">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition"
              >
                Done
              </button>
            </div>
          </div>
        )}

        {!pre && !preErr && !txID && (
          <div className="px-5 pb-5 flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin shrink-0" />
            <span>Reading the acceleration range…</span>
          </div>
        )}

        {pre && !txID && early && (
          <div className="px-5 pb-5 space-y-4">
            <div className="rounded-md bg-warning/10 border border-warning/40 p-3 text-xs text-foreground">
              {pre.earlyAcceleration?.wasAccelerated
                ? `You accelerated this order only ${Math.floor((pre.earlyAcceleration?.timePast ?? 0) / 60)} minutes ago.`
                : `Your oldest unconfirmed swap was submitted only ${Math.floor((pre.earlyAcceleration?.timePast ?? 0) / 60)} minutes ago.`}{' '}
              Accelerating again will not harm the order, but it may waste money. It only helps if
              the rate is too low to be mined, not if blocks are just slow. Check a block explorer
              first.
            </div>
            <div className="flex justify-between gap-2">
              <button
                type="button"
                onClick={() => setEarly(false)}
                className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
              >
                Back
              </button>
              <button
                type="button"
                onClick={() => {
                  setEarly(false);
                  setConfirming(true);
                }}
                className="px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition"
              >
                Continue anyway
              </button>
            </div>
          </div>
        )}

        {pre && !txID && !early && (
          <>
            <div className="px-5 space-y-1 text-xs">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Effective swap fee rate</span>
                <span className="text-foreground tabular-nums">
                  {pre.swapRate} {unit}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Suggested network rate</span>
                <span className="text-foreground tabular-nums">
                  {pre.suggestedRate} {unit}
                </span>
              </div>
            </div>

            <div className="px-5 pt-4">
              <label htmlFor="dex-accel-rate" className="block text-xs text-muted-foreground mb-1">
                New effective rate
              </label>
              <input
                id="dex-accel-rate"
                type="range"
                min={Math.round(pre.suggestedRange.start.y)}
                max={Math.round(pre.suggestedRange.end.y)}
                step={1}
                value={rate}
                disabled={confirming || busy}
                onChange={(e) => setRate(Number(e.target.value))}
                onMouseUp={() => estimate(rate)}
                onTouchEnd={() => estimate(rate)}
                onKeyUp={() => estimate(rate)}
                className="w-full accent-[hsl(var(--primary))] disabled:opacity-50"
              />
              <div className="flex justify-between text-[10px] text-muted-foreground tabular-nums">
                <span>{pre.suggestedRange.start.label}</span>
                <span className="text-foreground">
                  {rate} {unit} · {multiple.toFixed(2)}
                  {pre.suggestedRange.xUnit}
                </span>
                <span>{pre.suggestedRange.end.label}</span>
              </div>
            </div>

            <div className="px-5 pt-3 text-xs">
              {feeErr ? (
                <p className="text-destructive">{feeErr}</p>
              ) : estimating ? (
                <p className="text-muted-foreground">Estimating…</p>
              ) : fee !== null ? (
                <p className="text-muted-foreground">
                  Raising the effective rate to {rate} {unit} costs{' '}
                  <span className="text-foreground">
                    {fmtAmt(convQty(fee, fromConv), 8)} {fromSym}
                  </span>
                  .
                </p>
              ) : null}
            </div>

            {err && (
              <div className="px-5 pt-3 flex items-start gap-2 text-xs text-destructive">
                <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                <span className="break-words">{err}</span>
              </div>
            )}

            {confirming ? (
              <div className="px-5 pt-4 space-y-3">
                <p className="text-xs text-foreground">
                  Raising the effective rate to {rate} {unit}
                  {fee !== null ? ` costs about ${fmtAmt(convQty(fee, fromConv), 8)} ${fromSym}` : ''}.
                  This spends from your {fromSym} wallet and cannot be undone.
                </p>
                <input
                  type="password"
                  value={appPass}
                  onChange={(e) => setAppPass(e.target.value)}
                  placeholder="DCRDEX app password"
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
                />
                <div className="flex justify-end gap-2 pb-5">
                  <button
                    type="button"
                    onClick={() => {
                      setConfirming(false);
                      setAppPass('');
                    }}
                    className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm text-foreground transition-colors"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    onClick={submit}
                    disabled={busy || !appPass}
                    className="px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
                  >
                    {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                    Accelerate
                  </button>
                </div>
              </div>
            ) : (
              <div className="flex justify-end gap-2 p-5 pt-4">
                <button
                  type="button"
                  onClick={onClose}
                  className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm text-foreground transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={() => (pre.earlyAcceleration ? setEarly(true) : setConfirming(true))}
                  disabled={estimating}
                  className="px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Continue
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
};
