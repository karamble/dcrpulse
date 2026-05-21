import { useEffect, useState } from 'react';
import { AlertCircle, ChevronDown, Server } from 'lucide-react';
import { VSPInfo, listVSPs } from '../../services/api';

interface Props {
  network: 'mainnet' | 'testnet';
  value: VSPInfo | null;
  onChange: (vsp: VSPInfo | null) => void;
}

export const VSPSelect = ({ network, value, onChange }: Props) => {
  const [vsps, setVsps] = useState<VSPInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    listVSPs()
      .then((all) => {
        if (cancelled) return;
        const filtered = all
          .filter((v) => v.network === network)
          .sort((a, b) => a.feePercentage - b.feePercentage);
        setVsps(filtered);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err?.message || 'Failed to load VSPs');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [network]);

  return (
    <div className="space-y-1">
      <label className="text-sm font-medium text-muted-foreground flex items-center gap-2">
        <Server className="h-4 w-4" />
        Voting Service Provider (VSP)
      </label>
      <div className="relative">
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          disabled={loading || vsps.length === 0}
          className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-left text-sm hover:border-primary/30 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-between"
        >
          <span className={value ? 'text-foreground' : 'text-muted-foreground'}>
            {loading
              ? 'Loading VSPs...'
              : value
              ? `${value.host}  (${value.feePercentage.toFixed(2)}% fee)`
              : 'Select a VSP'}
          </span>
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        </button>
        {open && (
          <div className="absolute z-10 mt-1 w-full max-h-72 overflow-auto rounded-lg bg-card border border-border/50 shadow-xl">
            {vsps.map((vsp) => (
              <button
                key={vsp.host}
                type="button"
                onClick={() => {
                  onChange(vsp);
                  setOpen(false);
                }}
                className="w-full px-3 py-2 text-left text-sm hover:bg-muted/20 transition-colors flex justify-between items-center gap-3"
              >
                <span className="font-mono text-xs truncate flex-1">{vsp.host}</span>
                <span className="text-xs text-primary shrink-0">{vsp.feePercentage.toFixed(2)}%</span>
              </button>
            ))}
          </div>
        )}
      </div>
      {error && (
        <p className="text-xs text-destructive flex items-center gap-1">
          <AlertCircle className="h-3 w-3" />
          {error}
        </p>
      )}
      {value && (
        <p className="text-xs text-muted-foreground">
          PubKey: <span className="font-mono">{value.pubkey.slice(0, 16)}…</span>
          {value.vspdVersion && <> · vspd {value.vspdVersion}</>}
        </p>
      )}
    </div>
  );
};
