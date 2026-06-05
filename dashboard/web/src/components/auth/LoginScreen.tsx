// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, type FormEvent } from 'react';
import { Lock, Loader2 } from 'lucide-react';
import { login } from '../../services/auth';

export function LoginScreen({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!password || busy) return;
    setBusy(true);
    setError('');
    try {
      await login(password);
      setPassword('');
      onSuccess();
    } catch (err: any) {
      setError(
        err?.response?.status === 401
          ? 'Incorrect password.'
          : err?.message || 'Login failed.',
      );
      setBusy(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <form
        onSubmit={submit}
        className="w-full max-w-sm p-6 rounded-xl bg-gradient-card border border-border/50 space-y-4"
      >
        <div className="flex items-center gap-3">
          <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
            <Lock className="h-6 w-6 text-primary" />
          </div>
          <div>
            <h1 className="text-lg font-semibold">Dashboard locked</h1>
            <p className="text-sm text-muted-foreground">
              Enter your app password to continue.
            </p>
          </div>
        </div>
        <input
          type="password"
          autoFocus
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="App password"
          className="w-full px-4 py-3 rounded-lg bg-background border border-border/60 focus:border-primary outline-none"
        />
        {error && <p className="text-sm text-red-500">{error}</p>}
        <button
          type="submit"
          disabled={busy || !password}
          className="w-full px-4 py-3 rounded-lg bg-primary text-primary-foreground font-semibold disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {busy && <Loader2 className="h-4 w-4 animate-spin" />}
          Unlock
        </button>
      </form>
    </div>
  );
}
