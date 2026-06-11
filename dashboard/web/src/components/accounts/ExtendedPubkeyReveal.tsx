import { useState } from 'react';
import { Eye, EyeOff, AlertCircle } from 'lucide-react';
import { getAccountExtendedPubKey } from '../../services/api';
import { CopyButton } from '../explorer/CopyButton';

interface Props {
  accountNumber: number;
  disabled?: boolean;
}

export const ExtendedPubkeyReveal = ({ accountNumber, disabled }: Props) => {
  const [xpub, setXpub] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleReveal = async () => {
    if (loading) return;
    setLoading(true);
    setError(null);
    try {
      const value = await getAccountExtendedPubKey(accountNumber);
      setXpub(value);
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Failed to fetch xpub';
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  const handleHide = () => {
    setXpub(null);
    setError(null);
  };

  if (disabled) return null;

  if (xpub) {
    return (
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">Extended public key</span>
          <div className="flex items-center gap-1">
            <CopyButton text={xpub} label="Copy" />
            <button
              onClick={handleHide}
              className="inline-flex items-center gap-1 px-2 py-1 rounded hover:bg-muted/20 transition-colors text-xs text-muted-foreground"
              title="Hide pubkey"
            >
              <EyeOff className="h-3 w-3" />
              <span>Hide</span>
            </button>
          </div>
        </div>
        <div className="p-2 rounded bg-background/50 border border-border/30">
          <p className="text-xs font-mono break-all text-foreground">{xpub}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <button
        onClick={handleReveal}
        disabled={loading}
        className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-xs disabled:opacity-50"
      >
        <Eye className="h-3.5 w-3.5" />
        <span>{loading ? 'Loading…' : 'Reveal extended pubkey'}</span>
      </button>
      {error && (
        <div className="flex items-start gap-1.5 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}
    </div>
  );
};
