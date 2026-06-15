// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { ShieldCheck, AlertTriangle, Loader2 } from 'lucide-react';
import { AccountInfo, MCPGrant, setMCPGrant, revokeMCPGrant } from '../../services/api';

interface Props {
  agentId: string;
  grant?: MCPGrant;
  accounts: AccountInfo[];
  onChanged: () => void;
}

const fmtDcr = (n: number) => `${n.toLocaleString(undefined, { maximumFractionDigits: 8 })} DCR`;
const cap = (n: number) => (n > 0 ? fmtDcr(n) : 'no limit');

// Spendable accounts only: exclude watch-only xpub accounts and the imported
// private-key bucket (account number 2^31-1).
const spendableAccounts = (accounts: AccountInfo[]) =>
  accounts.filter((a) => a.accountNumber < 2147483647);

export const AgentSpendGrant = ({ agentId, grant, accounts, onChanged }: Props) => {
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [selected, setSelected] = useState<Set<number>>(new Set(grant?.accounts ?? []));
  const [perTx, setPerTx] = useState(grant ? String(grant.perTxDcr) : '');
  const [daily, setDaily] = useState(grant ? String(grant.dailyDcr) : '');
  const [allowlist, setAllowlist] = useState((grant?.allowlist ?? []).join('\n'));
  const [expiryHours, setExpiryHours] = useState('');
  const [passphrase, setPassphrase] = useState('');

  const openForm = () => {
    setSelected(new Set(grant?.accounts ?? []));
    setPerTx(grant ? String(grant.perTxDcr) : '');
    setDaily(grant ? String(grant.dailyDcr) : '');
    setAllowlist((grant?.allowlist ?? []).join('\n'));
    setExpiryHours('');
    setPassphrase('');
    setError(null);
    setEditing(true);
  };

  const toggleAccount = (n: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(n)) next.delete(n);
      else next.add(n);
      return next;
    });
  };

  const submit = async () => {
    if (selected.size === 0) {
      setError('Select at least one account.');
      return;
    }
    if (!passphrase) {
      setError('Enter the wallet passphrase.');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await setMCPGrant(agentId, {
        accounts: Array.from(selected),
        perTxDcr: parseFloat(perTx) || 0,
        dailyDcr: parseFloat(daily) || 0,
        allowlist: allowlist
          .split(/[\n,]/)
          .map((s) => s.trim())
          .filter(Boolean),
        expiryHours: parseFloat(expiryHours) || 0,
        passphrase,
      });
      setPassphrase('');
      setEditing(false);
      onChanged();
    } catch {
      setError('Failed to grant spend access. Check the passphrase and try again.');
    } finally {
      setBusy(false);
    }
  };

  const revoke = async () => {
    setBusy(true);
    setError(null);
    try {
      await revokeMCPGrant(agentId);
      onChanged();
    } catch {
      setError('Failed to revoke the spend grant.');
    } finally {
      setBusy(false);
    }
  };

  const accountLabel = (n: number) => {
    const a = accounts.find((x) => x.accountNumber === n);
    return a ? `${a.accountName} (#${n})` : `account #${n}`;
  };

  return (
    <div className="rounded-lg border border-border/50 bg-muted/5 p-3 space-y-3">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm font-medium">
          <ShieldCheck className={`h-4 w-4 ${grant ? 'text-success' : 'text-muted-foreground'}`} />
          Spend access
        </div>
        {!editing && (
          <div className="flex items-center gap-2">
            {grant && (
              <button
                type="button"
                onClick={revoke}
                disabled={busy}
                className="rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-destructive/20 hover:text-destructive disabled:opacity-50"
              >
                Revoke
              </button>
            )}
            <button
              type="button"
              onClick={openForm}
              disabled={busy}
              className="rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-foreground hover:bg-muted/30 disabled:opacity-50"
            >
              {grant ? 'Edit' : 'Grant spend access'}
            </button>
          </div>
        )}
      </div>

      {error && (
        <div className="flex items-center gap-2 text-xs text-destructive">
          <AlertTriangle className="h-3.5 w-3.5" /> {error}
        </div>
      )}

      {!editing && !grant && (
        <p className="text-xs text-muted-foreground">
          No spend access. This agent can read but cannot move funds until you grant an
          account-scoped allowance.
        </p>
      )}

      {!editing && grant && (
        <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <div className="text-muted-foreground">Accounts</div>
          <div>{grant.accounts.map(accountLabel).join(', ') || '-'}</div>
          <div className="text-muted-foreground">Per transaction</div>
          <div>{cap(grant.perTxDcr)}</div>
          <div className="text-muted-foreground">Daily</div>
          <div>
            {cap(grant.dailyDcr)}
            {grant.dailyDcr > 0 && (
              <span className="text-muted-foreground">
                {' '}
                ({fmtDcr(grant.spentTodayDcr)} spent, {fmtDcr(grant.remainingTodayDcr)} left)
              </span>
            )}
          </div>
          {grant.allowlist.length > 0 && (
            <>
              <div className="text-muted-foreground">Allowlist</div>
              <div>{grant.allowlist.length} address(es)</div>
            </>
          )}
          {grant.expiry && (
            <>
              <div className="text-muted-foreground">Expires</div>
              <div>{new Date(grant.expiry).toLocaleString()}</div>
            </>
          )}
        </div>
      )}

      {editing && (
        <div className="space-y-3">
          <div className="flex items-start gap-2 rounded-lg bg-warning/10 border border-warning/40 p-2 text-xs text-warning">
            <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" />
            <span>
              The dashboard verifies and then holds this passphrase in memory to spend on the
              agent's behalf, only within the limits below. The agent never sees it. Revoke any
              time; the passphrase is wiped on revoke and on restart.
            </span>
          </div>

          <div>
            <div className="text-xs text-muted-foreground mb-1">Accounts the agent may spend from</div>
            <div className="flex flex-wrap gap-2">
              {spendableAccounts(accounts).map((a) => {
                const on = selected.has(a.accountNumber);
                return (
                  <button
                    key={a.accountNumber}
                    type="button"
                    onClick={() => toggleAccount(a.accountNumber)}
                    className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                      on
                        ? 'bg-success/20 text-success hover:bg-success/30'
                        : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                    }`}
                  >
                    {a.accountName} (#{a.accountNumber})
                  </button>
                );
              })}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <label className="text-xs text-muted-foreground">
              Per-transaction cap (DCR, 0 = none)
              <input
                type="number"
                min={0}
                step="any"
                value={perTx}
                onChange={(e) => setPerTx(e.target.value)}
                className="mt-1 w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm text-foreground"
              />
            </label>
            <label className="text-xs text-muted-foreground">
              Daily cap (DCR, 0 = none)
              <input
                type="number"
                min={0}
                step="any"
                value={daily}
                onChange={(e) => setDaily(e.target.value)}
                className="mt-1 w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm text-foreground"
              />
            </label>
          </div>

          <label className="block text-xs text-muted-foreground">
            Recipient allowlist (optional, one address per line; empty = any)
            <textarea
              value={allowlist}
              onChange={(e) => setAllowlist(e.target.value)}
              rows={2}
              className="mt-1 w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-xs font-mono text-foreground"
            />
          </label>

          <div className="grid grid-cols-2 gap-3">
            <label className="text-xs text-muted-foreground">
              Expires in (hours, 0 = never)
              <input
                type="number"
                min={0}
                step="any"
                value={expiryHours}
                onChange={(e) => setExpiryHours(e.target.value)}
                className="mt-1 w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm text-foreground"
              />
            </label>
            <label className="text-xs text-muted-foreground">
              Wallet passphrase
              <input
                type="password"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                autoComplete="off"
                className="mt-1 w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm text-foreground"
              />
            </label>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={submit}
              disabled={busy}
              className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}
              {grant ? 'Update grant' : 'Grant spend access'}
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              disabled={busy}
              className="rounded-lg bg-muted/20 px-4 py-2 text-sm font-medium text-muted-foreground hover:bg-muted/30 disabled:opacity-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
};
