// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, KeyRound, X } from 'lucide-react';

interface ChangePassphraseModalProps {
  isOpen: boolean;
  onSubmit: (oldPassphrase: string, newPassphrase: string) => Promise<void>;
  onClose: () => void;
}

export const ChangePassphraseModal = ({ isOpen, onSubmit, onClose }: ChangePassphraseModalProps) => {
  const [oldPass, setOldPass] = useState('');
  const [newPass, setNewPass] = useState('');
  const [confirm, setConfirm] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setOldPass('');
      setNewPass('');
      setConfirm('');
      setError(null);
      setSubmitting(false);
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const tooShort = newPass !== '' && newPass.length < 8;
  const mismatch = newPass !== '' && confirm !== '' && newPass !== confirm;
  const canSubmit = oldPass && newPass && confirm && !tooShort && !mismatch && !submitting;

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
      await onSubmit(oldPass, newPass);
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Failed to change passphrase';
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in">
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
              className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {submitting ? 'Changing…' : 'Change passphrase'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
