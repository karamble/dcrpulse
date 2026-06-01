// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Search, X } from 'lucide-react';

interface DiscoverAddressesModalProps {
  isOpen: boolean;
  defaultGapLimit: number;
  onSubmit: (passphrase: string, gapLimit: number) => Promise<void>;
  onClose: () => void;
}

export const DiscoverAddressesModal = ({
  isOpen,
  defaultGapLimit,
  onSubmit,
  onClose,
}: DiscoverAddressesModalProps) => {
  const [passphrase, setPassphrase] = useState('');
  const [gapLimit, setGapLimit] = useState<number>(defaultGapLimit);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setPassphrase('');
      setGapLimit(defaultGapLimit);
      setError(null);
      setSubmitting(false);
    } else {
      setGapLimit(defaultGapLimit);
    }
  }, [isOpen, defaultGapLimit]);

  if (!isOpen) return null;

  const handleClose = () => {
    if (submitting) return;
    onClose();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!passphrase || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit(passphrase, gapLimit);
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Discovery failed';
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
            <Search className="h-5 w-5 text-primary" />
            Discover Address Usage
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
            Scans the chain for previously-used addresses derived under the gap limit you provide.
            This can take several minutes; the wallet is briefly unlocked during the scan and
            re-locked afterwards.
          </p>

          <div>
            <label className="block text-sm text-muted-foreground mb-1">Gap limit</label>
            <input
              type="number"
              min={20}
              max={10000}
              step={20}
              value={gapLimit}
              onChange={(e) => setGapLimit(Math.max(20, Number(e.target.value) || defaultGapLimit))}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50 font-mono"
            />
            <p className="text-xs text-muted-foreground mt-1">
              Default 200. Increase if you restored a wallet that previously used a higher gap.
            </p>
          </div>

          <div>
            <label className="block text-sm text-muted-foreground mb-1">Wallet passphrase</label>
            <input
              type="password"
              autoComplete="current-password"
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
            />
          </div>

          {error && (
            <div className="flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {submitting && (
            <p className="text-xs text-muted-foreground">
              Scanning… this can take several minutes. Don't close the tab.
            </p>
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
              disabled={!passphrase || submitting}
              className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {submitting ? 'Scanning…' : 'Discover'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
