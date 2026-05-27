// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle } from 'lucide-react';
import { getDexStatus, type DexStatus } from '../services/dcrdexApi';
import { DexSetupWizard } from '../components/dex/DexSetupWizard';
import { DexWalletSetup } from '../components/dex/DexWalletSetup';
import { DexHome } from '../components/dex/DexHome';
import { DexMarketView } from '../components/dex/DexMarketView';

export const DexPage = () => {
  const [status, setStatus] = useState<DexStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);
  // Dev preview: /dex?preview renders the trading view with sample data,
  // bypassing onboarding/registration and any DEX-server dependency.
  const previewMode = new URLSearchParams(window.location.search).has('preview');

  const refresh = async () => {
    try {
      setStatus(await getDexStatus());
      setErr(null);
    } catch (e: any) {
      setErr(e?.message || 'Failed to load DCRDEX status');
    }
  };

  useEffect(() => {
    if (previewMode) return;
    refresh();
    const id = window.setInterval(refresh, 10000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [previewMode]);

  if (previewMode) {
    return <DexMarketView preview />;
  }

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

  if (status.stage === 'needs-wallet') {
    return <DexWalletSetup onReady={refresh} />;
  }

  // stage === 'ready' — full-bleed unlocked view (registration or trading).
  return <DexHome onLock={refresh} />;
};
