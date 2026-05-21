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
            // Setup succeeded; dcrlnd is up but its wallet is locked.
            // Auto-unlock once with the passphrase the user just typed —
            // matches Decrediton's chained init→unlock UX so the user is
            // not prompted twice for the same passphrase.
            if (!unlockAttempted && passphrase) {
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
            setProgressMsg('Starting dcrlnd…');
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
  }, [step, onReady, passphrase]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!passphrase || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      if (needsSetup) {
        setStep('running');
        setProgressMsg('Creating lightning account…');
        await setupLightning(passphrase);
      } else {
        setStep('running');
        setProgressMsg('Unlocking Lightning wallet…');
        await unlockLightning(passphrase);
      }
      setPassphrase('');
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
