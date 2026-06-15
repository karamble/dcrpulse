// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import {
  Bot,
  Plus,
  Trash2,
  Copy,
  Check,
  AlertTriangle,
  Loader2,
  KeyRound,
} from 'lucide-react';
import {
  MCPSettings,
  MCPAgent,
  AccountInfo,
  getMCPSettings,
  setMCPEnabled,
  createMCPToken,
  revokeMCPToken,
  setMCPAgentDomains,
  getAccounts,
} from '../../services/api';
import { AgentSpendGrant } from './AgentSpendGrant';

// Friendly labels for the capability domains. Unknown keys fall back to the raw
// domain name, so newly added tool domains still render without a code change.
const domainLabels: Record<string, string> = {
  node: 'Node & blockchain',
  wallet: 'Wallet',
  staking: 'Staking',
  governance: 'Governance',
  privacy: 'Privacy',
  lightning: 'Lightning',
  dex: 'DCRDEX',
  bisonrelay: 'Bison Relay',
  treasury: 'Treasury',
  explorer: 'Explorer',
  timestamp: 'Timestamps',
  tor: 'Tor',
};

const domainLabel = (d: string) => domainLabels[d] || d;

const fmtDate = (iso: string) => {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  return new Date(t).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
};

interface Feedback {
  kind: 'info' | 'error';
  text: string;
}

export const AgentsSection = () => {
  const [settings, setSettings] = useState<MCPSettings | null>(null);
  const [newName, setNewName] = useState('');
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState<Feedback | null>(null);
  const [created, setCreated] = useState<{ id: string; name: string; token: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [confirmRevoke, setConfirmRevoke] = useState<string | null>(null);
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);

  const refresh = useCallback(async () => {
    try {
      setSettings(await getMCPSettings());
    } catch {
      /* keep last good values */
    }
  }, []);

  useEffect(() => {
    refresh();
    getAccounts()
      .then(setAccounts)
      .catch(() => {});
    const id = window.setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, [refresh]);

  const toggleEnabled = async (next: boolean) => {
    setBusy(true);
    setFeedback(null);
    try {
      await setMCPEnabled(next);
      await refresh();
    } catch {
      setFeedback({
        kind: 'error',
        text: next
          ? 'Failed to start the MCP server (is the port already in use?).'
          : 'Failed to stop the MCP server.',
      });
    } finally {
      setBusy(false);
    }
  };

  const create = async () => {
    const name = newName.trim();
    if (!name) return;
    setBusy(true);
    setFeedback(null);
    try {
      const res = await createMCPToken(name);
      setCreated(res);
      setCopied(false);
      setNewName('');
      await refresh();
    } catch {
      setFeedback({ kind: 'error', text: 'Failed to create the agent token.' });
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (a: MCPAgent) => {
    setBusy(true);
    setFeedback(null);
    try {
      await revokeMCPToken(a.id);
      if (created?.id === a.id) setCreated(null);
      setConfirmRevoke(null);
      await refresh();
    } catch {
      setFeedback({ kind: 'error', text: 'Failed to revoke the agent.' });
    } finally {
      setBusy(false);
    }
  };

  const toggleDomain = async (a: MCPAgent, domain: string, on: boolean) => {
    if (domain === 'node') return; // node is always granted
    const set = new Set(a.domains.filter((d) => d !== 'node'));
    if (on) set.add(domain);
    else set.delete(domain);
    setBusy(true);
    setFeedback(null);
    try {
      await setMCPAgentDomains(a.id, Array.from(set));
      await refresh();
    } catch {
      setFeedback({ kind: 'error', text: 'Failed to update the agent access.' });
    } finally {
      setBusy(false);
    }
  };

  const copyToken = () => {
    if (!created || !navigator.clipboard) return;
    navigator.clipboard.writeText(created.token).then(
      () => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      },
      () => {},
    );
  };

  if (!settings) {
    return <div className="text-muted-foreground">Loading...</div>;
  }

  const activeIds = new Set(settings.sessions.map((s) => s.agentId));
  const grantable = settings.domains.filter((d) => d !== 'node');

  return (
    <div className="space-y-6">
      {/* Overview */}
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <Bot className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">AI Agents (MCP)</h3>
        </div>
        <p className="text-sm text-muted-foreground">
          Expose dcrpulse to AI agents over the Model Context Protocol. Each agent authenticates
          with its own bearer token and, on first connect, can only read node and blockchain
          status. Grant additional capabilities per agent below.
        </p>

        <div className="flex items-center justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
          <div>
            <span className="font-medium block">MCP server</span>
            <span className="text-sm text-muted-foreground block">
              {settings.enabled
                ? `Accepting agent connections on ${settings.bind}:${settings.port}`
                : `Off. When on, it listens on ${settings.bind}:${settings.port}.`}
            </span>
          </div>
          <button
            type="button"
            onClick={() => toggleEnabled(!settings.enabled)}
            disabled={busy}
            className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
              settings.enabled
                ? 'bg-success/20 text-success hover:bg-success/30'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            } disabled:opacity-50 disabled:cursor-wait`}
          >
            {settings.enabled ? 'On' : 'Off'}
          </button>
        </div>

        <div className="flex items-start gap-2 text-sm text-muted-foreground">
          <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />
          <span>
            The server binds to {settings.bind} (local only by default). Expose it beyond this
            machine only behind an authenticated, TLS-terminating reverse proxy. Each agent still
            needs its own token and starts limited to node status.
          </span>
        </div>
      </div>

      {feedback && (
        <div
          className={`flex items-center gap-2 text-sm ${
            feedback.kind === 'error' ? 'text-destructive' : 'text-success'
          }`}
        >
          <AlertTriangle className="h-4 w-4" />
          {feedback.text}
        </div>
      )}

      {/* Create token */}
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <KeyRound className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Create an agent token</h3>
        </div>
        <div className="flex flex-col sm:flex-row gap-3">
          <input
            type="text"
            value={newName}
            maxLength={64}
            placeholder="Agent name (e.g. trading-assistant)"
            disabled={busy}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') create();
            }}
            className="flex-1 px-3 py-2 rounded-lg bg-background border border-border/50 text-sm disabled:opacity-50"
          />
          <button
            type="button"
            onClick={create}
            disabled={busy || !newName.trim()}
            className="inline-flex items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
            Create
          </button>
        </div>

        {created && (
          <div className="p-4 rounded-lg bg-warning/10 border border-warning/40 space-y-2">
            <div className="flex items-center gap-2 text-sm font-medium text-warning">
              <AlertTriangle className="h-4 w-4" />
              Copy this token now. It is shown only once and cannot be recovered.
            </div>
            <div className="text-xs text-muted-foreground">
              Token for <span className="font-medium text-foreground">{created.name}</span>
            </div>
            <div className="flex items-center gap-2">
              <input
                readOnly
                value={created.token}
                onFocus={(e) => e.currentTarget.select()}
                className="flex-1 px-3 py-2 rounded-lg bg-background border border-border/50 font-mono text-xs break-all"
              />
              <button
                type="button"
                onClick={copyToken}
                title="Copy"
                className="shrink-0 inline-flex items-center justify-center rounded-lg bg-muted/20 p-2 text-muted-foreground hover:bg-muted/30"
              >
                {copied ? (
                  <Check className="h-4 w-4 text-success" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </button>
            </div>
            <div className="text-xs text-muted-foreground">
              Set it as the <span className="font-mono">Authorization: Bearer</span> header on the
              agent's MCP connection.
            </div>
          </div>
        )}
      </div>

      {/* Agent roster */}
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <h3 className="text-lg font-semibold">Agents</h3>
        {settings.agents.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No agents yet. Create a token above to let an agent connect.
          </p>
        ) : (
          <div className="space-y-3">
            {settings.agents.map((a) => {
              const connected = activeIds.has(a.id);
              const granted = new Set(a.domains);
              return (
                <div
                  key={a.id}
                  className="p-4 rounded-lg bg-muted/10 border border-border/50 space-y-3"
                >
                  <div className="flex items-start justify-between gap-4">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium truncate">{a.name}</span>
                        <span
                          className={`inline-flex items-center gap-1.5 text-xs ${
                            connected ? 'text-success' : 'text-muted-foreground'
                          }`}
                        >
                          <span
                            className={`h-2 w-2 rounded-full ${
                              connected ? 'bg-success' : 'bg-muted-foreground/40'
                            }`}
                          />
                          {connected ? 'Connected' : 'Idle'}
                        </span>
                      </div>
                      <div className="text-xs text-muted-foreground mt-0.5">
                        <span className="font-mono">{a.id}</span>
                        {fmtDate(a.createdAt) && <span> · created {fmtDate(a.createdAt)}</span>}
                      </div>
                    </div>
                    {confirmRevoke === a.id ? (
                      <div className="flex items-center gap-2 shrink-0">
                        <button
                          type="button"
                          onClick={() => revoke(a)}
                          disabled={busy}
                          className="rounded-lg bg-destructive/20 px-3 py-1.5 text-xs font-medium text-destructive hover:bg-destructive/30 disabled:opacity-50"
                        >
                          Confirm
                        </button>
                        <button
                          type="button"
                          onClick={() => setConfirmRevoke(null)}
                          disabled={busy}
                          className="rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted/30"
                        >
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <button
                        type="button"
                        onClick={() => setConfirmRevoke(a.id)}
                        disabled={busy}
                        title="Revoke this agent"
                        className="shrink-0 inline-flex items-center gap-1.5 rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-destructive/20 hover:text-destructive disabled:opacity-50"
                      >
                        <Trash2 className="h-3.5 w-3.5" /> Revoke
                      </button>
                    )}
                  </div>

                  <div>
                    <div className="text-xs text-muted-foreground uppercase tracking-wide mb-2">
                      Capabilities
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <span
                        className="inline-flex items-center rounded-lg bg-success/20 px-3 py-1.5 text-xs font-medium text-success"
                        title="Always available"
                      >
                        {domainLabel('node')} (always on)
                      </span>
                      {grantable.map((d) => {
                        const on = granted.has(d);
                        return (
                          <button
                            key={d}
                            type="button"
                            onClick={() => toggleDomain(a, d, !on)}
                            disabled={busy}
                            className={`inline-flex items-center rounded-lg px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50 ${
                              on
                                ? 'bg-success/20 text-success hover:bg-success/30'
                                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                            }`}
                          >
                            {domainLabel(d)}
                          </button>
                        );
                      })}
                    </div>
                  </div>

                  <AgentSpendGrant
                    agentId={a.id}
                    grant={settings.grants?.[a.id]}
                    accounts={accounts}
                    onChanged={refresh}
                  />
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Recent agent spends (audit) */}
      {(settings.audit?.length ?? 0) > 0 && (
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3">
          <h3 className="text-lg font-semibold">Recent agent spends</h3>
          <div className="space-y-1.5">
            {settings.audit.map((e, i) => (
              <div
                key={`${e.time}-${i}`}
                className="flex items-start justify-between gap-3 p-2 rounded-lg bg-muted/10 border border-border/50 text-xs"
              >
                <div className="min-w-0">
                  <div>
                    <span className="font-medium">{e.agent}</span>
                    <span className="text-muted-foreground"> · {e.tool}</span>
                    {e.amountDcr > 0 && (
                      <span className="text-muted-foreground">
                        {' · '}
                        {e.amountDcr.toLocaleString(undefined, { maximumFractionDigits: 8 })} DCR
                      </span>
                    )}
                  </div>
                  {e.detail && (
                    <div className="font-mono text-muted-foreground truncate">{e.detail}</div>
                  )}
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span
                    className={`rounded px-2 py-0.5 font-medium ${
                      e.result === 'ok'
                        ? 'bg-success/20 text-success'
                        : e.result === 'denied'
                          ? 'bg-warning/20 text-warning'
                          : 'bg-destructive/20 text-destructive'
                    }`}
                  >
                    {e.result}
                  </span>
                  <span className="text-muted-foreground">
                    {new Date(e.time).toLocaleTimeString()}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};
