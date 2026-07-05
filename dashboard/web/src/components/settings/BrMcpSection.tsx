// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Check, Copy, HelpCircle, Loader2, Radio, RefreshCw } from 'lucide-react';
import {
  BrMcpPendingPayment,
  BrMcpSettings,
  BrMcpSpendEntry,
  getBrMcpPending,
  getBrMcpSettings,
  getBrMcpSpend,
  resolveBrMcpPending,
  setBrMcpSettings,
} from '../../services/bisonrelayApi';
import { McpHelpModal } from './McpHelpModal';

const UID_RE = /^[0-9a-f]{64}$/i;

// The BR-MCP listener is published on port 8891 (brclientd's mcplisten, fixed
// for the container). The host cannot be known from the dashboard on a headless
// appliance (behind an app proxy), so the connect command carries a placeholder
// the user fills in with their box's address (127.0.0.1 on the same machine).
const BR_MCP_CONNECT_URL = 'http://<ip address>:8891/mcp/<bot-uid>';

const fmtDcr = (v: number) =>
  `${v.toLocaleString(undefined, { maximumFractionDigits: 8 })} DCR`;

// BR-MCP: brclientd can call MCP tool services offered by Bison Relay bots
// and expose them to local agents at /mcp/<bot-uid>. This section configures
// the listener, the payment mode (approval vs auto-pay), the caps, and the
// callable-bot allowlist, and resolves payments parked for approval.
export const BrMcpSection = () => {
  const [settings, setSettings] = useState<BrMcpSettings | null>(null);
  const [draft, setDraft] = useState<BrMcpSettings | null>(null);
  const [pending, setPending] = useState<BrMcpPendingPayment[]>([]);
  const [spend, setSpend] = useState<{ entries: BrMcpSpendEntry[]; todayDcr: number }>({
    entries: [],
    todayDcr: 0,
  });
  const [newBot, setNewBot] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [showHelp, setShowHelp] = useState(false);

  const refreshSettings = useCallback(async () => {
    try {
      const s = await getBrMcpSettings();
      setSettings(s);
      setDraft((d) => d ?? s);
      setError(null);
    } catch {
      /* brclientd may still be starting; keep last state */
    }
  }, []);

  const refreshLive = useCallback(async () => {
    try {
      setPending(await getBrMcpPending());
      setSpend(await getBrMcpSpend());
    } catch {
      /* ignore */
    }
  }, []);

  useEffect(() => {
    refreshSettings();
  }, [refreshSettings]);

  useEffect(() => {
    if (!settings?.enabled) return;
    refreshLive();
    const id = window.setInterval(refreshLive, 5000);
    return () => clearInterval(id);
  }, [settings?.enabled, refreshLive]);

  const apply = async (next: BrMcpSettings) => {
    setBusy(true);
    setError(null);
    try {
      const applied = await setBrMcpSettings(next);
      setSettings(applied);
      setDraft(applied);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Save failed');
    } finally {
      setBusy(false);
    }
  };

  const resolve = async (id: string, approve: boolean) => {
    try {
      await resolveBrMcpPending(id, approve);
    } catch {
      /* the entry may have timed out already */
    }
    refreshLive();
  };

  const copyToken = () => {
    if (!settings?.token || !navigator.clipboard) return;
    navigator.clipboard.writeText(settings.token).then(
      () => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      },
      () => {},
    );
  };

  // Recycle the bearer token: clearing it makes brclientd mint a fresh one and
  // live-restart the listener. Any agent still using the old token is cut off,
  // so confirm first.
  const recycleToken = () => {
    if (!draft) return;
    if (
      !window.confirm(
        'Generate a new bearer token? Any agent using the current token will stop working until you give it the new one.',
      )
    ) {
      return;
    }
    apply({ ...draft, token: '' });
  };

  if (!settings || !draft) {
    return null;
  }

  const addBot = () => {
    const uid = newBot.trim().toLowerCase();
    if (!UID_RE.test(uid) || draft.allowedBots.includes(uid)) return;
    setDraft({ ...draft, allowedBots: [...draft.allowedBots, uid] });
    setNewBot('');
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
      <div className="flex items-center gap-2">
        <Radio className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">BR-MCP (remote tools over Bison Relay)</h3>
      </div>
      <p className="text-sm text-muted-foreground">
        Call MCP tool services offered by Bison Relay bots. brclientd relays each session over
        the relay and exposes it to local agents as a streamable HTTP endpoint per bot. Paid
        tools settle as Bison Relay tips (the clients exchange and pay Lightning invoices
        under the hood), capped below; you choose whether every payment needs your approval.
      </p>

      <div className="flex items-center justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
        <div>
          <span className="font-medium flex items-center gap-1.5">
            BR-MCP endpoint
            <button
              type="button"
              onClick={() => setShowHelp(true)}
              className="text-muted-foreground hover:text-foreground"
              aria-label="How to connect an AI agent"
              title="How to connect an AI agent"
            >
              <HelpCircle className="h-4 w-4" />
            </button>
          </span>
          <span className="text-sm text-muted-foreground block">
            {settings.enabled
              ? `Agents connect to ${BR_MCP_CONNECT_URL}`
              : `Off. When on, agents connect to ${BR_MCP_CONNECT_URL}.`}
          </span>
        </div>
        <button
          type="button"
          onClick={() => apply({ ...draft, enabled: !settings.enabled })}
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

      {showHelp && (
        <McpHelpModal
          title="Connect an AI agent to BR-MCP"
          agentName="braibot"
          connectUrl={BR_MCP_CONNECT_URL}
          token={settings.token || undefined}
          tokenHint="Enable BR-MCP to generate a bearer token."
          onClose={() => setShowHelp(false)}
        />
      )}
      {settings.enabled && settings.token && (
        <div className="flex items-center gap-2 p-3 rounded-lg bg-muted/10 border border-border/50 text-xs">
          <span className="text-muted-foreground shrink-0">Bearer token</span>
          <code className="font-mono truncate flex-1">{settings.token}</code>
          <button
            type="button"
            onClick={copyToken}
            className="p-1 rounded text-muted-foreground hover:text-foreground"
            aria-label="Copy token"
          >
            {copied ? <Check className="h-3.5 w-3.5 text-success" /> : <Copy className="h-3.5 w-3.5" />}
          </button>
          <button
            type="button"
            onClick={recycleToken}
            disabled={busy}
            className="p-1 rounded text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Generate a new token"
            title="Generate a new token (revokes the current one)"
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Payment mode</span>
          <select
            value={draft.mode}
            onChange={(e) => setDraft({ ...draft, mode: e.target.value as 'approval' | 'autopay' })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          >
            <option value="approval">Approval: ask me for every payment</option>
            <option value="autopay">Auto-pay: pay under the caps without asking</option>
          </select>
        </label>
        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Approval wait (seconds)</span>
          <input
            type="number"
            min={10}
            value={draft.approvalTimeoutSecs}
            onChange={(e) =>
              setDraft({ ...draft, approvalTimeoutSecs: Number(e.target.value) || 0 })
            }
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
        </label>
        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Payment wait (seconds)</span>
          <input
            type="number"
            min={10}
            value={draft.tipWaitSecs}
            onChange={(e) => setDraft({ ...draft, tipWaitSecs: Number(e.target.value) || 0 })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
        </label>
        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Per-call cap (DCR, 0 = never pay)</span>
          <input
            type="number"
            min={0}
            step="0.0001"
            value={draft.perCallCapDcr}
            onChange={(e) => setDraft({ ...draft, perCallCapDcr: Number(e.target.value) || 0 })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
        </label>
        <label className="text-xs space-y-1">
          <span className="text-muted-foreground block">Daily cap (DCR, 0 = never pay)</span>
          <input
            type="number"
            min={0}
            step="0.001"
            value={draft.perDayCapDcr}
            onChange={(e) => setDraft({ ...draft, perDayCapDcr: Number(e.target.value) || 0 })}
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
        </label>
      </div>

      <div className="space-y-2">
        <span className="text-xs text-muted-foreground block">
          Callable bots (64-hex Bison Relay uids; nothing is callable until listed)
        </span>
        <div className="flex gap-2">
          <input
            value={newBot}
            onChange={(e) => setNewBot(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                addBot();
              }
            }}
            placeholder="bot uid"
            className="flex-1 px-2 py-1.5 rounded-lg bg-background border border-border text-sm font-mono"
          />
          <button
            type="button"
            onClick={addBot}
            disabled={!UID_RE.test(newBot.trim())}
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-muted/20 hover:bg-muted/30 disabled:opacity-50"
          >
            Add
          </button>
        </div>
        {draft.allowedBots.map((b) => (
          <div
            key={b}
            className="flex items-center gap-2 p-2 rounded-lg bg-muted/10 border border-border/50 text-xs"
          >
            <code className="font-mono truncate flex-1">{b}</code>
            <button
              type="button"
              onClick={() =>
                setDraft({ ...draft, allowedBots: draft.allowedBots.filter((x) => x !== b) })
              }
              className="text-muted-foreground hover:text-destructive"
            >
              Remove
            </button>
          </div>
        ))}
      </div>

      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => apply(draft)}
          disabled={busy}
          className="px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold disabled:opacity-50"
        >
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Save BR-MCP settings'}
        </button>
        {error && <span className="text-xs text-destructive">{error}</span>}
      </div>

      {settings.enabled && (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-semibold">Pending payment approvals</h4>
            <span className="text-xs text-muted-foreground">
              Spent last 24h: {fmtDcr(spend.todayDcr)}
            </span>
          </div>
          {pending.length === 0 ? (
            <p className="text-xs text-muted-foreground">Nothing waiting.</p>
          ) : (
            pending.map((p) => (
              <div
                key={p.id}
                className="flex items-center justify-between gap-3 p-2 rounded-lg bg-warning/10 border border-warning/30 text-xs"
              >
                <div className="min-w-0">
                  <span className="font-medium">{p.tool}</span>
                  <span className="text-muted-foreground"> · bot {p.bot.slice(0, 8)}</span>
                  <span className="text-muted-foreground"> · {fmtDcr(p.amountDcr)}</span>
                  <span className="text-muted-foreground">
                    {' · '}
                    {new Date(p.created * 1000).toLocaleTimeString()}
                  </span>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <button
                    type="button"
                    onClick={() => resolve(p.id, true)}
                    className="px-2.5 py-1 rounded-md bg-success/20 text-success text-[11px] font-semibold hover:bg-success/30"
                  >
                    Approve
                  </button>
                  <button
                    type="button"
                    onClick={() => resolve(p.id, false)}
                    className="px-2.5 py-1 rounded-md bg-destructive/20 text-destructive text-[11px] font-semibold hover:bg-destructive/30"
                  >
                    Deny
                  </button>
                </div>
              </div>
            ))
          )}
          {spend.entries.length > 0 && (
            <div className="space-y-1">
              <h4 className="text-sm font-semibold">Recent payments</h4>
              {spend.entries
                .slice(-8)
                .reverse()
                .map((e, i) => (
                  <div
                    key={i}
                    className="flex items-center justify-between gap-3 p-2 rounded-lg bg-muted/10 border border-border/50 text-xs"
                  >
                    <div className="min-w-0">
                      <span className="font-medium">{e.tool}</span>
                      <span className="text-muted-foreground"> · bot {e.bot.slice(0, 8)}</span>
                      <span className="text-muted-foreground"> · {fmtDcr(e.amountDcr)}</span>
                      <span className="text-muted-foreground"> · {e.rail}</span>
                    </div>
                    <span className="text-muted-foreground shrink-0">
                      {new Date(e.ts * 1000).toLocaleTimeString()}
                    </span>
                  </div>
                ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
