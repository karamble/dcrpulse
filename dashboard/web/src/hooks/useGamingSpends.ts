// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useSyncExternalStore } from 'react';
import { GamingSpend, getGamingSpends } from '../services/gamingApi';
import { startVisiblePoll } from './useVisiblePoll';
import { apiError } from '../utils/apiError';

// One poll of the spend list, shared by everything that needs to know a game
// is waiting on somebody.
//
// It is a store rather than component state because three places need the same
// answer - the approvals panel, the strip that follows the operator between
// sections, and the count on the tab itself - and a request that lapses in two
// minutes must not depend on which of them happens to be mounted. The panel
// used to own the poll and offer the number through a callback prop that no
// caller ever passed.
export interface GamingSpendsSnapshot {
  // pending awaits a person, sorted by deadline; publishing is money on its
  // way to the network, with nothing left to answer.
  pending: GamingSpend[];
  publishing: GamingSpend[];
  // decided is the first page of history, newest first; decidedTotal is how
  // much of it exists. Deeper pages are fetched by whoever is reading them.
  decided: GamingSpend[];
  decidedTotal: number;
  // usedToday is the server's day total per game - the number the cap is
  // enforced against, not a client-side re-derivation that can drift.
  usedToday: Record<string, number>;
  // serverNow is the clock the deadline is enforced against; 0 when unknown.
  serverNow: number;
  offset: number;
  failures: number;
  error: string | null;
  lastOkAt: number;
}

const empty: GamingSpendsSnapshot = {
  pending: [],
  publishing: [],
  decided: [],
  decidedTotal: 0,
  usedToday: {},
  serverNow: 0,
  offset: 0,
  failures: 0,
  error: null,
  lastOkAt: 0,
};

let snapshot: GamingSpendsSnapshot = empty;
const listeners = new Set<() => void>();
let stop: (() => void) | null = null;

// signature is what makes an idle poll free. The arrays are new objects every
// five seconds, so comparing them would re-render every subscriber forever;
// comparing what they say does not.
const signature = (s: GamingSpendsSnapshot): string =>
  `${s.failures}|${s.error ?? ''}|${s.decidedTotal}|${Object.entries(s.usedToday)
    .sort()
    .map(([g, v]) => `${g}=${v}`)
    .join(',')}|${[...s.pending, ...s.publishing, ...s.decided]
    .map((x) => `${x.id}:${x.state}:${x.decidedAt ?? ''}:${x.txid ?? ''}`)
    .join(',')}`;

const publish = (next: GamingSpendsSnapshot) => {
  if (signature(next) === signature(snapshot)) {
    // Keep the newer clock even when nothing else moved, without waking
    // anybody: a countdown reads it on its own tick.
    snapshot = { ...snapshot, serverNow: next.serverNow, offset: next.offset };
    return;
  }
  snapshot = next;
  for (const l of listeners) l();
};

const check = async () => {
  try {
    const answer = await getGamingSpends();
    publish({
      pending: answer.pending
        .filter((s) => s.state === 'pending')
        .sort((a, b) => a.expiresAt - b.expiresAt),
      publishing: answer.pending.filter((s) => s.state === 'publishing'),
      decided: answer.decided,
      decidedTotal: answer.decidedTotal,
      usedToday: answer.usedToday,
      serverNow: answer.serverNow,
      offset: answer.serverNow ? answer.serverNow - Math.floor(Date.now() / 1000) : snapshot.offset,
      failures: 0,
      error: null,
      lastOkAt: Date.now(),
    });
  } catch (e) {
    // Keep the last known list. A transient failure is not evidence that
    // nothing is pending, and blanking the panel would say exactly that.
    publish({
      ...snapshot,
      failures: snapshot.failures + 1,
      error: apiError(e, 'Could not read the list of requests'),
    });
  }
};

const subscribe = (fn: () => void): (() => void) => {
  listeners.add(fn);
  if (!stop) stop = startVisiblePoll(() => void check(), 5000);
  return () => {
    listeners.delete(fn);
    if (listeners.size === 0 && stop) {
      stop();
      stop = null;
    }
  };
};

export const refreshGamingSpends = (): Promise<void> => check();

export const useGamingSpends = (): GamingSpendsSnapshot =>
  useSyncExternalStore(
    subscribe,
    () => snapshot,
    () => snapshot,
  );
