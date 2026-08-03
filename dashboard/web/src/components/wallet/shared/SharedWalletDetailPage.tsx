// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { AlertCircle, ArrowLeft, Download, Loader2, XCircle } from 'lucide-react';
import { KeyEnds } from '../../AddressGroups';
import { useBisonrelayLive } from '../../bisonrelay/BisonrelayLiveProvider';
import { CoordinationCard } from './CoordinationCard';
import { ReceiveCard } from './ReceiveCard';
import {
  MsigDetail,
  cancelMsigRound,
  getMsigBackup,
  getMsigDetail,
  msigPeerLabel,
  msigStatusLabel,
} from '../../../services/msigApi';
import { ProposalComposePanel } from './ProposalComposePanel';
import { ProposalList } from './ProposalList';

const formatDcr = (atoms: number): string => (atoms / 1e8).toFixed(8);

const downloadJson = (data: unknown, filename: string) => {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
};

// SharedWalletDetailPage shows one multisig wallet: its address, funds,
// cosigner states and backup card.
export const SharedWalletDetailPage = () => {
  const { id = '' } = useParams();
  const [detail, setDetail] = useState<MsigDetail | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const { addListener } = useBisonrelayLive();

  const load = useCallback(async () => {
    try {
      setDetail(await getMsigDetail(id));
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load this shared wallet');
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  // Reload on the frame's arrival AND on the engine's commit
  // (msig-state); the arrival event alone races the engine and would
  // render the pre-frame state. Debounced so a burst fetches once.
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | null = null;
    const remove = addListener((evt) => {
      if (evt.type !== 'msig' && evt.type !== 'msig-state') return;
      if (timer) clearTimeout(timer);
      timer = setTimeout(load, 300);
    });
    return () => {
      if (timer) clearTimeout(timer);
      remove();
    };
  }, [addListener, load]);

  const backup = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const card = await getMsigBackup(id);
      downloadJson(card, `shared-wallet-${detail?.record.label || id}.json`);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not export the backup');
    } finally {
      setBusy(false);
    }
  };

  const cancel = async () => {
    if (busy) return;
    setBusy(true);
    try {
      await cancelMsigRound(id);
      await load();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not cancel');
    } finally {
      setBusy(false);
    }
  };

  if (!detail) {
    return (
      <div className="p-6">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          {err ? (
            <span className="flex items-start gap-2 text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              {err}
            </span>
          ) : (
            <>
              <Loader2 className="h-4 w-4 animate-spin" /> Loading...
            </>
          )}
        </div>
      </div>
    );
  }

  const rec = detail.record;
  const active = rec.status === 'active';
  const proposals = Object.values(rec.proposals ?? {}).sort((a, b) => b.createdAt - a.createdAt);

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center gap-3 flex-wrap">
        <Link
          to="/wallet/shared"
          className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Shared wallets
        </Link>
        <div className="flex-1 min-w-0">
          <h1 className="text-2xl font-bold truncate">{rec.label}</h1>
          <p className="text-sm text-muted-foreground">
            {rec.m} of {rec.n} signatures - {msigStatusLabel(rec.status)}
            {!detail.isActiveWallet && ` - belongs to wallet ${detail.walletName}`}
          </p>
        </div>
        {active && (
          <button
            type="button"
            onClick={backup}
            disabled={busy}
            className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2 disabled:opacity-50"
          >
            <Download className="h-4 w-4" />
            Backup card
          </button>
        )}
        {rec.role === 'initiator' && rec.status === 'inviting' && (
          <button
            type="button"
            onClick={cancel}
            disabled={busy}
            className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2 disabled:opacity-50"
          >
            <XCircle className="h-4 w-4" />
            Cancel round
          </button>
        )}
      </div>

      {err && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      {rec.failReason && (
        <div className="p-4 rounded-xl bg-destructive/10 border border-destructive/20 text-sm text-destructive">
          {rec.failReason}
        </div>
      )}

      {active && rec.address ? (
        <div className="grid gap-4 lg:grid-cols-5">
          <div className="lg:col-span-3">
            {detail.receive ? (
              <ReceiveCard walletId={rec.tempId} receive={detail.receive} m={rec.m} n={rec.n} />
            ) : (
              <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
                <p className="text-sm font-medium mb-3">Receive</p>
                <p className="text-sm text-muted-foreground">
                  Switch to wallet {detail.walletName} to receive into this shared wallet.
                </p>
              </div>
            )}
          </div>

          <div className="p-6 rounded-xl bg-gradient-card border border-border/50 lg:col-span-2">
            <p className="text-sm font-medium mb-3">Funds</p>
            {detail.isActiveWallet ? (
              <>
                <p className="text-3xl font-bold">{formatDcr(detail.balanceAtoms)} DCR</p>
                <p className="text-sm text-muted-foreground mt-1">
                  {(detail.utxos ?? []).length} unspent output
                  {(detail.utxos ?? []).length === 1 ? '' : 's'}
                </p>
                {(detail.utxos ?? []).some((u) => u.locked) && (
                  <p className="text-xs text-warning mt-1">
                    {formatDcr(
                      (detail.utxos ?? [])
                        .filter((u) => u.locked)
                        .reduce((sum, u) => sum + u.atoms, 0),
                    )}{' '}
                    DCR committed to pending payments
                  </p>
                )}
                {(detail.utxos ?? []).length > 0 && (
                  <div className="mt-4 overflow-x-auto">
                    <table className="w-full text-xs">
                      <thead className="text-muted-foreground">
                        <tr>
                          <th className="text-left font-medium py-1">Address</th>
                          <th className="text-right font-medium py-1">Amount</th>
                          <th className="text-right font-medium py-1">Confirmations</th>
                        </tr>
                      </thead>
                      <tbody>
                        {(detail.utxos ?? []).map((u) => (
                          <tr key={`${u.txid}:${u.vout}`} className="border-t border-border/50">
                            <td className="py-1 max-w-[16rem]" title={`${u.txid}:${u.vout}`}>
                              <span className="flex items-center gap-2 min-w-0">
                                <span className="font-mono truncate">
                                  {(u.address || u.txid).slice(0, 14)}...
                                </span>
                                {u.locked && (
                                  <span
                                    className="px-1.5 py-0.5 rounded-full border border-warning/40 bg-warning/10 text-warning text-[10px] font-semibold whitespace-nowrap"
                                    title="Reserved by a pending payment"
                                  >
                                    committed
                                  </span>
                                )}
                              </span>
                            </td>
                            <td className="py-1 text-right">{formatDcr(u.atoms)}</td>
                            <td className="py-1 text-right">{u.confirmations}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            ) : (
              <p className="text-sm text-muted-foreground">
                Switch to wallet {detail.walletName} to see the balance.
              </p>
            )}
          </div>
        </div>
      ) : (
        <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
          <p className="text-sm text-muted-foreground">
            Receive addresses appear once every cosigner has confirmed. Do not send funds
            before then.
          </p>
        </div>
      )}

      {active && detail.isActiveWallet && (
        <ProposalComposePanel wallet={rec} onProposed={load} />
      )}

      {active && (
        <div className="space-y-3">
          <p className="text-sm font-medium">Payments</p>
          <ProposalList wallet={rec} proposals={proposals} onChanged={load} />
        </div>
      )}

      {rec.transport === 'manual' && !['declined', 'failed', 'cancelled'].includes(rec.status) && (
        <CoordinationCard wallet={rec} onChanged={load} />
      )}

      <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
        <p className="text-sm font-medium mb-3">Cosigners</p>
        <ul className="space-y-2 text-sm">
          <li className="flex items-center justify-between gap-3 py-2 border-b border-border/50">
            <span className="min-w-0">
              <span className="block truncate">You</span>
              {rec.ownHd && (
                <span className="block text-xs text-muted-foreground truncate">
                  <KeyEnds value={rec.ownHd.xpub} />
                </span>
              )}
            </span>
            <span className="text-xs text-muted-foreground whitespace-nowrap">
              {rec.role === 'initiator' ? 'Initiator' : 'Cosigner'}
            </span>
          </li>
          {rec.peers.map((p) => (
            <li key={p.uid} className="flex items-center justify-between gap-3 py-2 border-b border-border/50 last:border-0">
              <span className="min-w-0">
                <span className="block truncate">{p.nick || p.uid.slice(0, 12)}</span>
                {p.xpub ? (
                  <span className="block text-xs text-muted-foreground truncate">
                    <KeyEnds value={p.xpub} />
                  </span>
                ) : (
                  <span className="block text-xs font-mono text-muted-foreground truncate">
                    {p.uid.slice(0, 16)}
                  </span>
                )}
              </span>
              <span className="text-xs text-muted-foreground whitespace-nowrap">
                {msigPeerLabel(p.state)}
                {p.reason ? ` - ${p.reason}` : ''}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
};

export default SharedWalletDetailPage;
