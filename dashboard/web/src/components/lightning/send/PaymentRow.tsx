import { ArrowUpRight, CheckCircle2, Loader2, XCircle } from 'lucide-react';
import type { LightningPayment } from '../../../services/lightningApi';

const atomsPerDcr = 1e8;
const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';
const truncHash = (s: string) => (s.length <= 18 ? s : `${s.slice(0, 10)}…${s.slice(-6)}`);

const fmtDate = (sec: number) => {
  if (!sec) return '-';
  const d = new Date(sec * 1000);
  const date = d.toLocaleDateString();
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  return `${date} ${time}`;
};

const StatusPill = ({ status }: { status: LightningPayment['status'] }) => {
  if (status === 'confirmed') {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-success/15 text-success text-xs">
        <CheckCircle2 className="h-3 w-3" /> Confirmed
      </span>
    );
  }
  if (status === 'failed') {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-destructive/15 text-destructive text-xs">
        <XCircle className="h-3 w-3" /> Failed
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-warning/15 text-warning text-xs">
      <Loader2 className="h-3 w-3 animate-spin" /> Pending
    </span>
  );
};

interface Props {
  payment: LightningPayment;
  onClick: () => void;
}

export const PaymentRow = ({ payment, onClick }: Props) => (
  <button
    type="button"
    onClick={onClick}
    className="w-full text-left px-3 py-2 rounded-lg bg-background/40 border border-border/60 hover:border-primary/50 hover:bg-background/60 transition-colors flex items-center justify-between gap-3"
  >
    <div className="flex items-center gap-3 min-w-0">
      <div className="p-1.5 rounded-md bg-primary/10 text-primary shrink-0">
        <ArrowUpRight className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <div className="text-sm font-medium truncate">Sent {fmtDcr(payment.valueAtoms)}</div>
        <div className="text-xs text-muted-foreground font-mono truncate">
          {truncHash(payment.paymentHash)}
        </div>
      </div>
    </div>
    <div className="flex flex-col items-end gap-1 shrink-0">
      <StatusPill status={payment.status} />
      <span className="text-xs text-muted-foreground">{fmtDate(payment.creationDate)}</span>
    </div>
  </button>
);
