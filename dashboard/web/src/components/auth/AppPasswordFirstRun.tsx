// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, type FormEvent } from 'react';
import { ShieldCheck, Loader2 } from 'lucide-react';
import { setupAppPassword, skipAppPasswordSetup } from '../../services/auth';
import { UnprotectedWarning } from './UnprotectedWarning';

// AppPasswordFirstRun is the one-time prompt shown on a fresh dashboard. The
// user can set an app password now or, after acknowledging the warning, skip
// and run unprotected; either choice
// dismisses it for good (the backend records the dismissal). It can be enabled
// later from Settings > Security.
export function AppPasswordFirstRun({ onDone }: { onDone: () => void }) {
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [confirmingSkip, setConfirmingSkip] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);

  const enable = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    if (!password) {
      setError('Enter a password.');
      return;
    }
    if (password !== confirm) {
      setError('Passwords do not match.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await setupAppPassword(password);
      onDone();
    } catch (err: any) {
      setError(err?.message || 'Could not enable the app password.');
      setBusy(false);
    }
  };

  const skip = async () => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      await skipAppPasswordSetup();
      onDone();
    } catch {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
      <form
        onSubmit={enable}
        className="relative w-full max-w-md p-6 rounded-xl bg-background border border-border/50 shadow-xl space-y-4"
      >
        <div className="flex items-center gap-3">
          <div className="p-3 rounded-xl bg-warning/10 border border-warning/30">
            <ShieldCheck className="h-6 w-6 text-warning" />
          </div>
          <div>
            <span className="inline-block text-[10px] font-semibold uppercase tracking-wide px-1.5 py-0.5 rounded bg-warning/15 text-warning border border-warning/30">
              Strongly recommended
            </span>
            <h2 className="text-lg font-semibold">Set an app password before you continue</h2>
          </div>
        </div>
        <div>
          <p className="text-sm text-muted-foreground">
            This dashboard can <strong className="text-foreground">spend your funds</strong>:
            it pays Lightning invoices, sends tips and runs your DEX and store.
            Without an app password, anything that can reach it gets that power
            too. That includes{' '}
            <strong className="text-foreground">
              other apps on this device, other devices on your network, and
              websites built to attack local services
            </strong>
            .
          </p>
        </div>
        <input
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="App password"
          className="w-full px-4 py-3 rounded-lg bg-background border border-border/60 focus:border-primary outline-none"
        />
        <input
          type="password"
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          placeholder="Confirm password"
          className="w-full px-4 py-3 rounded-lg bg-background border border-border/60 focus:border-primary outline-none"
        />
        {confirm.length > 0 && password !== confirm && (
          <p className="text-sm text-red-500">Passwords do not match.</p>
        )}
        {error && <p className="text-sm text-red-500">{error}</p>}
        {confirmingSkip ? (
          <div className="space-y-3 pt-2">
            <UnprotectedWarning acknowledged={acknowledged} onAcknowledge={setAcknowledged} />
            <div className="flex flex-wrap items-center justify-end gap-3">
              <button
                type="button"
                onClick={skip}
                disabled={busy || !acknowledged}
                className="px-4 py-2 rounded-lg border border-warning/50 text-warning hover:bg-warning/10 disabled:opacity-50 flex items-center gap-2"
              >
                {busy && <Loader2 className="h-4 w-4 animate-spin" />}
                Leave unprotected
              </button>
              <button
                type="button"
                autoFocus
                onClick={() => {
                  setConfirmingSkip(false);
                  setAcknowledged(false);
                }}
                disabled={busy}
                className="px-4 py-2 rounded-lg bg-primary text-primary-foreground font-semibold disabled:opacity-50"
              >
                Set a password
              </button>
            </div>
          </div>
        ) : (
          <div className="flex items-center justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={() => setConfirmingSkip(true)}
              disabled={busy}
              className="px-1 py-2 text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground disabled:opacity-50"
            >
              Continue without a password
            </button>
            <button
              type="submit"
              disabled={busy || !password || password !== confirm}
              className="px-4 py-2 rounded-lg bg-primary text-primary-foreground font-semibold disabled:opacity-50 flex items-center gap-2"
            >
              {busy && <Loader2 className="h-4 w-4 animate-spin" />}
              Set password
            </button>
          </div>
        )}
        <p className="text-xs text-muted-foreground">
          You can enable or change this anytime in Settings &gt; Security.
        </p>
      </form>
    </div>
  );
}
