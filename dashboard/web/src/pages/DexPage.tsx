// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, AlertTriangle, X } from 'lucide-react';
import { getDexStatus, type DexStatus } from '../services/dcrdexApi';
import { DexSetupWizard } from '../components/dex/DexSetupWizard';
import { DexWalletSetup } from '../components/dex/DexWalletSetup';
import { DexHome } from '../components/dex/DexHome';
import { DexMarketView } from '../components/dex/DexMarketView';
import { DexSeedBackup } from '../components/dex/DexSeedBackup';
import { useWalletReady } from '../hooks/useWalletReady';
import { WalletSyncGate } from '../components/common/WalletSyncGate';

export const DexPage = () => {
  const [status, setStatus] = useState<DexStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);
  // Backup-reminder modal state (session-only "remind me later").
  const [remindLater, setRemindLater] = useState(false);
  const [backupOpen, setBackupOpen] = useState(false);
  // Dev preview: /dex?preview renders the trading view with sample data,
  // bypassing onboarding/registration and any DEX-server dependency.
  const previewMode = new URLSearchParams(window.location.search).has('preview');
  const wallet = useWalletReady();

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
          <h2 className="text-lg font-semibold">DCRDEX is starting</h2>
          <p className="text-sm text-muted-foreground break-words">
            {status.message ||
              'The DCRDEX backend is not reachable yet. This page will recover automatically once it is ready.'}
          </p>
        </div>
      </div>
    );
  }

  // First-time DCRDEX setup (init the client or create the dex account) is gated
  // on a synced wallet; unlocking an already-initialized client is not.
  if ((status.stage === 'needs-init' || status.stage === 'needs-wallet') && !wallet.ready) {
    return <WalletSyncGate feature="DCRDEX" message={wallet.message} progress={wallet.progress} />;
  }

  if (status.stage === 'needs-init' || status.stage === 'needs-unlock') {
    return <DexSetupWizard mode={status.stage} onReady={refresh} />;
  }

  if (status.stage === 'needs-wallet') {
    return <DexWalletSetup onReady={refresh} />;
  }

  // stage === 'ready' — full-bleed unlocked view (registration or trading).
  const needsBackup = !status.seedBackedUp;
  return (
    <>
      <DexHome onLock={refresh} />

      {needsBackup && !remindLater && !backupOpen && (
        <div className="fixed inset-0 z-30 flex items-center justify-center bg-background/70 p-6">
          <div className="w-full max-w-md p-6 rounded-xl bg-card border border-border/60 shadow-xl space-y-4">
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-warning" />
              <h2 className="text-lg font-semibold">Back up your DCRDEX seed</h2>
            </div>
            <p className="text-sm text-muted-foreground">
              You have not backed up your DCRDEX recovery seed yet. These 15 words are the only way to
              restore your DEX account, your fidelity bond, and any native coin wallets - without them
              those funds can be lost for good.
            </p>
            <div className="flex gap-3">
              <button
                type="button"
                onClick={() => setRemindLater(true)}
                className="flex-1 px-4 py-2.5 border border-border rounded-lg hover:bg-background/50 transition-colors text-sm"
              >
                Remind me later
              </button>
              <button
                type="button"
                onClick={() => setBackupOpen(true)}
                className="flex-1 bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2.5 transition-colors hover:bg-primary/90"
              >
                Back up now
              </button>
            </div>
          </div>
        </div>
      )}

      {backupOpen && (
        <div className="fixed inset-0 z-30 flex items-center justify-center bg-background/70 p-6">
          <div className="w-full max-w-md p-6 rounded-xl bg-card border border-border/60 shadow-xl space-y-4">
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-semibold">Back up your DCRDEX seed</h2>
              <button
                type="button"
                onClick={() => setBackupOpen(false)}
                title="Close"
                className="p-1.5 rounded-md hover:bg-background/60"
              >
                <X className="h-4 w-4 text-muted-foreground" />
              </button>
            </div>
            <DexSeedBackup
              onDone={() => {
                setBackupOpen(false);
                refresh();
              }}
            />
          </div>
        </div>
      )}
    </>
  );
};
