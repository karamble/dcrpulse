// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, KeyRound, ShieldCheck } from 'lucide-react';
import { initDex, unlockDex } from '../../services/dcrdexApi';

interface DexSetupWizardProps {
  mode: 'needs-init' | 'needs-unlock';
  onReady: () => void;
}

// DexSetupWizard gates the DEX feature until the bisonw backend is initialized
// and unlocked. The app password is sent to the dashboard for the session only
// and is never stored; after a restart the user unlocks again here.
export const DexSetupWizard = ({ mode, onReady }: DexSetupWizardProps) => {
  const isInit = mode === 'needs-init';
  const [pass, setPass] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const mismatch = isInit && confirm.length > 0 && pass !== confirm;
  const canSubmit = pass.length > 0 && !busy && (!isInit || pass === confirm);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setBusy(true);
    setErr(null);
    try {
      if (isInit) {
        await initDex(pass);
      } else {
        await unlockDex(pass);
      }
      onReady();
    } catch (e2: any) {
      const body = e2?.response?.data;
      setErr((typeof body === 'string' && body.trim()) || e2?.message || 'Request failed');
      setBusy(false);
    }
  };

  return (
    <div className="min-h-[60vh] flex items-center justify-center p-6">
      <div className="w-full max-w-md p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-5">
        <div className="flex items-center gap-2">
          {isInit ? (
            <ShieldCheck className="h-5 w-5 text-primary" />
          ) : (
            <KeyRound className="h-5 w-5 text-primary" />
          )}
          <h2 className="text-lg font-semibold">
            {isInit ? 'Set up DCRDEX' : 'Unlock DCRDEX'}
          </h2>
        </div>

        <p className="text-sm text-muted-foreground">
          {isInit
            ? 'Choose an app password for the DCRDEX backend. It encrypts your DEX account and is required to trade. It is held only for this session and never stored, so you will enter it again after a restart.'
            : 'Enter your DCRDEX app password to unlock trading for this session.'}
        </p>

        <form onSubmit={onSubmit} className="space-y-4">
          <div>
            <label className="block text-xs text-muted-foreground mb-1" htmlFor="dex-pass">
              App password
            </label>
            <input
              id="dex-pass"
              type="password"
              autoFocus
              value={pass}
              onChange={(e) => setPass(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
            />
          </div>

          {isInit && (
            <div>
              <label className="block text-xs text-muted-foreground mb-1" htmlFor="dex-confirm">
                Confirm password
              </label>
              <input
                id="dex-confirm"
                type="password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
              />
              {mismatch && (
                <p className="text-xs text-destructive mt-1">Passwords do not match.</p>
              )}
            </div>
          )}

          {err && (
            <div className="p-3 rounded-lg bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          )}

          <button
            type="submit"
            disabled={!canSubmit}
            className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2.5 transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {busy ? (isInit ? 'Setting up…' : 'Unlocking…') : isInit ? 'Set up DCRDEX' : 'Unlock'}
          </button>
        </form>
      </div>
    </div>
  );
};
