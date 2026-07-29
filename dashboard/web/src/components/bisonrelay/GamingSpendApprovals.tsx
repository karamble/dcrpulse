// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { GamingSpend, decideGamingSpend, getGamingSpends } from '../../services/gamingApi';

const fmtDcr = (atoms: number): string => (atoms / 1e8).toFixed(8).replace(/\.?0+$/, '');
const fmtWhen = (unix: number): string => new Date(unix * 1000).toLocaleString();

// GamingSpendApprovals is where a person answers a game asking to spend.
//
// A game never holds a wallet key and never learns the passphrase; it asks, and
// this is the asking. Nothing about a request can be edited here - what gets
// signed is the amount and address recorded when it was made, so what is
// approved is what is shown.
export const GamingSpendApprovals = ({ onPending }: { onPending?: (n: number) => void }) => {
  const [spends, setSpends] = useState<GamingSpend[]>([]);
  const [passphrase, setPassphrase] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setSpends(await getGamingSpends());
    } catch {
      /* An unreachable list is not worth an error banner over the policy. */
    }
  }, []);

  useEffect(() => {
    void refresh();
    // A request expires on its own, so this has to keep looking rather than
    // leave somebody staring at one that can no longer be answered.
    const t = setInterval(() => void refresh(), 5000);
    return () => clearInterval(t);
  }, [refresh]);

  const decide = async (spend: GamingSpend, approve: boolean) => {
    if (approve && !passphrase) {
      setError('Approving a spend needs your wallet passphrase.');
      return;
    }
    setBusy(spend.id);
    setError(null);
    try {
      await decideGamingSpend(spend.id, approve, approve ? passphrase : undefined);
      setPassphrase('');
      await refresh();
    } catch (e) {
      const detail =
        (e as { response?: { data?: string } })?.response?.data ?? (e as Error)?.message ?? '';
      setError(String(detail).trim() || 'could not answer that request');
    } finally {
      setBusy(null);
    }
  };

  const pending = spends.filter((s) => s.state === 'pending');
  const recent = spends.filter((s) => s.state !== 'pending').slice(0, 8);

  // Told, not asked. The panel above could have sent word that it wants a
  // payment approved, and deliberately does not: this already polls, so it
  // knows without being told, and a message would hand a game a new thing to
  // say and a new moment to choose for information the host already has.
  useEffect(() => {
    onPending?.(pending.length);
  }, [pending.length, onPending]);

  if (pending.length === 0 && recent.length === 0) return null;

  return (
    <div className="space-y-2">
      {/* Wallet chrome, and it has to look like it - but said once. The
        * collapsible handle above this strip now carries the "dcrpulse · your
        * wallet" identity, so repeating it here read as a stutter. The handle
        * owns the name; this owns the asking. */}

      {error && (
        <div className="p-2 rounded-lg bg-destructive/10 border border-destructive/30 text-xs text-destructive">
          {error}
        </div>
      )}

      {pending.length > 0 && (
        <div className="space-y-2 p-3 rounded-lg bg-primary/5 border-2 border-primary/40">
          <p className="text-xs text-muted-foreground">
            A game is asking to spend from the account you bound. It never learns your
            passphrase and never chooses where the money goes. Nothing moves unless you say
            so, and what gets signed is what was recorded when it asked — which is exactly
            what is shown here, not anything it could change since.
          </p>
          <input
            type="password"
            value={passphrase}
            onChange={(e) => setPassphrase(e.target.value)}
            placeholder="Wallet passphrase"
            className="w-full px-2 py-1.5 rounded-md bg-background border border-border/50 text-sm"
          />
          {pending.map((s) => (
            <div key={s.id} className="flex items-start justify-between gap-3 text-sm">
              <div className="min-w-0">
                <p className="font-medium">
                  {s.game} wants {fmtDcr(s.amountAtoms)} DCR
                </p>
                <p className="text-xs text-muted-foreground break-all">
                  {s.reason ? `${s.reason} — ` : ''}to {s.address}
                </p>
                <p className="text-xs text-muted-foreground">
                  Asked {fmtWhen(s.requestedAt)}; lapses {fmtWhen(s.expiresAt)}.
                </p>
              </div>
              <div className="flex shrink-0 gap-1">
                <button
                  type="button"
                  onClick={() => void decide(s, true)}
                  disabled={busy === s.id}
                  className="inline-flex items-center gap-1 px-2 py-1 rounded-md bg-primary/20 hover:bg-primary/30 disabled:opacity-50 text-xs font-medium"
                >
                  {busy === s.id && <Loader2 className="h-3 w-3 animate-spin" />}
                  Approve
                </button>
                <button
                  type="button"
                  onClick={() => void decide(s, false)}
                  disabled={busy === s.id}
                  className="px-2 py-1 rounded-md bg-muted/20 hover:bg-muted/30 disabled:opacity-50 text-xs font-medium"
                >
                  Deny
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {recent.length > 0 && (
        <div className="space-y-1 p-3 rounded-lg bg-muted/10 border border-border/50">
          <p className="text-xs text-muted-foreground">
            What games have asked for, and what happened. Refusals are kept as well as payments.
          </p>
          {recent.map((s) => (
            <div key={s.id} className="flex items-baseline justify-between gap-3 text-xs">
              <span className="truncate">
                {s.game} — {fmtDcr(s.amountAtoms)} DCR{s.reason ? ` — ${s.reason}` : ''}
              </span>
              <span
                className={`shrink-0 ${
                  s.state === 'approved' ? 'text-muted-foreground' : 'text-muted-foreground/70'
                }`}
                title={s.error || s.txid || ''}
              >
                {s.state}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
