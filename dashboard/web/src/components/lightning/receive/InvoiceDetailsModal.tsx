import { useEffect, useState } from 'react';
import { toYMDTime } from '../../../utils/date';
import { CheckCircle2, Copy, X } from 'lucide-react';
import type { LightningInvoice } from '../../../services/lightningApi';
import { cancelLnInvoice } from '../../../services/lightningApi';

const atomsPerDcr = 1e8;
const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';
const trunc = (s: string, head = 12, tail = 8) =>
  s.length <= head + tail + 1 ? s : `${s.slice(0, head)}…${s.slice(-tail)}`;
const fmtDate = (sec: number) => (!sec ? '-' : toYMDTime(new Date(sec * 1000)));

const fmtExpiry = (creation: number, expiry: number): string => {
  if (!creation || !expiry) return '-';
  return toYMDTime(new Date((creation + expiry) * 1000));
};

interface Props {
  invoice: LightningInvoice;
  onClose: () => void;
  onCanceled?: () => void;
}

export const InvoiceDetailsModal = ({ invoice, onClose, onCanceled }: Props) => {
  const [copied, setCopied] = useState<string | null>(null);
  const [canceling, setCanceling] = useState(false);
  const [cancelError, setCancelError] = useState<string | null>(null);

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

  const onCancel = async () => {
    setCanceling(true);
    setCancelError(null);
    try {
      await cancelLnInvoice(invoice.rHashHex);
      onCanceled?.();
    } catch (err: any) {
      const body = err?.response?.data;
      setCancelError(typeof body === 'string' ? body : err?.message || 'Cancel failed');
    } finally {
      setCanceling(false);
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
      <span className="text-xs uppercase tracking-wide text-muted-foreground shrink-0">{label}</span>
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
          <h3 className="text-lg font-semibold">Lightning invoice</h3>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground" type="button" title="Close">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div>
          <Field label="Status" value={invoice.status} />
          <Field
            label="Requested"
            value={invoice.valueAtoms > 0 ? fmtDcr(invoice.valueAtoms) : 'Open amount'}
          />
          {invoice.amtPaidAtoms > 0 && (
            <Field label="Received" value={fmtDcr(invoice.amtPaidAtoms)} />
          )}
          <Field label="Created" value={fmtDate(invoice.creationDate)} />
          {invoice.status === 'settled' && invoice.settleDate ? (
            <Field label="Settled" value={fmtDate(invoice.settleDate)} />
          ) : (
            <Field label="Expires" value={fmtExpiry(invoice.creationDate, invoice.expiry)} />
          )}
          {invoice.memo && <Field label="Memo" value={invoice.memo} />}
          <Field
            label="Hash"
            value={trunc(invoice.rHashHex)}
            copyKey="hash"
            copyValue={invoice.rHashHex}
            mono
          />
        </div>

        <div>
          <div className="text-xs uppercase tracking-wide text-muted-foreground mb-1">
            Payment request
          </div>
          <div className="p-3 rounded-lg bg-background/40 border border-border/60 break-all font-mono text-xs">
            {invoice.paymentRequest}
          </div>
          <div className="flex justify-end mt-2">
            <button
              type="button"
              onClick={() => copy(invoice.paymentRequest, 'pr')}
              className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
            >
              {copied === 'pr' ? (
                <>
                  <CheckCircle2 className="h-3.5 w-3.5 text-success" /> Copied
                </>
              ) : (
                <>
                  <Copy className="h-3.5 w-3.5" /> Copy
                </>
              )}
            </button>
          </div>
        </div>

        {cancelError && <div className="text-sm text-destructive">{cancelError}</div>}

        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onCancel}
            disabled={canceling || invoice.status !== 'open'}
            className="px-3 py-1.5 rounded-lg bg-destructive/15 text-destructive text-sm hover:bg-destructive/25 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {canceling ? 'Canceling…' : 'Cancel invoice'}
          </button>
        </div>
      </div>
    </div>
  );
};
