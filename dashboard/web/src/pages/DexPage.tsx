// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Lock, TrendingUp } from 'lucide-react';
import { getDexStatus, lockDex, type DexStatus } from '../services/dcrdexApi';
import { DexSetupWizard } from '../components/dex/DexSetupWizard';

export const DexPage = () => {
  const [status, setStatus] = useState<DexStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const refresh = async () => {
    try {
      setStatus(await getDexStatus());
      setErr(null);
    } catch (e: any) {
      setErr(e?.message || 'Failed to load DCRDEX status');
    }
  };

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 10000);
    return () => window.clearInterval(id);
  }, []);

  if (!status && err) {
    return (
      <div className="p-6 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
        <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
        <span>{err}</span>
      </div>
    );
  }
  if (!status) {
    return (
      <div className="min-h-[60vh] flex items-center justify-center">
        <div className="animate-spin rounded-full h-10 w-10 border-b-2 border-primary" />
      </div>
    );
  }

  if (status.stage === 'unavailable') {
    return (
      <div className="min-h-[60vh] flex items-center justify-center p-6">
        <div className="w-full max-w-md p-6 rounded-xl bg-gradient-card border border-border/50 space-y-3 text-center">
          <AlertCircle className="h-6 w-6 text-warning mx-auto" />
          <h2 className="text-lg font-semibold">DCRDEX is unavailable</h2>
          <p className="text-sm text-muted-foreground break-words">
            The DCRDEX backend is not reachable yet. {status.error}
          </p>
        </div>
      </div>
    );
  }

  if (status.stage === 'needs-init' || status.stage === 'needs-unlock') {
    return <DexSetupWizard mode={status.stage} onReady={refresh} />;
  }

  // stage === 'ready' — full-bleed trading view (placeholder for now).
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4 px-4">
        <div className="flex items-center gap-2">
          <TrendingUp className="h-5 w-5 text-primary" />
          <h1 className="text-lg font-semibold">DCRDEX</h1>
          {status.bisonwVersion && (
            <span className="text-xs text-muted-foreground font-mono">
              bisonw {status.bisonwVersion}
            </span>
          )}
        </div>
        <button
          type="button"
          onClick={async () => {
            await lockDex();
            refresh();
          }}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-border rounded-lg hover:bg-background/50 transition-colors"
        >
          <Lock className="h-4 w-4" />
          Lock
        </button>
      </div>

      <div className="px-4">
        <div className="rounded-xl bg-gradient-card border border-border/50 p-8 text-center text-muted-foreground">
          DCRDEX is unlocked and ready. Markets, order book and trading come next.
        </div>
      </div>
    </div>
  );
};
