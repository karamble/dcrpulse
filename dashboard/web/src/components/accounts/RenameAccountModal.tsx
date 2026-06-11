import { useEffect, useState } from 'react';
import { AlertCircle, Edit2, X } from 'lucide-react';
import { renameAccount } from '../../services/api';

interface Props {
  isOpen: boolean;
  accountNumber: number;
  currentName: string;
  onClose: () => void;
  onSuccess: (newName: string) => void;
}

export const RenameAccountModal = ({
  isOpen,
  accountNumber,
  currentName,
  onClose,
  onSuccess,
}: Props) => {
  const [name, setName] = useState(currentName);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      setName(currentName);
      setError(null);
    }
  }, [isOpen, currentName]);

  if (!isOpen) return null;

  const trimmedName = name.trim();
  const nameError =
    trimmedName.length === 0
      ? null
      : trimmedName.length > 50
        ? 'Name must be 50 characters or fewer'
        : trimmedName.toLowerCase() === 'imported'
          ? "'imported' is reserved"
          : null;

  const canSubmit =
    trimmedName.length > 0 &&
    !nameError &&
    trimmedName !== currentName &&
    !submitting;

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
      await renameAccount(accountNumber, trimmedName);
      onSuccess(trimmedName);
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Failed to rename';
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in">
      <div className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <Edit2 className="h-5 w-5 text-primary" />
            Rename Account
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
          <div className="text-xs text-muted-foreground">
            Renaming account <span className="font-mono">#{accountNumber}</span>{' '}
            (currently <span className="font-medium text-foreground">{currentName}</span>).
          </div>

          <div>
            <label className="block text-sm text-muted-foreground mb-1" htmlFor="rename-account-name">
              New name
            </label>
            <input
              id="rename-account-name"
              type="text"
              autoFocus
              maxLength={50}
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
            />
            {nameError && <p className="mt-1 text-xs text-destructive">{nameError}</p>}
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
              {submitting ? 'Renaming…' : 'Save'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
