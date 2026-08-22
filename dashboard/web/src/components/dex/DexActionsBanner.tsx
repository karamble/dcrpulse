// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, Loader2 } from 'lucide-react';
import {
  DEX_ACTION_REDEEM_REJECTED,
  DexAction,
  DexRejectedRedemption,
  getDexActions,
  resolveRejectedRedemption,
} from '../../services/dcrdexApi';
import { useDexRefreshOnNotes } from './DexLiveProvider';
import { apiError } from '../../utils/apiError';

// DexActionsBanner shows the decisions bisonw is waiting on. The daemon raises
// these as notifications at a severity it never stores, so the list is the
// source of truth and the notes only prompt a re-read. Only rejected
// redemptions are answered here; the daemon's other action kinds are raised
// solely by its Ethereum wallet, which this dashboard does not ship.
export const DexActionsBanner = () => {
  const [actions, setActions] = useState<DexAction[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const refresh = useCallback(() => {
    getDexActions()
      .then(setActions)
      .catch(() => {
        /* the banner stays as it was; a live note will prompt another read */
      });
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);
  // Deliberately not walletnote: it carries the daemon's Ethereum-only action
  // kinds, which are out of scope here, and it fires often enough that
  // re-reading the whole user snapshot on each one would be wasteful. The order
  // and match notes cover a decision taken in another tab.
  useDexRefreshOnNotes(['actionrequired', 'order', 'match'], refresh);

  const answer = async (action: DexAction, payload: DexRejectedRedemption, retry: boolean) => {
    setBusy(action.uniqueID);
    setErr(null);
    try {
      await resolveRejectedRedemption(action, payload, retry);
      // The daemon sends no resolution note for this kind, so the prompt is
      // cleared here. Another tab keeps it until its own next read.
      setActions((prev) => prev.filter((a) => a.uniqueID !== action.uniqueID));
    } catch (e: any) {
      setErr(apiError(e, 'Could not answer'));
    } finally {
      setBusy(null);
    }
  };

  const rejected = actions.filter((a) => a.actionID === DEX_ACTION_REDEEM_REJECTED && a.payload);
  if (rejected.length === 0) return null;

  return (
    <div className="mx-4 space-y-2">
      {rejected.map((a) => {
        const p = a.payload as DexRejectedRedemption;
        return (
          <div
            key={a.uniqueID}
            className="px-3 py-2 rounded-lg bg-warning/10 border border-warning/30 text-xs text-foreground"
          >
            <div className="flex items-start gap-2">
              <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0 text-warning" />
              <div className="min-w-0 flex-1 space-y-2">
                <p>
                  A redemption was rejected by the network and its fees are already spent. Trying
                  again will probably cost more fees and may be rejected again. Leaving it alone
                  parks the trade until DCRDEX restarts and asks once more.
                </p>
                <p className="font-mono text-[10px] text-muted-foreground break-all">{p.coinFmt}</p>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    disabled={busy === a.uniqueID}
                    onClick={() => answer(a, p, true)}
                    className="px-3 py-1 rounded-md bg-gradient-primary text-white text-xs font-semibold transition disabled:opacity-50 inline-flex items-center gap-1.5"
                  >
                    {busy === a.uniqueID && <Loader2 className="h-3 w-3 animate-spin" />}
                    Try again
                  </button>
                  <button
                    type="button"
                    disabled={busy === a.uniqueID}
                    onClick={() => answer(a, p, false)}
                    className="px-3 py-1 rounded-md border border-border text-xs text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50"
                  >
                    Leave it
                  </button>
                  <a
                    href="https://docs.decred.org/getting-started/joining-matrix-channels/"
                    target="_blank"
                    rel="noreferrer"
                    className="text-[11px] text-primary underline ml-auto"
                  >
                    Find technical support
                  </a>
                </div>
              </div>
            </div>
          </div>
        );
      })}
      {err && <p className="text-[11px] text-destructive px-1">{err}</p>}
    </div>
  );
};
