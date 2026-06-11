// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, CheckCircle2, ExternalLink, KeyRound, Landmark, Loader2 } from 'lucide-react';
import {
  TSpendPolicyEntry,
  TreasuryKeyPolicy,
  getTSpendPolicies,
  getTreasuryKeyPolicies,
  setTSpendPolicy,
  setTreasuryKeyPolicy,
} from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';

const POLICIES = ['yes', 'no', 'abstain'] as const;
type Policy = (typeof POLICIES)[number];

const policyLabel: Record<Policy, string> = {
  yes: 'Approve',
  no: 'Reject',
  abstain: 'Abstain',
};

const policyStyle = (current: string, candidate: Policy) =>
  current === candidate
    ? 'border-primary/40 bg-primary/10 text-foreground'
    : 'border-border/50 bg-muted/10 hover:bg-muted/20';

const truncateHex = (s: string) => (s.length > 20 ? `${s.slice(0, 10)}...${s.slice(-10)}` : s);

interface PendingChange {
  kind: 'key' | 'tspend';
  id: string;
  policy: Policy;
}

export const TreasuryTab = () => {
  const [keys, setKeys] = useState<TreasuryKeyPolicy[]>([]);
  const [tspends, setTspends] = useState<TSpendPolicyEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState<PendingChange | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);

  const load = async () => {
    setError(null);
    try {
      const k = await getTreasuryKeyPolicies();
      setKeys(k);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to load treasury policies');
    } finally {
      setLoading(false);
    }
    // TSpend list is wallet-dependent; fetch independently so a transient
    // walletrpc.VotingService outage does not hide the sanctioned PiKey
    // cards above.
    try {
      const t = await getTSpendPolicies();
      setTspends(t);
    } catch {
      setTspends([]);
    }
  };

  useEffect(() => {
    load();
    const id = window.setInterval(load, 30000);
    return () => window.clearInterval(id);
  }, []);

  const handleSubmit = async (passphrase: string) => {
    if (!pending) return;
    try {
      if (pending.kind === 'key') {
        await setTreasuryKeyPolicy(pending.id, pending.policy, passphrase);
      } else {
        await setTSpendPolicy(pending.id, pending.policy, passphrase);
      }
      setFeedback(
        pending.kind === 'key'
          ? `Saved policy "${pending.policy}" for treasury key.`
          : `Saved policy "${pending.policy}" for TSpend.`,
      );
      setPending(null);
      await load();
    } catch (err: any) {
      const body = err?.response?.data;
      throw new Error(typeof body === 'string' ? body : err?.message || 'Failed to set policy');
    }
  };

  if (loading && keys.length === 0 && tspends.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading treasury policies...
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {error && (
        <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}
      {feedback && (
        <div className="flex items-center gap-2 text-sm text-success">
          <CheckCircle2 className="h-4 w-4" />
          {feedback}
        </div>
      )}

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <KeyRound className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Politeia Keys</h3>
        </div>
        <p className="text-sm text-muted-foreground">
          Blanket policy applied to all future TSpends signed by a given Politeia treasury key.
          Per-TSpend overrides below take precedence on individual spends.
        </p>
        {keys.map((k) => {
          const effective = k.policy || 'abstain';
          return (
            <div
              key={k.key}
              className="p-3 rounded-lg bg-muted/10 border border-border/50 flex flex-wrap items-center gap-3"
            >
              <span className="font-mono text-xs text-muted-foreground flex-1 break-all">
                {k.key}
              </span>
              <div className="flex gap-2">
                {POLICIES.map((p) => (
                  <button
                    key={p}
                    type="button"
                    onClick={() => {
                      setFeedback(null);
                      setPending({ kind: 'key', id: k.key, policy: p });
                    }}
                    className={`px-3 py-1.5 rounded-lg border text-xs font-medium transition-colors ${policyStyle(effective, p)}`}
                  >
                    {policyLabel[p]}
                  </button>
                ))}
              </div>
            </div>
          );
        })}
      </div>

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <Landmark className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">TSpend Overrides</h3>
        </div>
        <p className="text-sm text-muted-foreground">
          Per-hash policies override the matching Politeia key policy for that specific TSpend.
          Active TSpends in the mempool show their requested amount and expiry below.
        </p>
        {tspends.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No TSpends currently tracked. The wallet only knows about TSpends it has observed in the
            mempool or in mined blocks.
          </p>
        ) : null}
        {tspends.map((t) => (
          <div
            key={t.hash}
            className="p-3 rounded-lg bg-muted/10 border border-border/50 space-y-2"
          >
            <div className="flex flex-wrap items-center gap-3">
              <Link
                to={`/explorer/tx/${t.hash}`}
                className="font-mono text-xs text-primary hover:underline inline-flex items-center gap-1 flex-1 break-all"
              >
                {truncateHex(t.hash)}
                <ExternalLink className="h-3 w-3 shrink-0" />
              </Link>
              <div className="flex gap-2">
                {POLICIES.map((p) => (
                  <button
                    key={p}
                    type="button"
                    onClick={() => {
                      setFeedback(null);
                      setPending({ kind: 'tspend', id: t.hash, policy: p });
                    }}
                    className={`px-3 py-1.5 rounded-lg border text-xs font-medium transition-colors ${policyStyle(t.policy, p)}`}
                  >
                    {policyLabel[p]}
                  </button>
                ))}
              </div>
            </div>
            {(t.amount || t.expiry) && (
              <div className="text-xs text-muted-foreground space-x-3">
                {t.amount ? (
                  <span>amount: {(t.amount / 1e8).toFixed(2)} DCR</span>
                ) : null}
                {t.expiry ? <span>expires at block {t.expiry.toLocaleString()}</span> : null}
              </div>
            )}
          </div>
        ))}
      </div>

      <PassphraseModal
        isOpen={pending !== null}
        title="Confirm Treasury Policy Change"
        description={
          pending
            ? `Set "${pending.policy}" for this ${pending.kind === 'key' ? 'treasury key' : 'TSpend'}. The wallet is unlocked briefly.`
            : ''
        }
        submitLabel="Save"
        busyLabel="Saving..."
        onSubmit={handleSubmit}
        onClose={() => setPending(null)}
      />
    </div>
  );
};
