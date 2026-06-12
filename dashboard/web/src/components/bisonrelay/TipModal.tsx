// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { FormEvent, useEffect, useState } from 'react';
import { toYMDTime } from '../../utils/date';
import { X } from 'lucide-react';
import {
  BisonrelayTipAttempt,
  getBisonrelayTipAttempts,
} from '../../services/bisonrelayApi';

// tipAttemptState derives a short display state from a tracked attempt.
export const tipAttemptState = (a: BisonrelayTipAttempt): string => {
  if (a.completed) return 'completed';
  if (a.last_invoice_error) return 'failed';
  if (a.attempts >= a.max_attempts && a.payment_attempt_failed) return 'failed';
  return `in flight (attempt ${a.attempts}/${a.max_attempts})`;
};

export const formatTipDcr = (matoms: number): string =>
  (matoms / 1e11).toFixed(8).replace(/\.?0+$/, '');

export const TipModal = ({
  nick,
  uid,
  onClose,
  onSubmit,
}: {
  nick: string;
  uid: string;
  onClose: () => void;
  // Fire-and-forget. The page inserts a pending placeholder into the
  // chat thread and tracks completion; this modal just collects the
  // amount and dismisses.
  onSubmit: (dcrAmount: number) => void;
}) => {
  const [value, setValue] = useState('');
  const [history, setHistory] = useState<BisonrelayTipAttempt[] | null>(null);

  useEffect(() => {
    getBisonrelayTipAttempts(uid)
      .then((atts) => {
        atts.sort((a, b) => Date.parse(b.created) - Date.parse(a.created));
        setHistory(atts.slice(0, 5));
      })
      .catch(() => {
        /* older brclientd without the endpoint; history stays hidden */
      });
  }, [uid]);

  const parsed = parseFloat(value);
  const canSubmit = uid !== '' && Number.isFinite(parsed) && parsed > 0;

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    onSubmit(parsed);
    onClose();
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <form
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4"
      >
        <div className="flex items-start justify-between">
          <h3 className="text-base font-semibold">Pay tip to {nick}</h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <p className="text-xs text-muted-foreground">
          The tip rides over your Lightning channel. Both you and {nick} must
          be online; the payment is delivered when their client is reachable.
        </p>
        <div>
          <label className="block text-xs text-muted-foreground mb-1" htmlFor="br-tip-amount">
            Amount (DCR)
          </label>
          <input
            id="br-tip-amount"
            type="number"
            autoFocus
            min="0"
            step="any"
            inputMode="decimal"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="0.001"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
          />
        </div>
        {history !== null && history.length > 0 && (
          <div className="space-y-1">
            <div className="text-[10px] text-muted-foreground uppercase tracking-wide">
              Recent tips
            </div>
            {history.map((a) => (
              <div
                key={`${a.tag}-${a.created}`}
                className="flex items-center gap-2 text-[11px] text-muted-foreground"
                title={a.last_invoice_error || undefined}
              >
                <span className="font-medium text-foreground/90 tabular-nums">
                  {formatTipDcr(a.amount_matoms)} DCR
                </span>
                <span className="opacity-50">·</span>
                <span>{toYMDTime(new Date(a.created))}</span>
                <span className="opacity-50">·</span>
                <span
                  className={
                    tipAttemptState(a) === 'completed'
                      ? 'text-success/90'
                      : tipAttemptState(a) === 'failed'
                        ? 'text-destructive/90'
                        : undefined
                  }
                >
                  {tipAttemptState(a)}
                </span>
              </div>
            ))}
          </div>
        )}
        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={!canSubmit}
            className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Send tip
          </button>
        </div>
      </form>
    </div>
  );
};
