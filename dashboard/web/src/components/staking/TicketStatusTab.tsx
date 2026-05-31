// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  HelpCircle,
  Loader2,
  Send,
  ShieldCheck,
  Wallet,
} from 'lucide-react';
import {
  AccountInfo,
  SyncFailedVSPTicketsResponse,
  TicketRecord,
  VSPInfo,
  getAccounts,
  listTickets,
  processUnmanagedVSPTickets,
  syncFailedVSPTickets,
} from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';
import { VSPSelect } from './VSPSelect';

const ACTIVE_STATES = new Set<TicketRecord['status']>(['UNMINED', 'IMMATURE', 'LIVE']);

const feeGroups: Array<{ key: TicketRecord['feeStatus'] | 'NONE'; label: string; tone: string }> = [
  { key: 'ERRORED', label: 'Fee Error', tone: 'border-destructive/30 bg-destructive/5' },
  { key: 'UNPAID', label: 'Unpaid Fee', tone: 'border-warning/30 bg-warning/5' },
  { key: 'PAID', label: 'Paid Fee', tone: 'border-info/30 bg-info/5' },
  { key: 'CONFIRMED', label: 'Confirmed Fee', tone: 'border-success/30 bg-success/5' },
  { key: 'NONE', label: 'Untracked', tone: 'border-border/50 bg-muted/5' },
];

const truncateHash = (h: string) => (h.length > 16 ? `${h.slice(0, 8)}…${h.slice(-8)}` : h);
const formatDcr = (v: number) => v.toFixed(8);
const formatAge = (unixSec: number) => {
  if (!unixSec) return '-';
  const seconds = Math.floor(Date.now() / 1000 - unixSec);
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
};

const statusBadge = (status: TicketRecord['status']) => {
  const styles: Record<TicketRecord['status'], string> = {
    UNMINED: 'bg-muted/20 text-muted-foreground',
    IMMATURE: 'bg-warning/10 text-warning',
    LIVE: 'bg-success/10 text-success',
    VOTED: 'bg-success/10 text-success',
    MISSED: 'bg-destructive/10 text-destructive',
    EXPIRED: 'bg-destructive/10 text-destructive',
    REVOKED: 'bg-destructive/10 text-destructive',
    UNKNOWN: 'bg-muted/20 text-muted-foreground',
  };
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${styles[status]}`}>{status}</span>
  );
};

// StatusLegendTooltip is a pure-hover (i) bubble explaining the ticket fee and
// lifecycle statuses. Mirrors the InfoTooltip pattern in BisonrelaySetupWizard.
const StatusLegendTooltip = () => (
  <span className="relative group inline-flex">
    <HelpCircle className="h-4 w-4 text-muted-foreground/60 hover:text-muted-foreground cursor-help" />
    <span className="pointer-events-none absolute left-6 top-0 w-80 p-3 rounded-md bg-background border border-border/50 shadow-lg text-xs text-foreground/90 opacity-0 group-hover:opacity-100 transition-opacity z-20 space-y-2">
      <span className="block">
        <span className="font-semibold">Fee status</span> (the goal is Confirmed; a ticket only
        votes once its fee is confirmed by the VSP):
      </span>
      <span className="block space-y-1">
        <span className="block"><b>Unpaid Fee</b>: fee tx started, not yet paid.</span>
        <span className="block"><b>Paid Fee</b>: fee paid, waiting for the VSP to confirm it.</span>
        <span className="block"><b>Confirmed Fee</b>: VSP registered the fee; the ticket will vote.</span>
        <span className="block"><b>Fee Error</b>: fee payment failed; the ticket will not vote until you Sync to retry.</span>
        <span className="block"><b>Untracked</b>: ticket not associated with a VSP.</span>
      </span>
      <span className="block">
        <span className="font-semibold">Lifecycle</span>: Live (eligible to vote), Unmined/Immature
        (maturing), Voted/Missed/Expired/Revoked (spent outcomes).
      </span>
    </span>
  </span>
);

// SyncResultPanel summarizes a sync run, framed around progress toward the
// Confirmed fee status (the only state in which the VSP votes the ticket).
const SyncResultPanel = ({ result }: { result: SyncFailedVSPTicketsResponse }) => {
  const { before, after, vspHost } = result;
  const notConfirmed = after.errored + after.unpaid + after.paid;

  let tone = 'border-info/30 bg-info/5 text-info';
  let verdict = '';
  if (after.confirmed > before.confirmed) {
    tone = 'border-success/30 bg-success/5 text-success';
    const n = after.confirmed - before.confirmed;
    verdict = `${n} more ticket${n === 1 ? '' : 's'} now Confirmed - the VSP has registered the fee and ${n === 1 ? 'it' : 'they'} will vote.`;
  } else if (notConfirmed === 0 && after.confirmed > 0) {
    tone = 'border-success/30 bg-success/5 text-success';
    verdict = 'All tracked tickets are Confirmed; their fees are registered with the VSP and they will vote.';
  } else if (after.errored > 0) {
    tone = 'border-warning/30 bg-warning/5 text-warning';
    verdict = `${after.errored} ticket${after.errored === 1 ? '' : 's'} still in Fee Error; the fee is not registered and ${after.errored === 1 ? 'it' : 'they'} will not vote yet. The VSP may need more time, re-sync shortly.`;
  } else if (after.paid > 0 || after.unpaid > 0) {
    tone = 'border-info/30 bg-info/5 text-info';
    verdict = `${notConfirmed} ticket${notConfirmed === 1 ? '' : 's'} awaiting fee confirmation; the VSP confirms the fee once its tx gains on-chain confirmations. This completes automatically, or re-sync to re-check now.`;
  } else {
    tone = 'border-border/50 bg-muted/5 text-muted-foreground';
    verdict = 'No VSP tickets to sync.';
  }

  const rows: Array<{ label: string; b: number; a: number }> = [
    { label: 'Fee Error', b: before.errored, a: after.errored },
    { label: 'Unpaid Fee', b: before.unpaid, a: after.unpaid },
    { label: 'Paid Fee', b: before.paid, a: after.paid },
    { label: 'Confirmed Fee', b: before.confirmed, a: after.confirmed },
  ].filter((r) => r.b > 0 || r.a > 0);

  return (
    <div className={`p-4 rounded-xl border text-sm space-y-2 ${tone}`}>
      <div className="flex items-center gap-2 font-medium">
        <CheckCircle2 className="h-4 w-4 shrink-0" />
        Sync complete for {vspHost}
      </div>
      <p>{verdict}</p>
      {rows.length > 0 && (
        <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-foreground/80">
          {rows.map((r) => (
            <span key={r.label} className="whitespace-nowrap">
              {r.label}: {r.b} {'->'} {r.a}
            </span>
          ))}
        </div>
      )}
    </div>
  );
};

// UnmanagedResultPanel summarizes a re-tracking run: how many previously
// untracked tickets are now associated with the VSP and moving toward Confirmed.
const UnmanagedResultPanel = ({ result }: { result: SyncFailedVSPTicketsResponse }) => {
  const { before, after, vspHost } = result;
  const tracked = (n: VSPFeeStatusCounts) => n.unpaid + n.paid + n.errored + n.confirmed;
  const newlyTracked = Math.max(0, tracked(after) - tracked(before));

  let tone = 'border-info/30 bg-info/5 text-info';
  let verdict = '';
  if (newlyTracked > 0) {
    tone = 'border-success/30 bg-success/5 text-success';
    verdict = `${newlyTracked} ticket${newlyTracked === 1 ? ' is' : 's are'} now tracked by ${vspHost}; ${newlyTracked === 1 ? 'its' : 'their'} fee will be confirmed by the VSP and ${newlyTracked === 1 ? 'it' : 'they'} will vote.`;
  } else {
    tone = 'border-border/50 bg-muted/5 text-muted-foreground';
    verdict = `No untracked tickets were claimed by ${vspHost}. If you bought them from a different VSP, run this again with that VSP selected.`;
  }

  return (
    <div className={`p-4 rounded-xl border text-sm space-y-2 ${tone}`}>
      <div className="flex items-center gap-2 font-medium">
        <CheckCircle2 className="h-4 w-4 shrink-0" />
        Re-tracking complete for {vspHost}
      </div>
      <p>{verdict}</p>
    </div>
  );
};

// VSPFeeStatusCounts is the shape returned in the before/after snapshots; kept
// local to type the panel helper above.
type VSPFeeStatusCounts = SyncFailedVSPTicketsResponse['before'];

export const TicketStatusTab = () => {
  const [tickets, setTickets] = useState<TicketRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [account, setAccount] = useState<number | null>(null);
  const [vsp, setVsp] = useState<VSPInfo | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [syncResult, setSyncResult] = useState<SyncFailedVSPTicketsResponse | null>(null);
  const [vspU, setVspU] = useState<VSPInfo | null>(null);
  const [modalOpenU, setModalOpenU] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [processResult, setProcessResult] = useState<SyncFailedVSPTicketsResponse | null>(null);

  const load = async () => {
    try {
      const list = await listTickets();
      setTickets(list);
      setError(null);
    } catch (err: any) {
      setError(err?.message || 'Failed to load tickets');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    load();
    getAccounts()
      .then((accs) => {
        if (cancelled) return;
        const usable = accs.filter((a) => a.accountName !== 'imported');
        setAccounts(usable);
        const mixed = usable.find((a) => a.accountName === 'mixed');
        if (mixed) setAccount(mixed.accountNumber);
      })
      .catch(() => {});
    const id = window.setInterval(load, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const active = useMemo(() => tickets.filter((t) => ACTIVE_STATES.has(t.status)), [tickets]);

  const grouped = useMemo(() => {
    const out: Record<string, TicketRecord[]> = { ERRORED: [], UNPAID: [], PAID: [], CONFIRMED: [], NONE: [] };
    for (const t of active) {
      const key = t.feeStatus || 'NONE';
      (out[key] ||= []).push(t);
    }
    return out;
  }, [active]);

  const handleSync = async (passphrase: string) => {
    if (!vsp || account === null) return;
    setSyncResult(null);
    setSyncing(true);
    try {
      const result = await syncFailedVSPTickets({
        vspHost: vsp.host,
        vspPubkey: vsp.pubkey,
        account,
        changeAccount: account,
        passphrase,
      });
      setSyncResult(result);
      setModalOpen(false);
      // Refresh the ticket list immediately so it reflects the new fee status.
      await load();
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Sync failed';
      throw new Error(msg);
    } finally {
      setSyncing(false);
    }
  };

  const handleProcessUnmanaged = async (passphrase: string) => {
    if (!vspU || account === null) return;
    setProcessResult(null);
    setProcessing(true);
    try {
      const result = await processUnmanagedVSPTickets({
        vspHost: vspU.host,
        vspPubkey: vspU.pubkey,
        account,
        changeAccount: account,
        passphrase,
      });
      setProcessResult(result);
      setModalOpenU(false);
      // Refresh the ticket list immediately so re-tracked tickets leave Untracked.
      await load();
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Failed to process tickets';
      throw new Error(msg);
    } finally {
      setProcessing(false);
    }
  };

  const canSync = vsp !== null && account !== null;
  const untrackedCount = grouped.NONE.length;
  const canProcess = vspU !== null && account !== null;

  return (
    <div className="space-y-6">
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Sync Failed VSP Tickets</h3>
          <StatusLegendTooltip />
        </div>
        <p className="text-sm text-muted-foreground">
          Retries fee payment for tickets with VSP fee errors and re-checks paid fees against the
          VSP. A ticket only votes once its fee reaches the Confirmed status. You can also use this
          to migrate tracked tickets to a different VSP.
        </p>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <div className="space-y-1">
            <label className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <Wallet className="h-4 w-4" />
              Fee account
            </label>
            <select
              value={account ?? ''}
              onChange={(e) => setAccount(e.target.value === '' ? null : Number(e.target.value))}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
            >
              <option value="" disabled>
                Select account
              </option>
              {accounts.map((a) => (
                <option key={a.accountNumber} value={a.accountNumber}>
                  {a.accountName} ({a.spendableBalance.toFixed(2)} DCR)
                </option>
              ))}
            </select>
          </div>
          <VSPSelect network="mainnet" value={vsp} onChange={setVsp} />
        </div>

        {syncResult && <SyncResultPanel result={syncResult} />}

        <button
          onClick={() => setModalOpen(true)}
          disabled={!canSync || syncing}
          className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
        >
          {syncing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
          {syncing ? 'Syncing…' : 'Sync Failed VSP Tickets'}
        </button>
      </div>

      {untrackedCount > 0 && (
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
          <div className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5 text-primary" />
            <h3 className="text-lg font-semibold">Process Unmanaged Tickets</h3>
          </div>
          <p className="text-sm text-muted-foreground">
            You have {untrackedCount} live ticket{untrackedCount === 1 ? '' : 's'} that{' '}
            {untrackedCount === 1 ? 'is' : 'are'} not associated with a VSP (shown as Untracked
            below). This typically happens after restoring or importing a wallet: the tickets are
            recovered but their VSP fee records are not. Select the VSP you bought them from to
            re-associate them so their fees are confirmed and they keep voting. If you used more
            than one VSP, run this once per VSP.
          </p>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <div className="space-y-1">
              <label className="text-sm font-medium text-muted-foreground flex items-center gap-2">
                <Wallet className="h-4 w-4" />
                Fee account
              </label>
              <select
                value={account ?? ''}
                onChange={(e) => setAccount(e.target.value === '' ? null : Number(e.target.value))}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
              >
                <option value="" disabled>
                  Select account
                </option>
                {accounts.map((a) => (
                  <option key={a.accountNumber} value={a.accountNumber}>
                    {a.accountName} ({a.spendableBalance.toFixed(2)} DCR)
                  </option>
                ))}
              </select>
            </div>
            <VSPSelect network="mainnet" value={vspU} onChange={setVspU} />
          </div>

          {processResult && <UnmanagedResultPanel result={processResult} />}

          <button
            onClick={() => setModalOpenU(true)}
            disabled={!canProcess || processing}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
          >
            {processing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
            {processing ? 'Processing…' : 'Process Unmanaged Tickets'}
          </button>
        </div>
      )}

      {loading && tickets.length === 0 && (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
          Loading tickets…
        </div>
      )}

      {error && (
        <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}

      {!loading && active.length === 0 && !error && (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
          No active tickets. Purchase tickets on the Purchase tab to get started.
        </div>
      )}

      {feeGroups.map((g) => {
        const rows = grouped[g.key];
        if (!rows || rows.length === 0) return null;
        return (
          <div key={g.key} className={`p-6 rounded-xl border ${g.tone}`}>
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold">
                {g.label} <span className="text-muted-foreground font-normal">({rows.length})</span>
              </h3>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-muted-foreground border-b border-border/30">
                    <th className="py-2 pr-3">Ticket Hash</th>
                    <th className="py-2 pr-3">Lifecycle</th>
                    <th className="py-2 pr-3 text-right">Price</th>
                    <th className="py-2 pr-3">VSP</th>
                    <th className="py-2 pr-3">Purchased</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((t) => (
                    <tr key={t.hash} className="border-b border-border/20">
                      <td className="py-2 pr-3 font-mono">
                        <Link
                          to={`/explorer/tx/${t.hash}`}
                          className="text-primary hover:underline inline-flex items-center gap-1"
                        >
                          {truncateHash(t.hash)}
                          <ExternalLink className="h-3 w-3" />
                        </Link>
                      </td>
                      <td className="py-2 pr-3">{statusBadge(t.status)}</td>
                      <td className="py-2 pr-3 text-right font-mono whitespace-nowrap">
                        {formatDcr(t.ticketPrice)} DCR
                      </td>
                      <td className="py-2 pr-3 text-muted-foreground truncate max-w-[16rem]">
                        {t.vspHost || '-'}
                      </td>
                      <td className="py-2 pr-3 text-muted-foreground whitespace-nowrap">
                        {t.blockHeight > 0 ? (
                          <>
                            {t.blockHeight.toLocaleString()} · {formatAge(t.blockTime)}
                          </>
                        ) : (
                          'unmined'
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        );
      })}

      <PassphraseModal
        isOpen={modalOpen}
        title="Sync Failed VSP Tickets"
        description="Enter your private passphrase to authorise the VSP fee retry."
        submitLabel="Sync"
        busyLabel="Syncing…"
        onSubmit={handleSync}
        onClose={() => setModalOpen(false)}
      />

      <PassphraseModal
        isOpen={modalOpenU}
        title="Process Unmanaged Tickets"
        description="Enter your private passphrase to re-associate your untracked tickets with the VSP."
        submitLabel="Process"
        busyLabel="Processing…"
        onSubmit={handleProcessUnmanaged}
        onClose={() => setModalOpenU(false)}
      />
    </div>
  );
};
