// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { ShieldCheck, AlertTriangle, Loader2 } from 'lucide-react';
import {
  AccountInfo,
  MCPGrant,
  MCPWriteScope,
  setMCPGrant,
  revokeMCPGrant,
} from '../../services/api';

interface Props {
  agentId: string;
  grant?: MCPGrant;
  accounts: AccountInfo[];
  scopes: MCPWriteScope[];
  onChanged: () => void;
}

const fmtDcr = (n: number) => `${n.toLocaleString(undefined, { maximumFractionDigits: 8 })} DCR`;

// Spendable accounts only: exclude watch-only xpub accounts and the imported
// private-key bucket (account number 2^31-1).
const spendableAccounts = (accounts: AccountInfo[]) =>
  accounts.filter((a) => a.accountNumber < 2147483647);

export const AgentSpendGrant = ({ agentId, grant, accounts, scopes, onChanged }: Props) => {
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [selected, setSelected] = useState<Set<number>>(new Set(grant?.accounts ?? []));
  const [perTx, setPerTx] = useState(grant ? String(grant.perTxDcr) : '');
  const [daily, setDaily] = useState(grant ? String(grant.dailyDcr) : '');
  const [allowlist, setAllowlist] = useState((grant?.allowlist ?? []).join('\n'));
  const [expiryHours, setExpiryHours] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [scopeKeys, setScopeKeys] = useState<Set<string>>(new Set(grant?.writeScopes ?? []));

  const byKey = (k: string) => scopes.find((s) => s.key === k);
  const scopeLabel = (k: string) => byKey(k)?.label ?? k;
  const selectedNeedsFund = Array.from(scopeKeys).some((k) => byKey(k)?.fund);
  const selectedNeedsPass = Array.from(scopeKeys).some((k) => byKey(k)?.needsPass);

  const openForm = () => {
    setSelected(new Set(grant?.accounts ?? []));
    setPerTx(grant ? String(grant.perTxDcr) : '');
    setDaily(grant ? String(grant.dailyDcr) : '');
    setAllowlist((grant?.allowlist ?? []).join('\n'));
    setExpiryHours('');
    setPassphrase('');
    setScopeKeys(new Set(grant?.writeScopes ?? []));
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

  const toggleScope = (k: string) => {
    setScopeKeys((prev) => {
      const next = new Set(prev);
      if (next.has(k)) next.delete(k);
      else next.add(k);
      return next;
    });
  };

  const submit = async () => {
    if (selected.size === 0 && scopeKeys.size === 0) {
      setError('Select an account to spend from, or enable at least one action.');
      return;
    }
    // Fund-moving grants need explicit positive caps. Caps are literal limits
    // (0 permits nothing); there is no unlimited.
    if (selected.size > 0 || selectedNeedsFund) {
      if (!(parseFloat(perTx) > 0) || !(parseFloat(daily) > 0)) {
        setError('Enter a per-transaction and daily cap (greater than 0).');
        return;
      }
    }
    // The passphrase is only needed for spend (accounts) or signing scopes.
    if ((selected.size > 0 || selectedNeedsPass) && !passphrase) {
      setError('Enter the wallet passphrase (required for spend, voting, or staking access).');
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
        writeScopes: Array.from(scopeKeys),
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
          No spend access. This agent can read but cannot move funds or take write actions until you
          grant an account-scoped allowance and/or action scopes.
        </p>
      )}

      {!editing && grant && (
        <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <div className="text-muted-foreground">Accounts</div>
          <div>{grant.accounts.map(accountLabel).join(', ') || '-'}</div>
          <div className="text-muted-foreground">Per transaction</div>
          <div>{fmtDcr(grant.perTxDcr)}</div>
          <div className="text-muted-foreground">Daily</div>
          <div>
            {fmtDcr(grant.dailyDcr)}
            {grant.dailyDcr > 0 && (
              <span className="text-muted-foreground">
                {' '}
                ({fmtDcr(grant.spentTodayDcr)} spent, {fmtDcr(grant.remainingTodayDcr)} left)
              </span>
            )}
          </div>
          <div className="text-muted-foreground">Actions</div>
          <div>
            {(grant.writeScopes ?? []).map(scopeLabel).join(', ') || 'wallet send + staking only'}
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
              Per-transaction cap (DCR)
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
              Daily cap (DCR)
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

          <div>
            <div className="text-xs text-muted-foreground mb-1">
              Action scopes (fund scopes use the caps above; risk scopes are high blast-radius)
            </div>
            <div className="flex flex-wrap gap-2">
              {scopes.map((s) => {
                const on = scopeKeys.has(s.key);
                const base = s.risk
                  ? on
                    ? 'bg-warning/25 text-warning hover:bg-warning/35'
                    : 'bg-warning/10 text-warning/80 hover:bg-warning/20'
                  : on
                    ? 'bg-success/20 text-success hover:bg-success/30'
                    : 'bg-muted/20 text-muted-foreground hover:bg-muted/30';
                return (
                  <button
                    key={s.key}
                    type="button"
                    title={`${s.domain}${s.fund ? ' - spends DCR (caps apply)' : ''}${s.needsPass ? ' - signs with passphrase' : ''}${s.risk ? ' - high blast-radius' : ''}`}
                    onClick={() => toggleScope(s.key)}
                    className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${base}`}
                  >
                    {s.label}
                    {s.fund && ' *'}
                  </button>
                );
              })}
            </div>
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
