import { useState } from 'react';
import { AlertCircle, AlertTriangle, X } from 'lucide-react';
import { LightningChannel, closeLightningChannel } from '../../../services/lightningApi';

interface Props {
  channel: LightningChannel;
  onClose: () => void;
  onClosed: () => void;
}

export const CloseChannelModal = ({ channel, onClose, onClosed }: Props) => {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const force = !channel.active;

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await closeLightningChannel(channel.channelPoint, force);
      onClosed();
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Close failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in">
      <div className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold">
            {force ? 'Force-close channel' : 'Cooperative close'}
          </h3>
          <button
            onClick={onClose}
            disabled={submitting}
            className="text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="p-6 space-y-4">
          <p className="text-sm text-muted-foreground">
            {force
              ? 'Counterparty appears offline. Force-closing publishes the latest commitment transaction and ties your funds in a multi-day CSV timelock before they become spendable.'
              : 'Negotiate a cooperative close with the counterparty. Funds become spendable on-chain after the close transaction confirms.'}
          </p>
          {force && (
            <div className="rounded-lg bg-warning/10 border border-warning/30 p-3 text-xs text-foreground/80 flex items-start gap-2">
              <AlertTriangle className="h-4 w-4 text-warning shrink-0 mt-0.5" />
              <span>Force-close is only appropriate when the peer is unreachable.</span>
            </div>
          )}
          <div className="text-xs text-muted-foreground font-mono break-all">
            {channel.channelPoint}
          </div>
          {error && (
            <div className="flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>{error}</span>
            </div>
          )}
          <div className="flex justify-end gap-2 pt-2">
            <button
              onClick={onClose}
              disabled={submitting}
              className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              onClick={handleSubmit}
              disabled={submitting}
              className={`px-4 py-2 rounded-lg text-white font-semibold transition-all text-sm disabled:opacity-50 ${
                force ? 'bg-destructive' : 'bg-gradient-primary'
              }`}
            >
              {submitting ? 'Closing…' : force ? 'Force close' : 'Close channel'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
