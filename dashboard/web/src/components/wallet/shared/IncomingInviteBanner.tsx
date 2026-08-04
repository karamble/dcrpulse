// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { AlertCircle, Loader2, UserPlus } from 'lucide-react';
import { useBisonrelayLive } from '../../bisonrelay/BisonrelayLiveProvider';
import { PassphraseModal } from '../PassphraseModal';
import {
  MsigPendingItem,
  acceptMsigInvite,
  declineMsigInvite,
  getMsigPending,
} from '../../../services/msigApi';

// IncomingInviteBanner surfaces shared-wallet invitations waiting for an
// answer, plus rosters that need a wallet switch to finish importing.
// Accepting creates the wallet's dedicated account, so it asks for the
// wallet passphrase; declining never does.
export const IncomingInviteBanner = ({ onChanged }: { onChanged: () => void }) => {
  const [items, setItems] = useState<MsigPendingItem[]>([]);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [askPassFor, setAskPassFor] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();

  const load = useCallback(async () => {
    try {
      setItems(await getMsigPending());
    } catch {
      // A daemon hiccup should not blank the page it sits on.
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

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

  const decline = async (item: MsigPendingItem) => {
    if (busyId) return;
    setBusyId(item.tempId);
    setErr(null);
    try {
      await declineMsigInvite(item.tempId);
      await load();
      onChanged();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not answer the invitation');
    } finally {
      setBusyId(null);
    }
  };

  const accept = async (tempId: string, passphrase: string) => {
    setErr(null);
    try {
      await acceptMsigInvite(tempId, passphrase);
      setAskPassFor(null);
      await load();
      onChanged();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not accept the invitation');
      throw e;
    }
  };

  if (items.length === 0) return null;

  return (
    <div className="space-y-3">
      {err && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{err}</span>
        </div>
      )}
      {items.map((item) => (
        <div
          key={`${item.walletName}:${item.tempId}`}
          className="p-4 rounded-xl bg-gradient-card border border-primary/30"
        >
          <div className="flex items-start gap-3 flex-wrap">
            <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
              <UserPlus className="h-4 w-4 text-primary" />
            </div>
            <div className="flex-1 min-w-0">
              {item.kind === 'invite' ? (
                <>
                  <p className="font-medium">
                    {item.initiatorNick || 'A contact'} invited you to the shared wallet
                    {' '}
                    <span className="font-semibold">{item.label}</span>
                  </p>
                  <p className="text-sm text-muted-foreground">
                    {item.m} of {item.n} signatures.
                    {item.needsSwitch
                      ? ` Belongs to wallet ${item.walletName}; switch to it to accept.`
                      : ' Accepting creates a dedicated account and shares only its extended public key.'}
                  </p>
                  {/* Who the sender says the other cosigners are. The sender
                      supplies these names, so they are a claim and nothing
                      more; the real check comes before the wallet can be
                      funded. Showing them here at least lets an invitee spot a
                      participant they never agreed to. */}
                  {(item.proposedPeers?.length ?? 0) > 0 && (
                    <p className="text-sm text-muted-foreground mt-1">
                      Says the cosigners are:{' '}
                      <span className="text-foreground">
                        {item.proposedPeers?.map((p) => p.nick || 'unnamed').join(', ')}
                      </span>
                      . You will check every cosigner's key before this wallet can receive
                      funds.
                    </p>
                  )}
                </>
              ) : (
                <>
                  <p className="font-medium">
                    <span className="font-semibold">{item.label}</span> is verified and waiting
                  </p>
                  <p className="text-sm text-muted-foreground">
                    Switch to wallet {item.walletName} to finish setting it up.
                  </p>
                </>
              )}
            </div>
            {item.kind === 'invite' && !item.needsSwitch && (
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => decline(item)}
                  disabled={busyId === item.tempId}
                  className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm disabled:opacity-50"
                >
                  Decline
                </button>
                <button
                  type="button"
                  onClick={() => setAskPassFor(item.tempId)}
                  disabled={busyId === item.tempId}
                  className="px-3 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm disabled:opacity-50 inline-flex items-center gap-2"
                >
                  {busyId === item.tempId && <Loader2 className="h-4 w-4 animate-spin" />}
                  Accept
                </button>
              </div>
            )}
          </div>
        </div>
      ))}

      <PassphraseModal
        isOpen={askPassFor !== null}
        title="Join the shared wallet"
        description="Your wallet passphrase creates this wallet's dedicated account. Only the account's extended public key is shared with cosigners."
        submitLabel="Accept invitation"
        busyLabel="Joining..."
        onSubmit={(pass) => accept(askPassFor!, pass)}
        onClose={() => setAskPassFor(null)}
      />
    </div>
  );
};

export default IncomingInviteBanner;
