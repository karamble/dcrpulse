// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { getBisonrelayRates } from '../../services/bisonrelayApi';
import {
  LightningDecodedPayReq,
  decodeLnPayReq,
  lnFeeLimitAtoms,
  streamLnPayment,
} from '../../services/lightningApi';
import { formatAtomsTrimmed, toDcr } from '../../utils/amounts';
import { apiError } from '../../utils/apiError';

const fmtDcr = formatAtomsTrimmed;

// A confirmation and every payment status refer to this exact reviewed request.
interface ReviewedInvoice {
  readonly invoice: string;
  readonly decoded: Readonly<LightningDecodedPayReq>;
  readonly amountLabel: string;
}

type PaymentState = {
  request: ReviewedInvoice;
  status: 'paying' | 'paid' | 'failed' | 'unknown';
  error?: string;
};

// Keep payment tracking alive when remote content replaces an invoice. Only the
// review controls below are keyed by invoice; remounting the whole chip would
// close the websocket of an already submitted payment.
export const LnPayChip = ({ invoice }: { invoice: string }) => {
  const [payment, setPayment] = useState<PaymentState | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);
  const generationRef = useRef(0);
  const submittingRef = useRef(false);

  useEffect(() => () => {
    generationRef.current++;
    cleanupRef.current?.();
  }, []);

  const pay = (request: ReviewedInvoice) => {
    if (request.invoice !== invoice || submittingRef.current) return;
    submittingRef.current = true;
    const generation = ++generationRef.current;
    cleanupRef.current?.();
    cleanupRef.current = null;
    setPayment({ request, status: 'paying' });
    let settled = false;
    const finish = (status: PaymentState['status'], error?: string) => {
      if (generation !== generationRef.current || settled) return;
      settled = true;
      submittingRef.current = false;
      setPayment({ request, status, error });
    };
    try {
      cleanupRef.current = streamLnPayment(
        { payReq: request.invoice, feeLimitAtoms: lnFeeLimitAtoms(request.decoded.numAtoms) },
        (snap) => {
          if (snap.status === 'pending') return;
          if (snap.status === 'confirmed') finish('paid');
          else finish('failed', snap.failureReason || 'Payment failed');
        },
        // A transport error is not evidence that the node did not pay.
        (msg) => finish('unknown', msg),
        () => {
          if (generation !== generationRef.current) return;
          cleanupRef.current = null;
          finish('unknown');
        },
      );
    } catch (error) {
      finish('unknown', apiError(error, 'Lightning send connection failed'));
    }
  };

  const retry = () => {
    generationRef.current++;
    cleanupRef.current?.();
    cleanupRef.current = null;
    setPayment(null);
  };
  const canReview = !payment || (payment.status !== 'paying' && payment.request.invoice !== invoice);

  return (
    <>
      {payment && (
        <div className="my-2 rounded-lg border border-border/50 bg-background/40 p-3 text-sm space-y-2">
          <div className={payment.status === 'paid' ? 'font-semibold text-emerald-400' : 'text-muted-foreground'}>
            {payment.status === 'paying' ? 'Paying' : payment.status === 'paid' ? 'Paid' : 'Payment for'}{' '}
            {payment.request.amountLabel}
            {payment.request.decoded.description && <> for {payment.request.decoded.description}</>}
            {payment.status === 'paying' ? '…' : ''}
          </div>
          {payment.status === 'unknown' && (
            <div className="text-xs text-rose-300">Payment status unknown—check payment history.</div>
          )}
          {payment.error && <div className="text-xs text-rose-300 break-words">{payment.error}</div>}
          {payment.status === 'failed' && payment.request.invoice === invoice && (
            <button type="button" onClick={retry} className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm">
              Try again
            </button>
          )}
        </div>
      )}
      {canReview && <InvoiceReview key={invoice} invoice={invoice} onPay={pay} />}
    </>
  );
};

// Invoice identity is the reset boundary, including A → B → A. An effect-only
// reset would briefly render the previous decoded details and confirmation.
const InvoiceReview = ({ invoice, onPay }: { invoice: string; onPay: (request: ReviewedInvoice) => void }) => {
  const [decoded, setDecoded] = useState<LightningDecodedPayReq | null>(null);
  const [decodeErr, setDecodeErr] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState<ReviewedInvoice | null>(null);
  const [usd, setUsd] = useState<number | null>(null);

  // Decode on mount so the chip can show the amount before the user commits.
  useEffect(() => {
    let cancelled = false;
    decodeLnPayReq(invoice)
      .then((d) => {
        if (!cancelled) setDecoded(d);
      })
      .catch((e: any) => {
        if (!cancelled) {
          setDecodeErr(apiError(e, 'Could not decode invoice'));
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
        if (!cancelled && r.dcr_usd > 0) setUsd(toDcr(decoded.numAtoms) * r.dcr_usd);
      })
      .catch(() => {
        /* USD is best-effort */
      });
    return () => {
      cancelled = true;
    };
  }, [decoded]);

  const usdSuffix = usd != null ? ` (~$${usd < 0.01 ? usd.toFixed(4) : usd.toFixed(2)})` : '';
  const amtLabel = decoded ? `${fmtDcr(decoded.numAtoms)} DCR${usdSuffix}` : 'Lightning invoice';

  if (decodeErr) {
    return (
      <div className="my-2 rounded-lg border border-border/50 bg-background/40 p-2 text-xs text-rose-300 break-words">
        Invalid Lightning invoice: {decodeErr}
      </div>
    );
  }

  return (
    <div className="my-2 rounded-lg border border-border/50 bg-background/40 p-3 text-sm space-y-2">
      {confirmation ? (
        <div className="space-y-2">
          <div className="break-words">
            Pay <span className="font-semibold text-foreground">{confirmation.amountLabel}</span>
            {confirmation.decoded.description ? (
              <>
                {' '}
                for <span className="font-mono">{confirmation.decoded.description}</span>
              </>
            ) : null}
            ?
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => { if (confirmation.invoice === invoice) onPay(confirmation); }}
              className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
            >
              Pay now
            </button>
            <button
              type="button"
              onClick={() => setConfirmation(null)}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => {
            if (decoded) setConfirmation({ invoice, decoded: { ...decoded }, amountLabel: amtLabel });
          }}
          disabled={!decoded}
          className="max-w-full truncate px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors disabled:opacity-50"
        >
          Pay {amtLabel}
        </button>
      )}
    </div>
  );
};
