// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, CheckCircle2, KeyRound, Loader2, X } from 'lucide-react';
import { apiError } from '../../utils/apiError';
import { getDexStatus } from '../../services/dcrdexApi';
import { getLightningStatus, unlockLightning } from '../../services/lightningApi';

interface ChangePassphraseModalProps {
  isOpen: boolean;
  onSubmit: (oldPassphrase: string, newPassphrase: string, dexAppPass?: string) => Promise<void>;
  onClose: () => void;
}

type LnPhase = 'waiting' | 'manual' | 'unlocked' | 'timeout';

export const ChangePassphraseModal = ({ isOpen, onSubmit, onClose }: ChangePassphraseModalProps) => {
  const [oldPass, setOldPass] = useState('');
  const [newPass, setNewPass] = useState('');
  const [confirm, setConfirm] = useState('');
  const [dexAppPass, setDexAppPass] = useState('');
  const [dexLocked, setDexLocked] = useState(false);
  const [lnActive, setLnActive] = useState(false);
  const [step, setStep] = useState<'form' | 'ln'>('form');
  const [lnPhase, setLnPhase] = useState<LnPhase>('waiting');
  const [lnError, setLnError] = useState<string | null>(null);
  const [lnManualPass, setLnManualPass] = useState('');
  const [lnBusy, setLnBusy] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setOldPass('');
      setNewPass('');
      setConfirm('');
      setDexAppPass('');
      setDexLocked(false);
      setLnActive(false);
      setStep('form');
      setLnPhase('waiting');
      setLnError(null);
      setLnManualPass('');
      setLnBusy(false);
      setError(null);
      setSubmitting(false);
      return;
    }
    // A locked DCRDEX holds the wallet passphrase too; its app password is
    // needed to hand over the new one. An unlocked session covers it.
    getDexStatus()
      .then((s) => setDexLocked(s.stage === 'needs-unlock'))
      .catch(() => setDexLocked(false));
    // A set-up Lightning node also holds it, keyed into its macaroon store.
    // The change arms a macaroon reset, so a re-unlock step follows.
    getLightningStatus()
      .then((s) => setLnActive(s.stage === 'needs-unlock' || s.stage === 'syncing' || s.stage === 'ready'))
      .catch(() => setLnActive(false));
  }, [isOpen]);

  // The Lightning step: wait out the supervisor's dcrlnd restart, then unlock
  // once with the new passphrase the user just set. A failed attempt keeps
  // waiting (the node may still have been cycling); the second failure hands
  // over to the manual field.
  useEffect(() => {
    if (!isOpen || step !== 'ln') return;
    let cancelled = false;
    let attempts = 0;
    const startedAt = Date.now();
    const run = async () => {
      await new Promise((r) => setTimeout(r, 4000));
      while (!cancelled) {
        if (Date.now() - startedAt > 120000) {
          setLnPhase('timeout');
          return;
        }
        try {
          const s = await getLightningStatus();
          if (cancelled) return;
          if (s.stage === 'needs-unlock') {
            attempts += 1;
            try {
              await unlockLightning(newPass);
              if (!cancelled) setLnPhase('unlocked');
              return;
            } catch (err: any) {
              if (cancelled) return;
              if (attempts >= 2) {
                setLnError(apiError(err, 'Unlock failed'));
                setLnPhase('manual');
                return;
              }
            }
          }
        } catch {
          // Status unreachable while dcrlnd cycles; keep polling.
        }
        await new Promise((r) => setTimeout(r, 2000));
      }
    };
    run();
    return () => {
      cancelled = true;
    };
  }, [isOpen, step]);

  if (!isOpen) return null;

  const tooShort = newPass !== '' && newPass.length < 8;
  const mismatch = newPass !== '' && confirm !== '' && newPass !== confirm;
  const canSubmit =
    oldPass && newPass && confirm && !tooShort && !mismatch && !submitting && (!dexLocked || dexAppPass);

  const handleClose = () => {
    if (submitting) return;
    onClose();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit(oldPass, newPass, dexLocked ? dexAppPass : undefined);
      if (lnActive) {
        setStep('ln');
      } else {
        onClose();
      }
    } catch (err: any) {
      const msg = apiError(err, 'Failed to change passphrase');
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  const handleManualUnlock = async (e: React.FormEvent) => {
    e.preventDefault();
    if (lnManualPass.length < 8 || lnBusy) return;
    setLnBusy(true);
    setLnError(null);
    try {
      await unlockLightning(lnManualPass);
      setLnPhase('unlocked');
    } catch (err: any) {
      setLnError(apiError(err, 'Unlock failed'));
    } finally {
      setLnBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <KeyRound className="h-5 w-5 text-primary" />
            Change Private Passphrase
          </h3>
          <button
            onClick={handleClose}
            disabled={submitting}
            className="text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {step === 'ln' && (
          <div className="p-6 space-y-4">
            <p className="text-sm text-success flex items-start gap-2">
              <CheckCircle2 className="h-4 w-4 mt-0.5 shrink-0" />
              <span>Private passphrase changed.</span>
            </p>

            {lnPhase === 'waiting' && (
              <div className="flex items-start gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 mt-0.5 shrink-0 animate-spin" />
                <span>
                  Restarting the Lightning node to re-key its macaroons, then unlocking it with the
                  new passphrase…
                </span>
              </div>
            )}

            {lnPhase === 'unlocked' && (
              <>
                <p className="text-sm text-success flex items-start gap-2">
                  <CheckCircle2 className="h-4 w-4 mt-0.5 shrink-0" />
                  <span>Lightning unlocked with the new passphrase.</span>
                </p>
                <div className="flex justify-end pt-2">
                  <button
                    type="button"
                    onClick={onClose}
                    className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition text-sm"
                  >
                    Done
                  </button>
                </div>
              </>
            )}

            {lnPhase === 'manual' && (
              <form onSubmit={handleManualUnlock} className="space-y-4">
                <div>
                  <label className="block text-sm text-muted-foreground mb-1">
                    Wallet passphrase
                  </label>
                  <input
                    type="password"
                    autoComplete="off"
                    minLength={8}
                    autoFocus
                    value={lnManualPass}
                    onChange={(e) => setLnManualPass(e.target.value)}
                    disabled={lnBusy}
                    className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
                  />
                  <p className="text-xs text-muted-foreground mt-1">
                    Automatic unlock failed; enter the new passphrase to unlock the Lightning node.
                  </p>
                </div>
                {lnError && (
                  <div className="flex items-start gap-2 text-sm text-destructive">
                    <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                    <span>{lnError}</span>
                  </div>
                )}
                <div className="flex justify-end gap-2 pt-2">
                  <button
                    type="button"
                    onClick={onClose}
                    disabled={lnBusy}
                    className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm disabled:opacity-50"
                  >
                    Close — unlock later
                  </button>
                  <button
                    type="submit"
                    disabled={lnManualPass.length < 8 || lnBusy}
                    className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition text-sm disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {lnBusy ? 'Unlocking…' : 'Unlock'}
                  </button>
                </div>
              </form>
            )}

            {lnPhase === 'timeout' && (
              <>
                <p className="text-sm text-muted-foreground">
                  The Lightning node is still restarting. It can be unlocked later from the
                  Lightning page with the new passphrase.
                </p>
                <div className="flex justify-end pt-2">
                  <button
                    type="button"
                    onClick={onClose}
                    className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm"
                  >
                    Close
                  </button>
                </div>
              </>
            )}
          </div>
        )}

        {step === 'form' && (
        <form onSubmit={handleSubmit} className="p-6 space-y-4">
          <p className="text-sm text-muted-foreground">
            Rotate the wallet's private (signing) passphrase. The new passphrase will be required
            for all future ticket purchases, transaction signing, and unlock operations.
          </p>

          <div>
            <label className="block text-sm text-muted-foreground mb-1">Current passphrase</label>
            <input
              type="password"
              autoComplete="current-password"
              value={oldPass}
              onChange={(e) => setOldPass(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
            />
          </div>

          <div>
            <label className="block text-sm text-muted-foreground mb-1">New passphrase</label>
            <input
              type="password"
              autoComplete="new-password"
              minLength={8}
              value={newPass}
              onChange={(e) => setNewPass(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
            />
            {tooShort && <p className="text-xs text-destructive mt-1">Must be at least 8 characters.</p>}
          </div>

          <div>
            <label className="block text-sm text-muted-foreground mb-1">Confirm new passphrase</label>
            <input
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
            />
            {mismatch && <p className="text-xs text-destructive mt-1">New passphrases do not match.</p>}
          </div>

          {dexLocked && (
            <div>
              <label className="block text-sm text-muted-foreground mb-1">DCRDEX app password</label>
              <input
                type="password"
                autoComplete="off"
                value={dexAppPass}
                onChange={(e) => setDexAppPass(e.target.value)}
                disabled={submitting}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
              />
              <p className="text-xs text-muted-foreground mt-1">
                DCRDEX is locked and also stores the wallet passphrase. Enter the app password
                chosen at DCRDEX setup (not the wallet passphrase) to propagate the change to
                DCRDEX; it stays locked afterwards.
              </p>
            </div>
          )}

          {error && (
            <div className="flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={handleClose}
              disabled={submitting}
              className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {submitting ? 'Changing…' : 'Change passphrase'}
            </button>
          </div>
        </form>
        )}
      </div>
    </div>
  );
};
