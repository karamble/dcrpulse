import { ArrowUpRight, CheckCircle2, Loader2, XCircle } from 'lucide-react';
import { toYMDTime } from '../../../utils/date';
import type { LightningPayment } from '../../../services/lightningApi';
import { StatusPill, StatusTone } from '../StatusPill';

const atomsPerDcr = 1e8;
const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';
const truncHash = (s: string) => (s.length <= 18 ? s : `${s.slice(0, 10)}…${s.slice(-6)}`);

const fmtDate = (sec: number) => {
  if (!sec) return '-';
  const d = new Date(sec * 1000);
  return toYMDTime(d);
};

const paymentPill = (status: LightningPayment['status']): { label: string; tone: StatusTone; icon: JSX.Element } => {
  if (status === 'confirmed') {
    return { label: 'Confirmed', tone: 'success', icon: <CheckCircle2 className="h-3 w-3" /> };
  }
  if (status === 'failed') {
    return { label: 'Failed', tone: 'destructive', icon: <XCircle className="h-3 w-3" /> };
  }
  return { label: 'Pending', tone: 'warning', icon: <Loader2 className="h-3 w-3 animate-spin" /> };
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
      {(() => {
        const { label, tone, icon } = paymentPill(payment.status);
        return <StatusPill label={label} tone={tone} icon={icon} />;
      })()}
      <span className="text-xs text-muted-foreground">{fmtDate(payment.creationDate)}</span>
    </div>
  </button>
);
