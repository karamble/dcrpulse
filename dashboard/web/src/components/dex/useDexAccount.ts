// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { dexAccountState, getDexAccount, type DexAccount } from '../../services/dcrdexApi';
import { useDexRefreshOnNotes } from './DexLiveProvider';

// useDexAccount fetches a DEX server's account state (tier, bonds, reputation)
// and keeps it fresh off the bond/reputation notification feed. It backs both
// the account-active indicator and the trade gate: `tradable` is true once the
// effective tier reaches 1, mirroring the dcrdex web client. A transient fetch
// failure leaves acct null, in which case the caller should not gate (the daemon
// still rejects an order on an inactive account).
export const useDexAccount = (host: string) => {
  const [acct, setAcct] = useState<DexAccount | null>(null);

  const refresh = () => getDexAccount(host).then(setAcct).catch(() => {});
  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 60000);
    return () => window.clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host]);
  useDexRefreshOnNotes(['bondpost', 'bondrefund', 'reputation'], refresh);

  const status = dexAccountState(acct);
  return { acct, status, tradable: status.state === 'active', refresh };
};
