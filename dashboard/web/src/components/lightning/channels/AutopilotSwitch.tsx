import { useEffect, useState } from 'react';
import { Bot, Loader2 } from 'lucide-react';
import {
  getLightningAutopilot,
  setLightningAutopilot,
} from '../../../services/lightningApi';

export const AutopilotSwitch = () => {
  const [active, setActive] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getLightningAutopilot()
      .then((r) => setActive(r.active))
      .catch((err) => setError(err?.message || 'Failed to load autopilot status'));
  }, []);

  const toggle = async () => {
    if (active === null) return;
    setBusy(true);
    setError(null);
    try {
      await setLightningAutopilot(!active);
      setActive(!active);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Toggle failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="p-4 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 flex items-center gap-3">
      <Bot className="h-5 w-5 text-primary shrink-0" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-semibold">Autopilot</div>
        <div className="text-xs text-muted-foreground">
          Automatically opens channels using up to 60% of the lightning account's spendable funds.
        </div>
        {error && <div className="text-xs text-destructive mt-1">{error}</div>}
      </div>
      <button
        type="button"
        onClick={toggle}
        disabled={busy || active === null}
        className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors disabled:opacity-50 ${
          active ? 'bg-primary/20 text-primary' : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
        }`}
      >
        {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : active ? 'Enabled' : 'Disabled'}
      </button>
    </div>
  );
};
