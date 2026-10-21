// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, AlertTriangle, Loader2, Lock, X } from 'lucide-react';
import { estimateDexSendFee, sendDexWallet, type DexSendFee } from '../../services/dcrdexApi';
import { fmtAmt } from './dexFormat';
import { apiError } from '../../utils/apiError';
import { secretFieldProps } from '../../utils/secretField';

interface Props {
  assetID: number;
  symbol: string;
  address: string;
  value: number;
  decimals: number;
  subtract: boolean;
  onClose: () => void;
  onSent: (coin: string) => void;
}

// DexSendConfirmModal is the last step of a wallet send, like bisonw's confirm
// form: a fresh fee estimate, the full recipient, and the app password.
export const DexSendConfirmModal = ({ assetID, symbol, address, value, decimals, subtract, onClose, onSent }: Props) => {
  const [fee, setFee] = useState<DexSendFee | null>(null);
  const [feeErr, setFeeErr] = useState<string | null>(null);
  const [appPass, setAppPass] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    estimateDexSendFee(assetID, value, address, subtract)
      .then((f) => !cancelled && setFee(f))
      .catch((e) => !cancelled && setFeeErr(apiError(e, 'estimate failed')));
    return () => {
      cancelled = true;
    };
  }, [assetID, value, address, subtract]);

  const close = () => {
    if (!busy) onClose();
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!appPass || busy) return;
    setBusy(true);
    setErr(null);
    try {
      const coin = await sendDexWallet(assetID, value, address, appPass, subtract);
      setAppPass('');
      onSent(coin);
    } catch (e) {
      setErr(apiError(e, 'Send failed'));
      setAppPass('');
    } finally {
      setBusy(false);
    }
  };

  const feeSym = fee?.feeSymbol.toUpperCase() ?? '';
  const feeSource = subtract
    ? 'Taken out of the amount sent.'
    : feeSym && feeSym !== symbol
      ? `Paid on top of the amount, from your ${feeSym} balance.`
      : 'Paid on top of the amount.';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4" onClick={close}>
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md max-h-[85vh] overflow-y-auto rounded-xl bg-card border border-border/50 shadow-xl"
      >
        <div className="flex items-center justify-between p-5 border-b border-border/50">
          <h3 className="flex items-center gap-2 text-base font-semibold text-foreground">
            <Lock className="h-4 w-4 text-primary" />
            Confirm send
          </h3>
          <button
            type="button"
            onClick={close}
            disabled={busy}
            className="text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={submit} className="p-5 space-y-4">
          <div className="space-y-2 p-4 rounded-lg bg-background border border-border text-sm">
            <div className="flex justify-between gap-3">
              <span className="text-muted-foreground shrink-0">To</span>
              <span className="font-mono text-xs break-all text-right">{address}</span>
            </div>
            <div className="flex justify-between gap-3">
              <span className="text-muted-foreground">Amount</span>
              <span className="font-semibold font-mono tabular-nums">
                {fmtAmt(value, decimals)} {symbol}
              </span>
            </div>
            <div className="flex justify-between gap-3">
              <span className="text-muted-foreground">Network fee (estimate)</span>
              {fee ? (
                <span className="font-mono tabular-nums">
                  ~{fmtAmt(fee.fee, 8)} {feeSym}
                </span>
              ) : feeErr ? (
                <span className="text-warning text-right">Fee estimate not available</span>
              ) : (
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              )}
            </div>
            {feeErr && <p className="text-[11px] text-muted-foreground break-words">{feeErr}</p>}
            <p className="text-[11px] text-muted-foreground">{feeSource}</p>
            {fee && !fee.validAddress && <p className="text-[11px] text-warning">The address may be invalid.</p>}
          </div>

          <div className="p-2.5 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning flex items-start gap-2">
            <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>This spends real funds and cannot be undone.</span>
          </div>

          <div>
            <label className="block text-xs text-muted-foreground mb-1" htmlFor="dex-send-pass">
              Authorize the transfer with your DCRDEX app password.
            </label>
            <input
              {...secretFieldProps}
              id="dex-send-pass"
              type="password"
              autoFocus
              value={appPass}
              onChange={(e) => setAppPass(e.target.value)}
              disabled={busy}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
            />
          </div>

          {err && (
            <div className="flex items-start gap-2 text-xs text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          )}

          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={close}
              disabled={busy}
              className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={busy || !appPass}
              className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition text-sm disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
            >
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {busy ? 'Sending...' : 'Confirm & Send'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
