import { useEffect, useState } from 'react';
import { CheckCircle2, Copy, X } from 'lucide-react';
import type { LightningPayment } from '../../../services/lightningApi';

const atomsPerDcr = 1e8;
const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';
const trunc = (s: string, head = 12, tail = 8) =>
  s.length <= head + tail + 1 ? s : `${s.slice(0, head)}…${s.slice(-tail)}`;
const fmtDate = (sec: number) => {
  if (!sec) return '-';
  return new Date(sec * 1000).toLocaleString();
};

interface Props {
  payment: LightningPayment;
  onClose: () => void;
}

export const PaymentDetailsModal = ({ payment, onClose }: Props) => {
  const [copied, setCopied] = useState<string | null>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const copy = async (value: string, key: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(key);
      window.setTimeout(() => setCopied((c) => (c === key ? null : c)), 1200);
    } catch {
      /* clipboard unavailable */
    }
  };

  const Field = ({
    label,
    value,
    copyKey,
    copyValue,
    mono,
  }: {
    label: string;
    value: React.ReactNode;
    copyKey?: string;
    copyValue?: string;
    mono?: boolean;
  }) => (
    <div className="flex items-start justify-between gap-3 py-2 border-b border-border/40 last:border-b-0">
      <span className="text-xs uppercase tracking-wide text-muted-foreground shrink-0">
        {label}
      </span>
      <span className={`text-sm text-right break-all ${mono ? 'font-mono' : ''}`}>
        {value}
        {copyKey && copyValue !== undefined && (
          <button
            onClick={() => copy(copyValue, copyKey)}
            className="ml-2 inline-flex items-center text-muted-foreground hover:text-foreground"
            title="Copy"
            type="button"
          >
            {copied === copyKey ? (
              <CheckCircle2 className="h-3.5 w-3.5 text-success" />
            ) : (
              <Copy className="h-3.5 w-3.5" />
            )}
          </button>
        )}
      </span>
    </div>
  );

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg max-h-[85vh] overflow-y-auto rounded-xl bg-gradient-card border border-border/60 backdrop-blur-sm p-5 space-y-3"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Payment details</h3>
          <button
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground"
            type="button"
            title="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div>
          <Field label="Status" value={payment.status} />
          <Field label="Amount" value={fmtDcr(payment.valueAtoms)} />
          <Field label="Fee" value={fmtDcr(payment.feeAtoms)} />
          <Field label="Date" value={fmtDate(payment.creationDate)} />
          <Field
            label="Hash"
            value={trunc(payment.paymentHash)}
            copyKey="hash"
            copyValue={payment.paymentHash}
            mono
          />
          {payment.destination && (
            <Field
              label="Destination"
              value={trunc(payment.destination)}
              copyKey="dest"
              copyValue={payment.destination}
              mono
            />
          )}
          {payment.description && <Field label="Description" value={payment.description} />}
          {payment.paymentPreimage && (
            <Field
              label="Preimage"
              value={trunc(payment.paymentPreimage)}
              copyKey="preimage"
              copyValue={payment.paymentPreimage}
              mono
            />
          )}
          {payment.failureReason && (
            <Field
              label="Failure"
              value={<span className="text-destructive">{payment.failureReason}</span>}
            />
          )}
        </div>

        {payment.htlcs && payment.htlcs.length > 0 && (
          <div className="pt-2">
            <div className="text-xs uppercase tracking-wide text-muted-foreground mb-2">
              HTLC attempts
            </div>
            <div className="space-y-3">
              {payment.htlcs.map((htlc, i) => (
                <div
                  key={i}
                  className="p-3 rounded-lg bg-background/40 border border-border/60 space-y-2"
                >
                  <div className="text-xs flex items-center justify-between">
                    <span className="text-muted-foreground">HTLC #{i + 1}</span>
                    <span>{htlc.status}</span>
                  </div>
                  <div className="text-xs text-muted-foreground">
                    Total {fmtDcr(htlc.totalAmt)} · Fees {fmtDcr(htlc.totalFees)}
                  </div>
                  {htlc.hops && htlc.hops.length > 0 && (
                    <ol className="text-xs space-y-1 list-decimal list-inside text-foreground/80">
                      {htlc.hops.map((hop, j) => (
                        <li key={j} className="font-mono">
                          {trunc(hop.pubKey, 8, 6)} <span className="text-muted-foreground">
                            fee {fmtDcr(hop.feeAtoms)}
                          </span>
                        </li>
                      ))}
                    </ol>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
