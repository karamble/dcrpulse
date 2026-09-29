// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useSyncExternalStore } from 'react';
import { startVisiblePoll } from './useVisiblePoll';
import { shallowEqual } from '../utils/shallowEqual';

// createPolledStore backs a hook with one visibility-gated poll shared by the
// whole app: the first subscriber starts it, the last stops it, and a result
// that compares equal to the previous one is never republished, so idle polls
// cause no re-renders. onError decides what a failed poll shows. Snapshots
// must be flat objects, per the shallowEqual dedupe.
export const createPolledStore = <T extends object>({
  load,
  ms,
  initial,
  onError,
}: {
  load: () => Promise<T>;
  ms: number;
  initial: T;
  onError: (err: unknown, prev: T) => T;
}) => {
  let snapshot = initial;
  const listeners = new Set<() => void>();
  let stopPoll: (() => void) | null = null;
  let inflight = false;

  const check = async () => {
    if (inflight) return;
    inflight = true;
    let next: T;
    try {
      next = await load();
    } catch (err) {
      next = onError(err, snapshot);
    } finally {
      inflight = false;
    }
    if (!shallowEqual(next, snapshot)) {
      snapshot = next;
      listeners.forEach((l) => l());
    }
  };

  const subscribe = (listener: () => void) => {
    listeners.add(listener);
    if (listeners.size === 1) stopPoll = startVisiblePoll(check, ms);
    return () => {
      listeners.delete(listener);
      if (listeners.size === 0) {
        stopPoll?.();
        stopPoll = null;
      }
    };
  };

  return {
    use: (): T => useSyncExternalStore(subscribe, () => snapshot),
    refresh: () => void check(),
  };
};
