// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, X } from 'lucide-react';
import { cancelDexOrder, type DexMarket, type DexOrder } from '../../services/dcrdexApi';
import { convQty, fmtAmt } from './dexFormat';

const serverMsg = (e: any): string =>
  (typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Cancel failed';

interface ModalProps {
  order: DexOrder;
  market?: DexMarket;
  busy: boolean;
  err: string | null;
  onConfirm: () => void;
  onDismiss: () => void;
}

// DexCancelModal is the cancel confirmation, mirroring bisonw's cancelOrderForm:
// it submits a cancel order for the still-booked remainder (no password), and
// notes that the remainder may change before the cancel is matched.
const DexCancelModal = ({ order, market, busy, err, onConfirm, onDismiss }: ModalProps) => {
  const baseConv = market?.baseConvFactor || 1e8;
  const baseSym = market?.base || order.marketName.split('_')[0]?.toUpperCase() || '';
  const remaining = convQty(Math.max(0, order.quantity - order.filled), baseConv);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in" onClick={busy ? undefined : onDismiss}>
      <div className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between p-5 border-b border-border/50">
          <h3 className="text-lg font-semibold">Cancel order</h3>
          <button
            type="button"
            onClick={onDismiss}
            disabled={busy}
            className="text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-5 space-y-3 text-sm">
          <p>
            Submit a cancel order for the remaining{' '}
            <span className="font-mono tabular-nums font-semibold">{fmtAmt(remaining, 8)} {baseSym}</span>.
          </p>
          <p className="text-muted-foreground">The remaining amount may change before the cancel order is matched.</p>

          {err && (
            <div className="flex items-start gap-2 text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          )}

          <div className="flex justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={onDismiss}
              disabled={busy}
              className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm disabled:opacity-50"
            >
              Dismiss
            </button>
            <button
              type="button"
              onClick={onConfirm}
              disabled={busy}
              className="px-4 py-2 rounded-lg bg-destructive hover:bg-destructive/90 text-white font-semibold transition-colors text-sm disabled:opacity-50"
            >
              {busy ? 'Submitting…' : 'Submit cancel'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

// useDexCancel centralizes the cancel-confirm flow shared by the order panels:
// requestCancel(order, market) opens the confirmation modal, and the returned
// `modal` node renders it. On confirm it submits the cancel and, on success,
// calls onDone (typically a myorders refetch). Mirrors bisonw's showCancel /
// submitCancel two-step.
export function useDexCancel(onDone?: () => void) {
  const [pending, setPending] = useState<{ order: DexOrder; market?: DexMarket } | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const requestCancel = (order: DexOrder, market?: DexMarket) => {
    setErr(null);
    setPending({ order, market });
  };
  const dismiss = () => {
    if (busy) return;
    setPending(null);
    setErr(null);
  };
  const confirm = async () => {
    if (!pending) return;
    setBusy(true);
    setErr(null);
    try {
      await cancelDexOrder(pending.order.id);
      setPending(null);
      onDone?.();
    } catch (e) {
      setErr(serverMsg(e));
    } finally {
      setBusy(false);
    }
  };

  const modal = pending ? (
    <DexCancelModal order={pending.order} market={pending.market} busy={busy} err={err} onConfirm={confirm} onDismiss={dismiss} />
  ) : null;

  return { requestCancel, modal };
}
