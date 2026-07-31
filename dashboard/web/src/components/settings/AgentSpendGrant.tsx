// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import {
  ShieldCheck,
  AlertTriangle,
  Loader2,
  Zap,
  Wallet,
  Gauge,
  KeyRound,
  Clock,
  Coins,
} from 'lucide-react';
import {
  AccountInfo,
  MCPGrant,
  MCPWriteScope,
  setMCPGrant,
  revokeMCPGrant,
} from '../../services/api';
import { ConfigSection, domainLabels, domainLabel } from './ConfigSection';
import { isReservedAccount } from '../accounts/AccountRow';

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

  // Progressive reveal: limits matter once funds can move; the passphrase matters
  // once something signs. Mirrors the submit-time validation exactly.
  const needsLimits = selected.size > 0 || selectedNeedsFund;
  const needsAuth = selected.size > 0 || selectedNeedsPass;
  // A per-transaction cap above the daily one is legal but misleading: the
  // daily cap silently becomes the real ceiling.
  const capsInverted = parseFloat(perTx) > 0 && parseFloat(daily) > 0 && parseFloat(perTx) > parseFloat(daily);

  // Scopes grouped by their capability domain, in the canonical domain order.
  const scopeGroups = (() => {
    const groups = Object.keys(domainLabels)
      .map((d) => ({ domain: d, items: scopes.filter((s) => s.domain === d) }))
      .filter((g) => g.items.length > 0);
    const known = new Set(Object.keys(domainLabels));
    const extra = scopes.filter((s) => !known.has(s.domain));
    if (extra.length) groups.push({ domain: 'other', items: extra });
    return groups;
  })();

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
      setError('Select an account to spend from, or enable at least one action scope.');
      return;
    }
    // Fund-moving grants need explicit positive caps. Caps are literal limits
    // (0 permits nothing); there is no unlimited.
    if (needsLimits) {
      if (!(parseFloat(perTx) > 0) || !(parseFloat(daily) > 0)) {
        setError('Enter a per-transaction and daily cap (greater than 0).');
        return;
      }
    }
    // The passphrase is only needed for spend (accounts) or signing scopes.
    if (needsAuth && !passphrase) {
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

  // One scope chip: toggle button plus small badges for fund / signs / risk.
  const scopeChip = (s: MCPWriteScope) => {
    const on = scopeKeys.has(s.key);
    const base = s.risk
      ? on
        ? 'bg-warning/25 text-warning hover:bg-warning/35'
        : 'bg-warning/10 text-warning/80 hover:bg-warning/20'
      : on
        ? 'bg-success/20 text-success hover:bg-success/30'
        : 'bg-muted/20 text-muted-foreground hover:bg-muted/30';
    const hint = [
      s.fund && 'spends DCR (uses the limits below)',
      s.needsPass && 'signs with your wallet passphrase',
      s.risk && 'high-risk; grant sparingly',
    ]
      .filter(Boolean)
      .join('; ');
    return (
      <button
        key={s.key}
        type="button"
        onClick={() => toggleScope(s.key)}
        title={hint ? `${s.label} - ${hint}` : s.label}
        className={`inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${base}`}
      >
        {s.label}
        {s.fund && <Coins className="h-3 w-3" />}
        {s.needsPass && <KeyRound className="h-3 w-3" />}
        {s.risk && <AlertTriangle className="h-3 w-3" />}
      </button>
    );
  };

  return (
    <div className="rounded-lg border border-border/50 bg-muted/5 p-3 space-y-3">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm font-medium">
          <ShieldCheck className={`h-4 w-4 ${grant ? 'text-success' : 'text-muted-foreground'}`} />
          Spend &amp; action grant
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
        <div className="flex items-start gap-2 rounded-lg bg-destructive/10 border border-destructive/40 p-2 text-xs text-destructive">
          <AlertTriangle className="h-3.5 w-3.5 shrink-0 mt-0.5" /> {error}
        </div>
      )}

      {!editing && !grant && (
        <p className="text-xs text-muted-foreground">
          No spend access. This agent can read but cannot move funds or take write actions until you
          grant action scopes and/or account access.
        </p>
      )}

      {!editing && grant && (
        <div className="space-y-3">
          <div>
            <div className="flex items-center gap-1.5 text-xs font-medium text-foreground mb-1.5">
              <Zap className="h-3.5 w-3.5 text-primary" /> Action scopes
            </div>
            {grant.writeScopes?.length ? (
              <div className="flex flex-wrap gap-1.5">
                {grant.writeScopes.map((k) => {
                  const s = byKey(k);
                  return (
                    <span
                      key={k}
                      className={`inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs ${
                        s?.risk ? 'bg-warning/15 text-warning' : 'bg-muted/20 text-foreground'
                      }`}
                    >
                      {scopeLabel(k)}
                      {s?.fund && <Coins className="h-3 w-3" />}
                      {s?.needsPass && <KeyRound className="h-3 w-3" />}
                      {s?.risk && <AlertTriangle className="h-3 w-3" />}
                    </span>
                  );
                })}
              </div>
            ) : (
              <span className="text-xs text-muted-foreground">none</span>
            )}
          </div>

          <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
            <div className="flex items-center gap-1.5 text-muted-foreground">
              <Wallet className="h-3.5 w-3.5" /> Accounts
            </div>
            <div>{grant.accounts.map(accountLabel).join(', ') || '-'}</div>
            <div className="flex items-center gap-1.5 text-muted-foreground">
              <Gauge className="h-3.5 w-3.5" /> Per transaction
            </div>
            <div>{fmtDcr(grant.perTxDcr)}</div>
            <div className="text-muted-foreground pl-5">Daily</div>
            <div>
              {fmtDcr(grant.dailyDcr)}
              {grant.dailyDcr > 0 && (
                <span className="text-muted-foreground">
                  {' '}
                  ({fmtDcr(grant.spentTodayDcr)} spent, {fmtDcr(grant.remainingTodayDcr)} left)
                </span>
              )}
            </div>
            {grant.allowlist.length > 0 && (
              <>
                <div className="text-muted-foreground pl-5">Allowlist</div>
                <div>{grant.allowlist.length} address(es)</div>
              </>
            )}
            {grant.expiry && (
              <>
                <div className="flex items-center gap-1.5 text-muted-foreground">
                  <Clock className="h-3.5 w-3.5" /> Expires
                </div>
                <div>{new Date(grant.expiry).toLocaleString()}</div>
              </>
            )}
          </div>
        </div>
      )}

      {editing && (
        <div className="space-y-3">
          <ConfigSection
            icon={Zap}
            tone="primary"
            title="Action scopes"
            description={
              <>
                What the agent may do; each scope unlocks a group of write actions. A scope also
                needs its matching read-access domain above.
                <span className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-0.5">
                  <span className="inline-flex items-center gap-1">
                    <Coins className="h-3 w-3" /> spends DCR
                  </span>
                  <span className="inline-flex items-center gap-1">
                    <KeyRound className="h-3 w-3" /> signs with passphrase
                  </span>
                  <span className="inline-flex items-center gap-1">
                    <AlertTriangle className="h-3 w-3" /> high-risk
                  </span>
                </span>
              </>
            }
          >
            <div className="space-y-2">
              {scopeGroups.map((g) => (
                <div key={g.domain} className="flex items-start gap-2">
                  <span className="w-24 shrink-0 pt-1.5 text-xs text-muted-foreground">
                    {domainLabel(g.domain)}
                  </span>
                  <div className="flex flex-wrap gap-2">{g.items.map(scopeChip)}</div>
                </div>
              ))}
            </div>
          </ConfigSection>

          <ConfigSection
            icon={Wallet}
            tone="success"
            title="Account access"
            description="Wallet accounts the agent may spend DCR from - this enables send and ticket purchase from these accounts. Watch-only and imported accounts are excluded."
          >
            <div className="flex flex-wrap gap-2">
              {spendableAccounts(accounts).map((a) => {
                const on = selected.has(a.accountNumber);
                return (
                  <button
                    key={a.accountNumber}
                    type="button"
                    onClick={() => toggleAccount(a.accountNumber)}
                    title={
                      isReservedAccount(a)
                        ? `${a.accountName} is used by another part of the stack (mixer, Lightning or DEX). Granting it lets the agent spend funds those subsystems rely on.`
                        : undefined
                    }
                    className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                      on
                        ? isReservedAccount(a)
                          ? 'bg-warning/25 text-warning hover:bg-warning/35'
                          : 'bg-success/20 text-success hover:bg-success/30'
                        : isReservedAccount(a)
                          ? 'bg-warning/10 text-warning/80 hover:bg-warning/20'
                          : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
                    }`}
                  >
                    {a.accountName} (#{a.accountNumber})
                    {isReservedAccount(a) && <AlertTriangle className="ml-1 inline h-3 w-3" />}
                  </button>
                );
              })}
            </div>
            {selected.size > 0 && (
              <label className="block text-xs text-muted-foreground animate-fade-in">
                Restrict wallet sends to these addresses (optional; blank = any)
                <textarea
                  value={allowlist}
                  onChange={(e) => setAllowlist(e.target.value)}
                  rows={2}
                  placeholder="One address per line"
                  className="mt-1 w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-xs font-mono text-foreground"
                />
                <span className="mt-1 block text-[11px] text-muted-foreground/80">
                  Applies to wallet sends only. Lightning payments and DEX
                  withdrawals have their own destinations and are bounded by the
                  caps, not by this list.
                </span>
              </label>
            )}
          </ConfigSection>

          <ConfigSection
            icon={Gauge}
            tone="warning"
            dimmed={!needsLimits}
            title="Spending limits"
            description="Hard caps on the DCR this agent can move - absolute, there is no unlimited. One shared budget across wallet sends, ticket purchases, Lightning payments and DEX trades; both are required. A DEX withdrawal in another asset cannot be measured in DCR, so it is bounded by the scope alone."
          >
            <div className="grid grid-cols-2 gap-3 animate-fade-in">
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
            {grant && grant.dailyDcr > 0 && (
              <p className="text-xs text-muted-foreground">
                {fmtDcr(grant.spentTodayDcr)} spent today, {fmtDcr(grant.remainingTodayDcr)}{' '}
                remaining on the current grant.
              </p>
            )}
            {capsInverted && (
              <p className="text-xs text-warning">
                The per-transaction cap is above the daily cap, so the daily
                cap is the real limit.
              </p>
            )}
          </ConfigSection>

          <ConfigSection
            icon={KeyRound}
            tone="primary"
            dimmed={!needsAuth}
            title="Authorization"
            description="Confirm with your wallet passphrase. The dashboard verifies it once and holds it in memory to spend on the agent's behalf within these limits - the agent never sees it, and it is wiped on revoke and on restart."
          >
            <div className="grid grid-cols-2 gap-3 animate-fade-in">
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
              <label className="text-xs text-muted-foreground">
                <span className="inline-flex items-center gap-1">
                  <Clock className="h-3 w-3" /> Auto-revoke after (hours, 0 = never)
                </span>
                <input
                  type="number"
                  min={0}
                  step="any"
                  value={expiryHours}
                  onChange={(e) => setExpiryHours(e.target.value)}
                  className="mt-1 w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm text-foreground"
                />
              </label>
            </div>
          </ConfigSection>

          {scopeKeys.size > 0 && !needsLimits && !needsAuth && (
            <p className="text-xs text-muted-foreground">
              The selected scopes move no funds and need no passphrase, so no spending limit or
              authorization is required.
            </p>
          )}

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
