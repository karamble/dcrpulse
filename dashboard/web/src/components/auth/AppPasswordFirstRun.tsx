// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, type FormEvent } from 'react';
import { ShieldCheck, Loader2 } from 'lucide-react';
import { setupAppPassword, skipAppPasswordSetup } from '../../services/auth';

// AppPasswordFirstRun is the one-time prompt shown on a fresh dashboard. The
// user can set an app password now or skip and run unprotected; either choice
// dismisses it for good (the backend records the dismissal). It can be enabled
// later from Settings > Security.
export function AppPasswordFirstRun({ onDone }: { onDone: () => void }) {
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

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
          <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
            <ShieldCheck className="h-6 w-6 text-primary" />
          </div>
          <div>
            <h2 className="text-lg font-semibold">Protect your dashboard</h2>
            <p className="text-sm text-muted-foreground">
              Optionally set an app password so a login is required before anyone
              can use this dashboard.
            </p>
          </div>
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
        <div className="flex items-center justify-end gap-3 pt-2">
          <button
            type="button"
            onClick={skip}
            disabled={busy}
            className="px-4 py-2 rounded-lg text-muted-foreground hover:text-foreground disabled:opacity-50"
          >
            Skip for now
          </button>
          <button
            type="submit"
            disabled={busy || !password || password !== confirm}
            className="px-4 py-2 rounded-lg bg-primary text-primary-foreground font-semibold disabled:opacity-50 flex items-center gap-2"
          >
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            Enable
          </button>
        </div>
        <p className="text-xs text-muted-foreground">
          You can enable or change this anytime in Settings &gt; Security.
        </p>
      </form>
    </div>
  );
}
