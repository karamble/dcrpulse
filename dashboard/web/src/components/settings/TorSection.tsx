// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Shield, AlertCircle, CheckCircle2, RefreshCw, Loader2, Info } from 'lucide-react';
import {
  TorSettings,
  TorStatus,
  TorControl,
  getTorSettings,
  saveTorSettings,
  getTorStatus,
  getTorControl,
  torNewIdentity,
} from '../../services/tor/client';

interface ToggleProps {
  label: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (next: boolean) => void;
}

const Toggle = ({ label, description, checked, disabled, onChange }: ToggleProps) => (
  <div className="flex items-start justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
    <div>
      <span className="font-medium block">{label}</span>
      <span className="text-sm text-muted-foreground block">{description}</span>
    </div>
    <button
      type="button"
      onClick={() => onChange(!checked)}
      disabled={disabled}
      className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
        checked
          ? 'bg-success/20 text-success hover:bg-success/30'
          : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
      } disabled:opacity-50 disabled:cursor-wait`}
    >
      {checked ? 'On' : 'Off'}
    </button>
  </div>
);

const formatBytes = (n: number): string => {
  if (!n || n < 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
};

const Stat = ({ label, value }: { label: string; value: string }) => (
  <div className="p-3 rounded-lg bg-muted/10 border border-border/50">
    <div className="text-xs text-muted-foreground uppercase tracking-wide">{label}</div>
    <div className="font-mono mt-1">{value}</div>
  </div>
);

const daemonLabels: Record<string, string> = {
  dcrd: 'Node (dcrd)',
  dcrwallet: 'Wallet (dcrwallet)',
  dcrlnd: 'Lightning (dcrlnd)',
  dcrdex: 'Dex (bisonw)',
  brclientd: 'Bison Relay (brclientd)',
  dashboard: 'Dashboard',
};

export const TorSection = () => {
  const [settings, setSettings] = useState<TorSettings | null>(null);
  const [status, setStatus] = useState<TorStatus | null>(null);
  const [control, setControl] = useState<TorControl | null>(null);
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: 'info' | 'error'; text: string } | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [st, ctl] = await Promise.all([getTorStatus(), getTorControl()]);
      setStatus(st);
      setControl(ctl);
      setSettings((prev) => prev ?? st.settings);
    } catch {
      /* keep last good values */
    }
  }, []);

  useEffect(() => {
    getTorSettings().then(setSettings).catch(() => {});
    refresh();
    const id = window.setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, [refresh]);

  const apply = async (patch: Partial<TorSettings>) => {
    if (!settings) return;
    const next = { ...settings, ...patch };
    setSettings(next);
    setBusy(true);
    setFeedback(null);
    try {
      const saved = await saveTorSettings(next);
      setSettings(saved);
      setFeedback({ kind: 'info', text: 'Saved. Applying to the daemons...' });
      refresh();
    } catch {
      setFeedback({ kind: 'error', text: 'Failed to save Tor settings' });
    } finally {
      setBusy(false);
    }
  };

  const newIdentity = async () => {
    setBusy(true);
    setFeedback(null);
    try {
      await torNewIdentity();
      setFeedback({ kind: 'info', text: 'Requested a new Tor identity' });
      refresh();
    } catch {
      setFeedback({ kind: 'error', text: 'Failed to request a new identity (is Tor enabled?)' });
    } finally {
      setBusy(false);
    }
  };

  if (!settings) {
    return <div className="text-muted-foreground">Loading...</div>;
  }

  const rev = String(settings.rev);
  const applying =
    settings.enabled && (status?.daemons.some((d) => d.running && d.torRev !== rev) ?? false);

  const daemonBadge = (d: TorStatus['daemons'][number]) => {
    if (!d.running) {
      return <span className="text-xs text-muted-foreground">Not running</span>;
    }
    if (d.torRev !== rev) {
      return (
        <span className="inline-flex items-center gap-1 text-xs text-warning">
          <Loader2 className="h-3 w-3 animate-spin" /> Applying...
        </span>
      );
    }
    return d.tor ? (
      <span className="text-xs font-medium text-success">Via Tor</span>
    ) : (
      <span className="text-xs text-muted-foreground">Clearnet</span>
    );
  };

  return (
    <div className="space-y-6">
      {/* Settings */}
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <Shield className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Tor</h3>
        </div>
        <p className="text-sm text-muted-foreground">
          Route the node, wallet, Lightning, and DEX connections through a bundled Tor proxy to hide
          your IP address. Changes apply within a few seconds without restarting the app.
        </p>

        {feedback && (
          <div
            className={`flex items-center gap-2 text-sm ${
              feedback.kind === 'error' ? 'text-destructive' : 'text-success'
            }`}
          >
            {feedback.kind === 'error' ? (
              <AlertCircle className="h-4 w-4" />
            ) : (
              <CheckCircle2 className="h-4 w-4" />
            )}
            {feedback.text}
          </div>
        )}

        <Toggle
          label="Route connections through Tor"
          description="Send outbound traffic for every daemon through the Tor network."
          checked={settings.enabled}
          disabled={busy}
          onChange={(v) => apply({ enabled: v })}
        />
        <Toggle
          label="Stream isolation"
          description="Use a separate Tor circuit per connection. Recommended."
          checked={settings.isolation}
          disabled={busy || !settings.enabled}
          onChange={(v) => apply({ isolation: v })}
        />
        <Toggle
          label="Host an inbound onion service (dcrd)"
          description="Advertise a .onion address so other nodes can reach yours anonymously."
          checked={settings.dcrdOnion}
          disabled={busy || !settings.enabled}
          onChange={(v) => apply({ dcrdOnion: v })}
        />
        <div className="flex items-center justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
          <div>
            <span className="font-medium block">Circuit limit</span>
            <span className="text-sm text-muted-foreground block">
              Maximum concurrent Tor circuits when stream isolation is on.
            </span>
          </div>
          <input
            type="number"
            min={1}
            max={1000}
            defaultValue={settings.circuitLimit}
            disabled={busy || !settings.enabled}
            onBlur={(e) => {
              const n = parseInt(e.target.value, 10);
              if (!Number.isNaN(n) && n !== settings.circuitLimit) apply({ circuitLimit: n });
            }}
            className="w-20 px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm text-right disabled:opacity-50"
          />
        </div>
        <div className="flex items-start gap-2 text-sm text-muted-foreground">
          <Info className="h-4 w-4 shrink-0 mt-0.5" />
          <span>
            Changing any setting here relaunches every daemon. The wallet, Lightning, and the
            DEX will lock and need their passphrases entered again, and any running DEX bots
            stop.
          </span>
        </div>
      </div>

      {/* Health / control */}
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-lg font-semibold">Tor network</h3>
          <button
            type="button"
            onClick={newIdentity}
            disabled={busy || !control?.reachable}
            className="inline-flex items-center gap-2 rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted/30 disabled:opacity-50"
            title="Build fresh Tor circuits"
          >
            <RefreshCw className="h-3.5 w-3.5" /> New identity
          </button>
        </div>

        {control && !control.reachable ? (
          <div className="text-sm text-muted-foreground">
            Tor control port not reachable{control.error ? ` (${control.error})` : ''}.
          </div>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-sm">
            <Stat
              label="Bootstrap"
              value={control ? `${control.bootstrapPct}% ${control.bootstrapTag}` : '-'}
            />
            <Stat label="Circuits" value={control ? String(control.circuits) : '-'} />
            <Stat label="Proxy" value={status?.proxyReachable ? 'reachable' : 'down'} />
            <Stat label="Version" value={control?.version || '-'} />
            <Stat label="Sent" value={control ? formatBytes(control.bytesWritten) : '-'} />
            <Stat label="Received" value={control ? formatBytes(control.bytesRead) : '-'} />
          </div>
        )}

        {settings.dcrdOnion && status?.onionAddress && (
          <div className="p-3 rounded-lg bg-muted/10 border border-border/50">
            <div className="text-xs text-muted-foreground uppercase tracking-wide mb-1">
              dcrd onion address
            </div>
            <div className="font-mono text-sm break-all">{status.onionAddress}</div>
          </div>
        )}
      </div>

      {/* Daemon routing */}
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3">
        <div className="flex items-center gap-2">
          <h3 className="text-lg font-semibold">Daemon routing</h3>
          {applying && (
            <span className="inline-flex items-center gap-1 text-xs text-warning">
              <Loader2 className="h-3 w-3 animate-spin" /> applying...
            </span>
          )}
        </div>
        <div className="space-y-2">
          {(status?.daemons ?? []).map((d) => (
            <div
              key={d.name}
              className="flex items-center justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50"
            >
              <span className="font-medium">{daemonLabels[d.name] || d.name}</span>
              {daemonBadge(d)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
