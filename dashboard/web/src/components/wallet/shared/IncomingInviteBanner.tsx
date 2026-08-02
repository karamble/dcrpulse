// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { AlertCircle, Loader2, UserPlus } from 'lucide-react';
import { AccountInfo, getAccounts } from '../../../services/api';
import { useBisonrelayLive } from '../../bisonrelay/BisonrelayLiveProvider';
import {
  MsigPendingItem,
  acceptMsigInvite,
  declineMsigInvite,
  getMsigPending,
} from '../../../services/msigApi';

// IncomingInviteBanner surfaces shared-wallet invitations waiting for an
// answer, plus rosters that need a wallet switch to finish importing.
export const IncomingInviteBanner = ({ onChanged }: { onChanged: () => void }) => {
  const [items, setItems] = useState<MsigPendingItem[]>([]);
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [account, setAccount] = useState<number | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
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
    getAccounts()
      .then((data) => {
        const visible = data
          .filter((a) => a.accountName !== 'imported' && a.accountNumber < 2147483647)
          .sort((a, b) => a.accountNumber - b.accountNumber);
        setAccounts(visible);
        if (visible.length > 0) setAccount(visible[0].accountNumber);
      })
      .catch(() => undefined);
  }, [load]);

  useEffect(() => addListener((evt) => {
    if (evt.type === 'msig') load();
  }), [addListener, load]);

  const act = async (item: MsigPendingItem, accept: boolean) => {
    if (busyId || account === null) return;
    setBusyId(item.tempId);
    setErr(null);
    try {
      if (accept) await acceptMsigInvite(item.tempId, account);
      else await declineMsigInvite(item.tempId);
      await load();
      onChanged();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not answer the invitation');
    } finally {
      setBusyId(null);
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
          className="p-4 rounded-xl bg-gradient-card backdrop-blur-sm border border-primary/30"
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
                      : ' Accepting contributes one key from the account you choose.'}
                  </p>
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
                <select
                  value={account ?? ''}
                  onChange={(e) => setAccount(Number(e.target.value))}
                  className="px-2 py-2 rounded-lg bg-background border border-border text-sm focus:outline-none focus:border-primary"
                >
                  {accounts.map((a) => (
                    <option key={a.accountNumber} value={a.accountNumber}>
                      {a.accountName}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  onClick={() => act(item, false)}
                  disabled={busyId === item.tempId}
                  className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm disabled:opacity-50"
                >
                  Decline
                </button>
                <button
                  type="button"
                  onClick={() => act(item, true)}
                  disabled={busyId === item.tempId || account === null}
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
    </div>
  );
};

export default IncomingInviteBanner;
