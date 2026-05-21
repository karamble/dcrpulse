// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, ExternalLink, RefreshCw, Send, ShieldCheck, Wallet } from 'lucide-react';
import {
  AccountInfo,
  TicketRecord,
  VSPInfo,
  getAccounts,
  listTickets,
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
  if (!unixSec) return '—';
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

export const TicketStatusTab = () => {
  const [tickets, setTickets] = useState<TicketRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [account, setAccount] = useState<number | null>(null);
  const [vsp, setVsp] = useState<VSPInfo | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [syncStatus, setSyncStatus] = useState<string | null>(null);

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
        if (!cancelled) {
          setAccounts(accs.filter((a) => a.accountName !== 'imported' && a.accountName !== 'mixed'));
        }
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
    setSyncStatus(null);
    try {
      await syncFailedVSPTickets({
        vspHost: vsp.host,
        vspPubkey: vsp.pubkey,
        account,
        changeAccount: account,
        passphrase,
      });
      setSyncStatus('Sync requested — refreshing ticket list.');
      setModalOpen(false);
      await load();
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Sync failed';
      throw new Error(msg);
    }
  };

  const canSync = vsp !== null && account !== null;

  return (
    <div className="space-y-6">
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Sync Failed VSP Tickets</h3>
        </div>
        <p className="text-sm text-muted-foreground">
          Retries fee payment for tickets with VSP fee errors. You can also use this to migrate
          tracked tickets to a different VSP.
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

        {syncStatus && (
          <div className="flex items-center gap-2 text-sm text-success">
            <RefreshCw className="h-4 w-4" />
            {syncStatus}
          </div>
        )}

        <button
          onClick={() => setModalOpen(true)}
          disabled={!canSync}
          className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
        >
          <Send className="h-4 w-4" />
          Sync Failed VSP Tickets
        </button>
      </div>

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
                        {t.vspHost || '—'}
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
    </div>
  );
};
