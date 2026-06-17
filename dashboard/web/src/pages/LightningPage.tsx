import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import {
  LightningStage,
  LightningStatus,
  getLightningStatus,
} from '../services/lightningApi';
import { LightningSetupWizard } from '../components/lightning/setup/LightningSetupWizard';
import { LightningLayout } from '../components/lightning/LightningLayout';
import { useWalletReady } from '../hooks/useWalletReady';
import { WalletSyncGate } from '../components/common/WalletSyncGate';

export const LightningPage = () => {
  const [status, setStatus] = useState<LightningStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const wallet = useWalletReady();

  const refresh = async () => {
    try {
      const s = await getLightningStatus();
      setStatus(s);
      setError(null);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to load Lightning status');
    }
  };

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 10000);
    return () => window.clearInterval(id);
  }, []);

  if (!status) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading Lightning…
      </div>
    );
  }

  const stage: LightningStage = status.stage;

  if (stage === 'needs-setup') {
    return wallet.ready ? (
      <LightningSetupWizard needsSetup onReady={refresh} />
    ) : (
      <WalletSyncGate feature="Lightning" message={wallet.message} progress={wallet.progress} />
    );
  }

  if (stage === 'needs-unlock') {
    return <LightningSetupWizard needsSetup={false} onReady={refresh} />;
  }

  if (stage === 'unavailable') {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin" />
        {status.message ||
          error ||
          'Lightning is starting or not reachable. This page will recover automatically once dcrlnd is ready.'}
      </div>
    );
  }

  // syncing | ready — render the layout shell + Overview tab.
  return <LightningLayout />;
};
