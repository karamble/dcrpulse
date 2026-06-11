import { useState } from 'react';
import { AlertCircle, Lock, X } from 'lucide-react';
import { signPublishTransaction } from '../../services/api';

interface SendPassphraseModalProps {
  isOpen: boolean;
  sourceAccount: number;
  recipient: string;
  amountAtoms: number;
  feeAtoms: number;
  unsignedTxHex: string;
  onClose: () => void;
  onSuccess: (txHash: string) => void;
  onWatchOnly: (message: string) => void;
}

const formatDcr = (atoms: number): string => (atoms / 1e8).toFixed(8);

export const SendPassphraseModal = ({
  isOpen,
  sourceAccount,
  recipient,
  amountAtoms,
  feeAtoms,
  unsignedTxHex,
  onClose,
  onSuccess,
  onWatchOnly,
}: SendPassphraseModalProps) => {
  const [passphrase, setPassphrase] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!isOpen) return null;

  const handleClose = () => {
    if (submitting) return;
    setPassphrase('');
    setError(null);
    onClose();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!passphrase || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const resp = await signPublishTransaction(sourceAccount, unsignedTxHex, passphrase);
      setPassphrase('');
      onSuccess(resp.txHash);
    } catch (err: any) {
      const status = err?.response?.status;
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Failed to send';
      if (status === 400 && /watch-?only/i.test(msg)) {
        setPassphrase('');
        onWatchOnly(msg);
        return;
      }
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  const totalAtoms = amountAtoms + feeAtoms;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in">
      <div className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <Lock className="h-5 w-5 text-primary" />
            Confirm Send
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
          <div className="space-y-2 p-4 rounded-lg bg-background border border-border">
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">To</span>
              <span className="font-mono text-xs break-all text-right ml-3">{recipient}</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">Amount</span>
              <span className="font-semibold">{formatDcr(amountAtoms)} DCR</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">Network fee</span>
              <span>{formatDcr(feeAtoms)} DCR</span>
            </div>
            <div className="flex justify-between text-sm pt-2 border-t border-border/50">
              <span className="text-muted-foreground">Total</span>
              <span className="font-semibold">{formatDcr(totalAtoms)} DCR</span>
            </div>
          </div>

          <div>
            <label className="block text-sm text-muted-foreground mb-1" htmlFor="send-passphrase">
              Wallet passphrase
            </label>
            <input
              id="send-passphrase"
              type="password"
              autoComplete="current-password"
              autoFocus
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
              {submitting ? 'Sending...' : 'Confirm & Send'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
