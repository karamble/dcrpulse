// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Network, AlertTriangle, Loader2 } from 'lucide-react';
import { setMCPAgentAllowedIPs } from '../../services/api';
import { ConfigSection } from './ConfigSection';
import { toYMDTime } from '../../utils/date';

interface Props {
  agentId: string;
  allowedIps: string[];
  lastDenied?: { ip: string; at: string };
  onChanged: () => void;
}

export const AgentAllowedIPs = ({ agentId, allowedIps, lastDenied, onChanged }: Props) => {
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [draft, setDraft] = useState('');

  const openForm = () => {
    setDraft(allowedIps.join('\n'));
    setError(null);
    setEditing(true);
  };

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await setMCPAgentAllowedIPs(
        agentId,
        draft.split(/[\n,]/).map((s) => s.trim()).filter(Boolean),
      );
      setEditing(false);
      onChanged();
    } catch (e) {
      const data = (e as { response?: { data?: unknown } }).response?.data;
      setError(
        typeof data === 'string' && data ? data.trim() : 'Failed to update the allowed IP addresses.',
      );
    } finally {
      setBusy(false);
    }
  };

  const allowDenied = async () => {
    if (!lastDenied) return;
    setBusy(true);
    setError(null);
    try {
      await setMCPAgentAllowedIPs(agentId, [...allowedIps, lastDenied.ip]);
      onChanged();
    } catch (e) {
      const data = (e as { response?: { data?: unknown } }).response?.data;
      setError(
        typeof data === 'string' && data ? data.trim() : 'Failed to update the allowed IP addresses.',
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <ConfigSection
      icon={Network}
      tone="muted"
      title="Allowed IP addresses"
      description="Where this agent may connect from. One entry per line: a single IP or a CIDR range (e.g. 192.168.1.7 or 10.0.0.0/8, IPv4 or IPv6). Empty means any address. Behind Docker or a reverse proxy the dashboard sees the bridge or proxy address, not the agent's own - check the address shown next to Connected while the agent is online, and allow that."
    >
      {error && (
        <div className="flex items-start gap-2 rounded-lg bg-destructive/10 border border-destructive/40 p-2 text-xs text-destructive">
          <AlertTriangle className="h-3.5 w-3.5 shrink-0 mt-0.5" /> {error}
        </div>
      )}

      {lastDenied && (
        <div className="flex items-center justify-between gap-2 rounded-lg bg-warning/10 border border-warning/40 p-2 text-xs text-warning">
          <span className="min-w-0">
            <AlertTriangle className="h-3.5 w-3.5 inline mr-1.5 align-text-bottom" />
            An agent using this token was denied from{' '}
            <span className="font-mono">{lastDenied.ip}</span> at{' '}
            {toYMDTime(new Date(lastDenied.at))}.
          </span>
          <button
            type="button"
            onClick={allowDenied}
            disabled={busy}
            className="shrink-0 rounded-lg bg-warning/20 px-3 py-1.5 font-medium text-warning hover:bg-warning/30 disabled:opacity-50"
          >
            Allow this IP
          </button>
        </div>
      )}

      {!editing ? (
        <div className="flex items-center justify-between gap-2">
          {allowedIps.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {allowedIps.map((ip) => (
                <span
                  key={ip}
                  className="inline-flex items-center rounded-md bg-muted/20 px-2 py-0.5 text-xs font-mono text-foreground"
                >
                  {ip}
                </span>
              ))}
            </div>
          ) : (
            <span className="text-xs text-muted-foreground">Any address</span>
          )}
          <button
            type="button"
            onClick={openForm}
            disabled={busy}
            className="shrink-0 rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-foreground hover:bg-muted/30 disabled:opacity-50"
          >
            Edit
          </button>
        </div>
      ) : (
        <div className="space-y-2">
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            rows={3}
            placeholder="One IP or CIDR per line"
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-xs font-mono text-foreground"
          />
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={save}
              disabled={busy}
              className="inline-flex items-center gap-2 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {busy && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              Save
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              disabled={busy}
              className="rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted/30 disabled:opacity-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </ConfigSection>
  );
};
