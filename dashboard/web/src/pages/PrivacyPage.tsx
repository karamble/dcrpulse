import { useCallback, useEffect, useState } from 'react';
import { AlertCircle, ShieldCheck } from 'lucide-react';
import { AccountInfo, PrivacyStatus, getAccounts, getPrivacyStatus } from '../services/api';
import { PrivacySetupCard } from '../components/privacy/PrivacySetupCard';
import { MixerStatusBadge } from '../components/privacy/MixerStatusBadge';
import { MixerBalanceCards } from '../components/privacy/MixerBalanceCards';
import { MixerControls } from '../components/privacy/MixerControls';
import { MixerConfigCard } from '../components/privacy/MixerConfigCard';
import { SendToUnmixedCard } from '../components/privacy/SendToUnmixedCard';
import { MixerEventLog } from '../components/privacy/MixerEventLog';

export const PrivacyPage = () => {
  const [status, setStatus] = useState<PrivacyStatus | null>(null);
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [s, a] = await Promise.all([getPrivacyStatus(), getAccounts()]);
      setStatus(s);
      setAccounts(a);
      setError(null);
    } catch (err: any) {
      if (err?.response?.status === 503) {
        setError('Wallet is not loaded yet.');
      } else {
        setError('Failed to load privacy status');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const id = window.setInterval(load, 5000);
    return () => window.clearInterval(id);
  }, [load]);

  const balanceFor = (accountNumber: number | undefined): number => {
    if (accountNumber === undefined) return 0;
    const found = accounts.find((a) => a.accountNumber === accountNumber);
    return found ? found.totalBalance : 0;
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
            <ShieldCheck className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">Privacy</h2>
            <p className="text-sm text-muted-foreground">
              Mix your DCR peer-to-peer to improve transaction privacy.
            </p>
          </div>
        </div>
        {status?.configured && <MixerStatusBadge running={status.mixerRunning} />}
      </div>

      {error && (
        <div className="p-4 rounded-xl bg-destructive/10 border border-destructive/30 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {loading ? (
        <div className="p-12 rounded-xl bg-gradient-card border border-border/50 text-center text-muted-foreground">
          Loading privacy status…
        </div>
      ) : !status?.configured ? (
        <PrivacySetupCard onConfigured={load} />
      ) : (
        <>
          <MixerBalanceCards
            unmixedBalance={balanceFor(status.changeAccount)}
            mixedBalance={balanceFor(status.mixedAccount)}
            running={status.mixerRunning}
          />

          <MixerControls running={status.mixerRunning} onChanged={load} />

          {status.lastError && !status.mixerRunning && (
            <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>Last error: {status.lastError}</span>
            </div>
          )}

          {status.mixedAccount !== undefined && status.changeAccount !== undefined && (
            <>
              <MixerConfigCard
                mixedAccount={status.mixedAccount}
                changeAccount={status.changeAccount}
                csppsolverState={status.csppsolverState}
              />
              <SendToUnmixedCard changeAccount={status.changeAccount} />
            </>
          )}

          <MixerEventLog />
        </>
      )}
    </div>
  );
};
