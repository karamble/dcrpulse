// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef } from 'react';

// useLatestRequest guards async work so only the latest request writes state.
// start() supersedes every earlier request and returns this one's check;
// current() returns a check for the latest request without superseding it, for
// follow-up work that belongs to it; cancel() and unmount supersede everything.
export function useLatestRequest() {
  const seq = useRef(0);
  useEffect(
    () => () => {
      seq.current++;
    },
    [],
  );
  return useMemo(() => {
    const checkFor = (id: number) => () => id === seq.current;
    return {
      start: () => checkFor(++seq.current),
      current: () => checkFor(seq.current),
      cancel: () => {
        seq.current++;
      },
    };
  }, []);
}
