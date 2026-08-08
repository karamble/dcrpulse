// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, Check, Loader2, Send, X } from 'lucide-react';
import { PassphraseModal } from '../PassphraseModal';
import {
  MsigProposal,
  MsigWallet,
  abortMsigProposal,
  msigHopLabel,
  msigProposalLabel,
  rebroadcastMsigProposal,
  rejectMsigProposal,
  signMsigProposal,
} from '../../../services/msigApi';
import { formatAtoms } from '../../../utils/amounts';
import { apiError } from '../../../utils/apiError';


const tone = (status: string): string => {
  switch (status) {
    case 'confirmed':
    case 'broadcast':
      return 'bg-success/10 text-success border-success/20';
    case 'incoming':
      return 'bg-warning/10 text-warning border-warning/20';
    case 'declined':
    case 'failed':
    case 'aborted':
    case 'superseded':
      return 'bg-destructive/10 text-destructive border-destructive/20';
    default:
      return 'bg-muted/30 text-muted-foreground border-border/50';
  }
};

// ProposalList renders every payment of a shared wallet, with the review
// and approval controls for requests waiting on this node.
export const ProposalList = ({
  wallet,
  proposals,
  onChanged,
}: {
  wallet: MsigWallet;
  proposals: MsigProposal[];
  onChanged: () => void;
}) => {
  const [askPassFor, setAskPassFor] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  if (proposals.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
        <p className="text-sm text-muted-foreground">No payments yet.</p>
      </div>
    );
  }

  const run = async (txid: string, fn: () => Promise<void>) => {
    setBusy(txid);
    setErr(null);
    try {
      await fn();
      onChanged();
    } catch (e: any) {
      setErr(apiError(e, 'Action failed'));
      // A rejected action usually means the card is stale; re-sync it.
      onChanged();
    } finally {
      setBusy(null);
    }
  };

  const approve = async (txid: string, passphrase: string) => {
    try {
      await signMsigProposal(wallet.tempId, txid, passphrase);
      setAskPassFor(null);
      onChanged();
    } catch (e: any) {
      setErr(apiError(e, 'Could not approve'));
      throw e;
    }
  };

  return (
    <div className="space-y-3">
      {err && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{err}</span>
        </div>
      )}
      {proposals.map((p) => {
        const payments = p.outputs.filter((o) => !o.isChange);
        const total = payments.reduce((sum, o) => sum + o.atoms, 0);
        return (
          <div key={p.txid} className="p-5 rounded-xl bg-gradient-card border border-border/50 space-y-3">
            <div className="flex items-start justify-between gap-3 flex-wrap">
              <div className="min-w-0">
                <p className="font-semibold">
                  {formatAtoms(total)} DCR
                  {p.role === 'cosigner' && p.fromNick ? ` requested by ${p.fromNick}` : ''}
                </p>
                <p className="text-xs text-muted-foreground">
                  {p.sigCount} of {wallet.m} signatures - fee {formatAtoms(p.feeAtoms)} DCR
                </p>
                {p.note && <p className="text-sm mt-1">{p.note}</p>}
              </div>
              <span className={`px-2 py-1 rounded-full border text-[10px] font-semibold whitespace-nowrap ${tone(p.status)}`}>
                {msigProposalLabel(p.status)}
              </span>
            </div>

            {payments.length > 0 && (
              <ul className="text-xs space-y-1">
                {payments.map((o, i) => (
                  <li key={`${o.address}-${i}`} className="flex justify-between gap-3">
                    <span className="font-mono truncate">{o.address || 'unrecognized output'}</span>
                    <span className="whitespace-nowrap">{formatAtoms(o.atoms)} DCR</span>
                  </li>
                ))}
              </ul>
            )}

            {p.reason && <p className="text-xs text-destructive">{p.reason}</p>}

            {p.queue && p.queue.length > 0 && (
              <ul className="text-xs text-muted-foreground space-y-1">
                {p.queue.map((h) => (
                  <li key={h.uid} className="flex justify-between gap-3">
                    <span className="truncate">{h.nick || h.uid.slice(0, 12)}</span>
                    <span className="whitespace-nowrap">
                      {msigHopLabel(h.state)}
                      {h.reason ? ` - ${h.reason}` : ''}
                    </span>
                  </li>
                ))}
              </ul>
            )}

            {wallet.transport === 'manual' &&
              (p.queue ?? []).some((h) => h.state === 'sent') && (
                <p className="text-xs text-muted-foreground">
                  Hand the signing request to its cosigner from the Coordination card below,
                  and import their answer there.
                </p>
              )}

            <p className="text-[10px] font-mono text-muted-foreground break-all">
              <Link to={`/explorer/tx/${p.txid}`} className="hover:text-primary">
                {p.txid}
              </Link>
            </p>

            <div className="flex items-center gap-2 flex-wrap">
              {p.status === 'incoming' && (
                <>
                  <button
                    type="button"
                    onClick={() => setAskPassFor(p.txid)}
                    disabled={busy === p.txid}
                    className="px-3 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm disabled:opacity-50 inline-flex items-center gap-2"
                  >
                    <Check className="h-4 w-4" />
                    Approve
                  </button>
                  <button
                    type="button"
                    onClick={() => run(p.txid, () => rejectMsigProposal(wallet.tempId, p.txid))}
                    disabled={busy === p.txid}
                    className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm disabled:opacity-50 inline-flex items-center gap-2"
                  >
                    {busy === p.txid ? <Loader2 className="h-4 w-4 animate-spin" /> : <X className="h-4 w-4" />}
                    Decline
                  </button>
                </>
              )}
              {p.role === 'initiator' && (p.status === 'collecting' || p.status === 'ready') && (
                <button
                  type="button"
                  onClick={() => run(p.txid, () => abortMsigProposal(wallet.tempId, p.txid))}
                  disabled={busy === p.txid}
                  className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm disabled:opacity-50"
                >
                  Cancel payment
                </button>
              )}
              {p.role === 'initiator' && p.status === 'ready' && (
                <button
                  type="button"
                  onClick={() => run(p.txid, () => rebroadcastMsigProposal(wallet.tempId, p.txid))}
                  disabled={busy === p.txid}
                  className="px-3 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm disabled:opacity-50 inline-flex items-center gap-2"
                >
                  <Send className="h-4 w-4" />
                  Broadcast now
                </button>
              )}
            </div>
          </div>
        );
      })}

      <PassphraseModal
        isOpen={askPassFor !== null}
        title="Approve this payment"
        description="Check the amounts and destination above. Your signature is added to the transaction and returned to the proposer."
        submitLabel="Approve"
        busyLabel="Signing..."
        onSubmit={(pass) => approve(askPassFor!, pass)}
        onClose={() => setAskPassFor(null)}
      />
    </div>
  );
};

export default ProposalList;
