import { useEffect, useState } from 'react';
import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import { LightningDisclaimer } from './LightningDisclaimer';
import {
  getLightningStatus,
  setupLightning,
  unlockLightning,
} from '../../../services/lightningApi';

type Step = 'disclaimer' | 'passphrase' | 'running' | 'done';

interface Props {
  // When false, render the unlock-only flow (sentinel exists, dcrlnd
  // is up, wallet locked) instead of the full setup.
  needsSetup: boolean;
  onReady: () => void;
}

export const LightningSetupWizard = ({ needsSetup, onReady }: Props) => {
  const [step, setStep] = useState<Step>(needsSetup ? 'disclaimer' : 'passphrase');
  const [passphrase, setPassphrase] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [progressMsg, setProgressMsg] = useState('');
  // Non-null only on the setup path (i.e. we just called InitWallet via
  // setupLightning). Used to label the dcrlnd boot window with an elapsed
  // counter and to skip the auto-unlock that only makes sense in the
  // dashboard-restart unlock-only flow.
  const [setupStartedAt, setSetupStartedAt] = useState<number | null>(null);

  useEffect(() => {
    if (step !== 'running') return;
    let cancelled = false;
    let unlockAttempted = false;
    const poll = async () => {
      while (!cancelled) {
        try {
          const status = await getLightningStatus();
          if (status.stage === 'ready' || status.stage === 'syncing') {
            if (!cancelled) {
              setStep('done');
              setTimeout(() => onReady(), 400);
            }
            return;
          }
          if (status.stage === 'needs-unlock') {
            if (setupStartedAt !== null) {
              // Post-setup boot window. dcrlnd's InitWallet has run but
              // the daemon still requires an explicit UnlockWallet to
              // flip its Lightning RPC out of "wallet locked" state.
              // The first attempt usually races with dcrlnd's startup
              // and fails silently, so retry on every poll until status
              // flips. Mirrors Decrediton's STARTUPSTAGE_UNLOCK feeding
              // into STARTUPSTAGE_CONNECT (LNActions.js:140-177).
              const secs = Math.max(0, Math.floor((Date.now() - setupStartedAt) / 1000));
              setProgressMsg(
                `Connecting to Lightning daemon (${secs}s)… this can take 1-2 minutes.`,
              );
              if (secs > 180) {
                if (!cancelled) {
                  setError(
                    'Lightning daemon did not come up after 3 minutes. Check dashboard logs.',
                  );
                  setStep('passphrase');
                }
                return;
              }
              if (passphrase) {
                try {
                  await unlockLightning(passphrase);
                } catch {
                  // dcrlnd may not be ready yet; retry next poll.
                }
              }
            } else if (!unlockAttempted && passphrase) {
              // Unlock-only flow (dashboard restart with sentinel
              // present). Single-shot auto-unlock matches Decrediton's
              // LNWALLET_STARTUPSTAGE_UNLOCK.
              unlockAttempted = true;
              setProgressMsg('Unlocking Lightning wallet…');
              try {
                await unlockLightning(passphrase);
              } catch {
                // Surface the error in the unlock form on the next pass.
              }
            } else {
              setProgressMsg('Waiting for Lightning wallet to unlock…');
            }
          } else if (status.stage === 'unavailable') {
            const secs = setupStartedAt
              ? Math.max(0, Math.floor((Date.now() - setupStartedAt) / 1000))
              : 0;
            setProgressMsg(
              setupStartedAt
                ? `Starting Lightning daemon (${secs}s)…`
                : 'Starting dcrlnd…',
            );
          }
        } catch {
          // Transient — keep polling.
        }
        await new Promise((r) => setTimeout(r, 2000));
      }
    };
    poll();
    return () => {
      cancelled = true;
    };
  }, [step, onReady, passphrase, setupStartedAt]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!passphrase || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      if (needsSetup) {
        setSetupStartedAt(Date.now());
        setStep('running');
        setProgressMsg('Creating lightning account…');
        await setupLightning(passphrase);
      } else {
        setSetupStartedAt(null);
        setStep('running');
        setProgressMsg('Unlocking Lightning wallet…');
        await unlockLightning(passphrase);
      }
      // Keep the passphrase in component state while the polling effect
      // retries UnlockWallet during the dcrlnd boot window. It is cleared
      // when the component unmounts on transition to the LN Overview.
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Operation failed');
      setStep('passphrase');
    } finally {
      setSubmitting(false);
    }
  };

  if (step === 'disclaimer') {
    return <LightningDisclaimer onAcknowledge={() => setStep('passphrase')} />;
  }

  if (step === 'passphrase') {
    return (
      <form
        onSubmit={handleSubmit}
        className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4 max-w-md"
      >
        <h2 className="text-lg font-semibold">
          {needsSetup ? 'Enable Lightning' : 'Unlock Lightning Wallet'}
        </h2>
        <p className="text-sm text-muted-foreground">
          {needsSetup
            ? 'Enter your wallet passphrase to create the lightning account and initialise the LN wallet.'
            : 'Enter your wallet passphrase to unlock the LN wallet.'}
        </p>
        <div>
          <label className="block text-sm text-muted-foreground mb-1" htmlFor="ln-passphrase">
            Wallet passphrase
          </label>
          <input
            id="ln-passphrase"
            type="password"
            autoComplete="current-password"
            minLength={8}
            autoFocus
            value={passphrase}
            onChange={(e) => setPassphrase(e.target.value)}
            disabled={submitting}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
          />
          <p className="text-xs text-muted-foreground mt-1">Minimum 8 characters.</p>
        </div>
        {error && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>{error}</span>
          </div>
        )}
        <div className="flex justify-end pt-2">
          <button
            type="submit"
            disabled={passphrase.length < 8 || submitting}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {submitting ? 'Working…' : needsSetup ? 'Enable Lightning' : 'Unlock'}
          </button>
        </div>
      </form>
    );
  }

  if (step === 'running') {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4 max-w-md">
        <div className="flex items-center gap-3">
          <Loader2 className="h-5 w-5 animate-spin text-primary" />
          <h2 className="text-lg font-semibold">Setting up Lightning</h2>
        </div>
        <p className="text-sm text-muted-foreground">{progressMsg}</p>
      </div>
    );
  }

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4 max-w-md">
      <div className="flex items-center gap-3 text-success">
        <CheckCircle2 className="h-5 w-5" />
        <h2 className="text-lg font-semibold">Lightning is ready</h2>
      </div>
      <p className="text-sm text-muted-foreground">Loading Overview…</p>
    </div>
  );
};
