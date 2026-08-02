// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef } from 'react';

// startVisiblePoll invokes fn every ms while the page is visible, skips ticks
// while it is hidden, and fires immediately when the page returns to the
// foreground so stale panels catch up. Plain function so non-hook callers
// (the shared wallet-status store) reuse the same behavior. Returns a stop
// function.
export function startVisiblePoll(fn: () => void, ms: number, immediate = true): () => void {
  let id: number | undefined;
  const stop = () => {
    if (id !== undefined) {
      window.clearInterval(id);
      id = undefined;
    }
  };
  const start = (fire: boolean) => {
    if (id !== undefined) return;
    if (fire) fn();
    id = window.setInterval(fn, ms);
  };
  const onVis = () => {
    if (document.visibilityState === 'visible') {
      start(true);
    } else {
      stop();
    }
  };
  document.addEventListener('visibilitychange', onVis);
  if (document.visibilityState === 'visible') start(immediate);
  return () => {
    stop();
    document.removeEventListener('visibilitychange', onVis);
  };
}

// useVisiblePoll is the hook form: a drop-in replacement for the repo's
// `load(); const id = setInterval(load, ms); return () => clearInterval(id)`
// effect shape. fn is kept in a ref so callers need no useCallback.
export function useVisiblePoll(
  fn: () => void,
  ms: number,
  opts?: { enabled?: boolean; immediate?: boolean },
): void {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const enabled = opts?.enabled ?? true;
  const immediate = opts?.immediate ?? true;
  useEffect(() => {
    if (!enabled) return;
    return startVisiblePoll(() => fnRef.current(), ms, immediate);
  }, [ms, enabled, immediate]);
}
