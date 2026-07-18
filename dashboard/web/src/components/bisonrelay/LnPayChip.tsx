// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { getBisonrelayRates } from '../../services/bisonrelayApi';
import {
  LightningDecodedPayReq,
  decodeLnPayReq,
  streamLnPayment,
} from '../../services/lightningApi';

const fmtDcr = (atoms: number): string => (atoms / 1e8).toFixed(8).replace(/\.?0+$/, '');

// LnPayChip renders an lnpay://<bolt11> link found in BR page/post content as a
// pay chip with an explicit confirm - matching how bruig/brclient turn an
// lnpay:// URL into a Pay action. It decodes the invoice to show the amount (DCR
// + approximate USD), and on confirm pays it over the wallet's Lightning node
// through the streaming send endpoint the Send tab already uses.
export const LnPayChip = ({ invoice }: { invoice: string }) => {
  const [decoded, setDecoded] = useState<LightningDecodedPayReq | null>(null);
  const [decodeErr, setDecodeErr] = useState<string | null>(null);
  const [phase, setPhase] = useState<'idle' | 'confirm' | 'paying' | 'paid' | 'error'>('idle');
  const [err, setErr] = useState<string | null>(null);
  const [usd, setUsd] = useState<number | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);

  // Decode on mount so the chip can show the amount before the user commits.
  useEffect(() => {
    let cancelled = false;
    decodeLnPayReq(invoice)
      .then((d) => {
        if (!cancelled) setDecoded(d);
      })
      .catch((e: any) => {
        if (!cancelled) {
          const body = e?.response?.data;
          setDecodeErr(typeof body === 'string' ? body : e?.message || 'Could not decode invoice');
        }
      });
    return () => {
      cancelled = true;
    };
  }, [invoice]);

  // Approximate USD (best-effort; DCR always shows).
  useEffect(() => {
    if (!decoded || decoded.numAtoms <= 0) return undefined;
    let cancelled = false;
    getBisonrelayRates()
      .then((r) => {
        if (!cancelled && r.dcr_usd > 0) setUsd((decoded.numAtoms / 1e8) * r.dcr_usd);
      })
      .catch(() => {
        /* USD is best-effort */
      });
    return () => {
      cancelled = true;
    };
  }, [decoded]);

  // Tear down the payment websocket if the chip unmounts mid-flight.
  useEffect(() => () => cleanupRef.current?.(), []);

  const usdSuffix = usd != null ? ` (~$${usd < 0.01 ? usd.toFixed(4) : usd.toFixed(2)})` : '';
  const amtLabel = decoded ? `${fmtDcr(decoded.numAtoms)} DCR${usdSuffix}` : 'Lightning invoice';

  const pay = () => {
    setErr(null);
    setPhase('paying');
    cleanupRef.current = streamLnPayment(
      { payReq: invoice },
      (snap) => {
        if (snap.status === 'pending') return;
        if (snap.status === 'confirmed') {
          setPhase('paid');
        } else {
          setErr(snap.failureReason || 'Payment failed');
          setPhase('error');
        }
      },
      (msg) => {
        setErr(msg);
        setPhase('error');
      },
      () => {
        cleanupRef.current = null;
      },
    );
  };

  if (decodeErr) {
    return (
      <div className="my-2 rounded-lg border border-border/50 bg-background/40 p-2 text-xs text-rose-300 break-words">
        Invalid Lightning invoice: {decodeErr}
      </div>
    );
  }

  return (
    <div className="my-2 rounded-lg border border-border/50 bg-background/40 p-3 text-sm space-y-2">
      {phase === 'paid' ? (
        <div className="font-semibold text-emerald-400">Paid {amtLabel}</div>
      ) : phase === 'confirm' ? (
        <div className="space-y-2">
          <div className="break-words">
            Pay <span className="font-semibold text-foreground">{amtLabel}</span>
            {decoded?.description ? (
              <>
                {' '}
                for <span className="font-mono">{decoded.description}</span>
              </>
            ) : null}
            ?
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={pay}
              className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
            >
              Pay now
            </button>
            <button
              type="button"
              onClick={() => setPhase('idle')}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : phase === 'paying' ? (
        <div className="text-muted-foreground">Paying {amtLabel}…</div>
      ) : phase === 'error' ? (
        <div className="space-y-2">
          <div className="text-xs text-rose-300 break-words">{err}</div>
          <button
            type="button"
            onClick={() => setPhase('idle')}
            className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
          >
            Try again
          </button>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setPhase('confirm')}
          disabled={!decoded}
          className="max-w-full truncate px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors disabled:opacity-50"
        >
          Pay {amtLabel}
        </button>
      )}
    </div>
  );
};
