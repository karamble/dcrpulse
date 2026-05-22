import { ArrowDownLeft, CheckCircle2, Clock, Loader2, XCircle } from 'lucide-react';
import type { LightningInvoice, LightningInvoiceStatus } from '../../../services/lightningApi';
import { StatusPill, StatusTone } from '../StatusPill';

const atomsPerDcr = 1e8;
const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';
const truncHash = (s: string) => (s.length <= 18 ? s : `${s.slice(0, 10)}…${s.slice(-6)}`);

const fmtDate = (sec: number) => {
  if (!sec) return '-';
  const d = new Date(sec * 1000);
  return `${d.toLocaleDateString()} ${d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
};

const invoicePill = (status: LightningInvoiceStatus): { label: string; tone: StatusTone; icon: JSX.Element } => {
  switch (status) {
    case 'settled':
      return { label: 'Received', tone: 'success', icon: <CheckCircle2 className="h-3 w-3" /> };
    case 'expired':
      return { label: 'Expired', tone: 'muted', icon: <Clock className="h-3 w-3" /> };
    case 'canceled':
      return { label: 'Canceled', tone: 'destructive', icon: <XCircle className="h-3 w-3" /> };
    case 'open':
    default:
      return { label: 'Not Paid Yet', tone: 'warning', icon: <Loader2 className="h-3 w-3 animate-spin" /> };
  }
};

interface Props {
  invoice: LightningInvoice;
  onClick: () => void;
}

export const InvoiceRow = ({ invoice, onClick }: Props) => {
  const { label, tone, icon } = invoicePill(invoice.status);
  return (
    <button
      type="button"
      onClick={onClick}
      className="w-full text-left px-3 py-2 rounded-lg bg-background/40 border border-border/60 hover:border-primary/50 hover:bg-background/60 transition-colors flex items-center justify-between gap-3"
    >
      <div className="flex items-center gap-3 min-w-0">
        <div className="p-1.5 rounded-md bg-primary/10 text-primary shrink-0">
          <ArrowDownLeft className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <div className="text-sm font-medium truncate">
            {invoice.valueAtoms > 0 ? `Requested ${fmtDcr(invoice.valueAtoms)}` : 'Open amount'}
          </div>
          <div className="text-xs text-muted-foreground font-mono truncate">
            {truncHash(invoice.rHashHex)}
          </div>
        </div>
      </div>
      <div className="flex flex-col items-end gap-1 shrink-0">
        <StatusPill label={label} tone={tone} icon={icon} />
        <span className="text-xs text-muted-foreground">{fmtDate(invoice.creationDate)}</span>
      </div>
    </button>
  );
};
