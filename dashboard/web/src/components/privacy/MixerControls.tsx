import { useEffect, useState } from 'react';
import { AlertCircle, Play, Square } from 'lucide-react';
import { getAutobuyerStatus, startMixer, stopMixer } from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';

interface Props {
  running: boolean;
  onChanged: () => void;
}

export const MixerControls = ({ running, onChanged }: Props) => {
  const [modalOpen, setModalOpen] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // The autobuyer and the mixer both spend the mixed account, so the mixer
  // can't start while the autobuyer is running. Poll so the block clears once
  // the autobuyer is stopped.
  const [autobuyerRunning, setAutobuyerRunning] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const check = async () => {
      const ab = await getAutobuyerStatus().catch(() => null);
      if (!cancelled) setAutobuyerRunning(!!ab?.running);
    };
    check();
    const id = window.setInterval(check, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

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
            disabled={autobuyerRunning}
            title={autobuyerRunning ? 'Stop the ticket autobuyer first' : undefined}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Play className="h-4 w-4" />
            Start mixer
          </button>
        )}
      </div>

      {!running && autobuyerRunning && (
        <div className="flex items-start gap-2 text-xs text-warning">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span>
            The ticket autobuyer is running. Stop it before starting the mixer; the autobuyer
            mixes its ticket buys while it runs.
          </span>
        </div>
      )}

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
