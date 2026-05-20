import { useState } from 'react';
import { AlertCircle, Play, Square } from 'lucide-react';
import { startMixer, stopMixer } from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';

interface Props {
  running: boolean;
  onChanged: () => void;
}

export const MixerControls = ({ running, onChanged }: Props) => {
  const [modalOpen, setModalOpen] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleStart = async (passphrase: string) => {
    await startMixer(passphrase);
    setModalOpen(false);
    onChanged();
  };

  const handleStop = async () => {
    setStopping(true);
    setError(null);
    try {
      await stopMixer();
      onChanged();
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to stop');
    } finally {
      setStopping(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        {running ? (
          <button
            onClick={handleStop}
            disabled={stopping}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-destructive/20 hover:bg-destructive/30 border border-destructive/40 text-destructive font-semibold text-sm transition-colors disabled:opacity-50"
          >
            <Square className="h-4 w-4" />
            {stopping ? 'Stopping…' : 'Stop mixer'}
          </button>
        ) : (
          <button
            onClick={() => setModalOpen(true)}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all hover:opacity-90"
          >
            <Play className="h-4 w-4" />
            Start mixer
          </button>
        )}
      </div>

      <p className="text-xs text-muted-foreground">
        The dashboard container must stay running for mixing to continue. Mix cycles run
        peer-to-peer over the Decred network and complete only when enough peers are paired.
      </p>

      {error && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <PassphraseModal
        isOpen={modalOpen}
        title="Start mixer"
        description="Enter your wallet passphrase to start the CoinJoin mixer."
        submitLabel="Start mixer"
        busyLabel="Starting…"
        onSubmit={handleStart}
        onClose={() => setModalOpen(false)}
      />
    </div>
  );
};
